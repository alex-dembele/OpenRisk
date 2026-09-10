// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Activation — the server-side source of truth for "how far into the product is
// this tenant?".
//
// ROOT CAUSE this model fixes: activation state used to live on the client
// (localStorage + counts derived in the checklist component). That produced the
// whole bug family — a checklist that never ticked (the flag was written on one
// device and read on another), confetti firing at random (a re-render re-ran the
// "is it done?" heuristic), and TWO items striking through after a single import
// (two client-side steps were derived from the same underlying count).
//
// The fix is structural, not cosmetic:
//
//   - Activation is an APPEND-ONLY event log written by the domain use cases
//     (ActivationEvent). Nothing derives state from a count any more.
//   - A step is bound to EXACTLY ONE event key (ActivationStepDef.EventKey, and
//     ValidateActivationSteps enforces the bijection). One import can therefore
//     never tick two steps.
//   - Celebration is a server fact too (ActivationCelebration), so a burst fires
//     once per step per user no matter how many times the panel re-renders or
//     how many devices the user has.
// ---------------------------------------------------------------------------

// ActivationEventKey is the closed vocabulary of activation events. Anything not
// in this list is not an activation signal — use the audit trail instead.
type ActivationEventKey string

const (
	// ActivationSignup anchors t0 for the time-to-Aha metric. Recorded once, at
	// registration, before the user has seen a single screen.
	ActivationSignup ActivationEventKey = "signup"

	ActivationProfileCompleted  ActivationEventKey = "profile.completed"
	ActivationRiskCreated       ActivationEventKey = "risk.created"
	ActivationFrameworkImported ActivationEventKey = "framework.imported"
	ActivationMemberInvited     ActivationEventKey = "member.invited"
	ActivationAssetConnected    ActivationEventKey = "asset.connected"
	ActivationMitigationCreated ActivationEventKey = "mitigation.created"
	ActivationReportGenerated   ActivationEventKey = "report.generated"

	// ActivationAhaReached is NOT a checklist step: it is the outcome the whole
	// journey exists to produce (see IsAhaReached). Recorded by the executive
	// dashboard use case the first time a cyber score is computed on the tenant's
	// OWN data while at least one compliance gap is identified.
	ActivationAhaReached ActivationEventKey = "aha.reached"

	// ActivationPostureRevealed is NOT a checklist step either: it is recorded
	// server-side when the Posture Reveal renders a posture summary computed from
	// the tenant's own rows (#438, D-011). It is the v2 Aha definition — the
	// endpoint the time-to-value promise is measured against.
	//
	// Named `ActivationPostureRevealed` rather than D-011's `EventKeyPostureRevealed`
	// so it reads like every other key in this block; the wire value
	// "posture.revealed" is exactly what D-011 specified.
	//
	// Deliberately NOT added to activationSteps: it is a server-observed outcome,
	// not a user chore, and putting it in the panel would show the user a row they
	// cannot act on. ValidateNonChecklistEventKeys() is what keeps a later edit
	// from quietly promoting it.
	ActivationPostureRevealed ActivationEventKey = "posture.revealed"
)

// nonChecklistEventKeys are the event keys that must never be bound to a
// checklist step: anchors (signup) and server-observed outcomes (aha.reached,
// posture.revealed).
var nonChecklistEventKeys = []ActivationEventKey{
	ActivationSignup,
	ActivationAhaReached,
	ActivationPostureRevealed,
}

