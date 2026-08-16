package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/infrapilot/agent/internal/config"
	"github.com/infrapilot/agent/internal/docker"
	"github.com/infrapilot/agent/internal/enrollment"
	agentgrpc "github.com/infrapilot/agent/internal/grpc"
	"github.com/infrapilot/agent/internal/logstreamer"
	"github.com/infrapilot/agent/internal/metrics"
	"github.com/infrapilot/agent/internal/nginx"
	"github.com/infrapilot/agent/internal/ssl"
	"github.com/infrapilot/agent/internal/sync"
)

func main() {
	// Initialize logger
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	logger.Info("Starting InfraPilot Agent")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize Docker client
	dockerClient, err := docker.NewClient()
	if err != nil {
		logger.Fatal("Failed to connect to Docker", zap.Error(err))
	}
	defer dockerClient.Close()
	logger.Info("Connected to Docker daemon")

	// Initialize Nginx controller only in managed mode
	var nginxController *nginx.Controller
	if cfg.IsManagedProxy() {
		nginxController, err = nginx.NewController(cfg.NginxConfigPath, cfg.NginxContainerName, logger)
		if err != nil {
			logger.Fatal("Failed to create nginx controller", zap.Error(err))
		}
		defer nginxController.Close()
		logger.Info("Nginx controller initialized (managed mode)",
			zap.String("config_path", cfg.NginxConfigPath),
			zap.String("container", cfg.NginxContainerName),
		)
	} else {
		logger.Info("External proxy mode - nginx controller disabled",
			zap.String("proxy_mode", cfg.ProxyMode),
		)
	}

	// Initialize metrics collector
	metricsCollector := metrics.NewCollector(logger)

	// Initialize enrollment manager
	enrollmentMgr := enrollment.NewManager(cfg.BackendHTTPURL, cfg.EnrollmentToken, cfg.DataDir, logger)

	// Load existing credentials or enroll with backend
	// If AGENT_ID is set via env, use it (for backwards compatibility)
	if cfg.AgentID != "" {
		logger.Info("Using agent ID from environment", zap.String("agent_id", cfg.AgentID))
	} else {
		// Try to enroll or load existing credentials
		if err := enrollmentMgr.LoadOrEnroll(ctx); err != nil {
			logger.Fatal("Failed to enroll agent", zap.Error(err))
		}
		cfg.AgentID = enrollmentMgr.GetAgentID()
	}

	// Initialize gRPC client
	grpcClient, err := agentgrpc.NewClient(cfg.BackendGRPCAddr, cfg.EnrollmentToken, logger)
	if err != nil {
		logger.Fatal("Failed to create gRPC client", zap.Error(err))
	}
	defer grpcClient.Close()

	// Start heartbeat loop using enrollment manager (fingerprint-based)
	if enrollmentMgr.IsEnrolled() {
		enrollmentMgr.StartHeartbeatLoop(ctx, time.Duration(cfg.HeartbeatInterval)*time.Second, metricsCollector)
	} else {
		// Fallback: start metrics-only heartbeat loop if using legacy agent ID
		go func() {
			ticker := time.NewTicker(time.Duration(cfg.HeartbeatInterval) * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					// Collect metrics
					sysMetrics := metricsCollector.CollectSystemMetrics()

					// Get container states
					containers, err := dockerClient.ListContainers(ctx)
					if err != nil {
						logger.Error("Failed to list containers", zap.Error(err))
						continue
					}

					// Send heartbeat via gRPC (legacy)
					if err := grpcClient.Heartbeat(ctx, cfg.AgentID, sysMetrics, containers); err != nil {
						logger.Error("Heartbeat failed", zap.Error(err))
					}
				}
			}
		}()
	}

	// Initialize SSL/ACME certificate manager
	// Always initialize in managed mode - email can be set later via commands
	var certManager *ssl.CertManager
	if cfg.IsManagedProxy() {
		certManager = ssl.NewCertManager(
			cfg.LetsEncryptDir,
			cfg.ACMEWebRoot,
			cfg.LetsEncryptEmail,
			cfg.LetsEncryptStage,
			logger,
		)
		logger.Info("SSL certificate manager initialized",
			zap.String("dir", cfg.LetsEncryptDir),
			zap.String("webroot", cfg.ACMEWebRoot),
			zap.String("email", cfg.LetsEncryptEmail),
			zap.Bool("staging", cfg.LetsEncryptStage),
		)
	}

	// Set agent ID for gRPC client
	grpcClient.SetAgentID(cfg.AgentID)

	// Create command handler
	cmdHandler := &CommandHandler{
		nginx:              nginxController,
		docker:             dockerClient,
		certManager:        certManager,
		logger:             logger,
		nginxContainerName: cfg.NginxContainerName,
		proxyMode:          cfg.ProxyMode,
		grpcClient:         grpcClient,
		dataDir:            cfg.DataDir,
	}

	// Start command stream processor
	go func() {
		if err := grpcClient.ConnectCommandStream(ctx, cmdHandler); err != nil {
			logger.Error("Command stream error", zap.Error(err))
		}
	}()

	// Start proxy syncer (HTTP polling fallback when gRPC streaming isn't available)
	if cfg.IsManagedProxy() && nginxController != nil {
		proxySyncer := sync.NewProxySyncer(cfg.BackendHTTPURL, cfg.AgentID, nginxController, logger)
		go proxySyncer.Start(ctx)
	}

	// Start log streamer (sends container logs to backend for persistence)
	if cfg.LogPersistence {
		logStreamer := logstreamer.NewStreamer(dockerClient.Client(), cfg.BackendHTTPURL, cfg.AgentID, logger)
		go logStreamer.Start(ctx)
		logger.Info("Log persistence enabled - streaming logs to backend")
	}

	// Start nginx log collector (sends nginx access logs to backend for analytics)
	if cfg.NginxLogAnalytics && cfg.IsManagedProxy() {
		nginxConfig := logstreamer.NginxCollectorConfig{
			LogPath:          cfg.NginxAccessLogPath,
			BackendURL:       cfg.BackendHTTPURL,
			AgentID:          cfg.AgentID,
			BufferSize:       cfg.NginxLogBufferSize,
			FlushInterval:    time.Duration(cfg.NginxLogFlushInterval) * time.Second,
			SkipHealthChecks: cfg.NginxLogSkipHealth,
			NormalizePaths:   false, // Keep original paths for detailed analytics
		}
		nginxCollector := logstreamer.NewNginxCollector(nginxConfig, logger)
		go func() {
			if err := nginxCollector.Start(ctx); err != nil {
				logger.Error("Nginx log collector error", zap.Error(err))
			}
		}()
		logger.Info("Nginx log analytics enabled - streaming access logs to backend",
			zap.String("log_path", cfg.NginxAccessLogPath),
		)
	}

	// Start certificate renewal checker (runs daily)
	if certManager != nil {
		go func() {
			// Check for renewals once at startup after a delay
			time.Sleep(30 * time.Second)
			cmdHandler.checkCertificateRenewals(ctx)

			// Then check daily
			ticker := time.NewTicker(24 * time.Hour)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					cmdHandler.checkCertificateRenewals(ctx)
				}
			}
		}()
		logger.Info("Certificate renewal checker started")
	}

	logger.Info("Agent running",
		zap.String("agent_id", cfg.AgentID),
		zap.String("backend", cfg.BackendGRPCAddr),
		zap.String("proxy_mode", cfg.ProxyMode),
	)

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down agent...")
	cancel()

	// Give goroutines time to cleanup
	time.Sleep(time.Second)
	logger.Info("Agent stopped")
}

func init() {
	fmt.Println("InfraPilot Agent v0.1.0")
}

// CommandHandler processes commands from the backend
type CommandHandler struct {
	nginx              *nginx.Controller
	docker             *docker.Client
	certManager        *ssl.CertManager
	logger             *zap.Logger
	nginxContainerName string
	proxyMode          string // "managed" or "external"
	grpcClient         *agentgrpc.Client
	dataDir            string // base dir for agent state; source builds check out under dataDir/builds
}

// sendPullProgress sends a pull progress update back to the backend
func (h *CommandHandler) sendPullProgress(requestID string, progress *docker.PullProgressInfo) {
	if h.grpcClient == nil {
		h.logger.Warn("grpcClient is nil, cannot send progress")
		return
	}
	h.logger.Info("Sending pull progress",
		zap.String("request_id", requestID),
		zap.String("status", progress.Status),
		zap.Int("progress", progress.Progress),
	)
	h.grpcClient.SendProgress(requestID, "pull_progress", progress)
}

// IsManagedProxy returns true if InfraPilot manages the proxy
func (h *CommandHandler) IsManagedProxy() bool {
	return h.proxyMode != "external"
}

