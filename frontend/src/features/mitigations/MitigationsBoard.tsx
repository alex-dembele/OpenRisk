// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// Mitigations (OpenRisk.dc.html §6.4) — wired to the real /mitigations list, with
// three selectable views: Kanban (To do → In progress → In review → Done), a dense
// Table, and a Gantt timeline positioned by real start/due dates. Loading skeleton
// + empty state on all three.

import { localeTag } from '../../i18n/locales';
import { useMemo, useState } from 'react';
import { useNavigate } from 'react-router';
import { useQueryClient, useMutation } from '@tanstack/react-query';
import { toast } from 'sonner';
import {
  Plus,
  Clock,
  ShieldCheck,
  KanbanSquare,
  Rows3,
  GanttChartSquare,
  ChevronDown,
  Check,
  Loader2,
} from 'lucide-react';
import {
  PageFrame,
  PageHeader,
  Btn,
  Avatar,
  Skeleton,
  EmptyState,
  softFill,
} from '../../shared/ui';
import {
  DataTable,
  useTableState,
  type Column as TableColumn,
  type Facet,
  type RowAction,
} from '../../shared/datatable';
import { critColor } from '../../shared/riskColors';
import { useUIStrings } from '../../shared/uiStrings';
import { useUIStore } from '../../store/uiStore';
import { mitigationService, type BoardStatus } from '../../services/mitigationService';
import { useMitigations, type Column, type UiMiti } from './useMitigations';
import { useMitigationEvents } from './useMitigationEvents';

// Backend status ⇄ board column. A freshly-created plan is PLANNED (the "todo" column).
const COL_TO_STATUS: Record<Column, BoardStatus> = {
  todo: 'PLANNED',
  progress: 'IN_PROGRESS',
  review: 'REVIEW',
  done: 'DONE',
};

type View = 'kanban' | 'table' | 'gantt';

