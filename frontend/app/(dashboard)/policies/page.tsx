"use client";

import { useState, useEffect } from "react";
import {
  Shield,
  Plus,
  AlertTriangle,
  CheckCircle,
  XCircle,
  FileText,
  Clock,
  Filter,
  Search,
  MoreVertical,
  Trash2,
  Edit,
  Eye,
  Play,
  Pause,
} from "lucide-react";
import {
  api,
  Policy,
  PolicyTemplate,
  PolicyViolation,
  PolicyStats,
  PolicyType,
  PolicyAction,
} from "@/lib/api";

type TabType = "policies" | "templates" | "violations";

export default function PoliciesPage() {
  const [activeTab, setActiveTab] = useState<TabType>("policies");
  const [policies, setPolicies] = useState<Policy[]>([]);
  const [templates, setTemplates] = useState<PolicyTemplate[]>([]);
  const [violations, setViolations] = useState<PolicyViolation[]>([]);
  const [loading, setLoading] = useState(true);
  const [stats, setStats] = useState<PolicyStats | null>(null);
  const [error, setError] = useState<string | null>(null);

  // Filters
  const [typeFilter, setTypeFilter] = useState<PolicyType | "">("");
  const [searchQuery, setSearchQuery] = useState("");
  const [showResolved, setShowResolved] = useState(false);

  // Modals
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [showTemplateModal, setShowTemplateModal] = useState(false);
  const [selectedTemplate, setSelectedTemplate] =
    useState<PolicyTemplate | null>(null);

  // Form state for new policy
  const [newPolicy, setNewPolicy] = useState({
    name: "",
    policy_type: "container" as PolicyType,
    action: "warn" as PolicyAction,
    conditions: {} as Record<string, unknown>,
    enabled: true,
    priority: 0,
  });

  useEffect(() => {
    loadData();
  }, [typeFilter, showResolved]);

  const loadData = async () => {
    try {
      setLoading(true);
      const [policiesData, templatesData, violationsData, statsData] =
        await Promise.all([
          api.getPolicies({ type: typeFilter || undefined }),
          api.getPolicyTemplates(),
          api.getPolicyViolations({ resolved: showResolved || undefined }),
          api.getPolicyStats(),
        ]);
      setPolicies(policiesData || []);
      setTemplates(templatesData || []);
      setViolations(violationsData || []);
      setStats(statsData);
    } catch (error) {
      console.error("Failed to load policies:", error);
    } finally {
      setLoading(false);
    }
  };

  const handleCreatePolicy = async () => {
    try {
      setError(null);
      await api.createPolicy(newPolicy);
      setShowCreateModal(false);
      setNewPolicy({
        name: "",
        policy_type: "container",
        action: "warn",
        conditions: {},
        enabled: true,
        priority: 0,
      });
      loadData();
    } catch (error) {
      console.error("Failed to create policy:", error);
      setError(error instanceof Error ? error.message : "Failed to create policy");
    }
  };

  const handleCreateFromTemplate = async (templateId: string) => {
    try {
      const template = templates.find(t => t.id === templateId);
      await api.createPolicyFromTemplate(templateId, {
        name: template?.name?.replace(/_/g, " ") || "New Policy",
      });
      setShowTemplateModal(false);
      setSelectedTemplate(null);
      loadData();
      setError(null);
    } catch (error) {
      console.error("Failed to create policy from template:", error);
      setError(error instanceof Error ? error.message : "Failed to create policy");
    }
  };

  const handleTogglePolicy = async (policy: Policy) => {
    try {
      await api.updatePolicy(policy.id, { enabled: !policy.enabled });
      loadData();
    } catch (error) {
      console.error("Failed to toggle policy:", error);
    }
  };

  const handleDeletePolicy = async (policyId: string) => {
    try {
      await api.deletePolicy(policyId);
      loadData();
    } catch (error) {
      console.error("Failed to delete policy:", error);
    }
  };

  const handleResolveViolation = async (violationId: string) => {
    try {
      await api.resolvePolicyViolation(violationId, "Manually resolved");
      loadData();
    } catch (error) {
      console.error("Failed to resolve violation:", error);
    }
  };

  const getActionBadge = (action: PolicyAction) => {
    const styles = {
      block: "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400",
      warn: "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400",
      audit:
        "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400",
    };
    return styles[action] || styles.audit;
  };

  const getTypeBadge = (type: PolicyType) => {
    const styles = {
      container:
        "bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400",
      proxy:
        "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400",
      access: "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400",
      security:
        "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400",
    };
    return styles[type] || styles.container;
  };

  const filteredPolicies = policies.filter(
    (p) =>
      !searchQuery ||
      p.name.toLowerCase().includes(searchQuery.toLowerCase())
  );

  if (loading) {
    return (
      <div className="h-full flex items-center justify-center">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600" />
      </div>
    );
  }

  return (
    <div className="h-full overflow-auto">
      <div className="max-w-7xl mx-auto py-6 px-4">
        {/* Header */}
        <div className="flex items-center justify-between mb-6">
          <div>
            <h1 className="text-2xl font-bold text-gray-900 dark:text-white flex items-center gap-2">
              <Shield className="h-6 w-6" />
              Policy Engine
            </h1>
            <p className="text-sm text-gray-500 mt-1">
              Define and enforce security policies across your infrastructure
            </p>
          </div>
          <div className="flex gap-2">
            <button
              onClick={() => setShowTemplateModal(true)}
              className="flex items-center gap-2 px-4 py-2 border border-gray-300 dark:border-gray-600 rounded-lg hover:bg-gray-50 dark:hover:bg-gray-800"
            >
              <FileText className="h-4 w-4" />
              From Template
            </button>
            <button
              onClick={() => setShowCreateModal(true)}
              className="flex items-center gap-2 px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700"
            >
              <Plus className="h-4 w-4" />
              Create Policy
            </button>
          </div>
        </div>

        {/* Error Banner */}
        {error && (
          <div className="mb-6 p-4 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg flex items-center justify-between">
            <div className="flex items-center gap-3">
              <AlertTriangle className="h-5 w-5 text-red-500" />
              <span className="text-red-700 dark:text-red-400">{error}</span>
            </div>
            <button
              onClick={() => setError(null)}
              className="text-red-500 hover:text-red-700"
            >
              <XCircle className="h-5 w-5" />
            </button>
          </div>
        )}

        {/* Stats Cards */}
        {stats && (
          <div className="grid grid-cols-1 md:grid-cols-4 gap-4 mb-6">
            <div className="bg-white dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-700 p-4">
              <div className="flex items-center justify-between">
                <div>
                  <p className="text-sm text-gray-500">Total Policies</p>
                  <p className="text-2xl font-bold text-gray-900 dark:text-white">
                    {stats.total_policies}
                  </p>
                </div>
                <Shield className="h-8 w-8 text-gray-400" />
              </div>
            </div>
            <div className="bg-white dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-700 p-4">
              <div className="flex items-center justify-between">
                <div>
                  <p className="text-sm text-gray-500">Enabled Policies</p>
                  <p className="text-2xl font-bold text-green-600">
                    {stats.enabled_policies}
                  </p>
                </div>
                <CheckCircle className="h-8 w-8 text-green-400" />
              </div>
            </div>
            <div className="bg-white dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-700 p-4">
              <div className="flex items-center justify-between">
                <div>
                  <p className="text-sm text-gray-500">Total Violations</p>
                  <p className="text-2xl font-bold text-gray-900 dark:text-white">
                    {stats.total_violations}
                  </p>
                </div>
                <AlertTriangle className="h-8 w-8 text-gray-400" />
              </div>
            </div>
            <div className="bg-white dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-700 p-4">
              <div className="flex items-center justify-between">
                <div>
                  <p className="text-sm text-gray-500">Unresolved</p>
                  <p className="text-2xl font-bold text-red-600">
                    {stats.unresolved_violations}
                  </p>
                </div>
                <XCircle className="h-8 w-8 text-red-400" />
              </div>
            </div>
          </div>
        )}

        {/* Tabs */}
        <div className="flex gap-1 border-b border-gray-200 dark:border-gray-700 mb-6">
          {[
            {
              id: "policies",
              label: "Policies",
              count: policies.length,
              icon: Shield,
            },
            {
              id: "templates",
              label: "Templates",
              count: templates.length,
              icon: FileText,
            },
            {
              id: "violations",
              label: "Violations",
              count: violations.length,
              icon: AlertTriangle,
            },
          ].map((tab) => (
            <button
              key={tab.id}
              onClick={() => setActiveTab(tab.id as TabType)}
              className={`flex items-center gap-2 px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors ${
                activeTab === tab.id
                  ? "border-primary-600 text-primary-600"
                  : "border-transparent text-gray-500 hover:text-gray-700 dark:hover:text-gray-300"
              }`}
            >
              <tab.icon className="h-4 w-4" />
              {tab.label}
              <span className="px-2 py-0.5 text-xs bg-gray-100 dark:bg-gray-800 rounded-full">
                {tab.count}
              </span>
            </button>
          ))}
        </div>

        {/* Policies Tab */}
        {activeTab === "policies" && (
          <div className="space-y-4">
            {/* Filters */}
            <div className="flex gap-4 items-center">
              <div className="relative flex-1 max-w-md">
                <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-gray-400" />
                <input
                  type="text"
                  value={searchQuery}
                  onChange={(e) => setSearchQuery(e.target.value)}
                  placeholder="Search policies..."
                  className="w-full pl-10 pr-4 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800"
                />
              </div>
              <select
                value={typeFilter}
                onChange={(e) =>
                  setTypeFilter(e.target.value as PolicyType | "")
                }
                className="px-4 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800"
              >
                <option value="">All Types</option>
                <option value="container">Container</option>
                <option value="proxy">Proxy</option>
                <option value="access">Access</option>
                <option value="security">Security</option>
              </select>
            </div>

            {/* Policies List */}
            <div className="bg-white dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-700">
              {filteredPolicies.length === 0 ? (
                <div className="p-8 text-center text-gray-500">
                  <Shield className="h-12 w-12 mx-auto mb-4 opacity-50" />
                  <p className="font-medium">No policies found</p>
                  <p className="text-sm">
                    Create a policy or use a template to get started
                  </p>
                </div>
              ) : (
                <div className="divide-y divide-gray-200 dark:divide-gray-700">
                  {filteredPolicies.map((policy) => (
                    <div
                      key={policy.id}
                      className="p-4 hover:bg-gray-50 dark:hover:bg-gray-800/50"
                    >
                      <div className="flex items-center justify-between">
                        <div className="flex items-center gap-3">
                          <div
                            className={`w-2 h-2 rounded-full ${
                              policy.enabled ? "bg-green-500" : "bg-gray-400"
                            }`}
                          />
                          <div>
                            <div className="flex items-center gap-2">
                              <span className="font-medium text-gray-900 dark:text-white">
                                {policy.name}
                              </span>
                              <span
                                className={`px-2 py-0.5 text-xs rounded ${getTypeBadge(
                                  policy.policy_type
                                )}`}
                              >
                                {policy.policy_type}
                              </span>
                              <span
                                className={`px-2 py-0.5 text-xs rounded ${getActionBadge(
                                  policy.action
                                )}`}
                              >
                                {policy.action}
                              </span>
                            </div>
                            <p className="text-sm text-gray-500 mt-1">
                              Priority: {policy.priority} | Created:{" "}
                              {new Date(policy.created_at).toLocaleDateString()}
                            </p>
                          </div>
                        </div>
                        <div className="flex items-center gap-2">
                          <button
                            onClick={() => handleTogglePolicy(policy)}
                            className={`p-2 rounded-lg ${
                              policy.enabled
                                ? "text-green-600 hover:bg-green-50 dark:hover:bg-green-900/20"
                                : "text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800"
                            }`}
                            title={policy.enabled ? "Disable" : "Enable"}
                          >
                            {policy.enabled ? (
                              <Pause className="h-4 w-4" />
                            ) : (
                              <Play className="h-4 w-4" />
                            )}
                          </button>
                          <button
                            onClick={() => handleDeletePolicy(policy.id)}
                            className="p-2 text-red-500 hover:bg-red-50 dark:hover:bg-red-900/20 rounded-lg"
                          >
                            <Trash2 className="h-4 w-4" />
                          </button>
                        </div>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>
        )}

        {/* Templates Tab */}
        {activeTab === "templates" && (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {templates.map((template) => (
              <div
                key={template.id}
                className="bg-white dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-700 p-4"
              >
                <div className="flex items-start justify-between mb-2">
                  <h3 className="font-medium text-gray-900 dark:text-white">
                    {template.name.replace(/_/g, " ")}
                  </h3>
                  <span
                    className={`px-2 py-0.5 text-xs rounded ${getTypeBadge(
                      template.policy_type
                    )}`}
                  >
                    {template.policy_type}
                  </span>
                </div>
                <p className="text-sm text-gray-500 mb-3">
                  {template.description}
                </p>
                <div className="flex items-center justify-between">
                  <span
                    className={`px-2 py-0.5 text-xs rounded ${getActionBadge(
                      template.recommended_action
                    )}`}
                  >
                    Recommended: {template.recommended_action}
                  </span>
                  <button
                    onClick={() => handleCreateFromTemplate(template.id)}
                    className="text-sm text-primary-600 hover:underline"
                  >
                    Use Template
                  </button>
                </div>
              </div>
            ))}
            {templates.length === 0 && (
              <div className="col-span-full p-8 text-center text-gray-500">
                <FileText className="h-12 w-12 mx-auto mb-4 opacity-50" />
                <p>No templates available</p>
              </div>
            )}
          </div>
        )}

        {/* Violations Tab */}
        {activeTab === "violations" && (
          <div className="space-y-4">
            {/* Filter */}
            <div className="flex items-center gap-4">
              <label className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={showResolved}
                  onChange={(e) => setShowResolved(e.target.checked)}
                  className="rounded border-gray-300"
                />
                Show resolved
              </label>
            </div>

            {/* Violations List */}
            <div className="bg-white dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-700">
              {violations.length === 0 ? (
                <div className="p-8 text-center text-gray-500">
                  <CheckCircle className="h-12 w-12 mx-auto mb-4 text-green-400" />
                  <p className="font-medium">No violations</p>
                  <p className="text-sm">
                    All policies are being followed
                  </p>
                </div>
              ) : (
                <div className="divide-y divide-gray-200 dark:divide-gray-700">
                  {violations.map((violation) => (
                    <div
                      key={violation.id}
                      className="p-4 hover:bg-gray-50 dark:hover:bg-gray-800/50"
                    >
                      <div className="flex items-start justify-between">
                        <div className="flex items-start gap-3">
                          {violation.resolved ? (
                            <CheckCircle className="h-5 w-5 text-green-500 mt-0.5" />
                          ) : (
                            <AlertTriangle className="h-5 w-5 text-red-500 mt-0.5" />
                          )}
                          <div>
                            <p className="font-medium text-gray-900 dark:text-white">
                              {violation.message}
                            </p>
                            <div className="flex items-center gap-4 mt-1 text-sm text-gray-500">
                              <span className="flex items-center gap-1">
                                <Clock className="h-3 w-3" />
                                {new Date(
                                  violation.created_at
                                ).toLocaleString()}
                              </span>
                              {violation.resource_type && (
                                <span>
                                  {violation.resource_type}:{" "}
                                  {violation.resource_id}
                                </span>
                              )}
                            </div>
                          </div>
                        </div>
                        {!violation.resolved && (
                          <button
                            onClick={() => handleResolveViolation(violation.id)}
                            className="px-3 py-1 text-sm bg-green-600 text-white rounded-lg hover:bg-green-700"
                          >
                            Resolve
                          </button>
                        )}
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>
        )}

        {/* Create Policy Modal */}
        {showCreateModal && (
          <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
            <div className="bg-white dark:bg-gray-900 rounded-lg p-6 w-full max-w-lg max-h-[90vh] overflow-auto">
              <h3 className="text-lg font-semibold mb-4">Create Policy</h3>
              <div className="space-y-4">
                <div>
                  <label className="block text-sm font-medium mb-1">Name</label>
                  <input
                    type="text"
                    value={newPolicy.name}
                    onChange={(e) =>
                      setNewPolicy({ ...newPolicy, name: e.target.value })
                    }
                    className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800"
                    placeholder="No root containers"
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium mb-1">Type</label>
                  <select
                    value={newPolicy.policy_type}
                    onChange={(e) =>
                      setNewPolicy({
                        ...newPolicy,
                        policy_type: e.target.value as PolicyType,
                      })
                    }
                    className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800"
                  >
                    <option value="container">Container</option>
                    <option value="proxy">Proxy</option>
                    <option value="access">Access</option>
                    <option value="security">Security</option>
                  </select>
                </div>
                <div>
                  <label className="block text-sm font-medium mb-1">
                    Action
                  </label>
                  <select
                    value={newPolicy.action}
                    onChange={(e) =>
                      setNewPolicy({
                        ...newPolicy,
                        action: e.target.value as PolicyAction,
                      })
                    }
                    className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800"
                  >
                    <option value="block">Block</option>
                    <option value="warn">Warn</option>
                    <option value="audit">Audit</option>
                  </select>
                </div>
                <div>
                  <label className="block text-sm font-medium mb-1">
                    Priority
                  </label>
                  <input
                    type="number"
                    value={newPolicy.priority}
                    onChange={(e) =>
                      setNewPolicy({
                        ...newPolicy,
                        priority: parseInt(e.target.value) || 0,
                      })
                    }
                    className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800"
                  />
                  <p className="text-xs text-gray-500 mt-1">
                    Higher priority policies are evaluated first
                  </p>
                </div>
                <div>
                  <label className="block text-sm font-medium mb-1">
                    Conditions (JSON)
                  </label>
                  <textarea
                    value={JSON.stringify(newPolicy.conditions, null, 2)}
                    onChange={(e) => {
                      try {
                        setNewPolicy({
                          ...newPolicy,
                          conditions: JSON.parse(e.target.value),
                        });
                      } catch {
                        // Invalid JSON, ignore
                      }
                    }}
                    className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 font-mono text-sm"
                    rows={4}
                    placeholder='{"check": "user", "operator": "equals", "value": "root"}'
                  />
                </div>
                <div className="flex gap-3 justify-end pt-4">
                  <button
                    onClick={() => setShowCreateModal(false)}
                    className="px-4 py-2 border border-gray-300 dark:border-gray-600 rounded-lg"
                  >
                    Cancel
                  </button>
                  <button
                    onClick={handleCreatePolicy}
                    disabled={!newPolicy.name.trim()}
                    className="px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700 disabled:opacity-50"
                  >
                    Create Policy
                  </button>
                </div>
              </div>
            </div>
          </div>
        )}

        {/* Template Modal */}
        {showTemplateModal && (
          <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
            <div className="bg-white dark:bg-gray-900 rounded-lg p-6 w-full max-w-2xl max-h-[90vh] overflow-auto">
              <h3 className="text-lg font-semibold mb-4">
                Create Policy from Template
              </h3>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4 mb-4">
                {templates.map((template) => (
                  <button
                    key={template.id}
                    onClick={() => handleCreateFromTemplate(template.id)}
                    className="text-left p-4 border border-gray-200 dark:border-gray-700 rounded-lg hover:border-primary-600 hover:bg-primary-50 dark:hover:bg-primary-900/20 transition-colors"
                  >
                    <div className="flex items-center justify-between mb-2">
                      <span className="font-medium">
                        {template.name.replace(/_/g, " ")}
                      </span>
                      <span
                        className={`px-2 py-0.5 text-xs rounded ${getTypeBadge(
                          template.policy_type
                        )}`}
                      >
                        {template.policy_type}
                      </span>
                    </div>
                    <p className="text-sm text-gray-500">
                      {template.description}
                    </p>
                  </button>
                ))}
              </div>
              <div className="flex justify-end">
                <button
                  onClick={() => setShowTemplateModal(false)}
                  className="px-4 py-2 border border-gray-300 dark:border-gray-600 rounded-lg"
                >
                  Cancel
                </button>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
