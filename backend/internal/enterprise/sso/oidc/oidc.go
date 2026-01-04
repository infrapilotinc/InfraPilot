package oidc

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/oauth2"

	"github.com/infrapilot/backend/internal/enterprise/license"
	"github.com/infrapilot/backend/internal/enterprise/sso"
)

// Handler handles OIDC authentication
type Handler struct {
	db         *pgxpool.Pool
	baseURL    string
	jwtSecret  []byte
	providers  map[string]*oauth2.Config // Cache of OAuth2 configs by provider ID
	verifiers  map[string]*oidc.IDTokenVerifier
}

// NewHandler creates a new OIDC handler
func NewHandler(db *pgxpool.Pool, baseURL string, jwtSecret []byte) *Handler {
	return &Handler{
		db:        db,
		baseURL:   baseURL,
		jwtSecret: jwtSecret,
		providers: make(map[string]*oauth2.Config),
		verifiers: make(map[string]*oidc.IDTokenVerifier),
	}
}

// StateData holds the state for OIDC callback verification
type StateData struct {
	ProviderID string `json:"provider_id"`
	Nonce      string `json:"nonce"`
	ReturnTo   string `json:"return_to"`
}

// Authorize initiates the OIDC authorization flow
func (h *Handler) Authorize(c *gin.Context) {
	// Check license
	if err := license.RequireFeature(c.Request.Context(), "sso_oidc"); err != nil {
		c.JSON(http.StatusForbidden, gin.H{
			"error":   err.Error(),
			"code":    "ENTERPRISE_REQUIRED",
			"feature": "sso_oidc",
		})
		return
	}

	providerID := c.Query("provider_id")
	if providerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "provider_id is required"})
		return
	}

	returnTo := c.Query("return_to")
	if returnTo == "" {
		returnTo = "/"
	}

	// Get provider configuration
	provider, err := h.getProvider(c.Request.Context(), providerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Provider not found"})
		return
	}

	if provider.ProviderType != sso.ProviderTypeOIDC {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Provider is not OIDC type"})
		return
	}

	if !provider.Enabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Provider is disabled"})
		return
	}

	// Get OAuth2 config
	oauth2Config, verifier, err := h.getOAuth2Config(c.Request.Context(), provider)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to configure OIDC: " + err.Error()})
		return
	}

	// Generate state with nonce
	nonce := generateNonce()
	state := fmt.Sprintf("%s:%s:%s", providerID, nonce, base64.URLEncoding.EncodeToString([]byte(returnTo)))

	// Store state in cache/session (for simplicity, using cookie)
	c.SetCookie("oidc_state", state, 600, "/", "", false, true)

	// Cache verifier for callback
	h.verifiers[providerID] = verifier

	// Redirect to IdP
	authURL := oauth2Config.AuthCodeURL(state,
		oauth2.SetAuthURLParam("nonce", nonce),
		oauth2.AccessTypeOffline,
	)

	c.Redirect(http.StatusFound, authURL)
}

// Callback handles the OIDC callback from the IdP
func (h *Handler) Callback(c *gin.Context) {
	// Verify state
	stateCookie, err := c.Cookie("oidc_state")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing state cookie"})
		return
	}

	stateParam := c.Query("state")
	if stateParam != stateCookie {
		c.JSON(http.StatusBadRequest, gin.H{"error": "State mismatch"})
		return
	}

	// Parse state
	stateParts := strings.SplitN(stateParam, ":", 3)
	if len(stateParts) < 2 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid state format"})
		return
	}

	providerID := stateParts[0]
	nonce := stateParts[1]
	returnTo := "/"
	if len(stateParts) == 3 {
		decoded, _ := base64.URLEncoding.DecodeString(stateParts[2])
		returnTo = string(decoded)
	}

	// Check for error from IdP
	if errParam := c.Query("error"); errParam != "" {
		errDesc := c.Query("error_description")
		c.JSON(http.StatusBadRequest, gin.H{
			"error":       errParam,
			"description": errDesc,
		})
		return
	}

	// Get authorization code
	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing authorization code"})
		return
	}

	// Get provider configuration
	provider, err := h.getProvider(c.Request.Context(), providerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Provider not found"})
		return
	}

	// Get OAuth2 config
	oauth2Config, verifier, err := h.getOAuth2Config(c.Request.Context(), provider)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to configure OIDC"})
		return
	}

	// Exchange code for tokens
	token, err := oauth2Config.Exchange(c.Request.Context(), code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Token exchange failed: " + err.Error()})
		return
	}

	// Extract ID token
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "No id_token in response"})
		return
	}

	// Verify ID token
	idToken, err := verifier.Verify(c.Request.Context(), rawIDToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "ID token verification failed: " + err.Error()})
		return
	}

	// Verify nonce
	if idToken.Nonce != nonce {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Nonce mismatch"})
		return
	}

	// Extract claims
	var claims struct {
		Email         string   `json:"email"`
		EmailVerified bool     `json:"email_verified"`
		Name          string   `json:"name"`
		Groups        []string `json:"groups"`
		Sub           string   `json:"sub"`
	}
	if err := idToken.Claims(&claims); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse claims"})
		return
	}

	if claims.Email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Email not provided by IdP"})
		return
	}

	// Find or create user (JIT provisioning)
	authResult := &sso.AuthResult{
		Email:      claims.Email,
		Name:       claims.Name,
		ExternalID: claims.Sub,
		Groups:     claims.Groups,
		ProviderID: providerID,
	}

	jwtToken, err := h.provisionUserAndCreateSession(c.Request.Context(), provider, authResult)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Clear state cookie
	c.SetCookie("oidc_state", "", -1, "/", "", false, true)

	// Redirect with token (or set cookie)
	// For SPA, redirect to frontend with token in query param (or use secure cookie)
	redirectURL := fmt.Sprintf("%s?token=%s", returnTo, jwtToken)
	c.Redirect(http.StatusFound, redirectURL)
}

