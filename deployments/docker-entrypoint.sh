#!/bin/sh

# =============================================================
# InfraPilot Docker Entrypoint
# =============================================================

echo "================================================"
echo "  InfraPilot - Starting..."
echo "================================================"

# -------------------------------------------------------------
# Environment Defaults
# -------------------------------------------------------------
export ENV="${ENV:-production}"
export JWT_SECRET="${JWT_SECRET:?JWT_SECRET is required}"
export DATA_DIR="${DATA_DIR:-/data}"

# Database
export POSTGRES_USER="${POSTGRES_USER:-infrapilot}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-infrapilot}"
export POSTGRES_DB="${POSTGRES_DB:-infrapilot}"

# Check if using external or embedded database
if [ -n "$DATABASE_URL" ]; then
    echo "[*] Using external database"
    export EMBEDDED_DB=false
else
    echo "[*] Using embedded PostgreSQL"
    export EMBEDDED_DB=true
    export DATABASE_URL="postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@localhost:5432/${POSTGRES_DB}?sslmode=disable"
fi

# Redis
export REDIS_PASSWORD="${REDIS_PASSWORD:-infrapilot}"

if [ -n "$REDIS_URL" ] && [ "$REDIS_URL" != "redis://:${REDIS_PASSWORD}@localhost:6379" ]; then
    echo "[*] Using external Redis"
    export EMBEDDED_REDIS=false
else
    echo "[*] Using embedded Redis"
    export EMBEDDED_REDIS=true
    export REDIS_URL="redis://:${REDIS_PASSWORD}@localhost:6379"
fi

# Frontend
export ALLOWED_ORIGINS="${ALLOWED_ORIGINS:-http://localhost:3000,http://localhost:80}"

# SSL
export LETSENCRYPT_EMAIL="${LETSENCRYPT_EMAIL:-}"
export LETSENCRYPT_STAGING="${LETSENCRYPT_STAGING:-true}"

# -------------------------------------------------------------
# Initialize Data Directories
# -------------------------------------------------------------
echo "[*] Initializing data directories..."

mkdir -p \
    "$DATA_DIR/postgres" \
    "$DATA_DIR/redis" \
    "$DATA_DIR/nginx/conf.d" \
    "$DATA_DIR/nginx/logs" \
    "$DATA_DIR/nginx/logs/domains" \
    "$DATA_DIR/nginx/certs" \
    "$DATA_DIR/letsencrypt" \
    "$DATA_DIR/agent" \
    /var/log/supervisor \
    /run/postgresql \
    /var/www/acme-challenge/.well-known/acme-challenge \
    /var/www/html

# Set permissions
chmod 755 /var/log/supervisor
chmod 755 /run/postgresql
chmod 755 "$DATA_DIR/nginx/logs"
chmod 755 "$DATA_DIR/nginx/logs/domains"

# Symlink nginx logs directory for persistence
mkdir -p "$DATA_DIR/nginx/logs/domains"
if [ ! -L /var/log/nginx ]; then
    rm -rf /var/log/nginx
    ln -s "$DATA_DIR/nginx/logs" /var/log/nginx
    echo "[*] Nginx logs symlinked to $DATA_DIR/nginx/logs for persistence"
fi

# Symlink letsencrypt directory for persistence
if [ ! -L /etc/letsencrypt ]; then
    rm -rf /etc/letsencrypt
    ln -s "$DATA_DIR/letsencrypt" /etc/letsencrypt
fi

