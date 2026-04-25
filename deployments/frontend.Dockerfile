# =============================================================
# InfraPilot Frontend - Next.js Dashboard
# =============================================================

FROM node:22-alpine AS builder

WORKDIR /build

# Enable pnpm
RUN corepack enable && corepack prepare pnpm@latest --activate

# Copy package files
COPY package.json pnpm-lock.yaml* ./
RUN pnpm install --frozen-lockfile

# Copy source
COPY . .

# Ensure public directory exists
RUN mkdir -p ./public

# Build Next.js
ENV NEXT_TELEMETRY_DISABLED=1
RUN pnpm build

# -------------------------------------------------------------
# Runtime Stage
# -------------------------------------------------------------
FROM node:22-alpine

# OCI Labels
LABEL org.opencontainers.image.title="InfraPilot Frontend"
LABEL org.opencontainers.image.description="InfraPilot Web Dashboard"
LABEL org.opencontainers.image.source="https://github.com/infrapilothq/InfraPilot"
LABEL org.opencontainers.image.licenses="Apache-2.0"
LABEL org.opencontainers.image.vendor="InfraPilot"

# Create app user
RUN adduser -D -H -s /sbin/nologin appuser

WORKDIR /app

# Copy built application
COPY --from=builder --chown=appuser:appuser /build/.next/standalone ./
COPY --from=builder --chown=appuser:appuser /build/.next/static ./.next/static
COPY --from=builder --chown=appuser:appuser /build/public ./public

USER appuser

# Environment
ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
ENV PORT=3000
ENV HOSTNAME=0.0.0.0

# Expose port
EXPOSE 3000

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:3000/ || exit 1

CMD ["node", "server.js"]
