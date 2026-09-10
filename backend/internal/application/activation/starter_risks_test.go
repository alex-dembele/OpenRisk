// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only

package activation

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/opendefender/openrisk/internal/domain"
	"github.com/opendefender/openrisk/pkg/onboarding"
)

// ---------------------------------------------------------------------------
// Writer double, keyed BY TENANT.
//
// A double that ignored the tenant argument could not fail the isolation test,
// and a test that cannot fail proves nothing (#438 criterion 10).
// ---------------------------------------------------------------------------

type fakeStarterWriter struct {
	written map[uuid.UUID][]domain.StarterRiskDraft
	failOn  int // 1-based index of the write that fails; 0 never fails
	failHas bool
	calls   int
}

func newFakeStarterWriter() *fakeStarterWriter {
	return &fakeStarterWriter{written: map[uuid.UUID][]domain.StarterRiskDraft{}}
}

func (f *fakeStarterWriter) HasStarterRisks(_ context.Context, tenantID uuid.UUID) (bool, error) {
	if f.failHas {
		return false, errors.New("risk store down")
	}
	return len(f.written[tenantID]) > 0, nil
}

func (f *fakeStarterWriter) CreateStarterRisk(_ context.Context, tenantID uuid.UUID, draft domain.StarterRiskDraft) (*domain.Risk, error) {
	f.calls++
	if f.failOn == f.calls {
		return nil, errors.New("risk store down")
	}
	f.written[tenantID] = append(f.written[tenantID], draft)
	return &domain.Risk{ID: uuid.New(), TenantID: tenantID, Title: draft.Title}, nil
}

func threeKeys(t *testing.T) []string {
	t.Helper()
	offered := onboarding.StarterRisksFor("banking", "CM")
	if len(offered) < onboarding.StarterRiskPickCount {
		t.Fatalf("the catalogue offered %d statements", len(offered))
	}
	keys := make([]string, 0, onboarding.StarterRiskPickCount)
	for _, r := range offered[:onboarding.StarterRiskPickCount] {
		keys = append(keys, r.Key)
	}
	return keys
}

func seedAnswers(repo *fakeRepo, tenantID, userID uuid.UUID, industry, country string) {
	repo.progress[userID.String()] = &domain.OnboardingProgress{
		TenantID: tenantID,
		UserID:   userID,
		Industry: industry,
		Country:  country,
	}
}

// ---------------------------------------------------------------------------
// RULE #4 — the trio
// ---------------------------------------------------------------------------

func TestAdoptStarterRisks_Success(t *testing.T) {
	repo, writer := newFakeRepo(), newFakeStarterWriter()
	tenant, user := uuid.New(), uuid.New()
	seedAnswers(repo, tenant, user, "banking", "CM")
	keys := threeKeys(t)

	got, err := NewStarterRisksUseCase(repo, writer).Adopt(context.Background(), tenant, user, keys)
	if err != nil {
		t.Fatalf("Adopt: %v", err)
	}
	if len(got.Created) != onboarding.StarterRiskPickCount {
		t.Errorf("created %d risks, want %d", len(got.Created), onboarding.StarterRiskPickCount)
	}
	if len(writer.written[tenant]) != onboarding.StarterRiskPickCount {
		t.Fatalf("wrote %d rows", len(writer.written[tenant]))
	}

	// THE property that matters: the content is the catalogue's, not the
	// caller's. Nothing in the request carried a title.
	for i, draft := range writer.written[tenant] {
		statement, ok := onboarding.StarterRiskByKey(keys[i])
		if !ok {
			t.Fatalf("key %q vanished from the catalogue", keys[i])
		}
		if draft.Title != statement.Title("fr") {
			t.Errorf("title = %q, want the catalogue's %q", draft.Title, statement.Title("fr"))
		}
		if draft.Description != statement.Description("fr") {
			t.Errorf("description for %q did not come from the catalogue", keys[i])
		}
		if draft.Probability != statement.Probability || draft.Impact != statement.Impact {
			t.Errorf("scores for %q did not come from the catalogue", keys[i])
		}
		if draft.StarterKey != statement.Key {
			t.Errorf("provenance key = %q, want %q", draft.StarterKey, statement.Key)
		}
		if draft.CreatedBy != user {
			t.Errorf("createdBy = %v, want the acting user", draft.CreatedBy)
		}
	}
}

