// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// Maps the backend Risk shape onto the display shape the dc.html Risk Register /
// drawer render. Normalizes the two live status vocabularies (DRAFT/ACTIVE/… and
// open/in_progress/…) and the criticality/level fields into the design's tokens.

import { localeTag } from '../../i18n/locales';
import type { Risk, RiskPhase } from '../../hooks/useRiskStore';
import type { RiskControlMapping } from '../../services/taxonomyService';
import { mappingHref, mappingLabel } from '../../services/taxonomyService';
import type { Criticality } from '../../shared/riskColors';
import type { RiskStatus } from '../../shared/ui';
import type { LocaleCode } from '../../i18n/locales';

export interface UiRisk {
  id: string;
  raw: Risk;
  name: string;
  crit: Criticality;
  score: number;
  prob: number;
  impact: number;
  ac: number;
  asset: string;
  /** Free-text labels → colonne « Étiquettes ». */
  tags: string[];
  /** The controlled vocabulary → colonne « Catégorie ». Empty when unclassified. */
  categoryName: string;
  categoryColor: string;
  /** Real compliance references → colonne « Référentiel ». Empty when unmapped. */
  mappings: RiskControlMapping[];
  fw: string;
  fwHref: string;
  status: RiskStatus;
  phase: RiskPhase;
  owner: string;
  ownerName: string;
  mod: string;
  desc: string;
}

const CRITS = new Set<Criticality>(['critical', 'high', 'medium', 'low']);

function toCrit(r: Risk): Criticality {
  const c = ((r as { criticality?: string }).criticality ?? r.level ?? '').toString().toLowerCase();
  if (CRITS.has(c as Criticality)) return c as Criticality;
  // No client-side fallback from the number. Deriving a band here is exactly how
  // the label came to disagree with the value it sat next to; a server that did
  // not send a criticality has not said "low", so we do not say it either.
  return 'low';
}

/** Both live vocabularies → the design's 4-state pill. */
export function toRiskStatus(s?: string): RiskStatus {
  const v = (s ?? '').toString().toLowerCase();
  if (v.includes('progress') || v === 'active') return 'progress';
  if (v.includes('mitigat')) return 'mitigated';
  if (v.includes('accept')) return 'accepted';
  if (v === 'closed' || v === 'done') return 'mitigated';
  return 'open';
}

export function initialsOf(name?: string, fallback = '—'): string {
  if (!name?.trim()) return fallback;
  const parts = name.trim().split(/\s+/);
  const s = ((parts[0]?.[0] ?? '') + (parts[1]?.[0] ?? '')).toUpperCase();
  return s || fallback;
}

/** Compact locale-aware relative time without pulling a heavy dep. */
export function relTime(iso?: string, lang: LocaleCode = 'fr'): string {
  if (!iso) return '—';
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) return '—';
  const s = Math.max(0, Math.floor((Date.now() - then) / 1000));
  const fr = lang === 'fr';
  if (s < 60) return fr ? "à l'instant" : 'just now';
  const m = Math.floor(s / 60);
  if (m < 60) return fr ? `il y a ${m} min` : `${m} min ago`;
  const h = Math.floor(m / 60);
  if (h < 24) return fr ? `il y a ${h} h` : `${h} h ago`;
  const d = Math.floor(h / 24);
  if (d === 1) return fr ? 'hier' : 'yesterday';
  if (d < 7) return fr ? `il y a ${d} j` : `${d} d ago`;
  const w = Math.floor(d / 7);
  if (w < 5) return fr ? `il y a ${w} sem.` : `${w} w ago`;
  return new Date(iso).toLocaleDateString(localeTag(lang));
}

export function mapRisk(r: Risk, lang: LocaleCode): UiRisk {
  const rr = r as Risk & { name?: string; owner?: string; asset_id?: string; updated_at?: string };
  // The owner is now a real user id resolved server-side; the legacy free-text
  // fields are only a fallback for rows written before migration 0044.
  const ownerName = r.owner_email || r.assignee_email || rr.owner || r.assigned_to || '';
  const mappings = r.control_mappings ?? [];
  const primary = mappings[0];
  return {
    id: r.id,
    raw: r,
    name: rr.name || r.title || '—',
    crit: toCrit(r),
    score: r.score ?? 0,
    prob: r.probability ?? 0,
    impact: r.impact ?? 0,
    ac: (r as { asset_criticality?: number }).asset_criticality ?? 1,
    asset: r.assets?.[0]?.name ?? '—',
    tags: r.tags ?? [],
    categoryName: r.category?.name ?? '',
    categoryColor: r.category?.color ?? 'neutral',
    mappings,
    // The "Référentiel" column renders a REAL reference or nothing at all.
    //
    // It used to read `frameworks[0] ?? tags[0]`, so a user's free-text label was
    // rendered with a framework badge whenever the (also free-text) frameworks
    // array was empty. Both halves of that were wrong: a tag is not a framework,
    // and neither was the string in `frameworks`, which came from a hard-coded
    // dropdown that never consulted the tenant's imported frameworks. An empty
    // cell says "not mapped", which is true; the old fallback said "ISO 27001"
    // on the strength of a label somebody typed.
    fw: primary ? mappingLabel(primary) : '—',
    fwHref: primary ? mappingHref(primary) : '',
    status: toRiskStatus(r.status),
    phase: r.lifecycle_phase ?? 'identified',
    owner: initialsOf(ownerName),
    ownerName: ownerName || '—',
    mod: relTime(rr.updated_at ?? r.created_at, lang),
    desc: r.description || '—',
  };
}
