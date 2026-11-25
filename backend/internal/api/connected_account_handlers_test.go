package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/infrapilot/backend/internal/license"
)

func TestConnectedFeatureForChannel(t *testing.T) {
	cases := map[string]string{
		"smtp":    license.FeatureEmailAlerts,
		"slack":   license.FeatureChatNotifications,
		"discord": license.FeatureChatNotifications,
		"teams":   license.FeatureChatNotifications,
		"webhook": license.FeatureWebhooks,
		"bogus":   "",
	}
	for channelType, want := range cases {
		if got := connectedFeatureForChannel(channelType); got != want {
			t.Errorf("connectedFeatureForChannel(%q) = %q, want %q", channelType, got, want)
		}
	}
}

func runConnectedGate(h *Handler, feature string) (bool, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	return h.requireConnectedFeature(c, feature), w
}

func TestKeylessCERefusesConnectedFeatures(t *testing.T) {
	h := &Handler{license: license.NewCommunityModeClient(zap.NewNop())}
	for _, feature := range license.ConnectedFreeFeatures() {
		ok, w := runConnectedGate(h, feature)
		if ok {
			t.Fatalf("keyless CE should not have %q", feature)
		}
		if w.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", w.Code)
		}
		var body map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body["code"] != connectedAccountRequiredCode {
			t.Fatalf("code = %q, want %q so the UI can show the sign-up", body["code"], connectedAccountRequiredCode)
		}
	}
}

func TestKeylessCEKeepsDeployWebhooks(t *testing.T) {
	// Push to deploy from CI is part of the free core and must never sit behind the
	// connected-account gate.
	h := &Handler{license: license.NewCommunityModeClient(zap.NewNop())}
	if !h.license.HasFeature(license.FeatureCDDeployments) {
		t.Fatal("keyless CE must keep cd_deployments (deploy webhooks)")
	}
}

func TestKeyedInstallPassesConnectedGate(t *testing.T) {
	h := &Handler{license: license.NewOfflineClient(zap.NewNop())}
	for _, feature := range license.ConnectedFreeFeatures() {
		if ok, _ := runConnectedGate(h, feature); !ok {
			t.Fatalf("a keyed install should pass the gate for %q", feature)
		}
	}
}

func TestEmptyFeaturePassesGate(t *testing.T) {
	h := &Handler{license: license.NewCommunityModeClient(zap.NewNop())}
	if ok, _ := runConnectedGate(h, ""); !ok {
		t.Fatal("no feature needed should pass")
	}
}
