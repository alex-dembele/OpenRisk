// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// The reports library: what has been produced, what state each document is in,
// and the two things you do with one — download it, or move it through review.
//
// Every row is a real document with a real hash. Nothing here is a tile that
// navigates somewhere else to do the work.

import { localeTag } from '../../i18n/locales';
import { useState } from 'react';
import {
  FileText,
  Plus,
  Download,
  Trash2,
  ShieldCheck,
  Clock,
  Loader2,
  AlertTriangle,
  Check,
  Send,
} from 'lucide-react';
import { PageFrame, PageHeader, Btn, Card, Chip, SkeletonRows, ErrorState } from '../../shared/ui';
import { EmptyState } from '../../shared/EmptyState';
import { useUIStore } from '../../store/uiStore';
import { useAuthStore } from '../../hooks/useAuthStore';
import { useToast } from '../../hooks/useToast';
import { reportService } from '../../services/reportService';
import {
  useReports,
  useCreateReport,
  useTransitionReport,
  useDeleteReport,
  useVerifyReport,
} from './useReports';
import { ReportConfigurator } from './ReportConfigurator';
import type { Report, ReportLifecycle } from '../../types/report';

const LIFECYCLE_META: Record<ReportLifecycle, { color: string; fr: string; en: string }> = {
  draft: { color: 'var(--ink-muted)', fr: 'Brouillon', en: 'Draft' },
  in_review: { color: 'var(--medium)', fr: 'En relecture', en: 'In review' },
  approved: { color: 'var(--low)', fr: 'Approuvé', en: 'Approved' },
  published: { color: 'var(--accent-500)', fr: 'Publié', en: 'Published' },
};

/** The next step in the lifecycle, and what to call the button. */
const NEXT_STEP: Partial<Record<ReportLifecycle, { to: ReportLifecycle; fr: string; en: string }>> =
  {
    draft: { to: 'in_review', fr: 'Envoyer en relecture', en: 'Send for review' },
    in_review: { to: 'approved', fr: 'Approuver', en: 'Approve' },
    approved: { to: 'published', fr: 'Publier', en: 'Publish' },
  };

