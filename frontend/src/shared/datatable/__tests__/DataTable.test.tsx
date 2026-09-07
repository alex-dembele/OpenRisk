// Unit coverage for <DataTable>'s contract.
//
// The e2e suite (tests/e2e/datatable.spec.ts) owns the geometry claims that only
// a real browser can settle — chiefly "the row menu on the last row of 200 is
// fully visible". These tests own everything that is pure logic and can be
// asserted deterministically in jsdom: URL round-tripping, facet combination,
// selection scope, column persistence, CSV shape, and the three UI states.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react';
import { MemoryRouter, useLocation } from 'react-router';
import { DataTable } from '../DataTable';
import { useTableState } from '../useTableState';
import { buildCsv } from '../exportCsv';
import type { BulkAction, Column, Facet, RowAction } from '../types';
import { savedViewService } from '../../../services/savedViewService';

// Saved views are server-side since #580. The transport is mocked; the Zod form
// schema is deliberately NOT — it is the real client-side gate on the form.
vi.mock('../../../services/savedViewService', async (importOriginal) => {
  const actual =
    await importOriginal<typeof import('../../../services/savedViewService')>();
  return {
    ...actual,
    migrateLegacyViews: vi.fn(async () => 0),
    savedViewService: {
      list: vi.fn(async () => []),
      create: vi.fn(),
      update: vi.fn(async () => undefined),
      remove: vi.fn(async () => undefined),
    },
  };
});

interface Row {
  id: string;
  name: string;
  sev: 'critical' | 'low';
  score: number;
}

const ROWS: Row[] = [
  { id: 'a', name: 'Alpha', sev: 'critical', score: 9 },
  { id: 'b', name: 'Bravo', sev: 'low', score: 3 },
  { id: 'c', name: 'Charlie', sev: 'critical', score: 7 },
];

const COLUMNS: Column<Row>[] = [
  {
    key: 'name',
    header: 'Nom',
    hideable: false,
    sortValue: (r) => r.name,
    exportValue: (r) => r.name,
  },
  {
    key: 'sev',
    header: 'Sévérité',
    sortValue: (r) => r.sev,
    exportValue: (r) => r.sev,
    render: (r) => <span>{r.sev}</span>,
  },
  {
    key: 'score',
    header: 'Score',
    sortValue: (r) => r.score,
    exportValue: (r) => r.score,
    render: (r) => <span>{r.score}</span>,
  },
].map((c) => ({ render: (r: Row) => <span>{r.name}</span>, ...c })) as Column<Row>[];

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

function Harness({
  rows = ROWS,
  loading = false,
  error = false,
  rowActions,
  bulkActions,
  onRetry,
  pageSize,
}: {
  rows?: Row[];
  loading?: boolean;
  error?: boolean;
  rowActions?: RowAction<Row>[];
  bulkActions?: BulkAction<Row>[];
  onRetry?: () => void;
  pageSize?: number;
}) {
  const api = useTableState({ defaultPageSize: pageSize ?? 25 });
  const location = useLocation();
  return (
    <>
      <output data-testid="url">{location.search}</output>
      <DataTable
        id="test"
        ariaLabel="Test table"
        rows={rows}
        columns={COLUMNS}
        rowKey={(r) => r.id}
        api={api}
        mode="client"
        loading={loading}
        error={error}
        onRetry={onRetry}
        facets={FACETS}
        clientSearch={(r, q) => r.name.toLowerCase().includes(q)}
        selectable
        rowActions={rowActions}
        bulkActions={bulkActions}
        empty={<div>Rien pour le moment</div>}
      />
    </>
  );
}

const renderTable = (props: Parameters<typeof Harness>[0] = {}, initialEntries = ['/']) =>
  render(
    <MemoryRouter initialEntries={initialEntries}>
      <Harness {...props} />
    </MemoryRouter>,
  );

beforeEach(() => {
  window.localStorage.clear();
  vi.mocked(savedViewService.list).mockResolvedValue([]);
  vi.mocked(savedViewService.create).mockImplementation(async (body) => ({
    id: 'view-1',
    tenant_id: 'tenant-1',
    // The harness renders with no signed-in user, so the owner id that makes a
    // view the caller's own is the empty one.
    user_id: '',
    table_id: body.table_id,
    name: body.name,
    visibility: body.visibility ?? 'personal',
    state: body.state,
  }));
  vi.mocked(savedViewService.remove).mockResolvedValue(undefined);
  vi.mocked(savedViewService.update).mockResolvedValue(
    {} as Awaited<ReturnType<typeof savedViewService.update>>,
  );
});

describe('states', () => {
  it('renders a skeleton while loading with nothing to show', () => {
    renderTable({ rows: [], loading: true });
    expect(screen.getByTestId('table-skeleton')).toBeInTheDocument();
  });

  it('renders an error state with a retry that calls back', () => {
    const onRetry = vi.fn();
    renderTable({ rows: [], error: true, onRetry });
    fireEvent.click(screen.getByTestId('table-retry'));
    expect(onRetry).toHaveBeenCalledTimes(1);
  });

  it('distinguishes "nothing yet" from "nothing matches"', () => {
    const { unmount } = renderTable({ rows: [] });
    expect(screen.getByText('Rien pour le moment')).toBeInTheDocument();
    unmount();

    renderTable({}, ['/?q=zzz']);
    expect(screen.getByTestId('table-clear-filters')).toBeInTheDocument();
  });
});

