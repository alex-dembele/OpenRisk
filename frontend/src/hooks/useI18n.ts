// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

import { useCallback, useMemo } from 'react';
import {
  boundFormatters,
  catalogs,
  translate,
  type BoundFormatters,
  type LocaleCode,
  type TranslateParams,
} from '../i18n';
import { useUIStore } from '../store/uiStore';

/**
 * The component-facing i18n hook.
 *
 * The language lives in the central UI store, so the header language toggle
 * re-renders every consumer reactively. Everything below it — key lookup,
 * fallback, pluralization, formatting — is the pure core in `src/i18n`, which is
 * why it is testable without React.
 *
 * The second argument of `t` accepts both shapes on purpose. Roughly 200 call
 * sites already pass a default string, and rewriting them was not the job of
 * this change; passing an object opts into interpolation and plurals:
 *
 *   t('risks.title')
 *   t('risks.title', 'Risk register')                 // legacy default value
 *   t('common.results', { count: rows.length })        // plural + grouping
 */
export interface UseI18nReturn {
  /** Translate a dotted key. Falls back to the default locale, then the key. */
  t: (key: string, defaultOrParams?: string | TranslateParams) => string;
  /** The active language. */
  locale: LocaleCode;
  setLocale: (locale: LocaleCode) => void;
  /** Formatters already bound to the active locale. */
  fmt: BoundFormatters;
}

export function useI18n(): UseI18nReturn {
  const locale = useUIStore((s) => s.lang);
  const setLocale = useUIStore((s) => s.setLang);

  const t = useCallback(
    (key: string, defaultOrParams?: string | TranslateParams): string => {
      if (typeof defaultOrParams === 'string') {
        return translate(catalogs, locale, key, { defaultValue: defaultOrParams });
      }
      return translate(catalogs, locale, key, { params: defaultOrParams });
    },
    [locale],
  );

  const fmt = useMemo(() => boundFormatters(locale), [locale]);

  return { t, locale, setLocale, fmt };
}

/** Formatters alone, for components that format but do not translate. */
export function useFormat(): BoundFormatters {
  const locale = useUIStore((s) => s.lang);
  return useMemo(() => boundFormatters(locale), [locale]);
}

/**
 * Standalone interpolation, kept for the call sites that read a string first and
 * substitute afterwards (`interpolate(t('actionCenter.range'), { … })`). New
 * code should pass the params straight to `t` so plurals apply too.
 */
export { interpolate as interpolateRaw } from '../i18n/translate';

export function interpolate(str: string, values: Record<string, unknown>): string {
  return str.replace(/\{(\w+)\}/g, (match, key: string) =>
    Object.prototype.hasOwnProperty.call(values, key) ? String(values[key] ?? '') : match,
  );
}
