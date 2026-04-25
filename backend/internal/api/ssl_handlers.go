package api

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net"
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

// SSLCertificate represents a registered SSL certificate
type SSLCertificate struct {
	ID           uuid.UUID  `json:"id"`
	OrgID        uuid.UUID  `json:"org_id"`
	Name         string     `json:"name"`
	Domain       string     `json:"domain"`
	IsWildcard   bool       `json:"is_wildcard"`
	CertPath     string     `json:"cert_path"`
	KeyPath      string     `json:"key_path"`
	Issuer       string     `json:"issuer,omitempty"`
	Subject      string     `json:"subject,omitempty"`
	SAN          string     `json:"san,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	AutoDetected bool       `json:"auto_detected"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// SSLCertificateInfo represents SSL certificate information
type SSLCertificateInfo struct {
	Exists      bool      `json:"exists"`
	Domain      string    `json:"domain"`
	Issuer      string    `json:"issuer,omitempty"`
	Subject     string    `json:"subject,omitempty"`
	ExpiresAt   time.Time `json:"expires_at,omitempty"`
	DaysLeft    int       `json:"days_left,omitempty"`
	IsWildcard  bool      `json:"is_wildcard,omitempty"`
	ValidForDomain bool   `json:"valid_for_domain"`
	Error       string    `json:"error,omitempty"`
	SANs        []string  `json:"sans,omitempty"`
}

// SSLCheckRequest is the request for checking SSL status
type SSLCheckRequest struct {
	Domain      string `json:"domain" binding:"required"`
	CheckRemote bool   `json:"check_remote"` // Check remote server, not local files
}

// SSLRequestOptions represents options for requesting a certificate
type SSLRequestOptions struct {
	Domain       string `json:"domain" binding:"required"`
	Email        string `json:"email"`
	DNSProvider  string `json:"dns_provider,omitempty"` // For DNS-01 challenge
	Staging      bool   `json:"staging"`                // Use Let's Encrypt staging
	ForceRenew   bool   `json:"force_renew"`            // Force renewal even if valid
}

// SSLStatusResponse represents the SSL configuration status
type SSLStatusResponse struct {
	LetsEncryptEmail   string `json:"letsencrypt_email"`
	LetsEncryptStaging bool   `json:"letsencrypt_staging"`
	AccountConfigured  bool   `json:"account_configured"`
	CertDirectory      string `json:"cert_directory"`
}

// checkDomainSSL checks SSL certificate status for a domain
// GET /api/v1/ssl/check/:domain
func (h *Handler) checkDomainSSL(c *gin.Context) {
	domain := c.Param("domain")
	if domain == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "domain is required"})
		return
	}

	// Check remote means checking the actual server's certificate
	checkRemote := c.Query("remote") == "true"

	var certInfo SSLCertificateInfo
	certInfo.Domain = domain

	if checkRemote {
		// Check the remote server's SSL certificate
		certInfo = h.checkRemoteSSL(domain)
	} else {
		// Check local Let's Encrypt certificates via agent
		// First, try to get info from any connected agent
		orgID := c.MustGet("org_id").(uuid.UUID)
		certInfo = h.checkLocalSSL(c, orgID, domain)
	}

	c.JSON(http.StatusOK, certInfo)
}

// checkRemoteSSL checks the SSL certificate served by a remote domain
func (h *Handler) checkRemoteSSL(domain string) SSLCertificateInfo {
	info := SSLCertificateInfo{
		Domain: domain,
		Exists: false,
	}

	// Connect to the server with a timeout
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	conn, err := tls.DialWithDialer(dialer, "tcp", domain+":443", &tls.Config{
		InsecureSkipVerify: true, // We want to check even invalid certs
	})
	if err != nil {
		// Don't show technical errors for expected cases
		// Connection refused, reset, timeout, EOF are all expected for domains without SSL
		errStr := err.Error()
		if strings.Contains(errStr, "connection refused") ||
			strings.Contains(errStr, "connection reset") ||
			strings.Contains(errStr, "no such host") ||
			strings.Contains(errStr, "timeout") ||
			strings.Contains(errStr, "i/o timeout") ||
			strings.Contains(errStr, "EOF") ||
			strings.Contains(errStr, "eof") {
			// This is expected for new domains - no error message needed
			return info
		}
		info.Error = fmt.Sprintf("Could not connect: %v", err)
		return info
	}
	defer conn.Close()

	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		info.Error = "No certificates found"
		return info
	}

	cert := certs[0]
	info.Exists = true
	info.Issuer = cert.Issuer.CommonName
	info.Subject = cert.Subject.CommonName
	info.ExpiresAt = cert.NotAfter
	info.DaysLeft = int(time.Until(cert.NotAfter).Hours() / 24)
	info.SANs = cert.DNSNames

	// Check if cert is valid for this domain
	info.ValidForDomain = false
	for _, san := range cert.DNSNames {
		if san == domain {
			info.ValidForDomain = true
			break
		}
		// Check for wildcard match
		if strings.HasPrefix(san, "*.") {
			parentDomain := san[2:] // Remove "*."
			if strings.HasSuffix(domain, parentDomain) {
				// Check if it's a direct subdomain (not nested)
				prefix := strings.TrimSuffix(domain, parentDomain)
				prefix = strings.TrimSuffix(prefix, ".")
				if !strings.Contains(prefix, ".") {
					info.ValidForDomain = true
					info.IsWildcard = true
					break
				}
			}
		}
	}

	// Also check against common name
	if cert.Subject.CommonName == domain {
		info.ValidForDomain = true
	}

	return info
}

// checkLocalSSL checks if Let's Encrypt certificate exists locally via agent
func (h *Handler) checkLocalSSL(c *gin.Context, orgID uuid.UUID, domain string) SSLCertificateInfo {
	info := SSLCertificateInfo{
		Domain: domain,
		Exists: false,
	}

	// Find an active agent for this org
	var agentID string
	err := h.db.QueryRow(c.Request.Context(), `
		SELECT id FROM agents WHERE org_id = $1 AND status = 'active' LIMIT 1
	`, orgID).Scan(&agentID)
	if err != nil {
		info.Error = "No active agent found"
		return info
	}

	// Check if agent is connected
	if !agentgrpc.IsAgentConnected(agentID) {
		info.Error = "Agent not connected"
		return info
	}

	// Send command to check certificate
	cmd := agentgrpc.NewSSLCheckCommand(domain)
	resp, err := agentgrpc.SendCommand(agentID, cmd, 15*time.Second)
	if err != nil {
		h.logger.Error("Failed to check SSL via agent", zap.Error(err))
		info.Error = fmt.Sprintf("Agent communication error: %v", err)
		return info
	}

	// Parse response
	if result, err := resp.GetSSLCheckResult(); err == nil && result != nil {
		info.Exists = result.Exists
		info.Issuer = result.Issuer
		info.ExpiresAt = result.ExpiresAt
		info.DaysLeft = result.DaysLeft
		info.ValidForDomain = result.ValidForDomain
		info.IsWildcard = result.IsWildcard
		info.SANs = result.SANs
		if result.Error != "" {
			info.Error = result.Error
		}
	}

	return info
}

