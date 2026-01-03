"use client";

import { cn } from "@/lib/utils";

interface PageLayoutProps {
  children: React.ReactNode;
  title: string;
  description?: string;
  actions?: React.ReactNode;
  panel?: React.ReactNode;
  panelOpen?: boolean;
}

export function PageLayout({
  children,
  title,
  description,
  actions,
  panel,
  panelOpen = false,
}: PageLayoutProps) {
  return (
    <div className="flex h-full max-h-full overflow-hidden -m-4 lg:-m-8">
      {/* Main content area */}
      <div
        className={cn(
          "flex-1 flex flex-col min-w-0 min-h-0 transition-all duration-300",
          panelOpen && panel ? "lg:mr-0" : ""
        )}
      >
        {/* Page header */}
        <div className="flex items-center justify-between p-4 lg:p-6 border-b border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-900 flex-shrink-0">
          <div>
            <h1 className="text-xl font-semibold text-gray-900 dark:text-white">
              {title}
            </h1>
            {description && (
              <p className="text-sm text-gray-500 dark:text-gray-400 mt-0.5">
                {description}
              </p>
            )}
          </div>
          {actions && <div className="flex items-center gap-2">{actions}</div>}
        </div>

        {/* Scrollable content */}
        <div className="flex-1 overflow-auto p-4 lg:p-6 min-h-0">{children}</div>
      </div>

      {/* Detail panel */}
      {panel}
    </div>
  );
}

// Card component for list items
export function ListCard({
  children,
  selected,
  onClick,
  className,
}: {
  children: React.ReactNode;
  selected?: boolean;
  onClick?: () => void;
  className?: string;
}) {
  return (
    <div
      onClick={onClick}
      className={cn(
        "p-4 bg-white dark:bg-gray-900 border rounded-lg transition-all duration-200 ease-out",
        selected
          ? "border-primary-500 ring-1 ring-primary-500 shadow-sm"
          : "border-gray-200 dark:border-gray-800 hover:border-gray-300 dark:hover:border-gray-700 hover:shadow-sm",
        onClick && "cursor-pointer active:scale-[0.99]",
        className
      )}
    >
      {children}
    </div>
  );
}

// Empty state component
export function EmptyState({
  icon: Icon,
  title,
  description,
  action,
}: {
  icon: React.ElementType;
  title: string;
  description?: string;
  action?: React.ReactNode;
}) {
  return (
    <div className="flex flex-col items-center justify-center py-12 px-4 text-center">
      <div className="w-12 h-12 rounded-full bg-gray-100 dark:bg-gray-800 flex items-center justify-center mb-4">
        <Icon className="h-6 w-6 text-gray-400 dark:text-gray-500" />
      </div>
      <h3 className="text-sm font-medium text-gray-900 dark:text-white mb-1">
        {title}
      </h3>
      {description && (
        <p className="text-sm text-gray-500 dark:text-gray-400 max-w-sm mb-4">
          {description}
        </p>
      )}
      {action}
    </div>
  );
}

// Button variants
export function Button({
  children,
  variant = "primary",
  size = "md",
  icon: Icon,
  onClick,
  disabled,
  className,
  type = "button",
}: {
  children: React.ReactNode;
  variant?: "primary" | "secondary" | "ghost" | "danger";
  size?: "sm" | "md" | "lg";
  icon?: React.ElementType;
  onClick?: () => void;
  disabled?: boolean;
  className?: string;
  type?: "button" | "submit";
}) {
  const variants = {
    primary:
      "bg-primary-600 hover:bg-primary-700 text-white disabled:bg-primary-400",
    secondary:
      "bg-gray-100 dark:bg-gray-800 hover:bg-gray-200 dark:hover:bg-gray-700 text-gray-900 dark:text-white",
    ghost:
      "hover:bg-gray-100 dark:hover:bg-gray-800 text-gray-700 dark:text-gray-300",
    danger:
      "bg-red-600 hover:bg-red-700 text-white disabled:bg-red-400",
  };

  const sizes = {
    sm: "px-3 py-1.5 text-sm",
    md: "px-4 py-2 text-sm",
    lg: "px-5 py-2.5 text-base",
  };

  return (
    <button
      type={type}
      onClick={onClick}
      disabled={disabled}
      className={cn(
        "inline-flex items-center justify-center font-medium rounded-lg transition-colors",
        "disabled:opacity-50 disabled:cursor-not-allowed",
        variants[variant],
        sizes[size],
        className
      )}
    >
      {Icon && <Icon className={cn("h-4 w-4", children ? "mr-2" : "")} />}
      {children}
    </button>
  );
}

// Input component
export function Input({
  label,
  error,
  className,
  ...props
}: React.InputHTMLAttributes<HTMLInputElement> & {
  label?: string;
  error?: string;
}) {
  return (
    <div className={className}>
      {label && (
        <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1.5">
          {label}
        </label>
      )}
      <input
        className={cn(
          "w-full px-3 py-2 bg-white dark:bg-gray-800 border rounded-lg text-gray-900 dark:text-white",
          "placeholder-gray-400 dark:placeholder-gray-500",
          "focus:ring-2 focus:ring-primary-500 focus:border-transparent",
          "disabled:opacity-50 disabled:cursor-not-allowed",
          error
            ? "border-red-500"
            : "border-gray-300 dark:border-gray-700"
        )}
        {...props}
      />
      {error && (
        <p className="mt-1 text-sm text-red-500">{error}</p>
      )}
    </div>
  );
}

// Tabs component
export function Tabs({
  tabs,
  activeTab,
  onChange,
}: {
  tabs: { id: string; label: string; count?: number }[];
  activeTab: string;
  onChange: (id: string) => void;
}) {
  return (
    <div className="flex items-center gap-1 p-1 bg-gray-100 dark:bg-gray-800 rounded-lg">
      {tabs.map((tab) => (
        <button
          key={tab.id}
          onClick={() => onChange(tab.id)}
          className={cn(
            "px-3 py-1.5 text-sm font-medium rounded-md transition-colors",
            activeTab === tab.id
              ? "bg-white dark:bg-gray-700 text-gray-900 dark:text-white shadow-sm"
              : "text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-white"
          )}
        >
          {tab.label}
          {tab.count !== undefined && (
            <span
              className={cn(
                "ml-1.5 px-1.5 py-0.5 text-xs rounded-full",
                activeTab === tab.id
                  ? "bg-primary-100 dark:bg-primary-900/30 text-primary-700 dark:text-primary-400"
                  : "bg-gray-200 dark:bg-gray-700 text-gray-600 dark:text-gray-400"
              )}
            >
              {tab.count}
            </span>
          )}
        </button>
      ))}
    </div>
  );
}
