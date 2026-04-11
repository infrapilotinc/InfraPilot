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
#   --platform    Target platform (e.g., linux/amd64,linux/arm64)
#
# Examples:
#   ./scripts/build-and-publish.sh v1.2.3
#   ./scripts/build-and-publish.sh --backend --no-push
#   ./scripts/build-and-publish.sh latest --platform linux/amd64,linux/arm64
#
# =============================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Registries — images are pushed to both Docker Hub and GHCR.
# Override with env vars for private registries.
DH_PREFIX="${DH_REGISTRY:-infrapilotsh/infrapilot-ce}"
GHCR_PREFIX="${GHCR_REGISTRY:-ghcr.io/infrapilotsh/infrapilot-ce}"

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
PLATFORM=""

# Process options
shift || true
while [[ $# -gt 0 ]]; do
    case $1 in
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
        --no-push)
            PUSH=false
            shift
            ;;
        --platform)
            PLATFORM="--platform $2"
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
echo -e "  Platform: ${BLUE}${PLATFORM:-default}${NC}"
echo -e "  Docker Hub: ${BLUE}${DH_PREFIX}${NC}"
echo -e "  GHCR:       ${BLUE}${GHCR_PREFIX}${NC}"
echo ""

# Helper: tag and push an image to both registries
push_both() {
    local name="$1"    # e.g. "-backend" or ""
    local ver="$2"     # e.g. "v1.0.0"

    local dh_img="${DH_PREFIX}${name}"
    local ghcr_img="${GHCR_PREFIX}${name}"

    # Tag for GHCR (Docker Hub tag already applied during build)
    docker tag "${dh_img}:${ver}" "${ghcr_img}:${ver}"

    echo -e "${YELLOW}Pushing to Docker Hub...${NC}"
    docker push "${dh_img}:${ver}"

    echo -e "${YELLOW}Pushing to GHCR...${NC}"
    docker push "${ghcr_img}:${ver}"

    if [ "$ver" != "latest" ]; then
        docker tag "${dh_img}:${ver}" "${dh_img}:latest"
        docker tag "${dh_img}:${ver}" "${ghcr_img}:latest"
        docker push "${dh_img}:latest"
        docker push "${ghcr_img}:latest"
    fi
}

# =============================================================
# Build Backend
# =============================================================
if [ "$BUILD_BACKEND" = true ]; then
    echo -e "${YELLOW}Building Backend API...${NC}"
    docker build \
        $PLATFORM \
        --build-arg VERSION="${VERSION}" \
        -t "${DH_PREFIX}-backend:${VERSION}" \
        -f deployments/backend.Dockerfile \
        ./backend

    if [ "$PUSH" = true ]; then
        push_both "-backend" "${VERSION}"
    fi
    echo -e "${GREEN}✓ Backend complete${NC}"
    echo ""
fi

# =============================================================
# Build Frontend
# =============================================================
if [ "$BUILD_FRONTEND" = true ]; then
    echo -e "${YELLOW}Building Frontend Dashboard...${NC}"
    docker build \
        $PLATFORM \
        -t "${DH_PREFIX}-frontend:${VERSION}" \
        -f deployments/frontend.Dockerfile \
        ./frontend

    if [ "$PUSH" = true ]; then
        push_both "-frontend" "${VERSION}"
    fi
    echo -e "${GREEN}✓ Frontend complete${NC}"
    echo ""
fi

# =============================================================
# Build Agent
# =============================================================
if [ "$BUILD_AGENT" = true ]; then
    echo -e "${YELLOW}Building Agent Controller...${NC}"
    docker build \
        $PLATFORM \
        -t "${DH_PREFIX}-agent:${VERSION}" \
        -f deployments/agent.Dockerfile \
        ./agent

    if [ "$PUSH" = true ]; then
        push_both "-agent" "${VERSION}"
    fi
    echo -e "${GREEN}✓ Agent complete${NC}"
    echo ""
fi

# =============================================================
# Build All-in-One (Legacy)
# =============================================================
if [ "$BUILD_ALL_IN_ONE" = true ]; then
    echo -e "${YELLOW}Building All-in-One (legacy)...${NC}"
    docker build \
        $PLATFORM \
        --build-arg VERSION="${VERSION}" \
        -t "${DH_PREFIX}:${VERSION}" \
        -f Dockerfile \
        .

    if [ "$PUSH" = true ]; then
        push_both "" "${VERSION}"
    fi
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
