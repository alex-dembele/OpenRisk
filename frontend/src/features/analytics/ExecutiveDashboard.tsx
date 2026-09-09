// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
//
// Executive dashboard (spec §11 « Tableau de bord exécutif ») — a board-level view
// of the whole security posture: a cyber-score grade, financial exposure, key risk
// indicators, the top-10 risks, risk & incident trends and compliance coverage.
// Every figure comes from ONE consolidated request (GET /analytics/executive) — no
// fixtures. Charts follow the project's dc.html tokens: reserved status colours for
// severity (with labels/legend), one hue for magnitude, ink tokens for text, and
// they render in light + dark.

import { localeTag, type LocaleCode } from '../../i18n/locales';
import { useMemo } from 'react';
import { toast } from 'sonner';
import { CartesianChart, RadarChart } from '../../shared/ds/charts';
import {
  ShieldCheck,
  RefreshCw,
  Coins,
  AlertTriangle,
  Bug,
  Activity,
  TrendingUp,
  type LucideIcon,
} from 'lucide-react';
import { PageFrame, PageHeader, Card, Btn, Skeleton, ErrorState } from '../../shared/ui';
import { softFill } from '../../shared/riskColors';
import { useUIStore } from '../../store/uiStore';
import { useExecutiveDashboard } from './useExecutive';
import type {
  ExecutiveDashboard as ExecData,
  CyberScore,
  KRI,
  ExecRisk,
  ComplianceCoverage,
  MonthlyRiskPoint,
  IncidentTrendPoint,
  DistributionSlice,
} from './executiveService';

/* ---------------- colours & formatters ---------------- */

const CRIT: Record<string, string> = {
  critical: 'var(--critical)',
  high: 'var(--high)',
  medium: 'var(--medium)',
  low: 'var(--low)',
};
const SEV_COLOR: Record<string, string> = {
  critical: 'var(--critical)',
  warn: 'var(--high)',
  ok: 'var(--low)',
};

function fmtInt(n: number, lang: LocaleCode): string {
  return Math.round(n).toLocaleString(localeTag(lang));
}
function fmtCompactFCFA(n: number, lang: LocaleCode): string {
  const abs = Math.abs(n);
  const u = lang === 'fr' ? { b: ' Md', m: ' M', k: ' k' } : { b: 'B', m: 'M', k: 'K' };
  const f = (v: number) => (lang === 'fr' ? v.toFixed(1).replace('.', ',') : v.toFixed(1));
  if (abs >= 1e9) return `${f(n / 1e9)}${u.b} FCFA`;
  if (abs >= 1e6) return `${f(n / 1e6)}${u.m} FCFA`;
  if (abs >= 1e3) return `${f(n / 1e3)}${u.k} FCFA`;
  return `${Math.round(n)} FCFA`;
}
/** "2026-07" → "07/26" (compact, locale-neutral). */
function monthLabel(m: string): string {
  const [y, mo] = m.split('-');
  return y && mo ? `${mo}/${y.slice(2)}` : m;
}
function gradeColor(grade: string): string {
  switch (grade) {
    case 'A':
    case 'B':
      return 'var(--low)';
    case 'C':
      return 'var(--medium)';
    case 'D':
      return 'var(--high)';
    default:
      return 'var(--critical)';
  }
}

/* ---------------- SVG gauge helpers ---------------- */
function polar(cx: number, cy: number, r: number, deg: number): [number, number] {
  const a = ((deg - 90) * Math.PI) / 180;
  return [cx + r * Math.cos(a), cy + r * Math.sin(a)];
}
function arcPath(cx: number, cy: number, r: number, a0: number, a1: number): string {
  const [x0, y0] = polar(cx, cy, r, a1);
  const [x1, y1] = polar(cx, cy, r, a0);
  const large = a1 - a0 <= 180 ? 0 : 1;
  return `M ${x0} ${y0} A ${r} ${r} 0 ${large} 0 ${x1} ${y1}`;
}

/* ---------------- page ---------------- */

