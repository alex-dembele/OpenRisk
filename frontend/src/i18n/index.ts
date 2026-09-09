// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

/**
 * The i18n public surface. Import from `@/i18n` (or a relative `../i18n`) —
 * never reach into the individual modules from a feature.
 *
 * Typical use inside a component:
 *
 *   const { t, fmt } = useI18n();
 *   t('common.results', { count: rows.length })   // "0 result" is never printed
 *   fmt.currency(ale, { currency: 'XAF' })        // "1 234 567 FCFA"
 *   fmt.relative(risk.updated_at)                 // "il y a 3 jours"
 */

export {
  CURRENCIES,
  DEFAULT_LOCALE,
  ENABLED_LOCALES,
  LOCALES,
  LOCALE_CODES,
  isEnabledLocale,
  isLocaleCode,
  localeDefinition,
  localeDirection,
  localeTag,
  negotiateLocale,
  resolveLocale,
  type CurrencyCode,
  type Direction,
  type LocaleCode,
  type LocaleDefinition,
} from './locales';

export {
  PLURAL_CATEGORIES,
  isPluralForms,
  pluralCategory,
  resolveMessage,
  selectPlural,
  type Message,
  type PluralCategory,
  type PluralForms,
} from './plural';

export {
  boundFormatters,
  formatCompact,
  formatCurrency,
  formatDate,
  formatDateTime,
  formatList,
  formatNumber,
  formatPercent,
  formatRelativeTime,
  formatTime,
  type BoundFormatters,
  type CurrencyOptions,
  type DateInput,
} from './format';

export {
  lookup,
  translate,
  type Catalog,
  type TranslateOptions,
  type TranslateParams,
} from './translate';

export { catalogs, merge } from './catalog';
export { runtimeMessages } from './messages';
