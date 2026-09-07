// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/opendefender/openrisk/internal/domain"
	"github.com/opendefender/openrisk/internal/testsupport/sqliteschema"
)

// The collection routes of /analytics/* and /dashboard/* (#421).
//
// These are the shape that leaked: a GET with no id in its path, whose answer is
// an aggregate over a whole table. A parameterised route that forgets its check
// hands over one row the caller had to name. One of these, having lost its
// WHERE tenant_id, hands over EVERY tenant's numbers to whoever opens the
// dashboard — which is what GET /timeline/recent did (docs/JOURNAL.md item 36).
//
// So every test here has one shape: seed the same qualifying rows in two
// tenants, ask as tenant A, and prove tenant B's rows are absent from the
// answer. Asserting "A sees its own" is not enough — a query with no predicate
// at all passes that. The assertion that matters is the count tenant B's rows
// would have changed.

var (
	crtA = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	crtB = uuid.MustParse("22222222-2222-2222-2222-222222222222")
)

// newCollectionDB reconciles the full models, because GetTopRisks and
// GetMitigationProgress load domain.Risk / domain.Mitigation whole (Preload
// included) rather than selecting the handful of columns they display.
func newCollectionDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	for _, ddl := range []string{
		`CREATE TABLE risks (id TEXT PRIMARY KEY)`,
		`CREATE TABLE mitigations (id TEXT PRIMARY KEY)`,
		// domain.Risk has an AfterSave hook that appends to risk_histories, and
		// GetTopRisks reads that table for its trend, so the fixture carries it
		// rather than disabling the hook.
		`CREATE TABLE risk_histories (id TEXT PRIMARY KEY)`,
		`CREATE TABLE teams (id TEXT PRIMARY KEY)`,
	} {
		require.NoError(t, db.Exec(ddl).Error)
	}
	// Two columns the aggregation SQL references that domain.Risk does not
	// declare — GetSeverityDistribution filters on `severity`, and
	// GetFrameworkAnalytics groups on singular `framework` while the model has
	// `frameworks pq.StringArray`. Reconcile only adds what the MODEL declares,
	// so the fixture adds these by hand to keep the queries executable here.
	// Their absence from the model is a separate defect (those two endpoints
	// swallow the resulting error and answer zero in production); it is not an
	// isolation defect and is out of scope for #421.
	for _, col := range []string{
		`ALTER TABLE risks ADD COLUMN severity TEXT`,
		`ALTER TABLE risks ADD COLUMN framework TEXT`,
	} {
		require.NoError(t, db.Exec(col).Error)
	}
	for _, m := range []struct {
		table string
		model any
	}{
		{"risks", &domain.Risk{}},
		{"mitigations", &domain.Mitigation{}},
		{"risk_histories", &domain.RiskHistory{}},
		{"teams", &domain.Team{}},
	} {
		require.NoError(t, sqliteschema.Reconcile(db, m.table, m.model))
	}
	return db
}

func seedCollectionRisk(t *testing.T, db *gorm.DB, tenant uuid.UUID, title string, score, impact, probability float64, status string, created time.Time) uuid.UUID {
	t.Helper()
	id := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO risks (id, tenant_id, title, status, level, severity, framework, score, impact, probability, criticality, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		id.String(), tenant.String(), title, status, "high", "high", "ISO27001",
		score, impact, probability, "high", created, created,
	).Error)
	return id
}

func seedCollectionMitigation(t *testing.T, db *gorm.DB, tenant, riskID uuid.UUID, title string, status string, created time.Time) {
	t.Helper()
	due := created.AddDate(0, 0, 30)
	require.NoError(t, db.Exec(
		`INSERT INTO mitigations (id, tenant_id, risk_id, title, status, progress, due_date, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?)`,
		uuid.NewString(), tenant.String(), riskID.String(), title, status, 50, due, created, created,
	).Error)
}