// checkWildcardSSL checks if a wildcard certificate covers this domain
// GET /api/v1/ssl/check-wildcard/:domain
func (h *Handler) checkWildcardSSL(c *gin.Context) {
	domain := c.Param("domain")
	if domain == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "domain is required"})
		return
	}

	// Extract parent domain for wildcard check
	parts := strings.Split(domain, ".")
	if len(parts) < 2 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid domain"})
		return
	}

	results := []SSLCertificateInfo{}

	// Check exact domain
	exactInfo := h.checkRemoteSSL(domain)
	exactInfo.Domain = domain
	results = append(results, exactInfo)

	// Check parent domain for wildcard
	if len(parts) >= 2 {
		parentDomain := strings.Join(parts[1:], ".")
		wildcardDomain := "*." + parentDomain

		// Check if there's a cert for the parent domain
		parentInfo := h.checkRemoteSSL(parentDomain)
		parentInfo.Domain = wildcardDomain

		// Check if any SAN is a wildcard covering our domain
		for _, san := range parentInfo.SANs {
			if san == wildcardDomain {
				parentInfo.IsWildcard = true
				parentInfo.ValidForDomain = true
				break
			}
		}
		results = append(results, parentInfo)
	}

	c.JSON(http.StatusOK, gin.H{
		"domain":       domain,
		"certificates": results,
	})
}

// getSSLStatus returns the SSL/Let's Encrypt configuration status
// GET /api/v1/ssl/status
func (h *Handler) getSSLStatus(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)

	// Get Let's Encrypt settings from system_settings
	var settingsJSON []byte
	err := h.db.QueryRow(c.Request.Context(), `
		SELECT setting_value FROM system_settings
		WHERE org_id = $1 AND setting_key = 'letsencrypt_config'
	`, orgID).Scan(&settingsJSON)

	status := SSLStatusResponse{
		LetsEncryptStaging: true, // Default to staging
		CertDirectory:      "/etc/letsencrypt",
	}

	if err == nil && len(settingsJSON) > 0 {
		// Parse existing settings
		var settings struct {
			Email   string `json:"email"`
			Staging bool   `json:"staging"`
		}
		if err := json.Unmarshal(settingsJSON, &settings); err == nil {
			status.LetsEncryptEmail = settings.Email
			status.LetsEncryptStaging = settings.Staging
			status.AccountConfigured = settings.Email != ""
		}
	}

	c.JSON(http.StatusOK, status)
}