// "NotFound" on this path is an unknown catalogue key, and it is a REFUSAL, not
// a skip: silently writing two rows when three were asked for leaves the user
// counting their own register to find out what happened.
func TestAdoptStarterRisks_NotFound(t *testing.T) {
	repo, writer := newFakeRepo(), newFakeStarterWriter()
	tenant, user := uuid.New(), uuid.New()
	seedAnswers(repo, tenant, user, "banking", "CM")
	keys := threeKeys(t)
	keys[2] = "starter_does_not_exist"

	_, err := NewStarterRisksUseCase(repo, writer).Adopt(context.Background(), tenant, user, keys)
	if err == nil || !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
	if !strings.Contains(err.Error(), "starter_does_not_exist") {
		t.Errorf("the error must name the offending key, got %q", err.Error())
	}
	if len(writer.written[tenant]) != 0 {
		t.Errorf("a rejected selection wrote %d rows; it must write none", len(writer.written[tenant]))
	}
}

func TestAdoptStarterRisks_Unauthorized(t *testing.T) {
	repo, writer := newFakeRepo(), newFakeStarterWriter()
	tenant, user := uuid.New(), uuid.New()
	uc := NewStarterRisksUseCase(repo, writer)
	keys := threeKeys(t)

	for name, call := range map[string][2]uuid.UUID{
		"no tenant": {uuid.Nil, user},
		"no user":   {tenant, uuid.Nil},
		"neither":   {uuid.Nil, uuid.Nil},
	} {
		if _, err := uc.Adopt(context.Background(), call[0], call[1], keys); err == nil || !errors.Is(err, domain.ErrForbidden) {
			t.Errorf("Adopt %s: error = %v, want ErrForbidden", name, err)
		}
		if _, err := uc.List(context.Background(), call[0], call[1]); err == nil || !errors.Is(err, domain.ErrForbidden) {
			t.Errorf("List %s: error = %v, want ErrForbidden", name, err)
		}
	}
	if len(writer.written) != 0 {
		t.Error("an unauthorised call wrote rows")
	}
}

// ---------------------------------------------------------------------------
// The properties that keep a customer's register clean
// ---------------------------------------------------------------------------

// The tunnel is resumable and back-navigable BY DESIGN, so a user will return to
// step 2. Without this guard every return visit doubles the register.
func TestAdoptStarterRisks_IsIdempotentPerTenant(t *testing.T) {
	repo, writer := newFakeRepo(), newFakeStarterWriter()
	tenant, user := uuid.New(), uuid.New()
	seedAnswers(repo, tenant, user, "banking", "CM")
	uc := NewStarterRisksUseCase(repo, writer)
	keys := threeKeys(t)

	if _, err := uc.Adopt(context.Background(), tenant, user, keys); err != nil {
		t.Fatalf("first adopt: %v", err)
	}

	_, err := uc.Adopt(context.Background(), tenant, user, keys)
	if err == nil || !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("second adopt: error = %v, want ErrConflict", err)
	}
	if len(writer.written[tenant]) != onboarding.StarterRiskPickCount {
		t.Errorf("the register holds %d starter rows, want %d",
			len(writer.written[tenant]), onboarding.StarterRiskPickCount)
	}
}

// Criterion 10 on the write path, which is where it matters most: these rows
// land in a customer's register.
func TestAdoptStarterRisks_NeverWritesToAnotherTenant(t *testing.T) {
	repo, writer := newFakeRepo(), newFakeStarterWriter()
	tenantA, tenantB, user := uuid.New(), uuid.New(), uuid.New()
	seedAnswers(repo, tenantA, user, "banking", "CM")

	if _, err := NewStarterRisksUseCase(repo, writer).Adopt(context.Background(), tenantA, user, threeKeys(t)); err != nil {
		t.Fatalf("Adopt: %v", err)
	}
	if len(writer.written[tenantB]) != 0 {
		t.Errorf("tenant B received %d rows from tenant A's adoption", len(writer.written[tenantB]))
	}

	// And tenant B's own adoption is not blocked by tenant A's.
	seedAnswers(repo, tenantB, user, "health", "FR")
	if _, err := NewStarterRisksUseCase(repo, writer).Adopt(context.Background(), tenantB, user, threeKeys(t)); err != nil {
		t.Errorf("tenant B must be able to adopt independently: %v", err)
	}
}

