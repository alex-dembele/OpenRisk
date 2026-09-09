// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// The get-started panel.
//
// THIS COMPONENT HAS NO ACTIVATION LOGIC. It renders what GET /activation/state
// returns — labels, hints, completion, deep links — and reports celebrations
// back. It does not count risks, does not read localStorage, and does not decide
// what "done" means.
//
// That is the whole fix. The previous version derived steps from client-side
// counts and persisted flags in localStorage, which produced: a checklist that
// never ticked (flag written on one device, read on another), confetti on
// re-render (the "is it done?" heuristic re-ran), and two rows struck through
// after a single import (two steps read the same count).

import { useNavigate } from 'react-router';
import { Check, ArrowRight, Sparkles, Loader2, LifeBuoy } from 'lucide-react';

import { useUIStore } from '../../store/uiStore';
import { i18n, type ActivationStep } from '../../services/activationService';
import { useActivationState, useCelebrateActivation } from './useActivation';
import type { LocaleCode } from '../../i18n/locales';

export function OnboardingChecklist() {
  const navigate = useNavigate();
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);

  const { data: state, isLoading, isError } = useActivationState();

  // The celebration is driven by the server's `celebrate` flag, once per step
  // per user. Called unconditionally (hooks rule); it no-ops without data.
  useCelebrateActivation(state, lang);

  // Loading: a slim skeleton rather than a spinner, so the dashboard does not
  // jump when the card arrives.
  if (isLoading) {
    return (
      <div
        className="rounded-[16px] p-5 mb-4"
        style={{ background: 'var(--bg-elevated)', border: '1px solid var(--border-strong)' }}
        aria-busy="true"
      >
        <div className="flex items-center gap-2 text-[13px] text-ink-soft">
          <Loader2 size={15} className="animate-spin" />
          {tr('Chargement de votre progression…', 'Loading your progress…')}
        </div>
      </div>
    );
  }

  // Error: stay quiet. A broken activation endpoint must not put an error banner
  // at the top of someone's dashboard — the rest of the page is still true.
  if (isError || !state || state.steps.length === 0) return null;

  const done = state.steps.filter((s) => s.completed).length;
  const total = state.steps.length;
  const complete = done === total;

  // Everything done: the panel has served its purpose and disappears. No
  // "dismiss" flag to store — completion is a server fact, so it stays hidden on
  // every device without anything being persisted client-side.
  if (complete) return null;

  // The step to do next: the product's promise first if still open, else the
  // first incomplete one.
  const current =
    state.steps.find((s) => s.primary && !s.completed) ?? state.steps.find((s) => !s.completed);

  return (
    <section
      className="rounded-[16px] p-5 mb-4"
      style={{
        background: 'var(--bg-elevated)',
        border: '1px solid var(--border-strong)',
        animation: 'or-fadeup .4s ease',
      }}
      aria-label={tr('Prise en main', 'Get started')}
    >
      <div className="flex items-start justify-between gap-4 mb-3.5">
        <div>
          <div className="text-[15.5px] font-bold text-ink flex items-center gap-2">
            <Sparkles size={16} style={{ color: 'var(--accent-500)' }} />
            {tr('Prise en main', 'Get started')}
          </div>
          <div className="text-[13px] text-ink-soft mt-0.5">
            {state.aha_reached_at
              ? tr(
                  'Votre posture est calculée sur vos propres données. Continuez.',
                  'Your posture is computed on your own data. Keep going.',
                )
              : tr(
                  'Quelques actions pour voir votre exposition réelle.',
                  'A few actions to see your real exposure.',
                )}
          </div>
        </div>
        <span
          className="mono text-[12.5px] font-semibold text-ink-soft shrink-0"
          aria-live="polite"
        >
          {done}/{total}
        </span>
      </div>

      <div
        className="h-1.5 rounded-full overflow-hidden mb-4"
        style={{ background: 'var(--bg-hover)' }}
        role="progressbar"
        // A progressbar with no accessible name is announced as a bare
        // percentage with nothing to say what it measures. Named, not
        // aria-hidden: the number IS the useful part of this card.
        aria-label={tr('Progression de la prise en main', 'Onboarding progress')}
        aria-valuenow={state.percent}
        aria-valuemin={0}
        aria-valuemax={100}
      >
        <div
          className="h-full rounded-full"
          style={{
            width: `${state.percent}%`,
            background: 'var(--accent)',
            transition: 'width .5s var(--ease-out, ease)',
          }}
        />
      </div>

      <ul className="flex flex-col gap-2 list-none p-0 m-0">
        {state.steps.map((step) => (
          <StepRow
            key={step.key}
            step={step}
            lang={lang}
            isCurrent={step.key === current?.key}
            onGo={() => navigate(step.deep_link)}
            ctaLabel={tr('Commencer', 'Start')}
            nowLabel={tr('À faire maintenant', 'Do this next')}
          />
        ))}
      </ul>

      {/* The tour lives here rather than as a permanent header button: it is a
          first-run aid, and someone who wants to replay it is already looking at
          the getting-started card. */}
      <button
        type="button"
        onClick={() => window.dispatchEvent(new CustomEvent('openrisk:tour'))}
        className="mt-3 inline-flex items-center gap-1.5 text-[12px] self-start"
        style={{ color: 'var(--fg-muted)' }}
      >
        <LifeBuoy size={13} />
        {tr('Revoir la visite guidée', 'Replay the product tour')}
      </button>
    </section>
  );
}

