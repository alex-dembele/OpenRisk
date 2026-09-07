// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package handler

import (
	"bufio"
	"context"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	scanapp "github.com/opendefender/openrisk/internal/application/scanner"
	"github.com/opendefender/openrisk/internal/domain"
	"github.com/opendefender/openrisk/internal/infrastructure/redis"
	scanpkg "github.com/opendefender/openrisk/internal/scanner"
	authpkg "github.com/opendefender/openrisk/pkg/auth"
	"github.com/opendefender/openrisk/pkg/validation"
)

// ScannerHandler exposes the scan-engine HTTP API. It groups the user-facing
// endpoints (this file) and the agent-facing endpoints (scanner_agent_handler.go).
type ScannerHandler struct {
	createConfig  *scanapp.CreateScanConfigUseCase
	listConfigs   *scanapp.ListScanConfigsUseCase
	getConfig     *scanapp.GetScanConfigUseCase
	deleteConfig  *scanapp.DeleteScanConfigUseCase
	trigger       *scanapp.TriggerScanUseCase
	listAgents    *scanapp.ListAgentsUseCase
	revokeAgent   *scanapp.RevokeAgentUseCase
	register      *scanapp.RegisterAgentUseCase
	push          *scanapp.PushResultsUseCase
	heartbeat     *scanapp.HeartbeatAgentUseCase
	listJobs      *scanapp.ListScanJobsUseCase
	getPreview    *scanapp.GetScanPreviewUseCase
	importPreview *scanapp.ImportPreviewUseCase
	ignorePreview *scanapp.IgnorePreviewUseCase

	agentRepo domain.ScannerAgentRepository
	jobRepo   domain.ScanJobRepository
	cipher    *scanapp.CredentialCipher
	rsaKeys   *authpkg.RSAKeys
	blacklist func(jti string) (bool, error)
	redis     *redis.Client
}

// NewScannerHandler wires the scan-engine use cases and the agent-auth deps.
func NewScannerHandler(
	createConfig *scanapp.CreateScanConfigUseCase,
	listConfigs *scanapp.ListScanConfigsUseCase,
	getConfig *scanapp.GetScanConfigUseCase,
	deleteConfig *scanapp.DeleteScanConfigUseCase,
	trigger *scanapp.TriggerScanUseCase,
	listAgents *scanapp.ListAgentsUseCase,
	revokeAgent *scanapp.RevokeAgentUseCase,
	register *scanapp.RegisterAgentUseCase,
	push *scanapp.PushResultsUseCase,
	heartbeat *scanapp.HeartbeatAgentUseCase,
	listJobs *scanapp.ListScanJobsUseCase,
	getPreview *scanapp.GetScanPreviewUseCase,
	importPreview *scanapp.ImportPreviewUseCase,
	ignorePreview *scanapp.IgnorePreviewUseCase,
	agentRepo domain.ScannerAgentRepository,
	jobRepo domain.ScanJobRepository,
	cipher *scanapp.CredentialCipher,
	rsaKeys *authpkg.RSAKeys,
	blacklist func(jti string) (bool, error),
	redisClient *redis.Client,
) *ScannerHandler {
	return &ScannerHandler{
		createConfig: createConfig, listConfigs: listConfigs, getConfig: getConfig,
		deleteConfig: deleteConfig, trigger: trigger, listAgents: listAgents,
		revokeAgent: revokeAgent, register: register, push: push, heartbeat: heartbeat,
		listJobs: listJobs, getPreview: getPreview, importPreview: importPreview,
		ignorePreview: ignorePreview, agentRepo: agentRepo, jobRepo: jobRepo, cipher: cipher,
		rsaKeys: rsaKeys, blacklist: blacklist, redis: redisClient,
	}
}

// --- Scan configs ----------------------------------------------------------

type createScanConfigInput struct {
	Name            string            `json:"name" validate:"required"`
	Provider        string            `json:"provider" validate:"required,oneof=aws azure gcp nmap agent kubernetes docker vmware active_directory m365 github gitlab"`
	Credentials     map[string]string `json:"credentials"`
	Regions         []string          `json:"regions"`
	Targets         []string          `json:"targets"`
	AgentIDs        []string          `json:"agent_ids"`
	Options         map[string]any    `json:"options"`
	ScheduleMinutes int               `json:"schedule_minutes" validate:"omitempty,min=0"`
}

