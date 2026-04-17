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
} from "lucide-react";
import { api } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Card } from "@/components/ui/Card";
import { Badge } from "@/components/ui/Badge";

function InfraPilotDomainSection() {
  const queryClient = useQueryClient();
  const [domain, setDomain] = useState("");
  const [sslEnabled, setSSLEnabled] = useState(true);
  const [forceSSL, setForceSSL] = useState(true);
  const [http2Enabled, setHTTP2Enabled] = useState(true);
  const [hasChanges, setHasChanges] = useState(false);
  const [showDelete, setShowDelete] = useState(false);

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
                <Badge className="bg-green-500/10 text-green-400 border-green-500/30">
                  Active
                </Badge>
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
  );
}

function SoftwareUpdateSection() {
  const [applyResult, setApplyResult] = useState<{ status: string; message: string } | null>(null);
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

  const applyMutation = useMutation({
    mutationFn: () => api.applyUpdate(),
    onSuccess: (data) => setApplyResult(data),
    onError: (err: Error) => {
      // A network error here likely means the container restarted mid-response — treat as success.
      if (err.message === "Failed to fetch" || err.name === "TypeError") {
        setApplyResult({ status: "restarting", message: "Update applied. InfraPilot is restarting — refresh in a few seconds." });
      }
    },
  });

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
                    onClick={() => applyMutation.mutate()}
                    disabled={applyMutation.isPending || applyResult?.status === "restarting"}
                    className="flex items-center gap-2 px-4 py-2 bg-primary-600 hover:bg-primary-700 disabled:bg-primary-400 text-white text-sm font-medium rounded-lg transition-colors"
                  >
                    {applyMutation.isPending ? (
                      <Loader2 className="h-4 w-4 animate-spin" />
                    ) : (
                      <ArrowUpCircle className="h-4 w-4" />
                    )}
                    {applyMutation.isPending ? "Updating…" : "Update Now"}
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

          {/* Apply result */}
          {applyResult && (
            <div className={cn(
              "p-4 rounded-lg border text-sm",
              applyResult.status === "restarting"
                ? "bg-primary-500/5 border-primary-500/20 text-primary-400"
                : applyResult.status === "error"
                ? "bg-red-500/5 border-red-500/20 text-red-400"
                : "bg-yellow-500/5 border-yellow-500/20 text-yellow-400"
            )}>
              {applyResult.message}
              {applyResult.status === "restarting" && (
                <span className="ml-2 inline-flex items-center gap-1">
                  <Loader2 className="h-3.5 w-3.5 animate-spin" />
                  Restarting…
                </span>
              )}
            </div>
          )}

          {applyMutation.isError && (
            <div className="p-4 bg-red-500/5 border border-red-500/20 rounded-lg text-sm text-red-400">
              {applyMutation.error?.message ?? "Update failed"}
            </div>
          )}
        </div>
      </Card.Body>
    </Card>
  );
}

export default function SettingsGeneralPage() {
  return (
    <div className="space-y-6">
      <SoftwareUpdateSection />
      <InfraPilotDomainSection />
    </div>
  );
}
