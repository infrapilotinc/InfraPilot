package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	agentgrpc "github.com/infrapilot/backend/internal/grpc"
)

// ProxyHost represents an nginx reverse proxy configuration
type ProxyHost struct {
	ID             uuid.UUID  `json:"id"`
	AgentID        uuid.UUID  `json:"agent_id"`
	Domain         string     `json:"domain"`
	UpstreamTarget string     `json:"upstream_target"`
	SSLEnabled     bool       `json:"ssl_enabled"`
	SSLCertPath    *string    `json:"ssl_cert_path,omitempty"`
	SSLKeyPath     *string    `json:"ssl_key_path,omitempty"`
	SSLExpiresAt   *time.Time `json:"ssl_expires_at,omitempty"`
	ForceSSL       bool       `json:"force_ssl"`
	HTTP2Enabled   bool       `json:"http2_enabled"`
	ConfigHash     *string    `json:"config_hash,omitempty"`
	Status         string     `json:"status"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// SecurityHeaders represents nginx security headers configuration
type SecurityHeaders struct {
	ID                    uuid.UUID `json:"id"`
	ProxyHostID           uuid.UUID `json:"proxy_host_id"`
	HSTSEnabled           bool      `json:"hsts_enabled"`
	HSTSMaxAge            int       `json:"hsts_max_age"`
	XFrameOptions         string    `json:"x_frame_options"`
	XContentTypeOptions   bool      `json:"x_content_type_options"`
	XXSSProtection        bool      `json:"x_xss_protection"`
	ContentSecurityPolicy *string   `json:"content_security_policy,omitempty"`
}

// CreateProxyRequest is the request body for creating a proxy host
type CreateProxyRequest struct {
	Domain         string `json:"domain" binding:"required"`
	UpstreamTarget string `json:"upstream_target" binding:"required"`
	ForceSSL       bool   `json:"force_ssl"`
	HTTP2Enabled   bool   `json:"http2_enabled"`
}

// UpdateProxyRequest is the request body for updating a proxy host
type UpdateProxyRequest struct {
	Domain         *string `json:"domain,omitempty"`
	UpstreamTarget *string `json:"upstream_target,omitempty"`
	ForceSSL       *bool   `json:"force_ssl,omitempty"`
	HTTP2Enabled   *bool   `json:"http2_enabled,omitempty"`
	Status         *string `json:"status,omitempty"`
}

// listProxyHosts returns all proxy hosts for an agent
func (h *Handler) listProxyHosts(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	agentIDStr := c.Param("id")

	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	// Verify agent belongs to org
	var exists bool
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT EXISTS(SELECT 1 FROM agents WHERE id = $1 AND org_id = $2)
	`, agentID, orgID).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	rows, err := h.db.Query(c.Request.Context(), `
		SELECT id, agent_id, domain, upstream_target, ssl_enabled, ssl_cert_path,
		       ssl_key_path, ssl_expires_at, force_ssl, http2_enabled, config_hash,
		       status, created_at, updated_at
		FROM proxy_hosts
		WHERE agent_id = $1
		ORDER BY domain ASC
	`, agentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch proxy hosts"})
		return
	}
	defer rows.Close()

	proxies := []ProxyHost{}
	for rows.Next() {
		var p ProxyHost
		if err := rows.Scan(
			&p.ID, &p.AgentID, &p.Domain, &p.UpstreamTarget, &p.SSLEnabled,
			&p.SSLCertPath, &p.SSLKeyPath, &p.SSLExpiresAt, &p.ForceSSL,
			&p.HTTP2Enabled, &p.ConfigHash, &p.Status, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			continue
		}
		proxies = append(proxies, p)
	}

	c.JSON(http.StatusOK, proxies)
}

