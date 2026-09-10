// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package domain

// ============================================================================
// BUSINESS ROLES — canonical, least-privilege GRC job roles
// ============================================================================
//
// The runtime authorization path is: OrganizationMember.Role (root|admin|user)
// → login expands to the JWT `permissions` []string → middleware.RequirePermission
// gates each route. root/admin get the "*" wildcard; a "user" member historically
// got permissions ONLY from an attached Profile, whose narrow resource/action
// vocabulary did not match the strings the routes actually check (risks:create,
// compliance:frameworks:read, vulnerabilities:read, scanner:*, automation:*,
// reports:board:*, incidents:*, …). A "user" member was therefore locked out of
// most of the app.
//
// Business roles close that gap: each is a named, least-privilege PRESET that
// expands to the exact permission strings the route guards check. They are the
// GRC job functions the product targets (RSSI/CISO, DSI/CIO, Risk Manager,
// Auditor, Compliance Officer, Internal Control, Asset Owner, Risk Owner,
// Security Analyst, Executive, Viewer). Assigning a business role to a "user"
// member is how a tenant admin grants scoped, coherent access without touching
// code.
//
// This file is PURE (stdlib only, no GORM/Fiber imports) so the catalog and
// presets are trivially unit-testable and shared verbatim between backend
// resolution and the API that feeds the frontend RBAC matrix.

// PermissionKey is a canonical "resource:action" (or "resource:sub:action")
// permission string, matching exactly what middleware.RequirePermission checks.
type PermissionKey = string

// PermissionGroup buckets permissions by product domain, for the RBAC matrix UI.
type PermissionGroup string

const (
	PermGroupRisks       PermissionGroup = "risks"
	PermGroupAssets      PermissionGroup = "assets"
	PermGroupMitigations PermissionGroup = "mitigations"
	PermGroupVulns       PermissionGroup = "vulnerabilities"
	PermGroupIncidents   PermissionGroup = "incidents"
	PermGroupCompliance  PermissionGroup = "compliance"
	PermGroupScanner     PermissionGroup = "scanner"
	PermGroupAutomation  PermissionGroup = "automation"
	PermGroupReports     PermissionGroup = "reports"
	// PermGroupOrg covers administering the organization itself: its members,
	// their invitations, the membership audit trail and the org profile.
	PermGroupOrg PermissionGroup = "organization"

	// PermGroupRealtime covers the server-sent event streams the UI subscribes
	// to. It is a real gate: /api/v1/realtime/events is mounted behind
	// RequirePermission("events:read") (main.go), and the mitigation stream
	// validates the same token itself because native EventSource cannot send an
	// Authorization header.
	PermGroupRealtime PermissionGroup = "realtime"

	// PermGroupGovernance covers the immutable audit trail and the approval
	// machinery. Its routes are guarded by the admin ROLE today, so an admin
	// already passes through "*"; the strings exist so an auditor or an internal
	// controller can be granted the audit trail without being handed the tenant.
	PermGroupGovernance PermissionGroup = "governance"
)

// PermissionDef documents one permission string with bilingual labels so the
// frontend can render a self-describing permission matrix.
type PermissionDef struct {
	Key     PermissionKey   `json:"key"`
	Group   PermissionGroup `json:"group"`
	LabelFR string          `json:"label_fr"`
	LabelEN string          `json:"label_en"`
}

