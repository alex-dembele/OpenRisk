// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only

package activation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/opendefender/openrisk/internal/domain"
	"github.com/opendefender/openrisk/pkg/monitoring"
	"github.com/opendefender/openrisk/pkg/scoring"
)

// ---------------------------------------------------------------------------
// Tenant-aware reader double.
//
// Keyed BY TENANT on purpose: a double that ignored the tenant argument could
// not fail the isolation test, and a test that cannot fail proves nothing
// (#438 criterion 10).
// ---------------------------------------------------------------------------

type fakePostureReader struct {
	counts   map[uuid.UUID]PostureRiskCounts
	controls map[uuid.UUID]PostureControlCounts
	top      map[uuid.UUID][]PostureRisk
	credits  map[uuid.UUID]map[uuid.UUID][]scoring.ControlCredit
	recog    map[uuid.UUID]RecognitionCounts

	fail bool
	// seenTenants records every tenant the use case actually asked about.
	seenTenants []uuid.UUID
}

func newFakePostureReader() *fakePostureReader {
	return &fakePostureReader{
		counts:   map[uuid.UUID]PostureRiskCounts{},
		controls: map[uuid.UUID]PostureControlCounts{},
		top:      map[uuid.UUID][]PostureRisk{},
		credits:  map[uuid.UUID]map[uuid.UUID][]scoring.ControlCredit{},
		recog:    map[uuid.UUID]RecognitionCounts{},
	}
}

func (f *fakePostureReader) RiskCounts(_ context.Context, tenantID uuid.UUID) (PostureRiskCounts, error) {
	f.seenTenants = append(f.seenTenants, tenantID)
	if f.fail {
		return PostureRiskCounts{}, errors.New("risk store down")
	}
	return f.counts[tenantID], nil
}

func (f *fakePostureReader) ControlCounts(_ context.Context, tenantID uuid.UUID) (PostureControlCounts, error) {
	f.seenTenants = append(f.seenTenants, tenantID)
	if f.fail {
		return PostureControlCounts{}, errors.New("compliance store down")
	}
	return f.controls[tenantID], nil
}

func (f *fakePostureReader) TopRisks(_ context.Context, tenantID uuid.UUID, limit int) ([]PostureRisk, error) {
	f.seenTenants = append(f.seenTenants, tenantID)
	if f.fail {
		return nil, errors.New("risk store down")
	}
	rs := f.top[tenantID]
	if len(rs) > limit {
		rs = rs[:limit]
	}
	return rs, nil
}

func (f *fakePostureReader) ControlCreditsByRisk(_ context.Context, tenantID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID][]scoring.ControlCredit, error) {
	f.seenTenants = append(f.seenTenants, tenantID)
	if f.fail {
		return nil, errors.New("mapping store down")
	}
	out := map[uuid.UUID][]scoring.ControlCredit{}
	for _, id := range ids {
		if c, ok := f.credits[tenantID][id]; ok {
			out[id] = c
		}
	}
	return out, nil
}

func (f *fakePostureReader) RecognitionCounts(_ context.Context, tenantID uuid.UUID) (RecognitionCounts, error) {
	f.seenTenants = append(f.seenTenants, tenantID)
	if f.fail {
		return RecognitionCounts{}, errors.New("store down")
	}
	return f.recog[tenantID], nil
}

// seedRevealable gives a tenant everything the reveal needs: risks, applicable
// controls and one mapped control.
func seedRevealable(r *fakePostureReader, tenant uuid.UUID) uuid.UUID {
	riskID := uuid.New()
	r.counts[tenant] = PostureRiskCounts{Total: 12, ByLevel: map[string]int{"critical": 2, "high": 4, "medium": 5, "low": 1}}
	r.controls[tenant] = PostureControlCounts{Frameworks: 1, Total: 20, Implemented: 8, NotApplicable: 4, InProgress: 3}
	r.top[tenant] = []PostureRisk{{ID: riskID, Title: "Compromission d'un compte à privilèges", Inherent: 16, Level: "critical"}}
	r.credits[tenant] = map[uuid.UUID][]scoring.ControlCredit{
		riskID: {{Status: scoring.ControlImplemented, HasEvidence: true}, {Status: scoring.ControlInProgress}},
	}
	return riskID
}

