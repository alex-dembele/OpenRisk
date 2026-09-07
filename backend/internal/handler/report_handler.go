// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package handler

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	redisinfra "github.com/opendefender/openrisk/internal/infrastructure/redis"

	appreport "github.com/opendefender/openrisk/internal/application/report"
	"github.com/opendefender/openrisk/internal/domain"
)

// ReportProgressChannel is the Redis channel the worker publishes progress on.
const ReportProgressChannel = "report.progress"

// ReportHandler serves the reporting engine.
type ReportHandler struct {
	svc   *appreport.Service
	redis *redisinfra.Client
}

func NewReportHandler(svc *appreport.Service, rdb *redisinfra.Client) *ReportHandler {
	return &ReportHandler{svc: svc, redis: rdb}
}

// Catalogue GET /reports/types — what the configurator offers.
//
// Served rather than hard-coded in the frontend so a new report type or format
// appears in the UI without a release: the picker reads what the engine can
// actually produce, which is also what stops it offering a format that would be
// refused on submit.
func (h *ReportHandler) Catalogue(c *fiber.Ctx) error {
	locale, err := domain.ParseReportLocale(c.Query("locale"))
	if err != nil {
		return writeAppError(c, err)
	}

	type formatDTO struct {
		Key   string `json:"key"`
		Label string `json:"label"`
	}
	type typeDTO struct {
		Type        string      `json:"type"`
		TemplateKey string      `json:"template_key"`
		Version     string      `json:"template_version"`
		Title       string      `json:"title"`
		Description string      `json:"description"`
		Formats     []formatDTO `json:"formats"`
		// Scope tells the configurator which extra question to ask.
		Scope string `json:"scope"`
	}

	out := make([]typeDTO, 0, 6)
	for _, tpl := range appreport.Templates() {
		formats := make([]formatDTO, 0, len(tpl.Formats))
		for _, f := range tpl.Formats {
			formats = append(formats, formatDTO{Key: string(f), Label: strings.ToUpper(string(f))})
		}
		out = append(out, typeDTO{
			Type: string(tpl.Type), TemplateKey: tpl.Key, Version: tpl.Version,
			Title: tpl.Title(locale), Description: tpl.Description(locale),
			Formats: formats, Scope: scopeFor(tpl.Type),
		})
	}
	return c.JSON(fiber.Map{
		"types":   out,
		"locales": []fiber.Map{{"key": "fr", "label": "Français"}, {"key": "en", "label": "English"}},
	})
}

// scopeFor says which additional input a type needs, so the configurator can ask
// for it instead of failing on submit.
func scopeFor(t domain.ReportType) string {
	switch t {
	case domain.ReportTypeComplianceByFramework:
		return "framework"
	case domain.ReportTypeAudit:
		return "audit"
	case domain.ReportTypeIncident:
		return "period"
	default:
		return "none"
	}
}

