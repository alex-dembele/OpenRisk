// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package auth

import (
	"context"
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/opendefender/openrisk/internal/application/auth"
	coreauth "github.com/opendefender/openrisk/internal/auth"
	"github.com/opendefender/openrisk/internal/domain"
	"github.com/opendefender/openrisk/internal/middleware"
)

// Handler handles authentication endpoints
type Handler struct {
	loginUseCase    *auth.LoginUseCase
	registerUseCase *auth.RegisterUseCase
	refreshUseCase  *auth.RefreshTokenUseCase
	logoutUseCase   *auth.LogoutUseCase
	passwordHasher  auth.PasswordHasher
	audit           *coreauth.AuditService // L7: full-fidelity auth audit trail (optional)
	// newDeviceNotifier warns on sign-in from an unrecognised device. Optional.
	newDeviceNotifier *auth.NotifyNewDeviceUseCase
	// userLookup resolves the authenticated user for /auth/me. Optional; without it
	// Me returns only the id/tenant it can read from the request context.
	userLookup UserByIDReader
	// mfaStatus resolves the caller's MFA requirement for the session contract
	// (OR26-03). Optional; without it /auth/me omits the mfa block entirely
	// rather than guessing, and the client treats an absent block as "unknown"
	// instead of as "not required".
	mfaStatus *auth.MFAStatusResolver
	// memberLookup resolves the caller's membership so /auth/me can report the
	// CURRENT business role. Optional; without it the field is omitted rather
	// than guessed (#338).
	memberLookup OrganizationMemberReader
}

// UserByIDReader resolves a user by id for /auth/me.
type UserByIDReader interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
}

// OrganizationMemberReader resolves the caller's membership in the active
// organization.
//
// business_role lives on the MEMBERSHIP, not on the user: the same account can
// be an RSSI in one organization and a viewer in another, so there is no such
// field on domain.User to serialise. /auth/me is where every authenticated path
// — password login, MFA, session restore — reads its profile back, which makes
// it the one place worth resolving it (#338).
type OrganizationMemberReader interface {
	GetOrganizationMember(ctx context.Context, userID, orgID uuid.UUID) (*domain.OrganizationMember, error)
}

// WithNewDeviceNotifier enables the new-device sign-in alert.
func (h *Handler) WithNewDeviceNotifier(n *auth.NotifyNewDeviceUseCase) *Handler {
	h.newDeviceNotifier = n
	return h
}

// WithUserLookup makes /auth/me return the real authenticated user.
func (h *Handler) WithUserLookup(r UserByIDReader) *Handler {
	h.userLookup = r
	return h
}

// WithMFAStatus makes /auth/me carry the resolved MFA requirement (OR26-03).
func (h *Handler) WithMFAStatus(r *auth.MFAStatusResolver) *Handler {
	h.mfaStatus = r
	return h
}

// WithMemberLookup makes /auth/me report the caller's current business role.
func (h *Handler) WithMemberLookup(r OrganizationMemberReader) *Handler {
	h.memberLookup = r
	return h
}

// resolveRequestLocale picks the language for security email triggered by this
// request, so a notice lands in the language the user was working in.
func resolveRequestLocale(c *fiber.Ctx) string {
	if l := c.Get("X-Locale"); l == "en" || l == "fr" {
		return l
	}
	if len(c.Get("Accept-Language")) >= 2 && c.Get("Accept-Language")[:2] == "en" {
		return "en"
	}
	return "fr"
}

// NewHandler creates a new auth handler
func NewHandler(
	loginUseCase *auth.LoginUseCase,
	registerUseCase *auth.RegisterUseCase,
	refreshUseCase *auth.RefreshTokenUseCase,
	logoutUseCase *auth.LogoutUseCase,
	passwordHasher auth.PasswordHasher,
	audit *coreauth.AuditService,
) *Handler {
	return &Handler{
		loginUseCase:    loginUseCase,
		registerUseCase: registerUseCase,
		refreshUseCase:  refreshUseCase,
		logoutUseCase:   logoutUseCase,
		passwordHasher:  passwordHasher,
		audit:           audit,
	}
}

