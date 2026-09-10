// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// RiskStatus represents the lifecycle state of a risk
type RiskStatus string

const (
	RiskOpen       RiskStatus = "open"        // Newly identified, under review
	RiskInProgress RiskStatus = "in_progress" // Mitigation underway
	RiskMitigated  RiskStatus = "mitigated"   // Treatment plan completed
	RiskAccepted   RiskStatus = "accepted"    // Formally accepted as residual
	RiskClosed     RiskStatus = "closed"      // Fully resolved/no longer relevant
	// Legacy statuses for compatibility
	StatusDraft     RiskStatus = "DRAFT"
	StatusActive    RiskStatus = "ACTIVE"
	StatusMitigated RiskStatus = "MITIGATED"
	StatusAccepted  RiskStatus = "ACCEPTED"
)

// CriticalityLevel represents the severity level calculated from score
type CriticalityLevel string

const (
	// New lowercase versions
	CriticalityLowNew      CriticalityLevel = "low"
	CriticalityMediumNew   CriticalityLevel = "medium"
	CriticalityHighNew     CriticalityLevel = "high"
	CriticalityCriticalNew CriticalityLevel = "critical"
	// Legacy constants for compatibility
	RiskCriticalityLow      CriticalityLevel = "LOW"
	RiskCriticalityMedium   CriticalityLevel = "MEDIUM"
	RiskCriticalityHigh     CriticalityLevel = "HIGH"
	RiskCriticalityCritical CriticalityLevel = "CRITICAL"
)

// CriticalityFromScore maps a raw Score Engine score (P × I × AC) to its
// severity band using the canonical thresholds (>=7 critical, >=4 high,
// >=2 medium, <2 low). It returns the lowercase form the ScoreWorker persists,
// so a value computed synchronously at create/update time agrees with the band
// the worker later writes. Kept in the domain so every producer bands
// identically (mirrors pkg/scoring engine.ToCriticality).
func CriticalityFromScore(score float64) CriticalityLevel {
	switch {
	case score >= 7.0:
		return CriticalityCriticalNew
	case score >= 4.0:
		return CriticalityHighNew
	case score >= 2.0:
		return CriticalityMediumNew
	default:
		return CriticalityLowNew
	}
}

// RiskPhase represents the ISO 31000 risk-management lifecycle stage of a risk.
// This is ORTHOGONAL to RiskStatus: Status is the resolution state
// (open/mitigated/accepted/…) while Phase is where the risk sits in the
// managed lifecycle "Identifier → Analyser → Évaluer → Traiter → Surveiller →
// Clôturer". Surfaced live on the real Risk entity (register + drawer stepper).
type RiskPhase string

const (
	PhaseIdentified RiskPhase = "identified" // Identifier — risk logged, context captured
	PhaseAnalyzed   RiskPhase = "analyzed"   // Analyser — probability/impact/causes assessed
	PhaseEvaluated  RiskPhase = "evaluated"  // Évaluer — prioritised vs risk appetite
	PhaseTreated    RiskPhase = "treated"    // Traiter — treatment plan chosen/underway
	PhaseMonitored  RiskPhase = "monitored"  // Surveiller — under continuous review
	PhaseClosed     RiskPhase = "closed"     // Clôturer — resolved / no longer relevant
)

// riskPhaseOrder is the canonical forward ordering of the lifecycle. Index is
// used to validate transitions (see CanTransitionTo).
var riskPhaseOrder = []RiskPhase{
	PhaseIdentified, PhaseAnalyzed, PhaseEvaluated, PhaseTreated, PhaseMonitored, PhaseClosed,
}

// phaseIndex returns the position of a phase in riskPhaseOrder, or -1 if unknown.
func phaseIndex(p RiskPhase) int {
	for i, phase := range riskPhaseOrder {
		if phase == p {
			return i
		}
	}
	return -1
}

// ParseRiskPhase validates and converts a string into a RiskPhase. An empty
// string defaults to PhaseIdentified (a freshly created risk is "identified").
func ParseRiskPhase(s string) (RiskPhase, error) {
	if s == "" {
		return PhaseIdentified, nil
	}
	if phaseIndex(RiskPhase(s)) >= 0 {
		return RiskPhase(s), nil
	}
	return "", NewValidationError(fmt.Sprintf("invalid risk lifecycle phase: %q", s))
}

