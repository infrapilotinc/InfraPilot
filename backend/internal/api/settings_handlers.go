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

// SystemSetting represents a system setting
type SystemSetting struct {
	ID           uuid.UUID       `json:"id"`
	OrgID        uuid.UUID       `json:"org_id"`
	SettingKey   string          `json:"setting_key"`
	SettingValue json.RawMessage `json:"setting_value"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// InfraPilotDomainSettings represents the InfraPilot domain configuration
type InfraPilotDomainSettings struct {
	Domain           string `json:"domain"`
	SSLEnabled       bool   `json:"ssl_enabled"`
	ForceSSL         bool   `json:"force_ssl"`
	HTTP2Enabled     bool   `json:"http2_enabled"`
	ProxyHostID      string `json:"proxy_host_id,omitempty"`
	Status           string `json:"status,omitempty"` // active, pending, error
	SSLSource        string `json:"ssl_source,omitempty"`        // 'letsencrypt', 'wildcard', 'external'
	SSLCertificateID string `json:"ssl_certificate_id,omitempty"` // Reference to ssl_certificates
	SSLCertPath      string `json:"ssl_cert_path,omitempty"`
	SSLKeyPath       string `json:"ssl_key_path,omitempty"`
}

// getSystemSettings returns all system settings
func (h *Handler) getSystemSettings(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)

	rows, err := h.db.Query(c.Request.Context(), `
		SELECT id, org_id, setting_key, setting_value, created_at, updated_at
		FROM system_settings
		WHERE org_id = $1
		ORDER BY setting_key
	`, orgID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch settings"})
		return
	}
	defer rows.Close()

	settings := map[string]json.RawMessage{}
	for rows.Next() {
		var s SystemSetting
		if err := rows.Scan(&s.ID, &s.OrgID, &s.SettingKey, &s.SettingValue, &s.CreatedAt, &s.UpdatedAt); err != nil {
			continue
		}
		settings[s.SettingKey] = s.SettingValue
	}

	c.JSON(http.StatusOK, settings)
}

// getInfraPilotDomain returns the InfraPilot domain settings
func (h *Handler) getInfraPilotDomain(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)

	var settingValue json.RawMessage
	err := h.db.QueryRow(c.Request.Context(), `
		SELECT setting_value
		FROM system_settings
		WHERE org_id = $1 AND setting_key = 'infrapilot_domain'
	`, orgID).Scan(&settingValue)

	if err != nil {
		// Return empty settings if not configured
		c.JSON(http.StatusOK, InfraPilotDomainSettings{})
		return
	}

	var settings InfraPilotDomainSettings
	if err := json.Unmarshal(settingValue, &settings); err != nil {
		c.JSON(http.StatusOK, InfraPilotDomainSettings{})
		return
	}

	c.JSON(http.StatusOK, settings)
}

// UpdateInfraPilotDomainRequest is the request body for updating InfraPilot domain
type UpdateInfraPilotDomainRequest struct {
	Domain           string  `json:"domain" binding:"required"`
	SSLEnabled       bool    `json:"ssl_enabled"`
	ForceSSL         bool    `json:"force_ssl"`
	HTTP2Enabled     bool    `json:"http2_enabled"`
	SSLSource        string  `json:"ssl_source"`         // 'letsencrypt', 'wildcard', 'external'
	SSLCertificateID *string `json:"ssl_certificate_id"` // Reference to ssl_certificates (for wildcard/external)
	SSLCertPath      *string `json:"ssl_cert_path"`      // Direct cert path (for scanned/unregistered certs)
	SSLKeyPath       *string `json:"ssl_key_path"`       // Direct key path (for scanned/unregistered certs)
}

// updateInfraPilotDomain updates the InfraPilot domain configuration
func (h *Handler) updateInfraPilotDomain(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	userID := c.MustGet("user_id").(uuid.UUID)

	var req UpdateInfraPilotDomainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate domain format
	if req.Domain != "" && !isValidDomain(req.Domain) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid domain format"})
		return
	}

	// Get or create the default agent
	var agentID uuid.UUID
	err := h.db.QueryRow(c.Request.Context(), `
		SELECT id FROM agents WHERE org_id = $1 AND status = 'active' ORDER BY created_at LIMIT 1
	`, orgID).Scan(&agentID)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no active agent found"})
		return
	}

	// Check if there's an existing system proxy for this agent
	var existingProxyID uuid.UUID
	var existingDomain string
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT id, domain FROM proxy_hosts WHERE agent_id = $1 AND is_system_proxy = TRUE
	`, agentID).Scan(&existingProxyID, &existingDomain)

	// Get certificate paths if using external/wildcard SSL
	var certPath, keyPath string
	sslSource := req.SSLSource
	if sslSource == "" {
		sslSource = "letsencrypt" // Default
	}

	// First, check if direct paths were provided (for scanned/unregistered certs)
	if req.SSLCertPath != nil && *req.SSLCertPath != "" {
		certPath = *req.SSLCertPath
	}
	if req.SSLKeyPath != nil && *req.SSLKeyPath != "" {
		keyPath = *req.SSLKeyPath
	}

	// If no direct paths but certificate ID provided, look up from database
	if certPath == "" && req.SSLCertificateID != nil && *req.SSLCertificateID != "" && (sslSource == "wildcard" || sslSource == "external") {
		certID, err := uuid.Parse(*req.SSLCertificateID)
		if err == nil {
			err = h.db.QueryRow(c.Request.Context(), `
				SELECT cert_path, key_path FROM ssl_certificates WHERE id = $1 AND org_id = $2
			`, certID, orgID).Scan(&certPath, &keyPath)
			if err != nil {
				h.logger.Warn("Could not find SSL certificate in database, using direct paths if available",
					zap.String("cert_id", *req.SSLCertificateID), zap.Error(err))
			}
		}
	}

	// Start transaction
	tx, err := h.db.Begin(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to start transaction"})
		return
	}
	defer tx.Rollback(c.Request.Context())

	var proxyID uuid.UUID

	// Prepare nullable cert path values
	var certPathPtr, keyPathPtr *string
	if certPath != "" {
		certPathPtr = &certPath
	}
	if keyPath != "" {
		keyPathPtr = &keyPath
	}

	if existingProxyID != uuid.Nil {
		// Update existing proxy
		if existingDomain != req.Domain {
			// Domain changed, need to delete old config
			go h.dispatchDeleteProxy(agentID, existingDomain)
		}

		_, err = tx.Exec(c.Request.Context(), `
			UPDATE proxy_hosts
			SET domain = $1, force_ssl = $2, http2_enabled = $3,
			    ssl_cert_path = $4, ssl_key_path = $5, ssl_source = $6, updated_at = NOW()
			WHERE id = $7
		`, req.Domain, req.ForceSSL, req.HTTP2Enabled, certPathPtr, keyPathPtr, sslSource, existingProxyID)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update proxy"})
			return
		}
		proxyID = existingProxyID
	} else {
		// Create new system proxy
		// Use special upstream that routes to frontend and backend
		upstream := "http://frontend:3000" // Will be handled specially in nginx config

		err = tx.QueryRow(c.Request.Context(), `
			INSERT INTO proxy_hosts (agent_id, domain, upstream_target, force_ssl, http2_enabled,
			                         ssl_cert_path, ssl_key_path, ssl_source, status, is_system_proxy)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'active', TRUE)
			RETURNING id
		`, agentID, req.Domain, upstream, req.ForceSSL, req.HTTP2Enabled, certPathPtr, keyPathPtr, sslSource).Scan(&proxyID)

		if err != nil {
			h.logger.Error("Failed to create system proxy", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create system proxy"})
			return
		}

		// Create default security headers
		_, err = tx.Exec(c.Request.Context(), `
			INSERT INTO proxy_security_headers (proxy_host_id)
			VALUES ($1)
		`, proxyID)
		if err != nil {
			h.logger.Warn("Failed to create security headers for system proxy", zap.Error(err))
		}
	}

	// Save settings
	settings := InfraPilotDomainSettings{
		Domain:           req.Domain,
		SSLEnabled:       req.SSLEnabled,
		ForceSSL:         req.ForceSSL,
		HTTP2Enabled:     req.HTTP2Enabled,
		ProxyHostID:      proxyID.String(),
		Status:           "active",
		SSLSource:        sslSource,
		SSLCertificateID: "",
		SSLCertPath:      certPath,
		SSLKeyPath:       keyPath,
	}

	if req.SSLCertificateID != nil {
		settings.SSLCertificateID = *req.SSLCertificateID
	}

	settingsJSON, _ := json.Marshal(settings)

	_, err = tx.Exec(c.Request.Context(), `
		INSERT INTO system_settings (org_id, setting_key, setting_value)
		VALUES ($1, 'infrapilot_domain', $2)
		ON CONFLICT (org_id, setting_key) DO UPDATE SET
			setting_value = EXCLUDED.setting_value,
			updated_at = NOW()
	`, orgID, settingsJSON)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save settings"})
		return
	}

	// Commit transaction
	if err := tx.Commit(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to commit transaction"})
		return
	}

	// Audit log
	h.auditLog(c, userID, orgID, "settings.infrapilot_domain.update", "system_settings", proxyID, req)

	// Fetch basic auth settings from proxy_hosts
	var basicAuthEnabled bool
	var basicAuthRealm string
	h.db.QueryRow(c.Request.Context(), `
		SELECT COALESCE(basic_auth_enabled, false), COALESCE(basic_auth_realm, 'Restricted')
		FROM proxy_hosts WHERE id = $1
	`, proxyID).Scan(&basicAuthEnabled, &basicAuthRealm)

	// Determine htpasswd path for this proxy (use sanitized domain)
	htpasswdPath := ""
	if basicAuthEnabled {
		htpasswdPath = fmt.Sprintf("/data/nginx/conf.d/.htpasswd_%s", strings.ReplaceAll(req.Domain, ".", "_"))
	}

	// Dispatch the special InfraPilot nginx config to the agent
	go h.dispatchInfraPilotProxyConfigWithCert(c.Request.Context(), agentID, proxyID, req.Domain, req.ForceSSL, req.HTTP2Enabled, req.SSLEnabled, certPath, keyPath, basicAuthEnabled, basicAuthRealm, htpasswdPath)

	// Update default.conf to serve welcome page for direct IP access
	go h.dispatchDefaultPageConfig(agentID, orgID, true)

	c.JSON(http.StatusOK, settings)
}

