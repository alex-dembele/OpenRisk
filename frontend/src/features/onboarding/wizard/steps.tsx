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
import { Check, Loader2 } from 'lucide-react';
import { toast } from 'sonner';

import { useUIStore } from '../../../store/uiStore';
import { useAuthStore } from '../../../hooks/useAuthStore';
import { i18n, type OnboardingStepKey } from '../../../services/activationService';
import { useAdoptStarterRisks, useOnboardingSuggestions, useStarterRisks } from '../useActivation';
import { useCatalogs, useImportCatalogAsFramework } from '../../compliance/useCompliance';
import type { StarterRiskOffer } from '../../../services/activationService';
import { Field, StepShell } from './stepPrimitives';
import { inputCls, inputStyle, str, useStepNav, useStoredAnswers } from './stepNav';
import type { LocaleCode } from '../../../i18n/locales';

// ---------------------------------------------------------------------------
// Shared primitives
// ---------------------------------------------------------------------------

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
  const { go, busy, error, retry } = useStepNav('organization');
  const { data: suggestions } = useOnboardingSuggestions();
  const orgName = useAuthStore((s) => s.user?.org_name);
  const user = useAuthStore((s) => s.user);

  const [name, setName] = useState('');
  const [industry, setIndustry] = useState('');
  const [size, setSize] = useState('');
  const [country, setCountry] = useState('');
  const [currency, setCurrency] = useState('');
  const [timezone, setTimezone] = useState('');
  // #438 merged the retired `profile` route into this step. The person's fields
  // live here now; the server records the `profile` checklist milestone from the
  // same submission, so the row still ticks from a server fact.
  const [fullName, setFullName] = useState('');
  const [jobTitle, setJobTitle] = useState('');

  // Seed from the server's stored answers once they arrive (resume), falling
  // back to what we already know about the account. `profile` is read as a
  // fallback source: a user who walked the OLD five-step wizard has their name
  // stored under that retired key, and asking them to type it again would be a
  // regression they would rightly report.
  const legacyProfile = useStoredAnswers('profile' as OnboardingStepKey);
  useEffect(() => {
    setName(str(stored, 'name', orgName ?? ''));
    setIndustry(str(stored, 'industry'));
    setSize(str(stored, 'size'));
    setCountry(str(stored, 'country'));
    setCurrency(str(stored, 'currency'));
    setTimezone(str(stored, 'timezone', Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC'));
    setFullName(str(stored, 'full_name', str(legacyProfile, 'full_name', user?.full_name ?? '')));
    setJobTitle(str(stored, 'job_title', str(legacyProfile, 'job_title', user?.department ?? '')));
  }, [stored, legacyProfile, orgName, user]);

  const answers = {
    name,
    industry,
    size,
    country,
    currency,
    timezone,
    full_name: fullName,
    job_title: jobTitle,
  };

  return (
    <StepShell
      title={tr('Votre organisation', 'Your organization')}
      subtitle={tr(
        'Le secteur et le pays choisissent le référentiel que nous vous proposerons ; votre nom rend les assignations lisibles par vos collègues.',
        'Your sector and country pick the framework we will propose; your name is what makes assignments readable to your colleagues.',
      )}
      onNext={() => go(answers, 1)}
      nextLabel={tr('Continuer', 'Continue')}
      // Both halves are required: the company names the tenant, the person makes
      // assignments readable. The server ticks the `profile` checklist row only
      // when a name is given, so letting this through empty would leave a row
      // that can never tick.
      nextDisabled={!name.trim() || !fullName.trim()}
      busy={busy}
      error={error}
      onRetry={retry}
      errorLabel={tr("Impossible d'enregistrer cette étape.", 'This step could not be saved.')}
      errorHint={tr(
        'Vos réponses sont conservées — réessayez, rien n\u2019est perdu.',
        'Your answers are kept — try again, nothing is lost.',
      )}
      retryLabel={tr('Réessayer', 'Try again')}
    >
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-x-4">
        <Field label={tr('Votre nom complet', 'Your full name')} htmlFor="p-name">
          <input
            id="p-name"
            data-testid="profile-name"
            className={inputCls}
            style={inputStyle}
            value={fullName}
            onChange={(e) => setFullName(e.target.value)}
          />
        </Field>
        <Field label={tr('Votre fonction', 'Your job title')} htmlFor="p-job">
          <input
            id="p-job"
            data-testid="profile-job"
            className={inputCls}
            style={inputStyle}
            value={jobTitle}
            onChange={(e) => setJobTitle(e.target.value)}
            placeholder={tr('RSSI, DSI, Auditeur…', 'CISO, CIO, Auditor…')}
          />
        </Field>
      </div>

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
// 2. Goal — this is what selects the template that gets loaded
// ---------------------------------------------------------------------------

export function GoalStep() {
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const stored = useStoredAnswers('goal');
  const { go, busy, error, retry } = useStepNav('goal');
  const { data: suggestions } = useOnboardingSuggestions();

  // #438 step 2: the objective filters, ON THE SAME SCREEN, eight pre-written
  // statements of which the user picks three. The eight come from the server —
  // they become REAL ROWS in this tenant's register, so the client never
  // authors them and never posts their text back.
  const { data: offer, isLoading: offerLoading } = useStarterRisks();
  const adopt = useAdoptStarterRisks();

  const [goal, setGoal] = useState('');
  const [picked, setPicked] = useState<string[]>([]);
  useEffect(() => {
    setGoal(str(stored, 'goal'));
    const storedPicks = stored.starter_risks;
    if (Array.isArray(storedPicks)) {
      setPicked(storedPicks.filter((k): k is string => typeof k === 'string'));
    }
  }, [stored]);

  const pick = offer?.pick ?? 3;
  const alreadyAdopted = offer?.already_adopted === true;
  const enoughPicked = alreadyAdopted || picked.length === pick;

  const toggle = (key: string) => {
    setPicked((current) => {
      if (current.includes(key)) return current.filter((k) => k !== key);
      // Hard cap rather than a warning: the server refuses anything but three,
      // and letting the user select a fourth only to be rejected on submit is a
      // worse way to learn the rule.
      if (current.length >= pick) return current;
      return [...current, key];
    });
  };

  const submit = (direction: 1 | -1) => {
    // Forward means the statements become rows. Backwards never writes: going
    // back to change the objective must not leave three risks behind.
    if (direction === 1 && !alreadyAdopted && picked.length === pick) {
      adopt.mutate(picked, {
        onSuccess: () => go({ goal, starter_risks: picked }, 1),
        // A 409 means this tenant already adopted — expected on a resumed
        // tunnel, and not a reason to block the user on a screen they finished.
        onError: (err: unknown) => {
          if (isExpectedAdoptionRefusal(err)) go({ goal, starter_risks: picked }, 1);
        },
      });
      return;
    }
    go({ goal, starter_risks: picked }, direction);
  };

  return (
    <StepShell
      title={tr("Qu'est-ce qui vous amène ?", 'What brings you here?')}
      subtitle={tr(
        'Votre réponse décide des référentiels proposés à l’étape suivante et de votre écran d’arrivée.',
        'Your answer decides which frameworks come next and where you land.',
      )}
      onBack={() => submit(-1)}
      onNext={() => submit(1)}
      nextLabel={tr('Continuer', 'Continue')}
      nextDisabled={!goal || !enoughPicked}
      busy={busy || adopt.isPending}
      error={error}
      onRetry={retry}
      errorLabel={tr("Impossible d'enregistrer cette étape.", 'This step could not be saved.')}
      errorHint={tr(
        'Vos réponses sont conservées — réessayez, rien n\u2019est perdu.',
        'Your answers are kept — try again, nothing is lost.',
      )}
      retryLabel={tr('Réessayer', 'Try again')}
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

      <StarterRiskPicker
        offer={offer}
        loading={offerLoading}
        picked={picked}
        pick={pick}
        alreadyAdopted={alreadyAdopted}
        onToggle={toggle}
        failed={adopt.isError && !isExpectedAdoptionRefusal(adopt.error)}
        lang={lang}
        tr={tr}
      />
    </StepShell>
  );
}

/**
 * The eight statements, of which the user picks three (#438 step 2).
 *
 * Grouped by scope so a banker sees "typical of your sector" above "applies to
 * any organisation" instead of eight cards in storage order — the ordering IS
 * the value here, and it comes from the server.
 *
 * A pressed card is a toggle button, not a checkbox in a label, because the
 * whole card is the hit target; `aria-pressed` is what tells a screen reader it
 * is a two-state control.
 */
function StarterRiskPicker({
  offer,
  loading,
  picked,
  pick,
  alreadyAdopted,
  onToggle,
  failed,
  lang,
  tr,
}: {
  offer: StarterRiskOffer | undefined;
  loading: boolean;
  picked: string[];
  pick: number;
  alreadyAdopted: boolean;
  onToggle: (key: string) => void;
  failed: boolean;
  lang: LocaleCode;
  tr: (fr: string, en: string) => string;
}) {
  const scopeLabel = (scope: string) =>
    scope === 'sector'
      ? tr('Typique de votre secteur', 'Typical of your sector')
      : scope === 'region'
        ? tr('Fréquent dans votre région', 'Common in your region')
        : tr('Concerne toute organisation', 'Applies to any organisation');

  if (loading) {
    return (
      <div className="mt-6 flex flex-col gap-2" aria-busy="true">
        {[0, 1, 2, 3].map((i) => (
          <div key={i} className="h-14 rounded-[12px] or-skeleton" />
        ))}
      </div>
    );
  }

  // There is deliberately no empty state: StarterRisksFor always returns eight,
  // falling back to a generic sector. "Pick three of nothing" is a broken screen,
  // not an empty one, so an absent offer means the request failed and says so.
  if (!offer || offer.risks.length === 0) {
    return (
      <div className="mt-6 text-[13px] text-ink-soft" role="alert">
        {tr(
          'Les risques proposés n’ont pas pu être chargés. Vous pourrez les ajouter depuis le registre.',
          'The suggested risks could not be loaded. You can add them from the register instead.',
        )}
      </div>
    );
  }

  // Group headers computed BEFORE render rather than by mutating a variable
  // inside the map: a reassignment that survives the render is exactly how a
  // list starts showing the previous render's headings after a re-order.
  const rows = offer.risks.map((risk, i) => ({
    risk,
    header: i === 0 || offer.risks[i - 1].scope !== risk.scope ? scopeLabel(risk.scope) : '',
  }));

  return (
    <div className="mt-7">
      <div className="flex items-baseline justify-between mb-2.5">
        <h2 className="text-[14px] font-bold text-ink m-0">
          {tr(
            `Sélectionnez ${pick} risques qui vous concernent`,
            `Pick ${pick} risks that apply to you`,
          )}
        </h2>
        {/* aria-live so the count is announced as the user selects, which is how
            a screen-reader user knows when the primary button will unlock. */}
        <span className="text-[12.5px] font-semibold text-ink-soft" aria-live="polite">
          {picked.length}/{pick}
        </span>
      </div>

      {alreadyAdopted && (
        <div className="text-[12.5px] text-ink-soft mb-3" data-testid="starter-already-adopted">
          {tr(
            'Ces risques ont déjà été ajoutés à votre registre — rien ne sera dupliqué.',
            'These risks are already in your register — nothing will be duplicated.',
          )}
        </div>
      )}

      {failed && (
        <div className="text-[12.5px] mb-3" role="alert" style={{ color: 'var(--high)' }}>
          {tr(
            'Les risques n’ont pas pu être ajoutés. Réessayez — vos choix sont conservés.',
            'The risks could not be added. Try again — your picks are kept.',
          )}
        </div>
      )}

      <div className="flex flex-col gap-2">
        {rows.map(({ risk, header }) => {
          const active = picked.includes(risk.key);
          const atCap = !active && picked.length >= pick;

          return (
            <div key={risk.key}>
              {header && (
                <div className="text-[10.5px] uppercase tracking-wide text-ink-muted mt-3 mb-1.5">
                  {header}
                </div>
              )}
              <button
                type="button"
                data-testid={`starter-${risk.key}`}
                onClick={() => onToggle(risk.key)}
                aria-pressed={active}
                disabled={alreadyAdopted || atCap}
                className="w-full text-left rounded-[12px] p-3.5 flex items-start gap-3 disabled:opacity-55"
                style={{
                  background: active ? 'var(--accent-soft)' : 'var(--bg-elevated)',
                  border: `1px solid ${active ? 'var(--accent)' : 'var(--border-strong)'}`,
                }}
              >
                <span
                  className="w-5 h-5 rounded shrink-0 mt-0.5 flex items-center justify-center"
                  style={{
                    border: `2px solid ${active ? 'var(--accent)' : 'var(--border-strong)'}`,
                    background: active ? 'var(--accent)' : 'transparent',
                    color: 'var(--fg-on-solid)',
                  }}
                >
                  {active && <Check size={12} strokeWidth={3} />}
                </span>
                <span className="min-w-0">
                  <span className="block text-[13.5px] font-semibold text-ink">
                    {i18n(risk.title_i18n, lang)}
                  </span>
                  <span className="block text-[12px] text-ink-soft mt-0.5">
                    {i18n(risk.description_i18n, lang)}
                  </span>
                </span>
              </button>
            </div>
          );
        })}
      </div>
    </div>
  );
}

/**
 * Answers the adoption endpoint gives that are NOT failures to retry.
 *
 *   409 — this tenant already adopted. The tunnel is resumable by design, so a
 *         user WILL come back to this step; a second adoption is refused rather
 *         than doubling their register.
 *   403 — the caller may not create risks. POST /onboarding/starter-risks
 *         carries `risks:create` like every other risk write, and an invited
 *         member without it must still be able to finish the tunnel: it blocks
 *         the whole app, so refusing to advance over a permission they will
 *         never have would lock them out of the product.
 *
 * Both let the step advance. Neither writes anything.
 */
function isExpectedAdoptionRefusal(err: unknown): boolean {
  const status = (err as { response?: { status?: number } } | null)?.response?.status;
  return status === 409 || status === 403;
}

// ---------------------------------------------------------------------------
// 3. Framework — suggested from sector + country, one-click import
// ---------------------------------------------------------------------------

export function FrameworkStep() {
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const { go, busy, error, retry } = useStepNav('framework');
  const { data: suggestions, isLoading } = useOnboardingSuggestions();
  const { data: catalogs } = useCatalogs();
  const importCatalog = useImportCatalogAsFramework();
  const stored = useStoredAnswers('framework');
  const [imported, setImported] = useState<string[]>([]);

  // Restore what was already imported. Without this, stepping Back and Forward
  // cleared every tick: the catalogues were still imported server-side, but the
  // step claimed they were not and offered to import them a second time.
  useEffect(() => {
    const previous = stored.imported;
    if (Array.isArray(previous)) {
      setImported(previous.filter((k): k is string => typeof k === 'string'));
    }
  }, [stored]);

  // The suggested keys, resolved against the real catalog registry so we never
  // offer something that cannot actually be imported.
  const offered = useMemo(() => {
    const byKey = new Map((catalogs ?? []).map((c) => [c.key, c]));
    return (suggestions?.frameworks ?? [])
      .map((k) => byKey.get(k))
      .filter((c): c is NonNullable<typeof c> => !!c && c.available !== false)
      .slice(0, 5);
  }, [suggestions, catalogs]);

  /**
   * Importing a catalogue is the ONLY thing in the whole tunnel that creates a
   * control, and the reveal this tunnel ends on cannot be computed without one:
   * `coveragePercent` returns nil with zero applicable controls, `IsRevealable`
   * is then false, and GET /posture answers 404. A user who walked five screens
   * landed on "Impossible de calculer votre posture" — the exact activation
   * cliff #438 exists to remove, rebuilt at the last step.
   *
   * The server already defines this step as satisfied by HasFramework — that is
   * what OnboardingStepData.SkipsStep tests to auto-skip it. The client simply
   * did not hold the same line. It does now.
   *
   * Except when the catalogue offers nothing: blocking there would trap the user
   * in a step with no control to press, which is worse than the 404. Then the
   * step stays passable and the reveal's own error state does its job.
   */
  const mustImport = offered.length > 0 && imported.length === 0;

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
      nextLabel={tr('Continuer', 'Continue')}
      nextDisabled={mustImport}
      busy={busy}
      error={error}
      onRetry={retry}
      errorLabel={tr("Impossible d'enregistrer cette étape.", 'This step could not be saved.')}
      errorHint={tr(
        'Vos réponses sont conservées — réessayez, rien n\u2019est perdu.',
        'Your answers are kept — try again, nothing is lost.',
      )}
      retryLabel={tr('Réessayer', 'Try again')}
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

      {/* A disabled button with no explanation is its own defect. This says what
          to press and what it buys, rather than leaving the user to guess. */}
      {mustImport && (
        <div className="text-[12.5px] text-ink-soft mt-4" data-testid="framework-required">
          {tr(
            'Importez-en un pour continuer : vos contrôles sont ce qui rend votre posture calculable.',
            'Import one to continue: your controls are what makes your posture computable.',
          )}
        </div>
      )}
    </StepShell>
  );
}
