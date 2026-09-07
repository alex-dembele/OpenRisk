// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apprealtime "github.com/opendefender/openrisk/internal/application/realtime"
	rt "github.com/opendefender/openrisk/pkg/realtime"
)

// #347 — the three legacy SSE endpoints announce their own retirement.
//
// A deprecation nobody can observe is a comment. These assert the RFC 8594
// headers reach the wire, that each one names its replacement, and that the
// sunset date the code emits is the date the documentation promises.

func TestMarkSSEDeprecated_AnnouncesTheRetirement(t *testing.T) {
	app := fiber.New()
	app.Get("/legacy", func(c *fiber.Ctx) error {
		markSSEDeprecated(c, "/api/v1/realtime/events?aggregates=mitigation")
		return c.SendString("ok")
	})

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/legacy", nil))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, "true", resp.Header.Get("Deprecation"),
		"RFC 8594 §2 — the endpoint says it is deprecated")

	sunset := resp.Header.Get("Sunset")
	require.NotEmpty(t, sunset, "RFC 8594 §3 — a client must be able to read the date")
	parsed, err := time.Parse(http1Date, sunset)
	require.NoError(t, err, "Sunset must be an IMF-fixdate, not an arbitrary string")

	// The header and the constant must be the same instant, or the docs and the
	// wire disagree about when the endpoint disappears.
	expected, err := time.Parse(time.RFC3339, sseSunsetDate)
	require.NoError(t, err)
	assert.True(t, parsed.Equal(expected), "the emitted sunset must match sseSunsetDate")

	link := resp.Header.Get("Link")
	assert.Contains(t, link, `rel="successor-version"`,
		"a deprecation that does not name its replacement is a complaint, not a migration path")
	assert.Contains(t, link, "/api/v1/realtime/events?aggregates=mitigation")
	assert.Contains(t, link, `rel="deprecation"`, "and where it is written down")
}

// The sunset must be in the future when this ships. A date that has already
// passed announces a removal that did not happen, which is worse than silence.
func TestSSESunsetDate_IsInTheFuture(t *testing.T) {
	sunset, err := time.Parse(time.RFC3339, sseSunsetDate)
	require.NoError(t, err)
	assert.True(t, sunset.After(time.Now()),
		"the sunset date has passed — either remove the endpoints or move the date")
}

// Each endpoint must point at its OWN replacement. A single shared successor
// would send a scanner client to the mitigation filter.
func TestMarkSSEDeprecated_EachEndpointNamesItsOwnSuccessor(t *testing.T) {
	cases := map[string]string{
		"mitigations": "/api/v1/realtime/events?aggregates=mitigation",
		"scanner":     "/api/v1/scanner/jobs",
		"reports":     "/api/v1/reports/:reportId",
	}

	for name, successor := range cases {
		t.Run(name, func(t *testing.T) {
			app := fiber.New()
			app.Get("/legacy", func(c *fiber.Ctx) error {
				markSSEDeprecated(c, successor)
				return c.SendString("ok")
			})

			resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/legacy", nil))
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Contains(t, resp.Header.Get("Link"), successor)
			// and not one of the others
			for other, otherSuccessor := range cases {
				if other == name {
					continue
				}
				assert.NotContains(t, resp.Header.Get("Link"), otherSuccessor)
			}
		})
	}
}

// The documentation and the code must agree on the date. If someone edits one,
// this fails rather than letting the wire and the reference drift apart.
func TestSSESunsetDate_MatchesTheAPIReference(t *testing.T) {
	doc, err := readRepoFile("docs/API_REFERENCE.md")
	if err != nil {
		t.Skipf("API reference not readable from this working directory: %v", err)
	}

	sunset, err := time.Parse(time.RFC3339, sseSunsetDate)
	require.NoError(t, err)
	formatted := sunset.UTC().Format(http1Date)

	assert.Contains(t, doc, formatted,
		"docs/API_REFERENCE.md must quote the same Sunset date the endpoints emit")
	assert.Contains(t, doc, "Deprecated endpoints",
		"the reference must carry the deprecation section the Link header points at")

	// Every replacement named in code must appear in the reference.
	for _, successor := range []string{
		"/realtime/events?aggregates=mitigation",
		"/scanner/jobs",
		"/reports/{reportId}",
	} {
		assert.True(t, strings.Contains(doc, successor),
			"the reference must document the replacement for %s", successor)
	}
}

// readRepoFile reads a path relative to the repository root, which is three
// levels up from this package.
func readRepoFile(rel string) (string, error) {
	b, err := os.ReadFile(filepath.Join("..", "..", "..", rel))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ===========================================================================
// The migration path itself, end to end (#347 acceptance criterion 3).
//
// The Link header on /mitigations/events promises
// /realtime/events?aggregates=mitigation. A promise nobody exercised is how a
// deprecation strands its consumers on the sunset date. This proves the
// replacement really carries the event the legacy stream carried — the same
// mitigation.auto_completed the scanner publishes.
// ===========================================================================

func TestDeprecation_MitigationSuccessorPathDeliversTheEvent(t *testing.T) {
	e := newStreamTestEnv(t, apprealtime.HubOptions{})
	tenant := uuid.New()

	// The exact query string the Link header names.
	stream, status := e.openStream(t, tenantHeaders(tenant), "aggregates=mitigation")
	require.Equal(t, http.StatusOK, status)
	stream.awaitHello(t)

	// Anything else stays out, so the filter is doing work rather than passing
	// everything through.
	e.publish(t, tenant, rt.RiskCreated, "risk-1")
	stream.expectNothing(t, 500*time.Millisecond)

	wanted := e.publish(t, tenant, rt.MitigationAutoCompleted, "plan-1")
	got := stream.next(t, 3*time.Second)
	assert.Equal(t, string(rt.MitigationAutoCompleted), got.Event,
		"the successor must deliver the event the deprecated stream delivered")
	assert.Equal(t, wanted.ID, got.Data["id"])
}

// The successor named in the header must be a filter the stream ACCEPTS. An
// unknown aggregate is refused with 400, so a typo in the Link header would
// hand a migrating client a broken URL.
func TestDeprecation_MitigationSuccessorFilterIsAccepted(t *testing.T) {
	e := newStreamTestEnv(t, apprealtime.HubOptions{})
	tenant := uuid.New()

	_, status := e.openStream(t, tenantHeaders(tenant), "aggregates=mitigation")
	assert.Equal(t, http.StatusOK, status,
		"the aggregate named in the Link header must be a valid filter token")
}
