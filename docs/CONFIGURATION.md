# Configuration Reference

All configuration is via environment variables. The `docker-compose.yml` and `docker-compose.prod.yml` files document the minimum required set. This page covers every available variable.

## Required Variables

| Variable | Description |
|----------|-------------|
| `JWT_SECRET` | Secret key for signing JWT tokens. Generate with `openssl rand -base64 32`. Must be at least 32 characters. |

## Database

| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_URL` | *(embedded)* | Full PostgreSQL connection string. Format: `postgres://user:password@host:5432/dbname?sslmode=disable`. If not set, the all-in-one image uses an embedded PostgreSQL instance. |
| `POSTGRES_USER` | `infrapilot` | PostgreSQL username (used when `DATABASE_URL` is not set) |
| `POSTGRES_PASSWORD` | `infrapilot` | PostgreSQL password (used when `DATABASE_URL` is not set) |
| `POSTGRES_DB` | `infrapilot` | PostgreSQL database name (used when `DATABASE_URL` is not set) |

> **Analytics requirement:** Traffic analytics require TimescaleDB. In production, use `timescale/timescaledb:latest-pg16` instead of standard `postgres:16-alpine`.

## Redis

| Variable | Default | Description |
|----------|---------|-------------|
| `REDIS_URL` | *(embedded)* | Redis connection string. Format: `redis://:password@host:6379`. If not set, the all-in-one image uses an embedded Redis instance. |
| `REDIS_PASSWORD` | `infrapilot` | Redis password (used when `REDIS_URL` is not set) |

## Network

| Variable | Default | Description |
|----------|---------|-------------|
| `HTTP_PORT` | `80` | Port the all-in-one container listens on for HTTP traffic |
| `HTTPS_PORT` | `443` | Port for HTTPS traffic |
| `ALLOWED_ORIGINS` | `http://localhost,https://localhost` | Comma-separated list of allowed CORS origins |

## API Server (Backend)

| Variable | Default | Description |
|----------|---------|-------------|
| `HTTP_PORT` | `8080` | Backend API HTTP port (inside the container) |
| `GRPC_PORT` | `9090` | gRPC port for agent communication |
| `ENV` | `production` | Environment: `development` or `production` |

## SSL / Let's Encrypt

| Variable | Default | Description |
|----------|---------|-------------|
| `LETSENCRYPT_EMAIL` | *(none)* | Email for Let's Encrypt certificate notifications and recovery. Required for SSL automation. |
| `LETSENCRYPT_STAGING` | `true` | Use the Let's Encrypt staging CA (no rate limits, untrusted cert). Set to `false` for production certificates. |

## Agent

| Variable | Default | Description |
|----------|---------|-------------|
| `AGENT_ID` | `00000000-0000-0000-0000-000000000001` | UUID identifying this agent. Use the default for single-server deployments. |
| `BACKEND_GRPC_ADDR` | `backend:9090` | gRPC address of the backend (used in multi-container deployments) |
| `BACKEND_HTTP_URL` | `http://backend:8080` | HTTP address of the backend (used in multi-container deployments) |
| `NGINX_CONTAINER_NAME` | `infrapilot-nginx` | Name of the Nginx container the agent manages |
| `NGINX_CONFIG_PATH` | `/etc/nginx/sites` | Path inside the Nginx container for virtual host config files |
| `NGINX_LOG_ANALYTICS` | `true` | Enable streaming Nginx access logs to the backend for analytics |
| `NGINX_ACCESS_LOG_PATH` | `/var/log/nginx/access.log` | Path to Nginx access log file |
| `DATA_DIR` | `./data` | Host path for persistent data (database, certificates, agent state) |
| `LOG_PERSISTENCE` | `true` | Persist container logs to disk |

## Basic Auth (All-in-one)

Optionally protect the entire InfraPilot dashboard with HTTP Basic Auth at the Nginx level:

| Variable | Default | Description |
|----------|---------|-------------|
| `BASIC_AUTH_USER` | *(none)* | Username for HTTP Basic Auth. Set both user and password to enable. |
| `BASIC_AUTH_PASSWORD` | *(none)* | Password for HTTP Basic Auth |

## Example `.env` File (Production)

```dotenv
# Required
JWT_SECRET=your-random-64-char-secret-here

# Database
POSTGRES_PASSWORD=a-strong-db-password
POSTGRES_USER=infrapilot
POSTGRES_DB=infrapilot

# Redis
REDIS_PASSWORD=a-strong-redis-password

# SSL
LETSENCRYPT_EMAIL=admin@yourdomain.com
LETSENCRYPT_STAGING=false

# Networking
HTTP_PORT=80
HTTPS_PORT=443
ALLOWED_ORIGINS=https://yourdomain.com

# (optional) Restrict dashboard access
# BASIC_AUTH_USER=admin
# BASIC_AUTH_PASSWORD=supersecret
```
