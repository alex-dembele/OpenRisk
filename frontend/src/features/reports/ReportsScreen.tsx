// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// Reports (OpenRisk.dc.html §6.14): a grid of report templates wired to real
// destinations/exports (Board report, Compliance PDFs, risk-register CSV export…),
// plus a recent-reports list.

import { localeTag } from '../../i18n/locales';
import {
  TrendingUp,
  FileText,
  ClipboardCheck,
  Siren,
  ShieldAlert,
  Atom,
  Sparkles,
  Plus,
  CalendarClock,
  type LucideIcon,
} from 'lucide-react';
import { useState } from 'react';
import { useNavigate } from 'react-router';
import { toast } from 'sonner';
import { PageFrame, PageHeader, Card, Btn, SkeletonRows } from '../../shared/ui';
import { EmptyState } from '../../shared/EmptyState';
import { useBoardReports } from './useBoardReports';
import { useGenerateReport } from './useReportJobs';
import { useFrameworks } from '../compliance/useCompliance';
import { FrameworkPickerDialog } from './FrameworkPickerDialog';
import { useUIStrings } from '../../shared/uiStrings';
import { useUIStore } from '../../store/uiStore';
import { riskService } from '../../services/riskService';
import { exportIncidentsCsv } from '../incidents/incidentService';

