const API_BASE = process.env.NEXT_PUBLIC_API_URL || "/api/v1";

async function fetchAPI<T>(
  endpoint: string,
  options: RequestInit = {},
  timeoutMs = 15000
): Promise<T> {
  const token =
    typeof window !== "undefined" ? localStorage.getItem("access_token") : null;

  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);

  const res = await fetch(`${API_BASE}${endpoint}`, {
    ...options,
    signal: options.signal ?? controller.signal,
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...options.headers,
    },
  }).finally(() => clearTimeout(timer));

  if (!res.ok) {
    const error = await res.json().catch(() => ({ error: "Request failed" }));
    throw new Error(error.error || "Request failed");
  }

  return res.json();
}

// Types
export interface Agent {
  id: string;
  org_id: string;
  name: string;
  hostname: string | null;
  status: "pending" | "active" | "offline";
  version: string | null;
  last_seen_at: string | null;
  created_at: string;
  enrollment_token?: string;
  // Resource metrics — populated on each heartbeat
  cpu_percent: number;
  memory_used_mb: number;
  memory_total_mb: number;
  disk_used_mb: number;
  disk_total_mb: number;
  uptime_seconds: number;
}

export interface Container {
  id: string;
  agent_id?: string;
  container_id: string;
  name: string;
  image: string;
  stack_name?: string | null;
  status: string;
  state?: string;
  cpu_percent?: number;
  memory_mb?: number;
  memory_limit_mb?: number;
  restart_count?: number;
  networks?: string[];
  created_at?: string;
}

export interface PortMapping {
  container_port: number;
  host_port: number;
  protocol: string;
  host_ip?: string;
}

export interface MountInfo {
  type: string;
  source: string;
  destination: string;
  mode: string;
  read_only: boolean;
}

export interface EnvVar {
  key: string;
  value: string;
}

export interface ContainerConfig {
  hostname: string;
  domainname: string;
  user: string;
  working_dir: string;
  entrypoint: string[];
  cmd: string[];
  restart_policy: string;
  privileged: boolean;
  tty: boolean;
  open_stdin: boolean;
}

export interface HealthCheckConfig {
  test: string[];
  interval: string;
  timeout: string;
  start_period: string;
  retries: number;
}

export interface HealthLogEntry {
  start: string;
  end: string;
  exit_code: number;
  output: string;
}

export interface NetworkDetail {
  name: string;
  network_id: string;
  ip_address: string;
  gateway: string;
  mac_address: string;
  aliases?: string[];
}

export interface ResourceLimits {
  cpu_shares: number;
  cpu_quota: number;
  cpu_period: number;
  cpuset_cpus: string;
  memory_limit: number;
  memory_swap: number;
  pids_limit: number;
}

export interface ContainerDetail extends Container {
  ports: PortMapping[];
  mounts: MountInfo[];
  environment: EnvVar[];
  config: ContainerConfig;
  health_check?: HealthCheckConfig;
  health_status?: string;
  health_log?: HealthLogEntry[];
  labels: Record<string, string>;
  network_details: NetworkDetail[];
  resources: ResourceLimits;
}

export interface Stack {
  name: string;
  container_count: number;
  running_count: number;
  status: "running" | "partial" | "stopped";
  containers: Container[];
}

export interface ProxyHost {
  id: string;
  agent_id: string;
  domain: string;
  upstream_target: string;
  proxy_type: 'upstream' | 'redirect';
  redirect_url?: string;
  redirect_code?: number;
  ssl_enabled: boolean;
  ssl_expires_at: string | null;
  force_ssl: boolean;
  http2_enabled: boolean;
  include_www: boolean;
  basic_auth_enabled: boolean;
  basic_auth_realm?: string;
  basic_auth_excluded_paths?: string[];
  access_log: boolean;
  error_log: boolean;
  log_format?: string;
  is_system_proxy: boolean;
  status: string;
  created_at: string;
  updated_at: string;
}

export interface TestNetworkRequest {
  container_name: string;
  port: number;
}

export interface TestNetworkResponse {
  reachable: boolean;
  message: string;
  available_ports?: number[];
}

export interface AuthUser {
  id: string;
  proxy_id: string;
  username: string;
  created_at: string;
  updated_at: string;
}

export interface ConfigPreviewRequest {
  domain: string;
  upstream_target: string;
  ssl_enabled: boolean;
  force_ssl: boolean;
  http2_enabled: boolean;
  include_www: boolean;
  basic_auth_enabled: boolean;
  basic_auth_realm: string;
  hsts_enabled: boolean;
  hsts_max_age: number;
  x_frame_options: string;
  x_content_type_options: boolean;
  x_xss_protection: boolean;
  content_security_policy?: string;
}

export interface ConfigTestResult {
  valid: boolean;
  message: string;
  error?: string;
  error_line?: number;
}

export interface LoginResponse {
  access_token?: string;
  refresh_token?: string;
  mfa_required?: boolean;
  mfa_token?: string;
}

export interface User {
  id: string;
  org_id: string;
  email: string;
  role: string;
  mfa_enabled: boolean;
}

// Network types for cross-network proxying
export interface DockerNetwork {
  id: string;
  name: string;
  driver: string;
  scope: string;
  internal: boolean;
  containers: Record<string, string>;
}

export interface ContainerNetworkInfo {
  network_id: string;
  network_name: string;
  ip_address: string;
}

export interface NetworkCheckResult {
  connected: boolean;
  network_id: string;
  network_name?: string;
}

export interface NginxNetworkAttachment {
  id: string;
  agent_id: string;
  network_id: string;
  network_name: string;
  attached_at: string;
  attached_by?: string;
  status: "attached" | "detached" | "error";
  error_message?: string;
}

// Extended Docker Resource Types

export interface DockerNetworkDetail extends DockerNetwork {
  attachable: boolean;
  ipam: {
    driver: string;
    configs: Array<{
      subnet: string;
      gateway: string;
      ip_range?: string;
    }>;
  };
  options: Record<string, string>;
  labels: Record<string, string>;
  created_at: string;
}

export interface NetworkEndpoint {
  name: string;
  endpoint_id: string;
  mac_address: string;
  ipv4_address: string;
  ipv6_address?: string;
}

export interface CreateNetworkRequest {
  name: string;
  driver?: string;
  internal?: boolean;
  attachable?: boolean;
  labels?: Record<string, string>;
}

export interface DockerVolume {
  name: string;
  driver: string;
  mountpoint: string;
  scope: string;
  labels: Record<string, string>;
  created_at: string;
  used_by: string[];
}

export interface CreateVolumeRequest {
  name: string;
  driver?: string;
  driver_opts?: Record<string, string>;
  labels?: Record<string, string>;
}

export interface DockerImage {
  id: string;
  tags: string[];
  size: number;
  size_mb: number;
  created: string;
  repo_digests: string[];
  used_by: string[];
}

export interface SecurityHeaders {
  id?: string;
  proxy_host_id?: string;
  hsts_enabled: boolean;
  hsts_max_age: number;
  x_frame_options: string;
  x_content_type_options: boolean;
  x_xss_protection: boolean;
  content_security_policy?: string | null;
}

