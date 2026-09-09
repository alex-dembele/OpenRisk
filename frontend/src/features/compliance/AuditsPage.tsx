// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// Compliance audits ("Audits" — planification, exécution, historique). Wired to
// GET/POST/PATCH/DELETE /compliance/audits. Schedule an audit, move it through
// its lifecycle (planned → in progress → completed), and keep the history.

import { localeTag } from '../../i18n/locales';
import { useMemo, useState } from 'react';
import { CalendarClock, Plus, Trash2, Wand2, ArrowLeft } from 'lucide-react';
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
import { Hint } from '../../shared/Hint';
import { useAudits, useFrameworks } from './useCompliance';
import { CreateAuditDialog } from './AuditRemediationModals';
import { AiAuditReportButton } from '../ai/AiAuditReportButton';
import type { AuditStatus, ComplianceAudit } from '../../types/compliance';

const STATUS_META: Record<AuditStatus, { color: string; fr: string; en: string }> = {
  planned: { color: 'var(--fg-muted)', fr: 'Planifié', en: 'Planned' },
  in_progress: { color: 'var(--high)', fr: 'En cours', en: 'In progress' },
  completed: { color: 'var(--low)', fr: 'Terminé', en: 'Completed' },
  cancelled: { color: 'var(--critical)', fr: 'Annulé', en: 'Cancelled' },
};
const TYPE_LABEL: Record<string, { fr: string; en: string }> = {
  internal: { fr: 'Interne', en: 'Internal' },
  external: { fr: 'Externe', en: 'External' },
  certification: { fr: 'Certification', en: 'Certification' },
  surveillance: { fr: 'Surveillance', en: 'Surveillance' },
};
const STATUS_ORDER: AuditStatus[] = ['planned', 'in_progress', 'completed', 'cancelled'];

