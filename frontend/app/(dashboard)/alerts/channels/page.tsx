"use client";

import { useState, useEffect } from "react";
import { useSearchParams, useRouter } from "next/navigation";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Settings,
  Mail,
  MessageSquare,
  Webhook,
  Trash2,
  Pencil,
  Send,
  Bell,
  X,
  CheckCircle2,
  XCircle,
  Loader2,
} from "lucide-react";
import { api, AlertChannel } from "@/lib/api";
import { formatRelativeTime, cn } from "@/lib/utils";
import { Card } from "@/components/ui/Card";
import { Badge } from "@/components/ui/Badge";
import { EmptyState } from "@/components/ui/EmptyState";
import { Spinner } from "@/components/ui/Spinner";
import { Button, Input } from "@/components/ui/page-layout";
import { FeatureGate } from "@/components/ui/FeatureGate";

const defaultForm = {
  name: "",
  channel_type: "slack" as "smtp" | "slack" | "webhook" | "discord" | "teams",
  enabled: true,
  // Slack / Discord / Teams (all use webhook_url)
  webhook_url: "",
  slack_channel: "",
  // SMTP
  smtp_host: "",
  smtp_port: 587,
  smtp_username: "",
  smtp_password: "",
  smtp_from: "",
  smtp_to: "",
  smtp_use_tls: true,
  // Webhook
  webhook_method: "POST",
  webhook_headers: "",
};

type FormState = typeof defaultForm;

/** One-line summary of a channel's destination shown in the list */
function getConfigSummary(channel: AlertChannel): string {
  const c = channel.config as Record<string, unknown>;
  switch (channel.channel_type) {
    case "slack": {
      const ch = c.channel as string;
      return ch ? `→ ${ch}` : "→ default channel";
    }
    case "smtp": {
      const to = (c.to as string[]) ?? [];
      return to.length > 0 ? `→ ${to[0]}${to.length > 1 ? ` +${to.length - 1}` : ""}` : "";
    }
    case "webhook":
      return `${c.method ?? "POST"} ${c.url ?? ""}`;
    case "discord":
    case "teams": {
      const url = (c.webhook_url as string) ?? "";
      return url ? `→ ${url.slice(0, 48)}${url.length > 48 ? "…" : ""}` : "";
    }
    default:
      return "";
  }
}

function getChannelIcon(type: string) {
  switch (type) {
    case "smtp":    return <Mail className="h-4 w-4" />;
    case "slack":
    case "discord":
    case "teams":   return <MessageSquare className="h-4 w-4" />;
    case "webhook": return <Webhook className="h-4 w-4" />;
    default:        return <Bell className="h-4 w-4" />;
  }
}

