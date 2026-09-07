// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package handler

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	recovermw "github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/opendefender/openrisk/internal/domain"
	"github.com/opendefender/openrisk/internal/middleware"
	authpkg "github.com/opendefender/openrisk/pkg/auth"
)

// ---------------------------------------------------------------------------
// #529 — the guards the audit of the 92 unguarded routes added.
//
// Two assertions per route, because either alone is worthless:
//
//   - the SOURCE assertion ties the guard to the route in cmd/server/main.go.
//     A live test of RequireRole("admin") proves the middleware works; it says
//     nothing about whether anybody attached it to /billing/checkout.
//   - the LIVE assertion drives that same guard with a real member's claims and
//     demands 403 with the handler never reached. A guard that answers 403 after
//     its handler has already read or written has not prevented anything.
//
// The route list here is deliberately hand-written rather than derived: it is
// the audit's verdict, and a derived list would agree with whatever main.go
// happens to say today, including a guard somebody deleted.
// ---------------------------------------------------------------------------

// newlyGuardedRoute is one route that left routesWithoutPermissionGuard.
type newlyGuardedRoute struct {
	method string
	path   string
	// guardSrc is the text that must appear in the registration in main.go. For
	// a guard mounted through a variable it is the variable name, which is why
	// varDecl then pins what that variable is.
	guardSrc string
	// varDecl, when set, is the declaration the guard variable must have.
	varDecl string
	// guard is the live middleware, built exactly as main.go builds it.
	guard fiber.Handler
	why   string
}

func newlyGuardedRoutes() []newlyGuardedRoute {
	adminGuard := func() fiber.Handler { return middleware.RequireRole("admin", "root") }
	riskRead := func() fiber.Handler { return middleware.RequirePermission("risks:read") }

	rows := []newlyGuardedRoute{
		{"Post", "/billing/checkout", `middleware.RequireRole("admin", "root")`, "", adminGuard(),
			"opens a payment session in the organisation's name"},
		{"Post", "/integrations/:id/test", `middleware.RequireRole("admin", "root")`, "", adminGuard(),
			"makes the server fetch a caller-supplied URL"},
		{"Post", "/custom-fields", "customFieldAdmin", `customFieldAdmin := middleware.RequireRole("admin", "root")`, adminGuard(),
			"defines a field on every risk or asset in the tenant"},
		{"Patch", "/custom-fields/:id", "customFieldAdmin", `customFieldAdmin := middleware.RequireRole("admin", "root")`, adminGuard(),
			"redefines a field already in use"},
		{"Delete", "/custom-fields/:id", "customFieldAdmin", `customFieldAdmin := middleware.RequireRole("admin", "root")`, adminGuard(),
			"drops a field and the values captured under it"},
		{"Post", "/custom-fields/templates/:id/apply", "customFieldAdmin", `customFieldAdmin := middleware.RequireRole("admin", "root")`, adminGuard(),
			"applies a whole template to the tenant's schema"},
		{"Post", "/bulk-operations", `middleware.RequirePermission("risks:update", "risks:delete")`, "",
			middleware.RequirePermission("risks:update", "risks:delete"),
			"a delete job with an empty filter empties the register"},
		{"Get", "/risks/:id/incidents", `middleware.RequirePermission("risks:read")`, "", riskRead(),
			"one risk's incident history is one risk's data"},
		{"Get", "/risks/:id/timeline", "riskTimelineRead", `riskTimelineRead := middleware.RequirePermission("risks:read")`, riskRead(), "one risk's history"},
		{"Get", "/risks/:id/timeline/status-changes", "riskTimelineRead", `riskTimelineRead := middleware.RequirePermission("risks:read")`, riskRead(), "one risk's history"},
		{"Get", "/risks/:id/timeline/score-changes", "riskTimelineRead", `riskTimelineRead := middleware.RequirePermission("risks:read")`, riskRead(), "one risk's history"},
		{"Get", "/risks/:id/timeline/trend", "riskTimelineRead", `riskTimelineRead := middleware.RequirePermission("risks:read")`, riskRead(), "one risk's history"},
		{"Get", "/risks/:id/timeline/changes/:type", "riskTimelineRead", `riskTimelineRead := middleware.RequirePermission("risks:read")`, riskRead(), "one risk's history"},
		{"Get", "/risks/:id/timeline/since/:timestamp", "riskTimelineRead", `riskTimelineRead := middleware.RequirePermission("risks:read")`, riskRead(), "one risk's history"},
	}
	return rows
}

