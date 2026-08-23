"use client";

import { useState, useMemo } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { FeatureGate } from "@/components/ui/FeatureGate";
import {
  Layers,
  AlertTriangle,
  CheckCircle,
  Eye,
  ChevronDown,
  ChevronRight,
  StopCircle,
  Box,
  Shield,
  Clock,
  User,
  Trash2,
  FileCode,
  RotateCcw,
} from "lucide-react";
import { api, Stack, Container, ManagedStack, RedeployStackRequest } from "@/lib/api";
import { useDocker } from "@/lib/docker-context";
import { StatCard, MetricsGrid } from "@/components/ui/StatCard";
import { Badge } from "@/components/ui/Badge";
import { EmptyState } from "@/components/ui/EmptyState";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/page-layout";
import { ConfirmDialog } from "@/components/ui/ConfirmDialog";
import { FilterToolbar, ToggleOption } from "@/components/ui/FilterToolbar";
import { StackRedeployModal } from "@/components/StackRedeployModal";
import { cn } from "@/lib/utils";

// Filter options
const STACK_STATUS_OPTIONS: ToggleOption[] = [
  { value: "all", label: "All" },
  { value: "running", label: "Running", color: "green" },
  { value: "partial", label: "Partial", color: "yellow" },
  { value: "stopped", label: "Stopped" },
];

type StackStatus = "running" | "partial" | "stopped";
type StatusFilter = "all" | StackStatus;

// Unified stack type combining discovered and managed data
interface UnifiedStack {
  name: string;
  container_count: number;
  running_count: number;
  status: StackStatus;
  containers: Container[];
  // Managed stack metadata (if available)
  isManaged: boolean;
  managedData?: ManagedStack;
}

const statusConfig: Record<StackStatus, { color: string; icon: React.ElementType; label: string }> = {
  running: { color: "green", icon: CheckCircle, label: "Running" },
  partial: { color: "yellow", icon: AlertTriangle, label: "Partial" },
  stopped: { color: "gray", icon: StopCircle, label: "Stopped" },
};

