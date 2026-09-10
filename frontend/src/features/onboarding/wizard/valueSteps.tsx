// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// Steps 4 and 5 of the guided tunnel (#438) — the two screens that RETURN
// something computed.
//
// The first three steps ask. These two show, and that asymmetry is the whole
// point of the issue: five screens that only collect facts and then drop the
// user on a checklist is the activation cliff. Here the matrix lights as the
// user moves a slider, and the residual falls on screen when they accept a
// control.
//
// Both honour prefers-reduced-motion by rendering the FINAL state directly
// (criterion 13) — not a shorter animation, no animation.

import { useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router';
import { ShieldCheck } from 'lucide-react';

import { useUIStore } from '../../../store/uiStore';
import { StepShell } from './stepPrimitives';
import { num, useStepNav, useStoredAnswers } from './stepNav';
import {
  useCompleteOnboarding,
  usePrefersReducedMotion,
  useSaveOnboardingStep,
  useStarterRisks,
} from '../useActivation';
import { i18n } from '../../../services/activationService';

// ---------------------------------------------------------------------------
// The matrix
//
// Five bands on each axis, on the Score Engine's own scales: probability [0,1]
// and impact [0,10]. The cell the user lands on is `P × I`, which is the same
// arithmetic the engine runs (asset criticality is its third factor and is not
// known at this point — the screen says so rather than inventing one).
// ---------------------------------------------------------------------------

const PROBABILITY_BANDS = [0.1, 0.3, 0.5, 0.7, 0.9];
const IMPACT_BANDS = [2, 4, 6, 8, 10];

/**
 * The risk these two steps are about.
 *
 * Both screens said "this risk" without ever naming one, on the step right after
 * the user picked THREE — so they were scoring and covering an anonymous thing.
 * The name comes from the selection stored in step 2 plus the catalogue the
 * server already served; nothing is fetched that could record anything.
 *
 * Returns '' rather than a placeholder when the selection is not there yet: a
 * made-up title on a screen about the user's own register is exactly what
 * criterion 7 forbids.
 */
function useScoredRiskTitle(): string {
  const lang = useUIStore((s) => s.lang);
  const goalAnswers = useStoredAnswers('goal');
  const { data: offer } = useStarterRisks();

  return useMemo(() => {
    const picked = goalAnswers.starter_risks;
    if (!Array.isArray(picked) || picked.length === 0 || !offer) return '';
    const first = offer.risks.find((r) => r.key === picked[0]);
    return first ? i18n(first.title_i18n, lang) : '';
  }, [goalAnswers, offer, lang]);
}

/** Score Engine bands, on the P×I×AC scale with AC unknown (so 0–10 here). */
function bandOf(score: number): 'low' | 'medium' | 'high' | 'critical' {
  if (score >= 7) return 'critical';
  if (score >= 4) return 'high';
  if (score >= 2) return 'medium';
  return 'low';
}

function bandColor(band: string): string {
  switch (band) {
    case 'critical':
      return 'var(--critical)';
    case 'high':
      return 'var(--high)';
    case 'medium':
      return 'var(--medium)';
    default:
      return 'var(--low)';
  }
}

// ---------------------------------------------------------------------------
// 4. Score — likelihood and impact for one of the adopted risks
// ---------------------------------------------------------------------------

export function ScoreStep() {
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const stored = useStoredAnswers('score');
  const { go, busy, error, retry } = useStepNav('score');
  const riskTitle = useScoredRiskTitle();

  const [probability, setProbability] = useState(0.5);
  const [impact, setImpact] = useState(6);

  useEffect(() => {
    setProbability(num(stored, 'probability', 0.5));
    setImpact(num(stored, 'impact', 6));
  }, [stored]);

  const score = Math.round(probability * impact * 1000) / 1000;
  const band = bandOf(score);

  // The nearest band cell, so the grid highlights exactly one square.
  const pIndex = PROBABILITY_BANDS.reduce(
    (best, value, i) =>
      Math.abs(value - probability) < Math.abs(PROBABILITY_BANDS[best] - probability) ? i : best,
    0,
  );
  const iIndex = IMPACT_BANDS.reduce(
    (best, value, i) => (Math.abs(value - impact) < Math.abs(IMPACT_BANDS[best] - impact) ? i : best),
    0,
  );

  return (
    <StepShell
      title={riskTitle || tr('Évaluez ce risque', 'Score this risk')}
      subtitle={tr(
        'Probabilité et impact. La matrice se met à jour pendant que vous bougez les curseurs — c’est le score que le moteur calculera.',
        'Likelihood and impact. The matrix updates as you move the sliders — this is the score the engine will compute.',
      )}
      onBack={() => go({ probability, impact }, -1)}
      onNext={() => go({ probability, impact }, 1)}
      nextLabel={tr('Continuer', 'Continue')}
      busy={busy}
      error={error}
      onRetry={retry}
      errorLabel={tr("Impossible d'enregistrer cette étape.", 'This step could not be saved.')}
      errorHint={tr(
        'Vos réponses sont conservées — réessayez, rien n’est perdu.',
        'Your answers are kept — try again, nothing is lost.',
      )}
      retryLabel={tr('Réessayer', 'Try again')}
    >
      <div className="grid gap-5 sm:grid-cols-2">
        <div>
          <Slider
            id="score-probability"
            testId="score-probability"
            label={tr('Probabilité', 'Likelihood')}
            min={0}
            max={1}
            step={0.05}
            value={probability}
            display={`${Math.round(probability * 100)} %`}
            onChange={setProbability}
          />
          <Slider
            id="score-impact"
            testId="score-impact"
            label={tr('Impact', 'Impact')}
            min={0}
            max={10}
            step={0.5}
            value={impact}
            display={impact.toFixed(1)}
            onChange={setImpact}
          />

          {/* The live announcement criterion 12 asks for. `polite` rather than
              `assertive`: a slider fires many changes, and an assertive region
              would interrupt the screen reader on every one of them. */}
          <div
            className="mt-4 rounded-lg p-3"
            style={{ background: 'var(--bg-elevated)', border: '1px solid var(--border-strong)' }}
            role="status"
            aria-live="polite"
            data-testid="score-readout"
          >
            <div className="text-[11px] uppercase tracking-wide text-ink-muted">
              {tr('Score', 'Score')}
            </div>
            <div className="text-[24px] font-bold tabular-nums" style={{ color: bandColor(band) }}>
              {score.toFixed(2)}
            </div>
            <div className="text-[12px] text-ink-soft mt-0.5">
              {tr('Niveau', 'Level')} : {band}
            </div>
          </div>

          <p className="text-[11.5px] text-ink-muted mt-3 m-0">
            {tr(
              'La criticité de l’actif est le troisième facteur du moteur ; elle sera appliquée quand vous rattacherez un actif.',
              'Asset criticality is the engine’s third factor; it is applied once you link an asset.',
            )}
          </p>
        </div>

        <Matrix pIndex={pIndex} iIndex={iIndex} tr={tr} />
      </div>
    </StepShell>
  );
}

function Slider({
  id,
  testId,
  label,
  min,
  max,
  step,
  value,
  display,
  onChange,
}: {
  id: string;
  testId: string;
  label: string;
  min: number;
  max: number;
  step: number;
  value: number;
  display: string;
  onChange: (v: number) => void;
}) {
  return (
    <div className="mb-4">
      <div className="flex items-baseline justify-between mb-1.5">
        <label htmlFor={id} className="text-[12.5px] font-semibold text-ink">
          {label}
        </label>
        <span className="text-[12.5px] font-semibold text-ink-soft tabular-nums">{display}</span>
      </div>
      <input
        id={id}
        data-testid={testId}
        type="range"
        min={min}
        max={max}
        step={step}
        value={value}
        onChange={(e) => onChange(Number(e.target.value))}
        className="w-full"
        aria-valuetext={display}
      />
    </div>
  );
}

/**
 * The 5×5 grid. A `<table>` rather than a div soup: it IS tabular data, and a
 * screen reader reading "likelihood 70%, impact 8, high" beats it reading
 * twenty-five unlabelled cells.
 */
function Matrix({
  pIndex,
  iIndex,
  tr,
}: {
  pIndex: number;
  iIndex: number;
  tr: (fr: string, en: string) => string;
}) {
  const reduced = usePrefersReducedMotion();

  return (
    <table className="w-full border-separate" style={{ borderSpacing: 3 }}>
      <caption className="sr-only">
        {tr(
          'Matrice de risque : probabilité en lignes, impact en colonnes',
          'Risk matrix: likelihood by row, impact by column',
        )}
      </caption>
      <tbody>
        {[...PROBABILITY_BANDS].reverse().map((p, row) => {
          const realRow = PROBABILITY_BANDS.length - 1 - row;
          return (
            <tr key={p}>
              {IMPACT_BANDS.map((impact, col) => {
                const cellScore = p * impact;
                const band = bandOf(cellScore);
                const active = realRow === pIndex && col === iIndex;
                return (
                  <td
                    key={impact}
                    data-testid={active ? 'matrix-cell-active' : undefined}
                    aria-current={active ? 'true' : undefined}
                    className="h-9 rounded text-center align-middle text-[10.5px] font-semibold"
                    style={{
                      background: active
                        ? bandColor(band)
                        : `color-mix(in srgb, ${bandColor(band)} 14%, transparent)`,
                      color: active ? 'var(--fg-on-solid)' : 'var(--fg-muted)',
                      outline: active ? '2px solid var(--fg-primary)' : 'none',
                      // Criterion 13: no transition at all under reduced motion,
                      // so the lit cell simply IS lit on the next paint.
                      transition: reduced ? 'none' : 'background .18s ease, outline .18s ease',
                    }}
                  >
                    {active ? (p * impact).toFixed(1) : ''}
                  </td>
                );
              })}
            </tr>
          );
        })}
      </tbody>
    </table>
  );
}

// ---------------------------------------------------------------------------
// 5. Cover — accept the proposed control and watch the residual fall
// ---------------------------------------------------------------------------

export function CoverStep() {
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const stored = useStoredAnswers('cover');
  const { go, busy: navBusy, error: navError } = useStepNav('cover');
  const navigate = useNavigate();

  // `cover` is the LAST step, so it does not merely advance a cursor: it is the
  // only place that can lift the route guard. Without the complete call the
  // tunnel has no exit — and because OnboardingGuard denies /app until
  // onboarding.completed is true, a user who reaches this screen is locked out
  // of the product entirely. That is the failure #438's own Risk section warns
  // about, and it is why this step does not use `go(..., 1)` like the others.
  const save = useSaveOnboardingStep();
  const complete = useCompleteOnboarding();

  // DELIBERATELY DOES NOT FETCH /posture.
  //
  // It used to, so the card could show a server-computed residual. But GET
  // /posture is what records `posture.revealed` — the Aha moment itself (D-010)
  // — so fetching it here fired the Aha one step early: the metric behind the
  // eight-minute promise was measured before the user had seen anything, and
  // `first_reveal` was already false by the time the reveal mounted, so the
  // reveal never celebrated.
  //
  // The screen therefore shows the score the user just set in step 4, which it
  // owns, and says plainly that the residual is computed on the next screen.
  // Nothing here is invented: the number comes from their own answers.
  const scored = useStoredAnswers('score');
  const riskTitle = useScoredRiskTitle();
  const [accepted, setAccepted] = useState(false);

  useEffect(() => {
    setAccepted(stored.accepted === true);
  }, [stored]);

  // Score Engine arithmetic on the user's own step-4 answers: P × I. Asset
  // criticality is the engine's third factor and is not known at this point,
  // which the step-4 copy already says.
  const inherent = useMemo(
    () => Math.round(num(scored, 'probability', 0) * num(scored, 'impact', 0) * 100) / 100,
    [scored],
  );

  /**
   * Save, then lift the guard, then land on the reveal.
   *
   * `onSuccess`, not `onSettled`: completing after a failed save would lift the
   * guard on an answer that was never stored. And the tunnel ends on /posture
   * because that is the whole point of #438 — five screens collected facts, and
   * this is the screen that returns something computed in exchange.
   */
  const finish = () => {
    save.mutate(
      { step: 'cover', answers: { accepted } },
      { onSuccess: () => complete.mutate(undefined, { onSuccess: () => navigate('/posture') }) },
    );
  };

  const busy = navBusy || save.isPending || complete.isPending;
  const error = navError || save.isError || complete.isError;

  return (
    <StepShell
      title={tr('Couvrez ce risque', 'Cover this risk')}
      subtitle={tr(
        'Acceptez le contrôle proposé : le risque résiduel est recalculé à partir de vos propres contrôles.',
        'Accept the proposed control: the residual risk is recomputed from your own controls.',
      )}
      onBack={() => go({ accepted }, -1)}
      onNext={finish}
      nextLabel={tr('Voir ma posture', 'See my posture')}
      busy={busy}
      error={error}
      onRetry={finish}
      errorLabel={tr("Impossible d'enregistrer cette étape.", 'This step could not be saved.')}
      errorHint={tr(
        'Vos réponses sont conservées — réessayez, rien n’est perdu.',
        'Your answers are kept — try again, nothing is lost.',
      )}
      retryLabel={tr('Réessayer', 'Try again')}
    >
      <InherentCard inherent={inherent} title={riskTitle} tr={tr} />

      <label
        className="mt-5 flex items-start gap-3 cursor-pointer"
        style={{ userSelect: 'none' }}
      >
        <input
          type="checkbox"
          data-testid="cover-accept"
          checked={accepted}
          onChange={(e) => setAccepted(e.target.checked)}
          className="mt-0.5"
        />
        <span className="text-[13.5px] text-ink">
          {tr(
            'J’accepte le contrôle proposé pour ce risque.',
            'I accept the proposed control for this risk.',
          )}
        </span>
      </label>
    </StepShell>
  );
}

/**
 * What the user has, and what comes next.
 *
 * No residual number here: computing one would need the tenant's control
 * mappings, and the only endpoint that returns them is the reveal — which
 * records the Aha. Showing a client-computed figure instead would be the
 * placeholder criterion 7 forbids, on the screen that leads into the reveal.
 */
function InherentCard({
  inherent,
  title,
  tr,
}: {
  inherent: number;
  title: string;
  tr: (fr: string, en: string) => string;
}) {
  return (
    <div
      className="rounded-xl p-5"
      style={{ background: 'var(--bg-surface)', border: '1px solid var(--border-subtle)' }}
      data-testid="cover-residual"
    >
      {title && <div className="text-[13.5px] font-semibold text-ink mb-3">{title}</div>}
      <div className="flex items-end gap-5">
        <Figure label={tr('Inhérent', 'Inherent')} value={inherent} />
        <ShieldCheck size={18} aria-hidden="true" style={{ color: 'var(--fg-muted)', marginBottom: 6 }} />
        <div>
          <div className="text-[10.5px] uppercase tracking-wide text-ink-muted">
            {tr('Résiduel', 'Residual')}
          </div>
          <div className="text-[13px] text-ink-soft mt-1" data-testid="cover-residual-pending">
            {tr('calculé à l’écran suivant', 'computed on the next screen')}
          </div>
        </div>
      </div>
    </div>
  );
}

function Figure({
  label,
  value,
  color,
  muted,
  testId,
  reduced,
}: {
  label: string;
  value: number;
  color?: string;
  muted?: boolean;
  testId?: string;
  reduced?: boolean;
}) {
  return (
    <div data-testid={testId}>
      <div className="text-[10.5px] uppercase tracking-wide text-ink-muted">{label}</div>
      <div
        className="text-[28px] font-bold tabular-nums leading-none"
        style={{
          color: muted ? 'var(--fg-secondary)' : (color ?? 'var(--fg-primary)'),
          transition: reduced ? 'none' : 'color .2s ease',
        }}
      >
        {value}
      </div>
    </div>
  );
}
