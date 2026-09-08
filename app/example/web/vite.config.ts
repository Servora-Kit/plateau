import { fileURLToPath, URL } from 'node:url'

import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import vueJsx from '@vitejs/plugin-vue-jsx'
import vueDevTools from 'vite-plugin-vue-devtools'

// https://vite.dev/config/
export default defineConfig({
  plugins: [
    vue(),
    vueJsx(),
    vueDevTools(),
  ],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
    dedupe: ['@servora/proto-utils'],
  },
  server: {
    port: 10032,
    strictPort: true,
    proxy: {
      '/v1': {
        target: 'http://127.0.0.1:10030',
        changeOrigin: true,
      },
    },
  },
  preview: {
    port: 10032,
    strictPort: true,
  },
})
