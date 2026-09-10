// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package activation

import (
	"context"

	"github.com/google/uuid"

	"github.com/opendefender/openrisk/internal/domain"
)

// ---------------------------------------------------------------------------
// Recognition (#438 criterion 9) — the screen for the tenant that was already
// configured before any of this shipped.
//
// #234's backfill made their checklist truthful and then stopped. Their
// activation question is not "how do I create a risk"; it is "what does OpenRisk
// already know about us that we did not". Walking a CISO whose bank has two
// hundred risks through a five-step tunnel that asks them to write their first
// one is worse than showing them nothing.
//
// So this surface answers with THEIR OWN NUMBERS and nothing else. Same rule as
// the reveal: no sample, no demo, no placeholder.
// ---------------------------------------------------------------------------

// Read model and port live in internal/domain (domain/posture.go explains why).
type (
	RecognitionCounts = domain.RecognitionCounts
	RecognitionReader = domain.RecognitionReader
)

// Recognition is the payload of GET /onboarding/recognition.
type Recognition struct {
	Counts RecognitionCounts `json:"counts"`
	// Recognised is true when the tenant holds pre-existing data, which is what
	// decides between this screen and the five-step tunnel. The SERVER decides:
	// a client that could choose would be a client that can skip the tunnel.
	Recognised bool `json:"recognised"`
	// SkipTunnel mirrors Recognised for the guard, named for what the client does
	// with it rather than for what it means.
	SkipTunnel bool `json:"skip_tunnel"`
}

// RecognitionUseCase answers "does this tenant already have a posture?".
type RecognitionUseCase struct {
	reader RecognitionReader
}

// NewRecognitionUseCase builds the use case.
func NewRecognitionUseCase(reader RecognitionReader) *RecognitionUseCase {
	return &RecognitionUseCase{reader: reader}
}

// Execute returns the tenant's own counts.
//
// Typed errors: ErrForbidden with no tenant or no user on the session. Unlike
// the reveal there is no ErrNotFound — a tenant holding nothing is a legitimate
// answer here ("you are new, take the tunnel"), not a failure.
func (uc *RecognitionUseCase) Execute(ctx context.Context, tenantID, userID uuid.UUID) (*Recognition, error) {
	if tenantID == uuid.Nil || userID == uuid.Nil {
		return nil, domain.NewForbiddenError("a tenant and a user are required to read a recognition")
	}
	if uc.reader == nil {
		return nil, domain.NewInternalError("recognition reader is not wired")
	}

	counts, err := uc.reader.RecognitionCounts(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	recognised := counts.Any()
	return &Recognition{
		Counts:     counts,
		Recognised: recognised,
		SkipTunnel: recognised,
	}, nil
}
