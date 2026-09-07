// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"

	actioncenterapp "github.com/opendefender/openrisk/internal/application/actioncenter"
	appactivation "github.com/opendefender/openrisk/internal/application/activation"
	appai "github.com/opendefender/openrisk/internal/application/ai"
	assetapp "github.com/opendefender/openrisk/internal/application/asset"
	"github.com/opendefender/openrisk/internal/application/assetschema"
	"github.com/opendefender/openrisk/internal/application/auth"
	appauto "github.com/opendefender/openrisk/internal/application/automation"
	billingapp "github.com/opendefender/openrisk/internal/application/billing"
	"github.com/opendefender/openrisk/internal/application/board"
	"github.com/opendefender/openrisk/internal/application/compliance"
	"github.com/opendefender/openrisk/internal/application/complianceaudit"
	entapp "github.com/opendefender/openrisk/internal/application/entitlements"
	"github.com/opendefender/openrisk/internal/application/entity"
	"github.com/opendefender/openrisk/internal/application/evidence"
	"github.com/opendefender/openrisk/internal/application/governance"
	appinc "github.com/opendefender/openrisk/internal/application/incident"
	"github.com/opendefender/openrisk/internal/application/membership"
	appmitigation "github.com/opendefender/openrisk/internal/application/mitigation"
	notificationapp "github.com/opendefender/openrisk/internal/application/notification"
	"github.com/opendefender/openrisk/internal/application/orgdeletion"
	"github.com/opendefender/openrisk/internal/application/ownership"
	apprealtime "github.com/opendefender/openrisk/internal/application/realtime"
	appreport "github.com/opendefender/openrisk/internal/application/report"
	"github.com/opendefender/openrisk/internal/application/reportjob"
	"github.com/opendefender/openrisk/internal/application/risk"
	savedviewapp "github.com/opendefender/openrisk/internal/application/savedview"
	scanapp "github.com/opendefender/openrisk/internal/application/scanner"
	searchapp "github.com/opendefender/openrisk/internal/application/search"
	vulnapp "github.com/opendefender/openrisk/internal/application/vulnerability"
	coreauth "github.com/opendefender/openrisk/internal/auth"
	"github.com/opendefender/openrisk/internal/config"
	"github.com/opendefender/openrisk/internal/domain"
	handlers "github.com/opendefender/openrisk/internal/handler"
	authhandler "github.com/opendefender/openrisk/internal/handler/auth"
	audittrailinfra "github.com/opendefender/openrisk/internal/infrastructure/audittrail"
	"github.com/opendefender/openrisk/internal/infrastructure/authmail"
	"github.com/opendefender/openrisk/internal/infrastructure/authmfa"
	autoinfra "github.com/opendefender/openrisk/internal/infrastructure/automation"
	"github.com/opendefender/openrisk/internal/infrastructure/ctimatch"
	"github.com/opendefender/openrisk/internal/infrastructure/database"
	"github.com/opendefender/openrisk/internal/infrastructure/demoseed"
	"github.com/opendefender/openrisk/internal/infrastructure/email"
	govinfra "github.com/opendefender/openrisk/internal/infrastructure/governance"
	incinfra "github.com/opendefender/openrisk/internal/infrastructure/incident"
	"github.com/opendefender/openrisk/internal/infrastructure/integrations/thehive"
	orgdeletioninfra "github.com/opendefender/openrisk/internal/infrastructure/orgdeletion"
	"github.com/opendefender/openrisk/internal/infrastructure/orgexport"
	redisclient "github.com/opendefender/openrisk/internal/infrastructure/redis"
	"github.com/opendefender/openrisk/internal/infrastructure/repository"
	"github.com/opendefender/openrisk/internal/infrastructure/scanmitigation"
	"github.com/opendefender/openrisk/internal/infrastructure/vulnrisk"
	"github.com/opendefender/openrisk/internal/infrastructure/workers"
	"github.com/opendefender/openrisk/internal/middleware"
	"github.com/opendefender/openrisk/internal/migrations"
	scanpkg "github.com/opendefender/openrisk/internal/scanner"
	"github.com/opendefender/openrisk/internal/scanner/collectors"
	"github.com/opendefender/openrisk/internal/service"
	"github.com/opendefender/openrisk/pkg/ai"
	authpkg "github.com/opendefender/openrisk/pkg/auth"
	pkgbilling "github.com/opendefender/openrisk/pkg/billing"
	"github.com/opendefender/openrisk/pkg/cache"
	"github.com/opendefender/openrisk/pkg/crq"
	"github.com/opendefender/openrisk/pkg/cti"
	ent "github.com/opendefender/openrisk/pkg/entitlements"
	"github.com/opendefender/openrisk/pkg/hibp"
	"github.com/opendefender/openrisk/pkg/notify"
	"github.com/opendefender/openrisk/pkg/pwpolicy"
	"github.com/opendefender/openrisk/pkg/scoring"
	"github.com/opendefender/openrisk/pkg/storage"
)

// Version and Commit are injected at build time via ldflags
// (-X main.Version=$(cat VERSION) -X main.Commit=$(git rev-parse --short HEAD);
// see Makefile). They are the SINGLE source of truth the /health endpoint
// reports. The defaults mark a non-release build (e.g. `go run`).
var (
	Version = "dev"
	Commit  = "none"
)

