package license

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

// register-ce lives in apps/cloud, deployed at app.infrapilot.sh -- a different app/
// domain from apps/web (infrapilot.org itself, where /api/license/validate lives).
// Pointing this at infrapilot.org was a bug: that domain never has and never will have
// this route, so every request 404s regardless of what gets deployed to it.
const (
	registerCEURL       = "https://app.infrapilot.sh/api/auth/register-ce"
	registerCEVerifyURL = "https://app.infrapilot.sh/api/auth/register-ce/verify"
)

// CommunitySignupStatus mirrors the JSON returned by infrapilot.org's register-ce/verify
// endpoint. Status values: "verified" | "invalid_code" | "expired" | "too_many_attempts"
// | "not_found".
type CommunitySignupStatus struct {
	Status    string `json:"status"`
	Key       string `json:"key,omitempty"`
	Tier      string `json:"tier,omitempty"`
	MaxAgents int    `json:"max_agents,omitempty"`
}

// RequestCommunitySignup asks infrapilot.org to create (or link) an account for email
// and email a 6-digit verification code — the in-app entry point for CE's "get a free
// key" setup step. instanceID travels with the request but is never written to the
// account until VerifyCommunitySignupOTP succeeds (that's the actual proof of email
// ownership; see the sibling infrapilot.org route for why).
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

// VerifyCommunitySignupOTP submits the 6-digit code the user was emailed by a prior
// RequestCommunitySignup call. On "verified" the license is already issued and ready to
// use — no separate polling step, unlike the older link-click flow.
func VerifyCommunitySignupOTP(email, code, instanceID, version string) (*CommunitySignupStatus, error) {
	body, err := json.Marshal(map[string]string{"email": email, "code": code, "instance_id": instanceID})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, registerCEVerifyURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", fmt.Sprintf("infrapilot-backend/%s", version))

	httpClient := &http.Client{Timeout: httpTimeout}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result CommunitySignupStatus
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode signup verify response: %w", err)
	}
	return &result, nil
}
