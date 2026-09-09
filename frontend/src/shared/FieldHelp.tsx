// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// Contextual help for the three fields a newcomer cannot answer honestly without
// a shared reference: probability, impact, asset criticality (spec §6).
//
// Why a dedicated component rather than the existing InfoHint: a one-line tooltip
// is enough for a label ("what is ALE?"), but these three fields ask for a NUMBER.
// "Impact: 7" means nothing until someone tells you what 7 is. So this shows a
// definition in one sentence, then the 1–5 scale with concrete wording, then an
// example from the user's own sector, then a link to the documentation.
//
// Accessible on hover AND on keyboard focus, and dismissible with Escape — help
// that only appears on hover is help that does not exist for keyboard users.

import { useEffect, useId, useRef, useState } from 'react';
import { HelpCircle, ExternalLink } from 'lucide-react';
import type { LocaleCode } from '../i18n/locales';

export type HelpField = 'probability' | 'impact' | 'asset_criticality';

interface ScaleRow {
  level: number;
  fr: string;
  en: string;
  /** The value this level maps to on the field's real scale. */
  value: string;
}

interface FieldHelpCopy {
  titleFr: string;
  titleEn: string;
  defFr: string;
  defEn: string;
  scale: ScaleRow[];
  /** Sector-specific illustrations, keyed by sector; "" is the generic default. */
  examples: Record<string, { fr: string; en: string }>;
  doc: string;
}

