// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// Compliance (OpenRisk.dc.html §6.10) — wired to real /compliance/frameworks +
// per-framework progress. Posture hero (aggregate radial + copy + CTAs) and a grid
// of framework cards; clicking a card opens its controls. Admins can add a blank
// framework, import one from the regulatory catalog, or delete one.

import { useState } from 'react';
import {
  FileText,
  AlertTriangle,
  Download,
  ClipboardCheck,
  ChevronRight,
  Plus,
  Library,
  Trash2,
  CalendarClock,
  Wrench,
} from 'lucide-react';
import { useNavigate } from 'react-router';
import { toast } from 'sonner';
import {
  PageFrame,
  PageHeader,
  Btn,
  Card,
  RingGauge,
  SkeletonRows,
  EmptyState,
} from '../../shared/ui';
import { useUIStrings } from '../../shared/uiStrings';
import { useUIStore } from '../../store/uiStore';
import { useAuthStore } from '../../hooks/useAuthStore';
import {
  useComplianceOverview,
  frameworkColorFor,
  type FrameworkWithProgress,
} from './complianceOverview';
import { useFrameworks } from './useCompliance';
import { useGenerateReport } from '../reports/useReportJobs';
import { CreateFrameworkDialog, ImportFrameworkDialog } from './ComplianceModals';
import { ImpactDialog } from '../../shared/ImpactDialog';
import { useI18n } from '../../hooks/useI18n';

