// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// Vulnerability Management (Module 3). A tenant-scoped register of findings
// ingested from Nessus/OpenVAS/Qualys/Defender/Inspector/Azure Defender/
// CrowdStrike, risk-based prioritised (CVSS + exploitability + business
// criticality + affected assets) into P1..P4. List sorted by priority, KPI
// stats, filters, a detail drawer (status lifecycle + prioritisation breakdown),
// an ingest modal and a connectors panel.

import { useMemo, useState } from 'react';
import { useNavigate } from 'react-router';
import { toast } from 'sonner';
import {
  Bug,
  X,
  Upload,
  Plug,
  Flame,
  ShieldAlert,
  Zap,
  Trash2,
  ChevronRight,
  ChevronDown,
  Check,
  Loader2,
  Ticket,
  ExternalLink,
  Unlink,
  GitBranch,
} from 'lucide-react';
import { PageFrame, PageHeader, Btn, Card, EmptyState } from '../../shared/ui';
import {
  DataTable,
  useTableState,
  type BulkAction,
  type Column,
  type Facet,
  type RowAction,
} from '../../shared/datatable';
import { Term } from '../../shared/Term';
import { useUIStore } from '../../store/uiStore';
import { useAuthStore } from '../../hooks/useAuthStore';
import { useFocusParam } from '../../shared/useFocusParam';
import { useSoftDelete } from '../../shared/useSoftDelete';
import { useVulnerabilities, useVulnStats, useVulnMutations } from './useVulnerabilities';
import type {
  Vulnerability,
  VulnStatus,
  VulnQueryParams,
  VulnListResponse,
} from './vulnerabilityService';
import {
  SEVERITY_META,
  STATUS_META,
  STATUS_ORDER,
  TIER_META,
  SOURCE_LABEL,
  pick,
} from './vulnMeta';
import { IngestModal } from './IngestModal';
import { IntegrationsPanel } from './IntegrationsPanel';
import { safeExternalUrl } from '../../shared/safeUrl';
import {
  BulkPreviewDialog,
  useGovernedBulk,
  type BulkChangeInput,
} from '../../shared/bulk';

const t = (lang: 'fr' | 'en', fr: string, en: string) => (lang === 'fr' ? fr : en);

// CVSS, NOT the OpenRisk score. CVSS is an external 0–10 scale defined by FIRST
// and its severity cuts (9/7/4) are part of that standard, not ours. Kept local
// and named so it is never mistaken for a score band — see docs/scoring/.
/**
 * The pending change, applied to a cached page of the register (ABSOLUTE RULE
 * 10). Restored verbatim by the hook if the server refuses — criterion 8.
 *
 * `cached` arrives untyped because the ['vulnerabilities'] prefix also holds the
 * KPI stats and the connector list. Anything that is not a page of the register
 * is handed back untouched rather than guessed at.
 */
function patchVulnPage(
  cached: unknown,
  ids: ReadonlySet<string>,
  change: BulkChangeInput,
): unknown {
  if (!isVulnPage(cached)) return cached;

  if (change.action === 'delete') {
    const items = cached.items.filter((v) => !ids.has(v.id));
    return { ...cached, items, total: Math.max(0, cached.total - (cached.items.length - items.length)) };
  }

  const status = change.status;
  if (status === undefined || !(status in STATUS_META)) return cached;
  return {
    ...cached,
    items: cached.items.map((v) =>
      ids.has(v.id) ? { ...v, status: status as VulnStatus } : v,
    ),
  };
}

function isVulnPage(cached: unknown): cached is VulnListResponse {
  if (typeof cached !== 'object' || cached === null) return false;
  const candidate = cached as { items?: unknown; total?: unknown };
  return Array.isArray(candidate.items) && typeof candidate.total === 'number';
}

const cvssColor = (s: number) =>
  s >= 9 ? 'var(--critical)' : s >= 7 ? 'var(--high)' : s >= 4 ? 'var(--medium)' : 'var(--low)';

