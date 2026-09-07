// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { render, screen, cleanup } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { TrendCard } from '../DashboardPage';
import type { PeriodSelection } from '../period';
import { useUIStore } from '../../../store/uiStore';
import type { RiskTrend, RiskTrendPoint } from '../useCommandCenter';

// The three cases issue 343 requires turn on ONE decision: is this widget empty
// because the register is empty, or because the selected window is? The two
// have opposite remedies — "create your first risk" versus "widen the period" —
// and showing the wrong one tells a tenant with a populated register that it has
// no risks.
//
// The decision was previously derived from the series' last `cumulative_total`.
// That is a stock counted at each bucket's end, so any window in the past reads
// 0 for a register first populated afterwards, and the widget concluded the
// tenant was empty. These tests pin the decision to the REGISTER's own total.

const PERIOD: PeriodSelection = { kind: 'preset', preset: '30d' };

function point(bucket: string, opened: number, cumulative: number): RiskTrendPoint {
  return {
    bucket,
    opened,
    opened_by_band: opened ? { CRITICAL: opened } : {},
    cumulative_total: cumulative,
  };
}

function trendOf(points: RiskTrendPoint[]): RiskTrend {
  return { from: '2026-08-04', to: '2026-09-03', granularity: 'day', points };
}

function renderTrend(props: {
  trend?: RiskTrend;
  registerTotal: number;
  onWidenPeriod?: () => void;
}) {
  return render(
    <TrendCard
      trend={props.trend}
      registerTotal={props.registerTotal}
      isLoading={false}
      error={null}
      retry={() => {}}
      selection={PERIOD}
      onWidenPeriod={props.onWidenPeriod ?? (() => {})}
    />,
  );
}

// The card reads its language from the store, not from a prop. Pinning it makes
// these assertions independent of the app's default locale, which is French.
beforeEach(() => {
  useUIStore.setState({ lang: 'en' });
});

afterEach(cleanup);

describe('TrendCard empty-state semantics — issue 343', () => {
  // Case 1 of the issue.
  it('a tenant with NO risks is told the register is empty, not the period', () => {
    renderTrend({ trend: trendOf([]), registerTotal: 0 });

    expect(screen.getByText(/no trend yet/i)).toBeInTheDocument();
    expect(screen.queryByText(/nothing in this period/i)).not.toBeInTheDocument();
    // Widening the window cannot help a register that holds nothing, so the
    // action that says so must not be offered.
    expect(screen.queryByRole('button', { name: /show all time/i })).not.toBeInTheDocument();
  });

  // Case 2 of the issue.
  it('a tenant with risks AND activity in the window gets the chart, not an empty state', () => {
    renderTrend({
      trend: trendOf([point('2026-09-01', 2, 7), point('2026-09-02', 1, 8)]),
      registerTotal: 8,
    });

    expect(screen.getByRole('img')).toBeInTheDocument();
    expect(screen.queryByText(/no trend yet/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/nothing in this period/i)).not.toBeInTheDocument();
  });

  // Case 3 of the issue — the one the bug got wrong.
  it('a tenant with risks but NO activity in the window is told the PERIOD is empty', () => {
    const onWidenPeriod = vi.fn();
    renderTrend({
      // Buckets exist, nothing was opened in them. The register holds 8 risks,
      // all created before this window.
      trend: trendOf([point('2026-09-01', 0, 0), point('2026-09-02', 0, 0)]),
      registerTotal: 8,
      onWidenPeriod,
    });

    expect(screen.getByText(/nothing in this period/i)).toBeInTheDocument();
    expect(screen.queryByText(/no trend yet/i)).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: /show all time/i })).toBeInTheDocument();
  });

  // The regression guard. This is the exact shape that was misread: a window in
  // the past over a register populated later, so every `cumulative_total` in the
  // series is 0 while the register is not empty. Deriving the decision from the
  // series again would fail here and nowhere else.
  it('does not conclude "no risks" from a zero cumulative series', () => {
    renderTrend({
      trend: trendOf([point('2026-01-01', 0, 0), point('2026-01-02', 0, 0)]),
      registerTotal: 8,
    });

    expect(screen.getByText(/nothing in this period/i)).toBeInTheDocument();
    expect(screen.queryByText(/no trend yet/i)).not.toBeInTheDocument();
  });

  // The mirror of the guard above: a non-zero cumulative must not be allowed to
  // stand in for the register either. Nothing opened, nothing in the register —
  // the series' stock is irrelevant to which empty state is correct.
  it('does not conclude "period is empty" from a non-zero cumulative alone', () => {
    renderTrend({
      trend: trendOf([point('2026-09-01', 0, 5)]),
      registerTotal: 0,
    });

    expect(screen.getByText(/no trend yet/i)).toBeInTheDocument();
    expect(screen.queryByText(/nothing in this period/i)).not.toBeInTheDocument();
  });

  // A missing payload is not the same as an empty one, and must not be reported
  // as a period problem the user could act on.
  it('treats an absent trend with an empty register as the empty register', () => {
    renderTrend({ trend: undefined, registerTotal: 0 });

    expect(screen.getByText(/no trend yet/i)).toBeInTheDocument();
    expect(screen.queryByText(/nothing in this period/i)).not.toBeInTheDocument();
  });
});
