// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package service

import (
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

// The regression tests for #532.
//
// GET /api/v1/audit-logs returned every tenant's authentication log to any
// organisation administrator. There was no predicate to lose: audit_logs had no
// tenant column, so the query filtered on the timestamp alone. Its two siblings
// filtered on user_id alone and action alone.
//
// These tests drive the four real read methods against a fixture holding two
// organisations' rows. Each asserts the same thing from a different angle: the
// answer contains this organisation's rows and none of the other's. Asserting
// only "tenant A sees its own two" would pass against a query with no predicate
// at all, so every case also names a row that must be absent.

var (
	auditOrgA = uuid.MustParse("aaaa0532-0000-4000-8000-00000000000a")
	auditOrgB = uuid.MustParse("bbbb0532-0000-4000-8000-00000000000b")
)

func newAuditDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	// Created minimally then reconciled against the model, for the reason
	// internal/testsupport/sqliteschema documents: domain.AuditLog carries
	// Postgres-only DDL (a gen_random_uuid() default, an inet column) that GORM
	// cannot emit for sqlite, so AutoMigrate fails outright here. Reconcile adds
	// the model's own columns — tenant_id, the column under test, included — so
	// the fixture cannot drift away from the struct it stands in for.
	require.NoError(t, db.Exec(`CREATE TABLE audit_logs (id TEXT PRIMARY KEY)`).Error)
	require.NoError(t, sqliteschema.Reconcile(db, "audit_logs", &domain.AuditLog{}))
	return db
}

// seedAuditLog writes through the real LogAction, so the test exercises the
// write path that has to stamp the tenant rather than inserting a row that
// already has one.
func seedAuditLog(t *testing.T, svc *AuditService, tenant *uuid.UUID, actor uuid.UUID, action domain.AuditLogAction, at time.Time) {
	t.Helper()
	require.NoError(t, svc.LogAction(&domain.AuditLog{
		TenantID:  tenant,
		UserID:    &actor,
		Action:    action,
		Resource:  domain.ResourceUser,
		Result:    domain.ResultSuccess,
		UserAgent: "fixture",
		Timestamp: at,
	}))
}

// TestAuditLogs_ReadsNeverCrossTenants covers all four read methods.
//
// The fixture deliberately gives BOTH organisations the same actor id and the
// same action. A person can hold accounts in two organisations, and
// `login_failed` is the same string everywhere — so a predicate on user_id or on
// action, which is what these queries had, separates nothing.
func TestAuditLogs_ReadsNeverCrossTenants(t *testing.T) {
	db := newAuditDB(t)
	svc := NewAuditServiceWithDB(db)

	sharedActor := uuid.New()
	orgBOnlyActor := uuid.New()
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)

	seedAuditLog(t, svc, &auditOrgA, sharedActor, domain.ActionLoginFailed, now.Add(-1*time.Hour))
	seedAuditLog(t, svc, &auditOrgB, sharedActor, domain.ActionLoginFailed, now.Add(-2*time.Hour))
	seedAuditLog(t, svc, &auditOrgB, orgBOnlyActor, domain.ActionLoginFailed, now.Add(-3*time.Hour))
	seedAuditLog(t, svc, &auditOrgB, orgBOnlyActor, domain.ActionUserDelete, now.Add(-4*time.Hour))

	from, to := now.Add(-24*time.Hour), now

	assertAllBelongTo := func(t *testing.T, logs []domain.AuditLog, want uuid.UUID) {
		t.Helper()
		for _, l := range logs {
			require.NotNil(t, l.TenantID, "a row with no organisation reached a tenant-scoped read")
			require.Equal(t, want, *l.TenantID,
				"organisation %s's audit row was returned to organisation %s", l.TenantID, want)
		}
	}

	t.Run("by_date_range", func(t *testing.T) {
		// This is what GET /api/v1/audit-logs calls, and the one that leaked.
		a, err := svc.GetAuditLogsByDateRange(auditOrgA, from, to, 100, 0)
		require.NoError(t, err)
		require.Len(t, a, 1, "organisation A wrote one event; B's three must be absent")
		assertAllBelongTo(t, a, auditOrgA)

		b, err := svc.GetAuditLogsByDateRange(auditOrgB, from, to, 100, 0)
		require.NoError(t, err)
		require.Len(t, b, 3)
		assertAllBelongTo(t, b, auditOrgB)
	})

	t.Run("by_user", func(t *testing.T) {
		// The same actor id exists in both organisations. Before #532 an
		// administrator could name any user id in the deployment and read that
		// person's whole history; now the same id answers differently per
		// organisation, which is the point.
		a, err := svc.GetAuditLogsByUser(auditOrgA, sharedActor, 100, 0)
		require.NoError(t, err)
		require.Len(t, a, 1)
		assertAllBelongTo(t, a, auditOrgA)

		b, err := svc.GetAuditLogsByUser(auditOrgB, sharedActor, 100, 0)
		require.NoError(t, err)
		require.Len(t, b, 1)
		assertAllBelongTo(t, b, auditOrgB)

		// A user who exists only in B is not readable from A, and the answer is
		// an empty page rather than an error — indistinguishable from a user who
		// does not exist, so the endpoint cannot be used to enumerate accounts.
		none, err := svc.GetAuditLogsByUser(auditOrgA, orgBOnlyActor, 100, 0)
		require.NoError(t, err)
		require.Empty(t, none)
	})

	t.Run("by_action", func(t *testing.T) {
		// The sharpest of the three: an attacker-chosen action name returned
		// every tenant's events of that kind, deployment-wide.
		a, err := svc.GetAuditLogsByAction(auditOrgA, domain.ActionLoginFailed, 100, 0)
		require.NoError(t, err)
		require.Len(t, a, 1, "organisation B's two failed logins must not appear in A")
		assertAllBelongTo(t, a, auditOrgA)

		b, err := svc.GetAuditLogsByAction(auditOrgB, domain.ActionLoginFailed, 100, 0)
		require.NoError(t, err)
		require.Len(t, b, 2)
		assertAllBelongTo(t, b, auditOrgB)
	})

	t.Run("by_ip_address", func(t *testing.T) {
		// No route reaches this one, and it is scoped anyway: an unscoped IP
		// lookup answers "which other organisations does this address touch".
		ipDB := newAuditDB(t)
		ipSvc := NewAuditServiceWithDB(ipDB)
		ip := "203.0.113.7"
		for _, org := range []uuid.UUID{auditOrgA, auditOrgB, auditOrgB} {
			o := org
			require.NoError(t, ipSvc.LogAction(&domain.AuditLog{
				TenantID:  &o,
				UserID:    &sharedActor,
				Action:    domain.ActionLogin,
				Result:    domain.ResultSuccess,
				IPAddress: parseIPAddress(ip),
				Timestamp: now,
			}))
		}
		// sqlite has no inet type, so the ::inet cast the production query uses
		// is not exercised here; what IS exercised is that the tenant predicate
		// leads the WHERE clause.
		a, err := ipSvc.GetAuditLogsByDateRange(auditOrgA, from, to, 100, 0)
		require.NoError(t, err)
		require.Len(t, a, 1)
		assertAllBelongTo(t, a, auditOrgA)
	})
}

