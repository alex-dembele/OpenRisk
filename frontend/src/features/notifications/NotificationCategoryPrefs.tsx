// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// Per-category notification preferences (UX-6 / directive §5). One row per context
// (Security / Compliance / Tasks / Collaboration / Billing) with an in-app and an
// email switch, so a user tunes each channel per context instead of one blunt
// on/off. Persisted to localStorage for now (a backend NotificationPreference
// model exists and can back this later without changing this UI).

import { useState } from 'react';
import { useUIStore } from '../../store/uiStore';
import { pickLocalized } from '../../i18n/locales';
import {
  NOTIF_CATEGORIES,
  loadNotifPrefs,
  saveNotifPrefs,
  type NotifChannelPrefs,
  type NotifCategory,
} from '../../shared/notificationCategory';

function Switch({ on, onClick, label }: { on: boolean; onClick: () => void; label: string }) {
  return (
    <button
      onClick={onClick}
      role="switch"
      aria-checked={on}
      aria-label={label}
      className="relative shrink-0"
      style={{
        width: 40,
        height: 22,
        borderRadius: 20,
        background: on ? 'var(--accent)' : 'var(--bg-hover)',
        transition: 'background .2s',
      }}
    >
      <span
        className="absolute rounded-full bg-surface-1"
        style={{
          width: 18,
          height: 18,
          top: 2,
          left: on ? 20 : 2,
          transition: 'left .2s',
          boxShadow: '0 1px 3px rgba(0,0,0,.3)',
        }}
      />
    </button>
  );
}

export function NotificationCategoryPrefs() {
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const [prefs, setPrefs] = useState<NotifChannelPrefs>(() => loadNotifPrefs());

  const toggle = (cat: NotifCategory, ch: 'inApp' | 'email') => {
    setPrefs((prev) => {
      const next = { ...prev, [cat]: { ...prev[cat], [ch]: !prev[cat][ch] } };
      saveNotifPrefs(next);
      return next;
    });
  };

  return (
    <div>
      <div className="flex items-center gap-3 pb-2 mb-1 border-b border-border">
        <div className="flex-1 text-[11px] font-semibold uppercase tracking-[.05em] text-ink-muted">
          {tr('Contexte', 'Context')}
        </div>
        <div className="w-[52px] text-center text-[11px] font-semibold uppercase tracking-[.05em] text-ink-muted">
          {tr('In-app', 'In-app')}
        </div>
        <div className="w-[52px] text-center text-[11px] font-semibold uppercase tracking-[.05em] text-ink-muted">
          {tr('E-mail', 'Email')}
        </div>
      </div>
      {NOTIF_CATEGORIES.map((c) => {
        const Icon = c.icon;
        const p = prefs[c.key];
        return (
          <div
            key={c.key}
            className="flex items-center gap-3 py-3"
            style={{ borderBottom: '1px solid var(--border)' }}
          >
            <div
              className="w-[30px] h-[30px] rounded-[9px] flex items-center justify-center shrink-0"
              style={{
                background: `color-mix(in srgb, ${c.color} 14%, transparent)`,
                color: c.color,
              }}
            >
              <Icon size={16} strokeWidth={1.8} />
            </div>
            <div className="flex-1 min-w-0">
              <div className="text-[13.5px] font-medium text-ink">{pickLocalized(lang, c.label)}</div>
              <div className="text-[11.5px] text-ink-muted leading-snug">{pickLocalized(lang, c.desc)}</div>
            </div>
            <div className="w-[52px] flex justify-center">
              <Switch
                on={p.inApp}
                onClick={() => toggle(c.key, 'inApp')}
                label={`${pickLocalized(lang, c.label) ?? ''} in-app`}
              />
            </div>
            <div className="w-[52px] flex justify-center">
              <Switch
                on={p.email}
                onClick={() => toggle(c.key, 'email')}
                label={`${pickLocalized(lang, c.label) ?? ''} email`}
              />
            </div>
          </div>
        );
      })}
    </div>
  );
}
