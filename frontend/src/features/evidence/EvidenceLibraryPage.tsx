// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// The evidence library: every proof artifact the tenant holds, what it covers,
// and whether it is still good.
//
// It is a library rather than a list of attachments because the same
// certificate answers a dozen controls across several frameworks. The screen
// leads with the status tabs, because the question people arrive with is not
// "what do we have" but "what is about to stop counting".

import { useMemo, useState } from 'react';
import {
  FileText,
  Camera,
  Settings2,
  BadgeCheck,
  ScrollText,
  Plus,
  Link2,
  Trash2,
  Check,
  X,
  Download,
  ExternalLink,
  Search,
} from 'lucide-react';
import { PageFrame, PageHeader, Btn, Card, Chip, SkeletonRows, ErrorState } from '../../shared/ui';
import { EmptyState } from '../../shared/EmptyState';
import { useUIStore } from '../../store/uiStore';
import { useAuthStore } from '../../hooks/useAuthStore';
import { useToast } from '../../hooks/useToast';
import {
  useEvidenceLibrary,
  useCreateEvidence,
  useReviewEvidence,
  useDeleteEvidence,
} from './useEvidence';
import { EVIDENCE_STATUS_META, EVIDENCE_TYPE_META, expiryLabel } from './evidenceMeta';
import { EvidenceDrawer } from './EvidenceDrawer';
import { CreateEvidenceModal } from './CreateEvidenceModal';
import { evidenceService } from '../../services/evidenceService';
import type { Evidence, EvidenceStatus, EvidenceType } from '../../types/evidence';
import { localeTag } from '../../i18n/locales';

const TYPE_ICON: Record<EvidenceType, typeof FileText> = {
  document: FileText,
  capture: Camera,
  configuration: Settings2,
  attestation: BadgeCheck,
  log: ScrollText,
};

/** The tabs, in the order they matter to someone arriving with a deadline. */
const TABS: { key: 'all' | EvidenceStatus; fr: string; en: string }[] = [
  { key: 'all', fr: 'Toutes', en: 'All' },
  { key: 'expired', fr: 'Expirées', en: 'Expired' },
  { key: 'expiring_soon', fr: 'Expirent bientôt', en: 'Expiring soon' },
  { key: 'valid', fr: 'Valides', en: 'Valid' },
  { key: 'rejected', fr: 'Rejetées', en: 'Rejected' },
];

