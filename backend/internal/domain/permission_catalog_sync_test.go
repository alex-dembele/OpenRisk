// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package domain

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"
)

// ---------------------------------------------------------------------------
// PermissionCatalog says of itself: "Keep it in sync with the
// RequirePermission(...) call sites in main.go." Nothing checked that, and it
// drifted: `events:read` guarded /api/v1/realtime/events while being absent from
// the catalog, which made ValidateBusinessRoles reject ALL ELEVEN business role
// presets and left the RBAC matrix unable to display a permission the API was
// really enforcing (#616).
//
// This closes the direction that hurts: a route may not gate on a permission the
// catalog does not know.
//
// The reverse is deliberately NOT asserted. Several catalog keys are gate-able
// without being gated by RequirePermission today — the organization profile and
// the audit trail are guarded by the admin ROLE, and the strings exist so a
// scoped business role CAN be granted them. Requiring a call site for every key
// would forbid that, which the catalog's own comments explicitly want to allow.
// ---------------------------------------------------------------------------

var requirePermissionCall = regexp.MustCompile(`RequirePermission\("([^"]+)"\)`)

func TestPermissionCatalog_CoversEveryGuardedPermission(t *testing.T) {
	main := filepath.Join("..", "..", "cmd", "server", "main.go")
	src, err := os.ReadFile(main)
	if err != nil {
		t.Fatalf("cmd/server/main.go must be readable from the domain package: %v", err)
	}

	matches := requirePermissionCall.FindAllStringSubmatch(string(src), -1)
	if len(matches) == 0 {
		t.Fatal("no RequirePermission(...) call found — this test has stopped testing anything")
	}

	missing := map[string]bool{}
	for _, m := range matches {
		key := m[1]
		if key == PermissionAll { // the admin wildcard is not a catalog entry
			continue
		}
		if !IsCatalogPermission(key) {
			missing[key] = true
		}
	}

	if len(missing) == 0 {
		return
	}
	keys := make([]string, 0, len(missing))
	for k := range missing {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	t.Errorf(`these permissions guard a route but are absent from PermissionCatalog: %v

A route gating on a key the catalog does not know has two consequences:
  * ValidateBusinessRoles() rejects every preset that grants it, and
  * the RBAC matrix cannot show it, so an administrator cannot see or grant a
    permission the API is really enforcing.

Add each one to PermissionCatalog with its group and its bilingual labels.`, keys)
}
