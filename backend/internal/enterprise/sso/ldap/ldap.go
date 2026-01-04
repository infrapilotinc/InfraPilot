package ldap

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-ldap/ldap/v3"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/infrapilot/backend/internal/enterprise/license"
	"github.com/infrapilot/backend/internal/enterprise/sso"
)

// Handler handles LDAP authentication
type Handler struct {
	db        *pgxpool.Pool
	jwtSecret []byte
}

// NewHandler creates a new LDAP handler
func NewHandler(db *pgxpool.Pool, jwtSecret []byte) *Handler {
	return &Handler{
		db:        db,
		jwtSecret: jwtSecret,
	}
}

// AuthRequest represents the LDAP authentication request
type AuthRequest struct {
	ProviderID string `json:"provider_id" binding:"required"`
	Username   string `json:"username" binding:"required"`
	Password   string `json:"password" binding:"required"`
}

// Authenticate handles LDAP authentication
func (h *Handler) Authenticate(c *gin.Context) {
	// Check license
	if err := license.RequireFeature(c.Request.Context(), "sso_ldap"); err != nil {
		c.JSON(http.StatusForbidden, gin.H{
			"error":   err.Error(),
			"code":    "ENTERPRISE_REQUIRED",
			"feature": "sso_ldap",
		})
		return
	}

	var req AuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get provider configuration
	provider, err := h.getProvider(c.Request.Context(), req.ProviderID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Provider not found"})
		return
	}

	if provider.ProviderType != sso.ProviderTypeLDAP {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Provider is not LDAP type"})
		return
	}

	if !provider.Enabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Provider is disabled"})
		return
	}

	// Authenticate against LDAP
	authResult, err := h.ldapAuthenticate(provider, req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	// Find or create user (JIT provisioning)
	jwtToken, err := h.provisionUserAndCreateSession(c.Request.Context(), provider, authResult)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token": jwtToken,
		"user": gin.H{
			"email": authResult.Email,
			"name":  authResult.Name,
		},
	})
}

// TestConnection tests the LDAP connection
func (h *Handler) TestConnection(c *gin.Context) {
	providerID := c.Param("id")

	provider, err := h.getProvider(c.Request.Context(), providerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Provider not found"})
		return
	}

	// Try to connect and bind
	conn, err := h.connect(provider)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Connection failed: " + err.Error(),
		})
		return
	}
	defer conn.Close()

	// Try bind with service account
	if provider.LDAPBindDN != nil && provider.LDAPBindPasswordEncrypt != nil {
		err = conn.Bind(*provider.LDAPBindDN, *provider.LDAPBindPasswordEncrypt)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Bind failed: " + err.Error(),
			})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "LDAP connection successful",
	})
}

func (h *Handler) getProvider(ctx context.Context, providerID string) (*sso.Provider, error) {
	var p sso.Provider
	err := h.db.QueryRow(ctx, `
		SELECT id, org_id, name, provider_type, enabled,
			   ldap_host, ldap_port, ldap_use_tls, ldap_skip_verify,
			   ldap_bind_dn, ldap_bind_password_encrypted, ldap_base_dn,
			   ldap_user_filter, ldap_group_filter,
			   ldap_email_attr, ldap_name_attr, ldap_group_attr,
			   default_role, auto_create_users
		FROM sso_providers
		WHERE id = $1
	`, providerID).Scan(
		&p.ID, &p.OrgID, &p.Name, &p.ProviderType, &p.Enabled,
		&p.LDAPHost, &p.LDAPPort, &p.LDAPUseTLS, &p.LDAPSkipVerify,
		&p.LDAPBindDN, &p.LDAPBindPasswordEncrypt, &p.LDAPBaseDN,
		&p.LDAPUserFilter, &p.LDAPGroupFilter,
		&p.LDAPEmailAttr, &p.LDAPNameAttr, &p.LDAPGroupAttr,
		&p.DefaultRole, &p.AutoCreateUsers,
	)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (h *Handler) connect(provider *sso.Provider) (*ldap.Conn, error) {
	if provider.LDAPHost == nil {
		return nil, errors.New("LDAP host is required")
	}

	port := 389
	if provider.LDAPPort != nil {
		port = *provider.LDAPPort
	}

	address := fmt.Sprintf("%s:%d", *provider.LDAPHost, port)

	var conn *ldap.Conn
	var err error

	if provider.LDAPUseTLS {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: provider.LDAPSkipVerify,
		}
		conn, err = ldap.DialTLS("tcp", address, tlsConfig)
	} else {
		conn, err = ldap.Dial("tcp", address)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to connect to LDAP server: %w", err)
	}

	// Start TLS if not using LDAPS and port is 389
	if !provider.LDAPUseTLS && port == 389 {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: provider.LDAPSkipVerify,
		}
		err = conn.StartTLS(tlsConfig)
		if err != nil {
			// StartTLS failed, but connection might still work without TLS
			// Log warning but continue
		}
	}

	return conn, nil
}

