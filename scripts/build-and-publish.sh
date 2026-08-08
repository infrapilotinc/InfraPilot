#!/bin/bash
set -e

# =============================================================
# InfraPilot Docker Build & Publish Script
# =============================================================
#
# Builds and publishes all InfraPilot container images:
# - Backend API
# - Frontend Dashboard
# - Agent Controller
# - All-in-One (legacy/convenience image)
#
# Pushes to both Docker Hub and GitHub Container Registry.
#
# Usage:
#   ./scripts/build-and-publish.sh [VERSION] [OPTIONS]
#
# Arguments:
#   VERSION       Image version tag (default: latest)
#
# Options:
#   --backend     Build and push backend only
#   --frontend    Build and push frontend only
#   --agent       Build and push agent only
#   --all-in-one  Build and push legacy all-in-one image only
#   --no-push     Build images but don't push to registry
#   --no-cache    Build without Docker layer cache
#   --platform    Target platform(s) (e.g., linux/amd64,linux/arm64)
#   --amd64       Shortcut: build amd64 only (fast — no arm64 emulation)
#   --arm64       Shortcut: build arm64 only
#   --docker-only Push to Docker Hub only (skip GHCR — what CE installs pull)
#   --ghcr-only   Push to GHCR only (skip Docker Hub)
#   (default is multi-arch + both registries; PLATFORMS/PUSH_GHCR/PUSH_DOCKERHUB env also work)
#
# Examples:
#   ./scripts/build-and-publish.sh v1.2.3                       # multi-arch release
#   ./scripts/build-and-publish.sh v1.2.3-staging --amd64       # fast single-arch staging
#   ./scripts/build-and-publish.sh --backend --no-push
#   ./scripts/build-and-publish.sh latest --platform linux/amd64,linux/arm64
#
# =============================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Load registry credentials from .developer.env if present, so pushes authenticate
# without a manual `docker login`. This file holds the dev team's push tokens and
# is gitignored — kept separate from the app's .env. See .developer.env.example.
#   DOCKERHUB_USERNAME, DOCKERHUB_TOKEN   — Docker Hub access token
#   GHCR_USERNAME,      GHCR_TOKEN        — GitHub PAT with write:packages
# Override the file location with ENV_FILE=/path/to/file.
ENV_FILE="${ENV_FILE:-$PROJECT_ROOT/.developer.env}"
if [ -f "$ENV_FILE" ]; then
    set -a
    # shellcheck disable=SC1090
    . "$ENV_FILE"
    set +a
fi

# Registries — images are pushed to both Docker Hub and GHCR.
# Override with env vars for private registries.
DH_PREFIX="${DH_REGISTRY:-infrapilothq/infrapilot-ce}"
GHCR_PREFIX="${GHCR_REGISTRY:-ghcr.io/infrapilothq/infrapilot-ce}"

# Set PUSH_DOCKERHUB=false to push to GHCR only (used by CI, which authenticates
# to GHCR via GITHUB_TOKEN and has no Docker Hub credentials). Defaults to true
# to preserve the original dual-registry behavior for local runs.
PUSH_DOCKERHUB="${PUSH_DOCKERHUB:-true}"
# Set PUSH_GHCR=false (or pass --docker-only) to skip GHCR entirely — handy when the
# GHCR token is unavailable/denied, since the CE install pulls from Docker Hub anyway.
PUSH_GHCR="${PUSH_GHCR:-true}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Parse arguments
VERSION="${1:-latest}"
BUILD_BACKEND=true
BUILD_FRONTEND=true
BUILD_AGENT=true
BUILD_ALL_IN_ONE=true
PUSH=true
# Multi-arch by default so ARM hosts (Graviton, Ampere, Apple-Silicon Linux, Pi)
# can pull. Override with --platform / --amd64 / --arm64, or the PLATFORMS env var.
# Requires buildx + QEMU (set up automatically).
PLATFORMS="${PLATFORMS:-linux/amd64,linux/arm64}"
NO_CACHE=""

