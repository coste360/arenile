// @ts-check
// Documentation site of Arenile (spec 28, M4): Astro Starlight, Italian as
// the source (root) locale and English, built-in search (Pagefind), real
// interactive components imported from web/shared (no duplication).
import react from '@astrojs/react';
import starlight from '@astrojs/starlight';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'astro/config';
import { fileURLToPath } from 'node:url';

const shared = fileURLToPath(new URL('../web/shared/src', import.meta.url));

export default defineConfig({
  site: process.env.ARENILE_DOCS_SITE ?? 'https://docs.arenile.it',
  base: process.env.ARENILE_DOCS_BASE ?? '/',
  integrations: [
    starlight({
      title: 'Arenile',
      description:
        'Gestionale per stabilimenti balneari, codice sorgente disponibile (BSL 1.1), di Coste360 SRL.',
      defaultLocale: 'root',
      locales: {
        root: { label: 'Italiano', lang: 'it' },
        en: { label: 'English', lang: 'en' },
      },
      social: [{ icon: 'github', label: 'GitHub', href: 'https://github.com/coste360/arenile' }],
      editLink: { baseUrl: 'https://github.com/coste360/arenile/edit/main/docs-site/' },
      lastUpdated: true,
      customCss: ['./src/styles/global.css'],
      sidebar: [
        {
          label: 'Guida del gestore',
          translations: { en: 'Manager guide' },
          items: [{ autogenerate: { directory: 'gestore' } }],
        },
        {
          label: 'Installazione e operatività',
          translations: { en: 'Installation and operations' },
          items: [{ autogenerate: { directory: 'installazione' } }],
        },
        {
          label: 'API e webhook',
          translations: { en: 'API and webhooks' },
          items: [{ autogenerate: { directory: 'api' } }],
        },
        {
          label: 'Sviluppo ed estensioni',
          translations: { en: 'Development and extensions' },
          items: [{ autogenerate: { directory: 'sviluppo' } }],
        },
      ],
    }),
    react(),
  ],
  vite: {
    plugins: [tailwindcss()],
    resolve: {
      // Real components from web/shared (map with the kit, price simulator,
      // status badges): one copy of React for the site and the components.
      alias: { '@arenile/shared': shared },
      dedupe: ['react', 'react-dom', '@tanstack/react-query', 'i18next', 'react-i18next'],
    },
    server: { fs: { allow: ['..'] } },
  },
});
