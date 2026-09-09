// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// Settings (OpenRisk.dc.html §6.16) — the consolidation point for every admin
// feature. Internal nav + tabs: General, Members (real /users), RBAC, API Tokens
// (real /tokens), Organizations, Audit log, Custom Fields (real /custom-fields),
// Integrations, Notifications, Security, Billing, Danger. Endpoints whose tables
// aren't migrated yet (roles/tenants/audit) degrade to an honest unavailable state.

import { useEffect, useMemo, useState } from 'react';
import { useSearchParams, useNavigate, useLocation } from 'react-router';
import { toast } from 'sonner';
import {
  Settings as SettingsIcon,
  Users,
  KeyRound,
  Building2,
  SlidersHorizontal,
  Plug,
  Siren,
  Shield,
  CreditCard,
  AlertTriangle,
  Plus,
  FileText,
  Check,
  Trash2,
  Copy,
  Database,
  PowerOff,
  type LucideIcon,
} from 'lucide-react';
import {
  PageFrame,
  PageHeader,
  Btn,
  Card,
  SkeletonRows,
  EmptyState,
  ErrorState,
} from '../../shared/ui';
import { useUIStrings } from '../../shared/uiStrings';
import { useUIStore } from '../../store/uiStore';
import { useAuthStore } from '../../hooks/useAuthStore';
import { SessionsPanel } from '../auth/SessionsPanel';
import { MembersView } from '../organization/MembersView';
import { relTime } from '../risks/riskMap';
import { api } from '../../lib/api';
import { useTokens, useCustomFields, useTenants, type ApiToken } from './adminData';
import {
  useNotificationPreferences,
  useUpdateNotificationPreferences,
} from '../notifications/useNotifications';
import type { NotificationPreferencePatch } from '../notifications/notificationService';
import { useChannelConfig } from '../automation/useAutomation';
import { useVulnIntegrations, useVulnTicketing } from '../vulnerabilities/useVulnIntegrations';
import {
  DataTable,
  useTableState,
  type Column,
  type Facet,
  type RowAction,
} from '../../shared/datatable';
import { DangerConfirm } from '../../shared/DangerConfirm';
import { PersonalizeCard } from '../onboarding/PersonalizeCard';
import { BillingPanel } from '../billing/BillingPanel';
import { DangerZonePanel } from '../billing/DangerZonePanel';
import { useOrganization } from '../organization/useOrganization';
import { MFAPolicyPanel, MFAAccountPanel } from './MFAPolicyPanel';
import type { LocaleCode } from '../../i18n/locales';
import { useI18n } from '../../hooks/useI18n';
import { localeTag } from '../../i18n/locales';

type TabKey =
  | 'general'
  | 'members'
  | 'tokens'
  | 'orgs'
  | 'fields'
  | 'integrations'
  | 'notif'
  | 'security'
  | 'billing'
  | 'danger';
type Tr = (fr: string, en: string) => string;

/* ---- reusable bits ----
 *
 * The three components removed here (W0-05 / D12) were `Toggle` (flipped local
 * state and persisted nothing), `ToggleRow` (rendered it), and `Field` (an
 * `<input defaultValue>` with no onChange and no save, so anything typed was
 * discarded on unmount). All three were unreferenced by the time this wave ran,
 * but they are exactly the primitives a hurried change reaches for — a settings
 * screen that looks saved is the easiest lie in the product to write by
 * accident. Deleted rather than left as a template.
 *
 * `SavedToggleRow` (localStorage-backed) is replaced by `ServerToggleRow`, which
 * writes to an API and reports what the server answered.
 */

/**
 * A switch bound to server state.
 *
 * Three properties the localStorage version did not have:
 *
 *  - the position rendered is the SERVER's, not an optimistic guess;
 *  - "Saved ✓" appears when the API confirms, and an error appears when it does
 *    not, instead of a timer that always says success;
 *  - it is disabled while in flight, so a double-click cannot race two patches.
 */
function ServerToggleRow({
  label,
  sub,
  checked,
  onChange,
  busy,
  disabled,
  disabledReason,
  tr,
}: {
  label: string;
  sub?: string | null;
  checked: boolean;
  onChange: (next: boolean) => Promise<unknown>;
  busy?: boolean;
  disabled?: boolean;
  disabledReason?: string;
  tr: Tr;
}) {
  const [state, setState] = useState<'idle' | 'saved' | 'error'>('idle');
  const toggle = async () => {
    setState('idle');
    try {
      await onChange(!checked);
      setState('saved');
      window.setTimeout(() => setState((s) => (s === 'saved' ? 'idle' : s)), 1600);
    } catch {
      // Left visible: a preference that failed to save is exactly the thing the
      // user must not be told is fine.
      setState('error');
    }
  };
  const inert = !!disabled || !!busy;
  return (
    <div
      className="flex items-center justify-between gap-5 py-[15px]"
      style={{ borderBottom: '1px solid var(--border)' }}
    >
      <div className="flex-1">
        <div className="text-[13.5px] font-medium text-ink" style={{ opacity: disabled ? 0.6 : 1 }}>
          {label}
        </div>
        {sub && <div className="text-[12px] text-ink-soft mt-0.5 leading-snug">{sub}</div>}
        {disabled && disabledReason && (
          <div className="text-[12px] mt-0.5 leading-snug" style={{ color: 'var(--medium)' }}>
            {disabledReason}
          </div>
        )}
      </div>
      <div className="flex items-center gap-2.5 shrink-0">
        <span
          className="text-[11.5px] font-semibold transition-opacity"
          role="status"
          style={{
            color: state === 'error' ? 'var(--critical)' : 'var(--low)',
            opacity: state === 'idle' ? 0 : 1,
          }}
        >
          {state === 'error'
            ? tr('Échec — réessayez', 'Failed — retry')
            : tr('Enregistré ✓', 'Saved ✓')}
        </span>
        <button
          onClick={toggle}
          disabled={inert}
          className="relative shrink-0 disabled:opacity-50"
          style={{
            width: 42,
            height: 24,
            borderRadius: 20,
            background: checked ? 'var(--accent)' : 'var(--bg-hover)',
            transition: 'background .2s',
          }}
          aria-pressed={checked}
          aria-label={label}
          data-testid={`pref-toggle-${label.replace(/\s+/g, '-').toLowerCase()}`}
        >
          <span
            className="absolute rounded-full bg-surface-1"
            style={{
              width: 20,
              height: 20,
              top: 2,
              left: checked ? 20 : 2,
              transition: 'left .2s',
              boxShadow: '0 1px 3px rgba(0,0,0,.3)',
            }}
          />
        </button>
      </div>
    </div>
  );
}

