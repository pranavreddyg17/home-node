import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { dependencyEvidence } from './dependency-evidence.mjs'

export default defineConfig({
  plugins: [react(), dependencyEvidence()],
  server: {
    host: '127.0.0.1',
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:8787',
        changeOrigin: true,
      },
    },
  },
})
