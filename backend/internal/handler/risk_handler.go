// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package handler

import (
	"context"
	"log"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/opendefender/openrisk/internal/application/risk"
	"github.com/opendefender/openrisk/internal/domain"
	"github.com/opendefender/openrisk/internal/infrastructure/audittrail"
	"github.com/opendefender/openrisk/internal/infrastructure/database"
	"github.com/opendefender/openrisk/internal/infrastructure/redis"
	"github.com/opendefender/openrisk/internal/middleware"
	"github.com/opendefender/openrisk/pkg/crq"
	"github.com/opendefender/openrisk/pkg/events"
	"github.com/opendefender/openrisk/pkg/validation"
)

// RiskHandler encapsulates the risk use cases.
type RiskHandler struct {
	createRiskUseCase *risk.CreateRiskUseCase
	getRiskUseCase    *risk.GetRiskUseCase
	listRisksUseCase  *risk.ListRisksUseCase
	updateRiskUseCase *risk.UpdateRiskUseCase
	deleteRiskUseCase *risk.DeleteRiskUseCase
	markReviewedUC    *risk.MarkRiskReviewedUseCase
	transitionStateUC *risk.TransitionRiskStateUseCase
	redisClient       *redis.Client
	crq               *crq.Quantifier                 // Cyber Risk Quantification (XAF + USD)
	presenters        *risk.FinancialPresenterFactory // optional: tenant currency + FX
	bulkActionUC      *risk.BulkActionUseCase         // #581, attached via WithBulkAction
}

// WithBulkAction attaches the bulk-action use case behind POST /risks/bulk.
//
// Optional in the WithX sense only so the nine-argument constructor and its
// existing callers stay untouched; the ROUTE is not registered unless this is
// set, so an unattached use case cannot be reached rather than half-working.
func (h *RiskHandler) WithBulkAction(uc *risk.BulkActionUseCase) *RiskHandler {
	h.bulkActionUC = uc
	return h
}

// WithFinancialPresenters attaches the tenant currency/FX presenter factory used
// by the per-risk financial + simulate endpoints. Optional (nil → XAF/static).
func (h *RiskHandler) WithFinancialPresenters(f *risk.FinancialPresenterFactory) *RiskHandler {
	h.presenters = f
	return h
}

func NewRiskHandler(
	createRisk *risk.CreateRiskUseCase,
	getRisk *risk.GetRiskUseCase,
	listRisks *risk.ListRisksUseCase,
	updateRisk *risk.UpdateRiskUseCase,
	deleteRisk *risk.DeleteRiskUseCase,
	markReviewed *risk.MarkRiskReviewedUseCase,
	transitionState *risk.TransitionRiskStateUseCase,
	redisClient *redis.Client,
	quantifier *crq.Quantifier,
) *RiskHandler {
	return &RiskHandler{
		createRiskUseCase: createRisk,
		getRiskUseCase:    getRisk,
		listRisksUseCase:  listRisks,
		updateRiskUseCase: updateRisk,
		deleteRiskUseCase: deleteRisk,
		markReviewedUC:    markReviewed,
		transitionStateUC: transitionState,
		redisClient:       redisClient,
		crq:               quantifier,
	}
}

// MarkReviewed POST /risks/:id/review — records a review now and reschedules the
// next one from the risk's cadence.
func (h *RiskHandler) MarkReviewed(c *fiber.Ctx) error {
	riskID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid UUID"})
	}
	mwCtx := middleware.GetContext(c)
	orgID := uuid.Nil
	if mwCtx != nil {
		orgID = mwCtx.OrganizationID
	}
	r, err := h.markReviewedUC.Execute(c.UserContext(), orgID, riskID)
	if err != nil {
		return writeAppError(c, err)
	}
	h.quantify(r)
	return c.JSON(r)
}

// TransitionRiskInput is the body for POST /risks/:id/transition.
//
// `to` + `comment` is the specified shape. `phase` + `note` are the previous
// one and are still accepted: the route predates this change and clients in the
// wild use it. Both map onto the same canonical state.
type TransitionRiskInput struct {
	To      string `json:"to" validate:"omitempty"`
	Comment string `json:"comment" validate:"omitempty,max=1000"`
	Phase   string `json:"phase" validate:"omitempty"`
	Note    string `json:"note" validate:"omitempty,max=1000"`
}