/**
 * A statement of behaviour the product enforces and does not let you configure.
 *
 * Replaces two switches (W0-05 / D3) whose labels described real server-side
 * enforcement — evidence required before a control counts as implemented, scores
 * recalculated after every scan — for which no configuration column exists
 * anywhere in the backend. They were on by default, could be switched off, and
 * switching them off changed nothing: the server went on enforcing.
 *
 * A lock and a sentence is the honest rendering. Turning them into real settings
 * would mean letting an administrator disable an evidence requirement, which is
 * a policy decision this wave is not entitled to make.
 */
function EnforcedPolicyRow({ label, sub, tr }: { label: string; sub: string; tr: Tr }) {
  return (
    <div
      className="flex items-center justify-between gap-5 py-[15px]"
      style={{ borderBottom: '1px solid var(--border)' }}
    >
      <div className="flex-1">
        <div className="text-[13.5px] font-medium text-ink">{label}</div>
        <div className="text-[12px] text-ink-soft mt-0.5 leading-snug">{sub}</div>
      </div>
      <span
        className="shrink-0 inline-flex items-center gap-1.5 text-[11px] font-semibold uppercase tracking-[.05em] px-2 py-1 rounded-full"
        style={{ color: 'var(--low)', background: 'color-mix(in srgb,var(--low) 14%,transparent)' }}
      >
        <Shield size={12} /> {tr('Toujours actif', 'Always on')}
      </span>
    </div>
  );
}

const Title = ({ children }: { children: React.ReactNode }) => (
  <div className="text-[14px] font-semibold text-ink mb-3.5">{children}</div>
);

/** Honest state for endpoints whose backing tables aren't provisioned yet. */
function Unavailable({ tr }: { tr: Tr }) {
  return (
    <Card>
      <EmptyState
        icon={Database}
        title={tr('Bientôt disponible', 'Not available yet')}
        description={tr(
          'Ce module nécessite une migration de base de données (tables non provisionnées dans cet environnement).',
          'This module needs a database migration (tables are not provisioned in this environment).',
        )}
      />
    </Card>
  );
}

const TAB_KEYS: TabKey[] = [
  'general',
  'members',
  'tokens',
  'orgs',
  'fields',
  'integrations',
  'notif',
  'security',
  'billing',
  'danger',
];

export function SettingsScreen() {
  const L = useUIStrings();
  const lang = useUIStore((s) => s.lang);
  const tr: Tr = (fr, en) => (lang === 'fr' ? fr : en);
  // Members is a route of its own (/settings/members) because things link TO it
  // — the onboarding CTA, the access-denied screen, /roles. A tab reachable only
  // by query string is not a place you can send someone. Other tabs stay on
  // ?tab= until they earn the same need.
  const navigate = useNavigate();
  const { pathname } = useLocation();
  const [params] = useSearchParams();
  const paramTab = params.get('tab');
  const routeTab: TabKey | null = pathname === '/settings/members' ? 'members' : null;
  const [tab, setTab] = useState<TabKey>(
    routeTab ?? (TAB_KEYS.includes(paramTab as TabKey) ? (paramTab as TabKey) : 'general'),
  );
  useEffect(() => {
    if (routeTab) {
      setTab(routeTab);
      return;
    }
    if (paramTab && TAB_KEYS.includes(paramTab as TabKey)) setTab(paramTab as TabKey);
  }, [paramTab, routeTab]);
  const selectTab = (k: TabKey) => {
    setTab(k);
    if (k === 'members') {
      navigate('/settings/members');
      return;
    }
    // Leaving Members must leave its URL too, or the route would force the tab
    // straight back on the next render.
    const base = pathname === '/settings/members' ? '/settings' : pathname;
    navigate(`${base}?tab=${k}`, { replace: pathname !== '/settings/members' });
  };

  const tabs: [TabKey, string, LucideIcon][] = [
    ['general', L.s_general, SettingsIcon],
    // Members owns invitations AND access. They are one job — "give this person
    // the right access" — and splitting them across two screens is why "Invite a
    // member" used to land on Roles & permissions.
    ['members', L.s_members, Users],
    ['tokens', tr('Jetons API', 'API Tokens'), KeyRound],
    ['orgs', tr('Organisations', 'Organizations'), Building2],
    ['fields', tr('Champs personnalisés', 'Custom fields'), SlidersHorizontal],
    ['integrations', L.s_integrations, Plug],
    ['notif', L.s_notif, Siren],
    ['security', L.s_security, Shield],
    ['billing', L.s_billing, CreditCard],
    ['danger', L.s_danger, AlertTriangle],
  ];

  return (
    <PageFrame>
      <PageHeader title={L.setTitle} />
      <div className="flex gap-7 items-start flex-col md:flex-row">
        <div className="w-full md:w-[210px] shrink-0 flex md:block gap-1 overflow-x-auto">
          {tabs.map(([k, lbl, Icon]) => (
            <button
              key={k}
              data-testid={`settings-tab-${k}`}
              onClick={() => selectTab(k)}
              className="w-full flex items-center gap-2.5 px-3 py-[9px] rounded-[9px] mb-0.5 text-[13px] text-left whitespace-nowrap transition-colors"
              style={{
                background: tab === k ? 'var(--accent-soft)' : 'transparent',
                color: tab === k ? 'var(--fg-primary)' : 'var(--fg-secondary)',
                fontWeight: tab === k ? 600 : 500,
              }}
            >
              <span
                style={{ color: tab === k ? 'var(--accent)' : 'var(--fg-muted)' }}
                className="flex"
              >
                <Icon size={17} />
              </span>
              {lbl}
            </button>
          ))}
        </div>
        <div className="flex-1 min-w-0 w-full">
          {tab === 'general' && <GeneralTab tr={tr} />}
          {tab === 'members' && <MembersView />}
          {tab === 'tokens' && <TokensTab tr={tr} lang={lang} />}
          {tab === 'orgs' && <OrgsTab tr={tr} />}
          {tab === 'fields' && <CustomFieldsTab tr={tr} />}
          {tab === 'integrations' && <IntegrationsTab tr={tr} />}
          {tab === 'notif' && <NotifTab tr={tr} />}
          {tab === 'security' && <SecurityTab tr={tr} />}
          {tab === 'billing' && <BillingTab tr={tr} />}
          {tab === 'danger' && <DangerTab L={L} tr={tr} />}
        </div>
      </div>
    </PageFrame>
  );
}

