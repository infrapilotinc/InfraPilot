package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// SetupStatusResponse represents the setup status
type SetupStatusResponse struct {
	SetupRequired bool `json:"setup_required"`
	UserCount     int  `json:"user_count"`
}

// SetupRequest represents the initial admin setup request
type SetupRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
}

// getSetupStatus checks if initial setup is required (no users exist)
func (h *Handler) getSetupStatus(c *gin.Context) {
	var count int
	err := h.db.QueryRow(c.Request.Context(), `SELECT COUNT(*) FROM users`).Scan(&count)
	if err != nil {
		h.logger.Error("Failed to count users")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check setup status"})
		return
	}

	c.JSON(http.StatusOK, SetupStatusResponse{
		SetupRequired: count == 0,
		UserCount:     count,
	})
}

// createInitialAdmin creates the first admin user during setup
func (h *Handler) createInitialAdmin(c *gin.Context) {
	// First check if any users exist
	var count int
	err := h.db.QueryRow(c.Request.Context(), `SELECT COUNT(*) FROM users`).Scan(&count)
	if err != nil {
		h.logger.Error("Failed to count users")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check setup status"})
		return
	}

	if count > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "setup already completed"})
		return
	}

	var req SetupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate password strength
	if len(req.Password) < 8 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password must be at least 8 characters"})
		return
	}

	// Hash password
	passwordHash, err := h.auth.HashPassword(req.Password)
	if err != nil {
		h.logger.Error("Failed to hash password")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create account"})
		return
	}

	// Ensure default organization exists
	orgID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	_, err = h.db.Exec(c.Request.Context(), `
		INSERT INTO organizations (id, name, slug)
		VALUES ($1, 'Default Organization', 'default')
		ON CONFLICT (id) DO NOTHING
	`, orgID)
	if err != nil {
		h.logger.Error("Failed to create organization")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create organization"})
		return
	}

	// Create admin user
	userID := uuid.New()
	_, err = h.db.Exec(c.Request.Context(), `
		INSERT INTO users (id, org_id, email, password_hash, role, mfa_enabled)
		VALUES ($1, $2, $3, $4, 'super_admin', false)
	`, userID, orgID, req.Email, passwordHash)

	if err != nil {
		h.logger.Error("Failed to create admin user")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create account"})
		return
	}

	// Audit log for setup
	h.db.Exec(c.Request.Context(), `
		INSERT INTO audit_logs (org_id, user_id, action, resource_type, resource_id, ip_address, user_agent)
		VALUES ($1, $2, 'setup.initial_admin_created', 'user', $2, $3, $4)
	`, orgID, userID, c.ClientIP(), c.Request.UserAgent())

	// Generate tokens for immediate login
	accessToken, err := h.auth.GenerateAccessToken(userID, orgID, req.Email, "super_admin", false)
	if err != nil {
		// User created but tokens failed - they can still login
		c.JSON(http.StatusOK, gin.H{
			"message": "Admin account created successfully. Please login.",
			"user_id": userID,
		})
		return
	}

	refreshToken, _ := h.auth.GenerateRefreshToken()

	c.JSON(http.StatusCreated, gin.H{
		"message":       "Admin account created successfully",
		"user_id":       userID,
		"access_token":  accessToken,
		"refresh_token": refreshToken,
	})
}
