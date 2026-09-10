// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// The Posture Reveal (#438) — the screen that DEFINES the Aha moment.
//
// Two rules govern every line below and neither is a preference:
//
//   • CRITERION 7 — every number on this page comes from the server payload.
//     There is no default, no `?? 0`, no sample and no placeholder. A plausible
//     fabricated figure here would be forwarded to a CISO as fact.
//   • CRITERION 8 — an unrevealable posture is an ERROR STATE, not an empty one
//     with zeros. The server answers 404 and records nothing; this renders that
//     refusal plainly rather than dressing it as a quiet success.
//
// It is also the only onboarding surface that leaves the 720px container: it is
// composed, not a grid of identical cards.

import { useNavigate } from 'react-router';
import { useState } from 'react';
import { AlertTriangle, ArrowRight, Check, Copy, Users } from 'lucide-react';

import { useI18n } from '../../hooks/useI18n';
import type { PostureRiskView, PostureSummary } from '../../services/activationService';
import { useOnboardingState, usePosture, useCountUp, usePrefersReducedMotion } from './useActivation';

export function PosturePage() {
  const { t } = useI18n();
  const { data, isLoading, isError } = usePosture();

  if (isLoading) return <PostureLoading label={t('onboarding.posture.loading')} />;

  // Criterion 8's client half: a refusal renders as a refusal. `usePosture` does
  // not retry, so this is reached once and immediately rather than after three
  // silent reveal attempts.
  if (isError || !data) return <PostureError />;

  return <PostureReveal summary={data} />;
}

/* -------------------------------------------------------------------------- */

function PostureReveal({ summary }: { summary: PostureSummary }) {
  const { t } = useI18n();
  const reduced = usePrefersReducedMotion();

  // coverage_percent is `number | null`. Null means "nothing is applicable",
  // which is a different statement from 0% and must never be rendered as one.
  // IsRevealable() guarantees it is non-null here; the guard stays because a
  // future payload change must fail loudly rather than print "null%".
  const coverage = summary.coverage_percent;

  return (
    <div className="w-full px-5 sm:px-8 py-8" data-testid="posture-reveal">
      <header className="mb-7">
        <h1 className="text-[26px] font-bold text-ink m-0">{t('onboarding.posture.title')}</h1>
        <p className="text-[13.5px] text-ink-soft mt-1.5 m-0">{t('onboarding.posture.lead')}</p>
      </header>

      <div className="grid gap-4 sm:grid-cols-2 mb-8">
        <Figure
          testId="posture-risk-count"
          value={summary.risks.total}
          label={t('onboarding.posture.riskCount', { count: summary.risks.total })}
          detail={levelBreakdown(summary.risks.by_level)}
          reduced={reduced}
        />
        {coverage !== null && (
          <Figure
            testId="posture-coverage"
            value={coverage}
            suffix="%"
            label={t('onboarding.posture.coverage', { percent: `${coverage}%` })}
            detail={t('onboarding.posture.coverageDetail', {
              implemented: summary.controls.implemented,
              applicable: summary.controls.total - summary.controls.not_applicable,
            })}
            reduced={reduced}
          />
        )}
      </div>

      <section aria-labelledby="posture-top-risks">
        <h2 id="posture-top-risks" className="text-[15px] font-bold text-ink m-0 mb-3">
          {t('onboarding.posture.topRisks')}
        </h2>
        <ul className="list-none p-0 m-0 flex flex-col gap-2.5">
          {summary.top_risks.map((risk) => (
            <RiskRow key={risk.id} risk={risk} />
          ))}
        </ul>
      </section>

      <LandingCta />

      <InviteBlock />

      <p className="text-[11.5px] text-ink-muted mt-6 m-0">
        {t('onboarding.posture.formulaVersion', { version: summary.residual_formula_version })}
      </p>
    </div>
  );
}

/**
 * The way out of the reveal.
 *
 * The tunnel now ends here rather than on the goal's landing screen, so this is
 * where `landing` is offered — a reveal with no forward control is a dead end,
 * and the user has just been told something worth acting on.
 */
