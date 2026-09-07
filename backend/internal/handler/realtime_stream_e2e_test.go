// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package handler

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	apprealtime "github.com/opendefender/openrisk/internal/application/realtime"
	"github.com/opendefender/openrisk/internal/domain"
	"github.com/opendefender/openrisk/internal/infrastructure/repository"
	"github.com/opendefender/openrisk/internal/middleware"
	authpkg "github.com/opendefender/openrisk/pkg/auth"
	rt "github.com/opendefender/openrisk/pkg/realtime"
)

// ---------------------------------------------------------------------------
// End-to-end coverage of GET /realtime/events over a real HTTP listener.
//
// A real listener rather than fiber's in-memory test helper: an SSE response
// never ends, so anything that reads the whole body before returning cannot
// exercise this endpoint at all. Every assertion below therefore travels the
// same path a browser does — real socket, real headers, real chunked frames.
// ---------------------------------------------------------------------------

// allPermissions is what a tenant admin holds.
var allPermissions = []string{"*"}

type streamTestEnv struct {
	baseURL string
	pub     *apprealtime.Publisher
	hub     *apprealtime.Hub
	log     *repository.GormRealtimeEventRepository
	db      *gorm.DB
}

// newStreamTestEnv stands up the hub, the durable log and the route, behind a
// stand-in for the auth middleware that reads the identity from test headers.
// The handler itself takes the tenant from the request context exactly as it
// does in production — the stand-in replaces how the session is resolved, never
// what the handler trusts.
func newStreamTestEnv(t *testing.T, opts apprealtime.HubOptions) *streamTestEnv {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.RealtimeEvent{}))

	eventLog := repository.NewGormRealtimeEventRepository(db)
	hub := apprealtime.NewHub(opts)
	pub := apprealtime.NewPublisher(eventLog, hub)
	// A short keepalive so a closed client is noticed in milliseconds: the
	// failing write IS how the server learns the socket is gone, and a suite
	// that opened streams on the production interval would spend its runtime
	// waiting for teardown rather than testing anything.
	h := NewRealtimeHandlerWithOptions(hub, eventLog, RealtimeStreamOptions{
		KeepaliveInterval: 150 * time.Millisecond,
		MaxLifetime:       30 * time.Second,
	})

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		tenantHeader := c.Get("X-Test-Tenant")
		if tenantHeader == "" {
			// No session at all: the handler must fail closed.
			return c.Next()
		}
		tenant, err := uuid.Parse(tenantHeader)
		if err != nil {
			return c.Next()
		}
		middleware.SetContext(c, &middleware.RequestContext{
			UserID:         uuid.New(),
			OrganizationID: tenant,
		})
		perms := allPermissions
		if raw := c.Get("X-Test-Perms"); raw != "" {
			perms = strings.Split(raw, ",")
		}
		c.Locals("user", &authpkg.Claims{Permissions: perms})
		// The session id the revocation check reads on each keepalive (#345).
		// Absent unless a test asks for one, so every pre-existing test keeps
		// the behaviour it was written against: no jti means nothing to revoke.
		if jti := c.Get("X-Test-JTI"); jti != "" {
			c.Locals("jti", jti)
		}
		return c.Next()
	})
	app.Get("/api/v1/realtime/events", h.Stream)
	app.Get("/api/v1/realtime/catalog", h.Catalog)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go func() { _ = app.Listener(ln) }()
	t.Cleanup(func() { _ = app.Shutdown() })

	return &streamTestEnv{
		baseURL: "http://" + ln.Addr().String(),
		pub:     pub,
		hub:     hub,
		log:     eventLog,
		db:      db,
	}
}

func (e *streamTestEnv) publish(t *testing.T, tenant uuid.UUID, typ rt.EventType, aggID string) rt.Envelope {
	t.Helper()
	out, err := e.pub.Publish(context.Background(), rt.Envelope{
		Type:      typ,
		TenantID:  tenant.String(),
		Aggregate: rt.Aggregate{ID: aggID},
	})
	require.NoError(t, err)
	return out
}

