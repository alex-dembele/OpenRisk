// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/opendefender/openrisk/internal/middleware"
	"github.com/opendefender/openrisk/pkg/cache"
)

// InvalidateCacheOnMutation drops the caller's cached responses after any write
// that succeeded (#337).
//
// Mounted once, on the authenticated group, rather than called from each
// handler. That is the whole point: the acceptance criterion is "resource
// mutations invalidate dependent responses", and a per-handler hook satisfies it
// exactly until somebody adds the next handler without one. A GRC product that
// shows an auditor a figure from before the last change has a worse problem than
// a cache that refills.
//
// The cost of the coarseness is one tenant re-computing at most five cached
// responses after each of its own writes. The cost of the precise version is a
// stale compliance number nobody can explain.
//
// Runs AFTER the handler and only on a 2xx: a rejected write changed nothing,
// and evicting on a 403 would let an unauthorised caller flush a tenant's cache
// at will.
func InvalidateCacheOnMutation(decoration *cache.CacheDecoration) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if err := c.Next(); err != nil {
			return err
		}

		switch c.Method() {
		case fiber.MethodPost, fiber.MethodPut, fiber.MethodPatch, fiber.MethodDelete:
		default:
			return nil
		}

		if status := c.Response().StatusCode(); status < 200 || status > 299 {
			return nil
		}

		ctx := middleware.GetContext(c)
		if ctx == nil || ctx.OrganizationID.String() == "" {
			return nil
		}

		// Best-effort, like the write itself: the mutation has already been
		// committed and acknowledged, so a failed eviction must not turn a
		// successful write into an error the client sees. The entry expires on
		// its TTL regardless.
		_ = decoration.InvalidateTenant(c.UserContext(), ctx.OrganizationID.String(), "mutation")
		return nil
	}
}