export function MitigationsBoard() {
  const L = useUIStrings();
  const lang = useUIStore((s) => s.lang);
  const navigate = useNavigate();
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const { columns, items, isLoading, isError, refetch } = useMitigations();
  const [view, setView] = useState<View>('kanban');

  // Live scanner-driven auto-completions push over SSE → refresh the board so the
  // "Auto-detected" badge appears without a manual reload.
  useMitigationEvents();

  const cols: [Column, string, string][] = [
    ['todo', L.col_todo, 'var(--fg-muted)'],
    ['progress', L.col_doing, 'var(--high)'],
    ['review', L.col_review, 'var(--info)'],
    ['done', L.col_done, 'var(--low)'],
  ];
  const statusLabel: Record<Column, string> = {
    todo: L.col_todo,
    progress: L.col_doing,
    review: L.col_review,
    done: L.col_done,
  };
  const statusColor: Record<Column, string> = {
    todo: 'var(--fg-muted)',
    progress: 'var(--high)',
    review: 'var(--info)',
    done: 'var(--low)',
  };

  const viewBtns: [View, typeof KanbanSquare, string][] = [
    ['kanban', KanbanSquare, tr('Kanban', 'Kanban')],
    ['table', Rows3, tr('Table', 'Table')],
    ['gantt', GanttChartSquare, tr('Gantt', 'Gantt')],
  ];

  return (
    <PageFrame wide>
      <PageHeader
        title={L.mitiTitle}
        actions={
          <>
            <div
              className="inline-flex rounded-[10px] p-0.5"
              style={{ border: '1px solid var(--border-strong)', background: 'var(--bg-elevated)' }}
            >
              {viewBtns.map(([v, Icon, lbl]) => (
                <button
                  key={v}
                  onClick={() => setView(v)}
                  title={lbl}
                  className="h-8 px-2.5 rounded-[8px] text-[12.5px] font-semibold inline-flex items-center gap-1.5 transition-colors"
                  style={{
                    background: view === v ? 'var(--accent-soft)' : 'transparent',
                    color: view === v ? 'var(--accent)' : 'var(--fg-secondary)',
                  }}
                >
                  <Icon size={15} /> <span className="hidden sm:inline">{lbl}</span>
                </button>
              ))}
            </div>
            <Btn label={L.addPlan} icon={Plus} primary onClick={() => navigate('/risks')} />
          </>
        }
      />

      {!isLoading && items.length === 0 ? (
        <EmptyState
          icon={ShieldCheck}
          title={tr('Aucune mitigation', 'No mitigations yet')}
          description={tr(
            'Créez une mitigation depuis un risque pour lancer son traitement.',
            'Create a mitigation from a risk to start treating it.',
          )}
          primaryAction={<Btn label={L.n_risks} onClick={() => navigate('/risks')} primary />}
        />
      ) : view === 'kanban' ? (
        <div className="overflow-x-auto -mx-1 px-1">
          <div
            className="grid gap-3.5 items-start"
            style={{ gridTemplateColumns: 'repeat(4,minmax(0,1fr))', minWidth: 760 }}
          >
            {cols.map(([key, label, col]) => (
              <div
                key={key}
                className="rounded-[14px] p-3"
                style={{
                  background: 'var(--bg-secondary)',
                  border: '1px solid var(--border)',
                  minHeight: 200,
                }}
              >
                <div className="flex items-center justify-between mb-3 px-1">
                  <div className="flex items-center gap-2">
                    <span className="w-2 h-2 rounded-full" style={{ background: col }} />
                    <span className="text-[12.5px] font-semibold text-ink">{label}</span>
                    <span className="text-[11px] font-semibold text-ink-muted">
                      {columns[key].length}
                    </span>
                  </div>
                  <button
                    onClick={() => navigate('/risks')}
                    className="w-[22px] h-[22px] rounded-md flex items-center justify-center text-ink-muted hover:bg-hover transition-colors"
                  >
                    <Plus size={14} />
                  </button>
                </div>
                {isLoading ? (
                  <div className="flex flex-col gap-2.5">
                    {[0, 1].map((i) => (
                      <Skeleton key={i} style={{ height: 92 }} />
                    ))}
                  </div>
                ) : (
                  columns[key].map((c) => (
                    <KanbanCard
                      key={c.id}
                      c={c}
                      onOpen={() => navigate(`/risks/mitigations/${c.id}`)}
                    />
                  ))
                )}
              </div>
            ))}
          </div>
        </div>
      ) : view === 'table' ? (
        <TableView
          items={items}
          isLoading={isLoading}
          isError={isError}
          onRetry={refetch}
          statusLabel={statusLabel}
          statusColor={statusColor}
          onOpen={(m: UiMiti) => navigate(`/risks/mitigations/${m.id}`)}
        />
      ) : (
        <GanttView items={items} isLoading={isLoading} statusColor={statusColor} />
      )}
    </PageFrame>
  );
}

function KanbanCard({ c, onOpen }: { c: UiMiti; onOpen: () => void }) {
  const [hover, setHover] = useState(false);
  return (
    <div
      onClick={onOpen}
      onMouseEnter={() => setHover(true)}
      onMouseLeave={() => setHover(false)}
      className="rounded-[11px] p-[13px] mb-2.5 cursor-pointer transition-all"
      style={{
        background: 'var(--bg-elevated)',
        border: `1px solid ${c.overdue ? 'rgba(255,69,58,.4)' : 'var(--border)'}`,
        boxShadow: hover ? 'var(--shadow-md)' : 'var(--shadow-sm)',
        transform: hover ? 'translateY(-2px)' : 'none',
      }}
    >
      <div className="text-[13px] font-medium text-ink mb-2.5 leading-snug">{c.title}</div>
      <div className="flex items-center gap-2 mb-[11px]">
        <span className="w-1.5 h-1.5 rounded-full" style={{ background: critColor[c.crit] }} />
        <span className="mono text-[11px] text-ink-muted">{c.risk}</span>
      </div>
      <div
        className="h-1 rounded overflow-hidden mb-[11px]"
        style={{ background: 'var(--bg-hover)' }}
      >
        <div
          className="h-full rounded"
          style={{
            width: `${c.progress}%`,
            background: c.progress === 100 ? 'var(--low)' : 'var(--accent)',
          }}
        />
      </div>
      <div className="flex items-center justify-between">
        <Avatar initials={c.owner} size={24} />
        <span
          className="text-[11px] font-semibold inline-flex items-center gap-1"
          style={{ color: c.overdue ? 'var(--critical)' : 'var(--fg-muted)' }}
        >
          {c.overdue && <Clock size={12} />}
          {c.deadline}
        </span>
      </div>
    </div>
  );
}