// createProxyHost creates a new proxy host for an agent
func (h *Handler) createProxyHost(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	userID := c.MustGet("user_id").(uuid.UUID)
	agentIDStr := c.Param("id")

	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	// Verify agent belongs to org
	var exists bool
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT EXISTS(SELECT 1 FROM agents WHERE id = $1 AND org_id = $2)
	`, agentID, orgID).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	var req CreateProxyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if domain already exists for this agent
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT EXISTS(SELECT 1 FROM proxy_hosts WHERE agent_id = $1 AND domain = $2)
	`, agentID, req.Domain).Scan(&exists)

	if exists {
		c.JSON(http.StatusConflict, gin.H{"error": "domain already exists for this agent"})
		return
	}

	// Insert proxy host
	var proxyID uuid.UUID
	err = h.db.QueryRow(c.Request.Context(), `
		INSERT INTO proxy_hosts (agent_id, domain, upstream_target, force_ssl, http2_enabled, status)
		VALUES ($1, $2, $3, $4, $5, 'active')
		RETURNING id
	`, agentID, req.Domain, req.UpstreamTarget, req.ForceSSL, req.HTTP2Enabled).Scan(&proxyID)

	if err != nil {
		h.logger.Error("Failed to create proxy host")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create proxy host"})
		return
	}

	// Create default security headers
	_, err = h.db.Exec(c.Request.Context(), `
		INSERT INTO proxy_security_headers (proxy_host_id)
		VALUES ($1)
	`, proxyID)

	if err != nil {
		h.logger.Error("Failed to create security headers")
	}

	// Audit log
	h.auditLog(c, userID, orgID, "proxy.create", "proxy_host", proxyID, req)

	// Push config to agent via gRPC
	go h.dispatchProxyConfig(c.Request.Context(), agentID, proxyID, req.Domain, req.UpstreamTarget, req.ForceSSL, req.HTTP2Enabled, false)

	// Fetch and return the created proxy
	var proxy ProxyHost
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT id, agent_id, domain, upstream_target, ssl_enabled, ssl_cert_path,
		       ssl_key_path, ssl_expires_at, force_ssl, http2_enabled, config_hash,
		       status, created_at, updated_at
		FROM proxy_hosts
		WHERE id = $1
	`, proxyID).Scan(
		&proxy.ID, &proxy.AgentID, &proxy.Domain, &proxy.UpstreamTarget, &proxy.SSLEnabled,
		&proxy.SSLCertPath, &proxy.SSLKeyPath, &proxy.SSLExpiresAt, &proxy.ForceSSL,
		&proxy.HTTP2Enabled, &proxy.ConfigHash, &proxy.Status, &proxy.CreatedAt, &proxy.UpdatedAt,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch created proxy"})
		return
	}

	c.JSON(http.StatusCreated, proxy)
}

// getProxyHost returns a single proxy host
func (h *Handler) getProxyHost(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	agentIDStr := c.Param("id")
	proxyIDStr := c.Param("pid")

	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	proxyID, err := uuid.Parse(proxyIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid proxy ID"})
		return
	}

	// Verify agent belongs to org
	var exists bool
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT EXISTS(SELECT 1 FROM agents WHERE id = $1 AND org_id = $2)
	`, agentID, orgID).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	var proxy ProxyHost
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT id, agent_id, domain, upstream_target, ssl_enabled, ssl_cert_path,
		       ssl_key_path, ssl_expires_at, force_ssl, http2_enabled, config_hash,
		       status, created_at, updated_at
		FROM proxy_hosts
		WHERE id = $1 AND agent_id = $2
	`, proxyID, agentID).Scan(
		&proxy.ID, &proxy.AgentID, &proxy.Domain, &proxy.UpstreamTarget, &proxy.SSLEnabled,
		&proxy.SSLCertPath, &proxy.SSLKeyPath, &proxy.SSLExpiresAt, &proxy.ForceSSL,
		&proxy.HTTP2Enabled, &proxy.ConfigHash, &proxy.Status, &proxy.CreatedAt, &proxy.UpdatedAt,
	)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "proxy host not found"})
		return
	}

	c.JSON(http.StatusOK, proxy)
}

// updateProxyHost updates a proxy host
func (h *Handler) updateProxyHost(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	userID := c.MustGet("user_id").(uuid.UUID)
	agentIDStr := c.Param("id")
	proxyIDStr := c.Param("pid")

	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	proxyID, err := uuid.Parse(proxyIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid proxy ID"})
		return
	}

	// Verify agent belongs to org
	var exists bool
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT EXISTS(SELECT 1 FROM agents WHERE id = $1 AND org_id = $2)
	`, agentID, orgID).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	var req UpdateProxyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Build dynamic update query
	query := "UPDATE proxy_hosts SET updated_at = NOW()"
	args := []interface{}{}
	argCount := 0

	if req.Domain != nil {
		argCount++
		query += fmt.Sprintf(", domain = $%d", argCount)
		args = append(args, *req.Domain)
	}
	if req.UpstreamTarget != nil {
		argCount++
		query += fmt.Sprintf(", upstream_target = $%d", argCount)
		args = append(args, *req.UpstreamTarget)
	}
	if req.ForceSSL != nil {
		argCount++
		query += fmt.Sprintf(", force_ssl = $%d", argCount)
		args = append(args, *req.ForceSSL)
	}
	if req.HTTP2Enabled != nil {
		argCount++
		query += fmt.Sprintf(", http2_enabled = $%d", argCount)
		args = append(args, *req.HTTP2Enabled)
	}
	if req.Status != nil {
		argCount++
		query += fmt.Sprintf(", status = $%d", argCount)
		args = append(args, *req.Status)
	}

	argCount++
	query += fmt.Sprintf(" WHERE id = $%d", argCount)
	args = append(args, proxyID)

	argCount++
	query += fmt.Sprintf(" AND agent_id = $%d", argCount)
	args = append(args, agentID)

	result, err := h.db.Exec(c.Request.Context(), query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update proxy host"})
		return
	}

	if result.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "proxy host not found"})
		return
	}

	// Audit log
	h.auditLog(c, userID, orgID, "proxy.update", "proxy_host", proxyID, req)

	// Fetch updated proxy to get current values
	var proxy ProxyHost
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT id, agent_id, domain, upstream_target, ssl_enabled, ssl_cert_path,
		       ssl_key_path, ssl_expires_at, force_ssl, http2_enabled, config_hash,
		       status, created_at, updated_at
		FROM proxy_hosts
		WHERE id = $1
	`, proxyID).Scan(
		&proxy.ID, &proxy.AgentID, &proxy.Domain, &proxy.UpstreamTarget, &proxy.SSLEnabled,
		&proxy.SSLCertPath, &proxy.SSLKeyPath, &proxy.SSLExpiresAt, &proxy.ForceSSL,
		&proxy.HTTP2Enabled, &proxy.ConfigHash, &proxy.Status, &proxy.CreatedAt, &proxy.UpdatedAt,
	)

	// Push updated config to agent via gRPC
	go h.dispatchProxyConfig(c.Request.Context(), agentID, proxyID, proxy.Domain, proxy.UpstreamTarget, proxy.ForceSSL, proxy.HTTP2Enabled, proxy.SSLEnabled)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch updated proxy"})
		return
	}

	c.JSON(http.StatusOK, proxy)
}

// deleteProxyHost deletes a proxy host
func (h *Handler) deleteProxyHost(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	userID := c.MustGet("user_id").(uuid.UUID)
	agentIDStr := c.Param("id")
	proxyIDStr := c.Param("pid")

	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	proxyID, err := uuid.Parse(proxyIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid proxy ID"})
		return
	}

	// Verify agent belongs to org
	var exists bool
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT EXISTS(SELECT 1 FROM agents WHERE id = $1 AND org_id = $2)
	`, agentID, orgID).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	// Get proxy domain for audit log before deleting
	var domain string
	h.db.QueryRow(c.Request.Context(), `
		SELECT domain FROM proxy_hosts WHERE id = $1
	`, proxyID).Scan(&domain)

	result, err := h.db.Exec(c.Request.Context(), `
		DELETE FROM proxy_hosts WHERE id = $1 AND agent_id = $2
	`, proxyID, agentID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete proxy host"})
		return
	}

	if result.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "proxy host not found"})
		return
	}

	// Audit log
	h.auditLog(c, userID, orgID, "proxy.delete", "proxy_host", proxyID, gin.H{"domain": domain})

	// Remove config from agent via gRPC
	go h.dispatchDeleteProxy(agentID, domain)

	c.JSON(http.StatusOK, gin.H{"message": "proxy host deleted"})
}

