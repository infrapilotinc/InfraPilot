package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/pem"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// License represents the license structure
type License struct {
	ID           string          `yaml:"id"`
	Edition      string          `yaml:"edition"`
	Organization string          `yaml:"organization"`
	OrgID        string          `yaml:"org_id"`
	Features     map[string]bool `yaml:"features"`
	Limits       Limits          `yaml:"limits"`
	IssuedAt     time.Time       `yaml:"issued_at"`
	ExpiresAt    time.Time       `yaml:"expires_at"`
	Signature    string          `yaml:"signature"`
}

// Limits defines usage limits
type Limits struct {
	MaxUsers     int `yaml:"max_users"`
	MaxAgents    int `yaml:"max_agents"`
	MaxResources int `yaml:"max_resources"`
}

// LicenseFile wraps the license for YAML output
type LicenseFile struct {
	License License `yaml:"license"`
}

// Available enterprise features
var allFeatures = []string{
	"sso_saml",
	"sso_oidc",
	"sso_ldap",
	"multi_tenant",
	"audit_unlimited",
	"audit_export",
	"advanced_rbac",
	"compliance_reports",
	"ha_clustering",
	"priority_support",
}

func main() {
	// Commands
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "keygen":
		keygenCmd()
	case "create":
		createCmd()
	case "verify":
		verifyCmd()
	case "help":
		printUsage()
	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`InfraPilot License Generator

Usage:
  license-gen <command> [options]

Commands:
  keygen    Generate a new Ed25519 key pair for license signing
  create    Create and sign a new license
  verify    Verify a license file
  help      Show this help message

Examples:
  # Generate key pair
  license-gen keygen -out ./keys

  # Create enterprise license
  license-gen create \
    -key ./keys/private.pem \
    -org "Acme Corp" \
    -org-id "org_12345" \
    -edition enterprise \
    -features all \
    -max-users 100 \
    -max-agents 50 \
    -expires 365 \
    -out license.yaml

  # Verify license
  license-gen verify -key ./keys/public.pem -license license.yaml
`)
}

func keygenCmd() {
	fs := flag.NewFlagSet("keygen", flag.ExitOnError)
	outDir := fs.String("out", ".", "Output directory for key files")
	fs.Parse(os.Args[2:])

	// Generate key pair
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		fmt.Printf("Error generating key pair: %v\n", err)
		os.Exit(1)
	}

	// Save private key
	privPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "ED25519 PRIVATE KEY",
		Bytes: privKey,
	})
	privPath := *outDir + "/private.pem"
	if err := os.WriteFile(privPath, privPEM, 0600); err != nil {
		fmt.Printf("Error writing private key: %v\n", err)
		os.Exit(1)
	}

	// Save public key
	pubPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "ED25519 PUBLIC KEY",
		Bytes: pubKey,
	})
	pubPath := *outDir + "/public.pem"
	if err := os.WriteFile(pubPath, pubPEM, 0644); err != nil {
		fmt.Printf("Error writing public key: %v\n", err)
		os.Exit(1)
	}

	// Also output the public key as a Go variable for embedding
	pubBase64 := base64.StdEncoding.EncodeToString(pubKey)
	goPath := *outDir + "/pubkey.go"
	goContent := fmt.Sprintf(`package license

// EmbeddedPublicKey is the public key for license validation
// This file is auto-generated. Do not edit.
var EmbeddedPublicKey = "%s"
`, pubBase64)
	if err := os.WriteFile(goPath, []byte(goContent), 0644); err != nil {
		fmt.Printf("Error writing Go file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Key pair generated:\n")
	fmt.Printf("  Private key: %s (keep secret!)\n", privPath)
	fmt.Printf("  Public key:  %s\n", pubPath)
	fmt.Printf("  Go embed:    %s\n", goPath)
	fmt.Printf("\nPublic key (base64): %s\n", pubBase64)
}

