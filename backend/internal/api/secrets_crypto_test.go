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

// Stack-deployed (and single-deploy "Env Vars" method) env vars — previously always
// plaintext at rest, unlike the opt-in Docker Secrets method. Same round-trip contract as
// container_config.Secrets[], applied to EnvVars via name heuristic instead of an explicit
// per-field opt-in.

func TestEnvVarSecretRoundTripThroughStorage(t *testing.T) {
	h := testHandler(t)
	cfg := &DeploymentContainerConfig{
		EnvVars: map[string]string{
			"NODE_ENV":          "production", // not secret-looking — must stay plaintext
			"DATABASE_PASSWORD": "s3cr3t-db-pass",
			"JWT_SECRET":        "s3cr3t-jwt",
		},
	}

	stored, err := h.marshalContainerConfig(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if contains(stored, "s3cr3t-db-pass") || contains(stored, "s3cr3t-jwt") {
		t.Fatal("plaintext env-var secret found in stored container_config")
	}
	if !contains(stored, "production") {
		t.Fatal("non-secret env var should still be stored in plaintext")
	}
	// The original config is untouched (still plaintext for dispatch).
	if cfg.EnvVars["DATABASE_PASSWORD"] != "s3cr3t-db-pass" {
		t.Fatal("marshal mutated the input config")
	}

	var back DeploymentContainerConfig
	if err := json.Unmarshal(stored, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	h.decryptEnvVarValues(back.EnvVars)
	if back.EnvVars["DATABASE_PASSWORD"] != "s3cr3t-db-pass" || back.EnvVars["JWT_SECRET"] != "s3cr3t-jwt" {
		t.Fatalf("decrypt mismatch: %+v", back.EnvVars)
	}
	if back.EnvVars["NODE_ENV"] != "production" {
		t.Fatalf("non-secret value corrupted: %+v", back.EnvVars)
	}
}

func TestEnvVarLegacyPlaintextPassesThrough(t *testing.T) {
	h := testHandler(t)
	envVars := map[string]string{"DB_PASSWORD": "legacy-plaintext"}
	h.decryptEnvVarValues(envVars)
	if envVars["DB_PASSWORD"] != "legacy-plaintext" {
		t.Fatalf("legacy plaintext should pass through, got %q", envVars["DB_PASSWORD"])
	}
}

func TestEnvVarEncryptIsIdempotent(t *testing.T) {
	h := testHandler(t)
	once := h.encryptEnvVarValues(map[string]string{"API_KEY": "v"})
	twice := h.encryptEnvVarValues(once)
	if once["API_KEY"] != twice["API_KEY"] {
		t.Fatal("double-encrypt changed the value (not idempotent)")
	}
}

func TestEnvVarNilServiceIsNoOp(t *testing.T) {
	h := &Handler{encryptionSvc: nil, logger: zap.NewNop()}
	out := h.encryptEnvVarValues(map[string]string{"API_KEY": "plain"})
	if out["API_KEY"] != "plain" {
		t.Fatal("nil encryptionSvc must be a no-op (plaintext, as today)")
	}
}

func TestEnvVarConnectionStringEncryptedRegardlessOfKeyName(t *testing.T) {
	h := testHandler(t)
	cfg := &DeploymentContainerConfig{
		EnvVars: map[string]string{
			"DATABASE_URL": "postgresql://signalforge:s3cr3t-db-pass@postgres:5432/signalforge",
			"REDIS_URL":    "redis://redis:6379", // no embedded credential — stays plaintext
			"NODE_ENV":     "production",
		},
	}

	stored, err := h.marshalContainerConfig(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if contains(stored, "s3cr3t-db-pass") {
		t.Fatal("plaintext credential embedded in connection string found in stored container_config")
	}
	if !contains(stored, "redis://redis:6379") {
		t.Fatal("connection string with no embedded credential should stay plaintext")
	}

	var back DeploymentContainerConfig
	if err := json.Unmarshal(stored, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	h.decryptEnvVarValues(back.EnvVars)
	if back.EnvVars["DATABASE_URL"] != "postgresql://signalforge:s3cr3t-db-pass@postgres:5432/signalforge" {
		t.Fatalf("decrypt mismatch: %+v", back.EnvVars)
	}
}

func TestLooksLikeSecretValue(t *testing.T) {
	cases := map[string]bool{
		"postgresql://user:pass@host:5432/db": true,
		"mysql://root:hunter2@db/app":          true,
		"redis://:onlypass@redis:6379":         true,
		"redis://redis:6379":                   false, // no credential
		"https://example.com/path":             false,
		"https://user@example.com":             false, // userinfo with no password
		"just a plain value":                   false,
	}
	for value, want := range cases {
		if got := looksLikeSecretValue(value); got != want {
			t.Errorf("looksLikeSecretValue(%q) = %v, want %v", value, got, want)
		}
	}
}

func TestLooksLikeSecretKey(t *testing.T) {
	cases := map[string]bool{
		"DATABASE_PASSWORD": true,
		"JWT_SECRET":         true,
		"API_KEY":            true,
		"AUTH_TOKEN":         true,
		"NODE_ENV":           false,
		"PORT":               false,
		"PUBLIC_URL":         false,
	}
	for key, want := range cases {
		if got := looksLikeSecretKey(key); got != want {
			t.Errorf("looksLikeSecretKey(%q) = %v, want %v", key, got, want)
		}
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
