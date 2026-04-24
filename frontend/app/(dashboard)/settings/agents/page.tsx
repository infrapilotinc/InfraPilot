"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Server,
  Plus,
  Trash2,
  Copy,
  Check,
  X,
  Circle,
  Terminal,
  Clock,
  Activity,
  AlertTriangle,
  ExternalLink,
} from "lucide-react";
import { api, Agent } from "@/lib/api";
import { StatCard, MetricsGrid } from "@/components/ui/StatCard";
import { Card } from "@/components/ui/Card";
import { EmptyState } from "@/components/ui/EmptyState";
import { Spinner } from "@/components/ui/Spinner";

const statusConfig: Record<Agent["status"], { label: string; color: string; dot: string }> = {
  active: { label: "Active", color: "text-green-600 dark:text-green-400", dot: "bg-green-500" },
  pending: { label: "Pending", color: "text-yellow-600 dark:text-yellow-400", dot: "bg-yellow-500" },
  offline: { label: "Offline", color: "text-gray-500 dark:text-gray-400", dot: "bg-gray-400" },
};

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);
  const copy = () => {
    navigator.clipboard.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };
  return (
    <button
      onClick={copy}
      className="p-1 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 transition-colors"
      title="Copy"
    >
      {copied ? <Check className="h-4 w-4 text-green-500" /> : <Copy className="h-4 w-4" />}
    </button>
  );
}

