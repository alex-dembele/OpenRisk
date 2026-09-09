// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

/**
 * Key lookup, fallback and interpolation.
 *
 * Three rules the rest of the app depends on:
 *   1. A missing key never renders blank. It falls back to the default locale
 *      and then to the key text, so a gap is visible in review, not invisible
 *      in production.
 *   2. Values are interpolated *through the formatters*, so `{count, number}`
 *      is grouped in the reader's conventions rather than pasted raw.
 *   3. A count selects the plural form before interpolation runs, so a message
 *      can say `{count, number} risques` and still be singular at one.
 */

import {
  boundFormatters,
  type CurrencyOptions,
  type DateInput,
} from './format';
import { DEFAULT_LOCALE, type LocaleCode } from './locales';
import { isPluralForms, resolveMessage, type Message } from './plural';

/** A catalogue is a tree of messages. Leaves are strings or plural forms. */
export interface Catalog {
  [key: string]: Message | Catalog;
}

/** What a component may interpolate. `count` additionally drives pluralization. */
export type TranslateParams = Record<
  string,
  string | number | boolean | Date | null | undefined
>;

function isCatalog(value: unknown): value is Catalog {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

/**
 * Read `a.b.c` out of a catalogue. Returns undefined — not the key — so the
 * caller can tell "absent here" from "the translation is literally this text"
 * and move on to the next locale in the chain.
 */
export function lookup(catalog: Catalog | undefined, key: string): Message | undefined {
  if (!catalog) return undefined;
  let node: Message | Catalog | undefined = catalog;
  for (const part of key.split('.')) {
    if (!isCatalog(node) || isPluralForms(node)) return undefined;
    node = node[part];
    if (node === undefined) return undefined;
  }
  if (typeof node === 'string' || isPluralForms(node)) return node;
  return undefined;
}

/**
 * `{name}`, `{count, number}`, `{amount, currency, EUR}`.
 * The optional third field is the format's argument — a currency code for
 * `currency`, a fraction-digit count for `percent` and `compact`.
 */
const PLACEHOLDER = /\{(\w+)(?:\s*,\s*(\w+))?(?:\s*,\s*([^}]+))?\}/g;

function applyFormat(
  locale: LocaleCode,
  value: string | number | boolean | Date | null | undefined,
  format: string | undefined,
  arg: string | undefined,
): string {
  if (value === null || value === undefined) return '';
  const f = boundFormatters(locale);
  switch (format) {
    case undefined:
      return String(value);
    case 'number':
      return f.number(Number(value));
    case 'compact':
      return f.compact(Number(value), arg === undefined ? undefined : Number(arg));
    case 'percent':
      return f.percent(Number(value), arg === undefined ? undefined : Number(arg));
    case 'currency':
      return f.currency(Number(value), { currency: arg } as CurrencyOptions);
    case 'date':
      return f.date(value as DateInput);
    case 'datetime':
      return f.dateTime(value as DateInput);
    case 'time':
      return f.time(value as DateInput);
    case 'relative':
      return f.relative(value as DateInput);
    default:
      // An unknown format is an authoring mistake, not a reason to blank the UI.
      return String(value);
  }
}

/** Substitute `{…}` placeholders. An unknown name is left visible, not erased. */
export function interpolate(
  template: string,
  params: TranslateParams | undefined,
  locale: LocaleCode = DEFAULT_LOCALE,
): string {
  if (!params) return template;
  return template.replace(PLACEHOLDER, (match, name: string, format?: string, arg?: string) => {
    if (!Object.prototype.hasOwnProperty.call(params, name)) return match;
    return applyFormat(locale, params[name], format, arg?.trim());
  });
}

export interface TranslateOptions {
  /** Text to show when the key is absent from every locale in the chain. */
  defaultValue?: string;
  params?: TranslateParams;
}

/**
 * Resolve `key` for `locale`, falling back to the default locale and finally to
 * `defaultValue` or the key text itself.
 *
 * `catalogs` is passed in rather than imported so the pure core stays testable
 * with a two-key fixture and the app wires the real catalogues in `catalog.ts`.
 */
export function translate(
  catalogs: Partial<Record<LocaleCode, Catalog>>,
  locale: LocaleCode,
  key: string,
  options: TranslateOptions = {},
): string {
  const { defaultValue, params } = options;
  const message = lookup(catalogs[locale], key) ?? lookup(catalogs[DEFAULT_LOCALE], key);
  if (message === undefined) return defaultValue ?? key;

  const count = typeof params?.count === 'number' ? params.count : undefined;
  const resolved = resolveMessage(locale, message, count);
  // An empty string in a catalogue is a hole, not a translation.
  if (resolved === '') return defaultValue ?? key;
  return interpolate(resolved, params, locale);
}
