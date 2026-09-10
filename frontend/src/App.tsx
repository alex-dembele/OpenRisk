// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

import { useEffect, useState, lazy, Suspense, type ReactNode } from 'react';
import {
  BrowserRouter,
  Routes,
  Route,
  Navigate,
  Outlet,
  useLocation,
  useNavigate,
  useParams,
} from 'react-router';

// --- Imports des Stores & Hooks ---
import { useAuthStore } from './hooks/useAuthStore';
import { useRiskStore } from './hooks/useRiskStore';
import { usePermissions } from './hooks/usePermissions';
import { useUIStore } from './store/uiStore';
import { useHotkeys } from './shared/useHotkeys';
import { ShortcutsOverlay } from './shared/ShortcutsOverlay';
import { permissionFor } from './shared/routeModel';
import { AccessDenied } from './shared/AccessDenied';
import { NotFound } from './shared/system/NotFound';
import { Maintenance } from './shared/system/Maintenance';
import {
  OnboardingGuard,
  OnboardingCompletedRedirect,
} from './features/onboarding/OnboardingGuard';

// --- App shell ---
import { Sidebar } from './components/layout/Sidebar';
import { AppHeader } from './components/layout/AppHeader';
// Command palette pulls @floating-ui + the search stack; it is only needed once
// the user opens it (⌘K / "/"), so it loads on first open, not at app start.
const CommandPalette = lazy(() =>
  import('./components/layout/CommandPalette').then((m) => ({ default: m.CommandPalette })),
);
import { GlobalShortcuts } from './components/layout/GlobalShortcuts';
import { DemoBanner } from './shared/DemoBanner';
import { OfflineBanner } from './shared/OfflineBanner';
import { ProductTour } from './features/onboarding/ProductTour';
import { EntityDrawerHost } from './features/entity-drawer';
import { useRealtimeQuerySync } from './features/realtime/useRealtime';
// The dc.html-redesign Create-Risk modal (crash-free, correct P×I×AC score scale).
// The older duplicate (features/risks/components/CreateRiskModal, which embedded
// ScoreEngineVisualizer and white-screened on a null response) was removed in RC1.
// --- The create-risk modal and the public auth screens are route/interaction
//     split too: they pull the form stack (react-hook-form + zod) and the auth
//     screens, none of which a logged-in user needs on the dashboard's first
//     paint. Each loads behind a Suspense boundary on demand. ---
const CreateRiskModal = lazy(() =>
  import('./features/risks/CreateRiskModal').then((m) => ({ default: m.CreateRiskModal })),
);
const AuthScreen = lazy(() =>
  import('./features/auth/AuthScreen').then((m) => ({ default: m.AuthScreen })),
);
const StatusPage = lazy(() =>
  import('./features/status/StatusPage').then((m) => ({ default: m.StatusPage })),
);
const AcceptInvitationPage = lazy(() =>
  import('./features/organization/AcceptInvitationPage').then((m) => ({
    default: m.AcceptInvitationPage,
  })),
);
const ForgotPasswordScreen = lazy(() =>
  import('./features/auth/ForgotPasswordScreen').then((m) => ({ default: m.ForgotPasswordScreen })),
);
const ResetPasswordScreen = lazy(() =>
  import('./features/auth/ResetPasswordScreen').then((m) => ({ default: m.ResetPasswordScreen })),
);

