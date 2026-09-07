// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

// Package bulk is the one governed bulk-operation engine every register shares.
//
// #582's stated risk is that generalising bulk operations register by register
// is "how one defect becomes seven": six hand-written endpoints are six chances
// to drop a tenant filter, forget a transaction, or skip the audit entry. So the
// guarantees live here once — transaction boundary, all-or-nothing, one audit
// record per modified row, tenant scoping — and a register supplies only a
// domain.BulkStore that knows its own columns.
//
// The preview is the other half. A user selecting two hundred vulnerabilities
// and pressing a button commits blind; Preview says what will change, mutates
// nothing, and hands back a fingerprint that Apply verifies, so a set that moved
// underneath them is refused rather than silently acted on.
package bulk

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/opendefender/openrisk/internal/domain"
)

// Caller is the authenticated identity acting, taken from the signed session.
// Never from the request body: tenantID is what every query is scoped by.
type Caller struct {
	TenantID uuid.UUID
	UserID   uuid.UUID
}

// Validate fails closed on a zero identity rather than running unscoped.
func (c Caller) Validate() error {
	if c.TenantID == uuid.Nil {
		return domain.NewForbiddenError("no tenant in context")
	}
	if c.UserID == uuid.Nil {
		return domain.NewForbiddenError("no user in context")
	}
	return nil
}

// Request is one bulk operation.
type Request struct {
	IDs    []uuid.UUID
	Change domain.BulkChange
	// Fingerprint, when non-empty, must match the current state of the selection.
	// The UI sends back what Preview returned; a mismatch is refused (#582
	// criterion 3). Empty means the caller did not preview — allowed, because an
	// API client is not obliged to, and refusing would make the endpoint
	// unusable outside the UI.
	Fingerprint string
}

// Preview is the non-mutating answer to "what is about to happen".
type Preview struct {
	// Requested is how many distinct ids the caller named.
	Requested int `json:"requested"`
	// Found is how many of them exist in this tenant. A shortfall is what makes
	// the apply fail, so it is shown BEFORE the user commits rather than after.
	Found int `json:"found"`
	// Missing names the ids that did not resolve — stale selections, or ids from
	// another tenant, which are deliberately not distinguished.
	Missing []uuid.UUID `json:"missing"`
	// Affected is how many rows the change would actually alter. It can be lower
	// than Found: setting the status of a row that already has it changes
	// nothing, and saying so is more honest than counting it.
	Affected int `json:"affected"`
	// Unchanged is Found minus Affected.
	Unchanged int `json:"unchanged"`
	// Sample is a bounded, human-readable list of exactly what will change.
	Sample []SampleChange `json:"sample"`
	// Fingerprint pins the selection's current state. Pass it to Apply.
	Fingerprint string `json:"fingerprint"`
	// Action echoes what was previewed, so a UI cannot show one and send another.
	Action domain.BulkAction `json:"action"`
}

// PreviewSampleSize bounds the sample. A preview is a statement a human reads
// before committing, not a second copy of the result set.
const PreviewSampleSize = 10

// SampleChange is one row's before → after as the dialog renders it.
type SampleChange struct {
	ID            uuid.UUID              `json:"id"`
	Label         string                 `json:"label"`
	Before        map[string]interface{} `json:"before"`
	After         map[string]interface{} `json:"after"`
	ChangedFields []string               `json:"changed_fields"`
}

// Result is the outcome of an applied bulk operation. Same shape discipline as
// #581: all-or-nothing, so there is no per-item tally to misread.
type Result struct {
	Total   int         `json:"total"`
	Applied int         `json:"applied"`
	IDs     []uuid.UUID `json:"ids"`
	Audited int         `json:"audited"`
}

// Journal is the append-only audit trail. domain.AuditEventRepository satisfies
// it structurally.
type Journal interface {
	Append(ctx context.Context, e *domain.AuditEvent) error
}

// Engine runs bulk operations for ONE register.
type Engine struct {
	store   domain.BulkStore
	journal Journal
}

// New wires an engine. The journal is required, not optional: #581's whole
// finding was that an unaudited bulk mutation leaves a supervisor's "who did
// this" unanswerable, and an optional dependency is one somebody forgets.
func New(store domain.BulkStore, journal Journal) *Engine {
	return &Engine{store: store, journal: journal}
}

// EntityType is the register this engine serves.
func (e *Engine) EntityType() string { return e.store.EntityType() }

// Preview reports what a change would do. It MUST NOT mutate anything: it goes
// through LoadBulk, which is the read-only half of the port, and computes the
// after-state with domain.BulkChange.PredictOn — the same function the store
// applies, so the preview cannot drift from the mutation it precedes.
func (e *Engine) Preview(ctx context.Context, caller Caller, req Request) (*Preview, error) {
	ids, err := e.validate(caller, req)
	if err != nil {
		return nil, err
	}

	rows, err := e.store.LoadBulk(ctx, caller.TenantID, ids)
	if err != nil {
		return nil, wrapStoreErr(err)
	}

	found := make(map[uuid.UUID]struct{}, len(rows))
	for _, r := range rows {
		found[r.ID] = struct{}{}
	}
	missing := []uuid.UUID{}
	for _, id := range ids {
		if _, ok := found[id]; !ok {
			missing = append(missing, id)
		}
	}

	preview := &Preview{
		Requested:   len(ids),
		Found:       len(rows),
		Missing:     missing,
		Sample:      []SampleChange{},
		Fingerprint: domain.BulkFingerprint(rows),
		Action:      req.Change.Action,
	}

	for _, row := range rows {
		before := row.Snapshot
		after := req.Change.PredictOn(before)
		changed := domain.BulkChangedFields(before, after)

		// A delete changes no field but certainly affects the row.
		if req.Change.Action == domain.BulkActionDelete || len(changed) > 0 {
			preview.Affected++
		}
		if len(preview.Sample) < PreviewSampleSize {
			preview.Sample = append(preview.Sample, SampleChange{
				ID: row.ID, Label: row.Label,
				Before: before, After: after, ChangedFields: changed,
			})
		}
	}
	preview.Unchanged = preview.Found - preview.Affected
	return preview, nil
}

