// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only

package activation

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/opendefender/openrisk/internal/domain"
)

// fakeProbe counts its calls: criterion 2 allows exactly ONE resolution per
// GET /onboarding/state, and a probe that were called per step would be the
// per-step status call the criterion forbids.
type fakeProbe struct {
	data  domain.OnboardingStepData
	calls int
	fail  bool
}

func (p *fakeProbe) OnboardingStepData(_ context.Context, _, _ uuid.UUID) (domain.OnboardingStepData, error) {
	p.calls++
	if p.fail {
		return domain.OnboardingStepData{}, errors.New("probe down")
	}
	return p.data, nil
}

// Criterion 3: a step whose data exists is absent from the stepper count shown
// to that user.
func TestWizard_AutoSkippedStepsAreAbsentFromTheStepper(t *testing.T) {
	repo := newFakeRepo()
	probe := &fakeProbe{data: domain.OnboardingStepData{
		HasOrganizationProfile: true,
		HasFramework:           true,
	}}
	uc := newWizard(repo).WithStepProbe(probe)
	tenant, user := uuid.New(), uuid.New()

	state, err := uc.GetState(context.Background(), tenant, user)
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}

	if len(state.Steps) != 3 {
		t.Errorf("stepper shows %d steps (%v), want 3", len(state.Steps), state.Steps)
	}
	for _, s := range state.Steps {
		if s == string(domain.OnboardingStepOrganization) || s == string(domain.OnboardingStepFramework) {
			t.Errorf("skipped step %q is still in the stepper", s)
		}
	}
	if len(state.SkippedSteps) != 2 {
		t.Errorf("skipped = %v, want organization and framework", state.SkippedSteps)
	}
	// The cursor must not land on a step the client is forbidden to render.
	if state.CurrentStep == string(domain.OnboardingStepOrganization) {
		t.Errorf("the cursor parked on a skipped step: %q", state.CurrentStep)
	}
}

// Criterion 2: one call resolves the whole tunnel.
func TestWizard_StateResolvesAutoSkipInASingleProbe(t *testing.T) {
	repo := newFakeRepo()
	probe := &fakeProbe{data: domain.OnboardingStepData{HasUserProfile: true}}
	uc := newWizard(repo).WithStepProbe(probe)

	if _, err := uc.GetState(context.Background(), uuid.New(), uuid.New()); err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if probe.calls != 1 {
		t.Errorf("GET /onboarding/state probed %d times, want exactly 1", probe.calls)
	}
}

