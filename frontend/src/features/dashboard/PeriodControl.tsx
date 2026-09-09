// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// The dashboard's period selector, and the sentence that keeps it honest.
//
// A global period control on a page of mixed widgets has one failure mode, and
// it is severe: it looks like it filters everything. It does not, and it cannot.
// The headline counters are STOCK quantities — how many critical risks exist —
// and narrowing them by created_at would answer "how many did we open last
// month" under a label that says otherwise. Only the flow widgets (the trend, the
// "opened"/"added" counters) are period-scoped.
//
// So the control states its own scope, in words, right under itself: it names
// what moves when you change it. That line is not decoration — it is the
// difference between a filter and a filter that misleads, and the brief this
// wave answers calls the alternative out by name.
//
// The selection lives in the URL (see ./period.ts), so the control has no state
// of its own to drift from what is displayed.

import { useId, useState } from 'react';
import { CalendarRange, Check } from 'lucide-react';

import { PERIOD_PRESETS, periodLabel, type PeriodSelection, type PeriodPreset } from './period';
import type { LocaleCode } from '../../i18n/locales';

export function PeriodControl({
  selection,
  onChange,
  lang,
  /**
   * What this control actually narrows, in the caller's own words. Required:
   * a period control that will not say what it filters should not ship.
   */
  scopeNote,
}: {
  selection: PeriodSelection;
  onChange: (next: PeriodSelection) => void;
  lang: LocaleCode;
  scopeNote: string;
}) {
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const [customOpen, setCustomOpen] = useState(selection.kind === 'custom');
  const [from, setFrom] = useState(selection.kind === 'custom' ? selection.from : '');
  const [to, setTo] = useState(selection.kind === 'custom' ? selection.to : '');
  const groupId = useId();

  const rangeValid = !!from && !!to && from < to;

  return (
    <div className="flex flex-col items-end gap-1.5">
      <div
        className="flex gap-0.5 p-0.5 rounded-[9px]"
        style={{ background: 'var(--bg-hover)' }}
        role="group"
        aria-labelledby={groupId}
      >
        <span id={groupId} className="sr-only">
          {tr('Période du tableau de bord', 'Dashboard period')}
        </span>
        {PERIOD_PRESETS.map((preset: PeriodPreset) => {
          const active = selection.kind === 'preset' && selection.preset === preset;
          return (
            <button
              key={preset}
              type="button"
              // aria-pressed rather than a bare button: a screen reader must be
              // able to say WHICH period is in force, not only that four
              // buttons exist.
              aria-pressed={active}
              onClick={() => {
                setCustomOpen(false);
                onChange({ kind: 'preset', preset });
              }}
              className="h-[26px] px-2.5 rounded-[7px] text-[11.5px] font-semibold transition-colors"
              style={{
                background: active ? 'var(--accent-hover)' : 'transparent',
                color: active ? 'var(--fg-inverse)' : 'var(--fg-secondary)',
              }}
            >
              {periodLabel({ kind: 'preset', preset }, lang)}
            </button>
          );
        })}
        <button
          type="button"
          aria-pressed={selection.kind === 'custom'}
          aria-expanded={customOpen}
          onClick={() => setCustomOpen((v) => !v)}
          className="h-[26px] px-2 rounded-[7px] text-[11.5px] font-semibold transition-colors flex items-center gap-1"
          style={{
            background: selection.kind === 'custom' ? 'var(--accent-hover)' : 'transparent',
            color: selection.kind === 'custom' ? 'var(--fg-inverse)' : 'var(--fg-secondary)',
          }}
        >
          <CalendarRange size={13} />
          {tr('Dates', 'Dates')}
        </button>
      </div>

      {customOpen && (
        <div
          className="flex items-center gap-2 p-2 rounded-[10px]"
          style={{ background: 'var(--bg-elevated)', border: '1px solid var(--border-strong)' }}
        >
          <label className="sr-only" htmlFor={`${groupId}-from`}>
            {tr('Date de début (incluse)', 'Start date (inclusive)')}
          </label>
          <input
            id={`${groupId}-from`}
            type="date"
            value={from}
            onChange={(e) => setFrom(e.target.value)}
            className="h-[28px] px-2 rounded-[7px] text-[12px] text-ink bg-transparent"
            style={{ border: '1px solid var(--border)' }}
          />
          <span className="text-[12px] text-ink-muted">→</span>
          <label className="sr-only" htmlFor={`${groupId}-to`}>
            {tr('Date de fin (exclue)', 'End date (exclusive)')}
          </label>
          <input
            id={`${groupId}-to`}
            type="date"
            value={to}
            onChange={(e) => setTo(e.target.value)}
            className="h-[28px] px-2 rounded-[7px] text-[12px] text-ink bg-transparent"
            style={{ border: '1px solid var(--border)' }}
          />
          <button
            type="button"
            disabled={!rangeValid}
            onClick={() => rangeValid && onChange({ kind: 'custom', from, to })}
            className="h-[28px] px-2.5 rounded-[7px] text-[12px] font-semibold flex items-center gap-1 disabled:opacity-40"
            style={{ background: 'var(--accent)', color: 'var(--fg-inverse)' }}
          >
            <Check size={13} />
            {tr('Appliquer', 'Apply')}
          </button>
        </div>
      )}

      {/* The scope sentence. The control filters some widgets and not others, so
          it says which — rather than letting the user assume "all of them". */}
      <p className="text-[11px] text-ink-muted max-w-[340px] text-right leading-snug">
        {scopeNote}
      </p>
    </div>
  );
}
