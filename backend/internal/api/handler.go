package api

import (
	"os"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/infrapilot/backend/internal/auth"
	"github.com/infrapilot/backend/internal/enterprise/audit"
	"github.com/infrapilot/backend/internal/enterprise/license"
	"github.com/infrapilot/backend/internal/enterprise/multitenancy"
	"github.com/infrapilot/backend/internal/enterprise/policy"
	"github.com/infrapilot/backend/internal/enterprise/sso"
	ssoldap "github.com/infrapilot/backend/internal/enterprise/sso/ldap"
	"github.com/infrapilot/backend/internal/enterprise/sso/oidc"
	"github.com/infrapilot/backend/internal/enterprise/sso/saml"
)

type Handler struct {
	db     *pgxpool.Pool
	auth   *auth.Service
	logger *zap.Logger
}

func NewHandler(db *pgxpool.Pool, authService *auth.Service, logger *zap.Logger) *Handler {
	return &Handler{
		db:     db,
		auth:   authService,
		logger: logger,
	}
}

func (h *Handler) RegisterRoutes(r *gin.Engine) {
	// Health check
	r.GET("/health", h.healthCheck)

	// API v1 routes
	v1 := r.Group("/api/v1")
	v1.Use(license.Middleware()) // Add license context to all requests
	{
		// License info (public - needed for UI to show edition)
		v1.GET("/license", license.LicenseInfoHandler())
		// Setup routes (public - only work when no users exist)
		v1.GET("/setup/status", h.getSetupStatus)
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
				agents.GET("/:id/proxies/:pid", h.getProxyHost)
				agents.PUT("/:id/proxies/:pid", h.RequireModifyProxy(), h.updateProxyHost)
				agents.DELETE("/:id/proxies/:pid", h.RequireModifyProxy(), h.deleteProxyHost)
				agents.POST("/:id/proxies/:pid/ssl", h.RequireModifyProxy(), h.requestSSL)
				agents.GET("/:id/proxies/:pid/config", h.getProxyConfig)
				agents.POST("/:id/proxies/:pid/test", h.RequireModifyProxy(), h.testProxyConfig)
				agents.GET("/:id/proxies/:pid/security-headers", h.getSecurityHeaders)
				agents.PUT("/:id/proxies/:pid/security-headers", h.RequireModifyProxy(), h.updateSecurityHeaders)

				// Nginx management
				agents.POST("/:id/nginx/test", h.RequireModifyProxy(), h.testNginxConfig)
				agents.POST("/:id/nginx/reload", h.RequireModifyProxy(), h.reloadNginx)

				// Rate limits
				agents.GET("/:id/proxies/:pid/rate-limits", h.listRateLimits)
				agents.POST("/:id/proxies/:pid/rate-limits", h.RequireModifyProxy(), h.createRateLimit)
				agents.PUT("/:id/proxies/:pid/rate-limits/:rlid", h.RequireModifyProxy(), h.updateRateLimit)
				agents.DELETE("/:id/proxies/:pid/rate-limits/:rlid", h.RequireModifyProxy(), h.deleteRateLimit)

				// Containers (TODO: Route through agent via gRPC in production)
				agents.GET("/:id/containers", h.listContainersReal)
				agents.GET("/:id/containers/:cid", h.getContainerReal)
				agents.POST("/:id/containers/:cid/start", h.RequireModifyContainers(), h.startContainerReal)
				agents.POST("/:id/containers/:cid/stop", h.RequireModifyContainers(), h.stopContainerReal)
				agents.POST("/:id/containers/:cid/restart", h.RequireModifyContainers(), h.restartContainerReal)
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

				// Logs
				agents.GET("/:id/logs/nginx", h.getNginxLogsReal)
				agents.GET("/:id/logs/unified", h.getUnifiedLogsReal)
				agents.GET("/:id/logs/stream", h.streamUnifiedLogs)

				// Databases
				agents.GET("/:id/databases", h.listDatabases)
				agents.POST("/:id/databases", h.RequireModifyContainers(), h.addDatabase)
				agents.DELETE("/:id/databases/:did", h.RequireModifyContainers(), h.removeDatabase)
				agents.GET("/:id/databases/:did/metrics", h.getDatabaseMetrics)
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

			// Audit
			protected.GET("/audit", h.getAuditLogsReal)

			// Users (super_admin only)
			users := protected.Group("/users")
			users.Use(h.RequireRole(auth.RoleSuperAdmin))
			{
				users.GET("", h.listUsersReal)
				users.POST("", h.createUserReal)
				users.PUT("/:id", h.updateUserReal)
				users.DELETE("/:id", h.deleteUserReal)
			}

			// System Settings (super_admin only)
			settings := protected.Group("/settings")
			settings.Use(h.RequireRole(auth.RoleSuperAdmin))
			{
				settings.GET("", h.getSystemSettings)
				settings.GET("/domain", h.getInfraPilotDomain)
				settings.PUT("/domain", h.updateInfraPilotDomain)
				settings.DELETE("/domain", h.deleteInfraPilotDomain)
			}

			// SSO Providers (super_admin only - enterprise feature)
			ssoHandler := sso.NewHandler(h.db)
			ssoGroup := protected.Group("/sso")
			ssoGroup.Use(h.RequireRole(auth.RoleSuperAdmin))
			{
				ssoGroup.GET("/providers", ssoHandler.ListProviders)
				ssoGroup.POST("/providers", ssoHandler.CreateProvider)
				ssoGroup.GET("/providers/:id", ssoHandler.GetProvider)
				ssoGroup.PUT("/providers/:id", ssoHandler.UpdateProvider)
				ssoGroup.DELETE("/providers/:id", ssoHandler.DeleteProvider)
				ssoGroup.GET("/providers/:id/mappings", ssoHandler.ListRoleMappings)
				ssoGroup.POST("/providers/:id/mappings", ssoHandler.CreateRoleMapping)
				ssoGroup.DELETE("/providers/:id/mappings/:mid", ssoHandler.DeleteRoleMapping)
			}

			// Enterprise Audit & Compliance (super_admin only - enterprise feature)
			auditHandler := audit.NewHandler(h.db, h.logger)
			auditGroup := protected.Group("/audit")
			auditGroup.Use(h.RequireRole(auth.RoleSuperAdmin))
			{
				// Configuration
				auditGroup.GET("/config", auditHandler.GetConfig)
				auditGroup.PUT("/config", auditHandler.UpdateConfig)

				// Exports
				auditGroup.GET("/exports", auditHandler.ListExports)
				auditGroup.POST("/exports", auditHandler.CreateExport)
				auditGroup.GET("/exports/:id", auditHandler.GetExport)
				auditGroup.GET("/exports/:id/download", auditHandler.DownloadExport)

				// Compliance Reports
				auditGroup.GET("/reports", auditHandler.ListReports)
				auditGroup.POST("/reports", auditHandler.CreateReport)
				auditGroup.GET("/reports/:id", auditHandler.GetReport)

				// Forwarding
				auditGroup.POST("/forwarding/test", auditHandler.TestForwarding)

				// Retention & Integrity
				auditGroup.POST("/retention/cleanup", auditHandler.RunRetentionCleanup)
				auditGroup.GET("/integrity", auditHandler.VerifyIntegrity)
			}

			// Multi-Tenancy (enterprise feature)
			mtHandler := multitenancy.NewHandler(h.db, h.logger)

			// Organizations
			orgs := protected.Group("/orgs")
			{
				orgs.GET("", mtHandler.ListOrganizations)
				orgs.POST("", mtHandler.CreateOrganization)
				orgs.GET("/:id", mtHandler.GetOrganization)
				orgs.PUT("/:id", h.RequireRole(auth.RoleSuperAdmin), mtHandler.UpdateOrganization)
				orgs.DELETE("/:id", h.RequireRole(auth.RoleSuperAdmin), mtHandler.DeleteOrganization)
				orgs.GET("/:id/usage", mtHandler.GetOrganizationUsage)

				// Members
				orgs.GET("/:id/members", mtHandler.ListMembers)
				orgs.POST("/:id/members", h.RequireRole(auth.RoleSuperAdmin), mtHandler.AddMember)
				orgs.PUT("/:id/members/:uid", h.RequireRole(auth.RoleSuperAdmin), mtHandler.UpdateMember)
				orgs.DELETE("/:id/members/:uid", h.RequireRole(auth.RoleSuperAdmin), mtHandler.RemoveMember)

				// Invitations
				orgs.GET("/:id/invitations", mtHandler.ListInvitations)
				orgs.POST("/:id/invitations", h.RequireRole(auth.RoleSuperAdmin), mtHandler.CreateInvitation)
				orgs.DELETE("/:id/invitations/:iid", h.RequireRole(auth.RoleSuperAdmin), mtHandler.RevokeInvitation)

				// Enrollment Tokens
				orgs.GET("/:id/enrollment-tokens", mtHandler.ListEnrollmentTokens)
				orgs.POST("/:id/enrollment-tokens", h.RequireRole(auth.RoleSuperAdmin), mtHandler.CreateEnrollmentToken)
				orgs.PUT("/:id/enrollment-tokens/:tid/revoke", h.RequireRole(auth.RoleSuperAdmin), mtHandler.RevokeEnrollmentToken)
				orgs.DELETE("/:id/enrollment-tokens/:tid", h.RequireRole(auth.RoleSuperAdmin), mtHandler.DeleteEnrollmentToken)
			}

			// Accept invitation (public within protected - user must be logged in)
			protected.POST("/invitations/:token/accept", mtHandler.AcceptInvitation)

			// Policy Engine (enterprise feature)
			policyHandler := policy.NewHandler(h.db, h.logger)
			policies := protected.Group("/policies")
			{
				policies.GET("", policyHandler.ListPolicies)
				policies.POST("", h.RequireRole(auth.RoleAdmin, auth.RoleSuperAdmin), policyHandler.CreatePolicy)
				policies.GET("/:id", policyHandler.GetPolicy)
				policies.PUT("/:id", h.RequireRole(auth.RoleAdmin, auth.RoleSuperAdmin), policyHandler.UpdatePolicy)
				policies.DELETE("/:id", h.RequireRole(auth.RoleAdmin, auth.RoleSuperAdmin), policyHandler.DeletePolicy)

				// Templates
				policies.GET("/templates", policyHandler.ListTemplates)
				policies.POST("/templates/:id/create", h.RequireRole(auth.RoleAdmin, auth.RoleSuperAdmin), policyHandler.CreateFromTemplate)

				// Violations
				policies.GET("/violations", policyHandler.ListViolations)
				policies.GET("/violations/:id", policyHandler.GetViolation)
				policies.POST("/violations/:id/resolve", h.RequireRole(auth.RoleAdmin, auth.RoleSuperAdmin), policyHandler.ResolveViolation)

				// Stats
				policies.GET("/stats", policyHandler.GetPolicyStats)
			}
		}

		// Public SSO routes (for login page - no auth required)
		ssoPublicHandler := sso.NewHandler(h.db)
		v1.GET("/auth/sso/providers", ssoPublicHandler.GetPublicProviders)

		// OIDC authentication routes (public - for SSO flow)
		baseURL := os.Getenv("BASE_URL")
		if baseURL == "" {
			baseURL = "http://localhost:8080"
		}
		jwtSecret := []byte(os.Getenv("JWT_SECRET"))

		oidcHandler := oidc.NewHandler(h.db, baseURL, jwtSecret)
		v1.GET("/auth/oidc/authorize", oidcHandler.Authorize)
		v1.GET("/auth/oidc/callback", oidcHandler.Callback)

		// SAML authentication routes (public - for SSO flow)
		samlHandler := saml.NewHandler(h.db, baseURL, jwtSecret)
		v1.GET("/auth/saml/metadata", samlHandler.Metadata)
		v1.GET("/auth/saml/authorize", samlHandler.Authorize)
		v1.POST("/auth/saml/acs", samlHandler.ACS)

		// LDAP authentication routes (public - for SSO flow)
		ldapHandler := ssoldap.NewHandler(h.db, jwtSecret)
		v1.POST("/auth/ldap", ldapHandler.Authenticate)

		// Agent enrollment routes (public - for SaaS one-liner install)
		v1.POST("/agents/enroll", h.EnrollAgent)
		v1.GET("/agents/enroll/status", h.GetEnrollmentStatus)
		v1.POST("/agents/heartbeat", h.AgentHeartbeat)

		// Agent WebSocket command stream (public - agents connect with their ID)
		v1.GET("/agents/:id/ws/commands", h.agentCommandStream)

		// Log ingestion (public - agents push logs)
		v1.POST("/logs/ingest", h.IngestLogs)
	}

	// Protected log routes (require auth)
	logs := v1.Group("/logs")
	logs.Use(h.AuthMiddleware())
	logs.Use(h.OrgMiddleware())
	{
		logs.GET("/persisted", h.GetPersistedLogs)
		logs.GET("/sources", h.GetLogSources)
		logs.GET("/retention", h.GetLogRetentionConfig)
		logs.PUT("/retention", h.RequireRole(auth.RoleSuperAdmin), h.UpdateLogRetentionConfig)
		logs.GET("/stats", h.GetLogStats)
		logs.POST("/cleanup", h.RequireRole(auth.RoleSuperAdmin), h.RunLogCleanup)
	}
}

// Health check endpoint
func (h *Handler) healthCheck(c *gin.Context) {
	lic := license.Current()
	c.JSON(200, gin.H{
		"status":  "ok",
		"edition": lic.Edition,
	})
}
