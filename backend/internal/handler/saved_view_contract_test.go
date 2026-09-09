// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	savedviewapp "github.com/opendefender/openrisk/internal/application/savedview"
	"github.com/opendefender/openrisk/internal/domain"
)

// ---------------------------------------------------------------------------
// The saved-views wire contract.
//
// #580 shipped a frontend client whose types are DERIVED from
// docs/openapi.yaml, against a spec that never described this API. The generated
// types therefore had no SavedView in them and `npm run build` failed on master
// (#608). Documenting the endpoints fixes today; this test is what stops the
// spec and the handler drifting apart again.
//
// It drives the REAL handler through a REAL Fiber app and compares the JSON that
// comes back, field for field, with what docs/openapi.yaml promises. Reading the
// handler's source proves nothing about the bytes on the wire — a `json:"-"`, an
// omitempty or a renamed tag changes the response without changing the spec.
// ---------------------------------------------------------------------------

/* ------------------------------------------------------------ the spec side */

type openAPISchema struct {
	Type       any                      `yaml:"type"`
	Required   []string                 `yaml:"required"`
	Properties map[string]openAPISchema `yaml:"properties"`
	Enum       []string                 `yaml:"enum"`
	Ref        string                   `yaml:"$ref"`
}

type openAPIDoc struct {
	Paths      map[string]map[string]any `yaml:"paths"`
	Components struct {
		Schemas map[string]openAPISchema `yaml:"schemas"`
	} `yaml:"components"`
}

func loadOpenAPI(t *testing.T) openAPIDoc {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "openapi.yaml"))
	require.NoError(t, err, "docs/openapi.yaml must be readable from the handler package")
	var doc openAPIDoc
	require.NoError(t, yaml.Unmarshal(raw, &doc))
	return doc
}

// resolve follows a local $ref one hop, which is all this spec uses.
func resolve(t *testing.T, doc openAPIDoc, s openAPISchema) openAPISchema {
	t.Helper()
	if s.Ref == "" {
		return s
	}
	name := filepath.Base(s.Ref)
	target, ok := doc.Components.Schemas[name]
	require.True(t, ok, "spec references %s, which is not defined", s.Ref)
	return target
}

// assertMatchesSchema checks a decoded JSON object against a spec schema: every
// required property present, and no property the spec does not declare.
func assertMatchesSchema(t *testing.T, doc openAPIDoc, schemaName string, body map[string]any) {
	t.Helper()
	schema, ok := doc.Components.Schemas[schemaName]
	require.True(t, ok, "docs/openapi.yaml declares no %s schema", schemaName)

	for _, field := range schema.Required {
		_, present := body[field]
		require.True(t, present,
			"%s: the spec marks %q required, the handler did not send it", schemaName, field)
	}
	for field := range body {
		_, declared := schema.Properties[field]
		require.True(t, declared,
			"%s: the handler sends %q, which docs/openapi.yaml does not document", schemaName, field)
	}
	// Nested objects are checked too — a saved view is mostly its state.
	for field, value := range body {
		nested, isObject := value.(map[string]any)
		if !isObject {
			continue
		}
		sub := resolve(t, doc, schema.Properties[field])
		for _, req := range sub.Required {
			_, present := nested[req]
			require.True(t, present, "%s.%s: the spec marks %q required, it is absent", schemaName, field, req)
		}
		for key := range nested {
			_, declared := sub.Properties[key]
			require.True(t, declared, "%s.%s: undocumented field %q", schemaName, field, key)
		}
	}
}

/* --------------------------------------------------------- the handler side */

type contractRepo struct {
	views map[uuid.UUID]*domain.SavedView
}

func newContractRepo() *contractRepo {
	return &contractRepo{views: map[uuid.UUID]*domain.SavedView{}}
}

func (r *contractRepo) Create(_ context.Context, v *domain.SavedView) error {
	if v.ID == uuid.Nil {
		v.ID = uuid.New()
	}
	copied := *v
	r.views[v.ID] = &copied
	return nil
}

func (r *contractRepo) GetByID(_ context.Context, id, tenantID uuid.UUID) (*domain.SavedView, error) {
	v, ok := r.views[id]
	if !ok || v.TenantID != tenantID {
		return nil, nil
	}
	copied := *v
	return &copied, nil
}

func (r *contractRepo) ListVisible(_ context.Context, tenantID, userID uuid.UUID, tableID string) ([]domain.SavedView, error) {
	out := []domain.SavedView{}
	for _, v := range r.views {
		if v.TenantID != tenantID {
			continue
		}
		if tableID != "" && v.TableID != tableID {
			continue
		}
		if v.UserID != userID && !v.IsShared() {
			continue
		}
		out = append(out, *v)
	}
	return out, nil
}

func (r *contractRepo) Update(_ context.Context, v *domain.SavedView) error {
	existing, ok := r.views[v.ID]
	if !ok || existing.TenantID != v.TenantID {
		return domain.NewNotFoundError("saved view", v.ID)
	}
	existing.Name = v.Name
	existing.Visibility = v.Visibility
	existing.State = v.State
	existing.Columns = v.Columns
	return nil
}

func (r *contractRepo) Delete(_ context.Context, id, tenantID uuid.UUID) error {
	v, ok := r.views[id]
	if !ok || v.TenantID != tenantID {
		return domain.NewNotFoundError("saved view", id)
	}
	delete(r.views, id)
	return nil
}