// deleteInfraPilotDomain removes the InfraPilot domain configuration
func (h *Handler) deleteInfraPilotDomain(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	userID := c.MustGet("user_id").(uuid.UUID)

	// Get current settings
	var settingValue json.RawMessage
	err := h.db.QueryRow(c.Request.Context(), `
		SELECT setting_value
		FROM system_settings
		WHERE org_id = $1 AND setting_key = 'infrapilot_domain'
	`, orgID).Scan(&settingValue)

	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "no configuration to delete"})
		return
	}

	var settings InfraPilotDomainSettings
	if err := json.Unmarshal(settingValue, &settings); err == nil && settings.Domain != "" {
		// Get agent ID and delete the proxy
		var agentID uuid.UUID
		err := h.db.QueryRow(c.Request.Context(), `
			SELECT agent_id FROM proxy_hosts WHERE id = $1
		`, settings.ProxyHostID).Scan(&agentID)

		if err == nil {
			// Delete the nginx config from agent
			go h.dispatchDeleteProxy(agentID, settings.Domain)

			// Delete the proxy host
			h.db.Exec(c.Request.Context(), `
				DELETE FROM proxy_hosts WHERE id = $1
			`, settings.ProxyHostID)
		}
	}

	// Delete the setting
	_, err = h.db.Exec(c.Request.Context(), `
		DELETE FROM system_settings WHERE org_id = $1 AND setting_key = 'infrapilot_domain'
	`, orgID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete settings"})
		return
	}

	// Get agent ID to reset default.conf
	var agentID uuid.UUID
	h.db.QueryRow(c.Request.Context(), `
		SELECT id FROM agents WHERE org_id = $1 AND status = 'active' ORDER BY created_at LIMIT 1
	`, orgID).Scan(&agentID)

	if agentID != uuid.Nil {
		// Reset default.conf to proxy to frontend (no domain configured)
		go h.dispatchDefaultPageConfig(agentID, orgID, false)
	}

	// Audit log
	h.auditLog(c, userID, orgID, "settings.infrapilot_domain.delete", "system_settings", uuid.Nil, nil)

	c.JSON(http.StatusOK, gin.H{"message": "domain configuration deleted"})
}

