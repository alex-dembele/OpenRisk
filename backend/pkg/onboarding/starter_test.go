// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only

package onboarding

import (
	"strings"
	"testing"
)

// Step 2 renders eight cards and asks for three. A set whose size depends on the
// sector is a screen that reflows, so the count is a contract, not a hope.
func TestStarterRisksFor_AlwaysReturnsExactlyEight(t *testing.T) {
	countries := []string{"CM", "SN", "FR", "BE", "MA", "CA", "US", "ZZ", "", "cm", "fr"}
	sectors := append(KnownStarterSectorKeys(), "unknown_sector", "")

	for _, sector := range sectors {
		for _, country := range countries {
			got := StarterRisksFor(sector, country)
			if len(got) != EightStarterRisks {
				t.Errorf("StarterRisksFor(%q, %q) returned %d, want %d",
					sector, country, len(got), EightStarterRisks)
			}
			seen := map[string]bool{}
			for _, r := range got {
				if seen[r.Key] {
					t.Errorf("StarterRisksFor(%q, %q) repeated %q", sector, country, r.Key)
				}
				seen[r.Key] = true
			}
		}
	}
}

// Most specific first: a banker must not have to scroll past six generic
// statements to find the one about their core banking platform.
func TestStarterRisksFor_OrdersSectorThenRegionThenGeneric(t *testing.T) {
	got := StarterRisksFor("banking", "CM")

	rank := map[StarterScope]int{ScopeSector: 0, ScopeRegion: 1, ScopeGeneric: 2}
	last := -1
	for _, r := range got {
		cur, ok := rank[r.Scope]
		if !ok {
			t.Fatalf("%q carries no scope", r.Key)
		}
		if cur < last {
			t.Errorf("%q (%s) appears after a wider scope", r.Key, r.Scope)
		}
		last = cur
	}

	if got[0].Scope != ScopeSector {
		t.Errorf("first card is %s, want the sector's own", got[0].Scope)
	}
	// CM is CEMAC, so a region statement must be present.
	var regions int
	for _, r := range got {
		if r.Scope == ScopeRegion {
			regions++
		}
	}
	if regions == 0 {
		t.Error("a CEMAC country produced no region-scoped statement")
	}
}

// The country actually changes the set — otherwise the scoping is decoration.
func TestStarterRisksFor_CountryChangesTheSet(t *testing.T) {
	cm := keysOf(StarterRisksFor("banking", "CM"))
	fr := keysOf(StarterRisksFor("banking", "FR"))
	if cm == fr {
		t.Error("CM and FR produced the same eight statements; the region scoping does nothing")
	}

	// An unknown country still works, and falls back to sector + generic.
	unknown := StarterRisksFor("banking", "ZZ")
	for _, r := range unknown {
		if r.Scope == ScopeRegion {
			t.Errorf("unknown country produced a region statement: %q", r.Key)
		}
	}
}

// Step 2 has no empty state that makes sense: "pick three of nothing" is broken.
func TestStarterRisksFor_UnknownSectorFallsBackNotEmpty(t *testing.T) {
	got := StarterRisksFor("aerospace_mining_whatever", "")
	if len(got) != EightStarterRisks {
		t.Fatalf("unknown sector returned %d statements", len(got))
	}
	other := keysOf(StarterRisksFor("other", ""))
	if keysOf(got) != other {
		t.Errorf("unknown sector did not fall back to \"other\"")
	}
}