// CanTransitionTo reports whether a risk may move from its current phase to
// the target. The lifecycle is pragmatic rather than rigid: you may advance
// one step, step back one step (re-open a phase), or jump straight to
// "closed" (early closure from any phase). A no-op (same phase) is rejected so
// the caller surfaces a clear validation error instead of a silent write.
func (p RiskPhase) CanTransitionTo(target RiskPhase) bool {
	from, to := phaseIndex(p), phaseIndex(target)
	if from < 0 || to < 0 {
		return false
	}
	if from == to {
		return false
	}
	if target == PhaseClosed {
		return true // early closure allowed from anywhere
	}
	// Re-opening from closed is allowed back to any earlier phase.
	if p == PhaseClosed {
		return true
	}
	// Otherwise move at most one step in either direction.
	diff := to - from
	return diff == 1 || diff == -1
}

// RiskTreatment represents the chosen treatment strategy
type RiskTreatment string

const (
	TreatmentAccept   RiskTreatment = "accept"
	TreatmentMitigate RiskTreatment = "mitigate"
	TreatmentTransfer RiskTreatment = "transfer"
	TreatmentAvoid    RiskTreatment = "avoid"
)

// RiskSource indicates where the risk originated
type RiskSource string

const (
	SourceManual   RiskSource = "manual"
	SourceCTIAuto  RiskSource = "cti_auto"  // From CTI/NVD/CISA KEV
	SourceScanAuto RiskSource = "scan_auto" // From vulnerability scanner
	SourceImport   RiskSource = "import"    // Imported from file
	SourceVendor   RiskSource = "vendor"    // From vendor assessment
	SourceAI       RiskSource = "ai"        // AI-generated
	// SourceStarter marks a risk the onboarding tunnel wrote from the starter
	// catalogue after the user selected it (#438, D-012). Provenance matters more
	// here than anywhere else on this enum: these are rows OpenRisk put in a
	// customer's register, and a GRC product that cannot say which risks its own
	// onboarding wrote has a credibility problem no support ticket recovers.
	//
	// The column is already varchar(20), so this value needs NO migration.
	SourceStarter RiskSource = "starter"
)

// ParseRiskSource validates and converts a string into a RiskSource.
// An empty string defaults to SourceManual (matches the ERD column default:
// `source VARCHAR(50) NOT NULL DEFAULT 'manual'`). Any other value must match
// one of the known constants, or a typed validation error is returned.
func ParseRiskSource(s string) (RiskSource, error) {
	if s == "" {
		return SourceManual, nil
	}
	switch RiskSource(s) {
	case SourceManual, SourceCTIAuto, SourceScanAuto, SourceImport, SourceVendor, SourceAI, SourceStarter:
		return RiskSource(s), nil
	default:
		return "", NewValidationError(fmt.Sprintf("invalid risk source: %q", s))
	}
}