// CreateScanConfig POST /scanner/configs
func (h *ScannerHandler) CreateScanConfig(c *fiber.Ctx) error {
	in := new(createScanConfigInput)
	if err := c.BodyParser(in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid input format"})
	}
	if err := validation.GetValidator().Struct(in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "validation_failed", "details": err.Error()})
	}
	agentIDs := make([]uuid.UUID, 0, len(in.AgentIDs))
	for _, s := range in.AgentIDs {
		if id, err := uuid.Parse(s); err == nil {
			agentIDs = append(agentIDs, id)
		}
	}
	cfg, err := h.createConfig.Execute(c.UserContext(), tenantID(c), userID(c), scanapp.CreateScanConfigInput{
		Name:            in.Name,
		Provider:        domain.ScannerProvider(in.Provider),
		Credentials:     in.Credentials,
		Regions:         in.Regions,
		Targets:         in.Targets,
		AgentIDs:        agentIDs,
		Options:         in.Options,
		ScheduleMinutes: in.ScheduleMinutes,
	})
	if err != nil {
		return writeAppError(c, err)
	}
	return c.Status(201).JSON(cfg)
}

// ListScanConfigs GET /scanner/configs
func (h *ScannerHandler) ListScanConfigs(c *fiber.Ctx) error {
	cfgs, err := h.listConfigs.Execute(c.UserContext(), tenantID(c))
	if err != nil {
		return writeAppError(c, err)
	}
	return c.JSON(cfgs)
}

// DeleteScanConfig DELETE /scanner/configs/:id
func (h *ScannerHandler) DeleteScanConfig(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid config id"})
	}
	if err := h.deleteConfig.Execute(c.UserContext(), tenantID(c), id); err != nil {
		return writeAppError(c, err)
	}
	return c.SendStatus(204)
}

// TriggerScan POST /scanner/configs/:id/scan
func (h *ScannerHandler) TriggerScan(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid config id"})
	}
	job, err := h.trigger.Execute(c.UserContext(), tenantID(c), userID(c), id)
	if err != nil {
		return writeAppError(c, err)
	}
	return c.Status(202).JSON(job)
}

// IssueRegistrationToken POST /scanner/configs/:id/registration-token
// Mints the 24h token embedded in an Agent download for this tenant + config,
// and returns the per-OS download URLs the onboarding page offers.
func (h *ScannerHandler) IssueRegistrationToken(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid config id"})
	}
	cfg, err := h.getConfig.Execute(c.UserContext(), tenantID(c), id)
	if err != nil {
		return writeAppError(c, err)
	}
	token, err := scanpkg.MintRegistrationToken(h.rsaKeys, tenantID(c), cfg.ID)
	if err != nil {
		return writeAppError(c, domain.NewInternalError(err.Error()))
	}
	return c.JSON(fiber.Map{
		"registration_token": token,
		"expires_at":         time.Now().Add(scanpkg.RegistrationTokenTTL),
		"config_id":          cfg.ID,
		"downloads": fiber.Map{
			"windows": "/downloads/openrisk-agent-windows-amd64.exe",
			"linux":   "/downloads/openrisk-agent-linux-amd64",
			"macos":   "/downloads/openrisk-agent-macos.app.zip",
			"docker":  "opendefender/openrisk-agent:latest",
		},
	})
}

// --- Agents (user side) ----------------------------------------------------

// ListAgents GET /scanner/agents
func (h *ScannerHandler) ListAgents(c *fiber.Ctx) error {
	agents, err := h.listAgents.Execute(c.UserContext(), tenantID(c))
	if err != nil {
		return writeAppError(c, err)
	}
	return c.JSON(agents)
}

// RevokeAgent DELETE /scanner/agents/:id
func (h *ScannerHandler) RevokeAgent(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid agent id"})
	}
	if err := h.revokeAgent.Execute(c.UserContext(), tenantID(c), id); err != nil {
		return writeAppError(c, err)
	}
	return c.SendStatus(204)
}

// --- Jobs & previews -------------------------------------------------------

// ListScanJobs GET /scanner/jobs
func (h *ScannerHandler) ListScanJobs(c *fiber.Ctx) error {
	jobs, err := h.listJobs.Execute(c.UserContext(), tenantID(c))
	if err != nil {
		return writeAppError(c, err)
	}
	return c.JSON(jobs)
}

