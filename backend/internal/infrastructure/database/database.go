// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package database

import (
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// gormLogger builds the SQL logger.
//
// Two things it fixes, both of which made a HEALTHY backend look broken (#615):
//
//   - IgnoreRecordNotFoundError. A "record not found" is not a database error,
//     it is an answer: the report worker polling an empty queue
//     (repository.ClaimQueued) gets one every interval and handles it correctly,
//     but GORM logged it, with a file and line, before the application ever saw
//     it. An operator watching `docker compose logs -f` after ./install.sh saw a
//     continuous stream of errors from an idle system.
//
//   - The level. logger.Info logs EVERY statement with its parameter values.
//     That buries anything real — one boot emits hundreds of pg_catalog lines —
//     and puts user and tenant values into a log the operator may ship anywhere,
//     which CLAUDE.md rule 6 does not want. Development keeps the full trace,
//     because that is where you read SQL; production keeps warnings, slow
//     queries and real errors.
//
// SlowThreshold is stated rather than left to GORM's implicit default, so the
// number is a decision someone can argue with.
func gormLogger() logger.Interface { return gormLoggerTo(os.Stdout) }

// gormLoggerTo is gormLogger with the destination injected, so the behaviour
// above can be asserted instead of described.
func gormLoggerTo(w io.Writer) logger.Interface {
	level := logger.Info
	if os.Getenv("APP_ENV") == "production" {
		level = logger.Warn
	}
	return logger.New(
		log.New(w, "", log.LstdFlags),
		logger.Config{
			SlowThreshold:             200 * time.Millisecond,
			LogLevel:                  level,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
		},
	)
}

func Connect() {
	// Build DSN from environment variables with sensible defaults
	host := os.Getenv("DB_HOST")
	if host == "" {
		host = "localhost"
	}

	port := os.Getenv("DB_PORT")
	if port == "" {
		port = "5434"
	}

	user := os.Getenv("DB_USER")
	if user == "" {
		user = "openrisk"
	}

	password := os.Getenv("DB_PASSWORD")

	dbname := os.Getenv("DB_NAME")
	if dbname == "" {
		dbname = "openrisk"
	}

	// Try to use DATABASE_URL if provided (takes precedence)
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = fmt.Sprintf(
			"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable TimeZone=UTC",
			host, user, password, dbname, port,
		)
	}

	var err error
	DB, err = gorm.Open(postgres.Open(databaseURL), &gorm.Config{
		Logger: gormLogger(),
		// TranslateError lets repositories detect constraint violations via
		// sentinel errors (e.g. errors.Is(err, gorm.ErrDuplicatedKey))
		// instead of parsing driver-specific error codes.
		TranslateError: true,
		// GORM's AutoMigrate topologically reorders models to satisfy FK constraints,
		// but with 15+ interrelated domain models (some intentionally cyclic, e.g.
		// User.DefaultOrg <-> Organization.Owner) that ordering isn't reliable and
		// AutoMigrate can crash on a fresh DB. Referential integrity across tenants is
		// already enforced at the application layer (tenant_id filtering on every query,
		// see CLAUDE.md), so DB-level FK constraints aren't load-bearing here.
		DisableForeignKeyConstraintWhenMigrating: true,
	})

	if err != nil {
		log.Fatal("Failed to connect to database! \n", err)
	}

	log.Println("Connected to PostgreSQL database successfully")

	sqlDB, err := DB.DB()
	if err != nil {
		log.Fatal("Failed to get database instance")
	}

	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)
}
