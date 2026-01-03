"use client";

import { useQuery } from "@tanstack/react-query";
import {
  Server,
  Container,
  Globe,
  AlertTriangle,
  Activity,
  Cpu,
  MemoryStick,
  Clock,
  TrendingUp,
  CheckCircle,
  XCircle,
  RefreshCw,
} from "lucide-react";
import Link from "next/link";
import { api } from "@/lib/api";
import { cn, formatRelativeTime } from "@/lib/utils";

function StatCard({
  title,
  value,
  subValue,
  icon: Icon,
  color,
  href,
}: {
  title: string;
  value: number | string;
  subValue?: string;
  icon: React.ElementType;
  color: string;
  href?: string;
}) {
  const content = (
    <div className={cn(
      "bg-white dark:bg-gray-900 rounded-lg p-6 border border-gray-200 dark:border-gray-800 transition-all",
      href && "hover:border-gray-300 dark:hover:border-gray-700 hover:shadow-sm cursor-pointer"
    )}>
      <div className="flex items-center gap-4">
        <div className={`p-3 rounded-lg ${color}`}>
          <Icon className="h-6 w-6 text-white" />
        </div>
        <div>
          <p className="text-sm text-gray-500 dark:text-gray-400">{title}</p>
          <p className="text-2xl font-bold text-gray-900 dark:text-white">{value}</p>
          {subValue && (
            <p className="text-xs text-gray-400 dark:text-gray-500 mt-0.5">{subValue}</p>
          )}
        </div>
      </div>
    </div>
  );

  if (href) {
    return <Link href={href}>{content}</Link>;
  }
  return content;
}

function ContainerRow({
  name,
  image,
  status,
  cpu,
  memory,
}: {
  name: string;
  image: string;
  status: string;
  cpu: number;
  memory: number;
}) {
  return (
    <div className="flex items-center justify-between py-3 border-b border-gray-100 dark:border-gray-800 last:border-0">
      <div className="flex items-center gap-3 min-w-0">
        <div className={cn(
          "w-2 h-2 rounded-full flex-shrink-0",
          status === "running" ? "bg-green-500" : "bg-red-500"
        )} />
        <div className="min-w-0">
          <p className="text-sm font-medium text-gray-900 dark:text-white truncate">{name}</p>
          <p className="text-xs text-gray-500 truncate">{image}</p>
        </div>
      </div>
      <div className="flex items-center gap-4 text-xs text-gray-500">
        <span className="flex items-center gap-1">
          <Cpu className="h-3 w-3" />
          {cpu.toFixed(1)}%
        </span>
        <span className="flex items-center gap-1">
          <MemoryStick className="h-3 w-3" />
          {memory}MB
        </span>
      </div>
    </div>
  );
}

function ProxyRow({
  domain,
  upstream,
  sslEnabled,
  status,
}: {
  domain: string;
  upstream: string;
  sslEnabled: boolean;
  status: string;
}) {
  return (
    <div className="flex items-center justify-between py-3 border-b border-gray-100 dark:border-gray-800 last:border-0">
      <div className="min-w-0">
        <div className="flex items-center gap-2">
          <p className="text-sm font-medium text-gray-900 dark:text-white truncate">{domain}</p>
          {sslEnabled ? (
            <CheckCircle className="h-3.5 w-3.5 text-green-500 flex-shrink-0" />
          ) : (
            <XCircle className="h-3.5 w-3.5 text-yellow-500 flex-shrink-0" />
          )}
        </div>
        <p className="text-xs text-gray-500 truncate">{upstream}</p>
      </div>
      <span className={cn(
        "px-2 py-0.5 text-xs rounded-full font-medium",
        status === "active"
          ? "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400"
          : "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400"
      )}>
        {status}
      </span>
    </div>
  );
}

