// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package risk

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/opendefender/openrisk/internal/domain"
)

// ---------------------------------------------------------------------------
// Bulk actions on the risk register.
//
// D-036 settled the semantics: **all-or-nothing**. A bulk action either applies
// to every named risk or to none of them. There is no per-item outcome to
// report, because for a COBAC- or BCEAO-supervised institution "31 of your 40
// risks were reassigned and we cannot tell you which" is a supervisory finding,
// and a clean rollback is not.
//
// This file previously claimed that guarantee in its comments and did the
// opposite in its code: it looped without a transaction, tallied successes and
// failures, and accepted a `performedBy` it never read. Comments that promise a
// governance property the code lacks are worse than no comments — a reviewer or
// an agent can report the promise as a capability (ABSOLUTE RULE 12). Every
// claim below is now one the code keeps.
// ---------------------------------------------------------------------------

// BulkActionType is the closed set of supported bulk actions.
type BulkActionType string

const (
	BulkActionChangeStatus BulkActionType = "change_status"
	BulkActionAssignTo     BulkActionType = "assign_to"
	BulkActionAddTags      BulkActionType = "add_tags"
	BulkActionRemoveTags   BulkActionType = "remove_tags"
	BulkActionDeleteRisks  BulkActionType = "delete"
)

// MaxBulkActionItems caps one request. The cap is what makes a synchronous
// implementation adequate — no queue, no progress stream.
const MaxBulkActionItems = 100

// BulkActionRequest is one bulk operation over a set of risks.
type BulkActionRequest struct {
	Type    BulkActionType `json:"type"`
	RiskIDs []uuid.UUID    `json:"risk_ids"` // Max MaxBulkActionItems

	// Action-specific parameters.
	Status        *domain.RiskStatus `json:"status,omitempty"`        // change_status
	AssignToID    *uuid.UUID         `json:"assign_to_id,omitempty"`  // assign_to
	Tags          []string           `json:"tags,omitempty"`          // add_tags / remove_tags
	Justification string             `json:"justification,omitempty"` // recorded on the audit entry
}

// BulkActionResult is the outcome of a bulk operation.
//
// BREAKING CHANGE (D-036): this replaces {Success, Failed, Errors, UpdatedRisks}.
// Under all-or-nothing those fields could only ever report "all" or "none", so a
// per-item shape would be theatre — a `Failed` count that is structurally always
// zero invites callers to write handling that can never run. A failure is now an
// error, not a field.
type BulkActionResult struct {
	// Total is how many risks the caller named (after de-duplication).
	Total int `json:"total"`
	// Applied equals Total on success. The call returns an error otherwise, so
	// this is never a partial number.
	Applied int `json:"applied"`
	// RiskIDs are the risks modified, in the order supplied.
	RiskIDs []uuid.UUID `json:"risk_ids"`
	// Audited is how many audit entries were written. It can fall short of
	// Applied only if the journal itself failed, which is deliberately not fatal
	// (D-004: the trail is observable best-effort now, guaranteed later) — but it
	// is reported rather than hidden, so "the change happened but was not fully
	// journalled" is visible instead of silent.
	Audited int `json:"audited"`
}

// AuditJournal is the append-only trail this use case writes to. Narrow on
// purpose: the use case needs to append, nothing else.
//
// domain.AuditEventRepository satisfies it structurally, so the chained,
// hash-sealed store is what it gets in production without this package
// depending on the whole port.
type AuditJournal interface {
	Append(ctx context.Context, e *domain.AuditEvent) error
}

// BulkActionUseCase applies one mutation to many risks, atomically and audited.
type BulkActionUseCase struct {
	riskRepo domain.RiskRepository
	journal  AuditJournal
}

// NewBulkActionUseCase wires the use case.
//
// The journal is a REQUIRED argument, not an optional WithX. Auditing a bulk
// mutation is the point of #581 — the previous code accepted `performedBy` and
// discarded it, so a supervisor asking "who reassigned these and when" had no
// answer. Making the journal impossible to omit is what stops that recurring.
// Pass nil only in a test that is explicitly about the un-journalled path.
func NewBulkActionUseCase(riskRepo domain.RiskRepository, journal AuditJournal) *BulkActionUseCase {
	return &BulkActionUseCase{riskRepo: riskRepo, journal: journal}
}