# Process options
shift || true
while [[ $# -gt 0 ]]; do
    case $1 in
        --no-cache)
            NO_CACHE="--no-cache"
            shift
            ;;
        --backend)
            BUILD_BACKEND=true
            BUILD_FRONTEND=false
            BUILD_AGENT=false
            BUILD_ALL_IN_ONE=false
            shift
            ;;
        --frontend)
            BUILD_BACKEND=false
            BUILD_FRONTEND=true
            BUILD_AGENT=false
            BUILD_ALL_IN_ONE=false
            shift
            ;;
        --agent)
            BUILD_BACKEND=false
            BUILD_FRONTEND=false
            BUILD_AGENT=true
            BUILD_ALL_IN_ONE=false
            shift
            ;;
        --all-in-one)
            BUILD_BACKEND=false
            BUILD_FRONTEND=false
            BUILD_AGENT=false
            BUILD_ALL_IN_ONE=true
            shift
            ;;
        --docker-only)
            PUSH_GHCR=false
            PUSH_DOCKERHUB=true
            shift
            ;;
        --ghcr-only)
            PUSH_DOCKERHUB=false
            PUSH_GHCR=true
            shift
            ;;
        --no-push)
            PUSH=false
            shift
            ;;
        --amd64)
            PLATFORMS="linux/amd64"
            shift
            ;;
        --arm64)
            PLATFORMS="linux/arm64"
            shift
            ;;
        --platform)
            PLATFORMS="$2"
            shift 2
            ;;
        *)
            echo -e "${RED}Unknown option: $1${NC}"
            exit 1
            ;;
    esac
done

cd "$PROJECT_ROOT"

echo -e "${GREEN}================================================${NC}"
echo -e "${GREEN}  InfraPilot Docker Build & Publish${NC}"
echo -e "${GREEN}================================================${NC}"
echo ""
echo -e "  Version:  ${BLUE}$VERSION${NC}"
echo -e "  Push:     ${BLUE}$PUSH${NC}"
echo -e "  Platform: ${BLUE}${PLATFORMS}${NC}"
echo -e "  Docker Hub: ${BLUE}${DH_PREFIX}${NC}"
echo -e "  GHCR:       ${BLUE}${GHCR_PREFIX}${NC}"
echo ""

# Ensure a buildx builder with the docker-container driver exists (required for
# multi-arch) and register QEMU so one host arch can cross-build the other.
ensure_builder() {
    if ! docker buildx inspect infrapilot-builder >/dev/null 2>&1; then
        echo -e "${BLUE}→ Creating buildx builder 'infrapilot-builder'...${NC}"
        docker buildx create --name infrapilot-builder --driver docker-container --use >/dev/null
    else
        docker buildx use infrapilot-builder
    fi
    # Idempotent; installs binfmt handlers for cross-arch emulation.
    docker run --privileged --rm tonistiigi/binfmt --install all >/dev/null 2>&1 || true
}

