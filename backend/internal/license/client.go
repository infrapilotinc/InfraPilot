package license

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

const (
	// CommunityModeKey is the placeholder key used when no real license key is configured.
	// The settings handler uses this to detect that the backend is running in community mode.
	CommunityModeKey = "IP-CE-FREE"

	// SetupModeKey is the placeholder key used when the backend is running in setup mode
	// (e.g. no valid license key has been configured yet).
	SetupModeKey = "IP-CE-SETUP"

	validationURL = "https://infrapilot.org/api/license/validate"
	cacheDuration = 24 * time.Hour
	graceDuration = 48 * time.Hour
	httpTimeout   = 10 * time.Second
	retryInterval = 5 * time.Minute // minimum gap between failed validation attempts
)

// ValidationResponse mirrors the JSON returned by infrapilot.org/api/license/validate.
type ValidationResponse struct {
	Valid      bool     `json:"valid"`
	Tier       string   `json:"tier"`
	MaxAgents  int      `json:"max_agents"`
	Features   []string `json:"features"`
	ExpiresAt  *string  `json:"expires_at"`
	Error      string   `json:"error,omitempty"`
	IssuedTo   *struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"issued_to,omitempty"`
}

// Client validates and caches a license key against infrapilot.org.
type Client struct {
	licenseKey string
	instanceID string
	hostname   string
	version    string
	logger     *zap.Logger

	mu             sync.RWMutex
	cached         *ValidationResponse
	cachedAt       time.Time
	lastValidAt    time.Time
	lastAttempt    time.Time  // timestamp of last fetchFromAPI call (for retry throttling)
	lastAttemptErr error      // error from the last failed attempt
}

// NewClient creates a license client. It loads or creates a stable instance ID
// from dataDir/instance.id, then performs an initial validation.
func NewClient(licenseKey, dataDir, version string, logger *zap.Logger) (*Client, error) {
	hostname, _ := os.Hostname()

	instanceID, err := loadOrCreateInstanceID(dataDir)
	if err != nil {
		return nil, fmt.Errorf("license: failed to get instance ID: %w", err)
	}

	return &Client{
		licenseKey: licenseKey,
		instanceID: instanceID,
		hostname:   hostname,
		version:    version,
		logger:     logger,
	}, nil
}

// Validate returns the current license state, using the 24h cache when fresh.
// On network failure it falls back to the cached response within a 48h grace period.
// Retries are throttled to at most once per 5 minutes to avoid blocking API requests.
func (c *Client) Validate() (*ValidationResponse, error) {
	// Fast path — return cached response if still fresh.
	c.mu.RLock()
	if c.cached != nil && time.Since(c.cachedAt) < cacheDuration {
		resp := c.cached
		c.mu.RUnlock()
		return resp, nil
	}
	// Throttle: if no valid cache and last attempt was recent, return the last error
	// rather than making another blocking HTTP request.
	throttled := c.cached == nil && !c.lastAttempt.IsZero() && time.Since(c.lastAttempt) < retryInterval
	lastErr := c.lastAttemptErr
	c.mu.RUnlock()

	if throttled {
		return nil, lastErr
	}

	// Mark the attempt timestamp before the network call.
	c.mu.Lock()
	c.lastAttempt = time.Now()
	c.mu.Unlock()

	resp, err := c.fetchFromAPI()
	if err != nil {
		c.logger.Warn("License validation network error", zap.Error(err))

		wrappedErr := fmt.Errorf("license validation failed: %w", err)
		c.mu.Lock()
		c.lastAttemptErr = wrappedErr
		c.mu.Unlock()

		// Fall back to stale cache within grace period.
		c.mu.RLock()
		defer c.mu.RUnlock()
		if c.cached != nil && time.Since(c.lastValidAt) < graceDuration {
			c.logger.Warn("Using cached license response during grace period",
				zap.Duration("age", time.Since(c.cachedAt)))
			return c.cached, nil
		}
		return nil, wrappedErr
	}

	c.mu.Lock()
	c.cached = resp
	c.cachedAt = time.Now()
	c.lastAttemptErr = nil
	if resp.Valid {
		c.lastValidAt = time.Now()
	}
	c.mu.Unlock()

	return resp, nil
}

// HasFeature reports whether the current license includes a specific feature.
// Returns false on any validation error.
func (c *Client) HasFeature(feature string) bool {
	resp, err := c.Validate()
	if err != nil || !resp.Valid {
		return false
	}
	for _, f := range resp.Features {
		if f == feature {
			return true
		}
	}
	return false
}

// Tier returns the current license tier ("community", "professional", "enterprise").
func (c *Client) Tier() string {
	resp, err := c.Validate()
	if err != nil || !resp.Valid {
		return "community"
	}
	return resp.Tier
}

// MaxAgents returns the max number of agents this license permits (-1 = unlimited).
func (c *Client) MaxAgents() int {
	resp, err := c.Validate()
	if err != nil || !resp.Valid {
		return 1
	}
	return resp.MaxAgents
}

func (c *Client) fetchFromAPI() (*ValidationResponse, error) {
	payload := map[string]string{
		"key":         c.licenseKey,
		"instance_id": c.instanceID,
		"hostname":    c.hostname,
		"version":     c.version,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, validationURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", fmt.Sprintf("infrapilot-backend/%s", c.version))

	httpClient := &http.Client{Timeout: httpTimeout}
	httpResp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	var result ValidationResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode validation response: %w", err)
	}

	return &result, nil
}

// loadOrCreateInstanceID reads dataDir/instance.id or creates it on first run.
func loadOrCreateInstanceID(dataDir string) (string, error) {
	if err := os.MkdirAll(dataDir, 0750); err != nil {
		return "", err
	}

	idFile := filepath.Join(dataDir, "instance.id")
	if data, err := os.ReadFile(idFile); err == nil {
		id := strings.TrimSpace(string(data))
		if id != "" {
			return id, nil
		}
	}

	// Generate a new stable UUID using crypto/rand via os
	// We use a simple approach compatible with all Go versions
	f, err := os.Open("/dev/urandom")
	if err != nil {
		return "", fmt.Errorf("failed to open /dev/urandom: %w", err)
	}
	defer f.Close()

	b := make([]byte, 16)
	if _, err := f.Read(b); err != nil {
		return "", err
	}
	// Format as UUID v4
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	id := fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])

	if err := os.WriteFile(idFile, []byte(id+"\n"), 0600); err != nil {
		return "", fmt.Errorf("failed to write instance ID: %w", err)
	}

	return id, nil
}

// GetKey returns the current license key (may be empty for offline/setup-mode clients).
func (c *Client) GetKey() string {
	return c.licenseKey
}

// UpdateKey validates a new license key and, if valid, replaces the in-memory state.
// If validation fails the existing key/cache is preserved and the validation response is returned.
func (c *Client) UpdateKey(newKey string) (*ValidationResponse, error) {
	// Build a temporary request to validate the new key against the API.
	tmpClient := &Client{
		licenseKey: newKey,
		instanceID: c.instanceID,
		hostname:   c.hostname,
		version:    c.version,
		logger:     c.logger,
	}
	resp, err := tmpClient.fetchFromAPI()
	if err != nil {
		return nil, fmt.Errorf("license validation request failed: %w", err)
	}
	if !resp.Valid {
		return resp, nil
	}
	// Swap the in-memory state atomically.
	c.mu.Lock()
	c.licenseKey = newKey
	c.cached = resp
	c.cachedAt = time.Now()
	c.lastValidAt = time.Now()
	c.mu.Unlock()
	return resp, nil
}

// NewOfflineClient returns a client with all features enabled — for development only.
// It never calls infrapilot.org. Panics in production.
func NewOfflineClient(logger *zap.Logger) *Client {
	if os.Getenv("ENV") == "production" {
		panic("NewOfflineClient must never be used in production")
	}
	logger.Warn("LICENSE: running in offline mode — all features enabled (development only)")
	c := &Client{
		licenseKey: "IP-CE-OFFLINE",
		instanceID: "offline-dev",
		hostname:   "localhost",
		version:    "dev",
		logger:     logger,
	}
	c.cached = &ValidationResponse{
		Valid:     true,
		Tier:      "enterprise",
		MaxAgents: -1,
		Features:  AllFeatures(),
	}
	c.cachedAt = time.Now()
	c.lastValidAt = time.Now()
	return c
}

// NewCommunityModeClient returns a client with Community Edition limits — used when
// no license key is configured. Community Edition is free forever; a key is only
// needed to unlock Professional or Enterprise features.
func NewCommunityModeClient(logger *zap.Logger) *Client {
	logger.Info("LICENSE: running in community mode — enter a Pro/Enterprise key in Settings → License to unlock advanced features")
	c := &Client{
		licenseKey: "IP-CE-FREE",
		instanceID: "community",
		hostname:   "localhost",
		version:    "community",
		logger:     logger,
	}
	c.cached = &ValidationResponse{
		Valid:     true,
		Tier:      "community",
		MaxAgents: 1,
		Features:  CommunityFeatures(),
	}
	c.cachedAt = time.Now()
	c.lastValidAt = time.Now()
	return c
}

// NewSetupModeClient returns a client with all features temporarily enabled for
// first-run setup mode. Unlike NewOfflineClient it does not panic in production —
// it is intended to be used only until the operator completes the setup wizard
// and the server is restarted with a real license key.
func NewSetupModeClient(logger *zap.Logger) *Client {
	logger.Warn("LICENSE: running in setup mode — all features temporarily enabled until setup is complete")
	c := &Client{
		licenseKey: "IP-CE-SETUP",
		instanceID: "setup",
		hostname:   "localhost",
		version:    "dev",
		logger:     logger,
	}
	c.cached = &ValidationResponse{
		Valid:     true,
		Tier:      "community",
		MaxAgents: -1,
		Features:  AllFeatures(),
	}
	c.cachedAt = time.Now()
	c.lastValidAt = time.Now()
	return c
}
