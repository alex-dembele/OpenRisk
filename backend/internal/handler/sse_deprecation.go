// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package handler

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
)

// Deprecation signalling for the three legacy SSE endpoints (#347).
//
// All three predate the shared realtime hub and none of them is consumed by
// this repository any more:
//
//	/mitigations/events        → GET /realtime/events, filtered on mitigation.*
//	/scanner/events            → GET /realtime/events, filtered on scan.*
//	/reports/:reportId/progress → GET /realtime/events, filtered on report.*
//
// They are announced rather than deleted because a self-hosted deployment may
// have scripts this repository cannot see. None of the three appears in
// docs/openapi.yaml, so nothing was ever promised about them in the published
// contract — which is why one release is a sufficient window rather than the
// several a contract endpoint would deserve.
//
// /mitigations/events is the one that matters. It accepts the access token as a
// QUERY PARAMETER, because EventSource cannot set an Authorization header, and a
// credential in a URL lands in access logs, proxy logs, browser history and any
// Referer that leaks. Its replacement carries the session in an HttpOnly cookie.

const (
	// sseSunsetDate is when these endpoints are removed: the 1.2.0 release.
	//
	// A date rather than a version because that is what RFC 8594 puts on the
	// wire, and a client cannot resolve "the next release" from a header. It is
	// approximately ninety days out — long enough for an operator to notice a
	// Deprecation header in a log and act on it, short enough that a credential
	// in a URL is not tolerated for a year.
	//
	// Change it here and every endpoint follows; the value is asserted by a test
	// so it cannot drift from the documentation.
	sseSunsetDate = "2026-12-02T00:00:00Z"

	// sseDeprecationDoc is where the replacement and the window are written out.
	sseDeprecationDoc = "https://github.com/opendefender/OpenRisk/blob/master/docs/API_REFERENCE.md#deprecated-endpoints"
)

// markSSEDeprecated writes the RFC 8594 announcement onto a legacy stream.
//
// `successor` is the path that replaces this endpoint. It is a required argument
// rather than a constant because a deprecation notice that does not say what to
// use instead is a complaint, not a migration path.
//
// Called before the stream body starts: headers cannot be set once the first
// byte is written, and these endpoints hold their response open indefinitely.
func markSSEDeprecated(c *fiber.Ctx, successor string) {
	// RFC 8594 §2. "true" rather than a date: the endpoints were deprecated
	// before anyone wrote down when, and inventing a past date would be a claim
	// about history rather than a fact.
	c.Set("Deprecation", "true")

	// RFC 8594 §3 — an HTTP-date, in IMF-fixdate form.
	if t, err := time.Parse(time.RFC3339, sseSunsetDate); err == nil {
		c.Set("Sunset", t.UTC().Format(http1Date))
	}

	// RFC 8288 relations: where to go next, and where it is written down.
	c.Set("Link", fmt.Sprintf(`<%s>; rel="successor-version", <%s>; rel="deprecation"`,
		successor, sseDeprecationDoc))
}

// http1Date is the IMF-fixdate layout RFC 9110 requires of an HTTP-date. Go's
// http.TimeFormat is the same string; spelling it out avoids pulling net/http
// into a Fiber handler for one constant.
const http1Date = "Mon, 02 Jan 2006 15:04:05 GMT"
