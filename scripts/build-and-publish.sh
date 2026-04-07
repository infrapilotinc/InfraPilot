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

# Registry target. Override with REGISTRY env var for private registries.
# Community images are published to ghcr.io/infrapilot-sh/infrapilot (public).
IMAGE_PREFIX="${REGISTRY:-ghcr.io/tybali/infrapilot}"

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
echo -e "  Version: ${BLUE}$VERSION${NC}"
echo -e "  Push:    ${BLUE}$PUSH${NC}"
echo -e "  Platform: ${BLUE}${PLATFORM:-default}${NC}"
echo ""

# =============================================================
# Build Backend
# =============================================================
if [ "$BUILD_BACKEND" = true ]; then
    echo -e "${YELLOW}Building Backend API...${NC}"
    docker build \
        $PLATFORM \
        --build-arg VERSION="${VERSION}" \
        -t "${IMAGE_PREFIX}-backend:${VERSION}" \
        -f deployments/backend.Dockerfile \
        ./backend

    if [ "$VERSION" != "latest" ]; then
        docker tag "${IMAGE_PREFIX}-backend:${VERSION}" "${IMAGE_PREFIX}-backend:latest"
    fi

    if [ "$PUSH" = true ]; then
        echo -e "${YELLOW}Pushing Backend...${NC}"
        docker push "${IMAGE_PREFIX}-backend:${VERSION}"
        if [ "$VERSION" != "latest" ]; then
            docker push "${IMAGE_PREFIX}-backend:latest"
        fi
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
        -t "${IMAGE_PREFIX}-frontend:${VERSION}" \
        -f deployments/frontend.Dockerfile \
        ./frontend

    if [ "$VERSION" != "latest" ]; then
        docker tag "${IMAGE_PREFIX}-frontend:${VERSION}" "${IMAGE_PREFIX}-frontend:latest"
    fi

    if [ "$PUSH" = true ]; then
        echo -e "${YELLOW}Pushing Frontend...${NC}"
        docker push "${IMAGE_PREFIX}-frontend:${VERSION}"
        if [ "$VERSION" != "latest" ]; then
            docker push "${IMAGE_PREFIX}-frontend:latest"
        fi
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
        -t "${IMAGE_PREFIX}-agent:${VERSION}" \
        -f deployments/agent.Dockerfile \
        ./agent

    if [ "$VERSION" != "latest" ]; then
        docker tag "${IMAGE_PREFIX}-agent:${VERSION}" "${IMAGE_PREFIX}-agent:latest"
    fi

    if [ "$PUSH" = true ]; then
        echo -e "${YELLOW}Pushing Agent...${NC}"
        docker push "${IMAGE_PREFIX}-agent:${VERSION}"
        if [ "$VERSION" != "latest" ]; then
            docker push "${IMAGE_PREFIX}-agent:latest"
        fi
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
        -t "${IMAGE_PREFIX}:${VERSION}" \
        -f Dockerfile \
        .

    if [ "$VERSION" != "latest" ]; then
        docker tag "${IMAGE_PREFIX}:${VERSION}" "${IMAGE_PREFIX}:latest"
    fi

    if [ "$PUSH" = true ]; then
        echo -e "${YELLOW}Pushing All-in-One...${NC}"
        docker push "${IMAGE_PREFIX}:${VERSION}"
        if [ "$VERSION" != "latest" ]; then
            docker push "${IMAGE_PREFIX}:latest"
        fi
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
    echo -e "  ${BLUE}Backend:${NC}    ${IMAGE_PREFIX}-backend:${VERSION}"
fi
if [ "$BUILD_FRONTEND" = true ]; then
    echo -e "  ${BLUE}Frontend:${NC}   ${IMAGE_PREFIX}-frontend:${VERSION}"
fi
if [ "$BUILD_AGENT" = true ]; then
    echo -e "  ${BLUE}Agent:${NC}      ${IMAGE_PREFIX}-agent:${VERSION}"
fi
if [ "$BUILD_ALL_IN_ONE" = true ]; then
    echo -e "  ${BLUE}All-in-One:${NC} ${IMAGE_PREFIX}:${VERSION}"
fi

echo ""

if [ "$PUSH" = true ]; then
    echo -e "${GREEN}Images pushed to GitHub Container Registry${NC}"
    echo ""
    echo "To pull images:"
    if [ "$BUILD_BACKEND" = true ]; then
        echo "  docker pull ${IMAGE_PREFIX}-backend:${VERSION}"
    fi
    if [ "$BUILD_FRONTEND" = true ]; then
        echo "  docker pull ${IMAGE_PREFIX}-frontend:${VERSION}"
    fi
    if [ "$BUILD_AGENT" = true ]; then
        echo "  docker pull ${IMAGE_PREFIX}-agent:${VERSION}"
    fi
    if [ "$BUILD_ALL_IN_ONE" = true ]; then
        echo "  docker pull ${IMAGE_PREFIX}:${VERSION}"
    fi
else
    echo -e "${YELLOW}Images built locally (not pushed)${NC}"
    echo ""
    echo "To push manually:"
    if [ "$BUILD_BACKEND" = true ]; then
        echo "  docker push ${IMAGE_PREFIX}-backend:${VERSION}"
    fi
    if [ "$BUILD_FRONTEND" = true ]; then
        echo "  docker push ${IMAGE_PREFIX}-frontend:${VERSION}"
    fi
    if [ "$BUILD_AGENT" = true ]; then
        echo "  docker push ${IMAGE_PREFIX}-agent:${VERSION}"
    fi
    if [ "$BUILD_ALL_IN_ONE" = true ]; then
        echo "  docker push ${IMAGE_PREFIX}:${VERSION}"
    fi
fi

echo ""
