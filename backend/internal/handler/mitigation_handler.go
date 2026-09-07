// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package handler

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/opendefender/openrisk/internal/application/mitigation"
	"github.com/opendefender/openrisk/internal/domain"
	"github.com/opendefender/openrisk/internal/infrastructure/database"
	"github.com/opendefender/openrisk/internal/infrastructure/repository"
	"github.com/opendefender/openrisk/internal/middleware"
)

// CreateMitigation creates a new mitigation plan for a risk
func CreateMitigation(c *fiber.Ctx) error {
	ctx := middleware.GetContext(c)
	if ctx == nil || ctx.UserID == uuid.Nil {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}

	riskID := c.Params("id")
	if _, err := uuid.Parse(riskID); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid risk ID"})
	}

	payload := struct {
		Title       string   `json:"title"`
		Description string   `json:"description"`
		Priority    string   `json:"priority"`
		AssignedTo  []string `json:"assigned_to"`
		DueDate     *string  `json:"due_date"`
		// Ownership — responsable / exécutant / validateur from <UserPicker>.
		domain.OwnershipPatch
		SubActions []struct {
			Title       string  `json:"title"`
			Description string  `json:"description"`
			DueDate     *string `json:"due_date"`
		} `json:"sub_actions"`
	}{}

	if err := c.BodyParser(&payload); err != nil || payload.Title == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid payload: title is required"})
	}

	// Convert assignedTo strings to UUIDs
	var assignedTo domain.UUIDArray
	for _, s := range payload.AssignedTo {
		if id, err := uuid.Parse(s); err == nil {
			assignedTo = append(assignedTo, id)
		}
	}

	var dueDate *time.Time
	if payload.DueDate != nil {
		if t, err := time.Parse(time.RFC3339, *payload.DueDate); err == nil {
			dueDate = &t
		}
	}

	// Build subactions input
	subActions := make([]struct {
		Title       string
		Description string
		DueDate     *time.Time
	}, len(payload.SubActions))
	for i, sa := range payload.SubActions {
		var saDueDate *time.Time
		if sa.DueDate != nil {
			if t, err := time.Parse(time.RFC3339, *sa.DueDate); err == nil {
				saDueDate = &t
			}
		}
		subActions[i] = struct {
			Title       string
			Description string
			DueDate     *time.Time
		}{
			Title:       sa.Title,
			Description: sa.Description,
			DueDate:     saDueDate,
		}
	}

	// Use case
	repo := repository.NewGormMitigationRepository(database.DB)
	subRepo := repository.NewGormMitigationSubActionRepository(database.DB)
	useCase := mitigation.NewCreateMitigationPlanUseCase(repo, subRepo).
		WithOwnership(OwnershipServiceInstance())
	// Activation is recorded inside the use case; this legacy handler builds its
	// own dependencies, so the recorder is carried in via the package seam
	// (see activation_wiring.go). Nil-safe when unwired.
	if rec := ActivationRecorderInstance(); rec != nil {
		useCase = useCase.WithActivation(rec)
	}

	input := mitigation.CreateMitigationPlanInput{
		TenantID:    ctx.OrganizationID,
		RiskID:      uuid.MustParse(riskID),
		Title:       payload.Title,
		Description: payload.Description,
		Priority:    domain.MitigationPriority(payload.Priority),
		AssignedTo:  assignedTo,
		DueDate:     dueDate,
		CreatedBy:   ctx.UserID,
		Source:      domain.SourceManual,
		SubActions:  subActions,
		Ownership:   payload.OwnershipPatch,
		Locale:      c.Query("locale", "fr"),
	}

	output, err := useCase.ExecuteContext(c.UserContext(), input)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	// The write has committed. From here on the request has succeeded and the
	// client must be told so — returning a 5xx now would report a failure for a
	// plan that exists, and the user's natural response is to create a second
	// one (#335).
	//
	// The read-back is still attempted, because it returns the row with its
	// relations resolved, which is a richer answer than the entity we just
	// wrote. But it is now an enrichment, not a gate: if it fails we answer
	// from what the use case committed.
	if createdPlan, err := repo.GetByIDWithSubActions(ctx.OrganizationID.String(), output.ID); err == nil {
		return c.Status(201).JSON(createdPlan)
	}

	return c.Status(201).JSON(output.Plan)
}

// GetMitigation retrieves a mitigation plan by ID
func GetMitigation(c *fiber.Ctx) error {
	ctx := middleware.GetContext(c)
	if ctx == nil || ctx.OrganizationID == uuid.Nil {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}

	planID := c.Params("id")
	if _, err := uuid.Parse(planID); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid plan ID"})
	}

	repo := repository.NewGormMitigationRepository(database.DB)
	plan, err := repo.GetByIDWithSubActions(ctx.OrganizationID.String(), uuid.MustParse(planID))
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Mitigation not found"})
	}

	return c.JSON(plan)
}

// ListMitigations retrieves all mitigations for the tenant, optionally filtered by
// status/priority/risk_id. Backs the Mitigation Kanban board's bare GET /mitigations
// call — this route never existed before, so the Kanban page has never been able to load.
func ListMitigations(c *fiber.Ctx) error {
	ctx := middleware.GetContext(c)
	if ctx == nil || ctx.OrganizationID == uuid.Nil {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}

	filters := map[string]interface{}{}
	if status := c.Query("status"); status != "" {
		filters["status"] = status
	}
	if priority := c.Query("priority"); priority != "" {
		filters["priority"] = priority
	}
	if riskID := c.Query("risk_id"); riskID != "" {
		filters["risk_id"] = riskID
	}
	// "Mes mitigations" — anything the caller owns, executes, or must validate.
	// Taken from the AUTHENTICATED user, never from a parameter, so
	// "mine=<someone else>" is not expressible.
	if c.Query("mine") == "true" && ctx.UserID != uuid.Nil {
		filters["involved_user"] = ctx.UserID
	}

	repo := repository.NewGormMitigationRepository(database.DB)
	plans, err := repo.List(ctx.OrganizationID.String(), filters)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to retrieve mitigations"})
	}

	return c.JSON(fiber.Map{"items": plans, "total": len(plans)})
}