// updateSSLSettings updates the SSL/Let's Encrypt settings
// PUT /api/v1/ssl/settings
func (h *Handler) updateSSLSettings(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	userID := c.MustGet("user_id").(uuid.UUID)

	var req struct {
		Email   string `json:"email" binding:"required,email"`
		Staging bool   `json:"staging"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	settingsJSON, _ := json.Marshal(map[string]interface{}{
		"email":   req.Email,
		"staging": req.Staging,
	})

	_, err := h.db.Exec(c.Request.Context(), `
		INSERT INTO system_settings (org_id, setting_key, setting_value)
		VALUES ($1, 'letsencrypt_config', $2)
		ON CONFLICT (org_id, setting_key) DO UPDATE SET
			setting_value = EXCLUDED.setting_value,
			updated_at = NOW()
	`, orgID, settingsJSON)

	if err != nil {
		h.logger.Error("Failed to update SSL settings", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update settings"})
		return
	}

	// Audit log
	h.auditLog(c, userID, orgID, "ssl.settings.update", "system_settings", uuid.Nil, req)

	c.JSON(http.StatusOK, gin.H{
		"email":   req.Email,
		"staging": req.Staging,
		"message": "SSL settings updated",
	})
}

// requestSSLCertificate requests a new SSL certificate
// POST /api/v1/ssl/request
func (h *Handler) requestSSLCertificate(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	userID := c.MustGet("user_id").(uuid.UUID)

	var req SSLRequestOptions
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get email from settings if not provided
	if req.Email == "" {
		var settingsJSON []byte
		h.db.QueryRow(c.Request.Context(), `
			SELECT setting_value FROM system_settings
			WHERE org_id = $1 AND setting_key = 'letsencrypt_config'
		`, orgID).Scan(&settingsJSON)

		if len(settingsJSON) > 0 {
			var settings struct {
				Email   string `json:"email"`
				Staging bool   `json:"staging"`
			}
			if err := json.Unmarshal(settingsJSON, &settings); err == nil {
				req.Email = settings.Email
				if !req.Staging {
					req.Staging = settings.Staging
				}
			}
		}
	}

	if req.Email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "email is required for Let's Encrypt"})
		return
	}

	// Find an active agent
	var agentID uuid.UUID
	err := h.db.QueryRow(c.Request.Context(), `
		SELECT id FROM agents WHERE org_id = $1 AND status = 'active' LIMIT 1
	`, orgID).Scan(&agentID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no active agent found"})
		return
	}

	agentIDStr := agentID.String()

	// Check if agent is connected
	if !agentgrpc.IsAgentConnected(agentIDStr) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "agent not connected"})
		return
	}

	// Check if the proxy has include_www enabled
	var includeWWW bool
	h.db.QueryRow(c.Request.Context(), `
		SELECT COALESCE(include_www, false) FROM proxy_hosts
		WHERE agent_id = $1 AND domain = $2
	`, agentID, req.Domain).Scan(&includeWWW)

	// Send SSL request to agent and wait for response (up to 2 minutes for ACME challenge)
	cmd := agentgrpc.NewSSLRequestCommand(req.Domain, req.Email, req.DNSProvider, req.Staging, includeWWW)

	h.logger.Info("Sending SSL request to agent",
		zap.String("agent_id", agentIDStr),
		zap.String("domain", req.Domain),
		zap.String("email", req.Email),
		zap.Bool("staging", req.Staging),
		zap.Bool("include_www", includeWWW),
	)

	resp, err := agentgrpc.SendCommand(agentIDStr, cmd, 120*time.Second)
	if err != nil {
		h.logger.Error("SSL request failed", zap.Error(err), zap.String("domain", req.Domain))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   fmt.Sprintf("SSL request failed: %v", err),
			"domain":  req.Domain,
		})
		return
	}

	// Audit log
	h.auditLog(c, userID, orgID, "ssl.request", "domain", uuid.Nil, req)

	// Parse response from agent
	success := false
	message := ""

	if result, err := resp.GetCommandResult(); err == nil && result != nil {
		success = result.Success
		message = result.Message
	}

	if success {
		// Update proxy_hosts to enable SSL and regenerate nginx config
		var proxyID uuid.UUID
		var forceSSL, http2Enabled, isSystemProxy, includeWWW bool
		var upstream, proxyType string
		var redirectURL *string
		var redirectCode *int
		err := h.db.QueryRow(c.Request.Context(), `
			UPDATE proxy_hosts SET ssl_enabled = true, force_ssl = true, status = 'active', updated_at = NOW()
			WHERE agent_id = $1 AND domain = $2
			RETURNING id, force_ssl, http2_enabled, is_system_proxy, include_www,
			          COALESCE(upstream_target, ''), COALESCE(proxy_type, 'upstream'), redirect_url, redirect_code
		`, agentID, req.Domain).Scan(&proxyID, &forceSSL, &http2Enabled, &isSystemProxy, &includeWWW,
			&upstream, &proxyType, &redirectURL, &redirectCode)

		if err == nil {
			h.logger.Info("Updated proxy_hosts for SSL",
				zap.String("domain", req.Domain),
				zap.String("proxy_id", proxyID.String()),
				zap.String("proxy_type", proxyType),
			)

			// Regenerate and dispatch nginx config with SSL enabled
			if isSystemProxy {
				go h.dispatchInfraPilotProxyConfig(c.Request.Context(), agentID, proxyID, req.Domain, true, http2Enabled, true)
			} else {
				// For regular proxies (upstream or redirect), dispatch full config
				go h.dispatchProxyConfigFull(c.Request.Context(), agentID, proxyID, req.Domain, upstream,
					proxyType, redirectURL, redirectCode, forceSSL, http2Enabled, includeWWW, true)
			}
		} else {
			h.logger.Warn("Could not find proxy_host to update SSL status",
				zap.String("domain", req.Domain),
				zap.Error(err),
			)
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": message,
			"domain":  req.Domain,
			"email":   req.Email,
			"staging": req.Staging,
		})
	} else {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   message,
			"domain":  req.Domain,
		})
	}
}

// dispatchSSLRequestWithOptions sends SSL request with full options
func (h *Handler) dispatchSSLRequestWithOptions(agentID uuid.UUID, domain, email, dnsProvider string, staging, includeWWW bool) {
	agentIDStr := agentID.String()

	if !agentgrpc.IsAgentConnected(agentIDStr) {
		h.logger.Warn("Agent not connected for SSL request",
			zap.String("agent_id", agentIDStr),
			zap.String("domain", domain),
		)
		return
	}

	cmd := agentgrpc.NewSSLRequestCommand(domain, email, dnsProvider, staging, includeWWW)

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
		zap.Bool("staging", staging),
		zap.Bool("include_www", includeWWW),
	)
}

// getPublicIP tries to detect the server's public IP
func getPublicIP() string {
	// Try multiple services for reliability
	services := []string{
		"https://api.ipify.org",
		"https://ifconfig.me/ip",
		"https://icanhazip.com",
	}

	client := &http.Client{Timeout: 5 * time.Second}
	for _, svc := range services {
		resp, err := client.Get(svc)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		body := make([]byte, 64)
		n, _ := resp.Body.Read(body)
		ip := strings.TrimSpace(string(body[:n]))

		// Validate it looks like an IP
		if net.ParseIP(ip) != nil {
			return ip
		}
	}
	return ""
}

// getDNSInstructions returns DNS configuration instructions for a domain
// GET /api/v1/ssl/dns-instructions/:domain
func (h *Handler) getDNSInstructions(c *gin.Context) {
	domain := c.Param("domain")
	if domain == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "domain is required"})
		return
	}

	// Get server's public IP
	serverIP := getPublicIP()

	// Fallback message if still not found
	if serverIP == "" {
		serverIP = "YOUR_SERVER_IP"
	}

	c.JSON(http.StatusOK, gin.H{
		"domain":    domain,
		"server_ip": serverIP,
		"records": []gin.H{
			{
				"type":  "A",
				"name":  domain,
				"value": serverIP,
				"ttl":   300,
			},
		},
		"instructions": fmt.Sprintf(`
To configure SSL for %s:

1. Add an A record pointing to your server:
   - Type: A
   - Name: %s
   - Value: %s
   - TTL: 300 (or Auto)

2. Wait for DNS propagation (usually 1-5 minutes, can take up to 48 hours)

3. Click "Request Certificate" to obtain SSL from Let's Encrypt

4. Let's Encrypt will verify domain ownership via HTTP-01 challenge
`, domain, domain, serverIP),
	})
}

// verifyDNS checks if DNS is properly configured for a domain
// GET /api/v1/ssl/verify-dns/:domain?include_www=true
func (h *Handler) verifyDNS(c *gin.Context) {
	domain := c.Param("domain")
	if domain == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "domain is required"})
		return
	}

	includeWWW := c.Query("include_www") == "true"

	// Get expected IP
	expectedIP := getPublicIP()

	// Helper to check a single domain
	type domainResult struct {
		Domain      string   `json:"domain"`
		Configured  bool     `json:"configured"`
		ResolvedIPs []string `json:"resolved_ips"`
		Matches     bool     `json:"matches"`
		Error       string   `json:"error,omitempty"`
	}

	checkDomain := func(d string) domainResult {
		result := domainResult{Domain: d}
		ips, err := net.LookupIP(d)
		if err != nil {
			result.Error = fmt.Sprintf("DNS lookup failed: %v", err)
			return result
		}

		result.Configured = len(ips) > 0
		result.ResolvedIPs = []string{}
		for _, ip := range ips {
			ipStr := ip.String()
			result.ResolvedIPs = append(result.ResolvedIPs, ipStr)
			if ipStr == expectedIP {
				result.Matches = true
			}
		}
		return result
	}

	// Check main domain
	mainResult := checkDomain(domain)

	// If include_www is enabled, also check www subdomain
	if includeWWW {
		wwwDomain := "www." + domain
		wwwResult := checkDomain(wwwDomain)

		// Both domains must be configured and match for overall success
		allConfigured := mainResult.Configured && wwwResult.Configured
		allMatches := mainResult.Matches && wwwResult.Matches

		// Combine resolved IPs for display
		allIPs := mainResult.ResolvedIPs
		for _, ip := range wwwResult.ResolvedIPs {
			// Add only if not already present
			found := false
			for _, existingIP := range allIPs {
				if existingIP == ip {
					found = true
					break
				}
			}
			if !found {
				allIPs = append(allIPs, ip)
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"domain":       domain,
			"configured":   allConfigured,
			"resolved_ips": allIPs,
			"expected_ip":  expectedIP,
			"matches":      allMatches,
			"include_www":  true,
			"www_result":   wwwResult,
			"main_result":  mainResult,
		})
		return
	}

	// Standard single-domain response
	c.JSON(http.StatusOK, gin.H{
		"domain":       domain,
		"configured":   mainResult.Configured,
		"resolved_ips": mainResult.ResolvedIPs,
		"expected_ip":  expectedIP,
		"matches":      mainResult.Matches,
	})
}

// listSSLCertificates returns all registered SSL certificates
// GET /api/v1/ssl/certificates
func (h *Handler) listSSLCertificates(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)

	rows, err := h.db.Query(c.Request.Context(), `
		SELECT id, org_id, name, domain, is_wildcard, cert_path, key_path,
		       issuer, subject, san, expires_at, auto_detected, created_at, updated_at
		FROM ssl_certificates
		WHERE org_id = $1
		ORDER BY domain ASC
	`, orgID)
	if err != nil {
		h.logger.Error("Failed to list SSL certificates", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list certificates"})
		return
	}
	defer rows.Close()

	certs := []SSLCertificate{}
	for rows.Next() {
		var cert SSLCertificate
		var issuer, subject, san *string
		var expiresAt *time.Time

		err := rows.Scan(
			&cert.ID, &cert.OrgID, &cert.Name, &cert.Domain, &cert.IsWildcard,
			&cert.CertPath, &cert.KeyPath, &issuer, &subject, &san,
			&expiresAt, &cert.AutoDetected, &cert.CreatedAt, &cert.UpdatedAt,
		)
		if err != nil {
			h.logger.Error("Failed to scan certificate row", zap.Error(err))
			continue
		}

		if issuer != nil {
			cert.Issuer = *issuer
		}
		if subject != nil {
			cert.Subject = *subject
		}
		if san != nil {
			cert.SAN = *san
		}
		cert.ExpiresAt = expiresAt

		certs = append(certs, cert)
	}

	c.JSON(http.StatusOK, certs)
}

// scanSSLCertificates scans /etc/letsencrypt/live for available certificates
// GET /api/v1/ssl/certificates/scan
func (h *Handler) scanSSLCertificates(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)

	letsEncryptDir := "/etc/letsencrypt/live"

	// Check if directory exists
	if _, err := os.Stat(letsEncryptDir); os.IsNotExist(err) {
		c.JSON(http.StatusOK, gin.H{
			"certificates": []SSLCertificate{},
			"message":      "Let's Encrypt directory not found",
		})
		return
	}

	// Read directory entries
	entries, err := os.ReadDir(letsEncryptDir)
	if err != nil {
		h.logger.Error("Failed to read Let's Encrypt directory", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to scan certificates"})
		return
	}

	certs := []SSLCertificate{}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "README" {
			continue
		}

		domain := entry.Name()
		certPath := filepath.Join(letsEncryptDir, domain, "fullchain.pem")
		keyPath := filepath.Join(letsEncryptDir, domain, "privkey.pem")

		// Check if cert and key files exist
		if _, err := os.Stat(certPath); os.IsNotExist(err) {
			continue
		}
		if _, err := os.Stat(keyPath); os.IsNotExist(err) {
			continue
		}

		// Parse the certificate to extract metadata
		certInfo, err := parseCertificateFile(certPath)
		if err != nil {
			h.logger.Warn("Failed to parse certificate", zap.String("path", certPath), zap.Error(err))
			continue
		}

		// Check if already registered
		var existingID uuid.UUID
		err = h.db.QueryRow(c.Request.Context(), `
			SELECT id FROM ssl_certificates WHERE org_id = $1 AND cert_path = $2
		`, orgID, certPath).Scan(&existingID)

		// Generate a deterministic UUID from cert_path for unregistered certs
		// This ensures consistent IDs across scans
		var certID uuid.UUID
		if err == nil {
			certID = existingID
		} else {
			// Use UUID v5 with a namespace to generate deterministic ID from cert path
			certID = uuid.NewSHA1(uuid.NameSpaceURL, []byte(certPath))
		}

		cert := SSLCertificate{
			ID:           certID,
			OrgID:        orgID,
			Name:         certInfo.Subject,
			Domain:       domain,
			IsWildcard:   certInfo.IsWildcard,
			CertPath:     certPath,
			KeyPath:      keyPath,
			Issuer:       certInfo.Issuer,
			Subject:      certInfo.Subject,
			SAN:          strings.Join(certInfo.SANs, ", "),
			ExpiresAt:    &certInfo.ExpiresAt,
			AutoDetected: true,
		}

		certs = append(certs, cert)
	}

	c.JSON(http.StatusOK, gin.H{
		"certificates": certs,
		"scanned_dir":  letsEncryptDir,
	})
}

// registerSSLCertificate registers an external SSL certificate
// POST /api/v1/ssl/certificates
func (h *Handler) registerSSLCertificate(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	userID := c.MustGet("user_id").(uuid.UUID)

	var req struct {
		Name     string `json:"name"`
		Domain   string `json:"domain"`
		CertPath string `json:"cert_path" binding:"required"`
		KeyPath  string `json:"key_path" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate cert file exists and is readable
	if _, err := os.Stat(req.CertPath); os.IsNotExist(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "certificate file not found"})
		return
	}
	if _, err := os.Stat(req.KeyPath); os.IsNotExist(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "key file not found"})
		return
	}

	// Parse certificate to extract metadata
	certInfo, err := parseCertificateFile(req.CertPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("failed to parse certificate: %v", err)})
		return
	}

	// Use domain from cert if not provided
	domain := req.Domain
	if domain == "" {
		if len(certInfo.SANs) > 0 {
			domain = certInfo.SANs[0]
		} else {
			domain = certInfo.Subject
		}
	}

	// Use name from cert if not provided
	name := req.Name
	if name == "" {
		name = certInfo.Subject
	}

	// Check for duplicate
	var existingID uuid.UUID
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT id FROM ssl_certificates WHERE org_id = $1 AND cert_path = $2
	`, orgID, req.CertPath).Scan(&existingID)

	if err == nil {
		// Update existing
		_, err = h.db.Exec(c.Request.Context(), `
			UPDATE ssl_certificates SET
				name = $1, domain = $2, is_wildcard = $3, key_path = $4,
				issuer = $5, subject = $6, san = $7, expires_at = $8,
				auto_detected = FALSE, updated_at = NOW()
			WHERE id = $9
		`, name, domain, certInfo.IsWildcard, req.KeyPath,
			certInfo.Issuer, certInfo.Subject, strings.Join(certInfo.SANs, ", "),
			certInfo.ExpiresAt, existingID)

		if err != nil {
			h.logger.Error("Failed to update certificate", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update certificate"})
			return
		}

		h.auditLog(c, userID, orgID, "ssl.certificate.update", "ssl_certificates", existingID, req)

		c.JSON(http.StatusOK, gin.H{
			"id":      existingID,
			"message": "certificate updated",
		})
		return
	}

	// Insert new
	var certID uuid.UUID
	err = h.db.QueryRow(c.Request.Context(), `
		INSERT INTO ssl_certificates (org_id, name, domain, is_wildcard, cert_path, key_path, issuer, subject, san, expires_at, auto_detected)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, FALSE)
		RETURNING id
	`, orgID, name, domain, certInfo.IsWildcard, req.CertPath, req.KeyPath,
		certInfo.Issuer, certInfo.Subject, strings.Join(certInfo.SANs, ", "),
		certInfo.ExpiresAt).Scan(&certID)

	if err != nil {
		h.logger.Error("Failed to register certificate", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to register certificate"})
		return
	}

	h.auditLog(c, userID, orgID, "ssl.certificate.register", "ssl_certificates", certID, req)

	c.JSON(http.StatusCreated, gin.H{
		"id":         certID,
		"name":       name,
		"domain":     domain,
		"is_wildcard": certInfo.IsWildcard,
		"issuer":     certInfo.Issuer,
		"expires_at": certInfo.ExpiresAt,
		"message":    "certificate registered",
	})
}

// deleteSSLCertificate removes a registered SSL certificate
// DELETE /api/v1/ssl/certificates/:id
func (h *Handler) deleteSSLCertificate(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	userID := c.MustGet("user_id").(uuid.UUID)

	certID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid certificate ID"})
		return
	}

	// Check if certificate is in use by any proxy
	var inUseCount int
	h.db.QueryRow(c.Request.Context(), `
		SELECT COUNT(*) FROM proxy_hosts WHERE ssl_certificate_id = $1
	`, certID).Scan(&inUseCount)

	if inUseCount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":    "certificate is in use by proxy hosts",
			"in_use_count": inUseCount,
		})
		return
	}

	result, err := h.db.Exec(c.Request.Context(), `
		DELETE FROM ssl_certificates WHERE id = $1 AND org_id = $2
	`, certID, orgID)
	if err != nil {
		h.logger.Error("Failed to delete certificate", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete certificate"})
		return
	}

	if result.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "certificate not found"})
		return
	}

	h.auditLog(c, userID, orgID, "ssl.certificate.delete", "ssl_certificates", certID, nil)

	c.JSON(http.StatusOK, gin.H{"message": "certificate deleted"})
}

// getSSLCertificate gets a specific SSL certificate by ID
// GET /api/v1/ssl/certificates/:id
func (h *Handler) getSSLCertificate(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)

	certID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid certificate ID"})
		return
	}

	var cert SSLCertificate
	var issuer, subject, san *string
	var expiresAt *time.Time

	err = h.db.QueryRow(c.Request.Context(), `
		SELECT id, org_id, name, domain, is_wildcard, cert_path, key_path,
		       issuer, subject, san, expires_at, auto_detected, created_at, updated_at
		FROM ssl_certificates
		WHERE id = $1 AND org_id = $2
	`, certID, orgID).Scan(
		&cert.ID, &cert.OrgID, &cert.Name, &cert.Domain, &cert.IsWildcard,
		&cert.CertPath, &cert.KeyPath, &issuer, &subject, &san,
		&expiresAt, &cert.AutoDetected, &cert.CreatedAt, &cert.UpdatedAt,
	)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "certificate not found"})
		return
	}

	if issuer != nil {
		cert.Issuer = *issuer
	}
	if subject != nil {
		cert.Subject = *subject
	}
	if san != nil {
		cert.SAN = *san
	}
	cert.ExpiresAt = expiresAt

	c.JSON(http.StatusOK, cert)
}

// CertificateInfo holds parsed certificate metadata
type CertificateInfo struct {
	Subject    string
	Issuer     string
	SANs       []string
	ExpiresAt  time.Time
	IsWildcard bool
}

// DNSChallengeInfo represents DNS-01 challenge information
type DNSChallengeInfo struct {
	Domain     string         `json:"domain"`
	TXTRecord  string         `json:"txt_record"`
	TXTName    string         `json:"txt_name"`
	TXTRecords []DNSTXTRecord `json:"txt_records,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
}

