FROM node:22-slim

WORKDIR /app
RUN apt update && apt install -y wget
RUN npm i -g https://github.com/sarco3t/openapi-mcp-generator/releases/download/v3.1.6/openapi-mcp-generator-3.1.6.tgz

# Create non-root user
RUN groupadd -r appuser && useradd -r -g appuser -u 1001 appuser

# Create npm cache directory for the user
RUN mkdir -p /tmp/.npm && chown -R appuser:appuser /tmp/.npm

COPY package*.json ./
RUN npm install

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Change ownership of /app to the non-root user
RUN chown -R appuser:appuser /app

USER appuser

# Configure npm to use /tmp for cache
ENV NPM_CONFIG_CACHE=/tmp/.npm

ENTRYPOINT ["/entrypoint.sh"]
