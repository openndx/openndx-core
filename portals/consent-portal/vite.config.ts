import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    proxy: {
      // Proxy token exchange requests to ThunderID to avoid CORS in development.
      '/oauth2/token': {
        target: 'https://localhost:8090',
        changeOrigin: true,
        secure: false,
      },
      // Proxy Consent Engine API requests via the API Gateway (port 9081).
      // The Consent Engine (port 8081) is only accessible within the Docker
      // network, so we route through APISIX which is exposed on the host.
      '/api/v1': {
        target: 'http://localhost:9081',
        changeOrigin: true,
      },
    },
  },
})
