// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// Real /mitigations list shaped into the dc.html Kanban columns.

import { DEFAULT_LOCALE, formatDate, type LocaleCode } from '../../i18n';
import { useUIStore } from '../../store/uiStore';
import { useQuery } from '@tanstack/react-query';
import { mitigationService } from '../../services/mitigationService';
import type { Mitigation } from '../../types/mitigation';
import type { Criticality } from '../../shared/riskColors';
import { initialsOf } from '../risks/riskMap';

export type Column = 'todo' | 'progress' | 'review' | 'done';

export interface UiMiti {
  id: string;
  title: string;
  risk: string;
  owner: string;
  deadline: string;
  progress: number;
  crit: Criticality;
  overdue: boolean;
  column: Column;
  /** Raw backend status (domain.MitigationStatus) for the drawer's status control. */
  rawStatus: string;
  description?: string;
  /** Raw ISO dates for the Gantt view (may be undefined). */
  startISO?: string;
  dueISO?: string;
}

// Backend uses PLANNED for a freshly-created plan (not TODO) — both land in "todo".
const COL: Record<string, Column> = {
  PLANNED: 'todo',
  TODO: 'todo',
  IN_PROGRESS: 'progress',
  REVIEW: 'review',
  DONE: 'done',
};
const CRIT: Record<string, Criticality> = {
  critical: 'critical',
  high: 'high',
  medium: 'medium',
  low: 'low',
};

function fmtDate(iso: string | undefined, locale: LocaleCode): string {
  if (!iso) return '—';
  return formatDate(locale, iso, { day: '2-digit', month: 'short' });
}

/**
 * `locale` is explicit rather than read from the store: this mapper runs inside
 * a query function, where a hook cannot. The caller passes the active language
 * and the query key carries it, so switching language refetches the labels.
 */
export function mapMitigation(m: Mitigation, locale: LocaleCode = DEFAULT_LOCALE): UiMiti {
  const mm = m as Mitigation & {
    assignee?: string;
    risk_title?: string;
    created_at?: string;
    progress?: number;
  };
  const column = COL[m.status] ?? 'todo';
  const overdue = column !== 'done' && !!m.due_date && new Date(m.due_date).getTime() < Date.now();
  return {
    id: m.id,
    title: m.title,
    risk: mm.risk_title || (m.risk_id ? `#${m.risk_id.slice(0, 8)}` : '—'),
    owner: initialsOf(mm.assignee),
    deadline: fmtDate(m.due_date, locale),
    // Backend serialises the field as `progress`; keep the legacy fallback.
    progress: mm.progress ?? m.progress_percentage ?? 0,
    crit: CRIT[(m.priority ?? 'low').toLowerCase()] ?? 'low',
    overdue,
    column,
    rawStatus: m.status,
    description: m.description,
    startISO: mm.created_at,
    dueISO: m.due_date,
  };
}

export function useMitigations() {
  const locale = useUIStore((s) => s.lang);
  const query = useQuery({
    // The locale is part of the key: the mapper bakes formatted dates into the
    // rows, so a language switch has to produce a different cache entry.
    queryKey: ['mitigations', 'board', locale],
    queryFn: async () => {
      const res = await mitigationService.listMitigations({ page: 1, per_page: 200 });
      return (res.items ?? []).map((m) => mapMitigation(m, locale));
    },
  });
  const items = query.data ?? [];
  const columns: Record<Column, UiMiti[]> = { todo: [], progress: [], review: [], done: [] };
  for (const m of items) columns[m.column].push(m);
  // isError/refetch let the table render a retry instead of an empty board.
  return {
    items,
    columns,
    isLoading: query.isLoading,
    isError: query.isError,
    refetch: () => void query.refetch(),
  };
}