// requestSSL initiates SSL certificate request for a proxy host
func (h *Handler) requestSSL(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	userID := c.MustGet("user_id").(uuid.UUID)
	agentIDStr := c.Param("id")
	proxyIDStr := c.Param("pid")

	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	proxyID, err := uuid.Parse(proxyIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid proxy ID"})
		return
	}

	// Verify agent belongs to org
	var exists bool
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT EXISTS(SELECT 1 FROM agents WHERE id = $1 AND org_id = $2)
	`, agentID, orgID).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	// Get proxy domain
	var domain string
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT domain FROM proxy_hosts WHERE id = $1 AND agent_id = $2
	`, proxyID, agentID).Scan(&domain)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "proxy host not found"})
		return
	}

	// Mark as pending
	_, err = h.db.Exec(c.Request.Context(), `
		UPDATE proxy_hosts SET status = 'ssl_pending' WHERE id = $1
	`, proxyID)

	// Get SSL email from settings or use default
	sslEmail := "admin@" + domain

	// Send SSL request command to agent via gRPC
	go h.dispatchSSLRequest(agentID, domain, sslEmail, "")

	// Audit log
	h.auditLog(c, userID, orgID, "proxy.ssl_request", "proxy_host", proxyID, gin.H{"domain": domain})

	c.JSON(http.StatusAccepted, gin.H{
		"message": "SSL certificate request initiated",
		"domain":  domain,
		"status":  "pending",
	})
}

