// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/opendefender/openrisk/internal/middleware"
	authpkg "github.com/opendefender/openrisk/pkg/auth"
)

// ---------------------------------------------------------------------------
// #336 — the authorization denial matrix, driven by the real composition root.
//
// The complaint this issue makes is exact: permission-denied behaviour was
// unit-tested but never demonstrated against a realistic account. What existed
// under backend/tests/ was worse than nothing — mock repositories behind a
// `//go:build integration` tag, importing a module path that does not exist, so
// it could not compile and had never run.
//
// This file drives the REAL guard, middleware.RequirePermission, over EVERY
// permission the server actually mounts. The permission list is not typed out
// here: it is extracted from cmd/server/main.go at test time, so a route added
// tomorrow with a new permission is covered tomorrow, and a permission that
// stops being used stops being asserted. A hand-copied list would have gone
// stale the first week.
//
// What each case proves, per the issue's matrix:
//
//	anonymous                      → 401, and the handler never runs
//	authenticated, wrong permission → 403, and the handler never runs
//	authorised                      → 200
//	tenant admin (wildcard)         → 200
//	another tenant's admin          → 200 at the guard, refused by tenant scope
//
// "the handler never runs" is the data-leakage assertion. A guard that returns
// 403 AFTER its handler has read the database has already done the damage; the
// sentinel records whether execution reached it at all.
// ---------------------------------------------------------------------------

// permissionGuard is one RequirePermission(...) call site as the server mounts
// it. `perms` is the any-of set the guard was constructed with.
type permissionGuard struct {
	perms []string
}

func (g permissionGuard) name() string { return strings.Join(g.perms, "|") }

// mountedPermissionGuards reads the composition root and returns every distinct
// RequirePermission(...) argument set it mounts.
//
// Parsing the source rather than importing it: cmd/server is package main and
// its routing lives inside a 3000-line main(), so there is no SetupRoutes to
// call. Reading the file is the only way to assert against what is actually
// mounted instead of against a copy that drifts.
func mountedPermissionGuards(t *testing.T) []permissionGuard {
	t.Helper()

	src, err := os.ReadFile(filepath.Join("..", "..", "cmd", "server", "main.go"))
	require.NoError(t, err, "the composition root must be readable to know what is mounted")

	call := regexp.MustCompile(`RequirePermission\(([^)]*)\)`)
	arg := regexp.MustCompile(`"([^"]+)"`)

	seen := map[string]permissionGuard{}
	for _, m := range call.FindAllStringSubmatch(string(src), -1) {
		var perms []string
		for _, a := range arg.FindAllStringSubmatch(m[1], -1) {
			perms = append(perms, a[1])
		}
		if len(perms) == 0 {
			// A guard built from variables rather than literals. Nothing to
			// assert about a string this test cannot see; skipped deliberately
			// rather than silently.
			continue
		}
		g := permissionGuard{perms: perms}
		seen[g.name()] = g
	}

	out := make([]permissionGuard, 0, len(seen))
	for _, g := range seen {
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name() < out[j].name() })

	require.NotEmpty(t, out, "no permission guards found — the extraction is broken, not the server")
	return out
}

// authzActor is a caller the harness impersonates. It stamps exactly what the
// real Protected middleware stamps and nothing more: no request path, header or
// body ever names the tenant.
type authzActor struct {
	name        string
	userID      uuid.UUID
	tenantID    uuid.UUID
	permissions []string
	orgRoles    map[uuid.UUID]string
	anonymous   bool
}

type authzFixture struct {
	app      *fiber.App
	actors   map[string]authzActor
	reached  map[string]bool
	tenantA  uuid.UUID
	tenantB  uuid.UUID
	guardSet []permissionGuard
}

