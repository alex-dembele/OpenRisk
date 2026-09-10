// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

// Activation & onboarding handler — the endpoints behind the newcomer journey.
//
// Everything here is tenant-scoped from the request context and reads/writes the
// SERVER's activation state. There is deliberately no endpoint that lets a client
// declare a step complete: steps are ticked by domain events only, which is what
// makes the checklist trustworthy.
package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	appactivation "github.com/opendefender/openrisk/internal/application/activation"
	"github.com/opendefender/openrisk/internal/domain"
	"github.com/opendefender/openrisk/internal/middleware"
)

// ActivationHandler serves the activation checklist and the onboarding wizard.
type ActivationHandler struct {
	state      *appactivation.GetStateUseCase
	celebrated *appactivation.MarkCelebratedUseCase
	onboarding *appactivation.OnboardingUseCase
	// posture and recognition are OPTIONAL (nil-safe): a deployment that has not
	// wired the posture reader still serves the checklist and the wizard. They
	// answer 503 rather than an empty posture — see GetPosture.
	posture     *appactivation.PostureUseCase
	recognition *appactivation.RecognitionUseCase
	starter     *appactivation.StarterRisksUseCase
}

// NewActivationHandler wires the handler.
func NewActivationHandler(
	state *appactivation.GetStateUseCase,
	celebrated *appactivation.MarkCelebratedUseCase,
	onboarding *appactivation.OnboardingUseCase,
) *ActivationHandler {
	return &ActivationHandler{state: state, celebrated: celebrated, onboarding: onboarding}
}

// WithPosture attaches the Posture Reveal use case (#438).
func (h *ActivationHandler) WithPosture(uc *appactivation.PostureUseCase) *ActivationHandler {
	h.posture = uc
	return h
}

// WithRecognition attaches the recognition use case (#438 criterion 9).
func (h *ActivationHandler) WithRecognition(uc *appactivation.RecognitionUseCase) *ActivationHandler {
	h.recognition = uc
	return h
}

// WithStarterRisks attaches the starter catalogue use case (#438 step 2).
func (h *ActivationHandler) WithStarterRisks(uc *appactivation.StarterRisksUseCase) *ActivationHandler {
	h.starter = uc
	return h
}

// GetStarterRisks GET /onboarding/starter-risks
//
// The eight statements step 2 renders, scoped to the sector and country the user
// gave in step 1. Never empty: asking someone to pick three of nothing is a
// broken screen, so the catalogue falls back rather than returning 404.
func (h *ActivationHandler) GetStarterRisks(c *fiber.Ctx) error {
	tenantID, userID, ok := h.identity(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}
	if h.starter == nil {
		return c.Status(fiber.StatusServiceUnavailable).
			JSON(fiber.Map{"error": "Starter risks are not available on this deployment"})
	}

	offer, err := h.starter.List(c.UserContext(), tenantID, userID)
	if err != nil {
		return writeAppError(c, err)
	}
	return c.JSON(offer)
}

type adoptStarterRisksInput struct {
	// Keys, and ONLY keys. There is deliberately no title or description field:
	// the statement is re-read from the catalogue server-side, because a client
	// that could post free text would be an unvalidated write into a customer's
	// risk register.
	Keys []string `json:"keys"`
	// Lang selects WHICH of the two server-authored strings is stored. It is a
	// preference, not content — it cannot introduce text of the client's own, and
	// anything unrecognised falls back to French. Sent by the client because it
	// is the only party that knows which language the person is reading.
	Lang string `json:"lang"`
}

