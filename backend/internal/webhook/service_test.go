package webhook

import (
	"crypto/rand"
	"encoding/hex"
	"testing"

	"go.uber.org/zap"

	"github.com/infrapilot/backend/internal/crypto"
)

// The /api/v1/webhooks/:id/receive endpoint is deliberately public (real CI providers
// can't attach a session token) -- signature verification is its ONLY authentication
// boundary. requireVerifiableSecret is what VerifyAndParse calls before ever parsing or
// acting on a payload; these tests lock in that it's fail-closed, not fail-open.

func TestRequireVerifiableSecretFailsWhenEncryptionUnconfigured(t *testing.T) {
	s := &Service{logger: zap.NewNop(), encryptionSvc: nil}
	if err := s.requireVerifiableSecret([]byte("some-ciphertext")); err == nil {
		t.Fatal("expected an error when encryptionSvc is nil, got nil (this used to silently skip verification)")
	}
}

func TestRequireVerifiableSecretFailsWhenNoEncryptedSecretStored(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand: %v", err)
	}
	svc, err := crypto.NewEncryptionService(hex.EncodeToString(key))
	if err != nil {
		t.Fatalf("NewEncryptionService: %v", err)
	}
	s := &Service{logger: zap.NewNop(), encryptionSvc: svc}
	if err := s.requireVerifiableSecret(nil); err == nil {
		t.Fatal("expected an error for an empty encrypted secret, got nil (this used to silently skip verification)")
	}
}

func TestRequireVerifiableSecretPassesWithBothPresent(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand: %v", err)
	}
	svc, err := crypto.NewEncryptionService(hex.EncodeToString(key))
	if err != nil {
		t.Fatalf("NewEncryptionService: %v", err)
	}
	s := &Service{logger: zap.NewNop(), encryptionSvc: svc}
	encrypted, err := svc.Encrypt([]byte("webhook-secret"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if err := s.requireVerifiableSecret(encrypted); err != nil {
		t.Fatalf("expected no error with a real encrypted secret and a configured service, got %v", err)
	}
}
