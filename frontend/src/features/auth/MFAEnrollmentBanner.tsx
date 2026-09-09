// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// OR26-03 — the non-blocking MFA prompt.
//
// This is what replaces the wall. Before, a privileged account met a QR code
// before it had seen a single screen; now it reaches the product and reads this
// instead. The banner is an INVITATION, not a gate: the enforcement lives on the
// server (MFAPolicyGuard), and nothing this component does — including being
// dismissed — changes who is allowed in.
//
// Dismissal is therefore a purely cosmetic preference, and is deliberately
// SESSION-scoped rather than persisted: a recommendation you can silence
// forever is one nobody acts on, and a privileged account's countdown is not
// dismissible at all.

import { useState } from 'react';
import { ShieldAlert, ShieldCheck, X, Loader2 } from 'lucide-react';

import { useMFAStatus } from './useMfa';
import { daysUntilDeadline, type MFAStatus } from './mfaPolicyService';
import { MFAEnrollmentDialog } from './MFAEnrollmentDialog';
import { useUIStore } from '../../store/uiStore';
import { catalogs, translate, DEFAULT_LOCALE, type LocaleCode } from '../../i18n';

/**
 * Session-scoped dismissal for the soft prompt.
 *
 * sessionStorage, not localStorage, and never consulted for anything the server
 * enforces. Writing something like `mfaSkipped = true` to durable storage would
 * be an invitation to mistake a UI preference for a security decision.
 */
const DISMISS_KEY = 'openrisk_mfa_banner_dismissed';

type Tone = 'info' | 'warn' | 'critical';

interface Copy {
  title: string;
  body: string;
  cta: string;
  tone: Tone;
  dismissible: boolean;
}

/**
 * Maps the server's state to what the user reads.
 *
 * Every branch comes from `status.state` — the closed vocabulary the backend
 * ships — rather than from a role check or a date comparison done here. Two
 * implementations of "are they required?" is how the banner and the guard end
 * up disagreeing, and the one the user believes is the wrong one.
 */
export function copyFor(
  status: MFAStatus,
  tr: (fr: string, en: string) => string,
  /** Locale for the day count's plural form. Optional so existing callers hold. */
  locale: LocaleCode = DEFAULT_LOCALE,
): Copy | null {
  const days = daysUntilDeadline(status);
  // "1 jour" / "2 jours" / "0 days" — the plural rule belongs to the language,
  // not to a `days > 1` written into a French sentence.
  const dayCount = (n: number) => translate(catalogs, locale, 'common.days', { params: { count: n } });

  switch (status.state) {
    case 'configured':
      return null;

    case 'recommended':
      return {
        tone: 'info',
        dismissible: true,
        title: tr('Sécurisez votre compte', 'Secure your account'),
        body: tr(
          "L'authentification à deux facteurs protège votre compte même si votre mot de passe fuite. Deux minutes suffisent.",
          'Two-factor authentication protects your account even if your password leaks. It takes two minutes.',
        ),
        cta: tr('Activer le MFA', 'Enable MFA'),
      };

    case 'grace_active':
      return {
        tone: 'info',
        dismissible: false,
        title: tr('Le MFA sera requis pour votre compte', 'MFA will be required for your account'),
        body:
          days === null
            ? tr(
                'Votre rôle donne accès à des données et des réglages sensibles : une authentification à deux facteurs est requise.',
                'Your role grants access to sensitive data and settings, so two-factor authentication is required.',
              )
            : tr(
                `Votre rôle donne accès à des données sensibles. Il vous reste ${dayCount(days)} pour activer le MFA.`,
                `Your role grants access to sensitive data. You have ${dayCount(days)} left to enable MFA.`,
              ),
        cta: tr('Activer le MFA', 'Enable MFA'),
      };

    case 'grace_expiring':
      return {
        tone: 'warn',
        dismissible: false,
        title: tr('Activez le MFA maintenant', 'Enable MFA now'),
        body:
          days === null || days <= 0
            ? tr(
                "L'accès à OpenRisk sera bloqué très prochainement tant que le MFA ne sera pas activé.",
                'Access to OpenRisk will be blocked shortly until MFA is enabled.',
              )
            : tr(
                `Il vous reste ${dayCount(days)}. Passé ce délai, l'accès sera bloqué jusqu'à l'activation du MFA.`,
                `You have ${dayCount(days)} left. After that, access is blocked until MFA is enabled.`,
              ),
        cta: tr('Activer le MFA', 'Enable MFA'),
      };

    case 'required':
      return {
        tone: 'critical',
        dismissible: false,
        title: tr('MFA requis', 'MFA required'),
        body: tr(
          "Le délai est écoulé. Activez une application d'authentification pour retrouver l'accès.",
          'The grace period has ended. Enroll an authenticator to restore access.',
        ),
        cta: tr('Activer le MFA', 'Enable MFA'),
      };

    default:
      return null;
  }
}