/* ---------------- table view ---------------- */
// Click-to-edit status right in the table row (ghost edit + optimistic autosave).
function InlineMitiStatus({
  c,
  statusLabel,
  statusColor,
}: {
  c: UiMiti;
  statusLabel: Record<Column, string>;
  statusColor: Record<Column, string>;
}) {
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const qc = useQueryClient();
  const [open, setOpen] = useState(false);
  const setStatus = useMutation({
    mutationFn: (col: Column) => mitigationService.setStatus(c.id, COL_TO_STATUS[col]),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['mitigations'] });
      toast.success(tr('Statut mis à jour', 'Status updated'));
    },
    onError: () => toast.error(tr('Échec', 'Failed')),
  });
  const cols: Column[] = ['todo', 'progress', 'review', 'done'];
  const pill = (col: Column) => (
    <span className="inline-flex items-center gap-1.5 text-[12px] font-medium text-ink-soft">
      <span className="w-[7px] h-[7px] rounded-full" style={{ background: statusColor[col] }} />
      {statusLabel[col]}
    </span>
  );
  return (
    <div className="relative inline-block" onClick={(e) => e.stopPropagation()}>
      <button
        onClick={() => setOpen((o) => !o)}
        disabled={setStatus.isPending}
        title={tr('Changer le statut', 'Change status')}
        className="inline-flex items-center gap-1 rounded-md px-1.5 py-1 -mx-1 hover:bg-hover transition-colors"
      >
        {pill(c.column)}
        {setStatus.isPending ? (
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
            {cols.map((col) => (
              <button
                key={col}
                onClick={() => {
                  setOpen(false);
                  if (col !== c.column) setStatus.mutate(col);
                }}
                className="w-full flex items-center justify-between gap-2 px-2 py-1.5 rounded-[7px] hover:bg-hover transition-colors text-left"
              >
                {pill(col)}
                {col === c.column && <Check size={13} className="text-accent" />}
              </button>
            ))}
          </div>
        </>
      )}
    </div>
  );
}