// getProxyConfig returns the generated nginx config for a proxy host
func (h *Handler) getProxyConfig(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	agentIDStr := c.Param("id")
	proxyIDStr := c.Param("pid")

	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	proxyID, err := uuid.Parse(proxyIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid proxy ID"})
		return
	}

	// Verify agent belongs to org
	var exists bool
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT EXISTS(SELECT 1 FROM agents WHERE id = $1 AND org_id = $2)
	`, agentID, orgID).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	// Get proxy details
	var proxy ProxyHost
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT id, agent_id, domain, upstream_target, ssl_enabled, ssl_cert_path,
		       ssl_key_path, ssl_expires_at, force_ssl, http2_enabled, config_hash,
		       status, created_at, updated_at
		FROM proxy_hosts
		WHERE id = $1 AND agent_id = $2
	`, proxyID, agentID).Scan(
		&proxy.ID, &proxy.AgentID, &proxy.Domain, &proxy.UpstreamTarget, &proxy.SSLEnabled,
		&proxy.SSLCertPath, &proxy.SSLKeyPath, &proxy.SSLExpiresAt, &proxy.ForceSSL,
		&proxy.HTTP2Enabled, &proxy.ConfigHash, &proxy.Status, &proxy.CreatedAt, &proxy.UpdatedAt,
	)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "proxy host not found"})
		return
	}

	// Get security headers
	var headers SecurityHeaders
	h.db.QueryRow(c.Request.Context(), `
		SELECT id, proxy_host_id, hsts_enabled, hsts_max_age, x_frame_options,
		       x_content_type_options, x_xss_protection, content_security_policy
		FROM proxy_security_headers
		WHERE proxy_host_id = $1
	`, proxyID).Scan(
		&headers.ID, &headers.ProxyHostID, &headers.HSTSEnabled, &headers.HSTSMaxAge,
		&headers.XFrameOptions, &headers.XContentTypeOptions, &headers.XXSSProtection,
		&headers.ContentSecurityPolicy,
	)

	// Generate nginx config
	config := generateNginxConfig(proxy, headers)

	c.JSON(http.StatusOK, gin.H{
		"domain": proxy.Domain,
		"config": config,
	})
}

