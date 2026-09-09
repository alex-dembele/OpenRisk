// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// The five wizard steps (spec §4). Shared rules, applied by every one of them:
//
//   • Savable   — "Continue" PUTs the step before navigating; nothing is lost.
//   • Resumable — fields are seeded from the server's stored answers.
//   • Reversible— "Back" is always present (except on step 1) and never destructive.
//   • Skippable where honest — the team step says so out loud, because pretending
//     an invitation is mandatory is how you lose someone on their first day.

import { useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router';
import { ArrowLeft, ArrowRight, Check, Copy, Loader2, Users } from 'lucide-react';
import { toast } from 'sonner';

import { useUIStore } from '../../../store/uiStore';
import { useAuthStore } from '../../../hooks/useAuthStore';
import { i18n, type OnboardingStepKey } from '../../../services/activationService';
import {
  useCompleteOnboarding,
  useOnboardingState,
  useOnboardingSuggestions,
  useSaveOnboardingStep,
} from '../useActivation';
import { useCatalogs, useImportCatalogAsFramework } from '../../compliance/useCompliance';
import { WIZARD_STEPS, stepPath } from './OnboardingWizard';
import type { LocaleCode } from '../../../i18n/locales';

// ---------------------------------------------------------------------------
// Shared primitives
// ---------------------------------------------------------------------------

const inputCls =
  'w-full h-11 px-3.5 rounded-[10px] text-[14px] text-ink outline-none transition-colors';
const inputStyle: React.CSSProperties = {
  background: 'var(--bg-elevated)',
  border: '1px solid var(--border-strong)',
};

function Field({
  label,
  hint,
  children,
  htmlFor,
}: {
  label: string;
  hint?: string;
  children: React.ReactNode;
  htmlFor?: string;
}) {
  return (
    <div className="mb-4">
      <label htmlFor={htmlFor} className="block text-[12.5px] font-semibold text-ink mb-1.5">
        {label}
      </label>
      {children}
      {hint && <div className="text-[11.5px] text-ink-muted mt-1">{hint}</div>}
    </div>
  );
}

function StepShell({
  title,
  subtitle,
  children,
  onBack,
  onNext,
  nextLabel,
  nextDisabled,
  busy,
  secondary,
}: {
  title: string;
  subtitle: string;
  children: React.ReactNode;
  onBack?: () => void;
  onNext: () => void;
  nextLabel: string;
  nextDisabled?: boolean;
  busy?: boolean;
  secondary?: React.ReactNode;
}) {
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        if (!busy && !nextDisabled) onNext();
      }}
      style={{ animation: 'or-fadeup .35s ease' }}
    >
      <h1 className="disp text-[24px] font-bold text-ink mb-1.5">{title}</h1>
      <p className="text-[14px] text-ink-soft mb-6">{subtitle}</p>

      {children}

      <div className="flex items-center gap-3 mt-7 flex-wrap">
        {onBack && (
          <button
            type="button"
            onClick={onBack}
            className="h-11 px-4 rounded-[10px] text-[13.5px] font-semibold text-ink inline-flex items-center gap-1.5"
            style={{ background: 'var(--bg-hover)', border: '1px solid var(--border-strong)' }}
          >
            <ArrowLeft size={15} />
            Retour
          </button>
        )}
        <button
          type="submit"
          disabled={busy || nextDisabled}
          data-testid="wizard-next"
          className="h-11 px-5 rounded-[10px] text-[13.5px] font-semibold inline-flex items-center gap-2 disabled:opacity-50"
          style={{
            background: 'var(--accent-solid)',
            color: 'var(--fg-on-solid)',
          }}
        >
          {busy ? <Loader2 size={15} className="animate-spin" /> : null}
          {nextLabel}
          {!busy && <ArrowRight size={15} />}
        </button>
        {secondary}
      </div>
    </form>
  );
}

/** Read a stored answer for a step, so a resumed wizard shows what was typed. */
function useStoredAnswers(step: OnboardingStepKey): Record<string, unknown> {
  const { data } = useOnboardingState();
  return useMemo(() => (data?.answers?.[step] ?? {}) as Record<string, unknown>, [data, step]);
}

function str(answers: Record<string, unknown>, key: string, fallback = ''): string {
  const v = answers[key];
  return typeof v === 'string' ? v : fallback;
}

