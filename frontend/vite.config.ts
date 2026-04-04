import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
const backendUrl = process.env.BACKEND_URL || 'http://localhost:8086'

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/api': backendUrl,
    },
  },
})