func (r newlyGuardedRoute) key() string { return strings.ToUpper(r.method) + " " + r.path }

// compositionRoot reads cmd/server/main.go once per test.
func compositionRoot(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "cmd", "server", "main.go"))
	require.NoError(t, err)
	return string(raw)
}

// registrationFor returns the full `protected.<Method>("<path>", …)` call text.
func registrationFor(t *testing.T, src string, r newlyGuardedRoute) string {
	t.Helper()
	open := regexp.MustCompile(`protected\.` + r.method + `\(\s*"` + regexp.QuoteMeta(r.path) + `"`).FindStringIndex(src)
	require.NotNil(t, open, "%s is not mounted on the authenticated group at all", r.key())

	depth, i := 0, open[0]
	for ; i < len(src); i++ {
		switch src[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return src[open[0] : i+1]
			}
		}
	}
	t.Fatalf("unterminated registration for %s", r.key())
	return ""
}

// The routes struck from routesWithoutPermissionGuard really do carry the guard
// the audit says they do — asserted against the composition root, not a copy.
func TestNewGuards_AreMountedOnTheRoute(t *testing.T) {
	src := compositionRoot(t)
	for _, r := range newlyGuardedRoutes() {
		t.Run(r.key(), func(t *testing.T) {
			reg := registrationFor(t, src, r)
			assert.Contains(t, reg, r.guardSrc,
				"%s must be mounted with %s — it is guarded because %s", r.key(), r.guardSrc, r.why)
			if r.varDecl != "" {
				assert.Contains(t, src, r.varDecl,
					"the guard variable %s must still be %s", r.guardSrc, r.varDecl)
			}
		})
	}
}

// None of them may be left in the allowlist: a route that is both guarded and
// allowlisted reads as "we decided the session is enough" and is a lie.
func TestNewGuards_AreOutOfTheAllowlist(t *testing.T) {
	allowed := map[string]bool{}
	for _, k := range routesWithoutPermissionGuard {
		allowed[k] = true
	}
	for _, r := range newlyGuardedRoutes() {
		assert.False(t, allowed[r.key()],
			"%s now carries a guard and must not remain in routesWithoutPermissionGuard", r.key())
	}
}

// guardFixture drives one guard with real claims through the real middleware.
type guardFixture struct {
	app     *fiber.App
	reached map[string]bool
	tenant  uuid.UUID
}

func newGuardFixture(t *testing.T) *guardFixture {
	t.Helper()
	f := &guardFixture{reached: map[string]bool{}, tenant: uuid.New()}

	app := fiber.New(fiber.Config{DisableStartupMessage: true})

	// Stands in for middleware.Protected: same 401, same stamps, nothing more.
	protected := app.Group("/api/v1", func(c *fiber.Ctx) error {
		switch c.Get("X-Test-Actor") {
		case "admin":
			f.stamp(c, []string{"*"}, "admin")
		case "member":
			// A real, legitimate account: the Viewer preset's rights minus the
			// one under test. Authenticated and entitled to nothing here.
			f.stamp(c, []string{"compliance:read", "assets:read"}, "user")
		default:
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
		}
		return c.Next()
	})

	for _, r := range newlyGuardedRoutes() {
		row := r
		protected.Add(strings.ToUpper(row.method), "/g/"+guardSlug(row), row.guard, func(c *fiber.Ctx) error {
			f.reached[row.key()] = true
			return c.JSON(fiber.Map{"secret": "tenant-scoped-payload"})
		})
	}

	f.app = app
	return f
}