// testProxyConfig tests if the nginx config is valid
func (h *Handler) testProxyConfig(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	agentIDStr := c.Param("id")
	proxyIDStr := c.Param("pid")

	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	proxyID, err := uuid.Parse(proxyIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid proxy ID"})
		return
	}

	// Verify agent belongs to org and proxy exists
	var exists bool
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT EXISTS(
			SELECT 1 FROM proxy_hosts p
			JOIN agents a ON p.agent_id = a.id
			WHERE p.id = $1 AND p.agent_id = $2 AND a.org_id = $3
		)
	`, proxyID, agentID, orgID).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "proxy host not found"})
		return
	}

	// Send test command to agent via gRPC
	if !agentgrpc.IsAgentConnected(agentID.String()) {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"valid":   false,
			"message": "Agent not connected",
		})
		return
	}

	cmd := agentgrpc.NewNginxTestConfigCommand()
	resp, err := agentgrpc.SendCommand(agentID.String(), cmd, 30*time.Second)
	if err != nil {
		h.logger.Error("Failed to send test command", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"valid":   false,
			"message": fmt.Sprintf("Failed to test config: %v", err),
		})
		return
	}

	// Parse response
	if result, ok := resp.Response.(*agentgrpc.CommandResult); ok {
		c.JSON(http.StatusOK, gin.H{
			"valid":   result.Success,
			"message": result.Message,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"valid":   true,
		"message": "Configuration test completed",
	})
}

// getSecurityHeaders retrieves security headers for a proxy host
func (h *Handler) getSecurityHeaders(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	agentIDStr := c.Param("id")
	proxyIDStr := c.Param("pid")

	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	proxyID, err := uuid.Parse(proxyIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid proxy ID"})
		return
	}

	// Verify proxy belongs to agent and org
	var exists bool
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT EXISTS(
			SELECT 1 FROM proxy_hosts p
			JOIN agents a ON p.agent_id = a.id
			WHERE p.id = $1 AND p.agent_id = $2 AND a.org_id = $3
		)
	`, proxyID, agentID, orgID).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "proxy host not found"})
		return
	}

	// Get security headers
	var headers SecurityHeaders
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT id, proxy_host_id, hsts_enabled, hsts_max_age, x_frame_options,
		       x_content_type_options, x_xss_protection, content_security_policy
		FROM proxy_security_headers
		WHERE proxy_host_id = $1
	`, proxyID).Scan(
		&headers.ID, &headers.ProxyHostID, &headers.HSTSEnabled, &headers.HSTSMaxAge,
		&headers.XFrameOptions, &headers.XContentTypeOptions, &headers.XXSSProtection,
		&headers.ContentSecurityPolicy,
	)

	if err != nil {
		// Return defaults if not found
		c.JSON(http.StatusOK, SecurityHeaders{
			ProxyHostID:         proxyID,
			HSTSEnabled:         false,
			HSTSMaxAge:          31536000,
			XFrameOptions:       "SAMEORIGIN",
			XContentTypeOptions: true,
			XXSSProtection:      true,
		})
		return
	}

	c.JSON(http.StatusOK, headers)
}

// UpdateSecurityHeadersRequest is the request body for updating security headers
type UpdateSecurityHeadersRequest struct {
	HSTSEnabled           bool    `json:"hsts_enabled"`
	HSTSMaxAge            int     `json:"hsts_max_age"`
	XFrameOptions         string  `json:"x_frame_options"`
	XContentTypeOptions   bool    `json:"x_content_type_options"`
	XXSSProtection        bool    `json:"x_xss_protection"`
	ContentSecurityPolicy *string `json:"content_security_policy,omitempty"`
}

// updateSecurityHeaders updates security headers for a proxy host
func (h *Handler) updateSecurityHeaders(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	agentIDStr := c.Param("id")
	proxyIDStr := c.Param("pid")

	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	proxyID, err := uuid.Parse(proxyIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid proxy ID"})
		return
	}

	var req UpdateSecurityHeadersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Verify proxy belongs to agent and org
	var exists bool
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT EXISTS(
			SELECT 1 FROM proxy_hosts p
			JOIN agents a ON p.agent_id = a.id
			WHERE p.id = $1 AND p.agent_id = $2 AND a.org_id = $3
		)
	`, proxyID, agentID, orgID).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "proxy host not found"})
		return
	}

	// Validate X-Frame-Options
	validXFrameOptions := map[string]bool{"DENY": true, "SAMEORIGIN": true, "": true}
	if !validXFrameOptions[req.XFrameOptions] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid x_frame_options value"})
		return
	}

	// Validate HSTS max-age
	if req.HSTSMaxAge < 0 {
		req.HSTSMaxAge = 31536000 // Default to 1 year
	}

	// Update or insert security headers
	var headers SecurityHeaders
	err = h.db.QueryRow(c.Request.Context(), `
		INSERT INTO proxy_security_headers (proxy_host_id, hsts_enabled, hsts_max_age, x_frame_options,
		                                    x_content_type_options, x_xss_protection, content_security_policy)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (proxy_host_id) DO UPDATE SET
			hsts_enabled = EXCLUDED.hsts_enabled,
			hsts_max_age = EXCLUDED.hsts_max_age,
			x_frame_options = EXCLUDED.x_frame_options,
			x_content_type_options = EXCLUDED.x_content_type_options,
			x_xss_protection = EXCLUDED.x_xss_protection,
			content_security_policy = EXCLUDED.content_security_policy,
			updated_at = NOW()
		RETURNING id, proxy_host_id, hsts_enabled, hsts_max_age, x_frame_options,
		          x_content_type_options, x_xss_protection, content_security_policy
	`, proxyID, req.HSTSEnabled, req.HSTSMaxAge, req.XFrameOptions,
		req.XContentTypeOptions, req.XXSSProtection, req.ContentSecurityPolicy).Scan(
		&headers.ID, &headers.ProxyHostID, &headers.HSTSEnabled, &headers.HSTSMaxAge,
		&headers.XFrameOptions, &headers.XContentTypeOptions, &headers.XXSSProtection,
		&headers.ContentSecurityPolicy,
	)

	if err != nil {
		h.logger.Error("Failed to update security headers", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update security headers"})
		return
	}

	c.JSON(http.StatusOK, headers)
}

