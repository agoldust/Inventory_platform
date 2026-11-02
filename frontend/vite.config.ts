import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// Dev only: forward /api/* to the Go API (stripping the prefix) so the browser
// stays same-origin and the API needs no CORS handling. In Docker, nginx does
// the same job (see nginx.conf).
export default defineConfig({
  plugins: [vue()],
  server: {
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        rewrite: (path) => path.replace(/^\/api/, ''),
      },
    },
  },
})
