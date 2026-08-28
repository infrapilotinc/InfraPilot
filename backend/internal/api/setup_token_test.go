package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/infrapilot/backend/internal/config"
)

func TestEnsureSetupTokenGeneratesAndPersists(t *testing.T) {
	dir := t.TempDir()
	logger := zap.NewNop()

	token, err := EnsureSetupToken(dir, logger)
	if err != nil {
		t.Fatalf("EnsureSetupToken: %v", err)
	}
	if len(token) == 0 {
		t.Fatal("expected a non-empty token")
	}

	b, err := os.ReadFile(filepath.Join(dir, setupTokenFile))
	if err != nil {
		t.Fatalf("token file not written: %v", err)
	}
	if string(b) != token {
		t.Fatalf("file contents %q != returned token %q", b, token)
	}
}

func TestEnsureSetupTokenIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	logger := zap.NewNop()

	first, err := EnsureSetupToken(dir, logger)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	second, err := EnsureSetupToken(dir, logger)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if first != second {
		t.Fatalf("token changed across calls: %q != %q", first, second)
	}
}

func TestEnsureSetupTokenUniquePerInstall(t *testing.T) {
	logger := zap.NewNop()

	a, err := EnsureSetupToken(t.TempDir(), logger)
	if err != nil {
		t.Fatalf("a: %v", err)
	}
	b, err := EnsureSetupToken(t.TempDir(), logger)
	if err != nil {
		t.Fatalf("b: %v", err)
	}
	if a == b {
		t.Fatal("two different data dirs produced the same token")
	}
}

func TestClearSetupTokenRemovesFile(t *testing.T) {
	dir := t.TempDir()
	logger := zap.NewNop()

	if _, err := EnsureSetupToken(dir, logger); err != nil {
		t.Fatalf("EnsureSetupToken: %v", err)
	}
	path := filepath.Join(dir, setupTokenFile)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected token file to exist before clear: %v", err)
	}

	clearSetupToken(dir)

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected token file to be removed, stat err = %v", err)
	}
}

func TestClearSetupTokenOnMissingFileIsNoOp(t *testing.T) {
	// Must not panic or error when there's nothing to remove (e.g. setup already
	// completed on a previous boot and the token was already cleared).
	clearSetupToken(t.TempDir())
}

// checkSetupToken is what setupLicense, communitySignup, communitySignupVerify,
// verifySetupToken, and createInitialAdmin all share to require the token upfront --
// re-added to the license/signup steps so the whole wizard is gated, not just the final
// admin-creation step.
func TestCheckSetupTokenAcceptsCorrectToken(t *testing.T) {
	dir := t.TempDir()
	logger := zap.NewNop()
	token, err := EnsureSetupToken(dir, logger)
	if err != nil {
		t.Fatalf("EnsureSetupToken: %v", err)
	}

	h := &Handler{cfg: &config.Config{DataDir: dir}, logger: logger}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	if !h.checkSetupToken(c, token) {
		t.Fatalf("expected correct token to pass, got response %d: %s", w.Code, w.Body.String())
	}
}

func TestCheckSetupTokenRejectsWrongToken(t *testing.T) {
	dir := t.TempDir()
	logger := zap.NewNop()
	if _, err := EnsureSetupToken(dir, logger); err != nil {
		t.Fatalf("EnsureSetupToken: %v", err)
	}

	h := &Handler{cfg: &config.Config{DataDir: dir}, logger: logger}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	if h.checkSetupToken(c, "wrong-token") {
		t.Fatal("expected a wrong token to be rejected")
	}
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}
