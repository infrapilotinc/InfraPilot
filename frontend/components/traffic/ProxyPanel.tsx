"use client";

import { useState, useEffect } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Globe,
  Lock,
  KeyRound,
  Shield,
  ShieldCheck,
  Trash2,
  Pencil,
  FileText,
  RefreshCw,
  Check,
  AlertTriangle,
  Settings,
  Activity,
  Loader2,
  Play,
  Save,
  Copy,
} from "lucide-react";
import { api, ProxyHost, SecurityHeaders, RateLimit } from "@/lib/api";
import { useTraffic, getSSLStatus, getProxyStatus } from "@/lib/traffic-context";
import { formatRelativeTime, cn } from "@/lib/utils";
import { Badge } from "@/components/ui/Badge";
import { StatusIndicator } from "@/components/ui/StatusIndicator";
import { SlideOver } from "@/components/ui/SlideOver";
import { ConfirmDialog } from "@/components/ui/ConfirmDialog";
import { Spinner } from "@/components/ui/Spinner";
import { SSLWizard } from "@/components/ssl-wizard";
import {
  Button,
  Input,
} from "@/components/ui/page-layout";

// Import shared components
import {
  Section,
  ConfigPreviewSection,
} from "@/components/traffic/DomainConfigPanel";

type PanelTab = "config" | "logs";

