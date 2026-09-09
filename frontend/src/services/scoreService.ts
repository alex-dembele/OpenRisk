// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// Typed client for the ONE score endpoint.
//
// THE RULE THIS FILE ENFORCES: there is no scoring arithmetic in the frontend.
// No formula, no threshold, no band mapping — not even a "small helper". The
// server sends the value AND the band together, and this file passes them
// through untouched.
//
// That is not stylistic. Four incompatible band mappings used to ship at once
// (shared/riskColors.ts ≥7/≥4/≥2, RiskRegisterPage ≥15/≥8/≥4, the audit
// manifest's ≥15/≥9/≥5 badges, MitigationCard's ≥40), each deriving a label from
// thresholds calibrated for a different scale than the number beside it. The only
// durable fix is that the client never derives a label at all.

import { api } from '../lib/api';
import type { LocaleCode } from '../i18n/locales';
import { pickLocalized } from '../i18n/locales';

export type ScoreScope = 'tenant' | 'risk' | 'asset';

/** The four bands. Server-assigned; never computed here. */
export type ScoreBand = 'low' | 'medium' | 'high' | 'critical';

export interface ScoreFactor {
  factor: string;
  /** Normalised share of the model, summing to 1 across available factors. */
  weight: number;
  /** This factor's own measurement, 0–100, where 100 is always worst. */
  raw: number;
  /** weight × raw. The contributions sum to the value — checkable by eye. */
  contribution: number;
  label_i18n_key: string;
  /**
   * False when the source could not be consulted. The factor is excluded from
   * the score and its weight redistributed; the explainer shows "not measured"
   * rather than pretending the model has fewer dimensions than it does.
   */
  available: boolean;
}

export interface Score {
  scope: ScoreScope;
  /** The number every surface displays: the residual score, 0–100. */
  value: number;
  band: ScoreBand;
  band_label_i18n_key: string;

  /** Exposure before mitigation credit. Does not move as treatment advances. */
  inherent: number;
  inherent_band: ScoreBand;
  /** What remains after applied mitigations. */
  residual: number;
  residual_band: ScoreBand;
  mitigation_effectiveness: number;

  computed_at: string;
  formula_version: string;
  /** The measurements the calculation actually used — the explainer's assumptions. */
  inputs: Record<string, unknown>;
  breakdown: ScoreFactor[];
}

export interface ScoreBandRange {
  band: ScoreBand;
  label_i18n_key: string;
  min: number;
  max: number;
  max_inclusive: boolean;
}

export interface ScoreModel {
  formula_version: string;
  min_value: number;
  max_value: number;
  bands: ScoreBandRange[];
  scopes: {
    scope: ScoreScope;
    factors: { factor: string; weight: number; label_i18n_key: string }[];
  }[];
  input_bounds: Record<string, number[]>;
}

/** What a form sends while a slider is moving. Same model as the saved score. */
export interface ScorePreviewInput {
  scope?: ScoreScope;
  probability?: number;
  impact?: number;
  asset_criticality?: number;
  mitigation_effectiveness?: number;
}

export const scoreService = {
  async get(scope: ScoreScope, id?: string, signal?: AbortSignal): Promise<Score> {
    const { data } = await api.get<Score>('/score', { params: { scope, id }, signal });
    return data;
  },

  async preview(input: ScorePreviewInput, signal?: AbortSignal): Promise<Score> {
    const { data } = await api.post<Score>(
      '/score/preview',
      { scope: 'risk', ...input },
      { signal },
    );
    return data;
  },

  async model(): Promise<ScoreModel> {
    const { data } = await api.get<ScoreModel>('/score/model');
    return data;
  },
};

/**
 * The design token for a band.
 *
 * This is the ONLY band-keyed function left on the client, and note what it does
 * NOT do: it takes a band, never a number. There is no threshold here, so it
 * cannot disagree with the server about where a boundary lies.
 */
export function bandColor(band: ScoreBand | undefined): string {
  switch (band) {
    case 'critical':
      return 'var(--critical)';
    case 'high':
      return 'var(--high)';
    case 'medium':
      return 'var(--medium)';
    case 'low':
      return 'var(--low)';
    default:
      return 'var(--fg-muted)';
  }
}

/**
 * The band token for SMALL TEXT, as opposed to a fill.
 *
 * The low band's green (--risk-low, #1a7f3c) is a fill colour: as 12px text on
 * --surface-sunken it measures 4.29:1, just under AA. The design system already
 * carries --success-text (#15682f, 5.83:1) for text-weight green, which is what
 * this returns. The other three bands clear 4.5:1 on that surface unchanged, so
 * they are passed through — the scale itself is not redefined here.
 *
 * Use bandColor for bars, chips and fills; use this for a number or a label.
 */
export function bandTextColor(band: ScoreBand | undefined): string {
  return band === 'low' ? 'var(--success-text)' : bandColor(band);
}

/** FR/EN label for a band, keyed by the server's i18n key. */
export function bandLabel(band: ScoreBand | undefined, lang: LocaleCode): string {
  const labels: Record<ScoreBand, { fr: string; en: string }> = {
    low: { fr: 'Faible', en: 'Low' },
    medium: { fr: 'Moyen', en: 'Medium' },
    high: { fr: 'Élevé', en: 'High' },
    critical: { fr: 'Critique', en: 'Critical' },
  };
  if (!band || !(band in labels)) return lang === 'fr' ? 'Non mesuré' : 'Not measured';
  return pickLocalized(lang, labels[band]) ?? band;
}

/** FR/EN label for a factor, keyed by the server's factor key. */
export function factorLabel(factor: string, lang: LocaleCode): string {
  const labels: Record<string, { fr: string; en: string }> = {
    risk_exposure: { fr: 'Exposition aux risques', en: 'Risk exposure' },
    control_gaps: { fr: 'Écarts de conformité', en: 'Control gaps' },
    vulnerability_pressure: { fr: 'Pression des vulnérabilités', en: 'Vulnerability pressure' },
    incident_pressure: { fr: 'Pression des incidents', en: 'Incident pressure' },
    probability: { fr: 'Probabilité', en: 'Probability' },
    impact: { fr: 'Impact', en: 'Impact' },
    asset_criticality: { fr: "Criticité de l'actif", en: 'Asset criticality' },
    criticality: { fr: 'Criticité', en: 'Criticality' },
    linked_risk_exposure: { fr: 'Risques liés', en: 'Linked risks' },
    internet_exposure: { fr: 'Exposition Internet', en: 'Internet exposure' },
  };
  return pickLocalized(lang, labels[factor]) ?? factor;
}