// sseFrame is one parsed server-sent event.
type sseFrame struct {
	ID    string
	Event string
	Data  map[string]any
}

// sseClient reads a stream incrementally, the way a browser does.
type sseClient struct {
	resp   *http.Response
	frames chan sseFrame
	cancel context.CancelFunc
	once   sync.Once
}

// openStream connects and starts parsing. The returned status lets a caller
// assert a refusal without waiting for frames that will never come.
func (e *streamTestEnv) openStream(t *testing.T, headers map[string]string, query string) (*sseClient, int) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	url := e.baseURL + "/api/v1/realtime/events"
	if query != "" {
		url += "?" + query
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	require.NoError(t, err)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("stream request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		cancel()
		return nil, resp.StatusCode
	}

	cl := &sseClient{resp: resp, frames: make(chan sseFrame, 256), cancel: cancel}
	go cl.read()
	t.Cleanup(cl.close)
	return cl, resp.StatusCode
}

func (c *sseClient) read() {
	defer close(c.frames)
	scanner := bufio.NewScanner(c.resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var cur sseFrame
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "":
			if cur.Event != "" {
				c.frames <- cur
			}
			cur = sseFrame{}
		case strings.HasPrefix(line, ":"):
			// A keepalive comment. A browser never surfaces these, which is
			// precisely why the server also emits a named heartbeat.
		case strings.HasPrefix(line, "id: "):
			cur.ID = strings.TrimPrefix(line, "id: ")
		case strings.HasPrefix(line, "event: "):
			cur.Event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			raw := strings.TrimPrefix(line, "data: ")
			var payload map[string]any
			if err := json.Unmarshal([]byte(raw), &payload); err == nil {
				cur.Data = payload
			}
		}
	}
}

func (c *sseClient) close() {
	c.once.Do(func() {
		c.cancel()
		_ = c.resp.Body.Close()
	})
}

// next returns the next frame, skipping heartbeats, or fails the test.
func (c *sseClient) next(t *testing.T, within time.Duration) sseFrame {
	t.Helper()
	deadline := time.After(within)
	for {
		select {
		case f, ok := <-c.frames:
			if !ok {
				t.Fatal("the stream closed while a frame was expected")
			}
			if f.Event == "stream.heartbeat" {
				continue
			}
			return f
		case <-deadline:
			t.Fatal("timed out waiting for a stream frame")
		}
	}
}

// expectNothing asserts no domain event arrives. This is the assertion that
// carries the isolation guarantee, so it waits long enough to be meaningful.
func (c *sseClient) expectNothing(t *testing.T, within time.Duration) {
	t.Helper()
	deadline := time.After(within)
	for {
		select {
		case f, ok := <-c.frames:
			if !ok {
				return
			}
			if f.Event == "stream.heartbeat" || strings.HasPrefix(f.Event, "stream.") {
				continue
			}
			t.Fatalf("received an event that must never have been delivered: %s %v", f.Event, f.Data)
		case <-deadline:
			return
		}
	}
}

func (c *sseClient) awaitHello(t *testing.T) sseFrame {
	t.Helper()
	f := c.next(t, 3*time.Second)
	require.Equal(t, "stream.hello", f.Event, "the first frame must identify the stream")
	return f
}

func tenantHeaders(tenant uuid.UUID, perms ...string) map[string]string {
	h := map[string]string{"X-Test-Tenant": tenant.String()}
	if len(perms) > 0 {
		h["X-Test-Perms"] = strings.Join(perms, ",")
	}
	return h
}

// ===========================================================================
// THE mandatory cross-tenant test, end to end over HTTP.
// ===========================================================================

