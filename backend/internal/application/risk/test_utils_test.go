// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package risk

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/opendefender/openrisk/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockRiskRepository is a mock implementation for testing
type MockRiskRepository struct {
	createFunc            func(ctx context.Context, risk *domain.Risk) error
	getByIDFunc           func(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*domain.Risk, error)
	updateFunc            func(ctx context.Context, risk *domain.Risk) error
	deleteFunc            func(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) error
	countFunc             func(ctx context.Context, tenantID uuid.UUID) (int64, error)
	updateScoreFunc       func(ctx context.Context, riskID uuid.UUID, tenantID uuid.UUID, score float64, criticality string) error
	getRiskScoreFunc      func(ctx context.Context, riskID uuid.UUID, tenantID uuid.UUID) (float64, error)
	getRisksByAssetIDFunc func(ctx context.Context, assetID uuid.UUID, tenantID uuid.UUID) ([]domain.RiskForScoring, error)
	getHistoryFunc        func(ctx context.Context, riskID uuid.UUID, tenantID uuid.UUID, page, limit int) ([]domain.AuditLogEntry, error)
	createAuditEntryFunc  func(ctx context.Context, entry *domain.AuditLogEntry) error
	getBySourceFunc       func(ctx context.Context, tenantID uuid.UUID, source string) ([]domain.Risk, error)
	getByCVEFunc          func(ctx context.Context, cveID string, tenantID uuid.UUID) (*domain.Risk, error)
	bulkUpdateFunc        func(ctx context.Context, tenantID uuid.UUID, updates []domain.RiskUpdate) (int64, error)
	bulkCreateFunc        func(ctx context.Context, risks []*domain.Risk) (int64, error)
	bulkDeleteFunc        func(ctx context.Context, ids []uuid.UUID, tenantID uuid.UUID) (int64, error)
	bulkApplyFunc         func(ctx context.Context, tenantID uuid.UUID, ids []uuid.UUID, mutate func(*domain.Risk) error) ([]domain.RiskMutation, error)
	listFunc              func(ctx context.Context, tenantID uuid.UUID, query domain.RiskQuery) (*domain.PaginatedResult[domain.Risk], error)
}

func (m *MockRiskRepository) BulkApply(
	ctx context.Context,
	tenantID uuid.UUID,
	ids []uuid.UUID,
	mutate func(*domain.Risk) error,
) ([]domain.RiskMutation, error) {
	if m.bulkApplyFunc != nil {
		return m.bulkApplyFunc(ctx, tenantID, ids, mutate)
	}
	return nil, nil
}

func (m *MockRiskRepository) Create(ctx context.Context, risk *domain.Risk) error {
	if m.createFunc != nil {
		return m.createFunc(ctx, risk)
	}
	risk.ID = uuid.New()
	return nil
}

func (m *MockRiskRepository) GetByID(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*domain.Risk, error) {
	if m.getByIDFunc != nil {
		return m.getByIDFunc(ctx, id, tenantID)
	}
	return nil, nil
}

func (m *MockRiskRepository) List(ctx context.Context, tenantID uuid.UUID, query domain.RiskQuery) (*domain.PaginatedResult[domain.Risk], error) {
	if m.listFunc != nil {
		return m.listFunc(ctx, tenantID, query)
	}
	return &domain.PaginatedResult[domain.Risk]{
		Data:       []domain.Risk{},
		Total:      0,
		Page:       1,
		Limit:      20,
		TotalPages: 0,
	}, nil
}

func (m *MockRiskRepository) Update(ctx context.Context, risk *domain.Risk) error {
	if m.updateFunc != nil {
		return m.updateFunc(ctx, risk)
	}
	return nil
}

func (m *MockRiskRepository) Delete(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, id, tenantID)
	}
	return nil
}

func (m *MockRiskRepository) Count(ctx context.Context, tenantID uuid.UUID) (int64, error) {
	if m.countFunc != nil {
		return m.countFunc(ctx, tenantID)
	}
	return 0, nil
}

func (m *MockRiskRepository) UpdateScore(ctx context.Context, riskID uuid.UUID, tenantID uuid.UUID, score float64, criticality string) error {
	if m.updateScoreFunc != nil {
		return m.updateScoreFunc(ctx, riskID, tenantID, score, criticality)
	}
	return nil
}

