// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// One artifact: what it is, what it covers, whether it is still good, and the
// two actions a reviewer takes on it.

import { useState } from 'react';
import { X, Link2, Trash2, Check, Calendar, User, Layers } from 'lucide-react';
import { Btn } from '../../shared/ui';
import { useUIStore } from '../../store/uiStore';
import { useToast } from '../../hooks/useToast';
import {
  useEvidence,
  useReviewEvidence,
  useUnlinkEvidence,
  useUpdateEvidence,
} from './useEvidence';
import { EVIDENCE_STATUS_META, EVIDENCE_TYPE_META, expiryLabel } from './evidenceMeta';
import { localeTag } from '../../i18n/locales';

export function EvidenceDrawer({
  evidenceId,
  onClose,
  canWrite,
}: {
  evidenceId: string;
  onClose: () => void;
  canWrite: boolean;
}) {
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const toast = useToast();

  const { data: evidence, isLoading } = useEvidence(evidenceId);
  const review = useReviewEvidence();
  const unlink = useUnlinkEvidence();
  const update = useUpdateEvidence();

  const [rejectNote, setRejectNote] = useState('');
  const [rejecting, setRejecting] = useState(false);
  const [newExpiry, setNewExpiry] = useState('');

  const meta = evidence ? EVIDENCE_STATUS_META[evidence.status] : null;

  return (
    <div className="fixed inset-0 z-50 flex justify-end">
      <div className="absolute inset-0 bg-black/40" onClick={onClose} />
      <aside className="relative w-full max-w-[560px] h-full bg-surface border-l border-line overflow-y-auto or-slidein">
        <header className="sticky top-0 bg-surface border-b border-line px-5 py-4 flex items-start gap-3">
          <div className="min-w-0 flex-1">
            <h2 className="text-ink font-semibold text-[15px] truncate">
              {isLoading ? tr('Chargement…', 'Loading…') : evidence?.title}
            </h2>
            {evidence && meta ? (
              <div className="flex items-center gap-2 mt-1">
                <span
                  className="px-2 py-0.5 rounded-full text-[12px] font-medium"
                  style={{
                    color: meta.color,
                    background: `color-mix(in srgb, ${meta.color} 12%, transparent)`,
                  }}
                >
                  {tr(meta.fr, meta.en)}
                </span>
                <span className="text-[12px] text-ink-muted">
                  {tr(
                    EVIDENCE_TYPE_META[evidence.type]?.fr ?? '',
                    EVIDENCE_TYPE_META[evidence.type]?.en ?? '',
                  )}
                </span>
              </div>
            ) : null}
          </div>
          <button className="p-1.5 rounded-md hover:bg-surface-3 text-ink-muted" onClick={onClose}>
            <X size={16} />
          </button>
        </header>

        {evidence ? (
          <div className="px-5 py-4 space-y-5">
            {/* Why this status, in words. A colour alone does not tell someone
                whether they have work to do. */}
            {meta ? (
              <p className="text-[13px] text-ink-muted">{tr(meta.hint.fr, meta.hint.en)}</p>
            ) : null}

            {evidence.description ? (
              <p className="text-[13.5px] text-ink whitespace-pre-wrap">{evidence.description}</p>
            ) : null}

            <dl className="grid grid-cols-2 gap-x-4 gap-y-3 text-[13px]">
              <div>
                <dt className="text-ink-muted flex items-center gap-1.5">
                  <Calendar size={13} /> {tr('Collectée le', 'Collected on')}
                </dt>
                <dd className="text-ink mt-0.5">
                  {new Date(evidence.collected_at).toLocaleDateString(
                    localeTag(lang),
                  )}
                </dd>
              </div>
              <div>
                <dt className="text-ink-muted flex items-center gap-1.5">
                  <Calendar size={13} /> {tr("Valide jusqu'au", 'Valid until')}
                </dt>
                <dd className="text-ink mt-0.5">
                  {evidence.valid_until
                    ? new Date(evidence.valid_until).toLocaleDateString(
                        localeTag(lang),
                      )
                    : tr("N'expire pas", 'Never expires')}
                  {evidence.days_until_expiry !== undefined ? (
                    <span className="text-ink-muted">
                      {' '}
                      · {expiryLabel(lang, evidence.days_until_expiry)}
                    </span>
                  ) : null}
                </dd>
              </div>
              <div>
                <dt className="text-ink-muted flex items-center gap-1.5">
                  <User size={13} /> {tr('Collectée par', 'Collected by')}
                </dt>
                <dd className="text-ink mt-0.5 truncate">
                  {evidence.collected_by_email || tr('Inconnu', 'Unknown')}
                </dd>
              </div>
              <div>
                <dt className="text-ink-muted flex items-center gap-1.5">
                  <Layers size={13} /> {tr('Source', 'Source')}
                </dt>
                <dd className="text-ink mt-0.5">{evidence.source}</dd>
              </div>
            </dl>

            {evidence.review_note ? (
              <div className="rounded-lg border border-line bg-surface-2 px-3 py-2.5">
                <div className="text-[12px] text-ink-muted mb-0.5">
                  {tr('Avis du relecteur', "Reviewer's note")}
                </div>
                <div className="text-[13px] text-ink">{evidence.review_note}</div>
              </div>
            ) : null}

            {/* Renewing is the action an expiring artifact needs, so it is here
                rather than behind an edit form on another screen. */}
            {canWrite ? (
              <div className="rounded-lg border border-line px-3 py-3">
                <div className="text-[13px] text-ink font-medium mb-2">
                  {tr('Prolonger la validité', 'Extend validity')}
                </div>
                <div className="flex items-center gap-2">
                  <input
                    type="date"
                    value={newExpiry}
                    onChange={(e) => setNewExpiry(e.target.value)}
                    className="px-2.5 py-1.5 text-[13px] rounded-lg bg-surface-2 border border-line text-ink"
                  />
                  <Btn
                    disabled={!newExpiry || update.isPending}
                    onClick={() =>
                      update.mutate(
                        { id: evidence.id, input: { valid_until: newExpiry } },
                        {
                          onSuccess: () => {
                            toast.success(tr('Validité mise à jour', 'Validity updated'));
                            setNewExpiry('');
                          },
                          onError: () =>
                            toast.error(
                              tr('Date refusée par le serveur', 'The server refused that date'),
                            ),
                        },
                      )
                    }
                    label={tr('Enregistrer', 'Save')}
                  />
                </div>
              </div>
            ) : null}

            <section>
              <h3 className="text-[13px] font-medium text-ink mb-2 flex items-center gap-1.5">
                <Link2 size={14} />
                {tr('Contrôles justifiés', 'Controls substantiated')} (
                {evidence.controls?.length ?? 0})
              </h3>
              {evidence.controls && evidence.controls.length > 0 ? (
                <ul className="space-y-1.5">
                  {evidence.controls.map((c) => (
                    <li
                      key={c.control_id}
                      className="flex items-center justify-between gap-2 rounded-lg border border-line px-3 py-2"
                    >
                      <div className="min-w-0">
                        <div className="text-[13px] text-ink truncate">
                          <span className="font-mono text-[12px] text-ink-muted mr-1.5">
                            {c.reference_code}
                          </span>
                          {c.name}
                        </div>
                        <div className="text-[11.5px] text-ink-muted">{c.framework_name}</div>
                      </div>
                      {canWrite ? (
                        <button
                          className="p-1.5 rounded-md hover:bg-surface-3 text-ink-muted shrink-0"
                          title={tr('Détacher', 'Unlink')}
                          onClick={() =>
                            unlink.mutate(
                              { id: evidence.id, controlId: c.control_id },
                              {
                                onSuccess: () =>
                                  toast.success(tr('Contrôle détaché', 'Control unlinked')),
                              },
                            )
                          }
                        >
                          <Trash2 size={13} />
                        </button>
                      ) : null}
                    </li>
                  ))}
                </ul>
              ) : (
                <p className="text-[13px] text-ink-muted">
                  {tr(
                    "Cette preuve n'est rattachée à aucun contrôle : elle ne justifie rien pour l'instant.",
                    'This evidence is attached to no control: it substantiates nothing yet.',
                  )}
                </p>
              )}
            </section>

            {canWrite ? (
              <section className="pt-2 border-t border-line">
                {rejecting ? (
                  <div className="space-y-2">
                    <label className="text-[13px] text-ink font-medium block">
                      {tr('Pourquoi la rejeter ?', 'Why reject it?')}
                    </label>
                    {/* Required, and the server enforces it too: a rejection
                        nobody can act on is a rejection that wastes a cycle. */}
                    <textarea
                      value={rejectNote}
                      onChange={(e) => setRejectNote(e.target.value)}
                      rows={3}
                      placeholder={tr(
                        'ex. document illisible, pages manquantes',
                        'e.g. illegible document, missing pages',
                      )}
                      className="w-full px-3 py-2 text-[13px] rounded-lg bg-surface-2 border border-line text-ink"
                    />
                    <div className="flex gap-2">
                      <Btn
                        danger
                        disabled={!rejectNote.trim() || review.isPending}
                        onClick={() =>
                          review.mutate(
                            { id: evidence.id, review: 'rejected', note: rejectNote },
                            {
                              onSuccess: () => {
                                toast.success(tr('Preuve rejetée', 'Evidence rejected'));
                                setRejecting(false);
                                setRejectNote('');
                              },
                            },
                          )
                        }
                        label={tr('Confirmer le rejet', 'Confirm rejection')}
                      />
                      <Btn onClick={() => setRejecting(false)} label={tr('Annuler', 'Cancel')} />
                    </div>
                  </div>
                ) : (
                  <div className="flex gap-2">
                    {evidence.review !== 'accepted' ? (
                      <Btn
                        icon={Check}
                        onClick={() =>
                          review.mutate(
                            { id: evidence.id, review: 'accepted' },
                            {
                              onSuccess: () =>
                                toast.success(tr('Preuve acceptée', 'Evidence accepted')),
                            },
                          )
                        }
                        label={tr('Accepter', 'Accept')}
                      />
                    ) : null}
                    {evidence.review !== 'rejected' ? (
                      <Btn
                        icon={X}
                        onClick={() => setRejecting(true)}
                        label={tr('Rejeter', 'Reject')}
                      />
                    ) : null}
                  </div>
                )}
              </section>
            ) : null}
          </div>
        ) : null}
      </aside>
    </div>
  );
}
