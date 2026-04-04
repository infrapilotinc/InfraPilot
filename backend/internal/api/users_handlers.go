package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

// User represents a user account
type User struct {
	ID         uuid.UUID  `json:"id"`
	OrgID      uuid.UUID  `json:"org_id"`
	Email      string     `json:"email"`
	Role       string     `json:"role"`
	MFAEnabled bool       `json:"mfa_enabled"`
	LastLogin  *time.Time `json:"last_login,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// CreateUserRequest represents a request to create a user
type CreateUserRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
	Role     string `json:"role" binding:"required"`
}

// UpdateUserRequest represents a request to update a user
type UpdateUserRequest struct {
	Email    *string `json:"email,omitempty"`
	Password *string `json:"password,omitempty"`
	Role     *string `json:"role,omitempty"`
}

// listUsersReal returns all users in the organization
func (h *Handler) listUsersReal(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)

	rows, err := h.db.Query(c.Request.Context(), `
		SELECT id, org_id, email, role, mfa_enabled, last_login_at, created_at, updated_at
		FROM users
		WHERE org_id = $1
		ORDER BY created_at DESC
	`, orgID)
	if err != nil {
		h.logger.Error("Failed to list users", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list users"})
		return
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var user User
		if err := rows.Scan(
			&user.ID, &user.OrgID, &user.Email, &user.Role,
			&user.MFAEnabled, &user.LastLogin, &user.CreatedAt, &user.UpdatedAt,
		); err != nil {
			continue
		}
		users = append(users, user)
	}

	c.JSON(http.StatusOK, users)
}

// createUserReal creates a new user
func (h *Handler) createUserReal(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)

	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate role
	validRoles := map[string]bool{
		"super_admin": true,
		"admin":       true,
		"operator":    true,
		"viewer":      true,
	}
	if !validRoles[req.Role] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid role"})
		return
	}

	// Check if email already exists
	var exists bool
	h.db.QueryRow(c.Request.Context(), `
		SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)
	`, req.Email).Scan(&exists)
	if exists {
		c.JSON(http.StatusConflict, gin.H{"error": "email already exists"})
		return
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		h.logger.Error("Failed to hash password", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create user"})
		return
	}

	var user User
	err = h.db.QueryRow(c.Request.Context(), `
		INSERT INTO users (org_id, email, password_hash, role)
		VALUES ($1, $2, $3, $4)
		RETURNING id, org_id, email, role, mfa_enabled, created_at, updated_at
	`, orgID, req.Email, string(hashedPassword), req.Role).Scan(
		&user.ID, &user.OrgID, &user.Email, &user.Role,
		&user.MFAEnabled, &user.CreatedAt, &user.UpdatedAt,
	)

	if err != nil {
		h.logger.Error("Failed to create user", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create user"})
		return
	}

	c.JSON(http.StatusCreated, user)
}

// updateUserReal updates a user
func (h *Handler) updateUserReal(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	userIDStr := c.Param("id")

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	var req UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get existing user
	var user User
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT id, org_id, email, role, mfa_enabled, created_at, updated_at
		FROM users
		WHERE id = $1 AND org_id = $2
	`, userID, orgID).Scan(
		&user.ID, &user.OrgID, &user.Email, &user.Role,
		&user.MFAEnabled, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	// Apply updates
	if req.Email != nil {
		user.Email = *req.Email
	}
	if req.Role != nil {
		validRoles := map[string]bool{
			"super_admin": true,
			"admin":       true,
			"operator":    true,
			"viewer":      true,
		}
		if !validRoles[*req.Role] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid role"})
			return
		}
		user.Role = *req.Role
	}

	// Update password if provided
	if req.Password != nil && *req.Password != "" {
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(*req.Password), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
			return
		}
		_, err = h.db.Exec(c.Request.Context(), `
			UPDATE users SET password_hash = $1, updated_at = NOW() WHERE id = $2
		`, string(hashedPassword), userID)
		if err != nil {
			h.logger.Error("Failed to update password", zap.Error(err))
		}
	}

	// Update user
	err = h.db.QueryRow(c.Request.Context(), `
		UPDATE users
		SET email = $1, role = $2, updated_at = NOW()
		WHERE id = $3 AND org_id = $4
		RETURNING updated_at
	`, user.Email, user.Role, userID, orgID).Scan(&user.UpdatedAt)

	if err != nil {
		h.logger.Error("Failed to update user", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update user"})
		return
	}

	c.JSON(http.StatusOK, user)
}

// deleteUserReal deletes a user
func (h *Handler) deleteUserReal(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	currentUserID := c.MustGet("user_id").(uuid.UUID)
	userIDStr := c.Param("id")

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	// Prevent self-deletion
	if userID == currentUserID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete your own account"})
		return
	}

	// Get user email for audit log
	var email string
	h.db.QueryRow(c.Request.Context(), `
		SELECT email FROM users WHERE id = $1 AND org_id = $2
	`, userID, orgID).Scan(&email)

	result, err := h.db.Exec(c.Request.Context(), `
		DELETE FROM users
		WHERE id = $1 AND org_id = $2
	`, userID, orgID)

	if err != nil {
		h.logger.Error("Failed to delete user", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete user"})
		return
	}

	if result.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "user deleted"})
}
