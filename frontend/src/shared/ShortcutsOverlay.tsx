// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// Keyboard-shortcut help overlay (UX-26): opened with `?` (or the header button),
// it lists the key actions so shortcuts are discoverable rather than hidden. The
// display rows mirror the handlers registered in the shell (App DashboardLayout) —
// keep the two in sync. Theme-aware; closes on Esc / backdrop.

import { useEffect } from 'react';
import { Keyboard, X } from 'lucide-react';
import type { LocaleCode } from '../i18n/locales';

interface Row {
  keys: string[];
  label: string;
}
interface Section {
  title: string;
  rows: Row[];
}

function sections(fr: boolean): Section[] {
  const tr = (f: string, e: string) => (fr ? f : e);
  return [
    {
      title: tr('Actions rapides', 'Quick actions'),
      rows: [
        { keys: ['N'], label: tr('Nouveau risque', 'New risk') },
        { keys: ['/'], label: tr('Recherche & commandes', 'Search & commands') },
        { keys: ['G'], label: tr('Aller au tableau de bord', 'Go to dashboard') },
        { keys: ['T'], label: tr('Basculer le thème', 'Toggle theme') },
      ],
    },
    {
      title: tr('Aide & système', 'Help & system'),
      rows: [
        { keys: ['?'], label: tr('Afficher / masquer ce panneau', 'Show / hide this panel') },
        { keys: ['⌘', 'K'], label: tr('Palette de commandes', 'Command palette') },
        { keys: ['Esc'], label: tr('Fermer les fenêtres', 'Close dialogs') },
      ],
    },
  ];
}

export function ShortcutsOverlay({
  open,
  onClose,
  lang,
}: {
  open: boolean;
  onClose: () => void;
  lang: LocaleCode;
}) {
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [open, onClose]);

  if (!open) return null;
  const fr = lang === 'fr';

  return (
    <div
      className="fixed inset-0 z-85 flex items-center justify-center p-4"
      style={{ background: 'var(--surface-overlay)', backdropFilter: 'blur(4px)' }}
      onClick={onClose}
    >
      <div
        className="w-full max-w-[460px] rounded-[16px] overflow-hidden"
        style={{
          background: 'var(--bg-elevated)',
          border: '1px solid var(--border)',
          boxShadow: 'var(--shadow-overlay)',
          animation: 'or-scalein .14s cubic-bezier(.2,.8,.2,1)',
        }}
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-label={fr ? 'Raccourcis clavier' : 'Keyboard shortcuts'}
      >
        <div
          className="flex items-center gap-3 p-5"
          style={{ borderBottom: '1px solid var(--border)' }}
        >
          <div
            className="w-9 h-9 rounded-[10px] flex items-center justify-center shrink-0"
            style={{ background: 'var(--accent-soft)', color: 'var(--accent-500)' }}
          >
            <Keyboard size={18} />
          </div>
          <div className="flex-1 text-[15px] font-bold text-ink">
            {fr ? 'Raccourcis clavier' : 'Keyboard shortcuts'}
          </div>
          <button
            onClick={onClose}
            className="w-8 h-8 rounded-[8px] flex items-center justify-center text-ink-muted hover:bg-hover transition-colors shrink-0"
            aria-label={fr ? 'Fermer' : 'Close'}
          >
            <X size={17} />
          </button>
        </div>

        <div className="p-5 flex flex-col gap-5">
          {sections(fr).map((sec) => (
            <div key={sec.title}>
              <div className="text-[11px] text-ink-muted uppercase tracking-wide font-semibold mb-2">
                {sec.title}
              </div>
              <div className="flex flex-col gap-1">
                {sec.rows.map((row) => (
                  <div
                    key={row.label}
                    className="flex items-center justify-between gap-3 rounded-[10px] px-3 py-2"
                    style={{ background: 'var(--bg-hover)' }}
                  >
                    <span className="text-[13px] text-ink">{row.label}</span>
                    <span className="flex items-center gap-1 shrink-0">
                      {row.keys.map((k) => (
                        <kbd
                          key={k}
                          className="mono text-[11px] min-w-[22px] text-center px-1.5 py-1 rounded-[6px] text-ink-soft"
                          style={{
                            background: 'var(--bg-elevated)',
                            border: '1px solid var(--border-strong)',
                          }}
                        >
                          {k}
                        </kbd>
                      ))}
                    </span>
                  </div>
                ))}
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
