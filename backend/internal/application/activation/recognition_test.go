// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only

package activation

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/opendefender/openrisk/internal/domain"
)

// Criterion 9: the tenant configured before any of this shipped sees ITS OWN
// counts and is not walked through a tunnel that asks for its first risk.
func TestRecognition_Success(t *testing.T) {
	reader := newFakePostureReader()
	tenant, user := uuid.New(), uuid.New()
	reader.recog[tenant] = RecognitionCounts{Risks: 214, Frameworks: 2, Controls: 187, Assets: 43, Members: 9}

	got, err := NewRecognitionUseCase(reader).Execute(context.Background(), tenant, user)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got.Counts.Risks != 214 || got.Counts.Controls != 187 {
		t.Errorf("counts = %+v, want the tenant's own rows", got.Counts)
	}
	if !got.Recognised || !got.SkipTunnel {
		t.Errorf("a configured tenant must be recognised and skip the tunnel, got %+v", got)
	}
}

// A tenant that holds nothing is NOT an error — it is the brand-new case, and
// the honest answer is "take the tunnel".
func TestRecognition_BrandNewTenantIsNotAnError(t *testing.T) {
	reader := newFakePostureReader()
	tenant, user := uuid.New(), uuid.New()

	got, err := NewRecognitionUseCase(reader).Execute(context.Background(), tenant, user)
	if err != nil {
		t.Fatalf("a brand-new tenant must not error: %v", err)
	}
	if got.Recognised || got.SkipTunnel {
		t.Errorf("a brand-new tenant must not skip the tunnel, got %+v", got)
	}
}

// The founding member alone is not "a team". Counting them as pre-existing data
// would send every brand-new tenant to the recognition screen instead of the
// tunnel — the exact inversion of criterion 9.
func TestRecognition_FoundingMemberAloneIsNotRecognition(t *testing.T) {
	reader := newFakePostureReader()
	tenant, user := uuid.New(), uuid.New()
	reader.recog[tenant] = RecognitionCounts{Members: 1}

	got, err := NewRecognitionUseCase(reader).Execute(context.Background(), tenant, user)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got.Recognised {
		t.Error("one member and nothing else is a brand-new tenant, not a recognised one")
	}

	reader.recog[tenant] = RecognitionCounts{Members: 2}
	got, _ = NewRecognitionUseCase(reader).Execute(context.Background(), tenant, user)
	if !got.Recognised {
		t.Error("a second member is real pre-existing data")
	}
}

func TestRecognition_NotFound(t *testing.T) {
	// There is no NotFound on this path by design, and that is worth pinning:
	// an empty tenant is a legitimate answer, so a future edit that starts
	// returning ErrNotFound here would silently break the brand-new journey.
	reader := newFakePostureReader()
	tenant, user := uuid.New(), uuid.New()

	got, err := NewRecognitionUseCase(reader).Execute(context.Background(), tenant, user)
	if err != nil {
		t.Fatalf("an empty tenant must not be a 404: %v", err)
	}
	if got == nil {
		t.Fatal("an empty tenant must still get a payload")
	}

	// An unwired reader IS an error — a zeroed recognition would send a
	// configured bank into the tunnel.
	if _, err := NewRecognitionUseCase(nil).Execute(context.Background(), tenant, user); err == nil {
		t.Error("an unwired reader must error rather than report an empty tenant")
	}
}

func TestRecognition_Unauthorized(t *testing.T) {
	reader := newFakePostureReader()
	tenant, user := uuid.New(), uuid.New()
	uc := NewRecognitionUseCase(reader)

	for name, call := range map[string][2]uuid.UUID{
		"no tenant": {uuid.Nil, user},
		"no user":   {tenant, uuid.Nil},
		"neither":   {uuid.Nil, uuid.Nil},
	} {
		if _, err := uc.Execute(context.Background(), call[0], call[1]); err == nil || !errors.Is(err, domain.ErrForbidden) {
			t.Errorf("%s: error = %v, want ErrForbidden", name, err)
		}
	}
}

// Criterion 10 again, on this surface.
func TestRecognition_NeverReadsAnotherTenant(t *testing.T) {
	reader := newFakePostureReader()
	tenantA, tenantB, user := uuid.New(), uuid.New(), uuid.New()
	reader.recog[tenantB] = RecognitionCounts{Risks: 214, Frameworks: 2, Controls: 187}

	got, err := NewRecognitionUseCase(reader).Execute(context.Background(), tenantA, user)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got.Counts.Risks != 0 || got.Recognised {
		t.Errorf("tenant A saw tenant B's data: %+v", got.Counts)
	}
	for _, seen := range reader.seenTenants {
		if seen != tenantA {
			t.Errorf("the use case read tenant %v while serving tenant %v", seen, tenantA)
		}
	}
}

func TestRecognitionCounts_Any(t *testing.T) {
	cases := map[string]struct {
		counts RecognitionCounts
		want   bool
	}{
		"empty":       {RecognitionCounts{}, false},
		"one member":  {RecognitionCounts{Members: 1}, false},
		"two members": {RecognitionCounts{Members: 2}, true},
		"one risk":    {RecognitionCounts{Risks: 1}, true},
		"a framework": {RecognitionCounts{Frameworks: 1}, true},
		"controls":    {RecognitionCounts{Controls: 1}, true},
		"assets":      {RecognitionCounts{Assets: 1}, true},
	}
	for name, tc := range cases {
		if got := tc.counts.Any(); got != tc.want {
			t.Errorf("%s: Any() = %v, want %v", name, got, tc.want)
		}
	}
}
