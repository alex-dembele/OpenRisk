// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

/**
 * Messages that JSON cannot express — plural sets, and copy that has to run a
 * value through a formatter.
 *
 * These live in TypeScript so the `PluralForms` type enforces the one thing a
 * translator cannot be trusted to remember: every plural must define `other`,
 * the single category present in every language on earth.
 *
 * They are merged over `locales/*.json` at startup and win on collision, so a
 * key moves here the day it needs a plural and no call site changes.
 *
 * FR/EN parity is a test, not a convention — see `__tests__/parity.test.ts`.
 */

import type { LocaleCode } from './locales';
import type { Catalog } from './translate';

const fr: Catalog = {
  common: {
    results: {
      exact: { 0: 'Aucun résultat' },
      one: '{count, number} résultat',
      other: '{count, number} résultats',
    },
    selected: {
      one: '{count, number} sélectionné',
      other: '{count, number} sélectionnés',
    },
    assets: {
      exact: { 0: 'Aucun actif' },
      one: '{count, number} actif',
      other: '{count, number} actifs',
    },
    days: {
      one: '{count, number} jour',
      other: '{count, number} jours',
    },
    controls: {
      exact: { 0: 'Aucun contrôle' },
      one: '{count, number} contrôle',
      other: '{count, number} contrôles',
    },
    frameworks: {
      one: '{count, number} référentiel',
      other: '{count, number} référentiels',
    },
    sources: {
      exact: { 0: 'Aucune source configurée' },
      one: '{count, number} source configurée',
      other: '{count, number} sources configurées',
    },
    attributes: {
      one: '{count, number} attribut',
      other: '{count, number} attributs',
    },
  },
};

const en: Catalog = {
  common: {
    results: {
      exact: { 0: 'No results' },
      one: '{count, number} result',
      other: '{count, number} results',
    },
    selected: {
      one: '{count, number} selected',
      other: '{count, number} selected',
    },
    assets: {
      exact: { 0: 'No assets' },
      one: '{count, number} asset',
      other: '{count, number} assets',
    },
    days: {
      one: '{count, number} day',
      other: '{count, number} days',
    },
    controls: {
      exact: { 0: 'No controls' },
      one: '{count, number} control',
      other: '{count, number} controls',
    },
    frameworks: {
      one: '{count, number} framework',
      other: '{count, number} frameworks',
    },
    sources: {
      exact: { 0: 'No source configured' },
      one: '{count, number} source configured',
      other: '{count, number} sources configured',
    },
    attributes: {
      one: '{count, number} attribute',
      other: '{count, number} attributes',
    },
  },
};

/** Partial by design: a locale with no overlay simply has none. */
export const runtimeMessages: Partial<Record<LocaleCode, Catalog>> = { fr, en };
