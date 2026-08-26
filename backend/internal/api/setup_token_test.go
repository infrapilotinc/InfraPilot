package api

import (
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
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
