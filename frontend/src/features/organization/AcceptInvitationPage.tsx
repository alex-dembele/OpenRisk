// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// Invitation acceptance — where the link in the email lands (W0-04).
//
// This screen is public, and it has to be: the person following it may have no
// OpenRisk account at all. It therefore does the smallest useful thing before
// asking for anything — it previews the invitation, so the visitor can see
// which organization is asking and at what role before deciding.
//
// The four ways a link can be dead (unknown, expired, revoked, already used)
// each get their own words. "Invalid token" tells someone holding a legitimate
// link nothing they can act on.

import { localeTag } from '../../i18n/locales';
import { useEffect, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router';
import { toast } from 'sonner';
import { Building2, CheckCircle2, AlertTriangle, Loader2, ArrowRight } from 'lucide-react';
import { useUIStore } from '../../store/uiStore';
import { useAuthStore } from '../../hooks/useAuthStore';
import { OpenRiskLogo } from '../../shared/Logo';
import { organizationService, type InvitationPreview } from './organizationService';

type Tr = (fr: string, en: string) => string;

/** The server's message when it sent one. It distinguishes expired from
 *  revoked from already-used; a generic string would erase that. */
function apiMessage(err: unknown, fallback: string): string {
  const r = (err as { response?: { status?: number; data?: { error?: string; message?: string } } })
    ?.response;
  return r?.data?.error || r?.data?.message || fallback;
}

function statusOf(err: unknown): number | undefined {
  return (err as { response?: { status?: number } })?.response?.status;
}

export function AcceptInvitationPage() {
  const lang = useUIStore((s) => s.lang);
  const tr: Tr = (fr, en) => (lang === 'fr' ? fr : en);
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const token = params.get('token') ?? '';
  const login = useAuthStore((s) => s.login);
  const signedInEmail = useAuthStore((s) => s.user?.email);

  const [preview, setPreview] = useState<InvitationPreview | null>(null);
  const [loadError, setLoadError] = useState<{ message: string; recoverable: boolean } | null>(
    null,
  );
  const [loading, setLoading] = useState(true);
  const [fullName, setFullName] = useState('');
  const [password, setPassword] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [done, setDone] = useState<{ organization: string; createdAccount: boolean } | null>(null);

  useEffect(() => {
    let cancelled = false;
    if (!token) {
      setLoading(false);
      setLoadError({
        message: tr(
          "Ce lien est incomplet. Demandez une nouvelle invitation à un administrateur de l'organisation.",
          'This link is incomplete. Ask an administrator of the organization for a new invitation.',
        ),
        recoverable: false,
      });
      return;
    }
    organizationService
      .previewInvitation(token)
      .then((p) => {
        if (!cancelled) {
          setPreview(p);
          setLoading(false);
        }
      })
      .catch((err) => {
        if (cancelled) return;
        setLoading(false);
        const code = statusOf(err);
        setLoadError({
          // 410 means the invitation existed and is finished — expired,
          // revoked or already used — and the server says which. 404 means the
          // link resolves to nothing, and deliberately says no more than that.
          message:
            code === 410
              ? apiMessage(
                  err,
                  tr("Cette invitation n'est plus valable.", 'This invitation is no longer valid.'),
                )
              : tr(
                  "Ce lien d'invitation n'est pas valable. Il a peut-être déjà été utilisé, ou remplacé par un envoi plus récent.",
                  'This invitation link is not valid. It may have already been used, or replaced by a more recent one.',
                ),
          recoverable: code === 410,
        });
      });
    return () => {
      cancelled = true;
    };
    // tr is derived from lang; re-previewing on a language switch is harmless
    // and keeps the error copy in the right language.
  }, [token, lang]); // eslint-disable-line react-hooks/exhaustive-deps

  const accept = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!preview) return;
    setSubmitError(null);
    setSubmitting(true);
    try {
      const res = await organizationService.acceptInvitation(
        preview.requires_signup ? { token, full_name: fullName, password } : { token },
      );
      // A brand-new account: sign them straight in, so "accept" ends inside the
      // product rather than at a login form they must fill in again.
      if (res.created_account && password) {
        try {
          await login(preview.email, password);
        } catch {
          toast.info(tr('Compte créé — connectez-vous.', 'Account created — please sign in.'));
        }
      }
      setDone({
        organization: res.organization_name || preview.organization_name,
        createdAccount: res.created_account,
      });
    } catch (err) {
      const code = statusOf(err);
      if (code === 401) {
        // The address already has an account and nobody is signed in. Attaching
        // a seat to it without authentication would be an account takeover, so
        // the honest next step is: sign in, then come back to this link.
        setSubmitError(
          tr(
            `Un compte existe déjà pour ${preview.email}. Connectez-vous, puis rouvrez ce lien.`,
            `An account already exists for ${preview.email}. Sign in, then open this link again.`,
          ),
        );
      } else {
        setSubmitError(
          apiMessage(
            err,
            tr("L'acceptation a échoué. Réessayez.", 'Accepting failed. Please try again.'),
          ),
        );
      }
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div
      className="min-h-screen flex items-center justify-center p-5"
      style={{ background: 'var(--bg-app)' }}
    >
      <div className="w-full max-w-[440px]">
        <div className="flex items-center gap-2.5 mb-7 justify-center">
          <OpenRiskLogo size={30} />
          <span className="text-[19px] font-bold text-ink">OpenRisk</span>
        </div>

        <div
          className="rounded-[18px] p-6"
          style={{
            background: 'var(--bg-elevated)',
            border: '1px solid var(--border)',
            boxShadow: 'var(--shadow-card)',
          }}
        >
          {loading && (
            <div className="flex flex-col items-center gap-3 py-8 text-ink-soft">
              <Loader2 size={26} className="animate-spin" />
              <span className="text-[13.5px]">
                {tr("Vérification de l'invitation…", 'Checking the invitation…')}
              </span>
            </div>
          )}

          {!loading && loadError && (
            <div className="text-center py-4">
              <span
                aria-hidden
                className="w-12 h-12 rounded-[14px] mx-auto mb-4 flex items-center justify-center"
                style={{
                  background: 'color-mix(in srgb, var(--high) 14%, transparent)',
                  color: 'var(--high)',
                }}
              >
                <AlertTriangle size={24} />
              </span>
              <h1 className="text-[17px] font-bold text-ink mb-2">
                {tr('Invitation indisponible', 'Invitation unavailable')}
              </h1>
              <p role="alert" className="text-[13.5px] text-ink-soft leading-relaxed mb-5">
                {loadError.message}
              </p>
              <button
                onClick={() => navigate('/login')}
                className="w-full h-[42px] rounded-[10px] text-[14px] font-semibold"
                style={{ border: '1px solid var(--border-strong)', color: 'var(--fg-primary)' }}
              >
                {tr('Aller à la connexion', 'Go to sign in')}
              </button>
            </div>
          )}

          {!loading && done && (
            <div className="text-center py-4">
              <span
                aria-hidden
                className="w-12 h-12 rounded-[14px] mx-auto mb-4 flex items-center justify-center"
                style={{
                  background: 'color-mix(in srgb, var(--low) 14%, transparent)',
                  color: 'var(--low)',
                }}
              >
                <CheckCircle2 size={24} />
              </span>
              <h1 className="text-[17px] font-bold text-ink mb-2">
                {tr('Bienvenue !', 'Welcome!')}
              </h1>
              <p className="text-[13.5px] text-ink-soft leading-relaxed mb-5">
                {tr(
                  `Vous faites maintenant partie de ${done.organization}.`,
                  `You are now a member of ${done.organization}.`,
                )}
              </p>
              <button
                onClick={() => navigate('/')}
                className="w-full h-[42px] rounded-[10px] text-[14px] font-semibold inline-flex items-center justify-center gap-2"
                style={{ background: 'var(--accent-solid)', color: 'var(--fg-on-solid)' }}
              >
                {tr('Ouvrir OpenRisk', 'Open OpenRisk')} <ArrowRight size={16} />
              </button>
            </div>
          )}

          {!loading && preview && !done && (
            <>
              <div className="flex items-center gap-3 mb-5">
                <span
                  aria-hidden
                  className="w-11 h-11 rounded-[13px] flex items-center justify-center shrink-0"
                  style={{ background: 'var(--accent-soft)', color: 'var(--accent-500)' }}
                >
                  <Building2 size={21} />
                </span>
                <div className="min-w-0">
                  <h1 className="text-[16px] font-bold text-ink truncate">
                    {preview.organization_name}
                  </h1>
                  <div className="text-[12.5px] text-ink-soft">
                    {tr(
                      'vous invite à rejoindre son espace',
                      'invites you to join their workspace',
                    )}
                  </div>
                </div>
              </div>

              <dl
                className="rounded-[11px] p-3.5 mb-5 text-[13px]"
                style={{ background: 'var(--bg-hover)' }}
              >
                <div className="flex justify-between gap-3 mb-1.5">
                  <dt className="text-ink-muted">{tr('Adresse invitée', 'Invited address')}</dt>
                  <dd className="text-ink font-medium truncate">{preview.email}</dd>
                </div>
                <div className="flex justify-between gap-3">
                  <dt className="text-ink-muted">{tr('Rôle', 'Role')}</dt>
                  <dd className="text-ink font-medium">{preview.role}</dd>
                </div>
              </dl>

              {signedInEmail && signedInEmail.toLowerCase() !== preview.email.toLowerCase() && (
                <div
                  role="alert"
                  className="mb-4 px-3 py-2.5 rounded-[10px] text-[12.5px] flex items-start gap-2"
                  style={{
                    background: 'color-mix(in srgb, var(--high) 12%, transparent)',
                    color: 'var(--high)',
                  }}
                >
                  <AlertTriangle size={14} className="shrink-0 mt-0.5" />
                  <span>
                    {tr(
                      `Vous êtes connecté en tant que ${signedInEmail}. Cette invitation est destinée à ${preview.email}.`,
                      `You are signed in as ${signedInEmail}. This invitation is for ${preview.email}.`,
                    )}
                  </span>
                </div>
              )}

              <form onSubmit={accept}>
                {preview.requires_signup && (
                  <>
                    <label
                      className="block text-[12px] font-semibold text-ink-soft mb-1.5"
                      htmlFor="acc-name"
                    >
                      {tr('Votre nom complet', 'Your full name')}
                    </label>
                    <input
                      id="acc-name"
                      required
                      value={fullName}
                      autoFocus
                      onChange={(e) => setFullName(e.target.value)}
                      className="w-full h-[42px] px-3.5 rounded-[11px] text-[14px] text-ink outline-none mb-3"
                      style={{
                        border: '1px solid var(--border-strong)',
                        background: 'var(--bg-app)',
                      }}
                    />
                    <label
                      className="block text-[12px] font-semibold text-ink-soft mb-1.5"
                      htmlFor="acc-pw"
                    >
                      {tr('Choisissez un mot de passe', 'Choose a password')}
                    </label>
                    <input
                      id="acc-pw"
                      type="password"
                      required
                      minLength={12}
                      value={password}
                      onChange={(e) => setPassword(e.target.value)}
                      aria-describedby="acc-pw-help"
                      className="w-full h-[42px] px-3.5 rounded-[11px] text-[14px] text-ink outline-none mb-1.5"
                      style={{
                        border: '1px solid var(--border-strong)',
                        background: 'var(--bg-app)',
                      }}
                    />
                    <p id="acc-pw-help" className="text-[11.5px] text-ink-muted mb-4">
                      {tr('12 caractères minimum.', 'At least 12 characters.')}
                    </p>
                  </>
                )}

                {submitError && (
                  <div
                    role="alert"
                    className="mb-3 px-3 py-2.5 rounded-[10px] text-[12.5px] flex items-start gap-2"
                    style={{
                      background: 'color-mix(in srgb, var(--critical) 12%, transparent)',
                      color: 'var(--critical)',
                    }}
                  >
                    <AlertTriangle size={14} className="shrink-0 mt-0.5" />{' '}
                    <span>{submitError}</span>
                  </div>
                )}

                <button
                  type="submit"
                  disabled={submitting}
                  className="w-full h-[44px] rounded-[10px] text-[14px] font-semibold inline-flex items-center justify-center gap-2 disabled:opacity-60"
                  style={{ background: 'var(--accent-solid)', color: 'var(--fg-on-solid)' }}
                >
                  {submitting && <Loader2 size={15} className="animate-spin" />}
                  {submitting
                    ? tr('Un instant…', 'One moment…')
                    : preview.requires_signup
                      ? tr('Créer mon compte et rejoindre', 'Create my account and join')
                      : tr("Rejoindre l'organisation", 'Join the organization')}
                </button>
              </form>

              <p className="text-[11.5px] text-ink-muted mt-4 text-center leading-snug">
                {tr('Ce lien expire le ', 'This link expires on ')}
                {new Date(preview.expires_at).toLocaleDateString(
                  localeTag(lang),
                  {
                    day: 'numeric',
                    month: 'long',
                    year: 'numeric',
                  },
                )}
                .
              </p>
            </>
          )}
        </div>
      </div>
    </div>
  );
}

export default AcceptInvitationPage;
