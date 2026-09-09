// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// Members & access — Settings › Members (W0-04).
//
// Three tabs over one job, "who has access here": the roster, the outstanding
// invitations, and the history of every change to both.
//
// Every control on this screen is wired to a real endpoint, shows its own
// in-flight state, and reflects the server's answer — including the server's
// answer about whether the caller may act at all. Nothing renders as
// functional that is not: an action the API would refuse is disabled here for
// the same reason, read from the same fields.

import { localeTag } from '../../i18n/locales';
import { useMemo, useState } from 'react';
import { useSearchParams } from 'react-router';
import { toast } from 'sonner';
import {
  Users,
  UserPlus,
  Mail,
  History,
  ShieldCheck,
  X,
  Copy,
  RefreshCw,
  Ban,
  Trash2,
  CheckCircle2,
  Clock,
  AlertTriangle,
  Search,
} from 'lucide-react';
import { Card, Chip, SkeletonRows, ErrorState } from '../../shared/ui';
import { EmptyState } from '../../shared/EmptyState';
import { DangerConfirm } from '../../shared/DangerConfirm';
import { useUIStore } from '../../store/uiStore';
import { usePermissions } from '../../hooks/usePermissions';
import { useAuthStore } from '../../hooks/useAuthStore';
import { useEscapeToClose } from '../../shared/useBackTo';
import { useRbacCatalog } from '../rbac/useRbac';
import type { LocaleCode } from '../../i18n/locales';
import {
  useMembers,
  useInvitations,
  useMembershipAudit,
  useInviteMember,
  useResendInvitation,
  useRevokeInvitation,
  useSetMemberRole,
  useSetMemberStatus,
} from './useOrganization';
import type {
  MemberView,
  InvitationView,
  MemberRole,
  MembershipStatus,
  InviteResult,
} from './organizationService';

type Tab = 'members' | 'invitations' | 'history';
type Tr = (fr: string, en: string) => string;

/* --------------------------------------------------------------- presentation */

const STATUS_STYLE: Record<MembershipStatus, { color: string; fr: string; en: string }> = {
  invited: { color: 'var(--info)', fr: 'Invité', en: 'Invited' },
  active: { color: 'var(--low)', fr: 'Actif', en: 'Active' },
  deactivated: { color: 'var(--high)', fr: 'Désactivé', en: 'Deactivated' },
  revoked: { color: 'var(--critical)', fr: 'Révoqué', en: 'Revoked' },
};

const INVITE_STATUS_STYLE: Record<string, { color: string; fr: string; en: string }> = {
  pending: { color: 'var(--info)', fr: 'En attente', en: 'Pending' },
  accepted: { color: 'var(--low)', fr: 'Acceptée', en: 'Accepted' },
  expired: { color: 'var(--fg-muted)', fr: 'Expirée', en: 'Expired' },
  revoked: { color: 'var(--critical)', fr: 'Révoquée', en: 'Revoked' },
};

const ADMIN_OPTION = '__admin__';

function fmtDate(iso: string | undefined, lang: LocaleCode): string {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '—';
  return d.toLocaleDateString(localeTag(lang), {
    day: 'numeric',
    month: 'short',
    year: 'numeric',
  });
}

function TabBtn({
  id,
  label,
  icon: Icon,
  tab,
  count,
  onSelect,
}: {
  id: Tab;
  label: string;
  icon: typeof Users;
  tab: Tab;
  count?: number;
  onSelect: (t: Tab) => void;
}) {
  const active = tab === id;
  return (
    <button
      onClick={() => onSelect(id)}
      data-testid={`members-tab-${id}`}
      aria-current={active ? 'page' : undefined}
      className="h-9 px-3.5 rounded-[9px] text-[12.5px] font-semibold inline-flex items-center gap-1.5"
      style={{
        background: active ? 'var(--accent)' : 'transparent',
        color: active ? 'var(--fg-on-solid)' : 'var(--fg-secondary)',
        border: active ? 'none' : '1px solid var(--border-strong)',
      }}
    >
      <Icon size={14} /> {label}
      {count !== undefined && count > 0 && (
        <span
          className="ml-0.5 px-1.5 rounded-full text-[10.5px] font-bold"
          style={{
            background: active
              ? 'color-mix(in srgb, var(--fg-on-solid) 22%, transparent)'
              : 'var(--bg-hover)',
            color: active ? 'var(--fg-on-solid)' : 'var(--fg-secondary)',
          }}
        >
          {count}
        </span>
      )}
    </button>
  );
}

/* ------------------------------------------------------------------- container */

