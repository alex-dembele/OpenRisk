// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// TanStack Query hooks over the activation & onboarding endpoints, plus the
// celebration effect.
//
// Invalidation is the point of this file. The old checklist stopped ticking
// because nothing told it to look again after a mutation; here every mutation
// that can complete a step invalidates ACTIVATION_QUERY_KEY, and the key is
// exported so any feature that creates a risk / imports a framework / invites a
// teammate can do the same.

import { useEffect, useRef, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import {
  activationService,
  i18n,
  type ActivationState,
  type ActivationStep,
  type Lang,
  type OnboardingState,
  type OnboardingStepKey,
  type PostureSummary,
  type Recognition,
  type StarterRiskOffer,
  type AdoptStarterRisksResult,
} from '../../services/activationService';
import { confetti } from '../../shared/celebrate';
import { useUIStore } from '../../store/uiStore';

/** Shared key — invalidate it after any mutation that can complete a step. */
export const ACTIVATION_QUERY_KEY = ['activation', 'state'];
export const ONBOARDING_QUERY_KEY = ['onboarding', 'state'];
export const ONBOARDING_SUGGESTIONS_KEY = ['onboarding', 'suggestions'];
export const POSTURE_QUERY_KEY = ['posture'];
export const RECOGNITION_QUERY_KEY = ['onboarding', 'recognition'];
export const STARTER_RISKS_QUERY_KEY = ['onboarding', 'starter-risks'];

/** The activation checklist, straight from the server. */
export function useActivationState(enabled = true) {
  return useQuery({
    queryKey: ACTIVATION_QUERY_KEY,
    queryFn: () => activationService.getState(),
    enabled,
    staleTime: 15_000,
  });
}

/**
 * Invalidate activation state. Call after creating a risk, importing a
 * framework, adding an asset, planning a mitigation, inviting a teammate or
 * generating a report — the panel then re-reads the server rather than guessing.
 */
export function useInvalidateActivation() {
  const qc = useQueryClient();
  return () => {
    void qc.invalidateQueries({ queryKey: ACTIVATION_QUERY_KEY });
  };
}

/**
 * Fires the celebration for steps the SERVER says to celebrate, then reports it
 * so it never fires again for this user.
 *
 * Idempotence has three layers, because this is exactly where the old
 * implementation went wrong:
 *   1. the server only sets `celebrate` while the acknowledgement is missing;
 *   2. this hook keeps an in-flight set so a re-render during the round-trip
 *      cannot fire twice;
 *   3. the acknowledgement endpoint is idempotent.
 *
 * Under prefers-reduced-motion there is no burst — the milestone is announced as
 * a toast instead, so the feedback is never simply lost.
 */
export function useCelebrateActivation(state: ActivationState | undefined, lang: Lang) {
  const qc = useQueryClient();
  const inFlight = useRef<Set<string>>(new Set());

  useEffect(() => {
    if (!state) return;
    const pending = state.steps.filter((s) => s.celebrate && !inFlight.current.has(s.key));
    if (pending.length === 0) return;

    const reduced =
      typeof window !== 'undefined' &&
      window.matchMedia?.('(prefers-reduced-motion: reduce)').matches === true;

    pending.forEach((step) => {
      inFlight.current.add(step.key);

      if (reduced) {
        /* Imported at call time, not at module scope. This hook is reachable
           from main.tsx without a lazy boundary, so a static import put 64 KB of
           raw sonner source in the ENTRY chunk to service a celebration that
           fires only after a step completes. Fire-and-forget: the toast is
           feedback, and nothing downstream waits on it. */
        void import('sonner').then(({ toast }) =>
          toast.success(
            `${lang === 'fr' ? 'Étape terminée' : 'Step complete'} — ${i18n(step.label_i18n, lang)}`,
          ),
        );
      } else {
        // A slightly bigger burst for the step that IS the product's promise.
        confetti(step.primary ? 90 : 55);
      }

      activationService
        .markCelebrated(step.key)
        .then(() => qc.invalidateQueries({ queryKey: ACTIVATION_QUERY_KEY }))
        .catch(() => {
          // Let it be retried on the next load rather than swallowing the
          // milestone: an unacknowledged step simply celebrates again later.
          inFlight.current.delete(step.key);
        });
    });
  }, [state, lang, qc]);
}

/** The wizard's resumable state; also the source of truth for the route guard. */
export function useOnboardingState(enabled = true) {
  return useQuery({
    queryKey: ONBOARDING_QUERY_KEY,
    queryFn: () => activationService.getOnboardingState(),
    enabled,
    // The guard reads this on every protected navigation: keep it fresh but not
    // chatty.
    staleTime: 30_000,
    retry: 1,
  });
}

export function useSaveOnboardingStep() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      step,
      answers,
      next,
    }: {
      step: OnboardingStepKey;
      answers: Record<string, unknown>;
      next?: OnboardingStepKey;
    }) => activationService.saveStep(step, answers, next),
    onSuccess: (state: OnboardingState) => {
      qc.setQueryData(ONBOARDING_QUERY_KEY, state);
      // The profile step completes a checklist row server-side.
      void qc.invalidateQueries({ queryKey: ACTIVATION_QUERY_KEY });
      void qc.invalidateQueries({ queryKey: ONBOARDING_SUGGESTIONS_KEY });
      // The starter set is scoped by the sector and country step 1 stores, so a
      // saved step can change which eight statements step 2 must offer.
      void qc.invalidateQueries({ queryKey: STARTER_RISKS_QUERY_KEY });
    },
  });
}

