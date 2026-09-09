// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, it } from 'vitest';
import { pluralCategory, resolveMessage, selectPlural, type PluralForms } from '../plural';

const results: PluralForms = {
  exact: { 0: 'Aucun résultat' },
  one: '{count} résultat',
  other: '{count} résultats',
};

describe('pluralization', () => {
  it('uses the language’s own CLDR categories, not an English rule', () => {
    // French counts 0 as singular; English does not. This is the exact pair the
    // old `n > 1 ? 's' : ''` got wrong.
    expect(pluralCategory('fr', 0)).toBe('one');
    expect(pluralCategory('en', 0)).toBe('other');
    expect(pluralCategory('fr', 1)).toBe('one');
    expect(pluralCategory('en', 1)).toBe('one');
    expect(pluralCategory('fr', 2)).toBe('other');
  });

  it('exposes the six categories an RTL language actually needs', () => {
    expect(pluralCategory('ar', 0)).toBe('zero');
    expect(pluralCategory('ar', 1)).toBe('one');
    expect(pluralCategory('ar', 2)).toBe('two');
    expect(pluralCategory('ar', 3)).toBe('few');
    expect(pluralCategory('ar', 11)).toBe('many');
    expect(pluralCategory('ar', 100)).toBe('other');
  });

  it('supports ordinal rules as well as cardinal', () => {
    expect(pluralCategory('en', 1, 'ordinal')).toBe('one');
    expect(pluralCategory('en', 2, 'ordinal')).toBe('two');
    expect(pluralCategory('en', 4, 'ordinal')).toBe('other');
  });

  it('prefers an exact override over the category', () => {
    expect(selectPlural('fr', results, 0)).toBe('Aucun résultat');
    expect(selectPlural('fr', results, 1)).toBe('{count} résultat');
    expect(selectPlural('fr', results, 2)).toBe('{count} résultats');
  });

  it('falls back to `other` for a category the message does not define', () => {
    // Arabic selects `many` at 11; the message has no `many`, so `other` holds.
    expect(selectPlural('ar', results, 11)).toBe('{count} résultats');
  });

  it('never blows up on a non-finite count', () => {
    expect(pluralCategory('fr', Number.NaN)).toBe('other');
    expect(selectPlural('fr', results, Number.POSITIVE_INFINITY)).toBe('{count} résultats');
  });

  it('passes plain strings through and neutralises a countless plural', () => {
    expect(resolveMessage('fr', 'Bonjour')).toBe('Bonjour');
    expect(resolveMessage('fr', results)).toBe('{count} résultats');
  });
});
