"use client";

import { useState, useMemo, useEffect } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Activity,
  AlertTriangle,
  Clock,
  Globe,
  Users,
  Zap,
  Lock,
} from "lucide-react";
import { api } from "@/lib/api";
import { PageHeader } from "@/components/ui/PageHeader";
import { Breadcrumb } from "@/components/ui/Breadcrumb";
import { StatCard, MetricsGrid } from "@/components/ui/StatCard";
import { Card } from "@/components/ui/Card";
import { Table } from "@/components/ui/Table";
import { Badge } from "@/components/ui/Badge";
import { EmptyState } from "@/components/ui/EmptyState";
import { Spinner } from "@/components/ui/Spinner";
import { FilterToolbar, DEFAULT_TIME_RANGES, SelectOption, TimeRangeOption, ToggleOption } from "@/components/ui/FilterToolbar";
import { cn } from "@/lib/utils";
import {
  AreaChart,
  Area,
  LineChart,
  Line,
  BarChart,
  Bar,
  PieChart,
  Pie,
  Cell,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  Legend,
} from "recharts";

// =============================================================================
// CE Time Ranges — 7d and 30d are shown but gated (backend caps at 24h anyway)
// =============================================================================

const CE_MAX_HOURS = 24;

const CE_TIME_RANGES = DEFAULT_TIME_RANGES.map((r) => ({
  ...r,
  // Mark 7d (168h) and 30d (720h) as enterprise-only
  enterpriseOnly: r.unit === "hours" && r.value > CE_MAX_HOURS,
}));

// =============================================================================
// Filter Options
// =============================================================================

const ANALYTICS_STATUS_OPTIONS: ToggleOption[] = [
  { value: "all", label: "All" },
  { value: "success", label: "2xx/3xx", color: "green" },
  { value: "errors", label: "4xx/5xx", color: "red" },
];

// =============================================================================
// Types
// =============================================================================

interface NginxAnalyticsDataPoint {
  bucket: string;
  total_requests: number;
  count_2xx: number;
  count_3xx: number;
  count_4xx: number;
  count_5xx: number;
  total_bytes: number;
  avg_response_time: number;
  p95_response_time: number;
  p99_response_time: number;
  max_response_time: number;
  min_response_time: number;
  unique_visitors: number;
}

interface NginxAnalyticsResponse {
  interval: string;
  start_time: string;
  end_time: string;
  data_points: NginxAnalyticsDataPoint[];
}

interface NginxAnalyticsSummary {
  total_requests: number;
  total_bytes: number;
  unique_visitors: number;
  avg_response_time: number;
  p95_response_time: number;
  p99_response_time: number;
  error_rate: number;
  count_2xx: number;
  count_3xx: number;
  count_4xx: number;
  count_5xx: number;
  requests_per_second: number;
  bytes_per_second: number;
}

interface NginxTopPath {
  path: string;
  request_count: number;
  total_bytes: number;
  avg_response_time: number;
  p95_response_time: number;
  count_4xx: number;
  count_5xx: number;
  error_rate: number;
  unique_visitors: number;
}

interface StatusCodeDistribution {
  status_codes: Array<{ status_code: number; count: number; percentage: number }>;
  categories: { "2xx": number; "3xx": number; "4xx": number; "5xx": number };
  total: number;
}

interface MethodStats {
  method: string;
  count: number;
  total_bytes: number;
  avg_response_time: number;
  percentage: number;
}

interface ClientStats {
  client_ip: string;
  request_count: number;
  total_bytes: number;
  avg_response_time: number;
  error_count: number;
  error_rate: number;
  last_seen: string;
}

interface UserAgentStats {
  browsers: Array<{ name: string; count: number; percentage: number }>;
  devices: Array<{ name: string; count: number; percentage: number }>;
  top_agents: Array<{ user_agent: string; count: number; type: string }>;
  total: number;
}

// =============================================================================
// Constants
// =============================================================================

const STATUS_COLORS = {
  "2xx": "#22c55e",
  "3xx": "#3b82f6",
  "4xx": "#f59e0b",
  "5xx": "#ef4444",
};

