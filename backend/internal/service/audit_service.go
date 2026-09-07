// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package service

import (
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/opendefender/openrisk/internal/domain"
	"github.com/opendefender/openrisk/internal/infrastructure/database"
)

// AuditService handles logging of authentication and authorization events.
//
// TENANCY (#532). Every write takes the acting session's organisation and every
// read filters on it. Before 2026-09-04 neither did: the table had no tenant
// column, and GET /api/v1/audit-logs returned every tenant's log to any
// organisation administrator.
//
// The tenant is a PARAMETER on every method rather than something read from a
// context inside, so adding a caller is a compile error until that caller has
// said which organisation it is acting for. A tenant that can be forgotten is a
// tenant that will be.
type AuditService struct {
	// db defaults to the package-global handle. It is a field so tests can drive
	// the real methods against a fixture instead of asserting against a retyped
	// copy of their SQL.
	db *gorm.DB
}

// NewAuditService creates a new audit service bound to the process-wide handle.
func NewAuditService() *AuditService {
	return &AuditService{}
}

// NewAuditServiceWithDB binds the service to an explicit handle. For tests.
func NewAuditServiceWithDB(db *gorm.DB) *AuditService {
	return &AuditService{db: db}
}

func (s *AuditService) handle() *gorm.DB {
	if s.db != nil {
		return s.db
	}
	return database.DB
}

// errNoTenant is returned by every read below when no organisation resolves.
//
// It refuses rather than emitting `tenant_id = '00000000-...'` and returning an
// empty page. Both are safe; only one is honest. An empty page reads as "this
// organisation has no audit history", which is a different statement from "this
// request carried no organisation", and a caller cannot tell them apart.
var errNoTenant = errors.New("audit: an organisation is required to read the audit trail")

// LogLogin logs a user login attempt
func (s *AuditService) LogLogin(tenantID *uuid.UUID, userID uuid.UUID, result domain.AuditLogResult, ipAddress string, userAgent string, errorMsg string) error {
	return s.LogAction(&domain.AuditLog{
		TenantID:     tenantID,
		UserID:       &userID,
		Action:       domain.ActionLogin,
		Resource:     domain.ResourceAuth,
		Result:       result,
		ErrorMessage: errorMsg,
		IPAddress:    parseIPAddress(ipAddress),
		UserAgent:    userAgent,
		Timestamp:    time.Now(),
	})
}

// LogRegister logs a user registration attempt
func (s *AuditService) LogRegister(tenantID *uuid.UUID, userID *uuid.UUID, result domain.AuditLogResult, ipAddress string, userAgent string, errorMsg string) error {
	return s.LogAction(&domain.AuditLog{
		TenantID:     tenantID,
		UserID:       userID,
		Action:       domain.ActionRegister,
		Resource:     domain.ResourceAuth,
		Result:       result,
		ErrorMessage: errorMsg,
		IPAddress:    parseIPAddress(ipAddress),
		UserAgent:    userAgent,
		Timestamp:    time.Now(),
	})
}

// LogLogout logs a user logout
func (s *AuditService) LogLogout(tenantID *uuid.UUID, userID uuid.UUID, ipAddress string, userAgent string) error {
	return s.LogAction(&domain.AuditLog{
		TenantID:  tenantID,
		UserID:    &userID,
		Action:    domain.ActionLogout,
		Resource:  domain.ResourceAuth,
		Result:    domain.ResultSuccess,
		IPAddress: parseIPAddress(ipAddress),
		UserAgent: userAgent,
		Timestamp: time.Now(),
	})
}

// LogTokenRefresh logs a token refresh attempt
func (s *AuditService) LogTokenRefresh(tenantID *uuid.UUID, userID uuid.UUID, result domain.AuditLogResult, ipAddress string, userAgent string, errorMsg string) error {
	return s.LogAction(&domain.AuditLog{
		TenantID:     tenantID,
		UserID:       &userID,
		Action:       domain.ActionTokenRefresh,
		Resource:     domain.ResourceAuth,
		Result:       result,
		ErrorMessage: errorMsg,
		IPAddress:    parseIPAddress(ipAddress),
		UserAgent:    userAgent,
		Timestamp:    time.Now(),
	})
}

