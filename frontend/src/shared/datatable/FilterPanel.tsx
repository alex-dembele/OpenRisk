// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// The real "Filtres" panel.
//
// The button labelled "Filtres" used to open a single search input. Search and
// filtering are different intents and now have different affordances: the
// search box is always visible in the toolbar; this panel holds the facets —
// combinable, reflected in the URL, with a live result count, a reset, and
// named saved views.

import { useCallback, useState } from 'react';
import { AlertCircle, Check, Filter, Lock, RotateCcw, Star, Trash2, Users, X } from 'lucide-react';
import {
  FloatingFocusManager,
  FloatingPortal,
  autoUpdate,
  flip,
  offset,
  shift,
  size,
  useClick,
  useDismiss,
  useFloating,
  useInteractions,
  useRole,
} from '@floating-ui/react';
import { interpolate } from '../../hooks/useI18n';
import { savedViewFormSchema } from '../../services/savedViewService';
import type { Facet, SavedView, SavedViewVisibility, TableState } from './types';
import type { SavedViewsStatus, TableStateApi } from './useTableState';

interface FilterPanelProps<T> {
  facets: Facet<T>[];
  api: TableStateApi;
  /** Rows matching the current filter — shown live on the panel. */
  resultCount: number;
  views: SavedView[];
  /** Loading / error / ready — all three are rendered, none is swallowed. */
  viewsStatus: SavedViewsStatus;
  /** A save, share or delete that did not stick. Shown, never silent. */
  viewsMutationError: boolean;
  /** Whether the signed-in user is a tenant admin (may edit a shared view they do not own). */
  canEditOthersViews: boolean;
  onSaveView: (
    name: string,
    state: SavedView['state'],
    visibility: SavedViewVisibility,
  ) => void;
  onDeleteView: (id: string) => void;
  onSetViewVisibility: (id: string, visibility: SavedViewVisibility) => void;
  onRetryViews: () => void;
  labels: FilterLabels;
}

/** The saved-view strings, from /src/locales (savedViews.*). */
export interface SavedViewLabels {
  error: string;
  retry: string;
  empty: string;
  emptyHint: string;
  share: string;
  sharedBadge: string;
  sharedBy: string;
  makeShared: string;
  makePersonal: string;
  mutationError: string;
  nameRequired: string;
  nameTooLong: string;
  nameUnreadable: string;
}

/**
 * Map a Zod issue code to the localised sentence. The schema carries stable
 * keys rather than prose precisely so the copy lives in the locale files.
 */
function nameErrorMessage(code: string, labels: FilterLabels): string {
  if (code === 'tooLong') return labels.saved.nameTooLong;
  if (code === 'unreadable') return labels.saved.nameUnreadable;
  return labels.saved.nameRequired;
}

export interface FilterLabels {
  filters: string;
  results: (n: number) => string;
  reset: string;
  savedViews: string;
  saveCurrent: string;
  viewNamePlaceholder: string;
  save: string;
  close: string;
  apply: string;
  delete: string;
  saved: SavedViewLabels;
}

