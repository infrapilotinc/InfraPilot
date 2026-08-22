"use client";

import { useState, useEffect } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Check,
  Globe,
  Lock,
  Trash2,
  Loader2,
  RefreshCw,
  ArrowUpCircle,
  AlertCircle,
  Key,
  Plus,
  Copy,
  CheckCircle,
  Eye,
  EyeOff,
  Terminal,
} from "lucide-react";
import { api, APIKey, APIKeyWithSecret } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Card } from "@/components/ui/Card";
import { Badge } from "@/components/ui/Badge";
import { SSLWizard } from "@/components/ssl-wizard";
import { UpdateProgressModal } from "@/components/settings/UpdateProgressModal";

function InfraPilotDomainSection() {
  const queryClient = useQueryClient();
  const [domain, setDomain] = useState("");
  const [sslEnabled, setSSLEnabled] = useState(true);
  const [forceSSL, setForceSSL] = useState(true);
  const [http2Enabled, setHTTP2Enabled] = useState(true);
  const [hasChanges, setHasChanges] = useState(false);
  const [showDelete, setShowDelete] = useState(false);
  const [showSSLWizard, setShowSSLWizard] = useState(false);

  const { data: domainSettings, isLoading } = useQuery({
    queryKey: ["infrapilotDomain"],
    queryFn: () => api.getInfraPilotDomain(),
  });

  useEffect(() => {
    if (domainSettings?.domain) {
      setDomain(domainSettings.domain);
      setSSLEnabled(domainSettings.ssl_enabled);
      setForceSSL(domainSettings.force_ssl);
      setHTTP2Enabled(domainSettings.http2_enabled);
    }
  }, [domainSettings]);

  const saveMutation = useMutation({
    mutationFn: () =>
      api.updateInfraPilotDomain({
        domain,
        ssl_enabled: sslEnabled,
        force_ssl: forceSSL,
        http2_enabled: http2Enabled,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["infrapilotDomain"] });
      setHasChanges(false);
      if (sslEnabled) {
        setShowSSLWizard(true);
      }
    },
  });

  const deleteMutation = useMutation({
    mutationFn: () => api.deleteInfraPilotDomain(),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["infrapilotDomain"] });
      setDomain("");
      setSSLEnabled(true);
      setForceSSL(true);
      setHTTP2Enabled(true);
      setShowDelete(false);
      setHasChanges(false);
    },
  });

  const handleDomainChange = (value: string) => {
    setDomain(value);
    setHasChanges(true);
  };

  const isConfigured = domainSettings?.domain && domainSettings.domain.length > 0;

  return (
    <>
    <Card>
      <Card.Header>
        <div className="flex items-center gap-3">
          <div className="p-2 bg-blue-500/10 rounded-lg">
            <Globe className="h-5 w-5 text-blue-400" />
          </div>
          <div>
            <h2 className="text-lg font-semibold text-gray-900 dark:text-white">InfraPilot Domain</h2>
            <p className="text-sm text-gray-600 dark:text-gray-400">
              Configure a custom domain to access InfraPilot
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
            {isConfigured && (
              <div className="flex items-center justify-between p-4 bg-green-500/5 border border-green-500/20 rounded-lg">
                <div className="flex items-center gap-3">
                  <div className="w-2 h-2 rounded-full bg-green-500" />
                  <div>
                    <p className="font-medium text-gray-900 dark:text-white">
                      {domainSettings.domain}
                    </p>
                    <p className="text-sm text-gray-500">
                      {domainSettings.ssl_enabled ? "HTTPS enabled" : "HTTP only"}
                    </p>
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  {domainSettings.ssl_enabled && (
                    <button
                      onClick={() => setShowSSLWizard(true)}
                      className="flex items-center gap-1.5 px-3 py-1.5 text-xs bg-primary-600/10 hover:bg-primary-600/20 text-primary-400 rounded-lg transition-colors"
                    >
                      <Lock className="h-3.5 w-3.5" />
                      Manage SSL
                    </button>
                  )}
                  <Badge className="bg-green-500/10 text-green-400 border-green-500/30">
                    Active
                  </Badge>
                </div>
              </div>
            )}

            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                Domain Name
              </label>
              <input
                type="text"
                value={domain}
                onChange={(e) => handleDomainChange(e.target.value)}
                placeholder="infrapilot.example.com"
                className="w-full px-4 py-2 bg-gray-50 dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg text-gray-900 dark:text-white placeholder-gray-400 focus:ring-2 focus:ring-primary-500 focus:border-transparent"
              />
              <p className="mt-1.5 text-xs text-gray-500">
                Make sure DNS is pointed to this server before enabling SSL
              </p>
            </div>

            <div className="space-y-4">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <Lock className="h-4 w-4 text-gray-500" />
                  <div>
                    <p className="text-sm font-medium text-gray-900 dark:text-white">Enable SSL/HTTPS</p>
                    <p className="text-xs text-gray-500">Automatically provision Let's Encrypt certificate</p>
                  </div>
                </div>
                <button
                  type="button"
                  onClick={() => {
                    setSSLEnabled(!sslEnabled);
                    setHasChanges(true);
                  }}
                  className={cn(
                    "relative inline-flex h-6 w-11 items-center rounded-full transition-colors",
                    sslEnabled ? "bg-primary-600" : "bg-gray-300 dark:bg-gray-700"
                  )}
                >
                  <span
                    className={cn(
                      "inline-block h-4 w-4 transform rounded-full bg-white transition-transform",
                      sslEnabled ? "translate-x-6" : "translate-x-1"
                    )}
                  />
                </button>
              </div>

              {sslEnabled && (
                <>
                  <div className="flex items-center justify-between pl-7">
                    <div>
                      <p className="text-sm font-medium text-gray-900 dark:text-white">Force HTTPS</p>
                      <p className="text-xs text-gray-500">Redirect all HTTP requests to HTTPS</p>
                    </div>
                    <button
                      type="button"
                      onClick={() => {
                        setForceSSL(!forceSSL);
                        setHasChanges(true);
                      }}
                      className={cn(
                        "relative inline-flex h-6 w-11 items-center rounded-full transition-colors",
                        forceSSL ? "bg-primary-600" : "bg-gray-300 dark:bg-gray-700"
                      )}
                    >
                      <span
                        className={cn(
                          "inline-block h-4 w-4 transform rounded-full bg-white transition-transform",
                          forceSSL ? "translate-x-6" : "translate-x-1"
                        )}
                      />
                    </button>
                  </div>

                  <div className="flex items-center justify-between pl-7">
                    <div>
                      <p className="text-sm font-medium text-gray-900 dark:text-white">HTTP/2</p>
                      <p className="text-xs text-gray-500">Enable HTTP/2 protocol for better performance</p>
                    </div>
                    <button
                      type="button"
                      onClick={() => {
                        setHTTP2Enabled(!http2Enabled);
                        setHasChanges(true);
                      }}
                      className={cn(
                        "relative inline-flex h-6 w-11 items-center rounded-full transition-colors",
                        http2Enabled ? "bg-primary-600" : "bg-gray-300 dark:bg-gray-700"
                      )}
                    >
                      <span
                        className={cn(
                          "inline-block h-4 w-4 transform rounded-full bg-white transition-transform",
                          http2Enabled ? "translate-x-6" : "translate-x-1"
                        )}
                      />
                    </button>
                  </div>
                </>
              )}
            </div>

            {(saveMutation.isError || deleteMutation.isError) && (
              <div className="p-3 bg-red-500/10 border border-red-500/30 rounded-lg text-red-400 text-sm">
                {saveMutation.error?.message || deleteMutation.error?.message || "An error occurred"}
              </div>
            )}

            <div className="flex items-center justify-between pt-2">
              {isConfigured && !showDelete ? (
                <button
                  onClick={() => setShowDelete(true)}
                  className="text-sm text-red-400 hover:text-red-300 flex items-center gap-1.5"
                >
                  <Trash2 className="h-4 w-4" />
                  Remove Domain
                </button>
              ) : showDelete ? (
                <div className="flex items-center gap-2">
                  <span className="text-sm text-gray-500">Are you sure?</span>
                  <button
                    onClick={() => deleteMutation.mutate()}
                    disabled={deleteMutation.isPending}
                    className="px-3 py-1.5 text-sm bg-red-600 hover:bg-red-700 text-white rounded-lg"
                  >
                    {deleteMutation.isPending ? "Removing..." : "Yes, Remove"}
                  </button>
                  <button
                    onClick={() => setShowDelete(false)}
                    className="px-3 py-1.5 text-sm text-gray-500 hover:text-gray-300"
                  >
                    Cancel
                  </button>
                </div>
              ) : (
                <div />
              )}

              {hasChanges && domain && (
                <button
                  onClick={() => saveMutation.mutate()}
                  disabled={saveMutation.isPending || !domain}
                  className="px-4 py-2 bg-primary-600 hover:bg-primary-700 disabled:bg-primary-400 text-white rounded-lg transition-colors flex items-center gap-2"
                >
                  {saveMutation.isPending ? (
                    <Loader2 className="h-4 w-4 animate-spin" />
                  ) : (
                    <Check className="h-4 w-4" />
                  )}
                  {saveMutation.isPending ? "Saving..." : "Save Domain"}
                </button>
              )}
            </div>
          </div>
        )}
      </Card.Body>
    </Card>

    {showSSLWizard && domain && (
      <SSLWizard
        domain={domain}
        open={showSSLWizard}
        onOpenChange={setShowSSLWizard}
        onSuccess={() => {
          queryClient.invalidateQueries({ queryKey: ["infrapilotDomain"] });
          setShowSSLWizard(false);
        }}
      />
    )}
    </>
  );
}

