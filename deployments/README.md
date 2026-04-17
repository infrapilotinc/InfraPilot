# InfraPilot Production Deployment

This directory contains production-ready deployment configurations for InfraPilot with separate container services.

## Architecture

InfraPilot uses a multi-container architecture in production:

```
┌─────────────────────────────────────────────────────────┐
│                     InfraPilot Stack                     │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐             │
│  │  Nginx   │  │ Frontend │  │ Backend  │             │
│  │ (proxy)  │◄─┤  (Next)  │◄─┤  (API)   │             │
│  └──────────┘  └──────────┘  └──────────┘             │
│       ▲                            │                     │
│       │        ┌──────────┐        │                     │
│       └────────┤  Agent   │────────┘                     │
│                └──────────┘                              │
│                     │                                    │
│       ┌─────────────┼─────────────┐                     │
│       │                            │                     │
│  ┌──────────┐              ┌──────────┐                │
│  │Postgres  │              │  Redis   │                │
│  │   (DB)   │              │ (Cache)  │                │
│  └──────────┘              └──────────┘                │
│                                                          │
└─────────────────────────────────────────────────────────┘
```

### Services

| Service | Image | Purpose |
|---------|-------|---------|
| **postgres** | `postgres:16-alpine` | Database |
| **redis** | `redis:7-alpine` | Cache & sessions |
| **backend** | `infrapilothq/infrapilot-ce-backend` | API server |
| **frontend** | `infrapilothq/infrapilot-ce-frontend` | Web dashboard |
| **nginx** | `nginx:stable-alpine` | Reverse proxy (managed by agent) |
| **agent** | `infrapilothq/infrapilot-ce-agent` | Docker & Nginx controller |

## Quick Start

### 1. Configure Environment

```bash
# Copy production environment template
cp .env.prod.example .env

# Edit with your values
nano .env
```

**Required Configuration:**
```bash
# Generate secure secrets
JWT_SECRET=$(openssl rand -base64 32)
POSTGRES_PASSWORD=$(openssl rand -base64 32)
REDIS_PASSWORD=$(openssl rand -base64 32)

# SSL/TLS
LETSENCRYPT_EMAIL=admin@yourdomain.com
LETSENCRYPT_STAGING=false  # Set to false for production certs

# CORS
ALLOWED_ORIGINS=https://yourdomain.com
```

### 2. Deploy

```bash
# Using docker compose
docker compose -f docker-compose.prod.yml up -d

# Check status
docker compose -f docker-compose.prod.yml ps

# View logs
docker compose -f docker-compose.prod.yml logs -f
```

### 3. Access Dashboard

Open http://localhost (or your domain) in your browser.

On first access, you'll be prompted to create an admin account.

## Building Images

### Build All Images

```bash
./scripts/build-and-publish.sh v1.0.0
```

### Build Individual Components

```bash
# Backend only
./scripts/build-and-publish.sh v1.0.0 --backend

# Frontend only
./scripts/build-and-publish.sh v1.0.0 --frontend

# Agent only
./scripts/build-and-publish.sh v1.0.0 --agent

# Legacy all-in-one image
./scripts/build-and-publish.sh v1.0.0 --all-in-one
```

### Build Without Pushing

```bash
# Build locally without pushing to registry
./scripts/build-and-publish.sh latest --no-push
```

### Multi-Platform Builds

```bash
# Build for multiple architectures
./scripts/build-and-publish.sh v1.0.0 --platform linux/amd64,linux/arm64
```

## Component Details

### Backend (API Server)

**Dockerfile:** `deployments/backend.Dockerfile`
**Context:** `./backend`
**Ports:** 8080 (HTTP), 9090 (gRPC)

- Built from `golang:1.24-alpine`
- Runs database migrations on startup
- Handles REST API and gRPC agent communication

### Frontend (Dashboard)

**Dockerfile:** `deployments/frontend.Dockerfile`
**Context:** `./frontend`
**Port:** 3000

- Built from `node:22-alpine` with Next.js
- Standalone mode for optimal performance
- Proxies API requests through Nginx

### Agent (Controller)

**Dockerfile:** `deployments/agent.Dockerfile`
**Context:** `./agent`
**Volumes:** Docker socket, Nginx configs, SSL certs

