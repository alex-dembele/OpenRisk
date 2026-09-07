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

// CreateWithSubActions writes a mitigation plan and its whole checklist inside
// one transaction, so the create either lands whole or leaves nothing behind.
//
// Why this exists (#335): creating the plan and then looping over the
// sub-actions through a second repository call is a multi-table write with no
// transaction. When a sub-action insert failed, the caller got an error while
// the plan row — and every sub-action up to the failure — was already
// committed. The user saw an error, retried, and ended up with two plans, one
// of them holding half a checklist.
//
// The transaction lives in the repository rather than the use case because the
// application layer may not import GORM and therefore cannot hold a
// transaction handle. Side effects that must NOT be inside the transaction —
// the activation recorder, the ownership notifier — stay in the use case and
// run after this returns: holding the transaction open across a notification
// would turn a slow notifier into a held lock on the hot create path.
func (r *GormMitigationRepository) CreateWithSubActions(
	tenantID string,
	mitigation *domain.Mitigation,
	subActions []*domain.MitigationSubAction,
) error {
	if mitigation == nil {
		return errors.New("mitigation is required")
	}
	if mitigation.TenantID == uuid.Nil {
		return errors.New("tenant_id is required")
	}

	tenantUUID, err := uuid.Parse(tenantID)
	if err != nil {
		return fmt.Errorf("invalid tenant_id: %w", err)
	}
	// The caller's tenant and the row's tenant must be the same one. Without
	// this a caller could write a plan into another tenant by handing over a
	// pre-populated entity (ABSOLUTE RULE 2).
	if mitigation.TenantID != tenantUUID {
		return fmt.Errorf("%w: mitigation tenant_id does not match the caller", domain.ErrForbidden)
	}

	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(mitigation).Error; err != nil {
			return fmt.Errorf("failed to create mitigation: %w", err)
		}

		for _, subAction := range subActions {
			if subAction == nil {
				continue
			}
			// mitigation_subactions has no tenant_id column of its own; the row
			// is gated through its parent plan, whose tenant was verified above.
			// Binding the parent id here rather than trusting the caller keeps
			// that gate true by construction.
			subAction.MitigationID = mitigation.ID

			if err := tx.Create(subAction).Error; err != nil {
				return fmt.Errorf("failed to create subaction: %w", err)
			}
		}

		return nil
	})
}
