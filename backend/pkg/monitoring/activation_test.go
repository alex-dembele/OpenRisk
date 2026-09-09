// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only

package monitoring

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
)

// ahaSampleCount reads the number of observations recorded on one
// aha_definition series. testutil.ToFloat64 only handles counters and gauges, so
// the histogram is read through the dto the collector writes.
func ahaSampleCount(t *testing.T, definition AhaDefinition) uint64 {
	t.Helper()
	obs, err := TimeToAha.GetMetricWithLabelValues(definition)
	if err != nil {
		t.Fatalf("GetMetricWithLabelValues(%q): %v", definition, err)
	}
	metric, ok := obs.(prometheus.Metric)
	if !ok {
		t.Fatalf("observer for %q is not a prometheus.Metric", definition)
	}
	var out dto.Metric
	if err := metric.Write(&out); err != nil {
		t.Fatalf("writing metric for %q: %v", definition, err)
	}
	return out.GetHistogram().GetSampleCount()
}

// D-010: the two definitions measure different journeys. An observation under
// one must never land in the other's series, because SlowTimeToAha reads the
// quantile of v2 alone.
func TestObserveTimeToAha_SeriesAreSeparatePerDefinition(t *testing.T) {
	v1Before := ahaSampleCount(t, AhaDefinitionV1)
	v2Before := ahaSampleCount(t, AhaDefinitionV2)

	ObserveTimeToAha(AhaDefinitionV1, 7*time.Minute)

	if got := ahaSampleCount(t, AhaDefinitionV1) - v1Before; got != 1 {
		t.Errorf("v1 gained %d observations, want 1", got)
	}
	if got := ahaSampleCount(t, AhaDefinitionV2) - v2Before; got != 0 {
		t.Errorf("v2 gained %d observations from a v1 call, want 0", got)
	}

	ObserveTimeToAha(AhaDefinitionV2, 4*time.Minute)
	if got := ahaSampleCount(t, AhaDefinitionV2) - v2Before; got != 1 {
		t.Errorf("v2 gained %d observations, want 1", got)
	}
	if got := ahaSampleCount(t, AhaDefinitionV1) - v1Before; got != 1 {
		t.Errorf("v1 moved to %d observations after a v2 call, want it left at 1", got)
	}
}

// A missing signup anchor or a skewed clock must not flatter the P50, and an
// unlabelled observation must not create a third series nothing queries.
func TestObserveTimeToAha_DropsDishonestObservations(t *testing.T) {
	before := ahaSampleCount(t, AhaDefinitionV1)

	ObserveTimeToAha(AhaDefinitionV1, 0)
	ObserveTimeToAha(AhaDefinitionV1, -3*time.Minute)
	ObserveTimeToAha("", 5*time.Minute)

	if got := ahaSampleCount(t, AhaDefinitionV1) - before; got != 0 {
		t.Errorf("dropped observations still landed: +%d", got)
	}
	if n := testutil.CollectAndCount(TimeToAha); n > 2 {
		t.Errorf("time_to_aha_seconds has %d series; only v1 and v2 may exist", n)
	}
}

// The defect D-010 tells this work not to inherit: the increment used to live
// inside ObserveTimeToAha, so a tenant with no signup anchor reached the Aha
// without ever being counted.
func TestCountAhaReached_IsIndependentOfTheObservation(t *testing.T) {
	before := testutil.ToFloat64(AhaReachedTotal)

	CountAhaReached()
	if got := testutil.ToFloat64(AhaReachedTotal) - before; got != 1 {
		t.Fatalf("CountAhaReached() moved the counter by %v, want 1", got)
	}

	ObserveTimeToAha(AhaDefinitionV1, 6*time.Minute)
	if got := testutil.ToFloat64(AhaReachedTotal) - before; got != 1 {
		t.Errorf("observing a duration moved the counter by %v; it must count nothing", got)
	}
}
