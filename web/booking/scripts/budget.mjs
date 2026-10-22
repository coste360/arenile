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

const KB = 1024;
const INITIAL_BUDGET = 150 * KB;
const CSS_BUDGET = 25 * KB;
const EMBED_BUDGET = 3 * KB;
const ENTRY = 'index.html';

/**
 * Modules the first screen loads through `import()` before any interaction
 * (none today: the "Quando" step and the language picker are static imports;
 * the calendar loads when the date field opens, the map with its step).
 * List a source path here if a lazy module becomes part of the first paint.
 */
const STARTUP_DYNAMIC = [];

const dist = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const gz = (file) => gzipSync(readFileSync(file), { level: 9 }).length;
const fmt = (n) => `${(n / KB).toFixed(1)} KB`;

function fail(msg) {
  console.error(`budget: ${msg}`);
  process.exitCode = 1;
}

const manifestPath = join(dist, '.vite', 'manifest.json');
if (!existsSync(manifestPath)) {
  console.error('budget: dist/.vite/manifest.json not found, run the build first');
  process.exit(1);
}
const manifest = JSON.parse(readFileSync(manifestPath, 'utf8'));
const entry = manifest[ENTRY];
if (!entry) fail(`no "${ENTRY}" entry in the manifest`);

/** Static import closure of a manifest key (the chunk itself and its preloads). */
function staticClosure(key, seen = new Set()) {
  if (seen.has(key) || !manifest[key]) return seen;
  seen.add(key);
  for (const dep of manifest[key].imports ?? []) staticClosure(dep, seen);
  return seen;
}

const keys = staticClosure(ENTRY);
for (const src of STARTUP_DYNAMIC) {
  if (!manifest[src]) fail(`STARTUP_DYNAMIC "${src}" is not in the manifest`);
  else staticClosure(src, keys);
}

let js = 0;
let css = 0;
for (const key of keys) {
  const chunk = manifest[key];
  const size = gz(join(dist, chunk.file));
  js += size;
  console.log(`  js   ${fmt(size).padStart(9)}  ${chunk.file}  (${key})`);
  for (const sheet of chunk.css ?? []) {
    const cssSize = gz(join(dist, sheet));
    css += cssSize;
    console.log(`  css  ${fmt(cssSize).padStart(9)}  ${sheet}`);
  }
}
const lazy = Object.values(manifest)
  .filter((c) => !keys.has(c.src ?? '') && !c.isEntry && c.file.endsWith('.js'))
  .map((c) => `${c.file} ${fmt(gz(join(dist, c.file)))}`);
if (lazy.length) console.log(`  lazy (not counted): ${lazy.join(', ')}`);

console.log(`first-screen JS (gzip): ${fmt(js)} / ${fmt(INITIAL_BUDGET)}`);
if (js > INITIAL_BUDGET) fail(`first-screen JS ${fmt(js)} exceeds ${fmt(INITIAL_BUDGET)}`);
console.log(`render-blocking CSS (gzip): ${fmt(css)} / ${fmt(CSS_BUDGET)}`);
if (css > CSS_BUDGET) fail(`CSS ${fmt(css)} exceeds ${fmt(CSS_BUDGET)}`);

const embedPath = join(dist, 'embed.js');
if (!existsSync(embedPath)) {
  fail('dist/embed.js not found');
} else {
  const size = gz(embedPath);
  console.log(`embed.js (gzip): ${fmt(size)} / ${fmt(EMBED_BUDGET)} (${size} bytes)`);
  if (size > EMBED_BUDGET) fail(`embed.js ${fmt(size)} exceeds ${fmt(EMBED_BUDGET)}`);
}

if (!process.exitCode) console.log('budget: ok');