function DockerStacksPageContent() {
  const queryClient = useQueryClient();
  const { selectedAgent, openContainerPanel } = useDocker();

  // Local state
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all");
  const [searchFilter, setSearchFilter] = useState("");
  const [expandedStacks, setExpandedStacks] = useState<Set<string>>(new Set());
  const [stackToDelete, setStackToDelete] = useState<UnifiedStack | null>(null);
  const [stackToRedeploy, setStackToRedeploy] = useState<UnifiedStack | null>(null);
  const [redeployError, setRedeployError] = useState<string | null>(null);

  // Fetch discovered stacks from Docker
  const { data: discoveredStacks, isLoading: isLoadingDiscovered } = useQuery({
    queryKey: ["stacks", selectedAgent],
    queryFn: () => selectedAgent ? api.getStacks(selectedAgent) : Promise.resolve([]),
    enabled: !!selectedAgent,
    refetchInterval: 5000,
  });

  // Fetch managed stacks from database
  const { data: managedStacks, isLoading: isLoadingManaged } = useQuery({
    queryKey: ["managed-stacks", selectedAgent],
    queryFn: () => selectedAgent ? api.listManagedStacks(selectedAgent) : Promise.resolve([]),
    enabled: !!selectedAgent,
    refetchInterval: 5000,
  });

  // Delete managed stack mutation
  const deleteMutation = useMutation({
    mutationFn: (stackId: string) => api.deleteStack(selectedAgent!, stackId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["managed-stacks", selectedAgent] });
      queryClient.invalidateQueries({ queryKey: ["stacks", selectedAgent] });
      queryClient.invalidateQueries({ queryKey: ["containers", selectedAgent] });
      setStackToDelete(null);
    },
  });

  // Redeploy managed stack mutation
  const redeployMutation = useMutation({
    mutationFn: (request: RedeployStackRequest) =>
      api.redeployStack(selectedAgent!, stackToRedeploy!.managedData!.id, request),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["managed-stacks", selectedAgent] });
      queryClient.invalidateQueries({ queryKey: ["stacks", selectedAgent] });
      queryClient.invalidateQueries({ queryKey: ["containers", selectedAgent] });
      setStackToRedeploy(null);
      setRedeployError(null);
    },
    onError: (error: Error) => {
      setRedeployError(error.message || "Failed to redeploy stack");
    },
  });

  // Merge discovered and managed stacks
  const unifiedStacks = useMemo((): UnifiedStack[] => {
    if (!discoveredStacks) return [];

    // Create a map of managed stacks by name for quick lookup
    const managedMap = new Map<string, ManagedStack>();
    managedStacks?.forEach((ms) => {
      managedMap.set(ms.name, ms);
    });

    // Merge discovered stacks with managed data
    return discoveredStacks.map((ds): UnifiedStack => {
      const managed = managedMap.get(ds.name);
      return {
        name: ds.name,
        container_count: ds.container_count,
        running_count: ds.running_count,
        status: ds.status,
        containers: ds.containers,
        isManaged: !!managed,
        managedData: managed,
      };
    });
  }, [discoveredStacks, managedStacks]);

  // Filtered stacks
  const filteredStacks = useMemo(() => {
    return unifiedStacks.filter((s) => {
      const matchesStatus = statusFilter === "all" || s.status === statusFilter;
      const matchesSearch = !searchFilter ||
        s.name.toLowerCase().includes(searchFilter.toLowerCase()) ||
        s.managedData?.environment?.toLowerCase().includes(searchFilter.toLowerCase());
      return matchesStatus && matchesSearch;
    });
  }, [unifiedStacks, statusFilter, searchFilter]);

  // Metrics
  const metrics = {
    total: unifiedStacks.length,
    running: unifiedStacks.filter((s) => s.status === "running").length,
    partial: unifiedStacks.filter((s) => s.status === "partial").length,
    stopped: unifiedStacks.filter((s) => s.status === "stopped").length,
    managed: unifiedStacks.filter((s) => s.isManaged).length,
    totalContainers: unifiedStacks.reduce((sum, s) => sum + s.container_count, 0),
    runningContainers: unifiedStacks.reduce((sum, s) => sum + s.running_count, 0),
  };

  const isLoading = isLoadingDiscovered || isLoadingManaged;

  // Toggle stack expansion
  const toggleExpanded = (stackName: string) => {
    setExpandedStacks((prev) => {
      const next = new Set(prev);
      if (next.has(stackName)) {
        next.delete(stackName);
      } else {
        next.add(stackName);
      }
      return next;
    });
  };

  // Format relative time
  const formatRelativeTime = (dateString: string) => {
    const date = new Date(dateString);
    const now = new Date();
    const diffMs = now.getTime() - date.getTime();
    const diffMins = Math.floor(diffMs / 60000);
    const diffHours = Math.floor(diffMs / 3600000);
    const diffDays = Math.floor(diffMs / 86400000);

    if (diffMins < 1) return "just now";
    if (diffMins < 60) return `${diffMins}m ago`;
    if (diffHours < 24) return `${diffHours}h ago`;
    return `${diffDays}d ago`;
  };

  // Render status badge
  const renderStatusBadge = (status: StackStatus) => {
    const config = statusConfig[status];
    const Icon = config.icon;
    return (
      <Badge color={config.color as "gray" | "green" | "yellow"}>
        <Icon className="h-3 w-3 mr-1" />
        {config.label}
      </Badge>
    );
  };

  // Render environment badge
  const renderEnvBadge = (env: string) => {
    const colors: Record<string, "gray" | "blue" | "red"> = {
      dev: "gray",
      staging: "blue",
      prod: "red",
    };
    return <Badge color={colors[env] || "gray"}>{env}</Badge>;
  };

  if (isLoading) {
    return <Spinner.LogoPage label="Loading stacks..." />;
  }

  return (
    <div className="space-y-6">
      {/* Metrics */}
      <MetricsGrid>
        <StatCard
          title="Total Stacks"
          value={metrics.total}
          icon={Layers}
          iconColor="text-indigo-600"
          description={`${metrics.managed} managed`}
        />
        <StatCard
          title="Running"
          value={metrics.running}
          icon={CheckCircle}
          iconColor="text-green-600"
          description={`${metrics.runningContainers} containers`}
        />
        <StatCard
          title="Partial"
          value={metrics.partial}
          icon={AlertTriangle}
          iconColor="text-yellow-600"
        />
        <StatCard
          title="Stopped"
          value={metrics.stopped}
          icon={StopCircle}
          iconColor="text-gray-600"
        />
      </MetricsGrid>

      {/* Filter Toolbar */}
      <FilterToolbar
        primaryToggle={{
          options: STACK_STATUS_OPTIONS,
          value: statusFilter,
          onChange: (v) => setStatusFilter(v as StatusFilter),
        }}
        searchQuery={searchFilter}
        onSearchChange={setSearchFilter}
        searchPlaceholder="Search stacks..."
        showSearch={true}
        showRefresh={true}
        onRefresh={() => {
          queryClient.invalidateQueries({ queryKey: ["stacks", selectedAgent] });
          queryClient.invalidateQueries({ queryKey: ["managed-stacks", selectedAgent] });
        }}
        singleRow={true}
      />

      {/* Stacks List */}
      {filteredStacks.length === 0 ? (
        <EmptyState
          icon={Layers}
          title="No stacks found"
          description={
            unifiedStacks.length === 0
              ? "Deploy your first stack using the Deploy Stack button above. Stacks are groups of containers deployed via docker-compose."
              : "No stacks match your current filters"
          }
        />
      ) : (
        <div className="space-y-4">
          {filteredStacks.map((stack) => (
            <StackCard
              key={stack.name}
              stack={stack}
              isExpanded={expandedStacks.has(stack.name)}
              onToggleExpand={() => toggleExpanded(stack.name)}
              onOpenContainer={openContainerPanel}
              onDelete={() => setStackToDelete(stack)}
              onRedeploy={() => {
                setRedeployError(null);
                setStackToRedeploy(stack);
              }}
              renderStatusBadge={renderStatusBadge}
              renderEnvBadge={renderEnvBadge}
              formatRelativeTime={formatRelativeTime}
            />
          ))}
        </div>
      )}

      {/* Delete Confirmation */}
      <ConfirmDialog
        isOpen={!!stackToDelete}
        onClose={() => setStackToDelete(null)}
        onConfirm={() => {
          if (stackToDelete?.managedData) {
            deleteMutation.mutate(stackToDelete.managedData.id);
          } else {
            setStackToDelete(null);
          }
        }}
        title={`Delete Stack "${stackToDelete?.name}"?`}
        message={
          stackToDelete?.isManaged
            ? `This will stop and remove all containers in this stack. ${stackToDelete?.running_count || 0} running container(s) will be stopped.`
            : "This stack is not managed by InfraPilot. To remove it, use docker-compose down on the host."
        }
        confirmText={stackToDelete?.isManaged ? "Delete Stack" : "Close"}
        variant="danger"
        isLoading={deleteMutation.isPending}
      />

      {/* Redeploy */}
      <StackRedeployModal
        isOpen={!!stackToRedeploy}
        agentId={selectedAgent ?? undefined}
        stack={stackToRedeploy?.managedData ?? null}
        onClose={() => {
          setStackToRedeploy(null);
          setRedeployError(null);
        }}
        onConfirm={(request) => redeployMutation.mutate(request)}
        isLoading={redeployMutation.isPending}
        errorMessage={redeployError}
      />
    </div>
  );
}

