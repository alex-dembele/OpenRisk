// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package main

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/opendefender/openrisk/internal/domain"
)

// #350: the session resolvers are the single re-derivation point behind every
// path that mints a session without a password — refresh rotation, OAuth/SAML,
// the MFA challenge, org switching, and the PAT middleware. They validated the
// MEMBERSHIP and never the ACCOUNT, so a deactivated user kept minting access
// tokens from an existing refresh token until it expired.
//
// The issue names five states — blocked, suspended, disabled, deleted, revoked.
// domain.User has exactly two signals to express them, and these tests are
// written against what exists rather than against invented states:
//
//	IsActive == false   → disabled / blocked / suspended
//	row absent          → deleted (GORM's soft-delete scope hides it from GetByID)
//
// "revoked" is the membership's own IsActive, which was already enforced and is
// pinned here so the new account check cannot displace it.

type fakeAccountRepo struct {
	user       *domain.User
	userErr    error
	member     *domain.OrganizationMember
	memberErr  error
	defaultOrg *domain.Organization
	orgErr     error

	userCalls   int
	memberCalls int
}

func (f *fakeAccountRepo) GetByID(context.Context, uuid.UUID) (*domain.User, error) {
	f.userCalls++
	return f.user, f.userErr
}

func (f *fakeAccountRepo) GetOrganizationMember(context.Context, uuid.UUID, uuid.UUID) (*domain.OrganizationMember, error) {
	f.memberCalls++
	return f.member, f.memberErr
}

func (f *fakeAccountRepo) GetUserDefaultOrganization(context.Context, uuid.UUID) (*domain.Organization, error) {
	return f.defaultOrg, f.orgErr
}

func liveUser() *domain.User {
	return &domain.User{ID: uuid.New(), IsActive: true}
}

func liveMember(orgID uuid.UUID) *domain.OrganizationMember {
	return &domain.OrganizationMember{OrganizationID: orgID, IsActive: true, Role: domain.RoleAdmin}
}

func TestResolveSessionClaimsForOrg_Success(t *testing.T) {
	orgID := uuid.New()
	repo := &fakeAccountRepo{user: liveUser(), member: liveMember(orgID)}

	sc, err := resolveSessionClaimsForOrg(context.Background(), repo, uuid.New(), orgID)

	require.NoError(t, err)
	require.NotNil(t, sc)
	assert.Equal(t, orgID, sc.TenantID)
	assert.Equal(t, string(domain.RoleAdmin), sc.OrgRoles[orgID])
	assert.NotEmpty(t, sc.Permissions, "an active admin resolves permissions")
}

// The issue's list, mapped onto the states that exist. Every one must refuse to
// produce claims, and must refuse BEFORE the membership is consulted — a
// disabled account is disabled whether or not its membership row survived.
func TestResolveSessionClaimsForOrg_RefusesDeadAccounts(t *testing.T) {
	orgID := uuid.New()

	cases := map[string]struct {
		repo *fakeAccountRepo
		want string
	}{
		"disabled / blocked / suspended": {
			repo: &fakeAccountRepo{
				user:   &domain.User{ID: uuid.New(), IsActive: false},
				member: liveMember(orgID),
			},
			want: "account is disabled",
		},
		"deleted — soft-delete hides the row": {
			repo: &fakeAccountRepo{user: nil, member: liveMember(orgID)},
			want: "account no longer exists",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			sc, err := resolveSessionClaimsForOrg(context.Background(), tc.repo, uuid.New(), orgID)

			require.Error(t, err)
			assert.Nil(t, sc, "a refused account must not carry claims")
			assert.Contains(t, err.Error(), tc.want)
			assert.Equal(t, 0, tc.repo.memberCalls,
				"the account is checked first; a live membership must not rescue a dead account")
		})
	}
}

