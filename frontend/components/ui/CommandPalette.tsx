"use client";

import React, {
  useState,
  useEffect,
  useRef,
  useCallback,
  useMemo,
  Fragment,
} from "react";
import { useRouter } from "next/navigation";
import {
  Search,
  Command,
  LayoutDashboard,
  Container,
  Globe,
  FileText,
  Bell,
  Settings,
  Users,
  Activity,
  Network,
  HardDrive,
  Image,
  Package,
  Wrench,
  ArrowRight,
  CornerDownLeft,
  LucideIcon,
} from "lucide-react";
import { cn } from "@/lib/utils";

// ============================================================================
// Types
// ============================================================================

export interface CommandItem {
  id: string;
  label: string;
  description?: string;
  icon?: LucideIcon;
  href?: string;
  action?: () => void;
  keywords?: string[];
  section?: string;
  shortcut?: string;
}

export interface CommandSection {
  id: string;
  label: string;
  items: CommandItem[];
}

export interface CommandPaletteProps {
  /** Additional command items to include */
  additionalItems?: CommandItem[];
  /** Custom sections to include */
  customSections?: CommandSection[];
  /** Called when palette opens */
  onOpen?: () => void;
  /** Called when palette closes */
  onClose?: () => void;
  /** Custom placeholder text */
  placeholder?: string;
}

// ============================================================================
// Default Navigation Items
// ============================================================================

const defaultNavigationItems: CommandItem[] = [
  // Overview
  // Overview
  {
    id: "dashboard",
    label: "Dashboard",
    description: "Main overview dashboard",
    icon: LayoutDashboard,
    href: "/",
    section: "Overview",
    keywords: ["home", "main", "overview"],
  },

  // Deploy
  {
    id: "deployments",
    label: "Deployments",
    description: "Deployment history and status",
    icon: Package,
    href: "/docker/deployments",
    section: "Deploy",
    keywords: ["deploy", "deployment", "release"],
  },
  {
    id: "images",
    label: "Images",
    description: "Docker images",
    icon: Image,
    href: "/docker/images",
    section: "Deploy",
    keywords: ["image", "docker", "container"],
  },

  // Run
  {
    id: "containers",
    label: "Containers",
    description: "Running containers",
    icon: Container,
    href: "/docker",
    section: "Run",
    keywords: ["container", "docker", "running"],
  },
  {
    id: "logs",
    label: "Logs",
    description: "Application and system logs",
    icon: FileText,
    href: "/run/logs",
    section: "Run",
    keywords: ["log", "logs", "output"],
  },
  {
    id: "alerts",
    label: "Alerts",
    description: "Active alerts and notifications",
    icon: Bell,
    href: "/alerts",
    section: "Security",
    keywords: ["alert", "notification", "warning"],
  },

  // Webhooks
  {
    id: "webhooks",
    label: "Webhooks",
    description: "Webhook integrations for CD triggers",
    icon: Activity,
    href: "/webhooks",
    section: "Deploy",
    keywords: ["webhook", "integration", "hook", "cd"],
  },

  // Traffic
  {
    id: "proxies",
    label: "Proxies",
    description: "Proxy configuration",
    icon: Globe,
    href: "/traffic/proxies",
    section: "Traffic",
    keywords: ["proxy", "reverse proxy", "nginx", "traffic"],
  },

  // Platform
  {
    id: "networks",
    label: "Networks",
    description: "Docker networks",
    icon: Network,
    href: "/docker/networks",
    section: "Platform",
    keywords: ["network", "docker", "bridge"],
  },
  {
    id: "volumes",
    label: "Volumes",
    description: "Docker volumes",
    icon: HardDrive,
    href: "/docker/volumes",
    section: "Platform",
    keywords: ["volume", "storage", "docker"],
  },
  {
    id: "health",
    label: "System Health",
    description: "System health monitoring",
    icon: Activity,
    href: "/system-health",
    section: "Platform",
    keywords: ["health", "status", "monitoring"],
  },
  {
    id: "users",
    label: "Users",
    description: "User management",
    icon: Users,
    href: "/settings/users",
    section: "Platform",
    keywords: ["user", "account", "admin"],
  },
  {
    id: "settings",
    label: "Settings",
    description: "Application settings",
    icon: Settings,
    href: "/settings",
    section: "Platform",
    keywords: ["settings", "config", "preferences"],
  },
];

// ============================================================================
// Command Palette Component
// ============================================================================

