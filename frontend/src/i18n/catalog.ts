// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

/**
 * Where a language's words are wired in.
 *
 * Adding a language is two lines: an entry in `LOCALES` (src/i18n/locales.ts)
 * and an entry here pointing at its catalogue. `ar` deliberately has no entry —
 * it is registered for its direction and plural rules, and until someone writes
 * `ar.json` it falls back to French through `translate()`. That is the whole
 * point of the fallback chain: a partially translated language degrades to
 * readable text instead of to blank cells.
 *
 * `runtimeMessages` is the extension seam for plural forms and messages that the
 * legacy flat JSON files cannot express. Keys defined there win over the JSON,
 * which lets a screen migrate to real pluralization one key at a time without a
 * big-bang rewrite of `locales/fr.json`.
 */

import enJson from '../locales/en.json';
import frJson from '../locales/fr.json';
import { runtimeMessages } from './messages';
import { LOCALE_CODES, type LocaleCode } from './locales';
import { isPluralForms } from './plural';
import type { Catalog } from './translate';

/** A branch of the tree — as opposed to a leaf, which is a string or a plural set. */
function isBranch(value: unknown): value is Catalog {
  return typeof value === 'object' && value !== null && !Array.isArray(value) && !isPluralForms(value);
}

/** Deep-merge two catalogues, with `overlay` winning at every leaf. */
export function merge(base: Catalog, overlay: Catalog): Catalog {
  const out: Catalog = { ...base };
  for (const [key, value] of Object.entries(overlay)) {
    const existing = out[key];
    out[key] =
      isBranch(existing) && isBranch(value) ? merge(existing, value) : (value as Catalog[string]);
  }
  return out;
}

/** The JSON catalogues, keyed by locale. One line per shipped language. */
const jsonCatalogs: Partial<Record<LocaleCode, Catalog>> = {
  fr: frJson as Catalog,
  en: enJson as Catalog,
};

/**
 * Built once at module load: every registered locale that has words, with its
 * TypeScript overlay merged over its JSON. A registered locale with neither
 * (today: `ar`) is simply absent, and `translate()` falls back for it.
 */
export const catalogs: Partial<Record<LocaleCode, Catalog>> = Object.fromEntries(
  LOCALE_CODES.map((code) => [code, mergeFor(code)]).filter(([, catalog]) => catalog !== undefined),
) as Partial<Record<LocaleCode, Catalog>>;

function mergeFor(code: LocaleCode): Catalog | undefined {
  const json = jsonCatalogs[code];
  const overlay = runtimeMessages[code];
  if (!json) return overlay;
  return overlay ? merge(json, overlay) : json;
}
