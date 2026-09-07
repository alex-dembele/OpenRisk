// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package savedview

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/opendefender/openrisk/internal/domain"
)

// ---------------------------------------------------------------------------
// In-memory repository.
//
// It enforces the SAME tenant scoping the GORM implementation does — that
// behaviour is proven separately against real SQL in
// repository/gorm_saved_view_repository_test. What these tests are about is the
// authorisation ladder the use cases build ON TOP of it: what is invisible
// (404), what is visible but not yours (403), and what is yours.
// ---------------------------------------------------------------------------

type memRepo struct {
	views   map[uuid.UUID]*domain.SavedView
	failAll error
}

func newMemRepo() *memRepo { return &memRepo{views: map[uuid.UUID]*domain.SavedView{}} }

func (r *memRepo) Create(_ context.Context, v *domain.SavedView) error {
	if r.failAll != nil {
		return r.failAll
	}
	copied := *v
	r.views[v.ID] = &copied
	return nil
}

func (r *memRepo) GetByID(_ context.Context, id, tenantID uuid.UUID) (*domain.SavedView, error) {
	if r.failAll != nil {
		return nil, r.failAll
	}
	v, ok := r.views[id]
	if !ok || v.TenantID != tenantID {
		return nil, nil
	}
	copied := *v
	return &copied, nil
}

func (r *memRepo) ListVisible(_ context.Context, tenantID, userID uuid.UUID, tableID string) ([]domain.SavedView, error) {
	if r.failAll != nil {
		return nil, r.failAll
	}
	out := []domain.SavedView{}
	for _, v := range r.views {
		if v.TenantID != tenantID {
			continue
		}
		if tableID != "" && v.TableID != tableID {
			continue
		}
		if v.UserID != userID && !v.IsShared() {
			continue
		}
		out = append(out, *v)
	}
	return out, nil
}

func (r *memRepo) Update(_ context.Context, v *domain.SavedView) error {
	if r.failAll != nil {
		return r.failAll
	}
	existing, ok := r.views[v.ID]
	if !ok || existing.TenantID != v.TenantID {
		return errors.New("not found")
	}
	existing.Name = v.Name
	existing.Visibility = v.Visibility
	existing.State = v.State
	existing.Columns = v.Columns
	return nil
}

func (r *memRepo) Delete(_ context.Context, id, tenantID uuid.UUID) error {
	if r.failAll != nil {
		return r.failAll
	}
	v, ok := r.views[id]
	if !ok || v.TenantID != tenantID {
		return errors.New("not found")
	}
	delete(r.views, id)
	return nil
}

func (r *memRepo) ExistsByName(_ context.Context, tenantID, userID uuid.UUID, tableID, name string, excludeID uuid.UUID) (bool, error) {
	if r.failAll != nil {
		return false, r.failAll
	}
	for _, v := range r.views {
		if v.ID == excludeID {
			continue
		}
		if v.TenantID == tenantID && v.UserID == userID && v.TableID == tableID && v.Name == name {
			return true, nil
		}
	}
	return false, nil
}

type stubUsers map[uuid.UUID]string

func (s stubUsers) EmailsByIDs(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	out := map[uuid.UUID]string{}
	for _, id := range ids {
		if email, ok := s[id]; ok {
			out[id] = email
		}
	}
	return out, nil
}

func seed(r *memRepo, tenant, user uuid.UUID, name string, vis domain.SavedViewVisibility) *domain.SavedView {
	v := &domain.SavedView{
		ID: uuid.New(), TenantID: tenant, UserID: user, TableID: "risks", Name: name, Visibility: vis,
		State: domain.SavedViewState{Filters: map[string][]string{"severity": {"critical"}}},
	}
	r.views[v.ID] = v
	return v
}

// ---------------------------------------------------------------------------
// Success
// ---------------------------------------------------------------------------

