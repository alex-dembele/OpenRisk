// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package repository

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/opendefender/openrisk/internal/domain"
	"gorm.io/gorm"
)

// MitigationRepository defines repository pattern for mitigations
type MitigationRepository interface {
	// CRUD operations
	Create(ctx string, mitigation *domain.Mitigation) error

	// CreateWithSubActions inserts a plan and its whole checklist in ONE
	// transaction: either every row lands or none does. The plan insert and the
	// sub-action inserts are a multi-table write, so splitting them across two
	// repository calls leaves a committed plan with a truncated checklist when
	// the second insert fails (see #335).
	//
	// The transaction lives here, in the repository, because the application
	// layer may not import GORM and so cannot hold a transaction handle.
	CreateWithSubActions(ctx string, mitigation *domain.Mitigation, subActions []*domain.MitigationSubAction) error
	GetByID(ctx string, id uuid.UUID) (*domain.Mitigation, error)
	GetByIDWithSubActions(ctx string, id uuid.UUID) (*domain.Mitigation, error)
	List(ctx string, filters map[string]interface{}) ([]domain.Mitigation, error)
	Update(ctx string, mitigation *domain.Mitigation) error
	Delete(ctx string, id uuid.UUID) error

	// Risk-specific queries
	ListByRiskID(ctx string, riskID uuid.UUID) ([]domain.Mitigation, error)

	// Progress calculation
	RecalculateProgress(ctx string, mitigationID uuid.UUID) (int, error)
}

// GormMitigationRepository implements MitigationRepository using GORM
type GormMitigationRepository struct {
	db *gorm.DB
}

func NewGormMitigationRepository(db *gorm.DB) MitigationRepository {
	return &GormMitigationRepository{db: db}
}

// Create inserts a new mitigation
func (r *GormMitigationRepository) Create(tenantID string, mitigation *domain.Mitigation) error {
	if mitigation.TenantID == uuid.Nil {
		return errors.New("tenant_id is required")
	}

	result := r.db.WithContext(r.db.Statement.Context).Create(mitigation)
	if result.Error != nil {
		return fmt.Errorf("failed to create mitigation: %w", result.Error)
	}
	return nil
}

// GetByID retrieves a mitigation by ID (with tenant isolation)
func (r *GormMitigationRepository) GetByID(tenantID string, id uuid.UUID) (*domain.Mitigation, error) {
	var mitigation domain.Mitigation

	tenantUUID, err := uuid.Parse(tenantID)
	if err != nil {
		return nil, fmt.Errorf("invalid tenant_id: %w", err)
	}

	result := r.db.Where("tenant_id = ? AND id = ? AND deleted_at IS NULL", tenantUUID, id).
		First(&mitigation)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get mitigation: %w", result.Error)
	}

	return &mitigation, nil
}

// GetByIDWithSubActions retrieves a mitigation with all its subactions
func (r *GormMitigationRepository) GetByIDWithSubActions(tenantID string, id uuid.UUID) (*domain.Mitigation, error) {
	var mitigation domain.Mitigation

	tenantUUID, err := uuid.Parse(tenantID)
	if err != nil {
		return nil, fmt.Errorf("invalid tenant_id: %w", err)
	}

	result := r.db.Where("tenant_id = ? AND id = ? AND deleted_at IS NULL", tenantUUID, id).
		Preload("SubActions", func(db *gorm.DB) *gorm.DB {
			// mitigation_subactions has no tenant_id column: it is gated through
			// its parent, which the Where above already pinned to this tenant.
			return db.Where("deleted_at IS NULL").Order("\"order\", created_at")
		}).
		Preload("Risk").
		First(&mitigation)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get mitigation with subactions: %w", result.Error)
	}

	return &mitigation, nil
}

// List retrieves mitigations by filters (status, priority, etc.)
func (r *GormMitigationRepository) List(tenantID string, filters map[string]interface{}) ([]domain.Mitigation, error) {
	var mitigations []domain.Mitigation

	tenantUUID, err := uuid.Parse(tenantID)
	if err != nil {
		return nil, fmt.Errorf("invalid tenant_id: %w", err)
	}

	query := r.db.Where("tenant_id = ? AND deleted_at IS NULL", tenantUUID)

	// Apply filters
	if status, ok := filters["status"]; ok {
		// Chained onto query, which filters by tenant_id above.
		query = query.Where("status = ?", status)
	}
	if priority, ok := filters["priority"]; ok {
		// Chained onto query, which filters by tenant_id above.
		query = query.Where("priority = ?", priority)
	}
	if riskID, ok := filters["risk_id"]; ok {
		// Chained onto query, which filters by tenant_id above.
		query = query.Where("risk_id = ?", riskID)
	}
	// "Mes mitigations": any of the three accountability slots. The legacy jsonb
	// array is OR'd in so rows assigned before migration 0044 still answer.
	if user, ok := filters["involved_user"]; ok {
		// Chained onto query, which filters by tenant_id above.
		query = query.Where(
			"(owner_id = ? OR assignee_id = ? OR reviewer_id = ? OR assigned_to @> ?)",
			user, user, user, fmt.Sprintf(`["%v"]`, user),
		)
	}

	result := query.Order("created_at DESC").Find(&mitigations)

	if result.Error != nil {
		return nil, fmt.Errorf("failed to list mitigations: %w", result.Error)
	}

	return mitigations, nil
}

