// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

import { create } from 'zustand';
import { api } from '../lib/api';
import { decodeAccessToken, permitted } from '../lib/jwt';
import { setAccessToken } from '../lib/session';
import { clearSessionScope } from '../lib/sessionScope';

interface User {
  id: string;
  email: string;
  username: string;
  full_name: string;
  role: string; // org role: root | admin | user
  role_level?: number; // role level for RBAC
  permissions?: string[]; // resolved permission strings from the JWT (["*"] for admin)
  org_roles?: Record<string, string>; // { orgId: roleName } from the JWT
  business_role?: string; // GRC job-role preset (rssi/dsi/…), "" for admin/root
  tenant_id?: string;
  org_name?: string; // display name of the user's organization (from login)
  bio?: string;
  phone?: string;
  department?: string;
  timezone?: string;
}

/**
 * What /auth/login answered.
 *
 * Login does not always produce a session. It stops short in two cases, and the
 * caller has to be able to tell them apart:
 *
 *   - `mfa_required` — an authenticator is enrolled; complete the challenge.
 *   - `mfa_enrollment_required` — the role mandates MFA, nothing is enrolled,
 *     AND the grace window has run out; enrol first, which completes the login
 *     in the same step.
 *
 * Both carry a short-lived, permission-less token that only the corresponding
 * endpoint accepts. Previously this store typed login as Promise<void> and read
 * `data.user` unconditionally, so an MFA-enabled account hit a TypeError and
 * could not sign in from the SPA at all.
 *
 * OR26-03: `mfa_enrollment_required` used to fire on the FIRST login of any
 * privileged account. It is now the end of the road rather than the start of
 * it — an account still inside its grace window signs in normally and reads a
 * banner instead. The client does not decide that; the server does, and the
 * request-time guard refuses everything if the client pretends otherwise.
 */
export type LoginOutcome =
  | { status: 'signed_in' }
  | { status: 'mfa_required'; mfa_token: string }
  | { status: 'mfa_enrollment_required'; mfa_token: string };

interface AuthStore {
  user: User | null;
  token: string | null;
  expiresIn: number | null;
  isAuthenticated: boolean;
  login: (email: string, password: string) => Promise<LoginOutcome>;
  /**
   * Adopts a session minted by a second-factor step.
   *
   * The MFA challenge and mandated enrolment both end by setting the same
   * session cookies /auth/login does and returning the pair in the body, so this
   * finishes the job login would otherwise have done.
   */
  adoptSession: (accessToken: string) => Promise<void>;
  logout: () => void;
  refreshToken: () => Promise<void>;
  hasPermission: (permission: string) => boolean;
  hasRole: (roleName: string) => boolean;
}

/** Merge the JWT-derived permissions/roles/tenant onto a user object. */
function withTokenClaims(user: User, token: string, businessRole?: string): User {
  const claims = decodeAccessToken(token);
  return {
    ...user,
    permissions: claims.permissions ?? user.permissions ?? [],
    org_roles: claims.org_roles ?? user.org_roles,
    tenant_id: claims.tenant_id ?? user.tenant_id,
    business_role: businessRole ?? user.business_role ?? '',
  };
}