// ErrExternalProxyMode is returned when nginx operations are attempted in external mode
var ErrExternalProxyMode = fmt.Errorf("operation not available: proxy is in external mode")

// HandleNginxApply writes and applies an nginx proxy configuration
func (h *CommandHandler) HandleNginxApply(ctx context.Context, proxyID string, config nginx.ProxyConfig) (string, error) {
	if !h.IsManagedProxy() {
		h.logger.Warn("Ignoring nginx apply - external proxy mode",
			zap.String("domain", config.Domain),
		)
		return "", ErrExternalProxyMode
	}

	h.logger.Info("Applying nginx config",
		zap.String("proxy_id", proxyID),
		zap.String("domain", config.Domain),
	)

	hash, err := h.nginx.ApplyConfig(ctx, proxyID, config)
	if err != nil {
		h.logger.Error("Failed to apply nginx config",
			zap.Error(err),
			zap.String("domain", config.Domain),
		)
		return "", err
	}

	h.logger.Info("Nginx config applied successfully",
		zap.String("domain", config.Domain),
		zap.String("hash", hash[:16]),
	)
	return hash, nil
}

// HandleNginxDelete removes an nginx proxy configuration
func (h *CommandHandler) HandleNginxDelete(ctx context.Context, domain string) error {
	if !h.IsManagedProxy() {
		h.logger.Warn("Ignoring nginx delete - external proxy mode",
			zap.String("domain", domain),
		)
		return ErrExternalProxyMode
	}

	h.logger.Info("Deleting nginx config", zap.String("domain", domain))

	if err := h.nginx.DeleteConfig(ctx, domain); err != nil {
		h.logger.Error("Failed to delete nginx config",
			zap.Error(err),
			zap.String("domain", domain),
		)
		return err
	}

	// Reload nginx after deletion
	if err := h.nginx.Reload(ctx); err != nil {
		h.logger.Error("Failed to reload nginx after delete",
			zap.Error(err),
		)
		return err
	}

	h.logger.Info("Nginx config deleted successfully", zap.String("domain", domain))
	return nil
}

// ============ Network Command Handlers ============

// HandleListNetworks returns all available Docker networks
func (h *CommandHandler) HandleListNetworks(ctx context.Context) ([]docker.NetworkInfo, error) {
	h.logger.Info("Listing Docker networks")

	networks, err := h.docker.ListNetworks(ctx)
	if err != nil {
		h.logger.Error("Failed to list networks", zap.Error(err))
		return nil, err
	}

	h.logger.Info("Listed networks", zap.Int("count", len(networks)))
	return networks, nil
}

// HandleGetContainerNetworks returns networks for a specific container
func (h *CommandHandler) HandleGetContainerNetworks(ctx context.Context, containerID string) ([]docker.ContainerNetworkInfo, error) {
	h.logger.Info("Getting container networks", zap.String("container_id", containerID))

	networks, err := h.docker.GetContainerNetworks(ctx, containerID)
	if err != nil {
		h.logger.Error("Failed to get container networks",
			zap.Error(err),
			zap.String("container_id", containerID),
		)
		return nil, err
	}

	h.logger.Info("Got container networks",
		zap.String("container_id", containerID),
		zap.Int("count", len(networks)),
	)
	return networks, nil
}

// HandleCheckNginxNetwork checks if nginx is connected to a target network
func (h *CommandHandler) HandleCheckNginxNetwork(ctx context.Context, networkID string) (bool, error) {
	if !h.IsManagedProxy() {
		h.logger.Warn("Nginx network check not available - external proxy mode")
		return false, ErrExternalProxyMode
	}

	// In managed mode (nginx_container_name == "local"), use the current container ID
	containerName := h.nginxContainerName
	if containerName == "local" {
		hostname, err := os.Hostname()
		if err != nil {
			h.logger.Error("Failed to get container ID", zap.Error(err))
			return false, err
		}
		containerName = hostname
	}

	h.logger.Info("Checking nginx network connection",
		zap.String("network_id", networkID),
		zap.String("nginx_container", containerName),
	)

	connected, err := h.docker.IsContainerOnNetwork(ctx, containerName, networkID)
	if err != nil {
		h.logger.Error("Failed to check nginx network",
			zap.Error(err),
			zap.String("network_id", networkID),
		)
		return false, err
	}

	h.logger.Info("Nginx network check complete",
		zap.String("network_id", networkID),
		zap.Bool("connected", connected),
	)
	return connected, nil
}

// HandleAttachNginxNetwork attaches nginx to a Docker network with safety validation
func (h *CommandHandler) HandleAttachNginxNetwork(ctx context.Context, networkID string) (*docker.NetworkAttachResult, error) {
	if !h.IsManagedProxy() {
		h.logger.Warn("Nginx network attach not available - external proxy mode")
		return &docker.NetworkAttachResult{
			Success:      false,
			NetworkID:    networkID,
			ErrorMessage: "cannot attach network: proxy is in external mode",
		}, nil
	}

	// In managed mode (nginx_container_name == "local"), use the current container ID
	// The hostname in Docker is typically the container ID
	containerName := h.nginxContainerName
	if containerName == "local" {
		hostname, err := os.Hostname()
		if err != nil {
			return &docker.NetworkAttachResult{
				Success:      false,
				NetworkID:    networkID,
				ErrorMessage: "failed to get container ID: " + err.Error(),
			}, nil
		}
		containerName = hostname
	}

	h.logger.Info("Attaching nginx to network",
		zap.String("network_id", networkID),
		zap.String("container", containerName),
	)

	// Safety validation
	safe, reason := h.docker.IsNetworkSafe(ctx, networkID)
	if !safe {
		h.logger.Warn("Network attachment blocked by safety check",
			zap.String("network_id", networkID),
			zap.String("reason", reason),
		)
		return &docker.NetworkAttachResult{
			Success:      false,
			NetworkID:    networkID,
			ErrorMessage: reason,
		}, nil
	}

	// Check for duplicate attachment
	alreadyConnected, err := h.docker.IsContainerOnNetwork(ctx, containerName, networkID)
	if err != nil {
		h.logger.Error("Failed to check existing connection", zap.Error(err))
		return &docker.NetworkAttachResult{
			Success:      false,
			NetworkID:    networkID,
			ErrorMessage: err.Error(),
		}, nil
	}
	if alreadyConnected {
		h.logger.Info("Nginx already connected to network", zap.String("network_id", networkID))
		// Get network details for response
		netInfo, _ := h.docker.InspectNetwork(ctx, networkID)
		networkName := networkID
		if netInfo != nil {
			networkName = netInfo.Name
		}
		return &docker.NetworkAttachResult{
			Success:      true,
			NetworkID:    networkID,
			NetworkName:  networkName,
			Message:      "nginx is already attached to this network",
		}, nil
	}

	// Perform attachment
	if err := h.docker.ConnectNetwork(ctx, networkID, containerName); err != nil {
		h.logger.Error("Failed to attach nginx to network",
			zap.String("network_id", networkID),
			zap.Error(err),
		)
		return &docker.NetworkAttachResult{
			Success:      false,
			NetworkID:    networkID,
			ErrorMessage: err.Error(),
		}, nil
	}

	// Get network details for response
	netInfo, _ := h.docker.InspectNetwork(ctx, networkID)
	networkName := networkID
	if netInfo != nil {
		networkName = netInfo.Name
	}

	h.logger.Info("Successfully attached nginx to network",
		zap.String("network_id", networkID),
		zap.String("network_name", networkName),
	)

	return &docker.NetworkAttachResult{
		Success:     true,
		NetworkID:   networkID,
		NetworkName: networkName,
	}, nil
}

// HandleDetachNginxNetwork detaches nginx from a Docker network
func (h *CommandHandler) HandleDetachNginxNetwork(ctx context.Context, networkID string) error {
	if !h.IsManagedProxy() {
		h.logger.Warn("Nginx network detach not available - external proxy mode")
		return ErrExternalProxyMode
	}

	// In managed mode (nginx_container_name == "local"), use the current container ID
	containerName := h.nginxContainerName
	if containerName == "local" {
		hostname, err := os.Hostname()
		if err != nil {
			return fmt.Errorf("failed to get container ID: %w", err)
		}
		containerName = hostname
	}

	h.logger.Info("Detaching nginx from network",
		zap.String("network_id", networkID),
		zap.String("container", containerName),
	)

	if err := h.docker.DisconnectNetwork(ctx, networkID, containerName); err != nil {
		h.logger.Error("Failed to detach nginx from network",
			zap.String("network_id", networkID),
			zap.Error(err),
		)
		return err
	}

	h.logger.Info("Successfully detached nginx from network", zap.String("network_id", networkID))
	return nil
}

