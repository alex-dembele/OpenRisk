// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/opendefender/openrisk/pkg/monitoring"
)

// ResponseStore is the slice of the cache the response wrapper needs.
//
// An interface rather than *Cache so the wrapper can be tested without a Redis
// server. Adding a Redis test double would be a new dependency, and the
// behaviour worth testing here — that a tenant can never be served another
// tenant's entry — is decided by the key, not by Redis.
type ResponseStore interface {
	Get(ctx context.Context, key string, dest interface{}) error
	SetWithTTL(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	DeletePattern(ctx context.Context, pattern string) error
}

// CacheDecoration provides lightweight handler/cache helpers.
type CacheDecoration struct {
	cache ResponseStore
}

// NewCacheDecoration wires the decoration to a real cache.
//
// A nil *Cache stays nil in the interface rather than becoming a non-nil
// interface holding a nil pointer, so the `d.cache == nil` guards throughout
// this file keep working for a deployment with no Redis.
func NewCacheDecoration(cacheInstance *Cache) *CacheDecoration {
	if cacheInstance == nil {
		return &CacheDecoration{}
	}
	return &CacheDecoration{cache: cacheInstance}
}

// NewCacheDecorationWithStore wires an arbitrary store. Exists for tests.
func NewCacheDecorationWithStore(store ResponseStore) *CacheDecoration {
	return &CacheDecoration{cache: store}
}

func (d *CacheDecoration) BatchInvalidate(ctx context.Context, patterns ...string) error {
	if d == nil || d.cache == nil {
		return nil
	}
	for _, p := range patterns {
		if err := d.cache.DeletePattern(ctx, p); err != nil {
			return err
		}
	}
	return nil
}

// KeyFunc builds a cache key for one request.
//
// THE BOOLEAN IS THE SAFETY GATE, and it is why this type exists rather than a
// plain func(*fiber.Ctx) string. A key builder that cannot fully scope its key —
// most importantly, that cannot resolve the tenant — returns false, and the
// wrapper must then not cache at all.
//
// Before W0-06 the builders returned a bare string and none of the dashboard or
// register ones carried a tenant: `dashboard:stats:month`, `dashboard:matrix:all`,
// `risk:list:page:1:sev::status:`. Those keys were harmless only because
// WrapWithCache below ignores them, which is not a property anyone reading the
// call sites in main.go could see. With this signature a builder that forgets the
// tenant cannot return a key at all, so the omission is a compile error and not a
// cross-tenant disclosure discovered in production.
type KeyFunc func(*fiber.Ctx) (string, bool)

// cachedResponse is what a stored entry holds. Only what is needed to replay
// the answer: no headers a client could use to distinguish a served response
// from a fresh one beyond the explicit X-Cache marker.
type cachedResponse struct {
	Status      int    `json:"s"`
	ContentType string `json:"ct"`
	Body        []byte `json:"b"`
}

// scopedKey is the key actually written to Redis.
//
// The builder's key is not trusted to be complete. Everything below is appended
// by the WRAPPER, so a builder that forgets one of them cannot cause a
// collision — the gate makes an unscoped key impossible, and this makes an
// under-scoped one harmless:
//
//	method + path   two routes must never share an entry
//	query           the full query string, not the handful a builder remembered
//	authz           a fingerprint of the caller's permissions and org roles
//
// The authz fingerprint is what satisfies "authorization changes invalidate
// relevant entries" WITHOUT an invalidation path: a caller whose permissions
// changed hashes to a different key and therefore cannot be served the answer
// computed for their old entitlements. There is nothing to remember to evict,
// which is the only kind of invalidation that cannot be forgotten.
//
// It also covers the intra-tenant case the tenant scope does not: two members of
// one organization with different permissions may legitimately get different
// answers from the same URL, and a tenant-only key would serve one of them the
// other's.
func scopedKey(c *fiber.Ctx, builderKey string) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\n%s\n%s\n", c.Method(), c.Path(), c.Request().URI().QueryString())

	// Sorted, so an unstable claim order cannot fragment the cache or, worse,
	// make two different entitlement sets hash the same by accident.
	if perms, ok := c.Locals("permissions").([]string); ok {
		sorted := append([]string(nil), perms...)
		sort.Strings(sorted)
		for _, p := range sorted {
			h.Write([]byte(p))
			h.Write([]byte{0})
		}
	}
	h.Write([]byte{'|'})
	if roles, ok := c.Locals("org_roles").(map[uuid.UUID]string); ok {
		keys := make([]string, 0, len(roles))
		for k, v := range roles {
			keys = append(keys, k.String()+"="+v)
		}
		sort.Strings(keys)
		for _, k := range keys {
			h.Write([]byte(k))
			h.Write([]byte{0})
		}
	}

	return "resp:" + builderKey + ":z:" + hex.EncodeToString(h.Sum(nil)[:16])
}

// routeLabel keeps the metric cardinality bounded: the ROUTE pattern, not the
// resolved path, so /risks/:id is one series rather than one per risk.
func routeLabel(c *fiber.Ctx) string {
	if r := c.Route(); r != nil && r.Path != "" {
		return r.Path
	}
	return "unknown"
}