function SoftwareUpdateSection() {
  const [showUpdate, setShowUpdate] = useState(false);
  const [displayVersion, setDisplayVersion] = useState<string>("");

  const { data: versionInfo } = useQuery({
    queryKey: ["appVersion"],
    queryFn: () => api.getVersion(),
    staleTime: 5 * 60 * 1000,
  });

  const { data: updateInfo, isFetching, refetch } = useQuery({
    queryKey: ["updateCheck"],
    queryFn: () => api.checkForUpdate(),
    enabled: false, // only fetch on demand
    retry: false,
  });

  // Keep displayVersion stable — never let it go blank once we have a value
  const resolvedVersion = versionInfo?.version || updateInfo?.current_version || "";
  if (resolvedVersion && resolvedVersion !== displayVersion) {
    setDisplayVersion(resolvedVersion);
  }

  const pushedAt = updateInfo?.latest_pushed_at
    ? new Date(updateInfo.latest_pushed_at).toLocaleDateString(undefined, {
        year: "numeric", month: "short", day: "numeric",
      })
    : null;

  return (
    <Card>
      <Card.Header>
        <div className="flex items-center gap-3">
          <div className="p-2 bg-primary-500/10 rounded-lg">
            <ArrowUpCircle className="h-5 w-5 text-primary-400" />
          </div>
          <div>
            <h2 className="text-lg font-semibold text-gray-900 dark:text-white">Software Update</h2>
            <p className="text-sm text-gray-600 dark:text-gray-400">
              Check for and apply the latest InfraPilot CE release
            </p>
          </div>
        </div>
      </Card.Header>
      <Card.Body>
        <div className="space-y-4">
          {/* Version row */}
          <div className="flex items-center justify-between p-4 bg-gray-50 dark:bg-gray-800/50 rounded-lg border border-gray-200 dark:border-gray-700">
            <div>
              <p className="text-sm font-medium text-gray-900 dark:text-white">Current version</p>
              <p className="text-xs text-gray-500 font-mono mt-0.5">
                {displayVersion || "—"}
              </p>
            </div>
            <button
              onClick={() => refetch()}
              disabled={isFetching}
              className="flex items-center gap-2 px-3 py-1.5 text-sm bg-gray-100 dark:bg-gray-700 hover:bg-gray-200 dark:hover:bg-gray-600 text-gray-700 dark:text-gray-300 rounded-lg transition-colors disabled:opacity-50"
            >
              <RefreshCw className={cn("h-3.5 w-3.5", isFetching && "animate-spin")} />
              Check for updates
            </button>
          </div>

          {/* Result from check */}
          {updateInfo && !isFetching && (
            <>
              {updateInfo.error ? (
                <div className="flex items-start gap-3 p-4 bg-yellow-500/5 border border-yellow-500/20 rounded-lg">
                  <AlertCircle className="h-4 w-4 text-yellow-400 mt-0.5 shrink-0" />
                  <p className="text-sm text-yellow-400">{updateInfo.error}</p>
                </div>
              ) : updateInfo.update_available ? (
                <div className="flex items-center justify-between p-4 bg-primary-500/5 border border-primary-500/20 rounded-lg">
                  <div>
                    <p className="text-sm font-medium text-primary-400">Update available</p>
                    {pushedAt && (
                      <p className="text-xs text-gray-500 mt-0.5">Latest pushed {pushedAt}</p>
                    )}
                  </div>
                  <button
                    onClick={() => setShowUpdate(true)}
                    className="flex items-center gap-2 px-4 py-2 bg-primary-600 hover:bg-primary-700 text-white text-sm font-medium rounded-lg transition-colors"
                  >
                    <ArrowUpCircle className="h-4 w-4" />
                    Update Now
                  </button>
                </div>
              ) : (
                <div className="flex items-center gap-3 p-4 bg-green-500/5 border border-green-500/20 rounded-lg">
                  <Check className="h-4 w-4 text-green-400 shrink-0" />
                  <p className="text-sm text-green-400">InfraPilot is up to date</p>
                </div>
              )}
            </>
          )}

        </div>
      </Card.Body>

      {showUpdate && (
        <UpdateProgressModal
          title="Updating InfraPilot"
          icon="update"
          streamPath="/settings/update/apply/stream"
          steps={[
            { key: "download", label: "Download latest release" },
            { key: "restart", label: "Apply update & restart" },
          ]}
          verifyMode="version-changed"
          baselineVersion={displayVersion}
          successMessage="InfraPilot is up to date and running the latest release."
          onClose={() => setShowUpdate(false)}
          onDone={() => refetch()}
        />
      )}
    </Card>
  );
}

