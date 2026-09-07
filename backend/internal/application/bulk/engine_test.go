// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package bulk

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/opendefender/openrisk/internal/domain"
)

// fakeStore is a tenant-scoped, all-or-nothing store — the two guarantees the
// real ones make, so these tests are about the ENGINE on top of a correct store.
// That the SQL implementations keep them is proven separately against SQLite in
// repository/gorm_bulk_stores_test.go.
type fakeStore struct {
	rows      map[uuid.UUID]*row
	supported []domain.BulkAction
	loads     int
	failLoad  error
}

type row struct {
	tenant uuid.UUID
	label  string
	status string
}

func newFakeStore(actions ...domain.BulkAction) *fakeStore {
	if len(actions) == 0 {
		actions = []domain.BulkAction{domain.BulkActionChangeStatus, domain.BulkActionDelete}
	}
	return &fakeStore{rows: map[uuid.UUID]*row{}, supported: actions}
}

func (f *fakeStore) seed(tenant uuid.UUID, label, status string) uuid.UUID {
	id := uuid.New()
	f.rows[id] = &row{tenant: tenant, label: label, status: status}
	return id
}

func (f *fakeStore) EntityType() string                          { return "vulnerability" }
func (f *fakeStore) SupportedBulkActions() []domain.BulkAction    { return f.supported }
func (f *fakeStore) ValidateBulkChange(c domain.BulkChange) error {
	if c.Action == domain.BulkActionChangeStatus && c.Status == "not_a_status" {
		return domain.NewValidationError("unknown status")
	}
	return nil
}

func (f *fakeStore) project(id uuid.UUID, r *row) domain.BulkRow {
	return domain.BulkRow{ID: id, Label: r.label, Snapshot: map[string]interface{}{"status": r.status}}
}

func (f *fakeStore) LoadBulk(_ context.Context, tenantID uuid.UUID, ids []uuid.UUID) ([]domain.BulkRow, error) {
	f.loads++
	if f.failLoad != nil {
		return nil, f.failLoad
	}
	if tenantID == uuid.Nil {
		return nil, domain.NewForbiddenError("tenant_id is required")
	}
	out := []domain.BulkRow{}
	for _, id := range ids {
		r, ok := f.rows[id]
		if !ok || r.tenant != tenantID {
			continue // absent and foreign are the same thing here, on purpose
		}
		out = append(out, f.project(id, r))
	}
	return out, nil
}

func (f *fakeStore) ApplyBulk(_ context.Context, tenantID uuid.UUID, ids []uuid.UUID, change domain.BulkChange) ([]domain.BulkMutation, error) {
	staged := map[uuid.UUID]string{}
	muts := []domain.BulkMutation{}
	for _, id := range ids {
		r, ok := f.rows[id]
		if !ok || r.tenant != tenantID {
			return nil, domain.NewNotFoundError("vulnerability", id)
		}
		before := f.project(id, r).Snapshot
		after := change.PredictOn(before)
		staged[id] = after["status"].(string)
		muts = append(muts, domain.BulkMutation{
			ID: id, Label: r.label, Before: before, After: after,
			ChangedFields: domain.BulkChangedFields(before, after),
		})
	}
	for id, status := range staged { // commit only once the whole batch resolved
		f.rows[id].status = status
	}
	return muts, nil
}

func (f *fakeStore) DeleteBulk(_ context.Context, tenantID uuid.UUID, ids []uuid.UUID) ([]domain.BulkMutation, error) {
	muts := []domain.BulkMutation{}
	for _, id := range ids {
		r, ok := f.rows[id]
		if !ok || r.tenant != tenantID {
			return nil, domain.NewNotFoundError("vulnerability", id)
		}
		muts = append(muts, domain.BulkMutation{
			ID: id, Label: r.label, Before: f.project(id, r).Snapshot,
			After: map[string]interface{}{}, ChangedFields: []string{"deleted"},
		})
	}
	for _, id := range ids {
		delete(f.rows, id)
	}
	return muts, nil
}

type fakeJournal struct {
	events []*domain.AuditEvent
	fail   bool
}

func (j *fakeJournal) Append(_ context.Context, e *domain.AuditEvent) error {
	if j.fail {
		return errors.New("journal unavailable")
	}
	j.events = append(j.events, e)
	return nil
}

func newEngine(store domain.BulkStore) (*Engine, *fakeJournal) {
	j := &fakeJournal{}
	return New(store, j), j
}

func statusChange(status string) domain.BulkChange {
	return domain.BulkChange{Action: domain.BulkActionChangeStatus, Status: status}
}