export function VulnerabilitiesPage() {
  const navigate = useNavigate();
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const canWrite = useAuthStore((s) => s.hasPermission('vulnerabilities:update'));
  const canDelete = useAuthStore((s) => s.hasPermission('vulnerabilities:delete'));

  const [drawerId, setDrawerId] = useState<string | null>(null);
  const [ingestOpen, setIngestOpen] = useState(false);
  const [connectorsOpen, setConnectorsOpen] = useState(false);

  // URL-backed table state; the API does the sorting, filtering and paging.
  const table = useTableState({
    defaultSort: { key: 'priority_score', dir: 'desc' },
    defaultPageSize: 50,
  });
  const { state } = table;

  const params: VulnQueryParams = useMemo(() => {
    const p: VulnQueryParams = { page: state.page, limit: state.pageSize };
    if (state.sort) {
      p.sort_by = state.sort.key;
      p.sort_dir = state.sort.dir;
    }
    if (state.q.trim()) p.q = state.q.trim();
    const tier = state.filters.tier?.[0];
    const severity = state.filters.severity?.[0];
    const status = state.filters.status?.[0];
    const source = state.filters.source?.[0];
    if (tier) p.tier = tier;
    if (severity) p.severity = severity;
    if (status) p.status = status;
    if (source) p.source = source;
    if (state.filters.kev?.includes('true')) p.kev = true;
    return p;
  }, [state]);

  const { data, isLoading, isError, refetch } = useVulnerabilities(params);
  const { data: stats } = useVulnStats();
  const { remove, updateStatus } = useVulnMutations();
  // Soft delete: hide the row immediately + Undo toast; the API delete defers.
  const { pending, remove: softDeleteVuln } = useSoftDelete<Vulnerability>({
    onCommit: (id) => remove.mutateAsync(id),
    message: (v, l) =>
      l === 'fr' ? `Vulnérabilité « ${v.title} » supprimée` : `Vulnerability "${v.title}" deleted`,
  });
  const items = useMemo(
    () => (data?.items ?? []).filter((v) => !pending.has(v.id)),
    [data, pending],
  );

  // Deep-link from universal search (/vulnerabilities?focus=<id>): the drawer is
  // *derived* from the URL rather than copied into state by an effect, so the
  // link resolves as soon as the row lands in the list — no render cascade.
  const { focusId, clearFocus } = useFocusParam();
  const activeId = drawerId ?? focusId;
  const drawer = activeId ? (items.find((v) => v.id === activeId) ?? null) : null;
  const closeDrawer = () => {
    setDrawerId(null);
    clearFocus();
  };

  /* --------------------------------------------------------------- facets */
  // The backend takes one value per facet, so these are single-choice — the
  // panel renders them as such rather than letting the user tick two values
  // and silently applying one.
  const facets: Facet<Vulnerability>[] = useMemo(
    () => [
      {
        key: 'tier',
        label: t(lang, 'Priorité', 'Priority'),
        single: true,
        options: (['P1', 'P2', 'P3', 'P4'] as const).map((t) => ({
          value: t,
          label: t,
          color: TIER_META[t].color,
        })),
      },
      {
        key: 'severity',
        label: t(lang, 'Sévérité', 'Severity'),
        single: true,
        options: (['critical', 'high', 'medium', 'low'] as const).map((sev) => ({
          value: sev,
          label: pick(SEVERITY_META[sev].label, lang),
          color: SEVERITY_META[sev].color,
        })),
      },
      {
        key: 'status',
        label: t(lang, 'Statut', 'Status'),
        single: true,
        options: STATUS_ORDER.map((st) => ({
          value: st,
          label: pick(STATUS_META[st].label, lang),
          color: STATUS_META[st].color,
        })),
      },
      {
        key: 'kev',
        label: t(lang, 'Exploitation connue', 'Known exploitation'),
        single: true,
        options: [{ value: 'true', label: 'CISA-KEV', color: 'var(--critical)' }],
      },
    ],
    [lang],
  );

  /* -------------------------------------------------------------- columns */
  const columns: Column<Vulnerability>[] = useMemo(
    () => [
      {
        key: 'priority',
        header: t(lang, 'Priorité', 'Priority'),
        frozen: true,
        hideable: false,
        sortKey: 'priority_score',
        exportValue: (v) => `${v.priority_tier} (${v.priority_score.toFixed(0)})`,
        render: (v) => (
          <div className="inline-flex items-center gap-2">
            <span
              className="inline-flex items-center justify-center h-[22px] px-2 rounded-[6px] text-[11.5px] font-bold"
              style={{
                background: TIER_META[v.priority_tier]?.color ?? 'var(--low)',
                color: 'var(--fg-inverse)',
              }}
            >
              {v.priority_tier}
            </span>
            <span className="mono text-[13px] font-bold text-ink">
              {v.priority_score.toFixed(0)}
            </span>
            {v.kev && <Flame size={13} style={{ color: 'var(--critical)' }} />}
          </div>
        ),
      },
      {
        key: 'cve',
        header: 'CVE',
        sortKey: 'cve_id',
        exportValue: (v) => v.cve_id || '',
        render: (v) => <span className="mono text-[12.5px] text-ink">{v.cve_id || '—'}</span>,
      },
      {
        key: 'title',
        header: t(lang, 'Titre', 'Title'),
        exportValue: (v) => v.title,
        render: (v) => <div className="text-[13px] text-ink max-w-[280px] truncate">{v.title}</div>,
      },
      {
        key: 'severity',
        header: t(lang, 'Sévérité', 'Severity'),
        sortKey: 'severity',
        exportValue: (v) => v.severity,
        render: (v) => <SevBadge sev={v.severity} lang={lang} />,
      },
      {
        key: 'cvss',
        header: 'CVSS',
        align: 'right',
        sortKey: 'cvss_score',
        exportValue: (v) => (v.cvss_score ? v.cvss_score.toFixed(1) : ''),
        render: (v) => (
          <span
            className="mono text-[13px] font-semibold"
            style={{ color: cvssColor(v.cvss_score) }}
          >
            {v.cvss_score ? v.cvss_score.toFixed(1) : '—'}
          </span>
        ),
      },
      {
        key: 'asset',
        header: t(lang, 'Actif', 'Asset'),
        exportValue: (v) => v.asset_name || '',
        render: (v) => <span className="text-[12.5px] text-ink-soft">{v.asset_name || '—'}</span>,
      },
      {
        key: 'source',
        header: t(lang, 'Source', 'Source'),
        exportValue: (v) => v.source,
        render: (v) => (
          <span className="text-[12px] text-ink-muted">{SOURCE_LABEL[v.source] ?? v.source}</span>
        ),
      },
      {
        key: 'status',
        header: t(lang, 'Statut', 'Status'),
        exportValue: (v) => v.status,
        render: (v) =>
          canWrite ? <InlineVulnStatus v={v} /> : <StatusChip status={v.status} lang={lang} />,
      },
    ],
    [lang, canWrite],
  );

  /* ---------------------------------------------------------- row actions */
  const rowActions: RowAction<Vulnerability>[] = useMemo(
    () => [
      {
        key: 'view',
        label: t(lang, 'Voir le détail', 'View details'),
        icon: ChevronRight,
        onSelect: (v) => setDrawerId(v.id),
      },
      {
        key: 'triage',
        label: t(lang, 'Marquer « en remédiation »', 'Mark "in remediation"'),
        icon: Zap,
        hidden: () => !canWrite,
        disabled: (v) => v.status === 'in_remediation',
        onSelect: async (v) => {
          try {
            await updateStatus.mutateAsync({ id: v.id, status: 'in_remediation' });
            toast.success(t(lang, 'Statut mis à jour', 'Status updated'));
          } catch {
            toast.error(t(lang, 'Échec', 'Failed'));
          }
        },
      },
      {
        key: 'delete',
        label: t(lang, 'Supprimer', 'Delete'),
        icon: Trash2,
        danger: true,
        separatorBefore: true,
        hidden: () => !canDelete,
        onSelect: (v) => softDeleteVuln(v),
      },
    ],
    [canWrite, canDelete, softDeleteVuln, updateStatus, lang],
  );

  /* --------------------------------------------------------- bulk actions */
  // Governed (#582): every bulk action goes preview → confirm → transactional,
  // audited apply on the server. The register no longer fans a selection out
  // into one request per row — that was neither atomic nor attributable.
  const bulk = useGovernedBulk({
    register: 'vulnerabilities',
    optimistic: {
      // Narrow: the ['vulnerabilities'] prefix also caches stats and connectors,
      // which this patch has no business rewriting.
      queryKey: ['vulnerabilities', 'list'],
      apply: patchVulnPage,
      // Wide: a status change moves the KPI counts too.
      invalidateKey: ['vulnerabilities'],
    },
    onApplied: (result, change) => {
      const n = result.applied ?? 0;
      toast.success(
        change.action === 'delete'
          ? t(lang, `${n} vulnérabilité(s) supprimée(s)`, `${n} vulnerability(ies) deleted`)
          : t(lang, `${n} vulnérabilité(s) mise(s) à jour`, `${n} vulnerability(ies) updated`),
      );
    },
  });

  const bulkActions: BulkAction<Vulnerability>[] = useMemo(
    () => [
      {
        key: 'remediating',
        label: t(lang, 'Marquer « en remédiation »', 'Mark "in remediation"'),
        icon: Zap,
        // Two gates, and both are needed: the permission the user holds, and
        // what the server says this register can do. Neither is the enforcement
        // — that is the route middleware (criterion 4).
        hidden: !canWrite || !bulk.supports('change_status'),
        selectionOnly: true,
        run: ({ ids }) => bulk.request({ action: 'change_status', status: 'in_remediation' }, ids),
      },
      {
        key: 'delete',
        label: t(lang, 'Supprimer', 'Delete'),
        icon: Trash2,
        danger: true,
        hidden: !canDelete || !bulk.supports('delete'),
        selectionOnly: true,
        run: ({ ids }) => bulk.request({ action: 'delete' }, ids),
      },
    ],
    [canWrite, canDelete, bulk, lang],
  );

  const kpi = (
    label: React.ReactNode,
    value: number | string,
    color: string,
    Icon: typeof Flame,
  ) => (
    <Card style={{ padding: '14px 16px', flex: 1, minWidth: 130 }}>
      <div className="flex items-center gap-2 text-ink-muted text-[11px] font-semibold uppercase tracking-[.04em]">
        <Icon size={13} style={{ color }} /> {label}
      </div>
      <div className="mono text-[24px] font-bold text-ink mt-1" style={{ color }}>
        {value}
      </div>
    </Card>
  );

  return (
    <PageFrame wide>
      <PageHeader
        title={tr('Vulnérabilités', 'Vulnerabilities')}
        count={`${stats?.total ?? 0} ${tr('vulnérabilités', 'vulnerabilities')}`}
        actions={
          <>
            <Btn
              label={tr('Non rattachées', 'Unassigned')}
              icon={Unlink}
              onClick={() => navigate('/vulnerabilities/unassigned')}
            />
            <Btn
              label={tr('Règle → risque', 'Risk rule')}
              icon={GitBranch}
              onClick={() => navigate('/vulnerabilities/risk-rule')}
            />
            <Btn
              label={tr('Intégrations', 'Integrations')}
              icon={Plug}
              onClick={() => setConnectorsOpen(true)}
            />
            <Btn
              label={tr('Importer', 'Import')}
              icon={Upload}
              primary
              onClick={() => setIngestOpen(true)}
            />
          </>
        }
      />

      {/* KPI row */}
      <div className="flex gap-3 mb-4 flex-wrap">
        {kpi(tr('Total', 'Total'), stats?.total ?? 0, 'var(--accent)', Bug)}
        {kpi(tr('Ouvertes', 'Open'), stats?.open ?? 0, 'var(--high)', ShieldAlert)}
        {kpi('P1', stats?.by_tier?.P1 ?? 0, 'var(--critical)', Flame)}
        {kpi(<Term term="KEV">CISA-KEV</Term>, stats?.kev_count ?? 0, 'var(--critical)', Flame)}
        {kpi(tr('Exploitables', 'Exploitable'), stats?.exploit_count ?? 0, 'var(--high)', Zap)}
      </div>

      <DataTable
        id="vulnerabilities"
        ariaLabel={tr('Vulnérabilités', 'Vulnerabilities')}
        rows={items}
        total={data?.total ?? items.length}
        columns={columns}
        rowKey={(v) => v.id}
        api={table}
        mode="server"
        loading={isLoading}
        error={isError}
        onRetry={() => void refetch()}
        facets={facets}
        searchPlaceholder={tr('CVE, titre ou description…', 'CVE, title or description…')}
        selectable
        rowActions={rowActions}
        bulkActions={bulkActions}
        onRowClick={(v) => setDrawerId(v.id)}
        exportFilename="vulnerabilites"
        minWidth={960}
        empty={
          <EmptyState
            icon={Bug}
            title={tr('Aucune vulnérabilité', 'No vulnerabilities')}
            description={tr(
              'Importez des findings depuis Nessus, Qualys, Defender, Inspector, CrowdStrike…',
              'Import findings from Nessus, Qualys, Defender, Inspector, CrowdStrike…',
            )}
            primaryAction={
              <Btn
                label={tr('Importer', 'Import')}
                icon={Upload}
                primary
                onClick={() => setIngestOpen(true)}
              />
            }
          />
        }
      />

      {drawer && (
        <VulnDrawer
          v={drawer}
          onClose={closeDrawer}
          onDelete={() => {
            const d = drawer;
            closeDrawer();
            softDeleteVuln(d);
          }}
        />
      )}
      <BulkPreviewDialog bulk={bulk} entityLabel={tr('vulnérabilités', 'vulnerabilities')} />
      <IngestModal isOpen={ingestOpen} onClose={() => setIngestOpen(false)} />
      <IntegrationsPanel
        isOpen={connectorsOpen}
        onClose={() => setConnectorsOpen(false)}
        onImport={() => {
          setConnectorsOpen(false);
          setIngestOpen(true);
        }}
      />
    </PageFrame>
  );
}