// DNSTXTRecord represents a single TXT record for DNS challenge
type DNSTXTRecord struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// startDNSChallenge starts a DNS-01 challenge for wildcard certificates
// POST /api/v1/ssl/dns-challenge/start
func (h *Handler) startDNSChallenge(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	userID := c.MustGet("user_id").(uuid.UUID)

	var req struct {
		Domain  string `json:"domain" binding:"required"`
		Email   string `json:"email"`
		Staging bool   `json:"staging"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get email from settings if not provided
	if req.Email == "" {
		var settingsJSON []byte
		h.db.QueryRow(c.Request.Context(), `
			SELECT setting_value FROM system_settings
			WHERE org_id = $1 AND setting_key = 'letsencrypt_config'
		`, orgID).Scan(&settingsJSON)

		if len(settingsJSON) > 0 {
			var settings struct {
				Email   string `json:"email"`
				Staging bool   `json:"staging"`
			}
			if err := json.Unmarshal(settingsJSON, &settings); err == nil {
				req.Email = settings.Email
				if !req.Staging {
					req.Staging = settings.Staging
				}
			}
		}
	}

	if req.Email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "email is required for Let's Encrypt"})
		return
	}

	// Find an active agent
	var agentID uuid.UUID
	err := h.db.QueryRow(c.Request.Context(), `
		SELECT id FROM agents WHERE org_id = $1 AND status = 'active' LIMIT 1
	`, orgID).Scan(&agentID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no active agent found"})
		return
	}

	agentIDStr := agentID.String()

	if !agentgrpc.IsAgentConnected(agentIDStr) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "agent not connected"})
		return
	}

	// Send start DNS challenge command
	cmd := agentgrpc.NewSSLStartDNSChallengeCommand(req.Domain, req.Email, req.Staging)

	h.logger.Info("Starting DNS challenge",
		zap.String("agent_id", agentIDStr),
		zap.String("domain", req.Domain),
	)

	resp, err := agentgrpc.SendCommand(agentIDStr, cmd, 90*time.Second)
	if err != nil {
		h.logger.Error("DNS challenge start failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   fmt.Sprintf("failed to start DNS challenge: %v", err),
		})
		return
	}

	// Audit log
	h.auditLog(c, userID, orgID, "ssl.dns_challenge.start", "domain", uuid.Nil, req)

	// Parse response - handle both direct struct and JSON unmarshaled map
	success := false
	message := ""
	var challenge DNSChallengeInfo

	if result, err := resp.GetCommandResult(); err == nil && result != nil {
		success = result.Success
		message = result.Message
		if result.Data != nil {
			json.Unmarshal(result.Data, &challenge)
		}
	}

	if success {
		// Build instructions based on number of TXT records
		var instructions string
		if len(challenge.TXTRecords) > 1 {
			instructions = "Add these TXT records to your DNS:\n\n"
			for i, rec := range challenge.TXTRecords {
				instructions += fmt.Sprintf("Record %d:\n  Name:  %s\n  Type:  TXT\n  Value: %s\n\n", i+1, rec.Name, rec.Value)
			}
			instructions += "IMPORTANT: For wildcard certificates, you need to add ALL records listed above.\nAfter adding all records, wait 1-5 minutes for DNS propagation, then click \"Complete Challenge\"."
		} else {
			instructions = fmt.Sprintf(`Add this TXT record to your DNS:

Name:  %s
Type:  TXT
Value: %s

After adding the record, wait 1-5 minutes for DNS propagation, then click "Complete Challenge".`, challenge.TXTName, challenge.TXTRecord)
		}

		c.JSON(http.StatusOK, gin.H{
			"success":     true,
			"message":     message,
			"domain":      req.Domain,
			"txt_record":  challenge.TXTRecord,
			"txt_name":    challenge.TXTName,
			"txt_records": challenge.TXTRecords,
			"instructions": instructions,
		})
	} else {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   message,
		})
	}
}

// completeDNSChallenge completes a DNS-01 challenge after TXT record is added
// POST /api/v1/ssl/dns-challenge/complete
func (h *Handler) completeDNSChallenge(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	userID := c.MustGet("user_id").(uuid.UUID)

	var req struct {
		Domain  string `json:"domain" binding:"required"`
		Email   string `json:"email"`
		Staging bool   `json:"staging"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get email from settings if not provided
	if req.Email == "" {
		var settingsJSON []byte
		h.db.QueryRow(c.Request.Context(), `
			SELECT setting_value FROM system_settings
			WHERE org_id = $1 AND setting_key = 'letsencrypt_config'
		`, orgID).Scan(&settingsJSON)

		if len(settingsJSON) > 0 {
			var settings struct {
				Email   string `json:"email"`
				Staging bool   `json:"staging"`
			}
			if err := json.Unmarshal(settingsJSON, &settings); err == nil {
				req.Email = settings.Email
				if !req.Staging {
					req.Staging = settings.Staging
				}
			}
		}
	}

	// Find an active agent
	var agentID uuid.UUID
	err := h.db.QueryRow(c.Request.Context(), `
		SELECT id FROM agents WHERE org_id = $1 AND status = 'active' LIMIT 1
	`, orgID).Scan(&agentID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no active agent found"})
		return
	}

	agentIDStr := agentID.String()

	if !agentgrpc.IsAgentConnected(agentIDStr) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "agent not connected"})
		return
	}

	// Send complete DNS challenge command
	cmd := agentgrpc.NewSSLCompleteDNSChallengeCommand(req.Domain, req.Email, req.Staging)

	h.logger.Info("Completing DNS challenge",
		zap.String("agent_id", agentIDStr),
		zap.String("domain", req.Domain),
	)

	// DNS challenge completion can take time
	resp, err := agentgrpc.SendCommand(agentIDStr, cmd, 180*time.Second)
	if err != nil {
		h.logger.Error("DNS challenge complete failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   fmt.Sprintf("failed to complete DNS challenge: %v", err),
		})
		return
	}

	// Audit log
	h.auditLog(c, userID, orgID, "ssl.dns_challenge.complete", "domain", uuid.Nil, req)

	// Parse response
	success := false
	message := ""

	if result, err := resp.GetCommandResult(); err == nil && result != nil {
		success = result.Success
		message = result.Message
	}

	if success {
		// Auto-register the certificate in ssl_certificates table
		// For wildcards like *.example.com, the cert is stored under example.com
		certDomain := strings.TrimPrefix(req.Domain, "*.")
		certPath := fmt.Sprintf("/etc/letsencrypt/live/%s/fullchain.pem", certDomain)
		keyPath := fmt.Sprintf("/etc/letsencrypt/live/%s/privkey.pem", certDomain)
		isWildcard := strings.HasPrefix(req.Domain, "*.")

		// Parse the certificate to get metadata
		certInfo, certErr := parseCertificateFile(certPath)
		if certErr == nil {
			// Register or update the certificate
			var certName string
			if isWildcard {
				certName = "Wildcard: " + certDomain
			} else {
				certName = certDomain
			}

			_, dbErr := h.db.Exec(c.Request.Context(), `
				INSERT INTO ssl_certificates (org_id, name, domain, is_wildcard, cert_path, key_path, issuer, subject, san, expires_at, auto_detected)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, TRUE)
				ON CONFLICT (org_id, cert_path) DO UPDATE SET
					name = EXCLUDED.name,
					domain = EXCLUDED.domain,
					is_wildcard = EXCLUDED.is_wildcard,
					key_path = EXCLUDED.key_path,
					issuer = EXCLUDED.issuer,
					subject = EXCLUDED.subject,
					san = EXCLUDED.san,
					expires_at = EXCLUDED.expires_at,
					updated_at = NOW()
			`, orgID, certName, certDomain, isWildcard, certPath, keyPath,
				certInfo.Issuer, certInfo.Subject, strings.Join(certInfo.SANs, ", "), certInfo.ExpiresAt)

			if dbErr != nil {
				h.logger.Warn("Failed to auto-register SSL certificate after DNS challenge",
					zap.Error(dbErr),
					zap.String("domain", req.Domain),
				)
			} else {
				h.logger.Info("Auto-registered SSL certificate after DNS challenge",
					zap.String("domain", req.Domain),
					zap.String("cert_path", certPath),
					zap.Bool("is_wildcard", isWildcard),
				)
			}
		} else {
			h.logger.Warn("Could not parse certificate for auto-registration",
				zap.Error(certErr),
				zap.String("cert_path", certPath),
			)
		}

		// Update proxy_hosts if applicable and regenerate nginx config
		var proxyID uuid.UUID
		var forceSSL, http2Enabled, isSystemProxy, includeWWW bool
		var proxyDomain, upstream, proxyType string
		var redirectURL *string
		var redirectCode *int
		err := h.db.QueryRow(c.Request.Context(), `
			UPDATE proxy_hosts SET ssl_enabled = true, force_ssl = true, status = 'active', updated_at = NOW()
			WHERE agent_id = $1 AND (domain = $2 OR domain LIKE $3)
			RETURNING id, domain, force_ssl, http2_enabled, is_system_proxy, include_www,
			          COALESCE(upstream_target, ''), COALESCE(proxy_type, 'upstream'), redirect_url, redirect_code
		`, agentID, req.Domain, "%."+strings.TrimPrefix(req.Domain, "*.")).Scan(
			&proxyID, &proxyDomain, &forceSSL, &http2Enabled, &isSystemProxy, &includeWWW,
			&upstream, &proxyType, &redirectURL, &redirectCode)

		if err == nil {
			h.logger.Info("Updated proxy_hosts for wildcard SSL",
				zap.String("domain", req.Domain),
				zap.String("proxy_domain", proxyDomain),
				zap.String("proxy_id", proxyID.String()),
				zap.String("proxy_type", proxyType),
			)

			// Regenerate and dispatch nginx config with SSL enabled
			if isSystemProxy {
				go h.dispatchInfraPilotProxyConfig(c.Request.Context(), agentID, proxyID, proxyDomain, true, http2Enabled, true)
			} else {
				// For regular proxies (upstream or redirect), dispatch full config
				go h.dispatchProxyConfigFull(c.Request.Context(), agentID, proxyID, proxyDomain, upstream,
					proxyType, redirectURL, redirectCode, forceSSL, http2Enabled, includeWWW, true)
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"success":   true,
			"message":   message,
			"domain":    req.Domain,
			"cert_path": certPath,
			"key_path":  keyPath,
		})
	} else {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   message,
		})
	}
}