export const useAuthStore = create<AuthStore>((set, get) => ({
  // The cached profile is a paint optimisation only — it holds no credential.
  // Authority for "is this session valid" belongs to the HttpOnly cookie, which
  // the server validates on the first request after a reload.
  user: JSON.parse(localStorage.getItem('auth_user') || 'null'),
  token: null,
  expiresIn: null,
  // Optimistic: with the token no longer readable, a cached profile is the only
  // client-side hint that a session cookie exists. A stale guess costs one 401,
  // which the api client already turns into a redirect to /login.
  isAuthenticated: !!localStorage.getItem('auth_user'),

  login: async (email, password) => {
    // Backend shape: { user: domain.User, token_pair: { access_token, refresh_token, expires_in }, organization }
    // domain.User.role is a nested Role object (or absent) — flatten to the role name this store expects.
    const { data } = await api.post('/auth/login', { email, password });

    // No session yet: a second factor stands between the password and the app.
    if (data.mfa_required && data.mfa_token) {
      return { status: 'mfa_required', mfa_token: data.mfa_token };
    }
    if (data.mfa_enrollment_required && data.mfa_token) {
      return { status: 'mfa_enrollment_required', mfa_token: data.mfa_token };
    }
    // Flatten the nested Role object and fold in the JWT permissions/roles + the
    // business_role the backend returns, so RBAC gating works on the client.
    const base: User = {
      ...data.user,
      role: data.user.role?.name ?? '',
      org_name: data.organization?.name,
    };
    const user = withTokenClaims(base, data.token_pair.access_token, data.business_role);

    // Drop anything the PREVIOUS identity left behind before adopting this one.
    // A session does not always end through the logout button — it can lapse,
    // be revoked, or die with a crash — so sign-in is the last chance to
    // guarantee that no cached response outlives the tenant it came from
    // (W0-05 / D9, see lib/sessionScope).
    clearSessionScope();

    // Tokens are NOT persisted: the durable credential is the HttpOnly cookie
    // the backend set on this response. Only the display profile is cached.
    setAccessToken(data.token_pair.access_token);
    localStorage.setItem('auth_user', JSON.stringify(user));

    set({
      token: data.token_pair.access_token,
      user,
      expiresIn: data.token_pair.expires_in,
      isAuthenticated: true,
    });

    return { status: 'signed_in' };
  },

  adoptSession: async (accessToken: string) => {
    // Same reason as login: this is the other door into a session.
    clearSessionScope();
    setAccessToken(accessToken);

    // The MFA responses carry the token pair but not the profile, so read it
    // back. Permissions still come from the token, exactly as at login.
    //
    // /auth/me answers { organization_id, user: {...} }, NOT a bare user — the
    // same envelope /auth/login uses. Spreading the envelope stored a profile
    // whose email, id and full_name were all undefined at the top level, which
    // survived into localStorage and back out on the next page load. It went
    // unnoticed because permissions come from the JWT and so kept working;
    // what broke was everything keyed on identity — the account menu's name,
    // and the members screen's "this row is you" guard, which then offered
    // controls the server refuses.
    const { data } = await api.get('/auth/me');
    const profile = (data?.user ?? data) as Record<string, unknown> & { role?: { name?: string } };
    const base: User = { ...profile, role: profile.role?.name ?? '' } as User;
    // business_role sits on the MEMBERSHIP, so it arrives beside the user rather
    // than inside it. Passing it through is what makes an MFA login land on the
    // right persona: without it the role resolved to "" on every MFA sign-in,
    // because the user object has no such field to fall back to (#338).
    const user = withTokenClaims(base, accessToken, data?.business_role);

    localStorage.setItem('auth_user', JSON.stringify(user));
    set({ token: accessToken, user, isAuthenticated: true });
  },

  logout: () => {
    // Ask the server to expire the cookies; script cannot clear HttpOnly ones.
    // Fire-and-forget: local state must be cleared even if the call fails, or a
    // network blip would leave the user apparently logged in.
    void api.post('/auth/logout', {}).catch(() => undefined);

    setAccessToken(null);
    // Drops the query cache, the tenant stores and every user-scoped storage
    // key — including the legacy pre-cookie token keys. Clearing only the auth
    // state left the next user on this browser looking at this tenant's data,
    // because logout and login are both soft navigations and the tab is never
    // torn down (W0-05 / D9, see lib/sessionScope).
    clearSessionScope();
    set({
      token: null,
      user: null,
      expiresIn: null,
      isAuthenticated: false,
    });
  },

  refreshToken: async () => {
    try {
      // No body: the refresh token travels in its HttpOnly cookie, which is
      // scoped to this endpoint.
      const { data } = await api.post('/auth/refresh', {});

      setAccessToken(data.token_pair.access_token);

      // Re-derive permissions from the rotated token (a business-role change or a
      // re-scoped session takes effect here, mirroring the backend SessionResolver).
      const current = get().user;
      const user = current ? withTokenClaims(current, data.token_pair.access_token) : current;
      if (user) localStorage.setItem('auth_user', JSON.stringify(user));

      set({
        token: data.token_pair.access_token,
        user,
        expiresIn: data.token_pair.expires_in,
      });
    } catch (err) {
      // Token refresh failed, logout user
      get().logout();
      throw err;
    }
  },

  hasPermission: (permission: string) => {
    const { user } = get();
    // Use the real permission set resolved from the JWT (["*"] for admin/root),
    // with the same wildcard semantics the backend enforces.
    return permitted(user?.permissions, permission);
  },

  hasRole: (roleName: string) => {
    const { user } = get();
    if (!user) return false;
    // Match either the org role (root/admin/user) or the business role preset.
    const target = roleName.toLowerCase();
    return (
      user.role.toLowerCase() === target || (user.business_role ?? '').toLowerCase() === target
    );
  },
}));
