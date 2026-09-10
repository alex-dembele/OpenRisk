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
		// Both answers, because the organization step absorbed the profile one.
		HasOrganizationProfile: true,
		HasUserProfile:         true,
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

// The three steps that can never be skipped, each for its own reason: `goal` is
// a preference no stored row can prove, and `score`/`cover` are the two screens
// that RETURN something computed — skipping them hands back exactly the
// activation cliff #438 exists to remove.
func TestWizard_GoalScoreAndCoverAreNeverSkippable(t *testing.T) {
	everything := domain.OnboardingStepData{
		HasOrganizationProfile: true,
		HasUserProfile:         true,
		HasFramework:           true,
		HasTeam:                true,
	}

	visible := everything.VisibleSteps()
	want := []domain.OnboardingStepKey{
		domain.OnboardingStepGoal,
		domain.OnboardingStepScore,
		domain.OnboardingStepCover,
	}
	if len(visible) != len(want) {
		t.Fatalf("visible steps = %v, want %v", visible, want)
	}
	for i, step := range want {
		if visible[i] != step {
			t.Errorf("visible[%d] = %q, want %q", i, visible[i], step)
		}
		if everything.SkipsStep(step) {
			t.Errorf("%q must never be skipped", step)
		}
	}

	repo := newFakeRepo()
	uc := newWizard(repo).WithStepProbe(&fakeProbe{data: everything})
	state, err := uc.GetState(context.Background(), uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if len(state.Steps) != len(want) {
		t.Errorf("a fully configured user must still walk %d steps, got %v", len(want), state.Steps)
	}
}

// The organization step absorbed the retired profile step, so ONE of the two
// answers is not enough to skip it — the checklist's `profile` row is ticked
// from there, and skipping on the company alone would leave it unticked forever.
func TestWizard_OrganizationNeedsBothAnswersToBeSkipped(t *testing.T) {
	orgOnly := domain.OnboardingStepData{HasOrganizationProfile: true}
	if orgOnly.SkipsStep(domain.OnboardingStepOrganization) {
		t.Error("the company alone must not skip the step that also asks for the person")
	}
	userOnly := domain.OnboardingStepData{HasUserProfile: true}
	if userOnly.SkipsStep(domain.OnboardingStepOrganization) {
		t.Error("the person alone must not skip the step that also asks for the company")
	}
	both := domain.OnboardingStepData{HasOrganizationProfile: true, HasUserProfile: true}
	if !both.SkipsStep(domain.OnboardingStepOrganization) {
		t.Error("with both answers stored the step has nothing left to ask")
	}
}

// The cursor steps OVER hidden steps in both directions. Landing on one would
// strand the user on a screen the client must not draw.
func TestWizard_CursorStepsOverHiddenSteps(t *testing.T) {
	repo := newFakeRepo()
	// framework hidden ⇒ visible: organization, goal, score, cover
	probe := &fakeProbe{data: domain.OnboardingStepData{HasFramework: true}}
	uc := newWizard(repo).WithStepProbe(probe)
	tenant, user := uuid.New(), uuid.New()

	// Forward from goal: framework is hidden, so score is next.
	state, err := uc.SaveStep(context.Background(), tenant, user, SaveStepInput{
		Step:    domain.OnboardingStepGoal,
		Answers: domain.JSONMap{"goal": "compliance"},
	})
	if err != nil {
		t.Fatalf("SaveStep: %v", err)
	}
	if state.CurrentStep != string(domain.OnboardingStepScore) {
		t.Errorf("forward cursor = %q, want score (framework is hidden)", state.CurrentStep)
	}

	// An explicit Next naming a HIDDEN step is honoured as a DIRECTION, not a
	// destination: the cursor lands on the nearest visible step that way, never
	// on the hidden one and never staying put.
	state, err = uc.SaveStep(context.Background(), tenant, user, SaveStepInput{
		Step:    domain.OnboardingStepScore,
		Answers: domain.JSONMap{},
		Next:    string(domain.OnboardingStepFramework),
	})
	if err != nil {
		t.Fatalf("SaveStep: %v", err)
	}
	if state.CurrentStep != string(domain.OnboardingStepGoal) {
		t.Errorf("backwards cursor = %q, want goal (framework is hidden)", state.CurrentStep)
	}

	// A RETIRED route is rejected outright — no client may resurrect one.
	if _, err := uc.SaveStep(context.Background(), tenant, user, SaveStepInput{
		Step:    domain.OnboardingStepTeam,
		Answers: domain.JSONMap{},
	}); err == nil {
		t.Error("saving a retired step must be rejected")
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
		CurrentStep: domain.OnboardingStepCover,
	}
	// organization hidden ⇒ visible: goal, framework, score, cover
	uc := newWizard(repo).WithStepProbe(&fakeProbe{data: domain.OnboardingStepData{
		HasOrganizationProfile: true,
		HasUserProfile:         true,
	}})

	state, err := uc.GetState(context.Background(), tenant, user)
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if state.StepIndex != 3 || len(state.Steps) != 4 {
		t.Errorf("step %d of %d, want 3 of 4", state.StepIndex, len(state.Steps))
	}

	// A stored cursor parked on a NOW-hidden step must resolve inside the
	// visible range, and must not be REPORTED as the hidden step either.
	repo.progress[user.String()].CurrentStep = domain.OnboardingStepOrganization
	state, err = uc.GetState(context.Background(), tenant, user)
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if state.StepIndex < 0 || state.StepIndex >= len(state.Steps) {
		t.Errorf("a cursor on a hidden step resolved to index %d of %d", state.StepIndex, len(state.Steps))
	}
	if state.CurrentStep == string(domain.OnboardingStepOrganization) {
		t.Error("the reported cursor must be snapped onto the visible sequence")
	}

	// A cursor on a RETIRED route is the same problem with a different cause:
	// stored rows written before #438 point at `profile`.
	repo.progress[user.String()].CurrentStep = domain.OnboardingStepProfile
	state, err = uc.GetState(context.Background(), tenant, user)
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if state.StepIndex < 0 || state.StepIndex >= len(state.Steps) {
		t.Errorf("a retired cursor resolved to index %d of %d", state.StepIndex, len(state.Steps))
	}
	if state.CurrentStep == string(domain.OnboardingStepProfile) {
		t.Error("a retired route must never be reported as the current step")
	}
}

// THE BUG THIS FREEZE EXISTS TO FIX, pinned.
//
// The probe reads LIVE data. Before the decision was frozen, the instant a user
// saved the organization step that step's data existed, so it vanished from
// their tunnel — and they could never go back to fix a typo in the answer they
// had just given, on a wizard whose stated contract is that you can.
//
// Auto-skip is about data that existed BEFORE the tunnel started.
func TestWizard_AnsweringAStepDoesNotMakeItUnreachable(t *testing.T) {
	repo := newFakeRepo()
	tenant, user := uuid.New(), uuid.New()

	// A probe that answers from the tenant's live state, exactly as the real one
	// does: once the organization answers exist, it reports them.
	live := &liveProbe{repo: repo, user: user}
	uc := newWizard(repo).WithStepProbe(live)

	state, err := uc.SaveStep(context.Background(), tenant, user, SaveStepInput{
		Step: domain.OnboardingStepOrganization,
		Answers: domain.JSONMap{
			"name": "Banque Atlantique CM", "industry": "banking", "size": "201-1000",
			"full_name": "Awa Newcomer", "job_title": "RSSI",
		},
	})
	if err != nil {
		t.Fatalf("SaveStep: %v", err)
	}
	if state.CurrentStep != string(domain.OnboardingStepGoal) {
		t.Fatalf("cursor = %q, want goal", state.CurrentStep)
	}

	// The step the user JUST answered must still be reachable.
	for _, s := range state.Steps {
		if s == string(domain.OnboardingStepOrganization) {
			goto reachable
		}
	}
	t.Fatalf("the just-answered step vanished from the stepper: %v", state.Steps)

reachable:
	back, err := uc.SaveStep(context.Background(), tenant, user, SaveStepInput{
		Step:    domain.OnboardingStepGoal,
		Answers: domain.JSONMap{"goal": "pass_audit"},
		Next:    string(domain.OnboardingStepOrganization),
	})
	if err != nil {
		t.Fatalf("back-navigation must be permitted: %v", err)
	}
	if back.CurrentStep != string(domain.OnboardingStepOrganization) {
		t.Errorf("going back to fix a typo landed on %q, want organization", back.CurrentStep)
	}
}

// A tenant that ALREADY held the data before entering still skips the step —
// the freeze must not disable auto-skip, only pin when it is decided.
func TestWizard_PreExistingDataStillSkips(t *testing.T) {
	repo := newFakeRepo()
	tenant, user := uuid.New(), uuid.New()
	live := &liveProbe{repo: repo, user: user, preExisting: true}
	uc := newWizard(repo).WithStepProbe(live)

	state, err := uc.GetState(context.Background(), tenant, user)
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	for _, s := range state.Steps {
		if s == string(domain.OnboardingStepOrganization) {
			t.Error("a tenant that already had the data must not be asked again")
		}
	}
}

// liveProbe answers from the stored progress, the way the real repository probe
// answers from the tenant's live rows.
type liveProbe struct {
	repo        *fakeRepo
	user        uuid.UUID
	preExisting bool
}

func (p *liveProbe) OnboardingStepData(_ context.Context, _, _ uuid.UUID) (domain.OnboardingStepData, error) {
	if p.preExisting {
		return domain.OnboardingStepData{HasOrganizationProfile: true, HasUserProfile: true}, nil
	}
	progress := p.repo.progress[p.user.String()]
	if progress == nil {
		return domain.OnboardingStepData{}, nil
	}
	answers := progress.StepAnswers(domain.OnboardingStepOrganization)
	has := stringAnswer(answers, "industry") != "" && stringAnswer(answers, "size") != ""
	name := stringAnswer(answers, "full_name") != ""
	return domain.OnboardingStepData{HasOrganizationProfile: has, HasUserProfile: name}, nil
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
	// framework hidden ⇒ visible: organization, goal, score, cover
	uc := newWizard(repo).WithStepProbe(&fakeProbe{data: domain.OnboardingStepData{HasFramework: true}})
	tenant, user := uuid.New(), uuid.New()

	state, err := uc.Complete(context.Background(), tenant, user)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if state.CurrentStep != string(domain.OnboardingStepCover) {
		t.Errorf("cursor = %q, want the last visible step (cover)", state.CurrentStep)
	}
	if !state.Completed {
		t.Error("Complete must complete")
	}

	// With `cover` itself hidden the cursor would park on whatever is last —
	// but `cover` is unskippable, so this asserts that invariant from the other
	// side: it is ALWAYS the terminal step.
	hidden := domain.OnboardingStepData{
		HasOrganizationProfile: true,
		HasUserProfile:         true,
		HasFramework:           true,
		HasTeam:                true,
	}
	visible := hidden.VisibleSteps()
	if visible[len(visible)-1] != domain.OnboardingStepCover {
		t.Errorf("the tunnel must always end on cover, got %q", visible[len(visible)-1])
	}
}

func TestOnboardingStepData_SkipsStep(t *testing.T) {
	full := domain.OnboardingStepData{
		HasOrganizationProfile: true,
		HasUserProfile:         true,
		HasFramework:           true,
		HasTeam:                true,
	}
	// #438's sequence: organization (company + person), goal, framework, score,
	// cover. `profile` and `team` are retired routes and are not in it.
	for step, want := range map[domain.OnboardingStepKey]bool{
		domain.OnboardingStepOrganization: true,
		domain.OnboardingStepFramework:    true,
		domain.OnboardingStepGoal:         false,
		domain.OnboardingStepScore:        false,
		domain.OnboardingStepCover:        false,
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

	// A retired route is not "skippable", it is absent. VisibleSteps walks the
	// order, so it can never surface one.
	for _, retired := range []domain.OnboardingStepKey{
		domain.OnboardingStepProfile,
		domain.OnboardingStepTeam,
	} {
		for _, visible := range full.VisibleSteps() {
			if visible == retired {
				t.Errorf("the retired route %q surfaced in the visible steps", retired)
			}
		}
	}
}