export function useCompleteOnboarding() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => activationService.complete(),
    onSuccess: (state: OnboardingState) => {
      qc.setQueryData(ONBOARDING_QUERY_KEY, state);
      void qc.invalidateQueries({ queryKey: ACTIVATION_QUERY_KEY });
    },
  });
}

/** Sector/goal-driven suggestions; overrides preview a choice before saving it. */
export function useOnboardingSuggestions(params?: {
  industry?: string;
  country?: string;
  goal?: string;
}) {
  return useQuery({
    queryKey: [
      ...ONBOARDING_SUGGESTIONS_KEY,
      params?.industry ?? '',
      params?.country ?? '',
      params?.goal ?? '',
    ],
    queryFn: () => activationService.getSuggestions(params),
    staleTime: 5 * 60_000,
  });
}

/**
 * The Posture Reveal.
 *
 * `retry: false` is deliberate and is criterion 8's client half. A 404 here is
 * not a transient failure, it is the server saying "there is nothing to reveal
 * and I recorded nothing" — retrying it three times would fire three reveal
 * attempts and turn one honest refusal into a spinner that never resolves.
 *
 * `staleTime: 0` because the reveal is a measured moment, not a cached view: a
 * user who adds a control and comes back must see the residual move.
 */
export function usePosture(enabled = true) {
  return useQuery<PostureSummary>({
    queryKey: POSTURE_QUERY_KEY,
    queryFn: () => activationService.getPosture(),
    enabled,
    retry: false,
    staleTime: 0,
  });
}

/** What the tenant already holds — the recognition screen (criterion 9). */
export function useRecognition(enabled = true) {
  return useQuery<Recognition>({
    queryKey: RECOGNITION_QUERY_KEY,
    queryFn: () => activationService.getRecognition(),
    enabled,
    staleTime: 60_000,
  });
}

/** The eight statements step 2 renders, scoped by the stored sector/country. */
export function useStarterRisks(enabled = true) {
  return useQuery<StarterRiskOffer>({
    queryKey: STARTER_RISKS_QUERY_KEY,
    queryFn: () => activationService.getStarterRisks(),
    enabled,
    // The set only changes when the sector or country does, and both are saved
    // one step earlier. The step-1 save invalidates this key.
    staleTime: 5 * 60_000,
  });
}

