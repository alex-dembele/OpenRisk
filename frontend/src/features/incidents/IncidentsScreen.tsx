// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// Incidents register (routed at /incidents) — the real, backend-wired incident
// hub: KPI header (from /incidents/stats), status filter chips, a table with
// inline status change + row click to open the detail/edit drawer, CSV export,
// and a create dialog. The War Room console lives at /incidents/:id/war-room
// and is reachable per-incident; since W0-05 everything on it is real — the
// incident context, the timeline, the responders, the persisted response task
// board and the Close action.

import { useMemo, useState } from 'react';
import { useNavigate } from 'react-router';
import { toast } from 'sonner';
import {
  Siren,
  Plus,
  Trash2,
  Radio,
  ShieldAlert,
  CheckCircle2,
  Activity,
  X,
  Loader2,
  Download,
} from 'lucide-react';
import { PageFrame, PageHeader, Btn, Card, EmptyState, softFill } from '../../shared/ui';
import {
  DataTable,
  useTableState,
  type BulkAction,
  type Column,
  type Facet,
  type RowAction,
} from '../../shared/datatable';
import { useUIStore } from '../../store/uiStore';
import { useAuthStore } from '../../hooks/useAuthStore';
import { relTime } from '../risks/riskMap';
import { useSoftDelete } from '../../shared/useSoftDelete';
import { useIncidents, useIncidentStats } from './useIncidents';
import { IncidentDrawer } from './IncidentDrawer';
import { SEV, STATUS, STATUSES, SEVERITIES, TYPES, sevMeta, statusMeta } from './incidentMeta';
import { exportIncidentsCsv } from './incidentService';
import type { Incident, IncidentStatus } from './incidentService';
import { DeclareIncidentModal } from './DeclareIncidentModal';
import { OriginChip } from './IncidentProvenance';

