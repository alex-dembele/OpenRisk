// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// "Preuves manquantes" — the worklist that makes this a GRC tool rather than a
// document store.
//
// It separates two states that tools counting attachments collapse into one: a
// control nobody has ever evidenced, and a control whose proof has expired or
// been rejected. The second is the dangerous one — it reads green everywhere
// else in the product and falls over at the first question an auditor asks —
// and it is usually the smaller job, since the document exists and only needs
// refreshing.

import { useMemo, useState } from 'react';
import { AlertTriangle, RefreshCw, FileWarning, ShieldCheck, ArrowLeft } from 'lucide-react';
import { useNavigate, useSearchParams } from 'react-router';
import {
  PageFrame,
  PageHeader,
  Btn,
  Card,
  Chip,
  RingGauge,
  SkeletonRows,
  ErrorState,
} from '../../shared/ui';
import { EmptyState } from '../../shared/EmptyState';
import { useUIStore } from '../../store/uiStore';
import { useAuthStore } from '../../hooks/useAuthStore';
import { useToast } from '../../hooks/useToast';
import { useMissingEvidence, useCreateEvidence } from './useEvidence';
import { MISSING_KIND_META } from './evidenceMeta';
import { CreateEvidenceModal } from './CreateEvidenceModal';
import type { MissingControl, MissingKind } from '../../types/evidence';
import { localeTag } from '../../i18n/locales';

const FILTERS: { key: 'all' | MissingKind; fr: string; en: string }[] = [
  { key: 'all', fr: 'Tout', en: 'All' },
  { key: 'no_evidence', fr: 'Aucune preuve', en: 'No evidence' },
  { key: 'stale_evidence', fr: 'Preuve périmée', en: 'Stale evidence' },
  { key: 'expiring_soon', fr: 'Expire bientôt', en: 'Expiring soon' },
];

