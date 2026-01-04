package sso

import (
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/infrapilot/backend/internal/enterprise/license"
)

// Handler handles SSO-related HTTP requests
type Handler struct {
	db *pgxpool.Pool
}

// NewHandler creates a new SSO handler
func NewHandler(db *pgxpool.Pool) *Handler {
	return &Handler{db: db}
}

// ListProviders returns all SSO providers for the organization
func (h *Handler) ListProviders(c *gin.Context) {
	orgID := c.GetString("org_id")

	rows, err := h.db.Query(c.Request.Context(), `
		SELECT id, org_id, name, provider_type, enabled,
			   saml_entity_id, saml_sso_url, saml_slo_url, saml_sign_requests, saml_name_id_format,
			   oidc_issuer, oidc_client_id, oidc_scopes, oidc_redirect_uri,
			   ldap_host, ldap_port, ldap_use_tls, ldap_skip_verify, ldap_bind_dn, ldap_base_dn,
			   ldap_user_filter, ldap_group_filter, ldap_email_attr, ldap_name_attr, ldap_group_attr,
			   default_role, auto_create_users, created_at, updated_at
		FROM sso_providers
		WHERE org_id = $1
		ORDER BY created_at DESC
	`, orgID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch providers"})
		return
	}
	defer rows.Close()

	providers := []Provider{}
	for rows.Next() {
		var p Provider
		err := rows.Scan(
			&p.ID, &p.OrgID, &p.Name, &p.ProviderType, &p.Enabled,
			&p.SAMLEntityID, &p.SAMLSsoURL, &p.SAMLSloURL, &p.SAMLSignRequests, &p.SAMLNameIDFormat,
			&p.OIDCIssuer, &p.OIDCClientID, &p.OIDCScopes, &p.OIDCRedirectURI,
			&p.LDAPHost, &p.LDAPPort, &p.LDAPUseTLS, &p.LDAPSkipVerify, &p.LDAPBindDN, &p.LDAPBaseDN,
			&p.LDAPUserFilter, &p.LDAPGroupFilter, &p.LDAPEmailAttr, &p.LDAPNameAttr, &p.LDAPGroupAttr,
			&p.DefaultRole, &p.AutoCreateUsers, &p.CreatedAt, &p.UpdatedAt,
		)
		if err != nil {
			continue
		}
		providers = append(providers, p)
	}

	c.JSON(http.StatusOK, providers)
}

// GetProvider returns a single SSO provider
func (h *Handler) GetProvider(c *gin.Context) {
	orgID := c.GetString("org_id")
	providerID := c.Param("id")

	var p Provider
	err := h.db.QueryRow(c.Request.Context(), `
		SELECT id, org_id, name, provider_type, enabled,
			   saml_entity_id, saml_sso_url, saml_slo_url, saml_sign_requests, saml_name_id_format,
			   oidc_issuer, oidc_client_id, oidc_scopes, oidc_redirect_uri,
			   ldap_host, ldap_port, ldap_use_tls, ldap_skip_verify, ldap_bind_dn, ldap_base_dn,
			   ldap_user_filter, ldap_group_filter, ldap_email_attr, ldap_name_attr, ldap_group_attr,
			   default_role, auto_create_users, created_at, updated_at
		FROM sso_providers
		WHERE id = $1 AND org_id = $2
	`, providerID, orgID).Scan(
		&p.ID, &p.OrgID, &p.Name, &p.ProviderType, &p.Enabled,
		&p.SAMLEntityID, &p.SAMLSsoURL, &p.SAMLSloURL, &p.SAMLSignRequests, &p.SAMLNameIDFormat,
		&p.OIDCIssuer, &p.OIDCClientID, &p.OIDCScopes, &p.OIDCRedirectURI,
		&p.LDAPHost, &p.LDAPPort, &p.LDAPUseTLS, &p.LDAPSkipVerify, &p.LDAPBindDN, &p.LDAPBaseDN,
		&p.LDAPUserFilter, &p.LDAPGroupFilter, &p.LDAPEmailAttr, &p.LDAPNameAttr, &p.LDAPGroupAttr,
		&p.DefaultRole, &p.AutoCreateUsers, &p.CreatedAt, &p.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Provider not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch provider"})
		return
	}

	c.JSON(http.StatusOK, p)
}

