// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// #337 — WrapWithCache was a passthrough. It cached nothing, which is why the
// cross-tenant tests passed: there was no cache to collide in.
//
// These drive the real wrapper. The critical one is
// TestWrapWithCache_TenantBCannotBeServedTenantAsResponse: it is the shape the
// issue draws, and it is the only test here whose failure is a disclosure rather
// than a slowdown.
// ---------------------------------------------------------------------------

// memStore is an in-memory ResponseStore. Redis is not needed to decide the
// question these tests ask — whether two callers can land on one key — and
// adding a Redis double would be a new dependency.
type memStore struct {
	mu      sync.Mutex
	data    map[string][]byte
	failGet error
	failSet error
	sets    int32
}

func newMemStore() *memStore { return &memStore{data: map[string][]byte{}} }

func (m *memStore) Get(_ context.Context, key string, dest interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failGet != nil {
		return m.failGet
	}
	raw, ok := m.data[key]
	if !ok {
		return ErrCacheMiss
	}
	return json.Unmarshal(raw, dest)
}

func (m *memStore) SetWithTTL(_ context.Context, key string, value interface{}, _ time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failSet != nil {
		return m.failSet
	}
	atomic.AddInt32(&m.sets, 1)
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	m.data[key] = raw
	return nil
}

func (m *memStore) DeletePattern(_ context.Context, _ string) error { return nil }

func (m *memStore) keyCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.data)
}

// putRaw stores a hand-made entry, for the stale-entry case.
func (m *memStore) putRaw(key string, raw []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = raw
}

func (m *memStore) onlyKey(t *testing.T) string {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	require.Len(t, m.data, 1)
	for k := range m.data {
		return k
	}
	return ""
}

// tenantKey is a realistic builder: scoped, and refusing when the tenant is
// unresolved — the contract KeyFunc exists to enforce.
func tenantKey(c *fiber.Ctx) (string, bool) {
	tenant, ok := c.Locals("tenant").(string)
	if !ok || tenant == "" {
		return "", false
	}
	return "stats:t:" + tenant, true
}

type harness struct {
	app   *fiber.App
	store *memStore
	calls *int32
}

// newHarness mounts one cached GET whose handler echoes the caller's tenant, so
// a leaked entry is visible in the body rather than inferred.
func newHarness(t *testing.T, store *memStore) *harness {
	t.Helper()
	var calls int32

	d := NewCacheDecorationWithStore(store)
	app := fiber.New(fiber.Config{DisableStartupMessage: true})

	app.Get("/stats", func(c *fiber.Ctx) error {
		// Identity comes from headers, standing in for what the auth middleware
		// stamps. Nothing in the request path names a tenant.
		if tenant := c.Get("X-Tenant"); tenant != "" {
			c.Locals("tenant", tenant)
		}
		if perms := c.Get("X-Perms"); perms != "" {
			c.Locals("permissions", []string{perms})
		}
		if roles := c.Get("X-Role"); roles != "" {
			c.Locals("org_roles", map[uuid.UUID]string{uuid.Nil: roles})
		}
		return c.Next()
	}, d.WrapWithCache(func(c *fiber.Ctx) error {
		atomic.AddInt32(&calls, 1)
		return c.JSON(fiber.Map{"tenant": c.Locals("tenant"), "secret": "data-of-" + fmt.Sprint(c.Locals("tenant"))})
	}, tenantKey, time.Minute))

	return &harness{app: app, store: store, calls: &calls}
}

type result struct {
	status int
	body   string
	cache  string
}

