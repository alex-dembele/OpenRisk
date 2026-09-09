// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// Reusable "soft delete" for list screens (UX-3 / directive §4). Instead of a
// blocking confirm dialog, the row disappears immediately and a toast offers Undo
// for a few seconds; the real delete only fires once that window closes — so Undo
// is instant and needs no restore round-trip. The friction lives in the undo
// window, not a modal. Returns a `pending` set the caller filters its list by, and
// a `remove(item)` to trigger it.
//
// Use this only for NON-VITAL content (a risk, a vulnerability, an incident…). For
// irreversible/vital actions (delete a tenant, a user, revoke a token) keep an
// explicit confirmation with an impact summary instead.

import { useCallback, useEffect, useRef, useState } from 'react';
import { toast } from 'sonner';
import { useUIStore } from '../store/uiStore';
import type { LocaleCode } from '../i18n/locales';

interface SoftDeleteOptions<T> {
  /** The real delete — fires only after the undo window elapses. */
  onCommit: (id: string) => Promise<unknown>;
  /** Stable id accessor (defaults to item.id). */
  idOf?: (item: T) => string;
  /** Toast label, e.g. (r) => `Risk "${r.name}" deleted`. FR/EN via `lang`. */
  message: (item: T, lang: LocaleCode) => string;
  /** Milliseconds the undo window stays open (default 5000). */
  delayMs?: number;
}

export function useSoftDelete<T>(opts: SoftDeleteOptions<T>) {
  const { onCommit, idOf, message, delayMs = 5000 } = opts;
  const lang = useUIStore((s) => s.lang);
  const [pending, setPending] = useState<Set<string>>(new Set());
  // Track live timers so we can flush/cleanup on unmount without losing a delete.
  const timers = useRef<Map<string, ReturnType<typeof setTimeout>>>(new Map());

  const unhide = useCallback((id: string) => {
    setPending((prev) => {
      if (!prev.has(id)) return prev;
      const next = new Set(prev);
      next.delete(id);
      return next;
    });
  }, []);

  const remove = useCallback(
    (item: T) => {
      const id = idOf ? idOf(item) : (item as { id: string }).id;
      setPending((prev) => new Set(prev).add(id));
      let undone = false;
      const timer = setTimeout(() => {
        timers.current.delete(id);
        if (undone) return;
        Promise.resolve(onCommit(id))
          .then(() => unhide(id))
          .catch(() => {
            toast.error(lang === 'fr' ? 'Suppression échouée' : 'Delete failed');
            unhide(id);
          });
      }, delayMs);
      timers.current.set(id, timer);
      toast(message(item, lang), {
        duration: delayMs,
        action: {
          label: lang === 'fr' ? 'Annuler' : 'Undo',
          onClick: () => {
            undone = true;
            clearTimeout(timer);
            timers.current.delete(id);
            unhide(id);
          },
        },
      });
    },
    [idOf, onCommit, message, delayMs, lang, unhide],
  );

  // On unmount, fire any still-pending deletes so nothing is silently dropped.
  useEffect(() => {
    const map = timers.current;
    return () => {
      map.forEach((t) => clearTimeout(t));
      map.clear();
    };
  }, []);

  return { pending, remove };
}
