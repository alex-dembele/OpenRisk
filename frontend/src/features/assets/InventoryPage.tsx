// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// Inventory (OpenRisk.dc.html §6.12) — wired to the real /assets store. Asset table
// with type-icon, criticality badge, derived score (max of linked risks), linked-risk
// count and last-updated. Type-filter chips; create/edit modals; loading + empty states.

import { useMemo, useState } from 'react';
import { useNavigate } from 'react-router';
import { toast } from 'sonner';
import {
  Atom,
  Plus,
  SlidersHorizontal,
  Server,
  Laptop,
  Database,
  Cloud,
  Globe,
  HardDrive,
  Boxes,
  AppWindow,
  Users,
  Building2,
  History,
  Pencil,
  Trash2,
  type LucideIcon,
} from 'lucide-react';
import { PageFrame, PageHeader, Btn, CritBadge, EmptyState } from '../../shared/ui';
import {
  DataTable,
  useTableState,
  type BulkAction,
  type Column,
  type Facet,
  type RowAction,
} from '../../shared/datatable';
import { useAuthStore } from '../../hooks/useAuthStore';
import { ImpactDialog } from '../../shared/ImpactDialog';
import { critColor, softFill, type Criticality } from '../../shared/riskColors';
import { useUIStrings } from '../../shared/uiStrings';
import { useUIStore } from '../../store/uiStore';
import { useAssets } from './useAssets';
import { CreateAssetModal } from './CreateAssetModal';
import { EditAssetModal } from './EditAssetModal';
import { AssetHistoryDrawer } from './AssetHistoryDrawer';
import { useFocusParam } from '../../shared/useFocusParam';
import { relTime } from '../risks/riskMap';
import { AttributeSearchBar } from '../attackSurface/AttributeSearchBar';
import { CATEGORY_LABELS, type AssetCategory } from '../attackSurface/schemaTypes';
import type { Asset } from '../../types/asset';
import { BulkPreviewDialog, useGovernedBulk, type BulkChangeInput } from '../../shared/bulk';
import type { LocaleCode } from '../../i18n/locales';

const TYPE_ICON: Record<string, LucideIcon> = {
  Server: Server,
  Application: AppWindow,
  Cloud: Cloud,
  Database: Database,
  SaaS: Cloud,
  Storage: HardDrive,
  Network: Globe,
  Laptop: Laptop,
  Data: Database,
  User: Users,
  Supplier: Building2,
};

// Derived asset score = the max score of its linked risks (null when none).
const scoreOf = (a: Asset): number | null => {
  const rs = a.risks ?? [];
  if (!rs.length) return null;
  return Math.max(...rs.map((r) => r.score ?? 0));
};
const CRIT_RANK: Record<string, number> = { critical: 4, high: 3, medium: 2, low: 1 };
const CRIT_LABEL_FR: Record<string, string> = {
  critical: 'Critique',
  high: 'Élevée',
  medium: 'Moyenne',
  low: 'Faible',
};
const CRIT_LABEL_EN: Record<string, string> = {
  critical: 'Critical',
  high: 'High',
  medium: 'Medium',
  low: 'Low',
};
/**
 * The pending change, applied to the cached inventory (ABSOLUTE RULE 10).
 * Restored verbatim by the hook if the server refuses — criterion 8.
 *
 * The ['assets'] prefix also caches each asset's history, which is not an array
 * of assets; anything that is not the inventory is handed back untouched.
 * Assets support delete alone, so there is no second branch to write.
 */
function patchInventory(cached: unknown, ids: ReadonlySet<string>, change: BulkChangeInput): unknown {
  if (change.action !== 'delete' || !isAssetList(cached)) return cached;
  return cached.filter((a) => !ids.has(a.id as string));
}

function isAssetList(cached: unknown): cached is Asset[] {
  return Array.isArray(cached) && cached.every((a) => typeof a === 'object' && a !== null && 'id' in a);
}

const t = (lang: LocaleCode, fr: string, en: string) => (lang === 'fr' ? fr : en);

