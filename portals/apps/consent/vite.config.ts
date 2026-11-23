/**
 * SPDX-License-Identifier: Apache-2.0
 * Copyright (c) 2026 Lanka Software Foundation
 */

import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
})
