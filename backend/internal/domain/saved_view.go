// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package domain

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// SavedView — a named view of a register (filters + sort + column layout),
// stored server-side so it survives a browser and can be handed to a colleague.
//
// Before this, views lived in localStorage: a risk manager who had built the
// right view of the register for their committee could not give it to anyone,
// and clearing site data destroyed it. Persisting it is a usability capability,
// NOT a compliance control — nothing here may be marketed as one (#580).
//
// ABSOLUTE RULE 2 applies with teeth: a view carries filter VALUES, which name
// assets, tags and people. A saved-views read that forgets tenant_id leaks the
// shape of another institution's register. Every query filters by tenant_id,
// and a foreign id reads back as not-found — never forbidden, because a 403
// confirms the row exists.
// ---------------------------------------------------------------------------

// SavedViewVisibility is a CLOSED two-value vocabulary. Anything finer — a view
// shared with a named subset of users, per-view ACLs — is a permissions design
// and is deliberately out of scope for #580.
type SavedViewVisibility string

const (
	// SavedViewPersonal is visible only to the user who saved it.
	SavedViewPersonal SavedViewVisibility = "personal"
	// SavedViewShared is visible to every member of the same tenant.
	SavedViewShared SavedViewVisibility = "shared"
)

// ParseSavedViewVisibility validates a visibility string. An empty value means
// "personal": sharing is an explicit act, never a default, because a default of
// shared would publish one user's working view to their whole institution.
func ParseSavedViewVisibility(s string) (SavedViewVisibility, error) {
	switch SavedViewVisibility(strings.ToLower(strings.TrimSpace(s))) {
	case "":
		return SavedViewPersonal, nil
	case SavedViewPersonal:
		return SavedViewPersonal, nil
	case SavedViewShared:
		return SavedViewShared, nil
	default:
		return "", NewValidationError(fmt.Sprintf("unknown visibility %q (expected personal or shared)", s))
	}
}

// Limits on the stored payload. A saved view is a handful of facet selections,
// not a document store: without a ceiling, the state column is an unbounded
// write primitive for any authenticated user.
const (
	MaxSavedViewNameLen    = 120
	MaxSavedViewTableIDLen = 64
	MaxSavedViewFacets     = 40
	MaxSavedViewFacetVals  = 200
	MaxSavedViewColumns    = 200
)

// SavedViewSort mirrors the frontend SortState exactly (shared/datatable/types.ts).
type SavedViewSort struct {
	Key string `json:"key"`
	Dir string `json:"dir"` // "asc" | "desc"
}

// SavedViewState is the part of the table state a view restores: the instant
// search, the facet selections and the sort. Page and page size are deliberately
// absent — "page 3" is not part of what a view means.
type SavedViewState struct {
	Q       string              `json:"q"`
	Filters map[string][]string `json:"filters"`
	Sort    *SavedViewSort      `json:"sort"`
}

// Value implements driver.Valuer so GORM persists the state as jsonb.
func (s SavedViewState) Value() (driver.Value, error) { return json.Marshal(s) }

// Scan implements sql.Scanner.
func (s *SavedViewState) Scan(value interface{}) error {
	b, err := jsonBytes("saved view state", value)
	if err != nil {
		return err
	}
	if len(b) == 0 {
		*s = SavedViewState{}
		return nil
	}
	return json.Unmarshal(b, s)
}

// Validate bounds the stored payload and normalises the sort direction.
func (s *SavedViewState) Validate() error {
	if len(s.Filters) > MaxSavedViewFacets {
		return NewValidationError(fmt.Sprintf("a view may carry at most %d facets", MaxSavedViewFacets))
	}
	for key, values := range s.Filters {
		if len(values) > MaxSavedViewFacetVals {
			return NewValidationError(fmt.Sprintf("facet %q carries more than %d values", key, MaxSavedViewFacetVals))
		}
	}
	if s.Sort != nil {
		if strings.TrimSpace(s.Sort.Key) == "" {
			// A sort with no column is not a sort. Drop it rather than storing a
			// half-object the frontend would have to defend against on read.
			s.Sort = nil
		} else if s.Sort.Dir != "asc" {
			s.Sort.Dir = "desc"
		}
	}
	return nil
}

// SavedViewColumns is the per-user column layout, mirroring the frontend
// ColumnPrefs. Unknown keys are dropped by the frontend on read, so the server
// stores what it is given and only bounds the size.
type SavedViewColumns struct {
	Order  []string `json:"order"`
	Hidden []string `json:"hidden"`
}

// Value implements driver.Valuer.
func (c SavedViewColumns) Value() (driver.Value, error) { return json.Marshal(c) }

// Scan implements sql.Scanner.
func (c *SavedViewColumns) Scan(value interface{}) error {
	b, err := jsonBytes("saved view columns", value)
	if err != nil {
		return err
	}
	if len(b) == 0 {
		*c = SavedViewColumns{}
		return nil
	}
	return json.Unmarshal(b, c)
}

// Validate bounds the column layout.
func (c *SavedViewColumns) Validate() error {
	if len(c.Order) > MaxSavedViewColumns || len(c.Hidden) > MaxSavedViewColumns {
		return NewValidationError(fmt.Sprintf("a view may carry at most %d columns", MaxSavedViewColumns))
	}
	return nil
}