// PermissionCatalog is the exhaustive list of permission strings the API gates
// on. Keep it in sync with the RequirePermission(...) call sites in main.go.
// ValidateBusinessRoles() (see below, exercised by tests) asserts every preset
// only references keys that exist here.
var PermissionCatalog = []PermissionDef{
	// Risks
	{"risks:read", PermGroupRisks, "Lire les risques", "Read risks"},
	{"risks:create", PermGroupRisks, "Créer des risques", "Create risks"},
	{"risks:update", PermGroupRisks, "Modifier les risques", "Update risks"},
	{"risks:delete", PermGroupRisks, "Supprimer des risques", "Delete risks"},
	// Assets
	{"assets:read", PermGroupAssets, "Lire les actifs", "Read assets"},
	{"assets:create", PermGroupAssets, "Créer des actifs", "Create assets"},
	{"assets:update", PermGroupAssets, "Modifier les actifs", "Update assets"},
	{"assets:delete", PermGroupAssets, "Supprimer des actifs", "Delete assets"},
	// Mitigations
	{"mitigations:read", PermGroupMitigations, "Lire les plans de traitement", "Read mitigations"},
	{"mitigations:create", PermGroupMitigations, "Créer des plans de traitement", "Create mitigations"},
	{"mitigations:update", PermGroupMitigations, "Modifier les plans de traitement", "Update mitigations"},
	{"mitigations:delete", PermGroupMitigations, "Supprimer des plans de traitement", "Delete mitigations"},
	// Vulnerabilities
	{"vulnerabilities:read", PermGroupVulns, "Lire les vulnérabilités", "Read vulnerabilities"},
	{"vulnerabilities:update", PermGroupVulns, "Traiter les vulnérabilités", "Update vulnerabilities"},
	{"vulnerabilities:delete", PermGroupVulns, "Supprimer des vulnérabilités", "Delete vulnerabilities"},
	// Incidents
	{"incidents:read", PermGroupIncidents, "Lire les incidents", "Read incidents"},
	{"incidents:create", PermGroupIncidents, "Déclarer des incidents", "Create incidents"},
	{"incidents:update", PermGroupIncidents, "Gérer les incidents", "Update incidents"},
	{"incidents:delete", PermGroupIncidents, "Supprimer des incidents", "Delete incidents"},
	// Compliance
	{"compliance:read", PermGroupCompliance, "Lire la conformité", "Read compliance"},
	{"compliance:frameworks:read", PermGroupCompliance, "Lire les référentiels", "Read frameworks"},
	{"compliance:frameworks:create", PermGroupCompliance, "Créer des référentiels", "Create frameworks"},
	{"compliance:frameworks:delete", PermGroupCompliance, "Supprimer des référentiels", "Delete frameworks"},
	{"compliance:controls:read", PermGroupCompliance, "Lire les contrôles", "Read controls"},
	{"compliance:controls:create", PermGroupCompliance, "Créer des contrôles", "Create controls"},
	{"compliance:controls:update", PermGroupCompliance, "Modifier les contrôles", "Update controls"},
	{"compliance:controls:delete", PermGroupCompliance, "Supprimer des contrôles", "Delete controls"},
	{"compliance:evidences:read", PermGroupCompliance, "Lire les preuves", "Read evidences"},
	{"compliance:evidences:create", PermGroupCompliance, "Ajouter des preuves", "Add evidences"},
	{"compliance:evidences:delete", PermGroupCompliance, "Supprimer des preuves", "Delete evidences"},
	{"compliance:audits:read", PermGroupCompliance, "Lire les audits", "Read audits"},
	{"compliance:audits:write", PermGroupCompliance, "Gérer les audits", "Manage audits"},
	{"compliance:remediations:read", PermGroupCompliance, "Lire les plans de remédiation", "Read remediations"},
	{"compliance:remediations:write", PermGroupCompliance, "Gérer les plans de remédiation", "Manage remediations"},
	// Scanner / infrastructure discovery
	{"scanner:read", PermGroupScanner, "Consulter les scans", "Read scans"},
	{"scanner:create", PermGroupScanner, "Configurer des scans", "Configure scans"},
	{"scanner:scan", PermGroupScanner, "Lancer des scans", "Trigger scans"},
	{"scanner:import", PermGroupScanner, "Importer les découvertes", "Import discoveries"},
	{"scanner:delete", PermGroupScanner, "Supprimer des scans", "Delete scans"},
	// Automation / SOAR
	{"automation:read", PermGroupAutomation, "Consulter l'automatisation", "Read automation"},
	{"automation:write", PermGroupAutomation, "Gérer l'automatisation", "Manage automation"},
	// Board reports
	{"reports:board:read", PermGroupReports, "Lire les rapports Comex", "Read board reports"},
	{"reports:board:create", PermGroupReports, "Générer des rapports Comex", "Create board reports"},
	{"reports:board:update", PermGroupReports, "Éditer les rapports Comex", "Edit board reports"},
	{"reports:board:approve", PermGroupReports, "Approuver les rapports Comex", "Approve board reports"},
	{"reports:board:delete", PermGroupReports, "Supprimer des rapports Comex", "Delete board reports"},
	// Organization administration (W0-04). Admins hold "*" and so already pass
	// every one of these; the strings exist so a scoped business role CAN be
	// granted member administration without being handed the whole tenant.
	{"organization:read", PermGroupOrg, "Voir le profil de l'organisation", "View organization profile"},
	{"organization:update", PermGroupOrg, "Modifier le profil de l'organisation", "Update organization profile"},
	{"organization:members:read", PermGroupOrg, "Voir les membres", "View members"},
	{"organization:members:invite", PermGroupOrg, "Inviter des membres", "Invite members"},
	{"organization:members:update", PermGroupOrg, "Modifier le rôle des membres", "Change member roles"},
	{"organization:members:deactivate", PermGroupOrg, "Désactiver ou révoquer des membres", "Deactivate or revoke members"},
	{"organization:audit:read", PermGroupOrg, "Consulter l'audit des accès", "Read membership audit"},
	// Realtime. Every business role preset already grants this, and
	// /api/v1/realtime/events already enforces it — the key was simply missing
	// from the catalog, so ValidateBusinessRoles rejected all eleven presets and
	// the RBAC matrix could not show a permission the API was really applying.
	{"events:read", PermGroupRealtime, "Recevoir les événements temps réel", "Receive real-time events"},
	// Governance. The entity drawer's audit tab (W1-02) gates on this: the
	// timeline says what changed, the audit trail carries the before/after
	// snapshot, the actor's IP and the request id, and those are not the same
	// disclosure.
	{"governance:audit:read", PermGroupGovernance, "Consulter la piste d'audit", "Read the audit trail"},
}

