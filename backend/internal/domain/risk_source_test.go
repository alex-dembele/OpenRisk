// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only

package domain

import "testing"

// D-012: the onboarding tunnel writes real rows into a customer's register, so
// the register must be able to say which rows it wrote. "starter" is that answer,
// and PR 4 of #438 proves the rows by querying `source = 'starter'` — which only
// works if the parser accepts the value the writer stores.
func TestParseRiskSource_AcceptsStarter(t *testing.T) {
	got, err := ParseRiskSource("starter")
	if err != nil {
		t.Fatalf("ParseRiskSource(\"starter\") errored: %v", err)
	}
	if got != SourceStarter {
		t.Errorf("ParseRiskSource(\"starter\") = %q, want %q", got, SourceStarter)
	}
	if SourceStarter != "starter" {
		t.Errorf("SourceStarter = %q, want the stored value %q", SourceStarter, "starter")
	}
	// The column is varchar(20); a longer value would be silently truncated by
	// Postgres and would break the provenance query that PR 4 depends on.
	if len(string(SourceStarter)) > 20 {
		t.Errorf("SourceStarter is %d chars, the column holds 20", len(string(SourceStarter)))
	}
}

func TestParseRiskSource_KnownValuesAndRejections(t *testing.T) {
	for _, want := range []RiskSource{
		SourceManual, SourceCTIAuto, SourceScanAuto,
		SourceImport, SourceVendor, SourceAI, SourceStarter,
	} {
		got, err := ParseRiskSource(string(want))
		if err != nil || got != want {
			t.Errorf("ParseRiskSource(%q) = (%q, %v)", want, got, err)
		}
	}

	// Empty still means manual — the ERD column default.
	if got, err := ParseRiskSource(""); err != nil || got != SourceManual {
		t.Errorf("ParseRiskSource(\"\") = (%q, %v), want manual", got, err)
	}

	for _, bad := range []string{"Starter", "starter_risk", "seed", "onboarding"} {
		if _, err := ParseRiskSource(bad); err == nil {
			t.Errorf("ParseRiskSource(%q) must be rejected", bad)
		}
	}
}
