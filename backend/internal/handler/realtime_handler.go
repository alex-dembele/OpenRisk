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
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	apprealtime "github.com/opendefender/openrisk/internal/application/realtime"
	"github.com/opendefender/openrisk/internal/domain"
	"github.com/opendefender/openrisk/internal/middleware"
	"github.com/opendefender/openrisk/pkg/monitoring"
	rt "github.com/opendefender/openrisk/pkg/realtime"
)

// ---------------------------------------------------------------------------
// GET /realtime/events — the authenticated, tenant-scoped SSE stream.
//
// SSE, not WebSocket. The decision is recorded in
// docs/W0-07_REALTIME_EVENT_HUB.md; the short version is that this stream is
// one-directional (every command in OpenRisk already travels as an HTTP
// request), SSE gives resumption from a cursor as a protocol feature rather than
// as something each application has to invent, and the codebase already runs
// three SSE endpoints and no WebSocket at all. A bidirectional transport would
// buy nothing here and would need its own auth, framing and heartbeat story.
//
// AUTHENTICATION. The route sits behind the ordinary session gate: the browser
// sends its HttpOnly cookie, other callers send a bearer token. There is
// deliberately NO `?token=` fallback, unlike the older mitigation stream — a
// credential in a URL lands in access logs, proxy logs, browser history and any
// Referer that leaks. Cookie sessions arrived with W0-03 and make that fallback
// unnecessary.
//
// TENANT. Taken from the resolved session context, never from a query
// parameter or a header. A client cannot name the tenant it wants.
// ---------------------------------------------------------------------------

// Stream tuning. Each of these is a bound, and each exists because the
// unbounded version is a resource the client controls.
const (
	// realtimeKeepaliveInterval is how often the server proves it is alive.
	// Below the 30–60 s idle timeout typical of proxies and load balancers, so
	// an idle stream is never cut by an intermediary that saw no bytes.
	realtimeKeepaliveInterval = 20 * time.Second

	// realtimeMaxLifetime closes a stream after this long. The client
	// reconnects with its cursor and replays, so the interruption is invisible;
	// what it buys is that a connection can never outlive a deploy, a rotated
	// session or a leaked goroutine indefinitely.
	realtimeMaxLifetime = 2 * time.Hour

	// realtimeReplayPageSize bounds one catch-up read.
	realtimeReplayPageSize = 200

	// realtimeMaxReplayPages bounds a whole catch-up. A client that missed more
	// than this many events is told to resync instead of being handed the day:
	// replaying 100k events into a browser is slower and less correct than
	// refetching the handful of screens it is showing.
	realtimeMaxReplayPages = 10
)

// SSE event names for the control frames the stream sends alongside domain
// events. They are namespaced so they can never collide with a catalog type,
// which is always `<aggregate>.<action>`.
const (
	sseHello     = "stream.hello"
	sseHeartbeat = "stream.heartbeat"
	sseResync    = "stream.resync"
	// sseRevoked is terminal, and deliberately not a resync: a resync tells the
	// client to reconnect with its cursor, which is precisely what a revoked
	// session must not be invited to do. The client is being told to
	// authenticate again, not to come back where it left off (#345).
	sseRevoked = "stream.revoked"
)

// RealtimeEventReader is the replay side of the durable log.
type RealtimeEventReader interface {
	Replay(ctx context.Context, tenantID uuid.UUID, after int64, limit int) ([]domain.RealtimeEvent, error)
	Bounds(ctx context.Context, tenantID uuid.UUID) (oldest, newest int64, err error)
}

// RealtimeHandler serves the event stream and its contract.
type RealtimeHandler struct {
	hub *apprealtime.Hub
	log RealtimeEventReader

	keepalive   time.Duration
	maxLifetime time.Duration
}

// RealtimeStreamOptions overrides the stream's timings. A zero field keeps the
// default.
//
// These are settings rather than constants because the keepalive is also how
// the server LEARNS a client has gone: the write that fails is the write that
// ends the goroutine. A deployment behind a proxy with a short idle timeout
// needs a shorter one, and a test that opens and closes streams should not have
// to wait a production interval to observe a teardown.
type RealtimeStreamOptions struct {
	KeepaliveInterval time.Duration
	MaxLifetime       time.Duration
}

