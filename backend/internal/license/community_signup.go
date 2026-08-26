package license

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// register-ce lives in apps/cloud, deployed at app.infrapilot.sh -- a different app/
// domain from apps/web (infrapilot.org itself, where /api/license/validate lives).
// Pointing this at infrapilot.org was a bug: that domain never has and never will have
// this route, so every request 404s regardless of what gets deployed to it.
const (
	registerCEURL       = "https://app.infrapilot.sh/api/auth/register-ce"
	registerCEStatusURL = "https://app.infrapilot.sh/api/auth/register-ce/status"
)

// CommunitySignupStatus mirrors the JSON returned by infrapilot.org/api/auth/register-ce/status.
type CommunitySignupStatus struct {
	Status    string `json:"status"` // "pending" | "verified" | "not_found"
	Key       string `json:"key,omitempty"`
	Tier      string `json:"tier,omitempty"`
	MaxAgents int    `json:"max_agents,omitempty"`
}

// RequestCommunitySignup asks infrapilot.org to create (or link) an account for email
// and send a verification email — the in-app entry point for CE's "get a free key"
// setup step. instanceID ties the request to this specific box so a later
// CheckCommunitySignupStatus call can't be satisfied by polling a different box's email.
func RequestCommunitySignup(email, instanceID, version string) error {
	body, err := json.Marshal(map[string]string{"email": email, "instance_id": instanceID})
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, registerCEURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", fmt.Sprintf("infrapilot-backend/%s", version))

	httpClient := &http.Client{Timeout: httpTimeout}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		var errBody struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errBody)
		if errBody.Error != "" {
			return fmt.Errorf("%s", errBody.Error)
		}
		return fmt.Errorf("signup request failed: HTTP %d", resp.StatusCode)
	}
	return nil
}

// CheckCommunitySignupStatus polls infrapilot.org for the outcome of a prior
// RequestCommunitySignup — "pending" until the user clicks the verification email in
// their inbox, then "verified" with the issued key.
func CheckCommunitySignupStatus(email, instanceID, version string) (*CommunitySignupStatus, error) {
	statusURL := fmt.Sprintf("%s?email=%s&instance_id=%s",
		registerCEStatusURL, url.QueryEscape(email), url.QueryEscape(instanceID))

	req, err := http.NewRequest(http.MethodGet, statusURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", fmt.Sprintf("infrapilot-backend/%s", version))

	httpClient := &http.Client{Timeout: httpTimeout}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result CommunitySignupStatus
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode signup status response: %w", err)
	}
	return &result, nil
}
