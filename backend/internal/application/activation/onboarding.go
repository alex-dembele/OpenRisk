// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package activation

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/opendefender/openrisk/internal/domain"
	"github.com/opendefender/openrisk/pkg/onboarding"
)

// ---------------------------------------------------------------------------
// The five-step signup wizard (spec §4).
//
// Every step is savable, resumable and reversible: the client PUTs one step at a
// time, the server stores the raw answers per step, and back-navigation is a
// plain sequence move — there is no forbidden transition, because a wizard that
// will not let you go back to fix a typo is a wizard people abandon.
// ---------------------------------------------------------------------------

// OrgUpdater applies the organization step to the real Organization row. Optional
// (nil-safe): if it is absent the answers are still stored, so the wizard works
// even where the caller has no organization write path.
type OrgUpdater interface {
	UpdateOrganizationProfile(ctx context.Context, orgID uuid.UUID, name, industry, size string) error
}

// OrgCurrencyUpdater persists the tenant's display currency (onboarding step, and
// later the settings screen). Optional (nil-safe), kept off OrgUpdater so callers
// without a currency path are unaffected.
type OrgCurrencyUpdater interface {
	SetOrganizationCurrency(ctx context.Context, orgID uuid.UUID, currency string) error
}

// ProfileUpdater applies the profile step to the User row. Optional.
type ProfileUpdater interface {
	UpdateUserProfile(ctx context.Context, userID uuid.UUID, fullName, jobTitle, avatarURL string) error
}

// WizardState is the payload of GET /onboarding/state.
type WizardState struct {
	CurrentStep string `json:"current_step"`
	// Steps is what the ProgressStepper renders: the steps THIS USER will
	// actually see, with auto-skipped ones removed (#438 criterion 3). It is not
	// the canonical catalogue — a user who skips two steps must read
	// "step 2 of 3", not "step 2 of 5" with two rows that never arrive.
	Steps []string `json:"steps"`
	// SkippedSteps is what was removed and is returned for transparency, not for
	// rendering. A client that drew these would flash a step criterion 3 forbids.
	SkippedSteps []string `json:"skipped_steps"`
	// StepIndex is the cursor's position among the VISIBLE steps.
	StepIndex   int            `json:"step_index"`
	Completed   bool           `json:"completed"`
	CompletedAt *time.Time     `json:"completed_at,omitempty"`
	Percent     int            `json:"percent"`
	Industry    string         `json:"industry,omitempty"`
	Country     string         `json:"country,omitempty"`
	Goal        string         `json:"goal,omitempty"`
	Answers     domain.JSONMap `json:"answers"`
	// Landing is where the wizard drops the user on completion (derived from the
	// chosen goal).
	Landing string `json:"landing"`
}

// OnboardingUseCase serves the wizard: read state, save one step, complete.
type OnboardingUseCase struct {
	repo     domain.OnboardingRepository
	recorder *Recorder
	orgs     OrgUpdater
	orgCur   OrgCurrencyUpdater
	profiles ProfileUpdater
	// probe decides which steps are auto-skipped. Optional and nil-safe: with no
	// probe every step is shown, which is the old behaviour and the safe default
	// — showing a step the user did not need costs them a click; hiding one they
	// did need loses their answer.
	probe domain.OnboardingStepProbe
	now   func() time.Time
}

// WithStepProbe attaches the auto-skip reader (#438 criteria 2 and 3).
func (uc *OnboardingUseCase) WithStepProbe(p domain.OnboardingStepProbe) *OnboardingUseCase {
	uc.probe = p
	return uc
}

// stepData resolves the auto-skip decisions once. A probe failure degrades to
// "skip nothing": a wizard that shows a redundant step is a minor annoyance, and
// one that hides a step whose data does NOT exist strands the user on a tunnel
// they cannot complete.
func (uc *OnboardingUseCase) stepData(ctx context.Context, tenantID, userID uuid.UUID) domain.OnboardingStepData {
	if uc.probe == nil {
		return domain.OnboardingStepData{}
	}
	data, err := uc.probe.OnboardingStepData(ctx, tenantID, userID)
	if err != nil {
		return domain.OnboardingStepData{}
	}
	return data
}

