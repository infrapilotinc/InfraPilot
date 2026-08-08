# =============================================================
# InfraPilot Agent - Host Controller
# =============================================================

FROM golang:1.23-alpine AS builder

WORKDIR /build

# Obfuscation toggle. GARBLE=1 → build with garble (-literals); GARBLE=0 → plain `go build`.
# Default 0: garble recompiles the whole dependency tree, which is brutally slow under
# cross-arch (arm64) emulation. Turn it on for release builds: `--garble` (--build-arg GARBLE=1).
ARG GARBLE=0

RUN apk add --no-cache git ca-certificates

# Install garble only when obfuscation is enabled. Pin the version — garble@latest can require a
# newer Go than this builder (v0.15.0+ need Go >= 1.25); v0.14.2 works with this Go 1.23 builder.
RUN if [ "$GARBLE" = "1" ]; then go install mvdan.cc/garble@v0.14.2; fi

# Copy go modules
COPY go.mod go.sum* ./
RUN go mod download

# Copy source
COPY . .

# Build the agent. TARGETARCH is auto-provided by buildx so the binary matches the image
# platform (garble respects GOARCH like `go build`). GARBLE=1 obfuscates; 0 (default) is faster.
ARG TARGETARCH
RUN if [ "$GARBLE" = "1" ]; then \
        echo "==> building agent WITH garble obfuscation" && \
        CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} garble -literals -seed=random build \
            -ldflags="-w -s" -o /agent ./cmd/agent; \
    else \
        echo "==> building agent WITHOUT garble (plain go build)" && \
        CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build \
            -ldflags="-w -s" -o /agent ./cmd/agent; \
    fi

# -------------------------------------------------------------
# Runtime Stage
# -------------------------------------------------------------
FROM alpine:3.21

# OCI Labels
LABEL org.opencontainers.image.title="InfraPilot Agent"
LABEL org.opencontainers.image.description="InfraPilot Host Agent for Docker and Nginx management"
LABEL org.opencontainers.image.source="https://github.com/infrapilothq/InfraPilot"
LABEL org.opencontainers.image.licenses="Apache-2.0"
LABEL org.opencontainers.image.vendor="InfraPilot"

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    docker-cli \
    curl \
    && rm -rf /var/cache/apk/*

# Create app user
RUN adduser -D -H -s /sbin/nologin appuser

WORKDIR /app

# Copy binary
COPY --from=builder /agent /app/agent

# Note: Agent needs elevated permissions to manage Docker and Nginx
# It runs as root but follows least-privilege principles internally
# Set ownership (agent will still run as root due to docker socket access)
RUN chown -R appuser:appuser /app

# Environment
ENV ENV=production
ENV DATA_DIR=/var/lib/infrapilot-agent
ENV LOG_PERSISTENCE=true

# Create data directory
RUN mkdir -p /var/lib/infrapilot-agent && \
    chown appuser:appuser /var/lib/infrapilot-agent

# Note: This container typically needs to run as root to access:
# - Docker socket
# - Nginx configuration files
# For security, configure Docker socket permissions on the host

CMD ["/app/agent"]
