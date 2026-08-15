package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"testing"

	"go.uber.org/zap"

	"github.com/infrapilot/backend/internal/crypto"
)

func testHandler(t *testing.T) *Handler {
	t.Helper()
	key := make([]byte, 32) // AES-256
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand: %v", err)
	}
	svc, err := crypto.NewEncryptionService(hex.EncodeToString(key))
	if err != nil {
		t.Fatalf("NewEncryptionService: %v", err)
	}
	return &Handler{encryptionSvc: svc, logger: zap.NewNop()}
}

func TestSecretRoundTripThroughStorage(t *testing.T) {
	h := testHandler(t)
	cfg := &DeploymentContainerConfig{
		EnvVars: map[string]string{"PUBLIC": "ok"},
		Secrets: []ContainerConfigSecret{
			{Name: "DB_PASSWORD", Value: "s3cr3t-value", MountPath: "/run/secrets/db"},
			{Name: "API_KEY", Value: "another-secret"},
		},
	}

	// Marshal-for-storage must NOT contain the plaintext.
	stored, err := h.marshalContainerConfig(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if contains(stored, "s3cr3t-value") || contains(stored, "another-secret") {
		t.Fatal("plaintext secret found in stored container_config")
	}
	// The original config is untouched (still plaintext for dispatch).
	if cfg.Secrets[0].Value != "s3cr3t-value" {
		t.Fatal("marshal mutated the input config")
	}

	// Read back + decrypt → original values.
	var back DeploymentContainerConfig
	if err := json.Unmarshal(stored, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	h.decryptSecretValues(back.Secrets)
	if back.Secrets[0].Value != "s3cr3t-value" || back.Secrets[1].Value != "another-secret" {
		t.Fatalf("decrypt mismatch: %+v", back.Secrets)
	}
}

func TestLegacyPlaintextPassesThrough(t *testing.T) {
	h := testHandler(t)
	// Simulate an existing row: unmarked plaintext values.
	secrets := []ContainerConfigSecret{{Name: "X", Value: "legacy-plaintext"}}
	h.decryptSecretValues(secrets)
	if secrets[0].Value != "legacy-plaintext" {
		t.Fatalf("legacy plaintext should pass through, got %q", secrets[0].Value)
	}
}

func TestEncryptIsIdempotent(t *testing.T) {
	h := testHandler(t)
	once := h.encryptSecretValues([]ContainerConfigSecret{{Name: "X", Value: "v"}})
	twice := h.encryptSecretValues(once)
	if once[0].Value != twice[0].Value {
		t.Fatal("double-encrypt changed the value (not idempotent)")
	}
}

func TestNilServiceIsNoOp(t *testing.T) {
	h := &Handler{encryptionSvc: nil, logger: zap.NewNop()}
	in := []ContainerConfigSecret{{Name: "X", Value: "plain"}}
	out := h.encryptSecretValues(in)
	if out[0].Value != "plain" {
		t.Fatal("nil encryptionSvc must be a no-op (plaintext, as today)")
	}
}

func contains(b []byte, s string) bool {
	return len(b) > 0 && len(s) > 0 && (indexOf(string(b), s) >= 0)
}

func indexOf(h, n string) int {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return i
		}
	}
	return -1
}