// NetworkTestResult represents the result of a network connectivity test
type NetworkTestResult struct {
	Reachable      bool  `json:"reachable"`
	Message        string `json:"message"`
	AvailablePorts []int  `json:"available_ports,omitempty"`
}

// HandleNetworkTest tests if a container:port is accessible from the agent
func (h *CommandHandler) HandleNetworkTest(ctx context.Context, containerName string, port int) (*NetworkTestResult, error) {
	h.logger.Info("Testing network connectivity",
		zap.String("container", containerName),
		zap.Int("port", port),
	)

	// Resolve container name to IP address via Docker
	containerIP, err := h.docker.GetContainerIP(ctx, containerName)
	if err != nil {
		h.logger.Warn("Could not resolve container IP, trying direct name",
			zap.String("container", containerName),
			zap.Error(err),
		)
		// Fall back to using the container name directly (Docker DNS)
		containerIP = containerName
	}

	// Test TCP connection to the specified port
	reachable, err := h.docker.TestTCPConnection(ctx, containerIP, port, 5*time.Second)
	if err != nil {
		return &NetworkTestResult{
			Reachable: false,
			Message:   fmt.Sprintf("Connection test failed: %v", err),
		}, nil
	}

	if reachable {
		return &NetworkTestResult{
			Reachable: true,
			Message:   fmt.Sprintf("Port %d on %s is accessible", port, containerName),
		}, nil
	}

	// Port is not reachable - scan for available ports
	commonPorts := []int{80, 443, 3000, 3001, 4000, 5000, 8000, 8080, 8443, 9000}
	availablePorts := []int{}

	for _, p := range commonPorts {
		if p == port {
			continue // Skip the port we already tested
		}
		reachable, _ := h.docker.TestTCPConnection(ctx, containerIP, p, 2*time.Second)
		if reachable {
			availablePorts = append(availablePorts, p)
		}
	}

	message := fmt.Sprintf("Port %d on %s is not accessible", port, containerName)
	if len(availablePorts) > 0 {
		message += fmt.Sprintf(". Found open ports: %v", availablePorts)
	}

	return &NetworkTestResult{
		Reachable:      false,
		Message:        message,
		AvailablePorts: availablePorts,
	}, nil
}

// HandleCommand implements the grpc.CommandHandler interface
// Routes incoming commands to the appropriate handler
func (h *CommandHandler) HandleCommand(ctx context.Context, cmd *agentgrpc.BackendMessage) *agentgrpc.CommandResult {
	h.logger.Info("Processing command",
		zap.String("request_id", cmd.RequestId),
		zap.String("type", cmd.Type),
	)

	switch cmd.Type {
	case "nginx":
		return h.handleNginxCommand(ctx, cmd)
	case "network":
		return h.handleNetworkCommand(ctx, cmd)
	case "docker":
		return h.handleDockerCommand(ctx, cmd)
	case "ssl":
		return h.handleSSLCommand(ctx, cmd)
	default:
		h.logger.Warn("Unknown command type", zap.String("type", cmd.Type))
		return &agentgrpc.CommandResult{
			Success: false,
			Message: fmt.Sprintf("unknown command type: %s", cmd.Type),
		}
	}
}

func (h *CommandHandler) handleNginxCommand(ctx context.Context, cmd *agentgrpc.BackendMessage) *agentgrpc.CommandResult {
	var nginxCmd agentgrpc.NginxCommand
	if err := json.Unmarshal(cmd.Command, &nginxCmd); err != nil {
		return &agentgrpc.CommandResult{
			Success: false,
			Message: fmt.Sprintf("failed to parse nginx command: %v", err),
		}
	}

	h.logger.Info("Handling nginx command", zap.String("action", nginxCmd.Action))

	switch nginxCmd.Action {
	case "write_config":
		if !h.IsManagedProxy() {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: "cannot write config: proxy is in external mode",
			}
		}
		// Write config file
		if err := h.nginx.WriteConfigFile(nginxCmd.ConfigPath, nginxCmd.ConfigContent); err != nil {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: fmt.Sprintf("failed to write config: %v", err),
			}
		}
		// Test config
		if err := h.nginx.TestConfig(ctx); err != nil {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: fmt.Sprintf("config test failed: %v", err),
			}
		}
		// Reload nginx
		if err := h.nginx.Reload(ctx); err != nil {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: fmt.Sprintf("reload failed: %v", err),
			}
		}
		return &agentgrpc.CommandResult{
			Success: true,
			Message: "config applied successfully",
		}

	case "test_config":
		if !h.IsManagedProxy() {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: "cannot test config: proxy is in external mode",
			}
		}
		if err := h.nginx.TestConfig(ctx); err != nil {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: fmt.Sprintf("config test failed: %v", err),
			}
		}
		return &agentgrpc.CommandResult{
			Success: true,
			Message: "config is valid",
		}

	case "reload":
		if !h.IsManagedProxy() {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: "cannot reload: proxy is in external mode",
			}
		}
		if err := h.nginx.Reload(ctx); err != nil {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: fmt.Sprintf("reload failed: %v", err),
			}
		}
		return &agentgrpc.CommandResult{
			Success: true,
			Message: "nginx reloaded",
		}

	case "request_ssl":
		if !h.IsManagedProxy() {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: "cannot request SSL: proxy is in external mode",
			}
		}
		if h.certManager == nil {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: "SSL certificate manager not configured (set LETSENCRYPT_EMAIL)",
			}
		}

		domain := nginxCmd.Domain
		if domain == "" {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: "domain is required for SSL request",
			}
		}

		// Build domains list (include www if requested)
		domains := []string{domain}
		if nginxCmd.IncludeWWW {
			domains = append(domains, "www."+domain)
		}

		// Request certificate for all domains
		if err := h.certManager.RequestCertificateMulti(domains); err != nil {
			h.logger.Error("SSL certificate request failed",
				zap.String("domain", domain),
				zap.Bool("include_www", nginxCmd.IncludeWWW),
				zap.Error(err),
			)
			return &agentgrpc.CommandResult{
				Success: false,
				Message: fmt.Sprintf("SSL request failed: %v", err),
			}
		}

		// Reload nginx to pick up the new certificate
		if err := h.nginx.Reload(ctx); err != nil {
			h.logger.Warn("Failed to reload nginx after SSL cert install",
				zap.Error(err),
			)
		}

		message := fmt.Sprintf("SSL certificate obtained for %s", domain)
		if nginxCmd.IncludeWWW {
			message = fmt.Sprintf("SSL certificate obtained for %s and www.%s", domain, domain)
		}
		return &agentgrpc.CommandResult{
			Success: true,
			Message: message,
		}

	case "write_htpasswd":
		if !h.IsManagedProxy() {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: "cannot write htpasswd: proxy is in external mode",
			}
		}
		if nginxCmd.HtpasswdPath == "" {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: "htpasswd_path is required",
			}
		}
		if err := h.nginx.WriteHtpasswdFile(nginxCmd.HtpasswdPath, nginxCmd.HtpasswdContent); err != nil {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: fmt.Sprintf("failed to write htpasswd: %v", err),
			}
		}
		return &agentgrpc.CommandResult{
			Success: true,
			Message: "htpasswd file written",
		}

	case "delete_htpasswd":
		if !h.IsManagedProxy() {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: "cannot delete htpasswd: proxy is in external mode",
			}
		}
		if nginxCmd.HtpasswdPath == "" {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: "htpasswd_path is required",
			}
		}
		if err := h.nginx.DeleteHtpasswdFile(nginxCmd.HtpasswdPath); err != nil {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: fmt.Sprintf("failed to delete htpasswd: %v", err),
			}
		}
		return &agentgrpc.CommandResult{
			Success: true,
			Message: "htpasswd file deleted",
		}

	default:
		return &agentgrpc.CommandResult{
			Success: false,
			Message: fmt.Sprintf("unknown nginx action: %s", nginxCmd.Action),
		}
	}
}

