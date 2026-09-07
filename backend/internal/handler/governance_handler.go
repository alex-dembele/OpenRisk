// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package handler

import (
	"bytes"
	"context"
	"encoding/csv"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/opendefender/openrisk/internal/application/governance"
	"github.com/opendefender/openrisk/internal/domain"
	"github.com/opendefender/openrisk/internal/infrastructure/audittrail"
	"github.com/opendefender/openrisk/internal/middleware"
)

// GovernanceHandler exposes the Governance module (spec §15): the immutable audit
// trail, time-boxed delegations, and the Maker-Checker approval engine. Tenant/
// user come from middleware.GetContext via the shared tenantID()/userID() helpers.
type GovernanceHandler struct {
	listAudit *governance.ListAuditEventsUseCase
	recorder  *governance.AuditRecorder

	// Tamper evidence + lifecycle of the trail itself. Optional: a deployment
	// without them still lists and files the trail, and the endpoints say so
	// (503) rather than pretending to verify.
	verifyChain  *governance.VerifyAuditChainUseCase
	exportAudit  *governance.ExportAuditTrailUseCase
	getRetention *governance.GetRetentionPolicyUseCase
	setRetention *governance.SetRetentionPolicyUseCase
	pruneAudit   *governance.PruneAuditTrailUseCase

	createDelegation *governance.CreateDelegationUseCase
	listDelegations  *governance.ListDelegationsUseCase
	revokeDelegation *governance.RevokeDelegationUseCase
	effectivePerms   *governance.ResolveEffectivePermissionsUseCase

	createWorkflow *governance.CreateWorkflowUseCase
	listWorkflows  *governance.ListWorkflowsUseCase
	updateWorkflow *governance.UpdateWorkflowUseCase
	deleteWorkflow *governance.DeleteWorkflowUseCase

	submitApproval *governance.SubmitApprovalRequestUseCase
	decideApproval *governance.DecideApprovalUseCase
	approvalDetail *governance.GetApprovalDetailUseCase
	cancelApproval *governance.CancelApprovalRequestUseCase
	listApprovals  *governance.ListApprovalRequestsUseCase
	getRequest     domain.ApprovalRequestRepository
}

// GovernanceDeps bundles the wired use cases (keeps the constructor readable).
type GovernanceDeps struct {
	ListAudit *governance.ListAuditEventsUseCase
	Recorder  *governance.AuditRecorder

	VerifyChain  *governance.VerifyAuditChainUseCase
	ExportAudit  *governance.ExportAuditTrailUseCase
	GetRetention *governance.GetRetentionPolicyUseCase
	SetRetention *governance.SetRetentionPolicyUseCase
	PruneAudit   *governance.PruneAuditTrailUseCase

	CreateDelegation *governance.CreateDelegationUseCase
	ListDelegations  *governance.ListDelegationsUseCase
	RevokeDelegation *governance.RevokeDelegationUseCase
	EffectivePerms   *governance.ResolveEffectivePermissionsUseCase

	CreateWorkflow *governance.CreateWorkflowUseCase
	ListWorkflows  *governance.ListWorkflowsUseCase
	UpdateWorkflow *governance.UpdateWorkflowUseCase
	DeleteWorkflow *governance.DeleteWorkflowUseCase

	SubmitApproval *governance.SubmitApprovalRequestUseCase
	DecideApproval *governance.DecideApprovalUseCase
	ApprovalDetail *governance.GetApprovalDetailUseCase
	CancelApproval *governance.CancelApprovalRequestUseCase
	ListApprovals  *governance.ListApprovalRequestsUseCase
	GetRequest     domain.ApprovalRequestRepository
}

