// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

import { localeTag, type LocaleCode } from '../../i18n/locales';
import { useQuery } from '@tanstack/react-query';
import { ExternalLink, ShieldAlert, X } from 'lucide-react';
import { SkeletonRows } from '../../shared/ui';
import { critColor, type Criticality } from '../../shared/riskColors';
import { useUIStore } from '../../store/uiStore';
import { ctiService, type CTIVulnerability } from './ctiService';
import { useEscapeToClose } from '../../shared/useBackTo';

const sevToCrit = (s?: string): Criticality => {
  switch ((s ?? '').toUpperCase()) {
    case 'CRITICAL':
      return 'critical';
    case 'HIGH':
      return 'high';
    case 'MEDIUM':
      return 'medium';
    default:
      return 'low';
  }
};

/**
 * The CVE detail panel.
 *
 * The feed rows carried a hover highlight and no click handler — they looked
 * openable and did nothing, while GET /cti/vulnerabilities/:cve had been serving
 * the full record (CPEs, MITRE mapping, CISA due date, remediation) since the
 * CTI engine was wired. This is that record.
 *
 * The list row it was opened from is passed in as `fallback`, so the panel
 * shows what is already known while the full record loads and still says
 * something useful if the fetch fails.
 */
export function CveDetailDrawer({
  cveId,
  fallback,
  onClose,
}: {
  cveId: string;
  fallback?: CTIVulnerability;
  onClose: () => void;
}) {
  useEscapeToClose(true, onClose);
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);

  const { data, isLoading, isError } = useQuery({
    queryKey: ['cti', 'vulnerability', cveId],
    queryFn: () => ctiService.get(cveId),
  });

  const v = data ?? fallback;
  const crit = sevToCrit(v?.severity);

  return (
    <>
      <div
        className="fixed inset-0 z-40"
        style={{ background: 'var(--surface-overlay)' }}
        onClick={onClose}
      />
      <aside
        className="fixed right-0 top-0 bottom-0 z-50 w-full max-w-[520px] flex flex-col or-slidein"
        style={{ background: 'var(--bg-elevated)', borderLeft: '1px solid var(--border-strong)' }}
        role="dialog"
        aria-label={cveId}
      >
        <header
          className="flex items-start justify-between gap-3 px-5 py-4 shrink-0"
          style={{ borderBottom: '1px solid var(--border)' }}
        >
          <div className="min-w-0">
            <div className="flex items-center gap-2 flex-wrap">
              <h2 className="mono text-[15px] font-bold text-ink">{cveId}</h2>
              {v?.cisa_known && (
                <span
                  className="inline-flex items-center gap-1 text-[10px] font-bold px-1.5 py-0.5 rounded"
                  style={{
                    background: 'color-mix(in srgb, var(--critical) 16%, transparent)',
                    color: 'var(--critical)',
                  }}
                >
                  <ShieldAlert size={10} /> CISA KEV
                </span>
              )}
            </div>
            {v && (
              <p className="text-[12px] text-ink-muted mt-0.5">
                {tr('Sévérité', 'Severity')} {v.severity || '—'} · CVSS{' '}
                <span className="mono font-semibold" style={{ color: critColor[crit] }}>
                  {v.cvss_v3 > 0 ? v.cvss_v3.toFixed(1) : '—'}
                </span>
              </p>
            )}
          </div>
          <button
            onClick={onClose}
            className="p-1.5 rounded-lg text-ink-muted"
            aria-label={tr('Fermer', 'Close')}
          >
            <X size={18} />
          </button>
        </header>

        <div className="flex-1 overflow-y-auto px-5 py-4 space-y-5">
          {isLoading && !fallback ? (
            <SkeletonRows rows={4} />
          ) : !v ? (
            <p className="text-[13px] text-ink-soft">
              {tr('Ce CVE est introuvable dans le flux.', 'This CVE is not in the feed.')}
            </p>
          ) : (
            <>
              {isError && (
                <p className="text-[12px]" style={{ color: 'var(--medium)' }}>
                  {tr(
                    'Le détail complet n’a pas pu être chargé — voici ce que le flux avait déjà.',
                    'The full record could not be loaded — this is what the feed already had.',
                  )}
                </p>
              )}

              <Section title={tr('Description', 'Description')}>
                <p className="text-[13px] text-ink-soft leading-relaxed">
                  {v.description ||
                    tr('Aucune description fournie par le flux.', 'No description in the feed.')}
                </p>
              </Section>

              {v.remediation && (
                <Section title={tr('Remédiation', 'Remediation')}>
                  <p className="text-[13px] text-ink-soft leading-relaxed">{v.remediation}</p>
                </Section>
              )}

              {v.cisa_known && v.cisa_due_date && (
                <Section title={tr('Échéance CISA', 'CISA due date')}>
                  <p className="text-[13px]" style={{ color: 'var(--critical)' }}>
                    {new Date(v.cisa_due_date).toLocaleDateString(
                      localeTag(lang),
                    )}
                  </p>
                </Section>
              )}

              {(v.mitre_tactics?.length || v.mitre_techniques?.length) && (
                <Section title="MITRE ATT&CK">
                  <div className="flex flex-wrap gap-1.5">
                    {[...(v.mitre_tactics ?? []), ...(v.mitre_techniques ?? [])].map((t) => (
                      <span
                        key={t}
                        className="mono text-[11px] font-semibold px-2 py-0.5 rounded"
                        style={{ background: 'var(--bg-hover)', color: 'var(--fg-secondary)' }}
                      >
                        {t}
                      </span>
                    ))}
                  </div>
                </Section>
              )}

              {v.affected_cpe?.length ? (
                <Section title={tr('Produits affectés (CPE)', 'Affected products (CPE)')}>
                  {/* These are exactly what the asset correlator matches on, so
                      seeing them explains why a CVE did or did not land on an
                      asset. */}
                  <ul className="space-y-1">
                    {v.affected_cpe.map((cpe) => (
                      <li key={cpe} className="mono text-[11.5px] text-ink-muted break-all">
                        {cpe}
                      </li>
                    ))}
                  </ul>
                </Section>
              ) : null}

              <Section title={tr('Dates', 'Dates')}>
                <dl className="text-[12.5px] space-y-1">
                  <Row label={tr('Publié', 'Published')} value={fmtDate(v.published_at, lang)} />
                  <Row
                    label={tr('Mis à jour', 'Updated')}
                    value={fmtDate(v.last_updated_at, lang)}
                  />
                </dl>
              </Section>

              <a
                href={`https://nvd.nist.gov/vuln/detail/${encodeURIComponent(cveId)}`}
                target="_blank"
                rel="noreferrer noopener"
                className="inline-flex items-center gap-1.5 text-[12.5px] font-semibold"
                style={{ color: 'var(--accent-500)' }}
              >
                {tr('Voir sur le NVD', 'View on the NVD')} <ExternalLink size={13} />
              </a>
            </>
          )}
        </div>
      </aside>
    </>
  );
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section>
      <h3 className="text-[11px] font-semibold uppercase tracking-[.06em] text-ink-muted mb-1.5">
        {title}
      </h3>
      {children}
    </section>
  );
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex justify-between gap-3">
      <dt className="text-ink-muted">{label}</dt>
      <dd className="text-ink-soft">{value}</dd>
    </div>
  );
}

function fmtDate(iso: string | undefined, lang: LocaleCode): string {
  if (!iso) return '—';
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? '—' : d.toLocaleDateString(localeTag(lang));
}
