// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// FR/EN parity is a gate, not a convention.
//
// A key that exists in one catalogue and not the other does not fail loudly: it
// silently falls back to the other language, so the screen reads half-French to
// an English user and nobody notices until a customer does. This test is the
// notice.

import { describe, expect, it } from 'vitest';
import enJson from '../../locales/en.json';
import frJson from '../../locales/fr.json';
import { runtimeMessages } from '../messages';
import { isPluralForms } from '../plural';
import type { Catalog } from '../translate';

/** Every leaf path in a catalogue, dotted. Plural sets count as one leaf. */
function keysOf(node: Catalog, prefix = ''): string[] {
  const out: string[] = [];
  for (const [key, value] of Object.entries(node)) {
    const path = prefix ? `${prefix}.${key}` : key;
    if (typeof value === 'object' && value !== null && !isPluralForms(value)) {
      out.push(...keysOf(value as Catalog, path));
    } else {
      out.push(path);
    }
  }
  return out.sort();
}

const frKeys = keysOf(frJson as Catalog);
const enKeys = keysOf(enJson as Catalog);

describe('FR/EN catalogue parity', () => {
  it('has no key present in French and missing in English', () => {
    expect(frKeys.filter((k) => !enKeys.includes(k))).toEqual([]);
  });

  it('has no key present in English and missing in French', () => {
    expect(enKeys.filter((k) => !frKeys.includes(k))).toEqual([]);
  });

  it('has no empty translation, which renders as a silent hole', () => {
    const empty = (node: Catalog, prefix = ''): string[] => {
      const out: string[] = [];
      for (const [key, value] of Object.entries(node)) {
        const path = prefix ? `${prefix}.${key}` : key;
        if (typeof value === 'string') {
          if (value.trim() === '') out.push(path);
        } else if (value && !isPluralForms(value)) {
          out.push(...empty(value as Catalog, path));
        }
      }
      return out;
    };
    expect(empty(frJson as Catalog)).toEqual([]);
    expect(empty(enJson as Catalog)).toEqual([]);
  });

  it('keeps the TypeScript plural overlay in parity too', () => {
    const fr = keysOf(runtimeMessages.fr ?? {});
    const en = keysOf(runtimeMessages.en ?? {});
    expect(fr).toEqual(en);
  });

  it('defines `other` on every plural set — the one universal category', () => {
    const check = (node: Catalog, prefix = ''): void => {
      for (const [key, value] of Object.entries(node)) {
        const path = prefix ? `${prefix}.${key}` : key;
        if (isPluralForms(value)) {
          expect(typeof value.other, `${path} must define "other"`).toBe('string');
        } else if (typeof value === 'object' && value !== null) {
          check(value as Catalog, path);
        }
      }
    };
    check(runtimeMessages.fr ?? {});
    check(runtimeMessages.en ?? {});
  });
});
