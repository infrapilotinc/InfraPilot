<p align="center">
  <img src="docs/assets/logo.svg" alt="InfraPilot Logo" width="120" height="120">
</p>

<h1 align="center">InfraPilot Community Edition</h1>

<p align="center">
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-Apache%202.0-blue.svg" alt="License"></a>
  <a href="https://hub.docker.com/r/devsimplex/infrapilot"><img src="https://img.shields.io/badge/Docker-Hub-2496ED?logo=docker" alt="Docker"></a>
  <a href="https://go.dev/"><img src="https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go" alt="Go Version"></a>
  <a href="https://nextjs.org/"><img src="https://img.shields.io/badge/Next.js-16-000000?logo=next.js" alt="Next.js"></a>
</p>

<p align="center">
  <strong>Docker-native infrastructure control plane</strong> for managing traffic, containers, logs, security, and alerts — without touching the host OS.
</p>

<p align="center">
  <img src="docs/assets/infrapilot-preview.png" alt="InfraPilot Dashboard" width="800">
</p>

## Who is InfraPilot for?

- SaaS founders running multiple Dockerized services on a single server
- DevOps teams who want visibility without SSH access
- Agencies managing client apps on shared infrastructure
- Engineers who want Nginx + Docker + observability in one control plane

## What InfraPilot is NOT

- Not a hosting control panel (cPanel, Plesk)
- Not a Kubernetes replacement
- Not a VM manager


## How InfraPilot compares

| Feature | InfraPilot | Nginx Proxy Manager | Portainer |
|------|-----------|--------------------|-----------|
| Reverse proxy | ✅ Advanced | ✅ Basic | ❌ |
| SSL automation | ✅ | ✅ | ❌ |
| Container exec | ✅ | ❌ | ✅ |
| Unified logs | ✅ | ❌ | ❌ |
| RBAC + MFA | ✅ | ❌ | ❌ |
| Docker-native | ✅ | ⚠️ | ✅ |


## Why InfraPilot?