function StepRow({
  step,
  lang,
  isCurrent,
  onGo,
  ctaLabel,
  nowLabel,
}: {
  step: ActivationStep;
  lang: LocaleCode;
  isCurrent: boolean;
  onGo: () => void;
  ctaLabel: string;
  nowLabel: string;
}) {
  const label = i18n(step.label_i18n, lang);
  const hint = i18n(step.hint_i18n, lang);

  return (
    <li
      className="flex items-start gap-3 rounded-[12px] p-3 transition-colors"
      style={{
        background: isCurrent ? 'var(--accent-soft)' : 'transparent',
        border: isCurrent ? '1px solid var(--accent-line)' : '1px solid transparent',
        opacity: step.completed ? 0.66 : 1,
      }}
      data-testid={`activation-step-${step.key}`}
      data-completed={step.completed ? 'true' : 'false'}
    >
      <div
        className="w-8 h-8 rounded-[10px] flex items-center justify-center shrink-0"
        style={{
          background: step.completed
            ? 'color-mix(in srgb,var(--low) 18%,transparent)'
            : 'var(--bg-hover)',
          color: step.completed ? 'var(--low)' : isCurrent ? 'var(--accent)' : 'var(--fg-muted)',
        }}
        aria-hidden="true"
      >
        {step.completed ? (
          <Check size={17} strokeWidth={2.5} />
        ) : (
          <span className="text-[13px] font-bold">{step.order}</span>
        )}
      </div>

      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2 flex-wrap">
          <span
            className="text-[13.5px] font-semibold text-ink"
            style={{ textDecoration: step.completed ? 'line-through' : 'none' }}
          >
            {label}
          </span>
          {isCurrent && !step.completed && (
            <span
              className="text-[10px] font-bold uppercase tracking-wide px-2 py-0.5 rounded-full"
              style={{ background: 'var(--accent)', color: '#fff' }}
            >
              {nowLabel}
            </span>
          )}
        </div>
        {!step.completed && hint && (
          <div className="text-[12.5px] text-ink-soft mt-0.5 leading-snug">{hint}</div>
        )}
      </div>

      {!step.completed && (
        <button
          onClick={onGo}
          className="h-9 px-3.5 rounded-[9px] text-[12.5px] font-semibold inline-flex items-center gap-1.5 shrink-0 transition-[filter] hover:brightness-110"
          style={
            isCurrent
              ? {
                  background: 'var(--accent-solid)',
                  color: 'var(--fg-on-solid)',
                }
              : {
                  background: 'var(--bg-hover)',
                  color: 'var(--fg-primary)',
                  border: '1px solid var(--border-strong)',
                }
          }
        >
          {ctaLabel} <ArrowRight size={14} />
        </button>
      )}
    </li>
  );
}