// GetScanPreview GET /scanner/jobs/:id/preview
func (h *ScannerHandler) GetScanPreview(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid job id"})
	}
	preview, err := h.getPreview.Execute(c.UserContext(), tenantID(c), id)
	if err != nil {
		return writeAppError(c, err)
	}
	return c.JSON(preview)
}

type importPreviewInput struct {
	Selections []struct {
		ExternalID  string `json:"external_id" validate:"required"`
		Criticality string `json:"criticality" validate:"omitempty,oneof=LOW MEDIUM HIGH CRITICAL"`
	} `json:"selections" validate:"required,min=1"`
}

// ImportPreview POST /scanner/jobs/:id/import
func (h *ScannerHandler) ImportPreview(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid job id"})
	}
	in := new(importPreviewInput)
	if err := c.BodyParser(in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid input format"})
	}
	if err := validation.GetValidator().Struct(in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "validation_failed", "details": err.Error()})
	}
	sels := make([]scanapp.ImportSelection, 0, len(in.Selections))
	for _, s := range in.Selections {
		sels = append(sels, scanapp.ImportSelection{
			ExternalID:  s.ExternalID,
			Criticality: domain.AssetCriticality(s.Criticality),
		})
	}
	res, err := h.importPreview.Execute(c.UserContext(), tenantID(c), scanapp.ImportPreviewInput{
		JobID: id, Selections: sels,
	})
	if err != nil {
		return writeAppError(c, err)
	}
	return c.JSON(res)
}

// IgnorePreview POST /scanner/jobs/:id/ignore
func (h *ScannerHandler) IgnorePreview(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid job id"})
	}
	if err := h.ignorePreview.Execute(c.UserContext(), tenantID(c), id); err != nil {
		return writeAppError(c, err)
	}
	return c.SendStatus(204)
}

// StreamScanEvents GET /scanner/events — browser SSE for live scan events
// (preview ready, scan failed). Tenant-scoped by the subscription channel.
func (h *ScannerHandler) StreamScanEvents(c *fiber.Ctx) error {
	tid := tenantID(c)
	if tid == uuid.Nil {
		return c.Status(401).JSON(fiber.Map{"error": "missing tenant"})
	}
	// DEPRECATED (#347). No browser ever consumed this: it requires a Bearer
	// header EventSource cannot send. There is NO streaming replacement — the
	// realtime catalog has no scan.* events — so the successor named here is the
	// polling endpoint the console actually uses. The agent's own stream,
	// /scanner/agent/stream, is a different endpoint and is NOT deprecated.
	markSSEDeprecated(c, "/api/v1/scanner/jobs")

	h.streamChannel(c, scanpkg.SSEChannel(tid), nil, nil)
	return nil
}

// streamChannel is the shared SSE loop: it subscribes to a Redis channel and
// relays each message as an SSE `data:` frame, with a keepalive comment every
// 20s. `preamble` frames (already-JSON payloads) are written right after the
// connection opens — used to replay queued agent jobs so a job published during
// a reconnect gap is never lost. onClose (if set) runs when the client
// disconnects.
func (h *ScannerHandler) streamChannel(c *fiber.Ctx, channel string, onClose func(), preamble []string) {
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no")

	redisClient := h.redis
	// Read off the request before the writer starts: it runs after this returns
	// and the Fiber context is recycled by then. Empty for a caller that carries
	// no session jti — an agent's scoped token — which sseSessionRevoked treats
	// as "nothing to revoke" rather than as a reason to refuse (#345).
	jti := streamJTI(c)
	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		if onClose != nil {
			defer onClose()
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pubsub := redisClient.Subscribe(ctx, channel)
		defer pubsub.Close()
		msgs := pubsub.Channel()

		fmt.Fprint(w, ": connected\n\n")
		if err := w.Flush(); err != nil {
			return
		}
		// Replay any queued jobs first (covers a job published during a reconnect
		// gap — Redis pub/sub is fire-and-forget).
		for _, frame := range preamble {
			fmt.Fprintf(w, "data: %s\n\n", frame)
			if err := w.Flush(); err != nil {
				return
			}
		}
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case m, ok := <-msgs:
				if !ok {
					return
				}
				fmt.Fprintf(w, "data: %s\n\n", m.Payload)
				if err := w.Flush(); err != nil {
					return
				}
			case <-ticker.C:
				// Re-authorize on the tick, bounding revocation to one interval
				// instead of the life of the connection (#345).
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
}
