// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package repository

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/opendefender/openrisk/internal/application/dashboard"
	"github.com/opendefender/openrisk/internal/domain"
)

// GormRiskRepository implements domain.RiskRepository using GORM.
// ABSOLUTE RULES:
// - Filter by tenant_id on EVERY query (never in handler)
// - If risk belongs to another tenant → return 404 (never 403)
// - Create audit entries on mutations
type GormRiskRepository struct {
	db *gorm.DB
}

// NewGormRiskRepository creates a new GORM-backed risk repository.
func NewGormRiskRepository(db *gorm.DB) *GormRiskRepository {
	return &GormRiskRepository{db: db}
}

// =============================================================================
// CRUD Operations
// =============================================================================

// Create persists a new risk.
func (r *GormRiskRepository) Create(ctx context.Context, risk *domain.Risk) error {
	// Validate tenant_id is set
	if risk.TenantID == uuid.Nil {
		return fmt.Errorf("tenant_id is required")
	}

	return r.db.WithContext(ctx).Create(risk).Error
}

// GetByID retrieves a risk by ID scoped to a tenant.
// Returns (nil, nil) if not found (use case handles 404).
func (r *GormRiskRepository) GetByID(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*domain.Risk, error) {
	var risk domain.Risk
	err := r.db.WithContext(ctx).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		Preload("Mitigations").
		Preload("Assets").
		// The controlled category is what the "Catégorie" column renders. Without
		// this the column shipped dead: the row carried category_id but no name,
		// so a classified risk read as unclassified.
		Preload("Category").
		First(&risk).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil // Not found → nil, nil (use case handles this)
		}
		return nil, fmt.Errorf("failed to get risk: %w", err)
	}
	risk.CountMitigations()
	return &risk, nil
}

