package ssl

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge"
	"github.com/go-acme/lego/v4/challenge/dns01"
	"github.com/go-acme/lego/v4/challenge/http01"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/providers/http/webroot"
	"github.com/go-acme/lego/v4/registration"
	"go.uber.org/zap"
)

// CertManager handles SSL certificate operations with Let's Encrypt
type CertManager struct {
	CertDir     string // Base directory for certificates (e.g., /etc/letsencrypt)
	WebRoot     string // Webroot directory for ACME HTTP-01 challenge (e.g., /var/www/acme-challenge)
	Email       string // Email for Let's Encrypt account
	Staging     bool   // Use staging server for testing
	logger      *zap.Logger
	accountFile string

	// DNS-01 challenge state
	dnsChallenges     map[string]*DNSChallenge
	dnsChallengesMu   sync.RWMutex
	pendingDNSCleanup map[string]func() error
}

// DNSChallenge represents a pending DNS-01 challenge
type DNSChallenge struct {
	Domain     string            `json:"domain"`
	Token      string            `json:"token"`
	KeyAuth    string            `json:"key_auth"`
	TXTRecord  string            `json:"txt_record"`   // Primary TXT record (for backward compat)
	TXTName    string            `json:"txt_name"`     // Primary TXT name
	TXTRecords []DNSTXTRecord    `json:"txt_records"`  // All TXT records needed (for wildcard)
	CreatedAt  time.Time         `json:"created_at"`
	Verified   bool              `json:"verified"`

	// Internal state for managing the ACME order flow
	proceedCh  chan struct{}     `json:"-"` // Signal to proceed with verification
	resultCh   chan error        `json:"-"` // Result of the ACME operation
	cancelCh   chan struct{}     `json:"-"` // Cancel the operation
}

// DNSTXTRecord represents a single TXT record needed for DNS challenge
type DNSTXTRecord struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// NewCertManager creates a new certificate manager
func NewCertManager(certDir, webRoot, email string, staging bool, logger *zap.Logger) *CertManager {
	return &CertManager{
		CertDir:           certDir,
		WebRoot:           webRoot,
		Email:             email,
		Staging:           staging,
		logger:            logger,
		accountFile:       filepath.Join(certDir, "account.json"),
		dnsChallenges:     make(map[string]*DNSChallenge),
		pendingDNSCleanup: make(map[string]func() error),
	}
}

// User implements the lego registration.User interface
type User struct {
	Email        string
	Registration *registration.Resource
	key          crypto.PrivateKey
}

func (u *User) GetEmail() string                        { return u.Email }
func (u *User) GetRegistration() *registration.Resource { return u.Registration }
func (u *User) GetPrivateKey() crypto.PrivateKey        { return u.key }

// AccountData stores the account info on disk
type AccountData struct {
	Email        string                 `json:"email"`
	Registration *registration.Resource `json:"registration"`
	KeyPEM       string                 `json:"key_pem"`
}

