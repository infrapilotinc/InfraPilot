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
};
