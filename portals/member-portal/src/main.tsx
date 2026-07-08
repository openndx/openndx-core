import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import App from './App.tsx'
import { AuthProvider } from "@asgardeo/auth-react";

declare global {
  interface Window {
    configs: {
      API_URL: string;
      LOGS_URL: string;
      CLIENT_ID: string;
      BASE_URL: string;
      SCOPE: string;
      SIGN_IN_REDIRECT_URL: string;
      SIGN_OUT_REDIRECT_URL: string;
    };
  }
}

const config = {
     signInRedirectURL: window?.configs?.SIGN_IN_REDIRECT_URL,
     signOutRedirectURL: window?.configs?.SIGN_OUT_REDIRECT_URL,
     clientID: window?.configs?.CLIENT_ID,
     baseUrl: window?.configs?.BASE_URL,
     scope: window?.configs?.SCOPE ? window.configs.SCOPE.split(',') : ['openid', 'profile'],
     endpoints: {
         authorizationEndpoint: "https://api.asgardeo.io/t/lankasoftwarefoundation/oauth2/authorize",
         tokenEndpoint: "https://api.asgardeo.io/t/lankasoftwarefoundation/oauth2/token",
         userInfoEndpoint: "https://api.asgardeo.io/t/lankasoftwarefoundation/oauth2/userinfo",
         endSessionEndpoint: "https://api.asgardeo.io/t/lankasoftwarefoundation/oidc/logout"
     }
};

console.log("Auth config:", config);
console.log("Window configs:", window.configs);

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <AuthProvider config={config}>
      <App />
    </AuthProvider>
  </StrictMode>,
)