// Table view — <DataTable> in client mode: GET /mitigations returns the board in
// one page (200 max) because the Kanban and the Gantt need the whole set, so the
// filtering, sorting and paging happen here rather than round-tripping.
function TableView({
  items,
  isLoading,
  isError,
  onRetry,
  statusLabel,
  statusColor,
  onOpen,
}: {
  items: UiMiti[];
  isLoading: boolean;
  isError: boolean;
  onRetry: () => void;
  statusLabel: Record<Column, string>;
  statusColor: Record<Column, string>;
  onOpen: (m: UiMiti) => void;
}) {
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const table = useTableState({ defaultSort: { key: 'due', dir: 'asc' }, defaultPageSize: 50 });

  const facets: Facet<UiMiti>[] = useMemo(
    () => [
      {
        key: 'status',
        label: lang === 'fr' ? 'Statut' : 'Status',
        options: (['todo', 'progress', 'review', 'done'] as Column[]).map((col) => ({
          value: col,
          label: statusLabel[col],
          color: statusColor[col],
        })),
        matches: (m, selected) => selected.includes(m.column),
      },
      {
        key: 'priority',
        label: lang === 'fr' ? 'Priorité' : 'Priority',
        options: (['critical', 'high', 'medium', 'low'] as const).map((c) => ({
          value: c,
          label: c,
          color: critColor[c],
        })),
        matches: (m, selected) => selected.includes(m.crit),
      },
      {
        key: 'overdue',
        label: lang === 'fr' ? 'Échéance' : 'Deadline',
        single: true,
        options: [
          {
            value: 'true',
            label: lang === 'fr' ? 'En retard' : 'Overdue',
            color: 'var(--critical)',
          },
        ],
        matches: (m, selected) => (selected.includes('true') ? m.overdue : true),
      },
    ],
    [lang, statusLabel, statusColor],
  );

  const columns: TableColumn<UiMiti>[] = useMemo(
    () => [
      {
        key: 'title',
        header: lang === 'fr' ? 'Plan' : 'Plan',
        frozen: true,
        hideable: false,
        sortValue: (m) => m.title.toLowerCase(),
        exportValue: (m) => m.title,
        render: (m) => (
          <span className="text-[13.5px] font-medium text-ink max-w-[300px] truncate inline-block">
            {m.title}
          </span>
        ),
      },
      {
        key: 'risk',
        header: lang === 'fr' ? 'Risque' : 'Risk',
        sortValue: (m) => m.risk,
        exportValue: (m) => m.risk,
        render: (m) => <span className="mono text-[11.5px] text-ink-muted">{m.risk}</span>,
      },
      {
        key: 'priority',
        header: lang === 'fr' ? 'Priorité' : 'Priority',
        sortValue: (m) => ({ critical: 4, high: 3, medium: 2, low: 1 })[m.crit] ?? 0,
        exportValue: (m) => m.crit,
        render: (m) => (
          <span
            className="inline-flex items-center gap-1.5 text-[11.5px] font-semibold px-[9px] py-[3px] rounded-full"
            style={{ color: critColor[m.crit], background: softFill(critColor[m.crit], 15) }}
          >
            <span className="w-1.5 h-1.5 rounded-full" style={{ background: critColor[m.crit] }} />
            {m.crit}
          </span>
        ),
      },
      {
        key: 'status',
        header: lang === 'fr' ? 'Statut' : 'Status',
        sortValue: (m) => m.column,
        exportValue: (m) => statusLabel[m.column],
        render: (m) => (
          <InlineMitiStatus c={m} statusLabel={statusLabel} statusColor={statusColor} />
        ),
      },
      {
        key: 'progress',
        header: lang === 'fr' ? 'Avancement' : 'Progress',
        sortValue: (m) => m.progress,
        exportValue: (m) => `${m.progress}%`,
        render: (m) => (
          <div className="flex items-center gap-2 min-w-[120px]">
            <div
              className="flex-1 h-1.5 rounded overflow-hidden"
              style={{ background: 'var(--bg-hover)' }}
            >
              <div
                className="h-full rounded"
                style={{
                  width: `${m.progress}%`,
                  background: m.progress === 100 ? 'var(--low)' : 'var(--accent)',
                }}
              />
            </div>
            <span className="mono text-[11px] text-ink-muted w-8 text-right">{m.progress}%</span>
          </div>
        ),
      },
      {
        key: 'due',
        header: lang === 'fr' ? 'Échéance' : 'Due',
        sortValue: (m) => (m.dueISO ? new Date(m.dueISO).getTime() : Number.MAX_SAFE_INTEGER),
        exportValue: (m) => m.dueISO ?? '',
        render: (m) => (
          <span
            className="text-[12.5px] whitespace-nowrap"
            style={{ color: m.overdue ? 'var(--critical)' : 'var(--fg-secondary)' }}
          >
            {m.deadline}
          </span>
        ),
      },
      {
        key: 'owner',
        header: lang === 'fr' ? 'Resp.' : 'Owner',
        exportValue: (m) => m.owner,
        render: (m) => <Avatar initials={m.owner} size={24} />,
      },
    ],
    [lang, statusLabel, statusColor],
  );

  const rowActions: RowAction<UiMiti>[] = useMemo(
    () => [
      {
        key: 'open',
        label: lang === 'fr' ? 'Ouvrir le plan' : 'Open plan',
        icon: ShieldCheck,
        onSelect: onOpen,
      },
    ],
    [lang, onOpen],
  );

  return (
    <DataTable
      id="mitigations"
      ariaLabel={lang === 'fr' ? 'Plans de mitigation' : 'Mitigation plans'}
      rows={items}
      columns={columns}
      rowKey={(m) => m.id}
      api={table}
      mode="client"
      loading={isLoading}
      error={isError}
      onRetry={onRetry}
      facets={facets}
      clientSearch={(m, q) => `${m.title} ${m.risk} ${m.owner}`.toLowerCase().includes(q)}
      searchPlaceholder={tr('Plan, risque ou responsable…', 'Plan, risk or owner…')}
      rowActions={rowActions}
      onRowClick={onOpen}
      exportFilename="mitigations"
      minWidth={880}
      empty={
        <EmptyState
          variant="first-use"
          icon={ShieldCheck}
          title={tr('Aucun plan de mitigation', 'No mitigation plan yet')}
          description={tr(
            'Créez un plan depuis un risque pour organiser son traitement.',
            'Create a plan from a risk to organise its treatment.',
          )}
        />
      }
    />
  );
}

