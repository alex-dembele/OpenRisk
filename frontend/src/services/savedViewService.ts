// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// Saved table views (#580) — the API client.
//
// A filtered register is a *place* (see useTableState). Saving that place used
// to mean localStorage: the view lived in one browser, a colleague had to
// rebuild it by hand, and clearing site data destroyed it. It now lives on the
// server, scoped to the tenant, and may be shared with the whole tenant.
//
// Every request/response type below is DERIVED from openapi.generated.ts, never
// hand-written: if the API changes shape and this file is not updated, the build
// breaks here rather than at runtime in front of a user.

import { z } from 'zod';
import { api } from '../lib/api';
import type { components } from '../types/openapi.generated';

export type SavedViewDTO = components['schemas']['SavedView'];
export type SavedViewVisibility = SavedViewDTO['visibility'];
export type SavedViewStateDTO = NonNullable<SavedViewDTO['state']>;
export type SavedViewColumnsDTO = NonNullable<SavedViewDTO['columns']>;
export type CreateSavedViewBody = components['schemas']['CreateSavedViewInput'];
export type UpdateSavedViewBody = components['schemas']['UpdateSavedViewInput'];

/** The save-view form, validated client-side before anything is sent. */
export const savedViewFormSchema = z.object({
  name: z
    .string()
    .trim()
    .min(1, 'required')
    .max(120, 'tooLong')
    // A view named only of punctuation is unfindable in a list; require a letter
    // or a digit so the picker never renders an unreadable row.
    .refine((v) => /[\p{L}\p{N}]/u.test(v), 'unreadable'),
  visibility: z.enum(['personal', 'shared']),
});

export type SavedViewForm = z.infer<typeof savedViewFormSchema>;

export const savedViewService = {
  /** The views the caller may see for one register: their own + shared ones. */
  async list(tableId: string, signal?: AbortSignal): Promise<SavedViewDTO[]> {
    const { data } = await api.get<SavedViewDTO[]>('/saved-views', {
      params: { table_id: tableId },
      signal,
    });
    return data ?? [];
  },

  async create(body: CreateSavedViewBody): Promise<SavedViewDTO> {
    const { data } = await api.post<SavedViewDTO>('/saved-views', body);
    return data;
  },

  async update(id: string, body: UpdateSavedViewBody): Promise<SavedViewDTO> {
    const { data } = await api.patch<SavedViewDTO>(`/saved-views/${id}`, body);
    return data;
  },

  async remove(id: string): Promise<void> {
    await api.delete(`/saved-views/${id}`);
  },
};

/* ------------------------------------------------- one-time local migration */

/**
 * The localStorage key the pre-#580 build wrote its views to. Kept verbatim:
 * this is the ONLY thing that can still find a user's existing work.
 */
export const legacyViewsKey = (tableId: string) => `openrisk.table.${tableId}.views`;

/** The shape the old build stored. Anything else is ignored, never thrown on. */
const legacyViewSchema = z.object({
  id: z.string(),
  name: z.string().trim().min(1),
  state: z
    .object({
      q: z.string().optional(),
      filters: z.record(z.string(), z.array(z.string())).optional(),
      sort: z.object({ key: z.string(), dir: z.enum(['asc', 'desc']) }).nullish(),
    })
    .default({}),
});

const legacyViewsSchema = z.array(legacyViewSchema);

export interface LegacyView {
  name: string;
  state: SavedViewStateDTO;
}

/** Reads the views this browser still holds for a table. Never throws. */
export function readLegacyViews(tableId: string): LegacyView[] {
  let raw: string | null = null;
  try {
    raw = window.localStorage.getItem(legacyViewsKey(tableId));
  } catch {
    // Private mode / blocked site data. Nothing to migrate.
    return [];
  }
  if (!raw) return [];

  try {
    const parsed = legacyViewsSchema.safeParse(JSON.parse(raw));
    if (!parsed.success) return [];
    return parsed.data.map((v) => ({
      name: v.name,
      state: {
        q: v.state.q ?? '',
        filters: v.state.filters ?? {},
        sort: v.state.sort ?? null,
      },
    }));
  } catch {
    return [];
  }
}

function clearLegacyViews(tableId: string): void {
  try {
    window.localStorage.removeItem(legacyViewsKey(tableId));
  } catch {
    /* nothing to clear */
  }
}

/**
 * Moves this browser's saved views to the server, once.
 *
 * THE RULE THIS FUNCTION EXISTS TO OBEY (#580 criterion 6, second risk): the
 * local copy is cleared only after every view is confirmed stored. If a single
 * POST fails, NOTHING is cleared — the whole set is retried on the next load
 * rather than discarded. Reading, posting and clearing optimistically is how a
 * migration destroys work irreversibly.
 *
 * A 409 counts as success: it means a previous run already stored that view and
 * failed to clear afterwards, which is precisely the retry this design creates.
 *
 * Returns the number of views newly stored, so the caller knows whether to
 * refetch.
 */
export async function migrateLegacyViews(tableId: string): Promise<number> {
  const legacy = readLegacyViews(tableId);
  if (legacy.length === 0) {
    // Nothing to migrate — but an empty array left behind would make every load
    // re-read it. Clearing an empty key loses nothing.
    clearLegacyViews(tableId);
    return 0;
  }

  let stored = 0;
  let allConfirmed = true;

  for (const view of legacy) {
    try {
      await savedViewService.create({
        table_id: tableId,
        name: view.name,
        // A view that existed only in one browser was, by construction, personal.
        // Migrating it as shared would publish one user's working filters to
        // their whole institution without them ever asking.
        visibility: 'personal',
        state: view.state,
      });
      stored += 1;
    } catch (error) {
      if (isConflict(error)) {
        // Already on the server from an earlier attempt.
        continue;
      }
      allConfirmed = false;
    }
  }

  if (allConfirmed) clearLegacyViews(tableId);
  return stored;
}

/** Narrow an axios error to a 409 without importing axios' types here. */
function isConflict(error: unknown): boolean {
  return (
    typeof error === 'object' &&
    error !== null &&
    'response' in error &&
    typeof (error as { response?: { status?: number } }).response?.status === 'number' &&
    (error as { response: { status: number } }).response.status === 409
  );
}