func (h *CommandHandler) handleNetworkCommand(ctx context.Context, cmd *agentgrpc.BackendMessage) *agentgrpc.CommandResult {
	var netCmd agentgrpc.NetworkCommand
	if err := json.Unmarshal(cmd.Command, &netCmd); err != nil {
		return &agentgrpc.CommandResult{
			Success: false,
			Message: fmt.Sprintf("failed to parse network command: %v", err),
		}
	}

	h.logger.Info("Handling network command", zap.String("action", netCmd.Action))

	switch netCmd.Action {
	case "list_networks":
		networks, err := h.HandleListNetworks(ctx)
		if err != nil {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: err.Error(),
			}
		}
		data, _ := json.Marshal(networks)
		return &agentgrpc.CommandResult{
			Success: true,
			Message: "networks listed",
			Data:    data,
		}

	case "get_container_networks":
		networks, err := h.HandleGetContainerNetworks(ctx, netCmd.ContainerID)
		if err != nil {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: err.Error(),
			}
		}
		data, _ := json.Marshal(networks)
		return &agentgrpc.CommandResult{
			Success: true,
			Message: "container networks retrieved",
			Data:    data,
		}

	case "check_nginx":
		connected, err := h.HandleCheckNginxNetwork(ctx, netCmd.NetworkID)
		if err != nil {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: err.Error(),
			}
		}
		return &agentgrpc.CommandResult{
			Success: connected,
			Message: fmt.Sprintf("nginx connected: %v", connected),
		}

	case "attach_nginx":
		result, err := h.HandleAttachNginxNetwork(ctx, netCmd.NetworkID)
		if err != nil {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: err.Error(),
			}
		}
		data, _ := json.Marshal(result)
		return &agentgrpc.CommandResult{
			Success: result.Success,
			Message: result.ErrorMessage,
			Data:    data,
		}

	case "detach_nginx":
		if err := h.HandleDetachNginxNetwork(ctx, netCmd.NetworkID); err != nil {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: err.Error(),
			}
		}
		return &agentgrpc.CommandResult{
			Success: true,
			Message: "nginx detached from network",
		}

	case "network_test":
		// Parse additional parameters for network test
		var testCmd struct {
			Action        string `json:"action"`
			ContainerName string `json:"container_name"`
			Port          int    `json:"port"`
		}
		if err := json.Unmarshal(cmd.Command, &testCmd); err != nil {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: fmt.Sprintf("failed to parse network test command: %v", err),
			}
		}
		result, err := h.HandleNetworkTest(ctx, testCmd.ContainerName, testCmd.Port)
		if err != nil {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: err.Error(),
			}
		}
		data, _ := json.Marshal(result)
		return &agentgrpc.CommandResult{
			Success: result.Reachable,
			Message: result.Message,
			Data:    data,
		}

	default:
		return &agentgrpc.CommandResult{
			Success: false,
			Message: fmt.Sprintf("unknown network action: %s", netCmd.Action),
		}
	}
}

// SSLCommand represents an SSL-related command from the backend
type SSLCommand struct {
	Action        string `json:"action"` // request_cert, renew_cert, check_cert, start_dns_challenge, complete_dns_challenge
	Domain        string `json:"domain"`
	Email         string `json:"email,omitempty"`
	DNSProvider   string `json:"dns_provider,omitempty"`
	Staging       bool   `json:"staging,omitempty"`
	ForceRenew    bool   `json:"force_renew,omitempty"`
	ChallengeType string `json:"challenge_type,omitempty"` // "http" or "dns" (default: http)
	IncludeWWW    bool   `json:"include_www,omitempty"`
}

func (h *CommandHandler) handleSSLCommand(ctx context.Context, cmd *agentgrpc.BackendMessage) *agentgrpc.CommandResult {
	var sslCmd SSLCommand
	if err := json.Unmarshal(cmd.Command, &sslCmd); err != nil {
		return &agentgrpc.CommandResult{
			Success: false,
			Message: fmt.Sprintf("failed to parse ssl command: %v", err),
		}
	}

	h.logger.Info("Handling SSL command",
		zap.String("action", sslCmd.Action),
		zap.String("domain", sslCmd.Domain),
	)

	// Check if cert manager is initialized
	if h.certManager == nil {
		return &agentgrpc.CommandResult{
			Success: false,
			Message: "SSL certificate manager not initialized",
		}
	}

	switch sslCmd.Action {
	case "request_cert":
		// Update cert manager config if email/staging provided
		if sslCmd.Email != "" {
			h.certManager.Email = sslCmd.Email
		}
		h.certManager.Staging = sslCmd.Staging

		// Validate email is set
		if h.certManager.Email == "" {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: "Let's Encrypt email not configured",
			}
		}

		// Build domains list (include www if requested)
		domains := []string{sslCmd.Domain}
		if sslCmd.IncludeWWW {
			domains = append(domains, "www."+sslCmd.Domain)
		}

		// Request the certificate for all domains
		if err := h.certManager.RequestCertificateMulti(domains); err != nil {
			h.logger.Error("Failed to request certificate",
				zap.String("domain", sslCmd.Domain),
				zap.Bool("include_www", sslCmd.IncludeWWW),
				zap.Error(err),
			)
			return &agentgrpc.CommandResult{
				Success: false,
				Message: fmt.Sprintf("failed to request certificate: %v", err),
			}
		}

		// Reload nginx to pick up the new certificate
		if h.nginx != nil {
			if err := h.nginx.Reload(ctx); err != nil {
				h.logger.Warn("Failed to reload nginx after certificate request", zap.Error(err))
			}
		}

		message := fmt.Sprintf("SSL certificate requested for %s", sslCmd.Domain)
		if sslCmd.IncludeWWW {
			message = fmt.Sprintf("SSL certificate requested for %s and www.%s", sslCmd.Domain, sslCmd.Domain)
		}
		return &agentgrpc.CommandResult{
			Success: true,
			Message: message,
		}

	case "renew_cert":
		if err := h.certManager.RenewCertificate(sslCmd.Domain); err != nil {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: fmt.Sprintf("failed to renew certificate: %v", err),
			}
		}

		// Reload nginx to pick up the renewed certificate
		if h.nginx != nil {
			if err := h.nginx.Reload(ctx); err != nil {
				h.logger.Warn("Failed to reload nginx after certificate renewal", zap.Error(err))
			}
		}

		return &agentgrpc.CommandResult{
			Success: true,
			Message: fmt.Sprintf("SSL certificate renewed for %s", sslCmd.Domain),
		}

	case "check_cert":
		info, err := h.certManager.GetCertificateInfo(sslCmd.Domain)
		if err != nil {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: fmt.Sprintf("failed to check certificate: %v", err),
			}
		}
		data, _ := json.Marshal(info)
		return &agentgrpc.CommandResult{
			Success: true,
			Message: "certificate info retrieved",
			Data:    data,
		}

	case "start_dns_challenge":
		// Start DNS-01 challenge for wildcard certificates
		if sslCmd.Email != "" {
			h.certManager.Email = sslCmd.Email
		}
		h.certManager.Staging = sslCmd.Staging

		if h.certManager.Email == "" {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: "Let's Encrypt email not configured",
			}
		}

		challenge, err := h.certManager.StartDNSChallenge(sslCmd.Domain)
		if err != nil {
			h.logger.Error("Failed to start DNS challenge",
				zap.String("domain", sslCmd.Domain),
				zap.Error(err),
			)
			return &agentgrpc.CommandResult{
				Success: false,
				Message: fmt.Sprintf("failed to start DNS challenge: %v", err),
			}
		}

		data, _ := json.Marshal(challenge)
		return &agentgrpc.CommandResult{
			Success: true,
			Message: "DNS challenge started - add TXT record and complete",
			Data:    data,
		}

	case "complete_dns_challenge":
		// Complete DNS-01 challenge after TXT record is added
		if sslCmd.Email != "" {
			h.certManager.Email = sslCmd.Email
		}
		h.certManager.Staging = sslCmd.Staging

		if err := h.certManager.RequestCertificateWithDNS(sslCmd.Domain); err != nil {
			h.logger.Error("Failed to complete DNS challenge",
				zap.String("domain", sslCmd.Domain),
				zap.Error(err),
			)
			return &agentgrpc.CommandResult{
				Success: false,
				Message: fmt.Sprintf("failed to complete DNS challenge: %v", err),
			}
		}

		// Reload nginx to pick up the new certificate
		if h.nginx != nil {
			if err := h.nginx.Reload(ctx); err != nil {
				h.logger.Warn("Failed to reload nginx after certificate request", zap.Error(err))
			}
		}

		return &agentgrpc.CommandResult{
			Success: true,
			Message: fmt.Sprintf("SSL certificate obtained via DNS-01 for %s", sslCmd.Domain),
		}

	case "get_dns_challenge":
		// Get current DNS challenge info
		challenge, err := h.certManager.GetDNSChallenge(sslCmd.Domain)
		if err != nil {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: fmt.Sprintf("no pending DNS challenge: %v", err),
			}
		}
		data, _ := json.Marshal(challenge)
		return &agentgrpc.CommandResult{
			Success: true,
			Message: "DNS challenge info retrieved",
			Data:    data,
		}

	default:
		return &agentgrpc.CommandResult{
			Success: false,
			Message: fmt.Sprintf("unknown ssl action: %s", sslCmd.Action),
		}
	}
}

// DockerAuthConfig represents authentication for Docker registry operations
type DockerAuthConfig struct {
	Username      string `json:"username,omitempty"`
	Password      string `json:"password,omitempty"`
	ServerAddress string `json:"server_address,omitempty"`
}

