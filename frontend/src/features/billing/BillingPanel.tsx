// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// The self-service billing page (spec §3): current plan, usage vs limits, the
// price table for the tenant's region (PPP), 14-day no-card trial, plan change,
// cancellation, invoices — plus the opt-in telemetry consent. Reads the real
// entitlements/billing endpoints; the backend enforces every gate.

import { type BoundFormatters } from '../../i18n';
import { useFormat } from '../../hooks/useI18n';
import { useState } from 'react';
import { toast } from 'sonner';
import { Check, Sparkles } from 'lucide-react';
import {
  useEntitlements,
  useBilling,
  useStartTrial,
  useCheckout,
  useChangePlan,
  useCancelSubscription,
  useTelemetry,
  useSetTelemetry,
} from './useEntitlements';
import type { PlanKey, Price } from '../../services/entitlementService';

const PLANS: PlanKey[] = ['free', 'pro', 'business', 'enterprise'];
const PLAN_LABEL: Record<PlanKey, string> = {
  free: 'Free',
  pro: 'Pro',
  business: 'Business',
  enterprise: 'Enterprise',
};
const PLAN_PITCH: Record<PlanKey, string> = {
  free: 'Découvrir OpenRisk',
  pro: 'Équipes IT / Sécurité',
  business: 'PME / organisations',
  enterprise: 'Grandes entreprises / secteurs régulés',
};
const PLAN_HIGHLIGHTS: Record<PlanKey, string[]> = {
  free: ['2 utilisateurs', '50 risques', 'Conformité de base', 'Support communauté'],
  pro: [
    '10 utilisateurs',
    '500 risques',
    'Quantification financière',
    'IA GRC · Automatisation',
    'Support e-mail',
  ],
  business: [
    '50 utilisateurs',
    'Risques illimités',
    'SSO · Gouvernance · CTI',
    'SLA 99,5 %',
    'Support prioritaire',
  ],
  enterprise: [
    'Utilisateurs illimités',
    'Multi-organisation · On-premise',
    'SLA 99,9 %',
    'Support dédié',
  ],
};

function formatPrice(p: Price | undefined, fmt: BoundFormatters): string {
  if (!p) return '—';
  if (p.custom) return 'Sur devis';
  if (p.amount === 0) return 'Gratuit';
  if (p.currency === 'EUR') return `${p.amount} € / mois`;
  return `${fmt.currency(p.amount, { currency: p.currency })} / mois`;
}

const LIMIT_LABEL: Record<string, string> = {
  users: 'Utilisateurs',
  risks: 'Risques',
  assets: 'Actifs',
  integrations: 'Intégrations',
};