export function IncidentsScreen() {
  const lang = useUIStore((s) => s.lang);
  const navigate = useNavigate();
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);

  // Gate on the permission the SERVER actually enforces, not on a role name.
  //
  // This was hasRole('admin') || hasRole('analyst'). The backend abandoned that
  // exact gate — cmd/server/main.go says so in as many words: "analyst" is not a
  // runtime org role (they are root/admin/user), so the check was effectively
  // admin-only and could never be granted to a business role. It moved to the
  // incidents:* permission family; the client never followed.
  //
  // The consequence was not cosmetic. A tenant's founding user carries
  // permissions ["*"] and an EMPTY role string, so hasRole('admin') was false and
  // the owner of the workspace could not declare an incident on their own
  // incident screen — while POST /incidents would have accepted it.
  const hasPermission = useAuthStore((s) => s.hasPermission);
  // Three permissions, because the server enforces three. One boolean for all of
  // them would hide Delete from someone who may resolve, or offer Delete to
  // someone the API will refuse.
  const canCreate = hasPermission('incidents:create');
  const canUpdate = hasPermission('incidents:update');
  const canDelete = hasPermission('incidents:delete');

  const [showCreate, setShowCreate] = useState(false);
  const [selected, setSelected] = useState<Incident | null>(null);
  const [exporting, setExporting] = useState(false);

  // URL-backed table state. GET /incidents has filters but no pagination, so the
  // table runs in client mode over the loaded register.
  const table = useTableState({
    defaultSort: { key: 'reported', dir: 'desc' },
    defaultPageSize: 50,
  });
  const statusFilter = (table.state.filters.status?.[0] ?? '') as '' | IncidentStatus;

  const { data: stats } = useIncidentStats();
  const { incidents, isLoading, isError, refetch, updateIncident, deleteIncident } = useIncidents(
    statusFilter ? { status: statusFilter } : {},
  );

  const setStatus = (inc: Incident, status: IncidentStatus) => {
    if (status === inc.status) return;
    updateIncident.mutate(
      { id: inc.id, input: { status } },
      { onError: () => toast.error(tr('Mise à jour échouée', 'Update failed')) },
    );
  };

  // Soft delete: hide the row immediately + Undo toast; the API delete defers.
  // (Incident ids are numeric; the hook keys on strings.)
  const { pending, remove: softDeleteIncident } = useSoftDelete<Incident>({
    idOf: (inc) => String(inc.id),
    onCommit: (id) => deleteIncident.mutateAsync(Number(id)),
    message: (inc, lang) =>
      lang === 'fr' ? `Incident « ${inc.title} » supprimé` : `Incident "${inc.title}" deleted`,
  });
  const visibleIncidents = useMemo(
    () => incidents.filter((inc) => !pending.has(String(inc.id))),
    [incidents, pending],
  );
  const remove = (inc: Incident) => softDeleteIncident(inc);

  const exportCsv = async () => {
    setExporting(true);
    try {
      const n = await exportIncidentsCsv(statusFilter ? { status: statusFilter } : {});
      toast.success(tr(`${n} incident(s) exporté(s)`, `${n} incident(s) exported`));
    } catch {
      toast.error(tr('Export échoué', 'Export failed'));
    } finally {
      setExporting(false);
    }
  };

  const openWarRoom = () => {
    const target =
      incidents.find((i) => i.status === 'open' || i.status === 'in_progress') ?? incidents[0];
    if (!target) {
      toast.message(tr('Aucun incident à ouvrir', 'No incident to open'));
      return;
    }
    navigate(`/incidents/${target.id}/war-room`);
  };

  const kpis = [
    {
      icon: Siren,
      label: tr('Total', 'Total'),
      value: stats?.total_incidents ?? 0,
      color: 'var(--accent-500)',
    },
    {
      icon: Radio,
      label: tr('Ouverts', 'Open'),
      value: stats?.open_incidents ?? 0,
      color: 'var(--critical)',
    },
    {
      icon: ShieldAlert,
      label: tr('Critiques', 'Critical'),
      value: stats?.critical_incidents ?? 0,
      color: 'var(--high)',
    },
    {
      icon: CheckCircle2,
      label: tr('Taux de résolution', 'Resolution rate'),
      value: `${Math.round(stats?.resolution_rate ?? 0)}%`,
      color: 'var(--low)',
    },
  ];

  /* --------------------------------------------------------------- facets */
  const facets: Facet<Incident>[] = useMemo(
    () => [
      {
        // Single-choice: this facet is pushed to the API (?status=), which takes one.
        key: 'status',
        label: tr('Statut', 'Status'),
        single: true,
        options: STATUSES.map((st) => ({
          value: st,
          label: tr(STATUS[st].fr, STATUS[st].en),
          color: STATUS[st].color,
        })),
        matches: () => true,
      },
      {
        key: 'severity',
        label: tr('Sévérité', 'Severity'),
        options: SEVERITIES.map((sv) => ({
          value: sv,
          label: tr(SEV[sv].fr, SEV[sv].en),
          color: SEV[sv].color,
        })),
        matches: (inc, selectedValues) => selectedValues.includes(inc.severity),
      },
      {
        key: 'type',
        label: tr('Type', 'Type'),
        options: TYPES.map((ty) => ({ value: ty, label: ty.replace('_', ' ') })),
        matches: (inc, selectedValues) => selectedValues.includes(inc.incident_type ?? ''),
      },
    ],
    [lang],
  ); // eslint-disable-line react-hooks/exhaustive-deps

  /* -------------------------------------------------------------- columns */
  const columns: Column<Incident>[] = useMemo(
    () => [
      {
        key: 'title',
        header: tr('Incident', 'Incident'),
        frozen: true,
        hideable: false,
        sortValue: (inc) => inc.title.toLowerCase(),
        exportValue: (inc) => inc.title,
        render: (inc) => (
          <>
            <div className="flex items-center gap-2 flex-wrap">
              <span className="text-[13.5px] font-medium text-ink">{inc.title}</span>
              {/* Says at a glance that nobody declared this one. */}
              <OriginChip incident={inc} />
            </div>
            {inc.description && (
              <div className="text-[12px] text-ink-muted mt-0.5 max-w-[420px] leading-snug truncate">
                {inc.description}
              </div>
            )}
          </>
        ),
      },
      {
        key: 'type',
        header: tr('Type', 'Type'),
        sortValue: (inc) => inc.incident_type ?? '',
        exportValue: (inc) => inc.incident_type ?? '',
        render: (inc) => (
          <span className="text-[12px] text-ink-soft capitalize">
            {inc.incident_type?.replace('_', ' ') || '—'}
          </span>
        ),
      },
      {
        key: 'severity',
        header: tr('Sévérité', 'Severity'),
        sortValue: (inc) => SEVERITIES.indexOf(inc.severity),
        exportValue: (inc) => inc.severity,
        render: (inc) => (
          <span
            className="inline-flex items-center gap-1.5 text-[11.5px] font-semibold px-[9px] py-[3px] rounded-full"
            style={{
              color: sevMeta(inc.severity).color,
              background: softFill(sevMeta(inc.severity).color, 15),
            }}
          >
            <span
              className="w-1.5 h-1.5 rounded-full"
              style={{ background: sevMeta(inc.severity).color }}
            />
            {tr(sevMeta(inc.severity).fr, sevMeta(inc.severity).en)}
          </span>
        ),
      },
      {
        key: 'status',
        header: tr('Statut', 'Status'),
        sortValue: (inc) => inc.status,
        exportValue: (inc) => inc.status,
        render: (inc) => (
          <div className="relative inline-flex items-center" onClick={(e) => e.stopPropagation()}>
            <span
              className="w-2 h-2 rounded-full absolute left-2.5 pointer-events-none"
              style={{ background: statusMeta(inc.status).color }}
            />
            <select
              value={inc.status}
              disabled={!canUpdate}
              aria-label={tr('Statut de l’incident', 'Incident status')}
              onChange={(e) => setStatus(inc, e.target.value as IncidentStatus)}
              className="appearance-none text-[12px] font-semibold rounded-full pl-6 pr-6 py-1.5 outline-none disabled:opacity-70"
              style={{
                color: statusMeta(inc.status).color,
                background: `color-mix(in srgb,${statusMeta(inc.status).color} 12%,transparent)`,
                border: `1px solid color-mix(in srgb,${statusMeta(inc.status).color} 30%,transparent)`,
                cursor: canUpdate ? 'pointer' : 'not-allowed',
              }}
            >
              {STATUSES.map((st) => (
                <option
                  key={st}
                  value={st}
                  style={{ color: 'var(--fg-primary)', background: 'var(--bg-elevated)' }}
                >
                  {tr(STATUS[st].fr, STATUS[st].en)}
                </option>
              ))}
            </select>
          </div>
        ),
      },
      {
        key: 'reported',
        header: tr('Signalé', 'Reported'),
        sortValue: (inc) => new Date(inc.created_at ?? 0).getTime(),
        exportValue: (inc) => inc.created_at ?? '',
        render: (inc) => (
          <>
            <div className="text-[12.5px] text-ink-soft">{relTime(inc.created_at, lang)}</div>
            {inc.reported_by && (
              <div className="text-[11.5px] text-ink-muted">{inc.reported_by}</div>
            )}
          </>
        ),
      },
    ],
    [lang, canUpdate],
  ); // eslint-disable-line react-hooks/exhaustive-deps

  /* -------------------------------------------------------------- actions */
  const rowActions: RowAction<Incident>[] = useMemo(
    () => [
      {
        key: 'open',
        label: tr('Ouvrir', 'Open'),
        icon: Activity,
        onSelect: (inc) => setSelected(inc),
      },
      {
        key: 'warroom',
        label: tr('War Room', 'War Room'),
        icon: Radio,
        onSelect: (inc) => navigate(`/incidents/${inc.id}/war-room`),
      },
      {
        key: 'delete',
        label: tr('Supprimer', 'Delete'),
        icon: Trash2,
        danger: true,
        separatorBefore: true,
        hidden: () => !canDelete,
        onSelect: (inc) => remove(inc),
      },
    ],
    [lang, canDelete, navigate],
  ); // eslint-disable-line react-hooks/exhaustive-deps

  const bulkActions: BulkAction<Incident>[] = useMemo(
    () => [
      {
        key: 'resolve',
        label: tr('Marquer résolus', 'Mark resolved'),
        icon: CheckCircle2,
        hidden: !canUpdate,
        selectionOnly: true,
        run: async ({ rows }) => {
          await Promise.all(
            rows.map((inc) =>
              updateIncident.mutateAsync({ id: inc.id, input: { status: 'resolved' } }),
            ),
          );
          toast.success(
            tr(`${rows.length} incident(s) résolu(s)`, `${rows.length} incident(s) resolved`),
          );
        },
      },
      {
        key: 'delete',
        label: tr('Supprimer', 'Delete'),
        icon: Trash2,
        danger: true,
        hidden: !canDelete,
        selectionOnly: true,
        run: async ({ rows }) => {
          rows.forEach((inc) => remove(inc));
        },
      },
    ],
    [lang, canUpdate, canDelete, updateIncident],
  ); // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <PageFrame wide>
      <PageHeader
        title={tr('Incidents', 'Incidents')}
        count={stats ? `${stats.open_incidents} ${tr('ouverts', 'open')}` : undefined}
        actions={
          <>
            <Btn
              label={exporting ? tr('Export…', 'Exporting…') : tr('Exporter', 'Export')}
              icon={Download}
              onClick={exportCsv}
            />
            <Btn label={tr('War Room', 'War Room')} icon={Activity} onClick={openWarRoom} />
            {canCreate && (
              <Btn
                label={tr('Déclarer un incident', 'Declare an incident')}
                icon={Siren}
                primary
                onClick={() => setShowCreate(true)}
              />
            )}
          </>
        }
      />

      {/* KPI header */}
      <div
        className="grid gap-3.5 mb-4"
        style={{ gridTemplateColumns: 'repeat(auto-fit,minmax(180px,1fr))' }}
      >
        {kpis.map((k, i) => (
          <Card
            key={k.label}
            style={{
              padding: '16px 18px',
              animation: 'or-fadeup .4s ease both',
              animationDelay: `${i * 0.05}s`,
            }}
          >
            <div className="flex items-center gap-3">
              <div
                className="w-10 h-10 rounded-[11px] flex items-center justify-center shrink-0"
                style={{ background: softFill(k.color, 14), color: k.color }}
              >
                <k.icon size={19} strokeWidth={1.9} />
              </div>
              <div>
                <div className="disp mono text-[22px] font-bold text-ink leading-none">
                  {k.value}
                </div>
                <div className="text-[12px] text-ink-muted mt-1">{k.label}</div>
              </div>
            </div>
          </Card>
        ))}
      </div>

      <DataTable
        id="incidents"
        ariaLabel={tr('Incidents', 'Incidents')}
        rows={visibleIncidents}
        columns={columns}
        rowKey={(inc) => String(inc.id)}
        api={table}
        mode="client"
        loading={isLoading}
        error={isError}
        onRetry={() => void refetch()}
        facets={facets}
        clientSearch={(inc, q) =>
          `${inc.title} ${inc.description ?? ''} ${inc.reported_by ?? ''}`.toLowerCase().includes(q)
        }
        searchPlaceholder={tr(
          'Titre, description ou déclarant…',
          'Title, description or reporter…',
        )}
        selectable={canUpdate || canDelete}
        rowActions={rowActions}
        bulkActions={bulkActions}
        onRowClick={(inc) => setSelected(inc)}
        exportFilename="incidents"
        minWidth={880}
        empty={
          <EmptyState
            icon={Siren}
            title={tr('Aucun incident', 'No incidents')}
            description={tr(
              'Rien à signaler. Un incident peut être déclaré ici, ou ouvert automatiquement par une règle, un scan ou un flux de menaces — chacun porte alors un bandeau qui dit lequel.',
              'Nothing to report. An incident can be declared here, or opened automatically by a rule, a scan or a threat feed — each one then carries a banner saying which.',
            )}
            primaryAction={
              canCreate ? (
                <Btn
                  label={tr('Déclarer un incident', 'Declare an incident')}
                  icon={Siren}
                  primary
                  onClick={() => setShowCreate(true)}
                />
              ) : undefined
            }
            learnMoreHref="/incidents/sources"
            learnMoreLabel={tr('D’où viennent les incidents ?', 'Where do incidents come from?')}
          />
        }
      />

      {showCreate && <DeclareIncidentModal onClose={() => setShowCreate(false)} />}
      {selected && (
        <IncidentDrawer
          incident={selected}
          canWrite={canUpdate}
          onClose={() => setSelected(null)}
        />
      )}
    </PageFrame>
  );
}
