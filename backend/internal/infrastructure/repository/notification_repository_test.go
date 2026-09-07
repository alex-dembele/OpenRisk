// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package repository

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/opendefender/openrisk/internal/domain"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupNotificationRepo(t *testing.T) *NotificationRepository {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`
		CREATE TABLE notifications (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			tenant_id TEXT NOT NULL,
			type TEXT,
			channel TEXT,
			status TEXT,
			subject TEXT,
			message TEXT,
			description TEXT,
			resource_id TEXT,
			resource_type TEXT,
			metadata TEXT,
			sent_at DATETIME,
			delivered_at DATETIME,
			read_at DATETIME,
			failure_reason TEXT,
			created_at DATETIME,
			updated_at DATETIME
		);
	`).Error)
	require.NoError(t, db.Exec(`
		CREATE TABLE notification_preferences (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			tenant_id TEXT NOT NULL,
			email_on_mitigation_deadline BOOLEAN,
			email_on_critical_risk BOOLEAN,
			email_on_action_assigned BOOLEAN,
			email_on_risk_update BOOLEAN,
			email_on_risk_resolved BOOLEAN,
			email_deadline_advance_days INTEGER,
			slack_enabled BOOLEAN,
			slack_channel_override TEXT,
			slack_on_mitigation_deadline BOOLEAN,
			slack_on_critical_risk BOOLEAN,
			slack_on_action_assigned BOOLEAN,
			webhook_enabled BOOLEAN,
			webhook_on_mitigation_deadline BOOLEAN,
			webhook_on_critical_risk BOOLEAN,
			webhook_on_action_assigned BOOLEAN,
			disable_all_notifications BOOLEAN,
			enable_sound_notifications BOOLEAN,
			enable_desktop_notifications BOOLEAN,
			created_at DATETIME,
			updated_at DATETIME
		);
	`).Error)
	return NewNotificationRepository(db)
}

func TestNotificationRepositoryTenantIsolationReadAndDelete(t *testing.T) {
	repo := setupNotificationRepo(t)
	userID := uuid.New()
	tenantAllowed := uuid.New()
	tenantOther := uuid.New()
	allowedID := uuid.New()
	otherID := uuid.New()

	require.NoError(t, repo.db.Table("notifications").Create(map[string]interface{}{
		"id":         allowedID.String(),
		"user_id":    userID.String(),
		"tenant_id":  tenantAllowed.String(),
		"channel":    string(domain.NotificationChannelInApp),
		"status":     string(domain.NotificationStatusPending),
		"subject":    "ok",
		"created_at": time.Now(),
		"updated_at": time.Now(),
	}).Error)
	require.NoError(t, repo.db.Table("notifications").Create(map[string]interface{}{
		"id":         otherID.String(),
		"user_id":    userID.String(),
		"tenant_id":  tenantOther.String(),
		"channel":    string(domain.NotificationChannelInApp),
		"status":     string(domain.NotificationStatusPending),
		"subject":    "other",
		"created_at": time.Now(),
		"updated_at": time.Now(),
	}).Error)

	items, err := repo.GetUserNotifications(userID, tenantAllowed, 10, 0)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, allowedID, items[0].ID)

	require.NoError(t, repo.DeleteNotification(otherID, userID, tenantAllowed))
	var stillThere domain.Notification
	err = repo.db.First(&stillThere, "id = ?", otherID).Error
	require.NoError(t, err)
}

func TestNotificationRepositoryMarkReadScopedByTenant(t *testing.T) {
	repo := setupNotificationRepo(t)
	userID := uuid.New()
	tenantA := uuid.New()
	tenantB := uuid.New()
	idA := uuid.New()
	idB := uuid.New()

	require.NoError(t, repo.db.Table("notifications").Create(map[string]interface{}{
		"id":         idA.String(),
		"user_id":    userID.String(),
		"tenant_id":  tenantA.String(),
		"channel":    string(domain.NotificationChannelInApp),
		"status":     string(domain.NotificationStatusPending),
		"subject":    "a",
		"created_at": time.Now(),
		"updated_at": time.Now(),
	}).Error)
	require.NoError(t, repo.db.Table("notifications").Create(map[string]interface{}{
		"id":         idB.String(),
		"user_id":    userID.String(),
		"tenant_id":  tenantB.String(),
		"channel":    string(domain.NotificationChannelInApp),
		"status":     string(domain.NotificationStatusPending),
		"subject":    "b",
		"created_at": time.Now(),
		"updated_at": time.Now(),
	}).Error)

	require.NoError(t, repo.MarkNotificationAsRead(idB, userID, tenantA))

	var targetA, targetB domain.Notification
	require.NoError(t, repo.db.First(&targetA, "id = ?", idA).Error)
	require.NoError(t, repo.db.First(&targetB, "id = ?", idB).Error)
	require.Equal(t, domain.NotificationStatusPending, targetB.Status)
	require.Equal(t, domain.NotificationStatusPending, targetA.Status)
}

func TestNotificationRepositoryPreferencesCreateAndUpdate(t *testing.T) {
	repo := setupNotificationRepo(t)
	userID := uuid.New()
	tenantID := uuid.New()

	prefs, err := repo.GetUserNotificationPreferences(userID, tenantID)
	require.NoError(t, err)
	require.Equal(t, 3, prefs.EmailDeadlineAdvanceDays)

	err = repo.UpdateNotificationPreferences(userID, tenantID, map[string]interface{}{
		"slack_enabled":               true,
		"email_deadline_advance_days": 5,
	})
	require.NoError(t, err)

	updated, err := repo.GetUserNotificationPreferences(userID, tenantID)
	require.NoError(t, err)
	require.True(t, updated.SlackEnabled)
	require.Equal(t, 5, updated.EmailDeadlineAdvanceDays)
	require.WithinDuration(t, time.Now(), updated.UpdatedAt, 2*time.Second)
}

// The two /notifications collection routes that the tests above did not reach
// (#421). GET /notifications is covered by
// TestNotificationRepositoryTenantIsolationReadAndDelete; the unread badge and
// the preferences screen each run their own query, and a query that is not
// asserted is not covered — the badge in particular is loaded on every page, so
// a lost predicate there counts every tenant's unread notifications for
// everyone.
//
// Both are keyed on (user_id, tenant_id): the same person legitimately belongs
// to more than one organisation, so the user id alone is not the boundary. The
// fixture therefore uses ONE user id in two tenants, which is the case a
// user-only predicate would get wrong.
func TestNotificationRepository_UnreadCountAndPreferences_AreTenantScoped(t *testing.T) {
	repo := setupNotificationRepo(t)
	userID := uuid.New()
	tenantA := uuid.New()
	tenantB := uuid.New()

	insert := func(tenant uuid.UUID, subject string) {
		require.NoError(t, repo.db.Table("notifications").Create(map[string]interface{}{
			"id":         uuid.New().String(),
			"user_id":    userID.String(),
			"tenant_id":  tenant.String(),
			"channel":    string(domain.NotificationChannelInApp),
			"status":     string(domain.NotificationStatusPending),
			"subject":    subject,
			"created_at": time.Now(),
			"updated_at": time.Now(),
		}).Error)
	}
	insert(tenantA, "a-1")
	for _, s := range []string{"b-1", "b-2", "b-3"} {
		insert(tenantB, s)
	}

	countA, err := repo.GetUnreadCount(userID, tenantA)
	require.NoError(t, err)
	require.Equal(t, int64(1), countA,
		"tenant B's three unread notifications must not inflate tenant A's badge")

	countB, err := repo.GetUnreadCount(userID, tenantB)
	require.NoError(t, err)
	require.Equal(t, int64(3), countB)

	countNone, err := repo.GetUnreadCount(userID, uuid.New())
	require.NoError(t, err)
	require.Equal(t, int64(0), countNone,
		"a tenant the user has no notifications in must read zero, not everything")

	// Preferences are per (user, tenant): the same person may want email in one
	// organisation and nothing in another, and reading the wrong row would both
	// leak a setting and mis-deliver.
	require.NoError(t, repo.UpdateNotificationPreferences(userID, tenantA, map[string]interface{}{
		"slack_enabled": true,
	}))
	prefsB, err := repo.GetUserNotificationPreferences(userID, tenantB)
	require.NoError(t, err)
	require.False(t, prefsB.SlackEnabled,
		"tenant A's preference was read back inside tenant B")

	prefsA, err := repo.GetUserNotificationPreferences(userID, tenantA)
	require.NoError(t, err)
	require.True(t, prefsA.SlackEnabled)
}
