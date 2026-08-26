package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/infrapilot/backend/internal/license"
)

// SetupStatusResponse represents the setup status
type SetupStatusResponse struct {
	SetupRequired     bool   `json:"setup_required"`
	LicenseConfigured bool   `json:"license_configured"`
	AdminCreated      bool   `json:"admin_created"`
	UserCount         int    `json:"user_count"`
	LicenseError      string `json:"license_error,omitempty"`
}

// SetupRequest represents the initial admin setup request
type SetupRequest struct {
	Email      string `json:"email" binding:"required,email"`
	Password   string `json:"password" binding:"required,min=8"`
	SetupToken string `json:"setup_token" binding:"required"`
}

// SetupLicenseRequest represents the license key submission during setup
type SetupLicenseRequest struct {
	Key        string `json:"key" binding:"required"`
	SetupToken string `json:"setup_token" binding:"required"`
}

// setupTokenFile is where the setup token lives under DATA_DIR while no admin exists yet.
const setupTokenFile = "setup_token"

// ensureSetupToken closes the "whoever visits /setup first becomes admin" race: without
// this, POST /api/v1/setup and POST /api/v1/setup/license have no protection beyond a
// SELECT COUNT(*) FROM users check, so a remote attacker who reaches a freshly-installed
// box before its owner finishes clicking through setup can claim super_admin outright.
//
// Generates a random token on first call and persists it to DATA_DIR (same file-marker
// pattern telemetry.go already uses), logging it once at creation so the real owner can
// read it via `docker logs`/filesystem access, something a remote attacker can't do.
// Idempotent: later calls just read the existing file back. Fails closed on any error, the
// caller must reject the request rather than silently skip the check.
func ensureSetupToken(dataDir string, logger *zap.Logger) (string, error) {
	path := filepath.Join(dataDir, setupTokenFile)
	if b, err := os.ReadFile(path); err == nil {
		return strings.TrimSpace(string(b)), nil
	}

	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw)

	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(token), 0o600); err != nil {
		return "", err
	}

	logger.Info("=================================================================")
	logger.Info("Setup required — a setup token is needed to create the admin account")
	logger.Info("Setup token: " + token)
	logger.Info("=================================================================")

	return token, nil
}

// clearSetupToken removes the token file once the first admin exists. It's meaningless
// afterward, both handlers already 400 once a user exists, but leaving it around is
// needless residue.
func clearSetupToken(dataDir string) {
	_ = os.Remove(filepath.Join(dataDir, setupTokenFile))
}

// getSetupStatus checks if initial setup is required (no users exist)
func (h *Handler) getSetupStatus(c *gin.Context) {
	var count int
	err := h.db.QueryRow(c.Request.Context(), `SELECT COUNT(*) FROM users`).Scan(&count)
	if err != nil {
		h.logger.Error("Failed to count users")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check setup status"})
		return
	}

	// Determine if a valid license key is active.
	// A real key is anything other than the setup/community placeholder keys.
	currentKey := h.license.GetKey()
	licenseConfigured := currentKey != "" &&
		currentKey != license.CommunityModeKey &&
		currentKey != license.SetupModeKey

	// If an ENV key was explicitly set but is not valid (server fell back to setup mode),
	// surface that error to the frontend so it can show a helpful message.
	var licenseError string
	if h.cfg.LicenseKey != "" && !licenseConfigured {
		licenseError = "The configured license key is invalid or could not be validated. Please enter a valid key, or get a free Community Edition key at infrapilot.org."
	}

	c.JSON(http.StatusOK, SetupStatusResponse{
		SetupRequired:     count == 0,
		LicenseConfigured: licenseConfigured,
		AdminCreated:      count > 0,
		UserCount:         count,
		LicenseError:      licenseError,
	})
}

