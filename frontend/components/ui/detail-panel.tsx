"use client";

import { useState, useRef, useEffect, useCallback } from "react";
import { X, GripVertical } from "lucide-react";
import { cn } from "@/lib/utils";

interface DetailPanelProps {
  open: boolean;
  onClose: () => void;
  title?: string;
  subtitle?: string;
  children: React.ReactNode;
  defaultWidth?: number;
  minWidth?: number;
  maxWidth?: number;
  footer?: React.ReactNode;
}

export function DetailPanel({
  open,
  onClose,
  title,
  subtitle,
  children,
  defaultWidth = 480,
  minWidth = 320,
  maxWidth = 800,
  footer,
}: DetailPanelProps) {
  const [width, setWidth] = useState(defaultWidth);
  const [isResizing, setIsResizing] = useState(false);
  const panelRef = useRef<HTMLElement>(null);

  const handleMouseDown = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
    setIsResizing(true);
  }, []);

  const handleMouseMove = useCallback(
    (e: MouseEvent) => {
      if (!isResizing) return;

      const newWidth = window.innerWidth - e.clientX;
      if (newWidth >= minWidth && newWidth <= maxWidth) {
        setWidth(newWidth);
      }
    },
    [isResizing, minWidth, maxWidth]
  );

  const handleMouseUp = useCallback(() => {
    setIsResizing(false);
  }, []);

  useEffect(() => {
    if (isResizing) {
      document.addEventListener("mousemove", handleMouseMove);
      document.addEventListener("mouseup", handleMouseUp);
      document.body.style.cursor = "col-resize";
      document.body.style.userSelect = "none";
    }

    return () => {
      document.removeEventListener("mousemove", handleMouseMove);
      document.removeEventListener("mouseup", handleMouseUp);
      document.body.style.cursor = "";
      document.body.style.userSelect = "";
    };
  }, [isResizing, handleMouseMove, handleMouseUp]);

  // Reset width when panel closes
  useEffect(() => {
    if (!open) {
      setWidth(defaultWidth);
    }
  }, [open, defaultWidth]);

  return (
    <>
      {/* Backdrop for mobile */}
      {open && (
        <div
          className="fixed inset-0 bg-black/20 dark:bg-black/40 z-40 lg:hidden"
          onClick={onClose}
        />
      )}

      {/* Panel */}
      <aside
        ref={panelRef}
        style={{ width: open ? width : 0 }}
        className={cn(
          "fixed lg:relative right-0 top-0 lg:top-auto h-full max-h-full bg-white dark:bg-gray-900 border-l border-gray-200 dark:border-gray-800 z-50 lg:z-auto",
          "flex flex-col min-h-0 overflow-hidden",
          "transition-[transform,opacity] duration-300 ease-in-out",
          open ? "translate-x-0 opacity-100" : "translate-x-full lg:translate-x-0 opacity-0 lg:w-0 lg:overflow-hidden"
        )}
      >
        {/* Resize Handle */}
        <div
          onMouseDown={handleMouseDown}
          className={cn(
            "absolute left-0 top-0 bottom-0 w-1 cursor-col-resize group hidden lg:flex items-center justify-center",
            "hover:bg-primary-500/20 transition-colors",
            isResizing && "bg-primary-500/30"
          )}
        >
          <div
            className={cn(
              "absolute left-0 w-4 h-12 -ml-1.5 flex items-center justify-center",
              "opacity-0 group-hover:opacity-100 transition-opacity",
              isResizing && "opacity-100"
            )}
          >
            <GripVertical className="h-4 w-4 text-gray-400" />
          </div>
        </div>

        {/* Header */}
        {(title || subtitle) && (
          <div className="flex items-start justify-between p-4 border-b border-gray-200 dark:border-gray-800 flex-shrink-0">
            <div className="flex-1 min-w-0 pr-4">
              {title && (
                <h2 className="text-lg font-semibold text-gray-900 dark:text-white truncate">
                  {title}
                </h2>
              )}
              {subtitle && (
                <p className="text-sm text-gray-500 dark:text-gray-400 truncate mt-0.5">
                  {subtitle}
                </p>
              )}
            </div>
            <button
              onClick={onClose}
              className="p-1.5 rounded-lg text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-800 transition-colors flex-shrink-0"
            >
              <X className="h-5 w-5" />
            </button>
          </div>
        )}

        {/* Content */}
        <div className="flex-1 overflow-y-auto p-4 animate-fade-in min-h-0">
          {children}
        </div>

        {/* Footer */}
        {footer && (
          <div className="p-4 border-t border-gray-200 dark:border-gray-800 bg-gray-50 dark:bg-gray-800/50 flex-shrink-0">
            {footer}
          </div>
        )}
      </aside>
    </>
  );
}

// Section component for organizing detail panel content
export function DetailSection({
  title,
  children,
  className,
}: {
  title?: string;
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <div className={cn("mb-6", className)}>
      {title && (
        <h3 className="text-xs font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wider mb-3">
          {title}
        </h3>
      )}
      {children}
    </div>
  );
}

// Key-value row component
export function DetailRow({
  label,
  value,
  mono,
}: {
  label: string;
  value: React.ReactNode;
  mono?: boolean;
}) {
  return (
    <div className="flex items-start justify-between py-2 border-b border-gray-100 dark:border-gray-800 last:border-0">
      <span className="text-sm text-gray-500 dark:text-gray-400">{label}</span>
      <span
        className={cn(
          "text-sm text-gray-900 dark:text-white text-right max-w-[60%] truncate",
          mono && "font-mono text-xs"
        )}
      >
        {value}
      </span>
    </div>
  );
}

// Status badge component
export function StatusBadge({
  status,
  size = "sm",
}: {
  status: "running" | "stopped" | "error" | "warning" | "success" | "pending";
  size?: "sm" | "md";
}) {
  const colors = {
    running: "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400",
    success: "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400",
    stopped: "bg-gray-100 text-gray-700 dark:bg-gray-800 dark:text-gray-400",
    error: "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400",
    warning: "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400",
    pending: "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400",
  };

  const sizes = {
    sm: "px-2 py-0.5 text-xs",
    md: "px-2.5 py-1 text-sm",
  };

  return (
    <span
      className={cn(
        "inline-flex items-center font-medium rounded-full capitalize",
        colors[status],
        sizes[size]
      )}
    >
      <span
        className={cn(
          "w-1.5 h-1.5 rounded-full mr-1.5",
          status === "running" || status === "success"
            ? "bg-green-500"
            : status === "error"
            ? "bg-red-500"
            : status === "warning"
            ? "bg-yellow-500"
            : status === "pending"
            ? "bg-blue-500"
            : "bg-gray-400"
        )}
      />
      {status}
    </span>
  );
}
