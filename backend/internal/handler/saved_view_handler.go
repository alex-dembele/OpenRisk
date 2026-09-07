// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	savedviewapp "github.com/opendefender/openrisk/internal/application/savedview"
	"github.com/opendefender/openrisk/internal/domain"
	"github.com/opendefender/openrisk/internal/middleware"
	"github.com/opendefender/openrisk/pkg/validation"
)

// SavedViewHandler exposes server-side saved table views (#580): the named
// filter/sort/column combinations a user keeps for a register, and optionally
// shares with their tenant.
//
// There is no permission gate on these routes and that is deliberate. The table
// is generic across seven registers, so no single `<module>:read` permission is
// the right one, and a saved view grants no access to data — applying it still
// runs the register's own query under the caller's own permissions. What the
// routes DO enforce, on every call, is the tenant and user identity taken from
// the signed session.
type SavedViewHandler struct {
	listUC   *savedviewapp.ListSavedViewsUseCase
	createUC *savedviewapp.CreateSavedViewUseCase
	updateUC *savedviewapp.UpdateSavedViewUseCase
	deleteUC *savedviewapp.DeleteSavedViewUseCase
}

func NewSavedViewHandler(
	list *savedviewapp.ListSavedViewsUseCase,
	create *savedviewapp.CreateSavedViewUseCase,
	update *savedviewapp.UpdateSavedViewUseCase,
	del *savedviewapp.DeleteSavedViewUseCase,
) *SavedViewHandler {
	return &SavedViewHandler{listUC: list, createUC: create, updateUC: update, deleteUC: del}
}

// savedViewCaller derives the acting identity from the JWT alone. Nothing here
// may come from the query string or the body — the tenant id is what every
// saved-view query filters on, so a client-supplied one is a cross-tenant read.
func savedViewCaller(c *fiber.Ctx) savedviewapp.Caller {
	caller := savedviewapp.Caller{
		TenantID: safeGetUUID(c, "tenant_id"),
		UserID:   safeGetUUID(c, "user_id"),
	}
	// Session-authenticated requests carry the same identity on the middleware
	// context instead of the locals; take it from there when the locals are empty.
	if caller.TenantID == uuid.Nil {
		caller.TenantID = tenantID(c)
	}
	if caller.UserID == uuid.Nil {
		caller.UserID = userID(c)
	}

	claims := middleware.GetUserClaims(c)
	if claims == nil {
		return caller
	}
	if claims.HasPermission("*") {
		caller.IsAdmin = true
	}
	for _, r := range claims.OrgRoles {
		if r == "admin" || r == "root" {
			caller.IsAdmin = true
		}
	}
	return caller
}

// savedViewStateInput mirrors the frontend TableState subset a view restores.
type savedViewStateInput struct {
	Q       string              `json:"q"`
	Filters map[string][]string `json:"filters"`
	Sort    *savedViewSortInput `json:"sort"`
}

type savedViewSortInput struct {
	Key string `json:"key" validate:"required"`
	Dir string `json:"dir" validate:"omitempty,oneof=asc desc"`
}

type savedViewColumnsInput struct {
	Order  []string `json:"order"`
	Hidden []string `json:"hidden"`
}

func (in *savedViewStateInput) toDomain() domain.SavedViewState {
	state := domain.SavedViewState{Q: in.Q, Filters: in.Filters}
	if in.Sort != nil {
		state.Sort = &domain.SavedViewSort{Key: in.Sort.Key, Dir: in.Sort.Dir}
	}
	return state
}

func (in *savedViewColumnsInput) toDomain() domain.SavedViewColumns {
	if in == nil {
		return domain.SavedViewColumns{}
	}
	return domain.SavedViewColumns{Order: in.Order, Hidden: in.Hidden}
}

// ListSavedViews — GET /api/v1/saved-views?table_id=risks
//
// Returns the caller's own views plus the tenant's shared ones. Omitting
// table_id returns every register's views, which is what the one-time
// localStorage migration reads to know what is already stored.
func (h *SavedViewHandler) ListSavedViews(c *fiber.Ctx) error {
	views, err := h.listUC.Execute(c.UserContext(), savedViewCaller(c), c.Query("table_id"))
	if err != nil {
		return writeAppError(c, err)
	}
	return c.JSON(views)
}

type createSavedViewInput struct {
	TableID    string                 `json:"table_id" validate:"required,max=64"`
	Name       string                 `json:"name" validate:"required,max=120"`
	Visibility string                 `json:"visibility" validate:"omitempty,oneof=personal shared"`
	State      savedViewStateInput    `json:"state"`
	Columns    *savedViewColumnsInput `json:"columns"`
}

// CreateSavedView — POST /api/v1/saved-views
func (h *SavedViewHandler) CreateSavedView(c *fiber.Ctx) error {
	input := new(createSavedViewInput)
	if err := c.BodyParser(input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid input format"})
	}
	if err := validation.GetValidator().Struct(input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "validation_failed", "details": err.Error()})
	}
	visibility, err := domain.ParseSavedViewVisibility(input.Visibility)
	if err != nil {
		return writeAppError(c, err)
	}

	view, err := h.createUC.Execute(c.UserContext(), savedViewCaller(c), savedviewapp.CreateSavedViewInput{
		TableID:    input.TableID,
		Name:       input.Name,
		Visibility: visibility,
		State:      input.State.toDomain(),
		Columns:    input.Columns.toDomain(),
	})
	if err != nil {
		return writeAppError(c, err)
	}
	return c.Status(201).JSON(view)
}

// updateSavedViewInput is a patch — an absent field is left alone, so renaming a
// view cannot silently reset its filters.
type updateSavedViewInput struct {
	Name       *string                `json:"name" validate:"omitempty,max=120"`
	Visibility *string                `json:"visibility" validate:"omitempty,oneof=personal shared"`
	State      *savedViewStateInput   `json:"state"`
	Columns    *savedViewColumnsInput `json:"columns"`
}

// UpdateSavedView — PATCH /api/v1/saved-views/:id
func (h *SavedViewHandler) UpdateSavedView(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid saved view id"})
	}
	input := new(updateSavedViewInput)
	if err := c.BodyParser(input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid input format"})
	}
	if err := validation.GetValidator().Struct(input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "validation_failed", "details": err.Error()})
	}

	patch := savedviewapp.UpdateSavedViewInput{Name: input.Name}
	if input.Visibility != nil {
		visibility, err := domain.ParseSavedViewVisibility(*input.Visibility)
		if err != nil {
			return writeAppError(c, err)
		}
		patch.Visibility = &visibility
	}
	if input.State != nil {
		state := input.State.toDomain()
		patch.State = &state
	}
	if input.Columns != nil {
		columns := input.Columns.toDomain()
		patch.Columns = &columns
	}

	view, err := h.updateUC.Execute(c.UserContext(), savedViewCaller(c), id, patch)
	if err != nil {
		return writeAppError(c, err)
	}
	return c.JSON(view)
}

// DeleteSavedView — DELETE /api/v1/saved-views/:id
func (h *SavedViewHandler) DeleteSavedView(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid saved view id"})
	}
	if err := h.deleteUC.Execute(c.UserContext(), savedViewCaller(c), id); err != nil {
		return writeAppError(c, err)
	}
	return c.SendStatus(204)
}
