package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/infrapilot/backend/internal/auth"
	"github.com/infrapilot/backend/internal/config"
	"github.com/infrapilot/backend/internal/crypto"
	agentgrpc "github.com/infrapilot/backend/internal/grpc"
	"github.com/infrapilot/backend/internal/license"
	"github.com/infrapilot/backend/internal/registry"
	"github.com/infrapilot/backend/internal/telemetry"
	"github.com/infrapilot/backend/internal/webhook"
)

type Handler struct {
	db              *pgxpool.Pool
	auth            *auth.Service
	logger          *zap.Logger
	webhookService  *webhook.Service
	encryptionSvc   *crypto.EncryptionService
	license         *license.Client
	cfg             *config.Config
	version         string
	registryService *registry.Service
	telemetry       *telemetry.Client
}

func NewHandler(db *pgxpool.Pool, authService *auth.Service, logger *zap.Logger, encryptionSvc *crypto.EncryptionService, licenseClient *license.Client, cfg *config.Config, version string, tel *telemetry.Client) *Handler {
	return &Handler{
		db:              db,
		auth:            authService,
		logger:          logger,
		webhookService:  webhook.NewService(db, logger, encryptionSvc),
		encryptionSvc:   encryptionSvc,
		license:         licenseClient,
		cfg:             cfg,
		version:         version,
		registryService: registry.NewService(db, logger, encryptionSvc),
		telemetry:       tel,
	}
}

