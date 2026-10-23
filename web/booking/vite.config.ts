import tailwindcss from '@tailwindcss/vite';
import react from '@vitejs/plugin-react';
import { defineConfig, type Plugin } from 'vite';

const apiTarget = process.env.ARENILE_DEV_API ?? 'http://localhost:8080';

/**
 * shadcn components the booking page never renders. Tailwind scans every
 * file under shared/src (`@source '../'` in the theme), so without this the
 * public page would ship the utilities of the admin shell (sidebar, charts,
 * data tables...) in its render-blocking CSS. This only trims the scan: no
 * CSS is written here (ADR 0009). `button-group` stays in: the zoom controls
 * of the seat map use it (the slider is hidden on the public page);
 * `radio-group` and `textarea` too: the renewal proposal of the manage area
 * uses them (M5B).
 */
const ADMIN_ONLY_UI = [
  'accordion',
  'avatar',
  'breadcrumb',
  'chart',
  'command',
  'context-menu',
  'dialog',
  'drawer',
  'dropdown-menu',
  'hover-card',
  'input-group',
  'kbd',
  'pagination',
  'progress',
  'resizable',
  'scroll-area',
  'select',
  'sheet',
  'sidebar',
  'slider',
  'sonner',
  'switch',
  'table',
  'tabs',
  'toggle',
  'toggle-group',
  'tooltip',
];

function trimTailwindSources(): Plugin {
  return {
    name: 'arenile-booking-tailwind-sources',
    enforce: 'pre',
    transform(code, id) {
      if (!id.endsWith('/shared/src/styles/globals.css')) return null;
      const ignores = [
        ...ADMIN_ONLY_UI.map((c) => `@source not '../ui/${c}.tsx';`),
        "@source not '../ui/patterns/DataTable.tsx';",
        "@source not '../ui/patterns/I18nTextInput.tsx';",
        "@source not '../../../admin';",
      ];
      return { code: `${code}\n${ignores.join('\n')}\n`, map: null };
    },
  };
}

export default defineConfig({
  base: '/b/',
  plugins: [trimTailwindSources(), react(), tailwindcss()],
  server: {
    port: 5174,
    proxy: {
      '/api': { target: apiTarget, changeOrigin: false },
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    sourcemap: true,
    target: 'es2022',
    // Read by scripts/budget.mjs to follow the import graph of the first screen.
    manifest: true,
  },
});
