// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package entitlements

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// docs/PRICING.md is a PUBLIC PRICE LIST. It says, in words a customer reads
// before paying, what each plan grants and what it costs.
//
// It also claims to be derived from this package ("The matrix below is the
// single source of truth backend/pkg/entitlements/entitlements.go"). Nothing
// checked that. #415 exists because a matrix change reached the code through a
// commit nobody was reviewing as pricing; the mirror image — the published page
// drifting away from the enforced matrix — is the same defect pointing the other
// way, and it is the one a customer notices.
//
// This test READS the published tables and compares them to the code. It
// deliberately does not generate the page: PRICING.md is written for humans,
// with ⭐, ∞ and "On quote", and regenerating it would flatten that. The
// vocabulary a cell may use is declared below, so a new word in the table is a
// failure rather than a silent pass.
//
// It asserts EVERY row and EVERY plan, so it is not satisfied by a spot check:
// a row missing from the page fails, and a feature missing from the page fails.
// ---------------------------------------------------------------------------

func pricingDocPath() string {
	return filepath.Join("..", "..", "..", "docs", "PRICING.md")
}

// docLimits maps a row label in the published table to the limit it describes.
var docLimits = map[string]LimitKey{
	"Users":        LimitUsers,
	"Risks":        LimitRisks,
	"Assets":       LimitAssets,
	"Integrations": LimitIntegrations,
}

// docFeatures maps a row label to the feature it describes. Every entry of
// AllFeatures must appear here, which is asserted below.
var docFeatures = map[string]Feature{
	"API":                            FeatAPI,
	"Automation (SOAR)":              FeatAutomation,
	"AI Advisor":                     FeatAIAdvisor,
	"Financial quant. (Monte-Carlo)": FeatFinancialQuant,
	"Smart risk score":               FeatSmartScore,
	"Executive dashboard":            FeatExecutiveDashboard,
	"Infra scanner":                  FeatScanner,
	"Compliance":                     FeatCompliance,
	"Threat intel (CTI)":             FeatCTI,
	"Governance / approvals":         FeatGovernance,
	"SSO / SAML":                     FeatSSO,
	"Multi-tenant":                   FeatMultiTenant,
	"On-premise":                     FeatOnPremise,
	"SLA":                            FeatSLA,
	"Support":                        FeatSupport,
}

// docLevels is the vocabulary a feature cell may use. An unlisted word fails,
// so the page cannot invent a tier that the code does not have.
var docLevels = map[string]Level{
	"—":         LevelOff,
	"✓":         LevelOn,
	"Limited":   LevelLimited,
	"Basic":     LevelBasic,
	"Standard":  LevelStandard,
	"Advanced":  LevelAdvanced,
	"Custom":    LevelCustom,
	"Community": LevelCommunity,
	"Email":     LevelEmail,
	"Priority":  LevelPriority,
	"Dedicated": LevelDedicated,
	"99.5 %":    Level("99.5"),
	"99.9 %":    Level("99.9"),
}

// docPlanOrder is the column order the published table uses.
var docPlanOrder = []Plan{PlanFree, PlanPro, PlanBusiness, PlanEnterprise}

// squash removes every kind of space, including the non-breaking and narrow
// no-break spaces a price like "12 500 XAF" is typeset with.
func squash(s string) string {
	r := strings.NewReplacer(" ", "", " ", "", " ", "", " ", "")
	return r.Replace(s)
}

// tableRows returns the cells of every markdown table row between two headings.
func tableRows(t *testing.T, doc, fromHeading, toHeading string) [][]string {
	t.Helper()
	start := strings.Index(doc, fromHeading)
	if start < 0 {
		t.Fatalf("docs/PRICING.md has no %q section", fromHeading)
	}
	rest := doc[start+len(fromHeading):]
	if toHeading != "" {
		if end := strings.Index(rest, toHeading); end >= 0 {
			rest = rest[:end]
		}
	}
	var out [][]string
	for _, line := range strings.Split(rest, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") {
			continue
		}
		if strings.Contains(line, "---") { // separator
			continue
		}
		parts := strings.Split(strings.Trim(line, "|"), "|")
		cells := make([]string, 0, len(parts))
		for _, p := range parts {
			cells = append(cells, strings.TrimSpace(strings.ReplaceAll(p, "**", "")))
		}
		out = append(out, cells)
	}
	return out
}