// AdoptStarterRisks POST /onboarding/starter-risks
//
// Writes the three chosen statements as real risks, with `source = "starter"`
// (D-012). Idempotent per tenant: a second adoption is a 409, not a duplicate —
// the tunnel is resumable, so a user WILL come back to this step.
func (h *ActivationHandler) AdoptStarterRisks(c *fiber.Ctx) error {
	tenantID, userID, ok := h.identity(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}
	if h.starter == nil {
		return c.Status(fiber.StatusServiceUnavailable).
			JSON(fiber.Map{"error": "Starter risks are not available on this deployment"})
	}

	var in adoptStarterRisksInput
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	result, err := h.starter.Adopt(c.UserContext(), tenantID, userID, in.Keys, in.Lang)
	if err != nil {
		return writeAppError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(result)
}

// GetPosture GET /posture
//
// The Posture Reveal: the screen that DEFINES the Aha moment, computed entirely
// from this tenant's own rows. Recording `posture.revealed` and observing the v2
// time-to-Aha histogram happen inside the use case, exactly once per tenant.
//
// The 404 on this route is load-bearing and is NOT a missing-resource error in
// the usual sense: #438 criterion 8 requires that a reveal which would render
// empty produces an explicit error state and records NOTHING. Answering 200 with
// zeros would turn a rendering failure into a green launch-gate metric.
func (h *ActivationHandler) GetPosture(c *fiber.Ctx) error {
	tenantID, userID, ok := h.identity(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}
	if h.posture == nil {
		return c.Status(fiber.StatusServiceUnavailable).
			JSON(fiber.Map{"error": "Posture is not available on this deployment"})
	}

	summary, err := h.posture.Execute(c.UserContext(), tenantID, userID)
	if err != nil {
		return writeAppError(c, err)
	}
	return c.JSON(summary)
}

// GetRecognition GET /onboarding/recognition
//
// What OpenRisk already knows about a tenant that was configured before the
// tunnel existed (#438 criterion 9). The SERVER decides whether the tunnel is
// skipped: a client that could choose would be a client that can skip it.
func (h *ActivationHandler) GetRecognition(c *fiber.Ctx) error {
	tenantID, userID, ok := h.identity(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}
	if h.recognition == nil {
		return c.Status(fiber.StatusServiceUnavailable).
			JSON(fiber.Map{"error": "Recognition is not available on this deployment"})
	}

	recognition, err := h.recognition.Execute(c.UserContext(), tenantID, userID)
	if err != nil {
		return writeAppError(c, err)
	}
	return c.JSON(recognition)
}

// identity resolves (tenant, user) from the request context.
func (h *ActivationHandler) identity(c *fiber.Ctx) (uuid.UUID, uuid.UUID, bool) {
	mw := middleware.GetContext(c)
	if mw == nil || mw.OrganizationID == uuid.Nil {
		return uuid.Nil, uuid.Nil, false
	}
	return mw.OrganizationID, mw.UserID, true
}

// GetActivationState GET /activation/state
//
// The single source of truth for the get-started panel: the steps with their
// copy, their completion, their deep links, and the server's instruction to
// celebrate. The panel renders this and holds no logic of its own.
func (h *ActivationHandler) GetActivationState(c *fiber.Ctx) error {
	tenantID, userID, ok := h.identity(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}

	state, err := h.state.Execute(c.UserContext(), tenantID, userID)
	if err != nil {
		return writeAppError(c, err)
	}
	return c.JSON(state)
}

type celebratedInput struct {
	StepKey string `json:"step_key"`
}

// MarkCelebrated POST /activation/celebrated
//
// Acknowledges that the user has seen a step's celebration. Idempotent by
// construction (unique on user+step), which is what stops the burst from firing
// again on the next render, the next reload, or the next device.
func (h *ActivationHandler) MarkCelebrated(c *fiber.Ctx) error {
	tenantID, userID, ok := h.identity(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}

	var in celebratedInput
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}
	if err := h.celebrated.Execute(c.UserContext(), tenantID, userID, in.StepKey); err != nil {
		return writeAppError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// GetOnboardingState GET /onboarding/state
//
// Drives both the wizard (which step to show, with which answers) and the route
// guard (`completed`). Never 404s: a newcomer's first call returns an empty,
// valid state.
func (h *ActivationHandler) GetOnboardingState(c *fiber.Ctx) error {
	tenantID, userID, ok := h.identity(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}

	state, err := h.onboarding.GetState(c.UserContext(), tenantID, userID)
	if err != nil {
		return writeAppError(c, err)
	}
	return c.JSON(state)
}

// canEditOrganization reports whether the caller may write the tenant's
// Organization row (org role admin/root, or a wildcard permission).
func canEditOrganization(c *fiber.Ctx) bool {
	claims := middleware.GetUserClaims(c)
	if claims == nil {
		return false
	}
	if claims.HasPermission("*") {
		return true
	}
	for _, role := range claims.OrgRoles {
		if role == "admin" || role == "root" {
			return true
		}
	}
	return false
}

type saveStepInput struct {
	// Answers is the step's raw payload, stored verbatim so the form repopulates
	// exactly as the user left it when they come back.
	Answers map[string]interface{} `json:"answers"`
	// Next optionally names the step to move to; it may point backwards.
	Next string `json:"next"`
}

// SaveOnboardingStep PUT /onboarding/steps/:step
func (h *ActivationHandler) SaveOnboardingStep(c *fiber.Ctx) error {
	tenantID, userID, ok := h.identity(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}

	step, err := domain.ParseOnboardingStep(c.Params("step"))
	if err != nil {
		return writeAppError(c, err)
	}

	var in saveStepInput
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	state, err := h.onboarding.SaveStep(c.UserContext(), tenantID, userID, appactivation.SaveStepInput{
		Step:    step,
		Answers: domain.JSONMap(in.Answers),
		Next:    in.Next,
		// Only an admin/root may have their organization answers written back to
		// the Organization row. A member invited into an existing tenant still
		// walks the wizard — their answers are stored and still drive their
		// suggestions, but they cannot rename the company through it.
		CanEditOrganization: canEditOrganization(c),
	})
	if err != nil {
		return writeAppError(c, err)
	}
	return c.JSON(state)
}

// CompleteOnboarding POST /onboarding/complete — lifts the route guard.
func (h *ActivationHandler) CompleteOnboarding(c *fiber.Ctx) error {
	tenantID, userID, ok := h.identity(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}

	state, err := h.onboarding.Complete(c.UserContext(), tenantID, userID)
	if err != nil {
		return writeAppError(c, err)
	}
	return c.JSON(state)
}

// GetOnboardingSuggestions GET /onboarding/suggestions
//
// Sector/goal-driven content: the sector and goal option lists, the three
// pre-filled first-risk drafts, and the suggested frameworks. Query overrides
// (?industry=&country=&goal=) let the wizard preview a choice before saving it.
//
// Nothing here creates anything — these are drafts the user opens, edits and
// validates. We never auto-create a risk (spec §5).
func (h *ActivationHandler) GetOnboardingSuggestions(c *fiber.Ctx) error {
	tenantID, userID, ok := h.identity(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}

	out := h.onboarding.GetSuggestions(
		c.UserContext(), tenantID, userID,
		c.Query("industry"), c.Query("country"), c.Query("goal"),
	)
	return c.JSON(out)
}