// logAudit records an auth event when an audit service is wired (nil-safe).
func (h *Handler) logAudit(c *fiber.Ctx, userID, tenantID *uuid.UUID, action coreauth.AuditAction, success bool, failureReason *string) {
	if h.audit == nil {
		return
	}
	_ = h.audit.LogFiber(c, userID, tenantID, action, success, failureReason)
}

// LoginRequest represents the login request body
type LoginRequest struct {
	Email             string `json:"email" validate:"required,email"`
	Password          string `json:"password" validate:"required"`
	DeviceFingerprint string `json:"device_fingerprint,omitempty"`
}

// LoginResponse represents the login response
type LoginResponse struct {
	User         *domain.User         `json:"user"`
	TokenPair    *coreauth.TokenPair  `json:"token_pair"`
	Organization *domain.Organization `json:"organization"`
	// BusinessRole is the member's GRC job-role preset (rssi/dsi/…) or "" for
	// root/admin. The frontend uses it to choose a role-appropriate landing.
	BusinessRole domain.BusinessRoleKey `json:"business_role,omitempty"`
	// CSRFToken mirrors the readable or_csrf cookie. Returning it here spares the
	// SPA a cookie read on first load; it is not a secret from the client, only
	// from other origins.
	CSRFToken string `json:"csrf_token,omitempty"`
	// MFA is the resolved enrolment state (OR26-03), carried on the response that
	// already exists rather than behind a second call the SPA would have to make
	// on every page. It reports state, whether enrolment is mandatory, and the
	// deadline — and never a secret: no TOTP seed, no QR payload, no backup codes.
	MFA *domain.MFADecision `json:"mfa,omitempty"`
}

// Login godoc
// @Summary User login
// @Description Authenticate user and return JWT tokens
// @Tags Authentication
// @Accept json
// @Produce json
// @Param request body LoginRequest true "Login credentials"
// @Success 200 {object} LoginResponse
// @Failure 400 {object} fiber.Map
// @Failure 401 {object} fiber.Map
// @Router /auth/login [post]
func (h *Handler) Login(c *fiber.Ctx) error {
	var req LoginRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Basic validation
	if req.Email == "" || req.Password == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Email and password are required",
		})
	}

	// Device fingerprint may arrive in the body or (more naturally) as a header.
	if req.DeviceFingerprint == "" {
		req.DeviceFingerprint = c.Get("X-Device-Fingerprint")
	}

	result, err := h.loginUseCase.Execute(c.UserContext(), auth.LoginInput{
		Email:             req.Email,
		Password:          req.Password,
		DeviceFingerprint: req.DeviceFingerprint,
		IP:                c.IP(),
		UserAgent:         c.Get("User-Agent"),
	})

	if err != nil {
		reason := "authentication failed"
		h.logAudit(c, nil, nil, coreauth.AuditActionLogin, false, &reason)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Authentication failed",
		})
	}

	// L4 — MFA gate: password verified but a second factor is required. No session
	// is issued; the client must POST /auth/mfa/challenge with the mfa_token.
	if result.MFARequired {
		var tenantID *uuid.UUID
		if result.Organization != nil {
			tenantID = &result.Organization.ID
		}
		h.logAudit(c, &result.User.ID, tenantID, coreauth.AuditActionLogin, true, strptr("mfa_required"))
		return c.JSON(fiber.Map{
			"mfa_required": true,
			"mfa_token":    result.MFAToken,
			"user_id":      result.User.ID,
		})
	}

	// L4 — MFA mandate: the role requires a second factor and none is enrolled.
	// Still no session: the client must complete /auth/mfa/setup + /auth/mfa/verify
	// with the mfa_token, which is an MFA_ENROLLMENT token accepted only there.
	if result.MFAEnrollmentRequired {
		var tenantID *uuid.UUID
		if result.Organization != nil {
			tenantID = &result.Organization.ID
		}
		h.logAudit(c, &result.User.ID, tenantID, coreauth.AuditActionLogin, true, strptr("mfa_enrollment_required"))
		return c.JSON(fiber.Map{
			"mfa_enrollment_required": true,
			"mfa_token":               result.MFAToken,
			"user_id":                 result.User.ID,
			// Says WHY enrolment is being demanded now — grace expired, or a
			// zero-day policy — so the enrolment screen can explain itself rather
			// than presenting an unexplained wall.
			"mfa": result.MFAStatus,
		})
	}

	var tenantID *uuid.UUID
	if result.Organization != nil {
		tenantID = &result.Organization.ID
	}
	h.logAudit(c, &result.User.ID, tenantID, coreauth.AuditActionLogin, true, nil)

	// Warn about a sign-in from a device this account has not used before. Fired
	// after the session is confirmed and best-effort throughout: a mail problem
	// must never fail a legitimate login.
	if h.newDeviceNotifier != nil {
		h.newDeviceNotifier.Execute(c.UserContext(), result.User, req.DeviceFingerprint, c.IP(), c.Get("User-Agent"), resolveRequestLocale(c))
	}

	csrfToken, err := h.issueSessionCookies(c, result.TokenPair)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Could not establish session",
		})
	}

	mfaStatus := result.MFAStatus
	return c.JSON(LoginResponse{
		User:         result.User,
		TokenPair:    result.TokenPair,
		Organization: result.Organization,
		BusinessRole: result.BusinessRole,
		CSRFToken:    csrfToken,
		MFA:          &mfaStatus,
	})
}