function APIKeysSection() {
  const queryClient = useQueryClient();
  const [showCreate, setShowCreate] = useState(false);
  const [newKeyName, setNewKeyName] = useState("");
  const [createdKey, setCreatedKey] = useState<APIKeyWithSecret | null>(null);
  const [copied, setCopied] = useState(false);
  const [visible, setVisible] = useState(false);
  const [deleteConfirm, setDeleteConfirm] = useState<string | null>(null);

  const { data: keys = [], isLoading } = useQuery({
    queryKey: ["api-keys"],
    queryFn: () => api.listAPIKeys(),
  });

  const createMutation = useMutation({
    mutationFn: () => api.createAPIKey(newKeyName.trim()),
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: ["api-keys"] });
      setCreatedKey(data);
      setNewKeyName("");
      setShowCreate(false);
      setVisible(false);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.deleteAPIKey(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["api-keys"] });
      setDeleteConfirm(null);
    },
  });

  const copyKey = (key: string) => {
    navigator.clipboard.writeText(key);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <Card className="p-6">
      <div className="flex items-center justify-between mb-6">
        <div className="flex items-center gap-3">
          <div className="p-2 bg-gray-100 dark:bg-gray-800 rounded-lg">
            <Key className="h-5 w-5 text-gray-600 dark:text-gray-400" />
          </div>
          <div>
            <h2 className="text-lg font-semibold text-gray-900 dark:text-white">API Keys</h2>
            <p className="text-sm text-gray-500 dark:text-gray-400">
              Authenticate the CLI and external integrations
            </p>
          </div>
        </div>
        <button
          onClick={() => setShowCreate(true)}
          className="flex items-center gap-2 px-3 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700 transition-colors text-sm"
        >
          <Plus className="h-4 w-4" />
          New Key
        </button>
      </div>

      {/* CLI hint */}
      <div className="mb-4 p-3 bg-gray-50 dark:bg-gray-800/50 rounded-lg border border-gray-200 dark:border-gray-700 flex items-start gap-3">
        <Terminal className="h-4 w-4 text-gray-400 mt-0.5 flex-shrink-0" />
        <div className="text-xs text-gray-500 dark:text-gray-400 font-mono">
          infrapilot connect --url https://infra.example.com --api-key &lt;key&gt;
        </div>
      </div>

      {/* Created key banner — shown once */}
      {createdKey && (
        <div className="mb-4 p-4 bg-green-50 dark:bg-green-900/20 border border-green-200 dark:border-green-800 rounded-lg">
          <div className="flex items-center gap-2 mb-2">
            <CheckCircle className="h-4 w-4 text-green-600" />
            <span className="text-sm font-medium text-green-800 dark:text-green-200">
              Key created — copy it now, it won't be shown again
            </span>
          </div>
          <div className="flex items-center gap-2">
            <code className="flex-1 px-3 py-2 bg-white dark:bg-gray-900 border border-green-200 dark:border-green-700 rounded text-sm font-mono text-gray-900 dark:text-white truncate">
              {visible ? createdKey.key : createdKey.key.slice(0, 16) + "•".repeat(20)}
            </code>
            <button
              onClick={() => setVisible(!visible)}
              className="p-2 text-gray-500 hover:text-gray-700 dark:hover:text-white rounded"
            >
              {visible ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
            </button>
            <button
              onClick={() => copyKey(createdKey.key)}
              className="p-2 text-gray-500 hover:text-gray-700 dark:hover:text-white rounded"
            >
              {copied ? <CheckCircle className="h-4 w-4 text-green-600" /> : <Copy className="h-4 w-4" />}
            </button>
          </div>
          <button
            onClick={() => setCreatedKey(null)}
            className="mt-2 text-xs text-green-700 dark:text-green-400 hover:underline"
          >
            Dismiss
          </button>
        </div>
      )}

      {/* Create form */}
      {showCreate && (
        <div className="mb-4 p-4 border border-gray-200 dark:border-gray-700 rounded-lg bg-gray-50 dark:bg-gray-800/50">
          <p className="text-sm font-medium text-gray-900 dark:text-white mb-3">New API Key</p>
          <div className="flex gap-2">
            <input
              type="text"
              value={newKeyName}
              onChange={(e) => setNewKeyName(e.target.value)}
              placeholder="Key name (e.g. CI pipeline, local dev)"
              className="flex-1 px-3 py-2 text-sm bg-white dark:bg-gray-900 border border-gray-200 dark:border-gray-700 rounded-lg text-gray-900 dark:text-white placeholder-gray-400 focus:ring-2 focus:ring-primary-500 focus:border-transparent"
              onKeyDown={(e) => e.key === "Enter" && newKeyName.trim() && createMutation.mutate()}
              autoFocus
            />
            <button
              onClick={() => createMutation.mutate()}
              disabled={!newKeyName.trim() || createMutation.isPending}
              className="px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700 disabled:opacity-50 text-sm"
            >
              {createMutation.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : "Create"}
            </button>
            <button
              onClick={() => { setShowCreate(false); setNewKeyName(""); }}
              className="px-3 py-2 text-gray-600 dark:text-gray-400 hover:bg-gray-200 dark:hover:bg-gray-700 rounded-lg text-sm"
            >
              Cancel
            </button>
          </div>
        </div>
      )}

      {/* Keys list */}
      {isLoading ? (
        <div className="flex items-center gap-2 text-sm text-gray-500 py-4">
          <Loader2 className="h-4 w-4 animate-spin" /> Loading...
        </div>
      ) : keys.length === 0 ? (
        <div className="text-center py-8 text-sm text-gray-400 dark:text-gray-500">
          No API keys yet. Create one to connect the CLI.
        </div>
      ) : (
        <div className="divide-y divide-gray-100 dark:divide-gray-800">
          {keys.map((k: APIKey) => (
            <div key={k.id} className="py-3 flex items-center gap-4">
              <div className="p-1.5 bg-gray-100 dark:bg-gray-800 rounded">
                <Key className="h-3.5 w-3.5 text-gray-500" />
              </div>
              <div className="flex-1 min-w-0">
                <p className="text-sm font-medium text-gray-900 dark:text-white">{k.name}</p>
                <p className="text-xs text-gray-400 font-mono">{k.key_prefix}</p>
              </div>
              <div className="text-right text-xs text-gray-400 hidden sm:block">
                {k.last_used_at ? (
                  <span>Used {new Date(k.last_used_at).toLocaleDateString()}</span>
                ) : (
                  <span>Never used</span>
                )}
                <br />
                <span>Created {new Date(k.created_at).toLocaleDateString()}</span>
              </div>
              {k.expires_at && (
                <Badge variant="status" status={new Date(k.expires_at) < new Date() ? "critical" : "warning"} size="sm">
                  {new Date(k.expires_at) < new Date() ? "Expired" : `Expires ${new Date(k.expires_at).toLocaleDateString()}`}
                </Badge>
              )}
              {deleteConfirm === k.id ? (
                <div className="flex items-center gap-2">
                  <span className="text-xs text-gray-500">Sure?</span>
                  <button
                    onClick={() => deleteMutation.mutate(k.id)}
                    disabled={deleteMutation.isPending}
                    className="text-xs text-red-600 hover:underline"
                  >
                    Delete
                  </button>
                  <button onClick={() => setDeleteConfirm(null)} className="text-xs text-gray-500 hover:underline">
                    Cancel
                  </button>
                </div>
              ) : (
                <button
                  onClick={() => setDeleteConfirm(k.id)}
                  className="p-1.5 text-gray-400 hover:text-red-600 rounded"
                >
                  <Trash2 className="h-4 w-4" />
                </button>
              )}
            </div>
          ))}
        </div>
      )}
    </Card>
  );
}