// generateNginxConfig creates nginx server block config
func generateNginxConfig(proxy ProxyHost, headers SecurityHeaders) string {
	var config string

	// HTTP server block
	config += "server {\n"
	config += "    listen 80;\n"
	config += "    listen [::]:80;\n"
	config += fmt.Sprintf("    server_name %s;\n\n", proxy.Domain)

	if proxy.SSLEnabled && proxy.ForceSSL {
		config += "    return 301 https://$host$request_uri;\n"
		config += "}\n\n"
	} else {
		config += generateLocationBlock(proxy.UpstreamTarget)
		config += "}\n\n"
	}

	// HTTPS server block (if SSL enabled)
	if proxy.SSLEnabled {
		listen := "443 ssl"
		if proxy.HTTP2Enabled {
			listen = "443 ssl http2"
		}

		config += "server {\n"
		config += fmt.Sprintf("    listen %s;\n", listen)
		config += fmt.Sprintf("    listen [::]:%s;\n", listen)
		config += fmt.Sprintf("    server_name %s;\n\n", proxy.Domain)

		// SSL configuration
		certPath := "/etc/letsencrypt/live/" + proxy.Domain + "/fullchain.pem"
		keyPath := "/etc/letsencrypt/live/" + proxy.Domain + "/privkey.pem"
		if proxy.SSLCertPath != nil {
			certPath = *proxy.SSLCertPath
		}
		if proxy.SSLKeyPath != nil {
			keyPath = *proxy.SSLKeyPath
		}

		config += fmt.Sprintf("    ssl_certificate %s;\n", certPath)
		config += fmt.Sprintf("    ssl_certificate_key %s;\n\n", keyPath)

		config += "    ssl_protocols TLSv1.2 TLSv1.3;\n"
		config += "    ssl_prefer_server_ciphers on;\n"
		config += "    ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384;\n\n"

		// Security headers
		config += generateSecurityHeaders(headers)

		config += generateLocationBlock(proxy.UpstreamTarget)
		config += "}\n"
	}

	return config
}

