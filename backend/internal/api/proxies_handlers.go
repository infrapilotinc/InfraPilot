package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	agentgrpc "github.com/infrapilot/backend/internal/grpc"
)

// ProxyHost represents an nginx reverse proxy configuration
type ProxyHost struct {
	ID               uuid.UUID  `json:"id"`
	AgentID          uuid.UUID  `json:"agent_id"`
	Domain           string     `json:"domain"`
	UpstreamTarget   string     `json:"upstream_target"`
	ProxyType        string     `json:"proxy_type"`                     // "upstream" or "redirect"
	RedirectURL      *string    `json:"redirect_url,omitempty"`         // URL to redirect to (for redirect type)
	RedirectCode     int        `json:"redirect_code,omitempty"`        // 301, 302, 307, 308 (for redirect type)
	SSLEnabled       bool       `json:"ssl_enabled"`
	SSLCertPath      *string    `json:"ssl_cert_path,omitempty"`
	SSLKeyPath       *string    `json:"ssl_key_path,omitempty"`
	SSLExpiresAt     *time.Time `json:"ssl_expires_at,omitempty"`
	ForceSSL         bool       `json:"force_ssl"`
	HTTP2Enabled     bool       `json:"http2_enabled"`
	IncludeWWW       bool       `json:"include_www"`
	BasicAuthEnabled       bool     `json:"basic_auth_enabled"`
	BasicAuthRealm         string   `json:"basic_auth_realm,omitempty"`
	BasicAuthExcludedPaths []string `json:"basic_auth_excluded_paths,omitempty"`
	BearerAuthEnabled      bool     `json:"bearer_auth_enabled"`
	AccessLog              bool     `json:"access_log"`
	ErrorLog               bool     `json:"error_log"`
	LogFormat              *string  `json:"log_format,omitempty"`
	ConfigHash             *string  `json:"config_hash,omitempty"`
	Status           string     `json:"status"`
	IsSystemProxy    bool       `json:"is_system_proxy"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
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
	Domain           string  `json:"domain" binding:"required"`
	UpstreamTarget   string  `json:"upstream_target"`                 // Required if proxy_type is "upstream"
	ProxyType        string  `json:"proxy_type"`                      // "upstream" (default) or "redirect"
	RedirectURL      string  `json:"redirect_url"`                    // Required if proxy_type is "redirect"
	RedirectCode     int     `json:"redirect_code"`                   // 301, 302, 307, 308 (default 301)
	ForceSSL         bool    `json:"force_ssl"`
	HTTP2Enabled     bool    `json:"http2_enabled"`
	IncludeWWW       bool    `json:"include_www"`
	BasicAuthEnabled bool    `json:"basic_auth_enabled"`
	BasicAuthRealm   string  `json:"basic_auth_realm,omitempty"`
	AccessLog        *bool   `json:"access_log,omitempty"`            // Enable access logging (default true)
	ErrorLog         *bool   `json:"error_log,omitempty"`             // Enable error logging (default true)
}

// UpdateProxyRequest is the request body for updating a proxy host
type UpdateProxyRequest struct {
	Domain                 *string   `json:"domain,omitempty"`
	UpstreamTarget         *string   `json:"upstream_target,omitempty"`
	ProxyType              *string   `json:"proxy_type,omitempty"`       // "upstream" or "redirect"
	RedirectURL            *string   `json:"redirect_url,omitempty"`     // URL to redirect to
	RedirectCode           *int      `json:"redirect_code,omitempty"`    // 301, 302, 307, 308
	ForceSSL               *bool     `json:"force_ssl,omitempty"`
	HTTP2Enabled           *bool     `json:"http2_enabled,omitempty"`
	IncludeWWW             *bool     `json:"include_www,omitempty"`
	BasicAuthEnabled       *bool     `json:"basic_auth_enabled,omitempty"`
	BasicAuthRealm         *string   `json:"basic_auth_realm,omitempty"`
	BasicAuthExcludedPaths *[]string `json:"basic_auth_excluded_paths,omitempty"`
	BearerAuthEnabled      *bool     `json:"bearer_auth_enabled,omitempty"`
	AccessLog              *bool     `json:"access_log,omitempty"`
	ErrorLog               *bool     `json:"error_log,omitempty"`
	Status                 *string   `json:"status,omitempty"`
}

// TestNetworkRequest is the request body for testing network connectivity
type TestNetworkRequest struct {
	ContainerName string `json:"container_name" binding:"required"`
	Port          int    `json:"port" binding:"required"`
}

// TestNetworkResponse is the response for network connectivity test
type TestNetworkResponse struct {
	Reachable      bool   `json:"reachable"`
	Message        string `json:"message"`
	AvailablePorts []int  `json:"available_ports,omitempty"`
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
		SELECT id, agent_id, domain, upstream_target,
		       COALESCE(proxy_type, 'upstream'), redirect_url, COALESCE(redirect_code, 301),
		       ssl_enabled, ssl_cert_path, ssl_key_path, ssl_expires_at,
		       force_ssl, http2_enabled, include_www,
		       COALESCE(basic_auth_enabled, false), COALESCE(basic_auth_realm, 'Restricted'),
		       COALESCE(basic_auth_excluded_paths, '{}'),
		       COALESCE(access_log, true), COALESCE(error_log, true), log_format,
		       config_hash, status, is_system_proxy, created_at, updated_at
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
			&p.ID, &p.AgentID, &p.Domain, &p.UpstreamTarget,
			&p.ProxyType, &p.RedirectURL, &p.RedirectCode,
			&p.SSLEnabled, &p.SSLCertPath, &p.SSLKeyPath, &p.SSLExpiresAt,
			&p.ForceSSL, &p.HTTP2Enabled, &p.IncludeWWW,
			&p.BasicAuthEnabled, &p.BasicAuthRealm, &p.BasicAuthExcludedPaths,
			&p.AccessLog, &p.ErrorLog, &p.LogFormat,
			&p.ConfigHash, &p.Status, &p.IsSystemProxy, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			continue
		}
		proxies = append(proxies, p)
	}

	c.JSON(http.StatusOK, proxies)
}

// evaluateProxyPolicy checks policies before proxy actions (community edition - no policy engine)
func (h *Handler) evaluateProxyPolicy(c *gin.Context, orgID uuid.UUID, domain string, sslEnabled bool, action string) (bool, string) {
	// Community edition: no policy engine, always allow
	return false, ""
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

	// A stray leading/trailing space (e.g. from a copy-paste) becomes part of the
	// generated nginx config, since UpstreamTarget is interpolated into a `set
	// $upstream "..."` variable rather than a literal -- nginx can't validate that
	// at config-load time, so it only 500s once a real request comes in, with no
	// clue in the UI about why. Trim once here rather than at every later read site.
	req.Domain = strings.TrimSpace(req.Domain)
	req.UpstreamTarget = strings.TrimSpace(req.UpstreamTarget)
	req.RedirectURL = strings.TrimSpace(req.RedirectURL)
	req.BasicAuthRealm = strings.TrimSpace(req.BasicAuthRealm)

	// Default proxy_type to "upstream" if not specified
	if req.ProxyType == "" {
		req.ProxyType = "upstream"
	}

	// Validate proxy type
	if req.ProxyType != "upstream" && req.ProxyType != "redirect" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid proxy_type: must be 'upstream' or 'redirect'"})
		return
	}

	// Validate based on proxy type
	if req.ProxyType == "upstream" {
		if req.UpstreamTarget == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "upstream_target is required for upstream proxy type"})
			return
		}
	} else if req.ProxyType == "redirect" {
		if req.RedirectURL == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "redirect_url is required for redirect proxy type"})
			return
		}
		// Default redirect code to 301 if not specified
		if req.RedirectCode == 0 {
			req.RedirectCode = 301
		}
		// Validate redirect code
		validCodes := map[int]bool{301: true, 302: true, 307: true, 308: true}
		if !validCodes[req.RedirectCode] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid redirect_code: must be 301, 302, 307, or 308"})
			return
		}
	}

	// Check policies before creating proxy
	if blocked, message := h.evaluateProxyPolicy(c, orgID, req.Domain, false, "create"); blocked {
		c.JSON(http.StatusForbidden, gin.H{
			"error":   "Action blocked by policy",
			"message": message,
		})
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
	basicAuthRealm := req.BasicAuthRealm
	if basicAuthRealm == "" {
		basicAuthRealm = "Restricted"
	}

	// Prepare redirect URL (nil for upstream type)
	var redirectURL *string
	var redirectCode *int
	if req.ProxyType == "redirect" {
		redirectURL = &req.RedirectURL
		redirectCode = &req.RedirectCode
	}

	// Default logging to true if not specified
	accessLog := true
	errorLog := true
	if req.AccessLog != nil {
		accessLog = *req.AccessLog
	}
	if req.ErrorLog != nil {
		errorLog = *req.ErrorLog
	}

	var proxyID uuid.UUID
	err = h.db.QueryRow(c.Request.Context(), `
		INSERT INTO proxy_hosts (agent_id, domain, upstream_target, proxy_type, redirect_url, redirect_code,
		                         force_ssl, http2_enabled, include_www, basic_auth_enabled, basic_auth_realm,
		                         access_log, error_log, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, 'active')
		RETURNING id
	`, agentID, req.Domain, req.UpstreamTarget, req.ProxyType, redirectURL, redirectCode,
		req.ForceSSL, req.HTTP2Enabled, req.IncludeWWW, req.BasicAuthEnabled, basicAuthRealm,
		accessLog, errorLog).Scan(&proxyID)

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

	// Push config to agent via gRPC (only for upstream proxies that have an upstream target)
	go h.dispatchProxyConfigFull(c.Request.Context(), agentID, proxyID, req.Domain, req.UpstreamTarget,
		req.ProxyType, redirectURL, redirectCode, req.ForceSSL, req.HTTP2Enabled, req.IncludeWWW, false)

	// Fetch and return the created proxy
	var proxy ProxyHost
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT id, agent_id, domain, upstream_target,
		       COALESCE(proxy_type, 'upstream'), redirect_url, COALESCE(redirect_code, 301),
		       ssl_enabled, ssl_cert_path, ssl_key_path, ssl_expires_at,
		       force_ssl, http2_enabled, include_www,
		       COALESCE(basic_auth_enabled, false), COALESCE(basic_auth_realm, 'Restricted'),
		       COALESCE(access_log, true), COALESCE(error_log, true), log_format,
		       config_hash, status, is_system_proxy, created_at, updated_at
		FROM proxy_hosts
		WHERE id = $1
	`, proxyID).Scan(
		&proxy.ID, &proxy.AgentID, &proxy.Domain, &proxy.UpstreamTarget,
		&proxy.ProxyType, &proxy.RedirectURL, &proxy.RedirectCode,
		&proxy.SSLEnabled, &proxy.SSLCertPath, &proxy.SSLKeyPath, &proxy.SSLExpiresAt,
		&proxy.ForceSSL, &proxy.HTTP2Enabled, &proxy.IncludeWWW,
		&proxy.BasicAuthEnabled, &proxy.BasicAuthRealm,
		&proxy.AccessLog, &proxy.ErrorLog, &proxy.LogFormat,
		&proxy.ConfigHash, &proxy.Status, &proxy.IsSystemProxy, &proxy.CreatedAt, &proxy.UpdatedAt,
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
		SELECT id, agent_id, domain, upstream_target,
		       COALESCE(proxy_type, 'upstream'), redirect_url, COALESCE(redirect_code, 301),
		       ssl_enabled, ssl_cert_path, ssl_key_path, ssl_expires_at,
		       force_ssl, http2_enabled, include_www,
		       COALESCE(basic_auth_enabled, false), COALESCE(basic_auth_realm, 'Restricted'),
		       COALESCE(access_log, true), COALESCE(error_log, true), log_format,
		       config_hash, status, is_system_proxy, created_at, updated_at
		FROM proxy_hosts
		WHERE id = $1 AND agent_id = $2
	`, proxyID, agentID).Scan(
		&proxy.ID, &proxy.AgentID, &proxy.Domain, &proxy.UpstreamTarget,
		&proxy.ProxyType, &proxy.RedirectURL, &proxy.RedirectCode,
		&proxy.SSLEnabled, &proxy.SSLCertPath, &proxy.SSLKeyPath, &proxy.SSLExpiresAt,
		&proxy.ForceSSL, &proxy.HTTP2Enabled, &proxy.IncludeWWW,
		&proxy.BasicAuthEnabled, &proxy.BasicAuthRealm,
		&proxy.AccessLog, &proxy.ErrorLog, &proxy.LogFormat,
		&proxy.ConfigHash, &proxy.Status, &proxy.IsSystemProxy, &proxy.CreatedAt, &proxy.UpdatedAt,
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

	// See createProxyHost for why these get trimmed before anything downstream
	// (dispatch, nginx config generation) ever sees them.
	if req.Domain != nil {
		trimmed := strings.TrimSpace(*req.Domain)
		req.Domain = &trimmed
	}
	if req.UpstreamTarget != nil {
		trimmed := strings.TrimSpace(*req.UpstreamTarget)
		req.UpstreamTarget = &trimmed
	}
	if req.RedirectURL != nil {
		trimmed := strings.TrimSpace(*req.RedirectURL)
		req.RedirectURL = &trimmed
	}
	if req.BasicAuthRealm != nil {
		trimmed := strings.TrimSpace(*req.BasicAuthRealm)
		req.BasicAuthRealm = &trimmed
	}

	// Get current proxy to check policies
	var currentDomain string
	var currentSSL bool
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT domain, ssl_enabled FROM proxy_hosts WHERE id = $1 AND agent_id = $2
	`, proxyID, agentID).Scan(&currentDomain, &currentSSL)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "proxy not found"})
		return
	}

	// Use new domain if provided, otherwise current
	domain := currentDomain
	if req.Domain != nil {
		domain = *req.Domain
	}

	// Check policies before updating proxy
	if blocked, message := h.evaluateProxyPolicy(c, orgID, domain, currentSSL, "update"); blocked {
		c.JSON(http.StatusForbidden, gin.H{
			"error":   "Action blocked by policy",
			"message": message,
		})
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
	if req.ProxyType != nil {
		// Validate proxy type
		if *req.ProxyType != "upstream" && *req.ProxyType != "redirect" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid proxy_type: must be 'upstream' or 'redirect'"})
			return
		}
		argCount++
		query += fmt.Sprintf(", proxy_type = $%d", argCount)
		args = append(args, *req.ProxyType)
	}
	if req.RedirectURL != nil {
		argCount++
		query += fmt.Sprintf(", redirect_url = $%d", argCount)
		args = append(args, *req.RedirectURL)
	}
	if req.RedirectCode != nil {
		// Validate redirect code
		validCodes := map[int]bool{301: true, 302: true, 307: true, 308: true}
		if !validCodes[*req.RedirectCode] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid redirect_code: must be 301, 302, 307, or 308"})
			return
		}
		argCount++
		query += fmt.Sprintf(", redirect_code = $%d", argCount)
		args = append(args, *req.RedirectCode)
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
	if req.IncludeWWW != nil {
		argCount++
		query += fmt.Sprintf(", include_www = $%d", argCount)
		args = append(args, *req.IncludeWWW)
	}
	if req.BasicAuthEnabled != nil {
		argCount++
		query += fmt.Sprintf(", basic_auth_enabled = $%d", argCount)
		args = append(args, *req.BasicAuthEnabled)
	}
	if req.BasicAuthRealm != nil {
		argCount++
		query += fmt.Sprintf(", basic_auth_realm = $%d", argCount)
		args = append(args, *req.BasicAuthRealm)
	}
	if req.BasicAuthExcludedPaths != nil {
		argCount++
		query += fmt.Sprintf(", basic_auth_excluded_paths = $%d", argCount)
		args = append(args, *req.BasicAuthExcludedPaths)
	}
	if req.BearerAuthEnabled != nil {
		argCount++
		query += fmt.Sprintf(", bearer_auth_enabled = $%d", argCount)
		args = append(args, *req.BearerAuthEnabled)
	}
	if req.AccessLog != nil {
		argCount++
		query += fmt.Sprintf(", access_log = $%d", argCount)
		args = append(args, *req.AccessLog)
	}
	if req.ErrorLog != nil {
		argCount++
		query += fmt.Sprintf(", error_log = $%d", argCount)
		args = append(args, *req.ErrorLog)
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
		SELECT id, agent_id, domain, upstream_target,
		       COALESCE(proxy_type, 'upstream'), redirect_url, COALESCE(redirect_code, 301),
		       ssl_enabled, ssl_cert_path, ssl_key_path, ssl_expires_at,
		       force_ssl, http2_enabled, include_www,
		       COALESCE(basic_auth_enabled, false), COALESCE(basic_auth_realm, 'Restricted'),
		       COALESCE(basic_auth_excluded_paths, '{}'), COALESCE(bearer_auth_enabled, false),
		       COALESCE(access_log, true), COALESCE(error_log, true), log_format,
		       config_hash, status, is_system_proxy, created_at, updated_at
		FROM proxy_hosts
		WHERE id = $1
	`, proxyID).Scan(
		&proxy.ID, &proxy.AgentID, &proxy.Domain, &proxy.UpstreamTarget,
		&proxy.ProxyType, &proxy.RedirectURL, &proxy.RedirectCode,
		&proxy.SSLEnabled, &proxy.SSLCertPath, &proxy.SSLKeyPath, &proxy.SSLExpiresAt,
		&proxy.ForceSSL, &proxy.HTTP2Enabled, &proxy.IncludeWWW,
		&proxy.BasicAuthEnabled, &proxy.BasicAuthRealm, &proxy.BasicAuthExcludedPaths, &proxy.BearerAuthEnabled,
		&proxy.AccessLog, &proxy.ErrorLog, &proxy.LogFormat,
		&proxy.ConfigHash, &proxy.Status, &proxy.IsSystemProxy, &proxy.CreatedAt, &proxy.UpdatedAt,
	)

	// Push updated config to agent via gRPC
	go h.dispatchProxyConfigFull(context.Background(), agentID, proxyID, proxy.Domain, proxy.UpstreamTarget,
		proxy.ProxyType, proxy.RedirectURL, &proxy.RedirectCode, proxy.ForceSSL, proxy.HTTP2Enabled, proxy.IncludeWWW, proxy.SSLEnabled)

	// If basic auth settings changed, dispatch htpasswd update (handles both system and regular proxies)
	if req.BasicAuthEnabled != nil || req.BasicAuthRealm != nil || req.BasicAuthExcludedPaths != nil {
		go h.dispatchHtpasswd(context.Background(), agentID, proxyID)
	}

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

	// Get proxy domain and SSL status for policy check and audit log
	var domain string
	var sslEnabled bool
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT domain, ssl_enabled FROM proxy_hosts WHERE id = $1 AND agent_id = $2
	`, proxyID, agentID).Scan(&domain, &sslEnabled)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "proxy host not found"})
		return
	}

	// Check policies before deleting proxy
	if blocked, message := h.evaluateProxyPolicy(c, orgID, domain, sslEnabled, "delete"); blocked {
		c.JSON(http.StatusForbidden, gin.H{
			"error":   "Action blocked by policy",
			"message": message,
		})
		return
	}

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

	// Get proxy domain and include_www setting
	var domain string
	var includeWWW bool
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT domain, include_www FROM proxy_hosts WHERE id = $1 AND agent_id = $2
	`, proxyID, agentID).Scan(&domain, &includeWWW)

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
	go h.dispatchSSLRequest(agentID, domain, sslEmail, "", includeWWW)

	// Audit log
	h.auditLog(c, userID, orgID, "proxy.ssl_request", "proxy_host", proxyID, gin.H{"domain": domain})

	c.JSON(http.StatusAccepted, gin.H{
		"message": "SSL certificate request initiated",
		"domain":  domain,
		"status":  "pending",
	})
}