// RequestCertificate requests a new SSL certificate for the domain
func (m *CertManager) RequestCertificate(domain string) error {
	m.logger.Info("Requesting SSL certificate",
		zap.String("domain", domain),
		zap.Bool("staging", m.Staging),
	)

	// Get or create user
	user, err := m.getOrCreateUser()
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	// Create lego config
	config := lego.NewConfig(user)
	if m.Staging {
		config.CADirURL = lego.LEDirectoryStaging
	} else {
		config.CADirURL = lego.LEDirectoryProduction
	}
	config.Certificate.KeyType = certcrypto.RSA2048

	// Create client
	client, err := lego.NewClient(config)
	if err != nil {
		return fmt.Errorf("failed to create ACME client: %w", err)
	}

	// Use HTTP-01 challenge with webroot
	// Nginx will serve the challenge files from the webroot directory
	if m.WebRoot != "" {
		// Ensure webroot directory exists
		if err := os.MkdirAll(m.WebRoot, 0755); err != nil {
			return fmt.Errorf("failed to create webroot directory: %w", err)
		}

		provider, err := webroot.NewHTTPProvider(m.WebRoot)
		if err != nil {
			return fmt.Errorf("failed to create webroot provider: %w", err)
		}
		err = client.Challenge.SetHTTP01Provider(provider)
		if err != nil {
			return fmt.Errorf("failed to set HTTP-01 provider: %w", err)
		}
	} else {
		// Fallback to standalone server (for backwards compatibility)
		err = client.Challenge.SetHTTP01Provider(http01.NewProviderServer("", "80"))
		if err != nil {
			return fmt.Errorf("failed to set HTTP-01 provider: %w", err)
		}
	}

	// Register if needed
	if user.Registration == nil {
		reg, err := client.Registration.Register(registration.RegisterOptions{
			TermsOfServiceAgreed: true,
		})
		if err != nil {
			return fmt.Errorf("failed to register: %w", err)
		}
		user.Registration = reg

		// Save account
		if err := m.saveAccount(user); err != nil {
			m.logger.Warn("Failed to save account", zap.Error(err))
		}
	}

	// Request certificate
	request := certificate.ObtainRequest{
		Domains: []string{domain},
		Bundle:  true,
	}

	certificates, err := client.Certificate.Obtain(request)
	if err != nil {
		return fmt.Errorf("failed to obtain certificate: %w", err)
	}

	// Save certificates
	if err := m.saveCertificate(domain, certificates); err != nil {
		return fmt.Errorf("failed to save certificate: %w", err)
	}

	m.logger.Info("SSL certificate obtained successfully",
		zap.String("domain", domain),
	)

	return nil
}

// RequestCertificateMulti requests a new SSL certificate for multiple domains (SAN certificate)
// The first domain in the list is the primary domain and determines the certificate filename
func (m *CertManager) RequestCertificateMulti(domains []string) error {
	if len(domains) == 0 {
		return fmt.Errorf("at least one domain is required")
	}

	primaryDomain := domains[0]

	m.logger.Info("Requesting SSL certificate for multiple domains",
		zap.Strings("domains", domains),
		zap.String("primary", primaryDomain),
		zap.Bool("staging", m.Staging),
	)

	// Get or create user
	user, err := m.getOrCreateUser()
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	// Create lego config
	config := lego.NewConfig(user)
	if m.Staging {
		config.CADirURL = lego.LEDirectoryStaging
	} else {
		config.CADirURL = lego.LEDirectoryProduction
	}
	config.Certificate.KeyType = certcrypto.RSA2048

	// Create client
	client, err := lego.NewClient(config)
	if err != nil {
		return fmt.Errorf("failed to create ACME client: %w", err)
	}

	// Use HTTP-01 challenge with webroot
	if m.WebRoot != "" {
		if err := os.MkdirAll(m.WebRoot, 0755); err != nil {
			return fmt.Errorf("failed to create webroot directory: %w", err)
		}

		provider, err := webroot.NewHTTPProvider(m.WebRoot)
		if err != nil {
			return fmt.Errorf("failed to create webroot provider: %w", err)
		}
		err = client.Challenge.SetHTTP01Provider(provider)
		if err != nil {
			return fmt.Errorf("failed to set HTTP-01 provider: %w", err)
		}
	} else {
		err = client.Challenge.SetHTTP01Provider(http01.NewProviderServer("", "80"))
		if err != nil {
			return fmt.Errorf("failed to set HTTP-01 provider: %w", err)
		}
	}

	// Register if needed
	if user.Registration == nil {
		reg, err := client.Registration.Register(registration.RegisterOptions{
			TermsOfServiceAgreed: true,
		})
		if err != nil {
			return fmt.Errorf("failed to register: %w", err)
		}
		user.Registration = reg

		if err := m.saveAccount(user); err != nil {
			m.logger.Warn("Failed to save account", zap.Error(err))
		}
	}

	// Request certificate for all domains
	request := certificate.ObtainRequest{
		Domains: domains,
		Bundle:  true,
	}

	certificates, err := client.Certificate.Obtain(request)
	if err != nil {
		return fmt.Errorf("failed to obtain certificate: %w", err)
	}

	// Save certificates using the primary domain as filename
	if err := m.saveCertificate(primaryDomain, certificates); err != nil {
		return fmt.Errorf("failed to save certificate: %w", err)
	}

	m.logger.Info("SSL certificate obtained successfully for multiple domains",
		zap.Strings("domains", domains),
		zap.String("primary", primaryDomain),
	)

	return nil
}

