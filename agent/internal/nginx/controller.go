package nginx

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html/template"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"go.uber.org/zap"
)

// Controller manages nginx configuration via the official nginx container
// It writes configs to a shared volume and controls nginx via docker exec
type Controller struct {
	configPath     string // Path to nginx conf.d (shared volume)
	containerName  string // Name of the nginx container
	dockerClient   *client.Client
	logger         *zap.Logger
}

// ProxyConfig represents a reverse proxy configuration
type ProxyConfig struct {
	Domain          string
	Upstream        string
	SSLEnabled      bool
	SSLCertPath     string
	SSLKeyPath      string
	ForceSSL        bool
	HTTP2Enabled    bool
	IncludeWWW      bool
	SecurityHeaders SecurityHeadersConfig
}

// SecurityHeadersConfig represents security headers for a proxy
type SecurityHeadersConfig struct {
	HSTSEnabled           bool
	HSTSMaxAge            int
	XFrameOptions         string
	XContentTypeOptions   bool
	XXSSProtection        bool
	ContentSecurityPolicy string
}

// NewController creates a new nginx controller
func NewController(configPath, containerName string, logger *zap.Logger) (*Controller, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	return &Controller{
		configPath:    configPath,
		containerName: containerName,
		dockerClient:  cli,
		logger:        logger,
	}, nil
}

// Close closes the docker client
func (c *Controller) Close() error {
	return c.dockerClient.Close()
}

// WriteConfig generates and writes an nginx config file for a proxy
func (c *Controller) WriteConfig(ctx context.Context, proxyID string, config ProxyConfig) (string, error) {
	// Generate config from template
	content, err := c.generateConfig(config)
	if err != nil {
		return "", fmt.Errorf("failed to generate config: %w", err)
	}

	// Calculate hash
	hash := sha256.Sum256([]byte(content))
	hashStr := hex.EncodeToString(hash[:])

	// Write to file
	filename := fmt.Sprintf("%s.conf", sanitizeFilename(config.Domain))
	filepath := filepath.Join(c.configPath, filename)

	// Write atomically (temp file then rename)
	tmpPath := filepath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write config: %w", err)
	}

	if err := os.Rename(tmpPath, filepath); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("failed to rename config: %w", err)
	}

	c.logger.Info("Wrote nginx config",
		zap.String("domain", config.Domain),
		zap.String("file", filename),
		zap.String("hash", hashStr[:16]),
	)

	return hashStr, nil
}

// DeleteConfig removes an nginx config file
func (c *Controller) DeleteConfig(ctx context.Context, domain string) error {
	filename := fmt.Sprintf("%s.conf", sanitizeFilename(domain))
	filepath := filepath.Join(c.configPath, filename)

	if err := os.Remove(filepath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete config: %w", err)
	}

	c.logger.Info("Deleted nginx config", zap.String("domain", domain))
	return nil
}

// WriteConfigFile writes raw config content to a file
// This is used when the backend generates the config and sends it via gRPC
func (c *Controller) WriteConfigFile(configPath, content string) error {
	// Use atomic write: write to temp file then rename
	tmpFile := configPath + ".tmp"

	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write temp config: %w", err)
	}

	if err := os.Rename(tmpFile, configPath); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("failed to rename config: %w", err)
	}

	c.logger.Info("Wrote nginx config file", zap.String("path", configPath))
	return nil
}

// WriteHtpasswdFile writes htpasswd content for basic auth
func (c *Controller) WriteHtpasswdFile(htpasswdPath, content string) error {
	// Ensure directory exists
	dir := filepath.Dir(htpasswdPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create htpasswd directory: %w", err)
	}

	// Use atomic write: write to temp file then rename
	tmpFile := htpasswdPath + ".tmp"

	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write temp htpasswd: %w", err)
	}

	if err := os.Rename(tmpFile, htpasswdPath); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("failed to rename htpasswd: %w", err)
	}

	c.logger.Info("Wrote htpasswd file", zap.String("path", htpasswdPath))
	return nil
}

// DeleteHtpasswdFile removes an htpasswd file
func (c *Controller) DeleteHtpasswdFile(htpasswdPath string) error {
	if err := os.Remove(htpasswdPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete htpasswd: %w", err)
	}

	c.logger.Info("Deleted htpasswd file", zap.String("path", htpasswdPath))
	return nil
}