const METHOD_COLORS: Record<string, string> = {
  GET: "#22c55e",
  POST: "#3b82f6",
  PUT: "#f59e0b",
  DELETE: "#ef4444",
  PATCH: "#8b5cf6",
  HEAD: "#6b7280",
  OPTIONS: "#06b6d4",
};

const DEVICE_COLORS: Record<string, string> = {
  Desktop: "#3b82f6",
  Mobile: "#22c55e",
  Tablet: "#f59e0b",
  Bot: "#ef4444",
  Other: "#6b7280",
};

const BROWSER_COLORS: Record<string, string> = {
  Chrome: "#4285F4",
  Firefox: "#FF7139",
  Safari: "#000000",
  Edge: "#0078D7",
  Opera: "#FF1B2D",
  IE: "#0076D6",
  Other: "#6b7280",
};

// =============================================================================
// Helper Functions
// =============================================================================

function formatBytes(bytes: number): string {
  if (bytes === 0) return "0 B";
  const k = 1024;
  const sizes = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + " " + sizes[i];
}

function formatResponseTime(ms: number): string {
  if (ms < 0.001) return "< 1ms";
  if (ms < 1) return `${(ms * 1000).toFixed(0)}ms`;
  return `${ms.toFixed(2)}s`;
}

function formatNumber(num: number): string {
  if (num >= 1000000) return `${(num / 1000000).toFixed(1)}M`;
  if (num >= 1000) return `${(num / 1000).toFixed(1)}K`;
  return num.toString();
}

function formatTime(timestamp: string): string {
  return new Date(timestamp).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}

// =============================================================================
// CE Time Range Selector (with Enterprise badge on 7d/30d)
// =============================================================================

