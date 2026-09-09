// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package entitlements

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// The self-hosting capability table is GENERATED from the matrix above.
//
// #328 asks for "documentation explicite des limites du self-hosted vs SaaS".
// A hand-written table would be true on the day it was written and quietly wrong
// on the day someone edits `matrix` — which is exactly the class of claim rule 12
// of CLAUDE.md forbids. So the table in docs/SELF_HOSTING.md is rendered from the
// code, and this test fails the build when the file and the matrix disagree.
//
//	regenerate with:  go test ./pkg/entitlements/ -run TestSelfHostDoc -update
// ---------------------------------------------------------------------------

var updateDoc = flag.Bool("update", false, "rewrite the generated block in docs/SELF_HOSTING.md")

const (
	docBeginMarker = "<!-- BEGIN GENERATED: entitlements matrix -->"
	docEndMarker   = "<!-- END GENERATED: entitlements matrix -->"
)

// selfHostDocPath is docs/SELF_HOSTING.md, relative to this package.
func selfHostDocPath() string {
	return filepath.Join("..", "..", "..", "docs", "SELF_HOSTING.md")
}

// limitLabels and featureLabels give each key the words a reader understands.
// A key with no label here fails the test rather than printing a raw
// identifier — adding a feature to the matrix must also name it for humans.
var limitLabels = map[LimitKey]string{
	LimitUsers:        "Users",
	LimitRisks:        "Risks",
	LimitAssets:       "Assets",
	LimitIntegrations: "Integrations",
}

var featureLabels = map[Feature]string{
	FeatAPI:                "REST API",
	FeatAutomation:         "Automation rules",
	FeatAIAdvisor:          "AI advisor",
	FeatCompliance:         "Compliance frameworks",
	FeatSSO:                "SSO (SAML / OIDC)",
	FeatMultiTenant:        "Multi-tenant",
	FeatOnPremise:          "On-premise entitlement",
	FeatFinancialQuant:     "Financial quantification",
	FeatSmartScore:         "SmartScore",
	FeatExecutiveDashboard: "Executive dashboard",
	FeatScanner:            "Scanner",
	FeatCTI:                "Threat intelligence (CTI)",
	FeatGovernance:         "Governance",
	FeatSLA:                "SLA",
	FeatSupport:            "Support",
}

func limitCell(p Plan, k LimitKey) string {
	if v := LimitOf(p, k); v != Unlimited {
		return fmt.Sprintf("%d", v)
	}
	return "unlimited"
}

func levelCell(p Plan, f Feature) string {
	lvl := LevelOf(p, f)
	if !lvl.Enabled() {
		return "—"
	}
	switch lvl {
	case LevelOn:
		return "yes"
	case Level("99.5"), Level("99.9"):
		return string(lvl) + "%"
	default:
		return string(lvl)
	}
}

// renderSelfHostMatrix produces the markdown block the documentation carries.
func renderSelfHostMatrix(t *testing.T) string {
	t.Helper()
	var b strings.Builder

	b.WriteString(docBeginMarker)
	b.WriteString("\n")
	b.WriteString("<!-- Generated from backend/pkg/entitlements/entitlements.go.\n")
	b.WriteString("     Do not edit by hand: `go test ./pkg/entitlements/ -run TestSelfHostDoc -update`. -->\n\n")

	header := "| Capability |"
	rule := "|---|"
	for _, p := range AllPlans {
		header += " " + planTitle(p) + " |"
		rule += "---|"
	}
	b.WriteString(header + "\n" + rule + "\n")

	for _, k := range AllLimits {
		label, ok := limitLabels[k]
		if !ok {
			t.Fatalf("limit %q has no human label in limitLabels — name it before shipping it", k)
		}
		row := "| " + label + " |"
		for _, p := range AllPlans {
			row += " " + limitCell(p, k) + " |"
		}
		b.WriteString(row + "\n")
	}

	for _, f := range AllFeatures {
		label, ok := featureLabels[f]
		if !ok {
			t.Fatalf("feature %q has no human label in featureLabels — name it before shipping it", f)
		}
		row := "| " + label + " |"
		for _, p := range AllPlans {
			row += " " + levelCell(p, f) + " |"
		}
		b.WriteString(row + "\n")
	}

	b.WriteString("\n")
	b.WriteString(fmt.Sprintf(
		"A self-hosted instance registers its first organisation with no plan set, "+
			"so it resolves to **%s**: %s users, %s risks, %s assets, %s integration(s).\n",
		planTitle(PlanFree),
		limitCell(PlanFree, LimitUsers),
		limitCell(PlanFree, LimitRisks),
		limitCell(PlanFree, LimitAssets),
		limitCell(PlanFree, LimitIntegrations),
	))
	b.WriteString("\n")
	b.WriteString(docEndMarker)
	return b.String()
}

func planTitle(p Plan) string {
	s := string(p)
	if s == "" {
		return "Free"
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func TestSelfHostDoc_MatchesTheMatrix(t *testing.T) {
	path := selfHostDocPath()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("docs/SELF_HOSTING.md must be readable: %v", err)
	}
	doc := string(raw)

	start := strings.Index(doc, docBeginMarker)
	end := strings.Index(doc, docEndMarker)
	if start < 0 || end < 0 || end < start {
		t.Fatalf("docs/SELF_HOSTING.md carries no generated block — expected %q … %q",
			docBeginMarker, docEndMarker)
	}

	want := renderSelfHostMatrix(t)
	got := doc[start : end+len(docEndMarker)]

	if got == want {
		return
	}

	if *updateDoc {
		updated := doc[:start] + want + doc[end+len(docEndMarker):]
		if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
			t.Fatalf("rewriting docs/SELF_HOSTING.md: %v", err)
		}
		t.Log("docs/SELF_HOSTING.md regenerated from the matrix")
		return
	}

	t.Errorf(`docs/SELF_HOSTING.md no longer matches the entitlement matrix.

The published self-hosting table is what a user reads before trusting this
product with their register. It has drifted from backend/pkg/entitlements.

Regenerate it:

    go test ./pkg/entitlements/ -run TestSelfHostDoc -update

--- documented ---
%s

--- actual matrix ---
%s`, got, want)
}
