// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/opendefender/openrisk/internal/domain"
)

// GormSavedViewRepository implements domain.SavedViewRepository.
//
// ABSOLUTE RULE 2: tenant_id is in the WHERE clause of every single query below,
// including the writes. A cross-tenant read returns (nil, nil) and a cross-tenant
// write affects zero rows — both of which the use case reports as 404, so the API
// never confirms that another tenant's view exists.
type GormSavedViewRepository struct {
	db *gorm.DB
}

// NewGormSavedViewRepository creates a GORM-backed saved-view repository.
func NewGormSavedViewRepository(db *gorm.DB) *GormSavedViewRepository {
	return &GormSavedViewRepository{db: db}
}

func (r *GormSavedViewRepository) Create(ctx context.Context, view *domain.SavedView) error {
	if view.TenantID == uuid.Nil {
		return fmt.Errorf("tenant_id is required")
	}
	if view.UserID == uuid.Nil {
		return fmt.Errorf("user_id is required")
	}
	return r.db.WithContext(ctx).Create(view).Error
}

func (r *GormSavedViewRepository) GetByID(ctx context.Context, id, tenantID uuid.UUID) (*domain.SavedView, error) {
	var view domain.SavedView
	err := r.db.WithContext(ctx).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		First(&view).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get saved view: %w", err)
	}
	return &view, nil
}

// ListVisible returns the caller's own views plus the tenant's shared ones.
//
// The visibility predicate is inside the SQL rather than applied in Go after a
// broader read: filtering in memory would mean another user's personal view had
// already crossed the repository boundary, and every later refactor would be one
// forgotten line away from returning it.
func (r *GormSavedViewRepository) ListVisible(ctx context.Context, tenantID, userID uuid.UUID, tableID string) ([]domain.SavedView, error) {
	views := []domain.SavedView{}
	q := r.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Where("user_id = ? OR visibility = ?", userID, domain.SavedViewShared)
	if tableID != "" {
		q = q.Where("table_id = ?", tableID)
	}
	err := q.Order("name ASC").Find(&views).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list saved views: %w", err)
	}
	return views, nil
}

func (r *GormSavedViewRepository) Update(ctx context.Context, view *domain.SavedView) error {
	if view.TenantID == uuid.Nil {
		return fmt.Errorf("tenant_id is required")
	}
	// Select the mutable columns explicitly: a Save() here would let a payload
	// that carried a different user_id or tenant_id rewrite ownership.
	result := r.db.WithContext(ctx).
		Model(&domain.SavedView{}).
		Where("id = ? AND tenant_id = ?", view.ID, view.TenantID).
		Updates(map[string]interface{}{
			"name":       view.Name,
			"visibility": view.Visibility,
			"state":      view.State,
			"columns":    view.Columns,
			"updated_at": gorm.Expr("CURRENT_TIMESTAMP"),
		})
	if result.Error != nil {
		return fmt.Errorf("failed to update saved view: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *GormSavedViewRepository) Delete(ctx context.Context, id, tenantID uuid.UUID) error {
	result := r.db.WithContext(ctx).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		Delete(&domain.SavedView{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete saved view: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *GormSavedViewRepository) ExistsByName(ctx context.Context, tenantID, userID uuid.UUID, tableID, name string, excludeID uuid.UUID) (bool, error) {
	var count int64
	q := r.db.WithContext(ctx).
		Model(&domain.SavedView{}).
		Where("tenant_id = ? AND user_id = ? AND table_id = ? AND name = ?", tenantID, userID, tableID, name)
	if excludeID != uuid.Nil {
		q = q.Where("id <> ?", excludeID)
	}
	if err := q.Count(&count).Error; err != nil {
		return false, fmt.Errorf("failed to check saved view name: %w", err)
	}
	return count > 0, nil
}