func TestRealtimeStream_CrossTenantIsolation(t *testing.T) {
	e := newStreamTestEnv(t, apprealtime.HubOptions{})
	a, b := uuid.New(), uuid.New()

	streamA, status := e.openStream(t, tenantHeaders(a), "")
	require.Equal(t, http.StatusOK, status)
	streamB, status := e.openStream(t, tenantHeaders(b), "")
	require.Equal(t, http.StatusOK, status)

	helloA := streamA.awaitHello(t)
	assert.Equal(t, a.String(), helloA.Data["tenant_id"])
	helloB := streamB.awaitHello(t)
	assert.Equal(t, b.String(), helloB.Data["tenant_id"])

	// Tenant A produces a risk event.
	e1 := e.publish(t, a, rt.RiskCreated, "risk-of-A")

	got := streamA.next(t, 3*time.Second)
	assert.Equal(t, string(rt.RiskCreated), got.Event)
	assert.Equal(t, e1.ID, got.Data["id"])
	assert.Equal(t, a.String(), got.Data["tenantId"])
	streamB.expectNothing(t, 400*time.Millisecond)

	// Tenant B produces an asset event.
	e2 := e.publish(t, b, rt.AssetCreated, "asset-of-B")

	got = streamB.next(t, 3*time.Second)
	assert.Equal(t, string(rt.AssetCreated), got.Event)
	assert.Equal(t, e2.ID, got.Data["id"])
	assert.Equal(t, b.String(), got.Data["tenantId"])
	streamA.expectNothing(t, 400*time.Millisecond)
}

// A client naming another tenant, by any means it can reach, must be ignored:
// the tenant comes from the resolved session and from nowhere else.
func TestRealtimeStream_ForgedTenantIdentifiersAreIgnored(t *testing.T) {
	e := newStreamTestEnv(t, apprealtime.HubOptions{})
	mine, victim := uuid.New(), uuid.New()

	headers := tenantHeaders(mine)
	headers["X-Organization-ID"] = victim.String()
	headers["X-Tenant-ID"] = victim.String()

	stream, status := e.openStream(t, headers,
		"tenant_id="+victim.String()+"&tenantId="+victim.String()+"&organization_id="+victim.String())
	require.Equal(t, http.StatusOK, status)

	hello := stream.awaitHello(t)
	assert.Equal(t, mine.String(), hello.Data["tenant_id"],
		"the stream must be bound to the session's tenant, not to one the client named")

	e.publish(t, victim, rt.RiskCreated, "secret-risk")
	stream.expectNothing(t, 700*time.Millisecond)

	own := e.publish(t, mine, rt.RiskCreated, "my-risk")
	got := stream.next(t, 3*time.Second)
	assert.Equal(t, own.ID, got.Data["id"])
}

func TestRealtimeStream_RefusesAnUnauthenticatedConnection(t *testing.T) {
	e := newStreamTestEnv(t, apprealtime.HubOptions{})
	_, status := e.openStream(t, nil, "")
	assert.Equal(t, http.StatusUnauthorized, status,
		"a stream with no session must be refused, never opened as an empty one")
}

// A stream is refused, not silently emptied, when the caller may read nothing
// it could carry. A connected-but-forever-silent stream is indistinguishable
// from a broken one.
func TestRealtimeStream_RefusesWhenNoCategoryIsReadable(t *testing.T) {
	e := newStreamTestEnv(t, apprealtime.HubOptions{})
	tenant := uuid.New()

	_, status := e.openStream(t, tenantHeaders(tenant, "reports:board:read"), "")
	assert.Equal(t, http.StatusForbidden, status)
}

// Holding events:read does not mean receiving every category: each event is
// gated on the read permission of its own aggregate.
func TestRealtimeStream_DeliversOnlyTheCategoriesTheCallerMayRead(t *testing.T) {
	e := newStreamTestEnv(t, apprealtime.HubOptions{})
	tenant := uuid.New()

	stream, status := e.openStream(t, tenantHeaders(tenant, "events:read", "risks:read"), "")
	require.Equal(t, http.StatusOK, status)
	stream.awaitHello(t)

	// An asset event this session may not read.
	e.publish(t, tenant, rt.AssetCreated, "asset-1")
	stream.expectNothing(t, 700*time.Millisecond)

	// A risk event it may.
	wanted := e.publish(t, tenant, rt.RiskCreated, "risk-1")
	got := stream.next(t, 3*time.Second)
	assert.Equal(t, wanted.ID, got.Data["id"])
}

