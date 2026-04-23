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
	FeatureDataGovernance      = "data_governance"
	FeatureRuntimeSecurity     = "runtime_security"
	FeatureUnlimitedAgents     = "unlimited_agents"
	FeatureSSO                 = "sso"
	FeatureAuditLogs           = "audit_logs"
	FeatureSLASupport          = "sla_support"
	FeatureCustomIntegrations  = "custom_integrations"
)

// CommunityFeatures returns the features available in Community Edition (no key required).
func CommunityFeatures() []string {
	return []string{
		FeatureCoreMonitoring,
		FeatureBasicAlerts,
		FeatureAdvancedAlerts,
		FeatureStackManagement,
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
		FeatureDataGovernance,
		FeatureRuntimeSecurity,
		FeatureUnlimitedAgents,
		FeatureSSO,
		FeatureAuditLogs,
		FeatureSLASupport,
		FeatureCustomIntegrations,
	}
}
