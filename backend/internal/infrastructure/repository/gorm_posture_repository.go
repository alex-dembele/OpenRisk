// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/opendefender/openrisk/internal/domain"
	"github.com/opendefender/openrisk/pkg/scoring"
)

// GormPostureRepository is the read side of the Posture Reveal and the
// recognition screen (#438).
//
// ONE RULE GOVERNS EVERY QUERY IN THIS FILE: `tenant_id = ?`, on the table's own
// tenant column, with no join that could reach a row belonging to somebody else.
// This is #438 criterion 10 and ABSOLUTE RULE #2, and it matters more here than
// almost anywhere else in the codebase — the reveal is the screen a brand-new
// customer forwards to their CISO, so a leak here is a leak into an email.
//
// Models are domain structs rather than table names so GORM applies the
// soft-delete scope: a tenant whose only risk was deleted has not created a
// risk, and raw SQL would silently have counted the tombstone.
type GormPostureRepository struct {
	db *gorm.DB
}

// NewGormPostureRepository builds the repository.
func NewGormPostureRepository(db *gorm.DB) *GormPostureRepository {
	return &GormPostureRepository{db: db}
}

// Compile-time proof that this satisfies both ports. Without these, a signature
// drift would only surface at wiring time in main.go.
var (
	_ domain.PostureReader       = (*GormPostureRepository)(nil)
	_ domain.RecognitionReader   = (*GormPostureRepository)(nil)
	_ domain.OnboardingStepProbe = (*GormPostureRepository)(nil)
)

// RiskCounts returns the tenant's register in numbers, banded by the Score
// Engine's own criticality column — the reveal never invents a banding.
func (r *GormPostureRepository) RiskCounts(ctx context.Context, tenantID uuid.UUID) (domain.PostureRiskCounts, error) {
	out := domain.PostureRiskCounts{ByLevel: map[string]int{}}
	if tenantID == uuid.Nil {
		return out, domain.NewForbiddenError("a tenant is required to count risks")
	}

	var rows []struct {
		Criticality string
		N           int64
	}
	if err := r.db.WithContext(ctx).
		Model(&domain.Risk{}).
		Select("criticality, COUNT(*) AS n").
		Where("tenant_id = ?", tenantID).
		Group("criticality").
		Scan(&rows).Error; err != nil {
		return out, fmt.Errorf("failed to count risks: %w", err)
	}

	for _, row := range rows {
		out.ByLevel[row.Criticality] = int(row.N)
		out.Total += int(row.N)
	}
	return out, nil
}

// TopRisks returns the tenant's most exposed risks, highest Score Engine score
// first. `Risk.Score` is read, never recomputed: it is frozen.
func (r *GormPostureRepository) TopRisks(ctx context.Context, tenantID uuid.UUID, limit int) ([]domain.PostureRisk, error) {
	if tenantID == uuid.Nil {
		return nil, domain.NewForbiddenError("a tenant is required to read risks")
	}
	if limit <= 0 {
		limit = domain.TopRiskLimit
	}

	var risks []domain.Risk
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Order("score DESC, created_at ASC").
		Limit(limit).
		Find(&risks).Error; err != nil {
		return nil, fmt.Errorf("failed to read top risks: %w", err)
	}

	out := make([]domain.PostureRisk, 0, len(risks))
	for _, risk := range risks {
		title := risk.Title
		if title == "" {
			title = risk.Name
		}
		out = append(out, domain.PostureRisk{
			ID:       risk.ID,
			Title:    title,
			Inherent: risk.Score,
			Level:    string(risk.Criticality),
		})
	}
	return out, nil
}

