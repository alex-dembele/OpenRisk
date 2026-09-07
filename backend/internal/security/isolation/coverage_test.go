// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package isolation

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/opendefender/openrisk/internal/security/routes"
)

func routerPath(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, "cmd", "server", "main.go")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("module root not found")
		}
		dir = parent
	}
}

func parameterisedRoutes(t *testing.T) []routes.Route {
	t.Helper()

	all, err := routes.Extract(routerPath(t))
	if err != nil {
		t.Fatalf("extract routes: %v", err)
	}
	var out []routes.Route
	for _, r := range all {
		if r.IsParameterised() {
			out = append(out, r)
		}
	}
	if len(out) == 0 {
		t.Fatal("no parameterised routes extracted — the gate would pass vacuously")
	}
	return out
}

// TestEveryParameterisedRouteHasAnIsolationDecision is the gate.
//
// Every route that takes an ID lets a caller name a resource they may not own.
// This fails the build when such a route has no recorded decision, which is what
// stops isolation coverage from drifting behind the router — the failure mode
// that let six real cross-tenant leaks ship before the July sweep.
//
// It asserts that a decision was MADE, not that isolation works. Proving the
// latter needs the live two-tenant probe (see TestCrossTenantProbe), which
// requires a database.
func TestEveryParameterisedRouteHasAnIsolationDecision(t *testing.T) {
	var undecided []routes.Route
	for _, r := range parameterisedRoutes(t) {
		if _, ok := Lookup(r.Path); !ok {
			undecided = append(undecided, r)
		}
	}

	if len(undecided) > 0 {
		var b strings.Builder
		b.WriteString("routes with no tenant-isolation decision:\n\n")
		for _, r := range undecided {
			b.WriteString("  " + r.String() + "\n")
			b.WriteString("      registered at cmd/server/main.go:" + itoa(r.Line) + "\n")
			b.WriteString("      pattern to declare: " + Normalise(r.Path) + "\n")
		}
		b.WriteString("\nAdd an entry to decisions in internal/security/isolation/registry.go.\n")
		b.WriteString("If the route reads or writes tenant data, it needs a cross-tenant test\n")
		b.WriteString("(Covered). If it genuinely cannot leak, record why.\n")
		t.Fatal(b.String())
	}
}

// TestEveryDecisionCarriesEvidence stops the registry degrading into a list of
// bare assertions. A decision without a reason cannot be reviewed.
func TestEveryDecisionCarriesEvidence(t *testing.T) {
	for _, d := range Decisions() {
		if strings.TrimSpace(d.Evidence) == "" {
			t.Errorf("%s (%s) has no evidence", d.Pattern, d.Status)
		}
		if len(d.Evidence) < 20 {
			t.Errorf("%s: evidence %q is too terse to review", d.Pattern, d.Evidence)
		}
	}
}

// TestNoStaleDecisions catches the opposite drift: an entry for a route that no
// longer exists is misleading, and worse, it silently keeps matching a prefix.
//
// "Live" must mean every route the gates ask about, not just the parameterised
// ones. When collection routes were added to the gate (#412 criterion 9) this
// test still knew only about parameterised routes, so every new collection
// decision looked stale.
func TestNoStaleDecisions(t *testing.T) {
	live := append(parameterisedRoutes(t), collectionRoutes(t)...)

	for _, d := range Decisions() {
		var used bool
		for _, r := range live {
			if matches(d.Pattern, Normalise(r.Path)) {
				used = true
				break
			}
		}
		if !used {
			t.Errorf("decision %q matches no live route — remove it or fix the pattern", d.Pattern)
		}
	}
}

