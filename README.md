<p align="center">
  <img src="docs/assets/logo.svg"
       alt="InfraPilot Logo"
       width="120"
       height="120">
</p>

<h1 align="center">InfraPilot Community Edition</h1>

<p align="center">
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-Apache%202.0-blue.svg" alt="License"></a>
  <a href="https://go.dev/"><img src="https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go" alt="Go Version"></a>
  <a href="https://nextjs.org/"><img src="https://img.shields.io/badge/Next.js-16-000000?logo=next.js" alt="Next.js"></a>
</p>

<p align="center">
  <strong>Docker-native infrastructure control plane</strong> — manage traffic, containers, logs, and alerts without touching the host OS.
</p>

<p align="center">
  <img src="docs/assets/infrapilot-preview.png"
       alt="InfraPilot Dashboard"
       width="800">
</p>


## What is InfraPilot CE?

InfraPilot CE is a self-hosted control plane for small teams running Dockerized workloads on a single Linux server. It combines Nginx proxy management, Docker operations, log analytics, and alerting into one dashboard — no Kubernetes, no cloud agent, no SSH required.

### Who it's for

- SaaS founders running multiple Dockerized services on one server
- DevOps teams who want visibility without SSH access
- Agencies managing client apps on shared infrastructure
- Engineers who want Nginx + Docker + observability in one place

### What it is NOT

- Not a hosting control panel (cPanel, Plesk)
- Not a Kubernetes replacement
- Not a VM manager


## Features

### Reverse Proxy & SSL
- Visual Nginx configuration with live preview
- Automatic SSL certificates via Let's Encrypt
- Security headers (HSTS, CSP, X-Frame-Options)
- Rate limiting and IP allowlists/denylists
- Basic authentication per proxy host
- Dynamic Docker network attachment

### Container & Stack Management
- Container list with real-time status
- Start, stop, restart, and delete containers
- Live log streaming and web-based terminal (exec)
- Docker Compose stack deployment wizard
- Image pull, volume and network management

### Traffic Analytics
- Nginx access log ingestion via TimescaleDB
- Real-time request rate, error rate, latency charts
- Top paths, status code distribution, client IPs
- Per-domain filtering, 7-day retention default

### Alerting
- Channels: SMTP, Slack, webhooks
- Rules: container crash, SSL expiry, high error rate
- Alert history

### Security & Access
- Role-based access control (RBAC)
- Multi-factor authentication (TOTP)
- JWT with refresh tokens

### Deployments
- Docker image deployments with rollback
- Redeploy with latest image
- Webhook triggers for CD pipelines


## CE vs Enterprise Edition

| Feature | CE | EE |
|---------|:--:|:--:|
| Reverse proxy + SSL | ✅ | ✅ |
| Container & stack management | ✅ | ✅ |
| Docker deployments + CD webhooks | ✅ | ✅ |
| Traffic analytics (TimescaleDB) | ✅ | ✅ |
| Alerting (SMTP / Slack / webhook) | ✅ | ✅ |
| RBAC + MFA (TOTP) | ✅ | ✅ |
| Log persistence | ✅ | ✅ |
| SSO / OIDC / SAML | ❌ | ✅ |
| Vulnerability scanning (Trivy) | ❌ | ✅ |
| SBOM generation & tracking | ❌ | ✅ |
| Policy-as-code (OPA) | ❌ | ✅ |
| Runtime security & drift detection | ❌ | ✅ |
| Secrets hygiene scanning | ❌ | ✅ |
| Database governance | ❌ | ✅ |
| Code quality integration | ❌ | ✅ |
| Security maturity scoring | ❌ | ✅ |
| Audit logs | ❌ | ✅ |
| Private container registry management | ❌ | ✅ |
| Priority support | ❌ | ✅ |

> CE is Apache 2.0 licensed and free forever. EE requires a license key — contact **enterprise@infrapilot.org**.


## CE Limitations

Be aware of these constraints before deploying CE in production:

**Single server only**
CE manages one Docker host via one agent. There is no multi-node or multi-agent support — each InfraPilot CE instance controls the server it is deployed on.

**No SSO**
Authentication is username + password with optional TOTP. OIDC, SAML, and LDAP/AD integration are EE-only.

**No image scanning before deploy**
CE deploys images directly without vulnerability scanning. You are responsible for vetting images before deployment.

**No audit log**
User actions (logins, proxy changes, deployments) are not recorded to a persistent audit trail in CE.

**No private registry auth**
Image pulls are unauthenticated. To pull from a private registry, configure Docker daemon credentials directly on the host — CE cannot manage registry credentials.