func NewGovernanceHandler(d GovernanceDeps) *GovernanceHandler {
	return &GovernanceHandler{
		listAudit:        d.ListAudit,
		recorder:         d.Recorder,
		verifyChain:      d.VerifyChain,
		exportAudit:      d.ExportAudit,
		getRetention:     d.GetRetention,
		setRetention:     d.SetRetention,
		pruneAudit:       d.PruneAudit,
		createDelegation: d.CreateDelegation,
		listDelegations:  d.ListDelegations,
		revokeDelegation: d.RevokeDelegation,
		effectivePerms:   d.EffectivePerms,
		createWorkflow:   d.CreateWorkflow,
		listWorkflows:    d.ListWorkflows,
		updateWorkflow:   d.UpdateWorkflow,
		deleteWorkflow:   d.DeleteWorkflow,
		submitApproval:   d.SubmitApproval,
		decideApproval:   d.DecideApproval,
		approvalDetail:   d.ApprovalDetail,
		cancelApproval:   d.CancelApproval,
		listApprovals:    d.ListApprovals,
		getRequest:       d.GetRequest,
	}
}

// govCtx wraps the request context with the acting identity + request metadata so
// the audit Recorder and the audittrail GORM plugin can attribute mutations.
func govCtx(c *fiber.Ctx) context.Context {
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

// approverFromCtx derives who is deciding, from the JWT. Delegated rights are
// resolved server-side by the use case — a client must never be able to claim
// them.
func approverFromCtx(c *fiber.Ctx) governance.ApproverIdentity {
	claims := middleware.GetUserClaims(c)
	who := governance.ApproverIdentity{UserID: userID(c)}
	if claims == nil {
		return who
	}
	if claims.HasPermission("*") {
		who.IsAdmin = true
	}
	roles := map[string]struct{}{}
	for _, r := range claims.OrgRoles {
		roles[r] = struct{}{}
		if r == "admin" || r == "root" {
			who.IsAdmin = true
		}
	}
	for r := range roles {
		who.Roles = append(who.Roles, r)
	}
	return who
}

// =============================================================================
// Audit trail
// =============================================================================

func (h *GovernanceHandler) buildAuditFilter(c *fiber.Ctx) (domain.AuditEventFilter, error) {
	f := domain.AuditEventFilter{
		EntityType: c.Query("entity_type"),
		EntityID:   c.Query("entity_id"),
		Action:     c.Query("action"),
		RequestID:  c.Query("request_id"),
		Source:     c.Query("source"),
		Search:     c.Query("search"),
	}
	if v := c.Query("actor_id"); v != "" {
		id, err := parseOptionalUUID(v)
		if err != nil {
			return f, err
		}
		f.ActorID = id
	}
	if v := c.Query("from"); v != "" {
		t, err := parseOptionalDate(v)
		if err != nil {
			return f, err
		}
		f.From = t
	}
	if v := c.Query("to"); v != "" {
		t, err := parseOptionalDate(v)
		if err != nil {
			return f, err
		}
		f.To = t
	}
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			f.Limit = n
		}
	}
	if v := c.Query("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			f.Offset = n
		}
	}
	return f, nil
}

// ListAuditEvents GET /governance/audit-events
func (h *GovernanceHandler) ListAuditEvents(c *fiber.Ctx) error {
	f, err := h.buildAuditFilter(c)
	if err != nil {
		return writeAppError(c, err)
	}
	res, err := h.listAudit.Execute(c.UserContext(), tenantID(c), f)
	if err != nil {
		return writeAppError(c, err)
	}
	return c.JSON(res)
}

