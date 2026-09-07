// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package risk

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/opendefender/openrisk/internal/domain"
)

// ---------------------------------------------------------------------------
// #581 — the bulk-action use case had NO test file at all, which is why a
// non-transactional loop could sit under doc comments promising atomicity, a
// declared remove_tags action could return "unknown action type", and a
// performedBy could be accepted and discarded, all at once and for months.
//
// The fake repository below reproduces the two guarantees the real one makes,
// so these tests are about the use case's behaviour on top of a correct store.
// That the GORM implementation actually keeps them — a real ROLLBACK, a real
// strict delete — is proven separately against SQLite in
// repository/gorm_risk_bulk_test.go. Neither test would catch the other's
// failure, which is why both exist.
// ---------------------------------------------------------------------------

type bulkRepo struct {
	MockRiskRepository
	tenant uuid.UUID
	risks  map[uuid.UUID]*domain.Risk

	// failOnSave makes the write of this particular risk fail, to exercise the
	// rollback path from the middle of a batch.
	failOnSave uuid.UUID
	// applyCalls counts BulkApply invocations (one transaction each).
	applyCalls int
}

func newBulkRepo(tenant uuid.UUID, risks ...*domain.Risk) *bulkRepo {
	m := map[uuid.UUID]*domain.Risk{}
	for _, r := range risks {
		m[r.ID] = r
	}
	return &bulkRepo{tenant: tenant, risks: m}
}

// BulkApply is all-or-nothing and tenant-scoped, like the GORM one: it stages
// every change and commits only if the whole batch succeeded.
func (r *bulkRepo) BulkApply(
	_ context.Context,
	tenantID uuid.UUID,
	ids []uuid.UUID,
	mutate func(*domain.Risk) error,
) ([]domain.RiskMutation, error) {
	r.applyCalls++
	if tenantID == uuid.Nil {
		return nil, domain.NewForbiddenError("tenant_id is required")
	}

	staged := map[uuid.UUID]*domain.Risk{}
	mutations := []domain.RiskMutation{}

	for _, id := range ids {
		live, ok := r.risks[id]
		// The tenant predicate is part of the lookup, so a foreign id is
		// indistinguishable from one that never existed.
		if !ok || live.TenantID != tenantID {
			return nil, domain.NewNotFoundError("risk", id)
		}
		clone := *live
		clone.Tags = append([]string(nil), live.Tags...)

		before := clone.BulkSnapshot()
		if err := mutate(&clone); err != nil {
			return nil, err
		}
		after := clone.BulkSnapshot()

		if id == r.failOnSave {
			return nil, errors.New("write failed")
		}

		staged[id] = &clone
		mutations = append(mutations, domain.RiskMutation{
			RiskID: id, Before: before, After: after,
			ChangedFields: changed(before, after),
		})
	}

	// Commit only now — nothing above touched the live map.
	for id, r2 := range staged {
		r.risks[id] = r2
	}
	return mutations, nil
}

func (r *bulkRepo) BulkDelete(_ context.Context, ids []uuid.UUID, tenantID uuid.UUID) (int64, error) {
	if tenantID == uuid.Nil {
		return 0, domain.NewForbiddenError("tenant_id is required")
	}
	for _, id := range ids {
		live, ok := r.risks[id]
		if !ok || live.TenantID != tenantID {
			// Strict: one missing id deletes nothing.
			return 0, domain.NewNotFoundError("risk", "one or more ids in the batch")
		}
	}
	for _, id := range ids {
		delete(r.risks, id)
	}
	return int64(len(ids)), nil
}

func changed(before, after map[string]interface{}) []string {
	out := []string{}
	for _, k := range []string{"status", "lifecycle_state", "assigned_to", "tags"} {
		if !equalish(before[k], after[k]) {
			out = append(out, k)
		}
	}
	return out
}

func equalish(a, b interface{}) bool {
	as, aok := a.([]string)
	bs, bok := b.([]string)
	if aok && bok {
		if len(as) != len(bs) {
			return false
		}
		for i := range as {
			if as[i] != bs[i] {
				return false
			}
		}
		return true
	}
	return a == b
}

