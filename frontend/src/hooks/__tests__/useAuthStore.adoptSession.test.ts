// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { vi, describe, it, expect, beforeEach, afterEach } from 'vitest';

import { useAuthStore } from '../useAuthStore';
import { api } from '../../lib/api';

// adoptSession is the door into a session for every path that does NOT go
// through /auth/login: the MFA challenge, mandated MFA enrolment, and anything
// else holding a fresh access token.
//
// It resolved the persona to "" on every one of them (#338). business_role lives
// on the membership, so it is not a field of the user object /auth/me returns —
// and adoptSession was calling withTokenClaims without the role argument, so the
// fallback chain `businessRole ?? user.business_role ?? ''` reached the end
// every time. Permissions come from the JWT and kept working, which is exactly
// why an RSSI signing in with MFA landed on the default dashboard and nobody
// read it as an authorization bug.

vi.mock('../../lib/sessionScope', () => ({ clearSessionScope: vi.fn() }));
vi.mock('../../lib/session', () => ({ setAccessToken: vi.fn() }));

// A syntactically valid RS256-shaped token whose payload carries permissions but
// NO business_role — the real shape, and the reason the field cannot come from
// the token.
function tokenWith(payload: Record<string, unknown>): string {
  const b64 = (o: unknown) =>
    btoa(JSON.stringify(o)).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
  return `${b64({ alg: 'RS256', typ: 'JWT' })}.${b64(payload)}.sig`;
}

const ACCESS_TOKEN = tokenWith({
  sub: 'user-1',
  permissions: ['risks:read', 'risks:create'],
  tenant_id: 'tenant-1',
});

beforeEach(() => {
  localStorage.clear();
  useAuthStore.setState({ user: null, token: null, isAuthenticated: false });
  vi.clearAllMocks();
});

afterEach(() => {
  vi.clearAllMocks();
});

describe('adoptSession — business_role (#338)', () => {
  it('takes the business role from /auth/me, beside the user rather than inside it', async () => {
    vi.spyOn(api, 'get').mockResolvedValueOnce({
      data: {
        organization_id: 'tenant-1',
        // Note: no business_role on the user. There is no such column.
        user: { id: 'user-1', email: 'rssi@example.com', full_name: 'R Ssi' },
        business_role: 'rssi',
      },
    });

    await useAuthStore.getState().adoptSession(ACCESS_TOKEN);

    expect(useAuthStore.getState().user?.business_role).toBe('rssi');
    // The permissions still come from the token, unchanged by this fix.
    expect(useAuthStore.getState().user?.permissions).toEqual(['risks:read', 'risks:create']);
    expect(useAuthStore.getState().isAuthenticated).toBe(true);
  });

  // The acceptance criterion: change the role, authenticate again, get the new
  // one. Nothing may be restored from the previous session's cached profile.
  it('a role changed server-side wins over the cached profile', async () => {
    localStorage.setItem(
      'auth_user',
      JSON.stringify({ id: 'user-1', email: 'rssi@example.com', business_role: 'auditor' }),
    );

    vi.spyOn(api, 'get').mockResolvedValueOnce({
      data: {
        organization_id: 'tenant-1',
        user: { id: 'user-1', email: 'rssi@example.com' },
        business_role: 'rssi',
      },
    });

    await useAuthStore.getState().adoptSession(ACCESS_TOKEN);

    expect(useAuthStore.getState().user?.business_role).toBe('rssi');
    // And the cache it writes back must carry the NEW role, or the next reload
    // reinstates the stale persona.
    expect(JSON.parse(localStorage.getItem('auth_user') ?? '{}').business_role).toBe('rssi');
  });

  // An admin/root legitimately has no preset. "" is an answer and must replace a
  // previously cached role rather than being treated as "no opinion".
  it('an explicit empty role replaces a cached one', async () => {
    localStorage.setItem(
      'auth_user',
      JSON.stringify({ id: 'user-1', email: 'a@example.com', business_role: 'rssi' }),
    );

    vi.spyOn(api, 'get').mockResolvedValueOnce({
      data: {
        organization_id: 'tenant-1',
        user: { id: 'user-1', email: 'a@example.com' },
        business_role: '',
      },
    });

    await useAuthStore.getState().adoptSession(ACCESS_TOKEN);

    expect(useAuthStore.getState().user?.business_role).toBe('');
  });

  // When the server could not resolve it, the field is absent. The session is
  // still adopted — a persona is a navigation hint, not a credential.
  it('survives /auth/me omitting the role', async () => {
    vi.spyOn(api, 'get').mockResolvedValueOnce({
      data: {
        organization_id: 'tenant-1',
        user: { id: 'user-1', email: 'a@example.com' },
      },
    });

    await useAuthStore.getState().adoptSession(ACCESS_TOKEN);

    expect(useAuthStore.getState().isAuthenticated).toBe(true);
    expect(useAuthStore.getState().user?.business_role).toBe('');
    expect(useAuthStore.getState().user?.permissions).toEqual(['risks:read', 'risks:create']);
  });

  // The identity regression this function already guards against: /auth/me
  // answers an envelope, not a bare user, and spreading the envelope stored a
  // profile whose id and email were undefined.
  it('reads the profile out of the envelope, not the envelope itself', async () => {
    vi.spyOn(api, 'get').mockResolvedValueOnce({
      data: {
        organization_id: 'tenant-1',
        user: { id: 'user-1', email: 'rssi@example.com', full_name: 'R Ssi' },
        business_role: 'dsi',
      },
    });

    await useAuthStore.getState().adoptSession(ACCESS_TOKEN);

    const user = useAuthStore.getState().user;
    expect(user?.id).toBe('user-1');
    expect(user?.email).toBe('rssi@example.com');
    expect(user?.business_role).toBe('dsi');
  });
});
