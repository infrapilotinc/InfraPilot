package api

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// letsEncryptLiveDir is where the agent stores issued certificates (shared volume).
const letsEncryptLiveDir = "/etc/letsencrypt/live"

// certDirName returns the live/ folder name the agent uses for a certificate
// domain. Wildcards get their own "_wildcard.<base>" folder so a DNS-01 wildcard
// and an HTTP-01 apex cert for the same base domain never overwrite each other.
// Underscores aren't valid in hostnames, so this can't collide with a real domain.
func certDirName(domain string) string {
	if strings.HasPrefix(domain, "*.") {
		return "_wildcard." + strings.TrimPrefix(domain, "*.")
	}
	return domain
}

// certDomainPaths returns the fullchain/privkey paths for a certificate domain.
func certDomainPaths(domain string) (string, string) {
	dir := filepath.Join(letsEncryptLiveDir, certDirName(domain))
	return filepath.Join(dir, "fullchain.pem"), filepath.Join(dir, "privkey.pem")
}

// certCoversDomain reports whether a certificate with the given SANs is valid for
// domain. A wildcard SAN matches exactly one label, and a wildcard proxy domain
// (*.example.com) is only covered by the identical wildcard SAN.
func certCoversDomain(sans []string, domain string) bool {
	domain = strings.ToLower(domain)
	for _, san := range sans {
		san = strings.ToLower(san)
		if san == domain {
			return true
		}
		if strings.HasPrefix(san, "*.") && !strings.HasPrefix(domain, "*.") {
			base := san[1:] // ".example.com"
			if strings.HasSuffix(domain, base) {
				label := strings.TrimSuffix(domain, base)
				if label != "" && !strings.Contains(label, ".") {
					return true
				}
			}
		}
	}
	return false
}

// autoCertCandidates lists, in preference order, the Let's Encrypt folders that
// could hold a cert for domain: its own, then a wildcard for its parent, then the
// legacy location where wildcards used to be saved (the bare parent folder).
func autoCertCandidates(domain string) []string {
	var domains []string
	if strings.HasPrefix(domain, "*.") {
		base := strings.TrimPrefix(domain, "*.")
		domains = []string{domain, base}
	} else {
		domains = []string{domain}
		if parts := strings.Split(domain, "."); len(parts) > 2 {
			parent := strings.Join(parts[1:], ".")
			domains = append(domains, "*."+parent, parent)
		}
	}
	return domains
}

// resolveAutoCert finds the Let's Encrypt certificate that actually covers domain.
// found is false when no issued cert on disk covers it; the returned paths are then
// the domain's own expected location, so nginx -t reports the missing file clearly.
func resolveAutoCert(domain string) (certPath, keyPath string, found bool) {
	for _, candidate := range autoCertCandidates(domain) {
		cp, kp := certDomainPaths(candidate)
		info, err := parseCertificateFile(cp)
		if err != nil {
			continue
		}
		if _, err := os.Stat(kp); err != nil {
			continue
		}
		if certCoversDomain(info.SANs, domain) {
			return cp, kp, true
		}
	}
	cp, kp := certDomainPaths(domain)
	return cp, kp, false
}