export function BillingPanel() {
  const fmt = useFormat();
  const { data: ent, isLoading } = useEntitlements();
  const { data: billing } = useBilling();
  const startTrial = useStartTrial();
  const checkout = useCheckout();
  const changePlan = useChangePlan();
  const cancel = useCancelSubscription();

  const [busyPlan, setBusyPlan] = useState<PlanKey | null>(null);

  if (isLoading || !ent) {
    return (
      <div className="h-40 rounded-[16px] animate-pulse" style={{ background: 'var(--bg-elev)' }} />
    );
  }

  const currentPlan = ent.plan;
  const region = ent.region;
  const sub = billing?.subscription ?? null;
  const providers = billing?.configured_providers ?? [];

  const act = async (plan: PlanKey) => {
    setBusyPlan(plan);
    try {
      if (plan === currentPlan) return;
      if (plan === 'enterprise') {
        window.location.href = 'mailto:sales@openrisk.io?subject=OpenRisk Enterprise';
        return;
      }
      if (plan === 'free') {
        await changePlan.mutateAsync({ plan, region });
        toast.success('Passage au plan Free effectué.');
        return;
      }
      // Paid plan: trial if never subscribed, otherwise checkout when a provider
      // is configured, otherwise start the trial as a graceful fallback.
      if (!sub) {
        await startTrial.mutateAsync({ plan, region });
        toast.success(`Essai ${PLAN_LABEL[plan]} de ${ent.trial_days} jours activé — sans carte.`);
        return;
      }
      if (providers.length > 0) {
        const session = await checkout.mutateAsync({ plan, region });
        window.location.href = session.url;
        return;
      }
      await changePlan.mutateAsync({ plan, region });
      toast.success(`Plan ${PLAN_LABEL[plan]} activé.`);
    } catch (e: unknown) {
      const err = e as { response?: { data?: { error?: string } } };
      toast.error(err.response?.data?.error ?? 'Action impossible pour le moment.');
    } finally {
      setBusyPlan(null);
    }
  };

  return (
    <div className="space-y-6">
      {/* Current plan + trial */}
      <div
        className="rounded-[16px] p-5"
        style={{ background: 'var(--bg-elev)', border: '1px solid var(--border)' }}
      >
        <div className="flex items-center justify-between flex-wrap gap-3">
          <div>
            <div className="text-[12px] uppercase tracking-wide text-ink-soft">Plan actuel</div>
            <div className="disp text-[24px] font-bold text-ink flex items-center gap-2">
              {PLAN_LABEL[currentPlan]}
              {sub?.status === 'trialing' && ent.trial?.active && (
                <span
                  className="text-[11px] font-semibold px-2 py-0.5 rounded-full text-accent"
                  style={{ background: 'color-mix(in srgb,var(--accent) 14%,transparent)' }}
                >
                  Essai · {ent.trial.days_left} j restants
                </span>
              )}
              {sub?.cancel_at_period_end && (
                <span
                  className="text-[11px] font-semibold px-2 py-0.5 rounded-full"
                  style={{
                    color: 'var(--medium)',
                    background: 'color-mix(in srgb,var(--medium) 14%,transparent)',
                  }}
                >
                  Résiliation programmée
                </span>
              )}
            </div>
          </div>
          {sub &&
            sub.status !== 'canceled' &&
            !sub.cancel_at_period_end &&
            currentPlan !== 'free' && (
              <button
                onClick={async () => {
                  try {
                    await cancel.mutateAsync();
                    toast.success('Résiliation enregistrée.');
                  } catch {
                    toast.error('Résiliation impossible.');
                  }
                }}
                className="h-9 px-4 rounded-[10px] text-[13px] font-semibold"
                style={{ border: '1px solid var(--border)', color: 'var(--ink-soft)' }}
              >
                Résilier
              </button>
            )}
        </div>
        {providers.length === 0 && (
          <div className="mt-3 text-[12.5px] text-ink-soft">
            Aucun moyen de paiement n'est configuré sur cette instance — les plans payants
            s'activent en essai gratuit ou via l'équipe OpenRisk.
          </div>
        )}
      </div>

      {/* Usage vs limits */}
      <div
        className="rounded-[16px] p-5"
        style={{ background: 'var(--bg-elev)', border: '1px solid var(--border)' }}
      >
        <div className="text-[13px] font-semibold text-ink mb-3">Usage vs limites</div>
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          {Object.entries(ent.limits).map(([key, l]) => {
            const unlimited = l.limit < 0;
            const pct = unlimited
              ? 0
              : Math.min(100, Math.round((l.used / Math.max(1, l.limit)) * 100));
            const near = !unlimited && pct >= 80;
            return (
              <div key={key}>
                <div className="flex justify-between text-[12.5px] mb-1">
                  <span className="text-ink-soft">{LIMIT_LABEL[key] ?? key}</span>
                  <span className="font-semibold text-ink">
                    {l.used} {unlimited ? '/ ∞' : `/ ${l.limit}`}
                  </span>
                </div>
                <div
                  className="h-2 rounded-full overflow-hidden"
                  style={{ background: 'var(--border)' }}
                >
                  <div
                    className="h-full rounded-full transition-[width]"
                    style={{
                      width: `${unlimited ? 6 : pct}%`,
                      background: near ? 'var(--high)' : 'var(--accent)',
                    }}
                  />
                </div>
              </div>
            );
          })}
        </div>
      </div>

      {/* Plan comparison */}
      <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-4 gap-4">
        {PLANS.map((plan) => {
          const isCurrent = plan === currentPlan;
          const price = ent.prices[plan];
          return (
            <div
              key={plan}
              className="rounded-[16px] p-5 flex flex-col"
              style={{
                background: 'var(--bg-elev)',
                border: `1px solid ${plan === 'pro' ? 'var(--accent)' : 'var(--border)'}`,
              }}
            >
              <div className="flex items-center gap-1.5">
                <span className="disp text-[18px] font-bold text-ink">{PLAN_LABEL[plan]}</span>
                {plan === 'pro' && <Sparkles size={14} className="text-accent" />}
              </div>
              <div className="text-[12px] text-ink-soft mb-2">{PLAN_PITCH[plan]}</div>
              <div className="disp text-[20px] font-bold text-ink mb-3">{formatPrice(price, fmt)}</div>
              <ul className="space-y-1.5 mb-4 flex-1">
                {PLAN_HIGHLIGHTS[plan].map((h) => (
                  <li key={h} className="flex items-start gap-1.5 text-[12.5px] text-ink-soft">
                    <Check size={13} className="mt-0.5 shrink-0 text-accent" /> {h}
                  </li>
                ))}
              </ul>
              <button
                disabled={isCurrent || busyPlan !== null}
                onClick={() => act(plan)}
                className="h-9 rounded-[10px] text-[13px] font-semibold transition disabled:opacity-50 disabled:pointer-events-none"
                style={
                  isCurrent
                    ? { border: '1px solid var(--border)', color: 'var(--ink-soft)' }
                    : { background: 'var(--accent-solid)', color: 'var(--fg-on-solid)' }
                }
              >
                {isCurrent
                  ? 'Plan actuel'
                  : plan === 'enterprise'
                    ? 'Nous contacter'
                    : plan === 'free'
                      ? 'Rétrograder'
                      : !sub
                        ? `Essai ${ent.trial_days} j gratuit`
                        : `Choisir ${PLAN_LABEL[plan]}`}
              </button>
            </div>
          );
        })}
      </div>

      {/* Invoices */}
      {billing && billing.invoices.length > 0 && (
        <div
          className="rounded-[16px] p-5"
          style={{ background: 'var(--bg-elev)', border: '1px solid var(--border)' }}
        >
          <div className="text-[13px] font-semibold text-ink mb-3">Factures</div>
          <div className="space-y-2">
            {billing.invoices.map((inv) => (
              <div key={inv.id} className="flex items-center justify-between text-[12.5px]">
                <span className="text-ink-soft">
                  {inv.number || inv.id.slice(0, 8)} ·{' '}
                  {fmt.date(inv.created_at)}
                </span>
                <span className="flex items-center gap-3">
                  <span className="font-semibold text-ink">
                    {fmt.currency(inv.amount_cents / 100, { currency: inv.currency })}
                  </span>
                  {inv.hosted_url && (
                    <a
                      href={inv.hosted_url}
                      target="_blank"
                      rel="noreferrer"
                      className="text-accent font-semibold"
                    >
                      Voir
                    </a>
                  )}
                </span>
              </div>
            ))}
          </div>
        </div>
      )}

      <TelemetryCard />
    </div>
  );
}

