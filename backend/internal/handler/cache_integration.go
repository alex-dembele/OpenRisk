// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/opendefender/openrisk/internal/middleware"
	"github.com/opendefender/openrisk/pkg/cache"
)

// CacheConfig holds cache configuration for handlers
type CacheConfig struct {
	RiskCacheTTL      time.Duration
	DashboardCacheTTL time.Duration
	ConnectorCacheTTL time.Duration
	MarketplaceAppTTL time.Duration
}

// DefaultCacheConfig returns default cache TTLs
func DefaultCacheConfig() CacheConfig {
	return CacheConfig{
		RiskCacheTTL:      5 * time.Minute,
		DashboardCacheTTL: 10 * time.Minute,
		ConnectorCacheTTL: 15 * time.Minute,
		MarketplaceAppTTL: 20 * time.Minute,
	}
}

// CacheableHandlers provides cache-enabled handler wrapper.
//
// A NIL *CacheableHandlers IS VALID and means "no cache configured". Caching is
// an optimisation, not a feature: losing Redis must cost latency, never
// availability. main.go left this nil whenever Redis was unreachable and then
// called methods on it while registering routes, so the server panicked during
// start-up — no Redis meant no server at all. Every method tolerates a nil
// receiver: wrappers return the handler unchanged, invalidation is a no-op, and
// get-or-set computes directly.
type CacheableHandlers struct {
	cache       *cache.Cache
	cacheConfig CacheConfig
	decoration  *cache.CacheDecoration
}

// NewCacheableHandlers creates a new caching handler wrapper
func NewCacheableHandlers(cacheInstance *cache.Cache) *CacheableHandlers {
	return &CacheableHandlers{
		cache:       cacheInstance,
		cacheConfig: DefaultCacheConfig(),
		decoration:  cache.NewCacheDecoration(cacheInstance),
	}
}

// Decoration exposes the underlying cache decoration so main.go can mount the
// invalidation middleware with the same instance the wrappers use. Nil-safe: a
// deployment without Redis gets a nil decoration, and every method on it is a
// no-op.
func (ch *CacheableHandlers) Decoration() *cache.CacheDecoration {
	if ch == nil {
		return nil
	}
	return ch.decoration
}

// SetCacheConfig updates cache TTL configuration
func (ch *CacheableHandlers) SetCacheConfig(cfg CacheConfig) {
	if ch == nil {
		return
	}
	ch.cacheConfig = cfg
}

// ---------------------------------------------------------------------------
// Cache keys
//
// Every key below starts with the tenant. That is the entire point of this
// section, and it is worth saying why in one place rather than five.
//
// Before W0-06 these were anonymous closures inside the wrapper methods, and not
// one of them carried a tenant: `dashboard:stats:month`, `dashboard:matrix:all`,
// `risk:list:page:1:sev::status:`. Any tenant's request would have produced the
// same key as any other tenant's, so the first response cached would have been
// served to everyone. It was not a live disclosure, for exactly one reason:
// cache.WrapWithCache returns the handler unchanged and never evaluates the key.
// Dead keys — but dead in a way nobody reading main.go could see, and one
// implementation of that wrapper away from being a leak on every dashboard at
// once.
//
// They are named functions now so the tests can call them directly and assert
// two things that no amount of reading guarantees: that a request with no
// resolved tenant yields NO key, and that two tenants never collide.
// ---------------------------------------------------------------------------

// tenantScope returns the request's tenant prefix, or false when there is none.
//
// False is not an error path to paper over. It is the correct answer for an
// unauthenticated or unresolved request, and the KeyFunc contract turns it into
// "do not cache" rather than "cache under a shared key".
func tenantScope(c *fiber.Ctx) (string, bool) {
	ctx := middleware.GetContext(c)
	if ctx == nil || ctx.OrganizationID == uuid.Nil {
		return "", false
	}
	return "t:" + ctx.OrganizationID.String(), true
}

