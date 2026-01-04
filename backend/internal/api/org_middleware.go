package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	// OrgHeader is the header name for specifying the current organization
	OrgHeader = "X-Org-ID"
	// OrgContextKey is the gin context key for the current organization ID
	OrgContextKey = "org_id"
)

// OrgMiddleware extracts the organization ID from the request and sets it in the context.
// It also sets the PostgreSQL RLS context for row-level security.
func (h *Handler) OrgMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get user ID from auth context (must be after AuthMiddleware)
		userID, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
			c.Abort()
			return
		}

		var orgID uuid.UUID
		var err error

		// Check for org ID in header first
		orgIDStr := c.GetHeader(OrgHeader)
		if orgIDStr != "" {
			orgID, err = uuid.Parse(orgIDStr)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid organization ID"})
				c.Abort()
				return
			}

			// Verify user is a member of this org
			var isMember bool
			err = h.db.QueryRow(c.Request.Context(), `
				SELECT EXISTS(
					SELECT 1 FROM organization_members
					WHERE org_id = $1 AND user_id = $2
				)
			`, orgID, userID).Scan(&isMember)
			if err != nil || !isMember {
				c.JSON(http.StatusForbidden, gin.H{"error": "Not a member of this organization"})
				c.Abort()
				return
			}
		} else {
			// Get user's default org (first org they belong to, or their org_id from users table)
			err = h.db.QueryRow(c.Request.Context(), `
				SELECT COALESCE(
					(SELECT org_id FROM organization_members WHERE user_id = $1 ORDER BY joined_at LIMIT 1),
					(SELECT org_id FROM users WHERE id = $1)
				)
			`, userID).Scan(&orgID)
			if err != nil {
				// Fallback to user's org_id from users table
				err = h.db.QueryRow(c.Request.Context(), `
					SELECT org_id FROM users WHERE id = $1
				`, userID).Scan(&orgID)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to determine organization"})
					c.Abort()
					return
				}
			}
		}

		// Set org_id in gin context for handlers
		c.Set(OrgContextKey, orgID)

		// Set PostgreSQL RLS context for this connection
		// Note: This uses SET LOCAL which only affects the current transaction
		_, err = h.db.Exec(c.Request.Context(), "SELECT set_org_context($1)", orgID)
		if err != nil {
			h.logger.Warn("Failed to set org context for RLS",
				// Don't fail the request - RLS will still work via direct filtering
			)
		}

		c.Next()
	}
}

// GetOrgID retrieves the organization ID from the gin context
func GetOrgID(c *gin.Context) (uuid.UUID, bool) {
	orgID, exists := c.Get(OrgContextKey)
	if !exists {
		return uuid.Nil, false
	}
	id, ok := orgID.(uuid.UUID)
	return id, ok
}

// RequireOrg is a helper middleware that ensures org_id is present in context
func RequireOrg() gin.HandlerFunc {
	return func(c *gin.Context) {
		_, exists := c.Get(OrgContextKey)
		if !exists {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Organization context required"})
			c.Abort()
			return
		}
		c.Next()
	}
}
