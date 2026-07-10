#!/bin/sh
set -e

# Generate config.js from environment variables at runtime
cat > /usr/share/nginx/html/config.js << EOF
window.configs = {
  API_URL: '${VITE_API_URL:-}',
  LOGS_URL: '${VITE_LOGS_URL:-}',
  CLIENT_ID: '${VITE_CLIENT_ID:-}',
  BASE_URL: '${VITE_BASE_URL:-}',
  SCOPE: '${VITE_SCOPE:-}',
  SIGN_IN_REDIRECT_URL: '${VITE_SIGN_IN_REDIRECT_URL:-}',
  SIGN_OUT_REDIRECT_URL: '${VITE_SIGN_OUT_REDIRECT_URL:-}'
};
EOF

echo "Configuration file generated at runtime with current environment variables"

# Start nginx
exec nginx -g 'daemon off;'