func TestRealtimeStream_RefusesAnUnknownFilterToken(t *testing.T) {
	e := newStreamTestEnv(t, apprealtime.HubOptions{})
	tenant := uuid.New()

	_, status := e.openStream(t, tenantHeaders(tenant), "types=risk.creted")
	assert.Equal(t, http.StatusBadRequest, status,
		"a misspelled subscription must be reported, not silently deliver nothing")
}

func TestRealtimeStream_ClientFilterNarrowsWithinTheTenant(t *testing.T) {
	e := newStreamTestEnv(t, apprealtime.HubOptions{})
	tenant := uuid.New()

	stream, status := e.openStream(t, tenantHeaders(tenant), "aggregates=asset")
	require.Equal(t, http.StatusOK, status)
	stream.awaitHello(t)

	e.publish(t, tenant, rt.RiskCreated, "risk-1")
	stream.expectNothing(t, 500*time.Millisecond)

	wanted := e.publish(t, tenant, rt.AssetUpdated, "asset-1")
	got := stream.next(t, 3*time.Second)
	assert.Equal(t, wanted.ID, got.Data["id"])
}

// ===========================================================================
// THE mandatory reconnect test.
//
//	connect → receive E1 → disconnect → produce E2 → reconnect with the cursor
//	→ receive E2, and E1 is NOT delivered again.
// ===========================================================================

func TestRealtimeStream_ReconnectReplaysOnlyWhatWasMissed(t *testing.T) {
	e := newStreamTestEnv(t, apprealtime.HubOptions{})
	tenant := uuid.New()

	stream, status := e.openStream(t, tenantHeaders(tenant), "")
	require.Equal(t, http.StatusOK, status)
	stream.awaitHello(t)

	e1 := e.publish(t, tenant, rt.RiskCreated, "risk-1")
	first := stream.next(t, 3*time.Second)
	require.Equal(t, e1.ID, first.Data["id"])
	require.Equal(t, "1", first.ID, "the SSE id must be the sequence, so it can be the resume cursor")

	// The client goes away.
	stream.close()

	// Two events happen while it is gone.
	e2 := e.publish(t, tenant, rt.RiskUpdated, "risk-1")
	e3 := e.publish(t, tenant, rt.RiskStatusChanged, "risk-1")

	// It comes back with the cursor the protocol gave it.
	reconnected, status := e.openStream(t, map[string]string{
		"X-Test-Tenant": tenant.String(),
		"Last-Event-ID": first.ID,
	}, "")
	require.Equal(t, http.StatusOK, status)
	reconnected.awaitHello(t)

	replay1 := reconnected.next(t, 3*time.Second)
	assert.Equal(t, e2.ID, replay1.Data["id"], "the first missed event must be replayed")
	replay2 := reconnected.next(t, 3*time.Second)
	assert.Equal(t, e3.ID, replay2.Data["id"], "replay must arrive in the order things happened")

	// E1 was already seen and must not come back.
	reconnected.expectNothing(t, 500*time.Millisecond)

	// And the stream is live again afterwards.
	e4 := e.publish(t, tenant, rt.RiskDeleted, "risk-1")
	live := reconnected.next(t, 3*time.Second)
	assert.Equal(t, e4.ID, live.Data["id"], "the stream must go live after catching up")
}

// A replay cursor is a client-supplied value, which makes it the input someone
// would use to reach another tenant's history.
func TestRealtimeStream_ReplayCannotReachAnotherTenantsHistory(t *testing.T) {
	e := newStreamTestEnv(t, apprealtime.HubOptions{})
	victim, attacker := uuid.New(), uuid.New()

	// The victim has three events; the attacker's tenant has none.
	e.publish(t, victim, rt.RiskCreated, "victim-risk-1")
	e.publish(t, victim, rt.RiskUpdated, "victim-risk-1")
	e.publish(t, victim, rt.RiskDeleted, "victim-risk-1")

	stream, status := e.openStream(t, map[string]string{
		"X-Test-Tenant": attacker.String(),
		"Last-Event-ID": "0",
	}, "")
	require.Equal(t, http.StatusOK, status)
	stream.awaitHello(t)
	stream.expectNothing(t, 700*time.Millisecond)

	// Even naming a sequence that exists in the victim's log yields nothing.
	stream2, status := e.openStream(t, map[string]string{
		"X-Test-Tenant": attacker.String(),
		"Last-Event-ID": "1",
	}, "")
	require.Equal(t, http.StatusOK, status)
	stream2.awaitHello(t)
	stream2.expectNothing(t, 700*time.Millisecond)
}

