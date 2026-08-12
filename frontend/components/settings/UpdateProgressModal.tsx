"use client";

import { useEffect, useRef, useState } from "react";
import {
  Loader2,
  X,
  AlertTriangle,
  CheckCircle2,
  Rocket,
  ArrowUpCircle,
} from "lucide-react";
import { api } from "@/lib/api";
import { cn } from "@/lib/utils";

const API_BASE = process.env.NEXT_PUBLIC_API_URL || "/api/v1";

export type StepStatus = "pending" | "running" | "done" | "error";

export interface StepDef {
  key: string;
  label: string;
}

function StepIcon({ status }: { status: StepStatus }) {
  if (status === "done")
    return <CheckCircle2 className="h-5 w-5 text-green-400 flex-shrink-0" />;
  if (status === "error")
    return <AlertTriangle className="h-5 w-5 text-red-400 flex-shrink-0" />;
  if (status === "running")
    return <Loader2 className="h-5 w-5 text-primary-400 animate-spin flex-shrink-0" />;
  return (
    <div className="h-5 w-5 rounded-full border-2 border-gray-300 dark:border-gray-700 flex-shrink-0" />
  );
}

// verifyMode controls how the final "verify" step decides success after the restart:
//  - "version-changed": running /version.version differs from the pre-update value (self-update)
//  - "edition-enterprise": running /version.edition === "enterprise" (CE→EE upgrade)
export type VerifyMode = "version-changed" | "edition-enterprise";

interface Props {
  title: string;
  icon?: "rocket" | "update";
  streamPath: string; // SSE endpoint, e.g. "/settings/update/apply/stream"
  steps: StepDef[]; // steps BEFORE verify; a "verify" step is appended automatically
  verifyMode: VerifyMode;
  baselineVersion?: string; // required for verifyMode="version-changed"
  successMessage: string;
  onClose: () => void;
  onDone?: () => void;
}

