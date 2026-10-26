import { defineConfig } from "@playwright/test";
import { existsSync, mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { DOCS_OWNER_PASSWORD } from "./support/env";

/**
 * Documentation screenshots (spec 28, M4): a dedicated Playwright run, kept
 * apart from the end-to-end suite (e2e/tests), against the REAL `arenile`
 * binary started on a fresh temporary SQLite data dir seeded with
 * `seed demo`.
 *
 *   make docs-screenshots              build SPAs + binary, then capture all
 *   DOCS_ONLY=admin-map make docs-screenshots   capture matching names only
 *
 * Projects:
 *   data     logs in, resolves the demo ids (establishment, map, booking...)
 *            and writes the data snapshots used by the live components
 *            (docs-site/src/data/*.json);
 *   capture  one test per manifest entry and theme (light, dark), WebP
 *            output in docs-site/src/assets/screenshots/{light,dark}/.
 *
 * Environment: ARENILE_DOCS_BIN (binary, default bin/arenile),
 * DOCS_BASE_URL (use a running instance instead), DOCS_PORT (default 8091),
 * DOCS_DATE (operating date, default: a week from today, inside the demo season).
 */
const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(here, "..", "..");
const port = Number(process.env.DOCS_PORT ?? 8091);
const external = process.env.DOCS_BASE_URL;
const baseURL = external ?? `http://localhost:${port}`;
const bin = resolve(process.env.ARENILE_DOCS_BIN ?? join(repoRoot, "bin", "arenile"));

if (!external && !existsSync(bin)) {
  throw new Error(
    `docs screenshots: ${bin} not found. Run \`make docs-screenshots\` (it builds the binary) or set ARENILE_DOCS_BIN.`,
  );
}

// Evaluated by the runner and by every worker: one directory per run.
process.env.ARENILE_DOCS_DATA_DIR ??= mkdtempSync(join(tmpdir(), "arenile-docs-"));
const dataDir = process.env.ARENILE_DOCS_DATA_DIR;

export default defineConfig({
  testDir: ".",
  outputDir: join(dataDir, "test-results"),
  fullyParallel: true,
  workers: process.env.CI ? 2 : 4,
  forbidOnly: !!process.env.CI,
  retries: 1,
  timeout: 60_000,
  expect: { timeout: 15_000 },
  reporter: process.env.CI ? [["github"], ["list"]] : "list",
  use: {
    baseURL,
    locale: "it-IT",
    timezoneId: "Europe/Rome",
    trace: "retain-on-failure",
    // Chromium with a fixed font rendering: same pixels on every machine.
    launchOptions: { args: ["--font-render-hinting=none", "--disable-lcd-text"] },
  },
  projects: [
    { name: "data", testMatch: /data\.setup\.ts$/ },
    { name: "capture", testMatch: /capture\.spec\.ts$/, dependencies: ["data"] },
  ],
  webServer: external
    ? undefined
    : {
        command: `"${bin}" migrate up && "${bin}" seed demo && "${bin}" serve`,
        cwd: repoRoot,
        url: `${baseURL}/healthz`,
        reuseExistingServer: false,
        timeout: 120_000,
        stdout: "pipe",
        stderr: "pipe",
        env: {
          ARENILE_ADDR: `:${port}`,
          ARENILE_BASE_URL: baseURL,
          ARENILE_DATA_DIR: dataDir,
          ARENILE_AUTO_MIGRATE: "true",
          // Fixed docs-only key (32 zero bytes, base64): never use in production.
          ARENILE_SECRET_KEY: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
          ARENILE_LOG_LEVEL: "warn",
          ARENILE_TELEMETRY: "false",
          // No update check (ADR 0025): screenshots never show the update banner.
          ARENILE_UPDATE_CHECK: "false",
          // Emails of the demo flows go to files, never to the network.
          ARENILE_MAIL_DIR: join(dataDir, "mail"),
          // Parallel captures from one address (spec 17.2 limits are per IP).
          ARENILE_PUBLIC_READS_PER_MINUTE: "100000",
          ARENILE_PUBLIC_CREATIONS_PER_MINUTE: "100000",
          ARENILE_ADMIN_PASSWORD: DOCS_OWNER_PASSWORD,
        },
      },
});