// getDNSChallenge gets the current DNS challenge info for a domain
// GET /api/v1/ssl/dns-challenge/:domain
func (h *Handler) getDNSChallenge(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	domain := c.Param("domain")

	if domain == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "domain is required"})
		return
	}

	// Find an active agent
	var agentID uuid.UUID
	err := h.db.QueryRow(c.Request.Context(), `
		SELECT id FROM agents WHERE org_id = $1 AND status = 'active' LIMIT 1
	`, orgID).Scan(&agentID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no active agent found"})
		return
	}

	agentIDStr := agentID.String()

	if !agentgrpc.IsAgentConnected(agentIDStr) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "agent not connected"})
		return
	}

	cmd := agentgrpc.NewSSLGetDNSChallengeCommand(domain)
	resp, err := agentgrpc.SendCommand(agentIDStr, cmd, 15*time.Second)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to get DNS challenge: %v", err)})
		return
	}

	// Parse response - handle both direct struct and JSON unmarshaled map
	success := false
	message := ""
	var challenge DNSChallengeInfo

	if result, err := resp.GetCommandResult(); err == nil && result != nil {
		success = result.Success
		message = result.Message
		if result.Data != nil {
			json.Unmarshal(result.Data, &challenge)
		}
	}

	if success {
		c.JSON(http.StatusOK, gin.H{
			"success":    true,
			"domain":     domain,
			"txt_record": challenge.TXTRecord,
			"txt_name":   challenge.TXTName,
			"created_at": challenge.CreatedAt,
		})
	} else {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   message,
		})
	}
}