/* ==================== real tabs ==================== */

// The token prefix cell used to render a Copy glyph that copied nothing. Module
// scope keeps the handler stable so the columns memo survives.
function copyPrefix(prefix: string, tr: Tr) {
  navigator.clipboard
    ?.writeText(prefix)
    .then(() => toast.success(tr('Préfixe copié', 'Prefix copied')))
    .catch(() => toast.error(tr('Copie impossible', 'Could not copy')));
}

function TokensTab({ tr, lang }: { tr: Tr; lang: LocaleCode }) {
  const { tokens, isLoading, isError, refetch, create, revoke } = useTokens();
  const [name, setName] = useState('');
  // Revoking a token breaks any integration using it → impact-radiography confirm.
  const [revokingToken, setRevokingToken] = useState<null | {
    id: string;
    name: string;
    lastUsed?: string | null;
  }>(null);
  const table = useTableState({
    defaultSort: { key: 'created', dir: 'desc' },
    defaultPageSize: 25,
    urlPrefix: 'tok_',
  });

  const doCreate = () => {
    const n = name.trim() || tr('Nouveau jeton', 'New token');
    create.mutate(n, {
      onSuccess: (res) => {
        setName('');
        const secret = res.data?.token;
        if (secret) {
          navigator.clipboard?.writeText(secret).catch(() => {});
          toast.success(
            tr(
              'Jeton créé et copié dans le presse-papiers',
              'Token created and copied to clipboard',
            ),
          );
        } else toast.success(tr('Jeton créé', 'Token created'));
      },
      onError: () => toast.error(tr('Création échouée', 'Creation failed')),
    });
  };

  const facets: Facet<ApiToken>[] = useMemo(
    () => [
      {
        key: 'state',
        label: tr('État', 'State'),
        single: true,
        options: [
          { value: 'active', label: tr('Actifs', 'Active'), color: 'var(--low)' },
          { value: 'revoked', label: tr('Révoqués', 'Revoked'), color: 'var(--critical)' },
        ],
        matches: (t, selected) => (selected.includes('revoked') ? !!t.revoked : !t.revoked),
      },
    ],
    [tr],
  );

  const columns: Column<ApiToken>[] = useMemo(
    () => [
      {
        key: 'name',
        header: tr('Nom', 'Name'),
        frozen: true,
        hideable: false,
        sortValue: (t) => t.name.toLowerCase(),
        exportValue: (t) => t.name,
        render: (t) => (
          <span
            className="text-[13.5px] font-medium text-ink"
            style={{ opacity: t.revoked ? 0.55 : 1 }}
          >
            {t.name}
            {t.revoked ? ` · ${tr('révoqué', 'revoked')}` : ''}
          </span>
        ),
      },
      {
        key: 'prefix',
        header: tr('Préfixe', 'Prefix'),
        exportValue: (t) => t.token_prefix ?? '',
        render: (t) =>
          t.token_prefix ? (
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                copyPrefix(t.token_prefix as string, tr);
              }}
              className="mono text-[12px] text-ink-soft inline-flex items-center gap-1.5 rounded px-1 -mx-1 hover:bg-hover"
              title={tr('Copier le préfixe', 'Copy prefix')}
            >
              {t.token_prefix}… <Copy size={12} className="text-ink-muted" />
            </button>
          ) : (
            <span className="text-ink-muted text-[12px]">—</span>
          ),
      },
      {
        key: 'created',
        header: tr('Créé', 'Created'),
        sortValue: (t) => new Date(t.created_at ?? 0).getTime(),
        exportValue: (t) => t.created_at ?? '',
        render: (t) => (
          <span className="text-[12px] text-ink-soft">{relTime(t.created_at, lang)}</span>
        ),
      },
      {
        key: 'used',
        header: tr('Dernière util.', 'Last used'),
        sortValue: (t) => new Date(t.last_used_at ?? 0).getTime(),
        exportValue: (t) => t.last_used_at ?? '',
        render: (t) => (
          <span className="text-[12px] text-ink-soft">
            {t.last_used_at ? relTime(t.last_used_at, lang) : tr('jamais', 'never')}
          </span>
        ),
      },
    ],
    [tr, lang],
  );

  const rowActions: RowAction<ApiToken>[] = useMemo(
    () => [
      {
        key: 'revoke',
        label: tr('Révoquer', 'Revoke'),
        icon: Trash2,
        danger: true,
        hidden: (t) => !!t.revoked,
        onSelect: (t) => setRevokingToken({ id: t.id, name: t.name, lastUsed: t.last_used_at }),
      },
    ],
    [tr],
  );

  return (
    <>
      <div className="flex items-center gap-2.5 mb-4 flex-wrap">
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder={tr('Nom du jeton (ex. CI/CD)', 'Token name (e.g. CI/CD)')}
          aria-label={tr('Nom du jeton', 'Token name')}
          className="flex-1 min-w-[200px] h-9 px-3.5 rounded-[10px] text-[13px] text-ink outline-none"
          style={{ border: '1px solid var(--border-strong)', background: 'var(--bg-elevated)' }}
        />
        <Btn
          label={tr('Générer un jeton', 'Generate token')}
          icon={Plus}
          primary
          onClick={doCreate}
          disabled={create.isPending}
        />
      </div>

      <DataTable
        id="api-tokens"
        ariaLabel={tr('Jetons API', 'API tokens')}
        rows={tokens}
        columns={columns}
        rowKey={(t) => t.id}
        api={table}
        mode="client"
        loading={isLoading}
        error={isError}
        onRetry={() => void refetch()}
        facets={facets}
        clientSearch={(t, q) => `${t.name} ${t.token_prefix ?? ''}`.toLowerCase().includes(q)}
        searchPlaceholder={tr('Nom ou préfixe…', 'Name or prefix…')}
        rowActions={rowActions}
        exportFilename="jetons-api"
        minWidth={620}
        pageSizeOptions={[10, 25, 50]}
        empty={
          <EmptyState
            icon={KeyRound}
            title={tr('Aucun jeton API', 'No API tokens')}
            description={tr(
              'Créez un jeton pour authentifier vos intégrations et scripts.',
              'Create a token to authenticate your integrations and scripts.',
            )}
          />
        }
      />

      <DangerConfirm
        open={!!revokingToken}
        onClose={() => setRevokingToken(null)}
        title={tr('Révoquer le jeton API', 'Revoke API token')}
        subject={revokingToken?.name}
        intro={tr(
          'Toute intégration ou script utilisant ce jeton cessera immédiatement de fonctionner. Cette action est irréversible.',
          'Any integration or script using this token stops working immediately. This action is irreversible.',
        )}
        impact={
          revokingToken
            ? [
                {
                  label: tr('Dernière utilisation', 'Last used'),
                  value: revokingToken.lastUsed
                    ? relTime(revokingToken.lastUsed, lang)
                    : tr('jamais', 'never'),
                },
              ]
            : []
        }
        confirmLabel={tr('Révoquer le jeton', 'Revoke token')}
        onConfirm={() => {
          if (revokingToken)
            revoke.mutate(revokingToken.id, {
              onSuccess: () => {
                toast.success(tr('Jeton révoqué', 'Token revoked'));
                setRevokingToken(null);
              },
              onError: () =>
                toast.error(tr('Révocation échouée — réessayez.', 'Revocation failed — retry.')),
            });
        }}
        busy={revoke.isPending}
      />
    </>
  );
}

