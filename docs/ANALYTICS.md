# Traffic Analytics

InfraPilot CE provides real-time traffic visibility by ingesting Nginx access logs into TimescaleDB and rendering them in the dashboard.

## How It Works

```
Nginx access log
      │
      ▼
  Agent (collector)
      │  streams log lines over gRPC
      ▼
  Backend (ingestion)
      │  writes to TimescaleDB hypertable
      ▼
  TimescaleDB (storage)
      │  continuous aggregates + retention policy
      ▼
  Dashboard (visualization)
```

1. The **agent** tails the Nginx access log file (`/var/log/nginx/access.log`) and streams new entries to the backend over the existing gRPC connection.
2. The **backend** parses each log line (standard combined log format) and inserts it into the `nginx_logs` TimescaleDB hypertable.
3. TimescaleDB's **continuous aggregates** pre-compute per-minute summaries for fast dashboard queries.
4. A **retention policy** automatically drops raw log data older than 24 hours (CE limit).

## Available Metrics

Navigate to **Traffic → Analytics** to see:

| Metric | Description |
|--------|-------------|
| **Request Rate** | Requests per minute, with error overlay |
| **Error Rate** | Percentage of 4xx + 5xx responses |
| **Status Code Breakdown** | Distribution of 2xx, 3xx, 4xx, 5xx |
| **Top Paths** | Most-requested URL paths |
| **Top Client IPs** | Highest-traffic source IPs |
| **Latency** | Average upstream response time (if logged) |

All charts default to the last hour and can be filtered to the last 6 or 24 hours using the time range selector.

## Per-Domain Filtering

If you have multiple proxy hosts, use the **Domain** dropdown to filter metrics to a single domain. Analytics are stored per-domain using the Nginx `$host` variable.

## Data Retention

Community Edition retains **24 hours** of raw log data. Older data is dropped automatically by TimescaleDB's retention policy.

## Requirements

Traffic analytics require **TimescaleDB**. Standard PostgreSQL does not support the hypertable and continuous aggregate features used.

- **Development stack:** uses `timescale/timescaledb:latest-pg16` automatically
- **Production stack (`docker-compose.prod.yml`):** uses standard `postgres:16-alpine` by default — swap to `timescale/timescaledb:latest-pg16` to enable analytics

The `NGINX_LOG_ANALYTICS=true` environment variable on the agent enables log streaming. It is on by default.

## Enabling Analytics

1. Ensure TimescaleDB is your database image
2. Set `NGINX_LOG_ANALYTICS=true` on the agent (default)
3. Set `NGINX_ACCESS_LOG_PATH` to the correct log file path (default: `/var/log/nginx/access.log`)
4. Make sure Nginx logs are not symlinked to `/dev/stdout` — they must be real files on disk

In the production multi-container setup, the agent and Nginx containers share the `nginx_dev_logs` volume so the agent can read Nginx's log files.

## Log Format

InfraPilot expects **Nginx combined log format**:

```nginx
log_format combined '$remote_addr - $remote_user [$time_local] '
                    '"$request" $status $body_bytes_sent '
                    '"$http_referer" "$http_user_agent"';
```

This is Nginx's default. If you've customised the format, the agent may not parse all fields correctly. The `deployments/nginx-dev.conf` shows the expected format.

## Troubleshooting

**No data in analytics dashboard**

1. Check the agent logs: `docker compose logs agent | grep analytics`
2. Confirm `NGINX_LOG_ANALYTICS=true` is set on the agent
3. Verify Nginx is writing to a real log file (not stdout): `docker exec infrapilot-nginx ls -la /var/log/nginx/`
4. Confirm TimescaleDB extension is active: run `SELECT extname FROM pg_extension;` — you should see `timescaledb`

**"TimescaleDB extension not found" error in backend logs**

You're using standard PostgreSQL. Switch the database image to `timescale/timescaledb:latest-pg16` and recreate the container (this will drop existing data, so migrate first if needed).

**Analytics stop updating after running for a while**

The agent reads the log file from the current position. If Nginx rotates the log file (e.g. via `logrotate`), the agent will stop reading until it detects the file has been replaced. By default the dev stack disables Nginx log rotation. In production, configure `logrotate` with `copytruncate` mode or send `SIGUSR1` to Nginx to reopen logs.
