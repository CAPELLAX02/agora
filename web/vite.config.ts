import { fileURLToPath, URL } from 'node:url'

import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

// Geliştirmede API isteklerini Vite sunucusu Go API'ye aktarır: tarayıcı için web ve
// API aynı origin'dedir (production'daki gibi). Böylece refresh çerezi (SameSite=Strict,
// Path=/api/v1/auth) ve CORS geliştirmede de production'daki gibi çalışır.
const apiTarget = process.env.AGORA_API_URL ?? 'http://localhost:8080'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) },
  },
  server: {
    port: 5173,
    strictPort: true,
    proxy: { '/api': { target: apiTarget } },
  },
  build: {
    sourcemap: true,
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    css: false,
    restoreMocks: true,
  },
})
