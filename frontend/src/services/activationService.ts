// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// Typed client for the activation & onboarding endpoints.
//
// THE RULE THIS FILE ENFORCES: activation state lives on the server. There is no
// localStorage here, and there is no client-side derivation of "is this step
// done?" — the previous checklist did both, which is why it never ticked
// reliably, why confetti fired at random, and why one import struck two rows
// through. The client asks; the server answers.

import { api } from '../lib/api';
import type { LocaleCode } from '../i18n/locales';

/** Language keys the server ships copy in. */
export type Lang = LocaleCode;

export interface ActivationStep {
  key: string;
  /** The ONE server event that ticks this step. Exposed for transparency/debug. */
  event_key: string;
  label_i18n: Record<string, string>;
  hint_i18n: Record<string, string>;
  completed: boolean;
  /** ISO date, or null. Set by the FIRST occurrence of the event; never moves. */
  completed_at: string | null;
  deep_link: string;
  order: number;
  /** The step that is the product's promise (the first risk). */
  primary: boolean;
  /**
   * The server's instruction to celebrate: true only when the step is completed
   * AND this user has not acknowledged it yet. The client must never decide this
   * on its own — that was the random-confetti bug.
   */
  celebrate: boolean;
}

export interface ActivationState {
  steps: ActivationStep[];
  percent: number;
  aha_reached_at: string | null;
  signup_at?: string | null;
  time_to_aha_seconds?: number | null;
}

export type OnboardingStepKey = 'organization' | 'profile' | 'goal' | 'framework' | 'team';

export interface OnboardingState {
  current_step: OnboardingStepKey;
  steps: OnboardingStepKey[];
  step_index: number;
  completed: boolean;
  completed_at?: string | null;
  percent: number;
  industry?: string;
  country?: string;
  goal?: string;
  /** Raw per-step answers, so a resumed wizard repopulates exactly as left. */
  answers: Record<string, Record<string, unknown>>;
  /** Where to land after the wizard, derived from the chosen goal. */
  landing: string;
}

export interface SectorOption {
  key: string;
  label_i18n: Record<string, string>;
}

export interface GoalOption {
  key: string;
  label_i18n: Record<string, string>;
  frameworks?: string[];
  landing: string;
}

/**
 * A pre-filled first-risk draft. Probability is [0,1] and impact [0,10] — the
 * scales the Score Engine consumes, so what the form shows is what gets scored.
 */
export interface RiskSuggestion {
  key: string;
  title: string;
  description: string;
  probability: number;
  impact: number;
  category: string;
  suggested_asset?: string;
  suggested_tags?: string[];
}

export interface OnboardingSuggestions {
  sectors: SectorOption[];
  goals: GoalOption[];
  risks: RiskSuggestion[];
  /** Catalog keys, most relevant first. */
  frameworks: string[];
  industry?: string;
  country?: string;
  goal?: string;
}

export const activationService = {
  /** The checklist, exactly as the server computes it. */
  async getState(): Promise<ActivationState> {
    const { data } = await api.get<ActivationState>('/activation/state');
    return data;
  },

  /**
   * Acknowledge that a step's celebration was shown to this user. Idempotent
   * server-side, so a double call is harmless.
   */
  async markCelebrated(stepKey: string): Promise<void> {
    await api.post('/activation/celebrated', { step_key: stepKey });
  },

  async getOnboardingState(): Promise<OnboardingState> {
    const { data } = await api.get<OnboardingState>('/onboarding/state');
    return data;
  },

  /**
   * Save one wizard step. `next` may point BACKWARDS — going back to correct an
   * answer is a supported move.
   */
  async saveStep(
    step: OnboardingStepKey,
    answers: Record<string, unknown>,
    next?: OnboardingStepKey,
  ): Promise<OnboardingState> {
    const { data } = await api.put<OnboardingState>(`/onboarding/steps/${step}`, { answers, next });
    return data;
  },

  async complete(): Promise<OnboardingState> {
    const { data } = await api.post<OnboardingState>('/onboarding/complete', {});
    return data;
  },

  /**
   * Sector/goal-driven content. The optional overrides let the wizard preview a
   * choice before it is saved.
   */
  async getSuggestions(params?: {
    industry?: string;
    country?: string;
    goal?: string;
  }): Promise<OnboardingSuggestions> {
    const { data } = await api.get<OnboardingSuggestions>('/onboarding/suggestions', { params });
    return data;
  },
};

/** Pick the copy for the active language, falling back to French then to any. */
export function i18n(map: Record<string, string> | undefined, lang: Lang): string {
  if (!map) return '';
  return map[lang] ?? map.fr ?? map.en ?? Object.values(map)[0] ?? '';
}
