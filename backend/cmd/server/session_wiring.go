// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package main

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	coreauth "github.com/opendefender/openrisk/internal/auth"
	"github.com/opendefender/openrisk/internal/domain"
)

// The session resolvers decide what a freshly minted token is allowed to carry.
//
// They are the single re-derivation point behind every path that issues a
// session without a password: refresh-token rotation, OAuth/SAML, the MFA
// challenge, organization switching, and the personal-access-token middleware.
// That makes them the one place worth enforcing account state, and the reason
// they live here as named functions rather than as closures inside main() —
// security logic that nothing can call in a test is security logic nobody
// checks (#350).

// sessionAccountReader is the narrow slice of the user repository the resolvers
// need. Declared as an interface so the resolvers can be driven in a test
// without a database.
type sessionAccountReader interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
	GetOrganizationMember(ctx context.Context, userID, orgID uuid.UUID) (*domain.OrganizationMember, error)
	GetUserDefaultOrganization(ctx context.Context, userID uuid.UUID) (*domain.Organization, error)
}

// resolveSessionClaimsForOrg re-derives a user's claims for a SPECIFIC
// organization, refusing when either the account or the membership is no longer
// live.
//
// Both halves matter and they fail independently:
//
//   - the ACCOUNT may be disabled or deleted while the membership row survives;
//   - the MEMBERSHIP may be revoked while the account stays perfectly healthy.
//
// Before #350 only the second was checked. `GetOrganizationMember` reads
// organization_members and never joins users, so nothing on this path consulted
// the user row at all: /auth/login refused a disabled account
// (application/auth/login.go, "account is disabled") and refresh did not, which
// left a deactivated account minting new access tokens from its existing refresh
// token until that token expired, and left its personal access tokens working
// indefinitely.
//
// Returning an error here is what kills the session: TokenManager.RefreshTokenPair
// revokes the whole refresh-token family when this resolver fails, so a
// deactivated account does not merely fail one refresh — it loses the lineage.
func resolveSessionClaimsForOrg(
	ctx context.Context,
	repo sessionAccountReader,
	uid, orgID uuid.UUID,
) (*coreauth.SessionClaims, error) {
	if orgID == uuid.Nil {
		return nil, fmt.Errorf("organization is required")
	}

	// Account first: a disabled account must be refused whether or not the
	// membership row survived, so the answer cannot depend on which of the two
	// was revoked.
	user, err := repo.GetByID(ctx, uid)
	if err != nil {
		return nil, err
	}
	// Deleted accounts resolve to nil — GORM's soft-delete scope excludes them
	// from this read — so "deleted" and "never existed" answer identically, which
	// is the correct answer to both.
	if user == nil {
		return nil, fmt.Errorf("account no longer exists")
	}
	if !user.IsActive {
		return nil, fmt.Errorf("account is disabled")
	}

	member, err := repo.GetOrganizationMember(ctx, uid, orgID)
	if err != nil {
		return nil, err
	}
	if member == nil {
		return nil, fmt.Errorf("user is not a member of this organization")
	}
	if !member.IsActive {
		return nil, fmt.Errorf("membership is not active")
	}

	sc := &coreauth.SessionClaims{
		TenantID: orgID,
		OrgRoles: map[uuid.UUID]string{orgID: string(member.Role)},
	}
	// EffectivePermissions unifies the admin wildcard, the business-role preset,
	// and any legacy profile rules — so a business-role user keeps its
	// permissions across a token refresh (same path as login).
	sc.Permissions = member.EffectivePermissions()
	return sc, nil
}

// resolveSessionClaims re-derives a user's claims for their DEFAULT
// organization. Shared by IssueSession (OAuth2/SAML/MFA) and the PAT
// middleware, which have no notion of a chosen org.
//
// The account check is inherited rather than repeated: this delegates to
// resolveSessionClaimsForOrg, so there is exactly one place where a disabled
// account is refused and no way for the two entry points to disagree.
func resolveSessionClaims(
	ctx context.Context,
	repo sessionAccountReader,
	uid uuid.UUID,
) (*coreauth.SessionClaims, error) {
	org, err := repo.GetUserDefaultOrganization(ctx, uid)
	if err != nil {
		return nil, err
	}
	if org == nil {
		return nil, fmt.Errorf("user has no organization")
	}
	return resolveSessionClaimsForOrg(ctx, repo, uid, org.ID)
}