export function InventoryPage() {
  const L = useUIStrings();
  const lang = useUIStore((s) => s.lang);
  const navigate = useNavigate();
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  // Typed-attribute search (Attack Surface §1). The terms travel to the server
  // as ?category=&attr.<key>=<value>; the matching rules live there, once.
  const [attrFilter, setAttrFilter] = useState<{
    category: AssetCategory | '';
    attributes: Record<string, string>;
  }>({ category: '', attributes: {} });
  const { assets, isLoading, isError, refetch, deleteAsset } = useAssets({
    category: attrFilter.category || undefined,
    attributes: attrFilter.attributes,
  });
  const canUpdate = useAuthStore((s) => s.hasPermission('assets:update'));
  const canDelete = useAuthStore((s) => s.hasPermission('assets:delete'));
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<Asset | undefined>(undefined);
  const [historyAssetId, setHistoryAssetId] = useState<string | null>(null);
  const [toDelete, setToDelete] = useState<Asset | null>(null);
  const [deleting, setDeleting] = useState(false);

  // GET /assets returns the whole inventory (no server pagination — the graph
  // view needs every node), so the table filters, sorts and pages client-side.
  const table = useTableState({ defaultSort: { key: 'crit', dir: 'desc' }, defaultPageSize: 50 });

  // Deep-link from universal search (/assets?focus=<id>): derived from the URL,
  // so it resolves as soon as the asset lands in the loaded list.
  const { focusId, clearFocus } = useFocusParam();
  const focused = focusId ? assets.find((a) => a.id === focusId) : undefined;
  const editTarget = editing ?? focused;
  const closeEditor = () => {
    setEditing(undefined);
    clearFocus();
  };

  const types = useMemo(
    () => [...new Set(assets.map((a) => a.type).filter(Boolean) as string[])],
    [assets],
  );

  const facets: Facet<Asset>[] = useMemo(
    () => [
      {
        key: 'type',
        label: t(lang, 'Type', 'Type'),
        options: types.map((ty) => ({ value: ty, label: ty })),
        matches: (a, selected) => selected.includes(a.type ?? ''),
      },
      {
        key: 'criticality',
        label: t(lang, 'Criticité', 'Criticality'),
        options: (['critical', 'high', 'medium', 'low'] as const).map((c) => ({
          value: c,
          label: t(lang, CRIT_LABEL_FR[c], CRIT_LABEL_EN[c]),
          color: critColor[c],
        })),
        matches: (a, selected) => selected.includes((a.criticality ?? 'LOW').toLowerCase()),
      },
    ],
    [types, lang],
  );

  const columns: Column<Asset>[] = useMemo(
    () => [
      {
        key: 'name',
        header: t(lang, 'Actif', 'Asset'),
        frozen: true,
        hideable: false,
        sortValue: (a) => (a.name ?? '').toLowerCase(),
        exportValue: (a) => a.name ?? '',
        render: (a) => {
          const crit = (a.criticality ?? 'LOW').toLowerCase() as Criticality;
          const Icon = TYPE_ICON[a.type ?? 'Server'] ?? Server;
          return (
            <div className="flex items-center gap-2.5">
              <div
                className="w-8 h-8 rounded-[9px] flex items-center justify-center shrink-0"
                style={{ background: softFill(critColor[crit], 14), color: critColor[crit] }}
              >
                <Icon size={17} />
              </div>
              <div>
                <div className="text-[13.5px] font-medium text-ink">{a.name}</div>
                <div className="mono text-[11px] text-ink-muted">{a.owner || '—'}</div>
              </div>
            </div>
          );
        },
      },
      {
        key: 'type',
        header: 'Type',
        sortValue: (a) => a.type ?? '',
        exportValue: (a) => a.type ?? '',
        render: (a) => <span className="text-[12.5px] text-ink-soft">{a.type ?? '—'}</span>,
      },
      {
        key: 'category',
        header: t(lang, 'Catégorie', 'Category'),
        sortValue: (a) => a.category ?? '',
        exportValue: (a) => a.category ?? '',
        render: (a) =>
          a.category ? (
            <span className="text-[12.5px] text-ink-soft">
              {CATEGORY_LABELS[a.category as AssetCategory] ?? a.category}
            </span>
          ) : (
            // An untyped asset is stated as untyped rather than blank: it is the
            // difference between "no attributes" and "attributes not shown".
            <span className="text-[12px] text-ink-muted">{t(lang, 'Non typé', 'Untyped')}</span>
          ),
      },
      {
        key: 'crit',
        header: L.col_crit,
        sortValue: (a) => CRIT_RANK[(a.criticality ?? 'LOW').toLowerCase()] ?? 0,
        exportValue: (a) => a.criticality ?? '',
        render: (a) => <CritBadge crit={(a.criticality ?? 'LOW').toLowerCase() as Criticality} />,
      },
      {
        key: 'score',
        header: 'Score',
        align: 'right',
        sortValue: (a) => scoreOf(a) ?? -1,
        exportValue: (a) => scoreOf(a)?.toFixed(1) ?? '',
        render: (a) => {
          const sc = scoreOf(a);
          return sc != null ? (
            <span
              className="mono text-[14px] font-bold"
              style={{ color: critColor[(a.criticality?.toLowerCase() as Criticality) || 'low'] }}
            >
              {sc.toFixed(1)}
            </span>
          ) : (
            <span className="text-ink-muted">—</span>
          );
        },
      },
      {
        key: 'risks',
        header: t(lang, 'Risques', 'Risks'),
        align: 'right',
        sortValue: (a) => a.risks?.length ?? 0,
        exportValue: (a) => a.risks?.length ?? 0,
        render: (a) => <span className="text-[13px] text-ink">{a.risks?.length || '—'}</span>,
      },
      {
        key: 'mod',
        header: L.col_mod,
        sortValue: (a) => new Date(a.updated_at ?? 0).getTime(),
        exportValue: (a) => a.updated_at ?? '',
        render: (a) => (
          <span className="text-[12px] text-ink-soft">{relTime(a.updated_at, lang)}</span>
        ),
      },
    ],
    [lang, L],
  );

  const rowActions: RowAction<Asset>[] = useMemo(
    () => [
      {
        key: 'edit',
        label: t(lang, 'Modifier', 'Edit'),
        icon: Pencil,
        hidden: () => !canUpdate,
        onSelect: (a) => setEditing(a),
      },
      {
        key: 'history',
        label: t(lang, 'Historique', 'History'),
        icon: History,
        onSelect: (a) => setHistoryAssetId(a.id as string),
      },
      {
        key: 'delete',
        label: t(lang, 'Supprimer', 'Delete'),
        icon: Trash2,
        danger: true,
        separatorBefore: true,
        hidden: () => !canDelete,
        onSelect: (a) => setToDelete(a),
      },
    ],
    [lang, canUpdate, canDelete],
  );

  // Governed (#582): preview → confirm → one transactional, audited request.
  // Previously this fanned the selection out into one DELETE per row, which was
  // neither atomic nor attributable.
  const bulk = useGovernedBulk({
    register: 'assets',
    optimistic: { queryKey: ['assets'], apply: patchInventory },
    onApplied: (result) => {
      const n = result.applied ?? 0;
      toast.success(t(lang, `${n} actif(s) supprimé(s)`, `${n} asset(s) deleted`));
    },
  });

  const bulkActions: BulkAction<Asset>[] = useMemo(
    () => [
      {
        key: 'delete',
        label: t(lang, 'Supprimer', 'Delete'),
        icon: Trash2,
        danger: true,
        // The permission the user holds AND what the server says this register
        // can do. Enforcement is the route middleware, not either of these.
        hidden: !canDelete || !bulk.supports('delete'),
        selectionOnly: true,
        run: ({ ids }) => bulk.request({ action: 'delete' }, ids),
      },
    ],
    [lang, canDelete, bulk],
  );

  const confirmDelete = async () => {
    if (!toDelete) return;
    setDeleting(true);
    try {
      await deleteAsset.mutateAsync(toDelete.id as string);
      toast.success(t(lang, 'Actif supprimé', 'Asset deleted'));
      setToDelete(null);
    } catch {
      toast.error(t(lang, 'Suppression échouée', 'Delete failed'));
    } finally {
      setDeleting(false);
    }
  };

  return (
    <PageFrame wide>
      <PageHeader
        title={L.n_assets}
        count={`${assets.length} ${L.uniAssets}`}
        actions={
          <>
            <Btn
              label={tr('Attributs', 'Attributes')}
              icon={SlidersHorizontal}
              onClick={() => navigate('/assets/schemas')}
            />
            <Btn
              label={tr('Topologie', 'Topology')}
              icon={Atom}
              onClick={() => navigate('/assets/topology')}
            />
            <Btn
              label={tr('Nouvel actif', 'New asset')}
              icon={Plus}
              primary
              onClick={() => setCreating(true)}
            />
          </>
        }
      />

      <div className="mb-3">
        <AttributeSearchBar
          category={attrFilter.category}
          attributes={attrFilter.attributes}
          onChange={setAttrFilter}
          resultCount={assets.length}
        />
      </div>

      <DataTable
        id="assets"
        ariaLabel={L.n_assets}
        rows={assets}
        columns={columns}
        rowKey={(a) => a.id as string}
        api={table}
        mode="client"
        loading={isLoading}
        error={isError}
        onRetry={() => void refetch()}
        facets={facets}
        clientSearch={(a, q) =>
          `${a.name ?? ''} ${a.type ?? ''} ${a.owner ?? ''}`.toLowerCase().includes(q)
        }
        searchPlaceholder={tr('Nom, type ou responsable…', 'Name, type or owner…')}
        selectable
        rowActions={rowActions}
        bulkActions={bulkActions}
        onRowClick={(a) => setEditing(a)}
        exportFilename="inventaire-actifs"
        minWidth={780}
        empty={
          <EmptyState
            icon={Boxes}
            title={tr('Aucun actif inventorié', 'No assets yet')}
            description={tr(
              'Ajoutez vos serveurs, bases de données et services pour cartographier votre surface d’attaque.',
              'Add your servers, databases and services to map your attack surface.',
            )}
            primaryAction={
              <Btn
                label={tr('Nouvel actif', 'New asset')}
                icon={Plus}
                primary
                onClick={() => setCreating(true)}
              />
            }
          />
        }
      />

      <CreateAssetModal isOpen={creating} onClose={() => setCreating(false)} />
      <EditAssetModal
        asset={editTarget}
        onClose={closeEditor}
        onShowHistory={(id) => {
          closeEditor();
          setHistoryAssetId(id);
        }}
      />
      <AssetHistoryDrawer assetId={historyAssetId} onClose={() => setHistoryAssetId(null)} />

      <BulkPreviewDialog bulk={bulk} entityLabel={tr('actifs', 'assets')} />

      <ImpactDialog
        open={!!toDelete}
        title={tr('Supprimer cet actif ?', 'Delete this asset?')}
        subject={toDelete?.name ?? ''}
        description={tr(
          'Action irréversible. Voici ce qui sera supprimé :',
          'This cannot be undone. Here is what will be removed:',
        )}
        impacts={
          toDelete
            ? [
                {
                  label: tr('Risques liés', 'Linked risks'),
                  detail: String(toDelete.risks?.length ?? 0),
                },
                {
                  label: tr('Dépendances cartographiées', 'Mapped dependencies'),
                  detail: tr('supprimées', 'removed'),
                },
                {
                  label: tr('Historique des modifications', 'Change history'),
                  detail: tr('conservé mais inaccessible', 'kept but unreachable'),
                },
              ]
            : []
        }
        confirmLabel={tr('Supprimer définitivement', 'Delete permanently')}
        cancelLabel={tr('Annuler', 'Cancel')}
        loading={deleting}
        onConfirm={confirmDelete}
        onClose={() => setToDelete(null)}
      />
    </PageFrame>
  );
}