function CustomFieldsTab({ tr }: { tr: Tr }) {
  const { fields, isLoading, isError } = useCustomFields();
  if (isError) return <Unavailable tr={tr} />;
  return (
    <Card style={{ padding: '8px 8px 0', overflow: 'hidden' }}>
      {isLoading ? (
        <SkeletonRows rows={3} />
      ) : fields.length === 0 ? (
        // The primary action here used to be a button whose entire effect was
        // `toast('Field editor — coming soon')`. An empty state whose CTA does
        // nothing is worse than one with no CTA: it reads as "you have not done
        // this yet" when the truth is "you cannot do this here". No editor
        // exists, so the state says so and offers the two things that do work —
        // the documented API, and a way back to a screen that functions
        // (W0-05 / D4).
        <EmptyState
          icon={SlidersHorizontal}
          title={tr('Éditeur de champs indisponible', 'Field editor unavailable')}
          description={tr(
            'Les champs personnalisés se créent aujourd’hui via l’API (POST /custom-fields) ; l’éditeur n’est pas encore disponible dans cet écran. Ceux qui existent déjà sont listés ici.',
            'Custom fields are created through the API today (POST /custom-fields); the in-app editor is not available yet. Any that already exist are listed here.',
          )}
          primaryAction={
            <Btn
              label={tr('Voir la documentation API', 'View the API docs')}
              icon={FileText}
              onClick={() => window.open('/docs/API_REFERENCE.md', '_blank', 'noopener')}
            />
          }
        />
      ) : (
        <div className="p-3 flex flex-col gap-2">
          {fields.map((f) => (
            <div
              key={f.id}
              className="flex items-center gap-3 px-3 py-2.5 rounded-[10px]"
              style={{ border: '1px solid var(--border)' }}
            >
              <SlidersHorizontal size={16} className="text-ink-muted" />
              <div className="flex-1">
                <div className="text-[13.5px] font-medium text-ink">{f.label || f.name}</div>
                <div className="text-[11.5px] text-ink-muted">
                  {f.field_type} · {f.entity_type}
                </div>
              </div>
              {f.required && (
                <span className="text-[11px] font-semibold" style={{ color: 'var(--high)' }}>
                  {tr('requis', 'required')}
                </span>
              )}
            </div>
          ))}
        </div>
      )}
    </Card>
  );
}

function OrgsTab({ tr }: { tr: Tr }) {
  const { tenants, isLoading, isError } = useTenants();
  if (isError) return <Unavailable tr={tr} />;
  return (
    <Card style={{ padding: '8px 8px 0', overflow: 'hidden' }}>
      {isLoading ? (
        <SkeletonRows rows={3} />
      ) : tenants.length === 0 ? (
        <EmptyState icon={Building2} title={tr('Aucune organisation', 'No organizations')} />
      ) : (
        <div className="p-3 flex flex-col gap-2">
          {tenants.map((t, i) => (
            <div
              key={t.id ?? i}
              className="flex items-center gap-3 px-3 py-2.5 rounded-[10px]"
              style={{ border: '1px solid var(--border)' }}
            >
              <div
                className="w-8 h-8 rounded-[9px] flex items-center justify-center text-[11px] font-bold"
                style={{ background: 'var(--accent-soft)', color: 'var(--accent-500)' }}
              >
                {(t.name ?? '?').slice(0, 2).toUpperCase()}
              </div>
              <div className="flex-1">
                <div className="text-[13.5px] font-medium text-ink">{t.name}</div>
                <div className="mono text-[11.5px] text-ink-muted">{t.slug ?? t.id}</div>
              </div>
            </div>
          ))}
        </div>
      )}
    </Card>
  );
}

