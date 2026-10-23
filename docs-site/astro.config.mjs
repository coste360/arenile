// @ts-check
// Documentation site of Arenile (spec 28, M4): Astro Starlight, Italian as
// the source (root) locale and English, built-in search (Pagefind), real
// interactive components imported from web/shared (no duplication), API
// reference generated from api/openapi.yaml at build time (starlight-openapi),
// internal links validated on every build (starlight-links-validator).
//
// Environment (all optional):
//   ARENILE_DOCS_SITE    canonical origin (default https://docs.arenile.it)
//   ARENILE_DOCS_BASE    base path, e.g. /arenile/ for GitHub Pages (default /)
//   ARENILE_DOCS_STRICT  1 = a missing screenshot fails the build (CI, releases)
import react from '@astrojs/react';
import starlight from '@astrojs/starlight';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'astro/config';
import { fileURLToPath } from 'node:url';
import starlightLinksValidator from 'starlight-links-validator';
import starlightOpenAPI, { openAPISidebarGroups } from 'starlight-openapi';
import { API_REFERENCE_BASE } from './src/lib/constants.mjs';

const shared = fileURLToPath(new URL('../web/shared/src', import.meta.url));
const src = fileURLToPath(new URL('./src', import.meta.url));
const webModules = fileURLToPath(new URL('../web/node_modules', import.meta.url));

export default defineConfig({
  site: process.env.ARENILE_DOCS_SITE ?? 'https://docs.arenile.it',
  base: process.env.ARENILE_DOCS_BASE ?? '/',
  trailingSlash: 'ignore',
  integrations: [
    starlight({
      title: 'Arenile',
      description:
        'Gestionale per stabilimenti balneari, codice sorgente disponibile (BSL 1.1), di Coste360 SRL.',
      // Same mark as the panel and the booking app (one source of truth).
      logo: { src: '../web/admin/public/icon.svg', alt: 'Arenile' },
      favicon: '/favicon.svg',
      defaultLocale: 'root',
      locales: {
        root: { label: 'Italiano', lang: 'it' },
        en: { label: 'English', lang: 'en' },
      },
      social: [{ icon: 'github', label: 'GitHub', href: 'https://github.com/coste360/arenile' }],
      editLink: { baseUrl: 'https://github.com/coste360/arenile/edit/main/docs-site/' },
      lastUpdated: true,
      credits: false,
      customCss: ['./src/styles/global.css'],
      // Keeps the 200+ API reference links out of the sidebar of the guide pages.
      routeMiddleware: './src/routeData.ts',
      head: [
        { tag: 'meta', attrs: { name: 'theme-color', content: '#0e6ba8' } },
        { tag: 'meta', attrs: { property: 'og:site_name', content: 'Arenile' } },
      ],
      components: {
        Footer: './src/components/overrides/Footer.astro',
      },
      plugins: [
        starlightOpenAPI([
          {
            base: API_REFERENCE_BASE,
            schema: '../api/openapi.yaml',
            sidebar: {
              label: 'Riferimento API',
              collapsed: true,
              operations: { badges: true, labels: 'summary', sort: 'document' },
            },
            snippets: {
              operation: {
                clients: { shell: ['curl'], javascript: ['fetch'], go: ['nethttp'] },
              },
            },
          },
        ]),
        starlightLinksValidator({
          // English pages may fall back to the Italian source while a
          // translation is pending: Starlight serves them with a notice.
          errorOnFallbackPages: false,
          // The API reference pages are generated routes, not content files.
          exclude: [`/${API_REFERENCE_BASE}/**`, `/en/${API_REFERENCE_BASE}/**`],
        }),
      ],
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
          items: [{ autogenerate: { directory: 'api' } }, ...openAPISidebarGroups],
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
      // Real components from web/shared (map with the kit, status badges,
      // shadcn primitives): one copy of React for the site and the components.
      alias: { '@arenile/shared': shared, '~': src },
      dedupe: ['react', 'react-dom', '@tanstack/react-query', 'i18next', 'react-i18next'],
    },
    // The shared sources import their runtime deps (radix-ui, lucide-react,
    // i18next, ...) from web/node_modules: allow Vite to serve them in dev.
    server: { fs: { allow: ['..', webModules] } },
    ssr: { noExternal: ['radix-ui', /^@radix-ui\//, 'lucide-react'] },
  },
});