// List retrieves risks with filtering, pagination, and sorting scoped to a tenant.
func (r *GormRiskRepository) List(ctx context.Context, tenantID uuid.UUID, query domain.RiskQuery) (*domain.PaginatedResult[domain.Risk], error) {
	query.Sanitize()

	db := r.db.WithContext(ctx).
		Model(&domain.Risk{}).
		Where("tenant_id = ?", tenantID)

	// Apply filters
	if query.Search != "" {
		// Full-text search (tsvector on name+description) OR'd with a prefix-ish
		// ILIKE. The tsvector alone is lexeme-exact: typing "log" matched nothing
		// while "Log4Shell" sat in the register, which made the register's search
		// box look broken. The ILIKE arm only ever ADDS rows, so full-text ranking
		// behaviour for real words is unchanged. Both arms are bound parameters.
		like := "%" + query.Search + "%"
		db = db.Where(
			"(to_tsvector(?, name || ' ' || COALESCE(description, '')) @@ plainto_tsquery(?, ?) OR name ILIKE ? OR title ILIKE ? OR COALESCE(description, '') ILIKE ?)",
			query.SearchLanguage,
			query.SearchLanguage,
			query.Search,
			like,
			like,
			like,
		)
	}

	// Status filter (supports multiple statuses)
	if len(query.Status) > 0 {
		db = db.Where("status IN ?", query.Status)
	}

	// Criticality filter
	if len(query.Criticality) > 0 {
		db = db.Where("criticality IN ?", query.Criticality)
	}

	// Lifecycle phase filter (ISO 31000)
	if len(query.LifecyclePhase) > 0 {
		db = db.Where("lifecycle_phase IN ?", query.LifecyclePhase)
	}

	// Canonical lifecycle state filter — the source of truth the other two are
	// derived from (see domain/risk_state.go).
	if len(query.LifecycleState) > 0 {
		db = db.Where("lifecycle_state IN ?", query.LifecycleState)
	}

	// Framework filter (single framework)
	if query.Framework != "" {
		db = db.Where("? = ANY(frameworks)", query.Framework)
	}

	// Asset filter
	if query.AssetID != nil {
		db = db.Where("asset_id = ?", *query.AssetID)
	}

	// Assigned to filter. The legacy `assigned_to` column and the new
	// `assignee_id` slot are OR'd: a row assigned before migration 0044 through
	// the old column must still answer this facet.
	if query.AssignedTo != nil {
		db = db.Where("(assigned_to = ? OR assignee_id = ?)", *query.AssignedTo, *query.AssignedTo)
	}

	// "Mes risques" — any of the three accountability slots. This is the whole
	// point of splitting them: a single filter can now mean "anything I answer
	// for, work on, or must validate".
	if query.InvolvedUser != nil {
		u := *query.InvolvedUser
		db = db.Where("(owner_id = ? OR assignee_id = ? OR reviewer_id = ? OR assigned_to = ?)", u, u, u, u)
	}
	if query.OwnedBy != nil {
		db = db.Where("owner_id = ?", *query.OwnedBy)
	}
	if query.ReviewedBy != nil {
		db = db.Where("reviewer_id = ?", *query.ReviewedBy)
	}

	// Tags filter (OR condition) — free text, unrelated to categories.
	if len(query.Tags) > 0 {
		db = db.Where("tags && ?", pq.Array(query.Tags))
	}

	// Category filter — the CONTROLLED vocabulary.
	if len(query.CategoryIDs) > 0 {
		db = db.Where("category_id IN ?", query.CategoryIDs)
	}

	// Compliance filters run against the real mappings, never against the frozen
	// free-text `frameworks` column.
	if query.FrameworkID != nil {
		db = db.Where(`EXISTS (
			SELECT 1 FROM risk_control_mappings m
			WHERE m.risk_id = risks.id AND m.tenant_id = risks.tenant_id
			  AND m.framework_id = ? AND m.deleted_at IS NULL)`, *query.FrameworkID)
	}
	if query.Unmapped {
		db = db.Where(`NOT EXISTS (
			SELECT 1 FROM risk_control_mappings m
			WHERE m.risk_id = risks.id AND m.tenant_id = risks.tenant_id
			  AND m.deleted_at IS NULL)`)
	}

	// Date range filter
	if query.DateFrom != nil {
		db = db.Where("created_at >= ?", *query.DateFrom)
	}
	if query.DateTo != nil {
		db = db.Where("created_at <= ?", *query.DateTo)
	}

	// Source filter (multiple sources)
	if len(query.Source) > 0 {
		db = db.Where("source IN ?", query.Source)
	}

	// Score range filter
	if query.MinScore != nil {
		db = db.Where("score >= ?", *query.MinScore)
	}
	if query.MaxScore != nil {
		db = db.Where("score <= ?", *query.MaxScore)
	}

	// Treatment plan filter
	if len(query.TreatmentPlan) > 0 {
		db = db.Where("treatment_plan IN ?", query.TreatmentPlan)
	}

	// Count total before pagination
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("failed to count risks: %w", err)
	}

	// Apply pagination and sorting
	var risks []domain.Risk
	err := db.
		Order(fmt.Sprintf("%s %s", query.SortBy, query.SortOrder)).
		Offset(query.Offset()).
		Limit(query.Limit).
		Preload("Mitigations").
		Preload("Assets").
		// The controlled category is what the "Catégorie" column renders. Without
		// this the column shipped dead: the row carried category_id but no name,
		// so a classified risk read as unclassified.
		Preload("Category").
		Find(&risks).Error

	if err != nil {
		return nil, fmt.Errorf("failed to list risks: %w", err)
	}
	for i := range risks {
		risks[i].CountMitigations()
	}

	totalPages := int(math.Ceil(float64(total) / float64(query.Limit)))

	return &domain.PaginatedResult[domain.Risk]{
		Data:       risks,
		Total:      total,
		Page:       query.Page,
		Limit:      query.Limit,
		TotalPages: totalPages,
	}, nil
}

// Update updates an existing risk.
func (r *GormRiskRepository) Update(ctx context.Context, risk *domain.Risk) error {
	return r.db.WithContext(ctx).Save(risk).Error
}

// Delete soft-deletes a risk by ID scoped to a tenant.
func (r *GormRiskRepository) Delete(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) error {
	result := r.db.WithContext(ctx).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		Delete(&domain.Risk{})

	if result.Error != nil {
		return fmt.Errorf("failed to delete risk: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("risk not found")
	}
	return nil
}

// Count returns the total number of risks for a tenant.
func (r *GormRiskRepository) Count(ctx context.Context, tenantID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&domain.Risk{}).
		Where("tenant_id = ?", tenantID).
		Count(&count).Error
	return count, err
}

