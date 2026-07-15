# Portals

React-based portals for the OpenDIF platform.

## Portals

- **Admin Portal** - Administrative interface for managing the platform
- **Consent Portal** - User-facing consent management interface
- **Member Portal** - Management of `Data sources` or `Applications` by members

## Configuration (`config.js`)

Each portal relies on a `config.js` file for runtime configuration. This file is:
1.  **Generated at runtime** in Docker (via `entrypoint.sh`).
2.  **Created manually** (or via script) for local development in the `public/` directory.
3.  **Loaded in HTML** (`<script src="/config.js">`) before the main app.
4.  **Accessed via `window.configs`** in the application.

### Locations & Variables

| Portal | Config Path | Required Variables |
|--------|------------|-------------------|
| **Admin** | `admin-portal/public/config.js` | `VITE_API_URL`, `VITE_LOGS_URL`, `VITE_IDP_CLIENT_ID`, `VITE_IDP_BASE_URL`, ... |
| **Consent** | `consent-portal/public/config.js` | `apiUrl`, `VITE_CLIENT_ID`, `VITE_BASE_URL`, `VITE_SCOPE`, ... |
| **Member** | `member-portal/public/config.js` | `apiUrl`, `logsUrl`, `VITE_CLIENT_ID`, `VITE_BASE_URL`, ... |

## Local Development Setup

Use the helper script to generate `config.js` files with test values for all portals:

```bash
cd portals
./setup-portals.sh
```

This script creates the files, validates dependencies, and ensures correct HTML references.

## Running & Testing

### 1. Start a Portal
```bash
# Example: Admin Portal
cd admin-portal
VITE_PORT=5174 npm run dev
```

### 2. Verify Configuration Loading
To confirm `config.js` is loaded correctly:

1.  Open the portal (e.g., `http://localhost:5174`).
2.  Open **DevTools** (F12) -> **Console**.
3.  Look for the log: `Window configs: { ... }`.
4.  Type `window.configs` in the console to inspect values.
5.  Check **Network** tab: Ensure `config.js` loads with status `200`.

> **Troubleshooting:** If `window.configs` is undefined, check that `public/config.js` exists and is referenced in `index.html`.