const COPY: Record<HelpField, FieldHelpCopy> = {
  probability: {
    titleFr: 'Probabilité',
    titleEn: 'Probability',
    defFr: 'La chance que ce risque se réalise au moins une fois dans les douze prochains mois.',
    defEn: 'The chance this risk materialises at least once in the next twelve months.',
    scale: [
      {
        level: 1,
        fr: 'Très improbable — jamais vu dans le secteur',
        en: 'Very unlikely — never seen in the sector',
        value: '0.1',
      },
      {
        level: 2,
        fr: 'Improbable — arrivé ailleurs, jamais chez vous',
        en: 'Unlikely — happened elsewhere, never here',
        value: '0.3',
      },
      {
        level: 3,
        fr: 'Possible — une fois tous les deux ou trois ans',
        en: 'Possible — once every two or three years',
        value: '0.5',
      },
      {
        level: 4,
        fr: 'Probable — au moins une fois par an',
        en: 'Likely — at least once a year',
        value: '0.7',
      },
      {
        level: 5,
        fr: 'Quasi certain — plusieurs fois par an',
        en: 'Almost certain — several times a year',
        value: '0.9',
      },
    ],
    examples: {
      '': {
        fr: 'Hameçonnage réussi sur un poste : la plupart des organisations en constatent au moins un par an → 4 (0,7).',
        en: 'A successful phishing click: most organisations see at least one a year → 4 (0.7).',
      },
      banking: {
        fr: 'Tentative de fraude au virement : constatée plusieurs fois par an dans la plupart des banques → 5 (0,9).',
        en: 'Attempted payment fraud: seen several times a year in most banks → 5 (0.9).',
      },
      health: {
        fr: 'Rançongiciel bloquant les soins : rare mais documenté chaque année dans le secteur → 3 (0,5).',
        en: 'Ransomware halting care: rare but documented yearly in the sector → 3 (0.5).',
      },
      tech: {
        fr: 'Dépendance logicielle compromise : plusieurs incidents publics par an → 4 (0,7).',
        en: 'A compromised software dependency: several public incidents a year → 4 (0.7).',
      },
    },
    doc: 'https://github.com/opendefender/OpenRisk/blob/master/docs/ENDPOINTS.md',
  },

  impact: {
    titleFr: 'Impact',
    titleEn: 'Impact',
    defFr:
      'La gravité des conséquences si le risque se réalise : perte financière, interruption, sanction, réputation.',
    defEn:
      'How severe the consequences are if it happens: financial loss, outage, penalty, reputation.',
    scale: [
      {
        level: 1,
        fr: 'Négligeable — géré dans la journée, sans coût notable',
        en: 'Negligible — handled same day, no notable cost',
        value: '2',
      },
      {
        level: 2,
        fr: 'Mineur — gêne interne, quelques heures perdues',
        en: 'Minor — internal disruption, a few hours lost',
        value: '4',
      },
      {
        level: 3,
        fr: 'Modéré — clients affectés, coût mesurable',
        en: 'Moderate — customers affected, measurable cost',
        value: '6',
      },
      {
        level: 4,
        fr: 'Majeur — service interrompu, régulateur à informer',
        en: 'Major — service down, regulator to notify',
        value: '8',
      },
      {
        level: 5,
        fr: 'Critique — survie de l’activité, sanction lourde',
        en: 'Critical — business survival, heavy sanction',
        value: '10',
      },
    ],
    examples: {
      '': {
        fr: 'Fuite de données clients : notification obligatoire, sanction possible, confiance entamée → 5 (10).',
        en: 'A customer data leak: mandatory notification, possible fine, damaged trust → 5 (10).',
      },
      banking: {
        fr: 'Core banking indisponible une journée : opérations bloquées et déclaration au superviseur → 5 (10).',
        en: 'Core banking down for a day: operations blocked, supervisor notified → 5 (10).',
      },
      retail: {
        fr: "Tunnel de paiement interrompu deux heures un samedi : chiffre d'affaires perdu, pas de sanction → 3 (6).",
        en: 'Checkout down two hours on a Saturday: revenue lost, no sanction → 3 (6).',
      },
      industry: {
        fr: 'Ligne de production arrêtée une équipe : retard de livraison et pénalités clients → 4 (8).',
        en: 'One shift of production stopped: delivery delays and customer penalties → 4 (8).',
      },
    },
    doc: 'https://github.com/opendefender/OpenRisk/blob/master/docs/ENDPOINTS.md',
  },

  asset_criticality: {
    titleFr: 'Criticité de l’actif',
    titleEn: 'Asset criticality',
    defFr:
      "L'importance de l'actif touché pour votre activité. Elle multiplie le score : le même incident sur un serveur de test et sur la production n'est pas le même risque.",
    defEn:
      'How important the affected asset is to the business. It multiplies the score: the same incident on a test server and on production is not the same risk.',
    scale: [
      {
        level: 1,
        fr: 'Faible — bac à sable, aucune donnée réelle',
        en: 'Low — sandbox, no real data',
        value: '0.5',
      },
      {
        level: 2,
        fr: 'Moyenne — outil interne, contournable',
        en: 'Medium — internal tool, workaround exists',
        value: '1.0',
      },
      {
        level: 3,
        fr: 'Élevée — utilisé quotidiennement par une équipe',
        en: 'High — used daily by a team',
        value: '1.5',
      },
      {
        level: 4,
        fr: 'Très élevée — expose des données clients',
        en: 'Very high — exposes customer data',
        value: '2.0',
      },
      {
        level: 5,
        fr: 'Critique — l’activité s’arrête sans lui',
        en: 'Critical — the business stops without it',
        value: '3.0',
      },
    ],
    examples: {
      '': {
        fr: "Serveur d'authentification : tout le reste en dépend → 5 (3,0).",
        en: 'The authentication server: everything else depends on it → 5 (3.0).',
      },
      banking: {
        fr: 'Système de compensation : indisponible, plus aucune transaction ne passe → 5 (3,0).',
        en: 'The clearing system: if it is down, no transaction settles → 5 (3.0).',
      },
      health: {
        fr: 'Dossier patient informatisé : indispensable à la prise en charge → 5 (3,0).',
        en: 'The electronic patient record: indispensable to care → 5 (3.0).',
      },
      public: {
        fr: "Portail de téléservices : les usagers n'ont pas d'alternative → 4 (2,0).",
        en: 'The citizen services portal: users have no alternative → 4 (2.0).',
      },
    },
    doc: 'https://github.com/opendefender/OpenRisk/blob/master/docs/ENDPOINTS.md',
  },
};

