#!/usr/bin/env node
// Lighthouse budget of the public booking page (spec 12.6, 28 M4): mobile
// form factor with Lighthouse's default simulated throttling (slow 4G, 4x
// CPU), against the real binary seeded with the demo data.
//
//   node e2e/lighthouse/run.mjs            (make lighthouse; needs bin/arenile)
//   LH_BASE_URL=http://localhost:8080 ...  (an instance already running)
//
// Assertions: performance >= 90, accessibility >= 95, best practices >= 95,
// SEO >= 95 on the indexable page; the embed variant (?embed=1, noindex by
// design) is held to the same bar except SEO. Scores are written to
// e2e/lighthouse/results.json and printed; any miss exits 1.
import { spawn, spawnSync } from 'node:child_process';
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import * as chromeLauncher from 'chrome-launcher';
import lighthouse from 'lighthouse';
import { chromium } from 'playwright';

const here = dirname(fileURLToPath(import.meta.url));
const root = resolve(here, '../..');
const port = Number(process.env.LH_PORT ?? 8095);
const external = process.env.LH_BASE_URL;
const base = external ?? `http://localhost:${port}`;
const RUNS = Number(process.env.LH_RUNS ?? 3); // median of N runs (performance is noisy)

const TARGETS = [
  { name: 'public page', path: '/b/lido-demo', min: { performance: 90, accessibility: 95, 'best-practices': 95, seo: 95 } },
  { name: 'embedded widget', path: '/b/lido-demo?embed=1', min: { performance: 90, accessibility: 95, 'best-practices': 95 } },
];

async function waitHealthy(url, ms = 30_000) {
  const until = Date.now() + ms;
  while (Date.now() < until) {
    try {
      if ((await fetch(url)).ok) return;
    } catch {
      /* not yet */
    }
    await new Promise((r) => setTimeout(r, 250));
  }
  throw new Error(`instance not healthy at ${url}`);
}

function startInstance() {
  const dir = mkdtempSync(join(tmpdir(), 'arenile-lh-'));
  const env = {
    ...process.env,
    ARENILE_DATA_DIR: dir,
    ARENILE_ADDR: `127.0.0.1:${port}`,
    ARENILE_BASE_URL: base,
    ARENILE_SECRET_KEY: 'AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=', // test-only key
    ARENILE_TELEMETRY: 'false',
    ARENILE_LOG_LEVEL: 'warn',
    ARENILE_ADMIN_PASSWORD: 'Lighthouse-Owner-2026!',
  };
  const bin = join(root, 'bin/arenile');
  for (const args of [['migrate', 'up'], ['seed', 'demo']]) {
    const r = spawnSync(bin, args, { env, stdio: 'ignore' });
    if (r.status !== 0) throw new Error(`arenile ${args.join(' ')} failed (run make build)`);
  }
  const proc = spawn(bin, ['serve'], { env, stdio: 'ignore' });
  return {
    stop() {
      proc.kill('SIGTERM');
      rmSync(dir, { recursive: true, force: true });
    },
  };
}

const median = (xs) => [...xs].sort((a, b) => a - b)[Math.floor(xs.length / 2)];

const instance = external ? null : startInstance();
let chrome;
let failed = false;
const results = { date: new Date().toISOString(), base, runs: RUNS, targets: [] };
try {
  await waitHealthy(`${base}/healthz`);
  chrome = await chromeLauncher.launch({
    chromePath: chromium.executablePath(),
    chromeFlags: ['--headless=new', '--no-sandbox', '--disable-gpu'],
  });
  for (const t of TARGETS) {
    const runs = [];
    for (let i = 0; i < RUNS; i++) {
      const { lhr } = await lighthouse(`${base}${t.path}`, {
        port: chrome.port,
        output: 'json',
        logLevel: 'error',
        onlyCategories: Object.keys(t.min),
      });
      if (lhr.runtimeError) throw new Error(`${t.name}: ${lhr.runtimeError.message}`);
      runs.push(lhr);
    }
    const scores = {};
    for (const cat of Object.keys(t.min)) {
      scores[cat] = median(runs.map((r) => Math.round((r.categories[cat]?.score ?? 0) * 100)));
    }
    const last = runs.at(-1);
    const metrics = {
      fcp: last.audits['first-contentful-paint']?.displayValue,
      lcp: last.audits['largest-contentful-paint']?.displayValue,
      tbt: last.audits['total-blocking-time']?.displayValue,
      cls: last.audits['cumulative-layout-shift']?.displayValue,
    };
    const misses = Object.entries(t.min).filter(([cat, min]) => scores[cat] < min);
    for (const [cat] of misses) {
      const failing = Object.values(last.audits)
        .filter((a) => a.score !== null && a.score < 1 && last.categories[cat].auditRefs.some((r) => r.id === a.id && r.weight > 0))
        .map((a) => `${a.id} (${a.score})`);
      console.error(`  ${t.name} ${cat}: ${scores[cat]} < ${t.min[cat]}; failing audits: ${failing.join(', ')}`);
    }
    failed ||= misses.length > 0;
    results.targets.push({ name: t.name, path: t.path, scores, min: t.min, metrics });
    console.log(
      `${misses.length ? 'FAIL' : 'ok  '} ${t.name.padEnd(16)} ${Object.entries(scores)
        .map(([k, v]) => `${k} ${v}`)
        .join(' · ')}  (LCP ${metrics.lcp}, TBT ${metrics.tbt}, CLS ${metrics.cls})`,
    );
  }
} finally {
  await chrome?.kill();
  instance?.stop();
}
writeFileSync(join(here, 'results.json'), `${JSON.stringify(results, null, 2)}\n`);
process.exit(failed ? 1 : 0);
