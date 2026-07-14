import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { GoogleOAuthProvider } from '@react-oauth/google';
import { BrowserRouter } from 'react-router-dom';
import App from './App.tsx';
import { AuthProvider } from './contexts/AuthContext';
import { ConsentProvider } from './contexts/ConsentContext';
import type { AppConfig } from './types/config';
import './index.css';

// Fail fast with a clear message if the runtime config (public/config.js) is
// missing or incomplete.
if (!window.configs) {
  throw new Error(
    'Runtime configuration is missing: window.configs is not defined. ' +
    'Copy public/config.example.js to public/config.js and set the values.'
  );
}

const requiredConfigKeys: (keyof AppConfig)[] = [
  'consentEngineUrl',
  'idpBaseUrl',
  'idpClientId',
  'idpSignInRedirectUrl',
  'idpSignOutRedirectUrl',
  'googleClientId',
  'thunderIdTokenUrl',
];

const missingConfigKeys = requiredConfigKeys.filter((key) => !window.configs[key]);
if (missingConfigKeys.length > 0) {
  throw new Error(`Missing required runtime configuration: ${missingConfigKeys.join(', ')}`);
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <GoogleOAuthProvider clientId={window.configs.googleClientId}>
      <BrowserRouter>
        <AuthProvider>
          <ConsentProvider>
            <App />
          </ConsentProvider>
        </AuthProvider>
      </BrowserRouter>
    </GoogleOAuthProvider>
  </StrictMode>,
)
