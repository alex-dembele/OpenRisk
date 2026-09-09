// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// Display metadata (colors + FR/EN labels) shared by the register, drawer and
// ingest modal. Kept in one place so the vocabulary stays consistent.

import type { VulnSeverity, VulnStatus, VulnSource } from './vulnerabilityService';
import type { LocaleCode } from '../../i18n/locales';

export const SEVERITY_META: Record<VulnSeverity, { label: [string, string]; color: string }> = {
  critical: { label: ['Critique', 'Critical'], color: 'var(--critical)' },
  high: { label: ['Élevé', 'High'], color: 'var(--high)' },
  medium: { label: ['Moyen', 'Medium'], color: 'var(--medium)' },
  low: { label: ['Faible', 'Low'], color: 'var(--low)' },
  info: { label: ['Info', 'Info'], color: 'var(--info)' },
};

export const STATUS_META: Record<VulnStatus, { label: [string, string]; color: string }> = {
  open: { label: ['Ouverte', 'Open'], color: 'var(--critical)' },
  triaged: { label: ['Triée', 'Triaged'], color: 'var(--high)' },
  in_remediation: { label: ['En correction', 'In remediation'], color: 'var(--info)' },
  remediated: { label: ['Corrigée', 'Remediated'], color: 'var(--low)' },
  accepted: { label: ['Acceptée', 'Accepted'], color: 'var(--fg-secondary)' },
  false_positive: { label: ['Faux positif', 'False positive'], color: 'var(--fg-muted)' },
};

export const STATUS_ORDER: VulnStatus[] = [
  'open',
  'triaged',
  'in_remediation',
  'remediated',
  'accepted',
  'false_positive',
];

export const TIER_META: Record<string, { color: string; label: [string, string] }> = {
  P1: { color: 'var(--critical)', label: ['P1 · Urgent', 'P1 · Urgent'] },
  P2: { color: 'var(--high)', label: ['P2 · Élevé', 'P2 · High'] },
  P3: { color: 'var(--medium)', label: ['P3 · Moyen', 'P3 · Medium'] },
  P4: { color: 'var(--low)', label: ['P4 · Faible', 'P4 · Low'] },
};

export const SOURCE_LABEL: Record<VulnSource, string> = {
  nessus: 'Nessus',
  openvas: 'OpenVAS',
  qualys: 'Qualys',
  ms_defender: 'MS Defender',
  aws_inspector: 'AWS Inspector',
  azure_defender: 'Azure Defender',
  crowdstrike: 'CrowdStrike',
  scanner: 'Scanner',
  manual: 'Manuel',
};

export const pick = <T>(v: [T, T], lang: LocaleCode): T => (lang === 'fr' ? v[0] : v[1]);
