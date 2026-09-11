// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// Import findings from a named integration: pick the source, paste the tool's
// native findings JSON (an array of objects, exactly as exported), submit. The
// backend normalises + risk-based prioritises + upserts them.

import { useState } from 'react';
import { toast } from 'sonner';
import { X, Upload } from 'lucide-react';
import { useUIStore } from '../../store/uiStore';
import { useVulnMutations } from './useVulnerabilities';
import type { VulnSource } from './vulnerabilityService';
import { SOURCE_LABEL } from './vulnMeta';
import { useEscapeToClose } from '../../shared/useBackTo';

const SOURCES: VulnSource[] = [
  'nessus',
  'openvas',
  'qualys',
  'ms_defender',
  'aws_inspector',
  'azure_defender',
  'crowdstrike',
  'manual',
];

// One JSON snippet per source documenting the native payload shape that source
// emits, so the user pasting an export knows what is expected. This is format
// documentation for an input field, not tenant data: nothing here is ever
// rendered as if it were a vulnerability the tenant has.
const FORMAT_EXAMPLES: Record<string, string> = {
  nessus: `[{ "plugin_id": "156032", "plugin_name": "Apache Log4j RCE", "cvss3_base_score": 9.8, "cve": "CVE-2021-44228", "severity": 4, "host": "web-01", "solution": "Upgrade log4j" }]`,
  crowdstrike: `[{ "id": "cs-1", "cve": { "id": "CVE-2023-23397", "base_score": 9.1, "severity": "CRITICAL", "exploit_status": 90 }, "host_info": { "hostname": "pc-42" } }]`,
  manual: `[{ "title": "SMB legacy", "cve": "CVE-2017-0144", "cvss": 5.0, "kev": true, "host": "web-01" }]`,
};

export function IngestModal({ isOpen, onClose }: { isOpen: boolean; onClose: () => void }) {
  // Esc closes this overlay (spec §2).
  useEscapeToClose(isOpen, onClose);
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const { ingest } = useVulnMutations();
  const [source, setSource] = useState<VulnSource>('nessus');
  const [raw, setRaw] = useState('');

  if (!isOpen) return null;

  const submit = async () => {
    let findings: Record<string, unknown>[];
    try {
      const parsed = JSON.parse(raw);
      findings = Array.isArray(parsed) ? parsed : [parsed];
    } catch {
      toast.error(tr('JSON invalide', 'Invalid JSON'));
      return;
    }
    if (findings.length === 0) {
      toast.error(tr('Aucun finding', 'No findings'));
      return;
    }
    try {
      const res = await ingest.mutateAsync({ source, findings });
      toast.success(
        tr(
          `${res.created} créées · ${res.updated} mises à jour`,
          `${res.created} created · ${res.updated} updated`,
        ),
      );
      setRaw('');
      onClose();
    } catch {
      toast.error(tr('Échec de l’import', 'Import failed'));
    }
  };

  return (
    <div
      className="fixed inset-0 z-80 flex items-center justify-center p-4"
      style={{ background: 'rgba(0,0,0,.5)', backdropFilter: 'blur(3px)' }}
      onClick={onClose}
    >
      {/* Announced as a dialog and named by its heading; the findings table
          behind it stays in the accessibility tree otherwise. */}
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="ingest-title"
        onClick={(e) => e.stopPropagation()}
        className="w-full max-w-[560px] rounded-[16px] flex flex-col"
        style={{
          maxHeight: '90vh',
          background: 'var(--bg-secondary)',
          border: '1px solid var(--border)',
          boxShadow: 'var(--shadow-lg)',
        }}
      >
        <div
          className="flex items-center justify-between px-5 py-4"
          style={{ borderBottom: '1px solid var(--border)' }}
        >
          {/* A dialog's name should come from a heading, not a styled div. */}
          <h2 id="ingest-title" className="flex items-center gap-2 text-[15px] font-bold text-ink">
            <Upload size={17} aria-hidden="true" /> {tr('Importer des findings', 'Import findings')}
          </h2>
          <button
            onClick={onClose}
            aria-label={tr('Fermer', 'Close')}
            className="w-8 h-8 rounded-[9px] flex items-center justify-center text-ink-soft"
            style={{ background: 'var(--bg-hover)' }}
          >
            <X size={18} aria-hidden="true" />
          </button>
        </div>

        <div className="flex-1 overflow-y-auto px-5 py-4 space-y-4">
          <div>
            <div className="text-[11px] font-semibold uppercase tracking-[.04em] text-ink-muted mb-1.5">
              {tr('Source', 'Source')}
            </div>
            <div className="flex flex-wrap gap-2">
              {SOURCES.map((s) => (
                <button
                  key={s}
                  onClick={() => setSource(s)}
                  className="h-8 px-3 rounded-[8px] text-[12.5px] font-semibold"
                  style={{
                    border: `1px solid ${source === s ? 'var(--accent)' : 'var(--border-strong)'}`,
                    color: source === s ? 'var(--accent)' : 'var(--fg-secondary)',
                    background: source === s ? 'var(--accent-soft)' : 'transparent',
                  }}
                >
                  {SOURCE_LABEL[s]}
                </button>
              ))}
            </div>
          </div>

          <div>
            <div className="flex items-center justify-between mb-1.5">
              <div className="text-[11px] font-semibold uppercase tracking-[.04em] text-ink-muted">
                {tr('Findings (JSON du tableau exporté)', 'Findings (exported array JSON)')}
              </div>
              {FORMAT_EXAMPLES[source] && (
                <button
                  onClick={() => setRaw(FORMAT_EXAMPLES[source])}
                  className="text-[11.5px] font-semibold"
                  style={{ color: 'var(--accent-500)' }}
                >
                  {tr('Insérer un exemple', 'Insert sample')}
                </button>
              )}
            </div>
            <textarea
              value={raw}
              onChange={(e) => setRaw(e.target.value)}
              rows={10}
              spellCheck={false}
              placeholder={FORMAT_EXAMPLES[source] ?? '[ { ... } ]'}
              className="w-full rounded-[10px] px-3 py-2.5 text-[12.5px] mono text-ink outline-none"
              style={{ background: 'var(--bg-primary)', border: '1px solid var(--border)' }}
            />
            <div className="text-[11px] text-ink-muted mt-1.5">
              {tr(
                'Collez le tableau de findings exporté par l’outil — la normalisation et la priorisation sont automatiques.',
                'Paste the array of findings exported by the tool — normalisation and prioritisation are automatic.',
              )}
            </div>
          </div>
        </div>

        <div
          className="px-5 py-4 flex justify-end gap-2"
          style={{ borderTop: '1px solid var(--border)' }}
        >
          <button
            onClick={onClose}
            className="h-9 px-4 rounded-[9px] text-[13px] font-semibold"
            style={{ border: '1px solid var(--border-strong)', color: 'var(--fg-secondary)' }}
          >
            {tr('Annuler', 'Cancel')}
          </button>
          <button
            onClick={submit}
            disabled={ingest.isPending || !raw.trim()}
            className="h-9 px-4 rounded-[9px] text-[13px] font-semibold text-fg-on-solid disabled:opacity-60"
            style={{ background: 'var(--accent-solid)' }}
          >
            {ingest.isPending ? tr('Import…', 'Importing…') : tr('Importer', 'Import')}
          </button>
        </div>
      </div>
    </div>
  );
}
