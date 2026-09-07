// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package handler

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// #336 — which routes carry a permission guard, and which rely on the session
// alone.
//
// The matrix in authz_matrix_test.go proves every mounted permission DENIES
// correctly. It cannot prove the right guard is attached to the right route:
// a guard tested in isolation says nothing about a route that was mounted
// without one. This file closes that gap by reading the composition root and
// classifying all 337 protected routes.
//
// It is a RATCHET, in the same spirit as frontend/.lint-ceiling.json. The
// unguarded set is frozen below. Mount a new protected route with no permission
// guard and this test fails until someone writes the path down here — which is
// the point: an authenticated-only route becomes a deliberate, reviewable act
// rather than an omission nobody noticed.
//
// IMPORTANT, so this list is not misread: an entry here is NOT a finding.
// Many of these are correct — a route that acts on the caller's OWN sessions,
// tokens or onboarding state is authorised by authentication itself, and some
// handlers enforce entitlements internally. What the list records is that the
// middleware layer does not decide, so something else must. Auditing which of
// the 92 genuinely need a guard is its own issue; this test exists so the
// number cannot grow quietly while that happens.
// ---------------------------------------------------------------------------

// routesWithoutPermissionGuard is the frozen set of protected routes carrying
// no RequirePermission/RequireRole at the middleware layer.
//
// This list may only SHRINK. Adding to it requires a reviewer to agree that the
// session alone is sufficient authorization for that route.
var routesWithoutPermissionGuard = []string{
	"DELETE /auth/pat/:id",
	"DELETE /auth/sessions/:id",
	"DELETE /auth/sessions/others",
	"DELETE /custom-fields/:id",
	"DELETE /tokens/:id",
	"GET /action-center",
	"GET /activation/state",
	"GET /ai/status",
	"GET /analytics/dashboard",
	"GET /analytics/export",
	"GET /analytics/frameworks",
	"GET /analytics/mitigations/metrics",
	"GET /analytics/risks/metrics",
	"GET /analytics/risks/trends",
	"GET /auth/organizations",
	"GET /auth/pat",
	"GET /auth/sessions",
	"GET /billing",
	"GET /bulk-operations",
	"GET /bulk-operations/:id",
	"GET /custom-fields",
	"GET /custom-fields/:id",
	"GET /custom-fields/scope/:scope",
	"GET /dashboard/complete",
	"GET /dashboard/metrics",
	"GET /dashboard/mitigation-progress",
	"GET /dashboard/mitigation-status",
	"GET /dashboard/risk-trends",
	"GET /dashboard/severity-distribution",
	"GET /dashboard/top-risks",
	"GET /entities",
	"GET /entities/:type/:id",
	"GET /entities/:type/:id/audit",
	"GET /entities/:type/:id/relations",
	"GET /entities/:type/:id/timeline",
	"GET /entitlements",
	"GET /governance/approvals",
	"GET /governance/approvals/:id",
	"GET /governance/delegations",
	"GET /governance/delegations/effective",
	"GET /governance/request-types",
	"GET /governance/workflows",
	"GET /marketplace/connectors",
	"GET /marketplace/connectors/:id",
	"GET /marketplace/connectors/search",
	"GET /onboarding/state",
	"GET /onboarding/suggestions",
	"GET /organization/counts",
	"GET /ownership/assignable",
	"GET /ownership/me",
	"GET /rbac/business-roles",
	"GET /risks/:id/incidents",
	"GET /risks/:id/timeline",
	"GET /risks/:id/timeline/changes/:type",
	"GET /risks/:id/timeline/score-changes",
	"GET /risks/:id/timeline/since/:timestamp",
	"GET /risks/:id/timeline/status-changes",
	"GET /risks/:id/timeline/trend",
	"GET /score",
	"GET /score/model",
	"GET /search",
	"GET /security/mfa-policy",
	"GET /stats",
	"GET /telemetry",
	"GET /timeline",
	"GET /tokens",
	"GET /tokens/:id",
	"PATCH /custom-fields/:id",
	"PATCH /users/:id",
	"POST /activation/celebrated",
	"POST /auth/mfa/disable",
	"POST /auth/pat",
	"POST /auth/switch-org",
	"POST /billing/checkout",
	"POST /billing/trial",
	"POST /bulk-operations",
	"POST /custom-fields",
	"POST /custom-fields/templates/:id/apply",
	"POST /governance/approvals",
	"POST /governance/approvals/:id/cancel",
	"POST /governance/approvals/:id/decide",
	"POST /governance/delegations",
	"POST /governance/delegations/:id/revoke",
	"POST /integrations/:id/test",
	"POST /marketplace/connectors/:id/reviews",
	"POST /onboarding/complete",
	"POST /score/preview",
	"POST /tokens",
	"POST /tokens/:id/revoke",
	"POST /tokens/:id/rotate",
	"PUT /onboarding/steps/:step",
	"PUT /tokens/:id",
}