// UpdateProgressModal streams a backend SSE job (self-update or CE→EE upgrade) as a live
// step list, then polls the public /version endpoint across the container restart to
// confirm the outcome — detecting an auto-revert on failure.
export function UpdateProgressModal({
  title,
  icon = "update",
  streamPath,
  steps,
  verifyMode,
  baselineVersion,
  successMessage,
  onClose,
  onDone,
}: Props) {
  const allSteps: StepDef[] = [...steps, { key: "verify", label: "Verify" }];
  const initial: Record<string, StepStatus> = {};
  allSteps.forEach((s) => (initial[s.key] = "pending"));

  const [statuses, setStatuses] = useState<Record<string, StepStatus>>(initial);
  const [progressLine, setProgressLine] = useState("");
  const [error, setError] = useState("");
  const [succeeded, setSucceeded] = useState(false);
  const [reconnecting, setReconnecting] = useState(false);
  const startedRef = useRef(false);

  const setStep = (k: string, s: StepStatus) =>
    setStatuses((prev) => ({ ...prev, [k]: s }));

  async function pollVerify() {
    setReconnecting(true);
    setStep("verify", "running");
    const deadline = Date.now() + 6 * 60 * 1000;
    while (Date.now() < deadline) {
      await new Promise((r) => setTimeout(r, 2500));
      // Failure signal (upgrade only): the helper reverted and the old image came back.
      if (verifyMode === "edition-enterprise") {
        try {
          const st = await api.getUpgradeStatus();
          if (st.state === "failed") {
            setError(st.error || "Enterprise failed to start; reverted to Community Edition.");
            setStep("verify", "error");
            setReconnecting(false);
            return;
          }
        } catch {
          /* container restarting — keep waiting */
        }
      }
      // Success signal: the running image reports the expected new state (image-agnostic).
      try {
        const res = await fetch(`${API_BASE}/version`, { cache: "no-store" });
        if (res.ok) {
          const v = await res.json();
          const ok =
            verifyMode === "edition-enterprise"
              ? v.edition === "enterprise"
              : baselineVersion
                ? v.version && v.version !== baselineVersion
                : false;
          if (ok) {
            setStep("verify", "done");
            setSucceeded(true);
            setReconnecting(false);
            onDone?.();
            return;
          }
        }
      } catch {
        /* container restarting — keep waiting */
      }
    }
    setError(
      "This is taking longer than expected. Refresh in a minute to check the result."
    );
    setStep("verify", "error");
    setReconnecting(false);
  }

  async function run() {
    setStatuses({ ...initial });
    setError("");
    setProgressLine("");
    const token =
      typeof window !== "undefined" ? localStorage.getItem("access_token") : null;
    try {
      const res = await fetch(`${API_BASE}${streamPath}`, {
        headers: { Authorization: token ? `Bearer ${token}` : "" },
      });
      if (!res.ok || !res.body) {
        throw new Error(`Could not start (HTTP ${res.status})`);
      }
      const reader = res.body.getReader();
      const dec = new TextDecoder();
      let buf = "";
      for (;;) {
        const { value, done } = await reader.read();
        if (done) break;
        buf += dec.decode(value, { stream: true });
        const frames = buf.split("\n\n");
        buf = frames.pop() ?? "";
        for (const frame of frames) {
          let ev = "message";
          let dataStr = "";
          for (const line of frame.split("\n")) {
            if (line.startsWith("event:")) ev = line.slice(6).trim();
            else if (line.startsWith("data:")) dataStr += line.slice(5).trim();
          }
          let data: Record<string, unknown> = {};
          try {
            data = dataStr ? JSON.parse(dataStr) : {};
          } catch {
            /* ignore malformed frame */
          }
          if (ev === "step") {
            const k = data.step as string;
            if (k) setStep(k, data.status === "done" ? "done" : "running");
          } else if (ev === "progress") {
            if (data.line) setProgressLine(String(data.line));
          } else if (ev === "error") {
            const k = data.step as string;
            if (k) setStep(k, "error");
            setError((data.error as string) || "Update failed.");
            return;
          } else if (ev === "done") {
            // Mark the last non-verify step done, then poll for the restart outcome.
            const lastStep = steps[steps.length - 1]?.key;
            if (lastStep) setStep(lastStep, "done");
            await pollVerify();
            return;
          }
        }
      }
      await pollVerify();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Update failed to start.");
    }
  }

  useEffect(() => {
    if (startedRef.current) return;
    startedRef.current = true;
    void run();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const busy = !succeeded && !error;
  const HeaderIcon = icon === "rocket" ? Rocket : ArrowUpCircle;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4">
      <div className="w-full max-w-md bg-white dark:bg-gray-900 rounded-2xl border border-gray-200 dark:border-gray-800 shadow-xl">
        <div className="flex items-center justify-between p-5 border-b border-gray-200 dark:border-gray-800">
          <div className="flex items-center gap-2.5">
            <HeaderIcon
              className={cn("h-5 w-5", icon === "rocket" ? "text-purple-400" : "text-primary-400")}
            />
            <h3 className="font-semibold text-gray-900 dark:text-white">{title}</h3>
          </div>
          {!busy && (
            <button
              onClick={onClose}
              className="text-gray-400 hover:text-gray-600 dark:hover:text-gray-200"
            >
              <X className="h-5 w-5" />
            </button>
          )}
        </div>

        <div className="p-5 space-y-3">
          {allSteps.map(({ key, label }) => (
            <div key={key} className="flex items-center gap-3">
              <StepIcon status={statuses[key]} />
              <div className="flex-1 min-w-0">
                <span
                  className={cn(
                    "text-sm",
                    statuses[key] === "pending"
                      ? "text-gray-400"
                      : "text-gray-900 dark:text-gray-100"
                  )}
                >
                  {label}
                </span>
                {statuses[key] === "running" && key !== "verify" && progressLine && (
                  <p className="text-xs text-gray-400 font-mono truncate">{progressLine}</p>
                )}
                {key === "verify" && reconnecting && (
                  <p className="text-xs text-gray-400">Restarting — reconnecting…</p>
                )}
              </div>
            </div>
          ))}

          {error && (
            <div className="mt-2 p-3 bg-red-500/10 border border-red-500/30 rounded-lg text-red-400 text-sm flex items-start gap-2">
              <AlertTriangle className="h-4 w-4 mt-0.5 flex-shrink-0" />
              <span>{error}</span>
            </div>
          )}
          {succeeded && (
            <div className="mt-2 p-3 bg-green-500/10 border border-green-500/30 rounded-lg text-green-400 text-sm flex items-start gap-2">
              <CheckCircle2 className="h-4 w-4 mt-0.5 flex-shrink-0" />
              <span>{successMessage}</span>
            </div>
          )}
        </div>

        <div className="p-5 border-t border-gray-200 dark:border-gray-800 flex justify-end gap-2">
          {error && (
            <>
              <button
                onClick={onClose}
                className="px-4 py-2 text-sm text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-white"
              >
                Close
              </button>
              <button
                onClick={() => void run()}
                className="px-4 py-2 bg-primary-600 hover:bg-primary-700 text-white rounded-lg text-sm"
              >
                Retry
              </button>
            </>
          )}
          {succeeded && (
            <button
              onClick={() => window.location.reload()}
              className="px-4 py-2 bg-primary-600 hover:bg-primary-700 text-white rounded-lg text-sm"
            >
              Reload now
            </button>
          )}
          {busy && <span className="text-xs text-gray-400">Please keep this tab open…</span>}
        </div>
      </div>
    </div>
  );
}
