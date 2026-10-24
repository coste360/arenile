import { expect, test, type Page } from "@playwright/test";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { applyTheme, capture, freezeClock, settle } from "./support/capture";
import { expandRoute, loadContext, missingPlaceholders, type RunContext } from "./support/context";
import { runFile } from "./support/env";
import { THEMES, loadManifest, selected, type ShotEntry } from "./support/manifest";

/**
 * One test per manifest entry and theme. A screen is captured after its
 * route loads and, for the entries listed in PREPARE, after the interaction
 * that brings it to the documented state (open a sheet, walk the public
 * flow). Unknown names just load their route, so adding a plain page to the
 * manifest needs no code here.
 */

const hostHtml = readFileSync(join(dirname(fileURLToPath(import.meta.url)), "fixtures", "host.html"), "utf8");

type Prepare = (page: Page, ctx: RunContext, entry: ShotEntry, seat: number) => Promise<void>;

/** Distinct free spot per public screenshot and theme (the captures run in parallel). */
function seatOf(name: string, theme: string): number {
  const names = Object.keys(PREPARE).filter((n) => n.startsWith("public-"));
  return Math.max(0, names.indexOf(name)) * THEMES.length + THEMES.indexOf(theme as (typeof THEMES)[number]);
}

/**
 * Walks the public flow to `step` like a customer: a single day in the
 * first bookable week, the first free spot (list view), the first pack.
 * The page is in the demo instance (manual payment: "Conferma").
 */
async function publicStep(page: Page, step: "where" | "what" | "who", seat = 0) {
  const order = ["when", "where", "what", "who"] as const;
  const target = order.indexOf(step);
  for (let i = 1; i <= target; i++) {
    const current = order[i - 1];
    if (current === "when") {
      await page.getByRole("button", { name: /Date del soggiorno/ }).click();
      const day = () => page.getByRole("grid").first().locator("button:not([disabled])").nth(9);
      await day().click();
      await day().click(); // single-day range
      await page.keyboard.press("Escape");
      await page.getByRole("button", { name: "Avanti" }).click();
    } else if (current === "where") {
      await page.getByRole("button", { name: "Mostra come elenco" }).click();
      // Each screenshot books its own spot (rows G and H): the captures run
      // in parallel and must not race for the same hold.
      const label = `${seat < 15 ? "G" : "H"}${(seat % 15) + 1}`;
      const own = page.getByRole("button", { name: new RegExp(`^${label} – Libero`) });
      await ((await own.count()) ? own : page.getByRole("button", { name: / – Libero/ }).last()).click();
      await page.getByRole("button", { name: "Mostra la mappa" }).click();
      await page.getByRole("button", { name: "Avanti" }).click();
    } else if (current === "what") {
      await page.getByRole("radio").first().click();
      await page.getByRole("button", { name: "Continua" }).click();
    }
    await settle(page);
  }
  await expect(page.locator('[aria-current="step"]')).toBeVisible();
}

/** Completes the public flow with a demo customer and waits for the confirmation. */
async function publicConfirmation(page: Page, _ctx: RunContext, _entry: ShotEntry, seat = 0) {
  await publicStep(page, "who", seat);
  await page.getByRole("textbox", { name: "Nome", exact: true }).fill("Giulia");
  await page.getByRole("textbox", { name: "Cognome" }).fill("Rossi");
  await page.getByRole("textbox", { name: "Email" }).fill("giulia.rossi@example.com");
  const terms = page.getByRole("checkbox", { name: /accetto le condizioni/ });
  if (await terms.count()) await terms.check();
  await page.getByRole("button", { name: /^(Conferma|Paga)/ }).last().click();
  await expect(page.getByRole("heading", { name: "Prenotazione confermata" })).toBeVisible({ timeout: 20_000 });
}

const PREPARE: Record<string, Prepare> = {
  "public-confirmation-mobile": publicConfirmation,
  "public-confirmation-desktop": publicConfirmation,
  "admin-map-spot": async (page) => {
    const spot = page.getByRole("button", { name: /^Postazione .*: (Presente|Prenotato|Da incassare|Abbonato)/ }).first();
    await spot.click();
    await expect(page.getByRole("dialog")).toBeVisible();
  },
  "admin-map-labels": async (page) => {
    // Occupant labels (spec 11.3): "Entrambi", one zoom step so the names read in full.
    await page.getByRole("radio", { name: "Entrambi" }).click();
    await page.getByRole("button", { name: "Ingrandisci" }).first().click();
    await page.getByTestId("beach-map").scrollIntoViewIfNeeded();
    await expect(page.locator('[data-map-label="caption"]').first()).toBeVisible();
  },
  "admin-booking-new": async (page) => {
    await page.getByRole("button", { name: "Nuova prenotazione" }).first().click();
    await expect(page.getByRole("dialog", { name: "Nuova prenotazione" })).toBeVisible();
  },
  "public-where-mobile": (page, _c, _e, seat) => publicStep(page, "where", seat),
  "public-where-desktop": (page, _c, _e, seat) => publicStep(page, "where", seat),
  "public-what-mobile": (page, _c, _e, seat) => publicStep(page, "what", seat),
  "public-what-desktop": (page, _c, _e, seat) => publicStep(page, "what", seat),
  "public-who-mobile": (page, _c, _e, seat) => publicStep(page, "who", seat),
  "public-who-desktop": (page, _c, _e, seat) => publicStep(page, "who", seat),
  "public-embed": embedReady,
  "public-embed-mobile": embedReady,
  "admin-printers-desktop": printersDesktop,
};

