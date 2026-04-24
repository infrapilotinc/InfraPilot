"use client";

import { use, useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Shield,
  GitBranch,
  Package,
  AlertTriangle,
  CheckCircle,
  XCircle,
  Clock,
  Container,
  Activity,
  ExternalLink,
  FileCode,
  RotateCcw,
  RefreshCw,
} from "lucide-react";
import { api } from "@/lib/api";
import { cn } from "@/lib/utils";
import Link from "next/link";

interface DeploymentSpine {
  deployment: {
    id: string;
    service_name: string;
    environment: string;
    status: string;
    status_message?: string;
    deployed_by?: string;
    deployed_at?: string;
    created_at: string;
    updated_at: string;
  };
  source?: {
    type: string;
    git_repo?: string;
    git_branch?: string;
    git_commit?: string;
    ci_provider?: string;
    ci_pipeline_id?: string;
    ci_build_url?: string;
    webhook_event?: {
      id: string;
      webhook_id: string;
      webhook_name: string;
      provider: string;
      event_type: string;
      verified: boolean;
      created_at: string;
    };
  };
  image: {
    registry?: string;
    repository: string;
    tag?: string;
    digest?: string;
    full_image: string;
  };
  runtime?: {
    container_id?: string;
    container_name?: string;
    container_status: string;
    started_at?: string;
    proxy_configured: boolean;
    proxy_url?: string;
  };
  timeline: Array<{
    timestamp: string;
    type: string;
    title: string;
    description: string;
    status: string;
    details?: Record<string, any>;
  }>;
}