func dashboardStatsCacheKey(c *fiber.Ctx) (string, bool) {
	scope, ok := tenantScope(c)
	if !ok {
		return "", false
	}
	// The period is part of the key because it is part of the answer: two
	// windows over the same register are two different responses.
	return fmt.Sprintf("dashboard:stats:%s:period:%s:from:%s:to:%s",
		scope, c.Query("period"), c.Query("from"), c.Query("to")), true
}

func dashboardMatrixCacheKey(c *fiber.Ctx) (string, bool) {
	scope, ok := tenantScope(c)
	if !ok {
		return "", false
	}
	return fmt.Sprintf("dashboard:matrix:%s", scope), true
}

func dashboardTimelineCacheKey(c *fiber.Ctx) (string, bool) {
	scope, ok := tenantScope(c)
	if !ok {
		return "", false
	}
	return fmt.Sprintf("dashboard:timeline:%s:days:%s", scope, c.Query("days", "30")), true
}

func riskListCacheKey(c *fiber.Ctx) (string, bool) {
	scope, ok := tenantScope(c)
	if !ok {
		return "", false
	}
	return fmt.Sprintf("risk:list:%s:page:%s:sev:%s:status:%s",
		scope, c.Query("page", "1"), c.Query("severity", ""), c.Query("status", "")), true
}

func riskSearchCacheKey(c *fiber.Ctx) (string, bool) {
	scope, ok := tenantScope(c)
	if !ok {
		return "", false
	}
	return fmt.Sprintf("risk:search:%s:%s", scope, hashQuery(c.Query("q", ""))), true
}

func riskByIDCacheKey(c *fiber.Ctx) (string, bool) {
	scope, ok := tenantScope(c)
	if !ok {
		return "", false
	}
	// A risk id is a UUID and is not guessable, but the tenant still belongs in
	// the key: the response body is tenant data, and "unguessable" is not the
	// same guarantee as "scoped".
	return fmt.Sprintf("risk:id:%s:%s", scope, c.Params("id")), true
}

func connectorListCacheKey(c *fiber.Ctx) (string, bool) {
	scope, ok := tenantScope(c)
	if !ok {
		return "", false
	}
	return fmt.Sprintf("connector:list:%s:cat:%s:status:%s",
		scope, c.Query("category", "all"), c.Query("status", "all")), true
}

func connectorByIDCacheKey(c *fiber.Ctx) (string, bool) {
	scope, ok := tenantScope(c)
	if !ok {
		return "", false
	}
	return fmt.Sprintf("connector:id:%s:%s", scope, c.Params("id")), true
}

func marketplaceAppByIDCacheKey(c *fiber.Ctx) (string, bool) {
	scope, ok := tenantScope(c)
	if !ok {
		return "", false
	}
	return fmt.Sprintf("marketplace:app:%s:%s", scope, c.Params("id")), true
}

// ============================================================================
// RISK HANDLER CACHE HELPERS
// ============================================================================

// GetRiskCacheKey generates cache key for risk operations
func (ch *CacheableHandlers) GetRiskCacheKey(operation string, params ...string) string {
	key := fmt.Sprintf("risk:%s", operation)
	for _, param := range params {
		key += fmt.Sprintf(":%s", param)
	}
	return key
}

// InvalidateRiskCaches invalidates all risk-related cache entries
func (ch *CacheableHandlers) InvalidateRiskCaches(ctx context.Context) error {
	if ch == nil {
		return nil // nothing cached, nothing to invalidate
	}
	return ch.decoration.BatchInvalidate(ctx,
		"risk:*",      // All risk entries
		"report:*",    // Reports depend on risks
		"dashboard:*", // Dashboard depends on risks
	)
}

// InvalidateSpecificRisk invalidates cache for a specific risk
func (ch *CacheableHandlers) InvalidateSpecificRisk(ctx context.Context, riskID string) error {
	if ch == nil {
		return nil // nothing cached, nothing to invalidate
	}
	return ch.decoration.BatchInvalidate(ctx,
		fmt.Sprintf("risk:id:%s", riskID),
		"risk:list:*",
		"risk:search:*",
		"report:*",
		"dashboard:*",
	)
}

