// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package domain

import (
	"database/sql/driver"
	"time"

	"net"

	"github.com/google/uuid"
)

// AuditLogAction defines the types of actions that can be audited
type AuditLogAction string

const (
	ActionLogin           AuditLogAction = "login"
	ActionLoginFailed     AuditLogAction = "login_failed"
	ActionRegister        AuditLogAction = "register"
	ActionLogout          AuditLogAction = "logout"
	ActionTokenRefresh    AuditLogAction = "token_refresh"
	ActionRoleChange      AuditLogAction = "role_change"
	ActionUserDelete      AuditLogAction = "user_delete"
	ActionUserDeactivate  AuditLogAction = "user_deactivate"
	ActionUserActivate    AuditLogAction = "user_activate"
	ActionUserCreate      AuditLogAction = "user_create"
	ActionPasswordChange  AuditLogAction = "password_change"
	ActionIntegrationTest AuditLogAction = "integration_test"
)

func (a AuditLogAction) String() string {
	return string(a)
}

// AuditLogResource defines the types of resources that can be audited
type AuditLogResource string

const (
	ResourceAuth        AuditLogResource = "auth"
	ResourceUser        AuditLogResource = "user"
	ResourceRole        AuditLogResource = "role"
	ResourceIntegration AuditLogResource = "integration"
)

func (r AuditLogResource) String() string {
	return string(r)
}

// AuditLogResult defines the outcome of an audited action
type AuditLogResult string

const (
	ResultSuccess AuditLogResult = "success"
	ResultFailure AuditLogResult = "failure"
)

func (r AuditLogResult) String() string {
	return string(r)
}

// AuditLog represents an audit trail entry for authentication and authorization events.
//
// TENANCY (#532). This table had no tenant column at all until 2026-09-04, so
// GET /api/v1/audit-logs returned every tenant's authentication log to any
// organisation administrator — the same failure as the retired
// GET /timeline/recent, which returned every tenant's risk history because
// RiskHistory carried no tenant_id either (docs/JOURNAL.md item 36).
//
// TenantID is a POINTER, and that is the whole isolation mechanism rather than a
// convenience:
//
//   - It holds an organizations.id — the same id space as the `tenant_id` JWT
//     claim, and NOT users.tenant_id, which points at the separate `tenants`
//     table. Writing the wrong one attributes a row to an id no session ever
//     carries, which loses the event silently instead of leaking it.
//   - NULL means "could not be attributed to an organisation": a pre-auth event
//     where no user resolved. In SQL `tenant_id = ?` never matches NULL, so an
//     unattributed row is invisible to EVERY tenant rather than visible to all
//     of them. That is the fail-closed direction, and it is why the reads below
//     need no special case for it.
//
// Mirrors AuthAuditLog.TenantID, the same shape on the newer auth trail.
type AuditLog struct {
	ID           uuid.UUID        `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	TenantID     *uuid.UUID       `gorm:"type:uuid;index" json:"tenant_id,omitempty"` // NULL = unattributed; see above
	UserID       *uuid.UUID       `gorm:"index" json:"user_id,omitempty"`             // NULL for pre-auth events
	Action       AuditLogAction   `gorm:"type:varchar(100);index" json:"action"`
	Resource     AuditLogResource `gorm:"type:varchar(100)" json:"resource,omitempty"`
	ResourceID   *uuid.UUID       `json:"resource_id,omitempty"` // ID of affected resource
	Result       AuditLogResult   `gorm:"type:varchar(20);index" json:"result"`
	ErrorMessage string           `json:"error_message,omitempty"` // Description of failure
	IPAddress    *net.IP          `gorm:"type:inet" json:"ip_address,omitempty"`
	UserAgent    string           `json:"user_agent,omitempty"`
	Timestamp    time.Time        `gorm:"index;default:CURRENT_TIMESTAMP" json:"timestamp"`
	// Metadata for advanced queries
	Duration int64 `json:"duration_ms,omitempty"` // Action duration in milliseconds
}

// Implement database scanner and valuer interfaces
func (a *AuditLogAction) Scan(value interface{}) error {
	*a = AuditLogAction(value.(string))
	return nil
}

func (a AuditLogAction) Value() (driver.Value, error) {
	return a.String(), nil
}

func (r *AuditLogResource) Scan(value interface{}) error {
	*r = AuditLogResource(value.(string))
	return nil
}

func (r AuditLogResource) Value() (driver.Value, error) {
	return r.String(), nil
}

func (r *AuditLogResult) Scan(value interface{}) error {
	*r = AuditLogResult(value.(string))
	return nil
}

func (r AuditLogResult) Value() (driver.Value, error) {
	return r.String(), nil
}

// TableName specifies the table name for this model
func (AuditLog) TableName() string {
	return "audit_logs"
}