// issueSessionCookies writes the HttpOnly session pair and returns the CSRF
// token minted alongside it.
//
// The token pair stays in the response body as well. Dropping it would break
// every non-browser consumer in one step — the scanner agent, CI scripts and the
// generated OpenAPI client all read it — so the cookies are additive and the
// browser client simply stops persisting what it receives.
func (h *Handler) issueSessionCookies(c *fiber.Ctx, pair *coreauth.TokenPair) (string, error) {
	if pair == nil {
		return "", nil
	}
	return middleware.IssueSessionCookies(
		c,
		pair.AccessToken,
		pair.RefreshToken,
		coreauth.AccessTokenTTL,
		coreauth.RefreshTokenTTL,
	)
}

func strptr(s string) *string { return &s }

// RegisterRequest represents the registration request body
type RegisterRequest struct {
	Email       string `json:"email" validate:"required,email"`
	Username    string `json:"username" validate:"required,min=3,max=50"`
	Password    string `json:"password" validate:"required,min=12"`
	FullName    string `json:"full_name" validate:"required"`
	CompanyName string `json:"company_name" validate:"required"`
}

// RegisterResponse represents the registration response
type RegisterResponse struct {
	User         *domain.User         `json:"user"`
	Organization *domain.Organization `json:"organization"`
	Message      string               `json:"message"`
}

// Register godoc
// @Summary User registration
// @Description Register a new user and create organization
// @Tags Authentication
// @Accept json
// @Produce json
// @Param request body RegisterRequest true "Registration data"
// @Success 201 {object} RegisterResponse
// @Failure 400 {object} fiber.Map
// @Failure 409 {object} fiber.Map
// @Router /auth/register [post]
func (h *Handler) Register(c *fiber.Ctx) error {
	var req RegisterRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	result, err := h.registerUseCase.Execute(c.UserContext(), auth.RegisterInput{
		Email:       req.Email,
		Username:    req.Username,
		Password:    req.Password,
		FullName:    req.FullName,
		CompanyName: req.CompanyName,
	})

	if err != nil {
		if domainErr, ok := err.(*domain.AppError); ok {
			switch domainErr.Err {
			case domain.ErrValidation:
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
					"error": domainErr.Message,
				})
			case domain.ErrConflict:
				return c.Status(fiber.StatusConflict).JSON(fiber.Map{
					"error": domainErr.Message,
				})
			}
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Registration failed",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(RegisterResponse{
		User:         result.User,
		Organization: result.Organization,
		Message:      result.Message,
	})
}

// RefreshTokenRequest represents the refresh token request body
type RefreshTokenRequest struct {
	RefreshToken      string `json:"refresh_token" validate:"required"`
	DeviceFingerprint string `json:"device_fingerprint,omitempty"`
}

// RefreshTokenResponse represents the refresh token response
type RefreshTokenResponse struct {
	// CSRFToken mirrors the refreshed or_csrf cookie (see LoginResponse).
	CSRFToken string              `json:"csrf_token,omitempty"`
	TokenPair *coreauth.TokenPair `json:"token_pair"`
}

