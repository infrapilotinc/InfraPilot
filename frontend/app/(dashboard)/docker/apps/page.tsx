"use client";

import { useState, useMemo } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Rocket, Trash2, RefreshCw, Box, CheckCircle, XCircle, Clock,
  Search, MoreHorizontal, Tag, GitBranch, AlertTriangle,
} from "lucide-react";
import { api, ServiceConfig } from "@/lib/api";
import { EmptyState } from "@/components/ui/EmptyState";
import { Spinner } from "@/components/ui/Spinner";
import { StatCard, MetricsGrid } from "@/components/ui/StatCard";
import { ConfirmDialog } from "@/components/ui/ConfirmDialog";
import { cn } from "@/lib/utils";

// ─── Helpers ─────────────────────────────────────────────────────────────────

function relativeTime(ts?: string | null): string {
  if (!ts) return "—";
  const diff = Date.now() - new Date(ts).getTime();
  const mins = Math.floor(diff / 60000);
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins}m ago`;
  const hrs = Math.floor(mins / 60);
  if (hrs < 24) return `${hrs}h ago`;
  return `${Math.floor(hrs / 24)}d ago`;
}

function statusBadge(status?: string) {
  const colorMap: Record<string, string> = {
    running:   "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400",
    failed:    "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400",
    deploying: "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400",
    pending:   "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400",
    stopped:   "bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-400",
  };
  const cls = colorMap[status ?? ""] ?? "bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-400";
  return <span className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${cls}`}>{status ?? "unknown"}</span>;
}