const TONE_STYLE: Record<Tone, { bg: string; border: string; accent: string }> = {
  info: { bg: 'var(--accent-soft)', border: 'var(--accent)', accent: 'var(--accent)' },
  warn: { bg: 'var(--bg-elevated)', border: 'var(--high)', accent: 'var(--high)' },
  critical: { bg: 'var(--bg-elevated)', border: 'var(--critical)', accent: 'var(--critical)' },
};

export function MFAEnrollmentBanner() {
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const { data: status, isLoading, isError, refetch } = useMFAStatus();
  const [dialogOpen, setDialogOpen] = useState(false);
  const [dismissed, setDismissed] = useState(() => sessionStorage.getItem(DISMISS_KEY) === '1');

  // Loading: say nothing rather than flashing a security warning that may not
  // apply. A banner that appears and vanishes on every page load is noise, and
  // noise is what people learn to skip.
  if (isLoading) {
    return (
      <div
        className="flex items-center gap-2 text-[12.5px] text-ink-muted mb-4"
        role="status"
        aria-live="polite"
      >
        <Loader2 size={14} className="animate-spin" aria-hidden="true" />
        {tr('Vérification de la sécurité du compte…', 'Checking account security…')}
      </div>
    );
  }

  // Error / unavailable: an honest, quiet line with a retry. Never a green
  // "you're protected" — claiming a security control is active when we do not
  // know is the one failure mode a security product cannot afford.
  if (isError || status === null) {
    return (
      <div
        className="flex items-center justify-between gap-3 px-4 py-2.5 rounded-[12px] mb-4"
        style={{ background: 'var(--bg-elevated)', border: '1px solid var(--border)' }}
        role="status"
      >
        <span className="text-[12.5px] text-ink-soft">
          {tr(
            "Impossible de vérifier l'état du MFA de votre compte pour le moment.",
            'Could not check your account’s MFA status right now.',
          )}
        </span>
        <button
          type="button"
          onClick={() => void refetch()}
          className="text-[12.5px] font-semibold shrink-0"
          style={{ color: 'var(--accent-500)' }}
        >
          {tr('Réessayer', 'Retry')}
        </button>
      </div>
    );
  }

  if (!status) return null;

  const copy = copyFor(status, tr, lang);
  if (!copy) return null;
  if (copy.dismissible && dismissed) return null;

  const tone = TONE_STYLE[copy.tone];
  const Icon = copy.tone === 'info' ? ShieldCheck : ShieldAlert;

  return (
    <>
      <section
        // A deadline that has passed is an alert; a recommendation is a status.
        // Screen readers should interrupt for the first and not for the second.
        role={copy.tone === 'critical' ? 'alert' : 'status'}
        aria-live={copy.tone === 'critical' ? 'assertive' : 'polite'}
        aria-labelledby="mfa-banner-title"
        data-testid="mfa-enrollment-banner"
        data-mfa-state={status.state}
        className="flex items-start gap-3 px-4 py-3.5 rounded-[13px] mb-4"
        style={{ background: tone.bg, border: `1px solid ${tone.border}` }}
      >
        <span className="mt-px shrink-0" style={{ color: tone.accent }} aria-hidden="true">
          <Icon size={18} />
        </span>

        <div className="flex-1 min-w-0">
          <div id="mfa-banner-title" className="text-[13.5px] font-semibold text-ink mb-0.5">
            {copy.title}
          </div>
          <div className="text-[12.5px] text-ink-soft leading-relaxed">{copy.body}</div>
        </div>

        <div className="flex items-center gap-1.5 shrink-0">
          <button
            type="button"
            onClick={() => setDialogOpen(true)}
            className="h-8 px-3.5 rounded-[9px] text-[12.5px] font-semibold text-fg-primary transition-all"
            style={{
              border: 'none',
              background: 'var(--accent-solid)',
              color: 'var(--fg-on-solid)',
            }}
          >
            {copy.cta}
          </button>
          {copy.dismissible && (
            <button
              type="button"
              onClick={() => {
                sessionStorage.setItem(DISMISS_KEY, '1');
                setDismissed(true);
              }}
              className="w-8 h-8 rounded-[9px] flex items-center justify-center text-ink-muted hover:text-ink transition-colors"
              aria-label={tr('Masquer ce rappel', 'Dismiss this reminder')}
            >
              <X size={16} aria-hidden="true" />
            </button>
          )}
        </div>
      </section>

      {dialogOpen && <MFAEnrollmentDialog onClose={() => setDialogOpen(false)} />}
    </>
  );
}
