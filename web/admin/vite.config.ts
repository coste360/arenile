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
    // Panel PWA (spec 21.2, ADR 0014): generateSW precaches the shell and the
    // static assets only. API responses are never cached by the service
    // worker (per user, personal data): offline data lives in IndexedDB,
    // written by the app (src/offline). registerType "prompt": a new version
    // waits for the user (src/pwa/UpdatePrompt), registered by src/pwa/register.
    VitePWA({
      registerType: 'prompt',
      injectRegister: false,
      includeAssets: ['icon.svg', 'apple-touch-icon-180x180.png'],
      manifest: {
        id: '/admin/',
        name: 'Arenile – Pannello del gestore',
        short_name: 'Arenile',
        description:
          'Pannello di gestione dello stabilimento balneare: mappa, prenotazioni, cassa.',
        lang: 'it',
        dir: 'ltr',
        start_url: '/admin/',
        scope: '/admin/',
        display: 'standalone',
        orientation: 'any',
        categories: ['business', 'productivity'],
        background_color: '#ffffff',
        theme_color: '#0e6ba8',
        icons: [
          { src: 'pwa-192x192.png', sizes: '192x192', type: 'image/png', purpose: 'any' },
          { src: 'pwa-512x512.png', sizes: '512x512', type: 'image/png', purpose: 'any' },
          {
            src: 'maskable-icon-512x512.png',
            sizes: '512x512',
            type: 'image/png',
            purpose: 'maskable',
          },
          { src: 'icon.svg', sizes: 'any', type: 'image/svg+xml', purpose: 'any' },
        ],
        shortcuts: [
          {
            name: 'Oggi',
            url: '/admin/oggi',
            icons: [{ src: 'pwa-192x192.png', sizes: '192x192' }],
          },
          {
            name: 'Mappa',
            url: '/admin/mappa',
            icons: [{ src: 'pwa-192x192.png', sizes: '192x192' }],
          },
        ],
      },
      workbox: {
        // Shell, lazy route chunks and locales (manifest icons and includeAssets
        // are added by the plugin); never /api/.
        globPatterns: ['**/*.{js,css,html,svg,woff2}'],
        // Sourcemaps are not needed offline.
        globIgnores: ['**/*.map'],
        navigateFallback: '/admin/index.html',
        navigateFallbackAllowlist: [/^\/admin\//],
        navigateFallbackDenylist: [/^\/api\//],
        cleanupOutdatedCaches: true,
        // No runtimeCaching on purpose: see the comment above.
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