// Risk represents a business risk with full lifecycle management
// Follows Clean Architecture: pure domain entity, ZERO external dependencies
type Risk struct {
	// Primary Key
	ID uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`

	// Multi-tenancy (ABSOLUTE: filter by tenant_id in repository, never in handler)
	TenantID       uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	OrganizationID uuid.UUID `gorm:"type:uuid;index" json:"organization_id"` // Legacy alias for TenantID

	// Core Risk Definition
	Name        string `gorm:"size:255;not null;index" json:"name"`
	Title       string `gorm:"size:255;not null;index" json:"title"` // Alias for Name, for compatibility
	Description string `gorm:"type:text" json:"description"`

	// Risk Scoring (via Score Engine: P × I × AssetCriticality, 3 decimal places)
	// New system (0.0-1.0, 0.0-10.0, 0.1-3.0)
	Probability float64          `gorm:"type:numeric(5,3);check:probability >= 0 AND probability <= 1" json:"probability"`
	Impact      float64          `gorm:"type:numeric(5,1);check:impact >= 0 AND impact <= 10" json:"impact"`
	Score       float64          `gorm:"type:numeric(8,3);default:0" json:"score"`                // Calculated ONLY via Score Engine (Redis event)
	Criticality CriticalityLevel `gorm:"type:varchar(20);default:'low';index" json:"criticality"` // low|medium|high|critical

	// Legacy scoring system (1-5 scale, for backwards compatibility)
	// Will be deprecated as system migrates to new scale
	ImpactLegacy      int `gorm:"default:1;check:impact_legacy >= 1 AND impact_legacy <= 5" json:"impact_legacy"`
	ProbabilityLegacy int `gorm:"default:1;check:probability_legacy >= 1 AND probability_legacy <= 5" json:"probability_legacy"`

	// Lifecycle Management
	Status RiskStatus `gorm:"type:varchar(20);default:'open';index" json:"status"` // open|in_progress|mitigated|accepted|closed
	Level  string     `gorm:"size:20;default:'medium';index" json:"level"`         // Legacy: CRITICAL|HIGH|MEDIUM|LOW

	// Deprecated as a WRITABLE field: derived from LifecycleState. The column
	// stays (and stays correct) so the phase facet and any legacy reader keep
	// working — but nothing sets it independently any more.
	LifecyclePhase RiskPhase `gorm:"type:varchar(20);default:'identified';index" json:"lifecycle_phase"`

	// LifecycleState is the SINGLE source of truth for where a risk stands:
	// DRAFT → IDENTIFIED → ASSESSED → TREATMENT_PLANNED → IN_TREATMENT →
	// (RESIDUAL_ACCEPTED | MITIGATED) → CLOSED ↘ REOPENED ↗. Status and
	// LifecyclePhase above are derived from it on every write (SetState), which
	// is what stops the three from drifting apart.
	LifecycleState RiskState `gorm:"type:varchar(24);default:'draft';index" json:"lifecycle_state"`

	// Ownership & Assignment — the three accountability slots (owner_id /
	// assignee_id / reviewer_id) are embedded from domain.Ownership so every
	// actionable entity carries the exact same block. `reviewer_id` used to be
	// declared here by hand; it now comes from the embed with an identical
	// column and JSON key.
	Ownership `gorm:"embedded"`

	CreatedBy uuid.UUID `gorm:"type:uuid;not null;index" json:"created_by"`
	// Deprecated: superseded by Ownership.AssigneeID. Kept (and backfilled FROM,
	// migration 0044) so pre-existing filters and the RiskQuery.AssignedTo facet
	// keep answering while callers migrate.
	AssignedTo *uuid.UUID `gorm:"type:uuid;index" json:"assigned_to"`
	// Deprecated: free-text owner (email or user id). Superseded by
	// Ownership.OwnerID; kept for legacy readers.
	Owner string `json:"owner"`

	// Asset Association
	AssetID *uuid.UUID `gorm:"type:uuid;index" json:"asset_id"` // Linked asset if risk is asset-specific

	// Classification — three separate concepts, three separate columns. See
	// risk_taxonomy.go for why conflating them was the "étiquette affichée comme
	// framework" bug.

	// Tags are free text, authored by the user, unbounded → column "Étiquettes".
	Tags pq.StringArray `gorm:"type:text[];default:'{}'" json:"tags"`

	// CategoryID points at the tenant's CONTROLLED vocabulary → column
	// "Catégorie". Nullable: a risk may be unclassified, and forcing a category
	// at creation would just push people to pick the first entry.
	CategoryID *uuid.UUID    `gorm:"type:uuid;index" json:"category_id"`
	Category   *RiskCategory `gorm:"foreignKey:CategoryID" json:"category,omitempty"`

	// ControlMappings are references to REAL compliance controls → column
	// "Référentiel". Loaded by the list/get use cases, never stored inline.
	ControlMappings []RiskControlMapping `gorm:"-" json:"control_mappings,omitempty"`

	// Deprecated (migration 0046): free-text framework names, populated by a
	// hard-coded dropdown and never checked against the tenant's imported
	// frameworks. Frozen — no longer read or written. Superseded by
	// ControlMappings; the column is kept for one release so a rollback is
	// possible, and dropped in a later migration.
	Frameworks pq.StringArray `gorm:"type:text[];default:'{}'" json:"frameworks"`
	// Deprecated: never written by anything but duplicate_risk, never read.
	// Migrated into risk_control_mappings by 0046 where resolvable.
	ControlIDs pq.StringArray `gorm:"type:text[];default:'{}'" json:"control_ids"`

	// Treatment & Mitigation
	TreatmentPlan   RiskTreatment `gorm:"type:varchar(20);default:'mitigate'" json:"treatment_plan"` // accept|mitigate|transfer|avoid
	ResidualRisk    *float64      `gorm:"type:numeric(8,3)" json:"residual_risk"`                    // Score after treatments
	LastMitigatedAt *time.Time    `json:"last_mitigated_at"`

	// Cyber Risk Quantification (CRQ) — monetary loss inputs (pkg/crq). Optional:
	// when both are set, ALE = SLE × ARO; otherwise a reference value per
	// criticality is used. Amounts are XAF (FCFA); USD is derived on read.
	SLEXAF *float64 `gorm:"type:numeric(16,2)" json:"sle_xaf"` // single loss expectancy (XAF)
	ARO    *float64 `gorm:"type:numeric(10,4)" json:"aro"`     // annualized rate of occurrence (events/year)

	// Full financial-quantification drivers (spec §9). When SLE is not supplied
	// explicitly it is composed from these: downtime cost + fines + data loss +
	// other. RemediationCost + MitigationEffectiveness drive ROSI. All XAF, all
	// optional; the engine (pkg/crq) degrades gracefully to the reference model.
	DowntimeHours           *float64 `gorm:"type:numeric(10,2)" json:"downtime_hours"`           // business hours lost per incident
	HourlyDowntimeCostXAF   *float64 `gorm:"type:numeric(16,2)" json:"hourly_downtime_cost_xaf"` // cost per hour of downtime
	DataLossCostXAF         *float64 `gorm:"type:numeric(16,2)" json:"data_loss_cost_xaf"`       // data recovery / breach cost
	FinesXAF                *float64 `gorm:"type:numeric(16,2)" json:"fines_xaf"`                // regulatory fines
	OtherDirectCostXAF      *float64 `gorm:"type:numeric(16,2)" json:"other_direct_cost_xaf"`    // any other direct per-incident cost
	RemediationCostXAF      *float64 `gorm:"type:numeric(16,2)" json:"remediation_cost_xaf"`     // budget to deploy the control
	MitigationEffectiveness *float64 `gorm:"type:numeric(5,4)" json:"mitigation_effectiveness"`  // [0,1] share of ALE removed

	// Computed, NOT persisted — filled by the handler via pkg/crq before responding.
	ALEXAF   float64 `gorm:"-" json:"ale_xaf"`   // annual loss expectancy (XAF)
	ALEUSD   float64 `gorm:"-" json:"ale_usd"`   // annual loss expectancy (USD)
	ALEBasis string  `gorm:"-" json:"ale_basis"` // "explicit" | "reference"

	// Smart Risk Calculation (spec §8) — the multifactor score computed by
	// pkg/scoring.ComputeSmart from eight factors (business criticality, internet
	// exposure, vulnerabilities, control maturity, incident history, exploitability,
	// financial value, active threats). Persisted so the register can sort/badge on
	// it; refreshed on demand via GET /risks/:id/smart-score. SmartFactors is the
	// frozen per-factor breakdown (radar-ready []scoring.FactorScore snapshot).
	SmartScore      float64          `gorm:"type:numeric(5,2);default:0;index" json:"smart_score"` // 0–100
	SmartLevel      CriticalityLevel `gorm:"type:varchar(20)" json:"smart_level"`                  // low|medium|high|critical
	SmartFactors    datatypes.JSON   `gorm:"type:jsonb" json:"smart_factors,omitempty"`
	SmartComputedAt *time.Time       `json:"smart_computed_at,omitempty"`

	// Review cadence — automated risk-review reminders. ReviewIntervalDays = 0
	// disables it; NextReviewAt is when the owner is next nudged; LastReviewedAt is
	// the last time the risk was marked reviewed.
	ReviewIntervalDays int        `gorm:"default:0" json:"review_interval_days"`
	NextReviewAt       *time.Time `gorm:"index" json:"next_review_at,omitempty"`
	LastReviewedAt     *time.Time `json:"last_reviewed_at,omitempty"`

	// Source Tracking
	Source      RiskSource `gorm:"type:varchar(20);default:'manual';index" json:"source"` // manual|cti_auto|scan_auto|import|vendor|ai|starter
	SourceCVEID *string    `gorm:"index" json:"source_cve_id"`                            // CVE identifier if from CTI

	// Origin of an automatically proposed risk (Attack Surface §4). A machine
	// may propose a risk, but the person reviewing it must be able to see
	// exactly what proposed it and why — otherwise a draft register fills with
	// items nobody can evaluate, and the honest response to that is to ignore
	// all of them.
	SourceVulnerabilityID *uuid.UUID `gorm:"type:uuid;index" json:"source_vulnerability_id,omitempty"`
	// SourceRuleReason is the rule's own explanation, captured at creation time
	// ("CVSS 9.8 ≥ 7.0, asset criticality CRITICAL ≥ HIGH"). Frozen: re-deriving
	// it later would describe the rule as it is NOW, not the rule that fired.
	SourceRuleReason string `gorm:"type:text" json:"source_rule_reason,omitempty"`
	ExternalID       string `gorm:"index" json:"external_id"` // ID in external system

	// Custom Fields (JSONB for flexibility)
	CustomFields datatypes.JSON `gorm:"type:jsonb" json:"custom_fields,omitempty"`

	// Audit Trail
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"` // Soft delete

	// Relations (loaded via Preload)
	Mitigations []Mitigation `gorm:"foreignKey:RiskID" json:"mitigations,omitempty"`

	// MitigationsCount is computed, NOT persisted: the count of the plans above.
	// The UI derives its "Créer une mitigation" vs "Voir les mitigations (n)"
	// button from it, so that button reflects what EXISTS rather than what the
	// user did in this session — a state the previous version got wrong the
	// moment you reloaded the page.
	MitigationsCount int      `gorm:"-" json:"mitigations_count"`
	Assets           []*Asset `gorm:"many2many:risk_assets;" json:"assets,omitempty"`

	// Computed Fields (NOT persisted, populated by handlers/use cases)
	// These help with API responses but are never stored in DB
	RiskWithDetails *RiskDetail `gorm:"-" json:"-"`
}

