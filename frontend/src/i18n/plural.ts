// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

/**
 * Pluralization through `Intl.PluralRules` and the real CLDR categories.
 *
 * The pattern this replaces is `${n} result${n > 1 ? 's' : ''}`, which is wrong
 * in two directions at once: it prints "0 result" in English (English uses the
 * plural at zero) and it cannot express Arabic, which has six categories, or
 * Russian, which changes form again at 5. A category is a language fact, so the
 * language table decides it — never the component.
 */

import { localeTag, type LocaleCode } from './locales';

/** The six CLDR plural categories. No language uses all six; Arabic uses all six. */
export const PLURAL_CATEGORIES = ['zero', 'one', 'two', 'few', 'many', 'other'] as const;
export type PluralCategory = (typeof PLURAL_CATEGORIES)[number];

/**
 * A message that changes shape with a count. `other` is required because it is
 * the only category every language defines — it is the guaranteed fallback.
 * `exact` short-circuits the rules entirely for copy that reads better at a
 * literal count ("no results" rather than "0 results").
 */
export type PluralForms = Partial<Record<PluralCategory, string>> & {
  other: string;
  exact?: Record<number, string>;
};

/** A catalogue value: a plain string, or a set of plural forms. */
export type Message = string | PluralForms;

export function isPluralForms(value: unknown): value is PluralForms {
  return (
    typeof value === 'object' &&
    value !== null &&
    !Array.isArray(value) &&
    typeof (value as PluralForms).other === 'string'
  );
}

// `Intl.PluralRules` construction is not free and these are hit inside render.
const cardinalCache = new Map<string, Intl.PluralRules>();
const ordinalCache = new Map<string, Intl.PluralRules>();

function rules(tag: string, type: Intl.PluralRuleType): Intl.PluralRules {
  const cache = type === 'ordinal' ? ordinalCache : cardinalCache;
  let found = cache.get(tag);
  if (!found) {
    found = new Intl.PluralRules(tag, { type });
    cache.set(tag, found);
  }
  return found;
}

/**
 * The CLDR category for `count` in `locale`. Non-finite counts fall to `other`,
 * which is the safe form: it is the one category guaranteed to be present.
 */
export function pluralCategory(
  locale: LocaleCode,
  count: number,
  type: Intl.PluralRuleType = 'cardinal',
): PluralCategory {
  if (!Number.isFinite(count)) return 'other';
  return rules(localeTag(locale), type).select(count) as PluralCategory;
}

/**
 * Choose the form for `count`, preferring an exact override, then the CLDR
 * category, then `other`. Never returns undefined — `other` is mandatory in the
 * type, so a catalogue physically cannot leave a plural without a fallback.
 */
export function selectPlural(locale: LocaleCode, forms: PluralForms, count: number): string {
  const exact = forms.exact?.[count];
  if (exact !== undefined) return exact;
  return forms[pluralCategory(locale, count)] ?? forms.other;
}

/**
 * Resolve any catalogue value against a count. Plain strings pass through. A
 * plural message used without a count is an authoring mistake; it renders
 * `other`, the neutral form, rather than silently claiming a quantity of zero.
 */
export function resolveMessage(locale: LocaleCode, message: Message, count?: number): string {
  if (typeof message === 'string') return message;
  if (count === undefined) return message.other;
  return selectPlural(locale, message, count);
}
