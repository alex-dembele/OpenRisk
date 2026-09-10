// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package scoring

import "math"

// ---------------------------------------------------------------------------
// Residual risk from control coverage — ADR 0003 (accepted), approach decided by
// D-013, constants accepted by D-042.
//
// THE FIVE NUMBERS BELOW ARE NOW LOAD-BEARING. From the first row persisted to
// Risk.ResidualRisk, changing MaxEffectiveness or any credit weight silently
// restates that tenant's history. A different formula takes a NEW
// ResidualFormulaVersion; it does not edit these.
//
// ADDITIVE, like SmartScore. `Risk.Score` and the P×I×AC engine above are frozen
// and appear nowhere in this file's output path: this computes how much of an
// already-computed inherent score the tenant's implemented controls have earned
// back, and nothing else.
//
// Pure and stdlib-only on purpose. It takes credits the caller has already
// fetched, so TENANT ISOLATION IS THE CALLER'S DUTY — `risk_control_mappings`
// carries `tenant_id` and sits behind a tenant-scoped port, and a formula that
// took a repository could not be tested by hand the way this one can.
// ---------------------------------------------------------------------------

// ResidualFormulaVersion is stamped on every result and must be persisted
// alongside any stored residual. A later formula gets "residual-v2"; a stored
// v1 figure is never silently reinterpreted under v2 rules.
const ResidualFormulaVersion = "residual-v1"

// MaxEffectiveness caps how much of the inherent score a complete control set
// can remove. It is a FLOOR ON THE RESIDUAL, not a cap on ambition: a fully
// covered risk keeps 20% of its exposure, because a product that lets paperwork
// drive a risk to zero teaches its users something false.
const MaxEffectiveness = 0.80

// ControlCoverageStatus is the implementation state of one mapped control. The
// values mirror `domain.ControlStatus` deliberately as plain strings: `pkg` must
// not import `internal/domain`, and the parse below is the seam.
type ControlCoverageStatus string

const (
	ControlNotImplemented ControlCoverageStatus = "not_implemented"
	ControlInProgress     ControlCoverageStatus = "in_progress"
	ControlImplemented    ControlCoverageStatus = "implemented"
	ControlNotApplicable  ControlCoverageStatus = "not_applicable"
)

// ParseControlCoverageStatus maps a raw status onto the vocabulary above.
// Anything unrecognised is treated as NOT implemented: an unknown state is not
// evidence of control, and the dangerous failure mode for a security score is
// reading an absent signal as a good one.
func ParseControlCoverageStatus(raw string) ControlCoverageStatus {
	switch ControlCoverageStatus(raw) {
	case ControlImplemented:
		return ControlImplemented
	case ControlInProgress:
		return ControlInProgress
	case ControlNotApplicable:
		return ControlNotApplicable
	default:
		return ControlNotImplemented
	}
}

// ControlCredit is one control mapped to the risk being scored.
type ControlCredit struct {
	Status ControlCoverageStatus
	// HasEvidence discounts a control that is claimed but not proven. An auditor
	// discounts it; so does this.
	HasEvidence bool
}

// Credit is this control's contribution in [0,1]. A not-applicable control has
// no credit AND no weight — see Coverage.
func (c ControlCredit) Credit() float64 {
	switch c.Status {
	case ControlImplemented:
		if c.HasEvidence {
			return 1.0
		}
		return 0.70
	case ControlInProgress:
		return 0.30
	default:
		return 0.0
	}
}

// Coverage is how much of the risk the mapped controls answer for.
type Coverage struct {
	// Measured is false when no APPLICABLE control is mapped. Effectiveness is
	// then zero and the residual equals the inherent score — an absent signal
	// must never read as a good one.
	Measured bool `json:"measured"`
	// Ratio ∈ [0,1] is Σ credit / applicable controls.
	Ratio float64 `json:"ratio"`
	// Effectiveness ∈ [0, MaxEffectiveness] is Ratio scaled by the cap.
	Effectiveness float64 `json:"effectiveness"`
	// Applicable and Total let a caller say "3 of 5 controls, 2 scoped out"
	// without recounting the slice.
	Applicable int `json:"applicable"`
	Total      int `json:"total"`
}

// CoverageEffectiveness reduces a risk's mapped controls to one effectiveness.
//
// `not_applicable` is removed from the numerator AND the denominator: scoring a
// deliberately scoped-out control as an uncovered one would punish a tenant for
// scoping its framework correctly.
func CoverageEffectiveness(credits []ControlCredit) Coverage {
	cov := Coverage{Total: len(credits)}

	var sum float64
	for _, c := range credits {
		if c.Status == ControlNotApplicable {
			continue
		}
		cov.Applicable++
		sum += c.Credit()
	}
	if cov.Applicable == 0 {
		return cov
	}

	cov.Measured = true
	cov.Ratio = clampUnit(sum / float64(cov.Applicable))
	cov.Effectiveness = round3(MaxEffectiveness * cov.Ratio)
	return cov
}

// Residual is one computed residual, on the Score Engine's own scale.
type Residual struct {
	// Inherent is the score handed in, echoed so a caller never has to hold two
	// numbers to render "16 → 6".
	Inherent float64 `json:"inherent"`
	// Value is the residual, on the SAME scale and banded by the SAME thresholds
	// as `Risk.Score`, so a residual and a score are always comparable.
	Value float64          `json:"value"`
	Level CriticalityLevel `json:"level"`
	// Reduction is Inherent − Value, the number the reveal actually celebrates.
	Reduction      float64  `json:"reduction"`
	Coverage       Coverage `json:"coverage"`
	FormulaVersion string   `json:"formula_version"`
}

// ResidualFromInherent applies a coverage to an inherent score.
//
// A negative inherent is clamped to zero rather than propagated: it cannot be
// produced by the engine, and returning a negative residual would band as "low"
// and read as good news.
func ResidualFromInherent(inherent float64, cov Coverage) Residual {
	if inherent < 0 {
		inherent = 0
	}
	inherent = round3(inherent)

	value := round3(inherent * (1 - cov.Effectiveness))
	return Residual{
		Inherent:       inherent,
		Value:          value,
		Level:          residualCriticality(value),
		Reduction:      round3(inherent - value),
		Coverage:       cov,
		FormulaVersion: ResidualFormulaVersion,
	}
}

// ComputeResidual is the one-call form: credits in, residual out.
func ComputeResidual(inherent float64, credits []ControlCredit) Residual {
	return ResidualFromInherent(inherent, CoverageEffectiveness(credits))
}

// residualCriticality bands a residual with the Score Engine's OWN thresholds.
// Deliberately not `smartCriticality` (0–100 bands): a residual lives on the
// P×I×AC scale, and banding it on the wrong scale would call every residual low.
func residualCriticality(score float64) CriticalityLevel {
	score = round3(score)
	switch {
	case score >= 7.000:
		return CriticalityCritical
	case score >= 4.000:
		return CriticalityHigh
	case score >= 2.000:
		return CriticalityMedium
	default:
		return CriticalityLow
	}
}

func round3(v float64) float64 { return math.Round(v*1000) / 1000 }
