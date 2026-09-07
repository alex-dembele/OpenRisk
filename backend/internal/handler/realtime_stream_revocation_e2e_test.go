// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package handler

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apprealtime "github.com/opendefender/openrisk/internal/application/realtime"
	rt "github.com/opendefender/openrisk/pkg/realtime"
)

// #345 — revocation on a LIVE stream, over a real socket.
//
// The stream authorized once, in the middleware, and then held the connection
// open: /realtime/events for up to two hours. A session revoked a second after
// connecting kept receiving tenant-scoped events for the rest of that window.
//
// These tests revoke a session while the stream is open and assert the three
// things the issue asks for: the events stop within a bounded interval, the
// client is told to authenticate rather than to resume, and nothing further is
// delivered.
//
// The harness keepalive is 150ms (see newStreamTestEnv), so "one interval" is
// observable in a test rather than in twenty seconds.

// revocationSpy is a controllable blacklist. It starts permissive so a stream
// can be established legitimately before anything is revoked — the point is to
// revoke a session that was valid, not to refuse one that never was.
type revocationSpy struct {
	mu      sync.Mutex
	revoked map[string]bool
	err     error
	// asked counts lookups PER jti rather than in total. Streams opened by
	// earlier tests in this package outlive them and keep ticking against
	// whatever checker is installed, so a global counter would report their
	// traffic as this test's.
	asked map[string]int
}

func newRevocationSpy() *revocationSpy {
	return &revocationSpy{revoked: map[string]bool{}, asked: map[string]int{}}
}

func (s *revocationSpy) check(jti string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.asked[jti]++
	if s.err != nil {
		return false, s.err
	}
	return s.revoked[jti], nil
}

func (s *revocationSpy) revoke(jti string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revoked[jti] = true
}

func (s *revocationSpy) failWith(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.err = err
}

// askedAbout reports how many times this exact session was looked up.
func (s *revocationSpy) askedAbout(jti string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.asked[jti]
}

// installRevocationSpy wires the package seam and restores it afterwards. The
// checker is process-global, so leaving it set would leak into every other test
// in this package.
func installRevocationSpy(t *testing.T) *revocationSpy {
	t.Helper()
	spy := newRevocationSpy()
	previous := sseRevocationChecker
	SetSSERevocationChecker(spy.check)
	t.Cleanup(func() { sseRevocationChecker = previous })
	return spy
}

func revocationHeaders(tenant uuid.UUID, jti string) map[string]string {
	h := tenantHeaders(tenant)
	h["X-Test-JTI"] = jti
	return h
}

// waitForClose drains frames until the stream ends, returning any terminal
// control frame it saw on the way out.
func waitForClose(t *testing.T, c *sseClient, within time.Duration) (sseFrame, bool) {
	t.Helper()
	deadline := time.After(within)
	var terminal sseFrame
	var seen bool
	for {
		select {
		case f, ok := <-c.frames:
			if !ok {
				return terminal, seen
			}
			if f.Event == "stream.revoked" {
				terminal, seen = f, true
			}
		case <-deadline:
			t.Fatal("the stream was still open after a revoked session should have been torn down")
		}
	}
}

// THE test: a live, legitimate stream is cut off after its session is revoked.
func TestRealtimeStream_RevokedSessionIsTornDown(t *testing.T) {
	spy := installRevocationSpy(t)
	env := newStreamTestEnv(t, apprealtime.HubOptions{})
	tenant := uuid.New()
	jti := "session-" + uuid.New().String()

	cl, status := env.openStream(t, revocationHeaders(tenant, jti), "")
	require.Equal(t, 200, status)
	require.NotNil(t, cl)
	cl.awaitHello(t)

	// The session was valid when it connected, and events flow.
	env.publish(t, tenant, rt.EventType("risk.created"), uuid.NewString())
	first := cl.next(t, 3*time.Second)
	require.Equal(t, "risk.created", first.Event)

	// Now revoke it, mid-stream.
	spy.revoke(jti)

	terminal, seen := waitForClose(t, cl, 3*time.Second)
	require.True(t, seen, "the client must be told why the stream ended")
	assert.Equal(t, "stream.revoked", terminal.Event)
	assert.Equal(t, "session_revoked", terminal.Data["reason"],
		"a revoked session is told to authenticate, not to resync with its cursor")
}

// The guarantee the issue asks to be documented: bounded by one keepalive
// interval, not by the stream's lifetime.
func TestRealtimeStream_RevocationIsBoundedByTheKeepaliveInterval(t *testing.T) {
	spy := installRevocationSpy(t)
	env := newStreamTestEnv(t, apprealtime.HubOptions{})
	tenant := uuid.New()
	jti := "session-" + uuid.New().String()

	cl, status := env.openStream(t, revocationHeaders(tenant, jti), "")
	require.Equal(t, 200, status)
	cl.awaitHello(t)

	spy.revoke(jti)
	start := time.Now()
	_, seen := waitForClose(t, cl, 3*time.Second)
	elapsed := time.Since(start)

	require.True(t, seen)
	// The harness ticks every 150ms. A generous ceiling still proves the bound
	// is the TICK and not the 30s MaxLifetime this env is configured with.
	assert.Less(t, elapsed, 2*time.Second,
		"revocation must take effect on the keepalive tick, not at the stream's lifetime")
}