// RenewCertificate renews an existing certificate
func (m *CertManager) RenewCertificate(domain string) error {
	m.logger.Info("Renewing SSL certificate", zap.String("domain", domain))

	// Check if certificate exists
	certPath := m.getCertPath(domain)
	if _, err := os.Stat(filepath.Join(certPath, "fullchain.pem")); os.IsNotExist(err) {
		return fmt.Errorf("certificate does not exist for domain: %s", domain)
	}

	// For renewal, we just request a new certificate
	return m.RequestCertificate(domain)
}

// GetCertificateExpiry returns the expiry date of a certificate
func (m *CertManager) GetCertificateExpiry(domain string) (time.Time, error) {
	certPath := filepath.Join(m.getCertPath(domain), "fullchain.pem")

	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to read certificate: %w", err)
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		return time.Time{}, fmt.Errorf("failed to decode PEM block")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse certificate: %w", err)
	}

	return cert.NotAfter, nil
}

// NeedsRenewal checks if a certificate needs renewal (within 30 days of expiry)
func (m *CertManager) NeedsRenewal(domain string, daysBeforeExpiry int) (bool, error) {
	expiry, err := m.GetCertificateExpiry(domain)
	if err != nil {
		return false, err
	}

	renewalTime := expiry.AddDate(0, 0, -daysBeforeExpiry)
	return time.Now().After(renewalTime), nil
}

// CertificateExists checks if a certificate exists for a domain
func (m *CertManager) CertificateExists(domain string) bool {
	certPath := filepath.Join(m.getCertPath(domain), "fullchain.pem")
	_, err := os.Stat(certPath)
	return err == nil
}

// getCertPath returns the path for a domain's certificates. Wildcards get their
// own "_wildcard.<base>" folder: saving *.example.com under example.com (as this
// used to) overwrote the apex cert, and the apex's next HTTP-01 renewal then
// overwrote the wildcard. Underscores can't appear in hostnames, so no collision.
func (m *CertManager) getCertPath(domain string) string {
	if strings.HasPrefix(domain, "*.") {
		return filepath.Join(m.CertDir, "live", "_wildcard."+strings.TrimPrefix(domain, "*."))
	}
	return filepath.Join(m.CertDir, "live", domain)
}

// saveCertificate saves the certificate and key to disk
func (m *CertManager) saveCertificate(domain string, cert *certificate.Resource) error {
	certPath := m.getCertPath(domain)

	// Create directory
	if err := os.MkdirAll(certPath, 0700); err != nil {
		return fmt.Errorf("failed to create cert directory: %w", err)
	}

	// Save certificate chain
	fullchainPath := filepath.Join(certPath, "fullchain.pem")
	if err := os.WriteFile(fullchainPath, cert.Certificate, 0644); err != nil {
		return fmt.Errorf("failed to write certificate: %w", err)
	}

	// Save private key
	privkeyPath := filepath.Join(certPath, "privkey.pem")
	if err := os.WriteFile(privkeyPath, cert.PrivateKey, 0600); err != nil {
		return fmt.Errorf("failed to write private key: %w", err)
	}

	// Also save separate cert file (without chain)
	certOnlyPath := filepath.Join(certPath, "cert.pem")
	if err := os.WriteFile(certOnlyPath, cert.Certificate, 0644); err != nil {
		return fmt.Errorf("failed to write cert: %w", err)
	}

	m.logger.Info("Saved certificate",
		zap.String("domain", domain),
		zap.String("path", certPath),
	)

	return nil
}

// getOrCreateUser gets or creates the ACME user
func (m *CertManager) getOrCreateUser() (*User, error) {
	// Try to load existing account
	user, err := m.loadAccount()
	if err == nil {
		return user, nil
	}

	m.logger.Info("Creating new ACME account", zap.String("email", m.Email))

	// Generate new private key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	return &User{
		Email: m.Email,
		key:   privateKey,
	}, nil
}

