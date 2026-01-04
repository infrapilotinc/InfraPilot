package enrollment

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
)

const (
	credentialsFile = "credentials.json"
	agentVersion    = "0.1.0"
)

// Credentials holds the agent's enrollment credentials
type Credentials struct {
	AgentID     string `json:"agent_id"`
	OrgID       string `json:"org_id"`
	Fingerprint string `json:"fingerprint"`
	Hostname    string `json:"hostname"`
	EnrolledAt  string `json:"enrolled_at"`
}

// EnrollmentRequest is sent to the backend enrollment endpoint
type EnrollmentRequest struct {
	EnrollmentToken string            `json:"enrollment_token"`
	Hostname        string            `json:"hostname"`
	Version         string            `json:"version,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
}

// EnrollmentResponse is received from the backend enrollment endpoint
type EnrollmentResponse struct {
	AgentID     string `json:"agent_id"`
	AgentName   string `json:"agent_name"`
	OrgID       string `json:"org_id"`
	Fingerprint string `json:"fingerprint"`
	Endpoint    string `json:"endpoint,omitempty"`
}

// HeartbeatRequest is sent to the backend heartbeat endpoint
type HeartbeatRequest struct {
	Fingerprint string `json:"fingerprint"`
	Version     string `json:"version,omitempty"`
}

// Manager handles agent enrollment and credential management
type Manager struct {
	backendURL      string
	enrollmentToken string
	dataDir         string
	httpClient      *http.Client
	logger          *zap.Logger
	credentials     *Credentials
}

// NewManager creates a new enrollment manager
func NewManager(backendURL, enrollmentToken, dataDir string, logger *zap.Logger) *Manager {
	return &Manager{
		backendURL:      backendURL,
		enrollmentToken: enrollmentToken,
		dataDir:         dataDir,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
	}
}

// GetCredentials returns the current credentials (nil if not enrolled)
func (m *Manager) GetCredentials() *Credentials {
	return m.credentials
}

// GetAgentID returns the agent ID if enrolled, empty string otherwise
func (m *Manager) GetAgentID() string {
	if m.credentials != nil {
		return m.credentials.AgentID
	}
	return ""
}

// GetFingerprint returns the fingerprint if enrolled, empty string otherwise
func (m *Manager) GetFingerprint() string {
	if m.credentials != nil {
		return m.credentials.Fingerprint
	}
	return ""
}

// IsEnrolled returns true if the agent has valid credentials
func (m *Manager) IsEnrolled() bool {
	return m.credentials != nil && m.credentials.AgentID != "" && m.credentials.Fingerprint != ""
}

// LoadOrEnroll loads existing credentials or enrolls with the backend
func (m *Manager) LoadOrEnroll(ctx context.Context) error {
	// Try to load existing credentials
	if err := m.loadCredentials(); err == nil {
		m.logger.Info("Loaded existing enrollment credentials",
			zap.String("agent_id", m.credentials.AgentID),
			zap.String("org_id", m.credentials.OrgID),
		)

		// Verify credentials are still valid with backend
		if err := m.verifyEnrollment(ctx); err != nil {
			m.logger.Warn("Existing credentials invalid, will re-enroll", zap.Error(err))
		} else {
			return nil
		}
	}

	// No valid credentials, need to enroll
	if m.enrollmentToken == "" {
		return fmt.Errorf("no enrollment token provided - set ENROLLMENT_TOKEN env var")
	}

	m.logger.Info("Enrolling agent with backend")
	return m.enroll(ctx)
}

// loadCredentials loads credentials from the local file
func (m *Manager) loadCredentials() error {
	credPath := filepath.Join(m.dataDir, credentialsFile)

	data, err := os.ReadFile(credPath)
	if err != nil {
		return fmt.Errorf("failed to read credentials file: %w", err)
	}

	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return fmt.Errorf("failed to parse credentials: %w", err)
	}

	if creds.AgentID == "" || creds.Fingerprint == "" {
		return fmt.Errorf("invalid credentials: missing agent_id or fingerprint")
	}

	m.credentials = &creds
	return nil
}

// saveCredentials saves credentials to the local file
func (m *Manager) saveCredentials() error {
	// Ensure data directory exists
	if err := os.MkdirAll(m.dataDir, 0700); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	credPath := filepath.Join(m.dataDir, credentialsFile)

	data, err := json.MarshalIndent(m.credentials, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}

	if err := os.WriteFile(credPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write credentials file: %w", err)
	}

	m.logger.Info("Saved enrollment credentials", zap.String("path", credPath))
	return nil
}

// enroll registers the agent with the backend
func (m *Manager) enroll(ctx context.Context) error {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	req := EnrollmentRequest{
		EnrollmentToken: m.enrollmentToken,
		Hostname:        hostname,
		Version:         agentVersion,
		Labels:          map[string]string{},
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal enrollment request: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/agents/enroll", m.backendURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := m.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("enrollment request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		var errResp struct {
			Error string `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&errResp)
		return fmt.Errorf("enrollment failed (status %d): %s", resp.StatusCode, errResp.Error)
	}

	var enrollResp EnrollmentResponse
	if err := json.NewDecoder(resp.Body).Decode(&enrollResp); err != nil {
		return fmt.Errorf("failed to parse enrollment response: %w", err)
	}

	m.credentials = &Credentials{
		AgentID:     enrollResp.AgentID,
		OrgID:       enrollResp.OrgID,
		Fingerprint: enrollResp.Fingerprint,
		Hostname:    hostname,
		EnrolledAt:  time.Now().UTC().Format(time.RFC3339),
	}

	if err := m.saveCredentials(); err != nil {
		return fmt.Errorf("failed to save credentials: %w", err)
	}

	m.logger.Info("Agent enrolled successfully",
		zap.String("agent_id", m.credentials.AgentID),
		zap.String("org_id", m.credentials.OrgID),
		zap.String("hostname", hostname),
	)

	return nil
}

