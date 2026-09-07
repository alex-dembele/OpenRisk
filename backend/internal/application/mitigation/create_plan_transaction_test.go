// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package mitigation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/opendefender/openrisk/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeMitigationRepo records how the use case reaches persistence. It exists to
// assert the SHAPE of the call — one transactional write carrying the plan and
// its whole checklist — which is the thing #335 changed. Whether that write is
// really atomic is proved against a real database in
// internal/infrastructure/repository/mitigation_repository_tx_test.go.
type fakeMitigationRepo struct {
	txCalls      int
	legacyCalls  int
	gotTenantID  string
	gotPlan      *domain.Mitigation
	gotSubAction []*domain.MitigationSubAction
	failWith     error
}

func (f *fakeMitigationRepo) CreateWithSubActions(tenantID string, m *domain.Mitigation, subs []*domain.MitigationSubAction) error {
	f.txCalls++
	f.gotTenantID = tenantID
	f.gotPlan = m
	f.gotSubAction = subs
	return f.failWith
}

// Create is the pre-#335 single-row path. The create use case must no longer
// reach it; the counter is what proves that.
func (f *fakeMitigationRepo) Create(string, *domain.Mitigation) error { f.legacyCalls++; return nil }

func (f *fakeMitigationRepo) GetByID(string, uuid.UUID) (*domain.Mitigation, error) {
	return nil, domain.ErrNotFound
}
func (f *fakeMitigationRepo) GetByIDWithSubActions(string, uuid.UUID) (*domain.Mitigation, error) {
	return nil, domain.ErrNotFound
}
func (f *fakeMitigationRepo) List(string, map[string]interface{}) ([]domain.Mitigation, error) {
	return nil, nil
}
func (f *fakeMitigationRepo) Update(string, *domain.Mitigation) error { return nil }
func (f *fakeMitigationRepo) Delete(string, uuid.UUID) error          { return nil }
func (f *fakeMitigationRepo) ListByRiskID(string, uuid.UUID) ([]domain.Mitigation, error) {
	return nil, nil
}
func (f *fakeMitigationRepo) RecalculateProgress(string, uuid.UUID) (int, error) { return 0, nil }

func planInput(tenantID uuid.UUID) CreateMitigationPlanInput {
	return CreateMitigationPlanInput{
		TenantID:  tenantID,
		RiskID:    uuid.New(),
		Title:     "Patch the vulnerable dependency",
		CreatedBy: uuid.New(),
		Source:    domain.SourceManual,
		SubActions: []struct {
			Title       string
			Description string
			DueDate     *time.Time
		}{
			{Title: "Inventory affected hosts"},
			{Title: "Apply the patch"},
		},
	}
}

func TestCreateMitigationPlan_Success_WritesPlanAndChecklistInOneCall(t *testing.T) {
	repo := &fakeMitigationRepo{}
	tenantID := uuid.New()

	out, err := NewCreateMitigationPlanUseCase(repo, nil).
		ExecuteContext(context.Background(), planInput(tenantID))

	require.NoError(t, err)
	require.NotNil(t, out)

	assert.Equal(t, 1, repo.txCalls, "exactly one transactional write")
	assert.Equal(t, 0, repo.legacyCalls, "the untransacted single-row path must not be used")
	assert.Equal(t, tenantID.String(), repo.gotTenantID)
	require.Len(t, repo.gotSubAction, 2, "the whole checklist goes in with the plan")

	// Ordering is preserved and every sub-action belongs to the plan.
	for i, sub := range repo.gotSubAction {
		assert.Equal(t, i, sub.Order)
		assert.Equal(t, repo.gotPlan.ID, sub.MitigationID)
	}

	// Criterion 5 — the caller gets the committed plan back, checklist included,
	// so the handler never has to re-read to answer.
	require.NotNil(t, out.Plan)
	assert.Equal(t, out.ID, out.Plan.ID)
	assert.Len(t, out.Plan.SubActions, 2)
}

func TestCreateMitigationPlan_PersistenceFailure_ReturnsErrorAndNoPlan(t *testing.T) {
	repo := &fakeMitigationRepo{failWith: errors.New("insert failed")}

	out, err := NewCreateMitigationPlanUseCase(repo, nil).
		ExecuteContext(context.Background(), planInput(uuid.New()))

	require.Error(t, err)
	assert.Nil(t, out, "a failed write must not hand back a plan the caller could report as created")
	assert.Equal(t, 1, repo.txCalls)
}

// _NotFound-shaped: the required references are missing, so nothing is written.
func TestCreateMitigationPlan_NotFound_MissingRiskOrTitleWritesNothing(t *testing.T) {
	cases := map[string]func(*CreateMitigationPlanInput){
		"no risk":  func(in *CreateMitigationPlanInput) { in.RiskID = uuid.Nil },
		"no title": func(in *CreateMitigationPlanInput) { in.Title = "" },
		"no actor": func(in *CreateMitigationPlanInput) { in.CreatedBy = uuid.Nil },
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			repo := &fakeMitigationRepo{}
			in := planInput(uuid.New())
			mutate(&in)

			_, err := NewCreateMitigationPlanUseCase(repo, nil).
				ExecuteContext(context.Background(), in)

			require.Error(t, err)
			assert.Equal(t, 0, repo.txCalls, "validation must reject before any write")
		})
	}
}

// _Unauthorized: no tenant on the request means no write, ever.
func TestCreateMitigationPlan_Unauthorized_NoTenantWritesNothing(t *testing.T) {
	repo := &fakeMitigationRepo{}
	in := planInput(uuid.New())
	in.TenantID = uuid.Nil

	_, err := NewCreateMitigationPlanUseCase(repo, nil).
		ExecuteContext(context.Background(), in)

	require.Error(t, err)
	assert.Equal(t, 0, repo.txCalls)
	assert.Equal(t, 0, repo.legacyCalls)
}