// loadAccount loads the ACME account from disk
func (m *CertManager) loadAccount() (*User, error) {
	data, err := os.ReadFile(m.accountFile)
	if err != nil {
		return nil, err
	}

	var account AccountData
	if err := json.Unmarshal(data, &account); err != nil {
		return nil, err
	}

	// Parse private key
	block, _ := pem.Decode([]byte(account.KeyPEM))
	if block == nil {
		return nil, fmt.Errorf("failed to decode key PEM")
	}

	key, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	return &User{
		Email:        account.Email,
		Registration: account.Registration,
		key:          key,
	}, nil
}

// saveAccount saves the ACME account to disk
func (m *CertManager) saveAccount(user *User) error {
	// Encode private key
	keyBytes, err := x509.MarshalECPrivateKey(user.key.(*ecdsa.PrivateKey))
	if err != nil {
		return fmt.Errorf("failed to marshal private key: %w", err)
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyBytes,
	})

	account := AccountData{
		Email:        user.Email,
		Registration: user.Registration,
		KeyPEM:       string(keyPEM),
	}

	data, err := json.MarshalIndent(account, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal account: %w", err)
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(m.accountFile), 0700); err != nil {
		return fmt.Errorf("failed to create account directory: %w", err)
	}

	if err := os.WriteFile(m.accountFile, data, 0600); err != nil {
		return fmt.Errorf("failed to write account file: %w", err)
	}

	m.logger.Info("Saved ACME account", zap.String("email", user.Email))
	return nil
}