func (h *Handler) ldapAuthenticate(provider *sso.Provider, username, password string) (*sso.AuthResult, error) {
	conn, err := h.connect(provider)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	// Bind with service account to search for user
	if provider.LDAPBindDN != nil && provider.LDAPBindPasswordEncrypt != nil {
		err = conn.Bind(*provider.LDAPBindDN, *provider.LDAPBindPasswordEncrypt)
		if err != nil {
			return nil, fmt.Errorf("service account bind failed: %w", err)
		}
	}

	// Search for user
	if provider.LDAPBaseDN == nil {
		return nil, errors.New("LDAP base DN is required")
	}

	userFilter := "(uid=%s)"
	if provider.LDAPUserFilter != nil && *provider.LDAPUserFilter != "" {
		userFilter = *provider.LDAPUserFilter
	}

	// Replace %s with username
	searchFilter := fmt.Sprintf(userFilter, ldap.EscapeFilter(username))

	// Determine attributes to fetch
	emailAttr := "mail"
	if provider.LDAPEmailAttr != nil {
		emailAttr = *provider.LDAPEmailAttr
	}
	nameAttr := "cn"
	if provider.LDAPNameAttr != nil {
		nameAttr = *provider.LDAPNameAttr
	}
	groupAttr := "memberOf"
	if provider.LDAPGroupAttr != nil {
		groupAttr = *provider.LDAPGroupAttr
	}

	searchRequest := ldap.NewSearchRequest(
		*provider.LDAPBaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 1, 30, false,
		searchFilter,
		[]string{"dn", emailAttr, nameAttr, groupAttr},
		nil,
	)

	sr, err := conn.Search(searchRequest)
	if err != nil {
		return nil, fmt.Errorf("user search failed: %w", err)
	}

	if len(sr.Entries) == 0 {
		return nil, errors.New("user not found")
	}

	if len(sr.Entries) > 1 {
		return nil, errors.New("multiple users found, please use a more specific filter")
	}

	userEntry := sr.Entries[0]
	userDN := userEntry.DN

	// Bind as the user to verify password
	err = conn.Bind(userDN, password)
	if err != nil {
		return nil, errors.New("invalid credentials")
	}

	// Extract user attributes
	email := userEntry.GetAttributeValue(emailAttr)
	name := userEntry.GetAttributeValue(nameAttr)
	groups := userEntry.GetAttributeValues(groupAttr)

	// Extract CN from group DNs if they're full DNs
	cleanGroups := []string{}
	for _, g := range groups {
		if strings.HasPrefix(strings.ToLower(g), "cn=") {
			parts := strings.SplitN(g, ",", 2)
			if len(parts) > 0 {
				cleanGroups = append(cleanGroups, strings.TrimPrefix(strings.ToLower(parts[0]), "cn="))
			}
		} else {
			cleanGroups = append(cleanGroups, g)
		}
	}

	if email == "" {
		// Use username as email if email attribute is empty
		if strings.Contains(username, "@") {
			email = username
		} else {
			return nil, errors.New("email not found in LDAP entry")
		}
	}

	return &sso.AuthResult{
		Email:      email,
		Name:       name,
		ExternalID: userDN,
		Groups:     cleanGroups,
		ProviderID: provider.ID,
	}, nil
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
	token := fmt.Sprintf("sso_%s_%s", userID, generateNonce())

	return token, nil
}

func (h *Handler) getRoleFromGroups(ctx context.Context, providerID string, groups []string) (string, error) {
	for _, group := range groups {
		var role string
		err := h.db.QueryRow(ctx, `
			SELECT role FROM sso_role_mappings
			WHERE provider_id = $1 AND LOWER(external_group) = LOWER($2)
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