/* ==================== static tabs ==================== */

function GeneralTab({ tr }: { tr: Tr }) {
  const lang = useUIStore((s) => s.lang);
  const { data: org, isLoading, isError, refetch } = useOrganization();

  if (isLoading) {
    return (
      <Card style={{ padding: '20px 22px' }}>
        <SkeletonRows rows={4} />
      </Card>
    );
  }
  if (isError || !org) {
    return (
      <Card style={{ padding: '20px 22px' }}>
        <ErrorState
          title={tr("Impossible de charger l'organisation", 'Could not load the organization')}
          description={tr(
            'Réessayez, ou contactez un administrateur si le problème persiste.',
            'Retry, or contact an administrator if this persists.',
          )}
          onRetry={() => void refetch()}
        />
      </Card>
    );
  }

  const initials = (org.name || 'OR')
    .split(/\s+/)
    .map((w) => w[0])
    .slice(0, 2)
    .join('')
    .toUpperCase();
  const created = new Date(org.created_at).toLocaleDateString(localeTag(lang), {
    day: 'numeric',
    month: 'long',
    year: 'numeric',
  });

  return (
    <>
      <Card style={{ padding: '20px 22px', marginBottom: 16 }}>
        <Title>{tr('Profil de l’organisation', 'Organization profile')}</Title>
        <div className="flex items-center gap-4 mb-5">
          <div
            className="w-14 h-14 rounded-[14px] flex items-center justify-center text-[20px] font-bold overflow-hidden"
            style={{ background: 'var(--accent-soft)', color: 'var(--accent-500)' }}
          >
            {org.logo_url ? (
              <img src={org.logo_url} alt="" className="w-full h-full object-cover" />
            ) : (
              initials
            )}
          </div>
          <div className="min-w-0">
            <div className="text-[16px] font-bold text-ink truncate">{org.name}</div>
            <div className="mono text-[12px] text-ink-muted">{org.slug}</div>
          </div>
        </div>
        {/* Read-only, and honestly so: the backend serves this profile but has
            no endpoint that writes it. An input that looks editable and saves
            nothing is worse than a value that plainly is not. */}
        <dl
          className="grid gap-x-6 gap-y-[14px]"
          style={{ gridTemplateColumns: 'repeat(auto-fit,minmax(200px,1fr))' }}
        >
          <ReadOnly label={tr('Plan', 'Plan')} value={org.plan} />
          <ReadOnly
            label={tr('Statut', 'Status')}
            value={org.is_active ? tr('Active', 'Active') : tr('Suspendue', 'Suspended')}
          />
          <ReadOnly label={tr('Créée le', 'Created')} value={created} />
          <ReadOnly label={tr('Propriétaire', 'Owner')} value={org.owner_name || '—'} />
          {org.industry && <ReadOnly label={tr('Secteur', 'Industry')} value={org.industry} />}
          <ReadOnly
            label={tr('Fuseau horaire', 'Time zone')}
            value={org.timezone || tr('non défini', 'not set')}
            muted={!org.timezone}
          />
        </dl>
        {org.can_edit && (
          <p className="text-[11.5px] text-ink-muted mt-4 leading-snug">
            {tr(
              "Ces informations sont définies à la création de l'organisation. Leur modification depuis cet écran arrivera dans une prochaine version.",
              'These details are set when the organization is created. Editing them from this screen is coming in a future release.',
            )}
          </p>
        )}
      </Card>

      <Card style={{ padding: '20px 22px', marginBottom: 16 }}>
        <Title>{tr('Membres', 'Members')}</Title>
        <div
          className="grid gap-3"
          style={{ gridTemplateColumns: 'repeat(auto-fit,minmax(120px,1fr))' }}
        >
          <CountTile label={tr('Total', 'Total')} value={org.counts.total_members} />
          <CountTile
            label={tr('Actifs', 'Active')}
            value={org.counts.active_members}
            color="var(--low)"
          />
          <CountTile
            label={tr('Administrateurs', 'Administrators')}
            value={org.counts.admins}
            color="var(--accent)"
          />
          <CountTile
            label={tr('Désactivés', 'Deactivated')}
            value={org.counts.deactivated_members}
            color="var(--high)"
          />
          <CountTile
            label={tr('Invitations', 'Invitations')}
            value={org.counts.pending_invitations}
            color="var(--info)"
          />
        </div>
      </Card>

      <Card style={{ padding: '20px 22px', marginBottom: 16 }}>
        <Title>{tr('Apparence', 'Appearance')}</Title>
        <PersonalizeCard />
      </Card>
      {/* These were switches until W0-05. Both described enforcement the server
          really does, neither had a configuration column behind it, and turning
          one off changed nothing — the guard went on refusing. Stated as policy
          instead of offered as a choice the product cannot honour. */}
      <Card style={{ padding: '20px 22px' }}>
        <Title>{tr('Règles appliquées', 'Enforced policy')}</Title>
        <div className="text-[12.5px] text-ink-soft mb-1 leading-relaxed">
          {tr(
            'Ces règles sont appliquées par le serveur pour toutes les organisations et ne sont pas configurables.',
            'These rules are enforced by the server for every organisation and are not configurable.',
          )}
        </div>
        <EnforcedPolicyRow
          tr={tr}
          label={tr('Preuve obligatoire', 'Evidence required')}
          sub={tr(
            'Un contrôle ne peut pas passer à « Implémenté » sans au moins une preuve attachée.',
            'A control cannot be marked "Implemented" without at least one piece of evidence attached.',
          )}
        />
        <EnforcedPolicyRow
          tr={tr}
          label={tr('Recalcul automatique des scores', 'Automatic score recalculation')}
          sub={tr(
            'Les scores sont recalculés à chaque changement de criticité d’actif et après chaque scan.',
            'Scores are recalculated on every asset-criticality change and after every scan.',
          )}
        />
        <EnforcedPolicyRow
          tr={tr}
          label={tr('Gardes du cycle de vie', 'Lifecycle guards')}
          sub={tr(
            'Un risque ne peut être « Atténué » que si toutes les sous-actions sont terminées, ni « Risque résiduel accepté » sans approbation.',
            'A risk can only reach "Mitigated" once every sub-action is complete, and "Residual accepted" only with an approval.',
          )}
        />
      </Card>
    </>
  );
}