/**
 * Adopt the three chosen statements.
 *
 * `retry: false` on purpose. This WRITES REAL ROWS into a customer's register:
 * an automatic retry over a request whose failure mode is ambiguous (did the
 * server write before it timed out?) is how a register ends up with six rows
 * instead of three. The server's own idempotence guard answers 409 on a genuine
 * second attempt, and the caller treats that as success.
 */
export function useAdoptStarterRisks() {
  const qc = useQueryClient();
  // The rows are written in the language the user is READING. Resolved here
  // rather than at the call site so every caller gets it right by default.
  const lang = useUIStore((s) => s.lang);
  return useMutation<AdoptStarterRisksResult, unknown, string[]>({
    mutationFn: (keys: string[]) => activationService.adoptStarterRisks(keys, lang),
    retry: false,
    onSuccess: () => {
      // New risks exist now: the checklist's first_risk row, the offer's
      // already_adopted flag and the posture all move.
      void qc.invalidateQueries({ queryKey: ACTIVATION_QUERY_KEY });
      void qc.invalidateQueries({ queryKey: STARTER_RISKS_QUERY_KEY });
      void qc.invalidateQueries({ queryKey: POSTURE_QUERY_KEY });
    },
  });
}

/**
 * True when the viewer has asked not to be animated.
 *
 * Read through a hook rather than a media query inside each component so a
 * single place decides, and so criterion 13 can be tested by mocking one thing.
 * Subscribes to changes: a user who flips the OS setting with the tab open must
 * not have to reload to be obeyed.
 */
export function usePrefersReducedMotion(): boolean {
  const [reduced, setReduced] = useState(() => {
    if (typeof window === 'undefined' || !window.matchMedia) return false;
    return window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  });

  useEffect(() => {
    if (typeof window === 'undefined' || !window.matchMedia) return;
    const query = window.matchMedia('(prefers-reduced-motion: reduce)');
    const onChange = (e: MediaQueryListEvent) => setReduced(e.matches);
    query.addEventListener('change', onChange);
    return () => query.removeEventListener('change', onChange);
  }, []);

  return reduced;
}

/**
 * Counts from 0 to `target` over `durationMs`, or lands on `target` immediately
 * when the viewer prefers reduced motion (criterion 13).
 *
 * The reduced-motion branch is not a shorter animation, it is NO animation: the
 * final state renders directly, which is what the criterion asks for and what a
 * vestibular disorder requires.
 */
export function useCountUp(target: number, durationMs = 900): number {
  const reduced = usePrefersReducedMotion();
  // Progress, not the value. Keeping the 0..1 ratio in state rather than the
  // scaled number means the reduced-motion branch needs no setState at all —
  // it simply returns the target — and the effect never calls setState
  // synchronously in its own body.
  const [progress, setProgress] = useState(0);

  useEffect(() => {
    if (reduced) return;

    let frame = 0;
    const start = performance.now();
    const tick = (now: number) => {
      const ratio = Math.min(1, (now - start) / durationMs);
      setProgress(ratio);
      if (ratio < 1) frame = requestAnimationFrame(tick);
    };
    frame = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(frame);
  }, [target, durationMs, reduced]);

  // Criterion 13: reduced motion renders the FINAL state directly. Not a
  // shorter animation — no animation.
  if (reduced) return target;
  // easeOutCubic: fast then settling, so the number reads as "computed" rather
  // than as a slot machine.
  return target * (1 - Math.pow(1 - progress, 3));
}

/** Convenience: the next step the user should act on, or undefined when done. */
export function nextActionableStep(state: ActivationState | undefined): ActivationStep | undefined {
  if (!state) return undefined;
  // The primary step first if it is still open — it is the one that proves the
  // product. Otherwise the first incomplete step in order.
  const primary = state.steps.find((s) => s.primary && !s.completed);
  return primary ?? state.steps.find((s) => !s.completed);
}
