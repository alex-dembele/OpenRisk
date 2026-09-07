// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package handler

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/opendefender/openrisk/internal/domain"
	"github.com/opendefender/openrisk/internal/infrastructure/database"
	"github.com/opendefender/openrisk/internal/testsupport/sqliteschema"
)

// The /stats/* collection routes (#421).
//
// The issue names this module as the place to start, and the reason is shape,
// not size: these are unparameterised GETs that aggregate a whole table and
// hand the numbers to whoever loads the dashboard. GET /timeline/recent had the
// same shape and returned every tenant's risk history (docs/JOURNAL.md item 36).
//
// These handlers read their tenant from c.Locals("tenant_id"), which the auth
// middleware sets from the signed token, and query package-global database.DB.
// So the test drives them over a real Fiber app with a stub middleware standing
// in for the token, and swaps database.DB for an in-memory fixture — the same
// code path production runs, tenant resolution included, rather than a retyped
// copy of the SQL.

func newStatsIsolationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	for _, ddl := range []string{
		`CREATE TABLE risks (id TEXT PRIMARY KEY)`,
		`CREATE TABLE mitigations (id TEXT PRIMARY KEY)`,
		`CREATE TABLE risk_histories (id TEXT PRIMARY KEY)`,
		// ExportRisksPDF preloads Risk.Assets (many2many risk_assets), so the
		// join table and its target have to exist for the export to run at all.
		`CREATE TABLE assets (id TEXT PRIMARY KEY)`,
		`CREATE TABLE risk_assets (risk_id TEXT, asset_id TEXT)`,
	} {
		require.NoError(t, db.Exec(ddl).Error)
	}
	for _, m := range []struct {
		table string
		model any
	}{
		{"risks", &domain.Risk{}},
		{"mitigations", &domain.Mitigation{}},
		{"risk_histories", &domain.RiskHistory{}},
		{"assets", &domain.Asset{}},
	} {
		require.NoError(t, sqliteschema.Reconcile(db, m.table, m.model))
	}
	return db
}

// useStatsDB points the package-global handle at the fixture and restores the
// real one, so a failure here cannot leave the rest of the suite pointed at a
// closed in-memory database.
func useStatsDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	previous := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previous })
}

// statsApp mounts the real handlers behind a middleware that sets the same
// locals middleware.Protected sets from the JWT. Nothing in the request can
// influence the tenant — that is the property under test.
func statsApp(tenant uuid.UUID) *fiber.App {
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("tenant_id", tenant)
		c.Locals("userID", uuid.New().String())
		return c.Next()
	})
	app.Get("/stats/risk-matrix", GetRiskMatrixData)
	app.Get("/stats/risk-distribution", GetRiskDistribution)
	app.Get("/stats/mitigation-metrics", GetMitigationMetrics)
	app.Get("/stats/top-vulnerabilities", GetTopVulnerabilities)
	app.Get("/stats/trends", GetGlobalRiskTrend)
	app.Get("/export/pdf", ExportRisksPDF)
	return app
}

func statsGet(t *testing.T, app *fiber.App, path string) []byte {
	t.Helper()
	resp, err := app.Test(httptest.NewRequest("GET", path, nil), 5000)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, 200, resp.StatusCode, path)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return body
}

// pdfText inflates every FlateDecode content stream in the document and returns
// the concatenation, so an assertion can be made about what a reader would see
// rather than about compressed bytes.
func pdfText(t *testing.T, doc []byte) string {
	t.Helper()
	var out strings.Builder
	rest := doc
	for {
		i := bytes.Index(rest, []byte("stream\n"))
		if i < 0 {
			break
		}
		rest = rest[i+len("stream\n"):]
		j := bytes.Index(rest, []byte("\nendstream"))
		if j < 0 {
			break
		}
		if zr, err := zlib.NewReader(bytes.NewReader(rest[:j])); err == nil {
			if raw, err := io.ReadAll(zr); err == nil {
				out.Write(raw)
			}
			_ = zr.Close()
		}
		rest = rest[j:]
	}
	require.NotEmpty(t, out.String(), "no readable content stream in the PDF")
	return out.String()
}