function envBadge(env: string) {
  const cls =
    env === "prod"    ? "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400" :
    env === "staging" ? "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400" :
                        "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400";
  return <span className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${cls}`}>{env}</span>;
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function AppsPage() {
  const queryClient = useQueryClient();

  const [search, setSearch] = useState("");
  const [envFilter, setEnvFilter] = useState<"all" | "dev" | "staging" | "prod">("all");
  const [openMenu, setOpenMenu] = useState<string | null>(null);
  const [redeployTarget, setRedeployTarget] = useState<ServiceConfig | null>(null);
  const [redeployTag, setRedeployTag] = useState("");
  const [deleteTarget, setDeleteTarget] = useState<ServiceConfig | null>(null);

  const { data: services, isLoading } = useQuery({
    queryKey: ["services"],
    queryFn: () => api.listServices(),
    refetchInterval: 15000,
  });

  const redeployMutation = useMutation({
    mutationFn: (svc: ServiceConfig) =>
      api.deployService(svc.service_name, svc.environment, redeployTag ? { image_tag: redeployTag } : {}),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["services"] });
      queryClient.invalidateQueries({ queryKey: ["deployments"] });
      setRedeployTarget(null);
      setRedeployTag("");
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (svc: ServiceConfig) => api.deleteService(svc.service_name, svc.environment),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["services"] });
      setDeleteTarget(null);
    },
  });

  const filtered = useMemo(() => {
    if (!services) return [];
    return services.filter((s) => {
      if (envFilter !== "all" && s.environment !== envFilter) return false;
      if (!search) return true;
      const q = search.toLowerCase();
      return s.service_name.toLowerCase().includes(q) || s.image_repository.toLowerCase().includes(q);
    });
  }, [services, envFilter, search]);

  const metrics = useMemo(() => ({
    total:     services?.length ?? 0,
    running:   services?.filter((s) => s.status === "running").length ?? 0,
    failed:    services?.filter((s) => s.status === "failed").length ?? 0,
    deploying: services?.filter((s) => s.status === "deploying" || s.status === "pending").length ?? 0,
  }), [services]);

  if (isLoading) return <Spinner.LogoPage label="Loading services..." />;

  return (
    <div className="space-y-6">
      {/* Stats */}
      <MetricsGrid>
        <StatCard title="Total Services" value={metrics.total} icon={Box} />
        <StatCard title="Running" value={metrics.running} icon={CheckCircle} variant="success" />
        <StatCard title="Failed" value={metrics.failed} icon={XCircle} variant="danger" />
        <StatCard title="Deploying" value={metrics.deploying} icon={Clock} variant="warning" />
      </MetricsGrid>

      {/* Filters */}
      <div className="flex items-center gap-3 flex-wrap">
        <div className="relative flex-1 min-w-[200px] max-w-sm">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-gray-400" />
          <input
            type="text"
            placeholder="Search services..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="w-full pl-9 pr-3 py-2 text-sm bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg text-gray-900 dark:text-white placeholder-gray-400 focus:ring-2 focus:ring-primary-500 focus:border-transparent"
          />
        </div>
        <div className="flex gap-1 rounded-lg border border-gray-200 dark:border-gray-700 p-0.5 bg-gray-50 dark:bg-gray-800/50">
          {(["all", "dev", "staging", "prod"] as const).map((env) => (
            <button
              key={env}
              onClick={() => setEnvFilter(env)}
              className={cn(
                "px-3 py-1.5 text-xs font-medium rounded-md transition-colors",
                envFilter === env
                  ? "bg-white dark:bg-gray-700 text-gray-900 dark:text-white shadow-sm"
                  : "text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200"
              )}
            >
              {env === "all" ? "All Envs" : env}
            </button>
          ))}
        </div>
        <button
          onClick={() => queryClient.invalidateQueries({ queryKey: ["services"] })}
          className="p-2 text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-800 transition-colors"
        >
          <RefreshCw className="h-4 w-4" />
        </button>
      </div>

      {/* Table */}
      {filtered.length === 0 ? (
        <EmptyState
          icon={Box}
          title="No services found"
          description='Deploy a service with: infrapilot deploy <name> --image <image>'
        />
      ) : (
        <div className="bg-white dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-800 overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-gray-200 dark:border-gray-800 bg-gray-50 dark:bg-gray-800/50">
                <th className="text-left px-4 py-3 font-medium text-gray-500 dark:text-gray-400 uppercase text-xs tracking-wider">Service</th>
                <th className="text-left px-4 py-3 font-medium text-gray-500 dark:text-gray-400 uppercase text-xs tracking-wider">Env</th>
                <th className="text-left px-4 py-3 font-medium text-gray-500 dark:text-gray-400 uppercase text-xs tracking-wider">Image</th>
                <th className="text-left px-4 py-3 font-medium text-gray-500 dark:text-gray-400 uppercase text-xs tracking-wider">Status</th>
                <th className="text-left px-4 py-3 font-medium text-gray-500 dark:text-gray-400 uppercase text-xs tracking-wider">Deployed</th>
                <th className="text-left px-4 py-3 font-medium text-gray-500 dark:text-gray-400 uppercase text-xs tracking-wider">Container</th>
                <th className="px-4 py-3 w-10" />
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-gray-800">
              {filtered.map((svc) => {
                const key = `${svc.service_name}:${svc.environment}`;
                const displayTag = svc.current_tag ?? svc.image_tag;
                return (
                  <tr key={svc.id} className="hover:bg-gray-50 dark:hover:bg-gray-800/40 transition-colors">
                    <td className="px-4 py-3">
                      <div className="font-medium text-gray-900 dark:text-white">{svc.service_name}</div>
                      {svc.git_repo && (
                        <div className="flex items-center gap-1 mt-0.5 text-xs text-gray-400">
                          <GitBranch className="h-3 w-3" />{svc.git_repo}
                        </div>
                      )}
                    </td>
                    <td className="px-4 py-3">{envBadge(svc.environment)}</td>
                    <td className="px-4 py-3">
                      <div className="font-mono text-xs text-gray-700 dark:text-gray-300 flex items-center gap-1">
                        <Tag className="h-3 w-3 text-gray-400 shrink-0" />
                        <span className="truncate max-w-[220px]" title={`${svc.image_repository}:${displayTag}`}>
                          {svc.image_repository}:{displayTag}
                        </span>
                      </div>
                    </td>
                    <td className="px-4 py-3">{statusBadge(svc.status)}</td>
                    <td className="px-4 py-3 text-gray-500 dark:text-gray-400 text-xs">{relativeTime(svc.deployed_at)}</td>
                    <td className="px-4 py-3 font-mono text-xs text-gray-500 dark:text-gray-400">{svc.container_name ?? "—"}</td>
                    <td className="px-4 py-3">
                      <div className="relative">
                        <button
                          onClick={() => setOpenMenu(openMenu === key ? null : key)}
                          className="p-1.5 rounded-lg text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 hover:bg-gray-100 dark:hover:bg-gray-800 transition-colors"
                        >
                          <MoreHorizontal className="h-4 w-4" />
                        </button>
                        {openMenu === key && (
                          <>
                            <div className="fixed inset-0 z-10" onClick={() => setOpenMenu(null)} />
                            <div className="absolute right-0 top-full mt-1 w-40 bg-white dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-700 shadow-lg z-20 py-1">
                              <button
                                onClick={() => { setRedeployTarget(svc); setRedeployTag(svc.current_tag ?? svc.image_tag ?? ""); setOpenMenu(null); }}
                                className="flex items-center gap-2 w-full px-3 py-2 text-sm text-gray-700 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-800"
                              >
                                <Rocket className="h-4 w-4 text-primary-500" />Redeploy
                              </button>
                              <button
                                onClick={() => { setDeleteTarget(svc); setOpenMenu(null); }}
                                className="flex items-center gap-2 w-full px-3 py-2 text-sm text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/20"
                              >
                                <Trash2 className="h-4 w-4" />Delete
                              </button>
                            </div>
                          </>
                        )}
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      {/* Redeploy Modal */}
      {redeployTarget && (
        <div className="fixed inset-0 z-50 flex items-center justify-center">
          <div className="absolute inset-0 bg-black/50" onClick={() => { setRedeployTarget(null); setRedeployTag(""); }} />
          <div className="relative bg-white dark:bg-gray-900 rounded-xl shadow-xl w-full max-w-md mx-4 p-6">
            <div className="flex items-center gap-3 mb-4">
              <div className="p-2 bg-primary-100 dark:bg-primary-900/30 rounded-lg">
                <Rocket className="h-5 w-5 text-primary-600 dark:text-primary-400" />
              </div>
              <div>
                <h3 className="text-lg font-semibold text-gray-900 dark:text-white">
                  {redeployTarget.git_repo ? "Rebuild from Git" : "Redeploy Service"}
                </h3>
                <p className="text-sm text-gray-500 dark:text-gray-400">{redeployTarget.service_name} · {redeployTarget.environment}</p>
              </div>
            </div>
            {redeployTarget.git_repo ? (
              <div className="rounded-lg border border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-800/50 p-3 text-sm">
                <div className="flex items-center gap-2 text-gray-700 dark:text-gray-300 font-mono text-xs break-all">
                  <GitBranch className="h-4 w-4 shrink-0 text-gray-400" />
                  {redeployTarget.git_repo}{redeployTarget.git_branch ? ` @ ${redeployTarget.git_branch}` : ""}
                </div>
                <p className="text-xs text-gray-500 dark:text-gray-400 mt-2">
                  InfraPilot will re-clone, rebuild (Nixpacks / Dockerfile), then redeploy the latest commit.
                </p>
              </div>
            ) : (
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Image Tag</label>
                <input
                  type="text"
                  value={redeployTag}
                  onChange={(e) => setRedeployTag(e.target.value)}
                  placeholder={redeployTarget.image_tag || "latest"}
                  className="w-full px-3 py-2 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg text-sm font-mono text-gray-900 dark:text-white placeholder-gray-400 focus:ring-2 focus:ring-primary-500 focus:border-transparent"
                />
                <p className="text-xs text-gray-400 mt-1">Leave blank to use the currently configured tag</p>
              </div>
            )}
            {redeployMutation.isError && (
              <div className="flex items-start gap-2 p-3 mt-3 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg">
                <AlertTriangle className="h-4 w-4 text-red-600 shrink-0 mt-0.5" />
                <p className="text-sm text-red-600 dark:text-red-400">{(redeployMutation.error as Error)?.message ?? "Deploy failed"}</p>
              </div>
            )}
            <div className="flex justify-end gap-3 mt-6">
              <button onClick={() => { setRedeployTarget(null); setRedeployTag(""); redeployMutation.reset(); }} className="px-4 py-2 text-sm bg-gray-100 dark:bg-gray-800 hover:bg-gray-200 dark:hover:bg-gray-700 text-gray-700 dark:text-gray-300 rounded-lg font-medium">Cancel</button>
              <button onClick={() => redeployMutation.mutate(redeployTarget)} disabled={redeployMutation.isPending} className="inline-flex items-center gap-2 px-4 py-2 text-sm bg-primary-600 hover:bg-primary-700 text-white rounded-lg font-medium disabled:opacity-50">
                {redeployMutation.isPending ? <><RefreshCw className="h-4 w-4 animate-spin" />{redeployTarget.git_repo ? "Rebuilding..." : "Deploying..."}</> : <><Rocket className="h-4 w-4" />{redeployTarget.git_repo ? "Rebuild & Deploy" : "Deploy"}</>}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Delete Confirm */}
      <ConfirmDialog
        isOpen={!!deleteTarget}
        onClose={() => setDeleteTarget(null)}
        onConfirm={() => deleteTarget && deleteMutation.mutate(deleteTarget)}
        title={`Delete Service "${deleteTarget?.service_name}"?`}
        message="This removes the service definition. Running containers are not stopped."
        confirmText="Delete"
        variant="danger"
        isLoading={deleteMutation.isPending}
      />
    </div>
  );
}