// setupLicense validates and stores a license key during setup
func (h *Handler) setupLicense(c *gin.Context) {
	// Guard: if users exist, setup already completed
	var count int
	err := h.db.QueryRow(c.Request.Context(), `SELECT COUNT(*) FROM users`).Scan(&count)
	if err != nil {
		h.logger.Error("Failed to count users")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check setup status"})
		return
	}
	if count > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "setup already completed"})
		return
	}

	var req SetupLicenseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	expectedToken, err := ensureSetupToken(h.cfg.DataDir, h.logger)
	if err != nil {
		h.logger.Error("Failed to verify setup token", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to verify setup token"})
		return
	}
	if req.SetupToken != expectedToken {
		c.JSON(http.StatusForbidden, gin.H{"error": "invalid setup token — check `docker logs` for the value printed at startup"})
		return
	}

	// Validate the license key against infrapilot.org
	client, err := license.NewClient(req.Key, h.cfg.DataDir, h.version, h.logger)
	if err != nil {
		h.logger.Error("Failed to initialize license client", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to initialize license validation"})
		return
	}
	resp, err := client.Validate()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "license validation failed: " + err.Error()})
		return
	}
	if !resp.Valid {
		errMsg := "invalid license key"
		if resp.Error != "" {
			errMsg = resp.Error
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": errMsg})
		return
	}

	// Ensure default org exists first (same pattern as createInitialAdmin)
	orgID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	_, err = h.db.Exec(c.Request.Context(), `
		INSERT INTO organizations (id, name, slug)
		VALUES ($1, 'Default Organization', 'default')
		ON CONFLICT (id) DO NOTHING
	`, orgID)
	if err != nil {
		h.logger.Error("Failed to create organization", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create organization"})
		return
	}

	// Upsert license key into system_settings
	settingValue, err := json.Marshal(map[string]interface{}{
		"key":        req.Key,
		"tier":       resp.Tier,
		"max_agents": resp.MaxAgents,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to encode license data"})
		return
	}
	_, err = h.db.Exec(c.Request.Context(), `
		INSERT INTO system_settings (org_id, setting_key, setting_value)
		VALUES ($1, 'license_key', $2)
		ON CONFLICT (org_id, setting_key) DO UPDATE SET setting_value = EXCLUDED.setting_value
	`, orgID, string(settingValue))
	if err != nil {
		h.logger.Error("Failed to save license key", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save license key"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"valid":      true,
		"tier":       resp.Tier,
		"max_agents": resp.MaxAgents,
		"features":   resp.Features,
	})
}

// createInitialAdmin creates the first admin user during setup
func (h *Handler) createInitialAdmin(c *gin.Context) {
	// First check if any users exist
	var count int
	err := h.db.QueryRow(c.Request.Context(), `SELECT COUNT(*) FROM users`).Scan(&count)
	if err != nil {
		h.logger.Error("Failed to count users")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check setup status"})
		return
	}

	if count > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "setup already completed"})
		return
	}

	var req SetupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	expectedToken, err := ensureSetupToken(h.cfg.DataDir, h.logger)
	if err != nil {
		h.logger.Error("Failed to verify setup token", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to verify setup token"})
		return
	}
	if req.SetupToken != expectedToken {
		c.JSON(http.StatusForbidden, gin.H{"error": "invalid setup token — check `docker logs` for the value printed at startup"})
		return
	}

	// Validate password strength
	if len(req.Password) < 8 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password must be at least 8 characters"})
		return
	}

	// Hash password
	passwordHash, err := h.auth.HashPassword(req.Password)
	if err != nil {
		h.logger.Error("Failed to hash password")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create account"})
		return
	}

	// Ensure default organization exists
	orgID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	_, err = h.db.Exec(c.Request.Context(), `
		INSERT INTO organizations (id, name, slug)
		VALUES ($1, 'Default Organization', 'default')
		ON CONFLICT (id) DO NOTHING
	`, orgID)
	if err != nil {
		h.logger.Error("Failed to create organization")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create organization"})
		return
	}

	// Create admin user
	userID := uuid.New()
	_, err = h.db.Exec(c.Request.Context(), `
		INSERT INTO users (id, org_id, email, password_hash, role, mfa_enabled)
		VALUES ($1, $2, $3, $4, 'super_admin', false)
	`, userID, orgID, req.Email, passwordHash)

	if err != nil {
		h.logger.Error("Failed to create admin user")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create account"})
		return
	}

	clearSetupToken(h.cfg.DataDir)

	// Audit log for setup
	h.db.Exec(c.Request.Context(), `
		INSERT INTO audit_logs (org_id, user_id, action, resource_type, resource_id, ip_address, user_agent)
		VALUES ($1, $2, 'setup.initial_admin_created', 'user', $2, $3, $4)
	`, orgID, userID, c.ClientIP(), c.Request.UserAgent())

	// Generate tokens for immediate login
	accessToken, err := h.auth.GenerateAccessToken(userID, orgID, req.Email, "super_admin", false)
	if err != nil {
		// User created but tokens failed - they can still login
		c.JSON(http.StatusOK, gin.H{
			"message": "Admin account created successfully. Please login.",
			"user_id": userID,
		})
		return
	}

	refreshToken, _ := h.auth.GenerateRefreshToken()

	c.JSON(http.StatusCreated, gin.H{
		"message":       "Admin account created successfully",
		"user_id":       userID,
		"access_token":  accessToken,
		"refresh_token": refreshToken,
	})
}

// maskLicenseKey masks the middle segments of a license key for display.
// e.g. "IP-CE-DTSE-QPNW-WG7U" → "IP-CE-****-****-WG7U"
func maskLicenseKey(key string) string {
	parts := strings.Split(key, "-")
	if len(parts) < 5 {
		if len(key) > 4 {
			return key[:4] + strings.Repeat("*", len(key)-4)
		}
		return "****"
	}
	masked := make([]string, len(parts))
	for i, p := range parts {
		if i == 0 || i == 1 || i == len(parts)-1 {
			masked[i] = p
		} else {
			masked[i] = strings.Repeat("*", len(p))
		}
	}
	return strings.Join(masked, "-")
}