export function ExecutiveDashboard() {
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const { data, isLoading, isError, refetch, isFetching } = useExecutiveDashboard();

  if (isLoading) return <ExecSkeleton />;
  if (isError || !data) {
    return (
      <PageFrame>
        <PageHeader title={tr('Tableau de bord exécutif', 'Executive dashboard')} />
        <ErrorState
          title={tr('Impossible de charger le tableau de bord', 'Could not load the dashboard')}
          description={tr('Réessayez dans un instant.', 'Please try again in a moment.')}
          onRetry={() => refetch()}
          retryLabel={tr('Réessayer', 'Retry')}
        />
      </PageFrame>
    );
  }

  const gen = new Date(data.generated_at).toLocaleString(localeTag(lang), {
    dateStyle: 'medium',
    timeStyle: 'short',
  });

  return (
    <PageFrame wide>
      <PageHeader
        title={tr('Tableau de bord exécutif', 'Executive dashboard')}
        count={tr(`Généré le ${gen}`, `Generated ${gen}`)}
        actions={
          // "Actualiser" re-queries /analytics/executive. On a dashboard that
          // already refetches every 60 s the new payload is often identical, so
          // without a visible in-flight state the button reads as dead: it now
          // disables + spins while fetching and confirms when the data lands.
          <Btn
            label={isFetching ? tr('Actualisation…', 'Refreshing…') : tr('Actualiser', 'Refresh')}
            icon={RefreshCw}
            disabled={isFetching}
            className={isFetching ? '[&>svg]:animate-spin' : ''}
            onClick={async () => {
              const res = await refetch();
              if (res.error) toast.error(tr('Actualisation impossible', 'Could not refresh'));
              else toast.success(tr('Tableau de bord à jour', 'Dashboard up to date'));
            }}
          />
        }
      />

      {/* row 1 — cyber score + financial + KRIs */}
      <div className="grid grid-cols-1 lg:grid-cols-[300px_1fr] gap-4 mb-4">
        <CyberScoreCard cs={data.cyber_score} tr={tr} />
        <div className="flex flex-col gap-4">
          <FinancialCard data={data} lang={lang} tr={tr} />
          <KriStrip kris={data.kris} lang={lang} />
        </div>
      </div>

      {/* row 2 — risk trend + distribution */}
      <div className="grid grid-cols-1 lg:grid-cols-[1.6fr_1fr] gap-4 mb-4">
        <RiskTrendCard points={data.risk_trend} tr={tr} />
        <RiskDistributionCard slices={data.risk_distribution} lang={lang} tr={tr} />
      </div>

      {/* row 3 — top risks + control coverage radar */}
      <div className="grid grid-cols-1 lg:grid-cols-[1.5fr_1fr] gap-4 mb-4">
        <TopRisksCard risks={data.top_risks} lang={lang} tr={tr} />
        <ControlCoverageRadar frameworks={data.compliance} tr={tr} />
      </div>

      {/* row 4 — compliance donuts + incident trend */}
      <div className="grid grid-cols-1 lg:grid-cols-[1fr_1fr] gap-4">
        <ComplianceCard frameworks={data.compliance} tr={tr} />
        <IncidentTrendCard points={data.incident_trend} tr={tr} />
      </div>
    </PageFrame>
  );
}

