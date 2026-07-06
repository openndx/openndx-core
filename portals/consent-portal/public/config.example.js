// Runtime configuration for the Consent Portal.
//
// Copy this file to `config.js` (which is gitignored) and adjust the values for
// your environment. It is loaded as a plain <script> in index.html and exposed
// on `window.configs`, so it can be swapped per-deployment without rebuilding.
window.configs = {
    // Base URL of the Consent Engine API.
    consentEngineUrl: 'http://localhost:8081/api/v1',

    // OIDC / IdP settings. Endpoints are resolved via OIDC discovery from
    // idpBaseUrl (i.e. idpBaseUrl/.well-known/openid-configuration).
    idpClientId: 'your_client_id',
    idpBaseUrl: 'https://your-idp.example.com',
    idpScope: 'openid profile email',

    // OAuth2 redirect URLs (must be registered with the IdP).
    idpSignInRedirectUrl: 'http://localhost:5173',
    idpSignOutRedirectUrl: 'http://localhost:5173',
};