// TestConfig validates the nginx configuration by running nginx -t in the container
func (c *Controller) TestConfig(ctx context.Context) error {
	output, exitCode, err := c.execNginxCommand(ctx, []string{"nginx", "-t"})
	if err != nil {
		return fmt.Errorf("failed to exec nginx -t: %w", err)
	}

	if exitCode != 0 {
		return fmt.Errorf("nginx config test failed: %s", output)
	}

	c.logger.Info("Nginx config test passed")
	return nil
}

// Reload reloads nginx configuration (zero downtime)
func (c *Controller) Reload(ctx context.Context) error {
	// First test the config
	if err := c.TestConfig(ctx); err != nil {
		return err
	}

	output, exitCode, err := c.execNginxCommand(ctx, []string{"nginx", "-s", "reload"})
	if err != nil {
		return fmt.Errorf("failed to exec nginx reload: %w", err)
	}

	if exitCode != 0 {
		return fmt.Errorf("nginx reload failed: %s", output)
	}

	c.logger.Info("Nginx reloaded successfully")
	return nil
}

// execNginxCommand executes a command either locally or via docker exec depending on containerName
func (c *Controller) execNginxCommand(ctx context.Context, cmd []string) (string, int, error) {
	// If containerName is "local", run commands directly (single-container mode)
	if c.containerName == "local" {
		return c.execLocal(ctx, cmd)
	}
	// Otherwise, use docker exec (multi-container mode)
	return c.execInContainer(ctx, cmd)
}

// execLocal executes a command directly on the local system
func (c *Controller) execLocal(ctx context.Context, cmd []string) (string, int, error) {
	if len(cmd) == 0 {
		return "", -1, fmt.Errorf("empty command")
	}

	command := exec.CommandContext(ctx, cmd[0], cmd[1:]...)
	output, err := command.CombinedOutput()

	exitCode := 0
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		} else {
			return string(output), -1, err
		}
	}

	return string(output), exitCode, nil
}

// UpstreamStatus represents the result of upstream validation
type UpstreamStatus struct {
	Reachable   bool
	Host        string
	Port        string
	DNSResolved bool
	IPAddress   string
	Latency     time.Duration
	Error       string
}

// ValidateUpstream checks if the upstream target is reachable
func (c *Controller) ValidateUpstream(ctx context.Context, upstream string) (*UpstreamStatus, error) {
	status := &UpstreamStatus{}

	// Parse the upstream URL
	u, err := url.Parse(upstream)
	if err != nil {
		status.Error = fmt.Sprintf("invalid upstream URL: %v", err)
		return status, fmt.Errorf("invalid upstream URL: %w", err)
	}

	host := u.Hostname()
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}

	status.Host = host
	status.Port = port

	c.logger.Info("Validating upstream",
		zap.String("upstream", upstream),
		zap.String("host", host),
		zap.String("port", port),
	)

	// Step 1: DNS resolution
	ips, err := net.LookupIP(host)
	if err != nil {
		status.Error = fmt.Sprintf("DNS resolution failed for %s: %v", host, err)
		c.logger.Warn("Upstream DNS resolution failed",
			zap.String("host", host),
			zap.Error(err),
		)
		return status, fmt.Errorf("DNS resolution failed for %s: %w", host, err)
	}

	status.DNSResolved = true
	if len(ips) > 0 {
		status.IPAddress = ips[0].String()
	}

	c.logger.Info("Upstream DNS resolved",
		zap.String("host", host),
		zap.String("ip", status.IPAddress),
	)

	// Step 2: TCP connectivity check
	address := net.JoinHostPort(host, port)
	start := time.Now()

	conn, err := net.DialTimeout("tcp", address, 5*time.Second)
	if err != nil {
		status.Error = fmt.Sprintf("connection failed to %s: %v", address, err)
		c.logger.Warn("Upstream connection failed",
			zap.String("address", address),
			zap.Error(err),
		)
		return status, fmt.Errorf("connection failed to %s: %w", address, err)
	}
	conn.Close()

	status.Latency = time.Since(start)
	status.Reachable = true

	c.logger.Info("Upstream validation successful",
		zap.String("upstream", upstream),
		zap.String("ip", status.IPAddress),
		zap.Duration("latency", status.Latency),
	)

	return status, nil
}

