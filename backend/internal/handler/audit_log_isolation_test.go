// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package handler

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	authpkg "github.com/opendefender/openrisk/pkg/auth"

	"github.com/opendefender/openrisk/internal/domain"
	"github.com/opendefender/openrisk/internal/infrastructure/database"
	"github.com/opendefender/openrisk/internal/service"
	"github.com/opendefender/openrisk/internal/testsupport/sqliteschema"
)

// The HTTP-level regression test for #532.
//
// internal/service/audit_log_isolation_test proves the four QUERIES are scoped.
// This proves the three ROUTES pass the right tenant into them, which is a
// separate claim and the one a reviewer actually cares about: the defect was
// never that the SQL was wrong in isolation, it was that the handler had no
// tenant to give it.
//
// It drives the real handler over a real Fiber app, with the caller's identity
// supplied exactly as middleware.Protected supplies it — a *authpkg.Claims in
// c.Locals("user") — so the tenant travels the production path from token to
// WHERE clause. Nothing in the request can influence it, and that is the
// property under test.

func newAuditRouteDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE audit_logs (id TEXT PRIMARY KEY)`).Error)
	require.NoError(t, sqliteschema.Reconcile(db, "audit_logs", &domain.AuditLog{}))
	return db
}

// auditRouteApp mounts the three routes behind a stub that sets the same local
// middleware.Protected sets. adminOf is the organisation the caller administers.
func auditRouteApp(adminOf uuid.UUID, actor uuid.UUID) *fiber.App {
	h := NewAuditLogHandler()
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("user", &authpkg.Claims{
			Sub:      actor,
			TenantID: adminOf,
			// Administrator of their OWN organisation — the guard the route
			// already had, and the one that was mistaken for isolation.
			OrgRoles: map[uuid.UUID]string{adminOf: "admin"},
		})
		return c.Next()
	})
	app.Get("/audit-logs", h.GetAuditLogs)
	app.Get("/audit-logs/user/:user_id", h.GetUserAuditLogs)
	app.Get("/audit-logs/action/:action", h.GetAuditLogsByAction)
	return app
}

func auditRouteGet(t *testing.T, app *fiber.App, path string) []AuditLogDTO {
	t.Helper()
	resp, err := app.Test(httptest.NewRequest("GET", path, nil), 5000)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, 200, resp.StatusCode, path)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var envelope struct {
		Data []AuditLogDTO `json:"data"`
		Logs []AuditLogDTO `json:"logs"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope))
	if len(envelope.Data) > 0 {
		return envelope.Data
	}
	return envelope.Logs
}

// TestAuditLogRoutes_AdminOfOneOrganisationSeesOnlyItsOwn is the leak, driven
// end to end.
//
// Organisation B is seeded with three events and organisation A with one. Before
// the fix, A's administrator received all four from every one of these routes.
func TestAuditLogRoutes_AdminOfOneOrganisationSeesOnlyItsOwn(t *testing.T) {
	db := newAuditRouteDB(t)
	previous := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previous })

	orgA := uuid.MustParse("aaaa0532-1111-4000-8000-00000000000a")
	orgB := uuid.MustParse("bbbb0532-1111-4000-8000-00000000000b")
	adminA := uuid.New()
	// One person, accounts in both organisations — so a predicate on user_id,
	// which is what /audit-logs/user/:user_id had, separates nothing.
	sharedActor := uuid.New()

	svc := service.NewAuditServiceWithDB(db)
	now := time.Now().UTC()
	seed := func(org uuid.UUID, actor uuid.UUID, action domain.AuditLogAction, msg string) {
		o, a := org, actor
		require.NoError(t, svc.LogAction(&domain.AuditLog{
			TenantID: &o, UserID: &a, Action: action,
			Resource: domain.ResourceAuth, Result: domain.ResultFailure,
			ErrorMessage: msg, Timestamp: now.Add(-time.Minute),
		}))
	}
	seed(orgA, sharedActor, domain.ActionLoginFailed, "A-own-event")
	seed(orgB, sharedActor, domain.ActionLoginFailed, "B-secret-1")
	seed(orgB, sharedActor, domain.ActionLoginFailed, "B-secret-2")
	seed(orgB, uuid.New(), domain.ActionUserDelete, "B-secret-3")

	app := auditRouteApp(orgA, adminA)

	assertNoBSecrets := func(t *testing.T, rows []AuditLogDTO) {
		t.Helper()
		for _, r := range rows {
			require.NotContains(t, r.ErrorMessage, "B-secret",
				"organisation B's audit event reached organisation A's administrator")
		}
	}

	t.Run("GET_/audit-logs", func(t *testing.T) {
		rows := auditRouteGet(t, app, "/audit-logs")
		require.Len(t, rows, 1, "organisation A wrote one event; B's three must be absent")
		require.Contains(t, rows[0].ErrorMessage, "A-own-event")
		assertNoBSecrets(t, rows)
	})

	t.Run("GET_/audit-logs/user/:user_id", func(t *testing.T) {
		// The same actor id in both organisations. A's admin naming it must get
		// A's row and only A's.
		rows := auditRouteGet(t, app, "/audit-logs/user/"+sharedActor.String())
		require.Len(t, rows, 1)
		assertNoBSecrets(t, rows)
	})

	t.Run("GET_/audit-logs/action/:action", func(t *testing.T) {
		// The sharpest of the three: an attacker-chosen action name used to
		// return every tenant's events of that kind, deployment-wide.
		rows := auditRouteGet(t, app, "/audit-logs/action/login_failed")
		require.Len(t, rows, 1, "organisation B's two failed logins must not appear")
		assertNoBSecrets(t, rows)
	})

	t.Run("organisation_B_admin_sees_B", func(t *testing.T) {
		// The mirror case. Without it, a handler that returned nothing at all
		// would pass every assertion above.
		bApp := auditRouteApp(orgB, uuid.New())
		rows := auditRouteGet(t, bApp, "/audit-logs")
		require.Len(t, rows, 3)
		for _, r := range rows {
			require.NotContains(t, r.ErrorMessage, "A-own-event")
		}
	})
}

// A session carrying no organisation must be refused, not served globally.
//
// This is the shape that made the original defect reachable: a caller the route
// could not scope was answered anyway.
func TestAuditLogRoutes_SessionWithNoOrganisationIsRefused(t *testing.T) {
	db := newAuditRouteDB(t)
	previous := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previous })

	org := uuid.New()
	actor := uuid.New()
	svc := service.NewAuditServiceWithDB(db)
	require.NoError(t, svc.LogAction(&domain.AuditLog{
		TenantID: &org, UserID: &actor,
		Action: domain.ActionLogin, Result: domain.ResultSuccess,
		Timestamp: time.Now(),
	}))

	h := NewAuditLogHandler()
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		// A platform-wide "*" holder with no organisation in the token: passes
		// the permission guard, has no tenant to be scoped to.
		c.Locals("user", &authpkg.Claims{Sub: actor, Permissions: []string{"*"}})
		return c.Next()
	})
	app.Get("/audit-logs", h.GetAuditLogs)

	resp, err := app.Test(httptest.NewRequest("GET", "/audit-logs", nil), 5000)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, 401, resp.StatusCode,
		"a session with no organisation must be refused, never answered with every tenant's rows")
}
