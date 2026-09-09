// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

/**
 * The locale registry — the single place a language is declared.
 *
 * Adding a language is one entry here plus one message catalogue in
 * `src/i18n/catalog.ts`. Nothing else in the application names a language:
 * `Lang` is derived from this object, so widening it widens the app.
 *
 * `enabled` is the honesty switch. A locale may be *registered* (its direction,
 * currency and plural rules are live and tested) long before it is *offered* to
 * users. Only enabled locales appear in the language switcher, and only they can
 * be persisted, so a half-translated language can never leak into the product.
 */

/** ISO 4217 codes the product formats money in. XAF is the canonical currency. */
export const CURRENCIES = ['XAF', 'EUR', 'USD'] as const;
export type CurrencyCode = (typeof CURRENCIES)[number];

/** Writing direction. Drives `<html dir>` and the logical-property CSS. */
export type Direction = 'ltr' | 'rtl';

export interface LocaleDefinition {
  /** Internal key and the value persisted in localStorage. */
  readonly code: string;
  /** BCP-47 tag handed to every `Intl` constructor. Never assume it equals `code`. */
  readonly tag: string;
  readonly dir: Direction;
  /** Endonym, shown in the switcher — a language names itself in its own words. */
  readonly nativeName: string;
  readonly englishName: string;
  /** Currency used when a figure carries no explicit currency of its own. */
  readonly defaultCurrency: CurrencyCode;
  /** False = registered and testable, but not offered to users yet. */
  readonly enabled: boolean;
}

export const LOCALES = {
  fr: {
    code: 'fr',
    tag: 'fr-FR',
    dir: 'ltr',
    nativeName: 'Français',
    englishName: 'French',
    defaultCurrency: 'XAF',
    enabled: true,
  },
  en: {
    code: 'en',
    tag: 'en-US',
    dir: 'ltr',
    nativeName: 'English',
    englishName: 'English',
    defaultCurrency: 'USD',
    enabled: true,
  },
  /**
   * Registered, not shipped. Arabic exists here so RTL is exercised by the test
   * suite rather than asserted in a document: it proves the direction plumbing,
   * the RTL plural categories (zero/one/two/few/many/other) and the Arabic
   * numeral formatting all work before anyone writes a catalogue. It stays
   * `enabled: false` until that catalogue exists — see the Maghreb line in
   * ROADMAP.md.
   */
  ar: {
    code: 'ar',
    tag: 'ar-MA',
    dir: 'rtl',
    nativeName: 'العربية',
    englishName: 'Arabic',
    defaultCurrency: 'EUR',
    enabled: false,
  },
} as const satisfies Record<string, LocaleDefinition>;

/** Every registered language. Widens automatically when LOCALES grows. */
export type LocaleCode = keyof typeof LOCALES;

/** The fallback for a missing key, an unknown code, and a first visit. */
export const DEFAULT_LOCALE: LocaleCode = 'fr';

export const LOCALE_CODES = Object.keys(LOCALES) as LocaleCode[];

/** The locales the switcher may offer and the store may persist. */
export const ENABLED_LOCALES: LocaleCode[] = LOCALE_CODES.filter((c) => LOCALES[c].enabled);

export function isLocaleCode(value: unknown): value is LocaleCode {
  return typeof value === 'string' && Object.prototype.hasOwnProperty.call(LOCALES, value);
}

export function isEnabledLocale(value: unknown): value is LocaleCode {
  return isLocaleCode(value) && LOCALES[value].enabled;
}

export function localeDefinition(code: LocaleCode): LocaleDefinition {
  return LOCALES[code];
}

/** BCP-47 tag for `Intl`. Unknown codes resolve to the default rather than throw. */
export function localeTag(code: LocaleCode): string {
  return (LOCALES[code] ?? LOCALES[DEFAULT_LOCALE]).tag;
}

export function localeDirection(code: LocaleCode): Direction {
  return (LOCALES[code] ?? LOCALES[DEFAULT_LOCALE]).dir;
}

/**
 * Narrow anything — a localStorage string, an `Accept-Language` header, a URL
 * param — onto a locale the product actually offers. Matches the exact code
 * first, then the primary subtag ("fr-CM" and "fr_CA" both land on "fr").
 */
export function resolveLocale(value: unknown, fallback: LocaleCode = DEFAULT_LOCALE): LocaleCode {
  if (typeof value !== 'string' || value === '') return fallback;
  const normalized = value.trim().toLowerCase().replace('_', '-');
  if (isEnabledLocale(normalized)) return normalized;
  const primary = normalized.split('-')[0];
  if (isEnabledLocale(primary)) return primary;
  return fallback;
}

/**
 * Pick the best offered locale out of an `Accept-Language`-shaped preference
 * list, honouring q-weights. Used by the first-visit default and by anything
 * that receives a browser language list.
 */
export function negotiateLocale(
  accepted: readonly string[] | string | undefined,
  fallback: LocaleCode = DEFAULT_LOCALE,
): LocaleCode {
  if (!accepted) return fallback;
  const raw = Array.isArray(accepted) ? accepted : String(accepted).split(',');
  const ranked = raw
    .map((entry, index) => {
      const [tag, ...params] = entry.split(';').map((part: string) => part.trim());
      const q = params
        .map((param: string) => /^q=([0-9.]+)$/i.exec(param))
        .find(Boolean)?.[1];
      return { tag, q: q === undefined ? 1 : Number(q), index };
    })
    .filter((e) => e.tag !== '' && Number.isFinite(e.q) && e.q > 0)
    .sort((a, b) => b.q - a.q || a.index - b.index);

  for (const entry of ranked) {
    const match = resolveLocale(entry.tag, null as unknown as LocaleCode);
    if (match) return match;
  }
  return fallback;
}

/**
 * Read a value out of an inline `{ fr: …, en: … }` literal.
 *
 * Transitional by design: these literals are exactly the pattern this framework
 * exists to replace, and 287 of them are still in the tree. Until each one is
 * extracted into a catalogue, this resolves it through the same fallback chain
 * `translate()` uses — so registering a new language turns those labels into the
 * default language's words, never into `undefined` rendered as a blank cell.
 */
export function pickLocalized<T>(
  locale: LocaleCode,
  options: Partial<Record<LocaleCode, T>> | null | undefined,
): T | undefined {
  if (!options) return undefined;
  return options[locale] ?? options[DEFAULT_LOCALE];
}
