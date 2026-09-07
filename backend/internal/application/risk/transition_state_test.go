// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only

package risk

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/opendefender/openrisk/internal/domain"
)

// ---------------------------------------------------------------------------
// Fakes for the two guard ports.
// ---------------------------------------------------------------------------

type fakeInspector struct {
	plans []MitigationSnapshot
	err   error
}

func (f *fakeInspector) SnapshotForRisk(context.Context, uuid.UUID, uuid.UUID) ([]MitigationSnapshot, error) {
	return f.plans, f.err
}

type fakeApprovals struct {
	approved bool
	pending  *uuid.UUID
	err      error
}

func (f *fakeApprovals) HasApprovedAcceptance(context.Context, uuid.UUID, uuid.UUID) (bool, *uuid.UUID, error) {
	return f.approved, f.pending, f.err
}

// stateRepo is a minimal in-memory RiskRepository for the FSM tests. It is
// tenant-scoped like the real one: a mismatched tenant reads back as nil.
type stateRepo struct {
	risk    *domain.Risk
	tenant  uuid.UUID
	updated *domain.Risk
	audits  []*domain.AuditLogEntry
}

func (r *stateRepo) GetByID(_ context.Context, id, tenantID uuid.UUID) (*domain.Risk, error) {
	if r.risk == nil || r.risk.ID != id || tenantID != r.tenant {
		return nil, nil
	}
	clone := *r.risk
	return &clone, nil
}

func (r *stateRepo) Update(_ context.Context, risk *domain.Risk) error {
	r.updated = risk
	r.risk = risk
	return nil
}

func (r *stateRepo) CreateAuditEntry(_ context.Context, e *domain.AuditLogEntry) error {
	r.audits = append(r.audits, e)
	return nil
}

