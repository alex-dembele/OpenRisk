// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// The Command Center's selected period — ONE definition, mirroring
// internal/domain/timeframe on the server.
//
// It lives in the URL and nowhere else. That is the whole design decision, and
// it is worth the paragraph:
//
// A filtered dashboard is a *place*. "The last 30 days, for this tenant" has to
// survive a reload, a back button, and being pasted into a chat window, or the
// period control is decoration. Keeping it in component state gives every widget
// its own copy to drift out of sync — which is exactly what the old trend card
// did: a 7/30/90 toggle in useState, driving one client-side computation, invisible
// to the URL and to every other widget on the page.
//
// The URL is therefore the single source of truth, and it is also the API
// request: `?period=30d` in the address bar becomes `?period=30d` on the wire,
// unchanged. There is no second representation to disagree with the first.

import { useCallback, useMemo } from 'react';
import { useSearchParams } from 'react-router';
import type { LocaleCode } from '../../i18n/locales';
import { pickLocalized } from '../../i18n/locales';

/** The presets the server accepts. Anything else is a 400, by design. */
export const PERIOD_PRESETS = ['all', '7d', '30d', '90d'] as const;
export type PeriodPreset = (typeof PERIOD_PRESETS)[number];

/**
 * The default is `all`, matching timeframe.DefaultPreset on the server.
 *
 * Not `30d`: the headline counters are stock quantities (how many critical risks
 * exist), not flow quantities. Defaulting them to a window would put the
 * dashboard permanently at odds with the register on first paint, with nothing
 * on screen to say a filter had been applied on the user's behalf.
 */
export const DEFAULT_PERIOD: PeriodPreset = 'all';

/** A resolved selection: either a preset or an explicit half-open range. */
export type PeriodSelection =
  { kind: 'preset'; preset: PeriodPreset } | { kind: 'custom'; from: string; to: string };

/** What goes on the wire. Sent verbatim; the server owns the resolution. */
export type PeriodParams = { period: PeriodPreset } | { from: string; to: string };

const ISO_DATE = /^\d{4}-\d{2}-\d{2}$/;

function isPreset(v: string | null): v is PeriodPreset {
  return !!v && (PERIOD_PRESETS as readonly string[]).includes(v);
}

/**
 * Read the selection out of a query string.
 *
 * Unreadable input falls back to the default HERE, on the client, before any
 * request is made — and the fallback is then written back into the URL by
 * `useDashboardPeriod`, so the address bar and the data agree. That is the
 * opposite of the server's behaviour, which rejects a malformed period with a
 * 400 rather than answering a different question, and the two are consistent:
 * the client never *sends* something it could not parse.
 */
export function readPeriod(params: URLSearchParams): PeriodSelection {
  const from = params.get('from');
  const to = params.get('to');
  if (from && to && ISO_DATE.test(from) && ISO_DATE.test(to) && from < to) {
    return { kind: 'custom', from, to };
  }
  const preset = params.get('period');
  return { kind: 'preset', preset: isPreset(preset) ? preset : DEFAULT_PERIOD };
}

/** Serialise a selection for the wire. */
export function periodParams(sel: PeriodSelection): PeriodParams {
  return sel.kind === 'custom' ? { from: sel.from, to: sel.to } : { period: sel.preset };
}

/**
 * A stable cache-key fragment for this selection.
 *
 * Every dashboard query keys on this. Two windows over the same register are two
 * different answers, and serving one for the other is the cache equivalent of
 * ignoring the filter — the same reason the server's cache key carries the
 * period.
 */
export function periodKey(sel: PeriodSelection): string {
  return sel.kind === 'custom' ? `custom:${sel.from}:${sel.to}` : sel.preset;
}

/**
 * Carry the selection onto a deep link.
 *
 * Only applied when the destination understands it — see `deepLink` in
 * ./deepLinks.ts. A period appended to a screen that ignores it is a URL that
 * lies about what is being shown.
 */
export function periodToSearchParams(
  sel: PeriodSelection,
  into = new URLSearchParams(),
): URLSearchParams {
  if (sel.kind === 'custom') {
    into.set('from', sel.from);
    into.set('to', sel.to);
  } else if (sel.preset !== DEFAULT_PERIOD) {
    // The default is omitted so a clean dashboard has a clean URL, exactly as
    // the DataTable omits its default sort.
    into.set('period', sel.preset);
  }
  return into;
}

/** Human label, FR/EN. Kept beside the values so a new preset cannot ship unlabelled. */
export function periodLabel(sel: PeriodSelection, lang: LocaleCode): string {
  if (sel.kind === 'custom') return `${sel.from} → ${sel.to}`;
  const labels: Record<PeriodPreset, { fr: string; en: string }> = {
    all: { fr: 'Tout', en: 'All time' },
    '7d': { fr: '7 jours', en: '7 days' },
    '30d': { fr: '30 jours', en: '30 days' },
    '90d': { fr: '90 jours', en: '90 days' },
  };
  return pickLocalized(lang, labels[sel.preset]) ?? sel.preset;
}

/**
 * The hook every persona uses. Reads the selection from the URL and writes
 * changes back to it.
 *
 * `replace: true` on the write: changing a filter is not a navigation. Pushing a
 * history entry per click would make the back button walk through every period a
 * user tried instead of returning them to where they came from.
 */
export function useDashboardPeriod() {
  const [params, setParams] = useSearchParams();
  const selection = useMemo(() => readPeriod(params), [params]);

  const setSelection = useCallback(
    (next: PeriodSelection) => {
      setParams(
        (prev) => {
          const out = new URLSearchParams(prev);
          // Drop everything this control owns, then re-add only what is not the
          // default — so resetting to "all" leaves a clean URL rather than
          // `?period=all`.
          out.delete('period');
          out.delete('from');
          out.delete('to');
          return periodToSearchParams(next, out);
        },
        { replace: true },
      );
    },
    [setParams],
  );

  return { selection, setSelection, params: periodParams(selection), key: periodKey(selection) };
}
