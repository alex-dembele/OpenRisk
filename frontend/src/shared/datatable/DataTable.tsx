// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// <DataTable> — the one table of the app.
//
// Every register in OpenRisk (risks, vulnerabilities, assets, mitigations,
// incidents, the audit trail, API tokens) is the same object: a filtered,
// sorted, paginated, selectable list of rows with per-row actions. They used to
// be seven bespoke tables with seven different bugs. This is the one
// implementation, and the contract it enforces is:
//
//   (a) every affordance has a handler — the types make a decorative header,
//       facet or row action unrepresentable (see types.ts);
//   (b) every affordance has a visible loading / success / error state;
//   (c) every affordance is covered by an e2e test asserting its observable
//       effect (tests/e2e/datatable.spec.ts).
//
// Capabilities: virtualised body (@tanstack/react-virtual — smooth at 10k rows),
// server-side sort + pagination, faceted filters mirrored in the URL with saved
// views, instant search kept separate from the filters, page-vs-all-results
// selection, a permission-aware floating bulk bar, a portalled row menu that
// cannot be clipped, per-user column layout, CSV export of the selection or of
// the current view, skeleton / empty / error states, full keyboard navigation
// and grid ARIA semantics.
//
// Two data modes:
//   mode="server"  rows are the current page; `total` comes from the API; sort
//                  and paging round-trip (risks, vulnerabilities, audit trail).
//   mode="client"  rows are the whole collection and the table filters, sorts
//                  and pages them locally. Used only for endpoints that return
//                  an unpaginated array (assets, mitigations, incidents, API
//                  tokens) — see docs/ui/dead-controls.md § "Server vs client".

import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { useVirtualizer } from '@tanstack/react-virtual';
import {
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  ChevronUp,
  Download,
  Loader2,
  Search,
  X,
} from 'lucide-react';
import { useI18n } from '../../hooks/useI18n';
import { useUIStore } from '../../store/uiStore';
import { EmptyState } from '../EmptyState';
import { SkeletonRows } from '../ui';
import { BulkBar } from './BulkBar';
import { useDensityRowHeight } from './useDensityRowHeight';
import { ColumnsMenu } from './ColumnsMenu';
import { ActiveFilterChips, FilterPanel } from './FilterPanel';
import { RowMenu } from './RowMenu';
import { exportRowsToCsv } from './exportCsv';
import type { BulkAction, BulkScope, Column, Facet, RowAction } from './types';
import { useColumnPrefs, useSavedViews, type TableStateApi } from './useTableState';

export interface DataTableProps<T> {
  /** Stable id — namespaces the per-user column layout and saved views. */
  id: string;
  /** Server mode: the current page. Client mode: the whole collection. */
  rows: T[];
  columns: Column<T>[];
  rowKey: (row: T) => string;
  /** URL-backed state, from `useTableState()`. */
  api: TableStateApi;
  mode?: 'server' | 'client';
  /** Server mode: total rows matching the filter (drives the pager). */
  total?: number;
  loading?: boolean;
  error?: boolean;
  onRetry?: () => void;
  /** Shown when the tenant has no rows at all (first-use). */
  empty?: ReactNode;
  facets?: Facet<T>[];
  /** Client mode: how a row is matched against the instant-search box. */
  clientSearch?: (row: T, q: string) => boolean;
  searchPlaceholder?: string;
  selectable?: boolean;
  rowActions?: RowAction<T>[];
  bulkActions?: BulkAction<T>[];
  onRowClick?: (row: T) => void;
  /** Base name of the exported file (no extension). '' disables the export. */
  exportFilename?: string;
  /** Server mode: export every matching row (not just the loaded page). */
  onExportAllMatching?: () => Promise<void> | void;
  /** Extra toolbar controls, rendered before the filter button. */
  toolbarExtra?: ReactNode;
  minWidth?: number;
  /**
   * Row height for the virtualiser, in px. Omit to follow the density token
   * (--den-row: 40 comfort / 32 compact / 48 spacious), which is what almost
   * every table should do. Pass a number only for a table whose rows are not on
   * the ramp — two-line cells, thumbnails.
   */
  estimateRowHeight?: number;
  /** Height of the scrolling body. */
  maxBodyHeight?: number;
  /** Rows above which the body virtualises. */
  virtualThreshold?: number;
  /** Accessible name of the grid. */
  ariaLabel: string;
  pageSizeOptions?: number[];
}

