// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package handler

import (
	"context"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/opendefender/openrisk/internal/middleware"
	"github.com/opendefender/openrisk/pkg/cache"
)

// #337 — a write must drop the writer's cached responses, and only theirs.
//
// The middleware is mounted once on the authenticated group rather than called
// from each handler, so these tests assert the behaviour that arrangement is
// supposed to buy: it fires for any mutation, on any route, without the handler
// knowing it exists.

// patternStore records the DeletePattern calls so a test can assert WHICH keys
// an invalidation targeted — the difference between evicting one tenant and
// flushing the estate.
type patternStore struct {
	mu       sync.Mutex
	patterns []string
}

func (p *patternStore) Get(context.Context, string, interface{}) error { return cache.ErrCacheMiss }
func (p *patternStore) SetWithTTL(context.Context, string, interface{}, time.Duration) error {
	return nil
}

func (p *patternStore) DeletePattern(_ context.Context, pattern string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.patterns = append(p.patterns, pattern)
	return nil
}

func (p *patternStore) seen() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.patterns...)
}

func newInvalidationApp(t *testing.T, tenant uuid.UUID, store cache.ResponseStore) *fiber.App {
	t.Helper()

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		if c.Get("X-Anon") == "" {
			middleware.SetContext(c, &middleware.RequestContext{
				UserID: uuid.New(), OrganizationID: tenant,
			})
		}
		return c.Next()
	})
	app.Use(InvalidateCacheOnMutation(cache.NewCacheDecorationWithStore(store)))

	app.Get("/risks", func(c *fiber.Ctx) error { return c.JSON(fiber.Map{"ok": true}) })
	app.Post("/risks", func(c *fiber.Ctx) error { return c.Status(201).JSON(fiber.Map{"ok": true}) })
	app.Patch("/risks/:id", func(c *fiber.Ctx) error { return c.JSON(fiber.Map{"ok": true}) })
	app.Delete("/risks/:id", func(c *fiber.Ctx) error { return c.SendStatus(204) })
	app.Post("/refused", func(c *fiber.Ctx) error { return c.Status(403).JSON(fiber.Map{"error": "no"}) })
	app.Post("/broken", func(c *fiber.Ctx) error { return c.Status(500).JSON(fiber.Map{"error": "boom"}) })
	return app
}

func do(t *testing.T, app *fiber.App, method, path string, headers map[string]string) int {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode
}

func TestInvalidateCacheOnMutation_EverySuccessfulWriteEvicts(t *testing.T) {
	tenant := uuid.New()

	for _, tc := range []struct{ method, path string }{
		{fiber.MethodPost, "/risks"},
		{fiber.MethodPatch, "/risks/" + uuid.NewString()},
		{fiber.MethodDelete, "/risks/" + uuid.NewString()},
	} {
		t.Run(tc.method, func(t *testing.T) {
			store := &patternStore{}
			app := newInvalidationApp(t, tenant, store)

			require.Less(t, do(t, app, tc.method, tc.path, nil), 300)

			seen := store.seen()
			require.Len(t, seen, 1, "a successful write must evict exactly once")
			assert.Contains(t, seen[0], tenant.String(),
				"the eviction must name the writer's tenant, not every tenant")
			assert.Contains(t, seen[0], cache.ResponseKeyPrefix,
				"it must target the response namespace WrapWithCache actually writes")
		})
	}
}

// A read changes nothing, so evicting on one would keep the cache permanently
// cold — the failure mode where a cache exists and never serves.
func TestInvalidateCacheOnMutation_ReadsDoNotEvict(t *testing.T) {
	store := &patternStore{}
	app := newInvalidationApp(t, uuid.New(), store)

	require.Equal(t, fiber.StatusOK, do(t, app, fiber.MethodGet, "/risks", nil))

	assert.Empty(t, store.seen(), "a GET must never invalidate")
}

// A rejected write changed nothing. Evicting on a 403 would also hand any
// authenticated caller a way to flush a tenant's cache on demand.
func TestInvalidateCacheOnMutation_RefusedAndFailedWritesDoNotEvict(t *testing.T) {
	for _, path := range []string{"/refused", "/broken"} {
		t.Run(path, func(t *testing.T) {
			store := &patternStore{}
			app := newInvalidationApp(t, uuid.New(), store)

			status := do(t, app, fiber.MethodPost, path, nil)
			require.GreaterOrEqual(t, status, 400)

			assert.Empty(t, store.seen(),
				"a write that did not happen must not evict — nor give a caller a way to flush the cache")
		})
	}
}

// One tenant's write must not cool another's cache.
func TestInvalidateCacheOnMutation_EvictionIsScopedToTheWritersTenant(t *testing.T) {
	writer, bystander := uuid.New(), uuid.New()
	store := &patternStore{}
	app := newInvalidationApp(t, writer, store)

	require.Equal(t, fiber.StatusCreated, do(t, app, fiber.MethodPost, "/risks", nil))

	seen := store.seen()
	require.Len(t, seen, 1)
	assert.Contains(t, seen[0], writer.String())
	assert.NotContains(t, seen[0], bystander.String(),
		"an eviction pattern must not reach another organization's entries")
}

// Without a resolved tenant there is nothing to scope an eviction to, and a
// pattern that matched everything would flush the estate.
func TestInvalidateCacheOnMutation_NoTenantEvictsNothing(t *testing.T) {
	store := &patternStore{}
	app := newInvalidationApp(t, uuid.New(), store)

	do(t, app, fiber.MethodPost, "/risks", map[string]string{"X-Anon": "1"})

	assert.Empty(t, store.seen(), "an unscoped eviction is worse than none")
}

// A deployment without Redis must be unaffected: nil decoration, no panic.
func TestInvalidateCacheOnMutation_NilDecorationIsSafe(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		middleware.SetContext(c, &middleware.RequestContext{UserID: uuid.New(), OrganizationID: uuid.New()})
		return c.Next()
	})
	app.Use(InvalidateCacheOnMutation(nil))
	app.Post("/risks", func(c *fiber.Ctx) error { return c.Status(201).JSON(fiber.Map{"ok": true}) })

	assert.Equal(t, fiber.StatusCreated, do(t, app, fiber.MethodPost, "/risks", nil))
}