// target resolves the requested state from either spelling. A legacy ISO 31000
// phase name is translated to the state that phase now corresponds to.
func (in TransitionRiskInput) target() string {
	if in.To != "" {
		return in.To
	}
	switch domain.RiskPhase(in.Phase) {
	case domain.PhaseIdentified:
		return string(domain.StateIdentified)
	case domain.PhaseAnalyzed:
		return string(domain.StateAssessed)
	case domain.PhaseEvaluated:
		return string(domain.StateTreatmentPlanned)
	case domain.PhaseTreated:
		return string(domain.StateInTreatment)
	case domain.PhaseMonitored:
		return string(domain.StateMitigated)
	case domain.PhaseClosed:
		return string(domain.StateClosed)
	}
	return in.Phase
}

func (in TransitionRiskInput) comment() string {
	if in.Comment != "" {
		return in.Comment
	}
	return in.Note
}

// TransitionPhase POST /risks/:id/transition — moves a risk through the single
// canonical lifecycle. Tenant-scoped, guarded by risks:update, audited.
//
// The guards live in the use case, NOT here and not in the frontend: a blocked
// transition must be refused whichever client asks.
func (h *RiskHandler) TransitionPhase(c *fiber.Ctx) error {
	riskID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid UUID"})
	}

	input := new(TransitionRiskInput)
	if err := c.BodyParser(input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid input"})
	}
	if err := validation.GetValidator().Struct(input); err != nil {
		return badValidation(c, err)
	}
	if input.target() == "" {
		return c.Status(400).JSON(fiber.Map{"error": "validation_failed", "details": "`to` is required"})
	}

	mwCtx := middleware.GetContext(c)
	orgID := uuid.Nil
	actorID := uuid.Nil
	if mwCtx != nil {
		orgID = mwCtx.OrganizationID
		actorID = mwCtx.UserID
	}

	r, err := h.transitionStateUC.Execute(c.UserContext(), orgID, riskID, risk.TransitionRiskStateInput{
		To:      domain.RiskState(input.target()),
		Comment: input.comment(),
		Actor:   actorID,
		Locale:  c.Query("locale", "fr"),
	})
	if err != nil {
		return writeAppError(c, err)
	}

	h.quantify(r)
	return c.JSON(r)
}

// ListTransitions GET /risks/:id/transitions — the states reachable from here,
// which of them are allowed right now, and for the rest exactly what is in the
// way. Backs <LifecycleStepper>.
//
// Blocked options are RETURNED rather than hidden: a user who cannot see the
// next step has no way to learn what to do about it.
func (h *RiskHandler) ListTransitions(c *fiber.Ctx) error {
	riskID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid UUID"})
	}
	mwCtx := middleware.GetContext(c)
	orgID := uuid.Nil
	if mwCtx != nil {
		orgID = mwCtx.OrganizationID
	}
	view, err := h.transitionStateUC.AvailableTransitions(c.UserContext(), orgID, riskID, c.Query("locale", "fr"))
	if err != nil {
		return writeAppError(c, err)
	}
	return c.JSON(view)
}

// badValidation renders validator errors as human-readable, per-field messages
// (WHAT/WHY/HOW) instead of leaking the raw go-playground string
// "Key: 'CreateRiskInput.Title' Error:Field validation..." (audit-2026 #245).
// `errors` is a field→message map; `message` is a single summary line.
func badValidation(c *fiber.Ctx, err error) error {
	fields := validation.HumanizeErrors(err)
	if len(fields) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "validation_failed", "message": "Some fields are invalid."})
	}
	first := ""
	for _, m := range fields {
		first = m
		break
	}
	return c.Status(400).JSON(fiber.Map{"error": "validation_failed", "message": first, "errors": fields})
}

// deref reads an optional string field as a plain one ("" when absent).
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// quantify fills a risk's computed CRQ fields (ALE in XAF + USD, basis) from its
// SLE/ARO (or the reference model). Safe on nil.
func (h *RiskHandler) quantify(r *domain.Risk) {
	if r == nil || h.crq == nil {
		return
	}
	q := h.crq.Quantify(r.SLEXAF, r.ARO, string(r.Criticality))
	r.ALEXAF = q.ALE.XAF
	r.ALEUSD = q.ALE.USD
	r.ALEBasis = string(q.Basis)
}

