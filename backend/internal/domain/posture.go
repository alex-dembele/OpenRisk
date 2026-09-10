// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package domain

import (
	"context"

	"github.com/google/uuid"

	"github.com/opendefender/openrisk/pkg/scoring"
)

// ---------------------------------------------------------------------------
// Posture read models (#438).
//
// These live in the domain rather than in the application package for one
// structural reason: `internal/infrastructure/repository` cannot import
// `internal/application/activation` — that package imports
// `infrastructure/audittrail`, which imports `infrastructure/repository`, and
// the cycle is real. Ports are declared in the application layer and satisfied
// structurally by the repository (the pattern `OrgUpdater` already uses), which
// only works if the TYPES they exchange sit in a package both may import.
//
// So: the shapes are here, the use cases and the ports stay in the application
// layer, and no layer points the wrong way.
// ---------------------------------------------------------------------------

// TopRiskLimit is how many risks the reveal composes. Small on purpose: the
// screen makes one statement the user can forward to their CISO, not a register
// dump. Lives here so the use case and the repository share one number.
const TopRiskLimit = 5

// PostureRiskCounts is the tenant's risk register, in numbers.
type PostureRiskCounts struct {
	Total int `json:"total"`
	// ByLevel is keyed by the Score Engine's own bands (low/medium/high/critical).
	// The reveal never invents a banding of its own.
	ByLevel map[string]int `json:"by_level"`
}

// PostureControlCounts is the tenant's compliance position, in numbers.
type PostureControlCounts struct {
	Frameworks int `json:"frameworks"`
	Total      int `json:"total"`
	// Implemented, InProgress and NotApplicable are counted separately because
	// coverage and residual credit treat them differently: coverage excludes
	// scoped-out controls from BOTH sides of the ratio, and gives `in_progress`
	// nothing, while ADR 0003 gives `in_progress` partial residual credit.
	Implemented   int `json:"implemented"`
	NotApplicable int `json:"not_applicable"`
	InProgress    int `json:"in_progress"`
}

// Applicable is Total minus the deliberately scoped-out controls.
func (c PostureControlCounts) Applicable() int {
	n := c.Total - c.NotApplicable
	if n < 0 {
		return 0
	}
	return n
}

// PostureRisk is one risk as the reveal reads it, before the residual is applied.
type PostureRisk struct {
	ID    uuid.UUID `json:"id"`
	Title string    `json:"title"`
	// Inherent is Risk.Score — the FROZEN P×I×AC value. The reveal reads it and
	// never recomputes it.
	Inherent float64 `json:"inherent"`
	Level    string  `json:"level"`
}

// RecognitionCounts is what a tenant already holds, straight from its own rows.
type RecognitionCounts struct {
	Risks      int `json:"risks"`
	Frameworks int `json:"frameworks"`
	Controls   int `json:"controls"`
	Assets     int `json:"assets"`
	Members    int `json:"members"`
}

// Any is false for a tenant that holds nothing — the brand-new case, which
// belongs in the tunnel and not on the recognition screen.
//
// One member is NOT recognition: every brand-new tenant has exactly one. Testing
// `Members > 0` here would send every newcomer to the recognition screen and
// nobody to the tunnel, which is the precise inversion of criterion 9.
func (c RecognitionCounts) Any() bool {
	return c.Risks > 0 || c.Frameworks > 0 || c.Controls > 0 || c.Assets > 0 || c.Members > 1
}

// PostureReader is the tenant-scoped read side of the Posture Reveal.
//
// EVERY method MUST filter on tenantID — #438 criterion 10, ABSOLUTE RULE #2.
// The tenant is an argument on every call rather than a field on the
// implementation, so a forgotten filter is a visible omission in a query rather
// than an invisible one in a long-lived struct.
type PostureReader interface {
	RiskCounts(ctx context.Context, tenantID uuid.UUID) (PostureRiskCounts, error)
	// TopRisks returns the tenant's highest-scoring risks, most exposed first.
	TopRisks(ctx context.Context, tenantID uuid.UUID, limit int) ([]PostureRisk, error)
	ControlCounts(ctx context.Context, tenantID uuid.UUID) (PostureControlCounts, error)
	// ControlCreditsByRisk returns, per risk id, the credits ADR 0003 consumes.
	// HasEvidence must be filled from the evidence library, or every implemented
	// control silently earns 0.70 instead of 1.00 and the reveal under-reports
	// the tenant's own posture.
	ControlCreditsByRisk(ctx context.Context, tenantID uuid.UUID, riskIDs []uuid.UUID) (map[uuid.UUID][]scoring.ControlCredit, error)
}

// ---------------------------------------------------------------------------
// Auto-skip (#438 criteria 2 and 3)
// ---------------------------------------------------------------------------

// OnboardingStepData says which of the tunnel's questions this (tenant, user)
// has ALREADY answered with real data.
//
// The server decides, and it decides once: criterion 2 allows exactly one
// GET /onboarding/state for the whole tunnel, so no step may probe on mount.
// Criterion 3 then requires a step whose data exists to never render, never
// flash, and to be absent from the stepper count shown to that user.
type OnboardingStepData struct {
	// HasOrganizationProfile is true when the organisation already carries the
	// facts the organization step collects (an industry and a size), so asking
	// again would be asking a member to re-describe a company that is already
	// described.
	HasOrganizationProfile bool `json:"has_organization_profile"`
	// HasUserProfile is true when this user already has a name and a job title.
	HasUserProfile bool `json:"has_user_profile"`
	// HasFramework is true when the tenant already imported a framework.
	HasFramework bool `json:"has_framework"`
	// HasTeam is true when the tenant has more than its founding member.
	HasTeam bool `json:"has_team"`
}

// OnboardingStepProbe reads the auto-skip decisions. Tenant AND user scoped:
// the profile question is about a person, the framework question is about a
// company, and conflating them would skip a new member's profile step because
// their colleague filled one in.
type OnboardingStepProbe interface {
	OnboardingStepData(ctx context.Context, tenantID, userID uuid.UUID) (OnboardingStepData, error)
}

// SkipsStep answers whether one wizard step should be hidden from this user.
//
// `goal` is NEVER skippable and that is deliberate: it is a preference, not a
// record. No stored row can prove what a user wants to do next, and skipping it
// from an inferred answer would silently choose their landing page for them.
func (d OnboardingStepData) SkipsStep(step OnboardingStepKey) bool {
	switch step {
	case OnboardingStepOrganization:
		return d.HasOrganizationProfile
	case OnboardingStepProfile:
		return d.HasUserProfile
	case OnboardingStepFramework:
		return d.HasFramework
	case OnboardingStepTeam:
		return d.HasTeam
	default:
		return false
	}
}

// VisibleSteps is the ordered sequence this user will actually walk. Never
// empty: `goal` is unskippable, so there is always at least one step and the
// tunnel can never complete without the user having done anything.
func (d OnboardingStepData) VisibleSteps() []OnboardingStepKey {
	out := make([]OnboardingStepKey, 0, len(OnboardingStepOrder))
	for _, s := range OnboardingStepOrder {
		if !d.SkipsStep(s) {
			out = append(out, s)
		}
	}
	return out
}

// RecognitionReader is the tenant-scoped read side of the recognition screen.
type RecognitionReader interface {
	RecognitionCounts(ctx context.Context, tenantID uuid.UUID) (RecognitionCounts, error)
}