/** Save-then-navigate, shared by every step. */
function useStepNav(step: OnboardingStepKey) {
  const navigate = useNavigate();
  const save = useSaveOnboardingStep();
  const index = WIZARD_STEPS.findIndex((s) => s.key === step);

  const go = (answers: Record<string, unknown>, direction: 1 | -1) => {
    const target = WIZARD_STEPS[Math.min(WIZARD_STEPS.length - 1, Math.max(0, index + direction))];
    save.mutate(
      { step, answers, next: target.key },
      {
        onSuccess: () => navigate(stepPath(target.key)),
        onError: () =>
          toast.error(
            "Impossible d'enregistrer cette étape. Vérifiez votre connexion et réessayez.",
          ),
      },
    );
  };

  return { go, busy: save.isPending, index };
}

// ---------------------------------------------------------------------------
// 1. Organization
// ---------------------------------------------------------------------------

const SIZES = ['1-50', '51-200', '201-1000', '1000+'];

const COUNTRIES = [
  { code: 'CM', fr: 'Cameroun', en: 'Cameroon' },
  { code: 'SN', fr: 'Sénégal', en: 'Senegal' },
  { code: 'CI', fr: "Côte d'Ivoire", en: "Côte d'Ivoire" },
  { code: 'GA', fr: 'Gabon', en: 'Gabon' },
  { code: 'CG', fr: 'Congo', en: 'Congo' },
  { code: 'BJ', fr: 'Bénin', en: 'Benin' },
  { code: 'BF', fr: 'Burkina Faso', en: 'Burkina Faso' },
  { code: 'ML', fr: 'Mali', en: 'Mali' },
  { code: 'TG', fr: 'Togo', en: 'Togo' },
  { code: 'FR', fr: 'France', en: 'France' },
  { code: 'BE', fr: 'Belgique', en: 'Belgium' },
  { code: 'MA', fr: 'Maroc', en: 'Morocco' },
  { code: 'TN', fr: 'Tunisie', en: 'Tunisia' },
  { code: 'DZ', fr: 'Algérie', en: 'Algeria' },
  { code: 'US', fr: 'États-Unis', en: 'United States' },
  { code: 'CA', fr: 'Canada', en: 'Canada' },
];

const CURRENCIES = ['XAF', 'XOF', 'EUR', 'USD', 'NGN', 'MAD', 'GHS', 'ZAR'];

