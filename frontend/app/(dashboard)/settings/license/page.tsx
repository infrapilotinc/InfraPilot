"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Crown,
  Sparkles,
  ExternalLink,
  Key,
  KeyRound,
  Loader2,
  Check,
  Rocket,
} from "lucide-react";
import { api } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Card } from "@/components/ui/Card";
import { Badge } from "@/components/ui/Badge";
import { UpdateProgressModal } from "@/components/settings/UpdateProgressModal";

const PAID_TIERS = ["professional", "business", "enterprise"];

const TIER_CONFIG: Record<string, { label: string; color: string; badgeClass: string; iconClass: string }> = {
  community: {
    label: "Community Edition",
    color: "green",
    badgeClass: "bg-green-500/10 text-green-400 border-green-500/30",
    iconClass: "text-green-400",
  },
  professional: {
    label: "Professional Edition",
    color: "blue",
    badgeClass: "bg-blue-500/10 text-blue-400 border-blue-500/30",
    iconClass: "text-blue-400",
  },
  enterprise: {
    label: "Enterprise Edition",
    color: "purple",
    badgeClass: "bg-purple-500/10 text-purple-400 border-purple-500/30",
    iconClass: "text-purple-400",
  },
};

const FEATURE_LABELS: Record<string, string> = {
  core_monitoring: "Core Monitoring",
  proxy_management: "Proxy Management",
  container_ops: "Container Operations",
  unified_logs: "Unified Logs",
  alert_channels: "Alerts & Notifications",
  user_management: "User Management",
  supply_chain_security: "Supply Chain Security",
  runtime_security: "Runtime Security",
  policy_as_code: "Policy-as-Code",
  secrets_management: "Secrets Management",
  data_governance: "Data Governance",
  code_quality: "Code Quality",
  stack_management: "Docker Stacks",
  vulnerability_scanning: "Vulnerability Scanning",
  basic_alerts: "Basic Alerts",
  advanced_alerts: "Advanced Alerts",
  unlimited_agents: "Unlimited Agents",
  sso: "Single Sign-On",
  audit_logs: "Audit Logs",
  sla_support: "SLA Support",
  custom_integrations: "Custom Integrations",
};

