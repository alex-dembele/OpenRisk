// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// Governed bulk operations (#582) — the client-side state machine.
//
// The rule this hook encodes: a bulk action is never applied straight from a
// button press. It is previewed, the preview is shown, the user confirms, and
// the confirmation carries the preview's fingerprint back to the server so a
// selection that moved in between is refused rather than applied to rows the
// user never saw (criterion 3). The server enforces that; this hook is what
// makes the UI unable to skip it.
//
// It owns no rendering. <BulkPreviewDialog> renders `phase`; the pages own the
// bulk-bar entries and the cache shape.

import { useCallback, useMemo, useRef, useState } from 'react';
import { useMutation, useQuery, useQueryClient, type QueryKey } from '@tanstack/react-query';

import {
  bulkService,
  isMissingRows,
  isStalePreview,
  type BulkActionKind,
  type BulkPreview,
  type BulkRegister,
  type BulkResult,
} from '../../services/bulkService';

/** What the user asked for, minus the ids. */
export interface BulkChangeInput {
  action: BulkActionKind;
  /** Target value, for `change_status`. */
  status?: string;
}

/**
 * How to reflect a pending change in the cache before the server answers
 * (ABSOLUTE RULE 10). Every query whose key starts with `queryKey` is
 * snapshotted, patched through `apply`, and restored verbatim if the request
 * fails — criterion 8.
 *
 * The page supplies `apply` because only the page knows its cache shape: the
 * inventory caches `Asset[]`, the vulnerability register caches a paged object.
 *
 * `cached` is `unknown` on purpose. A key prefix in this app covers more than
 * one shape — `['assets']` also holds each asset's history — so handing the page
 * a confidently-typed value would be a lie it could not check. Narrow it.
 */
export interface OptimisticPlan {
  /** Prefix of the queries to patch. Keep it narrow enough to be patchable. */
  queryKey: readonly unknown[];
  apply: (cached: unknown, ids: ReadonlySet<string>, change: BulkChangeInput) => unknown;
  /**
   * Prefix to invalidate once the server has answered. Defaults to `queryKey`.
   * Usually wider: a bulk status change moves the KPI counts too, and those live
   * under a sibling key the patch cannot express.
   */
  invalidateKey?: readonly unknown[];
}

/**
 * Why a confirmed action did not run. Both are the server refusing to touch a
 * set the user did not see, and both are recoverable by previewing again.
 */
export type BulkRefusal = 'moved' | 'missing';

export type BulkPhase =
  | { kind: 'idle' }
  | { kind: 'previewing'; change: BulkChangeInput; count: number }
  | { kind: 'preview-failed'; change: BulkChangeInput; detail?: string }
  | { kind: 'ready'; change: BulkChangeInput; preview: BulkPreview }
  | { kind: 'applying'; change: BulkChangeInput; preview: BulkPreview }
  | { kind: 'refused'; change: BulkChangeInput; preview: BulkPreview; refusal: BulkRefusal }
  | { kind: 'apply-failed'; change: BulkChangeInput; preview: BulkPreview; detail?: string };

export interface UseGovernedBulkOptions {
  register: BulkRegister;
  /** Cache to patch optimistically and to invalidate once the server answers. */
  optimistic: OptimisticPlan;
  /** Called after a successful apply, for the toast. */
  onApplied?: (result: BulkResult, change: BulkChangeInput) => void;
}

export interface UseGovernedBulk {
  /** Actions the SERVER says this register supports. Empty while loading. */
  supported: BulkActionKind[];
  /** False until capabilities are known, so nothing is offered on a guess. */
  supports: (action: BulkActionKind) => boolean;
  capabilitiesLoading: boolean;
  /**
   * Start an interaction: preview, then wait for the user.
   *
   * The returned promise settles when the dialog closes — resolved if the
   * action applied or the user cancelled, rejected if it failed. That is what
   * keeps the bulk bar's own in-flight state (criterion 7) covering the whole
   * interaction rather than just the network call.
   */
  request: (change: BulkChangeInput, ids: string[]) => Promise<void>;
  phase: BulkPhase;
  /** Apply what the preview showed. */
  confirm: (justification?: string) => void;
  /** Re-run the preview — after a refusal, or after a failed preview. */
  retryPreview: () => void;
  /** Close. Rejects the pending request when the interaction ended in failure. */
  close: () => void;
}