// LogRoleChange logs a user role change
func (s *AuditService) LogRoleChange(tenantID *uuid.UUID, performedByID uuid.UUID, targetUserID uuid.UUID, oldRole string, newRole string, ipAddress string, userAgent string) error {
	errorMsg := fmt.Sprintf("Role changed from %s to %s", oldRole, newRole)
	return s.LogAction(&domain.AuditLog{
		TenantID:     tenantID,
		UserID:       &performedByID,
		Action:       domain.ActionRoleChange,
		Resource:     domain.ResourceUser,
		ResourceID:   &targetUserID,
		Result:       domain.ResultSuccess,
		ErrorMessage: errorMsg, // We reuse this field to store the change description
		IPAddress:    parseIPAddress(ipAddress),
		UserAgent:    userAgent,
		Timestamp:    time.Now(),
	})
}

// LogUserDeactivate logs a user deactivation
func (s *AuditService) LogUserDeactivate(tenantID *uuid.UUID, performedByID uuid.UUID, targetUserID uuid.UUID, ipAddress string, userAgent string) error {
	return s.LogAction(&domain.AuditLog{
		TenantID:   tenantID,
		UserID:     &performedByID,
		Action:     domain.ActionUserDeactivate,
		Resource:   domain.ResourceUser,
		ResourceID: &targetUserID,
		Result:     domain.ResultSuccess,
		IPAddress:  parseIPAddress(ipAddress),
		UserAgent:  userAgent,
		Timestamp:  time.Now(),
	})
}

// LogUserActivate logs a user activation
func (s *AuditService) LogUserActivate(tenantID *uuid.UUID, performedByID uuid.UUID, targetUserID uuid.UUID, ipAddress string, userAgent string) error {
	return s.LogAction(&domain.AuditLog{
		TenantID:   tenantID,
		UserID:     &performedByID,
		Action:     domain.ActionUserActivate,
		Resource:   domain.ResourceUser,
		ResourceID: &targetUserID,
		Result:     domain.ResultSuccess,
		IPAddress:  parseIPAddress(ipAddress),
		UserAgent:  userAgent,
		Timestamp:  time.Now(),
	})
}

// LogUserDelete logs a user deletion
func (s *AuditService) LogUserDelete(tenantID *uuid.UUID, performedByID uuid.UUID, targetUserID uuid.UUID, ipAddress string, userAgent string) error {
	return s.LogAction(&domain.AuditLog{
		TenantID:   tenantID,
		UserID:     &performedByID,
		Action:     domain.ActionUserDelete,
		Resource:   domain.ResourceUser,
		ResourceID: &targetUserID,
		Result:     domain.ResultSuccess,
		IPAddress:  parseIPAddress(ipAddress),
		UserAgent:  userAgent,
		Timestamp:  time.Now(),
	})
}

// LogAction logs a generic audit action.
//
// log.TenantID is the caller's responsibility and is deliberately not defaulted
// here. A default would have to be either the zero UUID (a value no organisation
// has, so the row would be silently unreadable) or something guessed from
// ambient state (an attribution presented as a fact). Leaving it explicit means
// a NULL in this column always records a real judgement — "no organisation could
// be resolved for this event" — rather than an omission.
func (s *AuditService) LogAction(log *domain.AuditLog) error {
	if log == nil {
		return fmt.Errorf("audit log cannot be nil")
	}
	// The zero UUID is not an organisation. Accepting it would write a row that
	// no tenant can ever read while looking, in the column, exactly like a real
	// attribution. NULL says the same thing honestly.
	if log.TenantID != nil && *log.TenantID == uuid.Nil {
		log.TenantID = nil
	}

	// Set timestamp if not already set
	if log.Timestamp.IsZero() {
		log.Timestamp = time.Now()
	}

	// Generate ID if not present
	if log.ID == uuid.Nil {
		log.ID = uuid.New()
	}

	// Insert into database
	if err := s.handle().Create(log).Error; err != nil {
		return fmt.Errorf("failed to log audit action: %w", err)
	}

	return nil
}

