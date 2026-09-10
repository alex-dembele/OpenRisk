// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package activation

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/opendefender/openrisk/internal/domain"
	"github.com/opendefender/openrisk/pkg/onboarding"
)

// ---------------------------------------------------------------------------
// Step 2 of the guided tunnel (#438): eight pre-written statements, the user
// picks three, and three REAL ROWS appear in their register.
//
// This is the most dangerous write in the whole issue and the code is shaped
// around that. These rows land in a customer's risk register, so:
//
//   • NOTHING the client sends becomes content. The request carries catalogue
//     KEYS; the statement is re-read from `pkg/onboarding` server-side. A client
//     that could post a title and a description would be an unvalidated write
//     into a GRC register — content the customer did not author, attributed to
//     them, which is a credibility loss no support ticket recovers.
//   • Provenance is recorded twice: `Source = "starter"` (indexed, so the rows
//     are queryable as a set) and `ExternalID = "starter:<key>"` (so a human
//     reading one row can trace it to the catalogue entry).
//   • Adoption is IDEMPOTENT. The tunnel is resumable and back-navigable by
//     design, so a user will return to step 2; without the guard, every return
//     visit would double the register.
// ---------------------------------------------------------------------------

// StarterRisksUseCase serves and adopts the starter catalogue.
type StarterRisksUseCase struct {
	repo   domain.OnboardingRepository
	writer domain.StarterRiskWriter
}

// NewStarterRisksUseCase builds the use case. Both dependencies are required:
// unlike the rest of this package there is no useful degraded mode — a screen
// that offers statements it cannot write is worse than no screen.
func NewStarterRisksUseCase(repo domain.OnboardingRepository, writer domain.StarterRiskWriter) *StarterRisksUseCase {
	return &StarterRisksUseCase{repo: repo, writer: writer}
}

// StarterRiskOffer is the payload of GET /onboarding/starter-risks.
type StarterRiskOffer struct {
	Risks []onboarding.StarterRisk `json:"risks"`
	// Pick is how many the user must select. Sent rather than hard-coded in the
	// client so the two can never disagree about what "select three" means.
	Pick int `json:"pick"`
	// Industry and Country say which answers produced this set, so the screen can
	// explain itself instead of presenting eight statements from nowhere.
	Industry string `json:"industry,omitempty"`
	Country  string `json:"country,omitempty"`
	// AlreadyAdopted is true when this tenant has starter rows already. The
	// screen then shows the selection as done rather than inviting a second
	// adoption the server would refuse.
	AlreadyAdopted bool `json:"already_adopted"`
}

// List returns the eight statements for this user's stored sector and country.
//
// Typed errors: ErrForbidden without a tenant or a user. There is deliberately
// no ErrNotFound — `StarterRisksFor` always returns eight, falling back to the
// "other" sector, because step 2 has no empty state that makes sense: asking a
// user to pick three of nothing is a broken screen, not an empty one.
func (uc *StarterRisksUseCase) List(ctx context.Context, tenantID, userID uuid.UUID) (*StarterRiskOffer, error) {
	if tenantID == uuid.Nil || userID == uuid.Nil {
		return nil, domain.NewForbiddenError("a tenant and a user are required to read starter risks")
	}

	industry, country := uc.answers(ctx, tenantID, userID)

	offer := &StarterRiskOffer{
		Risks:    onboarding.StarterRisksFor(industry, country),
		Pick:     onboarding.StarterRiskPickCount,
		Industry: industry,
		Country:  country,
	}
	if uc.writer != nil {
		// A read failure here must not hide the statements: the worst case is a
		// screen that offers a second adoption the server then refuses with a
		// clear conflict, which is far better than an empty step.
		if adopted, err := uc.writer.HasStarterRisks(ctx, tenantID); err == nil {
			offer.AlreadyAdopted = adopted
		}
	}
	return offer, nil
}

// AdoptResult is the payload of POST /onboarding/starter-risks.
type AdoptResult struct {
	Created []uuid.UUID `json:"created"`
	Keys    []string    `json:"keys"`
}

