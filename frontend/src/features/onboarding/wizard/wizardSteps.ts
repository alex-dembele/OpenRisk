// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// The tunnel's step vocabulary, in its own module.
//
// It used to live in OnboardingWizard.tsx, which made that file export both a
// component and constants — the thing `react-refresh/only-export-components`
// exists to stop, because a Fast Refresh of the component then reloads the
// module and resets anything holding those constants. Splitting it also makes
// the dependency honest: `steps.tsx` needs the labels, not the shell.
//
// NOTE ON ORDER: the ORDER a user walks comes from the SERVER
// (`OnboardingState.steps`, with auto-skipped steps already removed — #438
// criterion 3). WIZARD_STEPS below is copy plus a fallback for the moment
// before the first response lands, and nothing else.

import type { OnboardingStepKey } from '../../../services/activationService';

/** Step labels. Copy only — never an order. */
export const WIZARD_STEP_LABELS: Record<OnboardingStepKey, { fr: string; en: string }> = {
  organization: { fr: 'Organisation', en: 'Organization' },
  profile: { fr: 'Profil', en: 'Profile' },
  goal: { fr: 'Objectif', en: 'Goal' },
  framework: { fr: 'Référentiel', en: 'Framework' },
  team: { fr: 'Équipe', en: 'Team' },
};

/**
 * The canonical order, used ONLY as the fallback while the first fetch is in
 * flight or after it fails. Rendering it in the normal path would show steps
 * this user will never reach.
 */
export const WIZARD_STEPS: { key: OnboardingStepKey; fr: string; en: string }[] = (
  ['organization', 'profile', 'goal', 'framework', 'team'] as OnboardingStepKey[]
).map((key) => ({ key, ...WIZARD_STEP_LABELS[key] }));

export function stepPath(step: OnboardingStepKey): string {
  return `/onboarding/${step}`;
}