// NewRealtimeHandler wires the stream handler with the default timings.
func NewRealtimeHandler(hub *apprealtime.Hub, eventLog RealtimeEventReader) *RealtimeHandler {
	return NewRealtimeHandlerWithOptions(hub, eventLog, RealtimeStreamOptions{})
}

// NewRealtimeHandlerWithOptions wires the stream handler with explicit timings.
func NewRealtimeHandlerWithOptions(hub *apprealtime.Hub, eventLog RealtimeEventReader, opts RealtimeStreamOptions) *RealtimeHandler {
	h := &RealtimeHandler{
		hub:         hub,
		log:         eventLog,
		keepalive:   realtimeKeepaliveInterval,
		maxLifetime: realtimeMaxLifetime,
	}
	if opts.KeepaliveInterval > 0 {
		h.keepalive = opts.KeepaliveInterval
	}
	if opts.MaxLifetime > 0 {
		h.maxLifetime = opts.MaxLifetime
	}
	return h
}

// Catalog is GET /realtime/catalog — the machine-readable stream contract.
//
// It exists so a consumer can discover which events it may subscribe to, at
// which version, instead of reading a document that has drifted. It is the same
// catalog the publisher validates against, so it cannot describe an event that
// cannot be published.
func (h *RealtimeHandler) Catalog(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"envelope_version": rt.EnvelopeVersion,
		"transport":        "sse",
		"endpoint":         "/api/v1/realtime/events",
		"events":           rt.Catalog(),
		"aggregates":       rt.Aggregates(),
		"limits": fiber.Map{
			"max_payload_bytes":      rt.MaxPayloadBytes,
			"max_filter_entries":     rt.MaxFilterEntries,
			"replay_retention_hours": int(domain.DefaultReplayRetention.Hours()),
			"replay_page_size":       realtimeReplayPageSize,
			"max_replay_pages":       realtimeMaxReplayPages,
			"keepalive_seconds":      int(realtimeKeepaliveInterval.Seconds()),
			"max_lifetime_seconds":   int(realtimeMaxLifetime.Seconds()),
		},
		"delivery": fiber.Map{
			// Stated plainly because the alternative — claiming exactly-once —
			// would be false, and a consumer that believes it stops writing the
			// idempotency it actually needs.
			"semantics": "at-least-once",
			"ordering":  "total per tenant in the durable log (sequence); live delivery is ordered per publishing instance and clients order by sequence",
			"dedup_key": "id",
			"cursor":    "Last-Event-ID (the sequence of the last event received)",
		},
	})
}

// Stats is GET /realtime/stats — what this instance is currently serving.
// Admin-only at the route; it exposes no tenant's data, only counts.
func (h *RealtimeHandler) Stats(c *fiber.Ctx) error {
	s := h.hub.Stats()
	return c.JSON(fiber.Map{
		"connections":        s.Connections,
		"tenants":            s.Tenants,
		"buffered_events":    s.Buffered,
		"tenant_connections": h.hub.SubscriberCount(tenantID(c)),
	})
}

