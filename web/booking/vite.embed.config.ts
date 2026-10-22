import { defineConfig } from 'vite';

/** Builds the tiny host-page loader served at /embed.js (spec 12.1). */
export default defineConfig({
  publicDir: false,
  build: {
    outDir: 'dist',
    emptyOutDir: false,
    sourcemap: false,
    target: 'es2017',
    minify: true,
    lib: {
      entry: 'src/embed/embed.ts',
      name: 'ArenileEmbed',
      formats: ['iife'],
      fileName: () => 'embed.js',
    },
  },
});