// ControlCounts returns the tenant's compliance position. Statuses are counted
// separately rather than reduced here, because coverage and residual credit
// treat `in_progress` and `not_applicable` differently (ADR 0003).
func (r *GormPostureRepository) ControlCounts(ctx context.Context, tenantID uuid.UUID) (domain.PostureControlCounts, error) {
	var out domain.PostureControlCounts
	if tenantID == uuid.Nil {
		return out, domain.NewForbiddenError("a tenant is required to count controls")
	}

	var rows []struct {
		Status string
		N      int64
	}
	if err := r.db.WithContext(ctx).
		Model(&domain.ComplianceControl{}).
		Select("status, COUNT(*) AS n").
		Where("tenant_id = ?", tenantID).
		Group("status").
		Scan(&rows).Error; err != nil {
		return out, fmt.Errorf("failed to count controls: %w", err)
	}

	for _, row := range rows {
		n := int(row.N)
		out.Total += n
		switch domain.ControlStatus(row.Status) {
		case domain.ControlStatusImplemented:
			out.Implemented += n
		case domain.ControlStatusInProgress:
			out.InProgress += n
		case domain.ControlStatusNotApplicable:
			out.NotApplicable += n
		}
	}

	var frameworks int64
	if err := r.db.WithContext(ctx).
		Model(&domain.ComplianceFramework{}).
		Where("tenant_id = ?", tenantID).
		Count(&frameworks).Error; err != nil {
		return out, fmt.Errorf("failed to count frameworks: %w", err)
	}
	out.Frameworks = int(frameworks)

	return out, nil
}

// ControlCreditsByRisk returns, per risk, the credits ADR 0003 consumes.
//
// Two tenancy guards, not one. The WHERE pins `risk_control_mappings.tenant_id`,
// AND the join to `compliance_controls` carries `c.tenant_id = m.tenant_id`, so
// a mapping row that pointed at another tenant's control (a corrupted row, a bad
// import) still cannot pull that control's status into this tenant's score.
//
// HasEvidence is filled from the evidence library on the same terms the
// compliance repository uses — accepted review, not expired. Getting this wrong
// is silent: every implemented control would earn 0.70 instead of 1.00 and the
// reveal would under-report the tenant's own posture.
func (r *GormPostureRepository) ControlCreditsByRisk(ctx context.Context, tenantID uuid.UUID, riskIDs []uuid.UUID) (map[uuid.UUID][]scoring.ControlCredit, error) {
	out := map[uuid.UUID][]scoring.ControlCredit{}
	if tenantID == uuid.Nil {
		return out, domain.NewForbiddenError("a tenant is required to read control mappings")
	}
	if len(riskIDs) == 0 {
		return out, nil
	}

	var rows []struct {
		RiskID        uuid.UUID
		Status        string
		EvidenceCount int64
	}
	if err := r.db.WithContext(ctx).
		Table("risk_control_mappings m").
		Select(`m.risk_id AS risk_id,
		        c.status AS status,
		        (SELECT COUNT(*)
		           FROM evidence_control_links l
		           JOIN evidences e
		             ON e.id = l.evidence_id
		            AND e.tenant_id = l.tenant_id
		            AND e.deleted_at IS NULL
		          WHERE l.tenant_id = m.tenant_id
		            AND l.control_id = c.id
		            AND e.review = ?
		            AND (e.valid_until IS NULL OR e.valid_until > ?)
		        ) AS evidence_count`,
			domain.EvidenceReviewAccepted, time.Now()).
		Joins("JOIN compliance_controls c ON c.id = m.control_id AND c.tenant_id = m.tenant_id AND c.deleted_at IS NULL").
		Where("m.tenant_id = ? AND m.risk_id IN ? AND m.deleted_at IS NULL", tenantID, riskIDs).
		Scan(&rows).Error; err != nil {
		return out, fmt.Errorf("failed to read control mappings: %w", err)
	}

	for _, row := range rows {
		out[row.RiskID] = append(out[row.RiskID], scoring.ControlCredit{
			Status:      scoring.ParseControlCoverageStatus(row.Status),
			HasEvidence: row.EvidenceCount > 0,
		})
	}
	return out, nil
}