// DockerCommand represents a Docker resource command from the backend
type DockerCommand struct {
	Action     string                 `json:"action"`
	NetworkID  string                 `json:"network_id,omitempty"`
	VolumeName string                 `json:"volume_name,omitempty"`
	ImageRef   string                 `json:"image_ref,omitempty"`
	ImageID    string                 `json:"image_id,omitempty"`
	Force      bool                   `json:"force,omitempty"`
	Options    map[string]interface{} `json:"options,omitempty"`
	AuthConfig *DockerAuthConfig      `json:"auth_config,omitempty"`
}

func (h *CommandHandler) handleDockerCommand(ctx context.Context, cmd *agentgrpc.BackendMessage) *agentgrpc.CommandResult {
	var dockerCmd DockerCommand
	if err := json.Unmarshal(cmd.Command, &dockerCmd); err != nil {
		return &agentgrpc.CommandResult{
			Success: false,
			Message: fmt.Sprintf("failed to parse docker command: %v", err),
		}
	}

	h.logger.Info("Handling docker command", zap.String("action", dockerCmd.Action))

	switch dockerCmd.Action {
	// ============ Network Operations ============
	case "inspect_network":
		network, err := h.docker.InspectNetworkDetail(ctx, dockerCmd.NetworkID)
		if err != nil {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: err.Error(),
			}
		}
		data, _ := json.Marshal(network)
		return &agentgrpc.CommandResult{
			Success: true,
			Message: "network inspected",
			Data:    data,
		}

	case "create_network":
		opts := docker.NetworkCreateOptions{
			Name:   getStringOption(dockerCmd.Options, "name"),
			Driver: getStringOption(dockerCmd.Options, "driver"),
		}
		if opts.Name == "" {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: "network name is required",
			}
		}
		network, err := h.docker.CreateNetwork(ctx, opts)
		if err != nil {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: err.Error(),
			}
		}
		data, _ := json.Marshal(network)
		return &agentgrpc.CommandResult{
			Success: true,
			Message: "network created",
			Data:    data,
		}

	case "delete_network":
		// Check if network is in use
		inUse, containers, err := h.docker.IsNetworkInUse(ctx, dockerCmd.NetworkID)
		if err != nil {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: err.Error(),
			}
		}
		if inUse && !dockerCmd.Force {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: fmt.Sprintf("network is in use by containers: %v", containers),
			}
		}
		if err := h.docker.DeleteNetwork(ctx, dockerCmd.NetworkID); err != nil {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: err.Error(),
			}
		}
		return &agentgrpc.CommandResult{
			Success: true,
			Message: "network deleted",
		}

	// ============ Volume Operations ============
	case "list_volumes":
		volumes, err := h.docker.ListVolumes(ctx)
		if err != nil {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: err.Error(),
			}
		}
		data, _ := json.Marshal(volumes)
		return &agentgrpc.CommandResult{
			Success: true,
			Message: "volumes listed",
			Data:    data,
		}

	case "inspect_volume":
		volume, err := h.docker.InspectVolume(ctx, dockerCmd.VolumeName)
		if err != nil {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: err.Error(),
			}
		}
		data, _ := json.Marshal(volume)
		return &agentgrpc.CommandResult{
			Success: true,
			Message: "volume inspected",
			Data:    data,
		}

	case "create_volume":
		opts := docker.VolumeCreateOptions{
			Name:   getStringOption(dockerCmd.Options, "name"),
			Driver: getStringOption(dockerCmd.Options, "driver"),
		}
		if opts.Name == "" {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: "volume name is required",
			}
		}
		volume, err := h.docker.CreateVolume(ctx, opts)
		if err != nil {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: err.Error(),
			}
		}
		data, _ := json.Marshal(volume)
		return &agentgrpc.CommandResult{
			Success: true,
			Message: "volume created",
			Data:    data,
		}

	case "delete_volume":
		// Check if volume is in use
		inUse, containers, err := h.docker.IsVolumeInUse(ctx, dockerCmd.VolumeName)
		if err != nil {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: err.Error(),
			}
		}
		if inUse && !dockerCmd.Force {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: fmt.Sprintf("volume is in use by containers: %v", containers),
			}
		}
		if err := h.docker.DeleteVolume(ctx, dockerCmd.VolumeName, dockerCmd.Force); err != nil {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: err.Error(),
			}
		}
		return &agentgrpc.CommandResult{
			Success: true,
			Message: "volume deleted",
		}

	// ============ Image Operations ============
	case "list_images":
		images, err := h.docker.ListImages(ctx)
		if err != nil {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: err.Error(),
			}
		}
		data, _ := json.Marshal(images)
		return &agentgrpc.CommandResult{
			Success: true,
			Message: "images listed",
			Data:    data,
		}

	case "inspect_image":
		image, err := h.docker.InspectImage(ctx, dockerCmd.ImageID)
		if err != nil {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: err.Error(),
			}
		}
		data, _ := json.Marshal(image)
		return &agentgrpc.CommandResult{
			Success: true,
			Message: "image inspected",
			Data:    data,
		}

	case "pull_image":
		if dockerCmd.ImageRef == "" {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: "image reference is required",
			}
		}
		// Convert auth config if provided
		var auth *docker.AuthConfig
		if dockerCmd.AuthConfig != nil {
			auth = &docker.AuthConfig{
				Username:      dockerCmd.AuthConfig.Username,
				Password:      dockerCmd.AuthConfig.Password,
				ServerAddress: dockerCmd.AuthConfig.ServerAddress,
			}
		}
		if err := h.docker.PullImage(ctx, dockerCmd.ImageRef, auth); err != nil {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: err.Error(),
			}
		}
		return &agentgrpc.CommandResult{
			Success: true,
			Message: fmt.Sprintf("image %s pulled successfully", dockerCmd.ImageRef),
		}

	case "pull_image_stream":
		if dockerCmd.ImageRef == "" {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: "image reference is required",
			}
		}
		// Convert auth config if provided
		var auth *docker.AuthConfig
		if dockerCmd.AuthConfig != nil {
			auth = &docker.AuthConfig{
				Username:      dockerCmd.AuthConfig.Username,
				Password:      dockerCmd.AuthConfig.Password,
				ServerAddress: dockerCmd.AuthConfig.ServerAddress,
			}
		}
		// Use streaming pull with progress callback
		progressFn := func(progress *docker.PullProgressInfo) {
			h.sendPullProgress(cmd.RequestId, progress)
		}
		if err := h.docker.PullImageWithProgress(ctx, dockerCmd.ImageRef, auth, progressFn); err != nil {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: err.Error(),
			}
		}
		return &agentgrpc.CommandResult{
			Success: true,
			Message: fmt.Sprintf("image %s pulled successfully", dockerCmd.ImageRef),
		}

	case "delete_image":
		// Check if image is in use
		inUse, containers, err := h.docker.IsImageInUse(ctx, dockerCmd.ImageID)
		if err != nil {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: err.Error(),
			}
		}
		if inUse && !dockerCmd.Force {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: fmt.Sprintf("image is in use by containers: %v", containers),
			}
		}
		if err := h.docker.DeleteImage(ctx, dockerCmd.ImageID, dockerCmd.Force); err != nil {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: err.Error(),
			}
		}
		return &agentgrpc.CommandResult{
			Success: true,
			Message: "image deleted",
		}

	// ============ Source Build (push-to-deploy) ============
	case "build":
		repoURL := getStringOption(dockerCmd.Options, "repo_url")
		imageTag := getStringOption(dockerCmd.Options, "image_tag")
		if repoURL == "" || imageTag == "" {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: "repo_url and image_tag are required",
			}
		}
		buildArgs := make(map[string]string)
		if m, ok := dockerCmd.Options["build_args"].(map[string]interface{}); ok {
			for k, v := range m {
				if s, ok := v.(string); ok {
					buildArgs[k] = s
				}
			}
		}
		buildRes, err := h.docker.BuildImage(ctx, docker.BuildImageConfig{
			RepoURL:        repoURL,
			Ref:            getStringOption(dockerCmd.Options, "ref"),
			Commit:         getStringOption(dockerCmd.Options, "commit"),
			DockerfilePath: getStringOption(dockerCmd.Options, "dockerfile_path"),
			ImageTag:       imageTag,
			GitToken:       getStringOption(dockerCmd.Options, "git_token"),
			BuildArgs:      buildArgs,
			WorkDir:        filepath.Join(h.dataDir, "builds"),
		})
		if err != nil {
			// Carry the build log back even on failure for debugging.
			var data []byte
			if buildRes != nil {
				data, _ = json.Marshal(buildRes)
			}
			return &agentgrpc.CommandResult{
				Success: false,
				Message: fmt.Sprintf("build failed: %v", err),
				Data:    data,
			}
		}
		data, _ := json.Marshal(buildRes)
		return &agentgrpc.CommandResult{
			Success: true,
			Message: "image built",
			Data:    data,
		}

	// ============ Container Run Operation ============
	case "run_container":
		// Parse run container options from the Options map
		imageRef := getStringOption(dockerCmd.Options, "image_ref")
		name := getStringOption(dockerCmd.Options, "name")
		networkID := getStringOption(dockerCmd.Options, "network_id")
		restartPolicy := getStringOption(dockerCmd.Options, "restart_policy")
		secretMethod := getStringOption(dockerCmd.Options, "secret_method")

		if imageRef == "" {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: "image_ref is required",
			}
		}
		if name == "" {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: "container name is required",
			}
		}

		// Parse environment variables
		env := make(map[string]string)
		if envMap, ok := dockerCmd.Options["env"].(map[string]interface{}); ok {
			for k, v := range envMap {
				if str, ok := v.(string); ok {
					env[k] = str
				}
			}
		}

		// Parse port mappings
		ports := make(map[string]string)
		if portMap, ok := dockerCmd.Options["ports"].(map[string]interface{}); ok {
			for k, v := range portMap {
				if str, ok := v.(string); ok {
					ports[k] = str
				}
			}
		}

		// Parse labels
		labels := make(map[string]string)
		if labelMap, ok := dockerCmd.Options["labels"].(map[string]interface{}); ok {
			for k, v := range labelMap {
				if str, ok := v.(string); ok {
					labels[k] = str
				}
			}
		}

		// Parse secrets configuration
		var secrets []docker.SecretConfig
		if secretsList, ok := dockerCmd.Options["secrets"].([]interface{}); ok {
			for _, s := range secretsList {
				if secretMap, ok := s.(map[string]interface{}); ok {
					secret := docker.SecretConfig{
						Name:      getStringFromMap(secretMap, "name"),
						Value:     getStringFromMap(secretMap, "value"),
						MountPath: getStringFromMap(secretMap, "mount_path"),
					}
					secrets = append(secrets, secret)
				}
			}
		}

		// Parse networks configuration
		var networks []docker.NetworkConfig
		if networksList, ok := dockerCmd.Options["networks"].([]interface{}); ok {
			for _, n := range networksList {
				if netMap, ok := n.(map[string]interface{}); ok {
					netCfg := docker.NetworkConfig{
						NetworkID:       getStringFromMap(netMap, "network_id"),
						NetworkName:     getStringFromMap(netMap, "network_name"),
						CreateIfMissing: getBoolFromMap(netMap, "create_if_missing"),
						Driver:          getStringFromMap(netMap, "driver"),
					}
					if aliasesList, ok := netMap["aliases"].([]interface{}); ok {
						for _, a := range aliasesList {
							if alias, ok := a.(string); ok {
								netCfg.Aliases = append(netCfg.Aliases, alias)
							}
						}
					}
					networks = append(networks, netCfg)
				}
			}
		}

		// Parse volumes configuration
		var volumes []docker.VolumeConfig
		if volumesList, ok := dockerCmd.Options["volumes"].([]interface{}); ok {
			for _, v := range volumesList {
				if volMap, ok := v.(map[string]interface{}); ok {
					volume := docker.VolumeConfig{
						VolumeName:      getStringFromMap(volMap, "volume_name"),
						MountPath:       getStringFromMap(volMap, "mount_path"),
						CreateIfMissing: getBoolFromMap(volMap, "create_if_missing"),
						HostPath:        getStringFromMap(volMap, "host_path"),
						ReadOnly:        getBoolFromMap(volMap, "read_only"),
						FileMode:        getStringFromMap(volMap, "file_mode"),
					}
					// Stack companion file (v3/45): presence of "content" marks a file to materialize
					// on the host before mounting (empty string = an empty file, so key presence, not
					// value, is the signal).
					if cv, ok := volMap["content"].(string); ok {
						volume.Content = &cv
					}
					volumes = append(volumes, volume)
				}
			}
		}

		// Parse command
		var command []string
		if cmdList, ok := dockerCmd.Options["command"].([]interface{}); ok {
			for _, c := range cmdList {
				if str, ok := c.(string); ok {
					command = append(command, str)
				}
			}
		}

		// Parse pull_latest flag (default to true if not specified)
		pullLatest := true
		if pullLatestVal, ok := dockerCmd.Options["pull_latest"]; ok {
			if pullLatestBool, ok := pullLatestVal.(bool); ok {
				pullLatest = pullLatestBool
			}
		}

		// Parse auth config for private registries
		var authConfig *docker.AuthConfig
		if authMap, ok := dockerCmd.Options["auth"].(map[string]interface{}); ok {
			authConfig = &docker.AuthConfig{
				Username:      getStringFromMap(authMap, "username"),
				Password:      getStringFromMap(authMap, "password"),
				ServerAddress: getStringFromMap(authMap, "server_address"),
			}
		}

		// Use extended run if we have secrets, networks, volumes, command, or auth
		if len(secrets) > 0 || len(networks) > 0 || len(volumes) > 0 || len(command) > 0 || authConfig != nil {
			result, err := h.docker.RunContainerExtended(ctx, docker.ContainerRunExtendedConfig{
				ImageRef:      imageRef,
				Name:          name,
				NetworkID:     networkID,
				Env:           env,
				Ports:         ports,
				RestartPolicy: restartPolicy,
				Labels:        labels,
				Secrets:       secrets,
				SecretMethod:  secretMethod,
				Networks:      networks,
				Volumes:       volumes,
				Command:       command,
				PullLatest:    pullLatest,
				Auth:          authConfig,
			})
			if err != nil {
				return &agentgrpc.CommandResult{
					Success: false,
					Message: err.Error(),
				}
			}

			data, _ := json.Marshal(result)
			return &agentgrpc.CommandResult{
				Success: true,
				Message: "container started with extended configuration",
				Data:    data,
			}
		}

		// Use basic run for simple containers
		result, err := h.docker.RunContainer(ctx, docker.ContainerRunConfig{
			ImageRef:      imageRef,
			Name:          name,
			NetworkID:     networkID,
			Env:           env,
			Ports:         ports,
			RestartPolicy: restartPolicy,
			Labels:        labels,
		})
		if err != nil {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: err.Error(),
			}
		}

		data, _ := json.Marshal(result)
		return &agentgrpc.CommandResult{
			Success: true,
			Message: "container started",
			Data:    data,
		}

	case "scan_secrets":
		// Scan all containers for environment variable secrets and .env files
		containers, err := h.docker.ListContainers(ctx)
		if err != nil {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: fmt.Sprintf("failed to list containers: %v", err),
			}
		}

		var scannedSecrets []ScannedSecret
		for _, container := range containers {
			// Inspect each container to get environment variables
			info, err := h.docker.InspectContainer(ctx, container.ID)
			if err != nil {
				h.logger.Warn("Failed to inspect container",
					zap.String("container_id", container.ID),
					zap.Error(err),
				)
				continue
			}

			// Extract and analyze environment variables
			if info.Config != nil && len(info.Config.Env) > 0 {
				for _, envVar := range info.Config.Env {
					// Parse KEY=VALUE format
					parts := strings.SplitN(envVar, "=", 2)
					if len(parts) != 2 {
						continue
					}
					key := parts[0]
					value := parts[1]

					// Check if this looks like a secret
					secretType, isSensitive := detectSecretType(key, value)
					if secretType != "" {
						scannedSecrets = append(scannedSecrets, ScannedSecret{
							ContainerID:   container.ID,
							ContainerName: container.Name,
							Name:          key,
							SecretType:    secretType,
							Source:        "environment",
							IsSensitive:   isSensitive,
							Strength:      analyzeSecretStrength(value),
						})
					}
				}
			}

			// Check for .env files inside the container
			if container.State == "running" {
				envFileSecrets := h.scanContainerEnvFiles(ctx, container.ID, container.Name)
				scannedSecrets = append(scannedSecrets, envFileSecrets...)
			}
		}

		data, _ := json.Marshal(scannedSecrets)
		return &agentgrpc.CommandResult{
			Success: true,
			Message: fmt.Sprintf("scanned %d containers, found %d secrets", len(containers), len(scannedSecrets)),
			Data:    data,
		}

	// ============ Secret Migration Operations ============
	case "migrate_secrets":
		// Migrate secrets from .env file to Docker file secrets
		var migrateCmd struct {
			ContainerID   string              `json:"container_id"`
			Secrets       []docker.SecretMount `json:"secrets"`
			RemoveEnvVars []string            `json:"remove_env_vars"`
		}
		if err := json.Unmarshal(cmd.Command, &migrateCmd); err != nil {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: fmt.Sprintf("failed to parse migrate_secrets command: %v", err),
			}
		}

		if migrateCmd.ContainerID == "" {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: "container_id is required",
			}
		}
		if len(migrateCmd.Secrets) == 0 {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: "at least one secret is required",
			}
		}

		h.logger.Info("Starting secret migration",
			zap.String("container_id", migrateCmd.ContainerID),
			zap.Int("secrets_count", len(migrateCmd.Secrets)),
		)

		// Store original container config for rollback
		originalInfo, err := h.docker.InspectContainer(ctx, migrateCmd.ContainerID)
		if err != nil {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: fmt.Sprintf("failed to inspect container: %v", err),
			}
		}
		originalConfig, _ := json.Marshal(originalInfo)

		// Perform the migration
		result, err := h.docker.RecreateContainerWithSecrets(ctx, migrateCmd.ContainerID, migrateCmd.Secrets, migrateCmd.RemoveEnvVars)
		if err != nil {
			h.logger.Error("Secret migration failed",
				zap.String("container_id", migrateCmd.ContainerID),
				zap.Error(err),
			)
			return &agentgrpc.CommandResult{
				Success: false,
				Message: fmt.Sprintf("migration failed: %v", err),
			}
		}

		h.logger.Info("Secret migration completed",
			zap.String("new_container_id", result.NewContainerID),
			zap.Int("secrets_mounted", len(result.MountedSecrets)),
		)

		// Include original config in response for potential rollback
		response := struct {
			*docker.SecretMigrationResult
			OriginalConfig json.RawMessage `json:"original_config"`
		}{
			SecretMigrationResult: result,
			OriginalConfig:        originalConfig,
		}
		data, _ := json.Marshal(response)
		return &agentgrpc.CommandResult{
			Success: true,
			Message: fmt.Sprintf("migrated %d secrets to container %s", len(result.MountedSecrets), result.NewContainerID),
			Data:    data,
		}

	case "verify_migration":
		// Verify that secrets are properly mounted in the container
		var verifyCmd struct {
			ContainerID     string   `json:"container_id"`
			ExpectedSecrets []string `json:"expected_secrets"`
		}
		if err := json.Unmarshal(cmd.Command, &verifyCmd); err != nil {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: fmt.Sprintf("failed to parse verify_migration command: %v", err),
			}
		}

		if verifyCmd.ContainerID == "" {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: "container_id is required",
			}
		}

		h.logger.Info("Verifying secret migration",
			zap.String("container_id", verifyCmd.ContainerID),
			zap.Int("expected_secrets", len(verifyCmd.ExpectedSecrets)),
		)

		result, err := h.docker.VerifySecretsMigration(ctx, verifyCmd.ContainerID, verifyCmd.ExpectedSecrets)
		if err != nil {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: fmt.Sprintf("verification error: %v", err),
			}
		}

		data, _ := json.Marshal(result)
		if result.Success {
			return &agentgrpc.CommandResult{
				Success: true,
				Message: "all secrets verified successfully",
				Data:    data,
			}
		}
		return &agentgrpc.CommandResult{
			Success: false,
			Message: fmt.Sprintf("verification failed: %v", result.Errors),
			Data:    data,
		}

	case "rollback_migration":
		// Rollback a secret migration to restore original container
		var rollbackCmd struct {
			ContainerID    string          `json:"container_id"`
			OriginalConfig json.RawMessage `json:"original_config"`
			CreatedSecrets []string        `json:"created_secrets"`
		}
		if err := json.Unmarshal(cmd.Command, &rollbackCmd); err != nil {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: fmt.Sprintf("failed to parse rollback_migration command: %v", err),
			}
		}

		if rollbackCmd.ContainerID == "" {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: "container_id is required",
			}
		}
		if len(rollbackCmd.OriginalConfig) == 0 {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: "original_config is required",
			}
		}

		h.logger.Info("Rolling back secret migration",
			zap.String("container_id", rollbackCmd.ContainerID),
		)

		result, err := h.docker.RollbackSecretsMigration(ctx, rollbackCmd.ContainerID, rollbackCmd.OriginalConfig, rollbackCmd.CreatedSecrets)
		if err != nil {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: fmt.Sprintf("rollback error: %v", err),
			}
		}

		data, _ := json.Marshal(result)
		if result.Success {
			h.logger.Info("Migration rollback completed",
				zap.String("restored_container_id", result.RestoredContainerID),
				zap.Int("cleaned_secrets", result.CleanedUpSecrets),
			)
			return &agentgrpc.CommandResult{
				Success: true,
				Message: fmt.Sprintf("rollback successful, restored container %s", result.RestoredContainerID),
				Data:    data,
			}
		}
		return &agentgrpc.CommandResult{
			Success: false,
			Message: fmt.Sprintf("rollback failed: %s", result.ErrorMessage),
			Data:    data,
		}

	case "import_secrets":
		// Import secrets as Docker file secrets (similar to migrate_secrets but without removing env vars)
		var importCmd struct {
			ContainerID string `json:"container_id"`
			Secrets     []struct {
				Name      string `json:"name"`
				Value     string `json:"value"`
				MountPath string `json:"mount_path"`
			} `json:"secrets"`
		}
		// Parse the full command payload directly
		if err := json.Unmarshal(cmd.Command, &importCmd); err != nil {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: fmt.Sprintf("failed to parse import_secrets command: %v", err),
			}
		}

		h.logger.Info("Importing secrets to container",
			zap.String("container_id", importCmd.ContainerID),
			zap.Int("secrets_count", len(importCmd.Secrets)),
		)

		// Convert to SecretMount format
		secretMounts := make([]docker.SecretMount, len(importCmd.Secrets))
		for i, s := range importCmd.Secrets {
			secretMounts[i] = docker.SecretMount{
				Name:      s.Name,
				Value:     s.Value,
				MountPath: s.MountPath,
			}
		}

		// Use RecreateContainerWithSecrets without removing any env vars
		result, err := h.docker.RecreateContainerWithSecrets(ctx, importCmd.ContainerID, secretMounts, nil)
		if err != nil {
			h.logger.Error("Failed to import secrets",
				zap.Error(err),
				zap.String("container_id", importCmd.ContainerID),
			)
			return &agentgrpc.CommandResult{
				Success: false,
				Message: fmt.Sprintf("failed to import secrets: %v", err),
			}
		}

		h.logger.Info("Secrets imported successfully",
			zap.String("new_container_id", result.NewContainerID),
			zap.Int("secrets_mounted", len(result.MountedSecrets)),
		)

		data, _ := json.Marshal(map[string]interface{}{
			"new_container_id":  result.NewContainerID,
			"mounted_secrets":   result.MountedSecrets,
		})
		return &agentgrpc.CommandResult{
			Success: true,
			Message: fmt.Sprintf("imported %d secrets to container %s", len(result.MountedSecrets), result.NewContainerID),
			Data:    data,
		}

	case "import_env_vars":
		// Import secrets as environment variables
		var importCmd struct {
			ContainerID string            `json:"container_id"`
			EnvVars     map[string]string `json:"env_vars"`
		}
		// Parse the full command payload directly
		if err := json.Unmarshal(cmd.Command, &importCmd); err != nil {
			return &agentgrpc.CommandResult{
				Success: false,
				Message: fmt.Sprintf("failed to parse import_env_vars command: %v", err),
			}
		}

		h.logger.Info("Importing env vars to container",
			zap.String("container_id", importCmd.ContainerID),
			zap.Int("env_vars_count", len(importCmd.EnvVars)),
		)

		// Recreate container with additional env vars
		result, err := h.docker.RecreateContainerWithEnvVars(ctx, importCmd.ContainerID, importCmd.EnvVars)
		if err != nil {
			h.logger.Error("Failed to import env vars",
				zap.Error(err),
				zap.String("container_id", importCmd.ContainerID),
			)
			return &agentgrpc.CommandResult{
				Success: false,
				Message: fmt.Sprintf("failed to import env vars: %v", err),
			}
		}

		h.logger.Info("Env vars imported successfully",
			zap.String("new_container_id", result.NewContainerID),
		)

		data, _ := json.Marshal(map[string]interface{}{
			"new_container_id": result.NewContainerID,
		})
		return &agentgrpc.CommandResult{
			Success: true,
			Message: fmt.Sprintf("imported %d env vars to container %s", len(importCmd.EnvVars), result.NewContainerID),
			Data:    data,
		}

	default:
		return &agentgrpc.CommandResult{
			Success: false,
			Message: fmt.Sprintf("unknown docker action: %s", dockerCmd.Action),
		}
	}
}