/* ---------------- Cyber score ---------------- */
function CyberScoreCard({ cs, tr }: { cs: CyberScore; tr: (f: string, e: string) => string }) {
  const col = gradeColor(cs.grade);
  const cx = 100,
    cy = 104,
    r = 72;
  const track = arcPath(cx, cy, r, -115, 115);
  const prog = arcPath(cx, cy, r, -115, -115 + 230 * (cs.score / 100));
  return (
    <Card style={{ padding: '18px 20px' }}>
      <div className="text-[13px] font-semibold text-ink-soft mb-1">
        {tr('Cyber score', 'Cyber score')}
      </div>
      <div className="relative flex justify-center">
        <svg viewBox="0 0 200 140" width="200" height="140">
          <path
            d={track}
            fill="none"
            stroke="var(--bg-hover)"
            strokeWidth={13}
            strokeLinecap="round"
          />
          <path
            d={prog}
            fill="none"
            stroke={col}
            strokeWidth={13}
            strokeLinecap="round"
            style={{ filter: `drop-shadow(0 0 6px ${col})` }}
          />
        </svg>
        <div className="absolute left-0 right-0 text-center" style={{ top: '44px' }}>
          <div className="disp mono text-[46px] font-bold leading-none" style={{ color: col }}>
            {cs.grade}
          </div>
          <div className="text-[13px] text-ink-soft mt-1">
            {cs.score} / 100 · {cs.label}
          </div>
        </div>
      </div>
      <div className="flex flex-col gap-2 mt-3">
        {cs.components.map((c) => (
          <div key={c.key} className="flex items-center gap-2.5">
            <span className="w-[92px] text-[11.5px] text-ink-soft shrink-0 truncate">
              {c.label}
            </span>
            <div
              className="flex-1 h-[6px] rounded-full overflow-hidden"
              style={{ background: 'var(--bg-hover)' }}
            >
              <div
                className="h-full rounded-full"
                style={{ width: `${c.value}%`, background: col }}
              />
            </div>
            <span className="mono text-[11px] font-semibold text-ink w-[30px] text-right">
              {c.value}
            </span>
          </div>
        ))}
        {cs.components.length === 0 && (
          <div className="text-[12px] text-ink-muted py-2 text-center">
            {tr('Données insuffisantes', 'Not enough data')}
          </div>
        )}
      </div>
    </Card>
  );
}

/* ---------------- Financial exposure ---------------- */
function FinancialCard({
  data,
  lang,
  tr,
}: {
  data: ExecData;
  lang: LocaleCode;
  tr: (f: string, e: string) => string;
}) {
  const f = data.financial;
  return (
    <Card style={{ padding: '18px 20px' }}>
      <div className="flex items-start justify-between">
        <div>
          <div className="flex items-center gap-2 text-[12.5px] text-ink-soft mb-2">
            <Coins size={15} style={{ color: 'var(--accent-500)' }} />
            {tr('Exposition financière annuelle (ALE)', 'Annual financial exposure (ALE)')}
          </div>
          <div className="disp mono text-[30px] font-bold text-ink leading-none">
            {fmtCompactFCFA(f.total_ale.xaf, lang)}
          </div>
          <div className="text-[12px] text-ink-muted mt-1.5">
            {tr('Pire cas', 'Worst case')} :{' '}
            <span className="font-semibold" style={{ color: 'var(--critical)' }}>
              {fmtCompactFCFA(f.total_ale_worst.xaf, lang)}
            </span>
            {' · '}${fmtInt(f.total_ale.usd, 'en')}
          </div>
        </div>
        <div className="text-right shrink-0">
          <div className="mono text-[22px] font-bold text-ink">
            {f.quantified_risks}/{f.total_risks}
          </div>
          <div className="text-[11px] text-ink-muted">
            {tr('risques quantifiés', 'quantified risks')}
          </div>
        </div>
      </div>
    </Card>
  );
}

/* ---------------- KRI strip ---------------- */
const KRI_ICON: Record<string, LucideIcon> = {
  open_vulns: Bug,
  kev_exploited: AlertTriangle,
  critical_vulns: Bug,
  critical_risks: ShieldCheck,
  open_incidents: Activity,
  avg_mttr_days: TrendingUp,
  compliance_coverage: ShieldCheck,
};
function KriStrip({ kris, lang }: { kris: KRI[]; lang: LocaleCode }) {
  if (kris.length === 0) return null;
  return (
    <div className="grid grid-cols-2 sm:grid-cols-3 xl:grid-cols-4 gap-3">
      {kris.map((k) => {
        const col = SEV_COLOR[k.severity] ?? 'var(--accent)';
        const Icon = KRI_ICON[k.key] ?? Activity;
        const val =
          k.unit === '%'
            ? `${fmtInt(k.value, lang)} %`
            : k.unit === 'days'
              ? `${k.value.toLocaleString(localeTag(lang), { maximumFractionDigits: 1 })} ${lang === 'fr' ? 'j' : 'd'}`
              : fmtInt(k.value, lang);
        return (
          <Card key={k.key} style={{ padding: '13px 15px' }}>
            <div className="flex items-center gap-2 mb-2">
              <span
                className="w-[26px] h-[26px] rounded-lg flex items-center justify-center"
                style={{ color: col, background: softFill(col, 14) }}
              >
                <Icon size={14} strokeWidth={1.9} />
              </span>
            </div>
            <div className="disp mono text-[22px] font-bold text-ink leading-none">{val}</div>
            <div className="text-[11.5px] text-ink-soft mt-1 leading-tight">{k.label}</div>
          </Card>
        );
      })}
    </div>
  );
}

