// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// Governed bulk operations (#582) — the client half of criteria 2, 3, 4, 6, 7
// and 8.
//
// What is worth testing here is not that the dialog renders. It is that the UI
// cannot get to an apply without a preview, that it refuses rather than retries
// when the server says the selection moved, and that a rejected batch leaves
// the list exactly as it was. Those are the guarantees a user is being asked to
// trust when they press a button that changes a hundred rows at once.

import { useState } from 'react';
import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { BulkPreviewDialog } from '../BulkPreviewDialog';
import { useGovernedBulk, type BulkChangeInput } from '../useGovernedBulk';
import { BulkBar } from '../../datatable/BulkBar';
import { useUIStore } from '../../../store/uiStore';
import type { BulkPreview, BulkResult } from '../../../services/bulkService';

/* ------------------------------------------------------------------ mocks -- */

const capabilities = vi.fn();
const preview = vi.fn();
const apply = vi.fn();

vi.mock('../../../services/bulkService', async (importOriginal) => {
  // The 409/404 predicates are the real ones on purpose: mocking them would
  // test that the mock returns true, not that a stale preview is recognised.
  const actual = await importOriginal<typeof import('../../../services/bulkService')>();
  return {
    ...actual,
    bulkService: {
      capabilities: (...args: unknown[]) => capabilities(...args),
      preview: (...args: unknown[]) => preview(...args),
      apply: (...args: unknown[]) => apply(...args),
    },
  };
});

/** An axios-shaped rejection, which is what the predicates read. */
const httpError = (status: number, message = 'refused') =>
  Object.assign(new Error(message), { response: { status, data: { error: message } } });

const A_PREVIEW: BulkPreview = {
  requested: 3,
  found: 3,
  missing: [],
  affected: 2,
  unchanged: 1,
  fingerprint: 'f'.repeat(64),
  action: 'change_status',
  sample: [
    {
      id: '11111111-1111-4111-8111-111111111111',
      label: 'CVE-2026-0001',
      before: { status: 'open' },
      after: { status: 'in_remediation' },
      changed_fields: ['status'],
    },
  ],
};

const IDS = [
  '11111111-1111-4111-8111-111111111111',
  '22222222-2222-4222-8222-222222222222',
  '33333333-3333-4333-8333-333333333333',
];

/* ---------------------------------------------------------------- harness -- */

interface HarnessProps {
  /** Seeded cache, so the optimistic patch has something real to rewrite. */
  seed?: { id: string; status: string }[];
  change?: BulkChangeInput;
  onSettled?: (outcome: 'resolved' | 'rejected') => void;
}

let queryClient: QueryClient;

function Harness({
  seed = [
    { id: IDS[0], status: 'open' },
    { id: IDS[1], status: 'open' },
  ],
  change = { action: 'change_status', status: 'in_remediation' },
  onSettled,
}: HarnessProps) {
  const [applied, setApplied] = useState<number | null>(null);

  const bulk = useGovernedBulk({
    register: 'vulnerabilities',
    optimistic: {
      queryKey: ['vulnerabilities', 'list'],
      apply: (cached, ids, pending) => {
        if (!Array.isArray(cached)) return cached;
        const rows = cached as { id: string; status: string }[];
        return pending.action === 'delete'
          ? rows.filter((r) => !ids.has(r.id))
          : rows.map((r) => (ids.has(r.id) ? { ...r, status: pending.status ?? r.status } : r));
      },
    },
    onApplied: (result) => setApplied(result.applied ?? 0),
  });

  return (
    <>
      <BulkBar
        count={IDS.length}
        actions={[
          {
            key: 'change-status',
            label: 'Mark in remediation',
            hidden: !bulk.supports('change_status'),
            run: () =>
              bulk.request(change, IDS).then(
                () => onSettled?.('resolved'),
                () => onSettled?.('rejected'),
              ),
          },
          {
            key: 'delete',
            label: 'Delete',
            danger: true,
            hidden: !bulk.supports('delete'),
            run: () =>
              bulk.request({ action: 'delete' }, IDS).then(
                () => onSettled?.('resolved'),
                () => onSettled?.('rejected'),
              ),
          },
        ]}
        buildScope={() => ({
          scope: 'selection',
          ids: IDS,
          rows: [],
          count: IDS.length,
          state: { q: '', sort: null, page: 1, pageSize: 50, filters: {} },
        })}
        onClear={() => {}}
        labels={{ selected: (n) => `${n} selected`, clear: 'Clear', failed: 'Failed' }}
      />
      <BulkPreviewDialog bulk={bulk} entityLabel="vulnerabilities" />
      <output data-testid="applied">{applied === null ? '' : String(applied)}</output>
      <output data-testid="cache">
        {JSON.stringify(queryClient.getQueryData(['vulnerabilities', 'list', {}]) ?? seed)}
      </output>
    </>
  );
}