// `goal` is a preference, not a record. Nothing stored can prove what a user
// wants next, so skipping it would silently choose their landing page.
func TestWizard_GoalIsNeverSkippable(t *testing.T) {
	everything := domain.OnboardingStepData{
		HasOrganizationProfile: true,
		HasUserProfile:         true,
		HasFramework:           true,
		HasTeam:                true,
	}

	visible := everything.VisibleSteps()
	if len(visible) != 1 || visible[0] != domain.OnboardingStepGoal {
		t.Fatalf("visible steps = %v, want only the goal", visible)
	}
	if everything.SkipsStep(domain.OnboardingStepGoal) {
		t.Error("the goal step must never be skipped")
	}

	repo := newFakeRepo()
	uc := newWizard(repo).WithStepProbe(&fakeProbe{data: everything})
	state, err := uc.GetState(context.Background(), uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if len(state.Steps) != 1 || state.Steps[0] != string(domain.OnboardingStepGoal) {
		t.Errorf("a fully configured user must still answer the goal, got %v", state.Steps)
	}
}

// The cursor steps OVER hidden steps in both directions. Landing on one would
// strand the user on a screen the client must not draw.
func TestWizard_CursorStepsOverHiddenSteps(t *testing.T) {
	repo := newFakeRepo()
	// profile and framework hidden ⇒ visible: organization, goal, team
	probe := &fakeProbe{data: domain.OnboardingStepData{HasUserProfile: true, HasFramework: true}}
	uc := newWizard(repo).WithStepProbe(probe)
	tenant, user := uuid.New(), uuid.New()

	// Forward from organization: profile is hidden, so goal is next.
	state, err := uc.SaveStep(context.Background(), tenant, user, SaveStepInput{
		Step:    domain.OnboardingStepOrganization,
		Answers: domain.JSONMap{"industry": "banking", "country": "CM"},
	})
	if err != nil {
		t.Fatalf("SaveStep: %v", err)
	}
	if state.CurrentStep != string(domain.OnboardingStepGoal) {
		t.Errorf("forward cursor = %q, want goal (profile is hidden)", state.CurrentStep)
	}

	// Forward from goal: framework is hidden, so team is next.
	state, err = uc.SaveStep(context.Background(), tenant, user, SaveStepInput{
		Step:    domain.OnboardingStepGoal,
		Answers: domain.JSONMap{"goal": "compliance"},
	})
	if err != nil {
		t.Fatalf("SaveStep: %v", err)
	}
	if state.CurrentStep != string(domain.OnboardingStepTeam) {
		t.Errorf("forward cursor = %q, want team (framework is hidden)", state.CurrentStep)
	}

	// An explicit backwards Next naming a HIDDEN step is honoured as a direction:
	// the cursor lands on the nearest visible step behind it, never on the hidden
	// one and never staying put.
	state, err = uc.SaveStep(context.Background(), tenant, user, SaveStepInput{
		Step:    domain.OnboardingStepTeam,
		Answers: domain.JSONMap{},
		Next:    string(domain.OnboardingStepFramework),
	})
	if err != nil {
		t.Fatalf("SaveStep: %v", err)
	}
	if state.CurrentStep != string(domain.OnboardingStepGoal) {
		t.Errorf("backwards cursor = %q, want goal (framework is hidden)", state.CurrentStep)
	}
}

// Criterion 5 in the presence of skips: the resumed cursor still reads as a
// sensible "step N of M" rather than "step 0 of 3" or a negative index.
func TestWizard_StepIndexIsRelativeToVisibleSteps(t *testing.T) {
	repo := newFakeRepo()
	tenant, user := uuid.New(), uuid.New()
	repo.progress[user.String()] = &domain.OnboardingProgress{
		TenantID:    tenant,
		UserID:      user,
		CurrentStep: domain.OnboardingStepTeam,
	}
	// organization hidden ⇒ visible: profile, goal, framework, team
	uc := newWizard(repo).WithStepProbe(&fakeProbe{data: domain.OnboardingStepData{HasOrganizationProfile: true}})

	state, err := uc.GetState(context.Background(), tenant, user)
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if state.StepIndex != 3 || len(state.Steps) != 4 {
		t.Errorf("step %d of %d, want 3 of 4", state.StepIndex, len(state.Steps))
	}

	// A stored cursor parked on a NOW-hidden step must not render as index −1.
	repo.progress[user.String()].CurrentStep = domain.OnboardingStepOrganization
	state, err = uc.GetState(context.Background(), tenant, user)
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if state.StepIndex < 0 || state.StepIndex >= len(state.Steps) {
		t.Errorf("a cursor on a hidden step resolved to index %d of %d", state.StepIndex, len(state.Steps))
	}
}

// A probe failure must show every step, not hide one whose data does not exist.
// Showing a redundant step costs a click; hiding a needed one strands the user.
func TestWizard_ProbeFailureShowsEveryStep(t *testing.T) {
	repo := newFakeRepo()
	uc := newWizard(repo).WithStepProbe(&fakeProbe{fail: true})

	state, err := uc.GetState(context.Background(), uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("a probe failure must not fail the wizard: %v", err)
	}
	if len(state.Steps) != len(domain.OnboardingStepOrder) {
		t.Errorf("a failed probe hid %d steps", len(domain.OnboardingStepOrder)-len(state.Steps))
	}
	if len(state.SkippedSteps) != 0 {
		t.Errorf("a failed probe reported skips: %v", state.SkippedSteps)
	}
}

// No probe at all is the old behaviour, unchanged.
func TestWizard_NoProbeShowsEveryStep(t *testing.T) {
	state, err := newWizard(newFakeRepo()).GetState(context.Background(), uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if len(state.Steps) != len(domain.OnboardingStepOrder) {
		t.Errorf("steps = %v, want the full catalogue", state.Steps)
	}
}

// Complete parks the cursor on the last step this user actually saw, not on the
// catalogue's last step — which may be one they never walked.
func TestWizard_CompleteParksOnTheLastVisibleStep(t *testing.T) {
	repo := newFakeRepo()
	uc := newWizard(repo).WithStepProbe(&fakeProbe{data: domain.OnboardingStepData{HasTeam: true}})
	tenant, user := uuid.New(), uuid.New()

	state, err := uc.Complete(context.Background(), tenant, user)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if state.CurrentStep == string(domain.OnboardingStepTeam) {
		t.Error("the cursor parked on the hidden team step")
	}
	if state.CurrentStep != string(domain.OnboardingStepFramework) {
		t.Errorf("cursor = %q, want the last visible step (framework)", state.CurrentStep)
	}
	if !state.Completed {
		t.Error("Complete must complete")
	}
}

func TestOnboardingStepData_SkipsStep(t *testing.T) {
	full := domain.OnboardingStepData{
		HasOrganizationProfile: true,
		HasUserProfile:         true,
		HasFramework:           true,
		HasTeam:                true,
	}
	for step, want := range map[domain.OnboardingStepKey]bool{
		domain.OnboardingStepOrganization: true,
		domain.OnboardingStepProfile:      true,
		domain.OnboardingStepFramework:    true,
		domain.OnboardingStepTeam:         true,
		domain.OnboardingStepGoal:         false,
	} {
		if got := full.SkipsStep(step); got != want {
			t.Errorf("SkipsStep(%q) = %v, want %v", step, got, want)
		}
	}

	empty := domain.OnboardingStepData{}
	for _, step := range domain.OnboardingStepOrder {
		if empty.SkipsStep(step) {
			t.Errorf("an empty tenant must skip nothing, skipped %q", step)
		}
	}
	if len(empty.VisibleSteps()) != len(domain.OnboardingStepOrder) {
		t.Error("an empty tenant must see every step")
	}
}