function CETimeRangeSelector({
  value,
  onChange,
}: {
  value: TimeRangeOption;
  onChange: (v: TimeRangeOption) => void;
}) {
  return (
    <div className="flex items-center gap-1 bg-gray-100 dark:bg-gray-800 rounded-lg p-1">
      {CE_TIME_RANGES.map((option) => {
        const isActive = value.label === option.label;
        const isGated = option.enterpriseOnly;
        return (
          <div key={option.label} className="relative group">
            <button
              onClick={() => !isGated && onChange(option)}
              disabled={isGated}
              className={cn(
                "px-2.5 py-1 rounded text-xs font-medium transition-colors",
                isGated
                  ? "text-gray-400 dark:text-gray-600 cursor-not-allowed"
                  : isActive
                  ? "bg-white dark:bg-gray-700 text-gray-900 dark:text-white shadow"
                  : "text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-white"
              )}
            >
              {option.label}
              {isGated && (
                <span className="ml-1 text-[9px] font-semibold text-purple-500 dark:text-purple-400 uppercase">Pro</span>
              )}
            </button>
            {isGated && (
              <div className="absolute bottom-full left-1/2 -translate-x-1/2 mb-2 px-2 py-1 bg-gray-900 text-white text-xs rounded whitespace-nowrap opacity-0 group-hover:opacity-100 pointer-events-none transition-opacity z-10">
                Enterprise feature
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}

// =============================================================================
// Main Component
// =============================================================================

export default function TrafficAnalyticsPage() {
  const queryClient = useQueryClient();

  const [selectedRange, setSelectedRange] = useState<TimeRangeOption>(DEFAULT_TIME_RANGES[2]); // Default 30m
  const [autoRefresh, setAutoRefresh] = useState(false);
  const [selectedAgent, setSelectedAgent] = useState<string | null>(null);
  const [selectedDomain, setSelectedDomain] = useState<string>("");
  const [statusFilter, setStatusFilter] = useState<string>("all");

  const { data: agents } = useQuery({
    queryKey: ["agents"],
    queryFn: () => api.getAgents(),
  });

  const activeAgents = agents?.filter((a) => a.status === "active") || [];

  useEffect(() => {
    if (!selectedAgent && activeAgents.length > 0) {
      setSelectedAgent(activeAgents[0].id);
    }
  }, [activeAgents, selectedAgent]);

  const { data: proxies } = useQuery({
    queryKey: ["proxies", selectedAgent],
    queryFn: () => (selectedAgent ? api.getProxyHosts(selectedAgent) : Promise.resolve([])),
    enabled: !!selectedAgent,
  });

  const domains = useMemo(() => {
    if (!proxies) return [];
    return [...new Set(proxies.map((p) => p.domain))].sort();
  }, [proxies]);

  const getTimeParams = (): Record<string, string> => {
    if (selectedRange.unit === "minutes") {
      return { minutes: selectedRange.value.toString() };
    }
    return { hours: selectedRange.value.toString() };
  };

  const { data: analytics, isLoading: isLoadingAnalytics, isFetching: isFetchingAnalytics } = useQuery({
    queryKey: ["traffic-analytics", selectedRange.value, selectedRange.unit, selectedRange.interval, selectedAgent, selectedDomain],
    queryFn: async () => {
      const params = new URLSearchParams({
        ...getTimeParams(),
        interval: selectedRange.interval || "1m",
      });
      if (selectedAgent) params.append("agent_id", selectedAgent);
      if (selectedDomain) params.append("domain", selectedDomain);
      return api.fetchAPI<NginxAnalyticsResponse>(`/traffic/analytics?${params}`);
    },
    refetchInterval: autoRefresh ? 30000 : false,
  });

  const { data: summary, isLoading: isLoadingSummary } = useQuery({
    queryKey: ["traffic-analytics-summary", selectedRange.value, selectedRange.unit, selectedAgent, selectedDomain],
    queryFn: async () => {
      const params = new URLSearchParams(getTimeParams());
      if (selectedAgent) params.append("agent_id", selectedAgent);
      if (selectedDomain) params.append("domain", selectedDomain);
      return api.fetchAPI<NginxAnalyticsSummary>(`/traffic/analytics/summary?${params}`);
    },
    refetchInterval: autoRefresh ? 30000 : false,
  });

  const { data: topPathsResponse } = useQuery({
    queryKey: ["traffic-top-paths", selectedRange.value, selectedRange.unit, selectedAgent, selectedDomain],
    queryFn: async () => {
      const params = new URLSearchParams({ ...getTimeParams(), limit: "15" });
      if (selectedAgent) params.append("agent_id", selectedAgent);
      if (selectedDomain) params.append("domain", selectedDomain);
      return api.fetchAPI<{ paths: NginxTopPath[] }>(`/traffic/analytics/top-paths?${params}`);
    },
    refetchInterval: autoRefresh ? 30000 : false,
  });

  const { data: statusCodes } = useQuery({
    queryKey: ["traffic-status-codes", selectedRange.value, selectedRange.unit, selectedAgent, selectedDomain],
    queryFn: async () => {
      const params = new URLSearchParams(getTimeParams());
      if (selectedAgent) params.append("agent_id", selectedAgent);
      if (selectedDomain) params.append("domain", selectedDomain);
      return api.fetchAPI<StatusCodeDistribution>(`/traffic/analytics/status-codes?${params}`);
    },
    refetchInterval: autoRefresh ? 30000 : false,
  });

  const { data: methodsData } = useQuery({
    queryKey: ["traffic-methods", selectedRange.value, selectedRange.unit, selectedAgent, selectedDomain],
    queryFn: async () => {
      const params = new URLSearchParams(getTimeParams());
      if (selectedAgent) params.append("agent_id", selectedAgent);
      if (selectedDomain) params.append("domain", selectedDomain);
      return api.fetchAPI<{ methods: MethodStats[]; total: number }>(`/traffic/analytics/methods?${params}`);
    },
    refetchInterval: autoRefresh ? 30000 : false,
  });

  const { data: clientsData } = useQuery({
    queryKey: ["traffic-clients", selectedRange.value, selectedRange.unit, selectedAgent, selectedDomain],
    queryFn: async () => {
      const params = new URLSearchParams({ ...getTimeParams(), limit: "10" });
      if (selectedAgent) params.append("agent_id", selectedAgent);
      if (selectedDomain) params.append("domain", selectedDomain);
      return api.fetchAPI<{ clients: ClientStats[] }>(`/traffic/analytics/clients?${params}`);
    },
    refetchInterval: autoRefresh ? 30000 : false,
  });

  const { data: userAgentData } = useQuery({
    queryKey: ["traffic-user-agents", selectedRange.value, selectedRange.unit, selectedAgent, selectedDomain],
    queryFn: async () => {
      const params = new URLSearchParams(getTimeParams());
      if (selectedAgent) params.append("agent_id", selectedAgent);
      if (selectedDomain) params.append("domain", selectedDomain);
      return api.fetchAPI<UserAgentStats>(`/traffic/analytics/user-agents?${params}`);
    },
    refetchInterval: autoRefresh ? 30000 : false,
  });

  const chartData = useMemo(() => {
    if (!analytics?.data_points) return [];
    return analytics.data_points.map((dp) => ({
      time: formatTime(dp.bucket),
      requests: dp.total_requests,
      "2xx": dp.count_2xx,
      "3xx": dp.count_3xx,
      "4xx": dp.count_4xx,
      "5xx": dp.count_5xx,
      avgLatency: dp.avg_response_time * 1000,
      p95Latency: dp.p95_response_time * 1000,
      visitors: dp.unique_visitors,
      bytes: dp.total_bytes,
    }));
  }, [analytics]);

  const pieData = useMemo(() => {
    if (!statusCodes?.categories) return [];
    return [
      { name: "2xx Success", value: statusCodes.categories["2xx"], color: STATUS_COLORS["2xx"] },
      { name: "3xx Redirect", value: statusCodes.categories["3xx"], color: STATUS_COLORS["3xx"] },
      { name: "4xx Client Error", value: statusCodes.categories["4xx"], color: STATUS_COLORS["4xx"] },
      { name: "5xx Server Error", value: statusCodes.categories["5xx"], color: STATUS_COLORS["5xx"] },
    ].filter((d) => d.value > 0);
  }, [statusCodes]);

  const topPathsColumns = [
    {
      key: "path",
      header: "Path",
      render: (value: string) => (
        <span className="font-mono text-sm text-gray-900 dark:text-white truncate max-w-xs block">{value}</span>
      ),
    },
    {
      key: "request_count",
      header: "Requests",
      width: "100px",
      align: "right" as const,
      render: (value: number) => <span className="font-medium text-gray-900 dark:text-white">{formatNumber(value)}</span>,
    },
    {
      key: "avg_response_time",
      header: "Avg Latency",
      width: "100px",
      align: "right" as const,
      render: (value: number) => <span className="text-gray-600 dark:text-gray-400">{formatResponseTime(value)}</span>,
    },
    {
      key: "p95_response_time",
      header: "P95",
      width: "100px",
      align: "right" as const,
      render: (value: number) => <span className="text-gray-600 dark:text-gray-400">{formatResponseTime(value)}</span>,
    },
    {
      key: "total_bytes",
      header: "Bandwidth",
      width: "100px",
      align: "right" as const,
      render: (value: number) => <span className="text-gray-600 dark:text-gray-400">{formatBytes(value)}</span>,
    },
    {
      key: "error_rate",
      header: "Errors",
      width: "100px",
      align: "right" as const,
      render: (value: number, row: NginxTopPath) => {
        const hasErrors = row.count_4xx + row.count_5xx > 0;
        return (
          <span className={hasErrors ? "text-red-600 dark:text-red-400" : "text-green-600 dark:text-green-400"}>
            {value.toFixed(1)}%
          </span>
        );
      },
    },
  ];

  const agentOptions: SelectOption[] = agents
    ? agents.filter((a) => a.status === "active").map((a) => ({ value: a.id, label: a.name }))
    : [];

  const handleRefresh = () => {
    queryClient.invalidateQueries({ queryKey: ["traffic-analytics"] });
    queryClient.invalidateQueries({ queryKey: ["traffic-analytics-summary"] });
    queryClient.invalidateQueries({ queryKey: ["traffic-top-paths"] });
    queryClient.invalidateQueries({ queryKey: ["traffic-status-codes"] });
  };

  const isLoading = isLoadingAnalytics || isLoadingSummary;

  const pageHeader = (
    <PageHeader
      title="Traffic Analytics"
      description="Real-time nginx access log analytics and insights"
      breadcrumbs={
        <Breadcrumb
          items={[
            { label: "Overview", href: "/" },
            { label: "Traffic", href: "/traffic" },
            { label: "Analytics", current: true },
          ]}
        />
      }
    />
  );

  if (isLoading && !summary) {
    return (
      <div className="space-y-6">
        {pageHeader}
        <Spinner.Page label="Loading analytics..." />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {pageHeader}

      {/* Filter Toolbar — agent/domain/status/refresh; time range is inline below */}
      <FilterToolbar
        agents={agentOptions}
        selectedAgent={selectedAgent}
        onAgentChange={setSelectedAgent}
        showAgentFilter={agentOptions.length > 1}
        domains={domains}
        selectedDomain={selectedDomain}
        onDomainChange={setSelectedDomain}
        showDomainFilter={true}
        statusFilter={{
          options: ANALYTICS_STATUS_OPTIONS,
          value: statusFilter,
          onChange: setStatusFilter,
        }}
        showSearch={false}
        showAutoRefresh={true}
        autoRefresh={autoRefresh}
        onAutoRefreshChange={setAutoRefresh}
        autoRefreshLabel={{ active: "Live", inactive: "Auto" }}
        showRefresh={true}
        onRefresh={handleRefresh}
        isRefreshing={isFetchingAnalytics}
        customActions={<CETimeRangeSelector value={selectedRange} onChange={setSelectedRange} />}
      />

      {/* CE retention notice */}
      <div className="flex items-center gap-2 px-3 py-2 bg-amber-50 dark:bg-amber-900/10 border border-amber-200 dark:border-amber-800 rounded-lg">
        <Lock className="h-3.5 w-3.5 text-amber-600 dark:text-amber-400 flex-shrink-0" />
        <p className="text-xs text-amber-700 dark:text-amber-400">
          7-day+ data retention available in{" "}
          <a href="https://infrapilot.org/enterprise" target="_blank" rel="noopener noreferrer" className="font-medium underline hover:no-underline">
            Enterprise
          </a>.
        </p>
      </div>

      {/* Summary Stats */}
      {!isLoading && (!summary || summary.total_requests === 0) ? (
        <EmptyState
          icon={Activity}
          title="No traffic data available"
          description="Traffic analytics will appear here once nginx access logs start flowing from your agents."
        />
      ) : (
        <>
          {summary && (
            <MetricsGrid columns={5}>
              <StatCard
                label="Total Requests"
                value={formatNumber(summary.total_requests)}
                description={`${summary.requests_per_second.toFixed(1)} req/s`}
                icon={Activity}
                iconColor="text-blue-600 dark:text-blue-400"
              />
              <StatCard
                label="Error Rate"
                value={`${summary.error_rate.toFixed(2)}%`}
                description={`${formatNumber(summary.count_4xx + summary.count_5xx)} errors`}
                icon={AlertTriangle}
                iconColor={
                  summary.error_rate > 5
                    ? "text-red-600 dark:text-red-400"
                    : summary.error_rate > 1
                    ? "text-yellow-600 dark:text-yellow-400"
                    : "text-green-600 dark:text-green-400"
                }
                valueColor={
                  summary.error_rate > 5
                    ? "text-red-600 dark:text-red-400"
                    : summary.error_rate > 1
                    ? "text-yellow-600 dark:text-yellow-400"
                    : "text-green-600 dark:text-green-400"
                }
              />
              <StatCard
                label="Avg Latency"
                value={formatResponseTime(summary.avg_response_time)}
                description={`P95: ${formatResponseTime(summary.p95_response_time)}`}
                icon={Clock}
                iconColor="text-purple-600 dark:text-purple-400"
              />
              <StatCard
                label="P99 Latency"
                value={formatResponseTime(summary.p99_response_time)}
                description="99th percentile"
                icon={Zap}
                iconColor="text-orange-600 dark:text-orange-400"
              />
              <StatCard
                label="Unique Visitors"
                value={formatNumber(summary.unique_visitors)}
                description={formatBytes(summary.total_bytes) + " transferred"}
                icon={Users}
                iconColor="text-indigo-600 dark:text-indigo-400"
              />
            </MetricsGrid>
          )}

          {/* Charts Row 1 */}
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
            <Card>
              <Card.Header>
                <h3 className="text-lg font-semibold text-gray-900 dark:text-white">Request Rate</h3>
              </Card.Header>
              <Card.Body>
                <ResponsiveContainer width="100%" height={280}>
                  <AreaChart data={chartData}>
                    <CartesianGrid strokeDasharray="3 3" stroke="#374151" />
                    <XAxis dataKey="time" stroke="#9CA3AF" fontSize={12} />
                    <YAxis stroke="#9CA3AF" fontSize={12} />
                    <Tooltip contentStyle={{ backgroundColor: "#1F2937", border: "1px solid #374151", borderRadius: "8px" }} />
                    <Legend />
                    <Area type="monotone" dataKey="2xx" stackId="1" stroke={STATUS_COLORS["2xx"]} fill={STATUS_COLORS["2xx"]} fillOpacity={0.6} name="2xx" />
                    <Area type="monotone" dataKey="3xx" stackId="1" stroke={STATUS_COLORS["3xx"]} fill={STATUS_COLORS["3xx"]} fillOpacity={0.6} name="3xx" />
                    <Area type="monotone" dataKey="4xx" stackId="1" stroke={STATUS_COLORS["4xx"]} fill={STATUS_COLORS["4xx"]} fillOpacity={0.6} name="4xx" />
                    <Area type="monotone" dataKey="5xx" stackId="1" stroke={STATUS_COLORS["5xx"]} fill={STATUS_COLORS["5xx"]} fillOpacity={0.6} name="5xx" />
                  </AreaChart>
                </ResponsiveContainer>
              </Card.Body>
            </Card>

            <Card>
              <Card.Header>
                <h3 className="text-lg font-semibold text-gray-900 dark:text-white">Response Time</h3>
              </Card.Header>
              <Card.Body>
                <ResponsiveContainer width="100%" height={280}>
                  <LineChart data={chartData}>
                    <CartesianGrid strokeDasharray="3 3" stroke="#374151" />
                    <XAxis dataKey="time" stroke="#9CA3AF" fontSize={12} />
                    <YAxis stroke="#9CA3AF" fontSize={12} unit="ms" />
                    <Tooltip contentStyle={{ backgroundColor: "#1F2937", border: "1px solid #374151", borderRadius: "8px" }} formatter={(value: number) => [`${value.toFixed(1)}ms`]} />
                    <Legend />
                    <Line type="monotone" dataKey="avgLatency" stroke="#3b82f6" strokeWidth={2} dot={false} name="Avg" />
                    <Line type="monotone" dataKey="p95Latency" stroke="#f59e0b" strokeWidth={2} dot={false} name="P95" />
                  </LineChart>
                </ResponsiveContainer>
              </Card.Body>
            </Card>
          </div>

          {/* Charts Row 2: Status Codes */}
          <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
            <Card>
              <Card.Header>
                <h3 className="text-lg font-semibold text-gray-900 dark:text-white">Status Codes</h3>
              </Card.Header>
              <Card.Body>
                {pieData.length > 0 ? (
                  <ResponsiveContainer width="100%" height={250}>
                    <PieChart>
                      <Pie data={pieData} cx="50%" cy="50%" innerRadius={60} outerRadius={90} paddingAngle={2} dataKey="value" label={({ percent }) => percent > 0.05 ? `${(percent * 100).toFixed(0)}%` : ""} labelLine={false}>
                        {pieData.map((entry, index) => <Cell key={`cell-${index}`} fill={entry.color} />)}
                      </Pie>
                      <Tooltip contentStyle={{ backgroundColor: "#1F2937", border: "1px solid #374151", borderRadius: "8px" }} formatter={(value: number) => [formatNumber(value), "Requests"]} />
                      <Legend />
                    </PieChart>
                  </ResponsiveContainer>
                ) : (
                  <div className="flex items-center justify-center h-[250px] text-gray-500">No data available</div>
                )}
              </Card.Body>
            </Card>

            <Card className="lg:col-span-2">
              <Card.Header>
                <h3 className="text-lg font-semibold text-gray-900 dark:text-white">Status Code Breakdown</h3>
              </Card.Header>
              <Card.Body>
                {statusCodes?.status_codes && statusCodes.status_codes.length > 0 ? (
                  <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
                    {statusCodes.status_codes.slice(0, 8).map((sc) => (
                      <div key={sc.status_code} className="p-3 bg-gray-50 dark:bg-gray-800 rounded-lg">
                        <div className="flex items-center justify-between mb-1">
                          <Badge color={sc.status_code >= 500 ? "red" : sc.status_code >= 400 ? "yellow" : sc.status_code >= 300 ? "blue" : "green"} size="sm">{sc.status_code}</Badge>
                          <span className="text-xs text-gray-500">{sc.percentage.toFixed(1)}%</span>
                        </div>
                        <div className="text-lg font-semibold text-gray-900 dark:text-white">{formatNumber(sc.count)}</div>
                      </div>
                    ))}
                  </div>
                ) : (
                  <div className="flex items-center justify-center h-[200px] text-gray-500">No status code data available</div>
                )}
              </Card.Body>
            </Card>
          </div>

          {/* Top Paths */}
          <Card>
            <Card.Header>
              <div className="flex items-center justify-between">
                <h3 className="text-lg font-semibold text-gray-900 dark:text-white">Top Endpoints</h3>
                <Badge color="gray" size="sm">{topPathsResponse?.paths?.length || 0} paths</Badge>
              </div>
            </Card.Header>
            <Card.Body className="p-0">
              <Table
                columns={topPathsColumns}
                data={topPathsResponse?.paths || []}
                keyExtractor={(row) => row.path}
                hoverable
                emptyState={<EmptyState icon={Globe} title="No endpoint data" description="Top endpoints will appear once traffic is recorded" size="sm" />}
              />
            </Card.Body>
          </Card>

          {/* Row 3: Methods, Devices, Browsers */}
          <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
            <Card>
              <Card.Header>
                <h3 className="text-lg font-semibold text-gray-900 dark:text-white">Request Methods</h3>
              </Card.Header>
              <Card.Body>
                {methodsData?.methods && methodsData.methods.length > 0 ? (
                  <div className="space-y-3">
                    {methodsData.methods.map((m) => (
                      <div key={m.method} className="flex items-center gap-3">
                        <Badge color={m.method === "GET" ? "green" : m.method === "POST" ? "blue" : m.method === "PUT" ? "yellow" : m.method === "DELETE" ? "red" : m.method === "PATCH" ? "purple" : "gray"} className="w-16 justify-center">{m.method}</Badge>
                        <div className="flex-1">
                          <div className="flex justify-between text-sm mb-1">
                            <span className="text-gray-600 dark:text-gray-400">{formatNumber(m.count)} requests</span>
                            <span className="text-gray-500">{m.percentage.toFixed(1)}%</span>
                          </div>
                          <div className="h-2 bg-gray-200 dark:bg-gray-700 rounded-full overflow-hidden">
                            <div className="h-full rounded-full transition-all" style={{ width: `${m.percentage}%`, backgroundColor: METHOD_COLORS[m.method] || "#6b7280" }} />
                          </div>
                        </div>
                      </div>
                    ))}
                  </div>
                ) : (
                  <div className="flex items-center justify-center h-[200px] text-gray-500">No method data available</div>
                )}
              </Card.Body>
            </Card>

            <Card>
              <Card.Header>
                <h3 className="text-lg font-semibold text-gray-900 dark:text-white">Device Types</h3>
              </Card.Header>
              <Card.Body>
                {userAgentData?.devices && userAgentData.devices.length > 0 ? (
                  <ResponsiveContainer width="100%" height={220}>
                    <PieChart>
                      <Pie data={userAgentData.devices} cx="50%" cy="50%" innerRadius={50} outerRadius={80} paddingAngle={2} dataKey="count" nameKey="name" label={({ name, percent }) => percent > 0.05 ? `${name}` : ""} labelLine={false}>
                        {userAgentData.devices.map((entry, index) => <Cell key={`cell-${index}`} fill={DEVICE_COLORS[entry.name] || "#6b7280"} />)}
                      </Pie>
                      <Tooltip contentStyle={{ backgroundColor: "#1F2937", border: "1px solid #374151", borderRadius: "8px" }} formatter={(value: number, name: string) => [`${formatNumber(value)} (${((value / (userAgentData?.total || 1)) * 100).toFixed(1)}%)`, name]} />
                      <Legend />
                    </PieChart>
                  </ResponsiveContainer>
                ) : (
                  <div className="flex items-center justify-center h-[220px] text-gray-500">No device data available</div>
                )}
              </Card.Body>
            </Card>

            <Card>
              <Card.Header>
                <h3 className="text-lg font-semibold text-gray-900 dark:text-white">Browsers</h3>
              </Card.Header>
              <Card.Body>
                {userAgentData?.browsers && userAgentData.browsers.length > 0 ? (
                  <div className="space-y-3">
                    {userAgentData.browsers.slice(0, 6).map((b) => (
                      <div key={b.name} className="flex items-center gap-3">
                        <span className="w-16 text-sm font-medium text-gray-700 dark:text-gray-300">{b.name}</span>
                        <div className="flex-1">
                          <div className="h-2 bg-gray-200 dark:bg-gray-700 rounded-full overflow-hidden">
                            <div className="h-full rounded-full transition-all" style={{ width: `${b.percentage}%`, backgroundColor: BROWSER_COLORS[b.name] || "#6b7280" }} />
                          </div>
                        </div>
                        <span className="text-sm text-gray-500 w-16 text-right">{b.percentage.toFixed(1)}%</span>
                      </div>
                    ))}
                  </div>
                ) : (
                  <div className="flex items-center justify-center h-[200px] text-gray-500">No browser data available</div>
                )}
              </Card.Body>
            </Card>
          </div>

          {/* Top Clients */}
          <Card>
            <Card.Header>
              <div className="flex items-center justify-between">
                <h3 className="text-lg font-semibold text-gray-900 dark:text-white">Top Client IPs</h3>
                <Badge color="gray" size="sm">{clientsData?.clients?.length || 0} clients</Badge>
              </div>
            </Card.Header>
            <Card.Body className="p-0">
              {clientsData?.clients && clientsData.clients.length > 0 ? (
                <div className="overflow-x-auto">
                  <table className="w-full">
                    <thead className="bg-gray-50 dark:bg-gray-800">
                      <tr>
                        <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase">IP Address</th>
                        <th className="px-4 py-3 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase">Requests</th>
                        <th className="px-4 py-3 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase">Bandwidth</th>
                        <th className="px-4 py-3 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase">Avg Latency</th>
                        <th className="px-4 py-3 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase">Errors</th>
                        <th className="px-4 py-3 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase">Last Seen</th>
                      </tr>
                    </thead>
                    <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
                      {clientsData.clients.map((client) => (
                        <tr key={client.client_ip} className="hover:bg-gray-50 dark:hover:bg-gray-800/50">
                          <td className="px-4 py-3"><span className="font-mono text-sm text-gray-900 dark:text-white">{client.client_ip}</span></td>
                          <td className="px-4 py-3 text-right"><span className="font-medium text-gray-900 dark:text-white">{formatNumber(client.request_count)}</span></td>
                          <td className="px-4 py-3 text-right text-gray-600 dark:text-gray-400">{formatBytes(client.total_bytes)}</td>
                          <td className="px-4 py-3 text-right text-gray-600 dark:text-gray-400">{formatResponseTime(client.avg_response_time)}</td>
                          <td className="px-4 py-3 text-right"><span className={client.error_rate > 5 ? "text-red-500" : "text-green-500"}>{client.error_rate.toFixed(1)}%</span></td>
                          <td className="px-4 py-3 text-right text-gray-500 text-sm">{new Date(client.last_seen).toLocaleTimeString()}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              ) : (
                <div className="flex items-center justify-center py-12 text-gray-500">No client data available</div>
              )}
            </Card.Body>
          </Card>
        </>
      )}
    </div>
  );
}