// CreateRiskInput : DTO pour séparer la logique API de la logique DB
type CreateRiskInput struct {
	Title       string   `json:"title" validate:"required"`
	Description string   `json:"description"`
	Impact      float64  `json:"impact" validate:"required,min=0,max=10"`     // ERD numeric(5,1) — bounds [0,10]
	Probability float64  `json:"probability" validate:"required,min=0,max=1"` // ERD numeric(5,3) — bounds [0,1]
	Tags        []string `json:"tags"`
	AssetIDs    []string `json:"asset_ids"` // Liste des UUIDs des assets concernés
	Frameworks  []string `json:"frameworks"`
	// CategoryID is the tenant's CONTROLLED classification. Optional — a risk
	// stays creatable without one, exactly like the compliance mapping.
	CategoryID *string `json:"category_id" validate:"omitempty,uuid4"`
	// Ownership — responsable / exécutant / validateur, as picked in <UserPicker>.
	// Embedded so the three keys sit at the top level of the payload.
	domain.OwnershipPatch
	// CRQ monetary inputs (XAF). Optional.
	SLEXAF *float64 `json:"sle_xaf" validate:"omitempty,min=0"`
	ARO    *float64 `json:"aro" validate:"omitempty,min=0"`
	// Full financial-quantification drivers (spec §9). All optional XAF amounts.
	DowntimeHours           *float64 `json:"downtime_hours" validate:"omitempty,min=0"`
	HourlyDowntimeCostXAF   *float64 `json:"hourly_downtime_cost_xaf" validate:"omitempty,min=0"`
	DataLossCostXAF         *float64 `json:"data_loss_cost_xaf" validate:"omitempty,min=0"`
	FinesXAF                *float64 `json:"fines_xaf" validate:"omitempty,min=0"`
	OtherDirectCostXAF      *float64 `json:"other_direct_cost_xaf" validate:"omitempty,min=0"`
	RemediationCostXAF      *float64 `json:"remediation_cost_xaf" validate:"omitempty,min=0"`
	MitigationEffectiveness *float64 `json:"mitigation_effectiveness" validate:"omitempty,min=0,max=1"`
}

// UpdateRiskInput : DTO pour la mise à jour partielle
type UpdateRiskInput struct {
	Title       string   `json:"title" validate:"omitempty"`
	Description string   `json:"description" validate:"omitempty"`
	Impact      float64  `json:"impact" validate:"omitempty,min=0,max=10"`
	Probability float64  `json:"probability" validate:"omitempty,min=0,max=1"`
	Status      string   `json:"status" validate:"omitempty"`
	Tags        []string `json:"tags" validate:"omitempty,dive,required"`
	AssetIDs    []string `json:"asset_ids" validate:"omitempty,dive,uuid4"`
	Frameworks  []string `json:"frameworks" validate:"omitempty,dive,required"`
	// Category is tri-state like ownership: absent leaves it, null clears it.
	Category domain.NullableUUID `json:"category_id"`
	// Ownership — tri-state: a key absent from the body leaves the slot alone,
	// an explicit null unassigns it. Embedded so the three keys sit at the top
	// level of the payload.
	domain.OwnershipPatch
	// CRQ monetary inputs (XAF). Pointers → nil means "leave unchanged".
	SLEXAF *float64 `json:"sle_xaf" validate:"omitempty,min=0"`
	ARO    *float64 `json:"aro" validate:"omitempty,min=0"`
	// Full financial-quantification drivers (spec §9). nil → leave unchanged.
	DowntimeHours           *float64 `json:"downtime_hours" validate:"omitempty,min=0"`
	HourlyDowntimeCostXAF   *float64 `json:"hourly_downtime_cost_xaf" validate:"omitempty,min=0"`
	DataLossCostXAF         *float64 `json:"data_loss_cost_xaf" validate:"omitempty,min=0"`
	FinesXAF                *float64 `json:"fines_xaf" validate:"omitempty,min=0"`
	OtherDirectCostXAF      *float64 `json:"other_direct_cost_xaf" validate:"omitempty,min=0"`
	RemediationCostXAF      *float64 `json:"remediation_cost_xaf" validate:"omitempty,min=0"`
	MitigationEffectiveness *float64 `json:"mitigation_effectiveness" validate:"omitempty,min=0,max=1"`
	// Review cadence in days (0 disables).
	ReviewIntervalDays *int `json:"review_interval_days" validate:"omitempty,min=0"`
}

