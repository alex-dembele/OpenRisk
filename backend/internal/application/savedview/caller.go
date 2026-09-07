// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

// Package savedview holds the use cases behind server-side saved table views
// (#580): a named combination of search, facets, sort and column layout that a
// risk manager can keep and, if they choose, hand to their whole tenant.
//
// The isolation contract these use cases enforce, stated once here because every
// file below depends on it:
//
//   - A view is addressed only through a tenant-scoped repository read. A view
//     belonging to another tenant reads back as nil and is reported as 404 —
//     byte-identical to the answer for an id that was never issued. A 403 would
//     confirm the row exists, and a saved view's filter values name that
//     institution's assets, tags and people (#580 criterion 5).
//   - Another user's PERSONAL view inside the same tenant is likewise 404: it is
//     invisible, so its existence must not be confirmed either.
//   - A SHARED view of the same tenant is visible to all; editing or deleting it
//     is refused with 403 unless the caller owns it or is a tenant admin
//     (#580 criterion 4). Here 403 is correct precisely because the caller can
//     already see the row — there is nothing left to leak.
package savedview

import (
	"context"

	"github.com/google/uuid"

	"github.com/opendefender/openrisk/internal/domain"
)

// Caller is the authenticated identity acting on a view. It is assembled by the
// handler from the signed session context — never from the request body, which
// is how POST /scanner/mitigations/auto-complete became an IDOR.
type Caller struct {
	TenantID uuid.UUID
	UserID   uuid.UUID
	// IsAdmin marks a tenant admin (or root). It widens edit rights over SHARED
	// views only; it never reveals another user's personal view.
	IsAdmin bool
}

// Validate fails closed on a zero identity. A use case that accepted uuid.Nil
// would run an unscoped query rather than refusing.
func (c Caller) Validate() error {
	if c.TenantID == uuid.Nil {
		return domain.NewForbiddenError("no tenant in context")
	}
	if c.UserID == uuid.Nil {
		return domain.NewForbiddenError("no user in context")
	}
	return nil
}

// UserLookup resolves user IDs to emails so a shared view can say who shared it
// instead of showing a UUID. Optional everywhere it is used: without it, views
// simply carry an empty owner_email. Structurally satisfied by the existing user
// repository, so no new port is added to the domain.
type UserLookup interface {
	EmailsByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error)
}

// resolveOwnerEmails fills OwnerEmail on each view, best effort. A failure to
// resolve display names must never fail the read: the views are the payload, the
// names are decoration.
func resolveOwnerEmails(ctx context.Context, users UserLookup, views []domain.SavedView) {
	if users == nil || len(views) == 0 {
		return
	}
	seen := make(map[uuid.UUID]struct{}, len(views))
	ids := make([]uuid.UUID, 0, len(views))
	for _, v := range views {
		if _, ok := seen[v.UserID]; ok {
			continue
		}
		seen[v.UserID] = struct{}{}
		ids = append(ids, v.UserID)
	}
	emails, err := users.EmailsByIDs(ctx, ids)
	if err != nil {
		return
	}
	for i := range views {
		views[i].OwnerEmail = emails[views[i].UserID]
	}
}

// load fetches a view for a caller and applies the visibility rule that both the
// update and the delete path depend on. It returns ErrNotFound for anything the
// caller may not see — foreign tenant, absent id, or another user's personal
// view — so the three cases are indistinguishable from outside.
func load(ctx context.Context, repo domain.SavedViewRepository, caller Caller, id uuid.UUID) (*domain.SavedView, error) {
	if err := caller.Validate(); err != nil {
		return nil, err
	}
	if id == uuid.Nil {
		return nil, domain.NewValidationError("view id is required")
	}
	view, err := repo.GetByID(ctx, id, caller.TenantID)
	if err != nil {
		return nil, domain.NewInternalError(err.Error())
	}
	if view == nil || !view.IsVisibleTo(caller.UserID) {
		return nil, domain.NewNotFoundError("saved view", id)
	}
	return view, nil
}