// dispatchInfraPilotProxyConfig sends the special InfraPilot nginx config to the agent (legacy, no custom cert)
func (h *Handler) dispatchInfraPilotProxyConfig(ctx interface{}, agentID, proxyID uuid.UUID, domain string, forceSSL, http2, sslEnabled bool) {
	h.dispatchInfraPilotProxyConfigWithCert(ctx, agentID, proxyID, domain, forceSSL, http2, sslEnabled, "", "", false, "", "")
}

// dispatchInfraPilotProxyConfigWithCert sends the special InfraPilot nginx config with optional custom cert paths
func (h *Handler) dispatchInfraPilotProxyConfigWithCert(ctx interface{}, agentID, proxyID uuid.UUID, domain string, forceSSL, http2, sslEnabled bool, certPath, keyPath string, basicAuthEnabled bool, basicAuthRealm, htpasswdPath string) {
	agentIDStr := agentID.String()

	if !agentgrpc.IsAgentConnected(agentIDStr) {
		h.logger.Warn("Agent not connected, cannot dispatch InfraPilot proxy config",
			zap.String("agent_id", agentIDStr),
			zap.String("domain", domain),
		)
		return
	}

	// Generate special InfraPilot nginx config that routes /api to backend
	config := generateInfraPilotNginxConfig(domain, forceSSL, http2, sslEnabled, certPath, keyPath, basicAuthEnabled, basicAuthRealm, htpasswdPath)

	// Build config path (same location as regular proxies)
	configPath := filepath.Join("/data/nginx/conf.d", domain+".conf")

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

	// Send command
	if err := agentgrpc.SendCommandAsync(agentIDStr, cmd); err != nil {
		h.logger.Error("Failed to dispatch InfraPilot proxy config",
			zap.Error(err),
			zap.String("domain", domain),
		)
		return
	}

	h.logger.Info("Dispatched InfraPilot proxy config to agent",
		zap.String("agent_id", agentIDStr),
		zap.String("domain", domain),
		zap.Bool("custom_cert", certPath != ""),
	)
}