func TestSavedViews_Success(t *testing.T) {
	ctx := context.Background()
	repo := newMemRepo()
	tenant, alice := uuid.New(), uuid.New()
	caller := Caller{TenantID: tenant, UserID: alice}

	created, err := NewCreateSavedViewUseCase(repo).Execute(ctx, caller, CreateSavedViewInput{
		TableID: "risks",
		Name:    "  Comité T3  ",
		State: domain.SavedViewState{
			Q:       "log4j",
			Filters: map[string][]string{"severity": {"critical", "high"}},
			Sort:    &domain.SavedViewSort{Key: "score", Dir: "asc"},
		},
		Columns: domain.SavedViewColumns{Order: []string{"title", "score"}, Hidden: []string{"owner"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "Comité T3", created.Name, "the name is trimmed before it is stored")
	assert.Equal(t, tenant, created.TenantID)
	assert.Equal(t, alice, created.UserID)
	assert.Equal(t, domain.SavedViewPersonal, created.Visibility, "sharing is an explicit act, never a default")

	listed, err := NewListSavedViewsUseCase(repo).Execute(ctx, caller, "risks")
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.Equal(t, "log4j", listed[0].State.Q)

	shared := domain.SavedViewShared
	newName := "Comité T4"
	updated, err := NewUpdateSavedViewUseCase(repo).Execute(ctx, caller, created.ID, UpdateSavedViewInput{
		Name:       &newName,
		Visibility: &shared,
	})
	require.NoError(t, err)
	assert.Equal(t, "Comité T4", updated.Name)
	assert.True(t, updated.IsShared())
	assert.Equal(t, "log4j", updated.State.Q, "a patch that omits the state must not reset the filters")

	require.NoError(t, NewDeleteSavedViewUseCase(repo).Execute(ctx, caller, created.ID))
	after, err := NewListSavedViewsUseCase(repo).Execute(ctx, caller, "risks")
	require.NoError(t, err)
	assert.Empty(t, after)
}

// A shared view is listed to a colleague; a personal one is not (criterion 3).
func TestSavedViews_Success_SharedIsVisibleToTheTenantAndPersonalIsNot(t *testing.T) {
	ctx := context.Background()
	repo := newMemRepo()
	tenant, alice, bob := uuid.New(), uuid.New(), uuid.New()
	seed(repo, tenant, alice, "Alice personal", domain.SavedViewPersonal)
	seed(repo, tenant, alice, "Alice shared", domain.SavedViewShared)

	listed, err := NewListSavedViewsUseCase(repo).
		WithUserLookup(stubUsers{alice: "alice@banque.cm"}).
		Execute(ctx, Caller{TenantID: tenant, UserID: bob}, "risks")
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.Equal(t, "Alice shared", listed[0].Name)
	assert.Equal(t, "alice@banque.cm", listed[0].OwnerEmail, "a shared view says who shared it")
}

// ---------------------------------------------------------------------------
// NotFound
// ---------------------------------------------------------------------------

func TestSavedViews_NotFound(t *testing.T) {
	ctx := context.Background()
	repo := newMemRepo()
	caller := Caller{TenantID: uuid.New(), UserID: uuid.New()}
	absent := uuid.New()

	_, err := NewUpdateSavedViewUseCase(repo).Execute(ctx, caller, absent, UpdateSavedViewInput{})
	assert.ErrorIs(t, err, domain.ErrNotFound)

	err = NewDeleteSavedViewUseCase(repo).Execute(ctx, caller, absent)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

// Another user's PERSONAL view inside the same tenant is invisible, so it must
// answer exactly as an id that never existed — not 403, which would confirm it.
func TestSavedViews_NotFound_AnotherUsersPersonalViewIsInvisible(t *testing.T) {
	ctx := context.Background()
	repo := newMemRepo()
	tenant, alice, bob := uuid.New(), uuid.New(), uuid.New()
	alicesView := seed(repo, tenant, alice, "Alice personal", domain.SavedViewPersonal)
	bobCaller := Caller{TenantID: tenant, UserID: bob}

	_, err := NewUpdateSavedViewUseCase(repo).Execute(ctx, bobCaller, alicesView.ID, UpdateSavedViewInput{})
	assert.ErrorIs(t, err, domain.ErrNotFound)
	assert.NotErrorIs(t, err, domain.ErrForbidden)

	err = NewDeleteSavedViewUseCase(repo).Execute(ctx, bobCaller, alicesView.ID)
	assert.ErrorIs(t, err, domain.ErrNotFound)

	// A tenant admin gets the same answer: admin power is over the tenant's data,
	// not over a colleague's working filter.
	admin := Caller{TenantID: tenant, UserID: uuid.New(), IsAdmin: true}
	err = NewDeleteSavedViewUseCase(repo).Execute(ctx, admin, alicesView.ID)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

// ---------------------------------------------------------------------------
// Unauthorized
// ---------------------------------------------------------------------------

// A zero identity must fail closed rather than run an unscoped query.
func TestSavedViews_Unauthorized(t *testing.T) {
	ctx := context.Background()
	repo := newMemRepo()
	view := seed(repo, uuid.New(), uuid.New(), "someone's", domain.SavedViewShared)

	for name, caller := range map[string]Caller{
		"no tenant": {UserID: uuid.New()},
		"no user":   {TenantID: uuid.New()},
		"neither":   {},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := NewListSavedViewsUseCase(repo).Execute(ctx, caller, "risks")
			assert.ErrorIs(t, err, domain.ErrForbidden)

			_, err = NewCreateSavedViewUseCase(repo).Execute(ctx, caller, CreateSavedViewInput{TableID: "risks", Name: "x"})
			assert.ErrorIs(t, err, domain.ErrForbidden)

			_, err = NewUpdateSavedViewUseCase(repo).Execute(ctx, caller, view.ID, UpdateSavedViewInput{})
			assert.ErrorIs(t, err, domain.ErrForbidden)

			assert.ErrorIs(t, NewDeleteSavedViewUseCase(repo).Execute(ctx, caller, view.ID), domain.ErrForbidden)
		})
	}
}

// A SHARED view owned by somebody else IS 403 — and here that is right, because
// the caller can already see the row, so naming the refusal leaks nothing.
// A tenant admin may edit and delete it (criterion 4).
func TestSavedViews_Unauthorized_SharedViewIsEditableOnlyByOwnerOrAdmin(t *testing.T) {
	ctx := context.Background()
	repo := newMemRepo()
	tenant, alice, bob := uuid.New(), uuid.New(), uuid.New()
	name := "renamed"

	shared := seed(repo, tenant, alice, "Alice shared", domain.SavedViewShared)
	bobCaller := Caller{TenantID: tenant, UserID: bob}

	_, err := NewUpdateSavedViewUseCase(repo).Execute(ctx, bobCaller, shared.ID, UpdateSavedViewInput{Name: &name})
	assert.ErrorIs(t, err, domain.ErrForbidden)
	assert.NotErrorIs(t, err, domain.ErrNotFound)

	err = NewDeleteSavedViewUseCase(repo).Execute(ctx, bobCaller, shared.ID)
	assert.ErrorIs(t, err, domain.ErrForbidden)

	// The owner may.
	_, err = NewUpdateSavedViewUseCase(repo).Execute(ctx, Caller{TenantID: tenant, UserID: alice}, shared.ID, UpdateSavedViewInput{Name: &name})
	require.NoError(t, err)

	// A tenant admin may.
	admin := Caller{TenantID: tenant, UserID: uuid.New(), IsAdmin: true}
	require.NoError(t, NewDeleteSavedViewUseCase(repo).Execute(ctx, admin, shared.ID))
}

// ---------------------------------------------------------------------------
// Cross-tenant — #580 criterion 5, the one that matters
// ---------------------------------------------------------------------------

// A view of tenant A, addressed by a user of tenant B, must produce the SAME
// error as a fabricated id: ErrNotFound, never ErrForbidden. A 403 here would
// confirm the resource exists, and a saved view's filter values name that
// institution's assets, tags and people.
func TestSavedViews_CrossTenant_IsNotFound_Not403(t *testing.T) {
	ctx := context.Background()
	repo := newMemRepo()
	tenantA, tenantB := uuid.New(), uuid.New()
	alice, mallory := uuid.New(), uuid.New()

	// Both visibilities, because "shared" is the case most likely to be leaked:
	// sharing is within ONE tenant, so it must not widen anything across them.
	personal := seed(repo, tenantA, alice, "A personal", domain.SavedViewPersonal)
	shared := seed(repo, tenantA, alice, "A shared", domain.SavedViewShared)
	fabricated := uuid.New()

	// Every caller shape from tenant B, including one that claims admin.
	callers := map[string]Caller{
		"member": {TenantID: tenantB, UserID: mallory},
		"admin":  {TenantID: tenantB, UserID: mallory, IsAdmin: true},
	}

	for callerName, caller := range callers {
		t.Run(callerName, func(t *testing.T) {
			// The reference answer: an id that was never issued.
			_, refErr := NewUpdateSavedViewUseCase(repo).Execute(ctx, caller, fabricated, UpdateSavedViewInput{})
			require.ErrorIs(t, refErr, domain.ErrNotFound)
			refDelErr := NewDeleteSavedViewUseCase(repo).Execute(ctx, caller, fabricated)
			require.ErrorIs(t, refDelErr, domain.ErrNotFound)

			for _, target := range []*domain.SavedView{personal, shared} {
				_, err := NewUpdateSavedViewUseCase(repo).Execute(ctx, caller, target.ID, UpdateSavedViewInput{})
				assert.ErrorIs(t, err, domain.ErrNotFound, "cross-tenant update must be not-found")
				assert.NotErrorIs(t, err, domain.ErrForbidden, "a 403 confirms the view exists")
				assert.Equal(t, clientAnswer(refErr), clientAnswer(err),
					"cross-tenant and fabricated ids must be indistinguishable to the caller")

				delErr := NewDeleteSavedViewUseCase(repo).Execute(ctx, caller, target.ID)
				assert.ErrorIs(t, delErr, domain.ErrNotFound)
				assert.NotErrorIs(t, delErr, domain.ErrForbidden)
				assert.Equal(t, clientAnswer(refDelErr), clientAnswer(delErr))
			}

			// And the listing shows tenant A nothing of its own.
			listed, err := NewListSavedViewsUseCase(repo).Execute(ctx, caller, "risks")
			require.NoError(t, err)
			assert.Empty(t, listed)
		})
	}

	// Tenant A's rows are untouched by any of it.
	still, err := NewListSavedViewsUseCase(repo).Execute(ctx, Caller{TenantID: tenantA, UserID: alice}, "risks")
	require.NoError(t, err)
	assert.Len(t, still, 2)
}

// ---------------------------------------------------------------------------
// Validation and conflict
// ---------------------------------------------------------------------------

func TestSavedViews_Validation(t *testing.T) {
	ctx := context.Background()
	repo := newMemRepo()
	caller := Caller{TenantID: uuid.New(), UserID: uuid.New()}
	uc := NewCreateSavedViewUseCase(repo)

	_, err := uc.Execute(ctx, caller, CreateSavedViewInput{TableID: "", Name: "x"})
	assert.ErrorIs(t, err, domain.ErrValidation)

	_, err = uc.Execute(ctx, caller, CreateSavedViewInput{TableID: "risks", Name: "   "})
	assert.ErrorIs(t, err, domain.ErrValidation)

	_, err = uc.Execute(ctx, caller, CreateSavedViewInput{
		TableID: "risks", Name: "too many facets",
		State: domain.SavedViewState{Filters: manyFacets(domain.MaxSavedViewFacets + 1)},
	})
	assert.ErrorIs(t, err, domain.ErrValidation, "the state column is not an unbounded write primitive")

	// A sort with no column is dropped rather than stored half-formed.
	created, err := uc.Execute(ctx, caller, CreateSavedViewInput{
		TableID: "risks", Name: "empty sort",
		State: domain.SavedViewState{Sort: &domain.SavedViewSort{Key: "  ", Dir: "desc"}},
	})
	require.NoError(t, err)
	assert.Nil(t, created.State.Sort)
}

func TestSavedViews_Conflict_SameNameTwiceOnTheSameTable(t *testing.T) {
	ctx := context.Background()
	repo := newMemRepo()
	caller := Caller{TenantID: uuid.New(), UserID: uuid.New()}
	uc := NewCreateSavedViewUseCase(repo)

	_, err := uc.Execute(ctx, caller, CreateSavedViewInput{TableID: "risks", Name: "Comité T3"})
	require.NoError(t, err)

	_, err = uc.Execute(ctx, caller, CreateSavedViewInput{TableID: "risks", Name: "Comité T3"})
	assert.ErrorIs(t, err, domain.ErrConflict)

	// The same name on another register is fine.
	_, err = uc.Execute(ctx, caller, CreateSavedViewInput{TableID: "assets", Name: "Comité T3"})
	assert.NoError(t, err)
}

// clientAnswer is exactly what the handler puts on the wire — writeAppError
// sends HTTPStatusFromError + MessageFromError, and nothing else. AppError.Detail
// carries the id for the server log and never leaves the process, so comparing
// full Error() strings would compare something the caller cannot observe.
func clientAnswer(err error) [2]string {
	return [2]string{
		fmt.Sprint(domain.HTTPStatusFromError(err)),
		domain.MessageFromError(err),
	}
}

func manyFacets(n int) map[string][]string {
	out := make(map[string][]string, n)
	for i := 0; i < n; i++ {
		out[uuid.New().String()] = []string{"v"}
	}
	return out
}