// Rate limit types
export interface RateLimit {
  id: string;
  proxy_host_id: string;
  zone_name: string;
  requests_per: number;
  time_window: string;
  burst: number;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

// MFA types
export interface MFASetupResponse {
  secret: string;
  otpauth: string;
}

export interface MFAConfirmResponse {
  message: string;
  backup_codes: string[];
}

// Alert types
export interface AlertChannel {
  id: string;
  org_id: string;
  name: string;
  channel_type: "smtp" | "slack" | "webhook" | "discord" | "teams";
  config: Record<string, unknown>;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface AlertRule {
  id: string;
  org_id: string;
  name: string;
  rule_type: string;
  conditions: Record<string, unknown>;
  channels: string[];
  cooldown_mins: number;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface AlertHistoryEntry {
  id: string;
  rule_id?: string;
  rule_name?: string;
  agent_id?: string;
  agent_name?: string;
  triggered_at: string;
  resolved_at?: string;
  severity: string;
  message: string;
  metadata?: Record<string, unknown>;
}

// Log types
export interface LogEntry {
  timestamp: string;
  source: string;
  container_id?: string;
  container_name?: string;
  stream: string;
  level: string;
  message: string;
}

export interface UnifiedLogsResponse {
  logs: LogEntry[];
  count: number;
}

export interface NginxLogsResponse {
  logs: LogEntry[];
  type: string;
  count: number;
}

// Audit types
export interface AuditLogEntry {
  id: string;
  org_id?: string;
  user_id?: string;
  user_email?: string;
  agent_id?: string;
  agent_name?: string;
  action: string;
  resource_type?: string;
  resource_id?: string;
  ip_address?: string;
  user_agent?: string;
  request_body?: Record<string, unknown>;
  created_at: string;
}

export interface AuditLogsResponse {
  logs: AuditLogEntry[];
  total: number;
  limit: number;
  offset: number;
}

// User management types
export interface UserAccount {
  id: string;
  org_id: string;
  email: string;
  role: "super_admin" | "admin" | "operator" | "viewer";
  mfa_enabled: boolean;
  last_login?: string;
  created_at: string;
  updated_at: string;
}

// Health monitoring types
export interface TLSHealthResult {
  domain: string;
  proxy_id: string;
  ssl_enabled: boolean;
  valid: boolean;
  issuer?: string;
  expires_at?: string;
  days_left?: number;
  score: number;
  status: "healthy" | "warning" | "critical" | "expired" | "none";
  error_message?: string;
}

export interface TLSHealthSummary {
  total_proxies: number;
  ssl_enabled: number;
  healthy: number;
  warning: number;
  critical: number;
  expired: number;
  overall_score: number;
  certificates: TLSHealthResult[];
}

export interface DatabaseHealth {
  status: "healthy" | "degraded" | "unhealthy";
  connected: boolean;
  latency_ms: number;
  active_connections: number;
  max_connections: number;
  idle_connections: number;
  database_size?: string;
  table_count: number;
  score: number;
}

export interface SystemHealth {
  status: string;
  uptime: string;
  goroutines: number;
  memory_mb: number;
  cpu_cores: number;
}

// SSL/Certificate types
export interface SSLCertificateInfo {
  exists: boolean;
  domain: string;
  issuer?: string;
  subject?: string;
  expires_at?: string;
  days_left?: number;
  is_wildcard?: boolean;
  valid_for_domain: boolean;
  error?: string;
  sans?: string[];
}

export interface SSLStatus {
  letsencrypt_email: string;
  letsencrypt_staging: boolean;
  account_configured: boolean;
  cert_directory: string;
}

export interface SSLRequestOptions {
  domain: string;
  email?: string;
  dns_provider?: string;
  staging?: boolean;
  force_renew?: boolean;
  ssl_source?: 'letsencrypt' | 'wildcard' | 'external';
  certificate_id?: string;
}

// SSL Certificate Management types
export type SSLSource = 'letsencrypt' | 'wildcard' | 'external' | 'dns_challenge';

export interface SSLCertificateRecord {
  id: string;
  org_id: string;
  name: string;
  domain: string;
  is_wildcard: boolean;
  cert_path: string;
  key_path: string;
  issuer?: string;
  subject?: string;
  san?: string;
  expires_at?: string;
  auto_detected: boolean;
  created_at: string;
  updated_at: string;
}

export interface SSLCertificateScanResult {
  certificates: SSLCertificateRecord[];
  scanned_dir: string;
}

export interface DNSInstructions {
  domain: string;
  server_ip: string;
  records: Array<{
    type: string;
    name: string;
    value: string;
    ttl: number;
  }>;
  instructions: string;
}

export interface DNSVerifyResult {
  domain: string;
  configured: boolean;
  resolved_ips: string[];
  expected_ip: string;
  matches: boolean;
  error?: string;
}

// Setup types
export interface SetupStatusResponse {
  setup_required: boolean;
  license_configured: boolean;
  admin_created: boolean;
  user_count: number;
  license_error?: string;
}

export interface SetupResponse {
  message: string;
  user_id: string;
  access_token?: string;
  refresh_token?: string;
}

export interface SetupLicenseResponse {
  valid: boolean;
  tier: string;
  max_agents: number;
  features: string[];
}

export interface LicenseSettingsResponse {
  valid: boolean;
  tier: string;
  max_agents: number;
  features: string[];
  expires_at: string | null;
  upgrade_url: string;
  key_display: string; // masked key, e.g. "IP-CE-****-****-WG7U"
  key_source: "env" | "database" | "setup_mode" | "offline";
  edition?: "community" | "enterprise"; // the running image
}

export interface UpgradeStatus {
  state: "none" | "switching" | "completed" | "failed" | "unknown";
  tier?: string;
  message?: string;
  error?: string;
  at?: string;
  edition?: "community" | "enterprise";
}

// Matches what GET /api/v1/license actually returns (available to all authenticated users)
export interface LicenseTierInfo {
  valid: boolean;
  tier: string;       // "community" | "professional" | "enterprise"
  max_agents: number;
  features: string[]; // e.g. ["core_monitoring", "basic_alerts", ...]
  expires_at: string | null;
  upgrade_url: string;
}

// System Settings types
export interface InfraPilotDomainSettings {
  domain: string;
  ssl_enabled: boolean;
  force_ssl: boolean;
  http2_enabled: boolean;
  proxy_host_id?: string;
  status?: string;
  ssl_source?: SSLSource;
  ssl_certificate_id?: string;
  ssl_cert_path?: string;
  ssl_key_path?: string;
}

// Default Pages types
export type DefaultPageType = "welcome" | "404" | "500" | "502" | "503" | "maintenance";

export interface DefaultPage {
  id?: string;
  org_id?: string;
  page_type: DefaultPageType;
  enabled: boolean;
  title: string;
  heading: string;
  message: string;
  show_logo: boolean;
  custom_css?: string;
  created_at?: string;
  updated_at?: string;
}

// SSO types
export type SSOProviderType = "saml" | "oidc" | "ldap";

export interface SSOProvider {
  id: string;
  org_id: string;
  name: string;
  provider_type: SSOProviderType;
  enabled: boolean;
  // SAML
  saml_entity_id?: string;
  saml_sso_url?: string;
  saml_slo_url?: string;
  saml_certificate?: string;
  saml_sign_requests?: boolean;
  saml_name_id_format?: string;
  // OIDC
  oidc_issuer?: string;
  oidc_client_id?: string;
  oidc_client_secret?: string;
  oidc_scopes?: string;
  oidc_redirect_uri?: string;
  // LDAP
  ldap_host?: string;
  ldap_port?: number;
  ldap_use_tls?: boolean;
  ldap_skip_verify?: boolean;
  ldap_bind_dn?: string;
  ldap_bind_password?: string;
  ldap_base_dn?: string;
  ldap_user_filter?: string;
  ldap_group_filter?: string;
  ldap_email_attr?: string;
  ldap_name_attr?: string;
  ldap_group_attr?: string;
  // Common
  default_role: string;
  auto_create_users: boolean;
  created_at: string;
  updated_at: string;
}

export interface SSOPublicProvider {
  id: string;
  name: string;
  provider_type: SSOProviderType;
}

export interface SSORoleMapping {
  id: string;
  provider_id: string;
  external_group: string;
  role: string;
  created_at: string;
}

export interface CreateSSOProviderRequest {
  name: string;
  provider_type: SSOProviderType;
  enabled?: boolean;
  // Type-specific fields
  saml_entity_id?: string;
  saml_sso_url?: string;
  saml_certificate?: string;
  oidc_issuer?: string;
  oidc_client_id?: string;
  oidc_client_secret?: string;
  oidc_scopes?: string;
  ldap_host?: string;
  ldap_port?: number;
  ldap_use_tls?: boolean;
  ldap_bind_dn?: string;
  ldap_bind_password?: string;
  ldap_base_dn?: string;
  ldap_user_filter?: string;
  // Common
  default_role?: string;
  auto_create_users?: boolean;
}

// Enterprise Audit & Compliance types
export interface AuditConfig {
  id: string;
  org_id: string;
  retention_days: number;
  retention_policy: "delete" | "archive" | "export";
  forwarding_enabled: boolean;
  forwarding_type?: "syslog" | "webhook" | "splunk" | "s3";
  forwarding_config?: Record<string, unknown>;
  compliance_mode?: "soc2" | "hipaa" | "gdpr" | "pci";
  immutable_logs: boolean;
  hash_chain_enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface AuditExport {
  id: string;
  org_id: string;
  format: "csv" | "json" | "cef" | "syslog";
  status: "pending" | "processing" | "completed" | "failed";
  start_date?: string;
  end_date?: string;
  filters?: Record<string, unknown>;
  row_count?: number;
  file_size?: number;
  download_url?: string;
  expires_at?: string;
  created_at: string;
  completed_at?: string;
}

export interface ComplianceReport {
  id: string;
  org_id: string;
  report_type: "soc2" | "hipaa" | "access" | "activity" | "security";
  status: "pending" | "generating" | "completed" | "failed";
  start_date: string;
  end_date: string;
  summary?: Record<string, unknown>;
  download_url?: string;
  created_at: string;
  completed_at?: string;
}

export interface IntegrityCheckResult {
  status: "valid" | "compromised";
  verified: number;
  broken_chain: number;
  broken_entries?: Array<{
    id: string;
    expected_prev: string;
    actual_prev: string;
  }>;
}

// License types
export type LicenseEdition = "community" | "enterprise";

export interface LicenseLimits {
  max_users: number;
  max_agents: number;
  max_resources: number;
}

export interface LicenseFeatureInfo {
  feature: string;
  name: string;
  description: string;
  licensed: boolean;
}

export interface LicenseInfo {
  edition: LicenseEdition;
  organization: string;
  features: LicenseFeatureInfo[];
  limits: LicenseLimits;
  valid: boolean;
  expires_at: string | null;
}

// Multi-tenancy types
export type OrgPlan = "free" | "pro" | "team" | "enterprise";
export type OrgMemberRole = "owner" | "admin" | "member" | "viewer";

export interface Organization {
  id: string;
  name: string;
  slug: string;
  plan: OrgPlan;
  stripe_customer_id?: string;
  subscription_status?: string;
  max_users: number;
  max_agents: number;
  settings: Record<string, unknown>;
  created_at: string;
  updated_at: string;
  member_role?: OrgMemberRole; // When listing user's orgs
}

export interface OrganizationMember {
  id: string;
  org_id: string;
  user_id: string;
  role: OrgMemberRole;
  invited_by?: string;
  joined_at: string;
  email?: string;
  user_name?: string;
}

export interface OrganizationInvitation {
  id: string;
  org_id: string;
  email: string;
  role: OrgMemberRole;
  token?: string;
  expires_at: string;
  accepted_at?: string;
  created_by?: string;
  created_at: string;
}

export interface EnrollmentToken {
  id: string;
  org_id: string;
  token?: string;
  name?: string;
  created_by?: string;
  expires_at?: string;
  max_uses?: number;
  use_count: number;
  labels: Record<string, unknown>;
  enabled: boolean;
  created_at: string;
  last_used_at?: string;
}

export interface OrgUsage {
  users: number;
  max_users: number;
  agents: number;
  max_agents: number;
}

export interface CreateOrgRequest {
  name: string;
  slug: string;
}

export interface UpdateOrgRequest {
  name?: string;
  max_users?: number;
  max_agents?: number;
  settings?: Record<string, unknown>;
}

export interface CreateInvitationRequest {
  email: string;
  role: OrgMemberRole;
}

export interface CreateEnrollmentTokenRequest {
  name?: string;
  expires_at?: string;
  max_uses?: number;
  labels?: Record<string, unknown>;
}

// Policy Engine types
export type PolicyType = "container" | "proxy" | "access" | "security";
export type PolicyAction = "block" | "warn" | "audit";

export interface Policy {
  id: string;
  org_id: string;
  name: string;
  description?: string;
  policy_type: PolicyType;
  conditions: Record<string, unknown>;
  action: PolicyAction;
  applies_to?: Record<string, unknown>;
  enabled: boolean;
  priority: number;
  created_by?: string;
  created_at: string;
  updated_at: string;
}

export interface PolicyTemplate {
  id: string;
  name: string;
  description?: string;
  policy_type: PolicyType;
  conditions: Record<string, unknown>;
  recommended_action: PolicyAction;
  category?: string;
  created_at: string;
}

export interface PolicyViolation {
  id: string;
  policy_id: string;
  policy_name?: string;
  org_id: string;
  agent_id?: string;
  agent_name?: string;
  resource_type: string;
  resource_id?: string;
  resource_name?: string;
  message: string;
  details?: Record<string, unknown>;
  action_taken: PolicyAction;
  resolved: boolean;
  resolved_by?: string;
  resolved_at?: string;
  resolution_note?: string;
  created_at: string;
}

export interface PolicyStats {
  total_policies: number;
  enabled_policies: number;
  total_violations: number;
  unresolved_violations: number;
  blocked_actions: number;
  warned_actions: number;
}

export interface CreatePolicyRequest {
  name: string;
  description?: string;
  policy_type: PolicyType;
  conditions: Record<string, unknown>;
  action: PolicyAction;
  applies_to?: Record<string, unknown>;
  enabled?: boolean;
  priority?: number;
}

export interface UpdatePolicyRequest {
  name?: string;
  description?: string;
  conditions?: Record<string, unknown>;
  action?: PolicyAction;
  applies_to?: Record<string, unknown>;
  enabled?: boolean;
  priority?: number;
}

// Deployments & Security Scanning
export interface ServiceConfig {
  id: string;
  org_id: string;
  agent_id: string;
  service_name: string;
  environment: string;
  image_repository: string;
  image_tag: string;
  git_repo?: string;
  git_branch?: string;
  webhook_id?: string;
  created_at: string;
  updated_at: string;
  // Runtime info joined from latest deployment
  status?: string;
  current_tag?: string;
  deployed_at?: string;
  container_name?: string;
  container_id?: string;
}

export interface Deployment {
  id: string;
  org_id: string;
  agent_id: string;
  service_name: string;
  environment: string;
  image_registry?: string;
  image_repository: string;
  image_tag?: string;
  image_digest?: string;
  git_repo?: string;
  git_branch?: string;
  git_commit?: string;
  ci_provider?: string;
  ci_pipeline_id?: string;
  ci_build_url?: string;
  scan_result_id?: string;
  sbom_id?: string;
  policy_decision?: "allow" | "warn" | "deny";
  policy_reason?: string;
  status: "pending" | "scanning" | "policy_check" | "deploying" | "running" | "failed" | "rolled_back" | "stopped";
  status_message?: string;
  container_id?: string;
  container_name?: string;
  proxy_host_id?: string;
  replaces_deployment_id?: string;
  rollback_of_deployment_id?: string;
  deployed_by?: string;
  deployed_at?: string;
  created_at: string;
  updated_at: string;
  stack_id?: string;
  service_order?: number;
}

export interface ScanResult {
  id: string;
  org_id?: string;
  image_digest?: string;
  image_repository: string;
  image_tag?: string;
  critical: number;
  high: number;
  medium: number;
  low: number;
  unknown: number;
  total: number;
  fixable: number;
  scanner_name: string;
  scanner_version?: string;
  scan_duration_ms?: number;
  scanned_at: string;
  created_at: string;
}

export interface Vulnerability {
  id: string;
  cve_id: string;
  severity: "critical" | "high" | "medium" | "low" | "unknown";
  package_name: string;
  package_version?: string;
  package_type?: string;
  fixed_version?: string;
  fix_available: boolean;
  title?: string;
  description?: string;
  cvss_score?: number;
  cvss_vector?: string;
  created_at: string;
}

export interface SBOM {
  id: string;
  org_id: string;
  image_digest?: string;
  image_repository: string;
  format: string;
  spec_version: string;
  total_packages: number;
  os_packages: number;
  library_packages: number;
  generator_name: string;
  generator_version?: string;
  created_at: string;
}

// Container configuration for deployments
export interface ContainerConfigSecret {
  name: string;
  value: string;
  mount_path?: string;
}

export interface ContainerConfigNetwork {
  network_id?: string;
  network_name?: string;
  create_if_missing?: boolean;
  driver?: string;
}

export interface ContainerConfigVolume {
  volume_name?: string;
  mount_path: string;
  create_if_missing?: boolean;
  host_path?: string;
  read_only?: boolean;
}

export interface DeploymentContainerConfig {
  env_vars?: Record<string, string>;
  secrets?: ContainerConfigSecret[];
  secret_method?: "env_vars" | "docker_secrets";
  networks?: ContainerConfigNetwork[];
  volumes?: ContainerConfigVolume[];
}

export interface CreateDeploymentRequest {
  service_name: string;
  environment: string;
  image_repository: string;
  image_tag?: string;
  image_digest?: string;
  git_repo?: string;
  git_branch?: string;
  git_commit?: string;
  ci_provider?: string;
  ci_pipeline_id?: string;
  ci_build_url?: string;
  container_config?: DeploymentContainerConfig;
}

// Type aliases for backward compatibility in DeployWizard
export type NetworkInfo = DockerNetwork;
export type VolumeInfo = DockerVolume;

// Developer Feedback (Epic 8)
export interface Feedback {
  id: string;
  org_id: string;
  source_type: "vulnerability" | "policy_violation" | "sbom" | "drift" | "code_quality";
  source_id: string;
  provider: "github" | "gitlab" | "bitbucket" | "azure_devops";
  repo_full_name: string;
  pull_request_number?: number;
  commit_sha?: string;
  branch_name?: string;
  severity?: string;
  title: string;
  message: string;
  remediation?: string;
  metadata?: Record<string, any>;
  delivery_status: "pending" | "delivered" | "failed" | "skipped";
  delivered_at?: string;
  delivery_error?: string;
  external_id?: string;
  created_at: string;
  updated_at: string;
}

export interface VCSConfiguration {
  id: string;
  org_id: string;
  provider: "github" | "gitlab" | "bitbucket" | "azure_devops";
  enabled: boolean;
  auth_type: string;
  app_id?: string;
  installation_id?: string;
  default_repo?: string;
  auto_comment_on_pr: boolean;
  auto_comment_on_commit: boolean;
  created_at: string;
  updated_at: string;
}

export interface SaveVCSConfigRequest {
  provider: string;
  enabled: boolean;
  token?: string;
  default_repo?: string;
  auto_comment_on_pr: boolean;
  auto_comment_on_commit: boolean;
}

// Risk Exceptions (Epic 9)
export interface RiskException {
  id: string;
  org_id: string;
  scope_type: "cve" | "policy_rule" | "deployment" | "package" | "image";
  scope_reference: string;
  deployment_id?: string;
  scan_result_id?: string;
  requested_by: string;
  justification: string;
  business_impact?: string;
  mitigation_plan?: string;
  status: "pending" | "approved" | "denied" | "expired" | "revoked";
  approved_by?: string;
  approved_at?: string;
  denial_reason?: string;
  expires_at: string;
  auto_renewed: boolean;
  renewal_count: number;
  revoked_at?: string;
  revoked_by?: string;
  revoked_reason?: string;
  tags?: string[];
  created_at: string;
  updated_at: string;
}

export interface CreateExceptionRequest {
  scope_type: string;
  scope_reference: string;
  deployment_id?: string;
  scan_result_id?: string;
  justification: string;
  business_impact?: string;
  mitigation_plan?: string;
  duration_days: number;
  tags?: string[];
}

export interface ApproveExceptionRequest {
  comment?: string;
}

export interface DenyExceptionRequest {
  reason: string;
}

export interface RevokeExceptionRequest {
  reason: string;
}

// Service Ownership (Epic 10)
export interface ServiceOwnership {
  id: string;
  org_id: string;
  service_name: string;
  team_name: string;
  team_email?: string;
  team_slack_channel?: string;
  team_slack_webhook?: string;
  primary_contact_id?: string;
  secondary_contact_id?: string;
  primary_contact_email?: string;
  secondary_contact_email?: string;
  description?: string;
  repository_url?: string;
  documentation_url?: string;
  oncall_url?: string;
  tags?: string[];
  status: string;
  created_at: string;
  updated_at: string;
}

export interface Team {
  id: string;
  org_id: string;
  name: string;
  display_name?: string;
  email?: string;
  slack_channel?: string;
  slack_webhook?: string;
  pagerduty_integration_key?: string;
  description?: string;
  manager_id?: string;
  tags?: string[];
  active: boolean;
  created_at: string;
  updated_at: string;
}

export interface CreateServiceOwnershipRequest {
  service_name: string;
  team_name: string;
  team_email?: string;
  team_slack_channel?: string;
  team_slack_webhook?: string;
  primary_contact_email?: string;
  secondary_contact_email?: string;
  description?: string;
  repository_url?: string;
  documentation_url?: string;
  oncall_url?: string;
  tags?: string[];
  status?: string;
}

export interface CreateTeamRequest {
  name: string;
  display_name?: string;
  email?: string;
  slack_channel?: string;
  slack_webhook?: string;
  pagerduty_integration_key?: string;
  description?: string;
  tags?: string[];
}

// Security Maturity (Epic 11)
export interface SecurityScore {
  id: string;
  org_id: string;
  scope_type: string;
  scope_reference: string;
  overall_score: number;
  vulnerability_score?: number;
  policy_score?: number;
  deployment_score?: number;
  exception_score?: number;
  response_score?: number;
  total_deployments: number;
  vulnerable_deployments: number;
  critical_vulnerabilities: number;
  high_vulnerabilities: number;
  policy_violations: number;
  active_exceptions: number;
  mean_time_to_fix_hours?: number;
  calculated_at: string;
  period_start: string;
  period_end: string;
  created_at: string;
}

export interface TeamScore {
  org_id: string;
  team_name: string;
  overall_score: number;
  vulnerability_score?: number;
  policy_score?: number;
  deployment_score?: number;
  exception_score?: number;
  response_score?: number;
  total_deployments: number;
  vulnerable_deployments: number;
  critical_vulnerabilities: number;
  high_vulnerabilities: number;
  mean_time_to_fix_hours?: number;
  calculated_at: string;
}

export interface TeamLeaderboard {
  org_id: string;
  team_name: string;
  overall_score: number;
  rank: number;
  vulnerability_score?: number;
  policy_score?: number;
  response_score?: number;
  calculated_at: string;
}

export interface ScoreTrendPoint {
  id: string;
  overall_score: number;
  vulnerability_score?: number;
  policy_score?: number;
  response_score?: number;
  calculated_at: string;
  period_start: string;
  period_end: string;
}

export interface CalculateScoreRequest {
  scope_type: string;
  scope_reference: string;
  period_days?: number;
}

// Code Quality (Epic 12)
export interface CodeQualityResult {
  id: string;
  org_id: string;
  deployment_id?: string;
  tool: string;
  tool_version?: string;
  project_key: string;
  project_name?: string;
  git_repo?: string;
  git_branch?: string;
  commit_sha?: string;
  commit_author?: string;
  bugs: number;
  vulnerabilities: number;
  code_smells: number;
  security_hotspots: number;
  critical_issues: number;
  high_issues: number;
  medium_issues: number;
  low_issues: number;
  info_issues: number;
  coverage?: number;
  complexity?: number;
  duplication?: number;
  technical_debt_minutes?: number;
  quality_gate?: string;
  quality_gate_details?: string;
  scan_duration_ms?: number;
  lines_of_code?: number;
  files_analyzed?: number;
  raw_report?: any;
  created_at: string;
  updated_at: string;
}

export interface CodeQualityIssue {
  id: string;
  code_quality_result_id: string;
  issue_key?: string;
  rule_id: string;
  rule_name?: string;
  severity: string;
  issue_type: string;
  file_path: string;
  start_line?: number;
  end_line?: number;
  start_column?: number;
  end_column?: number;
  message: string;
  description?: string;
  cwe_ids?: string[];
  owasp_category?: string;
  confidence?: string;
  fix_available: boolean;
  fix_suggestion?: string;
  metadata?: any;
  created_at: string;
}

export interface CodeQualityPolicy {
  id: string;
  org_id: string;
  name: string;
  description?: string;
  environments: string[];
  projects: string[];
  tools: string[];
  max_critical?: number;
  max_high?: number;
  max_medium?: number;
  max_vulnerabilities?: number;
  max_bugs?: number;
  max_code_smells?: number;
  min_coverage?: number;
  max_complexity?: number;
  max_duplication?: number;
  enforcement: string;
  block_deployment: boolean;
  version: number;
  enabled: boolean;
  created_by?: string;
  created_at: string;
  updated_at: string;
}

export interface PolicyEvaluation {
  id: string;
  code_quality_result_id: string;
  policy_id?: string;
  decision: string;
  reason?: string;
  violations?: any;
  evaluation_time_ms?: number;
  created_at: string;
}

export interface ProjectQualitySummary {
  org_id: string;
  project_key: string;
  project_name?: string;
  tool: string;
  total_scans: number;
  avg_coverage?: number;
  avg_complexity?: number;
  avg_duplication?: number;
  total_bugs: number;
  total_vulnerabilities: number;
  total_code_smells: number;
  total_critical: number;
  total_high: number;
  failed_gates: number;
  passed_gates: number;
  last_scan_at: string;
}

export interface QualityLeaderboard {
  org_id: string;
  project_key: string;
  project_name?: string;
  tool: string;
  coverage?: number;
  complexity?: number;
  total_issues: number;
  critical_issues: number;
  high_issues: number;
  quality_gate?: string;
  last_scan_at: string;
  quality_score: number;
}

export interface CreateCodeQualityResultRequest {
  deployment_id?: string;
  tool: string;
  tool_version?: string;
  project_key: string;
  project_name?: string;
  git_repo?: string;
  git_branch?: string;
  commit_sha?: string;
  commit_author?: string;
  bugs: number;
  vulnerabilities: number;
  code_smells: number;
  security_hotspots: number;
  critical_issues: number;
  high_issues: number;
  medium_issues: number;
  low_issues: number;
  info_issues: number;
  coverage?: number;
  complexity?: number;
  duplication?: number;
  technical_debt_minutes?: number;
  quality_gate?: string;
  quality_gate_details?: string;
  scan_duration_ms?: number;
  lines_of_code?: number;
  files_analyzed?: number;
  raw_report?: any;
  issues?: CreateCodeQualityIssueRequest[];
}

export interface CreateCodeQualityIssueRequest {
  issue_key?: string;
  rule_id: string;
  rule_name?: string;
  severity: string;
  issue_type: string;
  file_path: string;
  start_line?: number;
  end_line?: number;
  start_column?: number;
  end_column?: number;
  message: string;
  description?: string;
  cwe_ids?: string[];
  owasp_category?: string;
  confidence?: string;
  fix_available: boolean;
  fix_suggestion?: string;
  metadata?: any;
}

// Platform Security (Epic 6)
export interface PlatformSecurityConfig {
  id: string;
  org_id: string;
  mfa_required_for_admins: boolean;
  docker_socket_read_only: boolean;
  tls_only: boolean;
  min_password_length: number;
  password_require_uppercase: boolean;
  password_require_lowercase: boolean;
  password_require_numbers: boolean;
  password_require_special: boolean;
  session_timeout_minutes: number;
  audit_logging_enabled: boolean;
  audit_logging_immutable: boolean;
  block_privileged_containers: boolean;
  block_docker_socket_mounts: boolean;
  block_docker_api_exposure: boolean;
  block_root_containers: boolean;
  block_host_network: boolean;
  block_host_pid: boolean;
  block_host_ipc: boolean;
  block_capability_additions: boolean;
  track_violation_attempts: boolean;
  alert_on_violations: boolean;
  max_violations_before_lockout?: number;
  created_at: string;
  updated_at: string;
}

export interface PolicyViolation {
  id: string;
  org_id: string;
  violation_type: string;
  severity: string;
  user_id?: string;
  user_email?: string;
  ip_address?: string;
  user_agent?: string;
  attempted_action: string;
  blocked: boolean;
  policy_rule?: string;
  description: string;
  request_payload?: any;
  acknowledged: boolean;
  acknowledged_by?: string;
  acknowledged_at?: string;
  notes?: string;
  created_at: string;
}

export interface SecurityCheck {
  check_name: string;
  status: string;
  severity: string;
  message: string;
  recommendation?: string;
}

export interface SecurityPosture {
  org_id: string;
  total_checks: number;
  passed_checks: number;
  failed_checks: number;
  warning_checks: number;
  critical_failures: number;
  high_failures: number;
  last_check_at: string;
  overall_status: string;
}

export interface ViolationSummary {
  org_id: string;
  violation_type: string;
  violation_count: number;
  blocked_count: number;
  allowed_count: number;
  max_severity: string;
  last_violation_at: string;
  unique_users: number;
}

// Runtime Security (Epic 3)
export interface DriftEvent {
  id: string;
  org_id: string;
  agent_id: string;
  deployment_id: string;
  container_id: string;
  container_name: string;
  drift_type: string;
  severity: string;
  description: string;
  expected_state: any;
  actual_state: any;
  diff?: any;
  resolved: boolean;
  resolved_at?: string;
  resolved_by?: string;
  resolution_notes?: string;
  detected_at: string;
  created_at: string;
}

export interface BehavioralAnomaly {
  id: string;
  org_id: string;
  agent_id: string;
  deployment_id?: string;
  container_id?: string;
  container_name?: string;
  anomaly_type: string;
  severity: string;
  description: string;
  metrics: any;
  threshold: any;
  resolved: boolean;
  resolved_at?: string;
  resolved_by?: string;
  resolution_notes?: string;
  detected_at: string;
  first_seen_at: string;
  occurrence_count: number;
  created_at: string;
}

export interface RuntimeSecurityConfig {
  id: string;
  org_id: string;
  drift_detection_enabled: boolean;
  behavioral_monitoring_enabled: boolean;
  auto_resolve_info_drift: boolean;
  auto_resolve_after_hours: number;
  alert_on_critical_drift: boolean;
  alert_on_high_drift: boolean;
  alert_on_critical_anomaly: boolean;
  alert_on_high_anomaly: boolean;
  crash_loop_threshold: number;
  crash_loop_window_minutes: number;
  log_volume_spike_multiplier: number;
  error_rate_spike_multiplier: number;
  cpu_exhaustion_threshold_percent: number;
  mem_exhaustion_threshold_percent: number;
  network_spike_multiplier: number;
  process_spawn_threshold: number;
  created_at: string;
  updated_at: string;
}

export interface ContainerDriftSummary {
  container_name: string;
  deployment_id: string;
  total_drift: number;
  unresolved_drift: number;
  last_drift_at: string;
}

export interface RuntimeSecurityPosture {
  overall_status: string;
  drift_events_last_24h: number;
  anomalies_last_24h: number;
  unresolved_drift: number;
  unresolved_anomalies: number;
  critical_drift: number;
  critical_anomalies: number;
  drift_by_type: Record<string, number>;
  anomaly_by_type: Record<string, number>;
  top_affected_containers: ContainerDriftSummary[];
  recent_events: any[];
}

// Container Registry Types
export type RegistryProvider = 'ghcr' | 'dockerhub' | 'ecr' | 'gcr' | 'acr';
export type RegistryConnectionStatus = 'pending' | 'connected' | 'failed';

export interface ContainerRegistry {
  id: string;
  name: string;
  provider: RegistryProvider;
  enabled: boolean;
  namespace?: string;
  connection_status: RegistryConnectionStatus;
  connection_error?: string;
  last_connected_at?: string;
  created_at: string;
  updated_at: string;
}

export interface CreateRegistryRequest {
  name: string;
  provider: RegistryProvider;
  namespace?: string;
  // GHCR
  token?: string;
  // Docker Hub
  username?: string;
  password?: string;
  // ECR
  aws_access_key_id?: string;
  aws_secret_access_key?: string;
  aws_region?: string;
  aws_account_id?: string;
  // GCR
  gcp_service_account_json?: string;
  gcp_project_id?: string;
  // ACR
  azure_tenant_id?: string;
  azure_client_id?: string;
  azure_client_secret?: string;
  azure_registry_name?: string;
}

export interface UpdateRegistryRequest {
  name?: string;
  enabled?: boolean;
  namespace?: string;
  token?: string;
  username?: string;
  password?: string;
}

export interface RegistryRepository {
  name: string;
  full_name: string;
  description?: string;
  is_private: boolean;
  pull_count: number;
  updated_at?: string;
}

export interface RegistryTag {
  name: string;
  digest?: string;
  size_bytes: number;
  pushed_at?: string;
}

export interface ListRepositoriesResponse {
  repositories: RegistryRepository[];
  total_count: number;
  page: number;
  page_size: number;
}

export interface ListTagsResponse {
  tags: RegistryTag[];
  total_count: number;
  page: number;
  page_size: number;
}

export interface TestConnectionResponse {
  success: boolean;
  message?: string;
  error?: string;
}

// Epic 13: Traffic & Exposure Governance Types
export interface ExposedEndpoint {
  id: string;
  org_id: string;
  agent_id: string;
  proxy_host_id?: string;
  deployment_id?: string;
  domain: string;
  path: string;
  port: number;
  protocol: string;
  exposure_type: 'public' | 'internal' | 'private' | 'partner';
  authentication_required: boolean;
  authentication_type?: string;
  risk_score: number;
  risk_factors: Record<string, unknown>;
  last_risk_assessment?: string;
  avg_requests_per_hour?: number;
  peak_requests_per_hour?: number;
  unique_clients_24h?: number;
  last_accessed_at?: string;
  service_name?: string;
  service_owner?: string;
  description?: string;
  tags: string[];
  status: 'active' | 'inactive' | 'deprecated' | 'blocked';
  discovered_at: string;
  created_at: string;
  updated_at: string;
}

export interface RateLimitProfile {
  id: string;
  org_id: string;
  name: string;
  description?: string;
  requests_per_second?: number;
  requests_per_minute?: number;
  requests_per_hour?: number;
  burst_size: number;
  key_type: 'ip' | 'api_key' | 'user' | 'custom';
  custom_key_header?: string;
  exceeded_action: 'reject' | 'throttle' | 'queue';
  exceeded_status_code: number;
  exceeded_message: string;
  is_default: boolean;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface RateLimitAssignment {
  id: string;
  org_id: string;
  profile_id: string;
  endpoint_id?: string;
  proxy_host_id?: string;
  domain_pattern?: string;
  requests_per_second?: number;
  requests_per_minute?: number;
  burst_size?: number;
  priority: number;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface TLSAlertConfig {
  id: string;
  org_id: string;
  expiry_warning_days: number;
  expiry_critical_days: number;
  alert_on_weak_cipher: boolean;
  alert_on_protocol_downgrade: boolean;
  alert_on_cert_mismatch: boolean;
  alert_on_self_signed: boolean;
  alert_on_expired: boolean;
  notification_channels: string[];
  auto_scan_enabled: boolean;
  scan_interval_hours: number;
  last_scan_at?: string;
  created_at: string;
  updated_at: string;
}

export interface TLSScanResult {
  id: string;
  org_id: string;
  endpoint_id?: string;
  proxy_host_id?: string;
  domain: string;
  cert_valid: boolean;
  cert_issuer?: string;
  cert_subject?: string;
  cert_expires_at?: string;
  cert_days_left?: number;
  cert_san: string[];
  is_self_signed: boolean;
  protocol_version?: string;
  cipher_suite?: string;
  key_exchange?: string;
  is_weak_cipher: boolean;
  hsts_enabled: boolean;
  hsts_max_age?: number;
  hsts_preload: boolean;
  grade?: string;
  score?: number;
  issues: Record<string, unknown>;
  scanned_at: string;
  created_at: string;
}

export interface ExposureSummary {
  org_id: string;
  total_endpoints: number;
  public_endpoints: number;
  internal_endpoints: number;
  unauthenticated_public: number;
  avg_risk_score: number;
  high_risk_endpoints: number;
  active_endpoints: number;
}

export interface TLSPosture {
  org_id: string;
  total_domains: number;
  valid_certs: number;
  invalid_certs: number;
  self_signed: number;
  weak_ciphers: number;
  expiring_critical: number;
  expiring_warning: number;
  avg_score: number;
}

export interface ExposureMap {
  endpoints: ExposedEndpoint[];
  connection_matrix: Array<{
    source: string;
    target: string;
    protocol: string;
    port: number;
    encrypted: boolean;
  }>;
  external_domains: string[];
  internal_services: string[];
}

export interface CreateExposedEndpointRequest {
  agent_id: string;
  proxy_host_id?: string;
  deployment_id?: string;
  domain: string;
  path?: string;
  port: number;
  protocol?: string;
  exposure_type: 'public' | 'internal' | 'private' | 'partner';
  authentication_required?: boolean;
  authentication_type?: string;
  service_name?: string;
  service_owner?: string;
  description?: string;
  tags?: string[];
}

export interface CreateRateLimitProfileRequest {
  name: string;
  description?: string;
  requests_per_second?: number;
  requests_per_minute?: number;
  requests_per_hour?: number;
  burst_size?: number;
  key_type?: 'ip' | 'api_key' | 'user' | 'custom';
  custom_key_header?: string;
  exceeded_action?: 'reject' | 'throttle' | 'queue';
  exceeded_status_code?: number;
  exceeded_message?: string;
  is_default?: boolean;
  enabled?: boolean;
}

// Epic 13 v2: Traffic Resources & Policies Types
export interface PathConfig {
  path: string;
  match: 'prefix' | 'exact' | 'regex';
}

export interface TrafficResource {
  id: string;
  org_id: string;
  agent_id: string;
  name: string;
  description?: string;
  domains: string[];
  paths: PathConfig[];
  gateway_type: 'system' | 'application';
  status: 'draft' | 'pending' | 'active' | 'disabled' | 'error';
  desired_state?: Record<string, unknown>;
  applied_state?: Record<string, unknown>;
  last_applied_at?: string;
  last_error?: string;
  labels: Record<string, string>;
  annotations: Record<string, string>;
  created_at: string;
  updated_at: string;
}

export interface UpstreamTarget {
  id?: string;
  address: string;
  port: number;
  weight: number;
  max_fails: number;
  fail_timeout_sec: number;
  is_healthy: boolean;
  last_health_check?: string;
  consecutive_failures: number;
  last_error?: string;
  enabled: boolean;
}

export interface TrafficUpstream {
  id: string;
  traffic_resource_id: string;
  name: string;
  targets: UpstreamTarget[];
  lb_method: 'round_robin' | 'least_conn' | 'ip_hash' | 'random';
  health_check_enabled: boolean;
  health_check_path: string;
  health_check_interval_sec: number;
  health_check_timeout_sec: number;
  healthy_threshold: number;
  unhealthy_threshold: number;
  max_connections?: number;
  max_keepalive: number;
  connect_timeout_sec: number;
  read_timeout_sec: number;
  send_timeout_sec: number;
  last_health_check?: string;
  healthy_targets: number;
  total_targets: number;
  created_at: string;
  updated_at: string;
}

export type TrafficPolicyType =
  | 'rate_limit'
  | 'bot_protection'
  | 'geo_blocking'
  | 'captcha'
  | 'ip_filtering'
  | 'header_transform'
  | 'cors'
  | 'authentication';

export interface TrafficPolicy {
  id: string;
  org_id: string;
  name: string;
  description?: string;
  policy_type: TrafficPolicyType;
  config: Record<string, unknown>;
  is_global: boolean;
  priority: number;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface TrafficPolicyAssignment {
  id: string;
  policy_id: string;
  traffic_resource_id?: string;
  path_pattern?: string;
  config_override?: Record<string, unknown>;
  priority: number;
  enabled: boolean;
  created_at: string;
}

export interface TrafficTLSConfig {
  id: string;
  traffic_resource_id: string;
  cert_source: 'auto' | 'manual' | 'acme';
  cert_id?: string;
  min_version: string;
  max_version: string;
  cipher_suites?: string[];
  prefer_server_ciphers: boolean;
  hsts_enabled: boolean;
  hsts_max_age: number;
  hsts_include_subdomains: boolean;
  hsts_preload: boolean;
  ocsp_stapling: boolean;
  mtls_enabled: boolean;
  mtls_ca_cert?: string;
  mtls_verify_depth: number;
  mtls_verify_client: 'off' | 'optional' | 'require';
  created_at: string;
  updated_at: string;
}

export interface TrafficApplyHistory {
  id: string;
  traffic_resource_id: string;
  action: 'create' | 'update' | 'delete' | 'rollback' | 'apply' | 'validate';
  previous_state?: Record<string, unknown>;
  new_state?: Record<string, unknown>;
  dry_run_result?: Record<string, unknown>;
  validation_passed?: boolean;
  validation_errors?: string[];
  applied_by?: string;
  applied_at: string;
  apply_duration_ms?: number;
  success?: boolean;
  error_message?: string;
  rendered_nginx_config?: string;
  config_hash?: string;
}

export interface ApplyTrafficResponse {
  success: boolean;
  resource_id: string;
  resource_name?: string;
  action: 'apply' | 'dry_run' | 'rollback';
  validation_passed: boolean;
  validation_output?: string;
  warnings?: string[];
  errors?: string[];
  config_hash?: string;
  applied_at?: string;
  duration_ms: number;
  nginx_config?: string;
  diff?: {
    has_changes: boolean;
    current?: string;
    proposed?: string;
  };
}

export interface TrafficConfigPreview {
  resource_id: string;
  resource_name: string;
  nginx_config: string;
  config_hash: string;
  domains: string[];
  upstreams: TrafficUpstream[];
  policies: TrafficPolicy[];
  tls_config?: TrafficTLSConfig;
}

export interface TrafficResourceSummary {
  org_id: string;
  total_resources: number;
  active_resources: number;
  draft_resources: number;
  error_resources: number;
  system_gateways: number;
  application_gateways: number;
  agents_with_traffic: number;
}

export interface CreateTrafficResourceRequest {
  agent_id: string;
  name: string;
  description?: string;
  domains: string[];
  paths?: PathConfig[];
  gateway_type?: 'system' | 'application';
  labels?: Record<string, string>;
  annotations?: Record<string, string>;
}

export interface CreateTrafficPolicyRequest {
  name: string;
  description?: string;
  policy_type: TrafficPolicyType;
  config: Record<string, unknown>;
  is_global?: boolean;
  priority?: number;
  enabled?: boolean;
}

export interface CreateTrafficUpstreamRequest {
  name: string;
  targets?: UpstreamTarget[];
  lb_method?: 'round_robin' | 'least_conn' | 'ip_hash' | 'random';
  health_check_enabled?: boolean;
  health_check_path?: string;
  health_check_interval_sec?: number;
  health_check_timeout_sec?: number;
  healthy_threshold?: number;
  unhealthy_threshold?: number;
  max_connections?: number;
  max_keepalive?: number;
  connect_timeout_sec?: number;
  read_timeout_sec?: number;
  send_timeout_sec?: number;
}

export interface AssignTrafficPolicyRequest {
  traffic_resource_id: string;
  path_pattern?: string;
  config_override?: Record<string, unknown>;
  priority?: number;
  enabled?: boolean;
}

// Epic 14: Database & Data Governance Types
export interface DatabaseInventoryItem {
  id: string;
  org_id: string;
  agent_id: string;
  deployment_id?: string;
  name: string;
  db_type: 'postgresql' | 'mysql' | 'mongodb' | 'redis' | 'elasticsearch';
  version?: string;
  container_id?: string;
  container_name?: string;
  host?: string;
  port?: number;
  database_name?: string;
  data_classification: 'public' | 'internal' | 'confidential' | 'restricted';
  environment: 'production' | 'staging' | 'development' | 'testing';
  is_primary: boolean;
  is_replica: boolean;
  replica_of?: string;
  pii_data: boolean;
  pci_data: boolean;
  hipaa_data: boolean;
  gdpr_relevant: boolean;
  size_bytes?: number;
  table_count?: number;
  active_connections?: number;
  max_connections?: number;
  status: 'healthy' | 'degraded' | 'unhealthy' | 'offline';
  last_health_check?: string;
  replication_lag_seconds?: number;
  owner_team?: string;
  owner_contact?: string;
  description?: string;
  tags: string[];
  discovered_at: string;
  auto_discovered: boolean;
  created_at: string;
  updated_at: string;
}

export interface DatabaseBackup {
  id: string;
  org_id: string;
  database_id: string;
  backup_type: 'full' | 'incremental' | 'differential' | 'snapshot' | 'wal';
  backup_name?: string;
  backup_path?: string;
  size_bytes?: number;
  compressed_size_bytes?: number;
  compression_ratio?: number;
  encryption_enabled: boolean;
  encryption_algorithm?: string;
  started_at: string;
  completed_at?: string;
  duration_seconds?: number;
  status: 'running' | 'completed' | 'failed' | 'cancelled' | 'verified';
  error_message?: string;
  retention_days: number;
  expires_at?: string;
  is_offsite: boolean;
  offsite_location?: string;
  last_verified_at?: string;
  verification_status?: 'verified' | 'failed' | 'pending';
  verification_notes?: string;
  recovery_point_objective_minutes?: number;
  recovery_time_objective_minutes?: number;
  can_point_in_time_restore: boolean;
  earliest_restore_point?: string;
  latest_restore_point?: string;
  created_at: string;
}

export interface DatabasePosture {
  org_id: string;
  total_databases: number;
  healthy_databases: number;
  degraded_databases: number;
  unhealthy_databases: number;
  pii_databases: number;
  restricted_databases: number;
  replica_count: number;
  total_size_bytes?: number;
}

export interface BackupCompliance {
  org_id: string;
  database_id: string;
  database_name: string;
  db_type: string;
  data_classification: string;
  last_backup_at?: string;
  backups_last_24h: number;
  backups_last_7d: number;
  compliance_status: 'compliant' | 'warning' | 'non_compliant';
}

export interface CreateDatabaseRequest {
  agent_id: string;
  deployment_id?: string;
  name: string;
  db_type: 'postgresql' | 'mysql' | 'mongodb' | 'redis' | 'elasticsearch';
  version?: string;
  container_id?: string;
  container_name?: string;
  host?: string;
  port?: number;
  database_name?: string;
  data_classification?: 'public' | 'internal' | 'confidential' | 'restricted';
  environment?: 'production' | 'staging' | 'development' | 'testing';
  is_primary?: boolean;
  is_replica?: boolean;
  replica_of?: string;
  pii_data?: boolean;
  pci_data?: boolean;
  hipaa_data?: boolean;
  gdpr_relevant?: boolean;
  owner_team?: string;
  owner_contact?: string;
  description?: string;
  tags?: string[];
}

// Agent-scoped database types
export interface AgentDatabase {
  id: string;
  name: string;
  db_type: string;
  version?: string;
  host?: string;
  port?: number;
  status: string;
  environment: string;
  created_at: string;
}

export interface AddAgentDatabaseRequest {
  name: string;
  db_type: string;
  host?: string;
  port?: number;
  environment?: string;
  version?: string;
}

export interface AgentDatabaseMetrics {
  database_id: string;
  name: string;
  db_type: string;
  status: string;
  environment: string;
  version?: string;
  size_bytes: number;
  connection_count: number;
  max_connections: number;
}

// Epic 15: Backup & Recovery Types
export interface BackupJob {
  id: string;
  org_id: string;
  policy_id?: string;
  database_id?: string;
  job_type: 'scheduled' | 'manual' | 'emergency';
  triggered_by?: string;
  status: 'pending' | 'running' | 'completed' | 'failed' | 'cancelled' | 'skipped';
  progress_percent: number;
  current_step?: string;
  scheduled_at?: string;
  started_at?: string;
  completed_at?: string;
  duration_seconds?: number;
  backup_id?: string;
  error_message?: string;
  error_code?: string;
  retry_count: number;
  max_retries: number;
  created_at: string;
  updated_at: string;
  database_name?: string;
  policy_name?: string;
}

export interface RestoreOperation {
  id: string;
  org_id: string;
  backup_id: string;
  source_database_id: string;
  target_type: 'existing' | 'new' | 'test_environment';
  target_database_id?: string;
  target_name?: string;
  target_host?: string;
  target_port?: number;
  restore_type: 'full' | 'partial' | 'point_in_time';
  point_in_time_target?: string;
  triggered_by_email?: string;
  reason?: string;
  status: 'pending' | 'running' | 'completed' | 'failed' | 'cancelled' | 'rollback';
  progress_percent: number;
  current_step?: string;
  started_at?: string;
  completed_at?: string;
  duration_seconds?: number;
  rows_restored?: number;
  tables_restored?: number;
  size_restored_bytes?: number;
  error_message?: string;
  created_at: string;
  updated_at: string;
  source_database_name?: string;
  backup_name?: string;
}

export interface RecoveryTest {
  id: string;
  org_id: string;
  database_id: string;
  backup_id?: string;
  test_type: 'restore' | 'failover' | 'switchover' | 'full_dr';
  test_name?: string;
  test_scenario?: string;
  triggered_by_email?: string;
  status: 'pending' | 'running' | 'passed' | 'failed' | 'cancelled';
  scheduled_at?: string;
  started_at?: string;
  completed_at?: string;
  actual_rto_seconds?: number;
  expected_rto_seconds?: number;
  actual_rpo_seconds?: number;
  expected_rpo_seconds?: number;
  rto_met?: boolean;
  rpo_met?: boolean;
  data_integrity_check?: boolean;
  error_message?: string;
  notes?: string;
  created_at: string;
  updated_at: string;
  database_name?: string;
}

export interface BackupAlert {
  id: string;
  org_id: string;
  database_id?: string;
  backup_id?: string;
  policy_id?: string;
  alert_type: 'backup_failed' | 'backup_missed' | 'storage_warning' | 'retention_expiring' | 'verification_failed' | 'rpo_breach';
  severity: 'info' | 'warning' | 'critical';
  title: string;
  message: string;
  status: 'active' | 'acknowledged' | 'resolved' | 'snoozed';
  acknowledged_at?: string;
  resolved_at?: string;
  created_at: string;
  database_name?: string;
}

export interface BackupStorage {
  id: string;
  org_id: string;
  name: string;
  storage_type: 'local' | 's3' | 'gcs' | 'azure_blob' | 'nfs';
  location: string;
  is_primary: boolean;
  is_offsite: boolean;
  encryption_enabled: boolean;
  compression_enabled: boolean;
  total_capacity_bytes?: number;
  used_bytes: number;
  warning_threshold_percent: number;
  critical_threshold_percent: number;
  status: 'healthy' | 'degraded' | 'unreachable' | 'full';
  last_health_check?: string;
  backup_count: number;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface BackupOverview {
  databases_with_backups: number;
  total_backups: number;
  successful_backups: number;
  failed_backups: number;
  backups_last_24h: number;
  backups_last_7d: number;
  total_backup_size_bytes: number;
  last_backup_at?: string;
  avg_backup_duration_seconds?: number;
}

export interface RecoveryReadiness {
  database_id: string;
  database_name: string;
  db_type: string;
  data_classification: string;
  last_successful_backup?: string;
  last_successful_test?: string;
  last_rto_seconds?: number;
  last_rpo_seconds?: number;
  tests_last_90d: number;
  readiness_status: 'ready' | 'untested' | 'no_backup';
}

export interface TriggerBackupRequest {
  database_id: string;
  backup_type?: string;
  reason?: string;
}

export interface TriggerRestoreRequest {
  backup_id: string;
  target_type: 'existing' | 'new' | 'test_environment';
  target_database_id?: string;
  target_name?: string;
  restore_type?: string;
  point_in_time_target?: string;
  reason?: string;
}

export interface TriggerRecoveryTestRequest {
  database_id: string;
  test_type?: string;
  test_name?: string;
  test_scenario?: string;
}

export interface CreateBackupStorageRequest {
  name: string;
  storage_type: 'local' | 's3' | 'gcs' | 'azure_blob' | 'nfs';
  location: string;
  is_primary?: boolean;
  is_offsite?: boolean;
  encryption_enabled?: boolean;
  compression_enabled?: boolean;
  total_capacity_bytes?: number;
  warning_threshold_percent?: number;
  critical_threshold_percent?: number;
}

// Epic 16: Secrets Hygiene Types
export interface SecretInventory {
  id: string;
  org_id: string;
  agent_id?: string;
  name: string;
  secret_type: 'api_key' | 'password' | 'certificate' | 'ssh_key' | 'oauth_token' | 'database_credential' | 'encryption_key';
  source: 'env_var' | 'env_file' | 'file' | 'file_secret' | 'vault' | 'k8s_secret' | 'docker_secret' | 'config' | 'environment';
  location?: string;
  deployment_id?: string;
  container_id?: string;
  container_name?: string;
  service_name?: string;
  classification: 'public' | 'internal' | 'confidential' | 'restricted';
  environment: string;
  is_sensitive: boolean;
  age_days?: number;
  created_date?: string;
  last_rotated_at?: string;
  rotation_policy_days: number;
  is_expired: boolean;
  needs_rotation: boolean;
  strength?: 'weak' | 'moderate' | 'strong' | 'unknown';
  is_hardcoded: boolean;
  is_exposed: boolean;
  exposure_risk: 'low' | 'medium' | 'high' | 'critical';
  description?: string;
  owner_team?: string;
  owner_contact?: string;
  discovered_at: string;
  auto_discovered: boolean;
  last_scanned_at?: string;
  created_at: string;
  updated_at: string;
}

export interface SecretRotationHistory {
  id: string;
  org_id: string;
  secret_id: string;
  rotation_type: 'manual' | 'automated' | 'forced' | 'emergency';
  triggered_by_email?: string;
  status: 'pending' | 'in_progress' | 'completed' | 'failed' | 'rolled_back';
  started_at?: string;
  completed_at?: string;
  old_strength?: string;
  new_strength?: string;
  error_message?: string;
  restart_required: boolean;
  services_restarted: boolean;
  created_at: string;
  secret_name?: string;
}

export interface SecretExposureEvent {
  id: string;
  org_id: string;
  secret_id?: string;
  detection_type: 'log_leak' | 'git_commit' | 'config_exposure' | 'network_capture' | 'public_endpoint' | 'error_message';
  detection_source?: string;
  severity: 'low' | 'medium' | 'high' | 'critical';
  exposure_location?: string;
  exposure_content?: string;
  file_path?: string;
  line_number?: number;
  commit_sha?: string;
  repository_url?: string;
  status: 'active' | 'investigating' | 'mitigated' | 'false_positive' | 'accepted_risk';
  acknowledged_at?: string;
  resolved_at?: string;
  resolution_notes?: string;
  potential_impact?: string;
  was_accessed?: boolean;
  access_attempts: number;
  detected_at: string;
  created_at: string;
  secret_name?: string;
}

export interface SecretRotationPolicy {
  id: string;
  org_id: string;
  name: string;
  description?: string;
  max_age_days: number;
  min_strength: string;
  require_unique: boolean;
  allow_reuse: boolean;
  reuse_history_count: number;
  auto_rotate: boolean;
  rotation_cron?: string;
  notify_before_days: number;
  notify_on_expiry: boolean;
  notify_on_exposure: boolean;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface SecretScanResult {
  id: string;
  org_id: string;
  agent_id?: string;
  deployment_id?: string;
  scan_type: 'full' | 'incremental' | 'targeted';
  scan_target?: string;
  scanner: string;
  status: 'running' | 'completed' | 'failed';
  started_at: string;
  completed_at?: string;
  duration_seconds?: number;
  secrets_found: number;
  high_risk_findings: number;
  hardcoded_secrets: number;
  expired_secrets: number;
  weak_secrets: number;
  files_scanned: number;
  containers_scanned: number;
  env_vars_scanned: number;
  error_message?: string;
  created_at: string;
}

export interface SecretsHygieneSummary {
  total_secrets: number;
  needs_rotation: number;
  expired_secrets: number;
  weak_secrets: number;
  hardcoded_secrets: number;
  exposed_secrets: number;
  critical_risk: number;
  high_risk: number;
  avg_age_days: number;
}

export interface SecretExposureSummary {
  total_exposures: number;
  active_exposures: number;
  critical_exposures: number;
  high_exposures: number;
  exposures_last_7d: number;
  exposures_last_30d: number;
}

// Secret Migration Types
export interface SecretMigration {
  id: string;
  org_id: string;
  agent_id: string;
  container_id: string;
  container_name: string;
  source_type: 'env_file' | 'env_var';
  source_path?: string;
  status: 'pending' | 'creating_secrets' | 'stopping_container' | 'starting_container' | 'verifying' | 'completed' | 'failed' | 'rolled_back';
  error_message?: string;
  new_container_id?: string;
  created_at: string;
  updated_at: string;
  completed_at?: string;
  total_secrets?: number;
  completed_secrets?: number;
  failed_secrets?: number;
}

export interface SecretMigrationItem {
  id: string;
  migration_id: string;
  secret_name: string;
  secret_type?: string;
  old_source: string;
  new_source: string;
  status: 'pending' | 'created' | 'attached' | 'verified' | 'completed' | 'failed';
  mount_path?: string;
  error_message?: string;
  created_at: string;
}

export interface SecretMigrationDetails {
  migration: SecretMigration;
  items: SecretMigrationItem[];
}

export interface CreateMigrationRequest {
  container_id: string;
  container_name: string;
  source_type: string;
  source_path?: string;
  secrets: {
    name: string;
    value: string;
    type?: string;
  }[];
}

export interface CreateSecretRequest {
  name: string;
  secret_type: string;
  source: string;
  location?: string;
  agent_id?: string;
  deployment_id?: string;
  container_id?: string;
  container_name?: string;
  service_name?: string;
  classification?: string;
  environment?: string;
  rotation_policy_days?: number;
  description?: string;
  owner_team?: string;
  owner_contact?: string;
}

export interface TriggerSecretScanRequest {
  scan_type?: string;
  scan_target?: string;
  agent_id?: string;
  deployment_id?: string;
}

export interface CreateRotationPolicyRequest {
  name: string;
  description?: string;
  max_age_days?: number;
  min_strength?: string;
  auto_rotate?: boolean;
  rotation_cron?: string;
  notify_before_days?: number;
  notify_on_expiry?: boolean;
  notify_on_exposure?: boolean;
}

// Epic 17: Background Jobs & Workers Types
export interface BackgroundJob {
  id: string;
  org_id: string;
  job_type: string;
  job_name: string;
  job_category: 'system' | 'security' | 'maintenance' | 'user_triggered';
  triggered_by?: string;
  triggered_by_user?: string;
  triggered_by_email?: string;
  target_type?: string;
  target_id?: string;
  target_name?: string;
  parameters?: Record<string, unknown>;
  status: 'pending' | 'queued' | 'running' | 'completed' | 'failed' | 'cancelled' | 'timeout' | 'retrying';
  priority: number;
  progress_percent: number;
  current_step?: string;
  steps_total: number;
  steps_completed: number;
  scheduled_at?: string;
  queued_at?: string;
  started_at?: string;
  completed_at?: string;
  timeout_seconds: number;
  deadline_at?: string;
  result?: Record<string, unknown>;
  output?: string;
  error_message?: string;
  error_code?: string;
  error_stack?: string;
  retry_count: number;
  max_retries: number;
  retry_delay_seconds: number;
  next_retry_at?: string;
  worker_id?: string;
  worker_hostname?: string;
  tags: string[];
  metadata?: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export interface JobSchedule {
  id: string;
  org_id: string;
  name: string;
  description?: string;
  job_type: string;
  job_name: string;
  job_category: string;
  target_type?: string;
  target_id?: string;
  target_name?: string;
  cron_expression: string;
  timezone: string;
  parameters?: Record<string, unknown>;
  priority: number;
  timeout_seconds: number;
  max_retries: number;
  enabled: boolean;
  last_run_at?: string;
  last_run_status?: string;
  last_run_duration_seconds?: number;
  next_run_at?: string;
  run_count: number;
  failure_count: number;
  consecutive_failures: number;
  alert_on_failure: boolean;
  alert_after_consecutive_failures: number;
  pause_after_failures: number;
  created_at: string;
  updated_at: string;
}

export interface Worker {
  id: string;
  org_id?: string;
  hostname: string;
  worker_type: 'general' | 'scan' | 'backup' | 'heavy';
  version?: string;
  pid?: number;
  status: 'starting' | 'idle' | 'busy' | 'stopping' | 'stopped' | 'unhealthy';
  current_job_id?: string;
  jobs_processed: number;
  jobs_failed: number;
  max_concurrent_jobs: number;
  current_jobs: number;
  queue_size: number;
  last_heartbeat_at: string;
  started_at: string;
  stopped_at?: string;
  health_check_failures: number;
  cpu_percent?: number;
  memory_mb?: number;
  memory_percent?: number;
  capabilities: string[];
  metadata?: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export interface JobLog {
  id: string;
  org_id: string;
  job_id: string;
  log_level: 'debug' | 'info' | 'warn' | 'error';
  message: string;
  details?: Record<string, unknown>;
  logged_at: string;
}

export interface JobStatistics {
  org_id: string;
  job_type: string;
  total_jobs: number;
  completed_jobs: number;
  failed_jobs: number;
  running_jobs: number;
  pending_jobs: number;
  jobs_last_24h: number;
  avg_duration_seconds?: number;
  last_completed_at?: string;
}

export interface WorkerSummary {
  org_id: string;
  total_workers: number;
  idle_workers: number;
  busy_workers: number;
  unhealthy_workers: number;
  total_jobs_processed: number;
  total_jobs_failed: number;
  avg_cpu_percent?: number;
  avg_memory_percent?: number;
}

export interface CreateJobRequest {
  job_type: string;
  job_name: string;
  job_category?: string;
  target_type?: string;
  target_id?: string;
  target_name?: string;
  parameters?: Record<string, unknown>;
  priority?: number;
  scheduled_at?: string;
  timeout_seconds?: number;
  max_retries?: number;
  tags?: string[];
}

export interface CreateScheduleRequest {
  name: string;
  description?: string;
  job_type: string;
  job_name: string;
  job_category?: string;
  target_type?: string;
  target_id?: string;
  target_name?: string;
  cron_expression: string;
  timezone?: string;
  parameters?: Record<string, unknown>;
  priority?: number;
  timeout_seconds?: number;
  max_retries?: number;
  alert_on_failure?: boolean;
  alert_after_consecutive_failures?: number;
  pause_after_failures?: number;
}

// Epic 18: External Dependency Mapping Types
export interface ExternalService {
  id: string;
  org_id: string;
  name: string;
  service_type: 'api' | 'database' | 'storage' | 'messaging' | 'cdn' | 'dns' | 'monitoring' | 'auth' | 'payment' | 'other';
  provider?: string;
  endpoint_url?: string;
  base_domain?: string;
  port?: number;
  protocol: string;
  criticality: 'critical' | 'high' | 'medium' | 'low';
  data_classification?: string;
  environment: string;
  health_status: 'healthy' | 'degraded' | 'unhealthy' | 'unknown';
  last_health_check?: string;
  health_check_url?: string;
  health_check_interval_seconds: number;
  expected_uptime_percent?: number;
  sla_document_url?: string;
  compliance_requirements: string[];
  auth_type?: string;
  auth_secret_id?: string;
  owner_team?: string;
  owner_contact?: string;
  vendor_contact?: string;
  contract_end_date?: string;
  discovered_at: string;
  auto_discovered: boolean;
  discovery_source?: string;
  description?: string;
  documentation_url?: string;
  tags: string[];
  metadata?: Record<string, unknown>;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface ServiceDependency {
  id: string;
  org_id: string;
  source_type: string;
  source_id?: string;
  source_name: string;
  external_service_id: string;
  dependency_type: 'runtime' | 'build' | 'test' | 'optional';
  is_critical: boolean;
  fallback_available: boolean;
  fallback_service_id?: string;
  usage_pattern?: string;
  avg_requests_per_minute?: number;
  avg_latency_ms?: number;
  failure_impact?: string;
  discovered_at: string;
  auto_discovered: boolean;
  last_seen_at: string;
  created_at: string;
  updated_at: string;
  external_service_name?: string;
  external_service_type?: string;
  provider?: string;
}

export interface ExternalServiceHealth {
  id: string;
  org_id: string;
  service_id: string;
  status: 'healthy' | 'degraded' | 'unhealthy' | 'timeout' | 'error';
  response_time_ms?: number;
  status_code?: number;
  check_type: string;
  error_message?: string;
  checked_at: string;
  service_name?: string;
}

export interface ExternalServiceIncident {
  id: string;
  org_id: string;
  service_id: string;
  title: string;
  description?: string;
  severity: 'minor' | 'major' | 'critical';
  incident_type?: string;
  status: 'active' | 'investigating' | 'identified' | 'monitoring' | 'resolved';
  started_at: string;
  detected_at: string;
  acknowledged_at?: string;
  resolved_at?: string;
  affected_services: string[];
  impact_description?: string;
  user_impact?: string;
  resolution_notes?: string;
  root_cause?: string;
  reported_by?: string;
  vendor_incident_url?: string;
  created_at: string;
  updated_at: string;
  service_name?: string;
}

export interface DependencyRisk {
  source_name: string;
  source_type: string;
  external_service_name: string;
  service_type: string;
  provider?: string;
  service_criticality: string;
  dependency_critical: boolean;
  fallback_available: boolean;
  health_status: string;
  failure_impact?: string;
  risk_level: 'critical' | 'high' | 'medium' | 'low';
}

export interface ExternalHealthOverview {
  total_services: number;
  healthy_services: number;
  degraded_services: number;
  unhealthy_services: number;
  unknown_services: number;
  critical_services: number;
  critical_unhealthy: number;
}

export interface CreateExternalServiceRequest {
  name: string;
  service_type: string;
  provider?: string;
  endpoint_url?: string;
  base_domain?: string;
  port?: number;
  protocol?: string;
  criticality?: string;
  data_classification?: string;
  environment?: string;
  health_check_url?: string;
  health_check_interval_seconds?: number;
  expected_uptime_percent?: number;
  auth_type?: string;
  owner_team?: string;
  owner_contact?: string;
  vendor_contact?: string;
  description?: string;
  documentation_url?: string;
  tags?: string[];
  metadata?: Record<string, unknown>;
}

export interface CreateServiceDependencyRequest {
  source_type: string;
  source_id?: string;
  source_name: string;
  external_service_id: string;
  dependency_type?: string;
  is_critical?: boolean;
  fallback_available?: boolean;
  fallback_service_id?: string;
  usage_pattern?: string;
  avg_requests_per_minute?: number;
  avg_latency_ms?: number;
  failure_impact?: string;
}

export interface CreateExternalIncidentRequest {
  service_id: string;
  title: string;
  description?: string;
  severity: string;
  incident_type?: string;
  started_at?: string;
  affected_services?: string[];
  impact_description?: string;
  user_impact?: string;
  vendor_incident_url?: string;
}

// Epic 7: Research & Metrics Types
export interface MetricDefinition {
  id: string;
  org_id: string;
  name: string;
  display_name?: string;
  description?: string;
  category: 'security' | 'performance' | 'compliance' | 'usage' | 'cost' | 'health';
  subcategory?: string;
  metric_type: 'counter' | 'gauge' | 'histogram' | 'summary';
  unit?: string;
  value_type: 'numeric' | 'boolean' | 'string';
  collection_method: 'automatic' | 'manual' | 'calculated' | 'aggregated';
  collection_interval_seconds: number;
  source?: string;
  query_template?: string;
  aggregation_type?: 'sum' | 'avg' | 'min' | 'max' | 'count' | 'last' | 'p50' | 'p95' | 'p99';
  aggregation_window_seconds?: number;
  warning_threshold?: number;
  critical_threshold?: number;
  threshold_direction: 'above' | 'below';
  is_primary: boolean;
  display_order: number;
  chart_type?: 'line' | 'bar' | 'pie' | 'gauge' | 'number';
  color?: string;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface MetricValue {
  id: string;
  org_id: string;
  metric_id: string;
  agent_id?: string;
  deployment_id?: string;
  resource_type?: string;
  resource_id?: string;
  labels?: Record<string, string>;
  value_numeric?: number;
  value_string?: string;
  value_bool?: boolean;
  collected_at: string;
  period_start?: string;
  period_end?: string;
  source?: string;
  metadata?: Record<string, unknown>;
  metric_name?: string;
  metric_category?: string;
}

export interface MetricSummary {
  org_id: string;
  category: string;
  total_metrics: number;
  enabled_metrics: number;
  metrics_with_data: number;
  last_data_collected?: string;
}

export interface Dashboard {
  id: string;
  org_id: string;
  name: string;
  description?: string;
  slug?: string;
  created_by?: string;
  is_default: boolean;
  is_shared: boolean;
  layout?: Record<string, unknown>[];
  settings?: Record<string, unknown>;
  visibility: 'private' | 'team' | 'org' | 'public';
  widget_count?: number;
  created_at: string;
  updated_at: string;
}

export interface DashboardWidget {
  id: string;
  org_id: string;
  dashboard_id: string;
  widget_type: 'metric' | 'chart' | 'table' | 'text' | 'alert_summary';
  title?: string;
  metric_ids?: string[];
  query?: Record<string, unknown>;
  filters?: Record<string, unknown>;
  chart_type?: 'line' | 'bar' | 'pie' | 'area' | 'gauge' | 'number' | 'table';
  time_range?: 'last_hour' | 'last_24h' | 'last_7d' | 'last_30d' | 'custom';
  refresh_interval_seconds: number;
  position_x: number;
  position_y: number;
  width: number;
  height: number;
  settings?: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export interface Report {
  id: string;
  org_id: string;
  name: string;
  description?: string;
  report_type: 'security_summary' | 'compliance' | 'executive' | 'custom';
  template?: string;
  config?: Record<string, unknown>;
  filters?: Record<string, unknown>;
  schedule_enabled: boolean;
  schedule_cron?: string;
  schedule_timezone: string;
  next_run_at?: string;
  last_run_at?: string;
  delivery_method?: 'email' | 'download' | 'webhook' | 's3';
  delivery_config?: Record<string, unknown>;
  recipients?: string[];
  created_by?: string;
  visibility: 'private' | 'team' | 'org' | 'public';
  created_at: string;
  updated_at: string;
  last_execution_status?: string;
}

export interface ReportExecution {
  id: string;
  org_id: string;
  report_id: string;
  triggered_by: 'schedule' | 'manual' | 'api';
  triggered_by_user?: string;
  status: 'pending' | 'running' | 'completed' | 'failed';
  started_at?: string;
  completed_at?: string;
  duration_seconds?: number;
  output_format?: 'pdf' | 'html' | 'csv' | 'json';
  output_path?: string;
  output_size_bytes?: number;
  delivered_at?: string;
  delivery_status?: string;
  delivery_error?: string;
  data_start?: string;
  data_end?: string;
  error_message?: string;
  retry_count: number;
  created_at: string;
  report_name?: string;
  report_type?: string;
}

export interface TelemetrySettings {
  id: string;
  org_id: string;
  collect_usage_metrics: boolean;
  collect_performance_metrics: boolean;
  collect_security_metrics: boolean;
  collect_error_reports: boolean;
  research_participation: boolean;
  research_consent_date?: string;
  research_consent_version?: string;
  anonymize_data: boolean;
  data_retention_days: number;
  allow_data_export: boolean;
  export_format: string;
  updated_at: string;
  updated_by?: string;
}

export interface CreateMetricDefinitionRequest {
  name: string;
  display_name?: string;
  description?: string;
  category: string;
  subcategory?: string;
  metric_type?: string;
  unit?: string;
  value_type?: string;
  collection_method?: string;
  collection_interval_seconds?: number;
  source?: string;
  query_template?: string;
  aggregation_type?: string;
  aggregation_window_seconds?: number;
  warning_threshold?: number;
  critical_threshold?: number;
  threshold_direction?: string;
  is_primary?: boolean;
  display_order?: number;
  chart_type?: string;
  color?: string;
}

export interface CreateDashboardRequest {
  name: string;
  description?: string;
  slug?: string;
  is_default?: boolean;
  is_shared?: boolean;
  layout?: Record<string, unknown>[];
  settings?: Record<string, unknown>;
  visibility?: string;
}

export interface CreateReportRequest {
  name: string;
  description?: string;
  report_type: string;
  template?: string;
  config?: Record<string, unknown>;
  filters?: Record<string, unknown>;
  schedule_enabled?: boolean;
  schedule_cron?: string;
  schedule_timezone?: string;
  delivery_method?: string;
  delivery_config?: Record<string, unknown>;
  recipients?: string[];
  visibility?: string;
}

export interface UpdateTelemetrySettingsRequest {
  collect_usage_metrics?: boolean;
  collect_performance_metrics?: boolean;
  collect_security_metrics?: boolean;
  collect_error_reports?: boolean;
  research_participation?: boolean;
  research_consent_version?: string;
  anonymize_data?: boolean;
  data_retention_days?: number;
  allow_data_export?: boolean;
  export_format?: string;
}

// Managed Stacks (multi-service docker-compose deployments)
export type ManagedStackStatus = "pending" | "deploying" | "running" | "partial" | "failed" | "stopped";

export interface ManagedStack {
  id: string;
  org_id: string;
  agent_id: string;
  name: string;
  environment: string;
  compose_yaml: string;
  variables: Record<string, string>;
  service_count: number;
  running_count: number;
  failed_count: number;
  status: ManagedStackStatus;
  status_message?: string;
  deployed_by?: string;
  deployed_at?: string;
  created_at: string;
  updated_at: string;
  deployments?: Deployment[];
  // Persisted default service-name selection for stack-level redeploy; absent/empty means
  // "all services" (never explicitly saved yet).
  redeploy_services?: string[];
}

export interface CreateStackRequest {
  name: string;
  environment: string;
  compose_yaml: string;
  variables?: Record<string, string>;
  overrides?: ServiceOverride[];
  skip_scanning?: boolean;
  // Stack companion files (v3/45): the files this compose bind-mounts, keyed by their
  // stack-root-relative path. Required whenever parsed.required_files is non-empty.
  files?: StackFile[];
}

// RedeployStackRequest redeploys some or all of a managed stack's services, reusing the
// stack's saved compose/variables/files unless new ones are supplied.
export interface RedeployStackRequest {
  services?: string[]; // empty/omitted = the stack's saved default, or all services
  compose_yaml?: string; // provide to update; omit (with variables/files) to keep as-is
  variables?: Record<string, string>;
  files?: StackFile[];
  pull_latest?: boolean; // default true
  skip_scanning?: boolean; // default: the stack's saved setting
  save_selection?: boolean; // persist `services` as the new default
}

// StackFile is a companion file shipped with a stack deploy (v3/45).
export interface StackFile {
  path: string; // stack-root-relative, e.g. "scripts/db-init.sh"
  content: string;
  mode?: string; // octal, e.g. "0755"; default "0644"
}

// RequiredFile is a relative bind-mount source the compose needs supplied as a companion file (v3/45).
export interface RequiredFile {
  source: string; // as written, e.g. "./scripts/db-init.sh"
  path: string; // normalized stack-root-relative path, e.g. "scripts/db-init.sh"
  service: string;
  target: string; // in-container mount path
}

export interface ServiceOverride {
  service_name: string;
  tag_override?: string;
  env_overrides?: Record<string, string>;
  enabled: boolean;
}

export interface ParseComposeRequest {
  compose_yaml: string;
  variables?: Record<string, string>;
}

export interface ParsedCompose {
  services: ComposeService[];
  networks: ComposeNetwork[];
  volumes: ComposeVolume[];
  variables: ComposeVariable[];
  required_files?: RequiredFile[];
  errors?: string[];
}

export interface ComposeService {
  name: string;
  image: string;
  depends_on?: string[];
  ports?: string[];
  environment?: Record<string, string>;
  volumes?: string[];
  networks?: string[];
  command?: string[];
  labels?: Record<string, string>;
  restart?: string;
  env_files?: string[];
  order: number;
}

export interface ComposeNetwork {
  name: string;
  driver?: string;
  external: boolean;
}

export interface ComposeVolume {
  name: string;
  driver?: string;
  external: boolean;
}

export interface ComposeVariable {
  name: string;
  default?: string;
  used: boolean;
  required: boolean;
}

export interface StackProgress {
  stack_id: string;
  status: ManagedStackStatus;
  service_count: number;
  running_count: number;
  failed_count: number;
  services: ServiceProgress[];
}

export interface ServiceProgress {
  name: string;
  status: string;
  message?: string;
  order: number;
}

export interface APIKey {
  id: string;
  org_id: string;
  user_id: string;
  name: string;
  key_prefix: string;
  last_used_at?: string;
  created_at: string;
  expires_at?: string;
}

export interface APIKeyWithSecret extends APIKey {
  key: string; // Only returned on creation
}

// API methods
export const api = {
  // Setup (first-run)
  getSetupStatus: (options?: RequestInit, timeoutMs?: number) => fetchAPI<SetupStatusResponse>("/setup/status", options ?? {}, timeoutMs),

  setupLicense: (key: string, setupToken: string) =>
    fetchAPI<SetupLicenseResponse>("/setup/license", {
      method: "POST",
      body: JSON.stringify({ key, setup_token: setupToken }),
    }),

  createInitialAdmin: (email: string, password: string, setupToken: string) =>
    fetchAPI<SetupResponse>("/setup", {
      method: "POST",
      body: JSON.stringify({ email, password, setup_token: setupToken }),
    }),

  // Auth
  login: (email: string, password: string) =>
    fetchAPI<LoginResponse>("/auth/login", {
      method: "POST",
      body: JSON.stringify({ email, password }),
    }),

  logout: () =>
    fetchAPI("/auth/logout", {
      method: "POST",
    }),

  refreshToken: (refreshToken: string) =>
    fetchAPI<{ access_token: string }>("/auth/refresh", {
      method: "POST",
      body: JSON.stringify({ refresh_token: refreshToken }),
    }),

  getCurrentUser: () => fetchAPI<User>("/auth/me"),

  // Agents
  getAgents: () => fetchAPI<Agent[]>("/agents"),

  getAgent: (id: string) => fetchAPI<Agent>(`/agents/${id}`),

  createAgent: (name: string) =>
    fetchAPI<Agent>("/agents", {
      method: "POST",
      body: JSON.stringify({ name }),
    }),

  deleteAgent: (id: string) =>
    fetchAPI(`/agents/${id}`, {
      method: "DELETE",
    }),

  // Containers
  getContainers: (agentId: string) =>
    fetchAPI<Container[]>(`/agents/${agentId}/containers`),

  getContainerDetail: (agentId: string, containerId: string) =>
    fetchAPI<ContainerDetail>(`/agents/${agentId}/containers/${containerId}`),

  startContainer: (agentId: string, containerId: string) =>
    fetchAPI(`/agents/${agentId}/containers/${containerId}/start`, {
      method: "POST",
    }),

  stopContainer: (agentId: string, containerId: string) =>
    fetchAPI(`/agents/${agentId}/containers/${containerId}/stop`, {
      method: "POST",
    }),

  restartContainer: (agentId: string, containerId: string) =>
    fetchAPI(`/agents/${agentId}/containers/${containerId}/restart`, {
      method: "POST",
    }),

  deleteContainer: (agentId: string, containerId: string, confirmName: string, force: boolean = false) =>
    fetchAPI<{ message: string; container_id: string; name: string }>(
      `/agents/${agentId}/containers/${containerId}`,
      {
        method: "DELETE",
        body: JSON.stringify({ confirm_name: confirmName, force }),
      }
    ),

  pauseContainer: (agentId: string, containerId: string) =>
    fetchAPI(`/agents/${agentId}/containers/${containerId}/pause`, {
      method: "POST",
    }),

  unpauseContainer: (agentId: string, containerId: string) =>
    fetchAPI(`/agents/${agentId}/containers/${containerId}/unpause`, {
      method: "POST",
    }),

  killContainer: (agentId: string, containerId: string, signal: string = "SIGKILL") =>
    fetchAPI(`/agents/${agentId}/containers/${containerId}/kill`, {
      method: "POST",
      body: JSON.stringify({ signal }),
    }),

  renameContainer: (agentId: string, containerId: string, newName: string) =>
    fetchAPI<{ message: string; container_id: string; old_name: string; new_name: string }>(
      `/agents/${agentId}/containers/${containerId}/rename`,
      {
        method: "POST",
        body: JSON.stringify({ new_name: newName }),
      }
    ),

  updateContainer: (agentId: string, containerId: string, config: {
    restart_policy?: string;
    max_retries?: number;
    memory_limit?: number;
    memory_swap?: number;
    cpu_shares?: number;
    cpu_quota?: number;
    cpu_period?: number;
  }) =>
    fetchAPI(`/agents/${agentId}/containers/${containerId}`, {
      method: "PUT",
      body: JSON.stringify(config),
    }),

  inspectContainer: (agentId: string, containerId: string) =>
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    fetchAPI<any>(`/agents/${agentId}/containers/${containerId}/inspect`),

  getContainerLogs: (agentId: string, containerId: string, tail: number = 100) =>
    fetchAPI<{ container_id: string; logs: string }>(
      `/agents/${agentId}/containers/${containerId}/logs?tail=${tail}`
    ),

  getStacks: (agentId: string) =>
    fetchAPI<Stack[]>(`/agents/${agentId}/stacks`),

  // Proxies
  getProxyHosts: (agentId: string) =>
    fetchAPI<ProxyHost[]>(`/agents/${agentId}/proxies`),

  getProxyHost: (agentId: string, proxyId: string) =>
    fetchAPI<ProxyHost>(`/agents/${agentId}/proxies/${proxyId}`),

  createProxyHost: (
    agentId: string,
    data: {
      domain: string;
      upstream_target?: string;
      proxy_type?: 'upstream' | 'redirect';
      redirect_url?: string;
      redirect_code?: number;
      force_ssl?: boolean;
      http2_enabled?: boolean;
      include_www?: boolean;
    }
  ) =>
    fetchAPI<ProxyHost>(`/agents/${agentId}/proxies`, {
      method: "POST",
      body: JSON.stringify(data),
    }),

  testNetworkConnectivity: (
    agentId: string,
    data: TestNetworkRequest
  ) =>
    fetchAPI<TestNetworkResponse>(`/agents/${agentId}/proxies/test-network`, {
      method: "POST",
      body: JSON.stringify(data),
    }),

  updateProxyHost: (
    agentId: string,
    proxyId: string,
    data: Partial<{
      domain: string;
      upstream_target: string;
      proxy_type: 'upstream' | 'redirect';
      redirect_url: string;
      redirect_code: number;
      force_ssl: boolean;
      http2_enabled: boolean;
      include_www: boolean;
      basic_auth_enabled: boolean;
      basic_auth_realm: string;
      basic_auth_excluded_paths: string[];
      status: string;
    }>
  ) =>
    fetchAPI<ProxyHost>(`/agents/${agentId}/proxies/${proxyId}`, {
      method: "PUT",
      body: JSON.stringify(data),
    }),

  deleteProxyHost: (agentId: string, proxyId: string) =>
    fetchAPI(`/agents/${agentId}/proxies/${proxyId}`, {
      method: "DELETE",
    }),

  requestSSL: (agentId: string, proxyId: string) =>
    fetchAPI(`/agents/${agentId}/proxies/${proxyId}/ssl`, {
      method: "POST",
    }),

  applyWildcardSSL: (
    agentId: string,
    proxyId: string,
    data: {
      ssl_enabled: boolean;
      force_ssl: boolean;
      http2_enabled: boolean;
      ssl_source: SSLSource;
      ssl_cert_path: string;
      ssl_key_path: string;
    }
  ) =>
    fetchAPI(`/agents/${agentId}/proxies/${proxyId}/ssl/wildcard`, {
      method: "POST",
      body: JSON.stringify(data),
    }),

  getProxyConfig: (agentId: string, proxyId: string) =>
    fetchAPI<{ domain: string; config: string }>(
      `/agents/${agentId}/proxies/${proxyId}/config`
    ),

  getSecurityHeaders: (agentId: string, proxyId: string) =>
    fetchAPI<SecurityHeaders>(
      `/agents/${agentId}/proxies/${proxyId}/security-headers`
    ),

  updateSecurityHeaders: (
    agentId: string,
    proxyId: string,
    data: Omit<SecurityHeaders, "id" | "proxy_host_id">
  ) =>
    fetchAPI<SecurityHeaders>(
      `/agents/${agentId}/proxies/${proxyId}/security-headers`,
      {
        method: "PUT",
        body: JSON.stringify(data),
      }
    ),

  // Basic Auth Users
  listAuthUsers: (agentId: string, proxyId: string) =>
    fetchAPI<AuthUser[]>(`/agents/${agentId}/proxies/${proxyId}/auth-users`),

  createAuthUser: (
    agentId: string,
    proxyId: string,
    data: { username: string; password: string }
  ) =>
    fetchAPI<AuthUser>(`/agents/${agentId}/proxies/${proxyId}/auth-users`, {
      method: "POST",
      body: JSON.stringify(data),
    }),

  deleteAuthUser: (agentId: string, proxyId: string, userId: string) =>
    fetchAPI<void>(
      `/agents/${agentId}/proxies/${proxyId}/auth-users/${userId}`,
      {
        method: "DELETE",
      }
    ),

  // Config Preview
  previewProxyConfig: (
    agentId: string,
    proxyId: string,
    config: ConfigPreviewRequest
  ) =>
    fetchAPI<{ config: string }>(
      `/agents/${agentId}/proxies/${proxyId}/config/preview`,
      {
        method: "POST",
        body: JSON.stringify(config),
      }
    ),

  // Config Test
  testProxyConfig: (agentId: string, proxyId: string) =>
    fetchAPI<{ valid: boolean; message: string }>(
      `/agents/${agentId}/proxies/${proxyId}/test`,
      {
        method: "POST",
      }
    ),

  // Alerts - Channels
  getAlertChannels: () => fetchAPI<AlertChannel[]>("/alerts/channels"),

  createAlertChannel: (data: {
    name: string;
    channel_type: "smtp" | "slack" | "webhook" | "discord" | "teams";
    config: Record<string, unknown>;
    enabled?: boolean;
  }) =>
    fetchAPI<AlertChannel>("/alerts/channels", {
      method: "POST",
      body: JSON.stringify(data),
    }),

  updateAlertChannel: (
    channelId: string,
    data: {
      name?: string;
      config?: Record<string, unknown>;
      enabled?: boolean;
    }
  ) =>
    fetchAPI<AlertChannel>(`/alerts/channels/${channelId}`, {
      method: "PUT",
      body: JSON.stringify(data),
    }),

  deleteAlertChannel: (channelId: string) =>
    fetchAPI(`/alerts/channels/${channelId}`, {
      method: "DELETE",
    }),

  testAlertChannel: (channelId: string) =>
    fetchAPI<{ success: boolean; message: string }>(`/alerts/channels/${channelId}/test`, {
      method: "POST",
    }),

  // Alerts - Rules
  getAlertRules: () => fetchAPI<AlertRule[]>("/alerts/rules"),

  createAlertRule: (data: {
    name: string;
    rule_type: string;
    conditions: Record<string, unknown>;
    channels: string[];
    cooldown_mins?: number;
    enabled?: boolean;
  }) =>
    fetchAPI<AlertRule>("/alerts/rules", {
      method: "POST",
      body: JSON.stringify(data),
    }),

  updateAlertRule: (
    ruleId: string,
    data: {
      name?: string;
      conditions?: Record<string, unknown>;
      channels?: string[];
      cooldown_mins?: number;
      enabled?: boolean;
    }
  ) =>
    fetchAPI<AlertRule>(`/alerts/rules/${ruleId}`, {
      method: "PUT",
      body: JSON.stringify(data),
    }),

  deleteAlertRule: (ruleId: string) =>
    fetchAPI(`/alerts/rules/${ruleId}`, {
      method: "DELETE",
    }),

  // Alerts - History
  getAlertHistory: (limit?: number) =>
    fetchAPI<AlertHistoryEntry[]>(`/alerts/history${limit ? `?limit=${limit}` : ""}`),

  // Networks (for cross-network proxying)
  getNetworks: (agentId: string) =>
    fetchAPI<DockerNetwork[]>(`/agents/${agentId}/networks`),

  getContainerNetworks: (agentId: string, containerId: string) =>
    fetchAPI<ContainerNetworkInfo[]>(
      `/agents/${agentId}/containers/${containerId}/networks`
    ),

  checkNginxNetwork: (agentId: string, networkId: string) =>
    fetchAPI<NetworkCheckResult>(
      `/agents/${agentId}/networks/${networkId}/check-nginx`
    ),

  getNginxNetworkAttachments: (agentId: string) =>
    fetchAPI<NginxNetworkAttachment[]>(`/agents/${agentId}/networks/attachments`),

  attachNginxNetwork: (agentId: string, networkId: string) =>
    fetchAPI<{ success: boolean; id: string; network_id: string; network_name: string }>(
      `/agents/${agentId}/networks/attach`,
      {
        method: "POST",
        body: JSON.stringify({ network_id: networkId }),
      }
    ),

  detachNginxNetwork: (agentId: string, networkId: string) =>
    fetchAPI<{ success: boolean; network_id: string }>(
      `/agents/${agentId}/networks/detach`,
      {
        method: "POST",
        body: JSON.stringify({ network_id: networkId }),
      }
    ),

  // Docker Resources (Networks, Volumes, Images)

  // Docker Networks
  getDockerNetworks: (agentId: string) =>
    fetchAPI<DockerNetwork[]>(`/agents/${agentId}/docker/networks`),

  getDockerNetwork: (agentId: string, networkId: string) =>
    fetchAPI<DockerNetworkDetail>(`/agents/${agentId}/docker/networks/${networkId}`),

  createDockerNetwork: (agentId: string, data: CreateNetworkRequest) =>
    fetchAPI<DockerNetwork>(`/agents/${agentId}/docker/networks`, {
      method: "POST",
      body: JSON.stringify(data),
    }),

  deleteDockerNetwork: (agentId: string, networkId: string, force?: boolean) =>
    fetchAPI<{ success: boolean; message: string }>(
      `/agents/${agentId}/docker/networks/${networkId}${force ? "?force=true" : ""}`,
      { method: "DELETE" }
    ),

  // Docker Volumes
  getDockerVolumes: (agentId: string) =>
    fetchAPI<DockerVolume[]>(`/agents/${agentId}/docker/volumes`),

  getDockerVolume: (agentId: string, name: string) =>
    fetchAPI<DockerVolume>(`/agents/${agentId}/docker/volumes/${encodeURIComponent(name)}`),

  createDockerVolume: (agentId: string, data: CreateVolumeRequest) =>
    fetchAPI<DockerVolume>(`/agents/${agentId}/docker/volumes`, {
      method: "POST",
      body: JSON.stringify(data),
    }),

  deleteDockerVolume: (agentId: string, name: string, force?: boolean) =>
    fetchAPI<{ success: boolean; message: string }>(
      `/agents/${agentId}/docker/volumes/${encodeURIComponent(name)}${force ? "?force=true" : ""}`,
      { method: "DELETE" }
    ),

  // Docker Images
  getDockerImages: (agentId: string) =>
    fetchAPI<DockerImage[]>(`/agents/${agentId}/docker/images`),

  getDockerImage: (agentId: string, imageId: string) =>
    fetchAPI<DockerImage>(`/agents/${agentId}/docker/images/${encodeURIComponent(imageId)}`),

  pullDockerImage: (agentId: string, image: string, registryId?: string) =>
    fetchAPI<{ success: boolean; message: string }>(
      `/agents/${agentId}/docker/images/pull`,
      {
        method: "POST",
        body: JSON.stringify({ image, registry_id: registryId }),
      }
    ),

  deleteDockerImage: (agentId: string, imageId: string, force?: boolean) =>
    fetchAPI<{ success: boolean; message: string }>(
      `/agents/${agentId}/docker/images/${encodeURIComponent(imageId)}${force ? "?force=true" : ""}`,
      { method: "DELETE" }
    ),

  // Logs
  getUnifiedLogs: (
    agentId: string,
    options?: {
      sources?: string[];
      levels?: string[];
      search?: string;
      tail?: number;
      containers?: string[];
    }
  ) => {
    const params = new URLSearchParams();
    if (options?.sources) options.sources.forEach(s => params.append("sources", s));
    if (options?.levels) options.levels.forEach(l => params.append("levels", l));
    if (options?.search) params.set("search", options.search);
    if (options?.tail) params.set("tail", options.tail.toString());
    if (options?.containers) options.containers.forEach(c => params.append("containers", c));
    const query = params.toString();
    return fetchAPI<UnifiedLogsResponse>(`/agents/${agentId}/logs/unified${query ? `?${query}` : ""}`);
  },

  getNginxLogs: (agentId: string, type: "access" | "error" = "access", tail: number = 100) =>
    fetchAPI<NginxLogsResponse>(`/agents/${agentId}/logs/nginx?type=${type}&tail=${tail}`),

  // Audit logs
  getAuditLogs: (options?: { limit?: number; offset?: number; action?: string; resource_type?: string }) => {
    const params = new URLSearchParams();
    if (options?.limit) params.set("limit", options.limit.toString());
    if (options?.offset) params.set("offset", options.offset.toString());
    if (options?.action) params.set("action", options.action);
    if (options?.resource_type) params.set("resource_type", options.resource_type);
    const query = params.toString();
    return fetchAPI<AuditLogsResponse>(`/audit${query ? `?${query}` : ""}`);
  },

  // User management
  getUsers: () => fetchAPI<UserAccount[]>("/users"),

  createUser: (data: { email: string; password: string; role: string }) =>
    fetchAPI<UserAccount>("/users", {
      method: "POST",
      body: JSON.stringify(data),
    }),

  updateUser: (userId: string, data: { email?: string; password?: string; role?: string }) =>
    fetchAPI<UserAccount>(`/users/${userId}`, {
      method: "PUT",
      body: JSON.stringify(data),
    }),

  deleteUser: (userId: string) =>
    fetchAPI(`/users/${userId}`, {
      method: "DELETE",
    }),

  // Health monitoring
  getTLSHealth: () => fetchAPI<TLSHealthSummary>("/health/tls"),
  getDBHealth: () => fetchAPI<DatabaseHealth>("/health/database"),
  getSystemHealth: () => fetchAPI<SystemHealth>("/health/system"),

  // Rate Limits
  getRateLimits: (agentId: string, proxyId: string) =>
    fetchAPI<RateLimit[]>(`/agents/${agentId}/proxies/${proxyId}/rate-limits`),

  createRateLimit: (
    agentId: string,
    proxyId: string,
    data: {
      zone_name: string;
      requests_per: number;
      time_window: string;
      burst?: number;
      enabled?: boolean;
    }
  ) =>
    fetchAPI<RateLimit>(`/agents/${agentId}/proxies/${proxyId}/rate-limits`, {
      method: "POST",
      body: JSON.stringify(data),
    }),

  updateRateLimit: (
    agentId: string,
    proxyId: string,
    rateLimitId: string,
    data: {
      zone_name: string;
      requests_per: number;
      time_window: string;
      burst?: number;
      enabled?: boolean;
    }
  ) =>
    fetchAPI<RateLimit>(
      `/agents/${agentId}/proxies/${proxyId}/rate-limits/${rateLimitId}`,
      {
        method: "PUT",
        body: JSON.stringify(data),
      }
    ),

  deleteRateLimit: (agentId: string, proxyId: string, rateLimitId: string) =>
    fetchAPI(`/agents/${agentId}/proxies/${proxyId}/rate-limits/${rateLimitId}`, {
      method: "DELETE",
    }),

  // MFA
  verifyMFA: (mfaToken: string, code: string) =>
    fetchAPI<LoginResponse>("/auth/mfa/verify", {
      method: "POST",
      body: JSON.stringify({ mfa_token: mfaToken, code }),
    }),

  setupMFA: () => fetchAPI<MFASetupResponse>("/auth/mfa/setup", { method: "POST" }),

  confirmMFA: (code: string) =>
    fetchAPI<MFAConfirmResponse>("/auth/mfa/confirm", {
      method: "POST",
      body: JSON.stringify({ code }),
    }),

  disableMFA: (password: string, code: string) =>
    fetchAPI<{ message: string }>("/auth/mfa/disable", {
      method: "POST",
      body: JSON.stringify({ password, code }),
    }),

  regenerateBackupCodes: (code: string) =>
    fetchAPI<MFAConfirmResponse>("/auth/mfa/backup-codes", {
      method: "POST",
      body: JSON.stringify({ code }),
    }),

  // System Settings
  getSystemSettings: () =>
    fetchAPI<Record<string, unknown>>("/settings"),

  getInfraPilotDomain: () =>
    fetchAPI<InfraPilotDomainSettings>("/settings/domain"),

  updateInfraPilotDomain: (data: {
    domain: string;
    ssl_enabled?: boolean;
    force_ssl?: boolean;
    http2_enabled?: boolean;
    ssl_source?: SSLSource;
    ssl_certificate_id?: string;
    ssl_cert_path?: string;
    ssl_key_path?: string;
  }) =>
    fetchAPI<InfraPilotDomainSettings>("/settings/domain", {
      method: "PUT",
      body: JSON.stringify(data),
    }),

  deleteInfraPilotDomain: () =>
    fetchAPI<{ message: string }>("/settings/domain", {
      method: "DELETE",
    }),

  // SSL/TLS Certificate Management
  checkSSL: (domain: string, remote: boolean = false) =>
    fetchAPI<SSLCertificateInfo>(`/ssl/check/${encodeURIComponent(domain)}${remote ? '?remote=true' : ''}`),

  checkWildcardSSL: (domain: string) =>
    fetchAPI<{ domain: string; certificates: SSLCertificateInfo[] }>(
      `/ssl/check-wildcard/${encodeURIComponent(domain)}`
    ),

  getSSLStatus: () =>
    fetchAPI<SSLStatus>("/ssl/status"),

  updateSSLSettings: (data: { email: string; staging: boolean }) =>
    fetchAPI<{ email: string; staging: boolean; message: string }>("/ssl/settings", {
      method: "PUT",
      body: JSON.stringify(data),
    }),

  requestSSLCertificate: async (options: SSLRequestOptions): Promise<{
    success: boolean;
    message?: string;
    error?: string;
    domain: string;
    email?: string;
    staging?: boolean;
  }> => {
    const token = typeof window !== "undefined" ? localStorage.getItem("access_token") : null;
    const res = await fetch(`${API_BASE}/ssl/request`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
      body: JSON.stringify(options),
    });
    const data = await res.json();
    // Return the full response regardless of status code
    return data;
  },

  uploadSSLCertificate: async (data: {
    cert_pem: string;
    key_pem: string;
    agent_id?: string;
    proxy_id?: string;
  }): Promise<{
    cert_path: string;
    key_path: string;
    domain: string;
    issuer: string;
    expires_at: string;
    sans: string[];
    is_wildcard: boolean;
    message?: string;
    error?: string;
  }> => {
    const token = typeof window !== "undefined" ? localStorage.getItem("access_token") : null;
    const res = await fetch(`${API_BASE}/ssl/upload`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
      body: JSON.stringify(data),
    });
    return res.json();
  },

  getDNSInstructions: (domain: string) =>
    fetchAPI<DNSInstructions>(`/ssl/dns-instructions/${encodeURIComponent(domain)}`),

  verifyDNS: (domain: string, includeWWW?: boolean) =>
    fetchAPI<DNSVerifyResult>(`/ssl/verify-dns/${encodeURIComponent(domain)}${includeWWW ? '?include_www=true' : ''}`),

  // DNS-01 Challenge (for wildcard certificates)
  startDNSChallenge: async (options: { domain: string; email?: string; staging?: boolean }): Promise<{
    success: boolean;
    message?: string;
    error?: string;
    domain: string;
    txt_record?: string;
    txt_name?: string;
    txt_records?: { name: string; value: string }[];
    instructions?: string;
  }> => {
    const token = typeof window !== "undefined" ? localStorage.getItem("access_token") : null;
    const res = await fetch(`${API_BASE}/ssl/dns-challenge/start`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
      body: JSON.stringify(options),
    });
    return res.json();
  },

  completeDNSChallenge: async (options: { domain: string; email?: string; staging?: boolean }): Promise<{
    success: boolean;
    message?: string;
    error?: string;
    domain: string;
  }> => {
    const token = typeof window !== "undefined" ? localStorage.getItem("access_token") : null;
    const res = await fetch(`${API_BASE}/ssl/dns-challenge/complete`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
      body: JSON.stringify(options),
    });
    return res.json();
  },

  getDNSChallenge: (domain: string) =>
    fetchAPI<{ success: boolean; domain: string; txt_record?: string; txt_name?: string; created_at?: string; error?: string }>(`/ssl/dns-challenge/${encodeURIComponent(domain)}`),

  verifyDNSTXTRecord: (domain: string, expectedValue?: string) =>
    fetchAPI<{ verified: boolean; txt_name: string; found: string[]; expected?: string; error?: string }>(
      `/ssl/dns-challenge/verify/${encodeURIComponent(domain)}${expectedValue ? `?value=${encodeURIComponent(expectedValue)}` : ''}`
    ),

  // SSL Certificate Management
  listSSLCertificates: () =>
    fetchAPI<SSLCertificateRecord[]>("/ssl/certificates"),

  scanSSLCertificates: () =>
    fetchAPI<SSLCertificateScanResult>("/ssl/certificates/scan"),

  registerSSLCertificate: (data: { name?: string; domain?: string; cert_path: string; key_path: string }) =>
    fetchAPI<{ id: string; name: string; domain: string; is_wildcard: boolean; issuer?: string; expires_at?: string; message: string }>("/ssl/certificates", {
      method: "POST",
      body: JSON.stringify(data),
    }),

  getSSLCertificate: (id: string) =>
    fetchAPI<SSLCertificateRecord>(`/ssl/certificates/${id}`),

  deleteSSLCertificate: (id: string) =>
    fetchAPI<{ message: string }>(`/ssl/certificates/${id}`, {
      method: "DELETE",
    }),

  // Default Pages
  getDefaultPages: () =>
    fetchAPI<DefaultPage[]>("/settings/default-pages"),

  getDefaultPage: (pageType: string) =>
    fetchAPI<DefaultPage>(`/settings/default-pages/${pageType}`),

  updateDefaultPage: (pageType: string, data: Partial<DefaultPage>) =>
    fetchAPI<DefaultPage>(`/settings/default-pages/${pageType}`, {
      method: "PUT",
      body: JSON.stringify(data),
    }),

  previewDefaultPage: (pageType: string) =>
    fetchAPI<{ html: string }>(`/settings/default-pages/${pageType}/preview`),

  // Nginx Management
  testNginxConfig: (agentId: string) =>
    fetchAPI<{ success: boolean; message: string }>(`/agents/${agentId}/nginx/test`, {
      method: "POST",
    }),

  reloadNginx: (agentId: string) =>
    fetchAPI<{ success: boolean; message: string }>(`/agents/${agentId}/nginx/reload`, {
      method: "POST",
    }),

  // License
  getLicenseInfo: () => fetchAPI<LicenseInfo>("/license"),
  getLicenseTierInfo: () => fetchAPI<LicenseTierInfo>("/license"),

  // License settings (admin)
  getLicenseSettings: () => fetchAPI<LicenseSettingsResponse>("/settings/license"),
  updateLicenseKey: (key: string) =>
    fetchAPI<LicenseSettingsResponse>("/settings/license", {
      method: "PUT",
      body: JSON.stringify({ key }),
    }),
  // Outcome recorded by the CE→EE switch helper; polled across the restart.
  getUpgradeStatus: () =>
    fetchAPI<UpgradeStatus>("/settings/license/upgrade/status"),

  // SSO Providers (admin)
  getSSOProviders: () => fetchAPI<SSOProvider[]>("/sso/providers"),
  getSSOProvider: (id: string) => fetchAPI<SSOProvider>(`/sso/providers/${id}`),
  createSSOProvider: (data: CreateSSOProviderRequest) =>
    fetchAPI<SSOProvider>("/sso/providers", {
      method: "POST",
      body: JSON.stringify(data),
    }),
  updateSSOProvider: (id: string, data: Partial<CreateSSOProviderRequest>) =>
    fetchAPI<SSOProvider>(`/sso/providers/${id}`, {
      method: "PUT",
      body: JSON.stringify(data),
    }),
  deleteSSOProvider: (id: string) =>
    fetchAPI(`/sso/providers/${id}`, {
      method: "DELETE",
    }),
  getSSOProviderMappings: (id: string) =>
    fetchAPI<SSORoleMapping[]>(`/sso/providers/${id}/mappings`),
  createSSOProviderMapping: (id: string, external_group: string, role: string) =>
    fetchAPI<SSORoleMapping>(`/sso/providers/${id}/mappings`, {
      method: "POST",
      body: JSON.stringify({ external_group, role }),
    }),
  deleteSSOProviderMapping: (providerId: string, mappingId: string) =>
    fetchAPI(`/sso/providers/${providerId}/mappings/${mappingId}`, {
      method: "DELETE",
    }),

  // SSO Public (for login page)
  getPublicSSOProviders: () => fetchAPI<SSOPublicProvider[]>("/auth/sso/providers"),

  // Enterprise Audit & Compliance
  getAuditConfig: () => fetchAPI<AuditConfig>("/audit/config"),

  updateAuditConfig: (data: {
    retention_days?: number;
    retention_policy?: "delete" | "archive" | "export";
    forwarding_enabled?: boolean;
    forwarding_type?: "syslog" | "webhook" | "splunk" | "s3";
    forwarding_config?: Record<string, unknown>;
    compliance_mode?: "soc2" | "hipaa" | "gdpr" | "pci" | null;
    immutable_logs?: boolean;
    hash_chain_enabled?: boolean;
  }) =>
    fetchAPI<{ message: string }>("/audit/config", {
      method: "PUT",
      body: JSON.stringify(data),
    }),

  // Audit Exports
  getAuditExports: () => fetchAPI<AuditExport[]>("/audit/exports"),

  createAuditExport: (data: {
    format: "csv" | "json" | "cef" | "syslog";
    start_date?: string;
    end_date?: string;
    filters?: Record<string, unknown>;
  }) =>
    fetchAPI<{ id: string; status: string; message: string }>("/audit/exports", {
      method: "POST",
      body: JSON.stringify(data),
    }),

  getAuditExport: (id: string) => fetchAPI<AuditExport>(`/audit/exports/${id}`),

  downloadAuditExport: (id: string) => `${API_BASE}/audit/exports/${id}/download`,

  // Compliance Reports
  getComplianceReports: () => fetchAPI<ComplianceReport[]>("/audit/reports"),

  createComplianceReport: (data: {
    report_type: "soc2" | "hipaa" | "access" | "activity" | "security";
    start_date: string;
    end_date: string;
  }) =>
    fetchAPI<{ id: string; status: string; message: string }>("/audit/reports", {
      method: "POST",
      body: JSON.stringify(data),
    }),

  getComplianceReport: (id: string) => fetchAPI<ComplianceReport>(`/audit/reports/${id}`),

  // Forwarding Test
  testAuditForwarding: () =>
    fetchAPI<{ message: string }>("/audit/forwarding/test", {
      method: "POST",
    }),

  // Retention Cleanup
  runRetentionCleanup: () =>
    fetchAPI<{ message: string; deleted: number; cutoff_date: string }>("/audit/retention/cleanup", {
      method: "POST",
    }),

  // Integrity Verification
  verifyAuditIntegrity: (limit?: number) =>
    fetchAPI<IntegrityCheckResult>(`/audit/integrity${limit ? `?limit=${limit}` : ""}`),

  // Organizations (Multi-tenancy)
  getOrganizations: () => fetchAPI<Organization[]>("/orgs"),

  getOrganization: (id: string) => fetchAPI<Organization>(`/orgs/${id}`),

  createOrganization: (data: CreateOrgRequest) =>
    fetchAPI<{ id: string }>("/orgs", {
      method: "POST",
      body: JSON.stringify(data),
    }),

  updateOrganization: (id: string, data: UpdateOrgRequest) =>
    fetchAPI<{ status: string }>(`/orgs/${id}`, {
      method: "PUT",
      body: JSON.stringify(data),
    }),

  deleteOrganization: (id: string) =>
    fetchAPI<{ status: string }>(`/orgs/${id}`, {
      method: "DELETE",
    }),

  getOrganizationUsage: (id: string) => fetchAPI<OrgUsage>(`/orgs/${id}/usage`),

  // Organization Members
  getOrganizationMembers: (orgId: string) =>
    fetchAPI<OrganizationMember[]>(`/orgs/${orgId}/members`),

  addOrganizationMember: (orgId: string, userId: string, role: OrgMemberRole) =>
    fetchAPI<{ id: string }>(`/orgs/${orgId}/members`, {
      method: "POST",
      body: JSON.stringify({ user_id: userId, role }),
    }),

  updateOrganizationMember: (orgId: string, userId: string, role: OrgMemberRole) =>
    fetchAPI<{ status: string }>(`/orgs/${orgId}/members/${userId}`, {
      method: "PUT",
      body: JSON.stringify({ role }),
    }),

  removeOrganizationMember: (orgId: string, userId: string) =>
    fetchAPI<{ status: string }>(`/orgs/${orgId}/members/${userId}`, {
      method: "DELETE",
    }),

  // Organization Invitations
  getOrganizationInvitations: (orgId: string) =>
    fetchAPI<OrganizationInvitation[]>(`/orgs/${orgId}/invitations`),

  createOrganizationInvitation: (orgId: string, data: CreateInvitationRequest) =>
    fetchAPI<{ id: string; token: string; expires_at: string }>(`/orgs/${orgId}/invitations`, {
      method: "POST",
      body: JSON.stringify(data),
    }),

  revokeOrganizationInvitation: (orgId: string, invitationId: string) =>
    fetchAPI<{ status: string }>(`/orgs/${orgId}/invitations/${invitationId}`, {
      method: "DELETE",
    }),

  acceptInvitation: (token: string) =>
    fetchAPI<{ status: string; org_id: string }>(`/invitations/${token}/accept`, {
      method: "POST",
    }),

  // Enrollment Tokens
  getEnrollmentTokens: (orgId: string) =>
    fetchAPI<EnrollmentToken[]>(`/orgs/${orgId}/enrollment-tokens`),

  createEnrollmentToken: (orgId: string, data: CreateEnrollmentTokenRequest) =>
    fetchAPI<{ id: string; token: string }>(`/orgs/${orgId}/enrollment-tokens`, {
      method: "POST",
      body: JSON.stringify(data),
    }),

  revokeEnrollmentToken: (orgId: string, tokenId: string) =>
    fetchAPI<{ status: string }>(`/orgs/${orgId}/enrollment-tokens/${tokenId}/revoke`, {
      method: "PUT",
    }),

  deleteEnrollmentToken: (orgId: string, tokenId: string) =>
    fetchAPI<{ status: string }>(`/orgs/${orgId}/enrollment-tokens/${tokenId}`, {
      method: "DELETE",
    }),

  // Policy Engine
  getPolicies: (options?: { type?: PolicyType; enabled?: boolean }) => {
    const params = new URLSearchParams();
    if (options?.type) params.set("type", options.type);
    if (options?.enabled !== undefined) params.set("enabled", options.enabled.toString());
    const query = params.toString();
    return fetchAPI<Policy[]>(`/policies${query ? `?${query}` : ""}`);
  },

  getPolicy: (id: string) => fetchAPI<Policy>(`/policies/${id}`),

  createPolicy: (data: CreatePolicyRequest) =>
    fetchAPI<{ id: string }>("/policies", {
      method: "POST",
      body: JSON.stringify(data),
    }),

  updatePolicy: (id: string, data: UpdatePolicyRequest) =>
    fetchAPI<{ status: string }>(`/policies/${id}`, {
      method: "PUT",
      body: JSON.stringify(data),
    }),

  deletePolicy: (id: string) =>
    fetchAPI<{ status: string }>(`/policies/${id}`, {
      method: "DELETE",
    }),

  // Policy Templates
  getPolicyTemplates: (category?: string) => {
    const query = category ? `?category=${category}` : "";
    return fetchAPI<PolicyTemplate[]>(`/policies/templates${query}`);
  },

  createPolicyFromTemplate: (templateId: string, data: { name: string; action?: PolicyAction; applies_to?: Record<string, unknown> }) =>
    fetchAPI<{ id: string; template: string }>(`/policies/templates/${templateId}/create`, {
      method: "POST",
      body: JSON.stringify(data),
    }),

  // Policy Violations
  getPolicyViolations: (options?: { resolved?: boolean; policy_id?: string; agent_id?: string }) => {
    const params = new URLSearchParams();
    if (options?.resolved !== undefined) params.set("resolved", options.resolved.toString());
    if (options?.policy_id) params.set("policy_id", options.policy_id);
    if (options?.agent_id) params.set("agent_id", options.agent_id);
    const query = params.toString();
    return fetchAPI<PolicyViolation[]>(`/policies/violations${query ? `?${query}` : ""}`);
  },

  getPolicyViolation: (id: string) => fetchAPI<PolicyViolation>(`/policies/violations/${id}`),

  resolvePolicyViolation: (id: string, resolutionNote: string) =>
    fetchAPI<{ status: string }>(`/policies/violations/${id}/resolve`, {
      method: "POST",
      body: JSON.stringify({ resolution_note: resolutionNote }),
    }),

  getPolicyStats: () => fetchAPI<PolicyStats>("/policies/stats"),

  // Deployments
  getDeployments: (agentId: string, params?: { service?: string; environment?: string; status?: string }) => {
    const searchParams = new URLSearchParams();
    if (params?.service) searchParams.set("service", params.service);
    if (params?.environment) searchParams.set("environment", params.environment);
    if (params?.status) searchParams.set("status", params.status);
    const query = searchParams.toString();
    return fetchAPI<Deployment[]>(`/agents/${agentId}/deployments${query ? `?${query}` : ""}`);
  },

  getDeployment: (agentId: string, deploymentId: string) =>
    fetchAPI<Deployment>(`/agents/${agentId}/deployments/${deploymentId}`),

  createDeployment: (agentId: string, request: CreateDeploymentRequest) =>
    fetchAPI<{ id: string; status: string; message: string }>(`/agents/${agentId}/deployments`, {
      method: "POST",
      body: JSON.stringify(request),
    }),

  rollbackDeployment: (agentId: string, deploymentId: string) =>
    fetchAPI<{ id: string; status: string; message: string }>(`/agents/${agentId}/deployments/${deploymentId}/rollback`, {
      method: "POST",
    }),

  redeployDeployment: (agentId: string, deploymentId: string, pullLatest: boolean = true, skipScanning: boolean = false) =>
    fetchAPI<{ id: string; status: string; message: string }>(`/agents/${agentId}/deployments/${deploymentId}/redeploy`, {
      method: "POST",
      body: JSON.stringify({ pull_latest: pullLatest, skip_scanning: skipScanning }),
    }),

  syncDeploymentStatus: (agentId: string) =>
    fetchAPI<{ message: string; synced: number; checked: number }>(`/agents/${agentId}/deployments/sync`, {
      method: "POST",
    }),

  deleteDeployment: (agentId: string, deploymentId: string) =>
    fetchAPI<{ message: string }>(`/agents/${agentId}/deployments/${deploymentId}`, {
      method: "DELETE",
    }),

  // Managed Stacks (multi-service docker-compose deployments)
  parseComposeYAML: (agentId: string, request: ParseComposeRequest) =>
    fetchAPI<ParsedCompose>(`/agents/${agentId}/stacks/parse`, {
      method: "POST",
      body: JSON.stringify(request),
    }),

  createStack: (agentId: string, request: CreateStackRequest) =>
    fetchAPI<{ id: string; status: string; service_count: number; message: string }>(`/agents/${agentId}/managed-stacks`, {
      method: "POST",
      body: JSON.stringify(request),
    }),

  listManagedStacks: (agentId: string) =>
    fetchAPI<ManagedStack[]>(`/agents/${agentId}/managed-stacks`),

  getManagedStack: (agentId: string, stackId: string) =>
    fetchAPI<ManagedStack>(`/agents/${agentId}/managed-stacks/${stackId}`),

  getStackProgress: (agentId: string, stackId: string) =>
    fetchAPI<StackProgress>(`/agents/${agentId}/managed-stacks/${stackId}/progress`),

  redeployStack: (agentId: string, stackId: string, request: RedeployStackRequest) =>
    fetchAPI<{ id: string; status: string; service_count: number; message: string }>(
      `/agents/${agentId}/managed-stacks/${stackId}/redeploy`,
      { method: "POST", body: JSON.stringify(request) }
    ),

  deleteStack: (agentId: string, stackId: string) =>
    fetchAPI<{ message: string; containers_stopped: number }>(`/agents/${agentId}/managed-stacks/${stackId}`, {
      method: "DELETE",
    }),

  // Services view (cross-agent)
  listServices: () => fetchAPI<ServiceConfig[]>("/services"),
  deployService: (name: string, env: string, body: { image_tag?: string } = {}) =>
    fetchAPI<{ deployment_id: string; service: string; environment: string; image: string; message: string }>(
      `/services/${encodeURIComponent(name)}/deploy?env=${env}`,
      { method: "POST", body: JSON.stringify(body) }
    ),
  deleteService: (name: string, env: string) =>
    fetchAPI<{ message: string }>(`/services/${encodeURIComponent(name)}?env=${env}`, { method: "DELETE" }),

  listServiceDeployments: (serviceName: string) =>
    fetchAPI<Deployment[]>(`/services/${serviceName}/deployments`),

  getCurrentDeployment: (serviceName: string, params?: { environment?: string }) => {
    const searchParams = new URLSearchParams();
    if (params?.environment) searchParams.set("environment", params.environment);
    const query = searchParams.toString();
    return fetchAPI<Deployment>(`/services/${serviceName}/current${query ? `?${query}` : ""}`);
  },

  // Scan Results
  listScans: (params?: { org_id?: string }) => {
    const searchParams = new URLSearchParams();
    if (params?.org_id) searchParams.set("org_id", params.org_id);
    const query = searchParams.toString();
    return fetchAPI<ScanResult[]>(`/scans${query ? `?${query}` : ""}`);
  },

  getScanDetails: (scanId: string) => fetchAPI<ScanResult>(`/scans/${scanId}`),

  getScanVulnerabilities: (scanId: string, params?: { severity?: string }) => {
    const searchParams = new URLSearchParams();
    if (params?.severity) searchParams.set("severity", params.severity);
    const query = searchParams.toString();
    return fetchAPI<Vulnerability[]>(`/scans/${scanId}/vulnerabilities${query ? `?${query}` : ""}`);
  },

  triggerImageScan: (request: { image: string; image_digest?: string; image_tag?: string }) =>
    fetchAPI<{ id: string; image: string; status: string }>("/scans", {
      method: "POST",
      body: JSON.stringify(request),
    }),

  // Batch scan multiple images - scans sequentially and returns all results
  triggerBatchScan: async (
    images: Array<{ image: string; image_digest?: string; image_tag?: string }>,
    onProgress?: (completed: number, total: number, current: string) => void
  ): Promise<Array<{ id: string; image: string; status: string; error?: string }>> => {
    const results: Array<{ id: string; image: string; status: string; error?: string }> = [];
    for (let i = 0; i < images.length; i++) {
      const img = images[i];
      onProgress?.(i, images.length, img.image);
      try {
        const result = await fetchAPI<{ id: string; image: string; status: string }>("/scans", {
          method: "POST",
          body: JSON.stringify(img),
        });
        results.push(result);
      } catch (error) {
        results.push({
          id: "",
          image: img.image,
          status: "failed",
          error: error instanceof Error ? error.message : "Scan failed",
        });
      }
    }
    onProgress?.(images.length, images.length, "");
    return results;
  },

  // Batch generate SBOMs - generates sequentially and returns all results
  generateBatchSBOM: async (
    images: Array<{ image: string }>,
    onProgress?: (completed: number, total: number, current: string) => void
  ): Promise<Array<{ id: string; image: string; format?: string; total_packages?: number; error?: string }>> => {
    const results: Array<{ id: string; image: string; format?: string; total_packages?: number; error?: string }> = [];
    for (let i = 0; i < images.length; i++) {
      const img = images[i];
      onProgress?.(i, images.length, img.image);
      try {
        const result = await fetchAPI<{ id: string; image: string; format: string; total_packages: number }>("/sboms", {
          method: "POST",
          body: JSON.stringify(img),
        });
        results.push(result);
      } catch (error) {
        results.push({
          id: "",
          image: img.image,
          error: error instanceof Error ? error.message : "SBOM generation failed",
        });
      }
    }
    onProgress?.(images.length, images.length, "");
    return results;
  },

  // SBOMs
  listSBOMs: (params?: { org_id?: string }) => {
    const searchParams = new URLSearchParams();
    if (params?.org_id) searchParams.set("org_id", params.org_id);
    const query = searchParams.toString();
    return fetchAPI<SBOM[]>(`/sboms${query ? `?${query}` : ""}`);
  },

  generateSBOM: (request: { image: string }) =>
    fetchAPI<{ id: string; image: string; format: string; total_packages: number }>("/sboms", {
      method: "POST",
      body: JSON.stringify(request),
    }),

  downloadSBOM: (sbomId: string) => {
    const token = typeof window !== "undefined" ? localStorage.getItem("access_token") : null;
    window.open(`${API_BASE}/sboms/${sbomId}/download${token ? `?token=${token}` : ""}`, "_blank");
  },

  // Developer Feedback (Epic 8)
  listFeedback: (params?: { source_type?: string; status?: string; repo?: string }) => {
    const searchParams = new URLSearchParams();
    if (params?.source_type) searchParams.set("source_type", params.source_type);
    if (params?.status) searchParams.set("status", params.status);
    if (params?.repo) searchParams.set("repo", params.repo);
    const query = searchParams.toString();
    return fetchAPI<{ feedbacks: Feedback[]; count: number }>(`/feedback${query ? `?${query}` : ""}`);
  },

  getFeedback: (feedbackId: string) =>
    fetchAPI<Feedback>(`/feedback/${feedbackId}`),

  deliverFeedback: (feedbackId: string) =>
    fetchAPI<{ message: string }>(`/feedback/${feedbackId}/deliver`, {
      method: "POST",
    }),

  // VCS Configuration
  getVCSConfig: (provider: string) =>
    fetchAPI<VCSConfiguration>(`/vcs/${provider}`),

  saveVCSConfig: (provider: string, config: SaveVCSConfigRequest) =>
    fetchAPI<VCSConfiguration>(`/vcs/${provider}`, {
      method: "PUT",
      body: JSON.stringify(config),
    }),

  deleteVCSConfig: (provider: string) =>
    fetchAPI<{ message: string }>(`/vcs/${provider}`, {
      method: "DELETE",
    }),

  // Risk Exceptions (Epic 9)
  listExceptions: (params?: { scope_type?: string; status?: string; scope_reference?: string }) => {
    const searchParams = new URLSearchParams();
    if (params?.scope_type) searchParams.set("scope_type", params.scope_type);
    if (params?.status) searchParams.set("status", params.status);
    if (params?.scope_reference) searchParams.set("scope_reference", params.scope_reference);
    const query = searchParams.toString();
    return fetchAPI<{ exceptions: RiskException[]; count: number }>(`/exceptions${query ? `?${query}` : ""}`);
  },

  getException: (exceptionId: string) =>
    fetchAPI<RiskException>(`/exceptions/${exceptionId}`),

  createException: (request: CreateExceptionRequest) =>
    fetchAPI<RiskException>(`/exceptions`, {
      method: "POST",
      body: JSON.stringify(request),
    }),

  approveException: (exceptionId: string, request?: ApproveExceptionRequest) =>
    fetchAPI<{ message: string }>(`/exceptions/${exceptionId}/approve`, {
      method: "POST",
      body: JSON.stringify(request || {}),
    }),

  denyException: (exceptionId: string, request: DenyExceptionRequest) =>
    fetchAPI<{ message: string }>(`/exceptions/${exceptionId}/deny`, {
      method: "POST",
      body: JSON.stringify(request),
    }),

  revokeException: (exceptionId: string, request: RevokeExceptionRequest) =>
    fetchAPI<{ message: string }>(`/exceptions/${exceptionId}/revoke`, {
      method: "POST",
      body: JSON.stringify(request),
    }),

  getExceptionHistory: (exceptionId: string) =>
    fetchAPI<{ history: any[]; count: number }>(`/exceptions/${exceptionId}/history`),

  // Service Ownership (Epic 10)
  listServiceOwnerships: (params?: { service_name?: string; team_name?: string; status?: string }) => {
    const searchParams = new URLSearchParams();
    if (params?.service_name) searchParams.set("service_name", params.service_name);
    if (params?.team_name) searchParams.set("team_name", params.team_name);
    if (params?.status) searchParams.set("status", params.status);
    const query = searchParams.toString();
    return fetchAPI<{ ownerships: ServiceOwnership[]; count: number }>(`/ownership/services${query ? `?${query}` : ""}`);
  },

  getServiceOwnership: (ownershipId: string) =>
    fetchAPI<ServiceOwnership>(`/ownership/services/${ownershipId}`),

  createServiceOwnership: (request: CreateServiceOwnershipRequest) =>
    fetchAPI<{ id: string; message: string }>(`/ownership/services`, {
      method: "POST",
      body: JSON.stringify(request),
    }),

  updateServiceOwnership: (ownershipId: string, request: CreateServiceOwnershipRequest) =>
    fetchAPI<{ message: string }>(`/ownership/services/${ownershipId}`, {
      method: "PUT",
      body: JSON.stringify(request),
    }),

  deleteServiceOwnership: (ownershipId: string) =>
    fetchAPI<{ message: string }>(`/ownership/services/${ownershipId}`, {
      method: "DELETE",
    }),

  // Teams
  listTeams: (params?: { active?: boolean }) => {
    const searchParams = new URLSearchParams();
    if (params?.active !== undefined) searchParams.set("active", params.active.toString());
    const query = searchParams.toString();
    return fetchAPI<{ teams: Team[]; count: number }>(`/ownership/teams${query ? `?${query}` : ""}`);
  },

  createTeam: (request: CreateTeamRequest) =>
    fetchAPI<{ id: string; message: string }>(`/ownership/teams`, {
      method: "POST",
      body: JSON.stringify(request),
    }),

  updateTeam: (teamId: string, request: CreateTeamRequest) =>
    fetchAPI<{ message: string }>(`/ownership/teams/${teamId}`, {
      method: "PUT",
      body: JSON.stringify(request),
    }),

  deleteTeam: (teamId: string) =>
    fetchAPI<{ message: string }>(`/ownership/teams/${teamId}`, {
      method: "DELETE",
    }),

  // Security Maturity (Epic 11)
  listSecurityScores: (params?: { scope_type?: string; scope_reference?: string; limit?: number }) => {
    const searchParams = new URLSearchParams();
    if (params?.scope_type) searchParams.set("scope_type", params.scope_type);
    if (params?.scope_reference) searchParams.set("scope_reference", params.scope_reference);
    if (params?.limit) searchParams.set("limit", params.limit.toString());
    const query = searchParams.toString();
    return fetchAPI<{ scores: SecurityScore[]; count: number }>(`/maturity/scores${query ? `?${query}` : ""}`);
  },

  getLatestTeamScores: () =>
    fetchAPI<{ team_scores: TeamScore[]; count: number }>(`/maturity/scores/teams`),

  getTeamLeaderboard: () =>
    fetchAPI<{ leaderboard: TeamLeaderboard[]; count: number }>(`/maturity/scores/leaderboard`),

  calculateSecurityScore: (request: CalculateScoreRequest) =>
    fetchAPI<SecurityScore>(`/maturity/scores/calculate`, {
      method: "POST",
      body: JSON.stringify(request),
    }),

  calculateAllTeamScores: () =>
    fetchAPI<{ message: string; total_teams: number; calculated: number }>(`/maturity/scores/calculate-all`, {
      method: "POST",
    }),

  getScoreTrend: (params: { scope_type: string; scope_reference: string }) => {
    const searchParams = new URLSearchParams();
    searchParams.set("scope_type", params.scope_type);
    searchParams.set("scope_reference", params.scope_reference);
    return fetchAPI<{
      scope_type: string;
      scope_reference: string;
      trend: ScoreTrendPoint[];
      trend_direction: string;
      data_points: number;
    }>(`/maturity/scores/trend?${searchParams.toString()}`);
  },

  // Code Quality (Epic 12)
  listCodeQualityResults: (params?: {
    tool?: string;
    project_key?: string;
    commit_sha?: string;
    quality_gate?: string;
    limit?: number;
  }) => {
    const searchParams = new URLSearchParams();
    if (params?.tool) searchParams.set("tool", params.tool);
    if (params?.project_key) searchParams.set("project_key", params.project_key);
    if (params?.commit_sha) searchParams.set("commit_sha", params.commit_sha);
    if (params?.quality_gate) searchParams.set("quality_gate", params.quality_gate);
    if (params?.limit) searchParams.set("limit", params.limit.toString());
    const query = searchParams.toString();
    return fetchAPI<{ results: CodeQualityResult[]; count: number; total: number }>(
      `/code-quality/results${query ? `?${query}` : ""}`
    );
  },

  getCodeQualityResult: (resultId: string) =>
    fetchAPI<CodeQualityResult>(`/code-quality/results/${resultId}`),

  createCodeQualityResult: (request: CreateCodeQualityResultRequest) =>
    fetchAPI<{ id: string; message: string }>(`/code-quality/results`, {
      method: "POST",
      body: JSON.stringify(request),
    }),

  getCodeQualityIssues: (resultId: string, params?: { severity?: string; issue_type?: string }) => {
    const searchParams = new URLSearchParams();
    if (params?.severity) searchParams.set("severity", params.severity);
    if (params?.issue_type) searchParams.set("issue_type", params.issue_type);
    const query = searchParams.toString();
    return fetchAPI<{ issues: CodeQualityIssue[]; count: number }>(
      `/code-quality/results/${resultId}/issues${query ? `?${query}` : ""}`
    );
  },

  getProjectQualitySummary: () =>
    fetchAPI<{ summaries: ProjectQualitySummary[]; count: number }>(
      `/code-quality/summary`
    ),

  getCodeQualityLeaderboard: () =>
    fetchAPI<{ leaderboard: QualityLeaderboard[]; count: number }>(
      `/code-quality/leaderboard`
    ),

  // Code Quality Policies
  listCodeQualityPolicies: () =>
    fetchAPI<{ policies: CodeQualityPolicy[]; count: number }>(
      `/code-quality/policies`
    ),

  createCodeQualityPolicy: (policy: Partial<CodeQualityPolicy>) =>
    fetchAPI<CodeQualityPolicy>(`/code-quality/policies`, {
      method: "POST",
      body: JSON.stringify(policy),
    }),

  updateCodeQualityPolicy: (policyId: string, policy: Partial<CodeQualityPolicy>) =>
    fetchAPI<{ message: string }>(`/code-quality/policies/${policyId}`, {
      method: "PUT",
      body: JSON.stringify(policy),
    }),

  deleteCodeQualityPolicy: (policyId: string) =>
    fetchAPI<{ message: string }>(`/code-quality/policies/${policyId}`, {
      method: "DELETE",
    }),

  getCodeQualityPolicyEvaluations: (resultId: string) =>
    fetchAPI<{ evaluations: PolicyEvaluation[]; count: number }>(
      `/code-quality/results/${resultId}/evaluations`
    ),

  evaluateCodeQualityPolicies: (resultId: string) =>
    fetchAPI<{ results: any[]; count: number }>(
      `/code-quality/results/${resultId}/evaluate`,
      { method: "POST" }
    ),

  // Platform Security (Epic 6)
  getPlatformSecurityConfig: () =>
    fetchAPI<PlatformSecurityConfig>(`/security/config`),

  updatePlatformSecurityConfig: (config: Partial<PlatformSecurityConfig>) =>
    fetchAPI<{ message: string }>(`/security/config`, {
      method: "PUT",
      body: JSON.stringify(config),
    }),

  runSecuritySelfCheck: () =>
    fetchAPI<{ checks: SecurityCheck[]; summary: any }>(`/security/self-check`, {
      method: "POST",
    }),

  getSecurityPosture: () =>
    fetchAPI<SecurityPosture>(`/security/posture`),

  listPolicyViolations: (params?: {
    violation_type?: string;
    severity?: string;
    blocked?: boolean;
    acknowledged?: boolean;
  }) => {
    const searchParams = new URLSearchParams();
    if (params?.violation_type) searchParams.set("violation_type", params.violation_type);
    if (params?.severity) searchParams.set("severity", params.severity);
    if (params?.blocked !== undefined) searchParams.set("blocked", params.blocked.toString());
    if (params?.acknowledged !== undefined) searchParams.set("acknowledged", params.acknowledged.toString());
    const query = searchParams.toString();
    return fetchAPI<{ violations: PolicyViolation[]; count: number }>(
      `/security/violations${query ? `?${query}` : ""}`
    );
  },

  getViolationSummary: () =>
    fetchAPI<{ summaries: ViolationSummary[]; count: number }>(
      `/security/violations/summary`
    ),

  acknowledgeViolation: (violationId: string, notes: string) =>
    fetchAPI<{ message: string }>(`/security/violations/${violationId}/acknowledge`, {
      method: "POST",
      body: JSON.stringify({ notes }),
    }),

  // Runtime Security (Epic 3)
  listDriftEvents: (params?: {
    severity?: string;
    drift_type?: string;
    resolved?: boolean;
    container_name?: string;
    limit?: number;
    offset?: number;
  }) => {
    const searchParams = new URLSearchParams();
    if (params?.severity) searchParams.set("severity", params.severity);
    if (params?.drift_type) searchParams.set("drift_type", params.drift_type);
    if (params?.resolved !== undefined) searchParams.set("resolved", params.resolved.toString());
    if (params?.container_name) searchParams.set("container_name", params.container_name);
    if (params?.limit) searchParams.set("limit", params.limit.toString());
    if (params?.offset) searchParams.set("offset", params.offset.toString());
    const query = searchParams.toString();
    return fetchAPI<{ drift_events: DriftEvent[]; total: number }>(
      `/runtime/drift-events${query ? `?${query}` : ""}`
    );
  },

  getDriftEvent: (eventId: string) =>
    fetchAPI<DriftEvent>(`/runtime/drift-events/${eventId}`),

  resolveDriftEvent: (eventId: string, notes: string) =>
    fetchAPI<{ message: string }>(`/runtime/drift-events/${eventId}/resolve`, {
      method: "POST",
      body: JSON.stringify({ notes }),
    }),

  listBehavioralAnomalies: (params?: {
    severity?: string;
    anomaly_type?: string;
    resolved?: boolean;
    container_name?: string;
    limit?: number;
    offset?: number;
  }) => {
    const searchParams = new URLSearchParams();
    if (params?.severity) searchParams.set("severity", params.severity);
    if (params?.anomaly_type) searchParams.set("anomaly_type", params.anomaly_type);
    if (params?.resolved !== undefined) searchParams.set("resolved", params.resolved.toString());
    if (params?.container_name) searchParams.set("container_name", params.container_name);
    if (params?.limit) searchParams.set("limit", params.limit.toString());
    if (params?.offset) searchParams.set("offset", params.offset.toString());
    const query = searchParams.toString();
    return fetchAPI<{ anomalies: BehavioralAnomaly[]; total: number }>(
      `/runtime/anomalies${query ? `?${query}` : ""}`
    );
  },

  getBehavioralAnomaly: (anomalyId: string) =>
    fetchAPI<BehavioralAnomaly>(`/runtime/anomalies/${anomalyId}`),

  resolveBehavioralAnomaly: (anomalyId: string, notes: string) =>
    fetchAPI<{ message: string }>(`/runtime/anomalies/${anomalyId}/resolve`, {
      method: "POST",
      body: JSON.stringify({ notes }),
    }),

  getRuntimeSecurityConfig: () =>
    fetchAPI<RuntimeSecurityConfig>(`/runtime/config`),

  updateRuntimeSecurityConfig: (config: Partial<RuntimeSecurityConfig>) =>
    fetchAPI<{ message: string }>(`/runtime/config`, {
      method: "PUT",
      body: JSON.stringify(config),
    }),

  getRuntimeSecurityPosture: () =>
    fetchAPI<RuntimeSecurityPosture>(`/runtime/posture`),

  // Container Registries
  getRegistries: () =>
    fetchAPI<ContainerRegistry[]>("/registries"),

  getRegistry: (registryId: string) =>
    fetchAPI<ContainerRegistry>(`/registries/${registryId}`),

  createRegistry: (data: CreateRegistryRequest) =>
    fetchAPI<ContainerRegistry>("/registries", {
      method: "POST",
      body: JSON.stringify(data),
    }),

  updateRegistry: (registryId: string, data: UpdateRegistryRequest) =>
    fetchAPI<{ message: string }>(`/registries/${registryId}`, {
      method: "PUT",
      body: JSON.stringify(data),
    }),

  deleteRegistry: (registryId: string) =>
    fetchAPI<{ message: string }>(`/registries/${registryId}`, {
      method: "DELETE",
    }),

  testRegistryConnection: (registryId: string) =>
    fetchAPI<TestConnectionResponse>(`/registries/${registryId}/test`, {
      method: "POST",
    }),

  getRegistryRepositories: (registryId: string, page: number = 1, pageSize: number = 20) =>
    fetchAPI<ListRepositoriesResponse>(
      `/registries/${registryId}/repositories?page=${page}&page_size=${pageSize}`
    ),

  getRegistryTags: (registryId: string, repository: string, page: number = 1, pageSize: number = 20) =>
    fetchAPI<ListTagsResponse>(
      `/registries/${registryId}/repositories/${encodeURIComponent(repository)}/tags?page=${page}&page_size=${pageSize}`
    ),

  // Epic 13: Traffic & Exposure Governance
  listExposedEndpoints: (params?: {
    exposure_type?: string;
    status?: string;
    min_risk_score?: number;
    limit?: number;
    offset?: number;
  }) => {
    const searchParams = new URLSearchParams();
    if (params?.exposure_type) searchParams.set("exposure_type", params.exposure_type);
    if (params?.status) searchParams.set("status", params.status);
    if (params?.min_risk_score !== undefined) searchParams.set("min_risk_score", params.min_risk_score.toString());
    if (params?.limit) searchParams.set("limit", params.limit.toString());
    if (params?.offset) searchParams.set("offset", params.offset.toString());
    const query = searchParams.toString();
    return fetchAPI<{ endpoints: ExposedEndpoint[]; total: number }>(
      `/exposure/endpoints${query ? `?${query}` : ""}`
    );
  },

  getExposedEndpoint: (endpointId: string) =>
    fetchAPI<ExposedEndpoint>(`/exposure/endpoints/${endpointId}`),

  createExposedEndpoint: (data: CreateExposedEndpointRequest) =>
    fetchAPI<ExposedEndpoint>(`/exposure/endpoints`, {
      method: "POST",
      body: JSON.stringify(data),
    }),

  updateExposedEndpoint: (endpointId: string, data: Partial<ExposedEndpoint>) =>
    fetchAPI<{ message: string }>(`/exposure/endpoints/${endpointId}`, {
      method: "PUT",
      body: JSON.stringify(data),
    }),

  deleteExposedEndpoint: (endpointId: string) =>
    fetchAPI<{ message: string }>(`/exposure/endpoints/${endpointId}`, {
      method: "DELETE",
    }),

  getExposureSummary: () =>
    fetchAPI<ExposureSummary>(`/exposure/summary`),

  getExposureMap: () =>
    fetchAPI<ExposureMap>(`/exposure/map`),

  // Rate Limit Profiles
  listRateLimitProfiles: () =>
    fetchAPI<{ profiles: RateLimitProfile[] }>(`/ratelimits/profiles`),

  createRateLimitProfile: (data: CreateRateLimitProfileRequest) =>
    fetchAPI<RateLimitProfile>(`/ratelimits/profiles`, {
      method: "POST",
      body: JSON.stringify(data),
    }),

  updateRateLimitProfile: (profileId: string, data: Partial<RateLimitProfile>) =>
    fetchAPI<{ message: string }>(`/ratelimits/profiles/${profileId}`, {
      method: "PUT",
      body: JSON.stringify(data),
    }),

  deleteRateLimitProfile: (profileId: string) =>
    fetchAPI<{ message: string }>(`/ratelimits/profiles/${profileId}`, {
      method: "DELETE",
    }),

  // TLS Governance
  getTLSPosture: () =>
    fetchAPI<TLSPosture>(`/tls/posture`),

  listTLSScans: (params?: { domain?: string; limit?: number }) => {
    const searchParams = new URLSearchParams();
    if (params?.domain) searchParams.set("domain", params.domain);
    if (params?.limit) searchParams.set("limit", params.limit.toString());
    const query = searchParams.toString();
    return fetchAPI<{ scans: TLSScanResult[] }>(
      `/tls/scans${query ? `?${query}` : ""}`
    );
  },

  getTLSAlertConfig: () =>
    fetchAPI<TLSAlertConfig>(`/tls/config`),

  updateTLSAlertConfig: (config: Partial<TLSAlertConfig>) =>
    fetchAPI<{ message: string }>(`/tls/config`, {
      method: "PUT",
      body: JSON.stringify(config),
    }),

  // Epic 13 v2: Traffic Resources & Policies
  getTrafficSummary: () =>
    fetchAPI<TrafficResourceSummary>(`/traffic/summary`),

  listTrafficResources: (params?: {
    agent_id?: string;
    status?: string;
    gateway_type?: string;
    limit?: number;
    offset?: number;
  }) => {
    const searchParams = new URLSearchParams();
    if (params?.agent_id) searchParams.set("agent_id", params.agent_id);
    if (params?.status) searchParams.set("status", params.status);
    if (params?.gateway_type) searchParams.set("gateway_type", params.gateway_type);
    if (params?.limit) searchParams.set("limit", params.limit.toString());
    if (params?.offset) searchParams.set("offset", params.offset.toString());
    const query = searchParams.toString();
    return fetchAPI<{ resources: TrafficResource[]; total: number }>(
      `/traffic/resources${query ? `?${query}` : ""}`
    );
  },

  getTrafficResource: (resourceId: string) =>
    fetchAPI<TrafficResource>(`/traffic/resources/${resourceId}`),

  createTrafficResource: (data: CreateTrafficResourceRequest) =>
    fetchAPI<TrafficResource>(`/traffic/resources`, {
      method: "POST",
      body: JSON.stringify(data),
    }),

  updateTrafficResource: (resourceId: string, data: Partial<TrafficResource>) =>
    fetchAPI<{ message: string }>(`/traffic/resources/${resourceId}`, {
      method: "PUT",
      body: JSON.stringify(data),
    }),

  deleteTrafficResource: (resourceId: string) =>
    fetchAPI<{ message: string }>(`/traffic/resources/${resourceId}`, {
      method: "DELETE",
    }),

  // Traffic Upstreams
  listTrafficUpstreams: (resourceId: string) =>
    fetchAPI<{ upstreams: TrafficUpstream[] }>(`/traffic/resources/${resourceId}/upstreams`),

  createTrafficUpstream: (resourceId: string, data: CreateTrafficUpstreamRequest) =>
    fetchAPI<TrafficUpstream>(`/traffic/resources/${resourceId}/upstreams`, {
      method: "POST",
      body: JSON.stringify(data),
    }),

  updateTrafficUpstream: (resourceId: string, upstreamId: string, data: Partial<CreateTrafficUpstreamRequest>) =>
    fetchAPI<TrafficUpstream>(`/traffic/resources/${resourceId}/upstreams/${upstreamId}`, {
      method: "PUT",
      body: JSON.stringify(data),
    }),

  deleteTrafficUpstream: (resourceId: string, upstreamId: string) =>
    fetchAPI<{ message: string }>(`/traffic/resources/${resourceId}/upstreams/${upstreamId}`, {
      method: "DELETE",
    }),

  // Traffic Apply History
  getTrafficApplyHistory: (resourceId: string) =>
    fetchAPI<{ history: TrafficApplyHistory[] }>(`/traffic/resources/${resourceId}/history`),

  // Traffic Resource Apply/Validate/Rollback
  applyTrafficResource: (resourceId: string, force?: boolean) =>
    fetchAPI<ApplyTrafficResponse>(`/traffic/resources/${resourceId}/apply`, {
      method: "POST",
      body: JSON.stringify({ force: force || false }),
    }),

  dryRunTrafficResource: (resourceId: string) =>
    fetchAPI<ApplyTrafficResponse>(`/traffic/resources/${resourceId}/dry-run`, {
      method: "POST",
    }),

  rollbackTrafficResource: (resourceId: string) =>
    fetchAPI<ApplyTrafficResponse>(`/traffic/resources/${resourceId}/rollback`, {
      method: "POST",
    }),

  getTrafficResourceConfigPreview: (resourceId: string) =>
    fetchAPI<TrafficConfigPreview>(`/traffic/resources/${resourceId}/config-preview`),

  // Traffic Policies
  listTrafficPolicies: (params?: { type?: TrafficPolicyType; limit?: number; offset?: number }) => {
    const searchParams = new URLSearchParams();
    if (params?.type) searchParams.set("type", params.type);
    if (params?.limit) searchParams.set("limit", params.limit.toString());
    if (params?.offset) searchParams.set("offset", params.offset.toString());
    const query = searchParams.toString();
    return fetchAPI<{ policies: TrafficPolicy[] }>(
      `/traffic/policies${query ? `?${query}` : ""}`
    );
  },

  getTrafficPolicy: (policyId: string) =>
    fetchAPI<TrafficPolicy>(`/traffic/policies/${policyId}`),

  createTrafficPolicy: (data: CreateTrafficPolicyRequest) =>
    fetchAPI<TrafficPolicy>(`/traffic/policies`, {
      method: "POST",
      body: JSON.stringify(data),
    }),

  updateTrafficPolicy: (policyId: string, data: Partial<TrafficPolicy>) =>
    fetchAPI<{ message: string }>(`/traffic/policies/${policyId}`, {
      method: "PUT",
      body: JSON.stringify(data),
    }),

  deleteTrafficPolicy: (policyId: string) =>
    fetchAPI<{ message: string }>(`/traffic/policies/${policyId}`, {
      method: "DELETE",
    }),

  // Policy Assignment
  assignTrafficPolicy: (policyId: string, data: AssignTrafficPolicyRequest) =>
    fetchAPI<TrafficPolicyAssignment>(`/traffic/policies/${policyId}/assign`, {
      method: "POST",
      body: JSON.stringify(data),
    }),

  listTrafficResourcePolicyAssignments: (resourceId: string) =>
    fetchAPI<{ assignments: TrafficPolicyAssignment[] }>(`/traffic/resources/${resourceId}/policy-assignments`),

  unassignTrafficPolicy: (assignmentId: string) =>
    fetchAPI<{ message: string }>(`/traffic/policy-assignments/${assignmentId}`, {
      method: "DELETE",
    }),

  // Epic 14: Database & Data Governance
  listDatabases: (params?: {
    db_type?: string;
    environment?: string;
    status?: string;
    data_classification?: string;
    limit?: number;
    offset?: number;
  }) => {
    const searchParams = new URLSearchParams();
    if (params?.db_type) searchParams.set("db_type", params.db_type);
    if (params?.environment) searchParams.set("environment", params.environment);
    if (params?.status) searchParams.set("status", params.status);
    if (params?.data_classification) searchParams.set("data_classification", params.data_classification);
    if (params?.limit) searchParams.set("limit", params.limit.toString());
    if (params?.offset) searchParams.set("offset", params.offset.toString());
    const query = searchParams.toString();
    return fetchAPI<{ databases: DatabaseInventoryItem[]; total: number }>(
      `/databases${query ? `?${query}` : ""}`
    );
  },

  getDatabase: (databaseId: string) =>
    fetchAPI<DatabaseInventoryItem>(`/databases/${databaseId}`),

  createDatabase: (data: CreateDatabaseRequest) =>
    fetchAPI<DatabaseInventoryItem>(`/databases`, {
      method: "POST",
      body: JSON.stringify(data),
    }),

  updateDatabase: (databaseId: string, data: Partial<DatabaseInventoryItem>) =>
    fetchAPI<{ message: string }>(`/databases/${databaseId}`, {
      method: "PUT",
      body: JSON.stringify(data),
    }),

  deleteDatabase: (databaseId: string) =>
    fetchAPI<{ message: string }>(`/databases/${databaseId}`, {
      method: "DELETE",
    }),

  getDatabasePosture: () =>
    fetchAPI<DatabasePosture>(`/databases/posture`),

  getDatabaseBackups: (databaseId: string, params?: { status?: string; limit?: number }) => {
    const searchParams = new URLSearchParams();
    if (params?.status) searchParams.set("status", params.status);
    if (params?.limit) searchParams.set("limit", params.limit.toString());
    const query = searchParams.toString();
    return fetchAPI<{ backups: DatabaseBackup[] }>(
      `/databases/${databaseId}/backups${query ? `?${query}` : ""}`
    );
  },

  getDatabaseBackupCompliance: () =>
    fetchAPI<{
      compliance: BackupCompliance[];
      summary: {
        total: number;
        compliant: number;
        warning: number;
        non_compliant: number;
      };
    }>(`/databases/compliance`),

  // Agent-scoped database endpoints
  getAgentDatabases: (agentId: string) =>
    fetchAPI<{ databases: AgentDatabase[] }>(`/agents/${agentId}/databases`),

  addAgentDatabase: (agentId: string, data: AddAgentDatabaseRequest) =>
    fetchAPI<AgentDatabase>(`/agents/${agentId}/databases`, {
      method: "POST",
      body: JSON.stringify(data),
    }),

  removeAgentDatabase: (agentId: string, databaseId: string) =>
    fetchAPI<void>(`/agents/${agentId}/databases/${databaseId}`, {
      method: "DELETE",
    }),

  getAgentDatabaseMetrics: (agentId: string, databaseId: string) =>
    fetchAPI<AgentDatabaseMetrics>(`/agents/${agentId}/databases/${databaseId}/metrics`),

  // Epic 15: Backup & Recovery
  getBackupOverview: () =>
    fetchAPI<BackupOverview>(`/backups/overview`),

  listBackupJobs: (params?: { status?: string; database_id?: string }) => {
    const searchParams = new URLSearchParams();
    if (params?.status) searchParams.set("status", params.status);
    if (params?.database_id) searchParams.set("database_id", params.database_id);
    const query = searchParams.toString();
    return fetchAPI<BackupJob[]>(`/backups/jobs${query ? `?${query}` : ""}`);
  },

  triggerBackup: (data: TriggerBackupRequest) =>
    fetchAPI<{ id: string; status: string; message: string }>(`/backups/jobs`, {
      method: "POST",
      body: JSON.stringify(data),
    }),

  listRestoreOperations: () =>
    fetchAPI<RestoreOperation[]>(`/backups/restores`),

  triggerRestore: (data: TriggerRestoreRequest) =>
    fetchAPI<{ id: string; status: string; message: string }>(`/backups/restores`, {
      method: "POST",
      body: JSON.stringify(data),
    }),

  listRecoveryTests: (params?: { database_id?: string }) => {
    const searchParams = new URLSearchParams();
    if (params?.database_id) searchParams.set("database_id", params.database_id);
    const query = searchParams.toString();
    return fetchAPI<RecoveryTest[]>(`/backups/recovery-tests${query ? `?${query}` : ""}`);
  },

  triggerRecoveryTest: (data: TriggerRecoveryTestRequest) =>
    fetchAPI<{ id: string; status: string; message: string }>(`/backups/recovery-tests`, {
      method: "POST",
      body: JSON.stringify(data),
    }),

  getRecoveryReadiness: () =>
    fetchAPI<RecoveryReadiness[]>(`/backups/readiness`),

  listBackupAlerts: (params?: { status?: string; severity?: string }) => {
    const searchParams = new URLSearchParams();
    if (params?.status) searchParams.set("status", params.status);
    if (params?.severity) searchParams.set("severity", params.severity);
    const query = searchParams.toString();
    return fetchAPI<BackupAlert[]>(`/backups/alerts${query ? `?${query}` : ""}`);
  },

  updateBackupAlertStatus: (alertId: string, data: { status: string; resolution_notes?: string }) =>
    fetchAPI<{ message: string }>(`/backups/alerts/${alertId}`, {
      method: "PUT",
      body: JSON.stringify(data),
    }),

  listBackupStorage: () =>
    fetchAPI<BackupStorage[]>(`/backups/storage`),

  createBackupStorage: (data: CreateBackupStorageRequest) =>
    fetchAPI<{ id: string }>(`/backups/storage`, {
      method: "POST",
      body: JSON.stringify(data),
    }),

  deleteBackupStorage: (storageId: string) =>
    fetchAPI<{ message: string }>(`/backups/storage/${storageId}`, {
      method: "DELETE",
    }),

  // Epic 16: Secrets Hygiene
  listSecrets: (params?: { secret_type?: string; source?: string; exposure_risk?: string; needs_rotation?: boolean }) => {
    const searchParams = new URLSearchParams();
    if (params?.secret_type) searchParams.set("secret_type", params.secret_type);
    if (params?.source) searchParams.set("source", params.source);
    if (params?.exposure_risk) searchParams.set("exposure_risk", params.exposure_risk);
    if (params?.needs_rotation) searchParams.set("needs_rotation", "true");
    const query = searchParams.toString();
    return fetchAPI<SecretInventory[]>(`/secrets${query ? `?${query}` : ""}`);
  },

  getSecret: (secretId: string) =>
    fetchAPI<SecretInventory>(`/secrets/${secretId}`),

  createSecret: (data: CreateSecretRequest) =>
    fetchAPI<{ id: string }>(`/secrets`, {
      method: "POST",
      body: JSON.stringify(data),
    }),

  deleteSecret: (secretId: string) =>
    fetchAPI<{ message: string }>(`/secrets/${secretId}`, {
      method: "DELETE",
    }),

  getSecretsHygieneSummary: () =>
    fetchAPI<SecretsHygieneSummary>(`/secrets/hygiene`),

  listSecretExposures: (params?: { status?: string; severity?: string }) => {
    const searchParams = new URLSearchParams();
    if (params?.status) searchParams.set("status", params.status);
    if (params?.severity) searchParams.set("severity", params.severity);
    const query = searchParams.toString();
    return fetchAPI<SecretExposureEvent[]>(`/secrets/exposures${query ? `?${query}` : ""}`);
  },

  getSecretExposureSummary: () =>
    fetchAPI<SecretExposureSummary>(`/secrets/exposures/summary`),

  updateSecretExposureStatus: (exposureId: string, data: { status: string; resolution_notes?: string }) =>
    fetchAPI<{ message: string }>(`/secrets/exposures/${exposureId}`, {
      method: "PUT",
      body: JSON.stringify(data),
    }),

  listSecretRotationHistory: (params?: { secret_id?: string }) => {
    const searchParams = new URLSearchParams();
    if (params?.secret_id) searchParams.set("secret_id", params.secret_id);
    const query = searchParams.toString();
    return fetchAPI<SecretRotationHistory[]>(`/secrets/rotations${query ? `?${query}` : ""}`);
  },

  listSecretRotationPolicies: () =>
    fetchAPI<SecretRotationPolicy[]>(`/secrets/policies`),

  createSecretRotationPolicy: (data: CreateRotationPolicyRequest) =>
    fetchAPI<{ id: string }>(`/secrets/policies`, {
      method: "POST",
      body: JSON.stringify(data),
    }),

  deleteSecretRotationPolicy: (policyId: string) =>
    fetchAPI<{ message: string }>(`/secrets/policies/${policyId}`, {
      method: "DELETE",
    }),

  listSecretScans: () =>
    fetchAPI<SecretScanResult[]>(`/secrets/scans`),

  triggerSecretScan: (data: TriggerSecretScanRequest) =>
    fetchAPI<{ id: string; status: string; message: string }>(`/secrets/scans`, {
      method: "POST",
      body: JSON.stringify(data),
    }),

  // Secret Migrations (agent-scoped)
  listSecretMigrations: (agentId: string) =>
    fetchAPI<SecretMigration[]>(`/agents/${agentId}/secrets/migrations`),

  getSecretMigration: (agentId: string, migrationId: string) =>
    fetchAPI<SecretMigrationDetails>(`/agents/${agentId}/secrets/migrations/${migrationId}`),

  createSecretMigration: (agentId: string, data: CreateMigrationRequest) =>
    fetchAPI<{ id: string; status: string }>(`/agents/${agentId}/secrets/migrations`, {
      method: "POST",
      body: JSON.stringify(data),
    }),

  rollbackSecretMigration: (agentId: string, migrationId: string) =>
    fetchAPI<{ message: string }>(`/agents/${agentId}/secrets/migrations/${migrationId}/rollback`, {
      method: "POST",
    }),

  // Import secrets to container
  importSecrets: (agentId: string, data: {
    container_id: string;
    container_name: string;
    method: "env_vars" | "docker_secrets";
    secrets: { name: string; value: string }[];
  }) =>
    fetchAPI<{ success: boolean; error?: string; new_container_id?: string }>(`/agents/${agentId}/secrets/import`, {
      method: "POST",
      body: JSON.stringify(data),
    }),

  // Epic 17: Background Jobs & Workers
  listJobs: (params?: {
    status?: string;
    job_type?: string;
    job_category?: string;
    triggered_by?: string;
    limit?: number;
    offset?: number;
  }) => {
    const searchParams = new URLSearchParams();
    if (params?.status) searchParams.set("status", params.status);
    if (params?.job_type) searchParams.set("job_type", params.job_type);
    if (params?.job_category) searchParams.set("job_category", params.job_category);
    if (params?.triggered_by) searchParams.set("triggered_by", params.triggered_by);
    if (params?.limit) searchParams.set("limit", params.limit.toString());
    if (params?.offset) searchParams.set("offset", params.offset.toString());
    const query = searchParams.toString();
    return fetchAPI<BackgroundJob[]>(`/jobs${query ? `?${query}` : ""}`);
  },

  getJob: (jobId: string) =>
    fetchAPI<BackgroundJob>(`/jobs/${jobId}`),

  createJob: (data: CreateJobRequest) =>
    fetchAPI<{ id: string; status: string; message: string }>(`/jobs`, {
      method: "POST",
      body: JSON.stringify(data),
    }),

  cancelJob: (jobId: string) =>
    fetchAPI<{ message: string }>(`/jobs/${jobId}/cancel`, {
      method: "POST",
    }),

  retryJob: (jobId: string) =>
    fetchAPI<{ id: string; message: string }>(`/jobs/${jobId}/retry`, {
      method: "POST",
    }),

  getJobLogs: (jobId: string, params?: { level?: string; limit?: number }) => {
    const searchParams = new URLSearchParams();
    if (params?.level) searchParams.set("level", params.level);
    if (params?.limit) searchParams.set("limit", params.limit.toString());
    const query = searchParams.toString();
    return fetchAPI<JobLog[]>(`/jobs/${jobId}/logs${query ? `?${query}` : ""}`);
  },

  getJobStatistics: () =>
    fetchAPI<JobStatistics[]>(`/jobs/statistics`),

  listSchedules: (params?: { enabled?: boolean; job_type?: string }) => {
    const searchParams = new URLSearchParams();
    if (params?.enabled !== undefined) searchParams.set("enabled", params.enabled.toString());
    if (params?.job_type) searchParams.set("job_type", params.job_type);
    const query = searchParams.toString();
    return fetchAPI<JobSchedule[]>(`/jobs/schedules${query ? `?${query}` : ""}`);
  },

  getSchedule: (scheduleId: string) =>
    fetchAPI<JobSchedule>(`/jobs/schedules/${scheduleId}`),

  createSchedule: (data: CreateScheduleRequest) =>
    fetchAPI<{ id: string; message: string }>(`/jobs/schedules`, {
      method: "POST",
      body: JSON.stringify(data),
    }),

  updateSchedule: (scheduleId: string, data: Partial<CreateScheduleRequest>) =>
    fetchAPI<{ message: string }>(`/jobs/schedules/${scheduleId}`, {
      method: "PUT",
      body: JSON.stringify(data),
    }),

  toggleSchedule: (scheduleId: string, enabled: boolean) =>
    fetchAPI<{ message: string }>(`/jobs/schedules/${scheduleId}/toggle`, {
      method: "POST",
      body: JSON.stringify({ enabled }),
    }),

  deleteSchedule: (scheduleId: string) =>
    fetchAPI<{ message: string }>(`/jobs/schedules/${scheduleId}`, {
      method: "DELETE",
    }),

  listWorkers: (params?: { status?: string; worker_type?: string }) => {
    const searchParams = new URLSearchParams();
    if (params?.status) searchParams.set("status", params.status);
    if (params?.worker_type) searchParams.set("worker_type", params.worker_type);
    const query = searchParams.toString();
    return fetchAPI<Worker[]>(`/jobs/workers${query ? `?${query}` : ""}`);
  },

  getWorkerSummary: () =>
    fetchAPI<WorkerSummary>(`/jobs/workers/summary`),

  // Epic 18: External Dependencies
  listExternalServices: (params?: {
    service_type?: string;
    provider?: string;
    criticality?: string;
    health_status?: string;
    environment?: string;
    limit?: number;
    offset?: number;
  }) => {
    const searchParams = new URLSearchParams();
    if (params?.service_type) searchParams.set("service_type", params.service_type);
    if (params?.provider) searchParams.set("provider", params.provider);
    if (params?.criticality) searchParams.set("criticality", params.criticality);
    if (params?.health_status) searchParams.set("health_status", params.health_status);
    if (params?.environment) searchParams.set("environment", params.environment);
    if (params?.limit) searchParams.set("limit", params.limit.toString());
    if (params?.offset) searchParams.set("offset", params.offset.toString());
    const query = searchParams.toString();
    return fetchAPI<{ services: ExternalService[]; total: number }>(
      `/dependencies${query ? `?${query}` : ""}`
    );
  },

  getExternalService: (serviceId: string) =>
    fetchAPI<ExternalService>(`/dependencies/${serviceId}`),

  createExternalService: (data: CreateExternalServiceRequest) =>
    fetchAPI<{ id: string; message: string }>(`/dependencies`, {
      method: "POST",
      body: JSON.stringify(data),
    }),

  updateExternalService: (serviceId: string, data: Partial<ExternalService>) =>
    fetchAPI<{ message: string }>(`/dependencies/${serviceId}`, {
      method: "PUT",
      body: JSON.stringify(data),
    }),

  deleteExternalService: (serviceId: string) =>
    fetchAPI<{ message: string }>(`/dependencies/${serviceId}`, {
      method: "DELETE",
    }),

  getExternalHealthOverview: () =>
    fetchAPI<ExternalHealthOverview>(`/dependencies/overview`),

  getServiceHealthHistory: (serviceId: string, params?: { limit?: number }) => {
    const searchParams = new URLSearchParams();
    if (params?.limit) searchParams.set("limit", params.limit.toString());
    const query = searchParams.toString();
    return fetchAPI<{ history: ExternalServiceHealth[]; total: number }>(
      `/dependencies/${serviceId}/health${query ? `?${query}` : ""}`
    );
  },

  listServiceDependencies: (params?: {
    source_name?: string;
    external_service_id?: string;
    is_critical?: boolean;
  }) => {
    const searchParams = new URLSearchParams();
    if (params?.source_name) searchParams.set("source_name", params.source_name);
    if (params?.external_service_id) searchParams.set("external_service_id", params.external_service_id);
    if (params?.is_critical !== undefined) searchParams.set("is_critical", params.is_critical.toString());
    const query = searchParams.toString();
    return fetchAPI<{ dependencies: ServiceDependency[]; total: number }>(
      `/dependencies/mappings${query ? `?${query}` : ""}`
    );
  },

  createServiceDependency: (data: CreateServiceDependencyRequest) =>
    fetchAPI<{ id: string; message: string }>(`/dependencies/mappings`, {
      method: "POST",
      body: JSON.stringify(data),
    }),

  deleteServiceDependency: (dependencyId: string) =>
    fetchAPI<{ message: string }>(`/dependencies/mappings/${dependencyId}`, {
      method: "DELETE",
    }),

  getDependencyRisks: (params?: { risk_level?: string }) => {
    const searchParams = new URLSearchParams();
    if (params?.risk_level) searchParams.set("risk_level", params.risk_level);
    const query = searchParams.toString();
    return fetchAPI<{ risks: DependencyRisk[]; total: number }>(
      `/dependencies/risks${query ? `?${query}` : ""}`
    );
  },

  listExternalIncidents: (params?: {
    service_id?: string;
    status?: string;
    severity?: string;
  }) => {
    const searchParams = new URLSearchParams();
    if (params?.service_id) searchParams.set("service_id", params.service_id);
    if (params?.status) searchParams.set("status", params.status);
    if (params?.severity) searchParams.set("severity", params.severity);
    const query = searchParams.toString();
    return fetchAPI<{ incidents: ExternalServiceIncident[]; total: number }>(
      `/dependencies/incidents${query ? `?${query}` : ""}`
    );
  },

  createExternalIncident: (data: CreateExternalIncidentRequest) =>
    fetchAPI<{ id: string; message: string }>(`/dependencies/incidents`, {
      method: "POST",
      body: JSON.stringify(data),
    }),

  updateExternalIncidentStatus: (incidentId: string, data: { status: string; resolution_notes?: string; root_cause?: string }) =>
    fetchAPI<{ message: string }>(`/dependencies/incidents/${incidentId}`, {
      method: "PUT",
      body: JSON.stringify(data),
    }),

  // Epic 7: Research & Metrics
  listMetricDefinitions: (params?: {
    category?: string;
    enabled?: boolean;
    is_primary?: boolean;
    limit?: number;
    offset?: number;
  }) => {
    const searchParams = new URLSearchParams();
    if (params?.category) searchParams.set("category", params.category);
    if (params?.enabled !== undefined) searchParams.set("enabled", params.enabled.toString());
    if (params?.is_primary !== undefined) searchParams.set("is_primary", params.is_primary.toString());
    if (params?.limit) searchParams.set("limit", params.limit.toString());
    if (params?.offset) searchParams.set("offset", params.offset.toString());
    const query = searchParams.toString();
    return fetchAPI<{ metrics: MetricDefinition[]; total: number }>(
      `/metrics${query ? `?${query}` : ""}`
    );
  },

  getMetricDefinition: (metricId: string) =>
    fetchAPI<MetricDefinition>(`/metrics/${metricId}`),

  createMetricDefinition: (data: CreateMetricDefinitionRequest) =>
    fetchAPI<{ id: string; message: string }>(`/metrics`, {
      method: "POST",
      body: JSON.stringify(data),
    }),

  updateMetricDefinition: (metricId: string, data: Partial<CreateMetricDefinitionRequest>) =>
    fetchAPI<{ message: string }>(`/metrics/${metricId}`, {
      method: "PUT",
      body: JSON.stringify(data),
    }),

  deleteMetricDefinition: (metricId: string) =>
    fetchAPI<{ message: string }>(`/metrics/${metricId}`, {
      method: "DELETE",
    }),

  getMetricValues: (metricId: string, params?: {
    start_time?: string;
    end_time?: string;
    agent_id?: string;
    resource_type?: string;
    resource_id?: string;
    limit?: number;
  }) => {
    const searchParams = new URLSearchParams();
    if (params?.start_time) searchParams.set("start_time", params.start_time);
    if (params?.end_time) searchParams.set("end_time", params.end_time);
    if (params?.agent_id) searchParams.set("agent_id", params.agent_id);
    if (params?.resource_type) searchParams.set("resource_type", params.resource_type);
    if (params?.resource_id) searchParams.set("resource_id", params.resource_id);
    if (params?.limit) searchParams.set("limit", params.limit.toString());
    const query = searchParams.toString();
    return fetchAPI<{ values: MetricValue[]; total: number }>(
      `/metrics/${metricId}/values${query ? `?${query}` : ""}`
    );
  },

  getMetricSummary: () =>
    fetchAPI<{ summaries: MetricSummary[] }>(`/metrics/summary`),

  // Dashboards
  listDashboards: (params?: {
    visibility?: string;
    is_default?: boolean;
    created_by?: string;
  }) => {
    const searchParams = new URLSearchParams();
    if (params?.visibility) searchParams.set("visibility", params.visibility);
    if (params?.is_default !== undefined) searchParams.set("is_default", params.is_default.toString());
    if (params?.created_by) searchParams.set("created_by", params.created_by);
    const query = searchParams.toString();
    return fetchAPI<{ dashboards: Dashboard[]; total: number }>(
      `/dashboards${query ? `?${query}` : ""}`
    );
  },

  getDashboard: (dashboardId: string) =>
    fetchAPI<Dashboard>(`/dashboards/${dashboardId}`),

  createDashboard: (data: CreateDashboardRequest) =>
    fetchAPI<{ id: string; message: string }>(`/dashboards`, {
      method: "POST",
      body: JSON.stringify(data),
    }),

  updateDashboard: (dashboardId: string, data: Partial<CreateDashboardRequest>) =>
    fetchAPI<{ message: string }>(`/dashboards/${dashboardId}`, {
      method: "PUT",
      body: JSON.stringify(data),
    }),

  deleteDashboard: (dashboardId: string) =>
    fetchAPI<{ message: string }>(`/dashboards/${dashboardId}`, {
      method: "DELETE",
    }),

  // Reports
  listReports: (params?: {
    report_type?: string;
    schedule_enabled?: boolean;
    visibility?: string;
    limit?: number;
    offset?: number;
  }) => {
    const searchParams = new URLSearchParams();
    if (params?.report_type) searchParams.set("report_type", params.report_type);
    if (params?.schedule_enabled !== undefined) searchParams.set("schedule_enabled", params.schedule_enabled.toString());
    if (params?.visibility) searchParams.set("visibility", params.visibility);
    if (params?.limit) searchParams.set("limit", params.limit.toString());
    if (params?.offset) searchParams.set("offset", params.offset.toString());
    const query = searchParams.toString();
    return fetchAPI<{ reports: Report[]; total: number }>(
      `/reports${query ? `?${query}` : ""}`
    );
  },

  getReport: (reportId: string) =>
    fetchAPI<Report>(`/reports/${reportId}`),

  createReport: (data: CreateReportRequest) =>
    fetchAPI<{ id: string; message: string }>(`/reports`, {
      method: "POST",
      body: JSON.stringify(data),
    }),

  updateReport: (reportId: string, data: Partial<CreateReportRequest>) =>
    fetchAPI<{ message: string }>(`/reports/${reportId}`, {
      method: "PUT",
      body: JSON.stringify(data),
    }),

  deleteReport: (reportId: string) =>
    fetchAPI<{ message: string }>(`/reports/${reportId}`, {
      method: "DELETE",
    }),

  runReport: (reportId: string) =>
    fetchAPI<{ execution_id: string; message: string }>(`/reports/${reportId}/run`, {
      method: "POST",
    }),

  listReportExecutions: (params?: {
    report_id?: string;
    status?: string;
    limit?: number;
    offset?: number;
  }) => {
    const searchParams = new URLSearchParams();
    if (params?.report_id) searchParams.set("report_id", params.report_id);
    if (params?.status) searchParams.set("status", params.status);
    if (params?.limit) searchParams.set("limit", params.limit.toString());
    if (params?.offset) searchParams.set("offset", params.offset.toString());
    const query = searchParams.toString();
    return fetchAPI<{ executions: ReportExecution[]; total: number }>(
      `/reports/executions${query ? `?${query}` : ""}`
    );
  },

  getReportExecution: (executionId: string) =>
    fetchAPI<ReportExecution>(`/reports/executions/${executionId}`),

  // Telemetry Settings
  getTelemetrySettings: () =>
    fetchAPI<TelemetrySettings>(`/telemetry/settings`),

  updateTelemetrySettings: (data: UpdateTelemetrySettingsRequest) =>
    fetchAPI<{ message: string }>(`/telemetry/settings`, {
      method: "PUT",
      body: JSON.stringify(data),
    }),

  // Self-update (CE)
  getVersion: () =>
    fetchAPI<{ version: string; edition: string }>("/version"),

  checkForUpdate: () =>
    fetchAPI<{
      current_version: string;
      latest_pushed_at?: string;
      update_available: boolean;
      error?: string;
    }>(`/settings/update/check`),

  applyUpdate: () =>
    fetchAPI<{ status: string; message: string }>(`/settings/update/apply`, {
      method: "POST",
    }),

  // Anonymous funnel telemetry opt-out (v3/40 G1b)
  getPrivacySettings: () =>
    fetchAPI<{ telemetry_enabled: boolean }>("/settings/privacy"),

  updatePrivacySettings: (telemetryEnabled: boolean) =>
    fetchAPI<{ telemetry_enabled: boolean }>("/settings/privacy", {
      method: "PUT",
      body: JSON.stringify({ telemetry_enabled: telemetryEnabled }),
    }),

  // Generic fetch for custom endpoints
  fetchAPI: <T>(endpoint: string, options?: RequestInit) => fetchAPI<T>(endpoint, options),

  // Sensitive operation verification
  verifyPassword: (password: string) =>
    fetchAPI<{ valid: boolean; error?: string }>(`/auth/verify-password`, {
      method: "POST",
      body: JSON.stringify({ password }),
    }),

  sendConfirmationOTP: () =>
    fetchAPI<{ success: boolean; message: string; error?: string }>(`/auth/send-confirmation-otp`, {
      method: "POST",
    }),

  verifyConfirmationOTP: (otp: string) =>
    fetchAPI<{ valid: boolean; error?: string }>(`/auth/verify-confirmation-otp`, {
      method: "POST",
      body: JSON.stringify({ otp }),
    }),

  // API Keys
  listAPIKeys: () =>
    fetchAPI<APIKey[]>(`/api-keys`),

  createAPIKey: (name: string, expiresAt?: string) =>
    fetchAPI<APIKeyWithSecret>(`/api-keys`, {
      method: "POST",
      body: JSON.stringify({ name, expires_at: expiresAt ?? null }),
    }),

  deleteAPIKey: (id: string) =>
    fetchAPI<{ message: string }>(`/api-keys/${id}`, { method: "DELETE" }),
};