// ScannedSecret represents a secret found during scanning
type ScannedSecret struct {
	ContainerID   string `json:"container_id"`
	ContainerName string `json:"container_name"`
	Name          string `json:"name"`
	SecretType    string `json:"secret_type"`
	Source        string `json:"source"` // "environment", "env_file"
	IsSensitive   bool   `json:"is_sensitive"`
	Strength      string `json:"strength"`
	EnvFilePath   string `json:"env_file_path,omitempty"` // Path to .env file if source is env_file
}

// scanContainerEnvFiles looks for .env files inside a running container
func (h *CommandHandler) scanContainerEnvFiles(ctx context.Context, containerID, containerName string) []ScannedSecret {
	var secrets []ScannedSecret

	// Common locations to check for .env files
	// Include various environment-specific variants
	basePaths := []string{"/app", "/", "/home", "/var/www", "/var/www/html", "/opt/app", "/src", "/root"}
	envVariants := []string{
		".env",
		".env.local",
		".env.production",
		".env.prod",
		".env.development",
		".env.dev",
		".env.staging",
		".env.stage",
		".env.test",
		".env.example", // Often contains real values by mistake
	}

	// First, find which .env files actually exist using a single command
	// This is much faster than checking each path individually
	var envFilePaths []string
	for _, basePath := range basePaths {
		for _, variant := range envVariants {
			if basePath == "/" {
				envFilePaths = append(envFilePaths, "/"+variant)
			} else {
				envFilePaths = append(envFilePaths, basePath+"/"+variant)
			}
		}
	}

	// Use ls to check which files exist in one command (faster than individual checks)
	lsArgs := append([]string{"ls", "-1"}, envFilePaths...)
	execID, err := h.docker.ExecCreate(ctx, containerID, lsArgs)
	var existingFiles []string
	if err == nil {
		output, err := h.docker.ExecAttach(ctx, execID)
		if err == nil && output != "" {
			// ls outputs existing files, one per line
			for _, line := range strings.Split(output, "\n") {
				line = strings.TrimSpace(line)
				if line != "" && !strings.HasPrefix(line, "ls:") {
					existingFiles = append(existingFiles, line)
				}
			}
		}
	}

	// If ls didn't work (no ls in container), fall back to checking a smaller set
	if len(existingFiles) == 0 {
		// Just check the most common locations
		existingFiles = []string{"/app/.env", "/.env", "/var/www/.env", "/src/.env"}
	}

	for _, envPath := range existingFiles {
		// Try to read the .env file using cat
		execID, err := h.docker.ExecCreate(ctx, containerID, []string{"cat", envPath})
		if err != nil {
			continue
		}

		// Get the output
		output, err := h.docker.ExecAttach(ctx, execID)
		if err != nil || output == "" {
			continue
		}

		// Parse the .env file content
		lines := strings.Split(output, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			// Skip empty lines and comments
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}

			// Parse KEY=VALUE
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			value := strings.Trim(strings.TrimSpace(parts[1]), "\"'")

			// Check if this looks like a secret
			secretType, isSensitive := detectSecretType(key, value)
			if secretType != "" {
				secrets = append(secrets, ScannedSecret{
					ContainerID:   containerID,
					ContainerName: containerName,
					Name:          key,
					SecretType:    secretType,
					Source:        "env_file",
					IsSensitive:   isSensitive,
					Strength:      analyzeSecretStrength(value),
					EnvFilePath:   envPath,
				})
			}
		}
	}

	return secrets
}