// A cursor older than the retained window cannot be honoured, and the client
// must be told so rather than handed a partial replay it would mistake for a
// complete one.
func TestRealtimeStream_ExpiredCursorAsksForAResync(t *testing.T) {
	e := newStreamTestEnv(t, apprealtime.HubOptions{})
	tenant := uuid.New()

	for i := 0; i < 4; i++ {
		e.publish(t, tenant, rt.RiskUpdated, "risk-1")
	}
	// Retention removed the first three.
	require.NoError(t, e.db.Where("tenant_id = ? AND sequence <= ?", tenant, 3).
		Delete(&domain.RealtimeEvent{}).Error)

	stream, status := e.openStream(t, map[string]string{
		"X-Test-Tenant": tenant.String(),
		"Last-Event-ID": "1",
	}, "")
	require.Equal(t, http.StatusOK, status)
	stream.awaitHello(t)

	f := stream.next(t, 3*time.Second)
	require.Equal(t, "stream.resync", f.Event)
	assert.Equal(t, "cursor_expired", f.Data["reason"])
	assert.EqualValues(t, 4, f.Data["oldest_retained"])
}

// A first connection is not a gap: a fresh page loads its data from the API, so
// replaying history into it would be work nobody asked for.
func TestRealtimeStream_FirstConnectionDoesNotReplayHistory(t *testing.T) {
	e := newStreamTestEnv(t, apprealtime.HubOptions{})
	tenant := uuid.New()

	e.publish(t, tenant, rt.RiskCreated, "old-1")
	e.publish(t, tenant, rt.RiskUpdated, "old-1")

	stream, status := e.openStream(t, tenantHeaders(tenant), "")
	require.Equal(t, http.StatusOK, status)
	stream.awaitHello(t)
	stream.expectNothing(t, 600*time.Millisecond)
}

// A malformed cursor must not lock a client out of its own stream.
func TestRealtimeStream_MalformedCursorIsTreatedAsNoCursor(t *testing.T) {
	e := newStreamTestEnv(t, apprealtime.HubOptions{})
	tenant := uuid.New()

	stream, status := e.openStream(t, map[string]string{
		"X-Test-Tenant": tenant.String(),
		"Last-Event-ID": "not-a-number",
	}, "")
	require.Equal(t, http.StatusOK, status)
	stream.awaitHello(t)

	wanted := e.publish(t, tenant, rt.RiskCreated, "risk-1")
	got := stream.next(t, 3*time.Second)
	assert.Equal(t, wanted.ID, got.Data["id"])
}

// ===========================================================================
// Duplicates, capacity, contract.
// ===========================================================================

// THE mandatory duplicate test: the same event presented three times reaches a
// client once, so a consumer's state cannot be triple-applied.
func TestRealtimeStream_ADuplicatedEventIsDeliveredOnce(t *testing.T) {
	e := newStreamTestEnv(t, apprealtime.HubOptions{})
	tenant := uuid.New()

	stream, status := e.openStream(t, tenantHeaders(tenant), "")
	require.Equal(t, http.StatusOK, status)
	stream.awaitHello(t)

	published := e.publish(t, tenant, rt.IncidentCreated, "incident-1")

	// The same envelope re-presented, as a cross-instance echo would be.
	e.hub.Dispatch(published)
	e.hub.Dispatch(published)

	got := stream.next(t, 3*time.Second)
	assert.Equal(t, published.ID, got.Data["id"])
	stream.expectNothing(t, 600*time.Millisecond)
}

