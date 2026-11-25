import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// The page talks to the Go server. In development that server runs beside
// vite, so its /api is proxied there.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: { '/api': 'http://127.0.0.1:8080' },
  },
  build: { outDir: 'dist', emptyOutDir: true },
})
