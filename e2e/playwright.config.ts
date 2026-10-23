import { defineConfig, devices } from '@playwright/test';
import { mkdtempSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { DEMO_OWNER_PASSWORD, hostPort, stripePort } from './tests/helpers';

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
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
      // The M4 acceptance is a smartphone scenario (spec 28); M5B has its own project.
      testIgnore: /(m4-acceptance|m5b)\.spec\.ts/,
    },
    {
      name: 'mobile',
      use: { ...devices['Pixel 7'] },
      testMatch: /(booking|m4-acceptance)\.spec\.ts/,
    },
    // M5 part B (season rollover, renewals, waitlist, credits): after the
    // other projects, because it creates the next season and switches the
    // payments to Stripe.
    {
      name: 'm5b',
      use: { ...devices['Desktop Chrome'] },
      testMatch: /m5b\.spec\.ts/,
      dependencies: ['chromium', 'mobile'],
    },
    // iOS Safari is the main phone browser on Italian beaches (spec 13.2):
    // cross-site iframes, partitioned storage and top navigation from a
    // frame behave differently in WebKit. Needs `playwright install webkit`
    // (CI does; locally set E2E_WEBKIT=1).
    ...(process.env.CI || process.env.E2E_WEBKIT
      ? [
          {
            name: 'mobile-safari',
            use: { ...devices['iPhone 14'] },
            testMatch: /(booking|m4-acceptance)\.spec\.ts/,
          },
        ]
      : []),
  ],
  webServer: externalBaseURL
    ? undefined
    : [
        {
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
            // Emails as .eml files (magic link of the manage area).
            ARENILE_MAIL_DIR: join(dataDir, 'mail'),
            // Stripe test double (never in production).
            ARENILE_STRIPE_API_URL: `http://127.0.0.1:${stripePort}`,
            // Parallel workers from one address (spec 17.2 limits are per IP).
            ARENILE_PUBLIC_READS_PER_MINUTE: '100000',
            ARENILE_PUBLIC_CREATIONS_PER_MINUTE: '100000',
          },
        },
        {
          command: `go run ./internal/testtools/stripefake -addr 127.0.0.1:${stripePort} -arenile ${baseURL}`,
          cwd: repoRoot,
          url: `http://127.0.0.1:${stripePort}/healthz`,
          // Never reuse: a stray fake bound to another instance would
          // swallow the webhooks.
          reuseExistingServer: false,
          timeout: 180_000,
        },
        {
          command: 'node fixtures/host-server.mjs',
          cwd: here,
          url: `http://127.0.0.1:${hostPort}/healthz`,
          reuseExistingServer: false,
          env: { E2E_HOST_PORT: String(hostPort), ARENILE_URL: baseURL },
        },
      ],
});
