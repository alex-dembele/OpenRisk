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
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/opendefender/openrisk/internal/domain"
	redisinfra "github.com/opendefender/openrisk/internal/infrastructure/redis"
	authpkg "github.com/opendefender/openrisk/pkg/auth"
)

// MitigationEventsHandler streams mitigation.auto_completed events to the browser
// over SSE so a scanner-driven completion updates the mitigation map in real time
// (Auto-detected badge). Native EventSource can't send an Authorization header, so
// the access token is passed as ?token= and validated here; events are filtered to
// the caller's tenant.
type MitigationEventsHandler struct {
	redis     *redisinfra.Client
	rsaKeys   *authpkg.RSAKeys
	blacklist func(jti string) (bool, error)
}

// NewMitigationEventsHandler wires the SSE handler.
func NewMitigationEventsHandler(redis *redisinfra.Client, rsaKeys *authpkg.RSAKeys, blacklist func(jti string) (bool, error)) *MitigationEventsHandler {
	return &MitigationEventsHandler{redis: redis, rsaKeys: rsaKeys, blacklist: blacklist}
}

// Stream is GET /mitigations/events?token=<jwt>.
func (h *MitigationEventsHandler) Stream(c *fiber.Ctx) error {
	claims, err := authpkg.ValidateAccessToken(h.rsaKeys, c.Query("token"), h.blacklist)
	if err != nil || claims.TenantID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid or missing token"})
	}
	tenantID := claims.TenantID

	// DEPRECATED (#347). Superseded by the shared realtime stream, which carries
	// the session in an HttpOnly cookie instead of accepting the access token as
	// a query parameter the way this endpoint does.
	markSSEDeprecated(c, "/api/v1/realtime/events?aggregates=mitigation")

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no")

	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pubsub := h.redis.Subscribe(ctx, "mitigation.auto_completed")
		defer pubsub.Close()
		msgs := pubsub.Channel()

		fmt.Fprint(w, ": connected\n\n")
		if err := w.Flush(); err != nil {
			return
		}

		keepalive := time.NewTicker(20 * time.Second)
		defer keepalive.Stop()

		for {
			select {
			case msg, ok := <-msgs:
				if !ok {
					return
				}
				// Only forward events for this tenant.
				var evt domain.MitigationAutoCompleted
				if err := json.Unmarshal([]byte(msg.Payload), &evt); err != nil || evt.TenantID != tenantID {
					continue
				}
				fmt.Fprintf(w, "event: mitigation.auto_completed\ndata: %s\n\n", msg.Payload)
				if err := w.Flush(); err != nil {
					return
				}
			case <-keepalive.C:
				// Re-authorize on the tick: the token was validated once, when
				// the stream opened, and this connection can outlive its
				// revocation by hours otherwise (#345). h.blacklist is the same
				// predicate ValidateAccessToken used above.
				if sseRevokedWith(h.blacklist, claims.JTI) {
					fmt.Fprint(w, "event: stream.revoked\ndata: {\"reason\":\"session_revoked\"}\n\n")
					_ = w.Flush()
					return
				}
				fmt.Fprint(w, ": keepalive\n\n")
				if err := w.Flush(); err != nil {
					return
				}
			}
		}
	})
	return nil
}