func newPosture(reader PostureReader, repo *fakeRepo) *PostureUseCase {
	return NewPostureUseCase(reader, repo, NewRecorder(repo))
}

// ---------------------------------------------------------------------------
// RULE #4 — the trio
// ---------------------------------------------------------------------------

func TestPostureSummary_Success(t *testing.T) {
	reader := newFakePostureReader()
	repo := newFakeRepo()
	tenant, user := uuid.New(), uuid.New()
	seedRevealable(reader, tenant)

	got, err := newPosture(reader, repo).Execute(context.Background(), tenant, user)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if got.Risks.Total != 12 {
		t.Errorf("risk count = %d, want 12", got.Risks.Total)
	}
	// 8 implemented over 16 applicable (20 − 4 not applicable) = 50%.
	if got.CoveragePercent == nil || *got.CoveragePercent != 50 {
		t.Errorf("coverage = %v, want 50", got.CoveragePercent)
	}
	if len(got.TopRisks) != 1 {
		t.Fatalf("top risks = %d, want 1", len(got.TopRisks))
	}
	// ADR 0003, checkable by hand: (1.0 + 0.30)/2 = 0.65 · 0.80 × 0.65 = 0.52
	// 16 × (1 − 0.52) = 7.68
	if v := got.TopRisks[0].Residual.Value; v != 7.68 {
		t.Errorf("residual = %v, want 7.68", v)
	}
	if got.ResidualFormulaVersion != scoring.ResidualFormulaVersion {
		t.Errorf("formula version = %q", got.ResidualFormulaVersion)
	}
	if !got.FirstReveal || got.RevealedAt == nil {
		t.Errorf("the first reveal must be flagged and dated, got first=%v at=%v", got.FirstReveal, got.RevealedAt)
	}
	if has, _ := repo.HasEvent(context.Background(), tenant, domain.ActivationPostureRevealed); !has {
		t.Error("posture.revealed must be recorded on a successful reveal")
	}
}

// Criterion 8: a payload that would be empty renders an error state, records
// NOTHING and observes NOTHING.
func TestPostureSummary_NotFound(t *testing.T) {
	cases := map[string]func(*fakePostureReader, uuid.UUID){
		"no risks at all": func(r *fakePostureReader, tenant uuid.UUID) {
			r.controls[tenant] = PostureControlCounts{Total: 10, Implemented: 5}
		},
		"risks but no applicable control": func(r *fakePostureReader, tenant uuid.UUID) {
			r.counts[tenant] = PostureRiskCounts{Total: 4}
			r.controls[tenant] = PostureControlCounts{Total: 3, NotApplicable: 3}
			r.top[tenant] = []PostureRisk{{ID: uuid.New(), Title: "x", Inherent: 9}}
		},
		"counted risks but none readable": func(r *fakePostureReader, tenant uuid.UUID) {
			r.counts[tenant] = PostureRiskCounts{Total: 4}
			r.controls[tenant] = PostureControlCounts{Total: 10, Implemented: 5}
		},
	}

	for name, seed := range cases {
		reader := newFakePostureReader()
		repo := newFakeRepo()
		tenant, user := uuid.New(), uuid.New()
		seed(reader, tenant)

		countBefore := testutil.ToFloat64(monitoring.AhaReachedTotal)
		v2Before := ahaObservations(t, monitoring.AhaDefinitionV2)

		got, err := newPosture(reader, repo).Execute(context.Background(), tenant, user)
		if err == nil {
			t.Errorf("%s: an unrevealable posture must error, got %+v", name, got)
			continue
		}
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("%s: error = %v, want ErrNotFound", name, err)
		}
		if has, _ := repo.HasEvent(context.Background(), tenant, domain.ActivationPostureRevealed); has {
			t.Errorf("%s: posture.revealed was recorded for an empty reveal", name)
		}
		if got := testutil.ToFloat64(monitoring.AhaReachedTotal) - countBefore; got != 0 {
			t.Errorf("%s: aha_reached_total moved by %v on an empty reveal", name, got)
		}
		if got := ahaObservations(t, monitoring.AhaDefinitionV2) - v2Before; got != 0 {
			t.Errorf("%s: v2 histogram observed %d times on an empty reveal", name, got)
		}
	}
}