// ListMitigationsByRisk retrieves all mitigations for a risk
func ListMitigationsByRisk(c *fiber.Ctx) error {
	ctx := middleware.GetContext(c)
	if ctx == nil || ctx.OrganizationID == uuid.Nil {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}

	riskID := c.Params("id")
	if _, err := uuid.Parse(riskID); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid risk ID"})
	}

	// Verify the parent risk belongs to this tenant. Without this the endpoint
	// returned 200 [] for a foreign/nonexistent risk, which is inconsistent with
	// the 404 convention every other by-id route follows (audit-2026 #248).
	var riskCount int64
	if err := database.DB.Table("risks").
		Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", riskID, ctx.OrganizationID).
		Count(&riskCount).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to retrieve mitigations"})
	}
	if riskCount == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "risk not found"})
	}

	repo := repository.NewGormMitigationRepository(database.DB)
	plans, err := repo.ListByRiskID(ctx.OrganizationID.String(), uuid.MustParse(riskID))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to retrieve mitigations"})
	}

	return c.JSON(plans)
}

// UpdateMitigation updates a mitigation plan
func UpdateMitigation(c *fiber.Ctx) error {
	ctx := middleware.GetContext(c)
	if ctx == nil || ctx.OrganizationID == uuid.Nil {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}

	planID := c.Params("id")
	if _, err := uuid.Parse(planID); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid plan ID"})
	}

	payload := struct {
		Title       *string  `json:"title"`
		Description *string  `json:"description"`
		Status      *string  `json:"status"`
		Priority    *string  `json:"priority"`
		AssignedTo  []string `json:"assigned_to"`
		DueDate     *string  `json:"due_date"`
		// Ownership — tri-state; an absent key leaves the slot alone.
		domain.OwnershipPatch
	}{}

	if err := c.BodyParser(&payload); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid payload"})
	}

	var status *domain.MitigationStatus
	if payload.Status != nil {
		s := domain.MitigationStatus(*payload.Status)
		status = &s
	}

	// Convert assignedTo if provided
	var assignedTo *domain.UUIDArray
	if len(payload.AssignedTo) > 0 {
		var arr domain.UUIDArray
		for _, s := range payload.AssignedTo {
			if id, err := uuid.Parse(s); err == nil {
				arr = append(arr, id)
			}
		}
		assignedTo = &arr
	}

	var priority *domain.MitigationPriority
	if payload.Priority != nil {
		p := domain.MitigationPriority(*payload.Priority)
		priority = &p
	}

	var dueDate *time.Time
	if payload.DueDate != nil {
		if t, err := time.Parse(time.RFC3339, *payload.DueDate); err == nil {
			dueDate = &t
		}
	}

	repo := repository.NewGormMitigationRepository(database.DB)
	useCase := mitigation.NewUpdateMitigationPlanUseCase(repo).
		WithOwnership(OwnershipServiceInstance())

	input := mitigation.UpdateMitigationPlanInput{
		TenantID:    ctx.OrganizationID,
		PlanID:      uuid.MustParse(planID),
		Title:       payload.Title,
		Description: payload.Description,
		Status:      status,
		Priority:    priority,
		AssignedTo:  assignedTo,
		DueDate:     dueDate,
		Ownership:   payload.OwnershipPatch,
		Actor:       ctx.UserID,
		Locale:      c.Query("locale", "fr"),
	}

	if err := useCase.Execute(input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	updated, _ := repo.GetByIDWithSubActions(ctx.OrganizationID.String(), uuid.MustParse(planID))
	return c.JSON(updated)
}

// DeleteMitigation deletes (soft-deletes) a mitigation plan
func DeleteMitigation(c *fiber.Ctx) error {
	ctx := middleware.GetContext(c)
	if ctx == nil || ctx.OrganizationID == uuid.Nil {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}

	planID := c.Params("id")
	if _, err := uuid.Parse(planID); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid plan ID"})
	}

	repo := repository.NewGormMitigationRepository(database.DB)
	if err := repo.Delete(ctx.OrganizationID.String(), uuid.MustParse(planID)); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return c.SendStatus(204)
}

// ValidateMitigation transitions a plan from REVIEW to DONE (reviewer approval)
func ValidateMitigation(c *fiber.Ctx) error {
	ctx := middleware.GetContext(c)
	if ctx == nil || ctx.UserID == uuid.Nil {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}

	planID := c.Params("id")
	if _, err := uuid.Parse(planID); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid plan ID"})
	}

	repo := repository.NewGormMitigationRepository(database.DB)
	notifier := &NoOpNotifier{} // Placeholder
	useCase := mitigation.NewValidateMitigationPlanUseCase(repo, notifier)

	input := mitigation.ValidateMitigationPlanInput{
		TenantID:   ctx.OrganizationID,
		PlanID:     uuid.MustParse(planID),
		ReviewedBy: ctx.UserID,
	}

	if err := useCase.Execute(input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	validated, _ := repo.GetByIDWithSubActions(ctx.OrganizationID.String(), uuid.MustParse(planID))
	return c.JSON(validated)
}

// NoOpNotifier is a placeholder notify.Service (TODO: implement real notification system)
type NoOpNotifier struct{}

func (n *NoOpNotifier) SendWelcomeEmail(ctx context.Context, email, fullName string) error {
	return nil
}
