// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// Audit detail — /compliance/audits/:auditId.
//
// An audit had no page of its own: everything happened inline in the list, so
// there was nothing to link to and nothing to come back from. It is now a route
// with a declared parent, which is what makes Back and the breadcrumb work.

import { useMemo } from 'react';
import { useNavigate, useParams, Link } from 'react-router';
import { Wand2, Trash2, FileText } from 'lucide-react';
import { toast } from 'sonner';
import { DetailPage, DetailField } from '../../shared/DetailPage';
import { Btn, Card } from '../../shared/ui';
import { DangerConfirm } from '../../shared/DangerConfirm';
import { useUIStore } from '../../store/uiStore';
import { useAuthStore } from '../../hooks/useAuthStore';
import { useAudits, useFrameworks } from './useCompliance';
import {
  AUDIT_STATUS_META,
  AUDIT_TYPE_LABEL,
  AUDIT_STATUS_ORDER,
  formatDate,
} from './complianceMeta';
import { useState } from 'react';
import type { AuditStatus } from '../../types/compliance';
import { pickLocalized } from '../../i18n/locales';

export function AuditDetailPage() {
  const { auditId } = useParams<{ auditId: string }>();
  const navigate = useNavigate();
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const hasPermission = useAuthStore((s) => s.hasPermission);
  const canWrite = hasPermission('compliance:audits:write');
  const canRemediate = hasPermission('compliance:remediations:write');
  const [confirmDelete, setConfirmDelete] = useState(false);

  const { audits, isLoading, error, updateAudit, deleteAudit, generateRemediations } = useAudits();
  const { frameworks } = useFrameworks();
  const audit = useMemo(() => audits.find((a) => a.id === auditId), [audits, auditId]);
  const framework = frameworks?.find((f) => f.id === audit?.framework_id);

  const setStatus = (status: AuditStatus) => {
    if (!audit) return;
    updateAudit.mutate(
      { id: audit.id, payload: { status } },
      {
        onSuccess: () => toast.success(tr('Audit mis à jour', 'Audit updated')),
        onError: () => toast.error(tr('Mise à jour échouée', 'Update failed')),
      },
    );
  };

  const remediate = () => {
    if (!audit) return;
    generateRemediations.mutate(audit.id, {
      onSuccess: (res) => {
        toast.success(
          tr(
            `${res.created} plan(s) créé(s), ${res.skipped} ignoré(s)`,
            `${res.created} plan(s) created, ${res.skipped} skipped`,
          ),
        );
        navigate('/compliance/remediation');
      },
      onError: () => toast.error(tr('Génération échouée', 'Generation failed')),
    });
  };

  const status = audit ? AUDIT_STATUS_META[audit.status] : null;

  return (
    <DetailPage
      title={audit?.title}
      backLabel={tr('Audits', 'Audits')}
      loading={isLoading}
      error={error}
      notFound={!isLoading && !error && !audit}
      subtitle={
        audit && status ? (
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
            <span className="text-ink-muted">
              {pickLocalized(lang, AUDIT_TYPE_LABEL[audit.type]) ?? audit.type}
            </span>
            {framework && (
              // Cross-link to the framework, which is a sibling branch of the
              // tree rather than an ancestor — the breadcrumb cannot express it,
              // so it belongs in the body.
              <Link
                to={`/compliance/frameworks/${framework.id}`}
                className="text-accent hover:underline"
              >
                {framework.name}
              </Link>
            )}
          </>
        ) : null
      }
      actions={
        audit ? (
          <>
            {canRemediate && audit.framework_id && (
              <Btn
                label={tr('Générer les plans', 'Generate plans')}
                icon={Wand2}
                onClick={remediate}
              />
            )}
            {canWrite && (
              <Btn
                label={tr('Supprimer', 'Delete')}
                icon={Trash2}
                danger
                onClick={() => setConfirmDelete(true)}
              />
            )}
          </>
        ) : null
      }
    >
      {audit && (
        <>
          <Card style={{ padding: '4px 18px 14px' }}>
            <DetailField label={tr('Auditeur', 'Auditor')}>{audit.auditor}</DetailField>
            <DetailField label={tr('Périmètre', 'Scope')}>{audit.scope}</DetailField>
            <DetailField label={tr('Synthèse', 'Summary')}>{audit.summary}</DetailField>
            <DetailField label={tr('Référentiel', 'Framework')}>
              {framework?.name ?? tr('Programme entier', 'Programme-wide')}
            </DetailField>
            <DetailField label={tr('Début planifié', 'Scheduled start')}>
              {formatDate(audit.scheduled_start, lang)}
            </DetailField>
            <DetailField label={tr('Terminé le', 'Completed at')}>
              {formatDate(audit.completed_at, lang)}
            </DetailField>
            {audit.status === 'completed' && (
              <DetailField label={tr('Score de conformité', 'Compliance score')}>
                {audit.compliance_score.toFixed(1)} %
              </DetailField>
            )}
          </Card>

          {canWrite && (
            <div className="mt-5">
              <div className="text-[13px] font-semibold text-ink mb-2.5">
                {tr('Statut', 'Status')}
              </div>
              <div className="flex gap-2 flex-wrap">
                {AUDIT_STATUS_ORDER.map((s) => {
                  const m = AUDIT_STATUS_META[s];
                  const active = audit.status === s;
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

          <div className="mt-5">
            <Link
              to="/compliance/remediation"
              className="inline-flex items-center gap-1.5 text-[12.5px] font-semibold text-accent hover:underline"
            >
              <FileText size={14} /> {tr('Voir les plans de remédiation', 'View remediation plans')}
            </Link>
          </div>
        </>
      )}

      {confirmDelete && audit && (
        <DangerConfirm
          open
          onClose={() => setConfirmDelete(false)}
          onConfirm={() => {
            deleteAudit.mutate(audit.id, {
              onSuccess: () => {
                toast.success(tr('Audit supprimé', 'Audit deleted'));
                navigate('/compliance/audits');
              },
              onError: () => toast.error(tr('Suppression échouée', 'Delete failed')),
            });
            setConfirmDelete(false);
          }}
          subject={audit.title}
          title={tr('Supprimer cet audit ?', 'Delete this audit?')}
          intro={tr(
            'L’audit et son historique disparaissent. Les plans de remédiation déjà générés sont conservés.',
            'The audit and its history are removed. Remediation plans already generated are kept.',
          )}
          impact={[
            { label: tr('Statut', 'Status'), value: pickLocalized(lang, status) ?? audit.status },
            {
              label: tr('Référentiel', 'Framework'),
              value: framework?.name ?? tr('Programme entier', 'Programme-wide'),
            },
          ]}
          confirmLabel={tr('Supprimer', 'Delete')}
        />
      )}
    </DetailPage>
  );
}