# Build one image for all target platforms and push a multi-arch manifest to both
# registries in a SINGLE invocation. Plain `docker build` + `docker push` can only
# emit a single-arch image — which is exactly why arm64 pulls failed. With --no-push
# there is no local multi-arch image, so we build the host arch and --load it.
#   $1 name suffix ("" | "-backend" | ...)   $2 dockerfile   $3 context   $4.. extra build args
buildx_image() {
    local name="$1" dockerfile="$2" context="$3"; shift 3
    local dh="${DH_PREFIX}${name}" ghcr="${GHCR_PREFIX}${name}"

    local tags=()
    [ "$PUSH_DOCKERHUB" = true ] && tags+=(-t "${dh}:${VERSION}")
    [ "$PUSH_GHCR" = true ] && tags+=(-t "${ghcr}:${VERSION}")
    # Only promote to :latest for a full MULTI-arch release — a single-arch
    # (staging) build must not clobber the multi-arch :latest that ARM users pull.
    if [ "$VERSION" != "latest" ] && [[ "$PLATFORMS" == *,* ]]; then
        [ "$PUSH_DOCKERHUB" = true ] && tags+=(-t "${dh}:latest")
        [ "$PUSH_GHCR" = true ] && tags+=(-t "${ghcr}:latest")
    fi

    if [ "$PUSH" = true ]; then
        docker buildx build \
            --platform "$PLATFORMS" $NO_CACHE "$@" \
            "${tags[@]}" --push \
            -f "$dockerfile" "$context"
    else
        echo -e "${YELLOW}  (--no-push: building host arch only — multi-arch can't load locally)${NC}"
        docker buildx build \
            $NO_CACHE "$@" \
            -t "${dh}:${VERSION}" --load \
            -f "$dockerfile" "$context"
    fi
}

# Helper: authenticate to the registries before pushing, using creds from .env.
# Falls back to any existing `docker login` session if a key is absent.
registry_login() {
    [ "$PUSH" = true ] || return 0

    if [ "$PUSH_DOCKERHUB" = true ]; then
        if [ -n "${DOCKERHUB_USERNAME:-}" ] && [ -n "${DOCKERHUB_TOKEN:-}" ]; then
            echo -e "${BLUE}→ Logging in to Docker Hub as ${DOCKERHUB_USERNAME}...${NC}"
            echo "${DOCKERHUB_TOKEN}" | docker login -u "${DOCKERHUB_USERNAME}" --password-stdin
        else
            echo -e "${YELLOW}  ! DOCKERHUB_USERNAME/DOCKERHUB_TOKEN not set — relying on existing 'docker login'.${NC}"
        fi
    fi

    if [ "$PUSH_GHCR" = true ]; then
        local ghcr_user="${GHCR_USERNAME:-${GITHUB_ACTOR:-}}"
        local ghcr_token="${GHCR_TOKEN:-${GITHUB_TOKEN:-}}"
        if [ -n "$ghcr_user" ] && [ -n "$ghcr_token" ]; then
            echo -e "${BLUE}→ Logging in to ghcr.io as ${ghcr_user}...${NC}"
            echo "${ghcr_token}" | docker login ghcr.io -u "${ghcr_user}" --password-stdin \
                || echo -e "${YELLOW}  ! GHCR login failed — pass --docker-only to skip GHCR (Docker Hub is what installs pull).${NC}"
        else
            echo -e "${YELLOW}  ! GHCR_USERNAME/GHCR_TOKEN not set — relying on existing 'docker login' for ghcr.io.${NC}"
        fi
    fi
}

registry_login
ensure_builder

# =============================================================
# Build Backend
# =============================================================
if [ "$BUILD_BACKEND" = true ]; then
    echo -e "${YELLOW}Building Backend API...${NC}"
    buildx_image "-backend" deployments/backend.Dockerfile ./backend --build-arg VERSION="${VERSION}"
    echo -e "${GREEN}✓ Backend complete${NC}"
    echo ""
fi

# =============================================================
# Build Frontend
# =============================================================
if [ "$BUILD_FRONTEND" = true ]; then
    echo -e "${YELLOW}Building Frontend Dashboard...${NC}"
    buildx_image "-frontend" deployments/frontend.Dockerfile ./frontend
    echo -e "${GREEN}✓ Frontend complete${NC}"
    echo ""
fi

# =============================================================
# Build Agent
# =============================================================
if [ "$BUILD_AGENT" = true ]; then
    echo -e "${YELLOW}Building Agent Controller...${NC}"
    buildx_image "-agent" deployments/agent.Dockerfile ./agent
    echo -e "${GREEN}✓ Agent complete${NC}"
    echo ""
fi

