// Runtime configuration for the Consent Portal.
//
// QUICK START (local development):
//   1. Copy this file:  cp public/config.example.js public/config.js
//   2. Replace googleClientId with the real Google OAuth 2.0 Client ID
//      from your Google Cloud Console.
//   3. Run: npm run dev
//
// In development, Vite proxies /oauth2/token and /api/v1 to avoid CORS
// (see vite.config.ts). Use relative paths as shown below.
//
// In production (Docker), entrypoint.sh generates config.js with absolute
// URLs pointing to the actual ThunderID and Consent Engine endpoints.
//
// This file is loaded as a plain <script> in index.html and exposed on
// `window.configs`. It is gitignored — config.example.js is the template.

window.configs = {
    // Consent Engine API base URL.
    // Development: proxied via Vite (see vite.config.ts → localhost:8081).
    // Production:  set to the actual Consent Engine URL.
    consentEngineUrl: '/api/v1',

    // OIDC / IdP settings (ThunderID).
    idpClientId: 'CONSENT_PORTAL_APP',
    idpBaseUrl: 'https://localhost:8090',
    idpScope: 'openid profile email',

    // OAuth2 redirect URLs (must be registered with the IdP).
    idpSignInRedirectUrl: 'http://localhost:5173',
    idpSignOutRedirectUrl: 'http://localhost:5173',

    // Google OAuth 2.0 Client ID (from Google Cloud Console).
    // IMPORTANT: Add http://localhost:5173 to "Authorized JavaScript origins"
    // (NOT "Authorized redirect URIs") in the Google Cloud Console.
    googleClientId: 'your_google_client_id.apps.googleusercontent.com',

    // ThunderID token endpoint for RFC 8693 token exchange.
    // Development: proxied via Vite (see vite.config.ts → localhost:8090).
    // Production:  set to the actual ThunderID token URL.
    thunderIdTokenUrl: '/oauth2/token',
};