// ExportAuditEvents GET /governance/audit-events/export?format=csv|json
//
// Both formats carry a signature and the chain verdict at export time: a CSV
// gets them as header comment lines and response headers, a JSON export as an
// envelope. Exporting the audit log is itself an audited action.
func (h *GovernanceHandler) ExportAuditEvents(c *fiber.Ctx) error {
	if h.exportAudit == nil {
		return c.Status(503).JSON(fiber.Map{"error": "audit export is not available on this deployment"})
	}
	f, err := h.buildAuditFilter(c)
	if err != nil {
		return writeAppError(c, err)
	}
	f.Limit = 0 // export is not paginated — the whole filtered trail
	f.Offset = 0

	exp, err := h.exportAudit.Execute(c.UserContext(), tenantID(c), userID(c).String(), f)
	if err != nil {
		return writeAppError(c, err)
	}

	if h.recorder != nil {
		uid := userID(c)
		h.recorder.Record(govCtx(c), domain.AuditEvent{
			TenantID:   tenantID(c),
			ActorID:    &uid,
			Action:     domain.AuditActionExport,
			EntityType: "audit_events",
			EntityID:   "-",
			Summary:    "exported the audit trail (" + strconv.Itoa(exp.Count) + " entries, chain " + validLabel(exp.Verification.Valid) + ")",
		})
	}

	if sig := exp.Signature; sig != nil {
		c.Set("X-OpenRisk-Signature-Alg", sig.Algorithm)
		if sig.Value != "" {
			c.Set("X-OpenRisk-Signature", sig.Value)
			c.Set("X-OpenRisk-Signature-KeyId", sig.KeyID)
		}
	}
	c.Set("X-OpenRisk-Chain-Valid", validLabel(exp.Verification.Valid))

	if strings.EqualFold(c.Query("format"), governance.ExportFormatJSON) {
		c.Set("Content-Type", "application/json")
		c.Set("Content-Disposition", "attachment; filename=audit-trail.json")
		return c.JSON(exp)
	}

	var buf bytes.Buffer
	// Signed CSV: the provenance block is part of the file, so a spreadsheet
	// saved to disk still says where it came from and whether the chain held.
	buf.WriteString("# openrisk.audit-trail v1\n")
	buf.WriteString("# exported_at=" + exp.ExportedAt.Format(time.RFC3339) + "\n")
	buf.WriteString("# tenant=" + exp.TenantID.String() + "\n")
	buf.WriteString("# entries=" + strconv.Itoa(exp.Count) + "\n")
	buf.WriteString("# chain_valid=" + validLabel(exp.Verification.Valid) +
		" head=" + exp.Verification.HeadHash + " breaks=" + strconv.Itoa(len(exp.Verification.Breaks)) + "\n")
	if exp.Signature != nil {
		if exp.Signature.Value != "" {
			buf.WriteString("# signature=" + exp.Signature.Algorithm + ":" + exp.Signature.Value + " key_id=" + exp.Signature.KeyID + "\n")
		} else {
			buf.WriteString("# signature=none reason=" + exp.Signature.Reason + "\n")
		}
	}

	w := csv.NewWriter(&buf)
	_ = w.Write([]string{
		"sequence", "timestamp", "actor", "action", "entity_type", "entity_id",
		"summary", "changed_fields", "ip_address", "user_agent", "request_id",
		"method", "path", "status_code", "source", "prev_hash", "hash",
	})
	for _, e := range exp.Events {
		actor := e.ActorEmail
		if actor == "" && e.ActorID != nil {
			actor = e.ActorID.String()
		}
		if actor == "" {
			actor = "system"
		}
		_ = w.Write([]string{
			strconv.FormatInt(e.Sequence, 10),
			e.CreatedAt.Format(time.RFC3339Nano),
			actor,
			string(e.Action),
			e.EntityType,
			e.EntityID,
			e.Summary,
			strings.Join([]string(e.ChangedFields), " "),
			e.IPAddress,
			e.UserAgent,
			e.RequestID,
			e.Method,
			e.Path,
			strconv.Itoa(e.StatusCode),
			e.Source,
			e.PrevHash,
			e.Hash,
		})
	}
	w.Flush()

	c.Set("Content-Type", "text/csv")
	c.Set("Content-Disposition", "attachment; filename=audit-trail.csv")
	return c.Send(buf.Bytes())
}

// VerifyAuditChain GET /governance/audit-events/verify — recomputes every hash
// and every link, and reports exactly where the trail was altered, if anywhere.
func (h *GovernanceHandler) VerifyAuditChain(c *fiber.Ctx) error {
	if h.verifyChain == nil {
		return c.Status(503).JSON(fiber.Map{"error": "chain verification is not available on this deployment"})
	}
	rep, err := h.verifyChain.Execute(c.UserContext(), tenantID(c))
	if err != nil {
		return writeAppError(c, err)
	}
	return c.JSON(rep)
}

