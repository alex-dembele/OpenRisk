// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, it } from 'vitest';
import {
  DEFAULT_LOCALE,
  ENABLED_LOCALES,
  LOCALES,
  LOCALE_CODES,
  isEnabledLocale,
  isLocaleCode,
  localeDirection,
  localeTag,
  negotiateLocale,
  pickLocalized,
  resolveLocale,
} from '../locales';

describe('locale registry', () => {
  it('declares a complete definition for every registered locale', () => {
    for (const code of LOCALE_CODES) {
      const def = LOCALES[code];
      expect(def.code).toBe(code);
      // A tag that Intl refuses is a locale that formats nothing.
      expect(() => new Intl.NumberFormat(def.tag)).not.toThrow();
      expect(['ltr', 'rtl']).toContain(def.dir);
      expect(def.nativeName.length).toBeGreaterThan(0);
      expect(['XAF', 'EUR', 'USD']).toContain(def.defaultCurrency);
    }
  });

  it('offers only locales that are enabled', () => {
    expect(ENABLED_LOCALES).toEqual(['fr', 'en']);
    expect(isEnabledLocale('ar')).toBe(false);
    expect(isLocaleCode('ar')).toBe(true);
  });

  it('registers an RTL locale so direction is exercised, not assumed', () => {
    expect(localeDirection('ar')).toBe('rtl');
    expect(localeDirection('fr')).toBe('ltr');
    expect(localeDirection('en')).toBe('ltr');
  });

  it('resolves regional tags onto the offered language', () => {
    expect(resolveLocale('fr-CM')).toBe('fr');
    expect(resolveLocale('fr_CA')).toBe('fr');
    expect(resolveLocale('EN-gb')).toBe('en');
    // Registered but not offered: must not become the active language.
    expect(resolveLocale('ar')).toBe(DEFAULT_LOCALE);
    expect(resolveLocale('de')).toBe(DEFAULT_LOCALE);
    expect(resolveLocale(undefined)).toBe(DEFAULT_LOCALE);
    expect(resolveLocale('')).toBe(DEFAULT_LOCALE);
  });

  it('negotiates an Accept-Language list by q-weight', () => {
    expect(negotiateLocale('de-DE,en-GB;q=0.8,fr;q=0.5')).toBe('en');
    expect(negotiateLocale('en;q=0.2, fr-CM;q=0.9')).toBe('fr');
    expect(negotiateLocale(['de', 'ar'])).toBe(DEFAULT_LOCALE);
    expect(negotiateLocale('en;q=0')).toBe(DEFAULT_LOCALE);
    expect(negotiateLocale(undefined)).toBe(DEFAULT_LOCALE);
  });

  it('maps a locale to the BCP-47 tag Intl needs, not to its own code', () => {
    expect(localeTag('fr')).toBe('fr-FR');
    expect(localeTag('en')).toBe('en-US');
    expect(localeTag('ar')).toBe('ar-MA');
  });

  it('falls an inline {fr,en} literal back to the default language', () => {
    const label = { fr: 'Actif', en: 'Asset' };
    expect(pickLocalized('en', label)).toBe('Asset');
    // The whole point: a newly registered locale reads French, never undefined.
    expect(pickLocalized('ar', label)).toBe('Actif');
    expect(pickLocalized('fr', undefined)).toBeUndefined();
    expect(pickLocalized('fr', null)).toBeUndefined();
  });
});
