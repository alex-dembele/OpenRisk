// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package savedview

import (
	"context"

	"github.com/opendefender/openrisk/internal/domain"
)

// ListSavedViewsUseCase returns the views a caller may see for one register:
// their own, whatever the visibility, plus every view their colleagues chose to
// share. Another user's personal view is never in the result (#580 criterion 3).
type ListSavedViewsUseCase struct {
	repo  domain.SavedViewRepository
	users UserLookup
}

func NewListSavedViewsUseCase(repo domain.SavedViewRepository) *ListSavedViewsUseCase {
	return &ListSavedViewsUseCase{repo: repo}
}

// WithUserLookup resolves the owner of each shared view to an email for display.
// Optional — see resolveOwnerEmails.
func (uc *ListSavedViewsUseCase) WithUserLookup(users UserLookup) *ListSavedViewsUseCase {
	uc.users = users
	return uc
}

// Execute lists the visible views. An empty tableID lists every register's views,
// which is what the one-time localStorage migration needs to know what is already
// on the server.
func (uc *ListSavedViewsUseCase) Execute(ctx context.Context, caller Caller, tableID string) ([]domain.SavedView, error) {
	if err := caller.Validate(); err != nil {
		return nil, err
	}
	views, err := uc.repo.ListVisible(ctx, caller.TenantID, caller.UserID, tableID)
	if err != nil {
		return nil, domain.NewInternalError(err.Error())
	}
	resolveOwnerEmails(ctx, uc.users, views)
	return views, nil
}