// GetAuditRetention GET /governance/audit-retention
func (h *GovernanceHandler) GetAuditRetention(c *fiber.Ctx) error {
	if h.getRetention == nil {
		return c.Status(503).JSON(fiber.Map{"error": "retention configuration is not available on this deployment"})
	}
	p, err := h.getRetention.Execute(c.UserContext(), tenantID(c))
	if err != nil {
		return writeAppError(c, err)
	}
	return c.JSON(p)
}

type retentionBody struct {
	RetentionDays int `json:"retention_days"`
}

// SetAuditRetention PUT /governance/audit-retention — 0 keeps entries forever.
func (h *GovernanceHandler) SetAuditRetention(c *fiber.Ctx) error {
	if h.setRetention == nil {
		return c.Status(503).JSON(fiber.Map{"error": "retention configuration is not available on this deployment"})
	}
	var body retentionBody
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid input", "details": err.Error()})
	}
	p, err := h.setRetention.Execute(govCtx(c), tenantID(c), userID(c), body.RetentionDays)
	if err != nil {
		return writeAppError(c, err)
	}
	return c.JSON(p)
}

// RunAuditRetention POST /governance/audit-retention/apply — applies the window
// now instead of waiting for the nightly sweep.
func (h *GovernanceHandler) RunAuditRetention(c *fiber.Ctx) error {
	if h.pruneAudit == nil {
		return c.Status(503).JSON(fiber.Map{"error": "retention pruning is not available on this deployment"})
	}
	res, err := h.pruneAudit.ExecuteForTenant(c.UserContext(), tenantID(c), time.Now().UTC())
	if err != nil {
		return writeAppError(c, err)
	}
	return c.JSON(res)
}

func validLabel(ok bool) string {
	if ok {
		return "true"
	}
	return "false"
}

// =============================================================================
// Delegations
// =============================================================================

type createDelegationBody struct {
	DelegatorID string   `json:"delegator_id"` // optional; defaults to the caller
	DelegateID  string   `json:"delegate_id"`
	Reason      string   `json:"reason"`
	Permissions []string `json:"permissions"`
	StartsAt    string   `json:"starts_at"`
	EndsAt      string   `json:"ends_at"`
}

func (h *GovernanceHandler) ListDelegations(c *fiber.Ctx) error {
	f := domain.DelegationFilter{ActiveOnly: c.Query("active") == "true"}
	if v := c.Query("delegator_id"); v != "" {
		id, err := parseOptionalUUID(v)
		if err != nil {
			return writeAppError(c, err)
		}
		f.DelegatorID = id
	}
	if v := c.Query("delegate_id"); v != "" {
		id, err := parseOptionalUUID(v)
		if err != nil {
			return writeAppError(c, err)
		}
		f.DelegateID = id
	}
	list, err := h.listDelegations.Execute(c.UserContext(), tenantID(c), f)
	if err != nil {
		return writeAppError(c, err)
	}
	return c.JSON(list)
}

func (h *GovernanceHandler) CreateDelegation(c *fiber.Ctx) error {
	var body createDelegationBody
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid input format"})
	}
	delegateID, err := uuid.Parse(strings.TrimSpace(body.DelegateID))
	if err != nil {
		return writeAppError(c, domain.NewValidationError("delegate_id must be a valid uuid"))
	}
	in := governance.CreateDelegationInput{
		DelegateID:  delegateID,
		Reason:      body.Reason,
		Permissions: body.Permissions,
		// #529 — the route carries no permission guard by design (any member
		// lends their own rights). Naming somebody ELSE as the delegator is the
		// privileged case, and the use case refuses it unless this is true.
		ActorIsAdmin: approverFromCtx(c).IsAdmin,
	}
	if body.DelegatorID != "" {
		did, err := uuid.Parse(body.DelegatorID)
		if err != nil {
			return writeAppError(c, domain.NewValidationError("delegator_id must be a valid uuid"))
		}
		in.DelegatorID = did
	}
	if body.StartsAt != "" {
		t, err := parseOptionalDate(body.StartsAt)
		if err != nil {
			return writeAppError(c, err)
		}
		in.StartsAt = t
	}
	if body.EndsAt != "" {
		t, err := parseOptionalDate(body.EndsAt)
		if err != nil {
			return writeAppError(c, err)
		}
		in.EndsAt = t
	}
	d, err := h.createDelegation.Execute(govCtx(c), tenantID(c), userID(c), in)
	if err != nil {
		return writeAppError(c, err)
	}
	return c.Status(201).JSON(d)
}