- Built from `golang:1.24-alpine`
- Manages Docker containers via Docker API
- Generates and reloads Nginx configurations
- Handles SSL certificate automation (Let's Encrypt)

## Deployment Scenarios

### Single-Server Deployment

The default configuration deploys everything on a single server:

```bash
docker compose -f docker-compose.prod.yml up -d
```

### Using External Database

To use an external PostgreSQL or Redis:

```bash
# .env
DATABASE_URL=postgres://user:pass@external-host:5432/infrapilot?sslmode=require
REDIS_URL=redis://:password@external-host:6379
```

Then remove postgres and redis services from docker-compose.prod.yml.

### High Availability Setup

For HA deployments:

1. **External managed databases:**
   - Use AWS RDS, Azure Database, or managed PostgreSQL
   - Use AWS ElastiCache, Redis Cloud, or managed Redis

2. **Multiple agents:**
   - Deploy agents on multiple hosts
   - Each agent manages its own Docker daemon
   - All agents connect to central backend

3. **Load balancing:**
   - Use external load balancer (AWS ALB, Cloudflare, etc.)
   - Point to Nginx on multiple hosts

## Volumes & Data Persistence

| Volume | Purpose | Backup |
|--------|---------|--------|
| `postgres_data` | Database | **Critical** |
| `redis_data` | Cache | Optional |
| `nginx_certs` | SSL certificates | **Critical** |
| `nginx_conf` | Nginx configs | Generated |
| `agent_data` | Agent state | Important |

### Backup Strategy

```bash
# Backup database
docker exec infrapilot-postgres pg_dump -U infrapilot infrapilot > backup.sql

# Backup SSL certificates
docker cp infrapilot-nginx:/etc/letsencrypt ./letsencrypt-backup

# Restore database
cat backup.sql | docker exec -i infrapilot-postgres psql -U infrapilot infrapilot
```

## Monitoring

### Health Checks

All services have built-in health checks:

```bash
# Check all services
docker compose -f docker-compose.prod.yml ps

# Backend health
curl http://localhost:8080/health

# Frontend health
curl http://localhost:3000/

# Nginx health
curl http://localhost:80/
```

### Logs

```bash
# All logs
docker compose -f docker-compose.prod.yml logs -f

# Specific service
docker compose -f docker-compose.prod.yml logs -f backend

# Last 100 lines
docker compose -f docker-compose.prod.yml logs --tail=100
```

## Upgrading

```bash
# Pull latest images
docker compose -f docker-compose.prod.yml pull

# Restart services
docker compose -f docker-compose.prod.yml up -d

# Verify
docker compose -f docker-compose.prod.yml ps
```

## Troubleshooting

### Backend can't connect to database

```bash
# Check postgres is running
docker compose -f docker-compose.prod.yml ps postgres

# Check logs
docker compose -f docker-compose.prod.yml logs postgres

# Verify DATABASE_URL in .env
```

### Agent can't manage Docker

```bash
# Verify Docker socket is mounted
docker inspect infrapilot-agent | grep docker.sock

# Check agent logs
docker compose -f docker-compose.prod.yml logs agent
```

### SSL certificates not working

```bash
# Check LETSENCRYPT_EMAIL is set
# Check domain DNS is pointing to server
# Verify port 80 and 443 are accessible

# Agent logs show certificate requests
docker compose -f docker-compose.prod.yml logs agent | grep -i acme
```

### Nginx showing default page

```bash
# Check if agent generated configs
docker exec infrapilot-nginx ls -la /etc/nginx/sites/

# Reload nginx
docker exec infrapilot-nginx nginx -s reload

# Check agent logs
docker compose -f docker-compose.prod.yml logs agent | grep nginx
```

## Security Best Practices

1. **Change default secrets** - Generate strong random values for all passwords
2. **Enable SSL** - Set `LETSENCRYPT_STAGING=false` for production certificates
3. **Restrict CORS** - Set `ALLOWED_ORIGINS` to your actual domains only
4. **Firewall** - Only expose ports 80 and 443 to the internet
5. **Regular updates** - Keep images updated with `docker compose pull`
6. **Backup database** - Automated daily backups of postgres_data
7. **Monitor logs** - Set up log aggregation for audit trails

## Support

- **Documentation:** https://github.com/tybali/infrapilot-ce
- **Issues:** https://github.com/tybali/infrapilot-ce/issues
- **Security:** security@infrapilot.org