func TestPostureSummary_Unauthorized(t *testing.T) {
	reader := newFakePostureReader()
	repo := newFakeRepo()
	tenant, user := uuid.New(), uuid.New()
	seedRevealable(reader, tenant)
	uc := newPosture(reader, repo)

	for name, call := range map[string][2]uuid.UUID{
		"no tenant": {uuid.Nil, user},
		"no user":   {tenant, uuid.Nil},
		"neither":   {uuid.Nil, uuid.Nil},
	} {
		_, err := uc.Execute(context.Background(), call[0], call[1])
		if err == nil || !errors.Is(err, domain.ErrForbidden) {
			t.Errorf("%s: error = %v, want ErrForbidden", name, err)
		}
	}
}

// ---------------------------------------------------------------------------
// Criteria 10 and 11
// ---------------------------------------------------------------------------

// Criterion 10: every read is scoped to the caller's tenant. Tenant B holds a
// full posture; tenant A holds nothing and must see nothing of B's.
func TestPostureSummary_NeverReadsAnotherTenant(t *testing.T) {
	reader := newFakePostureReader()
	repo := newFakeRepo()
	tenantA, tenantB, user := uuid.New(), uuid.New(), uuid.New()
	seedRevealable(reader, tenantB)

	_, err := newPosture(reader, repo).Execute(context.Background(), tenantA, user)
	if err == nil {
		t.Fatal("tenant A holds nothing; it must not be able to reveal tenant B's posture")
	}
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}

	for _, seen := range reader.seenTenants {
		if seen != tenantA {
			t.Errorf("the use case read tenant %v while serving tenant %v", seen, tenantA)
		}
	}
	if has, _ := repo.HasEvent(context.Background(), tenantB, domain.ActivationPostureRevealed); has {
		t.Error("tenant A's request recorded a reveal against tenant B")
	}
}

// Criterion 11: rendering twice creates no duplicate row and does not move
// completed_at.
func TestPostureSummary_RevealIsIdempotent(t *testing.T) {
	reader := newFakePostureReader()
	repo := newFakeRepo()
	tenant, user := uuid.New(), uuid.New()
	seedRevealable(reader, tenant)
	uc := newPosture(reader, repo)

	first, err := uc.Execute(context.Background(), tenant, user)
	if err != nil {
		t.Fatalf("first reveal: %v", err)
	}
	second, err := uc.Execute(context.Background(), tenant, user)
	if err != nil {
		t.Fatalf("second reveal: %v", err)
	}

	rows := 0
	for _, e := range repo.events {
		if e.TenantID == tenant && e.EventKey == domain.ActivationPostureRevealed {
			rows++
		}
	}
	if rows != 1 {
		t.Errorf("posture.revealed recorded %d times, want exactly 1", rows)
	}
	if second.FirstReveal {
		t.Error("the second render must not be flagged as the first reveal")
	}
	if first.RevealedAt == nil || second.RevealedAt == nil || !first.RevealedAt.Equal(*second.RevealedAt) {
		t.Errorf("revealed_at moved: %v then %v", first.RevealedAt, second.RevealedAt)
	}
}

// D-010: the reveal is the v2 definition. It must never touch v1, whose series
// is frozen at the executive-dashboard definition.
func TestPostureSummary_ObservesV2AndNeverV1(t *testing.T) {
	reader := newFakePostureReader()
	repo := newFakeRepo()
	tenant, user := uuid.New(), uuid.New()
	seedRevealable(reader, tenant)
	repo.events = append(repo.events, domain.ActivationEvent{
		TenantID:   tenant,
		EventKey:   domain.ActivationSignup,
		OccurredAt: time.Now().UTC().Add(-5 * time.Minute),
	})

	v1Before := ahaObservations(t, monitoring.AhaDefinitionV1)
	v2Before := ahaObservations(t, monitoring.AhaDefinitionV2)
	countBefore := testutil.ToFloat64(monitoring.AhaReachedTotal)

	if _, err := newPosture(reader, repo).Execute(context.Background(), tenant, user); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if got := ahaObservations(t, monitoring.AhaDefinitionV2) - v2Before; got != 1 {
		t.Errorf("v2 gained %d observations, want 1", got)
	}
	if got := ahaObservations(t, monitoring.AhaDefinitionV1) - v1Before; got != 0 {
		t.Errorf("the reveal leaked %d observations into the frozen v1 series", got)
	}
	if got := testutil.ToFloat64(monitoring.AhaReachedTotal) - countBefore; got != 1 {
		t.Errorf("aha_reached_total moved by %v, want 1", got)
	}
}