// CreateRisk godoc
func (h *RiskHandler) CreateRisk(c *fiber.Ctx) error {
	input := new(CreateRiskInput)

	if err := c.BodyParser(input); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error":   "Invalid input format",
			"details": err.Error(),
		})
	}

	if err := validation.GetValidator().Struct(input); err != nil {
		return badValidation(c, err)
	}

	stdCtx := c.UserContext()

	mwCtx := middleware.GetContext(c)
	orgID := uuid.Nil
	createdBy := uuid.Nil
	if mwCtx != nil {
		orgID = mwCtx.OrganizationID
		createdBy = mwCtx.UserID
	}

	categoryID, err := parseOptionalUUID(deref(input.CategoryID))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "validation_failed", "details": err.Error()})
	}

	ucInput := risk.CreateRiskInput{
		Title:       input.Title,
		Description: input.Description,
		Impact:      input.Impact,
		Probability: input.Probability,
		Tags:        input.Tags,
		Frameworks:  input.Frameworks,
		Ownership:   input.OwnershipPatch,
		CategoryID:  categoryID,
		CreatedBy:   createdBy,
		SLEXAF:      input.SLEXAF,
		ARO:         input.ARO,

		DowntimeHours:           input.DowntimeHours,
		HourlyDowntimeCostXAF:   input.HourlyDowntimeCostXAF,
		DataLossCostXAF:         input.DataLossCostXAF,
		FinesXAF:                input.FinesXAF,
		OtherDirectCostXAF:      input.OtherDirectCostXAF,
		RemediationCostXAF:      input.RemediationCostXAF,
		MitigationEffectiveness: input.MitigationEffectiveness,
	}

	domainRisk, err := h.createRiskUseCase.Execute(stdCtx, orgID, ucInput)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	// Link Assets (fallback until AssetRepo introduced)
	var linkedAssets []*domain.Asset
	if len(input.AssetIDs) > 0 {
		query := database.DB
		if mwCtx != nil {
			query = query.Where("organization_id = ?", mwCtx.OrganizationID)
		}
		if err := query.Where("id IN ?", input.AssetIDs).Find(&linkedAssets).Error; err == nil {
			domainRisk.Assets = linkedAssets
			// Save relationships (no direct score compute — publish Redis event instead)
			if err := database.DB.Model(&domainRisk).Association("Assets").Replace(linkedAssets); err != nil {
				log.Printf("Warning: failed to update asset associations for risk %s: %v", domainRisk.ID, err)
			}
		}
	}

	// RULE #12: Score Engine is NEVER called directly from handler.
	// Always publish Redis event → ScoreWorker listens and recalculates async,
	// using the real criticality of whichever assets were just linked instead
	// of a hardcoded placeholder.
	if h.redisClient != nil {
		event := events.RiskUpdatedEvent{
			RiskID:           domainRisk.ID.String(),
			TenantID:         orgID.String(),
			Probability:      float64(domainRisk.Probability),
			Impact:           float64(domainRisk.Impact),
			AssetCriticality: averageAssetCriticalityFactor(linkedAssets),
			TriggeredBy:      createdBy.String(),
		}
		_ = h.redisClient.Publish(c.Context(), events.RiskUpdated, event)
	}

	var out domain.Risk
	if err := database.DB.Preload("Mitigations").Preload("Mitigations.SubActions").Preload("Assets").First(&out, "id = ?", domainRisk.ID).Error; err != nil {
		h.quantify(domainRisk)
		return c.Status(201).JSON(domainRisk)
	}

	h.quantify(&out)
	return c.Status(201).JSON(out)
}

