package api

import (
	"testing"

	"go.uber.org/zap"

	"github.com/infrapilot/backend/internal/license"
)

func TestLicenseConfiguredFalseForCommunityMode(t *testing.T) {
	h := &Handler{license: license.NewCommunityModeClient(zap.NewNop())}
	if h.licenseConfigured() {
		t.Fatal("community-mode sentinel key should not count as configured — this is exactly the case createInitialAdmin must reject")
	}
}

func TestLicenseConfiguredFalseForSetupMode(t *testing.T) {
	h := &Handler{license: license.NewSetupModeClient(zap.NewNop())}
	if h.licenseConfigured() {
		t.Fatal("setup-mode sentinel key should not count as configured")
	}
}

func TestLicenseConfiguredTrueForRealKey(t *testing.T) {
	// NewOfflineClient sets a real-looking key with no network validation, closest
	// available fixture for "a real key is present" without hitting infrapilot.org.
	h := &Handler{license: license.NewOfflineClient(zap.NewNop())}
	if !h.licenseConfigured() {
		t.Fatal("a real (non-sentinel) key should count as configured")
	}
}
