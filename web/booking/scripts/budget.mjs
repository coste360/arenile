#!/usr/bin/env node
// Bundle budget for the public booking page (spec 3.3, 12.6):
// - JS needed for the first screen < 150 KB gzip: the entry chunk, every
//   chunk it imports statically (preloads) and the dynamic imports that the
//   first screen still triggers at startup (STARTUP_DYNAMIC), each followed
//   through the Vite manifest;
// - render-blocking CSS < 25 KB gzip (the theme must not carry admin-only utilities);
// - embed.js < 3 KB gzip.
import { existsSync, readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { gzipSync } from 'node:zlib';

export const KB = 1024;
export const INITIAL_BUDGET = 150 * KB;
export const CSS_BUDGET = 25 * KB;
export const EMBED_BUDGET = 3 * KB;
const ENTRY = 'index.html';
const fmt = (n) => `${(n / KB).toFixed(1)} KB`;

/**
 * Modules the first screen loads through `import()` before any interaction
 * (none today: the "Quando" step and the language picker are static imports;
 * the calendar loads when the date field opens, the map with its step).
 * List a source path here if a lazy module becomes part of the first paint.
 */
const STARTUP_DYNAMIC = [];

/** Static import closure of a manifest key (the chunk itself and its preloads). */
export function staticClosure(manifest, key, seen = new Set()) {
  if (seen.has(key) || !manifest[key]) return seen;
  seen.add(key);
  for (const dep of manifest[key].imports ?? []) staticClosure(manifest, dep, seen);
  return seen;
}

/**
 * Measures a build: first-screen JS (entry + static imports + STARTUP_DYNAMIC
 * closures), its render-blocking CSS and embed.js, all gzip level 9.
 * `size(file)` returns the gzip size of a dist-relative file (injectable for tests).
 */
export function measure(manifest, size, { entry = ENTRY, startupDynamic = STARTUP_DYNAMIC } = {}) {
  const errors = [];
  const rows = [];
  if (!manifest[entry]) errors.push(`no "${entry}" entry in the manifest`);
  const keys = staticClosure(manifest, entry);
  for (const src of startupDynamic) {
    if (!manifest[src]) errors.push(`STARTUP_DYNAMIC "${src}" is not in the manifest`);
    else staticClosure(manifest, src, keys);
  }
  let js = 0;
  let css = 0;
  for (const key of keys) {
    const chunk = manifest[key];
    const s = size(chunk.file);
    js += s;
    rows.push({ kind: 'js', size: s, file: chunk.file, key });
    for (const sheet of chunk.css ?? []) {
      const c = size(sheet);
      css += c;
      rows.push({ kind: 'css', size: c, file: sheet, key });
    }
  }
  const lazy = Object.values(manifest)
    .filter((c) => !keys.has(c.src ?? '') && !c.isEntry && c.file.endsWith('.js'))
    .map((c) => ({ file: c.file, size: size(c.file) }));
  const embed = size('embed.js');
  if (js > INITIAL_BUDGET) errors.push(`first-screen JS ${fmt(js)} exceeds ${fmt(INITIAL_BUDGET)}`);
  if (css > CSS_BUDGET) errors.push(`CSS ${fmt(css)} exceeds ${fmt(CSS_BUDGET)}`);
  if (embed === null) errors.push('dist/embed.js not found');
  else if (embed > EMBED_BUDGET) errors.push(`embed.js ${fmt(embed)} exceeds ${fmt(EMBED_BUDGET)}`);
  return { js, css, embed, rows, lazy, errors };
}

function main() {
  const dist = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
  const manifestPath = join(dist, '.vite', 'manifest.json');
  if (!existsSync(manifestPath)) {
    console.error('budget: dist/.vite/manifest.json not found, run the build first');
    process.exit(1);
  }
  const manifest = JSON.parse(readFileSync(manifestPath, 'utf8'));
  const size = (file) => {
    const path = join(dist, file);
    return existsSync(path) ? gzipSync(readFileSync(path), { level: 9 }).length : null;
  };
  const r = measure(manifest, size);
  for (const row of r.rows) {
    console.log(`  ${row.kind.padEnd(4)} ${fmt(row.size).padStart(9)}  ${row.file}  (${row.key})`);
  }
  if (r.lazy.length) {
    console.log(
      `  lazy (not counted): ${r.lazy.map((l) => `${l.file} ${fmt(l.size)}`).join(', ')}`,
    );
  }
  console.log(`first-screen JS (gzip): ${fmt(r.js)} / ${fmt(INITIAL_BUDGET)}`);
  console.log(`render-blocking CSS (gzip): ${fmt(r.css)} / ${fmt(CSS_BUDGET)}`);
  if (r.embed !== null) {
    console.log(`embed.js (gzip): ${fmt(r.embed)} / ${fmt(EMBED_BUDGET)} (${r.embed} bytes)`);
  }
  for (const e of r.errors) console.error(`budget: ${e}`);
  if (r.errors.length) process.exitCode = 1;
  else console.log('budget: ok');
}

if (process.argv[1] && fileURLToPath(import.meta.url) === process.argv[1]) main();