// Execute applies a bulk action.
//
// Guarantees, all of them enforced below rather than asserted:
//  1. Every risk is modified, or none is (the repository runs one transaction).
//  2. One audit entry per modified risk, carrying performedBy.
//  3. tenant_id scopes every read and write, so an id from another tenant fails
//     exactly as a fabricated one does — not found, nothing modified.
//  4. At most MaxBulkActionItems per call; an empty selection is a validation
//     error, not a silent no-op.
func (uc *BulkActionUseCase) Execute(
	ctx context.Context,
	tenantID uuid.UUID,
	input BulkActionRequest,
	performedBy uuid.UUID,
) (*BulkActionResult, error) {
	if tenantID == uuid.Nil {
		return nil, domain.NewForbiddenError("no tenant in context")
	}

	ids, err := distinctIDs(input.RiskIDs)
	if err != nil {
		return nil, err
	}

	switch input.Type {
	case BulkActionChangeStatus:
		if input.Status == nil {
			return nil, domain.NewValidationError("status is required for change_status action")
		}
		return uc.apply(ctx, tenantID, ids, performedBy, input,
			fmt.Sprintf("bulk status change to %q", *input.Status),
			func(r *domain.Risk) error {
				// Never write Status directly: the domain derives status, phase and
				// canonical state from one another (Risk.SetState), and writing one
				// of the three by hand is what let them disagree in the first place.
				r.SetState(domain.RiskStateFromLegacy(*input.Status, r.LifecyclePhase))
				return nil
			})

	case BulkActionAssignTo:
		if input.AssignToID == nil {
			return nil, domain.NewValidationError("assign_to_id is required for assign_to action")
		}
		return uc.apply(ctx, tenantID, ids, performedBy, input,
			fmt.Sprintf("bulk assignment to %s", input.AssignToID),
			func(r *domain.Risk) error {
				assignee := *input.AssignToID
				r.AssignedTo = &assignee
				return nil
			})

	case BulkActionAddTags:
		if len(input.Tags) == 0 {
			return nil, domain.NewValidationError("tags are required for add_tags action")
		}
		return uc.apply(ctx, tenantID, ids, performedBy, input,
			fmt.Sprintf("bulk add tags %v", input.Tags),
			func(r *domain.Risk) error {
				r.Tags = addTags(r.Tags, input.Tags)
				return nil
			})

	case BulkActionRemoveTags:
		// Previously declared and never handled, so every remove_tags request fell
		// through to the default branch and came back "unknown action type:
		// remove_tags" — a constant whose only reference in the repo was its own
		// declaration.
		if len(input.Tags) == 0 {
			return nil, domain.NewValidationError("tags are required for remove_tags action")
		}
		return uc.apply(ctx, tenantID, ids, performedBy, input,
			fmt.Sprintf("bulk remove tags %v", input.Tags),
			func(r *domain.Risk) error {
				r.Tags = removeTags(r.Tags, input.Tags)
				return nil
			})

	case BulkActionDeleteRisks:
		return uc.bulkDelete(ctx, tenantID, ids, performedBy, input)

	default:
		return nil, domain.NewValidationError(fmt.Sprintf("unknown action type: %s", input.Type))
	}
}

// apply runs one mutation over the batch inside the repository's transaction,
// then journals what changed.
func (uc *BulkActionUseCase) apply(
	ctx context.Context,
	tenantID uuid.UUID,
	ids []uuid.UUID,
	performedBy uuid.UUID,
	input BulkActionRequest,
	summary string,
	mutate func(*domain.Risk) error,
) (*BulkActionResult, error) {
	mutations, err := uc.riskRepo.BulkApply(ctx, tenantID, ids, mutate)
	if err != nil {
		// The repository already rolled back. Typed errors travel out unchanged so
		// a missing or foreign id stays 404 rather than becoming a 500.
		return nil, err
	}

	result := &BulkActionResult{
		Total:   len(ids),
		Applied: len(mutations),
		RiskIDs: make([]uuid.UUID, 0, len(mutations)),
	}
	for _, m := range mutations {
		result.RiskIDs = append(result.RiskIDs, m.RiskID)
	}

	result.Audited = uc.journalMutations(ctx, tenantID, performedBy, input, summary, mutations)
	return result, nil
}