func (h *Handler) RegisterRoutes(r *gin.Engine) {
	// Health check
	r.GET("/health", h.healthCheck)

	// API v1 routes
	v1 := r.Group("/api/v1")
	{
		// Version (public)
		v1.GET("/version", func(c *gin.Context) {
			c.JSON(200, gin.H{"version": h.version, "edition": Edition})
		})

		// Setup routes (public - only work when no users exist)
		v1.GET("/setup/status", h.getSetupStatus)
		v1.POST("/setup/license", h.setupLicense)
		v1.POST("/setup", h.createInitialAdmin)

		// Auth routes (public)
		authGroup := v1.Group("/auth")
		{
			authGroup.POST("/login", h.login)
			authGroup.POST("/logout", h.logout)
			authGroup.POST("/refresh", h.refreshToken)
			authGroup.POST("/mfa/verify", h.verifyMFA) // Public - uses MFA token
			authGroup.GET("/me", h.AuthMiddleware(), h.getCurrentUser)

			// MFA management (requires auth)
			authGroup.POST("/mfa/setup", h.AuthMiddleware(), h.setupMFA)
			authGroup.POST("/mfa/confirm", h.AuthMiddleware(), h.confirmMFASetup)
			authGroup.POST("/mfa/disable", h.AuthMiddleware(), h.disableMFA)
			authGroup.POST("/mfa/backup-codes", h.AuthMiddleware(), h.regenerateBackupCodes)

			// Sensitive operation verification (requires auth)
			authGroup.POST("/verify-password", h.AuthMiddleware(), h.verifyPassword)
			authGroup.POST("/send-confirmation-otp", h.AuthMiddleware(), h.sendConfirmationOTP)
			authGroup.POST("/verify-confirmation-otp", h.AuthMiddleware(), h.verifyConfirmationOTP)
		}

		// Webhook receiver (public - uses signature verification)
		webhooks := v1.Group("/webhooks")
		{
			webhooks.POST("/:id/receive", h.receiveWebhook)
		}

		// Protected routes
		protected := v1.Group("")
		protected.Use(h.AuthMiddleware())
		protected.Use(h.OrgMiddleware())
		{
			// Agents
			agents := protected.Group("/agents")
			{
				agents.GET("", h.listAgents)
				agents.POST("", h.RequireRole(auth.RoleSuperAdmin), h.createAgent)
				agents.GET("/:id", h.getAgent)
				agents.DELETE("/:id", h.RequireRole(auth.RoleSuperAdmin), h.deleteAgent)
				agents.GET("/:id/metrics", h.getAgentMetrics)

				// Proxy hosts
				agents.GET("/:id/proxies", h.listProxyHosts)
				agents.POST("/:id/proxies", h.RequireModifyProxy(), h.createProxyHost)
				agents.POST("/:id/proxies/test-network", h.RequireModifyProxy(), h.testNetworkConnectivity)
				agents.GET("/:id/proxies/:pid", h.getProxyHost)
				agents.PUT("/:id/proxies/:pid", h.RequireModifyProxy(), h.updateProxyHost)
				agents.DELETE("/:id/proxies/:pid", h.RequireModifyProxy(), h.deleteProxyHost)
				agents.POST("/:id/proxies/:pid/ssl", h.RequireModifyProxy(), h.requestSSL)
				agents.POST("/:id/proxies/:pid/ssl/wildcard", h.RequireModifyProxy(), h.applyWildcardSSL)
				agents.GET("/:id/proxies/:pid/config", h.getProxyConfig)
				agents.POST("/:id/proxies/:pid/config/preview", h.RequireModifyProxy(), h.previewProxyConfig)
				agents.POST("/:id/proxies/:pid/test", h.RequireModifyProxy(), h.testProxyConfig)
				agents.GET("/:id/proxies/:pid/security-headers", h.getSecurityHeaders)
				agents.PUT("/:id/proxies/:pid/security-headers", h.RequireModifyProxy(), h.updateSecurityHeaders)

				// Basic auth users
				agents.GET("/:id/proxies/:pid/auth-users", h.listAuthUsers)
				agents.POST("/:id/proxies/:pid/auth-users", h.RequireModifyProxy(), h.createAuthUser)
				agents.DELETE("/:id/proxies/:pid/auth-users/:uid", h.RequireModifyProxy(), h.deleteAuthUser)

				// Nginx management
				agents.POST("/:id/nginx/test", h.RequireModifyProxy(), h.testNginxConfig)
				agents.POST("/:id/nginx/reload", h.RequireModifyProxy(), h.reloadNginx)

				// Rate limits
				agents.GET("/:id/proxies/:pid/rate-limits", h.listRateLimits)
				agents.POST("/:id/proxies/:pid/rate-limits", h.RequireModifyProxy(), h.createRateLimit)
				agents.PUT("/:id/proxies/:pid/rate-limits/:rlid", h.RequireModifyProxy(), h.updateRateLimit)
				agents.DELETE("/:id/proxies/:pid/rate-limits/:rlid", h.RequireModifyProxy(), h.deleteRateLimit)

				// Containers
				agents.GET("/:id/containers", h.listContainersReal)
				agents.GET("/:id/containers/:cid", h.getContainerReal)
				agents.POST("/:id/containers/:cid/start", h.RequireModifyContainers(), h.startContainerReal)
				agents.POST("/:id/containers/:cid/stop", h.RequireModifyContainers(), h.stopContainerReal)
				agents.POST("/:id/containers/:cid/restart", h.RequireModifyContainers(), h.restartContainerReal)
				agents.POST("/:id/containers/:cid/pause", h.RequireModifyContainers(), h.pauseContainerReal)
				agents.POST("/:id/containers/:cid/unpause", h.RequireModifyContainers(), h.unpauseContainerReal)
				agents.POST("/:id/containers/:cid/kill", h.RequireModifyContainers(), h.killContainerReal)
				agents.POST("/:id/containers/:cid/rename", h.RequireModifyContainers(), h.renameContainerReal)
				agents.PUT("/:id/containers/:cid", h.RequireModifyContainers(), h.updateContainerReal)
				agents.GET("/:id/containers/:cid/inspect", h.inspectContainerReal)
				agents.DELETE("/:id/containers/:cid", h.RequireModifyContainers(), h.deleteContainerReal)
				agents.GET("/:id/containers/:cid/logs", h.getContainerLogsReal)
				agents.GET("/:id/containers/:cid/logs/stream", h.streamContainerLogs)
				agents.GET("/:id/containers/:cid/exec", h.execContainer)
				agents.GET("/:id/containers/:cid/networks", h.getContainerNetworks)
				agents.GET("/:id/stacks", h.listStacksReal)

				// Networks (for nginx cross-network proxying)
				agents.GET("/:id/networks", h.listNetworks)
				agents.GET("/:id/networks/attachments", h.listNginxNetworkAttachments)
				agents.POST("/:id/networks/attach", h.RequireModifyProxy(), h.attachNginxNetwork)
				agents.POST("/:id/networks/detach", h.RequireModifyProxy(), h.detachNginxNetwork)
				agents.GET("/:id/networks/:nid/check-nginx", h.checkNginxNetwork)

				// Docker Resources (networks, volumes, images)
				docker := agents.Group("/:id/docker")
				{
					// Networks (full CRUD)
					docker.GET("/networks", h.listDockerNetworks)
					docker.GET("/networks/:nid", h.inspectDockerNetwork)
					docker.POST("/networks", h.RequireModifyContainers(), h.createDockerNetwork)
					docker.DELETE("/networks/:nid", h.RequireModifyContainers(), h.deleteDockerNetwork)

					// Volumes
					docker.GET("/volumes", h.listDockerVolumes)
					docker.GET("/volumes/:name", h.inspectDockerVolume)
					docker.POST("/volumes", h.RequireModifyContainers(), h.createDockerVolume)
					docker.DELETE("/volumes/:name", h.RequireModifyContainers(), h.deleteDockerVolume)

					// Images
					docker.GET("/images", h.listDockerImages)
					docker.GET("/images/:imgid", h.inspectDockerImage)
					docker.POST("/images/pull", h.RequireModifyContainers(), h.pullDockerImage)
					docker.DELETE("/images/:imgid", h.RequireModifyContainers(), h.deleteDockerImage)
				}

				// Logs
				agents.GET("/:id/logs/nginx", h.getNginxLogsReal)
				agents.GET("/:id/logs/unified", h.getUnifiedLogsReal)
				agents.GET("/:id/logs/stream", h.streamUnifiedLogs)

				// Deployments & Webhooks
				agents.GET("/:id/deployments", h.listDeployments)
				agents.POST("/:id/deployments", h.RequireModifyContainers(), h.createDeployment)
				agents.GET("/:id/deployments/:did", h.getDeployment)
				agents.GET("/:id/deployments/:did/spine", h.getDeploymentSpine)
				agents.POST("/:id/deployments/:did/rollback", h.RequireModifyContainers(), h.rollbackDeployment)
				agents.POST("/:id/deployments/:did/redeploy", h.RequireModifyContainers(), h.redeployDeployment)
				agents.DELETE("/:id/deployments/:did", h.RequireModifyContainers(), h.deleteDeployment)
				agents.POST("/:id/deployments/sync", h.syncDeploymentStatus)

				// Managed Stacks (multi-service docker-compose deployments)
				agents.POST("/:id/stacks/parse", h.RequireModifyContainers(), h.parseComposeYAML)
				agents.POST("/:id/managed-stacks", h.RequireModifyContainers(), h.createStack)
				agents.GET("/:id/managed-stacks", h.listManagedStacks)
				agents.GET("/:id/managed-stacks/:sid", h.getManagedStack)
				agents.GET("/:id/managed-stacks/:sid/progress", h.getStackProgress)
				agents.POST("/:id/managed-stacks/:sid/redeploy", h.RequireModifyContainers(), h.redeployManagedStack)
				agents.DELETE("/:id/managed-stacks/:sid", h.RequireModifyContainers(), h.deleteManagedStack)

				agents.GET("/:id/webhooks", h.listWebhooks)
				agents.POST("/:id/webhooks", h.RequireModifyContainers(), h.createWebhook)
				agents.GET("/:id/webhooks/:wid", h.getWebhook)
				agents.PUT("/:id/webhooks/:wid", h.RequireModifyContainers(), h.updateWebhook)
				agents.DELETE("/:id/webhooks/:wid", h.RequireModifyContainers(), h.deleteWebhook)
				agents.GET("/:id/webhooks/:wid/events", h.listWebhookEvents)
				agents.POST("/:id/webhooks/:wid/regenerate", h.RequireModifyContainers(), h.regenerateWebhookSecret)
		}

			// WebSocket helpers
			protected.GET("/pulls/ws", h.pullWebSocket)

			// License info (authenticated users can see their tier)
			protected.GET("/license", h.LicenseInfo)

			// Exposure & Rate Limits (used by /run/exposure section)
			exposure := protected.Group("/exposure")
			{
				exposure.GET("/endpoints", h.listExposedEndpoints)
				exposure.GET("/endpoints/:id", h.getExposedEndpoint)
				exposure.POST("/endpoints", h.RequireManageAlerts(), h.createExposedEndpoint)
				exposure.PUT("/endpoints/:id", h.RequireManageAlerts(), h.updateExposedEndpoint)
				exposure.DELETE("/endpoints/:id", h.RequireManageAlerts(), h.deleteExposedEndpoint)
				exposure.GET("/summary", h.getExposureSummary)
				exposure.GET("/map", h.getExposureMap)
			}

			ratelimits := protected.Group("/ratelimits")
			{
				ratelimits.GET("/profiles", h.listRateLimitProfiles)
				ratelimits.POST("/profiles", h.RequireManageAlerts(), h.createRateLimitProfile)
				ratelimits.PUT("/profiles/:id", h.RequireManageAlerts(), h.updateRateLimitProfile)
				ratelimits.DELETE("/profiles/:id", h.RequireManageAlerts(), h.deleteRateLimitProfile)
			}

			tls := protected.Group("/tls")
			{
				tls.GET("/posture", h.getTLSPosture)
				tls.GET("/scans", h.listTLSScans)
				tls.GET("/config", h.getTLSAlertConfig)
				tls.PUT("/config", h.RequireManageAlerts(), h.updateTLSAlertConfig)
			}

			// Traffic (CE: summary only; resources & policies are Pro/Enterprise)
			traffic := protected.Group("/traffic")
			{
				traffic.GET("/summary", h.getTrafficSummary)
			}

			// Alerts
			alerts := protected.Group("/alerts")
			{
				alerts.GET("/channels", h.listAlertChannelsReal)
				alerts.POST("/channels", h.RequireManageAlerts(), h.createAlertChannelReal)
				alerts.PUT("/channels/:id", h.RequireManageAlerts(), h.updateAlertChannelReal)
				alerts.DELETE("/channels/:id", h.RequireManageAlerts(), h.deleteAlertChannelReal)
				alerts.POST("/channels/:id/test", h.RequireManageAlerts(), h.testAlertChannelReal)

				alerts.GET("/rules", h.listAlertRulesReal)
				alerts.POST("/rules", h.RequireManageAlerts(), h.createAlertRuleReal)
				alerts.PUT("/rules/:id", h.RequireManageAlerts(), h.updateAlertRuleReal)
				alerts.DELETE("/rules/:id", h.RequireManageAlerts(), h.deleteAlertRuleReal)

				alerts.GET("/history", h.getAlertHistoryReal)
			}

			// Health & Monitoring
			protected.GET("/health/tls", h.getTLSHealth)
			protected.GET("/health/database", h.getDBHealth)
			protected.GET("/health/system", h.getSystemHealth)

			// Users (super_admin only)
			users := protected.Group("/users")
			users.Use(h.RequireRole(auth.RoleSuperAdmin))
			{
				users.GET("", h.listUsersReal)
				users.POST("", h.createUserReal)
				users.PUT("/:id", h.updateUserReal)
				users.DELETE("/:id", h.deleteUserReal)
			}

			// API Keys (any authenticated user manages their own keys)
			protected.GET("/api-keys", h.listAPIKeys)
			protected.POST("/api-keys", h.createAPIKey)
			protected.DELETE("/api-keys/:id", h.deleteAPIKey)

			// Container Registries
			registries := protected.Group("/registries")
			{
				registries.GET("", h.listRegistries)
				registries.POST("", h.RequireManageAlerts(), h.createRegistry)
				registries.GET("/:rid", h.getRegistry)
				registries.PUT("/:rid", h.RequireManageAlerts(), h.updateRegistry)
				registries.DELETE("/:rid", h.RequireManageAlerts(), h.deleteRegistry)
				registries.POST("/:rid/test", h.RequireManageAlerts(), h.testRegistryConnection)
				registries.GET("/:rid/repositories", h.listRegistryRepositories)
				registries.GET("/:rid/repositories/:repo/tags", h.listRegistryTags)
			}

			// Services — canonical deploy interface for CLI, webhooks, and UI
			services := protected.Group("/services")
			{
				services.GET("", h.listServices)
				services.PUT("", h.upsertService)
				services.GET("/:name", h.getService)
				services.DELETE("/:name", h.deleteService)
				services.POST("/:name/deploy", h.deployService)
				services.POST("/:name/rollback", h.rollbackService)
				services.GET("/:name/deployments", h.listServiceDeployments)
				services.GET("/:name/current", h.getCurrentDeployment)
			}

			// System Settings (super_admin only)
			settings := protected.Group("/settings")
			settings.Use(h.RequireRole(auth.RoleSuperAdmin))
			{
				settings.GET("", h.getSystemSettings)
				settings.GET("/domain", h.getInfraPilotDomain)
				settings.PUT("/domain", h.updateInfraPilotDomain)
				settings.DELETE("/domain", h.deleteInfraPilotDomain)

				// License key management
				settings.GET("/license", h.getLicenseSettings)
				settings.PUT("/license", h.updateLicenseKey)

				// Live CE→EE upgrade (SSE progress + auto-revert). CE only.
				settings.GET("/license/upgrade", h.upgradeToEnterprise)
				settings.GET("/license/upgrade/status", h.getUpgradeStatus)

				// Default pages
				settings.GET("/default-pages", h.listDefaultPages)
				settings.GET("/default-pages/:type", h.getDefaultPage)
				settings.PUT("/default-pages/:type", h.updateDefaultPage)
				settings.GET("/default-pages/:type/preview", h.previewDefaultPage)

				// Self-update (CE only)
				settings.GET("/update/check", h.checkForUpdate)
				settings.POST("/update/apply", h.applyUpdate)      // legacy fire-and-forget
				settings.GET("/update/apply/stream", h.applyUpdateStream) // live SSE progress

				// Anonymous funnel telemetry opt-out (v3/40 G1b)
				settings.GET("/privacy", h.getPrivacySettings)
				settings.PUT("/privacy", h.updatePrivacySettings)
			}

			// SSL/TLS Management
			ssl := protected.Group("/ssl")
			{
				ssl.GET("/check/:domain", h.checkDomainSSL)
				ssl.GET("/check-wildcard/:domain", h.checkWildcardSSL)
				ssl.GET("/verify-dns/:domain", h.verifyDNS)
				ssl.GET("/dns-instructions/:domain", h.getDNSInstructions)
				ssl.GET("/status", h.getSSLStatus)
				ssl.PUT("/settings", h.RequireRole(auth.RoleSuperAdmin), h.updateSSLSettings)
				ssl.POST("/request", h.RequireRole(auth.RoleSuperAdmin), h.requestSSLCertificate)
			ssl.POST("/upload", h.RequireRole(auth.RoleSuperAdmin), h.uploadSSLCertificate)

				// DNS-01 Challenge (for wildcard certificates)
				ssl.POST("/dns-challenge/start", h.RequireRole(auth.RoleSuperAdmin), h.startDNSChallenge)
				ssl.POST("/dns-challenge/complete", h.RequireRole(auth.RoleSuperAdmin), h.completeDNSChallenge)
				ssl.GET("/dns-challenge/:domain", h.getDNSChallenge)
				ssl.GET("/dns-challenge/verify/:domain", h.verifyDNSTXTRecord)

				// Certificate management
				ssl.GET("/certificates", h.listSSLCertificates)
				ssl.GET("/certificates/scan", h.scanSSLCertificates)
				ssl.POST("/certificates", h.RequireRole(auth.RoleSuperAdmin), h.registerSSLCertificate)
				ssl.GET("/certificates/:id", h.getSSLCertificate)
				ssl.DELETE("/certificates/:id", h.RequireRole(auth.RoleSuperAdmin), h.deleteSSLCertificate)
			}
		}

		// Agent enrollment routes
		v1.POST("/agents/enroll", h.EnrollAgent)
		v1.GET("/agents/enroll/status", h.GetEnrollmentStatus)
		v1.POST("/agents/heartbeat", h.AgentHeartbeat)
		v1.POST("/agents/:id/heartbeat", h.AgentHeartbeatByID) // legacy / unenrolled path

		// Agent WebSocket command stream
		v1.GET("/agents/:id/ws/commands", h.agentCommandStream)

		// Log ingestion (agents push logs)
		v1.POST("/logs/ingest", h.IngestLogs)

		// Nginx log ingestion (agents push nginx access logs)
		v1.POST("/logs/nginx/ingest", h.IngestNginxLogs)
	}

	// Traffic analytics routes (protected, require auth)
	analytics := v1.Group("/traffic/analytics")
	analytics.Use(h.AuthMiddleware())
	analytics.Use(h.OrgMiddleware())
	{
		analytics.GET("", h.GetTrafficAnalytics)
		analytics.GET("/summary", h.GetTrafficAnalyticsSummary)
		analytics.GET("/top-paths", h.GetTopPaths)
		analytics.GET("/status-codes", h.GetStatusCodeDistribution)
		analytics.GET("/domains", h.GetLogDomains)
		analytics.GET("/methods", h.GetMethodDistribution)
		analytics.GET("/clients", h.GetTopClients)
		analytics.GET("/user-agents", h.GetUserAgentStats)
	}
}