// "revoked" in the issue's list is the membership's own state. It was already
// enforced; this pins it so the new account check cannot displace it.
func TestResolveSessionClaimsForOrg_RefusesRevokedMembership(t *testing.T) {
	orgID := uuid.New()

	cases := map[string]struct {
		member *domain.OrganizationMember
		want   string
	}{
		"membership revoked": {
			member: &domain.OrganizationMember{OrganizationID: orgID, IsActive: false},
			want:   "membership is not active",
		},
		"never a member": {
			member: nil,
			want:   "user is not a member of this organization",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			repo := &fakeAccountRepo{user: liveUser(), member: tc.member}

			sc, err := resolveSessionClaimsForOrg(context.Background(), repo, uuid.New(), orgID)

			require.Error(t, err)
			assert.Nil(t, sc)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

// A read failure must not be mistaken for a healthy account.
func TestResolveSessionClaimsForOrg_PropagatesLookupErrors(t *testing.T) {
	orgID := uuid.New()
	boom := errors.New("database unavailable")

	t.Run("account lookup fails", func(t *testing.T) {
		repo := &fakeAccountRepo{userErr: boom, member: liveMember(orgID)}
		sc, err := resolveSessionClaimsForOrg(context.Background(), repo, uuid.New(), orgID)
		require.ErrorIs(t, err, boom)
		assert.Nil(t, sc)
	})

	t.Run("membership lookup fails", func(t *testing.T) {
		repo := &fakeAccountRepo{user: liveUser(), memberErr: boom}
		sc, err := resolveSessionClaimsForOrg(context.Background(), repo, uuid.New(), orgID)
		require.ErrorIs(t, err, boom)
		assert.Nil(t, sc)
	})
}

func TestResolveSessionClaimsForOrg_RequiresAnOrganization(t *testing.T) {
	repo := &fakeAccountRepo{user: liveUser()}

	sc, err := resolveSessionClaimsForOrg(context.Background(), repo, uuid.New(), uuid.Nil)

	require.Error(t, err)
	assert.Nil(t, sc)
	assert.Contains(t, err.Error(), "organization is required")
	assert.Equal(t, 0, repo.userCalls, "a nil org is rejected before any read")
}

// The default-org entry point — OAuth/SAML, the MFA challenge and the PAT
// middleware all arrive here — must inherit the account check, not skip it.
func TestResolveSessionClaims_DefaultOrg_InheritsTheAccountCheck(t *testing.T) {
	orgID := uuid.New()
	org := &domain.Organization{ID: orgID}

	t.Run("live account resolves", func(t *testing.T) {
		repo := &fakeAccountRepo{user: liveUser(), member: liveMember(orgID), defaultOrg: org}
		sc, err := resolveSessionClaims(context.Background(), repo, uuid.New())
		require.NoError(t, err)
		assert.Equal(t, orgID, sc.TenantID)
	})

	t.Run("disabled account is refused here too", func(t *testing.T) {
		repo := &fakeAccountRepo{
			user:       &domain.User{ID: uuid.New(), IsActive: false},
			member:     liveMember(orgID),
			defaultOrg: org,
		}
		sc, err := resolveSessionClaims(context.Background(), repo, uuid.New())
		require.Error(t, err)
		assert.Nil(t, sc)
		assert.Contains(t, err.Error(), "account is disabled")
	})

	t.Run("deleted account is refused here too", func(t *testing.T) {
		repo := &fakeAccountRepo{user: nil, member: liveMember(orgID), defaultOrg: org}
		sc, err := resolveSessionClaims(context.Background(), repo, uuid.New())
		require.Error(t, err)
		assert.Nil(t, sc)
		assert.Contains(t, err.Error(), "account no longer exists")
	})

	t.Run("no default organization", func(t *testing.T) {
		repo := &fakeAccountRepo{user: liveUser(), defaultOrg: nil}
		sc, err := resolveSessionClaims(context.Background(), repo, uuid.New())
		require.Error(t, err)
		assert.Nil(t, sc)
		assert.Contains(t, err.Error(), "user has no organization")
	})
}
