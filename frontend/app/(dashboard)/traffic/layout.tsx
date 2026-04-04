"use client";

import { ReactNode, useState } from "react";
import { usePathname, useRouter } from "next/navigation";
import {
  RefreshCw,
  Plus,
  Globe,
  Shield,
  Network,
  FileText,
  Activity,
  Settings,
  Layers,
  BarChart3,
} from "lucide-react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { PageHeader } from "@/components/ui/PageHeader";
import { Breadcrumb } from "@/components/ui/Breadcrumb";
import { Spinner } from "@/components/ui/Spinner";
import { cn } from "@/lib/utils";
import { TrafficProvider } from "@/lib/traffic-context";
import { ProxyPanel } from "@/components/traffic/ProxyPanel";

// Tab groups for visual separation
const tabGroups = [
  {
    id: "governance",
    label: "Traffic Governance",
    tabs: [
      { id: "overview", label: "Overview", href: "/traffic", icon: Layers },
      { id: "resources", label: "Resources", href: "/traffic/resources", icon: Network },
      { id: "policies", label: "Policies", href: "/traffic/policies", icon: Shield },
    ],
  },
  {
    id: "proxies",
    label: "Reverse Proxy",
    tabs: [
      { id: "proxies", label: "Proxy Hosts", href: "/traffic/proxies", icon: Globe },
      { id: "analytics", label: "Analytics", href: "/traffic/analytics", icon: BarChart3 },
      { id: "logs", label: "Nginx Logs", href: "/traffic/logs", icon: FileText },
    ],
  },
  {
    id: "exposure",
    label: "Exposure",
    tabs: [
      { id: "exposure", label: "Exposure", href: "/traffic/exposure", icon: Activity },
      { id: "endpoints", label: "Endpoints", href: "/traffic/exposure/endpoints", icon: Globe },
      { id: "ratelimits", label: "Rate Limits", href: "/traffic/exposure/ratelimits", icon: Settings },
      { id: "tls", label: "TLS Posture", href: "/traffic/exposure/tls", icon: Shield },
    ],
  },
];