// CacheRiskListGET wraps a GET risk list handler with caching
func (ch *CacheableHandlers) CacheRiskListGET(handler fiber.Handler) fiber.Handler {
	if ch == nil {
		return handler // no cache configured — serve directly
	}
	return ch.decoration.WrapWithCache(
		handler,
		riskListCacheKey,
		ch.cacheConfig.RiskCacheTTL,
	)
}

// CacheRiskSearchGET wraps a GET risk search handler with caching
func (ch *CacheableHandlers) CacheRiskSearchGET(handler fiber.Handler) fiber.Handler {
	if ch == nil {
		return handler // no cache configured — serve directly
	}
	return ch.decoration.WrapWithCache(
		handler,
		riskSearchCacheKey,
		ch.cacheConfig.RiskCacheTTL,
	)
}

// CacheRiskGetByIDGET wraps a GET risk by ID handler with caching
func (ch *CacheableHandlers) CacheRiskGetByIDGET(handler fiber.Handler) fiber.Handler {
	if ch == nil {
		return handler // no cache configured — serve directly
	}
	return ch.decoration.WrapWithCache(
		handler,
		riskByIDCacheKey,
		ch.cacheConfig.RiskCacheTTL,
	)
}

// computeInto runs the compute function and copies its result into dest, which
// is what the cache path does on a miss. Used when there is no cache at all.
func computeInto(dest interface{}, compute func() (interface{}, error)) error {
	value, err := compute()
	if err != nil {
		return err
	}
	if dest == nil || value == nil {
		return nil
	}
	// Round-trip through JSON so dest is populated exactly as the cache path
	// would have populated it (same tags, same conversions).
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, dest)
}

// ============================================================================
// DASHBOARD HANDLER CACHE HELPERS
// ============================================================================

// GetDashboardCacheKey generates cache key for dashboard operations
func (ch *CacheableHandlers) GetDashboardCacheKey(operation string, params ...string) string {
	key := fmt.Sprintf("dashboard:%s", operation)
	for _, param := range params {
		key += fmt.Sprintf(":%s", param)
	}
	return key
}

// InvalidateDashboardCaches invalidates all dashboard cache entries
func (ch *CacheableHandlers) InvalidateDashboardCaches(ctx context.Context) error {
	if ch == nil {
		return nil // nothing cached, nothing to invalidate
	}
	return ch.cache.DeletePattern(ctx, "dashboard:*")
}

// CacheDashboardStatsGET wraps a GET dashboard stats handler with caching
func (ch *CacheableHandlers) CacheDashboardStatsGET(handler fiber.Handler) fiber.Handler {
	if ch == nil {
		return handler // no cache configured — serve directly
	}
	return ch.decoration.WrapWithCache(
		handler,
		dashboardStatsCacheKey,
		ch.cacheConfig.DashboardCacheTTL,
	)
}

// CacheDashboardMatrixGET wraps a GET dashboard matrix handler with caching
func (ch *CacheableHandlers) CacheDashboardMatrixGET(handler fiber.Handler) fiber.Handler {
	if ch == nil {
		return handler // no cache configured — serve directly
	}
	return ch.decoration.WrapWithCache(
		handler,
		dashboardMatrixCacheKey,
		ch.cacheConfig.DashboardCacheTTL,
	)
}

// CacheDashboardTimelineGET wraps a GET dashboard timeline handler with caching
func (ch *CacheableHandlers) CacheDashboardTimelineGET(handler fiber.Handler) fiber.Handler {
	if ch == nil {
		return handler // no cache configured — serve directly
	}
	return ch.decoration.WrapWithCache(
		handler,
		dashboardTimelineCacheKey,
		ch.cacheConfig.DashboardCacheTTL,
	)
}

// ============================================================================
// MARKETPLACE HANDLER CACHE HELPERS
// ============================================================================

// GetMarketplaceCacheKey generates cache key for marketplace operations
func (ch *CacheableHandlers) GetMarketplaceCacheKey(operation string, params ...string) string {
	key := fmt.Sprintf("connector:%s", operation)
	for _, param := range params {
		key += fmt.Sprintf(":%s", param)
	}
	return key
}

