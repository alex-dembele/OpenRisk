// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// RTL readiness, proved rather than asserted.
//
// The product ships no RTL language today. What it ships is the plumbing: the
// registry owns direction, `<html dir>` follows the active locale, and a
// registered-but-disabled RTL locale can never sneak into the store. Those are
// the three things that would otherwise be discovered broken on the day Arabic
// is switched on.

import { beforeEach, describe, expect, it } from 'vitest';
import { LOCALES, localeDirection } from '../locales';
import { useUIStore } from '../../store/uiStore';

describe('writing direction', () => {
  beforeEach(() => {
    useUIStore.getState().setLang('fr');
  });

  it('puts both lang and dir on <html>', () => {
    useUIStore.getState().setLang('en');
    expect(document.documentElement.getAttribute('lang')).toBe('en');
    expect(document.documentElement.getAttribute('dir')).toBe('ltr');

    useUIStore.getState().setLang('fr');
    expect(document.documentElement.getAttribute('lang')).toBe('fr');
    expect(document.documentElement.getAttribute('dir')).toBe('ltr');
  });

  it('reads direction from the registry, not from a list of RTL codes', () => {
    expect(localeDirection(LOCALES.ar.code as 'ar')).toBe('rtl');
  });

  it('refuses to activate a registered locale that is not offered yet', () => {
    // `ar` has direction and plural rules but no catalogue. Activating it would
    // show a half-translated product, so the store rejects it and holds.
    useUIStore.getState().setLang('ar' as 'fr');
    expect(useUIStore.getState().lang).toBe('fr');
    expect(document.documentElement.getAttribute('dir')).toBe('ltr');
  });

  it('cycles the toggle only through offered languages', () => {
    useUIStore.getState().setLang('fr');
    useUIStore.getState().toggleLang();
    expect(useUIStore.getState().lang).toBe('en');
    useUIStore.getState().toggleLang();
    expect(useUIStore.getState().lang).toBe('fr');
  });
});