// verifyDNSTXTRecord checks if a TXT record exists for the domain
// GET /api/v1/ssl/dns-challenge/verify/:domain
func (h *Handler) verifyDNSTXTRecord(c *gin.Context) {
	domain := c.Param("domain")
	if domain == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "domain is required"})
		return
	}

	expectedValue := c.Query("value")

	// Build the TXT record name
	txtName := "_acme-challenge." + domain
	if strings.HasPrefix(domain, "*.") {
		txtName = "_acme-challenge." + strings.TrimPrefix(domain, "*.")
	}

	// Lookup TXT records
	records, err := net.LookupTXT(txtName)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"verified":   false,
			"txt_name":   txtName,
			"error":      fmt.Sprintf("DNS lookup failed: %v", err),
			"found":      []string{},
		})
		return
	}

	// Check if expected value is present
	verified := false
	if expectedValue != "" {
		for _, record := range records {
			if record == expectedValue {
				verified = true
				break
			}
		}
	} else {
		verified = len(records) > 0
	}

	c.JSON(http.StatusOK, gin.H{
		"verified": verified,
		"txt_name": txtName,
		"found":    records,
		"expected": expectedValue,
	})
}

// uploadSSLCertificate accepts raw PEM certificate + key content, validates them,
// writes them to disk, and optionally applies them to a proxy host.
// POST /api/v1/ssl/upload
func (h *Handler) uploadSSLCertificate(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)

	var req struct {
		CertPEM string `json:"cert_pem" binding:"required"`
		KeyPEM  string `json:"key_pem" binding:"required"`
		AgentID string `json:"agent_id"`
		ProxyID string `json:"proxy_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Size limits — 64 KB each
	const maxPEMSize = 64 * 1024
	if len(req.CertPEM) > maxPEMSize {
		c.JSON(http.StatusBadRequest, gin.H{"error": "certificate too large (max 64 KB)"})
		return
	}
	if len(req.KeyPEM) > maxPEMSize {
		c.JSON(http.StatusBadRequest, gin.H{"error": "private key too large (max 64 KB)"})
		return
	}

	// Validate and parse certificate
	certInfo, cert, err := validateUploadedCertPEM([]byte(req.CertPEM))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid certificate: %v", err)})
		return
	}

	// Validate and parse private key
	privKey, err := validateUploadedKeyPEM([]byte(req.KeyPEM))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid private key: %v", err)})
		return
	}

	// Verify the private key matches the certificate's public key
	if err := verifyCertKeyMatch(cert, privKey); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "private key does not match the certificate"})
		return
	}

	// Determine domain for directory name
	domain := certInfo.Subject
	if len(certInfo.SANs) > 0 {
		domain = certInfo.SANs[0]
	}
	cleanDomain := strings.TrimPrefix(domain, "*.")
	// Sanitise directory component — only allow hostname chars
	for _, ch := range cleanDomain {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') || ch == '-' || ch == '.') {
			c.JSON(http.StatusBadRequest, gin.H{"error": "certificate domain contains invalid characters"})
			return
		}
	}

	// Save to /data/certs/<domain>/
	certDir := filepath.Join("/data/certs", cleanDomain)
	if err := os.MkdirAll(certDir, 0700); err != nil {
		h.logger.Error("Failed to create cert directory", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create certificate directory"})
		return
	}

	certPath := filepath.Join(certDir, "fullchain.pem")
	keyPath := filepath.Join(certDir, "privkey.pem")

	if err := os.WriteFile(certPath, []byte(strings.TrimSpace(req.CertPEM)+"\n"), 0644); err != nil {
		h.logger.Error("Failed to write certificate", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save certificate"})
		return
	}
	if err := os.WriteFile(keyPath, []byte(strings.TrimSpace(req.KeyPEM)+"\n"), 0600); err != nil {
		os.Remove(certPath)
		h.logger.Error("Failed to write private key", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save private key"})
		return
	}

	h.logger.Info("Custom SSL certificate uploaded",
		zap.String("domain", domain),
		zap.String("issuer", certInfo.Issuer),
		zap.Time("expires_at", certInfo.ExpiresAt),
	)

	// Optionally apply directly to a proxy host
	if req.AgentID != "" && req.ProxyID != "" {
		agentID, err1 := uuid.Parse(req.AgentID)
		proxyID, err2 := uuid.Parse(req.ProxyID)
		if err1 == nil && err2 == nil {
			var proxyDomain, upstream string
			err = h.db.QueryRow(c.Request.Context(), `
				SELECT ph.domain, ph.upstream_target
				FROM proxy_hosts ph
				JOIN agents a ON ph.agent_id = a.id
				WHERE ph.id = $1 AND ph.agent_id = $2 AND a.org_id = $3
			`, proxyID, agentID, orgID).Scan(&proxyDomain, &upstream)

			if err == nil {
				_, err = h.db.Exec(c.Request.Context(), `
					UPDATE proxy_hosts SET
						ssl_enabled   = true,
						force_ssl     = true,
						http2_enabled = true,
						ssl_source    = 'external',
						ssl_cert_path = $1,
						ssl_key_path  = $2,
						status        = 'active',
						updated_at    = NOW()
					WHERE id = $3
				`, certPath, keyPath, proxyID)
				if err != nil {
					h.logger.Error("Failed to update proxy SSL settings", zap.Error(err))
				} else {
					go h.dispatchProxyConfigWithCert(context.Background(), agentID, proxyID,
						proxyDomain, upstream, true, true, true, certPath, keyPath)
				}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"cert_path":   certPath,
		"key_path":    keyPath,
		"domain":      domain,
		"issuer":      certInfo.Issuer,
		"expires_at":  certInfo.ExpiresAt.Format(time.RFC3339),
		"sans":        certInfo.SANs,
		"is_wildcard": certInfo.IsWildcard,
		"message":     "Certificate uploaded and validated successfully",
	})
}

// validateUploadedCertPEM validates and parses PEM-encoded certificate content.
// Returns parsed CertificateInfo, the x509.Certificate, and any validation error.
func validateUploadedCertPEM(data []byte) (*CertificateInfo, *x509.Certificate, error) {
	data = bytes.TrimSpace(data)

	// Must start with PEM certificate header — reject anything else outright
	if !bytes.HasPrefix(data, []byte("-----BEGIN CERTIFICATE-----")) {
		return nil, nil, fmt.Errorf("must start with -----BEGIN CERTIFICATE----- (got non-certificate PEM or binary data)")
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return nil, nil, fmt.Errorf("failed to decode PEM block — check for encoding errors")
	}
	if block.Type != "CERTIFICATE" {
		return nil, nil, fmt.Errorf("expected PEM type CERTIFICATE, got %q", block.Type)
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid X.509 structure: %w", err)
	}

	info := &CertificateInfo{
		Subject:   cert.Subject.CommonName,
		Issuer:    cert.Issuer.CommonName,
		SANs:      cert.DNSNames,
		ExpiresAt: cert.NotAfter,
	}
	for _, san := range cert.DNSNames {
		if strings.HasPrefix(san, "*.") {
			info.IsWildcard = true
			break
		}
	}
	return info, cert, nil
}

// validateUploadedKeyPEM validates a PEM-encoded private key.
// Accepts PKCS#8 (PRIVATE KEY), PKCS#1 RSA (RSA PRIVATE KEY), SEC1 EC (EC PRIVATE KEY).
// Returns the parsed key or an error — does NOT accept encrypted keys.
func validateUploadedKeyPEM(data []byte) (crypto.PrivateKey, error) {
	data = bytes.TrimSpace(data)

	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block — check for encoding errors")
	}

	// Reject encrypted keys
	if strings.Contains(block.Type, "ENCRYPTED") || block.Headers["Proc-Type"] != "" {
		return nil, fmt.Errorf("encrypted private keys are not supported — please decrypt the key first")
	}

	switch block.Type {
	case "PRIVATE KEY": // PKCS#8 (RSA, EC, Ed25519)
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("invalid PKCS#8 private key: %w", err)
		}
		return key, nil
	case "RSA PRIVATE KEY": // PKCS#1
		key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("invalid RSA private key: %w", err)
		}
		return key, nil
	case "EC PRIVATE KEY": // SEC1
		key, err := x509.ParseECPrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("invalid EC private key: %w", err)
		}
		return key, nil
	default:
		return nil, fmt.Errorf("unsupported PEM type %q — expected PRIVATE KEY, RSA PRIVATE KEY, or EC PRIVATE KEY", block.Type)
	}
}

// verifyCertKeyMatch checks that a private key corresponds to the certificate's public key
// by marshalling both public keys to DER and comparing byte-for-byte.
func verifyCertKeyMatch(cert *x509.Certificate, privKey crypto.PrivateKey) error {
	var privPub crypto.PublicKey
	switch k := privKey.(type) {
	case *rsa.PrivateKey:
		privPub = &k.PublicKey
	case *ecdsa.PrivateKey:
		privPub = &k.PublicKey
	case ed25519.PrivateKey:
		privPub = k.Public()
	default:
		return fmt.Errorf("unsupported private key type %T", privKey)
	}

	certPubDER, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
	if err != nil {
		return fmt.Errorf("failed to encode certificate public key: %w", err)
	}
	privPubDER, err := x509.MarshalPKIXPublicKey(privPub)
	if err != nil {
		return fmt.Errorf("failed to encode private key public part: %w", err)
	}

	if !bytes.Equal(certPubDER, privPubDER) {
		return fmt.Errorf("key mismatch")
	}
	return nil
}

// parseCertificateFile reads and parses a PEM certificate file
func parseCertificateFile(certPath string) (*CertificateInfo, error) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("read certificate: %w", err)
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse certificate: %w", err)
	}

	info := &CertificateInfo{
		Subject:   cert.Subject.CommonName,
		Issuer:    cert.Issuer.CommonName,
		SANs:      cert.DNSNames,
		ExpiresAt: cert.NotAfter,
	}

	// Check if any SAN is a wildcard
	for _, san := range cert.DNSNames {
		if strings.HasPrefix(san, "*.") {
			info.IsWildcard = true
			break
		}
	}

	return info, nil
}