// fakeJournal records what the use case appends.
type fakeJournal struct {
	events []*domain.AuditEvent
	fail   bool
}

func (j *fakeJournal) Append(_ context.Context, e *domain.AuditEvent) error {
	if j.fail {
		return errors.New("journal unavailable")
	}
	j.events = append(j.events, e)
	return nil
}

func seedRisk(tenant uuid.UUID, title string) *domain.Risk {
	r := &domain.Risk{ID: uuid.New(), TenantID: tenant, Title: title, Tags: []string{"pci"}}
	r.SetState(domain.StateIdentified)
	return r
}

func newUC(repo domain.RiskRepository) (*BulkActionUseCase, *fakeJournal) {
	j := &fakeJournal{}
	return NewBulkActionUseCase(repo, j), j
}

// ---------------------------------------------------------------------------
// Success
// ---------------------------------------------------------------------------

func TestBulkAction_Success(t *testing.T) {
	ctx := context.Background()
	tenant, actor := uuid.New(), uuid.New()
	a, b := seedRisk(tenant, "Fuite S3"), seedRisk(tenant, "Log4Shell")
	repo := newBulkRepo(tenant, a, b)
	uc, journal := newUC(repo)

	status := domain.RiskMitigated
	res, err := uc.Execute(ctx, tenant, BulkActionRequest{
		Type:    BulkActionChangeStatus,
		RiskIDs: []uuid.UUID{a.ID, b.ID},
		Status:  &status,
	}, actor)
	require.NoError(t, err)

	assert.Equal(t, 2, res.Total)
	assert.Equal(t, 2, res.Applied)
	assert.Equal(t, 2, res.Audited)
	assert.ElementsMatch(t, []uuid.UUID{a.ID, b.ID}, res.RiskIDs)

	// Both risks moved, and the canonical state moved WITH the status — writing
	// Status alone is what let the two disagree.
	for _, id := range []uuid.UUID{a.ID, b.ID} {
		got := repo.risks[id]
		assert.Equal(t, domain.StateMitigated, got.State())
		assert.Equal(t, got.State().DerivedStatus(), got.Status)
		assert.Equal(t, got.State().DerivedPhase(), got.LifecyclePhase)
	}
	assert.Equal(t, 1, repo.applyCalls, "the batch must be ONE transaction, not one per risk")
	_ = journal
}

func TestBulkAction_Success_AssignToAndAddTags(t *testing.T) {
	ctx := context.Background()
	tenant, actor, assignee := uuid.New(), uuid.New(), uuid.New()
	a := seedRisk(tenant, "Fuite S3")
	repo := newBulkRepo(tenant, a)
	uc, _ := newUC(repo)

	_, err := uc.Execute(ctx, tenant, BulkActionRequest{
		Type: BulkActionAssignTo, RiskIDs: []uuid.UUID{a.ID}, AssignToID: &assignee,
	}, actor)
	require.NoError(t, err)
	require.NotNil(t, repo.risks[a.ID].AssignedTo)
	assert.Equal(t, assignee, *repo.risks[a.ID].AssignedTo)

	_, err = uc.Execute(ctx, tenant, BulkActionRequest{
		Type: BulkActionAddTags, RiskIDs: []uuid.UUID{a.ID}, Tags: []string{"kev", "pci"},
	}, actor)
	require.NoError(t, err)
	// Sorted and de-duplicated: "pci" was already there, and the order is stable
	// rather than Go's randomised map order, so an unchanged set does not read as
	// a change on every write.
	assert.Equal(t, []string{"kev", "pci"}, []string(repo.risks[a.ID].Tags))
}

// ---------------------------------------------------------------------------
// Criterion 3 — remove_tags, which used to answer "unknown action type"
// ---------------------------------------------------------------------------

