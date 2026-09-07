// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package repository

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/opendefender/openrisk/internal/domain"
)

// #582 against real SQL.
//
// The engine tests in application/bulk prove behaviour on a store that keeps its
// promises. These prove the stores keep them: a real ROLLBACK, a strict delete,
// and — the one criterion 2 is actually about — that LoadBulk leaves the rows
// byte-identical, which no fake can demonstrate.
func setupBulkStores(t *testing.T) (*VulnerabilityBulkStore, *AssetBulkStore, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	createTableFromModel(t, db, &domain.Vulnerability{})
	createTableFromModel(t, db, &domain.Asset{})
	return NewVulnerabilityBulkStore(db), NewAssetBulkStore(db), db
}

func seedVuln(t *testing.T, db *gorm.DB, tenant uuid.UUID, cve, status string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO vulnerabilities (id, tenant_id, cve_id, title, status) VALUES (?, ?, ?, ?, ?)`,
		id.String(), tenant.String(), cve, "seed", status).Error)
	return id
}

func seedBulkAsset(t *testing.T, db *gorm.DB, tenant uuid.UUID, name string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO assets (id, tenant_id, name, type, criticality) VALUES (?, ?, ?, ?, ?)`,
		id.String(), tenant.String(), name, "server", "HIGH").Error)
	return id
}

func vulnStatus(t *testing.T, db *gorm.DB, id uuid.UUID) string {
	t.Helper()
	var got string
	require.NoError(t, db.Raw(`SELECT status FROM vulnerabilities WHERE id = ?`, id.String()).Scan(&got).Error)
	return got
}