export default function DashboardPage() {
  // Fetch agents
  const { data: agents } = useQuery({
    queryKey: ["agents"],
    queryFn: () => api.getAgents(),
  });

  const activeAgent = agents?.find((a) => a.status === "active");
  const activeAgents = agents?.filter((a) => a.status === "active").length || 0;
  const totalAgents = agents?.length || 0;

  // Fetch containers for active agent
  const { data: containers, isLoading: containersLoading } = useQuery({
    queryKey: ["containers", activeAgent?.id],
    queryFn: () => activeAgent ? api.getContainers(activeAgent.id) : Promise.resolve([]),
    enabled: !!activeAgent,
    refetchInterval: 10000,
  });

  // Fetch proxies for active agent
  const { data: proxies, isLoading: proxiesLoading } = useQuery({
    queryKey: ["proxies", activeAgent?.id],
    queryFn: () => activeAgent ? api.getProxyHosts(activeAgent.id) : Promise.resolve([]),
    enabled: !!activeAgent,
  });

  // Fetch alert history
  const { data: alertHistory } = useQuery({
    queryKey: ["alertHistory"],
    queryFn: () => api.getAlertHistory(10),
  });

  const runningContainers = containers?.filter((c) => c.status === "running").length || 0;
  const totalContainers = containers?.length || 0;
  const activeProxies = proxies?.filter((p: { status: string }) => p.status === "active").length || 0;
  const totalProxies = proxies?.length || 0;
  const activeAlerts = alertHistory?.filter((a) => !a.resolved_at).length || 0;

  // Calculate total resources
  const totalCpu = containers?.reduce((sum, c) => sum + (c.cpu_percent || 0), 0) || 0;
  const totalMemory = containers?.reduce((sum, c) => sum + (c.memory_mb || 0), 0) || 0;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-gray-900 dark:text-white">Overview</h1>
        <div className="flex items-center gap-2 text-sm text-gray-500">
          <Activity className="h-4 w-4" />
          <span>Auto-refresh every 10s</span>
        </div>
      </div>

      {/* Stats Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
        <StatCard
          title="Containers"
          value={`${runningContainers}/${totalContainers}`}
          subValue={runningContainers === totalContainers ? "All running" : `${totalContainers - runningContainers} stopped`}
          icon={Container}
          color="bg-blue-600"
          href="/containers"
        />
        <StatCard
          title="Proxy Hosts"
          value={`${activeProxies}/${totalProxies}`}
          subValue={totalProxies > 0 ? `${proxies?.filter((p: { ssl_enabled: boolean }) => p.ssl_enabled).length || 0} with SSL` : "No proxies configured"}
          icon={Globe}
          color="bg-purple-600"
          href="/proxies"
        />
        <StatCard
          title="Agents"
          value={`${activeAgents}/${totalAgents}`}
          subValue={activeAgents === totalAgents ? "All online" : `${totalAgents - activeAgents} offline`}
          icon={Server}
          color="bg-green-600"
        />
        <StatCard
          title="Active Alerts"
          value={activeAlerts}
          subValue={activeAlerts === 0 ? "No issues" : "Attention needed"}
          icon={AlertTriangle}
          color={activeAlerts > 0 ? "bg-red-600" : "bg-yellow-600"}
          href="/alerts"
        />
      </div>

      {/* Resource Usage */}
      {containers && containers.length > 0 && (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div className="bg-white dark:bg-gray-900 rounded-lg p-4 border border-gray-200 dark:border-gray-800">
            <div className="flex items-center justify-between mb-2">
              <span className="text-sm text-gray-500">Total CPU Usage</span>
              <span className="text-lg font-semibold text-gray-900 dark:text-white">{totalCpu.toFixed(1)}%</span>
            </div>
            <div className="h-2 bg-gray-200 dark:bg-gray-700 rounded-full overflow-hidden">
              <div
                className="h-full bg-blue-500 rounded-full transition-all"
                style={{ width: `${Math.min(totalCpu, 100)}%` }}
              />
            </div>
          </div>
          <div className="bg-white dark:bg-gray-900 rounded-lg p-4 border border-gray-200 dark:border-gray-800">
            <div className="flex items-center justify-between mb-2">
              <span className="text-sm text-gray-500">Total Memory Usage</span>
              <span className="text-lg font-semibold text-gray-900 dark:text-white">{totalMemory.toFixed(0)} MB</span>
            </div>
            <div className="h-2 bg-gray-200 dark:bg-gray-700 rounded-full overflow-hidden">
              <div
                className="h-full bg-purple-500 rounded-full transition-all"
                style={{ width: `${Math.min(totalMemory / 40, 100)}%` }}
              />
            </div>
          </div>
        </div>
      )}

      {/* Two Column Layout */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Containers */}
        <div className="bg-white dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-800">
          <div className="flex items-center justify-between p-4 border-b border-gray-200 dark:border-gray-800">
            <h2 className="text-lg font-semibold text-gray-900 dark:text-white">
              Containers
            </h2>
            <Link href="/containers" className="text-sm text-primary-600 hover:text-primary-500">
              View all
            </Link>
          </div>
          <div className="p-4">
            {containersLoading ? (
              <div className="flex items-center justify-center py-8">
                <RefreshCw className="h-5 w-5 text-gray-400 animate-spin" />
              </div>
            ) : containers && containers.length > 0 ? (
              <div>
                {containers.slice(0, 5).map((container) => (
                  <ContainerRow
                    key={container.container_id}
                    name={container.name}
                    image={container.image}
                    status={container.status}
                    cpu={container.cpu_percent || 0}
                    memory={container.memory_mb || 0}
                  />
                ))}
                {containers.length > 5 && (
                  <p className="text-xs text-gray-500 mt-3 text-center">
                    +{containers.length - 5} more containers
                  </p>
                )}
              </div>
            ) : (
              <p className="text-gray-500 text-sm text-center py-8">No containers found</p>
            )}
          </div>
        </div>

        {/* Proxy Hosts */}
        <div className="bg-white dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-800">
          <div className="flex items-center justify-between p-4 border-b border-gray-200 dark:border-gray-800">
            <h2 className="text-lg font-semibold text-gray-900 dark:text-white">
              Proxy Hosts
            </h2>
            <Link href="/proxies" className="text-sm text-primary-600 hover:text-primary-500">
              View all
            </Link>
          </div>
          <div className="p-4">
            {proxiesLoading ? (
              <div className="flex items-center justify-center py-8">
                <RefreshCw className="h-5 w-5 text-gray-400 animate-spin" />
              </div>
            ) : proxies && proxies.length > 0 ? (
              <div>
                {proxies.slice(0, 5).map((proxy: { id: string; domain: string; upstream_target: string; ssl_enabled: boolean; status: string }) => (
                  <ProxyRow
                    key={proxy.id}
                    domain={proxy.domain}
                    upstream={proxy.upstream_target}
                    sslEnabled={proxy.ssl_enabled}
                    status={proxy.status}
                  />
                ))}
                {proxies.length > 5 && (
                  <p className="text-xs text-gray-500 mt-3 text-center">
                    +{proxies.length - 5} more proxies
                  </p>
                )}
              </div>
            ) : (
              <p className="text-gray-500 text-sm text-center py-8">No proxy hosts configured</p>
            )}
          </div>
        </div>
      </div>

      {/* Recent Alerts */}
      {alertHistory && alertHistory.length > 0 && (
        <div className="bg-white dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-800">
          <div className="flex items-center justify-between p-4 border-b border-gray-200 dark:border-gray-800">
            <h2 className="text-lg font-semibold text-gray-900 dark:text-white">
              Recent Alerts
            </h2>
            <Link href="/alerts" className="text-sm text-primary-600 hover:text-primary-500">
              View all
            </Link>
          </div>
          <div className="p-4">
            {alertHistory.slice(0, 5).map((alert) => (
              <div key={alert.id} className="flex items-center justify-between py-3 border-b border-gray-100 dark:border-gray-800 last:border-0">
                <div className="flex items-center gap-3">
                  <div className={cn(
                    "p-1.5 rounded",
                    alert.resolved_at
                      ? "bg-green-100 dark:bg-green-900/30"
                      : "bg-red-100 dark:bg-red-900/30"
                  )}>
                    {alert.resolved_at ? (
                      <CheckCircle className="h-4 w-4 text-green-600 dark:text-green-400" />
                    ) : (
                      <AlertTriangle className="h-4 w-4 text-red-600 dark:text-red-400" />
                    )}
                  </div>
                  <div>
                    <p className="text-sm text-gray-900 dark:text-white">{alert.message}</p>
                    <p className="text-xs text-gray-500">{formatRelativeTime(alert.triggered_at)}</p>
                  </div>
                </div>
                <span className={cn(
                  "px-2 py-0.5 text-xs rounded font-medium",
                  alert.severity === "critical"
                    ? "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400"
                    : alert.severity === "warning"
                    ? "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400"
                    : "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400"
                )}>
                  {alert.severity}
                </span>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
