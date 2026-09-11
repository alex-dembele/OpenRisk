// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

import { useEffect, useMemo, useState } from 'react';
import { Link, useNavigate, useLocation } from 'react-router';
import {
  ChevronsUpDown,
  PanelLeftClose,
  PanelLeftOpen,
  Plus,
  Settings,
  LogOut,
  Star,
} from 'lucide-react';
import { cn } from '../../shared/ds';
import { useUIStore } from '../../store/uiStore';
import { useUIStrings } from '../../shared/uiStrings';
import { useAuthStore } from '../../hooks/useAuthStore';
import { usePermissions } from '../../hooks/usePermissions';
import { OpenRiskLogo } from '../../shared/Logo';
import {
  visibleNavGroups,
  pinnedItems,
  ALL_NAV_ITEMS,
  type NavItem,
  type NavCount,
} from '../../shared/navModel';
import { useScore } from '../../hooks/useScore';
import { useOrganizationCounts } from '../../features/organization/useOrganization';
import { bandColor, bandLabel, bandTextColor } from '../../services/scoreService';

interface SidebarProps {
  /** Off-canvas drawer open on mobile (< lg). Ignored on desktop, where the
   *  sidebar is always in the layout flow. */
  mobileOpen?: boolean;
  onMobileClose?: () => void;
}

function initials(name?: string, fallback = 'AD'): string {
  if (!name?.trim()) return fallback;
  const parts = name.trim().split(/\s+/);
  return ((parts[0]?.[0] ?? '') + (parts[1]?.[0] ?? '')).toUpperCase() || fallback;
}

// Human-readable role label: the GRC business role (RSSI, Risk Manager, …) when
// set, otherwise the org role (Administrator / Member).
const BUSINESS_ROLE_LABELS: Record<string, string> = {
  rssi: 'RSSI / CISO',
  dsi: 'DSI / CIO',
  risk_manager: 'Risk Manager',
  auditor: 'Auditeur',
  compliance_officer: 'Responsable conformité',
  internal_control: 'Contrôle interne',
  asset_owner: "Propriétaire d'actif",
  risk_owner: 'Propriétaire de risque',
  security_analyst: 'Analyste sécurité',
  executive: 'Direction',
  viewer: 'Lecteur',
};
function roleLabel(user?: { role?: string; business_role?: string } | null): string {
  if (user?.business_role && BUSINESS_ROLE_LABELS[user.business_role])
    return BUSINESS_ROLE_LABELS[user.business_role];
  if (user?.role === 'admin' || user?.role === 'root') return 'Administrateur';
  return user?.role ? user.role : 'Membre';
}

