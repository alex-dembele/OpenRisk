// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// Generated report — /reports/jobs/:jobId.
//
// Where a report request lands. Before this page existed, "generate a compliance
// report" navigated to the Reports screen, whose Compliance tile navigated back
// to Compliance: a closed loop that never produced a document. The request now
// terminates here, on the artifact, with a download and a way back.

import { localeTag } from '../../i18n/locales';
import { useParams, Link } from 'react-router';
import { Download, RefreshCw, FileText, CheckCircle2, AlertTriangle } from 'lucide-react';
import { toast } from 'sonner';
import { DetailPage, DetailField } from '../../shared/DetailPage';
import { Btn, Card } from '../../shared/ui';
import { useUIStore } from '../../store/uiStore';
import { useReportJob } from './useReportJobs';
import { reportJobService } from './reportJobService';
import type { LocaleCode } from '../../i18n/locales';

function formatBytes(n: number | undefined, lang: LocaleCode): string {
  if (!n) return '—';
  const kb = n / 1024;
  if (kb < 1024) return `${kb.toFixed(0)} ${lang === 'fr' ? 'Ko' : 'KB'}`;
  return `${(kb / 1024).toFixed(1)} ${lang === 'fr' ? 'Mo' : 'MB'}`;
}

export function ReportJobPage() {
  const { jobId } = useParams<{ jobId: string }>();
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const { job, isLoading, isError } = useReportJob(jobId);

  const running = job?.status === 'queued' || job?.status === 'running';
  const failed = job?.status === 'failed';
  const done = job?.status === 'succeeded';

  const download = async () => {
    if (!job) return;
    try {
      await reportJobService.download(job);
    } catch {
      toast.error(tr('Téléchargement échoué', 'Download failed'));
    }
  };

  return (
    <DetailPage
      title={job?.title || tr('Rapport', 'Report')}
      backLabel={tr('Rapports', 'Reports')}
      loading={isLoading}
      error={isError ? new Error('load failed') : undefined}
      notFound={!isLoading && !isError && !job}
      subtitle={
        job ? (
          <span className="text-ink-muted">
            {new Date(job.created_at).toLocaleString(localeTag(lang))}
          </span>
        ) : null
      }
      actions={
        done ? (
          <Btn label={tr('Télécharger', 'Download')} icon={Download} primary onClick={download} />
        ) : null
      }
    >
      {job && (
        <>
          <Card style={{ padding: '22px 24px' }}>
            <div className="flex items-start gap-4">
              <div
                className="w-12 h-12 rounded-xl flex items-center justify-center shrink-0"
                style={{
                  background: failed
                    ? 'color-mix(in srgb, var(--critical) 12%, transparent)'
                    : done
                      ? 'color-mix(in srgb, var(--low) 12%, transparent)'
                      : 'var(--bg-hover)',
                  color: failed ? 'var(--critical)' : done ? 'var(--low)' : 'var(--fg-muted)',
                }}
              >
                {failed ? (
                  <AlertTriangle size={22} />
                ) : done ? (
                  <CheckCircle2 size={22} />
                ) : (
                  <RefreshCw size={22} className="animate-spin" />
                )}
              </div>

              <div className="flex-1 min-w-0">
                {running && (
                  <>
                    <div className="text-[14px] font-semibold text-ink mb-1">
                      {tr('Génération en cours…', 'Generating…')}
                    </div>
                    <div className="text-[12.5px] text-ink-soft">
                      {tr(
                        'Cette page se mettra à jour automatiquement.',
                        'This page will update on its own.',
                      )}
                    </div>
                  </>
                )}
                {done && (
                  <>
                    <div className="text-[14px] font-semibold text-ink mb-1">
                      {tr('Rapport prêt', 'Report ready')}
                    </div>
                    <div className="text-[12.5px] text-ink-soft">
                      {tr(
                        'Ce document est figé à la date de génération : le retélécharger renvoie exactement le même fichier.',
                        'This document is frozen at generation time: re-downloading returns exactly the same file.',
                      )}
                    </div>
                  </>
                )}
                {failed && (
                  <>
                    <div className="text-[14px] font-semibold text-ink mb-1">
                      {tr('Génération échouée', 'Generation failed')}
                    </div>
                    <div className="text-[12.5px] text-ink-soft">
                      {job.error || tr('Raison inconnue.', 'Unknown reason.')}
                    </div>
                  </>
                )}
              </div>
            </div>
          </Card>

          <Card style={{ padding: '4px 18px 14px', marginTop: 16 }}>
            <DetailField label={tr('Fichier', 'File')}>{job.filename}</DetailField>
            <DetailField label={tr('Taille', 'Size')}>
              {formatBytes(job.size_bytes, lang)}
            </DetailField>
            <DetailField label={tr('Généré le', 'Generated at')}>
              {job.completed_at
                ? new Date(job.completed_at).toLocaleString(localeTag(lang))
                : '—'}
            </DetailField>
          </Card>

          {/* Deliberately NOT a link back to Compliance's report button — that is
              the round trip. The only onward paths are the artifact itself and
              the list of past reports. */}
          <div className="mt-5 flex items-center gap-4">
            <Link
              to="/reports"
              className="inline-flex items-center gap-1.5 text-[12.5px] font-semibold text-accent hover:underline"
            >
              <FileText size={14} /> {tr('Tous les rapports', 'All reports')}
            </Link>
          </div>
        </>
      )}
    </DetailPage>
  );
}