// Apply performs the change, all or none, and journals one entry per modified
// row.
func (e *Engine) Apply(ctx context.Context, caller Caller, req Request) (*Result, error) {
	ids, err := e.validate(caller, req)
	if err != nil {
		return nil, err
	}

	// #582 criterion 3 — if the caller previewed, the set must not have moved
	// since. Verified against a fresh read, before anything is written.
	if req.Fingerprint != "" {
		rows, err := e.store.LoadBulk(ctx, caller.TenantID, ids)
		if err != nil {
			return nil, wrapStoreErr(err)
		}
		if current := domain.BulkFingerprint(rows); current != req.Fingerprint {
			return nil, domain.NewConflictError(
				"the selection changed since it was previewed", "fingerprint")
		}
	}

	var mutations []domain.BulkMutation
	if req.Change.Action == domain.BulkActionDelete {
		mutations, err = e.store.DeleteBulk(ctx, caller.TenantID, ids)
	} else {
		mutations, err = e.store.ApplyBulk(ctx, caller.TenantID, ids, req.Change)
	}
	if err != nil {
		// Typed errors travel out unchanged: a stale or foreign id stays 404
		// rather than becoming a 500.
		return nil, wrapStoreErr(err)
	}

	result := &Result{
		Total:   len(ids),
		Applied: len(mutations),
		IDs:     make([]uuid.UUID, 0, len(mutations)),
	}
	for _, m := range mutations {
		result.IDs = append(result.IDs, m.ID)
	}
	result.Audited = e.journalAll(ctx, caller, req.Change, mutations)
	return result, nil
}

// validate applies every module-agnostic guard, in the order that fails
// cheapest first.
func (e *Engine) validate(caller Caller, req Request) ([]uuid.UUID, error) {
	if err := caller.Validate(); err != nil {
		return nil, err
	}
	if err := req.Change.Validate(); err != nil {
		return nil, err
	}
	if !domain.SupportsBulkAction(e.store, req.Change.Action) {
		// Refused rather than silently ignored: a register that cannot assign
		// must say so, not accept the request and change nothing.
		return nil, domain.NewValidationError(fmt.Sprintf(
			"%s does not support the %q bulk action", e.store.EntityType(), req.Change.Action))
	}
	if err := e.store.ValidateBulkChange(req.Change); err != nil {
		return nil, err
	}
	return distinctIDs(req.IDs)
}

// distinctIDs validates the selection and removes repeats. De-duplication is not
// cosmetic: the stores enforce all-or-nothing by comparing rows affected against
// the number of ids, so a repeated id would make a valid batch look short.
func distinctIDs(ids []uuid.UUID) ([]uuid.UUID, error) {
	if len(ids) == 0 {
		return nil, domain.NewValidationError("at least one id is required")
	}
	if len(ids) > domain.MaxBulkItems {
		return nil, domain.NewValidationError(fmt.Sprintf(
			"bulk operations are limited to %d items maximum", domain.MaxBulkItems))
	}
	seen := make(map[uuid.UUID]struct{}, len(ids))
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if id == uuid.Nil {
			return nil, domain.NewValidationError("an id in the selection is empty")
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out, nil
}

// journalAll writes one audit entry per modified row and returns how many
// landed. A journal outage does not fail a mutation that already committed
// (D-004, observable best-effort) but the shortfall is reported, never hidden.
func (e *Engine) journalAll(
	ctx context.Context,
	caller Caller,
	change domain.BulkChange,
	mutations []domain.BulkMutation,
) int {
	if e.journal == nil || len(mutations) == 0 {
		return 0
	}

	action := domain.AuditActionUpdate
	if change.Action == domain.BulkActionDelete {
		action = domain.AuditActionDelete
	}
	var actor *uuid.UUID
	if caller.UserID != uuid.Nil {
		actor = &caller.UserID
	}

	summary := fmt.Sprintf("bulk %s on %s", change.Action, e.store.EntityType())
	if change.Action == domain.BulkActionChangeStatus {
		summary = fmt.Sprintf("bulk status change to %q on %s", change.Status, e.store.EntityType())
	}
	if change.Justification != "" {
		summary = fmt.Sprintf("%s — %s", summary, change.Justification)
	}

	written := 0
	now := time.Now().UTC()
	for _, m := range mutations {
		event := &domain.AuditEvent{
			ID:            uuid.New(),
			TenantID:      caller.TenantID,
			ActorID:       actor,
			Action:        action,
			EntityType:    e.store.EntityType(),
			EntityID:      m.ID.String(),
			Summary:       summary,
			Before:        domain.JSONMap(m.Before),
			After:         domain.JSONMap(m.After),
			ChangedFields: domain.StringList(m.ChangedFields),
			Source:        "explicit",
			CreatedAt:     now,
		}
		if err := e.journal.Append(ctx, event); err != nil {
			continue
		}
		written++
	}
	return written
}

// wrapStoreErr keeps typed errors typed and turns anything else into an internal
// error, so a repository's raw driver message never reaches a client.
func wrapStoreErr(err error) error {
	var appErr *domain.AppError
	if asAppError(err, &appErr) {
		return err
	}
	return domain.NewInternalError(err.Error())
}

// asAppError is errors.As, kept behind a name so the intent reads at the call
// site: "is this already a typed application error?".
func asAppError(err error, target **domain.AppError) bool {
	return errors.As(err, target)
}