// ApplyConfig writes config, validates upstream, validates nginx config, and reloads
func (c *Controller) ApplyConfig(ctx context.Context, proxyID string, config ProxyConfig) (string, error) {
	// Step 1: Validate upstream is reachable
	status, err := c.ValidateUpstream(ctx, config.Upstream)
	if err != nil {
		return "", fmt.Errorf("upstream validation failed: %w", err)
	}

	c.logger.Info("Upstream validated",
		zap.String("domain", config.Domain),
		zap.String("upstream", config.Upstream),
		zap.String("resolved_ip", status.IPAddress),
		zap.Duration("latency", status.Latency),
	)

	// Keep whatever is there now so a failed test can put it back. Deleting it
	// instead would throw away a working config written by someone else.
	configFile := filepath.Join(c.configPath, sanitizeFilename(config.Domain)+".conf")
	previous, readErr := os.ReadFile(configFile)

	// Step 2: Write the config
	hash, err := c.WriteConfig(ctx, proxyID, config)
	if err != nil {
		return "", err
	}

	// Step 3: Test nginx config
	if err := c.TestConfig(ctx); err != nil {
		// Rollback: restore the previous file, or remove ours if there wasn't one
		if readErr == nil {
			if werr := c.WriteConfigFile(configFile, string(previous)); werr != nil {
				c.logger.Error("Failed to restore previous config", zap.String("file", configFile), zap.Error(werr))
			}
		} else {
			c.DeleteConfig(ctx, config.Domain)
		}
		return "", fmt.Errorf("nginx config validation failed, rolled back: %w", err)
	}

	// Step 4: Reload nginx
	if err := c.Reload(ctx); err != nil {
		return "", err
	}

	return hash, nil
}

// execInContainer executes a command inside the nginx container
func (c *Controller) execInContainer(ctx context.Context, cmd []string) (string, int, error) {
	execConfig := container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	}

	execID, err := c.dockerClient.ContainerExecCreate(ctx, c.containerName, execConfig)
	if err != nil {
		return "", -1, fmt.Errorf("failed to create exec: %w", err)
	}

	resp, err := c.dockerClient.ContainerExecAttach(ctx, execID.ID, container.ExecAttachOptions{})
	if err != nil {
		return "", -1, fmt.Errorf("failed to attach exec: %w", err)
	}
	defer resp.Close()

	// Read output
	var buf bytes.Buffer
	buf.ReadFrom(resp.Reader)
	output := buf.String()

	// Get exit code
	inspect, err := c.dockerClient.ContainerExecInspect(ctx, execID.ID)
	if err != nil {
		return output, -1, fmt.Errorf("failed to inspect exec: %w", err)
	}

	return output, inspect.ExitCode, nil
}

// letsEncryptLiveDir is where issued certificates live (see ssl.CertManager).
const letsEncryptLiveDir = "/etc/letsencrypt/live"

// certDirName mirrors ssl.CertManager: wildcards live under _wildcard.<base>.
func certDirName(domain string) string {
	if strings.HasPrefix(domain, "*.") {
		return "_wildcard." + strings.TrimPrefix(domain, "*.")
	}
	return domain
}

// resolveCertPaths picks an existing cert for domain: its own, a wildcard for its
// parent, or the legacy location wildcards used to be saved in (the bare parent).
func resolveCertPaths(domain string) (string, string) {
	candidates := []string{domain}
	if strings.HasPrefix(domain, "*.") {
		candidates = append(candidates, strings.TrimPrefix(domain, "*."))
	} else if parts := strings.Split(domain, "."); len(parts) > 2 {
		parent := strings.Join(parts[1:], ".")
		candidates = append(candidates, "*."+parent, parent)
	}
	for _, d := range candidates {
		dir := filepath.Join(letsEncryptLiveDir, certDirName(d))
		if _, err := os.Stat(filepath.Join(dir, "fullchain.pem")); err == nil {
			return filepath.Join(dir, "fullchain.pem"), filepath.Join(dir, "privkey.pem")
		}
	}
	dir := filepath.Join(letsEncryptLiveDir, certDirName(domain))
	return filepath.Join(dir, "fullchain.pem"), filepath.Join(dir, "privkey.pem")
}

