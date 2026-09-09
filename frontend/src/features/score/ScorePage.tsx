// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// The dedicated score page: the same number the dashboard hero and the sidebar
// footer show, with the whole explanation opened out rather than behind an icon.
//
// It reads useScore('tenant') — the identical query key the other two use — so
// "the three surfaces agree" is not a thing anyone has to maintain. They share
// one cache entry and therefore one object.

import { useNavigate } from 'react-router';
import { ShieldAlert, ArrowRight } from 'lucide-react';

import { useUIStore } from '../../store/uiStore';
import { useScore, useScoreModel } from '../../hooks/useScore';
import { ScoreGauge } from '../../shared/ScoreGauge';
import { ScoreExplainer } from '../../shared/ScoreExplainer';
import { bandColor, bandLabel } from '../../services/scoreService';
import { ErrorState, SkeletonRows } from '../../shared/ui';
import type { LocaleCode } from '../../i18n/locales';

export function ScorePage() {
  const navigate = useNavigate();
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);

  const { data: score, isLoading, isError, refetch } = useScore('tenant');
  const { data: model } = useScoreModel();

  return (
    <div className="flex-1 overflow-y-auto">
      <div
        className="mx-auto px-5 sm:px-7 pt-6 pb-10 max-w-[1000px]"
        style={{ animation: 'or-fadeup .4s ease' }}
      >
        <header className="mb-5">
          <h1 className="disp text-[22px] font-bold text-ink flex items-center gap-2">
            <ShieldAlert size={20} style={{ color: 'var(--accent-500)' }} />
            {tr('Score de risque', 'Risk score')}
          </h1>
          <p className="text-[13.5px] text-ink-soft mt-1">
            {tr(
              'Un seul score, calculé côté serveur, affiché à l’identique partout dans l’application.',
              'One score, computed server-side, shown identically everywhere in the app.',
            )}
          </p>
        </header>

        {isLoading && <SkeletonRows rows={4} height={64} />}

        {isError && (
          <ErrorState
            title={tr('Le score n’a pas pu être calculé', 'The score could not be computed')}
            description={tr(
              'Réessayez dans un instant ; si cela persiste, contactez un administrateur.',
              'Try again in a moment; if it persists, contact an administrator.',
            )}
            onRetry={() => void refetch()}
            retryLabel={tr('Réessayer', 'Retry')}
          />
        )}

        {score && (
          <>
            <div className="grid grid-cols-1 lg:grid-cols-[300px_1fr] gap-4 mb-4">
              <ScoreGauge score={score} title={tr('Score global', 'Overall score')} />

              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <Metric
                  label={tr('Risque inhérent', 'Inherent risk')}
                  hint={tr('Avant traitement', 'Before treatment')}
                  value={score.inherent}
                  band={score.inherent_band}
                  lang={lang}
                />
                <Metric
                  label={tr('Risque résiduel', 'Residual risk')}
                  hint={tr('Après mitigations appliquées', 'After applied mitigations')}
                  value={score.residual}
                  band={score.residual_band}
                  lang={lang}
                />
                <div
                  className="rounded-[14px] p-4 sm:col-span-2"
                  style={{
                    background: 'var(--bg-elevated)',
                    border: '1px solid var(--border-strong)',
                  }}
                >
                  <div className="text-[12px] text-ink-muted">
                    {tr(
                      'Réduction obtenue par les mitigations',
                      'Reduction achieved by mitigations',
                    )}
                  </div>
                  <div className="mono text-[20px] font-bold text-ink mt-0.5">
                    {Math.round(score.mitigation_effectiveness * 100)}%
                  </div>
                  <div className="text-[11.5px] text-ink-muted mt-1 leading-snug">
                    {tr(
                      'Le score inhérent ne bouge pas quand un plan avance : c’est la taille du problème. Le résiduel est ce qu’il en reste.',
                      'The inherent score does not move as a plan advances: it is the size of the problem. The residual is what remains of it.',
                    )}
                  </div>
                </div>
              </div>
            </div>

            {/* The full explanation, opened out rather than hidden behind an icon. */}
            <div
              className="rounded-[16px] overflow-hidden"
              style={{ background: 'var(--bg-elevated)', border: '1px solid var(--border-strong)' }}
            >
              <ScoreExplainer score={score} inline />
            </div>

            {model && (
              <div className="text-[11.5px] text-ink-muted mt-3">
                {tr('Échelle ', 'Scale ')}
                <span className="mono">
                  {model.min_value}–{model.max_value}
                </span>
                {' · '}
                {model.bands.map((b, i) => (
                  <span key={b.band}>
                    {i > 0 && ' · '}
                    <span style={{ color: bandColor(b.band) }}>{bandLabel(b.band, lang)}</span>{' '}
                    <span className="mono">
                      {b.min}–{b.max}
                      {b.max_inclusive ? ']' : '['}
                    </span>
                  </span>
                ))}
              </div>
            )}

            <button
              onClick={() => navigate('/risks')}
              className="mt-5 h-10 px-4 rounded-[10px] text-[13px] font-semibold inline-flex items-center gap-2"
              style={{
                background: 'var(--accent-solid)',
                color: 'var(--fg-on-solid)',
              }}
            >
              {tr('Ouvrir le registre des risques', 'Open the risk register')}
              <ArrowRight size={15} />
            </button>
          </>
        )}
      </div>
    </div>
  );
}

function Metric({
  label,
  hint,
  value,
  band,
  lang,
}: {
  label: string;
  hint: string;
  value: number;
  band: Parameters<typeof bandColor>[0];
  lang: LocaleCode;
}) {
  const color = bandColor(band);
  return (
    <div
      className="rounded-[14px] p-4"
      style={{ background: 'var(--bg-elevated)', border: '1px solid var(--border-strong)' }}
    >
      <div className="text-[12px] text-ink-muted">{label}</div>
      <div className="flex items-baseline gap-1.5 mt-0.5">
        <span className="mono text-[26px] font-bold" style={{ color }}>
          {value.toFixed(1)}
        </span>
        <span className="text-[11px] text-ink-muted">/ 100</span>
      </div>
      <div className="text-[12px] font-semibold mt-0.5" style={{ color }}>
        {bandLabel(band, lang)}
      </div>
      <div className="text-[11px] text-ink-muted mt-0.5">{hint}</div>
    </div>
  );
}
