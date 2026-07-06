// Runtime configuration injected via public/config.js and exposed on
// `window.configs`. See public/config.example.js for the template.
export interface AppConfig {
  // Base URL of the Consent Engine API.
  consentEngineUrl: string;
  // OIDC / IdP settings. Endpoints are resolved via discovery from idpBaseUrl.
  idpBaseUrl: string;
  idpClientId: string;
  idpScope: string;
  // OAuth2 redirect URLs (must be registered with the IdP).
  idpSignInRedirectUrl: string;
  idpSignOutRedirectUrl: string;
}

declare global {
  interface Window {
    configs: AppConfig;
  }
}