// The tenant with no signup anchor is still counted — the D-010 defect again,
// this time on the reveal's own path.
func TestPostureSummary_CountsATenantWithNoSignupAnchor(t *testing.T) {
	reader := newFakePostureReader()
	repo := newFakeRepo()
	tenant, user := uuid.New(), uuid.New()
	seedRevealable(reader, tenant)

	countBefore := testutil.ToFloat64(monitoring.AhaReachedTotal)
	v2Before := ahaObservations(t, monitoring.AhaDefinitionV2)

	if _, err := newPosture(reader, repo).Execute(context.Background(), tenant, user); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if got := testutil.ToFloat64(monitoring.AhaReachedTotal) - countBefore; got != 1 {
		t.Errorf("aha_reached_total moved by %v, want 1", got)
	}
	if got := ahaObservations(t, monitoring.AhaDefinitionV2) - v2Before; got != 0 {
		t.Errorf("a duration was observed with no signup anchor: +%d", got)
	}
}

// A dead store must surface, not degrade into a plausible-looking zero posture:
// that is the failure that would make the launch-gate metric lie.
func TestPostureSummary_PropagatesReaderFailure(t *testing.T) {
	reader := newFakePostureReader()
	reader.fail = true
	repo := newFakeRepo()
	tenant, user := uuid.New(), uuid.New()

	if _, err := newPosture(reader, repo).Execute(context.Background(), tenant, user); err == nil {
		t.Fatal("a failing reader must surface as an error, not as an empty posture")
	}
	if has, _ := repo.HasEvent(context.Background(), tenant, domain.ActivationPostureRevealed); has {
		t.Error("a failed read recorded a reveal")
	}
}

// Zero coverage and "nothing to cover" are different statements. Nil is the only
// honest answer to the second.
func TestCoveragePercent_NilWhenNothingIsApplicable(t *testing.T) {
	if got := coveragePercent(PostureControlCounts{}); got != nil {
		t.Errorf("no controls at all: coverage = %v, want nil", *got)
	}
	if got := coveragePercent(PostureControlCounts{Total: 5, NotApplicable: 5}); got != nil {
		t.Errorf("everything scoped out: coverage = %v, want nil", *got)
	}
	if got := coveragePercent(PostureControlCounts{Total: 4, Implemented: 0}); got == nil || *got != 0 {
		t.Errorf("applicable but none implemented: coverage = %v, want 0", got)
	}
	// in_progress earns no coverage: a control being worked on is not done.
	got := coveragePercent(PostureControlCounts{Total: 4, Implemented: 1, InProgress: 3})
	if got == nil || *got != 25 {
		t.Errorf("coverage = %v, want 25", got)
	}
}

func TestPostureSummary_IsRevealableRequiresAllThree(t *testing.T) {
	pct := 40.0
	full := &PostureSummary{
		Risks:           PostureRiskCounts{Total: 3},
		CoveragePercent: &pct,
		TopRisks:        []PostureRiskView{{}},
	}
	if !full.IsRevealable() {
		t.Fatal("a complete summary must be revealable")
	}

	noRisks := *full
	noRisks.Risks = PostureRiskCounts{}
	noCoverage := *full
	noCoverage.CoveragePercent = nil
	noTop := *full
	noTop.TopRisks = nil

	for name, s := range map[string]*PostureSummary{
		"no risk count": &noRisks,
		"no coverage":   &noCoverage,
		"no residual":   &noTop,
		"nil summary":   nil,
	} {
		if s.IsRevealable() {
			t.Errorf("%s must not be revealable", name)
		}
	}
}
