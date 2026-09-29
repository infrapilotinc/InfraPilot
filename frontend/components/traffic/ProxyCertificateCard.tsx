"use client";

import { useState } from "react";
import Link from "next/link";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, Check, Loader2, RefreshCw, ShieldCheck, Wand2 } from "lucide-react";
import { api, ProxyCertificateOption } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/page-layout";

const AUTO = "__auto__";

function expiryColor(daysLeft: number): "green" | "yellow" | "red" {
  if (daysLeft < 7) return "red";
  if (daysLeft < 30) return "yellow";
  return "green";
}

function formatDate(iso?: string) {
  return iso ? new Date(iso).toLocaleDateString() : "unknown";
}

function CertSummary({ cert }: { cert: ProxyCertificateOption }) {
  return (
    <div className="space-y-1.5">
      <div className="flex flex-wrap items-center gap-1.5">
        <span className="text-sm font-medium text-gray-900 dark:text-white break-all">{cert.name || cert.subject}</span>
        {cert.is_wildcard && <Badge color="purple" size="sm">Wildcard</Badge>}
        <Badge color={expiryColor(cert.days_left)} size="sm">
          {cert.days_left < 0 ? "Expired" : `${cert.days_left}d left`}
        </Badge>
      </div>
      <p className="text-xs text-gray-500">
        {cert.issuer ? `${cert.issuer} · ` : ""}expires {formatDate(cert.expires_at)}
      </p>
      {cert.sans.length > 0 && (
        <div className="flex flex-wrap gap-1">
          {cert.sans.map((san) => (
            <code key={san} className="px-1.5 py-0.5 text-[11px] rounded bg-gray-100 dark:bg-gray-800 text-gray-700 dark:text-gray-300">
              {san}
            </code>
          ))}
        </div>
      )}
      <p className="text-[11px] text-gray-400 font-mono break-all">{cert.cert_path}</p>
    </div>
  );
}