export function useGovernedBulk({
  register,
  optimistic,
  onApplied,
}: UseGovernedBulkOptions): UseGovernedBulk {
  const queryClient = useQueryClient();
  const [phase, setPhase] = useState<BulkPhase>({ kind: 'idle' });

  // The ids under interaction. Held in a ref, not in `phase`: they are an input
  // to confirm/retry, never rendered, and keeping them out of the phase keeps
  // the rendered states small enough to read.
  const idsRef = useRef<string[]>([]);
  // Resolver of the promise handed to the bulk bar.
  const settleRef = useRef<{ resolve: () => void; reject: (reason: Error) => void } | null>(null);

  const capabilities = useQuery({
    queryKey: ['bulk', 'capabilities', register],
    queryFn: ({ signal }) => bulkService.capabilities(register, signal),
    // Capabilities are a property of the build, not of the data. Refetching them
    // on every window focus would be a request per register per focus for an
    // answer that cannot change while the tab is open.
    staleTime: Infinity,
    retry: false,
  });

  const supported = useMemo<BulkActionKind[]>(
    () => capabilities.data?.actions ?? [],
    [capabilities.data],
  );

  const supports = useCallback((action: BulkActionKind) => supported.includes(action), [supported]);

  const settle = useCallback((error?: Error) => {
    const pending = settleRef.current;
    settleRef.current = null;
    if (!pending) return;
    if (error) pending.reject(error);
    else pending.resolve();
  }, []);

  const runPreview = useCallback(
    async (change: BulkChangeInput, ids: string[]) => {
      setPhase({ kind: 'previewing', change, count: ids.length });
      try {
        const preview = await bulkService.preview(register, change.action, ids, change.status);
        setPhase({ kind: 'ready', change, preview });
      } catch (error) {
        setPhase({ kind: 'preview-failed', change, detail: detailOf(error) });
      }
    },
    [register],
  );

  const request = useCallback(
    (change: BulkChangeInput, ids: string[]) => {
      idsRef.current = ids;
      const promise = new Promise<void>((resolve, reject) => {
        settleRef.current = { resolve, reject };
      });
      void runPreview(change, ids);
      return promise;
    },
    [runPreview],
  );

  const apply = useMutation({
    mutationFn: ({
      change,
      preview,
      justification,
    }: {
      change: BulkChangeInput;
      preview: BulkPreview;
      justification?: string;
    }) =>
      bulkService.apply(register, change.action, {
        ids: idsRef.current,
        status: change.status,
        justification,
        fingerprint: preview.fingerprint,
      }),

    onMutate: async ({ change }) => {
      // Stop an in-flight refetch from landing on top of the optimistic state
      // and being mistaken for the server's answer.
      await queryClient.cancelQueries({ queryKey: optimistic.queryKey });
      const snapshot = queryClient.getQueriesData({ queryKey: optimistic.queryKey });
      const ids = new Set(idsRef.current);
      queryClient.setQueriesData({ queryKey: optimistic.queryKey }, (cached: unknown) =>
        cached === undefined ? cached : optimistic.apply(cached, ids, change),
      );
      return { snapshot };
    },

    onError: (error, { change, preview }, context) => {
      // Criterion 8: put back exactly what was there. Every snapshotted key,
      // including the ones a filter made distinct.
      for (const [key, data] of context?.snapshot ?? []) {
        queryClient.setQueryData(key as QueryKey, data);
      }
      if (isStalePreview(error)) {
        setPhase({ kind: 'refused', change, preview, refusal: 'moved' });
        return;
      }
      if (isMissingRows(error)) {
        setPhase({ kind: 'refused', change, preview, refusal: 'missing' });
        return;
      }
      setPhase({ kind: 'apply-failed', change, preview, detail: detailOf(error) });
    },

    onSuccess: (result, { change }) => {
      setPhase({ kind: 'idle' });
      settle();
      onApplied?.(result, change);
    },

    // Truth comes from the server either way: a success invalidates so the
    // optimistic rows are replaced by real ones, a failure so the restored
    // snapshot is re-verified rather than trusted.
    onSettled: () => {
      void queryClient.invalidateQueries({
        queryKey: optimistic.invalidateKey ?? optimistic.queryKey,
      });
    },
  });

  const confirm = useCallback(
    (justification?: string) => {
      if (phase.kind !== 'ready') return;
      setPhase({ kind: 'applying', change: phase.change, preview: phase.preview });
      apply.mutate({ change: phase.change, preview: phase.preview, justification });
    },
    [apply, phase],
  );

  const retryPreview = useCallback(() => {
    if (phase.kind === 'idle' || phase.kind === 'previewing' || phase.kind === 'applying') return;
    void runPreview(phase.change, idsRef.current);
  }, [phase, runPreview]);

  const close = useCallback(() => {
    // An apply in flight is not cancellable — it is a transaction on the server.
    // Refusing to close is the honest answer; the dialog hides its dismiss
    // affordances in that phase anyway.
    if (phase.kind === 'applying') return;
    const failed = phase.kind === 'apply-failed' || phase.kind === 'refused';
    setPhase({ kind: 'idle' });
    settle(failed ? new Error('bulk action did not apply') : undefined);
  }, [phase, settle]);

  return {
    supported,
    supports,
    capabilitiesLoading: capabilities.isLoading,
    request,
    phase,
    confirm,
    retryPreview,
    close,
  };
}

/**
 * The server's message, when it sent one.
 *
 * Shown collapsed under the error state for support to quote. Never surfaced as
 * the primary message: it is English, unlocalised and written for us.
 */
function detailOf(error: unknown): string | undefined {
  if (typeof error !== 'object' || error === null) return undefined;
  const response = (error as { response?: { data?: { error?: unknown } } }).response;
  const message = response?.data?.error;
  if (typeof message === 'string' && message.length > 0) return message;
  return error instanceof Error ? error.message : undefined;
}
