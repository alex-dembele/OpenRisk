// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/opendefender/openrisk/internal/domain"
	"github.com/opendefender/openrisk/internal/testsupport/sqliteschema"
)

// The /governance/* collection routes (#421).
//
// Nine unparameterised GETs — the audit trail and its export, the chain verdict,
// the retention policy, delegations, effective permissions, workflows and
// approvals — all reach the database through the three repositories below. The
// handlers pass the caller's tenant from the signed token and nothing else, so
// this is where a lost predicate would turn a list into every tenant's list.
//
// Shape, as everywhere in this sweep: seed the same qualifying row in two
// tenants, ask as tenant A, prove tenant B's row is absent. The audit trail
// matters most — it records who did what to whose data, so leaking it leaks the
// shape of another organisation's operations even where the rows themselves
// stayed put.

var (
	govA = uuid.MustParse("a0a0a0a0-0000-4000-8000-000000000001")
	govB = uuid.MustParse("b0b0b0b0-0000-4000-8000-000000000002")
)

func newGovernanceDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	for _, ddl := range []string{
		`CREATE TABLE audit_events (id TEXT PRIMARY KEY)`,
		`CREATE TABLE delegations (id TEXT PRIMARY KEY)`,
		`CREATE TABLE approval_workflows (id TEXT PRIMARY KEY)`,
		`CREATE TABLE approval_requests (id TEXT PRIMARY KEY)`,
		`CREATE TABLE audit_chain_seals (id TEXT PRIMARY KEY)`,
		`CREATE TABLE audit_retention_policies (id TEXT PRIMARY KEY)`,
	} {
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatalf("create table: %v", err)
		}
	}
	for _, m := range []struct {
		table string
		model any
	}{
		{"audit_events", &domain.AuditEvent{}},
		{"delegations", &domain.Delegation{}},
		{"approval_workflows", &domain.ApprovalWorkflow{}},
		{"approval_requests", &domain.ApprovalRequest{}},
		{"audit_chain_seals", &domain.AuditChainSeal{}},
		{"audit_retention_policies", &domain.AuditRetentionPolicy{}},
	} {
		if err := sqliteschema.Reconcile(db, m.table, m.model); err != nil {
			t.Fatalf("reconcile %s: %v", m.table, err)
		}
	}
	return db
}

