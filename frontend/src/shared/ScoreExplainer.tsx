// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// <ScoreExplainer> — openable wherever a score appears.
//
// THE LAW OF ZERO LIES, made concrete. Everything here is rendered from the
// server's `breakdown`; the component computes nothing and assumes nothing:
//
//   • the bars are the contributions the server sent, and they SUM TO THE VALUE,
//     so the reader can add them up and get the number back;
//   • a factor the server could not measure is shown as "not measured", not
//     hidden and not silently scored zero — hiding it would make the model look
//     like it has fewer dimensions than it does;
//   • the assumptions (the inputs the calculation used), the computation time and
//     the formula version are all shown, because a score whose provenance is
//     unstated is a score you cannot argue with;
//   • inherent and residual are both shown, so "we treated it" is visible as a
//     delta rather than as an unexplained drop.

import { localeTag } from '../i18n/locales';
import { useEffect, useState } from 'react';
import { Info, X, CheckCircle2, MinusCircle } from 'lucide-react';

import { useUIStore } from '../store/uiStore';
import type { LocaleCode } from '../i18n/locales';
import {
  bandColor,
  bandLabel,
  factorLabel,
  type Score,
  type ScoreFactor,
} from '../services/scoreService';

/** The "?" affordance placed next to a score. */
export function ScoreExplainerButton({
  score,
  className = '',
}: {
  score: Score | undefined;
  className?: string;
}) {
  const [open, setOpen] = useState(false);
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);

  if (!score) return null;

  return (
    <>
      <button
        type="button"
        onClick={() => setOpen(true)}
        data-testid="score-explainer-open"
        aria-label={tr('Comment ce score est-il calculé ?', 'How is this score computed?')}
        className={`inline-flex items-center gap-1 text-ink-muted hover:text-ink transition-colors ${className}`}
      >
        <Info size={14} />
      </button>
      {open && <ScoreExplainer score={score} onClose={() => setOpen(false)} />}
    </>
  );
}

/**
 * The panel itself.
 *
 * `inline` drops the overlay and the close affordance, for the dedicated score
 * page where the explanation IS the content rather than an interruption.
 */
export function ScoreExplainer({
  score,
  onClose,
  inline,
}: {
  score: Score;
  onClose?: () => void;
  inline?: boolean;
}) {
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);

  useEffect(() => {
    if (inline || !onClose) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [onClose, inline]);

  const available = score.breakdown.filter((f) => f.available);
  const missing = score.breakdown.filter((f) => !f.available);
  const maxContribution = Math.max(1, ...available.map((f) => f.contribution));
  const treated = score.inherent - score.residual;

  const body = (
    <div
      role={inline ? undefined : 'dialog'}
      aria-label={tr('Explication du score', 'Score explanation')}
      data-testid="score-explainer"
      className={
        inline
          ? 'w-full p-5'
          : 'relative w-full max-w-[520px] max-h-[85vh] overflow-y-auto rounded-[16px] p-5'
      }
      style={
        inline
          ? undefined
          : {
              background: 'var(--bg-elevated)',
              border: '1px solid var(--border-strong)',
              boxShadow: '0 20px 50px rgba(0,0,0,.3)',
              animation: 'or-scalein .18s ease',
            }
      }
    >
      <div className="flex items-start justify-between gap-3 mb-4">
        <div>
          <div className="text-[15.5px] font-bold text-ink">
            {tr('Comment ce score est calculé', 'How this score is computed')}
          </div>
          <div className="text-[12.5px] text-ink-soft mt-0.5">
            {tr(
              'Chaque contribution ci-dessous s’additionne pour donner le score.',
              'Each contribution below adds up to the score.',
            )}
          </div>
        </div>
        {!inline && onClose && (
          <button
            type="button"
            onClick={onClose}
            className="text-ink-muted hover:text-ink transition-colors shrink-0"
            aria-label={tr('Fermer', 'Close')}
          >
            <X size={17} />
          </button>
        )}
      </div>

      {/* Inherent vs residual — the pair an auditor asks for. */}
      <div className="grid grid-cols-2 gap-3 mb-4">
        <ScoreCard
          label={tr('Risque inhérent', 'Inherent risk')}
          hint={tr('Avant traitement', 'Before treatment')}
          value={score.inherent}
          band={score.inherent_band}
          lang={lang}
        />
        <ScoreCard
          label={tr('Risque résiduel', 'Residual risk')}
          hint={tr('Après mitigations appliquées', 'After applied mitigations')}
          value={score.residual}
          band={score.residual_band}
          lang={lang}
          emphasis
        />
      </div>

      {treated > 0.05 && (
        <div
          className="rounded-[10px] p-2.5 mb-4 text-[12.5px] text-ink-soft"
          style={{ background: 'var(--bg-hover)' }}
        >
          {tr('Les mitigations appliquées retirent ', 'Applied mitigations remove ')}
          <span className="mono font-semibold text-ink">{treated.toFixed(1)}</span>
          {tr(' points, soit ', ' points, i.e. ')}
          <span className="mono font-semibold text-ink">
            {Math.round(score.mitigation_effectiveness * 100)}%
          </span>
          {tr(" de l'exposition.", ' of the exposure.')}
        </div>
      )}

      {/* Contributions */}
      <div className="text-[11px] font-bold uppercase tracking-wide text-ink-muted mb-2">
        {tr('Contribution de chaque facteur', 'Contribution of each factor')}
      </div>
      <ul className="list-none p-0 m-0 flex flex-col gap-2.5 mb-4">
        {available.map((f) => (
          <FactorBar key={f.factor} factor={f} max={maxContribution} lang={lang} />
        ))}
      </ul>

      {/* The sum, stated. If it did not reconcile, this is where you would see it. */}
      <div
        className="flex items-center justify-between rounded-[10px] px-3 py-2 mb-4 text-[13px]"
        style={{ background: 'var(--bg-hover)' }}
      >
        <span className="font-semibold text-ink">
          {tr('Total (risque inhérent)', 'Total (inherent risk)')}
        </span>
        <span className="mono font-bold text-ink">{score.inherent.toFixed(1)} / 100</span>
      </div>

      {missing.length > 0 && (
        <div className="mb-4">
          <div className="text-[11px] font-bold uppercase tracking-wide text-ink-muted mb-1.5">
            {tr('Non mesuré', 'Not measured')}
          </div>
          <ul className="list-none p-0 m-0 flex flex-col gap-1">
            {missing.map((f) => (
              <li
                key={f.factor}
                className="flex items-center gap-2 text-[12.5px] text-ink-muted"
                data-testid={`score-factor-missing-${f.factor}`}
              >
                <MinusCircle size={13} />
                {factorLabel(f.factor, lang)}
              </li>
            ))}
          </ul>
          <div className="text-[11.5px] text-ink-muted mt-1.5 leading-snug">
            {tr(
              'Ces facteurs sont exclus du calcul et leur poids redistribué — une donnée absente n’est jamais comptée comme un bon résultat.',
              'These factors are excluded and their weight redistributed — a missing signal is never counted as a good one.',
            )}
          </div>
        </div>
      )}

      {/* Assumptions */}
      {Object.keys(score.inputs ?? {}).length > 0 && (
        <details className="mb-3">
          <summary className="text-[12.5px] font-semibold text-ink cursor-pointer">
            {tr('Hypothèses utilisées', 'Assumptions used')}
          </summary>
          <dl className="mt-2 grid grid-cols-2 gap-x-3 gap-y-1 text-[12px]">
            {Object.entries(score.inputs).map(([key, value]) => (
              <div key={key} className="contents">
                <dt className="text-ink-muted truncate">{key}</dt>
                <dd className="mono text-ink text-right m-0">{formatInput(value)}</dd>
              </div>
            ))}
          </dl>
        </details>
      )}

      {/* Provenance */}
      <div className="text-[11.5px] text-ink-muted flex flex-wrap items-center gap-x-3 gap-y-1">
        <span>
          {tr('Calculé le ', 'Computed ')}
          <span className="mono">{formatDate(score.computed_at, lang)}</span>
        </span>
        <span>
          {tr('Formule ', 'Formula ')}
          <span className="mono">v{score.formula_version}</span>
        </span>
        <span className="mono">{score.scope}</span>
      </div>
    </div>
  );

  if (inline) return body;

  return (
    <div className="fixed inset-0 z-80 flex items-center justify-center p-4">
      <div
        className="absolute inset-0"
        style={{ background: 'var(--bg-overlay, rgba(0,0,0,.4))', backdropFilter: 'blur(2px)' }}
        onClick={onClose}
      />
      {body}
    </div>
  );
}

