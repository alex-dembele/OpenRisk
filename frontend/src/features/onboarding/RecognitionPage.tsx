// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// The recognition screen (#438 criterion 9).
//
// For the tenant that was already configured before any of this shipped. #234's
// backfill made their checklist truthful and then stopped; their activation
// question is not "how do I create a risk", it is "what does OpenRisk already
// know about us that we did not".
//
// Every number here is the tenant's own count. Same rule as the reveal: no
// sample, no demo, no placeholder — a fabricated "214 risks" shown to a bank
// that has 12 would end the relationship on the first screen.

import { useNavigate } from 'react-router';
import { AlertTriangle } from 'lucide-react';

import { useI18n } from '../../hooks/useI18n';
import type { RecognitionCounts } from '../../services/activationService';
import { useCountUp, usePrefersReducedMotion, useRecognition } from './useActivation';

export function RecognitionPage() {
  const { t } = useI18n();
  const navigate = useNavigate();
  const { data, isLoading, isError } = useRecognition();

  if (isLoading) return <RecognitionSkeleton label={t('onboarding.recognition.loading')} />;

  if (isError || !data) {
    return (
      <div className="w-full max-w-[720px] mx-auto px-5 py-16 text-center" role="alert">
        <AlertTriangle size={26} className="mx-auto mb-3" style={{ color: 'var(--high)' }} />
        <p className="text-[14px] text-ink m-0">{t('onboarding.recognition.error')}</p>
      </div>
    );
  }

  const rows = countRows(data.counts);

  return (
    <div className="w-full max-w-[720px] mx-auto px-5 py-10" data-testid="recognition">
      <h1 className="text-[24px] font-bold text-ink m-0">{t('onboarding.recognition.title')}</h1>
      <p className="text-[13.5px] text-ink-soft mt-2 mb-7 m-0">
        {t('onboarding.recognition.lead')}
      </p>

      {/* An empty list is impossible on this screen: the guard only routes a
          RECOGNISED tenant here, and recognition means at least one of these is
          non-zero. Rendering zero rows would be a routing bug, not an empty
          state, so there is deliberately no empty-state branch to hide it. */}
      <dl className="grid gap-3 sm:grid-cols-3 m-0">
        {rows.map(({ key, value }) => (
          <CountTile key={key} value={value} label={t(`onboarding.recognition.${key}`)} />
        ))}
      </dl>

      <button
        type="button"
        onClick={() => navigate('/posture')}
        className="mt-8 px-5 py-2.5 rounded-lg text-[13.5px] font-semibold"
        style={{ background: 'var(--accent-solid)', color: 'var(--fg-on-solid)' }}
      >
        {t('onboarding.recognition.action')}
      </button>
    </div>
  );
}

function CountTile({ value, label }: { value: number; label: string }) {
  const reduced = usePrefersReducedMotion();
  const animated = useCountUp(value);
  // Criterion 13 again: the final state renders directly under reduced motion.
  const shown = reduced ? value : animated;

  return (
    <div
      className="rounded-xl p-4"
      style={{ background: 'var(--bg-surface)', border: '1px solid var(--border-subtle)' }}
    >
      {/* The visible number animates and is hidden from assistive tech; the
          <dt>/<dd> pair below carries the settled, announceable value. */}
      <div className="text-[28px] font-bold text-ink leading-none" aria-hidden="true">
        {Math.round(shown)}
      </div>
      <dt className="sr-only">{label}</dt>
      <dd className="text-[12.5px] text-ink-soft mt-1.5 m-0">
        <span className="sr-only">{value} </span>
        {label}
      </dd>
    </div>
  );
}

function RecognitionSkeleton({ label }: { label: string }) {
  return (
    <div className="w-full max-w-[720px] mx-auto px-5 py-10" aria-busy="true" aria-label={label}>
      <div className="h-6 w-72 rounded or-skeleton mb-3" />
      <div className="h-4 w-full max-w-[440px] rounded or-skeleton mb-8" />
      <div className="grid gap-3 sm:grid-cols-3">
        {[0, 1, 2, 3, 4, 5].map((i) => (
          <div key={i} className="h-24 rounded-xl or-skeleton" />
        ))}
      </div>
    </div>
  );
}

/**
 * Only the non-zero counts, in the order that answers the question best: what
 * you have assessed, against what, and over what estate.
 *
 * Zeros are DROPPED rather than shown as "0 assets". This screen exists to tell
 * a customer what the product already knows about them; listing what it does not
 * know turns a recognition into an audit of their gaps, on the one screen whose
 * job is the opposite.
 */
function countRows(counts: RecognitionCounts): { key: keyof RecognitionCounts; value: number }[] {
  const order: (keyof RecognitionCounts)[] = [
    'risks',
    'frameworks',
    'controls',
    'assets',
    'members',
  ];
  return order.map((key) => ({ key, value: counts[key] })).filter(({ value }) => value > 0);
}

export default RecognitionPage;
