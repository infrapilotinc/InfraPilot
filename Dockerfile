# =============================================================
# InfraPilot - All-in-One Docker Image
# =============================================================
# This image contains: Backend + Frontend + Agent + Nginx
#
# Usage:
#   docker run -d -p 80:80 -p 443:443 \
#     -v /var/run/docker.sock:/var/run/docker.sock \
#     -v infrapilot_data:/data \
#     -e JWT_SECRET=your-secret-key \
#     ghcr.io/tybali/infrapilot-ce
# =============================================================

# -------------------------------------------------------------
# Stage 1: Build Backend
# -------------------------------------------------------------
FROM golang:1.24-alpine AS backend-builder

WORKDIR /build

RUN apk add --no-cache git ca-certificates tzdata

COPY backend/go.mod backend/go.sum* ./backend/
RUN cd backend && go mod download

COPY backend/ ./backend/

ARG VERSION=dev
RUN cd backend && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-X main.version=${VERSION} -w -s" \
    -o /backend ./cmd/server

# -------------------------------------------------------------
# Stage 2: Build Agent
# -------------------------------------------------------------
FROM golang:1.24-alpine AS agent-builder

WORKDIR /build

RUN apk add --no-cache git ca-certificates

COPY agent/go.mod agent/go.sum* ./agent/
RUN cd agent && go mod download

COPY agent/ ./agent/

RUN cd agent && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -o /agent ./cmd/agent

# -------------------------------------------------------------
# Stage 3: Build Frontend
# -------------------------------------------------------------
FROM node:22-alpine AS frontend-builder

WORKDIR /build

RUN corepack enable && corepack prepare pnpm@latest --activate

COPY frontend/package.json frontend/pnpm-lock.yaml* ./
RUN pnpm install --frozen-lockfile

COPY frontend/ ./

# Ensure public directory exists
RUN mkdir -p ./public

ENV NEXT_TELEMETRY_DISABLED=1
RUN pnpm build

# -------------------------------------------------------------
# Stage 4: Production Runtime
# -------------------------------------------------------------
FROM alpine:3.21





# -------------------------------------------------------------
# OCI Image Metadata (Docker Hub / Registry visibility)
# -------------------------------------------------------------

# Human-readable name of the image
LABEL org.opencontainers.image.title="InfraPilot CE"
# Short description shown on Docker Hub search & repo page
LABEL org.opencontainers.image.description="Open-source community edition control plane for Docker, NGINX, and self-hosted infrastructure"
# Project homepage (can be same as repo or website)
LABEL org.opencontainers.image.url="https://infrapilot.org"
# Source code repository (VERY IMPORTANT)
LABEL org.opencontainers.image.source="https://github.com/tybali/infrapilot-ce"
# Documentation / README link (Docker Hub auto-links this)
LABEL org.opencontainers.image.documentation="https://github.com/tybali/infrapilot-ce#readme"
# License identifier (SPDX format)
LABEL org.opencontainers.image.licenses="Apache-2.0"
# Organization / vendor name
LABEL org.opencontainers.image.vendor="tybali"
# Author / maintainer (optional but professional)
LABEL org.opencontainers.image.authors="tybali"
# Image version (should match git tag or release)
LABEL org.opencontainers.image.version="1.0.0"
# Build creation time (auto-filled during build)
ARG BUILD_DATE
LABEL org.opencontainers.image.created=$BUILD_DATE
# Git commit SHA (optional but very useful)
ARG VCS_REF
LABEL org.opencontainers.image.revision=$VCS_REF






# Install runtime dependencies (including PostgreSQL 17 + TimescaleDB for nginx log analytics)
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    nginx \
    nodejs \
    npm \
    docker-cli \
    supervisor \
    postgresql17 \
    postgresql17-contrib \
    postgresql-timescaledb \
    redis \
    curl \
    wget \
    bash \
    su-exec \
    apache2-utils \
    && rm -rf /var/cache/apk/*

# Install Trivy for vulnerability scanning
RUN wget -q -O /tmp/install-trivy.sh https://raw.githubusercontent.com/aquasecurity/trivy/main/contrib/install.sh && \
    chmod +x /tmp/install-trivy.sh && \
    (/tmp/install-trivy.sh -b /usr/local/bin || (sleep 5 && /tmp/install-trivy.sh -b /usr/local/bin)) && \
    rm /tmp/install-trivy.sh && \
    trivy --version

# Install Syft for SBOM generation
RUN wget -q -O /tmp/install-syft.sh https://raw.githubusercontent.com/anchore/syft/main/install.sh && \
    chmod +x /tmp/install-syft.sh && \
    (/tmp/install-syft.sh -b /usr/local/bin || (sleep 5 && /tmp/install-syft.sh -b /usr/local/bin)) && \
    rm /tmp/install-syft.sh && \
    syft version

# Install OPA (Open Policy Agent) for policy evaluation
RUN wget -q -O /usr/local/bin/opa https://openpolicyagent.org/downloads/latest/opa_linux_amd64_static && \
    chmod +x /usr/local/bin/opa && \
    opa version

# Create directories
RUN mkdir -p \
    /app/backend \
    /app/frontend \
    /app/agent \
    /data/postgres \
    /data/redis \
    /data/nginx/conf.d \
    /data/nginx/logs \
    /data/nginx/certs \
    /data/letsencrypt \
    /var/log/supervisor \
    /run/nginx

# Copy backend
COPY --from=backend-builder /backend /app/backend/server
COPY backend/internal/db/migrations /app/backend/migrations

# Copy agent
COPY --from=agent-builder /agent /app/agent/agent

# Copy frontend
COPY --from=frontend-builder /build/.next/standalone /app/frontend
COPY --from=frontend-builder /build/.next/static /app/frontend/.next/static
COPY --from=frontend-builder /build/public /app/frontend/public

# Copy supervisor config
COPY deployments/supervisor/supervisord.conf /etc/supervisor/conf.d/supervisord.conf

# Copy entrypoint script
COPY deployments/docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh

# Environment defaults
ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
ENV DATA_DIR=/data
ENV NGINX_CONFIG_PATH=/data/nginx/conf.d
ENV NGINX_CONTAINER_NAME=local
ENV PROXY_MODE=managed

# Expose ports
EXPOSE 80 443 3000 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

# Volumes for persistence
VOLUME ["/data", "/var/run/docker.sock"]

ENTRYPOINT ["/docker-entrypoint.sh"]
CMD ["supervisord", "-c", "/etc/supervisor/conf.d/supervisord.conf"]
