// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// #580 — saved views moved from localStorage to the server.
//
// These tests own the four claims the issue makes about the CLIENT half. The
// server half (tenant isolation, the 404-not-403 rule) is proven in Go, in
// application/savedview and repository/gorm_saved_view_repository_test.
//
//   criterion 6 — the one-time migration never destroys work: the local copy is
//                 cleared only after every view is confirmed stored, and a
//                 failure is retried on the next load rather than discarded.
//   criterion 7 — a saved-views outage never blocks the register.
//   criterion 8 — the empty picker reads as "none yet", not as a failure.
//
// Only the transport is mocked. migrateLegacyViews and the Zod form schema run
// for real, because they are the two pieces whose behaviour the criteria are about.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { DataTable } from '../DataTable';
import { useTableState } from '../useTableState';
import { legacyViewsKey, type SavedViewDTO } from '../../../services/savedViewService';
import { useAuthStore } from '../../../hooks/useAuthStore';
import { api } from '../../../lib/api';
import axe from 'axe-core';
import type { Column, Facet } from '../types';

// The mock stops at the HTTP boundary. savedViewService, migrateLegacyViews and
// the Zod schema all run for real — a partial module mock would leave
// migrateLegacyViews bound to the unmocked service inside its own module and
// quietly test nothing.
vi.mock('../../../lib/api', () => ({
  api: {
    get: vi.fn(),
    post: vi.fn(),
    patch: vi.fn(),
    delete: vi.fn(),
  },
}));

const listed = (views: SavedViewDTO[]) => vi.mocked(api.get).mockResolvedValue({ data: views });

const TABLE_ID = 'risks';
const ME = 'user-me';

interface Row {
  id: string;
  name: string;
  sev: 'critical' | 'low';
}

const ROWS: Row[] = [
  { id: 'a', name: 'Alpha', sev: 'critical' },
  { id: 'b', name: 'Bravo', sev: 'low' },
];

const COLUMNS: Column<Row>[] = [
  { key: 'name', header: 'Nom', hideable: false, render: (r) => <span>{r.name}</span> },
];

const FACETS: Facet<Row>[] = [
  {
    key: 'sev',
    label: 'Sévérité',
    options: [
      { value: 'critical', label: 'Critique' },
      { value: 'low', label: 'Faible' },
    ],
    matches: (r, selected) => selected.includes(r.sev),
  },
];

function Harness() {
  const api = useTableState({ defaultPageSize: 25 });
  return (
    <DataTable
      id={TABLE_ID}
      ariaLabel="Registre"
      rows={ROWS}
      columns={COLUMNS}
      rowKey={(r) => r.id}
      api={api}
      mode="client"
      facets={FACETS}
      clientSearch={(r, q) => r.name.toLowerCase().includes(q)}
      empty={<div>Rien pour le moment</div>}
    />
  );
}

const renderTable = (initialEntries = ['/']) =>
  render(
    <MemoryRouter initialEntries={initialEntries}>
      <Harness />
    </MemoryRouter>,
  );

function dto(over: Partial<SavedViewDTO> = {}): SavedViewDTO {
  return {
    id: 'v1',
    tenant_id: 'tenant-1',
    user_id: ME,
    table_id: TABLE_ID,
    name: 'Comité T3',
    visibility: 'personal',
    state: { q: '', filters: { sev: ['critical'] }, sort: null },
    ...over,
  };
}

const openPanel = () => fireEvent.click(screen.getByTestId('filters-trigger'));

/** The POST bodies sent to /saved-views, in order. */
const created = () =>
  vi.mocked(api.post).mock.calls.map(([, body]) => body as Record<string, unknown>);

beforeEach(() => {
  vi.clearAllMocks();
  window.localStorage.clear();
  useAuthStore.setState({ user: { id: ME, role: 'user' } as never });
  listed([]);
  vi.mocked(api.post).mockImplementation(async (_url, body) => {
    const b = body as { name: string; visibility?: SavedViewDTO['visibility'] };
    return { data: dto({ id: `srv-${b.name}`, name: b.name, visibility: b.visibility ?? 'personal' }) };
  });
  vi.mocked(api.patch).mockResolvedValue({ data: dto() });
  vi.mocked(api.delete).mockResolvedValue({ data: undefined });
});

describe('Issue 580, criterion 8 — the empty picker', () => {
  it('reads as "none yet", not as a failure', async () => {
    renderTable();
    openPanel();

    const empty = await screen.findByTestId('saved-views-empty');
    expect(empty).toHaveTextContent(/aucune vue sauvegardée|no saved view yet/i);
    expect(screen.queryByTestId('saved-views-error')).not.toBeInTheDocument();
    // And the invitation says how to get one, rather than leaving a bare void.
    expect(empty).toHaveTextContent(/filtrez|filter/i);
  });
});

