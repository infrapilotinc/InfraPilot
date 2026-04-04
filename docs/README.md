# InfraPilot CE — Documentation

## Quick Navigation

| I want to... | Document |
|--------------|----------|
| Set up a development environment | [DEVELOPMENT.md](DEVELOPMENT.md) |
| Understand nginx log analytics | [NGINX-ANALYTICS.md](NGINX-ANALYTICS.md) |
| Learn about stack deployments | [STACK-DEPLOYMENTS.md](STACK-DEPLOYMENTS.md) |
| Understand proxy management | [PROXY-MANAGEMENT.md](PROXY-MANAGEMENT.md) |
| Understand the setup wizard | [SETUP-WIZARD.md](SETUP-WIZARD.md) |
| Quick-start production deploy | [Root README](../README.md) |

---

## What is InfraPilot CE?

InfraPilot Community Edition is a Docker-native infrastructure control plane for managing traffic, containers, logs, and alerts on a single Linux host. It wraps Nginx, Docker, and observability into one dashboard — no cloud required.

**CE includes:**

| Area | Features |
|------|---------|
| **Auth** | Login, MFA (TOTP), JWT refresh |
| **Docker** | Containers, stacks, images, volumes, networks |
| **Deployments** | Docker deployments, rollback, redeploy, webhooks (CD) |
| **Proxy** | Nginx proxy hosts, basic auth, rate limits, SSL |
| **SSL** | Let's Encrypt automation, DNS challenges |
| **Traffic** | Nginx log analytics, resources, policies, TLS governance |
| **Alerts** | Rules, channels (SMTP/Slack/webhook), history |
| **Monitoring** | Metrics, health checks |
| **Settings** | Users, license, domain |

---

## Architecture

```
┌─────────────────────────────────────────────────┐
│                InfraPilot CE                    │
│                                                 │
│  ┌────────────┐   ┌────────────┐                │
│  │  Frontend  │   │  Backend   │                │
│  │ (Next.js)  │   │   (Go)     │                │
│  │  :3000     │   │ :8080/:9090│                │
│  └────────────┘   └─────┬──────┘                │
│                         │ gRPC                  │
│                   ┌─────▼──────┐                │
│                   │   Agent    │──► Docker API  │
│                   │   (Go)     │──► Nginx mgmt  │
│                   └────────────┘                │
│                                                 │
│  ┌────────────┐   ┌────────────┐                │
│  │ PostgreSQL │   │   Redis    │                │
│  │ +TimescaleDB   │            │                │
│  └────────────┘   └────────────┘                │
└─────────────────────────────────────────────────┘
```

The **Agent** runs alongside your containers, communicates with the Backend via gRPC, and is responsible for all Docker and Nginx operations. It never requires SSH access to the host.

---

## Tech Stack

| Component | Technology |
|-----------|------------|
| Backend | Go 1.24 + Gin |
| Frontend | Next.js 16 + React 19 |
| Database | PostgreSQL 16 + TimescaleDB |
| Cache | Redis 7 |
| Proxy | Nginx |
| Container runtime | Docker 24+ |

---

## Documents

### [DEVELOPMENT.md](DEVELOPMENT.md)
Local development environment setup, hot-reload dev stack, database access, debugging tips.

### [NGINX-ANALYTICS.md](NGINX-ANALYTICS.md)
How nginx log collection works end-to-end: agent file watcher → backend ingestion → TimescaleDB hypertables → dashboard. Includes API reference and database schema.

### [STACK-DEPLOYMENTS.md](STACK-DEPLOYMENTS.md)
Deploying multi-service `docker-compose.yml` files as stacks, including variable substitution, per-service env files, dependency ordering, and the deploy wizard.

### [PROXY-MANAGEMENT.md](PROXY-MANAGEMENT.md)
Proxy host form UI design, basic auth configuration, nginx config generation and live preview.

### [SETUP-WIZARD.md](SETUP-WIZARD.md)
First-run setup wizard: license key entry, admin account creation, startup flow.
