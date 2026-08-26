"use client";

import { useState, useEffect } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Globe,
  Plus,
  Shield,
  ShieldCheck,
  AlertTriangle,
  Container as ContainerIcon,
  Network,
  Check,
  Lock,
  X,
  RefreshCw,
  Loader2,
} from "lucide-react";
import { api, Container, ProxyHost, TestNetworkResponse } from "@/lib/api";
import { Link2, ArrowRight } from "lucide-react";
import { formatRelativeTime, cn } from "@/lib/utils";
import { useTraffic, getSSLStatus, getProxyStatus } from "@/lib/traffic-context";
import { StatCard, MetricsGrid } from "@/components/ui/StatCard";
import { Table } from "@/components/ui/Table";
import { Badge } from "@/components/ui/Badge";
import { StatusIndicator } from "@/components/ui/StatusIndicator";
import { EmptyState } from "@/components/ui/EmptyState";
import { Spinner } from "@/components/ui/Spinner";
import { Card } from "@/components/ui/Card";
import {
  Button,
  Input,
} from "@/components/ui/page-layout";
import { FilterToolbar, SelectOption } from "@/components/ui/FilterToolbar";

type UpstreamMode = "manual" | "container";
type ProxyTypeMode = "upstream" | "redirect";

export default function ProxiesPage() {
  const queryClient = useQueryClient();
  const { selectedAgent, setSelectedAgent, openProxyPanel } = useTraffic();
  const [showCreateModal, setShowCreateModal] = useState(false);

  // Form state for create
  const [proxyType, setProxyType] = useState<ProxyTypeMode>("upstream");
  const [upstreamMode, setUpstreamMode] = useState<UpstreamMode>("manual");
  const [selectedContainer, setSelectedContainer] = useState<string | null>(null);
  const [containerPort, setContainerPort] = useState<string>("80");
  const [redirectUrl, setRedirectUrl] = useState("");
  const [redirectCode, setRedirectCode] = useState(301);
  const [networkTestResult, setNetworkTestResult] = useState<TestNetworkResponse | null>(null);
  const [testingNetwork, setTestingNetwork] = useState(false);
  const [newProxy, setNewProxy] = useState({
    domain: "",
    upstream_target: "",
    force_ssl: true,
    http2_enabled: true,
    include_www: false,
    access_log: true,
    error_log: true,
  });

  // Network warning state
  const [showNetworkWarning, setShowNetworkWarning] = useState(false);
  const [networkToAttach, setNetworkToAttach] = useState<{ id: string; name: string } | null>(null);
  const [pendingProxySubmit, setPendingProxySubmit] = useState(false);

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

  // Build agent options for FilterToolbar
  const agentOptions: SelectOption[] = agents
    ? agents.map((a) => ({
        value: a.id,
        label: `${a.name} (${a.status})`,
        disabled: a.status !== "active",
      }))
    : [];

  // Auto-select first active agent
  useEffect(() => {
    if (!selectedAgent && activeAgents.length > 0) {
      setSelectedAgent(activeAgents[0].id);
    }
  }, [activeAgents, selectedAgent, setSelectedAgent]);

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
    mutationFn: (data: {
      domain: string;
      upstream_target?: string;
      proxy_type?: 'upstream' | 'redirect';
      redirect_url?: string;
      redirect_code?: number;
      force_ssl?: boolean;
      http2_enabled?: boolean;
      include_www?: boolean;
      access_log?: boolean;
      error_log?: boolean;
    }) => api.createProxyHost(selectedAgent!, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["proxies", selectedAgent] });
      setShowCreateModal(false);
      resetForm();
    },
  });

  // Nginx operations
  const testNginxMutation = useMutation({
    mutationFn: () => (selectedAgent ? api.testNginxConfig(selectedAgent) : Promise.reject("No agent")),
  });

  const reloadNginxMutation = useMutation({
    mutationFn: () => (selectedAgent ? api.reloadNginx(selectedAgent) : Promise.reject("No agent")),
  });

  // Helper functions
  const resetForm = () => {
    setNewProxy({
      domain: "",
      upstream_target: "",
      force_ssl: true,
      http2_enabled: true,
      include_www: false,
      access_log: true,
      error_log: true,
    });
    setProxyType("upstream");
    setUpstreamMode("manual");
    setSelectedContainer(null);
    setContainerPort("80");
    setRedirectUrl("");
    setRedirectCode(301);
    setNetworkTestResult(null);
    setTestingNetwork(false);
    setPendingProxySubmit(false);
  };

  // Network test function
  const handleNetworkTest = async () => {
    if (!selectedAgent) return;

    let containerName = "";
    let port = parseInt(containerPort, 10);

    if (upstreamMode === "container" && selectedContainer) {
      const container = containers?.find((c) => c.container_id === selectedContainer);
      containerName = container?.name?.replace(/^\//, "") || "";
    } else if (upstreamMode === "manual" && newProxy.upstream_target) {
      const match = newProxy.upstream_target.match(/^https?:\/\/([^:/]+):?(\d+)?/);
      if (match) {
        containerName = match[1];
        port = match[2] ? parseInt(match[2], 10) : 80;
      }
    }

    if (!containerName || !port) return;

    setTestingNetwork(true);
    setNetworkTestResult(null);

    try {
      const result = await api.testNetworkConnectivity(selectedAgent, {
        container_name: containerName,
        port: port,
      });
      setNetworkTestResult(result);
    } catch (error) {
      setNetworkTestResult({
        reachable: false,
        message: "Failed to test network connectivity",
      });
    } finally {
      setTestingNetwork(false);
    }
  };

  const buildContainerUpstream = (container: Container | undefined) => {
    if (!container) return "";
    const containerName = container.name.startsWith("/") ? container.name.slice(1) : container.name;
    return `http://${containerName}:${containerPort}`;
  };

  const handleProxySubmit = (e: React.FormEvent) => {
    e.preventDefault();

    // Handle redirect type proxy
    if (proxyType === "redirect") {
      const proxyData = {
        domain: newProxy.domain,
        proxy_type: "redirect" as const,
        redirect_url: redirectUrl,
        redirect_code: redirectCode,
        force_ssl: newProxy.force_ssl,
        http2_enabled: newProxy.http2_enabled,
        include_www: newProxy.include_www,
        access_log: newProxy.access_log,
        error_log: newProxy.error_log,
      };
      createMutation.mutate(proxyData);
      return;
    }

    // Handle upstream type proxy
    if (upstreamMode === "container" && selectedContainer) {
      const container = containers?.find((c) => c.container_id === selectedContainer);
      const upstream = buildContainerUpstream(container);
      const proxyData = {
        ...newProxy,
        upstream_target: upstream,
        proxy_type: "upstream" as const,
      };
      setNewProxy(proxyData);

      if (containerNetworks?.[0] && !nginxNetworkCheck?.connected) {
        setNetworkToAttach({ id: containerNetworks[0].network_id, name: containerNetworks[0].network_name });
        setShowNetworkWarning(true);
        setPendingProxySubmit(true);
        return;
      }
      createMutation.mutate(proxyData);
    } else {
      createMutation.mutate({ ...newProxy, proxy_type: "upstream" as const });
    }
  };

  // Calculate metrics
  const totalProxies = proxies?.length || 0;
  const sslEnabled = proxies?.filter((p: ProxyHost) => p.ssl_enabled).length || 0;
  const activeProxies = proxies?.filter((p: ProxyHost) => p.status === "active").length || 0;
  const uniqueDomains = new Set(proxies?.map((p: ProxyHost) => p.domain.split('.').slice(-2).join('.'))).size || 0;

  // Define table columns
  const columns = [
    {
      key: "domain",
      header: "Domain",
      sortable: true,
      render: (value: string, row: ProxyHost) => (
        <div className="flex items-center gap-2">
          <Globe className="h-4 w-4 text-gray-400" />
          <span className="font-medium">{value}</span>
          {row.is_system_proxy && (
            <Badge color="blue" size="sm">InfraPilot</Badge>
          )}
          {row.proxy_type === "redirect" && (
            <Badge color="purple" size="sm">Redirect</Badge>
          )}
          <StatusIndicator status={getSSLStatus(row)} showLabel={false} size="sm" />
        </div>
      ),
    },
    {
      key: "upstream_target",
      header: "Target",
      render: (value: string, row: ProxyHost) => (
        <div className="flex items-center gap-1">
          {row.proxy_type === "redirect" ? (
            <>
              <ArrowRight className="h-3 w-3 text-purple-500" />
              <code className="text-xs text-purple-600 dark:text-purple-400">{row.redirect_url}</code>
            </>
          ) : (
            <code className="text-xs text-gray-600 dark:text-gray-400">{value}</code>
          )}
        </div>
      ),
    },
    {
      key: "status",
      header: "Status",
      sortable: true,
      render: (value: string) => (
        <StatusIndicator status={getProxyStatus(value)} />
      ),
    },
    {
      key: "created_at",
      header: "Created",
      sortable: true,
      render: (value: string) => (
        <span className="text-sm text-gray-600 dark:text-gray-400">
          {formatRelativeTime(value)}
        </span>
      ),
    },
  ];

  return (
    <div className="space-y-6">
      {/* Nginx operation status messages */}
      {(testNginxMutation.isSuccess || testNginxMutation.isError || reloadNginxMutation.isSuccess || reloadNginxMutation.isError) && (
        <div
          className={cn(
            "p-3 rounded-lg text-sm flex items-center justify-between",
            (testNginxMutation.data?.success || reloadNginxMutation.data?.success)
              ? "bg-green-500/10 border border-green-500/30 text-green-700 dark:text-green-400"
              : "bg-red-500/10 border border-red-500/30 text-red-700 dark:text-red-400"
          )}
        >
          <div className="flex items-center gap-2">
            {(testNginxMutation.data?.success || reloadNginxMutation.data?.success) ? (
              <Check className="h-4 w-4" />
            ) : (
              <AlertTriangle className="h-4 w-4" />
            )}
            <span>{testNginxMutation.data?.message || reloadNginxMutation.data?.message || "Operation failed"}</span>
          </div>
          <button
            onClick={() => {
              testNginxMutation.reset();
              reloadNginxMutation.reset();
            }}
            className="text-gray-500 hover:text-gray-700 dark:hover:text-gray-300"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
      )}

      {/* Metrics */}
      {selectedAgent && (
        <MetricsGrid columns={4}>
          <StatCard
            label="Total Proxies"
            value={totalProxies}
            icon={Globe}
            iconColor="text-blue-600"
          />
          <StatCard
            label="SSL Enabled"
            value={sslEnabled}
            description={`${totalProxies > 0 ? Math.round((sslEnabled / totalProxies) * 100) : 0}% of total`}
            icon={ShieldCheck}
            iconColor="text-green-600"
          />
          <StatCard
            label="Active"
            value={activeProxies}
            icon={Check}
            iconColor="text-emerald-600"
          />
          <StatCard
            label="Domains"
            value={uniqueDomains}
            icon={Network}
            iconColor="text-purple-600"
          />
        </MetricsGrid>
      )}

      {/* Filter Toolbar */}
      <FilterToolbar
        agents={agentOptions}
        selectedAgent={selectedAgent}
        onAgentChange={setSelectedAgent}
        showAgentFilter={true}
        showSearch={false}
        showRefresh={true}
        onRefresh={() => queryClient.invalidateQueries({ queryKey: ["proxies", selectedAgent] })}
        singleRow={true}
        customActions={
          <div className="flex items-center gap-2">
            <button
              onClick={() => testNginxMutation.mutate()}
              disabled={!selectedAgent || testNginxMutation.isPending}
              className={cn(
                "px-3 py-1.5 text-sm rounded-lg transition-colors flex items-center gap-1.5",
                testNginxMutation.isSuccess && testNginxMutation.data?.success
                  ? "bg-green-500/10 text-green-600 dark:text-green-400 border border-green-500/30"
                  : testNginxMutation.isError || (testNginxMutation.isSuccess && !testNginxMutation.data?.success)
                  ? "bg-red-500/10 text-red-600 dark:text-red-400 border border-red-500/30"
                  : "bg-gray-100 dark:bg-gray-800 text-gray-700 dark:text-gray-300 hover:bg-gray-200 dark:hover:bg-gray-700 disabled:opacity-50"
              )}
              title={testNginxMutation.data?.message || "Test nginx configuration"}
            >
              {testNginxMutation.isPending ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : testNginxMutation.isSuccess && testNginxMutation.data?.success ? (
                <Check className="h-4 w-4" />
              ) : (
                <AlertTriangle className="h-4 w-4" />
              )}
              {testNginxMutation.isPending ? "Testing..." : "Test Config"}
            </button>
            <button
              onClick={() => reloadNginxMutation.mutate()}
              disabled={!selectedAgent || reloadNginxMutation.isPending}
              className={cn(
                "px-3 py-1.5 text-sm rounded-lg transition-colors flex items-center gap-1.5",
                reloadNginxMutation.isSuccess && reloadNginxMutation.data?.success
                  ? "bg-green-500/10 text-green-600 dark:text-green-400 border border-green-500/30"
                  : reloadNginxMutation.isError
                  ? "bg-red-500/10 text-red-600 dark:text-red-400 border border-red-500/30"
                  : "bg-blue-600 hover:bg-blue-700 text-white disabled:opacity-50"
              )}
              title={reloadNginxMutation.data?.message || "Reload nginx"}
            >
              {reloadNginxMutation.isPending ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <RefreshCw className="h-4 w-4" />
              )}
              {reloadNginxMutation.isPending ? "Reloading..." : "Reload Nginx"}
            </button>
            <Button
              variant="primary"
              icon={Plus}
              onClick={() => setShowCreateModal(true)}
              disabled={!selectedAgent}
            >
              Add Proxy Host
            </Button>
          </div>
        }
      />

      {/* Proxies table */}
      {selectedAgent ? (
        proxiesLoading ? (
          <div className="flex items-center justify-center h-64">
            <Spinner size="lg" />
          </div>
        ) : proxies && proxies.length > 0 ? (
          <Card>
            <Table
              columns={columns}
              data={proxies}
              keyExtractor={(row: ProxyHost) => row.id}
              onRowClick={(row: ProxyHost) => {
                openProxyPanel(row.id);
              }}
              hoverable
            />
          </Card>
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
          <div className="bg-white dark:bg-gray-900 rounded-lg p-6 w-full max-w-md border border-gray-200 dark:border-gray-800 max-h-[90vh] overflow-y-auto">
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
              {/* Proxy Type Toggle */}
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                  Proxy Type
                </label>
                <div className="flex gap-2">
                  <button
                    type="button"
                    onClick={() => setProxyType("upstream")}
                    className={cn(
                      "flex-1 flex flex-col items-center justify-center gap-1 px-4 py-3 rounded-lg border transition-colors",
                      proxyType === "upstream"
                        ? "bg-primary-50 dark:bg-primary-900/20 border-primary-500 text-primary-700 dark:text-primary-400"
                        : "bg-gray-100 dark:bg-gray-800 border-gray-200 dark:border-gray-700 text-gray-600 dark:text-gray-400"
                    )}
                  >
                    <ContainerIcon className="h-5 w-5" />
                    <span className="text-sm font-medium">Container Proxy</span>
                    <span className="text-xs opacity-70">Forward to service</span>
                  </button>
                  <button
                    type="button"
                    onClick={() => setProxyType("redirect")}
                    className={cn(
                      "flex-1 flex flex-col items-center justify-center gap-1 px-4 py-3 rounded-lg border transition-colors",
                      proxyType === "redirect"
                        ? "bg-purple-50 dark:bg-purple-900/20 border-purple-500 text-purple-700 dark:text-purple-400"
                        : "bg-gray-100 dark:bg-gray-800 border-gray-200 dark:border-gray-700 text-gray-600 dark:text-gray-400"
                    )}
                  >
                    <Link2 className="h-5 w-5" />
                    <span className="text-sm font-medium">URL Redirect</span>
                    <span className="text-xs opacity-70">Redirect to URL</span>
                  </button>
                </div>
              </div>

              <Input
                label="Domain"
                value={newProxy.domain}
                onChange={(e) => setNewProxy({ ...newProxy, domain: e.target.value })}
                placeholder="example.com"
                required
              />

              {proxyType === "redirect" ? (
                <>
                  <Input
                    label="Redirect URL"
                    value={redirectUrl}
                    onChange={(e) => setRedirectUrl(e.target.value)}
                    placeholder="https://devsimplex.com"
                    required
                  />
                  <div>
                    <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                      Redirect Type
                    </label>
                    <select
                      value={redirectCode}
                      onChange={(e) => setRedirectCode(parseInt(e.target.value, 10))}
                      className="w-full px-4 py-2 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg text-gray-900 dark:text-white focus:ring-2 focus:ring-primary-500"
                    >
                      <option value={301}>301 - Permanent Redirect</option>
                      <option value={302}>302 - Temporary Redirect</option>
                      <option value={307}>307 - Temporary Redirect (preserve method)</option>
                      <option value={308}>308 - Permanent Redirect (preserve method)</option>
                    </select>
                  </div>
                </>
              ) : (
                <>
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
                    <div className="space-y-2">
                      <div className="flex gap-2">
                        <div className="flex-1">
                          <Input
                            label="Upstream Target"
                            value={newProxy.upstream_target}
                            onChange={(e) => setNewProxy({ ...newProxy, upstream_target: e.target.value })}
                            placeholder="http://backend:3000"
                            required
                          />
                          <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
                            Proxying something running on this host machine, not in a container?{" "}
                            <button
                              type="button"
                              onClick={() =>
                                setNewProxy({ ...newProxy, upstream_target: "http://host.docker.internal:" })
                              }
                              className="underline hover:no-underline text-primary-600 dark:text-primary-400"
                            >
                              Use host.docker.internal
                            </button>{" "}
                            instead of a Docker bridge IP.
                          </p>
                        </div>
                        <div className="flex items-end">
                          <Button
                            type="button"
                            variant="secondary"
                            onClick={handleNetworkTest}
                            disabled={testingNetwork || !newProxy.upstream_target}
                          >
                            {testingNetwork ? <Loader2 className="h-4 w-4 animate-spin" /> : "Test"}
                          </Button>
                        </div>
                      </div>
                      {networkTestResult && (
                        <div
                          className={cn(
                            "p-3 rounded-lg border text-sm",
                            networkTestResult.reachable
                              ? "bg-green-50 dark:bg-green-900/20 border-green-200 dark:border-green-800 text-green-700 dark:text-green-400"
                              : "bg-yellow-50 dark:bg-yellow-900/20 border-yellow-200 dark:border-yellow-800 text-yellow-700 dark:text-yellow-400"
                          )}
                        >
                          <div className="flex items-center gap-2">
                            {networkTestResult.reachable ? <Check className="h-4 w-4" /> : <AlertTriangle className="h-4 w-4" />}
                            <span>{networkTestResult.message}</span>
                          </div>
                        </div>
                      )}
                    </div>
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
                            "flex items-center justify-between px-3 py-2 rounded-lg border text-sm",
                            nginxNetworkCheck?.connected
                              ? "bg-green-50 dark:bg-green-900/20 border-green-200 dark:border-green-800 text-green-700 dark:text-green-400"
                              : "bg-yellow-50 dark:bg-yellow-900/20 border-yellow-200 dark:border-yellow-800 text-yellow-700 dark:text-yellow-400"
                          )}
                        >
                          <div className="flex items-center gap-2">
                            {nginxNetworkCheck?.connected ? <Check className="h-4 w-4" /> : <AlertTriangle className="h-4 w-4" />}
                            <span>
                              {nginxNetworkCheck?.connected
                                ? `Connected to ${containerNetworks[0].network_name}`
                                : `Nginx not on ${containerNetworks[0].network_name}`}
                            </span>
                          </div>
                          {!nginxNetworkCheck?.connected && (
                            <Button
                              type="button"
                              variant="primary"
                              size="sm"
                              icon={Network}
                              onClick={() => attachNetworkMutation.mutate(containerNetworks[0].network_id)}
                              disabled={attachNetworkMutation.isPending}
                            >
                              {attachNetworkMutation.isPending ? "Attaching..." : "Attach"}
                            </Button>
                          )}
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
                </>
              )}

              {/* Options */}
              <div className="space-y-3 pt-2 border-t border-gray-200 dark:border-gray-700">
                <div className="flex flex-wrap items-center gap-4">
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
                  <label className="flex items-center gap-2 cursor-pointer">
                    <input
                      type="checkbox"
                      checked={newProxy.include_www}
                      onChange={(e) => setNewProxy({ ...newProxy, include_www: e.target.checked })}
                      className="w-4 h-4 rounded"
                    />
                    <span className="text-sm text-gray-700 dark:text-gray-300">Include www</span>
                  </label>
                </div>

                {/* Logging options */}
                <div className="flex flex-wrap items-center gap-4">
                  <label className="flex items-center gap-2 cursor-pointer">
                    <input
                      type="checkbox"
                      checked={newProxy.access_log}
                      onChange={(e) => setNewProxy({ ...newProxy, access_log: e.target.checked })}
                      className="w-4 h-4 rounded"
                    />
                    <span className="text-sm text-gray-700 dark:text-gray-300">Access Log</span>
                  </label>
                  <label className="flex items-center gap-2 cursor-pointer">
                    <input
                      type="checkbox"
                      checked={newProxy.error_log}
                      onChange={(e) => setNewProxy({ ...newProxy, error_log: e.target.checked })}
                      className="w-4 h-4 rounded"
                    />
                    <span className="text-sm text-gray-700 dark:text-gray-300">Error Log</span>
                  </label>
                </div>
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
    </div>
  );
}