function SevBadge({ sev, lang }: { sev: Vulnerability['severity']; lang: 'fr' | 'en' }) {
  const m = SEVERITY_META[sev] ?? SEVERITY_META.info;
  return (
    <span
      className="inline-flex items-center h-[20px] px-2 rounded-full text-[11px] font-semibold"
      style={{ color: m.color, background: `color-mix(in srgb, ${m.color} 14%, transparent)` }}
    >
      {pick(m.label, lang)}
    </span>
  );
}

// Click-to-edit status right in the row (ghost edit + optimistic autosave + toast).
function InlineVulnStatus({ v }: { v: Vulnerability }) {
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const { updateStatus } = useVulnMutations();
  const [open, setOpen] = useState(false);
  const [saving, setSaving] = useState(false);
  const change = async (s: VulnStatus) => {
    setOpen(false);
    if (s === v.status) return;
    setSaving(true);
    try {
      await updateStatus.mutateAsync({ id: v.id, status: s });
      toast.success(tr('Statut mis à jour', 'Status updated'));
    } catch {
      toast.error(tr('Échec', 'Failed'));
    } finally {
      setSaving(false);
    }
  };
  return (
    <div className="relative inline-block" onClick={(e) => e.stopPropagation()}>
      <button
        onClick={() => setOpen((o) => !o)}
        disabled={saving}
        title={tr('Changer le statut', 'Change status')}
        className="inline-flex items-center gap-1 rounded-md px-1.5 py-1 -mx-1 hover:bg-hover transition-colors"
      >
        <StatusChip status={v.status} lang={lang} />
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
            className="absolute left-0 top-full mt-1 z-50 min-w-[170px] rounded-[10px] p-1 shadow-card-lg"
            style={{ background: 'var(--bg-elevated)', border: '1px solid var(--border)' }}
          >
            {STATUS_ORDER.map((s) => (
              <button
                key={s}
                onClick={() => change(s)}
                className="w-full flex items-center justify-between gap-2 px-2 py-1.5 rounded-[7px] hover:bg-hover transition-colors text-left"
              >
                <StatusChip status={s} lang={lang} />
                {s === v.status && <Check size={13} className="text-accent" />}
              </button>
            ))}
          </div>
        </>
      )}
    </div>
  );
}

