# Nginx Log Analytics System

> Production-grade nginx log collection and analytics using TimescaleDB, with real-time dashboards and persistent storage.

## Table of Contents

1. [Architecture Overview](#architecture-overview)
2. [Data Flow](#data-flow)
3. [Agent Collector](#agent-collector)
4. [Backend Ingestion](#backend-ingestion)
5. [Database Schema](#database-schema)
6. [API Endpoints](#api-endpoints)
7. [Frontend Dashboard](#frontend-dashboard)
8. [Configuration](#configuration)
9. [Performance](#performance)

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                           NGINX LOG ANALYTICS SYSTEM                             │
└─────────────────────────────────────────────────────────────────────────────────┘

┌─────────────┐     ┌─────────────┐     ┌─────────────┐     ┌─────────────────────┐
│   Nginx     │────▶│   Agent     │────▶│   Backend   │────▶│    TimescaleDB      │
│ access.log  │     │  Collector  │     │  Ingestion  │     │    Hypertable       │
└─────────────┘     └─────────────┘     └─────────────┘     └─────────────────────┘
                     500 entries          COPY bulk              │
                     5s flush             insert                 │
                                                    ┌────────────┴────────────┐
                                                    ▼                         ▼
                                          ┌─────────────────┐      ┌─────────────────┐
                                          │ Continuous Agg  │      │ Continuous Agg  │
                                          │ nginx_stats_1m  │      │ nginx_stats_1h  │
                                          │ (30s refresh)   │      │ (hourly)        │
                                          └─────────────────┘      └─────────────────┘
                                                    │
                                                    ▼
                                          ┌─────────────────┐
                                          │    Frontend     │
                                          │   Analytics     │
                                          │   Dashboard     │
                                          └─────────────────┘
```

---

## Data Flow

### Complete Flow Diagram

```
1. NGINX (writes logs)
   │
   │  Log format: infrapilot_analytics
   │  '$remote_addr - $remote_user [$time_local] "$request" '
   │  '$status $body_bytes_sent "$http_referer" '
   │  '"$http_user_agent" $request_time "$host" "$upstream_addr"'
   │
   ▼
2. LOG FILES
   │  /var/log/nginx/access.log           (main access log)
   │  /var/log/nginx/domains/*.access.log (per-domain logs)
   │
   │  Symlinked to /data/nginx/logs/ for persistence
   │
   ▼
3. AGENT COLLECTOR (fsnotify file watcher)
   │
   │  - Watches /var/log/nginx and /var/log/nginx/domains
   │  - Parses each line with regex (extended format)
   │  - Extracts structured fields
   │  - Buffers up to 500 entries OR flushes every 5 seconds
   │  - Filters out health checks (/health, /healthz, etc.)
   │
   ▼
4. HTTP POST to Backend
   │
   │  POST /api/v1/logs/nginx/ingest
   │  {
   │    "agent_id": "00000000-0000-0000-0000-000000000001",
   │    "entries": [
   │      {
   │        "time": "2026-02-06T12:00:00Z",
   │        "client_ip": "192.168.1.1",
   │        "method": "GET",
   │        "path": "/api/users",
   │        "status_code": 200,
   │        "response_bytes": 1234,
   │        "response_time": 0.042,
   │        "host": "api.example.com",
   │        "user_agent": "Mozilla/5.0..."
   │      },
   │      ...
   │    ]
   │  }
   │
   ▼
5. BACKEND INGESTION
   │
   │  - Validates batch
   │  - Uses PostgreSQL COPY for bulk insert (10,000+ rows/sec)
   │  - Inserts into nginx_access_logs hypertable
   │
   ▼
6. TIMESCALEDB HYPERTABLE
   │
   │  Table: nginx_access_logs
   │  - Auto-partitioned by time (daily chunks)
   │  - Compression after 7 days (~90% reduction)
   │  - Retention: 90 days (configurable)
   │
   ▼
7. CONTINUOUS AGGREGATES (pre-computed)
   │
   │  nginx_stats_1m  - 1-minute buckets, refreshes every 30s
   │  nginx_stats_1h  - 1-hour buckets, refreshes hourly
   │
   ▼
8. FRONTEND QUERIES
   │
   │  GET /api/v1/traffic/analytics?minutes=30&interval=1m
   │  GET /api/v1/traffic/analytics/summary
   │  GET /api/v1/traffic/analytics/top-paths
   │  GET /api/v1/traffic/analytics/status-codes
   │  GET /api/v1/traffic/analytics/domains
   │
   └──▶ Dashboard renders charts & stats
```

---

## Agent Collector

### Overview

The Agent Collector watches nginx log files, parses entries, buffers them, and sends batches to the backend.

**Files:**
- `agent/internal/logstreamer/nginx_collector.go` - File watcher & buffer
- `agent/internal/logstreamer/nginx_parser.go` - Log line parser

### Configuration

```go
type NginxCollectorConfig struct {
    LogPath          string        // Default: "/var/log/nginx/access.log"
    BufferSize       int           // Default: 500 entries
    FlushInterval    time.Duration // Default: 5 seconds
    SkipHealthChecks bool          // Default: true
    NormalizePaths   bool          // Default: false
}
```

### Watched Directories

```
/var/log/nginx/
├── access.log              # Main access log
├── error.log               # Error log
└── domains/
    ├── example.com.access.log
    ├── example.com.error.log
    ├── api.example.com.access.log
    └── ...
```

### Log Parsing

**Extended Format (InfraPilot default):**
```
192.168.1.1 - - [06/Feb/2026:12:00:00 +0000] "GET /api/users HTTP/1.1" 200 1234 "https://example.com" "Mozilla/5.0" 0.042 "api.example.com" "127.0.0.1:8080"
     │                    │                      │      │         │      │           │                    │           │          │              │
     │                    │                      │      │         │      │           │                    │           │          │              └─ upstream
     │                    │                      │      │         │      │           │                    │           │          └─ host/domain
     │                    │                      │      │         │      │           │                    │           └─ response_time (seconds)
     │                    │                      │      │         │      │           │                    └─ user_agent
     │                    │                      │      │         │      │           └─ referer
     │                    │                      │      │         │      └─ response_bytes
     │                    │                      │      │         └─ status_code
     │                    │                      │      └─ path
     │                    │                      └─ method
     │                    └─ timestamp
     └─ client_ip
```

**Parsed Entry Structure:**
```go
type NginxLogEntry struct {
    Time          time.Time `json:"time"`
    ClientIP      string    `json:"client_ip"`
    Method        string    `json:"method"`
    Path          string    `json:"path"`
    QueryString   string    `json:"query_string,omitempty"`
    Protocol      string    `json:"protocol,omitempty"`
    StatusCode    int       `json:"status_code"`
    ResponseBytes int64     `json:"response_bytes"`
    ResponseTime  float64   `json:"response_time,omitempty"`
    Host          string    `json:"host,omitempty"`
    Referer       string    `json:"referer,omitempty"`
    UserAgent     string    `json:"user_agent,omitempty"`
    Upstream      string    `json:"upstream,omitempty"`
}
```

### Filtering

**Health checks are automatically skipped:**

Paths:
- `/health`, `/healthz`, `/ready`, `/readyz`, `/ping`, `/status`, `/_health`

User Agents:
- `kube-probe`, `GoogleHC`, `ELB-HealthChecker`

### Flush Triggers

1. **Buffer full** - When 500 entries accumulated → immediate flush
2. **Timer** - Every 5 seconds → periodic flush
3. **Shutdown** - On graceful stop → final flush

---

## Backend Ingestion

### Endpoint

```
POST /api/v1/logs/nginx/ingest
```

**File:** `backend/internal/api/nginx_analytics_handlers.go`

### Request Format

```json
{
  "agent_id": "00000000-0000-0000-0000-000000000001",
  "entries": [
    {
      "time": "2026-02-06T12:00:00Z",
      "client_ip": "192.168.1.1",
      "method": "GET",
      "path": "/api/users",
      "query_string": "page=1",
      "protocol": "HTTP/1.1",
      "status_code": 200,
      "response_bytes": 1234,
      "response_time": 0.042,
      "host": "api.example.com",
      "referer": "https://example.com",
      "user_agent": "Mozilla/5.0...",
      "upstream": "127.0.0.1:8080"
    }
  ]
}
```

### Bulk Insert

Uses PostgreSQL `COPY` command for high-performance bulk inserts:

```go
// Achieves 10,000+ inserts/second
_, err = conn.CopyFrom(
    ctx,
    pgx.Identifier{"nginx_access_logs"},
    columns,
    pgx.CopyFromRows(rows),
)
```

---

## Database Schema

### Main Table: nginx_access_logs

```sql
CREATE TABLE nginx_access_logs (
    time            TIMESTAMPTZ NOT NULL,
    org_id          UUID NOT NULL,
    agent_id        UUID NOT NULL,
    client_ip       INET,
    method          VARCHAR(10),
    path            TEXT,
    query_string    TEXT,
    protocol        VARCHAR(20),
    status_code     SMALLINT,
    response_bytes  BIGINT,
    response_time   DOUBLE PRECISION,
    host            VARCHAR(255),
    referer         TEXT,
    user_agent      TEXT,
    upstream        VARCHAR(255)
);

-- Convert to TimescaleDB hypertable (auto-partitioned by day)
SELECT create_hypertable('nginx_access_logs', 'time', chunk_time_interval => INTERVAL '1 day');

-- Indexes for common queries
CREATE INDEX idx_nginx_logs_org_time ON nginx_access_logs (org_id, time DESC);
CREATE INDEX idx_nginx_logs_host ON nginx_access_logs (host, time DESC);
CREATE INDEX idx_nginx_logs_status ON nginx_access_logs (status_code, time DESC);
```

### Continuous Aggregates

**1-Minute Stats (real-time dashboards):**
```sql
CREATE MATERIALIZED VIEW nginx_stats_1m
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 minute', time) AS bucket,
    org_id,
    host,
    COUNT(*) AS total_requests,
    SUM(CASE WHEN status_code >= 200 AND status_code < 300 THEN 1 ELSE 0 END) AS count_2xx,
    SUM(CASE WHEN status_code >= 300 AND status_code < 400 THEN 1 ELSE 0 END) AS count_3xx,
    SUM(CASE WHEN status_code >= 400 AND status_code < 500 THEN 1 ELSE 0 END) AS count_4xx,
    SUM(CASE WHEN status_code >= 500 THEN 1 ELSE 0 END) AS count_5xx,
    SUM(response_bytes) AS total_bytes,
    AVG(response_time) AS avg_response_time,
    PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY response_time) AS p95_response_time,
    COUNT(DISTINCT client_ip) AS unique_visitors
FROM nginx_access_logs
GROUP BY bucket, org_id, host;

-- Refresh every 30 seconds
SELECT add_continuous_aggregate_policy('nginx_stats_1m',
    start_offset => INTERVAL '10 minutes',
    end_offset => INTERVAL '1 minute',
    schedule_interval => INTERVAL '30 seconds');
```

### Data Lifecycle

```sql
-- Compression after 7 days (~90% space savings)
SELECT add_compression_policy('nginx_access_logs', INTERVAL '7 days');

-- Retention: drop data older than 90 days
SELECT add_retention_policy('nginx_access_logs', INTERVAL '90 days');
```

---

## API Endpoints

### Analytics Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/traffic/analytics` | GET | Time-series metrics |
| `/api/v1/traffic/analytics/summary` | GET | Summary stats for time range |
| `/api/v1/traffic/analytics/top-paths` | GET | Top N paths by request count |
| `/api/v1/traffic/analytics/status-codes` | GET | Status code distribution |
| `/api/v1/traffic/analytics/domains` | GET | Unique domains from logs |
| `/api/v1/traffic/analytics/methods` | GET | Request method distribution (GET/POST/etc.) |
| `/api/v1/traffic/analytics/clients` | GET | Top client IPs by request count |
| `/api/v1/traffic/analytics/user-agents` | GET | User agent/browser classification |

### Query Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `minutes` | int | - | Time range in minutes |
| `hours` | int | 24 | Time range in hours (if minutes not set) |
| `interval` | string | "1m" | Aggregation interval (1m, 5m, 1h, 1d) |
| `agent_id` | uuid | - | Filter by agent |
| `domain` | string | - | Filter by domain/host |
| `limit` | int | 20 | Max results (for top-paths) |

### Example Requests

```bash
# Get 30-minute analytics with 1-minute intervals
GET /api/v1/traffic/analytics?minutes=30&interval=1m

# Get summary for last 24 hours
GET /api/v1/traffic/analytics/summary?hours=24

# Get top 20 paths for specific domain
GET /api/v1/traffic/analytics/top-paths?hours=24&domain=api.example.com&limit=20

# Get status code distribution
GET /api/v1/traffic/analytics/status-codes?minutes=60

# Get unique domains from logs
GET /api/v1/traffic/analytics/domains?hours=24
```

### Log Retrieval Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/agents/:id/logs/nginx` | GET | Get nginx logs |

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `type` | string | "access" | Log type (access/error) |
| `lines` | int | 100 | Number of lines |
| `domain` | string | - | Filter by domain |
| `source` | string | "db" | Source: "db" (persistent) or "file" (real-time) |

---

## Frontend Dashboard

### Location

```
/traffic/analytics - Analytics dashboard
/traffic/logs      - Log viewer
```

### Analytics Features

1. **Time Range Selector** - 5m, 15m, 30m, 1h, 6h, 24h, 7d
2. **Domain Filter** - Dropdown populated from proxy hosts
3. **Summary Stats** - Total requests, error rate, avg latency, P95, unique visitors
4. **Request Rate Chart** - Stacked area chart by status code (2xx, 4xx, 5xx)
5. **Response Time Chart** - Line chart with avg and P95
6. **Status Code Distribution** - Pie chart and detailed breakdown
7. **Top Paths Table** - Path, requests, latency, bandwidth, error rate
8. **Request Methods** - Distribution of GET/POST/PUT/DELETE with bar chart
9. **Device Types** - Desktop/Mobile/Tablet/Bot breakdown with pie chart
10. **Browser Analytics** - Chrome/Firefox/Safari/Edge distribution
11. **Top Client IPs** - Table with request count, bandwidth, latency, error rate
12. **Auto-refresh** - Toggle for 30-second refresh interval

### Logs Features

1. **Log Type Toggle** - Access logs / Error logs
2. **Domain Filter** - Filter by specific domain
3. **Status Filter** - All / 2xx-3xx / 4xx / 5xx
4. **Method Filter** - Filter by HTTP method (GET/POST/PUT/DELETE)
5. **IP Filter** - Filter logs by client IP address
6. **Search** - Full-text search across log messages
7. **Colorful Display**:
   - Green badge: 2xx status codes
   - Yellow badge: 4xx status codes
   - Red badge: 5xx status codes
   - Method colors: GET=green, POST=blue, PUT=yellow, DELETE=red
8. **Timestamp** - HH:MM:SS format
9. **Domain Column** - Shows which domain each request belongs to
10. **Export** - Download as TXT, JSON, or CSV

---

## Configuration

### Nginx Log Format

**Set in:** `deployments/docker-entrypoint.sh`

```nginx
log_format infrapilot_analytics
    '$remote_addr - $remote_user [$time_local] "$request" '
    '$status $body_bytes_sent "$http_referer" '
    '"$http_user_agent" $request_time "$host" "$upstream_addr"';

access_log /var/log/nginx/access.log infrapilot_analytics;
```

### Per-Domain Logging

When proxy hosts are created, per-domain log files are configured:

```nginx
# In each proxy host config
access_log /var/log/nginx/domains/example.com.access.log infrapilot_analytics;
error_log /var/log/nginx/domains/example.com.error.log warn;
```

### Log Persistence

Nginx logs are symlinked to the data volume for persistence across container restarts:

```bash
# In docker-entrypoint.sh
ln -s "$DATA_DIR/nginx/logs" /var/log/nginx
```

### TimescaleDB

Enabled in PostgreSQL configuration:

```bash
# In docker-entrypoint.sh
echo "shared_preload_libraries = 'timescaledb'" >> "$DATA_DIR/postgres/postgresql.conf"
```

---

## Performance

### Benchmarks

| Metric | Value |
|--------|-------|
| Ingestion rate | 10,000+ logs/second |
| Dashboard query | <100ms (using continuous aggregates) |
| Storage compression | ~90% after 7 days |
| Retention | 90 days default |

### Optimization Tips

1. **Continuous Aggregates** - Pre-compute stats for fast dashboard queries
2. **COPY Bulk Insert** - Much faster than individual INSERTs
3. **Hypertable Partitioning** - Auto-partitioned by day for efficient time-range queries
4. **Compression** - Automatically compress old data
5. **Health Check Filtering** - Skip noisy health check requests

---

## Files Reference

### Agent
| File | Purpose |
|------|---------|
| `agent/internal/logstreamer/nginx_collector.go` | File watcher, buffer, flush |
| `agent/internal/logstreamer/nginx_parser.go` | Log line regex parsing |
| `agent/cmd/agent/main.go` | Starts nginx collector |

### Backend
| File | Purpose |
|------|---------|
| `backend/internal/api/nginx_analytics_handlers.go` | Analytics API handlers |
| `backend/internal/api/logs_handlers.go` | Log retrieval handlers |
| `backend/internal/api/handler.go` | Route registration |
| `backend/internal/db/migrations/044_nginx_analytics_timescale.up.sql` | Schema |

### Frontend
| File | Purpose |
|------|---------|
| `frontend/app/(dashboard)/traffic/analytics/page.tsx` | Analytics dashboard |
| `frontend/app/(dashboard)/traffic/logs/page.tsx` | Log viewer |
| `frontend/lib/api.ts` | API client functions |

### Infrastructure
| File | Purpose |
|------|---------|
| `deployments/docker-entrypoint.sh` | Nginx config, TimescaleDB setup |
| `Dockerfile` | Includes timescaledb package |

---

## Troubleshooting

### No logs appearing

1. Check agent is running: `docker logs infrapilot | grep nginx`
2. Verify log files exist: `ls -la /var/log/nginx/`
3. Check database: `SELECT COUNT(*) FROM nginx_access_logs;`

### Domain filter not working

1. Ensure proxy hosts are configured
2. Check `host` field is populated in logs
3. Verify agent is selected in UI

### High latency queries

1. Check continuous aggregates are refreshing
2. Verify indexes exist on nginx_access_logs
3. Consider reducing time range

### Logs not persisting

1. Check symlink: `ls -la /var/log/nginx`
2. Verify data volume is mounted
3. Check `/data/nginx/logs/` permissions