export function FieldHelp({
  field,
  lang = 'fr',
  sector = '',
  className = '',
}: {
  field: HelpField;
  lang?: LocaleCode;
  /** The tenant's sector, so the example is one they recognise. */
  sector?: string;
  className?: string;
}) {
  const [open, setOpen] = useState(false);
  const id = useId();
  const wrapRef = useRef<HTMLSpanElement>(null);
  const copy = COPY[field];

  // Escape closes; a help popover that traps you is worse than no help.
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setOpen(false);
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [open]);

  const example = copy.examples[sector] ?? copy.examples[''];
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);

  return (
    <span
      ref={wrapRef}
      className={`relative inline-flex items-center ${className}`}
      onMouseEnter={() => setOpen(true)}
      onMouseLeave={() => setOpen(false)}
    >
      <button
        type="button"
        aria-describedby={open ? id : undefined}
        aria-expanded={open}
        aria-label={tr(`Aide : ${copy.titleFr}`, `Help: ${copy.titleEn}`)}
        data-testid={`field-help-${field}`}
        onClick={(e) => {
          e.stopPropagation();
          e.preventDefault();
          setOpen((o) => !o);
        }}
        // Keyboard parity with hover — this is the whole point of the component.
        onFocus={() => setOpen(true)}
        onBlur={(e) => {
          if (!wrapRef.current?.contains(e.relatedTarget as Node)) setOpen(false);
        }}
        className="inline-flex text-ink-muted hover:text-ink transition-colors"
      >
        <HelpCircle size={13} />
      </button>

      {open && (
        <span
          id={id}
          role="tooltip"
          className="absolute left-0 top-[calc(100%+8px)] z-50 w-[320px] max-w-[80vw] rounded-[12px] p-3.5 text-left normal-case tracking-normal"
          style={{
            background: 'var(--bg-elevated)',
            border: '1px solid var(--border-strong)',
            boxShadow: '0 12px 32px rgba(0,0,0,.22)',
          }}
        >
          <span className="block text-[12.5px] font-bold text-ink mb-1">
            {tr(copy.titleFr, copy.titleEn)}
          </span>
          <span className="block text-[12px] text-ink-soft leading-snug mb-2.5">
            {tr(copy.defFr, copy.defEn)}
          </span>

          <span className="block text-[10.5px] font-bold uppercase tracking-wide text-ink-muted mb-1">
            {tr('Échelle', 'Scale')}
          </span>
          <span className="block mb-2.5">
            {copy.scale.map((row) => (
              <span key={row.level} className="flex items-start gap-2 text-[11.5px] mb-0.5">
                <span
                  className="mono shrink-0 font-bold"
                  style={{ color: 'var(--accent-500)', minWidth: 44 }}
                >
                  {row.level} · {row.value}
                </span>
                <span className="text-ink-soft leading-snug">{tr(row.fr, row.en)}</span>
              </span>
            ))}
          </span>

          <span
            className="block rounded-[8px] p-2 text-[11.5px] text-ink-soft leading-snug mb-2"
            style={{ background: 'var(--bg-hover)' }}
          >
            <span className="font-semibold text-ink">{tr('Exemple', 'Example')} — </span>
            {tr(example.fr, example.en)}
          </span>

          <a
            href={copy.doc}
            target="_blank"
            rel="noreferrer"
            className="inline-flex items-center gap-1 text-[11.5px] font-semibold"
            style={{ color: 'var(--accent-500)' }}
          >
            {tr('Documentation', 'Documentation')}
            <ExternalLink size={11} />
          </a>
        </span>
      )}
    </span>
  );
}
