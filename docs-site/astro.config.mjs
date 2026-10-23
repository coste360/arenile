// @ts-check
// Documentation site of Arenile (spec 28, M4): Astro Starlight, Italian as
// the source (root) locale and English, built-in search (Pagefind), real
// interactive components imported from web/shared (no duplication), API
// reference generated from api/openapi.yaml at build time (starlight-openapi),
// internal links validated on every build (starlight-links-validator).
//
// Environment (all optional):
//   ARENILE_DOCS_SITE    canonical origin (default https://docs.arenile.app)
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

const site = process.env.ARENILE_DOCS_SITE ?? 'https://docs.arenile.app';
const base = process.env.ARENILE_DOCS_BASE ?? '/';
/** Path of a file in public/ under the base path. @param {string} file */
const publicPath = (file) => `${base.replace(/\/?$/, '/')}${file}`;
/** Absolute URL of a file in public/ (social images need absolute URLs). @param {string} file */
const publicUrl = (file) => new URL(publicPath(file), site).href;

export default defineConfig({
  site,
  base,
  trailingSlash: 'ignore',
  integrations: [
    starlight({
      title: 'Arenile',
      description:
        'Gestionale per stabilimenti balneari, codice sorgente disponibile (BSL 1.1), di Coste360 SRL.',
      // Official logo (assets/brand/arenile/outlined, text as outlines so it
      // needs no web font), cropped to its content; light and dark variants.
      logo: {
        light: './src/assets/brand/arenile-logo-horizontal.svg',
        dark: './src/assets/brand/arenile-logo-horizontal-dark.svg',
        alt: 'Arenile — Coste360.it',
        replacesTitle: true,
      },
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
        { tag: 'meta', attrs: { name: 'theme-color', content: '#092D62' } },
        { tag: 'meta', attrs: { property: 'og:site_name', content: 'Arenile' } },
        { tag: 'meta', attrs: { property: 'og:image', content: publicUrl('og.png') } },
        { tag: 'meta', attrs: { property: 'og:image:width', content: '1200' } },
        { tag: 'meta', attrs: { property: 'og:image:height', content: '630' } },
        { tag: 'meta', attrs: { property: 'og:image:alt', content: 'Arenile — Coste360.it' } },
        { tag: 'meta', attrs: { name: 'twitter:card', content: 'summary_large_image' } },
        { tag: 'meta', attrs: { name: 'twitter:image', content: publicUrl('og.png') } },
        // PNG fallbacks of the SVG favicon (Safari, older browsers, home screen).
        {
          tag: 'link',
          attrs: {
            rel: 'icon',
            type: 'image/png',
            sizes: '32x32',
            href: publicPath('favicon-32x32.png'),
          },
        },
        { tag: 'link', attrs: { rel: 'apple-touch-icon', href: publicPath('apple-touch-icon.png') } },
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