// seedTwoTenants gives A one risk with one mitigation, and B three risks with
// three mitigations. Every count below is therefore wrong in a specific,
// recognisable way if the predicate is dropped: A's totals become 4, not 1.
func seedTwoTenants(t *testing.T, db *gorm.DB) {
	t.Helper()
	now := time.Now().UTC()

	// The status strings are the LOWERCASE ones these two services actually
	// match on ("in_progress", "completed") rather than the domain vocabulary
	// ("IN_PROGRESS", "DONE"). The mismatch is a real counting defect — in
	// production these tiles read zero — but it is not an isolation defect, and
	// seeding what the query matches is what makes the isolation assertion mean
	// something: a counter stuck at zero cannot prove a predicate holds.
	rA := seedCollectionRisk(t, db, crtA, "A-only risk", 8.5, 8, 0.9, "active", now.AddDate(0, 0, -2))
	seedCollectionMitigation(t, db, crtA, rA, "A-only mitigation", "in_progress", now.AddDate(0, 0, -1))

	for i, title := range []string{"B-one", "B-two", "B-three"} {
		rB := seedCollectionRisk(t, db, crtB, title, 9.5, 9, 1.0, "active", now.AddDate(0, 0, -i-1))
		seedCollectionMitigation(t, db, crtB, rB, title+" mitigation", "completed", now.AddDate(0, 0, -1))
	}
}

// ---------------------------------------------------------------------------
// AnalyticsService — /analytics/risks/trends, /analytics/mitigations/metrics,
// /analytics/frameworks, /analytics/dashboard, /analytics/export
// ---------------------------------------------------------------------------

func TestAnalyticsCollections_NeverCrossTenants(t *testing.T) {
	db := newCollectionDB(t)
	seedTwoTenants(t, db)
	svc := NewAnalyticsService(db)
	ctx := context.Background()

	t.Run("risk_trends", func(t *testing.T) {
		points, err := svc.GetRiskTrends(ctx, crtA, 30)
		require.NoError(t, err)
		require.NotEmpty(t, points, "the trend must be computed, not skipped")
		// Each bucket is a running total over the tenant's own register. If the
		// predicate were gone the last bucket would carry all four risks.
		require.Equal(t, int64(1), points[len(points)-1].Count,
			"tenant B's three risks must not appear in tenant A's trend")
		require.InDelta(t, 8.5, points[len(points)-1].AvgScore, 0.001,
			"the average score must be over tenant A's register alone")
	})

	t.Run("mitigation_metrics", func(t *testing.T) {
		m, err := svc.GetMitigationMetrics(ctx, crtA)
		require.NoError(t, err)
		require.Equal(t, int64(1), m.TotalMitigations,
			"tenant A owns exactly one mitigation; tenant B's three must be absent")
		require.Equal(t, int64(0), m.CompletedMitigations,
			"tenant B's completed mitigations must not be counted as tenant A's")

		mB, err := svc.GetMitigationMetrics(ctx, crtB)
		require.NoError(t, err)
		require.Equal(t, int64(3), mB.TotalMitigations)
		require.Equal(t, int64(3), mB.CompletedMitigations)
		require.Equal(t, int64(0), mB.PendingMitigations,
			"tenant A's in-progress mitigation must not be counted as tenant B's")
	})

	t.Run("frameworks", func(t *testing.T) {
		fa, err := svc.GetFrameworkAnalytics(ctx, crtA)
		require.NoError(t, err)
		var total int64
		for _, f := range fa {
			total += f.AssociatedRisks
		}
		require.Equal(t, int64(1), total,
			"framework coverage is a count of the tenant's own risks")
	})

	t.Run("dashboard_snapshot_and_export", func(t *testing.T) {
		// GET /analytics/dashboard and GET /analytics/export return the same
		// object: the export handler calls GetDashboardSnapshot and re-encodes it.
		snap, err := svc.GetDashboardSnapshot(ctx, crtA)
		require.NoError(t, err)
		require.Equal(t, int64(1), snap.RiskMetrics.TotalRisks)
		require.Equal(t, int64(1), snap.MitigationMetrics.TotalMitigations)
	})

	t.Run("no_tenant_returns_nothing", func(t *testing.T) {
		// uuid.Nil is what the handler's context helper yields when no tenant
		// resolves. It must read as an empty tenant, never as "all tenants".
		m, err := svc.GetRiskMetrics(ctx, uuid.Nil)
		require.NoError(t, err)
		require.Equal(t, int64(0), m.TotalRisks)
		mm, err := svc.GetMitigationMetrics(ctx, uuid.Nil)
		require.NoError(t, err)
		require.Equal(t, int64(0), mm.TotalMitigations)
	})
}