func (m *MockRiskRepository) GetRiskScore(ctx context.Context, riskID uuid.UUID, tenantID uuid.UUID) (float64, error) {
	if m.getRiskScoreFunc != nil {
		return m.getRiskScoreFunc(ctx, riskID, tenantID)
	}
	return 0, nil
}

func (m *MockRiskRepository) GetRisksByAssetID(ctx context.Context, assetID uuid.UUID, tenantID uuid.UUID) ([]domain.RiskForScoring, error) {
	if m.getRisksByAssetIDFunc != nil {
		return m.getRisksByAssetIDFunc(ctx, assetID, tenantID)
	}
	return []domain.RiskForScoring{}, nil
}

func (m *MockRiskRepository) GetHistory(ctx context.Context, riskID uuid.UUID, tenantID uuid.UUID, page, limit int) ([]domain.AuditLogEntry, error) {
	if m.getHistoryFunc != nil {
		return m.getHistoryFunc(ctx, riskID, tenantID, page, limit)
	}
	return []domain.AuditLogEntry{}, nil
}

func (m *MockRiskRepository) CreateAuditEntry(ctx context.Context, entry *domain.AuditLogEntry) error {
	if m.createAuditEntryFunc != nil {
		return m.createAuditEntryFunc(ctx, entry)
	}
	return nil
}

func (m *MockRiskRepository) GetBySource(ctx context.Context, tenantID uuid.UUID, source string) ([]domain.Risk, error) {
	if m.getBySourceFunc != nil {
		return m.getBySourceFunc(ctx, tenantID, source)
	}
	return []domain.Risk{}, nil
}

func (m *MockRiskRepository) GetByCVE(ctx context.Context, cveID string, tenantID uuid.UUID) (*domain.Risk, error) {
	if m.getByCVEFunc != nil {
		return m.getByCVEFunc(ctx, cveID, tenantID)
	}
	return nil, nil
}

func (m *MockRiskRepository) BulkUpdate(ctx context.Context, tenantID uuid.UUID, updates []domain.RiskUpdate) (int64, error) {
	if m.bulkUpdateFunc != nil {
		return m.bulkUpdateFunc(ctx, tenantID, updates)
	}
	return 0, nil
}

func (m *MockRiskRepository) BulkCreate(ctx context.Context, risks []*domain.Risk) (int64, error) {
	if m.bulkCreateFunc != nil {
		return m.bulkCreateFunc(ctx, risks)
	}
	return 0, nil
}

func (m *MockRiskRepository) BulkDelete(ctx context.Context, ids []uuid.UUID, tenantID uuid.UUID) (int64, error) {
	if m.bulkDeleteFunc != nil {
		return m.bulkDeleteFunc(ctx, ids, tenantID)
	}
	return 0, nil
}

// =============================================================================
// Test Cases
// =============================================================================

func TestCreateRiskUseCase_Success(t *testing.T) {
	mockRepo := &MockRiskRepository{}
	uc := NewCreateRiskUseCase(mockRepo)

	tenantID := uuid.New()
	input := CreateRiskInput{
		Title:       "Test Risk",
		Description: "Test Description",
		Impact:      0.8,
		Probability: 0.5,
		Status:      domain.RiskOpen,
		Tags:        []string{"network", "critical"},
		Frameworks:  []string{"iso27001", "nist"},
		Owner:       "test@example.com",
		Source:      "manual",
	}

	// Execute
	result, err := uc.Execute(context.Background(), tenantID, input)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, input.Title, result.Title)
	assert.Equal(t, input.Description, result.Description)
	assert.Equal(t, tenantID, result.TenantID)
	assert.Equal(t, domain.RiskOpen, result.Status)
}

func TestCreateRiskUseCase_MissingTitle(t *testing.T) {
	mockRepo := &MockRiskRepository{}
	uc := NewCreateRiskUseCase(mockRepo)

	tenantID := uuid.New()
	input := CreateRiskInput{
		Description: "Test Description",
		Impact:      0.8,
		Probability: 0.5,
		// Missing Title
	}

	result, err := uc.Execute(context.Background(), tenantID, input)

	require.Error(t, err)
	assert.Nil(t, result)
}

