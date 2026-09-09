// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, it } from 'vitest';
import {
  boundFormatters,
  formatCompact,
  formatCurrency,
  formatDate,
  formatList,
  formatNumber,
  formatPercent,
  formatRelativeTime,
} from '../format';

/** Intl inserts narrow/non-breaking spaces; compare on the digits and marks. */
const norm = (s: string) => s.replace(/[  ‎‏]/g, ' ');

describe('locale-aware formatting', () => {
  it('groups numbers in the reader’s conventions', () => {
    expect(norm(formatNumber('fr', 1234567))).toBe('1 234 567');
    expect(formatNumber('en', 1234567)).toBe('1,234,567');
  });

  it('formats XAF with its real symbol and no minor unit', () => {
    // The CFA franc has no cents. Intl knows; a hand-rolled formatter would not.
    expect(norm(formatCurrency('fr', 1234567, { currency: 'XAF' }))).toBe('1 234 567 FCFA');
    expect(norm(formatCurrency('en', 1234567, { currency: 'XAF' }))).toBe('FCFA 1,234,567');
  });

  it('formats EUR and USD in each language', () => {
    expect(norm(formatCurrency('fr', 1234.5, { currency: 'EUR' }))).toBe('1 234,50 €');
    expect(formatCurrency('en', 1234.5, { currency: 'USD' })).toBe('$1,234.50');
  });

  it('uses the locale’s default currency when none is given', () => {
    expect(norm(formatCurrency('fr', 1000))).toContain('FCFA');
    expect(formatCurrency('en', 1000)).toContain('$');
  });

  it('degrades an unknown ISO code instead of blanking the cell', () => {
    expect(norm(formatCurrency('fr', 1000, { currency: 'NOTACODE' }))).toBe('1 000 NOTACODE');
  });

  it('renders — rather than NaN for a missing figure', () => {
    expect(formatNumber('fr', Number.NaN)).toBe('—');
    expect(formatCurrency('fr', Number.NaN)).toBe('—');
    expect(formatPercent('fr', Number.POSITIVE_INFINITY)).toBe('—');
    expect(formatCompact('fr', Number.NaN)).toBe('—');
    expect(formatDate('fr', 'not-a-date')).toBe('—');
  });

  it('treats a percent input as a ratio', () => {
    expect(formatPercent('en', 0.42)).toBe('42%');
    expect(formatPercent('en', 0.4235, 1)).toBe('42.4%');
  });

  it('formats dates and relative times per language', () => {
    const iso = '2026-03-12T08:30:00Z';
    expect(formatDate('fr', iso, { day: '2-digit', month: '2-digit', year: 'numeric' })).toBe(
      '12/03/2026',
    );
    expect(formatDate('en', iso, { day: '2-digit', month: '2-digit', year: 'numeric' })).toBe(
      '03/12/2026',
    );
    const now = new Date('2026-03-15T08:30:00Z');
    expect(formatRelativeTime('fr', iso, now)).toBe('il y a 3 jours');
    expect(formatRelativeTime('en', iso, now)).toBe('3 days ago');
  });

  it('joins a list with the language’s own conjunction', () => {
    expect(formatList('fr', ['a', 'b', 'c'])).toBe('a, b et c');
    expect(formatList('en', ['a', 'b', 'c'])).toBe('a, b, and c');
    expect(formatList('fr', [])).toBe('');
  });

  it('binds every formatter to one locale', () => {
    const fmt = boundFormatters('en');
    expect(fmt.locale).toBe('en');
    expect(fmt.number(1000)).toBe('1,000');
    expect(fmt.percent(0.5)).toBe('50%');
    expect(fmt.currency(10, { currency: 'USD' })).toBe('$10.00');
  });
});