// ApplyWildcardSSLRequest is the request body for applying wildcard SSL
type ApplyWildcardSSLRequest struct {
	SSLEnabled   bool   `json:"ssl_enabled"`
	ForceSSL     bool   `json:"force_ssl"`
	HTTP2Enabled bool   `json:"http2_enabled"`
	SSLSource    string `json:"ssl_source"`
	SSLCertPath  string `json:"ssl_cert_path" binding:"required"`
	SSLKeyPath   string `json:"ssl_key_path" binding:"required"`
}

// applyWildcardSSL applies a wildcard SSL certificate to a proxy host
func (h *Handler) applyWildcardSSL(c *gin.Context) {
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

	var req ApplyWildcardSSLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
	var domain, upstream string
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT domain, upstream_target FROM proxy_hosts WHERE id = $1 AND agent_id = $2
	`, proxyID, agentID).Scan(&domain, &upstream)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "proxy host not found"})
		return
	}

	// Update proxy_hosts with SSL settings
	_, err = h.db.Exec(c.Request.Context(), `
		UPDATE proxy_hosts SET
			ssl_enabled = $1,
			force_ssl = $2,
			http2_enabled = $3,
			ssl_source = $4,
			ssl_cert_path = $5,
			ssl_key_path = $6,
			status = 'active',
			updated_at = NOW()
		WHERE id = $7
	`, req.SSLEnabled, req.ForceSSL, req.HTTP2Enabled, req.SSLSource,
		req.SSLCertPath, req.SSLKeyPath, proxyID)

	if err != nil {
		h.logger.Error("Failed to update proxy SSL settings", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update proxy"})
		return
	}

	// Dispatch updated nginx config to agent
	go h.dispatchProxyConfigWithCert(c.Request.Context(), agentID, proxyID, domain, upstream,
		req.ForceSSL, req.HTTP2Enabled, req.SSLEnabled, req.SSLCertPath, req.SSLKeyPath)

	// Audit log
	h.auditLog(c, userID, orgID, "proxy.ssl_wildcard", "proxy_host", proxyID, gin.H{
		"domain":    domain,
		"cert_path": req.SSLCertPath,
	})

	c.JSON(http.StatusOK, gin.H{
		"message":   "Wildcard SSL certificate applied",
		"domain":    domain,
		"cert_path": req.SSLCertPath,
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
		       ssl_key_path, ssl_expires_at, force_ssl, http2_enabled, include_www,
		       COALESCE(basic_auth_enabled, false), COALESCE(basic_auth_realm, 'Restricted'),
		       COALESCE(bearer_auth_enabled, false),
		       config_hash, status, created_at, updated_at
		FROM proxy_hosts
		WHERE id = $1 AND agent_id = $2
	`, proxyID, agentID).Scan(
		&proxy.ID, &proxy.AgentID, &proxy.Domain, &proxy.UpstreamTarget, &proxy.SSLEnabled,
		&proxy.SSLCertPath, &proxy.SSLKeyPath, &proxy.SSLExpiresAt, &proxy.ForceSSL,
		&proxy.HTTP2Enabled, &proxy.IncludeWWW, &proxy.BasicAuthEnabled, &proxy.BasicAuthRealm,
		&proxy.BearerAuthEnabled,
		&proxy.ConfigHash, &proxy.Status, &proxy.CreatedAt, &proxy.UpdatedAt,
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

	bearerTokens, err := h.decryptBearerTokens(c.Request.Context(), proxyID)
	if err != nil {
		h.logger.Error("Failed to decrypt bearer tokens", zap.Error(err))
	}

	// Generate nginx config
	config := generateNginxConfig(proxy, headers, bearerTokens)

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
	if result, err := resp.GetCommandResult(); err == nil && result != nil {
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

// ConfigPreviewRequest is the request body for generating nginx config preview
type ConfigPreviewRequest struct {
	Domain           string  `json:"domain" binding:"required"`
	UpstreamTarget   string  `json:"upstream_target" binding:"required"`
	SSLEnabled       bool    `json:"ssl_enabled"`
	ForceSSL         bool    `json:"force_ssl"`
	HTTP2Enabled     bool    `json:"http2_enabled"`
	IncludeWWW       bool    `json:"include_www"`
	BasicAuthEnabled bool    `json:"basic_auth_enabled"`
	BasicAuthRealm   string  `json:"basic_auth_realm"`
	BearerAuthEnabled bool   `json:"bearer_auth_enabled"`
	HSTSEnabled      bool    `json:"hsts_enabled"`
	HSTSMaxAge       int     `json:"hsts_max_age"`
	XFrameOptions    string  `json:"x_frame_options"`
	XContentTypeOptions bool `json:"x_content_type_options"`
	XXSSProtection   bool    `json:"x_xss_protection"`
	ContentSecurityPolicy *string `json:"content_security_policy,omitempty"`
}

// previewProxyConfig generates nginx config from form values without saving
func (h *Handler) previewProxyConfig(c *gin.Context) {
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

	var req ConfigPreviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// See createProxyHost for why -- this path never touches the DB, so it needs
	// its own trim rather than inheriting one applied elsewhere.
	req.Domain = strings.TrimSpace(req.Domain)
	req.UpstreamTarget = strings.TrimSpace(req.UpstreamTarget)

	// Set default values
	if req.BasicAuthRealm == "" {
		req.BasicAuthRealm = "Restricted"
	}
	if req.HSTSMaxAge == 0 {
		req.HSTSMaxAge = 31536000
	}

	// Build proxy host for config generation
	proxy := ProxyHost{
		Domain:            req.Domain,
		UpstreamTarget:    req.UpstreamTarget,
		SSLEnabled:        req.SSLEnabled,
		ForceSSL:          req.ForceSSL,
		HTTP2Enabled:      req.HTTP2Enabled,
		IncludeWWW:        req.IncludeWWW,
		BasicAuthEnabled:  req.BasicAuthEnabled,
		BasicAuthRealm:    req.BasicAuthRealm,
		BearerAuthEnabled: req.BearerAuthEnabled,
	}

	// Build security headers for config generation
	headers := SecurityHeaders{
		HSTSEnabled:           req.HSTSEnabled,
		HSTSMaxAge:            req.HSTSMaxAge,
		XFrameOptions:         req.XFrameOptions,
		XContentTypeOptions:   req.XContentTypeOptions,
		XXSSProtection:        req.XXSSProtection,
		ContentSecurityPolicy: req.ContentSecurityPolicy,
	}

	// This proxy already exists (it's being edited), so its real saved tokens (if any)
	// are what would actually end up in the deployed config -- fetch them rather than
	// showing a placeholder.
	bearerTokens, err := h.decryptBearerTokens(c.Request.Context(), proxyID)
	if err != nil {
		h.logger.Error("Failed to decrypt bearer tokens for preview", zap.Error(err))
	}

	// Generate nginx config
	config := generateNginxConfig(proxy, headers, bearerTokens)

	c.JSON(http.StatusOK, gin.H{
		"config": config,
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

	// Handle nil content_security_policy by converting to empty string for database
	var csp *string
	if req.ContentSecurityPolicy != nil {
		csp = req.ContentSecurityPolicy
	}

	// Update or insert security headers
	var headers SecurityHeaders
	var cspResult *string
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
		req.XContentTypeOptions, req.XXSSProtection, csp).Scan(
		&headers.ID, &headers.ProxyHostID, &headers.HSTSEnabled, &headers.HSTSMaxAge,
		&headers.XFrameOptions, &headers.XContentTypeOptions, &headers.XXSSProtection,
		&cspResult,
	)
	headers.ContentSecurityPolicy = cspResult

	if err != nil {
		h.logger.Error("Failed to update security headers", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update security headers"})
		return
	}

	c.JSON(http.StatusOK, headers)
}

// generateLoggingConfig creates nginx logging directives for a domain
func generateLoggingConfig(domain string, accessLog, errorLog bool) string {
	var config string

	// Use per-domain log files for better filtering
	// Logs are stored in /var/log/nginx/domains/
	if accessLog {
		config += fmt.Sprintf("    access_log /var/log/nginx/domains/%s.access.log combined;\n", domain)
	} else {
		config += "    access_log off;\n"
	}

	if errorLog {
		config += fmt.Sprintf("    error_log /var/log/nginx/domains/%s.error.log warn;\n", domain)
	} else {
		config += "    error_log /dev/null;\n"
	}

	if accessLog || errorLog {
		config += "\n"
	}

	return config
}

// generateNginxConfig creates nginx server block config
func generateNginxConfig(proxy ProxyHost, headers SecurityHeaders, bearerTokens []string) string {
	var config string

	// Build server_name with optional www
	serverName := proxy.Domain
	if proxy.IncludeWWW {
		serverName = fmt.Sprintf("%s www.%s", proxy.Domain, proxy.Domain)
	}

	// HTTP server block
	config += "server {\n"
	config += "    listen 80;\n"
	config += "    listen [::]:80;\n"
	config += fmt.Sprintf("    server_name %s;\n\n", serverName)

	// Logging configuration for HTTP block
	config += generateLoggingConfig(proxy.Domain, proxy.AccessLog, proxy.ErrorLog)

	// Always include ACME challenge location for Let's Encrypt certificate issuance/renewal
	config += "    # ACME challenge for Let's Encrypt\n"
	config += "    location /.well-known/acme-challenge/ {\n"
	config += "        root /var/www/acme-challenge;\n"
	config += "        try_files $uri =404;\n"
	config += "    }\n\n"

	if proxy.SSLEnabled && proxy.ForceSSL {
		config += "    # Redirect all other HTTP to HTTPS\n"
		config += "    location / {\n"
		config += "        return 301 https://$host$request_uri;\n"
		config += "    }\n"
		config += "}\n\n"
	} else {
		// Check if this is a redirect proxy
		if proxy.ProxyType == "redirect" && proxy.RedirectURL != nil && *proxy.RedirectURL != "" {
			config += generateRedirectLocationBlock(*proxy.RedirectURL, proxy.RedirectCode)
		} else {
			// Docker DNS resolver: defers upstream hostname resolution to request time
			// so nginx starts cleanly even when the upstream container is stopped.
			config += "    resolver 127.0.0.11 valid=30s ipv6=off;\n"
			config += "    root /var/www/html;\n"
			config += "    error_page 502 503 504 /502.html;\n"
			config += "    location = /502.html { internal; }\n\n"
			config += generateLocationBlockWithAuth(proxy.UpstreamTarget, proxy.BasicAuthEnabled, proxy.BasicAuthRealm, proxy.Domain, proxy.BasicAuthExcludedPaths, proxy.BearerAuthEnabled, bearerTokens)
		}
		config += "}\n\n"
	}

	// HTTPS server block (if SSL enabled)
	if proxy.SSLEnabled {
		config += "server {\n"
		config += "    listen 443 ssl;\n"
		config += "    listen [::]:443 ssl;\n"
		if proxy.HTTP2Enabled {
			config += "    http2 on;\n"
		}
		config += fmt.Sprintf("    server_name %s;\n\n", serverName)

		// Logging configuration for HTTPS block
		config += generateLoggingConfig(proxy.Domain, proxy.AccessLog, proxy.ErrorLog)

		// SSL configuration - determine effective cert paths
		var certPath, keyPath string
		if proxy.SSLCertPath != nil && *proxy.SSLCertPath != "" {
			certPath = *proxy.SSLCertPath
		}
		if proxy.SSLKeyPath != nil && *proxy.SSLKeyPath != "" {
			keyPath = *proxy.SSLKeyPath
		}
		// If no explicit paths, determine the correct certificate to use
		if certPath == "" {
			// First, check if this exact domain has its own certificate
			exactCertPath := "/etc/letsencrypt/live/" + proxy.Domain + "/fullchain.pem"
			exactKeyPath := "/etc/letsencrypt/live/" + proxy.Domain + "/privkey.pem"

			if _, err := os.Stat(exactCertPath); err == nil {
				// Domain has its own certificate - use it
				certPath = exactCertPath
				keyPath = exactKeyPath
			} else {
				// No exact cert, check for wildcard cert at parent domain for subdomains
				parts := strings.Split(proxy.Domain, ".")
				if len(parts) > 2 {
					// It's a subdomain - check if wildcard cert exists for parent domain
					// Note: We only use parent cert if this domain doesn't have its own cert
					parentDomain := strings.Join(parts[1:], ".")
					wildcardCertPath := fmt.Sprintf("/etc/letsencrypt/live/%s/fullchain.pem", parentDomain)
					if _, err := os.Stat(wildcardCertPath); err == nil {
						// Parent domain has a cert - use it (assuming it's a wildcard)
						// TODO: Ideally verify it's actually a wildcard cert covering this subdomain
						certPath = wildcardCertPath
						keyPath = fmt.Sprintf("/etc/letsencrypt/live/%s/privkey.pem", parentDomain)
					}
				}
				// If still no cert found, default to exact domain path (will fail if cert doesn't exist)
				if certPath == "" {
					certPath = exactCertPath
					keyPath = exactKeyPath
				}
			}
		}

		config += fmt.Sprintf("    ssl_certificate %s;\n", certPath)
		config += fmt.Sprintf("    ssl_certificate_key %s;\n\n", keyPath)

		config += "    ssl_protocols TLSv1.2 TLSv1.3;\n"
		config += "    ssl_prefer_server_ciphers on;\n"
		config += "    ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384;\n\n"

		// Security headers
		config += generateSecurityHeaders(headers)

		// Check if this is a redirect proxy
		if proxy.ProxyType == "redirect" && proxy.RedirectURL != nil && *proxy.RedirectURL != "" {
			config += generateRedirectLocationBlock(*proxy.RedirectURL, proxy.RedirectCode)
		} else {
			// Docker DNS resolver: defers upstream hostname resolution to request time
			// so nginx starts cleanly even when the upstream container is stopped.
			config += "    resolver 127.0.0.11 valid=30s ipv6=off;\n"
			config += "    root /var/www/html;\n"
			config += "    error_page 502 503 504 /502.html;\n"
			config += "    location = /502.html { internal; }\n\n"
			config += generateLocationBlockWithAuth(proxy.UpstreamTarget, proxy.BasicAuthEnabled, proxy.BasicAuthRealm, proxy.Domain, proxy.BasicAuthExcludedPaths, proxy.BearerAuthEnabled, bearerTokens)
		}
		config += "}\n"
	}

	return config
}

// generateRedirectLocationBlock creates a location block that redirects to an external URL
func generateRedirectLocationBlock(redirectURL string, redirectCode int) string {
	var config string

	// Default to 301 if no valid code provided
	if redirectCode == 0 {
		redirectCode = 301
	}

	config += "    location / {\n"
	// Check if redirect URL already has $request_uri or variables
	if strings.Contains(redirectURL, "$") {
		config += fmt.Sprintf("        return %d %s;\n", redirectCode, redirectURL)
	} else {
		// Append $request_uri to preserve the path
		config += fmt.Sprintf("        return %d %s$request_uri;\n", redirectCode, redirectURL)
	}
	config += "    }\n"

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
	return generateLocationBlockWithAuth(upstream, false, "", "", nil, false, nil)
}

func generateLocationBlockWithAuth(upstream string, basicAuthEnabled bool, basicAuthRealm, domain string, excludedPaths []string, bearerAuthEnabled bool, bearerTokens []string) string {
	var config string

	// Generate location blocks for excluded paths (no auth)
	if basicAuthEnabled && len(excludedPaths) > 0 {
		for _, path := range excludedPaths {
			// Ensure path starts with /
			if !strings.HasPrefix(path, "/") {
				path = "/" + path
			}
			config += fmt.Sprintf("    location %s {\n", path)
			config += "        auth_basic off;\n"
			// Use variable-based upstream so nginx resolves at request time,
			// not at startup — prevents nginx crash when container is stopped.
			config += fmt.Sprintf("        set $upstream \"%s\";\n", upstream)
			config += "        proxy_pass $upstream;\n"
			config += "        proxy_http_version 1.1;\n"
			config += "        proxy_set_header Host $host;\n"
			config += "        proxy_set_header X-Real-IP $remote_addr;\n"
			config += "        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n"
			config += "        proxy_set_header X-Forwarded-Proto $scheme;\n"
			config += "        proxy_set_header Upgrade $http_upgrade;\n"
			config += "        proxy_set_header Connection \"upgrade\";\n"
			config += "    }\n\n"
		}
	}

	// Main location block
	config += "    location / {\n"

	// Add basic auth if enabled
	if basicAuthEnabled {
		realm := basicAuthRealm
		if realm == "" {
			realm = "Restricted"
		}
		config += fmt.Sprintf("        auth_basic \"%s\";\n", realm)
		config += fmt.Sprintf("        auth_basic_user_file /data/nginx/conf.d/.htpasswd_%s;\n\n", sanitizeFilename(domain))
	}

	// Bearer token auth: nginx has no built-in directive to check a header against a
	// list of valid values (that's what `map` is for, but map is only legal at the
	// http{} level, not inside this per-domain server/location block), so this checks
	// the Authorization header against each token directly. If Basic Auth is also
	// enabled, both guards are simply present -- a request must satisfy both.
	if bearerAuthEnabled && len(bearerTokens) > 0 {
		config += "        set $bearer_token_valid 0;\n"
		for _, t := range bearerTokens {
			config += fmt.Sprintf("        if ($http_authorization = \"Bearer %s\") { set $bearer_token_valid 1; }\n", t)
		}
		config += "        if ($bearer_token_valid = 0) { return 401; }\n\n"
	}

	// Use variable-based upstream so nginx resolves at request time,
	// not at startup — prevents nginx crash when container is stopped.
	config += fmt.Sprintf("        set $upstream \"%s\";\n", upstream)
	config += "        proxy_pass $upstream;\n"
	config += "        proxy_http_version 1.1;\n"
	config += "        proxy_set_header Host $host;\n"
	config += "        proxy_set_header X-Real-IP $remote_addr;\n"
	config += "        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n"
	config += "        proxy_set_header X-Forwarded-Proto $scheme;\n"
	config += "        proxy_set_header Upgrade $http_upgrade;\n"
	config += "        proxy_set_header Connection \"upgrade\";\n"
	config += "        proxy_connect_timeout 60s;\n"
	config += "        proxy_send_timeout 60s;\n"
	config += "        proxy_read_timeout 60s;\n"
	// Intercept upstream errors so nginx serves the custom error page (502.html)
	// instead of the raw nginx error when the upstream container is down.
	config += "        proxy_intercept_errors on;\n"
	config += "    }\n"
	return config
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

	if result, err := resp.GetCommandResult(); err == nil && result != nil {
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

	if result, err := resp.GetCommandResult(); err == nil && result != nil {
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
func (h *Handler) dispatchProxyConfig(ctx context.Context, agentID, proxyID uuid.UUID, domain, upstream string, forceSSL, http2, includeWWW, sslEnabled bool) {
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

	// Fetch basic auth settings
	var basicAuthEnabled, bearerAuthEnabled bool
	var basicAuthRealm string
	var basicAuthExcludedPaths []string
	h.db.QueryRow(ctx, `
		SELECT COALESCE(basic_auth_enabled, false), COALESCE(basic_auth_realm, 'Restricted'),
		       COALESCE(basic_auth_excluded_paths, '{}'), COALESCE(bearer_auth_enabled, false)
		FROM proxy_hosts WHERE id = $1
	`, proxyID).Scan(&basicAuthEnabled, &basicAuthRealm, &basicAuthExcludedPaths, &bearerAuthEnabled)

	bearerTokens, err := h.decryptBearerTokens(ctx, proxyID)
	if err != nil {
		h.logger.Error("Failed to decrypt bearer tokens", zap.Error(err))
	}

	// Build proxy host for config generation
	proxy := ProxyHost{
		Domain:                 domain,
		UpstreamTarget:         upstream,
		ForceSSL:               forceSSL,
		HTTP2Enabled:           http2,
		IncludeWWW:             includeWWW,
		SSLEnabled:             sslEnabled,
		BasicAuthEnabled:       basicAuthEnabled,
		BasicAuthRealm:         basicAuthRealm,
		BasicAuthExcludedPaths: basicAuthExcludedPaths,
		BearerAuthEnabled:      bearerAuthEnabled,
	}

	// Generate nginx config
	config := generateNginxConfig(proxy, headers, bearerTokens)

	// Build config path
	configPath := filepath.Join("/data/nginx/conf.d", sanitizeFilename(domain)+".conf")

	// Create command with config content
	cmdPayload, _ := json.Marshal(agentgrpc.NginxCommand{
		Action:        agentgrpc.NginxActionWriteConfig,
		ConfigContent: config,
		ConfigPath:    configPath,
	})

	cmd := &agentgrpc.BackendMessage{
		RequestId: uuid.New().String(),
		Type:      "nginx",
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

// dispatchProxyConfigFull sends nginx config to the agent with full proxy details including redirect support
func (h *Handler) dispatchProxyConfigFull(ctx context.Context, agentID, proxyID uuid.UUID, domain, upstream, proxyType string, redirectURL *string, redirectCode *int, forceSSL, http2, includeWWW, sslEnabled bool) {
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

	// Fetch basic auth settings
	var basicAuthEnabled, bearerAuthEnabled bool
	var basicAuthRealm string
	var basicAuthExcludedPaths []string
	h.db.QueryRow(ctx, `
		SELECT COALESCE(basic_auth_enabled, false), COALESCE(basic_auth_realm, 'Restricted'),
		       COALESCE(basic_auth_excluded_paths, '{}'), COALESCE(bearer_auth_enabled, false)
		FROM proxy_hosts WHERE id = $1
	`, proxyID).Scan(&basicAuthEnabled, &basicAuthRealm, &basicAuthExcludedPaths, &bearerAuthEnabled)

	bearerTokens, err := h.decryptBearerTokens(ctx, proxyID)
	if err != nil {
		h.logger.Error("Failed to decrypt bearer tokens", zap.Error(err))
	}

	// Default proxy type to upstream if empty
	if proxyType == "" {
		proxyType = "upstream"
	}

	// Default redirect code if not provided
	code := 301
	if redirectCode != nil {
		code = *redirectCode
	}

	// Build proxy host for config generation
	proxy := ProxyHost{
		Domain:                 domain,
		UpstreamTarget:         upstream,
		ProxyType:              proxyType,
		RedirectURL:            redirectURL,
		RedirectCode:           code,
		ForceSSL:               forceSSL,
		HTTP2Enabled:           http2,
		IncludeWWW:             includeWWW,
		SSLEnabled:             sslEnabled,
		BasicAuthEnabled:       basicAuthEnabled,
		BasicAuthRealm:         basicAuthRealm,
		BasicAuthExcludedPaths: basicAuthExcludedPaths,
		BearerAuthEnabled:      bearerAuthEnabled,
	}

	// Generate nginx config
	config := generateNginxConfig(proxy, headers, bearerTokens)

	// Build config path
	configPath := filepath.Join("/data/nginx/conf.d", sanitizeFilename(domain)+".conf")

	// Create command with config content
	cmdPayload, _ := json.Marshal(agentgrpc.NginxCommand{
		Action:        agentgrpc.NginxActionWriteConfig,
		ConfigContent: config,
		ConfigPath:    configPath,
	})

	cmd := &agentgrpc.BackendMessage{
		RequestId: uuid.New().String(),
		Type:      "nginx",
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
		zap.String("proxy_type", proxyType),
	)
}

// redispatchProxyConfig re-reads a proxy's own settings from the DB and regenerates/
// redeploys its full nginx config. Used by changes (like a bearer token add/remove)
// that don't touch domain/upstream/SSL themselves but still need to be reflected in the
// generated config -- unlike Basic Auth, bearer tokens have no separate file of their
// own to write, so there's nothing beyond "regenerate the config" to dispatch.
func (h *Handler) redispatchProxyConfig(ctx context.Context, agentID, proxyID uuid.UUID) {
	var domain, upstreamTarget, proxyType string
	var redirectURL *string
	var redirectCode int
	var forceSSL, http2Enabled, includeWWW, sslEnabled bool
	err := h.db.QueryRow(ctx, `
		SELECT domain, upstream_target, COALESCE(proxy_type, 'upstream'), redirect_url,
		       COALESCE(redirect_code, 301), force_ssl, http2_enabled, COALESCE(include_www, false), ssl_enabled
		FROM proxy_hosts WHERE id = $1
	`, proxyID).Scan(&domain, &upstreamTarget, &proxyType, &redirectURL, &redirectCode, &forceSSL, &http2Enabled, &includeWWW, &sslEnabled)
	if err != nil {
		h.logger.Error("Failed to fetch proxy for redispatch", zap.Error(err), zap.String("proxy_id", proxyID.String()))
		return
	}

	h.dispatchProxyConfigFull(ctx, agentID, proxyID, domain, upstreamTarget, proxyType, redirectURL, &redirectCode, forceSSL, http2Enabled, includeWWW, sslEnabled)
}

// dispatchProxyConfigWithCert sends nginx config with custom certificate paths to the agent
func (h *Handler) dispatchProxyConfigWithCert(ctx context.Context, agentID, proxyID uuid.UUID, domain, upstream string, forceSSL, http2, sslEnabled bool, certPath, keyPath string) {
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
		SSLCertPath:    &certPath,
		SSLKeyPath:     &keyPath,
	}

	// Generate nginx config with custom cert paths. Note: like Basic Auth, this
	// wildcard-SSL-cert path doesn't carry Bearer Auth settings through either --
	// a pre-existing gap for Basic Auth, not a new one introduced here.
	config := generateNginxConfig(proxy, headers, nil)

	// Build config path
	configPath := filepath.Join("/data/nginx/conf.d", sanitizeFilename(domain)+".conf")

	// Create command with config content
	cmdPayload, _ := json.Marshal(agentgrpc.NginxCommand{
		Action:        agentgrpc.NginxActionWriteConfig,
		ConfigContent: config,
		ConfigPath:    configPath,
	})

	cmd := &agentgrpc.BackendMessage{
		RequestId: uuid.New().String(),
		Type:      "nginx",
		Command:   cmdPayload,
	}

	// Send command (non-blocking)
	if err := agentgrpc.SendCommandAsync(agentIDStr, cmd); err != nil {
		h.logger.Error("Failed to dispatch proxy config with cert",
			zap.Error(err),
			zap.String("domain", domain),
		)
		return
	}

	h.logger.Info("Dispatched proxy config with custom cert to agent",
		zap.String("agent_id", agentIDStr),
		zap.String("domain", domain),
		zap.String("cert_path", certPath),
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
	configPath := filepath.Join("/data/nginx/conf.d", sanitizeFilename(domain)+".conf")

	// Create delete command (write empty config will fail validation, so we use a different approach)
	// The agent should delete the file and reload nginx
	cmdPayload, _ := json.Marshal(map[string]interface{}{
		"action":      "delete_config",
		"config_path": configPath,
		"domain":      domain,
	})

	cmd := &agentgrpc.BackendMessage{
		RequestId: uuid.New().String(),
		Type:      "nginx",
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
func (h *Handler) dispatchSSLRequest(agentID uuid.UUID, domain, email, dnsProvider string, includeWWW bool) {
	agentIDStr := agentID.String()

	if !agentgrpc.IsAgentConnected(agentIDStr) {
		h.logger.Warn("Agent not connected, cannot dispatch SSL request",
			zap.String("agent_id", agentIDStr),
			zap.String("domain", domain),
		)
		return
	}

	cmd := agentgrpc.NewNginxSSLCommandWithWWW(domain, email, dnsProvider, includeWWW)

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
		zap.Bool("include_www", includeWWW),
	)
}

// dispatchHtpasswd sends htpasswd file update to the agent
func (h *Handler) dispatchHtpasswd(ctx context.Context, agentID, proxyID uuid.UUID) {
	agentIDStr := agentID.String()

	if !agentgrpc.IsAgentConnected(agentIDStr) {
		h.logger.Warn("Agent not connected, cannot dispatch htpasswd",
			zap.String("agent_id", agentIDStr),
		)
		return
	}

	// Get proxy details for htpasswd filename
	var domain string
	var upstreamTarget string
	var basicAuthEnabled bool
	var basicAuthRealm string
	var isSystemProxy bool
	var sslEnabled, forceSSL, http2Enabled, includeWWW bool
	var sslCertPath, sslKeyPath *string
	err := h.db.QueryRow(ctx, `
		SELECT domain, upstream_target, COALESCE(basic_auth_enabled, false), COALESCE(basic_auth_realm, 'Restricted'),
		       COALESCE(is_system_proxy, false), ssl_enabled, force_ssl, http2_enabled, COALESCE(include_www, false), ssl_cert_path, ssl_key_path
		FROM proxy_hosts
		WHERE id = $1
	`, proxyID).Scan(&domain, &upstreamTarget, &basicAuthEnabled, &basicAuthRealm, &isSystemProxy, &sslEnabled, &forceSSL, &http2Enabled, &includeWWW, &sslCertPath, &sslKeyPath)

	if err != nil {
		h.logger.Error("Failed to fetch proxy for htpasswd dispatch", zap.Error(err))
		return
	}

	h.logger.Info("dispatchHtpasswd: fetched proxy details",
		zap.String("proxy_id", proxyID.String()),
		zap.String("domain", domain),
		zap.Bool("basic_auth_enabled", basicAuthEnabled),
		zap.Bool("is_system_proxy", isSystemProxy),
	)

	// Build htpasswd file path
	htpasswdPath := filepath.Join("/data/nginx/conf.d", ".htpasswd_"+sanitizeFilename(domain))

	// If basic auth is disabled or no users, delete htpasswd file
	if !basicAuthEnabled {
		cmd := agentgrpc.NewNginxDeleteHtpasswdCommand(htpasswdPath)
		if err := agentgrpc.SendCommandAsync(agentIDStr, cmd); err != nil {
			h.logger.Error("Failed to dispatch htpasswd delete",
				zap.Error(err),
				zap.String("domain", domain),
			)
		}
		// For system proxies, regenerate the nginx config without basic auth
		if isSystemProxy {
			certPath := ""
			keyPath := ""
			if sslCertPath != nil {
				certPath = *sslCertPath
			}
			if sslKeyPath != nil {
				keyPath = *sslKeyPath
			}
			h.dispatchInfraPilotProxyConfigWithCert(ctx, agentID, proxyID, domain, forceSSL, http2Enabled, sslEnabled, certPath, keyPath, false, "", "")
			h.logger.Info("Regenerated system proxy nginx config without basic auth",
				zap.String("domain", domain),
			)
		} else {
			// For regular proxies, regenerate nginx config without basic auth
			h.dispatchProxyConfig(ctx, agentID, proxyID, domain, upstreamTarget, forceSSL, http2Enabled, includeWWW, sslEnabled)
			h.logger.Info("Regenerated regular proxy nginx config without basic auth",
				zap.String("domain", domain),
			)
		}
		return
	}

	// Generate htpasswd content
	htpasswdContent, err := h.generateHtpasswd(ctx, proxyID)
	if err != nil {
		h.logger.Error("Failed to generate htpasswd", zap.Error(err))
		return
	}

	// If no users, delete htpasswd file
	if htpasswdContent == "" {
		cmd := agentgrpc.NewNginxDeleteHtpasswdCommand(htpasswdPath)
		if err := agentgrpc.SendCommandAsync(agentIDStr, cmd); err != nil {
			h.logger.Error("Failed to dispatch htpasswd delete",
				zap.Error(err),
				zap.String("domain", domain),
			)
		}
		return
	}

	// Send htpasswd write command
	cmd := agentgrpc.NewNginxWriteHtpasswdCommand(htpasswdContent, htpasswdPath)
	if err := agentgrpc.SendCommandAsync(agentIDStr, cmd); err != nil {
		h.logger.Error("Failed to dispatch htpasswd write",
			zap.Error(err),
			zap.String("domain", domain),
		)
		return
	}

	h.logger.Info("Dispatched htpasswd to agent",
		zap.String("agent_id", agentIDStr),
		zap.String("domain", domain),
	)

	// For system proxies, also regenerate the nginx config to include/update basic auth directives
	if isSystemProxy {
		certPath := ""
		keyPath := ""
		if sslCertPath != nil {
			certPath = *sslCertPath
		}
		if sslKeyPath != nil {
			keyPath = *sslKeyPath
		}
		h.dispatchInfraPilotProxyConfigWithCert(ctx, agentID, proxyID, domain, forceSSL, http2Enabled, sslEnabled, certPath, keyPath, basicAuthEnabled, basicAuthRealm, htpasswdPath)
		h.logger.Info("Regenerated system proxy nginx config with basic auth",
			zap.String("domain", domain),
			zap.Bool("basic_auth_enabled", basicAuthEnabled),
		)
	} else {
		// For regular proxies, regenerate nginx config with basic auth
		h.dispatchProxyConfig(ctx, agentID, proxyID, domain, upstreamTarget, forceSSL, http2Enabled, includeWWW, sslEnabled)
		h.logger.Info("Regenerated regular proxy nginx config with basic auth",
			zap.String("domain", domain),
			zap.Bool("basic_auth_enabled", basicAuthEnabled),
		)
	}
}

// sanitizeFilename sanitizes a domain name for use in a filename
func sanitizeFilename(domain string) string {
	return strings.ReplaceAll(domain, ".", "_")
}

// testNetworkConnectivity tests if a container:port is accessible from the agent
// POST /agents/:id/proxies/test-network
func (h *Handler) testNetworkConnectivity(c *gin.Context) {
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

	var req TestNetworkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate port range
	if req.Port < 1 || req.Port > 65535 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid port number"})
		return
	}

	// Check if agent is connected
	if !agentgrpc.IsAgentConnected(agentIDStr) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "agent is not connected"})
		return
	}

	// Create network test command
	cmdPayload, _ := json.Marshal(map[string]interface{}{
		"action":         "network_test",
		"container_name": req.ContainerName,
		"port":           req.Port,
	})

	cmd := &agentgrpc.BackendMessage{
		RequestId: uuid.New().String(),
		Type:      "network",
		Command:   cmdPayload,
	}

	// Send command and wait for response. The agent's own fallback port scan (see
	// HandleNetworkTest) can take several seconds even after being parallelized, and
	// this budget matches the other agent-command timeouts elsewhere in this file
	// (30s) rather than the 10s this used to have, which was tight enough that a
	// slightly slow first dial (e.g. against host.docker.internal, whose resolution
	// timing differs from a container name) could blow the deadline before the
	// agent's real answer ever arrived, producing a generic 500 instead of the
	// actual reachable/unreachable result.
	resp, err := agentgrpc.SendCommand(agentIDStr, cmd, 30*time.Second)
	if err != nil {
		h.logger.Error("Failed to test network connectivity",
			zap.Error(err),
			zap.String("container", req.ContainerName),
			zap.Int("port", req.Port),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to test network connectivity"})
		return
	}

	// Parse command result from response
	result, err := resp.GetCommandResult()
	if err != nil {
		h.logger.Error("Failed to parse network test result", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse response"})
		return
	}

	// Build response
	response := TestNetworkResponse{
		Reachable: result.Success,
		Message:   result.Message,
	}

	// If data contains available ports, extract them
	if result.Data != nil {
		var data struct {
			AvailablePorts []int `json:"available_ports"`
		}
		if err := json.Unmarshal(result.Data, &data); err == nil && len(data.AvailablePorts) > 0 {
			response.AvailablePorts = data.AvailablePorts
		}
	}

	c.JSON(http.StatusOK, response)
}