export function MembersView() {
  const lang = useUIStore((s) => s.lang);
  const tr: Tr = (fr, en) => (lang === 'fr' ? fr : en);
  const { can } = usePermissions();
  const canRead = can('organization:members:read');
  const canInvite = can('organization:members:invite');
  const canAudit = can('organization:audit:read');

  const [tab, setTab] = useState<Tab>('members');
  // ?action=invite opens the dialog on arrival, so anything wanting to send
  // someone here to invite a colleague can link straight to the action. Derived
  // from the URL rather than mirrored into state, so the two cannot disagree.
  const [params, setParams] = useSearchParams();
  const [manualInvite, setManualInvite] = useState(false);
  const inviteOpen = manualInvite || params.get('action') === 'invite';
  const closeInvite = () => {
    setManualInvite(false);
    if (params.get('action')) {
      setParams(
        (prev) => {
          const n = new URLSearchParams(prev);
          n.delete('action');
          return n;
        },
        { replace: true },
      );
    }
  };

  const invitations = useInvitations();
  const pendingCount = useMemo(
    () => (invitations.data?.items ?? []).filter((i) => i.status === 'pending').length,
    [invitations.data],
  );

  // The permission-denied state. The API refuses this member; saying so beats
  // rendering an empty table that looks like an organization with nobody in it.
  if (!canRead) {
    return (
      <Card>
        <EmptyState
          icon={ShieldCheck}
          title={tr('Accès non autorisé', 'You do not have access')}
          description={tr(
            "La gestion des membres est réservée aux administrateurs de l'organisation. Demandez à un administrateur de vous accorder l'accès.",
            'Member management is reserved for organization administrators. Ask an administrator to grant you access.',
          )}
        />
      </Card>
    );
  }

  return (
    <>
      <div className="flex gap-2 mb-4 flex-wrap items-center">
        <TabBtn
          id="members"
          label={tr('Membres', 'Members')}
          icon={Users}
          tab={tab}
          onSelect={setTab}
        />
        <TabBtn
          id="invitations"
          label={tr('Invitations', 'Invitations')}
          icon={Mail}
          tab={tab}
          count={pendingCount}
          onSelect={setTab}
        />
        {canAudit && (
          <TabBtn
            id="history"
            label={tr("Journal d'accès", 'Access history')}
            icon={History}
            tab={tab}
            onSelect={setTab}
          />
        )}
        <div className="flex-1" />
        {canInvite && (
          <button
            onClick={() => setManualInvite(true)}
            data-testid="invite-member"
            className="h-9 px-3.5 rounded-[9px] text-[12.5px] font-semibold inline-flex items-center gap-1.5"
            style={{ background: 'var(--accent-solid)', color: 'var(--fg-on-solid)' }}
          >
            <UserPlus size={15} /> {tr('Inviter un membre', 'Invite a member')}
          </button>
        )}
      </div>

      {tab === 'members' && <MembersTable tr={tr} lang={lang} />}
      {tab === 'invitations' && <InvitationsTable tr={tr} lang={lang} />}
      {tab === 'history' && canAudit && <AccessHistory tr={tr} lang={lang} />}

      {inviteOpen && <InviteDialog tr={tr} onClose={closeInvite} />}
    </>
  );
}

/* ------------------------------------------------------------------- members */

