"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Webhook,
  GitBranch,
  Clock,
  Plus,
  Copy,
  CheckCircle,
  XCircle,
  Trash2,
  Settings,
  Key,
  AlertCircle,
} from "lucide-react";
import { api } from "@/lib/api";
import { cn } from "@/lib/utils";
import { FeatureGate } from "@/components/ui/FeatureGate";
import { PageHeader } from "@/components/ui/PageHeader";
import { Breadcrumb } from "@/components/ui/Breadcrumb";
import { StatCard, MetricsGrid } from "@/components/ui/StatCard";
import { Table } from "@/components/ui/Table";
import { SlideOver } from "@/components/ui/SlideOver";
import { StatusIndicator } from "@/components/ui/StatusIndicator";
import { Badge } from "@/components/ui/Badge";
import { Timeline } from "@/components/ui/Timeline";
import { EmptyState } from "@/components/ui/EmptyState";

interface WebhookConfig {
  id: string;
  name: string;
  provider: "github" | "gitlab" | "jenkins" | "generic";
  service_name: string;
  environment: "dev" | "staging" | "prod";
  enabled: boolean;
  webhook_url: string;
  created_at: string;
  last_used_at?: string;
}

interface WebhookEvent {
  id: string;
  webhook_id: string;
  provider: string;
  event_type: string;
  verified: boolean;
  processed: boolean;
  deployment_id?: string;
  error?: string;
  created_at: string;
  processed_at?: string;
}

interface CreateWebhookRequest {
  name: string;
  provider: "github" | "gitlab" | "jenkins" | "generic";
  service_name: string;
  environment: "dev" | "staging" | "prod";
}

interface CreateWebhookResponse {
  id: string;
  name: string;
  provider: string;
  service_name: string;
  environment: string;
  enabled: boolean;
  secret: string;
  webhook_url: string;
  created_at: string;
}

const providerIcons: Record<string, string> = {
  github: "🐙",
  gitlab: "🦊",
  jenkins: "🤖",
  generic: "⚡",
};

const providerColors: Record<string, string> = {
  github: "text-gray-900 dark:text-gray-100",
  gitlab: "text-orange-600 dark:text-orange-400",
  jenkins: "text-red-600 dark:text-red-400",
  generic: "text-blue-600 dark:text-blue-400",
};

