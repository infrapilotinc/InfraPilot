package sso

import (
	"time"
)

// ProviderType represents the type of SSO provider
type ProviderType string

const (
	ProviderTypeSAML ProviderType = "saml"
	ProviderTypeOIDC ProviderType = "oidc"
	ProviderTypeLDAP ProviderType = "ldap"
)

// Provider represents an SSO provider configuration
type Provider struct {
	ID           string       `json:"id" db:"id"`
	OrgID        string       `json:"org_id" db:"org_id"`
	Name         string       `json:"name" db:"name"`
	ProviderType ProviderType `json:"provider_type" db:"provider_type"`
	Enabled      bool         `json:"enabled" db:"enabled"`

	// SAML specific
	SAMLEntityID      *string `json:"saml_entity_id,omitempty" db:"saml_entity_id"`
	SAMLSsoURL        *string `json:"saml_sso_url,omitempty" db:"saml_sso_url"`
	SAMLSloURL        *string `json:"saml_slo_url,omitempty" db:"saml_slo_url"`
	SAMLCertificate   *string `json:"saml_certificate,omitempty" db:"saml_certificate"`
	SAMLSignRequests  bool    `json:"saml_sign_requests" db:"saml_sign_requests"`
	SAMLNameIDFormat  *string `json:"saml_name_id_format,omitempty" db:"saml_name_id_format"`

	// OIDC specific
	OIDCIssuer              *string `json:"oidc_issuer,omitempty" db:"oidc_issuer"`
	OIDCClientID            *string `json:"oidc_client_id,omitempty" db:"oidc_client_id"`
	OIDCClientSecretEncrypt *string `json:"-" db:"oidc_client_secret_encrypted"`
	OIDCClientSecret        *string `json:"oidc_client_secret,omitempty" db:"-"` // For input only
	OIDCScopes              *string `json:"oidc_scopes,omitempty" db:"oidc_scopes"`
	OIDCRedirectURI         *string `json:"oidc_redirect_uri,omitempty" db:"oidc_redirect_uri"`

	// LDAP specific
	LDAPHost                 *string `json:"ldap_host,omitempty" db:"ldap_host"`
	LDAPPort                 *int    `json:"ldap_port,omitempty" db:"ldap_port"`
	LDAPUseTLS               bool    `json:"ldap_use_tls" db:"ldap_use_tls"`
	LDAPSkipVerify           bool    `json:"ldap_skip_verify" db:"ldap_skip_verify"`
	LDAPBindDN               *string `json:"ldap_bind_dn,omitempty" db:"ldap_bind_dn"`
	LDAPBindPasswordEncrypt  *string `json:"-" db:"ldap_bind_password_encrypted"`
	LDAPBindPassword         *string `json:"ldap_bind_password,omitempty" db:"-"` // For input only
	LDAPBaseDN               *string `json:"ldap_base_dn,omitempty" db:"ldap_base_dn"`
	LDAPUserFilter           *string `json:"ldap_user_filter,omitempty" db:"ldap_user_filter"`
	LDAPGroupFilter          *string `json:"ldap_group_filter,omitempty" db:"ldap_group_filter"`
	LDAPEmailAttr            *string `json:"ldap_email_attr,omitempty" db:"ldap_email_attr"`
	LDAPNameAttr             *string `json:"ldap_name_attr,omitempty" db:"ldap_name_attr"`
	LDAPGroupAttr            *string `json:"ldap_group_attr,omitempty" db:"ldap_group_attr"`

	// Common settings
	DefaultRole     string `json:"default_role" db:"default_role"`
	AutoCreateUsers bool   `json:"auto_create_users" db:"auto_create_users"`

	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// RoleMapping represents a mapping from external group to internal role
type RoleMapping struct {
	ID            string    `json:"id" db:"id"`
	ProviderID    string    `json:"provider_id" db:"provider_id"`
	ExternalGroup string    `json:"external_group" db:"external_group"`
	Role          string    `json:"role" db:"role"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
}

// Session represents an SSO session
type Session struct {
	ID                    string     `json:"id" db:"id"`
	UserID                string     `json:"user_id" db:"user_id"`
	ProviderID            string     `json:"provider_id" db:"provider_id"`
	ExternalID            *string    `json:"external_id,omitempty" db:"external_id"`
	SessionIndex          *string    `json:"session_index,omitempty" db:"session_index"`
	AccessTokenHash       *string    `json:"-" db:"access_token_hash"`
	RefreshTokenEncrypted *string    `json:"-" db:"refresh_token_encrypted"`
	CreatedAt             time.Time  `json:"created_at" db:"created_at"`
	ExpiresAt             *time.Time `json:"expires_at,omitempty" db:"expires_at"`
}

// CreateProviderRequest is the request body for creating an SSO provider
type CreateProviderRequest struct {
	Name         string       `json:"name" binding:"required"`
	ProviderType ProviderType `json:"provider_type" binding:"required,oneof=saml oidc ldap"`
	Enabled      *bool        `json:"enabled"`

	// SAML
	SAMLEntityID     *string `json:"saml_entity_id"`
	SAMLSsoURL       *string `json:"saml_sso_url"`
	SAMLSloURL       *string `json:"saml_slo_url"`
	SAMLCertificate  *string `json:"saml_certificate"`
	SAMLSignRequests *bool   `json:"saml_sign_requests"`
	SAMLNameIDFormat *string `json:"saml_name_id_format"`

	// OIDC
	OIDCIssuer       *string `json:"oidc_issuer"`
	OIDCClientID     *string `json:"oidc_client_id"`
	OIDCClientSecret *string `json:"oidc_client_secret"`
	OIDCScopes       *string `json:"oidc_scopes"`
	OIDCRedirectURI  *string `json:"oidc_redirect_uri"`

	// LDAP
	LDAPHost         *string `json:"ldap_host"`
	LDAPPort         *int    `json:"ldap_port"`
	LDAPUseTLS       *bool   `json:"ldap_use_tls"`
	LDAPSkipVerify   *bool   `json:"ldap_skip_verify"`
	LDAPBindDN       *string `json:"ldap_bind_dn"`
	LDAPBindPassword *string `json:"ldap_bind_password"`
	LDAPBaseDN       *string `json:"ldap_base_dn"`
	LDAPUserFilter   *string `json:"ldap_user_filter"`
	LDAPGroupFilter  *string `json:"ldap_group_filter"`
	LDAPEmailAttr    *string `json:"ldap_email_attr"`
	LDAPNameAttr     *string `json:"ldap_name_attr"`
	LDAPGroupAttr    *string `json:"ldap_group_attr"`

	// Common
	DefaultRole     *string `json:"default_role"`
	AutoCreateUsers *bool   `json:"auto_create_users"`
}

// UpdateProviderRequest is the request body for updating an SSO provider
type UpdateProviderRequest struct {
	Name    *string `json:"name"`
	Enabled *bool   `json:"enabled"`

	// SAML
	SAMLEntityID     *string `json:"saml_entity_id"`
	SAMLSsoURL       *string `json:"saml_sso_url"`
	SAMLSloURL       *string `json:"saml_slo_url"`
	SAMLCertificate  *string `json:"saml_certificate"`
	SAMLSignRequests *bool   `json:"saml_sign_requests"`
	SAMLNameIDFormat *string `json:"saml_name_id_format"`

	// OIDC
	OIDCIssuer       *string `json:"oidc_issuer"`
	OIDCClientID     *string `json:"oidc_client_id"`
	OIDCClientSecret *string `json:"oidc_client_secret"`
	OIDCScopes       *string `json:"oidc_scopes"`
	OIDCRedirectURI  *string `json:"oidc_redirect_uri"`

	// LDAP
	LDAPHost         *string `json:"ldap_host"`
	LDAPPort         *int    `json:"ldap_port"`
	LDAPUseTLS       *bool   `json:"ldap_use_tls"`
	LDAPSkipVerify   *bool   `json:"ldap_skip_verify"`
	LDAPBindDN       *string `json:"ldap_bind_dn"`
	LDAPBindPassword *string `json:"ldap_bind_password"`
	LDAPBaseDN       *string `json:"ldap_base_dn"`
	LDAPUserFilter   *string `json:"ldap_user_filter"`
	LDAPGroupFilter  *string `json:"ldap_group_filter"`
	LDAPEmailAttr    *string `json:"ldap_email_attr"`
	LDAPNameAttr     *string `json:"ldap_name_attr"`
	LDAPGroupAttr    *string `json:"ldap_group_attr"`

	// Common
	DefaultRole     *string `json:"default_role"`
	AutoCreateUsers *bool   `json:"auto_create_users"`
}

// CreateRoleMappingRequest is the request body for creating a role mapping
type CreateRoleMappingRequest struct {
	ExternalGroup string `json:"external_group" binding:"required"`
	Role          string `json:"role" binding:"required,oneof=super_admin admin operator viewer"`
}

// AuthResult represents the result of an SSO authentication
type AuthResult struct {
	Email       string
	Name        string
	ExternalID  string
	Groups      []string
	RawClaims   map[string]interface{}
	ProviderID  string
	AccessToken string
}