func TestPricingDoc_FeatureMatrixMatchesTheCode(t *testing.T) {
	raw, err := os.ReadFile(pricingDocPath())
	if err != nil {
		t.Fatalf("docs/PRICING.md must be readable: %v", err)
	}

	rows := tableRows(t, string(raw), "## Feature matrix", "## Pricing")
	if len(rows) == 0 {
		t.Fatal("no table found under ## Feature matrix")
	}

	seenLimits := map[LimitKey]bool{}
	seenFeatures := map[Feature]bool{}

	for _, cells := range rows {
		label := cells[0]
		if label == "" { // the header row's empty first cell
			continue
		}
		if len(cells) != 5 {
			t.Errorf("row %q has %d cells, expected 5 (label + 4 plans)", label, len(cells))
			continue
		}
		values := cells[1:]

		if key, ok := docLimits[label]; ok {
			seenLimits[key] = true
			for i, plan := range docPlanOrder {
				cell := squash(values[i])
				want := LimitOf(plan, key)
				var got int
				switch cell {
				case "∞", "Custom":
					got = Unlimited
				default:
					n, convErr := strconv.Atoi(cell)
					if convErr != nil {
						t.Errorf("%s / %s: cannot read limit cell %q", label, plan, values[i])
						continue
					}
					got = n
				}
				if got != want {
					t.Errorf("%s / %s: PRICING.md says %q, the code enforces %d",
						label, plan, values[i], want)
				}
			}
			continue
		}

		if feat, ok := docFeatures[label]; ok {
			seenFeatures[feat] = true
			for i, plan := range docPlanOrder {
				cell := values[i]
				want := LevelOf(plan, feat)
				got, known := docLevels[cell]
				if !known {
					t.Errorf("%s / %s: %q is not a level this table may use — add it to docLevels or fix the page",
						label, plan, cell)
					continue
				}
				if got != want {
					t.Errorf("%s / %s: PRICING.md says %q (%s), the code grants %q",
						label, plan, cell, got, want)
				}
			}
			continue
		}

		t.Errorf("PRICING.md row %q maps to no limit and no feature — the page "+
			"advertises something the matrix does not define", label)
	}

	// Nothing may be quietly dropped from the published page.
	for _, k := range AllLimits {
		if !seenLimits[k] {
			t.Errorf("limit %q is enforced by the code and absent from PRICING.md", k)
		}
	}
	for _, f := range AllFeatures {
		if !seenFeatures[f] {
			t.Errorf("feature %q is enforced by the code and absent from PRICING.md", f)
		}
	}
}

func TestPricingDoc_PricesMatchTheCode(t *testing.T) {
	raw, err := os.ReadFile(pricingDocPath())
	if err != nil {
		t.Fatalf("docs/PRICING.md must be readable: %v", err)
	}
	doc := string(raw)

	for _, tc := range []struct {
		heading string
		next    string
		region  Region
		symbol  string
	}{
		{"### Europe (EUR)", "### Africa", RegionEU, "€"},
		{"### Africa (XAF/XOF — CFA franc)", "## How enforcement works", RegionAfrica, "XAF"},
	} {
		rows := tableRows(t, doc, tc.heading, tc.next)
		seen := map[Plan]bool{}
		for _, cells := range rows {
			label := strings.TrimSpace(strings.ReplaceAll(cells[0], "⭐", ""))
			if label == "" || label == "Plan" {
				continue
			}
			plan := ParsePlan(strings.ToLower(label))
			if strings.ToLower(label) != string(plan) {
				t.Errorf("%s: %q is not a plan in the code", tc.heading, label)
				continue
			}
			seen[plan] = true

			want := PriceFor(tc.region, plan)
			cell := strings.TrimSpace(cells[1])

			if want.Custom {
				if !strings.EqualFold(cell, "On quote") {
					t.Errorf("%s / %s: the code prices this on quote, PRICING.md says %q",
						tc.heading, plan, cell)
				}
				continue
			}
			amount := squash(strings.NewReplacer("€", "", "XAF", "").Replace(cell))
			n, convErr := strconv.Atoi(amount)
			if convErr != nil {
				t.Errorf("%s / %s: cannot read price %q", tc.heading, plan, cell)
				continue
			}
			if n != want.Amount {
				t.Errorf("%s / %s: PRICING.md says %d, the code charges %d",
					tc.heading, plan, n, want.Amount)
			}
			if want.Currency != "" && !strings.Contains(cell, tc.symbol) {
				t.Errorf("%s / %s: %q does not carry the %s currency mark",
					tc.heading, plan, cell, want.Currency)
			}
		}
		for _, p := range AllPlans {
			if !seen[p] {
				t.Errorf("%s: plan %q is priced in the code and absent from the table", tc.heading, p)
			}
		}
	}
}