# =============================================================
# Build All-in-One (Legacy)
# =============================================================
if [ "$BUILD_ALL_IN_ONE" = true ]; then
    echo -e "${YELLOW}Building All-in-One (legacy)...${NC}"
    buildx_image "" Dockerfile . --build-arg VERSION="${VERSION}"
    echo -e "${GREEN}✓ All-in-One complete${NC}"
    echo ""
fi

# =============================================================
# Summary
# =============================================================
echo -e "${GREEN}================================================${NC}"
echo -e "${GREEN}  Build Complete!${NC}"
echo -e "${GREEN}================================================${NC}"
echo ""

if [ "$BUILD_BACKEND" = true ]; then
    echo -e "  ${BLUE}Backend:${NC}    ${DH_PREFIX}-backend:${VERSION}"
fi
if [ "$BUILD_FRONTEND" = true ]; then
    echo -e "  ${BLUE}Frontend:${NC}   ${DH_PREFIX}-frontend:${VERSION}"
fi
if [ "$BUILD_AGENT" = true ]; then
    echo -e "  ${BLUE}Agent:${NC}      ${DH_PREFIX}-agent:${VERSION}"
fi
if [ "$BUILD_ALL_IN_ONE" = true ]; then
    echo -e "  ${BLUE}All-in-One:${NC} ${DH_PREFIX}:${VERSION}"
fi

echo ""

if [ "$PUSH" = true ]; then
    echo -e "${GREEN}Images pushed to Docker Hub and GHCR${NC}"
    echo ""
    echo "Docker Hub:"
    if [ "$BUILD_BACKEND" = true ]; then
        echo "  docker pull ${DH_PREFIX}-backend:${VERSION}"
    fi
    if [ "$BUILD_FRONTEND" = true ]; then
        echo "  docker pull ${DH_PREFIX}-frontend:${VERSION}"
    fi
    if [ "$BUILD_AGENT" = true ]; then
        echo "  docker pull ${DH_PREFIX}-agent:${VERSION}"
    fi
    if [ "$BUILD_ALL_IN_ONE" = true ]; then
        echo "  docker pull ${DH_PREFIX}:${VERSION}"
    fi
    echo ""
    echo "GHCR:"
    if [ "$BUILD_BACKEND" = true ]; then
        echo "  docker pull ${GHCR_PREFIX}-backend:${VERSION}"
    fi
    if [ "$BUILD_FRONTEND" = true ]; then
        echo "  docker pull ${GHCR_PREFIX}-frontend:${VERSION}"
    fi
    if [ "$BUILD_AGENT" = true ]; then
        echo "  docker pull ${GHCR_PREFIX}-agent:${VERSION}"
    fi
    if [ "$BUILD_ALL_IN_ONE" = true ]; then
        echo "  docker pull ${GHCR_PREFIX}:${VERSION}"
    fi
else
    echo -e "${YELLOW}Images built locally (not pushed)${NC}"
    echo ""
    echo "To push manually:"
    if [ "$BUILD_BACKEND" = true ]; then
        echo "  docker push ${DH_PREFIX}-backend:${VERSION}"
        echo "  docker push ${GHCR_PREFIX}-backend:${VERSION}"
    fi
    if [ "$BUILD_FRONTEND" = true ]; then
        echo "  docker push ${DH_PREFIX}-frontend:${VERSION}"
        echo "  docker push ${GHCR_PREFIX}-frontend:${VERSION}"
    fi
    if [ "$BUILD_AGENT" = true ]; then
        echo "  docker push ${DH_PREFIX}-agent:${VERSION}"
        echo "  docker push ${GHCR_PREFIX}-agent:${VERSION}"
    fi
    if [ "$BUILD_ALL_IN_ONE" = true ]; then
        echo "  docker push ${DH_PREFIX}:${VERSION}"
        echo "  docker push ${GHCR_PREFIX}:${VERSION}"
    fi
fi

echo ""