/**
 * ESC/POS printing runs on the desktop edition only (ADR 0021), and the
 * docs instance is a server edition: the desktop view of the Printers page
 * is shown on simulated data (desktop /meta, one reachable network printer,
 * a routing rule and a printed test page).
 */
async function printersDesktop(page: Page) {
  const at = "2027-07-15T08:30:00Z";
  const printer = {
    id: "00000000-0000-4000-8000-0000000000a1",
    name: "Banco bar",
    driver: "network",
    host: "192.168.1.50",
    port: 9100,
    paper_width_mm: 80,
    chars_per_line: 48,
    codepage: "cp858",
    cut_mode: "partial",
    feed_lines: 4,
    print_logo: true,
    logo_cached: true,
    drawer_pin: 2,
    active: true,
    status: "reachable",
    last_checked_at: at,
    last_success_at: at,
    version: 1,
    created_at: at,
    updated_at: at,
  };
  const json = (body: unknown, headers: Record<string, string> = {}) => ({
    status: 200,
    contentType: "application/json",
    headers,
    body: JSON.stringify(body),
  });
  await page.route("**/api/admin/v1/meta", async (route) => {
    const res = await route.fetch();
    const meta = (await res.json()) as Record<string, unknown>;
    await route.fulfill({ response: res, json: { ...meta, edition: "desktop", desktop: { mode: "local" } } });
  });
  await page.route("**/printers?*", (r) => r.fulfill(json({ items: [printer], escpos_available: true })));
  await page.route("**/print-routes", (r) =>
    r.fulfill(
      json(
        {
          items: [
            { document_kind: "cash_z", printer_id: printer.id, copies: 1, kick_drawer: false },
            { document_kind: "payment_receipt", printer_id: printer.id, copies: 1, kick_drawer: true },
          ],
        },
        { ETag: '"docs"' },
      ),
    ),
  );
  await page.route("**/print-jobs?*", (r) =>
    r.fulfill(
      json({
        items: [
          {
            id: "00000000-0000-4000-8000-0000000000b1",
            printer_id: printer.id,
            printer_name: printer.name,
            document_kind: "test_page",
            copies: 1,
            status: "printed",
            attempts: 1,
            max_attempts: 5,
            created_at: at,
            updated_at: at,
          },
        ],
      }),
    ),
  );
  await page.route("**/cash-registers", (r) => r.fulfill(json({ items: [] })));
  await page.reload();
  await expect(page.getByRole("listitem", { name: "Banco bar" })).toBeVisible();
  await expect(page.getByText("Stampato", { exact: true })).toBeVisible();
}

async function embedReady(page: Page, ctx: RunContext) {
  const frame = page.frameLocator(`[data-arenile="${ctx.slug}"] iframe`);
  await expect(frame.getByText(ctx.establishmentName).first()).toBeVisible();
  await expect
    .poll(() => page.locator(`[data-arenile="${ctx.slug}"] iframe`).evaluate((el) => (el as HTMLIFrameElement).style.height))
    .toMatch(/^\d+px$/);
}

const manifest = loadManifest();

for (const entry of manifest.screenshots.filter((s) => selected(s.name))) {
  for (const theme of THEMES) {
    test(`${entry.name} (${theme})`, async ({ browser, baseURL }) => {
      const ctx = loadContext();
      const missing = missingPlaceholders(entry.route, ctx);
      test.skip(missing.length > 0, `${entry.name}: {${missing.join("}, {")}} not available yet on this build`);
      const viewport = manifest.viewports[entry.viewport];
      const mobile = entry.viewport === "mobile";
      const context = await browser.newContext({
        baseURL,
        viewport,
        deviceScaleFactor: mobile ? 2 : 1,
        isMobile: mobile,
        hasTouch: mobile,
        locale: "it-IT",
        timezoneId: "Europe/Rome",
        colorScheme: theme,
        reducedMotion: "reduce",
        storageState: entry.auth === false ? undefined : runFile("owner.json"),
      });
      const page = await context.newPage();
      try {
        await applyTheme(page, theme);
        // The panel shows "today": its clock is frozen on the documented day.
        // The public pages book real holds on the server, whose expiry the
        // page counts down on the server's clock: they keep the real time.
        if (entry.section !== "pagina-pubblica") await freezeClock(page, ctx.date);

        let route = expandRoute(entry.route, ctx);
        if (route.startsWith("/__docs/")) {
          // Host page served from the instance origin, so the default
          // frame-ancestors 'self' policy allows the widget iframe.
          await page.route(`${baseURL}${route}`, (r) =>
            r.fulfill({ status: 200, contentType: "text/html; charset=utf-8", body: hostHtml }),
          );
        } else if (route.startsWith("/b/")) {
          route += `${route.includes("?") ? "&" : "?"}theme=${theme}`;
        }
        await page.goto(route);
        if (entry.auth !== false) await expect(page).not.toHaveURL(/\/login/);
        await settle(page);
        await PREPARE[entry.name]?.(page, ctx, entry, seatOf(entry.name, theme));
        await settle(page);
        await capture(page, entry.name, theme);
      } finally {
        await context.close();
      }
    });
  }
}
