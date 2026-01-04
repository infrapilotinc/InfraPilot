package api

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// EnrollAgentRequest is the request body for agent enrollment
type EnrollAgentRequest struct {
	EnrollmentToken string            `json:"enrollment_token" binding:"required"`
	Hostname        string            `json:"hostname" binding:"required"`
	Version         string            `json:"version,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
}

// EnrollAgentResponse is the response for successful enrollment
type EnrollAgentResponse struct {
	AgentID     string `json:"agent_id"`
	AgentName   string `json:"agent_name"`
	OrgID       string `json:"org_id"`
	Fingerprint string `json:"fingerprint"`
	Endpoint    string `json:"endpoint,omitempty"`
	// Future: mTLS certificate for agent-to-backend communication
	// Certificate string `json:"certificate,omitempty"`
	// PrivateKey  string `json:"private_key,omitempty"`
}

// EnrollAgent handles the public agent enrollment endpoint
// POST /api/v1/agents/enroll
// This is called by agents when they run the one-liner install script
func (h *Handler) EnrollAgent(c *gin.Context) {
	var req EnrollAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Start transaction
	tx, err := h.db.Begin(c.Request.Context())
	if err != nil {
		h.logger.Error("Failed to start transaction", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}
	defer tx.Rollback(c.Request.Context())

	// Find and validate enrollment token
	var tokenID, orgID uuid.UUID
	var maxUses, useCount sql.NullInt32
	var expiresAt sql.NullTime
	var enabled bool
	var tokenLabels map[string]interface{}

	err = tx.QueryRow(c.Request.Context(), `
		SELECT id, org_id, expires_at, max_uses, use_count, enabled, COALESCE(labels, '{}')
		FROM enrollment_tokens
		WHERE token = $1
	`, req.EnrollmentToken).Scan(&tokenID, &orgID, &expiresAt, &maxUses, &useCount, &enabled, &tokenLabels)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid enrollment token"})
		return
	}
	if err != nil {
		h.logger.Error("Failed to find enrollment token", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	// Validate token
	if !enabled {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Enrollment token is disabled"})
		return
	}

	if expiresAt.Valid && time.Now().After(expiresAt.Time) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Enrollment token has expired"})
		return
	}

	if maxUses.Valid && useCount.Int32 >= maxUses.Int32 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Enrollment token has reached maximum uses"})
		return
	}

	// Check if agent with this hostname already exists for this org
	var existingAgentID uuid.UUID
	err = tx.QueryRow(c.Request.Context(), `
		SELECT id FROM agents WHERE org_id = $1 AND hostname = $2
	`, orgID, req.Hostname).Scan(&existingAgentID)

	if err != nil && err != sql.ErrNoRows {
		h.logger.Error("Failed to check existing agent", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	if err == nil {
		// Agent already exists - update and return existing
		fingerprint := generateFingerprint()
		_, err = tx.Exec(c.Request.Context(), `
			UPDATE agents SET
				fingerprint = $1,
				version = $2,
				status = 'active',
				last_seen_at = NOW(),
				updated_at = NOW()
			WHERE id = $3
		`, fingerprint, req.Version, existingAgentID)

		if err != nil {
			h.logger.Error("Failed to update existing agent", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
			return
		}

		// Update token usage
		_, err = tx.Exec(c.Request.Context(), `
			UPDATE enrollment_tokens SET use_count = use_count + 1, last_used_at = NOW()
			WHERE id = $1
		`, tokenID)
		if err != nil {
			h.logger.Error("Failed to update token usage", zap.Error(err))
		}

		if err := tx.Commit(c.Request.Context()); err != nil {
			h.logger.Error("Failed to commit transaction", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
			return
		}

		c.JSON(http.StatusOK, EnrollAgentResponse{
			AgentID:     existingAgentID.String(),
			AgentName:   req.Hostname,
			OrgID:       orgID.String(),
			Fingerprint: fingerprint,
		})
		return
	}

	// Check org agent limit
	var agentCount, maxAgents int
	err = tx.QueryRow(c.Request.Context(), `
		SELECT
			(SELECT COUNT(*) FROM agents WHERE org_id = $1),
			(SELECT max_agents FROM organizations WHERE id = $1)
	`, orgID).Scan(&agentCount, &maxAgents)

	if err != nil {
		h.logger.Error("Failed to check agent limits", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	if agentCount >= maxAgents {
		c.JSON(http.StatusForbidden, gin.H{
			"error":      "Organization has reached maximum agent limit",
			"max_agents": maxAgents,
			"current":    agentCount,
		})
		return
	}

	// Generate fingerprint for new agent
	fingerprint := generateFingerprint()

	// Create new agent
	var agentID uuid.UUID
	err = tx.QueryRow(c.Request.Context(), `
		INSERT INTO agents (org_id, name, hostname, fingerprint, version, status, last_seen_at)
		VALUES ($1, $2, $3, $4, $5, 'active', NOW())
		RETURNING id
	`, orgID, req.Hostname, req.Hostname, fingerprint, req.Version).Scan(&agentID)

	if err != nil {
		h.logger.Error("Failed to create agent", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	// Update token usage
	_, err = tx.Exec(c.Request.Context(), `
		UPDATE enrollment_tokens SET use_count = use_count + 1, last_used_at = NOW()
		WHERE id = $1
	`, tokenID)

	if err != nil {
		h.logger.Error("Failed to update token usage", zap.Error(err))
		// Non-fatal, continue
	}

	// Record audit log
	_, err = tx.Exec(c.Request.Context(), `
		INSERT INTO audit_logs (org_id, agent_id, action, resource_type, resource_id, ip_address)
		VALUES ($1, $2, 'agent.enrolled', 'agent', $2, $3)
	`, orgID, agentID, c.ClientIP())

	if err != nil {
		h.logger.Error("Failed to record audit log", zap.Error(err))
		// Non-fatal, continue
	}

	if err := tx.Commit(c.Request.Context()); err != nil {
		h.logger.Error("Failed to commit transaction", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	h.logger.Info("Agent enrolled successfully",
		zap.String("agent_id", agentID.String()),
		zap.String("org_id", orgID.String()),
		zap.String("hostname", req.Hostname),
	)

	c.JSON(http.StatusCreated, EnrollAgentResponse{
		AgentID:     agentID.String(),
		AgentName:   req.Hostname,
		OrgID:       orgID.String(),
		Fingerprint: fingerprint,
	})
}

// generateFingerprint creates a unique fingerprint for an agent
func generateFingerprint() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// GetEnrollmentStatus checks if an agent is enrolled
// GET /api/v1/agents/enroll/status?fingerprint=xxx
func (h *Handler) GetEnrollmentStatus(c *gin.Context) {
	fingerprint := c.Query("fingerprint")
	if fingerprint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "fingerprint query parameter required"})
		return
	}

	var agentID uuid.UUID
	var status string
	var lastSeenAt sql.NullTime

	err := h.db.QueryRow(c.Request.Context(), `
		SELECT id, status, last_seen_at FROM agents WHERE fingerprint = $1
	`, fingerprint).Scan(&agentID, &status, &lastSeenAt)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"enrolled": false})
		return
	}
	if err != nil {
		h.logger.Error("Failed to check enrollment status", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"enrolled":     true,
		"agent_id":     agentID.String(),
		"status":       status,
		"last_seen_at": lastSeenAt.Time,
	})
}

// AgentHeartbeat updates the agent's last seen timestamp
// POST /api/v1/agents/heartbeat
func (h *Handler) AgentHeartbeat(c *gin.Context) {
	var req struct {
		Fingerprint string `json:"fingerprint" binding:"required"`
		Version     string `json:"version,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := h.db.Exec(c.Request.Context(), `
		UPDATE agents SET
			last_seen_at = NOW(),
			status = 'active',
			version = COALESCE($2, version),
			updated_at = NOW()
		WHERE fingerprint = $1
	`, req.Fingerprint, req.Version)

	if err != nil {
		h.logger.Error("Failed to update heartbeat", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	if result.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Agent not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
