// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	coreauth "github.com/opendefender/openrisk/internal/auth"
	"github.com/opendefender/openrisk/internal/domain"
	authpkg "github.com/opendefender/openrisk/pkg/auth"
)

// The acceptance criterion of #350 stated end to end: a blocked or revoked
// account cannot get a fresh access token out of refresh-token rotation.
//
// The unit tests beside this one prove the resolver refuses. This one proves the
// REAL resolver, wired into a REAL TokenManager exactly as main() wires it,
// turns that refusal into a refused refresh — and into a revoked family, so the
// session dies rather than merely stalling for one request.

func newRefreshHarness(t *testing.T, repo sessionAccountReader) (*coreauth.TokenManager, *gorm.DB) {
	t.Helper()

	dsn := "file:session_refresh_" + uuid.New().String() + "?mode=memory&cache=private"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	require.NoError(t, db.Exec(`
		CREATE TABLE refresh_tokens (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			tenant_id TEXT NOT NULL,
			family_id TEXT NOT NULL,
			token_hash TEXT NOT NULL UNIQUE,
			device_fingerprint TEXT,
			ip_address TEXT,
			user_agent TEXT,
			expires_at DATETIME NOT NULL,
			rotated_at DATETIME,
			last_used_at DATETIME,
			created_at DATETIME,
			updated_at DATETIME
		)`).Error)

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	tm := coreauth.NewTokenManager(db, &authpkg.RSAKeys{PrivateKey: priv, PublicKey: &priv.PublicKey})

	// Wired exactly as main() wires it — the production resolvers, not a spy.
	tm.SetOrgSessionResolver(func(ctx context.Context, uid, orgID uuid.UUID) (*coreauth.SessionClaims, error) {
		return resolveSessionClaimsForOrg(ctx, repo, uid, orgID)
	})
	tm.SetSessionResolver(func(ctx context.Context, uid uuid.UUID) (*coreauth.SessionClaims, error) {
		return resolveSessionClaims(ctx, repo, uid)
	})

	return tm, db
}

func countRefreshTokens(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Table("refresh_tokens").Where("rotated_at IS NULL").Count(&n).Error)
	return n
}

// The control: a live account refreshes normally. Without this, the refusal
// tests below would pass against a resolver that refused everything.
func TestRefresh_LiveAccount_Succeeds(t *testing.T) {
	orgID := uuid.New()
	repo := &fakeAccountRepo{user: liveUser(), member: liveMember(orgID), defaultOrg: &domain.Organization{ID: orgID}}
	tm, db := newRefreshHarness(t, repo)
	ctx := context.Background()

	pair, err := tm.GenerateTokenPair(ctx, uuid.New(), orgID, nil, []string{"*"}, nil, coreauth.DeviceContext{})
	require.NoError(t, err)

	next, err := tm.RefreshTokenPair(ctx, pair.RefreshToken, coreauth.DeviceContext{})
	require.NoError(t, err)
	require.NotNil(t, next)
	assert.NotEmpty(t, next.AccessToken)
	assert.Equal(t, int64(1), countRefreshTokens(t, db), "the family lives on, rotated into a new token")
}

// The issue's headline: blocked / suspended / disabled, and deleted. Each must
// fail the refresh AND take the family with it.
func TestRefresh_DeadAccount_RefusedAndFamilyRevoked(t *testing.T) {
	orgID := uuid.New()

	cases := map[string]func(*fakeAccountRepo){
		"account disabled after the session was issued": func(r *fakeAccountRepo) {
			r.user = &domain.User{ID: uuid.New(), IsActive: false}
		},
		"account deleted after the session was issued": func(r *fakeAccountRepo) {
			r.user = nil
		},
		"membership revoked after the session was issued": func(r *fakeAccountRepo) {
			r.member = &domain.OrganizationMember{OrganizationID: orgID, IsActive: false}
		},
	}

	for name, kill := range cases {
		t.Run(name, func(t *testing.T) {
			repo := &fakeAccountRepo{user: liveUser(), member: liveMember(orgID)}
			tm, db := newRefreshHarness(t, repo)
			ctx := context.Background()

			// A session that was perfectly legitimate when it was minted.
			pair, err := tm.GenerateTokenPair(ctx, uuid.New(), orgID, nil, []string{"*"}, nil, coreauth.DeviceContext{})
			require.NoError(t, err)
			require.Equal(t, int64(1), countRefreshTokens(t, db))

			// The account or the membership dies.
			kill(repo)

			next, err := tm.RefreshTokenPair(ctx, pair.RefreshToken, coreauth.DeviceContext{})

			require.Error(t, err, "a dead account must not receive a fresh access token")
			assert.Nil(t, next, "no token pair may be handed back")
			assert.Equal(t, int64(0), countRefreshTokens(t, db),
				"the family is revoked, so the session dies rather than surviving to the next attempt")
		})
	}
}

// IssueSession is the path OAuth, SAML and the MFA challenge take. It resolves
// through the same code, so a disabled account must be refused there too — this
// is the half the issue's "password flow / MFA flow" bullets are about.
func TestIssueSession_DeadAccount_Refused(t *testing.T) {
	orgID := uuid.New()
	org := &domain.Organization{ID: orgID}

	t.Run("live account is issued a session", func(t *testing.T) {
		repo := &fakeAccountRepo{user: liveUser(), member: liveMember(orgID), defaultOrg: org}
		tm, _ := newRefreshHarness(t, repo)

		pair, err := tm.IssueSession(context.Background(), uuid.New(), coreauth.DeviceContext{})
		require.NoError(t, err)
		assert.NotEmpty(t, pair.AccessToken)
	})

	t.Run("disabled account is refused", func(t *testing.T) {
		repo := &fakeAccountRepo{
			user:       &domain.User{ID: uuid.New(), IsActive: false},
			member:     liveMember(orgID),
			defaultOrg: org,
		}
		tm, _ := newRefreshHarness(t, repo)

		pair, err := tm.IssueSession(context.Background(), uuid.New(), coreauth.DeviceContext{})
		require.Error(t, err)
		assert.Nil(t, pair)
		assert.Contains(t, err.Error(), "account is disabled")
	})

	t.Run("deleted account is refused", func(t *testing.T) {
		repo := &fakeAccountRepo{user: nil, member: liveMember(orgID), defaultOrg: org}
		tm, _ := newRefreshHarness(t, repo)

		pair, err := tm.IssueSession(context.Background(), uuid.New(), coreauth.DeviceContext{})
		require.Error(t, err)
		assert.Nil(t, pair)
		assert.Contains(t, err.Error(), "account no longer exists")
	})
}
