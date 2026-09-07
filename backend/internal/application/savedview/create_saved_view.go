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

// CreateSavedViewInput describes a view to store. TenantID and UserID are
// deliberately absent: they come from the Caller, which comes from the signed
// session, so a payload cannot name whose view this is.
type CreateSavedViewInput struct {
	TableID    string
	Name       string
	Visibility domain.SavedViewVisibility
	State      domain.SavedViewState
	Columns    domain.SavedViewColumns
}

// CreateSavedViewUseCase stores one named view for the calling user.
type CreateSavedViewUseCase struct {
	repo domain.SavedViewRepository
}

func NewCreateSavedViewUseCase(repo domain.SavedViewRepository) *CreateSavedViewUseCase {
	return &CreateSavedViewUseCase{repo: repo}
}

func (uc *CreateSavedViewUseCase) Execute(ctx context.Context, caller Caller, input CreateSavedViewInput) (*domain.SavedView, error) {
	if err := caller.Validate(); err != nil {
		return nil, err
	}

	visibility := input.Visibility
	if visibility == "" {
		visibility = domain.SavedViewPersonal
	}

	view := &domain.SavedView{
		ID:         uuid.New(),
		TenantID:   caller.TenantID,
		UserID:     caller.UserID,
		TableID:    input.TableID,
		Name:       input.Name,
		Visibility: visibility,
		State:      input.State,
		Columns:    input.Columns,
	}
	if err := view.Validate(); err != nil {
		return nil, err
	}

	// One name per (user, table). The frontend's localStorage implementation
	// replaced a same-named view silently; server-side that would let a retried
	// migration overwrite a view the user had since edited, so it is a conflict
	// the caller is told about instead.
	exists, err := uc.repo.ExistsByName(ctx, caller.TenantID, caller.UserID, view.TableID, view.Name, uuid.Nil)
	if err != nil {
		return nil, domain.NewInternalError(err.Error())
	}
	if exists {
		return nil, domain.NewConflictError("saved view", "name")
	}

	if err := uc.repo.Create(ctx, view); err != nil {
		return nil, domain.NewInternalError(err.Error())
	}
	return view, nil
}
