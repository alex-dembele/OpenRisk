// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

// Package isolation holds the tenant-isolation coverage gate.
//
// Audit finding F-07. Tenant isolation is the existential risk of a multi-tenant
// GRC platform, and the project has already shipped six real cross-tenant leaks
// (fixed 2026-07-23). Every one of them was found by a human reading code, not
// by CI. The gate here changes that growth pattern: the route surface is derived
// from the router by AST, and any parameterised route with no recorded isolation
// decision fails the build.
//
// The gate deliberately does NOT assert that isolation works — it asserts that
// somebody has decided, in writing, how each route is isolated. That is a weaker
// claim than an end-to-end probe and it is stated as such, but it is the part
// that closes automatically over new code.
//
// Known blind spot: the gate tracks routes with an ID in the *path*, because
// that is the surface where a caller names someone else's resource. A route that
// takes the target id (or worse, the tenant) in the request *body* is an equally
// good IDOR surface and is invisible here. That is not hypothetical —
// POST /scanner/mitigations/auto-complete read its tenant from the payload and
// let any authenticated user complete another tenant's sub-action; it is
// unparameterised, so this gate never asked about it. It is now covered by
// handler/mitigation_autocomplete_isolation_test. Detecting the shape statically
// would mean parsing each handler's body struct, which is a larger change than
// the gate warrants; until then, body-addressed writes need a probe by hand.
package isolation

import "strings"

// Status records how a route's tenant isolation is accounted for.
type Status string

const (
	// Covered: an automated test asserts cross-tenant access is refused.
	Covered Status = "covered"

	// PublicByDesign: no tenant scoping applies. Unauthenticated or
	// pre-authentication surface.
	PublicByDesign Status = "public-by-design"

	// MachineAuthenticated: authenticated by a non-user credential (scanner agent
	// token, webhook secret) whose scope is the credential itself, not a tenant
	// session.
	MachineAuthenticated Status = "machine-authenticated"

	// SelfScoped: operates only on the caller's own identity, taken from the
	// signed token rather than from a path parameter.
	SelfScoped Status = "self-scoped"

	// Unreachable: route exists but cannot execute — its tables are deliberately
	// absent from AutoMigrate, so every call fails before reaching a query.
	Unreachable Status = "unreachable"

	// Pending: a known gap. Visible debt, not a silent hole.
	Pending Status = "pending"
)

// Decision is the recorded reasoning for one route pattern.
type Decision struct {
	// Pattern matches route paths with ":param" segments normalised to "{id}".
	// A trailing "/*" makes it a prefix match covering a module's subtree.
	Pattern string
	Status  Status
	// Evidence names the test or states the justification. Required — a decision
	// without a reason is indistinguishable from a guess.
	Evidence string
}

