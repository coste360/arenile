import { defineConfig, devices } from '@playwright/test';
import { mkdtempSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { DEMO_OWNER_PASSWORD } from './tests/helpers';

/**
 * End-to-end tests against a real `arenile` binary.
 *
 * The web server is started from the repository root with a fresh temporary
 * ARENILE_DATA_DIR (SQLite), seeded with the demo establishment `lido-demo`.
 * The SPAs are served from the embedded static files, so build and copy the
 * frontend first (e.g. `make build`) to test the real pages.
 *
 * Set E2E_BASE_URL to run against an already running instance instead.
 */
const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(here, '..');
const port = Number(process.env.E2E_PORT ?? 8090);
const externalBaseURL = process.env.E2E_BASE_URL;
const baseURL = externalBaseURL ?? `http://localhost:${port}`;

// Evaluated by the runner and by each worker: keep one directory per run.
process.env.ARENILE_E2E_DATA_DIR ??= mkdtempSync(join(tmpdir(), 'arenile-e2e-'));
const dataDir = process.env.ARENILE_E2E_DATA_DIR;

export default defineConfig({
  testDir: './tests',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [['github'], ['html', { open: 'never' }]] : 'list',
  use: {
    baseURL,
    trace: 'retain-on-failure',
    locale: 'it-IT',
  },
  projects: [
    { name: 'chromium', use: { ...devices['Desktop Chrome'] } },
    { name: 'mobile', use: { ...devices['Pixel 7'] }, testMatch: /booking\.spec\.ts/ },
  ],
  webServer: externalBaseURL
    ? undefined
    : {
        command:
          'go run ./cmd/arenile migrate up && go run ./cmd/arenile seed demo && go run ./cmd/arenile serve',
        cwd: repoRoot,
        url: `${baseURL}/healthz`,
        reuseExistingServer: !process.env.CI,
        timeout: 180_000,
        stdout: 'pipe',
        stderr: 'pipe',
        env: {
          ARENILE_ADDR: `:${port}`,
          ARENILE_BASE_URL: baseURL,
          ARENILE_DATA_DIR: dataDir,
          ARENILE_AUTO_MIGRATE: 'true',
          // Fixed test-only key (32 zero bytes, base64): never use in production.
          ARENILE_SECRET_KEY: 'AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=',
          ARENILE_LOG_LEVEL: 'warn',
          ARENILE_TELEMETRY: 'false',
          // Outgoing webhooks may reach the local test receiver (spec 22: off in production).
          ARENILE_WEBHOOKS_ALLOW_PRIVATE: 'true',
          // Password of the demo owner created by `seed demo` (test-only value).
          ARENILE_ADMIN_PASSWORD: DEMO_OWNER_PASSWORD,
        },
      },
});