// GetRisks godoc
func (h *RiskHandler) GetRisks(c *fiber.Ctx) error {
	mwCtx := middleware.GetContext(c)
	orgID := uuid.Nil
	if mwCtx != nil {
		orgID = mwCtx.OrganizationID
	}

	query := domain.NewRiskQuery()

	if q := c.Query("q"); q != "" {
		query.Search = q
	}
	// Faceted filters. Each accepts a comma-separated list so the register's
	// filter panel can combine several values of the same facet in one query
	// (?status=open,in_progress&criticality=critical,high). All of them are
	// applied ON TOP of the tenant predicate in the repository (RULE #2).
	if status := c.Query("status"); status != "" {
		query.Status = csvValues(status)
	}
	if crit := c.Query("criticality"); crit != "" {
		query.Criticality = csvValues(crit)
	}
	if phase := c.Query("phase"); phase != "" {
		query.LifecyclePhase = csvValues(phase)
	}
	if source := c.Query("source"); source != "" {
		query.Source = csvValues(source)
	}
	if framework := c.Query("framework"); framework != "" {
		query.Framework = framework
	}
	if assignee := c.Query("assigned_to"); assignee != "" {
		if id, err := uuid.Parse(assignee); err == nil {
			query.AssignedTo = &id
		}
	}
	if owner := c.Query("owner_id"); owner != "" {
		if id, err := uuid.Parse(owner); err == nil {
			query.OwnedBy = &id
		}
	}
	if reviewer := c.Query("reviewer_id"); reviewer != "" {
		if id, err := uuid.Parse(reviewer); err == nil {
			query.ReviewedBy = &id
		}
	}
	// "Mes risques" — anything the caller owns, works on, or must validate.
	// Deliberately derived from the AUTHENTICATED user, never from a query
	// parameter: "mine=<someone else's id>" must not be expressible.
	if c.Query("mine") == "true" {
		if mwCtx := middleware.GetContext(c); mwCtx != nil && mwCtx.UserID != uuid.Nil {
			me := mwCtx.UserID
			query.InvolvedUser = &me
		}
	}
	if minScoreStr := c.Query("min_score"); minScoreStr != "" {
		if v, err := strconv.ParseFloat(minScoreStr, 64); err == nil {
			query.MinScore = &v
		}
	}
	if maxScoreStr := c.Query("max_score"); maxScoreStr != "" {
		if v, err := strconv.ParseFloat(maxScoreStr, 64); err == nil {
			query.MaxScore = &v
		}
	}
	if tag := c.Query("tag"); tag != "" {
		query.Tags = []string{tag}
	}
	// Taxonomy facets: the CONTROLLED category, and the real compliance mapping.
	// `framework` above still filters the frozen free-text column for old
	// clients; `framework_id` is the one that means anything.
	if cats := c.Query("category_id"); cats != "" {
		for _, raw := range csvValues(cats) {
			if id, err := uuid.Parse(raw); err == nil {
				query.CategoryIDs = append(query.CategoryIDs, id)
			}
		}
	}
	if fw := c.Query("framework_id"); fw != "" {
		if id, err := uuid.Parse(fw); err == nil {
			query.FrameworkID = &id
		}
	}
	if c.Query("unmapped") == "true" {
		query.Unmapped = true
	}

	// Page & Limit
	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			query.Page = p
		}
	}
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 200 {
			query.Limit = l
		}
	}

	// Sorting
	sortBy := c.Query("sort_by")
	sortDir := strings.ToLower(c.Query("sort_dir"))

	if sortBy != "" {
		switch strings.ToLower(sortBy) {
		case "score", "name", "title", "created_at", "updated_at", "impact", "probability", "status", "criticality", "source":
			query.SortBy = sortBy
		case "newest":
			query.SortBy = "created_at"
			sortDir = "desc"
		case "oldest":
			query.SortBy = "created_at"
			sortDir = "asc"
		case "updated":
			query.SortBy = "updated_at"
		}
	}
	if sortDir == "asc" || sortDir == "desc" {
		query.SortOrder = sortDir
	}

	result, err := h.listRisksUseCase.Execute(c.UserContext(), orgID, query)
	if err != nil {
		return serverError(c, "Could not fetch risks", err)
	}

	for i := range result.Data {
		h.quantify(&result.Data[i])
	}
	return c.JSON(fiber.Map{"items": result.Data, "total": result.Total})
}