// Health check endpoint
func (h *Handler) healthCheck(c *gin.Context) {
	c.JSON(200, gin.H{
		"status":  "ok",
		"edition": Edition,
		"version": h.version,
	})
}

// StartBackgroundTasks starts background tasks like dispatching default page config
// This should be called after the HTTP server starts
func (h *Handler) StartBackgroundTasks(ctx context.Context) {
	go h.dispatchDefaultPageConfigOnStartup(ctx)
}

// dispatchDefaultPageConfigOnStartup checks if a domain is configured and dispatches
// the default page config and system proxy config once an agent connects
func (h *Handler) dispatchDefaultPageConfigOnStartup(ctx context.Context) {
	// Wait a bit for the agent to connect
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	dispatched := false

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if dispatched {
				return
			}

			// Check if a domain is configured for InfraPilot
			var agentID uuid.UUID
			var proxyID uuid.UUID
			var orgID uuid.UUID
			var domain string
			var sslEnabled, forceSSL, http2 bool
			var sslCertPath, sslKeyPath *string
			var basicAuthEnabled bool
			var basicAuthRealm string

			err := h.db.QueryRow(ctx, `
				SELECT ph.id, ph.agent_id, a.org_id, ph.domain, ph.ssl_enabled, ph.force_ssl, ph.http2_enabled,
				       ph.ssl_cert_path, ph.ssl_key_path,
				       COALESCE(ph.basic_auth_enabled, false), COALESCE(ph.basic_auth_realm, 'Restricted')
				FROM proxy_hosts ph
				JOIN agents a ON a.id = ph.agent_id
				WHERE ph.is_system_proxy = TRUE
				LIMIT 1
			`).Scan(&proxyID, &agentID, &orgID, &domain, &sslEnabled, &forceSSL, &http2, &sslCertPath, &sslKeyPath, &basicAuthEnabled, &basicAuthRealm)

			if err != nil {
				// No domain configured, nothing to do
				h.logger.Debug("No InfraPilot domain configured yet")
				return
			}

			// Check if agent is connected
			agentIDStr := agentID.String()
			if !agentgrpc.IsAgentConnected(agentIDStr) {
				h.logger.Debug("Waiting for agent to connect before dispatching startup config")
				continue
			}

			// Agent is connected and domain is configured
			h.logger.Info("Dispatching startup configs",
				zap.String("domain", domain),
				zap.String("agent_id", agentIDStr),
			)

			// Get cert paths from settings if available
			certPath := ""
			keyPath := ""
			if sslCertPath != nil {
				certPath = *sslCertPath
			}
			if sslKeyPath != nil {
				keyPath = *sslKeyPath
			}

			// Determine htpasswd path for this proxy (use sanitized domain)
			htpasswdPath := ""
			if basicAuthEnabled {
				htpasswdPath = fmt.Sprintf("/data/nginx/conf.d/.htpasswd_%s", strings.ReplaceAll(domain, ".", "_"))
			}

			// Dispatch the InfraPilot system proxy config (routes /api to backend)
			h.dispatchInfraPilotProxyConfigWithCert(ctx, agentID, proxyID, domain, forceSSL, http2, sslEnabled, certPath, keyPath, basicAuthEnabled, basicAuthRealm, htpasswdPath)

			// Dispatch the default page config (welcome page for IP access)
			h.dispatchDefaultPageConfig(agentID, orgID, true)

			dispatched = true
		}
	}
}