function ReadOnly({ label, value, muted }: { label: string; value: string; muted?: boolean }) {
  return (
    <div>
      <dt className="text-[11.5px] font-semibold text-ink-muted uppercase tracking-wide mb-1">
        {label}
      </dt>
      <dd className="text-[13.5px] text-ink" style={{ opacity: muted ? 0.6 : 1 }}>
        {value}
      </dd>
    </div>
  );
}

function CountTile({ label, value, color }: { label: string; value: number; color?: string }) {
  return (
    <div className="px-3.5 py-3 rounded-[11px]" style={{ border: '1px solid var(--border)' }}>
      <div
        className="text-[22px] font-bold leading-none"
        style={{ color: color ?? 'var(--fg-primary)' }}
      >
        {value}
      </div>
      <div className="text-[11.5px] text-ink-muted mt-1.5">{label}</div>
    </div>
  );
}

/**
 * Integrations (W0-05 / D1).
 *
 * This tab rendered six providers from a literal, with Slack, Microsoft Teams
 * and Splunk shown as ENABLED — on every tenant, in every deployment, from the
 * day it shipped. The switches flipped local state and persisted nothing, so a
 * reload restored the same three "connected" channels.
 *
 * That is the most expensive shape of deceptive UI a security product can have.
 * A user who sees Slack connected stops watching Slack for alerts that are not
 * going there. It is not a cosmetic defect; it is a monitoring gap the interface
 * created and then concealed.
 *
 * There was no need to invent any of it. OpenRisk has three real, tenant-scoped
 * integration surfaces, each with its own configuration screen:
 *
 *   - /automation/channels        — Slack, Teams, outbound webhook, SMS, e-mail
 *   - /vulnerabilities/integrations — scanner sources (Nessus, Qualys, Defender…)
 *   - /vulnerabilities/ticketing  — Jira / ServiceNow
 *
 * This tab now reads all three and reports what is actually configured. It shows
 * a count only when one was measured, and every card routes to the screen that
 * owns the configuration rather than pretending to own it here. No secret is
 * read back: the channel API exposes `has_slack`-style booleans precisely so a
 * UI can say "configured" without ever handling a webhook URL.
 */
function IntegrationsTab({ tr }: { tr: Tr }) {
  const { t } = useI18n();
  const navigate = useNavigate();
  const channels = useChannelConfig();
  const scanners = useVulnIntegrations();
  const ticketing = useVulnTicketing();

  const loading = channels.isLoading || scanners.isLoading || ticketing.isLoading;
  // Each source degrades on its own. One failing endpoint must not blank the
  // other two — and must not be rendered as "nothing connected", which would be
  // the same lie pointing the other way.
  const ch = channels.data;
  const scanList = scanners.data ?? [];
  const tick = ticketing.data;

  type Row = {
    key: string;
    name: string;
    desc: string;
    configured: boolean;
    enabled: boolean;
    unknown: boolean;
    where: string;
  };

  const chRow = (
    key: string,
    name: string,
    desc: string,
    configured: boolean | undefined,
    enabled: boolean | undefined,
  ): Row => ({
    key,
    name,
    desc,
    configured: !!configured,
    enabled: !!enabled,
    unknown: channels.isError || !ch,
    where: '/automation?tab=channels',
  });

  const rows: Row[] = [
    chRow(
      'slack',
      'Slack',
      tr('Alertes d’automatisation et d’incident', 'Automation and incident alerts'),
      ch?.has_slack,
      ch?.slack_enabled,
    ),
    chRow(
      'teams',
      'Microsoft Teams',
      tr('Alertes d’automatisation et d’incident', 'Automation and incident alerts'),
      ch?.has_teams,
      ch?.teams_enabled,
    ),
    chRow(
      'webhook',
      tr('Webhook sortant', 'Outbound webhook'),
      tr('Charge utile signée (HMAC-SHA256)', 'Signed payload (HMAC-SHA256)'),
      ch?.has_webhook,
      ch?.webhook_enabled,
    ),
    chRow(
      'sms',
      'SMS',
      tr('Passerelle HTTP générique', 'Generic HTTP gateway'),
      ch?.has_sms,
      ch?.sms_enabled,
    ),
    {
      key: 'ticketing',
      name: tick?.provider === 'servicenow' ? 'ServiceNow' : 'Jira',
      desc: tr('Ouvrir un ticket depuis une vulnérabilité', 'Open a ticket from a vulnerability'),
      configured: !!tick?.has_credentials,
      enabled: !!tick?.enabled,
      unknown: ticketing.isError,
      where: '/vulnerabilities',
    },
  ];

  const scannersConfigured = scanList.filter((s) => s.has_credentials || s.webhook_enabled).length;

  return (
    <>
      <div className="text-[13px] text-ink-soft mb-4 leading-relaxed">
        {tr(
          'L’état ci-dessous est lu depuis votre configuration réelle. Chaque intégration se configure sur l’écran qui la possède.',
          'The state below is read from your real configuration. Each integration is configured on the screen that owns it.',
        )}
      </div>

      {loading ? (
        <Card style={{ padding: '20px 22px' }}>
          <SkeletonRows rows={4} />
        </Card>
      ) : (
        <div
          className="grid gap-4"
          style={{ gridTemplateColumns: 'repeat(auto-fill,minmax(280px,1fr))' }}
        >
          {rows.map((r) => (
            <Card key={r.key} style={{ padding: 18 }}>
              <div className="flex items-center gap-3 mb-3">
                <div
                  className="w-10 h-10 rounded-[11px] flex items-center justify-center"
                  style={{
                    background: r.configured
                      ? 'color-mix(in srgb,var(--low) 16%,transparent)'
                      : 'var(--bg-hover)',
                    color: r.configured ? 'var(--low)' : 'var(--fg-muted)',
                  }}
                >
                  <Plug size={20} />
                </div>
                <div className="flex-1 text-[14px] font-semibold text-ink">{r.name}</div>
                <IntegrationState row={r} tr={tr} />
              </div>
              <div className="text-[12.5px] text-ink-soft leading-snug mb-3.5">{r.desc}</div>
              <button
                onClick={() => navigate(r.where)}
                className="w-full h-9 rounded-[10px] text-[12.5px] font-semibold text-ink inline-flex items-center justify-center gap-1.5 hover:bg-hover transition-colors"
                style={{ border: '1px solid var(--border-strong)' }}
              >
                {r.configured ? tr('Gérer', 'Manage') : tr('Configurer', 'Configure')}
              </button>
            </Card>
          ))}

          {/* Scanner sources are a list, not a fixed set, so this card reports a
              measured count and routes to the register that owns them. */}
          <Card style={{ padding: 18 }}>
            <div className="flex items-center gap-3 mb-3">
              <div
                className="w-10 h-10 rounded-[11px] flex items-center justify-center"
                style={{
                  background:
                    scannersConfigured > 0
                      ? 'color-mix(in srgb,var(--low) 16%,transparent)'
                      : 'var(--bg-hover)',
                  color: scannersConfigured > 0 ? 'var(--low)' : 'var(--fg-muted)',
                }}
              >
                <Plug size={20} />
              </div>
              <div className="flex-1 text-[14px] font-semibold text-ink">
                {tr('Sources de vulnérabilités', 'Vulnerability sources')}
              </div>
            </div>
            <div className="text-[12.5px] text-ink-soft leading-snug mb-3.5">
              {scanners.isError
                ? tr('État indisponible.', 'State unavailable.')
                : scannersConfigured > 0
                  ? `${t('common.sources', { count: scannersConfigured })} (Nessus, Qualys, Defender…).`
                  : tr(
                      'Aucune source connectée. Connectez un scanner pour ingérer des vulnérabilités.',
                      'No source connected. Connect a scanner to start ingesting vulnerabilities.',
                    )}
            </div>
            <button
              onClick={() => navigate('/vulnerabilities')}
              className="w-full h-9 rounded-[10px] text-[12.5px] font-semibold text-ink inline-flex items-center justify-center gap-1.5 hover:bg-hover transition-colors"
              style={{ border: '1px solid var(--border-strong)' }}
            >
              {scannersConfigured > 0
                ? tr('Gérer', 'Manage')
                : tr('Connecter une source', 'Connect a source')}
            </button>
          </Card>
        </div>
      )}
    </>
  );
}

