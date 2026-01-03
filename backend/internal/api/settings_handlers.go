package api

import (
	"encoding/json"
	"fmt"
	"net/http"
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
	Domain       string `json:"domain"`
	SSLEnabled   bool   `json:"ssl_enabled"`
	ForceSSL     bool   `json:"force_ssl"`
	HTTP2Enabled bool   `json:"http2_enabled"`
	ProxyHostID  string `json:"proxy_host_id,omitempty"`
	Status       string `json:"status,omitempty"` // active, pending, error
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
	Domain       string `json:"domain" binding:"required"`
	SSLEnabled   bool   `json:"ssl_enabled"`
	ForceSSL     bool   `json:"force_ssl"`
	HTTP2Enabled bool   `json:"http2_enabled"`
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

	// Start transaction
	tx, err := h.db.Begin(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to start transaction"})
		return
	}
	defer tx.Rollback(c.Request.Context())

	var proxyID uuid.UUID

	if existingProxyID != uuid.Nil {
		// Update existing proxy
		if existingDomain != req.Domain {
			// Domain changed, need to delete old config
			go h.dispatchDeleteProxy(agentID, existingDomain)
		}

		_, err = tx.Exec(c.Request.Context(), `
			UPDATE proxy_hosts
			SET domain = $1, force_ssl = $2, http2_enabled = $3, updated_at = NOW()
			WHERE id = $4
		`, req.Domain, req.ForceSSL, req.HTTP2Enabled, existingProxyID)

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
			INSERT INTO proxy_hosts (agent_id, domain, upstream_target, force_ssl, http2_enabled, status, is_system_proxy)
			VALUES ($1, $2, $3, $4, $5, 'active', TRUE)
			RETURNING id
		`, agentID, req.Domain, upstream, req.ForceSSL, req.HTTP2Enabled).Scan(&proxyID)

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
		Domain:       req.Domain,
		SSLEnabled:   req.SSLEnabled,
		ForceSSL:     req.ForceSSL,
		HTTP2Enabled: req.HTTP2Enabled,
		ProxyHostID:  proxyID.String(),
		Status:       "active",
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

	// Dispatch the special InfraPilot nginx config to the agent
	go h.dispatchInfraPilotProxyConfig(c.Request.Context(), agentID, proxyID, req.Domain, req.ForceSSL, req.HTTP2Enabled, req.SSLEnabled)

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

	// Audit log
	h.auditLog(c, userID, orgID, "settings.infrapilot_domain.delete", "system_settings", uuid.Nil, nil)

	c.JSON(http.StatusOK, gin.H{"message": "domain configuration deleted"})
}

// dispatchInfraPilotProxyConfig sends the special InfraPilot nginx config to the agent
func (h *Handler) dispatchInfraPilotProxyConfig(ctx interface{}, agentID, proxyID uuid.UUID, domain string, forceSSL, http2, sslEnabled bool) {
	agentIDStr := agentID.String()

	if !agentgrpc.IsAgentConnected(agentIDStr) {
		h.logger.Warn("Agent not connected, cannot dispatch InfraPilot proxy config",
			zap.String("agent_id", agentIDStr),
			zap.String("domain", domain),
		)
		return
	}

	// Generate special InfraPilot nginx config that routes /api to backend
	config := generateInfraPilotNginxConfig(domain, forceSSL, http2, sslEnabled)

	// Build config path
	configPath := filepath.Join("/etc/nginx/sites", domain+".conf")

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
	)
}

// generateInfraPilotNginxConfig creates the special nginx config for InfraPilot's domain
// This routes /api/* to backend and everything else to frontend
func generateInfraPilotNginxConfig(domain string, forceSSL, http2, sslEnabled bool) string {
	var config strings.Builder

	config.WriteString("# Managed by InfraPilot - System Proxy\n")
	config.WriteString(fmt.Sprintf("# Domain: %s\n\n", domain))

	// HTTP server block
	config.WriteString("server {\n")
	config.WriteString("    listen 80;\n")
	config.WriteString("    listen [::]:80;\n")
	config.WriteString(fmt.Sprintf("    server_name %s;\n\n", domain))

	if sslEnabled && forceSSL {
		config.WriteString("    return 301 https://$host$request_uri;\n")
		config.WriteString("}\n\n")
	} else {
		writeInfraPilotLocations(&config)
		config.WriteString("}\n\n")
	}

	// HTTPS server block (if SSL enabled)
	if sslEnabled {
		listen := "443 ssl"
		if http2 {
			listen = "443 ssl http2"
		}

		config.WriteString("server {\n")
		config.WriteString(fmt.Sprintf("    listen %s;\n", listen))
		config.WriteString(fmt.Sprintf("    listen [::]:%s;\n", listen))
		config.WriteString(fmt.Sprintf("    server_name %s;\n\n", domain))

		// SSL configuration
		config.WriteString(fmt.Sprintf("    ssl_certificate /etc/letsencrypt/live/%s/fullchain.pem;\n", domain))
		config.WriteString(fmt.Sprintf("    ssl_certificate_key /etc/letsencrypt/live/%s/privkey.pem;\n\n", domain))

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

		writeInfraPilotLocations(&config)
		config.WriteString("}\n")
	}

	return config.String()
}

// writeInfraPilotLocations writes the location blocks for InfraPilot
func writeInfraPilotLocations(config *strings.Builder) {
	// API routes to backend
	config.WriteString("    # API routes to backend\n")
	config.WriteString("    location /api/ {\n")
	config.WriteString("        proxy_pass http://backend:8080;\n")
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

	// Health check endpoint
	config.WriteString("    location = /health {\n")
	config.WriteString("        proxy_pass http://backend:8080/health;\n")
	config.WriteString("        proxy_http_version 1.1;\n")
	config.WriteString("        proxy_set_header Host $host;\n")
	config.WriteString("    }\n\n")

	// Frontend (everything else)
	config.WriteString("    # Frontend\n")
	config.WriteString("    location / {\n")
	config.WriteString("        proxy_pass http://frontend:3000;\n")
	config.WriteString("        proxy_http_version 1.1;\n")
	config.WriteString("        proxy_set_header Host $host;\n")
	config.WriteString("        proxy_set_header X-Real-IP $remote_addr;\n")
	config.WriteString("        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n")
	config.WriteString("        proxy_set_header X-Forwarded-Proto $scheme;\n")
	config.WriteString("        proxy_set_header Upgrade $http_upgrade;\n")
	config.WriteString("        proxy_set_header Connection \"upgrade\";\n")
	config.WriteString("    }\n\n")

	// ACME challenge for Let's Encrypt
	config.WriteString("    # ACME challenge for Let's Encrypt\n")
	config.WriteString("    location /.well-known/acme-challenge/ {\n")
	config.WriteString("        root /data/letsencrypt/webroot;\n")
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