func (f *guardFixture) stamp(c *fiber.Ctx, perms []string, role string) {
	user := uuid.New()
	roles := map[uuid.UUID]string{f.tenant: role}
	c.Locals("permissions", perms)
	c.Locals("org_roles", roles)
	c.Locals("user", &authpkg.Claims{Sub: user, TenantID: f.tenant, Permissions: perms, OrgRoles: roles})
	middleware.SetContext(c, &middleware.RequestContext{UserID: user, OrganizationID: f.tenant})
}

// guardSlug turns a route into a path segment with no parameters, so the test
// route matches on the verb and the guard alone.
func guardSlug(r newlyGuardedRoute) string {
	return strings.NewReplacer("/", "_", ":", "-").Replace(strings.ToLower(r.method) + r.path)
}

func (f *guardFixture) call(t *testing.T, actor string, r newlyGuardedRoute) (int, string) {
	t.Helper()
	f.reached[r.key()] = false

	req := httptest.NewRequest(strings.ToUpper(r.method), "/api/v1/g/"+guardSlug(r), nil)
	if actor != "" {
		req.Header.Set("X-Test-Actor", actor)
	}
	resp, err := f.app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	buf := make([]byte, 2048)
	n, _ := resp.Body.Read(buf)
	return resp.StatusCode, string(buf[:n])
}

// The live denial: an ordinary member is refused, and the handler never runs.
func TestNewGuards_DenyAuthenticatedMemberWithoutTheRight(t *testing.T) {
	f := newGuardFixture(t)
	for _, r := range newlyGuardedRoutes() {
		t.Run(r.key(), func(t *testing.T) {
			status, body := f.call(t, "member", r)
			assert.Equal(t, fiber.StatusForbidden, status,
				"%s must refuse an authenticated member with no right to it (%s)", r.key(), r.why)
			assert.False(t, f.reached[r.key()],
				"%s: the guard let execution reach the handler — a 403 after the work is done is not a refusal", r.key())
			assert.NotContains(t, body, "tenant-scoped-payload",
				"%s: the refusal body leaked the handler's payload", r.key())
		})
	}
}

func TestNewGuards_DenyAnonymous(t *testing.T) {
	f := newGuardFixture(t)
	for _, r := range newlyGuardedRoutes() {
		t.Run(r.key(), func(t *testing.T) {
			status, _ := f.call(t, "", r)
			assert.Equal(t, fiber.StatusUnauthorized, status, "%s must refuse an anonymous caller", r.key())
			assert.False(t, f.reached[r.key()], "%s: anonymous execution reached the handler", r.key())
		})
	}
}

// The other half of a guard: it must still ADMIT the people who need it, or the
// audit has traded a security hole for an outage.
func TestNewGuards_AdmitAnAdministrator(t *testing.T) {
	f := newGuardFixture(t)
	for _, r := range newlyGuardedRoutes() {
		t.Run(r.key(), func(t *testing.T) {
			status, _ := f.call(t, "admin", r)
			assert.Equal(t, fiber.StatusOK, status, "%s must still admit an org admin", r.key())
			assert.True(t, f.reached[r.key()], "%s: the admin was let past but the handler did not run", r.key())
		})
	}
}

// Every business role preset holds risks:read, so the risk-history guards are a
// consistency fix and not a lockout. Stated as a test because the claim is the
// reason those seven routes could be guarded without a migration.
func TestNewGuards_RiskReadIsHeldByEveryBusinessRole(t *testing.T) {
	for _, role := range domain.ListBusinessRoles() {
		var found bool
		for _, p := range role.Permissions {
			if p == "risks:read" {
				found = true
				break
			}
		}
		assert.True(t, found,
			"business role %q lost risks:read — the guards on /risks/:id/timeline* now lock it out", role.Key)
	}
}

