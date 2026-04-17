"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  FileText,
  Loader2,
  X,
} from "lucide-react";
import { api, DefaultPage, DefaultPageType } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Card } from "@/components/ui/Card";

function DefaultPagesSection() {
  const queryClient = useQueryClient();
  const [selectedPage, setSelectedPage] = useState<DefaultPageType | null>(null);
  const [editingPage, setEditingPage] = useState<DefaultPage | null>(null);
  const [previewHtml, setPreviewHtml] = useState<string | null>(null);

  const { data: pages, isLoading } = useQuery({
    queryKey: ["defaultPages"],
    queryFn: () => api.getDefaultPages(),
  });

  const updateMutation = useMutation({
    mutationFn: ({ pageType, data }: { pageType: string; data: Partial<DefaultPage> }) =>
      api.updateDefaultPage(pageType, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["defaultPages"] });
      setEditingPage(null);
    },
  });

  const pageTypeLabels: Record<DefaultPageType, { label: string; description: string }> = {
    welcome: { label: "Welcome Page", description: "Shown when domain is first configured" },
    "404": { label: "404 Not Found", description: "Page not found error" },
    "500": { label: "500 Server Error", description: "Internal server error" },
    "502": { label: "502 Bad Gateway", description: "Backend server unavailable" },
    "503": { label: "503 Service Unavailable", description: "Service temporarily unavailable" },
    maintenance: { label: "Maintenance", description: "Scheduled maintenance page" },
  };

  const handlePreview = async (pageType: DefaultPageType) => {
    try {
      const result = await api.previewDefaultPage(pageType);
      setPreviewHtml(result.html);
    } catch {
      setPreviewHtml(null);
    }
  };

  const handleEditPage = (page: DefaultPage) => {
    setEditingPage({ ...page });
    setSelectedPage(page.page_type);
  };

  const handleSave = () => {
    if (!editingPage) return;
    updateMutation.mutate({
      pageType: editingPage.page_type,
      data: {
        enabled: editingPage.enabled,
        title: editingPage.title,
        heading: editingPage.heading,
        message: editingPage.message,
        show_logo: editingPage.show_logo,
        custom_css: editingPage.custom_css,
      },
    });
  };

  return (
    <Card>
      <Card.Header>
        <div className="flex items-center gap-3">
          <div className="p-2 bg-purple-500/10 rounded-lg">
            <FileText className="h-5 w-5 text-purple-400" />
          </div>
          <div>
            <h2 className="text-lg font-semibold text-gray-900 dark:text-white">Default Pages</h2>
            <p className="text-sm text-gray-600 dark:text-gray-400">
              Configure welcome and error pages served by InfraPilot
            </p>
          </div>
        </div>
      </Card.Header>
      <Card.Body>
        {isLoading ? (
          <div className="flex items-center justify-center py-8">
            <Loader2 className="h-6 w-6 text-gray-400 animate-spin" />
          </div>
        ) : (
          <div className="space-y-4">
            {pages?.map((page) => (
              <div
                key={page.page_type}
                className={cn(
                  "p-4 rounded-lg border transition-colors",
                  selectedPage === page.page_type
                    ? "border-primary-500 bg-primary-500/5"
                    : "border-gray-200 dark:border-gray-700"
                )}
              >
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-3">
                    <button
                      onClick={() =>
                        updateMutation.mutate({
                          pageType: page.page_type,
                          data: { enabled: !page.enabled },
                        })
                      }
                      className={cn(
                        "relative inline-flex h-5 w-9 items-center rounded-full transition-colors",
                        page.enabled ? "bg-primary-600" : "bg-gray-300 dark:bg-gray-700"
                      )}
                    >
                      <span
                        className={cn(
                          "inline-block h-3 w-3 transform rounded-full bg-white transition-transform",
                          page.enabled ? "translate-x-5" : "translate-x-1"
                        )}
                      />
                    </button>
                    <div>
                      <p className="font-medium text-gray-900 dark:text-white">
                        {pageTypeLabels[page.page_type]?.label || page.page_type}
                      </p>
                      <p className="text-xs text-gray-500">
                        {pageTypeLabels[page.page_type]?.description}
                      </p>
                    </div>
                  </div>
                  <div className="flex items-center gap-2">
                    <button
                      onClick={() => handlePreview(page.page_type)}
                      className="px-3 py-1.5 text-xs text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-white hover:bg-gray-100 dark:hover:bg-gray-800 rounded transition-colors"
                    >
                      Preview
                    </button>
                    <button
                      onClick={() => handleEditPage(page)}
                      className="px-3 py-1.5 text-xs bg-gray-100 dark:bg-gray-800 text-gray-700 dark:text-gray-300 hover:bg-gray-200 dark:hover:bg-gray-700 rounded transition-colors"
                    >
                      Edit
                    </button>
                  </div>
                </div>

                {editingPage && selectedPage === page.page_type && (
                  <div className="mt-4 pt-4 border-t border-gray-200 dark:border-gray-700 space-y-4">
                    <div className="grid sm:grid-cols-2 gap-4">
                      <div>
                        <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                          Page Title
                        </label>
                        <input
                          type="text"
                          value={editingPage.title}
                          onChange={(e) =>
                            setEditingPage({ ...editingPage, title: e.target.value })
                          }
                          className="w-full px-3 py-2 bg-gray-50 dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg text-sm text-gray-900 dark:text-white"
                        />
                      </div>
                      <div>
                        <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                          Heading
                        </label>
                        <input
                          type="text"
                          value={editingPage.heading}
                          onChange={(e) =>
                            setEditingPage({ ...editingPage, heading: e.target.value })
                          }
                          className="w-full px-3 py-2 bg-gray-50 dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg text-sm text-gray-900 dark:text-white"
                        />
                      </div>
                    </div>
                    <div>
                      <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                        Message
                      </label>
                      <textarea
                        value={editingPage.message}
                        onChange={(e) =>
                          setEditingPage({ ...editingPage, message: e.target.value })
                        }
                        rows={2}
                        className="w-full px-3 py-2 bg-gray-50 dark:bg-gray-800 border border-gray-300 dark:border-gray-700 rounded-lg text-sm text-gray-900 dark:text-white"
                      />
                    </div>
                    <div className="flex items-center gap-4">
                      <label className="flex items-center gap-2 cursor-pointer">
                        <input
                          type="checkbox"
                          checked={editingPage.show_logo}
                          onChange={(e) =>
                            setEditingPage({ ...editingPage, show_logo: e.target.checked })
                          }
                          className="w-4 h-4 rounded"
                        />
                        <span className="text-sm text-gray-700 dark:text-gray-300">Show logo</span>
                      </label>
                    </div>
                    <div className="flex justify-end gap-2">
                      <button
                        onClick={() => {
                          setEditingPage(null);
                          setSelectedPage(null);
                        }}
                        className="px-3 py-1.5 text-sm text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-white"
                      >
                        Cancel
                      </button>
                      <button
                        onClick={handleSave}
                        disabled={updateMutation.isPending}
                        className="px-4 py-1.5 text-sm bg-primary-600 hover:bg-primary-700 text-white rounded-lg transition-colors"
                      >
                        {updateMutation.isPending ? "Saving..." : "Save Changes"}
                      </button>
                    </div>
                  </div>
                )}
              </div>
            ))}
          </div>
        )}

        {previewHtml && (
          <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4">
            <div className="bg-white dark:bg-gray-900 rounded-xl shadow-xl max-w-4xl w-full max-h-[90vh] overflow-hidden">
              <div className="p-4 border-b border-gray-200 dark:border-gray-800 flex items-center justify-between">
                <h3 className="font-semibold text-gray-900 dark:text-white">Page Preview</h3>
                <button
                  onClick={() => setPreviewHtml(null)}
                  className="p-2 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-800"
                >
                  <X className="h-5 w-5" />
                </button>
              </div>
              <div className="h-[70vh]">
                <iframe
                  srcDoc={previewHtml}
                  className="w-full h-full border-0"
                  title="Page Preview"
                />
              </div>
            </div>
          </div>
        )}
      </Card.Body>
    </Card>
  );
}

export default function SettingsPagesPage() {
  return <DefaultPagesSection />;
}