func generateSecurityHeaders(headers SecurityHeaders) string {
	var config string

	if headers.HSTSEnabled {
		config += fmt.Sprintf("    add_header Strict-Transport-Security \"max-age=%d; includeSubDomains\" always;\n", headers.HSTSMaxAge)
	}

	if headers.XFrameOptions != "" {
		config += fmt.Sprintf("    add_header X-Frame-Options \"%s\" always;\n", headers.XFrameOptions)
	}

	if headers.XContentTypeOptions {
		config += "    add_header X-Content-Type-Options \"nosniff\" always;\n"
	}

	if headers.XXSSProtection {
		config += "    add_header X-XSS-Protection \"1; mode=block\" always;\n"
	}

	if headers.ContentSecurityPolicy != nil && *headers.ContentSecurityPolicy != "" {
		config += fmt.Sprintf("    add_header Content-Security-Policy \"%s\" always;\n", *headers.ContentSecurityPolicy)
	}

	if config != "" {
		config += "\n"
	}

	return config
}

func generateLocationBlock(upstream string) string {
	return fmt.Sprintf(`    location / {
        proxy_pass %s;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
    }
`, upstream)
}

// ============ Nginx Management ============

// testNginxConfig tests the entire nginx configuration
func (h *Handler) testNginxConfig(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	agentIDStr := c.Param("id")

	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	// Verify agent belongs to org
	var exists bool
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT EXISTS(SELECT 1 FROM agents WHERE id = $1 AND org_id = $2)
	`, agentID, orgID).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	// Check agent connection
	if !agentgrpc.IsAgentConnected(agentIDStr) {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"message": "Agent not connected",
		})
		return
	}

	// Send test command
	cmd := agentgrpc.NewNginxTestConfigCommand()
	resp, err := agentgrpc.SendCommand(agentIDStr, cmd, 30*time.Second)
	if err != nil {
		h.logger.Error("Failed to send nginx test command", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": fmt.Sprintf("Failed to test config: %v", err),
		})
		return
	}

	if result, ok := resp.Response.(*agentgrpc.CommandResult); ok {
		c.JSON(http.StatusOK, gin.H{
			"success": result.Success,
			"message": result.Message,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Configuration test completed",
	})
}

// reloadNginx reloads the nginx configuration
func (h *Handler) reloadNginx(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	userID := c.MustGet("user_id").(uuid.UUID)
	agentIDStr := c.Param("id")

	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	// Verify agent belongs to org
	var exists bool
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT EXISTS(SELECT 1 FROM agents WHERE id = $1 AND org_id = $2)
	`, agentID, orgID).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	// Check agent connection
	if !agentgrpc.IsAgentConnected(agentIDStr) {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"message": "Agent not connected",
		})
		return
	}

	// Send reload command
	cmd := agentgrpc.NewNginxReloadCommand()
	resp, err := agentgrpc.SendCommand(agentIDStr, cmd, 30*time.Second)
	if err != nil {
		h.logger.Error("Failed to send nginx reload command", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": fmt.Sprintf("Failed to reload nginx: %v", err),
		})
		return
	}

	// Audit log
	h.auditLog(c, userID, orgID, "nginx.reload", "agent", agentID, nil)

	if result, ok := resp.Response.(*agentgrpc.CommandResult); ok {
		c.JSON(http.StatusOK, gin.H{
			"success": result.Success,
			"message": result.Message,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Nginx reloaded successfully",
	})
}

// ============ gRPC Dispatch Functions ============