// generateInfraPilotNginxConfig creates the special nginx config for InfraPilot's domain
// This routes /api/* to backend and everything else to frontend
// certPath and keyPath are optional - if empty, defaults to Let's Encrypt paths for the domain
// basicAuthEnabled, basicAuthRealm, and htpasswdPath control basic authentication
func generateInfraPilotNginxConfig(domain string, forceSSL, http2, sslEnabled bool, certPath, keyPath string, basicAuthEnabled bool, basicAuthRealm, htpasswdPath string) string {
	var config strings.Builder

	// If no cert path specified, determine the correct Let's Encrypt path
	// For subdomains, check if there's a wildcard cert for the parent domain
	effectiveCertPath := certPath
	effectiveKeyPath := keyPath
	if effectiveCertPath == "" && sslEnabled {
		// For subdomains, first check if wildcard cert exists for parent domain
		parts := strings.Split(domain, ".")
		if len(parts) > 2 {
			// It's a subdomain - check if wildcard cert exists for parent domain
			parentDomain := strings.Join(parts[1:], ".")
			wildcardCertPath := fmt.Sprintf("/etc/letsencrypt/live/%s/fullchain.pem", parentDomain)
			wildcardKeyPath := fmt.Sprintf("/etc/letsencrypt/live/%s/privkey.pem", parentDomain)
			// Check if wildcard cert exists on filesystem
			if _, err := os.Stat(wildcardCertPath); err == nil {
				// Wildcard cert exists - use it
				effectiveCertPath = wildcardCertPath
				effectiveKeyPath = wildcardKeyPath
			}
		}
		// If no wildcard cert found, only use exact domain path if cert actually exists
		if effectiveCertPath == "" {
			exactCertPath := fmt.Sprintf("/etc/letsencrypt/live/%s/fullchain.pem", domain)
			if _, err := os.Stat(exactCertPath); err == nil {
				effectiveCertPath = exactCertPath
				effectiveKeyPath = fmt.Sprintf("/etc/letsencrypt/live/%s/privkey.pem", domain)
			}
		}
	}

	config.WriteString("# Managed by InfraPilot - System Proxy\n")
	config.WriteString(fmt.Sprintf("# Domain: %s\n", domain))
	if certPath != "" {
		config.WriteString(fmt.Sprintf("# SSL: Custom certificate from %s\n", certPath))
	} else if effectiveCertPath != "" {
		config.WriteString(fmt.Sprintf("# SSL: Auto-detected certificate from %s\n", effectiveCertPath))
	}
	config.WriteString("\n")

	// HTTP server block
	config.WriteString("server {\n")
	config.WriteString("    listen 80;\n")
	config.WriteString("    listen [::]:80;\n")
	config.WriteString(fmt.Sprintf("    server_name %s;\n\n", domain))
	// Per-domain logging for analytics
	config.WriteString(fmt.Sprintf("    access_log /var/log/nginx/domains/%s.access.log infrapilot_analytics;\n", domain))
	config.WriteString(fmt.Sprintf("    error_log /var/log/nginx/domains/%s.error.log warn;\n\n", domain))

	if sslEnabled && forceSSL && effectiveCertPath != "" {
		// Cert exists: ACME challenge support for renewals + redirect to HTTPS
		config.WriteString("    # ACME challenge for Let's Encrypt (renewals)\n")
		config.WriteString("    location /.well-known/acme-challenge/ {\n")
		config.WriteString("        root /var/www/acme-challenge;\n")
		config.WriteString("    }\n\n")
		config.WriteString("    # Redirect all other HTTP to HTTPS\n")
		config.WriteString("    location / {\n")
		config.WriteString("        return 301 https://$host$request_uri;\n")
		config.WriteString("    }\n")
		config.WriteString("}\n\n")
	} else {
		if sslEnabled {
			// SSL requested but cert not yet available — serve ACME challenge for initial issuance
			config.WriteString("    # ACME challenge for Let's Encrypt (initial issuance)\n")
			config.WriteString("    location /.well-known/acme-challenge/ {\n")
			config.WriteString("        root /var/www/acme-challenge;\n")
			config.WriteString("    }\n\n")
		}
		writeInfraPilotLocations(&config, basicAuthEnabled, basicAuthRealm, htpasswdPath)
		config.WriteString("}\n\n")
	}

	// HTTPS server block (only if SSL enabled AND cert actually exists)
	if sslEnabled && effectiveCertPath != "" {
		config.WriteString("server {\n")
		config.WriteString("    listen 443 ssl;\n")
		config.WriteString("    listen [::]:443 ssl;\n")
		if http2 {
			config.WriteString("    http2 on;\n")
		}
		config.WriteString(fmt.Sprintf("    server_name %s;\n\n", domain))
		// Per-domain logging for analytics
		config.WriteString(fmt.Sprintf("    access_log /var/log/nginx/domains/%s.access.log infrapilot_analytics;\n", domain))
		config.WriteString(fmt.Sprintf("    error_log /var/log/nginx/domains/%s.error.log warn;\n\n", domain))

		// SSL configuration - use effective paths (custom, wildcard, or exact domain)
		config.WriteString(fmt.Sprintf("    ssl_certificate %s;\n", effectiveCertPath))
		config.WriteString(fmt.Sprintf("    ssl_certificate_key %s;\n\n", effectiveKeyPath))

		config.WriteString("    ssl_protocols TLSv1.2 TLSv1.3;\n")
		config.WriteString("    ssl_prefer_server_ciphers on;\n")
		config.WriteString("    ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384;\n")
		config.WriteString("    ssl_session_cache shared:SSL:10m;\n")
		config.WriteString("    ssl_session_timeout 1d;\n")
		config.WriteString("    ssl_session_tickets off;\n\n")

		// Security headers
		config.WriteString("    add_header Strict-Transport-Security \"max-age=31536000; includeSubDomains\" always;\n")
		config.WriteString("    add_header X-Frame-Options \"SAMEORIGIN\" always;\n")
		config.WriteString("    add_header X-Content-Type-Options \"nosniff\" always;\n")
		config.WriteString("    add_header X-XSS-Protection \"1; mode=block\" always;\n\n")

		writeInfraPilotLocations(&config, basicAuthEnabled, basicAuthRealm, htpasswdPath)
		config.WriteString("}\n")
	}

	return config.String()
}