// jsonBytes normalises the three shapes a driver hands back for a json column.
func jsonBytes(what string, value interface{}) ([]byte, error) {
	switch v := value.(type) {
	case nil:
		return nil, nil
	case []byte:
		return v, nil
	case string:
		return []byte(v), nil
	default:
		return nil, fmt.Errorf("%s: unsupported scan type %T", what, value)
	}
}

// SavedView is one named view of one register, owned by one user inside one
// tenant.
//
// TableID is the register the view belongs to ("risks", "vulnerabilities",
// "assets", …) — the same id the frontend DataTable is mounted with. It is a
// free string on purpose: the eighth register does not need a migration.
type SavedView struct {
	ID       uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	TenantID uuid.UUID `gorm:"type:uuid;not null;index:idx_saved_views_tenant_table" json:"tenant_id"`
	// UserID is the owner — the only person who may edit it, plus tenant admins.
	UserID  uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	TableID string    `gorm:"size:64;not null;index:idx_saved_views_tenant_table" json:"table_id"`
	Name    string    `gorm:"size:120;not null" json:"name"`

	Visibility SavedViewVisibility `gorm:"size:16;not null;default:'personal'" json:"visibility"`

	State   SavedViewState   `gorm:"type:jsonb" json:"state"`
	Columns SavedViewColumns `gorm:"type:jsonb" json:"columns"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	// OwnerEmail is resolved on read for display ("shared by Amina"). It is not
	// a column — the frontend needs a name, not a UUID, and a saved view is not
	// worth a join per row.
	OwnerEmail string `gorm:"-" json:"owner_email,omitempty"`
}

// TableName overrides the default GORM table name.
func (SavedView) TableName() string { return "saved_views" }

// IsShared reports whether the whole tenant can see this view.
func (v *SavedView) IsShared() bool { return v.Visibility == SavedViewShared }

// CanBeEditedBy reports whether a caller may update or delete this view: its
// owner, or a tenant admin. Callers must have already established that the view
// belongs to the caller's tenant — this answers authorisation, never isolation.
func (v *SavedView) CanBeEditedBy(userID uuid.UUID, isAdmin bool) bool {
	return v.UserID == userID || isAdmin
}

// IsVisibleTo reports whether a caller may see this view at all. A personal view
// belonging to somebody else is invisible even to a tenant admin: an admin's
// power is over the tenant's data, and one colleague's working filter is not
// something the product needs to expose to satisfy #580.
func (v *SavedView) IsVisibleTo(userID uuid.UUID) bool {
	return v.UserID == userID || v.IsShared()
}

// Validate normalises and bounds a view before it is persisted.
func (v *SavedView) Validate() error {
	v.Name = strings.TrimSpace(v.Name)
	v.TableID = strings.TrimSpace(v.TableID)

	if v.TenantID == uuid.Nil {
		return NewValidationError("tenant_id is required")
	}
	if v.UserID == uuid.Nil {
		return NewValidationError("user_id is required")
	}
	if v.TableID == "" {
		return NewValidationError("table_id is required")
	}
	if len(v.TableID) > MaxSavedViewTableIDLen {
		return NewValidationError(fmt.Sprintf("table_id is longer than %d characters", MaxSavedViewTableIDLen))
	}
	if v.Name == "" {
		return NewValidationError("name is required")
	}
	if len([]rune(v.Name)) > MaxSavedViewNameLen {
		return NewValidationError(fmt.Sprintf("name is longer than %d characters", MaxSavedViewNameLen))
	}
	if v.Visibility != SavedViewPersonal && v.Visibility != SavedViewShared {
		return NewValidationError("visibility must be personal or shared")
	}
	if err := v.State.Validate(); err != nil {
		return err
	}
	return v.Columns.Validate()
}

// SavedViewRepository is the port for persisting saved views.
//
// ABSOLUTE RULE: every method filters by tenant_id. A view belonging to another
// tenant MUST read back as (nil, nil) — the use case turns that into 404, which
// is byte-identical to the answer for an id that never existed.
type SavedViewRepository interface {
	// Create persists a new view.
	Create(ctx context.Context, view *SavedView) error

	// GetByID returns a view by id, scoped to a tenant.
	// Returns (nil, nil) when absent OR owned by another tenant.
	GetByID(ctx context.Context, id, tenantID uuid.UUID) (*SavedView, error)

	// ListVisible returns the views a user may see for one table: their own
	// (whatever the visibility) plus every shared view of the same tenant.
	ListVisible(ctx context.Context, tenantID, userID uuid.UUID, tableID string) ([]SavedView, error)

	// Update persists a change to an existing view, scoped to a tenant.
	Update(ctx context.Context, view *SavedView) error

	// Delete soft-deletes a view by id, scoped to a tenant.
	Delete(ctx context.Context, id, tenantID uuid.UUID) error

	// ExistsByName reports whether the user already owns a view of that name on
	// that table. excludeID skips one row so a rename onto its own name is not a
	// conflict with itself.
	ExistsByName(ctx context.Context, tenantID, userID uuid.UUID, tableID, name string, excludeID uuid.UUID) (bool, error)
}
