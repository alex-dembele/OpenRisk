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

/**
 * The five tunnel routes (#438).
 *
 * `profile` and `team` are RETIRED: the profile question merged into the
 * organization step, and team invitations moved to the Posture Reveal, where the
 * issue's design-system note puts them. They are not in this union, so the
 * compiler stops any code from routing to a screen that no longer exists.
 */
export type OnboardingStepKey = 'organization' | 'goal' | 'framework' | 'score' | 'cover';

export interface OnboardingState {
  current_step: OnboardingStepKey;
  /**
   * What the stepper renders: the steps THIS USER will actually see, with
   * auto-skipped ones already removed server-side (#438 criterion 3). Render
   * from this array and nothing else.
   */
  steps: OnboardingStepKey[];
  /**
   * What was removed. Diagnostic only — drawing these would flash a step
   * criterion 3 forbids.
   */
  skipped_steps: OnboardingStepKey[];
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

/* ---------------------------------------------------------------------------
 * Posture Reveal (#438)
 *
 * Every field below is computed server-side from the tenant's OWN rows. There is
 * deliberately no client-side fallback, no default and no placeholder anywhere in
 * this block: criterion 7 forbids a sample value reaching the DOM, and a
 * `?? 0` here would be exactly that.
 * ------------------------------------------------------------------------- */

/** Coverage of one risk by its mapped controls (ADR 0003). */
export interface ResidualCoverage {
  /** False when no APPLICABLE control is mapped — the residual then equals the
   *  inherent score. An absent signal must never render as a good one. */
  measured: boolean;
  ratio: number;
  effectiveness: number;
  applicable: number;
  total: number;
}

/** One residual, on the Score Engine's own scale and bands. */
export interface Residual {
  inherent: number;
  value: number;
  level: 'low' | 'medium' | 'high' | 'critical';
  reduction: number;
  coverage: ResidualCoverage;
  formula_version: string;
}

export interface PostureRiskView {
  id: string;
  title: string;
  inherent: number;
  level: string;
  residual: Residual;
}

export interface PostureSummary {
  risks: { total: number; by_level: Record<string, number> };
  controls: {
    frameworks: number;
    total: number;
    implemented: number;
    not_applicable: number;
    in_progress: number;
  };
  /** null, NOT zero, when nothing is applicable. Zero would say "you have
   *  covered nothing", which is a different and false statement. */
  coverage_percent: number | null;
  top_risks: PostureRiskView[];
  residual_formula_version: string;
  generated_at: string;
  revealed_at?: string | null;
  /** True only on the render that recorded the event, so the client celebrates
   *  once without deciding anything itself. */
  first_reveal: boolean;
}

export interface RecognitionCounts {
  risks: number;
  frameworks: number;
  controls: number;
  assets: number;
  members: number;
}

export interface Recognition {
  counts: RecognitionCounts;
  /** The SERVER decides. A client that could choose would be a client that can
   *  skip the tunnel. */
  recognised: boolean;
  skip_tunnel: boolean;
}

/** One statement from the starter catalogue (step 2 renders eight). */
export interface StarterRisk {
  key: string;
  title_i18n: Record<string, string>;
  description_i18n: Record<string, string>;
  probability: number;
  impact: number;
  category: string;
  tags?: string[];
  scope: 'sector' | 'region' | 'generic';
}

export interface StarterRiskOffer {
  risks: StarterRisk[];
  /** How many the user must pick. Sent by the server so the two can never
   *  disagree about what "select three" means. */
  pick: number;
  industry?: string;
  country?: string;
  /** True when this tenant already has starter rows: the screen then shows the
   *  selection as done instead of inviting an adoption the server would refuse. */
  already_adopted: boolean;
}

export interface AdoptStarterRisksResult {
  created: string[];
  keys: string[];
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

  /**
   * The Posture Reveal.
   *
   * A 404 here is NOT a missing page: it is criterion 8's explicit refusal —
   * the tenant has nothing to reveal, and the server recorded nothing and
   * measured nothing rather than reporting a zeroed success. The caller must
   * render an error state, never an empty posture.
   */
  async getPosture(): Promise<PostureSummary> {
    const { data } = await api.get<PostureSummary>('/posture');
    return data;
  },

  /** What the tenant already holds, for the population #234 backfilled. */
  async getRecognition(): Promise<Recognition> {
    const { data } = await api.get<Recognition>('/onboarding/recognition');
    return data;
  },

  /** The eight statements step 2 renders, scoped to the stored sector/country. */
  async getStarterRisks(): Promise<StarterRiskOffer> {
    const { data } = await api.get<StarterRiskOffer>('/onboarding/starter-risks');
    return data;
  },

  /**
   * Adopt the three chosen statements.
   *
   * KEYS ONLY. There is deliberately no way to send a title or a description
   * from here: the server re-reads the statement from its own catalogue, because
   * a client that could post free text would be an unvalidated write into a
   * customer's risk register.
   *
   * A 409 means this tenant already adopted — the tunnel is resumable, so that
   * is an expected answer and not a failure to retry.
   *
   * `lang` decides the language the rows are WRITTEN in. Getting it wrong writes
   * statements a customer's colleagues may not read into their own register.
   */
  async adoptStarterRisks(keys: string[], lang: Lang): Promise<AdoptStarterRisksResult> {
    const { data } = await api.post<AdoptStarterRisksResult>('/onboarding/starter-risks', {
      keys,
      // Which of the two SERVER-AUTHORED strings to store. A preference, not
      // content: it cannot introduce text of our own, and the server falls back
      // to French on anything it does not recognise. Sent because the client is
      // the only party that knows which language the person is reading.
      lang,
    });
    return data;
  },
};

/** Pick the copy for the active language, falling back to French then to any. */
export function i18n(map: Record<string, string> | undefined, lang: Lang): string {
  if (!map) return '';
  return map[lang] ?? map.fr ?? map.en ?? Object.values(map)[0] ?? '';
}