func TestBulkAction_RemoveTags(t *testing.T) {
	ctx := context.Background()
	tenant, actor := uuid.New(), uuid.New()
	a := seedRisk(tenant, "Fuite S3")
	a.Tags = []string{"pci", "kev", "sox"}
	repo := newBulkRepo(tenant, a)
	uc, journal := newUC(repo)

	res, err := uc.Execute(ctx, tenant, BulkActionRequest{
		Type: BulkActionRemoveTags, RiskIDs: []uuid.UUID{a.ID}, Tags: []string{"kev"},
	}, actor)
	require.NoError(t, err, "remove_tags must be handled, not fall through to the default branch")
	assert.Equal(t, 1, res.Applied)
	assert.Equal(t, []string{"pci", "sox"}, []string(repo.risks[a.ID].Tags))
	assert.Len(t, journal.events, 1)

	// Removing a tag the risk does not carry is a no-op, not an error.
	res, err = uc.Execute(ctx, tenant, BulkActionRequest{
		Type: BulkActionRemoveTags, RiskIDs: []uuid.UUID{a.ID}, Tags: []string{"never-applied"},
	}, actor)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Applied)
	assert.Equal(t, []string{"pci", "sox"}, []string(repo.risks[a.ID].Tags))
	// …and it is journalled as a change of nothing rather than silently.
	assert.Empty(t, journal.events[1].ChangedFields)

	// Removing every tag leaves an empty set, not a nil surprise.
	_, err = uc.Execute(ctx, tenant, BulkActionRequest{
		Type: BulkActionRemoveTags, RiskIDs: []uuid.UUID{a.ID}, Tags: []string{"pci", "sox"},
	}, actor)
	require.NoError(t, err)
	assert.Empty(t, repo.risks[a.ID].Tags)

	_, err = uc.Execute(ctx, tenant, BulkActionRequest{
		Type: BulkActionRemoveTags, RiskIDs: []uuid.UUID{a.ID},
	}, actor)
	assert.ErrorIs(t, err, domain.ErrValidation, "remove_tags with no tags is a validation error")
}

// ---------------------------------------------------------------------------
// Criterion 1 — all-or-nothing
// ---------------------------------------------------------------------------

func TestBulkAction_Rollback_OneFailureModifiesNothing(t *testing.T) {
	ctx := context.Background()
	tenant, actor := uuid.New(), uuid.New()
	a, b, c := seedRisk(tenant, "A"), seedRisk(tenant, "B"), seedRisk(tenant, "C")
	repo := newBulkRepo(tenant, a, b, c)
	repo.failOnSave = c.ID // the LAST one fails, after two have been staged
	uc, journal := newUC(repo)

	status := domain.RiskMitigated
	res, err := uc.Execute(ctx, tenant, BulkActionRequest{
		Type:    BulkActionChangeStatus,
		RiskIDs: []uuid.UUID{a.ID, b.ID, c.ID},
		Status:  &status,
	}, actor)

	require.Error(t, err)
	assert.Nil(t, res, "a failed batch returns no partial result to misread")

	// The whole point: the two that "succeeded" are NOT modified.
	for _, r := range []*domain.Risk{a, b, c} {
		assert.Equal(t, domain.StateIdentified, repo.risks[r.ID].State(),
			"no risk may be modified when any item in the batch fails")
	}
	assert.Empty(t, journal.events, "a rolled-back batch must journal nothing")
}

func TestBulkAction_Rollback_AStaleIDFailsTheWholeBatch(t *testing.T) {
	ctx := context.Background()
	tenant, actor := uuid.New(), uuid.New()
	a := seedRisk(tenant, "A")
	repo := newBulkRepo(tenant, a)
	uc, journal := newUC(repo)

	status := domain.RiskMitigated
	// The common failure: a selection made before somebody else deleted one row.
	_, err := uc.Execute(ctx, tenant, BulkActionRequest{
		Type:    BulkActionChangeStatus,
		RiskIDs: []uuid.UUID{a.ID, uuid.New()},
		Status:  &status,
	}, actor)

	assert.ErrorIs(t, err, domain.ErrNotFound)
	assert.Equal(t, domain.StateIdentified, repo.risks[a.ID].State())
	assert.Empty(t, journal.events)
}

// ---------------------------------------------------------------------------
// Criterion 2 — one audit record per modified risk, carrying performedBy
// ---------------------------------------------------------------------------