func main() {
	// Migration subcommand: `<server> migrate <up|down|force <version>|version>`.
	// Handled FIRST, before config.LoadConfig() (which requires RSA keys and would
	// panic) and before any DB/service init — migrations need only DATABASE_URL.
	// This makes `make migrate`, `make migrate-rollback` and `make migrate-force`
	// real migration operations; previously the binary ignored its arguments and
	// these targets silently booted the whole server instead.
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		if err := migrations.RunCLI(os.Args[2:]); err != nil {
			log.Fatalf("%v", err)
		}
		return
	}

	// =========================================================================
	// 1. CONFIGURATION & INFRASTRUCTURE
	// =========================================================================

	// Chargement de la configuration (.env)
	cfg := config.LoadConfig()
	// if err != nil {
	// 	log.Printf("Warning: No config file found, using environment variables. Error: %v", err)
	// }

	// Initialisation de la Timezone (Important pour les logs/dates)
	time.Local = time.UTC

	// Connexion Base de Données
	database.Connect()

	// =========================================================================
	// 1.5 CACHE INITIALIZATION
	// =========================================================================

	// Initialize Redis cache for performance optimization
	redisHost := os.Getenv("REDIS_HOST")
	if redisHost == "" {
		redisHost = "localhost"
	}
	redisPort := os.Getenv("REDIS_PORT")
	if redisPort == "" {
		redisPort = "6379"
	}
	redisPassword := os.Getenv("REDIS_PASSWORD")

	var cacheInstance interface{}
	var cacheErr error

	// Create Redis cache instance
	redisCache, cacheErr := cache.NewRedisCache(
		redisHost,
		redisPort,
		redisPassword,
	)
	if cacheErr != nil {
		log.Printf("Warning: Redis cache initialization failed: %v. Using in-memory cache.", cacheErr)
		cacheInstance = cache.NewMemoryCache()
	} else {
		log.Println("Cache: Redis initialized successfully")
		cacheInstance = redisCache
	}
	defer func() {
		if c, ok := cacheInstance.(interface{ Close() error }); ok {
			c.Close()
		}
	}()

	// Initialize caching handler utilities - use Redis if available
	var cacheableHandlers *handlers.CacheableHandlers
	if redisCache != nil && cacheErr == nil {
		cacheableHandlers = handlers.NewCacheableHandlers(redisCache)
		log.Println("Cache: Handler utilities initialized with Redis")
	} else {
		// Left nil ON PURPOSE: *CacheableHandlers is nil-safe (see its doc), so
		// every wrapped route serves its handler directly. This used to be a nil
		// that got dereferenced during route registration, which meant an
		// unreachable Redis panicked the server at boot — caching is an
		// optimisation, and losing it must cost latency, not availability.
		log.Println("Cache: Redis unavailable — responses will not be cached (the server runs normally)")
	}

	// =========================================================================
	// 2. MIGRATIONS & SEEDING (DevOps Friendly)
	// =========================================================================

	// Data repairs AutoMigrate cannot do for itself. It creates the indexes the
	// model declares but never touches rows, so a model that gains a UNIQUE
	// index over an already-populated column makes the whole boot fail against
	// any database with existing data. Idempotent; safe on a fresh database.
	if err := database.PrepareForAutoMigrate(database.DB); err != nil {
		log.Fatalf("Database preparation failed: %v", err)
	}

	log.Println("Database: Running Auto-Migrations...")
	if err := database.DB.AutoMigrate(
		&domain.User{},
		&domain.Organization{},
		&domain.OrganizationMember{},
		// Organization invitations (W0-04, migration 0058). The previous
		// invitations model was never in AutoMigrate, so the table never existed
		// and the invitation code that referenced it could not have run.
		&domain.Invitation{},
		&coreauth.RefreshToken{},
		// L2/L4/L5/L7 auth tables. Previously absent from AutoMigrate, so the whole
		// feature set (MFA setup/challenge, PAT auth, SSO account linking, and the
		// full-fidelity auth audit trail) errored on non-existent tables.
		&domain.MFASecret{},
		&domain.MFABackupCode{},
		// OR26-03 — how long a privileged member of this tenant may defer
		// enrolment. One row per tenant; an absent row means the 7-day default.
		&domain.MFAPolicy{},
		&domain.PersonalAccessToken{},
		&domain.OAuthProvider{},
		&domain.AuthAuditLog{},
		// Password reset (migration 0042). Rows are written for every request,
		// including ones for addresses with no account — see the migration for why
		// that is what keeps the rate limiter from leaking account existence.
		&domain.PasswordResetToken{},
		&domain.Risk{},
		// Risk taxonomy (spec §3): the tenant's CONTROLLED category vocabulary and
		// the risk↔compliance-control references. tags need no table — they are a
		// free array on the risk, which is the whole point of the distinction.
		&domain.RiskCategory{},
		&domain.RiskControlMapping{},
		// Smart Risk Calculation (spec §8) — per-tenant configurable weights for the
		// eight-factor multifactor score. The smart-score columns themselves live on
		// domain.Risk above. Additive; the classic Score Engine is untouched.
		&domain.RiskScoringWeights{},
		&domain.Mitigation{},
		&domain.Asset{},
		&domain.AssetSnapshot{},
		// Typed attributes by asset category (Attack Surface §1). One row per
		// (tenant, category) holding the tenant-editable schema that the asset
		// form is generated from and every asset write is validated against.
		&domain.AssetTypeSchema{},
		// The tenant's vulnerability→risk rule (Attack Surface §4). One row per
		// tenant; disabled until they configure it, because automatic risk
		// creation writes to somebody's register.
		&domain.VulnRiskRule{},
		// Directed edges of the asset dependency graph ("cartographie des
		// dépendances"). Tenant-scoped; both endpoints reference assets.
		&domain.AssetDependency{},
		// Cross-framework control crosswalks: how much of a newly imported
		// framework a tenant's existing proof already answers (migration 0053).
		// Tenant-scoped undirected links between two compliance controls.
		&domain.ControlCrosswalk{},
		&domain.RiskHistory{},
		&domain.CustomField{},
		&domain.CustomFieldTemplate{},
		&domain.BulkOperation{},
		&domain.BulkOperationLog{},
		&domain.Team{},
		&domain.TeamMember{},
		// domain.Connector / MarketplaceApp / ConnectorUpdate / MarketplaceLog are intentionally
		// excluded: they carry only `json:` tags (no `gorm:` tags, no primary key), so AutoMigrate
		// fatally errors on them ("unsupported data type"). Marketplace is a pre-existing partial
		// module (see ROADMAP.md) — needs real GORM tagging before it can be added back here.
		&domain.AdminAuditEvent{},
		// M4 (second half) — monthly board-of-directors report (draft → approved),
		// with a per-tenant posture snapshot and an editable AI/template narrative.
		&domain.BoardReport{},
		&domain.ReportJob{},
		// The reporting engine (migration 0054): asynchronous generation, three
		// formats, versioned templates, an integrity hash and an editorial
		// lifecycle. ReportJob above is the earlier synchronous compliance-only
		// job, kept until its route is removed.
		&domain.Report{},
		&domain.ReportComment{},
		// RBAC + audit + multi-tenant tables. These back the Settings admin tabs
		// (Roles / Organizations / Audit log). RoleEnhanced maps onto the existing
		// "roles" table and only ADDS columns (tenant_id/level/is_predefined/...);
		// it never drops the legacy Role columns. Seeded by SeedRBAC() below.
		&domain.PermissionDB{},
		&domain.RoleEnhanced{},
		&domain.RolePermission{},
		&domain.Tenant{},
		&domain.UserTenant{},
		&domain.AuditLog{},
		// M5 (Incident Management) — the incident register + its timeline and
		// mitigation actions. Previously missing here, so every /incidents route
		// 500'd on a non-existent table; War Room stayed a fixture-only preview.
		&domain.Incident{},
		&domain.IncidentTimeline{},
		&domain.IncidentAction{},
		// Scanner engine — tenant-scoped scan configs, on-prem Agents, and scan
		// jobs. The pipeline never writes Assets/Risks itself: results land in a
		// Redis preview (48h TTL) and the user imports/ignores from there.
		&domain.ScanConfig{},
		&domain.ScannerAgent{},
		&domain.ScanJob{},
		// CTI / Intel Threat — vulnerabilities pulled from NVD + CISA KEV, enriched
		// with MITRE ATT&CK. Matched against asset CPEs to auto-create risks.
		&cti.CTIVulnerability{},
		// Vulnerability Management (Module 3) — the tenant-scoped vulnerability
		// register: findings normalised from Nessus/OpenVAS/Qualys/Defender/
		// Inspector/Azure Defender/CrowdStrike and risk-based prioritised.
		&domain.Vulnerability{},
		// Vulnerability integrations — per-source connector config (encrypted API
		// credentials, live-pull schedule, inbound webhook token, automation
		// toggles) + tenant ITSM/ticketing config for auto-ticketing.
		&domain.VulnIntegration{},
		&domain.VulnTicketingConfig{},
		// Notifications — the in-app centre + delivery preferences. Previously
		// missing from AutoMigrate, so every /notifications route errored on a
		// non-existent table (and the scan-completion in-app notification had
		// nowhere to land). Metadata is jsonb; it stays NULL unless a producer
		// sets it (a bare map[string]interface{} has no driver.Valuer).
		&domain.Notification{},
		&domain.NotificationPreference{},
		// The compliance register itself. Created by migration 0028 and, until
		// now, absent from AutoMigrate — so a column added to the model never
		// reached a database where migrations are blocked, and every write failed
		// with "column does not exist". Listed here so model and schema stay in
		// step, which is how every other module in this file works.
		&domain.ComplianceFramework{},
		&domain.ComplianceControl{},
		// Compliance audits ("Audits" — plan/execute/history) and remediation
		// plans ("Plans de remédiation" — close a gap, assign, track). Tenant-scoped.
		&domain.ComplianceAudit{},
		&domain.RemediationPlan{},
		// Evidence library (migration 0052): one proof artifact answering N
		// controls, with a collection date and an expiry. Replaces the read path
		// of control_evidences, which stays in place, backfilled from, for a
		// release.
		&domain.Evidence{},
		&domain.EvidenceControlLink{},
		// Security Automation / SOAR (spec §10 « Automatisation »): tenant-scoped
		// playbooks (trigger + conditions + action chain + SLA policy), their
		// execution audit trail, and the live SLA countdowns the monitor escalates.
		&domain.AutomationRule{},
		&domain.AutomationExecution{},
		&domain.SLATracker{},
		&domain.AutomationChannelConfig{},
		// Governance (spec §15 « Gouvernance »): the immutable audit trail
		// (append-only who/what/when/before→after), time-boxed delegations, and
		// the configurable Maker-Checker approval engine (workflows + requests).
		&domain.IncidentPostMortem{},
		&domain.AuditEvent{},
		&domain.AuditChainSeal{},
		&domain.AuditRetentionPolicy{},
		&domain.Delegation{},
		&domain.ApprovalWorkflow{},
		&domain.ApprovalRequest{},
		// Activation & onboarding (migration 0043). The server-side source of
		// truth for the newcomer journey: an append-only event log written by the
		// domain use cases, a per-user celebration ledger that makes the burst
		// idempotent, and the resumable signup wizard state the route guard reads.
		&domain.ActivationEvent{},
		&domain.ActivationCelebration{},
		&domain.OnboardingProgress{},
		// Open-core commercialisation: the per-tenant subscription + mirrored
		// invoices behind the plan gates, instance-level opt-in telemetry consent,
		// and the danger-zone organization-erasure record (30-day cancelable grace).
		&domain.Subscription{},
		&domain.Invoice{},
		&domain.TelemetryConfig{},
		&domain.OrgDeletionRequest{},
		// Realtime event hub (W0-07, migration 0059): the durable, per-tenant
		// ordered log behind GET /realtime/events. It is what lets a client that
		// reconnects replay what it missed instead of refetching everything, and
		// it is where the ordering guarantee lives (unique tenant_id+sequence).
		&domain.RealtimeEvent{},
		// Saved table views (#580): the named filter/sort/column combinations a
		// user keeps for a register and may share with their tenant. Purely
		// additive — a new table, no column dropped or renamed — which is why it
		// arrives through AutoMigrate alone and carries no .sql file (see
		// internal/migrations/migrator.go:64-66 on which layer owns the schema).
		&domain.SavedView{},
	); err != nil {
		log.Fatalf("Database Migration Failed: %v", err)
	}

	// Run the additive SQL migration layer (if DATABASE_URL is set), on top of the
	// schema AutoMigrate has just built. Fail loudly: a real migration error must
	// abort the boot rather than let the app serve on a half-applied schema.
	if err := migrations.RunMigrations(); err != nil {
		log.Fatalf("SQL migrations failed: %v", err)
	}

	// Governance audit trail (spec §15): install the GORM plugin AFTER the tables
	// exist. From here on, every struct-form mutation of an Auditable model
	// (Asset, ComplianceControl, …) is journaled automatically into audit_events —
	// developers can't forget to log. Best-effort: a failure never blocks a write.
	//
	// The plugin appends through the CHAINED store: entries written outside an
	// HTTP request (workers, schedulers) still get a sequence number and a hash
	// linking them to the previous entry, so chain verification covers them too.
	auditChainRepo := repository.NewGormAuditChainRepository(database.DB)
	if err := database.DB.Use(audittrailinfra.New(database.DB).WithAppender(auditChainRepo)); err != nil {
		log.Printf("audit trail: failed to install GORM plugin: %v", err)
	} else {
		log.Println("Governance: immutable hash-chained audit-trail plugin installed")
	}

	// Création du compte Admin par défaut si la DB est vide
	// Cela garantit que l'app est utilisable immédiatement après déploiement.
	handlers.SeedAdminUser()

	// Provision RBAC / tenant / audit tables (permissions, predefined roles, a Tenant
	// per Organization, UserTenant per membership) so the Settings admin tabs are live.
	handlers.SeedRBAC()

	// Demonstration data (DEMO_MODE=true only). Fixtures live outside the binary in
	// dev/fixtures/demo.json and are never embedded, so a production build cannot
	// materialise them by accident. A seeded tenant advertises itself through
	// /health's demo_mode flag, which drives a permanent banner in the UI — demo
	// content is never allowed to pass for a customer's real posture.
	if demoseed.Enabled() {
		var demoAdmin domain.User
		if err := database.DB.Where("email = ?", "admin@opendefender.io").First(&demoAdmin).Error; err != nil {
			log.Printf("DEMO_MODE: no seed admin found, skipping demo data: %v", err)
		} else if demoAdmin.DefaultOrgID == nil {
			log.Println("DEMO_MODE: seed admin has no default organization, skipping demo data")
		} else {
			res, err := demoseed.Seed(database.DB, *demoAdmin.DefaultOrgID, demoAdmin.ID)
			switch {
			case err != nil:
				log.Printf("DEMO_MODE: seeding failed: %v", err)
			case res.Skipped:
				log.Println("DEMO_MODE: tenant already contains data, demo fixtures not re-applied")
			default:
				log.Printf("DEMO_MODE: seeded %d assets, %d dependencies, %d risks, %d incidents", res.Assets, res.Dependencies, res.Risks, res.Incidents)
			}
		}
	}

	// =========================================================================
	// 3. SECURITY SERVICES INITIALIZATION
	// =========================================================================

	// Initialize Permission Service for advanced access control
	permissionService := service.NewPermissionService()
	if err := permissionService.InitializeDefaultRoles(); err != nil {
		log.Fatalf("Failed to initialize default permission roles: %v", err)
	}

	// Initialize Token Service for API token management
	tokenService := service.NewTokenService()

	// Initialize Score Engine Service for automatic risk score calculation
	scoreEngineService := service.NewScoreEngineService(database.DB)
	log.Println("Score Engine: Service initialized with default configuration")

	// =========================================================================
	// 3.5 JWT RS256 & SCORE WORKER INITIALIZATION (Critical Security & Events)
	// =========================================================================

	// Load RSA keys for JWT RS256 (fail-fast if missing)
	rsaKeys := authpkg.MustLoadRSAKeys(
		cfg.Server.RSAPrivateKeyPath,
		cfg.Server.RSAPublicKeyPath,
	)
	log.Println("Auth: RSA keys loaded successfully for JWT RS256")

	// Initialize Redis client for caching, events, and JWT blacklist
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379"
	}
	redisClientInstance := redisclient.NewClient(redisURL)
	log.Println("Redis: Client connected for events, caching, and JWT blacklist")

	// Initialize JWT token blacklist manager (via Redis)
	tokenBlacklistManager := authpkg.NewTokenBlacklistManager(redisClientInstance)
	log.Println("Auth: Token blacklist manager initialized (Redis-backed)")

	// Closure used by middleware.Protected(rsaKeys, jtiBlacklistChecker) to check the JTI blacklist on every request
	jtiBlacklistChecker := tokenBlacklistManager.CheckJTIBlacklist(context.Background())

	// The SAME closure re-checks live SSE streams on their keepalive tick, so a
	// revoked session stops receiving events within one interval instead of
	// holding a connection open for the stream's whole lifetime — up to two
	// hours on /realtime/events (#345). One predicate for both: a stream and a
	// request must never disagree about whether a session is revoked.
	handlers.SetSSERevocationChecker(jtiBlacklistChecker)

	// Initialize Score Engine (pure, stateless)
	scoreEngine := scoring.NewEngine()
	log.Println("Scoring: Engine initialized (pure, zero dependencies)")

	// Initialize file storage (compliance evidence, etc.)
	// STORAGE_DRIVER selects the backend; only "local" exists today. An
	// S3-backed driver can be added later behind the same storage.Storage
	// interface without touching any use case or handler.
	storageLocalPath := os.Getenv("STORAGE_LOCAL_PATH")
	if storageLocalPath == "" {
		storageLocalPath = "./uploads"
	}
	fileStorage, err := storage.NewLocalStorage(storageLocalPath)
	if err != nil {
		log.Fatal("Storage: failed to initialize local storage: ", err)
	}
	log.Println("Storage: local driver initialized at", storageLocalPath)

	// Initialize Score Worker (listens to Redis events)
	zeroLogger := zerolog.New(os.Stderr).With().Timestamp().Logger()
	riskRepoForWorker := repository.NewGormRiskRepository(database.DB)
	scoreWorker := workers.NewScoreWorker(redisClientInstance, scoreEngine, riskRepoForWorker, zeroLogger)

	// Start Score Worker in background goroutine
	go scoreWorker.Start(context.Background())
	log.Println("Workers: Score Engine worker started (listening for risk.updated events)")

	// =========================================================================
	// 3.6 REALTIME EVENT HUB (W0-07)
	// =========================================================================
	//
	// The backbone every live surface reads from: notifications, incidents,
	// automation, analytics and the dashboards. It replaces three hand-rolled
	// SSE loops that each re-implemented auth, tenant filtering and keepalive,
	// and that lost every event published while a client was reconnecting.
	//
	//   publisher → durable log (per-tenant sequence) → hub → SSE
	//                              ↘ Redis fanout → relay on other instances ↗
	//
	// The log is what makes a reconnect a replay instead of a full refetch. The
	// relay is what makes a two-replica deployment behave like a one-replica
	// one — without it a change made on instance 1 is invisible to a browser
	// connected to instance 2, which looks like the feature works until it is
	// scaled.
	realtimeEventLog := repository.NewGormRealtimeEventRepository(database.DB)
	realtimeHub := apprealtime.NewHub(apprealtime.HubOptions{})
	realtimePublisher := apprealtime.NewPublisher(
		realtimeEventLog,
		realtimeHub,
		apprealtime.WithFanout(redisClientInstance),
	)
	go apprealtime.NewRelay(redisClientInstance, realtimeHub).Start(context.Background())

	// Canonical events for the changes that never pass through an authenticated
	// mutation: a vulnerability arriving on a webhook with no session, a score
	// recomputed by the worker minutes later, a finding the scanner stopped
	// seeing. Each already publishes on an internal channel; this republishes
	// them through the canonical publisher so they land in the durable log with
	// a sequence and reach clients as the same envelope as everything else. The
	// internal channels and their existing consumers are untouched.
	go apprealtime.NewDomainEventRelay(redisClientInstance, realtimePublisher).Start(context.Background())

	// Retention is what makes the replay promise honest. The stream tells a
	// client "reconnect inside the window and you are caught up" and tells one
	// whose cursor predates it to resynchronise; both statements are only true
	// if something enforces the window. REALTIME_REPLAY_RETENTION_HOURS
	// overrides the default; 0 disables the sweep, which is a deliberate choice
	// to let the table grow rather than a default nobody asked for.
	realtimeRetention := domain.DefaultReplayRetention
	if raw := os.Getenv("REALTIME_REPLAY_RETENTION_HOURS"); raw != "" {
		if hours, err := strconv.Atoi(raw); err == nil && hours >= 0 {
			realtimeRetention = time.Duration(hours) * time.Hour
		} else {
			log.Printf("Realtime: ignoring unreadable REALTIME_REPLAY_RETENTION_HOURS=%q, keeping %s", raw, realtimeRetention)
		}
	}
	go workers.NewRealtimeRetentionWorker(realtimeEventLog, realtimeRetention, zeroLogger).Start(context.Background())
	log.Printf("Realtime: event hub started (durable log + per-tenant fanout + domain relay, replay window %s)", realtimeRetention)

	// =========================================================================
	// 4. HEXAGONAL ARCHITECTURE WIRING (Integrations)
	// =========================================================================

	// Initialisation des Adapters (TheHive, OpenRMF, OpenCTI)
	// Ils respectent les interfaces définies dans core/ports
	theHiveAdapter := thehive.NewTheHiveAdapter(cfg.Integrations.TheHive)

	// Get organization ID for SyncEngine (multi-tenant scoping - Rule 1)
	// In a multi-tenant setup, there would be one SyncEngine per organization
	// For now, we use the default organization from environment or placeholder
	organizationID := os.Getenv("SYNC_ORGANIZATION_ID")
	if organizationID == "" {
		// Fall back to first organization in DB or placeholder
		// TODO: In production, each organization should have its own SyncEngine instance
		organizationID = "550e8400-e29b-41d4-a716-446655440000" // Default placeholder
		log.Println("Warning: SYNC_ORGANIZATION_ID not set, using default placeholder. Set this env var for proper multi-tenant operation.")
	}

	// Initialisation du Moteur de Synchro (Background Worker)
	// Il tourne indépendamment de l'API HTTP
	syncEngine := workers.NewSyncEngine(theHiveAdapter, organizationID)
	syncEngine.Start(context.Background())

	log.Println("OpenDefender SyncEngine started in background")

	// =========================================================================
	// 4. WEB SERVER SETUP (Fiber)
	// =========================================================================

	app := fiber.New(fiber.Config{
		AppName:               "OpenRisk API (OpenDefender Suite)",
		DisableStartupMessage: true, // Plus propre dans les logs de prod
		ReadTimeout:           10 * time.Second,
		WriteTimeout:          10 * time.Second,
		// Trusted proxies (audit finding F-04). Fiber only resolves ProxyHeader
		// into c.IP() when the peer socket address is in TrustedProxies. With an
		// empty TRUSTED_PROXIES the check trusts nobody, so a client-supplied
		// X-Forwarded-For is ignored and c.IP() falls back to the real socket IP
		// — the safe posture for a directly exposed service. Deployments behind a
		// load balancer must set TRUSTED_PROXIES to the balancer's addresses.
		EnableTrustedProxyCheck: true,
		TrustedProxies:          middleware.TrustedProxies(),
		ProxyHeader:             fiber.HeaderXForwardedFor,
		// Always JSON. In production the raw error is logged server-side against a
		// correlation ID and never returned to the client — it used to be echoed
		// verbatim, leaking GORM's SQL and schema (audit finding F-02).
		ErrorHandler: middleware.ErrorHandler(middleware.IsProductionEnv()),
	})

	// --- Middlewares Globaux ---
	app.Use(recover.New()) // Empêche le crash complet en cas de panic
	app.Use(logger.New(logger.Config{
		Format: "[${time}] ${status} - ${method} ${path} (${latency})\n",
	}))
	// Security headers. helmet's zero-value config omits Content-Security-Policy
	// and Strict-Transport-Security entirely (audit finding F-01) — both are now
	// configured explicitly. See middleware.LoadSecurityHeadersConfig for the
	// env overrides and why the API policy is maximally strict.
	app.Use(middleware.SecurityHeaders(middleware.LoadSecurityHeadersConfig()))

	// CORS: honour the CORS_ORIGINS env var (comma-separated allowlist) when set
	// — this is what .env.example documents and what deployments need to point at
	// their own domain without recompiling. Fall back to sane per-env defaults.
	allowOrigins := os.Getenv("CORS_ORIGINS")
	if allowOrigins == "" {
		allowOrigins = "http://localhost:5173,http://localhost:3000"
		if os.Getenv("APP_ENV") == "production" {
			allowOrigins = "https://app.opendefender.io"
		}
	}
	// AllowCredentials is required for the HttpOnly session cookies: without it a
	// browser refuses to attach them to a cross-origin request. It also forbids
	// the "*" origin, which is why the allowlist above must stay explicit — the
	// combination of a wildcard origin and credentials is precisely the
	// misconfiguration that lets any site call the API as the logged-in user.
	app.Use(cors.New(cors.Config{
		AllowOrigins:     allowOrigins,
		AllowHeaders:     "Origin, Content-Type, Accept, Authorization, X-CSRF-Token, X-Device-Fingerprint",
		AllowMethods:     "GET, POST, PUT, PATCH, DELETE, OPTIONS",
		AllowCredentials: true,
	}))

	// =========================================================================
	// 5. API ROUTES
	// =========================================================================

	// Prometheus scrape endpoint, mounted on the root app (outside /api/v1 and
	// outside the auth gate, as a scrape target must be). It exposes the DEFAULT
	// registry, which is where pkg/monitoring registers its metrics at import
	// time — including openrisk_time_to_aha_seconds, the histogram behind the
	// SlowTimeToAha alert (deployment/monitoring/alerts.yml).
	//
	// Until now the codebase declared Prometheus metrics but never served them in
	// the exposition format, so nothing could scrape them.
	app.Get("/metrics", adaptor.HTTPHandler(promhttp.Handler()))

	api := app.Group("/api/v1")

	// Baseline per-IP quota across the whole API (audit finding F-03: only 2 of
	// 289 routes were metered). Mounted here so it also covers unauthenticated
	// surface — OAuth/SAML callbacks, webhooks, health. Credential endpoints keep
	// their own much stricter throttle below. The per-tenant quota is mounted
	// after the auth gate, where the tenant is known.
	//
	// Shares the Redis-backed store so counters hold across a horizontally-scaled
	// deployment, falling back to a per-instance limiter if Redis is unreachable.
	apiQuota := middleware.LoadQuotaConfig()
	quotaStore := middleware.NewRedisRateLimitStore(redisClientInstance)
	api.Use(middleware.IPRateLimit(quotaStore, apiQuota))

	// =========================================================================
	// 5.1 CLEAN ARCHITECTURE AUTH MODULE INITIALIZATION
	// =========================================================================

	// Initialize repositories
	userRepo := repository.NewGormUserRepository(database.DB)
	orgRepo := repository.NewGormOrganizationRepository(database.DB)
	// The single tenant-scoped store for organization memberships. Built here
	// because three call sites read through it: the ownership <UserPicker>, the
	// global search, and member management (§5.10).
	membershipRepo := repository.NewGormMembershipRepository(database.DB)

	// Initialize notification service (mock email transport - swap for SMTP in production)
	emailFromAddr := os.Getenv("EMAIL_FROM")
	if emailFromAddr == "" {
		emailFromAddr = "noreply@openrisk.local"
	}
	appBaseURL := os.Getenv("APP_BASE_URL")
	if appBaseURL == "" {
		appBaseURL = "http://localhost:5173"
	}
	// Email transport. Real SMTP when SMTP_HOST is set, a logging transport when
	// EMAIL_TRANSPORT=log, and otherwise a transport that returns
	// email.ErrNotConfigured rather than reporting a success it did not achieve.
	//
	// This used to be a mock that printed to stdout and returned nil, so every
	// caller believed its mail had been delivered. Best-effort callers (scan
	// notices, sign-in alerts) still ignore the error and behave as before; the
	// invitation flow surfaces it, because "we emailed them" is that feature's
	// entire user-visible outcome.
	emailTransport := buildEmailTransport()
	notificationService := notify.NewEmailService(emailTransport, emailFromAddr, appBaseURL)

	// Initialize password hasher (Argon2id, OWASP recommended — matches handlers.SeedAdminUser)
	passwordHasher := coreauth.NewArgon2idPasswordHasher()

	// Initialize token manager (access/refresh JWT pairs, backed by the DB).
	// There is now a SINGLE RSA key set + JWT implementation (pkg/auth): the exact
	// same `rsaKeys` used by middleware.Protected is handed to the TokenManager, so
	// every token is signed and validated by one implementation.
	tokenManager := coreauth.NewTokenManager(database.DB, rsaKeys)

	// Both resolvers live in session_wiring.go as named functions so they can be
	// driven in a test without a database. They enforce ACCOUNT state as well as
	// membership state; see the commentary there for why both are needed (#350).
	resolveSessionForOrg := func(ctx context.Context, uid, orgID uuid.UUID) (*coreauth.SessionClaims, error) {
		return resolveSessionClaimsForOrg(ctx, userRepo, uid, orgID)
	}
	resolveSession := func(ctx context.Context, uid uuid.UUID) (*coreauth.SessionClaims, error) {
		return resolveSessionClaims(ctx, userRepo, uid)
	}
	tokenManager.SetSessionResolver(resolveSession)
	tokenManager.SetOrgSessionResolver(resolveSessionForOrg)

	// L5 — Personal Access Tokens. DB-backed service (survives restarts, scoped),
	// its auth middleware, and a management handler. The same resolveSession gives a
	// PAT the owner's tenant + permissions (narrowed to the token's scopes).
	patService := coreauth.NewPersonalAccessTokenService(repository.NewGormPersonalAccessTokenRepository(database.DB))

	// L7 — full-fidelity auth audit trail (auth_audit_logs: IP, UA, geo, device
	// fingerprint, timestamp). Shared by the clean auth handler, MFA, and SSO.
	authAudit := coreauth.NewAuditService(repository.NewGormAuthAuditLogRepository(database.DB))

	// MFA (L4). AES-256 key for the encrypted TOTP secret at rest.
	mfaRepo := repository.NewGormMFARepository(database.DB)
	mfaKeyRaw := os.Getenv("MFA_ENCRYPTION_KEY")
	if mfaKeyRaw == "" {
		mfaKeyRaw = "openrisk-dev-mfa-encryption-key-change-me"
		log.Println("Warning: MFA_ENCRYPTION_KEY not set — using an insecure dev key. Set a strong 32-byte key in production.")
	}
	mfaKey := sha256.Sum256([]byte(mfaKeyRaw)) // 32 bytes for AES-256-GCM

	// --- Password policy (server-authoritative) ------------------------------
	// One policy object shared by registration, reset and the live strength meter,
	// so the rule the browser previews is byte-for-byte the rule the server
	// enforces. Breach checking is opt-out via PASSWORD_HIBP_DISABLED for
	// air-gapped installs; disabled, the local gates (12 chars, three classes,
	// zxcvbn >= 3) still apply.
	passwordPolicy := pwpolicy.New()
	if os.Getenv("PASSWORD_HIBP_DISABLED") != "true" {
		passwordPolicy = passwordPolicy.WithBreachChecker(hibp.New())
	}

	// --- Account-security email ----------------------------------------------
	// Wrapped in authmail.Async so a reset request costs the same whether or not
	// the address has an account. Sending inline would make "account exists"
	// measurable through response latency, reinstating the enumeration the
	// uniform response exists to prevent.
	securityMailer := authmail.NewAsync(authmail.New(emailTransport))

	// --- Activation (the newcomer journey) ------------------------------------
	// The server-side source of truth for "how far into the product is this
	// tenant?". Domain use cases below get `WithActivation(activationRecorder)`;
	// the recorder is best-effort and nil-safe, so noting a milestone can never
	// fail the business action that produced it.
	activationRepo := repository.NewGormActivationRepository(database.DB)
	// Existing members were using the product before onboarding existed: mark them
	// done so the new route guard cannot lock them out of their own workspace.
	// Idempotent; migration 0043 does the same in SQL for migrate-only deploys.
	if err := activationRepo.BackfillExistingMembers(context.Background()); err != nil {
		log.Printf("Warning: activation backfill failed (existing users may see the onboarding wizard): %v", err)
	}
	// The event log is forward-only, so a tenant configured before activation
	// shipped has no events at all and is told to create the first risk it
	// created months ago. Seed the checklist from the records the tenant already
	// holds, dated from those records. Idempotent; safe on every boot.
	if err := activationRepo.BackfillDerivedEvents(context.Background()); err != nil {
		log.Printf("Warning: activation derived backfill failed (configured tenants may see completed steps as incomplete): %v", err)
	}
	activationRecorder := appactivation.NewRecorder(activationRepo)
	ahaRecorder := appactivation.NewAhaRecorder(activationRepo)
	// Legacy handlers that build their own use cases inline (mitigation) read the
	// recorder from this package seam — see internal/handler/activation_wiring.go.
	handlers.SetActivationRecorder(activationRecorder)

	// --- Sessions -------------------------------------------------------------
	sessionRepo := repository.NewGormSessionRepository(database.DB)
	passwordResetRepo := repository.NewGormPasswordResetRepository(database.DB)
	oauthLinkRepo := repository.NewGormOAuthLinkRepository(database.DB)

	// Initialize use cases. Login enforces MFA when the user has a verified secret,
	// and REQUIRES it for admin/root: those accounts can change permissions across
	// the tenant and mint API tokens, so a password alone is not a proportionate
	// gate. Members keep MFA optional.
	// Which org roles must hold MFA is a deployment policy. Default admin+root
	// (privileged accounts); a deployment onboarding non-technical members who may
	// lack an authenticator can widen or narrow it via MFA_REQUIRED_ROLES
	// (comma-separated; empty string disables mandatory enrolment entirely).
	mfaRequiredRoles, mfaRequiredBusinessRoles := domain.DefaultMFAPrivilegeRoles()
	if v, ok := os.LookupEnv("MFA_REQUIRED_ROLES"); ok {
		mfaRequiredRoles = splitCSVEnv(v)
	}
	// OR26-03 — the security officer holds org role `user`, so a check on the org
	// role alone would exempt exactly the account the requirement is written for.
	if v, ok := os.LookupEnv("MFA_REQUIRED_BUSINESS_ROLES"); ok {
		mfaRequiredBusinessRoles = splitCSVEnv(v)
	}
	// OR26-03 — the tenant's grace window. Deployment decides WHO is privileged;
	// each tenant decides HOW LONG they may defer (default 7 days).
	mfaPolicyRepo := repository.NewGormMFAPolicyRepository(database.DB)
	loginUseCase := auth.NewLoginUseCase(userRepo, tokenManager, passwordHasher).
		WithMFA(mfaRepo).
		RequireMFAForRoles(mfaRequiredRoles, mfaRequiredBusinessRoles).
		WithMFAPolicies(mfaPolicyRepo)
	registerUseCase := auth.NewRegisterUseCase(userRepo, orgRepo, notificationService, passwordHasher).
		// Anchors t0 for the time-to-Aha histogram.
		WithActivation(activationRecorder)
	refreshUseCase := auth.NewRefreshTokenUseCase(tokenManager)
	logoutUseCase := auth.NewLogoutUseCase(tokenManager)

	// MFA use cases + handler.
	setupMFAUseCase := auth.NewSetupMFAUseCase(mfaRepo, mfaKey[:])
	verifyMFAUseCase := auth.NewVerifyMFAUseCase(mfaRepo, *userRepo, mfaKey[:])
	disableMFAUseCase := auth.NewDisableMFAUseCase(mfaRepo, passwordHasher)
	challengeMFAUseCase := auth.NewChallengeMFAUseCase(mfaRepo, mfaKey[:])
	// OR26-03 — one resolver answers "must this member enrol now?" for /auth/me
	// and for the request-time guard, so the banner and the enforcement can never
	// disagree. Cached per (user, tenant) for a minute; enrolment, disabling and
	// policy changes drop the entry immediately.
	mfaStatusResolver := auth.NewMFAStatusResolver(mfaRepo, userRepo, mfaRequiredRoles, mfaRequiredBusinessRoles).
		WithPolicies(mfaPolicyRepo)
	mfaHandler := authhandler.NewMFAHandler(setupMFAUseCase, verifyMFAUseCase, disableMFAUseCase, challengeMFAUseCase, tokenManager, userRepo, authAudit).
		WithMFAStatus(mfaStatusResolver)
	mfaPolicyHandler := authhandler.NewMFAPolicyHandler(
		auth.NewGetMFAPolicyUseCase(mfaPolicyRepo, mfaRequiredRoles, mfaRequiredBusinessRoles),
		auth.NewUpdateMFAPolicyUseCase(mfaPolicyRepo, mfaRequiredRoles, mfaRequiredBusinessRoles).
			WithCacheInvalidator(mfaStatusResolver),
	)
	patHandler := authhandler.NewPATHandler(patService, authAudit)

	// Password reset use cases + handler.
	requestResetUseCase := auth.NewRequestPasswordResetUseCase(userRepo, passwordResetRepo, securityMailer)
	confirmResetUseCase := auth.NewConfirmPasswordResetUseCase(
		userRepo, passwordResetRepo, passwordHasher, passwordPolicy, tokenManager, securityMailer,
	)
	passwordHandler := authhandler.NewPasswordHandler(
		requestResetUseCase, confirmResetUseCase, passwordPolicy, appBaseURL, authAudit,
	)

	// Session (device) management use cases + handler.
	listSessionsUseCase := auth.NewListSessionsUseCase(sessionRepo)
	revokeSessionUseCase := auth.NewRevokeSessionUseCase(sessionRepo)
	revokeOtherSessionsUseCase := auth.NewRevokeOtherSessionsUseCase(sessionRepo)
	sessionHandler := authhandler.NewSessionHandler(
		listSessionsUseCase, revokeSessionUseCase, revokeOtherSessionsUseCase, authAudit,
	)
	newDeviceNotifier := auth.NewNotifyNewDeviceUseCase(sessionRepo, securityMailer)

	// Organization switching (server-authorized). userRepo satisfies the narrow
	// MembershipRepository the use cases need; the switch mints an org-scoped
	// session via the org resolver wired above, so membership is re-checked at mint
	// time and a forged org id can never yield a session.
	listOrgsUseCase := auth.NewListOrganizationsUseCase(userRepo)
	switchOrgUseCase := auth.NewSwitchOrganizationUseCase(userRepo, tokenManager)
	switchHandler := authhandler.NewSwitchHandler(listOrgsUseCase, switchOrgUseCase, authAudit)

	// Initialize Clean Architecture auth handler
	cleanAuthHandler := authhandler.NewHandler(
		loginUseCase,
		registerUseCase,
		refreshUseCase,
		logoutUseCase,
		passwordHasher,
		authAudit,
	).WithNewDeviceNotifier(newDeviceNotifier).
		WithUserLookup(userRepo).
		WithMFAStatus(mfaStatusResolver).
		// The same repository resolveSessionForOrg reads at token mint time, so
		// /auth/me's business role and the token's permissions come from one
		// membership row and cannot disagree (#338).
		WithMemberLookup(userRepo)

	// OAuth identity resolution: known link → verified-email link → provision.
	// No provisioner is wired, so an identity with no OpenRisk account is refused
	// rather than silently admitted — SSO here signs EXISTING members in, it does
	// not create tenants. See internal/application/auth/oauth_link.go.
	oauthResolveUseCase := auth.NewResolveOAuthIdentityUseCase(userRepo, oauthLinkRepo)
	handlers.ConfigureOAuth2Resolver(oauthResolveUseCase, appBaseURL)

	// Initialize legacy auth handler (for backward compatibility)
	authHandler := handlers.NewAuthHandler()

	// Initialize OAuth2 and SAML2 configurations. Hand SSO the SAME token manager +
	// audit service so OAuth/SAML issue RS256 access+refresh pairs identical to
	// password login (previously they minted HS256 tokens the RS256 middleware
	// rejected) and are audited with the full field set.
	handlers.InitializeOAuth2()
	handlers.SetSSOTokenManager(tokenManager, authAudit, userRepo, orgRepo)

	// --- Routes Publiques ---
	// Public status endpoint (task §4) — probes the critical dependencies so the
	// public status page reflects reality, not a hardcoded "UP". No auth: it must
	// be reachable exactly when the app is degraded.
	api.Get("/status", func(c *fiber.Ctx) error {
		components := []fiber.Map{{"name": "api", "status": "operational"}}
		overall := "operational"

		dbStatus := "operational"
		if sqlDB, err := database.DB.DB(); err != nil || sqlDB.Ping() != nil {
			dbStatus = "down"
			overall = "major_outage"
		}
		components = append(components, fiber.Map{"name": "database", "status": dbStatus})

		return c.JSON(fiber.Map{
			"status":     overall,
			"version":    Version,
			"components": components,
		})
	})

	api.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":  "UP",
			"version": Version,
			"commit":  Commit,
			"db":      "CONNECTED",
			// Drives the permanent "demonstration data" banner. Served from the
			// backend rather than a frontend build flag so the two cannot disagree
			// about whether the data on screen is real.
			"demo_mode": demoseed.Enabled(),
		})
	})

	// Brute-force protection on credential endpoints (5 attempts / 15 min per IP).
	// Backed by Redis so the counter is shared across every instance of a
	// horizontally-scaled deployment; degrades gracefully to a per-instance
	// in-memory limiter if Redis is unreachable.
	authLimiterStore := middleware.NewRedisRateLimitStore(redisClientInstance)
	// Per-IP throttle on auth endpoints. 5/15min locked out legitimate users (a
	// couple of mistyped passwords, MFA re-auth, or shared-NAT colleagues) for a
	// quarter hour (OR-BUG-008). 15/5min still blocks rapid brute force (~3/min
	// sustained) without punishing normal use. Per-account lockout + captcha are
	// the stronger long-term controls (tracked separately).
	authRateLimit := middleware.RateLimit(middleware.RateLimitConfig{
		MaxRequests: 15,
		WindowSize:  5 * time.Minute,
		Store:       authLimiterStore,
	})

	// Clean Architecture Auth Routes
	api.Post("/auth/login", authRateLimit, cleanAuthHandler.Login)
	api.Post("/auth/register", authRateLimit, cleanAuthHandler.Register)
	api.Post("/auth/refresh", cleanAuthHandler.RefreshToken)
	api.Post("/auth/logout", cleanAuthHandler.Logout)

	// NOTE: the legacy HS256 login/refresh routes (/auth/legacy/*) were removed for
	// RC1. They minted HS256 sessions signed with JWT_SECRET that the RS256 gate
	// rejected anyway (dead but dangerous surface). /auth/login (RS256) is the sole
	// session-issuing path. The legacy handler is retained only for /users/me below.

	// --- Password reset (public) ---
	// Both legs sit behind the per-IP auth limiter on top of the per-address cap
	// the use case enforces: the address cap stops targeting one account, the IP
	// limiter stops sweeping many.
	//
	// /password/check is unauthenticated by necessity — it serves the strength
	// meter on the registration and reset screens, where there is no session. It
	// discloses nothing: the caller already knows the password they typed.
	api.Post("/auth/password/forgot", authRateLimit, passwordHandler.ForgotPassword)
	api.Post("/auth/password/reset", authRateLimit, passwordHandler.ResetPassword)
	api.Post("/auth/password/check", authRateLimit, passwordHandler.CheckPassword)

	// --- OAuth2 Routes ---
	api.Get("/auth/oauth2/login/:provider", handlers.OAuth2Login)
	api.Get("/auth/oauth2/callback/:provider", handlers.OAuth2Callback)

	// --- SAML2 Routes ---
	api.Get("/auth/saml2/login", handlers.SAML2InitiateLogin)
	api.Post("/auth/saml2/acs", handlers.SAML2ACS)
	api.Get("/auth/saml2/metadata", handlers.SAMLMetadata)

	// --- MFA challenge (L4, second login leg) ---
	// Reached with the short-lived MFA_REQUIRED token from /auth/login. Registered
	// on `api` BEFORE the Protected group: MFATokenMiddleware validates the special
	// token itself and rejects full/absent tokens. On a valid code the handler
	// mints the real access+refresh pair.
	api.Post("/auth/mfa/challenge", middleware.MFATokenMiddleware(rsaKeys, jtiBlacklistChecker), mfaHandler.Challenge)

	// Scanner AGENT endpoints (register/stream/push) are mounted on `app` HERE —
	// deliberately BEFORE the /api/v1 user-token middleware below — so they are
	// NOT wrapped by middleware.Protected or the marketplace's RequireRole (both
	// mounted at the /api/v1 prefix, which /api/v1/scanner/... would otherwise
	// inherit). Agents authenticate with their own scoped tokens (+ HMAC on push)
	// inside the handlers. scannerHandler is assigned in §5.6; these closures
	// capture it and run only at request time, by which point it is set.
	var scannerHandler *handlers.ScannerHandler
	app.Post("/api/v1/scanner/agents/register", func(c *fiber.Ctx) error { return scannerHandler.RegisterAgent(c) })
	app.Get("/api/v1/scanner/agent/stream", func(c *fiber.Ctx) error { return scannerHandler.AgentStream(c) })
	app.Post("/api/v1/scanner/agent/push", func(c *fiber.Ctx) error { return scannerHandler.AgentPush(c) })
	app.Post("/api/v1/scanner/agent/heartbeat", func(c *fiber.Ctx) error { return scannerHandler.AgentHeartbeat(c) })

	// Mitigation SSE stream — mounted here (before the /api/v1 user middleware) so it
	// escapes the JWT gate: native EventSource can't send a Bearer header, so the
	// token is validated from ?token= inside the handler. Assigned in §5.7.
	var mitigationEventsHandler *handlers.MitigationEventsHandler
	app.Get("/api/v1/mitigations/events", func(c *fiber.Ctx) error { return mitigationEventsHandler.Stream(c) })

	// Invitation preview / acceptance — mounted on `app` BEFORE the /api/v1 JWT
	// gate, for the same reason as the scanner agent routes: `api.Use(Protected)`
	// below is registered on the /api/v1 PREFIX and therefore wraps every route
	// under it, whenever it was declared. An invitee following their link may
	// have no OpenRisk account at all, so a JWT gate would 401 exactly the people
	// these endpoints exist for.
	//
	// They are not unauthenticated, though: OptionalAuth stamps identity WHEN a
	// valid session is present (the use case then requires that identity to be
	// the invited address) and lets the request through anonymously otherwise.
	// It deliberately never stamps a tenant — the organization comes from the
	// invitation the token resolves to, never from the caller.
	var invitationPublicHandler *handlers.OrganizationMemberHandler
	optionalAuth := middleware.OptionalAuth(rsaKeys, jtiBlacklistChecker)
	app.Get("/api/v1/invitations/preview", optionalAuth, func(c *fiber.Ctx) error {
		return invitationPublicHandler.PreviewInvitation(c)
	})
	app.Post("/api/v1/invitations/accept", optionalAuth, func(c *fiber.Ctx) error {
		return invitationPublicHandler.AcceptInvitation(c)
	})

	// Vulnerability scanner webhook — external tools (Nessus/Qualys/Defender/…) POST
	// findings here authenticated by the integration's opaque webhook token (NOT a
	// user JWT). Mounted on `app` BEFORE the /api/v1 JWT gate for the same reason as
	// the scanner agent endpoints. Assigned in the vulnerability section below.
	var vulnWebhookHandler *handlers.VulnWebhookHandler
	app.Post("/api/v1/vulnerabilities/webhook/:source", func(c *fiber.Ctx) error { return vulnWebhookHandler.Ingest(c) })

	// --- Routes Protégées (Nécessitent JWT) ---
	// Le middleware injecte user_id et role dans le contexte
	// L5 — PAT authentication runs BEFORE the JWT gate: it authenticates PAT-shaped
	// bearers and is a no-op for JWTs (which the RS256 middleware then handles). The
	// JWT middleware skips when a PAT already authenticated the request.
	api.Use(middleware.PATMiddleware(patService, resolveSession))
	protected := api.Use(middleware.Protected(rsaKeys, jtiBlacklistChecker))

	// Response-cache invalidation (#337). Mounted here, right after the gate that
	// resolves the tenant, so every authenticated write drops that tenant's
	// cached responses. One place rather than a hook in each handler: the next
	// handler somebody adds cannot forget it. No-op without Redis.
	protected.Use(handlers.InvalidateCacheOnMutation(cacheableHandlers.Decoration()))

	// Per-tenant quota (audit finding F-03), mounted immediately after the auth
	// gate because that is what populates the tenant local. Without this a single
	// tenant — runaway integration or compromised token — can exhaust shared DB
	// and Redis capacity and degrade every other customer, while staying under
	// the per-IP limit by spreading traffic across hosts.
	protected.Use(middleware.TenantRateLimit(quotaStore, apiQuota))

	// OR26-03 — MFA policy, enforced at request time rather than only at the
	// door. Enforcing at login alone would let a privileged account that signed
	// in on day one keep working past its deadline simply by never signing out,
	// which would make the window bound how long you may wait to log in rather
	// than how long you may go without a second factor.
	//
	// Mounted here so it sees the identity the auth gate has just published, and
	// before every business route. The enrolment endpoints and logout stay
	// reachable (see mfaGuardExemptSuffixes) — a requirement you cannot satisfy
	// is a lockout, not a control.
	protected.Use(middleware.MFAPolicyGuard(mfaStatusResolver))

	// Governance audit trail (spec §15): stamp the acting identity + request
	// metadata onto the request context for every authenticated route, so any
	// repository that threads c.UserContext() into GORM lets the audittrail
	// plugin attribute the mutation to the real user (the "Qui"). Additive and
	// value-only — it never alters request handling.
	protected.Use(func(c *fiber.Ctx) error {
		if mw := middleware.GetContext(c); mw != nil {
			var actorID *uuid.UUID
			if mw.UserID != uuid.Nil {
				id := mw.UserID
				actorID = &id
			}
			c.SetUserContext(audittrailinfra.WithActor(c.UserContext(), audittrailinfra.Actor{
				ID:        actorID,
				TenantID:  mw.OrganizationID,
				IPAddress: c.IP(),
				UserAgent: c.Get("User-Agent"),
				RequestID: c.Get("X-Request-ID"),
			}))
		}
		return c.Next()
	})

	// Exhaustive mutation journaling. Mounted right after the actor stamp so it
	// sees the identity, and BEFORE every business route so no route can escape
	// it: each successful POST/PUT/PATCH/DELETE produces exactly one chained
	// audit entry (actor, tenant, action, resource, before → after, ip, user
	// agent, request id, timestamp). Adding a route adds coverage automatically.
	// The realtime bridge rides the SAME observation. Every successful mutation
	// already yields the tenant, the actor, the resource that actually changed
	// and the changed field names; deriving canonical events from that beats
	// adding a publish call to several hundred use cases, because the call
	// somebody eventually forgets is the one that mattered. What is published
	// is still decided by an explicit map onto the catalog, so an unmapped
	// mutation stays silent and no UI-shaped event can ever be emitted.
	protected.Use(middleware.AuditMutations(auditChainRepo, apprealtime.NewMutationBridge(realtimePublisher)))

	// =========================================================================
	// Open-core: entitlements + billing. This is where the plan model becomes
	// enforceable. The entitlement service resolves a tenant's effective plan
	// (subscription-driven, briefly cached) and the middleware below REFUSES
	// premium routes for plans that do not include them — the frontend only greys
	// and explains. Payment gateways are read from the environment; with no keys
	// the instance stays on Free + manual upgrades and says so honestly.
	// =========================================================================
	entitlementRepo := repository.NewGormEntitlementRepository(database.DB)
	billingRepo := repository.NewGormBillingRepository(database.DB)
	entitlementService := entapp.NewService(entitlementRepo).
		WithSubscriptions(billingRepo).
		WithUsage(entitlementRepo)

	billingRegistry := pkgbilling.NewRegistry(
		pkgbilling.NewStripeGateway(os.Getenv("STRIPE_SECRET_KEY")),
		pkgbilling.NewNotchpayGateway(os.Getenv("NOTCHPAY_PUBLIC_KEY")),
		pkgbilling.NewCinetpayGateway(os.Getenv("CINETPAY_API_KEY"), os.Getenv("CINETPAY_SITE_ID")),
	)
	billingService := billingapp.NewService(billingRepo, billingRegistry).
		WithCache(entitlementService).
		WithBaseURL(os.Getenv("APP_BASE_URL"))

	entitlementHandler := handlers.NewEntitlementHandler(entitlementService)
	billingHandler := handlers.NewBillingHandler(billingService, billingRegistry)

	// The entitlements snapshot drives the whole paywall UX and the current
	// subscription is read by every member — the session alone authorises those.
	//
	// The line between the two POSTs is D-034, and it is not where the #529 audit
	// first drew it. Checkout opens a hosted payment session in the ORGANISATION's
	// name, so it sits with change-plan and cancel behind the admin role. Starting
	// a trial does not spend anything and sits on the activation path, where
	// friction is the expensive thing — so it stays open to any member. Its
	// missing guard is therefore deliberate, not an omission, which is why it is
	// still listed in routesWithoutPermissionGuard.
	protected.Get("/entitlements", entitlementHandler.GetEntitlements)
	protected.Get("/billing", billingHandler.GetBilling)
	protected.Post("/billing/trial", billingHandler.StartTrial)
	protected.Post("/billing/checkout", middleware.RequireRole("admin", "root"), billingHandler.Checkout)
	protected.Post("/billing/change-plan", middleware.RequireRole("admin", "root"), billingHandler.ChangePlan)
	protected.Post("/billing/cancel", middleware.RequireRole("admin", "root"), billingHandler.Cancel)

	// Telemetry (opt-in, anonymous, env-killable). Reading state is open to any
	// member so the UI reflects it; toggling consent is admin-only. The env kill
	// switch OPENRISK_TELEMETRY=off always wins over stored consent.
	telemetryHandler := handlers.NewTelemetryHandler(
		repository.NewGormTelemetryRepository(database.DB),
		os.Getenv("OPENRISK_TELEMETRY"), Version,
	)
	protected.Get("/telemetry", telemetryHandler.GetTelemetry)
	protected.Put("/telemetry", middleware.RequireRole("admin", "root"), telemetryHandler.SetTelemetry)

	// Feature/limit gates reused on the premium routes below. A gated read that a
	// plan does not include returns 402 with an explaining body the frontend turns
	// into an upgrade prompt (the blurred UpsellLock), never a bare wall.
	featFinancial := middleware.RequireFeature(entitlementService, ent.FeatFinancialQuant)
	featSmartScore := middleware.RequireFeature(entitlementService, ent.FeatSmartScore)
	featExecutive := middleware.RequireFeature(entitlementService, ent.FeatExecutiveDashboard)
	featAI := middleware.RequireFeature(entitlementService, ent.FeatAIAdvisor)
	capRisks := middleware.RequireCapacity(entitlementService, ent.LimitRisks)
	capAssets := middleware.RequireCapacity(entitlementService, ent.LimitAssets)

	// --- MFA enrollment (L4) ---
	// Registered on `api` with MFAEnrollmentMiddleware rather than under
	// `protected`, because enrolment must be reachable in two states: by a
	// signed-in user turning MFA on from Settings (full access token), and by an
	// admin whose role MANDATES MFA and who therefore holds no session yet
	// (MFA_ENROLLMENT token). The middleware accepts exactly those two types and
	// refuses MFA_REQUIRED — see internal/middleware/mfa_enrollment.go for why
	// that exclusion is what stops a stolen password overwriting a victim's
	// authenticator.
	//
	// Disabling MFA stays under `protected`: it needs a real session and the
	// account password.
	mfaEnrollmentGuard := middleware.MFAEnrollmentMiddleware(rsaKeys, jtiBlacklistChecker)
	api.Post("/auth/mfa/setup", mfaEnrollmentGuard, mfaHandler.Setup)
	api.Post("/auth/mfa/verify", mfaEnrollmentGuard, mfaHandler.Verify)
	protected.Post("/auth/mfa/disable", mfaHandler.Disable)

	// --- MFA policy (OR26-03) — "force MFA after N days" -----------------------
	// Reading is open to any authenticated member: everyone subject to a deadline
	// deserves to see what it is, and the banner needs the bounds to validate
	// against the server's numbers rather than a copy that can drift. Writing is
	// admin-only — moving the deadline by which privileged accounts must hold a
	// second factor is a security decision, and it lands in the governance audit
	// trail through domain.MFAPolicy's Auditable opt-in.
	protected.Get("/security/mfa-policy", mfaPolicyHandler.Get)
	protected.Put("/security/mfa-policy", middleware.RequireRole("admin", "root"), mfaPolicyHandler.Update)

	// --- Sessions / devices (L3) — full session required ---
	protected.Get("/auth/sessions", sessionHandler.ListSessions)
	protected.Delete("/auth/sessions/others", sessionHandler.RevokeOtherSessions)
	protected.Delete("/auth/sessions/:id", sessionHandler.RevokeSession)

	// Organization switching — list the orgs the user may enter, and switch into
	// one. Both require an authenticated session (mounted under protected); the
	// switch re-validates active membership of the target org on the server.
	protected.Get("/auth/organizations", switchHandler.ListOrganizations)
	protected.Post("/auth/switch-org", switchHandler.SwitchOrganization)

	// --- Personal Access Tokens (L5) management — full session required ---
	// Every PAT verb acts on the CALLER's own tokens (the service scopes by user
	// id and Create refuses a PAT minting another PAT), so the session is the
	// authorization — #529, bucket 1.
	protected.Post("/auth/pat", patHandler.Create)
	protected.Get("/auth/pat", patHandler.List)
	protected.Delete("/auth/pat/:id", patHandler.Revoke)

	// Current user profile endpoint
	api.Get("/auth/me", middleware.Protected(rsaKeys, jtiBlacklistChecker), cleanAuthHandler.Me)
	api.Get("/users/me", authHandler.GetProfile)

	// Dashboard & Analytics (Read-Only accessible à tous les connectés)
	// Tenant-wide counters, no per-entity disclosure, and every business role
	// preset already holds risks:read — the session is the authorization (#529).
	protected.Get("/stats", cacheableHandlers.CacheDashboardStatsGET(handlers.GetDashboardStats))

	// --- Generalised ownership (responsable / exécutant / validateur) ---
	// One service backs the <UserPicker> for every entity: it decides who may be
	// assigned (active members of THIS tenant), applies the tri-state patch, and
	// notifies the newly assigned. Its dependencies are stateless wrappers over
	// database.DB, so instantiating them here (ahead of the RBAC and notification
	// blocks further down) costs nothing and keeps the risk module's wiring local.
	ownershipMembers := memberDirectory{repo: membershipRepo}
	ownershipService := ownership.NewService().
		WithMembers(ownershipMembers).
		WithUsers(repository.NewGormUserRepository(database.DB)).
		WithNotifier(notificationapp.NewUseCase(repository.NewNotificationRepository(database.DB)))
	ownershipHandler := handlers.NewOwnershipHandler(ownership.NewListAssignableUseCase(ownershipMembers))
	// Legacy handlers (mitigation, incident) build their use cases inline and
	// cannot receive this through the constructor — same seam as the activation
	// recorder above.
	handlers.SetOwnershipService(ownershipService)
	// Any authenticated member may see who they can assign work to — the picker
	// is useless otherwise, and it exposes nothing an org chart would not.
	protected.Get("/ownership/assignable", ownershipHandler.ListAssignable)
	protected.Get("/ownership/me", ownershipHandler.Me)

	// Initialize clean architecture risk module
	riskRepo := repository.NewGormRiskRepository(database.DB)
	riskControlMappingRepo := repository.NewGormRiskControlMappingRepository(database.DB)
	createRiskUseCase := risk.NewCreateRiskUseCase(riskRepo).
		WithActivation(activationRecorder).
		WithOwnership(ownershipService)
	getRiskUseCase := risk.NewGetRiskUseCase(riskRepo).
		WithMappings(riskControlMappingRepo).
		WithOwnership(ownershipService)
	listRisksUseCase := risk.NewListRisksUseCase(riskRepo).
		WithMappings(riskControlMappingRepo).
		WithOwnership(ownershipService)
	updateRiskUseCase := risk.NewUpdateRiskUseCase(riskRepo).WithOwnership(ownershipService)
	deleteRiskUseCase := risk.NewDeleteRiskUseCase(riskRepo)
	// Cyber Risk Quantification: XAF→USD rate configurable via XAF_USD_RATE
	// (default ≈ 600 FCFA/USD). Reference ALE bands match the board ExposureModel.
	xafPerUSD := crq.DefaultXAFPerUSD
	if v := os.Getenv("XAF_USD_RATE"); v != "" {
		if parsed, perr := strconv.ParseFloat(v, 64); perr == nil && parsed > 0 {
			xafPerUSD = parsed
		}
	}
	riskQuantifier := crq.NewQuantifier(xafPerUSD, crq.DefaultReference())
	// Daily FX rate job (spec §3): keeps a dated rate table so every converted
	// amount can show its reference date. Defaults to the static reference source
	// (honest fallback) until a live feed is plugged into FXRateSource.
	fxWorker := workers.NewFXRateWorker(workers.StaticFXSource{}, zeroLogger)
	go fxWorker.Start(context.Background())
	// Currency/FX presenter factory: resolves each tenant's display currency (org
	// settings) + the live rate table so financial figures convert consistently.
	financialPresenters := risk.NewFinancialPresenterFactory(riskQuantifier).
		WithCurrency(orgRepo).
		WithRates(fxWorker)
	markReviewedUseCase := risk.NewMarkRiskReviewedUseCase(riskRepo)
	// The lifecycle FSM enforces its guards server-side. Its two inspectors are
	// wired here so "IN_TREATMENT needs an active mitigation", "MITIGATED needs
	// every sub-action done" and "RESIDUAL_ACCEPTED needs a validated Governance
	// approval" are answered from real data rather than trusted from the client.
	transitionStateUseCase := risk.NewTransitionRiskStateUseCase(riskRepo).
		WithMitigations(newMitigationInspector(
			repository.NewGormMitigationRepository(database.DB),
			repository.NewGormMitigationSubActionRepository(database.DB),
		)).
		WithApprovals(newApprovalChecker(repository.NewGormApprovalRepository(database.DB)))
	riskHandler := handlers.NewRiskHandler(createRiskUseCase, getRiskUseCase, listRisksUseCase, updateRiskUseCase, deleteRiskUseCase, markReviewedUseCase, transitionStateUseCase, redisClientInstance, riskQuantifier).
		WithFinancialPresenters(financialPresenters)

	// Financial Risk Quantification (spec §9): tenant-wide CFO/CISO dashboard
	// (portfolio FAIR-lite P10/P50/P90, ALE, worst-case, residual, remediation
	// budget, ROSI). Reuses the same quantifier as per-risk CRQ so figures agree;
	// the coverage counter makes "N/M quantified" a SQL fact, and the presenter
	// factory converts every figure into the tenant's currency at a dated rate.
	financialSummaryUseCase := risk.NewFinancialSummaryUseCase(riskRepo, riskQuantifier).
		WithPresenters(financialPresenters).
		WithCoverageCounter(riskRepo)
	financialAnalyticsHandler := handlers.NewFinancialAnalyticsHandler(financialSummaryUseCase).
		WithCurrencyWriter(orgRepo)

	// NOTE: same bug class as compliance (see comment above complianceFrameworkRead) —
	// middleware.RequirePermissions reads the legacy *domain.UserClaims, which the RS256
	// middleware on `protected` never populates. Using middleware.RequirePermission instead.
	protected.Get("/risks",
		middleware.RequirePermission("risks:read"),
		cacheableHandlers.CacheRiskListGET(riskHandler.GetRisks))
	// --- Risk taxonomy (spec §3): three separate concepts, three separate
	// columns. tags stay a free array on the risk; categories are the tenant's
	// CONTROLLED vocabulary; control mappings are references to real compliance
	// controls (what the "Référentiel" column may render, and nothing else).
	riskTaxonomyHandler := handlers.NewRiskTaxonomyHandler(
		risk.NewCategoryUseCase(repository.NewGormRiskCategoryRepository(database.DB)),
		risk.NewControlMappingUseCase(riskControlMappingRepo, riskRepo).
			WithControls(repository.NewGormComplianceRepository(database.DB)),
	)
	// FIBER TRAP: the static path must be registered BEFORE /risks/:id, or
	// "unmapped" is parsed as a uuid and 400s. Same trap as /compliance and
	// /asset-dependencies.
	protected.Get("/risks/unmapped",
		middleware.RequirePermission("risks:read"), riskTaxonomyHandler.ListUnmappedRisks)
	protected.Get("/risks/:id",
		middleware.RequirePermission("risks:read"),
		cacheableHandlers.CacheRiskGetByIDGET(riskHandler.GetRisk))
	// Financial Risk Quantification (spec §9). Read-only: full per-risk assessment
	// and a non-persisting investment-scenario simulator. Static "financial"/
	// "simulate" segments are risk-scoped so they never collide with :id parsing.
	protected.Get("/risks/:id/financial",
		middleware.RequirePermission("risks:read"), featFinancial, riskHandler.GetRiskFinancial)
	protected.Post("/risks/:id/simulate",
		middleware.RequirePermission("risks:read"), featFinancial, riskHandler.SimulateRiskFinancial)

	// Gestion des Risques (Écriture = Analyst & Admin uniquement)
	// Respect du principe "Simplicité & Sécurité" + Fine-grained Permission Checks
	riskCreate := middleware.RequirePermission("risks:create")
	riskUpdate := middleware.RequirePermission("risks:update")
	riskDelete := middleware.RequirePermission("risks:delete")

	protected.Post("/risks", riskCreate, capRisks, riskHandler.CreateRisk)
	protected.Patch("/risks/:id", riskUpdate, riskHandler.UpdateRisk)
	protected.Post("/risks/:id/review", riskUpdate, riskHandler.MarkReviewed)
	// ISO 31000 lifecycle transition (Identifier → … → Clôturer). Tenant-scoped, audited.
	protected.Post("/risks/:id/transition", riskUpdate, riskHandler.TransitionPhase)
	// The stepper's contract: what can this risk become next, and what is in the
	// way. Read-only, so it rides the read permission.
	protected.Get("/risks/:id/transitions", middleware.RequirePermission("risks:read"), riskHandler.ListTransitions)
	protected.Delete("/risks/:id", riskDelete, riskHandler.DeleteRisk)

	// A risk's references to compliance controls. Reading rides risks:read;
	// writing rides risks:update, because mapping a risk IS editing the risk.
	protected.Get("/risks/:id/control-mappings",
		middleware.RequirePermission("risks:read"), riskTaxonomyHandler.ListRiskMappings)
	protected.Post("/risks/:id/control-mappings", riskUpdate, riskTaxonomyHandler.CreateRiskMapping)
	protected.Delete("/risks/:id/control-mappings/:mappingId", riskUpdate, riskTaxonomyHandler.DeleteRiskMapping)

	// The controlled category vocabulary. Any member may read it (the register
	// renders it); only an admin curates it — a vocabulary anyone can extend is
	// a tag list with extra steps.
	protected.Get("/risk-categories",
		middleware.RequirePermission("risks:read"), riskTaxonomyHandler.ListCategories)
	protected.Post("/risk-categories",
		middleware.RequireRole("admin", "root"), riskTaxonomyHandler.CreateCategory)
	protected.Patch("/risk-categories/:id",
		middleware.RequireRole("admin", "root"), riskTaxonomyHandler.UpdateCategory)
	protected.Delete("/risk-categories/:id",
		middleware.RequireRole("admin", "root"), riskTaxonomyHandler.DeleteCategory)

	// Mitigation Plans (CRUD). NOTE: this whole module previously used
	// middleware.RequireRole ("writerRole"), which reads c.Locals("role") — a flat
	// string AuthMiddlewareRS256 never sets (it sets "org_roles", a map, instead).
	// Every mitigation route has therefore been returning 401 "No role in token" for
	// every request regardless of caller — the same legacy-middleware bug class
	// already fixed for /risks and /compliance/*, just never applied here. Switched
	// to middleware.RequirePermission to match those two modules.
	mitigationRead := middleware.RequirePermission("mitigations:read")
	mitigationCreate := middleware.RequirePermission("mitigations:create")
	mitigationUpdate := middleware.RequirePermission("mitigations:update")
	mitigationDelete := middleware.RequirePermission("mitigations:delete")
	// Incident write guards. These used to be RequireRole("admin","analyst"), but
	// "analyst" is not a runtime org role (org roles are root/admin/user), so that
	// gate was effectively admin-only and could never be granted to a business
	// role. Switched to permission strings (the incidents:* family) so RSSI and
	// Security Analyst business roles can declare/manage incidents while admin/root
	// still pass via the "*" wildcard — consistent with risks/mitigations/compliance.
	incidentCreate := middleware.RequirePermission("incidents:create")
	incidentUpdate := middleware.RequirePermission("incidents:update")
	incidentDelete := middleware.RequirePermission("incidents:delete")

	protected.Get("/mitigations", mitigationRead, handlers.ListMitigations)
	protected.Post("/risks/:id/mitigations", mitigationCreate, handlers.CreateMitigation)
	protected.Get("/mitigations/:id", mitigationRead, handlers.GetMitigation)
	protected.Get("/risks/:id/mitigations", mitigationRead, handlers.ListMitigationsByRisk)
	protected.Patch("/mitigations/:id", mitigationUpdate, handlers.UpdateMitigation)
	protected.Delete("/mitigations/:id", mitigationDelete, handlers.DeleteMitigation)
	protected.Patch("/mitigations/:id/validate", mitigationUpdate, handlers.ValidateMitigation)

	// Sub-actions (checklist) for mitigations
	protected.Post("/mitigations/:id/sub-actions", mitigationCreate, handlers.CreateSubAction)
	protected.Patch("/mitigations/:id/sub-actions/:aid", mitigationUpdate, handlers.UpdateSubAction)
	protected.Post("/mitigations/:id/sub-actions/:aid/complete", mitigationUpdate, handlers.CompleteSubAction)
	protected.Post("/mitigations/:id/sub-actions/:aid/revert", mitigationUpdate, handlers.RevertSubAction)
	protected.Delete("/mitigations/:id/sub-actions/:aid", mitigationDelete, handlers.DeleteSubAction)
	protected.Patch("/mitigations/:id/reorder-subactions", mitigationUpdate, handlers.ReorderSubActions)

	// Scanner auto-completion. Guarded like every other mitigation write: the
	// handler derives the tenant from the session, and this permission is the
	// authorisation. It previously relied on a shared key compiled into the
	// source, which authenticated nobody once the repository was public.
	protected.Post("/scanner/mitigations/auto-complete", mitigationUpdate, handlers.AutoCompleteMitigationSubAction)

	// Compliance Frameworks (M1 — see ROADMAP.md §3)
	complianceRepo := repository.NewGormComplianceRepository(database.DB)
	createFrameworkUC := compliance.NewCreateFrameworkUseCase(complianceRepo)
	getFrameworkUC := compliance.NewGetFrameworkUseCase(complianceRepo)
	listFrameworksUC := compliance.NewListFrameworksUseCase(complianceRepo)
	deleteFrameworkUC := compliance.NewDeleteFrameworkUseCase(complianceRepo)
	createControlUC := compliance.NewCreateControlUseCase(complianceRepo)
	getControlUC := compliance.NewGetControlUseCase(complianceRepo)
	listControlsUC := compliance.NewListControlsUseCase(complianceRepo)
	updateControlUC := compliance.NewUpdateControlUseCase(complianceRepo)
	deleteControlUC := compliance.NewDeleteControlUseCase(complianceRepo)
	getProgressUC := compliance.NewGetComplianceProgressUseCase(complianceRepo)
	getGapAnalysisUC := compliance.NewGetGapAnalysisUseCase(complianceRepo)
	controlMappingRepo := repository.NewGormControlCrosswalkRepository(database.DB)
	createMappingUC := compliance.NewCreateControlMappingUseCase(controlMappingRepo, complianceRepo)
	listMappingsUC := compliance.NewListControlMappingsUseCase(controlMappingRepo, complianceRepo)
	deleteMappingUC := compliance.NewDeleteControlMappingUseCase(controlMappingRepo)
	listCatalogsUC := compliance.NewListCatalogsUseCase()
	// Evidence library (spec §1). complianceRepo satisfies the narrow
	// ControlLookup port structurally, so the library can verify a control belongs
	// to the tenant without being handed the ability to mutate the register.
	// userRepo resolves "collected by" to an email; optional and nil-safe.
	evidenceRepo := repository.NewGormEvidenceRepository(database.DB)
	// Move any pre-library attachments into the library. Idempotent, and
	// best-effort: an old attachment that fails to move is worth a log line, not
	// a server that refuses to start.
	if moved, err := evidenceRepo.BackfillFromControlEvidences(context.Background()); err != nil {
		log.Printf("Evidence: backfill from control_evidences failed (pre-library attachments may be missing): %v", err)
	} else if moved > 0 {
		log.Printf("Evidence: migrated %d pre-library attachment(s) into the evidence library", moved)
	}
	evidenceService := evidence.NewService(evidenceRepo, complianceRepo, fileStorage).
		WithUserLookup(userRepo)
	evidenceHandler := handlers.NewEvidenceHandler(evidenceService)

	// Curated crosswalks are materialised at import time and the head start they
	// produce is returned with the import — the answer arrives when the question
	// is asked, not when the user goes looking for it.
	crosswalkMaterialiser := compliance.NewCrosswalkMaterialiser(complianceRepo, controlMappingRepo)
	inheritedCoverageUC := compliance.NewGetInheritedCoverageUseCase(complianceRepo, controlMappingRepo, evidenceRepo)
	importCatalogUC := compliance.NewImportCatalogUseCase(complianceRepo).
		WithActivation(activationRecorder).
		WithCrosswalks(crosswalkMaterialiser, inheritedCoverageUC)
	// M4 — official compliance report (PDF). Reuses userRepo/orgRepo (declared
	// above) to resolve the "generated by" and organization identity lines.
	generateReportUC := compliance.NewGenerateComplianceReportUseCase(complianceRepo, orgRepo, userRepo)
	complianceHandler := handlers.NewComplianceHandler(
		createFrameworkUC, getFrameworkUC, listFrameworksUC, deleteFrameworkUC,
		createControlUC, getControlUC, listControlsUC, updateControlUC, deleteControlUC,
		getProgressUC, listCatalogsUC, importCatalogUC, generateReportUC,
		getGapAnalysisUC,
		createMappingUC, listMappingsUC, deleteMappingUC,
	).WithInheritedCoverage(inheritedCoverageUC)

	// NOTE: these routes sit under `protected`, whose base middleware (middleware.Protected,
	// RS256) stores the *new* multi-tenant claims in c.Locals("user")/("permissions"). The
	// legacy middleware.RequirePermissions expects the old HMAC-era *domain.UserClaims instead,
	// so it always failed the type assertion here ("user context not found", 401 on every
	// request) — the granular admin/analyst/viewer split below was never actually reachable.
	// middleware.RequirePermission (singular) is the one that matches what's really in context;
	// domain.PermissionSet only grants "*" to root/admin org members today (no per-resource
	// Profile rules for compliance yet), so this is admin/root-only until that's extended.
	complianceFrameworkRead := middleware.RequirePermission("compliance:frameworks:read")
	complianceFrameworkCreate := middleware.RequirePermission("compliance:frameworks:create")
	complianceFrameworkDelete := middleware.RequirePermission("compliance:frameworks:delete")
	complianceControlRead := middleware.RequirePermission("compliance:controls:read")
	complianceControlCreate := middleware.RequirePermission("compliance:controls:create")
	complianceControlUpdate := middleware.RequirePermission("compliance:controls:update")
	complianceControlDelete := middleware.RequirePermission("compliance:controls:delete")
	complianceEvidenceRead := middleware.RequirePermission("compliance:evidences:read")
	complianceEvidenceCreate := middleware.RequirePermission("compliance:evidences:create")
	complianceEvidenceDelete := middleware.RequirePermission("compliance:evidences:delete")

	// Catalogs are static regulatory reference data (pkg/compliance) — global, read-only,
	// same permission tier as listing frameworks. Importing one instantiates controls under
	// a tenant's own framework, so it's gated the same as creating a framework by hand.
	protected.Get("/compliance/catalogs", complianceFrameworkRead, complianceHandler.ListCatalogs)
	protected.Post("/compliance/frameworks/:frameworkId/import-catalog", complianceFrameworkCreate, complianceHandler.ImportCatalog)

	protected.Get("/compliance/frameworks", complianceFrameworkRead, complianceHandler.ListFrameworks)
	protected.Post("/compliance/frameworks", complianceFrameworkCreate, complianceHandler.CreateFramework)
	protected.Get("/compliance/frameworks/:frameworkId", complianceFrameworkRead, complianceHandler.GetFramework)
	protected.Delete("/compliance/frameworks/:frameworkId", complianceFrameworkDelete, complianceHandler.DeleteFramework)
	protected.Get("/compliance/frameworks/:frameworkId/progress", complianceControlRead, complianceHandler.GetProgress)
	// Gap analysis ("analyse d'écarts") — every unsatisfied control across the
	// tenant's frameworks (optional ?framework_id= scopes to one).
	protected.Get("/compliance/gap-analysis", complianceControlRead, complianceHandler.GetGapAnalysis)
	// The head start: how much of a framework the tenant's existing proof already
	// answers, through the crosswalks. Read tier — it reports, it changes nothing.
	protected.Get("/compliance/frameworks/:frameworkId/inherited-coverage", complianceControlRead, complianceHandler.GetInheritedCoverage)
	// Cross-framework control mappings ("cross-mapping entre référentiels"). Static
	// path — no :param collision with /compliance/controls/:controlId.
	protected.Get("/compliance/control-mappings", complianceControlRead, complianceHandler.ListControlMappings)
	protected.Post("/compliance/control-mappings", complianceControlUpdate, complianceHandler.CreateControlMapping)
	protected.Delete("/compliance/control-mappings/:mappingId", complianceControlUpdate, complianceHandler.DeleteControlMapping)
	// Official compliance report (PDF, 1-click) — reads a tenant's controls/evidence, same tier as reading them.
	protected.Get("/compliance/frameworks/:frameworkId/report", complianceControlRead, complianceHandler.GenerateReport)
	protected.Get("/compliance/frameworks/:frameworkId/controls", complianceControlRead, complianceHandler.ListControls)
	protected.Post("/compliance/frameworks/:frameworkId/controls", complianceControlCreate, complianceHandler.CreateControl)
	protected.Get("/compliance/controls/:controlId", complianceControlRead, complianceHandler.GetControl)
	protected.Patch("/compliance/controls/:controlId", complianceControlUpdate, complianceHandler.UpdateControl)
	protected.Delete("/compliance/controls/:controlId", complianceControlDelete, complianceHandler.DeleteControl)
	// Per-control evidence, served by the LIBRARY (application/evidence), not by
	// the old one-file-one-control use cases. The paths are unchanged so existing
	// clients keep working; what changes is that an upload here lands in the
	// library and can then be reused elsewhere, and that the list carries derived
	// status and expiry. Two writers into one register would mean two truths, so
	// the compliance handler's evidence methods are no longer mounted.
	protected.Get("/compliance/controls/:controlId/evidences", complianceEvidenceRead, evidenceHandler.ListByControl)
	protected.Post("/compliance/controls/:controlId/evidences", complianceEvidenceCreate, evidenceHandler.CreateForControl)
	protected.Get("/compliance/evidences/:evidenceId/download", complianceEvidenceRead, evidenceHandler.Download)
	protected.Delete("/compliance/evidences/:evidenceId", complianceEvidenceDelete, evidenceHandler.Delete)

	// -------------------------------------------------------------------------
	// Evidence library (spec §1). One artifact, N controls, an expiry and a
	// review verdict — plus the "missing evidence" worklist per framework.
	//
	// Mounted under /evidence rather than /compliance/evidences because the
	// library is not a property of the compliance module: the same certificate
	// answers a framework control, a customer questionnaire and an audit finding.
	// Static sub-paths (/missing) are registered BEFORE /:evidenceId — the Fiber
	// trap this codebase has hit on every module.
	// -------------------------------------------------------------------------
	protected.Get("/evidence/missing", complianceEvidenceRead, evidenceHandler.Missing)
	protected.Get("/evidence", complianceEvidenceRead, evidenceHandler.List)
	protected.Post("/evidence", complianceEvidenceCreate, evidenceHandler.Create)
	protected.Get("/evidence/:evidenceId", complianceEvidenceRead, evidenceHandler.Get)
	protected.Patch("/evidence/:evidenceId", complianceEvidenceCreate, evidenceHandler.Update)
	protected.Delete("/evidence/:evidenceId", complianceEvidenceDelete, evidenceHandler.Delete)
	protected.Get("/evidence/:evidenceId/download", complianceEvidenceRead, evidenceHandler.Download)
	// Reuse: attach an artifact the tenant already holds to further controls.
	protected.Post("/evidence/:evidenceId/links", complianceEvidenceCreate, evidenceHandler.Link)
	protected.Delete("/evidence/:evidenceId/links/:controlId", complianceEvidenceCreate, evidenceHandler.Unlink)
	// Accepting or rejecting proof is a review verdict, not a create — it rides
	// the same tier as attaching evidence.
	protected.Post("/evidence/:evidenceId/review", complianceEvidenceCreate, evidenceHandler.Review)

	// -------------------------------------------------------------------------
	// Compliance audits ("Audits") + remediation plans ("Plans de remédiation").
	// One Gorm repo backs both aggregates. New permission strings — admin/root
	// hold "*" so they're granted; a future Profile rule can open them per-role.
	// -------------------------------------------------------------------------
	complianceAuditRepo := repository.NewGormComplianceAuditRepository(database.DB)
	complianceAuditHandler := handlers.NewComplianceAuditHandler(
		complianceaudit.NewCreateAuditUseCase(complianceAuditRepo),
		complianceaudit.NewListAuditsUseCase(complianceAuditRepo),
		complianceaudit.NewGetAuditUseCase(complianceAuditRepo),
		complianceaudit.NewUpdateAuditUseCase(complianceAuditRepo),
		complianceaudit.NewDeleteAuditUseCase(complianceAuditRepo),
		complianceaudit.NewCreateRemediationUseCase(complianceAuditRepo, complianceRepo),
		complianceaudit.NewListRemediationsUseCase(complianceAuditRepo, complianceRepo),
		complianceaudit.NewUpdateRemediationUseCase(complianceAuditRepo),
		complianceaudit.NewDeleteRemediationUseCase(complianceAuditRepo),
		complianceaudit.NewGenerateRemediationsFromAuditUseCase(complianceAuditRepo, complianceAuditRepo, complianceRepo),
	)
	complianceAuditRead := middleware.RequirePermission("compliance:audits:read")
	complianceAuditWrite := middleware.RequirePermission("compliance:audits:write")
	complianceRemediationRead := middleware.RequirePermission("compliance:remediations:read")
	complianceRemediationWrite := middleware.RequirePermission("compliance:remediations:write")

	// Static paths — registered as siblings of /compliance/frameworks etc.; no
	// dynamic :segment under /compliance would greedily catch "audits"/"remediations".
	protected.Get("/compliance/audits", complianceAuditRead, complianceAuditHandler.ListAudits)
	protected.Post("/compliance/audits", complianceAuditWrite, complianceAuditHandler.CreateAudit)
	protected.Get("/compliance/audits/:id", complianceAuditRead, complianceAuditHandler.GetAudit)
	protected.Patch("/compliance/audits/:id", complianceAuditWrite, complianceAuditHandler.UpdateAudit)
	protected.Delete("/compliance/audits/:id", complianceAuditWrite, complianceAuditHandler.DeleteAudit)
	// One-click: open a remediation plan for every open gap under the audit's framework.
	protected.Post("/compliance/audits/:id/generate-remediations", complianceRemediationWrite, complianceAuditHandler.GenerateRemediations)

	protected.Get("/compliance/remediations", complianceRemediationRead, complianceAuditHandler.ListRemediations)
	protected.Post("/compliance/remediations", complianceRemediationWrite, complianceAuditHandler.CreateRemediation)
	protected.Patch("/compliance/remediations/:id", complianceRemediationWrite, complianceAuditHandler.UpdateRemediation)
	protected.Delete("/compliance/remediations/:id", complianceRemediationWrite, complianceAuditHandler.DeleteRemediation)

	// =========================================================================
	// Board Report (M4, second half — see ROADMAP.md §3 M4).
	// Monthly, non-technical board-of-directors report: aggregates the tenant's
	// risk/compliance posture, estimates financial exposure in FCFA, and asks an
	// AI advisor to write the narrative. The LLM is best-effort: when
	// ANTHROPIC_API_KEY is set a ClaudeAdvisor (claude-opus-4-8) writes the prose;
	// otherwise (or on any API error) a deterministic TemplateAdvisor does, so the
	// feature works out of the box with no key. Human-in-the-loop: reports are
	// generated as drafts, editable, and must be approved before diffusion.
	// =========================================================================
	boardRepo := repository.NewGormBoardReportRepository(database.DB)
	boardAdvisor := ai.NewAdvisor(os.Getenv("ANTHROPIC_API_KEY"), os.Getenv("ANTHROPIC_MODEL"))
	if _, isTemplate := boardAdvisor.(*ai.TemplateAdvisor); isTemplate {
		log.Println("Board Report: no ANTHROPIC_API_KEY set — using deterministic template advisor")
	} else {
		log.Printf("Board Report: Claude advisor enabled (%s)", boardAdvisor.Name())
	}
	generateBoardUC := board.NewGenerateBoardReportUseCase(
		boardRepo, riskRepo, complianceRepo, orgRepo, boardAdvisor, board.DefaultExposureModel(),
	).WithActivation(activationRecorder)
	getBoardUC := board.NewGetBoardReportUseCase(boardRepo)
	listBoardUC := board.NewListBoardReportsUseCase(boardRepo)
	updateBoardUC := board.NewUpdateBoardReportUseCase(boardRepo)
	approveBoardUC := board.NewApproveBoardReportUseCase(boardRepo)
	deleteBoardUC := board.NewDeleteBoardReportUseCase(boardRepo)
	boardHandler := handlers.NewBoardReportHandler(
		generateBoardUC, getBoardUC, listBoardUC, updateBoardUC, approveBoardUC, deleteBoardUC, userRepo,
	)

	// admin/root-only today (same permission model as compliance — see the note above).
	boardRead := middleware.RequirePermission("reports:board:read")
	boardCreate := middleware.RequirePermission("reports:board:create")
	boardUpdate := middleware.RequirePermission("reports:board:update")
	boardApprove := middleware.RequirePermission("reports:board:approve")
	boardDelete := middleware.RequirePermission("reports:board:delete")

	protected.Get("/reports/board", boardRead, boardHandler.List)
	protected.Post("/reports/board", boardCreate, boardHandler.Generate)
	protected.Get("/reports/board/:reportId", boardRead, boardHandler.Get)
	protected.Patch("/reports/board/:reportId", boardUpdate, boardHandler.Update)
	protected.Post("/reports/board/:reportId/approve", boardApprove, boardHandler.Approve)
	protected.Delete("/reports/board/:reportId", boardDelete, boardHandler.Delete)
	protected.Get("/reports/board/:reportId/pdf", boardRead, boardHandler.DownloadPDF)

	// ---------------------------------------------------------------------
	// Report jobs — a generated report is an addressable artifact.
	//
	// This is what breaks the Compliance <-> Reports navigation loop: asking for
	// a compliance report used to send the user to the Reports screen, whose
	// Compliance tile sent them back, producing nothing. POST /reports/jobs
	// renders the document and returns a job the client redirects to, so the
	// request ends on the artifact instead of on the screen it started from.
	//
	// Guarded by the compliance read tier: a report reveals exactly the controls
	// and evidence the caller may already read, so requiring more would lock
	// auditors out of the one deliverable they exist to produce.
	reportJobRepo := repository.NewGormReportJobRepository(database.DB)
	reportJobUC := reportjob.New(
		reportJobRepo,
		reportjob.NewComplianceGenerator(generateReportUC),
	)
	reportJobHandler := handlers.NewReportJobHandler(reportJobUC)

	// Static sub-path before the parameterised one (Fiber matches in order).
	protected.Get("/reports/jobs", complianceControlRead, reportJobHandler.List)
	protected.Post("/reports/jobs", complianceControlRead, reportJobHandler.Create)
	protected.Get("/reports/jobs/:jobId", complianceControlRead, reportJobHandler.Get)
	protected.Get("/reports/jobs/:jobId/download", complianceControlRead, reportJobHandler.Download)

	// Assets (M3 — see ROADMAP.md §3). Previously these two routes bypassed
	// RBAC entirely (any authenticated user, any role, could write inventory
	// data) — now gated the same way as risks/compliance.
	assetRepo := repository.NewGormAssetRepository(database.DB)
	assetDepRepo := repository.NewGormAssetDependencyRepository(database.DB)

	// Typed attributes (Attack Surface §1). One schema per (tenant, category);
	// the SAME service backs the form generator (read), the tenant's schema
	// editor (write) and the server-side write-path validator, so the form can
	// never render a field the validator would reject.
	assetSchemaSvc := assetschema.NewService(repository.NewGormAssetTypeSchemaRepository(database.DB))
	assetAttrValidator := assetschema.NewValidator(assetSchemaSvc)

	createAssetUC := assetapp.NewCreateAssetUseCase(assetRepo).
		WithActivation(activationRecorder).
		WithAttributeValidator(assetAttrValidator)
	getAssetUC := assetapp.NewGetAssetUseCase(assetRepo)
	listAssetsUC := assetapp.NewListAssetsUseCase(assetRepo)
	updateAssetUC := assetapp.NewUpdateAssetUseCase(assetRepo).
		WithAttributeValidator(assetAttrValidator)
	// Deleting an asset also prunes its dependency edges (no dangling links).
	deleteAssetUC := assetapp.NewDeleteAssetUseCase(assetRepo).WithDependencyRepository(assetDepRepo)
	// Resolve each history snapshot's changed_by UUID to an email so the
	// inventory's history view shows "qui a modifié" as a human name, not a UUID.
	listAssetSnapshotsUC := assetapp.NewListAssetSnapshotsUseCase(assetRepo).WithUserLookup(userRepo)
	// The inventory's shape, counted in SQL rather than reduced in the browser
	// from the whole collection (W0-06). assetRepo satisfies the narrow
	// StatisticsReader port structurally — the method is concrete, off the
	// domain port, so no mock in the suite has to grow it.
	assetStatisticsUC := assetapp.NewAssetStatisticsUseCase(assetRepo)
	assetHandler := handlers.NewAssetHandler(
		createAssetUC, getAssetUC, listAssetsUC, updateAssetUC, deleteAssetUC, listAssetSnapshotsUC,
		assetStatisticsUC,
		redisClientInstance,
	)

	// Asset dependency graph (cartography). Both endpoints must belong to the
	// tenant; edges cascade-delete when either asset is removed.
	assetDepHandler := handlers.NewAssetDependencyHandler(
		assetapp.NewListAssetDependenciesUseCase(assetDepRepo),
		assetapp.NewCreateAssetDependencyUseCase(assetDepRepo, assetRepo),
		assetapp.NewDeleteAssetDependencyUseCase(assetDepRepo),
	)

	assetRead := middleware.RequirePermission("assets:read")
	assetCreate := middleware.RequirePermission("assets:create")
	assetUpdate := middleware.RequirePermission("assets:update")
	assetDelete := middleware.RequirePermission("assets:delete")

	protected.Get("/assets", assetRead, assetHandler.ListAssets)
	protected.Post("/assets", assetCreate, capAssets, assetHandler.CreateAsset)
	// NB: register the static /asset-dependencies resource as a sibling of
	// /assets (not /assets/:id/...) so "dependencies" is never parsed as an
	// asset UUID.
	// Same Fiber trap as /asset-dependencies below: a static sub-path of /assets
	// must be registered BEFORE /assets/:id, or "statistics" is parsed as an
	// asset UUID and the route answers 400 for a request that is perfectly valid.
	protected.Get("/assets/statistics", assetRead, assetHandler.GetAssetStatistics)
	protected.Get("/asset-dependencies", assetRead, assetDepHandler.ListAssetDependencies)
	protected.Post("/asset-dependencies", assetUpdate, assetDepHandler.CreateAssetDependency)
	protected.Delete("/asset-dependencies/:id", assetUpdate, assetDepHandler.DeleteAssetDependency)
	protected.Get("/assets/:id", assetRead, assetHandler.GetAsset)
	protected.Patch("/assets/:id", assetUpdate, assetHandler.UpdateAsset)
	protected.Delete("/assets/:id", assetDelete, assetHandler.DeleteAsset)
	protected.Get("/assets/:id/history", assetRead, assetHandler.GetAssetHistory)

	// Attack Surface — typed attribute schemas. Reading is open to anyone who
	// can read assets (the form generator needs it); editing the schema is an
	// admin act, because it changes the contract every asset of that category is
	// validated against.
	assetSchemaHandler := handlers.NewAssetSchemaHandler(assetSchemaSvc)
	adminOnly := middleware.RequireRole("admin", "root")
	protected.Get("/attack-surface/schemas", assetRead, assetSchemaHandler.ListSchemas)
	protected.Get("/attack-surface/schemas/:category", assetRead, assetSchemaHandler.GetSchema)
	protected.Put("/attack-surface/schemas/:category", adminOnly, assetSchemaHandler.UpdateSchema)
	protected.Post("/attack-surface/schemas/:category/reset", adminOnly, assetSchemaHandler.ResetSchema)

	// Saved table views (#580). Cross-cutting: the SAME resource backs all seven
	// registers (risks, vulnerabilities, assets, mitigations, incidents,
	// governance, settings), keyed by the table_id the frontend DataTable is
	// mounted with.
	//
	// No RBAC middleware, deliberately. No single "<module>:read" permission fits
	// a resource shared by seven registers, and a saved view grants access to
	// nothing — applying one still runs the register's own query under the
	// caller's own permissions. What IS enforced on every call is the tenant and
	// user identity taken from the signed session: a view of another tenant reads
	// back 404, identical to a fabricated id, because a 403 would confirm it
	// exists and its filter values name that institution's assets and people.
	savedViewRepo := repository.NewGormSavedViewRepository(database.DB)
	savedViewHandler := handlers.NewSavedViewHandler(
		savedviewapp.NewListSavedViewsUseCase(savedViewRepo).WithUserLookup(userRepo),
		savedviewapp.NewCreateSavedViewUseCase(savedViewRepo),
		savedviewapp.NewUpdateSavedViewUseCase(savedViewRepo),
		savedviewapp.NewDeleteSavedViewUseCase(savedViewRepo),
	)
	protected.Get("/saved-views", savedViewHandler.ListSavedViews)
	protected.Post("/saved-views", savedViewHandler.CreateSavedView)
	protected.Patch("/saved-views/:id", savedViewHandler.UpdateSavedView)
	protected.Delete("/saved-views/:id", savedViewHandler.DeleteSavedView)

	// Attack Surface — topology. Wired later than the asset block because the
	// node badges need the vulnerability repository; see the "topology wiring"
	// block after the vulnerability module.

	// --- Vulnerability Management (Module 3) — integrations + risk-based
	// prioritisation. Findings from Nessus/OpenVAS/Qualys/Defender/Inspector/
	// Azure Defender/CrowdStrike are normalised (internal/vulnscan), scored by
	// pkg/vulnprio (CVSS + exploitability + business criticality + affected
	// assets) and upserted into a tenant-scoped register.
	vulnRepo := repository.NewGormVulnerabilityRepository(database.DB)
	// Enrich ingested findings against the CTI feed (CISA-KEV / CVSS / severity)
	// before prioritisation. A stateless repo instance on the shared DB — the CTI
	// handler wires its own later.
	vulnCTIEnricher := vulnapp.NewCTIRepoEnricher(repository.NewGormCTIRepository(database.DB))
	// Vulnerability→risk rule (Attack Surface §4). The condition used to be
	// hardcoded ("P1 or CISA-KEV"); it is now a tenant-configured rule, and every
	// risk it produces lands in DRAFT — a machine may propose, never enrol.
	vulnRiskRuleRepo := repository.NewGormVulnRiskRuleRepository(database.DB)
	assetExposure := repository.NewGormAssetExposureLookup(database.DB)

	vulnIngestUC := vulnapp.NewIngestUseCase(vulnRepo, assetRepo).
		WithCTIEnricher(vulnCTIEnricher).
		WithRiskProposer(vulnrisk.NewRiskCreator(database.DB)).
		WithRiskRules(vulnRiskRuleRepo).
		WithAssetExposure(assetExposure)
	vulnHandler := handlers.NewVulnerabilityHandler(
		vulnIngestUC,
		vulnapp.NewListUseCase(vulnRepo),
		vulnapp.NewGetUseCase(vulnRepo),
		vulnapp.NewUpdateStatusUseCase(vulnRepo),
		vulnapp.NewDeleteUseCase(vulnRepo),
		vulnapp.NewStatsUseCase(vulnRepo),
	)
	vulnRead := middleware.RequirePermission("vulnerabilities:read")
	vulnWrite := middleware.RequirePermission("vulnerabilities:update")
	vulnDelete := middleware.RequirePermission("vulnerabilities:delete")

	// --- Smart Risk Calculation (spec §8 "Calcul de risque intelligent") ---
	// The multifactor risk score: blends business criticality, internet exposure,
	// vulnerabilities/CVSS, control maturity, incident history, exploitability,
	// financial value and live threat intel (CTI) via pkg/scoring.ComputeSmart with
	// per-tenant CONFIGURABLE weights. The classic Score Engine (P × I × AC) is
	// untouched — this is an additional, richer view. Every signal source is wired
	// here from the already-built repositories; each is nil-safe in the use case.
	smartWeightsRepo := repository.NewGormRiskScoringWeightsRepository(database.DB)
	computeSmartUC := risk.NewComputeSmartScoreUseCase(riskRepo, smartWeightsRepo).
		WithAssetRepo(assetRepo).
		WithVulnLister(vulnRepo).
		WithCompliance(complianceRepo).
		WithIncidents(repository.NewGormIncidentCounter(database.DB)).
		WithQuantifier(riskQuantifier).
		WithPersister(riskRepo)
	smartScoreHandler := handlers.NewSmartScoreHandler(
		computeSmartUC,
		risk.NewGetRiskWeightsUseCase(smartWeightsRepo),
		risk.NewUpdateRiskWeightsUseCase(smartWeightsRepo),
	)
	// Per-risk multifactor score + breakdown (read), and a non-persisting simulator
	// for live weight tuning. Static "risk-scoring" path is a sibling of /risks.
	protected.Get("/risks/:id/smart-score",
		middleware.RequirePermission("risks:read"), featSmartScore, smartScoreHandler.GetRiskSmartScore)
	protected.Post("/risks/:id/smart-score/simulate",
		middleware.RequirePermission("risks:read"), featSmartScore, smartScoreHandler.SimulateRiskSmartScore)
	// Per-tenant factor weights: read for anyone with risks:read, write admin-only.
	protected.Get("/risk-scoring/weights",
		middleware.RequirePermission("risks:read"), smartScoreHandler.GetRiskWeights)
	protected.Put("/risk-scoring/weights",
		middleware.RequireRole("admin"), smartScoreHandler.UpdateRiskWeights)

	// Connector + ticketing configuration. Credentials are AES-256-GCM encrypted
	// with the same key family as the scanner (SCANNER_CREDENTIAL_KEY) and are
	// never returned to the API. A tenant can wire the 7 external tools, an inbound
	// webhook token, and its ITSM (Jira/ServiceNow) here.
	vulnIntegKeyRaw := os.Getenv("SCANNER_CREDENTIAL_KEY")
	if vulnIntegKeyRaw == "" {
		vulnIntegKeyRaw = "openrisk-dev-scanner-credential-key-change-me"
	}
	vulnIntegCipher, vulnIntegCipherErr := scanapp.NewCredentialCipher([]byte(vulnIntegKeyRaw))
	if vulnIntegCipherErr != nil {
		log.Fatalf("failed to init vulnerability integration cipher: %v", vulnIntegCipherErr)
	}
	vulnIntegRepo := repository.NewGormVulnIntegrationRepository(database.DB)
	// Auto-ticketing: the opener composes the tenant ITSM config + Jira/ServiceNow
	// providers (pkg/ticketing). Wired into ingest (auto-open for P1/KEV) and into
	// the manual "Open ticket" use case. Mutating vulnIngestUC here still affects the
	// already-built handlers — they hold the same *IngestUseCase pointer.
	vulnTicketOpener := vulnapp.NewConfigTicketOpener(vulnIntegRepo, vulnIntegCipher)
	vulnIngestUC.WithTicketOpener(vulnTicketOpener)
	vulnLivePullUC := vulnapp.NewTriggerLivePullUseCase(vulnIntegRepo, vulnIntegCipher, vulnapp.LivePullAdapter{}, vulnIngestUC)
	vulnIntegHandler := handlers.NewVulnIntegrationHandler(
		vulnapp.NewSaveIntegrationUseCase(vulnIntegRepo, vulnIntegCipher),
		vulnapp.NewListIntegrationsUseCase(vulnIntegRepo),
		vulnapp.NewGetIntegrationUseCase(vulnIntegRepo),
		vulnapp.NewDeleteIntegrationUseCase(vulnIntegRepo),
		vulnLivePullUC,
		vulnapp.NewSaveTicketingUseCase(vulnIntegRepo, vulnIntegCipher),
		vulnapp.NewGetTicketingUseCase(vulnIntegRepo),
		vulnapp.NewDeleteTicketingUseCase(vulnIntegRepo),
		vulnapp.NewCreateTicketUseCase(vulnRepo, vulnTicketOpener),
	)
	// Assign the forward-declared webhook handler (route mounted before the JWT gate).
	vulnWebhookHandler = handlers.NewVulnWebhookHandler(vulnIntegRepo, vulnIngestUC)

	// Static resources first so they are never parsed as /:id.
	protected.Get("/vulnerability-connectors", vulnRead, vulnHandler.ListConnectors)
	protected.Get("/vulnerabilities/stats", vulnRead, vulnHandler.Stats)
	protected.Post("/vulnerabilities/ingest", vulnWrite, vulnHandler.Ingest)
	// Integration + ticketing config (static prefixes before /vulnerabilities/:id).
	protected.Get("/vulnerabilities/integrations", vulnRead, vulnIntegHandler.ListIntegrations)
	protected.Post("/vulnerabilities/integrations", vulnWrite, vulnIntegHandler.SaveIntegration)
	protected.Get("/vulnerabilities/integrations/:id", vulnRead, vulnIntegHandler.GetIntegration)
	protected.Post("/vulnerabilities/integrations/:id/pull", vulnWrite, vulnIntegHandler.TriggerPull)
	protected.Delete("/vulnerabilities/integrations/:id", vulnDelete, vulnIntegHandler.DeleteIntegration)
	protected.Get("/vulnerabilities/ticketing", vulnRead, vulnIntegHandler.GetTicketing)
	protected.Put("/vulnerabilities/ticketing", vulnWrite, vulnIntegHandler.SaveTicketing)
	protected.Delete("/vulnerabilities/ticketing", vulnDelete, vulnIntegHandler.DeleteTicketing)
	// Vulnerability↔asset correlation (Attack Surface §3). Static sub-paths
	// BEFORE /vulnerabilities/:id — "unassigned" must never be parsed as a UUID.
	vulnCorrelationHandler := handlers.NewVulnCorrelationHandler(
		vulnapp.NewListUnassignedUseCase(vulnRepo),
		vulnapp.NewGetCandidatesUseCase(vulnRepo, assetRepo),
		vulnapp.NewResolveAssetUseCase(vulnRepo, assetRepo),
	)
	protected.Get("/vulnerabilities/unassigned", vulnRead, vulnCorrelationHandler.ListUnassigned)

	// Attack Surface §4 — the vuln→risk rule and the review queue of the DRAFT
	// risks it proposes. Reading the rule is open to anyone who can read
	// vulnerabilities (the preview is how you understand what fires); changing
	// it is admin, because it changes what appears in everyone's register.
	vulnRiskRuleHandler := handlers.NewVulnRiskRuleHandler(
		vulnapp.NewRiskRuleService(vulnRiskRuleRepo, vulnRepo).WithAssetExposure(assetExposure),
		risk.NewDraftReviewUseCase(riskRepo),
	)
	protected.Get("/attack-surface/risk-rule", vulnRead, vulnRiskRuleHandler.GetRule)
	protected.Put("/attack-surface/risk-rule", adminOnly, vulnRiskRuleHandler.UpdateRule)
	protected.Post("/attack-surface/risk-rule/preview", vulnRead, vulnRiskRuleHandler.PreviewRule)
	protected.Get("/attack-surface/draft-risks", middleware.RequirePermission("risks:read"), vulnRiskRuleHandler.ListDrafts)
	protected.Post("/attack-surface/draft-risks/review", middleware.RequirePermission("risks:update"), vulnRiskRuleHandler.BulkReview)

	protected.Get("/vulnerabilities", vulnRead, vulnHandler.List)
	protected.Get("/vulnerabilities/:id", vulnRead, vulnHandler.Get)
	protected.Patch("/vulnerabilities/:id/status", vulnWrite, vulnHandler.UpdateStatus)
	protected.Post("/vulnerabilities/:id/ticket", vulnWrite, vulnIntegHandler.CreateTicket)
	protected.Get("/vulnerabilities/:id/match-candidates", vulnRead, vulnCorrelationHandler.GetCandidates)
	protected.Put("/vulnerabilities/:id/asset", vulnWrite, vulnCorrelationHandler.ResolveAsset)
	protected.Delete("/vulnerabilities/:id", vulnDelete, vulnHandler.Delete)

	// --- Attack Surface topology wiring --------------------------------------
	// Placed here (rather than beside the other asset routes) because the node
	// badges read open-vulnerability counts, which needs vulnRepo — declared
	// just above. The vulnerability counter is optional: if it fails, the graph
	// still renders, minus one badge.
	assetTopologyHandler := handlers.NewAssetTopologyHandler(
		assetapp.NewGetTopologyUseCase(assetRepo, assetDepRepo).WithVulnCounter(vulnRepo),
		assetapp.NewGetCompromiseChainUseCase(assetRepo, assetDepRepo),
	)
	// Static sub-paths BEFORE /:id (the Fiber trap): "edge-types" must never be
	// parsed as an asset UUID.
	protected.Get("/attack-surface/topology/edge-types", assetRead, assetTopologyHandler.GetEdgeTypes)
	protected.Get("/attack-surface/topology", assetRead, assetTopologyHandler.GetTopology)
	protected.Get("/attack-surface/topology/:id/compromise-chain", assetRead, assetTopologyHandler.GetCompromiseChain)

	// =========================================================================
	// AI GRC Assistant (spec §12 — see ROADMAP.md Module 12).
	// Unified AI service over the tenant's own GRC data: treatment-plan
	// suggestions, emerging-risk detection, a natural-language Q&A assistant
	// (hybrid RAG over risks/controls/vulnerabilities), audit-report generation,
	// and evidence document analysis. The LLM is best-effort: when
	// ANTHROPIC_API_KEY is set a ClaudeAssistant (claude-opus-4-8) is used;
	// otherwise (or on any API error) a deterministic TemplateAssistant does, so
	// every endpoint works out of the box with no key. Reuses the same key/model
	// env vars as the board report.
	// =========================================================================
	aiAssistant := ai.NewAssistant(os.Getenv("ANTHROPIC_API_KEY"), os.Getenv("ANTHROPIC_MODEL"))
	if ai.IsLLMBacked(aiAssistant) {
		log.Printf("AI Assistant: Claude enabled (%s)", aiAssistant.Name())
	} else {
		log.Println("AI Assistant: no ANTHROPIC_API_KEY set — using deterministic template assistant")
	}
	aiTreatmentUC := appai.NewSuggestTreatmentPlanUseCase(aiAssistant, riskRepo).WithAssetReader(assetRepo)
	aiEmergingUC := appai.NewDetectEmergingRisksUseCase(aiAssistant).WithRiskLister(riskRepo)
	// One reader spanning both halves of the register (see evidence_wiring.go).
	aiComplianceReader := newAIComplianceReader(complianceRepo, evidenceRepo)
	aiQueryUC := appai.NewAssistantQueryUseCase(aiAssistant).
		WithRisks(riskRepo).
		WithCompliance(aiComplianceReader).
		WithVulns(vulnRepo).
		WithOrgs(orgRepo)
	aiAuditReportUC := appai.NewGenerateAuditReportUseCase(aiAssistant, complianceAuditRepo).WithGapAnalyzer(getGapAnalysisUC)
	aiEvidenceUC := appai.NewAnalyzeEvidenceUseCase(aiAssistant, aiComplianceReader)
	aiHandler := handlers.NewAIHandler(aiAssistant, aiTreatmentUC, aiEmergingUC, aiQueryUC, aiAuditReportUC, aiEvidenceUC)

	// AI features are advisory (non-mutating): guarded by the read permission of
	// the relevant module. The assistant/emerging endpoints use risks:read.
	aiRiskRead := middleware.RequirePermission("risks:read")
	aiComplianceRead := middleware.RequirePermission("compliance:read")
	// Deployment capability only ("is an LLM wired in, and which"). No tenant
	// data, no per-user data — session-sufficient (#529).
	protected.Get("/ai/status", aiHandler.Status)
	protected.Post("/ai/assistant/query", aiRiskRead, featAI, aiHandler.AssistantQuery)
	protected.Post("/ai/emerging-risks", aiRiskRead, featAI, aiHandler.DetectEmergingRisks)
	protected.Post("/ai/risks/:id/treatment-plan", aiRiskRead, featAI, aiHandler.SuggestTreatmentPlan)
	protected.Post("/ai/audits/:id/report", aiComplianceRead, featAI, aiHandler.GenerateAuditReport)
	protected.Post("/ai/evidence/:id/analyze", aiComplianceRead, featAI, aiHandler.AnalyzeEvidence)

	// (/users/me is already registered once above — duplicate registration removed for RC1.)
	api.Get("/stats/risk-matrix", cacheableHandlers.CacheDashboardMatrixGET(handlers.GetRiskMatrixData))
	api.Get("/stats/risk-distribution", cacheableHandlers.CacheDashboardStatsGET(handlers.GetRiskDistribution))
	api.Get("/stats/mitigation-metrics", cacheableHandlers.CacheDashboardStatsGET(handlers.GetMitigationMetrics))
	api.Get("/stats/top-vulnerabilities", cacheableHandlers.CacheDashboardStatsGET(handlers.GetTopVulnerabilities))
	api.Get("/export/pdf", handlers.ExportRisksPDF)
	api.Get("/stats/trends", middleware.Protected(rsaKeys, jtiBlacklistChecker), cacheableHandlers.CacheDashboardTimelineGET(handlers.GetGlobalRiskTrend))
	// TODO(Phase 3): Reconnect /mitigations/recommended once the handler is
	// properly implemented with tests and pagination.
	// api.Get("/mitigations/recommended", handlers.GetRecommendedMitigations)
	api.Get("/gamification/me", middleware.Protected(rsaKeys, jtiBlacklistChecker), handlers.GetMyGamificationProfile)

	// --- Score Engine Management (Protected routes) ---
	scoreEngineHandler := handlers.NewScoreEngineHandler(database.DB, scoreEngineService)
	scoreEngineRoutes := protected.Group("/score-engine")
	scoreEngineRoutes.Get("/configs", scoreEngineHandler.GetScoringConfigs)
	scoreEngineRoutes.Get("/configs/:id", scoreEngineHandler.GetScoringConfig)
	scoreEngineRoutes.Post("/configs", middleware.RequireRole("admin"), scoreEngineHandler.CreateScoringConfig)
	scoreEngineRoutes.Put("/configs/:id", middleware.RequireRole("admin"), scoreEngineHandler.UpdateScoringConfig)
	scoreEngineRoutes.Post("/compute", scoreEngineHandler.ComputeRiskScore)
	scoreEngineRoutes.Get("/matrix", scoreEngineHandler.GetRiskMatrix)
	scoreEngineRoutes.Post("/classify", scoreEngineHandler.ClassifyRisk)
	scoreEngineRoutes.Get("/metrics", scoreEngineHandler.GetScoringMetrics)

	// --- User Management (Admin only) ---
	adminRole := middleware.RequireRole("admin")
	protected.Get("/users", adminRole, handlers.GetUsers)
	protected.Post("/users", adminRole, handlers.CreateUser)
	protected.Patch("/users/:id/status", adminRole, handlers.UpdateUserStatus)
	protected.Patch("/users/:id/role", adminRole, handlers.UpdateUserRole)
	protected.Delete("/users/:id", adminRole, handlers.DeleteUser)
	// Despite the ":id", this edits the CALLER's own profile: the handler reads
	// claims.Sub and ignores the parameter, so it cannot touch another account.
	// Session-sufficient (#529); the misleading path is issue #574.
	protected.Patch("/users/:id", handlers.UpdateUserProfile)

	// --- Team Management (Admin only) ---
	protected.Post("/teams", adminRole, handlers.CreateTeam)
	protected.Get("/teams", adminRole, handlers.GetTeams)
	protected.Get("/teams/:id", adminRole, handlers.GetTeam)
	protected.Patch("/teams/:id", adminRole, handlers.UpdateTeam)
	protected.Delete("/teams/:id", adminRole, handlers.DeleteTeam)
	protected.Post("/teams/:id/members/:userId", adminRole, handlers.AddTeamMember)
	protected.Delete("/teams/:id/members/:userId", adminRole, handlers.RemoveTeamMember)

	// --- Integration Testing (Protected routes) ---
	// Fetches a caller-supplied URL from the server and reports the result, so
	// it must not be reachable by every member (#529). The guard narrows who can
	// aim it; issue #573 removes the arbitrary-URL shape itself.
	protected.Post("/integrations/:id/test", middleware.RequireRole("admin", "root"), handlers.TestIntegration)

	// --- Audit Logs (Admin only) ---
	auditHandler := handlers.NewAuditLogHandler()
	protected.Get("/audit-logs", adminRole, auditHandler.GetAuditLogs)
	protected.Get("/audit-logs/user/:user_id", adminRole, auditHandler.GetUserAuditLogs)
	protected.Get("/audit-logs/action/:action", adminRole, auditHandler.GetAuditLogsByAction)

	// --- API Token Management (Protected routes) ---
	// Tokens can be managed by any authenticated user for their own tokens
	tokenHandler := handlers.NewTokenHandler(tokenService)

	// API tokens are personal: every verb below loads the token and refuses when
	// token.UserID is not the caller. The session is the authorization (#529).
	protected.Post("/tokens", tokenHandler.CreateToken)
	protected.Get("/tokens", tokenHandler.ListTokens)
	protected.Get("/tokens/:id", tokenHandler.GetToken)
	protected.Put("/tokens/:id", tokenHandler.UpdateToken)
	protected.Post("/tokens/:id/revoke", tokenHandler.RevokeToken)
	protected.Post("/tokens/:id/rotate", tokenHandler.RotateToken)
	protected.Delete("/tokens/:id", tokenHandler.DeleteToken)

	// --- Custom Fields Management (Protected routes) ---
	customFieldHandler := handlers.NewCustomFieldHandler()
	// Reading the field definitions is what every risk and asset FORM does, so
	// any member may. Defining them changes the tenant's schema for everyone —
	// and deleting one drops the values already captured under it — which is an
	// administrator's call (#529).
	customFieldAdmin := middleware.RequireRole("admin", "root")
	protected.Post("/custom-fields", customFieldAdmin, customFieldHandler.CreateCustomField)
	protected.Get("/custom-fields", customFieldHandler.ListCustomFields)
	protected.Get("/custom-fields/:id", customFieldHandler.GetCustomField)
	protected.Patch("/custom-fields/:id", customFieldAdmin, customFieldHandler.UpdateCustomField)
	protected.Delete("/custom-fields/:id", customFieldAdmin, customFieldHandler.DeleteCustomField)
	protected.Get("/custom-fields/scope/:scope", customFieldHandler.ListCustomFieldsByScope)
	protected.Post("/custom-fields/templates/:id/apply", customFieldAdmin, customFieldHandler.ApplyTemplate)

	// --- Bulk Operations (Protected routes) ---
	bulkOpHandler := handlers.NewBulkOperationHandler()
	// A bulk job mutates the register wholesale. The middleware floor keeps
	// read-only roles out entirely; the handler then demands the exact
	// permission for the verb in the body (update/delete/export/assign), which
	// middleware cannot see (#529).
	protected.Post("/bulk-operations",
		middleware.RequirePermission("risks:update", "risks:delete"),
		bulkOpHandler.CreateBulkOperation)
	// Job status for the caller's own jobs, tenant-scoped. Session-sufficient (#529).
	protected.Get("/bulk-operations", bulkOpHandler.ListBulkOperations)
	protected.Get("/bulk-operations/:id", bulkOpHandler.GetBulkOperation)

	// --- Incidents (Protected routes) ---
	incidentService := service.NewIncidentService(database.DB)

	// The structured review. Its two collaborators — the multi-channel dispatcher
	// and the mitigation use case — are built further down (the automation block),
	// so the service is attached to them there rather than duplicating either.
	postMortemRepo := repository.NewGormIncidentPostMortemRepository(database.DB)
	postMortemService := appinc.NewPostMortemService(postMortemRepo, incidentService).
		WithMitigations(incinfra.NewMitigationCreator(
			appmitigation.NewCreateMitigationPlanUseCase(
				repository.NewGormMitigationRepository(database.DB),
				repository.NewGormMitigationSubActionRepository(database.DB),
			).WithOwnership(handlers.OwnershipServiceInstance()),
		)).
		WithUserLookup(userRepo)
	incidentService.WithPostMortemGate(incinfra.NewPostMortemGate(postMortemService))

	// =========================================================================
	// Reporting engine (spec §5). Asynchronous generation into PDF / DOCX /
	// XLSX, six report types, a document language chosen independently of the
	// interface language, a printed integrity hash, and an editorial lifecycle
	// (draft → in_review → approved → published) with comments and versions.
	//
	// The sources are the repositories the reports read; every one is optional,
	// so a deployment missing a module produces the other five reports and says
	// what is unavailable rather than failing.
	// =========================================================================
	reportRepo := repository.NewGormReportRepository(database.DB)
	reportSources := appreport.Sources{
		Compliance: complianceRepo,
		Evidence:   evidenceRepo,
		Risks:      riskRepo,
		Incidents:  reportIncidentAdapter{svc: incidentService},
		Audits:     reportAuditAdapter{repo: complianceAuditRepo},
		Org:        orgRepo,
	}
	reportService := appreport.NewService(reportRepo, reportSources).WithUserLookup(userRepo)
	reportHandler := handlers.NewReportHandler(reportService, redisClientInstance)

	// The worker renders queued reports and publishes progress; the SSE endpoint
	// forwards it. Without Redis the client falls back to polling, which the
	// progress endpoint handles by sending the current state and closing.
	reportWorker := workers.NewReportWorker(reportRepo, reportSources, zeroLogger).
		WithOrgLookup(func(ctx context.Context, tenantID uuid.UUID) string {
			if org, err := orgRepo.GetByID(ctx, tenantID); err == nil && org != nil {
				return org.Name
			}
			return ""
		}).
		WithProgressPublisher(func(ctx context.Context, tenantID, reportID uuid.UUID, progress int, step, runState string) {
			if redisClientInstance == nil {
				return
			}
			payload, err := json.Marshal(map[string]any{
				"tenant_id": tenantID.String(), "report_id": reportID.String(),
				"progress": progress, "step": step, "run_state": runState,
			})
			if err != nil {
				return
			}
			// Best-effort: a dropped progress event costs a smoother bar, not the
			// document. The report row is always updated first.
			_ = redisClientInstance.Publish(ctx, handlers.ReportProgressChannel, string(payload))
		})
	go reportWorker.Start(context.Background())

	// Static sub-paths before the parameterised one (Fiber matches in order).
	protected.Get("/reports/types", complianceControlRead, reportHandler.Catalogue)
	protected.Get("/reports", complianceControlRead, reportHandler.List)
	protected.Post("/reports", complianceControlRead, reportHandler.Create)
	protected.Get("/reports/:reportId", complianceControlRead, reportHandler.Get)
	protected.Delete("/reports/:reportId", complianceControlRead, reportHandler.Delete)
	protected.Get("/reports/:reportId/download", complianceControlRead, reportHandler.Download)
	protected.Get("/reports/:reportId/verify", complianceControlRead, reportHandler.Verify)
	protected.Get("/reports/:reportId/progress", complianceControlRead, reportHandler.Progress)
	protected.Get("/reports/:reportId/versions", complianceControlRead, reportHandler.Versions)
	protected.Get("/reports/:reportId/compare", complianceControlRead, reportHandler.Compare)
	protected.Post("/reports/:reportId/comments", complianceControlRead, reportHandler.Comment)
	// Approving and publishing are editorial acts, not reads: they need the
	// board-report approval tier, which admins hold through the "*" wildcard.
	protected.Post("/reports/:reportId/transition", middleware.RequirePermission("reports:board:approve"), reportHandler.Transition)

	incidentHandler := handlers.NewIncidentHandler(incidentService).
		WithPostMortems(postMortemService)
	incidentsGroup := protected.Group("/incidents")
	incidentsGroup.Post("", incidentCreate, incidentHandler.CreateIncident)
	incidentsGroup.Get("/stats", incidentHandler.GetIncidentStats)
	// Static sub-paths before /:id (Fiber trap).
	incidentsGroup.Get("/origins", incidentHandler.ListIncidentOrigins)
	incidentsGroup.Get("", incidentHandler.ListIncidents)
	incidentsGroup.Get("/:id", incidentHandler.GetIncident)
	incidentsGroup.Put("/:id", incidentUpdate, incidentHandler.UpdateIncident)
	incidentsGroup.Delete("/:id", incidentDelete, incidentHandler.DeleteIncident)
	incidentsGroup.Get("/:id/timeline", incidentHandler.GetIncidentTimeline)
	incidentsGroup.Get("/:id/post-mortem", incidentHandler.GetPostMortem)
	incidentsGroup.Put("/:id/post-mortem", incidentUpdate, incidentHandler.SavePostMortem)
	incidentsGroup.Post("/:id/post-mortem/publish", incidentUpdate, incidentHandler.PublishPostMortem)
	incidentsGroup.Post("/:id/risks/:riskId", incidentUpdate, incidentHandler.LinkRisk)
	incidentsGroup.Post("/:id/actions", incidentUpdate, incidentHandler.CreateIncidentAction)
	incidentsGroup.Get("/:id/actions", incidentHandler.GetIncidentActions)
	incidentsGroup.Put("/:id/actions/:actionId", incidentUpdate, incidentHandler.UpdateIncidentAction)
	protected.Get("/risks/:id/incidents", middleware.RequirePermission("risks:read"), incidentHandler.GetIncidentsForRisk)

	// NOTE: the legacy /risk-management/* lifecycle subsystem (service +
	// handler + duplicate RiskRegister/TreatmentPlan/… models) was removed. Its
	// tables were never in AutoMigrate (every route 500'd) and its queries were
	// not tenant-scoped (cross-tenant leak). The ISO 31000 lifecycle now lives on
	// the real Risk entity: POST /risks/:id/transition (see riskHandler above).

	// --- Notifications (Protected routes) ---
	notificationRepo := repository.NewNotificationRepository(database.DB)
	notificationUseCase := notificationapp.NewUseCase(notificationRepo)
	notificationHandler := handlers.NewNotificationHandler(notificationUseCase)

	// Attack Surface §4: tell the tenant's admins when the vuln→risk rule
	// proposes a draft. Attached here (rather than at the ingest wiring above)
	// because the notification use case only exists at this point; With* mutates
	// the same pointer, so ordering is the only constraint. A draft nobody is
	// told about is a draft nobody reviews.
	vulnIngestUC.WithRiskProposalNotifier(vulnrisk.NewDraftRiskNotifier(database.DB, notificationUseCase))
	notificationsGroup := protected.Group("/notifications")
	notificationsGroup.Get("", notificationHandler.GetNotifications)
	notificationsGroup.Get("/unread-count", notificationHandler.GetUnreadCount)
	notificationsGroup.Patch("/read-all", notificationHandler.MarkAllAsRead)
	notificationsGroup.Patch("/:notificationId/read", notificationHandler.MarkAsRead)
	notificationsGroup.Delete("/:notificationId", notificationHandler.DeleteNotification)
	notificationsGroup.Get("/preferences", notificationHandler.GetNotificationPreferences)
	notificationsGroup.Patch("/preferences", notificationHandler.UpdateNotificationPreferences)
	notificationsGroup.Post("/test", notificationHandler.TestNotification)

	// --- Action Center (#429) --------------------------------------------------
	// A live, role-aware read over six existing tables. Registered on `protected`
	// so it inherits the same auth middleware as /notifications: the endpoint has
	// no identity of its own to fall back on, and an unauthenticated call must
	// reach the handler with an empty context and be refused, never be served.
	actionCenterHandler := handlers.NewActionCenterHandler(
		actioncenterapp.NewUseCase(repository.NewActionCenterRepository(database.DB)),
	)
	// "What is waiting on ME" — the use case builds it from the caller's own
	// assignments. Session-sufficient (#529).
	protected.Get("/action-center", actionCenterHandler.GetActionCenter)

	// --- Risk Timeline (Protected routes) ---
	timelineHandler := handlers.NewRiskTimelineHandler()
	// One risk's history is one risk's data: same permission as reading the risk
	// itself, which every other /risks/* read already carries (#529).
	riskTimelineRead := middleware.RequirePermission("risks:read")
	protected.Get("/risks/:id/timeline", riskTimelineRead, timelineHandler.GetRiskTimeline)
	protected.Get("/risks/:id/timeline/status-changes", riskTimelineRead, timelineHandler.GetStatusChanges)
	protected.Get("/risks/:id/timeline/score-changes", riskTimelineRead, timelineHandler.GetScoreChanges)
	protected.Get("/risks/:id/timeline/trend", riskTimelineRead, timelineHandler.GetRiskTrend)
	protected.Get("/risks/:id/timeline/changes/:type", riskTimelineRead, timelineHandler.GetChangesByType)
	protected.Get("/risks/:id/timeline/since/:timestamp", riskTimelineRead, timelineHandler.GetChangesSince)
	// GET /timeline/recent is RETIRED (#412 criterion 16). It returned the risk
	// history of every tenant in the deployment on 2026-07-23 (docs/JOURNAL.md
	// item 36), and it still read its tenant through safeGetUUID, which falls
	// back to uuid.Nil rather than failing closed. GET /timeline supersedes it,
	// is tenant-scoped from Caller alone, and is covered by a cross-tenant test.
	// TestIsolationGate_LegacyRecentTimelineIsRetired fails if it is re-mounted.

	// --- Universal Entity Drawer + Global Timeline (W1-02) ---
	//
	// Five routes for eight entity types. The type is a path parameter so the
	// permission gate is applied ONCE, from one descriptor table, instead of
	// thirty-two per-type routes each free to answer differently.
	//
	// Nothing here owns data. Every resolver composes a repository that is
	// already tenant-scoped, and the relation reader is the single place the
	// cross-module joins live so there is one file to audit rather than five.
	entityRelations := repository.NewGormEntityRelationRepository(database.DB)
	entityTimeline := entity.NewTimelineService(auditChainRepo).
		WithUserLookup(userRepo).
		// The journals the canonical audit trail structurally cannot see: the
		// score worker runs outside any request, and the incident service keeps
		// its own log.
		WithSource(entity.TypeRisk, repository.NewRiskHistorySource(database.DB)).
		WithSource(entity.TypeIncident, repository.NewIncidentTimelineSource(database.DB)).
		WithSource(entity.TypeAsset, repository.NewAssetSnapshotSource(database.DB)).
		WithSource(entity.TypeVendor, repository.NewAssetSnapshotSource(database.DB))

	// Bind joins each resolver to the tenant gate its type declares in
	// entity.isolationProfiles. Register panics on an incomplete registration,
	// so a type added without that decision fails here, at boot, in front of
	// whoever added it — rather than serving an unscoped read.
	entityRegistry := entity.NewRegistry().
		Register(entity.Bind(entity.TypeAsset, entity.NewAssetResolver(assetRepo, entityRelations))).
		Register(entity.Bind(entity.TypeVendor, entity.NewVendorResolver(assetRepo, entityRelations))).
		Register(entity.Bind(entity.TypeRisk, entity.NewRiskResolver(riskRepo, entityRelations))).
		Register(entity.Bind(entity.TypeVulnerability, entity.NewVulnerabilityResolver(vulnRepo, entityRelations))).
		Register(entity.Bind(entity.TypeFinding, entity.NewFindingResolver(vulnRepo, entityRelations))).
		Register(entity.Bind(entity.TypeControl, entity.NewControlResolver(complianceRepo, entityRelations))).
		Register(entity.Bind(entity.TypeIncident, entity.NewIncidentResolver(incidentService, entityRelations))).
		Register(entity.Bind(entity.TypeEvidence, entity.NewEvidenceResolver(evidenceRepo, entityRelations)))

	entityHandler := handlers.NewEntityHandler(entity.NewService(entityRegistry).WithTimeline(entityTimeline))

	// The tenant-wide feed and the catalogue are static paths and are mounted
	// BEFORE /entities/:type/:id (the Fiber trap this codebase has hit before).
	// The entity module authorises INSIDE the service: every read goes through
	// Service.access(type) → the type's own read permission, relations are
	// filtered by the target type's permission, and the audit tab additionally
	// demands governance:audit:read. Handler-enforced, not unguarded (#529).
	protected.Get("/timeline", entityHandler.GetTenantTimeline)
	protected.Get("/entities", entityHandler.GetCatalogue)
	// No RequirePermission middleware here on purpose: the required permission
	// depends on :type, so the gate has to be inside the service where the type
	// is known. Mounting a fixed guard would either lock out every caller who is
	// not an admin, or gate a control behind the risk permission.
	protected.Get("/entities/:type/:id", entityHandler.GetEntity)
	protected.Get("/entities/:type/:id/relations", entityHandler.GetRelations)
	protected.Get("/entities/:type/:id/timeline", entityHandler.GetTimeline)
	protected.Get("/entities/:type/:id/audit", entityHandler.GetAudit)

	// --- Analytics & Advanced Reporting (Protected routes) ---
	analyticsService := service.NewAnalyticsService(database.DB)
	analyticsHandler := handlers.NewAnalyticsHandler(analyticsService)
	protected.Get("/analytics/risks/metrics", analyticsHandler.GetRiskMetrics)
	protected.Get("/analytics/risks/trends", analyticsHandler.GetRiskTrends)
	protected.Get("/analytics/mitigations/metrics", analyticsHandler.GetMitigationMetrics)
	protected.Get("/analytics/frameworks", analyticsHandler.GetFrameworkAnalytics)
	protected.Get("/analytics/dashboard", analyticsHandler.GetDashboardSnapshot)
	protected.Get("/analytics/export", analyticsHandler.GetExportData)
	// Financial Risk Quantification dashboard (spec §9) — tenant-wide portfolio
	// ALE / worst-case / residual / remediation budget / ROSI for the CFO/CISO screen.
	protected.Get("/analytics/financial",
		middleware.RequirePermission("risks:read"), featFinancial, financialAnalyticsHandler.GetFinancialSummary)
	// Tenant display currency — chosen at onboarding, changeable here (admin).
	protected.Put("/analytics/financial/currency",
		middleware.RequireRole("admin", "root"), financialAnalyticsHandler.SetCurrency)

	// Executive dashboard (spec §11) — ONE consolidated, tenant-scoped aggregation
	// (cyber score, financial exposure, KRIs, top-10 risks, risk & incident trends,
	// compliance coverage) so the frontend makes a single request. Composes the
	// existing tenant-scoped sources; every source is nil-safe in the use case.
	executiveDashboardHandler := newExecutiveDashboardHandler(
		financialSummaryUseCase, riskRepo, getGapAnalysisUC, vulnRepo, incidentService, riskQuantifier,
		ahaRecorder,
	)
	protected.Get("/analytics/executive",
		middleware.RequirePermission("risks:read"), featExecutive, executiveDashboardHandler.GetExecutiveDashboard)

	// --- Enhanced Dashboard Analytics (Protected routes) ---
	dashboardDataService := service.NewDashboardDataService(database.DB, nil)
	enhancedDashboardHandler := handlers.NewEnhancedDashboardHandler(dashboardDataService)
	protected.Get("/dashboard/metrics", enhancedDashboardHandler.GetDashboardMetrics)
	protected.Get("/dashboard/risk-trends", enhancedDashboardHandler.GetRiskTrends)
	protected.Get("/dashboard/severity-distribution", enhancedDashboardHandler.GetSeverityDistribution)
	protected.Get("/dashboard/mitigation-status", enhancedDashboardHandler.GetMitigationStatus)
	protected.Get("/dashboard/top-risks", enhancedDashboardHandler.GetTopRisks)
	protected.Get("/dashboard/mitigation-progress", enhancedDashboardHandler.GetMitigationProgress)
	protected.Get("/dashboard/complete", enhancedDashboardHandler.GetCompleteDashboard)

	// --- Marketplace Management (Protected routes) ---
	// Marketplace can be browsed by all authenticated users
	// Installation requires analyst or admin role
	marketplaceService := service.NewMarketplaceService(database.DB, log.New(os.Stderr, "[Marketplace] ", log.LstdFlags))
	marketplaceHandler := handlers.NewMarketplaceHandler(marketplaceService)

	// Public marketplace endpoints (all authenticated users can browse)
	protected.Get("/marketplace/connectors", marketplaceHandler.ListConnectors)
	protected.Get("/marketplace/connectors/:id", marketplaceHandler.GetConnector)
	protected.Get("/marketplace/connectors/search", marketplaceHandler.SearchConnectors)

	// Protected marketplace endpoints (analysts and admins only).
	// NOTE: this MUST be a per-route guard, not `protected.Use(...)`. In Fiber,
	// group.Use() appends the middleware to the group itself, so every route
	// registered on `protected` AFTER this line (automation, CTI, scanner,
	// vulnerabilities, governance, the RBAC business-role endpoints, …) would
	// silently inherit this RequireRole gate and 403 any non-admin — which is
	// exactly why business-role 'user' members were locked out of everything
	// below this point. Scope it to the marketplace app routes only.
	marketplaceManage := middleware.RequireRole("admin", "analyst")
	protected.Post("/marketplace/apps", marketplaceManage, marketplaceHandler.InstallApp)
	protected.Get("/marketplace/apps", marketplaceManage, marketplaceHandler.ListApps)
	protected.Get("/marketplace/apps/:id", marketplaceManage, marketplaceHandler.GetApp)
	protected.Put("/marketplace/apps/:id", marketplaceManage, marketplaceHandler.UpdateApp)
	protected.Post("/marketplace/apps/:id/enable", marketplaceManage, marketplaceHandler.EnableApp)
	protected.Post("/marketplace/apps/:id/disable", marketplaceManage, marketplaceHandler.DisableApp)
	protected.Delete("/marketplace/apps/:id", marketplaceManage, marketplaceHandler.UninstallApp)
	protected.Put("/marketplace/apps/:id/sync", marketplaceManage, marketplaceHandler.UpdateAppSync)
	protected.Post("/marketplace/apps/:id/sync", marketplaceManage, marketplaceHandler.TriggerSync)
	protected.Get("/marketplace/apps/:id/logs", marketplaceManage, marketplaceHandler.GetAppLogs)

	// Connector reviews (all authenticated users can review)
	protected.Post("/marketplace/connectors/:id/reviews", marketplaceHandler.AddConnectorReview)

	// =========================================================================
	// 5.5 RBAC MANAGEMENT ENDPOINTS
	// =========================================================================

	// Initialize RBAC services
	rbacUserService := service.NewUserService(database.DB)
	rbacRoleService := service.NewRoleService(database.DB)
	rbacTenantService := service.NewTenantService(database.DB)

	// Initialize RBAC handlers
	rbacUserHandler := handlers.NewRBACUserHandler(rbacUserService, rbacRoleService, rbacTenantService)
	rbacRoleHandler := handlers.NewRBACRoleHandler(rbacRoleService, permissionService, rbacUserService)
	rbacTenantHandler := handlers.NewRBACTenantHandler(rbacTenantService, rbacUserService)

	// User Management Endpoints (admin-only)
	rbacUsers := protected.Group("/rbac/users", adminRole)
	rbacUsers.Get("", rbacUserHandler.ListUsers)
	rbacUsers.Post("", rbacUserHandler.AddUserToTenant)
	rbacUsers.Get("/:user_id", rbacUserHandler.GetUser)
	rbacUsers.Patch("/:user_id/role", rbacUserHandler.ChangeUserRole)
	rbacUsers.Delete("/:user_id", rbacUserHandler.RemoveUserFromTenant)
	rbacUsers.Get("/:user_id/permissions", rbacUserHandler.GetUserPermissions)
	rbacUsers.Get("/stats", rbacUserHandler.GetTenantUserStats)

	// Role Management Endpoints (admin-only)
	rbacRoles := protected.Group("/rbac/roles", adminRole)
	rbacRoles.Get("", rbacRoleHandler.ListRoles)
	rbacRoles.Post("", rbacRoleHandler.CreateRole)
	rbacRoles.Get("/:role_id", rbacRoleHandler.GetRole)
	rbacRoles.Patch("/:role_id", rbacRoleHandler.UpdateRole)
	rbacRoles.Delete("/:role_id", rbacRoleHandler.DeleteRole)
	rbacRoles.Get("/:role_id/permissions", rbacRoleHandler.GetRolePermissions)
	rbacRoles.Post("/:role_id/permissions", rbacRoleHandler.AssignPermissionToRole)
	rbacRoles.Delete("/:role_id/permissions", rbacRoleHandler.RemovePermissionFromRole)

	// Tenant Management Endpoints
	rbacTenants := protected.Group("/rbac/tenants")
	rbacTenants.Get("", rbacTenantHandler.ListTenants)                                // Users see their own tenants
	rbacTenants.Post("", rbacTenantHandler.CreateTenant)                              // Create new tenant
	rbacTenants.Get("/:tenant_id", rbacTenantHandler.GetTenant)                       // Get tenant details
	rbacTenants.Patch("/:tenant_id", adminRole, rbacTenantHandler.UpdateTenant)       // Update (admin only)
	rbacTenants.Delete("/:tenant_id", rbacTenantHandler.DeleteTenant)                 // Delete (owner only)
	rbacTenants.Get("/:tenant_id/users", adminRole, rbacTenantHandler.GetTenantUsers) // List users
	rbacTenants.Get("/:tenant_id/stats", rbacTenantHandler.GetTenantStats)            // Get stats

	// Danger zone (spec §6): hardened organization erasure. Unlike the immediate
	// DELETE /rbac/tenants/:id above, this flow forces a full export first, an
	// exact-name confirmation, an MFA check for enrolled admins, a 30-day cancelable
	// grace window, an admin notification and a logged purge (RGPD / Cameroon law
	// 2024/017). A daily worker purges only after the grace window elapses.
	orgExporter := orgexport.New(database.DB, os.Getenv("EXPORT_DIR"))
	orgDeletionService := orgdeletion.NewService(
		repository.NewGormOrgDeletionRepository(database.DB),
		repository.NewGormOrgDeletionRepository(database.DB), // OrgReader.Name
		orgExporter,
		authmfa.NewGate(mfaRepo, mfaKey[:]),
		orgdeletioninfra.NewPurger(rbacTenantService.DeleteTenant),
	).WithNotifier(orgdeletioninfra.NewAdminNotifier(database.DB, notificationUseCase))
	orgDeletionHandler := handlers.NewOrgDeletionHandler(orgDeletionService, orgExporter)
	protected.Get("/organization/export", adminRole, orgDeletionHandler.ExportOrganization)
	protected.Get("/organization/deletion", adminRole, orgDeletionHandler.GetDeletion)
	protected.Post("/organization/deletion", adminRole, orgDeletionHandler.RequestDeletion)
	protected.Post("/organization/deletion/cancel", adminRole, orgDeletionHandler.CancelDeletion)

	// Purge worker: sweeps every 6h and erases only tenants whose 30-day grace
	// window has elapsed and were not cancelled. Cross-tenant by design.
	go func() {
		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			if n, err := orgDeletionService.RunDuePurges(context.Background()); err != nil {
				log.Printf("org-deletion purge sweep error: %v", err)
			} else if n > 0 {
				log.Printf("org-deletion purge sweep: %d organization(s) erased after grace window", n)
			}
		}
	}()

	// =========================================================================
	// 5.10 ORGANIZATION MEMBER MANAGEMENT (W0-04)
	//
	// Members, invitations, roles, deactivation and the membership audit trail.
	// The tenant for every route below comes from the authenticated session
	// (middleware.GetContext) — no path segment, query parameter or body field
	// names an organization, so there is nothing for a caller to substitute.
	//
	// The two acceptance routes are mounted OUTSIDE this group, further down,
	// because an invitee may not have an account yet.
	// =========================================================================
	membershipSvc := membership.NewService(membershipRepo, userRepo).
		WithOrganizations(orgRepo).
		// The recorder feeds the request collector, so a membership action lands
		// as ONE chained trail entry carrying both its meaning and its
		// before → after — not as a second entry beside the middleware's.
		WithAudit(governance.NewAuditRecorder(auditChainRepo)).
		WithAuditReader(auditChainRepo).
		WithMailer(authmail.NewInvitationMailer(emailTransport)).
		// Withdrawing access ends the member's refresh lineage, so no new session
		// can be minted for them.
		WithSessionRevoker(tokenManager).
		WithPasswordHasher(passwordHasher).
		// OR26-03 — promoting somebody into a privileged role re-anchors their MFA
		// grace window and drops their cached decision, so the new deadline is the
		// one they actually get and it applies on the next request.
		WithMFAPrivilegeRoles(mfaRequiredRoles, mfaRequiredBusinessRoles).
		WithMFAStatusInvalidator(mfaStatusResolver).
		// An invitee joins an organization that already exists, so the signup
		// wizard — whose first screen asks you to set one up — has nothing to
		// ask them. Without this the route guard would hold a newly joined
		// colleague in front of a form about a tenant they cannot edit.
		WithOnboarding(activationRepo).
		WithBaseURL(appBaseURL)
	memberHandler := handlers.NewOrganizationMemberHandler(membershipSvc)

	// Reading the organization and its member roster needs membership-read;
	// admins hold "*" and pass everything. The counts endpoint is open to any
	// authenticated member: knowing how many colleagues you have is not
	// privileged, and gating it would leave the sidebar badge permanently empty
	// for everyone who is not an administrator.
	orgRead := middleware.RequirePermission("organization:read", "organization:members:read")
	protected.Get("/organization", orgRead, memberHandler.GetOrganization)
	// Headline member counts for the org switcher; no member identities.
	// Session-sufficient (#529).
	protected.Get("/organization/counts", memberHandler.GetCounts)

	// STATIC SUB-PATHS BEFORE /:memberId — Fiber matches in registration order,
	// so "audit" would otherwise be parsed as a member id.
	protected.Get("/organization/members/audit",
		middleware.RequirePermission("organization:audit:read"), memberHandler.GetMembershipAudit)
	protected.Get("/organization/members",
		middleware.RequirePermission("organization:members:read"), memberHandler.ListMembers)
	protected.Get("/organization/members/:memberId",
		middleware.RequirePermission("organization:members:read"), memberHandler.GetMember)
	protected.Put("/organization/members/:memberId/role",
		middleware.RequirePermission("organization:members:update"), memberHandler.UpdateMemberRole)
	protected.Put("/organization/members/:memberId/status",
		middleware.RequirePermission("organization:members:deactivate"), memberHandler.UpdateMemberStatus)

	protected.Get("/organization/invitations",
		middleware.RequirePermission("organization:members:read"), memberHandler.ListInvitations)
	protected.Post("/organization/invitations",
		middleware.RequirePermission("organization:members:invite"), memberHandler.CreateInvitation)
	protected.Post("/organization/invitations/:invitationId/resend",
		middleware.RequirePermission("organization:members:invite"), memberHandler.ResendInvitation)
	protected.Delete("/organization/invitations/:invitationId",
		middleware.RequirePermission("organization:members:invite"), memberHandler.RevokeInvitation)

	// The two acceptance routes were mounted before the JWT gate (see the
	// forward declaration above); assign the handler they capture.
	invitationPublicHandler = memberHandler

	// The RBAC catalog: the permission vocabulary and the business-role presets,
	// readable by any authenticated member so the UI can render the permission
	// matrix. It is reference material about the product, not data about anyone's
	// organization.
	//
	// GET/POST /rbac/members and PUT /rbac/members/:userId/business-role were
	// removed here in favour of /organization/members (§5.10). Keeping both would
	// have meant two ways to add a colleague — and the one here created the user
	// immediately and handed the administrator a temporary password to relay,
	// with no token, no expiry, no revocation and no resend.
	businessRoleHandler := handlers.NewBusinessRoleHandler()
	// The static preset catalogue shipped with the product — the same list for
	// every tenant, no data. Session-sufficient (#529).
	protected.Get("/rbac/business-roles", businessRoleHandler.GetCatalog)

	// =========================================================================
	// 5.6 SCANNER ENGINE (cloud SDK scans + on-prem Agent, Redis preview)
	//
	// The pipeline NEVER writes Assets/Risks — every scan lands in a Redis preview
	// (48h TTL) and the user imports/ignores from the Scan Preview page. Cloud
	// scans run in-process (credentials AES-256-GCM, decrypted only at scan time);
	// nmap/osquery only ever run on the on-prem Agent, which pushes results back
	// over an RS256 (scoped "scanner") + HMAC-signed channel.
	// =========================================================================

	// Credential cipher (AES-256-GCM) for cloud creds + per-agent push secrets.
	scannerKeyRaw := os.Getenv("SCANNER_CREDENTIAL_KEY")
	if scannerKeyRaw == "" {
		scannerKeyRaw = "openrisk-dev-scanner-credential-key-change-me"
		log.Println("Warning: SCANNER_CREDENTIAL_KEY not set — using an insecure dev key. Set a strong key in production.")
	}
	scannerCipher, cipherErr := scanapp.NewCredentialCipher([]byte(scannerKeyRaw))
	if cipherErr != nil {
		log.Fatalf("Scanner: credential cipher init failed: %v", cipherErr)
	}

	// Provider registry. Cloud scanners run real SDK collectors (aws-sdk-go-v2,
	// Azure Resource Graph, GCP Compute) — credentials are decrypted only at scan
	// time. Agent/nmap providers validate here but execute on the on-prem Agent.
	scanRegistry := scanpkg.NewRegistry()
	scanRegistry.Register(scanpkg.NewAWSScanner(collectors.NewAWS()))
	scanRegistry.Register(scanpkg.NewAzureScanner(collectors.NewAzure()))
	scanRegistry.Register(scanpkg.NewGCPScanner(collectors.NewGCP()))
	scanRegistry.Register(scanpkg.NewNmapScanner())
	scanRegistry.Register(scanpkg.NewAgentScanner())
	// Auto-discovery API providers (spec "6. Découverte automatique des actifs").
	// Each runs in-process in the SaaS worker via its official SDK/REST collector.
	scanRegistry.Register(scanpkg.NewGitHubScanner(collectors.NewGitHub()))
	scanRegistry.Register(scanpkg.NewGitLabScanner(collectors.NewGitLab()))
	scanRegistry.Register(scanpkg.NewActiveDirectoryScanner(collectors.NewActiveDirectory()))
	scanRegistry.Register(scanpkg.NewM365Scanner(collectors.NewM365()))
	scanRegistry.Register(scanpkg.NewDockerScanner(collectors.NewDocker()))
	scanRegistry.Register(scanpkg.NewVMwareScanner(collectors.NewVMware()))
	scanRegistry.Register(scanpkg.NewKubernetesScanner(collectors.NewKubernetes()))

	scanPreview := scanpkg.NewPreviewStore(redisClientInstance)
	// In-app + e-mail sink: a completed scan raises a durable in-app notification
	// for the user who triggered it and (best-effort) e-mails them. Failures never
	// block the scan. A Nil user (e.g. a failed cloud scan) is skipped.
	scanInApp := func(ctx context.Context, tenantID, userID uuid.UUID, title, message string) {
		if userID == uuid.Nil {
			return
		}
		if err := notificationUseCase.NotifyInApp(userID, tenantID, domain.NotificationTypeScanComplete, title, message, nil, "scan"); err != nil && !errors.Is(err, notificationapp.ErrSuppressed) {
			zeroLogger.Warn().Err(err).Msg("scanner: could not create in-app notification")
		}
		// The recipient's stored e-mail preference governs the e-mail half too.
		// Checking it only for the in-app record would leave the Settings screen
		// governing half a delivery (W0-05 / D2).
		if notificationUseCase.ShouldNotify(userID, tenantID, domain.NotificationTypeScanComplete, domain.NotificationChannelEmail) {
			if user, err := userRepo.GetByID(ctx, userID); err == nil && user != nil && user.Email != "" {
				_ = emailTransport.SendEmail(ctx, user.Email, title, message)
			}
		}
	}
	scanNotifier := scanpkg.NewRedisNotifier(redisClientInstance, scanInApp)

	// Risk review cadence: a background worker nudges each risk's owner (in-app +
	// e-mail) when a review is due, keeping the register "updated regularly".
	riskReviewRepo := repository.NewGormRiskReviewRepository(database.DB)
	riskReviewWorker := workers.NewRiskReviewWorker(riskReviewRepo, func(ctx context.Context, tenantID, ownerID, riskID uuid.UUID, riskTitle string) {
		subject := "Revue de risque requise"
		message := "Le risque « " + riskTitle + " » est dû pour revue."
		if err := notificationUseCase.NotifyInApp(ownerID, tenantID, domain.NotificationTypeRiskReview, subject, message, &riskID, "risk"); err != nil && !errors.Is(err, notificationapp.ErrSuppressed) {
			zeroLogger.Warn().Err(err).Msg("risk review: in-app notification failed")
		}
		if notificationUseCase.ShouldNotify(ownerID, tenantID, domain.NotificationTypeRiskReview, domain.NotificationChannelEmail) {
			if user, uerr := userRepo.GetByID(ctx, ownerID); uerr == nil && user != nil && user.Email != "" {
				_ = emailTransport.SendEmail(ctx, user.Email, subject, message)
			}
		}
	}, zeroLogger)
	go riskReviewWorker.Start(context.Background())

	// Mitigation deadlines: D-7 and D-1 nudges to whoever is doing the work.
	// Hourly sweep — day-granularity reminders do not need a per-minute tick, and
	// the "past the threshold and not yet sent" rule means a missed tick (deploy,
	// restart) still sends the nudge instead of silently skipping it.
	mitigationDueWorker := workers.NewMitigationDueWorker(
		repository.NewGormMitigationDueRepository(database.DB),
		func(ctx context.Context, tenantID, userID, mitigationID uuid.UUID, subject, message string) {
			if err := notificationUseCase.NotifyInApp(userID, tenantID,
				domain.NotificationTypeMitigationDeadline, subject, message, &mitigationID, "mitigation"); err != nil && !errors.Is(err, notificationapp.ErrSuppressed) {
				zeroLogger.Warn().Err(err).Msg("mitigation due: in-app notification failed")
			}
			if notificationUseCase.ShouldNotify(userID, tenantID, domain.NotificationTypeMitigationDeadline, domain.NotificationChannelEmail) {
				if user, uerr := userRepo.GetByID(ctx, userID); uerr == nil && user != nil && user.Email != "" {
					_ = emailTransport.SendEmail(ctx, user.Email, subject, message)
				}
			}
		}, zeroLogger)
	go mitigationDueWorker.Start(context.Background())

	// Evidence expiry: warn the owner before proof goes stale. Without this, the
	// first person to notice a lapsed certificate is the auditor reading the
	// register — the register is still honest (the control shows as unevidenced),
	// but the organisation finds out at the worst possible moment.
	evidenceExpiryWorker := workers.NewEvidenceExpiryWorker(
		evidenceRepo,
		func(ctx context.Context, tenantID, userID, evidenceID uuid.UUID, subject, message string) {
			if err := notificationUseCase.NotifyInApp(userID, tenantID,
				domain.NotificationTypeRiskReview, subject, message, &evidenceID, "evidence"); err != nil && !errors.Is(err, notificationapp.ErrSuppressed) {
				zeroLogger.Warn().Err(err).Msg("evidence expiry: in-app notification failed")
			}
			if notificationUseCase.ShouldNotify(userID, tenantID, domain.NotificationTypeRiskReview, domain.NotificationChannelEmail) {
				if user, uerr := userRepo.GetByID(ctx, userID); uerr == nil && user != nil && user.Email != "" {
					_ = emailTransport.SendEmail(ctx, user.Email, subject, message)
				}
			}
		}, zeroLogger)
	go evidenceExpiryWorker.Start(context.Background())

	scanPipeline := scanpkg.NewPipeline(scanRegistry, scanPreview, scanNotifier, zeroLogger)

	// Remediation auto-detection: after a scan, a finding (CVE) that is no longer
	// detected auto-completes the matching mitigation sub-action on the linked risk
	// (CompletedSource=scanner, CompletedBy=nil, AutoDetectedAt=now) and publishes
	// mitigation.auto_completed for the SSE stream. Manual complete/revert stay available.
	ctiSubActionRepo := repository.NewGormMitigationSubActionRepository(database.DB)
	ctiMitigationRepo := repository.NewGormMitigationRepository(database.DB)
	autoCompleteUC := appmitigation.NewAutoCompleteSubActionUseCase(ctiSubActionRepo, ctiMitigationRepo)
	mitigationDetector := scanmitigation.NewDetector(database.DB, autoCompleteUC, ctiSubActionRepo, redisClientInstance, zeroLogger)
	scanPipeline = scanPipeline.WithMitigationDetector(mitigationDetector)

	// Assign the forward-declared SSE handler (route registered earlier on `app`,
	// before the /api/v1 JWT middleware).
	mitigationEventsHandler = handlers.NewMitigationEventsHandler(redisClientInstance, rsaKeys, jtiBlacklistChecker)
	scanLock := scanpkg.NewScanLock(redisClientInstance)

	scanConfigRepo := repository.NewGormScanConfigRepository(database.DB)
	scanAgentRepo := repository.NewGormScannerAgentRepository(database.DB)
	scanJobRepo := repository.NewGormScanJobRepository(database.DB)

	createScanConfigUC := scanapp.NewCreateScanConfigUseCase(scanConfigRepo, scanRegistry, scannerCipher)
	listScanConfigsUC := scanapp.NewListScanConfigsUseCase(scanConfigRepo)
	getScanConfigUC := scanapp.NewGetScanConfigUseCase(scanConfigRepo)
	deleteScanConfigUC := scanapp.NewDeleteScanConfigUseCase(scanConfigRepo)
	triggerScanUC := scanapp.NewTriggerScanUseCase(
		scanConfigRepo, scanJobRepo, scanLock, scanRegistry, scanPipeline, scannerCipher, redisClientInstance, zeroLogger,
	)
	// Recurring scans: a background scheduler triggers due configs every minute.
	scanScheduler := workers.NewScanScheduler(scanConfigRepo, triggerScanUC, zeroLogger)
	go scanScheduler.Start(context.Background())
	listAgentsUC := scanapp.NewListAgentsUseCase(scanAgentRepo)
	revokeAgentUC := scanapp.NewRevokeAgentUseCase(scanAgentRepo, redisClientInstance)
	registerAgentUC := scanapp.NewRegisterAgentUseCase(scanAgentRepo, rsaKeys, scannerCipher)
	pushResultsUC := scanapp.NewPushResultsUseCase(scanAgentRepo, scanJobRepo, scanLock, scanPipeline)
	heartbeatAgentUC := scanapp.NewHeartbeatAgentUseCase(scanAgentRepo)
	listScanJobsUC := scanapp.NewListScanJobsUseCase(scanJobRepo)
	getScanPreviewUC := scanapp.NewGetScanPreviewUseCase(scanPreview)
	importPreviewUC := scanapp.NewImportPreviewUseCase(scanPreview, assetRepo)
	ignorePreviewUC := scanapp.NewIgnorePreviewUseCase(scanPreview)

	scannerHandler = handlers.NewScannerHandler(
		createScanConfigUC, listScanConfigsUC, getScanConfigUC, deleteScanConfigUC, triggerScanUC,
		listAgentsUC, revokeAgentUC, registerAgentUC, pushResultsUC, heartbeatAgentUC,
		listScanJobsUC, getScanPreviewUC, importPreviewUC, ignorePreviewUC,
		scanAgentRepo, scanJobRepo, scannerCipher, rsaKeys, jtiBlacklistChecker, redisClientInstance,
	)

	// User-facing routes (RS256 user token). Admin/root pass via the "*" wildcard.
	scannerRead := middleware.RequirePermission("scanner:read")
	scannerCreate := middleware.RequirePermission("scanner:create")
	scannerDelete := middleware.RequirePermission("scanner:delete")
	scannerScan := middleware.RequirePermission("scanner:scan")
	scannerImport := middleware.RequirePermission("scanner:import")

	protected.Get("/scanner/configs", scannerRead, scannerHandler.ListScanConfigs)
	protected.Post("/scanner/configs", scannerCreate, scannerHandler.CreateScanConfig)
	protected.Delete("/scanner/configs/:id", scannerDelete, scannerHandler.DeleteScanConfig)
	protected.Post("/scanner/configs/:id/scan", scannerScan, scannerHandler.TriggerScan)
	protected.Post("/scanner/configs/:id/registration-token", scannerCreate, scannerHandler.IssueRegistrationToken)
	protected.Get("/scanner/agents", scannerRead, scannerHandler.ListAgents)
	protected.Delete("/scanner/agents/:id", scannerDelete, scannerHandler.RevokeAgent)
	protected.Get("/scanner/jobs", scannerRead, scannerHandler.ListScanJobs)
	protected.Get("/scanner/jobs/:id/preview", scannerRead, scannerHandler.GetScanPreview)
	protected.Post("/scanner/jobs/:id/import", scannerImport, scannerHandler.ImportPreview)
	protected.Post("/scanner/jobs/:id/ignore", scannerImport, scannerHandler.IgnorePreview)
	protected.Get("/scanner/events", scannerRead, scannerHandler.StreamScanEvents)

	// -------------------------------------------------------------------------
	// Realtime event hub (W0-07) — the one stream every live surface reads.
	//
	// Mounted on `protected`, so the browser authenticates with its HttpOnly
	// session cookie and any other caller with a bearer token. There is
	// deliberately no ?token= escape hatch: a credential in a URL ends up in
	// access logs, proxy logs and browser history, and cookie sessions (W0-03)
	// make it unnecessary.
	//
	// `events:read` says an identity may HOLD a stream. What travels on it is
	// decided per event by the read permission of its own aggregate, inside the
	// handler — so the stream can never become a way around the permission
	// model.
	realtimeHandler := handlers.NewRealtimeHandler(realtimeHub, realtimeEventLog)
	realtimeRead := middleware.RequirePermission("events:read")
	protected.Get("/realtime/catalog", realtimeRead, realtimeHandler.Catalog)
	protected.Get("/realtime/stats", middleware.RequireRole("admin", "root"), realtimeHandler.Stats)
	protected.Get("/realtime/events", realtimeRead, realtimeHandler.Stream)
	log.Println("Realtime: GET /realtime/events mounted (SSE, session-authenticated, tenant-scoped)")

	// The agent-facing routes (register/stream/push) were mounted earlier on `app`
	// (before the /api/v1 user middleware) — see the note above `var scannerHandler`.
	log.Println("Scanner: engine wired (cloud validate + agent register/stream/push, Redis preview)")

	// =========================================================================
	// 5.7 CTI / INTEL THREAT ENGINE (NVD + CISA KEV + MITRE ATT&CK)
	// =========================================================================
	// Feeds are ingested into the global cti_vulnerabilities table, enriched with
	// embedded MITRE ATT&CK data, then matched against each tenant's asset CPEs to
	// auto-create risks (Source=cti_auto). NVD hourly, CISA KEV every 6h.
	ctiRepo := repository.NewGormCTIRepository(database.DB)
	ctiClient := cti.NewExternalClient(nil, os.Getenv("NVD_API_KEY"))
	ctiService := cti.NewService(ctiRepo, ctiClient)
	ctiRiskCreator := ctimatch.NewAutoRiskCreator(database.DB)
	ctiMatcher := ctimatch.NewTenantSweepMatcher(database.DB, ctiRepo, ctiRiskCreator)
	ctiSyncWorker := cti.NewSyncWorker(ctiRepo, ctiClient, ctiMatcher, zeroLogger)
	ctiHandler := handlers.NewCTIHandler(ctiService, ctiSyncWorker, ctiMatcher, database.DB)

	ctiRead := middleware.RequirePermission("risks:read")
	ctiAdmin := middleware.RequireRole("admin", "root")
	protected.Get("/cti/vulnerabilities", ctiRead, ctiHandler.List)
	protected.Get("/cti/vulnerabilities/:cve", ctiRead, ctiHandler.Get)
	protected.Get("/cti/stats", ctiRead, ctiHandler.Stats)
	protected.Post("/cti/sync", ctiAdmin, ctiHandler.Sync)
	protected.Post("/cti/match", ctiAdmin, ctiHandler.Match)

	// =========================================================================
	// Universal Search (UX-1) — one GET /search behind the ⌘K palette. Composes
	// the existing tenant-scoped repos through nil-safe ports; each source is
	// searched only if the caller holds its read permission, so results respect
	// RBAC exactly like the sidebar. Best-effort: always 200. Placed after the CTI
	// service is built so CVE search can compose it.
	// =========================================================================
	searchHandler := handlers.NewSearchHandler(
		searchapp.New().
			WithRisks(riskRepo).
			WithAssets(assetRepo).
			WithVulns(vulnRepo).
			WithControls(complianceRepo).
			WithAudits(complianceAuditRepo).
			WithReports(boardRepo).
			WithCVE(ctiService).
			// The member source is the membership repository behind an adapter:
			// search wants "every member of this tenant" while the repository's
			// listing is paged and filtered. Adapting here keeps ONE member store
			// rather than a second one kept alive for this call site.
			WithMembers(memberDirectory{repo: membershipRepo}),
	)
	// Results are filtered per type by the caller's own permissions (the `can`
	// closure passed into the use case), so the guard is inside the handler and
	// a bare permission on the route would be coarser, not safer (#529).
	protected.Get("/search", searchHandler.Search)

	// =========================================================================
	// SCORE — the single authority for every number this product calls a "score"
	//
	// The dashboard hero, the sidebar footer, the dedicated page, the register
	// rows and the asset drawer all read THIS endpoint. They cannot disagree,
	// because there is no second source to disagree with — and the band always
	// travels with the value, computed from it server-side, so a label can never
	// drift from the number it describes.
	//
	// The calculation itself lives in internal/domain/scoring and nowhere else:
	// no formula, no threshold and no band mapping exists in the frontend.
	// =========================================================================
	scoreHandler := newScoreHandler(
		riskRepo, assetRepo, vulnRepo,
		repository.NewGormMitigationRepository(database.DB),
		getGapAnalysisUC, incidentService,
	)
	// Readable by any authenticated member: a posture score is what the product
	// is FOR, and the underlying detail is already gated per source.
	// Tenant-scoped score figures; every business role preset holds risks:read,
	// and preview persists nothing. Session-sufficient (#529).
	protected.Get("/score", scoreHandler.GetScore)
	protected.Get("/score/model", scoreHandler.GetScoreModel)
	// Live preview for forms (debounced client-side). Persists nothing, so it
	// carries no write permission.
	protected.Post("/score/preview", scoreHandler.PreviewScore)

	// =========================================================================
	// ACTIVATION & ONBOARDING — the newcomer journey (signup → Aha in < 8 min)
	//
	// Activation state is DERIVED FROM SERVER EVENTS, never from client state:
	// there is deliberately no endpoint that lets the client declare a step done.
	// The only writes are (a) acknowledging that a celebration was shown, and
	// (b) the wizard's own answers.
	// =========================================================================
	onboardingUC := appactivation.NewOnboardingUseCase(activationRepo, activationRecorder).
		WithOrgUpdater(orgRepo).
		WithOrgCurrencyUpdater(orgRepo).
		WithProfileUpdater(userRepo)
	activationHandler := handlers.NewActivationHandler(
		appactivation.NewGetStateUseCase(activationRepo),
		appactivation.NewMarkCelebratedUseCase(activationRepo),
		onboardingUC,
	)
	// Readable by any authenticated member: the get-started panel is not a
	// privileged view, and gating it behind a permission would hide the product's
	// own instructions from exactly the people who need them most.
	// Activation and onboarding are per-user state: every handler resolves
	// (tenant, user) from the session and takes no id from the request.
	// Session-sufficient (#529).
	protected.Get("/activation/state", activationHandler.GetActivationState)
	protected.Post("/activation/celebrated", activationHandler.MarkCelebrated)

	// Wizard. Static sub-paths before any ":param" route (the Fiber trap this
	// codebase has hit repeatedly).
	protected.Get("/onboarding/state", activationHandler.GetOnboardingState)
	protected.Get("/onboarding/suggestions", activationHandler.GetOnboardingSuggestions)
	protected.Post("/onboarding/complete", activationHandler.CompleteOnboarding)
	protected.Put("/onboarding/steps/:step", activationHandler.SaveOnboardingStep)

	// The periodic sync worker (NVD 1h / CISA 6h + post-sync matching) runs in
	// production. In dev it stays off by default to avoid hitting the feeds on every
	// restart — the manual POST /cti/sync + /cti/match endpoints are always live.
	if os.Getenv("CTI_SYNC_ENABLED") == "true" {
		go ctiSyncWorker.Start(context.Background())
		log.Println("CTI: sync worker started (NVD hourly, CISA KEV 6h, post-sync matching)")
	} else {
		log.Println("CTI: engine wired (periodic sync disabled — set CTI_SYNC_ENABLED=true; manual /cti/sync + /cti/match live)")
	}

	// Vulnerability live-pull scheduler — polls due integrations (schedule_minutes)
	// on the same pipeline as the manual "Pull now" button. Off by default in dev;
	// enable with VULN_LIVEPULL_ENABLED=true. Manual POST /vulnerabilities/
	// integrations/:id/pull is always live.
	if os.Getenv("VULN_LIVEPULL_ENABLED") == "true" {
		vulnPullScheduler := vulnapp.NewLivePullScheduler(vulnIntegRepo, vulnLivePullUC, time.Minute)
		go vulnPullScheduler.Run(context.Background())
		log.Println("Vuln: live-pull scheduler started (due integrations polled every minute)")
	} else {
		log.Println("Vuln: live-pull scheduler wired (disabled — set VULN_LIVEPULL_ENABLED=true; manual pull live)")
	}

	// =========================================================================
	// 5.9 SECURITY AUTOMATION / SOAR ENGINE (spec §10 « Automatisation »)
	// =========================================================================
	// Rules bind platform triggers (a newly detected vulnerability, a risk score
	// change) to ordered action chains (scan → create risk → assign → ticket →
	// notify → start SLA). The AutomationWorker consumes Redis events into the
	// engine; the SLAMonitor escalates overdue remediations and auto-closes
	// resolved ones. Action ports reuse the existing capabilities of the platform.
	automationRuleRepo := repository.NewGormAutomationRuleRepository(database.DB)
	automationExecRepo := repository.NewGormAutomationExecutionRepository(database.DB)
	slaTrackerRepo := repository.NewGormSLATrackerRepository(database.DB)
	automationChannelRepo := repository.NewGormAutomationChannelRepository(database.DB)

	// Multi-channel dispatcher (in-app + email + Slack + Teams) with owner/role
	// recipient resolution, and the concrete risk/ticket/scan action adapters.
	automationNotifier := autoinfra.NewNotifier(automationChannelRepo, notificationUseCase, emailTransport, userRepo, database.DB, zeroLogger)
	automationRiskActions := autoinfra.NewRiskActions(riskRepo, userRepo, database.DB, zeroLogger)
	automationTicketer := autoinfra.NewTicketer(vulnIntegRepo, vulnIntegCipher)
	automationScanAction := autoinfra.NewScanAction(scanConfigRepo, triggerScanUC, zeroLogger)

	// The subject resolver is what makes a dry run a test against REAL tenant
	// data: it loads a live vulnerability / risk / incident (read-only) and
	// rebuilds the trigger context an event would have produced. It also supplies
	// the asset tag vocabulary that a rule's asset-tag condition gates on — for
	// live events as well as dry runs.
	automationSubjects := autoinfra.NewSubjectResolver(database.DB)
	automationChannelService := appauto.NewChannelService(automationChannelRepo)

	automationEngine := appauto.NewEngine(automationRuleRepo, automationExecRepo, slaTrackerRepo, zeroLogger).
		WithNotifier(automationNotifier).
		WithTicketer(automationTicketer).
		WithRiskCreator(automationRiskActions).
		WithRiskAssigner(automationRiskActions).
		WithRiskResolver(automationRiskActions).
		WithAssetScanner(automationScanAction).
		WithSubjectResolver(automationSubjects).
		WithAssetFacts(automationSubjects).
		WithChannelProbe(automationChannelService)

	// Incidents reach their stakeholders through the SAME dispatcher automation
	// uses: one Slack webhook, one mail transport, one set of channel settings.
	// A second dispatcher would mean a tenant configuring Slack twice and
	// wondering why only half their alerts arrive.
	incidentService.WithNotifier(incinfra.NewNotifier(automationNotifier, zeroLogger))

	automationSLAService := appauto.NewSLAService(slaTrackerRepo, zeroLogger).
		WithNotifier(automationNotifier).
		WithRiskLookup(automationRiskActions)

	automationHandler := handlers.NewAutomationHandler(
		appauto.NewRuleService(automationRuleRepo),
		appauto.NewExecutionService(automationExecRepo),
		automationSLAService,
		automationChannelService,
		automationEngine,
	).WithUserEmails(userRepo).
		WithChannelTester(
			appauto.NewChannelTester(automationChannelRepo).
				WithInApp(notificationUseCase).
				WithEmail(emailTransport).
				WithUserEmail(func(ctx context.Context, userID uuid.UUID) string {
					emails, err := userRepo.EmailsByIDs(ctx, []uuid.UUID{userID})
					if err != nil {
						return ""
					}
					return emails[userID]
				}),
		)

	// A newly detected vulnerability fires the engine's vulnerability_detected
	// trigger. Mutating vulnIngestUC here still affects the vuln handlers/webhook
	// (they hold the same pointer — same pattern as WithTicketOpener above).
	vulnIngestUC.WithEventPublisher(autoinfra.NewVulnEventPublisher(redisClientInstance))

	automationRead := middleware.RequirePermission("automation:read")
	automationWrite := middleware.RequirePermission("automation:write")
	automationAdmin := middleware.RequireRole("admin", "root")
	protected.Get("/automation/rules", automationRead, automationHandler.ListRules)
	protected.Post("/automation/rules", automationWrite, automationHandler.CreateRule)
	// Static sub-paths before /:id so "executions"/"sla"/"channels" never parse as UUID.
	protected.Get("/automation/executions", automationRead, automationHandler.ListExecutions)
	protected.Get("/automation/sla", automationRead, automationHandler.ListSLA)
	protected.Get("/automation/sla/stats", automationRead, automationHandler.SLAStats)
	protected.Get("/automation/channels", automationRead, automationHandler.GetChannels)
	protected.Get("/automation/channels/catalogue", automationRead, automationHandler.ListChannelCatalogue)
	protected.Put("/automation/channels", automationAdmin, automationHandler.SaveChannels)
	protected.Post("/automation/channels/test", automationWrite, automationHandler.TestChannel)
	protected.Get("/automation/state", automationRead, automationHandler.AutomationState)
	protected.Get("/automation/templates", automationRead, automationHandler.ListTemplates)
	protected.Post("/automation/templates/:key/adopt", automationWrite, automationHandler.CreateRuleFromTemplate)
	protected.Get("/automation/dry-runs/:id", automationRead, automationHandler.GetDryRun)
	protected.Post("/automation/dry-runs/:id/cancel", automationRead, automationHandler.CancelDryRun)
	protected.Get("/automation/rules/:id", automationRead, automationHandler.GetRule)
	protected.Put("/automation/rules/:id", automationWrite, automationHandler.UpdateRule)
	protected.Delete("/automation/rules/:id", automationWrite, automationHandler.DeleteRule)
	// Dry run is a READ of what would happen — it touches no action port, so it
	// only needs read permission. Running a rule for real is a write.
	protected.Post("/automation/rules/:id/dry-run", automationRead, automationHandler.DryRunRule)
	protected.Post("/automation/rules/:id/run", automationWrite, automationHandler.RunRuleNow)
	protected.Post("/automation/rules/:id/enable", automationWrite, automationHandler.EnableRule)
	protected.Post("/automation/rules/:id/suspend", automationWrite, automationHandler.SuspendRule)
	protected.Get("/automation/rules/:id/executions", automationRead, automationHandler.ListRuleExecutions)
	protected.Post("/automation/executions/:id/replay", automationWrite, automationHandler.ReplayExecution)

	// Background workers: the SOAR engine (event-driven) and the SLA monitor (cadence).
	automationWorker := workers.NewAutomationWorker(redisClientInstance, automationEngine, zeroLogger)
	go automationWorker.Start(context.Background())
	slaMonitor := workers.NewSLAMonitor(automationSLAService, zeroLogger)
	go slaMonitor.Start(context.Background())
	log.Println("Automation: SOAR engine + SLA monitor started (triggers: vulnerability.detected, risk.score_updated)")

	// =========================================================================
	// 5.10 GOVERNANCE (spec §15 « Gouvernance »)
	// =========================================================================
	// One GORM store backs the four governance aggregates. Audit reads/writes and
	// email resolution reuse the existing tenant-scoped user repo. Writes are
	// guarded by role (audit trail + workflow config = admin; delegations +
	// approval decisions = any authenticated member — the use cases enforce
	// four-eyes and role-eligibility internally).
	// auditChainRepo (built above, before the GORM plugin) is the append-only,
	// hash-chained store: it assigns each entry its sequence number and the hash
	// of its predecessor, which is what makes tampering detectable.
	auditRetentionRepo := repository.NewGormAuditRetentionRepository(database.DB)
	delegationRepo := repository.NewGormDelegationRepository(database.DB)
	approvalRepo := repository.NewGormApprovalRepository(database.DB)
	governanceRecorder := governance.NewAuditRecorder(auditChainRepo)

	// An approval inbox nobody is told to open is a queue, not a workflow: the
	// notifier alerts whoever can sign right now (named approvers, role holders,
	// and anyone currently covering for one of them by delegation), and tells the
	// requester the outcome — with the refusal reason, which is the only part
	// that lets them act on it.
	approvalNotifier := govinfra.NewApprovalNotifier(database.DB, notificationUseCase, emailTransport, zeroLogger)
	approvalRoles := govinfra.NewRoleResolver(database.DB)

	governanceHandler := handlers.NewGovernanceHandler(handlers.GovernanceDeps{
		ListAudit: governance.NewListAuditEventsUseCase(auditChainRepo).WithUserLookup(userRepo),
		Recorder:  governanceRecorder,

		VerifyChain:  governance.NewVerifyAuditChainUseCase(auditChainRepo),
		ExportAudit:  governance.NewExportAuditTrailUseCase(auditChainRepo).WithUserLookup(userRepo),
		GetRetention: governance.NewGetRetentionPolicyUseCase(auditRetentionRepo),
		SetRetention: governance.NewSetRetentionPolicyUseCase(auditRetentionRepo).WithRecorder(governanceRecorder),
		PruneAudit:   governance.NewPruneAuditTrailUseCase(auditChainRepo, auditRetentionRepo),

		CreateDelegation: governance.NewCreateDelegationUseCase(delegationRepo).WithRecorder(governanceRecorder).WithUserLookup(userRepo),
		ListDelegations:  governance.NewListDelegationsUseCase(delegationRepo).WithUserLookup(userRepo),
		RevokeDelegation: governance.NewRevokeDelegationUseCase(delegationRepo).WithRecorder(governanceRecorder),
		EffectivePerms:   governance.NewResolveEffectivePermissionsUseCase(delegationRepo),

		CreateWorkflow: governance.NewCreateWorkflowUseCase(approvalRepo),
		ListWorkflows:  governance.NewListWorkflowsUseCase(approvalRepo),
		UpdateWorkflow: governance.NewUpdateWorkflowUseCase(approvalRepo),
		DeleteWorkflow: governance.NewDeleteWorkflowUseCase(approvalRepo),

		SubmitApproval: governance.NewSubmitApprovalRequestUseCase(approvalRepo, approvalRepo).
			WithRecorder(governanceRecorder).
			WithNotifier(approvalNotifier),
		DecideApproval: governance.NewDecideApprovalUseCase(approvalRepo).
			WithRecorder(governanceRecorder).
			WithNotifier(approvalNotifier).
			WithDelegations(delegationRepo, approvalRoles),
		ApprovalDetail: governance.NewGetApprovalDetailUseCase(approvalRepo).
			WithDelegations(delegationRepo, approvalRoles).
			WithUserLookup(userRepo),
		CancelApproval: governance.NewCancelApprovalRequestUseCase(approvalRepo),
		ListApprovals:  governance.NewListApprovalRequestsUseCase(approvalRepo).WithUserLookup(userRepo),
		GetRequest:     approvalRepo,
	})

	governanceAdmin := middleware.RequireRole("admin", "root")

	// Audit trail — admin only (matches /audit-logs). Static sub-paths first
	// (Fiber trap: /:id would otherwise swallow "export" and "verify").
	protected.Get("/governance/audit-events/export", governanceAdmin, governanceHandler.ExportAuditEvents)
	protected.Get("/governance/audit-events/verify", governanceAdmin, governanceHandler.VerifyAuditChain)
	protected.Get("/governance/audit-events", governanceAdmin, governanceHandler.ListAuditEvents)
	protected.Get("/governance/audit-retention", governanceAdmin, governanceHandler.GetAuditRetention)
	protected.Put("/governance/audit-retention", governanceAdmin, governanceHandler.SetAuditRetention)
	protected.Post("/governance/audit-retention/apply", governanceAdmin, governanceHandler.RunAuditRetention)

	// Nightly-ish retention sweep. No-op for every tenant that kept the default
	// (keep forever) — nothing is deleted unless someone configured a window.
	go workers.NewAuditRetentionWorker(
		governance.NewPruneAuditTrailUseCase(auditChainRepo, auditRetentionRepo), zeroLogger,
	).Start(context.Background())

	// Delegations — any authenticated member manages their OWN, so no route
	// guard; the authority checks live in the use cases and are the reason these
	// stay unguarded rather than an omission (#529): CreateDelegation refuses a
	// delegator other than the actor unless the actor is an org admin, and
	// RevokeDelegation admits only the delegator, the delegate or an admin.
	protected.Get("/governance/delegations/effective", governanceHandler.EffectiveDelegatedPermissions)
	protected.Get("/governance/delegations", governanceHandler.ListDelegations)
	protected.Post("/governance/delegations", governanceHandler.CreateDelegation)
	protected.Post("/governance/delegations/:id/revoke", governanceHandler.RevokeDelegation)

	// Approval workflows (config) — admin only.
	protected.Get("/governance/request-types", governanceHandler.ListRequestTypes)
	protected.Get("/governance/workflows", governanceHandler.ListWorkflows)
	protected.Post("/governance/workflows", governanceAdmin, governanceHandler.CreateWorkflow)
	protected.Put("/governance/workflows/:id", governanceAdmin, governanceHandler.UpdateWorkflow)
	protected.Delete("/governance/workflows/:id", governanceAdmin, governanceHandler.DeleteWorkflow)

	// Approval requests (the Maker-Checker inbox) — any authenticated member,
	// because who may act is a property of the REQUEST, not of the route:
	// domain.CanSign checks the caller against the workflow step, and cancel is
	// refused to anyone but the requester. Handler-enforced (#529).
	protected.Get("/governance/approvals", governanceHandler.ListApprovals)
	protected.Post("/governance/approvals", governanceHandler.SubmitApproval)
	protected.Get("/governance/approvals/:id", governanceHandler.GetApproval)
	protected.Post("/governance/approvals/:id/decide", governanceHandler.DecideApproval)
	protected.Post("/governance/approvals/:id/cancel", governanceHandler.CancelApproval)
	// Requests nobody decided in time close as EXPIRED — a distinct outcome from
	// a refusal, because "nobody said no" and "someone said no" call for
	// different next steps. Without this sweep, "expires in 48h" is a label.
	go workers.NewApprovalExpiryWorker(
		governance.NewExpireApprovalsUseCase(approvalRepo, approvalRepo).
			WithNotifier(approvalNotifier).
			WithRecorder(governanceRecorder),
		zeroLogger,
	).Start(context.Background())

	log.Println("Governance: audit trail + delegations + approval workflows mounted (/governance/*)")

	// =========================================================================
	// 6. GRACEFUL SHUTDOWN (Kubernetes Ready)
	// =========================================================================

	// Channel pour écouter les signaux OS (Ctrl+C, Docker Stop, K8s Terminate)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		port := os.Getenv("PORT")
		if port == "" {
			port = "8080"
		}
		log.Printf("⚡ OpenRisk API listening on port %s", port)
		if err := app.Listen(":" + port); err != nil {
			log.Panic(err)
		}
	}()

	<-quit // Bloque jusqu'à réception du signal
	log.Println("Shutting down server...")

	// Timeout de 5 secondes pour finir les requêtes en cours
	if err := app.Shutdown(); err != nil {
		log.Fatal("Forced shutdown:", err)
	}

	log.Println("Server exited properly")
}

