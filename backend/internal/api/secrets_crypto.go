package api

import (
	"encoding/json"
	"regexp"
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
	enc.EnvVars = h.encryptEnvVarValues(cfg.EnvVars)
	return json.Marshal(&enc)
}

// secretKeyHints — env-var name fragments that conventionally hold a secret. Same convention
// infrapilot-ee uses for its vault auto-promotion (v3/33 CB6): CE and EE flag the same
// variables, they just do different things once flagged (CE encrypts the value in place and
// still delivers it as a normal env var; EE promotes it into a named vault entry). Keeping
// the heuristic identical means a compose file behaves consistently across editions.
var secretKeyHints = []string{
	"KEY", "TOKEN", "SECRET", "PASSWORD", "PASSWD", "PASS", "PWD", "CREDENTIAL",
	"PRIVATE", "AUTH", "APIKEY", "ACCESS", "CERT", "SALT", "SIGNING", "WEBHOOK",
}

// looksLikeSecretKey flags an env-var name that conventionally holds a secret.
func looksLikeSecretKey(key string) bool {
	u := strings.ToUpper(key)
	for _, hint := range secretKeyHints {
		if strings.Contains(u, hint) {
			return true
		}
	}
	return false
}

// credentialURLPattern matches a connection string with embedded credentials, e.g.
// postgresql://user:pass@host:5432/db, mysql://user:pass@host/db, redis://:pass@host:6379.
// Requires a non-empty password between ":" and "@" so a bare "scheme://user@host" (no
// credential) doesn't match.
var credentialURLPattern = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9+.-]*://[^\s:@/]*:[^\s@/]+@`)

// looksLikeSecretValue flags a value that embeds credentials directly, independent of the
// env-var's name. DATABASE_URL, REDIS_URL, and similar connection-string variables carry a
// live password in the value itself but don't match any name hint in secretKeyHints, so the
// name-only heuristic above misses them entirely — this catches that class of leak.
func looksLikeSecretValue(v string) bool {
	return credentialURLPattern.MatchString(v)
}

// Stack-deployed (and single-deploy "Env Vars" method) env vars had NO encryption-at-rest at
// all — container_config.EnvVars was always plaintext, unlike the opt-in Docker Secrets
// method's container_config.Secrets[]. This closes that gap without introducing a vault: any
// value whose KEY looks like a secret, OR whose VALUE embeds credentials directly (e.g. a
// DATABASE_URL/REDIS_URL connection string — see looksLikeSecretValue), gets the same enc:v1:
// treatment, still delivered as a normal environment variable (matching what a compose service
// actually expects to read), just never stored in plaintext in between. No naming/reference/
// browse layer — that's the deliberately bigger EE feature (doc 21's native vault + external
// provider integrations).

// encryptEnvVarValues returns a COPY of envVars with each secret-looking value encrypted at
// rest. The input map is never mutated. Idempotent (an already-marked value is left as-is)
// and fail-open (an encryption error keeps the plaintext rather than losing the value).
func (h *Handler) encryptEnvVarValues(envVars map[string]string) map[string]string {
	if len(envVars) == 0 || h.encryptionSvc == nil {
		return envVars
	}
	out := make(map[string]string, len(envVars))
	for k, v := range envVars {
		if v == "" || strings.HasPrefix(v, secretEncPrefix) || (!looksLikeSecretKey(k) && !looksLikeSecretValue(v)) {
			out[k] = v
			continue
		}
		ct, err := h.encryptionSvc.EncryptString(v)
		if err != nil {
			h.logger.Error("failed to encrypt env var at rest", zap.String("key", k), zap.Error(err))
			out[k] = v // fail-open: keep the (plaintext) value rather than lose it
			continue
		}
		out[k] = secretEncPrefix + ct
	}
	return out
}

// decryptEnvVarValues decrypts marked values IN PLACE. Call it on a freshly-unmarshaled
// config right before env vars are dispatched to the agent — the same single funnel
// decryptSecretValues already uses for container_config.Secrets[]. Legacy/plaintext values
// (no marker) pass through untouched.
func (h *Handler) decryptEnvVarValues(envVars map[string]string) {
	if h.encryptionSvc == nil {
		return
	}
	for k, v := range envVars {
		if !strings.HasPrefix(v, secretEncPrefix) {
			continue
		}
		pt, err := h.encryptionSvc.DecryptString(strings.TrimPrefix(v, secretEncPrefix))
		if err != nil {
			h.logger.Error("failed to decrypt env var", zap.String("key", k), zap.Error(err))
			continue
		}
		envVars[k] = pt
	}
}
