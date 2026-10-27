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
      // The tenth bookable day of the month shown, or its last one near the
      // end of the month (a phone shows a single month).
      const days = page.getByRole("grid").first().locator("button:not([disabled])");
      await expect(days.first()).toBeVisible();
      const pick = Math.min(9, (await days.count()) - 1);
      const day = () => days.nth(pick);
      await day().click();
      await day().click(); // single-day range
      await page.keyboard.press("Escape");
      await page.getByRole("button", { name: "Avanti" }).click();
    } else if (current === "where") {
      await page.getByRole("button", { name: "Mostra come elenco" }).click();
      // Each screenshot books its own spot (rows G and H): the captures run
      // in parallel and must not race for the same hold.
      const label = `${seat < 15 ? "G" : "H"}${(seat % 15) + 1}`;
      const own = page.getByRole("button", { name: new RegExp(`^${label} Libero`) });
      await ((await own.count()) ? own : page.getByRole("button", { name: /^\S+ Libero/ }).last()).click();
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
  "admin-map-selection": async (page) => {
    // Selection only (no booking is created): E3 to E5 with Shift.
    await page.getByRole("button", { name: "Seleziona più postazioni" }).click();
    await page.getByRole("button", { name: /^Postazione E3[,:]/ }).click();
    await page.getByRole("button", { name: /^Postazione E5[,:]/ }).click({ modifiers: ["Shift"] });
    await expect(page.getByTestId("selection-bar")).toBeVisible();
  },
  "admin-map-block": async (page) => {
    await page.getByRole("button", { name: "Seleziona più postazioni" }).click();
    await page.getByRole("button", { name: /^Postazione E3[,:]/ }).click();
    await page.getByRole("button", { name: /^Postazione E4[,:]/ }).click({ modifiers: ["Shift"] });
    await page.getByTestId("selection-bar").getByRole("button", { name: "Blocca", exact: true }).click();
    await expect(page.getByRole("dialog")).toBeVisible();
  },
  "admin-imports": async (page) => {
    // Upload and preview only: nothing is applied.
    await page.getByLabel("File CSV").setInputFiles({
      name: "clienti.csv",
      mimeType: "text/csv",
      buffer: Buffer.from("cognome;nome;email\r\nVerdi;Anna;anna.verdi@example.org\r\n;Luca;luca@example\r\nBruni;Marco;marco.bruni@example.org\r\n"),
    });
    await page.getByRole("button", { name: "Carica e verifica" }).click();
    await page.getByRole("button", { name: "Anteprima" }).click();
    await expect(page.getByRole("table", { name: "Errori" })).toBeVisible();
  },
  "admin-pass-price": async (page) => {
    await page.getByRole("table", { name: "Abbonamenti" }).getByRole("row").nth(2).click();
    await expect(page.getByRole("button", { name: "Imposta prezzo manuale" })).toBeVisible();
  },
  "admin-notifications": async (page) => {
    await page.getByTestId("notifications-bell").click();
    await expect(page.getByRole("dialog").or(page.locator("[data-radix-popper-content-wrapper]")).first()).toBeVisible();
  },
  "admin-map-kit": async (page) => {
    await page.getByRole("tab", { name: "Mappa" }).click();
    await expect(page.locator('[data-map-kit-preview="classic"] use').first()).toBeAttached();
  },
  "admin-report-exports": async (page) => {
    await page.getByRole("button", { name: "Esporta elenchi" }).click();
    await expect(page.getByRole("menu")).toBeVisible();
  },
  "public-confirmation-mobile": publicConfirmation,
  "public-confirmation-desktop": publicConfirmation,
  "admin-map-spot": async (page) => {
    const spot = page.getByRole("button", { name: /^Postazione .*: (Presente|Prenotato|Da incassare|Abbonato)/ }).first();
    await spot.click();
    await expect(page.getByRole("dialog")).toBeVisible();
  },
  "admin-map-labels": async (page) => {
    // Occupant labels (spec 11.3): code and customer, one zoom step so the names read in full.
    await labelParts(page, ["Codice", "Cliente"]);
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
  // Daily sale and tickets (ADR 0020): data from support/tickets.ts.
  "admin-daily-sale": async (page) => {
    await page.getByRole("radio", { name: /Solo zona/ }).click();
    await page.getByRole("radiogroup", { name: "Zona" }).getByRole("radio", { name: /File successive/ }).click();
    await expect(page.getByTestId("sale-total")).toHaveText(/€/);
  },
  "admin-daily-sale-map": async (page) => {
    await page.getByRole("button", { name: /^Postazione .*: Libero/ }).first().click();
    await page.getByRole("dialog").getByRole("button", { name: "Vendi giornata" }).click();
    const sale = page.getByRole("dialog", { name: "Vendi giornata" });
    await expect(sale.getByTestId("sale-total")).toHaveText(/€/);
  },
  "admin-ticket-print": async (page) => {
    await page.getByRole("button", { name: "Stampa il biglietto #001" }).click();
    const dialog = page.getByRole("dialog", { name: "Stampa del biglietto #001" });
    await expect(dialog.getByRole("article", { name: "Biglietto #001" })).toBeVisible();
  },
  "admin-tickets": async (page) => {
    await expect(page.getByRole("table", { name: "Ticket del giorno" }).getByText("#006")).toBeVisible();
  },
  "admin-assign-panel": async (page) => {
    const panel = page.getByTestId("assign-panel");
    await panel.getByRole("button", { name: /^Biglietto #/ }).first().click();
    await expect(panel.getByRole("status")).toBeVisible();
  },
  "admin-entry-types": async (page) => {
    await expect(page.getByText("Ridotto").first()).toBeVisible();
  },
  "admin-entry-sale": async (page) => {
    await page.getByRole("button", { name: "Aumenta Adulto" }).click();
    await page.getByRole("button", { name: "Aumenta Adulto" }).click();
    await page.getByRole("button", { name: "Aumenta Bambino" }).click();
    await expect(page.getByText(/posti rimasti/)).toBeVisible();
  },
  "admin-entry-gate-mobile": async (page, ctx) => {
    // The gate admits only today's tickets (server clock) and the demo day is
    // in the future: the shot shows the page ready with a code typed in.
    const res = await page.request.get(
      `/api/admin/v1/establishments/${ctx.est}/tickets?date=${ctx.date}&kind=entry`,
    );
    const items = ((await res.json()) as { items: { booking_code: string }[] }).items;
    if (!items[0]) throw new Error("no entry ticket on the demo day");
    await page.getByLabel("Codice del biglietto").fill(items[0].booking_code);
    await expect(page.getByRole("button", { name: "Conferma ingresso" })).toBeEnabled();
  },
  "admin-printers-desktop": printersDesktop,
  // ---- M8, cash desk and agreements (data: support/m8.ts) ----------------
  "admin-map-m8": async (page) => {
    await labelParts(page, ["Codice", "Cliente", "Fine"]);
    await page.getByRole("radiogroup", { name: "Filtri rapidi" }).getByRole("radio", { name: "Arrivi" }).click();
    await page.getByTestId("beach-map").scrollIntoViewIfNeeded();
  },
  "admin-spot-calendar": async (page) => {
    await m8Spot(page, "Neri").click();
    await page.getByRole("dialog", { name: /^Postazione / }).getByRole("button", { name: "Calendario" }).click();
    await expect(page.getByTestId("spot-calendar").last()).toBeVisible();
  },
  "admin-find-spot": async (page, ctx) => {
    const dmy = (iso: string) => iso.split("-").reverse().join("/");
    const end = new Date(`${ctx.date}T12:00:00Z`);
    end.setUTCDate(end.getUTCDate() + 6);
    await page.getByRole("button", { name: /^Date:/ }).click();
    await page.getByRole("textbox", { name: "Dal", exact: true }).fill(dmy(ctx.date));
    await page.getByRole("textbox", { name: "Al", exact: true }).fill(dmy(end.toISOString().slice(0, 10)));
    await page.keyboard.press("Escape");
    await page.getByRole("radiogroup", { name: "Cambi di posto ammessi" }).getByRole("radio", { name: "1" }).click();
    await page.getByRole("combobox", { name: "Pack o allestimento" }).click();
    await page.getByRole("option", { name: "Base", exact: true }).click();
    await page.getByRole("form", { name: "Ricerca" }).getByRole("button", { name: "Cerca", exact: true }).click();
    await expect(page.getByTestId("findspot-proposal").first()).toBeVisible();
  },
  "admin-map-swap": async (page) => {
    await m8Spot(page, "Esposito").click();
    await page.getByRole("dialog", { name: /^Postazione / }).getByRole("button", { name: "Sposta" }).click();
    await m8Spot(page, "Galli").click();
    await expect(page.getByRole("alertdialog", { name: /^Scambiare / })).toBeVisible();
  },
  "admin-map-move-from": async (page) => {
    await m8Spot(page, "Esposito").click();
    await page.getByRole("dialog", { name: /^Postazione / }).getByRole("button", { name: "Sposta" }).click();
    await page.getByRole("button", { name: /^Postazione F\d+[,:] .*Libero/ }).first().click();
    const move = page.getByRole("alertdialog", { name: /^Spostare da / });
    await move.getByRole("radio", { name: "Da una data" }).click();
    await expect(move.getByRole("radio", { name: "Da una data" })).toBeChecked();
  },
  "admin-map-print": async (page) => {
    await page.getByRole("button", { name: "Stampa" }).click();
    await expect(page.getByRole("menuitem", { name: "Lista per i bagnini" })).toBeVisible();
  },
  "admin-partners": async (page) => {
    await expect(page.getByRole("table", { name: "Partner" }).getByText("Hotel Belvedere")).toBeVisible();
  },
  "admin-partner-account": async (page) => {
    await expect(page.getByRole("table", { name: "Prenotazioni a carico del partner" })).toBeVisible();
  },
  "admin-agreements-register": async (page) => {
    await expect(page.getByText("Residenti di Gaeta").first()).toBeVisible();
  },
  "admin-booking-agreement": async (page) => {
    await page.getByRole("button", { name: "Nuova prenotazione" }).first().click();
    const sheet = page.getByRole("dialog", { name: "Nuova prenotazione" });
    await sheet.getByRole("combobox", { name: "Pack" }).click();
    await page.getByRole("option", { name: "Base", exact: true }).click();
    await sheet.getByRole("button", { name: /^Postazione \S+, fila \S+: Libero/ }).first().click();
    await sheet.getByRole("combobox", { name: "Convenzione" }).click();
    await page.getByRole("option", { name: /Residenti di Gaeta/ }).click();
    await expect(sheet.getByRole("checkbox", { name: "Requisito verificato" })).toBeVisible();
    await sheet.getByRole("heading", { name: "Convenzione e ospiti" }).evaluate((el) => el.scrollIntoView({ block: "start" }));
  },
  "public-agreement-link": async (page) => {
    await expect(page.getByRole("status").filter({ hasText: "Ospiti Hotel Belvedere" })).toBeVisible();
  },
};

/** Sets the "Etichette" toggles of the operational map to exactly `parts` (spec 30.4). */
async function labelParts(page: Page, parts: string[]) {
  const bar = page.getByRole("toolbar", { name: "Etichette" });
  for (const name of ["Codice", "Cliente", "Fine", "Allestimento"]) {
    const button = bar.getByRole("button", { name, exact: true });
    if (!(await button.count())) continue;
    const pressed = (await button.getAttribute("aria-pressed")) === "true";
    if (pressed !== parts.includes(name)) await button.click();
  }
}

/** The spot of the M8 booking of `surname` on the demo day (support/m8.ts). */
function m8Spot(page: Page, surname: string) {
  return page.getByRole("button", { name: new RegExp(`^Postazione D\\d+[,:].*${surname}`) }).first();
}

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