/* ---------------- Risk trend (global risk level over time) ---------------- */
function RiskTrendCard({
  points,
  tr,
}: {
  points: MonthlyRiskPoint[];
  tr: (f: string, e: string) => string;
}) {
  const rows = points.map((p) => ({ ...p, m: monthLabel(p.month) }));
  return (
    <Card style={{ padding: '18px 20px' }}>
      <div className="text-[14px] font-semibold text-ink mb-1">
        {tr('Évolution du niveau de risque', 'Risk level trend')}
      </div>
      <div className="text-[12px] text-ink-muted mb-3">
        {tr('Score de risque moyen du registre', 'Register average risk score')}
      </div>
      {rows.length === 0 ? (
        <ChartEmpty label={tr("Pas encore d'historique", 'No history yet')} />
      ) : (
        <CartesianChart
          data={rows}
          x="m"
          height={220}
          series={[{ type: 'line', key: 'avg_score', label: tr('Score moyen', 'Avg score') }]}
          ariaLabel={tr(
            'Score de risque moyen du registre, par mois',
            'Register average risk score, per month',
          )}
        />
      )}
    </Card>
  );
}

/* ---------------- Risk distribution donut ---------------- */
type PieRow = { criticality: string; count: number; [k: string]: string | number };
function RiskDistributionCard({
  slices,
  lang,
  tr,
}: {
  slices: DistributionSlice[];
  lang: LocaleCode;
  tr: (f: string, e: string) => string;
}) {
  const data: PieRow[] = slices
    .filter((s) => s.count > 0)
    .map((s) => ({ criticality: s.criticality, count: s.count }));
  const total = slices.reduce((a, s) => a + s.count, 0);
  const label = (c: string) =>
    ({
      critical: tr('Critique', 'Critical'),
      high: tr('Élevé', 'High'),
      medium: tr('Moyen', 'Medium'),
      low: tr('Faible', 'Low'),
    })[c] ?? c;
  return (
    <Card style={{ padding: '18px 20px' }}>
      <div className="text-[14px] font-semibold text-ink mb-3">
        {tr('Répartition par criticité', 'Distribution by criticality')}
      </div>
      {total === 0 ? (
        <ChartEmpty label={tr('Aucun risque', 'No risks')} />
      ) : (
        <div>
          {/* D-025: this was a doughnut with the total in the middle, across four
              slices. That is the pattern #444's anti-cliché table bans RingChart
              for — "a number in the middle that a KPI tile would say better" —
              and four slices is past the three-slice pie limit. The total is now
              the KPI figure it always was, and the split is bars, which people
              compare by length instead of by angle. */}
          <div className="mb-2 flex items-baseline gap-1.5">
            <span className="disp mono text-[28px] font-bold text-ink leading-none">
              {fmtInt(total, lang)}
            </span>
            <span className="text-[11px] text-ink-muted">{tr('risques', 'risks')}</span>
          </div>
          <CartesianChart
            data={data}
            x="criticality"
            height={180}
            series={[{ type: 'bar', key: 'count', label: tr('Risques', 'Risks') }]}
            formatCategory={label}
            ariaLabel={tr('Nombre de risques par criticité', 'Risk count by criticality')}
          />
        </div>
      )}
      <div className="flex flex-wrap gap-x-4 gap-y-1.5 mt-3 justify-center">
        {slices.map((s) => (
          <span
            key={s.criticality}
            className="inline-flex items-center gap-1.5 text-[11.5px] text-ink-soft"
          >
            <span className="w-2.5 h-2.5 rounded-sm" style={{ background: CRIT[s.criticality] }} />
            {label(s.criticality)} · <span className="font-semibold text-ink">{s.count}</span>
          </span>
        ))}
      </div>
    </Card>
  );
}

