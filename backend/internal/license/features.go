package license

// Feature constants — must match what infrapilot.org returns in the features array.
const (
	FeatureCoreMonitoring      = "core_monitoring"
	FeatureBasicAlerts         = "basic_alerts"
	FeatureAdvancedAlerts      = "advanced_alerts"
	FeatureSecretsManagement   = "secrets_management"
	FeatureVulnScanning        = "vulnerability_scanning"
	FeatureCodeQuality         = "code_quality"
	FeatureStackManagement     = "stack_management"
	FeatureCDDeployments       = "cd_deployments"       // CI/CD webhooks, deployment history, rollback
	FeatureTrafficAnalytics    = "traffic_analytics"    // Nginx access log analytics
	FeatureDataGovernance      = "data_governance"
	FeatureRuntimeSecurity     = "runtime_security"
	FeatureUnlimitedAgents     = "unlimited_agents"
	FeatureSSO                 = "sso"
	FeatureAuditLogs           = "audit_logs"
	FeatureSLASupport          = "sla_support"
	FeatureCustomIntegrations  = "custom_integrations"
	FeaturePushToDeploy        = "push_to_deploy" // build from git; free in every tier

	// Connected features (v3/35): unlocked by any account key, including the free
	// Community key. Keyless CE runs without them; adding an alert channel is the
	// moment the app asks for an email. Must match CONNECTED_FREE in infrapilot.org
	// packages/license.
	FeatureWebhooks          = "webhooks"           // outgoing notification webhooks (not deploy webhooks)
	FeatureChatNotifications = "chat_notifications" // Slack, Discord, Teams
	FeatureEmailAlerts       = "email_alerts"       // SMTP alert channels
)

// CommunityFeatures returns the free, full single-node platform available in
// Community Edition (no key required). CE is gated on SERVER COUNT (1), not
// features — paid tiers add advanced security + team/compliance + more servers.
//
// This is the CE-side mirror of infrapilot.org packages/license COMMUNITY_BASE —
// keep the two in sync. To change what's free, edit this one list.
func CommunityFeatures() []string {
	return []string{
		FeatureCoreMonitoring,
		FeatureBasicAlerts,
		FeatureAdvancedAlerts,
		FeatureStackManagement,
		FeatureCDDeployments,
		FeatureTrafficAnalytics,
		FeatureSecretsManagement,
		FeaturePushToDeploy,
	}
}

// ConnectedFreeFeatures returns the features a free Community key adds on top of
// CommunityFeatures. The license server already includes them for every keyed tier.
func ConnectedFreeFeatures() []string {
	return []string{
		FeatureWebhooks,
		FeatureChatNotifications,
		FeatureEmailAlerts,
	}
}

// AllFeatures returns every feature — used for offline dev mode.
func AllFeatures() []string {
	return []string{
		FeatureCoreMonitoring,
		FeatureBasicAlerts,
		FeatureAdvancedAlerts,
		FeatureSecretsManagement,
		FeatureVulnScanning,
		FeatureCodeQuality,
		FeatureStackManagement,
		FeatureCDDeployments,
		FeatureTrafficAnalytics,
		FeatureDataGovernance,
		FeatureRuntimeSecurity,
		FeatureUnlimitedAgents,
		FeatureSSO,
		FeatureAuditLogs,
		FeatureSLASupport,
		FeatureCustomIntegrations,
		FeaturePushToDeploy,
		FeatureWebhooks,
		FeatureChatNotifications,
		FeatureEmailAlerts,
	}
}