function LicenseSection() {
  const queryClient = useQueryClient();
  const [newKey, setNewKey] = useState("");
  const [showUpdateForm, setShowUpdateForm] = useState(false);
  const [updateError, setUpdateError] = useState("");
  const [updateSuccess, setUpdateSuccess] = useState(false);
  const [showUpgrade, setShowUpgrade] = useState(false);

  const { data: license, isLoading } = useQuery({
    queryKey: ["licenseSettings"],
    queryFn: () => api.getLicenseSettings(),
  });

  const updateMutation = useMutation({
    mutationFn: (key: string) => api.updateLicenseKey(key),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["licenseSettings"] });
      queryClient.invalidateQueries({ queryKey: ["licenseTierInfo"] });
      setNewKey("");
      setShowUpdateForm(false);
      setUpdateError("");
      setUpdateSuccess(true);
      setTimeout(() => setUpdateSuccess(false), 4000);
    },
    onError: (err: Error) => {
      setUpdateError(err.message);
    },
  });

  const tier = license?.tier ?? "community";
  const tierCfg = TIER_CONFIG[tier] ?? TIER_CONFIG.community;
  const isEnvLocked = license?.key_source === "env";
  const isOffline = license?.key_source === "offline";
  // Paid key active, but this server is still running the Community image → needs the
  // in-place CE→EE image switch to actually deliver the paid features.
  const needsImageSwitch =
    license?.edition === "community" && PAID_TIERS.includes(tier) && !isEnvLocked && !isOffline;
  const tierLabel = tier.charAt(0).toUpperCase() + tier.slice(1);

  return (
    <Card>
      <Card.Header>
        <div className="flex items-center gap-3">
          <div className={cn("p-2 rounded-lg", tier === "enterprise" ? "bg-purple-500/10" : tier === "professional" ? "bg-blue-500/10" : "bg-green-500/10")}>
            <Crown className={cn("h-5 w-5", tierCfg.iconClass)} />
          </div>
          <div>
            <h2 className="text-lg font-semibold text-gray-900 dark:text-white">License</h2>
            <p className="text-sm text-gray-600 dark:text-gray-400">
              Manage your InfraPilot license
            </p>
          </div>
        </div>
      </Card.Header>
      <Card.Body>
        {isLoading ? (
          <div className="flex items-center justify-center py-8">
            <Loader2 className="h-6 w-6 text-gray-400 animate-spin" />
          </div>
        ) : (
          <div className="space-y-6">
            {updateSuccess && (
              <div className="flex items-center gap-3 p-3 bg-green-500/10 border border-green-500/30 rounded-lg text-green-400 text-sm">
                <Check className="h-4 w-4 flex-shrink-0" />
                License key updated successfully. Your new license is active immediately.
              </div>
            )}

            {needsImageSwitch && (
              <div className="p-4 rounded-lg border border-purple-500/40 bg-purple-500/5">
                <div className="flex items-start justify-between gap-4">
                  <div className="flex gap-3">
                    <Rocket className="h-5 w-5 text-purple-400 mt-0.5 flex-shrink-0" />
                    <div>
                      <h4 className="font-medium text-gray-900 dark:text-white">
                        Finish your upgrade to {tierLabel}
                      </h4>
                      <p className="text-sm text-gray-500 mt-0.5">
                        Your {tierLabel} license is active, but this server is still running the
                        Community image. Switch to the Enterprise image to unlock {tierLabel}{" "}
                        features — your data and settings are preserved, and it auto-reverts if
                        anything goes wrong.
                      </p>
                    </div>
                  </div>
                  <button
                    onClick={() => setShowUpgrade(true)}
                    className="px-4 py-2 bg-purple-600 hover:bg-purple-700 text-white rounded-lg text-sm flex-shrink-0 flex items-center gap-2"
                  >
                    <Rocket className="h-4 w-4" />
                    Switch to Enterprise
                  </button>
                </div>
              </div>
            )}

            {showUpgrade && (
              <UpdateProgressModal
                title={`Upgrading to ${tierLabel}`}
                icon="rocket"
                streamPath="/settings/license/upgrade"
                steps={[
                  { key: "validate", label: "Validate license" },
                  { key: "authenticate", label: "Authenticate with registry" },
                  { key: "download", label: "Download Enterprise image" },
                  { key: "switch", label: "Switch over & restart" },
                ]}
                verifyMode="edition-enterprise"
                successMessage="Enterprise Edition is active."
                onClose={() => setShowUpgrade(false)}
                onDone={() => {
                  queryClient.invalidateQueries({ queryKey: ["licenseSettings"] });
                  queryClient.invalidateQueries({ queryKey: ["licenseTierInfo"] });
                }}
              />
            )}

            <div className={cn("p-4 rounded-lg border", tier === "enterprise" ? "bg-purple-500/5 border-purple-500/30" : tier === "professional" ? "bg-blue-500/5 border-blue-500/30" : "bg-green-500/5 border-green-500/30")}>
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <div className={cn("w-3 h-3 rounded-full", license?.valid ? "bg-green-500" : "bg-yellow-500")} />
                  <div>
                    <span className="font-semibold text-gray-900 dark:text-white">
                      {tierCfg.label}
                    </span>
                    <p className="text-sm text-gray-500 mt-0.5">
                      {license?.max_agents === -1
                        ? "Unlimited agents"
                        : `Up to ${license?.max_agents ?? 1} agent${(license?.max_agents ?? 1) !== 1 ? "s" : ""}`}
                      {license?.expires_at && ` · Expires ${new Date(license.expires_at).toLocaleDateString()}`}
                    </p>
                  </div>
                </div>
                <Badge className={tierCfg.badgeClass}>
                  {license?.valid ? "Active" : "Invalid"}
                </Badge>
              </div>
            </div>

            {(license?.key_display || isEnvLocked || isOffline) && (
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                  Active License Key
                </label>
                <div className="flex items-center gap-2">
                  <div className="flex-1 flex items-center gap-2 px-3 py-2 bg-gray-100 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg">
                    <KeyRound className="h-4 w-4 text-gray-400 flex-shrink-0" />
                    <code className="text-sm font-mono text-gray-700 dark:text-gray-300 flex-1">
                      {isOffline ? "Offline Mode (dev)" : license?.key_display || "—"}
                    </code>
                    {isEnvLocked && (
                      <Badge className="bg-gray-200 dark:bg-gray-700 text-gray-500 dark:text-gray-400 text-xs border-0">
                        ENV
                      </Badge>
                    )}
                    {license?.key_source === "database" && (
                      <Badge className="bg-gray-200 dark:bg-gray-700 text-gray-500 dark:text-gray-400 text-xs border-0">
                        DB
                      </Badge>
                    )}
                  </div>
                </div>
                {isEnvLocked && (
                  <p className="mt-1.5 text-xs text-gray-500">
                    Locked via <code className="font-mono">LICENSE_KEY</code> environment variable. Remove the env var to manage the license from here.
                  </p>
                )}
              </div>
            )}

            {license?.features && license.features.length > 0 && (
              <div>
                <h3 className="text-sm font-medium text-gray-700 dark:text-gray-300 mb-3">Included Features</h3>
                <div className="grid sm:grid-cols-2 gap-2">
                  {license.features.map((f) => (
                    <div key={f} className="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                      <Check className="h-3.5 w-3.5 text-green-400 flex-shrink-0" />
                      {FEATURE_LABELS[f] ?? f.replace(/_/g, " ").replace(/\b\w/g, (c) => c.toUpperCase())}
                    </div>
                  ))}
                </div>
              </div>
            )}

            {!isEnvLocked && !isOffline && (
              <div className="pt-4 border-t border-gray-200 dark:border-gray-800">
                {!showUpdateForm ? (
                  <div className="flex items-center justify-between">
                    <div>
                      <p className="text-sm font-medium text-gray-700 dark:text-gray-300">
                        {license?.key_source === "setup_mode" ? "No license key configured" : "Update license key"}
                      </p>
                      <p className="text-xs text-gray-500 mt-0.5">
                        {license?.key_source === "setup_mode"
                          ? "Enter a license key to enable full validation"
                          : "Change or renew your license key"}
                      </p>
                    </div>
                    <button
                      onClick={() => setShowUpdateForm(true)}
                      className="px-4 py-2 bg-primary-600 hover:bg-primary-700 text-white rounded-lg text-sm transition-colors flex items-center gap-2"
                    >
                      <Key className="h-4 w-4" />
                      {license?.key_source === "setup_mode" ? "Add License Key" : "Update Key"}
                    </button>
                  </div>
                ) : (
                  <div className="space-y-3">
                    <div>
                      <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                        New License Key
                      </label>
                      <input
                        type="text"
                        value={newKey}
                        onChange={(e) => {
                          setNewKey(e.target.value);
                          setUpdateError("");
                        }}
                        placeholder="IP-CE-XXXX-XXXX-XXXX"
                        className="w-full px-4 py-2 bg-gray-50 dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg text-gray-900 dark:text-white font-mono placeholder-gray-400 focus:ring-2 focus:ring-primary-500 focus:border-transparent"
                        autoFocus
                      />
                      <p className="mt-1.5 text-xs text-gray-500">
                        Get a free key at{" "}
                        <a href="https://infrapilot.org/signup" target="_blank" rel="noopener noreferrer" className="text-primary-400 hover:underline">
                          infrapilot.org/signup
                        </a>
                      </p>
                    </div>

                    {updateError && (
                      <div className="p-3 bg-red-500/10 border border-red-500/30 rounded-lg text-red-400 text-sm">
                        {updateError}
                      </div>
                    )}

                    <div className="flex gap-2 justify-end">
                      <button
                        onClick={() => {
                          setShowUpdateForm(false);
                          setNewKey("");
                          setUpdateError("");
                        }}
                        className="px-4 py-2 text-sm text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-white"
                      >
                        Cancel
                      </button>
                      <button
                        onClick={() => updateMutation.mutate(newKey)}
                        disabled={!newKey.trim() || updateMutation.isPending}
                        className="px-4 py-2 bg-primary-600 hover:bg-primary-700 disabled:bg-primary-400 text-white rounded-lg text-sm transition-colors flex items-center gap-2"
                      >
                        {updateMutation.isPending ? (
                          <Loader2 className="h-4 w-4 animate-spin" />
                        ) : (
                          <Check className="h-4 w-4" />
                        )}
                        {updateMutation.isPending ? "Validating..." : "Validate & Save"}
                      </button>
                    </div>
                  </div>
                )}
              </div>
            )}

            {tier !== "enterprise" && (
              <div className="p-4 bg-gray-50 dark:bg-gray-800/50 border border-gray-200 dark:border-gray-700 rounded-lg">
                <div className="flex items-center justify-between">
                  <div>
                    <h4 className="font-medium text-gray-900 dark:text-white">Need more?</h4>
                    <p className="text-sm text-gray-500 mt-0.5">
                      Upgrade to Professional or Enterprise for more agents and advanced features
                    </p>
                  </div>
                  <a
                    href={license?.upgrade_url ?? "https://infrapilot.org/billing"}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="px-4 py-2 bg-primary-600 hover:bg-primary-700 text-white rounded-lg transition-colors flex items-center gap-2 text-sm flex-shrink-0"
                  >
                    <Sparkles className="h-4 w-4" />
                    Upgrade
                    <ExternalLink className="h-3 w-3" />
                  </a>
                </div>
              </div>
            )}
          </div>
        )}
      </Card.Body>
    </Card>
  );
}

export default function SettingsLicensePage() {
  return <LicenseSection />;
}
