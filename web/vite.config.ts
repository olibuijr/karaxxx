import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import path from 'path'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    proxy: {
      '/api': 'http://localhost:8799',
      '/thumb': 'http://localhost:8799',
      '/vid': 'http://localhost:8799',
      '/media': 'http://localhost:8799',
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
})
