// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only

package domain

import "testing"

// The invariant that fixes the "two items struck through after a single import"
// bug: a step maps to EXACTLY ONE event key, and no key is shared. If someone
// adds a step later and reuses an event key, this fails before it ships.
func TestActivationSteps_OneEventKeyPerStep(t *testing.T) {
	if err := ValidateActivationSteps(); err != nil {
		t.Fatalf("activation step catalog is invalid: %v", err)
	}

	steps := ActivationSteps()
	if len(steps) == 0 {
		t.Fatal("no activation steps defined")
	}

	byEvent := map[ActivationEventKey][]string{}
	for _, s := range steps {
		byEvent[s.EventKey] = append(byEvent[s.EventKey], s.Key)
	}
	for event, owners := range byEvent {
		if len(owners) != 1 {
			t.Errorf("event %q ticks %d steps (%v); it must tick exactly one", event, len(owners), owners)
		}
	}
}

// aha.reached and signup are outcomes/anchors, not checklist steps — binding a
// step to them would put an un-actionable row in the panel.
func TestActivationSteps_ExcludeNonStepEvents(t *testing.T) {
	for _, s := range ActivationSteps() {
		if s.EventKey == ActivationAhaReached || s.EventKey == ActivationSignup {
			t.Errorf("step %q must not be bound to the non-step event %q", s.Key, s.EventKey)
		}
	}
}

// D-011: the sibling assertion to ValidateActivationSteps. It answers the
// opposite question — not "does every step own one key?" but "has a key that is
// never a chore leaked into the catalogue?".
func TestValidateNonChecklistEventKeys(t *testing.T) {
	if err := ValidateNonChecklistEventKeys(); err != nil {
		t.Fatalf("catalogue holds a non-checklist event key: %v", err)
	}

	// posture.revealed is the key #438 adds; the wire value is what the reveal
	// and the E2E suite match on, so it is asserted literally.
	if ActivationPostureRevealed != "posture.revealed" {
		t.Errorf("posture event key = %q, want %q", ActivationPostureRevealed, "posture.revealed")
	}

	keys := NonChecklistEventKeys()
	want := map[ActivationEventKey]bool{
		ActivationSignup:          false,
		ActivationAhaReached:      false,
		ActivationPostureRevealed: false,
	}
	for _, k := range keys {
		if _, known := want[k]; !known {
			t.Errorf("unexpected non-checklist key %q", k)
			continue
		}
		want[k] = true
	}
	for k, seen := range want {
		if !seen {
			t.Errorf("%q must be declared non-checklist", k)
		}
	}
}

// The failure the assertion exists to catch: a later edit adds a checklist row
// bound to a server-observed outcome. ValidateActivationSteps stays happy — the
// bijection still holds — and the panel shows a row nobody can act on.
func TestValidateNonChecklistEventKeys_CatchesALeakedKey(t *testing.T) {
	original := activationSteps
	t.Cleanup(func() { activationSteps = original })

	activationSteps = append(append([]ActivationStepDef{}, original...), ActivationStepDef{
		Key:       "posture",
		EventKey:  ActivationPostureRevealed,
		LabelI18n: map[string]string{"fr": "Voyez votre posture", "en": "See your posture"},
		DeepLink:  "/posture",
		Order:     len(original) + 1,
	})

	if err := ValidateActivationSteps(); err != nil {
		t.Fatalf("precondition: the bijection must still hold, got %v", err)
	}
	if err := ValidateNonChecklistEventKeys(); err == nil {
		t.Error("a checklist step bound to posture.revealed must be rejected")
	}
}

// ActivationSteps hands out a copy: a caller mutating the result must not be able
// to corrupt the catalog for everyone else.
func TestActivationSteps_ReturnsCopy(t *testing.T) {
	first := ActivationSteps()
	first[0].Key = "mutated"
	if ActivationSteps()[0].Key == "mutated" {
		t.Error("ActivationSteps leaked the underlying slice")
	}
}

func TestParseOnboardingStep(t *testing.T) {
	for _, want := range OnboardingStepOrder {
		got, err := ParseOnboardingStep(string(want))
		if err != nil || got != want {
			t.Errorf("ParseOnboardingStep(%q) = (%q, %v)", want, got, err)
		}
	}
	if _, err := ParseOnboardingStep("dashboard"); err == nil {
		t.Error("an unknown step must be rejected")
	}
}

func TestOnboardingStep_Index(t *testing.T) {
	if OnboardingStepOrganization.Index() != 0 {
		t.Error("organization must be the first step")
	}
	// #438 re-sequenced the tunnel: the last step is `cover`, the one that shows
	// the residual falling. Ending on an invitation form was the old ordering's
	// mistake — it asked the user to invite colleagues to look at nothing.
	if OnboardingStepCover.Index() != len(OnboardingStepOrder)-1 {
		t.Errorf("cover must be the last step, got index %d of %d",
			OnboardingStepCover.Index(), len(OnboardingStepOrder))
	}
	if len(OnboardingStepOrder) != 5 {
		t.Errorf("the tunnel has %d steps; #438 keeps the count at five so the stepper still reads \"N sur 5\"",
			len(OnboardingStepOrder))
	}

	// The retired routes must not be walkable. Their constants survive so stored
	// answers stay readable, but no client may resurrect the route.
	for _, retired := range []OnboardingStepKey{OnboardingStepProfile, OnboardingStepTeam} {
		if _, err := ParseOnboardingStep(string(retired)); err == nil {
			t.Errorf("the retired step %q must be rejected by ParseOnboardingStep", retired)
		}
		if retired.Index() != -1 {
			t.Errorf("%q is retired but still has index %d", retired, retired.Index())
		}
	}
}

// Answers round-trip per step, and reading an absent step is safe (a resumed
// wizard reads steps the user has not reached yet on every render).
func TestOnboardingProgress_StepAnswers(t *testing.T) {
	var p OnboardingProgress
	if got := p.StepAnswers(OnboardingStepProfile); len(got) != 0 {
		t.Errorf("empty progress should yield no answers, got %v", got)
	}

	p.SetStepAnswers(OnboardingStepProfile, JSONMap{"full_name": "Awa", "language": "fr"})
	got := p.StepAnswers(OnboardingStepProfile)
	if got["full_name"] != "Awa" || got["language"] != "fr" {
		t.Errorf("answers did not round-trip: %v", got)
	}
	if len(p.StepAnswers(OnboardingStepGoal)) != 0 {
		t.Error("an unset step must read back empty, not panic")
	}

	// A second write replaces that step only.
	p.SetStepAnswers(OnboardingStepGoal, JSONMap{"goal": "pass_audit"})
	if p.StepAnswers(OnboardingStepProfile)["full_name"] != "Awa" {
		t.Error("writing one step clobbered another")
	}
}