# -------------------------------------------------------------
# Initialize Embedded PostgreSQL (if enabled)
# -------------------------------------------------------------
if [ "$EMBEDDED_DB" = "true" ]; then
    # Ensure postgres user exists
    if ! id postgres >/dev/null 2>&1; then
        adduser -D -H -s /sbin/nologin postgres
    fi

    # Set ownership
    chown -R postgres:postgres "$DATA_DIR/postgres"
    chown postgres:postgres /run/postgresql

    if [ ! -f "$DATA_DIR/postgres/PG_VERSION" ]; then
        echo "[*] Initializing PostgreSQL database..."

        # Initialize database cluster
        su-exec postgres initdb -D "$DATA_DIR/postgres" --auth-local=trust --auth-host=md5

        # Configure PostgreSQL
        echo "host all all 0.0.0.0/0 md5" >> "$DATA_DIR/postgres/pg_hba.conf"
        sed -i "s/#listen_addresses = 'localhost'/listen_addresses = 'localhost'/" "$DATA_DIR/postgres/postgresql.conf"
        echo "unix_socket_directories = '/run/postgresql'" >> "$DATA_DIR/postgres/postgresql.conf"

        # Enable TimescaleDB extension for nginx log analytics
        # This allows time-series optimizations for access log storage
        if ls /usr/lib/postgresql17/timescaledb*.so >/dev/null 2>&1; then
            echo "[*] Enabling TimescaleDB extension..."
            echo "shared_preload_libraries = 'timescaledb'" >> "$DATA_DIR/postgres/postgresql.conf"
        fi

        # Start PostgreSQL temporarily
        echo "[*] Starting PostgreSQL for initial setup..."
        su-exec postgres pg_ctl -D "$DATA_DIR/postgres" -o "-k /run/postgresql" start -w -t 60

        # Create user and database
        echo "[*] Creating database and user..."
        su-exec postgres psql -h /run/postgresql -c "CREATE USER $POSTGRES_USER WITH PASSWORD '$POSTGRES_PASSWORD';" || true
        su-exec postgres psql -h /run/postgresql -c "CREATE DATABASE $POSTGRES_DB OWNER $POSTGRES_USER;" || true
        su-exec postgres psql -h /run/postgresql -c "GRANT ALL PRIVILEGES ON DATABASE $POSTGRES_DB TO $POSTGRES_USER;" || true

        # Note: Migrations are now handled by the backend service on startup
        # This ensures proper tracking in schema_migrations table
        echo "[*] Database initialized - migrations will run on backend startup"

        # Stop PostgreSQL (supervisor will start it)
        su-exec postgres pg_ctl -D "$DATA_DIR/postgres" stop -w -t 60

        echo "[*] PostgreSQL initialized successfully"
    else
        echo "[*] PostgreSQL data directory exists, skipping initialization"
    fi
fi

# -------------------------------------------------------------
# Initialize Redis Data Directory
# -------------------------------------------------------------
if [ "$EMBEDDED_REDIS" = "true" ]; then
    echo "[*] Preparing Redis data directory..."
    mkdir -p "$DATA_DIR/redis"
    chmod 755 "$DATA_DIR/redis"
fi

# -------------------------------------------------------------
# Configure Nginx Basic Auth
# -------------------------------------------------------------
export BASIC_AUTH_USER="${BASIC_AUTH_USER:-}"
export BASIC_AUTH_PASSWORD="${BASIC_AUTH_PASSWORD:-}"

if [ -n "$BASIC_AUTH_USER" ] && [ -n "$BASIC_AUTH_PASSWORD" ]; then
    echo "[*] Configuring Nginx Basic Auth..."
    htpasswd -cb /etc/nginx/.htpasswd "$BASIC_AUTH_USER" "$BASIC_AUTH_PASSWORD"
    chmod 644 /etc/nginx/.htpasswd
    export BASIC_AUTH_ENABLED=true
else
    echo "[*] Basic Auth disabled (set BASIC_AUTH_USER and BASIC_AUTH_PASSWORD to enable)"
    # Create empty htpasswd file so nginx config doesn't error
    touch /etc/nginx/.htpasswd
    export BASIC_AUTH_ENABLED=false
fi

# -------------------------------------------------------------
# Configure Nginx
# -------------------------------------------------------------
echo "[*] Configuring Nginx..."

# Ensure nginx directories exist
mkdir -p /run/nginx /var/lib/nginx/tmp

# Create nginx.conf
mkdir -p /etc/nginx/conf.d /etc/nginx/sites
cat > /etc/nginx/nginx.conf << 'NGINX_CONF'
user nginx;
worker_processes auto;
error_log /var/log/nginx/error.log warn;
pid /run/nginx/nginx.pid;