func TestRealtimeStream_CapacityIsRefusedNotQueued(t *testing.T) {
	e := newStreamTestEnv(t, apprealtime.HubOptions{MaxConnections: 1, MaxPerTenant: 1})
	tenant := uuid.New()

	first, status := e.openStream(t, tenantHeaders(tenant), "")
	require.Equal(t, http.StatusOK, status)
	first.awaitHello(t)

	_, status = e.openStream(t, tenantHeaders(tenant), "")
	assert.Equal(t, http.StatusServiceUnavailable, status,
		"an instance at capacity must say so rather than accept a connection it cannot serve")
}

func TestRealtimeStream_SendsSseHeadersAndAHelloFrame(t *testing.T) {
	e := newStreamTestEnv(t, apprealtime.HubOptions{})
	tenant := uuid.New()

	stream, status := e.openStream(t, tenantHeaders(tenant), "")
	require.Equal(t, http.StatusOK, status)

	assert.Equal(t, "text/event-stream", stream.resp.Header.Get("Content-Type"))
	assert.Equal(t, "no-cache", stream.resp.Header.Get("Cache-Control"))
	assert.Equal(t, "no", stream.resp.Header.Get("X-Accel-Buffering"),
		"without this an nginx in front would buffer the stream into one lump")

	hello := stream.awaitHello(t)
	assert.NotEmpty(t, hello.Data["connection_id"])
	assert.EqualValues(t, rt.EnvelopeVersion, hello.Data["envelope_version"])
}

// A delivered event must satisfy the published contract, field for field.
func TestRealtimeStream_DeliveredEnvelopeMatchesTheContract(t *testing.T) {
	e := newStreamTestEnv(t, apprealtime.HubOptions{})
	tenant := uuid.New()

	stream, status := e.openStream(t, tenantHeaders(tenant), "")
	require.Equal(t, http.StatusOK, status)
	stream.awaitHello(t)

	e.publish(t, tenant, rt.ControlUpdated, "control-9")
	got := stream.next(t, 3*time.Second)

	for _, field := range []string{"id", "envelopeVersion", "type", "version", "occurredAt", "tenantId", "aggregate", "sequence"} {
		assert.Contains(t, got.Data, field, "the wire envelope lost its %q field", field)
	}
	agg, ok := got.Data["aggregate"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, rt.AggregateControl, agg["type"])
	assert.Equal(t, "control-9", agg["id"])
	// JSON numbers decode as float64, so the comparison is made on the integer
	// rendering. The property under test is that the resume cursor a browser
	// sends back and the ordering position in the envelope are one number, not
	// two that can disagree.
	seq, ok := got.Data["sequence"].(float64)
	require.True(t, ok, "sequence must be a number")
	assert.Equal(t, fmt.Sprintf("%.0f", seq), got.ID,
		"the SSE id and the envelope sequence must be the same number")
}

func TestRealtimeStream_CatalogDescribesTheContract(t *testing.T) {
	e := newStreamTestEnv(t, apprealtime.HubOptions{})
	tenant := uuid.New()

	req, err := http.NewRequest(http.MethodGet, e.baseURL+"/api/v1/realtime/catalog", nil)
	require.NoError(t, err)
	req.Header.Set("X-Test-Tenant", tenant.String())
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body struct {
		EnvelopeVersion int             `json:"envelope_version"`
		Transport       string          `json:"transport"`
		Events          []rt.Descriptor `json:"events"`
		Delivery        struct {
			Semantics string `json:"semantics"`
			DedupKey  string `json:"dedup_key"`
		} `json:"delivery"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

	assert.Equal(t, rt.EnvelopeVersion, body.EnvelopeVersion)
	assert.Equal(t, "sse", body.Transport)
	assert.Len(t, body.Events, len(rt.Catalog()))
	assert.Equal(t, "at-least-once", body.Delivery.Semantics,
		"the contract must not claim a guarantee the implementation does not provide")
	assert.Equal(t, "id", body.Delivery.DedupKey)
	for _, d := range body.Events {
		assert.NotEmpty(t, d.Permission, "%s is published without stating the permission it needs", d.Type)
	}
}
