// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package repository

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/opendefender/openrisk/internal/domain"
)

// #581 criterion 1 — all-or-nothing.
//
// The use-case tests in application/risk prove the use case behaves correctly on
// top of a store that keeps its promise. THIS file proves the store keeps it:
// that BulkApply issues a real ROLLBACK, so a failure halfway through a batch
// leaves the rows written before it untouched. A fake repository cannot show
// that, and it is the exact property the old code claimed in a comment and did
// not have.
func setupRiskBulkRepo(t *testing.T) (*GormRiskRepository, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// The fixture schema is DERIVED from the model rather than hand-written.
	//
	// Two dead ends first, so nobody repeats them: a sparse hand-written table
	// fails because GORM's Save writes EVERY column of domain.Risk, and
	// AutoMigrate fails because that model's DDL is Postgres-specific. Deriving
	// the columns from GORM's own parsed schema gives a table that always has
	// exactly the columns the code under test writes, and cannot drift from the
	// model. Risk.AfterSave writes a history row in the same transaction, so that
	// table is created the same way.
	createTableFromModel(t, db, &domain.Risk{})
	createTableFromModel(t, db, &domain.RiskHistory{})

	return NewGormRiskRepository(db), db
}

// seedBulkRisk inserts one row through raw SQL, so the fixture does not depend
// on the very write path under test.
func seedBulkRisk(t *testing.T, db *gorm.DB, tenant uuid.UUID, status string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO risks (id, tenant_id, title, status, lifecycle_state, lifecycle_phase, tags)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id.String(), tenant.String(), "seed", status, "identified", "identify", "{}",
	).Error)
	return id
}

func statusOf(t *testing.T, db *gorm.DB, id uuid.UUID) string {
	t.Helper()
	var got string
	require.NoError(t, db.Raw(`SELECT status FROM risks WHERE id = ?`, id.String()).Scan(&got).Error)
	return got
}