func TestCreateRiskUseCase_InvalidProbability(t *testing.T) {
	mockRepo := &MockRiskRepository{}
	uc := NewCreateRiskUseCase(mockRepo)

	tenantID := uuid.New()
	input := CreateRiskInput{
		Title:       "Test Risk",
		Probability: 1.5, // Out of range (>1.0)
		Impact:      0.8,
	}

	result, err := uc.Execute(context.Background(), tenantID, input)

	require.Error(t, err)
	assert.Nil(t, result)
}

func TestGetRiskUseCase_Success(t *testing.T) {
	tenantID := uuid.New()
	riskID := uuid.New()
	expectedRisk := &domain.Risk{
		ID:       riskID,
		TenantID: tenantID,
		Name:     "Test Risk",
		Status:   domain.RiskOpen,
	}

	mockRepo := &MockRiskRepository{
		getByIDFunc: func(ctx context.Context, id uuid.UUID, tid uuid.UUID) (*domain.Risk, error) {
			if id == riskID && tid == tenantID {
				return expectedRisk, nil
			}
			return nil, nil
		},
	}

	uc := NewGetRiskUseCase(mockRepo)
	result, err := uc.Execute(context.Background(), tenantID, riskID)

	require.NoError(t, err)
	assert.Equal(t, expectedRisk, result)
}

func TestGetRiskUseCase_NotFound(t *testing.T) {
	mockRepo := &MockRiskRepository{
		getByIDFunc: func(ctx context.Context, id uuid.UUID, tid uuid.UUID) (*domain.Risk, error) {
			return nil, nil
		},
	}

	uc := NewGetRiskUseCase(mockRepo)
	result, err := uc.Execute(context.Background(), uuid.New(), uuid.New())

	require.Error(t, err)
	assert.Nil(t, result)
}

func TestListRisksUseCase_Success(t *testing.T) {
	tenantID := uuid.New()
	risks := []domain.Risk{
		{
			ID:       uuid.New(),
			TenantID: tenantID,
			Name:     "Risk 1",
			Status:   domain.RiskOpen,
		},
		{
			ID:       uuid.New(),
			TenantID: tenantID,
			Name:     "Risk 2",
			Status:   domain.RiskMitigated,
		},
	}

	expectedResult := &domain.PaginatedResult[domain.Risk]{
		Data:       risks,
		Total:      2,
		Page:       1,
		Limit:      20,
		TotalPages: 1,
	}

	mockRepo := &MockRiskRepository{
		listFunc: func(ctx context.Context, tid uuid.UUID, query domain.RiskQuery) (*domain.PaginatedResult[domain.Risk], error) {
			return expectedResult, nil
		},
	}

	uc := NewListRisksUseCase(mockRepo)
	query := domain.NewRiskQuery()
	result, err := uc.Execute(context.Background(), tenantID, query)

	require.NoError(t, err)
	assert.Equal(t, expectedResult, result)
	assert.Len(t, result.Data, 2)
}

func TestDeleteRiskUseCase_Success(t *testing.T) {
	tenantID := uuid.New()
	riskID := uuid.New()

	mockRepo := &MockRiskRepository{
		getByIDFunc: func(ctx context.Context, id uuid.UUID, tid uuid.UUID) (*domain.Risk, error) {
			return &domain.Risk{ID: riskID, TenantID: tenantID}, nil
		},
		deleteFunc: func(ctx context.Context, id uuid.UUID, tid uuid.UUID) error {
			return nil
		},
	}

	uc := NewDeleteRiskUseCase(mockRepo)
	err := uc.Execute(context.Background(), tenantID, riskID)

	require.NoError(t, err)
}

// =============================================================================
// Benchmarks
// =============================================================================

func BenchmarkListRisks(b *testing.B) {
	mockRepo := &MockRiskRepository{
		listFunc: func(ctx context.Context, tid uuid.UUID, query domain.RiskQuery) (*domain.PaginatedResult[domain.Risk], error) {
			risks := make([]domain.Risk, 20)
			for i := range risks {
				risks[i] = domain.Risk{ID: uuid.New(), TenantID: tid}
			}
			return &domain.PaginatedResult[domain.Risk]{
				Data:       risks,
				Total:      1000,
				Page:       1,
				Limit:      20,
				TotalPages: 50,
			}, nil
		},
	}

	uc := NewListRisksUseCase(mockRepo)
	tenantID := uuid.New()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		query := domain.NewRiskQuery()
		uc.Execute(context.Background(), tenantID, query)
	}
}