export function EvidenceLibraryPage() {
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const toast = useToast();
  const hasPermission = useAuthStore((s) => s.hasPermission);
  const canWrite = hasPermission('compliance:evidences:create');
  const canDelete = hasPermission('compliance:evidences:delete');

  const [tab, setTab] = useState<'all' | EvidenceStatus>('all');
  const [search, setSearch] = useState('');
  const [selected, setSelected] = useState<Evidence | null>(null);
  const [creating, setCreating] = useState(false);

  const { data, isLoading, error, refetch } = useEvidenceLibrary({
    q: search || undefined,
    limit: 200,
  });
  const createEvidence = useCreateEvidence();
  const review = useReviewEvidence();
  const remove = useDeleteEvidence();

  // Status is derived server-side, so the tab filter is a plain client-side
  // partition of what came back — no second query, and no risk of the tab and
  // the row disagreeing about what "expired" means.
  const items = useMemo(() => {
    const all = data?.items ?? [];
    return tab === 'all' ? all : all.filter((e) => e.status === tab);
  }, [data, tab]);

  const summary = data?.summary;
  const tabCount = (key: 'all' | EvidenceStatus): number | undefined => {
    if (!summary) return undefined;
    if (key === 'all') return data?.total;
    return summary[key === 'expiring_soon' ? 'expiring_soon' : key];
  };

  async function handleDownload(e: Evidence) {
    try {
      const blob = await evidenceService.download(e.id);
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = e.filename || e.title;
      document.body.appendChild(a);
      a.click();
      a.remove();
      setTimeout(() => URL.revokeObjectURL(url), 0);
    } catch {
      toast.error(tr('Téléchargement impossible', 'Could not download'));
    }
  }

  return (
    <PageFrame wide>
      <PageHeader
        title={tr('Bibliothèque de preuves', 'Evidence library')}
        count={data ? `${data.total}` : null}
        actions={
          canWrite ? (
            <Btn
              icon={Plus}
              onClick={() => setCreating(true)}
              label={tr('Enregistrer une preuve', 'Record evidence')}
            />
          ) : null
        }
      />

      {/* The sentence the screen exists to say. */}
      <p className="text-[13px] text-ink-muted -mt-2 mb-4 max-w-[68ch]">
        {tr(
          'Une preuve enregistrée ici peut justifier plusieurs contrôles, dans plusieurs référentiels : inutile de la téléverser à nouveau. Une preuve expirée ou rejetée cesse de justifier quoi que ce soit.',
          'Evidence recorded here can substantiate several controls across several frameworks — no need to upload it again. Expired or rejected evidence stops substantiating anything.',
        )}
      </p>

      <div className="flex flex-wrap items-center gap-2 mb-4">
        {TABS.map((t) => {
          const n = tabCount(t.key);
          const meta = t.key === 'all' ? undefined : EVIDENCE_STATUS_META[t.key];
          return (
            <Chip
              key={t.key}
              label={`${tr(t.fr, t.en)}${n !== undefined ? ` · ${n}` : ''}`}
              active={tab === t.key}
              color={meta?.color}
              onClick={() => setTab(t.key)}
            />
          );
        })}
        <div className="relative ml-auto">
          <Search size={14} className="absolute left-2.5 top-1/2 -translate-y-1/2 text-ink-muted" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder={tr('Rechercher…', 'Search…')}
            className="pl-8 pr-3 py-1.5 text-[13px] rounded-lg bg-surface border border-line text-ink w-[220px]"
          />
        </div>
      </div>

      {isLoading ? (
        <SkeletonRows rows={6} />
      ) : error ? (
        <ErrorState
          title={tr('Impossible de charger la bibliothèque', 'Could not load the library')}
          description={tr(
            'Réessayez, ou contactez un administrateur.',
            'Try again, or contact an administrator.',
          )}
          onRetry={() => refetch()}
        />
      ) : items.length === 0 ? (
        <EmptyState
          variant={search || tab !== 'all' ? 'no-results' : 'first-use'}
          icon={FileText}
          title={
            search || tab !== 'all'
              ? tr('Aucune preuve ne correspond', 'No evidence matches')
              : tr('Aucune preuve enregistrée', 'No evidence recorded yet')
          }
          description={
            search || tab !== 'all'
              ? tr('Changez de filtre ou de recherche.', 'Change the filter or the search.')
              : tr(
                  "Enregistrez un document, une capture, un export de configuration ou une attestation, puis rattachez-le aux contrôles qu'il justifie.",
                  'Record a document, a capture, a configuration export or an attestation, then attach it to the controls it substantiates.',
                )
          }
          primaryAction={
            canWrite && !search && tab === 'all' ? (
              <Btn
                icon={Plus}
                onClick={() => setCreating(true)}
                label={tr('Enregistrer une preuve', 'Record evidence')}
              />
            ) : undefined
          }
        />
      ) : (
        <Card className="overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full text-[13px] min-w-[860px]">
              <thead>
                <tr className="text-left text-ink-muted border-b border-line">
                  <th className="px-4 py-2.5 font-medium">{tr('Preuve', 'Evidence')}</th>
                  <th className="px-4 py-2.5 font-medium">{tr('Type', 'Type')}</th>
                  <th className="px-4 py-2.5 font-medium">{tr('Couvre', 'Covers')}</th>
                  <th className="px-4 py-2.5 font-medium">{tr('Collectée', 'Collected')}</th>
                  <th className="px-4 py-2.5 font-medium">{tr('Statut', 'Status')}</th>
                  <th className="px-4 py-2.5" />
                </tr>
              </thead>
              <tbody>
                {items.map((e, i) => {
                  const meta = EVIDENCE_STATUS_META[e.status];
                  const Icon = TYPE_ICON[e.type] ?? FileText;
                  const expiry = expiryLabel(lang, e.days_until_expiry);
                  return (
                    <tr
                      key={e.id}
                      className="border-b border-line last:border-0 hover:bg-surface-2 cursor-pointer or-fadeup"
                      style={{ animationDelay: `${Math.min(i * 18, 260)}ms` }}
                      onClick={() => setSelected(e)}
                    >
                      <td className="px-4 py-3">
                        <div className="flex items-start gap-2.5">
                          <Icon size={15} className="text-ink-muted mt-0.5 shrink-0" />
                          <div className="min-w-0">
                            <div className="text-ink font-medium truncate">{e.title}</div>
                            {e.description ? (
                              <div className="text-ink-muted text-[12px] truncate max-w-[42ch]">
                                {e.description}
                              </div>
                            ) : null}
                          </div>
                        </div>
                      </td>
                      <td className="px-4 py-3 text-ink-muted whitespace-nowrap">
                        {tr(
                          EVIDENCE_TYPE_META[e.type]?.fr ?? e.type,
                          EVIDENCE_TYPE_META[e.type]?.en ?? e.type,
                        )}
                      </td>
                      <td className="px-4 py-3">
                        {/* The count IS the point of the library: one artifact,
                            N controls. Showing "1 contrôle" is as informative as
                            showing twelve — it tells you reuse is available. */}
                        <span className="inline-flex items-center gap-1.5 text-ink-muted">
                          <Link2 size={13} />
                          {e.control_ids.length}{' '}
                          {e.control_ids.length === 1
                            ? tr('contrôle', 'control')
                            : tr('contrôles', 'controls')}
                        </span>
                        {e.controls && e.controls.length > 0 ? (
                          <div className="text-[11.5px] text-ink-muted/80 truncate max-w-[26ch]">
                            {[...new Set(e.controls.map((c) => c.framework_name))].join(' · ')}
                          </div>
                        ) : null}
                      </td>
                      <td className="px-4 py-3 text-ink-muted whitespace-nowrap">
                        {new Date(e.collected_at).toLocaleDateString(
                          localeTag(lang),
                        )}
                      </td>
                      <td className="px-4 py-3">
                        <span
                          className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-[12px] font-medium"
                          style={{
                            color: meta.color,
                            background: `color-mix(in srgb, ${meta.color} 12%, transparent)`,
                          }}
                          title={tr(meta.hint.fr, meta.hint.en)}
                        >
                          {tr(meta.fr, meta.en)}
                        </span>
                        {expiry ? (
                          <div className="text-[11.5px] text-ink-muted mt-0.5">{expiry}</div>
                        ) : null}
                      </td>
                      <td className="px-4 py-3">
                        <div
                          className="flex items-center justify-end gap-1"
                          onClick={(ev) => ev.stopPropagation()}
                        >
                          {e.file_ref ? (
                            <button
                              className="p-1.5 rounded-md hover:bg-surface-3 text-ink-muted"
                              title={tr('Télécharger', 'Download')}
                              onClick={() => handleDownload(e)}
                            >
                              <Download size={14} />
                            </button>
                          ) : e.external_url ? (
                            <a
                              className="p-1.5 rounded-md hover:bg-surface-3 text-ink-muted inline-flex"
                              href={e.external_url}
                              target="_blank"
                              rel="noreferrer noopener"
                              title={tr('Ouvrir le lien', 'Open the link')}
                            >
                              <ExternalLink size={14} />
                            </a>
                          ) : null}
                          {canWrite && e.review !== 'accepted' ? (
                            <button
                              className="p-1.5 rounded-md hover:bg-surface-3"
                              style={{ color: 'var(--low)' }}
                              title={tr('Accepter', 'Accept')}
                              onClick={() =>
                                review.mutate(
                                  { id: e.id, review: 'accepted' },
                                  {
                                    onSuccess: () =>
                                      toast.success(tr('Preuve acceptée', 'Evidence accepted')),
                                  },
                                )
                              }
                            >
                              <Check size={14} />
                            </button>
                          ) : null}
                          {canWrite && e.review !== 'rejected' ? (
                            <button
                              className="p-1.5 rounded-md hover:bg-surface-3"
                              style={{ color: 'var(--critical)' }}
                              title={tr('Rejeter', 'Reject')}
                              onClick={() => setSelected(e)}
                            >
                              <X size={14} />
                            </button>
                          ) : null}
                          {canDelete ? (
                            <button
                              className="p-1.5 rounded-md hover:bg-surface-3 text-ink-muted"
                              title={tr('Supprimer', 'Delete')}
                              onClick={() =>
                                remove.mutate(e.id, {
                                  onSuccess: () =>
                                    toast.success(tr('Preuve supprimée', 'Evidence deleted')),
                                })
                              }
                            >
                              <Trash2 size={14} />
                            </button>
                          ) : null}
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </Card>
      )}

      {selected ? (
        <EvidenceDrawer
          evidenceId={selected.id}
          onClose={() => setSelected(null)}
          canWrite={canWrite}
        />
      ) : null}

      {creating ? (
        <CreateEvidenceModal
          onClose={() => setCreating(false)}
          onSubmit={async (input) => {
            await createEvidence.mutateAsync(input);
            toast.success(tr('Preuve enregistrée', 'Evidence recorded'));
            setCreating(false);
          }}
          pending={createEvidence.isPending}
        />
      ) : null}
    </PageFrame>
  );
}