// ---------------------------------------------------------------------------
// Success
// ---------------------------------------------------------------------------

func TestBulkEngine_Success(t *testing.T) {
	ctx := context.Background()
	tenant, actor := uuid.New(), uuid.New()
	store := newFakeStore()
	a := store.seed(tenant, "CVE-2021-44228", "open")
	b := store.seed(tenant, "CVE-2023-1234", "open")
	engine, journal := newEngine(store)
	caller := Caller{TenantID: tenant, UserID: actor}

	res, err := engine.Apply(ctx, caller, Request{
		IDs:    []uuid.UUID{a, b},
		Change: domain.BulkChange{Action: domain.BulkActionChangeStatus, Status: "triaged", Justification: "revue hebdo"},
	})
	require.NoError(t, err)

	assert.Equal(t, 2, res.Total)
	assert.Equal(t, 2, res.Applied)
	assert.Equal(t, 2, res.Audited)
	assert.Equal(t, "triaged", store.rows[a].status)
	assert.Equal(t, "triaged", store.rows[b].status)

	require.Len(t, journal.events, 2, "one audit record per modified row")
	for _, e := range journal.events {
		require.NotNil(t, e.ActorID)
		assert.Equal(t, actor, *e.ActorID)
		assert.Equal(t, tenant, e.TenantID)
		assert.Equal(t, "vulnerability", e.EntityType)
		assert.Equal(t, domain.AuditActionUpdate, e.Action)
		assert.Equal(t, "explicit", e.Source)
		assert.Contains(t, e.Summary, "revue hebdo")
		assert.Equal(t, "open", e.Before["status"])
		assert.Equal(t, "triaged", e.After["status"])
		assert.Contains(t, e.ChangedFields, "status")
		assert.NotEqual(t, uuid.Nil.String(), e.EntityID, "the entry must name the row")
	}
}