export default function DeploymentDetailPage({ params }: { params: Promise<{ id: string }> }) {
  const resolvedParams = use(params);
  const deploymentId = resolvedParams.id;
  const queryClient = useQueryClient();
  const [actionMessage, setActionMessage] = useState<{ type: "success" | "error"; text: string } | null>(null);

  // Fetch default agent
  const { data: agents } = useQuery({
    queryKey: ["agents"],
    queryFn: () => api.fetchAPI<any[]>("/agents"),
  });

  const defaultAgent = agents?.[0];

  // Fetch deployment spine
  const { data: spine, isLoading } = useQuery({
    queryKey: ["deployment-spine", deploymentId],
    queryFn: () =>
      api.fetchAPI<DeploymentSpine>(`/agents/${defaultAgent?.id}/deployments/${deploymentId}/spine`),
    enabled: !!defaultAgent?.id,
  });

  const showMessage = (type: "success" | "error", text: string) => {
    setActionMessage({ type, text });
    setTimeout(() => setActionMessage(null), 4000);
  };

  const rollbackMutation = useMutation({
    mutationFn: () => api.rollbackDeployment(defaultAgent!.id, deploymentId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["deployment-spine", deploymentId] });
      queryClient.invalidateQueries({ queryKey: ["deployments"] });
      showMessage("success", "Rollback initiated successfully.");
    },
    onError: (err: Error) => showMessage("error", err.message || "Rollback failed."),
  });

  const redeployMutation = useMutation({
    mutationFn: () => api.redeployDeployment(defaultAgent!.id, deploymentId, true, false),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["deployment-spine", deploymentId] });
      queryClient.invalidateQueries({ queryKey: ["deployments"] });
      showMessage("success", "Redeploy initiated successfully.");
    },
    onError: (err: Error) => showMessage("error", err.message || "Redeploy failed."),
  });

  if (isLoading) {
    return (
      <div className="flex h-screen items-center justify-center">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600"></div>
      </div>
    );
  }

  if (!spine) {
    return null;
  }

  const getStatusColor = (status: string) => {
    switch (status) {
      case "running":
        return "text-green-600 dark:text-green-400 bg-green-100 dark:bg-green-900/20";
      case "failed":
        return "text-red-600 dark:text-red-400 bg-red-100 dark:bg-red-900/20";
      case "pending":
      case "scanning":
      case "policy_check":
        return "text-yellow-600 dark:text-yellow-400 bg-yellow-100 dark:bg-yellow-900/20";
      default:
        return "text-gray-600 dark:text-gray-400 bg-gray-100 dark:bg-gray-800";
    }
  };

  const getTimelineIcon = (type: string, status: string) => {
    if (status === "error") return <XCircle className="w-5 h-5 text-red-600" />;
    if (status === "warning") return <AlertTriangle className="w-5 h-5 text-yellow-600" />;

    switch (type) {
      case "webhook":
      case "created":
        return <GitBranch className="w-5 h-5 text-blue-600" />;
      case "scanned":
        return <Shield className="w-5 h-5 text-purple-600" />;
      case "sbom_generated":
        return <Package className="w-5 h-5 text-indigo-600" />;
      case "policy_evaluated":
        return <FileCode className="w-5 h-5 text-orange-600" />;
      case "deployed":
        return <CheckCircle className="w-5 h-5 text-green-600" />;
      default:
        return <Activity className="w-5 h-5 text-gray-600" />;
    }
  };

  const isPending = rollbackMutation.isPending || redeployMutation.isPending;

  return (
    <div className="min-h-screen bg-gray-50 dark:bg-gray-950 p-4 lg:p-8">
      {/* Action feedback */}
      {actionMessage && (
        <div
          className={cn(
            "mb-4 px-4 py-3 rounded-lg text-sm font-medium",
            actionMessage.type === "success"
              ? "bg-green-100 dark:bg-green-900/20 text-green-700 dark:text-green-400"
              : "bg-red-100 dark:bg-red-900/20 text-red-700 dark:text-red-400"
          )}
        >
          {actionMessage.text}
        </div>
      )}

      {/* Header */}
      <div className="mb-6">
        <div className="flex items-center gap-2 text-sm text-gray-500 dark:text-gray-400 mb-2">
          <Link href="/docker/deployments" className="hover:text-primary-600">
            Deployments
          </Link>
          <span>›</span>
          <span>{spine.deployment.service_name}</span>
        </div>
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-2xl font-bold text-gray-900 dark:text-white flex items-center gap-3">
              {spine.deployment.service_name}
              <span
                className={cn(
                  "px-3 py-1 rounded-full text-sm font-medium",
                  getStatusColor(spine.deployment.status)
                )}
              >
                {spine.deployment.status}
              </span>
            </h1>
            <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
              {spine.deployment.environment} environment • {spine.deployment.id}
            </p>
          </div>
          <div className="flex items-center gap-2">
            <button
              onClick={() => rollbackMutation.mutate()}
              disabled={isPending || !defaultAgent}
              className="flex items-center gap-2 px-4 py-2 border border-gray-300 dark:border-gray-700 text-gray-700 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-800 rounded-lg transition-colors text-sm disabled:opacity-50"
            >
              <RotateCcw className={cn("w-4 h-4", rollbackMutation.isPending && "animate-spin")} />
              Rollback
            </button>
            <button
              onClick={() => redeployMutation.mutate()}
              disabled={isPending || !defaultAgent}
              className="flex items-center gap-2 px-4 py-2 bg-primary-600 hover:bg-primary-700 text-white rounded-lg transition-colors text-sm disabled:opacity-50"
            >
              <RefreshCw className={cn("w-4 h-4", redeployMutation.isPending && "animate-spin")} />
              Redeploy
            </button>
          </div>
        </div>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Main Column - Timeline & Details */}
        <div className="lg:col-span-2 space-y-6">
          {/* Source Information */}
          {spine.source && (
            <div className="bg-white dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-800 p-6">
              <h2 className="text-lg font-semibold text-gray-900 dark:text-white mb-4 flex items-center gap-2">
                <GitBranch className="w-5 h-5" />
                Source
              </h2>
              <div className="space-y-3">
                {spine.source.webhook_event && (
                  <div className="flex items-center gap-2 p-3 bg-blue-50 dark:bg-blue-900/10 rounded-lg">
                    <CheckCircle className="w-4 h-4 text-blue-600" />
                    <div className="flex-1">
                      <p className="text-sm font-medium text-gray-900 dark:text-white">
                        Triggered by {spine.source.ci_provider} webhook
                      </p>
                      <p className="text-xs text-gray-500 dark:text-gray-400">
                        {spine.source.webhook_event.webhook_name}
                        {spine.source.webhook_event.verified && " • Verified"}
                      </p>
                    </div>
                  </div>
                )}
                {spine.source.git_commit && (
                  <div className="grid grid-cols-2 gap-4 text-sm">
                    <div>
                      <p className="text-gray-500 dark:text-gray-400">Branch</p>
                      <p className="font-mono text-gray-900 dark:text-white">
                        {spine.source.git_branch}
                      </p>
                    </div>
                    <div>
                      <p className="text-gray-500 dark:text-gray-400">Commit</p>
                      <p className="font-mono text-gray-900 dark:text-white">
                        {spine.source.git_commit?.substring(0, 8)}
                      </p>
                    </div>
                  </div>
                )}
                {spine.source.ci_build_url && (
                  <a
                    href={spine.source.ci_build_url}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="flex items-center gap-2 text-sm text-primary-600 hover:text-primary-700"
                  >
                    View CI Build
                    <ExternalLink className="w-4 h-4" />
                  </a>
                )}
              </div>
            </div>
          )}

          {/* Deployment Timeline */}
          <div className="bg-white dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-800 p-6">
            <h2 className="text-lg font-semibold text-gray-900 dark:text-white mb-4 flex items-center gap-2">
              <Clock className="w-5 h-5" />
              Deployment Timeline
            </h2>
            <div className="space-y-4">
              {spine.timeline.map((event, idx) => (
                <div key={idx} className="flex gap-4">
                  <div className="flex flex-col items-center">
                    <div className="p-2 bg-gray-100 dark:bg-gray-800 rounded-full">
                      {getTimelineIcon(event.type, event.status)}
                    </div>
                    {idx < spine.timeline.length - 1 && (
                      <div className="w-px flex-1 bg-gray-200 dark:bg-gray-700 mt-2"></div>
                    )}
                  </div>
                  <div className="flex-1 pb-4">
                    <div className="flex items-center gap-2 mb-1">
                      <h3 className="text-sm font-medium text-gray-900 dark:text-white">
                        {event.title}
                      </h3>
                      <span className="text-xs text-gray-500 dark:text-gray-400">
                        {new Date(event.timestamp).toLocaleTimeString()}
                      </span>
                    </div>
                    <p className="text-sm text-gray-600 dark:text-gray-400">{event.description}</p>
                    {event.details && Object.keys(event.details).length > 0 && (
                      <div className="mt-2 p-2 bg-gray-50 dark:bg-gray-800 rounded text-xs font-mono">
                        {JSON.stringify(event.details, null, 2)}
                      </div>
                    )}
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>

        {/* Sidebar - Quick Info */}
        <div className="space-y-6">
          {/* Image Information */}
          <div className="bg-white dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-800 p-6">
            <h2 className="text-lg font-semibold text-gray-900 dark:text-white mb-4 flex items-center gap-2">
              <Container className="w-5 h-5" />
              Image
            </h2>
            <div className="space-y-3 text-sm">
              <div>
                <p className="text-gray-500 dark:text-gray-400">Repository</p>
                <p className="font-mono text-gray-900 dark:text-white break-all">
                  {spine.image.repository}
                </p>
              </div>
              {spine.image.tag && (
                <div>
                  <p className="text-gray-500 dark:text-gray-400">Tag</p>
                  <p className="font-mono text-gray-900 dark:text-white">{spine.image.tag}</p>
                </div>
              )}
              {spine.image.digest && (
                <div>
                  <p className="text-gray-500 dark:text-gray-400">Digest</p>
                  <p className="font-mono text-xs text-gray-900 dark:text-white break-all">
                    {spine.image.digest}
                  </p>
                </div>
              )}
            </div>
          </div>

          {/* Runtime Information */}
          {spine.runtime && spine.runtime.container_id && (
            <div className="bg-white dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-800 p-6">
              <h2 className="text-lg font-semibold text-gray-900 dark:text-white mb-4 flex items-center gap-2">
                <Activity className="w-5 h-5" />
                Runtime
              </h2>
              <div className="space-y-3 text-sm">
                <div>
                  <p className="text-gray-500 dark:text-gray-400">Container</p>
                  <p className="font-mono text-xs text-gray-900 dark:text-white">
                    {spine.runtime.container_name}
                  </p>
                </div>
                <div>
                  <p className="text-gray-500 dark:text-gray-400">Status</p>
                  <p className="font-medium text-gray-900 dark:text-white">
                    {spine.runtime.container_status}
                  </p>
                </div>
                {spine.runtime.proxy_configured && (
                  <div className="flex items-center gap-2 p-2 bg-green-50 dark:bg-green-900/10 rounded">
                    <CheckCircle className="w-4 h-4 text-green-600" />
                    <span className="text-sm text-green-700 dark:text-green-400">
                      Proxy Configured
                    </span>
                  </div>
                )}
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