// GetAuditLogsByUser retrieves one organisation's audit logs for a specific user.
//
// The tenant predicate comes FIRST and is not optional. Filtering on user_id
// alone let an administrator name any user id in the deployment and read that
// person's history, whichever organisation they belong to.
func (s *AuditService) GetAuditLogsByUser(tenantID uuid.UUID, userID uuid.UUID, limit int, offset int) ([]domain.AuditLog, error) {
	if tenantID == uuid.Nil {
		return nil, errNoTenant
	}
	var logs []domain.AuditLog
	query := s.handle().Where("tenant_id = ? AND user_id = ?", tenantID, userID).
		Order("timestamp DESC").
		Limit(limit).
		Offset(offset)

	if err := query.Find(&logs).Error; err != nil {
		return nil, fmt.Errorf("failed to retrieve audit logs for user: %w", err)
	}

	return logs, nil
}

// GetAuditLogsByAction retrieves one organisation's audit logs for an action.
//
// This was the sharpest of the three reads: an attacker-chosen action name
// returned every tenant's events of that kind, most recent first — `login_failed`
// across the whole deployment, for instance.
func (s *AuditService) GetAuditLogsByAction(tenantID uuid.UUID, action domain.AuditLogAction, limit int, offset int) ([]domain.AuditLog, error) {
	if tenantID == uuid.Nil {
		return nil, errNoTenant
	}
	var logs []domain.AuditLog
	query := s.handle().Where("tenant_id = ? AND action = ?", tenantID, action.String()).
		Order("timestamp DESC").
		Limit(limit).
		Offset(offset)

	if err := query.Find(&logs).Error; err != nil {
		return nil, fmt.Errorf("failed to retrieve audit logs by action: %w", err)
	}

	return logs, nil
}

// GetAuditLogsByIPAddress retrieves one organisation's audit logs from an IP.
//
// No route reaches this today, and it is scoped anyway: an unscoped IP lookup is
// a cross-tenant correlation tool — "which other organisations does this address
// touch" — and leaving it unscoped because nothing calls it yet is how the next
// route inherits the defect.
func (s *AuditService) GetAuditLogsByIPAddress(tenantID uuid.UUID, ipAddress string, limit int, offset int) ([]domain.AuditLog, error) {
	if tenantID == uuid.Nil {
		return nil, errNoTenant
	}
	var logs []domain.AuditLog
	query := s.handle().Where("tenant_id = ? AND ip_address = ?::inet", tenantID, ipAddress).
		Order("timestamp DESC").
		Limit(limit).
		Offset(offset)

	if err := query.Find(&logs).Error; err != nil {
		return nil, fmt.Errorf("failed to retrieve audit logs by IP address: %w", err)
	}

	return logs, nil
}

// GetAuditLogsByDateRange retrieves one organisation's audit logs in a window.
//
// This is what GET /api/v1/audit-logs calls. It filtered on the timestamp alone,
// so the page an administrator opened carried every tenant's rows (#532).
func (s *AuditService) GetAuditLogsByDateRange(tenantID uuid.UUID, startTime time.Time, endTime time.Time, limit int, offset int) ([]domain.AuditLog, error) {
	if tenantID == uuid.Nil {
		return nil, errNoTenant
	}
	var logs []domain.AuditLog
	query := s.handle().Where("tenant_id = ? AND timestamp BETWEEN ? AND ?", tenantID, startTime, endTime).
		Order("timestamp DESC").
		Limit(limit).
		Offset(offset)

	if err := query.Find(&logs).Error; err != nil {
		return nil, fmt.Errorf("failed to retrieve audit logs by date range: %w", err)
	}

	return logs, nil
}

// Helper function to parse IP address string
func parseIPAddress(ipStr string) *net.IP {
	if ipStr == "" {
		return nil
	}
	ip := net.ParseIP(ipStr)
	return &ip
}
