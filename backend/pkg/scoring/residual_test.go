// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only

package scoring

import "testing"

func TestCoverageEffectiveness_CreditTable(t *testing.T) {
	cases := []struct {
		name   string
		credit ControlCredit
		want   float64
	}{
		{"implemented with evidence", ControlCredit{ControlImplemented, true}, 1.0},
		{"implemented without evidence", ControlCredit{ControlImplemented, false}, 0.70},
		{"in progress", ControlCredit{ControlInProgress, false}, 0.30},
		{"not implemented", ControlCredit{ControlNotImplemented, false}, 0.0},
		{"not applicable", ControlCredit{ControlNotApplicable, false}, 0.0},
	}
	for _, tc := range cases {
		if got := tc.credit.Credit(); got != tc.want {
			t.Errorf("%s: credit = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// not_applicable leaves the denominator as well as the numerator: a tenant that
// scopes its framework correctly must not be scored as if it had failed.
func TestCoverageEffectiveness_NotApplicableIsExcludedFromBothSides(t *testing.T) {
	withScopeOuts := CoverageEffectiveness([]ControlCredit{
		{ControlImplemented, true},
		{ControlNotApplicable, false},
		{ControlNotApplicable, false},
	})
	alone := CoverageEffectiveness([]ControlCredit{{ControlImplemented, true}})

	if withScopeOuts.Ratio != alone.Ratio {
		t.Errorf("scoped-out controls changed the ratio: %v vs %v", withScopeOuts.Ratio, alone.Ratio)
	}
	if withScopeOuts.Applicable != 1 || withScopeOuts.Total != 3 {
		t.Errorf("applicable/total = %d/%d, want 1/3", withScopeOuts.Applicable, withScopeOuts.Total)
	}
	if withScopeOuts.Effectiveness != MaxEffectiveness {
		t.Errorf("full coverage effectiveness = %v, want %v", withScopeOuts.Effectiveness, MaxEffectiveness)
	}
}

// The rule that matters most in a security score: an absent signal must never
// read as a good one.
func TestCoverageEffectiveness_NoApplicableControlIsNotGoodNews(t *testing.T) {
	for _, credits := range [][]ControlCredit{
		nil,
		{},
		{{ControlNotApplicable, false}},
	} {
		cov := CoverageEffectiveness(credits)
		if cov.Measured {
			t.Errorf("%v: coverage must not be reported as measured", credits)
		}
		if cov.Effectiveness != 0 {
			t.Errorf("%v: effectiveness = %v, want 0", credits, cov.Effectiveness)
		}

		res := ResidualFromInherent(16, cov)
		if res.Value != 16 {
			t.Errorf("%v: residual = %v, want the inherent 16 untouched", credits, res.Value)
		}
		if res.Reduction != 0 {
			t.Errorf("%v: reduction = %v, want 0", credits, res.Reduction)
		}
	}
}

// MaxEffectiveness is a floor on the residual. Full coverage must not reach zero,
// whatever the inherent score is.
func TestResidual_FullCoverageNeverReachesZero(t *testing.T) {
	full := CoverageEffectiveness([]ControlCredit{
		{ControlImplemented, true},
		{ControlImplemented, true},
	})
	if full.Ratio != 1 {
		t.Fatalf("precondition: ratio = %v, want 1", full.Ratio)
	}

	for _, inherent := range []float64{30, 16, 7.5, 2, 0.5} {
		res := ResidualFromInherent(inherent, full)
		if res.Value <= 0 && inherent > 0 {
			t.Errorf("inherent %v: residual reached %v — the floor is gone", inherent, res.Value)
		}
		want := round3(inherent * (1 - MaxEffectiveness))
		if res.Value != want {
			t.Errorf("inherent %v: residual = %v, want %v", inherent, res.Value, want)
		}
	}
}

// A residual lives on the Score Engine's scale and must band on the Score
// Engine's thresholds — banding it on the 0–100 SmartScore scale would call
// every residual "low".
func TestResidual_BandsOnTheScoreEngineThresholds(t *testing.T) {
	none := Coverage{}
	cases := []struct {
		inherent float64
		want     CriticalityLevel
	}{
		{7.0, CriticalityCritical},
		{6.999, CriticalityHigh},
		{4.0, CriticalityHigh},
		{3.999, CriticalityMedium},
		{2.0, CriticalityMedium},
		{1.999, CriticalityLow},
		{0, CriticalityLow},
	}
	for _, tc := range cases {
		if got := ResidualFromInherent(tc.inherent, none).Level; got != tc.want {
			t.Errorf("residual %v banded %q, want %q", tc.inherent, got, tc.want)
		}
	}
}

// The arithmetic a reader of the reveal must be able to check by hand.
func TestComputeResidual_IsCheckableByHand(t *testing.T) {
	// Two controls, one implemented and evidenced (1.0), one in progress (0.30).
	// ratio = 1.30 / 2 = 0.65 · effectiveness = 0.80 × 0.65 = 0.52
	// residual = 16 × (1 − 0.52) = 7.68
	res := ComputeResidual(16, []ControlCredit{
		{ControlImplemented, true},
		{ControlInProgress, false},
	})

	if res.Coverage.Ratio != 0.65 {
		t.Errorf("ratio = %v, want 0.65", res.Coverage.Ratio)
	}
	if res.Coverage.Effectiveness != 0.52 {
		t.Errorf("effectiveness = %v, want 0.52", res.Coverage.Effectiveness)
	}
	if res.Value != 7.68 {
		t.Errorf("residual = %v, want 7.68", res.Value)
	}
	if res.Reduction != 8.32 {
		t.Errorf("reduction = %v, want 8.32", res.Reduction)
	}
	if res.Inherent != 16 {
		t.Errorf("inherent echoed as %v, want 16", res.Inherent)
	}
	if res.FormulaVersion != ResidualFormulaVersion {
		t.Errorf("formula version = %q, want %q", res.FormulaVersion, ResidualFormulaVersion)
	}
}

// An unknown status is not evidence of control.
func TestParseControlCoverageStatus_UnknownIsNotImplemented(t *testing.T) {
	for _, raw := range []string{"", "IMPLEMENTED", "partially", "waived", "unknown"} {
		if got := ParseControlCoverageStatus(raw); got != ControlNotImplemented {
			t.Errorf("ParseControlCoverageStatus(%q) = %q, want %q", raw, got, ControlNotImplemented)
		}
	}
	for _, raw := range []ControlCoverageStatus{
		ControlImplemented, ControlInProgress, ControlNotApplicable, ControlNotImplemented,
	} {
		if got := ParseControlCoverageStatus(string(raw)); got != raw {
			t.Errorf("ParseControlCoverageStatus(%q) = %q", raw, got)
		}
	}
}

// A negative inherent cannot come from the engine, but if one ever arrives it
// must not band as "low" and read as good news.
func TestResidual_NegativeInherentIsClamped(t *testing.T) {
	res := ResidualFromInherent(-5, Coverage{})
	if res.Inherent != 0 || res.Value != 0 {
		t.Errorf("negative inherent produced %+v, want zeros", res)
	}
}

// Risk.Score is frozen: nothing here may change what the engine returns.
func TestResidual_LeavesTheScoreEngineUntouched(t *testing.T) {
	e := NewEngine()
	before, err := e.Calculate(0.8, 8, 2.5)
	if err != nil {
		t.Fatalf("engine: %v", err)
	}

	ComputeResidual(before, []ControlCredit{{ControlImplemented, true}})

	after, err := e.Calculate(0.8, 8, 2.5)
	if err != nil || after != before {
		t.Errorf("engine moved from %v to %v (err %v)", before, after, err)
	}
}
