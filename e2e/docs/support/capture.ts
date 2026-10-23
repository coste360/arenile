import type { Locator, Page } from "@playwright/test";
import { mkdirSync, writeFileSync } from "node:fs";
import { createRequire } from "node:module";
import { join } from "node:path";
import { DOCS_ROOT, SCREENSHOTS_DIR } from "./env";
import type { Theme } from "./manifest";

/**
 * sharp is already installed for the docs (Astro's image service): reuse it
 * from docs-site/node_modules instead of adding a dependency to e2e/.
 */
type Sharp = (input: Buffer) => {
  webp(o: { quality: number; effort: number; smartSubsample?: boolean }): { toBuffer(): Promise<Buffer> };
};
let sharp: Sharp | undefined;
function loadSharp(): Sharp {
  if (!sharp) sharp = createRequire(join(DOCS_ROOT, "package.json"))("sharp") as Sharp;
  return sharp;
}

/** Colour scheme for both apps: OS preference + the panel's stored choice. */
export async function applyTheme(page: Page, theme: Theme): Promise<void> {
  await page.emulateMedia({ colorScheme: theme, reducedMotion: "reduce" });
  await page.addInitScript((t) => {
    try {
      window.localStorage.setItem("arenile-theme", t);
    } catch {
      /* storage disabled: the media emulation still applies */
    }
  }, theme);
}

/**
 * Freezes the browser clock on the demo operating date (09:30 local time):
 * "today" in the panel, calendars and countdowns are identical on every run.
 * Timers keep running, so the apps behave normally.
 */
export async function freezeClock(page: Page, date: string): Promise<void> {
  await page.clock.setFixedTime(new Date(`${date}T09:30:00+02:00`));
}

/** Waits for a quiet, fully painted page: network idle, fonts, no spinners. */
export async function settle(page: Page): Promise<void> {
  // The panel keeps an SSE stream open, so "networkidle" may never come:
  // wait for it a bounded time, then rely on the busy indicators below.
  await page.waitForLoadState("networkidle", { timeout: 4_000 }).catch(() => undefined);
  await page.evaluate(() => document.fonts.ready.then(() => undefined));
  const busy = page.locator('[aria-busy="true"], [data-slot="skeleton"], [role="progressbar"][data-state="indeterminate"]');
  await busy.first().waitFor({ state: "detached", timeout: 10_000 }).catch(() => undefined);
  // Two animation frames: layout and lazy images settle.
  await page.evaluate(
    () => new Promise<void>((r) => requestAnimationFrame(() => requestAnimationFrame(() => r()))),
  );
}

/** Elements whose content changes between runs (instants, relative times, tokens). */
export function volatile(page: Page): Locator[] {
  return [
    page.locator('time[datetime*="T"]'),
    page.locator("[data-docs-mask]"),
    page.locator('[data-slot="sonner-toaster"], [data-sonner-toaster]'),
  ];
}

const MASK_COLOR: Record<Theme, string> = { light: "#eef1f4", dark: "#262c35" };

/** Takes the screenshot and writes it as optimised WebP. Returns the file path. */
export async function capture(page: Page, name: string, theme: Theme, target?: Locator): Promise<string> {
  const png = await (target ?? page).screenshot({
    animations: "disabled",
    caret: "hide",
    scale: "device",
    mask: volatile(page),
    maskColor: MASK_COLOR[theme],
    ...(target ? {} : { fullPage: false }),
  });
  const webp = await loadSharp()(png).webp({ quality: 82, effort: 6, smartSubsample: true }).toBuffer();
  const dir = join(SCREENSHOTS_DIR, theme);
  mkdirSync(dir, { recursive: true });
  const file = join(dir, `${name}.webp`);
  writeFileSync(file, webp);
  return file;
}