function ScoreCard({
  label,
  hint,
  value,
  band,
  lang,
  emphasis,
}: {
  label: string;
  hint: string;
  value: number;
  band: Score['band'];
  lang: LocaleCode;
  emphasis?: boolean;
}) {
  const color = bandColor(band);
  return (
    <div
      className="rounded-[12px] p-3"
      style={{
        background: 'var(--bg-hover)',
        border: emphasis ? `1px solid ${color}` : '1px solid transparent',
      }}
    >
      <div className="text-[11.5px] text-ink-muted">{label}</div>
      <div className="flex items-baseline gap-1.5 mt-0.5">
        <span className="mono text-[22px] font-bold" style={{ color }}>
          {value.toFixed(1)}
        </span>
        <span className="text-[11px] text-ink-muted">/ 100</span>
      </div>
      {/* The band is the SERVER's, printed as received — never derived here. */}
      <div className="text-[11.5px] font-semibold mt-0.5" style={{ color }}>
        {bandLabel(band, lang)}
      </div>
      <div className="text-[10.5px] text-ink-muted mt-0.5">{hint}</div>
    </div>
  );
}

function FactorBar({ factor, max, lang }: { factor: ScoreFactor; max: number; lang: LocaleCode }) {
  const width = Math.max(2, (factor.contribution / max) * 100);
  return (
    <li data-testid={`score-factor-${factor.factor}`}>
      <div className="flex items-baseline justify-between gap-2 mb-1">
        <span className="text-[12.5px] font-semibold text-ink flex items-center gap-1.5">
          <CheckCircle2 size={12} className="text-ink-muted" />
          {factorLabel(factor.factor, lang)}
        </span>
        <span className="mono text-[11.5px] text-ink-soft shrink-0">
          {/* The arithmetic, shown: weight × raw = contribution. */}
          {Math.round(factor.weight * 100)}% × {factor.raw.toFixed(0)} ={' '}
          <span className="font-bold text-ink">{factor.contribution.toFixed(1)}</span>
        </span>
      </div>
      <div className="h-1.5 rounded-full overflow-hidden" style={{ background: 'var(--bg-hover)' }}>
        <div
          className="h-full rounded-full"
          style={{
            width: `${width}%`,
            background: 'var(--accent)',
          }}
        />
      </div>
    </li>
  );
}

function formatInput(value: unknown): string {
  if (value === null || value === undefined) return '—';
  if (typeof value === 'boolean') return value ? '✓' : '✗';
  if (Array.isArray(value)) return value.join(' – ');
  if (typeof value === 'number') return Number.isInteger(value) ? String(value) : value.toFixed(2);
  return String(value);
}

function formatDate(iso: string, lang: LocaleCode): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString(localeTag(lang), {
    dateStyle: 'short',
    timeStyle: 'short',
  });
}