// catalogIndex is a fast membership set over PermissionCatalog keys.
var catalogIndex = func() map[PermissionKey]struct{} {
	m := make(map[PermissionKey]struct{}, len(PermissionCatalog))
	for _, p := range PermissionCatalog {
		m[p.Key] = struct{}{}
	}
	return m
}()

// IsCatalogPermission reports whether key is a known, gate-able permission.
func IsCatalogPermission(key PermissionKey) bool {
	_, ok := catalogIndex[key]
	return ok
}

// ============================================================================
// BUSINESS ROLE PRESETS
// ============================================================================

// BusinessRoleKey identifies a preset. Stored on OrganizationMember.BusinessRole.
type BusinessRoleKey string

const (
	BusinessRoleRSSI              BusinessRoleKey = "rssi"
	BusinessRoleDSI               BusinessRoleKey = "dsi"
	BusinessRoleRiskManager       BusinessRoleKey = "risk_manager"
	BusinessRoleAuditor           BusinessRoleKey = "auditor"
	BusinessRoleComplianceOfficer BusinessRoleKey = "compliance_officer"
	BusinessRoleInternalControl   BusinessRoleKey = "internal_control"
	BusinessRoleAssetOwner        BusinessRoleKey = "asset_owner"
	BusinessRoleRiskOwner         BusinessRoleKey = "risk_owner"
	BusinessRoleSecurityAnalyst   BusinessRoleKey = "security_analyst"
	BusinessRoleExecutive         BusinessRoleKey = "executive"
	BusinessRoleViewer            BusinessRoleKey = "viewer"
)