function AlertChannelsPageContent() {
  const searchParams = useSearchParams();
  const router = useRouter();
  const queryClient = useQueryClient();

  const [showModal, setShowModal] = useState(false);
  const [editingChannel, setEditingChannel] = useState<AlertChannel | null>(null);
  const [form, setForm] = useState<FormState>(defaultForm);

  // Per-channel test result: { id, success, message }
  const [testResult, setTestResult] = useState<{
    id: string;
    success: boolean;
    message: string;
  } | null>(null);

  useEffect(() => {
    if (searchParams.get("action") === "add") {
      setForm(defaultForm);
      setEditingChannel(null);
      setShowModal(true);
      router.replace("/alerts/channels");
    }
  }, [searchParams, router]);

  const { data: channels, isLoading } = useQuery({
    queryKey: ["alertChannels"],
    queryFn: () => api.getAlertChannels(),
  });

  const createMutation = useMutation({
    mutationFn: (data: Parameters<typeof api.createAlertChannel>[0]) =>
      api.createAlertChannel(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["alertChannels"] });
      setShowModal(false);
      setForm(defaultForm);
    },
  });

  const updateMutation = useMutation({
    mutationFn: ({ id, data }: { id: string; data: Parameters<typeof api.updateAlertChannel>[1] }) =>
      api.updateAlertChannel(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["alertChannels"] });
      setShowModal(false);
      setEditingChannel(null);
      setForm(defaultForm);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.deleteAlertChannel(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["alertChannels"] }),
  });

  const testMutation = useMutation({
    mutationFn: (id: string) => api.testAlertChannel(id),
    onSuccess: (data, id) => {
      setTestResult({ id, success: data.success, message: data.message });
      setTimeout(() => setTestResult(null), 6000);
    },
    onError: (err, id) => {
      const msg = err instanceof Error ? err.message : "Test failed";
      setTestResult({ id, success: false, message: msg });
      setTimeout(() => setTestResult(null), 6000);
    },
  });

  const openEdit = (channel: AlertChannel) => {
    const c = channel.config as Record<string, unknown>;
    setEditingChannel(channel);
    setForm({
      name: channel.name,
      channel_type: channel.channel_type,
      enabled: channel.enabled,
      webhook_url: (c.webhook_url as string) || (c.url as string) || "",
      slack_channel: (c.channel as string) || "",
      smtp_host: (c.host as string) || "",
      smtp_port: (c.port as number) || 587,
      smtp_username: (c.username as string) || "",
      smtp_password: "", // never prefill passwords
      smtp_from: (c.from as string) || "",
      smtp_to: ((c.to as string[]) || []).join(", "),
      smtp_use_tls: (c.use_tls as boolean) ?? true,
      webhook_method: (c.method as string) || "POST",
      webhook_headers: JSON.stringify(c.headers || {}, null, 2),
    });
    setShowModal(true);
  };

  const buildConfig = (): Record<string, unknown> => {
    switch (form.channel_type) {
      case "slack":
        return {
          webhook_url: form.webhook_url,
          ...(form.slack_channel ? { channel: form.slack_channel } : {}),
        };
      case "discord":
      case "teams":
        return { webhook_url: form.webhook_url };
      case "smtp": {
        const cfg: Record<string, unknown> = {
          host: form.smtp_host,
          port: form.smtp_port,
          from: form.smtp_from,
          to: form.smtp_to.split(",").map((e) => e.trim()).filter(Boolean),
          use_tls: form.smtp_use_tls,
        };
        if (form.smtp_username) cfg.username = form.smtp_username;
        if (form.smtp_password) cfg.password = form.smtp_password;
        return cfg;
      }
      case "webhook": {
        let headers: Record<string, string> = {};
        try {
          if (form.webhook_headers.trim()) {
            headers = JSON.parse(form.webhook_headers);
          }
        } catch {
          // ignore parse errors — validated on submit
        }
        return {
          url: form.webhook_url,
          method: form.webhook_method,
          headers,
        };
      }
    }
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();

    // Validate JSON headers for webhook
    if (form.channel_type === "webhook" && form.webhook_headers.trim()) {
      try {
        JSON.parse(form.webhook_headers);
      } catch {
        alert("Headers must be valid JSON");
        return;
      }
    }

    const data = {
      name: form.name,
      channel_type: form.channel_type,
      config: buildConfig(),
      enabled: form.enabled,
    };
    if (editingChannel) {
      updateMutation.mutate({ id: editingChannel.id, data });
    } else {
      createMutation.mutate(data);
    }
  };

  const isMutating = createMutation.isPending || updateMutation.isPending;
  const error = createMutation.error || updateMutation.error;

  return (
    <>
      <Card>
        {isLoading ? (
          <div className="flex items-center justify-center h-32">
            <Spinner size="lg" />
          </div>
        ) : channels && channels.length > 0 ? (
          <div className="divide-y divide-gray-100 dark:divide-gray-800">
            {channels.map((channel) => {
              const isTestLoading = testMutation.isPending && testMutation.variables === channel.id;
              const result = testResult?.id === channel.id ? testResult : null;

              return (
                <div
                  key={channel.id}
                  className="flex items-center justify-between p-4 hover:bg-gray-50 dark:hover:bg-gray-800/50 transition-colors"
                >
                  <div className="flex items-center gap-4 min-w-0">
                    <div
                      className={cn(
                        "p-2 rounded-lg shrink-0",
                        channel.enabled
                          ? "bg-primary-100 dark:bg-primary-900/30 text-primary-600 dark:text-primary-400"
                          : "bg-gray-100 dark:bg-gray-800 text-gray-500"
                      )}
                    >
                      {getChannelIcon(channel.channel_type)}
                    </div>
                    <div className="min-w-0">
                      <div className="flex items-center gap-2 flex-wrap">
                        <span className="text-gray-900 dark:text-white font-medium">{channel.name}</span>
                        <Badge size="sm">{channel.channel_type}</Badge>
                        {!channel.enabled && (
                          <Badge
                            size="sm"
                            className="bg-yellow-100 dark:bg-yellow-900/30 text-yellow-700 dark:text-yellow-400"
                          >
                            Disabled
                          </Badge>
                        )}
                      </div>
                      <p className="text-sm text-gray-500 mt-0.5 truncate">
                        {getConfigSummary(channel)}
                        <span className="ml-2 text-gray-400">· Added {formatRelativeTime(channel.created_at)}</span>
                      </p>
                      {/* Test result inline feedback */}
                      {result && (
                        <p
                          className={cn(
                            "text-xs mt-1 flex items-center gap-1",
                            result.success ? "text-green-600 dark:text-green-400" : "text-red-500 dark:text-red-400"
                          )}
                        >
                          {result.success ? (
                            <CheckCircle2 className="h-3.5 w-3.5" />
                          ) : (
                            <XCircle className="h-3.5 w-3.5" />
                          )}
                          {result.message}
                        </p>
                      )}
                    </div>
                  </div>
                  <div className="flex items-center gap-1 shrink-0">
                    <button
                      onClick={() => testMutation.mutate(channel.id)}
                      disabled={isTestLoading}
                      className="p-2 text-gray-400 hover:text-gray-900 dark:hover:text-white rounded-lg hover:bg-gray-100 dark:hover:bg-gray-800 disabled:opacity-50"
                      title="Send test notification"
                    >
                      {isTestLoading ? (
                        <Loader2 className="h-4 w-4 animate-spin" />
                      ) : (
                        <Send className="h-4 w-4" />
                      )}
                    </button>
                    <button
                      onClick={() => openEdit(channel)}
                      className="p-2 text-gray-400 hover:text-gray-900 dark:hover:text-white rounded-lg hover:bg-gray-100 dark:hover:bg-gray-800"
                      title="Edit"
                    >
                      <Pencil className="h-4 w-4" />
                    </button>
                    <button
                      onClick={() => {
                        if (confirm(`Delete channel "${channel.name}"?`)) {
                          deleteMutation.mutate(channel.id);
                        }
                      }}
                      className="p-2 text-gray-400 hover:text-red-500 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-800"
                      title="Delete"
                    >
                      <Trash2 className="h-4 w-4" />
                    </button>
                  </div>
                </div>
              );
            })}
          </div>
        ) : (
          <div className="py-12">
            <EmptyState
              icon={Settings}
              title="No notification channels configured"
              description="Add a channel to receive alerts"
            />
          </div>
        )}
      </Card>

      {/* Modal */}
      {showModal && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-white dark:bg-gray-900 rounded-lg p-6 w-full max-w-md border border-gray-200 dark:border-gray-800 max-h-[90vh] overflow-y-auto">
            <div className="flex items-center justify-between mb-4">
              <h2 className="text-xl font-bold text-gray-900 dark:text-white">
                {editingChannel ? "Edit Channel" : "Add Channel"}
              </h2>
              <button
                onClick={() => { setShowModal(false); setEditingChannel(null); }}
                className="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
              >
                <X className="h-5 w-5" />
              </button>
            </div>

            {error && (
              <div className="mb-4 p-3 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-700 dark:text-red-400">
                {error instanceof Error ? error.message : "An error occurred"}
              </div>
            )}

            <form onSubmit={handleSubmit} className="space-y-4">
              <Input
                label="Name"
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
                required
                placeholder="My Slack Channel"
              />
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1.5">
                  Type
                </label>
                <select
                  value={form.channel_type}
                  onChange={(e) =>
                    setForm({ ...form, channel_type: e.target.value as FormState["channel_type"] })
                  }
                  disabled={!!editingChannel}
                  className="w-full px-3 py-2 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg text-gray-900 dark:text-white focus:ring-2 focus:ring-primary-500 focus:border-transparent disabled:opacity-60"
                >
                  <option value="slack">Slack</option>
                  <option value="discord">Discord</option>
                  <option value="teams">Microsoft Teams</option>
                  <option value="smtp">Email (SMTP)</option>
                  <option value="webhook">Webhook</option>
                </select>
                {editingChannel && (
                  <p className="text-xs text-gray-500 mt-1">Channel type cannot be changed after creation.</p>
                )}
              </div>

              {/* Slack fields */}
              {form.channel_type === "slack" && (
                <>
                  <Input
                    label="Incoming Webhook URL"
                    type="url"
                    value={form.webhook_url}
                    onChange={(e) => setForm({ ...form, webhook_url: e.target.value })}
                    required
                    placeholder="https://hooks.slack.com/services/..."
                  />
                  <Input
                    label="Channel (optional)"
                    value={form.slack_channel}
                    onChange={(e) => setForm({ ...form, slack_channel: e.target.value })}
                    placeholder="#alerts"
                  />
                </>
              )}

              {/* Discord fields */}
              {form.channel_type === "discord" && (
                <Input
                  label="Discord Webhook URL"
                  type="url"
                  value={form.webhook_url}
                  onChange={(e) => setForm({ ...form, webhook_url: e.target.value })}
                  required
                  placeholder="https://discord.com/api/webhooks/..."
                />
              )}

              {/* Microsoft Teams fields */}
              {form.channel_type === "teams" && (
                <Input
                  label="Teams Incoming Webhook URL"
                  type="url"
                  value={form.webhook_url}
                  onChange={(e) => setForm({ ...form, webhook_url: e.target.value })}
                  required
                  placeholder="https://outlook.office.com/webhook/..."
                />
              )}

              {/* SMTP fields */}
              {form.channel_type === "smtp" && (
                <>
                  <div className="grid grid-cols-2 gap-4">
                    <Input
                      label="SMTP Host"
                      value={form.smtp_host}
                      onChange={(e) => setForm({ ...form, smtp_host: e.target.value })}
                      required
                      placeholder="smtp.example.com"
                    />
                    <Input
                      label="Port"
                      type="number"
                      value={form.smtp_port}
                      onChange={(e) => setForm({ ...form, smtp_port: parseInt(e.target.value) })}
                      required
                    />
                  </div>
                  <div className="grid grid-cols-2 gap-4">
                    <Input
                      label="Username (optional)"
                      value={form.smtp_username}
                      onChange={(e) => setForm({ ...form, smtp_username: e.target.value })}
                      placeholder="user@example.com"
                      autoComplete="off"
                    />
                    <Input
                      label={editingChannel ? "Password (leave blank to keep)" : "Password (optional)"}
                      type="password"
                      value={form.smtp_password}
                      onChange={(e) => setForm({ ...form, smtp_password: e.target.value })}
                      autoComplete="new-password"
                    />
                  </div>
                  <Input
                    label="From Email"
                    type="email"
                    value={form.smtp_from}
                    onChange={(e) => setForm({ ...form, smtp_from: e.target.value })}
                    required
                    placeholder="alerts@example.com"
                  />
                  <Input
                    label="To Emails (comma-separated)"
                    value={form.smtp_to}
                    onChange={(e) => setForm({ ...form, smtp_to: e.target.value })}
                    required
                    placeholder="admin@example.com, ops@example.com"
                  />
                  <div className="flex items-center gap-2">
                    <input
                      type="checkbox"
                      id="smtp-tls"
                      checked={form.smtp_use_tls}
                      onChange={(e) => setForm({ ...form, smtp_use_tls: e.target.checked })}
                      className="w-4 h-4 rounded border-gray-300 dark:border-gray-600"
                    />
                    <label htmlFor="smtp-tls" className="text-sm text-gray-700 dark:text-gray-300">
                      Use TLS <span className="text-gray-400 font-normal">(port 587 = STARTTLS, port 465 = implicit TLS)</span>
                    </label>
                  </div>
                </>
              )}

              {/* Webhook fields */}
              {form.channel_type === "webhook" && (
                <>
                  <Input
                    label="Webhook URL"
                    type="url"
                    value={form.webhook_url}
                    onChange={(e) => setForm({ ...form, webhook_url: e.target.value })}
                    required
                    placeholder="https://api.example.com/webhook"
                  />
                  <div>
                    <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1.5">
                      Method
                    </label>
                    <select
                      value={form.webhook_method}
                      onChange={(e) => setForm({ ...form, webhook_method: e.target.value })}
                      className="w-full px-3 py-2 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg text-gray-900 dark:text-white focus:ring-2 focus:ring-primary-500 focus:border-transparent"
                    >
                      <option value="POST">POST</option>
                      <option value="PUT">PUT</option>
                    </select>
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1.5">
                      Custom Headers (JSON, optional)
                    </label>
                    <textarea
                      value={form.webhook_headers}
                      onChange={(e) => setForm({ ...form, webhook_headers: e.target.value })}
                      rows={3}
                      placeholder={'{\n  "Authorization": "Bearer token"\n}'}
                      className="w-full px-3 py-2 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg text-gray-900 dark:text-white focus:ring-2 focus:ring-primary-500 focus:border-transparent font-mono text-sm"
                    />
                  </div>
                </>
              )}

              <div className="flex items-center gap-2">
                <input
                  type="checkbox"
                  id="channel-enabled"
                  checked={form.enabled}
                  onChange={(e) => setForm({ ...form, enabled: e.target.checked })}
                  className="w-4 h-4 rounded border-gray-300 dark:border-gray-600"
                />
                <label htmlFor="channel-enabled" className="text-sm text-gray-700 dark:text-gray-300">
                  Enabled
                </label>
              </div>

              <div className="flex justify-end gap-3 pt-4">
                <Button
                  type="button"
                  variant="ghost"
                  onClick={() => { setShowModal(false); setEditingChannel(null); }}
                >
                  Cancel
                </Button>
                <Button type="submit" variant="primary" disabled={isMutating}>
                  {isMutating ? (
                    <span className="flex items-center gap-2">
                      <Loader2 className="h-4 w-4 animate-spin" />
                      Saving…
                    </span>
                  ) : editingChannel ? "Save Changes" : "Create Channel"}
                </Button>
              </div>
            </form>
          </div>
        </div>
      )}
    </>
  );
}

// Alert channels (email / Slack / Discord delivery) are "connected" features
// (doc 35) — they require a free account key. Keyless CE lacks the flag, so
// FeatureGate shows the "get a free CE key" prompt; a free key unlocks them.
// The alert *rules* + *history* pages stay free (advanced_alerts is in the base).
export default function AlertChannelsPage() {
  return (
    <FeatureGate feature="email_alerts" featureLabel="Alert Channels">
      <AlertChannelsPageContent />
    </FeatureGate>
  );
}
