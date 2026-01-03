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
    echo "Services starting..."
    echo "  Dashboard: http://localhost"
    echo "  API:       http://localhost/api/v1"
    echo ""
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
    echo "Test users created:"
    echo "  admin@infrapilot.local / admin123 (super_admin)"
    echo "  operator@infrapilot.local / admin123 (operator)"
    echo "  viewer@infrapilot.local / admin123 (viewer)"
    ;;

  "air")
    echo "Installing Air for hot reload..."
    go install github.com/air-verse/air@latest
    echo "Air installed! Run 'cd backend && air' to start with hot reload"
    ;;

  *)
    echo "InfraPilot Development Commands"
    echo ""
    echo "Usage: ./scripts/dev.sh <command>"
    echo ""
    echo "Commands:"
    echo "  up      Start full dev stack (all services with hot reload)"
    echo "  up:db   Start only postgres and redis (for local backend/frontend)"
    echo "  down    Stop all development services"
    echo "  reset   Reset database (destroys all data)"
    echo "  logs    View service logs (e.g., logs backend)"
    echo "  proto   Generate protobuf code"
    echo "  migrate Run database migrations"
    echo "  seed    Create test users and data"
    echo "  air     Install Air for hot reload"
    echo ""
    echo "Quick Start (Docker):"
    echo "  ./scripts/dev.sh up"
    echo "  Open http://localhost"
    echo ""
    echo "Quick Start (Local):"
    echo "  1. ./scripts/dev.sh up:db"
    echo "  2. cd backend && go run ./cmd/server"
    echo "  3. cd frontend && pnpm dev"
    ;;
esac
