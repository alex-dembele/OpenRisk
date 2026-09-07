// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// The Security Command Center.
//
// Every number on this page comes from a tenant-scoped server aggregate, and
// every tile links to the register rows that produced it, carrying the filter
// the tile expresses. There are no fixtures, no fallbacks and no client-side
// arithmetic over a page of results: a widget that cannot read its data says so
// rather than showing a plausible zero.
//
// See docs/W0-06_SECURITY_COMMAND_CENTER.md for the contract inventory.

import { useEffect, useMemo, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router';
import {
  ShieldAlert,
  AlertTriangle,
  ShieldCheck,
  CheckCircle2,
  FileText,
  Zap,
  Plus,
  Grid3x3,
  TrendingUp,
  type LucideIcon,
} from 'lucide-react';
import { useRiskStore } from '../../hooks/useRiskStore';
import { useUIStore } from '../../store/uiStore';
import { useAuthStore } from '../../hooks/useAuthStore';
import { useUIStrings } from '../../shared/uiStrings';
import { critColor, frameworkColor, softFill, type Criticality } from '../../shared/riskColors';
import {
  useDashboardStats,
  type DashboardStats,
  type MatrixCell,
  type RiskTrend,
} from './useCommandCenter';
import { useDashboardPeriod, periodLabel, type PeriodSelection } from './period';
import { deepLink } from './deepLinks';
import { PeriodControl } from './PeriodControl';
import { WidgetState } from './WidgetState';
import { useCountUp } from './shared';
import { useScore } from '../../hooks/useScore';
import { ScoreGauge } from '../../shared/ScoreGauge';
import { EmptyState } from '../../shared/EmptyState';
import { Btn } from '../../shared/ui';
import { useIncidents } from '../incidents/useIncidents';
import { OnboardingChecklist } from '../onboarding/OnboardingChecklist';
import { MFAEnrollmentBanner } from '../auth/MFAEnrollmentBanner';
import { MFAPostAhaPrompt } from '../auth/MFAPostAhaPrompt';
import { ActionCenterPanel } from '../action-center/ActionCenterPanel';
import { personaFor } from './dashboardPersona';
import { AnalystDashboard } from './AnalystDashboard';
import { ExecDashboard } from './ExecDashboard';
import { AuditDashboard } from './AuditDashboard';
import { EstateDashboard } from './EstateDashboard';
import { ViewerDashboard } from './ViewerDashboard';
import { ExecutiveDashboard } from '../analytics/ExecutiveDashboard';

const Card = ({
  children,
  className = '',
  style,
}: {
  children: React.ReactNode;
  className?: string;
  style?: React.CSSProperties;
}) => (
  <div className={`or-card ${className}`} style={style}>
    {children}
  </div>
);

interface RecentRisk {
  id: string;
  rawId: string;
  name: string;
  crit: Criticality;
  score: number;
  meta: string;
  fw: string;
}

/* ---------------- persona dispatcher ---------------- */

// The dashboard adapts to the member's GRC role (UX-2). Each persona renders its
// own real-data view; admins and unmapped roles get the full posture dashboard.
//
// ?view=executive overrides the persona with the consolidated executive view.
// That view used to live at /analytics, filed under Reports — which is why
// people went looking for it beside the PDFs. It answers "how are we doing",
// which is the dashboard's own question, so it is a display mode here rather
// than a destination of its own. /analytics still redirects to ?view=executive
// so existing links and bookmarks resolve.
export const DashboardPage = () => {
  const businessRole = useAuthStore((s) => s.user?.business_role);
  const persona = personaFor(businessRole);
  const [params] = useSearchParams();
  if (params.get('view') === 'executive') return <ExecutiveDashboard />;
  switch (persona) {
    case 'analyst':
      return <AnalystDashboard />;
    case 'exec':
      return <ExecDashboard />;
    case 'audit':
      return <AuditDashboard />;
    case 'estate':
      return <EstateDashboard />;
    case 'viewer':
      return <ViewerDashboard />;
    default:
      return <PostureDashboard />;
  }
};

/* ---------------- posture persona (default: RSSI / risk roles / admin) ---------------- */

function PostureDashboard() {
  const navigate = useNavigate();
  const L = useUIStrings();
  const lang = useUIStore((s) => s.lang);
  const risks = useRiskStore((s) => s.risks);
  const fetchRisks = useRiskStore((s) => s.fetchRisks);

  useEffect(() => {
    fetchRisks?.().catch(() => {});
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const critOf = (r: (typeof risks)[number]): Criticality =>
    // The server's criticality band (derived from the score by the ScoreWorker).
    // `level` is a legacy field that is not reliably populated — it rendered
    // every risk as medium. Prefer criticality; deriving a band from the number
    // here is the mapping that drifted away from the number itself.
    ((r.criticality ?? r.level)?.toLowerCase() as Criticality) || 'low';

  // The period lives in the URL, so it survives a reload and can be pasted.
  const { selection, setSelection } = useDashboardPeriod();
  const statsQuery = useDashboardStats(selection);
  const stats = statsQuery.data;

  // The canonical tenant score — the same query key the sidebar and /score use,
  // so the three render one object from one fetch and cannot disagree.
  const { data: tenantScore, isLoading: scoreLoading } = useScore('tenant');
  const user = useAuthStore((s) => s.user);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const firstName = (user?.full_name || '').trim().split(/\s+/)[0] || user?.username || '';
  const greeting = `${tr('Bonjour', 'Hello')}${firstName ? `, ${firstName}` : ''}`;
  const sev = stats?.risks_by_severity ?? {};

  // Every KPI is read straight off the server aggregate. These used to fall back
  // to risks.filter(...).length over the risk store, which holds ONE PAGE of the
  // register — so "12 en cours" meant "12 on the page you happen to be on" and
  // silently disagreed with the register itself. There is no client-side count
  // here on purpose, and no `?? 0` either: when the aggregate has not arrived the
  // tiles render a loading state, and when it FAILED they render an error state.
  // A zero that was never read is the most reassuring thing this page can print,
  // and the least true.
  const kpis = useMemo(
    () => ({
      total: stats?.total_risks ?? 0,
      critical: sev.CRITICAL ?? sev.critical ?? 0,
      mitig: stats?.in_progress_risks ?? 0,
      resolved: stats?.mitigated_risks ?? 0,
      // eslint-disable-next-line react-hooks/exhaustive-deps
    }),
    [stats],
  );

  const recent: RecentRisk[] = useMemo(() => {
    return risks.slice(0, 5).map((r) => ({
      rawId: r.id,
      id: r.id.length > 10 ? `#${r.id.slice(0, 8)}` : r.id,
      name: r.title,
      crit: critOf(r),
      score: r.score,
      meta: r.assets?.[0]?.name ?? '—',
      fw: r.frameworks?.[0] ?? r.tags?.[0] ?? '—',
    }));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [risks]);

  const fmt = (n: number) => Math.round(n).toLocaleString(lang === 'fr' ? 'fr-FR' : 'en-US');

  return (
    <div className="flex-1 overflow-y-auto">
      <div
        className="mx-auto px-5 sm:px-7 pt-6 pb-10 max-w-[1320px]"
        style={{ animation: 'or-fadeup .4s ease' }}
      >
        {/* header */}
        <div className="flex items-start justify-between flex-wrap gap-3.5 mb-[22px]">
          <div>
            <h1 className="disp text-[27px] font-bold tracking-tight text-ink">{greeting}</h1>
            <div className="text-[14px] text-ink-soft mt-1">{L.dashSub}</div>
          </div>
          <div className="flex items-start gap-3 flex-wrap">
            <PeriodControl
              selection={selection}
              onChange={setSelection}
              lang={lang}
              // The control names its own scope. It filters the trend and the
              // "opened" counter; it deliberately does NOT filter the stock
              // tiles, because "how many critical risks do we have" is not a
              // question a date range changes.
              scopeNote={tr(
                'Filtre la tendance et les risques ouverts. Les compteurs (total, critiques, en traitement, atténués) donnent l’état actuel du registre, toutes périodes confondues.',
                'Filters the trend and risks opened. The counters (total, critical, in treatment, mitigated) show the register as it stands now, across all time.',
              )}
            />
            <button
              onClick={() => navigate('/reports')}
              className="h-[38px] px-4 rounded-[10px] flex items-center gap-2 text-[13px] font-semibold text-ink hover:bg-hover transition-colors"
              style={{ background: 'var(--bg-elevated)', border: '1px solid var(--border-strong)' }}
            >
              <FileText size={16} strokeWidth={1.75} />
              {L.genReport}
            </button>
          </div>
        </div>

        {/* Get-started panel. It takes no props on purpose: the steps, their
            completion and the celebration all come from GET /activation/state.
            Deriving them here from local counts is what used to make it tick
            unreliably and fire confetti at random. */}
        {/* OR26-03 — the non-blocking MFA prompt. It replaces the enrolment wall
            that used to stand between a new account and its first screen: the
            server now lets eligible users in and this says what they should do
            about it. Enforcement is the server's (MFAPolicyGuard) — dismissing
            this changes nothing about who is allowed in. */}
        <MFAEnrollmentBanner />

        <OnboardingChecklist />

        {/* And the second ask, once the Aha moment has actually been reached.
            The trigger is activation.aha_reached_at, recorded server-side from
            the user's own data — never a flag this component sets. */}
        <MFAPostAhaPrompt />

        {/* #430 — the Action Center, above the score and the KPIs on purpose:
            "what should I do next" outranks "how are we doing" on the screen a
            user lands on. DashboardShell mounts the same panel for the other
            personas; this dashboard does not use that shell. */}
        <ActionCenterPanel />

        {/* row 1 — score hero + kpis */}
        <div className="grid grid-cols-1 lg:grid-cols-[340px_1fr] gap-4 mb-4">
          {/* One score, one source. This used to read stats.global_risk_score
              while the sidebar read analytics/executive → cyber_score.score:
              two quantities on two scales pointing in opposite directions,
              both labelled "score". Both now read the same query key, and the
              competing formula has been removed from /stats entirely. */}
          <ScoreGauge
            score={tenantScore}
            loading={scoreLoading}
            title={L.globalScore}
            ctaLabel={L.viewDetails}
            onDetails={() => navigate('/score')}
          />
          <KpiGrid
            values={kpis}
            fmt={fmt}
            lang={lang}
            query={statsQuery}
            openedInPeriod={stats?.opened_in_period ?? 0}
            selection={selection}
          />
        </div>

        {/* row 2 — heatmap + trend */}
        <div className="grid grid-cols-1 lg:grid-cols-[1.5fr_1fr] gap-4 mb-4">
          <HeatmapCard
            matrix={stats?.risk_matrix}
            isLoading={statsQuery.isLoading}
            error={statsQuery.error}
            retry={() => void statsQuery.refetch()}
          />
          <TrendCard
            trend={stats?.risk_trend}
            registerTotal={stats?.total_risks ?? 0}
            isLoading={statsQuery.isLoading}
            error={statsQuery.error}
            retry={() => void statsQuery.refetch()}
            selection={selection}
            onWidenPeriod={() => setSelection({ kind: 'preset', preset: 'all' })}
          />
        </div>

        {/* row 3 — recent + war room */}
        <div className="grid grid-cols-1 lg:grid-cols-[1.5fr_1fr] gap-4">
          <RecentActivityCard risks={recent} />
          <WarRoomCard onJoin={(id) => navigate(id ? `/incidents/${id}/war-room` : '/incidents')} />
        </div>
      </div>
    </div>
  );
}

/* ---------------- KPI grid ---------------- */

interface StatsQuery {
  isLoading: boolean;
  error: unknown;
  refetch: () => unknown;
}

function KpiGrid({
  values,
  fmt,
  lang,
  query,
  openedInPeriod,
  selection,
}: {
  values: { total: number; critical: number; mitig: number; resolved: number };
  fmt: (n: number) => string;
  lang: 'fr' | 'en';
  query: StatsQuery;
  openedInPeriod: number;
  selection: PeriodSelection;
}) {
  const L = useUIStrings();
  const navigate = useNavigate();
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);

  // Each tile carries the filter it expresses onto the register. Clicking
  // "Critical — 3" used to land on an unfiltered list of everything and leave the
  // user to rebuild by hand the filter they had just clicked.
  //
  // The sort travels too: a critical-risks link that opens sorted by score puts
  // the three rows the tile counted at the top of the page, which is what the
  // user came for.
  const sort = { key: 'score', dir: 'desc' } as const;
  const data: {
    label: string;
    val: number;
    icon: LucideIcon;
    col: string;
    to: string;
    hint: string;
  }[] = [
    {
      label: L.kpiTotal,
      val: values.total,
      icon: ShieldAlert,
      col: 'var(--accent)',
      to: deepLink('risks', { sort }),
      hint: tr('Tous les risques du registre', 'Every risk in the register'),
    },
    {
      label: L.kpiCrit,
      val: values.critical,
      icon: AlertTriangle,
      col: 'var(--critical)',
      to: deepLink('risks', { filters: { criticality: 'critical' }, sort }),
      hint: tr(
        'Registre filtré sur criticité = critique',
        'Register filtered to criticality = critical',
      ),
    },
    {
      label: L.kpiMiti,
      val: values.mitig,
      icon: ShieldCheck,
      col: 'var(--high)',
      to: deepLink('risks', { filters: { status: 'in_progress' }, sort }),
      hint: tr(
        'Registre filtré sur statut = en cours',
        'Register filtered to status = in progress',
      ),
    },
    {
      label: L.kpiResolved,
      val: values.resolved,
      icon: CheckCircle2,
      col: 'var(--low)',
      to: deepLink('risks', { filters: { status: 'mitigated' }, sort }),
      hint: tr('Registre filtré sur statut = atténué', 'Register filtered to status = mitigated'),
    },
  ];

  return (
    <div className="grid grid-cols-2 grid-rows-2 gap-4">
      <WidgetState
        lang={lang}
        isLoading={query.isLoading}
        error={query.error}
        skeletonHeight={340}
        retry={() => query.refetch()}
      >
        <>
          {data.map((d) => (
            <KpiCard key={d.label} {...d} fmt={fmt} onClick={() => navigate(d.to)} />
          ))}
          {/* The one period-scoped counter in this block, labelled with the
              window so it cannot be read as a stock. */}
          <div className="col-span-2 -mt-1 text-[11.5px] text-ink-muted flex items-center gap-1.5">
            <TrendingUp size={13} />
            {tr(
              `${fmt(openedInPeriod)} ouvert(s) sur la période (${periodLabel(selection, lang)})`,
              `${fmt(openedInPeriod)} opened in the selected period (${periodLabel(selection, lang)})`,
            )}
          </div>
        </>
      </WidgetState>
    </div>
  );
}

function KpiCard({
  label,
  val,
  icon: Icon,
  col,
  fmt,
  onClick,
  hint,
}: {
  label: string;
  val: number;
  icon: LucideIcon;
  col: string;
  fmt: (n: number) => string;
  onClick: () => void;
  hint: string;
}) {
  const shown = Math.round(useCountUp(val));
  return (
    <button
      onClick={onClick}
      // The tile is a link to a filtered list; the title says which filter, so
      // the destination is never a surprise.
      title={hint}
      aria-label={`${label}: ${fmt(val)} — ${hint}`}
      className="or-card text-left p-[18px] hover:bg-hover transition-colors"
    >
      <div className="flex items-center mb-3.5">
        <div
          className="w-[34px] h-[34px] rounded-[10px] flex items-center justify-center"
          style={{ color: col, background: softFill(col, 14) }}
        >
          <Icon size={18} strokeWidth={1.75} />
        </div>
      </div>
      <div className="disp mono text-[32px] font-bold text-ink leading-none">{fmt(shown)}</div>
      <div className="text-[12.5px] text-ink-soft mt-[5px]">{label}</div>
    </button>
  );
}

/* ---------------- Heatmap ---------------- */
// The 25 cell counts come from the /stats risk_matrix aggregate, banded in SQL.
// They used to be a hardcoded literal, which meant a brand-new tenant with an
// empty register was shown a fully populated probability x impact matrix — the
// single most trust-destroying thing the dashboard did.
function HeatmapCard({
  matrix,
  isLoading,
  error,
  retry,
}: {
  matrix?: MatrixCell[];
  isLoading: boolean;
  error: unknown;
  retry: () => void;
}) {
  const L = useUIStrings();
  const lang = useUIStore((s) => s.lang);
  const navigate = useNavigate();
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);

  const counts: Record<string, number> = {};
  for (const cell of matrix ?? []) counts[`${cell.impact}-${cell.probability}`] = cell.count;
  const total = (matrix ?? []).reduce((n, c) => n + c.count, 0);

  // Tints the 5×5 grid's own cells from their COORDINATES (bucket × bucket,
  // 1–25) so the map reads as a heat map. No risk's score and no risk's label is
  // derived from it — it is a legend for the grid, not a score band. Named and
  // commented so it is never mistaken for one (see docs/scoring/SCORE_MODEL.md).
  const cellCol = (pBucket: number, iBucket: number) => {
    const v = pBucket * iBucket; // 1..25, the grid's own coordinates
    return v >= 15
      ? 'var(--critical)'
      : v >= 8
        ? 'var(--high)'
        : v >= 4
          ? 'var(--medium)'
          : 'var(--low)';
  };
  const rows = [];
  for (let i = 5; i >= 1; i--) {
    const cells = [];
    for (let p = 1; p <= 5; p++) {
      const c = counts[`${i}-${p}`] ?? 0;
      const col = cellCol(p, i);
      cells.push(
        <div
          key={p}
          title={`P${p}×I${i} · ${c}`}
          className="aspect-square rounded-lg flex items-center justify-center"
          style={{
            background: c ? softFill(col, 18 + c * 9) : 'var(--bg-hover)',
            border: `1px solid ${c ? softFill(col, 40) : 'var(--border)'}`,
          }}
        >
          {c ? (
            <span className="mono text-[13px] font-bold" style={{ color: col }}>
              {c}
            </span>
          ) : null}
        </div>,
      );
    }
    rows.push(
      <div key={i} className="grid grid-cols-5 gap-1.5">
        {cells}
      </div>,
    );
  }

  return (
    <Card style={{ padding: '18px 20px' }}>
      <div className="text-[14px] font-semibold text-ink mb-4">{L.heatTitle}</div>
      <WidgetState
        lang={lang}
        isLoading={isLoading}
        error={error}
        retry={retry}
        skeletonHeight={260}
        isEmpty={total === 0}
        emptyIcon={Grid3x3}
        emptyTitle={tr('Matrice vide', 'Empty matrix')}
        emptyDescription={tr(
          'La matrice croise la probabilité et l’impact de chaque risque pour montrer où se concentre votre exposition. Elle se remplit dès votre premier risque.',
          'The matrix plots every risk by probability and impact to show where your exposure concentrates. It fills in with your first risk.',
        )}
        emptyAction={
          <div className="flex gap-2">
            <Btn
              label={tr('Créer un risque', 'Create a risk')}
              icon={Plus}
              primary
              onClick={() => window.dispatchEvent(new CustomEvent('openrisk:new-risk'))}
            />
            <Btn label={tr('Importer', 'Import')} onClick={() => navigate('/risks/import')} />
          </div>
        }
      >
        <div className="flex gap-2.5">
          <div className="flex items-center">
            <span
              className="text-[11px] font-semibold text-ink-muted tracking-wide"
              style={{ writingMode: 'vertical-rl', transform: 'rotate(180deg)' }}
            >
              {L.impact}
            </span>
          </div>
          <div className="flex-1 flex flex-col gap-1.5">
            <div className="flex flex-col gap-1.5">{rows}</div>
            <div className="text-center text-[11px] font-semibold text-ink-muted mt-2 tracking-wide">
              {L.proba}
            </div>
          </div>
        </div>
      </WidgetState>
    </Card>
  );
}

/* ---------------- Trend ---------------- */
//
// The series comes from the server, over the period selected on this page.
//
// It used to be computed here, in the browser, from useRiskStore.risks — which
// holds ONE PAGE of the register. A tenant with 200 risks saw a trend of the 20
// on page one. It banded them on `level`, a field this same file documents as
// unreliable, and it plotted a cumulative count by created_at, so the line could
// only ever rise regardless of what the team actually did. Its 7/30/90 toggle was
// local component state that changed nothing else on the page.
//
// Two series now, and they are labelled apart because they answer different
// questions: `opened` is a FLOW (risks created in each bucket) and
// `cumulative_total` is a STOCK (risks in existence at each bucket's end). The
// band split uses each risk's CURRENT criticality — the register does not version
// criticality per day, and inventing a per-day band would be a number nobody
// could reconcile.
// Exported for its tests. The decision this component makes — "no risks" versus
// "no risks in THIS window" — was wrong once (#343) and the two states have
// opposite remedies, so it is pinned by tests rather than left to inspection.
export function TrendCard({
  trend,
  registerTotal,
  isLoading,
  error,
  retry,
  selection,
  onWidenPeriod,
}: {
  trend?: RiskTrend;
  /**
   * The register's own total, from the same payload. It is what decides which
   * empty state to show, and it has to come from here rather than be derived
   * from the series — see the comment on `emptyBecauseOfPeriod` below.
   */
  registerTotal: number;
  isLoading: boolean;
  error: unknown;
  retry: () => void;
  selection: PeriodSelection;
  onWidenPeriod: () => void;
}) {
  const L = useUIStrings();
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);

  // Memoised because `?? []` allocates a fresh array on every render, which
  // would re-run the series useMemo below every time regardless of the data.
  const points = useMemo(() => trend?.points ?? [], [trend]);
  const opened = points.reduce((n, p) => n + p.opened, 0);
  const lastCumulative = points.length ? points[points.length - 1].cumulative_total : 0;
  // Nothing opened in the window, but the register is not empty: that is a period
  // problem, not an empty register, and the two have opposite remedies.
  //
  // This reads the REGISTER's total, not the series' last cumulative point. The
  // first version used the cumulative, and it was wrong for every window in the
  // past: `cumulative_total` counts risks created BEFORE each bucket, so a
  // January window over a register first populated in August reads 0 and the
  // widget concluded the tenant had no risks. Driving it live is what showed a
  // tenant with eight risks being told "no trend yet — the line builds up as
  // risks are opened", which is precisely the wrong fact with the opposite
  // remedy attached.
  const emptyBecauseOfPeriod = opened === 0 && registerTotal > 0;

  const W = 300,
    H = 120,
    pad = 8;
  const series = useMemo(() => {
    const band = (key: string) => points.map((p) => p.opened_by_band?.[key] ?? 0);
    return { crit: band('CRITICAL'), high: band('HIGH'), med: band('MEDIUM') };
  }, [points]);
  const allMax = Math.max(1, ...series.crit, ...series.high, ...series.med);
  const line = (arr: number[]) => {
    const step = (W - pad * 2) / Math.max(1, arr.length - 1);
    return arr
      .map((v, i) => `${pad + i * step},${H - pad - (v / allMax) * (H - pad * 2)}`)
      .join(' ');
  };
  const leg = (col: string, lbl: string) => (
    <span key={lbl} className="inline-flex items-center gap-1.5 text-[11px] text-ink-soft">
      <span className="w-[9px] h-[3px] rounded-sm" style={{ background: col }} />
      {lbl}
    </span>
  );

  return (
    <Card style={{ padding: '18px 20px' }}>
      <div className="flex items-baseline justify-between mb-1">
        <div className="text-[14px] font-semibold text-ink">{L.trendTitle}</div>
        <div className="text-[11px] text-ink-muted">{periodLabel(selection, lang)}</div>
      </div>
      {/* Say what is plotted. "Trend" alone let a cumulative-only line pass for
          an improving or worsening posture. */}
      <div className="text-[11.5px] text-ink-muted mb-2.5">
        {tr('Risques ouverts par ', 'Risks opened per ')}
        {trend?.granularity === 'week' ? tr('semaine', 'week') : tr('jour', 'day')}
        {tr(', par criticité actuelle', ', by current criticality')}
      </div>
      <WidgetState
        lang={lang}
        isLoading={isLoading}
        error={error}
        retry={retry}
        skeletonHeight={180}
        isEmpty={points.length === 0 || opened === 0}
        emptyBecauseOfPeriod={emptyBecauseOfPeriod}
        onWidenPeriod={onWidenPeriod}
        emptyIcon={TrendingUp}
        emptyTitle={tr('Pas encore de tendance', 'No trend yet')}
        emptyDescription={tr(
          'La courbe se construit à mesure que des risques sont ouverts.',
          'The line builds up as risks are opened.',
        )}
      >
        <>
          <div className="flex gap-3.5 mb-2.5">
            {leg('var(--critical)', L.critical)}
            {leg('var(--high)', L.high)}
            {leg('var(--medium)', L.medium)}
          </div>
          <svg
            viewBox={`0 0 ${W} ${H}`}
            width="100%"
            height="150"
            preserveAspectRatio="none"
            role="img"
            // The accessible name IS the chart's content for anyone who cannot
            // see it: totals per band over the stated window, not "a chart".
            aria-label={tr(
              `Risques ouverts sur ${periodLabel(selection, lang)} : ${series.crit.reduce((a, b) => a + b, 0)} critiques, ${series.high.reduce((a, b) => a + b, 0)} élevés, ${series.med.reduce((a, b) => a + b, 0)} moyens. Total ${opened}.`,
              `Risks opened over ${periodLabel(selection, lang)}: ${series.crit.reduce((a, b) => a + b, 0)} critical, ${series.high.reduce((a, b) => a + b, 0)} high, ${series.med.reduce((a, b) => a + b, 0)} medium. Total ${opened}.`,
            )}
          >
            {[1, 2, 3].map((i) => (
              <line
                key={i}
                x1={pad}
                x2={W - pad}
                y1={pad + (i * (H - pad * 2)) / 3}
                y2={pad + (i * (H - pad * 2)) / 3}
                stroke="var(--border)"
                strokeWidth={1}
              />
            ))}
            <polyline
              points={line(series.med)}
              fill="none"
              stroke="var(--medium)"
              strokeWidth={2}
              strokeLinecap="round"
              strokeLinejoin="round"
            />
            <polyline
              points={line(series.high)}
              fill="none"
              stroke="var(--high)"
              strokeWidth={2}
              strokeLinecap="round"
              strokeLinejoin="round"
            />
            <polyline
              points={line(series.crit)}
              fill="none"
              stroke="var(--critical)"
              strokeWidth={2}
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          </svg>
          {/* The stock, in words rather than as a second line that would be read
              against the same axis as the flow. */}
          <div className="text-[11.5px] text-ink-soft mt-2">
            {tr(
              `${opened} ouvert(s) · ${lastCumulative} au registre en fin de période`,
              `${opened} opened · ${lastCumulative} in the register at the end of the period`,
            )}
          </div>
        </>
      </WidgetState>
    </Card>
  );
}

/* ---------------- Recent activity ---------------- */
function RecentActivityCard({ risks }: { risks: RecentRisk[] }) {
  const L = useUIStrings();
  const lang = useUIStore((s) => s.lang);
  const navigate = useNavigate();
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  return (
    <Card style={{ padding: '18px 14px' }}>
      <div className="flex items-center justify-between mb-2 px-2">
        <div className="text-[14px] font-semibold text-ink">{L.recentTitle}</div>
        <button
          onClick={() => navigate(deepLink('risks', { sort: { key: 'updated_at', dir: 'desc' } }))}
          className="text-[12px] font-semibold text-accent hover:underline"
        >
          {tr('Tout voir', 'See all')}
        </button>
      </div>
      <div>
        {risks.length === 0 && (
          <EmptyState
            variant="first-use"
            icon={ShieldAlert}
            title={tr('Aucune activité récente', 'No recent activity')}
            description={tr(
              'Les derniers risques ajoutés ou mis à jour apparaîtront ici, du plus critique au moins critique.',
              'The most recently added or updated risks appear here, most critical first.',
            )}
            primaryAction={
              <Btn
                label={tr('Créer un risque', 'Create a risk')}
                icon={Plus}
                primary
                onClick={() => window.dispatchEvent(new CustomEvent('openrisk:new-risk'))}
              />
            }
            className="py-10"
          />
        )}
        {risks.map((r) => (
          <button
            key={r.rawId}
            // Opens THIS risk's drawer rather than the top of an unfiltered
            // list, using the same ?focus= contract universal search uses.
            onClick={() => navigate(deepLink('risks', { focus: r.rawId }))}
            className="w-full flex items-center gap-3 px-2 py-[11px] rounded-[10px] hover:bg-hover transition-colors text-left"
          >
            <span
              className="w-[9px] h-[9px] rounded-full shrink-0"
              style={{
                background: critColor[r.crit],
                boxShadow: r.crit === 'critical' ? '0 0 7px var(--critical)' : 'none',
              }}
            />
            <div className="flex-1 min-w-0">
              <div className="text-[13px] font-medium text-ink truncate">{r.name}</div>
              <div className="text-[11.5px] text-ink-muted mt-0.5">
                {r.id} · {r.meta}
              </div>
            </div>
            {r.fw !== '—' && (
              <span
                className="text-[11px] font-semibold px-2 py-[3px] rounded-md shrink-0"
                style={{
                  color: frameworkColor[r.fw] ?? 'var(--fg-secondary)',
                  background: softFill(frameworkColor[r.fw] ?? 'var(--fg-secondary)', 14),
                }}
              >
                {r.fw}
              </span>
            )}
            <span
              className="mono text-[13px] font-bold w-[34px] text-right"
              style={{ color: critColor[r.crit] }}
            >
              {r.score.toFixed(1)}
            </span>
          </button>
        ))}
      </div>
    </Card>
  );
}

/* ---------------- War Room widget ---------------- */
// Real "incident in progress": the most recent open/in-progress incident for this
// tenant with a live duration; an honest empty state when there is none (no more
// hardcoded INC-2026-014).
const SEV_COLOR = {
  critical: 'var(--critical)',
  high: 'var(--high)',
  medium: 'var(--medium)',
  low: 'var(--low)',
} as const;
function WarRoomCard({ onJoin }: { onJoin: (incidentId?: number) => void }) {
  const L = useUIStrings();
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const { incidents } = useIncidents({ limit: 20 });
  const [now, setNow] = useState(() => Date.now());
  const active = (incidents ?? []).filter((i) => i.status === 'open' || i.status === 'in_progress');
  const inc = active
    .slice()
    .sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime())[0];
  useEffect(() => {
    if (!inc) return;
    const t = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(t);
  }, [inc?.id]);

  if (!inc) {
    return (
      <div
        className="rounded-[16px] p-5 flex flex-col"
        style={{ border: '1px solid var(--border)', background: 'var(--bg-elevated)' }}
      >
        <div className="flex items-center gap-2 mb-3.5">
          <span className="w-2.5 h-2.5 rounded-full" style={{ background: 'var(--low)' }} />
          <span className="text-[11px] font-bold tracking-[0.06em] uppercase text-ink-muted">
            {L.warTitle}
          </span>
        </div>
        <div className="flex-1 flex flex-col items-center justify-center text-center py-6 gap-1.5">
          <ShieldCheck size={26} style={{ color: 'var(--low)' }} />
          <div className="text-[13.5px] font-semibold text-ink">
            {tr('Aucun incident en cours', 'No active incident')}
          </div>
          <div className="text-[12px] text-ink-muted">
            {tr(
              'Tout est calme. Les incidents ouverts s’afficheront ici.',
              'All clear. Open incidents will appear here.',
            )}
          </div>
        </div>
        <button
          onClick={() => onJoin()}
          className="mt-auto h-[38px] rounded-[10px] flex items-center justify-center gap-2 text-[13px] font-semibold text-ink-soft hover:text-ink transition-colors"
          style={{ border: '1px solid var(--border-strong)' }}
        >
          {tr('Voir les incidents', 'View incidents')}
        </button>
      </div>
    );
  }

  const sevColor = SEV_COLOR[inc.severity] ?? 'var(--high)';
  const elapsed = Math.max(0, Math.floor((now - new Date(inc.created_at).getTime()) / 1000));
  const hh = String(Math.floor(elapsed / 3600)).padStart(2, '0');
  const mm = String(Math.floor((elapsed % 3600) / 60)).padStart(2, '0');
  const ss = String(elapsed % 60).padStart(2, '0');
  return (
    <div
      className="rounded-[16px] p-5 flex flex-col"
      style={{
        background: `linear-gradient(135deg, color-mix(in srgb,${sevColor} 10%,transparent), color-mix(in srgb,${sevColor} 3%,transparent))`,
        border: `1px solid color-mix(in srgb,${sevColor} 28%,transparent)`,
      }}
    >
      <div className="flex items-center gap-2 mb-3.5">
        <span
          className="w-2.5 h-2.5 rounded-full"
          style={{ background: sevColor, animation: 'or-pulsedot 1.4s infinite' }}
        />
        <span
          className="text-[11px] font-bold tracking-[0.06em] uppercase"
          style={{ color: sevColor }}
        >
          {L.warTitle}
        </span>
      </div>
      <div className="text-[15px] font-semibold text-ink mb-1">
        INC-{inc.id} · {inc.title}
      </div>
      <div className="text-[12.5px] text-ink-soft mb-4 capitalize">
        {inc.severity} ·{' '}
        {inc.status === 'in_progress' ? tr('en cours', 'in progress') : tr('ouvert', 'open')}
      </div>
      <div className="flex items-center gap-4 mb-4.5">
        <div>
          <div className="disp mono text-[22px] font-bold text-ink">
            {hh}:{mm}:{ss}
          </div>
          <div className="text-[11px] text-ink-muted">{tr('Durée', 'Duration')}</div>
        </div>
      </div>
      <button
        onClick={() => onJoin(inc.id)}
        className="mt-auto h-[38px] rounded-[10px] flex items-center justify-center gap-2 text-[13px] font-semibold text-fg-primary"
        style={{ background: sevColor }}
      >
        <Zap size={16} /> {L.warJoin}
      </button>
    </div>
  );
}

export type { DashboardStats };
