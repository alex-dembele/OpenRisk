// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// Recording a proof artifact.
//
// A file is optional on purpose: evidence is legitimately a link to a system of
// record or a written statement, and forcing an upload makes people screenshot a
// page rather than point at it. What is NOT optional is at least one of the
// three, because a row asserting a control is covered by nothing is worse than
// no row.

import { useState } from 'react';
import { X, Upload, Paperclip } from 'lucide-react';
import { Btn } from '../../shared/ui';
import { useUIStore } from '../../store/uiStore';
import { useFrameworks, useControls } from '../compliance/useCompliance';
import { EVIDENCE_TYPE_META } from './evidenceMeta';
import type { CreateEvidenceInput, EvidenceType } from '../../types/evidence';

export function CreateEvidenceModal({
  onClose,
  onSubmit,
  pending,
  presetControlIds,
}: {
  onClose: () => void;
  onSubmit: (input: CreateEvidenceInput) => Promise<void>;
  pending?: boolean;
  presetControlIds?: string[];
}) {
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);

  const [title, setTitle] = useState('');
  const [type, setType] = useState<EvidenceType>('document');
  const [description, setDescription] = useState('');
  const [externalUrl, setExternalUrl] = useState('');
  const [collectedAt, setCollectedAt] = useState(new Date().toISOString().slice(0, 10));
  const [validUntil, setValidUntil] = useState('');
  const [file, setFile] = useState<File | null>(null);
  const [frameworkId, setFrameworkId] = useState('');
  const [controlIds, setControlIds] = useState<string[]>(presetControlIds ?? []);
  const [error, setError] = useState<string | null>(null);

  const { frameworks } = useFrameworks();
  const { controls } = useControls(frameworkId || undefined);

  const substance = Boolean(file || externalUrl.trim() || description.trim());
  const canSubmit = Boolean((title.trim() || file) && substance) && !pending;

  async function submit() {
    setError(null);
    try {
      await onSubmit({
        title: title.trim() || file?.name || '',
        type,
        description: description.trim() || undefined,
        external_url: externalUrl.trim() || undefined,
        collected_at: collectedAt || undefined,
        valid_until: validUntil || undefined,
        control_ids: controlIds,
        file: file ?? undefined,
      });
    } catch (e) {
      const message = e instanceof Error ? e.message : String(e);
      setError(message);
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-black/50" onClick={onClose} />
      {/* max-h + internal scroll: the submit button must never be pushed off
          screen on a short viewport, which is a bug this project has already had. */}
      {/* Announced as a dialog and named by its heading; the evidence library
          behind it stays in the accessibility tree otherwise. */}
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="create-evidence-title"
        className="relative w-full max-w-[560px] max-h-[90vh] flex flex-col rounded-xl bg-surface border border-line or-scalein"
      >
        <header className="px-5 py-4 border-b border-line flex items-center justify-between shrink-0">
          <h2 id="create-evidence-title" className="text-ink font-semibold text-[15px]">
            {tr('Enregistrer une preuve', 'Record evidence')}
          </h2>
          <button
            className="p-1.5 rounded-md hover:bg-surface-3 text-ink-muted"
            onClick={onClose}
            aria-label={tr('Fermer', 'Close')}
          >
            <X size={16} aria-hidden="true" />
          </button>
        </header>

        <div className="px-5 py-4 space-y-4 overflow-y-auto">
          <Field label={tr('Intitulé', 'Title')}>
            <input
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder={tr(
                "ex. Rapport de test d'intrusion 2026",
                'e.g. Penetration test report 2026',
              )}
              className="w-full px-3 py-2 text-[13px] rounded-lg bg-surface-2 border border-line text-ink"
            />
          </Field>

          <Field
            label={tr('Nature', 'Nature')}
            hint={tr(
              "Ce qu'un auditeur demande : pas le format du fichier.",
              'What an auditor asks about — not the file format.',
            )}
          >
            <div className="flex flex-wrap gap-1.5">
              {(Object.keys(EVIDENCE_TYPE_META) as EvidenceType[]).map((t) => (
                <button
                  key={t}
                  onClick={() => setType(t)}
                  className={`px-2.5 py-1 rounded-full text-[12.5px] border ${
                    type === t
                      ? 'border-accent text-accent bg-accent/10'
                      : 'border-line text-ink-muted'
                  }`}
                >
                  {tr(EVIDENCE_TYPE_META[t].fr, EVIDENCE_TYPE_META[t].en)}
                </button>
              ))}
            </div>
          </Field>

          <Field
            label={tr('Fichier', 'File')}
            hint={tr(
              'Facultatif : une preuve peut être un lien ou une déclaration écrite.',
              'Optional: evidence can be a link or a written statement.',
            )}
          >
            <label className="flex items-center gap-2 px-3 py-2 rounded-lg border border-dashed border-line cursor-pointer text-[13px] text-ink-muted hover:bg-surface-2">
              <Upload size={15} />
              {file ? (
                <span className="text-ink truncate">{file.name}</span>
              ) : (
                tr('Choisir un fichier…', 'Choose a file…')
              )}
              <input
                type="file"
                className="hidden"
                onChange={(e) => setFile(e.target.files?.[0] ?? null)}
              />
            </label>
          </Field>

          <Field label={tr('Lien externe', 'External link')}>
            <input
              value={externalUrl}
              onChange={(e) => setExternalUrl(e.target.value)}
              placeholder="https://…"
              className="w-full px-3 py-2 text-[13px] rounded-lg bg-surface-2 border border-line text-ink"
            />
          </Field>

          <Field label={tr('Description', 'Description')}>
            <textarea
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              rows={2}
              className="w-full px-3 py-2 text-[13px] rounded-lg bg-surface-2 border border-line text-ink"
            />
          </Field>

          <div className="grid grid-cols-2 gap-3">
            <Field
              label={tr('Collectée le', 'Collected on')}
              hint={tr(
                "Quand la preuve a été prise, pas aujourd'hui.",
                'When the proof was taken, not today.',
              )}
            >
              <input
                type="date"
                value={collectedAt}
                onChange={(e) => setCollectedAt(e.target.value)}
                className="w-full px-3 py-2 text-[13px] rounded-lg bg-surface-2 border border-line text-ink"
              />
            </Field>
            <Field
              label={tr("Valide jusqu'au", 'Valid until')}
              hint={tr("Laisser vide si elle n'expire pas.", 'Leave empty if it never expires.')}
            >
              <input
                type="date"
                value={validUntil}
                onChange={(e) => setValidUntil(e.target.value)}
                className="w-full px-3 py-2 text-[13px] rounded-lg bg-surface-2 border border-line text-ink"
              />
            </Field>
          </div>

          <Field
            label={tr('Contrôles justifiés', 'Controls substantiated')}
            hint={tr(
              'Une même preuve peut en couvrir plusieurs, dans plusieurs référentiels.',
              'One artifact can cover several, across several frameworks.',
            )}
          >
            <select
              value={frameworkId}
              onChange={(e) => setFrameworkId(e.target.value)}
              className="w-full px-3 py-2 text-[13px] rounded-lg bg-surface-2 border border-line text-ink mb-2"
            >
              <option value="">{tr('Choisir un référentiel…', 'Choose a framework…')}</option>
              {frameworks.map((f) => (
                <option key={f.id} value={f.id}>
                  {f.name} {f.version}
                </option>
              ))}
            </select>
            {frameworkId && controls.length > 0 ? (
              <div className="max-h-[160px] overflow-y-auto rounded-lg border border-line divide-y divide-line">
                {controls.map((c) => (
                  <label
                    key={c.id}
                    className="flex items-start gap-2 px-3 py-2 text-[13px] cursor-pointer hover:bg-surface-2"
                  >
                    <input
                      type="checkbox"
                      className="mt-0.5"
                      checked={controlIds.includes(c.id)}
                      onChange={(e) =>
                        setControlIds((prev) =>
                          e.target.checked ? [...prev, c.id] : prev.filter((id) => id !== c.id),
                        )
                      }
                    />
                    <span className="min-w-0">
                      <span className="font-mono text-[12px] text-ink-muted mr-1.5">
                        {c.reference_code}
                      </span>
                      <span className="text-ink">{c.name}</span>
                    </span>
                  </label>
                ))}
              </div>
            ) : null}
            {controlIds.length > 0 ? (
              <p className="text-[12px] text-ink-muted mt-1.5">
                <Paperclip size={12} className="inline mr-1" />
                {controlIds.length} {tr('contrôle(s) sélectionné(s)', 'control(s) selected')}
              </p>
            ) : null}
          </Field>

          {!substance ? (
            <p className="text-[12.5px]" style={{ color: 'var(--medium)' }}>
              {tr(
                'Ajoutez un fichier, un lien ou une description : sans quoi cette preuve ne prouve rien.',
                'Add a file, a link or a description — without one this evidence proves nothing.',
              )}
            </p>
          ) : null}
          {error ? (
            <p className="text-[12.5px]" style={{ color: 'var(--critical)' }}>
              {error}
            </p>
          ) : null}
        </div>

        <footer className="px-5 py-3.5 border-t border-line flex justify-end gap-2 shrink-0">
          <Btn onClick={onClose} label={tr('Annuler', 'Cancel')} />
          <Btn
            disabled={!canSubmit}
            onClick={submit}
            label={pending ? tr('Enregistrement…', 'Saving…') : tr('Enregistrer', 'Save')}
          />
        </footer>
      </div>
    </div>
  );
}

function Field({
  label,
  hint,
  children,
}: {
  label: string;
  hint?: string;
  children: React.ReactNode;
}) {
  return (
    <div>
      <label className="block text-[13px] font-medium text-ink mb-1">{label}</label>
      {hint ? <p className="text-[12px] text-ink-muted mb-1.5">{hint}</p> : null}
      {children}
    </div>
  );
}