func seedStatsRisk(t *testing.T, db *gorm.DB, tenant uuid.UUID, title string, score, impact, probability float64, level string) {
	t.Helper()
	now := time.Now().UTC()
	require.NoError(t, db.Exec(
		`INSERT INTO risks (id, tenant_id, title, status, level, criticality, score, impact, probability, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		uuid.NewString(), tenant.String(), title, "open", level, level,
		score, impact, probability, now, now,
	).Error)
}

// TestStatsCollections_NeverCrossTenants seeds two tenants and proves each of
// the five /stats reads and the PDF export answers with the caller's rows only.
//
// Tenant A owns one risk and one mitigation; tenant B owns three of each, with
// deliberately different impact/probability/level values. Every count below is
// therefore wrong in a recognisable way if a predicate is dropped.
func TestStatsCollections_NeverCrossTenants(t *testing.T) {
	db := newStatsIsolationDB(t)
	useStatsDB(t, db)

	tenantA := uuid.MustParse("aaaa1111-aaaa-1111-aaaa-111111111111")
	tenantB := uuid.MustParse("bbbb2222-bbbb-2222-bbbb-222222222222")

	seedStatsRisk(t, db, tenantA, "A-only risk", 4.0, 5, 0.8, "MEDIUM")
	for _, title := range []string{"B-one", "B-two", "B-three"} {
		seedStatsRisk(t, db, tenantB, title, 9.0, 9, 1.0, "CRITICAL")
	}
	for i := 0; i < 3; i++ {
		require.NoError(t, db.Exec(
			`INSERT INTO mitigations (id, tenant_id, risk_id, title, status, progress, created_at, updated_at)
			 VALUES (?,?,?,?,?,?,?,?)`,
			uuid.NewString(), tenantB.String(), uuid.NewString(), "B mitigation", "DONE", 100,
			time.Now(), time.Now()).Error)
	}
	require.NoError(t, db.Exec(
		`INSERT INTO mitigations (id, tenant_id, risk_id, title, status, progress, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?)`,
		uuid.NewString(), tenantA.String(), uuid.NewString(), "A mitigation", "PLANNED", 0,
		time.Now(), time.Now()).Error)

	app := statsApp(tenantA)

	t.Run("risk_matrix", func(t *testing.T) {
		var cells []RiskMatrixCell
		require.NoError(t, json.Unmarshal(statsGet(t, app, "/stats/risk-matrix"), &cells))
		require.Len(t, cells, 1, "one (impact, probability) pair exists in tenant A")
		require.Equal(t, 1, cells[0].Count)
		for _, c := range cells {
			require.NotEqual(t, float64(9), c.Impact, "tenant B's cell reached tenant A's matrix")
		}
	})

	t.Run("risk_distribution", func(t *testing.T) {
		var rows []RiskDistributionData
		require.NoError(t, json.Unmarshal(statsGet(t, app, "/stats/risk-distribution"), &rows))
		require.Len(t, rows, 1)
		require.Equal(t, "MEDIUM", rows[0].Level)
		require.Equal(t, 1, rows[0].Count)
		for _, r := range rows {
			require.NotEqual(t, "CRITICAL", r.Level, "tenant B's severity band reached tenant A")
		}
	})

	t.Run("mitigation_metrics", func(t *testing.T) {
		var m MitigationMetricsData
		require.NoError(t, json.Unmarshal(statsGet(t, app, "/stats/mitigation-metrics"), &m))
		require.Equal(t, 1, m.TotalMitigations, "tenant B's three mitigations must be absent")
		require.Equal(t, 0, m.CompletedMitigations,
			"tenant B's DONE mitigations must not be counted as tenant A's completions")
		require.Equal(t, 1, m.PlannedMitigations)
	})

	t.Run("top_vulnerabilities", func(t *testing.T) {
		var rows []TopVulnerability
		require.NoError(t, json.Unmarshal(statsGet(t, app, "/stats/top-vulnerabilities"), &rows))
		require.Len(t, rows, 1)
		require.Equal(t, "A-only risk", rows[0].Title)
		for _, r := range rows {
			require.NotContains(t, r.Title, "B-",
				"a tenant B risk reached tenant A's top-vulnerabilities widget")
		}
	})

	t.Run("trends", func(t *testing.T) {
		var points []TrendPoint
		require.NoError(t, json.Unmarshal(statsGet(t, app, "/stats/trends"), &points))
		require.Len(t, points, 1, "one creation day in tenant A")
		// The average is over tenant A's single risk. Tenant B's three 9.0s
		// would pull it to 7.75.
		require.InDelta(t, 4.0, points[0].Score, 0.001,
			"tenant B's scores must not enter tenant A's average")
	})

	t.Run("export_pdf", func(t *testing.T) {
		// The export renders every row it fetched into the document, so a lost
		// predicate here ships another tenant's register as a downloadable file.
		// The assertion is made on the rendered text, not on the row count, for
		// the same reason the rest of this file drives real handlers: a leak is
		// only real once it reaches the artifact the user receives.
		text := pdfText(t, statsGet(t, app, "/export/pdf"))
		require.Contains(t, text, "A-only risk")
		for _, title := range []string{"B-one", "B-two", "B-three"} {
			require.NotContains(t, text, title,
				"a tenant B risk was rendered into tenant A's PDF export")
		}
	})
}

// A request that resolves no tenant must read as an empty tenant, never as
// every tenant. The handlers take their tenant through safeGetUUID, which
// answers uuid.Nil when the local is absent; the predicate must still be
// emitted, so the answer is empty rather than global.
func TestStatsCollections_NoTenantReadsAsEmptyNotGlobal(t *testing.T) {
	db := newStatsIsolationDB(t)
	useStatsDB(t, db)

	tenant := uuid.New()
	seedStatsRisk(t, db, tenant, "somebody's risk", 9.0, 9, 1.0, "CRITICAL")

	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		// No tenant_id local at all — the shape a route mounted outside the
		// authenticated router would produce.
		c.Locals("userID", uuid.New().String())
		return c.Next()
	})
	app.Get("/stats/risk-distribution", GetRiskDistribution)
	app.Get("/stats/top-vulnerabilities", GetTopVulnerabilities)

	var rows []RiskDistributionData
	require.NoError(t, json.Unmarshal(statsGet(t, app, "/stats/risk-distribution"), &rows))
	require.Empty(t, rows, "an unresolved tenant must see nothing, not everything")

	var top []TopVulnerability
	require.NoError(t, json.Unmarshal(statsGet(t, app, "/stats/top-vulnerabilities"), &top))
	require.Empty(t, top, "an unresolved tenant must see nothing, not everything")
}