func (h *harness) get(t *testing.T, path string, headers map[string]string) result {
	t.Helper()
	req := httptest.NewRequest(fiber.MethodGet, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := h.app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return result{status: resp.StatusCode, body: string(raw), cache: resp.Header.Get("X-Cache")}
}

func tenantHdr(tenant string) map[string]string { return map[string]string{"X-Tenant": tenant} }

// ===========================================================================
// THE test. Tenant A warms the cache; tenant B must never see it.
// ===========================================================================

func TestWrapWithCache_TenantBCannotBeServedTenantAsResponse(t *testing.T) {
	store := newMemStore()
	h := newHarness(t, store)

	a := h.get(t, "/stats", tenantHdr("tenant-a"))
	require.Equal(t, fiber.StatusOK, a.status)
	assert.Contains(t, a.body, "data-of-tenant-a")

	b := h.get(t, "/stats", tenantHdr("tenant-b"))
	require.Equal(t, fiber.StatusOK, b.status)

	assert.Contains(t, b.body, "data-of-tenant-b", "tenant B must get its own answer")
	assert.NotContains(t, b.body, "tenant-a", "tenant B was served tenant A's response")
	assert.Equal(t, "MISS", b.cache, "tenant B must not hit an entry warmed by tenant A")
	assert.Equal(t, 2, store.keyCount(), "two tenants must occupy two keys")
	assert.EqualValues(t, 2, atomic.LoadInt32(h.calls), "the handler must have run for each tenant")
}

func TestWrapWithCache_SecondIdenticalRequestHitsTheCache(t *testing.T) {
	store := newMemStore()
	h := newHarness(t, store)

	first := h.get(t, "/stats", tenantHdr("tenant-a"))
	second := h.get(t, "/stats", tenantHdr("tenant-a"))

	assert.Equal(t, "MISS", first.cache)
	assert.Equal(t, "HIT", second.cache)
	assert.Equal(t, first.body, second.body, "a hit must replay the same answer")
	assert.EqualValues(t, 1, atomic.LoadInt32(h.calls), "the handler must run once, not twice")
}

// The gate. A builder that cannot scope its key means DO NOT CACHE — never
// "cache under a shared key", which is how an unauthenticated response ends up
// served to everyone.
func TestWrapWithCache_UnresolvedTenantIsNeverCached(t *testing.T) {
	store := newMemStore()
	h := newHarness(t, store)

	first := h.get(t, "/stats", nil)
	second := h.get(t, "/stats", nil)

	assert.Equal(t, fiber.StatusOK, first.status, "the request is still served")
	assert.Equal(t, fiber.StatusOK, second.status)
	assert.Empty(t, first.cache, "and is marked neither HIT nor MISS")
	assert.Empty(t, second.cache)
	assert.Equal(t, 0, store.keyCount(), "nothing may be stored without a scoped key")
	assert.EqualValues(t, 2, atomic.LoadInt32(h.calls), "every such request reaches the handler")
}

// Two members of ONE tenant with different entitlements may legitimately get
// different answers. The tenant scope alone would serve one of them the other's.
func TestWrapWithCache_DifferentPermissionsDoNotShareAnEntry(t *testing.T) {
	store := newMemStore()
	h := newHarness(t, store)

	h.get(t, "/stats", map[string]string{"X-Tenant": "tenant-a", "X-Perms": "risks:read"})
	second := h.get(t, "/stats", map[string]string{"X-Tenant": "tenant-a", "X-Perms": "*"})

	assert.Equal(t, "MISS", second.cache,
		"a caller with different permissions must not be served the other's entry")
	assert.Equal(t, 2, store.keyCount())
}

// The same fingerprint is what makes an authorization CHANGE invalidate without
// an invalidation path: the caller's new entitlements hash to a new key, so the
// answer computed under the old ones is unreachable.
func TestWrapWithCache_AuthorizationChangeCannotServeTheOldAnswer(t *testing.T) {
	store := newMemStore()
	h := newHarness(t, store)

	before := h.get(t, "/stats", map[string]string{"X-Tenant": "tenant-a", "X-Perms": "risks:read"})
	require.Equal(t, "MISS", before.cache)

	// The same user, after a role change.
	after := h.get(t, "/stats", map[string]string{"X-Tenant": "tenant-a", "X-Perms": "risks:read", "X-Role": "admin"})

	assert.Equal(t, "MISS", after.cache,
		"an entitlement change must not be served the answer computed before it")
}

// Query parameters are part of the answer. The wrapper appends the whole query
// string, so a builder that remembered only some of them cannot collide.
func TestWrapWithCache_QueryStringIsPartOfTheKey(t *testing.T) {
	store := newMemStore()
	h := newHarness(t, store)

	h.get(t, "/stats?period=7d", tenantHdr("tenant-a"))
	second := h.get(t, "/stats?period=30d", tenantHdr("tenant-a"))

	assert.Equal(t, "MISS", second.cache, "two windows are two answers")
	assert.Equal(t, 2, store.keyCount())
}

// Redis down must cost latency, never availability.
func TestWrapWithCache_BackendFailureDegradesToTheHandler(t *testing.T) {
	store := newMemStore()
	store.failGet = errors.New("redis unreachable")
	store.failSet = errors.New("redis unreachable")
	h := newHarness(t, store)

	first := h.get(t, "/stats", tenantHdr("tenant-a"))
	second := h.get(t, "/stats", tenantHdr("tenant-a"))

	assert.Equal(t, fiber.StatusOK, first.status)
	assert.Equal(t, fiber.StatusOK, second.status)
	assert.Contains(t, second.body, "data-of-tenant-a")
	assert.EqualValues(t, 2, atomic.LoadInt32(h.calls), "both requests are served from the handler")
	assert.Equal(t, 0, store.keyCount())
}

// A stored entry that cannot be replayed is recomputed rather than served.
func TestWrapWithCache_UnreadableEntryIsRecomputed(t *testing.T) {
	store := newMemStore()
	h := newHarness(t, store)

	h.get(t, "/stats", tenantHdr("tenant-a")) // warm it
	key := store.onlyKey(t)
	store.putRaw(key, []byte(`{"s":200,"ct":"application/json","b":null}`))

	again := h.get(t, "/stats", tenantHdr("tenant-a"))
	assert.Equal(t, fiber.StatusOK, again.status)
	assert.Contains(t, again.body, "data-of-tenant-a", "an unusable entry must not be served")
}

// A mutation must never be served from, or written to, the cache.
func TestWrapWithCache_NonGetIsNeverCached(t *testing.T) {
	store := newMemStore()
	var calls int32
	d := NewCacheDecorationWithStore(store)

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Post("/stats", func(c *fiber.Ctx) error {
		c.Locals("tenant", c.Get("X-Tenant"))
		return c.Next()
	}, d.WrapWithCache(func(c *fiber.Ctx) error {
		atomic.AddInt32(&calls, 1)
		return c.JSON(fiber.Map{"ok": true})
	}, tenantKey, time.Minute))

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(fiber.MethodPost, "/stats", nil)
		req.Header.Set("X-Tenant", "tenant-a")
		resp, err := app.Test(req)
		require.NoError(t, err)
		_ = resp.Body.Close()
	}

	assert.EqualValues(t, 2, atomic.LoadInt32(&calls), "a POST must always reach its handler")
	assert.Equal(t, 0, store.keyCount())
}

