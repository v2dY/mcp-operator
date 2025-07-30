#!/bin/bash
set -e
ulimit -n 65536

echo "Starting Buildah job for MCPServer: {{.MCPServerName}}"
echo "OpenAPI URL: {{.OpenAPIUrl}}"
echo "Base Path: {{.BasePath}}"
echo "Target Image: {{.ImageName}}"

# Create working container from Node.js base image
buildah from --name working-container node:22-slim

# Install system dependencies
buildah run working-container -- apt update
buildah run working-container -- apt install -y wget

# Install openapi-mcp-generator globally
buildah run working-container -- npm i -g https://github.com/sarco3t/openapi-mcp-generator/releases/download/v3.1.6/openapi-mcp-generator-3.1.6.tgz

# Create non-root user
buildah run working-container -- groupadd -r appuser
buildah run working-container -- useradd -r -g appuser -u 1001 appuser

# Create npm cache directory for the user
buildah run working-container -- mkdir -p /tmp/.npm
buildah run working-container -- chown -R appuser:appuser /tmp/.npm

# Set working directory
buildah run working-container -- mkdir -p /app
buildah run working-container -- chown -R appuser:appuser /app

# Copy package.json
echo "$PACKAGE_JSON" | buildah run working-container -- sh -c 'cat > /app/package.json'

# Copy entrypoint script
buildah copy working-container entrypoint.sh /entrypoint.sh
echo "$ENTRYPOINT_SH" | buildah run working-container -- sh -c 'cat > /entrypoint.sh'

# Download OpenAPI spec
echo "Downloading OpenAPI spec from: {{.OpenAPIUrl}}"
buildah run working-container -- wget -qO /tmp/openapi "{{.OpenAPIUrl}}"

# Generate MCP server code
echo "Generating MCP server code..."
buildah run working-container -- sh -c 'cd /app && \
  if [ -n "{{.BasePath}}" ]; then \
    openapi-mcp-generator --input /tmp/openapi -o . --transport=web --force --port=3001 -b "{{.BasePath}}"; \
  else \
    openapi-mcp-generator --input /tmp/openapi -o . --transport=web --force --port=3001; \
  fi'

# Install dependencies and build
echo "Installing dependencies and building..."
buildah run working-container -- sh -c 'cd /app && npm install && npm run build'

# Set container configuration
buildah config --user appuser working-container
buildah config --workingdir /app working-container
buildah config --env NPM_CONFIG_CACHE=/tmp/.npm working-container
buildah config --entrypoint '["/entrypoint.sh"]' working-container
buildah config --port 3001 working-container

# Commit the image
echo "Committing image: {{.ImageName}}"
buildah commit working-container "{{.ImageName}}"

# Push to registry
echo "Pushing image to registry..."
buildah push --tls-verify=false "{{.ImageName}}"

echo "Buildah job completed successfully!"