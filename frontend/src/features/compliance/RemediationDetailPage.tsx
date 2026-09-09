// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// Remediation plan detail — /compliance/remediation/:planId.

import { useMemo, useState } from 'react';
import { useNavigate, useParams, Link } from 'react-router';
import { Trash2, CalendarClock } from 'lucide-react';
import { toast } from 'sonner';
import { DetailPage, DetailField } from '../../shared/DetailPage';
import { Btn, Card } from '../../shared/ui';
import { DangerConfirm } from '../../shared/DangerConfirm';
import { useUIStore } from '../../store/uiStore';
import { useAuthStore } from '../../hooks/useAuthStore';
import { useRemediations, useFrameworks } from './useCompliance';
import {
  REMEDIATION_STATUS_META,
  REMEDIATION_PRIORITY_META,
  REMEDIATION_STATUS_ORDER,
  formatDate,
} from './complianceMeta';
import type { RemediationStatus } from '../../types/compliance';
import { pickLocalized } from '../../i18n/locales';

export function RemediationDetailPage() {
  const { planId } = useParams<{ planId: string }>();
  const navigate = useNavigate();
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const hasPermission = useAuthStore((s) => s.hasPermission);
  const canWrite = hasPermission('compliance:remediations:write');
  const [confirmDelete, setConfirmDelete] = useState(false);

  const { remediations, isLoading, error, updateRemediation, deleteRemediation } =
    useRemediations();
  const { frameworks } = useFrameworks();
  const plan = useMemo(() => remediations.find((r) => r.id === planId), [remediations, planId]);
  const framework = frameworks?.find((f) => f.id === plan?.framework_id);

  const setStatus = (status: RemediationStatus) => {
    if (!plan) return;
    updateRemediation.mutate(
      { id: plan.id, payload: { status } },
      {
        onSuccess: () => toast.success(tr('Plan mis à jour', 'Plan updated')),
        onError: () => toast.error(tr('Mise à jour échouée', 'Update failed')),
      },
    );
  };

  const status = plan ? REMEDIATION_STATUS_META[plan.status] : null;
  const priority = plan ? REMEDIATION_PRIORITY_META[plan.priority] : null;
  // Read the clock once per mount rather than on every render: an overdue flag
  // that can flip between two renders of the same page is a purity violation
  // and, worse, a badge that appears and disappears while you look at it.
  const [now] = useState(() => Date.now());
  const overdue =
    plan?.due_date && plan.status !== 'completed' && plan.status !== 'cancelled'
      ? new Date(plan.due_date).getTime() < now
      : false;

  return (
    <DetailPage
      title={plan?.title}
      backLabel={tr('Plans de remédiation', 'Remediation plans')}
      loading={isLoading}
      error={error}
      notFound={!isLoading && !error && !plan}
      subtitle={
        plan && status && priority ? (
          <>
            <span
              className="text-[11.5px] font-semibold px-2 py-[3px] rounded-md"
              style={{
                color: status.color,
                background: `color-mix(in srgb, ${status.color} 14%, transparent)`,
              }}
            >
              {pickLocalized(lang, status)}
            </span>
            <span
              className="text-[11.5px] font-semibold px-2 py-[3px] rounded-md"
              style={{
                color: priority.color,
                background: `color-mix(in srgb, ${priority.color} 14%, transparent)`,
              }}
            >
              {pickLocalized(lang, priority)}
            </span>
            {overdue && (
              <span
                className="inline-flex items-center gap-1 text-[11.5px] font-semibold"
                style={{ color: 'var(--critical)' }}
              >
                <CalendarClock size={13} /> {tr('En retard', 'Overdue')}
              </span>
            )}
          </>
        ) : null
      }
      actions={
        plan && canWrite ? (
          <Btn
            label={tr('Supprimer', 'Delete')}
            icon={Trash2}
            danger
            onClick={() => setConfirmDelete(true)}
          />
        ) : null
      }
    >
      {plan && (
        <>
          <Card style={{ padding: '4px 18px 14px' }}>
            <DetailField label={tr('Description', 'Description')}>{plan.description}</DetailField>
            <DetailField label={tr('Contrôle', 'Control')}>
              {plan.control_code ?? (plan.control_id ? plan.control_id : null)}
            </DetailField>
            <DetailField label={tr('Référentiel', 'Framework')}>
              {framework ? (
                <Link
                  to={`/compliance/frameworks/${framework.id}`}
                  className="text-accent hover:underline"
                >
                  {framework.name}
                </Link>
              ) : null}
            </DetailField>
            <DetailField label={tr('Audit d’origine', 'Originating audit')}>
              {plan.audit_id ? (
                // The audit that generated this plan is a sibling branch, so it
                // is a body link rather than a breadcrumb ancestor.
                <Link
                  to={`/compliance/audits/${plan.audit_id}`}
                  className="text-accent hover:underline"
                >
                  {tr('Voir l’audit', 'View audit')}
                </Link>
              ) : null}
            </DetailField>
            <DetailField label={tr('Échéance', 'Due date')}>
              {formatDate(plan.due_date, lang)}
            </DetailField>
            <DetailField label={tr('Terminé le', 'Completed at')}>
              {formatDate(plan.completed_at, lang)}
            </DetailField>
          </Card>

          {canWrite && (
            <div className="mt-5">
              <div className="text-[13px] font-semibold text-ink mb-2.5">
                {tr('Statut', 'Status')}
              </div>
              <div className="flex gap-2 flex-wrap">
                {REMEDIATION_STATUS_ORDER.map((s) => {
                  const m = REMEDIATION_STATUS_META[s];
                  const active = plan.status === s;
                  return (
                    <button
                      key={s}
                      onClick={() => !active && setStatus(s)}
                      className="h-8 px-3 rounded-[9px] text-[12.5px] font-semibold transition-colors"
                      style={{
                        color: active ? 'var(--fg-on-solid)' : m.color,
                        background: active
                          ? m.color
                          : `color-mix(in srgb, ${m.color} 12%, transparent)`,
                      }}
                    >
                      {pickLocalized(lang, m)}
                    </button>
                  );
                })}
              </div>
            </div>
          )}
        </>
      )}

      {confirmDelete && plan && (
        <DangerConfirm
          open
          onClose={() => setConfirmDelete(false)}
          onConfirm={() => {
            deleteRemediation.mutate(plan.id, {
              onSuccess: () => {
                toast.success(tr('Plan supprimé', 'Plan deleted'));
                navigate('/compliance/remediation');
              },
              onError: () => toast.error(tr('Suppression échouée', 'Delete failed')),
            });
            setConfirmDelete(false);
          }}
          subject={plan.title}
          title={tr('Supprimer ce plan ?', 'Delete this plan?')}
          intro={tr(
            'L’écart qu’il traite restera ouvert dans l’analyse d’écarts.',
            'The gap it addresses stays open in the gap analysis.',
          )}
          impact={[
            { label: tr('Statut', 'Status'), value: pickLocalized(lang, status) ?? plan.status },
            { label: tr('Priorité', 'Priority'), value: pickLocalized(lang, priority) ?? plan.priority },
            { label: tr('Échéance', 'Due date'), value: formatDate(plan.due_date, lang) },
          ]}
          confirmLabel={tr('Supprimer', 'Delete')}
        />
      )}
    </DetailPage>
  );
}
