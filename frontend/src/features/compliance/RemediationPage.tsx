// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// Remediation plans ("Plans de remédiation") — the actions that close compliance
// gaps. Wired to /compliance/remediation. Create, assign a priority + due date,
// and track status (open → in progress → completed). Plans link back to the
// compliance control they remediate.

import { localeTag } from '../../i18n/locales';
import { useMemo, useState } from 'react';
import { Wrench, Plus, Trash2, AlertCircle, ArrowLeft } from 'lucide-react';
import { useNavigate, Link } from 'react-router';
import { toast } from 'sonner';
import {
  PageFrame,
  PageHeader,
  Btn,
  Card,
  SkeletonRows,
  EmptyState,
  ErrorState,
} from '../../shared/ui';
import { useUIStore } from '../../store/uiStore';
import { useAuthStore } from '../../hooks/useAuthStore';
import { useUndoableRemove } from '../../shared/useUndoableRemove';
import { useRemediations } from './useCompliance';
import { CreateRemediationDialog } from './AuditRemediationModals';
import type {
  RemediationPlan,
  RemediationPriority,
  RemediationStatus,
} from '../../types/compliance';

const STATUS_META: Record<RemediationStatus, { color: string; fr: string; en: string }> = {
  open: { color: 'var(--critical)', fr: 'Ouvert', en: 'Open' },
  in_progress: { color: 'var(--high)', fr: 'En cours', en: 'In progress' },
  completed: { color: 'var(--low)', fr: 'Terminé', en: 'Completed' },
  cancelled: { color: 'var(--fg-muted)', fr: 'Annulé', en: 'Cancelled' },
};
const PRIORITY_META: Record<RemediationPriority, { color: string; fr: string; en: string }> = {
  low: { color: 'var(--low)', fr: 'Basse', en: 'Low' },
  medium: { color: 'var(--medium)', fr: 'Moyenne', en: 'Medium' },
  high: { color: 'var(--high)', fr: 'Haute', en: 'High' },
  critical: { color: 'var(--critical)', fr: 'Critique', en: 'Critical' },
};
const STATUS_ORDER: RemediationStatus[] = ['open', 'in_progress', 'completed', 'cancelled'];