func TestBulkAction_WritesOneAuditRecordPerModifiedRisk(t *testing.T) {
	ctx := context.Background()
	tenant, actor, assignee := uuid.New(), uuid.New(), uuid.New()
	a, b := seedRisk(tenant, "A"), seedRisk(tenant, "B")
	repo := newBulkRepo(tenant, a, b)
	uc, journal := newUC(repo)

	res, err := uc.Execute(ctx, tenant, BulkActionRequest{
		Type:          BulkActionAssignTo,
		RiskIDs:       []uuid.UUID{a.ID, b.ID},
		AssignToID:    &assignee,
		Justification: "revue trimestrielle",
	}, actor)
	require.NoError(t, err)

	require.Len(t, journal.events, 2, "one record per modified risk, not one per batch")
	assert.Equal(t, 2, res.Audited)

	seen := map[string]bool{}
	for _, e := range journal.events {
		// performedBy is the value the old code accepted and discarded, so a
		// supervisor asking "who reassigned these" had no answer.
		require.NotNil(t, e.ActorID, "the acting user must be attributed")
		assert.Equal(t, actor, *e.ActorID)
		assert.Equal(t, tenant, e.TenantID)
		assert.Equal(t, "risk", e.EntityType)
		assert.Equal(t, domain.AuditActionUpdate, e.Action)
		assert.Equal(t, "explicit", e.Source)
		assert.Contains(t, e.Summary, "revue trimestrielle", "the justification is carried")
		assert.Contains(t, e.ChangedFields, "assigned_to")
		// before → after is real, not a placeholder.
		assert.Nil(t, e.Before["assigned_to"])
		assert.Equal(t, assignee.String(), e.After["assigned_to"])
		assert.False(t, e.CreatedAt.IsZero())
		seen[e.EntityID] = true
	}
	assert.True(t, seen[a.ID.String()] && seen[b.ID.String()], "every modified risk is named")
}

// D-004: the trail is observable best-effort. A journal outage must not fail a
// mutation that already committed — but it must be VISIBLE, not swallowed.
func TestBulkAction_JournalFailure_IsReportedNotFatal(t *testing.T) {
	ctx := context.Background()
	tenant, actor := uuid.New(), uuid.New()
	a := seedRisk(tenant, "A")
	repo := newBulkRepo(tenant, a)
	journal := &fakeJournal{fail: true}
	uc := NewBulkActionUseCase(repo, journal)

	status := domain.RiskMitigated
	res, err := uc.Execute(ctx, tenant, BulkActionRequest{
		Type: BulkActionChangeStatus, RiskIDs: []uuid.UUID{a.ID}, Status: &status,
	}, actor)

	require.NoError(t, err, "the write committed; reporting it as failed would be worse")
	assert.Equal(t, 1, res.Applied)
	assert.Equal(t, 0, res.Audited, "the shortfall is reported so it cannot pass unnoticed")
	assert.Equal(t, domain.StateMitigated, repo.risks[a.ID].State())
}

// ---------------------------------------------------------------------------
// NotFound
// ---------------------------------------------------------------------------

func TestBulkAction_NotFound(t *testing.T) {
	ctx := context.Background()
	tenant, actor := uuid.New(), uuid.New()
	repo := newBulkRepo(tenant)
	uc, _ := newUC(repo)

	status := domain.RiskMitigated
	_, err := uc.Execute(ctx, tenant, BulkActionRequest{
		Type: BulkActionChangeStatus, RiskIDs: []uuid.UUID{uuid.New()}, Status: &status,
	}, actor)
	assert.ErrorIs(t, err, domain.ErrNotFound)

	err2 := func() error {
		_, e := uc.Execute(ctx, tenant, BulkActionRequest{
			Type: BulkActionDeleteRisks, RiskIDs: []uuid.UUID{uuid.New()},
		}, actor)
		return e
	}()
	assert.ErrorIs(t, err2, domain.ErrNotFound)
}

// ---------------------------------------------------------------------------
// Criterion 4 — cross-tenant
// ---------------------------------------------------------------------------

