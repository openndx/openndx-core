// Runtime configuration for the Consent Portal.
//
// Copy this file to `config.js` (which is gitignored) and adjust the values for
// your environment. It is loaded as a plain <script> in index.html and exposed
// on `window.configs`, so it can be swapped per-deployment without rebuilding.
window.configs = {
    // Base URL of the Consent Engine API.
    consentEngineUrl: 'http://localhost:8081/api/v1',

    // OIDC / IdP settings — used for RFC 8693 token exchange with ThunderID.
    idpClientId: 'your_client_id',
    idpBaseUrl: 'https://your-idp.example.com',
    idpScope: 'openid profile email',

    // OAuth2 redirect URLs (must be registered with the IdP).
    idpSignInRedirectUrl: 'http://localhost:5173',
    idpSignOutRedirectUrl: 'http://localhost:5173',

    // Google OAuth 2.0 Client ID (from Google Cloud Console).
    // Used by the GoogleLogin button to obtain a Google ID token.
    googleClientId: 'your_google_client_id.apps.googleusercontent.com',

    // ThunderID token endpoint for RFC 8693 token exchange.
    // The SPA sends the Google ID token here to get a ThunderID access_token.
    thunderIdTokenUrl: 'https://localhost:8090/oauth2/token',
};