export function OrganizationStep() {
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const stored = useStoredAnswers('organization');
  const { go, busy } = useStepNav('organization');
  const { data: suggestions } = useOnboardingSuggestions();
  const orgName = useAuthStore((s) => s.user?.org_name);

  const [name, setName] = useState('');
  const [industry, setIndustry] = useState('');
  const [size, setSize] = useState('');
  const [country, setCountry] = useState('');
  const [currency, setCurrency] = useState('');
  const [timezone, setTimezone] = useState('');

  // Seed from the server's stored answers once they arrive (resume), falling
  // back to what we already know about the account.
  useEffect(() => {
    setName(str(stored, 'name', orgName ?? ''));
    setIndustry(str(stored, 'industry'));
    setSize(str(stored, 'size'));
    setCountry(str(stored, 'country'));
    setCurrency(str(stored, 'currency'));
    setTimezone(str(stored, 'timezone', Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC'));
  }, [stored, orgName]);

  const answers = { name, industry, size, country, currency, timezone };

  return (
    <StepShell
      title={tr('Votre organisation', 'Your organization')}
      subtitle={tr(
        'Ces réponses choisissent les référentiels et les risques que nous vous proposerons — rien de plus.',
        'These answers pick the frameworks and risks we will suggest — nothing more.',
      )}
      onNext={() => go(answers, 1)}
      nextLabel={tr('Continuer', 'Continue')}
      nextDisabled={!name.trim()}
      busy={busy}
    >
      <Field label={tr("Nom de l'organisation", 'Organization name')} htmlFor="org-name">
        <input
          id="org-name"
          data-testid="org-name"
          className={inputCls}
          style={inputStyle}
          value={name}
          onChange={(e) => setName(e.target.value)}
          autoFocus
        />
      </Field>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-x-4">
        <Field label={tr("Secteur d'activité", 'Industry')} htmlFor="org-industry">
          <select
            id="org-industry"
            data-testid="org-industry"
            className={inputCls}
            style={inputStyle}
            value={industry}
            onChange={(e) => setIndustry(e.target.value)}
          >
            <option value="">{tr('— Choisir —', '— Select —')}</option>
            {(suggestions?.sectors ?? []).map((s) => (
              <option key={s.key} value={s.key}>
                {i18n(s.label_i18n, lang)}
              </option>
            ))}
          </select>
        </Field>

        <Field label={tr('Taille', 'Size')} htmlFor="org-size">
          <select
            id="org-size"
            className={inputCls}
            style={inputStyle}
            value={size}
            onChange={(e) => setSize(e.target.value)}
          >
            <option value="">{tr('— Choisir —', '— Select —')}</option>
            {SIZES.map((s) => (
              <option key={s} value={s}>
                {s} {tr('salariés', 'employees')}
              </option>
            ))}
          </select>
        </Field>

        <Field label={tr('Pays', 'Country')} htmlFor="org-country">
          <select
            id="org-country"
            data-testid="org-country"
            className={inputCls}
            style={inputStyle}
            value={country}
            onChange={(e) => setCountry(e.target.value)}
          >
            <option value="">{tr('— Choisir —', '— Select —')}</option>
            {COUNTRIES.map((c) => (
              <option key={c.code} value={c.code}>
                {lang === 'fr' ? c.fr : c.en}
              </option>
            ))}
          </select>
        </Field>

        <Field label={tr('Devise', 'Currency')} htmlFor="org-currency">
          <select
            id="org-currency"
            className={inputCls}
            style={inputStyle}
            value={currency}
            onChange={(e) => setCurrency(e.target.value)}
          >
            <option value="">{tr('— Choisir —', '— Select —')}</option>
            {CURRENCIES.map((c) => (
              <option key={c} value={c}>
                {c}
              </option>
            ))}
          </select>
        </Field>
      </div>

      <Field
        label={tr('Fuseau horaire', 'Time zone')}
        hint={tr(
          'Pré-rempli depuis votre navigateur — les échéances et les SLA le suivront.',
          'Pre-filled from your browser — deadlines and SLAs will follow it.',
        )}
        htmlFor="org-tz"
      >
        <input
          id="org-tz"
          className={inputCls}
          style={inputStyle}
          value={timezone}
          onChange={(e) => setTimezone(e.target.value)}
        />
      </Field>
    </StepShell>
  );
}

// ---------------------------------------------------------------------------
// 2. Profile
// ---------------------------------------------------------------------------

export function ProfileStep() {
  const lang = useUIStore((s) => s.lang);
  const setLang = useUIStore((s) => s.setLang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const stored = useStoredAnswers('profile');
  const { go, busy } = useStepNav('profile');
  const user = useAuthStore((s) => s.user);

  const [fullName, setFullName] = useState('');
  const [jobTitle, setJobTitle] = useState('');
  const [avatarUrl, setAvatarUrl] = useState('');
  const [language, setLanguage] = useState<LocaleCode>(lang);
  const [notifyInApp, setNotifyInApp] = useState(true);
  const [notifyEmail, setNotifyEmail] = useState(true);

  useEffect(() => {
    setFullName(str(stored, 'full_name', user?.full_name ?? ''));
    setJobTitle(str(stored, 'job_title', user?.department ?? ''));
    setAvatarUrl(str(stored, 'avatar_url'));
    const storedLang = str(stored, 'language');
    if (storedLang === 'fr' || storedLang === 'en') setLanguage(storedLang);
    if (typeof stored.notify_in_app === 'boolean') setNotifyInApp(stored.notify_in_app);
    if (typeof stored.notify_email === 'boolean') setNotifyEmail(stored.notify_email);
  }, [stored, user]);

  const answers = {
    full_name: fullName,
    job_title: jobTitle,
    avatar_url: avatarUrl,
    language,
    notify_in_app: notifyInApp,
    notify_email: notifyEmail,
  };

  const submit = (direction: 1 | -1) => {
    // Apply the language immediately: it is a display preference, and making
    // someone wait for a round-trip to read their own interface is absurd.
    if (language !== lang) setLang(language);
    go(answers, direction);
  };

  return (
    <StepShell
      title={tr('Votre profil', 'Your profile')}
      subtitle={tr(
        'Votre nom rend les assignations lisibles par vos collègues.',
        'Your name is what makes assignments readable to your colleagues.',
      )}
      onBack={() => submit(-1)}
      onNext={() => submit(1)}
      nextLabel={tr('Continuer', 'Continue')}
      nextDisabled={!fullName.trim()}
      busy={busy}
    >
      <Field label={tr('Nom complet', 'Full name')} htmlFor="p-name">
        <input
          id="p-name"
          data-testid="profile-name"
          className={inputCls}
          style={inputStyle}
          value={fullName}
          onChange={(e) => setFullName(e.target.value)}
          autoFocus
        />
      </Field>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-x-4">
        <Field label={tr('Fonction', 'Job title')} htmlFor="p-job">
          <input
            id="p-job"
            className={inputCls}
            style={inputStyle}
            value={jobTitle}
            onChange={(e) => setJobTitle(e.target.value)}
            placeholder={tr('RSSI, DSI, Auditeur…', 'CISO, CIO, Auditor…')}
          />
        </Field>

        <Field label={tr('Langue', 'Language')} htmlFor="p-lang">
          <select
            id="p-lang"
            className={inputCls}
            style={inputStyle}
            value={language}
            onChange={(e) => setLanguage(e.target.value as LocaleCode)}
          >
            <option value="fr">Français</option>
            <option value="en">English</option>
          </select>
        </Field>
      </div>

      <Field
        label={tr('Avatar (URL)', 'Avatar (URL)')}
        hint={tr('Facultatif.', 'Optional.')}
        htmlFor="p-avatar"
      >
        <input
          id="p-avatar"
          className={inputCls}
          style={inputStyle}
          value={avatarUrl}
          onChange={(e) => setAvatarUrl(e.target.value)}
          placeholder="https://…"
        />
      </Field>

      <fieldset className="mt-2">
        <legend className="text-[12.5px] font-semibold text-ink mb-2">
          {tr('Comment souhaitez-vous être alerté ?', 'How should we alert you?')}
        </legend>
        {[
          {
            id: 'n-inapp',
            checked: notifyInApp,
            set: setNotifyInApp,
            label: tr('Dans l’application', 'In-app'),
          },
          {
            id: 'n-mail',
            checked: notifyEmail,
            set: setNotifyEmail,
            label: tr('Par e-mail', 'By email'),
          },
        ].map((row) => (
          <label
            key={row.id}
            htmlFor={row.id}
            className="flex items-center gap-2.5 text-[13px] text-ink mb-2 cursor-pointer"
          >
            <input
              id={row.id}
              type="checkbox"
              checked={row.checked}
              onChange={(e) => row.set(e.target.checked)}
              className="w-4 h-4"
            />
            {row.label}
          </label>
        ))}
      </fieldset>
    </StepShell>
  );
}

// ---------------------------------------------------------------------------
// 3. Goal — this is what selects the template that gets loaded
// ---------------------------------------------------------------------------

export function GoalStep() {
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const stored = useStoredAnswers('goal');
  const { go, busy } = useStepNav('goal');
  const { data: suggestions } = useOnboardingSuggestions();

  const [goal, setGoal] = useState('');
  useEffect(() => setGoal(str(stored, 'goal')), [stored]);

  return (
    <StepShell
      title={tr("Qu'est-ce qui vous amène ?", 'What brings you here?')}
      subtitle={tr(
        'Votre réponse décide des référentiels proposés à l’étape suivante et de votre écran d’arrivée.',
        'Your answer decides which frameworks come next and where you land.',
      )}
      onBack={() => go({ goal }, -1)}
      onNext={() => go({ goal }, 1)}
      nextLabel={tr('Continuer', 'Continue')}
      nextDisabled={!goal}
      busy={busy}
    >
      <div className="flex flex-col gap-2.5">
        {(suggestions?.goals ?? []).map((g) => {
          const active = goal === g.key;
          return (
            <button
              key={g.key}
              type="button"
              data-testid={`goal-${g.key}`}
              onClick={() => setGoal(g.key)}
              aria-pressed={active}
              className="text-left rounded-[12px] p-4 flex items-center gap-3 transition-colors"
              style={{
                background: active ? 'var(--accent-soft)' : 'var(--bg-elevated)',
                border: `1px solid ${active ? 'var(--accent)' : 'var(--border-strong)'}`,
              }}
            >
              <span
                className="w-5 h-5 rounded-full shrink-0 flex items-center justify-center"
                style={{
                  border: `2px solid ${active ? 'var(--accent)' : 'var(--border-strong)'}`,
                  background: active ? 'var(--accent)' : 'transparent',
                  color: '#fff',
                }}
              >
                {active && <Check size={12} strokeWidth={3} />}
              </span>
              <span className="text-[14px] font-semibold text-ink">{i18n(g.label_i18n, lang)}</span>
            </button>
          );
        })}
      </div>
    </StepShell>
  );
}

// ---------------------------------------------------------------------------
// 4. Framework — suggested from sector + country, one-click import
// ---------------------------------------------------------------------------

export function FrameworkStep() {
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const { go, busy } = useStepNav('framework');
  const { data: suggestions, isLoading } = useOnboardingSuggestions();
  const { data: catalogs } = useCatalogs();
  const importCatalog = useImportCatalogAsFramework();
  const [imported, setImported] = useState<string[]>([]);

  // The suggested keys, resolved against the real catalog registry so we never
  // offer something that cannot actually be imported.
  const offered = useMemo(() => {
    const byKey = new Map((catalogs ?? []).map((c) => [c.key, c]));
    return (suggestions?.frameworks ?? [])
      .map((k) => byKey.get(k))
      .filter((c): c is NonNullable<typeof c> => !!c && c.available !== false)
      .slice(0, 5);
  }, [suggestions, catalogs]);

  const runImport = (key: string) => {
    const catalog = offered.find((c) => c.key === key);
    if (!catalog) return;
    importCatalog.mutate(catalog, {
      onSuccess: ({ result }) => {
        setImported((prev) => (prev.includes(key) ? prev : [...prev, key]));
        toast.success(
          tr(
            `${result.imported} contrôles importés — vos écarts sont calculés.`,
            `${result.imported} controls imported — your gaps are computed.`,
          ),
        );
      },
      onError: () =>
        toast.error(tr("L'import a échoué. Réessayez.", 'The import failed. Try again.')),
    });
  };

  return (
    <StepShell
      title={tr('Votre référentiel', 'Your framework')}
      subtitle={tr(
        'Suggérés d’après votre secteur et votre pays. Un clic suffit — vous pourrez en ajouter d’autres plus tard.',
        'Suggested from your sector and country. One click is enough — you can add more later.',
      )}
      onBack={() => go({ imported }, -1)}
      onNext={() => go({ imported }, 1)}
      nextLabel={imported.length ? tr('Continuer', 'Continue') : tr('Plus tard', 'Later')}
      busy={busy}
    >
      {isLoading && (
        <div className="flex items-center gap-2 text-[13px] text-ink-soft">
          <Loader2 size={15} className="animate-spin" />
          {tr('Sélection des référentiels…', 'Selecting frameworks…')}
        </div>
      )}

      <div className="flex flex-col gap-2.5">
        {offered.map((c) => {
          const done = imported.includes(c.key);
          const pending = importCatalog.isPending && importCatalog.variables?.key === c.key;
          return (
            <div
              key={c.key}
              className="rounded-[12px] p-4 flex items-center gap-3"
              style={{
                background: 'var(--bg-elevated)',
                border: `1px solid ${done ? 'var(--low)' : 'var(--border-strong)'}`,
              }}
            >
              <div className="flex-1 min-w-0">
                <div className="text-[14px] font-semibold text-ink">
                  {c.name} {c.version && <span className="text-ink-muted">· {c.version}</span>}
                </div>
                <div className="text-[12px] text-ink-soft mt-0.5 line-clamp-2">{c.description}</div>
              </div>
              <button
                type="button"
                data-testid={`import-${c.key}`}
                disabled={done || pending}
                onClick={() => runImport(c.key)}
                className="h-9 px-3.5 rounded-[9px] text-[12.5px] font-semibold inline-flex items-center gap-1.5 shrink-0 disabled:opacity-60"
                style={
                  done
                    ? {
                        background: 'color-mix(in srgb,var(--low) 16%,transparent)',
                        color: 'var(--low)',
                      }
                    : {
                        background: 'var(--accent-solid)',
                        color: 'var(--fg-on-solid)',
                      }
                }
              >
                {pending ? (
                  <Loader2 size={14} className="animate-spin" />
                ) : done ? (
                  <Check size={14} strokeWidth={3} />
                ) : null}
                {done ? tr('Importé', 'Imported') : tr('Importer', 'Import')}
              </button>
            </div>
          );
        })}
      </div>

      {!isLoading && offered.length === 0 && (
        <div className="text-[13px] text-ink-soft">
          {tr(
            'Aucune suggestion pour ces réponses — vous pourrez choisir un référentiel depuis Conformité.',
            'No suggestion for these answers — you can pick a framework from Compliance.',
          )}
        </div>
      )}
    </StepShell>
  );
}

// ---------------------------------------------------------------------------
// 5. Team — skippable, with a copyable share link
// ---------------------------------------------------------------------------

export function TeamStep() {
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const navigate = useNavigate();
  const stored = useStoredAnswers('team');
  const save = useSaveOnboardingStep();
  const complete = useCompleteOnboarding();
  const { data: state } = useOnboardingState();

  const [emails, setEmails] = useState('');
  useEffect(() => setEmails(str(stored, 'emails')), [stored]);

  const shareLink = `${window.location.origin}/register`;

  const finish = () => {
    // Save the answers, then lift the guard. The invitations themselves are sent
    // from Settings › Members, which owns roles and permissions — duplicating
    // that flow here would mean two places to keep correct.
    save.mutate(
      { step: 'team', answers: { emails } },
      {
        onSettled: () =>
          complete.mutate(undefined, {
            onSuccess: (s) => navigate(s.landing || '/'),
            onError: () =>
              toast.error(
                tr(
                  'Impossible de terminer la configuration. Réessayez.',
                  'Could not finish setup. Try again.',
                ),
              ),
          }),
      },
    );
  };

  const busy = save.isPending || complete.isPending;

  return (
    <StepShell
      title={tr('Invitez votre équipe', 'Invite your team')}
      subtitle={tr(
        'Facultatif — vous pouvez commencer seul et inviter plus tard.',
        'Optional — you can start alone and invite later.',
      )}
      onBack={() => {
        save.mutate({ step: 'team', answers: { emails }, next: 'framework' });
        navigate(stepPath('framework'));
      }}
      onNext={finish}
      nextLabel={tr('Terminer', 'Finish')}
      busy={busy}
      secondary={
        <button
          type="button"
          data-testid="wizard-skip"
          onClick={finish}
          disabled={busy}
          className="h-11 px-4 rounded-[10px] text-[13.5px] font-semibold text-ink-soft"
          style={{ background: 'transparent' }}
        >
          {tr('Passer cette étape', 'Skip this step')}
        </button>
      }
    >
      <Field
        label={tr('Adresses e-mail', 'Email addresses')}
        hint={tr(
          'Une par ligne. Nous préparons la liste ; les invitations partent depuis Paramètres › Membres, où vous choisissez le rôle de chacun.',
          'One per line. We keep the list; invitations are sent from Settings › Members, where you pick each person’s role.',
        )}
        htmlFor="team-emails"
      >
        <textarea
          id="team-emails"
          data-testid="team-emails"
          className="w-full px-3.5 py-2.5 rounded-[10px] text-[14px] text-ink outline-none min-h-[96px]"
          style={inputStyle}
          value={emails}
          onChange={(e) => setEmails(e.target.value)}
          placeholder={'awa@exemple.cm\nmoussa@exemple.cm'}
        />
      </Field>

      <div
        className="rounded-[12px] p-4 flex items-center gap-3"
        style={{ background: 'var(--bg-elevated)', border: '1px solid var(--border-strong)' }}
      >
        <Users size={18} style={{ color: 'var(--accent-500)' }} />
        <div className="flex-1 min-w-0">
          <div className="text-[13px] font-semibold text-ink">
            {tr('Lien de partage', 'Share link')}
          </div>
          <div className="text-[12px] text-ink-soft truncate mono">{shareLink}</div>
        </div>
        <button
          type="button"
          onClick={() => {
            void navigator.clipboard
              ?.writeText(shareLink)
              .then(() => toast.success(tr('Lien copié', 'Link copied')))
              .catch(() => toast.error(tr('Copie impossible', 'Copy failed')));
          }}
          className="h-9 px-3 rounded-[9px] text-[12.5px] font-semibold text-ink inline-flex items-center gap-1.5 shrink-0"
          style={{ background: 'var(--bg-hover)', border: '1px solid var(--border-strong)' }}
        >
          <Copy size={14} />
          {tr('Copier', 'Copy')}
        </button>
      </div>

      {state?.goal && (
        <div className="text-[12.5px] text-ink-muted mt-4">
          {tr('Vous arriverez sur ', 'You will land on ')}
          <span className="mono">{state.landing}</span>.
        </div>
      )}
    </StepShell>
  );
}