function PrivacySection() {
  const queryClient = useQueryClient();
  const { data, isLoading } = useQuery({
    queryKey: ["privacySettings"],
    queryFn: () => api.getPrivacySettings(),
  });

  const mutation = useMutation({
    mutationFn: (enabled: boolean) => api.updatePrivacySettings(enabled),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["privacySettings"] }),
  });

  const enabled = data?.telemetry_enabled ?? true;

  return (
    <Card>
      <Card.Header>
        <div className="flex items-center gap-3">
          <div className="p-2 bg-primary-500/10 rounded-lg">
            <Lock className="h-5 w-5 text-primary-400" />
          </div>
          <div>
            <h2 className="text-lg font-semibold text-gray-900 dark:text-white">Privacy</h2>
            <p className="text-sm text-gray-600 dark:text-gray-400">
              Control what this instance shares with infrapilot.org
            </p>
          </div>
        </div>
      </Card.Header>
      <Card.Body>
        <div className="flex items-center justify-between p-4 bg-gray-50 dark:bg-gray-800/50 rounded-lg border border-gray-200 dark:border-gray-700">
          <div className="pr-4">
            <p className="text-sm font-medium text-gray-900 dark:text-white">Anonymous usage telemetry</p>
            <p className="text-xs text-gray-500 dark:text-gray-400 mt-0.5">
              Sends an anonymous instance ID, install/deploy milestones, and coarse version/OS
              info to help us see what&apos;s working — never app names, repo URLs, env vars, or
              any of your infrastructure&apos;s content. Can also be disabled with the
              <code className="mx-1 px-1 py-0.5 bg-gray-200 dark:bg-gray-700 rounded text-[11px]">
                INFRAPILOT_TELEMETRY=off
              </code>
              environment variable.
            </p>
          </div>
          <button
            type="button"
            role="switch"
            aria-checked={enabled}
            onClick={() => mutation.mutate(!enabled)}
            disabled={isLoading || mutation.isPending}
            className={cn(
              "relative inline-flex h-5 w-9 flex-shrink-0 items-center rounded-full transition-colors disabled:opacity-50",
              enabled ? "bg-primary-600" : "bg-gray-300 dark:bg-gray-700"
            )}
          >
            <span
              className={cn(
                "inline-block h-3.5 w-3.5 transform rounded-full bg-white transition-transform",
                enabled ? "translate-x-5" : "translate-x-1"
              )}
            />
          </button>
        </div>
      </Card.Body>
    </Card>
  );
}

export default function SettingsGeneralPage() {
  return (
    <div className="space-y-6">
      <SoftwareUpdateSection />
      <PrivacySection />
      <InfraPilotDomainSection />
      <APIKeysSection />
    </div>
  );
}
