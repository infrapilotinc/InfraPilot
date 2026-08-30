package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// BearerToken represents a bearer token for a proxy. The token value itself is never
// part of this type -- it only ever exists in the create response (see
// CreateBearerTokenResponse) and inside the generated nginx config.
type BearerToken struct {
	ID        uuid.UUID `json:"id"`
	ProxyID   uuid.UUID `json:"proxy_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreateBearerTokenRequest is the request body for creating a bearer token.
type CreateBearerTokenRequest struct {
	Name string `json:"name" binding:"required"`
}

// CreateBearerTokenResponse includes the plaintext token -- shown exactly once, at
// creation. It is never returned by listBearerTokens or stored anywhere in plaintext.
type CreateBearerTokenResponse struct {
	BearerToken
	Token string `json:"token"`
}

// generateBearerToken mints a new opaque secret. Server-generated rather than
// user-typed, since a bearer token is meant to be an unguessable credential, not a
// chosen one like a Basic Auth password.
func generateBearerToken() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "ipt_" + hex.EncodeToString(raw), nil
}

// encryptToken encrypts a token for storage, using the same enc:v1: marker scheme as
// secrets_crypto.go. Fails open to plaintext if no ENCRYPTION_KEY is configured,
// matching that file's philosophy of working without encryption rather than breaking
// the feature entirely.
func (h *Handler) encryptToken(token string) string {
	if h.encryptionSvc == nil {
		return token
	}
	ct, err := h.encryptionSvc.EncryptString(token)
	if err != nil {
		h.logger.Error("Failed to encrypt bearer token, storing in plaintext", zap.Error(err))
		return token
	}
	return secretEncPrefix + ct
}

// decryptToken reverses encryptToken. A value with no enc:v1: prefix is legacy/already
// plaintext and passes through untouched, same as decryptSecretValues.
func (h *Handler) decryptToken(stored string) (string, error) {
	if !strings.HasPrefix(stored, secretEncPrefix) {
		return stored, nil
	}
	if h.encryptionSvc == nil {
		return "", fmt.Errorf("token is encrypted but no encryption key is configured")
	}
	return h.encryptionSvc.DecryptString(strings.TrimPrefix(stored, secretEncPrefix))
}

// decryptBearerTokens returns the plaintext tokens for a proxy, for embedding into its
// generated nginx config. Never returned to a client.
func (h *Handler) decryptBearerTokens(ctx context.Context, proxyID uuid.UUID) ([]string, error) {
	rows, err := h.db.Query(ctx, `
		SELECT token_encrypted FROM proxy_bearer_tokens WHERE proxy_id = $1
	`, proxyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []string
	for rows.Next() {
		var stored string
		if err := rows.Scan(&stored); err != nil {
			continue
		}
		token, err := h.decryptToken(stored)
		if err != nil {
			h.logger.Error("Failed to decrypt bearer token", zap.Error(err), zap.String("proxy_id", proxyID.String()))
			continue
		}
		tokens = append(tokens, token)
	}
	return tokens, nil
}

// listBearerTokens returns all bearer tokens for a proxy (metadata only, never the
// token value itself).
func (h *Handler) listBearerTokens(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	agentIDStr := c.Param("id")
	proxyIDStr := c.Param("pid")

	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	proxyID, err := uuid.Parse(proxyIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid proxy ID"})
		return
	}

	var exists bool
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT EXISTS(
			SELECT 1 FROM proxy_hosts p
			JOIN agents a ON p.agent_id = a.id
			WHERE p.id = $1 AND p.agent_id = $2 AND a.org_id = $3
		)
	`, proxyID, agentID, orgID).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "proxy host not found"})
		return
	}

	rows, err := h.db.Query(c.Request.Context(), `
		SELECT id, proxy_id, name, created_at, updated_at
		FROM proxy_bearer_tokens
		WHERE proxy_id = $1
		ORDER BY created_at ASC
	`, proxyID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch bearer tokens"})
		return
	}
	defer rows.Close()

	tokens := []BearerToken{}
	for rows.Next() {
		var t BearerToken
		if err := rows.Scan(&t.ID, &t.ProxyID, &t.Name, &t.CreatedAt, &t.UpdatedAt); err != nil {
			continue
		}
		tokens = append(tokens, t)
	}

	c.JSON(http.StatusOK, tokens)
}

