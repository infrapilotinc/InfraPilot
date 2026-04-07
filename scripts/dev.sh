#!/bin/bash
set -e

# InfraPilot Development Script

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Use docker compose (v2) or docker-compose (v1)
if docker compose version &> /dev/null; then
  COMPOSE="docker compose"
else
  COMPOSE="docker-compose"
fi

case "$1" in
  "up")
    echo "Starting full dev stack with hot reload..."
    cd "$PROJECT_ROOT"
    $COMPOSE -f docker-compose.dev.yml up -d
    echo ""
    echo "Services starting (internal Docker network only)..."
    echo "No ports exposed to host."
    echo ""
    echo "For local dev with host access, use: ./scripts/dev.sh dev"
    echo "View logs: ./scripts/dev.sh logs"
    ;;

  "up:db")
    echo "Starting database services only..."
    cd "$PROJECT_ROOT"
    $COMPOSE -f docker-compose.dev.yml up -d postgres redis
    echo "Waiting for services..."
    sleep 3
    echo "Database and Redis are ready!"
    echo ""
    echo "To start the backend: cd backend && go run ./cmd/server"
    echo "To start the frontend: cd frontend && pnpm dev"
    ;;

  "down")
    echo "Stopping development environment..."
    cd "$PROJECT_ROOT"
    $COMPOSE -f docker-compose.dev.yml down
    ;;

  "reset")
    echo "Resetting database..."
    cd "$PROJECT_ROOT"
    $COMPOSE -f docker-compose.dev.yml down -v
    $COMPOSE -f docker-compose.dev.yml up -d postgres redis
    echo "Database reset complete!"
    ;;

  "logs")
    cd "$PROJECT_ROOT"
    $COMPOSE -f docker-compose.dev.yml logs -f "${@:2}"
    ;;

  "proto")
    echo "Generating protobuf code..."
    cd "$PROJECT_ROOT"
    protoc --go_out=backend --go-grpc_out=backend \
           --go_opt=paths=source_relative \
           --go-grpc_opt=paths=source_relative \
           proto/agent/v1/agent.proto
    echo "Protobuf generation complete!"
    ;;

  "migrate")
    echo "Running migrations..."
    cd "$PROJECT_ROOT"
    $COMPOSE -f docker-compose.dev.yml exec postgres psql -U infrapilot -d infrapilot -f /docker-entrypoint-initdb.d/001_initial_schema.sql
    ;;

  "seed")
    echo "Seeding database..."
    cd "$PROJECT_ROOT"
    $COMPOSE -f docker-compose.dev.yml exec -T postgres psql -U infrapilot -d infrapilot < scripts/seed.sql
    echo ""
    echo "Seed data applied. Create admin account via setup flow at http://localhost"
    ;;

  "air")
    echo "Installing Air for hot reload..."
    go install github.com/air-verse/air@latest
    echo "Air installed! Run 'cd backend && air' to start with hot reload"
    ;;

  "storybook")
    echo "Starting Storybook..."
    cd "$PROJECT_ROOT/frontend"
    pnpm storybook
    ;;

  "dev")
    echo "Starting full local development environment..."
    echo ""
    echo "Starting Docker services (DB, Redis only)..."
    cd "$PROJECT_ROOT"
    $COMPOSE -f docker-compose.dev.yml up -d postgres redis
    echo "Waiting for services..."
    sleep 3

    echo ""
    echo "Starting local services in background..."
    echo ""

    # Create a temporary directory for log files
    mkdir -p "$PROJECT_ROOT/.dev-logs"

    # Start backend
    echo "  [1/3] Starting backend (Go)..."
    cd "$PROJECT_ROOT/backend"
    go run ./cmd/server > "$PROJECT_ROOT/.dev-logs/backend.log" 2>&1 &
    BACKEND_PID=$!
    echo "        Backend PID: $BACKEND_PID"

    # Start frontend
    echo "  [2/4] Starting frontend (Next.js)..."
    cd "$PROJECT_ROOT/frontend"
    pnpm dev > "$PROJECT_ROOT/.dev-logs/frontend.log" 2>&1 &
    FRONTEND_PID=$!
    echo "        Frontend PID: $FRONTEND_PID"

    # Start Storybook
    echo "  [3/4] Starting Storybook..."
    pnpm storybook > "$PROJECT_ROOT/.dev-logs/storybook.log" 2>&1 &
    STORYBOOK_PID=$!
    echo "        Storybook PID: $STORYBOOK_PID"

    # Start Agent
    echo "  [4/4] Starting Agent..."
    cd "$PROJECT_ROOT/agent"
    AGENT_ID=00000000-0000-0000-0000-000000000001 \
    BACKEND_GRPC_ADDR=localhost:9090 \
    BACKEND_HTTP_URL=http://localhost:8080 \
    go run ./cmd/agent > "$PROJECT_ROOT/.dev-logs/agent.log" 2>&1 &
    AGENT_PID=$!
    echo "        Agent PID: $AGENT_PID"

    # Save PIDs for cleanup
    echo "$BACKEND_PID" > "$PROJECT_ROOT/.dev-logs/backend.pid"
    echo "$FRONTEND_PID" > "$PROJECT_ROOT/.dev-logs/frontend.pid"
    echo "$STORYBOOK_PID" > "$PROJECT_ROOT/.dev-logs/storybook.pid"
    echo "$AGENT_PID" > "$PROJECT_ROOT/.dev-logs/agent.pid"

    echo ""
    echo "✅ Development environment started!"
    echo ""
    echo "Services (Direct Access - No Nginx):"
    echo "  📊 Dashboard:   http://localhost:3000"
    echo "  🔌 API:         http://localhost:8080/api/v1"
    echo "  📖 Storybook:   http://localhost:6006"
    echo ""
    echo "Database:"
    echo "  PostgreSQL:  localhost:5432 (infrapilot/infrapilot)"
    echo "  Redis:       localhost:6379"
    echo ""
    echo "Logs:"
    echo "  Backend:    tail -f $PROJECT_ROOT/.dev-logs/backend.log"
    echo "  Frontend:   tail -f $PROJECT_ROOT/.dev-logs/frontend.log"
    echo "  Storybook:  tail -f $PROJECT_ROOT/.dev-logs/storybook.log"
    echo "  Agent:      tail -f $PROJECT_ROOT/.dev-logs/agent.log"
    echo ""
    echo "To stop all services: ./scripts/dev.sh dev:stop"
    echo ""
    ;;

  "dev:stop")
    echo "Stopping local development environment..."
    cd "$PROJECT_ROOT"

    if [ -f ".dev-logs/backend.pid" ]; then
      BACKEND_PID=$(cat .dev-logs/backend.pid)
      echo "  Stopping backend (PID: $BACKEND_PID)..."
      kill $BACKEND_PID 2>/dev/null || true
      rm -f .dev-logs/backend.pid
    fi

    if [ -f ".dev-logs/frontend.pid" ]; then
      FRONTEND_PID=$(cat .dev-logs/frontend.pid)
      echo "  Stopping frontend (PID: $FRONTEND_PID)..."
      kill $FRONTEND_PID 2>/dev/null || true
      rm -f .dev-logs/frontend.pid
    fi

    if [ -f ".dev-logs/storybook.pid" ]; then
      STORYBOOK_PID=$(cat .dev-logs/storybook.pid)
      echo "  Stopping Storybook (PID: $STORYBOOK_PID)..."
      kill $STORYBOOK_PID 2>/dev/null || true
      rm -f .dev-logs/storybook.pid
    fi

    if [ -f ".dev-logs/agent.pid" ]; then
      AGENT_PID=$(cat .dev-logs/agent.pid)
      echo "  Stopping Agent (PID: $AGENT_PID)..."
      kill $AGENT_PID 2>/dev/null || true
      rm -f .dev-logs/agent.pid
    fi

    echo "  Stopping database services..."
    $COMPOSE -f docker-compose.dev.yml down

    echo ""
    echo "✅ All services stopped"
    ;;

  "dev:logs")
    echo "Development Logs"
    echo ""
    echo "Choose a service to view logs:"
    echo "  1. Backend"
    echo "  2. Frontend"
    echo "  3. Storybook"
    echo "  4. Agent"
    echo "  5. All (combined)"
    echo ""
    read -p "Enter choice (1-5): " choice

    case $choice in
      1)
        tail -f "$PROJECT_ROOT/.dev-logs/backend.log"
        ;;
      2)
        tail -f "$PROJECT_ROOT/.dev-logs/frontend.log"
        ;;
      3)
        tail -f "$PROJECT_ROOT/.dev-logs/storybook.log"
        ;;
      4)
        tail -f "$PROJECT_ROOT/.dev-logs/agent.log"
        ;;
      5)
        tail -f "$PROJECT_ROOT/.dev-logs/"*.log
        ;;
      *)
        echo "Invalid choice"
        ;;
    esac
    ;;

  *)
    echo "InfraPilot Development Commands"
    echo ""
    echo "Usage: ./scripts/dev.sh <command>"
    echo ""
    echo "Commands:"
    echo "  up          Start full dev stack (Docker - no host ports)"
    echo "  up:db       Start only postgres and redis (for local development)"
    echo "  down        Stop all Docker services"
    echo "  reset       Reset database (destroys all data)"
    echo "  logs        View Docker service logs (e.g., logs backend)"
    echo ""
    echo "Local Development (Recommended):"
    echo "  dev         Start everything: DB + Backend + Frontend + Storybook"
    echo "  dev:stop    Stop all local development services"
    echo "  dev:logs    View logs for local services"
    echo "  storybook   Start only Storybook (standalone)"
    echo ""
    echo "Code Generation:"
    echo "  proto       Generate protobuf code"
    echo "  migrate     Run database migrations"
    echo "  seed        Apply seed data (org, agent - no users)"
    echo "  air         Install Air for hot reload"
    echo ""
    echo "Quick Start (Local Development - Recommended):"
    echo "  ./scripts/dev.sh dev"
    echo "  Open http://localhost:3000 (Dashboard)"
    echo "  Open http://localhost:6006 (Storybook)"
    echo "  (Runs locally with hot reload, DB in Docker)"
    echo ""
    echo "Quick Start (Docker - no host ports):"
    echo "  ./scripts/dev.sh up"
    echo "  (Services run in Docker network only, no host access)"
    ;;
esac
