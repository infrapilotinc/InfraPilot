package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// getPrivacySettings reports whether anonymous product-funnel telemetry (v3/40) is enabled
// on this instance — distinct from the older per-metric telemetry_settings (getTelemetrySettings)
// in metrics_handlers.go, which governs a separate observability feature.
func (h *Handler) getPrivacySettings(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"telemetry_enabled": !h.telemetry.OptedOut()})
}

type updatePrivacyRequest struct {
	TelemetryEnabled *bool `json:"telemetry_enabled"`
}

// updatePrivacySettings toggles the Settings → Privacy opt-out (v3/40 G1b). The
// INFRAPILOT_TELEMETRY=off env var still disables the client outright regardless of this
// setting — this only controls the separate, persisted user-facing toggle.
func (h *Handler) updatePrivacySettings(c *gin.Context) {
	var req updatePrivacyRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.TelemetryEnabled == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "telemetry_enabled is required"})
		return
	}
	if err := h.telemetry.SetOptOut(!*req.TelemetryEnabled); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update privacy settings"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"telemetry_enabled": *req.TelemetryEnabled})
}
