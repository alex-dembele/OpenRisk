// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// The two form shells every tunnel step is built from.
//
// COMPONENTS ONLY — the hooks and constants live in stepNav.ts, so Fast Refresh
// keeps working on these two. StepShell renders the retry control #438
// criterion 4 requires; useStepNav (in stepNav.ts) decides when it appears.

import { ArrowLeft, ArrowRight, Loader2 } from 'lucide-react';

import { useI18n } from '../../../hooks/useI18n';


export function Field({
  label,
  hint,
  children,
  htmlFor,
}: {
  label: string;
  hint?: string;
  children: React.ReactNode;
  htmlFor?: string;
}) {
  return (
    <div className="mb-4">
      <label htmlFor={htmlFor} className="block text-[12.5px] font-semibold text-ink mb-1.5">
        {label}
      </label>
      {children}
      {hint && <div className="text-[11.5px] text-ink-muted mt-1">{hint}</div>}
    </div>
  );
}

export function StepShell({
  title,
  subtitle,
  children,
  onBack,
  onNext,
  nextLabel,
  nextDisabled,
  busy,
  secondary,
  error,
  onRetry,
  errorLabel,
  errorHint,
  retryLabel,
}: {
  title: string;
  subtitle: string;
  children: React.ReactNode;
  onBack?: () => void;
  onNext: () => void;
  nextLabel: string;
  nextDisabled?: boolean;
  busy?: boolean;
  secondary?: React.ReactNode;
  /** True when the last save failed. The user stays on this step. */
  error?: boolean;
  onRetry?: () => void;
  errorLabel?: string;
  errorHint?: string;
  retryLabel?: string;
}) {
  const { t } = useI18n();

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        if (!busy && !nextDisabled) onNext();
      }}
      style={{ animation: 'or-fadeup .35s ease' }}
    >
      {/* tabIndex -1 so the shell can move focus here on every step change
          (criterion 12): a screen-reader user is told where they landed instead
          of being left on a button that no longer exists. Not focusable by Tab,
          only programmatically. */}
      <h1
        className="disp text-[24px] font-bold text-ink mb-1.5"
        data-step-heading
        tabIndex={-1}
        style={{ outline: 'none' }}
      >
        {title}
      </h1>
      <p className="text-[14px] text-ink-soft mb-6">{subtitle}</p>

      {children}

      {/* Criterion 4: a failed save keeps the user here, keeps their answers,
          and gives them something to press. aria-live so it is announced rather
          than merely drawn. */}
      {error && (
        <div
          className="mt-5 rounded-lg p-3 flex flex-wrap items-center gap-3"
          style={{
            background: 'color-mix(in srgb,var(--high) 10%,transparent)',
            border: '1px solid color-mix(in srgb,var(--high) 35%,transparent)',
          }}
          role="alert"
          aria-live="polite"
          data-testid="wizard-save-error"
        >
          <div className="flex-1 min-w-[200px]">
            <div className="text-[13px] font-semibold text-ink">{errorLabel}</div>
            <div className="text-[12px] text-ink-soft mt-0.5">{errorHint}</div>
          </div>
          {onRetry && (
            <button
              type="button"
              onClick={onRetry}
              data-testid="wizard-retry"
              className="h-9 px-3.5 rounded-lg text-[12.5px] font-semibold shrink-0"
              style={{ background: 'var(--accent-solid)', color: 'var(--fg-on-solid)' }}
            >
              {retryLabel}
            </button>
          )}
        </div>
      )}

      <div className="flex items-center gap-3 mt-7 flex-wrap">
        {onBack && (
          <button
            type="button"
            onClick={onBack}
            className="h-11 px-4 rounded-[10px] text-[13.5px] font-semibold text-ink inline-flex items-center gap-1.5"
            style={{ background: 'var(--bg-hover)', border: '1px solid var(--border-strong)' }}
          >
            <ArrowLeft size={15} aria-hidden="true" />
            {t('onboarding.tunnel.back')}
          </button>
        )}
        <button
          type="submit"
          disabled={busy || nextDisabled}
          data-testid="wizard-next"
          className="h-11 px-5 rounded-[10px] text-[13.5px] font-semibold inline-flex items-center gap-2 disabled:opacity-50"
          style={{
            background: 'var(--accent-solid)',
            color: 'var(--fg-on-solid)',
          }}
        >
          {busy ? <Loader2 size={15} className="animate-spin" /> : null}
          {nextLabel}
          {!busy && <ArrowRight size={15} />}
        </button>
        {secondary}
      </div>
    </form>
  );
}
