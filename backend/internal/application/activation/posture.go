// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package activation

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/opendefender/openrisk/internal/domain"
	"github.com/opendefender/openrisk/pkg/monitoring"
	"github.com/opendefender/openrisk/pkg/scoring"
)

// ---------------------------------------------------------------------------
// The Posture Reveal (#438) — the surface that DEFINES the Aha moment.
//
// Everything here is computed from the tenant's OWN rows. #438 criterion 7 is
// absolute: no sample, no demo, no placeholder value may reach the payload. That
// is not a UI preference — the reveal is the launch-gate metric, and a screen
// that renders a plausible number from fixture data would make the metric report
// success while activation is nil.
//
// Criterion 8 is the counterpart and is why Execute returns an error rather than
// a half-empty summary: if the payload would be empty, NOTHING is recorded. No
// `posture.revealed` event, no histogram observation. A reveal that cannot show
// anything has not revealed anything, and recording it would quietly turn a
// rendering failure into a product success.
// ---------------------------------------------------------------------------

// The read models and the ports live in internal/domain (see domain/posture.go
// for why: repository cannot import this package). Aliased here so the use case
// and its tests read naturally and so a caller never has to know which package a
// shape came from.
type (
	PostureRiskCounts    = domain.PostureRiskCounts
	PostureControlCounts = domain.PostureControlCounts
	PostureRisk          = domain.PostureRisk
	PostureReader        = domain.PostureReader
)

// PostureRiskView is one risk with its residual, as the reveal renders it.
type PostureRiskView struct {
	PostureRisk
	Residual scoring.Residual `json:"residual"`
}

// TopRiskLimit is how many risks the reveal composes — defined in the domain so
// the repository shares the same number without importing this package.
const TopRiskLimit = domain.TopRiskLimit

// PostureSummary is the payload of GET /posture.
type PostureSummary struct {
	Risks    PostureRiskCounts    `json:"risks"`
	Controls PostureControlCounts `json:"controls"`
	// CoveragePercent is nil, NOT zero, when no applicable control exists. Zero
	// would read as "you have covered nothing", which is a different and false
	// statement from "there is nothing to cover yet".
	CoveragePercent *float64          `json:"coverage_percent"`
	TopRisks        []PostureRiskView `json:"top_risks"`
	// ResidualFormulaVersion is echoed so a stored or forwarded figure can never
	// be reinterpreted under a later formula (ADR 0003).
	ResidualFormulaVersion string    `json:"residual_formula_version"`
	GeneratedAt            time.Time `json:"generated_at"`
	// RevealedAt is when this tenant FIRST reached the reveal. Stable across
	// re-renders — criterion 11, first-occurrence-wins.
	RevealedAt *time.Time `json:"revealed_at,omitempty"`
	// FirstReveal is true only on the render that recorded the event, so the
	// client can celebrate once without deciding anything itself.
	FirstReveal bool `json:"first_reveal"`
}

// IsRevealable encodes criterion 6's three requirements and criterion 8's
// refusal. All three must hold or there is nothing to reveal:
//
//	a non-zero risk count · a non-null coverage percentage · at least one residual
func (s *PostureSummary) IsRevealable() bool {
	return s != nil &&
		s.Risks.Total > 0 &&
		s.CoveragePercent != nil &&
		len(s.TopRisks) > 0
}

// PostureUseCase computes the reveal and records the Aha exactly once.
type PostureUseCase struct {
	reader   PostureReader
	repo     domain.ActivationRepository
	recorder *Recorder
	now      func() time.Time
}