// writeInfraPilotLocations writes the location blocks for InfraPilot
// Uses localhost addresses for single-container deployment mode
func writeInfraPilotLocations(config *strings.Builder, basicAuthEnabled bool, basicAuthRealm, htpasswdPath string) {
	// API routes to backend (no basic auth - uses JWT)
	config.WriteString("    # API routes to backend (JWT auth, no basic auth)\n")
	config.WriteString("    location /api/ {\n")
	if basicAuthEnabled {
		config.WriteString("        auth_basic off;\n")
	}
	config.WriteString("        proxy_pass http://127.0.0.1:8080;\n")
	config.WriteString("        proxy_http_version 1.1;\n")
	config.WriteString("        proxy_set_header Host $host;\n")
	config.WriteString("        proxy_set_header X-Real-IP $remote_addr;\n")
	config.WriteString("        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n")
	config.WriteString("        proxy_set_header X-Forwarded-Proto $scheme;\n")
	config.WriteString("        proxy_set_header Upgrade $http_upgrade;\n")
	config.WriteString("        proxy_set_header Connection \"upgrade\";\n")
	config.WriteString("        proxy_read_timeout 86400;\n")
	config.WriteString("        proxy_buffering off;\n")
	config.WriteString("    }\n\n")

	// Frontend (everything else)
	config.WriteString("    # Frontend\n")
	config.WriteString("    location / {\n")
	if basicAuthEnabled && htpasswdPath != "" {
		config.WriteString(fmt.Sprintf("        auth_basic \"%s\";\n", basicAuthRealm))
		config.WriteString(fmt.Sprintf("        auth_basic_user_file %s;\n", htpasswdPath))
	}
	config.WriteString("        proxy_pass http://127.0.0.1:3000;\n")
	config.WriteString("        proxy_http_version 1.1;\n")
	config.WriteString("        proxy_set_header Host $host;\n")
	config.WriteString("        proxy_set_header X-Real-IP $remote_addr;\n")
	config.WriteString("        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n")
	config.WriteString("        proxy_set_header X-Forwarded-Proto $scheme;\n")
	config.WriteString("        proxy_set_header Upgrade $http_upgrade;\n")
	config.WriteString("        proxy_set_header Connection \"upgrade\";\n")
	config.WriteString("    }\n\n")

	// ACME challenge for Let's Encrypt (no auth)
	config.WriteString("    # ACME challenge for Let's Encrypt\n")
	config.WriteString("    location /.well-known/acme-challenge/ {\n")
	if basicAuthEnabled {
		config.WriteString("        auth_basic off;\n")
	}
	config.WriteString("        root /var/www/acme-challenge;\n")
	config.WriteString("    }\n")
}

