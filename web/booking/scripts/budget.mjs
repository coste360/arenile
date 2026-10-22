#!/usr/bin/env node
// Bundle budget for the public booking page (spec 3.3, 12.6):
// - initial JS (entry module + modulepreloads referenced by index.html) < 150 KB gzip
// - embed.js < 3 KB gzip
import { existsSync, readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { gzipSync } from 'node:zlib';

const KB = 1024;
const INITIAL_BUDGET = 150 * KB;
const EMBED_BUDGET = 3 * KB;
const BASE = '/b/';

const dist = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const gz = (file) => gzipSync(readFileSync(file), { level: 9 }).length;
const fmt = (n) => `${(n / KB).toFixed(1)} KB`;

function fail(msg) {
  console.error(`budget: ${msg}`);
  process.exitCode = 1;
}

const indexPath = join(dist, 'index.html');
if (!existsSync(indexPath)) {
  console.error('budget: dist/index.html not found, run the build first');
  process.exit(1);
}
const html = readFileSync(indexPath, 'utf8');
const refs = new Set();
for (const m of html.matchAll(/<script[^>]+type="module"[^>]+src="([^"]+)"/g)) refs.add(m[1]);
for (const m of html.matchAll(/<link[^>]+rel="modulepreload"[^>]+href="([^"]+)"/g)) refs.add(m[1]);
if (refs.size === 0) fail('no initial scripts found in index.html');

let total = 0;
for (const ref of refs) {
  const rel = ref.startsWith(BASE) ? ref.slice(BASE.length) : ref.replace(/^\//, '');
  const size = gz(join(dist, rel));
  total += size;
  console.log(`  initial  ${fmt(size).padStart(9)}  ${rel}`);
}
console.log(`initial JS (gzip): ${fmt(total)} / ${fmt(INITIAL_BUDGET)}`);
if (total > INITIAL_BUDGET) fail(`initial JS ${fmt(total)} exceeds ${fmt(INITIAL_BUDGET)}`);

const embedPath = join(dist, 'embed.js');
if (!existsSync(embedPath)) {
  fail('dist/embed.js not found');
} else {
  const size = gz(embedPath);
  console.log(`embed.js (gzip): ${fmt(size)} / ${fmt(EMBED_BUDGET)} (${size} bytes)`);
  if (size > EMBED_BUDGET) fail(`embed.js ${fmt(size)} exceeds ${fmt(EMBED_BUDGET)}`);
}

if (!process.exitCode) console.log('budget: ok');
