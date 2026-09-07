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

func setupSavedViewRepo(t *testing.T) *GormSavedViewRepository {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`
		CREATE TABLE saved_views (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			table_id TEXT NOT NULL,
			name TEXT NOT NULL,
			visibility TEXT NOT NULL DEFAULT 'personal',
			state TEXT,
			columns TEXT,
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME
		);
	`).Error)
	return NewGormSavedViewRepository(db)
}

func newView(tenant, user uuid.UUID, tableID, name string, vis domain.SavedViewVisibility) *domain.SavedView {
	return &domain.SavedView{
		ID:         uuid.New(),
		TenantID:   tenant,
		UserID:     user,
		TableID:    tableID,
		Name:       name,
		Visibility: vis,
		State: domain.SavedViewState{
			Q:       "log4j",
			Filters: map[string][]string{"severity": {"critical", "high"}},
			Sort:    &domain.SavedViewSort{Key: "cvss", Dir: "desc"},
		},
		Columns: domain.SavedViewColumns{Order: []string{"name", "cvss"}, Hidden: []string{"owner"}},
	}
}

// The state and column payloads must survive the round trip through the jsonb
// column: a view that forgets its filters on reload is not a saved view.
func TestSavedViewRepo_CreateAndGet_RoundTripsTheState(t *testing.T) {
	repo := setupSavedViewRepo(t)
	ctx := context.Background()
	tenant, user := uuid.New(), uuid.New()

	view := newView(tenant, user, "risks", "Comité T3", domain.SavedViewPersonal)
	require.NoError(t, repo.Create(ctx, view))

	got, err := repo.GetByID(ctx, view.ID, tenant)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "Comité T3", got.Name)
	assert.Equal(t, "log4j", got.State.Q)
	assert.Equal(t, []string{"critical", "high"}, got.State.Filters["severity"])
	require.NotNil(t, got.State.Sort)
	assert.Equal(t, "cvss", got.State.Sort.Key)
	assert.Equal(t, []string{"name", "cvss"}, got.Columns.Order)
	assert.Equal(t, []string{"owner"}, got.Columns.Hidden)
}

// ABSOLUTE RULE 2 at the repository boundary: every read and write is scoped by
// tenant_id, so another tenant's view is not merely refused — it is invisible.
func TestSavedViewRepo_CrossTenant_IsInvisible(t *testing.T) {
	repo := setupSavedViewRepo(t)
	ctx := context.Background()
	tenantA, tenantB := uuid.New(), uuid.New()
	userA, userB := uuid.New(), uuid.New()

	viewA := newView(tenantA, userA, "risks", "A's view", domain.SavedViewShared)
	require.NoError(t, repo.Create(ctx, viewA))

	// Read: nil, not an error and certainly not the row.
	got, err := repo.GetByID(ctx, viewA.ID, tenantB)
	require.NoError(t, err)
	assert.Nil(t, got)

	// List: tenant B sees nothing, even though A's view is SHARED — sharing is
	// within one tenant, full stop.
	listB, err := repo.ListVisible(ctx, tenantB, userB, "risks")
	require.NoError(t, err)
	assert.Empty(t, listB)

	// Update from tenant B affects zero rows and leaves A's data untouched.
	forged := *viewA
	forged.TenantID = tenantB
	forged.Name = "hijacked"
	assert.Error(t, repo.Update(ctx, &forged))

	// Delete from tenant B affects zero rows.
	assert.Error(t, repo.Delete(ctx, viewA.ID, tenantB))

	still, err := repo.GetByID(ctx, viewA.ID, tenantA)
	require.NoError(t, err)
	require.NotNil(t, still)
	assert.Equal(t, "A's view", still.Name)
}

// The listing predicate is the one that decides what a colleague sees. It must
// be scoped on BOTH axes at once: the tenant, and within it the visibility.
func TestSavedViewRepo_ListVisible_IsTenantAndVisibilityScoped(t *testing.T) {
	repo := setupSavedViewRepo(t)
	ctx := context.Background()
	tenantA, tenantB := uuid.New(), uuid.New()
	alice, bob := uuid.New(), uuid.New()

	mine := newView(tenantA, alice, "risks", "Mine personal", domain.SavedViewPersonal)
	minePub := newView(tenantA, alice, "risks", "Mine shared", domain.SavedViewShared)
	bobPersonal := newView(tenantA, bob, "risks", "Bob personal", domain.SavedViewPersonal)
	bobShared := newView(tenantA, bob, "risks", "Bob shared", domain.SavedViewShared)
	otherTable := newView(tenantA, alice, "assets", "Other table", domain.SavedViewPersonal)
	foreign := newView(tenantB, uuid.New(), "risks", "Foreign shared", domain.SavedViewShared)
	for _, v := range []*domain.SavedView{mine, minePub, bobPersonal, bobShared, otherTable, foreign} {
		require.NoError(t, repo.Create(ctx, v))
	}

	got, err := repo.ListVisible(ctx, tenantA, alice, "risks")
	require.NoError(t, err)

	names := make([]string, 0, len(got))
	for _, v := range got {
		names = append(names, v.Name)
	}
	assert.ElementsMatch(t, []string{"Bob shared", "Mine personal", "Mine shared"}, names)
	assert.NotContains(t, names, "Bob personal", "another user's personal view must never be listed")
	assert.NotContains(t, names, "Foreign shared", "another tenant's shared view must never be listed")
	assert.NotContains(t, names, "Other table", "table_id must narrow the listing")

	// An empty table_id lists every register — what the one-time localStorage
	// migration reads to know what is already on the server.
	all, err := repo.ListVisible(ctx, tenantA, alice, "")
	require.NoError(t, err)
	assert.Len(t, all, 4)
}