// createBearerToken generates a new bearer token for a proxy. The plaintext value is
// returned once in this response and never again.
func (h *Handler) createBearerToken(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	userID := c.MustGet("user_id").(uuid.UUID)
	agentIDStr := c.Param("id")
	proxyIDStr := c.Param("pid")

	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	proxyID, err := uuid.Parse(proxyIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid proxy ID"})
		return
	}

	var req CreateBearerTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	var exists bool
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT EXISTS(
			SELECT 1 FROM proxy_hosts p
			JOIN agents a ON p.agent_id = a.id
			WHERE p.id = $1 AND p.agent_id = $2 AND a.org_id = $3
		)
	`, proxyID, agentID, orgID).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "proxy host not found"})
		return
	}

	token, err := generateBearerToken()
	if err != nil {
		h.logger.Error("Failed to generate bearer token", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create token"})
		return
	}

	var tokenID uuid.UUID
	err = h.db.QueryRow(c.Request.Context(), `
		INSERT INTO proxy_bearer_tokens (proxy_id, name, token_encrypted)
		VALUES ($1, $2, $3)
		RETURNING id
	`, proxyID, req.Name, h.encryptToken(token)).Scan(&tokenID)

	if err != nil {
		h.logger.Error("Failed to create bearer token", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create token"})
		return
	}

	h.auditLog(c, userID, orgID, "proxy.bearer_token.create", "proxy_bearer_token", tokenID, gin.H{"name": req.Name})

	// Same 100ms-after-commit pattern as createAuthUser/deleteAuthUser.
	go func() {
		time.Sleep(100 * time.Millisecond)
		h.redispatchProxyConfig(context.Background(), agentID, proxyID)
	}()

	var created BearerToken
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT id, proxy_id, name, created_at, updated_at
		FROM proxy_bearer_tokens
		WHERE id = $1
	`, tokenID).Scan(&created.ID, &created.ProxyID, &created.Name, &created.CreatedAt, &created.UpdatedAt)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch created token"})
		return
	}

	c.JSON(http.StatusCreated, CreateBearerTokenResponse{BearerToken: created, Token: token})
}

// deleteBearerToken removes a bearer token from a proxy.
func (h *Handler) deleteBearerToken(c *gin.Context) {
	orgID := c.MustGet("org_id").(uuid.UUID)
	userID := c.MustGet("user_id").(uuid.UUID)
	agentIDStr := c.Param("id")
	proxyIDStr := c.Param("pid")
	tokenIDStr := c.Param("tid")

	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	proxyID, err := uuid.Parse(proxyIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid proxy ID"})
		return
	}

	tokenID, err := uuid.Parse(tokenIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid token ID"})
		return
	}

	var exists bool
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT EXISTS(
			SELECT 1 FROM proxy_hosts p
			JOIN agents a ON p.agent_id = a.id
			WHERE p.id = $1 AND p.agent_id = $2 AND a.org_id = $3
		)
	`, proxyID, agentID, orgID).Scan(&exists)

	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "proxy host not found"})
		return
	}

	var name string
	err = h.db.QueryRow(c.Request.Context(), `
		SELECT name FROM proxy_bearer_tokens WHERE id = $1 AND proxy_id = $2
	`, tokenID, proxyID).Scan(&name)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "bearer token not found"})
		return
	}

	result, err := h.db.Exec(c.Request.Context(), `
		DELETE FROM proxy_bearer_tokens WHERE id = $1 AND proxy_id = $2
	`, tokenID, proxyID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete bearer token"})
		return
	}

	if result.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "bearer token not found"})
		return
	}

	h.auditLog(c, userID, orgID, "proxy.bearer_token.delete", "proxy_bearer_token", tokenID, gin.H{"name": name})

	go func() {
		time.Sleep(100 * time.Millisecond)
		h.redispatchProxyConfig(context.Background(), agentID, proxyID)
	}()

	c.JSON(http.StatusOK, gin.H{"message": "bearer token deleted"})
}
