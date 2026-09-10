// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only

package activation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"

	"github.com/opendefender/openrisk/internal/domain"
	"github.com/opendefender/openrisk/pkg/monitoring"
)

// ---------------------------------------------------------------------------
// In-memory repository double
// ---------------------------------------------------------------------------

type fakeRepo struct {
	events     []domain.ActivationEvent
	celebrated map[string]map[string]time.Time // userID -> stepKey -> at
	progress   map[string]*domain.OnboardingProgress

	failFirstOccurrences bool
	failRecord           bool
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		celebrated: map[string]map[string]time.Time{},
		progress:   map[string]*domain.OnboardingProgress{},
	}
}

func (f *fakeRepo) RecordEvent(_ context.Context, e *domain.ActivationEvent) error {
	if f.failRecord {
		return errors.New("activation store down")
	}
	f.events = append(f.events, *e)
	return nil
}

func (f *fakeRepo) FirstOccurrences(_ context.Context, tenantID uuid.UUID) (map[domain.ActivationEventKey]time.Time, error) {
	if f.failFirstOccurrences {
		return nil, errors.New("activation store down")
	}
	out := map[domain.ActivationEventKey]time.Time{}
	for _, e := range f.events {
		if e.TenantID != tenantID {
			continue
		}
		if prev, ok := out[e.EventKey]; !ok || e.OccurredAt.Before(prev) {
			out[e.EventKey] = e.OccurredAt
		}
	}
	return out, nil
}