func riskCount(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM risks WHERE deleted_at IS NULL`).Scan(&n).Error)
	return n
}

// The central claim: a failure on the LAST item undoes the writes already made
// for the first two. Without a transaction this test fails — which is precisely
// the state master was in.
func TestBulkApply_RollsBackEveryWriteWhenOneItemFails(t *testing.T) {
	repo, db := setupRiskBulkRepo(t)
	ctx := context.Background()
	tenant := uuid.New()

	a := seedBulkRisk(t, db, tenant, "open")
	b := seedBulkRisk(t, db, tenant, "open")
	c := seedBulkRisk(t, db, tenant, "open")

	boom := errors.New("refused on the third")
	_, err := repo.BulkApply(ctx, tenant, []uuid.UUID{a, b, c}, func(r *domain.Risk) error {
		if r.ID == c {
			return boom
		}
		r.Status = domain.RiskMitigated
		return nil
	})

	require.ErrorIs(t, err, boom)
	for _, id := range []uuid.UUID{a, b, c} {
		assert.Equal(t, "open", statusOf(t, db, id),
			"a failure anywhere in the batch must leave every row as it was")
	}
}

func TestBulkApply_CommitsTheWholeBatchOnSuccess(t *testing.T) {
	repo, db := setupRiskBulkRepo(t)
	ctx := context.Background()
	tenant := uuid.New()

	a := seedBulkRisk(t, db, tenant, "open")
	b := seedBulkRisk(t, db, tenant, "open")

	mutations, err := repo.BulkApply(ctx, tenant, []uuid.UUID{a, b}, func(r *domain.Risk) error {
		r.Status = domain.RiskMitigated
		return nil
	})
	require.NoError(t, err)
	require.Len(t, mutations, 2)

	for _, id := range []uuid.UUID{a, b} {
		assert.Equal(t, string(domain.RiskMitigated), statusOf(t, db, id))
	}
	// The before → after the audit trail is built from is real, not a placeholder.
	assert.Equal(t, "open", mutations[0].Before["status"])
	assert.Equal(t, string(domain.RiskMitigated), mutations[0].After["status"])
	assert.Contains(t, mutations[0].ChangedFields, "status")
}

// A stale id — the selection was made before somebody else deleted a row — must
// fail the batch rather than quietly skipping it.
func TestBulkApply_AnAbsentIDFailsTheBatchAndWritesNothing(t *testing.T) {
	repo, db := setupRiskBulkRepo(t)
	ctx := context.Background()
	tenant := uuid.New()
	a := seedBulkRisk(t, db, tenant, "open")

	_, err := repo.BulkApply(ctx, tenant, []uuid.UUID{a, uuid.New()}, func(r *domain.Risk) error {
		r.Status = domain.RiskMitigated
		return nil
	})

	assert.ErrorIs(t, err, domain.ErrNotFound)
	assert.Equal(t, "open", statusOf(t, db, a))
}

// Criterion 4 at the storage boundary: the tenant predicate is on the load, so a
// foreign id cannot be told apart from one that never existed.
func TestBulkApply_ForeignTenantIDIsNotFoundAndUntouched(t *testing.T) {
	repo, db := setupRiskBulkRepo(t)
	ctx := context.Background()
	tenantA, tenantB := uuid.New(), uuid.New()
	mine := seedBulkRisk(t, db, tenantA, "open")
	theirs := seedBulkRisk(t, db, tenantB, "open")

	_, foreignErr := repo.BulkApply(ctx, tenantA, []uuid.UUID{theirs}, func(r *domain.Risk) error {
		r.Status = domain.RiskMitigated
		return nil
	})
	_, fabricatedErr := repo.BulkApply(ctx, tenantA, []uuid.UUID{uuid.New()}, func(r *domain.Risk) error {
		return nil
	})

	assert.ErrorIs(t, foreignErr, domain.ErrNotFound)
	assert.NotErrorIs(t, foreignErr, domain.ErrForbidden)
	assert.Equal(t, domain.HTTPStatusFromError(fabricatedErr), domain.HTTPStatusFromError(foreignErr))
	assert.Equal(t, domain.MessageFromError(fabricatedErr), domain.MessageFromError(foreignErr))
	assert.Equal(t, "open", statusOf(t, db, theirs), "another tenant's row must be untouched")

	// Mixing a foreign id into an otherwise valid batch modifies nothing.
	_, err := repo.BulkApply(ctx, tenantA, []uuid.UUID{mine, theirs}, func(r *domain.Risk) error {
		r.Status = domain.RiskMitigated
		return nil
	})
	assert.ErrorIs(t, err, domain.ErrNotFound)
	assert.Equal(t, "open", statusOf(t, db, mine))
}

func TestBulkApply_RefusesAZeroTenantOrEmptySelection(t *testing.T) {
	repo, db := setupRiskBulkRepo(t)
	ctx := context.Background()
	tenant := uuid.New()
	a := seedBulkRisk(t, db, tenant, "open")

	_, err := repo.BulkApply(ctx, uuid.Nil, []uuid.UUID{a}, func(*domain.Risk) error { return nil })
	assert.ErrorIs(t, err, domain.ErrForbidden, "a zero tenant must fail closed, not run unscoped")

	_, err = repo.BulkApply(ctx, tenant, nil, func(*domain.Risk) error { return nil })
	assert.ErrorIs(t, err, domain.ErrValidation)
	assert.Equal(t, "open", statusOf(t, db, a))
}

// BulkDelete was atomic (one statement) but not STRICT: it matched what it could
// and reported a count, so a selection with one stale id deleted the rest.
func TestBulkDelete_IsStrict_OneMissingIDDeletesNothing(t *testing.T) {
	repo, db := setupRiskBulkRepo(t)
	ctx := context.Background()
	tenant := uuid.New()
	a := seedBulkRisk(t, db, tenant, "open")
	b := seedBulkRisk(t, db, tenant, "open")

	_, err := repo.BulkDelete(ctx, []uuid.UUID{a, b, uuid.New()}, tenant)
	assert.ErrorIs(t, err, domain.ErrNotFound)
	assert.Equal(t, int64(2), riskCount(t, db), "a short match must delete nothing at all")

	// The happy path still works.
	deleted, err := repo.BulkDelete(ctx, []uuid.UUID{a, b}, tenant)
	require.NoError(t, err)
	assert.Equal(t, int64(2), deleted)
	assert.Equal(t, int64(0), riskCount(t, db))
}

func TestBulkDelete_ForeignTenantIDDeletesNothing(t *testing.T) {
	repo, db := setupRiskBulkRepo(t)
	ctx := context.Background()
	tenantA, tenantB := uuid.New(), uuid.New()
	mine := seedBulkRisk(t, db, tenantA, "open")
	theirs := seedBulkRisk(t, db, tenantB, "open")

	_, err := repo.BulkDelete(ctx, []uuid.UUID{mine, theirs}, tenantA)
	assert.ErrorIs(t, err, domain.ErrNotFound)
	assert.Equal(t, int64(2), riskCount(t, db), "neither row may go")
}


// createTableFromModel builds a permissive SQLite table with one column per
// persisted field of the model. SQLite's dynamic typing makes the affinities
// below sufficient for a round trip; nothing here asserts on the DDL itself.
func createTableFromModel(t *testing.T, db *gorm.DB, model interface{}) {
	t.Helper()
	stmt := &gorm.Statement{DB: db}
	require.NoError(t, stmt.Parse(model))

	cols := make([]string, 0, len(stmt.Schema.Fields))
	for _, f := range stmt.Schema.Fields {
		// Associations and computed fields carry no column.
		if f.DBName == "" {
			continue
		}
		col := f.DBName + " " + sqliteAffinity(f.FieldType)
		if f.PrimaryKey {
			col += " PRIMARY KEY"
		}
		cols = append(cols, col)
	}
	require.NotEmpty(t, cols)
	require.NoError(t, db.Exec(
		"CREATE TABLE "+stmt.Schema.Table+" ("+strings.Join(cols, ", ")+")").Error)
}

func sqliteAffinity(ft reflect.Type) string {
	for ft.Kind() == reflect.Ptr {
		ft = ft.Elem()
	}
	if ft == reflect.TypeOf(time.Time{}) {
		return "DATETIME"
	}
	switch ft.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "INTEGER"
	case reflect.Float32, reflect.Float64:
		return "REAL"
	case reflect.Bool:
		return "BOOLEAN"
	default:
		return "TEXT"
	}
}
