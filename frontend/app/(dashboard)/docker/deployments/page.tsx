"use client";

import { Zap, GitBranch, RefreshCw, RotateCcw, Webhook } from "lucide-react";

export default function DeploymentsPage() {
  return (
    <div className="flex flex-col items-center justify-center min-h-[60vh] text-center px-4">
      {/* Icon */}
      <div className="mb-6 w-20 h-20 rounded-2xl bg-gradient-to-br from-primary-100 to-purple-100 dark:from-primary-900/30 dark:to-purple-900/30 flex items-center justify-center">
        <GitBranch className="h-10 w-10 text-primary-600 dark:text-primary-400" />
      </div>

      {/* Heading */}
      <h1 className="text-2xl font-bold text-gray-900 dark:text-white mb-2">
        CD Deployments
      </h1>
      <p className="text-sm font-medium text-primary-600 dark:text-primary-400 mb-4">
        Available in Professional &amp; Enterprise
      </p>

      {/* Description */}
      <p className="max-w-md text-gray-500 dark:text-gray-400 mb-8">
        Automate container redeployments triggered by your CI pipeline. Create
        per-container webhook endpoints, track deployment history, and roll back
        to any previous image with a single click.
      </p>

      {/* Feature highlights */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 mb-10 w-full max-w-lg">
        {[
          { icon: Webhook,     label: "CI Webhooks",       desc: "Per-container webhook triggers" },
          { icon: RefreshCw,   label: "Auto Redeploy",     desc: "Pull latest image on push" },
          { icon: RotateCcw,   label: "One-step Rollback", desc: "Revert to any previous image" },
        ].map(({ icon: Icon, label, desc }) => (
          <div
            key={label}
            className="flex flex-col items-center gap-2 p-4 rounded-xl bg-gray-50 dark:bg-gray-800/50 border border-gray-200 dark:border-gray-700"
          >
            <Icon className="h-5 w-5 text-primary-500" />
            <span className="text-sm font-medium text-gray-900 dark:text-white">{label}</span>
            <span className="text-xs text-gray-500 dark:text-gray-400">{desc}</span>
          </div>
        ))}
      </div>

      {/* CTA */}
      <a
        href="https://infrapilot.org/enterprise"
        target="_blank"
        rel="noopener noreferrer"
        className="inline-flex items-center gap-2 px-6 py-3 bg-gradient-to-r from-primary-600 to-purple-600 hover:from-primary-700 hover:to-purple-700 text-white font-medium rounded-lg transition-all shadow-sm"
      >
        <Zap className="h-4 w-4" />
        Upgrade to Professional / Enterprise
      </a>
      <p className="mt-3 text-xs text-gray-400 dark:text-gray-500">
        Already have a license?{" "}
        <a href="/settings/license" className="text-primary-500 hover:underline">
          Activate it in Settings
        </a>
      </p>
    </div>
  );
}
