"use client";

import { ReactNode } from "react";
import { usePathname, useRouter } from "next/navigation";
import { RefreshCw, Plus } from "lucide-react";
import { useQueryClient, useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { PageHeader } from "@/components/ui/PageHeader";
import { Breadcrumb } from "@/components/ui/Breadcrumb";
import { cn } from "@/lib/utils";

const tabs = [
  { id: "channels", label: "Channels", href: "/alerts/channels" },
  { id: "rules",    label: "Rules",    href: "/alerts/rules" },
  { id: "history",  label: "History",  href: "/alerts/history" },
];

export default function AlertsLayout({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const queryClient = useQueryClient();

  const getActiveTab = () => {
    const segment = pathname.split("/")[2];
    return segment || "channels";
  };
  const activeTab = getActiveTab();

  // Alert channels are a "connected" feature (doc 35) — hide the channel-specific
  // "Add Channel" action when the license lacks it, so it doesn't leak past the
  // FeatureGate on the channels page. (Tabs stay: Rules/History are free.)
  const { data: license } = useQuery({
    queryKey: ["licenseTierInfo"],
    queryFn: () => api.getLicenseTierInfo(),
    staleTime: 60 * 1000,
  });
  const hasChannels = license?.features?.includes("email_alerts") ?? false;

  const getActionButton = () => {
    if (activeTab === "channels" && hasChannels) {
      return (
        <button
          onClick={() => router.push("/alerts/channels?action=add")}
          className="inline-flex items-center gap-2 px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700 transition-colors text-sm font-medium"
        >
          <Plus className="w-4 h-4" />
          Add Channel
        </button>
      );
    }
    if (activeTab === "rules") {
      return (
        <button
          onClick={() => router.push("/alerts/rules?action=add")}
          className="inline-flex items-center gap-2 px-4 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700 transition-colors text-sm font-medium"
        >
          <Plus className="w-4 h-4" />
          Add Rule
        </button>
      );
    }
    return null;
  };

  return (
    <div className="space-y-6">
      <PageHeader
        title="Alerts"
        description="Configure notification channels and alert rules"
        breadcrumbs={<Breadcrumb items={[{ label: "Run" }, { label: "Alerts" }]} />}
        action={
          <div className="flex items-center gap-3">
            {getActionButton()}
            <button
              onClick={() => queryClient.invalidateQueries()}
              className="inline-flex items-center gap-2 px-4 py-2 bg-gray-100 dark:bg-gray-800 hover:bg-gray-200 dark:hover:bg-gray-700 text-gray-900 dark:text-white rounded-lg transition-colors text-sm font-medium"
            >
              <RefreshCw className="w-4 h-4" />
              Refresh
            </button>
          </div>
        }
      />

      <div className="border-b border-gray-200 dark:border-gray-700">
        <nav className="-mb-px flex space-x-8">
          {tabs.map((tab) => (
            <button
              key={tab.id}
              onClick={() => router.push(tab.href)}
              className={cn(
                "whitespace-nowrap border-b-2 py-3 px-1 text-sm font-medium transition-colors",
                activeTab === tab.id
                  ? "border-primary-500 text-primary-600 dark:text-primary-400"
                  : "border-transparent text-gray-500 hover:border-gray-300 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300"
              )}
            >
              {tab.label}
            </button>
          ))}
        </nav>
      </div>

      {children}
    </div>
  );
}