// InvalidateMarketplaceCaches invalidates all marketplace cache entries
func (ch *CacheableHandlers) InvalidateMarketplaceCaches(ctx context.Context) error {
	return ch.decoration.BatchInvalidate(ctx,
		"connector:*",
		"marketplace:app:*",
	)
}

// InvalidateSpecificConnector invalidates cache for a specific connector
func (ch *CacheableHandlers) InvalidateSpecificConnector(ctx context.Context, connectorID string) error {
	if ch == nil {
		return nil // nothing cached, nothing to invalidate
	}
	return ch.decoration.BatchInvalidate(ctx,
		fmt.Sprintf("connector:id:%s", connectorID),
		"connector:list:*",
		"connector:health",
	)
}

// CacheConnectorListGET wraps a GET connector list handler with caching
func (ch *CacheableHandlers) CacheConnectorListGET(handler fiber.Handler) fiber.Handler {
	if ch == nil {
		return handler // no cache configured — serve directly
	}
	return ch.decoration.WrapWithCache(
		handler,
		connectorListCacheKey,
		ch.cacheConfig.ConnectorCacheTTL,
	)
}

// CacheConnectorGetByIDGET wraps a GET connector by ID handler with caching
func (ch *CacheableHandlers) CacheConnectorGetByIDGET(handler fiber.Handler) fiber.Handler {
	if ch == nil {
		return handler // no cache configured — serve directly
	}
	return ch.decoration.WrapWithCache(
		handler,
		connectorByIDCacheKey,
		ch.cacheConfig.ConnectorCacheTTL,
	)
}

// CacheMarketplaceAppGetByIDGET wraps a GET app by ID handler with caching
func (ch *CacheableHandlers) CacheMarketplaceAppGetByIDGET(handler fiber.Handler) fiber.Handler {
	if ch == nil {
		return handler // no cache configured — serve directly
	}
	return ch.decoration.WrapWithCache(
		handler,
		marketplaceAppByIDCacheKey,
		ch.cacheConfig.MarketplaceAppTTL,
	)
}

// ============================================================================
// UTILITY FUNCTIONS
// ============================================================================

// hashQuery creates a hash of a query string for cache key
func hashQuery(query string) string {
	if query == "" {
		return "empty"
	}
	// Simple hash for cache key (can use MD5 for collision resistance)
	h := 0
	for _, c := range query {
		h = ((h << 5) - h) + int(c)
	}
	return fmt.Sprintf("%x", h)
}

// ============================================================================
// CONTEXT CACHE HELPERS (for inline handler caching)
// ============================================================================

// GetOrSetRiskData provides cache-or-compute pattern for risk data
func (ch *CacheableHandlers) GetOrSetRiskData(ctx context.Context, key string, dest interface{}, compute func() (interface{}, error)) error {
	if ch == nil {
		return computeInto(dest, compute)
	}
	cacheCtx := cache.NewRequestCacheContext(ch.cache, ctx)
	return cacheCtx.GetOrSet(key, dest, compute)
}

// GetOrSetDashboardData provides cache-or-compute pattern for dashboard data
func (ch *CacheableHandlers) GetOrSetDashboardData(ctx context.Context, key string, dest interface{}, compute func() (interface{}, error)) error {
	if ch == nil {
		return computeInto(dest, compute)
	}
	cacheCtx := cache.NewRequestCacheContext(ch.cache, ctx)
	return cacheCtx.GetOrSet(key, dest, compute)
}

// GetOrSetMarketplaceData provides cache-or-compute pattern for marketplace data
func (ch *CacheableHandlers) GetOrSetMarketplaceData(ctx context.Context, key string, dest interface{}, compute func() (interface{}, error)) error {
	if ch == nil {
		return computeInto(dest, compute)
	}
	cacheCtx := cache.NewRequestCacheContext(ch.cache, ctx)
	return cacheCtx.GetOrSet(key, dest, compute)
}