func TestBulkEngine_Success_Delete(t *testing.T) {
	ctx := context.Background()
	tenant, actor := uuid.New(), uuid.New()
	store := newFakeStore()
	a := store.seed(tenant, "CVE-1", "open")
	engine, journal := newEngine(store)

	res, err := engine.Apply(ctx, Caller{TenantID: tenant, UserID: actor}, Request{
		IDs: []uuid.UUID{a}, Change: domain.BulkChange{Action: domain.BulkActionDelete},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Applied)
	assert.NotContains(t, store.rows, a)
	require.Len(t, journal.events, 1)
	assert.Equal(t, domain.AuditActionDelete, journal.events[0].Action)
	assert.Equal(t, a.String(), journal.events[0].EntityID)
	assert.Equal(t, "open", journal.events[0].Before["status"], "a deletion records what went")
}

// ---------------------------------------------------------------------------
// Criterion 2 — the preview mutates nothing
// ---------------------------------------------------------------------------

func TestBulkEngine_Preview_MutatesNothing(t *testing.T) {
	ctx := context.Background()
	tenant := uuid.New()
	store := newFakeStore()
	a := store.seed(tenant, "CVE-2021-44228", "open")
	b := store.seed(tenant, "CVE-2023-1234", "triaged")
	engine, journal := newEngine(store)

	before := map[uuid.UUID]string{a: store.rows[a].status, b: store.rows[b].status}

	preview, err := engine.Preview(ctx, Caller{TenantID: tenant, UserID: uuid.New()}, Request{
		IDs: []uuid.UUID{a, b}, Change: statusChange("triaged"),
	})
	require.NoError(t, err)

	// Byte-identical row state before and after the preview call.
	for id, status := range before {
		assert.Equal(t, status, store.rows[id].status, "preview must not mutate row %s", id)
	}
	assert.Empty(t, journal.events, "a preview writes no audit entry — nothing happened")

	assert.Equal(t, 2, preview.Requested)
	assert.Equal(t, 2, preview.Found)
	// b is already "triaged", so the change would alter exactly one row. Saying
	// "2 will change" would overstate what the user is about to do.
	assert.Equal(t, 1, preview.Affected)
	assert.Equal(t, 1, preview.Unchanged)
	assert.Empty(t, preview.Missing)
	assert.NotEmpty(t, preview.Fingerprint)
	assert.Equal(t, domain.BulkActionChangeStatus, preview.Action)

	require.Len(t, preview.Sample, 2)
	assert.Equal(t, "CVE-2021-44228", preview.Sample[0].Label, "the sample names rows, not ids")
	assert.Equal(t, "open", preview.Sample[0].Before["status"])
	assert.Equal(t, "triaged", preview.Sample[0].After["status"])
}

func TestBulkEngine_Preview_ReportsIdsThatWillFailTheApply(t *testing.T) {
	ctx := context.Background()
	tenant := uuid.New()
	store := newFakeStore()
	a := store.seed(tenant, "CVE-1", "open")
	stale := uuid.New()
	engine, _ := newEngine(store)

	preview, err := engine.Preview(ctx, Caller{TenantID: tenant, UserID: uuid.New()}, Request{
		IDs: []uuid.UUID{a, stale}, Change: statusChange("triaged"),
	})
	require.NoError(t, err)

	// The shortfall is shown BEFORE the user commits, rather than surfacing as a
	// failure after they press the button.
	assert.Equal(t, 2, preview.Requested)
	assert.Equal(t, 1, preview.Found)
	assert.Equal(t, []uuid.UUID{stale}, preview.Missing)
}

func TestBulkEngine_Preview_SampleIsBounded(t *testing.T) {
	ctx := context.Background()
	tenant := uuid.New()
	store := newFakeStore()
	ids := make([]uuid.UUID, 0, 25)
	for i := 0; i < 25; i++ {
		ids = append(ids, store.seed(tenant, "CVE", "open"))
	}
	engine, _ := newEngine(store)

	preview, err := engine.Preview(ctx, Caller{TenantID: tenant, UserID: uuid.New()}, Request{
		IDs: ids, Change: statusChange("triaged"),
	})
	require.NoError(t, err)
	assert.Equal(t, 25, preview.Affected)
	assert.Len(t, preview.Sample, PreviewSampleSize,
		"a preview is a statement a human reads, not a second copy of the result set")
}

// ---------------------------------------------------------------------------
// Criterion 3 — a set that moved under the user is refused
// ---------------------------------------------------------------------------

func TestBulkEngine_Apply_RefusesAStalePreview(t *testing.T) {
	ctx := context.Background()
	tenant := uuid.New()
	store := newFakeStore()
	a := store.seed(tenant, "CVE-1", "open")
	b := store.seed(tenant, "CVE-2", "open")
	engine, journal := newEngine(store)
	caller := Caller{TenantID: tenant, UserID: uuid.New()}
	req := Request{IDs: []uuid.UUID{a, b}, Change: statusChange("triaged")}

	preview, err := engine.Preview(ctx, caller, req)
	require.NoError(t, err)

	// Somebody else triages one of them between the preview and the confirm.
	store.rows[b].status = "false_positive"

	req.Fingerprint = preview.Fingerprint
	_, err = engine.Apply(ctx, caller, req)
	require.ErrorIs(t, err, domain.ErrConflict)
	assert.Contains(t, err.Error(), "previewed")

	// Refused BEFORE anything was written.
	assert.Equal(t, "open", store.rows[a].status)
	assert.Equal(t, "false_positive", store.rows[b].status)
	assert.Empty(t, journal.events)

	// Re-previewing and confirming against the new state goes through.
	fresh, err := engine.Preview(ctx, caller, req)
	require.NoError(t, err)
	req.Fingerprint = fresh.Fingerprint
	_, err = engine.Apply(ctx, caller, req)
	require.NoError(t, err)
	assert.Equal(t, "triaged", store.rows[a].status)
}

func TestBulkEngine_Apply_WithoutAFingerprintIsAllowed(t *testing.T) {
	ctx := context.Background()
	tenant := uuid.New()
	store := newFakeStore()
	a := store.seed(tenant, "CVE-1", "open")
	engine, _ := newEngine(store)

	// An API client is not obliged to preview; refusing would make the endpoint
	// unusable outside the UI.
	_, err := engine.Apply(ctx, Caller{TenantID: tenant, UserID: uuid.New()}, Request{
		IDs: []uuid.UUID{a}, Change: statusChange("triaged"),
	})
	require.NoError(t, err)
	assert.Equal(t, "triaged", store.rows[a].status)
}

// The fingerprint must not be sensitive to request ORDER, or re-selecting the
// same rows in a different order would look like a change.
func TestBulkEngine_FingerprintIsOrderIndependent(t *testing.T) {
	ctx := context.Background()
	tenant := uuid.New()
	store := newFakeStore()
	a := store.seed(tenant, "CVE-1", "open")
	b := store.seed(tenant, "CVE-2", "open")
	engine, _ := newEngine(store)
	caller := Caller{TenantID: tenant, UserID: uuid.New()}

	one, err := engine.Preview(ctx, caller, Request{IDs: []uuid.UUID{a, b}, Change: statusChange("triaged")})
	require.NoError(t, err)
	two, err := engine.Preview(ctx, caller, Request{IDs: []uuid.UUID{b, a}, Change: statusChange("triaged")})
	require.NoError(t, err)
	assert.Equal(t, one.Fingerprint, two.Fingerprint)
}

// ---------------------------------------------------------------------------
// NotFound / cross-tenant
// ---------------------------------------------------------------------------

func TestBulkEngine_NotFound(t *testing.T) {
	ctx := context.Background()
	tenant := uuid.New()
	store := newFakeStore()
	engine, _ := newEngine(store)

	_, err := engine.Apply(ctx, Caller{TenantID: tenant, UserID: uuid.New()}, Request{
		IDs: []uuid.UUID{uuid.New()}, Change: statusChange("triaged"),
	})
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

// Criterion 5 — ids from another tenant are indistinguishable from fabricated
// ones, and nothing is modified.
func TestBulkEngine_CrossTenant_IsIndistinguishableAndModifiesNothing(t *testing.T) {
	ctx := context.Background()
	tenantA, tenantB := uuid.New(), uuid.New()
	store := newFakeStore()
	mine := store.seed(tenantA, "mine", "open")
	theirs := store.seed(tenantB, "theirs", "open")
	engine, journal := newEngine(store)
	caller := Caller{TenantID: tenantA, UserID: uuid.New()}

	_, fabricatedErr := engine.Apply(ctx, caller, Request{
		IDs: []uuid.UUID{uuid.New()}, Change: statusChange("triaged"),
	})
	_, foreignErr := engine.Apply(ctx, caller, Request{
		IDs: []uuid.UUID{theirs}, Change: statusChange("triaged"),
	})

	require.ErrorIs(t, fabricatedErr, domain.ErrNotFound)
	assert.ErrorIs(t, foreignErr, domain.ErrNotFound)
	assert.NotErrorIs(t, foreignErr, domain.ErrForbidden, "a 403 would confirm the row exists")
	assert.Equal(t, domain.HTTPStatusFromError(fabricatedErr), domain.HTTPStatusFromError(foreignErr))
	assert.Equal(t, domain.MessageFromError(fabricatedErr), domain.MessageFromError(foreignErr))

	// Mixed into an otherwise valid batch: nothing at all is modified.
	_, err := engine.Apply(ctx, caller, Request{
		IDs: []uuid.UUID{mine, theirs}, Change: statusChange("triaged"),
	})
	assert.ErrorIs(t, err, domain.ErrNotFound)
	assert.Equal(t, "open", store.rows[mine].status)
	assert.Equal(t, "open", store.rows[theirs].status)
	assert.Empty(t, journal.events)

	// And the preview does not reveal it either — it comes back as missing.
	preview, err := engine.Preview(ctx, caller, Request{
		IDs: []uuid.UUID{theirs}, Change: statusChange("triaged"),
	})
	require.NoError(t, err)
	assert.Equal(t, 0, preview.Found)
	assert.Equal(t, []uuid.UUID{theirs}, preview.Missing)
}

// ---------------------------------------------------------------------------
// Unauthorized
// ---------------------------------------------------------------------------

// A zero identity fails closed rather than running an unscoped mutation. The
// HTTP 403 for a caller without the module permission is enforced by the route's
// RequirePermission middleware (criterion 4) and pinned in the handler tests.
func TestBulkEngine_Unauthorized(t *testing.T) {
	ctx := context.Background()
	tenant := uuid.New()
	store := newFakeStore()
	a := store.seed(tenant, "CVE-1", "open")
	engine, journal := newEngine(store)

	for name, caller := range map[string]Caller{
		"no tenant": {UserID: uuid.New()},
		"no user":   {TenantID: tenant},
		"neither":   {},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := engine.Apply(ctx, caller, Request{IDs: []uuid.UUID{a}, Change: statusChange("triaged")})
			assert.ErrorIs(t, err, domain.ErrForbidden)

			_, err = engine.Preview(ctx, caller, Request{IDs: []uuid.UUID{a}, Change: statusChange("triaged")})
			assert.ErrorIs(t, err, domain.ErrForbidden)
		})
	}
	assert.Equal(t, "open", store.rows[a].status)
	assert.Empty(t, journal.events)
	assert.Equal(t, 0, store.loads, "a refused call must never reach the store")
}

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

