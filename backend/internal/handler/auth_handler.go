// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU Affero General Public License v3.0 (see LICENSE).

package handler

import (
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/opendefender/openrisk/internal/auth"
	"github.com/opendefender/openrisk/internal/domain"
	"github.com/opendefender/openrisk/internal/infrastructure/database"
	"github.com/opendefender/openrisk/internal/middleware"
	"github.com/opendefender/openrisk/internal/service"
)

type LoginInput struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8"`
}

type RegisterInput struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=12"`
	Username string `json:"username" validate:"required,min=3"`
	FullName string `json:"full_name" validate:"required"`
}

type RefreshInput struct {
	Token string `json:"token" validate:"required"`
}

type AuthResponse struct {
	Token     string   `json:"token"`
	User      *UserDTO `json:"user"`
	ExpiresIn int64    `json:"expires_in"`
}

type UserDTO struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Username string `json:"username"`
	FullName string `json:"full_name"`
	Role     string `json:"role"`
}

type AuthHandler struct {
	authService    *service.AuthService
	auditService   *service.AuditService
	passwordHasher auth.PasswordHasher
}

func NewAuthHandler() *AuthHandler {
	jwtSecret := os.Getenv("JWT_SECRET")
	authService := service.NewAuthService(jwtSecret, 24*time.Hour)
	auditService := service.NewAuditService()
	// Use Argon2id for secure password hashing (OWASP recommended)
	passwordHasher := auth.NewArgon2idPasswordHasher()
	return &AuthHandler{
		authService:    authService,
		auditService:   auditService,
		passwordHasher: passwordHasher,
	}
}

// Login handles user authentication and returns JWT token
func (h *AuthHandler) Login(c *fiber.Ctx) error {
	ipAddress := c.IP()
	userAgent := c.Get("User-Agent")

	input := new(LoginInput)
	if err := c.BodyParser(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid input"})
	}

	// Validate input
	if input.Email == "" || input.Password == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Email and password required"})
	}

	if len(input.Password) < 8 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid credentials"})
	}

	var user domain.User
	// Find user by email with role preload
	if err := database.DB.Preload("Role").Where("email = ?", input.Email).First(&user).Error; err != nil {
		// Log failed login attempt
		// No user resolved, so there is no organisation to attribute this to. The
		// row is recorded unattributed rather than guessed at.
		_ = h.auditService.LogRegister(nil, nil, domain.ResultFailure, ipAddress, userAgent, "User not found")
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid credentials"})
	}

	// Verify password using Argon2id
	if !h.passwordHasher.Verify(user.Password, input.Password) {
		// Log failed login attempt
		_ = h.auditService.LogLogin(auditOrgOf(&user), user.ID, domain.ResultFailure, ipAddress, userAgent, "Invalid password")
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid credentials"})
	}

	// Check if user is active
	if !user.IsActive {
		// Log failed login attempt for inactive user
		_ = h.auditService.LogLogin(auditOrgOf(&user), user.ID, domain.ResultFailure, ipAddress, userAgent, "User account is inactive")
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "User account is inactive"})
	}

	// Generate JWT token
	token, err := h.authService.GenerateToken(&user)
	if err != nil {
		// Log failed login attempt
		_ = h.auditService.LogLogin(auditOrgOf(&user), user.ID, domain.ResultFailure, ipAddress, userAgent, "Failed to generate token")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to generate token"})
	}

	// Update last login timestamp
	_ = h.authService.UpdateLastLogin(user.ID)

	// Log successful login
	_ = h.auditService.LogLogin(auditOrgOf(&user), user.ID, domain.ResultSuccess, ipAddress, userAgent, "")

	return c.Status(fiber.StatusOK).JSON(AuthResponse{
		Token: token,
		User: &UserDTO{
			ID:       user.ID.String(),
			Email:    user.Email,
			Username: user.Username,
			FullName: user.FullName,
			Role:     user.Role.Name,
		},
		ExpiresIn: 24 * 60 * 60,
	})
}

// RefreshToken generates a new JWT token for authenticated user
func (h *AuthHandler) RefreshToken(c *fiber.Ctx) error {
	ipAddress := c.IP()
	userAgent := c.Get("User-Agent")

	// Get user claims from context (set by auth middleware)
	claims := middleware.GetUserClaims(c)
	if claims == nil {
		_ = h.auditService.LogTokenRefresh(nil, uuid.Nil, domain.ResultFailure, ipAddress, userAgent, "Unauthorized")
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}

	// Fetch user from database
	var user domain.User
	if err := database.DB.Preload("Role").First(&user, "id = ?", claims.Sub).Error; err != nil {
		_ = h.auditService.LogTokenRefresh(nil, claims.Sub, domain.ResultFailure, ipAddress, userAgent, "User not found")
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "User not found"})
	}

	// Check if user is still active
	if !user.IsActive {
		_ = h.auditService.LogTokenRefresh(auditOrgOf(&user), user.ID, domain.ResultFailure, ipAddress, userAgent, "User account is inactive")
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "User account is inactive"})
	}

	// Generate new token
	newToken, err := h.authService.GenerateToken(&user)
	if err != nil {
		_ = h.auditService.LogTokenRefresh(auditOrgOf(&user), user.ID, domain.ResultFailure, ipAddress, userAgent, "Failed to generate token")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to generate token"})
	}

	// Log successful token refresh
	_ = h.auditService.LogTokenRefresh(auditOrgOf(&user), user.ID, domain.ResultSuccess, ipAddress, userAgent, "")

	return c.Status(fiber.StatusOK).JSON(AuthResponse{
		Token: newToken,
		User: &UserDTO{
			ID:       user.ID.String(),
			Email:    user.Email,
			Username: user.Username,
			FullName: user.FullName,
			Role:     user.Role.Name,
		},
		ExpiresIn: 24 * 60 * 60,
	})
}

