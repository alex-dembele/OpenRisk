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

// The /automation/* collection routes (#421).
//
// GET /automation/rules, /executions, /sla, /sla/stats, /channels and /state are
// unparameterised reads that aggregate a whole table. Two of them are worse than
// a plain list if they lose their predicate: /automation/channels returns the
// tenant's outbound notification configuration (webhook endpoints, sender
// identities), and /automation/executions is the run history — what fired,
// against which entity, and when.
//
// Same shape as the rest of the sweep: seed both tenants, read as one, prove the
// other is absent.

var (
	autoA = uuid.MustParse("c0c0c0c0-0000-4000-8000-000000000001")
	autoB = uuid.MustParse("d0d0d0d0-0000-4000-8000-000000000002")
)

func newAutomationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	for _, ddl := range []string{
		`CREATE TABLE automation_rules (id TEXT PRIMARY KEY)`,
		`CREATE TABLE automation_executions (id TEXT PRIMARY KEY)`,
		`CREATE TABLE sla_trackers (id TEXT PRIMARY KEY)`,
		`CREATE TABLE automation_channels (id TEXT PRIMARY KEY)`,
	} {
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatalf("create table: %v", err)
		}
	}
	for _, m := range []struct {
		table string
		model any
	}{
		{"automation_rules", &domain.AutomationRule{}},
		{"automation_executions", &domain.AutomationExecution{}},
		{"sla_trackers", &domain.SLATracker{}},
		{"automation_channels", &domain.AutomationChannelConfig{}},
	} {
		if err := sqliteschema.Reconcile(db, m.table, m.model); err != nil {
			t.Fatalf("reconcile %s: %v", m.table, err)
		}
	}
	return db
}

func TestAutomationCollections_NeverCrossTenants(t *testing.T) {
	db := newAutomationDB(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)

	rules := NewGormAutomationRuleRepository(db)
	executions := NewGormAutomationExecutionRepository(db)
	slas := NewGormSLATrackerRepository(db)
	channels := NewGormAutomationChannelRepository(db)

	// Tenant A gets one of everything; tenant B gets two, so a dropped predicate
	// shows up as 3 rather than as a subtler off-by-one.
	seed := func(tenant uuid.UUID, n int, label string) {
		for i := 0; i < n; i++ {
			ruleID := uuid.New()
			if err := rules.Create(ctx, &domain.AutomationRule{
				ID: ruleID, TenantID: tenant, Name: label + " rule",
				Trigger: domain.TriggerRiskCreated, Enabled: true,
			}); err != nil {
				t.Fatalf("create rule: %v", err)
			}
			if err := executions.Create(ctx, &domain.AutomationExecution{
				ID: uuid.New(), TenantID: tenant, RuleID: ruleID,
				Status: domain.ExecutionSuccess, StartedAt: now,
			}); err != nil {
				t.Fatalf("create execution: %v", err)
			}
			if err := slas.Create(ctx, &domain.SLATracker{
				ID: uuid.New(), TenantID: tenant, RuleID: ruleID,
				Status: domain.SLAOpen, DueAt: now.AddDate(0, 0, 7),
			}); err != nil {
				t.Fatalf("create sla: %v", err)
			}
		}
		if err := channels.Upsert(ctx, &domain.AutomationChannelConfig{
			ID: uuid.New(), TenantID: tenant,
			SlackEnabled: true, SlackWebhookURL: "https://hooks.example/" + label,
		}); err != nil {
			t.Fatalf("save channels: %v", err)
		}
	}
	seed(autoA, 1, "A")
	seed(autoB, 2, "B")

	t.Run("rules", func(t *testing.T) {
		list, err := rules.List(ctx, autoA)
		if err != nil {
			t.Fatalf("list rules: %v", err)
		}
		if len(list) != 1 {
			t.Fatalf("tenant A owns one rule, got %d", len(list))
		}
		for _, r := range list {
			if r.TenantID != autoA {
				t.Fatalf("tenant %s's rule reached tenant A", r.TenantID)
			}
		}
		// GET /automation/state is computed from the same List; the trigger-scoped
		// read the engine uses on every event is asserted with it, because a leak
		// there would run another tenant's rules against this tenant's data.
		enabled, err := rules.ListEnabledByTrigger(ctx, autoA, domain.TriggerRiskCreated)
		if err != nil {
			t.Fatalf("list by trigger: %v", err)
		}
		if len(enabled) != 1 || enabled[0].TenantID != autoA {
			t.Fatalf("trigger lookup crossed the tenant boundary: %d rows", len(enabled))
		}
	})

	t.Run("executions", func(t *testing.T) {
		list, err := executions.List(ctx, autoA, 100, 0)
		if err != nil {
			t.Fatalf("list executions: %v", err)
		}
		if len(list) != 1 {
			t.Fatalf("tenant A owns one execution, got %d", len(list))
		}
		for _, e := range list {
			if e.TenantID != autoA {
				t.Fatalf("tenant %s's run history reached tenant A", e.TenantID)
			}
		}
	})

	t.Run("sla_and_stats", func(t *testing.T) {
		open, err := slas.ListOpen(ctx, autoA)
		if err != nil {
			t.Fatalf("list open sla: %v", err)
		}
		if len(open) != 1 || open[0].TenantID != autoA {
			t.Fatalf("tenant A owns one open SLA, got %d", len(open))
		}
		stats, err := slas.Stats(ctx, autoA)
		if err != nil {
			t.Fatalf("sla stats: %v", err)
		}
		if stats.Open != 1 {
			t.Fatalf("tenant B's two open trackers were counted into tenant A: open=%d", stats.Open)
		}
	})

	t.Run("channels", func(t *testing.T) {
		// The one that carries secrets. A missing predicate here hands another
		// organisation's webhook endpoints to whoever opens the settings page.
		cfg, err := channels.Get(ctx, autoA)
		if err != nil {
			t.Fatalf("get channels: %v", err)
		}
		if cfg == nil {
			t.Fatal("tenant A must read back its own channel config")
		}
		if cfg.TenantID != autoA {
			t.Fatalf("tenant %s's channel config reached tenant A", cfg.TenantID)
		}
		if cfg.SlackWebhookURL != "https://hooks.example/A" {
			t.Fatalf("tenant A read the wrong webhook: %q", cfg.SlackWebhookURL)
		}
	})

	t.Run("no_tenant_reads_as_empty_not_global", func(t *testing.T) {
		list, err := rules.List(ctx, uuid.Nil)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(list) != 0 {
			t.Fatalf("an unresolved tenant saw %d rules", len(list))
		}
		stats, err := slas.Stats(ctx, uuid.Nil)
		if err != nil {
			t.Fatalf("stats: %v", err)
		}
		if stats.Open != 0 {
			t.Fatalf("an unresolved tenant saw %d open SLAs", stats.Open)
		}
	})
}