/**
 * The connection state of one integration.
 *
 * Three states, and the third is the one that matters: "unknown" is rendered
 * when the endpoint that would answer failed. Falling back to "not connected"
 * would be a guess dressed as a fact — and in this tab, a wrong guess in either
 * direction is the bug being fixed.
 */
function IntegrationState({
  row,
  tr,
}: {
  row: { configured: boolean; enabled: boolean; unknown: boolean };
  tr: Tr;
}) {
  if (row.unknown) {
    return (
      <span
        className="text-[11px] font-semibold px-2 py-1 rounded-full"
        style={{
          color: 'var(--medium)',
          background: 'color-mix(in srgb,var(--medium) 14%,transparent)',
        }}
      >
        {tr('État inconnu', 'State unknown')}
      </span>
    );
  }
  if (!row.configured) {
    return (
      <span
        className="text-[11px] font-semibold px-2 py-1 rounded-full"
        style={{ color: 'var(--fg-muted)', background: 'var(--bg-hover)' }}
      >
        {tr('Non connecté', 'Not connected')}
      </span>
    );
  }
  // Configured but switched off is a real and useful distinction: the
  // credentials are there, the channel is deliberately silent.
  if (!row.enabled) {
    return (
      <span
        className="text-[11px] font-semibold px-2 py-1 rounded-full"
        style={{
          color: 'var(--medium)',
          background: 'color-mix(in srgb,var(--medium) 14%,transparent)',
        }}
      >
        {tr('Configuré · en pause', 'Configured · paused')}
      </span>
    );
  }
  return (
    <span
      className="text-[11px] font-semibold px-2 py-1 rounded-full inline-flex items-center gap-1"
      style={{ color: 'var(--low)', background: 'color-mix(in srgb,var(--low) 14%,transparent)' }}
    >
      <Check size={11} /> {tr('Actif', 'Active')}
    </span>
  );
}

/**
 * Notification preferences (W0-05 / D2).
 *
 * Every switch on this screen used to write to localStorage. Three consequences:
 * nothing changed what was delivered; the settings followed the BROWSER rather
 * than the person, so they carried to the next user to sign in; and two of the
 * five "notify me when" rows described events the product does not emit at all
 * (a score rising by 10%, a weekly digest).
 *
 * It now reads and writes GET/PATCH /notifications/preferences — per user, per
 * tenant — and the backend consults that row before it sends
 * (domain.NotificationPreference.Allows). Only switches the server honours are
 * offered; the rows for events that do not exist are gone rather than restyled.
 */