// CountRisksByCriticality returns, for a tenant, the number of active (non-deleted)
// risks grouped by criticality level, keyed by the lowercased level
// ("critical"/"high"/"medium"/"low"). Levels absent from the register are simply
// absent from the map. criticality is normalized with LOWER() because the column
// carries two historical vocabularies (low/high and LOW/HIGH) — see CLAUDE.md.
//
// This backs the board report's risk posture; it is a concrete-type method (not on
// the domain.RiskRepository port) so existing mocks of that interface stay valid.
func (r *GormRiskRepository) CountRisksByCriticality(ctx context.Context, tenantID uuid.UUID) (map[string]int, error) {
	type row struct {
		Criticality string
		Count       int
	}
	var rows []row
	err := r.db.WithContext(ctx).
		Model(&domain.Risk{}).
		Select("LOWER(criticality) AS criticality, COUNT(*) AS count").
		Where("tenant_id = ?", tenantID).
		Group("LOWER(criticality)").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[string]int, len(rows))
	for _, rr := range rows {
		out[rr.Criticality] = rr.Count
	}
	return out, nil
}

// ListRisksForFinancial returns, for a tenant, every active (non-deleted) risk so
// the CRQ financial model can be run in aggregate. It loads the full model (no
// Preload, so associations stay empty) rather than a hand-written column list —
// GORM maps the monetary columns correctly regardless of their derived names
// (e.g. SLEXAF → "slexaf"), which a raw SELECT would get wrong. Like
// CountRisksByCriticality it is a concrete-type method (not on the
// domain.RiskRepository port) so existing mocks stay valid; the financial-summary
// use case depends on a narrow port instead. Registers are small, so the extra
// columns are immaterial.
// SearchByText is a lightweight, tenant-scoped substring search over a risk's
// name/title/description for the universal search palette. Unlike List's full-text
// tsvector match (strict, whole-word), this ILIKE match is forgiving of partial
// words (type-ahead) and returns only a capped, score-ordered slice. Concrete
// method (off the RiskRepository port, like ListRisksForFinancial), so mocks stay
// intact.
func (r *GormRiskRepository) SearchByText(ctx context.Context, tenantID uuid.UUID, q string, limit int) ([]domain.Risk, error) {
	if limit <= 0 || limit > 50 {
		limit = 8
	}
	like := "%" + strings.ToLower(strings.TrimSpace(q)) + "%"
	var risks []domain.Risk
	err := r.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Where("LOWER(name) LIKE ? OR LOWER(title) LIKE ? OR LOWER(COALESCE(description,'')) LIKE ?", like, like, like).
		Order("score DESC").
		Limit(limit).
		Find(&risks).Error
	return risks, err
}

// CountFinancialCoverage returns, for a tenant, the total active risk count and
// how many are "quantified" — carrying an explicit SLE or at least one loss
// component (downtime pair, fines, data-loss or other direct cost). This is a
// single SQL aggregate so the "N/M risks quantified" counter is a server fact,
// not a client-side filter that can drift (spec §6). Concrete method (off the
// RiskRepository port) so mocks stay valid.
func (r *GormRiskRepository) CountFinancialCoverage(ctx context.Context, tenantID uuid.UUID) (total, quantified int, err error) {
	type row struct {
		Total      int64
		Quantified int64
	}
	var res row
	err = r.db.WithContext(ctx).
		Model(&domain.Risk{}).
		Select(`COUNT(*) AS total,
			COUNT(*) FILTER (WHERE
				sle_xaf IS NOT NULL
				OR fines_xaf IS NOT NULL
				OR data_loss_cost_xaf IS NOT NULL
				OR other_direct_cost_xaf IS NOT NULL
				OR (downtime_hours IS NOT NULL AND hourly_downtime_cost_xaf IS NOT NULL)
			) AS quantified`).
		Where("tenant_id = ?", tenantID).
		Scan(&res).Error
	return int(res.Total), int(res.Quantified), err
}

func (r *GormRiskRepository) ListRisksForFinancial(ctx context.Context, tenantID uuid.UUID) ([]domain.Risk, error) {
	var risks []domain.Risk
	err := r.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Find(&risks).Error
	if err != nil {
		return nil, err
	}
	return risks, nil
}

// TopRisksByScore returns the tenant's highest-scoring active risks, capped at
// limit, for the executive "top 10 risks" widget. It loads the full model (no
// hand-written SELECT) so the executive use case can quantify each one's ALE from
// the stored monetary drivers. Concrete method (off the domain.RiskRepository
// port) so existing mocks stay valid — same rationale as ListRisksForFinancial.
func (r *GormRiskRepository) TopRisksByScore(ctx context.Context, tenantID uuid.UUID, limit int) ([]domain.Risk, error) {
	if limit <= 0 {
		limit = 10
	}
	var risks []domain.Risk
	err := r.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Order("score DESC").
		Limit(limit).
		Find(&risks).Error
	if err != nil {
		return nil, err
	}
	return risks, nil
}