export function RemediationPage() {
  const lang = useUIStore((s) => s.lang);
  const navigate = useNavigate();
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const { remediations, isLoading, error, refetch, updateRemediation, deleteRemediation } =
    useRemediations();
  const { undoRemove, isHidden } = useUndoableRemove();
  const hasPermission = useAuthStore((s) => s.hasPermission);
  const canWrite = hasPermission('compliance:remediations:write');

  const [showCreate, setShowCreate] = useState(false);
  const [filter, setFilter] = useState<'all' | RemediationStatus>('all');

  const fmtDate = (d: string | null) =>
    d
      ? new Date(d).toLocaleDateString(localeTag(lang), {
          day: '2-digit',
          month: 'short',
          year: 'numeric',
        })
      : '—';
  const isOverdue = (p: RemediationPlan) =>
    p.due_date != null &&
    p.status !== 'completed' &&
    p.status !== 'cancelled' &&
    new Date(p.due_date) < new Date();

  const visible = useMemo(
    () => remediations.filter((p) => !isHidden(p.id)),
    [remediations, isHidden],
  );

  const counts = useMemo(() => {
    const c: Record<string, number> = { all: visible.length };
    for (const p of visible) c[p.status] = (c[p.status] ?? 0) + 1;
    return c;
  }, [visible]);

  const shown = filter === 'all' ? visible : visible.filter((p) => p.status === filter);

  const setStatus = (p: RemediationPlan, status: RemediationStatus) =>
    updateRemediation.mutate({ id: p.id, payload: { status } });
  const remove = (p: RemediationPlan) =>
    undoRemove(p.id, {
      message: tr(`Plan « ${p.title} » supprimé`, `Plan "${p.title}" removed`),
      undoLabel: tr('Annuler', 'Undo'),
      onCommit: () => deleteRemediation.mutate(p.id),
      onError: () => toast.error(tr('Suppression échouée', 'Delete failed')),
    });

  return (
    <PageFrame wide>
      <button
        onClick={() => navigate('/compliance')}
        className="inline-flex items-center gap-1.5 text-[12.5px] font-semibold text-ink-soft hover:text-ink mb-3 transition-colors"
      >
        <ArrowLeft size={15} /> {tr('Conformité', 'Compliance')}
      </button>
      <PageHeader
        title={tr('Plans de remédiation', 'Remediation plans')}
        actions={
          canWrite ? (
            <Btn
              label={tr('Nouveau plan', 'New plan')}
              icon={Plus}
              primary
              onClick={() => setShowCreate(true)}
            />
          ) : undefined
        }
      />

      {isLoading ? (
        <Card style={{ padding: 8 }}>
          <SkeletonRows rows={5} height={56} />
        </Card>
      ) : error ? (
        <Card>
          <ErrorState
            title={tr('Chargement impossible', 'Failed to load')}
            onRetry={() => refetch()}
            retryLabel={tr('Réessayer', 'Retry')}
          />
        </Card>
      ) : remediations.length === 0 ? (
        <Card>
          <EmptyState
            icon={Wrench}
            title={tr('Aucun plan de remédiation', 'No remediation plans')}
            description={tr(
              'Créez un plan pour corriger un écart de conformité, ou lancez-en un depuis l’analyse d’écarts.',
              'Create a plan to close a compliance gap, or start one from the gap analysis.',
            )}
            primaryAction={
              canWrite ? (
                <Btn
                  label={tr('Nouveau plan', 'New plan')}
                  icon={Plus}
                  primary
                  onClick={() => setShowCreate(true)}
                />
              ) : undefined
            }
          />
        </Card>
      ) : (
        <>
          <div className="flex items-center gap-2 flex-wrap mb-4">
            <FilterChip
              label={tr('Tous', 'All')}
              active={filter === 'all'}
              onClick={() => setFilter('all')}
              count={counts.all}
            />
            {STATUS_ORDER.map((s) => (
              <FilterChip
                key={s}
                label={lang === 'fr' ? STATUS_META[s].fr : STATUS_META[s].en}
                active={filter === s}
                onClick={() => setFilter(s)}
                count={counts[s] ?? 0}
                color={STATUS_META[s].color}
              />
            ))}
          </div>

          <Card style={{ padding: 0, overflow: 'hidden' }}>
            <div className="overflow-x-auto">
              <table className="w-full text-left" style={{ minWidth: 820 }}>
                <thead>
                  <tr
                    className="text-[11px] uppercase tracking-[.04em] text-ink-muted"
                    style={{ borderBottom: '1px solid var(--border)' }}
                  >
                    <th className="px-4 py-3 font-semibold">{tr('Plan', 'Plan')}</th>
                    <th className="px-4 py-3 font-semibold">
                      {tr('Contrôle lié', 'Linked control')}
                    </th>
                    <th className="px-4 py-3 font-semibold">{tr('Priorité', 'Priority')}</th>
                    <th className="px-4 py-3 font-semibold">{tr('Échéance', 'Due')}</th>
                    <th className="px-4 py-3 font-semibold">{tr('Statut', 'Status')}</th>
                    <th className="px-4 py-3 font-semibold text-right">
                      {tr('Actions', 'Actions')}
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {shown.map((p, i) => {
                    const pm = PRIORITY_META[p.priority];
                    return (
                      <tr
                        key={p.id}
                        className="text-[13px] text-ink"
                        style={{
                          borderTop: i === 0 ? 'none' : '1px solid var(--border)',
                          animation: 'or-fadeup .35s ease both',
                          animationDelay: `${Math.min(i * 0.03, 0.25)}s`,
                        }}
                      >
                        <td className="px-4 py-3">
                          {/* A link, not a row click: the detail is a route, so
                              it must be openable in a new tab and shareable. */}
                          <Link
                            to={`/compliance/remediation/${p.id}`}
                            className="font-semibold text-ink hover:text-accent-strong transition-colors"
                          >
                            {p.title}
                          </Link>
                          {p.description && (
                            <div
                              className="text-[11.5px] text-ink-muted truncate max-w-[300px]"
                              title={p.description}
                            >
                              {p.description}
                            </div>
                          )}
                        </td>
                        <td className="px-4 py-3 text-ink-soft">
                          {p.control_code ? (
                            <span className="mono text-[12px]" title={p.control_name}>
                              {p.control_code}
                            </span>
                          ) : (
                            '—'
                          )}
                        </td>
                        <td className="px-4 py-3">
                          <span
                            className="inline-flex items-center gap-1.5 text-[11.5px] font-semibold px-2.5 py-1 rounded-full"
                            style={{
                              background: `color-mix(in srgb,${pm.color} 12%,transparent)`,
                              color: pm.color,
                            }}
                          >
                            <span
                              className="h-1.5 w-1.5 rounded-full"
                              style={{ background: pm.color }}
                            />
                            {lang === 'fr' ? pm.fr : pm.en}
                          </span>
                        </td>
                        <td className="px-4 py-3 whitespace-nowrap">
                          <span
                            className="inline-flex items-center gap-1.5 text-ink-soft"
                            style={
                              isOverdue(p)
                                ? { color: 'var(--critical)', fontWeight: 600 }
                                : undefined
                            }
                          >
                            {isOverdue(p) && <AlertCircle size={13} />}
                            {fmtDate(p.due_date)}
                          </span>
                        </td>
                        <td className="px-4 py-3">
                          {canWrite ? (
                            <select
                              value={p.status}
                              onChange={(e) => setStatus(p, e.target.value as RemediationStatus)}
                              className="h-8 px-2 rounded-[8px] text-[12px] font-semibold outline-none"
                              style={{
                                border: `1px solid ${STATUS_META[p.status].color}`,
                                background: `color-mix(in srgb,${STATUS_META[p.status].color} 12%,transparent)`,
                                color: STATUS_META[p.status].color,
                              }}
                            >
                              {STATUS_ORDER.map((s) => (
                                <option key={s} value={s}>
                                  {lang === 'fr' ? STATUS_META[s].fr : STATUS_META[s].en}
                                </option>
                              ))}
                            </select>
                          ) : (
                            <span
                              className="inline-flex items-center gap-1.5 text-[12px] font-semibold px-2.5 py-1 rounded-full"
                              style={{
                                background: `color-mix(in srgb,${STATUS_META[p.status].color} 12%,transparent)`,
                                color: STATUS_META[p.status].color,
                              }}
                            >
                              {lang === 'fr' ? STATUS_META[p.status].fr : STATUS_META[p.status].en}
                            </span>
                          )}
                        </td>
                        <td className="px-4 py-3 text-right">
                          {canWrite && (
                            <button
                              onClick={() => remove(p)}
                              className="w-8 h-8 rounded-[8px] inline-flex items-center justify-center transition-colors hover:brightness-110"
                              style={{
                                border:
                                  '1px solid color-mix(in srgb,var(--critical) 30%,transparent)',
                                color: 'var(--critical)',
                              }}
                              title={tr('Supprimer', 'Delete')}
                            >
                              <Trash2 size={14} />
                            </button>
                          )}
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          </Card>
        </>
      )}

      {showCreate && <CreateRemediationDialog onClose={() => setShowCreate(false)} />}
    </PageFrame>
  );
}

function FilterChip({
  label,
  active,
  onClick,
  count,
  color,
}: {
  label: string;
  active?: boolean;
  onClick?: () => void;
  count?: number;
  color?: string;
}) {
  return (
    <button
      onClick={onClick}
      className="inline-flex items-center gap-1.5 h-7 px-3 rounded-full text-[12px] font-semibold transition-colors"
      style={{
        border: '1px solid var(--border-strong)',
        background: active ? (color ?? 'var(--accent)') : 'transparent',
        color: active ? '#fff' : 'var(--text-soft)',
      }}
    >
      {label}
      {typeof count === 'number' && (
        <span
          className="text-[10.5px] font-bold px-1.5 rounded-full"
          style={{ background: active ? 'rgba(255,255,255,.25)' : 'var(--hover)' }}
        >
          {count}
        </span>
      )}
    </button>
  );
}