// Stream is GET /realtime/events.
func (h *RealtimeHandler) Stream(c *fiber.Ctx) error {
	tenant := tenantID(c)
	if tenant == uuid.Nil {
		// Fail closed. Without a tenant there is no stream that could be safely
		// opened — certainly not one carrying "everything".
		monitoring.RealtimeSubscriptionsTotal.WithLabelValues("unauthenticated").Inc()
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "an authenticated session scoped to an organization is required",
		})
	}

	requested, err := rt.ParseFilter(c.Query("types"), c.Query("aggregates"))
	if err != nil {
		monitoring.RealtimeSubscriptionsTotal.WithLabelValues("invalid").Inc()
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	// Authorization, then isolation, then the client's own narrowing — in that
	// order. The permitted set is computed from the session's permissions using
	// the SAME wildcard semantics the route guards use, and the client's request
	// can only subtract from it. A subscription that could never deliver
	// anything is refused rather than left open as a stream that mysteriously
	// stays silent.
	claims := middleware.GetUserClaims(c)
	if claims == nil {
		monitoring.RealtimeSubscriptionsTotal.WithLabelValues("unauthenticated").Inc()
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "an authenticated session is required"})
	}
	allowed := rt.AllowedAggregates(claims.HasPermission)
	filter, permitted := rt.Restrict(requested, allowed)
	if !permitted {
		monitoring.RealtimeSubscriptionsTotal.WithLabelValues("forbidden").Inc()
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error":              "no event category in this subscription is readable with your permissions",
			"allowed_aggregates": allowed,
		})
	}

	cursor := parseCursor(c)

	sub, err := h.hub.Subscribe(tenant, filter)
	if err != nil {
		monitoring.RealtimeSubscriptionsTotal.WithLabelValues("capacity").Inc()
		// 503 with Retry-After rather than 429: the client did nothing wrong,
		// the instance is full, and the honest instruction is "come back".
		c.Set("Retry-After", "30")
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "the realtime stream is at capacity on this instance; retry shortly",
		})
	}
	monitoring.RealtimeSubscriptionsTotal.WithLabelValues("accepted").Inc()

	connID := sub.ID
	userID := userID(c)
	// Read off the request while it still exists: the body writer below runs
	// after this handler returns, and the Fiber context is recycled by then.
	jti := streamJTI(c)
	log.Printf("realtime: stream open conn=%s tenant=%s user=%s filter=%s cursor=%d",
		connID, tenant, userID, filter.Describe(), cursor)

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	// Nginx buffers proxied responses by default, which turns a live stream into
	// a stream that arrives in one lump when the connection ends.
	c.Set("X-Accel-Buffering", "no")

	// The replay read needs a context that outlives the request handler, because
	// the body writer runs after Stream returns.
	streamCtx := context.WithoutCancel(c.UserContext())

	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		openedAt := time.Now()
		defer func() {
			h.hub.Unsubscribe(sub)
			monitoring.RealtimeConnectionDurationSeconds.Observe(time.Since(openedAt).Seconds())
			log.Printf("realtime: stream closed conn=%s tenant=%s dropped=%d duration=%s",
				connID, tenant, sub.Dropped(), time.Since(openedAt).Round(time.Second))
		}()

		st := &streamWriter{w: w}

		// hello names the tenant and the connection so a client can tell that
		// the identity behind its stream changed — which is what a cursor from
		// another tenant would mean — and reset rather than replay into the
		// wrong context.
		if !st.control(sseHello, fiber.Map{
			"connection_id":      connID,
			"tenant_id":          tenant.String(),
			"envelope_version":   rt.EnvelopeVersion,
			"filter":             filter.Describe(),
			"allowed_aggregates": allowed,
			"server_time":        time.Now().UTC().Format(time.RFC3339),
			// The documented bound from #345: a revoked session stops receiving
			// events within this many seconds. Stated rather than implied, so a
			// client — or an auditor — reads the guarantee instead of inferring
			// it from the keepalive.
			"revocation_check_seconds": int(sseRevocationInterval.Seconds()),
		}) {
			return
		}

		// Catch up before going live. The subscription was registered BEFORE
		// this read, so an event published during the replay is buffered rather
		// than lost — the gap between "read history" and "start listening" is
		// exactly where a naive implementation drops events.
		replayed := h.catchUp(streamCtx, st, tenant, cursor, filter)
		if st.failed {
			return
		}

		keepalive := time.NewTicker(h.keepalive)
		defer keepalive.Stop()
		lifetime := time.After(h.maxLifetime)

		for {
			select {
			case <-lifetime:
				// Say why, so a client can distinguish a routine recycle from a
				// failure and reconnect immediately instead of backing off.
				st.control(sseResync, fiber.Map{
					"reason": "stream_recycled",
					"detail": "the connection reached its maximum lifetime; reconnect with your cursor",
				})
				return

			case <-sub.Resync():
				monitoring.RealtimeResyncsTotal.WithLabelValues("buffer_overflow").Inc()
				if !st.control(sseResync, fiber.Map{
					"reason": "buffer_overflow",
					"detail": "events were dropped because this client fell behind; refetch the data you display",
					"lost":   sub.Dropped(),
				}) {
					return
				}

			case env, ok := <-sub.Events():
				if !ok {
					return
				}
				// The catch-up window can hand the same event to both paths.
				// Suppressing it here keeps the promise the client is entitled
				// to: it deduplicates too, but it should not have to.
				if replayed.contains(env.ID) {
					continue
				}
				if !st.event(env) {
					return
				}
				monitoring.RealtimeEventsDeliveredTotal.WithLabelValues(env.Aggregate.Type, "live").Inc()
				monitoring.RealtimeDeliveryLagSeconds.Observe(time.Since(env.OccurredAt).Seconds())

			case <-keepalive.C:
				// Re-authorize before writing anything else. This tick is the
				// only regular event on an otherwise idle stream, so it is what
				// bounds how long a revoked session keeps receiving data — one
				// keepalive interval, and no longer (#345).
				//
				// Checked BEFORE the keepalive write, so a revoked session is
				// never told the stream is healthy.
				if sseSessionRevoked(jti) {
					monitoring.RealtimeResyncsTotal.WithLabelValues("session_revoked").Inc()
					log.Printf("realtime: stream revoked conn=%s tenant=%s user=%s", connID, tenant, userID)
					st.control(sseRevoked, fiber.Map{
						"reason": "session_revoked",
						"detail": "this session was revoked; authenticate again before reconnecting",
					})
					return
				}

				// Two forms on purpose: the comment keeps intermediaries from
				// timing the connection out, and the named event is what lets
				// the CLIENT arm a watchdog — EventSource never surfaces
				// comments to application code, so a comment alone cannot prove
				// liveness to the browser.
				if !st.comment("keepalive") {
					return
				}
				if !st.control(sseHeartbeat, fiber.Map{
					"server_time": time.Now().UTC().Format(time.RFC3339),
					"dropped":     sub.Dropped(),
				}) {
					return
				}
			}
		}
	})
	return nil
}

