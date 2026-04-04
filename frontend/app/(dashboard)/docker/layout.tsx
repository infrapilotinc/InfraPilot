"use client";

import { ReactNode, useState } from "react";
import { usePathname, useRouter } from "next/navigation";
import { RefreshCw, Server, Download, Plus, Layers } from "lucide-react";

type Tab = {
  id: string;
  label: string;
  href: string;
  requiredFeature?: string;
  tierLabel?: string;
};
import { useQueryClient } from "@tanstack/react-query";
import { DockerProvider, useDocker } from "@/lib/docker-context";
import { PageHeader } from "@/components/ui/PageHeader";
import { Breadcrumb } from "@/components/ui/Breadcrumb";
import { EmptyState } from "@/components/ui/EmptyState";
import { Spinner } from "@/components/ui/Spinner";
import { ContainerPanel } from "@/components/docker/ContainerPanel";
import { DeploymentPanel } from "@/components/docker/DeploymentPanel";
import { StackDeployWizard } from "@/components/StackDeployWizard";
import { cn } from "@/lib/utils";

const tabGroups: { id: string; label: string; tabs: Tab[] }[] = [
  {
    id: "resources",
    label: "Resources",
    tabs: [
      { id: "overview",    label: "Overview",    href: "/docker" },
      { id: "containers",  label: "Containers",  href: "/docker/containers" },
      { id: "images",      label: "Images",      href: "/docker/images" },
      { id: "volumes",     label: "Volumes",     href: "/docker/volumes" },
      { id: "networks",    label: "Networks",    href: "/docker/networks" },
      { id: "logs",        label: "Logs",        href: "/docker/logs" },
    ],
  },
  {
    id: "management",
    label: "Management",
    tabs: [
      { id: "stacks",      label: "Stacks",      href: "/docker/stacks" },
      { id: "deployments", label: "Deployments", href: "/docker/deployments" },
    ],
  },
];

function DockerLayoutContent({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const queryClient = useQueryClient();
  const { activeAgents, selectedAgent, setSelectedAgent, isLoadingAgents } = useDocker();
  const [showStackDeployWizard, setShowStackDeployWizard] = useState(false);

  // Check if we're on a detail page
  const parts = pathname.split("/");
  const isDetailPage = parts.length > 3;

  // Determine active tab from pathname
  const getActiveTab = () => {
    if (pathname === "/docker") return "overview";
    const segment = pathname.split("/")[2];
    return segment || "overview";
  };

  const activeTab = getActiveTab();

  // Get action button based on active tab
  const getActionButton = () => {
    switch (activeTab) {
      case "stacks":
        return (
          <button
            onClick={() => setShowStackDeployWizard(true)}
            className="inline-flex items-center gap-2 px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700 transition-colors text-sm font-medium"
          >
            <Layers className="w-4 h-4" />
            Deploy Stack
          </button>
        );
      case "images":
        return (
          <button
            onClick={() => router.push("/docker/images?action=pull")}
            className="inline-flex items-center gap-2 px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700 transition-colors text-sm font-medium"
          >
            <Download className="w-4 h-4" />
            Pull Image
          </button>
        );
      case "volumes":
        return (
          <button
            onClick={() => router.push("/docker/volumes?action=create")}
            className="inline-flex items-center gap-2 px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700 transition-colors text-sm font-medium"
          >
            <Plus className="w-4 h-4" />
            Create Volume
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
          title="Docker"
          description="Manage containers, images, volumes, and networks"
          breadcrumbs={<Breadcrumb items={[{ label: "Infrastructure" }, { label: "Docker" }]} />}
        />
        <Spinner.LogoPage label="Loading agents..." />
      </div>
    );
  }

  if (activeAgents.length === 0) {
    return (
      <div className="space-y-6">
        <PageHeader
          title="Docker"
          description="Manage containers, images, volumes, and networks"
          breadcrumbs={<Breadcrumb items={[{ label: "Infrastructure" }, { label: "Docker" }]} />}
        />
        <EmptyState
          icon={Server}
          title="No agents available"
          description="Connect an agent to manage Docker resources"
          action={
            <button className="px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700 transition-colors">
              Connect Agent
            </button>
          }
        />
      </div>
    );
  }

  // For detail pages, just render children without the layout wrapper
  if (isDetailPage) {
    return (
      <>
        {children}
        <ContainerPanel />
        <DeploymentPanel />
        <StackDeployWizard
          isOpen={showStackDeployWizard}
          onClose={() => setShowStackDeployWizard(false)}
        />
      </>
    );
  }

  return (
    <div className="space-y-6">
      {/* Page Header */}
      <PageHeader
        title="Docker"
        description="Manage containers, images, volumes, and networks"
        breadcrumbs={<Breadcrumb items={[{ label: "Infrastructure" }, { label: "Docker" }]} />}
        action={
          <div className="flex items-center gap-3">
            {activeAgents.length > 1 && (
              <select
                value={selectedAgent || ""}
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
                  {group.tabs.map((tab) => (
                    <button
                      key={tab.id}
                      onClick={() => router.push(tab.href)}
                      className={cn(
                        "inline-flex items-center gap-1.5 whitespace-nowrap border-b-2 py-3 px-3 text-sm font-medium transition-colors",
                        activeTab === tab.id
                          ? "border-primary-500 text-primary-600 dark:text-primary-400"
                          : "border-transparent text-gray-500 hover:border-gray-300 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300"
                      )}
                    >
                      {tab.label}
                    </button>
                  ))}
                </div>
              </div>
            </div>
          ))}
        </nav>
      </div>

      {/* Page Content */}
      {children}

      {/* Global Container Panel */}
      <ContainerPanel />

      {/* Global Deployment Panel */}
      <DeploymentPanel />

      {/* Stack Deploy Wizard Modal */}
      <StackDeployWizard
        isOpen={showStackDeployWizard}
        onClose={() => setShowStackDeployWizard(false)}
      />
    </div>
  );
}

export default function DockerLayout({ children }: { children: ReactNode }) {
  return (
    <DockerProvider>
      <DockerLayoutContent>{children}</DockerLayoutContent>
    </DockerProvider>
  );
}