export default function SettingsAgentsPage() {
  const queryClient = useQueryClient();
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [agentName, setAgentName] = useState("");
  const [createdAgent, setCreatedAgent] = useState<{ id: string; name: string; enrollment_token: string } | null>(null);
  const [deleteConfirm, setDeleteConfirm] = useState<Agent | null>(null);

  const { data: agents, isLoading } = useQuery({
    queryKey: ["agents"],
    queryFn: () => api.getAgents(),
    refetchInterval: 10000,
  });

  const { data: license } = useQuery({
    queryKey: ["licenseTierInfo"],
    queryFn: () => api.getLicenseTierInfo(),
  });

  const createMutation = useMutation({
    mutationFn: (name: string) => api.createAgent(name),
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: ["agents"] });
      setShowCreateModal(false);
      setAgentName("");
      if (data.enrollment_token) {
        setCreatedAgent({ id: data.id, name: data.name, enrollment_token: data.enrollment_token });
      }
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.deleteAgent(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["agents"] });
      setDeleteConfirm(null);
    },
  });

  const total = agents?.length || 0;
  const active = agents?.filter((a) => a.status === "active").length || 0;
  const pending = agents?.filter((a) => a.status === "pending").length || 0;
  const offline = agents?.filter((a) => a.status === "offline").length || 0;

  const maxAgents = license?.max_agents ?? 1;
  const atLimit = total >= maxAgents;

  const installOneliner = createdAgent
    ? `curl -fsSL https://infrapilot.org/agent.sh | DASHBOARD_URL=${typeof window !== "undefined" ? window.location.origin : "https://infrapilot.org"} ENROLLMENT_TOKEN=${createdAgent.enrollment_token} sudo -E bash`
    : "";

  const configureCmd = createdAgent
    ? `sudo infrapilot-agent configure \\\n  --dashboard-url ${typeof window !== "undefined" ? window.location.origin : "https://infrapilot.org"} \\\n  --token ${createdAgent.enrollment_token} \\\n  --install-service\n\nsudo systemctl enable --now infrapilot-agent`
    : "";

  return (
    <div className="space-y-6">
      {/* Upgrade banner — shown when at agent limit */}
      {atLimit && (
        <div className="flex items-start gap-3 px-4 py-3 bg-amber-50 dark:bg-amber-900/10 border border-amber-200 dark:border-amber-700 rounded-lg">
          <AlertTriangle className="h-5 w-5 text-amber-600 dark:text-amber-400 flex-shrink-0 mt-0.5" />
          <div className="flex-1 text-sm">
            <span className="font-medium text-amber-800 dark:text-amber-300">
              Community Edition supports {maxAgents} agent.{" "}
            </span>
            <a
              href="https://infrapilot.org/enterprise"
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-1 text-amber-700 dark:text-amber-400 underline underline-offset-2 hover:text-amber-900 dark:hover:text-amber-200 font-medium"
            >
              Upgrade to Professional
              <ExternalLink className="h-3.5 w-3.5" />
            </a>
            <span className="text-amber-700 dark:text-amber-400"> for unlimited agents.</span>
          </div>
        </div>
      )}

      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-base font-semibold text-gray-900 dark:text-white">Agents</h2>
          <p className="text-sm text-gray-500">Connected servers running the InfraPilot agent</p>
        </div>
        <button
          onClick={() => setShowCreateModal(true)}
          disabled={atLimit}
          className="flex items-center gap-2 px-4 py-2 bg-primary-600 hover:bg-primary-700 text-white rounded-lg transition-colors text-sm disabled:opacity-50 disabled:cursor-not-allowed"
          title={atLimit ? "Agent limit reached — upgrade to add more" : undefined}
        >
          <Plus className="h-4 w-4" />
          Add Agent
        </button>
      </div>

      <MetricsGrid columns={4}>
        <StatCard label="Total" value={total} icon={Server} iconColor="text-blue-600" />
        <StatCard label="Active" value={active} icon={Activity} iconColor="text-green-600" />
        <StatCard label="Pending" value={pending} description="Awaiting connection" icon={Clock} iconColor="text-yellow-600" />
        <StatCard label="Offline" value={offline} icon={Circle} iconColor="text-gray-400" />
      </MetricsGrid>

      <Card>
        {isLoading ? (
          <div className="flex items-center justify-center h-32">
            <Spinner size="lg" />
          </div>
        ) : !agents || agents.length === 0 ? (
          <div className="py-12">
            <EmptyState
              icon={Server}
              title="No agents yet"
              description="Add an agent to connect a server to InfraPilot"
              action={
                <button
                  onClick={() => setShowCreateModal(true)}
                  className="px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700 transition-colors text-sm"
                >
                  Add Agent
                </button>
              }
            />
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead className="bg-gray-50 dark:bg-gray-800/50 border-b border-gray-200 dark:border-gray-800">
                <tr className="text-left text-xs text-gray-600 dark:text-gray-400 uppercase">
                  <th className="px-4 py-3 font-medium">Name</th>
                  <th className="px-4 py-3 font-medium">Hostname</th>
                  <th className="px-4 py-3 font-medium">Status</th>
                  <th className="px-4 py-3 font-medium">Version</th>
                  <th className="px-4 py-3 font-medium">Last Seen</th>
                  <th className="px-4 py-3 font-medium">Added</th>
                  <th className="px-4 py-3 text-right font-medium">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200 dark:divide-gray-800">
                {agents.map((agent) => {
                  const s = statusConfig[agent.status];
                  return (
                    <tr key={agent.id} className="hover:bg-gray-50 dark:hover:bg-gray-800/30 transition-colors">
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-2">
                          <Server className="h-4 w-4 text-gray-400 flex-shrink-0" />
                          <span className="text-gray-900 dark:text-white font-medium">{agent.name}</span>
                        </div>
                      </td>
                      <td className="px-4 py-3 text-sm text-gray-600 dark:text-gray-400">
                        {agent.hostname || <span className="text-gray-400 italic">not connected</span>}
                      </td>
                      <td className="px-4 py-3">
                        <span className={`inline-flex items-center gap-1.5 text-xs font-medium ${s.color}`}>
                          <span className={`h-1.5 w-1.5 rounded-full ${s.dot}`} />
                          {s.label}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-sm text-gray-600 dark:text-gray-400 font-mono">
                        {agent.version || "\u2014"}
                      </td>
                      <td className="px-4 py-3 text-sm text-gray-600 dark:text-gray-400">
                        {agent.last_seen_at ? new Date(agent.last_seen_at).toLocaleString() : "Never"}
                      </td>
                      <td className="px-4 py-3 text-sm text-gray-600 dark:text-gray-400">
                        {new Date(agent.created_at).toLocaleDateString()}
                      </td>
                      <td className="px-4 py-3">
                        <div className="flex items-center justify-end gap-2">
                          <button
                            onClick={() => setDeleteConfirm(agent)}
                            className="p-2 text-gray-600 dark:text-gray-400 hover:text-red-400 hover:bg-red-500/10 rounded-lg transition-colors"
                            title="Remove agent"
                          >
                            <Trash2 className="h-4 w-4" />
                          </button>
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      {/* Create Agent Modal */}
      {showCreateModal && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-white dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-800 p-6 w-full max-w-md">
            <div className="flex items-center justify-between mb-4">
              <h2 className="text-lg font-semibold text-gray-900 dark:text-white">Add New Agent</h2>
              <button
                onClick={() => setShowCreateModal(false)}
                className="text-gray-400 hover:text-gray-600 dark:hover:text-gray-200"
              >
                <X className="h-5 w-5" />
              </button>
            </div>
            <form
              onSubmit={(e) => {
                e.preventDefault();
                createMutation.mutate(agentName);
              }}
              className="space-y-4"
            >
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                  Agent Name
                </label>
                <input
                  type="text"
                  value={agentName}
                  onChange={(e) => setAgentName(e.target.value)}
                  placeholder="e.g. prod-web-01"
                  className="w-full bg-gray-50 dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg px-3 py-2 text-gray-900 dark:text-white focus:ring-2 focus:ring-primary-500 focus:border-transparent"
                  required
                  autoFocus
                />
                <p className="mt-1 text-xs text-gray-500">A friendly name to identify this server</p>
              </div>
              {createMutation.isError && (
                <p className="text-sm text-red-500">Failed to create agent. Please try again.</p>
              )}
              <div className="flex justify-end gap-3 pt-2">
                <button
                  type="button"
                  onClick={() => setShowCreateModal(false)}
                  className="px-4 py-2 text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-white transition-colors"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={createMutation.isPending}
                  className="px-4 py-2 bg-primary-600 hover:bg-primary-700 text-white rounded-lg disabled:opacity-50 transition-colors"
                >
                  {createMutation.isPending ? "Creating..." : "Create Agent"}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Enrollment Token Modal */}
      {createdAgent && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-white dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-800 p-6 w-full max-w-2xl">
            <div className="flex items-center justify-between mb-6">
              <div>
                <h2 className="text-lg font-semibold text-gray-900 dark:text-white">Agent Created</h2>
                <p className="text-sm text-gray-500 mt-0.5">
                  Use the enrollment token to connect{" "}
                  <span className="font-medium text-gray-700 dark:text-gray-300">{createdAgent.name}</span>{" "}
                  to this dashboard
                </p>
              </div>
              <button
                onClick={() => setCreatedAgent(null)}
                className="text-gray-400 hover:text-gray-600 dark:hover:text-gray-200"
              >
                <X className="h-5 w-5" />
              </button>
            </div>

            <div className="space-y-5">
              <div>
                <p className="text-xs font-medium text-gray-500 dark:text-gray-400 uppercase mb-2">
                  Enrollment Token (one-time)
                </p>
                <div className="flex items-center gap-2 bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg px-3 py-2">
                  <code className="flex-1 text-sm font-mono text-gray-900 dark:text-white break-all">
                    {createdAgent.enrollment_token}
                  </code>
                  <CopyButton text={createdAgent.enrollment_token} />
                </div>
                <p className="mt-1 text-xs text-amber-600 dark:text-amber-400">
                  Save this token &mdash; it won&apos;t be shown again after you close this dialog.
                </p>
              </div>

              <div>
                <div className="flex items-center gap-2 mb-2">
                  <Terminal className="h-4 w-4 text-gray-500" />
                  <p className="text-xs font-medium text-gray-500 dark:text-gray-400 uppercase">
                    One-liner Install &amp; Configure
                  </p>
                </div>
                <div className="relative bg-gray-900 rounded-lg px-4 py-3 pr-10">
                  <code className="text-xs text-green-400 font-mono break-all whitespace-pre-wrap">
                    {installOneliner}
                  </code>
                  <div className="absolute top-2 right-2">
                    <CopyButton text={installOneliner} />
                  </div>
                </div>
              </div>

              <div>
                <p className="text-xs font-medium text-gray-500 dark:text-gray-400 uppercase mb-2">
                  Or &mdash; Install first, then configure
                </p>
                <div className="relative bg-gray-900 rounded-lg px-4 py-3 pr-10">
                  <code className="text-xs text-green-400 font-mono whitespace-pre">
                    {`curl -fsSL https://infrapilot.org/agent.sh | bash\n\n`}{configureCmd}
                  </code>
                  <div className="absolute top-2 right-2">
                    <CopyButton text={`curl -fsSL https://infrapilot.org/agent.sh | bash\n\n${configureCmd}`} />
                  </div>
                </div>
              </div>

              <div className="flex justify-end pt-2">
                <button
                  onClick={() => setCreatedAgent(null)}
                  className="px-4 py-2 bg-primary-600 hover:bg-primary-700 text-white rounded-lg transition-colors text-sm"
                >
                  Done
                </button>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Delete Confirmation */}
      {deleteConfirm && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-white dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-800 p-6 w-full max-w-md">
            <h2 className="text-lg font-semibold text-gray-900 dark:text-white mb-2">Remove Agent</h2>
            <p className="text-gray-600 dark:text-gray-400 mb-4">
              Are you sure you want to remove{" "}
              <span className="font-medium text-gray-900 dark:text-white">{deleteConfirm.name}</span>?
              The agent service on that server will stop reporting to this dashboard.
            </p>
            <div className="flex justify-end gap-3">
              <button
                onClick={() => setDeleteConfirm(null)}
                className="px-4 py-2 text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-white transition-colors"
              >
                Cancel
              </button>
              <button
                onClick={() => deleteMutation.mutate(deleteConfirm.id)}
                disabled={deleteMutation.isPending}
                className="px-4 py-2 bg-red-600 hover:bg-red-700 text-white rounded-lg disabled:opacity-50 transition-colors"
              >
                {deleteMutation.isPending ? "Removing..." : "Remove Agent"}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
