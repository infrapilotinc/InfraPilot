"use client";

import { useState, useEffect, useMemo } from "react";
import { useQuery, useQueryClient, useMutation } from "@tanstack/react-query";
import {
  Package,
  CheckSquare,
  MinusSquare,
  Square,
  Lock,
  RotateCcw,
  Trash2,
  Container,
  Activity,
  Play,
  XCircle,
  Zap,
  GitMerge,
  Layers,
  ClipboardList,
} from "lucide-react";
import { api, Deployment } from "@/lib/api";
import { useDocker } from "@/lib/docker-context";
import { Table } from "@/components/ui/Table";
import { Badge } from "@/components/ui/Badge";
import { EmptyState } from "@/components/ui/EmptyState";
import { Spinner } from "@/components/ui/Spinner";
import { StatusIndicator } from "@/components/ui/StatusIndicator";
import { StatCard, MetricsGrid } from "@/components/ui/StatCard";
import { RedeployWizard } from "@/components/RedeployWizard";
import { Button } from "@/components/ui/page-layout";
import { FilterToolbar, ToggleOption } from "@/components/ui/FilterToolbar";
import { cn } from "@/lib/utils";

// Filter options
const DEPLOYMENT_STATUS_OPTIONS: ToggleOption[] = [
  { value: "all", label: "All" },
  { value: "running", label: "Running", color: "green" },
  { value: "failed", label: "Failed", color: "red" },
  { value: "pending", label: "Pending", color: "yellow" },
];

type StatusFilter = "all" | "running" | "failed" | "pending" | "scanning";