// Update saves changes to a mitigation
func (r *GormMitigationRepository) Update(tenantID string, mitigation *domain.Mitigation) error {
	tenantUUID, err := uuid.Parse(tenantID)
	if err != nil {
		return fmt.Errorf("invalid tenant_id: %w", err)
	}

	// Verify ownership before updating
	result := r.db.Where("tenant_id = ? AND id = ? AND deleted_at IS NULL", tenantUUID, mitigation.ID).
		Updates(mitigation)

	if result.Error != nil {
		return fmt.Errorf("failed to update mitigation: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}

	return nil
}

// Delete soft-deletes a mitigation
func (r *GormMitigationRepository) Delete(tenantID string, id uuid.UUID) error {
	tenantUUID, err := uuid.Parse(tenantID)
	if err != nil {
		return fmt.Errorf("invalid tenant_id: %w", err)
	}

	result := r.db.Where("tenant_id = ? AND id = ?", tenantUUID, id).
		Delete(&domain.Mitigation{})

	if result.Error != nil {
		return fmt.Errorf("failed to delete mitigation: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}

	return nil
}

// ListByRiskID retrieves all mitigations for a specific risk
func (r *GormMitigationRepository) ListByRiskID(tenantID string, riskID uuid.UUID) ([]domain.Mitigation, error) {
	return r.List(tenantID, map[string]interface{}{"risk_id": riskID})
}

// RecalculateProgress recomputes a plan's progress from its sub-actions and
// persists it. This is the ONLY writer of `progress`: no endpoint accepts the
// field, so the bar and the checklist cannot disagree.
//
// Previously this returned 0 for a plan with no sub-actions regardless of its
// status — which is exactly the reported "progression bloquée à 0 %": a plan
// somebody marked DONE still read 0 %. The rule now lives in
// domain.ComputeMitigationProgress, is pure, and is tested there.
//
// Tenant-scoped: the UPDATE carries tenant_id, so a mitigation id from another
// organisation touches no rows.
func (r *GormMitigationRepository) RecalculateProgress(tenantID string, mitigationID uuid.UUID) (int, error) {
	tenantUUID, err := uuid.Parse(tenantID)
	if err != nil {
		return 0, fmt.Errorf("invalid tenant_id: %w", err)
	}

	var m domain.Mitigation
	if err := r.db.
		Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", mitigationID, tenantUUID).
		First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, domain.NewNotFoundError("mitigation", mitigationID)
		}
		return 0, err
	}

	var total, completed int64
	r.db.Model(&domain.MitigationSubAction{}).
		Where("mitigation_id = ? AND deleted_at IS NULL", mitigationID).
		Count(&total)
	if total > 0 {
		r.db.Model(&domain.MitigationSubAction{}).
			Where("mitigation_id = ? AND deleted_at IS NULL AND completed = ?", mitigationID, true).
			Count(&completed)
	}

	progress := domain.ComputeMitigationProgress(m.Status, int(total), int(completed))

	updates := map[string]interface{}{"progress": progress}
	// Advance the status when the checklist says the work is done, so the board
	// column and the bar agree. The plan lands in REVIEW rather than DONE: a
	// human still signs it off, and the risk lifecycle's MITIGATED guard reads
	// the sub-actions directly either way.
	if total > 0 && progress == 100 &&
		m.Status != domain.MitigationDone && m.Status != domain.MitigationCancelled {
		updates["status"] = domain.MitigationReview
	} else if total > 0 && progress > 0 && m.Status == domain.MitigationPlanned {
		updates["status"] = domain.MitigationInProgress
	}

	if err := r.db.Model(&domain.Mitigation{}).
		Where("id = ? AND tenant_id = ?", mitigationID, tenantUUID).
		Updates(updates).Error; err != nil {
		return 0, err
	}

	return progress, nil
}
