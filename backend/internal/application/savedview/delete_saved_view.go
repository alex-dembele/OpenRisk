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

// DeleteSavedViewUseCase removes a view. Same authorisation ladder as the
// update: invisible is 404, visible-but-not-yours is 403.
type DeleteSavedViewUseCase struct {
	repo domain.SavedViewRepository
}

func NewDeleteSavedViewUseCase(repo domain.SavedViewRepository) *DeleteSavedViewUseCase {
	return &DeleteSavedViewUseCase{repo: repo}
}

func (uc *DeleteSavedViewUseCase) Execute(ctx context.Context, caller Caller, id uuid.UUID) error {
	view, err := load(ctx, uc.repo, caller, id)
	if err != nil {
		return err
	}
	if !view.CanBeEditedBy(caller.UserID, caller.IsAdmin) {
		return domain.NewForbiddenError("a shared view can only be deleted by its owner or a tenant admin")
	}
	if err := uc.repo.Delete(ctx, id, caller.TenantID); err != nil {
		return domain.NewNotFoundError("saved view", id)
	}
	return nil
}