// RefreshToken godoc
// @Summary Refresh access token
// @Description Refresh access token using refresh token
// @Tags Authentication
// @Accept json
// @Produce json
// @Param request body RefreshTokenRequest true "Refresh token"
// @Success 200 {object} RefreshTokenResponse
// @Failure 400 {object} fiber.Map
// @Failure 401 {object} fiber.Map
// @Router /auth/refresh [post]
func (h *Handler) RefreshToken(c *fiber.Ctx) error {
	var req RefreshTokenRequest
	// A cookie-based client sends no body at all, so a parse failure is only
	// fatal once the cookie has also come up empty.
	parseErr := c.BodyParser(&req)

	if req.RefreshToken == "" {
		req.RefreshToken = middleware.RefreshTokenFromRequest(c)
	}
	if req.RefreshToken == "" {
		if parseErr != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid request body",
			})
		}
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Token refresh failed",
		})
	}

	if req.DeviceFingerprint == "" {
		req.DeviceFingerprint = c.Get("X-Device-Fingerprint")
	}

	result, err := h.refreshUseCase.Execute(c.UserContext(), auth.RefreshTokenInput{
		RefreshToken:      req.RefreshToken,
		DeviceFingerprint: req.DeviceFingerprint,
	})

	if err != nil {
		// Reuse of a rotated token (or a lost concurrent-rotation race) has already
		// revoked the whole family server-side. Clear the browser's cookies and
		// record it as the distinct security event it is, so a leaked token shows
		// up in the audit trail rather than as one more "refresh failed".
		if errors.Is(err, coreauth.ErrRefreshTokenReuse) {
			middleware.ClearSessionCookies(c)
			h.logAudit(c, nil, nil, coreauth.AuditActionRefreshReuse, false, strptr("reuse_detected"))
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Session revoked. Please sign in again.",
				"code":  "REFRESH_REUSE_DETECTED",
			})
		}
		reason := "token refresh failed"
		h.logAudit(c, nil, nil, coreauth.AuditActionRefresh, false, &reason)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Token refresh failed",
		})
	}

	h.logAudit(c, nil, nil, coreauth.AuditActionRefresh, true, nil)

	// Rotation is single-use (L3): the presented refresh token was destroyed, so
	// the cookies must be re-issued or the browser would keep replaying a token
	// that is now revoked. A fresh CSRF token comes with them.
	csrfToken, err := h.issueSessionCookies(c, result.TokenPair)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Could not establish session",
		})
	}

	return c.JSON(RefreshTokenResponse{
		TokenPair: result.TokenPair,
		CSRFToken: csrfToken,
	})
}

// LogoutRequest represents the logout request body
type LogoutRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

// Logout godoc
// @Summary User logout
// @Description Revoke refresh token
// @Tags Authentication
// @Accept json
// @Produce json
// @Param request body LogoutRequest true "Refresh token to revoke"
// @Success 200 {object} fiber.Map
// @Failure 400 {object} fiber.Map
// @Failure 401 {object} fiber.Map
// @Router /auth/logout [post]
func (h *Handler) Logout(c *fiber.Ctx) error {
	var req LogoutRequest
	_ = c.BodyParser(&req) // cookie-based clients send no body

	if req.RefreshToken == "" {
		req.RefreshToken = middleware.RefreshTokenFromRequest(c)
	}

	// Clear unconditionally, before the revocation attempt. If revocation fails
	// the browser must still lose its session — a logout that leaves usable
	// cookies behind is worse than one that reports an error.
	middleware.ClearSessionCookies(c)

	if req.RefreshToken == "" {
		return c.JSON(fiber.Map{"message": "Logged out"})
	}

	err := h.logoutUseCase.Execute(c.UserContext(), auth.LogoutInput{
		RefreshToken: req.RefreshToken,
	})

	if err != nil {
		reason := "logout failed"
		h.logAudit(c, nil, nil, coreauth.AuditActionLogout, false, &reason)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Logout failed",
		})
	}

	h.logAudit(c, nil, nil, coreauth.AuditActionLogout, true, nil)
	return c.JSON(fiber.Map{
		"message": "Logged out successfully",
	})
}