// TestGovernanceCollections_NeverCrossTenants covers GET /governance/audit-events,
// /audit-events/export, /audit-retention, /delegations, /workflows and /approvals.
func TestGovernanceCollections_NeverCrossTenants(t *testing.T) {
	db := newGovernanceDB(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)

	events := NewGormAuditEventRepository(db)
	chain := NewGormAuditChainRepository(db)
	delegations := NewGormDelegationRepository(db)
	approvals := NewGormApprovalRepository(db)
	retention := NewGormAuditRetentionRepository(db)

	// --- audit trail ------------------------------------------------------
	// The SAME summary in both tenants: an assertion on counts alone would pass
	// against a query that returns the wrong tenant's row, so the test also
	// names the row it must never see.
	for _, tenant := range []uuid.UUID{govA, govB} {
		for i := 0; i < 2; i++ {
			actor := uuid.New()
			e := &domain.AuditEvent{
				ID:       uuid.New(),
				TenantID: tenant, ActorID: &actor,
				Action: domain.AuditActionUpdate, EntityType: "risk",
				EntityID: uuid.NewString(), Summary: "changed a risk in " + tenant.String(),
			}
			if err := events.Append(ctx, e); err != nil {
				t.Fatalf("append audit event: %v", err)
			}
		}
	}

	t.Run("audit_events_list", func(t *testing.T) {
		rows, total, err := events.List(ctx, govA, domain.AuditEventFilter{})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if total != 2 || len(rows) != 2 {
			t.Fatalf("tenant A must see exactly its own 2 events, got total=%d rows=%d", total, len(rows))
		}
		for _, e := range rows {
			if e.TenantID != govA {
				t.Fatalf("tenant %s's audit event reached tenant A's trail", e.TenantID)
			}
		}
	})

	t.Run("audit_events_export_is_the_same_predicate", func(t *testing.T) {
		// The export uses ListAll on the chain repository — an unpaginated read
		// of the whole filtered trail. "Unpaginated" is exactly the shape where a
		// missing predicate hands over everything, so it is asserted separately
		// rather than assumed from the paginated list above.
		all, err := chain.ListAll(ctx, govA, domain.AuditEventFilter{})
		if err != nil {
			t.Fatalf("list all: %v", err)
		}
		if len(all) != 2 {
			t.Fatalf("export must carry tenant A's 2 events and no others, got %d", len(all))
		}
		for _, e := range all {
			if e.TenantID != govA {
				t.Fatalf("tenant %s's event was exported to tenant A", e.TenantID)
			}
		}
	})

	t.Run("audit_chain_verification_reads_one_tenants_seals", func(t *testing.T) {
		// GET /governance/audit-events/verify walks this tenant's seals. A seal
		// is the tamper-evidence anchor for a pruned segment, so reading another
		// organisation's seal would make this tenant's chain verdict a statement
		// about somebody else's trail.
		//
		// Both tenants are seeded, and the assertion names the count: a loop over
		// an empty set proves nothing, which is what an un-seeded version of this
		// test would have been.
		for i, tenant := range []uuid.UUID{govA, govB, govB} {
			if err := db.Create(&domain.AuditChainSeal{
				ID: uuid.New(), TenantID: tenant,
				Reason: "prune", FromSequence: 1, ToSequence: int64(i + 1),
				PrunedCount: 1, LastHash: "hash", CreatedAt: now,
			}).Error; err != nil {
				t.Fatalf("seed seal: %v", err)
			}
		}
		seals, err := chain.ListSeals(ctx, govA)
		if err != nil {
			t.Fatalf("list seals: %v", err)
		}
		if len(seals) != 1 {
			t.Fatalf("tenant A owns one seal, got %d", len(seals))
		}
		if seals[0].TenantID != govA {
			t.Fatalf("tenant %s's chain seal reached tenant A's verdict", seals[0].TenantID)
		}
	})

	t.Run("audit_retention_policy", func(t *testing.T) {
		for _, tenant := range []uuid.UUID{govA, govB} {
			days := 90
			if tenant == govB {
				days = 3650
			}
			if err := retention.Upsert(ctx, &domain.AuditRetentionPolicy{
				TenantID: tenant, RetentionDays: days,
			}); err != nil {
				t.Fatalf("upsert retention: %v", err)
			}
		}
		// Read BOTH, and assert each gets its own number. Reading only one would
		// pass against a query with no predicate whenever that tenant's row
		// happened to be the first inserted.
		pa, err := retention.Get(ctx, govA)
		if err != nil {
			t.Fatalf("get retention: %v", err)
		}
		if pa == nil || pa.RetentionDays != 90 {
			t.Fatalf("tenant A must read its own 90-day policy, got %+v", pa)
		}
		pb, err := retention.Get(ctx, govB)
		if err != nil {
			t.Fatalf("get retention: %v", err)
		}
		if pb == nil || pb.RetentionDays != 3650 {
			t.Fatalf("tenant B must read its own 3650-day policy, got %+v", pb)
		}
	})

	// --- delegations ------------------------------------------------------
	t.Run("delegations", func(t *testing.T) {
		for _, tenant := range []uuid.UUID{govA, govB} {
			d := &domain.Delegation{
				ID: uuid.New(), TenantID: tenant,
				DelegatorID: uuid.New(), DelegateID: uuid.New(),
				Permissions: []string{"risks:read"},
				Status:      domain.DelegationActive,
				StartsAt:    now.AddDate(0, 0, -1), EndsAt: now.AddDate(0, 0, 30),
			}
			if err := delegations.Create(ctx, d); err != nil {
				t.Fatalf("create delegation: %v", err)
			}
		}
		list, err := delegations.List(ctx, govA, domain.DelegationFilter{})
		if err != nil {
			t.Fatalf("list delegations: %v", err)
		}
		if len(list) != 1 {
			t.Fatalf("tenant A owns one delegation, got %d", len(list))
		}
		if list[0].TenantID != govA {
			t.Fatal("another tenant's delegation reached tenant A")
		}

		// GET /governance/delegations/effective resolves through
		// ActiveDelegationsTo, a second query with its own WHERE. A delegate id
		// that exists in tenant B must resolve to nothing in tenant A.
		bList, _ := delegations.List(ctx, govB, domain.DelegationFilter{})
		if len(bList) != 1 {
			t.Fatalf("fixture: tenant B must own one delegation, got %d", len(bList))
		}
		crossed, err := delegations.ActiveDelegationsTo(ctx, govA, bList[0].DelegateID, now)
		if err != nil {
			t.Fatalf("active delegations: %v", err)
		}
		if len(crossed) != 0 {
			t.Fatal("tenant B's delegate resolved to permissions inside tenant A")
		}
	})

	// --- approval workflows and requests ----------------------------------
	t.Run("workflows_and_approvals", func(t *testing.T) {
		for _, tenant := range []uuid.UUID{govA, govB} {
			w := &domain.ApprovalWorkflow{
				ID: uuid.New(), TenantID: tenant, Name: "risk sign-off",
				EntityType: "risk", Action: "close", Enabled: true,
			}
			if err := approvals.CreateWorkflow(ctx, w); err != nil {
				t.Fatalf("create workflow: %v", err)
			}
			r := &domain.ApprovalRequest{
				ID: uuid.New(), TenantID: tenant,
				EntityType: "risk", EntityID: uuid.NewString(),
				Action: "close", Title: "close a risk",
				Status: domain.ApprovalPending, RequestedBy: uuid.New(),
			}
			if err := approvals.CreateRequest(ctx, r); err != nil {
				t.Fatalf("create request: %v", err)
			}
		}

		ws, err := approvals.ListWorkflows(ctx, govA)
		if err != nil {
			t.Fatalf("list workflows: %v", err)
		}
		if len(ws) != 1 || ws[0].TenantID != govA {
			t.Fatalf("tenant A must see its own single workflow, got %d", len(ws))
		}

		rs, err := approvals.ListRequests(ctx, govA, domain.ApprovalRequestFilter{})
		if err != nil {
			t.Fatalf("list requests: %v", err)
		}
		if len(rs) != 1 || rs[0].TenantID != govA {
			t.Fatalf("tenant A must see its own single approval request, got %d", len(rs))
		}

		// FindWorkflow is how a write decides whether sign-off is required. A
		// workflow defined by another organisation must never route this one.
		found, err := approvals.FindWorkflow(ctx, govA, "risk", "close")
		if err != nil {
			t.Fatalf("find workflow: %v", err)
		}
		if found == nil || found.TenantID != govA {
			t.Fatalf("FindWorkflow returned %+v", found)
		}
	})

	t.Run("no_tenant_reads_as_empty_not_global", func(t *testing.T) {
		rows, total, err := events.List(ctx, uuid.Nil, domain.AuditEventFilter{})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if total != 0 || len(rows) != 0 {
			t.Fatalf("an unresolved tenant must read the trail as empty, got %d", total)
		}
		// ListAll does not refuse a zero tenant, and does not need to: the
		// predicate is still emitted, so uuid.Nil matches no row. What matters
		// is that it reads as an EMPTY tenant and never as every tenant — the
		// distinction the retired /timeline/recent got wrong.
		all, err := chain.ListAll(ctx, uuid.Nil, domain.AuditEventFilter{})
		if err != nil {
			t.Fatalf("list all: %v", err)
		}
		if len(all) != 0 {
			t.Fatalf("an unresolved tenant exported %d events", len(all))
		}
	})
}
