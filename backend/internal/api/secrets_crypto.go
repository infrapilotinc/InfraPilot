package api

import (
	"encoding/json"
	"strings"

	"go.uber.org/zap"
)

// Deployment secret values (ContainerConfig.Secrets[].Value) were historically stored as
// PLAINTEXT in the deployments/service_configs.container_config JSONB — anyone with DB or
// backup access could read them. This file encrypts those values at rest, mirroring how
// webhook/registry secrets are already encrypted.
//
// Design (deliberately safe for a retrofit that can't be fully runtime-tested here):
//   - Prefix marker `enc:v1:` marks an encrypted value. Decrypt only touches marked values;
//     legacy plaintext rows pass through unchanged → backward compatible.
//   - Idempotent: encrypting an already-marked value is a no-op → safe if a path double-writes.
//   - Nil-safe: with no ENCRYPTION_KEY (encryptionSvc == nil) both helpers are no-ops, so
//     behavior is exactly as today (plaintext) — no hard failure.
//   - Fail-open with a logged error on a crypto error, so a deploy is never silently dropped.
//   - A MISSED decrypt site sends `enc:v1:…` to the agent → the deploy fails LOUDLY (caught in
//     smoke tests) rather than leaking plaintext — the safe failure mode for this retrofit.
const secretEncPrefix = "enc:v1:"

// encryptSecretValues returns a COPY of secrets with each Value encrypted. The input slice is
// never mutated, so a caller can keep using the plaintext secrets (e.g. to build the agent
// dispatch) after handing these to storage.
func (h *Handler) encryptSecretValues(secrets []ContainerConfigSecret) []ContainerConfigSecret {
	if len(secrets) == 0 || h.encryptionSvc == nil {
		return secrets
	}
	out := make([]ContainerConfigSecret, len(secrets))
	for i, s := range secrets {
		out[i] = s
		if s.Value == "" || strings.HasPrefix(s.Value, secretEncPrefix) {
			continue // empty or already encrypted
		}
		ct, err := h.encryptionSvc.EncryptString(s.Value)
		if err != nil {
			h.logger.Error("failed to encrypt deployment secret at rest",
				zap.String("secret", s.Name), zap.Error(err))
			continue // fail-open: keep the (plaintext) value rather than lose it
		}
		out[i].Value = secretEncPrefix + ct
	}
	return out
}

// decryptSecretValues decrypts marked values IN PLACE. Call it on a freshly-unmarshaled config
// right before the secrets are used to dispatch to the agent. Legacy plaintext values (no
// marker) are left untouched.
func (h *Handler) decryptSecretValues(secrets []ContainerConfigSecret) {
	if h.encryptionSvc == nil {
		return
	}
	for i := range secrets {
		v := secrets[i].Value
		if !strings.HasPrefix(v, secretEncPrefix) {
			continue
		}
		pt, err := h.encryptionSvc.DecryptString(strings.TrimPrefix(v, secretEncPrefix))
		if err != nil {
			h.logger.Error("failed to decrypt deployment secret",
				zap.String("secret", secrets[i].Name), zap.Error(err))
			continue
		}
		secrets[i].Value = pt
	}
}

// marshalContainerConfig encrypts secret values (into a copy) then JSON-marshals for storage.
// Use this everywhere a *DeploymentContainerConfig is persisted to container_config, in place
// of a raw json.Marshal, so secrets are never written plaintext.
func (h *Handler) marshalContainerConfig(cfg *DeploymentContainerConfig) ([]byte, error) {
	if cfg == nil {
		return json.Marshal(cfg)
	}
	enc := *cfg
	enc.Secrets = h.encryptSecretValues(cfg.Secrets)
	return json.Marshal(&enc)
}
