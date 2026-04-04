# InfraPilot CE — Dev & Production Setup

> For detailed development docs see [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md).

---

## Development

```bash
cd /home/administrator/infrapilot/infrapilot-ce
docker compose -f docker-compose.dev.yml up --build -d
```

Containers started:
- `infrapilot-dev-postgres` — PostgreSQL + TimescaleDB
- `infrapilot-dev-redis` — Redis
- `infrapilot-dev-backend` — Go API with Air hot-reload
- `infrapilot-dev-frontend` — Next.js with hot-reload
- `infrapilot-dev-agent` — Agent (static ID `00000000-…-0001`)
- `infrapilot-nginx` — Nginx

```bash
# Stop
docker compose -f docker-compose.dev.yml down

# Stop + wipe volumes
docker compose -f docker-compose.dev.yml down -v
```

---

## Building & Publishing

### All-in-one image (recommended)

```bash
cd /home/administrator/infrapilot/infrapilot-ce

VERSION=1.0.0

docker build \
  --build-arg VERSION=$VERSION \
  --build-arg BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
  -t ghcr.io/tybali/infrapilot-ce:$VERSION \
  -t ghcr.io/tybali/infrapilot-ce:latest \
  .

docker push ghcr.io/tybali/infrapilot-ce:$VERSION
docker push ghcr.io/tybali/infrapilot-ce:latest
```

### Multi-container images (docker-compose.prod.yml)

```bash
VERSION=1.0.0

# Build
docker compose -f docker-compose.prod.yml build

# Tag & push each
for svc in backend frontend agent; do
  docker tag infrapilot-ce-$svc ghcr.io/tybali/infrapilot-ce-$svc:$VERSION
  docker tag infrapilot-ce-$svc ghcr.io/tybali/infrapilot-ce-$svc:latest
  docker push ghcr.io/tybali/infrapilot-ce-$svc:$VERSION
  docker push ghcr.io/tybali/infrapilot-ce-$svc:latest
done
```

---

## Production Deployment

### Using all-in-one image

```bash
cd /home/administrator/dx-core-ops/infra

docker compose --env-file .env -f docker-compose.infrapilot.yml pull
docker compose --env-file .env -f docker-compose.infrapilot.yml up -d --no-deps --force-recreate infrapilot
```

### Using multi-container stack

```bash
docker compose -f docker-compose.prod.yml pull
docker compose -f docker-compose.prod.yml up -d
```

---

## Image Names

| Component | Image |
|-----------|-------|
| All-in-one | `ghcr.io/tybali/infrapilot-ce` |
| Backend | `ghcr.io/tybali/infrapilot-ce-backend` |
| Frontend | `ghcr.io/tybali/infrapilot-ce-frontend` |
| Agent | `ghcr.io/tybali/infrapilot-ce-agent` |