export function FilterPanel<T>({
  facets,
  api,
  resultCount,
  views,
  viewsStatus,
  viewsMutationError,
  canEditOthersViews,
  onSaveView,
  onDeleteView,
  onSetViewVisibility,
  onRetryViews,
  labels,
}: FilterPanelProps<T>) {
  const [open, setOpen] = useState(false);
  const [viewName, setViewName] = useState('');
  const [shareNewView, setShareNewView] = useState(false);
  const [nameError, setNameError] = useState<string | null>(null);

  const {
    refs: anchor,
    floatingStyles,
    context,
  } = useFloating({
    open,
    onOpenChange: setOpen,
    placement: 'bottom-end',
    whileElementsMounted: autoUpdate,
    middleware: [
      offset(6),
      flip({ padding: 8 }),
      shift({ padding: 8 }),
      size({
        padding: 8,
        apply({ availableHeight, elements }) {
          Object.assign(elements.floating.style, {
            maxHeight: `${Math.max(220, availableHeight)}px`,
          });
        },
      }),
    ],
  });
  const { getReferenceProps, getFloatingProps } = useInteractions([
    useClick(context),
    useDismiss(context, { outsidePress: true, escapeKey: true }),
    useRole(context, { role: 'dialog' }),
  ]);

  const active = api.activeFilterCount;
  const canSaveView = active > 0 || api.state.q.length > 0;

  // floating-ui hands back *callback ref setters*, not ref objects. Wrapping
  // them keeps the JSX free of member access, which the react-hooks/refs rule
  // (rightly) forbids for real refs.
  const setAnchor = useCallback((node: HTMLElement | null) => anchor.setReference(node), [anchor]);
  const setPopover = useCallback((node: HTMLElement | null) => anchor.setFloating(node), [anchor]);

  return (
    <>
      <button
        ref={setAnchor}
        {...getReferenceProps()}
        type="button"
        data-testid="filters-trigger"
        aria-expanded={open}
        className="h-9 px-3.5 rounded-[10px] text-[13px] font-semibold inline-flex items-center gap-[7px] transition-all hover:bg-hover shrink-0"
        style={{
          border: `1px solid ${active ? 'var(--accent)' : 'var(--border-strong)'}`,
          background: active ? 'var(--accent-soft)' : 'var(--bg-elevated)',
          color: active ? 'var(--accent)' : 'var(--fg-primary)',
        }}
      >
        <Filter size={16} strokeWidth={1.8} />
        {labels.filters}
        {active > 0 && (
          <span
            data-testid="filters-active-count"
            className="mono text-[11px] font-bold px-1.5 rounded-full"
            style={{ background: 'var(--accent)', color: 'var(--fg-inverse)' }}
          >
            {active}
          </span>
        )}
      </button>

      {open && (
        <FloatingPortal>
          <FloatingFocusManager context={context} modal={false}>
            <div
              ref={setPopover}
              style={{
                ...floatingStyles,
                zIndex: 120,
                width: 320,
                overflowY: 'auto',
                background: 'var(--bg-elevated)',
                border: '1px solid var(--border)',
                borderRadius: 14,
                boxShadow: 'var(--shadow-lg)',
                animation: 'or-scalein .14s cubic-bezier(.2,.8,.2,1)',
              }}
              data-testid="filters-panel"
              aria-label={labels.filters}
              {...getFloatingProps()}
            >
              <div
                className="flex items-center justify-between px-4 py-3 sticky top-0"
                style={{
                  borderBottom: '1px solid var(--border)',
                  background: 'var(--bg-elevated)',
                }}
              >
                <span className="text-[13.5px] font-semibold text-ink">{labels.filters}</span>
                <button
                  type="button"
                  onClick={() => setOpen(false)}
                  aria-label={labels.close}
                  className="text-ink-muted hover:text-ink"
                >
                  <X size={16} />
                </button>
              </div>

              <div className="px-4 py-3 space-y-4">
                {facets.map((facet) => {
                  const selected = api.state.filters[facet.key] ?? [];
                  return (
                    <fieldset key={facet.key} data-testid={`facet-${facet.key}`}>
                      <legend className="text-[11px] font-semibold uppercase tracking-[.04em] text-ink-muted mb-1.5">
                        {facet.label}
                      </legend>
                      <div className="flex flex-wrap gap-1.5">
                        {facet.options.map((opt) => {
                          const on = selected.includes(opt.value);
                          return (
                            <button
                              key={opt.value}
                              type="button"
                              role="checkbox"
                              aria-checked={on}
                              data-testid={`facet-${facet.key}-${opt.value}`}
                              onClick={() => api.toggleFilter(facet.key, opt.value, facet.single)}
                              className="h-[28px] px-2.5 rounded-full text-[12px] font-semibold inline-flex items-center gap-1.5 transition-colors"
                              style={{
                                border: `1px solid ${on ? 'transparent' : 'var(--border)'}`,
                                background: on
                                  ? `color-mix(in srgb, ${opt.color ?? 'var(--accent)'} 16%, transparent)`
                                  : 'transparent',
                                color: on ? (opt.color ?? 'var(--accent)') : 'var(--fg-secondary)',
                              }}
                            >
                              {on && <Check size={12} />}
                              {opt.label}
                            </button>
                          );
                        })}
                      </div>
                    </fieldset>
                  );
                })}
              </div>

              {/* Saved views — a named filter combination the user can come back to,
                  and (since #580) hand to their whole organisation. All three UI
                  states are rendered: loading, error, empty. */}
              <div className="px-4 py-3" style={{ borderTop: '1px solid var(--border)' }}>
                <div className="text-[11px] font-semibold uppercase tracking-[.04em] text-ink-muted mb-2 flex items-center gap-1.5">
                  <Star size={12} /> {labels.savedViews}
                </div>

                {/* Loading — a skeleton, never a spinner. */}
                {viewsStatus === 'loading' && (
                  <div className="space-y-1 mb-2" data-testid="saved-views-loading" aria-hidden>
                    {[0, 1].map((i) => (
                      <div
                        key={i}
                        className="h-8 rounded-[8px] or-skeleton"
                        style={{ background: 'var(--bg-secondary)' }}
                      />
                    ))}
                  </div>
                )}

                {/* Error — the register itself is already on screen with its
                    default filters (criterion 7); only this strip failed, and it
                    offers a way back rather than a dead end. */}
                {viewsStatus === 'error' && (
                  <div
                    className="mb-2 flex items-start gap-2"
                    role="status"
                    data-testid="saved-views-error"
                  >
                    <AlertCircle size={14} style={{ color: 'var(--danger)' }} className="mt-[2px]" />
                    <div className="flex-1">
                      <p className="text-[11.5px] text-ink-muted">{labels.saved.error}</p>
                      <button
                        type="button"
                        onClick={onRetryViews}
                        data-testid="saved-views-retry"
                        className="mt-1 text-[11.5px] font-semibold hover:underline"
                        style={{ color: 'var(--accent)' }}
                      >
                        {labels.saved.retry}
                      </button>
                    </div>
                  </div>
                )}

                {/* Empty — reads as "none yet", not as a failure (criterion 8). */}
                {viewsStatus === 'ready' && views.length === 0 && (
                  <div className="mb-2" data-testid="saved-views-empty">
                    <p className="text-[12px] text-ink">{labels.saved.empty}</p>
                    <p className="text-[11px] text-ink-muted mt-0.5">{labels.saved.emptyHint}</p>
                  </div>
                )}

                {viewsStatus === 'ready' && views.length > 0 && (
                  <ul className="space-y-1 mb-2" data-testid="saved-views-list">
                    {views.map((v) => {
                      const editable = v.isOwn || canEditOthersViews;
                      return (
                        <li key={v.id} className="flex items-center gap-1">
                          <button
                            type="button"
                            data-testid={`saved-view-${v.name}`}
                            onClick={() => {
                              api.apply(v.state);
                              setOpen(false);
                            }}
                            className="flex-1 min-w-0 text-left h-8 px-2.5 rounded-[8px] text-[12.5px] font-medium text-ink hover:bg-hover transition-colors"
                          >
                            <span className="block truncate">{v.name}</span>
                            {!v.isOwn && v.ownerEmail && (
                              <span className="block truncate text-[10.5px] text-ink-muted">
                                {interpolate(labels.saved.sharedBy, { email: v.ownerEmail })}
                              </span>
                            )}
                          </button>

                          {/* Sharing is a toggle on the row, not a hidden setting:
                              whether the committee can see this view is the whole
                              point of saving it server-side. */}
                          {editable ? (
                            <button
                              type="button"
                              role="switch"
                              aria-checked={v.visibility === 'shared'}
                              aria-label={
                                v.visibility === 'shared'
                                  ? labels.saved.makePersonal
                                  : labels.saved.makeShared
                              }
                              title={
                                v.visibility === 'shared'
                                  ? labels.saved.makePersonal
                                  : labels.saved.makeShared
                              }
                              data-testid={`saved-view-share-${v.name}`}
                              onClick={() =>
                                onSetViewVisibility(
                                  v.id,
                                  v.visibility === 'shared' ? 'personal' : 'shared',
                                )
                              }
                              className="w-8 h-8 rounded-[8px] inline-flex items-center justify-center hover:bg-hover"
                              style={{
                                color:
                                  v.visibility === 'shared' ? 'var(--accent)' : 'var(--fg-muted)',
                              }}
                            >
                              {v.visibility === 'shared' ? <Users size={14} /> : <Lock size={14} />}
                            </button>
                          ) : (
                            <span
                              className="text-[10px] font-semibold px-1.5 py-0.5 rounded-full shrink-0"
                              style={{ background: 'var(--accent-soft)', color: 'var(--accent-500)' }}
                            >
                              {labels.saved.sharedBadge}
                            </span>
                          )}

                          {editable && (
                            <button
                              type="button"
                              onClick={() => onDeleteView(v.id)}
                              aria-label={`${labels.delete} ${v.name}`}
                              className="w-8 h-8 rounded-[8px] inline-flex items-center justify-center text-ink-muted hover:bg-hover"
                            >
                              <Trash2 size={14} />
                            </button>
                          )}
                        </li>
                      );
                    })}
                  </ul>
                )}

                <form
                  className="space-y-1.5"
                  onSubmit={(e) => {
                    e.preventDefault();
                    if (!canSaveView) return;
                    // Zod is the only gate on this form — the button's disabled
                    // state is an affordance, not a validator.
                    const parsed = savedViewFormSchema.safeParse({
                      name: viewName,
                      visibility: shareNewView ? 'shared' : 'personal',
                    });
                    if (!parsed.success) {
                      setNameError(parsed.error.issues[0]?.message ?? 'required');
                      return;
                    }
                    setNameError(null);
                    onSaveView(
                      parsed.data.name,
                      { q: api.state.q, filters: api.state.filters, sort: api.state.sort },
                      parsed.data.visibility,
                    );
                    setViewName('');
                    setShareNewView(false);
                  }}
                >
                  <div className="flex gap-1.5">
                    <input
                      value={viewName}
                      onChange={(e) => {
                        setViewName(e.target.value);
                        if (nameError) setNameError(null);
                      }}
                      placeholder={labels.viewNamePlaceholder}
                      aria-label={labels.viewNamePlaceholder}
                      aria-invalid={nameError ? true : undefined}
                      aria-describedby={nameError ? 'saved-view-name-error' : undefined}
                      data-testid="saved-view-name"
                      className="flex-1 h-8 px-2.5 rounded-[8px] text-[12.5px] text-ink outline-none"
                      style={{
                        background: 'var(--bg-secondary)',
                        border: `1px solid ${nameError ? 'var(--danger)' : 'var(--border)'}`,
                      }}
                    />
                    <button
                      type="submit"
                      disabled={!canSaveView || !viewName.trim()}
                      data-testid="saved-view-save"
                      className="h-8 px-2.5 rounded-[8px] text-[12px] font-semibold disabled:opacity-40"
                      style={{ background: 'var(--accent-soft)', color: 'var(--accent-500)' }}
                    >
                      {labels.save}
                    </button>
                  </div>

                  {nameError && (
                    <p
                      id="saved-view-name-error"
                      role="alert"
                      className="text-[11px]"
                      style={{ color: 'var(--danger)' }}
                      data-testid="saved-view-name-error"
                    >
                      {nameErrorMessage(nameError, labels)}
                    </p>
                  )}

                  <label className="flex items-center gap-1.5 text-[11.5px] text-ink-muted cursor-pointer">
                    <input
                      type="checkbox"
                      checked={shareNewView}
                      onChange={(e) => setShareNewView(e.target.checked)}
                      data-testid="saved-view-share-new"
                    />
                    {labels.saved.share}
                  </label>
                </form>

                {viewsMutationError && (
                  <p
                    role="alert"
                    data-testid="saved-views-mutation-error"
                    className="text-[11px] mt-1.5"
                    style={{ color: 'var(--danger)' }}
                  >
                    {labels.saved.mutationError}
                  </p>
                )}

                {!canSaveView && (
                  <p className="text-[11px] text-ink-muted mt-1.5">{labels.saveCurrent}</p>
                )}
              </div>

              <div
                className="flex items-center justify-between px-4 py-3 sticky bottom-0"
                style={{ borderTop: '1px solid var(--border)', background: 'var(--bg-elevated)' }}
              >
                <span
                  className="text-[12.5px] font-semibold text-ink"
                  data-testid="filters-result-count"
                >
                  {labels.results(resultCount)}
                </span>
                <button
                  type="button"
                  onClick={() => api.clearFilters()}
                  disabled={!canSaveView}
                  data-testid="filters-reset"
                  className="h-8 px-2.5 rounded-[8px] text-[12.5px] font-semibold inline-flex items-center gap-1.5 disabled:opacity-40 hover:bg-hover"
                  style={{ color: 'var(--fg-secondary)' }}
                >
                  <RotateCcw size={13} /> {labels.reset}
                </button>
              </div>
            </div>
          </FloatingFocusManager>
        </FloatingPortal>
      )}
    </>
  );
}