// BusinessRole is a named, least-privilege permission preset for a GRC job role.
type BusinessRole struct {
	Key           BusinessRoleKey `json:"key"`
	LabelFR       string          `json:"label_fr"`
	LabelEN       string          `json:"label_en"`
	DescriptionFR string          `json:"description_fr"`
	DescriptionEN string          `json:"description_en"`
	Permissions   []PermissionKey `json:"permissions"`
	// DefaultLanding is the route the frontend redirects to after login for this
	// role, so each profession lands on a screen relevant to its work.
	DefaultLanding string `json:"default_landing"`
}

// businessRoles is the ordered catalog of presets. Order drives UI listing.
var businessRoles = []BusinessRole{
	{
		Key:            BusinessRoleRSSI,
		LabelFR:        "RSSI",
		LabelEN:        "CISO",
		DescriptionFR:  "Responsable de la sécurité : risques cyber, vulnérabilités, incidents, contrôles et KPI de sécurité.",
		DescriptionEN:  "Chief information security officer: cyber risks, vulnerabilities, incidents, security controls and KPIs.",
		DefaultLanding: "/",
		Permissions: []PermissionKey{
			"events:read",
			"risks:read", "risks:create", "risks:update",
			"vulnerabilities:read", "vulnerabilities:update",
			"incidents:read", "incidents:create", "incidents:update",
			"mitigations:read", "mitigations:create", "mitigations:update",
			"assets:read",
			"compliance:read", "compliance:frameworks:read", "compliance:controls:read", "compliance:controls:update", "compliance:evidences:read",
			"automation:read", "automation:write",
			"scanner:read", "scanner:scan", "scanner:import",
			"reports:board:read",
		},
	},
	{
		Key:            BusinessRoleDSI,
		LabelFR:        "DSI",
		LabelEN:        "CIO",
		DescriptionFR:  "Direction des systèmes d'information : vision SI globale, actifs, disponibilité et risques IT.",
		DescriptionEN:  "IT director: global IT view, assets, availability and IT risks.",
		DefaultLanding: "/assets",
		Permissions: []PermissionKey{
			"events:read",
			"risks:read", "risks:create", "risks:update",
			"assets:read", "assets:create", "assets:update", "assets:delete",
			"mitigations:read", "mitigations:create", "mitigations:update",
			"vulnerabilities:read",
			"incidents:read", "incidents:create", "incidents:update",
			"compliance:read", "compliance:frameworks:read", "compliance:controls:read",
			"scanner:read", "scanner:create", "scanner:scan", "scanner:import",
			"automation:read",
			"reports:board:read",
		},
	},
	{
		Key:            BusinessRoleRiskManager,
		LabelFR:        "Risk Manager",
		LabelEN:        "Risk Manager",
		DescriptionFR:  "Gestion des risques : registre, évaluations, traitements, heatmaps et reporting.",
		DescriptionEN:  "Risk management: register, assessments, treatments, heatmaps and reporting.",
		DefaultLanding: "/risks",
		Permissions: []PermissionKey{
			"events:read",
			"risks:read", "risks:create", "risks:update", "risks:delete",
			"mitigations:read", "mitigations:create", "mitigations:update", "mitigations:delete",
			"assets:read",
			"vulnerabilities:read",
			"compliance:read", "compliance:frameworks:read", "compliance:controls:read",
			"reports:board:read", "reports:board:create", "reports:board:update",
			"automation:read",
		},
	},
	{
		Key:            BusinessRoleAuditor,
		LabelFR:        "Auditeur",
		LabelEN:        "Auditor",
		DescriptionFR:  "Audit : campagnes d'audit, constats, preuves, plans d'actions et historique (lecture large + gestion des audits).",
		DescriptionEN:  "Audit: audit campaigns, findings, evidences, action plans and history (broad read + audit management).",
		DefaultLanding: "/compliance",
		Permissions: []PermissionKey{
			"events:read",
			"risks:read",
			"assets:read",
			"mitigations:read",
			"vulnerabilities:read",
			"incidents:read",
			"compliance:read", "compliance:frameworks:read", "compliance:controls:read", "compliance:evidences:read",
			"compliance:audits:read", "compliance:audits:write",
			"compliance:remediations:read",
			"reports:board:read",
		},
	},
	{
		Key:            BusinessRoleComplianceOfficer,
		LabelFR:        "Responsable conformité",
		LabelEN:        "Compliance Officer",
		DescriptionFR:  "Conformité : référentiels, contrôles, preuves, plans de remédiation et obligations réglementaires.",
		DescriptionEN:  "Compliance: frameworks, controls, evidences, remediation plans and regulatory obligations.",
		DefaultLanding: "/compliance",
		Permissions: []PermissionKey{
			"events:read",
			"risks:read",
			"assets:read",
			"mitigations:read",
			"compliance:read",
			"compliance:frameworks:read", "compliance:frameworks:create",
			"compliance:controls:read", "compliance:controls:create", "compliance:controls:update",
			"compliance:evidences:read", "compliance:evidences:create",
			"compliance:audits:read", "compliance:audits:write",
			"compliance:remediations:read", "compliance:remediations:write",
			"reports:board:read",
		},
	},
	{
		Key:            BusinessRoleInternalControl,
		LabelFR:        "Contrôle interne",
		LabelEN:        "Internal Control",
		DescriptionFR:  "Contrôle interne : test des contrôles, preuves, audits et plans de remédiation.",
		DescriptionEN:  "Internal control: control testing, evidences, audits and remediation plans.",
		DefaultLanding: "/compliance",
		Permissions: []PermissionKey{
			"events:read",
			"risks:read",
			"assets:read",
			"mitigations:read", "mitigations:update",
			"compliance:read",
			"compliance:frameworks:read",
			"compliance:controls:read", "compliance:controls:update",
			"compliance:evidences:read", "compliance:evidences:create",
			"compliance:audits:read", "compliance:audits:write",
			"compliance:remediations:read", "compliance:remediations:write",
			"reports:board:read",
		},
	},
	{
		Key:            BusinessRoleAssetOwner,
		LabelFR:        "Propriétaire d'actif",
		LabelEN:        "Asset Owner",
		DescriptionFR:  "Propriétaire d'actifs : gère son périmètre d'actifs et suit les risques associés.",
		DescriptionEN:  "Asset owner: manages their asset scope and tracks associated risks.",
		DefaultLanding: "/assets",
		Permissions: []PermissionKey{
			"events:read",
			"assets:read", "assets:create", "assets:update",
			"risks:read",
			"mitigations:read", "mitigations:update",
			"vulnerabilities:read",
			"compliance:read",
		},
	},
	{
		Key:            BusinessRoleRiskOwner,
		LabelFR:        "Propriétaire de risque",
		LabelEN:        "Risk Owner",
		DescriptionFR:  "Propriétaire de risques : traite les risques dont il a la charge et leurs plans de mitigation.",
		DescriptionEN:  "Risk owner: treats the risks they own and their mitigation plans.",
		DefaultLanding: "/risks",
		Permissions: []PermissionKey{
			"events:read",
			"risks:read", "risks:update",
			"mitigations:read", "mitigations:create", "mitigations:update",
			"assets:read",
			"vulnerabilities:read",
			"compliance:read",
		},
	},
	{
		Key:            BusinessRoleSecurityAnalyst,
		LabelFR:        "Analyste sécurité",
		LabelEN:        "Security Analyst",
		DescriptionFR:  "Analyste SOC : vulnérabilités, incidents, scans, automatisation et traitement des risques.",
		DescriptionEN:  "SOC analyst: vulnerabilities, incidents, scans, automation and risk treatment.",
		DefaultLanding: "/vulnerabilities",
		Permissions: []PermissionKey{
			"events:read",
			"risks:read", "risks:create", "risks:update",
			"vulnerabilities:read", "vulnerabilities:update", "vulnerabilities:delete",
			"incidents:read", "incidents:create", "incidents:update", "incidents:delete",
			"mitigations:read", "mitigations:create", "mitigations:update",
			"assets:read",
			"scanner:read", "scanner:create", "scanner:scan", "scanner:import", "scanner:delete",
			"automation:read", "automation:write",
			"compliance:read", "compliance:controls:read",
		},
	},
	{
		Key:            BusinessRoleExecutive,
		LabelFR:        "Direction",
		LabelEN:        "Executive",
		DescriptionFR:  "Direction / Comex : tableau de bord stratégique, exposition financière et rapports — sans détail technique.",
		DescriptionEN:  "Executive / board: strategic dashboard, financial exposure and reports — no technical detail.",
		DefaultLanding: "/analytics",
		Permissions: []PermissionKey{
			"events:read",
			"risks:read",
			"compliance:read",
			"reports:board:read",
		},
	},
	{
		Key:            BusinessRoleViewer,
		LabelFR:        "Lecteur",
		LabelEN:        "Viewer",
		DescriptionFR:  "Accès en lecture seule à l'ensemble de la posture GRC.",
		DescriptionEN:  "Read-only access to the whole GRC posture.",
		DefaultLanding: "/",
		Permissions: []PermissionKey{
			"events:read",
			"risks:read",
			"assets:read",
			"mitigations:read",
			"vulnerabilities:read",
			"incidents:read",
			"compliance:read", "compliance:frameworks:read", "compliance:controls:read",
			"reports:board:read",
		},
	},
}

