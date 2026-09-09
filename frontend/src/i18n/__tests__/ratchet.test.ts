// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// The hardcoded-string ratchet.
//
// The framework in `src/i18n` is only worth having if the tree stops accumulating
// the patterns it replaces. Extracting all of them at once was not this change's
// job (#315 shipped the framework; the sweep is tracked separately), so instead
// the counts are frozen here at their measured value and may only ever go DOWN.
//
// If this test fails on your branch you have either:
//   * added a new inline `lang === 'fr' ? … : …` — use `t('some.key')` instead;
//   * added a new raw `toLocale*` — use `fmt.date()` / `fmt.number()` from
//     `useFormat()` or `useI18n()`;
//   * removed some, in which case LOWER the number here. That is the point.

import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

const SRC = join(process.cwd(), 'src');

/**
 * Measured 2026-09-08 on branch 315. Every number here is a debt, not a target.
 * THESE NUMBERS MAY ONLY EVER GO DOWN.
 */
const BASELINE = {
  /** Inline bilingual ternaries that should be catalogue keys. */
  frTernaries: 260,
  /** Raw `toLocale*` calls that should go through `src/i18n/format.ts`. */
  rawToLocale: 62,
};

function walk(dir: string, out: string[] = []): string[] {
  for (const entry of readdirSync(dir)) {
    const path = join(dir, entry);
    if (statSync(path).isDirectory()) {
      if (entry === '__tests__' || entry === 'node_modules') continue;
      walk(path, out);
    } else if (/\.tsx?$/.test(entry) && !/\.(test|spec)\.tsx?$/.test(entry)) {
      out.push(path);
    }
  }
  return out;
}

/** Every source file except the i18n module itself, which is allowed to know. */
const files = walk(SRC).filter((f) => !f.includes(`${join('src', 'i18n')}`));

function countMatches(pattern: RegExp): number {
  let total = 0;
  for (const file of files) {
    const matches = readFileSync(file, 'utf8').match(pattern);
    total += matches ? matches.length : 0;
  }
  return total;
}

describe('hardcoded-string ratchet', () => {
  it('adds no new inline bilingual ternary', () => {
    const count = countMatches(/lang === 'fr'/g);
    expect(
      count,
      `Inline \`lang === 'fr' ? …\` count is ${count}, baseline ${BASELINE.frTernaries}. ` +
        'Use a catalogue key via `t()`. If you removed some, lower the baseline.',
    ).toBeLessThanOrEqual(BASELINE.frTernaries);
  });

  it('adds no new raw toLocale* call', () => {
    const count = countMatches(/toLocale(?:Date|Time)?String\(/g);
    expect(
      count,
      `Raw \`toLocale*\` count is ${count}, baseline ${BASELINE.rawToLocale}. ` +
        'Use `useFormat()` / `useI18n().fmt`. If you removed some, lower the baseline.',
    ).toBeLessThanOrEqual(BASELINE.rawToLocale);
  });

  it('lets no BCP-47 tag be hardcoded outside the registry', () => {
    // This one is already at zero and must stay there: the registry owns tags.
    const count = countMatches(/'(?:fr-FR|en-US|en-GB|fr-CA|ar-MA)'/g);
    expect(
      count,
      'A BCP-47 tag is hardcoded outside src/i18n/locales.ts. Use `localeTag(lang)`.',
    ).toBe(0);
  });

  it('lets no new module redeclare the set of UI languages', () => {
    // `Lang` is derived from LOCALES; a local `'fr' | 'en'` union re-freezes the
    // product at two languages, and #315 removed 38 of them.
    //
    // Three survive on purpose, and they are not UI locales: `Locale`
    // (features/ai), `ReportLocale` (types/report) and `BoardLocale`
    // (types/board) each describe what the SERVER can render. Widening those to
    // LocaleCode would claim OpenRisk generates Arabic reports, which it does
    // not. They change when the backend gains a language — not when the UI does.
    const count = countMatches(/'fr'\s*\|\s*'en'/g);
    expect(
      count,
      "Declare UI languages in src/i18n/locales.ts, not as a `'fr' | 'en'` union.",
    ).toBeLessThanOrEqual(3);
  });
});