// GetRisk godoc
func (h *RiskHandler) GetRisk(c *fiber.Ctx) error {
	id := c.Params("id")
	riskID, err := uuid.Parse(id)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid UUID"})
	}

	mwCtx := middleware.GetContext(c)
	orgID := uuid.Nil
	if mwCtx != nil {
		orgID = mwCtx.OrganizationID
	}

	domainRisk, err := h.getRiskUseCase.Execute(c.UserContext(), orgID, riskID)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Risk not found", "details": err.Error()})
	}

	h.quantify(domainRisk)
	return c.JSON(domainRisk)
}

// UpdateRisk godoc
func (h *RiskHandler) UpdateRisk(c *fiber.Ctx) error {
	id := c.Params("id")
	riskID, err := uuid.Parse(id)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid UUID"})
	}

	input := new(UpdateRiskInput)
	if err := c.BodyParser(input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid input"})
	}
	if err := validation.GetValidator().Struct(input); err != nil {
		return badValidation(c, err)
	}

	mwCtx := middleware.GetContext(c)
	orgID := uuid.Nil
	actorID := uuid.Nil
	if mwCtx != nil {
		orgID = mwCtx.OrganizationID
		actorID = mwCtx.UserID
	}

	ucInput := risk.UpdateRiskInput{
		Title:              &input.Title,
		Description:        &input.Description,
		Impact:             &input.Impact,
		Probability:        &input.Probability,
		Tags:               input.Tags,
		Frameworks:         input.Frameworks,
		Ownership:          input.OwnershipPatch,
		Category:           input.Category,
		Actor:              actorID,
		Locale:             c.Query("locale", "fr"),
		SLEXAF:             input.SLEXAF,
		ARO:                input.ARO,
		ReviewIntervalDays: input.ReviewIntervalDays,

		DowntimeHours:           input.DowntimeHours,
		HourlyDowntimeCostXAF:   input.HourlyDowntimeCostXAF,
		DataLossCostXAF:         input.DataLossCostXAF,
		FinesXAF:                input.FinesXAF,
		OtherDirectCostXAF:      input.OtherDirectCostXAF,
		RemediationCostXAF:      input.RemediationCostXAF,
		MitigationEffectiveness: input.MitigationEffectiveness,
	}

	if input.Title == "" {
		ucInput.Title = nil
	}
	if input.Description == "" {
		ucInput.Description = nil
	}
	if input.Impact == 0 {
		ucInput.Impact = nil
	}
	if input.Probability == 0 {
		ucInput.Probability = nil
	}
	if input.Status != "" {
		s := domain.RiskStatus(input.Status)
		ucInput.Status = &s
	}

	domainRisk, err := h.updateRiskUseCase.Execute(c.UserContext(), orgID, riskID, ucInput)
	if err != nil {
		// Typed mapping: updating another tenant's risk is a not-found, not a
		// bad request, and the raw error must not reach the client.
		return writeAppError(c, err)
	}

	if len(input.AssetIDs) > 0 {
		var linkedAssets []*domain.Asset
		query := database.DB
		if mwCtx != nil {
			query = query.Where("organization_id = ?", mwCtx.OrganizationID)
		}
		if err := query.Where("id IN ?", input.AssetIDs).Find(&linkedAssets).Error; err == nil {
			domainRisk.Assets = linkedAssets
			// No direct score compute here (RULE #12) — save the association,
			// then publish a Redis event below so the ScoreWorker recalculates
			// via the real Score Engine, same as CreateRisk.
			if err := database.DB.Model(&domainRisk).Association("Assets").Replace(linkedAssets); err != nil {
				log.Printf("Warning: failed to update asset associations for risk %s: %v", domainRisk.ID, err)
			}
		}
	}

	var out domain.Risk
	hasOut := database.DB.Preload("Mitigations").Preload("Mitigations.SubActions").Preload("Assets").First(&out, "id = ?", riskID).Error == nil

	// RULE #12: Score Engine is NEVER called directly from handler.
	// Always publish Redis event → ScoreWorker listens and recalculates async.
	// Uses the risk's currently linked assets — freshly replaced above if this
	// update touched asset_ids, or its pre-existing ones otherwise — so an
	// Impact/Probability-only edit still gets a criticality-adjusted score.
	assetsForScoring := domainRisk.Assets
	if hasOut {
		assetsForScoring = out.Assets
	}
	if h.redisClient != nil {
		userID := uuid.Nil
		if mwCtx != nil {
			userID = mwCtx.UserID
		}
		event := events.RiskUpdatedEvent{
			RiskID:           domainRisk.ID.String(),
			TenantID:         orgID.String(),
			Probability:      float64(domainRisk.Probability),
			Impact:           float64(domainRisk.Impact),
			AssetCriticality: averageAssetCriticalityFactor(assetsForScoring),
			TriggeredBy:      userID.String(),
		}
		_ = h.redisClient.Publish(c.Context(), events.RiskUpdated, event)
	}

	if !hasOut {
		h.quantify(domainRisk)
		return c.JSON(domainRisk)
	}
	h.quantify(&out)
	return c.JSON(out)
}