// businessRoleIndex is a fast lookup by key.
var businessRoleIndex = func() map[BusinessRoleKey]BusinessRole {
	m := make(map[BusinessRoleKey]BusinessRole, len(businessRoles))
	for _, r := range businessRoles {
		m[r.Key] = r
	}
	return m
}()

// ListBusinessRoles returns the ordered preset catalog (defensive copy).
func ListBusinessRoles() []BusinessRole {
	out := make([]BusinessRole, len(businessRoles))
	copy(out, businessRoles)
	return out
}

// GetBusinessRole returns the preset for key, or false if unknown.
func GetBusinessRole(key BusinessRoleKey) (BusinessRole, bool) {
	r, ok := businessRoleIndex[key]
	return r, ok
}

// IsBusinessRole reports whether key names a known preset.
func IsBusinessRole(key BusinessRoleKey) bool {
	_, ok := businessRoleIndex[key]
	return ok
}

// BusinessRolePermissions returns the permission strings for a preset (nil if
// unknown). A defensive copy so callers can't mutate the preset.
func BusinessRolePermissions(key BusinessRoleKey) []PermissionKey {
	r, ok := businessRoleIndex[key]
	if !ok {
		return nil
	}
	out := make([]PermissionKey, len(r.Permissions))
	copy(out, r.Permissions)
	return out
}

// DefaultLandingFor returns the post-login landing route for a preset, or "/"
// when the key is unknown/empty.
func DefaultLandingFor(key BusinessRoleKey) string {
	if r, ok := businessRoleIndex[key]; ok && r.DefaultLanding != "" {
		return r.DefaultLanding
	}
	return "/"
}

// ValidateBusinessRoles asserts every preset only references catalog keys and
// carries a landing route. Returns the offending keys, if any. Exercised by
// tests so a typo in a preset can never silently ship a dead permission.
func ValidateBusinessRoles() []string {
	var bad []string
	for _, r := range businessRoles {
		if r.DefaultLanding == "" {
			bad = append(bad, string(r.Key)+":missing_landing")
		}
		seen := map[PermissionKey]struct{}{}
		for _, p := range r.Permissions {
			if !IsCatalogPermission(p) {
				bad = append(bad, string(r.Key)+":unknown_permission:"+p)
			}
			if _, dup := seen[p]; dup {
				bad = append(bad, string(r.Key)+":duplicate_permission:"+p)
			}
			seen[p] = struct{}{}
		}
	}
	return bad
}
