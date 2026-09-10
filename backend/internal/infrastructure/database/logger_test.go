// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package database

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

// trace drives the logger the way GORM does and returns what it wrote.
func trace(t *testing.T, env string, err error) string {
	t.Helper()
	t.Setenv("APP_ENV", env)
	var buf bytes.Buffer
	l := gormLoggerTo(&buf)
	l.Trace(context.Background(), time.Now(),
		func() (string, int64) { return `SELECT * FROM "reports" WHERE run_state = 'queued'`, 0 },
		err)
	return buf.String()
}

// A "record not found" is an answer, not a failure. The report worker polls an
// empty queue on an interval and handles it; before #615 GORM reported every one
// of them AS AN ERROR, with a file and line, so an idle backend looked broken.
//
// What must never appear is the error. In development the statement itself is
// still traced — that is what logger.Info is for, and it is not the complaint.
func TestGormLogger_RecordNotFoundIsNeverAnError(t *testing.T) {
	for _, env := range []string{"", "production"} {
		out := trace(t, env, gorm.ErrRecordNotFound)
		if strings.Contains(strings.ToLower(out), "record not found") {
			t.Errorf("APP_ENV=%q: an empty queue must not be reported as an error, got:\n%s", env, out)
		}
	}
}

// In production it is not merely not-an-error: it is not logged at all, so
// polling an idle queue writes nothing whatsoever.
func TestGormLogger_ProductionIsSilentOnAnEmptyQueue(t *testing.T) {
	if out := trace(t, "production", gorm.ErrRecordNotFound); out != "" {
		t.Errorf("production must log nothing for an empty queue, got:\n%s", out)
	}
}

// Production must not print every statement and its parameter values.
func TestGormLogger_ProductionDoesNotTraceStatements(t *testing.T) {
	if out := trace(t, "production", nil); out != "" {
		t.Errorf("production must not log a successful query, got:\n%s", out)
	}
}

// Development keeps the full trace — that is where SQL gets read.
func TestGormLogger_DevelopmentTracesStatements(t *testing.T) {
	out := trace(t, "", nil)
	if !strings.Contains(out, "reports") {
		t.Errorf("development must log the statement, got:\n%q", out)
	}
}

// Quieter must not mean deaf: a real error still reaches the log in production.
func TestGormLogger_ProductionStillLogsRealErrors(t *testing.T) {
	out := trace(t, "production", errors.New("connection refused"))
	if !strings.Contains(out, "connection refused") {
		t.Errorf("a real database error must be logged in production, got:\n%q", out)
	}
}
