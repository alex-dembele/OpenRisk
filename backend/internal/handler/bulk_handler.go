// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	bulkapp "github.com/opendefender/openrisk/internal/application/bulk"
	"github.com/opendefender/openrisk/internal/domain"
	"github.com/opendefender/openrisk/pkg/validation"
)

// BulkHandler exposes governed bulk operations for ONE register (#582).
//
// One instance per register, each wired to its own engine and mounted on its own
// path behind its own permission. Criterion 4 — "when the endpoint is called
// directly it returns 403 and modifies nothing" — is that route middleware, not
// a check in here: UI gating alone does not satisfy it, and neither would a
// handler-level check that a future route could forget to reach.
type BulkHandler struct {
	engine *bulkapp.Engine
}

// NewBulkHandler wires a handler to one register's engine.
func NewBulkHandler(engine *bulkapp.Engine) *BulkHandler {
	return &BulkHandler{engine: engine}
}

// bulkRequestInput is the wire shape shared by preview and apply.
type bulkRequestInput struct {
	// Action is read only by the preview, which needs to know WHICH change to
	// project. The apply routes take it from the route instead — see Apply.
	Action        string   `json:"action" validate:"omitempty,oneof=change_status delete"`
	IDs           []string `json:"ids" validate:"required,min=1,max=100,dive,uuid"`
	Status        string   `json:"status" validate:"omitempty,max=64"`
	Justification string   `json:"justification" validate:"omitempty,max=1000"`
	// Fingerprint is what Preview returned. Sent back on apply so a selection
	// that moved underneath the user is refused rather than silently applied.
	Fingerprint string `json:"fingerprint" validate:"omitempty,hexadecimal,len=64"`
}

func (in *bulkRequestInput) toRequest() (bulkapp.Request, error) {
	ids := make([]uuid.UUID, 0, len(in.IDs))
	for _, raw := range in.IDs {
		id, err := uuid.Parse(raw)
		if err != nil {
			return bulkapp.Request{}, domain.NewValidationError("an id in the selection is not a uuid")
		}
		ids = append(ids, id)
	}
	return bulkapp.Request{
		IDs: ids,
		Change: domain.BulkChange{
			Action:        domain.BulkAction(in.Action),
			Status:        in.Status,
			Justification: in.Justification,
		},
		Fingerprint: in.Fingerprint,
	}, nil
}

// parseBulk reads and validates the body. It returns false once it has already
// written a response.
func (h *BulkHandler) parseBulk(c *fiber.Ctx) (bulkapp.Request, bool) {
	input := new(bulkRequestInput)
	if err := c.BodyParser(input); err != nil {
		_ = c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
		return bulkapp.Request{}, false
	}
	if err := validation.GetValidator().Struct(input); err != nil {
		_ = c.Status(fiber.StatusBadRequest).
			JSON(fiber.Map{"error": "validation_failed", "details": err.Error()})
		return bulkapp.Request{}, false
	}
	req, err := input.toRequest()
	if err != nil {
		_ = writeAppError(c, err)
		return bulkapp.Request{}, false
	}
	return req, true
}

// bulkCaller takes the identity from the signed session only. Never from the
// body: tenant_id is what every read and write in the batch is scoped by, so a
// client-supplied one would be a cross-tenant mutation primitive.
func bulkCaller(c *fiber.Ctx) bulkapp.Caller {
	return bulkapp.Caller{TenantID: tenantID(c), UserID: userID(c)}
}

// Preview — POST /api/v1/<register>/bulk/preview
//
// Says what a bulk action would do and changes nothing.
//
// Mounted behind the register's READ permission, deliberately: a preview
// discloses only the current state of rows the caller can already fetch
// one by one, and it writes nothing. Gating it harder would stop a user from
// finding out what an action would do before asking someone who can perform it.
func (h *BulkHandler) Preview(c *fiber.Ctx) error {
	req, ok := h.parseBulk(c)
	if !ok {
		return nil
	}
	preview, err := h.engine.Preview(auditCtx(c), bulkCaller(c), req)
	if err != nil {
		return writeAppError(c, err)
	}
	return c.JSON(preview)
}

// Apply returns the handler for ONE action — POST /api/v1/<register>/bulk/<action>.
//
// The action comes from the ROUTE, not the body, and that is the point: each
// action is mounted behind its own permission (change-status behind
// <module>:update, delete behind <module>:delete), so the gate always matches
// what is actually performed. Reading the action from the body would mean one
// route, one permission, and a caller with update rights able to post
// {"action":"delete"} — the criterion-4 hole this shape closes by construction.
//
// All-or-nothing (D-036). Errors go through writeAppError so a stale or foreign
// id answers 404 and a selection that moved since the preview answers 409,
// rather than everything collapsing into a 500.
func (h *BulkHandler) Apply(action domain.BulkAction) fiber.Handler {
	return func(c *fiber.Ctx) error {
		req, ok := h.parseBulk(c)
		if !ok {
			return nil
		}
		req.Change.Action = action
		result, err := h.engine.Apply(auditCtx(c), bulkCaller(c), req)
		if err != nil {
			return writeAppError(c, err)
		}
		return c.JSON(result)
	}
}

// Capabilities — GET /api/v1/<register>/bulk/capabilities
//
// What this register can actually do. The bulk bar reads it instead of
// hard-coding a per-module list, so a register that supports only delete cannot
// be offered a status change the server would refuse — criterion 4's "the action
// is not offered" half, driven by the server rather than duplicated in the UI.
func (h *BulkHandler) Capabilities(c *fiber.Ctx) error {
	if tenantID(c) == uuid.Nil {
		return writeAppError(c, domain.NewForbiddenError("no tenant in context"))
	}
	return c.JSON(fiber.Map{
		"entity_type": h.engine.EntityType(),
		"actions":     h.engine.SupportedActions(),
	})
}