// MonthlyRiskTrend returns the register's risk evolution over the last `months`
// months — one point per month that has data. Points come from risk_histories
// (score snapshots, joined to risks so the query stays tenant-scoped); the current
// month is always anchored to the LIVE register average so the latest point is
// meaningful even before any history has accrued for a fresh tenant. Critical/high
// counts use the documented Score Engine bands (score ≥ 7 = critical, ≥ 4 = high).
func (r *GormRiskRepository) MonthlyRiskTrend(ctx context.Context, tenantID uuid.UUID, months int) ([]dashboard.MonthlyRiskPoint, error) {
	if months <= 0 {
		months = 6
	}
	now := time.Now().UTC()
	since := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -(months - 1), 0)

	type histRow struct {
		Month    string
		AvgScore float64
		Total    int
		Critical int
		High     int
	}
	var rows []histRow
	err := r.db.WithContext(ctx).
		Table("risk_histories AS h").
		Joins("JOIN risks r ON r.id = h.risk_id").
		Where("r.tenant_id = ? AND h.created_at >= ?", tenantID, since).
		Select("to_char(h.created_at, 'YYYY-MM') AS month, " +
			"AVG(h.score) AS avg_score, " +
			"COUNT(*) AS total, " +
			"COUNT(*) FILTER (WHERE h.score >= 7) AS critical, " +
			"COUNT(*) FILTER (WHERE h.score >= 4 AND h.score < 7) AS high").
		Group("month").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	byMonth := make(map[string]dashboard.MonthlyRiskPoint, len(rows)+1)
	for _, rr := range rows {
		byMonth[rr.Month] = dashboard.MonthlyRiskPoint{
			Month:    rr.Month,
			AvgScore: math.Round(rr.AvgScore*100) / 100,
			Critical: rr.Critical,
			High:     rr.High,
			Total:    rr.Total,
		}
	}

	// Anchor the current month to the live register.
	var live struct {
		AvgScore float64
		Total    int
		Critical int
		High     int
	}
	_ = r.db.WithContext(ctx).
		Model(&domain.Risk{}).
		Where("tenant_id = ?", tenantID).
		Select("COALESCE(AVG(score),0) AS avg_score, " +
			"COUNT(*) AS total, " +
			"COUNT(*) FILTER (WHERE score >= 7) AS critical, " +
			"COUNT(*) FILTER (WHERE score >= 4 AND score < 7) AS high").
		Scan(&live).Error
	if live.Total > 0 {
		cur := now.Format("2006-01")
		byMonth[cur] = dashboard.MonthlyRiskPoint{
			Month:    cur,
			AvgScore: math.Round(live.AvgScore*100) / 100,
			Critical: live.Critical,
			High:     live.High,
			Total:    live.Total,
		}
	}

	out := make([]dashboard.MonthlyRiskPoint, 0, len(byMonth))
	for _, p := range byMonth {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Month < out[j].Month })
	return out, nil
}

// =============================================================================
// Scoring Operations (called by Score Engine worker)
// =============================================================================