export default function DockerStacksPage() {
  return (
    <FeatureGate feature="stack_management" tier="professional" featureLabel="Docker Stacks">
      <DockerStacksPageContent />
    </FeatureGate>
  );
}

// Stack Card Component
interface StackCardProps {
  stack: UnifiedStack;
  isExpanded: boolean;
  onToggleExpand: () => void;
  onOpenContainer: (containerId: string) => void;
  onDelete: () => void;
  onRedeploy: () => void;
  renderStatusBadge: (status: StackStatus) => React.ReactNode;
  renderEnvBadge: (env: string) => React.ReactNode;
  formatRelativeTime: (date: string) => string;
}

function StackCard({
  stack,
  isExpanded,
  onToggleExpand,
  onOpenContainer,
  onDelete,
  onRedeploy,
  renderStatusBadge,
  renderEnvBadge,
  formatRelativeTime,
}: StackCardProps) {
  const managed = stack.managedData;

  return (
    <div className="bg-white dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-800 overflow-hidden">
      {/* Stack Header */}
      <div
        className="flex items-center justify-between p-4 cursor-pointer hover:bg-gray-50 dark:hover:bg-gray-800/50 transition-colors"
        onClick={onToggleExpand}
      >
        <div className="flex items-center gap-4">
          <button className="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300">
            {isExpanded ? (
              <ChevronDown className="h-5 w-5" />
            ) : (
              <ChevronRight className="h-5 w-5" />
            )}
          </button>
          <div className={cn(
            "p-2 rounded-lg",
            stack.isManaged
              ? "bg-indigo-100 dark:bg-indigo-900/30"
              : "bg-gray-100 dark:bg-gray-800"
          )}>
            <Layers className={cn(
              "h-5 w-5",
              stack.isManaged
                ? "text-indigo-600 dark:text-indigo-400"
                : "text-gray-500 dark:text-gray-400"
            )} />
          </div>
          <div>
            <div className="flex items-center gap-2 flex-wrap">
              <h3 className="font-semibold text-gray-900 dark:text-white">{stack.name}</h3>
              {managed?.environment && renderEnvBadge(managed.environment)}
              {renderStatusBadge(stack.status)}
              {stack.isManaged ? (
                <Badge color="purple" className="text-xs">
                  <Shield className="h-3 w-3 mr-1" />
                  Managed
                </Badge>
              ) : (
                <Badge color="gray" className="text-xs">
                  External
                </Badge>
              )}
            </div>
            <div className="flex items-center gap-4 text-sm text-gray-500 dark:text-gray-400 mt-1 flex-wrap">
              <span className="flex items-center gap-1">
                <Box className="h-3.5 w-3.5" />
                {stack.running_count}/{stack.container_count} containers
              </span>
              {managed?.deployed_at && (
                <span className="flex items-center gap-1">
                  <Clock className="h-3.5 w-3.5" />
                  Deployed {formatRelativeTime(managed.deployed_at)}
                </span>
              )}
              {managed?.status_message && (
                <span className="text-xs">{managed.status_message}</span>
              )}
            </div>
          </div>
        </div>

        <div className="flex items-center gap-2" onClick={(e) => e.stopPropagation()}>
          {stack.isManaged && (
            <>
              <Button
                variant="secondary"
                size="sm"
                onClick={onRedeploy}
              >
                <RotateCcw className="h-4 w-4" />
              </Button>
              <Button
                variant="secondary"
                size="sm"
                onClick={onDelete}
              >
                <Trash2 className="h-4 w-4" />
              </Button>
            </>
          )}
        </div>
      </div>

      {/* Managed Stack Details */}
      {isExpanded && stack.isManaged && managed && (
        <div className="px-4 py-3 bg-indigo-50 dark:bg-indigo-900/10 border-t border-indigo-100 dark:border-indigo-900/30">
          <div className="flex items-center gap-6 text-sm">
            <div className="flex items-center gap-2 text-indigo-700 dark:text-indigo-300">
              <Shield className="h-4 w-4" />
              <span className="font-medium">Managed Stack</span>
            </div>
            {managed.service_count > 0 && (
              <span className="text-gray-600 dark:text-gray-400">
                {managed.running_count}/{managed.service_count} services tracked
              </span>
            )}
            {managed.variables && Object.keys(managed.variables).length > 0 && (
              <span className="text-gray-600 dark:text-gray-400">
                {Object.keys(managed.variables).length} variables
              </span>
            )}
            {managed.compose_yaml && (
              <span className="flex items-center gap-1 text-gray-600 dark:text-gray-400">
                <FileCode className="h-3.5 w-3.5" />
                Compose YAML stored
              </span>
            )}
          </div>
        </div>
      )}

      {/* Expanded Content - Containers Table */}
      {isExpanded && (
        <div className="border-t border-gray-200 dark:border-gray-800">
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead className="bg-gray-50 dark:bg-gray-800/50">
                <tr>
                  <th className="px-4 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                    Container
                  </th>
                  <th className="px-4 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                    Image
                  </th>
                  <th className="px-4 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                    Status
                  </th>
                  <th className="px-4 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                    Networks
                  </th>
                  <th className="px-4 py-2 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                    Actions
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200 dark:divide-gray-800">
                {stack.containers.map((container) => (
                  <ContainerRow
                    key={container.container_id}
                    container={container}
                    onOpenContainer={onOpenContainer}
                  />
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  );
}

// Container Row Component
interface ContainerRowProps {
  container: Container;
  onOpenContainer: (containerId: string) => void;
}

function ContainerRow({ container, onOpenContainer }: ContainerRowProps) {
  const getStatusColor = (status: string) => {
    switch (status) {
      case "running":
        return "green";
      case "exited":
      case "stopped":
        return "gray";
      case "paused":
        return "yellow";
      default:
        return "gray";
    }
  };

  return (
    <tr className="hover:bg-gray-50 dark:hover:bg-gray-800/30">
      <td className="px-4 py-3">
        <span className="font-medium text-gray-900 dark:text-white">
          {container.name}
        </span>
      </td>
      <td className="px-4 py-3">
        <span className="text-sm text-gray-600 dark:text-gray-400 font-mono truncate block max-w-xs">
          {container.image}
        </span>
      </td>
      <td className="px-4 py-3">
        <Badge color={getStatusColor(container.status) as "green" | "gray" | "yellow"}>
          {container.status}
        </Badge>
      </td>
      <td className="px-4 py-3">
        <div className="flex flex-wrap gap-1">
          {container.networks?.slice(0, 2).map((network) => (
            <span
              key={network}
              className="text-xs px-2 py-0.5 bg-gray-100 dark:bg-gray-800 rounded text-gray-600 dark:text-gray-400"
            >
              {network}
            </span>
          ))}
          {container.networks && container.networks.length > 2 && (
            <span className="text-xs text-gray-500">+{container.networks.length - 2}</span>
          )}
        </div>
      </td>
      <td className="px-4 py-3 text-right">
        <Button
          variant="ghost"
          size="sm"
          onClick={() => onOpenContainer(container.container_id)}
        >
          <Eye className="h-4 w-4" />
        </Button>
      </td>
    </tr>
  );
}