/* ------------------------------------------------------------------- labels */

function useLabels() {
  const lang = useUIStore((s) => s.lang);
  const fr = lang === 'fr';
  const { t } = useI18n();
  return useMemo(
    () => ({
      search: fr ? 'Rechercher…' : 'Search…',
      searchAria: fr ? 'Recherche instantanée' : 'Instant search',
      clearSearch: fr ? 'Effacer la recherche' : 'Clear search',
      filters: fr ? 'Filtres' : 'Filters',
      // Pluralized through the catalogue, not through `n > 1`: English takes the
      // plural at zero ("0 results"), which the old inline rule got wrong.
      results: (n: number) => t('common.results', { count: n }),
      reset: fr ? 'Réinitialiser' : 'Reset',
      savedViews: fr ? 'Filtres sauvegardés' : 'Saved filters',
      // #580's strings live in /src/locales rather than inline, so FR/EN parity
      // is checkable by the i18n steward instead of by reading JSX.
      saved: {
        error: t('savedViews.error'),
        retry: t('savedViews.retry'),
        empty: t('savedViews.empty'),
        emptyHint: t('savedViews.emptyHint'),
        share: t('savedViews.share'),
        sharedBadge: t('savedViews.sharedBadge'),
        sharedBy: t('savedViews.sharedBy'),
        makeShared: t('savedViews.makeShared'),
        makePersonal: t('savedViews.makePersonal'),
        mutationError: t('savedViews.mutationError'),
        nameRequired: t('savedViews.nameRequired'),
        nameTooLong: t('savedViews.nameTooLong'),
        nameUnreadable: t('savedViews.nameUnreadable'),
      },
      saveCurrent: fr
        ? 'Appliquez un filtre pour pouvoir le sauvegarder.'
        : 'Apply a filter to be able to save it.',
      viewNamePlaceholder: fr ? 'Nommer ce filtre…' : 'Name this filter…',
      save: fr ? 'Enregistrer' : 'Save',
      close: fr ? 'Fermer' : 'Close',
      apply: fr ? 'Appliquer' : 'Apply',
      delete: fr ? 'Supprimer' : 'Delete',
      columns: fr ? 'Colonnes' : 'Columns',
      show: fr ? 'Afficher' : 'Show',
      hide: fr ? 'Masquer' : 'Hide',
      up: fr ? 'Monter' : 'Move up',
      down: fr ? 'Descendre' : 'Move down',
      exportSelection: fr ? 'Exporter la sélection' : 'Export selection',
      exportView: fr ? 'Exporter la vue' : 'Export view',
      selectAllPage: fr ? 'Tout sélectionner sur la page' : 'Select all on this page',
      selectRow: fr ? 'Sélectionner la ligne' : 'Select row',
      pageSelected: (n: number) =>
        fr
          ? `Les ${n} lignes de cette page sont sélectionnées.`
          : `All ${n} rows on this page are selected.`,
      selectAllMatching: (n: number) =>
        fr ? `Sélectionner les ${n} résultats` : `Select all ${n} results`,
      allMatchingSelected: (n: number) =>
        fr ? `Les ${n} résultats sont sélectionnés.` : `All ${n} results are selected.`,
      clearSelection: fr ? 'Effacer la sélection' : 'Clear selection',
      selected: (n: number) => t('common.selected', { count: n }),
      failed: fr ? 'Échec' : 'Failed',
      actions: fr ? 'Actions' : 'Actions',
      rowsRange: (from: number, to: number, total: number) =>
        fr ? `${from}–${to} sur ${total}` : `${from}–${to} of ${total}`,
      prev: fr ? 'Page précédente' : 'Previous page',
      next: fr ? 'Page suivante' : 'Next page',
      perPage: fr ? 'par page' : 'per page',
      noResultsTitle: fr ? 'Aucun résultat' : 'No results',
      noResultsDesc: fr
        ? 'Aucune ligne ne correspond à ces filtres. Le registre en contient d’autres.'
        : 'No row matches these filters. The register holds others.',
      clearFilters: fr ? 'Effacer les filtres' : 'Clear filters',
      errorTitle: fr ? 'Chargement impossible' : 'Could not load',
      errorDesc: fr ? 'La requête a échoué. Réessayez.' : 'The request failed. Please retry.',
      retry: fr ? 'Réessayer' : 'Retry',
      loading: fr ? 'Chargement…' : 'Loading…',
    }),
    [fr, t],
  );
}