// A register that does not support an action must REFUSE it, not accept the
// request and change nothing — which is how a user concludes the feature is
// broken rather than absent.
func TestBulkEngine_RefusesAnActionTheRegisterDoesNotSupport(t *testing.T) {
	ctx := context.Background()
	tenant := uuid.New()
	store := newFakeStore(domain.BulkActionDelete) // delete-only, like assets
	a := store.seed(tenant, "srv-01", "")
	engine, _ := newEngine(store)
	caller := Caller{TenantID: tenant, UserID: uuid.New()}

	_, err := engine.Apply(ctx, caller, Request{IDs: []uuid.UUID{a}, Change: statusChange("triaged")})
	require.ErrorIs(t, err, domain.ErrValidation)
	assert.Contains(t, err.Error(), "change_status")
	assert.Contains(t, store.rows, a)

	_, err = engine.Apply(ctx, caller, Request{
		IDs: []uuid.UUID{a}, Change: domain.BulkChange{Action: domain.BulkActionDelete},
	})
	require.NoError(t, err)
}

func TestBulkEngine_Validation(t *testing.T) {
	ctx := context.Background()
	tenant := uuid.New()
	store := newFakeStore()
	a := store.seed(tenant, "CVE-1", "open")
	engine, _ := newEngine(store)
	caller := Caller{TenantID: tenant, UserID: uuid.New()}

	tooMany := make([]uuid.UUID, domain.MaxBulkItems+1)
	for i := range tooMany {
		tooMany[i] = uuid.New()
	}

	for name, req := range map[string]Request{
		"empty selection":  {IDs: nil, Change: statusChange("triaged")},
		"over the cap":     {IDs: tooMany, Change: statusChange("triaged")},
		"a nil id":         {IDs: []uuid.UUID{uuid.Nil}, Change: statusChange("triaged")},
		"unknown action":   {IDs: []uuid.UUID{a}, Change: domain.BulkChange{Action: "teleport"}},
		"status missing":   {IDs: []uuid.UUID{a}, Change: domain.BulkChange{Action: domain.BulkActionChangeStatus}},
		"status not valid": {IDs: []uuid.UUID{a}, Change: statusChange("not_a_status")},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := engine.Apply(ctx, caller, req)
			assert.ErrorIs(t, err, domain.ErrValidation)
		})
	}
	assert.Contains(t, err100(engine, ctx, caller, tooMany), "100")
	assert.Equal(t, "open", store.rows[a].status)
}