// --- Feature pages are route-split with React.lazy so the initial bundle only
//     carries the shell + auth; each screen's chunk loads on navigation. This
//     cut the single ~1.5 MB bundle into per-route chunks. ---
const SettingsScreen = lazy(() =>
  import('./features/settings/SettingsScreen').then((m) => ({ default: m.SettingsScreen })),
);
const DashboardPage = lazy(() =>
  import('./features/dashboard/DashboardPage').then((m) => ({ default: m.DashboardPage })),
);
const ActionCenterPage = lazy(() =>
  import('./features/action-center/ActionCenterPage').then((m) => ({
    default: m.ActionCenterPage,
  })),
);
const ImportRisksPage = lazy(() =>
  import('./features/risks/ImportRisksPage').then((m) => ({ default: m.ImportRisksPage })),
);
const RiskRegisterPage = lazy(() =>
  import('./features/risks/RiskRegisterPage').then((m) => ({ default: m.RiskRegisterPage })),
);
const UnmappedRisksPage = lazy(() =>
  import('./features/risks/UnmappedRisksPage').then((m) => ({ default: m.UnmappedRisksPage })),
);
const RiskWeightsSettings = lazy(() =>
  import('./features/risks/RiskWeightsSettings').then((m) => ({ default: m.RiskWeightsSettings })),
);
const VulnerabilitiesPage = lazy(() =>
  import('./features/vulnerabilities/VulnerabilitiesPage').then((m) => ({
    default: m.VulnerabilitiesPage,
  })),
);
const MitigationsBoard = lazy(() =>
  import('./features/mitigations/MitigationsBoard').then((m) => ({ default: m.MitigationsBoard })),
);
const ComplianceScreen = lazy(() =>
  import('./features/compliance/ComplianceScreen').then((m) => ({ default: m.ComplianceScreen })),
);
const FrameworkDetail = lazy(() =>
  import('./features/compliance/FrameworkDetail').then((m) => ({ default: m.FrameworkDetail })),
);
const EvidenceLibraryPage = lazy(() =>
  import('./features/evidence/EvidenceLibraryPage').then((m) => ({
    default: m.EvidenceLibraryPage,
  })),
);
const MissingEvidencePage = lazy(() =>
  import('./features/evidence/MissingEvidencePage').then((m) => ({
    default: m.MissingEvidencePage,
  })),
);
const ReportsLibraryPage = lazy(() =>
  import('./features/reports/ReportsLibraryPage').then((m) => ({ default: m.ReportsLibraryPage })),
);
const GapAnalysisPage = lazy(() =>
  import('./features/compliance/GapAnalysisPage').then((m) => ({ default: m.GapAnalysisPage })),
);
const AuditsPage = lazy(() =>
  import('./features/compliance/AuditsPage').then((m) => ({ default: m.AuditsPage })),
);
const RemediationPage = lazy(() =>
  import('./features/compliance/RemediationPage').then((m) => ({ default: m.RemediationPage })),
);
const InventoryPage = lazy(() =>
  import('./features/assets/InventoryPage').then((m) => ({ default: m.InventoryPage })),
);
const AssetSchemaSettings = lazy(() => import('./features/attackSurface/AssetSchemaSettings'));
const TopologyView = lazy(() => import('./features/attackSurface/TopologyView'));
const UnassignedVulnerabilitiesPage = lazy(
  () => import('./features/vulnerabilities/UnassignedVulnerabilitiesPage'),
);
const RiskRulePage = lazy(() => import('./features/attackSurface/RiskRulePage'));
const FinancialDashboard = lazy(() =>
  import('./features/financial/FinancialDashboard').then((m) => ({
    default: m.FinancialDashboard,
  })),
);
const GlobalTimelinePage = lazy(() =>
  import('./features/entity-drawer/GlobalTimelinePage').then((m) => ({
    default: m.GlobalTimelinePage,
  })),
);
const AutomationPage = lazy(() =>
  import('./features/automation/AutomationPage').then((m) => ({ default: m.AutomationPage })),
);
const GovernancePage = lazy(() =>
  import('./features/governance/GovernancePage').then((m) => ({ default: m.GovernancePage })),
);
const LeaderboardPage = lazy(() =>
  import('./features/gamification/LeaderboardPage').then((m) => ({ default: m.LeaderboardPage })),
);
const WarRoom = lazy(() =>
  import('./features/incidents/WarRoom').then((m) => ({ default: m.WarRoom })),
);
const IncidentsScreen = lazy(() =>
  import('./features/incidents/IncidentsScreen').then((m) => ({ default: m.IncidentsScreen })),
);
const IncidentSourcesPage = lazy(() =>
  import('./features/incidents/IncidentSourcesPage').then((m) => ({
    default: m.IncidentSourcesPage,
  })),
);
const ThreatIntel = lazy(() =>
  import('./features/cti/ThreatIntel').then((m) => ({ default: m.ThreatIntel })),
);
const InfrastructurePage = lazy(() =>
  import('./features/infrastructure/InfrastructurePage').then((m) => ({
    default: m.InfrastructurePage,
  })),
);
const ScanPreviewPage = lazy(() =>
  import('./features/infrastructure/ScanPreviewPage').then((m) => ({ default: m.ScanPreviewPage })),
);
const SimulationsPage = lazy(() =>
  import('./features/simulations/SimulationsPage').then((m) => ({ default: m.SimulationsPage })),
);
const ReportsScreen = lazy(() =>
  import('./features/reports/ReportsScreen').then((m) => ({ default: m.ReportsScreen })),
);
const AiAdvisor = lazy(() =>
  import('./features/ai/AiAdvisor').then((m) => ({ default: m.AiAdvisor })),
);
const EmergingRisksPage = lazy(() =>
  import('./features/ai/EmergingRisksPage').then((m) => ({ default: m.EmergingRisksPage })),
);
const BoardReportPage = lazy(() =>
  import('./features/reports/BoardReportPage').then((m) => ({ default: m.BoardReportPage })),
);
const AuditDetailPage = lazy(() =>
  import('./features/compliance/AuditDetailPage').then((m) => ({ default: m.AuditDetailPage })),
);
const RemediationDetailPage = lazy(() =>
  import('./features/compliance/RemediationDetailPage').then((m) => ({
    default: m.RemediationDetailPage,
  })),
);
const MitigationDetailPage = lazy(() =>
  import('./features/mitigations/MitigationDetailPage').then((m) => ({
    default: m.MitigationDetailPage,
  })),
);
const ReportJobPage = lazy(() =>
  import('./features/reports/ReportJobPage').then((m) => ({ default: m.ReportJobPage })),
);
const ScorePage = lazy(() =>
  import('./features/score/ScorePage').then((m) => ({ default: m.ScorePage })),
);