// isValidDomain validates a domain name
func isValidDomain(domain string) bool {
	// Basic validation: non-empty, no spaces, contains at least one dot for real domains
	if domain == "" || strings.Contains(domain, " ") {
		return false
	}
	// Allow localhost for development
	if domain == "localhost" {
		return true
	}
	// Basic check for domain format
	parts := strings.Split(domain, ".")
	if len(parts) < 2 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
	}
	return true
}

// dispatchDefaultPageConfig sends the welcome page HTML and updates default.conf
// to serve it for direct IP access when a domain is configured
func (h *Handler) dispatchDefaultPageConfig(agentID uuid.UUID, orgID uuid.UUID, domainConfigured bool) {
	agentIDStr := agentID.String()

	if !agentgrpc.IsAgentConnected(agentIDStr) {
		h.logger.Warn("Agent not connected, cannot dispatch default page config",
			zap.String("agent_id", agentIDStr),
		)
		return
	}

	// Get the welcome page content from database or use default
	welcomeHTML := h.getWelcomePageHTML(orgID)

	// Write welcome.html to nginx's html directory
	htmlPayload, _ := json.Marshal(agentgrpc.NginxCommand{
		Action:        agentgrpc.NginxActionWriteConfig,
		ConfigContent: welcomeHTML,
		ConfigPath:    "/var/www/html/welcome.html",
	})
	htmlCmd := &agentgrpc.BackendMessage{
		RequestId: uuid.New().String(),
		Type:      "nginx",
		Command:   htmlPayload,
	}

	if err := agentgrpc.SendCommandAsync(agentIDStr, htmlCmd); err != nil {
		h.logger.Error("Failed to dispatch welcome page HTML", zap.Error(err))
		return
	}

	// Write 502.html — served by nginx when any upstream container is down.
	// Uses the user-configured 502/maintenance page, or a built-in fallback.
	errorHTML := h.getErrorPageHTML(orgID)
	errorPayload, _ := json.Marshal(agentgrpc.NginxCommand{
		Action:        agentgrpc.NginxActionWriteConfig,
		ConfigContent: errorHTML,
		ConfigPath:    "/var/www/html/502.html",
	})
	errorCmd := &agentgrpc.BackendMessage{
		RequestId: uuid.New().String(),
		Type:      "nginx",
		Command:   errorPayload,
	}
	if err := agentgrpc.SendCommandAsync(agentIDStr, errorCmd); err != nil {
		h.logger.Error("Failed to dispatch error page HTML", zap.Error(err))
	}

	// Generate and dispatch updated default.conf
	defaultConf := generateDefaultNginxConfig(domainConfigured)
	confPayload, _ := json.Marshal(agentgrpc.NginxCommand{
		Action:        agentgrpc.NginxActionWriteConfig,
		ConfigContent: defaultConf,
		ConfigPath:    "/etc/nginx/conf.d/default.conf",
	})
	confCmd := &agentgrpc.BackendMessage{
		RequestId: uuid.New().String(),
		Type:      "nginx",
		Command:   confPayload,
	}

	if err := agentgrpc.SendCommandAsync(agentIDStr, confCmd); err != nil {
		h.logger.Error("Failed to dispatch default.conf", zap.Error(err))
		return
	}

	h.logger.Info("Dispatched default page config to agent",
		zap.String("agent_id", agentIDStr),
		zap.Bool("domain_configured", domainConfigured),
	)
}