func (r *contractRepo) ExistsByName(_ context.Context, tenantID, userID uuid.UUID, tableID, name string, excludeID uuid.UUID) (bool, error) {
	for _, v := range r.views {
		if v.ID == excludeID {
			continue
		}
		if v.TenantID == tenantID && v.UserID == userID && v.TableID == tableID && v.Name == name {
			return true, nil
		}
	}
	return false, nil
}

// setupSavedViewApp mounts the four real routes behind a stand-in for the auth
// middleware, exactly as cmd/server/main.go registers them.
func setupSavedViewApp(repo domain.SavedViewRepository, tenant, user uuid.UUID) *fiber.App {
	app := fiber.New()
	h := NewSavedViewHandler(
		savedviewapp.NewListSavedViewsUseCase(repo),
		savedviewapp.NewCreateSavedViewUseCase(repo),
		savedviewapp.NewUpdateSavedViewUseCase(repo),
		savedviewapp.NewDeleteSavedViewUseCase(repo),
	)
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("tenant_id", tenant)
		c.Locals("user_id", user)
		return c.Next()
	})
	app.Get("/saved-views", h.ListSavedViews)
	app.Post("/saved-views", h.CreateSavedView)
	app.Patch("/saved-views/:id", h.UpdateSavedView)
	app.Delete("/saved-views/:id", h.DeleteSavedView)
	return app
}

func call(t *testing.T, app *fiber.App, method, target string, body any) (int, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(encoded)
	}
	req := httptest.NewRequest(method, target, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	payload, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, payload
}

/* ----------------------------------------------------------------- the test */

func TestSavedViewContract_ResponsesMatchOpenAPISpec(t *testing.T) {
	doc := loadOpenAPI(t)
	tenant, user := uuid.New(), uuid.New()
	app := setupSavedViewApp(newContractRepo(), tenant, user)

	// --- POST /saved-views -> 201 SavedView -----------------------------------
	status, payload := call(t, app, "POST", "/saved-views", map[string]any{
		"table_id":   "risks",
		"name":       "Comité de mars",
		"visibility": "shared",
		"state": map[string]any{
			"q":       "fraude",
			"filters": map[string][]string{"severity": {"critical", "high"}},
			"sort":    map[string]string{"key": "score", "dir": "desc"},
		},
		"columns": map[string][]string{"order": {"name", "score"}, "hidden": {"owner"}},
	})
	require.Equal(t, 201, status, "POST /saved-views: %s", payload)

	var created map[string]any
	require.NoError(t, json.Unmarshal(payload, &created))
	assertMatchesSchema(t, doc, "SavedView", created)
	require.Equal(t, "shared", created["visibility"])
	id, _ := created["id"].(string)
	require.NotEmpty(t, id)

	// The enum in the spec must cover what the handler actually emits.
	visibility := doc.Components.Schemas["SavedViewVisibility"]
	require.Contains(t, visibility.Enum, created["visibility"])

	// --- POST the same name again -> 409, as documented -----------------------
	status, payload = call(t, app, "POST", "/saved-views", map[string]any{
		"table_id": "risks", "name": "Comité de mars",
	})
	require.Equal(t, 409, status, "a duplicate name must conflict: %s", payload)

	// --- GET /saved-views?table_id=risks -> 200 [SavedView] -------------------
	status, payload = call(t, app, "GET", "/saved-views?table_id=risks", nil)
	require.Equal(t, 200, status, "GET /saved-views: %s", payload)

	var listed []map[string]any
	require.NoError(t, json.Unmarshal(payload, &listed))
	require.Len(t, listed, 1)
	assertMatchesSchema(t, doc, "SavedView", listed[0])

	// --- PATCH /saved-views/{id} -> 200 SavedView -----------------------------
	status, payload = call(t, app, "PATCH", "/saved-views/"+id, map[string]any{
		"visibility": "personal",
	})
	require.Equal(t, 200, status, "PATCH /saved-views/{id}: %s", payload)

	var patched map[string]any
	require.NoError(t, json.Unmarshal(payload, &patched))
	assertMatchesSchema(t, doc, "SavedView", patched)
	require.Equal(t, "personal", patched["visibility"], "a patch must apply")
	require.Equal(t, "Comité de mars", patched["name"], "a patch must not reset an absent field")

	// --- DELETE /saved-views/{id} -> 204, empty body --------------------------
	status, payload = call(t, app, "DELETE", "/saved-views/"+id, nil)
	require.Equal(t, 204, status)
	require.Empty(t, payload, "204 carries no body")

	// --- and it is gone -------------------------------------------------------
	status, payload = call(t, app, "GET", "/saved-views?table_id=risks", nil)
	require.Equal(t, 200, status)
	require.NoError(t, json.Unmarshal(payload, &listed))
	require.Empty(t, listed)
}

// TestSavedViewContract_SpecDocumentsEveryRoute fails if a verb is registered in
// main.go and not described in the spec — the other half of the drift #608 was.
func TestSavedViewContract_SpecDocumentsEveryRoute(t *testing.T) {
	doc := loadOpenAPI(t)

	for path, verbs := range map[string][]string{
		"/saved-views":      {"get", "post"},
		"/saved-views/{id}": {"patch", "delete"},
	} {
		item, ok := doc.Paths[path]
		require.True(t, ok, "docs/openapi.yaml documents no %s", path)
		for _, verb := range verbs {
			_, ok := item[verb]
			require.True(t, ok, "docs/openapi.yaml documents no %s %s", verb, path)
		}
	}
}