func err100(e *Engine, ctx context.Context, c Caller, ids []uuid.UUID) string {
	_, err := e.Apply(ctx, c, Request{IDs: ids, Change: statusChange("triaged")})
	return err.Error()
}

func TestBulkEngine_DeduplicatesTheSelection(t *testing.T) {
	ctx := context.Background()
	tenant := uuid.New()
	store := newFakeStore()
	a := store.seed(tenant, "CVE-1", "open")
	engine, journal := newEngine(store)

	res, err := engine.Apply(ctx, Caller{TenantID: tenant, UserID: uuid.New()}, Request{
		IDs: []uuid.UUID{a, a, a}, Change: statusChange("triaged"),
	})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Total)
	assert.Len(t, journal.events, 1, "one audit record per row, not per mention")
}

// D-004 — the trail is best-effort. An outage must not fail a mutation that
// already committed, but it must be visible.
func TestBulkEngine_JournalFailureIsReportedNotFatal(t *testing.T) {
	ctx := context.Background()
	tenant := uuid.New()
	store := newFakeStore()
	a := store.seed(tenant, "CVE-1", "open")
	engine := New(store, &fakeJournal{fail: true})

	res, err := engine.Apply(ctx, Caller{TenantID: tenant, UserID: uuid.New()}, Request{
		IDs: []uuid.UUID{a}, Change: statusChange("triaged"),
	})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Applied)
	assert.Equal(t, 0, res.Audited, "the shortfall is reported, not swallowed")
	assert.Equal(t, "triaged", store.rows[a].status)
}

// A raw store error must not reach the client as a driver message.
func TestBulkEngine_WrapsUntypedStoreErrors(t *testing.T) {
	ctx := context.Background()
	tenant := uuid.New()
	store := newFakeStore()
	store.failLoad = errors.New("pq: connection reset by peer")
	engine, _ := newEngine(store)

	_, err := engine.Preview(ctx, Caller{TenantID: tenant, UserID: uuid.New()}, Request{
		IDs: []uuid.UUID{uuid.New()}, Change: statusChange("triaged"),
	})
	require.Error(t, err)
	assert.Equal(t, 500, domain.HTTPStatusFromError(err))
	assert.NotContains(t, domain.MessageFromError(err), "connection reset")
}