// Adopt writes the chosen statements as real risks.
//
// Typed errors, each meaning something different to the client:
//
//   - ErrForbidden — no tenant or no user on the session.
//   - ErrValidation — the wrong number of keys, or a key that is not in the
//     catalogue. An unknown key is REFUSED rather than skipped: silently
//     writing two rows when three were asked for would leave the user counting.
//   - ErrConflict — this tenant already adopted. Idempotence with a name, so the
//     client can say "already done" instead of retrying into a duplicate.
func (uc *StarterRisksUseCase) Adopt(ctx context.Context, tenantID, userID uuid.UUID, keys []string, lang string) (*AdoptResult, error) {
	if tenantID == uuid.Nil || userID == uuid.Nil {
		return nil, domain.NewForbiddenError("a tenant and a user are required to adopt starter risks")
	}
	if uc.writer == nil {
		return nil, domain.NewInternalError("starter risk writer is not wired")
	}

	if len(keys) != onboarding.StarterRiskPickCount {
		return nil, domain.NewValidationError("exactly three starter risks must be selected")
	}

	writeLang := normaliseLang(lang)

	// Resolve EVERY key before writing ANY row. A half-adopted register — two
	// rows written, the third key rejected — is worse than a refusal, and the
	// repository has no transaction that spans three separate use-case calls.
	drafts := make([]domain.StarterRiskDraft, 0, len(keys))
	seen := map[string]bool{}
	for _, key := range keys {
		if seen[key] {
			return nil, domain.NewValidationError("the same starter risk was selected twice: " + key)
		}
		seen[key] = true

		statement, ok := onboarding.StarterRiskByKey(key)
		if !ok {
			return nil, domain.NewValidationError("unknown starter risk: " + key)
		}

		drafts = append(drafts, domain.StarterRiskDraft{
			StarterKey:  statement.Key,
			Title:       statement.Title(writeLang),
			Description: statement.Description(writeLang),
			Probability: statement.Probability,
			Impact:      statement.Impact,
			Tags:        statement.Tags,
			CreatedBy:   userID,
		})
	}

	// The idempotence guard sits AFTER validation so a malformed second attempt
	// still reports what is wrong with it, and BEFORE the first write so a valid
	// second attempt cannot double the register.
	adopted, err := uc.writer.HasStarterRisks(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if adopted {
		return nil, domain.NewConflictError("starter risks", "tenant")
	}

	result := &AdoptResult{
		Created: make([]uuid.UUID, 0, len(drafts)),
		Keys:    make([]string, 0, len(drafts)),
	}
	for _, draft := range drafts {
		risk, err := uc.writer.CreateStarterRisk(ctx, tenantID, draft)
		if err != nil {
			// Partial failure is surfaced, not swallowed. The rows already
			// written stay — they are real risks the user chose — and the error
			// tells the client which state it is in rather than pretending the
			// whole thing failed.
			return result, err
		}
		result.Created = append(result.Created, risk.ID)
		result.Keys = append(result.Keys, draft.StarterKey)
	}
	return result, nil
}

// answers reads the sector and country the user gave in step 1.
func (uc *StarterRisksUseCase) answers(ctx context.Context, tenantID, userID uuid.UUID) (industry, country string) {
	if uc.repo == nil {
		return "", ""
	}
	progress, err := uc.repo.Get(ctx, tenantID, userID)
	if err != nil || progress == nil {
		return "", ""
	}
	return progress.Industry, progress.Country
}

// normaliseLang resolves the language the rows are WRITTEN in.
//
// This used to read a `language` answer off the `profile` step. #438 retired
// that route, and nothing writes that answer any more — so every tenant, English
// ones included, got French statements written into their risk register. Rows a
// customer did not author, in a language their colleagues may not read, is the
// precise failure ADR 0003 and this file's header set out to avoid.
//
// The language now comes from the caller, because it is a PREFERENCE and not
// CONTENT: it selects which of two server-authored strings to store, and it
// cannot introduce text of the client's own. Anything unrecognised falls back to
// French — the catalogue's source language, so an unknown value stores the
// statement as authored rather than an accidental half-translation.
func normaliseLang(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "en":
		return "en"
	case "fr":
		return "fr"
	default:
		return "fr"
	}
}