/* ---------------- Top 10 risks ---------------- */
function TopRisksCard({
  risks,
  lang,
  tr,
}: {
  risks: ExecRisk[];
  lang: LocaleCode;
  tr: (f: string, e: string) => string;
}) {
  return (
    <Card style={{ padding: '18px 8px 12px' }}>
      <div className="text-[14px] font-semibold text-ink mb-2 px-3">
        {tr('Top 10 des risques', 'Top 10 risks')}
      </div>
      {risks.length === 0 ? (
        <ChartEmpty label={tr('Aucun risque', 'No risks')} />
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-left" style={{ minWidth: 420 }}>
            <thead>
              <tr className="text-[11px] text-ink-muted uppercase tracking-wide">
                <th className="font-semibold px-3 py-1.5">{tr('Risque', 'Risk')}</th>
                <th className="font-semibold px-3 py-1.5">{tr('Criticité', 'Criticality')}</th>
                <th className="font-semibold px-3 py-1.5 text-right">{tr('Score', 'Score')}</th>
                <th className="font-semibold px-3 py-1.5 text-right">ALE</th>
              </tr>
            </thead>
            <tbody>
              {risks.map((r) => {
                const col = CRIT[r.criticality] ?? 'var(--fg-muted)';
                return (
                  <tr key={r.id} className="hover:bg-hover transition-colors">
                    <td className="px-3 py-[9px]">
                      <div className="flex items-center gap-2">
                        <span
                          className="w-2 h-2 rounded-full shrink-0"
                          style={{ background: col }}
                        />
                        <span className="text-[13px] font-medium text-ink truncate max-w-[220px]">
                          {r.title}
                        </span>
                      </div>
                    </td>
                    <td className="px-3 py-[9px]">
                      <span
                        className="text-[11px] font-semibold px-2 py-[3px] rounded-md"
                        style={{ color: col, background: softFill(col, 14) }}
                      >
                        {r.criticality}
                      </span>
                    </td>
                    <td
                      className="px-3 py-[9px] text-right mono text-[13px] font-bold"
                      style={{ color: col }}
                    >
                      {r.score.toFixed(1)}
                    </td>
                    <td className="px-3 py-[9px] text-right mono text-[12px] text-ink-soft">
                      {r.ale.xaf > 0 ? fmtCompactFCFA(r.ale.xaf, lang) : '—'}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </Card>
  );
}

/* ---------------- Control coverage radar ---------------- */
function ControlCoverageRadar({
  frameworks,
  tr,
}: {
  frameworks: ComplianceCoverage[];
  tr: (f: string, e: string) => string;
}) {
  const data = frameworks.slice(0, 8).map((f) => ({ name: f.name, percent: f.percent }));
  return (
    <Card style={{ padding: '18px 20px' }}>
      <div className="text-[14px] font-semibold text-ink mb-1">
        {tr('Couverture des contrôles', 'Control coverage')}
      </div>
      <div className="text-[12px] text-ink-muted mb-2">
        {tr('% implémenté par référentiel', '% implemented per framework')}
      </div>
      {data.length === 0 ? (
        <ChartEmpty label={tr('Aucun référentiel', 'No frameworks')} />
      ) : (
        <RadarChart
          axes={data.map((d) => ({ key: d.name, label: d.name }))}
          series={[
            {
              key: 'coverage',
              label: tr('Couverture', 'Coverage'),
              values: data.map((d) => d.percent),
            },
          ]}
          max={100}
          size={230}
          formatValue={(v) => `${v} %`}
          ariaLabel={tr(
            'Pourcentage de contrôles implémentés par référentiel',
            'Percentage of controls implemented per framework',
          )}
        />
      )}
    </Card>
  );
}

/* ---------------- Compliance donuts per framework ---------------- */
/* `Ring` — a 72px progress ring with the percentage in the middle, eight of them
   in a grid — was deleted under D-025. It is the same banned pattern as the
   distribution doughnut ("a number in the middle that a KPI tile would say
   better"), and eight of them side by side asked the reader to compare eight
   arc lengths. The comparison across frameworks IS the reading, so it is now
   one bar chart: same data, one axis, ranked by eye in a glance. */
function ComplianceCard({
  frameworks,
  tr,
}: {
  frameworks: ComplianceCoverage[];
  tr: (f: string, e: string) => string;
}) {
  return (
    <Card style={{ padding: '18px 20px' }}>
      <div className="text-[14px] font-semibold text-ink mb-4">
        {tr('Conformité par référentiel', 'Compliance by framework')}
      </div>
      {frameworks.length === 0 ? (
        <ChartEmpty label={tr('Aucun référentiel', 'No frameworks')} />
      ) : (
        <CartesianChart
          data={frameworks.slice(0, 8)}
          x="name"
          height={200}
          series={[{ type: 'bar', key: 'percent', label: tr('Couverture', 'Coverage') }]}
          formatValue={(v) => `${Math.round(v)}%`}
          ariaLabel={tr(
            'Pourcentage de conformité par référentiel',
            'Compliance percentage per framework',
          )}
        />
      )}
    </Card>
  );
}

/* ---------------- Incident trend histogram ---------------- */
function IncidentTrendCard({
  points,
  tr,
}: {
  points: IncidentTrendPoint[];
  tr: (f: string, e: string) => string;
}) {
  const rows = useMemo(
    () =>
      points.map((p) => ({
        m: monthLabel(p.month),
        critical: p.critical,
        high: p.high,
        other: Math.max(0, p.total - p.critical - p.high),
      })),
    [points],
  );
  const hasData = points.some((p) => p.total > 0);
  return (
    <Card style={{ padding: '18px 20px' }}>
      <div className="text-[14px] font-semibold text-ink mb-1">
        {tr('Tendance des incidents', 'Incident trend')}
      </div>
      <div className="text-[12px] text-ink-muted mb-3">
        {tr('Volume mensuel par sévérité', 'Monthly volume by severity')}
      </div>
      {!hasData ? (
        <ChartEmpty label={tr('Aucun incident', 'No incidents')} />
      ) : (
        <CartesianChart
          data={rows}
          x="m"
          height={220}
          stacked
          series={[
            { type: 'bar', key: 'critical', label: tr('Critique', 'Critical'), band: 'critical' },
            { type: 'bar', key: 'high', label: tr('Élevé', 'High'), band: 'high' },
            { type: 'bar', key: 'other', label: tr('Autre', 'Other'), band: 'medium' },
          ]}
          ariaLabel={tr(
            'Volume mensuel d’incidents par sévérité',
            'Monthly incident volume by severity',
          )}
        />
      )}
    </Card>
  );
}

/* ---------------- shared small bits ---------------- */
function ChartEmpty({ label }: { label: string }) {
  return (
    <div className="h-[180px] flex items-center justify-center text-[13px] text-ink-muted">
      {label}
    </div>
  );
}

function ExecSkeleton() {
  return (
    <PageFrame wide>
      <PageHeader title="Executive dashboard" />
      <div className="grid grid-cols-1 lg:grid-cols-[300px_1fr] gap-4 mb-4">
        <Skeleton style={{ height: 300 }} />
        <div className="flex flex-col gap-4">
          <Skeleton style={{ height: 92 }} />
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
            {Array.from({ length: 4 }).map((_, i) => (
              <Skeleton key={i} style={{ height: 92 }} />
            ))}
          </div>
        </div>
      </div>
      <div className="grid grid-cols-1 lg:grid-cols-[1.6fr_1fr] gap-4 mb-4">
        <Skeleton style={{ height: 290 }} />
        <Skeleton style={{ height: 290 }} />
      </div>
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <Skeleton style={{ height: 260 }} />
        <Skeleton style={{ height: 260 }} />
      </div>
    </PageFrame>
  );
}
