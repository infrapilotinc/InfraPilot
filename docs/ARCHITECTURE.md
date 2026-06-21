# Architecture

InfraPilot CE is a Docker-native control plane. It ships as a small set of
services that can run together (the all-in-one image) or independently.

## Components

| Component | Path | Language | Responsibility |
|-----------|------|----------|----------------|
| Backend API | `backend/` | Go 1.24 | REST/gRPC API, orchestration, persistence |
| Frontend dashboard | `frontend/` | Next.js 16 / React 19 | Web UI |
| Agent | `agent/` | Go 1.24 | Runs on each managed host; talks to the backend |
| All-in-one | root `Dockerfile` | — | Backend + frontend + agent + NGINX in one image (convenience/legacy) |

The `proto/` directory holds the gRPC contracts shared between backend and
agent. `deployments/` holds the per-service Dockerfiles and compose files.

## Container images

Built and pushed by [`scripts/build-and-publish.sh`](../scripts/build-and-publish.sh)
to GHCR (and optionally Docker Hub). Prefix: `ghcr.io/infrapilothq/infrapilot-ce`.

| Image | Dockerfile |
|-------|-----------|
| `infrapilot-ce-backend` | `deployments/backend.Dockerfile` |
| `infrapilot-ce-frontend` | `deployments/frontend.Dockerfile` |
| `infrapilot-ce-agent` | `deployments/agent.Dockerfile` |
| `infrapilot-ce` (all-in-one) | `Dockerfile` |

These images are **public** on GHCR.

## Topology

```
                +-----------------------------+
                |        Browser (UI)         |
                +--------------+--------------+
                               | HTTPS
                +--------------v--------------+
                |   Frontend (Next.js)        |
                +--------------+--------------+
                               | API
                +--------------v--------------+
                |   Backend API (Go)          |
                +--------------+--------------+
                               | gRPC (proto/)
        +----------------------+----------------------+
        |                      |                      |
   +----v----+            +----v----+            +----v----+
   | Agent   |            | Agent   |            | Agent   |   one per host
   +---------+            +---------+            +---------+
```

See [DEPLOYMENT.md](DEPLOYMENT.md) for how these are built and run, and
[CONFIGURATION.md](CONFIGURATION.md) for runtime settings.