// detectSecretType checks if an environment variable looks like a secret
// Returns the secret type and whether it's sensitive
func detectSecretType(key, value string) (string, bool) {
	keyUpper := strings.ToUpper(key)

	// Skip empty values
	if value == "" {
		return "", false
	}

	// Check for common secret patterns in key names
	secretPatterns := map[string]string{
		"PASSWORD":     "password",
		"PASSWD":       "password",
		"SECRET":       "secret",
		"API_KEY":      "api_key",
		"APIKEY":       "api_key",
		"API_SECRET":   "api_secret",
		"ACCESS_KEY":   "access_key",
		"ACCESS_TOKEN": "access_token",
		"AUTH_TOKEN":   "auth_token",
		"TOKEN":        "token",
		"PRIVATE_KEY":  "private_key",
		"CREDENTIALS":  "credentials",
		"AWS_SECRET":   "aws_secret",
		"DB_PASSWORD":  "database_password",
		"DATABASE_PASSWORD": "database_password",
		"MYSQL_PASSWORD":    "database_password",
		"POSTGRES_PASSWORD": "database_password",
		"REDIS_PASSWORD":    "database_password",
		"MONGO_PASSWORD":    "database_password",
		"ENCRYPTION_KEY":    "encryption_key",
		"SIGNING_KEY":       "signing_key",
		"JWT_SECRET":        "jwt_secret",
		"SESSION_SECRET":    "session_secret",
		"COOKIE_SECRET":     "cookie_secret",
		"SMTP_PASSWORD":     "smtp_password",
		"SSH_KEY":           "ssh_key",
		"SSL_KEY":           "ssl_key",
		"TLS_KEY":           "tls_key",
		"WEBHOOK_SECRET":    "webhook_secret",
		"CLIENT_SECRET":     "client_secret",
	}

	for pattern, secretType := range secretPatterns {
		if strings.Contains(keyUpper, pattern) {
			return secretType, true
		}
	}

	// Check for suffixes
	suffixes := []string{"_KEY", "_SECRET", "_PASSWORD", "_TOKEN", "_CREDENTIALS"}
	for _, suffix := range suffixes {
		if strings.HasSuffix(keyUpper, suffix) {
			return "generic_secret", true
		}
	}

	return "", false
}

