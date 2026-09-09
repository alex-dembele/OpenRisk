// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// Shared labels and colours for audits and remediation plans.
//
// These tables were declared inline in AuditsPage and RemediationPage. Adding
// detail routes would have made a third and fourth copy, and a list showing
// "En cours" beside a detail showing "In progress" is the kind of drift nobody
// notices until a screenshot goes to a regulator.

import { localeTag } from '../../i18n/locales';
import type { AuditStatus, RemediationPriority, RemediationStatus } from '../../types/compliance';
import type { LocaleCode } from '../../i18n/locales';

export interface Meta {
  color: string;
  fr: string;
  en: string;
}

export const AUDIT_STATUS_META: Record<AuditStatus, Meta> = {
  planned: { color: 'var(--fg-muted)', fr: 'Planifié', en: 'Planned' },
  in_progress: { color: 'var(--high)', fr: 'En cours', en: 'In progress' },
  completed: { color: 'var(--low)', fr: 'Terminé', en: 'Completed' },
  cancelled: { color: 'var(--critical)', fr: 'Annulé', en: 'Cancelled' },
};

export const AUDIT_TYPE_LABEL: Record<string, { fr: string; en: string }> = {
  internal: { fr: 'Interne', en: 'Internal' },
  external: { fr: 'Externe', en: 'External' },
  certification: { fr: 'Certification', en: 'Certification' },
  surveillance: { fr: 'Surveillance', en: 'Surveillance' },
};

export const AUDIT_STATUS_ORDER: AuditStatus[] = [
  'planned',
  'in_progress',
  'completed',
  'cancelled',
];

export const REMEDIATION_STATUS_META: Record<RemediationStatus, Meta> = {
  open: { color: 'var(--critical)', fr: 'Ouvert', en: 'Open' },
  in_progress: { color: 'var(--high)', fr: 'En cours', en: 'In progress' },
  completed: { color: 'var(--low)', fr: 'Terminé', en: 'Completed' },
  cancelled: { color: 'var(--fg-muted)', fr: 'Annulé', en: 'Cancelled' },
};

export const REMEDIATION_PRIORITY_META: Record<RemediationPriority, Meta> = {
  low: { color: 'var(--low)', fr: 'Basse', en: 'Low' },
  medium: { color: 'var(--medium)', fr: 'Moyenne', en: 'Medium' },
  high: { color: 'var(--high)', fr: 'Haute', en: 'High' },
  critical: { color: 'var(--critical)', fr: 'Critique', en: 'Critical' },
};

export const REMEDIATION_STATUS_ORDER: RemediationStatus[] = [
  'open',
  'in_progress',
  'completed',
  'cancelled',
];

/** Formats an ISO date for display, or an em dash when absent. */
export function formatDate(iso: string | null | undefined, lang: LocaleCode): string {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '—';
  return d.toLocaleDateString(localeTag(lang), {
    day: '2-digit',
    month: 'short',
    year: 'numeric',
  });
}
