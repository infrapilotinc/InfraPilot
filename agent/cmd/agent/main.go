package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
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
		enrollmentMgr.StartHeartbeatLoop(ctx, time.Duration(cfg.HeartbeatInterval)*time.Second)
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
	var certManager *ssl.CertManager
	if cfg.IsManagedProxy() && cfg.LetsEncryptEmail != "" {
		certManager = ssl.NewCertManager(
			cfg.LetsEncryptDir,
			cfg.LetsEncryptEmail,
			cfg.LetsEncryptStage,
			logger,
		)
		logger.Info("SSL certificate manager initialized",
			zap.String("dir", cfg.LetsEncryptDir),
			zap.String("email", cfg.LetsEncryptEmail),
			zap.Bool("staging", cfg.LetsEncryptStage),
		)
	}

	// Create command handler
	cmdHandler := &CommandHandler{
		nginx:              nginxController,
		docker:             dockerClient,
		certManager:        certManager,
		logger:             logger,
		nginxContainerName: cfg.NginxContainerName,
		proxyMode:          cfg.ProxyMode,
	}

	// Set agent ID for gRPC client
	grpcClient.SetAgentID(cfg.AgentID)

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

	h.logger.Info("Checking nginx network connection",
		zap.String("network_id", networkID),
		zap.String("nginx_container", h.nginxContainerName),
	)

	connected, err := h.docker.IsContainerOnNetwork(ctx, h.nginxContainerName, networkID)
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

	h.logger.Info("Attaching nginx to network",
		zap.String("network_id", networkID),
		zap.String("nginx_container", h.nginxContainerName),
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
	alreadyConnected, err := h.docker.IsContainerOnNetwork(ctx, h.nginxContainerName, networkID)
	if err != nil {
		h.logger.Error("Failed to check existing connection", zap.Error(err))
		return &docker.NetworkAttachResult{
			Success:      false,
			NetworkID:    networkID,
			ErrorMessage: err.Error(),
		}, nil
	}
	if alreadyConnected {
		h.logger.Warn("Nginx already connected to network", zap.String("network_id", networkID))
		return &docker.NetworkAttachResult{
			Success:      false,
			NetworkID:    networkID,
			ErrorMessage: "nginx is already attached to this network",
		}, nil
	}

	// Perform attachment
	if err := h.docker.ConnectNetwork(ctx, networkID, h.nginxContainerName); err != nil {
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

	h.logger.Info("Detaching nginx from network",
		zap.String("network_id", networkID),
		zap.String("nginx_container", h.nginxContainerName),
	)

	if err := h.docker.DisconnectNetwork(ctx, networkID, h.nginxContainerName); err != nil {
		h.logger.Error("Failed to detach nginx from network",
			zap.String("network_id", networkID),
			zap.Error(err),
		)
		return err
	}

	h.logger.Info("Successfully detached nginx from network", zap.String("network_id", networkID))
	return nil
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

		// Request certificate
		if err := h.certManager.RequestCertificate(domain); err != nil {
			h.logger.Error("SSL certificate request failed",
				zap.String("domain", domain),
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

		return &agentgrpc.CommandResult{
			Success: true,
			Message: fmt.Sprintf("SSL certificate obtained for %s", domain),
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

	default:
		return &agentgrpc.CommandResult{
			Success: false,
			Message: fmt.Sprintf("unknown network action: %s", netCmd.Action),
		}
	}
}

func (h *CommandHandler) handleDockerCommand(ctx context.Context, cmd *agentgrpc.BackendMessage) *agentgrpc.CommandResult {
	// Docker commands are already handled via HTTP API directly using Docker SDK
	// This is a placeholder for any additional Docker operations via gRPC
	return &agentgrpc.CommandResult{
		Success: false,
		Message: "docker commands via gRPC not implemented - use HTTP API",
	}
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
