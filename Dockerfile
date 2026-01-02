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
#     devsimplex/infrapilot
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

RUN cd backend && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -a -installsuffix cgo \
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
    -a -installsuffix cgo \
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
FROM alpine:3.20





# -------------------------------------------------------------
# OCI Image Metadata (Docker Hub / Registry visibility)
# -------------------------------------------------------------

# Human-readable name of the image
LABEL org.opencontainers.image.title="InfraPilot"
# Short description shown on Docker Hub search & repo page
LABEL org.opencontainers.image.description="Open-source control plane for Docker, NGINX, and self-hosted infrastructure"
# Project homepage (can be same as repo or website)
LABEL org.opencontainers.image.url="https://infrapilot.org"
# Source code repository (VERY IMPORTANT)
LABEL org.opencontainers.image.source="https://github.com/devsimplex-org/infrapilot"
# Documentation / README link (Docker Hub auto-links this)
LABEL org.opencontainers.image.documentation="https://github.com/devsimplex-org/infrapilot#readme"
# License identifier (SPDX format)
LABEL org.opencontainers.image.licenses="Apache-2.0"
# Organization / vendor name
LABEL org.opencontainers.image.vendor="DevSimplex"
# Author / maintainer (optional but professional)
LABEL org.opencontainers.image.authors="DevSimplex <hello@devsimplex.com>"
# Image version (should match git tag or release)
LABEL org.opencontainers.image.version="0.1.0"
# Build creation time (auto-filled during build)
ARG BUILD_DATE
LABEL org.opencontainers.image.created=$BUILD_DATE
# Git commit SHA (optional but very useful)
ARG VCS_REF
LABEL org.opencontainers.image.revision=$VCS_REF






# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    nginx \
    nodejs \
    npm \
    docker-cli \
    supervisor \
    postgresql16 \
    postgresql16-contrib \
    redis \
    curl \
    su-exec \
    && rm -rf /var/cache/apk/*

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

# Copy nginx config
COPY deployments/nginx/default.conf /etc/nginx/http.d/default.conf

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