function renderHarness(props: HarnessProps = {}) {
  queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  queryClient.setQueryData(
    ['vulnerabilities', 'list', {}],
    props.seed ?? [
      { id: IDS[0], status: 'open' },
      { id: IDS[1], status: 'open' },
    ],
  );
  return render(
    <QueryClientProvider client={queryClient}>
      <Harness {...props} />
    </QueryClientProvider>,
  );
}

const cache = () => JSON.parse(screen.getByTestId('cache').textContent ?? '[]');

beforeEach(() => {
  vi.clearAllMocks();
  useUIStore.setState({ lang: 'en' });
  capabilities.mockResolvedValue({
    entity_type: 'vulnerability',
    actions: ['change_status', 'delete'],
  });
  preview.mockResolvedValue(A_PREVIEW);
  apply.mockResolvedValue({ total: 2, applied: 2, ids: IDS.slice(0, 2), audited: 2 } as BulkResult);
});

/* ------------------------------------------------- criterion 2: preview -- */

describe('the preview is not skippable', () => {
  it('previews on the button press and applies nothing until the user confirms', async () => {
    const user = userEvent.setup();
    renderHarness();

    await user.click(await screen.findByTestId('bulk-action-change-status'));

    await screen.findByRole('alertdialog');
    expect(preview).toHaveBeenCalledWith('vulnerabilities', 'change_status', IDS, 'in_remediation');
    // The whole point: the dialog is open and the server has not been asked to
    // change anything.
    expect(apply).not.toHaveBeenCalled();
  });

  it('shows what would change — the counts and the real rows, not a row count alone', async () => {
    const user = userEvent.setup();
    renderHarness();
    await user.click(await screen.findByTestId('bulk-action-change-status'));

    expect(await screen.findByTestId('bulk-affected')).toHaveTextContent('2');
    const list = await screen.findByTestId('bulk-preview-changes');
    expect(within(list).getByText('CVE-2026-0001')).toBeInTheDocument();
    expect(list).toHaveTextContent('open');
    expect(list).toHaveTextContent('in_remediation');
  });

  it('sends the preview fingerprint back with the apply', async () => {
    const user = userEvent.setup();
    renderHarness();
    await user.click(await screen.findByTestId('bulk-action-change-status'));
    await user.click(await screen.findByRole('button', { name: 'Apply' }));

    await waitFor(() =>
      expect(apply).toHaveBeenCalledWith('vulnerabilities', 'change_status', {
        ids: IDS,
        status: 'in_remediation',
        justification: undefined,
        fingerprint: A_PREVIEW.fingerprint,
      }),
    );
    await waitFor(() => expect(screen.getByTestId('applied')).toHaveTextContent('2'));
  });

  it('carries the typed reason onto the apply, for the audit entries', async () => {
    const user = userEvent.setup();
    renderHarness();
    await user.click(await screen.findByTestId('bulk-action-change-status'));
    await user.type(await screen.findByRole('textbox'), 'Q3 remediation push');
    await user.click(screen.getByRole('button', { name: 'Apply' }));

    await waitFor(() =>
      expect(apply).toHaveBeenCalledWith(
        'vulnerabilities',
        'change_status',
        expect.objectContaining({ justification: 'Q3 remediation push' }),
      ),
    );
  });

  it('offers nothing to confirm when the change would alter no row', async () => {
    preview.mockResolvedValue({ ...A_PREVIEW, affected: 0, unchanged: 3, sample: [] });
    const user = userEvent.setup();
    renderHarness();
    await user.click(await screen.findByTestId('bulk-action-change-status'));

    expect(await screen.findByTestId('bulk-preview-empty')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Apply' })).toBeDisabled();
  });

  it('says which rows have gone rather than quietly shrinking the batch', async () => {
    preview.mockResolvedValue({ ...A_PREVIEW, found: 2, missing: [IDS[2]] });
    const user = userEvent.setup();
    renderHarness();
    await user.click(await screen.findByTestId('bulk-action-change-status'));

    expect(await screen.findByTestId('bulk-preview-missing')).toHaveTextContent('1');
  });
});

/* --------------------------------------- criterion 3: the set that moved -- */

describe('a selection that moved under the user', () => {
  it('reports the mismatch instead of applying to the new set', async () => {
    apply.mockRejectedValue(httpError(409, 'selection changed since the preview'));
    const user = userEvent.setup();
    renderHarness();

    await user.click(await screen.findByTestId('bulk-action-change-status'));
    await user.click(await screen.findByRole('button', { name: 'Apply' }));

    const refusal = await screen.findByTestId('bulk-preview-refused');
    expect(refusal).toHaveTextContent(/changed since the preview/i);
    // Not a retry loop: the only way on is a fresh preview.
    expect(screen.getByRole('button', { name: 'Preview again' })).toBeInTheDocument();
    expect(apply).toHaveBeenCalledTimes(1);
  });

  it('re-previews on demand, and the second preview is the one that governs', async () => {
    apply.mockRejectedValueOnce(httpError(409));
    const user = userEvent.setup();
    renderHarness();

    await user.click(await screen.findByTestId('bulk-action-change-status'));
    await user.click(await screen.findByRole('button', { name: 'Apply' }));
    await screen.findByTestId('bulk-preview-refused');

    preview.mockResolvedValue({ ...A_PREVIEW, fingerprint: 'a'.repeat(64), affected: 1 });
    await user.click(screen.getByRole('button', { name: 'Preview again' }));

    await waitFor(() => expect(screen.getByTestId('bulk-affected')).toHaveTextContent('1'));
    await user.click(screen.getByRole('button', { name: 'Apply' }));
    await waitFor(() =>
      expect(apply).toHaveBeenLastCalledWith(
        'vulnerabilities',
        'change_status',
        expect.objectContaining({ fingerprint: 'a'.repeat(64) }),
      ),
    );
  });

  it('treats vanished rows as a refusal too, not as a partial success', async () => {
    apply.mockRejectedValue(httpError(404, 'an id in the selection does not resolve'));
    const user = userEvent.setup();
    renderHarness();

    await user.click(await screen.findByTestId('bulk-action-change-status'));
    await user.click(await screen.findByRole('button', { name: 'Apply' }));

    expect(await screen.findByTestId('bulk-preview-refused')).toHaveTextContent(/no longer exist/i);
  });
});

/* ------------------------------------------- criterion 8: the rollback -- */

describe('optimistic update', () => {
  it('shows the change immediately and puts the list back when the server refuses', async () => {
    let rejectApply: (reason: unknown) => void = () => {};
    apply.mockImplementation(
      () =>
        new Promise((_resolve, reject) => {
          rejectApply = reject;
        }),
    );

    const user = userEvent.setup();
    renderHarness();
    await user.click(await screen.findByTestId('bulk-action-change-status'));
    await user.click(await screen.findByRole('button', { name: 'Apply' }));

    // In flight: the list already reads as if it had worked (ABSOLUTE RULE 10).
    await waitFor(() =>
      expect(cache().filter((r: { status: string }) => r.status === 'in_remediation')).toHaveLength(
        2,
      ),
    );

    rejectApply(httpError(500, 'the database went away'));

    // Refused: byte-identical to what was there before the press.
    await waitFor(() =>
      expect(cache()).toEqual([
        { id: IDS[0], status: 'open' },
        { id: IDS[1], status: 'open' },
      ]),
    );
    expect(await screen.findByText(/did not run/i)).toBeInTheDocument();
  });

  it('drops the rows optimistically on a bulk delete', async () => {
    let resolveApply: (value: BulkResult) => void = () => {};
    apply.mockImplementation(() => new Promise((resolve) => (resolveApply = resolve)));

    const user = userEvent.setup();
    renderHarness();
    await user.click(await screen.findByTestId('bulk-action-delete'));
    // Both the bar and the dialog say "Delete"; the confirm is the dialog's.
    const confirmDialog = await screen.findByRole('alertdialog');
    await user.click(within(confirmDialog).getByRole('button', { name: 'Delete' }));

    await waitFor(() => expect(cache()).toHaveLength(0));
    resolveApply({ total: 2, applied: 2, ids: IDS.slice(0, 2), audited: 2 });
  });
});

/* ----------------------------------- criterion 4: the action is not offered -- */

describe('server-declared capabilities', () => {
  it('does not offer an action this register cannot perform', async () => {
    capabilities.mockResolvedValue({ entity_type: 'asset', actions: ['delete'] });
    renderHarness();

    expect(await screen.findByTestId('bulk-action-delete')).toBeInTheDocument();
    expect(screen.queryByTestId('bulk-action-change-status')).not.toBeInTheDocument();
  });

  it('offers nothing at all until the server has answered', () => {
    // A pending capabilities call must not be read as "everything is allowed".
    capabilities.mockReturnValue(new Promise(() => {}));
    renderHarness();

    expect(screen.queryByTestId('bulk-bar')).not.toBeInTheDocument();
  });
});

/* ---------------------------------- criterion 7: the bar's in-flight state -- */

describe('in-flight state', () => {
  it('keeps the bulk bar busy for the whole interaction, not just the request', async () => {
    const user = userEvent.setup();
    const settled = vi.fn();
    renderHarness({ onSettled: settled });

    await user.click(await screen.findByTestId('bulk-action-change-status'));
    // Dialog open, nothing applied: the bar's other action is still disabled,
    // because the interaction it started has not ended.
    expect(screen.getByTestId('bulk-action-delete')).toBeDisabled();
    expect(settled).not.toHaveBeenCalled();

    await user.click(await screen.findByRole('button', { name: 'Apply' }));
    await waitFor(() => expect(settled).toHaveBeenCalledWith('resolved'));
  });

  it('shows a skeleton while the preview is being computed — never a page spinner', async () => {
    preview.mockReturnValue(new Promise(() => {}));
    const user = userEvent.setup();
    renderHarness();

    await user.click(await screen.findByTestId('bulk-action-change-status'));
    const loading = await screen.findByTestId('bulk-preview-loading');
    expect(within(loading).getByRole('status')).toHaveTextContent(/what would change/i);
  });

  it('surfaces a failed preview as a retryable error, with nothing applied', async () => {
    preview.mockRejectedValue(httpError(500, 'preview blew up'));
    const user = userEvent.setup();
    renderHarness();

    await user.click(await screen.findByTestId('bulk-action-change-status'));
    expect(await screen.findByText(/could not be computed/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Preview again' })).toBeInTheDocument();
    expect(apply).not.toHaveBeenCalled();
  });
});

/* --------------------------------------- criterion 6: keyboard and axe -- */

describe('accessibility', () => {
  it('opens from the bar and hands focus to the dialog, keyboard only', async () => {
    const user = userEvent.setup();
    renderHarness();

    const action = await screen.findByTestId('bulk-action-change-status');
    action.focus();
    await user.keyboard('{Enter}');

    const dialog = await screen.findByRole('alertdialog');
    // Focus moved into the dialog rather than being left behind on the bar.
    await waitFor(() => expect(dialog.contains(document.activeElement)).toBe(true));
  });

  it('puts both answers in the tab order and lets the confirm be pressed by key', async () => {
    // NOT a tab traversal. The shared focus trap filters candidates on
    // `offsetParent`, which jsdom always reports as null because it does no
    // layout, so tabbing here would prove a jsdom quirk rather than the
    // keyboard contract. Real traversal is covered against a browser by the
    // Playwright suite. What IS provable here: neither control is removed from
    // the tab order or from the accessibility tree, and the confirm activates
    // from the keyboard.
    const user = userEvent.setup();
    renderHarness();
    await user.click(await screen.findByTestId('bulk-action-change-status'));

    const dialog = await screen.findByRole('alertdialog');
    const confirm = within(dialog).getByRole('button', { name: 'Apply' });
    const cancels = within(dialog).getAllByRole('button', { name: 'Cancel' });

    for (const control of [confirm, ...cancels]) {
      expect(control).toBeEnabled();
      expect(control).not.toHaveAttribute('tabindex', '-1');
      expect(control).not.toHaveAttribute('aria-hidden', 'true');
    }

    // Let the dialog's own opening focus land first — it is scheduled on a
    // frame, and it would otherwise steal focus back mid-test.
    await waitFor(() => expect(dialog.contains(document.activeElement)).toBe(true));
    confirm.focus();
    expect(document.activeElement).toBe(confirm);
    await user.keyboard('{Enter}');
    await waitFor(() => expect(apply).toHaveBeenCalledTimes(1));
  });

  it('closes on Escape without applying anything', async () => {
    const user = userEvent.setup();
    const settled = vi.fn();
    renderHarness({ onSettled: settled });

    await user.click(await screen.findByTestId('bulk-action-change-status'));
    await screen.findByRole('alertdialog');
    await user.keyboard('{Escape}');

    await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument());
    expect(apply).not.toHaveBeenCalled();
    // Cancelling is not a failure — the bar must not flash an error.
    await waitFor(() => expect(settled).toHaveBeenCalledWith('resolved'));
  });

  it('finds no serious or critical violation on the bar or the open preview', async () => {
    const axe = (await import('axe-core')).default;
    const user = userEvent.setup();
    const { baseElement } = renderHarness();

    await user.click(await screen.findByTestId('bulk-action-change-status'));
    await screen.findByRole('alertdialog');

    const results = await axe.run(baseElement, {
      resultTypes: ['violations'],
      // jsdom computes no colours, so contrast here would assert nothing. It is
      // covered against a real browser by the Playwright visual suite.
      rules: { 'color-contrast': { enabled: false } },
    });
    const serious = results.violations.filter(
      (v) => v.impact === 'serious' || v.impact === 'critical',
    );
    expect(serious.map((v) => `${v.id}: ${v.help}`)).toEqual([]);
  }, 20_000);
});