// --- Signup wizard (spec §4). Route-split like every other feature: it is shown
//     once per user, so it has no business sitting in the shell bundle. ---
const OnboardingWizard = lazy(() =>
  import('./features/onboarding/wizard/OnboardingWizard').then((m) => ({
    default: m.OnboardingWizard,
  })),
);
const OrganizationStep = lazy(() =>
  import('./features/onboarding/wizard/steps').then((m) => ({ default: m.OrganizationStep })),
);
const ProfileStep = lazy(() =>
  import('./features/onboarding/wizard/steps').then((m) => ({ default: m.ProfileStep })),
);
const GoalStep = lazy(() =>
  import('./features/onboarding/wizard/steps').then((m) => ({ default: m.GoalStep })),
);
const FrameworkStep = lazy(() =>
  import('./features/onboarding/wizard/steps').then((m) => ({ default: m.FrameworkStep })),
);
const TeamStep = lazy(() =>
  import('./features/onboarding/wizard/steps').then((m) => ({ default: m.TeamStep })),
);
// #438 — the tunnel's terminal screen and the recognition screen for tenants
// that were already configured. Lazy like every other route-level page.
const PosturePage = lazy(() =>
  import('./features/onboarding/PosturePage').then((m) => ({ default: m.PosturePage })),
);
const RecognitionPage = lazy(() =>
  import('./features/onboarding/RecognitionPage').then((m) => ({ default: m.RecognitionPage })),
);

/**
 * COMPOSANT 1: PROTECTION DE ROUTE
 * Vérifie si le token existe, sinon redirige vers Login.
 */
const ProtectedRoute = ({ children }: { children: React.ReactNode }) => {
  // Gate on isAuthenticated, not on the access token.
  //
  // Since the session moved to HttpOnly cookies the token is held in memory
  // only, so it is null after every reload even though the session is valid.
  // Gating on it logged the user out on any hard navigation. The cookie is the
  // credential now, and the server is the authority: if it has expired, the
  // first API call returns 401 and the axios interceptor redirects here.
  const isAuthenticated = useAuthStore((state) => state.isAuthenticated);
  if (!isAuthenticated) return <Navigate to="/login" replace />;
  return <>{children}</>;
};

