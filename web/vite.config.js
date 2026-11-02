import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// Dev only: forward /api/* to the Go API (stripping the prefix) so the browser
// stays same-origin and the API needs no CORS handling.
// shortcut: no production serving story; put a reverse proxy in front before deploying.
export default defineConfig({
  plugins: [vue()],
  server: {
    proxy: {
      '/api': {
        target: process.env.API_URL || 'http://localhost:8080',
        rewrite: (path) => path.replace(/^\/api/, ''),
      },
    },
  },
})
