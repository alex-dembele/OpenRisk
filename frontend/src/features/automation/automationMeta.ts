import type { LocaleCode } from '../../i18n/locales';
// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: LicenseRef-OpenRisk-Commercial
//
// Display metadata for the automation module: triggers, action types, notify
// channels, execution + SLA statuses. Kept separate so the page/editor share it.

import {
  Bug,
  ShieldAlert,
  Activity,
  Siren,
  Hand,
  Radar,
  FilePlus2,
  UserCheck,
  Ticket,
  Bell,
  Timer,
  CheckCircle2,
  XCircle,
  type LucideIcon,
} from 'lucide-react';
import type {
  AutomationTrigger,
  AutomationActionType,
  NotifyChannel,
  ExecutionStatus,
  SLAStatus,
} from './automationService';

type Lang = LocaleCode;
export const pick = (m: { fr: string; en: string }, lang: Lang) => (lang === 'fr' ? m.fr : m.en);

export const TRIGGER_META: Record<
  AutomationTrigger,
  { label: { fr: string; en: string }; icon: LucideIcon; hint: { fr: string; en: string } }
> = {
  vulnerability_detected: {
    label: { fr: 'Vulnérabilité détectée', en: 'Vulnerability detected' },
    icon: Bug,
    hint: {
      fr: 'Une nouvelle CVE est enregistrée (scan / flux)',
      en: 'A new CVE is ingested (scan / feed)',
    },
  },
  risk_score_updated: {
    label: { fr: 'Score de risque recalculé', en: 'Risk score updated' },
    icon: Activity,
    hint: { fr: 'Le Score Engine recalcule un risque', en: 'The Score Engine recomputes a risk' },
  },
  risk_created: {
    label: { fr: 'Risque créé', en: 'Risk created' },
    icon: ShieldAlert,
    hint: { fr: 'Un risque est ouvert', en: 'A risk is opened' },
  },
  incident_created: {
    label: { fr: 'Incident ouvert', en: 'Incident created' },
    icon: Siren,
    hint: { fr: 'Un incident est déclaré', en: 'An incident is declared' },
  },
  manual: {
    label: { fr: 'Manuel', en: 'Manual' },
    icon: Hand,
    hint: { fr: 'Exécuté uniquement à la demande', en: 'Only run on demand' },
  },
};

export const ACTION_META: Record<
  AutomationActionType,
  { label: { fr: string; en: string }; icon: LucideIcon; color: string }
> = {
  scan_asset: {
    label: { fr: 'Scanner l’actif', en: 'Scan asset' },
    icon: Radar,
    color: 'var(--accent-500)',
  },
  create_risk: {
    label: { fr: 'Créer un risque', en: 'Create risk' },
    icon: FilePlus2,
    color: 'var(--high)',
  },
  assign_owner: {
    label: { fr: 'Assigner', en: 'Assign owner' },
    icon: UserCheck,
    color: 'var(--medium)',
  },
  create_ticket: {
    label: { fr: 'Ouvrir un ticket', en: 'Open ticket' },
    icon: Ticket,
    color: 'var(--iris, #5A6ACF)',
  },
  notify: { label: { fr: 'Notifier', en: 'Notify' }, icon: Bell, color: 'var(--accent-500)' },
  start_sla: {
    label: { fr: 'Démarrer un SLA', en: 'Start SLA' },
    icon: Timer,
    color: 'var(--critical)',
  },
  resolve_risk: {
    label: { fr: 'Résoudre le risque', en: 'Resolve risk' },
    icon: CheckCircle2,
    color: 'var(--low)',
  },
  close_ticket: {
    label: { fr: 'Clôturer le ticket', en: 'Close ticket' },
    icon: XCircle,
    color: 'var(--low)',
  },
};

export const CHANNEL_META: Record<NotifyChannel, { label: string }> = {
  in_app: { label: 'In-app' },
  email: { label: 'Email' },
  slack: { label: 'Slack' },
  teams: { label: 'Microsoft Teams' },
  webhook: { label: 'Webhook' },
  sms: { label: 'SMS' },
};

export const EXEC_STATUS_META: Record<
  ExecutionStatus,
  { label: { fr: string; en: string }; color: string }
> = {
  success: { label: { fr: 'Succès', en: 'Success' }, color: 'var(--low)' },
  partial: { label: { fr: 'Partiel', en: 'Partial' }, color: 'var(--medium)' },
  failed: { label: { fr: 'Échec', en: 'Failed' }, color: 'var(--critical)' },
  running: { label: { fr: 'En cours', en: 'Running' }, color: 'var(--accent-500)' },
  pending: { label: { fr: 'En attente', en: 'Pending' }, color: 'var(--fg-secondary)' },
  skipped: { label: { fr: 'Ignoré', en: 'Skipped' }, color: 'var(--fg-secondary)' },
};

export const SLA_STATUS_META: Record<
  SLAStatus,
  { label: { fr: string; en: string }; color: string }
> = {
  open: { label: { fr: 'En cours', en: 'Open' }, color: 'var(--accent-500)' },
  breached: { label: { fr: 'Dépassé', en: 'Breached' }, color: 'var(--high)' },
  escalated: { label: { fr: 'Escaladé', en: 'Escalated' }, color: 'var(--critical)' },
  met: { label: { fr: 'Respecté', en: 'Met' }, color: 'var(--low)' },
  closed: { label: { fr: 'Clôturé', en: 'Closed' }, color: 'var(--fg-secondary)' },
};

export const SEVERITY_COLOR: Record<string, string> = {
  critical: 'var(--critical)',
  high: 'var(--high)',
  medium: 'var(--medium)',
  low: 'var(--low)',
};

// Format a minute count as "2h 30m" / "45m" / "overdue 12m".
export function fmtMinutes(mins: number, lang: Lang): string {
  const abs = Math.abs(mins);
  const h = Math.floor(abs / 60);
  const m = abs % 60;
  const body = h > 0 ? `${h}h ${m}m` : `${m}m`;
  if (mins < 0) return lang === 'fr' ? `dépassé de ${body}` : `overdue ${body}`;
  return body;
}
