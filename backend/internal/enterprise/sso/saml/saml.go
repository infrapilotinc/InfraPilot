package saml

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"encoding/xml"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/crewjam/saml"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/infrapilot/backend/internal/enterprise/license"
	"github.com/infrapilot/backend/internal/enterprise/sso"
)

// Handler handles SAML authentication
type Handler struct {
	db        *pgxpool.Pool
	baseURL   string
	jwtSecret []byte
	spKey     *rsa.PrivateKey
	spCert    *x509.Certificate
}

// NewHandler creates a new SAML handler
func NewHandler(db *pgxpool.Pool, baseURL string, jwtSecret []byte) *Handler {
	// Generate SP key pair (in production, load from config)
	key, cert := generateSPKeyPair()

	return &Handler{
		db:        db,
		baseURL:   baseURL,
		jwtSecret: jwtSecret,
		spKey:     key,
		spCert:    cert,
	}
}

// Metadata returns the SP metadata XML
func (h *Handler) Metadata(c *gin.Context) {
	providerID := c.Query("provider_id")

	sp := h.createServiceProvider(providerID)
	metadata := sp.Metadata()

	c.Header("Content-Type", "application/xml")
	c.Writer.Write([]byte(xml.Header))
	encoder := xml.NewEncoder(c.Writer)
	encoder.Encode(metadata)
}

// Authorize initiates the SAML authentication flow
func (h *Handler) Authorize(c *gin.Context) {
	// Check license
	if err := license.RequireFeature(c.Request.Context(), "sso_saml"); err != nil {
		c.JSON(http.StatusForbidden, gin.H{
			"error":   err.Error(),
			"code":    "ENTERPRISE_REQUIRED",
			"feature": "sso_saml",
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

	if provider.ProviderType != sso.ProviderTypeSAML {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Provider is not SAML type"})
		return
	}

	if !provider.Enabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Provider is disabled"})
		return
	}

	// Create SAML SP
	sp, err := h.createServiceProviderWithIdP(provider)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to configure SAML: " + err.Error()})
		return
	}

	// Generate relay state with provider ID and return URL
	relayState := fmt.Sprintf("%s:%s", providerID, base64.URLEncoding.EncodeToString([]byte(returnTo)))

	// Create AuthnRequest
	authnRequest, err := sp.MakeAuthenticationRequest(
		sp.GetSSOBindingLocation(saml.HTTPRedirectBinding),
		saml.HTTPRedirectBinding,
		saml.HTTPPostBinding,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create AuthnRequest"})
		return
	}

	// Build redirect URL
	redirectURL, err := authnRequest.Redirect(relayState, sp)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to build redirect URL"})
		return
	}

	c.Redirect(http.StatusFound, redirectURL.String())
}

// ACS handles the SAML Assertion Consumer Service (callback)
func (h *Handler) ACS(c *gin.Context) {
	// Parse relay state
	relayState := c.PostForm("RelayState")
	if relayState == "" {
		relayState = c.Query("RelayState")
	}

	parts := strings.SplitN(relayState, ":", 2)
	if len(parts) < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid relay state"})
		return
	}

	providerID := parts[0]
	returnTo := "/"
	if len(parts) == 2 {
		decoded, _ := base64.URLEncoding.DecodeString(parts[1])
		returnTo = string(decoded)
	}

	// Get provider configuration
	provider, err := h.getProvider(c.Request.Context(), providerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Provider not found"})
		return
	}

	// Create SAML SP with IdP config
	sp, err := h.createServiceProviderWithIdP(provider)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to configure SAML"})
		return
	}

	// Parse and validate SAML response
	samlResponse := c.PostForm("SAMLResponse")
	if samlResponse == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing SAMLResponse"})
		return
	}

	assertion, err := sp.ParseResponse(c.Request, []string{})
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "SAML response validation failed: " + err.Error()})
		return
	}

	// Extract attributes from assertion
	email := ""
	name := ""
	groups := []string{}
	externalID := ""

	if assertion.Subject != nil && assertion.Subject.NameID != nil {
		externalID = assertion.Subject.NameID.Value
		// Often the NameID is the email
		if strings.Contains(externalID, "@") {
			email = externalID
		}
	}

	for _, stmt := range assertion.AttributeStatements {
		for _, attr := range stmt.Attributes {
			values := []string{}
			for _, v := range attr.Values {
				values = append(values, v.Value)
			}

			switch strings.ToLower(attr.Name) {
			case "email", "emailaddress", "http://schemas.xmlsoap.org/ws/2005/05/identity/claims/emailaddress":
				if len(values) > 0 {
					email = values[0]
				}
			case "name", "displayname", "http://schemas.xmlsoap.org/ws/2005/05/identity/claims/name":
				if len(values) > 0 {
					name = values[0]
				}
			case "groups", "memberof", "http://schemas.microsoft.com/ws/2008/06/identity/claims/groups":
				groups = values
			}
		}
	}

	if email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Email not provided in SAML assertion"})
		return
	}

	// Find or create user (JIT provisioning)
	authResult := &sso.AuthResult{
		Email:      email,
		Name:       name,
		ExternalID: externalID,
		Groups:     groups,
		ProviderID: providerID,
	}

	jwtToken, err := h.provisionUserAndCreateSession(c.Request.Context(), provider, authResult)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Redirect with token
	redirectURL := fmt.Sprintf("%s?token=%s", returnTo, jwtToken)
	c.Redirect(http.StatusFound, redirectURL)
}