// TestPendingGapsAreVisible does not fail on pending entries; it reports them.
// Pending is deliberate, acknowledged debt, and the report is what keeps it from
// being forgotten. Turning it into a failure now would only invite someone to
// relabel gaps as covered to get a green build.
func TestPendingGapsAreVisible(t *testing.T) {
	counts := map[Status]int{}
	var pending []string

	for _, r := range parameterisedRoutes(t) {
		d, ok := Lookup(r.Path)
		if !ok {
			continue // already reported by the gate above
		}
		counts[d.Status]++
		if d.Status == Pending {
			pending = append(pending, r.String())
		}
	}

	sort.Strings(pending)
	t.Logf("isolation decisions across %d parameterised routes:", len(parameterisedRoutes(t)))
	for _, s := range []Status{Covered, Pending, PublicByDesign, MachineAuthenticated, SelfScoped, Unreachable} {
		t.Logf("  %-22s %d", s, counts[s])
	}
	if len(pending) > 0 {
		t.Logf("\n%d routes still need a cross-tenant test:", len(pending))
		for _, p := range pending {
			t.Logf("  %s", p)
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// collectionRoutes returns every read with no id in its path.
func collectionRoutes(t *testing.T) []routes.Route {
	t.Helper()

	all, err := routes.Extract(routerPath(t))
	if err != nil {
		t.Fatalf("extract routes: %v", err)
	}
	var out []routes.Route
	for _, r := range all {
		if r.IsCollection() {
			out = append(out, r)
		}
	}
	if len(out) == 0 {
		t.Fatal("no collection routes extracted — the gate would pass vacuously")
	}
	return out
}

// TestIsolationGate_DemandsADecisionForCollectionRoutes closes the blind spot
// that let an all-tenants read ship.
//
// The parameterised gate above asks "did anyone check that this id belongs to
// the caller?". It cannot ask anything about GET /timeline, because there is no
// id to check — and that is precisely the route shape that leaked. From
// docs/JOURNAL.md item 36 (2026-07-23): GET /timeline/recent returned the risk
// history of EVERY tenant in the deployment, because RiskHistory carries no
// tenant_id and the handler never read the tenant context. No path parameter,
// so the gate never asked; no leak found by CI; found by a human reading code.
//
// A missing predicate on a collection route is worse than a missing check on a
// parameterised one. The parameterised failure leaks one row that the attacker
// had to name. This one leaks every row of every tenant to whoever loads the
// page.
//
// Like the parameterised gate, this asserts that a decision was RECORDED, not
// that isolation works. Pending is a legitimate answer; silence is not.
func TestIsolationGate_DemandsADecisionForCollectionRoutes(t *testing.T) {
	var undecided []routes.Route
	for _, r := range collectionRoutes(t) {
		if _, ok := Lookup(r.Path); !ok {
			undecided = append(undecided, r)
		}
	}

	if len(undecided) > 0 {
		var b strings.Builder
		b.WriteString("collection routes with no tenant-isolation decision:\n\n")
		for _, r := range undecided {
			b.WriteString("  " + r.String() + "\n")
			b.WriteString("      registered at cmd/server/main.go:" + itoa(r.Line) + "\n")
			b.WriteString("      pattern to declare: " + Normalise(r.Path) + "\n")
		}
		b.WriteString("\nAdd an entry to decisions in internal/security/isolation/registry.go.\n")
		b.WriteString("A read with no id in its path returns MANY rows. If it returns tenant\n")
		b.WriteString("data, it needs a WHERE tenant_id and a cross-tenant test (Covered).\n")
		b.WriteString("If it is genuinely public or self-scoped, record which and why.\n")
		t.Fatal(b.String())
	}
}

// The two routes W1-02 adds are named explicitly, because this issue builds the
// successor to the route that leaked and a prefix pattern elsewhere must never
// be what silently accounts for them.
//
// Their correct statuses differ, and that difference is the point:
//
//   - GET /timeline returns tenant ROWS. Nothing but a cross-tenant test makes
//     it safe, so it must be Covered.
//   - GET /entities returns the static type catalogue plus the caller's own
//     permission flags (Service.Catalogue) — no tenant rows at all. Marking it
//     Covered would claim a cross-tenant assertion that has nothing to assert.
//     It must be decided and evidenced, but SelfScoped is the honest status.
func TestIsolationGate_TimelineAndCatalogueAreDecidedByName(t *testing.T) {
	for _, path := range []string{"/api/v1/timeline", "/api/v1/entities"} {
		d, ok := Lookup(path)
		if !ok {
			t.Fatalf("%s has no isolation decision", path)
		}
		if d.Pattern != path {
			t.Errorf("%s is accounted for by the broader pattern %q; it must carry its own "+
				"entry — this is the successor to the route that returned every tenant's "+
				"history, and it may not be covered by inheritance", path, d.Pattern)
		}
		if d.Evidence == "" {
			t.Errorf("%s records no evidence", path)
		}
		if d.Status == Pending {
			t.Errorf("%s is still Pending; W1-02 ships these two routes and may not "+
				"leave its own surface as debt", path)
		}
	}

	if d, _ := Lookup("/api/v1/timeline"); d.Status != Covered {
		t.Errorf("GET /timeline is %q, want %q — it returns tenant rows, and it is the "+
			"successor to the route that returned every tenant's history", d.Status, Covered)
	}
}

// The route that actually leaked is retired, and must stay retired.
//
// GET /timeline/recent returned the risk history of every tenant in the
// deployment (docs/JOURNAL.md item 36, 2026-07-23). Even after that was fixed it
// still read its tenant via safeGetUUID, which falls back to uuid.Nil rather
// than failing closed — the shape the constitution forbids, on a route that had
// already leaked once. #412 criterion 16 retired it; GET /timeline supersedes it.
//
// This test fails if anyone re-mounts it. Re-adding the route is not forbidden
// outright, but it may not happen silently: whoever does it has to delete this
// test and say why in the diff.
func TestIsolationGate_LegacyRecentTimelineIsRetired(t *testing.T) {
	all, err := routes.Extract(routerPath(t))
	if err != nil {
		t.Fatalf("extract routes: %v", err)
	}
	for _, r := range all {
		if Normalise(r.Path) == "/api/v1/timeline/recent" {
			t.Fatalf("%s is mounted again at cmd/server/main.go:%d. It returned every "+
				"tenant's risk history once and read its tenant through a uuid.Nil "+
				"fallback. GET /timeline replaces it.", r.String(), r.Line)
		}
	}
}

// genericEvidence is the string #412 stamped on every collection route when it
// made them visible: a placeholder meaning "nobody has looked at this yet".
//
// #421 is the looking. This constant exists so the placeholder cannot come back
// — not for the 97 routes it was written for, and not for the next module that
// arrives in a hurry.
const genericEvidence = "not assessed — collection route made visible by #412 criterion 9"

// TestNoRouteCarriesTheGenericEvidenceString is #421 criterion 1.
//
// The registry's own rule is that a decision without a reason is
// indistinguishable from a guess. TestEveryDecisionCarriesEvidence enforces that
// SOME reason is present; this enforces that the reason is about this route.
// A shared placeholder passes the first test and fails the purpose of both.
//
// Pending is still a legitimate status. What it may not be is anonymous: a
// pending entry has to say what is unresolved and what would settle it, so the
// next person can act on it without re-deriving the audit.
func TestNoRouteCarriesTheGenericEvidenceString(t *testing.T) {
	for _, d := range Decisions() {
		if strings.Contains(d.Evidence, genericEvidence) {
			t.Errorf("%s still carries the generic #412 placeholder. State what is "+
				"specifically unresolved about THIS route and what would settle it.",
				d.Pattern)
		}
	}
}

// TestCollectionGapsAreVisible is the counterpart to TestPendingGapsAreVisible,
// for the surface that gate covers. Like it, it reports rather than fails:
// turning acknowledged debt into a red build invites relabelling gaps as covered
// to get green, which is the one outcome this registry exists to prevent.
//
// The count it logs is the honest residual — #421 criterion 6. Read it as the
// number of collection routes whose isolation is reasoned about but not pinned
// by a test, not as a number of known leaks.
func TestCollectionGapsAreVisible(t *testing.T) {
	counts := map[Status]int{}
	var pending []string

	all := collectionRoutes(t)
	for _, r := range all {
		d, ok := Lookup(r.Path)
		if !ok {
			continue // already reported by the collection gate
		}
		counts[d.Status]++
		if d.Status == Pending {
			pending = append(pending, r.String())
		}
	}

	sort.Strings(pending)
	t.Logf("isolation decisions across %d collection routes:", len(all))
	for _, s := range []Status{Covered, Pending, PublicByDesign, MachineAuthenticated, SelfScoped, Unreachable} {
		t.Logf("  %-22s %d", s, counts[s])
	}
	if len(pending) > 0 {
		t.Logf("\n%d collection routes remain Pending — each states its own specific "+
			"remainder in registry.go:", len(pending))
		for _, p := range pending {
			t.Logf("  %s", p)
		}
	}
}

// TestLookup_ExactPatternBeatsAWildcardOverTheSamePath pins the tie-break.
//
// A wildcard matches its own bare prefix, and it is the longer string, so the
// original length-only comparison gave GET /api/v1/risks to /api/v1/risks/* and
// discarded the exact entry written for it. The route was assessed, carried its
// own decision, and still reported as unassessed — the failure mode this whole
// registry exists to make impossible.
//
// This is asserted on the live registry rather than on a fixture, because the
// value of the rule is that it holds for the entries actually recorded.
func TestLookup_ExactPatternBeatsAWildcardOverTheSamePath(t *testing.T) {
	for _, d := range Decisions() {
		if strings.HasSuffix(d.Pattern, "/*") {
			continue
		}
		got, ok := Lookup(d.Pattern)
		if !ok {
			t.Errorf("%s matches no decision, including its own", d.Pattern)
			continue
		}
		if got.Pattern != d.Pattern {
			t.Errorf("Lookup(%q) resolved to %q — a module-wide pattern is answering "+
				"for a route that carries its own entry, so that entry is dead text",
				d.Pattern, got.Pattern)
		}
	}
}
