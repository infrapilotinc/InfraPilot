# Changelog

All notable changes to InfraPilot CE are documented here.

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

---

## [2.0.0] — 2026-04-01

**Traffic Analytics · CD Webhooks · Rollback · Proxy Security**

CE v2.0.0 brings real-time traffic visibility, continuous deployment hooks, container rollback, and a comprehensive proxy security layer — all without requiring the Enterprise Edition.

### Added

**Traffic Analytics (real-time, 24-hour rolling window)**
- Request rate, error rate, and HTTP status-code breakdown charts
- Top paths, top client IPs, and latency metrics
- Per-domain filtering via the Nginx access log pipeline
- TimescaleDB hypertable with continuous aggregates and automatic 24-hour retention

**CD Webhooks**
- Per-container webhook URLs for pipeline-triggered redeploys
- Works with GitHub Actions, GitLab CI, Jenkins, and any tool that can send an HTTP `POST`
- Webhook secret for HMAC-SHA256 signature verification

**Deployments & Rollback**
- One-step rollback to the immediately previous container image
- Redeploy with the latest image tag (pull + hot-swap) without touching the terminal
- Deployment status visible in the Containers view

**Proxy Security Controls**
- IP allowlists and denylists per proxy host (CIDR and single IP support)
- HTTP Basic Auth per proxy host with bcrypt-hashed credentials
- Security headers toggle: HSTS, CSP, X-Frame-Options, X-Content-Type-Options, Referrer-Policy
- Configurable Content-Security-Policy per proxy host
- Dynamic Docker network attachment — proxy upstream containers without exposing ports to the host

**Docker Compose Wizard**
- Full Compose stack deploy, update, and teardown from the dashboard
- Inline environment variable injection with `${VAR}` substitution
- Per-service `.env` file upload
- Stack status overview with per-service health

**Alerting Expansion**
- New rule type: `high_error_rate` — fires when container log errors exceed N per minute
- New rule type: `ssl_expiry` — configurable warning and critical day thresholds
- Alert cooldown per rule to prevent notification floods

**Other**
- Stack environment variable editor — edit a deployed stack's variables without re-uploading the Compose file
- JWT refresh token rotation — tokens auto-renew without requiring re-login
- Log viewer improvements: filtering by log level, keyword search, auto-scroll toggle
- Health dashboard redesign with container CPU, memory, and status overview

### Fixed

- Nginx reload could occasionally skip if a proxy host was saved while a previous reload was in progress
- Container exec (web terminal) disconnected immediately on some browsers due to WebSocket handshake timing

---

## [1.0.0] — 2026-01-15

**Initial Release**

### Added

**Reverse Proxy & SSL**
- Visual Nginx proxy host management
- Automatic SSL certificates via Let's Encrypt (ACME HTTP-01)
- Custom SSL certificate upload
- HTTP → HTTPS redirect

**Container Management**
- Container list with real-time status (running, stopped, exited)
- Start, stop, restart, and delete containers
- Image pull from Docker Hub or any registry
- Volume and network management
- Live log streaming
- Web-based terminal (exec into running containers)

**Stack Management**
- Docker Compose stack deployment from the dashboard
- Stack status and log views

**Alerting**
- Alert channels: SMTP email, Slack webhooks, generic HTTP webhooks
- Alert rules: container crash, container stopped, high restart count, high CPU, high memory
- Alert history with severity and metadata

**Security & Access**
- User accounts with role-based access control (admin, operator, viewer)
- Multi-factor authentication (TOTP — Google Authenticator compatible)
- JWT authentication with session management
- First-run setup wizard — no default credentials

**Infrastructure**
- Go backend (Gin framework) + Next.js 15 frontend
- Go agent communicating via gRPC — the only component that touches the Docker socket and Nginx
- PostgreSQL + TimescaleDB for analytics
- Redis for sessions and caching
- 34 database migrations applied automatically at startup

---

## Enterprise Edition

InfraPilot EE adds multi-agent management, advanced analytics, deployment pipelines, secrets management, SSO/OIDC, audit logs, CVE scanning, and compliance reporting.

Contact **sales@infrapilot.org** for EE access or visit [infrapilot.org](https://infrapilot.org).
