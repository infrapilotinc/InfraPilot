# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Copy go mod files
COPY go.mod go.sum* ./
RUN go mod download

# Copy source
COPY . .

# Build binary with security flags
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -X main.version=${VERSION:-dev}" \
    -a -installsuffix cgo \
    -o agent ./cmd/agent

# Runtime stage - alpine for docker CLI compatibility
FROM alpine:3.20

WORKDIR /app

# Install runtime dependencies
# - ca-certificates: TLS connections
# - tzdata: timezone support
# - docker-cli: for docker exec to manage nginx
# - git + nixpacks: clone & build source for push-to-deploy (build from git)
ARG NIXPACKS_VERSION=1.41.0
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    docker-cli \
    git \
    curl \
    && curl -fsSL "https://github.com/railwayapp/nixpacks/releases/download/v${NIXPACKS_VERSION}/nixpacks-v${NIXPACKS_VERSION}-x86_64-unknown-linux-musl.tar.gz" -o /tmp/nixpacks.tar.gz \
    && tar -xzf /tmp/nixpacks.tar.gz -C /usr/local/bin nixpacks \
    && rm /tmp/nixpacks.tar.gz \
    && rm -rf /var/cache/apk/*

# Create non-root user with docker group access
RUN addgroup -g 1000 infrapilot && \
    adduser -D -u 1000 -G infrapilot infrapilot

# Copy binary from builder
COPY --from=builder /app/agent .

# Set ownership
RUN chown -R infrapilot:infrapilot /app

# Note: Running as root is required for Docker socket access
# The socket is mounted read-only for security
USER root

EXPOSE 9091

ENTRYPOINT ["./agent"]
