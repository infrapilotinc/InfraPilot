package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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

// SetupLicenseRequest represents the license key submission during setup. No setup
// token here — activating a license grants no access by itself, only createInitialAdmin
// (the actual privilege boundary) needs to check it, so the token is asked for once,
// right where it matters, instead of twice.
type SetupLicenseRequest struct {
	Key string `json:"key" binding:"required"`
}

// setupTokenFile is where the setup token lives under DATA_DIR while no admin exists yet.
const setupTokenFile = "setup_token"

// EnsureSetupToken closes the "whoever visits /setup first becomes admin" race: without
// this, POST /api/v1/setup and POST /api/v1/setup/license have no protection beyond a
// SELECT COUNT(*) FROM users check, so a remote attacker who reaches a freshly-installed
// box before its owner finishes clicking through setup can claim super_admin outright.
//
// Generates a random token on first call and persists it to DATA_DIR (same file-marker
// pattern telemetry.go already uses), logging it once at creation so the real owner can
// read it via `docker logs`/filesystem access, something a remote attacker can't do.
// Idempotent: later calls just read the existing file back. Fails closed on any error, the
// caller must reject the request rather than silently skip the check.
func EnsureSetupToken(dataDir string, logger *zap.Logger) (string, error) {
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

// licenseConfigured reports whether a real license key is active — anything other than
// the setup/community placeholder sentinels. Shared by getSetupStatus (to inform the
// frontend) and createInitialAdmin (to actually enforce it, v3/36).
func (h *Handler) licenseConfigured() bool {
	currentKey := h.license.GetKey()
	return currentKey != "" &&
		currentKey != license.CommunityModeKey &&
		currentKey != license.SetupModeKey
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

	licenseConfigured := h.licenseConfigured()

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

	resp, badKey, err := h.persistLicenseKey(c.Request.Context(), req.Key)
	if err != nil {
		status := http.StatusInternalServerError
		if badKey {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"valid":      true,
		"tier":       resp.Tier,
		"max_agents": resp.MaxAgents,
		"features":   resp.Features,
	})
}

// persistLicenseKey validates key against infrapilot.org and, on success, saves it into
// system_settings — the single write path both the manual "paste a key" step
// (setupLicense) and the automatic community-signup poll (communitySignupStatus) go
// through, so a key lands in the same place and gets the same validation either way.
// badKey distinguishes "the key itself is bad" (400-worthy) from an infra failure (500).
func (h *Handler) persistLicenseKey(ctx context.Context, key string) (resp *license.ValidationResponse, badKey bool, err error) {
	resp, err = h.license.UpdateKey(key)
	if err != nil {
		return nil, true, fmt.Errorf("license validation failed: %w", err)
	}
	if !resp.Valid {
		errMsg := "invalid license key"
		if resp.Error != "" {
			errMsg = resp.Error
		}
		return nil, true, errors.New(errMsg)
	}

	// Ensure default org exists first (same pattern as createInitialAdmin)
	orgID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	if _, err := h.db.Exec(ctx, `
		INSERT INTO organizations (id, name, slug)
		VALUES ($1, 'Default Organization', 'default')
		ON CONFLICT (id) DO NOTHING
	`, orgID); err != nil {
		return nil, false, fmt.Errorf("failed to create organization: %w", err)
	}

	settingValue, err := json.Marshal(map[string]interface{}{
		"key":        key,
		"tier":       resp.Tier,
		"max_agents": resp.MaxAgents,
	})
	if err != nil {
		return nil, false, fmt.Errorf("failed to encode license data: %w", err)
	}
	if _, err := h.db.Exec(ctx, `
		INSERT INTO system_settings (org_id, setting_key, setting_value)
		VALUES ($1, 'license_key', $2)
		ON CONFLICT (org_id, setting_key) DO UPDATE SET setting_value = EXCLUDED.setting_value
	`, orgID, string(settingValue)); err != nil {
		return nil, false, fmt.Errorf("failed to save license key: %w", err)
	}

	return resp, false, nil
}

// CommunitySignupRequest is the body for POST /setup/community-signup.
type CommunitySignupRequest struct {
	Email string `json:"email" binding:"required,email"`
}

// communitySignup kicks off the in-app "get a free key" path: asks infrapilot.org to
// create/link an account for this email and send a verification link, so a license (now
// required to complete setup, v3/36) can be obtained without leaving this page. The
// frontend polls communitySignupStatus afterward to find out when it's issued.
func (h *Handler) communitySignup(c *gin.Context) {
	var req CommunitySignupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	instanceID, err := license.GetInstanceID(h.cfg.DataDir)
	if err != nil {
		h.logger.Error("Failed to get instance ID for community signup", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to prepare signup request"})
		return
	}

	if err := license.RequestCommunitySignup(req.Email, instanceID, h.version); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "pending"})
}

// communitySignupStatus polls infrapilot.org for the outcome of a prior communitySignup
// call. Once verified, persists the issued key the same way a manually pasted key would
// (persistLicenseKey) so the frontend seeing "verified" means setup can proceed
// immediately, no separate paste step.
func (h *Handler) communitySignupStatus(c *gin.Context) {
	email := c.Query("email")
	if email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "email query parameter is required"})
		return
	}

	instanceID, err := license.GetInstanceID(h.cfg.DataDir)
	if err != nil {
		h.logger.Error("Failed to get instance ID for community signup status", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check signup status"})
		return
	}

	status, err := license.CheckCommunitySignupStatus(email, instanceID, h.version)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	if status.Status != "verified" || status.Key == "" {
		c.JSON(http.StatusOK, gin.H{"status": status.Status})
		return
	}

	if _, _, err := h.persistLicenseKey(c.Request.Context(), status.Key); err != nil {
		h.logger.Error("Failed to persist auto-issued community license", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "license was issued but could not be saved — try again"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":     "verified",
		"tier":       status.Tier,
		"max_agents": status.MaxAgents,
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

	expectedToken, err := EnsureSetupToken(h.cfg.DataDir, h.logger)
	if err != nil {
		h.logger.Error("Failed to verify setup token", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to verify setup token"})
		return
	}
	if req.SetupToken != expectedToken {
		c.JSON(http.StatusForbidden, gin.H{"error": "invalid setup token — run `docker exec <container> cat /data/setup_token` to find it (docker logs won't show it, the backend's own log output goes to a file, not the container's stdout)"})
		return
	}

	// A license (free Community or paid) is now required to complete setup (v3/36
	// reversal — the free key had no functional teeth and wasn't driving signups, so
	// requiring it here is the lever, not a feature gate). Activate one via setupLicense
	// or communitySignupStatus before this succeeds.
	if !h.licenseConfigured() {
		c.JSON(http.StatusForbidden, gin.H{"error": "a Community or Enterprise license is required to complete setup — activate one first"})
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
