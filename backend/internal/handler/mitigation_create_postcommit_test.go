// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/opendefender/openrisk/internal/infrastructure/database"
	"github.com/opendefender/openrisk/internal/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// newCreateMitigationApp wires POST /risks/:id/mitigations over an in-memory
// database holding only the two tables the write touches.
//
// withRisksTable is the whole point of the harness. GetByIDWithSubActions —
// the read-back the handler performs after the write has committed — issues a
// Preload("Risk"). With no `risks` table that read fails, which reproduces a
// post-commit read failure through the real code path rather than a mock.
func newCreateMitigationApp(t *testing.T, withRisksTable bool) (*fiber.App, *gorm.DB, uuid.UUID) {
	t.Helper()

	dsn := "file:mitigation_postcommit_" + uuid.New().String() + "?mode=memory&cache=private"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)

	require.NoError(t, db.Exec(`CREATE TABLE mitigations (
		id TEXT PRIMARY KEY, tenant_id TEXT, risk_id TEXT,
		title TEXT, description TEXT, status TEXT, priority TEXT,
		owner_id TEXT, assignee_id TEXT, reviewer_id TEXT,
		assigned_to TEXT, progress INTEGER DEFAULT 0,
		reminder_d7_sent_at DATETIME, reminder_d1_sent_at DATETIME,
		created_by TEXT, approved_by TEXT, approved_at DATETIME,
		source TEXT, auto_detected_at DATETIME, scanner_config_id TEXT,
		organization_id TEXT, assignee TEXT, cost INTEGER, mitigation_time INTEGER, due_date DATETIME,
		created_at DATETIME, updated_at DATETIME, deleted_at DATETIME
	);`).Error)

	require.NoError(t, db.Exec(`CREATE TABLE mitigation_subactions (
		id TEXT PRIMARY KEY, mitigation_id TEXT, title TEXT, description TEXT,
		completed BOOLEAN DEFAULT 0, completed_at DATETIME, completed_by TEXT,
		completed_source TEXT, auto_detected_at DATETIME, depends_on TEXT,
		"order" INTEGER DEFAULT 0, due_date DATETIME,
		created_at DATETIME, updated_at DATETIME, deleted_at DATETIME
	);`).Error)

	if withRisksTable {
		require.NoError(t, db.Exec(`CREATE TABLE risks (
			id TEXT PRIMARY KEY, tenant_id TEXT, title TEXT, description TEXT,
			status TEXT, score REAL, created_at DATETIME, updated_at DATETIME, deleted_at DATETIME
		);`).Error)
	}

	previous := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previous })

	tenantID := uuid.New()
	userID := uuid.New()

	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		middleware.SetContext(c, &middleware.RequestContext{UserID: userID, OrganizationID: tenantID})
		return c.Next()
	})
	app.Post("/api/v1/risks/:id/mitigations", CreateMitigation)

	return app, db, tenantID
}

func postMitigation(t *testing.T, app *fiber.App, riskID string) (int, map[string]interface{}) {
	t.Helper()

	body, err := json.Marshal(map[string]interface{}{
		"title":    "Patch the vulnerable dependency",
		"priority": "high",
		"sub_actions": []map[string]interface{}{
			{"title": "Inventory affected hosts"},
			{"title": "Apply the patch"},
			{"title": "Verify the fix"},
		},
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/risks/"+riskID+"/mitigations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	res, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()

	raw, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	decoded := map[string]interface{}{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &decoded)
	}
	return res.StatusCode, decoded
}

// Criterion 1 and 5 — the ordinary path still answers 201 with the canonical plan.
func TestCreateMitigation_Success_ReturnsCreatedPlan(t *testing.T) {
	app, db, _ := newCreateMitigationApp(t, true)

	status, body := postMitigation(t, app, uuid.New().String())

	require.Equal(t, http.StatusCreated, status)
	assert.NotEmpty(t, body["id"], "the response must carry the plan id")

	var plans, subs int64
	require.NoError(t, db.Table("mitigations").Count(&plans).Error)
	require.NoError(t, db.Table("mitigation_subactions").Count(&subs).Error)
	assert.Equal(t, int64(1), plans)
	assert.Equal(t, int64(3), subs)
}

// Criterion 4 — the write commits, the post-commit read-back fails, and the
// client is still told the truth: 201, with the plan it created.
//
// Before #335 this path returned 500 while the plan sat committed in the
// database, which is what drove users to retry and create duplicates.
func TestCreateMitigation_PostCommitReadFailure_StillReturns201(t *testing.T) {
	app, db, _ := newCreateMitigationApp(t, false) // no risks table -> Preload("Risk") fails

	status, body := postMitigation(t, app, uuid.New().String())

	assert.Equal(t, http.StatusCreated, status,
		"a committed write must never be reported to the client as a failure")
	assert.NotEqual(t, http.StatusInternalServerError, status)

	// Criterion 5 — the body identifies what was created, so a client that
	// retried can reconcile.
	assert.NotEmpty(t, body["id"], "the response must carry the plan id")

	// The write really did commit, whole.
	var plans, subs int64
	require.NoError(t, db.Table("mitigations").Count(&plans).Error)
	require.NoError(t, db.Table("mitigation_subactions").Count(&subs).Error)
	assert.Equal(t, int64(1), plans)
	assert.Equal(t, int64(3), subs, "the checklist committed with the plan")
}

// Criterion 6 — a request with no tenant context is refused before any write.
func TestCreateMitigation_Unauthorized_NoContext(t *testing.T) {
	_, db, _ := newCreateMitigationApp(t, true)

	app := fiber.New()
	app.Post("/api/v1/risks/:id/mitigations", CreateMitigation)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/risks/"+uuid.New().String()+"/mitigations",
		bytes.NewReader([]byte(`{"title":"x"}`)))
	req.Header.Set("Content-Type", "application/json")

	res, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, res.StatusCode)

	var plans int64
	require.NoError(t, db.Table("mitigations").Count(&plans).Error)
	assert.Equal(t, int64(0), plans, "an unauthorized request must write nothing")
}

// A malformed risk id is rejected before any write (NotFound-shaped input guard).
func TestCreateMitigation_InvalidRiskID_NoWrite(t *testing.T) {
	app, db, _ := newCreateMitigationApp(t, true)

	status, _ := postMitigation(t, app, "not-a-uuid")
	assert.Equal(t, http.StatusBadRequest, status)

	var plans int64
	require.NoError(t, db.Table("mitigations").Count(&plans).Error)
	assert.Equal(t, int64(0), plans)
}