// ProxyCertificateOption is a certificate that can be (or is) used by a proxy.
type ProxyCertificateOption struct {
	ID           *uuid.UUID `json:"id,omitempty"`
	Name         string     `json:"name"`
	CertPath     string     `json:"cert_path"`
	KeyPath      string     `json:"key_path"`
	Issuer       string     `json:"issuer,omitempty"`
	Subject      string     `json:"subject,omitempty"`
	SANs         []string   `json:"sans"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	DaysLeft     int        `json:"days_left"`
	IsWildcard   bool       `json:"is_wildcard"`
	CoversDomain bool       `json:"covers_domain"`
}

// ProxyCertificateResponse describes the cert a proxy is using and what it could use.
type ProxyCertificateResponse struct {
	Domain     string                   `json:"domain"`
	SSLEnabled bool                     `json:"ssl_enabled"`
	Mode       string                   `json:"mode"` // "auto" or "pinned"
	Current    *ProxyCertificateOption  `json:"current,omitempty"`
	Error      string                   `json:"error,omitempty"`
	Available  []ProxyCertificateOption `json:"available"`
}

func certOptionFromFile(certPath, keyPath, domain string) (*ProxyCertificateOption, error) {
	info, err := parseCertificateFile(certPath)
	if err != nil {
		return nil, err
	}
	expires := info.ExpiresAt
	return &ProxyCertificateOption{
		Name:         info.Subject,
		CertPath:     certPath,
		KeyPath:      keyPath,
		Issuer:       info.Issuer,
		Subject:      info.Subject,
		SANs:         info.SANs,
		ExpiresAt:    &expires,
		DaysLeft:     int(time.Until(expires).Hours() / 24),
		IsWildcard:   info.IsWildcard,
		CoversDomain: certCoversDomain(info.SANs, domain),
	}, nil
}

// listCertificateOptions gathers registered certificates plus anything on disk under
// the Let's Encrypt live dir, de-duplicated by cert path and evaluated against domain.
func (h *Handler) listCertificateOptions(c *gin.Context, orgID uuid.UUID, domain string) []ProxyCertificateOption {
	options := []ProxyCertificateOption{}
	seen := map[string]bool{}

	rows, err := h.db.Query(c.Request.Context(), `
		SELECT id, name, cert_path, key_path FROM ssl_certificates WHERE org_id = $1 ORDER BY domain ASC
	`, orgID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id uuid.UUID
			var name, certPath, keyPath string
			if err := rows.Scan(&id, &name, &certPath, &keyPath); err != nil {
				continue
			}
			opt, err := certOptionFromFile(certPath, keyPath, domain)
			if err != nil {
				continue
			}
			certID := id
			opt.ID = &certID
			opt.Name = name
			options = append(options, *opt)
			seen[certPath] = true
		}
	} else {
		h.logger.Warn("Failed to list registered certificates", zap.Error(err))
	}

	entries, err := os.ReadDir(letsEncryptLiveDir)
	if err != nil {
		return options
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		certPath := filepath.Join(letsEncryptLiveDir, entry.Name(), "fullchain.pem")
		keyPath := filepath.Join(letsEncryptLiveDir, entry.Name(), "privkey.pem")
		if seen[certPath] {
			continue
		}
		if _, err := os.Stat(keyPath); err != nil {
			continue
		}
		opt, err := certOptionFromFile(certPath, keyPath, domain)
		if err != nil {
			continue
		}
		options = append(options, *opt)
		seen[certPath] = true
	}
	return options
}

// getProxyCertificate returns the certificate a proxy is serving and the certificates
// it could be switched to.
// GET /api/v1/agents/:id/proxies/:pid/certificate
func (h *Handler) getProxyCertificate(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	agentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}
	proxyID, err := uuid.Parse(c.Param("pid"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid proxy ID"})
		return
	}

	var domain string
	var sslEnabled bool
	var certPath, keyPath *string
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT ph.domain, ph.ssl_enabled, ph.ssl_cert_path, ph.ssl_key_path
		FROM proxy_hosts ph JOIN agents a ON ph.agent_id = a.id
		WHERE ph.id = $1 AND ph.agent_id = $2 AND a.org_id = $3
	`, proxyID, agentID, orgID).Scan(&domain, &sslEnabled, &certPath, &keyPath)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "proxy host not found"})
		return
	}

	resp := ProxyCertificateResponse{
		Domain:     domain,
		SSLEnabled: sslEnabled,
		Mode:       "auto",
		Available:  h.listCertificateOptions(c, orgID, domain),
	}

	var currentCert, currentKey string
	if certPath != nil && *certPath != "" && keyPath != nil && *keyPath != "" {
		resp.Mode = "pinned"
		currentCert, currentKey = *certPath, *keyPath
	} else {
		var found bool
		currentCert, currentKey, found = resolveAutoCert(domain)
		if !found && sslEnabled {
			resp.Error = "No issued certificate on this server covers " + domain
		}
	}

	if sslEnabled && resp.Error == "" {
		opt, err := certOptionFromFile(currentCert, currentKey, domain)
		if err != nil {
			resp.Error = "Could not read certificate at " + currentCert
		} else {
			for _, a := range resp.Available {
				if a.CertPath == opt.CertPath && a.ID != nil {
					opt.ID, opt.Name = a.ID, a.Name
				}
			}
			resp.Current = opt
			if !opt.CoversDomain {
				resp.Error = "The selected certificate does not cover " + domain
			}
		}
	}

	c.JSON(http.StatusOK, resp)
}