// averageAssetCriticalityFactor averages domain.AssetCriticality.ScoreFactor()
// across a risk's linked assets, for the Redis event consumed by ScoreWorker.
// Defaults to 1.0 (neutral) when a risk has no linked assets yet.
func averageAssetCriticalityFactor(assets []*domain.Asset) float64 {
	if len(assets) == 0 {
		return 1.0
	}
	var sum float64
	for _, a := range assets {
		sum += a.Criticality.ScoreFactor()
	}
	return sum / float64(len(assets))
}

// DeleteRisk godoc
func (h *RiskHandler) DeleteRisk(c *fiber.Ctx) error {
	id := c.Params("id")
	riskID, err := uuid.Parse(id)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid UUID"})
	}

	mwCtx := middleware.GetContext(c)
	orgID := uuid.Nil
	if mwCtx != nil {
		orgID = mwCtx.OrganizationID
	}

	// Map the typed domain error instead of collapsing everything into 500.
	// Deleting a risk that belongs to another tenant used to answer 500 with the
	// raw error in `details`: wrong semantics (it is a not-found, not a server
	// fault) and an information leak that bypassed the global error handler,
	// since this response is constructed explicitly.
	if err = h.deleteRiskUseCase.Execute(c.UserContext(), orgID, riskID); err != nil {
		return writeAppError(c, err)
	}

	return c.SendStatus(204)
}


// ---------------------------------------------------------------------------
// Bulk actions — POST /api/v1/risks/bulk (#581)
// ---------------------------------------------------------------------------

// BulkAction applies one mutation to a set of risks, all or none (D-036).
//
// The route is guarded by RequirePermission("risks:update") at registration, so
// a caller without it never reaches this function — that is criterion 8, and it
// is enforced by middleware rather than re-implemented here.
//
// Errors are mapped through writeAppError, not flattened to 500. That matters:
// a stale or foreign id must come back 404, and the previous handler turned
// every use-case error into "failed to perform bulk action" with a 500, which
// made a perfectly ordinary "one of your ids no longer exists" indistinguishable
// from a server fault.
func (h *RiskHandler) BulkAction(c *fiber.Ctx) error {
	if h.bulkActionUC == nil {
		return c.Status(fiber.StatusNotImplemented).
			JSON(fiber.Map{"error": "bulk actions are not enabled"})
	}

	var req risk.BulkActionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	// Identity comes from the signed session only. The tenant is what every read
	// and write in the batch is scoped by, so a body-supplied one would be a
	// cross-tenant mutation primitive.
	result, err := h.bulkActionUC.Execute(
		auditCtx(c), tenantID(c), req, userID(c),
	)
	if err != nil {
		return writeAppError(c, err)
	}
	return c.JSON(result)
}

// auditCtx carries the acting identity and request metadata into the use case,
// so the audit entries it writes name a person and a request rather than a
// system actor. Same shape as govCtx in governance_handler.go.
func auditCtx(c *fiber.Ctx) context.Context {
	uid := userID(c)
	var actorID *uuid.UUID
	if uid != uuid.Nil {
		actorID = &uid
	}
	return audittrail.WithActor(c.UserContext(), audittrail.Actor{
		ID:        actorID,
		TenantID:  tenantID(c),
		IPAddress: c.IP(),
		UserAgent: c.Get("User-Agent"),
		RequestID: c.Get("X-Request-ID"),
	})
}