// DoD: seeded FR + EN. These rows land in a customer's register, so a missing
// translation would write a French statement into an English tenant's data.
func TestStarterCatalogue_IsCompleteInBothLanguages(t *testing.T) {
	check := func(label string, pool []StarterRisk) {
		for _, r := range pool {
			if r.Key == "" || !strings.HasPrefix(r.Key, "starter_") {
				t.Errorf("%s: key %q must be prefixed starter_", label, r.Key)
			}
			for _, lang := range []string{"fr", "en"} {
				if strings.TrimSpace(r.TitleI18n[lang]) == "" {
					t.Errorf("%s/%s: missing %s title", label, r.Key, lang)
				}
				if strings.TrimSpace(r.DescriptionI18n[lang]) == "" {
					t.Errorf("%s/%s: missing %s description", label, r.Key, lang)
				}
			}
			if r.Probability < 0 || r.Probability > 1 {
				t.Errorf("%s/%s: probability %v outside [0,1]", label, r.Key, r.Probability)
			}
			if r.Impact < 0 || r.Impact > 10 {
				t.Errorf("%s/%s: impact %v outside [0,10]", label, r.Key, r.Impact)
			}
			if r.Category == "" {
				t.Errorf("%s/%s: no category", label, r.Key)
			}
		}
	}

	for _, k := range KnownStarterSectorKeys() {
		check("sector:"+k, starterBySector[k])
		if len(starterBySector[k]) < 3 {
			t.Errorf("sector %q has %d statements; 3 are needed to reach eight without padding-only sets",
				k, len(starterBySector[k]))
		}
	}
	for _, k := range KnownStarterRegionKeys() {
		check("region:"+k, starterByRegion[k])
	}
	check("generic", starterGeneric)

	if len(starterGeneric) < EightStarterRisks-3 {
		t.Errorf("generic pool holds %d; sector(3) + generic must reach %d",
			len(starterGeneric), EightStarterRisks)
	}
}

// Every sector offered in the wizard must have a starter set, or a user picks a
// sector in step 1 and gets somebody else's risks in step 2.
func TestStarterCatalogue_CoversEverySelectableSector(t *testing.T) {
	for _, s := range Sectors() {
		if _, ok := starterBySector[s.Key]; !ok {
			t.Errorf("sector %q is selectable but has no starter set", s.Key)
		}
	}
}

// Keys must be globally unique: StarterRiskByKey is the write path's guard
// against a client posting arbitrary text into a risk register, and a duplicate
// key would make it ambiguous.
func TestStarterRiskByKey_UniqueAndResolvable(t *testing.T) {
	seen := map[string]string{}
	var all []StarterRisk
	for _, k := range KnownStarterSectorKeys() {
		all = append(all, starterBySector[k]...)
	}
	for _, k := range KnownStarterRegionKeys() {
		all = append(all, starterByRegion[k]...)
	}
	all = append(all, starterGeneric...)

	for _, r := range all {
		if where, dup := seen[r.Key]; dup {
			t.Errorf("key %q appears twice (%s)", r.Key, where)
		}
		seen[r.Key] = r.Key

		got, ok := StarterRiskByKey(r.Key)
		if !ok {
			t.Errorf("StarterRiskByKey(%q) did not resolve", r.Key)
			continue
		}
		if got.Title("fr") != r.Title("fr") {
			t.Errorf("StarterRiskByKey(%q) returned a different statement", r.Key)
		}
	}

	if _, ok := StarterRiskByKey("starter_does_not_exist"); ok {
		t.Error("an unknown key must not resolve — that is the write path's guard")
	}
}

func TestStarterRisk_LanguageFallback(t *testing.T) {
	r := StarterRisk{
		TitleI18n:       map[string]string{"fr": "Titre", "en": "Title"},
		DescriptionI18n: map[string]string{"fr": "Corps"},
	}
	if got := r.Title("en"); got != "Title" {
		t.Errorf("Title(en) = %q", got)
	}
	if got := r.Title("de"); got != "Titre" {
		t.Errorf("Title(de) must fall back to French, got %q", got)
	}
	if got := r.Description("en"); got != "Corps" {
		t.Errorf("Description(en) must fall back to French, got %q", got)
	}
	if got := (StarterRisk{}).Title("fr"); got != "" {
		t.Errorf("an empty statement must yield an empty title, got %q", got)
	}
}

func keysOf(rs []StarterRisk) string {
	parts := make([]string, len(rs))
	for i, r := range rs {
		parts[i] = r.Key
	}
	return strings.Join(parts, ",")
}
