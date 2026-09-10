// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package monitoring

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Activation metrics (spec §7 "Aha moment + métrique").
//
// These are package-level and registered on the DEFAULT registry at import time,
// deliberately: MetricsCollector is constructed explicitly and, at the time of
// writing, by nobody, so a metric that only exists once someone remembers to
// build the collector is a metric that is not there when the alert fires.
// Importing this package is the only requirement.
//
// The alert lives in deployment/monitoring/alert-rules.yml (SlowTimeToAha,
// P50 > 12 min). The buckets below are chosen around that threshold so the
// quantile is interpolated inside a narrow bucket rather than across a 10-minute
// gap: a histogram whose bucket edges straddle the SLO cannot measure the SLO.
// AhaDefinition is the value of the `aha_definition` label carried by
// openrisk_time_to_aha_seconds. It exists because the product moved the Aha
// moment (D-010): the two definitions measure different journeys and their
// durations are NOT comparable, so splicing them into one series would make the
// P50 behind SlowTimeToAha lie on the day of the switch.
//
// Rule: never add an observation to a definition that is frozen, and never
// reuse a value for a third definition. A new definition gets a new value.
type AhaDefinition = string

const (
	// AhaDefinitionV1 is the executive-dashboard definition: the first cyber
	// score computed on the tenant's own data while at least one compliance gap
	// is identified (internal/application/activation/aha.go). It FREEZES when
	// the Posture Reveal ships — v1 observations stop, the recorded history
	// stays queryable.
	AhaDefinitionV1 AhaDefinition = "v1"

	// AhaDefinitionV2 is the Posture Reveal definition: the posture summary
	// computed and shown to the user (`posture.revealed`). It starts empty; a
	// histogram with no observations has no quantile, so SlowTimeToAha simply
	// does not fire until the first tenant reaches it.
	AhaDefinitionV2 AhaDefinition = "v2"
)

var (
	// TimeToAha measures signup → first Aha, in seconds, PER DEFINITION. "Aha" is
	// defined by the product, not by this package; see AhaDefinition for why the
	// label is not optional.
	TimeToAha = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "openrisk",
		Name:      "time_to_aha_seconds",
		Help:      "Seconds between signup and the first Aha moment, by Aha definition (v1: first cyber score on the tenant's own data with a compliance gap; v2: posture reveal).",
		Buckets: []float64{
			60,    // 1 min
			120,   // 2 min
			240,   // 4 min
			360,   // 6 min
			480,   // 8 min — the product target is "under 8 minutes"
			600,   // 10 min
			720,   // 12 min — the alert threshold
			900,   // 15 min
			1800,  // 30 min
			3600,  // 1 h
			21600, // 6 h
			86400, // 1 day: came back another day to find value
		},
	}, []string{"aha_definition"})

	// ActivationEventsTotal counts activation signals by key. Labelled by key
	// only — never by tenant: tenant labels are unbounded cardinality, and the
	// funnel question ("how many tenants import a framework?") is answered by
	// the ratio between keys, not by per-tenant series.
	ActivationEventsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "openrisk",
		Name:      "activation_events_total",
		Help:      "Activation events recorded, by event key.",
	}, []string{"event_key"})

	// AhaReachedTotal counts tenants that reached the Aha moment. The ratio to
	// signups is the activation rate.
	//
	// Counted by CountAhaReached, which is deliberately NOT called from
	// ObserveTimeToAha: a tenant with no signup anchor has no honest duration to
	// observe but has still reached the Aha, and burying the increment inside the
	// observation dropped exactly those tenants from the numerator of the
	// activation rate (D-010).
	//
	// Left UNLABELLED on purpose while the histogram gains one: this counter is
	// the numerator of NoActivationDespiteSignups, and a CounterVec has no series
	// at all until its first increment, which would leave that alert with an
	// empty right-hand side and silently stop it firing.
	AhaReachedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "openrisk",
		Name:      "aha_reached_total",
		Help:      "Number of tenants that reached the Aha moment for the first time.",
	})
)

// ObserveTimeToAha records one signup → Aha duration under the given definition.
// A non-positive duration is dropped: a clock skew or a missing signup anchor
// would otherwise report an impossibly fast activation and quietly flatter the
// P50 the alert watches. An empty definition is dropped too — an unlabelled
// observation would silently create a third series nothing queries.
//
// It does NOT count the tenant: see AhaReachedTotal and CountAhaReached.
func ObserveTimeToAha(definition AhaDefinition, d time.Duration) {
	if definition == "" || d <= 0 {
		return
	}
	TimeToAha.WithLabelValues(definition).Observe(d.Seconds())
}

// CountAhaReached counts one tenant reaching the Aha moment. Called at the
// moment the event is recorded, whether or not a signup anchor exists to measure
// a duration against.
func CountAhaReached() {
	AhaReachedTotal.Inc()
}

// RecordActivationEvent counts one activation signal.
func RecordActivationEvent(eventKey string) {
	if eventKey == "" {
		return
	}
	ActivationEventsTotal.WithLabelValues(eventKey).Inc()
}
