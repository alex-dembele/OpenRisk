// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// OR26-03 — Settings › Security › MFA policy.
//
// "Organization-wide MFA policy is configured by an administrator — managing it
// from this screen is coming soon" is what this card used to say. This is the
// screen.
//
// The form validates against the bounds the SERVER ships (min/max/default come
// down with the policy), so the two cannot drift; and a non-admin sees the
// current value read-only rather than a control that would be refused.

import { useEffect, useState } from 'react';
import { toast } from 'sonner';
import { Loader2, ShieldCheck } from 'lucide-react';

import { Card, SkeletonRows, ErrorState } from '../../shared/ui';
import { useUIStore } from '../../store/uiStore';
import { useAuthStore } from '../../hooks/useAuthStore';
import { useMFAPolicy, useSaveMFAPolicy, useMFAStatus } from '../auth/useMfa';
import { MFAEnrollmentDialog } from '../auth/MFAEnrollmentDialog';
import { useI18n } from '../../hooks/useI18n';

export function MFAPolicyPanel() {
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const { t } = useI18n();
  const dayCount = (n: number) => t('common.days', { count: n });
  const hasPermission = useAuthStore((s) => s.hasPermission);
  const canEdit = hasPermission('*');

  const { data: policy, isLoading, isError, refetch } = useMFAPolicy();
  const save = useSaveMFAPolicy();

  const [days, setDays] = useState<string>('');
  const [touched, setTouched] = useState(false);

  // Seed the field from the server once it arrives, and re-seed after a save,
  // but never while the admin is mid-edit — overwriting somebody's typing with
  // a background refetch is how a form loses a change nobody knew was lost.
  useEffect(() => {
    if (policy && !touched) setDays(String(policy.grace_days));
  }, [policy, touched]);

  if (isLoading)
    return (
      <Card style={{ padding: '20px 22px' }}>
        <SkeletonRows rows={2} height={36} />
      </Card>
    );
  if (isError || !policy) {
    return (
      <Card style={{ padding: '20px 22px' }}>
        <ErrorState
          title={tr('Politique MFA indisponible', 'MFA policy unavailable')}
          description={tr(
            'Réessayez, ou contactez un administrateur si le problème persiste.',
            'Try again, or contact an administrator if this persists.',
          )}
          onRetry={() => void refetch()}
          retryLabel={tr('Réessayer', 'Retry')}
        />
      </Card>
    );
  }

  const parsed = Number(days);
  const valid =
    days.trim() !== '' &&
    Number.isInteger(parsed) &&
    parsed >= policy.min_days &&
    parsed <= policy.max_days;
  const dirty = touched && String(policy.grace_days) !== days.trim();
  const roles = [
    ...(policy.privileged_org_roles ?? []),
    ...(policy.privileged_business_roles ?? []),
  ];

  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!valid || !dirty || save.isPending) return;
    save.mutate(parsed, {
      onSuccess: () => {
        setTouched(false);
        toast.success(tr('Politique MFA enregistrée', 'MFA policy saved'));
      },
      onError: () =>
        toast.error(
          tr(
            'Enregistrement impossible. Vérifiez que vous êtes administrateur de cette organisation.',
            'Could not save. Check that you are an administrator of this organization.',
          ),
        ),
    });
  };

  return (
    <Card style={{ padding: '20px 22px', marginBottom: 16 }}>
      <div className="text-[15px] font-semibold text-ink mb-1.5">
        {tr('Politique MFA', 'MFA policy')}
      </div>
      <p className="text-[13px] text-ink-soft leading-relaxed mb-4">
        {tr(
          "Les comptes privilégiés disposent d'un délai pour activer l'authentification à deux facteurs. Passé ce délai, l'accès est bloqué côté serveur jusqu'à l'activation — sur l'interface comme sur l'API.",
          'Privileged accounts get a window to enable two-factor authentication. After it expires, access is blocked server-side until they enroll — in the UI and on the API alike.',
        )}
      </p>

      <form onSubmit={submit} className="flex flex-wrap items-end gap-3">
        <label className="flex flex-col gap-1.5">
          <span className="text-[11px] font-semibold uppercase tracking-[.04em] text-ink-muted">
            {tr('Forcer le MFA après', 'Force MFA after')}
          </span>
          <div className="flex items-center gap-2">
            <input
              type="number"
              min={policy.min_days}
              max={policy.max_days}
              step={1}
              value={days}
              disabled={!canEdit}
              aria-invalid={touched && !valid}
              aria-describedby="mfa-policy-help"
              onChange={(e) => {
                setDays(e.target.value);
                setTouched(true);
              }}
              className="w-[92px] h-10 px-3 rounded-[10px] text-[13px] text-ink outline-none focus:border-accent transition-colors disabled:opacity-60"
              style={{ border: '1px solid var(--border-strong)', background: 'var(--bg-elevated)' }}
            />
            <span className="text-[13px] text-ink-soft">{tr('jours', 'days')}</span>
          </div>
        </label>

        {canEdit && (
          <button
            type="submit"
            disabled={!valid || !dirty || save.isPending}
            className="h-10 px-4 rounded-[10px] text-[13px] font-semibold text-fg-primary inline-flex items-center gap-1.5 transition-all disabled:opacity-50"
            style={{
              border: 'none',
              background: 'var(--accent-solid)',
              color: 'var(--fg-on-solid)',
            }}
          >
            {save.isPending && <Loader2 size={15} className="animate-spin" aria-hidden="true" />}
            {tr('Enregistrer', 'Save')}
          </button>
        )}
      </form>

      <div id="mfa-policy-help" className="text-[12.5px] text-ink-soft mt-3 leading-relaxed">
        {touched && !valid ? (
          <span style={{ color: 'var(--critical)' }} role="alert">
            {tr(
              `Saisissez un nombre entier entre ${policy.min_days} et ${policy.max_days}.`,
              `Enter a whole number between ${policy.min_days} and ${policy.max_days}.`,
            )}
          </span>
        ) : (
          <>
            <div>
              {parsed === 0
                ? tr(
                    '0 jour : le MFA est exigé dès la première connexion des comptes concernés.',
                    '0 days: MFA is required from the first sign-in for affected accounts.',
                  )
                : tr(
                    `Après ${dayCount(policy.grace_days)}, les comptes concernés doivent activer le MFA avant de continuer.`,
                    `After ${dayCount(policy.grace_days)}, affected accounts must enable MFA before continuing.`,
                  )}
            </div>
            {roles.length > 0 && (
              <div className="mt-1">
                {tr('Comptes concernés : ', 'Affected accounts: ')}
                <span className="text-ink font-medium">{roles.join(', ')}</span>
                {tr(
                  '. Les autres membres reçoivent une recommandation, jamais un blocage.',
                  '. Other members get a recommendation, never a block.',
                )}
              </div>
            )}
            {!policy.configured && (
              <div className="mt-1 text-ink-muted">
                {tr(
                  `Valeur par défaut (${policy.default_days} jours) — cette organisation n'a pas encore choisi.`,
                  `Default value (${policy.default_days} days) — this organization has not chosen yet.`,
                )}
              </div>
            )}
            {!canEdit && (
              <div className="mt-1 text-ink-muted">
                {tr(
                  "Seul un administrateur de l'organisation peut modifier ce délai.",
                  'Only an organization administrator can change this window.',
                )}
              </div>
            )}
          </>
        )}
      </div>
    </Card>
  );
}