// ActivationEvent is one immutable occurrence. The table is append-only: the same
// key may be recorded many times (a tenant creates many risks) and the read model
// only ever looks at the FIRST occurrence per key.
type ActivationEvent struct {
	// No `default:gen_random_uuid()` on purpose: it is Postgres-only, and it made
	// AutoMigrate unusable against the sqlite the repository tests run on — which
	// would have forced hand-written test DDL, the very thing that has drifted
	// from the models twice in this codebase. Every writer assigns the id in Go;
	// migration 0043 still declares the DB-level default for raw inserts.
	ID         uuid.UUID          `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID   uuid.UUID          `gorm:"type:uuid;not null;index:idx_activation_tenant_key,priority:1" json:"tenant_id"`
	UserID     *uuid.UUID         `gorm:"type:uuid;index" json:"user_id,omitempty"`
	EventKey   ActivationEventKey `gorm:"type:varchar(64);not null;index:idx_activation_tenant_key,priority:2" json:"event_key"`
	OccurredAt time.Time          `gorm:"not null;index" json:"occurred_at"`
	Payload    JSONMap            `gorm:"type:jsonb" json:"payload,omitempty"`
}

// TableName pins the table name.
func (ActivationEvent) TableName() string { return "activation_events" }

// Deliberately NOT Auditable: the governance trail is what a compliance officer
// reads, and filling it with "the product noted that you created a risk" rows
// would bury the signal. Activation is product telemetry, not a governance act —
// the underlying business mutation is audited on its own.

// ActivationCelebration records that a user has already seen the celebration for
// a step. This is what makes the burst idempotent across reloads and devices —
// the client never decides, it only obeys the `celebrate` flag the server sets.
type ActivationCelebration struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID     uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	UserID       uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_activation_celebration,priority:1" json:"user_id"`
	StepKey      string    `gorm:"type:varchar(64);not null;uniqueIndex:idx_activation_celebration,priority:2" json:"step_key"`
	CelebratedAt time.Time `gorm:"not null" json:"celebrated_at"`
}

// TableName pins the table name.
func (ActivationCelebration) TableName() string { return "activation_celebrations" }

// ActivationStepDef is one row of the canonical checklist. It is static data, not
// a table: the steps are a product decision, and keeping them in code means the
// bijection step ↔ event key can be enforced by a test rather than by hope.
type ActivationStepDef struct {
	Key      string             `json:"key"`
	EventKey ActivationEventKey `json:"event_key"`
	// LabelI18n / HintI18n are keyed by language ("fr", "en"). The API ships both
	// so the panel stays a dumb renderer with no copy of its own.
	LabelI18n map[string]string `json:"label_i18n"`
	HintI18n  map[string]string `json:"hint_i18n"`
	DeepLink  string            `json:"deep_link"`
	Order     int               `json:"order"`
	// Primary marks the one step that IS the promise of the product (the first
	// risk). The panel emphasises it; nothing else depends on it.
	Primary bool `json:"primary"`
}

