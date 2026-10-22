import tailwindcss from '@tailwindcss/vite';
import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';
import { VitePWA } from 'vite-plugin-pwa';

const apiTarget = process.env.ARENILE_DEV_API ?? 'http://localhost:8080';

export default defineConfig({
  base: '/admin/',
  plugins: [
    react(),
    tailwindcss(),
    VitePWA({
      registerType: 'autoUpdate',
      injectRegister: 'script-defer',
      includeAssets: ['icon.svg'],
      manifest: {
        name: 'Arenile – Pannello del gestore',
        short_name: 'Arenile',
        description: 'Gestionale gratuito per stabilimenti balneari',
        lang: 'it',
        start_url: '/admin/',
        scope: '/admin/',
        display: 'standalone',
        background_color: '#ffffff',
        theme_color: '#0e6ba8',
        icons: [{ src: 'icon.svg', sizes: 'any', type: 'image/svg+xml', purpose: 'any' }],
      },
      workbox: {
        // Shell and static assets only; API data is never cached by the SW.
        globPatterns: ['**/*.{js,css,html,svg,woff2}'],
        navigateFallback: '/admin/index.html',
        navigateFallbackAllowlist: [/^\/admin\//],
        navigateFallbackDenylist: [/^\/api\//],
        cleanupOutdatedCaches: true,
      },
    }),
  ],
  server: {
    port: 5173,
    proxy: {
      '/api': { target: apiTarget, changeOrigin: false },
      '/healthz': apiTarget,
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    sourcemap: true,
    // The panel has no weight budget (spec 3.3); only the booking page does.
    chunkSizeWarningLimit: 1500,
  },
});
