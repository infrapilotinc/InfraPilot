"use client";

import { useState } from "react";
import { useParams, useRouter } from "next/navigation";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  ArrowLeft,
  Play,
  Square,
  RotateCcw,
  Cpu,
  MemoryStick,
  HardDrive,
  Network,
  Settings,
  FileText,
  Terminal as TerminalIcon,
  Key,
  Copy,
  Check,
  Heart,
  AlertTriangle,
  RefreshCw,
  Box,
  Clock,
  Server,
  Shield,
  Layers,
  ExternalLink,
  Trash2,
  X,
} from "lucide-react";
import { api, ContainerDetail } from "@/lib/api";
import { Terminal } from "@/components/containers/Terminal";
import { cn } from "@/lib/utils";

type TabId = "overview" | "environment" | "mounts" | "network" | "config" | "logs" | "terminal";

export default function ContainerDetailPage() {
  const params = useParams();
  const router = useRouter();
  const queryClient = useQueryClient();
  const agentId = params.agentId as string;
  const containerId = params.containerId as string;

  const [activeTab, setActiveTab] = useState<TabId>("overview");
  const [copied, setCopied] = useState<string | null>(null);
  const [logsTail, setLogsTail] = useState(100);
  const [showDeleteModal, setShowDeleteModal] = useState(false);
  const [deleteConfirmName, setDeleteConfirmName] = useState("");
  const [forceDelete, setForceDelete] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);
  const [showStopModal, setShowStopModal] = useState(false);
  const [showRestartModal, setShowRestartModal] = useState(false);

  // Fetch container details
  const { data: container, isLoading, error } = useQuery({
    queryKey: ["containerDetail", agentId, containerId],
    queryFn: () => api.getContainerDetail(agentId, containerId),
    refetchInterval: activeTab === "overview" ? 5000 : false, // Auto-refresh stats on overview
  });

  // Fetch logs when on logs tab
  const { data: logsData, isLoading: logsLoading, refetch: refetchLogs } = useQuery({
    queryKey: ["containerLogs", agentId, containerId, logsTail],
    queryFn: () => api.getContainerLogs(agentId, containerId, logsTail),
    enabled: activeTab === "logs",
  });

  // Container actions
  const startMutation = useMutation({
    mutationFn: () => api.startContainer(agentId, containerId),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["containerDetail", agentId, containerId] }),
  });

  const stopMutation = useMutation({
    mutationFn: () => api.stopContainer(agentId, containerId),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["containerDetail", agentId, containerId] }),
  });

  const restartMutation = useMutation({
    mutationFn: () => api.restartContainer(agentId, containerId),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["containerDetail", agentId, containerId] }),
  });

  const deleteMutation = useMutation({
    mutationFn: () => api.deleteContainer(agentId, containerId, deleteConfirmName, forceDelete),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["containers", agentId] });
      router.push("/containers");
    },
    onError: (error: Error) => {
      setDeleteError(error.message);
    },
  });

  const handleDelete = () => {
    setDeleteError(null);
    deleteMutation.mutate();
  };

  const handleCopy = (text: string, key: string) => {
    navigator.clipboard.writeText(text);
    setCopied(key);
    setTimeout(() => setCopied(null), 2000);
  };

  const tabs = [
    { id: "overview", label: "Overview", icon: Box },
    { id: "environment", label: "Environment", icon: Key },
    { id: "mounts", label: "Mounts", icon: HardDrive },
    { id: "network", label: "Network", icon: Network },
    { id: "config", label: "Config", icon: Settings },
    { id: "logs", label: "Logs", icon: FileText },
    ...(container?.status === "running" ? [{ id: "terminal" as TabId, label: "Terminal", icon: TerminalIcon }] : []),
  ];

  const getStatusColor = (status: string) => {
    switch (status) {
      case "running": return "bg-green-500";
      case "exited": return "bg-red-500";
      case "paused": return "bg-yellow-500";
      default: return "bg-gray-500";
    }
  };

  const getStatusBadge = (status: string) => {
    switch (status) {
      case "running": return "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400";
      case "exited": return "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400";
      case "paused": return "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400";
      default: return "bg-gray-100 text-gray-700 dark:bg-gray-800 dark:text-gray-400";
    }
  };

  const getHealthBadge = (status?: string) => {
    switch (status) {
      case "healthy": return "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400";
      case "unhealthy": return "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400";
      case "starting": return "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400";
      default: return "bg-gray-100 text-gray-700 dark:bg-gray-800 dark:text-gray-400";
    }
  };

  const formatBytes = (bytes: number) => {
    if (bytes === 0) return "0 B";
    const k = 1024;
    const sizes = ["B", "KB", "MB", "GB", "TB"];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + " " + sizes[i];
  };

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-500" />
      </div>
    );
  }

  if (error || !container) {
    return (
      <div className="p-8">
        <div className="text-center text-red-500">
          Failed to load container details
        </div>
      </div>
    );
  }

  const renderTabContent = () => {
    switch (activeTab) {
      case "overview":
        return (
          <div className="space-y-6">
            {/* Stats Cards */}
            <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
              <div className="bg-white dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-800 p-4">
                <div className="flex items-center gap-2 text-gray-500 dark:text-gray-400 text-sm mb-2">
                  <Cpu className="h-4 w-4" />
                  CPU Usage
                </div>
                <div className="text-2xl font-bold text-gray-900 dark:text-white">
                  {container.cpu_percent?.toFixed(1) || 0}%
                </div>
              </div>
              <div className="bg-white dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-800 p-4">
                <div className="flex items-center gap-2 text-gray-500 dark:text-gray-400 text-sm mb-2">
                  <MemoryStick className="h-4 w-4" />
                  Memory
                </div>
                <div className="text-2xl font-bold text-gray-900 dark:text-white">
                  {container.memory_mb || 0} MB
                </div>
                {container.memory_limit_mb && container.memory_limit_mb > 0 && (
                  <div className="text-xs text-gray-500 dark:text-gray-400 mt-1">
                    of {container.memory_limit_mb} MB limit
                  </div>
                )}
              </div>
              <div className="bg-white dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-800 p-4">
                <div className="flex items-center gap-2 text-gray-500 dark:text-gray-400 text-sm mb-2">
                  <RefreshCw className="h-4 w-4" />
                  Restarts
                </div>
                <div className="text-2xl font-bold text-gray-900 dark:text-white">
                  {container.restart_count || 0}
                </div>
              </div>
              <div className="bg-white dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-800 p-4">
                <div className="flex items-center gap-2 text-gray-500 dark:text-gray-400 text-sm mb-2">
                  <Heart className="h-4 w-4" />
                  Health
                </div>
                {container.health_status ? (
                  <span className={cn("px-2 py-1 text-sm font-medium rounded-full capitalize", getHealthBadge(container.health_status))}>
                    {container.health_status}
                  </span>
                ) : (
                  <span className="text-gray-500 dark:text-gray-400 text-sm">No healthcheck</span>
                )}
              </div>
            </div>

            {/* Container Info */}
            <div className="bg-white dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-800 p-6">
              <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">Container Info</h3>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <div>
                  <label className="text-sm text-gray-500 dark:text-gray-400">Container ID</label>
                  <div className="flex items-center gap-2 mt-1">
                    <code className="text-sm font-mono text-gray-900 dark:text-white bg-gray-100 dark:bg-gray-800 px-2 py-1 rounded">
                      {container.container_id.slice(0, 12)}
                    </code>
                    <button onClick={() => handleCopy(container.container_id, "id")} className="text-gray-400 hover:text-gray-600">
                      {copied === "id" ? <Check className="h-4 w-4 text-green-500" /> : <Copy className="h-4 w-4" />}
                    </button>
                  </div>
                </div>
                <div>
                  <label className="text-sm text-gray-500 dark:text-gray-400">Image</label>
                  <div className="flex items-center gap-2 mt-1">
                    <code className="text-sm font-mono text-gray-900 dark:text-white bg-gray-100 dark:bg-gray-800 px-2 py-1 rounded truncate max-w-[300px]">
                      {container.image}
                    </code>
                    <button onClick={() => handleCopy(container.image, "image")} className="text-gray-400 hover:text-gray-600">
                      {copied === "image" ? <Check className="h-4 w-4 text-green-500" /> : <Copy className="h-4 w-4" />}
                    </button>
                  </div>
                </div>
                <div>
                  <label className="text-sm text-gray-500 dark:text-gray-400">Created</label>
                  <p className="text-gray-900 dark:text-white mt-1">
                    {container.created_at ? new Date(container.created_at).toLocaleString() : "Unknown"}
                  </p>
                </div>
                <div>
                  <label className="text-sm text-gray-500 dark:text-gray-400">Stack</label>
                  <p className="text-gray-900 dark:text-white mt-1 flex items-center gap-2">
                    {container.stack_name ? (
                      <>
                        <Layers className="h-4 w-4 text-primary-500" />
                        {container.stack_name}
                      </>
                    ) : (
                      "Standalone"
                    )}
                  </p>
                </div>
              </div>
            </div>

            {/* Ports */}
            {container.ports && container.ports.length > 0 && (
              <div className="bg-white dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-800 p-6">
                <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">Exposed Ports</h3>
                <div className="space-y-2">
                  {container.ports.map((port, idx) => (
                    <div key={idx} className="flex items-center gap-3 text-sm">
                      <span className="px-2 py-1 bg-blue-100 dark:bg-blue-900/30 text-blue-700 dark:text-blue-400 rounded font-mono">
                        {port.host_ip || "0.0.0.0"}:{port.host_port}
                      </span>
                      <span className="text-gray-500">→</span>
                      <span className="px-2 py-1 bg-gray-100 dark:bg-gray-800 text-gray-700 dark:text-gray-300 rounded font-mono">
                        {port.container_port}/{port.protocol}
                      </span>
                    </div>
                  ))}
                </div>
              </div>
            )}

            {/* Health Check Log */}
            {container.health_log && container.health_log.length > 0 && (
              <div className="bg-white dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-800 p-6">
                <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">Health Check History</h3>
                <div className="space-y-2 max-h-48 overflow-y-auto">
                  {container.health_log.slice(-5).reverse().map((entry, idx) => (
                    <div key={idx} className={cn(
                      "flex items-start gap-3 text-sm p-2 rounded",
                      entry.exit_code === 0 ? "bg-green-50 dark:bg-green-900/10" : "bg-red-50 dark:bg-red-900/10"
                    )}>
                      {entry.exit_code === 0 ? (
                        <Check className="h-4 w-4 text-green-500 flex-shrink-0 mt-0.5" />
                      ) : (
                        <AlertTriangle className="h-4 w-4 text-red-500 flex-shrink-0 mt-0.5" />
                      )}
                      <div className="flex-1 min-w-0">
                        <div className="text-gray-500 dark:text-gray-400 text-xs">
                          {new Date(entry.start).toLocaleString()}
                        </div>
                        <div className="text-gray-900 dark:text-white truncate font-mono text-xs mt-1">
                          {entry.output || "(no output)"}
                        </div>
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            )}
          </div>
        );

      case "environment":
        return (
          <div className="bg-white dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-800 p-6">
            <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">
              Environment Variables ({container.environment?.length || 0})
            </h3>
            {container.environment && container.environment.length > 0 ? (
              <div className="space-y-2">
                {container.environment.map((env, idx) => (
                  <div key={idx} className="flex items-start gap-3 py-2 border-b border-gray-100 dark:border-gray-800 last:border-0">
                    <div className="flex-shrink-0">
                      <code className="text-sm font-mono text-primary-600 dark:text-primary-400 bg-primary-50 dark:bg-primary-900/20 px-2 py-0.5 rounded">
                        {env.key}
                      </code>
                    </div>
                    <div className="flex-1 min-w-0 flex items-center gap-2">
                      <code className="text-sm font-mono text-gray-700 dark:text-gray-300 break-all">
                        {env.value || "(empty)"}
                      </code>
                      <button
                        onClick={() => handleCopy(`${env.key}=${env.value}`, `env-${idx}`)}
                        className="flex-shrink-0 text-gray-400 hover:text-gray-600"
                      >
                        {copied === `env-${idx}` ? <Check className="h-3.5 w-3.5 text-green-500" /> : <Copy className="h-3.5 w-3.5" />}
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            ) : (
              <p className="text-gray-500 dark:text-gray-400">No environment variables configured</p>
            )}
          </div>
        );

      case "mounts":
        return (
          <div className="bg-white dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-800 p-6">
            <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">
              Mounts & Volumes ({container.mounts?.length || 0})
            </h3>
            {container.mounts && container.mounts.length > 0 ? (
              <div className="space-y-4">
                {container.mounts.map((mount, idx) => (
                  <div key={idx} className="p-4 bg-gray-50 dark:bg-gray-800/50 rounded-lg">
                    <div className="flex items-center gap-2 mb-2">
                      <span className={cn(
                        "px-2 py-0.5 text-xs font-medium rounded",
                        mount.type === "volume" ? "bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400" :
                        mount.type === "bind" ? "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400" :
                        "bg-gray-100 text-gray-700 dark:bg-gray-700 dark:text-gray-300"
                      )}>
                        {mount.type}
                      </span>
                      {mount.read_only && (
                        <span className="px-2 py-0.5 text-xs font-medium rounded bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400">
                          read-only
                        </span>
                      )}
                    </div>
                    <div className="grid grid-cols-1 md:grid-cols-2 gap-2 text-sm">
                      <div>
                        <span className="text-gray-500 dark:text-gray-400">Source:</span>
                        <code className="ml-2 font-mono text-gray-900 dark:text-white break-all">
                          {mount.source}
                        </code>
                      </div>
                      <div>
                        <span className="text-gray-500 dark:text-gray-400">Destination:</span>
                        <code className="ml-2 font-mono text-gray-900 dark:text-white break-all">
                          {mount.destination}
                        </code>
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            ) : (
              <p className="text-gray-500 dark:text-gray-400">No mounts or volumes configured</p>
            )}
          </div>
        );

      case "network":
        return (
          <div className="space-y-6">
            {/* Network Connections */}
            <div className="bg-white dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-800 p-6">
              <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">
                Networks ({container.network_details?.length || 0})
              </h3>
              {container.network_details && container.network_details.length > 0 ? (
                <div className="space-y-4">
                  {container.network_details.map((network, idx) => (
                    <div key={idx} className="p-4 bg-gray-50 dark:bg-gray-800/50 rounded-lg">
                      <div className="flex items-center gap-2 mb-3">
                        <Network className="h-4 w-4 text-primary-500" />
                        <span className="font-medium text-gray-900 dark:text-white">{network.name}</span>
                      </div>
                      <div className="grid grid-cols-2 gap-3 text-sm">
                        <div>
                          <span className="text-gray-500 dark:text-gray-400">IP Address</span>
                          <p className="font-mono text-gray-900 dark:text-white">{network.ip_address || "N/A"}</p>
                        </div>
                        <div>
                          <span className="text-gray-500 dark:text-gray-400">Gateway</span>
                          <p className="font-mono text-gray-900 dark:text-white">{network.gateway || "N/A"}</p>
                        </div>
                        <div>
                          <span className="text-gray-500 dark:text-gray-400">MAC Address</span>
                          <p className="font-mono text-gray-900 dark:text-white">{network.mac_address || "N/A"}</p>
                        </div>
                        <div>
                          <span className="text-gray-500 dark:text-gray-400">Aliases</span>
                          <p className="font-mono text-gray-900 dark:text-white">
                            {network.aliases && network.aliases.length > 0 ? network.aliases.join(", ") : "None"}
                          </p>
                        </div>
                      </div>
                    </div>
                  ))}
                </div>
              ) : (
                <p className="text-gray-500 dark:text-gray-400">Not connected to any networks</p>
              )}
            </div>

            {/* Exposed Ports */}
            <div className="bg-white dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-800 p-6">
              <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">
                Port Mappings ({container.ports?.length || 0})
              </h3>
              {container.ports && container.ports.length > 0 ? (
                <div className="overflow-x-auto">
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="border-b border-gray-200 dark:border-gray-700">
                        <th className="text-left py-2 text-gray-500 dark:text-gray-400 font-medium">Host</th>
                        <th className="text-left py-2 text-gray-500 dark:text-gray-400 font-medium">Container</th>
                        <th className="text-left py-2 text-gray-500 dark:text-gray-400 font-medium">Protocol</th>
                      </tr>
                    </thead>
                    <tbody>
                      {container.ports.map((port, idx) => (
                        <tr key={idx} className="border-b border-gray-100 dark:border-gray-800 last:border-0">
                          <td className="py-2 font-mono text-gray-900 dark:text-white">
                            {port.host_ip || "0.0.0.0"}:{port.host_port}
                          </td>
                          <td className="py-2 font-mono text-gray-900 dark:text-white">{port.container_port}</td>
                          <td className="py-2">
                            <span className="px-2 py-0.5 text-xs font-medium rounded bg-gray-100 dark:bg-gray-800 text-gray-700 dark:text-gray-300">
                              {port.protocol}
                            </span>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              ) : (
                <p className="text-gray-500 dark:text-gray-400">No ports exposed</p>
              )}
            </div>
          </div>
        );

      case "config":
        return (
          <div className="space-y-6">
            {/* Container Config */}
            <div className="bg-white dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-800 p-6">
              <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">Container Configuration</h3>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <div>
                  <label className="text-sm text-gray-500 dark:text-gray-400">Hostname</label>
                  <p className="font-mono text-gray-900 dark:text-white">{container.config?.hostname || "N/A"}</p>
                </div>
                <div>
                  <label className="text-sm text-gray-500 dark:text-gray-400">User</label>
                  <p className="font-mono text-gray-900 dark:text-white">{container.config?.user || "root"}</p>
                </div>
                <div>
                  <label className="text-sm text-gray-500 dark:text-gray-400">Working Directory</label>
                  <p className="font-mono text-gray-900 dark:text-white">{container.config?.working_dir || "/"}</p>
                </div>
                <div>
                  <label className="text-sm text-gray-500 dark:text-gray-400">Restart Policy</label>
                  <p className="font-mono text-gray-900 dark:text-white">{container.config?.restart_policy || "no"}</p>
                </div>
                <div className="md:col-span-2">
                  <label className="text-sm text-gray-500 dark:text-gray-400">Entrypoint</label>
                  <code className="block mt-1 font-mono text-sm text-gray-900 dark:text-white bg-gray-100 dark:bg-gray-800 p-2 rounded">
                    {container.config?.entrypoint?.join(" ") || "(default)"}
                  </code>
                </div>
                <div className="md:col-span-2">
                  <label className="text-sm text-gray-500 dark:text-gray-400">Command</label>
                  <code className="block mt-1 font-mono text-sm text-gray-900 dark:text-white bg-gray-100 dark:bg-gray-800 p-2 rounded">
                    {container.config?.cmd?.join(" ") || "(none)"}
                  </code>
                </div>
              </div>

              {/* Flags */}
              <div className="flex flex-wrap gap-2 mt-4">
                {container.config?.privileged && (
                  <span className="px-2 py-1 text-xs font-medium rounded bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400 flex items-center gap-1">
                    <Shield className="h-3 w-3" /> Privileged
                  </span>
                )}
                {container.config?.tty && (
                  <span className="px-2 py-1 text-xs font-medium rounded bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400">
                    TTY
                  </span>
                )}
                {container.config?.open_stdin && (
                  <span className="px-2 py-1 text-xs font-medium rounded bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400">
                    STDIN
                  </span>
                )}
              </div>
            </div>

            {/* Resource Limits */}
            <div className="bg-white dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-800 p-6">
              <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">Resource Limits</h3>
              <div className="grid grid-cols-2 md:grid-cols-3 gap-4">
                <div>
                  <label className="text-sm text-gray-500 dark:text-gray-400">Memory Limit</label>
                  <p className="font-mono text-gray-900 dark:text-white">
                    {container.resources?.memory_limit ? formatBytes(container.resources.memory_limit) : "Unlimited"}
                  </p>
                </div>
                <div>
                  <label className="text-sm text-gray-500 dark:text-gray-400">Memory + Swap</label>
                  <p className="font-mono text-gray-900 dark:text-white">
                    {container.resources?.memory_swap && container.resources.memory_swap > 0
                      ? formatBytes(container.resources.memory_swap)
                      : "Unlimited"}
                  </p>
                </div>
                <div>
                  <label className="text-sm text-gray-500 dark:text-gray-400">CPU Shares</label>
                  <p className="font-mono text-gray-900 dark:text-white">
                    {container.resources?.cpu_shares || 1024}
                  </p>
                </div>
                <div>
                  <label className="text-sm text-gray-500 dark:text-gray-400">CPU Quota</label>
                  <p className="font-mono text-gray-900 dark:text-white">
                    {container.resources?.cpu_quota || "Unlimited"}
                  </p>
                </div>
                <div>
                  <label className="text-sm text-gray-500 dark:text-gray-400">CPUSet</label>
                  <p className="font-mono text-gray-900 dark:text-white">
                    {container.resources?.cpuset_cpus || "All"}
                  </p>
                </div>
                <div>
                  <label className="text-sm text-gray-500 dark:text-gray-400">PIDs Limit</label>
                  <p className="font-mono text-gray-900 dark:text-white">
                    {container.resources?.pids_limit || "Unlimited"}
                  </p>
                </div>
              </div>
            </div>

            {/* Health Check */}
            {container.health_check && (
              <div className="bg-white dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-800 p-6">
                <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">Health Check</h3>
                <div className="space-y-3">
                  <div>
                    <label className="text-sm text-gray-500 dark:text-gray-400">Test Command</label>
                    <code className="block mt-1 font-mono text-sm text-gray-900 dark:text-white bg-gray-100 dark:bg-gray-800 p-2 rounded">
                      {container.health_check.test.join(" ")}
                    </code>
                  </div>
                  <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
                    <div>
                      <label className="text-sm text-gray-500 dark:text-gray-400">Interval</label>
                      <p className="font-mono text-gray-900 dark:text-white">{container.health_check.interval}</p>
                    </div>
                    <div>
                      <label className="text-sm text-gray-500 dark:text-gray-400">Timeout</label>
                      <p className="font-mono text-gray-900 dark:text-white">{container.health_check.timeout}</p>
                    </div>
                    <div>
                      <label className="text-sm text-gray-500 dark:text-gray-400">Start Period</label>
                      <p className="font-mono text-gray-900 dark:text-white">{container.health_check.start_period}</p>
                    </div>
                    <div>
                      <label className="text-sm text-gray-500 dark:text-gray-400">Retries</label>
                      <p className="font-mono text-gray-900 dark:text-white">{container.health_check.retries}</p>
                    </div>
                  </div>
                </div>
              </div>
            )}

            {/* Labels */}
            <div className="bg-white dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-800 p-6">
              <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">
                Labels ({Object.keys(container.labels || {}).length})
              </h3>
              {container.labels && Object.keys(container.labels).length > 0 ? (
                <div className="space-y-2 max-h-64 overflow-y-auto">
                  {Object.entries(container.labels).map(([key, value], idx) => (
                    <div key={idx} className="flex items-start gap-2 py-1 text-sm">
                      <code className="flex-shrink-0 font-mono text-primary-600 dark:text-primary-400">{key}</code>
                      <span className="text-gray-500">=</span>
                      <code className="font-mono text-gray-700 dark:text-gray-300 break-all">{value}</code>
                    </div>
                  ))}
                </div>
              ) : (
                <p className="text-gray-500 dark:text-gray-400">No labels configured</p>
              )}
            </div>
          </div>
        );

      case "logs":
        const logLines = logsData?.logs?.split("\n").filter((l: string) => l.trim()) || [];
        return (
          <div className="bg-gray-900 rounded-xl overflow-hidden">
            <div className="flex items-center justify-between px-4 py-2 bg-gray-800 border-b border-gray-700">
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
              <div className="flex items-center gap-2">
                <span className="text-xs text-gray-500">{logLines.length} lines</span>
                <button
                  onClick={() => refetchLogs()}
                  className="p-1.5 hover:bg-gray-700 rounded"
                >
                  <RefreshCw className={cn("h-4 w-4 text-gray-400", logsLoading && "animate-spin")} />
                </button>
              </div>
            </div>
            <div className="h-[500px] overflow-y-auto p-4 font-mono text-xs">
              {logsLoading ? (
                <div className="flex items-center justify-center h-32">
                  <div className="animate-spin rounded-full h-5 w-5 border-b-2 border-primary-500" />
                </div>
              ) : logLines.length > 0 ? (
                logLines.map((line: string, idx: number) => (
                  <div key={idx} className="text-gray-300 hover:bg-gray-800 py-0.5 px-2 -mx-2">
                    {line}
                  </div>
                ))
              ) : (
                <div className="text-center text-gray-500 py-8">No logs available</div>
              )}
            </div>
          </div>
        );

      case "terminal":
        return (
          <div className="h-[500px] bg-gray-900 rounded-xl overflow-hidden">
            <Terminal
              containerId={containerId}
              agentId={agentId}
              onClose={() => setActiveTab("overview")}
            />
          </div>
        );

      default:
        return null;
    }
  };

  return (
    <div className="p-4 lg:p-8">
      {/* Header */}
      <div className="mb-6">
        <button
          onClick={() => router.push("/containers")}
          className="flex items-center gap-2 text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200 mb-4"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to Containers
        </button>

        <div className="flex items-start justify-between">
          <div className="flex items-center gap-4">
            <div className={cn("w-3 h-3 rounded-full", getStatusColor(container.status))} />
            <div>
              <h1 className="text-2xl font-bold text-gray-900 dark:text-white">{container.name}</h1>
              <p className="text-gray-500 dark:text-gray-400 text-sm mt-1">{container.image}</p>
            </div>
            <span className={cn(
              "px-3 py-1 text-sm font-medium rounded-full capitalize",
              getStatusBadge(container.status)
            )}>
              {container.status}
            </span>
          </div>

          {/* Actions */}
          <div className="flex items-center gap-2">
            {container.status === "running" ? (
              <>
                <button
                  onClick={() => setShowStopModal(true)}
                  disabled={stopMutation.isPending}
                  className="flex items-center gap-2 px-4 py-2 bg-gray-100 dark:bg-gray-800 text-gray-700 dark:text-gray-300 rounded-lg hover:bg-gray-200 dark:hover:bg-gray-700 disabled:opacity-50"
                >
                  <Square className="h-4 w-4" />
                  Stop
                </button>
                <button
                  onClick={() => setShowRestartModal(true)}
                  disabled={restartMutation.isPending}
                  className="flex items-center gap-2 px-4 py-2 bg-gray-100 dark:bg-gray-800 text-gray-700 dark:text-gray-300 rounded-lg hover:bg-gray-200 dark:hover:bg-gray-700 disabled:opacity-50"
                >
                  <RotateCcw className="h-4 w-4" />
                  Restart
                </button>
              </>
            ) : (
              <button
                onClick={() => startMutation.mutate()}
                disabled={startMutation.isPending}
                className="flex items-center gap-2 px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700 disabled:opacity-50"
              >
                <Play className="h-4 w-4" />
                Start
              </button>
            )}
            <button
              onClick={() => {
                setShowDeleteModal(true);
                setDeleteConfirmName("");
                setDeleteError(null);
                setForceDelete(container.status === "running");
              }}
              className="flex items-center gap-2 px-4 py-2 bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-400 rounded-lg hover:bg-red-200 dark:hover:bg-red-900/50"
            >
              <Trash2 className="h-4 w-4" />
              Delete
            </button>
          </div>
        </div>
      </div>

      {/* Tabs */}
      <div className="border-b border-gray-200 dark:border-gray-800 mb-6">
        <nav className="flex gap-6 overflow-x-auto">
          {tabs.map((tab) => (
            <button
              key={tab.id}
              onClick={() => setActiveTab(tab.id as TabId)}
              className={cn(
                "flex items-center gap-2 py-3 text-sm font-medium border-b-2 -mb-px whitespace-nowrap",
                activeTab === tab.id
                  ? "border-primary-500 text-primary-600 dark:text-primary-400"
                  : "border-transparent text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200"
              )}
            >
              <tab.icon className="h-4 w-4" />
              {tab.label}
            </button>
          ))}
        </nav>
      </div>

      {/* Tab Content */}
      {renderTabContent()}

      {/* Stop Confirmation Modal */}
      {showStopModal && container && (
        <div className="fixed inset-0 z-50 flex items-center justify-center">
          <div
            className="absolute inset-0 bg-black/50"
            onClick={() => setShowStopModal(false)}
          />
          <div className="relative bg-white dark:bg-gray-900 rounded-xl shadow-xl max-w-md w-full mx-4 p-6">
            <button
              onClick={() => setShowStopModal(false)}
              className="absolute top-4 right-4 text-gray-400 hover:text-gray-600"
            >
              <X className="h-5 w-5" />
            </button>

            <div className="flex items-center gap-3 mb-4">
              <div className="p-2 bg-yellow-100 dark:bg-yellow-900/30 rounded-lg">
                <Square className="h-6 w-6 text-yellow-600 dark:text-yellow-400" />
              </div>
              <div>
                <h3 className="text-lg font-semibold text-gray-900 dark:text-white">
                  Stop Container
                </h3>
                <p className="text-sm text-gray-500">
                  Are you sure you want to stop this container?
                </p>
              </div>
            </div>

            <div className="mb-4 p-3 bg-gray-100 dark:bg-gray-800 rounded-lg">
              <p className="text-sm text-gray-600 dark:text-gray-400 mb-1">Container</p>
              <code className="text-sm text-gray-900 dark:text-white font-mono">
                {container.name}
              </code>
            </div>

            <div className="flex justify-end gap-3">
              <button
                onClick={() => setShowStopModal(false)}
                className="px-4 py-2 text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-800 rounded-lg"
              >
                Cancel
              </button>
              <button
                onClick={() => {
                  stopMutation.mutate();
                  setShowStopModal(false);
                }}
                disabled={stopMutation.isPending}
                className="px-4 py-2 bg-yellow-600 text-white rounded-lg hover:bg-yellow-700 disabled:opacity-50 flex items-center gap-2"
              >
                {stopMutation.isPending ? (
                  <>
                    <div className="animate-spin rounded-full h-4 w-4 border-2 border-white border-t-transparent" />
                    Stopping...
                  </>
                ) : (
                  <>
                    <Square className="h-4 w-4" />
                    Stop Container
                  </>
                )}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Restart Confirmation Modal */}
      {showRestartModal && container && (
        <div className="fixed inset-0 z-50 flex items-center justify-center">
          <div
            className="absolute inset-0 bg-black/50"
            onClick={() => setShowRestartModal(false)}
          />
          <div className="relative bg-white dark:bg-gray-900 rounded-xl shadow-xl max-w-md w-full mx-4 p-6">
            <button
              onClick={() => setShowRestartModal(false)}
              className="absolute top-4 right-4 text-gray-400 hover:text-gray-600"
            >
              <X className="h-5 w-5" />
            </button>

            <div className="flex items-center gap-3 mb-4">
              <div className="p-2 bg-blue-100 dark:bg-blue-900/30 rounded-lg">
                <RotateCcw className="h-6 w-6 text-blue-600 dark:text-blue-400" />
              </div>
              <div>
                <h3 className="text-lg font-semibold text-gray-900 dark:text-white">
                  Restart Container
                </h3>
                <p className="text-sm text-gray-500">
                  Are you sure you want to restart this container?
                </p>
              </div>
            </div>

            <div className="mb-4 p-3 bg-gray-100 dark:bg-gray-800 rounded-lg">
              <p className="text-sm text-gray-600 dark:text-gray-400 mb-1">Container</p>
              <code className="text-sm text-gray-900 dark:text-white font-mono">
                {container.name}
              </code>
            </div>

            <p className="text-sm text-gray-500 dark:text-gray-400 mb-4">
              This will briefly interrupt any services running in this container.
            </p>

            <div className="flex justify-end gap-3">
              <button
                onClick={() => setShowRestartModal(false)}
                className="px-4 py-2 text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-800 rounded-lg"
              >
                Cancel
              </button>
              <button
                onClick={() => {
                  restartMutation.mutate();
                  setShowRestartModal(false);
                }}
                disabled={restartMutation.isPending}
                className="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50 flex items-center gap-2"
              >
                {restartMutation.isPending ? (
                  <>
                    <div className="animate-spin rounded-full h-4 w-4 border-2 border-white border-t-transparent" />
                    Restarting...
                  </>
                ) : (
                  <>
                    <RotateCcw className="h-4 w-4" />
                    Restart Container
                  </>
                )}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Delete Confirmation Modal */}
      {showDeleteModal && container && (
        <div className="fixed inset-0 z-50 flex items-center justify-center">
          <div
            className="absolute inset-0 bg-black/50"
            onClick={() => setShowDeleteModal(false)}
          />
          <div className="relative bg-white dark:bg-gray-900 rounded-xl shadow-xl max-w-md w-full mx-4 p-6">
            <button
              onClick={() => setShowDeleteModal(false)}
              className="absolute top-4 right-4 text-gray-400 hover:text-gray-600"
            >
              <X className="h-5 w-5" />
            </button>

            <div className="flex items-center gap-3 mb-4">
              <div className="p-2 bg-red-100 dark:bg-red-900/30 rounded-lg">
                <Trash2 className="h-6 w-6 text-red-600 dark:text-red-400" />
              </div>
              <div>
                <h3 className="text-lg font-semibold text-gray-900 dark:text-white">
                  Delete Container
                </h3>
                <p className="text-sm text-gray-500">
                  This action cannot be undone
                </p>
              </div>
            </div>

            <div className="mb-4 p-3 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg">
              <p className="text-sm text-red-700 dark:text-red-400">
                You are about to permanently delete the container{" "}
                <strong>{container.name}</strong>. This will remove the container
                but not its associated volumes.
              </p>
            </div>

            {deleteError && (
              <div className="mb-4 p-3 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg">
                <p className="text-sm text-red-700 dark:text-red-400 flex items-center gap-2">
                  <AlertTriangle className="h-4 w-4" />
                  {deleteError}
                </p>
              </div>
            )}

            <div className="mb-4">
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                Type <span className="font-mono text-red-600 dark:text-red-400">{container.name}</span> to confirm
              </label>
              <input
                type="text"
                value={deleteConfirmName}
                onChange={(e) => setDeleteConfirmName(e.target.value)}
                placeholder="Enter container name"
                className="w-full px-3 py-2 border border-gray-300 dark:border-gray-700 rounded-lg bg-white dark:bg-gray-800 text-gray-900 dark:text-white focus:ring-2 focus:ring-red-500 focus:border-transparent"
                autoFocus
              />
            </div>

            {container.status === "running" && (
              <div className="mb-4">
                <label className="flex items-center gap-2 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={forceDelete}
                    onChange={(e) => setForceDelete(e.target.checked)}
                    className="w-4 h-4 text-red-600 border-gray-300 rounded focus:ring-red-500"
                  />
                  <span className="text-sm text-gray-700 dark:text-gray-300">
                    Force stop running container before deleting
                  </span>
                </label>
              </div>
            )}

            <div className="flex justify-end gap-3">
              <button
                onClick={() => setShowDeleteModal(false)}
                className="px-4 py-2 text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-800 rounded-lg"
              >
                Cancel
              </button>
              <button
                onClick={handleDelete}
                disabled={deleteConfirmName !== container.name || deleteMutation.isPending}
                className="px-4 py-2 bg-red-600 text-white rounded-lg hover:bg-red-700 disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-2"
              >
                {deleteMutation.isPending ? (
                  <>
                    <div className="animate-spin rounded-full h-4 w-4 border-2 border-white border-t-transparent" />
                    Deleting...
                  </>
                ) : (
                  <>
                    <Trash2 className="h-4 w-4" />
                    Delete Container
                  </>
                )}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