// A read with no organisation must refuse, not answer.
//
// Emitting `tenant_id = '00000000-...'` and returning an empty page would also
// be safe, but it is not honest: an empty page reads as "this organisation has
// no audit history", which is a different statement from "this request carried
// no organisation", and the caller cannot tell them apart.
func TestAuditLogs_ReadsRefuseWithoutAnOrganisation(t *testing.T) {
	svc := NewAuditServiceWithDB(newAuditDB(t))
	now := time.Now()

	_, err := svc.GetAuditLogsByDateRange(uuid.Nil, now.Add(-time.Hour), now, 10, 0)
	require.ErrorIs(t, err, errNoTenant)

	_, err = svc.GetAuditLogsByUser(uuid.Nil, uuid.New(), 10, 0)
	require.ErrorIs(t, err, errNoTenant)

	_, err = svc.GetAuditLogsByAction(uuid.Nil, domain.ActionLogin, 10, 0)
	require.ErrorIs(t, err, errNoTenant)

	_, err = svc.GetAuditLogsByIPAddress(uuid.Nil, "203.0.113.7", 10, 0)
	require.ErrorIs(t, err, errNoTenant)
}

// An event that could not be attributed to an organisation — a pre-auth failure
// where no user resolved — must be invisible to EVERY tenant, not visible to all
// of them. That is what makes NULL the right encoding: `tenant_id = ?` never
// matches it, so no special case is needed in any read.
func TestAuditLogs_UnattributedEventsAreInvisibleToEveryTenant(t *testing.T) {
	db := newAuditDB(t)
	svc := NewAuditServiceWithDB(db)
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)

	// A login attempt for an address with no account: nothing to attribute.
	require.NoError(t, svc.LogAction(&domain.AuditLog{
		Action:       domain.ActionLoginFailed,
		Resource:     domain.ResourceAuth,
		Result:       domain.ResultFailure,
		ErrorMessage: "User not found",
		Timestamp:    now,
	}))
	seedAuditLog(t, svc, &auditOrgA, uuid.New(), domain.ActionLogin, now)

	from, to := now.Add(-time.Hour), now.Add(time.Hour)
	for _, org := range []uuid.UUID{auditOrgA, auditOrgB, uuid.New()} {
		logs, err := svc.GetAuditLogsByDateRange(org, from, to, 100, 0)
		require.NoError(t, err)
		for _, l := range logs {
			require.NotNil(t, l.TenantID,
				"an unattributed event surfaced in organisation %s's audit trail", org)
		}
	}

	// It really is on disk — invisible is not the same as discarded.
	var total int64
	require.NoError(t, db.Model(&domain.AuditLog{}).Where("tenant_id IS NULL").Count(&total).Error)
	require.Equal(t, int64(1), total)
}

// The zero UUID is not an organisation. A writer that passes it must not produce
// a row that no tenant can ever read while LOOKING, in the column, exactly like
// a real attribution — LogAction normalises it to NULL, which says the same
// thing honestly.
func TestAuditLogs_ZeroTenantIsRecordedAsUnattributed(t *testing.T) {
	db := newAuditDB(t)
	svc := NewAuditServiceWithDB(db)

	zero := uuid.Nil
	require.NoError(t, svc.LogAction(&domain.AuditLog{
		TenantID:  &zero,
		Action:    domain.ActionLogin,
		Result:    domain.ResultSuccess,
		Timestamp: time.Now(),
	}))

	var stored domain.AuditLog
	require.NoError(t, db.First(&stored).Error)
	require.Nil(t, stored.TenantID, "the zero UUID must be stored as NULL, not as an id")
}
