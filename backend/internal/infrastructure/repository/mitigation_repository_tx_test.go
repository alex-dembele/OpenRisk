// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package repository

import (
	"testing"

	"github.com/google/uuid"
	"github.com/opendefender/openrisk/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupMitigationTxRepo builds the two tables the create path writes to.
//
// uniqueTitles adds a UNIQUE index on (mitigation_id, title). Production has no
// such constraint — it exists only so a test can make one specific insert in
// the middle of the checklist fail, which is the condition #335 is about. There
// is no way to prove a rollback without a write that fails.
func setupMitigationTxRepo(t *testing.T, uniqueTitles bool) (*GormMitigationRepository, *gorm.DB) {
	t.Helper()

	dsn := "file:mitigation_tx_" + uuid.New().String() + "?mode=memory&cache=private"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)

	require.NoError(t, db.Exec(`CREATE TABLE mitigations (
		id TEXT PRIMARY KEY, tenant_id TEXT, risk_id TEXT,
		title TEXT, description TEXT, status TEXT, priority TEXT,
		owner_id TEXT, assignee_id TEXT, reviewer_id TEXT,
		assigned_to TEXT, progress INTEGER DEFAULT 0,
		reminder_d7_sent_at DATETIME, reminder_d1_sent_at DATETIME,
		created_by TEXT, approved_by TEXT, approved_at DATETIME,
		source TEXT, auto_detected_at DATETIME, scanner_config_id TEXT,
		organization_id TEXT, assignee TEXT, cost INTEGER, mitigation_time INTEGER, due_date DATETIME,
		created_at DATETIME, updated_at DATETIME, deleted_at DATETIME
	);`).Error)

	require.NoError(t, db.Exec(`CREATE TABLE mitigation_subactions (
		id TEXT PRIMARY KEY, mitigation_id TEXT, title TEXT, description TEXT,
		completed BOOLEAN DEFAULT 0, completed_at DATETIME, completed_by TEXT,
		completed_source TEXT, auto_detected_at DATETIME, depends_on TEXT,
		"order" INTEGER DEFAULT 0, due_date DATETIME,
		created_at DATETIME, updated_at DATETIME, deleted_at DATETIME
	);`).Error)

	if uniqueTitles {
		require.NoError(t, db.Exec(
			`CREATE UNIQUE INDEX ux_subaction_title ON mitigation_subactions (mitigation_id, title);`,
		).Error)
	}

	return &GormMitigationRepository{db: db}, db
}

func newTxPlan(tenantID uuid.UUID) *domain.Mitigation {
	return &domain.Mitigation{
		ID:        uuid.New(),
		TenantID:  tenantID,
		RiskID:    uuid.New(),
		Title:     "Patch the vulnerable dependency",
		Status:    domain.MitigationPlanned,
		CreatedBy: uuid.New(),
	}
}

func newTxSubActions(titles ...string) []*domain.MitigationSubAction {
	out := make([]*domain.MitigationSubAction, 0, len(titles))
	for i, title := range titles {
		out = append(out, &domain.MitigationSubAction{ID: uuid.New(), Title: title, Order: i})
	}
	return out
}

func countRows(t *testing.T, db *gorm.DB, table string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Table(table).Count(&n).Error)
	return n
}

// Criterion 1 — a valid plan with three sub-actions commits whole.
func TestCreateWithSubActions_Success(t *testing.T) {
	repo, db := setupMitigationTxRepo(t, false)
	tenantID := uuid.New()
	plan := newTxPlan(tenantID)
	subs := newTxSubActions("Inventory", "Patch", "Verify")

	require.NoError(t, repo.CreateWithSubActions(tenantID.String(), plan, subs))

	assert.Equal(t, int64(1), countRows(t, db, "mitigations"))
	assert.Equal(t, int64(3), countRows(t, db, "mitigation_subactions"))

	// Every sub-action is bound to the plan by the repository, not by the caller.
	for _, sub := range subs {
		assert.Equal(t, plan.ID, sub.MitigationID)
	}
}

