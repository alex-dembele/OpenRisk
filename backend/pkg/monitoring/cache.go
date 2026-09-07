// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package monitoring

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Response-cache metrics (#337).
//
// Package-level, in the style of realtime.go, so pkg/cache can record without
// being handed a collector instance through three constructors.
//
// The labels are the point. A single "cache hits" number cannot answer the
// question anyone actually asks of a cache that sits in front of tenant data —
// "is it serving, and when it is not serving, why not" — and the difference
// between a miss and a REFUSAL to cache is the difference between a cold key and
// a request whose tenant could not be resolved.
var (
	// CacheRequestsTotal counts every request that passed through the response
	// cache, by what happened to it:
	//
	//	hit       served from the cache
	//	miss      not in the cache; the handler ran and the result was stored
	//	bypass    the key builder refused to produce a key — an unresolved tenant,
	//	          or a request the builder judged unsafe to cache. NEVER cached.
	//	degraded  the cache backend errored; the handler ran and nothing was stored
	//	stale     a stored entry was rejected before use (unreadable or the wrong
	//	          shape) and re-computed
	//	uncacheable the handler answered with something not worth storing — a
	//	          non-200, or a response that named itself no-store
	CacheRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "openrisk_cache_requests_total",
		Help: "Response-cache outcomes, by result and route.",
	}, []string{"result", "route"})

	// CacheInvalidationsTotal counts deliberate evictions, by what triggered
	// them. Distinct from a TTL expiry, which Redis performs and nothing here
	// observes — see the honest note in the issue rather than a fabricated
	// eviction counter.
	CacheInvalidationsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "openrisk_cache_invalidations_total",
		Help: "Cache entries deliberately invalidated, by the scope that triggered it.",
	}, []string{"scope"})
)
