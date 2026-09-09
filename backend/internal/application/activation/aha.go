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
)

// AhaSignal is what the executive dashboard observed while computing a score.
// Deliberately primitive: the dashboard use case declares a one-method interface
// with these arguments and satisfies it structurally, so `internal/application/
// dashboard` never imports this package (and no import cycle is possible).
type AhaSignal struct {
	// ScoreComputed is true when a cyber score was actually produced (not the
	// zero value of a degraded, source-less computation).
	ScoreComputed bool
	// OwnDataPoints counts the tenant's own records behind that score (risks,
	// vulnerabilities, incidents). Zero means the score describes an empty
	// workspace — a number, not an insight.
	OwnDataPoints int
	// ComplianceGaps is the number of identified compliance gaps.
	ComplianceGaps int
}

// IsAha implements the product definition of the Aha moment (spec §7):
//
//	the first cyber score computed on the user's OWN data, WITH at least one
//	compliance gap identified.
//
// Both halves matter and neither is decorative. A score with no data behind it is
// a gauge pointing at nothing; a score with no gap identified tells the user
// everything is fine, which is not the moment they understand what the product is
// for. The moment of understanding is "here is my posture, and here is precisely
// what is missing" — that is what this predicate encodes.
func (s AhaSignal) IsAha() bool {
	return s.ScoreComputed && s.OwnDataPoints > 0 && s.ComplianceGaps > 0
}

// AhaRecorder records the Aha moment at most once per tenant and observes the
// time_to_aha histogram the P50 alert watches.
type AhaRecorder struct {
	repo     domain.ActivationRepository
	recorder *Recorder
	now      func() time.Time
}

// NewAhaRecorder builds the recorder. A nil repo makes every method a no-op.
func NewAhaRecorder(repo domain.ActivationRepository) *AhaRecorder {
	return &AhaRecorder{
		repo:     repo,
		recorder: NewRecorder(repo),
		now:      func() time.Time { return time.Now().UTC() },
	}
}

// MaybeRecordAha is the entry point the dashboard use case calls on every score
// computation. It is cheap and idempotent: after the first Aha it does one
// existence check and returns.
//
// Best-effort like the rest of the package — a metric must never be able to fail
// a dashboard.
func (a *AhaRecorder) MaybeRecordAha(ctx context.Context, tenantID uuid.UUID, scoreComputed bool, ownDataPoints, complianceGaps int) {
	if a == nil || a.repo == nil || tenantID == uuid.Nil {
		return
	}
	signal := AhaSignal{
		ScoreComputed:  scoreComputed,
		OwnDataPoints:  ownDataPoints,
		ComplianceGaps: complianceGaps,
	}
	if !signal.IsAha() {
		return
	}

	// Once per tenant, ever.
	if seen, err := a.repo.HasEvent(ctx, tenantID, domain.ActivationAhaReached); err != nil || seen {
		return
	}

	reachedAt := a.now()
	a.recorder.Record(ctx, tenantID, string(domain.ActivationAhaReached), map[string]interface{}{
		"own_data_points": ownDataPoints,
		"compliance_gaps": complianceGaps,
	})

	// Count the tenant here, not inside ObserveTimeToAha. A tenant with no signup
	// anchor has still reached the Aha; counting it only alongside the duration
	// dropped exactly those tenants from the activation rate (D-010).
	monitoring.CountAhaReached()

	// Observe signup → Aha under the v1 definition. Without a signup anchor there
	// is no honest duration, so we record the event but observe nothing rather
	// than inventing a t0 that would flatter the histogram.
	//
	// v1 is this definition and only this one. When the Posture Reveal ships it
	// observes AhaDefinitionV2 from its own call site; the two series are never
	// mixed, because the durations they measure are not comparable (D-010).
	firsts, err := a.repo.FirstOccurrences(ctx, tenantID)
	if err != nil {
		return
	}
	if signup, ok := firsts[domain.ActivationSignup]; ok {
		monitoring.ObserveTimeToAha(monitoring.AhaDefinitionV1, reachedAt.Sub(signup))
	}
}