export function ComplianceScreen() {
  const { t } = useI18n();
  const L = useUIStrings();
  const lang = useUIStore((s) => s.lang);
  const navigate = useNavigate();
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const { data: fws = [], isLoading } = useComplianceOverview();
  const generate = useGenerateReport();
  const { deleteFramework } = useFrameworks();

  const hasPermission = useAuthStore((s) => s.hasPermission);
  const canCreate = hasPermission('compliance:frameworks:create');
  const canDelete = hasPermission('compliance:frameworks:delete');

  const [modal, setModal] = useState<null | 'create' | 'import'>(null);
  const [toDelete, setToDelete] = useState<FrameworkWithProgress | null>(null);

  const overall = fws.length ? Math.round(fws.reduce((a, f) => a + f.pct, 0) / fws.length) : 0;
  const totalControls = fws.reduce((a, f) => a + f.total, 0);
  const gaps = fws.reduce((a, f) => a + Math.max(0, f.total - f.passed), 0);
  // Compliance COVERAGE (0–100, higher is better) — not a risk score band.
  const overallColor =
    overall >= 70 ? 'var(--low)' : overall >= 40 ? 'var(--high)' : 'var(--critical)';

  // Generating a report creates a job and lands the user on it
  // (/reports/jobs/:id). It used to fire a blob download from here while the
  // hero button navigated to /reports, whose Compliance tile navigated back —
  // the round trip. There is now exactly one destination: the artifact.
  const download = (f: FrameworkWithProgress) => {
    generate.mutate(
      { kind: 'compliance_framework', params: { framework_id: f.id, locale: lang } },
      { onError: () => toast.error(tr('Échec de la génération', 'Report generation failed')) },
    );
  };

  // Important + irreversible → impact radiography (UX-11), not a bare confirm.
  const remove = (f: FrameworkWithProgress) => setToDelete(f);
  const confirmRemove = () => {
    const f = toDelete;
    if (!f) return;
    setToDelete(null);
    toast.promise(deleteFramework.mutateAsync(f.id), {
      loading: tr('Suppression…', 'Deleting…'),
      success: tr('Référentiel supprimé', 'Framework deleted'),
      error: tr('Suppression échouée — réessayez.', 'Delete failed — retry.'),
    });
  };

  const headerActions = canCreate ? (
    <>
      <Btn
        label={tr('Importer', 'Import')}
        icon={Library}
        primary
        onClick={() => setModal('import')}
      />
      <Btn label={tr('Nouveau', 'New')} icon={Plus} onClick={() => setModal('create')} />
    </>
  ) : undefined;

  return (
    <PageFrame>
      <PageHeader title={L.n_compliance} actions={headerActions} />

      {/* Compliance hub nav — the module's sub-areas beyond frameworks. */}
      <div className="flex items-center gap-2 flex-wrap mb-4">
        <HubLink
          icon={AlertTriangle}
          label={tr("Analyse d'écarts", 'Gap analysis')}
          onClick={() => navigate('/compliance/gaps')}
        />
        <HubLink
          icon={CalendarClock}
          label={tr('Audits', 'Audits')}
          onClick={() => navigate('/compliance/audits')}
        />
        <HubLink
          icon={Wrench}
          label={tr('Plans de remédiation', 'Remediation plans')}
          onClick={() => navigate('/compliance/remediation')}
        />
      </div>

      {isLoading ? (
        <Card style={{ padding: 8 }}>
          <SkeletonRows rows={4} height={64} />
        </Card>
      ) : fws.length === 0 ? (
        <Card>
          <EmptyState
            icon={ClipboardCheck}
            title={tr('Aucun référentiel', 'No frameworks yet')}
            description={tr(
              'Importez un référentiel (ISO 27001, SOC 2, BCEAO…) ou créez le vôtre pour suivre votre conformité.',
              'Import a framework (ISO 27001, SOC 2, BCEAO…) or create your own to track compliance.',
            )}
            primaryAction={
              canCreate ? (
                <div className="flex gap-2.5">
                  <Btn
                    label={tr('Importer un référentiel', 'Import a framework')}
                    icon={Library}
                    primary
                    onClick={() => setModal('import')}
                  />
                  <Btn
                    label={tr('Créer', 'Create')}
                    icon={Plus}
                    onClick={() => setModal('create')}
                  />
                </div>
              ) : undefined
            }
          />
        </Card>
      ) : (
        <>
          <Card style={{ padding: '22px 24px', marginBottom: 16 }}>
            <div className="flex items-center gap-6 flex-wrap">
              <RingGauge value={overall} size={128} color={overallColor}>
                <span className="disp mono text-[32px] font-bold text-ink">{overall}%</span>
                <span className="text-[11px] text-ink-muted">{tr('conforme', 'compliant')}</span>
              </RingGauge>
              <div className="flex-1 min-w-[280px]">
                <div className="disp text-[19px] font-bold text-ink mb-1.5">
                  {tr('Posture de conformité', 'Compliance posture')}
                </div>
                <div className="text-[13.5px] text-ink-soft leading-relaxed mb-3.5 max-w-[520px]">
                  {tr(
                    `${totalControls} contrôles suivis sur ${t('common.frameworks', { count: fws.length })}. ${t('common.controls', { count: gaps })} requièrent une action.`,
                    `${totalControls} controls tracked across ${t('common.frameworks', { count: fws.length })}. ${t('common.controls', { count: gaps })} need action.`,
                  )}
                </div>
                <div className="flex gap-2.5 flex-wrap">
                  {/* Generates for the first framework when there is only one;
                      with several, the per-card buttons are the unambiguous
                      choice and this simply lists what has been generated. It
                      must never navigate to /reports as a way of "generating". */}
                  {fws.length === 1 ? (
                    <Btn
                      label={L.genReport}
                      icon={FileText}
                      primary
                      disabled={generate.isPending}
                      onClick={() => download(fws[0])}
                    />
                  ) : (
                    <Btn
                      label={tr('Rapports générés', 'Generated reports')}
                      icon={FileText}
                      onClick={() => navigate('/reports')}
                    />
                  )}
                  <Btn
                    label={tr('Voir les écarts', 'View gaps')}
                    icon={AlertTriangle}
                    onClick={() => navigate('/compliance/gaps')}
                  />
                </div>
              </div>
            </div>
          </Card>

          <div
            className="grid gap-4"
            style={{ gridTemplateColumns: 'repeat(auto-fill,minmax(260px,1fr))' }}
          >
            {fws.map((f, i) => {
              const col = frameworkColorFor(f.name, i);
              return (
                <Card
                  key={f.id}
                  style={{
                    padding: 18,
                    animation: 'or-fadeup .4s ease both',
                    animationDelay: `${Math.min(i * 0.04, 0.3)}s`,
                  }}
                >
                  <button
                    onClick={() => navigate(`/compliance/frameworks/${f.id}`)}
                    className="w-full flex items-center gap-3.5 mb-3.5 text-left group"
                  >
                    <RingGauge value={f.pct} size={56} color={col} thickness={6}>
                      <span className="mono text-[13px] font-bold text-ink">{f.pct}</span>
                    </RingGauge>
                    <div className="flex-1 min-w-0">
                      <div
                        className="text-[14px] font-semibold text-ink truncate group-hover:text-accent-strong-strong transition-colors"
                        title={f.name}
                      >
                        {f.name}
                      </div>
                      <div className="text-[12px] text-ink-soft mt-0.5">
                        {f.passed} / {f.total} {tr('contrôles', 'controls')}
                      </div>
                    </div>
                    <ChevronRight
                      size={16}
                      className="text-ink-muted shrink-0 group-hover:text-accent-strong-strong transition-colors"
                    />
                  </button>
                  <div className="flex gap-2">
                    <button
                      onClick={() => navigate(`/compliance/frameworks/${f.id}`)}
                      className="flex-1 h-8 rounded-[9px] text-[12.5px] font-semibold text-ink inline-flex items-center justify-center gap-1.5 hover:bg-hover transition-colors"
                      style={{ border: '1px solid var(--border-strong)' }}
                    >
                      {tr('Voir les contrôles', 'View controls')}
                    </button>
                    <button
                      onClick={() => download(f)}
                      disabled={generate.isPending}
                      className="h-8 px-3 rounded-[9px] text-[12.5px] font-semibold text-ink inline-flex items-center justify-center gap-1.5 hover:bg-hover transition-colors disabled:opacity-60"
                      style={{ border: '1px solid var(--border-strong)' }}
                      title={L.exportPdf}
                    >
                      <Download size={14} />
                    </button>
                    {canDelete && (
                      <button
                        onClick={() => remove(f)}
                        className="h-8 px-3 rounded-[9px] inline-flex items-center justify-center transition-colors hover:brightness-110"
                        style={{
                          border: '1px solid color-mix(in srgb,var(--critical) 30%,transparent)',
                          background: 'color-mix(in srgb,var(--critical) 10%,transparent)',
                          color: 'var(--critical)',
                        }}
                        title={tr('Supprimer le référentiel', 'Delete framework')}
                      >
                        <Trash2 size={14} />
                      </button>
                    )}
                  </div>
                </Card>
              );
            })}
          </div>
        </>
      )}

      {modal === 'create' && (
        <CreateFrameworkDialog
          onClose={() => setModal(null)}
          onCreated={(id) => navigate(`/compliance/frameworks/${id}`)}
        />
      )}
      {modal === 'import' && (
        <ImportFrameworkDialog
          onClose={() => setModal(null)}
          onImported={(id) => navigate(`/compliance/frameworks/${id}`)}
        />
      )}

      <ImpactDialog
        open={!!toDelete}
        title={tr('Supprimer ce référentiel ?', 'Delete this framework?')}
        subject={toDelete?.name ?? ''}
        description={tr(
          'Action irréversible. Voici ce qui sera supprimé :',
          'This cannot be undone. Here is what will be removed:',
        )}
        impacts={[
          {
            label: tr('Contrôles supprimés', 'Controls removed'),
            detail: String(toDelete?.total ?? 0),
          },
          {
            label: tr('Preuves rattachées perdues', 'Attached evidence lost'),
            detail: tr('toutes', 'all'),
          },
        ]}
        alternatives={[
          {
            label: tr('Exporter le rapport avant de supprimer', 'Export the report first'),
            description: tr(
              'Gardez une trace PDF de la conformité actuelle.',
              'Keep a PDF record of the current posture.',
            ),
            onClick: () => {
              if (toDelete) download(toDelete);
            },
          },
        ]}
        confirmLabel={tr('Supprimer définitivement', 'Delete permanently')}
        cancelLabel={tr('Annuler', 'Cancel')}
        onConfirm={confirmRemove}
        onClose={() => setToDelete(null)}
      />
    </PageFrame>
  );
}

function HubLink({
  icon: Icon,
  label,
  onClick,
}: {
  icon: React.ComponentType<{ size?: number }>;
  label: string;
  onClick: () => void;
}) {
  return (
    <button
      onClick={onClick}
      className="inline-flex items-center gap-2 h-8 px-3 rounded-[10px] text-[12.5px] font-semibold text-ink-soft hover:text-ink transition-colors"
      style={{ border: '1px solid var(--border-strong)', background: 'var(--bg-elevated)' }}
    >
      <Icon size={14} />
      {label}
    </button>
  );
}