// decisions is the authoritative record.
//
// Adding a parameterised route without adding an entry here fails
// TestEveryParameterisedRouteHasAnIsolationDecision. That is the point: it
// converts "somebody must remember to test the new endpoint" into a build error.
var decisions = []Decision{
	// --- Public / pre-authentication -------------------------------------
	{"/api/v1/auth/oauth2/login/{id}", PublicByDesign,
		"provider name in path, no tenant resource touched; runs before a session exists"},
	{"/api/v1/auth/oauth2/callback/{id}", PublicByDesign,
		"provider name in path; issues the session rather than reading tenant data"},

	// --- Machine identities ----------------------------------------------
	{"/api/v1/vulnerabilities/webhook/{id}", MachineAuthenticated,
		"opaque per-integration webhook token resolves the tenant; the token IS the credential"},
	{"/api/v1/scanner/*", MachineAuthenticated,
		"agent-scoped token plus HMAC on push; tenant derives from the enrolled agent, not the caller"},

	// --- Caller's own identity -------------------------------------------
	{"/api/v1/auth/pat/{id}", SelfScoped,
		"PAT CRUD is scoped to the authenticated owner via claims, not the path"},
	{"/api/v1/auth/sessions/{id}", SelfScoped,
		"session revocation is scoped to the authenticated owner: GormSessionRepository.Revoke " +
			"filters id AND user_id, so a foreign session id affects 0 rows and reads back as " +
			"not-found rather than forbidden (covered by TestRevoke_CrossUser_IsNotFound)"},

	// --- Covered by automated isolation tests -----------------------------
	{"/api/v1/action-center", Covered,
		"repository/actioncenter_repository_test TestActionCenter_TenantIsolation seeds the same qualifying row in two tenants and asserts each of the SIX source queries (mitigations, risks, approvals, incidents, evidence, remediation plans) returns only the caller's; TestActionCenter_NilTenantIsRefused proves a zero tenant fails closed with ErrForbidden rather than returning an unscoped set, and TestActionCenter_BusinessRoleIsScopedToTheTenant proves the role driving what is shown is read per (user, tenant). The caller's tenant comes only from the JWT (handler.callerFromCtx), never from the query string. Note incidents.tenant_id is a VARCHAR, compared as a string on purpose"},
	{"/api/v1/assets/{id}", Covered,
		"application/asset get/update/delete cross-tenant tests + gorm_asset_repository_test"},
	{"/api/v1/assets/{id}/history", Covered,
		"application/asset list_asset_snapshots_test asserts tenant scoping"},
	{"/api/v1/asset-dependencies/{id}", Covered,
		"gorm_asset_dependency_repository_test: cross-tenant GetByID returns nil"},
	{"/api/v1/compliance/*", Covered,
		"application/compliance controls+evidences tests, gorm_compliance_repository_test, gorm_compliance_audit_repository_test, gorm_control_mapping_repository_test"},
	{"/api/v1/evidence/*", Covered,
		"gorm_evidence_repository_test: cross-tenant GetByID returns nil, cross-tenant Delete leaves the row, List is empty, and a forged link from another tenant never counts toward coverage; application/evidence TestGetAndDelete_AreTenantScoped and TestCreate_RejectsControlFromAnotherTenant cover the use-case gate on linking"},
	{"/api/v1/reports/{id}", Covered,
		"application/report: every read, download, transition and delete goes through repo.GetByID(tenantID, id), which returns ErrNotFound for another tenant's report; the SSE progress stream filters each event on BOTH tenant_id and report_id before writing it, so a shared channel cannot leak another tenant's job"},
	{"/api/v1/reports/{id}/*", Covered,
		"same GetByID gate as the parent: download, verify, versions, compare, comments and progress all resolve the report through the tenant-scoped read before touching bytes"},
	{"/api/v1/reports/jobs/{id}", Covered,
		"gorm_report_job_repository_test: cross-tenant GetByID is ErrNotFound, List is tenant-scoped, Update cannot cross tenants; application/reportjob TestGet_CrossTenant_IsNotFound"},
	{"/api/v1/reports/jobs/{id}/download", Covered,
		"same repository path as the job read — Download resolves the job through GetByID before serving any bytes, so a foreign id 404s before the artifact is touched"},
	{"/api/v1/incidents/*", Covered,
		"handler/incident_handler_test TestIncident_CrossTenant_TimelineAndActions (regression for the July IDOR)"},
	{"/api/v1/risks/{id}/timeline", Covered,
		"service/risk_timeline_service_test TestRiskTimeline_CrossTenant"},
	{"/api/v1/risks/{id}/timeline/*", Covered,
		"same ownsRisk gate as the parent timeline route; TestRiskTimeline_CrossTenant and _RecentChanges_ScopedToTenant cover the subpaths"},
	{"/api/v1/custom-fields/*", Covered,
		"service/custom_field_service_test TestCustomField_TenantIsolation"},
	{"/api/v1/teams/{id}", Covered,
		"handler/team_isolation_test TestTeam_TenantIsolationPredicate"},
	{"/api/v1/teams/*", Covered,
		"handler/team_isolation_test covers member add/remove paths"},
	{"/api/v1/users/{id}", Covered,
		"handler/user_isolation_test TestUser_TenantScoping"},
	{"/api/v1/users/*", Covered,
		"handler/user_isolation_test covers status/role/delete by id"},
	{"/api/v1/notifications/{id}", Covered,
		"repository/notification_repository_test scopes by user and tenant"},
	{"/api/v1/notifications/{id}/read", Covered,
		"repository/notification_repository_test scopes by user and tenant"},
	{"/api/v1/vulnerabilities/{id}", Covered,
		"application/vulnerability integrations_test: cross-tenant lookup returns 404"},

	// --- Universal entity drawer (W1-02) -------------------------------------
	// The drawer takes an arbitrary (type, id) pair off the URL, which makes it
	// the broadest id-forging surface in the product: one route reaches assets,
	// risks, vulnerabilities, controls, incidents, vendors and evidence. Every
	// read runs as entity.Caller, whose TenantID comes from the signed token and
	// cannot be influenced by the request; each resolver funnels through a single
	// load() onto the module's own tenant-scoped repository, and a row belonging
	// to another tenant is reported not-found — the same answer a fabricated id
	// gets, so the pair cannot be told apart.
	{"/api/v1/entities/{id}/{id}", Covered,
		"application/entity entity_test TestGet_CrossTenantIsNotFound covers all eight types; handler/entity_handler_test TestEntityDrawer_CrossTenantAccessDenied drives it through the real HTTP stack"},
	{"/api/v1/entities/{id}/{id}/relations", Covered,
		"application/entity entity_test TestRelations_AreTenantScoped: a relation naming another tenant's row is absent, and a group whose target type the caller cannot read comes back denied rather than populated"},
	{"/api/v1/entities/{id}/{id}/timeline", Covered,
		"application/entity timeline_test TestTimeline_CrossTenantIsNotFound + TestTenantTimeline_FiltersByPermission; the supplementary journals (risk_histories, incident_timelines) carry no tenant column and are gated through their parent entity"},
	{"/api/v1/entities/{id}/{id}/audit", Covered,
		"application/entity entity_test TestAudit_RequiresAuditPermission and TestAudit_CrossTenantIsNotFound"},
	{"/api/v1/vulnerabilities/*", Covered,
		"application/vulnerability integrations_test covers status/ticket subpaths"},
	{"/api/v1/onboarding/steps/{id}", Covered,
		"the :step param is a wizard step NAME from a closed vocabulary (domain.ParseOnboardingStep rejects anything else), never an entity id — there is no id in this path to forge. The row it writes is keyed by the caller's own (tenant, user) from the request context: gorm_activation_repository_test TestOnboardingProgress_MissingAndIsolated asserts a cross-tenant Get reads back nil, and application/activation TestWizard_OrganizationWriteRequiresPermission asserts a non-admin cannot write the organization through it"},

	// --- Unreachable ------------------------------------------------------
	{"/api/v1/marketplace/*", Unreachable,
		"Connector/MarketplaceApp carry no gorm tags and are excluded from AutoMigrate (main.go), so the tables do not exist and every call fails before querying. Documented latent IDOR — must be resolved before the module is revived"},

	// --- Known gaps -------------------------------------------------------
	// These execute against real tenant data and rely on repository-level
	// scoping that no test currently pins end to end.
	{"/api/v1/risks/{id}", Covered,
		"handler/risk_isolation_test drives read/update/transition/review/delete from tenant A at tenant B's risk over the real handler+repository chain; validated by sabotage"},
	{"/api/v1/risks/{id}/transition", Covered,
		"handler/risk_isolation_test TestRiskIsolation_CrossTenantAccessIsRefused/transition_phase"},
	{"/api/v1/risks/{id}/review", Covered,
		"handler/risk_isolation_test TestRiskIsolation_CrossTenantAccessIsRefused/mark_reviewed"},
	{"/api/v1/risks/{id}/transitions", Covered,
		"application/risk transition_state_test TestAvailableTransitions_NotFound: an id outside the tenant reads back as not-found through the same GetByID(tenant) path"},
	// The controlled category vocabulary. Both write routes go through
	// GormRiskCategoryRepository, which puts tenant_id in the WHERE clause of the
	// UPDATE and of the DELETE (and of the risk detach inside the same
	// transaction), so a forged id from another organisation affects 0 rows and
	// surfaces as not-found.
	{"/api/v1/risk-categories/{id}", Covered,
		"repository/gorm_risk_taxonomy_repository_test TestRiskCategoryRepo_CrossTenantUpdateAndDeleteRefused"},
	{"/api/v1/risks/*", Pending,
		"financial/smart-score/simulate/mitigations subpaths lack cross-tenant assertions"},
	{"/api/v1/mitigations/*", Pending,
		"sub-action paths verify ownership through the parent plan; not pinned by a test"},
	// Organization member management (W0-04). Every one of these resolves its
	// target through GormMembershipRepository, which carries organization_id in
	// the WHERE clause of every read AND every write — so a member or invitation
	// id from another organisation matches no row and reads back as not-found,
	// and a write aimed at one affects zero rows. The tenant itself is never in
	// the request: it comes from the session.
	{"/api/v1/organization/members/{id}", Covered,
		"application/membership service_test TestGetMember_CrossTenantIsIndistinguishableFromMissing + repository gorm_membership_repository_test TestMembershipRepo_GetAndSaveMember_CrossTenant; end-to-end in handler organization_member_isolation_test"},
	{"/api/v1/organization/members/{id}/role", Covered,
		"application/membership service_test TestChangeRole_RefusesEscalationAndLockout (cross-tenant branch) + handler organization_member_isolation_test"},
	{"/api/v1/organization/members/{id}/status", Covered,
		"application/membership service_test TestSetStatus_RefusesSelfOwnerAndCrossTenant + handler organization_member_isolation_test"},
	{"/api/v1/organization/invitations/{id}", Covered,
		"application/membership invitations_test TestInvitations_CrossTenantIsolation + repository TestMembershipRepo_Invitations_TenantIsolation"},
	{"/api/v1/organization/invitations/{id}/resend", Covered,
		"application/membership invitations_test TestInvitations_CrossTenantIsolation (resend branch)"},
	{"/api/v1/governance/*", Pending,
		"approvals/workflows/delegations are tenant-scoped in use cases; no cross-tenant test"},
	{"/api/v1/automation/rules/*", Pending,
		"ListEnabledByTrigger is tenant-scoped; per-rule routes lack cross-tenant assertions"},
	// Dry-run traces live in an in-process registry keyed by (id, tenant): Get and
	// Cancel both compare the entry's tenant to the caller's before doing
	// anything, so another organisation's trace reads back as absent.
	{"/api/v1/automation/dry-runs/{id}", Covered,
		"application/automation dryrun_test TestDryRunRegistry_CancelAndTenantScope"},
	{"/api/v1/automation/dry-runs/{id}/cancel", Covered,
		"application/automation dryrun_test TestDryRunRegistry_CancelAndTenantScope"},
	// Replay loads the execution through GetByID(id, tenant) and then the rule
	// through GetByID(id, tenant); a forged id from another organisation fails
	// the first lookup as not-found. Not yet pinned by a dedicated test.
	{"/api/v1/automation/executions/{id}/replay", Pending,
		"Replay resolves the execution and its rule through tenant-scoped GetByID; no cross-tenant assertion yet"},
	// Adopting a template reads from an in-code catalogue (no tenant data at all)
	// and writes a new rule stamped with the caller's tenant. There is nothing to
	// leak: the {key} is a catalogue key, not a record id.
	{"/api/v1/automation/templates/{id}/adopt", SelfScoped,
		"the path parameter is a static catalogue key, not a tenant record; the created rule is stamped with the caller's own tenant from the token"},
	{"/api/v1/reports/board/{id}", Pending,
		"GormBoardReportRepository is tenant-scoped; not pinned by a test"},
	{"/api/v1/reports/board/*", Pending,
		"approve/pdf subpaths lack cross-tenant assertions"},
	{"/api/v1/ai/*", Pending,
		"AI use cases read through tenant-scoped repos; no cross-tenant assertion on the id paths"},
	{"/api/v1/audit-logs/*", Pending,
		"admin-only; tenant scoping not pinned by a test"},
	{"/api/v1/rbac/*", Pending,
		"admin-only tenant/user/role admin paths; GetTenantUsers gates on level, not pinned"},
	{"/api/v1/tokens/*", Pending,
		"API token management; ownership checks not pinned by a test"},
	{"/api/v1/bulk-operations/{id}", Pending,
		"service filters by tenant (see bulk_operation_service); not pinned by a test"},
	{"/api/v1/integrations/{id}/test", Pending,
		"integration config is tenant-scoped; not pinned by a test"},
	{"/api/v1/cti/vulnerabilities/{id}", PublicByDesign,
		"CTI feed data (NVD/CISA) is global threat intelligence, not tenant-owned"},
	{"/api/v1/score-engine/*", Covered,
		"handler test TestScoreEngine_AssetLoadTenantScoped (regression for the July fail-open)"},

	// --- Attack Surface ------------------------------------------------------
	// The {id} here is an asset CATEGORY (server, vendor, …), not a record id, so
	// there is nothing cross-tenant to address: the tenant comes from the token
	// and the repository keys on (tenant_id, category).
	{"/api/v1/attack-surface/schemas/{id}", Covered,
		"application/assetschema: the tenant comes from the token; the path segment is a category, and the repository keys on (tenant_id, category)"},
	{"/api/v1/attack-surface/schemas/{id}/reset", Covered,
		"application/assetschema: same as above — deletes only the caller's tenant row"},
	{"/api/v1/attack-surface/topology/{id}/compromise-chain", Covered,
		"application/asset topology: the origin asset is loaded with GetByID(id, tenant) FIRST, so another tenant's id is a 404 before any graph is walked"},
	// =====================================================================
	// Collection routes — reads with no id in the path (#412 criterion 9)
	// =====================================================================
	//
	// These became visible to the gate only when it was extended past
	// parameterised routes. Until then GET /timeline/recent — which returned the
	// risk history of every tenant in the deployment — was a route CI had never
	// been able to ask a question about.
	//
	// Read the Pending entries below as what they say: NOT ASSESSED. They are
	// recorded so the surface is countable and so a new collection route cannot
	// be added in silence. None of them is a claim that the route is safe.

	// --- W1-02's own two routes: decided by name, never by inheritance -----
	{"/api/v1/timeline", Covered,
		"application/entity: TenantTimelineService.ForTenant reads audit_events filtered on the caller's tenant, taken from Caller alone. TestTenantTimeline_NeverCrossesTenants (service) and TestEntityDrawer_TenantTimelineIsScopedAndLinked (HTTP, real SQL) both seed a second tenant and assert its rows are absent"},
	{"/api/v1/entities", SelfScoped,
		"application/entity: Service.Catalogue returns the static type descriptors plus this caller's own permission flags (CanRead per type). It reads no tenant table, so there are no rows to leak; TestEntityContract_NoCollectionSerialisesAsNull exercises it"},

	// --- Public / pre-authentication --------------------------------------
	{"/api/v1/health", PublicByDesign, "liveness probe; returns no tenant data"},
	{"/api/v1/status", PublicByDesign, "public status page feed; returns no tenant data"},
	{"/metrics", PublicByDesign, "Prometheus scrape endpoint; carries no tenant rows and is not mounted on the authenticated router"},
	{"/api/v1/auth/saml2/metadata", PublicByDesign, "SAML service-provider metadata, published pre-authentication by design"},
	{"/api/v1/auth/saml2/login", PublicByDesign, "SAML authentication entry point; runs before any session exists"},

	// --- Self-scoped: the answer is a function of the caller's own token ---
	{"/api/v1/auth/me", SelfScoped, "returns the authenticated identity taken from the signed token, not from any parameter"},
	{"/api/v1/users/me", SelfScoped, "returns the authenticated caller own profile, keyed on the signed token user id rather than on any parameter"},
	{"/api/v1/gamification/me", SelfScoped, "the caller's own score and streak, keyed on the token's user id"},
	{"/api/v1/ownership/me", SelfScoped, "the caller's own ownership assignments, keyed on the token's user id"},

	// --- Assessed, #421 -----------------------------------------------------
	//
	// The 97 entries below replace one generic string that said, of every
	// collection route in the product, "not assessed". Each one now names the
	// query behind the route and either the test that pins it or the specific
	// thing that is still open. The two are not interchangeable: a Covered entry
	// names a test that seeds a SECOND tenant and asserts its rows are absent,
	// and a Pending entry states what is unresolved and what would settle it.
	//
	// One route was found to be leaking and is recorded as such below, not
	// quietly fixed and not quietly filed: GET /api/v1/audit-logs. See #532.
	//
	// The new tests written for this sweep were each validated by sabotage — the
	// tenant predicate was removed from the query under test and the test was
	// confirmed to fail. A cross-tenant test that passes against an unscoped
	// query is worse than no test, because it is read as coverage.

	// --- Governance (9) ---------------------------------------------------
	// All nine reach the database through GormAuditEventRepository,
	// GormAuditChainRepository, GormDelegationRepository, GormApprovalRepository
	// or GormAuditRetentionRepository. Every one of those puts tenant_id in the
	// WHERE clause, and the handler passes only the tenant from the signed token.
	{"/api/v1/governance/audit-events", Covered,
		"repository/governance_collection_isolation_test TestGovernanceCollections_NeverCrossTenants/audit_events_list seeds the same event in two tenants and asserts the caller sees only its own two"},
	{"/api/v1/governance/audit-events/export", Covered,
		"the export is UNPAGINATED (ListAll), the shape where a lost predicate hands over everything at once, so it is asserted separately from the paginated list: TestGovernanceCollections_NeverCrossTenants/audit_events_export_is_the_same_predicate"},
	{"/api/v1/governance/audit-events/verify", Covered,
		"walks this tenant's chain seals; TestGovernanceCollections_NeverCrossTenants/audit_chain_verification_reads_one_tenants_seals seeds seals in both tenants and asserts the verdict is built from one tenant's alone — reading another organisation's seal would make this tenant's tamper verdict a statement about somebody else's trail"},
	{"/api/v1/governance/audit-retention", Covered,
		"TestGovernanceCollections_NeverCrossTenants/audit_retention_policy writes a different retention window in each tenant and reads BOTH back — reading only one would pass against an unscoped Take() whenever that tenant's row happened to be inserted first"},
	{"/api/v1/governance/delegations", Covered,
		"TestGovernanceCollections_NeverCrossTenants/delegations: one delegation per tenant, and tenant A's listing contains only its own"},
	{"/api/v1/governance/delegations/effective", Covered,
		"resolves through ActiveDelegationsTo, a second query with its own WHERE; the same subtest asks tenant A to resolve a delegate id that exists only in tenant B and asserts it grants nothing. NOTE: the delegate_id query parameter lets a caller resolve ANOTHER USER's effective permissions inside their own tenant — that is an intra-tenant authorisation question, not a cross-tenant one, and is out of this issue's scope"},
	{"/api/v1/governance/workflows", Covered,
		"TestGovernanceCollections_NeverCrossTenants/workflows_and_approvals asserts both ListWorkflows and FindWorkflow — the latter is what decides whether a write needs sign-off, so another organisation's workflow routing this one would be a control failure as well as a disclosure"},
	{"/api/v1/governance/approvals", Covered,
		"TestGovernanceCollections_NeverCrossTenants/workflows_and_approvals: ListRequests returns tenant A's single request and not tenant B's"},
	{"/api/v1/governance/request-types", PublicByDesign,
		"returns domain.ApprovalRequestTypes(), a compiled-in vocabulary of decision kinds. It touches no table, so there is no row that could belong to a tenant"},

	// --- Analytics (8) ----------------------------------------------------
	// Every query runs through AnalyticsService.scoped(), which is
	// `db.Where(\"tenant_id = ?\", tenantID)`. The handler takes the tenant from
	// middleware.GetContext and nothing else.
	{"/api/v1/analytics/risks/metrics", Covered,
		"service/analytics_service_test TestAnalyticsService_GetRiskMetrics_TenantIsolation — the regression test for the leak where this service aggregated across ALL tenants"},
	{"/api/v1/analytics/risks/trends", Covered,
		"service/collection_routes_isolation_test TestAnalyticsCollections_NeverCrossTenants/risk_trends: tenant A owns one risk and tenant B three, and the last bucket's running total and average score are both over tenant A's register alone"},
	{"/api/v1/analytics/mitigations/metrics", Covered,
		"TestAnalyticsCollections_NeverCrossTenants/mitigation_metrics asserts in both directions — tenant B's completions are not counted as tenant A's, and tenant A's in-progress plan is not counted as tenant B's"},
	{"/api/v1/analytics/frameworks", Covered,
		"TestAnalyticsCollections_NeverCrossTenants/frameworks: the per-framework risk count sums to the tenant's own total"},
	{"/api/v1/analytics/dashboard", Covered,
		"TestAnalyticsCollections_NeverCrossTenants/dashboard_snapshot_and_export composes the four aggregates above and asserts the composed answer, rather than inferring it from its parts"},
	{"/api/v1/analytics/export", Covered,
		"the export handler calls GetDashboardSnapshot and re-encodes it, so it is the same object and the same assertion: TestAnalyticsCollections_NeverCrossTenants/dashboard_snapshot_and_export"},
	{"/api/v1/analytics/financial", Pending,
		"assessed, not pinned: GetFinancialSummary passes mwCtx.OrganizationID into FinancialSummaryUseCase.Execute and the request cannot influence it. What is unresolved is every query BELOW that call — the CRQ aggregation reads risks and their monetary drivers through its own path, which this sweep did not read. Settled by a two-tenant test over FinancialSummaryUseCase.Execute in the shape of TestAnalyticsCollections_NeverCrossTenants"},
	{"/api/v1/analytics/executive", Pending,
		"assessed, not pinned: the handler passes only the token's tenant, and the use case composes six sources (financial summary, risk repo, gap analysis, vuln repo, incident service, quantifier) that are individually tenant-scoped elsewhere. Unresolved: no test asserts the COMPOSED board, and a composition is exactly where one un-scoped source hides behind five scoped ones. Settled by a two-tenant test over ExecutiveDashboardUseCase.Execute asserting each section, not just the total"},

	// --- Automation (8) ---------------------------------------------------
	{"/api/v1/automation/rules", Covered,
		"repository/automation_collection_isolation_test TestAutomationCollections_NeverCrossTenants/rules asserts both List and ListEnabledByTrigger — the latter is what the engine consults on every event, so a leak there would run another tenant's rules against this tenant's data"},
	{"/api/v1/automation/executions", Covered,
		"TestAutomationCollections_NeverCrossTenants/executions: the run history is what fired, against which entity, and when — tenant A sees one row, tenant B's two are absent"},
	{"/api/v1/automation/sla", Covered,
		"TestAutomationCollections_NeverCrossTenants/sla_and_stats (ListOpen branch)"},
	{"/api/v1/automation/sla/stats", Covered,
		"TestAutomationCollections_NeverCrossTenants/sla_and_stats: the grouped count is over the caller's trackers alone"},
	{"/api/v1/automation/channels", Covered,
		"the one that carries secrets — a lost predicate here hands another organisation's webhook endpoints to whoever opens the settings page. TestAutomationCollections_NeverCrossTenants/channels seeds a distinct webhook URL per tenant and asserts the caller reads its own"},
	{"/api/v1/automation/channels/catalogue", Covered,
		"ChannelService.ConfiguredChannels resolves through the same GormAutomationChannelRepository.Get asserted by TestAutomationCollections_NeverCrossTenants/channels; the rest of the response is domain.AllAutomationChannels(), a compiled-in list"},
	{"/api/v1/automation/state", Covered,
		"RuleService.State is computed from the same tenant-scoped List asserted by TestAutomationCollections_NeverCrossTenants/rules"},
	{"/api/v1/automation/templates", PublicByDesign,
		"renders domain.AutomationTemplates(), a compiled-in playbook catalogue, into sentences. No table is read, so no tenant owns any of it — the same reasoning as the /templates/{id}/adopt entry above"},

	// --- Dashboard (7) ----------------------------------------------------
	// DashboardDataService.scoped() is the same `Where(\"tenant_id = ?\")` shape as
	// AnalyticsService.
	{"/api/v1/dashboard/metrics", Covered,
		"service/collection_routes_isolation_test TestDashboardCollections_NeverCrossTenants/metrics: tenant B's three risks are absent from tenant A's KPI tiles"},
	{"/api/v1/dashboard/risk-trends", Covered,
		"TestDashboardCollections_NeverCrossTenants/risk_trends"},
	{"/api/v1/dashboard/severity-distribution", Covered,
		"service/analytics_service_test TestDashboardDataService_SeverityDistribution_TenantIsolation. NOTE: this query filters on a `severity` column domain.Risk does not declare, so in production the GORM error is swallowed and every band reads zero. That is a counting defect, not an isolation defect — an endpoint that answers zero cannot leak — and it is out of #421's scope"},
	{"/api/v1/dashboard/mitigation-status", Covered,
		"TestDashboardCollections_NeverCrossTenants/mitigation_status asserts both directions. Same caveat as severity-distribution: the query matches lowercase status strings ('completed', 'in_progress') while the domain vocabulary is uppercase ('DONE', 'IN_PROGRESS'), so the tiles read zero in production. The test seeds what the query matches, because a counter stuck at zero cannot prove a predicate holds"},
	{"/api/v1/dashboard/mitigation-progress", Covered,
		"TestDashboardCollections_NeverCrossTenants/mitigation_progress"},
	{"/api/v1/dashboard/top-risks", Pending,
		"NOT a tenant-isolation gap and not claimable as covered either: GetTopRisks calls Preload(\"Team\") on domain.Risk, which declares no Team relation, so GORM refuses the query before it runs and this route answers 500 in every tenant on every request. There is no observable result to make an isolation claim about. TestDashboardCollections_NeverCrossTenants/top_risks_cannot_execute pins that refusal so the two facts stay linked. Settled by fixing the preload and then writing the isolation assertion the route will need — the tenant predicate is applied by scoped() before the preload, but that is unverified because the query cannot execute"},
	{"/api/v1/dashboard/complete", Pending,
		"composes the widgets above, GetTopRisks included, so it inherits that refusal wholesale and answers 500. TestDashboardCollections_NeverCrossTenants/complete_cannot_execute. Settled by the same fix"},

	// --- Organization (7) -------------------------------------------------
	{"/api/v1/organization/members", Covered,
		"repository/gorm_membership_repository_test TestMembershipRepo_ListMembers_TenantIsolationAndFilters: four members across two tenants, and the search filter is asserted not to cross the boundary either — searching for the other tenant's member by name returns nothing"},
	{"/api/v1/organization/invitations", Covered,
		"repository/gorm_membership_repository_test TestMembershipRepo_Invitations_TenantIsolation"},
	{"/api/v1/organization/counts", Covered,
		"repository/gorm_membership_repository_test TestMembershipRepo_Counts asserts tenant B's numbers are its own — a shared counter is how a sidebar leaks"},
	{"/api/v1/organization/members/audit", Covered,
		"Service.MembershipAudit reads through the same AuditEventRepository.List asserted by TestGovernanceCollections_NeverCrossTenants/audit_events_list, with an entity-type allowlist applied on top; the tenant comes from the token"},
	{"/api/v1/organization", Pending,
		"assessed, not pinned: GetOrganization refuses uuid.Nil with ErrUnauthorized and then reads orgs.GetByID(ctx, tenantID), keyed on the organisation's own primary key taken from the token — there is no id in the request to forge. Unresolved: no test asserts that a second organisation's row cannot come back. Settled by a two-tenant assertion on the organisation directory's GetByID"},
	{"/api/v1/organization/deletion", Pending,
		"assessed, not pinned: GetActive(ctx, tenantID(c)) with the tenant from the token. Unresolved: the orgdeletion store's query was not read in this sweep. Settled by a two-tenant test asserting one organisation's pending deletion request is invisible to another"},
	{"/api/v1/organization/export", Pending,
		"assessed, not pinned, and the highest-value single response in the product: Exporter.Export(ctx, tenantID(c)) writes a whole-organisation bundle to a file and serves it. Unresolved: the exporter walks many tables and this sweep did not read each one's predicate. Settled by a two-tenant test that seeds rows in both and asserts NO tenant B identifier appears anywhere in tenant A's bundle — asserted over the bundle's bytes, not over the row counts"},

	// --- Stats (6) --------------------------------------------------------
	// Named by #421 as the place to start, and rightly: unparameterised GETs
	// that aggregate a whole table for the dashboard. All six carry
	// `tenant_id = ?`, and all six now have an HTTP-level two-tenant test that
	// drives the real handler over the real tenant resolution.
	{"/api/v1/stats", Covered,
		"handler/dashboard_stats_test TestDashboardStats_IsTenantScopedIncludingTheTrend; GetDashboardStats also fails CLOSED with 401 when no tenant resolves, rather than querying every tenant"},
	{"/api/v1/stats/risk-matrix", Covered,
		"handler/stats_collection_isolation_test TestStatsCollections_NeverCrossTenants/risk_matrix"},
	{"/api/v1/stats/risk-distribution", Covered,
		"TestStatsCollections_NeverCrossTenants/risk_distribution"},
	{"/api/v1/stats/mitigation-metrics", Covered,
		"TestStatsCollections_NeverCrossTenants/mitigation_metrics"},
	{"/api/v1/stats/top-vulnerabilities", Covered,
		"TestStatsCollections_NeverCrossTenants/top_vulnerabilities"},
	{"/api/v1/stats/trends", Covered,
		"TestStatsCollections_NeverCrossTenants/trends asserts the AVERAGE, not just the count: tenant B's scores would move it, so an unscoped query cannot produce tenant A's number by accident"},

	// --- RBAC (5) ---------------------------------------------------------
	{"/api/v1/rbac/tenants", SelfScoped,
		"lists the organisations the CALLER belongs to: UserService.GetUserTenants filters on the token's user id, and each tenant is then fetched by an id that came out of that user's own memberships. The request carries no tenant"},
	{"/api/v1/rbac/business-roles", PublicByDesign,
		"returns domain.PermissionCatalog and domain.ListBusinessRoles(), both compiled in. No table is read"},
	{"/api/v1/rbac/users", Pending,
		"assessed, not pinned: UserService.GetTenantUsers puts `tenant_id = ?` in both the count and the page query, and the handler takes the tenant from c.Locals(\"tenantID\"), set by the auth middleware from the JWT. Unresolved: no test seeds a second tenant. Settled by a two-tenant assertion on GetTenantUsers"},
	{"/api/v1/rbac/users/stats", Pending,
		"same query as /rbac/users (GetTenantUsers with limit 1, read for its total) and the same gap. Settled by the same test"},
	{"/api/v1/rbac/roles", Pending,
		"assessed, not pinned, and the predicate is deliberately WIDER than tenant_id: RoleService.ListRoles matches `(tenant_id = ? OR is_predefined = true)`, so the built-in roles are shared across the deployment by design while custom roles are not. Unresolved: nothing asserts that a role belonging to another tenant and NOT marked predefined is absent, which is the case the OR branch could mask. Settled by a two-tenant test that seeds one predefined role and one custom role per tenant"},

	// --- Notifications (3) ------------------------------------------------
	// All three key on (user_id, tenant_id). The pair matters: the same person
	// legitimately belongs to several organisations, so the user id alone is not
	// the boundary.
	{"/api/v1/notifications", Covered,
		"repository/notification_repository_test TestNotificationRepositoryTenantIsolationReadAndDelete seeds one user in two tenants and asserts the listing carries one tenant's row"},
	{"/api/v1/notifications/unread-count", Covered,
		"repository/notification_repository_test TestNotificationRepository_UnreadCountAndPreferences_AreTenantScoped — the badge is loaded on every page, so a lost predicate here would count every tenant's unread notifications for everyone"},
	{"/api/v1/notifications/preferences", Covered,
		"same test: a preference written in one tenant must not read back in another, which would both disclose a setting and mis-deliver"},

	// --- Realtime (3) -----------------------------------------------------
	{"/api/v1/realtime/catalog", PublicByDesign,
		"the event catalogue, envelope version and transport limits — all compiled in. No tenant row is read"},
	{"/api/v1/realtime/events", Covered,
		"the SSE stream fails CLOSED on a zero tenant (401 rather than an unfiltered subscription), narrows the client's requested categories to what its permissions allow, and replays from a per-tenant durable log: repository/gorm_realtime_event_repository_test TestRealtimeRepo_ReplayNeverCrossesTheTenantBoundary and TestRealtimeRepo_ReplayFailsClosedWithoutATenant, plus handler/realtime_stream_e2e_test over the real stack"},
	{"/api/v1/realtime/stats", Pending,
		"no tenant ROWS leak, and that is not the whole question: alongside this tenant's own connection count the response carries the hub's DEPLOYMENT-WIDE counters — total connections, number of distinct tenants connected, buffered events. Any admin of any organisation can read how many other organisations are online. Unresolved: whether a tenant administrator may see deployment cardinality at all. Settled either by gating the route to a platform operator rather than the tenant's own admin role, or by dropping the three global fields and keeping tenant_connections"},

	// --- Reports (3) ------------------------------------------------------
	{"/api/v1/reports", Covered,
		"repository/gorm_report_repository_test TestReportRepo_TenantIsolation (List branch): the other tenant's listing is empty and its total is zero. A report is a document about a whole register, so one crossing the boundary discloses the register at once"},
	{"/api/v1/reports/jobs", Covered,
		"repository/gorm_report_job_repository_test TestReportJob_List_ScopedToTenant"},
	{"/api/v1/reports/types", PublicByDesign,
		"the report catalogue: template keys, versions, titles and formats, all compiled in and localised. No table is read"},

	// --- CTI (2) ----------------------------------------------------------
	{"/api/v1/cti/vulnerabilities", PublicByDesign,
		"the NVD/CISA feed is global threat intelligence, not tenant-owned — the same reasoning already recorded for /cti/vulnerabilities/{id}. Nothing in the response is derived from a tenant's data"},
	{"/api/v1/cti/stats", PublicByDesign,
		"three of the four counters are over the global CTI feed. The fourth — risks this tenant auto-created from CTI — IS tenant-scoped, on `tenant_id = ? AND source = 'cti_auto'`, and is computed only when the tenant local resolves, so an unresolved tenant yields zero rather than a global count"},

	// --- Mitigations (2) --------------------------------------------------
	{"/api/v1/mitigations", Pending,
		"assessed, not pinned: the handler refuses a zero tenant with 401 before touching the repository, and GormMitigationRepository.List opens with `tenant_id = ? AND deleted_at IS NULL` before any filter is applied. The `mine=true` filter takes the user from the authenticated context, so \"mine=<someone else>\" is not expressible. Unresolved: no test seeds a second tenant, and the filters are applied by string key onto the same query, which is where a future filter could widen it. Settled by a two-tenant test over List with each filter exercised"},
	{"/api/v1/mitigations/events", Pending,
		"assessed, not pinned, and structurally different from every other entry here: an SSE stream over a SHARED Redis channel (mitigation.auto_completed), filtered in Go per message on `evt.TenantID != tenantID`, with the tenant taken from a JWT validated out of the query string. The isolation is a comparison in the handler, not a WHERE clause — so a malformed payload that fails to unmarshal is skipped, but a payload published without a TenantID would compare against uuid.Nil. Unresolved: nothing asserts the filter. Settled by a test that publishes two tenants' events onto a stub bus and asserts one subscriber receives only its own"},

	// --- Onboarding and activation (3) ------------------------------------
	{"/api/v1/onboarding/state", Pending,
		"assessed, not pinned: the handler resolves (tenant, user) together and returns 401 unless both are present, then reads per (tenant, user). Unresolved: repository/gorm_activation_repository_test TestOnboardingProgress_MissingAndIsolated asserts a cross-tenant GET of the progress row reads back nil, but the STATE endpoint composes that row with counts drawn from other tables which this sweep did not read. Settled by a two-tenant test over onboarding.GetState asserting every composed field"},
	{"/api/v1/onboarding/suggestions", Pending,
		"assessed, not pinned: same (tenant, user) gate as /onboarding/state. The industry/country/goal query parameters steer a compiled-in suggestion set rather than a query. Unresolved: whether any part of the suggestion is computed from tenant rows. Settled by reading GetSuggestions' sources and asserting the tenant-derived ones"},
	{"/api/v1/activation/state", Pending,
		"assessed, not pinned: same (tenant, user) gate, 401 when either is missing. Unresolved: the activation state aggregates progress across several modules and this sweep did not read each source. Settled by a two-tenant test over the activation state use case"},

	// --- Risks and scoring (5) --------------------------------------------
	{"/api/v1/risks", Covered,
		"handler/risk_isolation_test TestRiskIsolation_ListLeaksNothing drives GET /risks over the real handler and repository from one tenant against another's register; TestRiskIsolation_NilTenantIsDenied covers the unresolved-tenant case"},
	{"/api/v1/risks/unmapped", Pending,
		"assessed, not pinned: ListUnmapped takes the tenant from the token, and repository/gorm_risk_taxonomy_repository_test TestRiskControlMappingRepo_EnrichesAndScopesToTenant covers the mapping repository's scoping generally. Unresolved: TestRiskControlMappingRepo_UnmappedExcludesMappedAndClosed asserts WHICH risks are unmapped but does not seed a second tenant, so the tenant predicate on THIS query is untested. Settled by adding a second tenant to that test"},
	{"/api/v1/risk-categories", Pending,
		"assessed, not pinned: GormRiskCategoryRepository.List opens with `tenant_id = ?`, and TestRiskCategoryRepo_SlugIsUniquePerTenantOnly proves the table holds two tenants' categories at once. Unresolved: no test reads the list as one tenant while the other's categories exist — TestRiskCategoryRepo_ListHidesInactiveByDefault seeds a single tenant. Settled by adding a second tenant's category to that test and asserting it is absent"},
	{"/api/v1/risk-scoring/weights", Pending,
		"assessed, not pinned: the handler passes mwCtx.OrganizationID (uuid.Nil when absent) into GetRiskWeights. Unresolved: whether the weights store falls back to a shared default row on a miss, which would be correct behaviour but must not be reachable by passing a zero tenant. Settled by a two-tenant test that also asserts what uuid.Nil returns"},
	{"/api/v1/score", Pending,
		"assessed, not pinned: fails CLOSED with 401 on uuid.Nil before anything is read, and the optional `id` query parameter is scoped by the `scope` vocabulary (tenant|risk|asset). Unresolved: for scope=risk and scope=asset the id names a record, so this route has a parameterised route's IDOR surface in a query string — the shape this gate does not model. Settled by a test that asks, as tenant A with scope=risk, for a risk id belonging to tenant B and asserts not-found"},
	{"/api/v1/score/model", PublicByDesign,
		"returns scoring.Describe() — the frozen formula, its bands and its factor names. Requires a session (401 on a zero tenant) but reads no table, so there is nothing tenant-owned in the response"},

	// --- Assets and attack surface (7) ------------------------------------
	{"/api/v1/assets", Covered,
		"the inventory list, one of the largest tenant collections in the product. ListAssetsUseCase.Search delegates to GormAssetRepository.List, which is `Where(\"tenant_id = ?\")`; repository/gorm_asset_repository_test TestAssetRepository_List_ScopedToTenant seeds both tenants and asserts each sees only its own. The category and attr.* filters are applied in memory AFTER that scoped read, so they can only narrow it"},
	{"/api/v1/assets/statistics", Covered,
		"repository/gorm_asset_statistics_test TestAssetStatistics_IsTenantScoped, plus TestAssetStatistics_RefusesWithoutTenant — the repository refuses uuid.Nil outright rather than emitting a predicate that matches nothing"},
	{"/api/v1/asset-dependencies", Covered,
		"repository/gorm_asset_dependency_repository_test TestDepRepo_CreateAndListByTenant_Isolation"},
	{"/api/v1/attack-surface/topology/edge-types", PublicByDesign,
		"returns domain.TopologyEdgeTypes, a compiled-in vocabulary of edge kinds. No table is read"},
	{"/api/v1/attack-surface/topology", Pending,
		"assessed, not pinned: GetTopologyUseCase refuses uuid.Nil with ErrForbidden, then builds the graph from assets.List(tenant) and deps.ListByTenant(tenant) — both of which ARE covered by the two tests named on /assets and /asset-dependencies above. Unresolved: the graph is assembled from those two reads plus further sources this sweep did not enumerate, and a node that entered the graph from an unscoped source would not be caught by testing its inputs. Settled by a two-tenant test over GetTopologyUseCase.Execute asserting no foreign node or edge id appears"},
	{"/api/v1/attack-surface/schemas", Pending,
		"assessed, not pinned: GormAssetSchemaRepository carries tenant_id in every query, and the per-category sibling /attack-surface/schemas/{id} is already recorded Covered on that basis. Unresolved: the LIST query has no test of its own. Settled by extending the assetschema test to seed a schema in a second tenant and assert the listing omits it"},
	{"/api/v1/attack-surface/risk-rule", Pending,
		"assessed, not pinned: GormVulnRiskRuleRepository.Get is `Where(\"tenant_id = ?\").First(...)`, one rule per tenant. Unresolved: First() on an unscoped query returns an arbitrary row, so this is precisely the shape where a single-row read hides a missing predicate behind plausible output. Settled by a two-tenant test that writes a DIFFERENT rule in each and asserts each reads back its own"},
	{"/api/v1/attack-surface/draft-risks", Pending,
		"assessed, not pinned: ListDrafts passes the token's tenant into a paginated draft listing. Unresolved: the draft store's query was not read in this sweep, and the page/limit parameters are taken from the query string without an upper bound, so an unscoped version would be enumerable in bulk. Settled by a two-tenant test over the draft list"},

	// --- Audit logs (1) — CONFIRMED CROSS-TENANT LEAK ---------------------
	{"/api/v1/audit-logs", Pending,
		"LEAKING. Not a gap in coverage — a gap in the data model. domain.AuditLog carries NO tenant_id column, and AuditService.GetAuditLogsByDateRange filters on `timestamp BETWEEN ? AND ?` alone, so this route returns the authentication and authorization log of EVERY tenant in the deployment: user ids, IP addresses, actions, resource ids, error messages, user agents. The adminRole guard asks whether the caller is an admin of their OWN organisation, which every customer's admin is; it gates who may call, not whose rows come back. This is the /timeline/recent shape (docs/JOURNAL.md item 36) and for the same underlying reason — a journal table with no tenant column. Filed as its own P0: #532. Settled by that issue: a tenant_id column, a migration, the predicate on all four read queries, a stated fail-closed rule for pre-auth events, and a two-tenant test"},

	// --- Caller's own identity (5) ----------------------------------------
	{"/api/v1/auth/organizations", SelfScoped,
		"lists the organisations the caller belongs to, keyed on mwCtx.UserID from the signed token; returns 401 rather than a global list when no user resolves. There is no tenant in the request to forge"},
	{"/api/v1/auth/sessions", SelfScoped,
		"the caller's own devices, keyed on mwCtx.UserID from the token; 401 when absent. The revoke sibling is already recorded SelfScoped with its cross-user test (TestRevoke_CrossUser_IsNotFound)"},
	{"/api/v1/auth/pat", SelfScoped,
		"ListUserTokens(callerUserID) — the caller's own personal access tokens, keyed on the token's user id; 401 when absent"},
	{"/api/v1/tokens", SelfScoped,
		"TokenService.ListTokens(callerUserID) — the caller's own API tokens. Note this listing is scoped by USER and not by tenant, so a person who belongs to two organisations sees all their own tokens in one list regardless of the session's organisation. That is a deliberate consequence of tokens belonging to a person rather than to a membership; it discloses nothing across the tenant boundary, because the person owns every row either way"},
	{"/api/v1/ownership/assignable", Pending,
		"assessed, not pinned: ListAssignableUseCase refuses uuid.Nil with ErrValidation and reads members.ListMembers(ctx, tenantID); the search, permission and only_capable filters narrow that set in memory afterwards. Unresolved: this is a DIFFERENT ListMembers from the paginated one covered by TestMembershipRepo_ListMembers_TenantIsolationAndFilters, so that test does not reach it. Settled by a two-tenant assertion on the unpaginated ListMembers"},

	// --- Public / pre-authentication (3) ----------------------------------
	{"/api/v1/invitations/preview", PublicByDesign,
		"mounted outside the JWT gate on purpose, so an invitee with no account can see what they were invited to. Addressed by the invitation TOKEN and nothing else — there is no tenant in the request, and the response is derived from the row that token resolves to. The decision this replaces asked whether the token alone is sufficient addressing: it is, on the same basis as the webhook entries above — the token IS the credential, it is single-use, and it names exactly one invitation"},
	{"/api/v1/ai/status", PublicByDesign,
		"reports whether this DEPLOYMENT has an LLM backend and which model. No table is read and no tenant appears in the answer"},
	{"/api/v1/vulnerability-connectors", PublicByDesign,
		"returns vulnscan.Connectors(), the compiled-in list of connector kinds this build supports. Reads no table"},

	// --- Remaining single routes (7) --------------------------------------
	{"/api/v1/export/pdf", Covered,
		"handler/stats_collection_isolation_test TestStatsCollections_NeverCrossTenants/export_pdf inflates the generated document's content streams and asserts no other tenant's risk title was rendered into it — the assertion is on the artifact the user receives, because that is where this leak would be real"},
	{"/api/v1/bulk-operations", Covered,
		"service/bulk_operation_isolation_test TestBulkOperation_ListIsTenantScoped"},
	{"/api/v1/security/mfa-policy", Covered,
		"repository/gorm_mfa_policy_repository_test TestMFAPolicyRepo_IsTenantScoped, with TestMFAPolicyRepo_UnsavedTenantReadsAsNil and TestMFAPolicyRepo_RefusesOutOfRangeAndMissingTenant covering the empty and zero-tenant cases"},
	{"/api/v1/search", Pending,
		"assessed, not pinned, and the broadest single read in the product: one query fans out to risks, assets, vulnerabilities, controls, audits, board reports and users. It refuses uuid.Nil before dispatching anything (application/search TestGlobalSearch_NilTenant) and every source port takes tenantID. Unresolved: TestGlobalSearch_* runs against stub sources, so it proves the fan-out passes the tenant along and proves nothing about the seven real queries behind it. Settled by a two-tenant test against the real repositories — and it is worth doing here rather than per-source, because search is where one un-scoped source out of seven would be least visible"},
	{"/api/v1/reports/board", Pending,
		"assessed, not pinned: BoardReportHandler.List passes tenantID(c) into the use case, and GormBoardReportRepository is described as tenant-scoped by the existing /reports/board/{id} entry. Unresolved: neither the parameterised route nor this one has a test — the {id} entry says so itself. Settled by one two-tenant test over the board report repository covering GetByID and List together"},
	{"/api/v1/billing", Pending,
		"assessed, not pinned: BillingService.Get(ctx, tenantID(c)) returns the subscription and INVOICES for the tenant taken from the token. Unresolved: invoices are financial records of another company, so this is a high-consequence list with no test; the billing store's query was not read in this sweep. Settled by a two-tenant test asserting neither the subscription nor any invoice of the other organisation is returned"},
	{"/api/v1/entitlements", Pending,
		"assessed, not pinned: EntitlementService.Resolve(ctx, tenantID(c)). Unresolved: resolution composes the plan with per-tenant overrides, and an override read without a predicate would grant one tenant another's plan rather than disclose data — a licensing failure rather than a privacy one, but a failure. Settled by a two-tenant test asserting the resolved snapshot differs when the two tenants' plans differ"},
	{"/api/v1/telemetry", Pending,
		"instance-level, not tenant-level, and recorded here rather than dismissed: TelemetryRepository.GetOrCreate takes NO tenant and returns the deployment's single telemetry row — consent state and instance_id — to any authenticated member of any organisation. No tenant rows leak. What is unresolved is whether a tenant's user may read a deployment identifier and the operator's consent choice at all, given the PUT sibling is restricted to admin/root. Settled either by gating the GET the same way, or by dropping instance_id from the tenant-facing response"},
}