export function MissingEvidencePage() {
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const navigate = useNavigate();
  const toast = useToast();
  const [params] = useSearchParams();
  const frameworkId = params.get('framework_id') ?? undefined;

  const hasPermission = useAuthStore((s) => s.hasPermission);
  const canWrite = hasPermission('compliance:evidences:create');

  const [kind, setKind] = useState<'all' | MissingKind>('all');
  const [collecting, setCollecting] = useState<MissingControl | null>(null);

  const { data, isLoading, error, refetch } = useMissingEvidence(frameworkId);
  const createEvidence = useCreateEvidence();

  const totals = useMemo(() => {
    const frameworks = data ?? [];
    return frameworks.reduce(
      (acc, f) => ({
        controls: acc.controls + f.total_controls,
        covered: acc.covered + f.covered_controls,
        none: acc.none + f.no_evidence,
        stale: acc.stale + f.stale_evidence,
        expiring: acc.expiring + f.expiring_soon,
      }),
      { controls: 0, covered: 0, none: 0, stale: 0, expiring: 0 },
    );
  }, [data]);

  const overall = totals.controls > 0 ? (totals.covered / totals.controls) * 100 : 0;

  return (
    <PageFrame wide>
      <PageHeader
        title={tr('Preuves manquantes', 'Missing evidence')}
        actions={
          <Btn
            icon={ArrowLeft}
            onClick={() => navigate('/compliance')}
            label={tr('Conformité', 'Compliance')}
          />
        }
      />

      {isLoading ? (
        <SkeletonRows rows={5} />
      ) : error ? (
        <ErrorState
          title={tr('Impossible de charger la liste', 'Could not load the worklist')}
          onRetry={() => refetch()}
        />
      ) : !data || data.length === 0 ? (
        <EmptyState
          variant="first-use"
          icon={ShieldCheck}
          title={tr('Aucun référentiel suivi', 'No framework tracked')}
          description={tr(
            'Importez un référentiel pour voir quelles preuves vous manquent.',
            'Import a framework to see which evidence you are missing.',
          )}
          primaryAction={
            <Btn onClick={() => navigate('/compliance')} label={tr('Conformité', 'Compliance')} />
          }
        />
      ) : (
        <>
          {/* The three numbers that decide what to do next, and the distinction
              the whole page exists for. */}
          <div className="grid grid-cols-1 sm:grid-cols-4 gap-3 mb-5">
            <Card className="p-4 flex items-center gap-3">
              <RingGauge value={overall} size={54} color="var(--accent)" thickness={6}>
                <span className="text-[12px] font-semibold text-ink">{Math.round(overall)}%</span>
              </RingGauge>
              <div>
                <div className="text-[12px] text-ink-muted">
                  {tr('Couverts par une preuve valide', 'Covered by valid evidence')}
                </div>
                <div className="text-[15px] text-ink font-semibold">
                  {totals.covered} / {totals.controls}
                </div>
              </div>
            </Card>
            <StatCard
              label={tr('Aucune preuve', 'No evidence')}
              value={totals.none}
              color={MISSING_KIND_META.no_evidence.color}
              hint={tr('À collecter', 'To collect')}
            />
            <StatCard
              label={tr('Preuve périmée', 'Stale evidence')}
              value={totals.stale}
              color={MISSING_KIND_META.stale_evidence.color}
              hint={tr('Document existant à renouveler', 'Document exists, needs refreshing')}
            />
            <StatCard
              label={tr('Expire bientôt', 'Expiring soon')}
              value={totals.expiring}
              color={MISSING_KIND_META.expiring_soon.color}
              hint={tr('Encore valable', 'Still valid')}
            />
          </div>

          <div className="flex flex-wrap gap-2 mb-4">
            {FILTERS.map((f) => (
              <Chip
                key={f.key}
                label={tr(f.fr, f.en)}
                active={kind === f.key}
                color={f.key === 'all' ? undefined : MISSING_KIND_META[f.key].color}
                onClick={() => setKind(f.key)}
              />
            ))}
          </div>

          <div className="space-y-5">
            {data.map((fw) => {
              const rows = kind === 'all' ? fw.missing : fw.missing.filter((m) => m.kind === kind);
              if (rows.length === 0 && kind !== 'all') return null;
              return (
                <Card key={fw.framework_id} className="overflow-hidden">
                  <div className="px-4 py-3 border-b border-line flex items-center justify-between gap-3">
                    <div className="min-w-0">
                      <div className="text-ink font-medium truncate">
                        {fw.framework_name} <span className="text-ink-muted">{fw.version}</span>
                      </div>
                      <div className="text-[12px] text-ink-muted">
                        {fw.covered_controls}/{fw.total_controls} {tr('couverts', 'covered')} ·{' '}
                        {Math.round(fw.percent_covered)}%
                      </div>
                    </div>
                    <Btn
                      onClick={() => navigate(`/compliance/frameworks/${fw.framework_id}`)}
                      label={tr('Ouvrir', 'Open')}
                    />
                  </div>

                  {rows.length === 0 ? (
                    <div className="px-4 py-6 text-center text-[13px] text-ink-muted">
                      {tr(
                        'Chaque contrôle applicable est justifié par une preuve valide.',
                        'Every applicable control is substantiated by valid evidence.',
                      )}
                    </div>
                  ) : (
                    <ul className="divide-y divide-line">
                      {rows.map((m) => {
                        const meta = MISSING_KIND_META[m.kind];
                        return (
                          <li key={m.control_id} className="px-4 py-3 flex items-start gap-3">
                            <span
                              className="mt-1 w-1.5 h-1.5 rounded-full shrink-0"
                              style={{ background: meta.color }}
                            />
                            <div className="min-w-0 flex-1">
                              <div className="text-[13px] text-ink">
                                <span className="font-mono text-[12px] text-ink-muted mr-1.5">
                                  {m.reference_code}
                                </span>
                                {m.name}
                              </div>
                              <div className="text-[12px] mt-0.5" style={{ color: meta.color }}>
                                {tr(meta.fr, meta.en)}
                                {/* The gap between the two counts IS the story. */}
                                {m.total_evidence > 0 ? (
                                  <span className="text-ink-muted">
                                    {' '}
                                    · {m.total_evidence} {tr('preuve(s) rattachée(s)', 'attached')},{' '}
                                    {m.covering_evidence} {tr('valide(s)', 'valid')}
                                  </span>
                                ) : null}
                                {m.nearest_expiry ? (
                                  <span className="text-ink-muted">
                                    {' '}
                                    · {tr('échéance', 'due')}{' '}
                                    {new Date(m.nearest_expiry).toLocaleDateString(
                                      localeTag(lang),
                                    )}
                                  </span>
                                ) : null}
                              </div>
                            </div>
                            {canWrite ? (
                              <Btn
                                icon={m.kind === 'no_evidence' ? FileWarning : RefreshCw}
                                onClick={() => setCollecting(m)}
                                label={tr(meta.action.fr, meta.action.en)}
                              />
                            ) : null}
                          </li>
                        );
                      })}
                    </ul>
                  )}
                </Card>
              );
            })}
          </div>
        </>
      )}

      {collecting ? (
        <CreateEvidenceModal
          presetControlIds={[collecting.control_id]}
          onClose={() => setCollecting(null)}
          pending={createEvidence.isPending}
          onSubmit={async (input) => {
            await createEvidence.mutateAsync(input);
            toast.success(tr('Preuve enregistrée', 'Evidence recorded'));
            setCollecting(null);
          }}
        />
      ) : null}
    </PageFrame>
  );
}

function StatCard({
  label,
  value,
  color,
  hint,
}: {
  label: string;
  value: number;
  color: string;
  hint: string;
}) {
  return (
    <Card className="p-4">
      <div className="text-[12px] text-ink-muted">{label}</div>
      <div
        className="text-[22px] font-semibold"
        style={{ color: value > 0 ? color : 'var(--ink)' }}
      >
        {value}
      </div>
      <div className="text-[11.5px] text-ink-muted">{hint}</div>
    </Card>
  );
}