func (h *Handler) getProvider(ctx context.Context, providerID string) (*sso.Provider, error) {
	var p sso.Provider
	err := h.db.QueryRow(ctx, `
		SELECT id, org_id, name, provider_type, enabled,
			   oidc_issuer, oidc_client_id, oidc_client_secret_encrypted, oidc_scopes, oidc_redirect_uri,
			   default_role, auto_create_users
		FROM sso_providers
		WHERE id = $1 AND enabled = true
	`, providerID).Scan(
		&p.ID, &p.OrgID, &p.Name, &p.ProviderType, &p.Enabled,
		&p.OIDCIssuer, &p.OIDCClientID, &p.OIDCClientSecretEncrypt, &p.OIDCScopes, &p.OIDCRedirectURI,
		&p.DefaultRole, &p.AutoCreateUsers,
	)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (h *Handler) getOAuth2Config(ctx context.Context, provider *sso.Provider) (*oauth2.Config, *oidc.IDTokenVerifier, error) {
	if provider.OIDCIssuer == nil || provider.OIDCClientID == nil {
		return nil, nil, errors.New("OIDC issuer and client_id are required")
	}

	// Create OIDC provider
	oidcProvider, err := oidc.NewProvider(ctx, *provider.OIDCIssuer)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create OIDC provider: %w", err)
	}

	// Get client secret
	clientSecret := ""
	if provider.OIDCClientSecretEncrypt != nil {
		// TODO: Decrypt in production
		clientSecret = *provider.OIDCClientSecretEncrypt
	}

	// Determine redirect URI
	redirectURI := h.baseURL + "/api/v1/auth/oidc/callback"
	if provider.OIDCRedirectURI != nil && *provider.OIDCRedirectURI != "" {
		redirectURI = *provider.OIDCRedirectURI
	}

	// Parse scopes
	scopes := []string{oidc.ScopeOpenID, "profile", "email"}
	if provider.OIDCScopes != nil && *provider.OIDCScopes != "" {
		scopes = strings.Split(*provider.OIDCScopes, " ")
	}

	oauth2Config := &oauth2.Config{
		ClientID:     *provider.OIDCClientID,
		ClientSecret: clientSecret,
		Endpoint:     oidcProvider.Endpoint(),
		RedirectURL:  redirectURI,
		Scopes:       scopes,
	}

	verifier := oidcProvider.Verifier(&oidc.Config{
		ClientID: *provider.OIDCClientID,
	})

	return oauth2Config, verifier, nil
}

func (h *Handler) provisionUserAndCreateSession(ctx context.Context, provider *sso.Provider, result *sso.AuthResult) (string, error) {
	// Check if user exists
	var userID, role string
	err := h.db.QueryRow(ctx, `
		SELECT id, role FROM users WHERE email = $1 AND org_id = $2
	`, result.Email, provider.OrgID).Scan(&userID, &role)

	if err != nil {
		// User doesn't exist - create if auto_create_users is enabled
		if !provider.AutoCreateUsers {
			return "", errors.New("user not found and auto-creation is disabled")
		}

		// Determine role from group mappings
		role = provider.DefaultRole
		if len(result.Groups) > 0 {
			mappedRole, err := h.getRoleFromGroups(ctx, provider.ID, result.Groups)
			if err == nil && mappedRole != "" {
				role = mappedRole
			}
		}

		// Create user
		err = h.db.QueryRow(ctx, `
			INSERT INTO users (org_id, email, password_hash, role, sso_provider_id, sso_external_id)
			VALUES ($1, $2, '', $3, $4, $5)
			RETURNING id
		`, provider.OrgID, result.Email, role, provider.ID, result.ExternalID).Scan(&userID)
		if err != nil {
			return "", fmt.Errorf("failed to create user: %w", err)
		}
	} else {
		// Update SSO info for existing user
		_, err = h.db.Exec(ctx, `
			UPDATE users SET sso_provider_id = $1, sso_external_id = $2, updated_at = NOW()
			WHERE id = $3
		`, provider.ID, result.ExternalID, userID)
		if err != nil {
			return "", fmt.Errorf("failed to update user SSO info: %w", err)
		}
	}

	// Create SSO session
	_, err = h.db.Exec(ctx, `
		INSERT INTO sso_sessions (user_id, provider_id, external_id, expires_at)
		VALUES ($1, $2, $3, $4)
	`, userID, provider.ID, result.ExternalID, time.Now().Add(24*time.Hour))
	if err != nil {
		// Non-fatal, continue
	}

	// Generate JWT token
	// TODO: Use proper JWT generation from auth service
	// For now, return a placeholder - this should be integrated with the main auth system
	token := fmt.Sprintf("sso_%s_%s", userID, generateNonce())

	return token, nil
}

func (h *Handler) getRoleFromGroups(ctx context.Context, providerID string, groups []string) (string, error) {
	for _, group := range groups {
		var role string
		err := h.db.QueryRow(ctx, `
			SELECT role FROM sso_role_mappings
			WHERE provider_id = $1 AND external_group = $2
		`, providerID, group).Scan(&role)
		if err == nil {
			return role, nil
		}
	}
	return "", errors.New("no matching role mapping found")
}

func generateNonce() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}