// LicenseSettingsResponse is returned by GET /settings/license
type LicenseSettingsResponse struct {
	Valid      bool     `json:"valid"`
	Tier       string   `json:"tier"`
	MaxAgents  int      `json:"max_agents"`
	Features   []string `json:"features"`
	ExpiresAt  *string  `json:"expires_at"`
	UpgradeURL string   `json:"upgrade_url"`
	KeyDisplay string   `json:"key_display"` // masked key, empty if none
	KeySource  string   `json:"key_source"`  // "env", "database", "setup_mode"
	Edition    string   `json:"edition"`     // "community" or "enterprise" — the running image
}

// getLicenseSettings returns the current license info for the settings page.
// This is a super_admin-only authenticated endpoint.
func (h *Handler) getLicenseSettings(c *gin.Context) {
	resp, err := h.license.Validate()
	if err != nil {
		c.JSON(http.StatusOK, LicenseSettingsResponse{
			Valid:      false,
			Tier:       "unknown",
			MaxAgents:  0,
			Features:   []string{},
			UpgradeURL: "https://infrapilot.org/billing",
			KeySource:  "setup_mode",
			Edition:    Edition,
		})
		return
	}

	// Determine key source and compute a masked display value.
	keyDisplay := ""
	keySource := "setup_mode"

	if h.cfg.LicenseKey != "" {
		keyDisplay = maskLicenseKey(h.cfg.LicenseKey)
		keySource = "env"
	} else if os.Getenv("LICENSE_OFFLINE") == "true" {
		keySource = "offline"
	} else {
		// Try to get the saved key from DB.
		var savedKey string
		_ = h.db.QueryRow(c.Request.Context(), `
			SELECT setting_value->>'key' FROM system_settings
			WHERE org_id = '00000000-0000-0000-0000-000000000001'
			AND setting_key = 'license_key'
		`).Scan(&savedKey)
		if savedKey != "" {
			keyDisplay = maskLicenseKey(savedKey)
			keySource = "database"
		}
	}

	c.JSON(http.StatusOK, LicenseSettingsResponse{
		Valid:      resp.Valid,
		Tier:       resp.Tier,
		MaxAgents:  resp.MaxAgents,
		Features:   resp.Features,
		ExpiresAt:  resp.ExpiresAt,
		UpgradeURL: "https://infrapilot.org/billing",
		KeyDisplay: keyDisplay,
		KeySource:  keySource,
		Edition:    Edition,
	})
}

// updateLicenseKey validates a new license key, saves it to system_settings,
// and updates the in-memory license client — no restart required.
func (h *Handler) updateLicenseKey(c *gin.Context) {
	// Reject if key is locked to the environment — changes must be done via env var.
	if h.cfg.LicenseKey != "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "License key is set via the LICENSE_KEY environment variable and cannot be changed from the UI. Remove the env var to manage the license here.",
		})
		return
	}

	var req SetupLicenseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate the new key against infrapilot.org and swap it in-memory.
	resp, err := h.license.UpdateKey(req.Key)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "license validation failed: " + err.Error()})
		return
	}
	if !resp.Valid {
		errMsg := "invalid license key"
		if resp.Error != "" {
			errMsg = resp.Error
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": errMsg})
		return
	}

	// Ensure default org exists.
	orgID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	_, err = h.db.Exec(c.Request.Context(), `
		INSERT INTO organizations (id, name, slug)
		VALUES ($1, 'Default Organization', 'default')
		ON CONFLICT (id) DO NOTHING
	`, orgID)
	if err != nil {
		h.logger.Error("Failed to ensure organization exists", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to ensure organization"})
		return
	}

	// Upsert license key into system_settings.
	settingValue, err := json.Marshal(map[string]interface{}{
		"key":        req.Key,
		"tier":       resp.Tier,
		"max_agents": resp.MaxAgents,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to encode license data"})
		return
	}
	_, err = h.db.Exec(c.Request.Context(), `
		INSERT INTO system_settings (org_id, setting_key, setting_value)
		VALUES ($1, 'license_key', $2)
		ON CONFLICT (org_id, setting_key) DO UPDATE SET setting_value = EXCLUDED.setting_value
	`, orgID, string(settingValue))
	if err != nil {
		h.logger.Error("Failed to save license key", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save license key"})
		return
	}

	h.logger.Info("License key updated via settings",
		zap.String("tier", resp.Tier),
		zap.Int("max_agents", resp.MaxAgents),
	)

	c.JSON(http.StatusOK, LicenseSettingsResponse{
		Valid:      resp.Valid,
		Tier:       resp.Tier,
		MaxAgents:  resp.MaxAgents,
		Features:   resp.Features,
		ExpiresAt:  resp.ExpiresAt,
		UpgradeURL: "https://infrapilot.org/billing",
		KeyDisplay: maskLicenseKey(req.Key),
		KeySource:  "database",
		Edition:    Edition,
	})
}