// ---------------------------------------------------------------------------
// #529 — the half of the bulk-operations guard that middleware cannot do.
//
// The route's guard is a floor: hold risks:update OR risks:delete and you get
// past it. Which mutation you actually asked for is in the BODY, so the exact
// permission is checked in the handler. Without this second check, anyone
// entitled to bulk-EDIT risks could bulk-DELETE them — and `delete` with an
// empty filter takes the whole register.
//
// The handler is built with a nil service on purpose: a refusal that reaches
// the service has already failed, and a nil pointer is the loudest possible way
// to find that out.
// ---------------------------------------------------------------------------

func TestBulkOperation_RefusesTheVerbTheCallerMayNotPerform(t *testing.T) {
	cases := []struct {
		name      string
		perms     []string
		operation string
		want      int
	}{
		{"editor cannot delete", []string{"risks:update"}, "delete", fiber.StatusForbidden},
		{"deleter cannot edit", []string{"risks:delete"}, "update", fiber.StatusForbidden},
		{"editor cannot assign mitigations", []string{"risks:update"}, "assign_mitigation", fiber.StatusForbidden},
		{"reader cannot export through a job", []string{"assets:read"}, "export", fiber.StatusForbidden},
		{"an unknown verb is rejected", []string{"*"}, "purge", fiber.StatusBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := &BulkOperationHandler{} // nil service: the refusal must never reach it
			app := fiber.New(fiber.Config{DisableStartupMessage: true})
			tenant, user := uuid.New(), uuid.New()
			app.Post("/bulk-operations", func(c *fiber.Ctx) error {
				c.Locals("permissions", tc.perms)
				c.Locals("user", &authpkg.Claims{Sub: user, TenantID: tenant, Permissions: tc.perms})
				middleware.SetContext(c, &middleware.RequestContext{UserID: user, OrganizationID: tenant})
				return c.Next()
			}, h.CreateBulkOperation)

			req := httptest.NewRequest("POST", "/bulk-operations",
				strings.NewReader(`{"operation_type":"`+tc.operation+`","resource_type":"risk"}`))
			req.Header.Set("Content-Type", "application/json")
			resp, err := app.Test(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, tc.want, resp.StatusCode,
				"a %q job asked for by a caller holding %v", tc.operation, tc.perms)
		})
	}
}

// The mirror: the right permission gets through the check. The nil service
// panics the instant execution passes the check, which fiber's recover turns
// into a 500 — so "not 403, not 400" is exactly the shape of a caller who was
// let through.
func TestBulkOperation_AdmitsTheVerbTheCallerMayPerform(t *testing.T) {
	for _, tc := range []struct {
		perm      string
		operation string
	}{
		{"risks:delete", "delete"},
		{"risks:update", "update"},
		{"risks:read", "export"},
		{"mitigations:update", "assign_mitigation"},
	} {
		t.Run(tc.operation, func(t *testing.T) {
			h := &BulkOperationHandler{}
			app := fiber.New(fiber.Config{DisableStartupMessage: true})
			app.Use(recovermw.New())
			tenant, user := uuid.New(), uuid.New()
			perms := []string{tc.perm}
			app.Post("/bulk-operations", func(c *fiber.Ctx) error {
				c.Locals("permissions", perms)
				c.Locals("user", &authpkg.Claims{Sub: user, TenantID: tenant, Permissions: perms})
				middleware.SetContext(c, &middleware.RequestContext{UserID: user, OrganizationID: tenant})
				return c.Next()
			}, h.CreateBulkOperation)

			req := httptest.NewRequest("POST", "/bulk-operations",
				strings.NewReader(`{"operation_type":"`+tc.operation+`","resource_type":"risk"}`))
			req.Header.Set("Content-Type", "application/json")
			resp, err := app.Test(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.NotEqual(t, fiber.StatusForbidden, resp.StatusCode,
				"%s holds %s and must not be refused the %q job", tc.operation, tc.perm, tc.operation)
			assert.NotEqual(t, fiber.StatusBadRequest, resp.StatusCode,
				"%q is a valid operation type", tc.operation)
		})
	}
}