func createCmd() {
	fs := flag.NewFlagSet("create", flag.ExitOnError)
	keyPath := fs.String("key", "", "Path to private key PEM file")
	org := fs.String("org", "", "Organization name")
	orgID := fs.String("org-id", "", "Organization ID")
	edition := fs.String("edition", "enterprise", "License edition (community/enterprise)")
	features := fs.String("features", "all", "Comma-separated features or 'all'")
	maxUsers := fs.Int("max-users", -1, "Max users (-1 for unlimited)")
	maxAgents := fs.Int("max-agents", -1, "Max agents (-1 for unlimited)")
	maxResources := fs.Int("max-resources", -1, "Max resources (-1 for unlimited)")
	expiresDays := fs.Int("expires", 365, "Days until expiration")
	outPath := fs.String("out", "license.yaml", "Output license file path")
	fs.Parse(os.Args[2:])

	if *keyPath == "" {
		fmt.Println("Error: -key is required")
		os.Exit(1)
	}
	if *org == "" {
		fmt.Println("Error: -org is required")
		os.Exit(1)
	}

	// Load private key
	keyData, err := os.ReadFile(*keyPath)
	if err != nil {
		fmt.Printf("Error reading private key: %v\n", err)
		os.Exit(1)
	}

	block, _ := pem.Decode(keyData)
	if block == nil {
		fmt.Println("Error: invalid PEM file")
		os.Exit(1)
	}

	privKey := ed25519.PrivateKey(block.Bytes)
	if len(privKey) != ed25519.PrivateKeySize {
		fmt.Println("Error: invalid private key size")
		os.Exit(1)
	}

	// Parse features
	featureMap := make(map[string]bool)
	if *features == "all" {
		for _, f := range allFeatures {
			featureMap[f] = true
		}
	} else {
		for _, f := range strings.Split(*features, ",") {
			f = strings.TrimSpace(f)
			if f != "" {
				featureMap[f] = true
			}
		}
	}

	// Generate license ID
	idBytes := make([]byte, 8)
	rand.Read(idBytes)
	licenseID := fmt.Sprintf("lic_%s", base64.RawURLEncoding.EncodeToString(idBytes))

	// Generate org ID if not provided
	if *orgID == "" {
		orgBytes := make([]byte, 8)
		rand.Read(orgBytes)
		*orgID = fmt.Sprintf("org_%s", base64.RawURLEncoding.EncodeToString(orgBytes))
	}

	// Create license
	now := time.Now().UTC()
	lic := License{
		ID:           licenseID,
		Edition:      *edition,
		Organization: *org,
		OrgID:        *orgID,
		Features:     featureMap,
		Limits: Limits{
			MaxUsers:     *maxUsers,
			MaxAgents:    *maxAgents,
			MaxResources: *maxResources,
		},
		IssuedAt:  now,
		ExpiresAt: now.AddDate(0, 0, *expiresDays),
	}

	// Marshal without signature first for signing
	licFile := LicenseFile{License: lic}
	licData, err := yaml.Marshal(licFile)
	if err != nil {
		fmt.Printf("Error marshaling license: %v\n", err)
		os.Exit(1)
	}

	// Sign
	signature := ed25519.Sign(privKey, licData)
	lic.Signature = base64.StdEncoding.EncodeToString(signature)

	// Marshal final license with signature
	licFile.License = lic
	finalData, err := yaml.Marshal(licFile)
	if err != nil {
		fmt.Printf("Error marshaling final license: %v\n", err)
		os.Exit(1)
	}

	// Write output
	if err := os.WriteFile(*outPath, finalData, 0644); err != nil {
		fmt.Printf("Error writing license file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("License created: %s\n", *outPath)
	fmt.Printf("  ID:           %s\n", lic.ID)
	fmt.Printf("  Organization: %s\n", lic.Organization)
	fmt.Printf("  Edition:      %s\n", lic.Edition)
	fmt.Printf("  Features:     %d enabled\n", len(lic.Features))
	fmt.Printf("  Expires:      %s\n", lic.ExpiresAt.Format("2006-01-02"))
}

func verifyCmd() {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	keyPath := fs.String("key", "", "Path to public key PEM file")
	licensePath := fs.String("license", "", "Path to license YAML file")
	fs.Parse(os.Args[2:])

	if *keyPath == "" || *licensePath == "" {
		fmt.Println("Error: -key and -license are required")
		os.Exit(1)
	}

	// Load public key
	keyData, err := os.ReadFile(*keyPath)
	if err != nil {
		fmt.Printf("Error reading public key: %v\n", err)
		os.Exit(1)
	}

	block, _ := pem.Decode(keyData)
	if block == nil {
		fmt.Println("Error: invalid PEM file")
		os.Exit(1)
	}

	pubKey := ed25519.PublicKey(block.Bytes)
	if len(pubKey) != ed25519.PublicKeySize {
		fmt.Println("Error: invalid public key size")
		os.Exit(1)
	}

	// Load license
	licData, err := os.ReadFile(*licensePath)
	if err != nil {
		fmt.Printf("Error reading license: %v\n", err)
		os.Exit(1)
	}

	var licFile LicenseFile
	if err := yaml.Unmarshal(licData, &licFile); err != nil {
		fmt.Printf("Error parsing license: %v\n", err)
		os.Exit(1)
	}

	lic := licFile.License

	// Verify signature
	// First, recreate the license without signature
	origSig := lic.Signature
	lic.Signature = ""
	licFile.License = lic
	unsignedData, _ := yaml.Marshal(licFile)

	sigBytes, err := base64.StdEncoding.DecodeString(origSig)
	if err != nil {
		fmt.Printf("Error decoding signature: %v\n", err)
		os.Exit(1)
	}

	if !ed25519.Verify(pubKey, unsignedData, sigBytes) {
		fmt.Println("INVALID: Signature verification failed")
		os.Exit(1)
	}

	// Check expiration
	expired := !lic.ExpiresAt.IsZero() && time.Now().After(lic.ExpiresAt)

	fmt.Println("License verification:")
	fmt.Printf("  Signature:    VALID\n")
	fmt.Printf("  ID:           %s\n", lic.ID)
	fmt.Printf("  Organization: %s\n", lic.Organization)
	fmt.Printf("  Edition:      %s\n", lic.Edition)
	fmt.Printf("  Features:     %d enabled\n", len(lic.Features))
	fmt.Printf("  Issued:       %s\n", lic.IssuedAt.Format("2006-01-02"))
	fmt.Printf("  Expires:      %s\n", lic.ExpiresAt.Format("2006-01-02"))
	if expired {
		fmt.Printf("  Status:       EXPIRED\n")
		os.Exit(1)
	}
	fmt.Printf("  Status:       ACTIVE\n")
}