export const Sidebar = ({ mobileOpen = false, onMobileClose }: SidebarProps) => {
  const collapsed = useUIStore((s) => s.sidebarCollapsed);
  const toggleCollapse = useUIStore((s) => s.toggleSidebar);
  const L = useUIStrings();
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const navigate = useNavigate();
  const { pathname, search } = useLocation();
  const user = useAuthStore((s) => s.user);
  const logout = useAuthStore((s) => s.logout);
  const { can, isAdmin } = usePermissions();
  const [menuOpen, setMenuOpen] = useState(false);
  // Real org identity + posture — replaces the former hardcoded fixtures.
  const orgName = user?.org_name?.trim() || tr('Mon organisation', 'My organization');
  const orgInitials = initials(orgName, 'OR');
  // The canonical tenant score — the SAME query key the dashboard hero and the
  // dedicated page use, so all three render one object from one fetch. The
  // sidebar used to read cyber_score off the executive dashboard while the hero
  // read stats.global_risk_score: two quantities, two scales, opposite
  // directions, both labelled "score".
  const { data: tenantScore } = useScore('tenant');

  // Role-aware navigation: only surface screens the member can actually reach
  // (same permission gates the API enforces), so each business role sees a menu
  // coherent with its job.
  const navGroups = useMemo(
    () => visibleNavGroups(can, isAdmin()),
    // `can`/`isAdmin` are stable per permission set (memoized in usePermissions).
    [can, isAdmin],
  );
  // Dashboard (and any future pinned entry) is hoisted above the intention groups.
  const pinned = useMemo(() => pinnedItems(navGroups), [navGroups]);

  const handleLogout = () => {
    setMenuOpen(false);
    logout();
    navigate('/login', { replace: true });
  };

  // Close the mobile drawer whenever the route changes.
  useEffect(() => {
    onMobileClose?.();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pathname]);

  // Longest-prefix match so /assets/universe highlights Universe, not Inventory.
  const activeKey = useMemo(() => {
    let best = '';
    let bestLen = -1;
    for (const it of ALL_NAV_ITEMS) {
      const p = it.path;
      // Items sharing a pathname (Dashboard vs Executive, both "/") are told
      // apart by ?view=; an item without a `view` is active only when the param
      // is absent, so the two never light up together.
      const viewParam = new URLSearchParams(search).get('view');
      if ((it.view ?? null) !== (viewParam || null)) continue;
      const match = p === '/' ? pathname === '/' : pathname === p || pathname.startsWith(p + '/');
      if (match && p.length > bestLen) {
        best = it.key;
        bestLen = p.length;
      }
    }
    return best;
  }, [pathname, search]);

  // Posture footer. No thresholds live here any more: the band comes from the
  // server alongside the value, and bandColor maps a BAND (never a number) to a
  // token — so this cannot disagree with the server about where a cut lies.
  const score = tenantScore ? Math.round(tenantScore.value) : undefined;
  const scoreColor = bandColor(tenantScore?.band);
  // The bar uses the fill colour; the number is small text and needs the
  // text-weight token (see bandTextColor).
  const scoreTextColor = bandTextColor(tenantScore?.band);

  // Live, tenant-scoped counters for the nav badges. A failed or refused read
  // leaves every count at zero, which renders no badge at all — the honest
  // outcome, since we do not know the number.
  const { data: orgCounts } = useOrganizationCounts();
  const navCounts: Record<NavCount, number> = {
    pending_invitations: orgCounts?.pending_invitations ?? 0,
  };

  const navItem = (item: NavItem) => {
    const active = item.key === activeKey;
    const Icon = item.icon;
    return (
      // A LINK, not a button. Every nav entry used to be `<button
      // onClick={navigate(...)}>`, which renders no href — so Ctrl/Cmd+click,
      // middle-click and "copy link address" all did nothing, and a screen
      // reader announced 27 buttons where a user expects a list of links.
      //
      // A risk manager comparing two registers side by side needs a second tab,
      // and there was no way to open one. `Link` keeps the SPA navigation for a
      // plain click and lets the browser handle the modified ones itself.
      <Link
        key={item.key}
        to={item.href ?? item.path}
        data-testid={`nav-${item.key}`}
        // Anchor for the product tour's third coach mark (features/onboarding/ProductTour).
        data-tour={`nav-${item.key}`}
        // The visual highlight told sighted users which page they were on; this
        // is the same fact, for everyone else.
        aria-current={active ? 'page' : undefined}
        title={L[item.labelKey]}
        className={cn(
          'w-full flex items-center gap-[11px] px-[11px] py-2 rounded-[9px] relative mb-0.5 transition-colors',
          active ? 'bg-accent-soft' : 'hover:bg-hover',
        )}
      >
        <span
          className="flex shrink-0"
          style={{ color: active ? 'var(--accent)' : 'var(--fg-secondary)' }}
        >
          <Icon size={19} strokeWidth={1.75} />
        </span>
        {!collapsed && (
          <span
            className="text-[13px] whitespace-nowrap flex-1 text-left"
            style={{
              fontWeight: active ? 600 : 500,
              color: active ? 'var(--fg-primary)' : 'var(--fg-secondary)',
            }}
          >
            {L[item.labelKey]}
          </span>
        )}
        {/* A badge appears only when its live count is a positive number.
            While the count is loading, errored, or refused by permission it is
            absent — a placeholder here would be indistinguishable from a real
            figure, and the whole point of removing the hardcoded '12' was that
            a counter must never state something nobody measured. Zero is also
            absent: a badge means "something is waiting", and nothing is. */}
        {item.badge &&
          navCounts[item.badge.count] > 0 &&
          (collapsed ? (
            <span
              className="absolute top-1.5 right-1.5 w-2 h-2 rounded-full"
              style={{ background: item.badge.color ?? 'var(--critical)' }}
              aria-label={`${navCounts[item.badge.count]}`}
            />
          ) : (
            <span
              className="text-[10px] font-bold min-w-[17px] h-[17px] px-[5px] rounded-[9px] flex items-center justify-center text-fg-primary"
              style={{ background: item.badge.color ?? 'var(--critical)' }}
            >
              {navCounts[item.badge.count]}
            </span>
          ))}
      </Link>
    );
  };

  return (
    <>
      {/* Backdrop — mobile only. */}
      {mobileOpen && (
        <div
          onClick={onMobileClose}
          className="lg:hidden fixed inset-0 bg-surface-overlay backdrop-blur-sm z-40"
          aria-hidden="true"
        />
      )}

      <aside
        style={{ background: 'var(--bg-secondary)', transition: 'width .25s ease' }}
        className={cn(
          'h-screen border-r border-border flex flex-col z-50',
          'lg:static lg:shrink-0 lg:translate-x-0',
          collapsed ? 'lg:w-[66px]' : 'lg:w-[248px]',
          'fixed inset-y-0 left-0 w-[248px] max-w-[82vw]',
          mobileOpen ? 'translate-x-0 shadow-card-lg' : '-translate-x-full lg:translate-x-0',
        )}
      >
        <div className="flex flex-col h-full">
          {/* Logo + org switcher */}
          <div className="px-[14px] pt-4 pb-2.5">
            <div
              className={cn(
                'flex items-center gap-2.5 px-1.5 pb-3.5',
                collapsed && 'justify-center px-0',
              )}
            >
              <div
                className="w-[30px] h-[30px] rounded-[9px] flex items-center justify-center shrink-0 text-fg-on-solid"
                style={{
                  background: 'var(--accent-solid)',
                }}
              >
                <OpenRiskLogo size={18} />
              </div>
              {!collapsed && (
                <span className="disp text-[17px] font-bold tracking-tight text-ink">OpenRisk</span>
              )}
            </div>

            {!collapsed && (
              <button
                onClick={() => navigate('/settings')}
                className="w-full flex items-center gap-2.5 px-2 py-1.5 rounded-[9px] hover:bg-hover transition-colors"
              >
                <div
                  className="w-[26px] h-[26px] rounded-[7px] flex items-center justify-center text-[11px] font-bold shrink-0 text-accent-strong"
                  style={{ background: 'var(--accent-soft)' }}
                >
                  {orgInitials}
                </div>
                <div className="min-w-0 flex-1 text-left">
                  <div className="text-[12.5px] font-semibold leading-tight text-ink truncate">
                    {orgName}
                  </div>
                  <div className="text-[10.5px] text-ink-soft">{L.enterprise}</div>
                </div>
                <ChevronsUpDown size={13} className="text-ink-muted shrink-0" />
              </button>
            )}
          </div>

          {/* Quick action */}
          <div className={cn('px-[14px] pb-2.5', collapsed && 'px-2.5')}>
            <button
              data-tour="new-risk"
              onClick={() => {
                window.dispatchEvent(new CustomEvent('openrisk:new-risk'));
                onMobileClose?.();
              }}
              className="w-full h-[38px] rounded-[10px] flex items-center justify-center gap-2 text-[13px] font-semibold text-fg-primary transition-[filter] hover:brightness-110"
              style={{
                background: 'var(--accent-solid)',
                color: 'var(--fg-on-solid)',
              }}
              title={L.newRisk}
            >
              <Plus size={16} strokeWidth={2.2} />
              {!collapsed && <span>{L.newRisk}</span>}
            </button>
          </div>

          {/* Navigation */}
          <nav className="flex-1 overflow-y-auto px-2.5 pt-1.5 pb-2.5">
            {/* Pinned entries (Dashboard) hoisted above the intention groups. */}
            {pinned.length > 0 && (
              <div className="mb-3 pb-3 border-b border-border">{pinned.map(navItem)}</div>
            )}
            {navGroups.map((group) => {
              const items = group.items.filter((i) => !i.pinned);
              if (items.length === 0) return null;
              return (
                <div key={group.groupKey} className="mb-4">
                  {!collapsed && (
                    <div
                      className={cn(
                        'text-[10px] tracking-[0.09em] uppercase font-semibold px-3 pb-[7px] flex items-center gap-1.5',
                        !group.core && 'text-ink-muted',
                      )}
                      style={group.core ? { color: 'var(--accent-500)' } : undefined}
                    >
                      {group.core && <Star size={10} strokeWidth={2.5} fill="var(--accent)" />}
                      {L[group.groupKey]}
                    </div>
                  )}
                  {items.map(navItem)}
                </div>
              );
            })}
          </nav>

          {/* Security score footer — real cyber score; hidden until available. */}
          {!collapsed && score !== undefined && (
            <button
              onClick={() => navigate('/?view=executive')}
              className="w-full text-left px-[14px] py-3 border-t border-border hover:bg-hover transition-colors"
              title={tr('Voir le tableau exécutif', 'Open the executive dashboard')}
            >
              <div className="flex items-center justify-between mb-[7px]">
                <span className="text-[10.5px] text-ink-soft font-medium">
                  {L.globalScore} · {bandLabel(tenantScore?.band, lang)}
                </span>
                <span className="mono text-[12px] font-semibold" style={{ color: scoreTextColor }}>
                  {score}/100
                </span>
              </div>
              <div
                className="h-[5px] rounded-[5px] overflow-hidden"
                style={{ background: 'var(--bg-hover)' }}
              >
                <div
                  className="h-full rounded-[5px]"
                  style={{
                    width: `${score}%`,
                    background: scoreColor,
                    transition: 'width .8s cubic-bezier(.2,.8,.2,1)',
                  }}
                />
              </div>
            </button>
          )}

          {/* User menu (account · settings · logout) + collapse */}
          <div className="relative px-[14px] py-3 border-t border-border">
            {menuOpen && (
              <>
                <div
                  className="fixed inset-0 z-59"
                  onClick={() => setMenuOpen(false)}
                  aria-hidden="true"
                />
                <div
                  className="absolute left-[14px] right-[14px] z-60 rounded-[12px] overflow-hidden shadow-card-lg"
                  style={{
                    bottom: 'calc(100% - 6px)',
                    background: 'var(--bg-elevated)',
                    border: '1px solid var(--border)',
                    animation: 'or-scalein .14s cubic-bezier(.2,.8,.2,1)',
                  }}
                >
                  <div className="px-3 py-2.5 border-b border-border">
                    <div className="text-[12.5px] font-semibold text-ink truncate">
                      {user?.full_name || user?.username || 'Admin'}
                    </div>
                    <div className="text-[11px] text-ink-muted truncate">{user?.email}</div>
                  </div>
                  <button
                    onClick={() => {
                      setMenuOpen(false);
                      navigate('/settings');
                    }}
                    className="w-full flex items-center gap-2.5 px-3 py-2.5 text-[13px] font-medium text-ink hover:bg-hover transition-colors"
                  >
                    <Settings size={16} strokeWidth={1.8} /> {tr('Paramètres', 'Settings')}
                  </button>
                  <button
                    onClick={handleLogout}
                    className="w-full flex items-center gap-2.5 px-3 py-2.5 text-[13px] font-medium hover:bg-hover transition-colors"
                    style={{ color: 'var(--critical)' }}
                  >
                    <LogOut size={16} strokeWidth={1.8} /> {tr('Se déconnecter', 'Log out')}
                  </button>
                </div>
              </>
            )}

            <div className={cn('flex items-center gap-2.5', collapsed && 'justify-center')}>
              <button
                onClick={() => setMenuOpen((v) => !v)}
                title={tr('Compte', 'Account')}
                aria-label={tr('Menu du compte', 'Account menu')}
                className={cn(
                  'flex items-center gap-2.5 min-w-0 rounded-[9px] py-1 pr-1.5 hover:bg-hover transition-colors',
                  collapsed ? 'px-1' : 'flex-1 pl-1',
                )}
              >
                <div
                  className="w-[30px] h-[30px] rounded-full flex items-center justify-center text-[11px] font-bold text-accent-strong shrink-0"
                  style={{ background: 'var(--accent-soft)' }}
                >
                  {initials(user?.full_name)}
                </div>
                {!collapsed && (
                  <div className="flex-1 min-w-0 text-left">
                    <div className="text-[12px] font-semibold leading-tight text-ink truncate">
                      {user?.full_name || user?.username || 'Admin'}
                    </div>
                    <div className="text-[10.5px] text-ink-soft truncate">{roleLabel(user)}</div>
                  </div>
                )}
              </button>
              {!collapsed && (
                <button
                  onClick={toggleCollapse}
                  className="w-[26px] h-[26px] rounded-[7px] flex items-center justify-center text-ink-muted hover:bg-hover hover:text-ink transition-colors shrink-0"
                  aria-label="Collapse sidebar"
                >
                  <PanelLeftClose size={16} strokeWidth={1.7} />
                </button>
              )}
            </div>
          </div>

          {/* Expand affordance when collapsed (desktop) */}
          {collapsed && (
            <button
              onClick={toggleCollapse}
              className="hidden lg:flex mx-auto mb-3 w-[26px] h-[26px] rounded-[7px] items-center justify-center text-ink-muted hover:bg-hover hover:text-ink transition-colors"
              aria-label="Expand sidebar"
            >
              <PanelLeftOpen size={16} strokeWidth={1.7} />
            </button>
          )}
        </div>
      </aside>
    </>
  );
};