func (h *GovernanceHandler) RevokeDelegation(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid delegation id"})
	}
	d, err := h.revokeDelegation.Execute(govCtx(c), tenantID(c), userID(c), governance.RevokeDelegationInput{
		ID:           id,
		ActorIsAdmin: approverFromCtx(c).IsAdmin,
	})
	if err != nil {
		return writeAppError(c, err)
	}
	return c.JSON(d)
}

// EffectiveDelegatedPermissions GET /governance/delegations/effective?delegate_id=
// Defaults to the calling user when delegate_id is omitted.
func (h *GovernanceHandler) EffectiveDelegatedPermissions(c *fiber.Ctx) error {
	delegateID := userID(c)
	if v := c.Query("delegate_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return writeAppError(c, domain.NewValidationError("delegate_id must be a valid uuid"))
		}
		delegateID = id
	}
	perms, err := h.effectivePerms.Execute(c.UserContext(), tenantID(c), delegateID)
	if err != nil {
		return writeAppError(c, err)
	}
	return c.JSON(fiber.Map{"delegate_id": delegateID, "permissions": perms})
}

// =============================================================================
// Approval workflows (config)
// =============================================================================

type workflowStepBody struct {
	Name            string   `json:"name"`
	ApproverRole    string   `json:"approver_role"`
	MinApprovals    int      `json:"min_approvals"`
	ApproverUserIDs []string `json:"approver_user_ids"`
	QuorumPercent   int      `json:"quorum_percent"`
}

type workflowBody struct {
	Name           string             `json:"name"`
	Description    string             `json:"description"`
	EntityType     string             `json:"entity_type"`
	Action         string             `json:"action"`
	RequestType    string             `json:"request_type"`
	Mode           string             `json:"mode"`
	ExpiresInHours int                `json:"expires_in_hours"`
	Enabled        *bool              `json:"enabled"`
	Steps          []workflowStepBody `json:"steps"`
}

func (b workflowBody) toInput() governance.WorkflowInput {
	steps := make([]domain.WorkflowStep, 0, len(b.Steps))
	for _, s := range b.Steps {
		steps = append(steps, domain.WorkflowStep{
			Name:            s.Name,
			ApproverRole:    s.ApproverRole,
			MinApprovals:    s.MinApprovals,
			ApproverUserIDs: s.ApproverUserIDs,
			QuorumPercent:   s.QuorumPercent,
		})
	}
	return governance.WorkflowInput{
		Name:           b.Name,
		Description:    b.Description,
		EntityType:     b.EntityType,
		Action:         b.Action,
		RequestType:    b.RequestType,
		Mode:           b.Mode,
		ExpiresInHours: b.ExpiresInHours,
		Enabled:        b.Enabled,
		Steps:          steps,
	}
}

// ListRequestTypes GET /governance/request-types — the shared vocabulary of
// decisions an organisation routes for sign-off.
func (h *GovernanceHandler) ListRequestTypes(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"items": domain.ApprovalRequestTypes()})
}

func (h *GovernanceHandler) ListWorkflows(c *fiber.Ctx) error {
	list, err := h.listWorkflows.Execute(c.UserContext(), tenantID(c))
	if err != nil {
		return writeAppError(c, err)
	}
	return c.JSON(list)
}