// WithOrgCurrencyUpdater attaches the optional tenant-currency writer.
func (uc *OnboardingUseCase) WithOrgCurrencyUpdater(u OrgCurrencyUpdater) *OnboardingUseCase {
	uc.orgCur = u
	return uc
}

// NewOnboardingUseCase builds the use case with the required repository.
func NewOnboardingUseCase(repo domain.OnboardingRepository, recorder *Recorder) *OnboardingUseCase {
	return &OnboardingUseCase{
		repo:     repo,
		recorder: recorder,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

// WithOrgUpdater attaches the optional organization writer.
func (uc *OnboardingUseCase) WithOrgUpdater(o OrgUpdater) *OnboardingUseCase { uc.orgs = o; return uc }

// WithProfileUpdater attaches the optional user-profile writer.
func (uc *OnboardingUseCase) WithProfileUpdater(p ProfileUpdater) *OnboardingUseCase {
	uc.profiles = p
	return uc
}

// GetState returns the user's wizard state, creating an implicit empty one the
// first time (never an error, never a 404 — a newcomer's very first request must
// not be a failure).
func (uc *OnboardingUseCase) GetState(ctx context.Context, tenantID, userID uuid.UUID) (*WizardState, error) {
	if tenantID == uuid.Nil || userID == uuid.Nil {
		return nil, domain.NewValidationError("tenant and user are required")
	}

	progress, err := uc.repo.Get(ctx, tenantID, userID)
	if err != nil {
		return nil, err
	}
	if progress == nil {
		progress = &domain.OnboardingProgress{
			TenantID:    tenantID,
			UserID:      userID,
			CurrentStep: domain.OnboardingStepOrganization,
		}
	}

	// Criterion 2: ONE call resolves the whole tunnel, auto-skip decisions
	// included. No step may issue its own status call on mount.
	return toWizardState(progress, uc.stepData(ctx, tenantID, userID)), nil
}

// SaveStepInput is one step's submission.
type SaveStepInput struct {
	Step domain.OnboardingStepKey
	// Answers is the raw step payload, stored verbatim so the form can be
	// repopulated exactly as the user left it.
	Answers domain.JSONMap
	// Next is the step to move to. Empty means "the following step"; it may point
	// BACKWARDS — going back to fix an answer is a supported move, not an error.
	Next string
	// CanEditOrganization gates the ONE answer in this wizard that is not personal.
	//
	// Everyone who joins a tenant walks their own wizard, including a member
	// invited into an already-configured organization. Without this gate, that
	// member's organization step would rename the whole company — a privilege
	// escalation smuggled in through an onboarding form. The handler sets it from
	// the caller's claims; when it is false the answers are still stored (so the
	// sector still drives their suggestions) but the Organization row is untouched.
	CanEditOrganization bool
}

// SaveStep persists one step and advances (or rewinds) the cursor.
func (uc *OnboardingUseCase) SaveStep(ctx context.Context, tenantID, userID uuid.UUID, input SaveStepInput) (*WizardState, error) {
	if tenantID == uuid.Nil || userID == uuid.Nil {
		return nil, domain.NewValidationError("tenant and user are required")
	}
	if _, err := domain.ParseOnboardingStep(string(input.Step)); err != nil {
		return nil, err
	}

	progress, err := uc.repo.Get(ctx, tenantID, userID)
	if err != nil {
		return nil, err
	}
	if progress == nil {
		progress = &domain.OnboardingProgress{TenantID: tenantID, UserID: userID}
	}
	progress.TenantID = tenantID
	progress.UserID = userID
	progress.SetStepAnswers(input.Step, input.Answers)

	// Promote the answers that drive the templates.
	switch input.Step {
	case domain.OnboardingStepOrganization:
		progress.Industry = stringAnswer(input.Answers, "industry")
		progress.Country = stringAnswer(input.Answers, "country")
		if uc.orgs != nil && input.CanEditOrganization {
			// Best-effort: a failure to rename the organization must not lose the
			// user's answers or block the wizard.
			_ = uc.orgs.UpdateOrganizationProfile(ctx, tenantID,
				stringAnswer(input.Answers, "name"),
				progress.Industry,
				stringAnswer(input.Answers, "size"),
			)
		}
		if uc.orgCur != nil && input.CanEditOrganization {
			// Persist the chosen display currency so the financial engine converts
			// every figure into it. Best-effort — never blocks the wizard.
			_ = uc.orgCur.SetOrganizationCurrency(ctx, tenantID, stringAnswer(input.Answers, "currency"))
		}
	case domain.OnboardingStepProfile:
		if uc.profiles != nil {
			_ = uc.profiles.UpdateUserProfile(ctx, userID,
				stringAnswer(input.Answers, "full_name"),
				stringAnswer(input.Answers, "job_title"),
				stringAnswer(input.Answers, "avatar_url"),
			)
		}
		// The profile step is itself a checklist step — one event key, recorded
		// here, so the panel ticks from a server fact like every other row.
		uc.recorder.RecordFor(ctx, tenantID, userID, string(domain.ActivationProfileCompleted), map[string]interface{}{
			"source": "onboarding_wizard",
		})
	case domain.OnboardingStepGoal:
		progress.Goal = stringAnswer(input.Answers, "goal")
	}

	// Move the cursor. An explicit Next wins (including backwards); otherwise
	// advance one step, clamped at the last. Auto-skipped steps are stepped over
	// in both directions — landing on a hidden step would strand the user on a
	// screen the client is forbidden to render.
	data := uc.stepData(ctx, tenantID, userID)
	progress.CurrentStep = uc.nextStep(input.Step, input.Next, data)

	if err := uc.repo.Save(ctx, progress); err != nil {
		return nil, err
	}
	return toWizardState(progress, data), nil
}

// nextStep resolves the cursor move over the VISIBLE steps only.
//
// An explicit Next that names a skipped step is honoured as a direction, not as
// a destination: the cursor lands on the nearest visible step in that direction.
// Silently ignoring it would leave the user on the step they just submitted.
func (uc *OnboardingUseCase) nextStep(current domain.OnboardingStepKey, next string, data domain.OnboardingStepData) domain.OnboardingStepKey {
	visible := data.VisibleSteps()
	if len(visible) == 0 {
		// Unreachable while `goal` is unskippable, but a cursor with nowhere to
		// go must not be a panic.
		return current
	}

	if next != "" {
		if parsed, err := domain.ParseOnboardingStep(next); err == nil {
			backwards := parsed.Index() < current.Index()
			return nearestVisible(parsed, visible, backwards)
		}
	}

	idx := current.Index() + 1
	if idx >= len(domain.OnboardingStepOrder) {
		idx = len(domain.OnboardingStepOrder) - 1
	}
	return nearestVisible(domain.OnboardingStepOrder[idx], visible, false)
}

// nearestVisible snaps a target onto the visible sequence, searching forward
// (or backward) and falling back to the other direction at the ends.
func nearestVisible(target domain.OnboardingStepKey, visible []domain.OnboardingStepKey, backwards bool) domain.OnboardingStepKey {
	for _, s := range visible {
		if s == target {
			return target
		}
	}

	if backwards {
		for i := len(visible) - 1; i >= 0; i-- {
			if visible[i].Index() < target.Index() {
				return visible[i]
			}
		}
		return visible[0]
	}
	for _, s := range visible {
		if s.Index() > target.Index() {
			return s
		}
	}
	return visible[len(visible)-1]
}

// Complete closes the wizard and lifts the route guard. Idempotent: completing an
// already-complete wizard returns the same state instead of erroring, so a double
// submit (or a refresh on the last step) is harmless.
func (uc *OnboardingUseCase) Complete(ctx context.Context, tenantID, userID uuid.UUID) (*WizardState, error) {
	if tenantID == uuid.Nil || userID == uuid.Nil {
		return nil, domain.NewValidationError("tenant and user are required")
	}

	progress, err := uc.repo.Get(ctx, tenantID, userID)
	if err != nil {
		return nil, err
	}
	if progress == nil {
		progress = &domain.OnboardingProgress{TenantID: tenantID, UserID: userID}
	}
	if !progress.Completed {
		now := uc.now()
		progress.Completed = true
		progress.CompletedAt = &now
	}
	// Park the cursor on the last step this user actually saw, not on the
	// catalogue's last step — which may be one they never walked.
	visible := uc.stepData(ctx, tenantID, userID).VisibleSteps()
	progress.CurrentStep = visible[len(visible)-1]
	progress.TenantID = tenantID
	progress.UserID = userID

	if err := uc.repo.Save(ctx, progress); err != nil {
		return nil, err
	}
	return toWizardState(progress, uc.stepData(ctx, tenantID, userID)), nil
}

// Suggestions is the payload of GET /onboarding/suggestions: the sector/goal
// driven content the wizard and the guided first risk render.
type Suggestions struct {
	Sectors    []onboarding.Sector         `json:"sectors"`
	Goals      []onboarding.Goal           `json:"goals"`
	Risks      []onboarding.RiskSuggestion `json:"risks"`
	Frameworks []string                    `json:"frameworks"`
	Industry   string                      `json:"industry,omitempty"`
	Country    string                      `json:"country,omitempty"`
	Goal       string                      `json:"goal,omitempty"`
}

// GetSuggestions resolves the templates for this user's stored answers, with
// per-request overrides so the wizard can preview a sector before saving it.
//
// Nothing here creates anything: these are drafts the user opens, edits and
// validates (spec §5 — we never auto-create a risk).
func (uc *OnboardingUseCase) GetSuggestions(ctx context.Context, tenantID, userID uuid.UUID, industry, country, goal string) *Suggestions {
	if industry == "" || country == "" || goal == "" {
		if progress, err := uc.repo.Get(ctx, tenantID, userID); err == nil && progress != nil {
			if industry == "" {
				industry = progress.Industry
			}
			if country == "" {
				country = progress.Country
			}
			if goal == "" {
				goal = progress.Goal
			}
		}
	}

	return &Suggestions{
		Sectors:    onboarding.Sectors(),
		Goals:      onboarding.Goals(),
		Risks:      onboarding.RiskSuggestionsFor(industry),
		Frameworks: onboarding.SuggestedFrameworks(industry, country, goal),
		Industry:   industry,
		Country:    country,
		Goal:       goal,
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func toWizardState(p *domain.OnboardingProgress, data domain.OnboardingStepData) *WizardState {
	visible := data.VisibleSteps()
	steps := make([]string, 0, len(visible))
	for _, s := range visible {
		steps = append(steps, string(s))
	}
	skipped := make([]string, 0, len(domain.OnboardingStepOrder)-len(visible))
	for _, s := range domain.OnboardingStepOrder {
		if data.SkipsStep(s) {
			skipped = append(skipped, string(s))
		}
	}

	answers := p.Answers
	if answers == nil {
		answers = domain.JSONMap{}
	}

	// The cursor's position among the VISIBLE steps — what "step N of M" reads.
	//
	// A cursor pointing at a hidden step is SNAPPED onto the visible sequence
	// rather than merely re-indexed. Two ways it gets there and both are normal:
	// a brand-new progress row is initialised on `organization`, which may be the
	// first step skipped; and a stored row can predate the data that now skips
	// its step. Reporting the hidden key would tell the client to render a screen
	// criterion 3 forbids, and reporting index 0 while naming a hidden step would
	// have the stepper and the screen disagree.
	cursor := p.CurrentStep
	idx := -1
	for i, s := range visible {
		if s == cursor {
			idx = i
			break
		}
	}
	if idx < 0 && len(visible) > 0 {
		cursor = nearestVisible(cursor, visible, false)
		for i, s := range visible {
			if s == cursor {
				idx = i
				break
			}
		}
	}
	if idx < 0 {
		idx = 0
	}

	state := &WizardState{
		CurrentStep:  string(cursor),
		Steps:        steps,
		SkippedSteps: skipped,
		StepIndex:    idx,
		Completed:    p.Completed,
		CompletedAt:  p.CompletedAt,
		Industry:     p.Industry,
		Country:      p.Country,
		Goal:         p.Goal,
		Answers:      answers,
		Landing:      onboarding.LandingForGoal(p.Goal),
	}
	if p.Completed {
		state.Percent = 100
	} else {
		state.Percent = idx * 100 / len(steps)
	}
	if state.CurrentStep == "" && len(visible) > 0 {
		state.CurrentStep = string(visible[0])
	}
	return state
}

func stringAnswer(answers domain.JSONMap, key string) string {
	if answers == nil {
		return ""
	}
	if v, ok := answers[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}