events {
    worker_connections 1024;
}

http {
    include /etc/nginx/mime.types;
    default_type application/octet-stream;

    # Standard combined log format
    log_format main '$remote_addr - $remote_user [$time_local] "$request" '
                    '$status $body_bytes_sent "$http_referer" '
                    '"$http_user_agent" "$http_x_forwarded_for"';

    # Extended log format for InfraPilot analytics (includes request_time and host)
    log_format infrapilot_analytics '$remote_addr - $remote_user [$time_local] "$request" '
                                    '$status $body_bytes_sent "$http_referer" '
                                    '"$http_user_agent" $request_time "$host" "$upstream_addr"';

    access_log /var/log/nginx/access.log infrapilot_analytics;

    sendfile on;
    tcp_nopush on;
    tcp_nodelay on;
    keepalive_timeout 65;
    types_hash_max_size 2048;

    gzip on;
    gzip_vary on;
    gzip_min_length 1024;
    gzip_proxied any;
    gzip_types text/plain text/css application/json application/javascript text/xml application/xml;

    # Main configuration (managed by InfraPilot agent)
    include /etc/nginx/conf.d/*.conf;
    # Domain-specific configurations
    include /etc/nginx/sites/*.conf;
    include /data/nginx/conf.d/*.conf;
}
NGINX_CONF

# Generate default.conf with optional basic auth
if [ "$BASIC_AUTH_ENABLED" = "true" ]; then
    BASIC_AUTH_BLOCK='
    # Basic Authentication
    auth_basic "InfraPilot";
    auth_basic_user_file /etc/nginx/.htpasswd;'
else
    BASIC_AUTH_BLOCK=""
fi

cat > /etc/nginx/conf.d/default.conf << NGINX_DEFAULT
# InfraPilot Base Nginx Configuration
# Auto-generated by docker-entrypoint.sh
# Note: /etc/nginx/sites/*.conf and /data/nginx/conf.d/*.conf are included from nginx.conf

upstream frontend {
    server 127.0.0.1:3000;
}

upstream backend {
    server 127.0.0.1:8080;
}

server {
    listen 80 default_server;
    listen [::]:80 default_server;
    server_name _ localhost;
$BASIC_AUTH_BLOCK

    # Backend health check (no auth for monitoring tools)
    location = /api/health {
        auth_basic off;
        proxy_pass http://backend/health;
    }

    # API routes (uses JWT auth, disable basic auth)
    location /api/ {
        auth_basic off;
        proxy_pass http://backend;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;

        # WebSocket support
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_read_timeout 86400;
        proxy_buffering off;
    }

    # Frontend with HMR support
    location / {
        proxy_pass http://frontend;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;

        # WebSocket for Next.js HMR
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection "upgrade";
    }

    # Next.js HMR WebSocket
    location /_next/webpack-hmr {
        proxy_pass http://frontend/_next/webpack-hmr;
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
NGINX_DEFAULT

# Create sites directory if it doesn't exist
mkdir -p /etc/nginx/sites

# -------------------------------------------------------------
# Print Configuration Summary
# -------------------------------------------------------------
echo ""
echo "================================================"
echo "  Configuration Summary"
echo "================================================"
echo "  Environment:     $ENV"
echo "  Embedded DB:     $EMBEDDED_DB"
echo "  Embedded Redis:  $EMBEDDED_REDIS"
echo "  Data Directory:  $DATA_DIR"
echo "  Let's Encrypt:   ${LETSENCRYPT_EMAIL:-disabled}"
echo "  Basic Auth:      ${BASIC_AUTH_ENABLED:-false}"
echo "================================================"
echo ""
echo "  Dashboard:  http://localhost:80"
echo "  API:        http://localhost:8080"
echo ""
echo "  No default account exists. First visit creates your admin login."
echo ""
echo "================================================"
echo ""

# -------------------------------------------------------------
# Start Services
# -------------------------------------------------------------
exec "$@"
