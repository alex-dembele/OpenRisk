// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/opendefender/openrisk/internal/domain"
	"github.com/opendefender/openrisk/internal/middleware"
	"github.com/opendefender/openrisk/internal/service"
)

// BulkOperationHandler handles bulk operation endpoints
type BulkOperationHandler struct {
	service *service.BulkOperationService
}

// NewBulkOperationHandler creates a new bulk operation handler
func NewBulkOperationHandler() *BulkOperationHandler {
	return &BulkOperationHandler{
		service: service.NewBulkOperationService(),
	}
}

// bulkOperationPermission maps each bulk verb to the permission its per-entity
// equivalent requires, so a bulk job can never be a cheaper route to a
// mutation than doing it one record at a time.
var bulkOperationPermission = map[domain.BulkOperationType]string{
	domain.BulkOperationTypeUpdate: "risks:update",
	domain.BulkOperationTypeDelete: "risks:delete",
	domain.BulkOperationTypeExport: "risks:read",
	domain.BulkOperationTypeAssign: "mitigations:update",
}

// CreateBulkOperation handles POST /bulk-operations
// Creates a new bulk operation job
func (h *BulkOperationHandler) CreateBulkOperation(c *fiber.Ctx) error {
	userClaims := middleware.GetUserClaims(c)
	if userClaims == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Unauthorized",
		})
	}

	// Parse request
	req := &domain.CreateBulkOperationRequest{}
	if err := c.BodyParser(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate operation type
	if req.OperationType == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Missing required field: operation_type",
		})
	}

	// #529 — a bulk job is the same mutation as the single-entity route, applied
	// N times: `delete` with an empty filter removes every risk in the tenant.
	// The route's middleware guard cannot tell which, because the verb is in the
	// BODY, so the exact permission is checked here. Without this, any
	// authenticated member — a Viewer included — could empty the register while
	// DELETE /risks/:id demands risks:delete.
	perm, known := bulkOperationPermission[req.OperationType]
	if !known {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid operation type: " + string(req.OperationType),
		})
	}
	if !userClaims.HasPermission(perm) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"code":    "FORBIDDEN",
			"message": "Missing required permission: " + perm,
		})
	}

	// Resolve tenant from the request context — every resource query is scoped to it.
	rc := middleware.GetContext(c)
	if rc == nil || rc.OrganizationID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "No tenant context"})
	}

	// Create bulk operation
	op, err := h.service.CreateBulkOperation(userClaims.Sub, rc.OrganizationID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(op)
}

// GetBulkOperation handles GET /bulk-operations/:id
// Retrieves a specific bulk operation
func (h *BulkOperationHandler) GetBulkOperation(c *fiber.Ctx) error {
	opID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid operation ID",
		})
	}

	rc := middleware.GetContext(c)
	if rc == nil || rc.OrganizationID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "No tenant context"})
	}

	op, err := h.service.GetBulkOperation(opID, rc.OrganizationID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Operation not found",
		})
	}

	return c.JSON(op)
}

// ListBulkOperations handles GET /bulk-operations
// Lists bulk operations for the authenticated user
func (h *BulkOperationHandler) ListBulkOperations(c *fiber.Ctx) error {
	userClaims := middleware.GetUserClaims(c)
	if userClaims == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Unauthorized",
		})
	}

	limit := 20
	if l := c.QueryInt("limit"); l > 0 && l <= 100 {
		limit = l
	}

	offset := 0
	if o := c.QueryInt("offset"); o >= 0 {
		offset = o
	}

	rc := middleware.GetContext(c)
	if rc == nil || rc.OrganizationID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "No tenant context"})
	}

	ops, err := h.service.ListBulkOperations(userClaims.Sub, rc.OrganizationID, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"operations": ops,
		"count":      len(ops),
	})
}