// UpdateScore updates the score and criticality fields for a risk.
// Called exclusively by the Score Engine worker after Redis event triggers recalculation.
func (r *GormRiskRepository) UpdateScore(ctx context.Context, riskID uuid.UUID, tenantID uuid.UUID, score float64, criticality string) error {
	result := r.db.WithContext(ctx).
		Model(&domain.Risk{}).
		Where("id = ? AND tenant_id = ?", riskID, tenantID).
		Update("score", score).
		Update("criticality", criticality).
		Update("updated_at", time.Now())

	if result.Error != nil {
		return fmt.Errorf("failed to update risk score: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("risk not found (tenant isolation)")
	}
	return nil
}

// UpdateSmartScore persists the multifactor smart score, its criticality band,
// the frozen per-factor breakdown and the computation timestamp for a risk.
// Targeted column update (like UpdateScore) — does not run the full Save path, so
// it never spams the risk-history timeline. MANDATORY: filter by tenant_id AND id.
func (r *GormRiskRepository) UpdateSmartScore(ctx context.Context, riskID uuid.UUID, tenantID uuid.UUID, score float64, level string, factors datatypes.JSON, computedAt time.Time) error {
	result := r.db.WithContext(ctx).
		Model(&domain.Risk{}).
		Where("id = ? AND tenant_id = ?", riskID, tenantID).
		Updates(map[string]interface{}{
			"smart_score":       score,
			"smart_level":       level,
			"smart_factors":     factors,
			"smart_computed_at": computedAt,
			"updated_at":        time.Now(),
		})
	if result.Error != nil {
		return fmt.Errorf("failed to update smart score: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("risk not found (tenant isolation)")
	}
	return nil
}

// GetRiskScore retrieves the current score of a risk.
func (r *GormRiskRepository) GetRiskScore(ctx context.Context, riskID uuid.UUID, tenantID uuid.UUID) (float64, error) {
	var score float64
	err := r.db.WithContext(ctx).
		Model(&domain.Risk{}).
		Where("id = ? AND tenant_id = ?", riskID, tenantID).
		Select("score").
		Scan(&score).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return 0, fmt.Errorf("risk not found")
		}
		return 0, fmt.Errorf("failed to get risk score: %w", err)
	}
	return score, nil
}

// GetRisksByAssetID retrieves all risks linked to an asset, so the caller can
// republish risk.updated for each of them when the asset's criticality changes.
//
// NOTE: this used to select "assets.criticality as asset_criticality" (a
// string enum column like "MEDIUM") directly into RiskForScoring's
// AssetCriticality float64 field — a scan type mismatch that would fail at
// runtime, and it only ever considered the ONE asset whose ID was passed in,
// discarding any other assets also linked to the same risk. Fixed by loading
// every linked asset's criticality per affected risk and averaging their
// domain.AssetCriticality.ScoreFactor() — consistent with how a risk's score
// is computed everywhere else a risk has multiple assets.
func (r *GormRiskRepository) GetRisksByAssetID(ctx context.Context, assetID uuid.UUID, tenantID uuid.UUID) ([]domain.RiskForScoring, error) {
	var riskIDs []uuid.UUID
	if err := r.db.WithContext(ctx).
		Table("risks").
		Joins("JOIN risk_assets ON risks.id = risk_assets.risk_id").
		Where("risks.tenant_id = ? AND risk_assets.asset_id = ?", tenantID, assetID).
		Distinct().
		Pluck("risks.id", &riskIDs).Error; err != nil {
		return nil, fmt.Errorf("failed to find risks linked to asset: %w", err)
	}
	if len(riskIDs) == 0 {
		return nil, nil
	}

	type riskRow struct {
		ID           uuid.UUID
		TenantID     uuid.UUID
		Probability  float64
		Impact       float64
		CurrentScore float64
	}
	var riskRows []riskRow
	if err := r.db.WithContext(ctx).
		Table("risks").
		Select("id, tenant_id, probability, impact, score as current_score").
		Where("id IN ?", riskIDs).
		Scan(&riskRows).Error; err != nil {
		return nil, fmt.Errorf("failed to load risks for scoring: %w", err)
	}

	type assetLinkRow struct {
		RiskID      uuid.UUID
		Criticality domain.AssetCriticality
	}
	var links []assetLinkRow
	if err := r.db.WithContext(ctx).
		Table("risk_assets").
		Select("risk_assets.risk_id as risk_id, assets.criticality as criticality").
		Joins("JOIN assets ON risk_assets.asset_id = assets.id").
		Where("risk_assets.risk_id IN ?", riskIDs).
		Scan(&links).Error; err != nil {
		return nil, fmt.Errorf("failed to load linked asset criticalities: %w", err)
	}

	factorSums := make(map[uuid.UUID]float64, len(riskIDs))
	factorCounts := make(map[uuid.UUID]int, len(riskIDs))
	for _, link := range links {
		factorSums[link.RiskID] += link.Criticality.ScoreFactor()
		factorCounts[link.RiskID]++
	}

	risks := make([]domain.RiskForScoring, 0, len(riskRows))
	for _, row := range riskRows {
		factor := 1.0
		if count := factorCounts[row.ID]; count > 0 {
			factor = factorSums[row.ID] / float64(count)
		}
		risks = append(risks, domain.RiskForScoring{
			ID:               row.ID,
			TenantID:         row.TenantID,
			Probability:      row.Probability,
			Impact:           row.Impact,
			AssetCriticality: factor,
			CurrentScore:     row.CurrentScore,
		})
	}
	return risks, nil
}