// Shows which certificate a proxy serves and lets the user switch to another
// existing one (or back to automatic Let's Encrypt matching).
export function ProxyCertificateCard({ agentId, proxyId }: { agentId: string; proxyId: string }) {
  const queryClient = useQueryClient();
  const [picking, setPicking] = useState(false);
  const [selected, setSelected] = useState<string>(AUTO);

  const { data, isLoading, refetch, isFetching } = useQuery({
    queryKey: ["proxy-certificate", agentId, proxyId],
    queryFn: () => api.getProxyCertificate(agentId, proxyId),
  });

  const setCert = useMutation({
    mutationFn: (value: string) =>
      api.setProxyCertificate(agentId, proxyId, value === AUTO ? { auto: true } : { cert_path: value }),
    onSuccess: () => {
      setPicking(false);
      queryClient.invalidateQueries({ queryKey: ["proxy-certificate", agentId, proxyId] });
      queryClient.invalidateQueries({ queryKey: ["proxy-detail"] });
      queryClient.invalidateQueries({ queryKey: ["proxy-config", agentId, proxyId] });
      queryClient.invalidateQueries({ queryKey: ["proxies"] });
    },
  });

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 text-xs text-gray-500">
        <Loader2 className="h-3.5 w-3.5 animate-spin" /> Loading certificate...
      </div>
    );
  }
  if (!data) return null;

  const covering = data.available.filter((c) => c.covers_domain);
  const others = data.available.filter((c) => !c.covers_domain);

  const openPicker = () => {
    setSelected(data.mode === "pinned" && data.current ? data.current.cert_path : AUTO);
    setPicking(true);
  };

  return (
    <div className="space-y-3">
      {data.ssl_enabled && (
        <div className="p-3 border border-gray-200 dark:border-gray-700 rounded-lg space-y-2">
          <div className="flex items-center justify-between">
            <span className="text-xs font-medium uppercase tracking-wide text-gray-500">Certificate in use</span>
            <div className="flex items-center gap-1.5">
              <Badge color={data.mode === "pinned" ? "blue" : "gray"} size="sm">
                {data.mode === "pinned" ? "Pinned" : "Auto"}
              </Badge>
              <button
                onClick={() => refetch()}
                className="p-1 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 rounded"
                title="Refresh"
              >
                <RefreshCw className={cn("h-3.5 w-3.5", isFetching && "animate-spin")} />
              </button>
            </div>
          </div>
          {data.current ? (
            <CertSummary cert={data.current} />
          ) : (
            <p className="text-xs text-gray-500">No certificate found.</p>
          )}
          {data.error && (
            <div className="flex items-start gap-2 p-2 rounded bg-red-50 dark:bg-red-900/20 text-xs text-red-700 dark:text-red-400">
              <AlertTriangle className="h-3.5 w-3.5 mt-0.5 shrink-0" />
              <span>{data.error}</span>
            </div>
          )}
        </div>
      )}

      {!picking ? (
        <Button variant="secondary" size="sm" icon={ShieldCheck} onClick={openPicker}>
          {data.ssl_enabled ? "Change certificate" : "Use an existing certificate"}
        </Button>
      ) : (
        <div className="p-3 border border-gray-200 dark:border-gray-700 rounded-lg space-y-2">
          <p className="text-xs font-medium text-gray-500">Choose a certificate for {data.domain}</p>

          <label
            className={cn(
              "flex items-start gap-2 p-2 rounded border cursor-pointer",
              selected === AUTO
                ? "border-primary-500 bg-primary-50 dark:bg-primary-900/20"
                : "border-gray-200 dark:border-gray-700"
            )}
          >
            <input
              type="radio"
              name="proxy-cert"
              checked={selected === AUTO}
              onChange={() => setSelected(AUTO)}
              className="mt-0.5"
            />
            <div>
              <span className="flex items-center gap-1 text-sm font-medium text-gray-900 dark:text-white">
                <Wand2 className="h-3.5 w-3.5" /> Automatic
              </span>
              <p className="text-xs text-gray-500">
                Use this domain&apos;s Let&apos;s Encrypt cert, or a wildcard for its parent domain.
              </p>
            </div>
          </label>

          {covering.map((cert) => (
            <label
              key={cert.cert_path}
              className={cn(
                "flex items-start gap-2 p-2 rounded border cursor-pointer",
                selected === cert.cert_path
                  ? "border-primary-500 bg-primary-50 dark:bg-primary-900/20"
                  : "border-gray-200 dark:border-gray-700"
              )}
            >
              <input
                type="radio"
                name="proxy-cert"
                checked={selected === cert.cert_path}
                onChange={() => setSelected(cert.cert_path)}
                className="mt-0.5"
              />
              <CertSummary cert={cert} />
            </label>
          ))}

          {covering.length === 0 && (
            <p className="text-xs text-gray-500">No existing certificate covers {data.domain}.</p>
          )}

          {others.length > 0 && (
            <details className="text-xs text-gray-500">
              <summary className="cursor-pointer">
                {others.length} other certificate{others.length > 1 ? "s" : ""} that don&apos;t cover this domain
              </summary>
              <ul className="mt-1 space-y-0.5 pl-3">
                {others.map((cert) => (
                  <li key={cert.cert_path} className="break-all">
                    {cert.name || cert.subject}: {cert.sans.join(", ")}
                  </li>
                ))}
              </ul>
            </details>
          )}

          {setCert.error && (
            <p className="text-xs text-red-600 dark:text-red-400">{(setCert.error as Error).message}</p>
          )}

          <div className="flex items-center justify-between pt-1">
            <Link href="/traffic/certificates" className="text-xs text-primary-600 hover:underline">
              Upload or issue a certificate
            </Link>
            <div className="flex gap-2">
              <Button variant="secondary" size="sm" onClick={() => setPicking(false)}>
                Cancel
              </Button>
              <Button
                variant="primary"
                size="sm"
                icon={setCert.isPending ? Loader2 : Check}
                disabled={setCert.isPending}
                onClick={() => setCert.mutate(selected)}
              >
                Apply
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