func (h *Handler) getProvider(ctx context.Context, providerID string) (*sso.Provider, error) {
	var p sso.Provider
	err := h.db.QueryRow(ctx, `
		SELECT id, org_id, name, provider_type, enabled,
			   saml_entity_id, saml_sso_url, saml_slo_url, saml_certificate, saml_sign_requests, saml_name_id_format,
			   default_role, auto_create_users
		FROM sso_providers
		WHERE id = $1 AND enabled = true
	`, providerID).Scan(
		&p.ID, &p.OrgID, &p.Name, &p.ProviderType, &p.Enabled,
		&p.SAMLEntityID, &p.SAMLSsoURL, &p.SAMLSloURL, &p.SAMLCertificate, &p.SAMLSignRequests, &p.SAMLNameIDFormat,
		&p.DefaultRole, &p.AutoCreateUsers,
	)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (h *Handler) createServiceProvider(providerID string) *saml.ServiceProvider {
	acsURL, _ := url.Parse(h.baseURL + "/api/v1/auth/saml/acs")
	metadataURL, _ := url.Parse(h.baseURL + "/api/v1/auth/saml/metadata")
	entityID := h.baseURL + "/saml/metadata"

	return &saml.ServiceProvider{
		EntityID:          entityID,
		Key:               h.spKey,
		Certificate:       h.spCert,
		AcsURL:            *acsURL,
		MetadataURL:       *metadataURL,
		AllowIDPInitiated: true,
	}
}

func (h *Handler) createServiceProviderWithIdP(provider *sso.Provider) (*saml.ServiceProvider, error) {
	sp := h.createServiceProvider(provider.ID)

	if provider.SAMLEntityID == nil || provider.SAMLSsoURL == nil {
		return nil, errors.New("SAML entity_id and sso_url are required")
	}

	ssoURL, err := url.Parse(*provider.SAMLSsoURL)
	if err != nil {
		return nil, fmt.Errorf("invalid SSO URL: %w", err)
	}

	// Parse IdP certificate
	var idpCert *x509.Certificate
	if provider.SAMLCertificate != nil && *provider.SAMLCertificate != "" {
		certPEM := *provider.SAMLCertificate
		// Handle both raw and PEM-wrapped certificates
		if !strings.Contains(certPEM, "-----BEGIN") {
			certPEM = "-----BEGIN CERTIFICATE-----\n" + certPEM + "\n-----END CERTIFICATE-----"
		}
		block, _ := pem.Decode([]byte(certPEM))
		if block != nil {
			idpCert, err = x509.ParseCertificate(block.Bytes)
			if err != nil {
				return nil, fmt.Errorf("invalid IdP certificate: %w", err)
			}
		}
	}

	// Configure IdP
	sp.IDPMetadata = &saml.EntityDescriptor{
		EntityID: *provider.SAMLEntityID,
		IDPSSODescriptors: []saml.IDPSSODescriptor{
			{
				SingleSignOnServices: []saml.Endpoint{
					{
						Binding:  saml.HTTPRedirectBinding,
						Location: ssoURL.String(),
					},
					{
						Binding:  saml.HTTPPostBinding,
						Location: ssoURL.String(),
					},
				},
			},
		},
	}

	if idpCert != nil {
		sp.IDPMetadata.IDPSSODescriptors[0].KeyDescriptors = []saml.KeyDescriptor{
			{
				Use: "signing",
				KeyInfo: saml.KeyInfo{
					X509Data: saml.X509Data{
						X509Certificates: []saml.X509Certificate{
							{Data: base64.StdEncoding.EncodeToString(idpCert.Raw)},
						},
					},
				},
			},
		}
	}

	return sp, nil
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

// generateSPKeyPair generates a self-signed certificate for the SP
func generateSPKeyPair() (*rsa.PrivateKey, *x509.Certificate) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "InfraPilot SAML SP",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	certDER, _ := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	cert, _ := x509.ParseCertificate(certDER)

	return key, cert
}