function NotifTab({ tr }: { tr: Tr }) {
  const user = useAuthStore((s) => s.user);
  const { prefs, isLoading, isError, refetch } = useNotificationPreferences();
  const update = useUpdateNotificationPreferences();

  // No default object while loading: rendering a switch position the server has
  // not confirmed is the same lie in a smaller frame.
  if (isLoading)
    return (
      <Card style={{ padding: '20px 22px' }}>
        <SkeletonRows rows={5} />
      </Card>
    );
  if (isError || !prefs) {
    return (
      <Card style={{ padding: '20px 22px' }}>
        <ErrorState
          title={tr('Préférences indisponibles', 'Preferences unavailable')}
          description={tr(
            'Impossible de lire vos préférences de notification. Réessayez ; si le problème persiste, contactez un administrateur.',
            'Could not read your notification preferences. Retry; if it persists, contact an administrator.',
          )}
          onRetry={() => void refetch()}
        />
      </Card>
    );
  }

  const set = (patch: NotificationPreferencePatch) => update.mutateAsync(patch);
  const muted = prefs.disable_all_notifications;

  return (
    <>
      <Card style={{ padding: '20px 22px', marginBottom: 16 }}>
        <Title>{tr('Tout couper', 'Mute everything')}</Title>
        <ServerToggleRow
          tr={tr}
          label={tr('Suspendre toutes les notifications', 'Pause all notifications')}
          sub={tr(
            'Coupe la cloche et les e-mails, quels que soient les réglages ci-dessous.',
            'Silences the bell and e-mail, whatever the switches below say.',
          )}
          checked={muted}
          busy={update.isPending}
          onChange={(next) => set({ disable_all_notifications: next })}
        />
      </Card>

      <Card style={{ padding: '20px 22px', marginBottom: 16 }}>
        <Title>{tr('E-mail', 'E-mail')}</Title>
        <div className="text-[12.5px] text-ink-soft mb-1">
          {user?.email || tr('Adresse du compte', 'Account address')}
        </div>
        {(
          [
            [
              'email_on_critical_risk',
              tr('Un risque critique est signalé', 'A critical risk is raised'),
              tr('Inclut les escalades de SLA.', 'Includes SLA escalations.'),
            ],
            [
              'email_on_mitigation_deadline',
              tr('Une échéance de mitigation approche', 'A mitigation deadline approaches'),
              tr('Rappels à J-7 et J-1.', 'Reminders at D-7 and D-1.'),
            ],
            [
              'email_on_action_assigned',
              tr('Une action m’est assignée', 'An action is assigned to me'),
              null,
            ],
            [
              'email_on_risk_update',
              tr('Un risque que je suis est modifié', 'A risk I follow is updated'),
              null,
            ],
            ['email_on_risk_resolved', tr('Un risque est résolu', 'A risk is resolved'), null],
          ] as const
        ).map(([key, label, sub]) => (
          <ServerToggleRow
            key={key}
            tr={tr}
            label={label}
            sub={sub}
            checked={prefs[key]}
            busy={update.isPending}
            disabled={muted}
            disabledReason={tr(
              'Toutes les notifications sont suspendues.',
              'All notifications are paused.',
            )}
            onChange={(next) => set({ [key]: next })}
          />
        ))}
      </Card>

      <Card style={{ padding: '20px 22px' }}>
        <Title>{tr('Dans l’application', 'In-app')}</Title>
        {/* The stored row has per-event columns for e-mail, Slack and webhook,
            but only a global switch for in-app. Offering per-event in-app rows
            would mean showing switches the server cannot honour, so this says
            what it actually controls. */}
        <div className="text-[12.5px] text-ink-soft leading-relaxed">
          {muted
            ? tr(
                'La cloche est suspendue. Réactivez les notifications ci-dessus pour la recevoir à nouveau.',
                'The bell is paused. Re-enable notifications above to start receiving it again.',
              )
            : tr(
                'La cloche reçoit tous les événements de votre organisation qui vous concernent. Le filtrage par type d’événement n’est disponible que pour l’e-mail pour le moment.',
                'The bell receives every event in your organisation that concerns you. Filtering by event type is available for e-mail only for now.',
              )}
        </div>
        <div className="mt-3.5">
          <ServerToggleRow
            tr={tr}
            label={tr('Sons de notification', 'Notification sounds')}
            checked={prefs.enable_sound_notifications}
            busy={update.isPending}
            disabled={muted}
            disabledReason={tr(
              'Toutes les notifications sont suspendues.',
              'All notifications are paused.',
            )}
            onChange={(next) => set({ enable_sound_notifications: next })}
          />
          <ServerToggleRow
            tr={tr}
            label={tr('Notifications du navigateur', 'Desktop notifications')}
            checked={prefs.enable_desktop_notifications}
            busy={update.isPending}
            disabled={muted}
            disabledReason={tr(
              'Toutes les notifications sont suspendues.',
              'All notifications are paused.',
            )}
            onChange={(next) => set({ enable_desktop_notifications: next })}
          />
        </div>
      </Card>
    </>
  );
}

function SecurityTab({ tr }: { tr: Tr }) {
  return (
    <>
      {/* OR26-03 — this card used to say organization-wide MFA policy was
          "coming soon". Both halves are real now: your own enrolment state with
          the button that turns it on, and the grace window an administrator
          sets for privileged accounts. */}
      <MFAAccountPanel />
      <MFAPolicyPanel />
      {/* Real device list, backed by GET/DELETE /auth/sessions. This card used to
          show only the current device and a "multi-device history will arrive"
          note; the sessions API now exists, so it lists every signed-in device
          and can revoke them. */}
      <Card style={{ padding: '20px 22px' }}>
        <SessionsPanel />
      </Card>
    </>
  );
}

function BillingTab({ tr: _tr }: { tr: Tr }) {
  return <BillingPanel />;
}

// The danger zone shipped a "Delete organization" button wired to nothing: the
// single most destructive control in the product was a <button> with no onClick.
// It now calls DELETE /rbac/tenants/:id for real, behind an impact radiography,
// and signs the user out afterwards — there is nothing left to be signed in to.
function DangerTab({ L, tr: _tr }: { L: ReturnType<typeof useUIStrings>; tr: Tr }) {
  return (
    <div>
      <div className="text-[15px] font-semibold mb-1.5" style={{ color: 'var(--critical)' }}>
        {L.s_danger}
      </div>
      <div className="text-[13px] text-ink-soft mb-[18px]">
        {_tr(
          'Ces actions sont irréversibles et suivent un délai de grâce de 30 jours.',
          'These actions are irreversible and follow a 30-day grace period.',
        )}
      </div>
      <DangerZonePanel />
    </div>
  );
}