// Normalise rewrites a concrete route path into registry pattern form, so
// ":id", ":riskId" and ":userId" all collapse onto "{id}".
func Normalise(path string) string {
	segments := strings.Split(path, "/")
	for i, segment := range segments {
		if strings.HasPrefix(segment, ":") {
			segments[i] = "{id}"
		}
	}
	return strings.Join(segments, "/")
}

// Lookup finds the decision for a route path, preferring the most specific
// match so a precise entry always beats a module-wide prefix.
//
// "Most specific" is not the same as "longest". A wildcard also matches its own
// bare prefix — matches("/api/v1/risks/*", "/api/v1/risks") is true — and the
// wildcard is the longer string, so a length-only comparison hands the decision
// for GET /api/v1/risks to the module-wide entry and silently discards the exact
// one written for it. That is how a route can be assessed, given its own entry,
// and still report as unassessed. So an exact pattern wins outright, and length
// only breaks ties between patterns of the same kind (#421).
func Lookup(path string) (Decision, bool) {
	normalised := Normalise(path)

	var best Decision
	var found bool
	for _, d := range decisions {
		if !matches(d.Pattern, normalised) {
			continue
		}
		if !found || moreSpecific(d.Pattern, best.Pattern) {
			best, found = d, true
		}
	}
	return best, found
}

// moreSpecific reports whether pattern a describes a narrower surface than b.
func moreSpecific(a, b string) bool {
	aWild := strings.HasSuffix(a, "/*")
	bWild := strings.HasSuffix(b, "/*")
	if aWild != bWild {
		return !aWild // an exact pattern always beats a wildcard
	}
	return len(a) > len(b)
}

func matches(pattern, path string) bool {
	if prefix, isWildcard := strings.CutSuffix(pattern, "/*"); isWildcard {
		return path == prefix || strings.HasPrefix(path, prefix+"/")
	}
	return pattern == path
}

// Decisions exposes the registry for reporting.
func Decisions() []Decision {
	out := make([]Decision, len(decisions))
	copy(out, decisions)
	return out
}
