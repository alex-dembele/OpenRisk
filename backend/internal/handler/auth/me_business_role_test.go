// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package auth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/opendefender/openrisk/internal/domain"
	"github.com/opendefender/openrisk/internal/middleware"
)

// /auth/me is the one place every authenticated path reads its profile back:
// password login, MFA challenge, mandated enrolment, session restore. Before
// #338 it did not report business_role at all, so any path that went through it
// resolved the persona to "" — an MFA user landed on the default dashboard while
// their token carried an RSSI's permissions.
//
// business_role lives on the MEMBERSHIP, not on domain.User, which is why it has
// to be resolved here rather than serialised with the user.

type stubUserReader struct {
	user *domain.User
	err  error
}

func (s stubUserReader) GetByID(context.Context, uuid.UUID) (*domain.User, error) {
	return s.user, s.err
}

type stubMemberReader struct {
	member *domain.OrganizationMember
	err    error
	calls  int
	gotUID uuid.UUID
	gotOrg uuid.UUID
}

func (s *stubMemberReader) GetOrganizationMember(_ context.Context, userID, orgID uuid.UUID) (*domain.OrganizationMember, error) {
	s.calls++
	s.gotUID, s.gotOrg = userID, orgID
	return s.member, s.err
}

func meApp(t *testing.T, h *Handler, userID, orgID uuid.UUID) *fiber.App {
	t.Helper()
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		middleware.SetContext(c, &middleware.RequestContext{UserID: userID, OrganizationID: orgID})
		return c.Next()
	})
	app.Get("/auth/me", h.Me)
	return app
}

func callMe(t *testing.T, app *fiber.App) (int, map[string]any) {
	t.Helper()
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/auth/me", nil))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	body := map[string]any{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &body)
	}
	return resp.StatusCode, body
}

func handlerWith(user *domain.User, member *stubMemberReader) *Handler {
	h := NewHandler(nil, nil, nil, nil, nil, nil).WithUserLookup(stubUserReader{user: user})
	if member != nil {
		h = h.WithMemberLookup(member)
	}
	return h
}

func TestMe_ReportsTheCurrentBusinessRole(t *testing.T) {
	userID, orgID := uuid.New(), uuid.New()
	member := &stubMemberReader{member: &domain.OrganizationMember{
		UserID: userID, OrganizationID: orgID, IsActive: true, BusinessRole: "rssi",
	}}

	status, body := callMe(t, meApp(t, handlerWith(&domain.User{ID: userID}, member), userID, orgID))

	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, "rssi", body["business_role"])

	// Resolved for the CALLER and the ACTIVE organization, not from anything the
	// client could supply.
	assert.Equal(t, 1, member.calls)
	assert.Equal(t, userID, member.gotUID)
	assert.Equal(t, orgID, member.gotOrg)
}

// The role changed server-side; the next /auth/me must report the new one. This
// is the acceptance criterion: a role change followed by re-authentication
// produces the current role, because nothing is cached between the two.
func TestMe_ReflectsARoleChangeOnTheNextCall(t *testing.T) {
	userID, orgID := uuid.New(), uuid.New()
	member := &stubMemberReader{member: &domain.OrganizationMember{
		UserID: userID, OrganizationID: orgID, IsActive: true, BusinessRole: "auditor",
	}}
	app := meApp(t, handlerWith(&domain.User{ID: userID}, member), userID, orgID)

	_, first := callMe(t, app)
	require.Equal(t, "auditor", first["business_role"])

	member.member.BusinessRole = "rssi"

	_, second := callMe(t, app)
	assert.Equal(t, "rssi", second["business_role"],
		"the role must be re-read per call, never served from a cached profile")
	assert.Equal(t, 2, member.calls)
}

// An admin or root carries no job-role preset. "" is a REAL answer here and must
// be present, so the client stops showing whatever persona it had before.
func TestMe_AnEmptyRoleIsAnAnswer_NotAnOmission(t *testing.T) {
	userID, orgID := uuid.New(), uuid.New()
	member := &stubMemberReader{member: &domain.OrganizationMember{
		UserID: userID, OrganizationID: orgID, IsActive: true, BusinessRole: "",
	}}

	_, body := callMe(t, meApp(t, handlerWith(&domain.User{ID: userID}, member), userID, orgID))

	role, present := body["business_role"]
	assert.True(t, present, "an admin's empty role must be reported, not omitted")
	assert.Equal(t, "", role)
}

// The mirror: when the role cannot be RESOLVED, the field is omitted rather than
// defaulted to "". A present "" asserts "this user is an admin"; an absent key
// says "not known" and leaves the client on what it had.
func TestMe_OmitsTheRoleWhenItCannotBeResolved(t *testing.T) {
	userID, orgID := uuid.New(), uuid.New()

	cases := map[string]*stubMemberReader{
		"lookup fails":       {err: errors.New("database down")},
		"no membership":      {member: nil},
		"membership revoked": {member: &domain.OrganizationMember{UserID: userID, OrganizationID: orgID, IsActive: false, BusinessRole: "rssi"}},
	}

	for name, member := range cases {
		t.Run(name, func(t *testing.T) {
			status, body := callMe(t, meApp(t, handlerWith(&domain.User{ID: userID}, member), userID, orgID))

			require.Equal(t, http.StatusOK, status, "an unresolvable role must not fail the call")
			_, present := body["business_role"]
			assert.False(t, present, "the field must be absent, not an empty string")
			assert.NotNil(t, body["user"], "the rest of the profile still answers")
		})
	}
}

// A deployment that wired no member lookup must not fabricate a persona.
func TestMe_OmitsTheRoleWhenNoLookupIsWired(t *testing.T) {
	userID, orgID := uuid.New(), uuid.New()

	_, body := callMe(t, meApp(t, handlerWith(&domain.User{ID: userID}, nil), userID, orgID))

	_, present := body["business_role"]
	assert.False(t, present)
}

// Without a tenant there is no membership to resolve, and the lookup must not
// even be attempted with a nil organization.
func TestMe_Unauthorized_NoTenantResolvesNoRole(t *testing.T) {
	userID := uuid.New()
	member := &stubMemberReader{member: &domain.OrganizationMember{IsActive: true, BusinessRole: "rssi"}}

	_, body := callMe(t, meApp(t, handlerWith(&domain.User{ID: userID}, member), userID, uuid.Nil))

	_, present := body["business_role"]
	assert.False(t, present)
	assert.Equal(t, 0, member.calls, "a nil organization must not reach the repository")
}

// No authenticated caller at all: 401 before anything is read.
func TestMe_Unauthorized_NoContext(t *testing.T) {
	member := &stubMemberReader{}
	app := fiber.New()
	app.Get("/auth/me", handlerWith(&domain.User{ID: uuid.New()}, member).Me)

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/auth/me", nil))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	assert.Equal(t, 0, member.calls)
}
