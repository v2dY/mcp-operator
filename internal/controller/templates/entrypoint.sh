#!/bin/sh
set -e

echo "Starting MCP Server for {{.MCPServerName}}"
echo "OpenAPI URL: {{.OpenAPIUrl}}"
echo "Base Path: {{.BasePath}}"

# Change to app directory
cd /app

# Start the generated MCP server
echo "Starting MCP server on port 3001..."
exec npm run start:web