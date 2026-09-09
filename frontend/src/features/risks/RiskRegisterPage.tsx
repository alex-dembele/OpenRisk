// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// Risk Register (OpenRisk.dc.html §6.3) — wired to the real /risks store.
//
// The register is the app's most demanding table, so it is the reference
// migration onto <DataTable> (shared/datatable): server-side sort + pagination,
// faceted filters mirrored in the URL with saved views, instant search kept
// distinct from those filters, page-vs-all-results selection driving a
// permission-aware bulk bar, a portalled row menu that cannot be clipped, a
// per-user column layout and CSV export of the selection or of the current
// view. The right-side drawer (Details / Lifecycle / Score / Financial / …)
// is unchanged.

import { useCallback, useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router';
import { useQuery } from '@tanstack/react-query';
import { toast } from 'sonner';
import {
  Upload,
  Plus,
  X,
  FileText,
  Pencil,
  Trash2,
  Eye,
  Download,
  ShieldCheck,
  ShieldAlert,
  Clock,
  Rows3,
  LayoutGrid,
  Check,
  ChevronDown,
  Coins,
  Route as RouteIcon,
  SlidersHorizontal,
  Sparkles,
  Loader2,
  UserCheck,
  Link2Off,
  Bug,
} from 'lucide-react';
import {
  PageFrame,
  PageHeader,
  Btn,
  Card,
  CritBadge,
  StatusPill,
  Avatar,
  FwBadge,
  arcPath,
  SkeletonRows,
  EmptyState,
  ErrorState,
  softFill,
  type RiskStatus,
} from '../../shared/ui';
import {
  DataTable,
  useTableState,
  type BulkAction,
  type Column,
  type Facet,
  type RowAction,
} from '../../shared/datatable';
import { RiskMatrix, type MatrixBucket } from '../../shared/ds';
import { critColor } from '../../shared/riskColors';
import type { Criticality } from '../../shared/riskColors';
import { ImpactDialog } from '../../shared/ImpactDialog';
import { ProgressState } from '../../shared/ProgressState';
import { HistoryTimeline, type HistoryEntry } from '../../shared/HistoryTimeline';
import { useRiskTimeline } from './useRiskTimeline';
import { useUIStrings } from '../../shared/uiStrings';
import { useUIStore } from '../../store/uiStore';
import { useRiskStore, type RiskPhase } from '../../hooks/useRiskStore';
import { useFocusParam } from '../../shared/useFocusParam';
import { useAuthStore } from '../../hooks/useAuthStore';
import { isMissingRows } from '../../services/bulkService';
import { mapRisk, relTime, type UiRisk } from './riskMap';
import { EditRiskModal } from './components/EditRiskModal';
import { CreateMitigationModal } from '../mitigations/CreateMitigationModal';
import { useRiskFinancial } from '../financial/useFinancial';
import { useRiskSmartScore } from './useSmartScore';
import { SmartRiskRadar } from './components/SmartRiskRadar';
import { useTreatmentPlan } from '../ai/useAi';
import { LifecycleStepper } from './LifecycleStepper';
import {
  useRiskCategories,
  useRiskMappings,
  useCreateMapping,
  useDeleteMapping,
} from './useTaxonomy';
import { ComplianceMappingField, type MappingDraft } from './ComplianceMappingField';
import { OwnershipFields } from '../../shared/UserPicker';
import { ownershipPatch, type OwnershipRole } from '../../services/ownershipService';
import { mappingHref, mappingLabel } from '../../services/taxonomyService';
import { ctiService } from '../cti/ctiService';
import { useQueryClient } from '@tanstack/react-query';

/* -------------------------------------------------------------- CSV export */

// exportRiskCsv downloads a single risk as CSV (client-side — no per-risk export
// endpoint). Multi-row export goes through <DataTable>'s own exporter, which
// respects the user's visible columns and their order.
function exportRiskCsv(r: UiRisk) {
  const cols: [string, string | number][] = [
    ['id', r.id],
    ['name', r.name],
    ['description', r.desc ?? ''],
    ['asset', r.asset],
    ['score', r.score],
    ['criticality', r.crit],
    ['status', r.status],
    ['category', r.categoryName],
    ['framework', r.fw],
    ['tags', r.tags.join(' ')],
    ['owner', r.ownerName],
    ['probability', r.prob],
    ['impact', r.impact],
    ['asset_criticality', r.ac],
    ['updated', r.mod],
  ];
  const esc = (v: unknown) => {
    const s = v == null ? '' : String(v);
    return /[",\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s;
  };
  const csv = `${cols.map((c) => c[0]).join(',')}\n${cols.map((c) => esc(c[1])).join(',')}`;
  const url = URL.createObjectURL(new Blob([csv], { type: 'text/csv;charset=utf-8' }));
  const a = document.createElement('a');
  a.href = url;
  a.download = `risk-${r.id.slice(0, 8)}.csv`;
  a.click();
  URL.revokeObjectURL(url);
}

export function RiskRegisterPage() {
  const L = useUIStrings();
  const lang = useUIStore((s) => s.lang);
  const navigate = useNavigate();
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const risks = useRiskStore((s) => s.risks);
  const total = useRiskStore((s) => s.total);
  const isLoading = useRiskStore((s) => s.isLoading);
  const loadError = useRiskStore((s) => s.error);
  const fetchRisks = useRiskStore((s) => s.fetchRisks);
  const deleteRisk = useRiskStore((s) => s.deleteRisk);
  const bulkDelete = useRiskStore((s) => s.bulkDelete);
  const canUpdate = useAuthStore((s) => s.hasPermission('risks:update'));
  const { data: categories } = useRiskCategories();
  const canDelete = useAuthStore((s) => s.hasPermission('risks:delete'));

  const [view, setView] = useState<'table' | 'map'>('table');
  const [drawerId, setDrawerId] = useState<string | null>(null);
  const [editRaw, setEditRaw] = useState<UiRisk['raw'] | null>(null);
  const [mitiRiskId, setMitiRiskId] = useState<string | null>(null);
  // Two quick scopes that sit apart from the facet panel because they answer
  // "what should I look at right now", not "narrow this list".
  const [mine, setMine] = useState(false);
  const [unmappedOnly, setUnmappedOnly] = useState(false);
  const [toDelete, setToDelete] = useState<UiRisk | null>(null);
  const [deleting, setDeleting] = useState(false);

  // Table state lives in the URL: ?q=&sort=score:desc&page=2&f.criticality=critical
  const table = useTableState({ defaultSort: { key: 'score', dir: 'desc' }, defaultPageSize: 50 });
  const { state } = table;

  // Sort, pagination AND filtering are server-side: the register is the one
  // table that routinely holds thousands of rows, and paging 50 of them is the
  // difference between a snappy page and a 4 MB response.
  const params = useMemo(() => {
    const p: Record<string, string | number> = { page: state.page, limit: state.pageSize };
    if (state.q.trim()) p.q = state.q.trim();
    if (state.sort) {
      p.sort_by = state.sort.key;
      p.sort_dir = state.sort.dir;
    }
    for (const [key, values] of Object.entries(state.filters)) {
      if (values.length) p[key] = values.join(',');
    }
    // "Mes risques" — anything I own, execute or must validate. The server takes
    // the user from the JWT, so this is a flag, never an id: `mine=<someone
    // else>` is not expressible.
    if (mine) p.mine = 'true';
    if (unmappedOnly) p.unmapped = 'true';
    return p;
  }, [state, mine, unmappedOnly]);

  const reload = useCallback(() => {
    void fetchRisks(params).catch(() => {});
  }, [fetchRisks, params]);
  useEffect(() => {
    reload();
  }, [reload]);

  const ui: UiRisk[] = useMemo(() => risks.map((r) => mapRisk(r, lang)), [risks, lang]);
  const drawer = drawerId ? (ui.find((r) => r.id === drawerId) ?? null) : null;

  // Deep-link from universal search (/risks?focus=<id>) → open that risk's
  // drawer. `&tab=` selects which tab, which is how the legacy
  // /risks/:id/timeline URL now resolves to the real timeline (W0-05 / D8).
  const { focusId, focusTab, clearFocus } = useFocusParam();
  const [drawerTab, setDrawerTab] = useState<DrawerTab | null>(null);
  useEffect(() => {
    if (focusId && ui.some((r) => r.id === focusId)) {
      setDrawerId(focusId);
      if (focusTab && DRAWER_TABS.includes(focusTab as DrawerTab))
        setDrawerTab(focusTab as DrawerTab);
      clearFocus();
    }
  }, [focusId, focusTab, ui, clearFocus]);

  /* ------------------------------------------------------------- deletion */
  // Important + irreversible (a risk carries linked mitigation plans + history) →
  // impact radiography (UX-11), not a bare confirm.
  const confirmDeleteRisk = async () => {
    const r = toDelete;
    if (!r) return;
    setDeleting(true);
    try {
      await deleteRisk(r.id);
      toast.success(tr('Risque supprimé', 'Risk deleted'));
      if (drawerId === r.id) setDrawerId(null);
      setToDelete(null);
    } catch {
      toast.error(tr('Suppression échouée', 'Delete failed'));
    } finally {
      setDeleting(false);
    }
  };

  /* --------------------------------------------------------------- facets */
  const facets: Facet<UiRisk>[] = useMemo(
    () => [
      {
        key: 'criticality',
        label: tr('Criticité', 'Criticality'),
        options: [
          { value: 'critical', label: L.critical, color: 'var(--critical)' },
          { value: 'high', label: L.high, color: 'var(--high)' },
          { value: 'medium', label: L.medium, color: 'var(--medium)' },
          { value: 'low', label: L.low, color: 'var(--low)' },
        ],
      },
      {
        key: 'status',
        label: tr('Statut', 'Status'),
        options: [
          { value: 'open', label: tr('Ouvert', 'Open') },
          { value: 'in_progress', label: tr('En cours', 'In progress') },
          { value: 'mitigated', label: tr('Atténué', 'Mitigated') },
          { value: 'accepted', label: tr('Accepté', 'Accepted') },
        ],
      },
      {
        key: 'phase',
        label: tr('Étape du cycle de vie', 'Lifecycle step'),
        // The facet still filters the derived `phase` column, which the server
        // keeps in step with lifecycle_state on every write.
        options: [
          { value: 'identified', label: tr('Identifié', 'Identified') },
          { value: 'analyzed', label: tr('Évalué', 'Assessed') },
          { value: 'evaluated', label: tr('Traitement planifié', 'Treatment planned') },
          { value: 'treated', label: tr('En traitement', 'In treatment') },
          { value: 'monitored', label: tr('Traité / accepté', 'Mitigated / accepted') },
          { value: 'closed', label: tr('Clôturé', 'Closed') },
        ],
      },
      {
        key: 'category_id',
        label: tr('Catégorie', 'Category'),
        // The CONTROLLED vocabulary — that is what makes it a usable facet at all.
        // Faceting on free-text tags would just list every string anyone typed.
        options: (categories ?? []).map((c) => ({ value: c.id, label: c.name })),
      },
      {
        key: 'source',
        label: tr('Origine', 'Source'),
        options: [
          { value: 'manual', label: tr('Manuel', 'Manual') },
          { value: 'cti_auto', label: tr('CTI (auto)', 'CTI (auto)') },
          { value: 'scan_auto', label: tr('Scanner (auto)', 'Scanner (auto)') },
          { value: 'import', label: tr('Import', 'Import') },
        ],
      },
    ],
    [L, lang, categories],
  ); // eslint-disable-line react-hooks/exhaustive-deps

  /* -------------------------------------------------------------- columns */
  const columns: Column<UiRisk>[] = useMemo(
    () => [
      {
        key: 'name',
        header: L.col_name,
        sortKey: 'name',
        hideable: false,
        frozen: true,
        exportValue: (r) => r.name,
        render: (r) => (
          <>
            <div className="text-[13.5px] font-medium text-ink max-w-[340px] truncate">
              {r.name}
            </div>
            <div className="mono text-[11px] text-ink-muted mt-0.5">
              #{r.id.slice(0, 8)} · {r.asset}
            </div>
          </>
        ),
      },
      {
        key: 'score',
        header: L.col_score,
        sortKey: 'score',
        align: 'right',
        exportValue: (r) => r.score.toFixed(1),
        // Coloured by the SERVER's criticality, not by a client threshold on the
        // number — the two used to be computed from different cut points.
        render: (r) => (
          <span className="mono text-[15px] font-bold" style={{ color: critColor[r.crit] }}>
            {r.score.toFixed(1)}
          </span>
        ),
      },
      {
        key: 'criticality',
        header: L.col_crit,
        sortKey: 'criticality',
        exportValue: (r) => r.crit,
        render: (r) => <CritBadge crit={r.crit} />,
      },
      {
        key: 'status',
        header: L.col_status,
        sortKey: 'status',
        exportValue: (r) => r.status,
        render: (r) => (canUpdate ? <InlineStatus risk={r} /> : <StatusPill status={r.status} />),
      },
      {
        key: 'phase',
        header: tr('Phase', 'Phase'),
        defaultHidden: true,
        exportValue: (r) => r.phase,
        render: (r) => <PhasePill phase={r.phase} lang={lang} />,
      },
      // --- Taxonomy: three concepts, three columns, three renderings.
      // Conflating them is the reported bug; keeping them apart is the fix, and
      // the types make it hard to reintroduce (each reads a different field).
      {
        key: 'category',
        header: L.col_category,
        exportValue: (r) => r.categoryName,
        render: (r) =>
          r.categoryName ? (
            <span
              className="rounded-full px-2 py-0.5 text-[11px] font-semibold whitespace-nowrap"
              style={{
                background: `color-mix(in srgb, var(--${r.categoryColor}, --ink-muted) 14%, transparent)`,
                color: `var(--${r.categoryColor}, var(--ink-soft))`,
              }}
            >
              {r.categoryName}
            </span>
          ) : (
            <span className="text-ink-muted text-[12px]">—</span>
          ),
      },
      {
        key: 'framework',
        header: L.col_fw,
        exportValue: (r) => r.fw,
        // A REAL reference, or nothing. Clickable through to the control it names.
        render: (r) =>
          r.fw !== '—' ? (
            <a
              href={r.fwHref}
              onClick={(e) => {
                e.stopPropagation();
                e.preventDefault();
                navigate(r.fwHref);
              }}
              title={
                r.mappings.length > 1
                  ? tr(`+${r.mappings.length - 1} autre(s)`, `+${r.mappings.length - 1} more`)
                  : undefined
              }
              className="inline-flex items-center gap-1"
            >
              <FwBadge fw={r.fw} />
              {r.mappings.length > 1 ? (
                <span className="text-[11px] text-ink-muted">+{r.mappings.length - 1}</span>
              ) : null}
            </a>
          ) : (
            <span className="text-ink-muted text-[12px]">—</span>
          ),
      },
      {
        key: 'tags',
        header: L.col_tags,
        defaultHidden: true,
        exportValue: (r) => r.tags.join(' '),
        // Rendered as neutral chips, deliberately NOT with FwBadge: a label must
        // never be able to look like a compliance reference again.
        render: (r) =>
          r.tags.length ? (
            <span className="flex flex-wrap gap-1">
              {r.tags.slice(0, 2).map((t) => (
                <span
                  key={t}
                  className="rounded-full border border-border px-1.5 py-0.5 text-[11px] text-ink-soft"
                >
                  {t}
                </span>
              ))}
              {r.tags.length > 2 ? (
                <span className="text-[11px] text-ink-muted">+{r.tags.length - 2}</span>
              ) : null}
            </span>
          ) : (
            <span className="text-ink-muted text-[12px]">—</span>
          ),
      },
      {
        key: 'owner',
        header: L.col_owner,
        exportValue: (r) => r.ownerName,
        render: (r) =>
          r.owner !== '—' ? (
            <Avatar initials={r.owner} title={r.ownerName} />
          ) : (
            <span className="text-ink-muted text-[12px]">—</span>
          ),
      },
      {
        key: 'updated',
        header: L.col_mod,
        sortKey: 'updated_at',
        exportValue: (r) => r.raw.updated_at ?? r.raw.created_at ?? '',
        render: (r) => <span className="text-[12px] text-ink-soft whitespace-nowrap">{r.mod}</span>,
      },
    ],
    [L, lang, canUpdate, navigate],
  ); // eslint-disable-line react-hooks/exhaustive-deps

  /* ---------------------------------------------------------- row actions */
  const rowActions: RowAction<UiRisk>[] = useMemo(
    () => [
      { key: 'view', label: tr('Voir', 'View'), icon: Eye, onSelect: (r) => setDrawerId(r.id) },
      {
        key: 'edit',
        label: L.edit,
        icon: Pencil,
        hidden: () => !canUpdate,
        onSelect: (r) => setEditRaw(r.raw),
      },
      {
        key: 'mitigate',
        // Same derivation in the row menu: offering "create" on a risk that
        // already has three plans is how duplicates get made.
        label: L.createMiti,
        icon: ShieldCheck,
        hidden: () => !canUpdate,
        onSelect: (r) => {
          const n = r.raw.mitigations_count ?? r.raw.mitigations?.length ?? 0;
          if (n > 0) navigate(`/risks/mitigations?risk_id=${r.id}`);
          else setMitiRiskId(r.id);
        },
      },
      {
        key: 'export',
        label: tr('Exporter CSV', 'Export CSV'),
        icon: Download,
        onSelect: (r) => exportRiskCsv(r),
      },
      {
        key: 'delete',
        label: L.del,
        icon: Trash2,
        danger: true,
        separatorBefore: true,
        hidden: () => !canDelete,
        onSelect: (r) => setToDelete(r),
      },
    ],
    [L, canUpdate, canDelete],
  ); // eslint-disable-line react-hooks/exhaustive-deps

  /* --------------------------------------------------------- bulk actions */
  const bulkActions: BulkAction<UiRisk>[] = useMemo(
    () => [
      {
        key: 'delete',
        label: L.del,
        icon: Trash2,
        danger: true,
        hidden: !canDelete,
        // The governed endpoint takes explicit ids and caps a batch at 100, so it
        // cannot address "all N results" — the largest page is exactly 100. This
        // is a property of the API, not a UI limitation we could lift here.
        selectionOnly: true,
        // ONE request (#598). `POST /risks/bulk` is transactional and audited:
        // every named risk is deleted or none is, and each deletion is recorded
        // against the actor. The bar shows its own error state on rejection; the
        // toast is what says *why*, because under all-or-nothing the fact that
        // nothing was deleted is the part the user has to know.
        run: async ({ ids }) => {
          try {
            await bulkDelete(ids);
          } catch (err) {
            toast.error(
              isMissingRows(err)
                ? tr(
                    'Aucun risque supprimé : la sélection a changé. Rechargez et réessayez.',
                    'No risk deleted: the selection has changed. Reload and try again.',
                  )
                : tr(
                    `Aucun risque supprimé (${ids.length} sélectionné(s)).`,
                    `No risk deleted (${ids.length} selected).`,
                  ),
            );
            throw err;
          }
          toast.success(tr(`${ids.length} risque(s) supprimé(s)`, `${ids.length} risk(s) deleted`));
          reload();
        },
      },
    ],
    [L, canDelete, bulkDelete, reload],
  ); // eslint-disable-line react-hooks/exhaustive-deps

  const critCount = ui.filter((r) => r.crit === 'critical').length;

  return (
    <PageFrame wide>
      <PageHeader
        title={L.riskTitle}
        count={`${total} ${tr('risques', 'risks')}`}
        actions={
          <>
            <div
              className="inline-flex rounded-[10px] p-0.5"
              style={{ border: '1px solid var(--border-strong)', background: 'var(--bg-elevated)' }}
            >
              {(
                [
                  ['table', Rows3, tr('Table', 'Table')],
                  ['map', LayoutGrid, tr('Matrice', 'Matrix')],
                ] as const
              ).map(([v, Icon, lbl]) => (
                <button
                  key={v}
                  onClick={() => setView(v)}
                  className="h-8 px-2.5 rounded-[8px] text-[12.5px] font-semibold inline-flex items-center gap-1.5 transition-colors"
                  style={{
                    background: view === v ? 'var(--accent-hover)' : 'transparent',
                    color: view === v ? '#fff' : 'var(--fg-secondary)',
                  }}
                  title={lbl}
                >
                  <Icon size={15} /> <span className="hidden sm:inline">{lbl}</span>
                </button>
              ))}
            </div>
            <Btn label={L.importCsv} icon={Upload} onClick={() => navigate('/risks/import')} />
            <Btn
              label={L.newRisk}
              icon={Plus}
              primary
              onClick={() => window.dispatchEvent(new CustomEvent('openrisk:new-risk'))}
            />
          </>
        }
      />

      {view === 'map' ? (
        <Card style={{ padding: '8px 8px 4px', overflow: 'hidden' }}>
          {isLoading && ui.length === 0 ? (
            <SkeletonRows rows={6} />
          ) : ui.length === 0 ? (
            <EmptyState
              icon={ShieldAlert}
              title={tr('Aucun risque pour le moment', 'No risks yet')}
              description={tr(
                'Créez votre premier risque pour commencer à cartographier votre exposition.',
                'Create your first risk to start mapping your exposure.',
              )}
              primaryAction={
                <Btn
                  label={L.newRisk}
                  icon={Plus}
                  primary
                  onClick={() => window.dispatchEvent(new CustomEvent('openrisk:new-risk'))}
                />
              }
            />
          ) : (
            <RiskMatrixView risks={ui} onOpen={setDrawerId} />
          )}
        </Card>
      ) : (
        <DataTable
          id="risks"
          ariaLabel={L.riskTitle}
          rows={ui}
          total={total}
          columns={columns}
          rowKey={(r) => r.id}
          api={table}
          mode="server"
          loading={isLoading}
          error={loadError}
          onRetry={reload}
          facets={facets}
          searchPlaceholder={tr(
            'Rechercher par nom ou description…',
            'Search by name or description…',
          )}
          selectable
          rowActions={rowActions}
          bulkActions={bulkActions}
          onRowClick={(r) => setDrawerId(r.id)}
          exportFilename="risques"
          minWidth={880}
          toolbarExtra={
            <>
              {/* A shortcut onto the SAME url-backed facet the panel writes — not
                a second, divergent filter state. */}
              <button
                type="button"
                onClick={() => table.toggleFilter('criticality', 'critical')}
                data-testid="quick-critical"
                className="h-9 px-3 rounded-[10px] text-[12.5px] font-semibold inline-flex items-center gap-1.5 shrink-0"
                style={
                  (state.filters.criticality ?? []).includes('critical')
                    ? {
                        background: softFill('var(--critical)', 16),
                        color: 'var(--critical)',
                        border: '1px solid transparent',
                      }
                    : {
                        background: 'var(--bg-elevated)',
                        color: 'var(--fg-secondary)',
                        border: '1px solid var(--border-strong)',
                      }
                }
              >
                <ShieldAlert size={14} /> {L.critical}
                {critCount ? ` · ${critCount}` : ''}
              </button>
              {/* "Mes risques" — the whole point of splitting owner/assignee/
                reviewer: one toggle can now mean "anything I answer for, work
                on, or must validate". */}
              <button
                type="button"
                onClick={() => setMine((v) => !v)}
                data-testid="quick-mine"
                className="h-9 px-3 rounded-[10px] text-[12.5px] font-semibold inline-flex items-center gap-1.5 shrink-0"
                style={
                  mine
                    ? {
                        background: softFill('var(--accent)', 16),
                        color: 'var(--accent-500)',
                        border: '1px solid transparent',
                      }
                    : {
                        background: 'var(--bg-elevated)',
                        color: 'var(--fg-secondary)',
                        border: '1px solid var(--border-strong)',
                      }
                }
              >
                <UserCheck size={14} /> {tr('Mes risques', 'My risks')}
              </button>
              <button
                type="button"
                onClick={() => setUnmappedOnly((v) => !v)}
                data-testid="quick-unmapped"
                className="h-9 px-3 rounded-[10px] text-[12.5px] font-semibold inline-flex items-center gap-1.5 shrink-0"
                style={
                  unmappedOnly
                    ? {
                        background: softFill('var(--medium)', 16),
                        color: 'var(--medium)',
                        border: '1px solid transparent',
                      }
                    : {
                        background: 'var(--bg-elevated)',
                        color: 'var(--fg-secondary)',
                        border: '1px solid var(--border-strong)',
                      }
                }
              >
                <Link2Off size={14} /> {tr('Non mappés', 'Unmapped')}
              </button>
            </>
          }
          empty={
            <EmptyState
              icon={ShieldAlert}
              title={tr('Aucun risque pour le moment', 'No risks yet')}
              description={tr(
                'Créez votre premier risque pour commencer à cartographier votre exposition.',
                'Create your first risk to start mapping your exposure.',
              )}
              primaryAction={
                <Btn
                  label={L.newRisk}
                  icon={Plus}
                  primary
                  onClick={() => window.dispatchEvent(new CustomEvent('openrisk:new-risk'))}
                />
              }
            />
          }
        />
      )}

      {drawer && (
        <RiskDrawer
          // Keyed on the tab so a deep link that names one lands on it rather
          // than on whatever the previous open left behind.
          key={`${drawer.id}:${drawerTab ?? 'details'}`}
          r={drawer}
          initialTab={drawerTab}
          onClose={() => {
            setDrawerId(null);
            setDrawerTab(null);
          }}
          onEdit={() => setEditRaw(drawer.raw)}
          onExport={() => exportRiskCsv(drawer)}
          onCreateMiti={() => setMitiRiskId(drawer.id)}
        />
      )}

      <EditRiskModal
        isOpen={!!editRaw}
        risk={editRaw}
        onClose={() => setEditRaw(null)}
        onSuccess={() => {
          setEditRaw(null);
          reload();
        }}
      />
      <CreateMitigationModal
        isOpen={!!mitiRiskId}
        riskId={mitiRiskId ?? undefined}
        onClose={() => setMitiRiskId(null)}
        onCreated={() => {
          setMitiRiskId(null);
          reload();
          toast.success(
            tr('Plan de mitigation lié au risque', 'Mitigation plan linked to the risk'),
          );
        }}
      />

      <ImpactDialog
        open={!!toDelete}
        title={tr('Supprimer ce risque ?', 'Delete this risk?')}
        subject={toDelete?.name ?? ''}
        description={tr(
          'Action irréversible. Voici ce qui sera supprimé :',
          'This cannot be undone. Here is what will be removed:',
        )}
        impacts={
          toDelete
            ? [
                {
                  label: tr('Plans de mitigation liés', 'Linked mitigation plans'),
                  detail: String(toDelete.raw.mitigations?.length ?? 0),
                },
                {
                  label: tr('Historique & scores du risque', 'Risk history & scores'),
                  detail: tr('perdus', 'lost'),
                },
              ]
            : []
        }
        alternatives={
          toDelete
            ? [
                {
                  label: tr(
                    'Exporter le risque (CSV) avant de supprimer',
                    'Export the risk (CSV) first',
                  ),
                  description: tr(
                    'Gardez une trace hors-ligne avant la suppression.',
                    'Keep an offline record before deleting.',
                  ),
                  onClick: () => {
                    if (toDelete) exportRiskCsv(toDelete);
                  },
                },
              ]
            : []
        }
        confirmLabel={tr('Supprimer définitivement', 'Delete permanently')}
        cancelLabel={tr('Annuler', 'Cancel')}
        loading={deleting}
        onConfirm={confirmDeleteRisk}
        onClose={() => setToDelete(null)}
      />
    </PageFrame>
  );
}

/* ---------------- risk matrix (map) view ---------------- */
// Standard 5×5 GRC risk map: Impact (x, 1→5) × Probability (y, 5→1). Each risk is
// bucketed from its real prob (0–1) and impact (0–10); the cell tint is the cell's
// own severity (prob-bucket × impact-bucket), and each risk chip opens its drawer.
// NOTE: this is NOT a score band. It tints the 5×5 grid's own cells from their
// coordinates (bucket × bucket, 1–25) so the map reads as a heat map; no risk's
// score, and no risk's label, is derived from it. Kept local and named for what
// it is, so it cannot be mistaken for the scoring model in docs/scoring/.
function cellCrit(pBucket: number, iBucket: number): Criticality {
  const v = pBucket * iBucket; // 1..25, the grid's own coordinates
  return v >= 15 ? 'critical' : v >= 8 ? 'high' : v >= 4 ? 'medium' : 'low';
}
/* ---------------- inline status editor (ghost edit + autosave) ---------------- */

const STATUS_TO_BACKEND: Record<RiskStatus, string> = {
  open: 'open',
  progress: 'in_progress',
  mitigated: 'mitigated',
  accepted: 'accepted',
};
const STATUS_OPTIONS: RiskStatus[] = ['open', 'progress', 'mitigated', 'accepted'];

// Click-to-edit status right in the register row: no modal, no Save button — the
// change autosaves optimistically (System-1 quick edit) and a toast confirms it.
function InlineStatus({ risk }: { risk: UiRisk }) {
  const updateRisk = useRiskStore((s) => s.updateRisk);
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const [open, setOpen] = useState(false);
  const [saving, setSaving] = useState(false);

  const change = async (s: RiskStatus) => {
    setOpen(false);
    if (s === risk.status) return;
    setSaving(true);
    try {
      await updateRisk(risk.id, { status: STATUS_TO_BACKEND[s] });
      toast.success(tr('Statut mis à jour', 'Status updated'));
    } catch {
      toast.error(tr('Mise à jour échouée', 'Update failed'));
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="relative inline-block" onClick={(e) => e.stopPropagation()}>
      <button
        onClick={() => setOpen((v) => !v)}
        disabled={saving}
        title={tr('Changer le statut', 'Change status')}
        className="inline-flex items-center gap-1 rounded-md px-1.5 py-1 -mx-1 hover:bg-hover transition-colors"
      >
        <StatusPill status={risk.status} />
        {saving ? (
          <Loader2 size={12} className="animate-spin text-ink-muted" />
        ) : (
          <ChevronDown size={12} className="text-ink-muted" />
        )}
      </button>
      {open && (
        <>
          <div className="fixed inset-0 z-40" onClick={() => setOpen(false)} aria-hidden="true" />
          <div
            className="absolute left-0 top-full mt-1 z-50 min-w-[150px] rounded-[10px] p-1 shadow-card-lg"
            style={{ background: 'var(--bg-elevated)', border: '1px solid var(--border)' }}
          >
            {STATUS_OPTIONS.map((s) => (
              <button
                key={s}
                onClick={() => change(s)}
                className="w-full flex items-center justify-between gap-2 px-2 py-1.5 rounded-[7px] hover:bg-hover transition-colors text-left"
              >
                <StatusPill status={s} />
                {s === risk.status && <Check size={13} className="text-accent" />}
              </button>
            ))}
          </div>
        </>
      )}
    </div>
  );
}

/* The grid itself is <RiskMatrix> in shared/ds. This wrapper keeps what is
   product knowledge and cannot live in an Apache-2.0 primitive: the axis
   bucketing (probability is 0..1, impact 0..10) and cellCrit, the matrix's own
   coordinate-to-band ramp, which the primitive deliberately refuses to own. */
const toBucket = (v: number, max: number): MatrixBucket =>
  Math.min(5, Math.max(1, Math.ceil((v / max) * 5) || 1)) as MatrixBucket;

function RiskMatrixView({ risks, onOpen }: { risks: UiRisk[]; onOpen: (id: string) => void }) {
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const critLabel: Record<Criticality, string> = {
    critical: tr('Critique', 'Critical'),
    high: tr('Élevé', 'High'),
    medium: tr('Moyen', 'Medium'),
    low: tr('Faible', 'Low'),
  };

  return (
    <div className="p-4">
      <RiskMatrix
        items={risks.map((r) => ({
          id: r.id,
          label: `${r.name} · ${r.score.toFixed(1)}`,
          probability: toBucket(r.prob, 1),
          impact: toBucket(r.impact, 10),
          band: r.crit,
          marker: r.score.toFixed(0),
        }))}
        cellBand={cellCrit}
        onSelect={onOpen}
        labels={{
          caption: tr(
            'Matrice des risques — probabilité par impact',
            'Risk matrix — probability by impact',
          ),
          probability: tr('Probabilité', 'Probability'),
          impact: tr('Impact', 'Impact'),
          band: (b) => critLabel[b as Criticality] ?? b,
          more: (n) => `+${n}`,
        }}
      />
    </div>
  );
}

/* ---------------- drawer ---------------- */
// DrawerTimeline — the "Timeline" tab: real time-travel history for this risk
// (who changed what, and when) from GET /risks/:id/timeline, rendered with the
// shared HistoryTimeline primitive (UX-25). Replaces the former "coming soon".
const NIL_UUID = '00000000-0000-0000-0000-000000000000';
function DrawerTimeline({ r }: { r: UiRisk }) {
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const { data, isLoading, error } = useRiskTimeline(r.id);

  const kindOf = (t: string): string | undefined => {
    const k = t.toUpperCase();
    if (k === 'CREATE' || k === 'CREATED') return 'create';
    if (k === 'MITIGATE' || k.startsWith('MITIGATION')) return 'mitigate';
    if (k === 'DELETE' || k === 'DELETED') return 'delete';
    if (k) return 'update';
    return undefined;
  };
  const titleOf = (t: string): string => {
    switch (kindOf(t)) {
      case 'create':
        return tr('Risque créé', 'Risk created');
      case 'mitigate':
        return tr('Mitigation appliquée', 'Mitigation applied');
      case 'delete':
        return tr('Risque supprimé', 'Risk deleted');
      case 'update':
        return tr('Mise à jour', 'Updated');
      default:
        return t;
    }
  };
  const actorOf = (changedBy: string): string => {
    if (!changedBy || changedBy === NIL_UUID || changedBy.toLowerCase() === 'system')
      return tr('Système', 'System');
    return changedBy.length > 12 ? changedBy.slice(0, 8) : changedBy;
  };

  const entries: HistoryEntry[] = (data ?? []).map((e) => ({
    id: e.id,
    kind: kindOf(e.change_type),
    title: titleOf(e.change_type),
    actor: actorOf(e.changed_by),
    at: e.created_at,
    fields: [
      { label: tr('Score', 'Score'), value: (e.score ?? 0).toFixed(1) },
      { label: tr('Statut', 'Status'), value: e.status },
    ],
  }));

  return (
    <div className="py-5 px-[22px]">
      <HistoryTimeline
        entries={entries}
        isLoading={isLoading}
        error={!!error}
        emptyLabel={tr(
          'Aucun changement enregistré pour ce risque.',
          'No changes recorded for this risk yet.',
        )}
        errorLabel={tr('Chargement de l’historique impossible.', 'Could not load the history.')}
        formatDate={(iso) => relTime(iso, lang)}
      />
    </div>
  );
}

// DrawerCTI — the "CTI" tab.
//
// This tab rendered the string "Coming soon" for every risk, including risks the
// CTI engine itself created. Those carry `source_cve_id`, and /cti/vulnerabilities/:cve
// returns the real NVD + CISA-KEV record for it, so the data was one call away
// (W0-05 / D10).
//
// Two honest outcomes, and which one you get depends on the risk rather than on
// the release:
//
//   - the risk names a CVE  → its real advisory: CVSS, CISA-KEV status and due
//     date, MITRE tactics/techniques, remediation, with a link into Threat Intel;
//   - the risk names none   → says so, and explains what would populate it,
//     instead of implying the whole tab is unbuilt.
function DrawerCTI({ r }: { r: UiRisk }) {
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const navigate = useNavigate();
  const cve = r.raw.source_cve_id ?? '';

  const { data, isLoading, isError } = useQuery({
    queryKey: ['cti', 'vulnerability', cve],
    queryFn: () => ctiService.get(cve),
    enabled: !!cve,
  });

  if (!cve) {
    return (
      <div className="py-8 px-[22px]">
        <EmptyState
          icon={Bug}
          title={tr('Aucune CVE associée', 'No CVE linked')}
          description={tr(
            'Ce risque n’est rattaché à aucune vulnérabilité publique. Les risques créés par le moteur CTI, ou rattachés à une CVE, affichent ici leur avis complet (CVSS, CISA-KEV, MITRE ATT&CK).',
            'This risk is not linked to a public vulnerability. Risks raised by the CTI engine, or linked to a CVE, show the full advisory here (CVSS, CISA-KEV, MITRE ATT&CK).',
          )}
          primaryAction={
            <Btn
              label={tr('Ouvrir Threat Intel', 'Open Threat Intel')}
              onClick={() => navigate('/threat-map')}
            />
          }
        />
      </div>
    );
  }

  if (isLoading)
    return (
      <div className="py-5 px-[22px]">
        <SkeletonRows rows={4} />
      </div>
    );

  if (isError || !data) {
    // Not an empty state: the risk DOES name a CVE, so "nothing here" would be
    // a different and wrong claim.
    return (
      <div className="py-8 px-[22px]">
        <ErrorState
          title={tr('Avis indisponible', 'Advisory unavailable')}
          description={tr(
            `${cve} n’a pas pu être lu depuis le moteur CTI. Le flux n’a peut-être pas encore été synchronisé.`,
            `${cve} could not be read from the CTI engine. The feed may not have been synchronised yet.`,
          )}
          onRetry={() => navigate('/threat-map')}
          retryLabel={tr('Ouvrir Threat Intel', 'Open Threat Intel')}
        />
      </div>
    );
  }

  return (
    <div className="py-5 px-[22px] flex flex-col gap-4">
      <div className="flex items-center gap-2.5 flex-wrap">
        <span className="mono text-[13px] font-bold text-ink">{data.cve_id}</span>
        {data.cvss_v3 > 0 && (
          <span
            className="mono text-[12px] font-semibold px-2 py-0.5 rounded"
            style={{ background: 'var(--bg-hover)', color: 'var(--fg-secondary)' }}
          >
            CVSS {data.cvss_v3.toFixed(1)}
          </span>
        )}
        {data.cisa_known && (
          <span
            className="text-[11px] font-bold uppercase tracking-[.05em] px-2 py-0.5 rounded-full"
            style={{
              color: 'var(--critical)',
              background: 'color-mix(in srgb,var(--critical) 14%,transparent)',
            }}
          >
            CISA-KEV
          </span>
        )}
      </div>

      {data.description && (
        <div className="text-[13px] text-ink-soft leading-relaxed">{data.description}</div>
      )}

      {data.cisa_known && data.cisa_due_date && (
        <Fact
          label={tr('Échéance CISA', 'CISA due date')}
          value={new Date(data.cisa_due_date).toLocaleDateString(lang === 'fr' ? 'fr-FR' : 'en-GB')}
        />
      )}
      {!!data.mitre_tactics?.length && (
        <Fact
          label={tr('Tactiques MITRE', 'MITRE tactics')}
          value={data.mitre_tactics.join(', ')}
        />
      )}
      {!!data.mitre_techniques?.length && (
        <Fact
          label={tr('Techniques MITRE', 'MITRE techniques')}
          value={data.mitre_techniques.join(', ')}
        />
      )}
      {data.remediation && (
        <Fact label={tr('Remédiation', 'Remediation')} value={data.remediation} />
      )}

      <Btn
        label={tr('Voir dans Threat Intel', 'View in Threat Intel')}
        onClick={() => navigate(`/threat-map?q=${encodeURIComponent(data.cve_id)}`)}
      />
    </div>
  );
}

function Fact({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <div className="text-[11.5px] font-semibold text-ink-muted uppercase tracking-wide mb-1">
        {label}
      </div>
      <div className="text-[13px] text-ink leading-relaxed">{value}</div>
    </div>
  );
}

// DrawerAI — the "IA" tab: generates a synthesis + suggested treatment plan for
// this risk via the live /ai/risks/:id/treatment-plan endpoint (spec §12.1). Claude
// when configured, deterministic template otherwise (shown in the provenance line).
function DrawerAI({ r }: { r: UiRisk }) {
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const locale = lang === 'fr' ? 'fr' : 'en';
  const plan = useTreatmentPlan();
  const res = plan.data;

  const stratLabel: Record<string, string> = {
    mitigate: tr('Atténuer', 'Mitigate'),
    accept: tr('Accepter', 'Accept'),
    transfer: tr('Transférer', 'Transfer'),
    avoid: tr('Éviter', 'Avoid'),
  };
  const prioColor: Record<string, string> = {
    high: 'var(--critical)',
    medium: 'var(--high)',
    low: 'var(--accent)',
  };

  return (
    <div className="py-5 px-[22px]">
      <div className="flex items-start gap-2 mb-4">
        <div className="flex-1">
          <div className="text-[14px] font-semibold text-ink mb-1">
            {tr('Plan de traitement assisté par IA', 'AI-assisted treatment plan')}
          </div>
          <div className="text-[12.5px] text-ink-soft leading-relaxed">
            {tr(
              "L'IA synthétise ce risque et propose une stratégie et un plan d'actions, à partir de son score, sa criticité et l'actif lié.",
              'The AI synthesises this risk and proposes a strategy and action plan from its score, criticality and linked asset.',
            )}
          </div>
        </div>
      </div>

      <button
        onClick={() => plan.mutate({ riskId: r.id, locale })}
        disabled={plan.isPending}
        className="w-full h-11 rounded-[12px] flex items-center justify-center gap-2 text-fg-primary text-[13.5px] font-semibold disabled:opacity-60"
        style={{ background: 'var(--accent-solid)', color: 'var(--fg-on-solid)' }}
      >
        {plan.isPending ? <Loader2 size={17} className="animate-spin" /> : <Sparkles size={17} />}
        {plan.isPending
          ? tr('Génération…', 'Generating…')
          : res
            ? tr('Régénérer', 'Regenerate')
            : tr('Générer avec l’IA', 'Generate with AI')}
      </button>

      {plan.isPending && (
        // Informative wait (UX-09): the LLM call takes a few seconds — surface the
        // conceptual steps + the risk being analysed instead of a bare spinner.
        <ProgressState
          title={tr('Analyse en cours…', 'Analysing…')}
          stat={tr(`Risque : ${r.name}`, `Risk: ${r.name}`)}
          steps={[
            tr('Analyse du risque et de l’actif lié', 'Analysing the risk and linked asset'),
            tr('Choix de la stratégie de traitement', 'Choosing the treatment strategy'),
            tr('Priorisation du plan d’actions', 'Prioritising the action plan'),
          ]}
        />
      )}

      {plan.isError && (
        <div className="mt-4 text-[12.5px]" style={{ color: 'var(--critical)' }}>
          {tr('La génération a échoué. Réessayez.', 'Generation failed. Please try again.')}
        </div>
      )}

      {res && (
        <div className="mt-5 space-y-4" style={{ animation: 'or-fadeup .25s ease' }}>
          <div>
            <div className="text-[11px] font-semibold text-ink-muted uppercase tracking-wide mb-1.5">
              {tr('Synthèse', 'Summary')}
            </div>
            <div className="text-[13px] text-ink leading-relaxed">{res.plan.summary}</div>
          </div>

          <div className="flex items-center gap-2">
            <span className="text-[11px] font-semibold text-ink-muted uppercase tracking-wide">
              {tr('Stratégie', 'Strategy')}
            </span>
            <span
              className="text-[12px] font-semibold px-2.5 py-1 rounded-full"
              style={{
                background: 'var(--accent-soft)',
                color: 'var(--accent-500)',
                border: '1px solid var(--accent-line)',
              }}
            >
              {stratLabel[res.plan.recommended_strategy] ?? res.plan.recommended_strategy}
            </span>
          </div>

          <div>
            <div className="text-[11px] font-semibold text-ink-muted uppercase tracking-wide mb-2">
              {tr('Plan d’actions', 'Action plan')}
            </div>
            <div className="space-y-2.5">
              {res.plan.actions.map((a, i) => (
                <div
                  key={i}
                  className="p-3 rounded-[11px]"
                  style={{ background: 'var(--bg-elevated)', border: '1px solid var(--border)' }}
                >
                  <div className="flex items-center gap-2 mb-1">
                    <span
                      className="w-5 h-5 rounded-full flex items-center justify-center text-[11px] font-bold text-fg-primary shrink-0"
                      style={{ background: prioColor[a.priority] ?? 'var(--accent)' }}
                    >
                      {i + 1}
                    </span>
                    <span className="text-[13px] font-semibold text-ink flex-1">{a.title}</span>
                    <span
                      className="text-[10.5px] font-semibold uppercase"
                      style={{ color: prioColor[a.priority] ?? 'var(--accent)' }}
                    >
                      {a.priority}
                    </span>
                  </div>
                  <div className="text-[12.5px] text-ink-soft leading-relaxed pl-7">
                    {a.description}
                  </div>
                </div>
              ))}
            </div>
          </div>

          {res.plan.rationale && (
            <div className="text-[12px] text-ink-soft italic leading-relaxed">
              {res.plan.rationale}
            </div>
          )}

          <div
            className="text-[11px] text-ink-muted pt-1"
            style={{ borderTop: '1px solid var(--border)' }}
          >
            <span className="pt-2 inline-block">
              {tr('Généré par', 'Generated by')} :{' '}
              <span className="font-semibold">{res.generated_by}</span>
              {res.generated_by === 'template' &&
                ' · ' +
                  tr(
                    'mode local (configurez ANTHROPIC_API_KEY pour Claude)',
                    'local mode (set ANTHROPIC_API_KEY for Claude)',
                  )}
            </span>
          </div>
        </div>
      )}
    </div>
  );
}

/** The drawer's tabs. Exported as a value so `?tab=` can be validated against
 *  the same list the drawer renders — a deep link cannot name a tab that does
 *  not exist, and adding a tab cannot forget to make it linkable. */
const DRAWER_TABS = [
  'details',
  'lifecycle',
  'score',
  'smart',
  'financial',
  'miti',
  'timeline',
  'cti',
  'ai',
] as const;
type DrawerTab = (typeof DRAWER_TABS)[number];

function RiskDrawer({
  r,
  onClose,
  onEdit,
  onExport,
  onCreateMiti,
  initialTab,
}: {
  r: UiRisk;
  onClose: () => void;
  onEdit: () => void;
  onExport: () => void;
  onCreateMiti: () => void;
  initialTab?: DrawerTab | null;
}) {
  const L = useUIStrings();
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const [tab, setTab] = useState<DrawerTab>(initialTab ?? 'details');
  const tabDef: [typeof tab, string][] = [
    ['details', L.tab_details],
    ['lifecycle', tr('Cycle de vie', 'Lifecycle')],
    ['score', L.tab_score],
    ['smart', tr('Score intelligent', 'Smart score')],
    ['financial', tr('Financier', 'Financial')],
    ['miti', L.tab_miti],
    ['timeline', L.tab_timeline],
    ['cti', L.tab_cti],
    ['ai', L.tab_ai],
  ];
  return (
    <div
      className="fixed inset-0 z-70 flex justify-end"
      style={{
        background: 'rgba(0,0,0,.45)',
        backdropFilter: 'blur(3px)',
        animation: 'or-fadein .2s ease',
      }}
      onClick={onClose}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        className="h-full flex flex-col"
        style={{
          width: 'min(94vw,560px)',
          background: 'var(--bg-secondary)',
          borderLeft: '1px solid var(--border)',
          boxShadow: 'var(--shadow-lg)',
          animation: 'or-slidein .3s cubic-bezier(.2,.8,.2,1)',
        }}
      >
        <div className="px-[22px] pt-5 pb-3.5">
          <div className="flex items-start gap-3 mb-3">
            <div className="flex-1">
              <div className="mono text-[11px] text-ink-muted mb-[5px]">#{r.id.slice(0, 8)}</div>
              <div className="disp text-[18px] font-bold text-ink leading-snug">{r.name}</div>
            </div>
            <button
              onClick={onClose}
              className="w-8 h-8 rounded-[9px] flex items-center justify-center shrink-0 text-ink-soft"
              style={{ background: 'var(--bg-hover)' }}
            >
              <X size={18} />
            </button>
          </div>
          <div className="flex items-center gap-2.5 flex-wrap">
            <CritBadge crit={r.crit} />
            <StatusPill status={r.status} />
            <PhasePill phase={r.phase} lang={lang} />
            <span
              className="mono text-[13px] font-bold ml-auto"
              style={{ color: critColor[r.crit] }}
            >
              Score {r.score.toFixed(1)}
            </span>
          </div>
          <div className="flex gap-2 mt-3.5">
            <Btn label={L.edit} icon={Pencil} onClick={onEdit} />
            <Btn label={L.exportCsv} icon={FileText} onClick={onExport} />
          </div>
        </div>

        <div
          className="flex gap-0.5 px-[22px] overflow-x-auto"
          style={{ borderBottom: '1px solid var(--border)' }}
        >
          {tabDef.map(([k, lbl]) => (
            <button
              key={k}
              onClick={() => setTab(k)}
              className="px-3 py-[11px] text-[13px] whitespace-nowrap"
              style={{
                color: tab === k ? 'var(--fg-primary)' : 'var(--fg-secondary)',
                fontWeight: tab === k ? 600 : 500,
                borderBottom: `2px solid ${tab === k ? 'var(--accent)' : 'transparent'}`,
                marginBottom: -1,
              }}
            >
              {lbl}
            </button>
          ))}
        </div>

        <div className="flex-1 overflow-y-auto">
          {tab === 'details' && <DrawerDetails r={r} onCreateMiti={onCreateMiti} />}
          {tab === 'lifecycle' && (
            <DrawerLifecycle r={r} onOpenMitigations={() => setTab('miti')} />
          )}
          {tab === 'score' && <DrawerScore r={r} />}
          {tab === 'smart' && <DrawerSmart r={r} />}
          {tab === 'financial' && <DrawerFinancial r={r} />}
          {tab === 'miti' && <DrawerMiti r={r} onCreateMiti={onCreateMiti} />}
          {tab === 'ai' && <DrawerAI r={r} />}
          {tab === 'timeline' && <DrawerTimeline r={r} />}
          {tab === 'cti' && <DrawerCTI r={r} />}
        </div>
      </div>
    </div>
  );
}

function DrawerDetails({ r, onCreateMiti }: { r: UiRisk; onCreateMiti: () => void }) {
  const L = useUIStrings();
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const navigate = useNavigate();
  const canUpdate = useAuthStore((s) => s.hasPermission('risks:update'));
  const updateRisk = useRiskStore((s) => s.updateRisk);
  const { data: mappings } = useRiskMappings(r.id);
  const deleteMapping = useDeleteMapping(r.id);
  const createMapping = useCreateMapping(r.id);
  const [mappingDrafts, setMappingDrafts] = useState<MappingDraft[]>([]);
  const [savingOwner, setSavingOwner] = useState(false);
  const mitiCount = r.raw.mitigations_count ?? r.raw.mitigations?.length ?? 0;

  const field = (lbl: string, val: string) => (
    <div className="mb-4">
      <div className="text-[11px] font-semibold uppercase tracking-[.04em] text-ink-muted mb-1.5">
        {lbl}
      </div>
      <div className="text-[13.5px] text-ink">{val}</div>
    </div>
  );

  // Assignment saves immediately. It is a one-field decision with an obvious
  // undo (pick someone else), so a Save button would only add a step people
  // forget — which is how "j'ai assigné mais rien ne s'est passé" happens.
  const assign = async (role: OwnershipRole, userId: string | null) => {
    setSavingOwner(true);
    try {
      await updateRisk(r.id, ownershipPatch({ [role]: userId }));
      toast.success(
        userId
          ? tr('Affectation enregistrée', 'Assignment saved')
          : tr('Affectation retirée', 'Assignment removed'),
      );
    } catch {
      toast.error(tr("L'affectation a échoué.", 'The assignment failed.'));
    } finally {
      setSavingOwner(false);
    }
  };

  const addMappings = async () => {
    for (const d of mappingDrafts) {
      try {
        await createMapping.mutateAsync({
          framework_id: d.framework_id,
          control_id: d.control_id ?? null,
        });
      } catch {
        toast.error(tr(`Impossible de rattacher ${d.label}`, `Could not map ${d.label}`));
      }
    }
    setMappingDrafts([]);
  };

  return (
    <div className="px-[22px] py-5">
      {field('Description', r.desc || '—')}
      <div className="grid grid-cols-2 gap-x-5">
        {field(lang === 'fr' ? 'Actif concerné' : 'Asset', r.asset)}
        {field(L.col_category, r.categoryName || '—')}
        {field(L.col_mod, r.mod)}
        {field(L.col_tags, r.tags.length ? r.tags.join(', ') : '—')}
      </div>

      {/* Ownership — three slots, one picker, available right here rather than
          behind an edit modal. */}
      <div className="mb-5">
        <div className="text-[11px] font-semibold uppercase tracking-[.04em] text-ink-muted mb-2">
          {tr('Responsabilités', 'Ownership')}
        </div>
        <OwnershipFields
          value={r.raw}
          onChange={assign}
          permission="risks:update"
          disabled={!canUpdate || savingOwner}
        />
      </div>

      {/* Compliance references — real controls, each one clickable through. */}
      <div className="mb-5">
        <div className="text-[11px] font-semibold uppercase tracking-[.04em] text-ink-muted mb-2">
          {L.col_fw}
        </div>
        {(mappings ?? []).length === 0 ? (
          <p className="text-[12.5px] text-ink-muted mb-2">
            {tr(
              'Ce risque n’est rattaché à aucun contrôle.',
              'This risk is not linked to any control.',
            )}
          </p>
        ) : (
          <div className="flex flex-wrap gap-1.5 mb-2">
            {(mappings ?? []).map((m) => (
              <span
                key={m.id}
                className="inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-[12px] font-semibold"
                style={{
                  background: 'color-mix(in srgb, var(--accent) 14%, transparent)',
                  color: 'var(--accent-500)',
                }}
              >
                <button
                  type="button"
                  onClick={() => navigate(mappingHref(m))}
                  className="hover:underline"
                >
                  {mappingLabel(m)}
                </button>
                {canUpdate ? (
                  <button
                    type="button"
                    onClick={() => deleteMapping.mutate(m.id)}
                    aria-label={tr('Retirer', 'Remove')}
                    className="opacity-70 hover:opacity-100"
                  >
                    <X size={11} />
                  </button>
                ) : null}
              </span>
            ))}
          </div>
        )}
        {canUpdate ? (
          <>
            <ComplianceMappingField
              value={mappingDrafts}
              onChange={setMappingDrafts}
              onImportFramework={() => navigate('/compliance')}
              disabled={createMapping.isPending}
            />
            {mappingDrafts.length > 0 ? (
              <button
                type="button"
                onClick={addMappings}
                className="mt-2 rounded-full px-3 py-1.5 text-[12px] font-semibold"
                style={{
                  background: 'var(--accent)',
                  color: 'var(--on-accent, var(--fg-primary))',
                }}
              >
                {tr('Rattacher', 'Map')}
              </button>
            ) : null}
          </>
        ) : null}
      </div>

      {/* §6: the button is DERIVED from what exists, not from what the user did
          a moment ago. Once a plan exists, "Créer une mitigation" is the wrong
          offer — the useful one is to go and look at it. */}
      {mitiCount > 0 ? (
        <button
          onClick={() => navigate(`/risks/mitigations?risk_id=${r.id}`)}
          className="mt-2 w-full h-10 rounded-[10px] flex items-center justify-center gap-2 text-[13px] font-semibold transition-all hover:brightness-110"
          style={{
            background: 'var(--bg-elevated)',
            border: '1px solid var(--border-strong)',
            color: 'var(--fg-secondary)',
          }}
        >
          <ShieldCheck size={16} />{' '}
          {tr(`Voir les mitigations (${mitiCount})`, `View mitigations (${mitiCount})`)}
        </button>
      ) : (
        <button
          onClick={onCreateMiti}
          className="mt-2 w-full h-10 rounded-[10px] flex items-center justify-center gap-2 text-[13px] font-semibold text-fg-on-solid transition-all hover:brightness-110"
          style={{ background: 'var(--accent-solid)' }}
        >
          <ShieldCheck size={16} /> {L.createMiti}
        </button>
      )}
    </div>
  );
}

function DrawerScore({ r }: { r: UiRisk }) {
  const L = useUIStrings();
  const lang = useUIStore((s) => s.lang);
  const gauge = (val: number, max: number, lbl: string, col: string) => {
    const pct = Math.max(0, Math.min(1, val / max)),
      cx = 52,
      cy = 52,
      rr = 42;
    const track = arcPath(cx, cy, rr, -130, 130);
    const prog = arcPath(cx, cy, rr, -130, -130 + 260 * pct);
    return (
      <div className="text-center">
        <div className="relative mx-auto" style={{ width: 104, height: 92 }}>
          <svg viewBox="0 0 104 96" width={104} height={96}>
            <path
              d={track}
              fill="none"
              stroke="var(--bg-hover)"
              strokeWidth={9}
              strokeLinecap="round"
            />
            <path d={prog} fill="none" stroke={col} strokeWidth={9} strokeLinecap="round" />
          </svg>
          <div
            className="mono absolute left-0 right-0 text-center text-[20px] font-bold text-ink"
            style={{ top: 30 }}
          >
            {val.toFixed(1)}
          </div>
        </div>
        <div className="text-[11.5px] text-ink-soft font-medium">{lbl}</div>
      </div>
    );
  };
  return (
    <div className="p-[22px]">
      <div className="flex justify-around mb-[22px]">
        {gauge(r.prob * 10, 10, L.proba, 'var(--accent)')}
        {gauge(r.impact, 10, L.impact, 'var(--high)')}
        {gauge(r.ac, 3, lang === 'fr' ? 'Criticité actif' : 'Asset criticality', 'var(--info)')}
      </div>
      <div
        className="text-center p-[18px] rounded-[14px]"
        style={{ background: 'var(--bg-hover)' }}
      >
        <div className="mono text-[15px] text-ink-soft">
          <span>{r.prob.toFixed(1)}</span>
          <span className="mx-2 text-ink-muted">×</span>
          <span>{r.impact.toFixed(1)}</span>
          <span className="mx-2 text-ink-muted">×</span>
          <span>{r.ac.toFixed(1)}</span>
          <span className="mx-2.5 text-ink-muted">=</span>
          <span className="text-[22px] font-bold" style={{ color: critColor[r.crit] }}>
            {r.score.toFixed(1)}
          </span>
        </div>
        <div className="text-[12px] text-ink-muted mt-2">
          {lang === 'fr'
            ? 'Probabilité × Impact × Criticité de l’actif'
            : 'Probability × Impact × Asset criticality'}
        </div>
      </div>
    </div>
  );
}

/* ---------------- smart score (multifactor, spec §8) ---------------- */
function DrawerSmart({ r }: { r: UiRisk }) {
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const navigate = useNavigate();
  const { data, isLoading, isError } = useRiskSmartScore(r.id);

  if (isLoading) {
    return (
      <div className="p-[22px]">
        <SkeletonRows rows={5} />
      </div>
    );
  }
  if (isError || !data) {
    return (
      <div className="py-10 px-[22px] text-center text-[13px] text-ink-soft">
        {tr('Impossible de calculer le score intelligent.', 'Could not compute the smart score.')}
      </div>
    );
  }
  return (
    <div className="p-[22px]">
      <SmartRiskRadar data={data} lang={lang} />
      <button
        onClick={() => navigate('/risks/weighting')}
        className="mt-4 w-full h-9 rounded-[10px] flex items-center justify-center gap-2 text-[12.5px] font-semibold transition-all hover:bg-hover"
        style={{ border: '1px solid var(--border-strong)', color: 'var(--fg-secondary)' }}
      >
        <SlidersHorizontal size={15} /> {tr('Configurer les pondérations', 'Configure weights')}
      </button>
      <p className="mt-3 text-[11px] text-ink-muted leading-snug">
        {tr(
          'Score multifactoriel (0–100) : criticité métier, exposition Internet, vulnérabilités, maturité des contrôles, historique d’incidents, exploitabilité, valeur financière et menaces actives (CTI). Le score classique P × I × Criticité reste inchangé.',
          'Multifactor score (0–100): business criticality, internet exposure, vulnerabilities, control maturity, incident history, exploitability, financial value and active threats (CTI). The classic P × I × Criticality score is unchanged.',
        )}
      </p>
    </div>
  );
}

/* ---------------- lifecycle ---------------- */
//
// There is no client-side copy of the state graph any more. This used to hold a
// `canTransition` that "mirrors the backend" — and mirrors drift: the button the
// UI offered and the transition the server would accept stopped agreeing, which
// is how a risk reached "traité" with open sub-actions. <LifecycleStepper>
// renders GET /risks/:id/transitions verbatim, blockers included.

/** State pill for the register row, from the single canonical lifecycle. */
function PhasePill({ phase, lang }: { phase: RiskPhase; lang: 'fr' | 'en' }) {
  const closed = phase === 'closed';
  const col = closed ? 'var(--fg-secondary)' : 'var(--accent)';
  const labels: Record<RiskPhase, [string, string]> = {
    identified: ['Identifié', 'Identified'],
    analyzed: ['Évalué', 'Assessed'],
    evaluated: ['Traitement planifié', 'Treatment planned'],
    treated: ['En traitement', 'In treatment'],
    monitored: ['Traité', 'Mitigated'],
    closed: ['Clôturé', 'Closed'],
  };
  return (
    <span
      className="inline-flex items-center gap-1.5 h-[22px] px-2.5 rounded-full text-[11.5px] font-semibold"
      style={{ color: col, background: 'color-mix(in srgb,var(--accent) 12%,transparent)' }}
    >
      <RouteIcon size={12} /> {labels[phase]?.[lang === 'fr' ? 0 : 1] ?? phase}
    </span>
  );
}

function DrawerLifecycle({ r, onOpenMitigations }: { r: UiRisk; onOpenMitigations?: () => void }) {
  const lang = useUIStore((s) => s.lang);
  const navigate = useNavigate();
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  return (
    <div className="px-[22px] py-5">
      <div className="text-[13px] text-ink-soft mb-4">
        {tr(
          'Le cycle de vie et le plan de mitigation sont un seul flux : une étape se débloque quand le travail correspondant est fait, pas quand on coche une case.',
          'The lifecycle and the mitigation plan are one flow: a step unlocks when the work behind it is done, not when someone ticks a box.',
        )}
      </div>
      <LifecycleStepper
        riskId={r.id}
        onOpenMitigations={onOpenMitigations}
        onOpenGovernance={() => navigate('/governance?tab=approvals')}
      />
    </div>
  );
}

/* ---------------- financial (CRQ) ---------------- */
function DrawerFinancial({ r }: { r: UiRisk }) {
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const updateRisk = useRiskStore((s) => s.updateRisk);
  const canUpdate = useAuthStore((s) => s.hasPermission('risks:update'));
  const qc = useQueryClient();
  const raw = r.raw;
  // Live full assessment from the backend (SLE, downtime, worst/avg, ROSI).
  const { data: fin } = useRiskFinancial(r.id);

  const s = (v: number | null | undefined) => (v == null ? '' : String(v));
  const [sle, setSle] = useState(s(raw.sle_xaf));
  const [aro, setAro] = useState(s(raw.aro));
  const [dh, setDh] = useState(s(raw.downtime_hours));
  const [hc, setHc] = useState(s(raw.hourly_downtime_cost_xaf));
  const [dl, setDl] = useState(s(raw.data_loss_cost_xaf));
  const [fn, setFn] = useState(s(raw.fines_xaf));
  const [oc, setOc] = useState(s(raw.other_direct_cost_xaf));
  const [rc, setRc] = useState(s(raw.remediation_cost_xaf));
  const [eff, setEff] = useState(
    raw.mitigation_effectiveness != null ? Number(raw.mitigation_effectiveness) : 0,
  );
  const [busy, setBusy] = useState(false);

  const fmtXAF = (v?: number) =>
    v == null ? '—' : `${Math.round(v).toLocaleString('fr-FR')} FCFA`;
  const fmtUSD = (v?: number) =>
    v == null ? '—' : `$${v.toLocaleString('en-US', { maximumFractionDigits: 0 })}`;
  const fmtPct = (ratio: number) => `${ratio >= 0 ? '+' : ''}${Math.round(ratio * 100)}%`;
  const num = (v: string) => (v.trim() === '' ? null : Number(v));

  const save = async () => {
    setBusy(true);
    try {
      await updateRisk(r.id, {
        sle_xaf: num(sle),
        aro: num(aro),
        downtime_hours: num(dh),
        hourly_downtime_cost_xaf: num(hc),
        data_loss_cost_xaf: num(dl),
        fines_xaf: num(fn),
        other_direct_cost_xaf: num(oc),
        remediation_cost_xaf: num(rc),
        mitigation_effectiveness: eff,
      });
      await qc.invalidateQueries({ queryKey: ['financial'] });
      toast.success(tr('Exposition recalculée', 'Exposure recalculated'));
    } catch {
      toast.error(tr('Échec du recalcul', 'Recalculation failed'));
    } finally {
      setBusy(false);
    }
  };

  const stat = (label: string, value: string, tone?: string) => (
    <div
      className="rounded-[10px] p-2.5"
      style={{ border: '1px solid var(--border)', background: 'var(--bg-hover)' }}
    >
      <div className="text-[10px] uppercase tracking-[.05em] text-ink-muted">{label}</div>
      <div className="mono text-[14px] font-bold mt-0.5" style={{ color: tone ?? 'var(--ink)' }}>
        {value}
      </div>
    </div>
  );
  const field = (label: string, val: string, set: (v: string) => void, step?: string) => (
    <label className="block">
      <span className="text-[11px] font-semibold uppercase tracking-[.04em] text-ink-muted">
        {label}
      </span>
      <input
        value={val}
        onChange={(e) => set(e.target.value)}
        type="number"
        step={step}
        min={0}
        className="mt-1.5 w-full rounded-[10px] px-3 py-2 text-[13px] text-ink outline-none"
        style={{ background: 'var(--bg-secondary)', border: '1px solid var(--border)' }}
      />
    </label>
  );

  return (
    <div className="px-[22px] py-5">
      <div className="text-[13px] text-ink-soft mb-4">
        {tr(
          'Quantification financière (FAIR/CRQ) — ALE = SLE × ARO ; ROSI = (ALE − ALE résiduel − remédiation) / remédiation.',
          'Financial quantification (FAIR/CRQ) — ALE = SLE × ARO; ROSI = (ALE − residual ALE − remediation) / remediation.',
        )}
      </div>

      {/* Headline ALE (XAF + USD) */}
      <div className="grid grid-cols-2 gap-3 mb-3">
        <div
          className="rounded-[12px] p-4"
          style={{
            border: '1px solid color-mix(in srgb,var(--accent) 30%,transparent)',
            background: 'color-mix(in srgb,var(--accent) 6%,transparent)',
          }}
        >
          <div className="text-[10.5px] uppercase tracking-[.06em] text-ink-muted">ALE (FCFA)</div>
          <div className="mono text-[20px] font-bold text-ink mt-1">
            {fmtXAF(fin?.ale.xaf ?? raw.ale_xaf)}
          </div>
        </div>
        <div
          className="rounded-[12px] p-4"
          style={{ border: '1px solid var(--border)', background: 'var(--bg-hover)' }}
        >
          <div className="text-[10.5px] uppercase tracking-[.06em] text-ink-muted">ALE (USD)</div>
          <div className="mono text-[20px] font-bold text-ink mt-1">
            {fmtUSD(fin?.ale.usd ?? raw.ale_usd)}
          </div>
        </div>
      </div>

      {/* Full assessment breakdown */}
      {fin && (
        <div className="grid grid-cols-3 gap-2 mb-3">
          {stat('SLE', fmtXAF(fin.sle.xaf))}
          {stat(tr('Coût interruptions', 'Downtime cost'), fmtXAF(fin.downtime_cost.xaf))}
          {stat(
            tr('Pire cas (ALE)', 'Worst-case ALE'),
            fmtXAF(fin.ale_worst.xaf),
            'var(--critical)',
          )}
          {stat(tr('ALE moyen', 'Average ALE'), fmtXAF(fin.ale_average.xaf))}
          {stat(tr('ALE résiduel', 'Residual ALE'), fmtXAF(fin.ale_after.xaf), 'var(--low)')}
          {stat(
            'ROSI',
            fin.rosi_computable ? fmtPct(fin.rosi) : '—',
            fin.rosi_computable ? (fin.rosi >= 0 ? 'var(--low)' : 'var(--critical)') : undefined,
          )}
        </div>
      )}
      <div className="text-[11.5px] text-ink-muted mb-4">
        {tr('Base SLE : ', 'SLE basis: ')}
        {fin?.sle_basis === 'explicit'
          ? tr('saisie explicite', 'explicit input')
          : fin?.sle_basis === 'composed'
            ? tr(
                'composé (interruptions + amendes + perte de données)',
                'composed (downtime + fines + data loss)',
              )
            : tr('valeur de référence par criticité', 'reference value by criticality')}
      </div>

      {canUpdate && (
        <div
          className="rounded-[12px] p-3.5"
          style={{ border: '1px solid var(--border)', background: 'var(--bg-hover)' }}
        >
          <div className="text-[11px] font-semibold uppercase tracking-[.05em] text-ink-muted mb-2">
            {tr('Pertes', 'Losses')}
          </div>
          <div className="grid grid-cols-2 gap-3 mb-3">
            {field(tr('SLE explicite (FCFA)', 'Explicit SLE (FCFA)'), sle, setSle)}
            {field(tr('ARO — Fréquence / an', 'ARO — Frequency / yr'), aro, setAro, '0.1')}
            {field(tr('Heures d’interruption', 'Downtime hours'), dh, setDh, '0.5')}
            {field(tr('Coût horaire (FCFA)', 'Hourly cost (FCFA)'), hc, setHc)}
            {field(tr('Perte de données (FCFA)', 'Data loss (FCFA)'), dl, setDl)}
            {field(tr('Amendes (FCFA)', 'Fines (FCFA)'), fn, setFn)}
            {field(tr('Autre coût direct (FCFA)', 'Other direct cost (FCFA)'), oc, setOc)}
          </div>
          <div className="text-[11px] font-semibold uppercase tracking-[.05em] text-ink-muted mb-2">
            {tr('Remédiation (ROSI)', 'Remediation (ROSI)')}
          </div>
          <div className="grid grid-cols-2 gap-3 mb-3 items-end">
            {field(tr('Coût de remédiation (FCFA)', 'Remediation cost (FCFA)'), rc, setRc)}
            <label className="block">
              <span className="flex items-center justify-between text-[11px] font-semibold uppercase tracking-[.04em] text-ink-muted">
                {tr('Efficacité', 'Effectiveness')}
                <span className="mono text-ink">{Math.round(eff * 100)}%</span>
              </span>
              <input
                value={eff}
                onChange={(e) => setEff(Number(e.target.value))}
                type="range"
                min={0}
                max={1}
                step={0.05}
                className="mt-3 w-full accent-(--accent)"
              />
            </label>
          </div>
          <button
            disabled={busy}
            onClick={save}
            className="w-full h-10 rounded-[10px] flex items-center justify-center gap-2 text-[13px] font-semibold text-fg-on-solid disabled:opacity-60"
            style={{ background: 'var(--accent-solid)', color: 'var(--fg-on-solid)' }}
          >
            <Coins size={16} /> {tr('Recalculer l’exposition', 'Recalculate exposure')}
          </button>
          <div className="text-[11px] text-ink-muted mt-2">
            {tr(
              'SLE explicite prioritaire ; sinon composé depuis interruptions + amendes + perte de données ; sinon référence par criticité.',
              'Explicit SLE wins; else composed from downtime + fines + data loss; else reference by criticality.',
            )}
          </div>
        </div>
      )}
    </div>
  );
}

function DrawerMiti({ r, onCreateMiti }: { r: UiRisk; onCreateMiti: () => void }) {
  const L = useUIStrings();
  const lang = useUIStore((s) => s.lang);
  const linked = r.raw.mitigations ?? [];
  if (!linked.length) {
    return (
      <div className="py-10 px-[22px] text-center">
        <div className="text-[13px] text-ink-soft mb-3.5">
          {lang === 'fr' ? 'Aucun plan de mitigation lié.' : 'No linked mitigation plan.'}
        </div>
        <div className="flex justify-center">
          <Btn label={L.createMiti} icon={Plus} primary onClick={onCreateMiti} />
        </div>
      </div>
    );
  }
  return (
    <div className="px-[22px] py-5">
      {linked.map((x) => (
        <div
          key={x.id}
          className="p-3.5 rounded-[12px] mb-2.5"
          style={{ border: '1px solid var(--border)' }}
        >
          <div className="text-[13.5px] font-medium text-ink mb-2.5">{x.title}</div>
          <div
            className="h-[5px] rounded-[5px] overflow-hidden mb-2"
            style={{ background: 'var(--bg-hover)' }}
          >
            <div
              className="h-full rounded-[5px]"
              style={{ width: `${x.progress ?? 0}%`, background: 'var(--low)' }}
            />
          </div>
          <div className="flex items-center justify-between text-[11.5px] text-ink-muted">
            <span>{x.progress ?? 0}%</span>
            <span className="inline-flex items-center gap-1">
              <Clock size={12} /> {x.status}
            </span>
          </div>
        </div>
      ))}
      <div className="flex justify-center mt-2">
        <Btn label={L.createMiti} icon={Plus} onClick={onCreateMiti} />
      </div>
    </div>
  );
}