// verifyEnrollment checks if the current credentials are still valid
func (m *Manager) verifyEnrollment(ctx context.Context) error {
	url := fmt.Sprintf("%s/api/v1/agents/enroll/status?fingerprint=%s", m.backendURL, m.credentials.Fingerprint)
	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := m.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("verification request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("agent not found - credentials may be invalid")
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("verification failed with status %d", resp.StatusCode)
	}

	var statusResp struct {
		Enrolled bool   `json:"enrolled"`
		AgentID  string `json:"agent_id"`
		Status   string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&statusResp); err != nil {
		return fmt.Errorf("failed to parse status response: %w", err)
	}

	if !statusResp.Enrolled {
		return fmt.Errorf("agent is not enrolled")
	}

	m.logger.Debug("Enrollment verified", zap.String("status", statusResp.Status))
	return nil
}

// SendHeartbeat sends a heartbeat to the backend
func (m *Manager) SendHeartbeat(ctx context.Context) error {
	if !m.IsEnrolled() {
		return fmt.Errorf("agent not enrolled")
	}

	req := HeartbeatRequest{
		Fingerprint: m.credentials.Fingerprint,
		Version:     agentVersion,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal heartbeat: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/agents/heartbeat", m.backendURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := m.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("heartbeat request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// Agent not found - may need to re-enroll
		m.credentials = nil
		return fmt.Errorf("agent not found - re-enrollment required")
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("heartbeat failed with status %d", resp.StatusCode)
	}

	m.logger.Debug("Heartbeat sent successfully")
	return nil
}

// StartHeartbeatLoop starts a background heartbeat loop
func (m *Manager) StartHeartbeatLoop(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				m.logger.Info("Heartbeat loop stopped")
				return
			case <-ticker.C:
				if err := m.SendHeartbeat(ctx); err != nil {
					m.logger.Error("Heartbeat failed", zap.Error(err))

					// If re-enrollment required, try to re-enroll
					if m.credentials == nil && m.enrollmentToken != "" {
						m.logger.Info("Attempting re-enrollment")
						if err := m.enroll(ctx); err != nil {
							m.logger.Error("Re-enrollment failed", zap.Error(err))
						}
					}
				}
			}
		}
	}()

	m.logger.Info("Heartbeat loop started", zap.Duration("interval", interval))
}
