// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// Governed bulk operations (#582) — the API client.
//
// Every register that has them exposes the same four endpoints under its own
// prefix, so this client is parameterised by the prefix rather than duplicated
// per module. Types come from openapi.generated.ts; nothing here is hand-written.

import { api } from '../lib/api';
import type { components } from '../types/openapi.generated';

export type BulkCapabilities = components['schemas']['BulkCapabilities'];
export type BulkPreview = components['schemas']['BulkPreview'];
export type BulkResult = components['schemas']['BulkResult'];
export type BulkSampleChange = components['schemas']['BulkSampleChange'];

/** The actions the API can express. A register supports a subset. */
export type BulkActionKind = NonNullable<BulkPreview['action']>;

/**
 * Which register to act on. The prefix is the API path segment, which is also
 * the permission namespace — keeping them one value stops the two drifting.
 */
export type BulkRegister = 'vulnerabilities' | 'assets';

/** The route segment each action is mounted on. */
const ACTION_PATH: Record<BulkActionKind, string> = {
  change_status: 'change-status',
  delete: 'delete',
};

export interface BulkApplyInput {
  ids: string[];
  status?: string;
  justification?: string;
  /**
   * What the preview returned. Sending it back is what lets the server refuse a
   * selection that moved underneath the user instead of applying to a set they
   * never saw. Omit only when applying without a preview.
   */
  fingerprint?: string;
}

export const bulkService = {
  /**
   * What this register can actually do. Read from the server rather than
   * hard-coded per module, so a delete-only register is never offered a status
   * change the server would refuse.
   */
  async capabilities(register: BulkRegister, signal?: AbortSignal): Promise<BulkCapabilities> {
    const { data } = await api.get<BulkCapabilities>(`/${register}/bulk/capabilities`, { signal });
    return data;
  },

  /** What would change. Mutates nothing. */
  async preview(
    register: BulkRegister,
    action: BulkActionKind,
    ids: string[],
    status?: string,
    signal?: AbortSignal,
  ): Promise<BulkPreview> {
    const { data } = await api.post<BulkPreview>(
      `/${register}/bulk/preview`,
      { action, ids, status },
      { signal },
    );
    return data;
  },

  /** Apply, all or none. */
  async apply(
    register: BulkRegister,
    action: BulkActionKind,
    input: BulkApplyInput,
  ): Promise<BulkResult> {
    const { data } = await api.post<BulkResult>(
      `/${register}/bulk/${ACTION_PATH[action]}`,
      input,
    );
    return data;
  },
};

/**
 * True when the server refused because the selection moved since the preview.
 *
 * Narrowed on the status rather than the message so it survives translation and
 * rewording; 409 is the only thing these endpoints return it for.
 */
export function isStalePreview(error: unknown): boolean {
  return statusOf(error) === 409;
}

/** True when at least one id no longer resolves — stale selection, or foreign. */
export function isMissingRows(error: unknown): boolean {
  return statusOf(error) === 404;
}

function statusOf(error: unknown): number | undefined {
  if (typeof error !== 'object' || error === null || !('response' in error)) return undefined;
  const response = (error as { response?: { status?: number } }).response;
  return typeof response?.status === 'number' ? response.status : undefined;
}
