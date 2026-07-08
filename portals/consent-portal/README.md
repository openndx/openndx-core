# Consent Portal

A citizen-facing React application that allows data owners to view, approve, or deny data access requests.

## Overview

The Consent Portal is the interface where citizens (data owners) interact with OpenDIF to manage their data consents. It is typically accessed via a redirect from a data consumer application when consent is required.

**Technology**: React + TypeScript + TailwindCSS + Vite

## Features

- **Consent Review** - View details of data access requests (who, what, why)
- **Approval/Denial** - Grant or deny access to requested data
- **Consent Management** - View and revoke previously granted consents
- **Secure Authentication** - Integration with IdP for user authentication

## Quick Start

### Prerequisites

- Node.js 18+
- npm 9+

### Run the Application

```bash
# Install dependencies
npm install

# Create your runtime config (see Configuration below)
cp public/config.example.js public/config.js

# Run in development mode
npm run dev
```

The application will be available at `http://localhost:5173`. To use a different
port, pass it to Vite: `npm run dev -- --port 5180`.

## Configuration

The portal is configured at **runtime**, not at build time. `public/config.js`
is loaded as a plain `<script>` in `index.html` and exposed on `window.configs`,
so the same build can be pointed at different environments just by swapping this
file — no rebuild required.

`public/config.js` is gitignored. Create it by copying the template:

```bash
cp public/config.example.js public/config.js
```

Then set the values in `public/config.js`:

| Key                     | Description                                                                                                 |
|-------------------------|-------------------------------------------------------------------------------------------------------------|
| `consentEngineUrl`      | Base URL of the Consent Engine API (e.g. `http://localhost:8081/api/v1`)                                    |
| `idpClientId`           | OAuth2 client ID registered with the IdP                                                                    |
| `idpBaseUrl`            | OIDC issuer base URL; endpoints are resolved via discovery at `idpBaseUrl/.well-known/openid-configuration` |
| `idpScope`              | Space-separated OAuth2 scopes (e.g. `openid profile email`)                                                 |
| `idpSignInRedirectUrl`  | Post-login redirect URL (must be registered with the IdP)                                                   |
| `idpSignOutRedirectUrl` | Post-logout redirect URL (must be registered with the IdP)                                                  |

## Testing Guide

### End-to-End Flow

1. **Start Backend Services**: Ensure Consent Engine is running on port 8081.
2. **Generate Consent Request**:
   - Use Postman or curl to create a consent request in Consent Engine.
   - Copy the `consent_id` from the response.
3. **Access Portal**:
   - Navigate to `http://localhost:5173/?consentId={consent_id}`
   - Log in if required.
   - Review and act on the consent request.