import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vitest/config'
import { loadEnv } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig(({ mode }) => ({
  plugins: [vue()],
  resolve: { alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) } },
  server: {
    port: 5173,
    proxy: { '/api': { target: loadEnv(mode, process.cwd(), '').VITE_API_TARGET ?? 'http://localhost:8080', changeOrigin: false } },
  },
  build: {
    chunkSizeWarningLimit: 700,
    rollupOptions: {
      output: {
        manualChunks: { vue: ['vue', 'vue-router', 'pinia'], primevue: ['primevue'] },
      },
    },
  },
  test: { environment: 'jsdom', globals: true },
}))
