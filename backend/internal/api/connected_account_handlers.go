package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/infrapilot/backend/internal/license"
)

// Keyless CE (v3/36, 2026-11-25): setup no longer asks for a license. The free
// Community key is asked for when the user adds an alert channel instead, because
// that is the first moment it gives them something (email, chat and outgoing
// webhook notifications are the "connected" bundle, v3/35).
//
// Deploy webhooks (CI -> push to deploy) are NOT part of this gate. They belong to
// cd_deployments, which every install has.

// connectedAccountRequiredCode lets the frontend tell this refusal apart from other
// errors and show the in-app email sign-up instead of a plain error message.
const connectedAccountRequiredCode = "connected_account_required"

// connectedFeatureForChannel maps an alert channel type to the connected feature it
// needs. An unknown type returns "" (the caller validates types separately).
func connectedFeatureForChannel(channelType string) string {
	switch channelType {
	case "smtp":
		return license.FeatureEmailAlerts
	case "slack", "discord", "teams":
		return license.FeatureChatNotifications
	case "webhook":
		return license.FeatureWebhooks
	}
	return ""
}

// requireConnectedFeature writes a 403 carrying connectedAccountRequiredCode and
// returns false when this install has no key that unlocks feature.
func (h *Handler) requireConnectedFeature(c *gin.Context, feature string) bool {
	if feature == "" || h.license.HasFeature(feature) {
		return true
	}
	c.JSON(http.StatusForbidden, gin.H{
		"error": "Connect a free InfraPilot account to send alerts. It only needs an email address.",
		"code":  connectedAccountRequiredCode,
	})
	return false
}

// ConnectAccountRequest is the body for POST /settings/license/connect.
type ConnectAccountRequest struct {
	Email string `json:"email" binding:"required,email"`
}

// ConnectAccountVerifyRequest is the body for POST /settings/license/connect/verify.
type ConnectAccountVerifyRequest struct {
	Email string `json:"email" binding:"required,email"`
	Code  string `json:"code" binding:"required"`
}

// licenseManagedByEnv rejects UI license changes when LICENSE_KEY is set, the same
// rule updateLicenseKey applies.
func (h *Handler) licenseManagedByEnv(c *gin.Context) bool {
	if h.cfg.LicenseKey == "" {
		return false
	}
	c.JSON(http.StatusBadRequest, gin.H{
		"error": "License key is set via the LICENSE_KEY environment variable and cannot be changed from the UI. Remove the env var to manage the license here.",
	})
	return true
}

// alreadyConnected refuses a connect when a real key is active, so the free key can
// never replace a paid one.
func (h *Handler) alreadyConnected(c *gin.Context) bool {
	if !h.licenseConfigured() {
		return false
	}
	c.JSON(http.StatusConflict, gin.H{"error": "This install already has a license key. Manage it in Settings → License."})
	return true
}

// connectAccount is the post-setup twin of communitySignup: an already logged-in
// super_admin asks for a 6-digit code to get a free Community key.
func (h *Handler) connectAccount(c *gin.Context) {
	if h.licenseManagedByEnv(c) || h.alreadyConnected(c) {
		return
	}
	var req ConnectAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.sendCommunitySignupCode(c, req.Email)
}

// connectAccountVerify is the post-setup twin of communitySignupVerify. On success
// the free key is live in memory and saved, so the next alert channel create passes.
func (h *Handler) connectAccountVerify(c *gin.Context) {
	if h.licenseManagedByEnv(c) || h.alreadyConnected(c) {
		return
	}
	var req ConnectAccountVerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.verifyCommunitySignupCode(c, req.Email, req.Code)
}
