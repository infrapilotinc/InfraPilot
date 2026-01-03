"use client";

import { useState, useEffect } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Container as ContainerIcon,
  Play,
  Square,
  RotateCcw,
  Server,
  FileText,
  RefreshCw,
  Clock,
  Layers,
  Globe,
  Copy,
  Terminal as TerminalIcon,
  ChevronRight,
  Cpu,
  MemoryStick,
  Check,
} from "lucide-react";
import { api, Container } from "@/lib/api";
import { Terminal } from "@/components/containers/Terminal";
import { cn } from "@/lib/utils";
import {
  PageLayout,
  ListCard,
  EmptyState,
  Button,
  Tabs,
} from "@/components/ui/page-layout";
import {
  DetailPanel,
  DetailSection,
  DetailRow,
} from "@/components/ui/detail-panel";

type PanelTab = "details" | "logs" | "terminal";

export default function ContainersPage() {
  const queryClient = useQueryClient();
  const [selectedAgent, setSelectedAgent] = useState<string | null>(null);
  const [selectedContainer, setSelectedContainer] = useState<Container | null>(null);
  const [panelTab, setPanelTab] = useState<PanelTab>("details");
  const [viewMode, setViewMode] = useState<"all" | "stacks">("all");
  const [logsTail, setLogsTail] = useState(100);
  const [copied, setCopied] = useState(false);

  // Fetch agents
  const { data: agents } = useQuery({
    queryKey: ["agents"],
    queryFn: () => api.getAgents(),
  });

  // Fetch containers for selected agent
  const { data: containers, isLoading } = useQuery({
    queryKey: ["containers", selectedAgent],
    queryFn: () =>
      selectedAgent ? api.getContainers(selectedAgent) : Promise.resolve([]),
    enabled: !!selectedAgent,
  });

  // Fetch logs for selected container
  const { data: logsData, isLoading: logsLoading, refetch: refetchLogs } = useQuery({
    queryKey: ["containerLogs", selectedAgent, selectedContainer?.container_id, logsTail],
    queryFn: () =>
      selectedAgent && selectedContainer
        ? api.getContainerLogs(selectedAgent, selectedContainer.container_id, logsTail)
        : Promise.resolve(null),
    enabled: !!selectedAgent && !!selectedContainer && panelTab === "logs",
  });

  // Container actions
  const startMutation = useMutation({
    mutationFn: ({ containerId }: { containerId: string }) =>
      api.startContainer(selectedAgent!, containerId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["containers", selectedAgent] });
    },
  });

  const stopMutation = useMutation({
    mutationFn: ({ containerId }: { containerId: string }) =>
      api.stopContainer(selectedAgent!, containerId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["containers", selectedAgent] });
    },
  });

  const restartMutation = useMutation({
    mutationFn: ({ containerId }: { containerId: string }) =>
      api.restartContainer(selectedAgent!, containerId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["containers", selectedAgent] });
    },
  });

  const activeAgents = agents?.filter((a) => a.status === "active") || [];

  // Auto-select first active agent
  useEffect(() => {
    if (!selectedAgent && activeAgents.length > 0) {
      setSelectedAgent(activeAgents[0].id);
    }
  }, [activeAgents, selectedAgent]);

  // Update selected container from fresh data
  useEffect(() => {
    if (selectedContainer && containers) {
      const updated = containers.find(c => c.container_id === selectedContainer.container_id);
      if (updated) {
        setSelectedContainer(updated);
      }
    }
  }, [containers, selectedContainer]);

  // Group containers by stack
  const containersByStack = containers?.reduce((acc, container) => {
    const stack = container.stack_name || "Standalone";
    if (!acc[stack]) acc[stack] = [];
    acc[stack].push(container);
    return acc;
  }, {} as Record<string, Container[]>) || {};

  const stackNames = Object.keys(containersByStack).sort((a, b) => {
    if (a === "Standalone") return 1;
    if (b === "Standalone") return -1;
    return a.localeCompare(b);
  });

  const getStatusColor = (status: string) => {
    if (status === "running") return "bg-green-500";
    if (status === "exited") return "bg-red-500";
    if (status === "paused") return "bg-yellow-500";
    return "bg-gray-500";
  };

  const getStatusBadgeClass = (status: string) => {
    switch (status) {
      case "running":
        return "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400";
      case "exited":
        return "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400";
      case "paused":
        return "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400";
      default:
        return "bg-gray-100 text-gray-700 dark:bg-gray-800 dark:text-gray-400";
    }
  };

  const handleCopyId = () => {
    if (selectedContainer) {
      navigator.clipboard.writeText(selectedContainer.container_id);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }
  };

  const handleContainerAction = (action: "start" | "stop" | "restart", container: Container) => {
    switch (action) {
      case "start":
        startMutation.mutate({ containerId: container.container_id });
        break;
      case "stop":
        stopMutation.mutate({ containerId: container.container_id });
        break;
      case "restart":
        restartMutation.mutate({ containerId: container.container_id });
        break;
    }
  };

  // Render container list item
  const renderContainerItem = (container: Container) => {
    const isNginx = container.image.includes("nginx") || container.name.toLowerCase().includes("nginx");
    const isSelected = selectedContainer?.container_id === container.container_id;

    return (
      <ListCard
        key={container.container_id}
        selected={isSelected}
        onClick={() => {
          setSelectedContainer(container);
          setPanelTab("details");
        }}
        className="group"
      >
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3 min-w-0">
            <div className={cn("w-2.5 h-2.5 rounded-full flex-shrink-0", getStatusColor(container.status))} />
            <div className="min-w-0">
              <div className="flex items-center gap-2">
                <span className="font-medium text-gray-900 dark:text-white truncate">
                  {container.name}
                </span>
                {isNginx && (
                  <span className="px-1.5 py-0.5 text-xs bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400 rounded flex-shrink-0">
                    nginx
                  </span>
                )}
              </div>
              <p className="text-xs text-gray-500 dark:text-gray-400 truncate mt-0.5">
                {container.image}
              </p>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <span className={cn(
              "px-2 py-0.5 text-xs font-medium rounded-full capitalize",
              getStatusBadgeClass(container.status)
            )}>
              {container.status}
            </span>
            <ChevronRight className={cn(
              "h-4 w-4 text-gray-400 transition-transform",
              isSelected && "text-primary-500"
            )} />
          </div>
        </div>

        {/* Quick metrics */}
        <div className="flex items-center gap-4 mt-3 text-xs text-gray-500 dark:text-gray-400">
          <div className="flex items-center gap-1">
            <Cpu className="h-3 w-3" />
            <span>{container.cpu_percent?.toFixed(1) || 0}%</span>
          </div>
          <div className="flex items-center gap-1">
            <MemoryStick className="h-3 w-3" />
            <span>{container.memory_mb || 0} MB</span>
          </div>
          {container.stack_name && (
            <div className="flex items-center gap-1">
              <Layers className="h-3 w-3" />
              <span className="truncate">{container.stack_name}</span>
            </div>
          )}
        </div>
      </ListCard>
    );
  };

  // Panel content based on active tab
  const renderPanelContent = () => {
    if (!selectedContainer) return null;

    switch (panelTab) {
      case "logs":
        const parseLogLine = (line: string) => {
          // Try to parse timestamp at the beginning
          const timestampMatch = line.match(/^(\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:?\d{2})?)\s*/);
          let timestamp = "";
          let message = line;

          if (timestampMatch) {
            timestamp = timestampMatch[1];
            message = line.slice(timestampMatch[0].length);
          }

          // Detect log level/type
          const lowerMessage = message.toLowerCase();
          let level: "error" | "warn" | "info" | "debug" = "info";
          if (lowerMessage.includes("error") || lowerMessage.includes("err ") || lowerMessage.includes("fatal") || lowerMessage.includes("panic")) {
            level = "error";
          } else if (lowerMessage.includes("warn") || lowerMessage.includes("warning")) {
            level = "warn";
          } else if (lowerMessage.includes("debug") || lowerMessage.includes("trace")) {
            level = "debug";
          }

          return { timestamp, message, level };
        };

        const logLines = logsData?.logs?.split("\n").filter((l: string) => l.trim()) || [];

        const getLevelColor = (level: string) => {
          switch (level) {
            case "error": return "text-red-400";
            case "warn": return "text-yellow-400";
            case "debug": return "text-gray-500";
            default: return "text-gray-300";
          }
        };

        const getLevelBg = (level: string) => {
          switch (level) {
            case "error": return "bg-red-500/10 border-l-2 border-red-500";
            case "warn": return "bg-yellow-500/10 border-l-2 border-yellow-500";
            default: return "";
          }
        };

        return (
          <div className="flex flex-col -mx-4 -mt-4 h-[calc(100vh-200px)] max-h-[600px]">
            <div className="flex items-center justify-between px-4 py-2 bg-gray-800 border-b border-gray-700 flex-shrink-0">
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
                  className="p-1.5 hover:bg-gray-700 rounded transition-colors"
                  title="Refresh logs"
                >
                  <RefreshCw className={cn("h-3.5 w-3.5 text-gray-400", logsLoading && "animate-spin")} />
                </button>
              </div>
            </div>
            <div className="flex-1 bg-gray-900 overflow-y-auto min-h-0">
              {logsLoading ? (
                <div className="flex items-center justify-center h-32">
                  <div className="animate-spin rounded-full h-5 w-5 border-b-2 border-primary-500" />
                </div>
              ) : logLines.length > 0 ? (
                <div className="font-mono text-xs">
                  {logLines.map((line: string, idx: number) => {
                    const { timestamp, message, level } = parseLogLine(line);
                    return (
                      <div
                        key={idx}
                        className={cn(
                          "flex gap-2 px-3 py-0.5 hover:bg-gray-800/50",
                          getLevelBg(level)
                        )}
                      >
                        {timestamp && (
                          <span className="text-gray-600 flex-shrink-0 w-20 truncate" title={timestamp}>
                            {new Date(timestamp).toLocaleTimeString()}
                          </span>
                        )}
                        <span className={cn("break-all", getLevelColor(level))}>
                          {message}
                        </span>
                      </div>
                    );
                  })}
                </div>
              ) : (
                <div className="text-center text-gray-500 py-8 text-sm">
                  No logs available
                </div>
              )}
            </div>
          </div>
        );

      case "terminal":
        if (selectedContainer.status !== "running") {
          return (
            <div className="flex flex-col items-center justify-center h-64 text-gray-500">
              <TerminalIcon className="h-8 w-8 mb-2 opacity-50" />
              <p className="text-sm">Container must be running to access terminal</p>
            </div>
          );
        }
        return (
          <div className="h-[500px] bg-gray-900 rounded-lg overflow-hidden">
            <Terminal
              containerId={selectedContainer.container_id}
              agentId={selectedAgent!}
              onClose={() => setPanelTab("details")}
            />
          </div>
        );

      default:
        return (
          <>
            {/* Quick Actions */}
            <DetailSection title="Actions">
              <div className="flex flex-wrap gap-2">
                {selectedContainer.status === "running" ? (
                  <>
                    <Button
                      variant="secondary"
                      size="sm"
                      icon={Square}
                      onClick={() => handleContainerAction("stop", selectedContainer)}
                      disabled={stopMutation.isPending}
                    >
                      Stop
                    </Button>
                    <Button
                      variant="secondary"
                      size="sm"
                      icon={RotateCcw}
                      onClick={() => handleContainerAction("restart", selectedContainer)}
                      disabled={restartMutation.isPending}
                    >
                      Restart
                    </Button>
                  </>
                ) : (
                  <Button
                    variant="primary"
                    size="sm"
                    icon={Play}
                    onClick={() => handleContainerAction("start", selectedContainer)}
                    disabled={startMutation.isPending}
                  >
                    Start
                  </Button>
                )}
              </div>
            </DetailSection>

            {/* Container Info */}
            <DetailSection title="Container Info">
              <DetailRow label="Status">
                <span className={cn(
                  "px-2 py-0.5 text-xs font-medium rounded-full capitalize",
                  getStatusBadgeClass(selectedContainer.status)
                )}>
                  {selectedContainer.status}
                </span>
              </DetailRow>
              <DetailRow label="Container ID" mono>
                <div className="flex items-center gap-1">
                  <span>{selectedContainer.container_id.slice(0, 12)}</span>
                  <button
                    onClick={handleCopyId}
                    className="p-1 hover:bg-gray-200 dark:hover:bg-gray-700 rounded"
                    title="Copy full ID"
                  >
                    {copied ? (
                      <Check className="h-3 w-3 text-green-500" />
                    ) : (
                      <Copy className="h-3 w-3 text-gray-400" />
                    )}
                  </button>
                </div>
              </DetailRow>
              <DetailRow label="Stack" value={selectedContainer.stack_name || "Standalone"} />
              {selectedContainer.created_at && (
                <DetailRow
                  label="Created"
                  value={new Date(selectedContainer.created_at).toLocaleString()}
                />
              )}
            </DetailSection>

            {/* Image */}
            <DetailSection title="Image">
              <div className="bg-gray-100 dark:bg-gray-800/50 rounded-lg p-3">
                <code className="text-sm text-gray-900 dark:text-white font-mono break-all">
                  {selectedContainer.image}
                </code>
              </div>
            </DetailSection>

            {/* Resources */}
            <DetailSection title="Resources">
              <div className="grid grid-cols-2 gap-3">
                <div className="bg-gray-100 dark:bg-gray-800/50 rounded-lg p-3">
                  <div className="flex items-center gap-2 text-gray-500 dark:text-gray-400 text-xs mb-1">
                    <Cpu className="h-3 w-3" />
                    CPU Usage
                  </div>
                  <p className="text-lg font-semibold text-gray-900 dark:text-white">
                    {selectedContainer.cpu_percent?.toFixed(1) || 0}%
                  </p>
                </div>
                <div className="bg-gray-100 dark:bg-gray-800/50 rounded-lg p-3">
                  <div className="flex items-center gap-2 text-gray-500 dark:text-gray-400 text-xs mb-1">
                    <MemoryStick className="h-3 w-3" />
                    Memory
                  </div>
                  <p className="text-lg font-semibold text-gray-900 dark:text-white">
                    {selectedContainer.memory_mb || 0} MB
                  </p>
                </div>
              </div>
            </DetailSection>

            {/* Networks */}
            {selectedContainer.networks && selectedContainer.networks.length > 0 && (
              <DetailSection title="Networks">
                <div className="flex flex-wrap gap-2">
                  {selectedContainer.networks.map((network) => (
                    <span
                      key={network}
                      className="px-2 py-1 text-xs bg-gray-100 dark:bg-gray-800 text-gray-700 dark:text-gray-300 rounded-lg"
                    >
                      {network}
                    </span>
                  ))}
                </div>
              </DetailSection>
            )}
          </>
        );
    }
  };

  // Panel tabs configuration
  const panelTabs = [
    { id: "details", label: "Details" },
    { id: "logs", label: "Logs" },
    ...(selectedContainer?.status === "running" ? [{ id: "terminal", label: "Terminal" }] : []),
  ] as { id: PanelTab; label: string }[];

  return (
    <PageLayout
      title="Containers"
      description="View and manage Docker containers"
      actions={
        <div className="flex items-center gap-3">
          {/* View Mode Toggle */}
          <Tabs
            tabs={[
              { id: "all", label: "All" },
              { id: "stacks", label: "By Stack" },
            ]}
            activeTab={viewMode}
            onChange={(id) => setViewMode(id as "all" | "stacks")}
          />
        </div>
      }
      panelOpen={!!selectedContainer}
      panel={
        <DetailPanel
          open={!!selectedContainer}
          onClose={() => setSelectedContainer(null)}
          title={selectedContainer?.name}
          subtitle={selectedContainer?.image}
          defaultWidth={520}
        >
          {selectedContainer && (
            <div className="space-y-4">
              {/* Panel Tabs */}
              <Tabs
                tabs={panelTabs}
                activeTab={panelTab}
                onChange={(id) => setPanelTab(id as PanelTab)}
              />

              {/* Tab Content */}
              <div className="mt-4">
                {renderPanelContent()}
              </div>
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
            setSelectedContainer(null);
          }}
          className="w-full max-w-xs px-4 py-2 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg text-gray-900 dark:text-white focus:ring-2 focus:ring-primary-500 focus:border-transparent"
        >
          <option value="">Select an agent...</option>
          {agents?.map((agent) => (
            <option
              key={agent.id}
              value={agent.id}
              disabled={agent.status !== "active"}
            >
              {agent.name} ({agent.status})
            </option>
          ))}
        </select>
      </div>

      {/* Container list */}
      {selectedAgent ? (
        isLoading ? (
          <div className="flex items-center justify-center h-32">
            <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-500" />
          </div>
        ) : containers && containers.length > 0 ? (
          viewMode === "all" ? (
            <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
              {containers.map(renderContainerItem)}
            </div>
          ) : (
            <div className="space-y-6">
              {stackNames.map((stackName) => {
                const stackContainers = containersByStack[stackName];
                const runningCount = stackContainers.filter(c => c.status === "running").length;

                return (
                  <div
                    key={stackName}
                    className="bg-white dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-800 overflow-hidden"
                  >
                    {/* Stack Header */}
                    <div className="px-4 py-3 bg-gray-50 dark:bg-gray-800/50 border-b border-gray-200 dark:border-gray-800 flex items-center justify-between">
                      <div className="flex items-center gap-3">
                        <div className="p-1.5 bg-primary-100 dark:bg-primary-900/30 rounded">
                          <Layers className="h-4 w-4 text-primary-600 dark:text-primary-400" />
                        </div>
                        <div>
                          <h3 className="text-gray-900 dark:text-white font-medium">{stackName}</h3>
                          <p className="text-xs text-gray-500 dark:text-gray-400">
                            {runningCount}/{stackContainers.length} running
                          </p>
                        </div>
                      </div>
                      <span className={cn(
                        "px-2 py-1 text-xs rounded-full font-medium",
                        runningCount === stackContainers.length
                          ? "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400"
                          : runningCount === 0
                          ? "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400"
                          : "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400"
                      )}>
                        {runningCount === stackContainers.length ? "All Running" :
                         runningCount === 0 ? "Stopped" : "Partial"}
                      </span>
                    </div>

                    {/* Stack Containers */}
                    <div className="divide-y divide-gray-100 dark:divide-gray-800">
                      {stackContainers.map((container) => {
                        const isSelected = selectedContainer?.container_id === container.container_id;
                        const isNginx = container.image.includes("nginx") || container.name.toLowerCase().includes("nginx");

                        return (
                          <div
                            key={container.container_id}
                            onClick={() => {
                              setSelectedContainer(container);
                              setPanelTab("details");
                            }}
                            className={cn(
                              "px-4 py-3 flex items-center justify-between cursor-pointer transition-colors",
                              isSelected
                                ? "bg-primary-50 dark:bg-primary-900/20"
                                : "hover:bg-gray-50 dark:hover:bg-gray-800/50"
                            )}
                          >
                            <div className="flex items-center gap-3">
                              <div className={cn("w-2 h-2 rounded-full", getStatusColor(container.status))} />
                              <div>
                                <div className="flex items-center gap-2">
                                  <span className="text-gray-900 dark:text-white font-medium">{container.name}</span>
                                  {isNginx && (
                                    <span className="px-1.5 py-0.5 text-xs bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400 rounded">
                                      nginx
                                    </span>
                                  )}
                                </div>
                                <p className="text-xs text-gray-500 dark:text-gray-400">{container.image}</p>
                              </div>
                            </div>

                            <div className="flex items-center gap-3">
                              <div className="flex items-center gap-3 text-xs text-gray-500 dark:text-gray-400">
                                <span>{container.cpu_percent?.toFixed(1) || 0}% CPU</span>
                                <span>{container.memory_mb || 0} MB</span>
                              </div>
                              <ChevronRight className={cn(
                                "h-4 w-4 text-gray-400",
                                isSelected && "text-primary-500"
                              )} />
                            </div>
                          </div>
                        );
                      })}
                    </div>
                  </div>
                );
              })}
            </div>
          )
        ) : (
          <EmptyState
            icon={ContainerIcon}
            title="No containers found"
            description="Make sure Docker is running and containers are deployed"
          />
        )
      ) : (
        <EmptyState
          icon={Server}
          title="Select an agent"
          description={activeAgents.length === 0
            ? "No active agents available. Register an agent first."
            : "Choose an agent to view its containers"
          }
        />
      )}
    </PageLayout>
  );
}