// Criterion 2 — the SECOND sub-action insert fails: the whole write rolls back,
// asserted by querying the database, not by inspecting a mock.
func TestCreateWithSubActions_PartialFailure_RollsBackEverything(t *testing.T) {
	repo, db := setupMitigationTxRepo(t, true)
	tenantID := uuid.New()
	plan := newTxPlan(tenantID)

	// The duplicate title trips the unique index on the second insert.
	subs := newTxSubActions("Inventory", "Inventory", "Verify")

	err := repo.CreateWithSubActions(tenantID.String(), plan, subs)
	require.Error(t, err, "the duplicate sub-action must fail the write")

	assert.Equal(t, int64(0), countRows(t, db, "mitigations"),
		"the plan must not survive a failed checklist insert")
	assert.Equal(t, int64(0), countRows(t, db, "mitigation_subactions"),
		"no sub-action may survive, including the one that inserted before the failure")
}

// Criterion 3 — the plan insert itself fails: no sub-action is persisted.
func TestCreateWithSubActions_PlanInsertFails_NoSubActions(t *testing.T) {
	repo, db := setupMitigationTxRepo(t, false)
	tenantID := uuid.New()

	first := newTxPlan(tenantID)
	require.NoError(t, repo.CreateWithSubActions(tenantID.String(), first, nil))

	// Same primary key: the plan insert fails before the checklist is reached.
	clash := newTxPlan(tenantID)
	clash.ID = first.ID

	err := repo.CreateWithSubActions(tenantID.String(), clash, newTxSubActions("A", "B"))
	require.Error(t, err)

	assert.Equal(t, int64(1), countRows(t, db, "mitigations"), "only the first plan exists")
	assert.Equal(t, int64(0), countRows(t, db, "mitigation_subactions"))
}

// Criterion 6 — a caller may not write a plan into another tenant, and the row
// it does write carries its own tenant.
func TestCreateWithSubActions_Unauthorized_CrossTenantWriteRefused(t *testing.T) {
	repo, db := setupMitigationTxRepo(t, false)
	tenantA, tenantB := uuid.New(), uuid.New()

	// The entity claims tenant B while the caller is acting as tenant A.
	plan := newTxPlan(tenantB)

	err := repo.CreateWithSubActions(tenantA.String(), plan, newTxSubActions("A"))
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrForbidden)

	assert.Equal(t, int64(0), countRows(t, db, "mitigations"))
	assert.Equal(t, int64(0), countRows(t, db, "mitigation_subactions"))
}

// Criterion 6 — a plan written by tenant A is not readable as tenant B.
func TestCreateWithSubActions_TenantIsolation_NotFoundForOtherTenant(t *testing.T) {
	repo, _ := setupMitigationTxRepo(t, false)
	tenantA, tenantB := uuid.New(), uuid.New()

	plan := newTxPlan(tenantA)
	require.NoError(t, repo.CreateWithSubActions(tenantA.String(), plan, newTxSubActions("A", "B")))

	got, err := repo.GetByID(tenantA.String(), plan.ID)
	require.NoError(t, err)
	assert.Equal(t, plan.ID, got.ID)
	assert.Equal(t, tenantA, got.TenantID)

	_, err = repo.GetByID(tenantB.String(), plan.ID)
	require.Error(t, err, "tenant B must not see tenant A's plan")
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

// Guard rails on the arguments themselves.
func TestCreateWithSubActions_RejectsMissingTenantAndNilPlan(t *testing.T) {
	repo, db := setupMitigationTxRepo(t, false)
	tenantID := uuid.New()

	require.Error(t, repo.CreateWithSubActions(tenantID.String(), nil, nil))

	noTenant := newTxPlan(tenantID)
	noTenant.TenantID = uuid.Nil
	require.Error(t, repo.CreateWithSubActions(tenantID.String(), noTenant, nil))

	badTenant := newTxPlan(tenantID)
	require.Error(t, repo.CreateWithSubActions("not-a-uuid", badTenant, nil))

	assert.Equal(t, int64(0), countRows(t, db, "mitigations"))
}