func TestBulkAction_CrossTenant_IsIndistinguishableFromAFabricatedID(t *testing.T) {
	ctx := context.Background()
	tenantA, tenantB, actor := uuid.New(), uuid.New(), uuid.New()
	mine := seedRisk(tenantA, "mine")
	theirs := seedRisk(tenantB, "theirs")
	repo := newBulkRepo(tenantA, mine, theirs)
	uc, journal := newUC(repo)

	status := domain.RiskMitigated

	// The reference answer: an id that was never issued.
	_, fabricatedErr := uc.Execute(ctx, tenantA, BulkActionRequest{
		Type: BulkActionChangeStatus, RiskIDs: []uuid.UUID{uuid.New()}, Status: &status,
	}, actor)
	require.ErrorIs(t, fabricatedErr, domain.ErrNotFound)

	// A real risk, belonging to tenant B, named by a caller in tenant A.
	_, foreignErr := uc.Execute(ctx, tenantA, BulkActionRequest{
		Type: BulkActionChangeStatus, RiskIDs: []uuid.UUID{theirs.ID}, Status: &status,
	}, actor)
	assert.ErrorIs(t, foreignErr, domain.ErrNotFound)
	assert.NotErrorIs(t, foreignErr, domain.ErrForbidden, "a 403 would confirm the risk exists")
	assert.Equal(t,
		domain.HTTPStatusFromError(fabricatedErr), domain.HTTPStatusFromError(foreignErr))
	assert.Equal(t,
		domain.MessageFromError(fabricatedErr), domain.MessageFromError(foreignErr),
		"the client-visible answer must be identical for a foreign and a fabricated id")

	// Nothing of tenant B's was touched, and mixing a foreign id into an
	// otherwise valid batch modifies nothing at all.
	assert.Equal(t, domain.StateIdentified, repo.risks[theirs.ID].State())
	_, err := uc.Execute(ctx, tenantA, BulkActionRequest{
		Type: BulkActionChangeStatus, RiskIDs: []uuid.UUID{mine.ID, theirs.ID}, Status: &status,
	}, actor)
	assert.ErrorIs(t, err, domain.ErrNotFound)
	assert.Equal(t, domain.StateIdentified, repo.risks[mine.ID].State())
	assert.Empty(t, journal.events)

	// Delete takes the same path.
	_, err = uc.Execute(ctx, tenantA, BulkActionRequest{
		Type: BulkActionDeleteRisks, RiskIDs: []uuid.UUID{theirs.ID},
	}, actor)
	assert.ErrorIs(t, err, domain.ErrNotFound)
	assert.Contains(t, repo.risks, theirs.ID, "another tenant's risk must survive")
}

// ---------------------------------------------------------------------------
// Unauthorized
// ---------------------------------------------------------------------------

// A zero tenant must fail closed rather than run an unscoped mutation. This is
// the use case's own guard; the HTTP 403 for a caller without risks:update is
// enforced by the route's RequirePermission middleware and pinned in
// handler/authz_route_coverage_test.
func TestBulkAction_Unauthorized(t *testing.T) {
	ctx := context.Background()
	tenant, actor := uuid.New(), uuid.New()
	a := seedRisk(tenant, "A")
	repo := newBulkRepo(tenant, a)
	uc, journal := newUC(repo)

	status := domain.RiskMitigated
	for _, tc := range []BulkActionType{
		BulkActionChangeStatus, BulkActionAddTags, BulkActionRemoveTags, BulkActionDeleteRisks,
	} {
		_, err := uc.Execute(ctx, uuid.Nil, BulkActionRequest{
			Type: tc, RiskIDs: []uuid.UUID{a.ID}, Status: &status, Tags: []string{"x"},
		}, actor)
		assert.ErrorIs(t, err, domain.ErrForbidden, "action %q must refuse a zero tenant", tc)
	}
	assert.Equal(t, domain.StateIdentified, repo.risks[a.ID].State())
	assert.Empty(t, journal.events)
	assert.Equal(t, 0, repo.applyCalls, "a refused call must never reach the store")
}

// ---------------------------------------------------------------------------
// Criteria 5 and 6 — the limit guards, pinned so they cannot regress
// ---------------------------------------------------------------------------

