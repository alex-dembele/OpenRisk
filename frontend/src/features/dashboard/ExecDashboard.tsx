// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// Executive dashboard persona (UX-2). Leads with cost & KPIs, no technical detail:
// the A–F cyber score, financial exposure (ALE, FCFA) and the key risk indicators.
// Real data from the consolidated /analytics/executive endpoint.
//
// W0-06: every value on this screen was `?? 0`. A failed fetch of
// /analytics/executive rendered a cyber score of 0 with grade F, an annual
// exposure of 0 FCFA and an empty KRI band — a complete, plausible, entirely
// unread executive briefing. On the persona whose whole job is to act on these
// numbers, and with no loading state to suggest anything was still in flight.
//
// Nothing is rendered now until it has been read. A failure says so, and offers
// a retry; a genuine zero is still a zero, and is now distinguishable from one.

import { localeTag } from '../../i18n/locales';
import { useNavigate } from 'react-router';
import { Coins, AlertTriangle, ShieldAlert, CheckCircle2 } from 'lucide-react';
import { useUIStore } from '../../store/uiStore';
import { useExecutiveDashboard } from '../analytics/useExecutive';
import { deepLink } from './deepLinks';
import { WidgetState } from './WidgetState';
import { DashboardShell, PersonaHeader, ScoreHero, StatCard, Card } from './shared';

const KRI_COL: Record<string, string> = {
  critical: 'var(--critical)',
  high: 'var(--high)',
  medium: 'var(--medium)',
  low: 'var(--low)',
  info: 'var(--fg-muted)',
};

export function ExecDashboard() {
  const navigate = useNavigate();
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const query = useExecutiveDashboard();
  const data = query.data;
  const cyber = data?.cyber_score;
  const fin = data?.financial;
  const kris = data?.kris ?? [];

  const money = (n: number) =>
    new Intl.NumberFormat(localeTag(lang), {
      notation: n >= 1_000_000 ? 'compact' : 'standard',
      maximumFractionDigits: 1,
    }).format(n) + ' FCFA';
  const kriUnit = (u: string) => (u === '%' ? '%' : u === 'days' ? tr(' j', ' d') : '');

  return (
    <DashboardShell>
      <PersonaHeader
        title={tr('Direction', 'Executive')}
        subtitle={tr(
          'La posture en un écran : score, exposition financière, indicateurs clés.',
          'Your posture at a glance: score, financial exposure, key indicators.',
        )}
        actionLabel={tr('Rapport conseil', 'Board report')}
        onAction={() => navigate('/reports')}
      />

      <WidgetState
        lang={lang}
        isLoading={query.isLoading}
        error={query.error}
        skeletonHeight={260}
        retry={() => void query.refetch()}
      >
        <div className="grid grid-cols-1 lg:grid-cols-[340px_1fr] gap-4 mb-4">
          <ScoreHero
            title={tr('Cyber score', 'Cyber score')}
            score={Math.round(cyber?.score ?? 0)}
            grade={cyber?.grade}
            hint={tr(
              'Note composite A–F sur 4 axes pondérés : conformité, risques, vulnérabilités, incidents.',
              'Composite A–F grade over 4 weighted axes: compliance, risks, vulnerabilities, incidents.',
            )}
            ctaLabel={tr('Voir Analytics', 'View analytics')}
            onDetails={() => navigate('/?view=executive')}
          />
          <div className="grid grid-cols-2 gap-4">
            <StatCard
              label={tr('Exposition annuelle (ALE)', 'Annual exposure (ALE)')}
              value={money(fin?.total_ale?.xaf ?? 0)}
              col="var(--accent)"
              icon={Coins}
              onClick={() => navigate('/analytics/financial')}
            />
            <StatCard
              label={tr('Pire cas', 'Worst case')}
              value={money(fin?.total_ale_worst?.xaf ?? 0)}
              col="var(--critical)"
              icon={AlertTriangle}
              onClick={() => navigate('/analytics/financial')}
            />
            <StatCard
              label={tr('Risques', 'Risks')}
              value={String(fin?.total_risks ?? 0)}
              col="var(--high)"
              icon={ShieldAlert}
              onClick={() => navigate(deepLink('risks', { sort: { key: 'score', dir: 'desc' } }))}
            />
            <StatCard
              label={tr('Quantifiés', 'Quantified')}
              value={String(fin?.quantified_risks ?? 0)}
              col="var(--low)"
              icon={CheckCircle2}
              onClick={() => navigate('/analytics/financial')}
            />
          </div>
        </div>
      </WidgetState>

      <Card style={{ padding: '18px 20px' }}>
        <div className="flex items-center justify-between mb-4">
          <div className="text-[14px] font-semibold text-ink">
            {tr('Indicateurs clés de risque (KRI)', 'Key risk indicators (KRI)')}
          </div>
          <button
            onClick={() => navigate('/?view=executive')}
            className="text-[12px] font-semibold text-accent hover:underline"
          >
            {tr('Détails', 'Details')}
          </button>
        </div>
        <WidgetState
          lang={lang}
          isLoading={query.isLoading}
          error={query.error}
          skeletonHeight={120}
          retry={() => void query.refetch()}
          isEmpty={kris.length === 0}
          emptyTitle={tr('Aucun indicateur calculable', 'No indicator can be computed yet')}
          emptyDescription={tr(
            'Les KRI se calculent à partir des risques, des vulnérabilités, de la conformité et des incidents. Ils apparaissent dès que l’une de ces sources contient des données.',
            'KRIs are computed from risks, vulnerabilities, compliance and incidents. They appear as soon as any of those holds data.',
          )}
        >
          <div className="grid grid-cols-2 lg:grid-cols-4 gap-3">
            {kris.map((k) => (
              <div
                key={k.key}
                className="rounded-xl p-3.5"
                style={{ background: 'var(--bg-hover)' }}
              >
                <div
                  className="mono text-[22px] font-bold leading-none"
                  style={{ color: KRI_COL[k.severity] ?? 'var(--fg-primary)' }}
                >
                  {new Intl.NumberFormat(localeTag(lang), {
                    maximumFractionDigits: 1,
                  }).format(k.value)}
                  {kriUnit(k.unit)}
                </div>
                <div className="text-[11.5px] text-ink-soft mt-1.5">{k.label}</div>
              </div>
            ))}
          </div>
        </WidgetState>
      </Card>
    </DashboardShell>
  );
}