// WrapWithCache caches a handler's response, scoped so that one tenant's answer
// can never be served to another (#337).
//
// It was a passthrough until this change: the call sites in main.go read exactly
// like cached routes and cached nothing. The key builders were already
// tenant-scoped and tested, which is why turning it on is a change to this
// function and not to twenty call sites.
//
// The rules, in the order they are applied:
//
//  1. Only GET is cached. A mutation must never be served from a cache, and a
//     HEAD is cheap enough not to be worth the risk of a body/verb mismatch.
//  2. The gate is honoured: `keyFn` returning false means DO NOT CACHE, not
//     "cache under a shared key". An unresolved tenant lands here.
//  3. A backend error is never fatal. Redis down means the handler runs and the
//     request is served from the database, which is the pre-cache behaviour —
//     degraded, not broken.
//  4. Only a 200 is stored. An error response is a moment in time, and caching a
//     403 or a 500 turns a transient failure into a sticky one.
func (d *CacheDecoration) WrapWithCache(handler fiber.Handler, keyFn KeyFunc, ttl time.Duration) fiber.Handler {
	// No cache wired: the deployment runs without Redis and every route behaves
	// exactly as it did before. Nil-safe on purpose — main.go leaves the whole
	// decoration nil when Redis is absent.
	if d == nil || d.cache == nil || keyFn == nil {
		return handler
	}

	return func(c *fiber.Ctx) error {
		route := routeLabel(c)

		if c.Method() != fiber.MethodGet {
			monitoring.CacheRequestsTotal.WithLabelValues("bypass", route).Inc()
			return handler(c)
		}

		builderKey, ok := keyFn(c)
		if !ok {
			// THE safety gate. The builder could not fully scope the key —
			// most often an unresolved tenant — so this request is served
			// uncached rather than under a key that might be shared.
			monitoring.CacheRequestsTotal.WithLabelValues("bypass", route).Inc()
			return handler(c)
		}
		key := scopedKey(c, builderKey)
		ctx := c.UserContext()

		var entry cachedResponse
		switch err := d.cache.Get(ctx, key, &entry); {
		case err == nil:
			if entry.Status == fiber.StatusOK && len(entry.Body) > 0 {
				monitoring.CacheRequestsTotal.WithLabelValues("hit", route).Inc()
				if entry.ContentType != "" {
					c.Set(fiber.HeaderContentType, entry.ContentType)
				}
				// Says the answer was replayed. Useful in a bug report and
				// harmless to a client that ignores it.
				c.Set("X-Cache", "HIT")
				return c.Status(entry.Status).Send(entry.Body)
			}
			// Stored, but not something worth replaying — a truncated or
			// wrong-shaped entry. Recompute rather than serve it.
			monitoring.CacheRequestsTotal.WithLabelValues("stale", route).Inc()
		case errors.Is(err, ErrCacheMiss):
			monitoring.CacheRequestsTotal.WithLabelValues("miss", route).Inc()
		default:
			// Redis unreachable or answering nonsense. Fall through to the
			// handler: the database is the source of truth and the cache is an
			// optimisation, never a dependency.
			monitoring.CacheRequestsTotal.WithLabelValues("degraded", route).Inc()
		}

		if err := handler(c); err != nil {
			return err
		}

		status := c.Response().StatusCode()
		if status != fiber.StatusOK {
			monitoring.CacheRequestsTotal.WithLabelValues("uncacheable", route).Inc()
			return nil
		}
		body := append([]byte(nil), c.Response().Body()...)
		if len(body) == 0 {
			monitoring.CacheRequestsTotal.WithLabelValues("uncacheable", route).Inc()
			return nil
		}

		// Best-effort. A failed write costs a future hit and nothing else; the
		// response has already been produced and must not be failed for it.
		_ = d.cache.SetWithTTL(ctx, key, cachedResponse{
			Status:      status,
			ContentType: string(c.Response().Header.ContentType()),
			Body:        body,
		}, ttl)

		c.Set("X-Cache", "MISS")
		return nil
	}
}

type RequestCacheContext struct {
	cache *Cache
	ctx   context.Context
}

func NewRequestCacheContext(cacheInstance *Cache, ctx context.Context) *RequestCacheContext {
	return &RequestCacheContext{cache: cacheInstance, ctx: ctx}
}

func (r *RequestCacheContext) GetOrSet(key string, dest interface{}, compute func() (interface{}, error)) error {
	if r == nil || r.cache == nil {
		value, err := compute()
		if err != nil {
			return err
		}
		b, err := json.Marshal(value)
		if err != nil {
			return err
		}
		return json.Unmarshal(b, dest)
	}

	if err := r.cache.Get(r.ctx, key, dest); err == nil {
		return nil
	}

	value, err := compute()
	if err != nil {
		return err
	}
	if err := r.cache.Set(r.ctx, key, value); err != nil {
		return err
	}
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dest)
}

// ResponseKeyPrefix is the namespace every response-cache entry lives under.
// Exported so an invalidator can target exactly what WrapWithCache wrote and
// nothing else — the pre-#337 patterns ("risk:*", "dashboard:*") match none of
// these keys, and would have silently invalidated nothing.
const ResponseKeyPrefix = "resp:"

// InvalidateTenant drops every cached response belonging to one tenant.
//
// Deliberately coarse. The alternative — a hook per handler naming the families
// it touches — is precise right up to the first handler somebody adds without
// one, and a stale compliance figure shown to an auditor is a worse outcome than
// a cache that refills. There are five cached routes; flushing one tenant's five
// entries on a write is cheap, and it cannot be forgotten.
//
// Scoped to the tenant on purpose: one organization's write must not evict
// another's entries, or a busy tenant would keep the whole estate cold.
func (d *CacheDecoration) InvalidateTenant(ctx context.Context, tenantID string, scope string) error {
	if d == nil || d.cache == nil || tenantID == "" {
		return nil
	}
	monitoring.CacheInvalidationsTotal.WithLabelValues(scope).Inc()
	// Every builder embeds the tenant as ":t:<uuid>", so this matches that
	// tenant's entries across every family and no other tenant's.
	return d.cache.DeletePattern(ctx, ResponseKeyPrefix+"*t:"+tenantID+"*")
}