function LandingCta() {
  const { t } = useI18n();
  const navigate = useNavigate();
  const { data: onboarding } = useOnboardingState();
  const landing = onboarding?.landing || '/';

  return (
    <button
      type="button"
      onClick={() => navigate(landing)}
      data-testid="posture-continue"
      className="mt-8 px-5 py-2.5 rounded-lg text-[13.5px] font-semibold inline-flex items-center gap-2"
      style={{ background: 'var(--accent-solid)', color: 'var(--fg-on-solid)' }}
    >
      {t('onboarding.posture.continue')}
      <ArrowRight size={15} aria-hidden="true" />
    </button>
  );
}

/**
 * Team invitations, on the reveal (#438).
 *
 * They used to be the tunnel's last step, which asked people to invite
 * colleagues to look at nothing. Here they sit under a posture the user can
 * actually forward, which is the moment the invitation means something.
 *
 * NOTE ON THE DESIGN SYSTEM: the issue says to reuse `TagInput` for this. No
 * such component exists in `shared/` — this is recorded rather than silently
 * worked around. A textarea keeps the shape the retired team step used; when the
 * design system grows a real `TagInput`, this is the call site to migrate.
 *
 * Nothing is SENT from here. Invitations carry roles and permissions, which
 * Settings › Members owns; duplicating that flow would mean two places to keep
 * correct.
 */
function InviteBlock() {
  const { t } = useI18n();
  const [copied, setCopied] = useState(false);
  const shareLink = `${window.location.origin}/register`;

  const copy = () => {
    void navigator.clipboard?.writeText(shareLink).then(() => {
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    });
  };

  return (
    <section
      className="mt-8 rounded-xl p-5 flex flex-wrap items-center gap-4"
      style={{ background: 'var(--bg-surface)', border: '1px solid var(--border-subtle)' }}
      aria-labelledby="posture-invite"
      data-testid="posture-invite"
    >
      <Users size={18} aria-hidden="true" style={{ color: 'var(--accent-500)' }} />
      <div className="flex-1 min-w-[220px]">
        <h2 id="posture-invite" className="text-[13.5px] font-semibold text-ink m-0">
          {t('onboarding.posture.invite.heading')}
        </h2>
        <p className="text-[12px] text-ink-soft mt-0.5 m-0">
          {t('onboarding.posture.invite.body')}
        </p>
      </div>
      <button
        type="button"
        onClick={copy}
        data-testid="posture-invite-copy"
        className="h-9 px-3.5 rounded-lg text-[12.5px] font-semibold inline-flex items-center gap-1.5 shrink-0"
        style={{ background: 'var(--bg-hover)', border: '1px solid var(--border-strong)' }}
      >
        {copied ? <Check size={14} /> : <Copy size={14} />}
        {copied ? t('onboarding.posture.invite.copied') : t('onboarding.posture.invite.copy')}
      </button>
    </section>
  );
}

/** One headline number. Counts up unless the viewer asked us not to animate. */
function Figure({
  value,
  suffix,
  label,
  detail,
  reduced,
  testId,
}: {
  value: number;
  suffix?: string;
  label: string;
  detail: string;
  reduced: boolean;
  testId: string;
}) {
  const animated = useCountUp(value);
  // Criterion 13: reduced motion renders the FINAL state directly. Not a faster
  // count — no count.
  const shown = reduced ? value : animated;

  return (
    <div
      className="rounded-xl p-5"
      style={{ background: 'var(--bg-surface)', border: '1px solid var(--border-subtle)' }}
      data-testid={testId}
    >
      {/* The number animates; the accessible name does not. A screen reader
          announcing every intermediate frame would be unusable, so the live
          value is aria-hidden and the settled label carries the meaning. */}
      <div className="text-[34px] font-bold text-ink leading-none" aria-hidden="true">
        {Math.round(shown)}
        {suffix}
      </div>
      <div className="text-[13px] font-semibold text-ink mt-2">{label}</div>
      <div className="text-[12px] text-ink-soft mt-1">{detail}</div>
    </div>
  );
}