// An error response is a moment in time. Caching a 403 or a 500 turns a
// transient failure into a sticky one.
func TestWrapWithCache_NonOKResponsesAreNotStored(t *testing.T) {
	store := newMemStore()
	d := NewCacheDecorationWithStore(store)

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/stats", func(c *fiber.Ctx) error {
		c.Locals("tenant", c.Get("X-Tenant"))
		return c.Next()
	}, d.WrapWithCache(func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "nope"})
	}, tenantKey, time.Minute))

	req := httptest.NewRequest(fiber.MethodGet, "/stats", nil)
	req.Header.Set("X-Tenant", "tenant-a")
	resp, err := app.Test(req)
	require.NoError(t, err)
	_ = resp.Body.Close()

	assert.Equal(t, fiber.StatusForbidden, resp.StatusCode)
	assert.Equal(t, 0, store.keyCount(), "a refusal must not become a cached answer")
}

// With no cache configured the wrapper is the handler, unchanged. A deployment
// without Redis must behave exactly as before.
func TestWrapWithCache_NilCacheIsAPassthrough(t *testing.T) {
	var calls int32
	d := NewCacheDecoration(nil)

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/stats", d.WrapWithCache(func(c *fiber.Ctx) error {
		atomic.AddInt32(&calls, 1)
		return c.JSON(fiber.Map{"ok": true})
	}, tenantKey, time.Minute))

	for i := 0; i < 2; i++ {
		resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/stats", nil))
		require.NoError(t, err)
		_ = resp.Body.Close()
	}
	assert.EqualValues(t, 2, atomic.LoadInt32(&calls))
}
