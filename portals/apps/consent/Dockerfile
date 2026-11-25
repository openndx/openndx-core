# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Lanka Software Foundation

# Build Stage
FROM node:24-alpine AS build

# Enable pnpm (version pinned via the workspace root's "packageManager" field)
RUN corepack enable

WORKDIR /app

# Copy workspace manifests first for better layer caching
COPY package.json pnpm-workspace.yaml pnpm-lock.yaml ./
COPY apps/admin/package.json ./apps/admin/
COPY apps/consent/package.json ./apps/consent/
COPY apps/member/package.json ./apps/member/

# Install only consent-portal's dependencies (and any workspace packages it depends on)
RUN pnpm install --frozen-lockfile --filter consent-portal...

# Copy consent-portal's source code
COPY apps/consent ./apps/consent

# Build the application (config is injected at runtime, not build time)
RUN pnpm --filter consent-portal run build

# Serve Stage
FROM nginx:1.27-alpine

# Copy built static files from build stage
COPY --from=build /app/apps/consent/dist /usr/share/nginx/html

# Copy custom nginx configuration
COPY apps/consent/nginx.conf /etc/nginx/conf.d/default.conf

# Copy entrypoint script
COPY apps/consent/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Set default environment variables (can be overridden at runtime)
ENV CONSENT_ENGINE_URL=""
ENV IDP_CLIENT_ID=""
ENV IDP_BASE_URL=""
ENV IDP_SCOPE=""
ENV IDP_SIGN_IN_REDIRECT_URL=""
ENV IDP_SIGN_OUT_REDIRECT_URL=""

# Expose port 80
EXPOSE 80

# Use entrypoint script to generate config.js at runtime
ENTRYPOINT ["/entrypoint.sh"]

