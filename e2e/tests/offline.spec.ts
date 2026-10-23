import { expect, test, type Page } from "@playwright/test";
import { login } from "./helpers";

/**
 * M5A panel PWA with limited offline (spec 21.2), in a real Chromium:
 * service worker precache, IndexedDB snapshot restored on a reload without
 * network, offline banner and indicator, cleanup on logout.
 */

const DB = "arenile-admin-offline";

async function openPanel(page: Page): Promise<string> {
  await login(page);
  const res = await page.request.get("/api/admin/v1/establishments");
  expect(res.ok()).toBeTruthy();
  const body = (await res.json()) as { items: { id: string; slug: string }[] };
  const demo = body.items.find((e) => e.slug === "lido-demo") ?? body.items[0];
  return `/admin/e/${demo!.id}`;
}

/** Query keys in the IndexedDB snapshot (empty when there is none). */
function snapshotKeys(page: Page): Promise<string[]> {
  return page.evaluate(
    (name) =>
      new Promise<string[]>((resolve) => {
        const open = indexedDB.open(name);
        open.onerror = () => resolve([]);
        open.onsuccess = () => {
          const db = open.result;
          if (!db.objectStoreNames.contains("cache")) return resolve([]);
          const get = db.transaction("cache").objectStore("cache").get("queries");
          get.onsuccess = () => {
            const snap = get.result as
              | { state: { queries: { queryKey: unknown[] }[] } }
              | undefined;
            resolve(snap ? snap.state.queries.map((q) => JSON.stringify(q.queryKey)) : []);
            db.close();
          };
          get.onerror = () => resolve([]);
        };
      }),
    DB,
  );
}

test("the panel manifest is installable with the Arenile icons", async ({ request }) => {
  const res = await request.get("/admin/manifest.webmanifest");
  expect(res.ok()).toBeTruthy();
  const manifest = (await res.json()) as {
    name: string;
    start_url: string;
    display: string;
    icons: { src: string; sizes: string; purpose?: string }[];
  };
  expect(manifest).toMatchObject({ start_url: "/admin/", display: "standalone" });
  expect(manifest.name).toContain("Arenile");
  const sizes = manifest.icons.map((i) => `${i.sizes}:${i.purpose ?? "any"}`);
  expect(sizes).toEqual(
    expect.arrayContaining(["192x192:any", "512x512:any", "512x512:maskable"]),
  );
  for (const icon of manifest.icons) {
    const img = await request.get(`/admin/${icon.src}`);
    expect(img.ok(), icon.src).toBeTruthy();
  }
});

test("starts offline from the saved copy and clears it on logout", async ({ page, context }) => {
  const base = await openPanel(page);
  await page.goto(`${base}/oggi`);
  await expect(page.getByRole("heading", { name: "Oggi", level: 1 })).toBeVisible();

  // The service worker installs on the first visit and controls the next load.
  await page.evaluate(() => navigator.serviceWorker.ready.then(() => undefined));
  await page.reload();
  await expect
    .poll(() => page.evaluate(() => !!navigator.serviceWorker.controller))
    .toBe(true);

  // The snapshot holds the user, the map and today's day; never customers or payments.
  await expect
    .poll(async () => (await snapshotKeys(page)).join("\n"), { timeout: 15_000 })
    .toMatch(/"operations","day"/);
  const keys = await snapshotKeys(page);
  expect(keys).toContain('["auth","me"]');
  expect(keys.some((k) => k.includes('"maps"'))).toBe(true);
  expect(keys.some((k) => /"customers"|"payments"|"team"/.test(k))).toBe(false);

  await context.setOffline(true);
  await page.reload();
  await expect(page.getByRole("heading", { name: "Oggi", level: 1 })).toBeVisible();
  const header = page.locator("header");
  await expect(header.getByRole("status")).toHaveText("Offline");
  await expect(page.getByText("Sei offline")).toBeVisible();
  await expect(page.getByText(/Stai consultando i dati salvati alle/)).toBeVisible();
  // The map of the day renders from the restored document and day (spec 21.2:
  // the cached map must be readable), with its spots; the takings, never
  // kept on the device, say so instead of showing a placeholder.
  await expect(page.getByRole("group", { name: /^Mappa con \d+ postazioni$/ })).toBeVisible();
  await expect(page.getByRole("button", { name: /^Postazione A1:/ })).toBeVisible();
  await expect(page.getByText("Non disponibile offline")).toBeVisible();

  // Client-side navigation works offline too (lazy chunks are precached).
  await page.goto(`${base}/mappa`);
  await expect(page.getByRole("heading", { name: "Mappa", level: 1 })).toBeVisible();

  await context.setOffline(false);
  await expect(header.getByRole("status")).toHaveText("Online");
  await expect(page.getByText("Sei offline")).toHaveCount(0);

  await page.getByRole("button", { name: /Menu utente/ }).click();
  await page.getByRole("menuitem", { name: "Esci" }).click();
  await expect(page).toHaveURL(/\/admin\/login/);
  await expect.poll(() => snapshotKeys(page)).toEqual([]);
});

test("the offline banner and indicator fit a 390px screen", async ({ page, context }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const base = await openPanel(page);
  await page.goto(`${base}/oggi`);
  await expect(page.getByRole("heading", { name: "Oggi", level: 1 })).toBeVisible();
  const overflow = () =>
    page.evaluate(
      () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
    );
  const before = await overflow();

  await context.setOffline(true);
  const banner = page.getByRole("alert").filter({ hasText: "Sei offline" });
  await expect(banner).toBeVisible();
  // The offline UI adds no horizontal scroll of its own.
  expect(await overflow()).toBeLessThanOrEqual(before);
  for (const el of [banner, page.locator("header").getByRole("status")]) {
    const box = await el.boundingBox();
    expect(box && box.x >= 0 && box.x + box.width <= 390).toBe(true);
  }
  await context.setOffline(false);
});
