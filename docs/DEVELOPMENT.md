# InfraPilot CE — Development Guide

## Prerequisites

- Docker 24+ and Docker Compose V2
- Git

> Go and Node.js are **not** required on the host — all builds happen inside containers.

---

## Repository Layout

```
infrapilot-ce/
├── backend/          Go API server (Gin, port 8080 HTTP / 9090 gRPC)
├── frontend/         Next.js dashboard (port 3000)
├── agent/            Go agent — manages Docker and Nginx on the host
├── deployments/      Nginx configs for dev and production
├── scripts/          Helper scripts
├── docs/             Documentation
└── docker-compose.dev.yml   Development stack
```

---

## Starting the Dev Stack

```bash
git clone https://github.com/tybali/infrapilot-ce.git
cd infrapilot-ce

docker compose -f docker-compose.dev.yml up --build
```

This starts six containers:

| Container | Purpose |
|-----------|---------|
| `infrapilot-dev-postgres` | PostgreSQL 16 + TimescaleDB |
| `infrapilot-dev-redis` | Redis cache |
| `infrapilot-dev-backend` | Go API with Air hot-reload |
| `infrapilot-dev-frontend` | Next.js with hot-reload |
| `infrapilot-dev-agent` | Agent (static ID `00000000-…-0001`) |
| `infrapilot-nginx` | Nginx (managed by agent) |

### Access

| Service | URL |
|---------|-----|
| Dashboard | http://localhost (via Nginx) |
| Frontend direct | http://localhost:3000 |
| Backend API | http://localhost:8080 |

> Ports are not exposed by default. Use `./scripts/dev.sh dev` for host-accessible ports.

### Stopping

```bash
docker compose -f docker-compose.dev.yml down
```

To also remove volumes (wipe database):
```bash
docker compose -f docker-compose.dev.yml down -v
```

---

## Environment Variables (Dev)

Configured in `docker-compose.dev.yml`. Key variables:

| Variable | Default |
|----------|---------|
| `JWT_SECRET` | `dev-secret-key-for-development-only-32chars` |
| `DATABASE_URL` | `postgres://infrapilot:infrapilot@postgres:5432/infrapilot` |
| `REDIS_URL` | `redis://redis:6379` |
| `AGENT_ID` | `00000000-0000-0000-0000-000000000001` (static for dev) |
| `LOG_PERSISTENCE` | `true` |
| `NGINX_LOG_ANALYTICS` | `true` |

---

## Database

### Migrations

Migrations run automatically on backend startup. Files live at:
```
backend/internal/db/migrations/
  *.up.sql    — applied automatically on start
  *.down.sql  — manual rollback only
```

### Direct Access

```bash
docker exec -it infrapilot-dev-postgres psql -U infrapilot -d infrapilot
```

### Backup & Restore

```bash
# Backup
docker exec infrapilot-dev-postgres pg_dump -U infrapilot -d infrapilot -F c -f /tmp/backup.dump
docker cp infrapilot-dev-postgres:/tmp/backup.dump ./backup.dump

# Restore
docker cp ./backup.dump infrapilot-dev-postgres:/tmp/backup.dump
docker exec infrapilot-dev-postgres pg_restore -U infrapilot -d infrapilot --clean --if-exists --no-owner /tmp/backup.dump
```

---

## Building

### Verify backend compiles

```bash
docker build --target builder -f backend/Dockerfile backend/
```

### Build all images

```bash
docker compose -f docker-compose.dev.yml build
```

---

## Logs & Debugging

```bash
# Follow backend logs
docker logs -f infrapilot-dev-backend

# Follow all services
docker compose -f docker-compose.dev.yml logs -f

# Nginx config test
docker exec infrapilot-nginx nginx -t
docker exec infrapilot-nginx nginx -s reload
```

---

## Production Build

```bash
# Build production image
docker build -t infrapilot-ce:latest .

# Or with version tag
docker build --build-arg VERSION=1.0.0 -t infrapilot-ce:1.0.0 .
```

See the root [README](../README.md) for production deployment instructions.

---

## Tech Stack

| Component | Technology |
|-----------|------------|
| Backend | Go 1.24 + Gin |
| Frontend | Next.js 16 + React 19 |
| Database | PostgreSQL 16 + TimescaleDB |
| Cache | Redis 7 |
| Proxy | Nginx (stable-alpine) |
| Hot reload | Air (backend), Next.js dev server (frontend) |

---

## Troubleshooting

### Backend won't start

```bash
docker logs infrapilot-dev-backend | grep -E "error|Error|FATAL"
```

Check if the DB is healthy:
```bash
docker compose -f docker-compose.dev.yml ps
```

### 502 Bad Gateway in browser

The backend may still be building. Air takes ~30s on first run. Watch:
```bash
docker logs -f infrapilot-dev-backend
```
Wait for: `running...` or `http server started on :8080`

### TimescaleDB extension missing

```bash
docker exec infrapilot-dev-postgres psql -U infrapilot -d infrapilot -c "CREATE EXTENSION IF NOT EXISTS timescaledb;"
```

### Port conflicts

If port 80 or 3000 is in use, check `docker-compose.dev.yml` and comment out the conflicting port mappings, or stop the conflicting service.