// GetProfile returns current user's profile
func (h *AuthHandler) GetProfile(c *fiber.Ctx) error {
	// Get user claims from context
	claims := middleware.GetUserClaims(c)
	if claims == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}

	// Fetch user from database
	var user domain.User
	if err := database.DB.Preload("Role").First(&user, "id = ?", claims.Sub).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "User not found"})
	}

	return c.Status(fiber.StatusOK).JSON(UserDTO{
		ID:       user.ID.String(),
		Email:    user.Email,
		Username: user.Username,
		FullName: user.FullName,
		Role:     user.Role.Name,
	})
}

// Register creates a new user account
func (h *AuthHandler) Register(c *fiber.Ctx) error {
	ipAddress := c.IP()
	userAgent := c.Get("User-Agent")

	input := new(RegisterInput)
	if err := c.BodyParser(input); err != nil {
		_ = h.auditService.LogRegister(nil, nil, domain.ResultFailure, ipAddress, userAgent, "Invalid input")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid input"})
	}

	// Validate input
	if input.Email == "" || input.Password == "" || input.Username == "" || input.FullName == "" {
		_ = h.auditService.LogRegister(nil, nil, domain.ResultFailure, ipAddress, userAgent, "Missing required fields")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "All fields are required"})
	}

	// Shared policy (audit finding F-05) — see domain.ValidatePassword.
	if err := domain.ValidatePassword(input.Password); err != nil {
		_ = h.auditService.LogRegister(nil, nil, domain.ResultFailure, ipAddress, userAgent, "Password does not meet policy")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if len(input.Username) < 3 {
		_ = h.auditService.LogRegister(nil, nil, domain.ResultFailure, ipAddress, userAgent, "Username too short")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Username must be at least 3 characters"})
	}

	// Check if email already exists
	var existingUser domain.User
	if err := database.DB.Where("email = ?", input.Email).First(&existingUser).Error; err == nil {
		_ = h.auditService.LogRegister(nil, nil, domain.ResultFailure, ipAddress, userAgent, "Email already in use")
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "Email already in use"})
	}

	// Check if username already exists
	if err := database.DB.Where("username = ?", input.Username).First(&existingUser).Error; err == nil {
		_ = h.auditService.LogRegister(nil, nil, domain.ResultFailure, ipAddress, userAgent, "Username already in use")
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "Username already in use"})
	}

	// Hash password using Argon2id (OWASP recommended)
	hashedPassword, err := h.passwordHasher.Hash(input.Password)
	if err != nil {
		_ = h.auditService.LogRegister(nil, nil, domain.ResultFailure, ipAddress, userAgent, "Failed to process password")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to process password"})
	}

	// Get viewer role (default role for new users)
	var viewerRole domain.Role
	if err := database.DB.Where("name = ?", "viewer").First(&viewerRole).Error; err != nil {
		_ = h.auditService.LogRegister(nil, nil, domain.ResultFailure, ipAddress, userAgent, "Default role not found")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Default role not found"})
	}

	// Create new user
	newUser := domain.User{
		Email:    input.Email,
		Username: input.Username,
		FullName: input.FullName,
		Password: hashedPassword,
		RoleID:   viewerRole.ID,
		IsActive: true,
	}

	if err := database.DB.Create(&newUser).Error; err != nil {
		_ = h.auditService.LogRegister(nil, nil, domain.ResultFailure, ipAddress, userAgent, "Failed to create user")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create user"})
	}

	// Reload with role
	if err := database.DB.Preload("Role").First(&newUser, "id = ?", newUser.ID).Error; err != nil {
		_ = h.auditService.LogRegister(auditOrgOf(&newUser), &newUser.ID, domain.ResultFailure, ipAddress, userAgent, "Failed to retrieve user")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve user"})
	}

	// Generate JWT token
	token, err := h.authService.GenerateToken(&newUser)
	if err != nil {
		_ = h.auditService.LogRegister(auditOrgOf(&newUser), &newUser.ID, domain.ResultFailure, ipAddress, userAgent, "Failed to generate token")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to generate token"})
	}

	// Log successful registration
	_ = h.auditService.LogRegister(auditOrgOf(&newUser), &newUser.ID, domain.ResultSuccess, ipAddress, userAgent, "")

	return c.Status(fiber.StatusCreated).JSON(AuthResponse{
		Token: token,
		User: &UserDTO{
			ID:       newUser.ID.String(),
			Email:    newUser.Email,
			Username: newUser.Username,
			FullName: newUser.FullName,
			Role:     newUser.Role.Name,
		},
		ExpiresIn: 24 * 60 * 60,
	})
}

// auditOrgOf resolves the organisation an authentication event belongs to (#532).
//
// It reads users.default_org_id — an organizations.id, which is the id space the
// `tenant_id` JWT claim carries and therefore the only one a later read can
// match. It deliberately does NOT read users.tenant_id: that column points at
// the separate `tenants` table (domain.Tenant, RBAC), and a row stamped with one
// of those ids would be attributed to something no session is ever scoped to —
// the event would vanish rather than leak, which is a quieter bug than the one
// being fixed and no less wrong.
//
// nil means the event could not be attributed: no user, or a user with no
// default organisation. A NULL tenant_id is invisible to every tenant, so
// failing to resolve one loses the event from the UI rather than showing it to
// everybody. That is the direction to fail in.
func auditOrgOf(user *domain.User) *uuid.UUID {
	if user == nil || user.DefaultOrgID == nil || *user.DefaultOrgID == uuid.Nil {
		return nil
	}
	id := *user.DefaultOrgID
	return &id
}