func liveCount(t *testing.T, db *gorm.DB, table string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM `+table+` WHERE deleted_at IS NULL`).Scan(&n).Error)
	return n
}

// ---------------------------------------------------------------------------
// Criterion 2 — the preview's read is genuinely read-only
// ---------------------------------------------------------------------------

func TestBulkStores_LoadBulk_MutatesNothing(t *testing.T) {
	vulns, assets, db := setupBulkStores(t)
	ctx := context.Background()
	tenant := uuid.New()

	v := seedVuln(t, db, tenant, "CVE-2021-44228", "open")
	a := seedBulkAsset(t, db, tenant, "srv-01")

	var beforeV, beforeA string
	require.NoError(t, db.Raw(`SELECT status FROM vulnerabilities WHERE id = ?`, v.String()).Scan(&beforeV).Error)
	require.NoError(t, db.Raw(`SELECT criticality FROM assets WHERE id = ?`, a.String()).Scan(&beforeA).Error)

	rows, err := vulns.LoadBulk(ctx, tenant, []uuid.UUID{v})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "CVE-2021-44228", rows[0].Label, "the preview names rows, not ids")
	assert.Equal(t, "open", rows[0].Snapshot["status"])

	assetRows, err := assets.LoadBulk(ctx, tenant, []uuid.UUID{a})
	require.NoError(t, err)
	require.Len(t, assetRows, 1)
	assert.Equal(t, "srv-01", assetRows[0].Label)

	var afterV, afterA string
	require.NoError(t, db.Raw(`SELECT status FROM vulnerabilities WHERE id = ?`, v.String()).Scan(&afterV).Error)
	require.NoError(t, db.Raw(`SELECT criticality FROM assets WHERE id = ?`, a.String()).Scan(&afterA).Error)
	assert.Equal(t, beforeV, afterV, "a preview read must leave the row byte-identical")
	assert.Equal(t, beforeA, afterA)
	assert.Equal(t, int64(1), liveCount(t, db, "vulnerabilities"))
	assert.Equal(t, int64(1), liveCount(t, db, "assets"))
}

// LoadBulk omits what the caller may not see, rather than erroring — the engine
// turns the shortfall into Preview.Missing.
func TestBulkStores_LoadBulk_OmitsForeignAndAbsentRows(t *testing.T) {
	vulns, _, db := setupBulkStores(t)
	ctx := context.Background()
	tenantA, tenantB := uuid.New(), uuid.New()
	mine := seedVuln(t, db, tenantA, "CVE-1", "open")
	theirs := seedVuln(t, db, tenantB, "CVE-2", "open")

	rows, err := vulns.LoadBulk(ctx, tenantA, []uuid.UUID{mine, theirs, uuid.New()})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, mine, rows[0].ID)
}

// ---------------------------------------------------------------------------
// All-or-nothing, against real SQL
// ---------------------------------------------------------------------------

func TestBulkStores_ApplyBulk_CommitsTheWholeBatch(t *testing.T) {
	vulns, _, db := setupBulkStores(t)
	ctx := context.Background()
	tenant := uuid.New()
	a := seedVuln(t, db, tenant, "CVE-1", "open")
	b := seedVuln(t, db, tenant, "CVE-2", "open")

	muts, err := vulns.ApplyBulk(ctx, tenant, []uuid.UUID{a, b},
		domain.BulkChange{Action: domain.BulkActionChangeStatus, Status: "triaged"})
	require.NoError(t, err)
	require.Len(t, muts, 2)

	assert.Equal(t, "triaged", vulnStatus(t, db, a))
	assert.Equal(t, "triaged", vulnStatus(t, db, b))
	assert.Equal(t, "open", muts[0].Before["status"])
	assert.Equal(t, "triaged", muts[0].After["status"])
	assert.Contains(t, muts[0].ChangedFields, "status")
	assert.Equal(t, a, muts[0].ID, "every mutation names its row")
}

func TestBulkStores_ApplyBulk_AnAbsentIDRollsBackTheBatch(t *testing.T) {
	vulns, _, db := setupBulkStores(t)
	ctx := context.Background()
	tenant := uuid.New()
	a := seedVuln(t, db, tenant, "CVE-1", "open")

	// The stale id is LAST, so the first row has already been written when the
	// batch fails. Without a transaction this test fails.
	_, err := vulns.ApplyBulk(ctx, tenant, []uuid.UUID{a, uuid.New()},
		domain.BulkChange{Action: domain.BulkActionChangeStatus, Status: "triaged"})

	assert.ErrorIs(t, err, domain.ErrNotFound)
	assert.Equal(t, "open", vulnStatus(t, db, a), "a failure anywhere rolls the whole batch back")
}

func TestBulkStores_ApplyBulk_ForeignTenantIsNotFoundAndUntouched(t *testing.T) {
	vulns, _, db := setupBulkStores(t)
	ctx := context.Background()
	tenantA, tenantB := uuid.New(), uuid.New()
	mine := seedVuln(t, db, tenantA, "CVE-1", "open")
	theirs := seedVuln(t, db, tenantB, "CVE-2", "open")
	change := domain.BulkChange{Action: domain.BulkActionChangeStatus, Status: "triaged"}

	_, foreignErr := vulns.ApplyBulk(ctx, tenantA, []uuid.UUID{theirs}, change)
	_, fabricatedErr := vulns.ApplyBulk(ctx, tenantA, []uuid.UUID{uuid.New()}, change)

	assert.ErrorIs(t, foreignErr, domain.ErrNotFound)
	assert.NotErrorIs(t, foreignErr, domain.ErrForbidden)
	assert.Equal(t, domain.HTTPStatusFromError(fabricatedErr), domain.HTTPStatusFromError(foreignErr))
	assert.Equal(t, domain.MessageFromError(fabricatedErr), domain.MessageFromError(foreignErr))
	assert.Equal(t, "open", vulnStatus(t, db, theirs), "another tenant's row is untouched")

	_, err := vulns.ApplyBulk(ctx, tenantA, []uuid.UUID{mine, theirs}, change)
	assert.ErrorIs(t, err, domain.ErrNotFound)
	assert.Equal(t, "open", vulnStatus(t, db, mine), "a foreign id in the batch modifies nothing")
}

func TestBulkStores_ApplyBulk_RefusesAZeroTenant(t *testing.T) {
	vulns, _, db := setupBulkStores(t)
	ctx := context.Background()
	tenant := uuid.New()
	a := seedVuln(t, db, tenant, "CVE-1", "open")

	_, err := vulns.ApplyBulk(ctx, uuid.Nil, []uuid.UUID{a},
		domain.BulkChange{Action: domain.BulkActionChangeStatus, Status: "triaged"})
	assert.ErrorIs(t, err, domain.ErrForbidden, "a zero tenant must fail closed, never run unscoped")
	assert.Equal(t, "open", vulnStatus(t, db, a))

	_, err = vulns.LoadBulk(ctx, uuid.Nil, []uuid.UUID{a})
	assert.ErrorIs(t, err, domain.ErrForbidden)
}

// ---------------------------------------------------------------------------
// Delete — strict, and it records what went
// ---------------------------------------------------------------------------

func TestBulkStores_DeleteBulk_IsStrictAndAudited(t *testing.T) {
	vulns, _, db := setupBulkStores(t)
	ctx := context.Background()
	tenant := uuid.New()
	a := seedVuln(t, db, tenant, "CVE-1", "open")
	b := seedVuln(t, db, tenant, "CVE-2", "triaged")

	// One stale id deletes nothing at all.
	_, err := vulns.DeleteBulk(ctx, tenant, []uuid.UUID{a, b, uuid.New()})
	assert.ErrorIs(t, err, domain.ErrNotFound)
	assert.Equal(t, int64(2), liveCount(t, db, "vulnerabilities"))

	muts, err := vulns.DeleteBulk(ctx, tenant, []uuid.UUID{a, b})
	require.NoError(t, err)
	require.Len(t, muts, 2)
	assert.Equal(t, int64(0), liveCount(t, db, "vulnerabilities"))

	// The trail records WHAT went — an entry with a nil id or an empty before
	// explains nothing after the fact.
	for _, m := range muts {
		assert.NotEqual(t, uuid.Nil, m.ID)
		assert.NotEmpty(t, m.Label)
		assert.NotEmpty(t, m.Before)
		assert.Equal(t, []string{"deleted"}, m.ChangedFields)
	}
}

func TestBulkStores_DeleteBulk_ForeignTenantDeletesNothing(t *testing.T) {
	vulns, assets, db := setupBulkStores(t)
	ctx := context.Background()
	tenantA, tenantB := uuid.New(), uuid.New()
	mine := seedVuln(t, db, tenantA, "CVE-1", "open")
	theirs := seedVuln(t, db, tenantB, "CVE-2", "open")
	myAsset := seedBulkAsset(t, db, tenantA, "srv-01")
	theirAsset := seedBulkAsset(t, db, tenantB, "srv-02")

	_, err := vulns.DeleteBulk(ctx, tenantA, []uuid.UUID{mine, theirs})
	assert.ErrorIs(t, err, domain.ErrNotFound)
	assert.Equal(t, int64(2), liveCount(t, db, "vulnerabilities"), "neither row may go")

	_, err = assets.DeleteBulk(ctx, tenantA, []uuid.UUID{myAsset, theirAsset})
	assert.ErrorIs(t, err, domain.ErrNotFound)
	assert.Equal(t, int64(2), liveCount(t, db, "assets"))
}

func TestBulkStores_Assets_DeleteOnly(t *testing.T) {
	_, assets, db := setupBulkStores(t)
	ctx := context.Background()
	tenant := uuid.New()
	a := seedBulkAsset(t, db, tenant, "srv-01")

	// The inventory declares delete only — Asset has no Status, no Tags, and a
	// free-text Owner, so nothing else from #581's action set maps onto it.
	assert.Equal(t, []domain.BulkAction{domain.BulkActionDelete}, assets.SupportedBulkActions())

	// And if the engine's guard ever regressed, ApplyBulk refuses rather than
	// silently succeeding.
	_, err := assets.ApplyBulk(ctx, tenant, []uuid.UUID{a},
		domain.BulkChange{Action: domain.BulkActionChangeStatus, Status: "open"})
	assert.ErrorIs(t, err, domain.ErrValidation)
	assert.Equal(t, int64(1), liveCount(t, db, "assets"))

	muts, err := assets.DeleteBulk(ctx, tenant, []uuid.UUID{a})
	require.NoError(t, err)
	require.Len(t, muts, 1)
	assert.Equal(t, "srv-01", muts[0].Label)
	assert.Equal(t, int64(0), liveCount(t, db, "assets"))
}

// The module's own vocabulary is the module's to police.
func TestBulkStores_Vulnerabilities_ValidateStatusVocabulary(t *testing.T) {
	vulns, _, _ := setupBulkStores(t)

	for _, ok := range []string{"open", "triaged", "in_remediation", "remediated", "accepted", "false_positive"} {
		assert.NoError(t, vulns.ValidateBulkChange(
			domain.BulkChange{Action: domain.BulkActionChangeStatus, Status: ok}), ok)
	}
	err := vulns.ValidateBulkChange(
		domain.BulkChange{Action: domain.BulkActionChangeStatus, Status: "resolved"})
	require.ErrorIs(t, err, domain.ErrValidation)
	assert.Contains(t, err.Error(), "false_positive", "the error names the accepted values")

	// A delete carries no status, so the vocabulary check does not apply.
	assert.NoError(t, vulns.ValidateBulkChange(domain.BulkChange{Action: domain.BulkActionDelete}))
}