// Me godoc
// @Summary Get current user profile
// @Description Get profile of currently authenticated user
// @Tags Authentication
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} domain.User
// @Failure 401 {object} fiber.Map
// @Router /auth/me [get]
func (h *Handler) Me(c *fiber.Ctx) error {
	mwCtx := middleware.GetContext(c)
	if mwCtx == nil || mwCtx.UserID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "User not authenticated",
		})
	}

	// OR26-03 — the session contract. Resolved server-side on every call so the
	// client cannot manufacture it, and omitted (rather than guessed) when the
	// resolver is unwired or errors: an absent block reads as "unknown", which
	// the banner renders as an error state, not as "you are fine".
	mfaBlock := h.resolveMFABlock(c, mwCtx.UserID, mwCtx.OrganizationID)

	// Resolve the real user. Password/MFA secret/deletion markers are json:"-", so
	// the domain.User serialises without secrets.
	if h.userLookup != nil {
		user, err := h.userLookup.GetByID(c.UserContext(), mwCtx.UserID)
		if err == nil && user != nil {
			body := fiber.Map{
				"user":            user,
				"organization_id": mwCtx.OrganizationID,
			}
			if mfaBlock != nil {
				body["mfa"] = mfaBlock
			}
			// The CURRENT business role, re-read per call like everything else
			// here. The client picks its persona and its landing route from
			// this; before #338 the field did not exist on this response, so
			// every path that restores a profile through /auth/me — MFA login
			// above all — resolved it to "" and dropped the user on the default
			// dashboard while their permissions said otherwise.
			if role, ok := h.resolveBusinessRole(c, mwCtx.UserID, mwCtx.OrganizationID); ok {
				body["business_role"] = role
			}
			return c.JSON(body)
		}
		if err == nil && user == nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to load user"})
	}

	// No lookup wired: return only what the verified token already carries, rather
	// than fabricating profile fields.
	body := fiber.Map{
		"id":              mwCtx.UserID,
		"organization_id": mwCtx.OrganizationID,
	}
	if mfaBlock != nil {
		body["mfa"] = mfaBlock
	}
	return c.JSON(body)
}

// resolveBusinessRole reads the caller's CURRENT business role for the active
// organization. The bool reports whether it could be resolved at all.
//
// Omitted rather than defaulted when it cannot be, for the same reason as the
// MFA block below: "" is a REAL value — root and admin carry no job-role preset
// — so returning it on a database error would assert "this user is an admin"
// when the truthful answer is "not known". An absent key leaves the client on
// what it already had; a present "" is an answer.
func (h *Handler) resolveBusinessRole(c *fiber.Ctx, userID, orgID uuid.UUID) (domain.BusinessRoleKey, bool) {
	if h.memberLookup == nil || orgID == uuid.Nil {
		return "", false
	}
	member, err := h.memberLookup.GetOrganizationMember(c.UserContext(), userID, orgID)
	if err != nil || member == nil {
		return "", false
	}
	// A revoked membership is not a persona. Enforcement lives at token mint and
	// in the request guard; reporting a role here for a membership that no longer
	// authorizes anything would only mislead the navigation.
	if !member.IsActive {
		return "", false
	}
	return member.BusinessRole, true
}

// resolveMFABlock resolves the caller's MFA requirement, or nil when it cannot
// be determined.
//
// Nil rather than a zero value on purpose: a zero MFADecision serialises as
// "not configured, not required", which is indistinguishable from a genuine
// answer and would let a transient database problem quietly tell the UI that a
// privileged account is fine. Enforcement does not depend on this — the
// request-time guard is what blocks — so reporting "unknown" here is safe.
func (h *Handler) resolveMFABlock(c *fiber.Ctx, userID, tenantID uuid.UUID) *domain.MFADecision {
	if h.mfaStatus == nil {
		return nil
	}
	orgRole := ""
	if claims := middleware.GetUserClaims(c); claims != nil {
		orgRole = claims.OrgRoles[tenantID]
	}
	decision, err := h.mfaStatus.Resolve(c.UserContext(), userID, tenantID, orgRole)
	if err != nil {
		return nil
	}
	return &decision
}
