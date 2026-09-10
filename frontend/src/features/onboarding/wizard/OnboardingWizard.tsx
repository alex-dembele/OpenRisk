// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// The guided tunnel's shell (#438).
//
// It is a TUNNEL, and the word is load-bearing (criterion 1). There is no close,
// no skip, no "later", no cross, no click-outside dismiss and NO ESCAPE HANDLER
// anywhere in this subtree. Do not add one: `OnboardingGuard` denies /app until
// POST /onboarding/complete succeeds, so a dismiss affordance here would strand
// the user on a screen they were allowed to leave and an app they cannot enter.
//
// The stepper renders `state.steps` — the steps the SERVER says THIS USER will
// see, with auto-skipped ones already removed (criterion 3). It never renders
// the canonical five, and it never renders `skipped_steps`: drawing those would
// be the flash the criterion forbids.
//
// One fetch resolves the whole tunnel (criterion 2). Steps read the same cached
// query; none of them issues its own status call on mount.

import { useEffect, useRef } from 'react';
import { Outlet, useNavigate, useLocation } from 'react-router';
import { Check } from 'lucide-react';

import { useI18n } from '../../../hooks/useI18n';
import { useUIStore } from '../../../store/uiStore';
import { OpenRiskLogo } from '../../../shared/Logo';
import { useOnboardingState } from '../useActivation';
import type { OnboardingStepKey } from '../../../services/activationService';
import { WIZARD_STEPS, WIZARD_STEP_LABELS, stepPath } from './wizardSteps';

export function OnboardingWizard() {
  const lang = useUIStore((s) => s.lang);
  const { t } = useI18n();
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const navigate = useNavigate();
  const { pathname } = useLocation();
  const { data: state, isLoading, isError } = useOnboardingState();

  // Criterion 3: the visible sequence is the server's, never the catalogue's.
  // The fallback applies only before the first response — showing an empty rail
  // would make the tunnel look broken on a slow connection.
  const steps: OnboardingStepKey[] = state?.steps?.length
    ? state.steps
    : WIZARD_STEPS.map((s) => s.key);

  const activeKey = (pathname.split('/')[2] ?? steps[0]) as OnboardingStepKey;
  const activeIndex = Math.max(
    0,
    steps.findIndex((s) => s === activeKey),
  );
  // Progress reflects steps FINISHED, not the one being filled in — a bar that
  // reads 20% while you are still on step 1 is a bar that lies.
  const percent = Math.round((activeIndex / steps.length) * 100);

  // Criterion 12: focus moves to the step heading on every step change, so a
  // screen-reader user is told where they are instead of being left on a button
  // that no longer exists. The step components own the heading and mark it
  // `data-step-heading`; this shell is what moves focus to it.
  const headingAnchor = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const heading = headingAnchor.current?.querySelector<HTMLElement>('[data-step-heading]');
    heading?.focus();
  }, [activeKey]);

  return (
    <div className="min-h-screen flex flex-col" style={{ background: 'var(--bg-primary)' }}>
      {/* No close button, by design. See the file header before adding one. */}
      <header
        className="px-5 sm:px-8 py-4 flex items-center justify-between gap-4"
        style={{ borderBottom: '1px solid var(--border-subtle)' }}
      >
        <div className="flex items-center gap-2.5">
          <OpenRiskLogo size={26} />
          <span className="text-[15px] font-bold text-ink">OpenRisk</span>
        </div>
        <div className="text-[12.5px] text-ink-soft" data-testid="wizard-step-of">
          {t('onboarding.tunnel.stepOf', { current: activeIndex + 1, total: steps.length })}
        </div>
      </header>

      <div className="px-5 sm:px-8 pt-5">
        <div
          className="h-1.5 rounded-full overflow-hidden"
          style={{ background: 'var(--bg-hover)' }}
          role="progressbar"
          aria-valuenow={percent}
          aria-valuemin={0}
          aria-valuemax={100}
          aria-label={tr('Progression de la configuration', 'Setup progress')}
        >
          <div
            className="h-full rounded-full"
            style={{
              width: `${percent}%`,
              background: 'var(--accent)',
              // Criterion 13: the bar does not slide for a viewer who asked not
              // to be animated. Handled in CSS rather than JS so it also covers
              // the very first paint.
              transition: 'width .45s var(--ease-out, ease)',
            }}
          />
        </div>

        <ol
          className="flex flex-wrap items-center gap-x-4 gap-y-1.5 mt-3 list-none p-0 m-0"
          data-testid="wizard-stepper"
        >
          {steps.map((key, i) => {
            const done = i < activeIndex;
            const active = i === activeIndex;
            const label = WIZARD_STEP_LABELS[key];
            return (
              <li key={key}>
                <button
                  type="button"
                  // Back-navigation is allowed and encouraged; forward is not,
                  // because a step ahead has no answers to show yet. This is NOT
                  // a skip affordance — it cannot move past the cursor.
                  disabled={i > activeIndex}
                  onClick={() => navigate(stepPath(key))}
                  aria-current={active ? 'step' : undefined}
                  className="inline-flex items-center gap-1.5 text-[12.5px] font-semibold disabled:cursor-default"
                  style={{
                    color: active
                      ? 'var(--accent)'
                      : done
                        ? 'var(--fg-secondary)'
                        : 'var(--fg-muted)',
                  }}
                >
                  <span
                    className="w-[18px] h-[18px] rounded-full inline-flex items-center justify-center text-[10px] font-bold"
                    style={{
                      background: done
                        ? 'color-mix(in srgb,var(--low) 20%,transparent)'
                        : active
                          ? 'var(--accent)'
                          : 'var(--bg-hover)',
                      color: done ? 'var(--low)' : active ? 'var(--fg-on-solid)' : 'var(--fg-muted)',
                    }}
                  >
                    {done ? <Check size={11} strokeWidth={3} /> : i + 1}
                  </span>
                  {tr(label.fr, label.en)}
                </button>
              </li>
            );
          })}
        </ol>
      </div>

      <main className="flex-1 px-5 sm:px-8 py-6 flex justify-center">
        <div className="w-full max-w-[640px]" ref={headingAnchor}>
          {isLoading && !state ? (
            <WizardSkeleton label={t('onboarding.tunnel.loading')} />
          ) : isError && !state ? (
            // The tunnel blocks the app, so a failed load must say so rather
            // than render an empty frame the user cannot act on or leave.
            <div className="py-10" role="alert">
              <p className="text-[14px] text-ink m-0">{t('onboarding.tunnel.loadFailed')}</p>
              <button
                type="button"
                onClick={() => navigate(0)}
                className="mt-4 px-4 py-2 rounded-lg text-[13px] font-semibold"
                style={{ background: 'var(--accent-solid)', color: 'var(--fg-on-solid)' }}
              >
                {t('onboarding.tunnel.retry')}
              </button>
            </div>
          ) : (
            <Outlet />
          )}
        </div>
      </main>
    </div>
  );
}

function WizardSkeleton({ label }: { label: string }) {
  // ABSOLUTE RULE #8: skeletons, never a full-page spinner.
  return (
    <div className="py-6" aria-busy="true" aria-label={label}>
      <div className="h-6 w-56 rounded or-skeleton mb-3" />
      <div className="h-4 w-full max-w-[420px] rounded or-skeleton mb-7" />
      <div className="h-11 w-full rounded-lg or-skeleton mb-3" />
      <div className="h-11 w-full rounded-lg or-skeleton mb-6" />
      <div className="h-10 w-36 rounded-lg or-skeleton" />
    </div>
  );
}