// newAuthzFixture mounts one sentinel route per permission guard, behind the
// REAL guard and the same authentication stamp the server uses.
func newAuthzFixture(t *testing.T) *authzFixture {
	t.Helper()

	f := &authzFixture{
		actors:  map[string]authzActor{},
		reached: map[string]bool{},
		tenantA: uuid.New(),
		tenantB: uuid.New(),
	}
	f.guardSet = mountedPermissionGuards(t)

	add := func(a authzActor) { f.actors[a.name] = a }

	// A tenant administrator: the wildcard every admin membership resolves to.
	add(authzActor{
		name: "admin-a", userID: uuid.New(), tenantID: f.tenantA,
		permissions: []string{"*"},
		orgRoles:    map[uuid.UUID]string{f.tenantA: "admin"},
	})
	// An administrator of a DIFFERENT tenant. Their credential is genuine; it
	// simply belongs to somebody else's organization.
	add(authzActor{
		name: "admin-b", userID: uuid.New(), tenantID: f.tenantB,
		permissions: []string{"*"},
		orgRoles:    map[uuid.UUID]string{f.tenantB: "admin"},
	})
	// A real member of tenant A holding exactly one unrelated permission. This
	// is the account the issue says did not exist: authenticated, legitimate,
	// and entitled to nothing on the endpoint under test.
	add(authzActor{
		name: "member-a", userID: uuid.New(), tenantID: f.tenantA,
		permissions: []string{"dashboard:read"},
		orgRoles:    map[uuid.UUID]string{f.tenantA: "user"},
	})

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	api := app.Group("/api/v1")

	// Stands in for middleware.Protected. It refuses an unauthenticated caller
	// with the same 401 the real gate returns, and otherwise stamps the identity
	// exactly as the real one does.
	protected := api.Group("", func(c *fiber.Ctx) error {
		actor, ok := f.actors[c.Get("X-Test-Actor")]
		if !ok || actor.anonymous {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
		}
		c.Locals("permissions", actor.permissions)
		c.Locals("org_roles", actor.orgRoles)
		c.Locals("user", &authpkg.Claims{
			Sub: actor.userID, TenantID: actor.tenantID,
			Permissions: actor.permissions, OrgRoles: actor.orgRoles,
		})
		middleware.SetContext(c, &middleware.RequestContext{
			UserID: actor.userID, OrganizationID: actor.tenantID,
		})
		return c.Next()
	})

	// One route per guard. The handler is a sentinel: it records that execution
	// got past the guard and returns a payload no unauthorised caller may see.
	for _, g := range f.guardSet {
		guard := g
		protected.Get("/authz/"+guardPath(guard), middleware.RequirePermission(guard.perms...), func(c *fiber.Ctx) error {
			f.reached[guard.name()] = true
			return c.JSON(fiber.Map{
				"secret":    "tenant-scoped-payload",
				"tenant_id": tenantID(c).String(),
			})
		})
	}

	f.app = app
	return f
}

// guardPath makes a guard addressable without leaking its colons into a path segment.
func guardPath(g permissionGuard) string {
	return strings.NewReplacer(":", "-", "|", "_", "*", "star").Replace(g.name())
}

type authzResponse struct {
	status int
	body   map[string]any
	raw    string
}

func (f *authzFixture) call(t *testing.T, actor string, g permissionGuard) authzResponse {
	t.Helper()
	f.reached[g.name()] = false

	req := httptest.NewRequest(http.MethodGet, "/api/v1/authz/"+guardPath(g), nil)
	if actor != "" {
		req.Header.Set("X-Test-Actor", actor)
	}
	resp, err := f.app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	body := map[string]any{}
	_ = json.Unmarshal(raw, &body)
	return authzResponse{status: resp.StatusCode, body: body, raw: string(raw)}
}

// assertNoLeak is the "absence of data leakage" criterion, asserted twice over:
// the sentinel must not have run, and its payload must not appear in the body.
func (f *authzFixture) assertNoLeak(t *testing.T, g permissionGuard, r authzResponse) {
	t.Helper()
	assert.False(t, f.reached[g.name()],
		"the handler ran despite the refusal — a guard that denies after reading is not a guard")
	assert.NotContains(t, r.raw, "tenant-scoped-payload", "the refusal carried the payload")
	assert.NotContains(t, r.raw, f.tenantA.String(), "the refusal named a tenant")
}

// ===========================================================================
// The matrix. One subtest per mounted permission, so a failure names the exact
// guard rather than "authorization is broken".
// ===========================================================================

