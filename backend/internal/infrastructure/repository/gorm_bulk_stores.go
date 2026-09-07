// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/opendefender/openrisk/internal/domain"
)

// ---------------------------------------------------------------------------
// domain.BulkStore implementations (#582).
//
// Both stores below are deliberately thin: the transaction boundary, the
// all-or-nothing rule and the audit entries live once in
// application/bulk.Engine. What a store owns is its own columns and its own
// vocabulary — which is the only part that differs between registers.
//
// ABSOLUTE RULE 2: tenant_id is in the WHERE clause of every query here, reads
// and writes alike. A bulk endpoint is the worst place to drop one: a single
// unfiltered request mutates many rows across tenants at once.
// ---------------------------------------------------------------------------

// bulkApply is the shared body both stores use: load each row under the tenant
// predicate inside ONE transaction, mutate it, save it, and report before →
// after. Any id that does not resolve — stale, fabricated, or another tenant's —
// fails the whole batch with ErrNotFound and writes nothing.
//
// It mirrors GormRiskRepository.BulkApply rather than calling it, because that
// one is typed to *domain.Risk. The shape is identical on purpose: the two
// should stay recognisably the same thing.
func bulkApply[T any](
	ctx context.Context,
	db *gorm.DB,
	tenantID uuid.UUID,
	ids []uuid.UUID,
	entity string,
	project func(*T) domain.BulkRow,
	mutate func(*T) error,
) ([]domain.BulkMutation, error) {
	if tenantID == uuid.Nil {
		return nil, domain.NewForbiddenError("tenant_id is required")
	}
	if len(ids) == 0 {
		return nil, domain.NewValidationError("at least one id is required")
	}

	mutations := make([]domain.BulkMutation, 0, len(ids))

	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		mutations = mutations[:0]

		for _, id := range ids {
			var row T
			q := tx.Where("id = ? AND tenant_id = ?", id, tenantID)
			// Locked FOR UPDATE so a concurrent delete cannot land between the
			// read and the save. SQLite has no row locks and serialises writers,
			// and it is only ever the test harness.
			if tx.Dialector.Name() != "sqlite" {
				q = q.Clauses(clause.Locking{Strength: "UPDATE"})
			}
			if err := q.First(&row).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return domain.NewNotFoundError(entity, id)
				}
				return fmt.Errorf("failed to load %s %s: %w", entity, id, err)
			}

			before := project(&row)
			if err := mutate(&row); err != nil {
				return err
			}
			after := project(&row)

			if err := tx.Save(&row).Error; err != nil {
				return fmt.Errorf("failed to update %s %s: %w", entity, id, err)
			}

			mutations = append(mutations, domain.BulkMutation{
				ID: id, Label: before.Label,
				Before: before.Snapshot, After: after.Snapshot,
				ChangedFields: domain.BulkChangedFields(before.Snapshot, after.Snapshot),
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return mutations, nil
}

// bulkDelete soft-deletes the named rows, all or none. Strict: a short match
// rolls back rather than deleting whatever it could, which is also what makes a
// foreign-tenant id indistinguishable from a fabricated one.
func bulkDelete[T any](
	ctx context.Context,
	db *gorm.DB,
	tenantID uuid.UUID,
	ids []uuid.UUID,
	entity string,
	project func(*T) domain.BulkRow,
) ([]domain.BulkMutation, error) {
	if tenantID == uuid.Nil {
		return nil, domain.NewForbiddenError("tenant_id is required")
	}
	if len(ids) == 0 {
		return nil, domain.NewValidationError("at least one id is required")
	}

	mutations := make([]domain.BulkMutation, 0, len(ids))

	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		mutations = mutations[:0]

		// Read before deleting: the audit trail records WHAT went, and a
		// deletion whose "before" is unknown explains nothing afterwards.
		var rows []T
		if err := tx.Where("id IN ? AND tenant_id = ?", ids, tenantID).Find(&rows).Error; err != nil {
			return fmt.Errorf("failed to load %s batch: %w", entity, err)
		}
		if len(rows) != len(ids) {
			// Which id was missing is not named: it would confirm the others
			// exist, and the caller supplied the list anyway.
			return domain.NewNotFoundError(entity, "one or more ids in the batch")
		}

		result := tx.Where("id IN ? AND tenant_id = ?", ids, tenantID).Delete(new(T))
		if result.Error != nil {
			return fmt.Errorf("failed to delete %s batch: %w", entity, result.Error)
		}
		if result.RowsAffected != int64(len(ids)) {
			return domain.NewNotFoundError(entity, "one or more ids in the batch")
		}

		for i := range rows {
			row := project(&rows[i])
			mutations = append(mutations, domain.BulkMutation{
				// The id must come from the row itself: an audit entry naming
				// uuid.Nil records that something was deleted without saying what.
				ID: row.ID, Label: row.Label,
				Before: row.Snapshot, After: map[string]interface{}{},
				ChangedFields: []string{"deleted"},
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return mutations, nil
}

// bulkLoad reads the named rows without locking or mutating — the read-only half
// the preview runs on.
func bulkLoad[T any](
	ctx context.Context,
	db *gorm.DB,
	tenantID uuid.UUID,
	ids []uuid.UUID,
	entity string,
	project func(*T) domain.BulkRow,
) ([]domain.BulkRow, error) {
	if tenantID == uuid.Nil {
		return nil, domain.NewForbiddenError("tenant_id is required")
	}
	if len(ids) == 0 {
		return []domain.BulkRow{}, nil
	}

	var rows []T
	if err := db.WithContext(ctx).
		Where("id IN ? AND tenant_id = ?", ids, tenantID).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to load %s batch: %w", entity, err)
	}

	out := make([]domain.BulkRow, 0, len(rows))
	for i := range rows {
		out = append(out, project(&rows[i]))
	}
	return out, nil
}

// ===========================================================================
// Vulnerabilities — change_status + delete
// ===========================================================================

// VulnerabilityBulkStore gives the vulnerability register governed bulk
// operations. Triage is the motivating case: a compliance officer working
// through two hundred findings marks a batch triaged or false-positive in one
// action instead of two hundred.
type VulnerabilityBulkStore struct{ db *gorm.DB }

// NewVulnerabilityBulkStore wires the store.
func NewVulnerabilityBulkStore(db *gorm.DB) *VulnerabilityBulkStore {
	return &VulnerabilityBulkStore{db: db}
}

func (s *VulnerabilityBulkStore) EntityType() string { return "vulnerability" }

// SupportedBulkActions — the vulnerability register carries a Status, so
// change_status applies. It has no tags and no structured assignee column, so
// assign_to and the tag actions are honestly absent rather than accepted and
// silently ignored.
func (s *VulnerabilityBulkStore) SupportedBulkActions() []domain.BulkAction {
	return []domain.BulkAction{domain.BulkActionChangeStatus, domain.BulkActionDelete}
}

func (s *VulnerabilityBulkStore) ValidateBulkChange(change domain.BulkChange) error {
	if change.Action != domain.BulkActionChangeStatus {
		return nil
	}
	switch domain.VulnStatus(change.Status) {
	case domain.VulnStatusOpen, domain.VulnStatusTriaged, domain.VulnStatusInRemediation,
		domain.VulnStatusRemediated, domain.VulnStatusAccepted, domain.VulnStatusFalsePositive:
		return nil
	default:
		return domain.NewValidationError(fmt.Sprintf(
			"unknown vulnerability status %q (expected one of: open, triaged, in_remediation, remediated, accepted, false_positive)",
			change.Status))
	}
}

// vulnSnapshot is the narrow projection a bulk action may touch. Narrow on
// purpose: journalling every column would bury the change, and drag unrelated
// columns — including any added later — into the audit trail.
func vulnRow(v *domain.Vulnerability) domain.BulkRow {
	label := v.Title
	if v.CVEID != "" {
		label = v.CVEID
	}
	return domain.BulkRow{
		ID:       v.ID,
		Label:    label,
		Snapshot: map[string]interface{}{"status": string(v.Status)},
	}
}

func (s *VulnerabilityBulkStore) LoadBulk(ctx context.Context, tenantID uuid.UUID, ids []uuid.UUID) ([]domain.BulkRow, error) {
	return bulkLoad(ctx, s.db, tenantID, ids, "vulnerability", vulnRow)
}

func (s *VulnerabilityBulkStore) ApplyBulk(
	ctx context.Context, tenantID uuid.UUID, ids []uuid.UUID, change domain.BulkChange,
) ([]domain.BulkMutation, error) {
	return bulkApply(ctx, s.db, tenantID, ids, "vulnerability", vulnRow,
		func(v *domain.Vulnerability) error {
			if change.Action == domain.BulkActionChangeStatus {
				v.Status = domain.VulnStatus(change.Status)
			}
			return nil
		})
}

func (s *VulnerabilityBulkStore) DeleteBulk(ctx context.Context, tenantID uuid.UUID, ids []uuid.UUID) ([]domain.BulkMutation, error) {
	return bulkDelete(ctx, s.db, tenantID, ids, "vulnerability", vulnRow)
}

// ===========================================================================
// Assets — delete only
// ===========================================================================

// AssetBulkStore gives the asset inventory governed bulk operations.
//
// DELETE ONLY, and that is a finding rather than a shortcut: domain.Asset has no
// Status column, no Tags, and its Owner is a free-text string rather than a user
// reference (see the field list on domain.Asset). Of #581's frozen action set —
// which #582 forbids extending — only delete maps onto this model. Bulk
// re-classification by Criticality would be a NEW action type and needs its own
// issue.
type AssetBulkStore struct{ db *gorm.DB }

// NewAssetBulkStore wires the store.
func NewAssetBulkStore(db *gorm.DB) *AssetBulkStore { return &AssetBulkStore{db: db} }

func (s *AssetBulkStore) EntityType() string { return "asset" }

func (s *AssetBulkStore) SupportedBulkActions() []domain.BulkAction {
	return []domain.BulkAction{domain.BulkActionDelete}
}

func (s *AssetBulkStore) ValidateBulkChange(domain.BulkChange) error { return nil }

func assetRow(a *domain.Asset) domain.BulkRow {
	return domain.BulkRow{
		ID:    a.ID,
		Label: a.Name,
		Snapshot: map[string]interface{}{
			"name":        a.Name,
			"type":        a.Type,
			"criticality": string(a.Criticality),
		},
	}
}

func (s *AssetBulkStore) LoadBulk(ctx context.Context, tenantID uuid.UUID, ids []uuid.UUID) ([]domain.BulkRow, error) {
	return bulkLoad(ctx, s.db, tenantID, ids, "asset", assetRow)
}

// ApplyBulk is unreachable for assets — SupportedBulkActions declares delete
// only, and the engine refuses anything else before it gets here. It is
// implemented to satisfy the port, and refuses explicitly rather than silently
// succeeding if that guard ever regresses.
func (s *AssetBulkStore) ApplyBulk(
	_ context.Context, _ uuid.UUID, _ []uuid.UUID, change domain.BulkChange,
) ([]domain.BulkMutation, error) {
	return nil, domain.NewValidationError(fmt.Sprintf(
		"the asset inventory supports no %q bulk action", change.Action))
}

func (s *AssetBulkStore) DeleteBulk(ctx context.Context, tenantID uuid.UUID, ids []uuid.UUID) ([]domain.BulkMutation, error) {
	return bulkDelete(ctx, s.db, tenantID, ids, "asset", assetRow)
}