// activationSteps is the canonical, ordered checklist. EXACTLY ONE event key per
// step — see ValidateActivationSteps.
var activationSteps = []ActivationStepDef{
	{
		Key:      "profile",
		EventKey: ActivationProfileCompleted,
		LabelI18n: map[string]string{
			"fr": "Complétez votre profil",
			"en": "Complete your profile",
		},
		HintI18n: map[string]string{
			"fr": "Votre nom et votre fonction rendent les assignations lisibles par l'équipe.",
			"en": "Your name and job title make assignments readable to your team.",
		},
		DeepLink: "/settings?tab=profile",
		Order:    1,
	},
	{
		Key:      "first_risk",
		EventKey: ActivationRiskCreated,
		LabelI18n: map[string]string{
			"fr": "Créez votre premier risque",
			"en": "Create your first risk",
		},
		HintI18n: map[string]string{
			"fr": "Le cœur d'OpenRisk : score et exposition financière calculés immédiatement.",
			"en": "The heart of OpenRisk: score and financial exposure computed instantly.",
		},
		DeepLink: "/risks?guided=1",
		Order:    2,
		Primary:  true,
	},
	{
		Key:      "framework",
		EventKey: ActivationFrameworkImported,
		LabelI18n: map[string]string{
			"fr": "Importez un référentiel",
			"en": "Import a framework",
		},
		HintI18n: map[string]string{
			"fr": "ISO 27001, COBAC, BCEAO… vos contrôles et vos écarts apparaissent en un clic.",
			"en": "ISO 27001, COBAC, BCEAO… your controls and gaps appear in one click.",
		},
		DeepLink: "/compliance",
		Order:    3,
	},
	{
		Key:      "asset",
		EventKey: ActivationAssetConnected,
		LabelI18n: map[string]string{
			"fr": "Cartographiez un actif",
			"en": "Map an asset",
		},
		HintI18n: map[string]string{
			"fr": "La criticité de l'actif pondère le score de chaque risque qui le touche.",
			"en": "Asset criticality weights the score of every risk that touches it.",
		},
		DeepLink: "/assets",
		Order:    4,
	},
	{
		Key:      "mitigation",
		EventKey: ActivationMitigationCreated,
		LabelI18n: map[string]string{
			"fr": "Planifiez un traitement",
			"en": "Plan a treatment",
		},
		HintI18n: map[string]string{
			"fr": "Un risque identifié sans plan reste un risque : ouvrez votre premier plan.",
			"en": "An identified risk with no plan is still a risk: open your first plan.",
		},
		DeepLink: "/risks/mitigations",
		Order:    5,
	},
	{
		Key:      "team",
		EventKey: ActivationMemberInvited,
		LabelI18n: map[string]string{
			"fr": "Invitez un collègue",
			"en": "Invite a teammate",
		},
		HintI18n: map[string]string{
			"fr": "La GRC est un sport d'équipe — assignez risques et contrôles à de vraies personnes.",
			"en": "GRC is a team sport — assign risks and controls to real people.",
		},
		DeepLink: "/settings/members?action=invite",
		Order:    6,
	},
	{
		Key:      "report",
		EventKey: ActivationReportGenerated,
		LabelI18n: map[string]string{
			"fr": "Générez votre premier rapport",
			"en": "Generate your first report",
		},
		HintI18n: map[string]string{
			"fr": "De la donnée brute à un document présentable en comité, en une action.",
			"en": "From raw data to a board-ready document, in a single action.",
		},
		DeepLink: "/reports",
		Order:    7,
	},
}

// ActivationSteps returns a copy of the canonical checklist, ordered.
func ActivationSteps() []ActivationStepDef {
	out := make([]ActivationStepDef, len(activationSteps))
	copy(out, activationSteps)
	return out
}

// ValidateActivationSteps enforces the invariant that killed the "two items
// struck through after one import" bug: every step maps to exactly one event key,
// no event key is shared by two steps, and orders are unique and contiguous.
// Called by a test; cheap enough to call at boot if ever wanted.
func ValidateActivationSteps() error {
	seenKey := map[string]bool{}
	seenEvent := map[ActivationEventKey]string{}
	seenOrder := map[int]bool{}
	for i, s := range activationSteps {
		if s.Key == "" {
			return NewValidationError("activation step has an empty key")
		}
		if seenKey[s.Key] {
			return NewValidationError("duplicate activation step key: " + s.Key)
		}
		seenKey[s.Key] = true

		if s.EventKey == "" {
			return NewValidationError("activation step " + s.Key + " has no event key")
		}
		if other, dup := seenEvent[s.EventKey]; dup {
			return NewValidationError("activation event " + string(s.EventKey) +
				" is bound to two steps (" + other + ", " + s.Key + ")")
		}
		seenEvent[s.EventKey] = s.Key

		if seenOrder[s.Order] {
			return NewValidationError("duplicate activation step order for " + s.Key)
		}
		seenOrder[s.Order] = true
		if s.Order != i+1 {
			return NewValidationError("activation step orders must be contiguous from 1")
		}

		for _, lang := range []string{"fr", "en"} {
			if s.LabelI18n[lang] == "" {
				return NewValidationError("activation step " + s.Key + " is missing the " + lang + " label")
			}
		}
		if s.DeepLink == "" {
			return NewValidationError("activation step " + s.Key + " has no deep link")
		}
	}
	return nil
}

