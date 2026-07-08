import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { AuthProvider, type AuthProviderProps } from "react-oidc-context";
import { BrowserRouter } from 'react-router-dom';
import App from './App.tsx';
import { ConsentProvider } from './contexts/ConsentContext';
import type { AppConfig } from './types/config';
import './index.css';

// Fail fast with a clear message if the runtime config (public/config.js) is
// missing or incomplete, instead of crashing later with an opaque OIDC error.
if (!window.configs) {
  throw new Error(
    'Runtime configuration is missing: window.configs is not defined. ' +
    'Copy public/config.example.js to public/config.js and set the values.'
  );
}

// idpScope is intentionally omitted — it has a sensible default below.
const requiredConfigKeys: (keyof AppConfig)[] = [
  'consentEngineUrl',
  'idpBaseUrl',
  'idpClientId',
  'idpSignInRedirectUrl',
  'idpSignOutRedirectUrl',
];

const missingConfigKeys = requiredConfigKeys.filter((key) => !window.configs[key]);
if (missingConfigKeys.length > 0) {
  throw new Error(`Missing required runtime configuration: ${missingConfigKeys.join(', ')}`);
}

const oidcConfig: AuthProviderProps = {
  authority: window.configs.idpBaseUrl, // Endpoints are resolved via OIDC discovery (.well-known/openid-configuration)
  client_id: window.configs.idpClientId,
  redirect_uri: window.configs.idpSignInRedirectUrl,
  post_logout_redirect_uri: window.configs.idpSignOutRedirectUrl,
  scope: window.configs.idpScope || 'openid profile email',
  onSigninCallback: () => {
    // Remove query params (code, state) after successful login
    window.history.replaceState({}, document.title, window.location.pathname);
  }
};

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <AuthProvider {...oidcConfig}>
      <BrowserRouter>
        <ConsentProvider>
          <App />
        </ConsentProvider>
      </BrowserRouter>
    </AuthProvider>
  </StrictMode>,
)
