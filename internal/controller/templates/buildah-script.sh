#!/bin/bash
set -e
ulimit -n 65536

echo "Starting Buildah job for MCPServer: {{.MCPServerName}}"
echo "OpenAPI URL: {{.OpenAPIUrl}}"
echo "Base Path: {{.BasePath}}"
echo "Target Image: {{.ImageName}}"

# Create build context directory (if not exists)
mkdir -p /var/lib/containers/build-context


# Build image using Dockerfile
echo "Building image using Dockerfile..."
cd /var/lib/containers/build-context

buildah build -t "{{.ImageName}}" .

# Push to registry
echo "Pushing image to registry..."
buildah push --tls-verify=false "{{.ImageName}}"

echo "Buildah job completed successfully!"