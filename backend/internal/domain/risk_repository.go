// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// RiskRepository defines the port for risk data persistence.
// Infrastructure layer implements this interface.
// ABSOLUTE RULE: All methods must filter by tenant_id in the repository,
// never in the handler. If a risk belongs to another tenant → return 404 (never 403).
type RiskRepository interface {
	// CRUD Operations

	// Create persists a new risk. Returns error if tenant_id validation fails.
	Create(ctx context.Context, risk *Risk) error

	// GetByID retrieves a risk by ID scoped to a tenant.
	// Returns (nil, nil) if not found (use case handles 404).
	GetByID(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*Risk, error)

	// List retrieves risks with filtering, pagination, and sorting scoped to a tenant.
	List(ctx context.Context, tenantID uuid.UUID, query RiskQuery) (*PaginatedResult[Risk], error)

	// Update updates an existing risk (partial updates via GORM's Save).
	// IMPORTANT: Must include tenant_id in WHERE clause.
	Update(ctx context.Context, risk *Risk) error

	// Delete soft-deletes a risk by ID scoped to a tenant.
	Delete(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) error

	// Count returns the total number of risks for a tenant.
	Count(ctx context.Context, tenantID uuid.UUID) (int64, error)

	// Scoring Operations (called by Score Engine via Redis worker)

	// UpdateScore updates the score and criticality fields for a risk.
	// Used exclusively by the Score Engine worker after Redis event triggers recalculation.
	// MANDATORY: Filter by tenant_id AND id (strict isolation).
	// Returns error if no rows affected (risk not found).
	UpdateScore(ctx context.Context, riskID uuid.UUID, tenantID uuid.UUID, score float64, criticality string) error

	// GetRiskScore retrieves the current score of a risk.
	// Used by the Score Engine worker to compute delta for events.
	GetRiskScore(ctx context.Context, riskID uuid.UUID, tenantID uuid.UUID) (float64, error)

	// GetRisksByAssetID retrieves all risks linked to an asset.
	// Called when asset criticality changes (to trigger risk recalculation).
	GetRisksByAssetID(ctx context.Context, assetID uuid.UUID, tenantID uuid.UUID) ([]RiskForScoring, error)

	// Audit & History

	// GetHistory retrieves paginated audit log entries for a risk.
	// Returns old_value and new_value for each change.
	GetHistory(ctx context.Context, riskID uuid.UUID, tenantID uuid.UUID, page, limit int) ([]AuditLogEntry, error)

	// CreateAuditEntry creates an audit log entry for a risk change.
	// Called by use cases to track mutations.
	CreateAuditEntry(ctx context.Context, entry *AuditLogEntry) error

	// Advanced Queries

	// GetBySource retrieves risks filtered by source (manual, cti_auto, scan_auto, etc.).
	GetBySource(ctx context.Context, tenantID uuid.UUID, source string) ([]Risk, error)

	// GetByCVE retrieves a risk by CVE ID.
	GetByCVE(ctx context.Context, cveID string, tenantID uuid.UUID) (*Risk, error)

	// BulkUpdate updates multiple risks atomically within a transaction.
	// Returns count of updated records and error.
	// MANDATORY: Transaction must fail entirely if any UPDATE fails.
	BulkUpdate(ctx context.Context, tenantID uuid.UUID, updates []RiskUpdate) (int64, error)

	// BulkCreate creates multiple risks atomically within a transaction.
	BulkCreate(ctx context.Context, risks []*Risk) (int64, error)

	// BulkDelete soft-deletes multiple risks atomically.
	BulkDelete(ctx context.Context, ids []uuid.UUID, tenantID uuid.UUID) (int64, error)

	// BulkApply loads every named risk (tenant-scoped), applies mutate to each,
	// and persists them all inside ONE transaction. It returns the risks as they
	// were BEFORE mutation and as they are AFTER, paired, so the caller can
	// journal a faithful before → after per risk.
	//
	// All-or-nothing (D-036, option A). If ANY id is absent — deleted since the
	// selection was made, fabricated, or belonging to another tenant — nothing is
	// written and the call fails with ErrNotFound. Same for a mutate that
	// refuses, or a write that errors. There is deliberately no partial outcome
	// to report: "31 of 40 risks were reassigned and we cannot tell you which"
	// is a supervisory finding, and a clean rollback is not.
	//
	// A foreign-tenant id is indistinguishable from a fabricated one, because the
	// load is scoped by tenant_id and both simply fail to resolve.
	BulkApply(ctx context.Context, tenantID uuid.UUID, ids []uuid.UUID, mutate func(*Risk) error) ([]RiskMutation, error)
}