// getErrorPageHTML returns the HTML to serve when an upstream is unavailable (502/503/504).
// Priority: enabled 502 page → enabled maintenance page → built-in default.
func (h *Handler) getErrorPageHTML(orgID uuid.UUID) string {
	ctx := context.Background()

	for _, pageType := range []string{"502", "maintenance"} {
		var page DefaultPage
		err := h.db.QueryRow(ctx, `
			SELECT page_type, enabled, title, heading, message, show_logo, custom_css
			FROM default_pages
			WHERE org_id = $1 AND page_type = $2
		`, orgID, pageType).Scan(
			&page.PageType, &page.Enabled, &page.Title, &page.Heading,
			&page.Message, &page.ShowLogo, &page.CustomCSS,
		)
		if err == nil && page.Enabled {
			return generateDefaultPageHTML(page)
		}
	}

	// Neither is configured/enabled — render the built-in 502 template.
	return generateDefaultPageHTML(defaultPageTemplates["502"])
}

// getWelcomePageHTML retrieves the welcome page HTML from database or returns default
func (h *Handler) getWelcomePageHTML(orgID uuid.UUID) string {
	var page struct {
		Enabled  bool
		Title    string
		Heading  string
		Message  string
		ShowLogo bool
	}

	ctx := context.Background()
	err := h.db.QueryRow(ctx, `
		SELECT enabled, title, heading, message, show_logo
		FROM default_pages
		WHERE org_id = $1 AND page_type = 'welcome'
	`, orgID).Scan(&page.Enabled, &page.Title, &page.Heading, &page.Message, &page.ShowLogo)

	if err != nil || !page.Enabled {
		// Use defaults
		page.Title = "Welcome"
		page.Heading = "Welcome"
		page.Message = "This site is being configured. Please check back soon."
		page.ShowLogo = true
	}

	return generateWelcomePageHTML(page.Title, page.Heading, page.Message, page.ShowLogo)
}