export function AuditsPage() {
  const lang = useUIStore((s) => s.lang);
  const navigate = useNavigate();
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const { audits, isLoading, error, refetch, updateAudit, deleteAudit, generateRemediations } =
    useAudits();
  const { frameworks } = useFrameworks();
  const { undoRemove, isHidden } = useUndoableRemove();
  const hasPermission = useAuthStore((s) => s.hasPermission);
  const canWrite = hasPermission('compliance:audits:write');
  const canRemediate = hasPermission('compliance:remediations:write');

  const [showCreate, setShowCreate] = useState(false);
  const [filter, setFilter] = useState<'all' | AuditStatus>('all');

  const fwName = (id: string | null) =>
    id
      ? (frameworks.find((f) => f.id === id)?.name ?? tr('Référentiel', 'Framework'))
      : tr('Programme entier', 'Whole program');
  const fmtDate = (d: string | null) =>
    d
      ? new Date(d).toLocaleDateString(localeTag(lang), {
          day: '2-digit',
          month: 'short',
          year: 'numeric',
        })
      : '—';

  const visible = useMemo(() => audits.filter((a) => !isHidden(a.id)), [audits, isHidden]);

  const counts = useMemo(() => {
    const c: Record<string, number> = { all: visible.length };
    for (const a of visible) c[a.status] = (c[a.status] ?? 0) + 1;
    return c;
  }, [visible]);

  const shown = filter === 'all' ? visible : visible.filter((a) => a.status === filter);

  const setStatus = (a: ComplianceAudit, status: AuditStatus) => {
    updateAudit.mutate({ id: a.id, payload: { status } });
  };
  const remove = (a: ComplianceAudit) =>
    undoRemove(a.id, {
      message: tr(`Audit « ${a.title} » supprimé`, `Audit "${a.title}" removed`),
      undoLabel: tr('Annuler', 'Undo'),
      onCommit: () => deleteAudit.mutate(a.id),
      onError: () => toast.error(tr('Suppression échouée', 'Delete failed')),
    });
  const genRemediations = (a: ComplianceAudit) => {
    if (!a.framework_id) {
      toast.error(
        tr(
          'Cet audit couvre le programme entier — rattachez-le à un référentiel.',
          'This audit is program-wide — scope it to a framework.',
        ),
      );
      return;
    }
    toast.promise(generateRemediations.mutateAsync(a.id), {
      loading: tr('Génération des plans…', 'Generating plans…'),
      success: (r) => {
        const msg = tr(
          `${r.created} plan(s) créé(s), ${r.skipped} déjà couvert(s)`,
          `${r.created} plan(s) created, ${r.skipped} already covered`,
        );
        if (r.created > 0) setTimeout(() => navigate('/compliance/remediation'), 600);
        return msg;
      },
      error: tr('Génération échouée', 'Generation failed'),
    });
  };

  return (
    <PageFrame wide>
      <button
        onClick={() => navigate('/compliance')}
        className="inline-flex items-center gap-1.5 text-[12.5px] font-semibold text-ink-soft hover:text-ink mb-3 transition-colors"
      >
        <ArrowLeft size={15} /> {tr('Conformité', 'Compliance')}
      </button>
      <PageHeader
        title={tr('Audits de conformité', 'Compliance audits')}
        actions={
          canWrite ? (
            <Btn
              label={tr('Planifier un audit', 'Schedule audit')}
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
      ) : audits.length === 0 ? (
        <Card>
          <EmptyState
            icon={CalendarClock}
            title={tr('Aucun audit', 'No audits yet')}
            description={tr(
              'Planifiez votre premier audit de conformité (interne, externe, certification…).',
              'Schedule your first compliance audit (internal, external, certification…).',
            )}
            primaryAction={
              canWrite ? (
                <Btn
                  label={tr('Planifier un audit', 'Schedule audit')}
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
              <table className="w-full text-left" style={{ minWidth: 860 }}>
                <thead>
                  <tr
                    className="text-[11px] uppercase tracking-[.04em] text-ink-muted"
                    style={{ borderBottom: '1px solid var(--border)' }}
                  >
                    <th className="px-4 py-3 font-semibold">{tr('Audit', 'Audit')}</th>
                    <th className="px-4 py-3 font-semibold">{tr('Type', 'Type')}</th>
                    <th className="px-4 py-3 font-semibold">{tr('Référentiel', 'Framework')}</th>
                    <th className="px-4 py-3 font-semibold">{tr('Auditeur', 'Auditor')}</th>
                    <th className="px-4 py-3 font-semibold">{tr('Période', 'Period')}</th>
                    <th className="px-4 py-3 font-semibold">{tr('Statut', 'Status')}</th>
                    <th className="px-4 py-3 font-semibold text-right">
                      {tr('Actions', 'Actions')}
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {shown.map((a, i) => (
                    <tr
                      key={a.id}
                      className="text-[13px] text-ink"
                      style={{
                        borderTop: i === 0 ? 'none' : '1px solid var(--border)',
                        animation: 'or-fadeup .35s ease both',
                        animationDelay: `${Math.min(i * 0.03, 0.25)}s`,
                      }}
                    >
                      <td className="px-4 py-3">
                        {/* A link, not a row click: the detail is a route, so it
                            must be openable in a new tab and shareable. */}
                        <Link
                          to={`/compliance/audits/${a.id}`}
                          className="font-semibold text-ink hover:text-accent-strong transition-colors"
                        >
                          {a.title}
                        </Link>
                        {a.scope && (
                          <div
                            className="text-[11.5px] text-ink-muted truncate max-w-[280px]"
                            title={a.scope}
                          >
                            {a.scope}
                          </div>
                        )}
                      </td>
                      <td className="px-4 py-3 text-ink-soft">
                        {lang === 'fr' ? TYPE_LABEL[a.type]?.fr : TYPE_LABEL[a.type]?.en}
                      </td>
                      <td className="px-4 py-3 text-ink-soft">{fwName(a.framework_id)}</td>
                      <td className="px-4 py-3 text-ink-soft">{a.auditor || '—'}</td>
                      <td className="px-4 py-3 text-ink-soft whitespace-nowrap">
                        {fmtDate(a.scheduled_start)} → {fmtDate(a.scheduled_end)}
                      </td>
                      <td className="px-4 py-3">
                        {canWrite ? (
                          <select
                            value={a.status}
                            onChange={(e) => setStatus(a, e.target.value as AuditStatus)}
                            className="h-8 px-2 rounded-[8px] text-[12px] font-semibold outline-none"
                            style={{
                              border: `1px solid ${STATUS_META[a.status].color}`,
                              background: `color-mix(in srgb,${STATUS_META[a.status].color} 12%,transparent)`,
                              color: STATUS_META[a.status].color,
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
                              background: `color-mix(in srgb,${STATUS_META[a.status].color} 12%,transparent)`,
                              color: STATUS_META[a.status].color,
                            }}
                          >
                            {lang === 'fr' ? STATUS_META[a.status].fr : STATUS_META[a.status].en}
                          </span>
                        )}
                      </td>
                      <td className="px-4 py-3">
                        <div className="flex items-center justify-end gap-1.5">
                          <AiAuditReportButton auditId={a.id} title={a.title} />
                          {canRemediate && a.framework_id && (
                            <Hint
                              id="audit-remediate"
                              text={tr(
                                'Crée automatiquement un plan de remédiation pour chaque écart du référentiel de cet audit.',
                                'Auto-creates a remediation plan for each gap in this audit’s framework.',
                              )}
                            >
                              <button
                                onClick={() => genRemediations(a)}
                                disabled={generateRemediations.isPending}
                                className="h-8 px-2.5 rounded-[8px] inline-flex items-center gap-1.5 text-[12px] font-semibold text-ink-soft hover:text-ink transition-colors disabled:opacity-60"
                                style={{
                                  border: '1px solid var(--border-strong)',
                                  background: 'var(--bg-elevated)',
                                }}
                                title={tr(
                                  'Générer les plans de remédiation pour les écarts',
                                  'Generate remediation plans for the gaps',
                                )}
                              >
                                <Wand2 size={13} /> {tr('Remédier', 'Remediate')}
                              </button>
                            </Hint>
                          )}
                          {canWrite && (
                            <button
                              onClick={() => remove(a)}
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
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </Card>
        </>
      )}

      {showCreate && <CreateAuditDialog onClose={() => setShowCreate(false)} />}
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