// bulkDelete removes the batch. It goes through the repository's BulkDelete,
// which is a single statement and therefore already atomic — and is now strict,
// so a stale or foreign id in the selection deletes nothing at all.
func (uc *BulkActionUseCase) bulkDelete(
	ctx context.Context,
	tenantID uuid.UUID,
	ids []uuid.UUID,
	performedBy uuid.UUID,
	input BulkActionRequest,
) (*BulkActionResult, error) {
	deleted, err := uc.riskRepo.BulkDelete(ctx, ids, tenantID)
	if err != nil {
		return nil, err
	}

	// A deletion has a before but no after. The trail records which ids went, and
	// the "before" is deliberately absent rather than fabricated: the rows were
	// not read, so claiming a snapshot would be inventing one.
	mutations := make([]domain.RiskMutation, 0, len(ids))
	for _, id := range ids {
		mutations = append(mutations, domain.RiskMutation{RiskID: id})
	}

	result := &BulkActionResult{
		Total:   len(ids),
		Applied: int(deleted),
		RiskIDs: ids,
	}
	result.Audited = uc.journalMutations(ctx, tenantID, performedBy, input, "bulk delete", mutations)
	return result, nil
}

// journalMutations appends one audit entry per modified risk and returns how
// many landed.
//
// A journal failure does NOT fail the request. That is D-004's recorded posture
// (observable best-effort now, guaranteed later): the mutation is already
// committed, so erroring here would report a failure for work that did happen —
// the worst of both. The shortfall is returned instead, so it is visible.
func (uc *BulkActionUseCase) journalMutations(
	ctx context.Context,
	tenantID uuid.UUID,
	performedBy uuid.UUID,
	input BulkActionRequest,
	summary string,
	mutations []domain.RiskMutation,
) int {
	if uc.journal == nil || len(mutations) == 0 {
		return 0
	}

	action := domain.AuditActionUpdate
	if input.Type == BulkActionDeleteRisks {
		action = domain.AuditActionDelete
	}

	// A nil actor marks a system change. A bulk action always has a human behind
	// it, so an absent id is worth recording as absent rather than as anybody.
	var actor *uuid.UUID
	if performedBy != uuid.Nil {
		actor = &performedBy
	}

	fullSummary := summary
	if input.Justification != "" {
		fullSummary = fmt.Sprintf("%s — %s", summary, input.Justification)
	}

	written := 0
	now := time.Now().UTC()
	for _, m := range mutations {
		event := &domain.AuditEvent{
			ID:            uuid.New(),
			TenantID:      tenantID,
			ActorID:       actor,
			Action:        action,
			EntityType:    "risk",
			EntityID:      m.RiskID.String(),
			Summary:       fullSummary,
			Before:        domain.JSONMap(m.Before),
			After:         domain.JSONMap(m.After),
			ChangedFields: domain.StringList(m.ChangedFields),
			Source:        "explicit",
			CreatedAt:     now,
		}
		if err := uc.journal.Append(ctx, event); err != nil {
			continue
		}
		written++
	}
	return written
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// distinctIDs validates the selection and removes repeats.
//
// De-duplication is not cosmetic: the repository enforces all-or-nothing by
// comparing rows affected against the number of ids, so a repeated id would make
// a perfectly valid batch look short and fail.
func distinctIDs(ids []uuid.UUID) ([]uuid.UUID, error) {
	if len(ids) == 0 {
		return nil, domain.NewValidationError("at least one risk ID is required")
	}
	if len(ids) > MaxBulkActionItems {
		return nil, domain.NewValidationError(
			fmt.Sprintf("bulk action is limited to %d items maximum", MaxBulkActionItems))
	}

	seen := make(map[uuid.UUID]struct{}, len(ids))
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if id == uuid.Nil {
			return nil, domain.NewValidationError("a risk ID in the selection is empty")
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out, nil
}

// addTags unions the two sets and returns a SORTED slice.
//
// The order is sorted rather than map-iteration order, which is what the
// previous implementation returned: Go randomises map iteration, so the same
// request produced a different column order on every call and every write looked
// like a change in the trail.
func addTags(existing, added []string) []string {
	set := make(map[string]struct{}, len(existing)+len(added))
	for _, t := range existing {
		set[t] = struct{}{}
	}
	for _, t := range added {
		set[t] = struct{}{}
	}
	return sortedKeys(set)
}

// removeTags subtracts the given tags. A tag the risk does not carry is a no-op,
// never an error — asking to remove a label that is already absent has got the
// outcome the caller wanted.
func removeTags(existing, removed []string) []string {
	drop := make(map[string]struct{}, len(removed))
	for _, t := range removed {
		drop[t] = struct{}{}
	}
	keep := make(map[string]struct{}, len(existing))
	for _, t := range existing {
		if _, gone := drop[t]; gone {
			continue
		}
		keep[t] = struct{}{}
	}
	return sortedKeys(keep)
}

func sortedKeys(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
