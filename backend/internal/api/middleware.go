package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/infrapilot/backend/internal/auth"
	"github.com/infrapilot/backend/internal/license"
)

// LoggerMiddleware logs HTTP requests
func LoggerMiddleware(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		logger.Info("HTTP request",
			zap.Int("status", status),
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.String("query", query),
			zap.Duration("latency", latency),
			zap.String("ip", c.ClientIP()),
			zap.String("user-agent", c.Request.UserAgent()),
		)
	}
}

// CORSMiddleware handles CORS headers
func CORSMiddleware(allowedOrigins []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		// Check if origin is allowed
		allowed := false
		for _, o := range allowedOrigins {
			if o == "*" || o == origin {
				allowed = true
				break
			}
		}

		if allowed {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Max-Age", "86400")
		}

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// AuthMiddleware validates JWT tokens
func (h *Handler) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Allow internal service calls (from agent)
		if c.GetHeader("X-Internal-Service") == "agent" {
			// Set a service identity for internal calls
			c.Set("internal_service", true)
			// Parse org_id as UUID to match handler expectations
			orgID, _ := uuid.Parse("00000000-0000-0000-0000-000000000001")
			c.Set("org_id", orgID) // Default org
			c.Set("role", "service")
			c.Next()
			return
		}

		var token string

		// Check Authorization header first
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && parts[0] == "Bearer" {
				token = parts[1]
			}
		}

		// Fallback to query parameter for WebSocket connections
		if token == "" {
			token = c.Query("token")
		}

		if token == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authorization required"})
			c.Abort()
			return
		}

		claims, err := h.auth.ValidateToken(token)
		if err != nil {
			status := http.StatusUnauthorized
			message := "invalid token"
			if err == auth.ErrTokenExpired {
				message = "token expired"
			}
			c.JSON(status, gin.H{"error": message})
			c.Abort()
			return
		}

		// Store claims in context
		c.Set("claims", claims)
		c.Set("user_id", claims.UserID)
		c.Set("org_id", claims.OrgID)
		c.Set("role", claims.Role)

		c.Next()
	}
}

// RequireRole creates a middleware that requires a specific role
func (h *Handler) RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			c.Abort()
			return
		}

		userRole := role.(string)
		allowed := false
		for _, r := range roles {
			if userRole == r {
				allowed = true
				break
			}
		}

		if !allowed {
			c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequireModifyContainers checks if user can modify containers
func (h *Handler) RequireModifyContainers() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			c.Abort()
			return
		}

		if !auth.CanModifyContainers(role.(string)) {
			c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequireModifyProxy checks if user can modify proxy hosts
func (h *Handler) RequireModifyProxy() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			c.Abort()
			return
		}

		if !auth.CanModifyProxies(role.(string)) {
			c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequireManageAlerts checks if user can manage alerts
func (h *Handler) RequireManageAlerts() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			c.Abort()
			return
		}

		if !auth.CanManageAlerts(role.(string)) {
			c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequireFeature blocks a route if the installed license does not include the
// given feature. It returns a 403 with an upgrade URL in the response body.
func (h *Handler) RequireFeature(feature string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !h.license.HasFeature(feature) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "This feature is not available on your current plan",
				"feature":     feature,
				"tier":        h.license.Tier(),
				"upgrade_url": "https://infrapilot.org/billing",
			})
			return
		}
		c.Next()
	}
}

// LicenseInfo returns the current license state as a JSON endpoint.
// Useful for the frontend to show upgrade prompts.
func (h *Handler) LicenseInfo(c *gin.Context) {
	resp, err := h.license.Validate()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	// Strip issued_to for non-admin callers — exposed info is fine here since
	// it is already stored server-side and shown in the UI.
	type licenseInfoResponse struct {
		Valid      bool                `json:"valid"`
		Tier       string              `json:"tier"`
		MaxAgents  int                 `json:"max_agents"`
		Features   []string            `json:"features"`
		ExpiresAt  *string             `json:"expires_at"`
		UpgradeURL string              `json:"upgrade_url"`
	}
	features := resp.Features
	if features == nil {
		features = []string{}
	}
	c.JSON(http.StatusOK, licenseInfoResponse{
		Valid:      resp.Valid,
		Tier:       resp.Tier,
		MaxAgents:  resp.MaxAgents,
		Features:   features,
		ExpiresAt:  resp.ExpiresAt,
		UpgradeURL: "https://infrapilot.org/billing",
	})
}

// Ensure license import is used (compile guard).
var _ = license.FeatureCoreMonitoring
