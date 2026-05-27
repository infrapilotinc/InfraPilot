# Deployment

## Building & publishing images

All images are built and pushed by
[`scripts/build-and-publish.sh`](../scripts/build-and-publish.sh).

```bash
# Build & push everything, tagged latest, to Docker Hub + GHCR
./scripts/build-and-publish.sh

# Tag a specific version
./scripts/build-and-publish.sh v1.2.3

# One component only, no push
./scripts/build-and-publish.sh --backend --no-push

# Multi-arch
./scripts/build-and-publish.sh latest --platform linux/amd64,linux/arm64
```

### Registry selection

| Env var | Default | Notes |
|---------|---------|-------|
| `DH_REGISTRY` | `infrapilothq/infrapilot-ce` | Docker Hub prefix |
| `GHCR_REGISTRY` | `ghcr.io/infrapilothq/infrapilot-ce` | GHCR prefix |
| `PUSH_DOCKERHUB` | `true` | Set to `false` to push to **GHCR only** (used by CI) |

CI authenticates to GHCR with the built-in `GITHUB_TOKEN` and has no Docker Hub
credentials, so it runs with `PUSH_DOCKERHUB=false`. Local runs keep the
original dual-registry behavior.

## Running

```bash
# Production compose stack
cp .env.prod.example .env.prod   # then edit
docker compose -f docker-compose.prod.yml up -d
```

The dashboard is served on `http://localhost:8080` by default.

For local development use `docker-compose.dev.yml` (see [DEVELOPMENT.md](DEVELOPMENT.md)).

## Releases via CI

Pushing a `v*` tag triggers the **Publish images** workflow, which builds and
pushes the versioned images to GHCR. See [CI-CD.md](CI-CD.md).