// Create POST /reports — queue a report.
func (h *ReportHandler) Create(c *fiber.Ctx) error {
	var body struct {
		Type        string   `json:"type"`
		Format      string   `json:"format"`
		Locale      string   `json:"locale"`
		From        string   `json:"from"`
		To          string   `json:"to"`
		FrameworkID string   `json:"framework_id"`
		AuditID     string   `json:"audit_id"`
		Recipients  []string `json:"recipients"`
		Supersedes  string   `json:"supersedes"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	in := appreport.CreateInput{
		Type: body.Type, Format: body.Format, Locale: body.Locale,
		Recipients: body.Recipients, RequestedBy: userID(c),
	}

	from, err := parseDate(body.From)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	in.From = from
	to, err := parseDate(body.To)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	in.To = to

	if body.FrameworkID != "" {
		id, err := uuid.Parse(body.FrameworkID)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid framework id"})
		}
		in.FrameworkID = &id
	}
	if body.AuditID != "" {
		id, err := uuid.Parse(body.AuditID)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid audit id"})
		}
		in.AuditID = &id
	}
	if body.Supersedes != "" {
		id, err := uuid.Parse(body.Supersedes)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid report id to supersede"})
		}
		in.Supersedes = &id
	}

	rep, err := h.svc.Create(c.UserContext(), tenantID(c), in)
	if err != nil {
		return writeAppError(c, err)
	}
	// 202: accepted and queued, not produced. The body carries the address the
	// client polls or watches.
	return c.Status(202).JSON(rep)
}

// List GET /reports?limit=5&sort=-created_at — the recent-reports list.
func (h *ReportHandler) List(c *fiber.Ctx) error {
	f := domain.ReportFilter{Limit: 20}

	if v := c.Query("type"); v != "" {
		f.Type = domain.ReportType(v)
	}
	if v := c.Query("lifecycle"); v != "" {
		lc, err := domain.ParseReportLifecycle(v)
		if err != nil {
			return writeAppError(c, err)
		}
		f.Lifecycle = lc
	}
	if v := c.Query("format"); v != "" {
		fm, err := domain.ParseReportFormat(v)
		if err != nil {
			return writeAppError(c, err)
		}
		f.Format = fm
	}
	if n, err := strconv.Atoi(c.Query("limit")); err == nil && n > 0 && n <= 200 {
		f.Limit = n
	}
	if n, err := strconv.Atoi(c.Query("offset")); err == nil && n > 0 {
		f.Offset = n
	}
	// "-created_at" is the documented default; the leading minus is the only
	// direction marker, so anything else means ascending.
	if strings.TrimSpace(c.Query("sort")) == "created_at" {
		f.Sort = "created_at"
	}

	items, total, err := h.svc.List(c.UserContext(), tenantID(c), f)
	if err != nil {
		return writeAppError(c, err)
	}
	return c.JSON(fiber.Map{"items": items, "total": total})
}

func (h *ReportHandler) Get(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("reportId"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid report id"})
	}
	rep, err := h.svc.Get(c.UserContext(), tenantID(c), id)
	if err != nil {
		return writeAppError(c, err)
	}
	return c.JSON(rep)
}

// Download GET /reports/:id/download — the bytes.
func (h *ReportHandler) Download(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("reportId"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid report id"})
	}
	rep, err := h.svc.Download(c.UserContext(), tenantID(c), id)
	if err != nil {
		return writeAppError(c, err)
	}
	c.Set(fiber.HeaderContentDisposition, fmt.Sprintf(`attachment; filename="%s"`, rep.Filename))
	c.Set(fiber.HeaderContentType, rep.ContentType)
	// The hash travels with the download so a client can check the file it
	// received against what the platform recorded, without a second request.
	c.Set("X-Content-SHA256", rep.ContentHash)
	return c.Send(rep.Artifact)
}

// Verify GET /reports/:id/verify — recompute and compare the hash.
func (h *ReportHandler) Verify(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("reportId"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid report id"})
	}
	res, err := h.svc.Verify(c.UserContext(), tenantID(c), id)
	if err != nil {
		return writeAppError(c, err)
	}
	return c.JSON(res)
}

// Transition POST /reports/:id/transition — move through the lifecycle.
func (h *ReportHandler) Transition(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("reportId"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid report id"})
	}
	var body struct {
		To      string `json:"to"`
		Comment string `json:"comment"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}
	rep, err := h.svc.Transition(c.UserContext(), tenantID(c), id, appreport.TransitionInput{
		To: body.To, Comment: body.Comment, Actor: userID(c),
	})
	if err != nil {
		return writeAppError(c, err)
	}
	return c.JSON(rep)
}

