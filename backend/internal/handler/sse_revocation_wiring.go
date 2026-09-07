// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"
)

// Revocation enforcement for LIVE server-sent-event streams (#345).
//
// Every SSE endpoint here authorizes once, in the middleware, and then holds the
// connection open — realtime for up to two hours. Authorization at connect time
// is a statement about the past: a session revoked a second later kept receiving
// tenant-scoped events until the stream ended on its own.
//
// The fix is deliberately small. Each stream already ticks a keepalive every 20
// seconds, because that write is also how the server learns the client has gone.
// That tick is where the session is now re-checked, which makes the worst-case
// exposure one keepalive interval — a bound the client is told about in the
// stream's hello payload rather than one it has to infer.
//
// Only the JTI blacklist is consulted, which is a single Redis EXISTS: that is
// the signal that means "this session was revoked", it costs nothing per tick,
// and it is the same check the request middleware already runs on every ordinary
// call. Account deactivation and membership removal are a different signal on a
// different clock; see the issue's remainders.

// SSERevocationChecker reports whether the session identified by jti has been
// revoked. It returns the blacklist's own answer and error, unchanged.
//
// Shape matches pkg/auth.TokenBlacklistManager.CheckJTIBlacklist so main.go can
// hand over the very closure the auth middleware uses; two different revocation
// predicates would be a bug waiting to happen.
type SSERevocationChecker func(jti string) (bool, error)

// sseRevocationChecker is nil until main.go wires it. nil is valid and means the
// streams behave exactly as before — an un-wired deployment must not panic.
var sseRevocationChecker SSERevocationChecker

// SetSSERevocationChecker injects the blacklist predicate from main.go. Call
// once at boot, before the server starts serving.
func SetSSERevocationChecker(f SSERevocationChecker) {
	sseRevocationChecker = f
}

// streamJTI reads the token id the auth middleware stored for this request.
//
// It must be read BEFORE the body-stream writer starts: the writer runs after
// the handler returns, and the Fiber context is recycled by then.
func streamJTI(c *fiber.Ctx) string {
	jti, _ := c.Locals("jti").(string)
	return jti
}

// sseSessionRevoked reports whether a live stream must be torn down now.
//
// False when no checker is wired or the stream carries no jti — a cookie session
// or a PAT reaches some of these endpoints without one, and refusing to serve
// those would be a regression, not a fix.
//
// It follows the blacklist's own fail-open posture: a Redis error answers "not
// revoked", exactly as pkg/auth.IsJTIBlacklisted does for every ordinary
// request. Diverging here would mean an outage silently closed every live stream
// while leaving every REST call authorized, which trades a availability
// incident for a security one. The consequence — an outage suspends revocation
// for the duration — is real and recorded on the issue rather than hidden here.
func sseSessionRevoked(jti string) bool {
	return sseRevokedWith(sseRevocationChecker, jti)
}

// sseRevokedWith is the same decision for a handler that already holds its own
// blacklist predicate — mitigation_events validates the token itself and has one
// injected. Both entry points go through this so the fail-open posture and the
// empty-jti rule cannot drift apart between endpoints.
func sseRevokedWith(check SSERevocationChecker, jti string) bool {
	if check == nil || jti == "" {
		return false
	}
	revoked, err := check(jti)
	if err != nil {
		return false
	}
	return revoked
}

// sseRevocationInterval is the documented upper bound on how long a revoked
// session can keep receiving events. It is the keepalive period because that is
// the tick the check rides on; naming it separately is what lets the value be
// stated in the stream contract instead of inferred from an implementation.
const sseRevocationInterval = 20 * time.Second