// RiskMutation is one risk's before → after pair from a BulkApply, and is what
// the audit journal is built from. Both snapshots are deep enough to explain the
// change and no deeper: a bulk action changes a handful of named fields.
type RiskMutation struct {
	RiskID uuid.UUID
	Before map[string]interface{}
	After  map[string]interface{}
	// ChangedFields names the keys that actually differ, so a no-op item (a tag
	// removal for a tag the risk never carried) is visible as such.
	ChangedFields []string
}

// RiskQuery encapsulates filtering/pagination parameters for listing risks.
// Sanitize() must be called before use to prevent SQL injection.
type RiskQuery struct {
	// Filtering
	Search         string   // Full-text search on title + description (PostgreSQL tsvector 'french')
	Status         []string // Filter by statuses (OR condition)
	Criticality    []string // Filter by criticality levels
	LifecyclePhase []string // Filter by ISO 31000 lifecycle phase (OR condition)
	// LifecycleState filters on the CANONICAL state column (draft, identified,
	// …). Prefer it over LifecyclePhase and Status, which are derived from it.
	LifecycleState []string
	Framework      string     // Filter by single framework (iso27001|cobac|bceao|etc.)
	AssetID        *uuid.UUID // Filter by linked asset
	AssignedTo     *uuid.UUID // Filter by assignee (matches the legacy assigned_to OR assignee_id)
	OwnedBy        *uuid.UUID // Filter by owner (responsable)
	ReviewedBy     *uuid.UUID // Filter by reviewer (validateur)
	// InvolvedUser backs the "Mes risques" filter: owner OR assignee OR reviewer.
	InvolvedUser *uuid.UUID
	Tags         []string    // Filter by tags (OR condition) — free text
	CategoryIDs  []uuid.UUID // Filter by controlled category (OR condition)
	// FrameworkID filters on the RISK↔CONTROL mappings, not on the frozen
	// free-text `frameworks` column.
	FrameworkID *uuid.UUID
	// Unmapped true keeps only risks with no control mapping at all.
	Unmapped      bool
	DateFrom      *time.Time // Created date range start
	DateTo        *time.Time // Created date range end
	Source        []string   // Filter by source (manual|cti_auto|scan_auto|etc.)
	MinScore      *float64   // Score range (new system: 0.0-30.0)
	MaxScore      *float64
	TreatmentPlan []string // accept|mitigate|transfer|avoid

	// Pagination
	Page  int // 1-indexed, validated in Sanitize()
	Limit int // max items per page, capped at 100

	// Sorting
	SortBy    string // Whitelisted field name (prevent SQL injection)
	SortOrder string // "asc" or "desc" only

	// Full-text search options
	SearchLanguage string // "french", "english", "german", etc. (default: french)
}

// PaginatedResult wraps a paginated query result.
type PaginatedResult[T any] struct {
	Data       []T   `json:"data"`
	Total      int64 `json:"total"`
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	TotalPages int   `json:"total_pages"`
}

// RiskUpdate represents a partial update for bulk operations.
type RiskUpdate struct {
	ID     uuid.UUID
	Status *RiskStatus
	Score  *float64
	// Add other patchable fields as needed
}

// RiskForScoring is the minimal struct for Score Engine worker recalculation.
type RiskForScoring struct {
	ID               uuid.UUID
	TenantID         uuid.UUID
	Probability      float64
	Impact           float64
	AssetCriticality float64 // Will be fetched from Asset.Criticality
	CurrentScore     float64
}

// NewRiskQuery returns a RiskQuery with sensible defaults.
func NewRiskQuery() RiskQuery {
	return RiskQuery{
		Page:           1,
		Limit:          20,
		SortBy:         "created_at",
		SortOrder:      "desc",
		SearchLanguage: "french",
	}
}

// Offset calculates the SQL offset from page and limit.
func (q RiskQuery) Offset() int {
	if q.Page < 1 {
		return 0
	}
	return (q.Page - 1) * q.Limit
}

// Sanitize ensures all values are within acceptable bounds.
// MUST be called before using the query.
func (q *RiskQuery) Sanitize() {
	// Pagination
	if q.Page < 1 {
		q.Page = 1
	}
	if q.Limit < 1 {
		q.Limit = 20
	}
	if q.Limit > 100 {
		q.Limit = 100
	}

	// Sorting
	if q.SortBy == "" {
		q.SortBy = "created_at"
	}
	if q.SortOrder == "" || (q.SortOrder != "asc" && q.SortOrder != "desc") {
		q.SortOrder = "desc"
	}

	// Whitelist sortable fields to prevent SQL injection
	allowed := map[string]bool{
		"created_at":  true,
		"updated_at":  true,
		"name":        true,
		"title":       true,
		"score":       true,
		"criticality": true,
		"status":      true,
		"impact":      true,
		"probability": true,
		"assigned_to": true,
		"source":      true,
	}
	if !allowed[q.SortBy] {
		q.SortBy = "created_at"
	}

	// Search language
	if q.SearchLanguage == "" {
		q.SearchLanguage = "french"
	}
}
