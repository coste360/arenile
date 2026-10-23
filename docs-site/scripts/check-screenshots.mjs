#!/usr/bin/env node
// Every screenshot of screenshots.manifest.json must be shown by at least one
// page (<Screenshot name="..."> in src/content): a captured image nobody
// references is dead weight shipped with the site, and usually a page that
// forgot its picture. Runs before the build (npm run build) and alone
// (node scripts/check-screenshots.mjs).
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = join(dirname(fileURLToPath(import.meta.url)), '..');
const manifest = JSON.parse(readFileSync(join(root, 'screenshots.manifest.json'), 'utf8'));

function* walk(dir) {
  for (const entry of readdirSync(dir)) {
    const p = join(dir, entry);
    if (statSync(p).isDirectory()) yield* walk(p);
    else if (/\.(mdx?|astro)$/.test(entry)) yield p;
  }
}

const referenced = new Set();
for (const file of walk(join(root, 'src'))) {
  for (const m of readFileSync(file, 'utf8').matchAll(/<Screenshot\s+name="([^"$]+)"/g)) referenced.add(m[1]);
}
const known = new Set(manifest.screenshots.map((s) => s.name));
const unreferenced = [...known].filter((n) => !referenced.has(n));
const unknown = [...referenced].filter((n) => !known.has(n));
if (unknown.length) console.error(`check-screenshots: unknown names used in pages: ${unknown.join(', ')}`);
if (unreferenced.length) console.error(`check-screenshots: captured but never shown: ${unreferenced.join(', ')}`);
if (unknown.length || unreferenced.length) process.exit(1);
console.log(`check-screenshots: ${known.size} screenshots, all referenced`);
