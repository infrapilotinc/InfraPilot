"use client";

import { useQuery } from "@tanstack/react-query";
import {
  Network,
  Server,
  Shield,
  CheckCircle2,
  ArrowRight,
  Globe,
  Lock,
  AlertTriangle,
  ShieldCheck,
  Layers,
  Activity,
} from "lucide-react";
import Link from "next/link";
import { api } from "@/lib/api";

// Component library imports
import { StatCard, MetricsGrid } from "@/components/ui/StatCard";
import { Badge } from "@/components/ui/Badge";
import { Spinner } from "@/components/ui/Spinner";
import { Card, CardBody } from "@/components/ui/Card";
import { cn } from "@/lib/utils";

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

export default function TrafficOverviewPage() {
  // Fetch agents
  const { data: agents } = useQuery({
    queryKey: ["agents"],
    queryFn: () => api.getAgents(),
  });

  const defaultAgentId = agents?.filter((a) => a.status === "active")[0]?.id;

  // Query traffic summary
  const { data: summary, isLoading: summaryLoading } = useQuery({
    queryKey: ["traffic-summary"],
    queryFn: () => api.getTrafficSummary(),
  });

  // Query resources
  const { data: resourcesData, isLoading: resourcesLoading } = useQuery({
    queryKey: ["traffic-resources"],
    queryFn: () => api.listTrafficResources({}),
  });

  // Query policies
  const { data: policiesData, isLoading: policiesLoading } = useQuery({
    queryKey: ["traffic-policies"],
    queryFn: () => api.listTrafficPolicies({}),
  });

  // Query proxies
  const { data: proxiesData, isLoading: proxiesLoading } = useQuery({
    queryKey: ["proxies", defaultAgentId],
    queryFn: () => api.getProxyHosts(defaultAgentId!),
    enabled: !!defaultAgentId,
  });

  const getStatusBadge = (status: string) => {
    switch (status) {
      case "active":
        return <Badge color="green">Active</Badge>;
      case "draft":
        return <Badge color="gray">Draft</Badge>;
      case "pending":
        return <Badge color="yellow">Pending</Badge>;
      case "disabled":
        return <Badge color="gray">Disabled</Badge>;
      case "error":
        return <Badge color="red">Error</Badge>;
      default:
        return <Badge>{status}</Badge>;
    }
  };

  const getGatewayBadge = (type: string) => {
    return type === "system" ? (
      <Badge color="purple">System</Badge>
    ) : (
      <Badge color="blue">Application</Badge>
    );
  };

  // Calculate proxy metrics
  const activeProxies = proxiesData?.filter((p: ProxyHost) => p.status === "active")?.length || 0;
  const sslEnabledProxies = proxiesData?.filter((p: ProxyHost) => p.ssl_enabled)?.length || 0;
  const expiringCerts = proxiesData?.filter((p: ProxyHost) => {
    if (!p.ssl_expires_at) return false;
    const expiresAt = new Date(p.ssl_expires_at);
    const thirtyDaysFromNow = new Date();
    thirtyDaysFromNow.setDate(thirtyDaysFromNow.getDate() + 30);
    return expiresAt <= thirtyDaysFromNow && expiresAt > new Date();
  })?.length || 0;

  const isLoading = summaryLoading || resourcesLoading || policiesLoading || proxiesLoading;

  if (isLoading) {
    return <Spinner.LogoPage label="Loading traffic overview..." />;
  }

  return (
    <div className="space-y-8">
      {/* Traffic Governance Section */}
      <div className="space-y-4">
        <div className="flex items-center gap-2">
          <Layers className="h-5 w-5 text-blue-600 dark:text-blue-400" />
          <h2 className="text-lg font-semibold text-gray-900 dark:text-white">Traffic Governance</h2>
          <span className="text-xs text-gray-500 dark:text-gray-400 ml-2">Advanced routing with load balancing & policies</span>
        </div>

        <MetricsGrid columns={4}>
          <StatCard
            label="Total Resources"
            value={summary?.total_resources || 0}
            icon={Network}
            iconColor="text-blue-600"
            onClick={() => window.location.href = "/traffic/resources"}
          />
          <StatCard
            label="Active Resources"
            value={summary?.active_resources || 0}
            icon={CheckCircle2}
            iconColor="text-green-600"
          />
          <StatCard
            label="System Gateways"
            value={summary?.system_gateways || 0}
            icon={Server}
            iconColor="text-purple-600"
          />
          <StatCard
            label="Policies"
            value={policiesData?.policies?.length || 0}
            icon={Shield}
            iconColor="text-indigo-600"
            onClick={() => window.location.href = "/traffic/policies"}
          />
        </MetricsGrid>
      </div>

      {/* Reverse Proxy Section */}
      <div className="space-y-4">
        <div className="flex items-center gap-2">
          <Globe className="h-5 w-5 text-orange-600 dark:text-orange-400" />
          <h2 className="text-lg font-semibold text-gray-900 dark:text-white">Reverse Proxy</h2>
          <span className="text-xs text-gray-500 dark:text-gray-400 ml-2">Simple domain-to-upstream mapping with SSL</span>
        </div>

        <MetricsGrid columns={4}>
          <StatCard
            label="Total Proxies"
            value={proxiesData?.length || 0}
            icon={Globe}
            iconColor="text-orange-600"
            onClick={() => window.location.href = "/traffic/proxies"}
          />
          <StatCard
            label="Active Proxies"
            value={activeProxies}
            icon={Activity}
            iconColor="text-green-600"
          />
          <StatCard
            label="SSL Enabled"
            value={sslEnabledProxies}
            icon={ShieldCheck}
            iconColor="text-emerald-600"
            description={proxiesData?.length ? `${Math.round((sslEnabledProxies / proxiesData.length) * 100)}% coverage` : undefined}
          />
          <StatCard
            label="Expiring Certs"
            value={expiringCerts}
            icon={AlertTriangle}
            iconColor={expiringCerts > 0 ? "text-yellow-600" : "text-gray-400"}
            description={expiringCerts > 0 ? "Within 30 days" : "All certificates healthy"}
            valueColor={expiringCerts > 0 ? "text-yellow-600" : undefined}
          />
        </MetricsGrid>
      </div>

      {/* Recent Resources & Proxies */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Recent Resources */}
        <Card>
          <div className="flex items-center justify-between px-6 py-4 border-b border-gray-200 dark:border-gray-800">
            <div className="flex items-center gap-2">
              <Network className="h-4 w-4 text-blue-600 dark:text-blue-400" />
              <h3 className="font-semibold text-gray-900 dark:text-white">Recent Resources</h3>
            </div>
            <Link
              href="/traffic/resources"
              className="text-sm text-primary-600 hover:text-primary-700 flex items-center gap-1"
            >
              View all
              <ArrowRight className="h-4 w-4" />
            </Link>
          </div>
          <CardBody noPadding>
            <div className="divide-y divide-gray-200 dark:divide-gray-800">
              {resourcesData?.resources && resourcesData.resources.length > 0 ? (
                resourcesData.resources.slice(0, 5).map((resource) => (
                  <Link
                    key={resource.id}
                    href={`/traffic/resources/${resource.id}`}
                    className="flex items-center justify-between px-6 py-4 hover:bg-gray-50 dark:hover:bg-gray-800/50 transition-colors"
                  >
                    <div className="flex items-center gap-3">
                      <div className="p-2 bg-gray-100 dark:bg-gray-800 rounded-lg">
                        <Network className="h-4 w-4 text-gray-600 dark:text-gray-400" />
                      </div>
                      <div>
                        <div className="font-medium text-gray-900 dark:text-white">{resource.name}</div>
                        <div className="text-sm text-gray-500 dark:text-gray-400">
                          {resource.domains?.slice(0, 2).join(", ")}
                        </div>
                      </div>
                    </div>
                    <div className="flex items-center gap-2">
                      {getGatewayBadge(resource.gateway_type)}
                      {getStatusBadge(resource.status)}
                    </div>
                  </Link>
                ))
              ) : (
                <div className="px-6 py-8 text-center">
                  <Network className="h-8 w-8 text-gray-400 mx-auto mb-2" />
                  <p className="text-sm text-gray-500 dark:text-gray-400">No traffic resources yet</p>
                  <Link
                    href="/traffic/resources/create"
                    className="inline-flex items-center gap-1 mt-3 text-sm text-primary-600 hover:text-primary-700"
                  >
                    Create your first resource
                    <ArrowRight className="h-3 w-3" />
                  </Link>
                </div>
              )}
            </div>
          </CardBody>
        </Card>

        {/* Recent Proxies */}
        <Card>
          <div className="flex items-center justify-between px-6 py-4 border-b border-gray-200 dark:border-gray-800">
            <div className="flex items-center gap-2">
              <Globe className="h-4 w-4 text-orange-600 dark:text-orange-400" />
              <h3 className="font-semibold text-gray-900 dark:text-white">Recent Proxies</h3>
            </div>
            <Link
              href="/traffic/proxies"
              className="text-sm text-primary-600 hover:text-primary-700 flex items-center gap-1"
            >
              View all
              <ArrowRight className="h-4 w-4" />
            </Link>
          </div>
          <CardBody noPadding>
            <div className="divide-y divide-gray-200 dark:divide-gray-800">
              {proxiesData && proxiesData.length > 0 ? (
                proxiesData.slice(0, 5).map((proxy: ProxyHost) => {
                  const isExpiringSoon = proxy.ssl_expires_at && (() => {
                    const expiresAt = new Date(proxy.ssl_expires_at!);
                    const thirtyDaysFromNow = new Date();
                    thirtyDaysFromNow.setDate(thirtyDaysFromNow.getDate() + 30);
                    return expiresAt <= thirtyDaysFromNow && expiresAt > new Date();
                  })();

                  return (
                    <div
                      key={proxy.id}
                      className="flex items-center justify-between px-6 py-4 hover:bg-gray-50 dark:hover:bg-gray-800/50 transition-colors cursor-pointer"
                    >
                      <div className="flex items-center gap-3">
                        <div className={cn(
                          "p-2 rounded-lg",
                          proxy.ssl_enabled
                            ? isExpiringSoon
                              ? "bg-yellow-100 dark:bg-yellow-900/30"
                              : "bg-green-100 dark:bg-green-900/30"
                            : "bg-gray-100 dark:bg-gray-800"
                        )}>
                          {proxy.ssl_enabled ? (
                            isExpiringSoon ? (
                              <AlertTriangle className="h-4 w-4 text-yellow-600 dark:text-yellow-400" />
                            ) : (
                              <Lock className="h-4 w-4 text-green-600 dark:text-green-400" />
                            )
                          ) : (
                            <Globe className="h-4 w-4 text-gray-600 dark:text-gray-400" />
                          )}
                        </div>
                        <div>
                          <div className="font-medium text-gray-900 dark:text-white">{proxy.domain}</div>
                          <div className="text-sm text-gray-500 dark:text-gray-400 truncate max-w-[200px]">
                            → {proxy.upstream_target}
                          </div>
                        </div>
                      </div>
                      <div className="flex items-center gap-2">
                        {proxy.ssl_enabled && (
                          isExpiringSoon ? (
                            <Badge color="yellow">SSL Expiring</Badge>
                          ) : (
                            <Badge color="green">SSL</Badge>
                          )
                        )}
                        {getStatusBadge(proxy.status)}
                      </div>
                    </div>
                  );
                })
              ) : (
                <div className="px-6 py-8 text-center">
                  <Globe className="h-8 w-8 text-gray-400 mx-auto mb-2" />
                  <p className="text-sm text-gray-500 dark:text-gray-400">No proxies configured yet</p>
                  <Link
                    href="/traffic/proxies"
                    className="inline-flex items-center gap-1 mt-3 text-sm text-primary-600 hover:text-primary-700"
                  >
                    Add your first proxy
                    <ArrowRight className="h-3 w-3" />
                  </Link>
                </div>
              )}
            </div>
          </CardBody>
        </Card>
      </div>

      {/* Quick Actions */}
      <Card>
        <CardBody>
          <div className="flex items-center justify-between">
            <div>
              <h3 className="font-semibold text-gray-900 dark:text-white">Quick Actions</h3>
              <p className="text-sm text-gray-500 dark:text-gray-400">Common traffic management tasks</p>
            </div>
            <div className="flex items-center gap-3">
              <Link
                href="/traffic/resources/create"
                className="inline-flex items-center gap-2 px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors text-sm font-medium"
              >
                <Network className="w-4 h-4" />
                New Resource
              </Link>
              <Link
                href="/traffic/policies/create"
                className="inline-flex items-center gap-2 px-4 py-2 bg-indigo-600 text-white rounded-lg hover:bg-indigo-700 transition-colors text-sm font-medium"
              >
                <Shield className="w-4 h-4" />
                New Policy
              </Link>
              <Link
                href="/traffic/proxies"
                className="inline-flex items-center gap-2 px-4 py-2 bg-orange-600 text-white rounded-lg hover:bg-orange-700 transition-colors text-sm font-medium"
              >
                <Globe className="w-4 h-4" />
                Add Proxy
              </Link>
            </div>
          </div>
        </CardBody>
      </Card>
    </div>
  );
}
