// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// Type surface of <DataTable> — the one table of the app.
//
// Design rule this file encodes: nothing in a table is decorative. A column is
// sortable only if it declares HOW to sort (a server `sortKey`, or a client
// `sortValue`); a facet filters only if it declares HOW to filter; a row action
// exists only if it has an `onSelect`. There is no way to render an affordance
// that does nothing — the types forbid it.

import type { LucideIcon } from 'lucide-react';
import type { ReactNode } from 'react';

/* ------------------------------------------------------------------ columns */

export interface Column<T> {
  /** Stable id — used by the URL (sort), by column prefs and by CSV export. */
  key: string;
  header: ReactNode;
  render: (row: T) => ReactNode;
  /**
   * Server-side sort field (`sort_by` value). Presence makes the header
   * clickable in `mode="server"`.
   */
  sortKey?: string;
  /**
   * Client-side comparable. Presence makes the header clickable in
   * `mode="client"`. Supplying neither leaves the header inert *and* unstyled,
   * so a user never clicks a header that cannot sort.
   */
  sortValue?: (row: T) => string | number;
  align?: 'left' | 'right';
  /** Sticky at horizontal scroll. Only the first frozen column is honoured. */
  frozen?: boolean;
  width?: number | string;
  /** Plain value for CSV export. Defaults to `sortValue`, then to ''. */
  exportValue?: (row: T) => string | number | null | undefined;
  /** false = always visible in the column picker (identity columns). */
  hideable?: boolean;
  /** Hidden until the user opts in from the column picker. */
  defaultHidden?: boolean;
  /** Accessible name when `header` is an icon or empty. */
  headerLabel?: string;
}

/* ------------------------------------------------------------------- facets */

export interface FacetOption {
  value: string;
  label: string;
  /** Token (e.g. `var(--critical)`) tinting the option's dot. */
  color?: string;
}

export interface Facet<T> {
  key: string;
  label: string;
  options: FacetOption[];
  /** Single-choice facet (radio semantics). Defaults to multi-select. */
  single?: boolean;
  /**
   * Client-mode predicate. Required when `mode="client"`; ignored in server
   * mode, where the page maps `state.filters` onto its query params.
   */
  matches?: (row: T, selected: string[]) => boolean;
}

/* ------------------------------------------------------------------ actions */

export interface RowAction<T> {
  key: string;
  label: string;
  icon?: LucideIcon;
  onSelect: (row: T) => void;
  danger?: boolean;
  /** Permission / state gate — a hidden action is never rendered. */
  hidden?: (row: T) => boolean;
  disabled?: (row: T) => boolean;
  /** Draws a separator above this item. */
  separatorBefore?: boolean;
}

/** What a bulk action was asked to operate on. */
export interface BulkScope<T> {
  /** 'page' = the explicitly ticked rows. 'all' = every row matching the filter. */
  scope: 'selection' | 'all-matching';
  /** Ids of the ticked rows (empty when scope is 'all-matching'). */
  ids: string[];
  /** The ticked row objects (empty when scope is 'all-matching'). */
  rows: T[];
  /** Result count the action applies to — the number shown on the bar. */
  count: number;
  /** Current table state, so an 'all-matching' action can re-derive the filter. */
  state: TableState;
}

export interface BulkAction<T> {
  key: string;
  label: string;
  icon?: LucideIcon;
  danger?: boolean;
  /**
   * The effect. Rejecting surfaces an error on the bar; resolving surfaces a
   * success tick. Both are visible states — rule (b).
   */
  run: (ctx: BulkScope<T>) => Promise<void> | void;
  /** Permission gate — a bulk action the user cannot perform is not rendered. */
  hidden?: boolean;
  /** Actions that cannot address the whole result set (e.g. a per-id API). */
  selectionOnly?: boolean;
}

/* -------------------------------------------------------------------- state */

export interface SortState {
  key: string;
  dir: 'asc' | 'desc';
}

export interface TableState {
  /** Instant search — deliberately NOT a facet (two distinct affordances). */
  q: string;
  sort: SortState | null;
  /** 1-based. */
  page: number;
  pageSize: number;
  /** facet key → selected values. Absent/empty key = facet inactive. */
  filters: Record<string, string[]>;
}

export const EMPTY_TABLE_STATE: TableState = {
  q: '',
  sort: null,
  page: 1,
  pageSize: 50,
  filters: {},
};

/**
 * A user-named filter combination, persisted per table id.
 *
 * Server-side since #580: a view lives against the tenant, not in one browser,
 * so it survives clearing site data and can be handed to the risk committee.
 */
export interface SavedView {
  id: string;
  name: string;
  state: Pick<TableState, 'q' | 'filters' | 'sort'>;
  /**
   * personal — only its owner sees it. shared — every member of the same tenant
   * sees it. Sharing never crosses a tenant, and there is deliberately nothing
   * in between: a view shared with a named subset of people is a permissions
   * design, not a table feature.
   */
  visibility: SavedViewVisibility;
  /** Whether the signed-in user owns it — only an owner (or a tenant admin) may edit it. */
  isOwn: boolean;
  /** Who shared it, when the server could resolve a name. Display only. */
  ownerEmail?: string;
}

export type SavedViewVisibility = 'personal' | 'shared';

/** Per-user column layout, persisted per table id. */
export interface ColumnPrefs {
  /** Column keys in display order. Unknown keys are dropped on read. */
  order: string[];
  /** Column keys the user hid. */
  hidden: string[];
}