export function ReportsScreen() {
  const L = useUIStrings();
  const lang = useUIStore((s) => s.lang);
  const navigate = useNavigate();
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const { reports, isLoading: reportsLoading, error: reportsError } = useBoardReports();
  const { frameworks } = useFrameworks();
  const generate = useGenerateReport();
  const [picking, setPicking] = useState(false);
  const recent = (reports ?? []).slice(0, 5);

  const exportRegister = async () => {
    try {
      const blob = await riskService.exportRisks({}, 'csv');
      const url = URL.createObjectURL(blob as Blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `risk-register-${new Date().toISOString().slice(0, 10)}.csv`;
      a.click();
      URL.revokeObjectURL(url);
      toast.success(tr('Registre exporté', 'Register exported'));
    } catch {
      toast.error(tr('Export échoué', 'Export failed'));
    }
  };

  const exportIncidents = async () => {
    try {
      const n = await exportIncidentsCsv();
      toast.success(tr(`${n} incident(s) exporté(s)`, `${n} incident(s) exported`));
    } catch {
      toast.error(tr('Export échoué', 'Export failed'));
    }
  };

  const tpls: [string, string, LucideIcon, () => void][] = [
    [
      tr('Synthèse exécutive', 'Executive summary'),
      tr(
        'Vue d’ensemble de la posture pour le COMEX',
        'Posture overview for the executive committee',
      ),
      TrendingUp,
      () => navigate('/?view=executive'),
    ],
    [
      tr('Rapport Conseil', 'Board report'),
      tr('Reporting trimestriel de gouvernance', 'Quarterly governance reporting'),
      FileText,
      () => navigate('/reports/board'),
    ],
    // The other half of the loop. This used to navigate to /compliance, whose
    // "Generate report" button navigated straight back here — a closed circuit
    // that never produced a PDF. It now picks a framework and generates, so the
    // journey ends on the document.
    [
      tr('Conformité', 'Compliance'),
      tr('Rapport PDF détaillé par référentiel', 'Detailed PDF report per framework'),
      ClipboardCheck,
      () => setPicking(true),
    ],
    [
      tr('Registre d’incidents', 'Incident register'),
      tr('Tous les incidents en CSV', 'All incidents as CSV'),
      Siren,
      exportIncidents,
    ],
    [
      tr('Export du registre', 'Register export'),
      tr('Tous les risques en CSV', 'All risks as CSV'),
      ShieldAlert,
      exportRegister,
    ],
    [
      tr('Rapport de topologie', 'Topology report'),
      tr('Cartographie et chemins d’attaque', 'Topology and attack paths'),
      Atom,
      () => navigate('/assets/topology'),
    ],
  ];

  return (
    <PageFrame>
      <PageHeader title={L.n_reports} />
      <div
        className="grid gap-4 mb-7"
        style={{ gridTemplateColumns: 'repeat(auto-fill,minmax(280px,1fr))' }}
      >
        {tpls.map(([title, desc, Icon, run], i) => (
          <Card
            key={title}
            style={{
              padding: 20,
              animation: 'or-fadeup .4s ease both',
              animationDelay: `${i * 0.04}s`,
            }}
          >
            <div
              className="w-[42px] h-[42px] rounded-xl flex items-center justify-center mb-3.5"
              style={{ background: 'var(--accent-soft)', color: 'var(--accent-500)' }}
            >
              <Icon size={21} />
            </div>
            <div className="text-[14.5px] font-semibold text-ink mb-1.5">{title}</div>
            <div
              className="text-[12.5px] text-ink-soft leading-relaxed mb-4"
              style={{ minHeight: 36 }}
            >
              {desc}
            </div>
            <button
              onClick={run}
              className="w-full h-9 rounded-[10px] text-[13px] font-semibold text-ink inline-flex items-center justify-center gap-1.5 hover:bg-hover transition-colors"
              style={{ border: '1px solid var(--border-strong)' }}
            >
              <Sparkles size={15} /> {tr('Générer', 'Generate')}
            </button>
          </Card>
        ))}
      </div>
      {/* Scheduled reports do not exist. This block used to render three
          schedules — "Executive summary · Every Monday 08:00 · Board email" and
          two more — blurred behind an upsell, each with a lit toggle, on every
          tenant.

          Blurring invented rows does not make them less invented; it makes them
          harder to check. The same call was made for the leaderboard's seven
          invented colleagues, and the reasoning holds here: a user peering at
          the blur reads three schedules their organisation does not have.

          The capability is described instead. The `PremiumPeek` primitive went
          with it: this was its only use, and a component whose whole job is
          "blur this and put an upsell over it" invites exactly this mistake.
          `FeatureGate`/`UpsellLock` remain for real paywalls — there the blurred
          children ARE the real component rendering real tenant data, and the
          backend returns 402 regardless, which is a wall, not a fiction
          (W0-05 / D5). */}
      <Card style={{ padding: '18px 22px', marginBottom: 28 }}>
        <div className="flex items-start gap-3.5">
          <div
            className="w-10 h-10 rounded-[11px] flex items-center justify-center shrink-0"
            style={{ background: 'var(--bg-hover)', color: 'var(--fg-muted)' }}
          >
            <CalendarClock size={20} />
          </div>
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2 mb-1 flex-wrap">
              <span className="text-[14px] font-semibold text-ink">
                {tr('Rapports programmés', 'Scheduled reports')}
              </span>
              <span
                className="text-[10.5px] font-bold uppercase tracking-[.06em] px-2 py-0.5 rounded-full"
                style={{ color: 'var(--fg-muted)', background: 'var(--bg-hover)' }}
                data-testid="scheduled-reports-unavailable"
              >
                {tr('Indisponible', 'Unavailable')}
              </span>
            </div>
            <div className="text-[12.5px] text-ink-soft leading-relaxed">
              {tr(
                'La livraison automatique par e-mail (hebdomadaire ou mensuelle) n’est pas encore disponible. En attendant, générez un rapport ci-dessus : il est produit à la demande et reste dans la bibliothèque, horodaté et signé.',
                'Automatic delivery by e-mail (weekly or monthly) is not available yet. In the meantime, generate a report above: it is produced on demand and stays in the library, timestamped and hashed.',
              )}
            </div>
            <button
              onClick={() => navigate('/reports/library')}
              className="mt-3 h-9 px-3.5 rounded-[10px] text-[12.5px] font-semibold text-ink inline-flex items-center gap-1.5 hover:bg-hover transition-colors"
              style={{ border: '1px solid var(--border-strong)' }}
            >
              <FileText size={14} /> {tr('Ouvrir la bibliothèque', 'Open the library')}
            </button>
          </div>
        </div>
      </Card>

      <Card style={{ padding: '18px 22px' }}>
        <div className="text-[14px] font-semibold text-ink mb-3.5">
          {tr('Rapports récents', 'Recent reports')}
        </div>
        {/* The tenant's actual generated board reports. This list used to be three
            invented PDFs with invented dates, offering a Download button that did
            nothing — a fresh tenant appeared to have a reporting history. */}
        {reportsLoading ? (
          <SkeletonRows rows={3} height={52} />
        ) : reportsError ? (
          <EmptyState
            variant="error"
            title={tr('Rapports indisponibles', 'Reports unavailable')}
            description={tr(
              'Impossible de charger vos rapports générés.',
              'Could not load your generated reports.',
            )}
          />
        ) : recent.length === 0 ? (
          <EmptyState
            variant="first-use"
            icon={FileText}
            title={tr('Aucun rapport généré', 'No reports yet')}
            description={tr(
              'Les rapports que vous générez sont archivés ici, prêts à être relus ou téléchargés. Commencez par un rapport Conseil.',
              'Reports you generate are archived here, ready to review or download. Start with a board report.',
            )}
            primaryAction={
              <Btn
                label={tr('Générer un rapport Conseil', 'Generate a board report')}
                icon={Plus}
                primary
                onClick={() => navigate('/reports/board')}
              />
            }
          />
        ) : (
          recent.map((r, i) => (
            <button
              key={r.id}
              onClick={() => navigate(`/reports/board?focus=${r.id}`)}
              className="w-full text-left flex items-center gap-3.5 py-3 px-1 hover:bg-hover transition-colors"
              style={{ borderTop: i ? '1px solid var(--border)' : 'none' }}
            >
              <div
                className="w-[34px] h-[34px] rounded-[9px] flex items-center justify-center text-ink-soft shrink-0"
                style={{ background: 'var(--bg-hover)' }}
              >
                <FileText size={17} />
              </div>
              <div className="flex-1 min-w-0">
                <div className="text-[13.5px] font-medium text-ink truncate">{r.title}</div>
                <div className="text-[11.5px] text-ink-muted mt-0.5">
                  {r.period_label} ·{' '}
                  {new Date(r.created_at).toLocaleDateString(localeTag(lang), {
                    day: '2-digit',
                    month: 'short',
                    year: 'numeric',
                  })}
                </div>
              </div>
              <span
                className="text-[11.5px] font-semibold px-2 py-[3px] rounded-md shrink-0"
                style={{ color: 'var(--fg-secondary)', background: 'var(--bg-hover)' }}
              >
                {r.status === 'approved' ? tr('Approuvé', 'Approved') : tr('Brouillon', 'Draft')}
              </span>
            </button>
          ))
        )}
      </Card>
      {picking && (
        <FrameworkPickerDialog
          frameworks={frameworks ?? []}
          busy={generate.isPending}
          onClose={() => setPicking(false)}
          onPick={(id) =>
            generate.mutate(
              { kind: 'compliance_framework', params: { framework_id: id, locale: lang } },
              { onSettled: () => setPicking(false) },
            )
          }
        />
      )}
    </PageFrame>
  );
}