- **No SSH Required** — All operations through Docker API and Nginx management
- **Container-First** — Everything maps to containers, reverse proxy, logs, metrics, alerts
- **Security by Design** — Least-privilege, mTLS agent communication, RBAC
- **Batteries Included** — Nginx bundled with automatic SSL (Let's Encrypt)

## Quick Start

### One Command Deploy

```bash
docker run -d \
  --name infrapilot \
  -p 80:80 -p 443:443 \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -v infrapilot_data:/data \
  -e JWT_SECRET=$(openssl rand -base64 32) \
  devsimplex/infrapilot
```

### Using Docker Compose (Recommended)

```bash
# Create project directory
mkdir infrapilot && cd infrapilot

# Create docker-compose.yml
cat > docker-compose.yml << 'EOF'
services:
  infrapilot:
    image: devsimplex/infrapilot:latest
    container_name: infrapilot
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
    environment:
      JWT_SECRET: ${JWT_SECRET:?Run: export JWT_SECRET=$(openssl rand -base64 32)}
    volumes:
      - infrapilot_data:/data
      - /var/run/docker.sock:/var/run/docker.sock:ro

volumes:
  infrapilot_data:
EOF

# Generate JWT secret and start
export JWT_SECRET=$(openssl rand -base64 32)
docker compose up -d
```

### Access Dashboard

- **URL:** http://localhost

On first start, you'll be prompted to create your admin account.
No default credentials are used - you set your own email and password.

> **Your first account will have full administrative access.**

## Features

### Reverse Proxy Management
- Visual Nginx configuration with live preview
- Automatic SSL certificates (Let's Encrypt)
- Security headers (HSTS, CSP, X-Frame-Options)
- Rate limiting and IP allow/deny lists
- Dynamic Docker network attachment

### Container Operations
- Container list with real-time metrics
- Start, stop, restart controls
- Live log streaming
- Web-based terminal (container exec)
- Docker Compose stack grouping

### Observability
- Unified log aggregation
- Real-time log streaming with search
- Nginx access and error logs
- Container log collection

### Alerting
- Multiple channels: SMTP, Slack, Webhooks
- Container crash detection
- SSL expiry warnings
- High error rate alerts

### Security
- Role-based access control (RBAC)
- Multi-factor authentication (TOTP)
- Complete audit trail
- TLS health scoring

## Architecture

```
┌───────────────────────────────────────────────────────┐
│              devsimplex/infrapilot                    │
│  ┌──────────────────────────────────────────────────┐ │
│  │                    Nginx                         │ │
│  │                  :80 / :443                      │ │
│  └──────────┬─────────────────┬─────────────────────┘ │
│             │                 │                       │
│  ┌──────────▼──────┐  ┌───────▼────────┐              │
│  │    Frontend     │  │    Backend     │              │
│  │   (Next.js)     │  │     (Go)       │              │
│  │     :3000       │  │  :8080 / :9090 │              │
│  └─────────────────┘  └───────┬────────┘              │
│                               │ gRPC                  │
│                       ┌───────▼────────┐              │
│                       │     Agent      │──► Docker    │
│                       │     (Go)       │    Daemon    │
│                       └────────────────┘              │
│                                                       │
│  ┌─────────────────┐  ┌─────────────────┐             │
│  │   PostgreSQL    │  │     Redis       │             │
│  │   (embedded)    │  │   (embedded)    │             │
│  └─────────────────┘  └─────────────────┘             │
└───────────────────────────────────────────────────────┘
```

## Configuration

### Required

```bash
# Generate a secure JWT secret
export JWT_SECRET=$(openssl rand -base64 32)
```

### Optional Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `JWT_SECRET` | *required* | Secret for JWT tokens |
| `DATABASE_URL` | embedded | External PostgreSQL URL |
| `REDIS_URL` | embedded | External Redis URL |
| `POSTGRES_PASSWORD` | infrapilot | Embedded DB password |
| `REDIS_PASSWORD` | infrapilot | Embedded Redis password |
| `LETSENCRYPT_EMAIL` | - | Email for SSL certificates |
| `LETSENCRYPT_STAGING` | true | Use Let's Encrypt staging |
| `HTTP_PORT` | 80 | HTTP port |
| `HTTPS_PORT` | 443 | HTTPS port |

### Using External Database

```bash
docker run -d \
  --name infrapilot \
  -p 80:80 \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -v infrapilot_data:/data \
  -e JWT_SECRET=your-secret \
  -e DATABASE_URL=postgres://user:pass@host:5432/infrapilot \
  -e REDIS_URL=redis://:password@host:6379 \
  devsimplex/infrapilot
```

### SSL Configuration

```bash
docker run -d \
  --name infrapilot \
  -p 80:80 -p 443:443 \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -v infrapilot_data:/data \
  -e JWT_SECRET=your-secret \
  -e LETSENCRYPT_EMAIL=admin@yourdomain.com \
  -e LETSENCRYPT_STAGING=false \
  devsimplex/infrapilot
```

## Production Checklist

- [ ] Set strong `JWT_SECRET`
- [ ] Create admin account with strong password
- [ ] Set `POSTGRES_PASSWORD` and `REDIS_PASSWORD`
- [ ] Configure `LETSENCRYPT_EMAIL`
- [ ] Set `LETSENCRYPT_STAGING=false`
- [ ] Use external database for high availability
- [ ] Configure firewall (only expose 80/443)
- [ ] Set up backup for `/data` volume

## Development

```bash
# Clone repository
git clone https://github.com/devsimplex-org/InfraPilot.git
cd InfraPilot

# Start dev infrastructure
./scripts/dev.sh up

# Run backend
cd backend && air

# Run frontend
cd frontend && pnpm install && pnpm dev
```

### Build Image Locally

```bash
docker build -t infrapilot .
```

## Documentation

- [Roadmap](docs/ROADMAP.md)
- [Progress Tracker](docs/PROGRESS.md)
- [Security Policy](SECURITY.md)
- [Contributing Guide](CONTRIBUTING.md)

## Security

Report vulnerabilities to **security@infrapilot.org** — do NOT open public issues.

See [SECURITY.md](SECURITY.md) for details.

## Contributing

We welcome contributions! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

Apache License 2.0 — see [LICENSE](LICENSE)

## Enterprise Edition

Need multi-node clustering, SSO/SAML, or priority support?

Contact **enterprise@infrapilot.org**

---

Built by [DevSimplex](https://github.com/devsimplex-org)