func TestSavedViewRepo_Update_PersistsAndCannotRewriteOwnership(t *testing.T) {
	repo := setupSavedViewRepo(t)
	ctx := context.Background()
	tenant, owner, other := uuid.New(), uuid.New(), uuid.New()

	view := newView(tenant, owner, "risks", "Draft", domain.SavedViewPersonal)
	require.NoError(t, repo.Create(ctx, view))

	// A payload that names a different owner must not be able to move the row:
	// Update writes an explicit column list, so user_id is not in it.
	patched := *view
	patched.UserID = other
	patched.Name = "Comité T4"
	patched.Visibility = domain.SavedViewShared
	patched.State = domain.SavedViewState{Q: "", Filters: map[string][]string{"status": {"open"}}}
	require.NoError(t, repo.Update(ctx, &patched))

	got, err := repo.GetByID(ctx, view.ID, tenant)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "Comité T4", got.Name)
	assert.Equal(t, domain.SavedViewShared, got.Visibility)
	assert.Equal(t, []string{"open"}, got.State.Filters["status"])
	assert.Equal(t, owner, got.UserID, "an update payload must not be able to rewrite ownership")
}

func TestSavedViewRepo_Delete_RemovesOnlyTheNamedRow(t *testing.T) {
	repo := setupSavedViewRepo(t)
	ctx := context.Background()
	tenant, user := uuid.New(), uuid.New()

	keep := newView(tenant, user, "risks", "Keep", domain.SavedViewPersonal)
	drop := newView(tenant, user, "risks", "Drop", domain.SavedViewPersonal)
	require.NoError(t, repo.Create(ctx, keep))
	require.NoError(t, repo.Create(ctx, drop))

	require.NoError(t, repo.Delete(ctx, drop.ID, tenant))

	gone, err := repo.GetByID(ctx, drop.ID, tenant)
	require.NoError(t, err)
	assert.Nil(t, gone)

	remaining, err := repo.ListVisible(ctx, tenant, user, "risks")
	require.NoError(t, err)
	require.Len(t, remaining, 1)
	assert.Equal(t, "Keep", remaining[0].Name)

	// A second delete of the same id is not found, not a silent success.
	assert.Error(t, repo.Delete(ctx, drop.ID, tenant))
}

// Name uniqueness is per (tenant, user, table) — two users may each have their
// own "Comité T3", and the same user may reuse the name on another register.
func TestSavedViewRepo_ExistsByName_IsScopedToOwnerAndTable(t *testing.T) {
	repo := setupSavedViewRepo(t)
	ctx := context.Background()
	tenant, alice, bob := uuid.New(), uuid.New(), uuid.New()

	view := newView(tenant, alice, "risks", "Comité T3", domain.SavedViewPersonal)
	require.NoError(t, repo.Create(ctx, view))

	dup, err := repo.ExistsByName(ctx, tenant, alice, "risks", "Comité T3", uuid.Nil)
	require.NoError(t, err)
	assert.True(t, dup)

	// Excluding the row itself: a rename onto its own name is not a conflict.
	self, err := repo.ExistsByName(ctx, tenant, alice, "risks", "Comité T3", view.ID)
	require.NoError(t, err)
	assert.False(t, self)

	otherUser, err := repo.ExistsByName(ctx, tenant, bob, "risks", "Comité T3", uuid.Nil)
	require.NoError(t, err)
	assert.False(t, otherUser)

	otherTable, err := repo.ExistsByName(ctx, tenant, alice, "assets", "Comité T3", uuid.Nil)
	require.NoError(t, err)
	assert.False(t, otherTable)

	otherTenant, err := repo.ExistsByName(ctx, uuid.New(), alice, "risks", "Comité T3", uuid.Nil)
	require.NoError(t, err)
	assert.False(t, otherTenant)
}

func TestSavedViewRepo_Create_RefusesAZeroIdentity(t *testing.T) {
	repo := setupSavedViewRepo(t)
	ctx := context.Background()

	assert.Error(t, repo.Create(ctx, newView(uuid.Nil, uuid.New(), "risks", "no tenant", domain.SavedViewPersonal)))
	assert.Error(t, repo.Create(ctx, newView(uuid.New(), uuid.Nil, "risks", "no user", domain.SavedViewPersonal)))
}