export default function DeploymentsPage() {
  const queryClient = useQueryClient();
  const { selectedAgent, openDeploymentPanel } = useDocker();

  // Local state
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all");
  const [searchFilter, setSearchFilter] = useState("");
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [showBulkDeleteModal, setShowBulkDeleteModal] = useState(false);
  const [bulkProgress, setBulkProgress] = useState<{ total: number; completed: number; failed: string[] } | null>(null);
  const [redeployWizardOpen, setRedeployWizardOpen] = useState(false);
  const [redeployTarget, setRedeployTarget] = useState<Deployment | null>(null);
  const [redeployingId, setRedeployingId] = useState<string | null>(null);

  // Fetch deployments
  const { data: deployments, isLoading } = useQuery({
    queryKey: ["deployments", selectedAgent],
    queryFn: () => selectedAgent ? api.getDeployments(selectedAgent) : Promise.resolve([]),
    enabled: !!selectedAgent,
    refetchInterval: 10000,
  });

  // Sync deployment status with actual container state
  useEffect(() => {
    if (selectedAgent) {
      api.syncDeploymentStatus(selectedAgent).then((result) => {
        if (result.synced > 0) {
          queryClient.invalidateQueries({ queryKey: ["deployments", selectedAgent] });
        }
      }).catch(() => {});
    }
  }, [selectedAgent, queryClient]);

  // Filtered deployments
  const filteredDeployments = useMemo(() => {
    if (!deployments) return [];
    return deployments.filter((d) => {
      const matchesStatus = statusFilter === "all" || d.status === statusFilter;
      const matchesSearch = !searchFilter ||
        d.service_name?.toLowerCase().includes(searchFilter.toLowerCase()) ||
        d.image_repository?.toLowerCase().includes(searchFilter.toLowerCase()) ||
        d.container_name?.toLowerCase().includes(searchFilter.toLowerCase());
      return matchesStatus && matchesSearch;
    });
  }, [deployments, statusFilter, searchFilter]);

  // Metrics
  const metrics = {
    total: deployments?.length || 0,
    running: deployments?.filter((d) => d.status === "running").length || 0,
    failed: deployments?.filter((d) => d.status === "failed").length || 0,
    pending: deployments?.filter((d) => d.status === "pending" || d.status === "scanning" || d.status === "deploying").length || 0,
  };

  // Selection helpers - can't select running deployments for deletion
  const isProtected = (d: Deployment) => d.status === "running";
  const selectableDeployments = filteredDeployments.filter((d) => !isProtected(d));

  const toggleSelection = (id: string) => {
    setSelectedIds((prev) => {
      const newSet = new Set(prev);
      if (newSet.has(id)) newSet.delete(id);
      else newSet.add(id);
      return newSet;
    });
  };

  const selectAll = () => setSelectedIds(new Set(selectableDeployments.map((d) => d.id)));
  const selectNone = () => setSelectedIds(new Set());
  const selectFailed = () => setSelectedIds(new Set(filteredDeployments.filter((d) => d.status === "failed").map((d) => d.id)));

  const isAllSelected = selectableDeployments.length > 0 && selectedIds.size === selectableDeployments.length;
  const isSomeSelected = selectedIds.size > 0 && !isAllSelected;
  const selectedData = filteredDeployments.filter((d) => selectedIds.has(d.id));

  // Redeploy mutation
  const redeployMutation = useMutation({
    mutationFn: ({ deploymentId, pullLatest, skipScanning }: { deploymentId: string; pullLatest: boolean; skipScanning: boolean }) => {
      if (!selectedAgent) throw new Error("No agent selected");
      return api.redeployDeployment(selectedAgent, deploymentId, pullLatest, skipScanning);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["deployments", selectedAgent] });
      setRedeployingId(null);
      setRedeployWizardOpen(false);
      setRedeployTarget(null);
    },
    onError: () => {
      setRedeployingId(null);
      setRedeployWizardOpen(false);
    },
  });

  // Bulk delete
  const handleBulkDelete = async () => {
    const toDelete = selectedData.filter((d) => !isProtected(d));
    if (toDelete.length === 0) return;
    setBulkProgress({ total: toDelete.length, completed: 0, failed: [] });
    const failed: string[] = [];
    for (let i = 0; i < toDelete.length; i++) {
      try {
        await api.deleteDeployment(selectedAgent!, toDelete[i].id);
      } catch {
        failed.push(toDelete[i].service_name || toDelete[i].id.slice(0, 8));
      }
      setBulkProgress({ total: toDelete.length, completed: i + 1, failed });
    }
    queryClient.invalidateQueries({ queryKey: ["deployments", selectedAgent] });
    if (failed.length === 0) {
      setShowBulkDeleteModal(false);
      selectNone();
      setBulkProgress(null);
    }
  };

  // Status helpers
  const getStatusLevel = (status: Deployment["status"]): "healthy" | "warning" | "degraded" | "critical" => {
    switch (status) {
      case "running": return "healthy";
      case "failed": return "critical";
      case "scanning":
      case "policy_check":
      case "deploying": return "warning";
      default: return "degraded";
    }
  };

  // Table columns — CE edition (no security scan / policy / SBOM columns)
  const columns = [
    {
      key: "checkbox",
      header: (
        <button onClick={(e) => { e.stopPropagation(); isAllSelected ? selectNone() : selectAll(); }} className="p-1 hover:bg-gray-100 dark:hover:bg-gray-800 rounded">
          {isAllSelected ? <CheckSquare className="h-4 w-4 text-primary-600" /> : isSomeSelected ? <MinusSquare className="h-4 w-4 text-primary-600" /> : <Square className="h-4 w-4 text-gray-400" />}
        </button>
      ),
      width: "40px",
      render: (_: unknown, row: Deployment) => (
        <button
          onClick={(e) => { e.stopPropagation(); if (!isProtected(row)) toggleSelection(row.id); }}
          className="p-1 hover:bg-gray-100 dark:hover:bg-gray-800 rounded"
          title={isProtected(row) ? "Running deployment" : undefined}
        >
          {isProtected(row) ? (
            <Lock className="h-4 w-4 text-green-500" />
          ) : selectedIds.has(row.id) ? (
            <CheckSquare className="h-4 w-4 text-primary-600" />
          ) : (
            <Square className="h-4 w-4 text-gray-400" />
          )}
        </button>
      ),
    },
    {
      key: "status",
      header: "Status",
      render: (_: unknown, row: Deployment) => (
        <StatusIndicator
          status={getStatusLevel(row.status)}
          label={row.status?.replace(/_/g, " ") || "unknown"}
          pulse={row.status === "running"}
        />
      ),
    },
    {
      key: "service_name",
      header: "Service",
      sortable: true,
      render: (_: unknown, row: Deployment) => (
        <div className="flex items-center gap-3">
          <div className="p-2 rounded-lg bg-gray-100 dark:bg-gray-800">
            <Package className="h-5 w-5 text-gray-500" />
          </div>
          <div>
            <p className="font-medium text-gray-900 dark:text-white">{row.service_name || "Unnamed"}</p>
            <p className="text-xs text-gray-500 font-mono">{row.id?.slice(0, 8)}...</p>
          </div>
        </div>
      ),
    },
    {
      key: "environment",
      header: "Environment",
      sortable: true,
      render: (value: string) => {
        const envColors: Record<string, string> = {
          prod: "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400",
          production: "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400",
          staging: "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400",
          dev: "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400",
          development: "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400",
        };
        return (
          <Badge className={cn("px-2 py-1 text-xs rounded border-0", envColors[value?.toLowerCase()] || "bg-gray-100 text-gray-700 dark:bg-gray-800 dark:text-gray-400")}>
            {value || "—"}
          </Badge>
        );
      },
    },
    {
      key: "image_repository",
      header: "Image",
      render: (_: unknown, row: Deployment) => (
        <div>
          <p className="text-sm font-mono text-gray-900 dark:text-white truncate max-w-[200px]">{row.image_repository || "—"}</p>
          {row.image_tag && <p className="text-xs text-primary-600 dark:text-primary-400 font-mono">:{row.image_tag}</p>}
        </div>
      ),
    },
    {
      key: "container_name",
      header: "Container",
      render: (_: unknown, row: Deployment) => {
        if (row.container_name || row.container_id) {
          return (
            <div className="flex items-center gap-2">
              <Container className="w-4 h-4 text-green-500" />
              <span className="text-xs font-mono text-gray-900 dark:text-white">{row.container_name || row.container_id?.substring(0, 12)}</span>
            </div>
          );
        }
        return <span className="text-xs text-gray-400">—</span>;
      },
    },
    {
      key: "deployed_at",
      header: "Deployed",
      render: (_: unknown, row: Deployment) => {
        const ts = (row as any).deployed_at || row.created_at;
        if (!ts) return <span className="text-xs text-gray-400">—</span>;
        return <span className="text-xs text-gray-500">{new Date(ts).toLocaleString()}</span>;
      },
    },
    {
      key: "actions",
      header: "",
      width: "80px",
      render: (_: unknown, row: Deployment) => (
        <div className="flex items-center gap-1">
          <button
            onClick={(e) => { e.stopPropagation(); setRedeployTarget(row); setRedeployWizardOpen(true); }}
            disabled={redeployingId === row.id}
            className="p-1.5 text-gray-400 hover:text-primary-600 dark:hover:text-primary-400 rounded disabled:opacity-50"
            title="Redeploy"
          >
            <RotateCcw className={cn("h-4 w-4", redeployingId === row.id && "animate-spin")} />
          </button>
          <button
            onClick={(e) => { e.stopPropagation(); if (!isProtected(row)) { setSelectedIds(new Set([row.id])); setShowBulkDeleteModal(true); } }}
            disabled={isProtected(row)}
            className="p-1.5 text-gray-400 hover:text-red-600 dark:hover:text-red-400 rounded disabled:opacity-50"
            title={isProtected(row) ? "Stop container first" : "Delete"}
          >
            <Trash2 className="h-4 w-4" />
          </button>
        </div>
      ),
    },
  ];

  if (!selectedAgent) {
    return <Spinner.LogoPage label="Selecting agent..." />;
  }

  return (
    <div className="space-y-4">
      {/* Metrics */}
      <MetricsGrid columns={4}>
        <StatCard label="Total" value={metrics.total} icon={Package} iconColor="text-blue-600 dark:text-blue-400" />
        <StatCard label="Running" value={metrics.running} icon={Play} iconColor="text-green-600 dark:text-green-400" />
        <StatCard label="Failed" value={metrics.failed} icon={XCircle} iconColor="text-red-600 dark:text-red-400" />
        <StatCard label="In Progress" value={metrics.pending} icon={Activity} iconColor="text-yellow-600 dark:text-yellow-400" />
      </MetricsGrid>

      {/* Filter Toolbar */}
      <FilterToolbar
        primaryToggle={{
          options: DEPLOYMENT_STATUS_OPTIONS,
          value: statusFilter,
          onChange: (v) => setStatusFilter(v as StatusFilter),
        }}
        searchQuery={searchFilter}
        onSearchChange={setSearchFilter}
        searchPlaceholder="Search deployments..."
        showSearch={true}
        showRefresh={true}
        onRefresh={() => queryClient.invalidateQueries({ queryKey: ["deployments", selectedAgent] })}
        singleRow={true}
      />

      {/* Selection Bar */}
      {deployments && deployments.length > 0 && (
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <span className="text-sm text-gray-500 dark:text-gray-400">Quick select:</span>
            <button onClick={selectAll} className="px-3 py-1.5 text-sm bg-gray-100 dark:bg-gray-800 hover:bg-gray-200 dark:hover:bg-gray-700 rounded-lg">All ({selectableDeployments.length})</button>
            <button onClick={selectFailed} disabled={metrics.failed === 0} className="px-3 py-1.5 text-sm bg-gray-100 dark:bg-gray-800 hover:bg-gray-200 dark:hover:bg-gray-700 rounded-lg disabled:opacity-50">Failed ({metrics.failed})</button>
            {selectedIds.size > 0 && <button onClick={selectNone} className="px-3 py-1.5 text-sm text-gray-600 dark:text-gray-400">Clear</button>}
          </div>
          {selectedIds.size > 0 && (
            <div className="flex items-center gap-3">
              <span className="text-sm text-gray-700 dark:text-gray-300"><strong>{selectedIds.size}</strong> selected</span>
              <Button variant="danger" size="sm" onClick={() => setShowBulkDeleteModal(true)}>
                <Trash2 className="h-4 w-4 mr-1" />Delete Selected
              </Button>
            </div>
          )}
        </div>
      )}

      {/* Table */}
      {isLoading ? (
        <Spinner.LogoPage label="Loading deployments..." />
      ) : filteredDeployments.length > 0 ? (
        <div className="bg-white dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-800 overflow-hidden">
          <Table
            data={filteredDeployments}
            columns={columns}
            keyExtractor={(row) => row.id}
            hoverable
            onRowClick={(row) => openDeploymentPanel(row.id)}
            rowClassName={(row) => cn(selectedIds.has(row.id) && "bg-blue-50 dark:bg-blue-900/10")}
          />
        </div>
      ) : (
        <EmptyState
          icon={Package}
          title="No deployments found"
          description={deployments?.length === 0 ? "Deploy a service to get started" : "No deployments match the current filters"}
        />
      )}

      {/* Enterprise Features Upgrade Card */}
      <div className="rounded-xl border border-gray-200 dark:border-gray-700 bg-gradient-to-br from-gray-50 to-purple-50/30 dark:from-gray-800/50 dark:to-purple-900/10 p-6">
        <div className="flex items-start gap-4">
          <div className="p-2 bg-purple-100 dark:bg-purple-900/30 rounded-lg flex-shrink-0">
            <Zap className="h-5 w-5 text-purple-600 dark:text-purple-400" />
          </div>
          <div className="flex-1 min-w-0">
            <h3 className="text-sm font-semibold text-gray-900 dark:text-white mb-1">Enterprise Deployment Features</h3>
            <p className="text-xs text-gray-500 dark:text-gray-400 mb-4">
              Unlock advanced deployment capabilities for teams with strict compliance and multi-environment requirements.
            </p>
            <div className="grid grid-cols-1 sm:grid-cols-3 gap-3 mb-4">
              {[
                { icon: GitMerge, label: "Multi-environment Promotion", desc: "Promote deployments dev → staging → production" },
                { icon: Layers, label: "Canary / Blue-Green", desc: "Progressive rollouts with traffic splitting" },
                { icon: ClipboardList, label: "Deployment Audit Log", desc: "Full compliance trail with who deployed what" },
              ].map(({ icon: Icon, label, desc }) => (
                <div key={label} className="flex items-start gap-3 p-3 bg-white dark:bg-gray-800/50 rounded-lg border border-gray-200 dark:border-gray-700">
                  <Icon className="h-4 w-4 text-purple-500 flex-shrink-0 mt-0.5" />
                  <div>
                    <p className="text-xs font-medium text-gray-900 dark:text-white">{label}</p>
                    <p className="text-xs text-gray-500 dark:text-gray-400">{desc}</p>
                  </div>
                </div>
              ))}
            </div>
            <a
              href="https://infrapilot.org/enterprise"
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-2 px-4 py-2 bg-gradient-to-r from-primary-600 to-purple-600 hover:from-primary-700 hover:to-purple-700 text-white text-sm font-medium rounded-lg transition-all shadow-sm"
            >
              <Zap className="h-3.5 w-3.5" />
              Learn about Enterprise
            </a>
          </div>
        </div>
      </div>

      {/* Redeploy Wizard */}
      <RedeployWizard
        isOpen={redeployWizardOpen}
        deployment={redeployTarget}
        onClose={() => { setRedeployWizardOpen(false); setRedeployTarget(null); }}
        onConfirm={(pullLatest, skipScanning) => {
          if (!redeployTarget) return;
          setRedeployingId(redeployTarget.id);
          redeployMutation.mutate({ deploymentId: redeployTarget.id, pullLatest, skipScanning });
        }}
        isLoading={redeployMutation.isPending}
      />

      {/* Bulk Delete Modal */}
      {showBulkDeleteModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center">
          <div className="absolute inset-0 bg-black/50" onClick={() => !bulkProgress && setShowBulkDeleteModal(false)} />
          <div className="relative bg-white dark:bg-gray-900 rounded-lg shadow-xl max-w-lg w-full mx-4 p-6">
            <div className="flex items-center gap-3 mb-4">
              <div className="p-2 bg-red-100 dark:bg-red-900/30 rounded-full">
                <Trash2 className="h-5 w-5 text-red-600" />
              </div>
              <h3 className="text-lg font-semibold text-gray-900 dark:text-white">Delete {selectedIds.size} Deployment{selectedIds.size > 1 ? "s" : ""}</h3>
            </div>
            {!bulkProgress ? (
              <>
                <p className="text-gray-600 dark:text-gray-400 mb-4">
                  This will permanently remove the selected deployment records. Running containers will not be affected.
                </p>
                <div className="flex justify-end gap-3">
                  <Button variant="secondary" onClick={() => { setShowBulkDeleteModal(false); }}>Cancel</Button>
                  <Button variant="danger" onClick={handleBulkDelete}>Delete</Button>
                </div>
              </>
            ) : (
              <div className="space-y-4">
                <div>
                  <div className="flex justify-between text-sm mb-1">
                    <span className="text-gray-600 dark:text-gray-400">Deleting...</span>
                    <span className="text-gray-900 dark:text-white">{bulkProgress.completed} / {bulkProgress.total}</span>
                  </div>
                  <div className="w-full bg-gray-200 dark:bg-gray-700 rounded-full h-2">
                    <div className="bg-red-600 h-2 rounded-full transition-all" style={{ width: `${(bulkProgress.completed / bulkProgress.total) * 100}%` }} />
                  </div>
                </div>
                {bulkProgress.failed.length > 0 && (
                  <div className="p-3 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg">
                    <p className="text-sm font-medium text-red-600 mb-2">Failed: {bulkProgress.failed.length}</p>
                    <ul className="text-xs text-red-600">{bulkProgress.failed.map((n, i) => <li key={i}>• {n}</li>)}</ul>
                  </div>
                )}
                {bulkProgress.completed === bulkProgress.total && (
                  <div className="flex justify-end">
                    <Button variant="secondary" onClick={() => { setShowBulkDeleteModal(false); selectNone(); setBulkProgress(null); }}>
                      {bulkProgress.failed.length > 0 ? "Close" : "Done"}
                    </Button>
                  </div>
                )}
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