/** Your own MFA state, and the button to turn it on. */
export function MFAAccountPanel() {
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const { data: status, isLoading, isError, refetch } = useMFAStatus();
  const [enrolling, setEnrolling] = useState(false);

  const body = () => {
    if (isLoading) return <SkeletonRows rows={1} height={30} />;
    // Never claim protection we could not verify.
    if (isError || status === null) {
      return (
        <div className="flex items-center justify-between gap-3">
          <span className="text-[13px] text-ink-soft">
            {tr('État du MFA indisponible pour le moment.', 'MFA status unavailable right now.')}
          </span>
          <button
            type="button"
            onClick={() => void refetch()}
            className="text-[13px] font-semibold"
            style={{ color: 'var(--accent-500)' }}
          >
            {tr('Réessayer', 'Retry')}
          </button>
        </div>
      );
    }
    if (status?.state === 'configured') {
      return (
        <div className="flex items-center gap-2 text-[13px]" style={{ color: 'var(--low)' }}>
          <ShieldCheck size={16} aria-hidden="true" />
          {tr('Le MFA est activé sur votre compte.', 'MFA is enabled on your account.')}
        </div>
      );
    }
    return (
      <div className="flex items-center justify-between gap-3 flex-wrap">
        <span className="text-[13px] text-ink-soft">
          {tr(
            "Le MFA n'est pas encore activé sur votre compte.",
            'MFA is not enabled on your account yet.',
          )}
        </span>
        <button
          type="button"
          onClick={() => setEnrolling(true)}
          className="h-9 px-4 rounded-[10px] text-[13px] font-semibold text-fg-primary transition-all"
          style={{ border: 'none', background: 'var(--accent-solid)', color: 'var(--fg-on-solid)' }}
        >
          {tr('Activer le MFA', 'Enable MFA')}
        </button>
      </div>
    );
  };

  return (
    <Card style={{ padding: '20px 22px', marginBottom: 16 }}>
      <div className="text-[15px] font-semibold text-ink mb-3">
        {tr('Votre authentification', 'Your authentication')}
      </div>
      {body()}
      {enrolling && <MFAEnrollmentDialog onClose={() => setEnrolling(false)} />}
    </Card>
  );
}