// CreateProvider creates a new SSO provider
func (h *Handler) CreateProvider(c *gin.Context) {
	orgID := c.GetString("org_id")

	// Check license for the specific SSO type
	var req CreateProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check feature license
	featureKey := "sso_" + string(req.ProviderType)
	if err := license.RequireFeature(c.Request.Context(), featureKey); err != nil {
		c.JSON(http.StatusForbidden, gin.H{
			"error":   err.Error(),
			"code":    "ENTERPRISE_REQUIRED",
			"feature": featureKey,
		})
		return
	}

	// Set defaults
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	defaultRole := "viewer"
	if req.DefaultRole != nil {
		defaultRole = *req.DefaultRole
	}
	autoCreate := true
	if req.AutoCreateUsers != nil {
		autoCreate = *req.AutoCreateUsers
	}

	// TODO: Encrypt secrets before storing
	var clientSecretEncrypted *string
	if req.OIDCClientSecret != nil {
		// For now, store as-is (should encrypt in production)
		clientSecretEncrypted = req.OIDCClientSecret
	}
	var bindPasswordEncrypted *string
	if req.LDAPBindPassword != nil {
		bindPasswordEncrypted = req.LDAPBindPassword
	}

	var providerID string
	err := h.db.QueryRow(c.Request.Context(), `
		INSERT INTO sso_providers (
			org_id, name, provider_type, enabled,
			saml_entity_id, saml_sso_url, saml_slo_url, saml_certificate, saml_sign_requests, saml_name_id_format,
			oidc_issuer, oidc_client_id, oidc_client_secret_encrypted, oidc_scopes, oidc_redirect_uri,
			ldap_host, ldap_port, ldap_use_tls, ldap_skip_verify, ldap_bind_dn, ldap_bind_password_encrypted,
			ldap_base_dn, ldap_user_filter, ldap_group_filter, ldap_email_attr, ldap_name_attr, ldap_group_attr,
			default_role, auto_create_users
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7, $8, $9, $10,
			$11, $12, $13, $14, $15,
			$16, $17, $18, $19, $20, $21,
			$22, $23, $24, $25, $26, $27,
			$28, $29
		) RETURNING id
	`,
		orgID, req.Name, req.ProviderType, enabled,
		req.SAMLEntityID, req.SAMLSsoURL, req.SAMLSloURL, req.SAMLCertificate, req.SAMLSignRequests, req.SAMLNameIDFormat,
		req.OIDCIssuer, req.OIDCClientID, clientSecretEncrypted, req.OIDCScopes, req.OIDCRedirectURI,
		req.LDAPHost, req.LDAPPort, req.LDAPUseTLS, req.LDAPSkipVerify, req.LDAPBindDN, bindPasswordEncrypted,
		req.LDAPBaseDN, req.LDAPUserFilter, req.LDAPGroupFilter, req.LDAPEmailAttr, req.LDAPNameAttr, req.LDAPGroupAttr,
		defaultRole, autoCreate,
	).Scan(&providerID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create provider: " + err.Error()})
		return
	}

	// Fetch the created provider
	c.Set("org_id", orgID)
	c.Params = append(c.Params, gin.Param{Key: "id", Value: providerID})
	h.GetProvider(c)
}

// UpdateProvider updates an existing SSO provider
func (h *Handler) UpdateProvider(c *gin.Context) {
	orgID := c.GetString("org_id")
	providerID := c.Param("id")

	var req UpdateProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Build dynamic update query
	query := `UPDATE sso_providers SET updated_at = $1`
	args := []interface{}{time.Now()}
	argNum := 2

	if req.Name != nil {
		query += fmt.Sprintf(`, name = $%d`, argNum)
		args = append(args, *req.Name)
		argNum++
	}
	if req.Enabled != nil {
		query += fmt.Sprintf(`, enabled = $%d`, argNum)
		args = append(args, *req.Enabled)
		argNum++
	}
	// SAML fields
	if req.SAMLEntityID != nil {
		query += fmt.Sprintf(`, saml_entity_id = $%d`, argNum)
		args = append(args, *req.SAMLEntityID)
		argNum++
	}
	if req.SAMLSsoURL != nil {
		query += fmt.Sprintf(`, saml_sso_url = $%d`, argNum)
		args = append(args, *req.SAMLSsoURL)
		argNum++
	}
	if req.SAMLCertificate != nil {
		query += fmt.Sprintf(`, saml_certificate = $%d`, argNum)
		args = append(args, *req.SAMLCertificate)
		argNum++
	}
	// OIDC fields
	if req.OIDCIssuer != nil {
		query += fmt.Sprintf(`, oidc_issuer = $%d`, argNum)
		args = append(args, *req.OIDCIssuer)
		argNum++
	}
	if req.OIDCClientID != nil {
		query += fmt.Sprintf(`, oidc_client_id = $%d`, argNum)
		args = append(args, *req.OIDCClientID)
		argNum++
	}
	if req.OIDCClientSecret != nil {
		query += fmt.Sprintf(`, oidc_client_secret_encrypted = $%d`, argNum)
		args = append(args, *req.OIDCClientSecret)
		argNum++
	}
	if req.OIDCScopes != nil {
		query += fmt.Sprintf(`, oidc_scopes = $%d`, argNum)
		args = append(args, *req.OIDCScopes)
		argNum++
	}
	// Common fields
	if req.DefaultRole != nil {
		query += fmt.Sprintf(`, default_role = $%d`, argNum)
		args = append(args, *req.DefaultRole)
		argNum++
	}
	if req.AutoCreateUsers != nil {
		query += fmt.Sprintf(`, auto_create_users = $%d`, argNum)
		args = append(args, *req.AutoCreateUsers)
		argNum++
	}

	query += fmt.Sprintf(` WHERE id = $%d AND org_id = $%d`, argNum, argNum+1)
	args = append(args, providerID, orgID)

	_, err := h.db.Exec(c.Request.Context(), query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update provider"})
		return
	}

	h.GetProvider(c)
}

