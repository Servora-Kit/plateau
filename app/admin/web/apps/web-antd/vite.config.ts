import process from 'node:process';

import { defineConfig } from '@vben/vite-config';

const backendOrigin =
  process.env.VITE_ADMIN_BACKEND_ORIGIN || 'http://127.0.0.1:10010';

export default defineConfig(async () => {
  return {
    application: {},
    vite: {
      preview: {
        port: 10_012,
        strictPort: true,
        proxy: {
          '/v1': {
            changeOrigin: true,
            target: backendOrigin,
          },
        },
      },
      resolve: {
        dedupe: ['@servora/proto-utils'],
      },
      server: {
        port: 10_012,
        strictPort: true,
        proxy: {
          '/v1': {
            changeOrigin: true,
            target: backendOrigin,
          },
        },
      },
    },
  };
});
