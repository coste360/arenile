import { defineConfig, devices } from "@playwright/test";

/**
 * M6 acceptance against a running desktop app (spec 28, "Edizione desktop"):
 * not part of `make e2e`, because it needs a fresh desktop data folder.
 *
 *   ARENILE_UPDATE_CHECK=false arenile-desktop -data-dir "$TMP/arenile" -no-browser &
 *   DESKTOP_OPEN_URL="$(jq -r .open_url "$TMP/arenile/instance.json")" \
 *     npx playwright test -c playwright.desktop.config.ts
 *
 * See docs/desktop-build.md ("Collaudo M6").
 */
const openURL = process.env.DESKTOP_OPEN_URL ?? "";

export default defineConfig({
  testDir: "./desktop",
  fullyParallel: false,
  workers: 1,
  reporter: "list",
  use: {
    baseURL: openURL ? new URL(openURL).origin : undefined,
    trace: "retain-on-failure",
    locale: "it-IT",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});