// generateWelcomePageHTML creates clean HTML for the welcome page
func generateWelcomePageHTML(title, heading, message string, showLogo bool) string {
	var b strings.Builder

	b.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>`)
	b.WriteString(title)
	b.WriteString(`</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            background: linear-gradient(135deg, #1a1a2e 0%, #16213e 100%);
            color: #fff;
            padding: 20px;
        }
        .container {
            text-align: center;
            max-width: 600px;
        }
        .heading {
            font-size: 4rem;
            font-weight: 700;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            background-clip: text;
            margin-bottom: 1rem;
        }
        .message {
            font-size: 1.25rem;
            color: rgba(255,255,255,0.7);
            line-height: 1.6;
        }
        .logo {
            width: 64px;
            height: 64px;
            margin-bottom: 2rem;
            opacity: 0.8;
        }
    </style>
</head>
<body>
    <div class="container">
`)

	if showLogo {
		b.WriteString(`        <svg class="logo" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
            <path d="M12 2L2 7L12 12L22 7L12 2Z" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
            <path d="M2 17L12 22L22 17" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
            <path d="M2 12L12 17L22 12" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
        </svg>
`)
	}

	b.WriteString(fmt.Sprintf(`        <h1 class="heading">%s</h1>
        <p class="message">%s</p>
    </div>
</body>
</html>`, heading, message))

	return b.String()
}

// generateDefaultNginxConfig creates the default.conf that handles IP access
// Uses localhost addresses for single-container deployment mode
// Note: /etc/nginx/sites/*.conf and /data/nginx/conf.d/*.conf are included from nginx.conf
func generateDefaultNginxConfig(domainConfigured bool) string {
	var config strings.Builder

	config.WriteString("# InfraPilot Base Nginx Configuration\n")
	config.WriteString("# Auto-generated - do not edit manually\n")
	config.WriteString("# Note: sites/*.conf and /data/nginx/conf.d/*.conf are included from nginx.conf\n\n")

	// HTTP default server
	config.WriteString("server {\n")
	config.WriteString("    listen 80 default_server;\n")
	config.WriteString("    listen [::]:80 default_server;\n")
	config.WriteString("    server_name _;\n\n")

	if domainConfigured {
		// When a domain is configured, serve welcome page for direct IP access
		config.WriteString("    # Domain is configured - serve welcome page for direct IP access\n")
		config.WriteString("    root /var/www/html;\n\n")
		config.WriteString("    location / {\n")
		config.WriteString("        try_files /welcome.html =404;\n")
		config.WriteString("    }\n")
	} else {
		// No domain configured - proxy to frontend (initial setup)
		config.WriteString("    # No domain configured - proxy to InfraPilot for initial setup\n")
		config.WriteString("    # API routes\n")
		config.WriteString("    location /api/ {\n")
		config.WriteString("        proxy_pass http://127.0.0.1:8080;\n")
		config.WriteString("        proxy_http_version 1.1;\n")
		config.WriteString("        proxy_set_header Host $host;\n")
		config.WriteString("        proxy_set_header X-Real-IP $remote_addr;\n")
		config.WriteString("        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n")
		config.WriteString("        proxy_set_header X-Forwarded-Proto $scheme;\n")
		config.WriteString("        proxy_set_header Upgrade $http_upgrade;\n")
		config.WriteString("        proxy_set_header Connection \"upgrade\";\n")
		config.WriteString("        proxy_read_timeout 86400;\n")
		config.WriteString("        proxy_buffering off;\n")
		config.WriteString("    }\n\n")

		config.WriteString("    # Frontend\n")
		config.WriteString("    location / {\n")
		config.WriteString("        proxy_pass http://127.0.0.1:3000;\n")
		config.WriteString("        proxy_http_version 1.1;\n")
		config.WriteString("        proxy_set_header Host $host;\n")
		config.WriteString("        proxy_set_header X-Real-IP $remote_addr;\n")
		config.WriteString("        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n")
		config.WriteString("        proxy_set_header X-Forwarded-Proto $scheme;\n")
		config.WriteString("        proxy_set_header Upgrade $http_upgrade;\n")
		config.WriteString("        proxy_set_header Connection \"upgrade\";\n")
		config.WriteString("    }\n\n")

		config.WriteString("    # Next.js HMR WebSocket\n")
		config.WriteString("    location /_next/webpack-hmr {\n")
		config.WriteString("        proxy_pass http://127.0.0.1:3000/_next/webpack-hmr;\n")
		config.WriteString("        proxy_http_version 1.1;\n")
		config.WriteString("        proxy_set_header Upgrade $http_upgrade;\n")
		config.WriteString("        proxy_set_header Connection \"upgrade\";\n")
		config.WriteString("    }\n")
	}

	config.WriteString("}\n\n")

	// HTTPS catch-all server - reject SSL handshake for unconfigured domains
	// This prevents certificate mismatch errors for domains not yet configured
	if domainConfigured {
		config.WriteString("# HTTPS catch-all - reject connections for unconfigured domains\n")
		config.WriteString("# This prevents certificate mismatch errors\n")
		config.WriteString("server {\n")
		config.WriteString("    listen 443 ssl default_server;\n")
		config.WriteString("    listen [::]:443 ssl default_server;\n")
		config.WriteString("    server_name _;\n\n")
		config.WriteString("    # Reject SSL handshake for unconfigured domains\n")
		config.WriteString("    ssl_reject_handshake on;\n")
		config.WriteString("}\n")
	}

	return config.String()
}
