const API_BASE = process.env.NEXT_PUBLIC_API_URL || "/api/v1";

async function fetchAPI<T>(
  endpoint: string,
  options: RequestInit = {}
): Promise<T> {
  const token =
    typeof window !== "undefined" ? localStorage.getItem("access_token") : null;

  const res = await fetch(`${API_BASE}${endpoint}`, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...options.headers,
    },
  });

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
  ssl_enabled: boolean;
  ssl_expires_at: string | null;
  force_ssl: boolean;
  http2_enabled: boolean;
  status: string;
  created_at: string;
  updated_at: string;
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
  channel_type: "smtp" | "slack" | "webhook";
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

// Setup types
export interface SetupStatusResponse {
  setup_required: boolean;
  user_count: number;
}

export interface SetupResponse {
  message: string;
  user_id: string;
  access_token?: string;
  refresh_token?: string;
}

// System Settings types
export interface InfraPilotDomainSettings {
  domain: string;
  ssl_enabled: boolean;
  force_ssl: boolean;
  http2_enabled: boolean;
  proxy_host_id?: string;
  status?: string;
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

// API methods
export const api = {
  // Setup (first-run)
  getSetupStatus: () => fetchAPI<SetupStatusResponse>("/setup/status"),

  createInitialAdmin: (email: string, password: string) =>
    fetchAPI<SetupResponse>("/setup", {
      method: "POST",
      body: JSON.stringify({ email, password }),
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
      upstream_target: string;
      force_ssl?: boolean;
      http2_enabled?: boolean;
    }
  ) =>
    fetchAPI<ProxyHost>(`/agents/${agentId}/proxies`, {
      method: "POST",
      body: JSON.stringify(data),
    }),

  updateProxyHost: (
    agentId: string,
    proxyId: string,
    data: Partial<{
      domain: string;
      upstream_target: string;
      force_ssl: boolean;
      http2_enabled: boolean;
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

  // Alerts - Channels
  getAlertChannels: () => fetchAPI<AlertChannel[]>("/alerts/channels"),

  createAlertChannel: (data: {
    name: string;
    channel_type: "smtp" | "slack" | "webhook";
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
  }) =>
    fetchAPI<InfraPilotDomainSettings>("/settings/domain", {
      method: "PUT",
      body: JSON.stringify(data),
    }),

  deleteInfraPilotDomain: () =>
    fetchAPI<{ message: string }>("/settings/domain", {
      method: "DELETE",
    }),

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
};