// ---------------------------------------------------------------------------
// DashboardDataService — /dashboard/metrics, /dashboard/risk-trends,
// /dashboard/mitigation-status, /dashboard/top-risks,
// /dashboard/mitigation-progress, /dashboard/complete
// ---------------------------------------------------------------------------

func TestDashboardCollections_NeverCrossTenants(t *testing.T) {
	db := newCollectionDB(t)
	seedTwoTenants(t, db)
	svc := NewDashboardDataService(db, nil)
	ctx := context.Background()

	t.Run("metrics", func(t *testing.T) {
		m, err := svc.GetDashboardMetrics(ctx, crtA)
		require.NoError(t, err)
		require.Equal(t, int64(1), m.TotalRisks,
			"tenant B's three risks must not be counted in tenant A's KPI tiles")
	})

	t.Run("risk_trends", func(t *testing.T) {
		points, err := svc.GetRiskTrends(ctx, crtA)
		require.NoError(t, err)
		require.NotEmpty(t, points)
		require.Equal(t, int64(1), points[len(points)-1].Count,
			"tenant B's risks must not appear in tenant A's trend")
	})

	t.Run("mitigation_status", func(t *testing.T) {
		s, err := svc.GetMitigationStatus(ctx, crtA)
		require.NoError(t, err)
		require.Equal(t, int64(0), s.Completed,
			"tenant B's three completed mitigations must not surface as tenant A's")
		require.Equal(t, int64(1), s.InProgress)

		sb, err := svc.GetMitigationStatus(ctx, crtB)
		require.NoError(t, err)
		require.Equal(t, int64(3), sb.Completed)
		require.Equal(t, int64(0), sb.InProgress,
			"tenant A's in-progress mitigation must not surface as tenant B's")
	})

	// GET /dashboard/top-risks and GET /dashboard/complete are NOT asserted here,
	// and deliberately so. GetTopRisks calls Preload("Team") on domain.Risk,
	// which declares no Team relation, so GORM refuses the query before it runs
	// and both endpoints answer 500 on every request in every tenant. There is
	// no observable result to make an isolation claim about. They stay Pending in
	// the isolation registry with exactly that reason; the defect is filed
	// separately. This test asserts the refusal so the two facts stay linked: if
	// the relation is ever added, this fails and whoever fixes it is told to
	// write the isolation assertion the route then needs.
	t.Run("top_risks_cannot_execute", func(t *testing.T) {
		_, err := svc.GetTopRisks(ctx, crtA, 10)
		require.Error(t, err, "GetTopRisks preloads a relation domain.Risk does not declare")
		require.Contains(t, err.Error(), "Team")
	})

	t.Run("mitigation_progress", func(t *testing.T) {
		prog, err := svc.GetMitigationProgress(ctx, crtA, 20)
		require.NoError(t, err)
		require.Len(t, prog, 1)
		require.Equal(t, "A-only mitigation", prog[0].Name)
	})

	t.Run("complete_cannot_execute", func(t *testing.T) {
		// GET /dashboard/complete composes the widgets above, GetTopRisks
		// included, so it inherits that refusal wholesale.
		_, err := svc.GetCompleteDashboardData(ctx, crtA)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Team")
	})

	t.Run("no_tenant_returns_nothing", func(t *testing.T) {
		m, err := svc.GetDashboardMetrics(ctx, uuid.Nil)
		require.NoError(t, err)
		require.Equal(t, int64(0), m.TotalRisks)
		st, err := svc.GetMitigationStatus(ctx, uuid.Nil)
		require.NoError(t, err)
		require.Equal(t, int64(0), st.InProgress)
		prog, err := svc.GetMitigationProgress(ctx, uuid.Nil, 20)
		require.NoError(t, err)
		require.Empty(t, prog)
	})
}