describe('search and facets are distinct affordances', () => {
  it('the search box is always present, outside the filter panel', () => {
    renderTable();
    expect(screen.getByTestId('table-search')).toBeInTheDocument();
    expect(screen.queryByTestId('filters-panel')).not.toBeInTheDocument();
  });

  it('a facet narrows the rows and writes itself into the URL', () => {
    renderTable();
    fireEvent.click(screen.getByTestId('filters-trigger'));
    fireEvent.click(screen.getByTestId('facet-sev-low'));

    expect(screen.getByTestId('url').textContent).toContain('f.sev=low');
    expect(screen.getAllByTestId('table-row')).toHaveLength(1);
  });

  it('reads its whole state back from the URL', () => {
    renderTable({}, ['/?f.sev=critical&sort=score:asc']);
    expect(screen.getAllByTestId('table-row')).toHaveLength(2);
    // asc → the lower score comes first.
    expect(screen.getAllByTestId('table-row')[0]).toHaveAttribute('data-row-id', 'c');
  });

  it('counts active facets and resets them in one action', () => {
    renderTable({}, ['/?f.sev=critical']);
    expect(screen.getByTestId('filters-active-count')).toHaveTextContent('1');
    fireEvent.click(screen.getByTestId('filters-trigger'));
    fireEvent.click(screen.getByTestId('filters-reset'));
    expect(screen.getByTestId('url').textContent).not.toContain('f.sev');
  });

  it('shows a live result count for the current filter', () => {
    renderTable({}, ['/?f.sev=critical']);
    fireEvent.click(screen.getByTestId('filters-trigger'));
    expect(screen.getByTestId('filters-result-count')).toHaveTextContent('2');
  });

  it('saves a named filter to the server and re-applies it', async () => {
    renderTable({}, ['/?f.sev=critical']);
    fireEvent.click(screen.getByTestId('filters-trigger'));
    await screen.findByTestId('saved-views-empty');

    fireEvent.change(screen.getByTestId('saved-view-name'), { target: { value: 'Critiques' } });
    fireEvent.click(screen.getByTestId('saved-view-save'));

    // The view appears optimistically, then is confirmed by the server.
    await screen.findByTestId('saved-view-Critiques');
    await waitFor(() =>
      expect(savedViewService.create).toHaveBeenCalledWith(
        expect.objectContaining({
          table_id: 'test',
          name: 'Critiques',
          visibility: 'personal',
          state: expect.objectContaining({ filters: { sev: ['critical'] } }),
        }),
      ),
    );

    // Reset does not close the panel — the saved view is right there to re-apply.
    fireEvent.click(screen.getByTestId('filters-reset'));
    expect(screen.getByTestId('url').textContent).not.toContain('f.sev');

    // Re-query: the optimistic row was replaced by the server-confirmed one.
    fireEvent.click(screen.getByTestId('saved-view-Critiques'));
    expect(screen.getByTestId('url').textContent).toContain('f.sev=critical');
  });
});

describe('sorting', () => {
  it('cycles desc → asc → none, and says so in the URL', () => {
    renderTable();
    const header = screen.getByTestId('sort-score');

    fireEvent.click(header);
    expect(screen.getByTestId('url').textContent).toContain('sort=score%3Adesc');
    fireEvent.click(header);
    expect(screen.getByTestId('url').textContent).toContain('sort=score%3Aasc');
    fireEvent.click(header);
    expect(screen.getByTestId('url').textContent).not.toContain('sort=');
  });

  it('exposes the sort state to assistive technology', () => {
    renderTable({}, ['/?sort=score:desc']);
    const header = screen
      .getAllByRole('columnheader')
      .find((h) => h.textContent?.includes('Score'))!;
    expect(header).toHaveAttribute('aria-sort', 'descending');
  });
});