**No policy gates**
Deployments are not checked against policies. There is no way to block a deploy based on image age, CVE score, or custom rules.

**Single organization**
CE is designed for a single team/organization. There is no multi-tenancy.


## How InfraPilot CE compares

| Feature | InfraPilot CE | Nginx Proxy Manager | Portainer |
|---------|:---:|:---:|:---:|
| Reverse proxy | ✅ | ✅ | ❌ |
| SSL automation | ✅ | ✅ | ❌ |
| Container management | ✅ | ❌ | ✅ |
| Container exec / terminal | ✅ | ❌ | ✅ |
| Log analytics | ✅ | ❌ | ❌ |
| Alerting | ✅ | ❌ | ❌ |
| CD webhooks | ✅ | ❌ | ❌ |
| RBAC + MFA | ✅ | ❌ | ✅ (paid) |
| Open source | ✅ | ✅ | ✅ (CE) |


## Quick Start

### Requirements

- Linux x86_64 or ARM64
- Docker 24+ and Docker Compose V2
- 2 CPU cores, 2 GB RAM minimum

### Deploy with Docker Compose (recommended)

```bash
mkdir infrapilot && cd infrapilot

cat > docker-compose.yml << 'EOF'
services:
  postgres:
    image: timescale/timescaledb:latest-pg16
    environment:
      POSTGRES_USER: infrapilot
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:-infrapilot}
      POSTGRES_DB: infrapilot
    volumes:
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U infrapilot"]
      interval: 5s
      retries: 5

  redis:
    image: redis:7-alpine
    volumes:
      - redis_data:/data

  backend:
    image: ghcr.io/tybali/infrapilot-ce:latest
    environment:
      DATABASE_URL: postgres://infrapilot:${POSTGRES_PASSWORD:-infrapilot}@postgres:5432/infrapilot?sslmode=disable
      REDIS_URL: redis://redis:6379
      JWT_SECRET: ${JWT_SECRET:?Set JWT_SECRET}
    depends_on:
      postgres:
        condition: service_healthy
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock

volumes:
  postgres_data:
  redis_data:
EOF

export JWT_SECRET=$(openssl rand -base64 32)
docker compose up -d
```

Then open **http://localhost** — you'll be prompted to create your admin account on first visit.

> **Your first account gets full admin access.** No default credentials are used.

### Environment Variables

| Variable | Required | Description |
|----------|:--------:|-------------|
| `JWT_SECRET` | ✅ | Secret for signing JWT tokens |
| `DATABASE_URL` | ✅ | PostgreSQL connection string |
| `REDIS_URL` | ✅ | Redis connection string |
| `ALLOWED_ORIGINS` | | CORS origins (default: same-origin) |
| `HTTP_PORT` | | HTTP port (default: `8080`) |
| `GRPC_PORT` | | gRPC port for agent (default: `9090`) |

### SSL Configuration

Set `LETSENCRYPT_EMAIL` on the agent container and point your DNS at the server. Certificates are issued and renewed automatically when you create a proxy host.


## Architecture

```
Browser
  │
  ▼
Nginx (port 80/443)
  │ proxy_pass /api  ──────────────────────┐
  │ proxy_pass /     ─────────┐            │
  │                           │            │
  ▼                           ▼            ▼
Frontend (Next.js)        Backend (Go API — :8080)
                               │
                               │ gRPC (:9090)
                               ▼
                          Agent (Go)
                            │     │
                            ▼     ▼
                         Docker  Nginx
                         Daemon  Config
                            │
                            ▼
                    Your containers
```

The **Agent** runs as a container, communicates with the Backend via gRPC, and is the only component that touches the Docker socket and Nginx config files. The Backend and Frontend never need host access.


## Development

```bash
git clone https://github.com/tybali/infrapilot-ce.git
cd infrapilot-ce

docker compose -f docker-compose.dev.yml up --build
```

See [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) for full details.


## Documentation

- [Development Guide](docs/DEVELOPMENT.md)
- [Nginx Analytics](docs/NGINX-ANALYTICS.md)
- [Stack Deployments](docs/STACK-DEPLOYMENTS.md)
- [Proxy Management](docs/PROXY-MANAGEMENT.md)
- [Setup Wizard](docs/SETUP-WIZARD.md)


## Contributing

Contributions welcome. Please open an issue before large changes to discuss direction.


## Security

Report vulnerabilities to **security@infrapilot.org** — do not open public issues.


## License

Apache License 2.0 — see [LICENSE](LICENSE)

---

<p align="center">InfraPilot CE is maintained by <a href="https://github.com/tybali">tybali</a></p>