export function ReportsLibraryPage() {
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const toast = useToast();
  const hasPermission = useAuthStore((s) => s.hasPermission);
  const canApprove = hasPermission('reports:board:approve');

  const [lifecycle, setLifecycle] = useState<ReportLifecycle | 'all'>('all');
  const [configuring, setConfiguring] = useState(false);

  const { data, isLoading, error, refetch } = useReports({
    lifecycle: lifecycle === 'all' ? undefined : lifecycle,
    limit: 50,
  });
  const createReport = useCreateReport();
  const transition = useTransitionReport();
  const remove = useDeleteReport();
  const verify = useVerifyReport();

  async function handleDownload(r: Report) {
    try {
      const { contentHash } = await reportService.download(r.id, r.filename);
      toast.success(
        contentHash
          ? tr(
              `Téléchargé · empreinte ${contentHash.slice(0, 16)}`,
              `Downloaded · hash ${contentHash.slice(0, 16)}`,
            )
          : tr('Téléchargé', 'Downloaded'),
      );
    } catch {
      toast.error(tr('Téléchargement impossible', 'Could not download'));
    }
  }

  async function handleVerify(r: Report) {
    const result = await verify.mutateAsync(r.id);
    if (result.intact) {
      toast.success(tr('Document intact', 'Document intact'));
    } else {
      toast.error(
        tr(
          'Le document ne correspond plus à son empreinte',
          'The document no longer matches its hash',
        ),
      );
    }
  }

  return (
    <PageFrame wide>
      <PageHeader
        title={tr('Rapports', 'Reports')}
        count={data ? `${data.total}` : null}
        actions={
          <Btn
            icon={Plus}
            onClick={() => setConfiguring(true)}
            label={tr('Générer un rapport', 'Generate a report')}
          />
        }
      />

      <div className="flex flex-wrap gap-2 mb-4">
        {(['all', 'draft', 'in_review', 'approved', 'published'] as const).map((k) => (
          <Chip
            key={k}
            label={k === 'all' ? tr('Tous', 'All') : tr(LIFECYCLE_META[k].fr, LIFECYCLE_META[k].en)}
            active={lifecycle === k}
            color={k === 'all' ? undefined : LIFECYCLE_META[k].color}
            onClick={() => setLifecycle(k)}
          />
        ))}
      </div>

      {isLoading ? (
        <SkeletonRows rows={5} />
      ) : error ? (
        <ErrorState
          title={tr('Impossible de charger les rapports', 'Could not load reports')}
          onRetry={() => refetch()}
        />
      ) : (data?.items.length ?? 0) === 0 ? (
        <EmptyState
          variant="first-use"
          icon={FileText}
          title={tr('Aucun rapport', 'No report yet')}
          description={tr(
            'Choisissez un type, une période, une langue et un format. La génération se fait en arrière-plan.',
            'Choose a type, a period, a language and a format. Generation runs in the background.',
          )}
          primaryAction={
            <Btn
              icon={Plus}
              onClick={() => setConfiguring(true)}
              label={tr('Générer un rapport', 'Generate a report')}
            />
          }
        />
      ) : (
        <div className="space-y-2">
          {data!.items.map((r, i) => {
            const meta = LIFECYCLE_META[r.lifecycle];
            const next = NEXT_STEP[r.lifecycle];
            const running = r.run_state === 'queued' || r.run_state === 'running';
            return (
              <Card
                key={r.id}
                className="px-4 py-3 or-fadeup"
                style={{ animationDelay: `${Math.min(i * 20, 260)}ms` }}
              >
                <div className="flex items-start gap-3 flex-wrap">
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2 flex-wrap">
                      <span className="text-ink font-medium text-[13.5px]">{r.title}</span>
                      <span
                        className="px-2 py-0.5 rounded-full text-[11.5px] font-medium"
                        style={{
                          color: meta.color,
                          background: `color-mix(in srgb, ${meta.color} 12%, transparent)`,
                        }}
                      >
                        {tr(meta.fr, meta.en)}
                      </span>
                      <span className="text-[11.5px] text-ink-muted uppercase">{r.format}</span>
                      <span className="text-[11.5px] text-ink-muted">{r.locale.toUpperCase()}</span>
                      {r.version > 1 ? (
                        <span className="text-[11.5px] text-ink-muted">v{r.version}</span>
                      ) : null}
                    </div>

                    <div className="text-[12px] text-ink-muted mt-1 flex items-center gap-2 flex-wrap">
                      <span>
                        {new Date(r.created_at).toLocaleString(localeTag(lang))}
                      </span>
                      {r.requested_by_email ? <span>· {r.requested_by_email}</span> : null}
                      <span>
                        · {r.template_key} v{r.template_version}
                      </span>
                      {r.content_fingerprint ? (
                        <span
                          className="font-mono"
                          title={tr(
                            "Empreinte du contenu, imprimée sur le document. Elle identifie ce que le rapport dit ; l'empreinte du fichier, elle, vérifie que les octets n'ont pas bougé.",
                            'Content fingerprint, printed on the document. It identifies what the report says; the file hash verifies the bytes have not moved.',
                          )}
                        >
                          · {r.content_fingerprint.slice(0, 16)}
                        </span>
                      ) : null}
                    </div>

                    {/* Real movement, not an indeterminate spinner. */}
                    {running ? (
                      <div className="mt-2 flex items-center gap-2">
                        <div className="h-1.5 rounded-full bg-surface-3 flex-1 max-w-[240px] overflow-hidden">
                          <div
                            className="h-full rounded-full transition-[width] duration-500"
                            style={{ width: `${r.progress}%`, background: 'var(--accent)' }}
                          />
                        </div>
                        <span className="text-[11.5px] text-ink-muted flex items-center gap-1">
                          <Loader2 size={11} className="animate-spin" />
                          {r.step || tr('en cours', 'working')} · {r.progress}%
                        </span>
                      </div>
                    ) : null}

                    {r.run_state === 'failed' ? (
                      <div
                        className="mt-2 text-[12px] flex items-start gap-1.5"
                        style={{ color: 'var(--critical)' }}
                      >
                        <AlertTriangle size={13} className="mt-0.5 shrink-0" />
                        <span>{r.error || tr('Échec de génération', 'Generation failed')}</span>
                      </div>
                    ) : null}
                  </div>

                  <div className="flex items-center gap-1.5 shrink-0">
                    {r.run_state === 'succeeded' ? (
                      <>
                        <Btn
                          icon={Download}
                          onClick={() => handleDownload(r)}
                          label={tr('Télécharger', 'Download')}
                        />
                        <button
                          className="p-1.5 rounded-md hover:bg-surface-3 text-ink-muted"
                          title={tr("Vérifier l'intégrité", 'Verify integrity')}
                          onClick={() => handleVerify(r)}
                        >
                          <ShieldCheck size={15} />
                        </button>
                        {next && canApprove ? (
                          <Btn
                            icon={next.to === 'published' ? Send : Check}
                            onClick={() =>
                              transition.mutate(
                                { id: r.id, to: next.to },
                                {
                                  onSuccess: () =>
                                    toast.success(tr('Rapport mis à jour', 'Report updated')),
                                  onError: (e) =>
                                    toast.error(
                                      e instanceof Error ? e.message : tr('Refusé', 'Refused'),
                                    ),
                                },
                              )
                            }
                            label={tr(next.fr, next.en)}
                          />
                        ) : null}
                      </>
                    ) : running ? (
                      <span className="text-[12px] text-ink-muted flex items-center gap-1">
                        <Clock size={13} /> {tr('En file', 'Queued')}
                      </span>
                    ) : null}

                    {/* Published documents are frozen: people already hold them. */}
                    {r.lifecycle !== 'published' ? (
                      <button
                        className="p-1.5 rounded-md hover:bg-surface-3 text-ink-muted"
                        title={tr('Supprimer', 'Delete')}
                        onClick={() =>
                          remove.mutate(r.id, {
                            onSuccess: () =>
                              toast.success(tr('Rapport supprimé', 'Report deleted')),
                            onError: (e) =>
                              toast.error(e instanceof Error ? e.message : tr('Refusé', 'Refused')),
                          })
                        }
                      >
                        <Trash2 size={15} />
                      </button>
                    ) : null}
                  </div>
                </div>
              </Card>
            );
          })}
        </div>
      )}

      {configuring ? (
        <ReportConfigurator
          onClose={() => setConfiguring(false)}
          pending={createReport.isPending}
          onGenerate={async (input) => {
            await createReport.mutateAsync(input);
            toast.success(
              tr(
                'Rapport en cours de génération — il apparaîtra ici.',
                'Report is being generated — it will appear here.',
              ),
            );
            setConfiguring(false);
          }}
        />
      ) : null}
    </PageFrame>
  );
}
