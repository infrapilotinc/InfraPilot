"use client";

import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Dialog } from "@headlessui/react";
import {
  X,
  RotateCcw,
  Download,
  AlertTriangle,
  Shield,
  ShieldOff,
  Bookmark,
  Pencil,
} from "lucide-react";
import { api, ManagedStack, RedeployStackRequest } from "@/lib/api";
import { cn } from "@/lib/utils";

interface StackRedeployModalProps {
  isOpen: boolean;
  agentId?: string;
  stack: ManagedStack | null; // summary row from the list; full detail (services, saved
  // selection) is fetched on open since the list view doesn't carry it.
  onClose: () => void;
  onConfirm: (request: RedeployStackRequest) => void;
  isLoading?: boolean;
  errorMessage?: string | null;
}

// StackRedeployModal lets the user redeploy some or all of a managed stack's services at
// once, reusing its saved compose/variables/files unless they choose to update them, and
// optionally saving the service selection as the stack's new default.
export function StackRedeployModal({
  isOpen,
  agentId,
  stack,
  onClose,
  onConfirm,
  isLoading = false,
  errorMessage,
}: StackRedeployModalProps) {
  const { data: detail } = useQuery({
    queryKey: ["managed-stack-detail", agentId, stack?.id],
    queryFn: () => api.getManagedStack(agentId!, stack!.id),
    enabled: isOpen && !!agentId && !!stack?.id,
  });

  const serviceNames = Array.from(
    new Set((detail?.deployments ?? []).map((d) => d.service_name))
  ).sort();

  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [pullLatest, setPullLatest] = useState(true);
  const [skipScanning, setSkipScanning] = useState(false);
  const [saveSelection, setSaveSelection] = useState(false);
  const [updateConfig, setUpdateConfig] = useState(false);
  const [composeYaml, setComposeYaml] = useState("");
  const [variablesText, setVariablesText] = useState("");

  // Re-seed local state whenever the modal opens for a (possibly different) stack, and once
  // the detail fetch lands (pre-check the saved default selection, or everything if unset).
  useEffect(() => {
    if (!isOpen) return;
    setPullLatest(true);
    setSkipScanning(false);
    setSaveSelection(false);
    setUpdateConfig(false);
    setComposeYaml(detail?.compose_yaml ?? "");
    const vars = detail?.variables ?? {};
    setVariablesText(Object.entries(vars).map(([k, v]) => `${k}=${v}`).join("\n"));
  }, [isOpen, detail]);

  useEffect(() => {
    if (!isOpen || serviceNames.length === 0) return;
    const saved = detail?.redeploy_services;
    setSelected(new Set(saved && saved.length > 0 ? saved : serviceNames));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isOpen, detail?.id]);

  if (!stack) return null;

  const toggleService = (name: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(name)) next.delete(name);
      else next.add(name);
      return next;
    });
  };

  const parseVariables = (): Record<string, string> => {
    const out: Record<string, string> = {};
    for (const line of variablesText.split("\n")) {
      const trimmed = line.trim();
      if (!trimmed || trimmed.startsWith("#")) continue;
      const eq = trimmed.indexOf("=");
      if (eq <= 0) continue;
      out[trimmed.slice(0, eq).trim()] = trimmed.slice(eq + 1).trim();
    }
    return out;
  };

  const handleConfirm = () => {
    const request: RedeployStackRequest = {
      services: Array.from(selected),
      pull_latest: pullLatest,
      skip_scanning: skipScanning,
      save_selection: saveSelection,
    };
    if (updateConfig) {
      request.compose_yaml = composeYaml;
      request.variables = parseVariables();
      // Companion files aren't editable from this modal — if the updated compose introduces
      // a new relative bind mount, the backend rejects with a clear "must be supplied" error
      // (surfaced via errorMessage) rather than silently deploying broken.
    }
    onConfirm(request);
  };

  return (
    <Dialog open={isOpen} onClose={onClose} className="relative z-50">
      <div className="fixed inset-0 bg-black/80 backdrop-blur-sm" aria-hidden="true" />
      <div className="fixed inset-0 flex items-center justify-center p-4">
        <Dialog.Panel className="mx-auto max-w-2xl w-full max-h-[90vh] overflow-y-auto bg-zinc-900 rounded-xl border border-zinc-800 shadow-2xl">
          <div className="flex items-center justify-between p-6 border-b border-zinc-800">
            <div className="flex items-center gap-3">
              <div className="p-2 bg-indigo-500/10 rounded-lg">
                <RotateCcw className="h-5 w-5 text-indigo-400" />
              </div>
              <div>
                <Dialog.Title className="text-lg font-semibold text-white">
                  Redeploy {stack.name}
                </Dialog.Title>
                <p className="text-sm text-zinc-400 mt-0.5">
                  Choose which services to redeploy and whether to update their config
                </p>
              </div>
            </div>
            <button
              onClick={onClose}
              disabled={isLoading}
              className="p-2 hover:bg-zinc-800 rounded-lg transition-colors disabled:opacity-50"
            >
              <X className="h-5 w-5 text-zinc-400" />
            </button>
          </div>

          <div className="p-6 space-y-6">
            {/* Service selection */}
            <div>
              <h3 className="text-sm font-medium text-white mb-3">Services to redeploy</h3>
              <div className="bg-zinc-800/50 rounded-lg border border-zinc-700 divide-y divide-zinc-700">
                {serviceNames.length === 0 ? (
                  <div className="p-3 text-sm text-zinc-500">Loading services…</div>
                ) : (
                  serviceNames.map((name) => (
                    <label
                      key={name}
                      className="p-3 flex items-center gap-3 cursor-pointer hover:bg-zinc-800/50"
                    >
                      <input
                        type="checkbox"
                        checked={selected.has(name)}
                        onChange={() => toggleService(name)}
                        disabled={isLoading}
                        className="w-4 h-4 rounded border-zinc-600 bg-zinc-800 text-indigo-500 focus:ring-2 focus:ring-indigo-500 disabled:opacity-50"
                      />
                      <span className="text-sm text-white font-mono">{name}</span>
                    </label>
                  ))
                )}
              </div>
              <label className="flex items-center gap-2 mt-2 text-xs text-zinc-400 cursor-pointer">
                <input
                  type="checkbox"
                  checked={saveSelection}
                  onChange={(e) => setSaveSelection(e.target.checked)}
                  disabled={isLoading}
                  className="w-3.5 h-3.5 rounded border-zinc-600 bg-zinc-800 text-indigo-500"
                />
                <Bookmark className="h-3.5 w-3.5" />
                Remember this selection as the default for future redeploys of this stack
              </label>
            </div>

            {/* Update configuration (collapsed by default — "keep same" path) */}
            <div>
              <button
                type="button"
                onClick={() => setUpdateConfig((v) => !v)}
                disabled={isLoading}
                className="flex items-center gap-2 text-sm font-medium text-white mb-3 disabled:opacity-50"
              >
                <Pencil className="h-4 w-4 text-zinc-400" />
                {updateConfig ? "Updating configuration" : "Keeping existing configuration"}
                <span className="text-xs text-indigo-400 ml-1">
                  {updateConfig ? "(click to keep as-is instead)" : "(click to update YAML/variables)"}
                </span>
              </button>
              {updateConfig && (
                <div className="space-y-3 bg-zinc-800/50 rounded-lg border border-zinc-700 p-4">
                  <div>
                    <label className="block text-xs text-zinc-400 mb-1">Compose YAML</label>
                    <textarea
                      value={composeYaml}
                      onChange={(e) => setComposeYaml(e.target.value)}
                      disabled={isLoading}
                      rows={8}
                      className="w-full px-3 py-2 bg-zinc-900 border border-zinc-700 rounded-lg text-white font-mono text-xs resize-y focus:outline-none focus:ring-2 focus:ring-indigo-500 disabled:opacity-50"
                    />
                  </div>
                  <div>
                    <label className="block text-xs text-zinc-400 mb-1">
                      Variables (KEY=VALUE, one per line)
                    </label>
                    <textarea
                      value={variablesText}
                      onChange={(e) => setVariablesText(e.target.value)}
                      disabled={isLoading}
                      rows={4}
                      className="w-full px-3 py-2 bg-zinc-900 border border-zinc-700 rounded-lg text-white font-mono text-xs resize-y focus:outline-none focus:ring-2 focus:ring-indigo-500 disabled:opacity-50"
                    />
                  </div>
                  <p className="text-xs text-amber-400/80">
                    If this compose bind-mounts a new local file, redeploy will fail with a
                    clear error naming it — companion files can't be supplied from this
                    dialog yet; use the stack wizard for that.
                  </p>
                </div>
              )}
            </div>

            {/* Redeploy options — same as the single-deployment RedeployWizard */}
            <div>
              <h3 className="text-sm font-medium text-white mb-3">Redeployment Options</h3>
              <div className="bg-zinc-800/50 rounded-lg border border-zinc-700 p-4 space-y-4">
                <label className="flex items-start gap-3 cursor-pointer group">
                  <input
                    type="checkbox"
                    checked={pullLatest}
                    onChange={(e) => setPullLatest(e.target.checked)}
                    disabled={isLoading}
                    className="w-5 h-5 mt-0.5 rounded border-zinc-600 bg-zinc-800 text-indigo-500 focus:ring-2 focus:ring-indigo-500 disabled:opacity-50"
                  />
                  <div className="flex-1">
                    <div className="text-sm font-medium text-white group-hover:text-indigo-300">
                      Pull Latest Image
                      <span className="ml-2 text-xs font-normal text-emerald-400">(Recommended)</span>
                    </div>
                    <p className="text-xs text-zinc-400 mt-1">
                      {pullLatest ? (
                        <>
                          <Download className="h-3 w-3 inline mr-1" />
                          Pulls the latest image for each redeployed service before deploying
                        </>
                      ) : (
                        <span className="text-amber-400">
                          <AlertTriangle className="h-3 w-3 inline mr-1" />
                          Uses the cached image if available
                        </span>
                      )}
                    </p>
                  </div>
                </label>

                <label className="flex items-start gap-3 cursor-pointer group">
                  <input
                    type="checkbox"
                    checked={!skipScanning}
                    onChange={(e) => setSkipScanning(!e.target.checked)}
                    disabled={isLoading}
                    className="w-5 h-5 mt-0.5 rounded border-zinc-600 bg-zinc-800 text-indigo-500 focus:ring-2 focus:ring-indigo-500 disabled:opacity-50"
                  />
                  <div className="flex-1">
                    <div className="text-sm font-medium text-white group-hover:text-indigo-300">
                      Security Scanning
                    </div>
                    <p className="text-xs text-zinc-400 mt-1">
                      {!skipScanning ? (
                        <span className="text-emerald-400">
                          <Shield className="h-3 w-3 inline mr-1" />
                          Scans each redeployed image for vulnerabilities first
                        </span>
                      ) : (
                        <span className="text-amber-400">
                          <ShieldOff className="h-3 w-3 inline mr-1" />
                          Scanning disabled — deploys immediately
                        </span>
                      )}
                    </p>
                  </div>
                </label>
              </div>
            </div>

            {errorMessage && (
              <div className="p-3 bg-red-500/10 border border-red-500/30 rounded-lg text-red-400 text-sm flex items-start gap-2">
                <AlertTriangle className="h-4 w-4 mt-0.5 flex-shrink-0" />
                <span className="whitespace-pre-wrap">{errorMessage}</span>
              </div>
            )}
          </div>

          <div className="flex items-center justify-end gap-3 p-6 border-t border-zinc-800">
            <button
              onClick={onClose}
              disabled={isLoading}
              className="px-4 py-2 text-sm font-medium text-zinc-300 hover:text-white hover:bg-zinc-800 rounded-lg transition-colors disabled:opacity-50"
            >
              Cancel
            </button>
            <button
              onClick={handleConfirm}
              disabled={isLoading || selected.size === 0}
              className={cn(
                "px-5 py-2 text-sm font-medium rounded-lg transition-all flex items-center gap-2",
                "bg-indigo-500 hover:bg-indigo-600 text-white disabled:opacity-50 disabled:cursor-not-allowed"
              )}
            >
              <RotateCcw className="h-4 w-4" />
              {isLoading
                ? "Redeploying..."
                : `Redeploy ${selected.size || ""} Service${selected.size === 1 ? "" : "s"}`}
            </button>
          </div>
        </Dialog.Panel>
      </div>
    </Dialog>
  );
}