// buildEmailTransport picks the mail transport from the environment. There is
// deliberately no default that pretends to send: a deployment either has SMTP,
// or asks for the logging transport, or gets a transport that says so.
func buildEmailTransport() email.Service {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("EMAIL_TRANSPORT"))) {
	case "log":
		log.Println("Email: log transport (messages are logged, not delivered)")
		return email.NewLogService()
	}
	host := strings.TrimSpace(os.Getenv("SMTP_HOST"))
	if host == "" {
		log.Println("Email: no SMTP_HOST configured — outbound mail is unavailable and will be reported as such")
		return email.NewUnconfigured()
	}
	port, err := strconv.Atoi(strings.TrimSpace(os.Getenv("SMTP_PORT")))
	if err != nil || port <= 0 {
		port = 587
	}
	from := strings.TrimSpace(os.Getenv("EMAIL_FROM"))
	if from == "" {
		from = "noreply@openrisk.local"
	}
	log.Printf("Email: SMTP transport %s:%d", host, port)
	return email.FromEnv(host, port, os.Getenv("SMTP_USERNAME"), os.Getenv("SMTP_PASSWORD"), from)
}

// memberDirectory adapts the tenant-scoped membership repository to the narrow
// "who is in this tenant" shape that the ownership picker and the global search
// expect: both want the roster, while the repository's listing is paged and
// filtered. Adapting here keeps ONE membership store rather than a second one
// kept alive for these two call sites. Both consumers cap their own output, so
// the page size below is the repository ceiling, not an unbounded read.
type memberDirectory struct{ repo domain.MembershipRepository }

func (m memberDirectory) ListMembers(ctx context.Context, tenantID uuid.UUID) ([]domain.OrganizationMember, error) {
	rows, _, err := m.repo.ListMembers(ctx, tenantID, domain.MemberQuery{Limit: 200})
	return rows, err
}

func (m memberDirectory) GetMember(ctx context.Context, tenantID, userID uuid.UUID) (*domain.OrganizationMember, error) {
	return m.repo.GetMember(ctx, tenantID, userID)
}

// splitCSVEnv parses a comma-separated environment override into a trimmed,
// blank-free list. An explicitly empty value yields an empty list, which is how
// a deployment says "nobody" rather than "the default" (OR26-03).
func splitCSVEnv(v string) []string {
	out := []string{}
	for _, part := range strings.Split(v, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