// RecognitionCounts answers "what does OpenRisk already know about us" for the
// tenant configured before any of this shipped (criterion 9).
//
// Members are counted on `organization_id`, which is that table's own tenant
// column — the same choice the activation backfill makes for the `team` step,
// and deliberately not `tenant_id`, which `organization_members` does not have.
func (r *GormPostureRepository) RecognitionCounts(ctx context.Context, tenantID uuid.UUID) (domain.RecognitionCounts, error) {
	var out domain.RecognitionCounts
	if tenantID == uuid.Nil {
		return out, domain.NewForbiddenError("a tenant is required to read a recognition")
	}

	counts := []struct {
		model  any
		column string
		into   *int
	}{
		{&domain.Risk{}, "tenant_id", &out.Risks},
		{&domain.ComplianceFramework{}, "tenant_id", &out.Frameworks},
		{&domain.ComplianceControl{}, "tenant_id", &out.Controls},
		{&domain.Asset{}, "tenant_id", &out.Assets},
		{&domain.OrganizationMember{}, "organization_id", &out.Members},
	}

	for _, c := range counts {
		var n int64
		if err := r.db.WithContext(ctx).
			Model(c.model).
			Where(c.column+" = ?", tenantID).
			Count(&n).Error; err != nil {
			return out, fmt.Errorf("failed to count %T: %w", c.model, err)
		}
		*c.into = int(n)
	}
	return out, nil
}

// OnboardingStepData resolves the tunnel's auto-skip decisions in one pass
// (#438 criteria 2 and 3). Called ONCE per GET /onboarding/state; no step may
// probe on mount.
//
// Tenant-scoped like everything else in this file, and user-scoped where the
// question is about a person: the profile step belongs to the individual, so a
// colleague having filled theirs in must not skip this user's.
func (r *GormPostureRepository) OnboardingStepData(ctx context.Context, tenantID, userID uuid.UUID) (domain.OnboardingStepData, error) {
	var out domain.OnboardingStepData
	if tenantID == uuid.Nil || userID == uuid.Nil {
		return out, domain.NewForbiddenError("a tenant and a user are required to resolve onboarding steps")
	}

	// The organisation step asks for an industry and a size. Both must already be
	// present to skip it: an organisation with a name and nothing else has not
	// answered the question the step actually asks.
	var org domain.Organization
	if err := r.db.WithContext(ctx).
		Select("industry", "size").
		Where("id = ?", tenantID).
		First(&org).Error; err != nil && err != gorm.ErrRecordNotFound {
		return out, fmt.Errorf("failed to read the organization profile: %w", err)
	}
	out.HasOrganizationProfile = strings.TrimSpace(org.Industry) != "" && strings.TrimSpace(string(org.Size)) != ""

	// The profile step asks for a name and a function. `job_title` is stored in
	// User.Department — see onboarding_profile_writes.go, which is the writer this
	// probe has to agree with. Reading a different column here would skip a step
	// the wizard never filled, or ask again for one it did.
	var user domain.User
	if err := r.db.WithContext(ctx).
		Select("full_name", "department").
		Where("id = ?", userID).
		First(&user).Error; err != nil && err != gorm.ErrRecordNotFound {
		return out, fmt.Errorf("failed to read the user profile: %w", err)
	}
	out.HasUserProfile = strings.TrimSpace(user.FullName) != "" && strings.TrimSpace(user.Department) != ""

	var frameworks int64
	if err := r.db.WithContext(ctx).
		Model(&domain.ComplianceFramework{}).
		Where("tenant_id = ?", tenantID).
		Count(&frameworks).Error; err != nil {
		return out, fmt.Errorf("failed to count frameworks: %w", err)
	}
	out.HasFramework = frameworks > 0

	// More than the founding member. Exactly one member is every brand-new
	// tenant, so `> 0` here would skip the team step for everybody.
	var members int64
	if err := r.db.WithContext(ctx).
		Model(&domain.OrganizationMember{}).
		Where("organization_id = ?", tenantID).
		Count(&members).Error; err != nil {
		return out, fmt.Errorf("failed to count members: %w", err)
	}
	out.HasTeam = members > 1

	return out, nil
}