// protectedRoute is one `protected.<Method>(...)` registration.
type protectedRoute struct {
	method  string
	path    string
	guarded bool
}

// parseProtectedRoutes reads the composition root and classifies every route
// mounted on the authenticated group.
//
// It resolves guard VARIABLES as well as inline calls: main.go builds 54 of
// them (riskCreate, scannerRead, complianceControlRead, …) and mounts most
// routes with those rather than with a literal RequirePermission. A checker
// that looked only for the literal would report 302 unguarded routes instead of
// 92 — a false alarm three times the size of the real number.
func parseProtectedRoutes(t *testing.T) []protectedRoute {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join("..", "..", "cmd", "server", "main.go"))
	require.NoError(t, err)
	src := string(raw)

	guardVars := map[string]bool{}
	for _, m := range regexp.MustCompile(`(\w+)\s*:?=\s*middleware\.(?:RequirePermission|RequireRole)\(`).FindAllStringSubmatch(src, -1) {
		guardVars[m[1]] = true
	}
	require.NotEmpty(t, guardVars, "no guard variables found — the parser is broken, not the server")

	call := regexp.MustCompile(`protected\.(Get|Post|Put|Patch|Delete)\(`)
	pathRe := regexp.MustCompile(`"([^"]*)"`)

	var out []protectedRoute
	for _, loc := range call.FindAllStringSubmatchIndex(src, -1) {
		method := src[loc[2]:loc[3]]

		// Walk to the matching close paren so a multi-line registration is read
		// whole; most guarded routes put the guard on the following line.
		depth, i := 1, loc[1]
		for i < len(src) && depth > 0 {
			switch src[i] {
			case '(':
				depth++
			case ')':
				depth--
			}
			i++
		}
		body := src[loc[0]:i]

		path := "?"
		if p := pathRe.FindStringSubmatch(body); p != nil {
			path = p[1]
		}

		guarded := strings.Contains(body, "RequirePermission") || strings.Contains(body, "RequireRole")
		if !guarded {
			for v := range guardVars {
				if regexp.MustCompile(`\b` + regexp.QuoteMeta(v) + `\b`).MatchString(body) {
					guarded = true
					break
				}
			}
		}
		out = append(out, protectedRoute{method: method, path: path, guarded: guarded})
	}
	return out
}

func routeKey(r protectedRoute) string { return strings.ToUpper(r.method) + " " + r.path }

// THE ratchet. A new protected route without a permission guard fails here.
func TestProtectedRoutes_UnguardedSetHasNotGrown(t *testing.T) {
	routes := parseProtectedRoutes(t)
	require.GreaterOrEqual(t, len(routes), 300,
		"far fewer protected routes than expected — the parser stopped matching")

	allowed := map[string]bool{}
	for _, k := range routesWithoutPermissionGuard {
		allowed[k] = true
	}

	var unexpected []string
	actual := map[string]bool{}
	for _, r := range routes {
		if r.guarded {
			continue
		}
		k := routeKey(r)
		actual[k] = true
		if !allowed[k] {
			unexpected = append(unexpected, k)
		}
	}
	sort.Strings(unexpected)

	assert.Empty(t, unexpected,
		"these protected routes were mounted with no permission guard.\n"+
			"Either give them one, or — if the session alone is sufficient — add them to\n"+
			"routesWithoutPermissionGuard with a reviewer's agreement:\n  %s",
		strings.Join(unexpected, "\n  "))
}

// The mirror: a route that GAINED a guard must be struck from the list, or the
// list slowly becomes fiction.
func TestProtectedRoutes_AllowlistHasNoStaleEntries(t *testing.T) {
	routes := parseProtectedRoutes(t)

	unguarded := map[string]bool{}
	for _, r := range routes {
		if !r.guarded {
			unguarded[routeKey(r)] = true
		}
	}

	var stale []string
	for _, k := range routesWithoutPermissionGuard {
		if !unguarded[k] {
			stale = append(stale, k)
		}
	}
	sort.Strings(stale)

	assert.Empty(t, stale,
		"these routes now carry a guard (or no longer exist) and must be removed from\n"+
			"routesWithoutPermissionGuard — the list may only shrink:\n  %s",
		strings.Join(stale, "\n  "))
}

// A guarded majority is the invariant worth stating out loud: if a refactor
// silently dropped guards, the two tests above would still pass once someone
// pasted the new paths into the list. This one would not.
func TestProtectedRoutes_MostRoutesCarryAPermissionGuard(t *testing.T) {
	routes := parseProtectedRoutes(t)

	guarded := 0
	for _, r := range routes {
		if r.guarded {
			guarded++
		}
	}

	t.Logf("protected routes: %d, guarded: %d, session-only: %d", len(routes), guarded, len(routes)-guarded)
	assert.Greater(t, guarded*100/len(routes), 60,
		"fewer than 60%% of protected routes carry a permission guard — something was dropped wholesale")
}