// NewPostureUseCase builds the use case. Both dependencies are required: unlike
// the rest of this package, the reveal must NOT degrade silently — a reveal that
// renders zeros because its reader was nil is exactly the failure criterion 8
// exists to prevent.
func NewPostureUseCase(reader PostureReader, repo domain.ActivationRepository, recorder *Recorder) *PostureUseCase {
	return &PostureUseCase{
		reader:   reader,
		repo:     repo,
		recorder: recorder,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

// Execute computes the reveal for one (tenant, user) and, when it is revealable
// and this is the first time, records `posture.revealed` and observes the v2
// time-to-Aha histogram.
//
// Errors are typed and each one means something different to the client:
//
//   - ErrForbidden  — no tenant or no user on the session.
//   - ErrNotFound   — the tenant has nothing to reveal yet (criterion 8). NOTHING
//     is recorded and NOTHING is observed on this path.
func (uc *PostureUseCase) Execute(ctx context.Context, tenantID, userID uuid.UUID) (*PostureSummary, error) {
	if tenantID == uuid.Nil || userID == uuid.Nil {
		return nil, domain.NewForbiddenError("a tenant and a user are required to read a posture")
	}
	if uc.reader == nil {
		return nil, domain.NewInternalError("posture reader is not wired")
	}

	counts, err := uc.reader.RiskCounts(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	controls, err := uc.reader.ControlCounts(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	top, err := uc.reader.TopRisks(ctx, tenantID, TopRiskLimit)
	if err != nil {
		return nil, err
	}

	ids := make([]uuid.UUID, 0, len(top))
	for _, r := range top {
		ids = append(ids, r.ID)
	}
	credits := map[uuid.UUID][]scoring.ControlCredit{}
	if len(ids) > 0 {
		credits, err = uc.reader.ControlCreditsByRisk(ctx, tenantID, ids)
		if err != nil {
			return nil, err
		}
	}

	views := make([]PostureRiskView, 0, len(top))
	for _, r := range top {
		views = append(views, PostureRiskView{
			PostureRisk: r,
			Residual:    scoring.ComputeResidual(r.Inherent, credits[r.ID]),
		})
	}

	summary := &PostureSummary{
		Risks:                  counts,
		Controls:               controls,
		CoveragePercent:        coveragePercent(controls),
		TopRisks:               views,
		ResidualFormulaVersion: scoring.ResidualFormulaVersion,
		GeneratedAt:            uc.now(),
	}

	if !summary.IsRevealable() {
		// Criterion 8: fail loudly, record nothing, observe nothing. The client
		// renders an explicit error state and the metric stays honest.
		return nil, domain.NewNotFoundError("posture", tenantID)
	}

	uc.recordReveal(ctx, tenantID, userID, summary)
	return summary, nil
}

// recordReveal writes `posture.revealed` at most once per tenant and observes the
// v2 histogram. Best-effort like the rest of this package: a metric must never be
// able to fail the screen it measures.
func (uc *PostureUseCase) recordReveal(ctx context.Context, tenantID, userID uuid.UUID, summary *PostureSummary) {
	if uc.repo == nil {
		return
	}

	firsts, err := uc.repo.FirstOccurrences(ctx, tenantID)
	if err != nil {
		return
	}

	// Criterion 11: first-occurrence-wins. An already-revealed tenant gets its
	// original timestamp back and no second row is written.
	if at, seen := firsts[domain.ActivationPostureRevealed]; seen {
		revealedAt := at
		summary.RevealedAt = &revealedAt
		return
	}

	recorded := uc.recorder.RecordOnce(ctx, tenantID, string(domain.ActivationPostureRevealed),
		map[string]interface{}{
			"risk_count":       summary.Risks.Total,
			"coverage_percent": summary.CoveragePercent,
			"top_risks":        len(summary.TopRisks),
			"formula_version":  summary.ResidualFormulaVersion,
		})
	if !recorded {
		// Another request won the race. Not an error, and not a second event.
		return
	}

	// Read the timestamp BACK from the stored event rather than taking a second
	// clock reading. The recorder stamps `occurred_at` with its own clock, so a
	// second reading here would report a `revealed_at` that differs — by
	// microseconds in production, by enough to fail criterion 11's "completed_at
	// is unchanged" the moment the screen is rendered twice.
	stored, err := uc.repo.FirstOccurrences(ctx, tenantID)
	if err != nil {
		return
	}
	at, ok := stored[domain.ActivationPostureRevealed]
	if !ok {
		// Recorded but unreadable: the store swallowed it. Do not observe a
		// duration against a t0 we cannot prove.
		return
	}
	revealedAt := at
	summary.RevealedAt = &revealedAt
	summary.FirstReveal = true

	// Count the tenant whether or not a signup anchor exists to measure against —
	// the defect D-010 told this work not to inherit.
	monitoring.CountAhaReached()

	// v2 is the Posture Reveal definition and only that. Without a signup anchor
	// there is no honest duration, so nothing is observed rather than inventing a
	// t0 that would flatter the histogram.
	if signup, ok := firsts[domain.ActivationSignup]; ok {
		monitoring.ObserveTimeToAha(monitoring.AhaDefinitionV2, revealedAt.Sub(signup))
	}
}

// coveragePercent is implemented controls over APPLICABLE controls, rounded to
// one decimal. Nil when nothing is applicable: zero would say "you have covered
// nothing", which is false when there is nothing to cover.
//
// `in_progress` earns no coverage here even though ADR 0003 gives it residual
// credit. The two answer different questions: coverage is "how much of the
// framework is done", and a control being worked on is not done.
func coveragePercent(c PostureControlCounts) *float64 {
	applicable := c.Applicable()
	if applicable <= 0 {
		return nil
	}
	pct := float64(c.Implemented) / float64(applicable) * 100
	pct = float64(int64(pct*10+0.5)) / 10
	return &pct
}