func (f *fakeRepo) HasEvent(_ context.Context, tenantID uuid.UUID, key domain.ActivationEventKey) (bool, error) {
	for _, e := range f.events {
		if e.TenantID == tenantID && e.EventKey == key {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeRepo) CelebratedSteps(_ context.Context, _, userID uuid.UUID) (map[string]time.Time, error) {
	out := map[string]time.Time{}
	for k, v := range f.celebrated[userID.String()] {
		out[k] = v
	}
	return out, nil
}

func (f *fakeRepo) MarkCelebrated(_ context.Context, _, userID uuid.UUID, stepKey string) error {
	if f.celebrated[userID.String()] == nil {
		f.celebrated[userID.String()] = map[string]time.Time{}
	}
	f.celebrated[userID.String()][stepKey] = time.Now()
	return nil
}

func (f *fakeRepo) Get(_ context.Context, _, userID uuid.UUID) (*domain.OnboardingProgress, error) {
	return f.progress[userID.String()], nil
}

func (f *fakeRepo) Save(_ context.Context, p *domain.OnboardingProgress) error {
	clone := *p
	f.progress[p.UserID.String()] = &clone
	return nil
}

// ---------------------------------------------------------------------------
// State
// ---------------------------------------------------------------------------

func TestGetState_DerivedFromServerEvents(t *testing.T) {
	repo := newFakeRepo()
	tenant, user := uuid.New(), uuid.New()
	ctx := context.Background()
	rec := NewRecorder(repo)

	state, err := NewGetStateUseCase(repo).Execute(ctx, tenant, user)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.Percent != 0 {
		t.Errorf("a fresh tenant starts at 0%%, got %d", state.Percent)
	}
	for _, s := range state.Steps {
		if s.Completed || s.Celebrate {
			t.Errorf("step %q must not start completed", s.Key)
		}
		if s.LabelI18n["fr"] == "" || s.DeepLink == "" {
			t.Errorf("step %q must ship copy and a deep link so the panel holds no logic", s.Key)
		}
	}

	rec.Record(ctx, tenant, string(domain.ActivationRiskCreated), nil)
	state, _ = NewGetStateUseCase(repo).Execute(ctx, tenant, user)

	completed := completedKeys(state)
	if len(completed) != 1 || completed[0] != "first_risk" {
		t.Fatalf("one event must tick exactly one step, got %v", completed)
	}
	if state.Percent != 100/len(state.Steps) {
		t.Errorf("percent = %d, want %d", state.Percent, 100/len(state.Steps))
	}
}

// THE regression test for the reported bug: importing a framework strikes through
// ONE row, not two.
func TestGetState_OneEventTicksExactlyOneStep(t *testing.T) {
	repo := newFakeRepo()
	tenant, user := uuid.New(), uuid.New()
	ctx := context.Background()

	NewRecorder(repo).Record(ctx, tenant, string(domain.ActivationFrameworkImported), nil)

	state, _ := NewGetStateUseCase(repo).Execute(ctx, tenant, user)
	completed := completedKeys(state)
	if len(completed) != 1 {
		t.Fatalf("a single import ticked %d steps (%v); it must tick exactly one", len(completed), completed)
	}
	if completed[0] != "framework" {
		t.Errorf("wrong step ticked: %v", completed)
	}
}

// A completed step never un-completes and its timestamp never moves, however many
// further events of the same kind arrive.
func TestGetState_CompletedAtIsStable(t *testing.T) {
	repo := newFakeRepo()
	tenant, user := uuid.New(), uuid.New()
	ctx := context.Background()

	first := time.Date(2026, 8, 10, 8, 0, 0, 0, time.UTC)
	repo.events = append(repo.events,
		domain.ActivationEvent{TenantID: tenant, EventKey: domain.ActivationRiskCreated, OccurredAt: first.Add(3 * time.Hour)},
		domain.ActivationEvent{TenantID: tenant, EventKey: domain.ActivationRiskCreated, OccurredAt: first},
	)

	state, _ := NewGetStateUseCase(repo).Execute(ctx, tenant, user)
	step := stepByKey(t, state, "first_risk")
	if step.CompletedAt == nil || !step.CompletedAt.Equal(first) {
		t.Errorf("completed_at = %v, want the FIRST occurrence %v", step.CompletedAt, first)
	}
}

// Celebration is a server instruction, once per step per user.
func TestGetState_CelebrateOncePerStepPerUser(t *testing.T) {
	repo := newFakeRepo()
	tenant := uuid.New()
	awa, moussa := uuid.New(), uuid.New()
	ctx := context.Background()

	NewRecorder(repo).Record(ctx, tenant, string(domain.ActivationRiskCreated), nil)

	state, _ := NewGetStateUseCase(repo).Execute(ctx, tenant, awa)
	if !stepByKey(t, state, "first_risk").Celebrate {
		t.Fatal("a freshly completed step must ask for a celebration")
	}

	// Awa acknowledges; a reload must not celebrate again.
	if err := NewMarkCelebratedUseCase(repo).Execute(ctx, tenant, awa, "first_risk"); err != nil {
		t.Fatalf("MarkCelebrated: %v", err)
	}
	state, _ = NewGetStateUseCase(repo).Execute(ctx, tenant, awa)
	if stepByKey(t, state, "first_risk").Celebrate {
		t.Error("a celebrated step must never ask again — this is the random-confetti bug")
	}
	if !stepByKey(t, state, "first_risk").Completed {
		t.Error("acknowledging a celebration must not un-complete the step")
	}

	// Moussa, in the same tenant, still gets his own moment.
	state, _ = NewGetStateUseCase(repo).Execute(ctx, tenant, moussa)
	if !stepByKey(t, state, "first_risk").Celebrate {
		t.Error("celebration is per user; a teammate should still see it")
	}
}

func TestMarkCelebrated_RejectsUnknownStep(t *testing.T) {
	repo := newFakeRepo()
	uc := NewMarkCelebratedUseCase(repo)
	ctx := context.Background()

	if err := uc.Execute(ctx, uuid.New(), uuid.New(), "not-a-step"); err == nil {
		t.Error("an unknown step key must be rejected")
	}
	if err := uc.Execute(ctx, uuid.Nil, uuid.New(), "first_risk"); err == nil {
		t.Error("a missing tenant must be rejected")
	}
}

// A broken event store degrades to "nothing done yet" instead of breaking the
// dashboard's first card.
func TestGetState_DegradesOnRepositoryError(t *testing.T) {
	repo := newFakeRepo()
	repo.failFirstOccurrences = true

	state, err := NewGetStateUseCase(repo).Execute(context.Background(), uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("must not fail the request: %v", err)
	}
	if len(state.Steps) == 0 || state.Percent != 0 {
		t.Errorf("want a full, all-incomplete checklist, got %+v", state)
	}
}

// A failing store must not make the business action fail either.
func TestRecorder_IsBestEffort(t *testing.T) {
	repo := newFakeRepo()
	repo.failRecord = true
	rec := NewRecorder(repo)

	rec.Record(context.Background(), uuid.New(), string(domain.ActivationRiskCreated), nil) // must not panic
	var nilRec *Recorder
	nilRec.Record(context.Background(), uuid.New(), string(domain.ActivationRiskCreated), nil)
	NewRecorder(nil).Record(context.Background(), uuid.New(), string(domain.ActivationRiskCreated), nil)
}

func TestRecorder_RecordOnce(t *testing.T) {
	repo := newFakeRepo()
	rec := NewRecorder(repo)
	tenant := uuid.New()
	ctx := context.Background()

	if !rec.RecordOnce(ctx, tenant, string(domain.ActivationSignup), nil) {
		t.Fatal("first RecordOnce should record")
	}
	if rec.RecordOnce(ctx, tenant, string(domain.ActivationSignup), nil) {
		t.Error("second RecordOnce must be a no-op")
	}
	if len(repo.events) != 1 {
		t.Errorf("want 1 event, got %d", len(repo.events))
	}
}

// ---------------------------------------------------------------------------
// Aha
// ---------------------------------------------------------------------------

func TestAhaSignal_Definition(t *testing.T) {
	cases := []struct {
		name   string
		signal AhaSignal
		want   bool
	}{
		{"score on own data with a gap", AhaSignal{true, 4, 12}, true},
		{"no score computed", AhaSignal{false, 4, 12}, false},
		{"score on an empty workspace", AhaSignal{true, 0, 12}, false},
		{"score with no gap identified", AhaSignal{true, 4, 0}, false},
		{"nothing at all", AhaSignal{}, false},
	}
	for _, tc := range cases {
		if got := tc.signal.IsAha(); got != tc.want {
			t.Errorf("%s: IsAha() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestMaybeRecordAha_OncePerTenant(t *testing.T) {
	repo := newFakeRepo()
	tenant := uuid.New()
	ctx := context.Background()
	rec := NewAhaRecorder(repo)

	// Not yet: a score, but nothing behind it.
	rec.MaybeRecordAha(ctx, tenant, true, 0, 5)
	if has, _ := repo.HasEvent(ctx, tenant, domain.ActivationAhaReached); has {
		t.Fatal("an empty workspace is not an Aha")
	}

	rec.MaybeRecordAha(ctx, tenant, true, 3, 7)
	rec.MaybeRecordAha(ctx, tenant, true, 9, 20)

	count := 0
	for _, e := range repo.events {
		if e.EventKey == domain.ActivationAhaReached {
			count++
		}
	}
	if count != 1 {
		t.Errorf("aha recorded %d times, want exactly 1", count)
	}
}

// D-010's carried-forward defect, proved at the call site: the tenant is counted
// when the Aha is recorded, even though no signup row exists to measure a
// duration against. Before this change the increment lived inside
// ObserveTimeToAha and this tenant vanished from the activation rate.
func TestMaybeRecordAha_CountsATenantWithNoSignupAnchor(t *testing.T) {
	repo := newFakeRepo()
	tenant := uuid.New()
	ctx := context.Background()

	countBefore := testutil.ToFloat64(monitoring.AhaReachedTotal)
	v1Before := ahaObservations(t, monitoring.AhaDefinitionV1)

	NewAhaRecorder(repo).MaybeRecordAha(ctx, tenant, true, 3, 7)

	if has, _ := repo.HasEvent(ctx, tenant, domain.ActivationAhaReached); !has {
		t.Fatal("the aha event must be recorded with or without a signup anchor")
	}
	if got := testutil.ToFloat64(monitoring.AhaReachedTotal) - countBefore; got != 1 {
		t.Errorf("aha_reached_total moved by %v, want 1", got)
	}
	if got := ahaObservations(t, monitoring.AhaDefinitionV1) - v1Before; got != 0 {
		t.Errorf("a duration was observed with no signup anchor: +%d", got)
	}
}

// The executive-dashboard Aha is the v1 definition and stays there: the Posture
// Reveal will observe v2 from its own call site (D-010).
func TestMaybeRecordAha_ObservesTheV1Definition(t *testing.T) {
	repo := newFakeRepo()
	tenant := uuid.New()
	ctx := context.Background()
	repo.events = append(repo.events, domain.ActivationEvent{
		TenantID:   tenant,
		EventKey:   domain.ActivationSignup,
		OccurredAt: time.Now().UTC().Add(-6 * time.Minute),
	})

	v1Before := ahaObservations(t, monitoring.AhaDefinitionV1)
	v2Before := ahaObservations(t, monitoring.AhaDefinitionV2)

	NewAhaRecorder(repo).MaybeRecordAha(ctx, tenant, true, 3, 7)

	if got := ahaObservations(t, monitoring.AhaDefinitionV1) - v1Before; got != 1 {
		t.Errorf("v1 gained %d observations, want 1", got)
	}
	if got := ahaObservations(t, monitoring.AhaDefinitionV2) - v2Before; got != 0 {
		t.Errorf("the dashboard Aha leaked %d observations into v2, want 0", got)
	}
}

// ahaObservations reads one aha_definition series of the time_to_aha histogram.
func ahaObservations(t *testing.T, definition monitoring.AhaDefinition) uint64 {
	t.Helper()
	obs, err := monitoring.TimeToAha.GetMetricWithLabelValues(definition)
	if err != nil {
		t.Fatalf("GetMetricWithLabelValues(%q): %v", definition, err)
	}
	metric, ok := obs.(prometheus.Metric)
	if !ok {
		t.Fatalf("observer for %q is not a prometheus.Metric", definition)
	}
	var out dto.Metric
	if err := metric.Write(&out); err != nil {
		t.Fatalf("writing metric for %q: %v", definition, err)
	}
	return out.GetHistogram().GetSampleCount()
}

func TestGetState_ReportsTimeToAha(t *testing.T) {
	repo := newFakeRepo()
	tenant, user := uuid.New(), uuid.New()
	signup := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	repo.events = append(repo.events,
		domain.ActivationEvent{TenantID: tenant, EventKey: domain.ActivationSignup, OccurredAt: signup},
		domain.ActivationEvent{TenantID: tenant, EventKey: domain.ActivationAhaReached, OccurredAt: signup.Add(7 * time.Minute)},
	)

	state, _ := NewGetStateUseCase(repo).Execute(context.Background(), tenant, user)
	if state.AhaReachedAt == nil {
		t.Fatal("aha_reached_at must be reported")
	}
	if state.TimeToAhaSeconds == nil || *state.TimeToAhaSeconds != 420 {
		t.Errorf("time_to_aha = %v, want 420s", state.TimeToAhaSeconds)
	}
}

// ---------------------------------------------------------------------------
// Wizard
// ---------------------------------------------------------------------------

func newWizard(repo *fakeRepo) *OnboardingUseCase {
	return NewOnboardingUseCase(repo, NewRecorder(repo))
}

func TestWizard_SaveIsResumableAndReversible(t *testing.T) {
	repo := newFakeRepo()
	uc := newWizard(repo)
	tenant, user := uuid.New(), uuid.New()
	ctx := context.Background()

	state, err := uc.GetState(ctx, tenant, user)
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if state.CurrentStep != string(domain.OnboardingStepOrganization) || state.Completed {
		t.Fatalf("a newcomer starts at organization, incomplete: %+v", state)
	}

	state, err = uc.SaveStep(ctx, tenant, user, SaveStepInput{
		Step:    domain.OnboardingStepOrganization,
		Answers: domain.JSONMap{"name": "Banque Atlantique", "industry": "banking", "country": "CM", "size": "201-1000"},
	})
	if err != nil {
		t.Fatalf("SaveStep: %v", err)
	}
	if state.CurrentStep != string(domain.OnboardingStepGoal) {
		t.Errorf("saving a step advances the cursor, got %q", state.CurrentStep)
	}
	if state.Industry != "banking" || state.Country != "CM" {
		t.Errorf("template drivers must be promoted: %+v", state)
	}

	// Resume: a reload shows the same answers.
	resumed, _ := uc.GetState(ctx, tenant, user)
	org, _ := resumed.Answers[string(domain.OnboardingStepOrganization)].(map[string]interface{})
	if org["name"] != "Banque Atlantique" {
		t.Errorf("answers must survive a reload: %v", resumed.Answers)
	}

	// Back-navigation is allowed: fixing a typo is not an error.
	back, err := uc.SaveStep(ctx, tenant, user, SaveStepInput{
		Step:    domain.OnboardingStepGoal,
		Answers: domain.JSONMap{"goal": "pass_audit"},
		Next:    string(domain.OnboardingStepOrganization),
	})
	if err != nil {
		t.Fatalf("back-navigation must be allowed: %v", err)
	}
	if back.CurrentStep != string(domain.OnboardingStepOrganization) {
		t.Errorf("cursor should have moved back, got %q", back.CurrentStep)
	}
}

// A member invited into an already-configured tenant walks their own wizard.
// Their organization answers must be STORED (so their sector still drives their
// suggestions) but must NOT rewrite the company's Organization row — otherwise
// the onboarding form is a privilege escalation.
func TestWizard_OrganizationWriteRequiresPermission(t *testing.T) {
	repo := newFakeRepo()
	orgs := &fakeOrgUpdater{}
	uc := newWizard(repo).WithOrgUpdater(orgs)
	tenant, user := uuid.New(), uuid.New()
	ctx := context.Background()

	// A plain member: no organization write.
	_, err := uc.SaveStep(ctx, tenant, user, SaveStepInput{
		Step:    domain.OnboardingStepOrganization,
		Answers: domain.JSONMap{"name": "Renamed By A Member", "industry": "tech"},
	})
	if err != nil {
		t.Fatalf("SaveStep: %v", err)
	}
	if orgs.calls != 0 {
		t.Errorf("a member must not be able to rename the organization (got %d writes)", orgs.calls)
	}

	state, _ := uc.GetState(ctx, tenant, user)
	if state.Industry != "tech" {
		t.Error("their answers must still be stored and still drive suggestions")
	}

	// An admin: the write goes through.
	_, err = uc.SaveStep(ctx, tenant, uuid.New(), SaveStepInput{
		Step:                domain.OnboardingStepOrganization,
		Answers:             domain.JSONMap{"name": "Banque Atlantique", "industry": "banking"},
		CanEditOrganization: true,
	})
	if err != nil {
		t.Fatalf("SaveStep: %v", err)
	}
	if orgs.calls != 1 || orgs.lastName != "Banque Atlantique" {
		t.Errorf("an admin should rename the organization, got %d writes / %q", orgs.calls, orgs.lastName)
	}
}

// `profile` is still a CHECKLIST step with its own event key — #438 retired its
// WIZARD ROUTE, not the milestone. The event is now recorded from the
// organization step, which absorbed the question, and it must still tick exactly
// one row.
func TestWizard_OrganizationStepRecordsTheProfileMilestone(t *testing.T) {
	repo := newFakeRepo()
	uc := newWizard(repo)
	tenant, user := uuid.New(), uuid.New()

	_, err := uc.SaveStep(context.Background(), tenant, user, SaveStepInput{
		Step: domain.OnboardingStepOrganization,
		Answers: domain.JSONMap{
			"name": "Banque Atlantique", "industry": "banking", "country": "CM",
			"full_name": "Awa", "job_title": "RSSI",
		},
	})
	if err != nil {
		t.Fatalf("SaveStep: %v", err)
	}

	state, _ := NewGetStateUseCase(repo).Execute(context.Background(), tenant, user)
	if !stepByKey(t, state, "profile").Completed {
		t.Error("giving a name in the organization step must tick the profile checklist row")
	}
	if len(completedKeys(state)) != 1 {
		t.Errorf("it must tick that row only, got %v", completedKeys(state))
	}
}

// And the milestone is NOT recorded for a company-only answer: a step that ticks
// "you completed your profile" when no person was named is the class of untruth
// the whole activation rewrite exists to remove.
func TestWizard_OrganizationWithoutAPersonDoesNotTickProfile(t *testing.T) {
	repo := newFakeRepo()
	uc := newWizard(repo)
	tenant, user := uuid.New(), uuid.New()

	if _, err := uc.SaveStep(context.Background(), tenant, user, SaveStepInput{
		Step:    domain.OnboardingStepOrganization,
		Answers: domain.JSONMap{"name": "Banque Atlantique", "industry": "banking"},
	}); err != nil {
		t.Fatalf("SaveStep: %v", err)
	}

	state, _ := NewGetStateUseCase(repo).Execute(context.Background(), tenant, user)
	if stepByKey(t, state, "profile").Completed {
		t.Error("no name was given; the profile row must not tick")
	}
}

func TestWizard_CompleteIsIdempotent(t *testing.T) {
	repo := newFakeRepo()
	uc := newWizard(repo)
	tenant, user := uuid.New(), uuid.New()
	ctx := context.Background()

	first, err := uc.Complete(ctx, tenant, user)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if !first.Completed || first.CompletedAt == nil || first.Percent != 100 {
		t.Fatalf("completion must be recorded: %+v", first)
	}

	second, err := uc.Complete(ctx, tenant, user)
	if err != nil {
		t.Fatalf("re-completing must not error: %v", err)
	}
	if !second.CompletedAt.Equal(*first.CompletedAt) {
		t.Error("completed_at must not move on a repeat submit")
	}
}

func TestWizard_RejectsUnknownStepAndMissingIdentity(t *testing.T) {
	uc := newWizard(newFakeRepo())
	ctx := context.Background()

	if _, err := uc.SaveStep(ctx, uuid.New(), uuid.New(), SaveStepInput{Step: "billing"}); err == nil {
		t.Error("an unknown step must be rejected")
	}
	if _, err := uc.GetState(ctx, uuid.Nil, uuid.New()); err == nil {
		t.Error("a missing tenant must be rejected")
	}
}

func TestWizard_SuggestionsFollowStoredAnswers(t *testing.T) {
	repo := newFakeRepo()
	uc := newWizard(repo)
	tenant, user := uuid.New(), uuid.New()
	ctx := context.Background()

	_, _ = uc.SaveStep(ctx, tenant, user, SaveStepInput{
		Step:    domain.OnboardingStepOrganization,
		Answers: domain.JSONMap{"industry": "banking", "country": "CM"},
	})
	_, _ = uc.SaveStep(ctx, tenant, user, SaveStepInput{
		Step:    domain.OnboardingStepGoal,
		Answers: domain.JSONMap{"goal": "cobac_compliance"},
	})

	s := uc.GetSuggestions(ctx, tenant, user, "", "", "")
	if len(s.Risks) != 3 {
		t.Fatalf("want 3 first-risk drafts, got %d", len(s.Risks))
	}
	if s.Frameworks[0] != "cobac" {
		t.Errorf("the chosen goal should lead the framework list, got %v", s.Frameworks)
	}

	// An explicit override wins, so the wizard can preview a sector before saving.
	preview := uc.GetSuggestions(ctx, tenant, user, "health", "FR", "pass_audit")
	if preview.Frameworks[0] != "iso27001-2022" {
		t.Errorf("override should re-resolve suggestions, got %v", preview.Frameworks)
	}
	if preview.Risks[0].Title == s.Risks[0].Title {
		t.Error("a different sector should propose different first risks")
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

type fakeOrgUpdater struct {
	calls    int
	lastName string
}

func (f *fakeOrgUpdater) UpdateOrganizationProfile(_ context.Context, _ uuid.UUID, name, _, _ string) error {
	f.calls++
	f.lastName = name
	return nil
}

func completedKeys(s *State) []string {
	var out []string
	for _, step := range s.Steps {
		if step.Completed {
			out = append(out, step.Key)
		}
	}
	return out
}

func stepByKey(t *testing.T, s *State, key string) Step {
	t.Helper()
	for _, step := range s.Steps {
		if step.Key == key {
			return step
		}
	}
	t.Fatalf("step %q not found", key)
	return Step{}
}
