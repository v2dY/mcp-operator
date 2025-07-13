#!/bin/sh
set -e

if [ -z "$1" ]; then
  echo "Usage: $0 <openapi_yaml_url> [base_path]"
  exit 1
fi

OPENAPI_URL="$1"
BASE_PATH="$2"
MERGED_FILE="/tmp/openapi"

echo "Downloading $OPENAPI_URL..."
wget -qO "$MERGED_FILE" "$OPENAPI_URL"

echo "Running openapi-mcp-generator..."
if [ -n "$BASE_PATH" ]; then
  openapi-mcp-generator --input "$MERGED_FILE" -o . --transport=web --force --port=3001 -b "$BASE_PATH"
else
  openapi-mcp-generator --input "$MERGED_FILE" -o . --transport=web --force --port=3001
fi

npm install
npm run build
npm run start:web 