func (h *GovernanceHandler) CreateWorkflow(c *fiber.Ctx) error {
	var body workflowBody
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid input format"})
	}
	w, err := h.createWorkflow.Execute(govCtx(c), tenantID(c), userID(c), body.toInput())
	if err != nil {
		return writeAppError(c, err)
	}
	return c.Status(201).JSON(w)
}

func (h *GovernanceHandler) UpdateWorkflow(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid workflow id"})
	}
	var body workflowBody
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid input format"})
	}
	w, err := h.updateWorkflow.Execute(govCtx(c), tenantID(c), id, body.toInput())
	if err != nil {
		return writeAppError(c, err)
	}
	return c.JSON(w)
}

func (h *GovernanceHandler) DeleteWorkflow(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid workflow id"})
	}
	if err := h.deleteWorkflow.Execute(govCtx(c), tenantID(c), id); err != nil {
		return writeAppError(c, err)
	}
	return c.SendStatus(204)
}

// =============================================================================
// Approval requests (the Maker-Checker inbox)
// =============================================================================

type submitApprovalBody struct {
	EntityType  string         `json:"entity_type"`
	EntityID    string         `json:"entity_id"`
	Action      string         `json:"action"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Payload     domain.JSONMap `json:"payload"`
}

type decideApprovalBody struct {
	Decision string `json:"decision"`
	Comment  string `json:"comment"`
	// StepOrder targets one branch of a parallel chain. Omitted in sequential
	// mode, where there is only ever one open step.
	StepOrder *int `json:"step_order"`
}

func (h *GovernanceHandler) ListApprovals(c *fiber.Ctx) error {
	f := domain.ApprovalRequestFilter{
		Status:     c.Query("status"),
		EntityType: c.Query("entity_type"),
	}
	if c.Query("mine") == "true" {
		uid := userID(c)
		f.RequestedBy = &uid
	}
	list, err := h.listApprovals.Execute(c.UserContext(), tenantID(c), f)
	if err != nil {
		return writeAppError(c, err)
	}
	return c.JSON(list)
}

func (h *GovernanceHandler) GetApproval(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request id"})
	}
	// The detail view carries per-step progress and whether the CALLER may sign,
	// so the UI can explain a disabled button instead of producing a 403 on click.
	if h.approvalDetail != nil {
		detail, err := h.approvalDetail.Execute(c.UserContext(), tenantID(c), id, approverFromCtx(c))
		if err != nil {
			return writeAppError(c, err)
		}
		return c.JSON(detail)
	}
	req, err := h.getRequest.GetRequestByID(c.UserContext(), id, tenantID(c))
	if err != nil {
		return writeAppError(c, err)
	}
	if req == nil {
		return writeAppError(c, domain.NewNotFoundError("approval request", id))
	}
	return c.JSON(req)
}

func (h *GovernanceHandler) SubmitApproval(c *fiber.Ctx) error {
	var body submitApprovalBody
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid input format"})
	}
	req, err := h.submitApproval.Execute(govCtx(c), tenantID(c), userID(c), governance.SubmitApprovalInput{
		EntityType:  body.EntityType,
		EntityID:    body.EntityID,
		Action:      body.Action,
		Title:       body.Title,
		Description: body.Description,
		Payload:     body.Payload,
	})
	if err != nil {
		return writeAppError(c, err)
	}
	return c.Status(201).JSON(req)
}

func (h *GovernanceHandler) DecideApproval(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request id"})
	}
	var body decideApprovalBody
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid input format"})
	}
	req, err := h.decideApproval.Execute(govCtx(c), tenantID(c), id, approverFromCtx(c), governance.DecideInput{
		Decision:  body.Decision,
		Comment:   body.Comment,
		StepOrder: body.StepOrder,
	})
	if err != nil {
		return writeAppError(c, err)
	}
	return c.JSON(req)
}

func (h *GovernanceHandler) CancelApproval(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request id"})
	}
	req, err := h.cancelApproval.Execute(govCtx(c), tenantID(c), userID(c), id)
	if err != nil {
		return writeAppError(c, err)
	}
	return c.JSON(req)
}