// BeforeSave hook ensures basic validation and legacy compatibility
func (r *Risk) BeforeSave(tx *gorm.DB) error {
	// Ensure TenantID and OrganizationID are consistent (both required for multi-tenancy)
	if r.TenantID == uuid.Nil && r.OrganizationID != uuid.Nil {
		r.TenantID = r.OrganizationID
	}
	if r.OrganizationID == uuid.Nil && r.TenantID != uuid.Nil {
		r.OrganizationID = r.TenantID
	}

	// Name is required (use Title as fallback for legacy code)
	if r.Name == "" && r.Title != "" {
		r.Name = r.Title
	}
	if r.Title == "" && r.Name != "" {
		r.Title = r.Name
	}

	// Score Engine will recalculate via Redis event, but set initial legacy score if not set
	if r.Score == 0 && r.ImpactLegacy > 0 && r.ProbabilityLegacy > 0 {
		r.Score = float64(r.ImpactLegacy * r.ProbabilityLegacy)
	}

	// Ensure CreatedBy is set if creating.
	// CreatedBy must be set by the use case before saving; this is a
	// deliberate no-op rather than an enforced error, since hardening it
	// could break an existing save path that currently relies on this being
	// silent. Revisit as part of a dedicated domain-invariants audit.
	if r.CreatedBy == uuid.Nil && !tx.Statement.Changed("created_by") { //nolint:staticcheck // intentional no-op, see comment above
	}

	return nil
}

