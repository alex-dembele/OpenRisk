// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// Settings → Sessions: which devices are signed in, and signing them out.
//
// A refresh token IS a session, so this list is the set of credentials that can
// still act as you. That framing is why revocation here has to be immediate and
// unambiguous rather than a "log out everywhere eventually".

import { useCallback, useEffect, useState } from 'react';
import { Loader2, LogOut, Monitor, ShieldAlert } from 'lucide-react';
import { toast } from 'sonner';

import { useUIStore } from '../../store/uiStore';
import { DangerConfirm } from '../../shared/DangerConfirm';
import { localeTag, type LocaleCode } from '../../i18n/locales';
import {
  listSessions,
  revokeOtherSessions,
  revokeSession,
  type SessionRecord,
} from './authService';

const copyFor = (lang: LocaleCode) =>
  lang === 'en'
    ? {
        title: 'Active sessions',
        subtitle: 'Devices currently signed in to your account.',
        thisDevice: 'This device',
        lastUsed: 'Last used',
        signedIn: 'Signed in',
        never: 'not yet',
        revoke: 'Sign out',
        revokeOthers: 'Sign out all other devices',
        empty: 'No other device is signed in.',
        loadError:
          'Could not load your sessions. Refresh the page, or contact an administrator if it persists.',
        revoked: 'Device signed out.',
        revokedMany: (n: number) => `${n} device${n === 1 ? '' : 's'} signed out.`,
        revokeFailed: 'Could not sign that device out. Try again in a moment.',
        confirmTitle: 'Sign this device out?',
        confirmBody:
          'That device will have to sign in again. Anything it was doing is interrupted immediately.',
        confirmAllTitle: 'Sign out every other device?',
        confirmAllBody:
          'Every device except this one is signed out immediately. Use this if you think someone else has access to your account.',
        alternative: 'Change your password instead — that also ends every session.',
        device: 'Device',
        ipAddress: 'IP address',
        unknown: 'unknown',
      }
    : {
        title: 'Sessions actives',
        subtitle: 'Les appareils actuellement connectés à votre compte.',
        thisDevice: 'Cet appareil',
        lastUsed: 'Dernière activité',
        signedIn: 'Connexion',
        never: 'jamais',
        revoke: 'Déconnecter',
        revokeOthers: 'Déconnecter tous les autres appareils',
        empty: 'Aucun autre appareil connecté.',
        loadError:
          'Impossible de charger vos sessions. Rechargez la page, ou contactez un administrateur si cela persiste.',
        revoked: 'Appareil déconnecté.',
        revokedMany: (n: number) =>
          `${n} appareil${n === 1 ? '' : 's'} déconnecté${n === 1 ? '' : 's'}.`,
        revokeFailed: 'Impossible de déconnecter cet appareil. Réessayez dans un instant.',
        confirmTitle: 'Déconnecter cet appareil ?',
        confirmBody:
          'Cet appareil devra se reconnecter. Tout travail en cours y est interrompu immédiatement.',
        confirmAllTitle: 'Déconnecter tous les autres appareils ?',
        confirmAllBody:
          'Tous les appareils sauf celui-ci sont déconnectés immédiatement. Utilisez ceci si vous pensez que quelqu’un d’autre accède à votre compte.',
        alternative: 'Changer votre mot de passe — cela met aussi fin à toutes les sessions.',
        device: 'Appareil',
        ipAddress: 'Adresse IP',
        unknown: 'inconnue',
      };

function formatWhen(iso: string | undefined, lang: LocaleCode, fallback: string): string {
  if (!iso) return fallback;
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return fallback;
  return d.toLocaleString(localeTag(lang), {
    dateStyle: 'medium',
    timeStyle: 'short',
  });
}

