// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, it } from 'vitest';
import { catalogs } from '../catalog';
import { lookup, translate, type Catalog } from '../translate';

const fixture: Partial<Record<'fr' | 'en' | 'ar', Catalog>> = {
  fr: {
    risks: { title: 'Registre des risques', greet: 'Bonjour {name}', blank: '' },
    common: { results: { exact: { 0: 'Aucun résultat' }, one: '{count, number} résultat', other: '{count, number} résultats' } },
  },
  en: {
    risks: { title: 'Risk register' },
    common: { results: { exact: { 0: 'No results' }, one: '{count, number} result', other: '{count, number} results' } },
  },
};

const norm = (s: string) => s.replace(/[  ‎‏]/g, ' ');

describe('translate', () => {
  it('reads a dotted key', () => {
    expect(translate(fixture, 'en', 'risks.title')).toBe('Risk register');
    expect(lookup(fixture.fr, 'risks.title')).toBe('Registre des risques');
    expect(lookup(fixture.fr, 'risks.missing')).toBeUndefined();
    expect(lookup(undefined, 'risks.title')).toBeUndefined();
  });

  it('falls back to the default locale before giving up', () => {
    // `risks.greet` exists only in French.
    expect(translate(fixture, 'en', 'risks.greet', { params: { name: 'Awa' } })).toBe('Bonjour Awa');
  });

  it('shows the key rather than a blank when nothing matches', () => {
    expect(translate(fixture, 'en', 'nope.at.all')).toBe('nope.at.all');
    expect(translate(fixture, 'en', 'nope.at.all', { defaultValue: 'Fallback' })).toBe('Fallback');
    // An empty catalogue entry is a hole, not a translation.
    expect(translate(fixture, 'fr', 'risks.blank')).toBe('risks.blank');
  });

  it('pluralizes and formats the count together', () => {
    expect(translate(fixture, 'en', 'common.results', { params: { count: 0 } })).toBe('No results');
    expect(translate(fixture, 'en', 'common.results', { params: { count: 1 } })).toBe('1 result');
    expect(translate(fixture, 'en', 'common.results', { params: { count: 4200 } })).toBe(
      '4,200 results',
    );
    expect(norm(translate(fixture, 'fr', 'common.results', { params: { count: 4200 } }))).toBe(
      '4 200 résultats',
    );
    // French is singular at 1 AND at 0 when no exact form overrides it.
    expect(translate(fixture, 'fr', 'common.results', { params: { count: 0 } })).toBe(
      'Aucun résultat',
    );
  });

  it('runs interpolated values through the formatters', () => {
    const c: Partial<Record<'fr' | 'en', Catalog>> = {
      fr: {
        money: 'Perte : {amount, currency, XAF}',
        when: 'Le {day, date}',
        share: '{ratio, percent}',
        big: '{n, compact}',
      },
    };
    expect(norm(translate(c, 'fr', 'money', { params: { amount: 1234567 } }))).toBe(
      'Perte : 1 234 567 FCFA',
    );
    expect(norm(translate(c, 'fr', 'share', { params: { ratio: 0.42 } }))).toBe('42 %');
    expect(translate(c, 'fr', 'when', { params: { day: '2026-03-12T00:00:00Z' } })).toContain('2026');
    expect(norm(translate(c, 'fr', 'big', { params: { n: 1_200_000 } }))).toBe('1,2 M');
  });

  it('leaves an unknown placeholder visible instead of erasing it', () => {
    const c: Partial<Record<'fr', Catalog>> = { fr: { hi: 'Bonjour {name} {surname}' } };
    expect(translate(c, 'fr', 'hi', { params: { name: 'Awa' } })).toBe('Bonjour Awa {surname}');
  });
});

describe('the real catalogues', () => {
  it('are wired for every language that has words', () => {
    expect(Object.keys(catalogs).sort()).toEqual(['en', 'fr']);
  });

  it('merge the TypeScript plural overlay over the JSON', () => {
    expect(translate(catalogs, 'en', 'common.results', { params: { count: 0 } })).toBe('No results');
    expect(translate(catalogs, 'en', 'common.results', { params: { count: 2 } })).toBe('2 results');
    expect(translate(catalogs, 'fr', 'common.days', { params: { count: 1 } })).toBe('1 jour');
    expect(translate(catalogs, 'fr', 'common.days', { params: { count: 3 } })).toBe('3 jours');
  });

  it('serve a registered-but-untranslated locale from the default language', () => {
    // `ar` has no catalogue. It must read French, not the raw key.
    expect(translate(catalogs, 'ar', 'common.days', { params: { count: 3 } })).toBe('3 jours');
  });

  it('keep the JSON keys the app already uses reachable', () => {
    expect(translate(catalogs, 'fr', 'risks.title')).not.toBe('risks.title');
    expect(translate(catalogs, 'en', 'risks.title')).not.toBe('risks.title');
  });
});