// CertificateInfo contains information about a certificate
type CertificateInfo struct {
	Domain    string    `json:"domain"`
	Exists    bool      `json:"exists"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	DaysLeft  int       `json:"days_left,omitempty"`
	Issuer    string    `json:"issuer,omitempty"`
	IsStaging bool      `json:"is_staging,omitempty"`
}

// GetCertificateInfo retrieves information about a domain's certificate
func (m *CertManager) GetCertificateInfo(domain string) (*CertificateInfo, error) {
	info := &CertificateInfo{
		Domain: domain,
		Exists: false,
	}

	if !m.CertificateExists(domain) {
		return info, nil
	}

	info.Exists = true

	// Read certificate
	certPath := filepath.Join(m.getCertPath(domain), "fullchain.pem")
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return info, fmt.Errorf("failed to read certificate: %w", err)
	}

	// Parse certificate
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return info, fmt.Errorf("failed to decode certificate PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return info, fmt.Errorf("failed to parse certificate: %w", err)
	}

	info.ExpiresAt = cert.NotAfter
	info.DaysLeft = int(time.Until(cert.NotAfter).Hours() / 24)
	info.Issuer = cert.Issuer.CommonName

	// Check if it's a staging certificate
	if cert.Issuer.Organization != nil && len(cert.Issuer.Organization) > 0 {
		for _, org := range cert.Issuer.Organization {
			if org == "(STAGING) Let's Encrypt" {
				info.IsStaging = true
				break
			}
		}
	}

	return info, nil
}

// manualDNSProvider implements the challenge.Provider interface for manual DNS-01 challenges
type manualDNSProvider struct {
	manager   *CertManager
	domain    string
	presented bool
}

func (p *manualDNSProvider) Present(domain, token, keyAuth string) error {
	// Calculate the TXT record value
	txtValue := dns01.GetChallengeInfo(domain, keyAuth).Value
	txtName := "_acme-challenge." + dns01.UnFqdn(domain)

	p.manager.logger.Info("DNS-01 challenge presented",
		zap.String("domain", domain),
		zap.String("txt_name", txtName),
		zap.String("txt_value", txtValue),
	)

	// Store the challenge info
	p.manager.dnsChallengesMu.Lock()
	p.manager.dnsChallenges[p.domain] = &DNSChallenge{
		Domain:    p.domain,
		Token:     token,
		KeyAuth:   keyAuth,
		TXTRecord: txtValue,
		TXTName:   txtName,
		CreatedAt: time.Now(),
		Verified:  false,
	}
	p.manager.dnsChallengesMu.Unlock()

	p.presented = true
	return nil
}

func (p *manualDNSProvider) CleanUp(domain, token, keyAuth string) error {
	p.manager.logger.Info("DNS-01 challenge cleanup",
		zap.String("domain", domain),
	)

	// Remove challenge from map
	p.manager.dnsChallengesMu.Lock()
	delete(p.manager.dnsChallenges, p.domain)
	p.manager.dnsChallengesMu.Unlock()

	return nil
}

// Ensure manualDNSProvider implements the interface
var _ challenge.Provider = (*manualDNSProvider)(nil)

// multiCaptureDNSProvider captures ALL DNS challenge info and waits for user to signal proceed
type multiCaptureDNSProvider struct {
	manager        *CertManager
	primaryDomain  string
	txtRecords     []DNSTXTRecord
	txtRecordsMu   sync.Mutex
	fqdnToValues   map[string][]string // Map fqdn to ALL expected TXT values (for wildcards, same FQDN has multiple values)
	allCapturedCh  chan struct{}       // Closed when all challenges captured
	proceedCh      chan struct{}       // Signal to proceed with verification
	cancelCh       chan struct{}       // Signal to cancel
	expectedCount  int                 // Number of domains to capture
	capturedCount  int
}

func (p *multiCaptureDNSProvider) Present(domain, token, keyAuth string) error {
	// Calculate the TXT record value
	txtValue := dns01.GetChallengeInfo(domain, keyAuth).Value
	txtName := "_acme-challenge." + dns01.UnFqdn(domain)

	// FQDN format used by preCheck (with trailing dot)
	fqdn := dns01.ToFqdn(txtName)

	p.manager.logger.Info("DNS-01 challenge captured",
		zap.String("domain", domain),
		zap.String("txt_name", txtName),
		zap.String("fqdn", fqdn),
		zap.String("txt_value", txtValue),
	)

	// Store this TXT record and the fqdn->values mapping for preCheck
	// For wildcards, the same FQDN may have multiple values (one for *.domain and one for domain)
	p.txtRecordsMu.Lock()
	p.txtRecords = append(p.txtRecords, DNSTXTRecord{Name: txtName, Value: txtValue})
	p.fqdnToValues[fqdn] = append(p.fqdnToValues[fqdn], txtValue) // Append, don't overwrite!
	p.capturedCount++
	allCaptured := p.capturedCount >= p.expectedCount
	p.txtRecordsMu.Unlock()

	// If all challenges captured, signal that we're ready
	if allCaptured {
		close(p.allCapturedCh)
	}

	// DON'T block here - return immediately so ACME can call Present() for other domains
	// The blocking happens in the preCheck function instead
	return nil
}

// preCheck is called before ACME verifies each challenge - this is where we wait for user
func (p *multiCaptureDNSProvider) preCheck(domain, fqdn, value string, check dns01.PreCheckFunc) (bool, error) {
	// Use the ACME-provided value directly - ACME knows exactly what value to verify for this domain
	// The stored values (fqdnToValues) are for showing to the user in the UI
	// For wildcards, ACME calls preCheck separately for each domain with the correct value for that domain
	expectedValue := value

	p.txtRecordsMu.Lock()
	storedValues, hasStored := p.fqdnToValues[fqdn]
	p.txtRecordsMu.Unlock()

	if hasStored {
		p.manager.logger.Info("Pre-check for DNS challenge",
			zap.String("domain", domain),
			zap.String("fqdn", fqdn),
			zap.String("expected_value", expectedValue),
			zap.Strings("all_stored_values", storedValues),
		)
	} else {
		p.manager.logger.Info("Pre-check for DNS challenge (no stored values)",
			zap.String("domain", domain),
			zap.String("fqdn", fqdn),
			zap.String("expected_value", expectedValue),
		)
	}

	p.manager.logger.Info("Pre-check waiting for user to add DNS TXT record...",
		zap.String("domain", domain),
		zap.String("fqdn", fqdn),
		zap.String("expected_value", expectedValue),
	)

	select {
	case <-p.proceedCh:
		// User signaled to proceed - verify DNS record exists
		p.manager.logger.Info("Proceed signal received, verifying DNS record",
			zap.String("fqdn", fqdn),
		)
	case <-p.cancelCh:
		return false, fmt.Errorf("challenge cancelled by user")
	case <-time.After(30 * time.Minute):
		return false, fmt.Errorf("timeout waiting for user to complete DNS challenge")
	}

	// Verify DNS TXT record exists (retry a few times)
	maxRetries := 6
	retryInterval := 5 * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		records, err := net.LookupTXT(fqdn)
		if err == nil {
			for _, record := range records {
				if record == expectedValue {
					p.manager.logger.Info("DNS TXT record verified successfully",
						zap.String("fqdn", fqdn),
						zap.Int("attempt", attempt),
					)
					return true, nil
				}
			}
			p.manager.logger.Warn("DNS TXT record found but value mismatch",
				zap.String("fqdn", fqdn),
				zap.String("expected", expectedValue),
				zap.Strings("found", records),
				zap.Int("attempt", attempt),
			)
		} else {
			p.manager.logger.Warn("DNS TXT record lookup failed",
				zap.String("fqdn", fqdn),
				zap.Error(err),
				zap.Int("attempt", attempt),
			)
		}

		if attempt < maxRetries {
			time.Sleep(retryInterval)
		}
	}

	return false, fmt.Errorf("DNS TXT record verification failed: record '%s' with value '%s' not found after %d attempts", fqdn, expectedValue, maxRetries)
}

func (p *multiCaptureDNSProvider) CleanUp(domain, token, keyAuth string) error {
	p.manager.logger.Info("DNS-01 challenge cleanup (manual - user should remove TXT record)",
		zap.String("domain", domain),
	)
	return nil
}

var _ challenge.Provider = (*multiCaptureDNSProvider)(nil)

// StartDNSChallenge initializes a DNS-01 challenge and returns ALL TXT record info
// For wildcard certificates, this returns multiple TXT records that all need to be added
func (m *CertManager) StartDNSChallenge(domain string) (*DNSChallenge, error) {
	m.logger.Info("Starting DNS-01 challenge",
		zap.String("domain", domain),
		zap.Bool("staging", m.Staging),
	)

	// Cancel any existing challenge for this domain
	m.dnsChallengesMu.Lock()
	if existingChallenge, exists := m.dnsChallenges[domain]; exists && existingChallenge.cancelCh != nil {
		close(existingChallenge.cancelCh)
	}
	m.dnsChallengesMu.Unlock()

	// Get or create user
	user, err := m.getOrCreateUser()
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// Create lego config
	config := lego.NewConfig(user)
	if m.Staging {
		config.CADirURL = lego.LEDirectoryStaging
	} else {
		config.CADirURL = lego.LEDirectoryProduction
	}
	config.Certificate.KeyType = certcrypto.RSA2048

	// Create client
	client, err := lego.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create ACME client: %w", err)
	}

	// Register if needed
	if user.Registration == nil {
		reg, err := client.Registration.Register(registration.RegisterOptions{
			TermsOfServiceAgreed: true,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to register: %w", err)
		}
		user.Registration = reg

		if err := m.saveAccount(user); err != nil {
			m.logger.Warn("Failed to save account", zap.Error(err))
		}
	}

	// For wildcard certificates, include both the wildcard and base domain
	domains := []string{domain}
	expectedCount := 1
	if strings.HasPrefix(domain, "*.") {
		baseDomain := strings.TrimPrefix(domain, "*.")
		domains = []string{domain, baseDomain}
		expectedCount = 2
	}

	// Create the multi-capture provider
	provider := &multiCaptureDNSProvider{
		manager:       m,
		primaryDomain: domain,
		txtRecords:    make([]DNSTXTRecord, 0, expectedCount),
		fqdnToValues:  make(map[string][]string),
		allCapturedCh: make(chan struct{}),
		proceedCh:     make(chan struct{}),
		cancelCh:      make(chan struct{}),
		expectedCount: expectedCount,
	}

	err = client.Challenge.SetDNS01Provider(provider,
		dns01.AddRecursiveNameservers([]string{"8.8.8.8:53", "1.1.1.1:53"}),
		dns01.WrapPreCheck(provider.preCheck),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to set DNS-01 provider: %w", err)
	}

	// Create result channel for the goroutine
	resultCh := make(chan error, 1)

	// Start the obtain process in a goroutine - this will call Present() for each domain
	go func() {
		request := certificate.ObtainRequest{
			Domains: domains,
			Bundle:  true,
		}
		certificates, obtainErr := client.Certificate.Obtain(request)
		if obtainErr != nil {
			m.logger.Error("Certificate obtain failed",
				zap.String("domain", domain),
				zap.Error(obtainErr),
			)
			resultCh <- obtainErr
			return
		}

		// Save certificates (wildcards go to their own folder, see getCertPath)
		if err := m.saveCertificate(domain, certificates); err != nil {
			resultCh <- fmt.Errorf("failed to save certificate: %w", err)
			return
		}

		m.logger.Info("SSL certificate obtained successfully with DNS-01",
			zap.String("domain", domain),
			zap.Strings("domains", domains),
		)
		resultCh <- nil
	}()

	// Wait for challenges to be captured
	// Note: ACME may skip some domains if authorizations are already valid from previous attempts
	// So we wait for either all challenges OR at least one with a shorter secondary timeout
	select {
	case <-provider.allCapturedCh:
		m.logger.Info("All DNS challenges captured",
			zap.String("domain", domain),
			zap.Int("count", len(provider.txtRecords)),
		)
	case <-time.After(10 * time.Second):
		// After 10 seconds, check if we have at least one challenge
		provider.txtRecordsMu.Lock()
		count := len(provider.txtRecords)
		provider.txtRecordsMu.Unlock()

		if count > 0 {
			m.logger.Info("Got partial challenges (some may be pre-authorized)",
				zap.String("domain", domain),
				zap.Int("count", count),
				zap.Int("expected", expectedCount),
			)
		} else {
			// No challenges captured at all - wait a bit more
			select {
			case <-provider.allCapturedCh:
				m.logger.Info("All DNS challenges captured (delayed)",
					zap.String("domain", domain),
					zap.Int("count", len(provider.txtRecords)),
				)
			case <-time.After(20 * time.Second):
				provider.txtRecordsMu.Lock()
				count = len(provider.txtRecords)
				provider.txtRecordsMu.Unlock()
				if count == 0 {
					close(provider.cancelCh)
					return nil, fmt.Errorf("timeout waiting for ACME challenges to be captured")
				}
				m.logger.Info("Got challenges after extended wait",
					zap.String("domain", domain),
					zap.Int("count", count),
				)
			}
		}
	}

	// Calculate primary TXT name
	baseDomain := domain
	if strings.HasPrefix(domain, "*.") {
		baseDomain = strings.TrimPrefix(domain, "*.")
	}
	txtName := "_acme-challenge." + baseDomain

	// Get the first TXT record value for backward compatibility
	txtRecord := ""
	if len(provider.txtRecords) > 0 {
		txtRecord = provider.txtRecords[0].Value
	}

	// Create and store the challenge with all TXT records
	challenge := &DNSChallenge{
		Domain:     domain,
		TXTName:    txtName,
		TXTRecord:  txtRecord,
		TXTRecords: provider.txtRecords,
		CreatedAt:  time.Now(),
		Verified:   false,
		proceedCh:  provider.proceedCh,
		resultCh:   resultCh,
		cancelCh:   provider.cancelCh,
	}

	m.dnsChallengesMu.Lock()
	m.dnsChallenges[domain] = challenge
	m.dnsChallengesMu.Unlock()

	return challenge, nil
}

// GetDNSChallenge returns the current DNS challenge for a domain
func (m *CertManager) GetDNSChallenge(domain string) (*DNSChallenge, error) {
	m.dnsChallengesMu.RLock()
	defer m.dnsChallengesMu.RUnlock()

	challenge, exists := m.dnsChallenges[domain]
	if !exists {
		return nil, fmt.Errorf("no pending DNS challenge for domain: %s", domain)
	}

	return challenge, nil
}

// RequestCertificateWithDNS signals the pending DNS challenge to proceed and waits for the result
// This should be called after the user has added the TXT records to their DNS
func (m *CertManager) RequestCertificateWithDNS(domain string) error {
	m.logger.Info("Completing DNS-01 challenge",
		zap.String("domain", domain),
	)

	// Get the stored challenge
	m.dnsChallengesMu.RLock()
	challenge, exists := m.dnsChallenges[domain]
	m.dnsChallengesMu.RUnlock()

	if !exists {
		return fmt.Errorf("no pending DNS challenge for domain: %s - please start a DNS challenge first", domain)
	}

	if challenge.proceedCh == nil || challenge.resultCh == nil {
		return fmt.Errorf("DNS challenge state is invalid - please start a new DNS challenge")
	}

	// Signal the waiting goroutine to proceed with verification
	m.logger.Info("Signaling DNS challenge to proceed",
		zap.String("domain", domain),
		zap.Int("txt_records", len(challenge.TXTRecords)),
	)

	// Close proceedCh to signal all waiting Present() calls to proceed
	close(challenge.proceedCh)

	// Wait for the result with a timeout
	select {
	case err := <-challenge.resultCh:
		// Clean up challenge state
		m.dnsChallengesMu.Lock()
		delete(m.dnsChallenges, domain)
		m.dnsChallengesMu.Unlock()

		if err != nil {
			return fmt.Errorf("DNS challenge failed: %w", err)
		}
		return nil
	case <-time.After(5 * time.Minute):
		// Cancel the challenge
		if challenge.cancelCh != nil {
			close(challenge.cancelCh)
		}
		m.dnsChallengesMu.Lock()
		delete(m.dnsChallenges, domain)
		m.dnsChallengesMu.Unlock()
		return fmt.Errorf("timeout waiting for DNS challenge to complete")
	}
}

// presetDNSProvider is a DNS provider for when TXT records are already in place
type presetDNSProvider struct {
	manager *CertManager
	domain  string
}

func (p *presetDNSProvider) Present(domain, token, keyAuth string) error {
	txtValue := dns01.GetChallengeInfo(domain, keyAuth).Value
	txtName := "_acme-challenge." + dns01.UnFqdn(domain)

	p.manager.logger.Info("DNS-01 verifying preset TXT record before ACME submission",
		zap.String("domain", domain),
		zap.String("txt_name", txtName),
		zap.String("txt_value", txtValue),
	)

	// Verify DNS TXT record exists before submitting to ACME
	// Retry a few times with short intervals since record should already be in place
	maxRetries := 6
	retryInterval := 5 * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		records, err := net.LookupTXT(txtName)
		if err == nil {
			for _, record := range records {
				if record == txtValue {
					p.manager.logger.Info("DNS TXT record verified successfully",
						zap.String("txt_name", txtName),
						zap.Int("attempt", attempt),
					)
					return nil
				}
			}
			p.manager.logger.Warn("DNS TXT record found but value mismatch",
				zap.String("txt_name", txtName),
				zap.String("expected", txtValue),
				zap.Strings("found", records),
				zap.Int("attempt", attempt),
			)
		} else {
			p.manager.logger.Warn("DNS TXT record lookup failed",
				zap.String("txt_name", txtName),
				zap.Error(err),
				zap.Int("attempt", attempt),
			)
		}

		if attempt < maxRetries {
			p.manager.logger.Debug("Retrying DNS verification...",
				zap.Int("attempt", attempt),
				zap.Int("max_retries", maxRetries),
			)
			time.Sleep(retryInterval)
		}
	}

	return fmt.Errorf("DNS TXT record verification failed: record '%s' with value '%s' not found after %d attempts - please ensure the DNS record is properly configured and has propagated", txtName, txtValue, maxRetries)
}

func (p *presetDNSProvider) CleanUp(domain, token, keyAuth string) error {
	p.manager.logger.Info("DNS-01 cleanup (manual - user should remove TXT record)",
		zap.String("domain", domain),
	)
	return nil
}

var _ challenge.Provider = (*presetDNSProvider)(nil)