// SetProxyCertificateRequest pins a proxy to a certificate, or (Auto) clears the pin
// so the best-matching Let's Encrypt cert is picked automatically.
type SetProxyCertificateRequest struct {
	Auto     bool   `json:"auto"`
	CertPath string `json:"cert_path"`
}

// setProxyCertificate switches the certificate a proxy uses.
// PUT /api/v1/agents/:id/proxies/:pid/certificate
func (h *Handler) setProxyCertificate(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	userID := c.MustGet("user_id").(uuid.UUID)
	agentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}
	proxyID, err := uuid.Parse(c.Param("pid"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid proxy ID"})
		return
	}

	var req SetProxyCertificateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !req.Auto && req.CertPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cert_path is required unless auto is true"})
		return
	}

	var domain string
	var isSystemProxy bool
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT ph.domain, COALESCE(ph.is_system_proxy, false)
		FROM proxy_hosts ph JOIN agents a ON ph.agent_id = a.id
		WHERE ph.id = $1 AND ph.agent_id = $2 AND a.org_id = $3
	`, proxyID, agentID, orgID).Scan(&domain, &isSystemProxy)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "proxy host not found"})
		return
	}

	var certPath, keyPath *string
	var certID *uuid.UUID // ssl_certificates row, so deleting that cert is guarded
	sslSource := "letsencrypt"
	if req.Auto {
		if _, _, found := resolveAutoCert(domain); !found {
			c.JSON(http.StatusBadRequest, gin.H{"error": "no issued certificate on this server covers " + domain})
			return
		}
	} else {
		// Only paths we discovered ourselves are accepted, so this can't be used to
		// point nginx at arbitrary files.
		var chosen *ProxyCertificateOption
		for _, opt := range h.listCertificateOptions(c, orgID, domain) {
			if opt.CertPath == req.CertPath {
				o := opt
				chosen = &o
				break
			}
		}
		if chosen == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown certificate"})
			return
		}
		if !chosen.CoversDomain {
			c.JSON(http.StatusBadRequest, gin.H{"error": "certificate does not cover " + domain})
			return
		}
		certPath, keyPath = &chosen.CertPath, &chosen.KeyPath
		certID = chosen.ID
		sslSource = "external"
		if chosen.IsWildcard {
			sslSource = "wildcard"
		}
	}

	_, err = h.db.Exec(c.Request.Context(), `
		UPDATE proxy_hosts SET
			ssl_enabled = true,
			ssl_source = $1,
			ssl_cert_path = $2,
			ssl_key_path = $3,
			ssl_certificate_id = $4,
			updated_at = NOW()
		WHERE id = $5
	`, sslSource, certPath, keyPath, certID, proxyID)
	if err != nil {
		h.logger.Error("Failed to update proxy certificate", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update proxy"})
		return
	}

	if isSystemProxy {
		// Regenerates the system proxy config (with its basic auth) from the DB row.
		// Background context: the request context is cancelled once we respond.
		go h.dispatchHtpasswd(context.Background(), agentID, proxyID)
	} else {
		go h.redispatchProxyConfig(context.Background(), agentID, proxyID)
	}

	h.auditLog(c, userID, orgID, "proxy.certificate", "proxy_host", proxyID, gin.H{
		"domain":    domain,
		"auto":      req.Auto,
		"cert_path": req.CertPath,
	})

	c.JSON(http.StatusOK, gin.H{"message": "certificate updated", "domain": domain})
}
