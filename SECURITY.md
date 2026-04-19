# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 2.x.x   | :white_check_mark: |
| 1.x.x   | :white_check_mark: |
| < 1.0   | :x:                |

## Reporting a Vulnerability

We take security seriously at InfraPilot. If you discover a security vulnerability, please report it responsibly.

### How to Report

1. **DO NOT** open a public GitHub issue for security vulnerabilities
2. Email us at **security@infrapilot.org** with:
   - Description of the vulnerability
   - Steps to reproduce
   - Potential impact
   - Any suggested fixes (optional)

### What to Expect

- **Acknowledgment:** Within 48 hours
- **Initial Assessment:** Within 7 days
- **Resolution Timeline:** Depends on severity
  - Critical: 24–48 hours
  - High: 7 days
  - Medium: 30 days
  - Low: 90 days

### Security Best Practices

When deploying InfraPilot CE:

1. **Use strong secrets**
   - Generate a strong JWT secret: `openssl rand -base64 32`
   - Set a strong PostgreSQL password (`POSTGRES_PASSWORD`)
   - Set a strong Redis password (`REDIS_PASSWORD`)

2. **Use HTTPS in production**
   - Set `LETSENCRYPT_EMAIL` and point your DNS at the server
   - Set `LETSENCRYPT_STAGING=false` for trusted certificates

3. **Protect the Docker socket**
   - The agent mounts `/var/run/docker.sock` — it needs write access to manage containers
   - Treat the InfraPilot agent container with the same trust as root on the host
   - Restrict who can deploy or configure InfraPilot using RBAC roles

4. **Network isolation**
   - Internal services communicate over an isolated Docker network
   - Only Nginx exposes ports 80 and 443 to the host
   - Use `ALLOWED_ORIGINS` to restrict CORS to your domain

5. **Keep updated**
   - Regularly pull the latest images: `docker compose pull && docker compose up -d`
   - Watch [GitHub Releases](https://github.com/infrapilothq/infrapilot-ce/releases) for security patches

### Security Features (CE)

| Feature | CE | Notes |
|---------|:--:|-------|
| No SSH access required | ✅ | All operations go through the Docker API |
| RBAC (admin / operator / viewer) | ✅ | Scope access per user role |
| MFA (TOTP) | ✅ | Google Authenticator compatible |
| JWT with refresh token rotation | ✅ | Short-lived access tokens |
| Security headers on proxy hosts | ✅ | HSTS, CSP, X-Frame-Options, etc. |
| IP allowlist / denylist per proxy | ✅ | Block or restrict access by IP/CIDR |
| Basic Auth per proxy host | ✅ | bcrypt-hashed credentials |
| Non-root container processes | ✅ | Backend and frontend run as non-root |
| Encrypted gRPC (TLS) | ✅ | Backend ↔ Agent communication |
| mTLS agent enrollment | ❌ EE only | Rust agent with ECDSA P-256 certificates |
| Audit log | ❌ EE only | Persistent audit trail of all user actions |
| CVE scanning | ❌ EE only | Continuous Trivy scanning of deployed images |
| Secrets management | ❌ EE only | AES-256-GCM encrypted secrets store |

## Acknowledgments

We appreciate the security research community and will acknowledge reporters in our release notes (with permission).
