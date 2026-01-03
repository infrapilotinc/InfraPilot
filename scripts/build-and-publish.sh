#!/bin/bash
set -e

# InfraPilot Docker Build & Publish Script

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
IMAGE_NAME="devsimplex/infrapilot"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Get version from argument or default to "latest"
VERSION="${1:-latest}"

echo -e "${GREEN}Building InfraPilot Docker Image${NC}"
echo "  Image: ${IMAGE_NAME}:${VERSION}"
echo ""

cd "$PROJECT_ROOT"

# Build the image
echo -e "${YELLOW}Building image...${NC}"
docker build -t "${IMAGE_NAME}:${VERSION}" .

# Also tag as latest if version is not "latest"
if [ "$VERSION" != "latest" ]; then
  echo -e "${YELLOW}Tagging as latest...${NC}"
  docker tag "${IMAGE_NAME}:${VERSION}" "${IMAGE_NAME}:latest"
fi

echo ""
echo -e "${YELLOW}Pushing to Docker Hub...${NC}"

# Push the versioned tag
docker push "${IMAGE_NAME}:${VERSION}"

# Push latest tag if version is not "latest"
if [ "$VERSION" != "latest" ]; then
  docker push "${IMAGE_NAME}:latest"
fi

echo ""
echo -e "${GREEN}Done!${NC}"
echo ""
echo "  Image: ${IMAGE_NAME}:${VERSION}"
echo "  Pull:  docker pull ${IMAGE_NAME}:${VERSION}"
echo ""
