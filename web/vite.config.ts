import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

const adminPath = (process.env.ADMIN_PATH ?? '').replace(/^\/+|\/+$/g, '')
const apiBase = adminPath ? `/${adminPath}` : ''
const proxyTarget = 'http://127.0.0.1:8080'

export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5173,
    proxy: {
      '/api': { target: proxyTarget, rewrite: (path) => `${apiBase}${path}` },
      '/icons': proxyTarget,
      '/source.json': proxyTarget,
      '/health': proxyTarget,
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
})

