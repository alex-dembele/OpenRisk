// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package savedview

import (
	"context"

	"github.com/google/uuid"

	"github.com/opendefender/openrisk/internal/domain"
)

// UpdateSavedViewInput is a patch: a nil field means "leave it alone". Without
// that distinction, saving a renamed view would silently reset its filters.
type UpdateSavedViewInput struct {
	Name       *string
	Visibility *domain.SavedViewVisibility
	State      *domain.SavedViewState
	Columns    *domain.SavedViewColumns
}

// UpdateSavedViewUseCase edits an existing view. The caller must own it or be a
// tenant admin; anything they cannot even see is 404, never 403 (#580 criteria
// 4 and 5).
type UpdateSavedViewUseCase struct {
	repo domain.SavedViewRepository
}

func NewUpdateSavedViewUseCase(repo domain.SavedViewRepository) *UpdateSavedViewUseCase {
	return &UpdateSavedViewUseCase{repo: repo}
}

func (uc *UpdateSavedViewUseCase) Execute(ctx context.Context, caller Caller, id uuid.UUID, input UpdateSavedViewInput) (*domain.SavedView, error) {
	view, err := load(ctx, uc.repo, caller, id)
	if err != nil {
		return nil, err
	}
	if !view.CanBeEditedBy(caller.UserID, caller.IsAdmin) {
		// Reachable only for a SHARED view owned by someone else — the caller can
		// already see it, so naming the refusal leaks nothing.
		return nil, domain.NewForbiddenError("a shared view can only be edited by its owner or a tenant admin")
	}

	if input.Name != nil {
		view.Name = *input.Name
	}
	if input.Visibility != nil {
		view.Visibility = *input.Visibility
	}
	if input.State != nil {
		view.State = *input.State
	}
	if input.Columns != nil {
		view.Columns = *input.Columns
	}
	if err := view.Validate(); err != nil {
		return nil, err
	}

	// Name uniqueness is per OWNER, so a rename is checked against the owner's
	// other views — not the admin's, when an admin is the one doing it.
	exists, err := uc.repo.ExistsByName(ctx, caller.TenantID, view.UserID, view.TableID, view.Name, view.ID)
	if err != nil {
		return nil, domain.NewInternalError(err.Error())
	}
	if exists {
		return nil, domain.NewConflictError("saved view", "name")
	}

	if err := uc.repo.Update(ctx, view); err != nil {
		// The row was visible a moment ago; a zero-row update means it has since
		// been deleted. Same answer as any other absent id.
		return nil, domain.NewNotFoundError("saved view", id)
	}
	return view, nil
}