// Exactly three. A client that could adopt eight would fill a register the
// customer never reviewed; one that adopted two would under-deliver the reveal.
func TestAdoptStarterRisks_RequiresExactlyThree(t *testing.T) {
	repo, writer := newFakeRepo(), newFakeStarterWriter()
	tenant, user := uuid.New(), uuid.New()
	seedAnswers(repo, tenant, user, "banking", "CM")
	uc := NewStarterRisksUseCase(repo, writer)

	all := onboarding.StarterRisksFor("banking", "CM")
	eight := make([]string, 0, len(all))
	for _, r := range all {
		eight = append(eight, r.Key)
	}

	for name, keys := range map[string][]string{
		"none":  {},
		"one":   eight[:1],
		"two":   eight[:2],
		"four":  eight[:4],
		"eight": eight,
	} {
		if _, err := uc.Adopt(context.Background(), tenant, user, keys); err == nil || !errors.Is(err, domain.ErrValidation) {
			t.Errorf("%s: error = %v, want ErrValidation", name, err)
		}
	}

	// A duplicate is three keys but not three statements.
	dup := []string{eight[0], eight[0], eight[1]}
	if _, err := uc.Adopt(context.Background(), tenant, user, dup); err == nil || !errors.Is(err, domain.ErrValidation) {
		t.Errorf("a repeated key must be rejected, got %v", err)
	}
	if len(writer.written[tenant]) != 0 {
		t.Error("a rejected selection wrote rows")
	}
}

// A store failure part-way through must be reported, not swallowed. The rows
// already written stay — they are real risks the user chose.
func TestAdoptStarterRisks_SurfacesAPartialFailure(t *testing.T) {
	repo, writer := newFakeRepo(), newFakeStarterWriter()
	writer.failOn = 2
	tenant, user := uuid.New(), uuid.New()
	seedAnswers(repo, tenant, user, "banking", "CM")

	got, err := NewStarterRisksUseCase(repo, writer).Adopt(context.Background(), tenant, user, threeKeys(t))
	if err == nil {
		t.Fatal("a failing writer must surface")
	}
	if got == nil || len(got.Created) != 1 {
		t.Errorf("the result must report what WAS written, got %+v", got)
	}
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

func TestListStarterRisks_FollowsTheStoredAnswers(t *testing.T) {
	repo, writer := newFakeRepo(), newFakeStarterWriter()
	tenant, user := uuid.New(), uuid.New()
	seedAnswers(repo, tenant, user, "banking", "CM")

	offer, err := NewStarterRisksUseCase(repo, writer).List(context.Background(), tenant, user)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(offer.Risks) != onboarding.EightStarterRisks {
		t.Errorf("offered %d statements, want %d", len(offer.Risks), onboarding.EightStarterRisks)
	}
	if offer.Pick != onboarding.StarterRiskPickCount {
		t.Errorf("pick = %d, want %d", offer.Pick, onboarding.StarterRiskPickCount)
	}
	if offer.Industry != "banking" || offer.Country != "CM" {
		t.Errorf("the offer must echo the stored answers, got %q/%q", offer.Industry, offer.Country)
	}
	if offer.AlreadyAdopted {
		t.Error("a tenant that has adopted nothing must not be reported as adopted")
	}

	// A different sector produces a different set — otherwise the scoping is
	// decoration.
	seedAnswers(repo, tenant, user, "health", "FR")
	other, _ := NewStarterRisksUseCase(repo, writer).List(context.Background(), tenant, user)
	if other.Risks[0].Key == offer.Risks[0].Key {
		t.Error("changing the sector did not change the offered set")
	}
}

// Step 2 has no empty state that makes sense: "pick three of nothing" is broken.
func TestListStarterRisks_NeverEmptyEvenWithNoAnswers(t *testing.T) {
	repo, writer := newFakeRepo(), newFakeStarterWriter()
	offer, err := NewStarterRisksUseCase(repo, writer).List(context.Background(), uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(offer.Risks) != onboarding.EightStarterRisks {
		t.Errorf("a user with no stored answers got %d statements", len(offer.Risks))
	}
}

func TestListStarterRisks_ReportsAnExistingAdoption(t *testing.T) {
	repo, writer := newFakeRepo(), newFakeStarterWriter()
	tenant, user := uuid.New(), uuid.New()
	seedAnswers(repo, tenant, user, "banking", "CM")
	uc := NewStarterRisksUseCase(repo, writer)

	if _, err := uc.Adopt(context.Background(), tenant, user, threeKeys(t)); err != nil {
		t.Fatalf("Adopt: %v", err)
	}
	offer, err := uc.List(context.Background(), tenant, user)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if !offer.AlreadyAdopted {
		t.Error("a tenant with starter rows must be told so, or the screen invites a refused adoption")
	}
	// A failing read must not hide the statements.
	writer.failHas = true
	offer, err = uc.List(context.Background(), tenant, user)
	if err != nil || len(offer.Risks) != onboarding.EightStarterRisks {
		t.Errorf("a failing adoption check must not empty the screen: %v / %d statements", err, len(offer.Risks))
	}
}