export default function TrafficLayout({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const queryClient = useQueryClient();

  // Fetch agents for agent selector
  const { data: agents, isLoading: isLoadingAgents } = useQuery({
    queryKey: ["agents"],
    queryFn: () => api.getAgents(),
  });

  const [selectedAgent, setSelectedAgent] = useState<string | null>(null);

  // Set default agent
  const activeAgents = agents?.filter((a) => a.status === "active") || [];
  const effectiveAgent = selectedAgent || activeAgents[0]?.id;

  // Check if we're on a detail or create page (e.g., /traffic/resources/[id] or /traffic/resources/create)
  // Exposure sub-tabs (/traffic/exposure/endpoints etc.) must NOT be treated as sub-pages
  const pathParts = pathname.split("/").filter(Boolean);
  const knownSegments = ["resources", "policies", "proxies", "logs", "analytics", "exposure"];
  const isSubPage = pathParts.length > 2 && pathParts[1] !== "exposure" && !knownSegments.includes(pathParts[2]);

  // Determine active tab from pathname
  const getActiveTab = () => {
    if (pathname === "/traffic") return "overview";
    if (pathname === "/traffic/proxies" || pathname.startsWith("/traffic/proxies/")) return "proxies";
    if (pathname === "/traffic/analytics" || pathname.startsWith("/traffic/analytics/")) return "analytics";
    if (pathname === "/traffic/logs" || pathname.startsWith("/traffic/logs/")) return "logs";
    if (pathname === "/traffic/exposure") return "exposure";
    if (pathname === "/traffic/exposure/endpoints") return "endpoints";
    if (pathname === "/traffic/exposure/ratelimits") return "ratelimits";
    if (pathname === "/traffic/exposure/tls") return "tls";
    const segment = pathname.split("/")[2];
    return segment || "overview";
  };

  const activeTab = getActiveTab();

  // Check which group the active tab belongs to
  const getActiveGroup = () => {
    for (const group of tabGroups) {
      if (group.tabs.some((t) => t.id === activeTab)) {
        return group.id;
      }
    }
    return "governance";
  };

  // Get action button based on active tab
  const getActionButton = () => {
    switch (activeTab) {
      case "resources":
        return (
          <button
            onClick={() => router.push("/traffic/resources/create")}
            className="inline-flex items-center gap-2 px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700 transition-colors text-sm font-medium"
          >
            <Plus className="w-4 h-4" />
            New Resource
          </button>
        );
      case "policies":
        return (
          <button
            onClick={() => router.push("/traffic/policies/create")}
            className="inline-flex items-center gap-2 px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700 transition-colors text-sm font-medium"
          >
            <Plus className="w-4 h-4" />
            New Policy
          </button>
        );
      case "proxies":
        return (
          <button
            onClick={() => router.push("/traffic/proxies/create")}
            className="inline-flex items-center gap-2 px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700 transition-colors text-sm font-medium"
          >
            <Plus className="w-4 h-4" />
            Add Proxy
          </button>
        );
      default:
        return null;
    }
  };

  if (isLoadingAgents) {
    return (
      <div className="space-y-6">
        <PageHeader
          title="Traffic"
          description="Manage traffic routing, policies, and nginx configuration"
          breadcrumbs={<Breadcrumb items={[{ label: "Overview" }, { label: "Traffic" }]} />}
        />
        <Spinner.LogoPage label="Loading..." />
      </div>
    );
  }

  // For detail/create pages, just render children without the layout wrapper
  if (isSubPage) {
    return (
      <TrafficProvider>
        {children}
        <ProxyPanel />
      </TrafficProvider>
    );
  }

  return (
    <TrafficProvider>
    <div className="space-y-6">
      {/* Page Header */}
      <PageHeader
        title="Traffic"
        description="Manage traffic routing, policies, and nginx configuration"
        breadcrumbs={<Breadcrumb items={[{ label: "Overview" }, { label: "Traffic" }]} />}
        action={
          <div className="flex items-center gap-3">
            {activeAgents.length > 1 && (
              <select
                value={effectiveAgent || ""}
                onChange={(e) => setSelectedAgent(e.target.value)}
                className="rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-800 px-4 py-2 text-sm text-gray-900 dark:text-white focus:ring-2 focus:ring-primary-500 focus:border-transparent"
              >
                {activeAgents.map((agent) => (
                  <option key={agent.id} value={agent.id}>
                    {agent.name} ({agent.hostname || "unknown host"})
                  </option>
                ))}
              </select>
            )}
            {getActionButton()}
            <button
              onClick={() => queryClient.invalidateQueries()}
              className="inline-flex items-center gap-2 px-4 py-2 bg-gray-100 dark:bg-gray-800 hover:bg-gray-200 dark:hover:bg-gray-700 text-gray-900 dark:text-white rounded-lg transition-colors text-sm font-medium"
            >
              <RefreshCw className="w-4 h-4" />
              Refresh
            </button>
          </div>
        }
      />

      {/* Navigation Tabs — grouped with separators */}
      <div className="border-b border-gray-200 dark:border-gray-700">
        <nav className="-mb-px flex items-end">
          {tabGroups.map((group, groupIndex) => (
            <div key={group.id} className="flex items-end">
              {groupIndex > 0 && (
                <div className="h-5 w-px bg-gray-200 dark:bg-gray-700 mx-2 mb-3 self-end" />
              )}
              <div className="flex flex-col">
                <span className="text-[10px] font-semibold uppercase tracking-wider text-gray-400 dark:text-gray-500 px-3 pb-1">
                  {group.label}
                </span>
                <div className="flex">
                  {group.tabs.map((tab) => {
                    const Icon = tab.icon;
                    return (
                      <button
                        key={tab.id}
                        onClick={() => router.push(tab.href)}
                        className={cn(
                          "flex items-center gap-1.5 whitespace-nowrap border-b-2 py-3 px-3 text-sm font-medium transition-colors",
                          activeTab === tab.id
                            ? "border-primary-500 text-primary-600 dark:text-primary-400"
                            : "border-transparent text-gray-500 hover:border-gray-300 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300"
                        )}
                      >
                        <Icon className="w-4 h-4" />
                        <span>{tab.label}</span>
                      </button>
                    );
                  })}
                </div>
              </div>
            </div>
          ))}
        </nav>
      </div>

      {/* Page Content */}
      {children}
    </div>

    {/* Proxy Panel - SlideOver for proxy details */}
    <ProxyPanel />
    </TrafficProvider>
  );
}
