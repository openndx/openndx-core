# Build Stage
FROM node:20-alpine AS build

WORKDIR /app

# Copy package files
COPY package.json package-lock.json ./

# Install dependencies
RUN npm ci

# Copy source code
COPY . .

# Build the application (config is injected at runtime, not build time)
RUN npm run build

# Serve Stage
FROM nginx:1.27-alpine

# Copy built static files from build stage
COPY --from=build /app/dist /usr/share/nginx/html

# Copy custom nginx configuration
COPY nginx.conf /etc/nginx/conf.d/default.conf

# Copy entrypoint script
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Set default environment variables (can be overridden at runtime)
ENV CONSENT_ENGINE_URL=""
ENV IDP_CLIENT_ID=""
ENV IDP_BASE_URL=""
ENV IDP_SCOPE=""
ENV IDP_SIGN_IN_REDIRECT_URL=""
ENV IDP_SIGN_OUT_REDIRECT_URL=""
ENV GOOGLE_CLIENT_ID=""
ENV THUNDERID_TOKEN_URL=""

# Expose port 80
EXPOSE 80

# Use entrypoint script to generate config.js at runtime
ENTRYPOINT ["/entrypoint.sh"]