func TestBulkAction_RejectsMoreThanOneHundredItems(t *testing.T) {
	ctx := context.Background()
	tenant := uuid.New()
	repo := newBulkRepo(tenant)
	uc, _ := newUC(repo)

	ids := make([]uuid.UUID, MaxBulkActionItems+1)
	for i := range ids {
		ids[i] = uuid.New()
	}
	status := domain.RiskMitigated
	_, err := uc.Execute(ctx, tenant, BulkActionRequest{
		Type: BulkActionChangeStatus, RiskIDs: ids, Status: &status,
	}, uuid.New())

	require.ErrorIs(t, err, domain.ErrValidation)
	assert.Contains(t, err.Error(), "100", "the error must name the limit")
	assert.Equal(t, 0, repo.applyCalls)
}

func TestBulkAction_RejectsAnEmptySelection(t *testing.T) {
	ctx := context.Background()
	tenant := uuid.New()
	repo := newBulkRepo(tenant)
	uc, _ := newUC(repo)

	status := domain.RiskMitigated
	_, err := uc.Execute(ctx, tenant, BulkActionRequest{
		Type: BulkActionChangeStatus, RiskIDs: []uuid.UUID{}, Status: &status,
	}, uuid.New())
	assert.ErrorIs(t, err, domain.ErrValidation)
	assert.Equal(t, 0, repo.applyCalls)
}

func TestBulkAction_ValidatesActionParameters(t *testing.T) {
	ctx := context.Background()
	tenant, actor := uuid.New(), uuid.New()
	a := seedRisk(tenant, "A")
	repo := newBulkRepo(tenant, a)
	uc, _ := newUC(repo)
	ids := []uuid.UUID{a.ID}

	for name, req := range map[string]BulkActionRequest{
		"change_status without a status": {Type: BulkActionChangeStatus, RiskIDs: ids},
		"assign_to without an assignee":  {Type: BulkActionAssignTo, RiskIDs: ids},
		"add_tags without tags":          {Type: BulkActionAddTags, RiskIDs: ids},
		"unknown action":                 {Type: BulkActionType("teleport"), RiskIDs: ids},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := uc.Execute(ctx, tenant, req, actor)
			assert.ErrorIs(t, err, domain.ErrValidation)
		})
	}
}

// A repeated id must not make an otherwise valid batch look short and fail —
// strictness is enforced by counting rows against ids.
func TestBulkAction_DeduplicatesTheSelection(t *testing.T) {
	ctx := context.Background()
	tenant, actor := uuid.New(), uuid.New()
	a := seedRisk(tenant, "A")
	repo := newBulkRepo(tenant, a)
	uc, journal := newUC(repo)

	status := domain.RiskMitigated
	res, err := uc.Execute(ctx, tenant, BulkActionRequest{
		Type: BulkActionChangeStatus, RiskIDs: []uuid.UUID{a.ID, a.ID, a.ID}, Status: &status,
	}, actor)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Total)
	assert.Equal(t, 1, res.Applied)
	assert.Len(t, journal.events, 1, "one audit record per risk, not per mention")
}

// ---------------------------------------------------------------------------
// Criterion 7 — no doc comment claims a guarantee the code does not implement
// ---------------------------------------------------------------------------

// The two strings named in #581 asserted atomicity over a loop that had no
// transaction. They are gone; the guarantee they described is now real and made
// by the repository. This pins the correction: re-introducing the claim without
// the mechanism fails here.
func TestBulkAction_DocCommentsDoNotOverclaim(t *testing.T) {
	source, err := os.ReadFile("bulk_action.go")
	require.NoError(t, err)
	text := string(source)

	for _, claim := range []string{
		"MANDATORY: All operations must be atomic within a transaction",
		"Operation is atomic (all succeed or all fail)",
	} {
		assert.NotContains(t, text, claim,
			"this comment promised a guarantee the file did not implement (#581)")
	}

	// And the mechanism that makes the real guarantee is present.
	assert.Contains(t, text, "BulkApply",
		"atomicity is delegated to the repository's single-transaction BulkApply")
	assert.False(t, strings.Contains(text, "result.Failed"),
		"the partial-success tally is gone with the semantics it described (D-036)")
}