/**
 * Fades/slides the route content on every navigation, keyed by pathname, so
 * switching pages never feels like an abrupt jump-cut.
 *
 * CSS rather than framer-motion, since #462. This was the ONLY framer-motion
 * usage in the entry graph, and it alone kept the whole 37 KB motion chunk in
 * the preload for every cold load — for a 180ms fade. The other 55 files that
 * use framer-motion sit behind lazy routes and still get it, on demand.
 *
 * `key` on the element is what replays the animation: React remounts the
 * subtree when the pathname changes, and `both` fill-mode holds the start frame
 * until it does.
 *
 * WHAT IS LOST, deliberately: AnimatePresence's exit fade. CSS cannot animate an
 * element that is being unmounted, so the outgoing page is removed at once
 * instead of fading over 180ms. The entering animation is identical to what it
 * replaces. See the note in #462 — this is the one visible change in that issue.
 */
const AnimatedOutlet = () => {
  const location = useLocation();
  return (
    <div
      key={location.pathname}
      className="motion-safe:animate-route flex flex-1 flex-col min-h-0 h-full"
    >
      <Outlet />
    </div>
  );
};

/**
 * COMPOSANT 2: LAYOUT GLOBAL — App Shell (OpenRisk.dc.html §5)
 * Grouped Sidebar (static ≥ lg, off-canvas drawer < lg) + glass AppHeader +
 * scrollable body. The ⌘K command palette and a global "New risk" modal live
 * here so they're reachable from anywhere (sidebar quick action, palette).
 */
const DashboardLayout = () => {
  // Opens the tab's single realtime connection and keeps the query cache in
  // step with it. Mounted once, here, so no feature has to open a connection of
  // its own — which is what the app used to do, ending up with several sockets
  // and several reconnect strategies to say the same thing.
  useRealtimeQuerySync();

  const [mobileNavOpen, setMobileNavOpen] = useState(false);
  const [newRiskOpen, setNewRiskOpen] = useState(false);
  const [showShortcuts, setShowShortcuts] = useState(false);
  const navigate = useNavigate();
  const lang = useUIStore((s) => s.lang);
  const setCmdkOpen = useUIStore((s) => s.setCmdkOpen);
  const cmdkOpen = useUIStore((s) => s.cmdkOpen);
  const toggleTheme = useUIStore((s) => s.toggleTheme);

  // The activation checklist's primary step deep-links to /risks?guided=1. Honour
  // it by opening the create-risk modal (which offers the three sector drafts),
  // then strip the parameter so a reload or a Back does not reopen it.
  const location = useLocation();
  useEffect(() => {
    if (!new URLSearchParams(location.search).has('guided')) return;
    setNewRiskOpen(true);
    const next = new URLSearchParams(location.search);
    next.delete('guided');
    navigate({ pathname: location.pathname, search: next.toString() }, { replace: true });
  }, [location.search, location.pathname, navigate]);

  // The sidebar quick action and command palette dispatch this to open the modal.
  // A header button dispatches openrisk:shortcuts to reveal the shortcuts overlay.
  useEffect(() => {
    const openRisk = () => setNewRiskOpen(true);
    const openShortcuts = () => setShowShortcuts(true);
    window.addEventListener('openrisk:new-risk', openRisk);
    window.addEventListener('openrisk:shortcuts', openShortcuts);
    return () => {
      window.removeEventListener('openrisk:new-risk', openRisk);
      window.removeEventListener('openrisk:shortcuts', openShortcuts);
    };
  }, []);

  // Discoverable global shortcuts (UX-26). Rows shown in ShortcutsOverlay must
  // mirror these handlers. The hook ignores keys while typing / with ⌘/Ctrl/Alt.
  useHotkeys([
    { key: '?', handler: () => setShowShortcuts((v) => !v) },
    {
      key: 'n',
      handler: () => {
        setShowShortcuts(false);
        window.dispatchEvent(new CustomEvent('openrisk:new-risk'));
      },
    },
    {
      key: '/',
      handler: () => {
        setShowShortcuts(false);
        setCmdkOpen(true);
      },
    },
    {
      key: 'g',
      handler: () => {
        setShowShortcuts(false);
        navigate('/');
      },
    },
    {
      key: 't',
      handler: () => {
        setShowShortcuts(false);
        toggleTheme();
      },
    },
  ]);

  return (
    <div className="flex h-screen bg-app text-ink overflow-hidden font-sans selection:bg-accent-soft">
      {/* Skip-to-content (WCAG 2.4.1): first focusable element, jumps past nav. */}
      <a href="#main-content" className="skip-link">
        {lang === 'fr' ? 'Aller au contenu principal' : 'Skip to main content'}
      </a>
      <Sidebar mobileOpen={mobileNavOpen} onMobileClose={() => setMobileNavOpen(false)} />
      <div
        className="flex-1 flex flex-col h-screen overflow-hidden relative min-w-0"
        style={{ background: 'var(--bg-primary)' }}
      >
        {/* Renders only when the server reports DEMO_MODE. Not dismissible by
            design — see shared/DemoBanner. */}
        <DemoBanner />
        <OfflineBanner />
        <AppHeader onOpenMobileNav={() => setMobileNavOpen(true)} />
        <main
          id="main-content"
          tabIndex={-1}
          data-testid="app-main"
          className="flex-1 overflow-hidden relative flex flex-col"
        >
          <Suspense fallback={<RouteFallback />}>
            <RoutePermissionGuard>
              <AnimatedOutlet />
            </RoutePermissionGuard>
          </Suspense>
        </main>
      </div>

      {/* Global shell overlays. The palette mounts only once opened. */}
      {cmdkOpen && (
        <Suspense fallback={null}>
          <CommandPalette />
        </Suspense>
      )}
      {/* Three non-blocking coach marks, shown once and replayable from the
          header's help button (spec §8). It never covers the app. */}
      <ProductTour />
      {/* The universal entity drawer (W1-02). Mounted ONCE for the whole app and
          driven entirely by the URL, which is what makes it a navigation
          primitive rather than a per-screen feature: a deep link opens it on any
          route, Back closes it, and the page behind keeps its filters. It
          renders nothing until the URL names an entity. */}
      <EntityDrawerHost />
      <ShortcutsOverlay open={showShortcuts} onClose={() => setShowShortcuts(false)} lang={lang} />
      {/* Only mounted when opened, and behind its own Suspense — so the form
          stack (react-hook-form + zod) loads on first use, not at app start. */}
      {newRiskOpen && (
        <Suspense fallback={null}>
          <CreateRiskModal
            isOpen={newRiskOpen}
            onClose={() => setNewRiskOpen(false)}
            onCreated={() => {
              void useRiskStore.getState().fetchRisks();
            }}
          />
        </Suspense>
      )}
    </div>
  );
};

