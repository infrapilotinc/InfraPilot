"use client";

import { ReactNode } from "react";
import { usePathname, useRouter } from "next/navigation";
import { Globe, Crown, Shield, FileText, Users, Server } from "lucide-react";
import { PageHeader } from "@/components/ui/PageHeader";
import { Breadcrumb } from "@/components/ui/Breadcrumb";
import { cn } from "@/lib/utils";

const tabs = [
  { id: "general", label: "General", href: "/settings", icon: Globe },
  { id: "license", label: "License", href: "/settings/license", icon: Crown },
  { id: "users", label: "Users", href: "/settings/users", icon: Users },
  { id: "agents", label: "Agents", href: "/settings/agents", icon: Server },
  { id: "security", label: "Security", href: "/settings/security", icon: Shield },
  { id: "pages", label: "Default Pages", href: "/settings/pages", icon: FileText },
];

export default function SettingsLayout({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();

  const getActiveTab = () => {
    if (pathname === "/settings") return "general";
    const segment = pathname.split("/")[2];
    return segment || "general";
  };

  const activeTab = getActiveTab();

  return (
    <div className="space-y-6">
      <PageHeader
        title="Settings"
        description="Configure InfraPilot for your infrastructure"
        breadcrumbs={
          <Breadcrumb
            items={[
              { label: "Platform" },
              { label: "Settings" },
            ]}
          />
        }
      />

      {/* Navigation Tabs */}
      <div className="border-b border-gray-200 dark:border-gray-700">
        <nav className="-mb-px flex space-x-8">
          {tabs.map((tab) => {
            const Icon = tab.icon;
            return (
              <button
                key={tab.id}
                onClick={() => router.push(tab.href)}
                className={cn(
                  "inline-flex items-center gap-2 whitespace-nowrap border-b-2 py-3 px-1 text-sm font-medium transition-colors",
                  activeTab === tab.id
                    ? "border-primary-500 text-primary-600 dark:text-primary-400"
                    : "border-transparent text-gray-500 hover:border-gray-300 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300"
                )}
              >
                <Icon className="w-4 h-4" />
                {tab.label}
              </button>
            );
          })}
        </nav>
      </div>

      {/* Tab Content */}
      {children}
    </div>
  );
}