// No protected data may follow the revocation.
func TestRealtimeStream_NoEventsAfterRevocation(t *testing.T) {
	spy := installRevocationSpy(t)
	env := newStreamTestEnv(t, apprealtime.HubOptions{})
	tenant := uuid.New()
	jti := "session-" + uuid.New().String()

	cl, status := env.openStream(t, revocationHeaders(tenant, jti), "")
	require.Equal(t, 200, status)
	cl.awaitHello(t)

	spy.revoke(jti)
	_, seen := waitForClose(t, cl, 3*time.Second)
	require.True(t, seen)

	// Publishing after the teardown must reach nobody on this connection. The
	// channel is closed, so any further frame would be a delivery to a revoked
	// session.
	env.publish(t, tenant, rt.EventType("risk.created"), uuid.NewString())
	cl.expectNothing(t, 500*time.Millisecond)
}

// The control. Without it, every test above would pass against a stream that
// simply closed itself.
func TestRealtimeStream_LiveSessionKeepsStreaming(t *testing.T) {
	spy := installRevocationSpy(t)
	env := newStreamTestEnv(t, apprealtime.HubOptions{})
	tenant := uuid.New()
	jti := "session-" + uuid.New().String()

	cl, status := env.openStream(t, revocationHeaders(tenant, jti), "")
	require.Equal(t, 200, status)
	cl.awaitHello(t)

	// Long enough for several keepalive ticks to have run the check.
	time.Sleep(600 * time.Millisecond)
	require.Greater(t, spy.askedAbout(jti), 1, "the check must actually run on the tick")

	env.publish(t, tenant, rt.EventType("risk.created"), uuid.NewString())
	f := cl.next(t, 3*time.Second)
	assert.Equal(t, "risk.created", f.Event, "a live session is unaffected")
}

// Fail-open, deliberately and identically to every ordinary request: a
// blacklist outage must not close every live stream in the estate. The cost —
// revocation is suspended while Redis is down — is recorded on the issue.
func TestRealtimeStream_BlacklistOutageDoesNotCloseTheStream(t *testing.T) {
	spy := installRevocationSpy(t)
	env := newStreamTestEnv(t, apprealtime.HubOptions{})
	tenant := uuid.New()
	jti := "session-" + uuid.New().String()

	cl, status := env.openStream(t, revocationHeaders(tenant, jti), "")
	require.Equal(t, 200, status)
	cl.awaitHello(t)

	spy.failWith(errors.New("redis unreachable"))
	time.Sleep(600 * time.Millisecond)

	env.publish(t, tenant, rt.EventType("risk.created"), uuid.NewString())
	f := cl.next(t, 3*time.Second)
	assert.Equal(t, "risk.created", f.Event,
		"an unreachable blacklist must not tear down live streams")
}

// A connection with no session id — a PAT or an agent token — is not something
// this check can revoke, and must not be refused because of that.
func TestRealtimeStream_NoJTIIsNotTreatedAsRevoked(t *testing.T) {
	spy := installRevocationSpy(t)
	env := newStreamTestEnv(t, apprealtime.HubOptions{})
	tenant := uuid.New()

	// tenantHeaders sets no X-Test-JTI.
	cl, status := env.openStream(t, tenantHeaders(tenant), "")
	require.Equal(t, 200, status)
	cl.awaitHello(t)

	time.Sleep(400 * time.Millisecond)

	env.publish(t, tenant, rt.EventType("risk.created"), uuid.NewString())
	f := cl.next(t, 3*time.Second)
	assert.Equal(t, "risk.created", f.Event)
	assert.Equal(t, 0, spy.askedAbout(""), "no session id means there is nothing to look up")
}

// The hello frame carries the bound, so the guarantee is published rather than
// inferred from the implementation.
func TestRealtimeStream_HelloDocumentsTheRevocationBound(t *testing.T) {
	installRevocationSpy(t)
	env := newStreamTestEnv(t, apprealtime.HubOptions{})
	tenant := uuid.New()

	cl, status := env.openStream(t, revocationHeaders(tenant, "session-x"), "")
	require.Equal(t, 200, status)

	hello := cl.awaitHello(t)
	require.Contains(t, hello.Data, "revocation_check_seconds")
	assert.EqualValues(t, sseRevocationInterval.Seconds(), hello.Data["revocation_check_seconds"])
}
