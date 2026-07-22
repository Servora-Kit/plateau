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
      '@servora/proto-utils/client': fileURLToPath(
        new URL('../../../../servora/web/packages/proto-utils/src/client/index.ts', import.meta.url),
      ),
      '@servora/proto-utils/crud': fileURLToPath(
        new URL('../../../../servora/web/packages/proto-utils/src/crud/index.ts', import.meta.url),
      ),
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    proxy: {
      '/v1': {
        target: 'http://127.0.0.1:28080',
        changeOrigin: true,
      },
    },
  },
})