// AfterSave hook creates a history snapshot for audit trail and timeline
// Called automatically by GORM after save is successful
func (r *Risk) AfterSave(tx *gorm.DB) error {
	// Create a history snapshot for timeline and trends
	history := RiskHistory{
		ID:          uuid.New(),
		RiskID:      r.ID,
		Score:       r.Score,
		Impact:      r.ImpactLegacy,
		Probability: r.ProbabilityLegacy,
		Status:      r.Status,
		ChangedBy:   r.CreatedBy.String(), // Use UUID string
		ChangeType:  "UPDATE",
		CreatedAt:   time.Now(),
	}

	return tx.Create(&history).Error
}

// BulkSnapshot captures the fields a bulk action is allowed to change, and only
// those. It is what the audit journal's before → after is built from.
//
// Deliberately narrow: a bulk action changes a handful of named fields, so
// journalling the whole risk would bury the change in noise and drag unrelated
// columns (including any added later) into the trail. Nothing secret is in here
// — RULE #6.
func (r *Risk) BulkSnapshot() map[string]interface{} {
	if r == nil {
		return map[string]interface{}{}
	}
	var assignedTo interface{}
	if r.AssignedTo != nil {
		assignedTo = r.AssignedTo.String()
	}
	// Copy the slice: the snapshot must not alias the live risk, or mutating the
	// tags would retroactively rewrite the "before" the caller already took.
	tags := make([]string, len(r.Tags))
	copy(tags, r.Tags)
	return map[string]interface{}{
		"status": string(r.Status),
		// The canonical state travels with the status: SetState derives both, and
		// a trail that showed one without the other would not explain the change.
		"lifecycle_state": string(r.State()),
		"assigned_to":     assignedTo,
		"tags":            tags,
	}
}