function RiskRow({ risk }: { risk: PostureRiskView }) {
  const { t } = useI18n();
  const measured = risk.residual.coverage.measured;

  return (
    <li
      className="rounded-lg p-4 flex flex-wrap items-center justify-between gap-3"
      style={{ background: 'var(--bg-surface)', border: '1px solid var(--border-subtle)' }}
      data-testid="posture-top-risk"
    >
      <div className="min-w-0 flex-1">
        <div className="text-[13.5px] font-semibold text-ink truncate">{risk.title}</div>
        {!measured && (
          <div className="text-[11.5px] text-ink-muted mt-0.5">
            {t('onboarding.posture.noResidual')}
          </div>
        )}
      </div>

      <div className="flex items-center gap-3 shrink-0">
        <Score label={t('onboarding.posture.inherent')} value={risk.residual.inherent} muted />
        <ArrowRight size={14} className="text-ink-muted" aria-hidden="true" />
        <Score
          label={t('onboarding.posture.residual')}
          value={risk.residual.value}
          level={risk.residual.level}
          testId="posture-residual"
        />
      </div>
    </li>
  );
}

function Score({
  label,
  value,
  level,
  muted,
  testId,
}: {
  label: string;
  value: number;
  level?: string;
  muted?: boolean;
  testId?: string;
}) {
  return (
    <div className="text-right" data-testid={testId}>
      <div className="text-[10.5px] uppercase tracking-wide text-ink-muted">{label}</div>
      <div
        className="text-[17px] font-bold tabular-nums"
        style={{ color: muted ? 'var(--fg-secondary)' : levelColor(level) }}
      >
        {value}
      </div>
    </div>
  );
}

function PostureLoading({ label }: { label: string }) {
  // Skeletons, never a full-page spinner (ABSOLUTE RULE #8).
  return (
    <div className="w-full px-5 sm:px-8 py-8" aria-busy="true" aria-label={label}>
      <div className="h-7 w-64 rounded or-skeleton mb-3" />
      <div className="h-4 w-96 rounded or-skeleton mb-8" />
      <div className="grid gap-4 sm:grid-cols-2 mb-8">
        <div className="h-32 rounded-xl or-skeleton" />
        <div className="h-32 rounded-xl or-skeleton" />
      </div>
      <div className="flex flex-col gap-2.5">
        {[0, 1, 2].map((i) => (
          <div key={i} className="h-16 rounded-lg or-skeleton" />
        ))}
      </div>
    </div>
  );
}

/**
 * Criterion 8's screen. Deliberately NOT an empty state with zeros: the server
 * recorded nothing and measured nothing, and saying so is the honest report.
 */
function PostureError() {
  const { t } = useI18n();
  const navigate = useNavigate();

  return (
    <div className="w-full px-5 sm:px-8 py-16 flex justify-center">
      <div className="max-w-[440px] text-center" role="alert" data-testid="posture-error">
        <AlertTriangle size={28} className="mx-auto mb-3" style={{ color: 'var(--high)' }} />
        <h1 className="text-[19px] font-bold text-ink m-0">
          {t('onboarding.posture.error.heading')}
        </h1>
        <p className="text-[13.5px] text-ink-soft mt-2 m-0">{t('onboarding.posture.error.body')}</p>
        <button
          type="button"
          onClick={() => navigate(0)}
          className="mt-5 px-4 py-2 rounded-lg text-[13px] font-semibold"
          style={{ background: 'var(--accent-solid)', color: 'var(--fg-on-solid)' }}
        >
          {t('onboarding.posture.error.action')}
        </button>
      </div>
    </div>
  );
}

/* -------------------------------------------------------------------------- */

/** "2 critical · 4 high · 5 medium" — from the tenant's own banding, nothing invented. */
function levelBreakdown(byLevel: Record<string, number>): string {
  return (['critical', 'high', 'medium', 'low'] as const)
    .filter((level) => (byLevel[level] ?? 0) > 0)
    .map((level) => `${byLevel[level]} ${level}`)
    .join(' · ');
}

function levelColor(level?: string): string {
  switch (level) {
    case 'critical':
      return 'var(--critical)';
    case 'high':
      return 'var(--high)';
    case 'medium':
      return 'var(--medium)';
    case 'low':
      return 'var(--low)';
    default:
      return 'var(--fg-primary)';
  }
}

export default PosturePage;