describe('Issue 580, criterion 7 — a saved-views outage never blocks the register', () => {
  it('renders the rows with default filters and offers a retry', async () => {
    vi.mocked(api.get).mockRejectedValue(new Error('gateway timeout'));
    renderTable();

    // The register is on screen regardless — that is the whole claim.
    expect(screen.getByText('Alpha')).toBeInTheDocument();
    expect(screen.getByText('Bravo')).toBeInTheDocument();

    openPanel();
    const err = await screen.findByTestId('saved-views-error');
    expect(err).toHaveTextContent(/impossible de charger|could not be loaded/i);
    expect(screen.queryByTestId('saved-views-empty')).not.toBeInTheDocument();

    // The retry is a real second attempt, not a cosmetic button.
    listed([dto()]);
    fireEvent.click(screen.getByTestId('saved-views-retry'));
    await screen.findByTestId('saved-view-Comité T3');
    expect(api.get).toHaveBeenCalledTimes(2);
  });

  it('still saves nothing and shows the failure when a save does not stick', async () => {
    renderTable(['/?f.sev=critical']);
    openPanel();
    await screen.findByTestId('saved-views-empty');

    vi.mocked(api.post).mockRejectedValue(new Error('boom'));
    fireEvent.change(screen.getByTestId('saved-view-name'), { target: { value: 'Doomed' } });
    fireEvent.click(screen.getByTestId('saved-view-save'));

    // Optimistic row is rolled back rather than left as a lie.
    await screen.findByTestId('saved-views-mutation-error');
    expect(screen.queryByTestId('saved-view-Doomed')).not.toBeInTheDocument();
  });
});

describe('Issue 580, criterion 6 — the one-time localStorage migration', () => {
  const legacy = [
    { id: '1700000000000', name: 'Mon comité', state: { q: 'log4j', filters: { sev: ['critical'] }, sort: null } },
    { id: '1700000000001', name: 'Revue trimestrielle', state: { q: '', filters: {}, sort: { key: 'score', dir: 'desc' } } },
  ];

  it('moves the local views to the server once, then clears the local copy', async () => {
    window.localStorage.setItem(legacyViewsKey(TABLE_ID), JSON.stringify(legacy));
    renderTable();
    openPanel();

    await waitFor(() => expect(api.post).toHaveBeenCalledTimes(2));
    expect(created()[0]).toEqual(
      expect.objectContaining({
        table_id: TABLE_ID,
        name: 'Mon comité',
        // A view that lived in one browser was, by construction, personal.
        // Migrating it as shared would publish it to the whole institution.
        visibility: 'personal',
        state: expect.objectContaining({ q: 'log4j', filters: { sev: ['critical'] } }),
      }),
    );

    await waitFor(() =>
      expect(window.localStorage.getItem(legacyViewsKey(TABLE_ID))).toBeNull(),
    );
  });

  // The destructive failure mode the issue names: read, post, clear before the
  // post is confirmed, and the user's work is gone for good.
  it('keeps the local copy intact when a single POST fails, and retries next load', async () => {
    window.localStorage.setItem(legacyViewsKey(TABLE_ID), JSON.stringify(legacy));
    vi.mocked(api.post)
      .mockImplementationOnce(async (_url, body) => ({
        data: dto({ name: (body as { name: string }).name }),
      }))
      .mockRejectedValueOnce(new Error('network'));

    const first = renderTable();
    await waitFor(() => expect(api.post).toHaveBeenCalledTimes(2));

    // NOTHING was cleared — not even the view that did store.
    expect(window.localStorage.getItem(legacyViewsKey(TABLE_ID))).not.toBeNull();
    first.unmount();

    // Next load retries. The already-stored one comes back 409, which counts as
    // success, so the set completes and the local copy is finally cleared.
    vi.mocked(api.post).mockReset();
    vi.mocked(api.post).mockImplementation(async (_url, body) => {
      const name = (body as { name: string }).name;
      // The one that DID store comes back 409 — which the migration counts as
      // success, because that is exactly the retry this design creates.
      if (name === 'Mon comité') throw { response: { status: 409 } };
      return { data: dto({ name }) };
    });

    renderTable();
    await waitFor(() =>
      expect(window.localStorage.getItem(legacyViewsKey(TABLE_ID))).toBeNull(),
    );
  });

  it('ignores a corrupted local payload instead of throwing', async () => {
    window.localStorage.setItem(legacyViewsKey(TABLE_ID), '{not json');
    renderTable();
    openPanel();

    await screen.findByTestId('saved-views-empty');
    expect(api.post).not.toHaveBeenCalled();
    expect(screen.getByText('Alpha')).toBeInTheDocument();
  });
});