// ValidateNonChecklistEventKeys is the sibling assertion to
// ValidateActivationSteps (D-011). The two answer different questions and
// neither implies the other:
//
//   - ValidateActivationSteps: every step in the catalogue maps to exactly one
//     event key. It iterates the catalogue and is deliberately left untouched.
//   - this one: no anchor or server-observed outcome has leaked INTO the
//     catalogue. That is the direction a future edit breaks — someone adds a
//     "see your posture" row to the checklist, binds it to posture.revealed, and
//     the bijection still holds while the panel now shows a row the user cannot
//     act on.
//
// Called by a test; cheap enough to call at boot if ever wanted.
func ValidateNonChecklistEventKeys() error {
	for _, key := range nonChecklistEventKeys {
		for _, s := range activationSteps {
			if s.EventKey == key {
				return NewValidationError("activation step " + s.Key +
					" is bound to the non-checklist event key " + string(key))
			}
		}
	}
	return nil
}

// NonChecklistEventKeys returns a copy of the keys that are recorded but never
// shown as a checklist row.
func NonChecklistEventKeys() []ActivationEventKey {
	out := make([]ActivationEventKey, len(nonChecklistEventKeys))
	copy(out, nonChecklistEventKeys)
	return out
}

// ---------------------------------------------------------------------------
// Onboarding wizard progress (spec §4) — per USER, not per tenant.
//
// Per-user is deliberate: a teammate invited into an already-configured
// organization still needs their own profile and their own sense of arrival, and
// the route guard ("no /dashboard until completed") is a statement about a
// person, not about a company.
// ---------------------------------------------------------------------------

// OnboardingStepKey enumerates the five wizard routes, in order.
type OnboardingStepKey string

const (
	OnboardingStepOrganization OnboardingStepKey = "organization"
	OnboardingStepGoal         OnboardingStepKey = "goal"
	OnboardingStepFramework    OnboardingStepKey = "framework"
	// OnboardingStepScore and OnboardingStepCover are the two steps #438 adds.
	// They exist because the first three ASK and the last two SHOW: scoring one
	// of the adopted risks against a live matrix, then watching the residual fall
	// when a control is accepted. Without them the tunnel collects five screens
	// of facts and returns nothing computed, which is the cliff the issue names.
	OnboardingStepScore OnboardingStepKey = "score"
	OnboardingStepCover OnboardingStepKey = "cover"

	// OnboardingStepProfile and OnboardingStepTeam are RETIRED from the tunnel
	// by #438 and are kept as constants on purpose.
	//
	// Stored `onboarding_progress` rows written before this change still carry
	// their answers under these keys, and `OnboardingProgress.StepAnswers` reads
	// by key. Deleting the constants would not delete the data; it would only
	// make the data unreadable.
	//
	//   - `profile` is MERGED INTO the organization step, which now collects the
	//     person as well as the company. `ActivationProfileCompleted` is still
	//     recorded — from there — so the checklist row still ticks.
	//   - `team` MOVES TO THE POSTURE REVEAL, where the issue's own design-system
	//     note puts it ("TagInput — invitations on the reveal"). Inviting people
	//     to look at nothing was the old ordering's mistake.
	//
	// Neither appears in OnboardingStepOrder, so ParseOnboardingStep rejects
	// them: no client may resurrect a retired route.
	OnboardingStepProfile OnboardingStepKey = "profile"
	OnboardingStepTeam    OnboardingStepKey = "team"
)

// OnboardingStepOrder is the canonical wizard order. Back-navigation is allowed,
// so this is a sequence, not a state machine with forbidden transitions.
//
// FIVE, still — #438 keeps the count so ProgressStepper keeps reading
// "Étape N sur 5" for a user who skips nothing.
var OnboardingStepOrder = []OnboardingStepKey{
	OnboardingStepOrganization,
	OnboardingStepGoal,
	OnboardingStepFramework,
	OnboardingStepScore,
	OnboardingStepCover,
}

// ParseOnboardingStep validates a raw step key.
func ParseOnboardingStep(raw string) (OnboardingStepKey, error) {
	for _, s := range OnboardingStepOrder {
		if string(s) == raw {
			return s, nil
		}
	}
	return "", NewValidationError("unknown onboarding step: " + raw)
}

