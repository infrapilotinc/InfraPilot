"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Bell,
  Plus,
  Trash2,
  Settings,
  Mail,
  MessageSquare,
  Webhook,
  Check,
  X,
  AlertTriangle,
  Clock,
  ToggleLeft,
  ToggleRight,
  Send,
  History,
  Pencil,
} from "lucide-react";
import { api, AlertChannel, AlertRule, AlertHistoryEntry } from "@/lib/api";
import { formatRelativeTime, cn } from "@/lib/utils";
import {
  PageLayout,
  Button,
  Tabs,
  Input,
  EmptyState,
} from "@/components/ui/page-layout";

type Tab = "channels" | "rules" | "history";

export default function AlertsPage() {
  const queryClient = useQueryClient();
  const [activeTab, setActiveTab] = useState<Tab>("channels");
  const [showChannelModal, setShowChannelModal] = useState(false);
  const [showRuleModal, setShowRuleModal] = useState(false);
  const [editingChannel, setEditingChannel] = useState<AlertChannel | null>(null);
  const [editingRule, setEditingRule] = useState<AlertRule | null>(null);

  // Channel form state
  const [channelForm, setChannelForm] = useState({
    name: "",
    channel_type: "slack" as "smtp" | "slack" | "webhook",
    enabled: true,
    // Slack config
    webhook_url: "",
    slack_channel: "",
    // SMTP config
    smtp_host: "",
    smtp_port: 587,
    smtp_from: "",
    smtp_to: "",
    // Webhook config
    webhook_method: "POST",
    webhook_headers: "",
  });

  // Rule form state
  const [ruleForm, setRuleForm] = useState({
    name: "",
    rule_type: "container_crash",
    enabled: true,
    cooldown_mins: 15,
    channels: [] as string[],
    // Conditions
    threshold: 3,
    duration_mins: 5,
    // SSL expiry specific
    warning_days: 14,
    critical_days: 7,
    // High error rate specific
    window_mins: 5,
    container_pattern: "",
  });

  // Fetch data
  const { data: channels, isLoading: channelsLoading } = useQuery({
    queryKey: ["alertChannels"],
    queryFn: () => api.getAlertChannels(),
  });

  const { data: rules, isLoading: rulesLoading } = useQuery({
    queryKey: ["alertRules"],
    queryFn: () => api.getAlertRules(),
  });

  const { data: history, isLoading: historyLoading } = useQuery({
    queryKey: ["alertHistory"],
    queryFn: () => api.getAlertHistory(50),
  });

  // Mutations
  const createChannelMutation = useMutation({
    mutationFn: (data: Parameters<typeof api.createAlertChannel>[0]) =>
      api.createAlertChannel(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["alertChannels"] });
      setShowChannelModal(false);
      resetChannelForm();
    },
  });

  const updateChannelMutation = useMutation({
    mutationFn: ({ id, data }: { id: string; data: Parameters<typeof api.updateAlertChannel>[1] }) =>
      api.updateAlertChannel(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["alertChannels"] });
      setShowChannelModal(false);
      setEditingChannel(null);
      resetChannelForm();
    },
  });

  const deleteChannelMutation = useMutation({
    mutationFn: (id: string) => api.deleteAlertChannel(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["alertChannels"] });
    },
  });

  const testChannelMutation = useMutation({
    mutationFn: (id: string) => api.testAlertChannel(id),
  });

  const createRuleMutation = useMutation({
    mutationFn: (data: Parameters<typeof api.createAlertRule>[0]) =>
      api.createAlertRule(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["alertRules"] });
      setShowRuleModal(false);
      resetRuleForm();
    },
  });

  const updateRuleMutation = useMutation({
    mutationFn: ({ id, data }: { id: string; data: Parameters<typeof api.updateAlertRule>[1] }) =>
      api.updateAlertRule(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["alertRules"] });
      setShowRuleModal(false);
      setEditingRule(null);
      resetRuleForm();
    },
  });

  const deleteRuleMutation = useMutation({
    mutationFn: (id: string) => api.deleteAlertRule(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["alertRules"] });
    },
  });

  const toggleRuleMutation = useMutation({
    mutationFn: ({ id, enabled }: { id: string; enabled: boolean }) =>
      api.updateAlertRule(id, { enabled }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["alertRules"] });
    },
  });

  // Helpers
  const resetChannelForm = () => {
    setChannelForm({
      name: "",
      channel_type: "slack",
      enabled: true,
      webhook_url: "",
      slack_channel: "",
      smtp_host: "",
      smtp_port: 587,
      smtp_from: "",
      smtp_to: "",
      webhook_method: "POST",
      webhook_headers: "",
    });
  };

  const resetRuleForm = () => {
    setRuleForm({
      name: "",
      rule_type: "container_crash",
      enabled: true,
      cooldown_mins: 15,
      channels: [],
      threshold: 3,
      duration_mins: 5,
      warning_days: 14,
      critical_days: 7,
      window_mins: 5,
      container_pattern: "",
    });
  };

  const openEditChannel = (channel: AlertChannel) => {
    setEditingChannel(channel);
    const config = channel.config as Record<string, unknown>;
    setChannelForm({
      name: channel.name,
      channel_type: channel.channel_type,
      enabled: channel.enabled,
      webhook_url: (config.webhook_url as string) || "",
      slack_channel: (config.channel as string) || "",
      smtp_host: (config.host as string) || "",
      smtp_port: (config.port as number) || 587,
      smtp_from: (config.from as string) || "",
      smtp_to: ((config.to as string[]) || []).join(", "),
      webhook_method: (config.method as string) || "POST",
      webhook_headers: JSON.stringify(config.headers || {}, null, 2),
    });
    setShowChannelModal(true);
  };

  const openEditRule = (rule: AlertRule) => {
    setEditingRule(rule);
    const conditions = rule.conditions as Record<string, unknown>;
    setRuleForm({
      name: rule.name,
      rule_type: rule.rule_type,
      enabled: rule.enabled,
      cooldown_mins: rule.cooldown_mins,
      channels: rule.channels,
      threshold: (conditions.threshold as number) || 3,
      duration_mins: (conditions.duration_mins as number) || 5,
      warning_days: (conditions.warning_days as number) || 14,
      critical_days: (conditions.critical_days as number) || 7,
      window_mins: (conditions.window_mins as number) || 5,
      container_pattern: (conditions.container_pattern as string) || "",
    });
    setShowRuleModal(true);
  };

  const buildChannelConfig = () => {
    switch (channelForm.channel_type) {
      case "slack":
        return {
          webhook_url: channelForm.webhook_url,
          channel: channelForm.slack_channel || undefined,
        };
      case "smtp":
        return {
          host: channelForm.smtp_host,
          port: channelForm.smtp_port,
          from: channelForm.smtp_from,
          to: channelForm.smtp_to.split(",").map((e) => e.trim()),
          use_tls: true,
        };
      case "webhook":
        return {
          url: channelForm.webhook_url,
          method: channelForm.webhook_method,
          headers: channelForm.webhook_headers ? JSON.parse(channelForm.webhook_headers) : {},
        };
    }
  };

  const handleChannelSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    const data = {
      name: channelForm.name,
      channel_type: channelForm.channel_type,
      config: buildChannelConfig(),
      enabled: channelForm.enabled,
    };

    if (editingChannel) {
      updateChannelMutation.mutate({ id: editingChannel.id, data });
    } else {
      createChannelMutation.mutate(data);
    }
  };

  const buildRuleConditions = () => {
    switch (ruleForm.rule_type) {
      case "ssl_expiry":
        return {
          warning_days: ruleForm.warning_days,
          critical_days: ruleForm.critical_days,
        };
      case "high_error_rate":
        return {
          threshold: ruleForm.threshold,
          window_mins: ruleForm.window_mins,
          container_pattern: ruleForm.container_pattern || undefined,
        };
      case "high_cpu":
      case "high_memory":
        return {
          threshold: ruleForm.threshold,
        };
      case "high_restart_count":
        return {
          threshold: ruleForm.threshold,
        };
      default:
        return {
          threshold: ruleForm.threshold,
          duration_mins: ruleForm.duration_mins,
        };
    }
  };

  const handleRuleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    const data = {
      name: ruleForm.name,
      rule_type: ruleForm.rule_type,
      conditions: buildRuleConditions(),
      channels: ruleForm.channels,
      cooldown_mins: ruleForm.cooldown_mins,
      enabled: ruleForm.enabled,
    };

    if (editingRule) {
      updateRuleMutation.mutate({ id: editingRule.id, data });
    } else {
      createRuleMutation.mutate(data);
    }
  };

  const getChannelIcon = (type: string) => {
    switch (type) {
      case "smtp":
        return <Mail className="h-4 w-4" />;
      case "slack":
        return <MessageSquare className="h-4 w-4" />;
      case "webhook":
        return <Webhook className="h-4 w-4" />;
      default:
        return <Bell className="h-4 w-4" />;
    }
  };

  const getSeverityColor = (severity: string) => {
    switch (severity) {
      case "critical":
        return "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400 border-red-200 dark:border-red-800";
      case "warning":
        return "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400 border-yellow-200 dark:border-yellow-800";
      case "info":
        return "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400 border-blue-200 dark:border-blue-800";
      default:
        return "bg-gray-100 text-gray-700 dark:bg-gray-800 dark:text-gray-400 border-gray-200 dark:border-gray-700";
    }
  };

  const ruleTypes = [
    { value: "container_crash", label: "Container Crash" },
    { value: "high_restart_count", label: "High Restart Count" },
    { value: "container_stopped", label: "Container Stopped" },
    { value: "high_cpu", label: "High CPU Usage" },
    { value: "high_memory", label: "High Memory Usage" },
    { value: "ssl_expiry", label: "SSL Certificate Expiring" },
    { value: "high_error_rate", label: "High Error Rate" },
  ];

  const tabs = [
    { id: "channels", label: "Channels" },
    { id: "rules", label: "Rules" },
    { id: "history", label: "History" },
  ];

  return (
    <PageLayout
      title="Alerts"
      description="Configure notification channels and alert rules"
      actions={
        activeTab !== "history" && (
          <Button
            variant="primary"
            icon={Plus}
            onClick={() => {
              if (activeTab === "channels") {
                resetChannelForm();
                setEditingChannel(null);
                setShowChannelModal(true);
              } else {
                resetRuleForm();
                setEditingRule(null);
                setShowRuleModal(true);
              }
            }}
          >
            Add {activeTab === "channels" ? "Channel" : "Rule"}
          </Button>
        )
      }
    >
      {/* Tabs */}
      <div className="mb-6">
        <Tabs tabs={tabs} activeTab={activeTab} onChange={(id) => setActiveTab(id as Tab)} />
      </div>

      {/* Channels Tab */}
      {activeTab === "channels" && (
        <div className="bg-white dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-800">
          {channelsLoading ? (
            <div className="flex items-center justify-center h-32">
              <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-500" />
            </div>
          ) : channels && channels.length > 0 ? (
            <div className="divide-y divide-gray-100 dark:divide-gray-800">
              {channels.map((channel) => (
                <div
                  key={channel.id}
                  className="flex items-center justify-between p-4 hover:bg-gray-50 dark:hover:bg-gray-800/50 transition-colors"
                >
                  <div className="flex items-center gap-4">
                    <div className={cn(
                      "p-2 rounded-lg",
                      channel.enabled ? "bg-primary-100 dark:bg-primary-900/30 text-primary-600 dark:text-primary-400" : "bg-gray-100 dark:bg-gray-800 text-gray-500"
                    )}>
                      {getChannelIcon(channel.channel_type)}
                    </div>
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="text-gray-900 dark:text-white font-medium">{channel.name}</span>
                        <span className="text-xs px-2 py-0.5 rounded bg-gray-100 dark:bg-gray-800 text-gray-500 uppercase">
                          {channel.channel_type}
                        </span>
                        {!channel.enabled && (
                          <span className="text-xs px-2 py-0.5 rounded bg-yellow-100 dark:bg-yellow-900/30 text-yellow-700 dark:text-yellow-400">
                            Disabled
                          </span>
                        )}
                      </div>
                      <p className="text-sm text-gray-500 mt-0.5">
                        Created {formatRelativeTime(channel.created_at)}
                      </p>
                    </div>
                  </div>
                  <div className="flex items-center gap-1">
                    <button
                      onClick={() => testChannelMutation.mutate(channel.id)}
                      disabled={testChannelMutation.isPending}
                      className="p-2 text-gray-400 hover:text-gray-900 dark:hover:text-white rounded-lg hover:bg-gray-100 dark:hover:bg-gray-800"
                      title="Send test"
                    >
                      <Send className="h-4 w-4" />
                    </button>
                    <button
                      onClick={() => openEditChannel(channel)}
                      className="p-2 text-gray-400 hover:text-gray-900 dark:hover:text-white rounded-lg hover:bg-gray-100 dark:hover:bg-gray-800"
                      title="Edit"
                    >
                      <Pencil className="h-4 w-4" />
                    </button>
                    <button
                      onClick={() => deleteChannelMutation.mutate(channel.id)}
                      className="p-2 text-gray-400 hover:text-red-500 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-800"
                      title="Delete"
                    >
                      <Trash2 className="h-4 w-4" />
                    </button>
                  </div>
                </div>
              ))}
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
        </div>
      )}

      {/* Rules Tab */}
      {activeTab === "rules" && (
        <div className="bg-white dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-800">
          {rulesLoading ? (
            <div className="flex items-center justify-center h-32">
              <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-500" />
            </div>
          ) : rules && rules.length > 0 ? (
            <div className="divide-y divide-gray-100 dark:divide-gray-800">
              {rules.map((rule) => (
                <div
                  key={rule.id}
                  className="flex items-center justify-between p-4 hover:bg-gray-50 dark:hover:bg-gray-800/50 transition-colors"
                >
                  <div className="flex items-center gap-4">
                    <button
                      onClick={() => toggleRuleMutation.mutate({ id: rule.id, enabled: !rule.enabled })}
                      className={cn(
                        "transition-colors",
                        rule.enabled ? "text-green-500" : "text-gray-400"
                      )}
                    >
                      {rule.enabled ? (
                        <ToggleRight className="h-6 w-6" />
                      ) : (
                        <ToggleLeft className="h-6 w-6" />
                      )}
                    </button>
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="text-gray-900 dark:text-white font-medium">{rule.name}</span>
                        <span className="text-xs px-2 py-0.5 rounded bg-gray-100 dark:bg-gray-800 text-gray-500">
                          {ruleTypes.find((t) => t.value === rule.rule_type)?.label || rule.rule_type}
                        </span>
                      </div>
                      <p className="text-sm text-gray-500 mt-0.5">
                        Cooldown: {rule.cooldown_mins} mins | {rule.channels.length} channel(s)
                      </p>
                    </div>
                  </div>
                  <div className="flex items-center gap-1">
                    <button
                      onClick={() => openEditRule(rule)}
                      className="p-2 text-gray-400 hover:text-gray-900 dark:hover:text-white rounded-lg hover:bg-gray-100 dark:hover:bg-gray-800"
                      title="Edit"
                    >
                      <Pencil className="h-4 w-4" />
                    </button>
                    <button
                      onClick={() => deleteRuleMutation.mutate(rule.id)}
                      className="p-2 text-gray-400 hover:text-red-500 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-800"
                      title="Delete"
                    >
                      <Trash2 className="h-4 w-4" />
                    </button>
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <div className="py-12">
              <EmptyState
                icon={Bell}
                title="No alert rules configured"
                description="Create a rule to start monitoring"
              />
            </div>
          )}
        </div>
      )}

      {/* History Tab */}
      {activeTab === "history" && (
        <div className="bg-white dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-800">
          {historyLoading ? (
            <div className="flex items-center justify-center h-32">
              <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-500" />
            </div>
          ) : history && history.length > 0 ? (
            <div className="divide-y divide-gray-100 dark:divide-gray-800">
              {history.map((entry) => (
                <div
                  key={entry.id}
                  className="flex items-center gap-4 p-4 hover:bg-gray-50 dark:hover:bg-gray-800/50 transition-colors"
                >
                  <div className={cn(
                    "p-2 rounded-lg",
                    entry.resolved_at ? "bg-green-100 dark:bg-green-900/30" : "bg-red-100 dark:bg-red-900/30"
                  )}>
                    {entry.resolved_at ? (
                      <Check className="h-4 w-4 text-green-600 dark:text-green-400" />
                    ) : (
                      <AlertTriangle className="h-4 w-4 text-red-600 dark:text-red-400" />
                    )}
                  </div>
                  <div className="flex-1">
                    <div className="flex items-center gap-2">
                      <span className="text-gray-900 dark:text-white font-medium">{entry.message}</span>
                      <span className={cn(
                        "text-xs px-2 py-0.5 rounded border",
                        getSeverityColor(entry.severity)
                      )}>
                        {entry.severity}
                      </span>
                    </div>
                    <div className="flex items-center gap-4 text-sm text-gray-500 mt-1">
                      {entry.rule_name && <span>Rule: {entry.rule_name}</span>}
                      {entry.agent_name && <span>Agent: {entry.agent_name}</span>}
                      <span className="flex items-center gap-1">
                        <Clock className="h-3 w-3" />
                        {formatRelativeTime(entry.triggered_at)}
                      </span>
                      {entry.resolved_at && (
                        <span className="text-green-600 dark:text-green-400">
                          Resolved {formatRelativeTime(entry.resolved_at)}
                        </span>
                      )}
                    </div>
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <div className="py-12">
              <EmptyState
                icon={History}
                title="No alert history"
                description="Triggered alerts will appear here"
              />
            </div>
          )}
        </div>
      )}

      {/* Channel Modal */}
      {showChannelModal && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-white dark:bg-gray-900 rounded-lg p-6 w-full max-w-md border border-gray-200 dark:border-gray-800 max-h-[90vh] overflow-y-auto">
            <div className="flex items-center justify-between mb-4">
              <h2 className="text-xl font-bold text-gray-900 dark:text-white">
                {editingChannel ? "Edit Channel" : "Add Channel"}
              </h2>
              <button
                onClick={() => {
                  setShowChannelModal(false);
                  setEditingChannel(null);
                }}
                className="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
              >
                <X className="h-5 w-5" />
              </button>
            </div>
            <form onSubmit={handleChannelSubmit} className="space-y-4">
              <Input
                label="Name"
                value={channelForm.name}
                onChange={(e) => setChannelForm({ ...channelForm, name: e.target.value })}
                required
                placeholder="My Slack Channel"
              />

              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1.5">
                  Type
                </label>
                <select
                  value={channelForm.channel_type}
                  onChange={(e) => setChannelForm({ ...channelForm, channel_type: e.target.value as "smtp" | "slack" | "webhook" })}
                  className="w-full px-3 py-2 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg text-gray-900 dark:text-white focus:ring-2 focus:ring-primary-500 focus:border-transparent"
                >
                  <option value="slack">Slack</option>
                  <option value="smtp">Email (SMTP)</option>
                  <option value="webhook">Webhook</option>
                </select>
              </div>

              {/* Slack Config */}
              {channelForm.channel_type === "slack" && (
                <Input
                  label="Webhook URL"
                  type="url"
                  value={channelForm.webhook_url}
                  onChange={(e) => setChannelForm({ ...channelForm, webhook_url: e.target.value })}
                  required
                  placeholder="https://hooks.slack.com/services/..."
                />
              )}

              {/* SMTP Config */}
              {channelForm.channel_type === "smtp" && (
                <>
                  <div className="grid grid-cols-2 gap-4">
                    <Input
                      label="SMTP Host"
                      value={channelForm.smtp_host}
                      onChange={(e) => setChannelForm({ ...channelForm, smtp_host: e.target.value })}
                      required
                      placeholder="smtp.example.com"
                    />
                    <Input
                      label="Port"
                      type="number"
                      value={channelForm.smtp_port}
                      onChange={(e) => setChannelForm({ ...channelForm, smtp_port: parseInt(e.target.value) })}
                      required
                    />
                  </div>
                  <Input
                    label="From Email"
                    type="email"
                    value={channelForm.smtp_from}
                    onChange={(e) => setChannelForm({ ...channelForm, smtp_from: e.target.value })}
                    required
                    placeholder="alerts@example.com"
                  />
                  <Input
                    label="To Emails (comma-separated)"
                    value={channelForm.smtp_to}
                    onChange={(e) => setChannelForm({ ...channelForm, smtp_to: e.target.value })}
                    required
                    placeholder="admin@example.com, ops@example.com"
                  />
                </>
              )}

              {/* Webhook Config */}
              {channelForm.channel_type === "webhook" && (
                <>
                  <Input
                    label="Webhook URL"
                    type="url"
                    value={channelForm.webhook_url}
                    onChange={(e) => setChannelForm({ ...channelForm, webhook_url: e.target.value })}
                    required
                    placeholder="https://api.example.com/webhook"
                  />
                  <div>
                    <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1.5">
                      Method
                    </label>
                    <select
                      value={channelForm.webhook_method}
                      onChange={(e) => setChannelForm({ ...channelForm, webhook_method: e.target.value })}
                      className="w-full px-3 py-2 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg text-gray-900 dark:text-white focus:ring-2 focus:ring-primary-500 focus:border-transparent"
                    >
                      <option value="POST">POST</option>
                      <option value="PUT">PUT</option>
                    </select>
                  </div>
                </>
              )}

              <div className="flex items-center gap-2">
                <input
                  type="checkbox"
                  id="channel-enabled"
                  checked={channelForm.enabled}
                  onChange={(e) => setChannelForm({ ...channelForm, enabled: e.target.checked })}
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
                  onClick={() => {
                    setShowChannelModal(false);
                    setEditingChannel(null);
                  }}
                >
                  Cancel
                </Button>
                <Button
                  type="submit"
                  variant="primary"
                  disabled={createChannelMutation.isPending || updateChannelMutation.isPending}
                >
                  {editingChannel ? "Save" : "Create"}
                </Button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Rule Modal */}
      {showRuleModal && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-white dark:bg-gray-900 rounded-lg p-6 w-full max-w-md border border-gray-200 dark:border-gray-800 max-h-[90vh] overflow-y-auto">
            <div className="flex items-center justify-between mb-4">
              <h2 className="text-xl font-bold text-gray-900 dark:text-white">
                {editingRule ? "Edit Rule" : "Add Rule"}
              </h2>
              <button
                onClick={() => {
                  setShowRuleModal(false);
                  setEditingRule(null);
                }}
                className="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
              >
                <X className="h-5 w-5" />
              </button>
            </div>
            <form onSubmit={handleRuleSubmit} className="space-y-4">
              <Input
                label="Name"
                value={ruleForm.name}
                onChange={(e) => setRuleForm({ ...ruleForm, name: e.target.value })}
                required
                placeholder="Container crash alert"
              />

              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1.5">
                  Rule Type
                </label>
                <select
                  value={ruleForm.rule_type}
                  onChange={(e) => setRuleForm({ ...ruleForm, rule_type: e.target.value })}
                  className="w-full px-3 py-2 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg text-gray-900 dark:text-white focus:ring-2 focus:ring-primary-500 focus:border-transparent"
                >
                  {ruleTypes.map((type) => (
                    <option key={type.value} value={type.value}>
                      {type.label}
                    </option>
                  ))}
                </select>
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1.5">
                  Notification Channels
                </label>
                {channels && channels.length > 0 ? (
                  <div className="space-y-2 p-3 bg-gray-50 dark:bg-gray-800/50 rounded-lg">
                    {channels.map((channel) => (
                      <label key={channel.id} className="flex items-center gap-2 cursor-pointer">
                        <input
                          type="checkbox"
                          checked={ruleForm.channels.includes(channel.id)}
                          onChange={(e) => {
                            if (e.target.checked) {
                              setRuleForm({ ...ruleForm, channels: [...ruleForm.channels, channel.id] });
                            } else {
                              setRuleForm({ ...ruleForm, channels: ruleForm.channels.filter((c) => c !== channel.id) });
                            }
                          }}
                          className="w-4 h-4 rounded border-gray-300 dark:border-gray-600"
                        />
                        <span className="text-sm text-gray-700 dark:text-gray-300">{channel.name}</span>
                        <span className="text-xs text-gray-500">({channel.channel_type})</span>
                      </label>
                    ))}
                  </div>
                ) : (
                  <p className="text-sm text-gray-500 p-3 bg-gray-50 dark:bg-gray-800/50 rounded-lg">
                    No channels configured. Create a channel first.
                  </p>
                )}
              </div>

              {/* Condition-specific fields */}
              {(ruleForm.rule_type === "high_cpu" ||
                ruleForm.rule_type === "high_memory" ||
                ruleForm.rule_type === "high_restart_count" ||
                ruleForm.rule_type === "high_error_rate") && (
                <Input
                  label={`Threshold ${ruleForm.rule_type === "high_cpu" || ruleForm.rule_type === "high_memory" ? "(%)" :
                            ruleForm.rule_type === "high_error_rate" ? "(errors/min)" : "(restarts)"}`}
                  type="number"
                  value={ruleForm.threshold}
                  onChange={(e) => setRuleForm({ ...ruleForm, threshold: parseFloat(e.target.value) })}
                />
              )}

              {ruleForm.rule_type === "high_error_rate" && (
                <>
                  <Input
                    label="Time Window (minutes)"
                    type="number"
                    value={ruleForm.window_mins}
                    onChange={(e) => setRuleForm({ ...ruleForm, window_mins: parseInt(e.target.value) })}
                  />
                  <Input
                    label="Container Pattern (optional)"
                    value={ruleForm.container_pattern}
                    onChange={(e) => setRuleForm({ ...ruleForm, container_pattern: e.target.value })}
                    placeholder="e.g., nginx, api-"
                  />
                </>
              )}

              {ruleForm.rule_type === "ssl_expiry" && (
                <div className="grid grid-cols-2 gap-4">
                  <Input
                    label="Warning (days)"
                    type="number"
                    value={ruleForm.warning_days}
                    onChange={(e) => setRuleForm({ ...ruleForm, warning_days: parseInt(e.target.value) })}
                  />
                  <Input
                    label="Critical (days)"
                    type="number"
                    value={ruleForm.critical_days}
                    onChange={(e) => setRuleForm({ ...ruleForm, critical_days: parseInt(e.target.value) })}
                  />
                </div>
              )}

              <Input
                label="Cooldown (minutes)"
                type="number"
                value={ruleForm.cooldown_mins}
                onChange={(e) => setRuleForm({ ...ruleForm, cooldown_mins: parseInt(e.target.value) })}
              />

              <div className="flex items-center gap-2">
                <input
                  type="checkbox"
                  id="rule-enabled"
                  checked={ruleForm.enabled}
                  onChange={(e) => setRuleForm({ ...ruleForm, enabled: e.target.checked })}
                  className="w-4 h-4 rounded border-gray-300 dark:border-gray-600"
                />
                <label htmlFor="rule-enabled" className="text-sm text-gray-700 dark:text-gray-300">
                  Enabled
                </label>
              </div>

              <div className="flex justify-end gap-3 pt-4">
                <Button
                  type="button"
                  variant="ghost"
                  onClick={() => {
                    setShowRuleModal(false);
                    setEditingRule(null);
                  }}
                >
                  Cancel
                </Button>
                <Button
                  type="submit"
                  variant="primary"
                  disabled={createRuleMutation.isPending || updateRuleMutation.isPending}
                >
                  {editingRule ? "Save" : "Create"}
                </Button>
              </div>
            </form>
          </div>
        </div>
      )}
    </PageLayout>
  );
}
