# Development Guide

This guide covers setting up a local development environment for InfraPilot CE.

## Prerequisites

| Tool | Version | Purpose |
|------|---------|---------|
| Go | 1.24+ | Backend and Agent |
| Node.js | 20+ | Frontend |
| pnpm | 9+ | Frontend package manager |
| Docker | 24+ | Container runtime |
| Docker Compose V2 | | Multi-service orchestration |

## Quick Start

All development commands go through `./scripts/dev.sh`:

```bash
git clone https://github.com/infrapilothq/infrapilot-ce.git
cd infrapilot-ce

# Show all available commands
./scripts/dev.sh
```

### Option A — Local development (recommended, with hot reload)

Starts PostgreSQL + Redis in Docker, then runs the backend, frontend, agent, and Storybook directly on the host:

```bash
./scripts/dev.sh dev
```

Services available at:

| Service | URL |
|---------|-----|
| Dashboard | http://localhost:3000 |
| Backend API | http://localhost:8080/api/v1 |
| Storybook | http://localhost:6006 |
| PostgreSQL | localhost:5432 (user: `infrapilot`, pass: `infrapilot`) |
| Redis | localhost:6379 |

Stop everything:
```bash
./scripts/dev.sh dev:stop
```

View logs:
```bash
./scripts/dev.sh dev:logs
```

### Option B — Fully containerised (no host ports)

Runs all services inside Docker on an internal network. Useful for testing the full stack without local toolchain:

```bash
./scripts/dev.sh up
```

No ports are exposed to the host. Use exec or `./scripts/dev.sh logs` to inspect services:
```bash
./scripts/dev.sh logs backend
./scripts/dev.sh logs           # all services
```

Stop:
```bash
./scripts/dev.sh down
```

## Project Structure

```
infrapilot-ce/
├── backend/           # Go API server (Gin framework)
│   ├── cmd/server/    # Entry point
│   ├── internal/
│   │   ├── api/       # HTTP handlers
│   │   ├── alerts/    # Alert evaluation and notifications
│   │   ├── auth/      # JWT, TOTP, session management
│   │   └── ...
│   └── migrations/    # SQL migrations (applied at startup)
├── agent/             # Go agent (Docker + Nginx controller)
│   ├── cmd/agent/     # Entry point
│   └── internal/
│       ├── docker/    # Docker API integration
│       └── nginx/     # Nginx config management
├── frontend/          # Next.js 15 dashboard
│   ├── app/           # App Router pages
│   ├── components/    # React components
│   └── lib/           # Utilities and context
├── proto/             # gRPC protocol definitions
│   └── agent/v1/      # Agent service proto
├── deployments/       # Dockerfiles and Nginx configs
├── scripts/           # Developer helper scripts (dev.sh)
└── docs/              # Documentation (you are here)
```

## Database

### Migrations

SQL migration files in `backend/migrations/` are applied automatically at backend startup in numeric order. To add a migration:

1. Create `backend/migrations/NNNN_description.sql` (increment `NNNN`)
2. Write idempotent SQL (`IF NOT EXISTS`, `CREATE OR REPLACE`, etc.)
3. Restart the backend

### Connecting Directly

```bash
# Only db-only mode starts a dev-accessible port
./scripts/dev.sh up:db

psql -h localhost -p 5432 -U infrapilot -d infrapilot
# Password: infrapilot
```

Or via Docker exec:
```bash
docker exec -it infrapilot-dev-postgres psql -U infrapilot -d infrapilot
```

### Resetting the Database

```bash
./scripts/dev.sh reset
```

This destroys all data and volumes — use with care.

### Seeding

```bash
./scripts/dev.sh seed
```

Applies `scripts/seed.sql` — creates a default org and agent entry. After seeding, create your admin account through the setup flow at http://localhost:3000.

## Proto Changes

When modifying `proto/agent/v1/agent.proto`:

```bash
./scripts/dev.sh proto
```

This runs `protoc` and regenerates Go stubs for both the backend and agent.

## Linting and Formatting

```bash
# Go (backend + agent)
go fmt ./...
go vet ./...

# Frontend
cd frontend
pnpm lint
pnpm type-check
```

## Building Production Images

```bash
VERSION=2.0.0

# All-in-one image
docker build \
  --build-arg VERSION=$VERSION \
  --build-arg BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
  -t infrapilothq/infrapilot-ce:$VERSION \
  -t infrapilothq/infrapilot-ce:latest \
  .

# Multi-container (backend, frontend, agent separately)
docker compose -f docker-compose.prod.yml build
```

## Storybook

Component library and UI testing:

```bash
./scripts/dev.sh storybook
# or, within the dev environment:
cd frontend && pnpm storybook
```

Opens at http://localhost:6006.

## Troubleshooting

**`dev.sh dev` — backend or agent won't start**
- Ensure Go 1.24+ is installed: `go version`
- Ensure the DB is healthy: `./scripts/dev.sh up:db` then check `docker logs infrapilot-dev-postgres`
- Check `.dev-logs/backend.log` for startup errors

**Frontend not updating on save**
- `dev.sh dev` runs `pnpm dev` which has Next.js HMR built in — check `.dev-logs/frontend.log`
- On Linux, if inotify events are missed: `echo fs.inotify.max_user_watches=524288 | sudo tee -a /etc/sysctl.conf && sudo sysctl -p`

**Agent can't reach Docker socket**
- Ensure the user is in the `docker` group: `sudo usermod -aG docker $USER` then log out/in

**TimescaleDB extension missing (analytics not working)**
- The dev stack uses `timescale/timescaledb:latest-pg16` which includes the extension automatically
- If you see `extension "timescaledb" does not exist`, run `./scripts/dev.sh reset` to recreate the DB container