// =============================================================================
// Audit & History Operations
// =============================================================================

// GetHistory retrieves paginated audit log entries for a risk.
func (r *GormRiskRepository) GetHistory(ctx context.Context, riskID uuid.UUID, tenantID uuid.UUID, page, limit int) ([]domain.AuditLogEntry, error) {
	// Validate pagination
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	// The tenant predicate leads, and the note that used to stand here ("add a
	// tenant_id filter IF audit_logs has the column") is answered: it does, since
	// #532. Until then this read filtered on risk_id alone — the same defect as
	// GET /api/v1/audit-logs, on the same table. No route reaches this method
	// today (internal/api/http/handlers is not wired into main.go), which is why
	// it survived the #532 sweep of the routed reads; an unscoped query that
	// nothing calls yet is how the next caller inherits the defect.
	var history []domain.AuditLogEntry
	err := r.db.WithContext(ctx).
		Table("audit_logs").
		Where("tenant_id = ? AND risk_id = ?", tenantID, riskID).
		Order("timestamp DESC").
		Offset((page - 1) * limit).
		Limit(limit).
		Scan(&history).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get risk history: %w", err)
	}
	return history, nil
}

// CreateAuditEntry creates an audit log entry for a risk change.
func (r *GormRiskRepository) CreateAuditEntry(ctx context.Context, entry *domain.AuditLogEntry) error {
	return r.db.WithContext(ctx).
		Table("audit_logs").
		Create(entry).Error
}

// =============================================================================
// Advanced Queries
// =============================================================================

// GetBySource retrieves risks filtered by source.
func (r *GormRiskRepository) GetBySource(ctx context.Context, tenantID uuid.UUID, source string) ([]domain.Risk, error) {
	var risks []domain.Risk
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND source = ?", tenantID, source).
		Find(&risks).Error
	return risks, err
}

// GetByCVE retrieves a risk by CVE ID.
func (r *GormRiskRepository) GetByCVE(ctx context.Context, cveID string, tenantID uuid.UUID) (*domain.Risk, error) {
	var risk domain.Risk
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND source_cve_id = ?", tenantID, cveID).
		First(&risk).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get risk by CVE: %w", err)
	}
	return &risk, nil
}

// BulkUpdate updates multiple risks atomically within a transaction.
func (r *GormRiskRepository) BulkUpdate(ctx context.Context, tenantID uuid.UUID, updates []domain.RiskUpdate) (int64, error) {
	tx := r.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return 0, tx.Error
	}

	updatedCount := int64(0)
	for _, update := range updates {
		db := tx.Model(&domain.Risk{}).
			Where("id = ? AND tenant_id = ?", update.ID, tenantID)

		if update.Status != nil {
			db = db.Update("status", *update.Status)
		}
		if update.Score != nil {
			db = db.Update("score", *update.Score)
		}

		if db.Error != nil {
			tx.Rollback()
			return 0, fmt.Errorf("failed to update risk %s: %w", update.ID, db.Error)
		}
		updatedCount += db.RowsAffected
	}

	if err := tx.Commit().Error; err != nil {
		return 0, fmt.Errorf("failed to commit bulk update: %w", err)
	}

	return updatedCount, nil
}

// BulkCreate creates multiple risks atomically within a transaction.
func (r *GormRiskRepository) BulkCreate(ctx context.Context, risks []*domain.Risk) (int64, error) {
	tx := r.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return 0, tx.Error
	}

	if err := tx.CreateInBatches(risks, 100).Error; err != nil {
		tx.Rollback()
		return 0, fmt.Errorf("failed to bulk create risks: %w", err)
	}

	if err := tx.Commit().Error; err != nil {
		return 0, fmt.Errorf("failed to commit bulk create: %w", err)
	}

	return int64(len(risks)), nil
}

// BulkDelete soft-deletes multiple risks atomically.
func (r *GormRiskRepository) BulkDelete(ctx context.Context, ids []uuid.UUID, tenantID uuid.UUID) (int64, error) {
	result := r.db.WithContext(ctx).
		Where("id IN ? AND tenant_id = ?", ids, tenantID).
		Delete(&domain.Risk{})

	if result.Error != nil {
		return 0, fmt.Errorf("failed to bulk delete risks: %w", result.Error)
	}

	return result.RowsAffected, nil
}