export function ProxyPanel() {
  const queryClient = useQueryClient();
  const { selectedAgent, proxyPanelId, closeProxyPanel } = useTraffic();

  // Local state
  const [tab, setTab] = useState<PanelTab>("config");
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [logsTail, setLogsTail] = useState(100);
  const [hasChanges, setHasChanges] = useState(false);

  // SSL Wizard state
  const [showSSLWizard, setShowSSLWizard] = useState(false);

  // Edit form state
  const [editProxy, setEditProxy] = useState({
    domain: "",
    upstream_target: "",
    redirect_url: "",
    redirect_code: 301,
    proxy_type: "upstream" as "upstream" | "redirect",
    force_ssl: true,
    http2_enabled: true,
    include_www: false,
    access_log: true,
    error_log: true,
    bearer_auth_enabled: false,
  });

  // Auth users state
  const [authUsers, setAuthUsers] = useState<{ id: string; username: string }[]>([]);
  const [newAuthUser, setNewAuthUser] = useState({ username: "", password: "" });

  // Bearer tokens state
  const [bearerTokens, setBearerTokens] = useState<{ id: string; name: string; created_at: string }[]>([]);
  const [newTokenName, setNewTokenName] = useState("");
  // Holds the plaintext value right after creation -- shown once, never fetchable again.
  const [justCreatedToken, setJustCreatedToken] = useState("");

  // Security headers state
  const [securityHeaders, setSecurityHeaders] = useState<SecurityHeaders>({
    hsts_enabled: false,
    hsts_max_age: 31536000,
    x_frame_options: "SAMEORIGIN",
    x_content_type_options: true,
    x_xss_protection: true,
    content_security_policy: null,
  });

  // Rate limits state
  const [rateLimits, setRateLimits] = useState<RateLimit[]>([]);
  const [newRateLimit, setNewRateLimit] = useState({
    zone_name: "default",
    requests_per: 100,
    time_window: "1m",
    burst: 50,
    enabled: true,
  });
  const [editingRateLimit, setEditingRateLimit] = useState<RateLimit | null>(null);

  // Reset tab when panel opens
  useEffect(() => {
    if (proxyPanelId) {
      setTab("config");
      setHasChanges(false);
    }
  }, [proxyPanelId]);

  // Fetch proxy details
  const { data: proxy, isLoading: proxyLoading } = useQuery({
    queryKey: ["proxy-detail", selectedAgent, proxyPanelId],
    queryFn: async () => {
      if (!selectedAgent || !proxyPanelId) return null;
      const proxies = await api.getProxyHosts(selectedAgent);
      return proxies.find((p: ProxyHost) => p.id === proxyPanelId) || null;
    },
    enabled: !!selectedAgent && !!proxyPanelId,
  });

  // Fetch nginx config
  const { data: configData, isLoading: configLoading, refetch: refetchConfig } = useQuery({
    queryKey: ["proxy-config", selectedAgent, proxyPanelId],
    queryFn: async () => {
      if (!selectedAgent || !proxyPanelId) return null;
      const response = await fetch(`/api/v1/agents/${selectedAgent}/proxies/${proxyPanelId}/config`, {
        headers: { Authorization: `Bearer ${localStorage.getItem("access_token")}` },
      });
      return response.json();
    },
    enabled: !!selectedAgent && !!proxyPanelId && tab === "config",
  });

  // Fetch proxy logs
  const { data: logsData, isLoading: logsLoading, refetch: refetchLogs } = useQuery({
    queryKey: ["proxy-logs", selectedAgent, proxyPanelId, logsTail],
    queryFn: async () => {
      if (!selectedAgent || !proxyPanelId || !proxy?.domain) return null;
      const response = await fetch(
        `/api/v1/agents/${selectedAgent}/nginx/logs?domain=${encodeURIComponent(proxy.domain)}&tail=${logsTail}`,
        {
          headers: { Authorization: `Bearer ${localStorage.getItem("access_token")}` },
        }
      );
      return response.json();
    },
    enabled: !!selectedAgent && !!proxyPanelId && !!proxy?.domain && tab === "logs",
  });

  // Load proxy details when selected
  useEffect(() => {
    if (proxy && selectedAgent) {
      setEditProxy({
        domain: proxy.domain,
        upstream_target: proxy.upstream_target || "",
        redirect_url: proxy.redirect_url || "",
        redirect_code: proxy.redirect_code || 301,
        proxy_type: proxy.proxy_type || "upstream",
        force_ssl: proxy.force_ssl,
        http2_enabled: proxy.http2_enabled,
        include_www: proxy.include_www,
        access_log: proxy.access_log ?? true,
        error_log: proxy.error_log ?? true,
        bearer_auth_enabled: proxy.bearer_auth_enabled ?? false,
      });
      setJustCreatedToken("");

      // Load auth users
      api.listAuthUsers(selectedAgent, proxy.id)
        .then((users) => {
          setAuthUsers(users.map(u => ({ id: u.id, username: u.username })));
        })
        .catch(() => {
          setAuthUsers([]);
        });

      // Load bearer tokens
      api.listBearerTokens(selectedAgent, proxy.id)
        .then((tokens) => {
          setBearerTokens(tokens.map(t => ({ id: t.id, name: t.name, created_at: t.created_at })));
        })
        .catch(() => {
          setBearerTokens([]);
        });

      // Load security headers
      api.getSecurityHeaders(selectedAgent, proxy.id)
        .then((headers) => {
          setSecurityHeaders({
            hsts_enabled: headers.hsts_enabled,
            hsts_max_age: headers.hsts_max_age,
            x_frame_options: headers.x_frame_options,
            x_content_type_options: headers.x_content_type_options,
            x_xss_protection: headers.x_xss_protection,
            content_security_policy: headers.content_security_policy ?? null,
          });
        })
        .catch(() => {
          setSecurityHeaders({
            hsts_enabled: false,
            hsts_max_age: 31536000,
            x_frame_options: "SAMEORIGIN",
            x_content_type_options: true,
            x_xss_protection: true,
            content_security_policy: null,
          });
        });

      // Load rate limits
      api.getRateLimits(selectedAgent, proxy.id)
        .then(setRateLimits)
        .catch(() => setRateLimits([]));

      setHasChanges(false);
    }
  }, [proxy, selectedAgent]);

  // Mutations
  const invalidateProxies = () => {
    queryClient.invalidateQueries({ queryKey: ["proxies", selectedAgent] });
    queryClient.invalidateQueries({ queryKey: ["proxy-detail", selectedAgent, proxyPanelId] });
  };

  const updateMutation = useMutation({
    mutationFn: ({ proxyId, data }: { proxyId: string; data: Partial<typeof editProxy> }) =>
      api.updateProxyHost(selectedAgent!, proxyId, data),
    onSuccess: () => {
      invalidateProxies();
      setHasChanges(false);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (proxyId: string) => api.deleteProxyHost(selectedAgent!, proxyId),
    onSuccess: () => {
      invalidateProxies();
      setShowDeleteConfirm(false);
      closeProxyPanel();
    },
  });

  const testConfigMutation = useMutation({
    mutationFn: () => api.testNginxConfig(selectedAgent!),
  });

  const createAuthUserMutation = useMutation({
    mutationFn: (data: { username: string; password: string }) =>
      api.createAuthUser(selectedAgent!, proxy!.id, data),
    onSuccess: (user) => {
      setAuthUsers([...authUsers, { id: user.id, username: user.username }]);
      setNewAuthUser({ username: "", password: "" });
      invalidateProxies();
    },
  });

  const deleteAuthUserMutation = useMutation({
    mutationFn: (userId: string) =>
      api.deleteAuthUser(selectedAgent!, proxy!.id, userId),
    onSuccess: (_, userId) => {
      setAuthUsers(authUsers.filter(u => u.id !== userId));
      invalidateProxies();
    },
  });

  const createBearerTokenMutation = useMutation({
    mutationFn: (data: { name: string }) =>
      api.createBearerToken(selectedAgent!, proxy!.id, data),
    onSuccess: (created) => {
      setBearerTokens([...bearerTokens, { id: created.id, name: created.name, created_at: created.created_at }]);
      setNewTokenName("");
      setJustCreatedToken(created.token);
      invalidateProxies();
    },
  });

  const deleteBearerTokenMutation = useMutation({
    mutationFn: (tokenId: string) =>
      api.deleteBearerToken(selectedAgent!, proxy!.id, tokenId),
    onSuccess: (_, tokenId) => {
      setBearerTokens(bearerTokens.filter(t => t.id !== tokenId));
      invalidateProxies();
    },
  });

  const securityHeadersMutation = useMutation({
    mutationFn: ({ proxyId, data }: { proxyId: string; data: Omit<SecurityHeaders, "id" | "proxy_host_id"> }) =>
      api.updateSecurityHeaders(selectedAgent!, proxyId, data),
    onSuccess: invalidateProxies,
  });

  const createRateLimitMutation = useMutation({
    mutationFn: (data: typeof newRateLimit) =>
      api.createRateLimit(selectedAgent!, proxy!.id, data),
    onSuccess: (createdLimit) => {
      setRateLimits([createdLimit, ...rateLimits]);
      setNewRateLimit({ zone_name: "default", requests_per: 100, time_window: "1m", burst: 50, enabled: true });
    },
  });

  const updateRateLimitMutation = useMutation({
    mutationFn: ({ id, data }: { id: string; data: typeof newRateLimit }) =>
      api.updateRateLimit(selectedAgent!, proxy!.id, id, data),
    onSuccess: (updatedLimit) => {
      setRateLimits(rateLimits.map((rl) => (rl.id === updatedLimit.id ? updatedLimit : rl)));
      setEditingRateLimit(null);
    },
  });

  const deleteRateLimitMutation = useMutation({
    mutationFn: (id: string) => api.deleteRateLimit(selectedAgent!, proxy!.id, id),
    onSuccess: (_, deletedId) => {
      setRateLimits(rateLimits.filter((rl) => rl.id !== deletedId));
    },
  });

  const panelTabs = [
    { id: "config", label: "Configuration", icon: Settings },
    { id: "logs", label: "Logs", icon: FileText },
  ] as { id: PanelTab; label: string; icon: React.ElementType }[];

  // Handle save all changes
  const handleSaveAll = async () => {
    if (!proxy || !selectedAgent) return;

    try {
      // Update proxy settings
      await updateMutation.mutateAsync({ proxyId: proxy.id, data: editProxy });

      // Update security headers
      await securityHeadersMutation.mutateAsync({ proxyId: proxy.id, data: securityHeaders });

      setHasChanges(false);
    } catch (error) {
      console.error("Failed to save changes:", error);
    }
  };

  // Mark form as changed
  const markChanged = () => setHasChanges(true);

  // Render tab content
  const renderTabContent = () => {
    if (!proxy) return null;

    switch (tab) {
      case "logs":
        return (
          <div className="flex flex-col -mx-6 h-[calc(100vh-300px)]">
            <div className="flex items-center justify-between px-6 py-2 bg-gray-800 border-b border-gray-700 flex-shrink-0">
              <div className="flex items-center gap-2">
                <span className="text-xs text-gray-400">Domain:</span>
                <code className="text-xs text-green-400">{proxy.domain}</code>
              </div>
              <div className="flex items-center gap-2">
                <select
                  value={logsTail}
                  onChange={(e) => setLogsTail(Number(e.target.value))}
                  className="px-2 py-1 bg-gray-700 border border-gray-600 rounded text-xs text-gray-200"
                >
                  <option value={50}>50 lines</option>
                  <option value={100}>100 lines</option>
                  <option value={500}>500 lines</option>
                  <option value={1000}>1000 lines</option>
                </select>
                <button onClick={() => refetchLogs()} className="p-1.5 hover:bg-gray-700 rounded transition-colors">
                  <RefreshCw className={cn("h-3.5 w-3.5 text-gray-400", logsLoading && "animate-spin")} />
                </button>
              </div>
            </div>
            <div className="flex-1 bg-gray-900 overflow-y-auto min-h-0 px-6">
              {logsLoading ? (
                <div className="flex items-center justify-center h-32">
                  <Spinner size="lg" />
                </div>
              ) : logsData?.logs ? (
                <div className="font-mono text-xs py-2">
                  {logsData.logs
                    .split("\n")
                    .filter((l: string) => l.trim())
                    .map((line: string, idx: number) => (
                      <div key={idx} className="text-gray-300 hover:bg-gray-800/50 py-0.5 px-3 -mx-3">
                        {line}
                      </div>
                    ))}
                </div>
              ) : (
                <div className="text-center text-gray-500 py-8 text-sm">
                  No logs available for this domain
                </div>
              )}
            </div>
          </div>
        );

      default:
        // Configuration tab - unified settings view
        return (
          <div className="space-y-4">
            {/* Basic Settings Section */}
            <Section title="Basic Settings" icon={Globe} defaultOpen={true}>
              <div className="space-y-4">
                <div>
                  <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Domain</label>
                  <input
                    type="text"
                    value={editProxy.domain}
                    onChange={(e) => { setEditProxy({ ...editProxy, domain: e.target.value }); markChanged(); }}
                    className="w-full px-3 py-2 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-600 rounded-lg text-sm"
                  />
                </div>

                {editProxy.proxy_type === "redirect" ? (
                  <>
                    <div>
                      <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Redirect URL</label>
                      <input
                        type="text"
                        value={editProxy.redirect_url}
                        onChange={(e) => { setEditProxy({ ...editProxy, redirect_url: e.target.value }); markChanged(); }}
                        placeholder="https://example.com"
                        className="w-full px-3 py-2 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-600 rounded-lg text-sm"
                      />
                    </div>
                    <div>
                      <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Redirect Code</label>
                      <select
                        value={editProxy.redirect_code}
                        onChange={(e) => { setEditProxy({ ...editProxy, redirect_code: parseInt(e.target.value) }); markChanged(); }}
                        className="w-full px-3 py-2 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-600 rounded-lg text-sm"
                      >
                        <option value={301}>301 - Permanent Redirect</option>
                        <option value={302}>302 - Temporary Redirect</option>
                        <option value={307}>307 - Temporary (preserve method)</option>
                        <option value={308}>308 - Permanent (preserve method)</option>
                      </select>
                    </div>
                  </>
                ) : (
                  <div>
                    <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Upstream Target</label>
                    <input
                      type="text"
                      value={editProxy.upstream_target}
                      onChange={(e) => { setEditProxy({ ...editProxy, upstream_target: e.target.value }); markChanged(); }}
                      placeholder="http://backend:3000"
                      className="w-full px-3 py-2 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-600 rounded-lg text-sm font-mono"
                    />
                  </div>
                )}

                {/* Toggle options */}
                <div className="grid grid-cols-2 gap-3">
                  <label className="flex items-center gap-2 p-3 border border-gray-200 dark:border-gray-700 rounded-lg cursor-pointer hover:bg-gray-50 dark:hover:bg-gray-800/50">
                    <input
                      type="checkbox"
                      checked={editProxy.force_ssl}
                      onChange={(e) => { setEditProxy({ ...editProxy, force_ssl: e.target.checked }); markChanged(); }}
                      className="w-4 h-4 rounded text-primary-600"
                    />
                    <div>
                      <span className="text-sm font-medium text-gray-900 dark:text-white">Force HTTPS</span>
                      <p className="text-xs text-gray-500">Redirect HTTP to HTTPS</p>
                    </div>
                  </label>

                  <label className="flex items-center gap-2 p-3 border border-gray-200 dark:border-gray-700 rounded-lg cursor-pointer hover:bg-gray-50 dark:hover:bg-gray-800/50">
                    <input
                      type="checkbox"
                      checked={editProxy.http2_enabled}
                      onChange={(e) => { setEditProxy({ ...editProxy, http2_enabled: e.target.checked }); markChanged(); }}
                      className="w-4 h-4 rounded text-primary-600"
                    />
                    <div>
                      <span className="text-sm font-medium text-gray-900 dark:text-white">HTTP/2</span>
                      <p className="text-xs text-gray-500">Enable HTTP/2 protocol</p>
                    </div>
                  </label>

                  <label className="flex items-center gap-2 p-3 border border-gray-200 dark:border-gray-700 rounded-lg cursor-pointer hover:bg-gray-50 dark:hover:bg-gray-800/50">
                    <input
                      type="checkbox"
                      checked={editProxy.include_www}
                      onChange={(e) => { setEditProxy({ ...editProxy, include_www: e.target.checked }); markChanged(); }}
                      className="w-4 h-4 rounded text-primary-600"
                    />
                    <div>
                      <span className="text-sm font-medium text-gray-900 dark:text-white">Include www</span>
                      <p className="text-xs text-gray-500">Also serve www subdomain</p>
                    </div>
                  </label>

                  <label className="flex items-center gap-2 p-3 border border-gray-200 dark:border-gray-700 rounded-lg cursor-pointer hover:bg-gray-50 dark:hover:bg-gray-800/50">
                    <input
                      type="checkbox"
                      checked={editProxy.access_log}
                      onChange={(e) => { setEditProxy({ ...editProxy, access_log: e.target.checked }); markChanged(); }}
                      className="w-4 h-4 rounded text-primary-600"
                    />
                    <div>
                      <span className="text-sm font-medium text-gray-900 dark:text-white">Access Log</span>
                      <p className="text-xs text-gray-500">Log all requests</p>
                    </div>
                  </label>
                </div>
              </div>
            </Section>

            {/* SSL Section */}
            <Section
              title="SSL / TLS"
              icon={Lock}
              defaultOpen={true}
              badge={
                proxy.ssl_enabled ? (
                  <Badge color="green" size="sm">Enabled</Badge>
                ) : (
                  <Badge color="gray" size="sm">Not configured</Badge>
                )
              }
            >
              <div className="space-y-3">
                <StatusIndicator
                  status={getSSLStatus(proxy)}
                  label={proxy.ssl_enabled
                    ? proxy.ssl_expires_at
                      ? `Expires ${formatRelativeTime(proxy.ssl_expires_at)}`
                      : "SSL Enabled"
                    : proxy.status === "ssl_pending"
                    ? "SSL Certificate Pending"
                    : "SSL Not Enabled"}
                />
                {!proxy.ssl_enabled && proxy.status !== "ssl_pending" && (
                  <Button
                    variant="primary"
                    size="sm"
                    icon={ShieldCheck}
                    onClick={() => setShowSSLWizard(true)}
                  >
                    Setup SSL Certificate
                  </Button>
                )}
              </div>
            </Section>

            {/* Basic Authentication Section */}
            <Section
              title="Basic Authentication"
              icon={Lock}
              defaultOpen={false}
              badge={authUsers.length > 0 ? <Badge color="green" size="sm">{authUsers.length} user{authUsers.length > 1 ? "s" : ""}</Badge> : undefined}
            >
              <div className="space-y-3">
                {/* Existing users */}
                {authUsers.map((user) => (
                  <div key={user.id} className="flex items-center justify-between p-3 border border-gray-200 dark:border-gray-700 rounded-lg">
                    <div className="flex items-center gap-2">
                      <Lock className="h-4 w-4 text-gray-400" />
                      <span className="text-sm font-medium text-gray-900 dark:text-white">{user.username}</span>
                    </div>
                    <button
                      onClick={() => deleteAuthUserMutation.mutate(user.id)}
                      disabled={deleteAuthUserMutation.isPending}
                      className="p-1.5 text-gray-400 hover:text-red-500 rounded"
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </button>
                  </div>
                ))}

                {/* Add user form */}
                <div className="border border-dashed border-gray-300 dark:border-gray-600 rounded-lg p-3">
                  <p className="text-xs font-medium text-gray-500 mb-2">Add User</p>
                  <div className="space-y-2">
                    <input
                      type="text"
                      value={newAuthUser.username}
                      onChange={(e) => setNewAuthUser({ ...newAuthUser, username: e.target.value })}
                      placeholder="Username"
                      className="w-full px-2 py-1.5 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-600 rounded text-sm"
                    />
                    <input
                      type="password"
                      value={newAuthUser.password}
                      onChange={(e) => setNewAuthUser({ ...newAuthUser, password: e.target.value })}
                      placeholder="Password"
                      className="w-full px-2 py-1.5 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-600 rounded text-sm"
                    />
                    <button
                      onClick={() => {
                        if (newAuthUser.username && newAuthUser.password) {
                          createAuthUserMutation.mutate(newAuthUser);
                        }
                      }}
                      disabled={!newAuthUser.username || !newAuthUser.password || createAuthUserMutation.isPending}
                      className="w-full px-2 py-1.5 text-xs bg-primary-600 text-white rounded hover:bg-primary-700 disabled:opacity-50"
                    >
                      {createAuthUserMutation.isPending ? "Adding..." : "Add User"}
                    </button>
                  </div>
                </div>

                {authUsers.length === 0 && (
                  <p className="text-xs text-gray-500 text-center">
                    No authentication users configured. Add a user to enable basic auth.
                  </p>
                )}
              </div>
            </Section>

            {/* Bearer Token Authentication Section */}
            <Section
              title="Bearer Token Authentication"
              icon={KeyRound}
              defaultOpen={false}
              badge={bearerTokens.length > 0 ? <Badge color="green" size="sm">{bearerTokens.length} token{bearerTokens.length > 1 ? "s" : ""}</Badge> : undefined}
            >
              <div className="space-y-3">
                <label className="flex items-center gap-2 p-3 border border-gray-200 dark:border-gray-700 rounded-lg cursor-pointer hover:bg-gray-50 dark:hover:bg-gray-800/50">
                  <input
                    type="checkbox"
                    checked={editProxy.bearer_auth_enabled}
                    onChange={(e) => { setEditProxy({ ...editProxy, bearer_auth_enabled: e.target.checked }); markChanged(); }}
                    className="w-4 h-4 rounded text-primary-600"
                  />
                  <div>
                    <span className="text-sm font-medium text-gray-900 dark:text-white">Enabled</span>
                    <p className="text-xs text-gray-500">Require an Authorization: Bearer &lt;token&gt; header. Save Changes below to apply.</p>
                  </div>
                </label>

                {justCreatedToken && (
                  <div className="p-3 border border-primary-300 dark:border-primary-700 bg-primary-50 dark:bg-primary-900/20 rounded-lg space-y-2">
                    <p className="text-xs font-medium text-primary-700 dark:text-primary-300">
                      Copy this token now -- it won&apos;t be shown again.
                    </p>
                    <div className="flex items-center gap-2">
                      <code className="flex-1 text-xs font-mono bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-600 rounded px-2 py-1.5 overflow-x-auto whitespace-nowrap">
                        {justCreatedToken}
                      </code>
                      <button
                        onClick={() => navigator.clipboard.writeText(justCreatedToken)}
                        className="p-1.5 text-gray-400 hover:text-primary-600 rounded shrink-0"
                        title="Copy"
                      >
                        <Copy className="h-3.5 w-3.5" />
                      </button>
                    </div>
                    <button
                      onClick={() => setJustCreatedToken("")}
                      className="text-xs text-gray-500 hover:text-gray-700"
                    >
                      Dismiss
                    </button>
                  </div>
                )}

                {/* Existing tokens */}
                {bearerTokens.map((token) => (
                  <div key={token.id} className="flex items-center justify-between p-3 border border-gray-200 dark:border-gray-700 rounded-lg">
                    <div className="flex items-center gap-2">
                      <KeyRound className="h-4 w-4 text-gray-400" />
                      <span className="text-sm font-medium text-gray-900 dark:text-white">{token.name}</span>
                    </div>
                    <button
                      onClick={() => deleteBearerTokenMutation.mutate(token.id)}
                      disabled={deleteBearerTokenMutation.isPending}
                      className="p-1.5 text-gray-400 hover:text-red-500 rounded"
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </button>
                  </div>
                ))}

                {/* Add token form */}
                <div className="border border-dashed border-gray-300 dark:border-gray-600 rounded-lg p-3">
                  <p className="text-xs font-medium text-gray-500 mb-2">Add Token</p>
                  <div className="space-y-2">
                    <input
                      type="text"
                      value={newTokenName}
                      onChange={(e) => setNewTokenName(e.target.value)}
                      placeholder="Name (e.g. CI pipeline)"
                      className="w-full px-2 py-1.5 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-600 rounded text-sm"
                    />
                    <button
                      onClick={() => {
                        if (newTokenName.trim()) {
                          createBearerTokenMutation.mutate({ name: newTokenName.trim() });
                        }
                      }}
                      disabled={!newTokenName.trim() || createBearerTokenMutation.isPending}
                      className="w-full px-2 py-1.5 text-xs bg-primary-600 text-white rounded hover:bg-primary-700 disabled:opacity-50"
                    >
                      {createBearerTokenMutation.isPending ? "Generating..." : "Generate Token"}
                    </button>
                  </div>
                </div>

                {bearerTokens.length === 0 && (
                  <p className="text-xs text-gray-500 text-center">
                    No tokens configured. Add one and enable Bearer Token Authentication above.
                  </p>
                )}
              </div>
            </Section>

            {/* Security Headers Section */}
            <Section title="Security Headers" icon={Shield} defaultOpen={false}>
              <div className="space-y-4">
                {/* HSTS */}
                <div className="flex items-center justify-between p-3 border border-gray-200 dark:border-gray-700 rounded-lg">
                  <div>
                    <p className="text-sm font-medium text-gray-900 dark:text-white">HSTS</p>
                    <p className="text-xs text-gray-500">Force HTTPS connections</p>
                  </div>
                  <label className="relative inline-flex items-center cursor-pointer">
                    <input
                      type="checkbox"
                      checked={securityHeaders.hsts_enabled}
                      onChange={(e) => { setSecurityHeaders({ ...securityHeaders, hsts_enabled: e.target.checked }); markChanged(); }}
                      className="sr-only peer"
                    />
                    <div className="w-10 h-5 bg-gray-300 dark:bg-gray-600 rounded-full peer peer-checked:bg-primary-600 peer-checked:after:translate-x-full after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:rounded-full after:h-4 after:w-4 after:transition-all"></div>
                  </label>
                </div>

                {securityHeaders.hsts_enabled && (
                  <div className="pl-4 border-l-2 border-primary-200 dark:border-primary-800">
                    <label className="block text-xs text-gray-500 mb-1">HSTS Max Age (seconds)</label>
                    <input
                      type="number"
                      value={securityHeaders.hsts_max_age}
                      onChange={(e) => { setSecurityHeaders({ ...securityHeaders, hsts_max_age: parseInt(e.target.value) || 31536000 }); markChanged(); }}
                      className="w-full px-3 py-2 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-600 rounded-lg text-sm"
                    />
                  </div>
                )}

                {/* X-Frame-Options */}
                <div>
                  <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">X-Frame-Options</label>
                  <select
                    value={securityHeaders.x_frame_options}
                    onChange={(e) => { setSecurityHeaders({ ...securityHeaders, x_frame_options: e.target.value }); markChanged(); }}
                    className="w-full px-3 py-2 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-600 rounded-lg text-sm"
                  >
                    <option value="">Disabled</option>
                    <option value="DENY">DENY</option>
                    <option value="SAMEORIGIN">SAMEORIGIN</option>
                  </select>
                </div>

                {/* X-Content-Type-Options */}
                <div className="flex items-center justify-between p-3 border border-gray-200 dark:border-gray-700 rounded-lg">
                  <div>
                    <p className="text-sm font-medium text-gray-900 dark:text-white">X-Content-Type-Options</p>
                    <p className="text-xs text-gray-500">Prevent MIME sniffing</p>
                  </div>
                  <label className="relative inline-flex items-center cursor-pointer">
                    <input
                      type="checkbox"
                      checked={securityHeaders.x_content_type_options}
                      onChange={(e) => { setSecurityHeaders({ ...securityHeaders, x_content_type_options: e.target.checked }); markChanged(); }}
                      className="sr-only peer"
                    />
                    <div className="w-10 h-5 bg-gray-300 dark:bg-gray-600 rounded-full peer peer-checked:bg-primary-600 peer-checked:after:translate-x-full after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:rounded-full after:h-4 after:w-4 after:transition-all"></div>
                  </label>
                </div>

                {/* X-XSS-Protection */}
                <div className="flex items-center justify-between p-3 border border-gray-200 dark:border-gray-700 rounded-lg">
                  <div>
                    <p className="text-sm font-medium text-gray-900 dark:text-white">X-XSS-Protection</p>
                    <p className="text-xs text-gray-500">XSS filter (legacy)</p>
                  </div>
                  <label className="relative inline-flex items-center cursor-pointer">
                    <input
                      type="checkbox"
                      checked={securityHeaders.x_xss_protection}
                      onChange={(e) => { setSecurityHeaders({ ...securityHeaders, x_xss_protection: e.target.checked }); markChanged(); }}
                      className="sr-only peer"
                    />
                    <div className="w-10 h-5 bg-gray-300 dark:bg-gray-600 rounded-full peer peer-checked:bg-primary-600 peer-checked:after:translate-x-full after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:rounded-full after:h-4 after:w-4 after:transition-all"></div>
                  </label>
                </div>

                {/* CSP */}
                <div>
                  <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Content-Security-Policy</label>
                  <textarea
                    value={securityHeaders.content_security_policy || ""}
                    onChange={(e) => { setSecurityHeaders({ ...securityHeaders, content_security_policy: e.target.value || null }); markChanged(); }}
                    placeholder="e.g., default-src 'self'"
                    rows={2}
                    className="w-full px-3 py-2 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-600 rounded-lg text-sm font-mono"
                  />
                </div>
              </div>
            </Section>

            {/* Rate Limits Section */}
            <Section
              title="Rate Limits"
              icon={Activity}
              defaultOpen={false}
              badge={rateLimits.length > 0 ? <Badge size="sm">{rateLimits.length}</Badge> : undefined}
            >
              <div className="space-y-3">
                {/* Existing limits */}
                {rateLimits.map((rl) => (
                  <div
                    key={rl.id}
                    className={cn(
                      "flex items-center justify-between p-3 rounded-lg border",
                      editingRateLimit?.id === rl.id
                        ? "border-primary-500 bg-primary-50 dark:bg-primary-900/20"
                        : "border-gray-200 dark:border-gray-700"
                    )}
                  >
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="font-medium text-gray-900 dark:text-white text-sm">{rl.zone_name}</span>
                        {!rl.enabled && <Badge size="sm">Disabled</Badge>}
                      </div>
                      <p className="text-xs text-gray-500">
                        {rl.requests_per} req / {rl.time_window} (burst: {rl.burst})
                      </p>
                    </div>
                    <div className="flex items-center gap-1">
                      <button
                        onClick={() => {
                          setEditingRateLimit(rl);
                          setNewRateLimit({
                            zone_name: rl.zone_name,
                            requests_per: rl.requests_per,
                            time_window: rl.time_window,
                            burst: rl.burst,
                            enabled: rl.enabled,
                          });
                        }}
                        className="p-1.5 text-gray-400 hover:text-gray-900 dark:hover:text-white rounded"
                      >
                        <Pencil className="h-3.5 w-3.5" />
                      </button>
                      <button
                        onClick={() => deleteRateLimitMutation.mutate(rl.id)}
                        className="p-1.5 text-gray-400 hover:text-red-500 rounded"
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </button>
                    </div>
                  </div>
                ))}

                {/* Add/Edit rate limit form */}
                <div className="border border-dashed border-gray-300 dark:border-gray-600 rounded-lg p-3">
                  <p className="text-xs font-medium text-gray-500 mb-2">
                    {editingRateLimit ? "Edit Rate Limit" : "Add Rate Limit"}
                  </p>
                  <div className="grid grid-cols-2 gap-2">
                    <input
                      type="text"
                      value={newRateLimit.zone_name}
                      onChange={(e) => setNewRateLimit({ ...newRateLimit, zone_name: e.target.value })}
                      placeholder="Zone name"
                      className="px-2 py-1.5 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-600 rounded text-xs"
                    />
                    <select
                      value={newRateLimit.time_window}
                      onChange={(e) => setNewRateLimit({ ...newRateLimit, time_window: e.target.value })}
                      className="px-2 py-1.5 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-600 rounded text-xs"
                    >
                      <option value="1s">1 second</option>
                      <option value="10s">10 seconds</option>
                      <option value="1m">1 minute</option>
                      <option value="5m">5 minutes</option>
                      <option value="1h">1 hour</option>
                    </select>
                    <input
                      type="number"
                      value={newRateLimit.requests_per}
                      onChange={(e) => setNewRateLimit({ ...newRateLimit, requests_per: parseInt(e.target.value) || 1 })}
                      placeholder="Requests"
                      className="px-2 py-1.5 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-600 rounded text-xs"
                    />
                    <input
                      type="number"
                      value={newRateLimit.burst}
                      onChange={(e) => setNewRateLimit({ ...newRateLimit, burst: parseInt(e.target.value) || 0 })}
                      placeholder="Burst"
                      className="px-2 py-1.5 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-600 rounded text-xs"
                    />
                  </div>
                  <div className="flex items-center justify-between mt-2">
                    <label className="flex items-center gap-1.5 text-xs text-gray-600 dark:text-gray-400">
                      <input
                        type="checkbox"
                        checked={newRateLimit.enabled}
                        onChange={(e) => setNewRateLimit({ ...newRateLimit, enabled: e.target.checked })}
                        className="w-3 h-3 rounded"
                      />
                      Enabled
                    </label>
                    <div className="flex gap-1">
                      {editingRateLimit && (
                        <button
                          onClick={() => {
                            setEditingRateLimit(null);
                            setNewRateLimit({ zone_name: "default", requests_per: 100, time_window: "1m", burst: 50, enabled: true });
                          }}
                          className="px-2 py-1 text-xs text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white"
                        >
                          Cancel
                        </button>
                      )}
                      <button
                        onClick={() => {
                          if (editingRateLimit) {
                            updateRateLimitMutation.mutate({ id: editingRateLimit.id, data: newRateLimit });
                          } else {
                            createRateLimitMutation.mutate(newRateLimit);
                          }
                        }}
                        disabled={createRateLimitMutation.isPending || updateRateLimitMutation.isPending}
                        className="px-2 py-1 text-xs bg-primary-600 text-white rounded hover:bg-primary-700 disabled:opacity-50"
                      >
                        {editingRateLimit ? "Update" : "Add"}
                      </button>
                    </div>
                  </div>
                </div>
              </div>
            </Section>

            {/* Nginx Configuration Preview - Using shared component */}
            <ConfigPreviewSection
              config={configData?.config}
              isLoading={configLoading}
              onRefresh={() => refetchConfig()}
            />

            {/* Action Buttons - Fixed at bottom */}
            <div className="sticky bottom-0 -mx-6 px-6 py-4 bg-white dark:bg-gray-900 border-t border-gray-200 dark:border-gray-700 flex items-center justify-between gap-3">
              <Button
                variant="danger"
                size="sm"
                icon={Trash2}
                onClick={() => setShowDeleteConfirm(true)}
              >
                Delete Proxy
              </Button>

              <div className="flex items-center gap-2">
                <Button
                  variant="secondary"
                  size="sm"
                  icon={testConfigMutation.isPending ? Loader2 : Play}
                  onClick={() => testConfigMutation.mutate()}
                  disabled={testConfigMutation.isPending}
                >
                  {testConfigMutation.isPending ? "Testing..." : "Test Configuration"}
                </Button>

                <Button
                  variant="primary"
                  size="sm"
                  icon={updateMutation.isPending || securityHeadersMutation.isPending ? Loader2 : Save}
                  onClick={handleSaveAll}
                  disabled={!hasChanges || updateMutation.isPending || securityHeadersMutation.isPending}
                >
                  {updateMutation.isPending ? "Saving..." : "Save Changes"}
                </Button>
              </div>
            </div>

            {/* Test config result */}
            {(testConfigMutation.isSuccess || testConfigMutation.isError) && (
              <div
                className={cn(
                  "p-3 rounded-lg text-sm",
                  testConfigMutation.data?.success
                    ? "bg-green-50 dark:bg-green-900/20 border border-green-200 dark:border-green-800 text-green-700 dark:text-green-400"
                    : "bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 text-red-700 dark:text-red-400"
                )}
              >
                <div className="flex items-center gap-2">
                  {testConfigMutation.data?.success ? <Check className="h-4 w-4" /> : <AlertTriangle className="h-4 w-4" />}
                  <span>{testConfigMutation.data?.message || "Test failed"}</span>
                </div>
              </div>
            )}
          </div>
        );
    }
  };

  // Prevent closing SlideOver when SSLWizard is open
  const handleSlideOverClose = () => {
    if (!showSSLWizard) {
      closeProxyPanel();
    }
  };

  return (
    <>
      <SlideOver isOpen={!!proxyPanelId} onClose={handleSlideOverClose} size="lg">
        {proxyLoading ? (
          <div className="flex items-center justify-center h-64">
            <Spinner.Logo size="lg" />
          </div>
        ) : proxy ? (
          <>
            <SlideOver.Header onClose={handleSlideOverClose}>
              <div>
                <div className="flex items-center gap-2">
                  <h2 className="text-lg font-semibold text-gray-900 dark:text-white">{proxy.domain}</h2>
                  {proxy.is_system_proxy && <Badge color="blue" size="sm">InfraPilot</Badge>}
                  {proxy.proxy_type === "redirect" && <Badge color="purple" size="sm">Redirect</Badge>}
                  <StatusIndicator status={getSSLStatus(proxy)} showLabel={false} size="sm" />
                </div>
                <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
                  {proxy.proxy_type === "redirect" ? proxy.redirect_url : proxy.upstream_target}
                </p>
              </div>
            </SlideOver.Header>
            <SlideOver.Body>
              {/* Tabs */}
              <div className="flex gap-1 mb-6 border-b border-gray-200 dark:border-gray-700 overflow-x-auto">
                {panelTabs.map((t) => {
                  const Icon = t.icon;
                  return (
                    <button
                      key={t.id}
                      onClick={() => setTab(t.id)}
                      className={cn(
                        "flex items-center gap-2 px-4 py-2 text-sm font-medium border-b-2 transition-colors whitespace-nowrap",
                        tab === t.id
                          ? "border-primary-600 text-primary-600 dark:text-primary-400"
                          : "border-transparent text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-300"
                      )}
                    >
                      <Icon className="h-4 w-4" />
                      {t.label}
                    </button>
                  );
                })}
              </div>

              {/* Tab content */}
              {renderTabContent()}
            </SlideOver.Body>
          </>
        ) : (
          <div className="flex items-center justify-center h-64 text-gray-500">
            <p className="text-sm">Proxy not found</p>
          </div>
        )}
      </SlideOver>

      {/* Delete Confirmation */}
      <ConfirmDialog
        isOpen={showDeleteConfirm && !!proxy}
        onClose={() => setShowDeleteConfirm(false)}
        onConfirm={() => {
          if (proxy) {
            deleteMutation.mutate(proxy.id);
          }
        }}
        title="Delete Proxy"
        message={`Are you sure you want to delete the proxy for "${proxy?.domain}"?\n\nThis will remove the nginx configuration and SSL certificates.`}
        confirmText="Delete Proxy"
        variant="danger"
        icon="delete"
        isLoading={deleteMutation.isPending}
      />

      {/* SSL Wizard */}
      {proxy && (
        <SSLWizard
          domain={proxy.domain}
          open={showSSLWizard}
          onOpenChange={setShowSSLWizard}
          agentId={selectedAgent || undefined}
          proxyId={proxy.id}
          includeWWW={proxy.include_www}
          onSuccess={() => {
            invalidateProxies();
          }}
        />
      )}
    </>
  );
}