/* ---------------- gantt view ---------------- */
function GanttView({
  items,
  isLoading,
  statusColor,
}: {
  items: UiMiti[];
  isLoading: boolean;
  statusColor: Record<Column, string>;
}) {
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const DAY = 864e5;

  const { rows, ticks, hasDates } = useMemo(() => {
    const now = Date.now();
    const dated = items
      .map((c) => {
        const due = c.dueISO ? new Date(c.dueISO).getTime() : NaN;
        let start = c.startISO ? new Date(c.startISO).getTime() : NaN;
        if (Number.isNaN(start)) start = Number.isNaN(due) ? now : Math.min(now, due - 3 * DAY);
        return { c, start, due: Number.isNaN(due) ? start + 7 * DAY : due };
      })
      .filter((r) => !Number.isNaN(r.start));
    if (dated.length === 0)
      return { rows: [], ticks: [] as { pct: number; label: string }[], hasDates: false };
    let min = Math.min(now, ...dated.map((r) => r.start));
    let max = Math.max(now, ...dated.map((r) => r.due));
    if (max - min < 7 * DAY) max = min + 7 * DAY;
    const pad = (max - min) * 0.04;
    min -= pad;
    max += pad;
    const range = max - min || 1;
    const pct = (t: number) => ((t - min) / range) * 100;
    const rows = dated
      .sort((a, b) => a.due - b.due)
      .map((r) => ({ c: r.c, left: pct(r.start), width: Math.max(2, pct(r.due) - pct(r.start)) }));
    const ticks: { pct: number; label: string }[] = [];
    for (let k = 0; k <= 4; k++) {
      const t = min + (range * k) / 4;
      ticks.push({
        pct: (k / 4) * 100,
        label: new Date(t).toLocaleDateString(localeTag(lang), {
          day: '2-digit',
          month: 'short',
        }),
      });
    }
    return { rows, ticks, hasDates: true };
  }, [items, lang, DAY]);

  if (isLoading)
    return (
      <div className="flex flex-col gap-2">
        {[0, 1, 2, 3].map((i) => (
          <Skeleton key={i} style={{ height: 40 }} />
        ))}
      </div>
    );
  if (!hasDates)
    return (
      <EmptyState
        icon={GanttChartSquare}
        title={tr('Aucune date planifiée', 'No scheduled dates')}
        description={tr(
          'Ajoutez une échéance aux plans pour les voir sur le Gantt.',
          'Add due dates to plans to see them on the Gantt.',
        )}
      />
    );

  return (
    <div
      className="rounded-[14px] p-4 overflow-x-auto"
      style={{ border: '1px solid var(--border)' }}
    >
      <div style={{ minWidth: 720 }}>
        {/* axis */}
        <div
          className="relative h-5 mb-2 ml-[220px]"
          style={{ borderBottom: '1px solid var(--border)' }}
        >
          {ticks.map((t, i) => (
            <span
              key={i}
              className="absolute text-[10.5px] text-ink-muted"
              style={{
                left: `${t.pct}%`,
                transform: i === ticks.length - 1 ? 'translateX(-100%)' : 'none',
              }}
            >
              {t.label}
            </span>
          ))}
        </div>
        {rows.map(({ c, left, width }) => (
          <div key={c.id} className="flex items-center gap-3 mb-1.5">
            <div className="w-[208px] shrink-0 text-[12.5px] text-ink truncate" title={c.title}>
              {c.title}
            </div>
            <div
              className="relative flex-1 h-7 rounded-[8px]"
              style={{ background: 'var(--bg-hover)' }}
            >
              <div
                className="absolute top-1 bottom-1 rounded-[6px] flex items-center overflow-hidden"
                style={{
                  left: `${left}%`,
                  width: `${width}%`,
                  background: softFill(c.overdue ? 'var(--critical)' : statusColor[c.column], 26),
                  border: `1px solid ${softFill(c.overdue ? 'var(--critical)' : statusColor[c.column], 45)}`,
                }}
                title={`${c.title} · ${c.deadline}`}
              >
                <div
                  className="h-full rounded-[6px]"
                  style={{
                    width: `${c.progress}%`,
                    background: c.overdue ? 'var(--critical)' : statusColor[c.column],
                    opacity: 0.55,
                  }}
                />
              </div>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