/**
 * Redirects the legacy flat framework URL to its nested home.
 *
 * /compliance/<uuid> predates /compliance/frameworks/<uuid>; links to it exist
 * in saved reports and bookmarks. `replace` keeps it out of history so Back
 * from the framework page returns to Compliance rather than re-triggering the
 * redirect.
 */
function LegacyFrameworkRedirect() {
  const { frameworkId } = useParams<{ frameworkId: string }>();
  return <Navigate to={`/compliance/frameworks/${frameworkId ?? ''}`} replace />;
}

/**
 * Sends the legacy per-risk timeline URL to the register's Timeline tab.
 *
 * `replace` keeps it out of the history stack, so Back from the register goes
 * where the user came from rather than bouncing through the redirect.
 */
function LegacyRiskTimelineRedirect() {
  const { riskId } = useParams<{ riskId: string }>();
  return <Navigate to={`/risks?focus=${riskId ?? ''}&tab=timeline`} replace />;
}

/**
 * Per-route permission guard (frontend defence-in-depth over the backend 403).
 *
 * Permissions come from the route tree, the same declaration the breadcrumb
 * reads, so the guard, the trail and the sidebar cannot disagree about what a
 * page is. Unmapped paths fail open to the backend, which still enforces.
 */
function RoutePermissionGuard({ children }: { children: ReactNode }) {
  const { pathname } = useLocation();
  const { can } = usePermissions();
  const required = permissionFor(pathname);
  if (required && !can(required)) {
    return <AccessDenied permission={required} pathname={pathname} />;
  }
  return <>{children}</>;
}