// Unused parts of the port.
func (r *stateRepo) Create(context.Context, *domain.Risk) error { return nil }
func (r *stateRepo) List(context.Context, uuid.UUID, domain.RiskQuery) (*domain.PaginatedResult[domain.Risk], error) {
	return nil, nil
}
func (r *stateRepo) Delete(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (r *stateRepo) Count(context.Context, uuid.UUID) (int64, error)    { return 0, nil }
func (r *stateRepo) UpdateScore(context.Context, uuid.UUID, uuid.UUID, float64, string) error {
	return nil
}
func (r *stateRepo) GetRiskScore(context.Context, uuid.UUID, uuid.UUID) (float64, error) {
	return 0, nil
}
func (r *stateRepo) GetRisksByAssetID(context.Context, uuid.UUID, uuid.UUID) ([]domain.RiskForScoring, error) {
	return nil, nil
}
func (r *stateRepo) GetHistory(context.Context, uuid.UUID, uuid.UUID, int, int) ([]domain.AuditLogEntry, error) {
	return nil, nil
}
func (r *stateRepo) GetBySource(context.Context, uuid.UUID, string) ([]domain.Risk, error) {
	return nil, nil
}
func (r *stateRepo) GetByCVE(context.Context, string, uuid.UUID) (*domain.Risk, error) {
	return nil, nil
}
func (r *stateRepo) BulkUpdate(context.Context, uuid.UUID, []domain.RiskUpdate) (int64, error) {
	return 0, nil
}
func (r *stateRepo) BulkDelete(context.Context, []uuid.UUID, uuid.UUID) (int64, error) { return 0, nil }
func (r *stateRepo) BulkCreate(context.Context, []*domain.Risk) (int64, error)         { return 0, nil }

func (r *stateRepo) BulkApply(
	context.Context, uuid.UUID, []uuid.UUID, func(*domain.Risk) error,
) ([]domain.RiskMutation, error) {
	return nil, nil
}

func newFixture(state domain.RiskState) (*stateRepo, uuid.UUID, uuid.UUID) {
	tenant, id := uuid.New(), uuid.New()
	r := &domain.Risk{ID: id, TenantID: tenant, Title: "Fuite S3"}
	r.SetState(state)
	return &stateRepo{risk: r, tenant: tenant}, tenant, id
}

func donePlan() MitigationSnapshot {
	return MitigationSnapshot{ID: uuid.New(), Ref: "MIT-ABC123", Active: true, TotalSubActions: 3, CompletedSubActions: 3}
}

func openPlan(remaining int) MitigationSnapshot {
	return MitigationSnapshot{ID: uuid.New(), Ref: "MIT-14", Active: true, TotalSubActions: 3, CompletedSubActions: 3 - remaining}
}

// ---------------------------------------------------------------------------
// Guard 1 — IN_TREATMENT requires at least one active mitigation.
// ---------------------------------------------------------------------------

func TestTransition_InTreatment_RequiresAnActiveMitigation(t *testing.T) {
	repo, tenant, id := newFixture(domain.StateTreatmentPlanned)
	uc := NewTransitionRiskStateUseCase(repo).
		WithMitigations(&fakeInspector{plans: nil})

	_, err := uc.Execute(context.Background(), tenant, id, TransitionRiskStateInput{To: domain.StateInTreatment})
	if err == nil {
		t.Fatal("moving into treatment with no mitigation must be refused")
	}
	if repo.updated != nil {
		t.Fatal("a refused transition must not have written anything")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "mitigation") {
		t.Fatalf("the reason must name the blocker, got %q", err)
	}

	// A cancelled plan is not work in progress.
	uc = NewTransitionRiskStateUseCase(repo).WithMitigations(&fakeInspector{
		plans: []MitigationSnapshot{{ID: uuid.New(), Active: false}},
	})
	if _, err := uc.Execute(context.Background(), tenant, id, TransitionRiskStateInput{To: domain.StateInTreatment}); err == nil {
		t.Fatal("a cancelled mitigation must not satisfy the guard")
	}

	// With a real plan it goes through, and the legacy fields follow.
	uc = NewTransitionRiskStateUseCase(repo).WithMitigations(&fakeInspector{plans: []MitigationSnapshot{openPlan(2)}})
	got, err := uc.Execute(context.Background(), tenant, id, TransitionRiskStateInput{To: domain.StateInTreatment})
	if err != nil {
		t.Fatalf("expected the transition to be allowed: %v", err)
	}
	if got.LifecycleState != domain.StateInTreatment {
		t.Fatalf("state = %q", got.LifecycleState)
	}
	if got.Status != domain.RiskInProgress || got.LifecyclePhase != domain.PhaseTreated {
		t.Fatalf("legacy fields did not follow the state: status=%q phase=%q", got.Status, got.LifecyclePhase)
	}
}

// ---------------------------------------------------------------------------
// Guard 2 — MITIGATED requires 100% of sub-actions complete.
// ---------------------------------------------------------------------------

func TestTransition_Mitigated_RequiresEverySubActionDone(t *testing.T) {
	repo, tenant, id := newFixture(domain.StateInTreatment)
	uc := NewTransitionRiskStateUseCase(repo).
		WithMitigations(&fakeInspector{plans: []MitigationSnapshot{openPlan(2)}})

	_, err := uc.Execute(context.Background(), tenant, id, TransitionRiskStateInput{To: domain.StateMitigated})
	if err == nil {
		t.Fatal("closing treatment with open sub-actions must be refused")
	}
	// The reason must be actionable: the count AND the plan reference.
	if !strings.Contains(err.Error(), "2") || !strings.Contains(err.Error(), "MIT-14") {
		t.Fatalf("reason must quote the remaining count and the plan, got %q", err)
	}
	if repo.updated != nil {
		t.Fatal("a refused transition must not have written anything")
	}

	// One plan finished but another still open → still refused.
	uc = NewTransitionRiskStateUseCase(repo).WithMitigations(&fakeInspector{
		plans: []MitigationSnapshot{donePlan(), openPlan(1)},
	})
	if _, err := uc.Execute(context.Background(), tenant, id, TransitionRiskStateInput{To: domain.StateMitigated}); err == nil {
		t.Fatal("every active plan must be complete, not just one")
	}

	// Everything done → allowed.
	uc = NewTransitionRiskStateUseCase(repo).WithMitigations(&fakeInspector{plans: []MitigationSnapshot{donePlan()}})
	got, err := uc.Execute(context.Background(), tenant, id, TransitionRiskStateInput{To: domain.StateMitigated})
	if err != nil {
		t.Fatalf("expected the transition to be allowed: %v", err)
	}
	if got.LifecycleState != domain.StateMitigated || got.Status != domain.RiskMitigated {
		t.Fatalf("state=%q status=%q", got.LifecycleState, got.Status)
	}
	if got.LastMitigatedAt == nil {
		t.Fatal("reaching MITIGATED must stamp last_mitigated_at")
	}
}

// A plan with no sub-actions at all counts as complete: there is nothing left
// to do on it. The guard is "no remaining work", not "someone made a checklist".
func TestTransition_Mitigated_PlanWithoutSubActionsCounts(t *testing.T) {
	repo, tenant, id := newFixture(domain.StateInTreatment)
	uc := NewTransitionRiskStateUseCase(repo).WithMitigations(&fakeInspector{
		plans: []MitigationSnapshot{{ID: uuid.New(), Ref: "MIT-EMPTY", Active: true}},
	})
	if _, err := uc.Execute(context.Background(), tenant, id, TransitionRiskStateInput{To: domain.StateMitigated}); err != nil {
		t.Fatalf("a plan with no sub-actions has no remaining work: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Guard 3 — RESIDUAL_ACCEPTED requires a validated Governance approval.
// ---------------------------------------------------------------------------

func TestTransition_ResidualAccepted_RequiresGovernanceApproval(t *testing.T) {
	repo, tenant, id := newFixture(domain.StateInTreatment)

	uc := NewTransitionRiskStateUseCase(repo).
		WithMitigations(&fakeInspector{plans: []MitigationSnapshot{openPlan(1)}}).
		WithApprovals(&fakeApprovals{approved: false})
	_, err := uc.Execute(context.Background(), tenant, id, TransitionRiskStateInput{To: domain.StateResidualAccepted})
	if err == nil {
		t.Fatal("accepting residual risk without an approval must be refused")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "gouvernance") {
		t.Fatalf("reason must point at Governance, got %q", err)
	}

	// A request that exists but is still pending is NOT an approval, and the
	// reason says which one to go and chase.
	pending := uuid.New()
	uc = NewTransitionRiskStateUseCase(repo).
		WithMitigations(&fakeInspector{plans: []MitigationSnapshot{openPlan(1)}}).
		WithApprovals(&fakeApprovals{approved: false, pending: &pending})
	_, err = uc.Execute(context.Background(), tenant, id, TransitionRiskStateInput{To: domain.StateResidualAccepted})
	if err == nil || !strings.Contains(err.Error(), pending.String()[:8]) {
		t.Fatalf("a pending request must be named in the reason, got %v", err)
	}

	// Approved → allowed, even with sub-actions still open: accepting residual
	// risk is precisely the decision to stop treating it.
	uc = NewTransitionRiskStateUseCase(repo).
		WithMitigations(&fakeInspector{plans: []MitigationSnapshot{openPlan(3)}}).
		WithApprovals(&fakeApprovals{approved: true})
	got, err := uc.Execute(context.Background(), tenant, id, TransitionRiskStateInput{To: domain.StateResidualAccepted})
	if err != nil {
		t.Fatalf("an approved acceptance must go through: %v", err)
	}
	if got.LifecycleState != domain.StateResidualAccepted || got.Status != domain.RiskAccepted {
		t.Fatalf("state=%q status=%q", got.LifecycleState, got.Status)
	}
}

// An unwired guard BLOCKS. An unverifiable precondition is not a satisfied one,
// and in a security tool the honest failure is the safe one.
func TestTransition_UnwiredGuardBlocksRatherThanPasses(t *testing.T) {
	repo, tenant, id := newFixture(domain.StateInTreatment)
	uc := NewTransitionRiskStateUseCase(repo) // no inspector, no approvals

	for _, target := range []domain.RiskState{domain.StateMitigated, domain.StateResidualAccepted} {
		if _, err := uc.Execute(context.Background(), tenant, id, TransitionRiskStateInput{To: target}); err == nil {
			t.Fatalf("%s must be blocked when its guard cannot be evaluated", target)
		}
	}
	// A guard-free transition still works with nothing wired.
	repo2, tenant2, id2 := newFixture(domain.StateAssessed)
	if _, err := NewTransitionRiskStateUseCase(repo2).Execute(context.Background(), tenant2, id2,
		TransitionRiskStateInput{To: domain.StateTreatmentPlanned}); err != nil {
		t.Fatalf("an unguarded transition must not need any port: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Graph, tenancy and audit
// ---------------------------------------------------------------------------

func TestTransition_RejectsGraphViolationsAndNoOps(t *testing.T) {
	repo, tenant, id := newFixture(domain.StateIdentified)
	uc := NewTransitionRiskStateUseCase(repo).WithMitigations(&fakeInspector{plans: []MitigationSnapshot{donePlan()}})

	// Skipping two steps.
	if _, err := uc.Execute(context.Background(), tenant, id, TransitionRiskStateInput{To: domain.StateInTreatment}); err == nil {
		t.Fatal("identified → in_treatment skips the graph and must be refused")
	}
	// A no-op.
	if _, err := uc.Execute(context.Background(), tenant, id, TransitionRiskStateInput{To: domain.StateIdentified}); err == nil {
		t.Fatal("transitioning to the current state must be refused")
	}
	// An unknown state.
	if _, err := uc.Execute(context.Background(), tenant, id, TransitionRiskStateInput{To: domain.RiskState("done")}); err == nil {
		t.Fatal("an unknown target must be a validation error")
	}
	if repo.updated != nil {
		t.Fatal("none of those should have written anything")
	}
}

func TestTransition_CrossTenantReadsAsNotFound(t *testing.T) {
	repo, _, id := newFixture(domain.StateIdentified)
	_, err := NewTransitionRiskStateUseCase(repo).
		Execute(context.Background(), uuid.New(), id, TransitionRiskStateInput{To: domain.StateAssessed})
	if err == nil {
		t.Fatal("a risk from another tenant must not be transitionable")
	}
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("cross-tenant access must read as not-found, got %v", err)
	}
}

func TestTransition_RecordsAnAuditEntry(t *testing.T) {
	repo, tenant, id := newFixture(domain.StateIdentified)
	actor := uuid.New()
	if _, err := NewTransitionRiskStateUseCase(repo).Execute(context.Background(), tenant, id,
		TransitionRiskStateInput{To: domain.StateAssessed, Comment: "scored with the RSSI", Actor: actor}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(repo.audits) != 1 {
		t.Fatalf("expected one audit entry, got %d", len(repo.audits))
	}
	e := repo.audits[0]
	if e.ChangedBy != actor || e.Action != "lifecycle_transition" {
		t.Fatalf("audit entry lost the actor or the action: %+v", e)
	}
	if e.OldValue["lifecycle_state"] != domain.StateIdentified || e.NewValue["lifecycle_state"] != domain.StateAssessed {
		t.Fatalf("audit entry did not record the move: %+v", e)
	}
	if e.NewValue["comment"] != "scored with the RSSI" {
		t.Fatal("the comment must be part of the trail — it is the why")
	}
}

func TestTransition_CommentIsLengthLimited(t *testing.T) {
	repo, tenant, id := newFixture(domain.StateIdentified)
	long := strings.Repeat("a", 1001)
	if _, err := NewTransitionRiskStateUseCase(repo).Execute(context.Background(), tenant, id,
		TransitionRiskStateInput{To: domain.StateAssessed, Comment: long}); err == nil {
		t.Fatal("an oversized comment must be refused")
	}
}

// ---------------------------------------------------------------------------
// GET /risks/:id/transitions
// ---------------------------------------------------------------------------

func TestAvailableTransitions_ExposesBlockersRatherThanHidingThem(t *testing.T) {
	repo, tenant, id := newFixture(domain.StateInTreatment)
	uc := NewTransitionRiskStateUseCase(repo).
		WithMitigations(&fakeInspector{plans: []MitigationSnapshot{openPlan(2)}}).
		WithApprovals(&fakeApprovals{approved: false})

	view, err := uc.AvailableTransitions(context.Background(), tenant, id, "fr")
	if err != nil {
		t.Fatalf("AvailableTransitions: %v", err)
	}
	if view.Current != domain.StateInTreatment || view.CurrentLabel == "" {
		t.Fatalf("current state not reported: %+v", view)
	}
	if len(view.Options) != len(domain.StateInTreatment.NextStates()) {
		t.Fatalf("every reachable state must be listed, got %d", len(view.Options))
	}

	byState := map[domain.RiskState]domain.TransitionOption{}
	for _, o := range view.Options {
		byState[o.To] = o
	}

	mitigated := byState[domain.StateMitigated]
	if mitigated.Allowed {
		t.Fatal("mitigated is blocked and must be reported as such")
	}
	if !strings.Contains(mitigated.Reason, "MIT-14") || !strings.Contains(mitigated.Reason, "2") {
		t.Fatalf("the blocker must be concrete, got %q", mitigated.Reason)
	}
	if mitigated.Guard != domain.GuardSubActionsComplete {
		t.Fatalf("the failed guard must be named so the UI can offer a way out, got %q", mitigated.Guard)
	}

	accepted := byState[domain.StateResidualAccepted]
	if accepted.Allowed || accepted.Guard != domain.GuardGovernanceApproval {
		t.Fatalf("residual acceptance should be blocked on the approval guard: %+v", accepted)
	}

	// Stepping back to re-plan the treatment has no guard.
	if back := byState[domain.StateTreatmentPlanned]; !back.Allowed {
		t.Fatalf("going back to re-plan must stay available: %+v", back)
	}

	// The view points at the NEAREST rung forward and carries its reason.
	if view.Next == "" || view.BlockedReason == "" {
		t.Fatalf("a blocked risk must still report its next step and why: %+v", view)
	}
}

func TestAvailableTransitions_AllowedWhenGuardsAreSatisfied(t *testing.T) {
	repo, tenant, id := newFixture(domain.StateInTreatment)
	view, err := NewTransitionRiskStateUseCase(repo).
		WithMitigations(&fakeInspector{plans: []MitigationSnapshot{donePlan()}}).
		WithApprovals(&fakeApprovals{approved: true}).
		AvailableTransitions(context.Background(), tenant, id, "en")
	if err != nil {
		t.Fatalf("AvailableTransitions: %v", err)
	}
	for _, o := range view.Options {
		if !o.Allowed {
			t.Fatalf("%s should be allowed: %s", o.To, o.Reason)
		}
		if o.Label == "" {
			t.Fatalf("%s has no label", o.To)
		}
	}
	if view.Next != domain.StateMitigated {
		t.Fatalf("the natural next step from treatment is mitigated, got %q", view.Next)
	}
}

// The "next step" must be the NEAREST rung forward, not the first one that
// happens to be allowed.
//
// From TREATMENT_PLANNED, IN_TREATMENT is blocked on the mitigation guard while
// CLOSED (early closure) is always allowed. Announcing "next: Clôturé" would
// point the user at abandoning the risk and hide the actual blocker — which is
// exactly what the first implementation did, visible the moment it was run.
func TestAvailableTransitions_NextIsTheNearestRungNotTheFirstAllowedOne(t *testing.T) {
	repo, tenant, id := newFixture(domain.StateTreatmentPlanned)
	view, err := NewTransitionRiskStateUseCase(repo).
		WithMitigations(&fakeInspector{plans: nil}). // no mitigation → in_treatment blocked
		AvailableTransitions(context.Background(), tenant, id, "fr")
	if err != nil {
		t.Fatalf("AvailableTransitions: %v", err)
	}
	if view.Next != domain.StateInTreatment {
		t.Fatalf("next = %q, want in_treatment (closed is further along the spine)", view.Next)
	}
	if view.BlockedReason == "" {
		t.Fatal("the nearest step is blocked, so its reason must be surfaced")
	}
	// Early closure is still offered as an option — it is just not "next".
	var sawClosed bool
	for _, o := range view.Options {
		if o.To == domain.StateClosed {
			sawClosed = o.Allowed
		}
	}
	if !sawClosed {
		t.Fatal("early closure must remain available as an option")
	}
}

func TestAvailableTransitions_NotFound(t *testing.T) {
	repo, tenant, _ := newFixture(domain.StateIdentified)
	if _, err := NewTransitionRiskStateUseCase(repo).
		AvailableTransitions(context.Background(), tenant, uuid.New(), "fr"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown risk must be not-found, got %v", err)
	}
}

// The whole point of the change: one flow, walked end to end, with the
// mitigation plan gating it.
func TestTransition_FullLifecycleWalk(t *testing.T) {
	repo, tenant, id := newFixture(domain.StateDraft)
	inspector := &fakeInspector{}
	uc := NewTransitionRiskStateUseCase(repo).
		WithMitigations(inspector).
		WithApprovals(&fakeApprovals{approved: false})
	ctx := context.Background()

	step := func(to domain.RiskState) {
		t.Helper()
		if _, err := uc.Execute(ctx, tenant, id, TransitionRiskStateInput{To: to}); err != nil {
			t.Fatalf("%s: %v", to, err)
		}
	}

	step(domain.StateIdentified)
	step(domain.StateAssessed)
	step(domain.StateTreatmentPlanned)

	// No mitigation yet → treatment is blocked.
	if _, err := uc.Execute(ctx, tenant, id, TransitionRiskStateInput{To: domain.StateInTreatment}); err == nil {
		t.Fatal("treatment must be blocked until a mitigation exists")
	}
	inspector.plans = []MitigationSnapshot{openPlan(2)}
	step(domain.StateInTreatment)

	// Sub-actions still open → mitigated is blocked.
	if _, err := uc.Execute(ctx, tenant, id, TransitionRiskStateInput{To: domain.StateMitigated}); err == nil {
		t.Fatal("mitigated must be blocked while sub-actions remain")
	}
	inspector.plans = []MitigationSnapshot{donePlan()}
	step(domain.StateMitigated)

	step(domain.StateClosed)
	step(domain.StateReopened)
	step(domain.StateAssessed) // and back into the flow

	if repo.risk.State() != domain.StateAssessed {
		t.Fatalf("final state = %q", repo.risk.State())
	}
	if len(repo.audits) != 8 {
		t.Fatalf("every accepted transition must be audited, got %d entries", len(repo.audits))
	}
}