function MembersTable({ tr, lang }: { tr: Tr; lang: LocaleCode }) {
  const { can } = usePermissions();
  const canUpdate = can('organization:members:update');
  const canDeactivate = can('organization:members:deactivate');
  const me = useAuthStore((s) => s.user);

  const [search, setSearch] = useState('');
  const [status, setStatus] = useState<MembershipStatus | ''>('');
  const query = useMemo(() => ({ q: search.trim(), status, limit: 100 }), [search, status]);
  const { data, isLoading, isError, refetch, isFetching } = useMembers(query);
  const { data: catalog } = useRbacCatalog();
  const setRole = useSetMemberRole();
  const setStatusM = useSetMemberStatus();

  const [confirm, setConfirm] = useState<null | { member: MemberView; next: MembershipStatus }>(
    null,
  );
  const [busyId, setBusyId] = useState<string | null>(null);

  const roleLabel = (key: string): string => {
    const r = catalog?.business_roles.find((b) => b.key === key);
    if (!r) return key;
    return lang === 'fr' ? r.label_fr : r.label_en;
  };

  const changeRole = (m: MemberView, value: string) => {
    setBusyId(m.member_id);
    const input =
      value === ADMIN_OPTION
        ? { memberId: m.member_id, role: 'admin' as MemberRole, businessRole: '' }
        : { memberId: m.member_id, role: 'user' as MemberRole, businessRole: value };
    setRole.mutate(input, {
      onSuccess: () => toast.success(tr('Rôle mis à jour', 'Role updated')),
      onError: (err) =>
        toast.error(apiMessage(err, tr('La mise à jour a échoué.', 'The update failed.'))),
      onSettled: () => setBusyId(null),
    });
  };

  const applyStatus = (m: MemberView, next: MembershipStatus, reason?: string) => {
    setBusyId(m.member_id);
    setStatusM.mutate(
      { memberId: m.member_id, status: next, reason },
      {
        onSuccess: () => {
          toast.success(
            next === 'active'
              ? tr('Accès rétabli', 'Access restored')
              : next === 'deactivated'
                ? tr('Membre désactivé', 'Member deactivated')
                : tr('Accès révoqué', 'Access revoked'),
          );
          setConfirm(null);
        },
        onError: (err) => toast.error(apiMessage(err, tr('Action impossible.', 'Action failed.'))),
        onSettled: () => setBusyId(null),
      },
    );
  };

  if (isLoading) return <SkeletonRows rows={5} />;
  if (isError) {
    return (
      <ErrorState
        title={tr('Impossible de charger les membres', 'Could not load members')}
        description={tr(
          'Réessayez, ou contactez un administrateur si le problème persiste.',
          'Retry, or contact an administrator if this persists.',
        )}
        onRetry={() => void refetch()}
        retryLabel={tr('Réessayer', 'Retry')}
      />
    );
  }

  const rows = data?.items ?? [];

  return (
    <>
      <div className="flex items-center gap-2.5 mb-3 flex-wrap">
        <div className="relative flex-1 min-w-[200px]">
          <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-ink-muted" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder={tr('Nom ou email…', 'Name or email…')}
            aria-label={tr('Rechercher un membre', 'Search members')}
            className="w-full h-9 pl-9 pr-3 rounded-[10px] text-[13px] text-ink outline-none"
            style={{ border: '1px solid var(--border-strong)', background: 'var(--bg-elevated)' }}
          />
        </div>
        {(['', 'active', 'deactivated', 'revoked'] as const).map((s) => (
          <Chip
            key={s || 'all'}
            label={s === '' ? tr('Tous', 'All') : tr(STATUS_STYLE[s].fr, STATUS_STYLE[s].en)}
            active={status === s}
            onClick={() => setStatus(s)}
            color={s === '' ? undefined : STATUS_STYLE[s].color}
          />
        ))}
        {isFetching && (
          <RefreshCw
            size={14}
            className="text-ink-muted animate-spin"
            aria-label={tr('Chargement', 'Loading')}
          />
        )}
      </div>

      {rows.length === 0 ? (
        <EmptyState
          variant={search || status ? 'no-results' : 'first-use'}
          icon={Users}
          title={
            search || status ? tr('Aucun résultat', 'No results') : tr('Aucun membre', 'No members')
          }
          description={
            search || status
              ? tr('Aucun membre ne correspond à ce filtre.', 'No member matches this filter.')
              : tr(
                  'Invitez votre premier collègue pour collaborer.',
                  'Invite your first colleague to collaborate.',
                )
          }
        />
      ) : (
        <Card className="p-0 overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full text-[13px]" style={{ minWidth: 760 }}>
              <caption className="sr-only">
                {tr("Membres de l'organisation", 'Organization members')}
              </caption>
              <thead>
                <tr
                  className="text-left text-ink-muted text-[11px] uppercase tracking-wide"
                  style={{ borderBottom: '1px solid var(--border)' }}
                >
                  <th scope="col" className="px-4 py-3 font-semibold">
                    {tr('Membre', 'Member')}
                  </th>
                  <th scope="col" className="px-4 py-3 font-semibold">
                    {tr('Statut', 'Status')}
                  </th>
                  <th scope="col" className="px-4 py-3 font-semibold">
                    {tr('Accès', 'Access')}
                  </th>
                  <th scope="col" className="px-4 py-3 font-semibold">
                    {tr('Membre depuis', 'Member since')}
                  </th>
                  <th scope="col" className="px-4 py-3 font-semibold text-right">
                    {tr('Actions', 'Actions')}
                  </th>
                </tr>
              </thead>
              <tbody>
                {rows.map((m) => {
                  const isSelf = me?.id === m.user_id;
                  const st = STATUS_STYLE[m.status] ?? STATUS_STYLE.active;
                  const selectValue =
                    m.org_role === 'admin' ? ADMIN_OPTION : (m.business_role ?? '');
                  const busy = busyId === m.member_id;
                  // The owner and your own row are refused by the server; the
                  // controls say so rather than offering a click that 403s.
                  const locked = m.is_owner || isSelf;
                  return (
                    <tr
                      key={m.member_id}
                      data-testid="member-row"
                      style={{
                        borderBottom: '1px solid var(--border)',
                        opacity: m.is_active ? 1 : 0.72,
                      }}
                    >
                      <td className="px-4 py-3">
                        <div className="font-semibold text-ink flex items-center gap-2">
                          {m.full_name || m.email}
                          {m.is_owner && (
                            <span
                              className="text-[10px] px-1.5 py-0.5 rounded font-bold"
                              style={{
                                background: 'var(--accent-soft)',
                                color: 'var(--accent-500)',
                              }}
                            >
                              {tr('Propriétaire', 'Owner')}
                            </span>
                          )}
                          {isSelf && (
                            <span className="text-[11px] text-ink-muted">
                              ({tr('vous', 'you')})
                            </span>
                          )}
                        </div>
                        <div className="text-[11.5px] text-ink-muted">{m.email}</div>
                      </td>
                      <td className="px-4 py-3">
                        {/* Colour AND words: a status must not depend on hue alone. */}
                        <span
                          className="inline-flex items-center gap-1.5 text-[12px] font-semibold"
                          style={{ color: st.color }}
                        >
                          <span
                            aria-hidden
                            className="w-1.5 h-1.5 rounded-full"
                            style={{ background: st.color }}
                          />
                          {tr(st.fr, st.en)}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        {locked ? (
                          <span className="text-[12px] text-ink-muted">
                            {m.is_owner
                              ? tr('Accès complet', 'Full access')
                              : tr('Votre propre rôle', 'Your own role')}
                          </span>
                        ) : (
                          <select
                            value={selectValue}
                            disabled={!canUpdate || busy}
                            onChange={(e) => changeRole(m, e.target.value)}
                            aria-label={tr(`Rôle de ${m.email}`, `Role for ${m.email}`)}
                            className="h-8 px-2 rounded-[8px] text-[12.5px] bg-transparent"
                            style={{
                              border: '1px solid var(--border-strong)',
                              color: 'var(--fg-primary)',
                              opacity: busy ? 0.6 : 1,
                            }}
                          >
                            <option value={ADMIN_OPTION}>
                              {tr('Administrateur (accès complet)', 'Administrator (full access)')}
                            </option>
                            <option value="">
                              {tr('— Aucun rôle métier —', '— No business role —')}
                            </option>
                            {catalog?.business_roles.map((r) => (
                              <option key={r.key} value={r.key}>
                                {roleLabel(r.key)}
                              </option>
                            ))}
                          </select>
                        )}
                      </td>
                      <td className="px-4 py-3 text-ink-soft text-[12px]">
                        {fmtDate(m.joined_at, lang)}
                      </td>
                      <td className="px-4 py-3">
                        <div className="flex items-center justify-end gap-1.5">
                          {!locked && canDeactivate && m.status === 'active' && (
                            <button
                              onClick={() => setConfirm({ member: m, next: 'deactivated' })}
                              disabled={busy}
                              className="h-8 px-2.5 rounded-[8px] text-[12px] font-semibold inline-flex items-center gap-1.5"
                              style={{
                                border: '1px solid var(--border-strong)',
                                color: 'var(--fg-secondary)',
                              }}
                            >
                              <Ban size={13} /> {tr('Désactiver', 'Deactivate')}
                            </button>
                          )}
                          {!locked && canDeactivate && m.status === 'deactivated' && (
                            <button
                              onClick={() => applyStatus(m, 'active')}
                              disabled={busy}
                              className="h-8 px-2.5 rounded-[8px] text-[12px] font-semibold inline-flex items-center gap-1.5"
                              style={{
                                border: '1px solid var(--border-strong)',
                                color: 'var(--low)',
                              }}
                            >
                              <CheckCircle2 size={13} />{' '}
                              {busy ? tr('…', '…') : tr('Réactiver', 'Reactivate')}
                            </button>
                          )}
                          {!locked && canDeactivate && m.status !== 'revoked' && (
                            <button
                              onClick={() => setConfirm({ member: m, next: 'revoked' })}
                              disabled={busy}
                              aria-label={tr(
                                `Révoquer l'accès de ${m.email}`,
                                `Revoke access for ${m.email}`,
                              )}
                              className="h-8 w-8 rounded-[8px] inline-flex items-center justify-center"
                              style={{
                                border: '1px solid var(--border-strong)',
                                color: 'var(--critical)',
                              }}
                            >
                              <Trash2 size={13} />
                            </button>
                          )}
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </Card>
      )}

      {/* Withdrawing someone's access is vital and outward-facing: an impact
          radiography, with the reversible alternative offered as a first-class
          button rather than buried. */}
      <DangerConfirm
        open={!!confirm}
        onClose={() => setConfirm(null)}
        title={
          confirm?.next === 'revoked'
            ? tr("Révoquer l'accès", 'Revoke access')
            : tr('Désactiver le membre', 'Deactivate member')
        }
        subject={confirm?.member.email}
        intro={
          confirm?.next === 'revoked'
            ? tr(
                "Cette personne perdra immédiatement l'accès à l'organisation et ses sessions seront fermées. La révocation est définitive : il faudra une nouvelle invitation pour la faire revenir.",
                'This person immediately loses access to the organization and their sessions are ended. Revocation is final: bringing them back requires a new invitation.',
              )
            : tr(
                "Cette personne perdra l'accès à l'organisation et ses sessions seront fermées. Vous pourrez la réactiver à tout moment.",
                'This person loses access to the organization and their sessions are ended. You can reactivate them at any time.',
              )
        }
        impact={
          confirm
            ? [
                { label: tr('Rôle', 'Role'), value: confirm.member.org_role },
                {
                  label: tr('Membre depuis', 'Member since'),
                  value: fmtDate(confirm.member.joined_at, lang),
                },
                {
                  label: tr('Sessions', 'Sessions'),
                  value: tr('fermées sous 15 minutes', 'ended within 15 minutes'),
                },
              ]
            : []
        }
        alternatives={
          confirm?.next === 'revoked'
            ? [
                {
                  label: tr('Désactiver — réversible', 'Deactivate — reversible'),
                  description: tr(
                    "L'accès est suspendu et peut être rétabli d'un clic.",
                    'Access is suspended and can be restored with one click.',
                  ),
                  icon: Ban,
                  onClick: () => confirm && applyStatus(confirm.member, 'deactivated'),
                },
              ]
            : undefined
        }
        confirmLabel={
          confirm?.next === 'revoked'
            ? tr("Révoquer l'accès", 'Revoke access')
            : tr('Désactiver', 'Deactivate')
        }
        onConfirm={() => confirm && applyStatus(confirm.member, confirm.next)}
        busy={setStatusM.isPending}
      />
    </>
  );
}

/* --------------------------------------------------------------- invitations */

function InvitationsTable({ tr, lang }: { tr: Tr; lang: LocaleCode }) {
  const { can } = usePermissions();
  const canInvite = can('organization:members:invite');
  const { data, isLoading, isError, refetch } = useInvitations();
  const resend = useResendInvitation();
  const revoke = useRevokeInvitation();
  const [busyId, setBusyId] = useState<string | null>(null);
  const [revoking, setRevoking] = useState<InvitationView | null>(null);
  const [relink, setRelink] = useState<InviteResult | null>(null);

  if (isLoading) return <SkeletonRows rows={4} />;
  if (isError) {
    return (
      <ErrorState
        title={tr('Impossible de charger les invitations', 'Could not load invitations')}
        description={tr(
          'Réessayez, ou contactez un administrateur si le problème persiste.',
          'Retry, or contact an administrator if this persists.',
        )}
        onRetry={() => void refetch()}
        retryLabel={tr('Réessayer', 'Retry')}
      />
    );
  }

  const rows = data?.items ?? [];
  if (rows.length === 0) {
    return (
      <EmptyState
        icon={Mail}
        title={tr('Aucune invitation', 'No invitations')}
        description={tr(
          'Les invitations en attente apparaîtront ici, avec leur date d’expiration.',
          'Outstanding invitations appear here, with their expiry.',
        )}
      />
    );
  }

  const doResend = (inv: InvitationView) => {
    setBusyId(inv.id);
    resend.mutate(
      { id: inv.id },
      {
        onSuccess: (res) => {
          if (res.delivery === 'sent')
            toast.success(tr('Invitation renvoyée', 'Invitation re-sent'));
          else setRelink(res); // the link has to be relayed by hand — show it
        },
        onError: (err) => toast.error(apiMessage(err, tr("L'envoi a échoué.", 'The send failed.'))),
        onSettled: () => setBusyId(null),
      },
    );
  };

  return (
    <>
      <Card className="p-0 overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full text-[13px]" style={{ minWidth: 720 }}>
            <caption className="sr-only">{tr('Invitations', 'Invitations')}</caption>
            <thead>
              <tr
                className="text-left text-ink-muted text-[11px] uppercase tracking-wide"
                style={{ borderBottom: '1px solid var(--border)' }}
              >
                <th scope="col" className="px-4 py-3 font-semibold">
                  {tr('Adresse', 'Address')}
                </th>
                <th scope="col" className="px-4 py-3 font-semibold">
                  {tr('Rôle', 'Role')}
                </th>
                <th scope="col" className="px-4 py-3 font-semibold">
                  {tr('Statut', 'Status')}
                </th>
                <th scope="col" className="px-4 py-3 font-semibold">
                  {tr('Expire le', 'Expires')}
                </th>
                <th scope="col" className="px-4 py-3 font-semibold">
                  {tr('Envois', 'Sends')}
                </th>
                <th scope="col" className="px-4 py-3 font-semibold text-right">
                  {tr('Actions', 'Actions')}
                </th>
              </tr>
            </thead>
            <tbody>
              {rows.map((inv) => {
                const st = INVITE_STATUS_STYLE[inv.status] ?? INVITE_STATUS_STYLE.pending;
                const busy = busyId === inv.id;
                const open = inv.status === 'pending' || inv.status === 'expired';
                return (
                  <tr
                    key={inv.id}
                    data-testid="invitation-row"
                    style={{ borderBottom: '1px solid var(--border)' }}
                  >
                    <td className="px-4 py-3">
                      <div className="font-medium text-ink">{inv.email}</div>
                      {inv.invited_by_email && (
                        <div className="text-[11.5px] text-ink-muted">
                          {tr('invité par', 'invited by')} {inv.invited_by_email}
                        </div>
                      )}
                    </td>
                    <td className="px-4 py-3 text-ink-soft">{inv.role}</td>
                    <td className="px-4 py-3">
                      <span
                        className="inline-flex items-center gap-1.5 text-[12px] font-semibold"
                        style={{ color: st.color }}
                      >
                        <span
                          aria-hidden
                          className="w-1.5 h-1.5 rounded-full"
                          style={{ background: st.color }}
                        />
                        {tr(st.fr, st.en)}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-ink-soft text-[12px]">
                      <span className="inline-flex items-center gap-1.5">
                        {inv.status === 'expired' && <Clock size={12} className="text-ink-muted" />}
                        {fmtDate(inv.expires_at, lang)}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-ink-soft text-[12px]">{inv.send_count}</td>
                    <td className="px-4 py-3">
                      <div className="flex items-center justify-end gap-1.5">
                        {canInvite && open && (
                          <button
                            onClick={() => doResend(inv)}
                            // The server's own answer, so the button is disabled
                            // for the same reason the API would refuse it.
                            disabled={busy || !inv.can_resend}
                            title={
                              inv.can_resend
                                ? tr("Renvoyer l'invitation", 'Re-send the invitation')
                                : tr(
                                    'Renvoi trop fréquent — patientez un instant',
                                    'Re-sending too often — wait a moment',
                                  )
                            }
                            className="h-8 px-2.5 rounded-[8px] text-[12px] font-semibold inline-flex items-center gap-1.5 disabled:opacity-45"
                            style={{
                              border: '1px solid var(--border-strong)',
                              color: 'var(--fg-secondary)',
                            }}
                          >
                            <RefreshCw size={13} className={busy ? 'animate-spin' : ''} />
                            {busy ? tr('Envoi…', 'Sending…') : tr('Renvoyer', 'Re-send')}
                          </button>
                        )}
                        {canInvite && open && (
                          <button
                            onClick={() => setRevoking(inv)}
                            disabled={busy}
                            aria-label={tr(
                              `Révoquer l'invitation de ${inv.email}`,
                              `Revoke the invitation for ${inv.email}`,
                            )}
                            className="h-8 w-8 rounded-[8px] inline-flex items-center justify-center"
                            style={{
                              border: '1px solid var(--border-strong)',
                              color: 'var(--critical)',
                            }}
                          >
                            <X size={14} />
                          </button>
                        )}
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </Card>

      <DangerConfirm
        open={!!revoking}
        onClose={() => setRevoking(null)}
        title={tr("Révoquer l'invitation", 'Revoke the invitation')}
        subject={revoking?.email}
        intro={tr(
          'Le lien envoyé cessera immédiatement de fonctionner. Vous pourrez inviter à nouveau cette adresse.',
          'The link that was sent stops working immediately. You can invite this address again afterwards.',
        )}
        impact={
          revoking
            ? [
                { label: tr('Rôle proposé', 'Offered role'), value: revoking.role },
                { label: tr('Envois', 'Sends'), value: String(revoking.send_count) },
              ]
            : []
        }
        confirmLabel={tr('Révoquer', 'Revoke')}
        onConfirm={() => {
          if (!revoking) return;
          revoke.mutate(revoking.id, {
            onSuccess: () => {
              toast.success(tr('Invitation révoquée', 'Invitation revoked'));
              setRevoking(null);
            },
            onError: (err) =>
              toast.error(apiMessage(err, tr('La révocation a échoué.', 'Revocation failed.'))),
          });
        }}
        busy={revoke.isPending}
      />

      {relink && <ManualLinkDialog tr={tr} result={relink} onClose={() => setRelink(null)} />}
    </>
  );
}

/* ------------------------------------------------------------------- history */

function AccessHistory({ tr, lang }: { tr: Tr; lang: LocaleCode }) {
  const { data, isLoading, isError, refetch } = useMembershipAudit(100);

  if (isLoading) return <SkeletonRows rows={6} />;
  if (isError) {
    return (
      <ErrorState
        title={tr('Impossible de charger le journal', 'Could not load the history')}
        description={tr(
          'Réessayez, ou contactez un administrateur si le problème persiste.',
          'Retry, or contact an administrator if this persists.',
        )}
        onRetry={() => void refetch()}
        retryLabel={tr('Réessayer', 'Retry')}
      />
    );
  }

  const rows = data?.items ?? [];
  if (rows.length === 0) {
    return (
      <EmptyState
        icon={History}
        title={tr('Aucun évènement', 'No events yet')}
        description={tr(
          "Chaque invitation, changement de rôle et retrait d'accès sera consigné ici.",
          'Every invitation, role change and access withdrawal is recorded here.',
        )}
      />
    );
  }

  return (
    <Card className="p-0 overflow-hidden">
      <ol className="divide-y" style={{ borderColor: 'var(--border)' }}>
        {rows.map((e) => (
          <li key={e.id} className="px-4 py-3 flex items-start gap-3" data-testid="audit-row">
            <span
              aria-hidden
              className="w-7 h-7 rounded-[9px] shrink-0 flex items-center justify-center mt-0.5"
              style={{
                background:
                  e.action === 'revoke'
                    ? 'color-mix(in srgb, var(--critical) 14%, transparent)'
                    : 'var(--bg-hover)',
                color: e.action === 'revoke' ? 'var(--critical)' : 'var(--fg-secondary)',
              }}
            >
              {e.entity_type === 'invitation' ? <Mail size={14} /> : <Users size={14} />}
            </span>
            <div className="flex-1 min-w-0">
              <div className="text-[13px] text-ink">{e.summary}</div>
              <div className="text-[11.5px] text-ink-muted mt-0.5">
                {/* An event with no actor is a self-registration, recorded
                    before any session existed. Saying "System" is honest;
                    attributing it to somebody would not be. */}
                {e.actor_email || (e.actor_id ? e.actor_id.slice(0, 8) : tr('Système', 'System'))}
                {' · '}
                {new Date(e.at).toLocaleString(localeTag(lang))}
                {e.ip_address ? ` · ${e.ip_address}` : ''}
              </div>
            </div>
          </li>
        ))}
      </ol>
    </Card>
  );
}

/* -------------------------------------------------------------------- invite */

function InviteDialog({ tr, onClose }: { tr: Tr; onClose: () => void }) {
  useEscapeToClose(true, onClose);
  const lang = useUIStore((s) => s.lang);
  const { data: catalog } = useRbacCatalog();
  const invite = useInviteMember();
  const [email, setEmail] = useState('');
  const [role, setRole] = useState('');
  const [result, setResult] = useState<InviteResult | null>(null);
  const [error, setError] = useState<string | null>(null);

  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    const input =
      role === ADMIN_OPTION
        ? { email, role: 'admin' as MemberRole, locale: lang }
        : { email, role: 'user' as MemberRole, business_role: role, locale: lang };
    invite.mutate(input, {
      onSuccess: (res) => setResult(res),
      onError: (err) =>
        setError(apiMessage(err, tr("L'invitation a échoué.", 'The invitation failed.'))),
    });
  };

  if (result) {
    return <ManualLinkDialog tr={tr} result={result} onClose={onClose} created />;
  }

  return (
    <Overlay onClose={onClose}>
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-[16px] font-bold text-ink">
          {tr('Inviter un membre', 'Invite a member')}
        </h2>
        <button
          onClick={onClose}
          className="w-8 h-8 rounded-[8px] flex items-center justify-center text-ink-muted hover:bg-hover"
          aria-label={tr('Fermer', 'Close')}
        >
          <X size={17} />
        </button>
      </div>
      <form onSubmit={submit}>
        <label className="block text-[12px] font-semibold text-ink-soft mb-1.5" htmlFor="inv-email">
          {tr('Adresse email', 'Email address')}
        </label>
        <input
          id="inv-email"
          type="email"
          required
          value={email}
          autoFocus
          onChange={(e) => setEmail(e.target.value)}
          placeholder="collegue@exemple.com"
          className="w-full h-[42px] px-3.5 rounded-[11px] text-[14px] text-ink outline-none mb-3"
          style={{ border: '1px solid var(--border-strong)', background: 'var(--bg-app)' }}
        />
        <label className="block text-[12px] font-semibold text-ink-soft mb-1.5" htmlFor="inv-role">
          {tr('Accès', 'Access')}
        </label>
        <select
          id="inv-role"
          value={role}
          onChange={(e) => setRole(e.target.value)}
          className="w-full h-[42px] px-3 rounded-[11px] text-[14px] text-ink outline-none mb-2"
          style={{ border: '1px solid var(--border-strong)', background: 'var(--bg-app)' }}
        >
          <option value="">{tr('— Aucun rôle métier —', '— No business role —')}</option>
          {catalog?.business_roles.map((r) => (
            <option key={r.key} value={r.key}>
              {lang === 'fr' ? r.label_fr : r.label_en}
            </option>
          ))}
          <option value={ADMIN_OPTION}>
            {tr('Administrateur (accès complet)', 'Administrator (full access)')}
          </option>
        </select>
        {/* The default option grants nothing — least privilege is the right
            default, but an admin who does not know that invites a colleague
            who then sees an almost empty product and assumes it is broken. */}
        {role === '' && (
          <p className="text-[11.5px] mb-2 leading-snug" style={{ color: 'var(--high)' }}>
            {tr(
              'Sans rôle métier, ce membre pourra se connecter mais ne verra presque rien. Vous pourrez lui en attribuer un à tout moment.',
              'With no business role, this member can sign in but will see almost nothing. You can assign one at any time.',
            )}
          </p>
        )}
        <p className="text-[11.5px] text-ink-muted mb-4 leading-snug">
          {tr(
            "L'invitation expire au bout de 7 jours et peut être révoquée à tout moment.",
            'The invitation expires after 7 days and can be revoked at any time.',
          )}
        </p>

        {error && (
          <div
            role="alert"
            className="mb-3 px-3 py-2.5 rounded-[10px] text-[12.5px] flex items-start gap-2"
            style={{
              background: 'color-mix(in srgb, var(--critical) 12%, transparent)',
              color: 'var(--critical)',
            }}
          >
            <AlertTriangle size={14} className="shrink-0 mt-0.5" /> <span>{error}</span>
          </div>
        )}

        <button
          type="submit"
          disabled={invite.isPending || !email}
          className="w-full h-[42px] rounded-[10px] text-[14px] font-semibold disabled:opacity-60"
          style={{ background: 'var(--accent-solid)', color: 'var(--fg-on-solid)' }}
        >
          {invite.isPending
            ? tr('Envoi…', 'Sending…')
            : tr("Envoyer l'invitation", 'Send invitation')}
        </button>
      </form>
    </Overlay>
  );
}

/**
 * Shown after an invite or a re-send. What it says depends on what actually
 * happened to the email — the product never claims a delivery it did not make.
 */
function ManualLinkDialog({
  tr,
  result,
  onClose,
  created,
}: {
  tr: Tr;
  result: InviteResult;
  onClose: () => void;
  created?: boolean;
}) {
  useEscapeToClose(true, onClose);
  const sent = result.delivery === 'sent';
  const copy = () => {
    if (!result.accept_url) return;
    navigator.clipboard
      ?.writeText(result.accept_url)
      .then(() => toast.success(tr('Lien copié', 'Link copied')))
      .catch(() => toast.error(tr('Copie impossible', 'Could not copy')));
  };

  return (
    <Overlay onClose={onClose}>
      <div className="flex items-start gap-3 mb-4">
        <span
          aria-hidden
          className="w-10 h-10 rounded-xl flex items-center justify-center shrink-0"
          style={{
            background: sent
              ? 'color-mix(in srgb, var(--low) 14%, transparent)'
              : 'color-mix(in srgb, var(--high) 14%, transparent)',
            color: sent ? 'var(--low)' : 'var(--high)',
          }}
        >
          {sent ? <CheckCircle2 size={20} /> : <AlertTriangle size={20} />}
        </span>
        <div className="flex-1 min-w-0">
          <h2 className="text-[15px] font-bold text-ink">
            {sent
              ? tr('Invitation envoyée', 'Invitation sent')
              : tr(
                  'Invitation créée — à transmettre vous-même',
                  'Invitation created — deliver it yourself',
                )}
          </h2>
          <div className="text-[13px] text-ink-soft mt-0.5 truncate">{result.invitation.email}</div>
        </div>
      </div>

      {sent ? (
        <p className="text-[13px] text-ink-soft leading-relaxed mb-4">
          {created
            ? tr(
                "Le lien d'invitation a été envoyé par email. Il expire dans 7 jours et vous pouvez le révoquer à tout moment depuis l'onglet Invitations.",
                'The invitation link was emailed. It expires in 7 days and you can revoke it at any time from the Invitations tab.',
              )
            : tr('Le lien a été renvoyé par email.', 'The link was re-sent by email.')}
        </p>
      ) : (
        <>
          <p className="text-[13px] text-ink-soft leading-relaxed mb-3">
            {result.delivery_detail ||
              tr(
                "L'email n'a pas pu partir. Transmettez ce lien à la personne concernée — il n'est affiché qu'une seule fois.",
                'The email could not be sent. Pass this link to the person yourself — it is shown only once.',
              )}
          </p>
          {result.accept_url && (
            <div
              className="flex items-center gap-2 p-3 rounded-[10px] mb-4"
              style={{ background: 'var(--bg-hover)', border: '1px solid var(--border-strong)' }}
            >
              <code className="mono text-[12px] text-ink flex-1 break-all">
                {result.accept_url}
              </code>
              <button
                onClick={copy}
                className="w-8 h-8 rounded-[8px] flex items-center justify-center text-ink-muted hover:bg-hover shrink-0"
                aria-label={tr('Copier le lien', 'Copy the link')}
              >
                <Copy size={16} />
              </button>
            </div>
          )}
        </>
      )}

      <button
        onClick={onClose}
        className="w-full h-[42px] rounded-[10px] text-[14px] font-semibold"
        style={{ background: 'var(--accent-solid)', color: 'var(--fg-on-solid)' }}
      >
        {tr('Terminé', 'Done')}
      </button>
    </Overlay>
  );
}

function Overlay({ children, onClose }: { children: React.ReactNode; onClose: () => void }) {
  return (
    <div
      className="fixed inset-0 z-90 flex items-center justify-center p-4"
      style={{ background: 'var(--surface-overlay)', backdropFilter: 'blur(6px)' }}
      onClick={onClose}
    >
      <div
        role="dialog"
        aria-modal="true"
        onClick={(e) => e.stopPropagation()}
        className="w-full max-w-[460px] rounded-[16px] p-5"
        style={{
          background: 'var(--bg-elevated)',
          border: '1px solid var(--border)',
          boxShadow: 'var(--shadow-overlay)',
          animation: 'or-scalein .18s cubic-bezier(.2,.8,.2,1)',
        }}
      >
        {children}
      </div>
    </div>
  );
}

/** The server's message when it sent one — it names the invariant that was
 *  broken ("this is the last active administrator"), which a generic string
 *  cannot. */
function apiMessage(err: unknown, fallback: string): string {
  const r = (err as { response?: { data?: { error?: string; message?: string } } })?.response;
  return r?.data?.error || r?.data?.message || fallback;
}