// generateConfig generates nginx config from template
func (c *Controller) generateConfig(config ProxyConfig) (string, error) {
	if config.SSLEnabled && (config.SSLCertPath == "" || config.SSLKeyPath == "") {
		config.SSLCertPath, config.SSLKeyPath = resolveCertPaths(config.Domain)
	}

	tmpl, err := template.New("nginx").Parse(nginxTemplate)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, config); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// sanitizeFilename converts a domain to a safe filename
func sanitizeFilename(domain string) string {
	// Replace dots and special chars with underscores
	safe := strings.ReplaceAll(domain, ".", "_")
	safe = strings.ReplaceAll(safe, "/", "_")
	safe = strings.ReplaceAll(safe, ":", "_")
	return safe
}

// nginxTemplate is the template for generating nginx server blocks
const nginxTemplate = `# Managed by InfraPilot - Do not edit manually
# Domain: {{ .Domain }}

{{ if and .SSLEnabled .ForceSSL }}
# HTTP to HTTPS redirect (with ACME challenge support)
server {
    listen 80;
    listen [::]:80;
{{ if .IncludeWWW }}    server_name {{ .Domain }} www.{{ .Domain }};
{{ else }}    server_name {{ .Domain }};
{{ end }}
    # Per-domain logging for analytics
    access_log /var/log/nginx/domains/{{ .Domain }}.access.log infrapilot_analytics;
    error_log /var/log/nginx/domains/{{ .Domain }}.error.log warn;

    # ACME challenge for Let's Encrypt certificate renewal
    location /.well-known/acme-challenge/ {
        root /var/www/acme-challenge;
        try_files $uri =404;
    }

    location / {
        return 301 https://$host$request_uri;
    }
}
{{ else if not .SSLEnabled }}
# HTTP server
server {
    listen 80;
    listen [::]:80;
{{ if .IncludeWWW }}    server_name {{ .Domain }} www.{{ .Domain }};
{{ else }}    server_name {{ .Domain }};
{{ end }}
    # Per-domain logging for analytics
    access_log /var/log/nginx/domains/{{ .Domain }}.access.log infrapilot_analytics;
    error_log /var/log/nginx/domains/{{ .Domain }}.error.log warn;

    # ACME challenge for Let's Encrypt certificate issuance
    location /.well-known/acme-challenge/ {
        root /var/www/acme-challenge;
        try_files $uri =404;
    }

    location / {
        proxy_pass {{ .Upstream }};
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
    }
}
{{ end }}

{{ if .SSLEnabled }}
# HTTPS server
server {
    listen 443 ssl;
    listen [::]:443 ssl;
{{ if .HTTP2Enabled }}    http2 on;
{{ end }}{{ if .IncludeWWW }}    server_name {{ .Domain }} www.{{ .Domain }};
{{ else }}    server_name {{ .Domain }};
{{ end }}
    # Per-domain logging for analytics
    access_log /var/log/nginx/domains/{{ .Domain }}.access.log infrapilot_analytics;
    error_log /var/log/nginx/domains/{{ .Domain }}.error.log warn;

    ssl_certificate {{ .SSLCertPath }};
    ssl_certificate_key {{ .SSLKeyPath }};

    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_prefer_server_ciphers on;
    ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384;
    ssl_session_cache shared:SSL:10m;
    ssl_session_timeout 1d;
    ssl_session_tickets off;

    # Security headers
{{ if .SecurityHeaders.HSTSEnabled }}
    add_header Strict-Transport-Security "max-age={{ .SecurityHeaders.HSTSMaxAge }}; includeSubDomains" always;
{{ end }}
{{ if .SecurityHeaders.XFrameOptions }}
    add_header X-Frame-Options "{{ .SecurityHeaders.XFrameOptions }}" always;
{{ end }}
{{ if .SecurityHeaders.XContentTypeOptions }}
    add_header X-Content-Type-Options "nosniff" always;
{{ end }}
{{ if .SecurityHeaders.XXSSProtection }}
    add_header X-XSS-Protection "1; mode=block" always;
{{ end }}
{{ if .SecurityHeaders.ContentSecurityPolicy }}
    add_header Content-Security-Policy "{{ .SecurityHeaders.ContentSecurityPolicy }}" always;
{{ end }}

    location / {
        proxy_pass {{ .Upstream }};
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
    }
}
{{ end }}
`
