"use client";

import { useState, useEffect } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Globe,
  Plus,
  Trash2,
  Shield,
  ShieldCheck,
  ShieldAlert,
  ExternalLink,
  AlertTriangle,
  Container as ContainerIcon,
  Network,
  Check,
  ChevronRight,
  Lock,
  Gauge,
  Code,
  Pencil,
  X,
  RefreshCw,
} from "lucide-react";
import { api, Container, SecurityHeaders, RateLimit } from "@/lib/api";
import { formatRelativeTime, cn } from "@/lib/utils";
import {
  PageLayout,
  ListCard,
  EmptyState,
  Button,
  Tabs,
  Input,
} from "@/components/ui/page-layout";
import {
  DetailPanel,
  DetailSection,
  DetailRow,
} from "@/components/ui/detail-panel";

interface ProxyHost {
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

type PanelTab = "details" | "security" | "ratelimits" | "config";
type UpstreamMode = "manual" | "container";

export default function ProxiesPage() {
  const queryClient = useQueryClient();
  const [selectedAgent, setSelectedAgent] = useState<string | null>(null);
  const [selectedProxy, setSelectedProxy] = useState<ProxyHost | null>(null);
  const [panelTab, setPanelTab] = useState<PanelTab>("details");
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [isEditing, setIsEditing] = useState(false);

  // Form state for create
  const [upstreamMode, setUpstreamMode] = useState<UpstreamMode>("manual");
  const [selectedContainer, setSelectedContainer] = useState<string | null>(null);
  const [containerPort, setContainerPort] = useState<string>("80");
  const [newProxy, setNewProxy] = useState({
    domain: "",
    upstream_target: "",
    force_ssl: true,
    http2_enabled: true,
  });

  // Edit form state
  const [editProxy, setEditProxy] = useState({
    domain: "",
    upstream_target: "",
    force_ssl: true,
    http2_enabled: true,
  });

  // Network warning state
  const [showNetworkWarning, setShowNetworkWarning] = useState(false);
  const [networkToAttach, setNetworkToAttach] = useState<{ id: string; name: string } | null>(null);
  const [pendingProxySubmit, setPendingProxySubmit] = useState(false);

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

  // Config state
  const [configContent, setConfigContent] = useState<string>("");
  const [configLoading, setConfigLoading] = useState(false);

  // Fetch agents
  const { data: agents } = useQuery({
    queryKey: ["agents"],
    queryFn: () => api.getAgents(),
  });

  // Fetch proxies for selected agent
  const { data: proxies, isLoading: proxiesLoading } = useQuery({
    queryKey: ["proxies", selectedAgent],
    queryFn: () =>
      selectedAgent ? api.getProxyHosts(selectedAgent) : Promise.resolve([]),
    enabled: !!selectedAgent,
  });

  // Fetch containers for create modal
  const { data: containers } = useQuery({
    queryKey: ["containers", selectedAgent],
    queryFn: () =>
      selectedAgent ? api.getContainers(selectedAgent) : Promise.resolve([]),
    enabled: !!selectedAgent && showCreateModal,
  });

  // Fetch networks for selected container
  const { data: containerNetworks } = useQuery({
    queryKey: ["containerNetworks", selectedAgent, selectedContainer],
    queryFn: () =>
      selectedAgent && selectedContainer
        ? api.getContainerNetworks(selectedAgent, selectedContainer)
        : Promise.resolve([]),
    enabled: !!selectedAgent && !!selectedContainer,
  });

  // Check nginx network connection
  const { data: nginxNetworkCheck } = useQuery({
    queryKey: ["nginxNetworkCheck", selectedAgent, containerNetworks?.[0]?.network_id],
    queryFn: () =>
      selectedAgent && containerNetworks?.[0]?.network_id
        ? api.checkNginxNetwork(selectedAgent, containerNetworks[0].network_id)
        : Promise.resolve({ connected: true, network_id: "" }),
    enabled: !!selectedAgent && !!containerNetworks?.[0]?.network_id,
  });

  const activeAgents = agents?.filter((a) => a.status === "active") || [];

  // Auto-select first active agent
  useEffect(() => {
    if (!selectedAgent && activeAgents.length > 0) {
      setSelectedAgent(activeAgents[0].id);
    }
  }, [activeAgents, selectedAgent]);

  // Load proxy details when selected
  useEffect(() => {
    if (selectedProxy && selectedAgent) {
      setEditProxy({
        domain: selectedProxy.domain,
        upstream_target: selectedProxy.upstream_target,
        force_ssl: selectedProxy.force_ssl,
        http2_enabled: selectedProxy.http2_enabled,
      });

      // Load security headers
      api.getSecurityHeaders(selectedAgent, selectedProxy.id)
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
      api.getRateLimits(selectedAgent, selectedProxy.id)
        .then(setRateLimits)
        .catch(() => setRateLimits([]));
    }
  }, [selectedProxy, selectedAgent]);

  // Load config when tab changes
  useEffect(() => {
    if (panelTab === "config" && selectedProxy && selectedAgent) {
      setConfigLoading(true);
      fetch(`/api/v1/agents/${selectedAgent}/proxies/${selectedProxy.id}/config`, {
        headers: { Authorization: `Bearer ${localStorage.getItem("access_token")}` },
      })
        .then((res) => res.json())
        .then((data) => setConfigContent(data.config || "No config generated"))
        .catch(() => setConfigContent("Failed to load config"))
        .finally(() => setConfigLoading(false));
    }
  }, [panelTab, selectedProxy, selectedAgent]);

  // Update selected proxy from fresh data
  useEffect(() => {
    if (selectedProxy && proxies) {
      const updated = proxies.find((p: ProxyHost) => p.id === selectedProxy.id);
      if (updated) {
        setSelectedProxy(updated);
      }
    }
  }, [proxies, selectedProxy]);

  // Mutations
  const attachNetworkMutation = useMutation({
    mutationFn: (networkId: string) => api.attachNginxNetwork(selectedAgent!, networkId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["nginxNetworkCheck"] });
      setShowNetworkWarning(false);
      setNetworkToAttach(null);
      if (pendingProxySubmit) {
        createMutation.mutate(newProxy);
        setPendingProxySubmit(false);
      }
    },
  });

  const createMutation = useMutation({
    mutationFn: (data: typeof newProxy) => api.createProxyHost(selectedAgent!, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["proxies", selectedAgent] });
      setShowCreateModal(false);
      resetForm();
    },
  });

  const updateMutation = useMutation({
    mutationFn: ({ proxyId, data }: { proxyId: string; data: typeof editProxy }) =>
      api.updateProxyHost(selectedAgent!, proxyId, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["proxies", selectedAgent] });
      setIsEditing(false);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (proxyId: string) => api.deleteProxyHost(selectedAgent!, proxyId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["proxies", selectedAgent] });
      setSelectedProxy(null);
    },
  });

  const sslMutation = useMutation({
    mutationFn: (proxyId: string) => api.requestSSL(selectedAgent!, proxyId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["proxies", selectedAgent] });
    },
  });

  const securityHeadersMutation = useMutation({
    mutationFn: ({ proxyId, data }: { proxyId: string; data: Omit<SecurityHeaders, "id" | "proxy_host_id"> }) =>
      api.updateSecurityHeaders(selectedAgent!, proxyId, data),
  });

  const createRateLimitMutation = useMutation({
    mutationFn: (data: typeof newRateLimit) =>
      api.createRateLimit(selectedAgent!, selectedProxy!.id, data),
    onSuccess: (createdLimit) => {
      setRateLimits([createdLimit, ...rateLimits]);
      setNewRateLimit({ zone_name: "default", requests_per: 100, time_window: "1m", burst: 50, enabled: true });
    },
  });

  const updateRateLimitMutation = useMutation({
    mutationFn: ({ id, data }: { id: string; data: typeof newRateLimit }) =>
      api.updateRateLimit(selectedAgent!, selectedProxy!.id, id, data),
    onSuccess: (updatedLimit) => {
      setRateLimits(rateLimits.map((rl) => (rl.id === updatedLimit.id ? updatedLimit : rl)));
      setEditingRateLimit(null);
    },
  });

  const deleteRateLimitMutation = useMutation({
    mutationFn: (id: string) => api.deleteRateLimit(selectedAgent!, selectedProxy!.id, id),
    onSuccess: (_, deletedId) => {
      setRateLimits(rateLimits.filter((rl) => rl.id !== deletedId));
    },
  });

  // Helper functions
  const resetForm = () => {
    setNewProxy({ domain: "", upstream_target: "", force_ssl: true, http2_enabled: true });
    setUpstreamMode("manual");
    setSelectedContainer(null);
    setContainerPort("80");
    setPendingProxySubmit(false);
  };

  const buildContainerUpstream = (container: Container | undefined) => {
    if (!container) return "";
    const containerName = container.name.startsWith("/") ? container.name.slice(1) : container.name;
    return `http://${containerName}:${containerPort}`;
  };

  const handleProxySubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (upstreamMode === "container" && selectedContainer) {
      const container = containers?.find((c) => c.container_id === selectedContainer);
      const upstream = buildContainerUpstream(container);
      const proxyData = { ...newProxy, upstream_target: upstream };
      setNewProxy(proxyData);

      if (containerNetworks?.[0] && !nginxNetworkCheck?.connected) {
        setNetworkToAttach({ id: containerNetworks[0].network_id, name: containerNetworks[0].network_name });
        setShowNetworkWarning(true);
        setPendingProxySubmit(true);
        return;
      }
      createMutation.mutate(proxyData);
    } else {
      createMutation.mutate(newProxy);
    }
  };

  const getSSLIcon = (proxy: ProxyHost) => {
    if (!proxy.ssl_enabled) return <ShieldAlert className="h-4 w-4 text-yellow-500" />;
    if (proxy.ssl_expires_at) {
      const daysUntilExpiry = Math.ceil((new Date(proxy.ssl_expires_at).getTime() - Date.now()) / (1000 * 60 * 60 * 24));
      if (daysUntilExpiry < 7) return <ShieldAlert className="h-4 w-4 text-red-500" />;
    }
    return <ShieldCheck className="h-4 w-4 text-green-500" />;
  };

  const getStatusBadgeClass = (status: string) => {
    switch (status) {
      case "active":
        return "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400";
      case "ssl_pending":
        return "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400";
      case "error":
        return "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400";
      default:
        return "bg-gray-100 text-gray-700 dark:bg-gray-800 dark:text-gray-400";
    }
  };

  // Render panel content
  const renderPanelContent = () => {
    if (!selectedProxy) return null;

    switch (panelTab) {
      case "security":
        return (
          <form
            onSubmit={(e) => {
              e.preventDefault();
              securityHeadersMutation.mutate({ proxyId: selectedProxy.id, data: securityHeaders });
            }}
            className="space-y-4"
          >
            {/* HSTS */}
            <div className="border border-gray-200 dark:border-gray-700 rounded-lg p-4">
              <div className="flex items-center justify-between">
                <div>
                  <h3 className="text-sm font-medium text-gray-900 dark:text-white">HSTS</h3>
                  <p className="text-xs text-gray-500 mt-0.5">Force HTTPS connections</p>
                </div>
                <label className="relative inline-flex items-center cursor-pointer">
                  <input
                    type="checkbox"
                    checked={securityHeaders.hsts_enabled}
                    onChange={(e) => setSecurityHeaders({ ...securityHeaders, hsts_enabled: e.target.checked })}
                    className="sr-only peer"
                  />
                  <div className="w-10 h-5 bg-gray-300 dark:bg-gray-600 rounded-full peer peer-checked:bg-primary-600 peer-checked:after:translate-x-full after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:rounded-full after:h-4 after:w-4 after:transition-all"></div>
                </label>
              </div>
              {securityHeaders.hsts_enabled && (
                <div className="mt-3">
                  <label className="block text-xs text-gray-500 mb-1">Max Age (seconds)</label>
                  <input
                    type="number"
                    value={securityHeaders.hsts_max_age}
                    onChange={(e) => setSecurityHeaders({ ...securityHeaders, hsts_max_age: parseInt(e.target.value) || 31536000 })}
                    className="w-full px-3 py-2 bg-gray-100 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg text-sm"
                  />
                </div>
              )}
            </div>

            {/* X-Frame-Options */}
            <div className="border border-gray-200 dark:border-gray-700 rounded-lg p-4">
              <h3 className="text-sm font-medium text-gray-900 dark:text-white mb-2">X-Frame-Options</h3>
              <select
                value={securityHeaders.x_frame_options}
                onChange={(e) => setSecurityHeaders({ ...securityHeaders, x_frame_options: e.target.value })}
                className="w-full px-3 py-2 bg-gray-100 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg text-sm"
              >
                <option value="">Disabled</option>
                <option value="DENY">DENY</option>
                <option value="SAMEORIGIN">SAMEORIGIN</option>
              </select>
            </div>

            {/* Toggle options */}
            <div className="space-y-3">
              <div className="flex items-center justify-between p-3 border border-gray-200 dark:border-gray-700 rounded-lg">
                <div>
                  <p className="text-sm font-medium text-gray-900 dark:text-white">X-Content-Type-Options</p>
                  <p className="text-xs text-gray-500">Prevent MIME sniffing</p>
                </div>
                <label className="relative inline-flex items-center cursor-pointer">
                  <input
                    type="checkbox"
                    checked={securityHeaders.x_content_type_options}
                    onChange={(e) => setSecurityHeaders({ ...securityHeaders, x_content_type_options: e.target.checked })}
                    className="sr-only peer"
                  />
                  <div className="w-10 h-5 bg-gray-300 dark:bg-gray-600 rounded-full peer peer-checked:bg-primary-600 peer-checked:after:translate-x-full after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:rounded-full after:h-4 after:w-4 after:transition-all"></div>
                </label>
              </div>

              <div className="flex items-center justify-between p-3 border border-gray-200 dark:border-gray-700 rounded-lg">
                <div>
                  <p className="text-sm font-medium text-gray-900 dark:text-white">X-XSS-Protection</p>
                  <p className="text-xs text-gray-500">XSS filter (legacy)</p>
                </div>
                <label className="relative inline-flex items-center cursor-pointer">
                  <input
                    type="checkbox"
                    checked={securityHeaders.x_xss_protection}
                    onChange={(e) => setSecurityHeaders({ ...securityHeaders, x_xss_protection: e.target.checked })}
                    className="sr-only peer"
                  />
                  <div className="w-10 h-5 bg-gray-300 dark:bg-gray-600 rounded-full peer peer-checked:bg-primary-600 peer-checked:after:translate-x-full after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:rounded-full after:h-4 after:w-4 after:transition-all"></div>
                </label>
              </div>
            </div>

            {/* CSP */}
            <div className="border border-gray-200 dark:border-gray-700 rounded-lg p-4">
              <h3 className="text-sm font-medium text-gray-900 dark:text-white mb-2">Content-Security-Policy</h3>
              <textarea
                value={securityHeaders.content_security_policy || ""}
                onChange={(e) => setSecurityHeaders({ ...securityHeaders, content_security_policy: e.target.value || null })}
                placeholder="e.g., default-src 'self'"
                rows={2}
                className="w-full px-3 py-2 bg-gray-100 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg text-sm font-mono"
              />
            </div>

            <Button
              type="submit"
              variant="primary"
              disabled={securityHeadersMutation.isPending}
              className="w-full"
            >
              {securityHeadersMutation.isPending ? "Saving..." : "Save Security Headers"}
            </Button>
          </form>
        );

      case "ratelimits":
        return (
          <div className="space-y-4">
            {/* Existing limits */}
            {rateLimits.length > 0 && (
              <div className="space-y-2">
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
                        {!rl.enabled && (
                          <span className="px-1.5 py-0.5 text-xs bg-gray-200 dark:bg-gray-700 text-gray-500 rounded">Disabled</span>
                        )}
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
              </div>
            )}

            {/* Add/Edit form */}
            <form
              onSubmit={(e) => {
                e.preventDefault();
                if (editingRateLimit) {
                  updateRateLimitMutation.mutate({ id: editingRateLimit.id, data: newRateLimit });
                } else {
                  createRateLimitMutation.mutate(newRateLimit);
                }
              }}
              className="border border-gray-200 dark:border-gray-700 rounded-lg p-4 space-y-3"
            >
              <h4 className="text-sm font-medium text-gray-900 dark:text-white">
                {editingRateLimit ? "Edit Rate Limit" : "Add Rate Limit"}
              </h4>

              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs text-gray-500 mb-1">Zone Name</label>
                  <input
                    type="text"
                    value={newRateLimit.zone_name}
                    onChange={(e) => setNewRateLimit({ ...newRateLimit, zone_name: e.target.value })}
                    className="w-full px-3 py-2 bg-gray-100 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg text-sm"
                  />
                </div>
                <div>
                  <label className="block text-xs text-gray-500 mb-1">Time Window</label>
                  <select
                    value={newRateLimit.time_window}
                    onChange={(e) => setNewRateLimit({ ...newRateLimit, time_window: e.target.value })}
                    className="w-full px-3 py-2 bg-gray-100 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg text-sm"
                  >
                    <option value="1s">1 second</option>
                    <option value="10s">10 seconds</option>
                    <option value="1m">1 minute</option>
                    <option value="5m">5 minutes</option>
                    <option value="1h">1 hour</option>
                  </select>
                </div>
                <div>
                  <label className="block text-xs text-gray-500 mb-1">Requests/Window</label>
                  <input
                    type="number"
                    value={newRateLimit.requests_per}
                    onChange={(e) => setNewRateLimit({ ...newRateLimit, requests_per: parseInt(e.target.value) || 1 })}
                    className="w-full px-3 py-2 bg-gray-100 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg text-sm"
                  />
                </div>
                <div>
                  <label className="block text-xs text-gray-500 mb-1">Burst</label>
                  <input
                    type="number"
                    value={newRateLimit.burst}
                    onChange={(e) => setNewRateLimit({ ...newRateLimit, burst: parseInt(e.target.value) || 0 })}
                    className="w-full px-3 py-2 bg-gray-100 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg text-sm"
                  />
                </div>
              </div>

              <div className="flex items-center gap-2">
                <input
                  type="checkbox"
                  id="rate-limit-enabled"
                  checked={newRateLimit.enabled}
                  onChange={(e) => setNewRateLimit({ ...newRateLimit, enabled: e.target.checked })}
                  className="w-4 h-4 rounded"
                />
                <label htmlFor="rate-limit-enabled" className="text-sm text-gray-700 dark:text-gray-300">
                  Enable this limit
                </label>
              </div>

              <div className="flex gap-2">
                {editingRateLimit && (
                  <Button
                    type="button"
                    variant="secondary"
                    size="sm"
                    onClick={() => {
                      setEditingRateLimit(null);
                      setNewRateLimit({ zone_name: "default", requests_per: 100, time_window: "1m", burst: 50, enabled: true });
                    }}
                  >
                    Cancel
                  </Button>
                )}
                <Button
                  type="submit"
                  variant="primary"
                  size="sm"
                  disabled={createRateLimitMutation.isPending || updateRateLimitMutation.isPending}
                >
                  {editingRateLimit ? "Update" : "Add"}
                </Button>
              </div>
            </form>
          </div>
        );

      case "config":
        return (
          <div className="h-full">
            {configLoading ? (
              <div className="flex items-center justify-center h-32">
                <div className="animate-spin rounded-full h-6 w-6 border-b-2 border-primary-500" />
              </div>
            ) : (
              <div className="bg-gray-900 rounded-lg p-4 overflow-auto max-h-[500px]">
                <pre className="text-xs text-gray-300 font-mono whitespace-pre-wrap">
                  {configContent}
                </pre>
              </div>
            )}
          </div>
        );

      default:
        return (
          <>
            {isEditing ? (
              <form
                onSubmit={(e) => {
                  e.preventDefault();
                  updateMutation.mutate({ proxyId: selectedProxy.id, data: editProxy });
                }}
                className="space-y-4"
              >
                <Input
                  label="Domain"
                  value={editProxy.domain}
                  onChange={(e) => setEditProxy({ ...editProxy, domain: e.target.value })}
                  required
                />
                <Input
                  label="Upstream Target"
                  value={editProxy.upstream_target}
                  onChange={(e) => setEditProxy({ ...editProxy, upstream_target: e.target.value })}
                  required
                />
                <div className="flex items-center gap-4">
                  <label className="flex items-center gap-2 cursor-pointer">
                    <input
                      type="checkbox"
                      checked={editProxy.force_ssl}
                      onChange={(e) => setEditProxy({ ...editProxy, force_ssl: e.target.checked })}
                      className="w-4 h-4 rounded"
                    />
                    <span className="text-sm">Force SSL</span>
                  </label>
                  <label className="flex items-center gap-2 cursor-pointer">
                    <input
                      type="checkbox"
                      checked={editProxy.http2_enabled}
                      onChange={(e) => setEditProxy({ ...editProxy, http2_enabled: e.target.checked })}
                      className="w-4 h-4 rounded"
                    />
                    <span className="text-sm">HTTP/2</span>
                  </label>
                </div>
                <div className="flex gap-2">
                  <Button type="button" variant="secondary" onClick={() => setIsEditing(false)}>
                    Cancel
                  </Button>
                  <Button type="submit" variant="primary" disabled={updateMutation.isPending}>
                    {updateMutation.isPending ? "Saving..." : "Save"}
                  </Button>
                </div>
              </form>
            ) : (
              <>
                <DetailSection title="Actions">
                  <div className="flex flex-wrap gap-2">
                    <Button variant="secondary" size="sm" icon={Pencil} onClick={() => setIsEditing(true)}>
                      Edit
                    </Button>
                    {!selectedProxy.ssl_enabled && selectedProxy.status !== "ssl_pending" && (
                      <Button
                        variant="secondary"
                        size="sm"
                        icon={ShieldCheck}
                        onClick={() => sslMutation.mutate(selectedProxy.id)}
                        disabled={sslMutation.isPending}
                      >
                        Request SSL
                      </Button>
                    )}
                    <Button
                      variant="danger"
                      size="sm"
                      icon={Trash2}
                      onClick={() => {
                        if (confirm("Are you sure you want to delete this proxy?")) {
                          deleteMutation.mutate(selectedProxy.id);
                        }
                      }}
                    >
                      Delete
                    </Button>
                  </div>
                </DetailSection>

                <DetailSection title="Proxy Info">
                  <DetailRow label="Domain">
                    <a
                      href={`http${selectedProxy.ssl_enabled ? "s" : ""}://${selectedProxy.domain}`}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="text-primary-600 hover:text-primary-500 flex items-center gap-1"
                    >
                      {selectedProxy.domain}
                      <ExternalLink className="h-3 w-3" />
                    </a>
                  </DetailRow>
                  <DetailRow label="Status">
                    <span className={cn("px-2 py-0.5 text-xs font-medium rounded-full", getStatusBadgeClass(selectedProxy.status))}>
                      {selectedProxy.status}
                    </span>
                  </DetailRow>
                  <DetailRow label="Created" value={formatRelativeTime(selectedProxy.created_at)} />
                </DetailSection>

                <DetailSection title="Upstream">
                  <div className="bg-gray-100 dark:bg-gray-800/50 rounded-lg p-3">
                    <code className="text-sm text-gray-900 dark:text-white font-mono break-all">
                      {selectedProxy.upstream_target}
                    </code>
                  </div>
                </DetailSection>

                <DetailSection title="SSL / TLS">
                  <div className="flex items-center gap-3 mb-3">
                    {getSSLIcon(selectedProxy)}
                    <span className="text-sm text-gray-700 dark:text-gray-300">
                      {selectedProxy.ssl_enabled
                        ? selectedProxy.ssl_expires_at
                          ? `Expires ${formatRelativeTime(selectedProxy.ssl_expires_at)}`
                          : "SSL Enabled"
                        : selectedProxy.status === "ssl_pending"
                        ? "SSL Certificate Pending"
                        : "SSL Not Enabled"}
                    </span>
                  </div>
                  <div className="flex flex-wrap gap-2">
                    {selectedProxy.force_ssl && (
                      <span className="px-2 py-1 text-xs bg-blue-100 dark:bg-blue-900/30 text-blue-700 dark:text-blue-400 rounded-lg">
                        Force SSL
                      </span>
                    )}
                    {selectedProxy.http2_enabled && (
                      <span className="px-2 py-1 text-xs bg-purple-100 dark:bg-purple-900/30 text-purple-700 dark:text-purple-400 rounded-lg">
                        HTTP/2
                      </span>
                    )}
                  </div>
                </DetailSection>
              </>
            )}
          </>
        );
    }
  };

  const panelTabs = [
    { id: "details", label: "Details" },
    { id: "security", label: "Security" },
    { id: "ratelimits", label: "Rate Limits" },
    { id: "config", label: "Config" },
  ] as { id: PanelTab; label: string }[];

  return (
    <PageLayout
      title="Proxy Hosts"
      description="Manage nginx reverse proxy configurations"
      actions={
        <Button
          variant="primary"
          icon={Plus}
          onClick={() => setShowCreateModal(true)}
          disabled={!selectedAgent}
        >
          Add Proxy Host
        </Button>
      }
      panelOpen={!!selectedProxy}
      panel={
        <DetailPanel
          open={!!selectedProxy}
          onClose={() => {
            setSelectedProxy(null);
            setIsEditing(false);
          }}
          title={selectedProxy?.domain}
          subtitle={selectedProxy?.upstream_target}
          defaultWidth={520}
        >
          {selectedProxy && (
            <div className="space-y-4">
              <Tabs tabs={panelTabs} activeTab={panelTab} onChange={(id) => setPanelTab(id as PanelTab)} />
              <div className="mt-4">{renderPanelContent()}</div>
            </div>
          )}
        </DetailPanel>
      }
    >
      {/* Agent selector */}
      <div className="mb-6">
        <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
          Select Agent
        </label>
        <select
          value={selectedAgent || ""}
          onChange={(e) => {
            setSelectedAgent(e.target.value || null);
            setSelectedProxy(null);
          }}
          className="w-full max-w-xs px-4 py-2 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg text-gray-900 dark:text-white focus:ring-2 focus:ring-primary-500 focus:border-transparent"
        >
          <option value="">Select an agent...</option>
          {agents?.map((agent) => (
            <option key={agent.id} value={agent.id} disabled={agent.status !== "active"}>
              {agent.name} ({agent.status})
            </option>
          ))}
        </select>
      </div>

      {/* Proxies list */}
      {selectedAgent ? (
        proxiesLoading ? (
          <div className="flex items-center justify-center h-32">
            <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-500" />
          </div>
        ) : proxies && proxies.length > 0 ? (
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
            {proxies.map((proxy: ProxyHost) => {
              const isSelected = selectedProxy?.id === proxy.id;

              return (
                <ListCard
                  key={proxy.id}
                  selected={isSelected}
                  onClick={() => {
                    setSelectedProxy(proxy);
                    setPanelTab("details");
                    setIsEditing(false);
                  }}
                >
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-3 min-w-0">
                      <div className="p-2 bg-gray-100 dark:bg-gray-800 rounded-lg">
                        <Globe className="h-4 w-4 text-gray-500" />
                      </div>
                      <div className="min-w-0">
                        <div className="flex items-center gap-2">
                          <span className="font-medium text-gray-900 dark:text-white truncate">
                            {proxy.domain}
                          </span>
                          {getSSLIcon(proxy)}
                        </div>
                        <p className="text-xs text-gray-500 dark:text-gray-400 truncate mt-0.5">
                          {proxy.upstream_target}
                        </p>
                      </div>
                    </div>
                    <div className="flex items-center gap-2">
                      <span className={cn("px-2 py-0.5 text-xs font-medium rounded-full", getStatusBadgeClass(proxy.status))}>
                        {proxy.status}
                      </span>
                      <ChevronRight className={cn("h-4 w-4 text-gray-400", isSelected && "text-primary-500")} />
                    </div>
                  </div>

                  <div className="flex items-center gap-3 mt-3 text-xs text-gray-500 dark:text-gray-400">
                    {proxy.http2_enabled && (
                      <span className="px-1.5 py-0.5 bg-gray-100 dark:bg-gray-800 rounded">HTTP/2</span>
                    )}
                    {proxy.force_ssl && (
                      <span className="px-1.5 py-0.5 bg-gray-100 dark:bg-gray-800 rounded">Force SSL</span>
                    )}
                    <span>{formatRelativeTime(proxy.created_at)}</span>
                  </div>
                </ListCard>
              );
            })}
          </div>
        ) : (
          <EmptyState
            icon={Globe}
            title="No proxy hosts configured"
            description="Click 'Add Proxy Host' to create your first reverse proxy"
          />
        )
      ) : (
        <EmptyState
          icon={Shield}
          title="Select an agent"
          description={activeAgents.length === 0
            ? "No active agents available. Register an agent first."
            : "Choose an agent to manage its proxy hosts"
          }
        />
      )}

      {/* Create Modal */}
      {showCreateModal && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-white dark:bg-gray-900 rounded-lg p-6 w-full max-w-md border border-gray-200 dark:border-gray-800">
            <div className="flex items-center justify-between mb-4">
              <h2 className="text-xl font-bold text-gray-900 dark:text-white">Add Proxy Host</h2>
              <button
                onClick={() => {
                  setShowCreateModal(false);
                  resetForm();
                }}
                className="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
              >
                <X className="h-5 w-5" />
              </button>
            </div>
            <form onSubmit={handleProxySubmit} className="space-y-4">
              <Input
                label="Domain"
                value={newProxy.domain}
                onChange={(e) => setNewProxy({ ...newProxy, domain: e.target.value })}
                placeholder="example.com"
                required
              />

              {/* Upstream Mode Toggle */}
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                  Upstream Type
                </label>
                <div className="flex gap-2">
                  <button
                    type="button"
                    onClick={() => setUpstreamMode("manual")}
                    className={cn(
                      "flex-1 flex items-center justify-center gap-2 px-4 py-2 rounded-lg border transition-colors",
                      upstreamMode === "manual"
                        ? "bg-primary-50 dark:bg-primary-900/20 border-primary-500 text-primary-700 dark:text-primary-400"
                        : "bg-gray-100 dark:bg-gray-800 border-gray-200 dark:border-gray-700 text-gray-600 dark:text-gray-400"
                    )}
                  >
                    <Globe className="h-4 w-4" />
                    Manual URL
                  </button>
                  <button
                    type="button"
                    onClick={() => setUpstreamMode("container")}
                    className={cn(
                      "flex-1 flex items-center justify-center gap-2 px-4 py-2 rounded-lg border transition-colors",
                      upstreamMode === "container"
                        ? "bg-primary-50 dark:bg-primary-900/20 border-primary-500 text-primary-700 dark:text-primary-400"
                        : "bg-gray-100 dark:bg-gray-800 border-gray-200 dark:border-gray-700 text-gray-600 dark:text-gray-400"
                    )}
                  >
                    <ContainerIcon className="h-4 w-4" />
                    Container
                  </button>
                </div>
              </div>

              {upstreamMode === "manual" ? (
                <Input
                  label="Upstream Target"
                  value={newProxy.upstream_target}
                  onChange={(e) => setNewProxy({ ...newProxy, upstream_target: e.target.value })}
                  placeholder="http://localhost:3000"
                  required
                />
              ) : (
                <>
                  <div>
                    <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                      Container
                    </label>
                    <select
                      value={selectedContainer || ""}
                      onChange={(e) => setSelectedContainer(e.target.value || null)}
                      required
                      className="w-full px-4 py-2 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg text-gray-900 dark:text-white focus:ring-2 focus:ring-primary-500"
                    >
                      <option value="">Select a container...</option>
                      {containers?.filter((c) => c.status === "running").map((container) => (
                        <option key={container.container_id} value={container.container_id}>
                          {container.name} ({container.image})
                        </option>
                      ))}
                    </select>
                  </div>
                  <Input
                    label="Container Port"
                    value={containerPort}
                    onChange={(e) => setContainerPort(e.target.value)}
                    placeholder="80"
                    required
                  />
                  {selectedContainer && containerNetworks?.[0] && (
                    <div
                      className={cn(
                        "flex items-center gap-2 px-3 py-2 rounded-lg border text-sm",
                        nginxNetworkCheck?.connected
                          ? "bg-green-50 dark:bg-green-900/20 border-green-200 dark:border-green-800 text-green-700 dark:text-green-400"
                          : "bg-yellow-50 dark:bg-yellow-900/20 border-yellow-200 dark:border-yellow-800 text-yellow-700 dark:text-yellow-400"
                      )}
                    >
                      {nginxNetworkCheck?.connected ? <Check className="h-4 w-4" /> : <AlertTriangle className="h-4 w-4" />}
                      <span>
                        {nginxNetworkCheck?.connected
                          ? `Connected to ${containerNetworks[0].network_name}`
                          : `Nginx not on ${containerNetworks[0].network_name}`}
                      </span>
                    </div>
                  )}
                  {selectedContainer && (
                    <p className="text-sm text-gray-500">
                      Upstream:{" "}
                      <code className="bg-gray-100 dark:bg-gray-800 px-2 py-0.5 rounded">
                        {buildContainerUpstream(containers?.find((c) => c.container_id === selectedContainer))}
                      </code>
                    </p>
                  )}
                </>
              )}

              <div className="flex items-center gap-4">
                <label className="flex items-center gap-2 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={newProxy.force_ssl}
                    onChange={(e) => setNewProxy({ ...newProxy, force_ssl: e.target.checked })}
                    className="w-4 h-4 rounded"
                  />
                  <span className="text-sm text-gray-700 dark:text-gray-300">Force SSL</span>
                </label>
                <label className="flex items-center gap-2 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={newProxy.http2_enabled}
                    onChange={(e) => setNewProxy({ ...newProxy, http2_enabled: e.target.checked })}
                    className="w-4 h-4 rounded"
                  />
                  <span className="text-sm text-gray-700 dark:text-gray-300">HTTP/2</span>
                </label>
              </div>

              <div className="flex justify-end gap-3 pt-4">
                <Button
                  type="button"
                  variant="ghost"
                  onClick={() => {
                    setShowCreateModal(false);
                    resetForm();
                  }}
                >
                  Cancel
                </Button>
                <Button type="submit" variant="primary" disabled={createMutation.isPending}>
                  {createMutation.isPending ? "Creating..." : "Create"}
                </Button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Network Warning Modal */}
      {showNetworkWarning && networkToAttach && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-white dark:bg-gray-900 rounded-lg p-6 w-full max-w-md border border-gray-200 dark:border-gray-800">
            <div className="flex items-center gap-3 mb-4">
              <div className="p-2 bg-yellow-100 dark:bg-yellow-900/30 rounded-lg">
                <AlertTriangle className="h-6 w-6 text-yellow-600 dark:text-yellow-400" />
              </div>
              <h2 className="text-xl font-bold text-gray-900 dark:text-white">Network Attachment Required</h2>
            </div>
            <p className="text-gray-600 dark:text-gray-400 mb-4">
              Nginx needs to be connected to the network{" "}
              <code className="bg-gray-100 dark:bg-gray-800 px-2 py-0.5 rounded text-primary-600 dark:text-primary-400">
                {networkToAttach.name}
              </code>{" "}
              to proxy traffic to this container.
            </p>
            <div className="flex justify-end gap-3">
              <Button
                variant="ghost"
                onClick={() => {
                  setShowNetworkWarning(false);
                  setNetworkToAttach(null);
                  setPendingProxySubmit(false);
                }}
              >
                Cancel
              </Button>
              <Button
                variant="primary"
                icon={Network}
                onClick={() => attachNetworkMutation.mutate(networkToAttach.id)}
                disabled={attachNetworkMutation.isPending}
              >
                {attachNetworkMutation.isPending ? "Attaching..." : "Attach Network"}
              </Button>
            </div>
          </div>
        </div>
      )}
    </PageLayout>
  );
}
