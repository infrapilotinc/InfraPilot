package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/infrapilot/agent/internal/nginx"
)

// ProxyHost represents a proxy configuration from the backend
type ProxyHost struct {
	ID             string  `json:"id"`
	AgentID        string  `json:"agent_id"`
	Domain         string  `json:"domain"`
	UpstreamTarget string  `json:"upstream_target"`
	SSLEnabled     bool    `json:"ssl_enabled"`
	SSLCertPath    *string `json:"ssl_cert_path,omitempty"`
	SSLKeyPath     *string `json:"ssl_key_path,omitempty"`
	ForceSSL       bool    `json:"force_ssl"`
	HTTP2Enabled   bool    `json:"http2_enabled"`
	IncludeWWW     bool    `json:"include_www"`
	Status         string  `json:"status"`
	IsSystemProxy  bool    `json:"is_system_proxy"`
}

// ProxySyncer periodically syncs proxy configurations from the backend
type ProxySyncer struct {
	backendURL   string
	agentID      string
	token        string
	nginx        *nginx.Controller
	logger       *zap.Logger
	syncInterval time.Duration
	lastSync     map[string]string // domain -> config hash
}

// NewProxySyncer creates a new proxy syncer
func NewProxySyncer(backendURL, agentID, token string, nginxCtrl *nginx.Controller, logger *zap.Logger) *ProxySyncer {
	return &ProxySyncer{
		backendURL:   backendURL,
		agentID:      agentID,
		token:        token,
		nginx:        nginxCtrl,
		logger:       logger,
		syncInterval: 10 * time.Second,
		lastSync:     make(map[string]string),
	}
}

// Start begins the periodic sync loop
func (s *ProxySyncer) Start(ctx context.Context) {
	s.logger.Info("Starting proxy syncer",
		zap.String("backend", s.backendURL),
		zap.String("agent_id", s.agentID),
		zap.Duration("interval", s.syncInterval),
	)

	// Initial sync
	s.syncProxies(ctx)

	ticker := time.NewTicker(s.syncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Proxy syncer stopped")
			return
		case <-ticker.C:
			s.syncProxies(ctx)
		}
	}
}

// syncProxies fetches proxy configs from backend and applies them
func (s *ProxySyncer) syncProxies(ctx context.Context) {
	proxies, err := s.fetchProxies(ctx)
	if err != nil {
		s.logger.Error("Failed to fetch proxies", zap.Error(err))
		return
	}

	s.logger.Debug("Fetched proxies", zap.Int("count", len(proxies)))

	// Track which domains we've seen
	seenDomains := make(map[string]bool)

	for _, proxy := range proxies {
		if proxy.Status != "active" {
			continue
		}

		// Skip system proxies - they are managed by the backend directly
		if proxy.IsSystemProxy {
			s.logger.Debug("Skipping system proxy", zap.String("domain", proxy.Domain))
			seenDomains[proxy.Domain] = true // Still mark as seen to prevent deletion
			continue
		}

		seenDomains[proxy.Domain] = true

		// Convert to nginx config
		config := nginx.ProxyConfig{
			Domain:       proxy.Domain,
			Upstream:     proxy.UpstreamTarget,
			SSLEnabled:   proxy.SSLEnabled,
			ForceSSL:     proxy.ForceSSL,
			HTTP2Enabled: proxy.HTTP2Enabled,
			IncludeWWW:   proxy.IncludeWWW,
		}

		// Use custom SSL paths if provided (e.g., for wildcard certs)
		if proxy.SSLCertPath != nil && *proxy.SSLCertPath != "" {
			config.SSLCertPath = *proxy.SSLCertPath
		}
		if proxy.SSLKeyPath != nil && *proxy.SSLKeyPath != "" {
			config.SSLKeyPath = *proxy.SSLKeyPath
		}

		// Generate config hash to check if changed
		configJSON, _ := json.Marshal(config)
		configHash := fmt.Sprintf("%x", configJSON)

		// Skip if unchanged
		if s.lastSync[proxy.Domain] == configHash {
			continue
		}

		s.logger.Info("Applying proxy config",
			zap.String("domain", proxy.Domain),
			zap.String("upstream", proxy.UpstreamTarget),
		)

		// Apply config
		hash, err := s.nginx.ApplyConfig(ctx, proxy.ID, config)
		if err != nil {
			s.logger.Error("Failed to apply proxy config",
				zap.String("domain", proxy.Domain),
				zap.Error(err),
			)
			continue
		}

		s.lastSync[proxy.Domain] = configHash
		s.logger.Info("Proxy config applied",
			zap.String("domain", proxy.Domain),
			zap.String("hash", hash[:16]),
		)
	}

	// Remove configs for deleted proxies
	for domain := range s.lastSync {
		if !seenDomains[domain] {
			s.logger.Info("Removing deleted proxy config", zap.String("domain", domain))
			if err := s.nginx.DeleteConfig(ctx, domain); err != nil {
				s.logger.Error("Failed to delete proxy config",
					zap.String("domain", domain),
					zap.Error(err),
				)
			}
			delete(s.lastSync, domain)
		}
	}
}

// fetchProxies retrieves proxy configurations from the backend API
func (s *ProxySyncer) fetchProxies(ctx context.Context) ([]ProxyHost, error) {
	url := fmt.Sprintf("%s/api/v1/agents/%s/proxies", s.backendURL, s.agentID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Internal service-to-service auth (shared INTERNAL_SERVICE_TOKEN)
	req.Header.Set("X-Internal-Token", s.token)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("backend returned %d: %s", resp.StatusCode, string(body))
	}

	var proxies []ProxyHost
	if err := json.NewDecoder(resp.Body).Decode(&proxies); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return proxies, nil
}
