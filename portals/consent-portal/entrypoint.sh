#!/bin/sh
set -e

# Generate config.js from environment variables at runtime
cat > /usr/share/nginx/html/config.js << EOF
window.configs = {
  consentEngineUrl: '${CONSENT_ENGINE_URL:-}',
  idpClientId: '${IDP_CLIENT_ID:-}',
  idpBaseUrl: '${IDP_BASE_URL:-}',
  idpScope: '${IDP_SCOPE:-}',
  idpSignInRedirectUrl: '${IDP_SIGN_IN_REDIRECT_URL:-}',
  idpSignOutRedirectUrl: '${IDP_SIGN_OUT_REDIRECT_URL:-}',
  googleClientId: '${GOOGLE_CLIENT_ID:-}',
  thunderIdTokenUrl: '${THUNDERID_TOKEN_URL:-}'
};
EOF

echo "Configuration file generated at runtime with current environment variables"

# Start nginx
exec nginx -g 'daemon off;'