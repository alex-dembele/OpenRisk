// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// The tunnel's non-component primitives: navigation, stored answers, and the
// shared input styling.
//
// Split out of stepPrimitives.tsx so that file exports COMPONENTS ONLY. A module
// exporting both a component and a constant breaks Fast Refresh: editing the
// component reloads the module and resets whatever held the constant. The lint
// rule that says so is `react-refresh/only-export-components`, and the frontend
// runs a ratchet on it.
//
// `useStepNav` is where #438 criterion 4 lives. Read its doc comment before
// changing anything about how a step advances.

import type React from 'react';
import { useMemo, useRef } from 'react';
import { useNavigate } from 'react-router';

import type { OnboardingStepKey } from '../../../services/activationService';
import { useOnboardingState, useSaveOnboardingStep } from '../useActivation';
import { WIZARD_STEPS, stepPath } from './wizardSteps';

/** The one input shape every tunnel field uses. Shared so a field added later
 *  cannot quietly look different from the four beside it. */
export const inputCls =
  'w-full h-11 px-3.5 rounded-[10px] text-[14px] text-ink outline-none transition-colors';
export const inputStyle: React.CSSProperties = {
  background: 'var(--bg-elevated)',
  border: '1px solid var(--border-strong)',
};

/** Read a stored answer for a step, so a resumed wizard shows what was typed. */
export function useStoredAnswers(step: OnboardingStepKey): Record<string, unknown> {
  const { data } = useOnboardingState();
  return useMemo(() => (data?.answers?.[step] ?? {}) as Record<string, unknown>, [data, step]);
}

export function str(answers: Record<string, unknown>, key: string, fallback = ''): string {
  const v = answers[key];
  return typeof v === 'string' ? v : fallback;
}

/**
 * Save-then-navigate, shared by every step (#438 criterion 4).
 *
 * Three properties, and each one is a criterion rather than a preference:
 *
 *   • THE DATA IS PERSISTED BEFORE THE NEXT STEP RENDERS. No optimistic advance.
 *     This is the one place ABSOLUTE RULE #10 (optimistic updates on critical
 *     mutations) is deliberately overridden — do not "fix" it. An optimistic
 *     tunnel that fails silently strands the user in an app they cannot enter,
 *     because the guard is reading the server, not the client.
 *   • THE TARGET COMES FROM THE SERVER. `state.steps` already has auto-skipped
 *     steps removed, and the response's `current_step` is authoritative. Walking
 *     the canonical five here would navigate straight onto a hidden step.
 *   • A FAILURE KEEPS THE USER ON THE STEP WITH THEIR ANSWERS AND A RETRY. The
 *     last attempt is held so `retry()` replays it verbatim; nothing is cleared,
 *     nothing is re-typed.
 */
export function useStepNav(step: OnboardingStepKey) {
  const navigate = useNavigate();
  const save = useSaveOnboardingStep();
  const { data: state } = useOnboardingState();
  const lastAttempt = useRef<{ answers: Record<string, unknown>; direction: 1 | -1 } | null>(null);

  const visible: OnboardingStepKey[] = state?.steps?.length
    ? state.steps
    : WIZARD_STEPS.map((s) => s.key);
  const index = Math.max(
    0,
    visible.findIndex((s) => s === step),
  );

  const run = (answers: Record<string, unknown>, direction: 1 | -1) => {
    lastAttempt.current = { answers, direction };
    const target = visible[Math.min(visible.length - 1, Math.max(0, index + direction))];

    save.mutate(
      { step, answers, next: target },
      {
        // The server's own cursor wins. It has already snapped the move onto the
        // visible sequence, so this cannot land on a step the shell refuses to
        // draw — which a client-side target can.
        onSuccess: (saved) => navigate(stepPath(saved.current_step ?? target)),
        // No toast. A toast is transient and carries no retry, and criterion 4
        // asks for a control the user can actually press. StepShell renders it
        // from `error` below.
      },
    );
  };

  const retry = () => {
    const attempt = lastAttempt.current;
    if (attempt) run(attempt.answers, attempt.direction);
  };

  return {
    go: run,
    retry,
    busy: save.isPending,
    error: save.isError,
    index,
    total: visible.length,
  };
}

/**
 * Read a numeric answer. Separate from `str` because a slider's value is a
 * number and coercing through a string loses precision on the probability axis
 * (0.05 steps), which would move the highlighted matrix cell.
 *
 * A stored value that is not a finite number falls back rather than producing
 * NaN: NaN propagates through the score, the band and the grid, and the screen
 * would render an empty matrix with no clue why.
 */
export function num(answers: Record<string, unknown>, key: string, fallback: number): number {
  const v = answers[key];
  if (typeof v === 'number' && Number.isFinite(v)) return v;
  if (typeof v === 'string') {
    const parsed = Number(v);
    if (Number.isFinite(parsed)) return parsed;
  }
  return fallback;
}