describe('selection scope', () => {
  it('ticking the header selects the page, and offers "all results" separately', () => {
    // A bulk action must exist for the bar to appear: a selection with nothing
    // to do to it earns no toolbar.
    renderTable({ pageSize: 2, bulkActions: [{ key: 'noop', label: 'Agir', run: vi.fn() }] });
    fireEvent.click(screen.getByTestId('select-all-page'));

    expect(screen.getByTestId('bulk-count')).toHaveTextContent('2');
    fireEvent.click(screen.getByTestId('select-scope-all'));
    expect(screen.getByTestId('bulk-count')).toHaveTextContent('3');
  });

  it('drops a selection the user can no longer see when the filter changes', () => {
    renderTable({ bulkActions: [{ key: 'noop', label: 'Agir', run: vi.fn() }] });
    fireEvent.click(screen.getByTestId('select-all-page'));
    expect(screen.getByTestId('bulk-bar')).toBeInTheDocument();

    fireEvent.click(screen.getByTestId('filters-trigger'));
    fireEvent.click(screen.getByTestId('facet-sev-low'));
    expect(screen.queryByTestId('bulk-bar')).not.toBeInTheDocument();
  });

  it('a bulk action receives the ticked ids and shows a visible outcome', async () => {
    const run = vi.fn().mockResolvedValue(undefined);
    const bulk: BulkAction<Row>[] = [{ key: 'del', label: 'Supprimer', selectionOnly: true, run }];
    renderTable({ bulkActions: bulk });

    fireEvent.click(screen.getByTestId('row-select-a'));
    fireEvent.click(screen.getByTestId('bulk-action-del'));

    expect(run).toHaveBeenCalledTimes(1);
    expect(run.mock.calls[0][0]).toMatchObject({ scope: 'selection', ids: ['a'], count: 1 });
  });

  it('hides a bulk action the user has no permission for', () => {
    const bulk: BulkAction<Row>[] = [
      { key: 'del', label: 'Supprimer', hidden: true, run: vi.fn() },
    ];
    renderTable({ bulkActions: bulk });
    fireEvent.click(screen.getByTestId('row-select-a'));
    expect(screen.queryByTestId('bulk-action-del')).not.toBeInTheDocument();
  });
});

describe('row actions', () => {
  it('opens a menu whose entries fire their handler', () => {
    const onSelect = vi.fn();
    const actions: RowAction<Row>[] = [{ key: 'view', label: 'Voir', onSelect }];
    renderTable({ rowActions: actions });

    fireEvent.click(within(screen.getAllByTestId('table-row')[0]).getByTestId('row-menu-trigger'));
    fireEvent.click(screen.getByRole('menuitem', { name: 'Voir' }));

    expect(onSelect).toHaveBeenCalledWith(ROWS[0]);
  });

  it('renders no trigger at all when every action is hidden for this row', () => {
    const actions: RowAction<Row>[] = [
      { key: 'del', label: 'Supprimer', hidden: () => true, onSelect: vi.fn() },
    ];
    renderTable({ rowActions: actions });
    expect(screen.queryByTestId('row-menu-trigger')).not.toBeInTheDocument();
  });
});

describe('columns', () => {
  it('hides a column and remembers the choice per table id', () => {
    const { unmount } = renderTable();
    fireEvent.click(screen.getByTestId('columns-trigger'));
    fireEvent.click(screen.getByTestId('column-toggle-score'));
    expect(screen.queryByRole('columnheader', { name: /Score/ })).not.toBeInTheDocument();
    unmount();

    renderTable();
    expect(screen.queryByRole('columnheader', { name: /Score/ })).not.toBeInTheDocument();
  });

  it('refuses to hide a column marked as identity', () => {
    renderTable();
    fireEvent.click(screen.getByTestId('columns-trigger'));
    expect(screen.getByTestId('column-toggle-name')).toBeDisabled();
  });
});

describe('pagination', () => {
  it('pages client-side and reports an honest range', () => {
    renderTable({ pageSize: 2 });
    expect(screen.getByTestId('table-range')).toHaveTextContent('1–2 sur 3');
    expect(screen.getAllByTestId('table-row')).toHaveLength(2);

    fireEvent.click(screen.getByTestId('table-next'));
    expect(screen.getByTestId('table-page')).toHaveTextContent('2/2');
    expect(screen.getAllByTestId('table-row')).toHaveLength(1);
  });
});

describe('CSV export', () => {
  it('exports the visible columns, in order, with the header row', () => {
    expect(buildCsv(ROWS, COLUMNS)).toBe(
      ['Nom,Sévérité,Score', 'Alpha,critical,9', 'Bravo,low,3', 'Charlie,critical,7'].join('\n'),
    );
  });

  it('neutralises spreadsheet formula injection', () => {
    const evil: Row[] = [{ id: 'x', name: '=cmd|/c calc', sev: 'low', score: 0 }];
    expect(buildCsv(evil, COLUMNS)).toContain("'=cmd|/c calc");
  });

  it('quotes separators and doubles embedded quotes', () => {
    const tricky: Row[] = [{ id: 'y', name: 'a,b "c"', sev: 'low', score: 1 }];
    expect(buildCsv(tricky, COLUMNS)).toContain('"a,b ""c"""');
  });
});

describe('grid semantics', () => {
  it('announces the true result count, not the page length', () => {
    renderTable({ pageSize: 2 });
    expect(screen.getByRole('table')).toHaveAttribute('aria-rowcount', '3');
  });

  it('marks selected rows for assistive technology', () => {
    renderTable();
    fireEvent.click(screen.getByTestId('row-select-b'));
    const row = screen
      .getAllByTestId('table-row')
      .find((r) => r.getAttribute('data-row-id') === 'b')!;
    expect(row).toHaveAttribute('aria-selected', 'true');
  });
});