export function SessionsPanel() {
  const lang = useUIStore((s) => s.lang);
  const t = copyFor(lang);

  const [sessions, setSessions] = useState<SessionRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [busyId, setBusyId] = useState<string | null>(null);
  const [confirming, setConfirming] = useState<SessionRecord | null>(null);
  const [confirmingAll, setConfirmingAll] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      setSessions(await listSessions());
    } catch {
      setError(t.loadError);
    } finally {
      setLoading(false);
    }
  }, [t.loadError]);

  useEffect(() => {
    void load();
  }, [load]);

  const doRevoke = async (session: SessionRecord) => {
    setBusyId(session.id);
    try {
      await revokeSession(session.id);
      setSessions((prev) => prev.filter((s) => s.id !== session.id));
      toast.success(t.revoked);
    } catch {
      toast.error(t.revokeFailed);
    } finally {
      setBusyId(null);
      setConfirming(null);
    }
  };

  const doRevokeOthers = async () => {
    setBusyId('others');
    try {
      const n = await revokeOtherSessions();
      setSessions((prev) => prev.filter((s) => s.current));
      toast.success(t.revokedMany(n));
    } catch {
      toast.error(t.revokeFailed);
    } finally {
      setBusyId(null);
      setConfirmingAll(false);
    }
  };

  const others = sessions.filter((s) => !s.current);

  return (
    <div data-testid="sessions-panel">
      <div className="mb-4">
        <h2 className="text-[15px] font-semibold text-ink">{t.title}</h2>
        <p className="text-[12.5px] text-ink-soft mt-0.5">{t.subtitle}</p>
      </div>

      {loading && (
        <div className="space-y-2" aria-busy="true">
          {[0, 1].map((i) => (
            <div
              key={i}
              className="h-[62px] rounded-[11px]"
              style={{ background: 'var(--bg-hover)' }}
            />
          ))}
        </div>
      )}

      {!loading && error && (
        <div
          role="alert"
          className="px-3 py-2.5 rounded-[11px] text-[12.5px]"
          style={{
            background: 'color-mix(in srgb, var(--critical) 10%, transparent)',
            border: '1px solid color-mix(in srgb, var(--critical) 35%, transparent)',
            color: 'var(--critical)',
          }}
        >
          {error}
        </div>
      )}

      {!loading && !error && (
        <>
          <ul className="space-y-2 list-none p-0 m-0">
            {sessions.map((s) => (
              <li
                key={s.id}
                data-testid="session-row"
                className="flex items-center gap-3 px-3.5 py-3 rounded-[11px]"
                style={{
                  border: '1px solid var(--border-strong)',
                  background: 'var(--bg-elevated)',
                }}
              >
                <Monitor size={18} className="text-ink-muted shrink-0" />
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="text-[13.5px] font-medium text-ink truncate">{s.device}</span>
                    {s.current && (
                      <span
                        className="text-[10.5px] font-semibold px-1.5 py-0.5 rounded"
                        style={{ background: 'var(--accent-soft)', color: 'var(--accent-500)' }}
                      >
                        {t.thisDevice}
                      </span>
                    )}
                  </div>
                  <div className="text-[11.5px] text-ink-muted mt-0.5 truncate">
                    {s.ip_address || t.unknown} · {t.lastUsed}:{' '}
                    {formatWhen(s.last_used_at ?? s.created_at, lang, t.never)}
                  </div>
                </div>

                {/* The current session has no revoke button: signing yourself out
                    from a device-management screen is a confusing way to log out,
                    and the ordinary sign-out already does it. */}
                {!s.current && (
                  <button
                    type="button"
                    data-testid="revoke-session"
                    onClick={() => setConfirming(s)}
                    disabled={busyId === s.id}
                    className="shrink-0 h-8 px-3 rounded-[9px] text-[12.5px] font-medium flex items-center gap-1.5 transition-colors"
                    style={{ border: '1px solid var(--border-strong)', color: 'var(--critical)' }}
                  >
                    {busyId === s.id ? (
                      <Loader2 size={14} className="animate-spin" />
                    ) : (
                      <LogOut size={14} />
                    )}
                    {t.revoke}
                  </button>
                )}
              </li>
            ))}
          </ul>

          {others.length === 0 && (
            <div className="text-[12.5px] text-ink-muted mt-3">{t.empty}</div>
          )}

          {others.length > 0 && (
            <button
              type="button"
              data-testid="revoke-other-sessions"
              onClick={() => setConfirmingAll(true)}
              disabled={busyId === 'others'}
              className="mt-4 h-9 px-3.5 rounded-[10px] text-[12.5px] font-semibold flex items-center gap-2 transition-colors"
              style={{ border: '1px solid var(--border-strong)', color: 'var(--critical)' }}
            >
              {busyId === 'others' ? (
                <Loader2 size={15} className="animate-spin" />
              ) : (
                <ShieldAlert size={15} />
              )}
              {t.revokeOthers}
            </button>
          )}
        </>
      )}

      {confirming && (
        <DangerConfirm
          open
          title={t.confirmTitle}
          subject={confirming.device}
          intro={t.confirmBody}
          impact={[
            { label: t.device, value: confirming.device },
            { label: t.ipAddress, value: confirming.ip_address || t.unknown },
            {
              label: t.lastUsed,
              value: formatWhen(confirming.last_used_at ?? confirming.created_at, lang, t.never),
            },
          ]}
          confirmLabel={t.revoke}
          busy={busyId === confirming.id}
          onConfirm={() => void doRevoke(confirming)}
          onClose={() => setConfirming(null)}
        />
      )}

      {confirmingAll && (
        <DangerConfirm
          open
          title={t.confirmAllTitle}
          subject={t.revokeOthers}
          intro={t.confirmAllBody}
          impact={[{ label: t.device, value: String(others.length) }]}
          alternatives={[{ label: t.alternative, onClick: () => setConfirmingAll(false) }]}
          confirmLabel={t.revokeOthers}
          busy={busyId === 'others'}
          onConfirm={() => void doRevokeOthers()}
          onClose={() => setConfirmingAll(false)}
        />
      )}
    </div>
  );
}

export default SessionsPanel;