// Index returns the 0-based position of a step in the wizard.
//
// Returns -1 for a step that is not in the sequence — a retired route
// (`profile`, `team`) or a typo. It used to return 0, which silently made every
// unknown key look like the FIRST step: after #438 retired two routes, a stored
// cursor pointing at one of them would have reported "you are on organization"
// and the wizard would have looked correct while being wrong.
func (s OnboardingStepKey) Index() int {
	for i, step := range OnboardingStepOrder {
		if step == s {
			return i
		}
	}
	return -1
}

// OnboardingProgress is one user's resumable wizard state.
type OnboardingProgress struct {
	ID       uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	TenantID uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	UserID   uuid.UUID `gorm:"type:uuid;not null;uniqueIndex" json:"user_id"`

	CurrentStep OnboardingStepKey `gorm:"type:varchar(32);not null;default:'organization'" json:"current_step"`
	Completed   bool              `gorm:"not null;default:false" json:"completed"`
	CompletedAt *time.Time        `json:"completed_at,omitempty"`

	// Promoted answers: these drive the sector/country templates (suggested risks
	// and frameworks), so they are columns rather than buried in the blob.
	Industry string `gorm:"type:varchar(64)" json:"industry,omitempty"`
	Country  string `gorm:"type:varchar(64)" json:"country,omitempty"`
	Goal     string `gorm:"type:varchar(64)" json:"goal,omitempty"`

	// Answers holds every step's raw payload so a half-finished wizard can be
	// resumed with the fields exactly as the user left them.
	Answers JSONMap `gorm:"type:jsonb" json:"answers,omitempty"`

	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName pins the table name.
func (OnboardingProgress) TableName() string { return "onboarding_progress" }

// Not Auditable either, for the same reason: a wizard answer is a preference,
// not a governance decision.

// StepAnswers returns the stored answers for one step (never nil).
func (p *OnboardingProgress) StepAnswers(step OnboardingStepKey) JSONMap {
	if p == nil || p.Answers == nil {
		return JSONMap{}
	}
	raw, ok := p.Answers[string(step)]
	if !ok {
		return JSONMap{}
	}
	if m, ok := raw.(map[string]interface{}); ok {
		return JSONMap(m)
	}
	if m, ok := raw.(JSONMap); ok {
		return m
	}
	return JSONMap{}
}

// SetStepAnswers stores one step's payload, allocating the blob if needed.
func (p *OnboardingProgress) SetStepAnswers(step OnboardingStepKey, answers JSONMap) {
	if p.Answers == nil {
		p.Answers = JSONMap{}
	}
	p.Answers[string(step)] = map[string]interface{}(answers)
}

// ---------------------------------------------------------------------------
// Ports
// ---------------------------------------------------------------------------

// ActivationRepository persists the activation event log and the per-user
// celebration ledger. Every method is tenant-scoped (RULE #2).
type ActivationRepository interface {
	// RecordEvent appends one occurrence. Append-only: never updates.
	RecordEvent(ctx context.Context, event *ActivationEvent) error
	// FirstOccurrences returns the earliest occurrence per event key for a tenant.
	// This is the only read the checklist needs, and it is what makes the panel
	// stable: later occurrences cannot move a completed_at.
	FirstOccurrences(ctx context.Context, tenantID uuid.UUID) (map[ActivationEventKey]time.Time, error)
	// HasEvent reports whether a key has ever occurred for a tenant (used to keep
	// once-only events, such as aha.reached, idempotent).
	HasEvent(ctx context.Context, tenantID uuid.UUID, key ActivationEventKey) (bool, error)
	// CelebratedSteps returns the step keys this user has already celebrated.
	CelebratedSteps(ctx context.Context, tenantID, userID uuid.UUID) (map[string]time.Time, error)
	// MarkCelebrated is idempotent (unique on user+step): a double POST is a no-op.
	MarkCelebrated(ctx context.Context, tenantID, userID uuid.UUID, stepKey string) error
}

// OnboardingRepository persists the resumable wizard state.
type OnboardingRepository interface {
	Get(ctx context.Context, tenantID, userID uuid.UUID) (*OnboardingProgress, error)
	Save(ctx context.Context, progress *OnboardingProgress) error
}