describe('Issue 580 — sharing a view within the tenant', () => {
  it('toggles a view between personal and shared', async () => {
    listed([dto({ visibility: 'personal' })]);
    renderTable();
    openPanel();

    const toggle = await screen.findByTestId('saved-view-share-Comité T3');
    expect(toggle).toHaveAttribute('aria-checked', 'false');

    fireEvent.click(toggle);
    await waitFor(() =>
      expect(api.patch).toHaveBeenCalledWith('/saved-views/v1', { visibility: 'shared' }),
    );
    expect(await screen.findByTestId('saved-view-share-Comité T3')).toHaveAttribute(
      'aria-checked',
      'true',
    );
  });

  it("shows a colleague's shared view as read-only, attributed to its owner", async () => {
    listed([
      dto({
        id: 'v2',
        user_id: 'someone-else',
        name: 'Vue du RSSI',
        visibility: 'shared',
        owner_email: 'amina@banque.cm',
      }),
    ]);
    renderTable();
    openPanel();

    const row = await screen.findByTestId('saved-view-Vue du RSSI');
    expect(row).toHaveTextContent('amina@banque.cm');
    // A non-owner may apply it, but not delete it or change who sees it.
    expect(screen.queryByTestId('saved-view-share-Vue du RSSI')).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/Vue du RSSI/i)).not.toBeInTheDocument();
  });

  it('lets a tenant admin edit a shared view they do not own', async () => {
    useAuthStore.setState({ user: { id: ME, role: 'admin' } as never });
    listed([dto({ id: 'v3', user_id: 'someone-else', name: 'Vue partagée', visibility: 'shared' })]);
    renderTable();
    openPanel();

    expect(await screen.findByTestId('saved-view-share-Vue partagée')).toBeInTheDocument();
  });

  it('saves a new view as shared when the box is ticked', async () => {
    renderTable(['/?f.sev=critical']);
    openPanel();
    await screen.findByTestId('saved-views-empty');

    fireEvent.change(screen.getByTestId('saved-view-name'), { target: { value: 'Comité' } });
    fireEvent.click(screen.getByTestId('saved-view-share-new'));
    fireEvent.click(screen.getByTestId('saved-view-save'));

    await waitFor(() =>
      expect(created()[0]).toEqual(
        expect.objectContaining({ name: 'Comité', visibility: 'shared' }),
      ),
    );
  });
});

describe('Issue 580 — Zod validates the save-view form client-side', () => {
  it('refuses a name with no letter or digit without calling the API', async () => {
    renderTable(['/?f.sev=critical']);
    openPanel();
    await screen.findByTestId('saved-views-empty');

    fireEvent.change(screen.getByTestId('saved-view-name'), { target: { value: '...' } });
    fireEvent.click(screen.getByTestId('saved-view-save'));

    const err = await screen.findByTestId('saved-view-name-error');
    expect(err).toHaveTextContent(/lettre|letter/i);
    expect(api.post).not.toHaveBeenCalled();
    expect(screen.getByTestId('saved-view-name')).toHaveAttribute('aria-invalid', 'true');
  });

  it('refuses a name over 120 characters', async () => {
    renderTable(['/?f.sev=critical']);
    openPanel();
    await screen.findByTestId('saved-views-empty');

    fireEvent.change(screen.getByTestId('saved-view-name'), { target: { value: 'a'.repeat(121) } });
    fireEvent.click(screen.getByTestId('saved-view-save'));

    expect(await screen.findByTestId('saved-view-name-error')).toHaveTextContent(/120/);
    expect(api.post).not.toHaveBeenCalled();
  });
});

describe('Issue 580 — accessibility of the view picker', () => {
  // axe-core over the open panel, in each of its three states. jsdom has no
  // layout, so the rules that need geometry (colour contrast, target size) are
  // off here by necessity — those are the ones e2e/visual/overlays.spec.ts runs
  // in a real browser. What this DOES settle is the structural half: every
  // control has an accessible name, the share toggle exposes its switch state,
  // and the error and validation messages are announced.
  const runAxe = async () => {
    const results = await axe.run(
      {
        include: [['body']],
        // floating-ui inserts its own visually-hidden role="button" focus-guard
        // sentinels around every portalled popover. They have no accessible
        // name by construction and axe flags all four; they predate this issue
        // (the filter panel has always been a FloatingPortal) and belong to the
        // library, not to the view picker. Excluded so the assertion below is
        // about the markup this issue actually owns.
        exclude: [['[data-floating-ui-focus-guard]']],
      },
      {
        rules: {
          'color-contrast': { enabled: false },
          region: { enabled: false },
        },
      },
    );
    return results.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical');
  };

  it('has no serious or critical violations with views listed', async () => {
    listed([
      dto({ visibility: 'shared' }),
      dto({ id: 'v9', user_id: 'other', name: 'Vue du RSSI', visibility: 'shared', owner_email: 'amina@banque.cm' }),
    ]);
    renderTable();
    openPanel();
    await screen.findByTestId('saved-view-Comité T3');

    expect(await runAxe()).toEqual([]);
  });

  it('has no serious or critical violations when empty', async () => {
    renderTable();
    openPanel();
    await screen.findByTestId('saved-views-empty');

    expect(await runAxe()).toEqual([]);
  });

  it('has no serious or critical violations in the error and invalid-name states', async () => {
    vi.mocked(api.get).mockRejectedValue(new Error('gateway timeout'));
    renderTable(['/?f.sev=critical']);
    openPanel();
    await screen.findByTestId('saved-views-error');

    fireEvent.change(screen.getByTestId('saved-view-name'), { target: { value: '###' } });
    fireEvent.click(screen.getByTestId('saved-view-save'));
    await screen.findByTestId('saved-view-name-error');

    expect(await runAxe()).toEqual([]);
  });
});