/**
 * Fallback shown while a route's lazy chunk is being fetched. Kept minimal and
 * theme-aware — route chunks are small so this is only visible briefly.
 */
function RouteFallback() {
  return (
    <div
      className="flex-1 flex items-center justify-center"
      style={{ background: 'var(--bg-primary)' }}
    >
      <div
        className="h-8 w-8 rounded-full animate-spin"
        style={{
          border: '3px solid var(--border-subtle)',
          borderTopColor: 'var(--accent, #2e6be6)',
        }}
        role="status"
        aria-label="Chargement…"
      />
    </div>
  );
}

/**
 * COMPOSANT PRINCIPAL : APP ROUTER
 */
function App() {
  return (
    <BrowserRouter>
      {/* App-wide Suspense: the public auth screens are lazy now, so they need a
          boundary above the route table (the shell routes have their own too). */}
      <Suspense fallback={<RouteFallback />}>
        <Routes>
          {/* Routes Publiques */}
          <Route path="/login" element={<AuthScreen initialView="login" />} />
          <Route path="/register" element={<AuthScreen initialView="register" />} />
          {/* Password reset. Both are public: someone locked out has no session,
            and the reset link in the email lands directly on /reset-password. */}
          <Route path="/forgot-password" element={<ForgotPasswordScreen />} />
          <Route path="/reset-password" element={<ResetPasswordScreen />} />
          {/* Public status page (task §4) — reachable without a session. */}
          <Route path="/status" element={<StatusPage />} />
          {/* Invitation acceptance. Public by necessity: the person following the
            link may have no OpenRisk account yet, and a sign-in wall here would
            block exactly the people the link exists for. The screen previews
            the invitation before asking for anything, and the server binds it
            to the invited address. */}
          <Route path="/invitations/accept" element={<AcceptInvitationPage />} />

          {/* Signup wizard — authenticated but OUTSIDE the app shell: the point of
            these five screens is that nothing else competes for attention. The
            guard sends anyone who has already finished back to the app. */}
          <Route
            path="/onboarding"
            element={
              <ProtectedRoute>
                <Suspense fallback={<RouteFallback />}>
                  <OnboardingCompletedRedirect>
                    <OnboardingWizard />
                  </OnboardingCompletedRedirect>
                </Suspense>
              </ProtectedRoute>
            }
          >
            <Route index element={<Navigate to="/onboarding/organization" replace />} />
            <Route path="organization" element={<OrganizationStep />} />
            <Route path="profile" element={<ProfileStep />} />
            <Route path="goal" element={<GoalStep />} />
            <Route path="framework" element={<FrameworkStep />} />
            <Route path="team" element={<TeamStep />} />
            {/* #438 criterion 9 — the screen for a tenant that was already
              configured before the tunnel existed. Inside the wizard shell so it
              carries the same chrome, outside the step sequence because it is
              not a step: it is what replaces the whole sequence. */}
            <Route path="recognition" element={<RecognitionPage />} />
          </Route>

          {/* Routes Protégées (Layout Global) */}
          <Route
            element={
              <ProtectedRoute>
                {/* No /dashboard (nor any other app route) until onboarding is
                  finished — the server's onboarding.completed is the authority. */}
                <OnboardingGuard>
                  <DashboardLayout />
                </OnboardingGuard>
              </ProtectedRoute>
            }
          >
            <Route index element={<DashboardPage />} />
            {/* #438 — the Posture Reveal. In the app shell, not the wizard's:
              the tunnel ends here, but the user comes back to it. */}
            <Route path="posture" element={<PosturePage />} />
            {/* #433 — the whole queue. The dashboard panel is a preview of the
              top of this list; ?page= makes each page a shareable address. */}
            <Route path="action-center" element={<ActionCenterPage />} />

            {/* ---------------- Risks ---------------- */}
            <Route path="risks" element={<RiskRegisterPage />} />
            <Route path="risks/import" element={<ImportRisksPage />} />
            {/* The compliance-mapping backlog. Mapping stays optional at creation;
              this is where it gets caught up. */}
            <Route path="risks/unmapped" element={<UnmappedRisksPage />} />
            <Route path="risks/weighting" element={<RiskWeightsSettings />} />
            {/* The real risk history is the register drawer's Timeline tab: it
              goes through the shared API client (so it carries the session
              cookie and the CSRF header), reads the {timeline,total,count}
              envelope the endpoint actually returns, and has loading, empty and
              error states.

              This URL used to render a second, older implementation that did
              none of that. It hand-rolled a fetch with only a Bearer header —
              null after any reload, since the session moved to HttpOnly
              cookies — and handled exactly two outcomes, ok and 404. So an
              authentication failure fell through to "Total changes: 0", which
              is a screen telling an auditor this risk was never touched. On the
              happy path it was worse: it did setTimeline(data || []) against an
              object, then called .filter on it during render, so any risk WITH
              history white-screened.

              Redirected rather than repaired: two implementations of one screen
              is how the second one drifted in the first place (W0-05 / D8). */}
            <Route path="risks/:riskId/timeline" element={<LegacyRiskTimelineRedirect />} />
            {/* Mitigations sit under Risks: a mitigation exists only to reduce a
              risk, so its detail has an unambiguous parent to return to. */}
            <Route path="risks/mitigations" element={<MitigationsBoard />} />
            <Route path="risks/mitigations/:mitigationId" element={<MitigationDetailPage />} />

            {/* ---------------- Threats ---------------- */}
            <Route path="vulnerabilities" element={<VulnerabilitiesPage />} />
            <Route path="vulnerabilities/unassigned" element={<UnassignedVulnerabilitiesPage />} />
            <Route path="vulnerabilities/risk-rule" element={<RiskRulePage />} />
            <Route path="threat-map" element={<ThreatIntel />} />
            <Route path="ai/emerging-risks" element={<EmergingRisksPage />} />
            <Route path="simulations" element={<SimulationsPage />} />

            {/* ---------------- Incidents ---------------- */}
            <Route path="incidents" element={<IncidentsScreen />} />
            <Route path="incidents/sources" element={<IncidentSourcesPage />} />
            <Route path="incidents/:id/war-room" element={<WarRoom />} />

            {/* ---------------- Compliance ----------------
              Every sub-view is a route. They were reachable but had no rendered
              way back; now each one's parent is declared in the route tree and
              the breadcrumb derives from it. Static segments precede the
              parameterised ones so /compliance/audits is never read as a
              framework id. */}
            <Route path="compliance" element={<ComplianceScreen />} />
            <Route path="compliance/gaps" element={<GapAnalysisPage />} />
            {/* The evidence library and its worklist. Under /compliance because
              that is where people go looking for proof, even though evidence
              outlives any one framework. */}
            <Route path="compliance/evidence" element={<EvidenceLibraryPage />} />
            <Route path="compliance/evidence/missing" element={<MissingEvidencePage />} />
            <Route path="compliance/audits" element={<AuditsPage />} />
            <Route path="compliance/audits/:auditId" element={<AuditDetailPage />} />
            <Route path="compliance/remediation" element={<RemediationPage />} />
            <Route path="compliance/remediation/:planId" element={<RemediationDetailPage />} />
            <Route path="compliance/frameworks/:frameworkId" element={<FrameworkDetail />} />
            <Route path="compliance/frameworks/:frameworkId/gaps" element={<GapAnalysisPage />} />

            {/* ---------------- Assets ---------------- */}
            <Route path="assets" element={<InventoryPage />} />
            {/* The Asset Universe was superseded by the topology view (same graph,
              plus zoom/pan, zone clustering, typed edges, compromise chain and
              export). Kept as a redirect so existing links and bookmarks land
              on its replacement rather than a 404. */}
            <Route path="assets/universe" element={<Navigate to="/assets/topology" replace />} />
            <Route path="assets/schemas" element={<AssetSchemaSettings />} />
            <Route path="assets/topology" element={<TopologyView />} />
            <Route path="infrastructure" element={<InfrastructurePage />} />
            <Route path="infrastructure/scans/:jobId" element={<ScanPreviewPage />} />

            {/* ---------------- Analytics ---------------- */}
            {/* The dedicated score page. Reads the same query key as the dashboard
              hero and the sidebar footer, so all three render one object. */}
            <Route path="score" element={<ScorePage />} />
            <Route path="analytics/financial" element={<FinancialDashboard />} />
            {/* The tenant-wide activity feed (W1-02). RBAC filtering is the
              server's, so any authenticated member may open it — they simply see
              only what they may read. */}
            <Route path="activity" element={<GlobalTimelinePage />} />
            <Route path="leaderboard" element={<LeaderboardPage />} />

            {/* ---------------- Reports ----------------
              /reports/jobs/:id is where a generation request lands, which is what
              stops Reports and Compliance pointing at each other. Static
              segments first: /reports/jobs and /reports/board must not be eaten
              by /reports/:reportId. */}
            <Route path="reports" element={<ReportsScreen />} />
            {/* The reporting engine's own library: real documents, their state,
              their hash. Static segment, so it is never eaten by
              /reports/:reportId below. */}
            <Route path="reports/library" element={<ReportsLibraryPage />} />
            <Route path="reports/jobs/:jobId" element={<ReportJobPage />} />
            <Route path="reports/board" element={<BoardReportPage />} />
            <Route path="reports/:reportId" element={<BoardReportPage />} />
            <Route path="recommendations" element={<AiAdvisor />} />

            {/* ---------------- Automation / governance ---------------- */}
            <Route path="automation" element={<AutomationPage />} />
            <Route path="governance" element={<GovernancePage />} />
            <Route path="governance/audit-trail" element={<GovernancePage />} />

            {/* ---------------- Settings ---------------- */}
            <Route path="settings" element={<SettingsScreen />} />
            {/* Members owns invitations AND role assignment — one job, one screen.
              Splitting them is why "Invite a member" landed on Roles. */}
            <Route path="settings/members" element={<SettingsScreen />} />

            {/* ---------------- Moves and legacy deep links ----------------
              Permanent client-side redirects. `replace` keeps the old URL out of
              the history stack, so Back from the new location goes where the
              user came from rather than bouncing through the redirect. */}
            <Route path="mitigations" element={<Navigate to="/risks/mitigations" replace />} />
            <Route
              path="compliance/gap-analysis"
              element={<Navigate to="/compliance/gaps" replace />}
            />
            <Route
              path="compliance/remediations"
              element={<Navigate to="/compliance/remediation" replace />}
            />
            {/* Roles & access folded into Members (spec §5). */}
            <Route path="roles" element={<Navigate to="/settings/members" replace />} />
            <Route path="settings/roles" element={<Navigate to="/settings/members" replace />} />
            {/* The Settings audit tab duplicated the governance audit trail; one
              of the two had to be the real one, and governance owns it. */}
            <Route
              path="settings/audit-log"
              element={<Navigate to="/governance/audit-trail" replace />}
            />
            <Route path="audit-logs" element={<Navigate to="/governance/audit-trail" replace />} />
            {/* The executive dashboard is a display mode of the dashboard, not a
              report route (spec §5). */}
            <Route path="analytics" element={<Navigate to="/?view=executive" replace />} />
            <Route path="users" element={<Navigate to="/settings/members" replace />} />
            <Route path="tenants" element={<Navigate to="/settings" replace />} />
            <Route path="tokens" element={<Navigate to="/settings" replace />} />
            <Route path="marketplace" element={<Navigate to="/settings" replace />} />
            <Route path="custom-fields" element={<Navigate to="/settings" replace />} />
            <Route path="analytics/permissions" element={<Navigate to="/settings" replace />} />
            <Route path="risk-management" element={<Navigate to="/risks" replace />} />
            <Route path="bulk-operations" element={<Navigate to="/risks" replace />} />
            {/* Legacy framework deep links: /compliance/<uuid> -> /compliance/frameworks/<uuid>. */}
            <Route path="compliance/:frameworkId" element={<LegacyFrameworkRedirect />} />
          </Route>

          {/* Redirection par défaut */}
          {/* System pages: a real, branded 404 (not a silent redirect) and a
            maintenance screen deployers can route to. */}
          <Route path="/maintenance" element={<Maintenance />} />
          <Route path="*" element={<NotFound />} />
        </Routes>
      </Suspense>
    </BrowserRouter>
  );
}

export default App;