function StatusChip({ status, lang }: { status: VulnStatus; lang: 'fr' | 'en' }) {
  const m = STATUS_META[status];
  return (
    <span
      className="inline-flex items-center gap-1.5 text-[12px] font-medium"
      style={{ color: m.color }}
    >
      <span className="w-1.5 h-1.5 rounded-full" style={{ background: m.color }} />{' '}
      {pick(m.label, lang)}
    </span>
  );
}

/* ---------------- drawer ---------------- */
function VulnDrawer({
  v,
  onClose,
  onDelete,
}: {
  v: Vulnerability;
  onClose: () => void;
  onDelete: () => void;
}) {
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const canWrite = useAuthStore((s) => s.hasPermission('vulnerabilities:update'));
  const canDelete = useAuthStore((s) => s.hasPermission('vulnerabilities:delete'));
  const { updateStatus, createTicket } = useVulnMutations();

  const openTicket = async () => {
    try {
      await createTicket.mutateAsync(v.id);
      toast.success(tr('Ticket ouvert', 'Ticket opened'));
    } catch (e) {
      const msg =
        e instanceof Error
          ? e.message
          : tr('Échec — ITSM configuré ?', 'Failed — is ITSM configured?');
      toast.error(msg);
    }
  };

  const setStatus = async (status: VulnStatus) => {
    try {
      await updateStatus.mutateAsync({ id: v.id, status });
      toast.success(tr('Statut mis à jour', 'Status updated'));
    } catch {
      toast.error(tr('Échec', 'Failed'));
    }
  };
  // Soft delete lives in the page (Undo toast + deferred API call); the drawer just
  // hands off to it.
  const del = () => onDelete();

  const field = (lbl: React.ReactNode, val: React.ReactNode) => (
    <div className="mb-3.5">
      <div className="text-[11px] font-semibold uppercase tracking-[.04em] text-ink-muted mb-1">
        {lbl}
      </div>
      <div className="text-[13.5px] text-ink">{val}</div>
    </div>
  );
  const signal = (on: boolean, label: string) => (
    <span
      className="inline-flex items-center gap-1.5 h-[24px] px-2.5 rounded-full text-[12px] font-semibold"
      style={{
        color: on ? 'var(--critical)' : 'var(--fg-muted)',
        background: on ? 'color-mix(in srgb,var(--critical) 12%,transparent)' : 'var(--bg-hover)',
      }}
    >
      {label}
    </span>
  );

  return (
    <div
      className="fixed inset-0 z-70 flex justify-end"
      style={{ background: 'rgba(0,0,0,.45)', backdropFilter: 'blur(3px)' }}
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
        <div className="px-[22px] pt-5 pb-3.5" style={{ borderBottom: '1px solid var(--border)' }}>
          <div className="flex items-start gap-3 mb-3">
            <div className="flex-1">
              <div className="mono text-[12px] text-ink-muted mb-1">
                {v.cve_id || v.external_id || '—'}
              </div>
              <div className="disp text-[17px] font-bold text-ink leading-snug">{v.title}</div>
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
            <span
              className="inline-flex items-center h-[24px] px-2.5 rounded-[7px] text-[12px] font-bold"
              style={{ background: TIER_META[v.priority_tier]?.color, color: '#12151c' }}
            >
              {v.priority_tier} · {v.priority_score.toFixed(0)}
            </span>
            <SevBadge sev={v.severity} lang={lang} />
            <StatusChip status={v.status} lang={lang} />
          </div>
        </div>

        <div className="flex-1 overflow-y-auto px-[22px] py-5">
          {/* Prioritisation breakdown */}
          <div
            className="rounded-[12px] p-4 mb-4"
            style={{
              border: '1px solid color-mix(in srgb,var(--accent) 30%,transparent)',
              background: 'color-mix(in srgb,var(--accent) 6%,transparent)',
            }}
          >
            <div className="text-[11px] font-semibold uppercase tracking-[.04em] text-ink-muted mb-1.5">
              {tr('Priorisation (risk-based)', 'Prioritisation (risk-based)')}
            </div>
            <div className="text-[13.5px] text-ink">
              {v.priority_explanation ||
                tr(
                  'Score dérivé du CVSS, de l’exploitabilité, de la criticité métier et des actifs concernés.',
                  'Score from CVSS, exploitability, business criticality and affected assets.',
                )}
            </div>
          </div>

          <div className="grid grid-cols-2 gap-x-5">
            {field(
              <Term term="CVSS">CVSS</Term>,
              <span className="mono font-semibold" style={{ color: cvssColor(v.cvss_score) }}>
                {v.cvss_score ? v.cvss_score.toFixed(1) : '—'}
                {v.cvss_vector ? ` · ${v.cvss_vector}` : ''}
              </span>,
            )}
            {field(<Term term="EPSS">EPSS</Term>, v.epss ? `${(v.epss * 100).toFixed(1)}%` : '—')}
            {field(
              tr('Actif concerné', 'Affected asset'),
              v.asset_name
                ? `${v.asset_name}${v.asset_criticality ? ` · ${v.asset_criticality}` : ''}`
                : '—',
            )}
            {field(tr('Actifs concernés', 'Affected assets'), String(v.affected_assets_count))}
            {field(tr('Source', 'Source'), SOURCE_LABEL[v.source] ?? v.source)}
            {field(
              tr('Vu pour la dernière fois', 'Last seen'),
              v.last_seen ? new Date(v.last_seen).toLocaleDateString() : '—',
            )}
          </div>

          <div className="flex gap-2 flex-wrap mb-4">
            {signal(v.kev, 'CISA-KEV')}
            {signal(v.exploit_available, tr('Exploit public', 'Public exploit'))}
            {v.exploit_maturity ? signal(true, `Maturité: ${v.exploit_maturity}`) : null}
          </div>

          {v.description
            ? field('Description', <span className="text-ink-soft">{v.description}</span>)
            : null}
          {v.remediation_hint
            ? field(
                tr('Remédiation', 'Remediation'),
                <span className="text-ink-soft">{v.remediation_hint}</span>,
              )
            : null}

          {/* Cross-module linkage: ITSM ticket + auto-created risk */}
          <div className="rounded-[12px] p-3.5 mb-4" style={{ border: '1px solid var(--border)' }}>
            <div className="flex items-center gap-2 mb-2">
              <Ticket size={14} className="text-ink-muted" />
              <span className="text-[12.5px] font-semibold text-ink">
                {tr('Ticket ITSM', 'ITSM ticket')}
              </span>
            </div>
            {v.ticket_key ? (
              // The ITSM base URL comes from tenant-configured integration data,
              // so the href is allowlisted to http(s) rather than trusted.
              <a
                href={safeExternalUrl(v.ticket_url) ?? '#'}
                target="_blank"
                rel="noreferrer"
                className="inline-flex items-center gap-1.5 text-[13px] font-semibold"
                style={{ color: 'var(--accent-500)' }}
              >
                {v.ticket_key} <ExternalLink size={13} />
              </a>
            ) : canWrite ? (
              <button
                onClick={openTicket}
                disabled={createTicket.isPending}
                className="h-8 px-3 rounded-[9px] text-[12.5px] font-semibold inline-flex items-center gap-1.5 disabled:opacity-60"
                style={{ border: '1px solid var(--border-strong)', color: 'var(--accent-500)' }}
              >
                <Ticket size={13} /> {tr('Ouvrir un ticket', 'Open a ticket')}
              </button>
            ) : (
              <span className="text-[12.5px] text-ink-muted">
                {tr('Aucun ticket', 'No ticket')}
              </span>
            )}
            {v.risk_id && (
              <div
                className="mt-2.5 pt-2.5 flex items-center gap-1.5 text-[12px] text-ink-soft"
                style={{ borderTop: '1px solid var(--border)' }}
              >
                <ShieldAlert size={13} style={{ color: 'var(--high)' }} />
                {tr('Risque auto-créé lié', 'Linked auto-created risk')}
              </div>
            )}
          </div>

          {/* Status lifecycle */}
          {canWrite && (
            <div className="mt-2">
              <div className="text-[11px] font-semibold uppercase tracking-[.04em] text-ink-muted mb-2">
                {tr('Changer le statut', 'Change status')}
              </div>
              <div className="flex flex-wrap gap-2">
                {STATUS_ORDER.filter((s) => s !== v.status).map((s) => (
                  <button
                    key={s}
                    disabled={updateStatus.isPending}
                    onClick={() => setStatus(s)}
                    className="h-8 px-3 rounded-[9px] text-[12.5px] font-semibold inline-flex items-center gap-1.5 disabled:opacity-60"
                    style={{
                      border: '1px solid var(--border-strong)',
                      color: STATUS_META[s].color,
                    }}
                  >
                    <ChevronRight size={13} /> {pick(STATUS_META[s].label, lang)}
                  </button>
                ))}
              </div>
            </div>
          )}
        </div>

        {canDelete && (
          <div className="px-[22px] py-3.5" style={{ borderTop: '1px solid var(--border)' }}>
            <button
              onClick={del}
              className="h-9 px-3 rounded-[9px] text-[12.5px] font-semibold inline-flex items-center gap-1.5"
              style={{
                color: 'var(--critical)',
                background: 'color-mix(in srgb,var(--critical) 12%,transparent)',
              }}
            >
              <Trash2 size={14} /> {tr('Supprimer', 'Delete')}
            </button>
          </div>
        )}
      </div>
    </div>
  );
}