// Comment POST /reports/:id/comments.
func (h *ReportHandler) Comment(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("reportId"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid report id"})
	}
	var body struct {
		Body string `json:"body"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}
	comment, err := h.svc.Comment(c.UserContext(), tenantID(c), id, userID(c), body.Body)
	if err != nil {
		return writeAppError(c, err)
	}
	return c.Status(201).JSON(comment)
}

func (h *ReportHandler) Delete(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("reportId"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid report id"})
	}
	if err := h.svc.Delete(c.UserContext(), tenantID(c), id); err != nil {
		return writeAppError(c, err)
	}
	return c.SendStatus(204)
}

// Versions GET /reports/:id/versions — the lineage.
func (h *ReportHandler) Versions(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("reportId"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid report id"})
	}
	versions, err := h.svc.Versions(c.UserContext(), tenantID(c), id)
	if err != nil {
		return writeAppError(c, err)
	}
	return c.JSON(fiber.Map{"versions": versions})
}

// Compare GET /reports/:id/compare?with=<id> — what changed between versions.
func (h *ReportHandler) Compare(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("reportId"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid report id"})
	}
	other, err := uuid.Parse(c.Query("with"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "pass ?with=<report id> to compare against"})
	}
	diff, err := h.svc.CompareVersions(c.UserContext(), tenantID(c), id, other)
	if err != nil {
		return writeAppError(c, err)
	}
	return c.JSON(diff)
}

// Progress GET /reports/:id/progress — server-sent events while it renders.
//
// SSE rather than polling because the whole point of going asynchronous is that
// the user watches something move. It still degrades: the client can poll GET
// /reports/:id, and every event carries the same fields that read returns.
func (h *ReportHandler) Progress(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("reportId"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid report id"})
	}
	tenant := tenantID(c)

	// DEPRECATED (#347). There is NO streaming replacement — the realtime catalog
	// has no report.* events — so the successor is the polling endpoint, which
	// returns the same progress/step/run_state this stream pushes.
	markSSEDeprecated(c, "/api/v1/reports/:reportId")

	// Read once up front: a report that already finished must not leave the
	// client watching an empty stream forever.
	rep, err := h.svc.Get(c.UserContext(), tenant, id)
	if err != nil {
		return writeAppError(c, err)
	}

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no")

	initial, _ := json.Marshal(fiber.Map{
		"report_id": rep.ID, "progress": rep.Progress,
		"step": rep.Step, "run_state": rep.RunState,
	})
	terminal := rep.RunState.Terminal()
	// Read off the request before the writer starts; it runs after this returns.
	jti := streamJTI(c)

	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		fmt.Fprintf(w, "data: %s\n\n", initial)
		if err := w.Flush(); err != nil {
			return
		}
		if terminal {
			return // nothing more will happen; closing beats an idle connection
		}
		if h.redis == nil {
			return // no live channel on this deployment; the client polls instead
		}

		pubsub := h.redis.Subscribe(ctx, ReportProgressChannel)
		defer pubsub.Close()
		msgs := pubsub.Channel()

		keepalive := time.NewTicker(20 * time.Second)
		defer keepalive.Stop()
		// A hard ceiling: a render that never finishes must not hold a connection
		// open for the life of the process.
		deadline := time.After(10 * time.Minute)

		for {
			select {
			case <-deadline:
				return
			case msg, ok := <-msgs:
				if !ok {
					return
				}
				var evt struct {
					TenantID string `json:"tenant_id"`
					ReportID string `json:"report_id"`
					Progress int    `json:"progress"`
					Step     string `json:"step"`
					RunState string `json:"run_state"`
				}
				if err := json.Unmarshal([]byte(msg.Payload), &evt); err != nil {
					continue
				}
				// Filter by tenant AND report: the channel is shared, and a client
				// watching its own report must never see another tenant's progress.
				if evt.TenantID != tenant.String() || evt.ReportID != id.String() {
					continue
				}
				fmt.Fprintf(w, "data: %s\n\n", msg.Payload)
				if err := w.Flush(); err != nil {
					return
				}
				if domain.ReportRunState(evt.RunState).Terminal() {
					return
				}
			case <-keepalive.C:
				// Re-authorize on the tick, bounding revocation to one interval
				// rather than the render's whole lifetime (#345).
				if sseSessionRevoked(jti) {
					fmt.Fprint(w, "event: stream.revoked\ndata: {\"reason\":\"session_revoked\"}\n\n")
					_ = w.Flush()
					return
				}
				fmt.Fprint(w, ": keepalive\n\n")
				if err := w.Flush(); err != nil {
					return
				}
			}
		}
	})
	return nil
}