// analyzeSecretStrength returns the strength of a secret value
func analyzeSecretStrength(value string) string {
	length := len(value)

	// Basic strength analysis
	hasLower := false
	hasUpper := false
	hasDigit := false
	hasSpecial := false

	for _, c := range value {
		switch {
		case c >= 'a' && c <= 'z':
			hasLower = true
		case c >= 'A' && c <= 'Z':
			hasUpper = true
		case c >= '0' && c <= '9':
			hasDigit = true
		default:
			hasSpecial = true
		}
	}

	complexity := 0
	if hasLower {
		complexity++
	}
	if hasUpper {
		complexity++
	}
	if hasDigit {
		complexity++
	}
	if hasSpecial {
		complexity++
	}

	if length >= 32 && complexity >= 3 {
		return "strong"
	} else if length >= 16 && complexity >= 2 {
		return "medium"
	} else if length >= 8 {
		return "weak"
	}
	return "very_weak"
}

// getStringOption safely extracts a string value from options map
func getStringOption(options map[string]interface{}, key string) string {
	if options == nil {
		return ""
	}
	if val, ok := options[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

func getStringFromMap(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

func getBoolFromMap(m map[string]interface{}, key string) bool {
	if m == nil {
		return false
	}
	if val, ok := m[key]; ok {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return false
}

// ============ Certificate Renewal ============

// checkCertificateRenewals checks all certificates and renews those expiring soon
func (h *CommandHandler) checkCertificateRenewals(ctx context.Context) {
	if h.certManager == nil {
		return
	}

	h.logger.Info("Checking for certificate renewals")

	// Get all proxy domains from nginx config directory
	// In a real implementation, you'd query the database or track domains
	// For now, we scan the cert directory
	certDir := h.certManager.CertDir
	liveDir := certDir + "/live"

	entries, err := os.ReadDir(liveDir)
	if err != nil {
		h.logger.Debug("No certificates to check", zap.Error(err))
		return
	}

	renewedCount := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		domain := entry.Name()
		if domain == "" || domain == "." || domain == ".." {
			continue
		}

		// Check if renewal is needed (30 days before expiry)
		needsRenewal, err := h.certManager.NeedsRenewal(domain, 30)
		if err != nil {
			h.logger.Debug("Could not check renewal status",
				zap.String("domain", domain),
				zap.Error(err),
			)
			continue
		}

		if !needsRenewal {
			h.logger.Debug("Certificate still valid",
				zap.String("domain", domain),
			)
			continue
		}

		h.logger.Info("Renewing certificate", zap.String("domain", domain))

		if err := h.certManager.RenewCertificate(domain); err != nil {
			h.logger.Error("Failed to renew certificate",
				zap.String("domain", domain),
				zap.Error(err),
			)
			continue
		}

		renewedCount++
		h.logger.Info("Certificate renewed successfully",
			zap.String("domain", domain),
		)
	}

	if renewedCount > 0 {
		// Reload nginx to pick up renewed certificates
		if err := h.nginx.Reload(ctx); err != nil {
			h.logger.Error("Failed to reload nginx after renewals", zap.Error(err))
		} else {
			h.logger.Info("Nginx reloaded after certificate renewals",
				zap.Int("renewed_count", renewedCount),
			)
		}
	}
}