// catchUp replays what the client missed, returning the ids it sent so the live
// loop can suppress the overlap.
func (h *RealtimeHandler) catchUp(ctx context.Context, st *streamWriter, tenant uuid.UUID, cursor int64, filter rt.Filter) *idSet {
	sent := newIDSet()
	if h.log == nil {
		return sent
	}
	if cursor <= 0 {
		// A first connection is not a gap: the client is about to load its data
		// from the API anyway, and replaying a day of history into a fresh page
		// would be work nobody asked for.
		return sent
	}

	oldest, newest, err := h.log.Bounds(ctx, tenant)
	if err != nil {
		monitoring.RealtimeReplaysTotal.WithLabelValues("error").Inc()
		log.Printf("realtime: replay bounds failed tenant=%s: %v", tenant, err)
		st.control(sseResync, fiber.Map{
			"reason": "replay_unavailable",
			"detail": "the event history could not be read; refetch the data you display",
		})
		return sent
	}
	if newest == 0 || cursor >= newest {
		monitoring.RealtimeReplaysTotal.WithLabelValues("empty").Inc()
		return sent
	}
	// The cursor names an event that has already aged out of the window, so the
	// gap between it and what survives cannot be filled. Saying so is the whole
	// point: a silent partial replay would leave the client believing it is
	// current.
	if oldest > cursor+1 {
		monitoring.RealtimeReplaysTotal.WithLabelValues("expired").Inc()
		monitoring.RealtimeResyncsTotal.WithLabelValues("cursor_expired").Inc()
		st.control(sseResync, fiber.Map{
			"reason":          "cursor_expired",
			"detail":          "your cursor is older than the retained history; refetch the data you display",
			"cursor":          cursor,
			"oldest_retained": oldest,
		})
		return sent
	}

	from := cursor
	for page := 0; page < realtimeMaxReplayPages; page++ {
		rows, err := h.log.Replay(ctx, tenant, from, realtimeReplayPageSize)
		if err != nil {
			monitoring.RealtimeReplaysTotal.WithLabelValues("error").Inc()
			log.Printf("realtime: replay failed tenant=%s from=%d: %v", tenant, from, err)
			st.control(sseResync, fiber.Map{
				"reason": "replay_failed",
				"detail": "the event history could not be read; refetch the data you display",
			})
			return sent
		}
		if len(rows) == 0 {
			monitoring.RealtimeReplaysTotal.WithLabelValues("served").Inc()
			return sent
		}
		for i := range rows {
			env := rows[i].ToEnvelope()
			// The cursor advances past events the filter withholds. Advancing
			// only on delivered events would loop forever on a page of
			// filtered-out history; and applying the SAME filter the live path
			// applies is what stops a reconnect from becoming a way to receive
			// what the live stream refuses.
			from = env.Sequence
			if !filter.Match(env) {
				continue
			}
			if !st.event(env) {
				return sent
			}
			sent.add(env.ID)
			monitoring.RealtimeEventsDeliveredTotal.WithLabelValues(env.Aggregate.Type, "replay").Inc()
		}
		if len(rows) < realtimeReplayPageSize {
			monitoring.RealtimeReplaysTotal.WithLabelValues("served").Inc()
			return sent
		}
	}

	// More history than one connection should carry. Refetching beats a replay
	// that would take longer than the refetch.
	monitoring.RealtimeReplaysTotal.WithLabelValues("expired").Inc()
	monitoring.RealtimeResyncsTotal.WithLabelValues("cursor_expired").Inc()
	st.control(sseResync, fiber.Map{
		"reason": "replay_too_large",
		"detail": "too many events were missed to replay; refetch the data you display",
		"cursor": from,
	})
	return sent
}