// OwnershipBlock implements OwnedEntity.
func (r *Risk) OwnershipBlock() *Ownership { return &r.Ownership }

// CountMitigations fills the computed count from the preloaded relation.
// Called by the repository after every read that preloads Mitigations.
func (r *Risk) CountMitigations() {
	if r != nil {
		r.MitigationsCount = len(r.Mitigations)
	}
}

// State returns the canonical lifecycle state, reconstructing it from the two
// legacy fields for any row written before the column existed. Never empty.
func (r *Risk) State() RiskState {
	if r == nil {
		return StateDraft
	}
	if IsRiskState(r.LifecycleState) {
		return r.LifecycleState
	}
	return RiskStateFromLegacy(r.Status, r.LifecyclePhase)
}

// SetState moves the risk to a state and re-derives the two legacy fields from
// it. This is the ONLY supported way to change where a risk stands: writing
// Status or LifecyclePhase directly is what let them disagree.
//
// It performs no validation — the use case owns the guards, because they need
// data (mitigations, approvals) the domain must not reach for.
func (r *Risk) SetState(s RiskState) {
	if r == nil {
		return
	}
	r.LifecycleState = s
	r.Status = s.DerivedStatus()
	r.LifecyclePhase = s.DerivedPhase()
}

// RiskDetail is a DTO for API responses with enriched data
// Includes calculated fields and related data
type RiskDetail struct {
	Risk           *Risk                 `json:"risk"`
	Mitigations    []Mitigation          `json:"mitigations,omitempty"`
	Assets         []*Asset              `json:"assets,omitempty"`
	ScoreBreakdown *ScoreBreakdownDetail `json:"score_breakdown,omitempty"`
	AuditHistory   []AuditLogEntry       `json:"audit_history,omitempty"`
	AssignedToUser *UserInfo             `json:"assigned_to_user,omitempty"`
	ReviewerUser   *UserInfo             `json:"reviewer_user,omitempty"`
	CreatedByUser  *UserInfo             `json:"created_by_user,omitempty"`
}

// ScoreBreakdownDetail extends scoring.ScoreBreakdown with context
type ScoreBreakdownDetail struct {
	Score            float64  `json:"score"`
	Probability      float64  `json:"probability"`
	Impact           float64  `json:"impact"`
	AssetCriticality float64  `json:"asset_criticality"`
	Criticality      string   `json:"criticality"`
	Explanation      string   `json:"explanation"`
	PreviousScore    *float64 `json:"previous_score,omitempty"`
	Delta            *float64 `json:"delta,omitempty"`
	CalculatedAt     string   `json:"calculated_at"`
}

// AuditLogEntry represents a historical change to a risk
type AuditLogEntry struct {
	ID        uuid.UUID              `json:"id"`
	RiskID    uuid.UUID              `json:"risk_id"`
	Timestamp time.Time              `json:"timestamp"`
	ChangedBy uuid.UUID              `json:"changed_by"`
	Action    string                 `json:"action"`
	OldValue  map[string]interface{} `json:"old_value,omitempty"`
	NewValue  map[string]interface{} `json:"new_value,omitempty"`
}

// UserInfo is a minimal user representation for API responses
type UserInfo struct {
	ID    uuid.UUID `json:"id"`
	Email string    `json:"email"`
	Name  string    `json:"name"`
}