/** Removable chips for the active facets, shown under the toolbar. */
export function ActiveFilterChips<T>({
  facets,
  state,
  onRemove,
  onClear,
  clearLabel,
}: {
  facets: Facet<T>[];
  state: TableState;
  onRemove: (facetKey: string, value: string) => void;
  onClear: () => void;
  clearLabel: string;
}) {
  const chips = facets.flatMap((facet) =>
    (state.filters[facet.key] ?? []).map((value) => ({
      facetKey: facet.key,
      value,
      label: `${facet.label}: ${facet.options.find((o) => o.value === value)?.label ?? value}`,
      color: facet.options.find((o) => o.value === value)?.color,
    })),
  );
  if (chips.length === 0) return null;
  return (
    <div className="flex flex-wrap items-center gap-1.5 mb-3" data-testid="active-filter-chips">
      {chips.map((c) => (
        <button
          key={`${c.facetKey}:${c.value}`}
          type="button"
          onClick={() => onRemove(c.facetKey, c.value)}
          className="h-[26px] px-2.5 rounded-full text-[11.5px] font-semibold inline-flex items-center gap-1.5"
          style={{
            background: `color-mix(in srgb, ${c.color ?? 'var(--accent)'} 14%, transparent)`,
            color: c.color ?? 'var(--accent)',
          }}
        >
          {c.label} <X size={12} />
        </button>
      ))}
      <button
        type="button"
        onClick={onClear}
        className="text-[11.5px] font-semibold text-ink-muted hover:text-ink px-1.5"
      >
        {clearLabel}
      </button>
    </div>
  );
}