// parseCursor reads the resume position.
//
// Last-Event-ID is the SSE protocol's own mechanism and is what a browser sends
// automatically on reconnect. The query parameter exists for non-browser clients
// that cannot set the header on a GET, and for tests.
func parseCursor(c *fiber.Ctx) int64 {
	raw := strings.TrimSpace(c.Get("Last-Event-ID"))
	if raw == "" {
		raw = strings.TrimSpace(c.Query("last_event_id"))
	}
	if raw == "" {
		return 0
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 0 {
		// A malformed cursor is treated as no cursor: refusing the connection
		// would lock a client out of its own stream over a value it may have
		// inherited from an older version of the client code.
		return 0
	}
	return n
}

// streamWriter serialises frames onto the SSE connection and remembers the
// first write failure, so every call site can simply stop.
type streamWriter struct {
	w      *bufio.Writer
	failed bool
}

func (s *streamWriter) comment(text string) bool {
	if s.failed {
		return false
	}
	if _, err := fmt.Fprintf(s.w, ": %s\n\n", text); err != nil {
		s.failed = true
		return false
	}
	return s.flush()
}

// event writes a domain event. The SSE id is the per-tenant sequence, which is
// what the browser sends back as Last-Event-ID — so the resume cursor and the
// ordering guarantee are the same number rather than two things that can
// disagree.
func (s *streamWriter) event(env rt.Envelope) bool {
	if s.failed {
		return false
	}
	raw, err := json.Marshal(env)
	if err != nil {
		// Refusing to write it is right: a half-serialised event would look
		// like data. The publisher validated this shape, so reaching here means
		// a defect worth a log line.
		log.Printf("realtime: dropping unserialisable event %s: %v", env.ID, err)
		return true
	}
	if _, err := fmt.Fprintf(s.w, "id: %d\nevent: %s\ndata: %s\n\n", env.Sequence, env.Type, raw); err != nil {
		s.failed = true
		return false
	}
	return s.flush()
}

// control writes a stream-lifecycle frame. Control frames deliberately carry NO
// `id:`: they are not positions in the tenant's event order, and letting one
// become a client's cursor would move the cursor to a number no event has.
func (s *streamWriter) control(name string, body fiber.Map) bool {
	if s.failed {
		return false
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return true
	}
	if _, err := fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", name, raw); err != nil {
		s.failed = true
		return false
	}
	return s.flush()
}

func (s *streamWriter) flush() bool {
	if err := s.w.Flush(); err != nil {
		// The client is gone. This is the normal way a stream ends.
		s.failed = true
		return false
	}
	return true
}

// idSet is a small bounded set of the event ids sent during catch-up.
type idSet struct {
	ids map[string]struct{}
}

func newIDSet() *idSet { return &idSet{ids: map[string]struct{}{}} }

func (s *idSet) add(id string) {
	if len(s.ids) >= realtimeReplayPageSize*realtimeMaxReplayPages {
		return
	}
	s.ids[id] = struct{}{}
}

func (s *idSet) contains(id string) bool {
	_, ok := s.ids[id]
	return ok
}
