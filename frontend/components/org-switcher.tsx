"use client";

import { useState, useEffect } from "react";
import { ChevronDown, Building2, Plus, Check, Settings } from "lucide-react";
import { api, Organization } from "@/lib/api";
import Link from "next/link";

interface OrgSwitcherProps {
  onOrgChange?: (orgId: string) => void;
}

export function OrgSwitcher({ onOrgChange }: OrgSwitcherProps) {
  const [orgs, setOrgs] = useState<Organization[]>([]);
  const [currentOrg, setCurrentOrg] = useState<Organization | null>(null);
  const [isOpen, setIsOpen] = useState(false);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const loadOrgs = async () => {
      try {
        const data = await api.getOrganizations();
        setOrgs(data || []);

        // Get stored org or use first one
        const storedOrgId = localStorage.getItem("current_org_id");
        const org = data?.find((o) => o.id === storedOrgId) || data?.[0];
        if (org) {
          setCurrentOrg(org);
          localStorage.setItem("current_org_id", org.id);
        }
      } catch (error) {
        // Silently handle - orgs may not be set up yet
        console.warn("Organizations not available:", error);
        setOrgs([]);
      } finally {
        setLoading(false);
      }
    };

    loadOrgs();
  }, []);

  const handleOrgSelect = (org: Organization) => {
    setCurrentOrg(org);
    localStorage.setItem("current_org_id", org.id);
    setIsOpen(false);
    onOrgChange?.(org.id);
  };

  if (loading) {
    return (
      <div className="px-3 py-2">
        <div className="h-10 bg-gray-100 dark:bg-gray-800 rounded-lg animate-pulse" />
      </div>
    );
  }

  if (orgs.length === 0) {
    // Show a placeholder when no orgs are available
    return (
      <div className="px-3 py-2">
        <div className="flex items-center gap-2 px-3 py-2 rounded-lg bg-gray-50 dark:bg-gray-800/50 text-gray-400">
          <Building2 className="h-4 w-4" />
          <span className="text-sm">No organization</span>
        </div>
      </div>
    );
  }

  // Single org - just show the name without dropdown
  if (orgs.length === 1) {
    return (
      <div className="px-3 py-2">
        <div className="flex items-center gap-2 px-3 py-2 rounded-lg bg-gray-50 dark:bg-gray-800/50">
          <Building2 className="h-4 w-4 text-gray-400" />
          <span className="text-sm font-medium text-gray-900 dark:text-white truncate">
            {currentOrg?.name}
          </span>
        </div>
      </div>
    );
  }

  return (
    <div className="px-3 py-2 relative">
      <button
        onClick={() => setIsOpen(!isOpen)}
        className="w-full flex items-center justify-between gap-2 px-3 py-2 rounded-lg bg-gray-50 dark:bg-gray-800/50 hover:bg-gray-100 dark:hover:bg-gray-800 transition-colors"
      >
        <div className="flex items-center gap-2 min-w-0">
          <Building2 className="h-4 w-4 text-gray-400 flex-shrink-0" />
          <span className="text-sm font-medium text-gray-900 dark:text-white truncate">
            {currentOrg?.name || "Select Organization"}
          </span>
        </div>
        <ChevronDown
          className={`h-4 w-4 text-gray-400 transition-transform ${
            isOpen ? "rotate-180" : ""
          }`}
        />
      </button>

      {isOpen && (
        <>
          {/* Backdrop */}
          <div
            className="fixed inset-0 z-10"
            onClick={() => setIsOpen(false)}
          />

          {/* Dropdown */}
          <div className="absolute left-3 right-3 top-full mt-1 z-20 bg-white dark:bg-gray-900 border border-gray-200 dark:border-gray-700 rounded-lg shadow-lg overflow-hidden">
            <div className="py-1 max-h-64 overflow-y-auto">
              {orgs.map((org) => (
                <button
                  key={org.id}
                  onClick={() => handleOrgSelect(org)}
                  className="w-full flex items-center justify-between px-3 py-2 text-left hover:bg-gray-50 dark:hover:bg-gray-800 transition-colors"
                >
                  <div className="flex items-center gap-2 min-w-0">
                    <Building2 className="h-4 w-4 text-gray-400 flex-shrink-0" />
                    <div className="min-w-0">
                      <p className="text-sm font-medium text-gray-900 dark:text-white truncate">
                        {org.name}
                      </p>
                      <p className="text-xs text-gray-500 truncate">
                        {org.member_role || "member"} · {org.plan}
                      </p>
                    </div>
                  </div>
                  {currentOrg?.id === org.id && (
                    <Check className="h-4 w-4 text-primary-600 flex-shrink-0" />
                  )}
                </button>
              ))}
            </div>

            <div className="border-t border-gray-200 dark:border-gray-700 py-1">
              <Link
                href="/orgs/new"
                onClick={() => setIsOpen(false)}
                className="flex items-center gap-2 px-3 py-2 text-sm text-gray-600 dark:text-gray-400 hover:bg-gray-50 dark:hover:bg-gray-800"
              >
                <Plus className="h-4 w-4" />
                Create Organization
              </Link>
              {currentOrg && (
                <Link
                  href={`/orgs/${currentOrg.id}/settings`}
                  onClick={() => setIsOpen(false)}
                  className="flex items-center gap-2 px-3 py-2 text-sm text-gray-600 dark:text-gray-400 hover:bg-gray-50 dark:hover:bg-gray-800"
                >
                  <Settings className="h-4 w-4" />
                  Organization Settings
                </Link>
              )}
            </div>
          </div>
        </>
      )}
    </div>
  );
}