export function CommandPalette({
  additionalItems = [],
  customSections = [],
  onOpen,
  onClose,
  placeholder = "Type a command or search...",
}: CommandPaletteProps) {
  const [isOpen, setIsOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [selectedIndex, setSelectedIndex] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);
  const router = useRouter();

  // Combine all items
  const allItems = useMemo(
    () => [...defaultNavigationItems, ...additionalItems],
    [additionalItems]
  );

  // Filter items based on query
  const filteredItems = useMemo(() => {
    if (!query.trim()) return allItems;

    const lowerQuery = query.toLowerCase();
    return allItems.filter((item) => {
      const matchesLabel = item.label.toLowerCase().includes(lowerQuery);
      const matchesDescription = item.description
        ?.toLowerCase()
        .includes(lowerQuery);
      const matchesKeywords = item.keywords?.some((k) =>
        k.toLowerCase().includes(lowerQuery)
      );
      return matchesLabel || matchesDescription || matchesKeywords;
    });
  }, [allItems, query]);

  // Group filtered items by section
  const groupedItems = useMemo(() => {
    const groups: Record<string, CommandItem[]> = {};
    filteredItems.forEach((item) => {
      const section = item.section || "Other";
      if (!groups[section]) {
        groups[section] = [];
      }
      groups[section].push(item);
    });
    return groups;
  }, [filteredItems]);

  // Flatten for keyboard navigation
  const flatItems = useMemo(() => {
    return Object.values(groupedItems).flat();
  }, [groupedItems]);

  // Open/close handlers
  const open = useCallback(() => {
    setIsOpen(true);
    setQuery("");
    setSelectedIndex(0);
    onOpen?.();
  }, [onOpen]);

  const close = useCallback(() => {
    setIsOpen(false);
    setQuery("");
    setSelectedIndex(0);
    onClose?.();
  }, [onClose]);

  // Execute selected item
  const executeItem = useCallback(
    (item: CommandItem) => {
      if (item.href) {
        router.push(item.href);
      } else if (item.action) {
        item.action();
      }
      close();
    },
    [router, close]
  );

  // Keyboard shortcuts
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      // Open with Cmd+K or Ctrl+K
      if ((e.metaKey || e.ctrlKey) && e.key === "k") {
        e.preventDefault();
        if (isOpen) {
          close();
        } else {
          open();
        }
        return;
      }

      if (!isOpen) return;

      // Close with Escape
      if (e.key === "Escape") {
        e.preventDefault();
        close();
        return;
      }

      // Navigate with arrow keys
      if (e.key === "ArrowDown") {
        e.preventDefault();
        setSelectedIndex((prev) =>
          prev < flatItems.length - 1 ? prev + 1 : 0
        );
        return;
      }

      if (e.key === "ArrowUp") {
        e.preventDefault();
        setSelectedIndex((prev) =>
          prev > 0 ? prev - 1 : flatItems.length - 1
        );
        return;
      }

      // Execute with Enter
      if (e.key === "Enter") {
        e.preventDefault();
        const selectedItem = flatItems[selectedIndex];
        if (selectedItem) {
          executeItem(selectedItem);
        }
        return;
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [isOpen, open, close, flatItems, selectedIndex, executeItem]);

  // Focus input when opened
  useEffect(() => {
    if (isOpen && inputRef.current) {
      inputRef.current.focus();
    }
  }, [isOpen]);

  // Scroll selected item into view
  useEffect(() => {
    if (listRef.current && flatItems.length > 0) {
      const selectedElement = listRef.current.querySelector(
        `[data-index="${selectedIndex}"]`
      );
      if (selectedElement) {
        selectedElement.scrollIntoView({ block: "nearest" });
      }
    }
  }, [selectedIndex, flatItems.length]);

  // Reset selected index when query changes
  useEffect(() => {
    setSelectedIndex(0);
  }, [query]);

  if (!isOpen) {
    return null;
  }

  return (
    <div className="fixed inset-0 z-50">
      {/* Backdrop */}
      <div
        className="absolute inset-0 bg-black/50 backdrop-blur-sm"
        onClick={close}
      />

      {/* Dialog */}
      <div className="absolute left-1/2 top-[20%] -translate-x-1/2 w-full max-w-xl">
        <div className="bg-white dark:bg-gray-900 rounded-xl shadow-2xl border border-gray-200 dark:border-gray-700 overflow-hidden">
          {/* Search Input */}
          <div className="flex items-center gap-3 px-4 py-3 border-b border-gray-200 dark:border-gray-700">
            <Search className="h-5 w-5 text-gray-400" />
            <input
              ref={inputRef}
              type="text"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder={placeholder}
              className="flex-1 bg-transparent border-none outline-none text-gray-900 dark:text-white placeholder-gray-400"
            />
            <kbd className="hidden sm:inline-flex items-center gap-1 px-2 py-1 text-xs font-medium text-gray-400 bg-gray-100 dark:bg-gray-800 rounded">
              ESC
            </kbd>
          </div>

          {/* Results */}
          <div
            ref={listRef}
            className="max-h-80 overflow-y-auto py-2"
          >
            {flatItems.length === 0 ? (
              <div className="px-4 py-8 text-center text-gray-500 dark:text-gray-400">
                No results found for "{query}"
              </div>
            ) : (
              Object.entries(groupedItems).map(([section, items]) => (
                <Fragment key={section}>
                  <div className="px-4 py-2">
                    <span className="text-xs font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                      {section}
                    </span>
                  </div>
                  {items.map((item) => {
                    const itemIndex = flatItems.indexOf(item);
                    const isSelected = itemIndex === selectedIndex;
                    const Icon = item.icon;

                    return (
                      <button
                        key={item.id}
                        data-index={itemIndex}
                        onClick={() => executeItem(item)}
                        onMouseEnter={() => setSelectedIndex(itemIndex)}
                        className={cn(
                          "w-full flex items-center gap-3 px-4 py-2.5 text-left transition-colors",
                          isSelected
                            ? "bg-primary-50 dark:bg-primary-900/20"
                            : "hover:bg-gray-50 dark:hover:bg-gray-800"
                        )}
                      >
                        {Icon && (
                          <div
                            className={cn(
                              "p-1.5 rounded-lg",
                              isSelected
                                ? "bg-primary-100 dark:bg-primary-900/40 text-primary-600 dark:text-primary-400"
                                : "bg-gray-100 dark:bg-gray-800 text-gray-500 dark:text-gray-400"
                            )}
                          >
                            <Icon className="h-4 w-4" />
                          </div>
                        )}
                        <div className="flex-1 min-w-0">
                          <div
                            className={cn(
                              "text-sm font-medium truncate",
                              isSelected
                                ? "text-primary-900 dark:text-primary-100"
                                : "text-gray-900 dark:text-white"
                            )}
                          >
                            {item.label}
                          </div>
                          {item.description && (
                            <div className="text-xs text-gray-500 dark:text-gray-400 truncate">
                              {item.description}
                            </div>
                          )}
                        </div>
                        {item.shortcut && (
                          <kbd className="hidden sm:inline-flex items-center px-2 py-1 text-xs font-medium text-gray-400 bg-gray-100 dark:bg-gray-800 rounded">
                            {item.shortcut}
                          </kbd>
                        )}
                        {isSelected && (
                          <CornerDownLeft className="h-4 w-4 text-gray-400" />
                        )}
                      </button>
                    );
                  })}
                </Fragment>
              ))
            )}
          </div>

          {/* Footer */}
          <div className="px-4 py-2 border-t border-gray-200 dark:border-gray-700 flex items-center justify-between text-xs text-gray-400">
            <div className="flex items-center gap-4">
              <span className="flex items-center gap-1">
                <kbd className="px-1.5 py-0.5 bg-gray-100 dark:bg-gray-800 rounded">
                  ↑
                </kbd>
                <kbd className="px-1.5 py-0.5 bg-gray-100 dark:bg-gray-800 rounded">
                  ↓
                </kbd>
                <span className="ml-1">Navigate</span>
              </span>
              <span className="flex items-center gap-1">
                <kbd className="px-1.5 py-0.5 bg-gray-100 dark:bg-gray-800 rounded">
                  ↵
                </kbd>
                <span className="ml-1">Select</span>
              </span>
            </div>
            <span className="flex items-center gap-1">
              <Command className="h-3 w-3" />
              <span>K to toggle</span>
            </span>
          </div>
        </div>
      </div>
    </div>
  );
}

// ============================================================================
// Hook for programmatic control
// ============================================================================

export function useCommandPalette() {
  const [isOpen, setIsOpen] = useState(false);

  const open = useCallback(() => setIsOpen(true), []);
  const close = useCallback(() => setIsOpen(false), []);
  const toggle = useCallback(() => setIsOpen((prev) => !prev), []);

  return { isOpen, open, close, toggle };
}