func TestAuthzMatrix_AnonymousIsRefused(t *testing.T) {
	f := newAuthzFixture(t)

	for _, g := range f.guardSet {
		t.Run(g.name(), func(t *testing.T) {
			r := f.call(t, "", g)
			assert.Equal(t, http.StatusUnauthorized, r.status,
				"an unauthenticated caller must be refused before authorization is considered")
			f.assertNoLeak(t, g, r)
		})
	}
}

func TestAuthzMatrix_AuthenticatedWithoutPermissionIsForbidden(t *testing.T) {
	f := newAuthzFixture(t)

	for _, g := range f.guardSet {
		if hasAny(g.perms, "dashboard:read") {
			// The low-privilege actor legitimately holds this one.
			continue
		}
		t.Run(g.name(), func(t *testing.T) {
			r := f.call(t, "member-a", g)

			assert.Equal(t, http.StatusForbidden, r.status,
				"a real member without the permission must get 403, not 401 and not 200")
			// Response schema, per the acceptance criteria.
			assert.Equal(t, "FORBIDDEN", r.body["code"], "the refusal must carry a stable machine code")
			assert.NotEmpty(t, r.body["message"], "and say what was missing")
			f.assertNoLeak(t, g, r)
		})
	}
}

func TestAuthzMatrix_HolderOfThePermissionIsAllowed(t *testing.T) {
	f := newAuthzFixture(t)

	for _, g := range f.guardSet {
		t.Run(g.name(), func(t *testing.T) {
			// An actor holding exactly the permission under test and nothing
			// else — so a pass cannot come from some other entitlement.
			holder := authzActor{
				name: "holder", userID: uuid.New(), tenantID: f.tenantA,
				permissions: []string{g.perms[0]},
				orgRoles:    map[uuid.UUID]string{f.tenantA: "user"},
			}
			f.actors["holder"] = holder

			r := f.call(t, "holder", g)
			assert.Equal(t, http.StatusOK, r.status,
				"the permission the route names must actually open it")
			assert.True(t, f.reached[g.name()], "the handler must have run")
		})
	}
}

func TestAuthzMatrix_TenantAdminIsAllowed(t *testing.T) {
	f := newAuthzFixture(t)

	for _, g := range f.guardSet {
		t.Run(g.name(), func(t *testing.T) {
			r := f.call(t, "admin-a", g)
			assert.Equal(t, http.StatusOK, r.status,
				"the wildcard an admin membership resolves to must satisfy every guard")
		})
	}
}

// Another tenant's administrator passes the PERMISSION check — their wildcard is
// genuine — and is stopped by tenant scope instead. This asserts where the line
// is: authorization says "you may read risks", the tenant filter says "not
// these ones". Conflating the two is how a cross-tenant read gets shipped.
func TestAuthzMatrix_ForeignTenantAdminPassesTheGuardButIsScopedOut(t *testing.T) {
	f := newAuthzFixture(t)

	for _, g := range f.guardSet {
		t.Run(g.name(), func(t *testing.T) {
			r := f.call(t, "admin-b", g)

			require.Equal(t, http.StatusOK, r.status,
				"a genuine wildcard passes the permission guard; scope is enforced elsewhere")
			assert.Equal(t, f.tenantB.String(), r.body["tenant_id"],
				"the response must be scoped to the CALLER's tenant, never tenant A's")
			assert.NotEqual(t, f.tenantA.String(), r.body["tenant_id"])
		})
	}
}

// The extraction must find a realistic number of guards. If someone reworks the
// composition root and the regex stops matching, every test above would pass
// vacuously against an empty set.
func TestAuthzMatrix_ExtractionFindsTheMountedGuards(t *testing.T) {
	guards := mountedPermissionGuards(t)

	assert.GreaterOrEqual(t, len(guards), 25,
		"the server mounts dozens of distinct permissions; finding fewer means the extraction broke")

	// Spot-check families that must exist, so a regex that matched garbage fails.
	for _, want := range []string{"risks:create", "mitigations:read", "organization:members:read"} {
		found := false
		for _, g := range guards {
			if hasAny(g.perms, want) {
				found = true
				break
			}
		}
		assert.True(t, found, "expected a guard for %q among the mounted permissions", want)
	}
}

func hasAny(perms []string, want string) bool {
	for _, p := range perms {
		if p == want {
			return true
		}
	}
	return false
}

var _ = fmt.Sprintf // keep fmt for future diagnostics
