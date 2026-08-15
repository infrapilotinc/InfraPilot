# =============================================================
# InfraPilot Backend - API Server
# =============================================================

FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS builder

WORKDIR /build

RUN apk add --no-cache git ca-certificates tzdata

# Copy go modules
COPY go.mod go.sum* ./
RUN go mod download

# Copy source
COPY . .

# VERSION is injected by build-and-publish.sh via --build-arg
ARG VERSION=dev
# TARGETARCH is auto-provided by buildx so the binary matches the image platform.
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /backend ./cmd/server

# -------------------------------------------------------------
# Runtime Stage
# -------------------------------------------------------------
FROM alpine:3.21

# OCI Labels
ARG VERSION=dev
LABEL org.opencontainers.image.title="InfraPilot Backend"
LABEL org.opencontainers.image.description="InfraPilot API Server"
LABEL org.opencontainers.image.source="https://github.com/infrapilot-sh/infrapilot"
LABEL org.opencontainers.image.licenses="AGPL-3.0"
LABEL org.opencontainers.image.vendor="InfraPilot"
LABEL org.opencontainers.image.version="${VERSION}"

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    curl \
    bash \
    wget \
    && rm -rf /var/cache/apk/*

# Install Trivy for vulnerability scanning with retry logic
RUN wget -q -O /tmp/install-trivy.sh https://raw.githubusercontent.com/aquasecurity/trivy/main/contrib/install.sh && \
    chmod +x /tmp/install-trivy.sh && \
    (/tmp/install-trivy.sh -b /usr/local/bin || (sleep 5 && /tmp/install-trivy.sh -b /usr/local/bin)) && \
    rm /tmp/install-trivy.sh && \
    trivy --version

# Install Syft for SBOM generation with retry logic
RUN wget -q -O /tmp/install-syft.sh https://raw.githubusercontent.com/anchore/syft/main/install.sh && \
    chmod +x /tmp/install-syft.sh && \
    (/tmp/install-syft.sh -b /usr/local/bin || (sleep 5 && /tmp/install-syft.sh -b /usr/local/bin)) && \
    rm /tmp/install-syft.sh && \
    syft version

# Install OPA (Open Policy Agent) for policy evaluation — arch-matched via TARGETARCH
ARG TARGETARCH
RUN wget -q -O /usr/local/bin/opa "https://openpolicyagent.org/downloads/latest/opa_linux_${TARGETARCH}_static" && \
    chmod +x /usr/local/bin/opa && \
    opa version

# Create app user and persistent data directory
RUN adduser -D -H -s /sbin/nologin appuser && \
    mkdir -p /data && chown appuser:appuser /data

WORKDIR /app

# Copy binary and migrations
COPY --from=builder /backend /app/server
COPY internal/db/migrations /app/migrations

# Set ownership
RUN chown -R appuser:appuser /app

USER appuser

# Persistent volume for instance ID, caches, etc.
VOLUME ["/data"]

# Environment
ENV ENV=production
ENV HTTP_PORT=8080
ENV GRPC_PORT=9090
ENV DATA_DIR=/data

# Expose ports
EXPOSE 8080 9090

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

CMD ["/app/server"]