// DeleteProvider deletes an SSO provider
func (h *Handler) DeleteProvider(c *gin.Context) {
	orgID := c.GetString("org_id")
	providerID := c.Param("id")

	result, err := h.db.Exec(c.Request.Context(), `
		DELETE FROM sso_providers WHERE id = $1 AND org_id = $2
	`, providerID, orgID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete provider"})
		return
	}

	if result.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Provider not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Provider deleted"})
}

// ListRoleMappings returns role mappings for a provider
func (h *Handler) ListRoleMappings(c *gin.Context) {
	orgID := c.GetString("org_id")
	providerID := c.Param("id")

	// Verify provider belongs to org
	var exists bool
	h.db.QueryRow(c.Request.Context(),
		`SELECT EXISTS(SELECT 1 FROM sso_providers WHERE id = $1 AND org_id = $2)`,
		providerID, orgID).Scan(&exists)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Provider not found"})
		return
	}

	rows, err := h.db.Query(c.Request.Context(), `
		SELECT id, provider_id, external_group, role, created_at
		FROM sso_role_mappings
		WHERE provider_id = $1
		ORDER BY created_at
	`, providerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch mappings"})
		return
	}
	defer rows.Close()

	mappings := []RoleMapping{}
	for rows.Next() {
		var m RoleMapping
		if err := rows.Scan(&m.ID, &m.ProviderID, &m.ExternalGroup, &m.Role, &m.CreatedAt); err != nil {
			continue
		}
		mappings = append(mappings, m)
	}

	c.JSON(http.StatusOK, mappings)
}

// CreateRoleMapping creates a new role mapping
func (h *Handler) CreateRoleMapping(c *gin.Context) {
	orgID := c.GetString("org_id")
	providerID := c.Param("id")

	// Verify provider belongs to org
	var exists bool
	h.db.QueryRow(c.Request.Context(),
		`SELECT EXISTS(SELECT 1 FROM sso_providers WHERE id = $1 AND org_id = $2)`,
		providerID, orgID).Scan(&exists)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Provider not found"})
		return
	}

	var req CreateRoleMappingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var mappingID string
	err := h.db.QueryRow(c.Request.Context(), `
		INSERT INTO sso_role_mappings (provider_id, external_group, role)
		VALUES ($1, $2, $3)
		ON CONFLICT (provider_id, external_group) DO UPDATE SET role = $3
		RETURNING id
	`, providerID, req.ExternalGroup, req.Role).Scan(&mappingID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create mapping"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":             mappingID,
		"provider_id":    providerID,
		"external_group": req.ExternalGroup,
		"role":           req.Role,
	})
}

// DeleteRoleMapping deletes a role mapping
func (h *Handler) DeleteRoleMapping(c *gin.Context) {
	orgID := c.GetString("org_id")
	providerID := c.Param("id")
	mappingID := c.Param("mid")

	// Verify provider belongs to org
	var exists bool
	h.db.QueryRow(c.Request.Context(),
		`SELECT EXISTS(SELECT 1 FROM sso_providers WHERE id = $1 AND org_id = $2)`,
		providerID, orgID).Scan(&exists)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Provider not found"})
		return
	}

	result, err := h.db.Exec(c.Request.Context(), `
		DELETE FROM sso_role_mappings WHERE id = $1 AND provider_id = $2
	`, mappingID, providerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete mapping"})
		return
	}

	if result.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Mapping not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Mapping deleted"})
}

// GetPublicProviders returns enabled SSO providers for the login page (no auth required)
func (h *Handler) GetPublicProviders(c *gin.Context) {
	// Get org_id from query param or header (for multi-tenant)
	// For now, return all enabled providers (single-tenant mode)
	rows, err := h.db.Query(c.Request.Context(), `
		SELECT id, name, provider_type
		FROM sso_providers
		WHERE enabled = true
		ORDER BY name
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch providers"})
		return
	}
	defer rows.Close()

	type PublicProvider struct {
		ID           string       `json:"id"`
		Name         string       `json:"name"`
		ProviderType ProviderType `json:"provider_type"`
	}

	providers := []PublicProvider{}
	for rows.Next() {
		var p PublicProvider
		if err := rows.Scan(&p.ID, &p.Name, &p.ProviderType); err != nil {
			continue
		}
		providers = append(providers, p)
	}

	c.JSON(http.StatusOK, providers)
}