function TelemetryCard() {
  const { data } = useTelemetry();
  const setTelemetry = useSetTelemetry();
  if (!data) return null;
  return (
    <div
      className="rounded-[16px] p-5"
      style={{ background: 'var(--bg-elev)', border: '1px solid var(--border)' }}
    >
      <div className="flex items-center justify-between gap-4 flex-wrap">
        <div className="max-w-[62ch]">
          <div className="text-[13px] font-semibold text-ink mb-1">Télémétrie anonyme (opt-in)</div>
          <div className="text-[12.5px] text-ink-soft leading-relaxed">
            Aide-nous à améliorer OpenRisk en partageant des statistiques d'usage{' '}
            <strong>anonymes</strong> (jamais vos risques, actifs ou identités).
            {data.env_forced_off &&
              ' Désactivée sur cette instance par la variable d’environnement OPENRISK_TELEMETRY.'}{' '}
            {data.schema_url && (
              <a
                href={data.schema_url}
                target="_blank"
                rel="noreferrer"
                className="text-accent font-semibold"
              >
                Schéma publié
              </a>
            )}
          </div>
        </div>
        <button
          disabled={data.env_forced_off || setTelemetry.isPending}
          onClick={() => setTelemetry.mutate(!data.consent)}
          className="h-9 px-4 rounded-[10px] text-[13px] font-semibold disabled:opacity-50 disabled:pointer-events-none"
          style={
            data.enabled
              ? { background: 'var(--accent)', color: 'var(--fg-primary)' }
              : { border: '1px solid var(--border)', color: 'var(--ink-soft)' }
          }
        >
          {data.enabled ? 'Activée' : 'Désactivée'}
        </button>
      </div>
    </div>
  );
}