// dispatchProxyConfig sends nginx config to the agent
func (h *Handler) dispatchProxyConfig(ctx context.Context, agentID, proxyID uuid.UUID, domain, upstream string, forceSSL, http2, sslEnabled bool) {
	agentIDStr := agentID.String()

	if !agentgrpc.IsAgentConnected(agentIDStr) {
		h.logger.Warn("Agent not connected, cannot dispatch proxy config",
			zap.String("agent_id", agentIDStr),
			zap.String("domain", domain),
		)
		return
	}

	// Fetch security headers for complete config
	var headers SecurityHeaders
	h.db.QueryRow(ctx, `
		SELECT id, proxy_host_id, hsts_enabled, hsts_max_age, x_frame_options,
		       x_content_type_options, x_xss_protection, content_security_policy
		FROM proxy_security_headers
		WHERE proxy_host_id = $1
	`, proxyID).Scan(
		&headers.ID, &headers.ProxyHostID, &headers.HSTSEnabled, &headers.HSTSMaxAge,
		&headers.XFrameOptions, &headers.XContentTypeOptions, &headers.XXSSProtection,
		&headers.ContentSecurityPolicy,
	)

	// Build proxy host for config generation
	proxy := ProxyHost{
		Domain:         domain,
		UpstreamTarget: upstream,
		ForceSSL:       forceSSL,
		HTTP2Enabled:   http2,
		SSLEnabled:     sslEnabled,
	}

	// Generate nginx config
	config := generateNginxConfig(proxy, headers)

	// Build config path
	configPath := filepath.Join("/etc/nginx/conf.d", domain+".conf")

	// Create command with config content
	cmdPayload, _ := json.Marshal(agentgrpc.NginxCommand{
		Action:        agentgrpc.NginxActionWriteConfig,
		ConfigContent: config,
		ConfigPath:    configPath,
	})

	cmd := &agentgrpc.BackendMessage{
		RequestId: uuid.New().String(),
		Command:   cmdPayload,
	}

	// Send command (non-blocking)
	if err := agentgrpc.SendCommandAsync(agentIDStr, cmd); err != nil {
		h.logger.Error("Failed to dispatch proxy config",
			zap.Error(err),
			zap.String("domain", domain),
		)
		return
	}

	h.logger.Info("Dispatched proxy config to agent",
		zap.String("agent_id", agentIDStr),
		zap.String("domain", domain),
	)
}

// dispatchDeleteProxy sends delete command to the agent
func (h *Handler) dispatchDeleteProxy(agentID uuid.UUID, domain string) {
	agentIDStr := agentID.String()

	if !agentgrpc.IsAgentConnected(agentIDStr) {
		h.logger.Warn("Agent not connected, cannot dispatch delete",
			zap.String("agent_id", agentIDStr),
			zap.String("domain", domain),
		)
		return
	}

	// Build config path to delete
	configPath := filepath.Join("/etc/nginx/conf.d", domain+".conf")

	// Create delete command (write empty config will fail validation, so we use a different approach)
	// The agent should delete the file and reload nginx
	cmdPayload, _ := json.Marshal(map[string]interface{}{
		"action":      "delete_config",
		"config_path": configPath,
		"domain":      domain,
	})

	cmd := &agentgrpc.BackendMessage{
		RequestId: uuid.New().String(),
		Command:   cmdPayload,
	}

	// Send command (non-blocking)
	if err := agentgrpc.SendCommandAsync(agentIDStr, cmd); err != nil {
		h.logger.Error("Failed to dispatch delete command",
			zap.Error(err),
			zap.String("domain", domain),
		)
		return
	}

	h.logger.Info("Dispatched delete command to agent",
		zap.String("agent_id", agentIDStr),
		zap.String("domain", domain),
	)
}

// dispatchSSLRequest sends SSL certificate request to the agent
func (h *Handler) dispatchSSLRequest(agentID uuid.UUID, domain, email, dnsProvider string) {
	agentIDStr := agentID.String()

	if !agentgrpc.IsAgentConnected(agentIDStr) {
		h.logger.Warn("Agent not connected, cannot dispatch SSL request",
			zap.String("agent_id", agentIDStr),
			zap.String("domain", domain),
		)
		return
	}

	cmd := agentgrpc.NewNginxSSLCommand(domain, email, dnsProvider)

	// Send command (non-blocking for now, but we should track the result)
	if err := agentgrpc.SendCommandAsync(agentIDStr, cmd); err != nil {
		h.logger.Error("Failed to dispatch SSL request",
			zap.Error(err),
			zap.String("domain", domain),
		)
		return
	}

	h.logger.Info("Dispatched SSL request to agent",
		zap.String("agent_id", agentIDStr),
		zap.String("domain", domain),
		zap.String("email", email),
	)
}