function WebhooksPageContent() {
  const [selectedWebhook, setSelectedWebhook] = useState<WebhookConfig | null>(null);
  const [copiedSecret, setCopiedSecret] = useState(false);
  const [copiedURL, setCopiedURL] = useState(false);
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [createdWebhook, setCreatedWebhook] = useState<CreateWebhookResponse | null>(null);
  const [formData, setFormData] = useState<CreateWebhookRequest>({
    name: "",
    provider: "github",
    service_name: "",
    environment: "dev",
  });
  const [formError, setFormError] = useState<string | null>(null);

  const queryClient = useQueryClient();

  // Fetch default agent
  const { data: agents } = useQuery({
    queryKey: ["agents"],
    queryFn: () => api.fetchAPI<any[]>("/agents"),
  });

  const defaultAgent = agents?.[0];

  // Fetch webhooks
  const { data: webhooks, isLoading } = useQuery({
    queryKey: ["webhooks", defaultAgent?.id],
    queryFn: () =>
      api.fetchAPI<WebhookConfig[]>(`/agents/${defaultAgent?.id}/webhooks`),
    enabled: !!defaultAgent?.id,
  });

  // Fetch webhook events
  const { data: webhookEvents } = useQuery({
    queryKey: ["webhook-events", selectedWebhook?.id],
    queryFn: () =>
      api.fetchAPI<WebhookEvent[]>(
        `/agents/${defaultAgent?.id}/webhooks/${selectedWebhook?.id}/events`
      ),
    enabled: !!defaultAgent?.id && !!selectedWebhook?.id,
  });

  // Create webhook mutation
  const createWebhookMutation = useMutation({
    mutationFn: async (data: CreateWebhookRequest) => {
      return api.fetchAPI<CreateWebhookResponse>(
        `/agents/${defaultAgent?.id}/webhooks`,
        {
          method: "POST",
          body: JSON.stringify(data),
        }
      );
    },
    onSuccess: (data) => {
      setCreatedWebhook(data);
      setFormError(null);
      queryClient.invalidateQueries({ queryKey: ["webhooks", defaultAgent?.id] });
    },
    onError: (error: Error) => {
      setFormError(error.message || "Failed to create webhook");
    },
  });

  // Handle form submission
  const handleCreateWebhook = (e: React.FormEvent) => {
    e.preventDefault();
    setFormError(null);

    // Validate
    if (!formData.name.trim()) {
      setFormError("Name is required");
      return;
    }
    if (!formData.service_name.trim()) {
      setFormError("Service name is required");
      return;
    }

    createWebhookMutation.mutate(formData);
  };

  // Reset form and close modal
  const handleCloseCreate = () => {
    setIsCreateOpen(false);
    setCreatedWebhook(null);
    setFormError(null);
    setFormData({
      name: "",
      provider: "github",
      service_name: "",
      environment: "dev",
    });
  };

  // Calculate statistics
  const stats = webhooks
    ? {
        total: webhooks.length,
        enabled: webhooks.filter((w) => w.enabled).length,
        deliveryRate: webhooks.length > 0
          ? Math.round((webhooks.filter((w) => w.last_used_at).length / webhooks.length) * 100)
          : 0,
        failures: webhookEvents?.filter((e) => e.error).length || 0,
      }
    : { total: 0, enabled: 0, deliveryRate: 0, failures: 0 };

  const copyToClipboard = (text: string, type: "secret" | "url") => {
    navigator.clipboard.writeText(text);
    if (type === "secret") {
      setCopiedSecret(true);
      setTimeout(() => setCopiedSecret(false), 2000);
    } else {
      setCopiedURL(true);
      setTimeout(() => setCopiedURL(false), 2000);
    }
  };

  const getEventStatus = (event: WebhookEvent) => {
    if (event.processed) return "healthy";
    if (event.error) return "critical";
    return "warning";
  };

  const getEnvironmentVariant = (env: string): "status" => "status";
  const getEnvironmentStatus = (env: string) => {
    if (env === "prod") return "critical";
    if (env === "staging") return "warning";
    return "healthy";
  };

  // Table columns
  const columns = [
    {
      key: "provider",
      header: "Provider",
      width: "80px",
      render: (value: string) => (
        <span className="text-2xl">{providerIcons[value]}</span>
      ),
    },
    {
      key: "name",
      header: "Webhook Name",
      render: (value: string, row: WebhookConfig) => (
        <div>
          <div className="font-medium text-gray-900 dark:text-white">{value}</div>
          <div className="flex items-center gap-2 mt-1">
            <GitBranch className="h-3 w-3 text-gray-400" />
            <span className="text-xs text-gray-500 dark:text-gray-400">
              {row.service_name}
            </span>
          </div>
        </div>
      ),
    },
    {
      key: "environment",
      header: "Environment",
      render: (value: string) => (
        <Badge
          variant={getEnvironmentVariant(value)}
          status={getEnvironmentStatus(value)}
          size="sm"
        >
          {value}
        </Badge>
      ),
    },
    {
      key: "enabled",
      header: "Status",
      render: (value: boolean) => (
        <StatusIndicator
          status={value ? "healthy" : "degraded"}
          label={value ? "Enabled" : "Disabled"}
          size="sm"
        />
      ),
    },
    {
      key: "last_used_at",
      header: "Last Used",
      align: "right" as const,
      render: (value: string) => (
        value ? (
          <div className="flex items-center justify-end gap-1 text-sm">
            <Clock className="h-3 w-3 text-gray-400" />
            <span>{new Date(value).toLocaleDateString()}</span>
          </div>
        ) : (
          <span className="text-xs text-gray-400">Never used</span>
        )
      ),
    },
  ];

  return (
    <div className="space-y-6">
      <PageHeader
        title="CI/CD Webhooks"
        description="Manage webhook integrations for automated deployments"
        breadcrumbs={
          <Breadcrumb
            items={[
              { label: "Govern", href: "/" },
              { label: "Webhooks", current: true },
            ]}
          />
        }
        action={
          <button
            onClick={() => setIsCreateOpen(true)}
            className="px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700 transition-colors flex items-center gap-2"
          >
            <Plus className="h-4 w-4" />
            Create Webhook
          </button>
        }
      />

      {/* Metrics */}
      <MetricsGrid columns={4} className="mb-6">
        <StatCard
          label="Total Webhooks"
          value={stats.total}
          icon={Webhook}
          iconColor="text-primary-600 dark:text-primary-400"
        />
        <StatCard
          label="Enabled"
          value={stats.enabled}
          icon={CheckCircle}
          iconColor="text-green-600 dark:text-green-400"
        />
        <StatCard
          label="Delivery Rate"
          value={`${stats.deliveryRate}%`}
          icon={GitBranch}
          iconColor="text-blue-600 dark:text-blue-400"
        />
        <StatCard
          label="Failures"
          value={stats.failures}
          icon={XCircle}
          iconColor="text-red-600 dark:text-red-400"
        />
      </MetricsGrid>

      {/* Webhooks Table */}
      {isLoading ? (
        <div className="flex items-center justify-center h-32">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-500" />
        </div>
      ) : webhooks && webhooks.length > 0 ? (
        <Table
          columns={columns}
          data={webhooks}
          keyExtractor={(row) => row.id}
          onRowClick={(row) => setSelectedWebhook(row)}
          selectedRows={selectedWebhook ? new Set([selectedWebhook.id]) : undefined}
          hoverable
        />
      ) : (
        <EmptyState
          icon={Webhook}
          title="No Webhooks Configured"
          description="Create a webhook to enable automated deployments from your CI/CD pipeline"
          action={
            <button
              onClick={() => setIsCreateOpen(true)}
              className="px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700 transition-colors flex items-center gap-2"
            >
              <Plus className="h-4 w-4" />
              Create Your First Webhook
            </button>
          }
        />
      )}

      {/* SlideOver for webhook details */}
      <SlideOver
        isOpen={!!selectedWebhook}
        onClose={() => setSelectedWebhook(null)}
        size="lg"
      >
        {selectedWebhook && (
          <>
            <SlideOver.Header onClose={() => setSelectedWebhook(null)}>
              <div>
                <div className="flex items-center gap-2">
                  <span className="text-2xl">{providerIcons[selectedWebhook.provider]}</span>
                  <h2 className="text-lg font-semibold text-gray-900 dark:text-white">
                    {selectedWebhook.name}
                  </h2>
                </div>
                <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
                  {selectedWebhook.provider} webhook
                </p>
              </div>
            </SlideOver.Header>

            <SlideOver.Body>
              <div className="space-y-6">
                {/* Configuration */}
                <div>
                  <h3 className="text-sm font-semibold text-gray-900 dark:text-white mb-3">
                    Configuration
                  </h3>
                  <div className="space-y-3">
                    <div className="flex justify-between">
                      <span className="text-sm text-gray-500 dark:text-gray-400">Service</span>
                      <span className="text-sm text-gray-900 dark:text-white font-medium">
                        {selectedWebhook.service_name}
                      </span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-sm text-gray-500 dark:text-gray-400">Environment</span>
                      <Badge
                        variant={getEnvironmentVariant(selectedWebhook.environment)}
                        status={getEnvironmentStatus(selectedWebhook.environment)}
                        size="sm"
                      >
                        {selectedWebhook.environment}
                      </Badge>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-sm text-gray-500 dark:text-gray-400">Status</span>
                      <StatusIndicator
                        status={selectedWebhook.enabled ? "healthy" : "degraded"}
                        label={selectedWebhook.enabled ? "Enabled" : "Disabled"}
                        size="sm"
                      />
                    </div>
                    <div className="flex justify-between">
                      <span className="text-sm text-gray-500 dark:text-gray-400">Created</span>
                      <span className="text-sm text-gray-900 dark:text-white">
                        {new Date(selectedWebhook.created_at).toLocaleString()}
                      </span>
                    </div>
                    {selectedWebhook.last_used_at && (
                      <div className="flex justify-between">
                        <span className="text-sm text-gray-500 dark:text-gray-400">Last Used</span>
                        <span className="text-sm text-gray-900 dark:text-white">
                          {new Date(selectedWebhook.last_used_at).toLocaleString()}
                        </span>
                      </div>
                    )}
                  </div>
                </div>

                {/* Webhook URL */}
                <div>
                  <h3 className="text-sm font-semibold text-gray-900 dark:text-white mb-3">
                    Webhook URL
                  </h3>
                  <div className="space-y-2">
                    <div className="flex items-center gap-2">
                      <input
                        type="text"
                        readOnly
                        value={`${window.location.origin}${selectedWebhook.webhook_url}`}
                        className="flex-1 px-3 py-2 text-sm bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg text-gray-900 dark:text-white"
                      />
                      <button
                        onClick={() =>
                          copyToClipboard(
                            `${window.location.origin}${selectedWebhook.webhook_url}`,
                            "url"
                          )
                        }
                        className="p-2 text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-white hover:bg-gray-100 dark:hover:bg-gray-800 rounded-lg"
                      >
                        {copiedURL ? (
                          <CheckCircle className="w-4 h-4 text-green-600" />
                        ) : (
                          <Copy className="w-4 h-4" />
                        )}
                      </button>
                    </div>
                    <p className="text-xs text-gray-500 dark:text-gray-400">
                      Configure this URL in your CI/CD provider to trigger deployments
                    </p>
                  </div>
                </div>

                {/* Recent Events Timeline */}
                {webhookEvents && webhookEvents.length > 0 && (
                  <div>
                    <h3 className="text-sm font-semibold text-gray-900 dark:text-white mb-4">
                      Recent Events
                    </h3>
                    <Timeline>
                      {webhookEvents.slice(0, 5).map((event) => {
                        const statusIcon = event.processed ? CheckCircle : event.error ? XCircle : Clock;
                        const statusColor = event.processed
                          ? "text-green-600 dark:text-green-400"
                          : event.error
                          ? "text-red-600 dark:text-red-400"
                          : "text-yellow-600 dark:text-yellow-400";

                        return (
                          <Timeline.Item
                            key={event.id}
                            icon={statusIcon}
                            iconColor={statusColor}
                            title={event.event_type || "Deployment"}
                            timestamp={new Date(event.created_at).toLocaleString()}
                            description={
                              <div className="space-y-2">
                                <div className="flex items-center gap-2">
                                  <Badge
                                    variant="status"
                                    status={getEventStatus(event)}
                                    size="sm"
                                  >
                                    {event.processed ? "Processed" : event.error ? "Failed" : "Pending"}
                                  </Badge>
                                  {event.verified && (
                                    <Badge size="sm">
                                      <CheckCircle className="h-3 w-3" />
                                      Verified
                                    </Badge>
                                  )}
                                </div>
                                {event.error && (
                                  <p className="text-xs text-red-600 dark:text-red-400">
                                    {event.error}
                                  </p>
                                )}
                              </div>
                            }
                          />
                        );
                      })}
                    </Timeline>
                  </div>
                )}

                {/* Actions */}
                <div>
                  <h3 className="text-sm font-semibold text-gray-900 dark:text-white mb-3">
                    Actions
                  </h3>
                  <div className="space-y-2">
                    <button className="w-full flex items-center gap-2 px-4 py-2 text-sm text-gray-700 dark:text-gray-300 bg-gray-100 dark:bg-gray-800 hover:bg-gray-200 dark:hover:bg-gray-700 rounded-lg transition-colors">
                      <Settings className="w-4 h-4" />
                      {selectedWebhook.enabled ? "Disable Webhook" : "Enable Webhook"}
                    </button>
                    <button className="w-full flex items-center gap-2 px-4 py-2 text-sm text-red-700 dark:text-red-400 bg-red-100 dark:bg-red-900/30 hover:bg-red-200 dark:hover:bg-red-900/50 rounded-lg transition-colors">
                      <Trash2 className="w-4 h-4" />
                      Delete Webhook
                    </button>
                  </div>
                </div>
              </div>
            </SlideOver.Body>
          </>
        )}
      </SlideOver>

      {/* Create Webhook SlideOver */}
      <SlideOver
        isOpen={isCreateOpen}
        onClose={handleCloseCreate}
        size="lg"
      >
        <SlideOver.Header onClose={handleCloseCreate}>
          <div>
            <h2 className="text-lg font-semibold text-gray-900 dark:text-white">
              {createdWebhook ? "Webhook Created" : "Create Webhook"}
            </h2>
            <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
              {createdWebhook
                ? "Save the secret below - it won't be shown again"
                : "Configure a webhook for automated deployments"}
            </p>
          </div>
        </SlideOver.Header>

        <SlideOver.Body>
          {createdWebhook ? (
            /* Success State - Show webhook URL and secret */
            <div className="space-y-6">
              <div className="p-4 bg-green-50 dark:bg-green-900/20 border border-green-200 dark:border-green-800 rounded-lg">
                <div className="flex items-center gap-2 text-green-800 dark:text-green-200">
                  <CheckCircle className="h-5 w-5" />
                  <span className="font-medium">Webhook created successfully!</span>
                </div>
              </div>

              {/* Webhook URL */}
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                  Webhook URL
                </label>
                <div className="flex items-center gap-2">
                  <input
                    type="text"
                    readOnly
                    value={`${typeof window !== 'undefined' ? window.location.origin : ''}${createdWebhook.webhook_url}`}
                    className="flex-1 px-3 py-2 text-sm bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg text-gray-900 dark:text-white font-mono"
                  />
                  <button
                    onClick={() => copyToClipboard(
                      `${typeof window !== 'undefined' ? window.location.origin : ''}${createdWebhook.webhook_url}`,
                      "url"
                    )}
                    className="p-2 text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-white hover:bg-gray-100 dark:hover:bg-gray-800 rounded-lg"
                  >
                    {copiedURL ? (
                      <CheckCircle className="w-4 h-4 text-green-600" />
                    ) : (
                      <Copy className="w-4 h-4" />
                    )}
                  </button>
                </div>
                <p className="text-xs text-gray-500 dark:text-gray-400 mt-1">
                  Add this URL to your CI/CD provider
                </p>
              </div>

              {/* Secret */}
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                  <span className="flex items-center gap-2">
                    <Key className="h-4 w-4" />
                    Webhook Secret
                  </span>
                </label>
                <div className="flex items-center gap-2">
                  <input
                    type="text"
                    readOnly
                    value={createdWebhook.secret}
                    className="flex-1 px-3 py-2 text-sm bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-700 rounded-lg text-gray-900 dark:text-white font-mono"
                  />
                  <button
                    onClick={() => copyToClipboard(createdWebhook.secret, "secret")}
                    className="p-2 text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-white hover:bg-gray-100 dark:hover:bg-gray-800 rounded-lg"
                  >
                    {copiedSecret ? (
                      <CheckCircle className="w-4 h-4 text-green-600" />
                    ) : (
                      <Copy className="w-4 h-4" />
                    )}
                  </button>
                </div>
                <div className="flex items-start gap-2 mt-2 p-2 bg-yellow-50 dark:bg-yellow-900/20 rounded-lg">
                  <AlertCircle className="h-4 w-4 text-yellow-600 dark:text-yellow-400 mt-0.5 flex-shrink-0" />
                  <p className="text-xs text-yellow-800 dark:text-yellow-200">
                    Copy and save this secret now. For security reasons, it cannot be retrieved later.
                  </p>
                </div>
              </div>

              {/* Configuration Summary */}
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                  Configuration
                </label>
                <div className="space-y-2 text-sm">
                  <div className="flex justify-between py-2 border-b border-gray-100 dark:border-gray-800">
                    <span className="text-gray-500 dark:text-gray-400">Provider</span>
                    <span className="text-gray-900 dark:text-white flex items-center gap-2">
                      <span>{providerIcons[createdWebhook.provider]}</span>
                      {createdWebhook.provider}
                    </span>
                  </div>
                  <div className="flex justify-between py-2 border-b border-gray-100 dark:border-gray-800">
                    <span className="text-gray-500 dark:text-gray-400">Service</span>
                    <span className="text-gray-900 dark:text-white">{createdWebhook.service_name}</span>
                  </div>
                  <div className="flex justify-between py-2">
                    <span className="text-gray-500 dark:text-gray-400">Environment</span>
                    <Badge
                      variant="status"
                      status={createdWebhook.environment === "prod" ? "critical" : createdWebhook.environment === "staging" ? "warning" : "healthy"}
                      size="sm"
                    >
                      {createdWebhook.environment}
                    </Badge>
                  </div>
                </div>
              </div>

              <button
                onClick={handleCloseCreate}
                className="w-full px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700 transition-colors"
              >
                Done
              </button>
            </div>
          ) : (
            /* Form State */
            <form onSubmit={handleCreateWebhook} className="space-y-6">
              {formError && (
                <div className="p-4 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg">
                  <div className="flex items-center gap-2 text-red-800 dark:text-red-200">
                    <XCircle className="h-5 w-5" />
                    <span>{formError}</span>
                  </div>
                </div>
              )}

              {/* Name */}
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                  Webhook Name
                </label>
                <input
                  type="text"
                  value={formData.name}
                  onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                  placeholder="e.g., my-app-prod-deploy"
                  className="w-full px-3 py-2 text-sm bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg text-gray-900 dark:text-white placeholder-gray-400 focus:ring-2 focus:ring-primary-500 focus:border-transparent"
                />
              </div>

              {/* Provider */}
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                  CI/CD Provider
                </label>
                <div className="grid grid-cols-2 gap-2">
                  {(["github", "gitlab", "jenkins", "generic"] as const).map((provider) => (
                    <button
                      key={provider}
                      type="button"
                      onClick={() => setFormData({ ...formData, provider })}
                      className={cn(
                        "px-4 py-3 rounded-lg border text-sm font-medium transition-colors flex items-center gap-2",
                        formData.provider === provider
                          ? "border-primary-500 bg-primary-50 dark:bg-primary-900/20 text-primary-700 dark:text-primary-300"
                          : "border-gray-200 dark:border-gray-700 hover:bg-gray-50 dark:hover:bg-gray-800 text-gray-700 dark:text-gray-300"
                      )}
                    >
                      <span className="text-xl">{providerIcons[provider]}</span>
                      <span className="capitalize">{provider}</span>
                    </button>
                  ))}
                </div>
              </div>

              {/* Service Name */}
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                  Service Name
                </label>
                <input
                  type="text"
                  value={formData.service_name}
                  onChange={(e) => setFormData({ ...formData, service_name: e.target.value })}
                  placeholder="e.g., my-app, api-server"
                  className="w-full px-3 py-2 text-sm bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg text-gray-900 dark:text-white placeholder-gray-400 focus:ring-2 focus:ring-primary-500 focus:border-transparent"
                />
                <p className="text-xs text-gray-500 dark:text-gray-400 mt-1">
                  This name will be used to identify deployments
                </p>
              </div>

              {/* Environment */}
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                  Target Environment
                </label>
                <div className="grid grid-cols-3 gap-2">
                  {(["dev", "staging", "prod"] as const).map((env) => (
                    <button
                      key={env}
                      type="button"
                      onClick={() => setFormData({ ...formData, environment: env })}
                      className={cn(
                        "px-4 py-2 rounded-lg border text-sm font-medium transition-colors",
                        formData.environment === env
                          ? env === "prod"
                            ? "border-red-500 bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-300"
                            : env === "staging"
                            ? "border-yellow-500 bg-yellow-50 dark:bg-yellow-900/20 text-yellow-700 dark:text-yellow-300"
                            : "border-green-500 bg-green-50 dark:bg-green-900/20 text-green-700 dark:text-green-300"
                          : "border-gray-200 dark:border-gray-700 hover:bg-gray-50 dark:hover:bg-gray-800 text-gray-700 dark:text-gray-300"
                      )}
                    >
                      {env}
                    </button>
                  ))}
                </div>
              </div>

              {/* Submit */}
              <div className="pt-4">
                <button
                  type="submit"
                  disabled={createWebhookMutation.isPending}
                  className="w-full px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center justify-center gap-2"
                >
                  {createWebhookMutation.isPending ? (
                    <>
                      <div className="animate-spin rounded-full h-4 w-4 border-2 border-white border-t-transparent" />
                      Creating...
                    </>
                  ) : (
                    <>
                      <Plus className="h-4 w-4" />
                      Create Webhook
                    </>
                  )}
                </button>
              </div>
            </form>
          )}
        </SlideOver.Body>
      </SlideOver>
    </div>
  );
}

// Webhooks is a "connected" feature (doc 35): it requires a free account key.
// Anonymous/keyless CE lacks the "webhooks" flag, so FeatureGate shows the
// "get a free CE key" prompt; a free key unlocks it.
export default function WebhooksPage() {
  return (
    <FeatureGate feature="webhooks" featureLabel="Webhooks">
      <WebhooksPageContent />
    </FeatureGate>
  );
}