/* ---------------------------------------------------------------- component */

export function DataTable<T>({
  id,
  rows,
  columns,
  rowKey,
  api,
  mode = 'server',
  total,
  loading = false,
  error = false,
  onRetry,
  empty,
  facets = [],
  clientSearch,
  searchPlaceholder,
  selectable = false,
  rowActions = [],
  bulkActions = [],
  onRowClick,
  exportFilename,
  onExportAllMatching,
  toolbarExtra,
  minWidth = 820,
  estimateRowHeight,
  maxBodyHeight = 620,
  virtualThreshold = 30,
  ariaLabel,
  pageSizeOptions = [25, 50, 100],
}: DataTableProps<T>) {
  const L = useLabels();
  /* The density ramp, unless the caller opted out. See useDensityRowHeight:
     the CSS has always said `height: var(--den-row)`, but the inline row
     height below overrode it at every density. */
  const rowHeight = useDensityRowHeight(estimateRowHeight);
  const { state } = api;

  /* ------------------------------------------------------ column preferences */
  const allKeys = useMemo(() => columns.map((c) => c.key), [columns]);
  const defaultHidden = useMemo(
    () => columns.filter((c) => c.defaultHidden).map((c) => c.key),
    [columns],
  );
  const prefs = useColumnPrefs(id, allKeys, defaultHidden);
  const visibleColumns = useMemo(
    () =>
      prefs.order
        .map((key) => columns.find((c) => c.key === key))
        .filter((c): c is Column<T> => !!c && !prefs.hidden.includes(c.key)),
    [prefs.order, prefs.hidden, columns],
  );

  /* ------------------------------------------------------------- saved views */
  // Server-side since #580. Fetched independently of the rows on purpose: a
  // saved-views outage must never keep the register off the screen.
  const savedViews = useSavedViews(id);

  /* ---------------------------------------------------------- client shaping */
  // In server mode the API already did all of this; `rows` is the page.
  const filtered = useMemo(() => {
    if (mode === 'server') return rows;
    let out = rows;
    for (const facet of facets) {
      const selected = state.filters[facet.key];
      if (!selected?.length || !facet.matches) continue;
      out = out.filter((row) => facet.matches!(row, selected));
    }
    const q = state.q.trim().toLowerCase();
    if (q && clientSearch) out = out.filter((row) => clientSearch(row, q));
    return out;
  }, [mode, rows, facets, state.filters, state.q, clientSearch]);

  const sorted = useMemo(() => {
    if (mode === 'server' || !state.sort) return filtered;
    const col = columns.find((c) => c.key === state.sort!.key);
    if (!col?.sortValue) return filtered;
    const get = col.sortValue;
    const factor = state.sort.dir === 'asc' ? 1 : -1;
    return [...filtered].sort((a, b) => {
      const va = get(a);
      const vb = get(b);
      if (va < vb) return -factor;
      if (va > vb) return factor;
      return 0;
    });
  }, [mode, filtered, state.sort, columns]);

  const resultCount = mode === 'server' ? (total ?? rows.length) : sorted.length;
  const pageCount = Math.max(1, Math.ceil(resultCount / state.pageSize));
  const currentPage = Math.min(state.page, pageCount);

  const pageRows = useMemo(() => {
    if (mode === 'server') return rows;
    const from = (currentPage - 1) * state.pageSize;
    return sorted.slice(from, from + state.pageSize);
  }, [mode, rows, sorted, currentPage, state.pageSize]);

  // A client-mode filter change can leave the user past the last page.
  useEffect(() => {
    if (state.page > pageCount) api.setPage(pageCount);
  }, [state.page, pageCount, api]);

  /* ---------------------------------------------------------------- selection */
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [allMatching, setAllMatching] = useState(false);
  // Any change to what the table is showing invalidates a selection the user
  // can no longer see — silently acting on invisible rows is how bulk delete
  // becomes an incident.
  //
  // Keyed on a serialised signature, not on the state objects: `useSearchParams`
  // hands back a fresh URLSearchParams on every render, so an identity-based
  // dependency would clear the selection on each re-render and no bulk action
  // would ever have anything to act on.
  const viewSignature = useMemo(
    () => JSON.stringify([state.q, state.filters, state.sort, currentPage, state.pageSize]),
    [state.q, state.filters, state.sort, currentPage, state.pageSize],
  );
  useEffect(() => {
    setSelectedIds(new Set());
    setAllMatching(false);
  }, [viewSignature]);

  const pageIds = useMemo(() => pageRows.map(rowKey), [pageRows, rowKey]);
  const allPageSelected = pageIds.length > 0 && pageIds.every((rid) => selectedIds.has(rid));
  const somePageSelected = pageIds.some((rid) => selectedIds.has(rid));

  const toggleAllOnPage = useCallback(() => {
    setAllMatching(false);
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (pageIds.every((rid) => next.has(rid))) pageIds.forEach((rid) => next.delete(rid));
      else pageIds.forEach((rid) => next.add(rid));
      return next;
    });
  }, [pageIds]);

  const toggleRow = useCallback((rid: string) => {
    setAllMatching(false);
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(rid)) next.delete(rid);
      else next.add(rid);
      return next;
    });
  }, []);

  const clearSelection = useCallback(() => {
    setSelectedIds(new Set());
    setAllMatching(false);
  }, []);

  const selectedRows = useMemo(
    () => pageRows.filter((r) => selectedIds.has(rowKey(r))),
    [pageRows, selectedIds, rowKey],
  );
  const selectionCount = allMatching ? resultCount : selectedIds.size;

  const buildScope = useCallback(
    (action: BulkAction<T>): BulkScope<T> => {
      const useAll = allMatching && !action.selectionOnly;
      return {
        scope: useAll ? 'all-matching' : 'selection',
        ids: useAll ? [] : Array.from(selectedIds),
        rows: useAll ? [] : selectedRows,
        count: useAll ? resultCount : selectedIds.size,
        state,
      };
    },
    [allMatching, selectedIds, selectedRows, resultCount, state],
  );

  /* -------------------------------------------------------------- keyboard */
  const bodyRef = useRef<HTMLDivElement>(null);
  const rowRefs = useRef<Array<HTMLTableRowElement | null>>([]);
  const [focusIndex, setFocusIndex] = useState<number>(-1);
  const pendingFocus = useRef<number | null>(null);

  const moveFocus = useCallback(
    (next: number) => {
      const clamped = Math.max(0, Math.min(pageRows.length - 1, next));
      setFocusIndex(clamped);
      pendingFocus.current = clamped;
    },
    [pageRows.length],
  );

  /* ------------------------------------------------------------ virtualiser */
  const virtualise = pageRows.length > virtualThreshold;
  const virtualizer = useVirtualizer({
    count: pageRows.length,
    getScrollElement: () => bodyRef.current,
    estimateSize: () => rowHeight,
    overscan: 12,
    enabled: virtualise,
  });
  const virtualItems = virtualise ? virtualizer.getVirtualItems() : [];
  const paddingTop = virtualise && virtualItems.length ? virtualItems[0].start : 0;
  const paddingBottom =
    virtualise && virtualItems.length
      ? virtualizer.getTotalSize() - virtualItems[virtualItems.length - 1].end
      : 0;

  // Focus follows the keyboard even when the target row is not mounted yet.
  useEffect(() => {
    const idx = pendingFocus.current;
    if (idx == null) return;
    const node = rowRefs.current[idx];
    if (node) {
      node.focus();
      pendingFocus.current = null;
    } else if (virtualise) {
      virtualizer.scrollToIndex(idx, { align: 'auto' });
    }
  });

  const onBodyKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if (pageRows.length === 0) return;
    const from = focusIndex < 0 ? 0 : focusIndex;
    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault();
        moveFocus(focusIndex < 0 ? 0 : from + 1);
        break;
      case 'ArrowUp':
        e.preventDefault();
        moveFocus(from - 1);
        break;
      case 'Home':
        e.preventDefault();
        moveFocus(0);
        break;
      case 'End':
        e.preventDefault();
        moveFocus(pageRows.length - 1);
        break;
      case 'PageDown':
        e.preventDefault();
        moveFocus(from + 10);
        break;
      case 'PageUp':
        e.preventDefault();
        moveFocus(from - 10);
        break;
      case ' ':
      case 'Spacebar':
        if (selectable && focusIndex >= 0) {
          e.preventDefault();
          toggleRow(rowKey(pageRows[focusIndex]));
        }
        break;
      case 'Enter':
        if (onRowClick && focusIndex >= 0) {
          e.preventDefault();
          onRowClick(pageRows[focusIndex]);
        }
        break;
      case 'Escape':
        if (selectionCount > 0) {
          e.preventDefault();
          clearSelection();
        }
        break;
      default:
        break;
    }
  };

  /* ----------------------------------------------------------------- export */
  const [exporting, setExporting] = useState(false);
  const doExport = async () => {
    if (!exportFilename) return;
    setExporting(true);
    try {
      if (allMatching && onExportAllMatching) {
        await onExportAllMatching();
      } else {
        const source = selectedIds.size > 0 ? selectedRows : mode === 'client' ? sorted : pageRows;
        exportRowsToCsv(
          `${exportFilename}-${new Date().toISOString().slice(0, 10)}.csv`,
          source,
          visibleColumns,
        );
      }
    } finally {
      setExporting(false);
    }
  };

  /* ------------------------------------------------------------ instant search */
  const [searchDraft, setSearchDraft] = useState(state.q);
  const searchDraftRef = useRef(state.q);
  useEffect(() => {
    // Keep in sync when the URL changes from elsewhere (saved view, reset, back).
    if (state.q !== searchDraftRef.current) {
      searchDraftRef.current = state.q;
      setSearchDraft(state.q);
    }
  }, [state.q]);
  useEffect(() => {
    if (searchDraft === state.q) return;
    const t = window.setTimeout(() => {
      searchDraftRef.current = searchDraft;
      api.setQuery(searchDraft);
    }, 220);
    return () => window.clearTimeout(t);
  }, [searchDraft, state.q, api]);

  /* -------------------------------------------------------------- rendering */
  const sortableKey = (col: Column<T>) =>
    mode === 'server' ? col.sortKey : col.sortValue ? col.key : undefined;
  const colCount = visibleColumns.length + (selectable ? 1 : 0) + (rowActions.length ? 1 : 0);
  const firstIndexOnPage = resultCount === 0 ? 0 : (currentPage - 1) * state.pageSize + 1;
  const lastIndexOnPage = Math.min(currentPage * state.pageSize, resultCount);
  const hasQueryOrFilters = state.q.length > 0 || api.activeFilterCount > 0;

  const toolbar = (
    <div className="flex flex-wrap items-center gap-2 mb-3">
      <div className="relative flex-1 min-w-[200px]">
        <Search
          size={15}
          className="absolute left-3 top-1/2 -translate-y-1/2 text-ink-muted pointer-events-none"
        />
        <input
          value={searchDraft}
          onChange={(e) => setSearchDraft(e.target.value)}
          placeholder={searchPlaceholder ?? L.search}
          aria-label={L.searchAria}
          data-testid="table-search"
          type="search"
          className="w-full h-9 pl-9 pr-8 rounded-[10px] text-[13px] text-ink outline-none focus:ring-2 focus:ring-(--accent)/40"
          style={{ background: 'var(--bg-elevated)', border: '1px solid var(--border-strong)' }}
        />
        {searchDraft && (
          <button
            type="button"
            onClick={() => setSearchDraft('')}
            aria-label={L.clearSearch}
            className="absolute right-2 top-1/2 -translate-y-1/2 text-ink-muted hover:text-ink"
          >
            <X size={14} />
          </button>
        )}
      </div>

      {toolbarExtra}

      {facets.length > 0 && (
        <FilterPanel
          facets={facets}
          api={api}
          resultCount={resultCount}
          views={savedViews.views}
          viewsStatus={savedViews.status}
          viewsMutationError={savedViews.mutationError}
          canEditOthersViews={savedViews.canEditOthers}
          onSaveView={savedViews.save}
          onDeleteView={savedViews.remove}
          onSetViewVisibility={savedViews.setVisibility}
          onRetryViews={savedViews.retry}
          labels={L}
        />
      )}

      <ColumnsMenu
        columns={columns}
        order={prefs.order}
        hidden={prefs.hidden}
        onToggle={prefs.toggle}
        onMove={prefs.move}
        onReset={prefs.reset}
        labels={L}
      />

      {exportFilename && (
        <button
          type="button"
          onClick={doExport}
          disabled={exporting}
          data-testid="table-export"
          className="h-9 px-3.5 rounded-[10px] text-[13px] font-semibold inline-flex items-center gap-[7px] transition-all hover:bg-hover disabled:opacity-60 shrink-0"
          style={{
            border: '1px solid var(--border-strong)',
            background: 'var(--bg-elevated)',
            color: 'var(--fg-primary)',
          }}
        >
          {exporting ? (
            <Loader2 size={16} className="animate-spin" />
          ) : (
            <Download size={16} strokeWidth={1.8} />
          )}
          {selectedIds.size > 0 ? L.exportSelection : L.exportView}
        </button>
      )}
    </div>
  );

  const renderRow = (row: T, index: number) => {
    const rid = rowKey(row);
    const isSelected = allMatching || selectedIds.has(rid);
    return (
      <tr
        key={rid}
        ref={(node) => {
          rowRefs.current[index] = node;
        }}
        tabIndex={index === Math.max(0, focusIndex) ? 0 : -1}
        aria-rowindex={firstIndexOnPage + index}
        aria-selected={selectable ? isSelected : undefined}
        data-selected={isSelected || undefined}
        data-testid="table-row"
        data-row-id={rid}
        onFocus={() => setFocusIndex(index)}
        onClick={onRowClick ? () => onRowClick(row) : undefined}
        style={{
          height: rowHeight,
          cursor: onRowClick ? 'pointer' : 'default',
          outlineOffset: -2,
        }}
      >
        {selectable && (
          <td
            style={{ width: 38 }}
            onClick={(e) => {
              e.stopPropagation();
              toggleRow(rid);
            }}
          >
            <input
              type="checkbox"
              checked={isSelected}
              onChange={() => toggleRow(rid)}
              onClick={(e) => e.stopPropagation()}
              aria-label={L.selectRow}
              data-testid={`row-select-${rid}`}
              style={{ accentColor: 'var(--accent)', width: 15, height: 15 }}
            />
          </td>
        )}
        {visibleColumns.map((col) => (
          <td
            key={col.key}
            className={[col.frozen ? 'frozen' : '', col.align === 'right' ? 'num' : '']
              .join(' ')
              .trim()}
            style={{ width: col.width }}
          >
            {col.render(row)}
          </td>
        ))}
        {rowActions.length > 0 && (
          <td style={{ width: 48, textAlign: 'right' }} onClick={(e) => e.stopPropagation()}>
            <RowMenu row={row} actions={rowActions} label={L.actions} />
          </td>
        )}
      </tr>
    );
  };

  /* ------------------------------------------------------------ body states */
  let body: ReactNode;
  if (error) {
    body = (
      <EmptyState
        variant="error"
        title={L.errorTitle}
        description={L.errorDesc}
        primaryAction={
          onRetry ? (
            <button
              type="button"
              onClick={onRetry}
              data-testid="table-retry"
              className="h-9 px-3.5 rounded-[10px] text-[13px] font-semibold"
              style={{
                border: '1px solid var(--border-strong)',
                background: 'var(--bg-elevated)',
                color: 'var(--fg-primary)',
              }}
            >
              {L.retry}
            </button>
          ) : undefined
        }
      />
    );
  } else if (loading && pageRows.length === 0) {
    body = (
      <div data-testid="table-skeleton" aria-busy="true" aria-live="polite" aria-label={L.loading}>
        <SkeletonRows rows={8} />
      </div>
    );
  } else if (pageRows.length === 0) {
    body = hasQueryOrFilters ? (
      <EmptyState
        variant="no-results"
        title={L.noResultsTitle}
        description={L.noResultsDesc}
        primaryAction={
          <button
            type="button"
            onClick={() => api.clearFilters()}
            data-testid="table-clear-filters"
            className="h-9 px-3.5 rounded-[10px] text-[13px] font-semibold"
            style={{
              border: '1px solid var(--border-strong)',
              background: 'var(--bg-elevated)',
              color: 'var(--fg-primary)',
            }}
          >
            {L.clearFilters}
          </button>
        }
      />
    ) : (
      (empty ?? <EmptyState variant="first-use" title={L.noResultsTitle} />)
    );
  } else {
    body = (
      <div
        ref={bodyRef}
        onKeyDown={onBodyKeyDown}
        data-testid="table-scroll"
        style={{ maxHeight: maxBodyHeight, overflow: 'auto', position: 'relative' }}
      >
        <table
          className="or-table"
          style={{ minWidth }}
          aria-label={ariaLabel}
          aria-rowcount={resultCount}
          aria-colcount={colCount}
          aria-busy={loading || undefined}
        >
          <thead>
            <tr>
              {selectable && (
                <th style={{ width: 38 }}>
                  <input
                    type="checkbox"
                    checked={allPageSelected}
                    ref={(node) => {
                      if (node) node.indeterminate = !allPageSelected && somePageSelected;
                    }}
                    onChange={toggleAllOnPage}
                    aria-label={L.selectAllPage}
                    data-testid="select-all-page"
                    style={{ accentColor: 'var(--accent)', width: 15, height: 15 }}
                  />
                </th>
              )}
              {visibleColumns.map((col) => {
                const key = sortableKey(col);
                const active = !!key && state.sort?.key === key;
                const ariaSort = active
                  ? state.sort!.dir === 'asc'
                    ? 'ascending'
                    : 'descending'
                  : 'none';
                const name =
                  col.headerLabel ?? (typeof col.header === 'string' ? col.header : col.key);
                return (
                  <th
                    key={col.key}
                    className={[key ? 'sortable' : '', col.align === 'right' ? 'num' : '']
                      .join(' ')
                      .trim()}
                    style={{ width: col.width }}
                    aria-sort={key ? ariaSort : undefined}
                  >
                    {key ? (
                      <button
                        type="button"
                        onClick={() => api.toggleSort(key)}
                        data-testid={`sort-${col.key}`}
                        aria-label={name}
                        className="inline-flex items-center gap-1 font-semibold uppercase tracking-[.05em]"
                        style={{
                          color: active ? 'var(--fg-primary)' : 'inherit',
                          fontSize: 'inherit',
                          letterSpacing: 'inherit',
                        }}
                      >
                        {col.header}
                        {active ? (
                          state.sort!.dir === 'asc' ? (
                            <ChevronUp size={13} />
                          ) : (
                            <ChevronDown size={13} />
                          )
                        ) : null}
                      </button>
                    ) : (
                      col.header
                    )}
                  </th>
                );
              })}
              {rowActions.length > 0 && (
                <th style={{ width: 48 }}>
                  <span className="sr-only">{L.actions}</span>
                </th>
              )}
            </tr>
          </thead>
          <tbody>
            {/* Virtualised: two spacer rows keep the scrollbar honest while only
                the visible window is in the DOM. Table semantics stay intact. */}
            {paddingTop > 0 && (
              <tr aria-hidden="true" style={{ height: paddingTop }}>
                <td colSpan={colCount} style={{ padding: 0, border: 'none' }} />
              </tr>
            )}
            {virtualise
              ? virtualItems.map((v) => renderRow(pageRows[v.index], v.index))
              : pageRows.map((row, i) => renderRow(row, i))}
            {paddingBottom > 0 && (
              <tr aria-hidden="true" style={{ height: paddingBottom }}>
                <td colSpan={colCount} style={{ padding: 0, border: 'none' }} />
              </tr>
            )}
          </tbody>
        </table>
      </div>
    );
  }

  /* --------------------------------------------------------------- assembly */
  return (
    <div data-testid={`datatable-${id}`}>
      {toolbar}

      <ActiveFilterChips
        facets={facets}
        state={state}
        onRemove={(facetKey, value) => api.toggleFilter(facetKey, value)}
        onClear={() => api.clearFilters()}
        clearLabel={L.clearFilters}
      />

      {/* Page-vs-all-results escalation: never implicit. */}
      {selectable && allPageSelected && resultCount > pageIds.length && (
        <div
          data-testid="select-scope-banner"
          className="mb-2 px-3 py-2 rounded-[10px] text-[12.5px] flex flex-wrap items-center gap-2"
          style={{ background: 'var(--accent-soft)', color: 'var(--accent-500)' }}
        >
          <span>
            {allMatching ? L.allMatchingSelected(resultCount) : L.pageSelected(pageIds.length)}
          </span>
          {allMatching ? (
            <button
              type="button"
              onClick={clearSelection}
              className="font-semibold underline"
              data-testid="select-scope-clear"
            >
              {L.clearSelection}
            </button>
          ) : (
            <button
              type="button"
              onClick={() => setAllMatching(true)}
              className="font-semibold underline"
              data-testid="select-scope-all"
            >
              {L.selectAllMatching(resultCount)}
            </button>
          )}
        </div>
      )}

      <div
        className="rounded-[14px] overflow-hidden"
        style={{ border: '1px solid var(--border)', background: 'var(--bg-secondary)' }}
      >
        {body}
      </div>

      {/* Pager. Rendered whenever there is anything to page through, so the user
          is never told "50 rows" while sitting on 1 284. */}
      {pageRows.length > 0 && (
        <div className="flex flex-wrap items-center gap-3 mt-3">
          <span
            className="text-[12.5px] text-ink-soft"
            data-testid="table-range"
            aria-live="polite"
          >
            {L.rowsRange(firstIndexOnPage, lastIndexOnPage, resultCount)}
          </span>
          <div className="flex-1" />
          <label className="text-[12.5px] text-ink-muted inline-flex items-center gap-1.5">
            <select
              value={state.pageSize}
              onChange={(e) => api.setPageSize(Number(e.target.value))}
              data-testid="table-page-size"
              className="h-8 px-2 rounded-[8px] text-[12.5px] text-ink outline-none"
              style={{ background: 'var(--bg-elevated)', border: '1px solid var(--border)' }}
            >
              {pageSizeOptions.map((n) => (
                <option key={n} value={n}>
                  {n}
                </option>
              ))}
            </select>
            {L.perPage}
          </label>
          <div className="inline-flex items-center gap-1">
            <button
              type="button"
              onClick={() => api.setPage(currentPage - 1)}
              disabled={currentPage <= 1}
              aria-label={L.prev}
              data-testid="table-prev"
              className="w-8 h-8 rounded-[8px] inline-flex items-center justify-center text-ink-soft hover:bg-hover disabled:opacity-35 disabled:pointer-events-none"
              style={{ border: '1px solid var(--border)' }}
            >
              <ChevronLeft size={15} />
            </button>
            <span className="mono text-[12.5px] text-ink px-1.5" data-testid="table-page">
              {currentPage}/{pageCount}
            </span>
            <button
              type="button"
              onClick={() => api.setPage(currentPage + 1)}
              disabled={currentPage >= pageCount}
              aria-label={L.next}
              data-testid="table-next"
              className="w-8 h-8 rounded-[8px] inline-flex items-center justify-center text-ink-soft hover:bg-hover disabled:opacity-35 disabled:pointer-events-none"
              style={{ border: '1px solid var(--border)' }}
            >
              <ChevronRight size={15} />
            </button>
          </div>
        </div>
      )}

      {selectable && (
        <BulkBar
          count={selectionCount}
          actions={bulkActions}
          buildScope={buildScope}
          onClear={clearSelection}
          labels={{ selected: L.selected, clear: L.clearSelection, failed: L.failed }}
        />
      )}
    </div>
  );
}
