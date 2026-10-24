import {
  expect,
  test,
  type APIRequestContext,
  type Cookie,
  type Page,
} from "@playwright/test";
import { login } from "./helpers";

/**
 * Daily sale and tickets (spec 14.3, ADR 0020) against the real binary:
 * walk-in sale on a spot from the day map with cash and the numbered
 * ticket (#001 every morning), zone-first sale "da assegnare" then
 * assignment by tap and automatic, void that keeps the number, entry-only
 * tickets with the gate check-in (one entry per ticket). The demo season is
 * in the future and its cash desk is shared with other tests, so each test
 * builds its own establishment with a season open today: numbering, cash
 * session and spots are all its own.
 */

const TZ = "Europe/Rome";

// One worker and one sign-in for the whole file: the login limit (30 per IP
// every 15 minutes) is shared with the rest of the suite.
test.describe.configure({ mode: "serial" });
let session: Cookie[] = [];
test.beforeAll(async ({ browser, baseURL }) => {
  const context = await browser.newContext({ baseURL, locale: "it-IT" });
  await login(await context.newPage());
  session = await context.cookies();
  await context.close();
});

/** Reuses the session signed in once by beforeAll. */
async function signIn(page: Page): Promise<void> {
  await page.context().addCookies(session);
}

/** YYYY-MM-DD of today (+ days) in the establishment timezone. */
function romeDate(plusDays = 0): string {
  const d = new Date(Date.now() + plusDays * 86_400_000);
  return new Intl.DateTimeFormat("en-CA", { timeZone: TZ }).format(d);
}

/** JSON request with the CSRF token of the cookie session. */
async function send(
  page: Page,
  method: "POST" | "PUT" | "PATCH",
  url: string,
  data: unknown,
  headers: Record<string, string> = {},
) {
  const csrf = (await page.context().cookies()).find(
    (c) => c.name === "arenile_csrf",
  )?.value;
  const res = await page.request.fetch(url, {
    method,
    data,
    headers: { "X-CSRF-Token": csrf ?? "", ...headers },
  });
  expect(
    res.ok(),
    `${method} ${url} -> ${res.status()} ${await res.text()}`,
  ).toBeTruthy();
  return res.json();
}

async function getJSON(request: APIRequestContext, url: string) {
  const res = await request.get(url);
  expect(res.ok(), `GET ${url} -> ${res.status()}`).toBeTruthy();
  return res.json();
}

interface Lido {
  id: string;
  /** API base of the establishment. */
  api: string;
  /** Panel base path. */
  base: string;
  seasonId: string;
  today: string;
  spots: Map<string, string>;
  zones: Map<string, string>;
  packId: string;
}

/** A new lido (classic rows) with a season open today and a low-season price list. */
async function seedLido(page: Page, name: string): Promise<Lido> {
  const suffix = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`;
  const est = await send(page, "POST", "/api/admin/v1/establishments", {
    name: `Lido ${name} ${suffix}`,
    slug: `lido-${name.toLowerCase()}-${suffix}`,
  });
  const api = `/api/admin/v1/establishments/${est.id}`;
  const applied = await send(page, "POST", `${api}/setup/apply-template`, {
    template: "classic_rows",
    opens_on: romeDate(-1),
    closes_on: romeDate(30),
  });
  const season = await getJSON(
    page.request,
    `${api}/seasons/${applied.season_id}`,
  );
  await send(page, "PUT", `${api}/seasons/${season.id}`, {
    year: season.year,
    opens_on: season.opens_on,
    closes_on: season.closes_on,
    status: "active",
  });
  // A season around today (autumn or spring) only has the low-season period.
  const prices = ["row1", "row2", "standard", "gazebo"].map((cat) => ({
    price_category: cat,
    rate_period_code: "low",
    amount: 2000,
  }));
  await send(page, "POST", `${api}/setup/quick-pricelist`, {
    season_id: season.id,
    prices,
  });

  const map = await getJSON(page.request, `${api}/maps/${applied.map_id}`);
  const spotList = (map.spots ?? map.document?.spots ?? []) as {
    id: string;
    label: string;
  }[];
  const zoneList = (await getJSON(page.request, `${api}/zones`)).items as {
    id: string;
    code: string;
  }[];
  const packs = (await getJSON(page.request, `${api}/packs`)).items as {
    id: string;
    code: string;
  }[];
  return {
    id: est.id,
    api,
    base: `/admin/e/${est.id}`,
    seasonId: season.id,
    today: romeDate(),
    spots: new Map(spotList.map((s) => [s.label, s.id])),
    zones: new Map(zoneList.map((z) => [z.code, z.id])),
    packId: (packs.find((p) => p.code === "base") ?? packs[0])!.id,
  };
}

/** Card sale through the API (no cash session needed). */
async function apiSale(
  page: Page,
  lido: Lido,
  where: { spot?: string; zone?: string },
) {
  return send(
    page,
    "POST",
    `${lido.api}/daily-sales`,
    {
      date: lido.today,
      slots: "FULL",
      pack_id: lido.packId,
      ...(where.spot ? { spot_id: lido.spots.get(where.spot) } : {}),
      ...(where.zone ? { zone_id: lido.zones.get(where.zone) } : {}),
      payment: { method: "card" },
    },
    { "Idempotency-Key": crypto.randomUUID() },
  );
}

function spot(page: Page, label: string) {
  return page.getByRole("button", {
    name: new RegExp(`^Postazione ${label}:`),
  });
}

/** The browser print dialog never opens in the tests: count the calls instead. */
async function stubPrint(page: Page) {
  await page.addInitScript(() => {
    (window as unknown as { __prints: number }).__prints = 0;
    window.print = () => {
      (window as unknown as { __prints: number }).__prints++;
    };
  });
}

test.describe("daily sale", () => {
  test.use({ viewport: { width: 1440, height: 900 } });

  test("spot-first sale from the map with cash: ticket #001, print preview, map label", async ({
    page,
  }) => {
    test.setTimeout(120_000);
    await stubPrint(page);
    await signIn(page);
    const lido = await seedLido(page, "Spot");
    await send(page, "POST", `${lido.api}/cash-sessions`, { opening_float: 0 });

    await page.goto(`${lido.base}/mappa?date=${lido.today}`);
    await expect(
      page.getByRole("heading", { name: "Mappa", level: 1 }),
    ).toBeVisible();
    await expect(spot(page, "B3")).toHaveAccessibleName(/Libero/);
    await spot(page, "B3").click();
    await page
      .getByRole("dialog", { name: /Postazione B3/ })
      .getByRole("button", { name: "Vendi giornata" })
      .click();

    const sale = page.getByRole("dialog", { name: "Vendi giornata" });
    await expect(sale.getByTestId("sale-total")).toHaveText(/20,00/);
    await sale.getByLabel("Contanti ricevuti").fill("50");
    await expect(sale.getByText(/30,00\s€/).first()).toBeVisible();
    await sale.getByRole("button", { name: /Contanti/ }).click();
    await expect(sale.getByTestId("ticket-number")).toHaveText("#001");
    // Auto-print is on by default: the browser print was asked once.
    await expect
      .poll(() =>
        page.evaluate(
          () => (window as unknown as { __prints: number }).__prints,
        ),
      )
      .toBe(1);

    // Reprint preview: the receipt with number, spot and QR.
    await sale.getByRole("button", { name: "Ristampa" }).click();
    const preview = page.getByRole("dialog", {
      name: "Stampa del biglietto #001",
    });
    const receipt = preview.getByRole("article", { name: "Biglietto #001" });
    await expect(receipt).toBeVisible();
    await expect(
      receipt.getByText("B3", { exact: false }).first(),
    ).toBeVisible();
    await expect(receipt.locator("svg").first()).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(preview).toBeHidden();
    await sale.getByRole("button", { name: "Chiudi" }).first().click();
    await expect(sale).toBeHidden();

    // The map labels the spot with the ticket number (no name given).
    await expect(spot(page, "B3")).toHaveAccessibleName(/occupato da #001/);

    // The ticket is in the list of the day, paid in cash.
    await page.goto(`${lido.base}/ticket?date=${lido.today}`);
    const table = page.getByRole("table", { name: "Ticket del giorno" });
    await expect(table.getByText("#001")).toBeVisible();
    await expect(table.getByText("Contanti").first()).toBeVisible();
  });

  test("zone-first sale is 'da assegnare', then assigned by tap and automatically", async ({
    page,
  }) => {
    test.setTimeout(120_000);
    await stubPrint(page);
    await signIn(page);
    const lido = await seedLido(page, "Zona");

    // Quick sale page, only the zone, card.
    await page.goto(`${lido.base}/vendita-rapida`);
    await expect(
      page.getByRole("heading", { name: "Vendita rapida", level: 1 }),
    ).toBeVisible();
    await page.getByRole("radio", { name: /Solo zona/ }).click();
    await page
      .getByRole("radiogroup", { name: "Zona" })
      .getByRole("radio", { name: /Prima fila/ })
      .click();
    await page.getByLabel("Cognome").fill("Bianchi");
    await expect(page.getByTestId("sale-total")).toHaveText(/20,00/);
    await page.getByRole("button", { name: /Carta/ }).click();
    await expect(page.getByTestId("ticket-number")).toHaveText("#001");
    // Two more zone tickets for the automatic assignment.
    await apiSale(page, lido, { zone: "row1" });
    await apiSale(page, lido, { zone: "row1" });

    // Oggi -> Da fare -> Da assegnare lists them, with a link to the map.
    await page.goto(`${lido.base}/oggi?date=${lido.today}`);
    await page.getByRole("tab", { name: /Da assegnare/ }).click();
    const todo = page.getByRole("tabpanel", { name: /Da assegnare/ });
    await expect(todo.getByText("#001 · Bianchi")).toBeVisible();
    await expect(
      todo.getByRole("link", { name: "Assegna il biglietto #003 sulla mappa" }),
    ).toBeVisible();
    // Ticket del giorno shows them as "Da assegnare".
    await page.goto(`${lido.base}/ticket?date=${lido.today}`);
    await expect(
      page
        .getByRole("table", { name: "Ticket del giorno" })
        .getByText("Da assegnare")
        .first(),
    ).toBeVisible();

    // Day map: the panel lists the tickets; tap #001 then a free spot of the row.
    await page.goto(`${lido.base}/mappa?date=${lido.today}`);
    const panel = page.getByTestId("assign-panel");
    await expect(panel).toBeVisible();
    await panel.getByRole("button", { name: /^Biglietto #001/ }).click();
    await expect(spot(page, "A1")).toHaveAccessibleName(/Libero/);
    await spot(page, "A1").click();
    await expect(
      page.getByText("Biglietto #001 assegnato alla postazione A1"),
    ).toBeVisible();
    await expect(spot(page, "A1")).toHaveAccessibleName(/occupato da Bianchi/);

    await panel
      .getByRole("button", { name: "Assegna automaticamente" })
      .click();
    await expect(page.getByText("2 biglietti assegnati")).toBeVisible();
    await expect(panel).toBeHidden();
    const tickets = await getJSON(
      page.request,
      `${lido.api}/tickets/unassigned?date=${lido.today}`,
    );
    expect(tickets.items).toHaveLength(0);
    await expect(
      page.getByRole("button", { name: /occupato da #002/ }),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: /occupato da #003/ }),
    ).toBeVisible();
  });

  test("a voided ticket keeps its number and the next sale continues the sequence", async ({
    page,
  }) => {
    test.setTimeout(120_000);
    await signIn(page);
    const lido = await seedLido(page, "Annullo");
    await apiSale(page, lido, { spot: "C1" });
    await apiSale(page, lido, { spot: "C2" });

    await page.goto(`${lido.base}/ticket?date=${lido.today}`);
    const table = page.getByRole("table", { name: "Ticket del giorno" });
    await expect(table.getByText("#002")).toBeVisible();
    await page
      .getByRole("button", { name: "Annulla il biglietto #001" })
      .click();
    const dialog = page.getByRole("dialog", {
      name: "Annullare il biglietto #001?",
    });
    await dialog.getByLabel("Motivo").fill("Cliente andato via");
    await dialog.getByRole("button", { name: "Annulla biglietto" }).click();
    await expect(dialog).toBeHidden();
    const row1 = table.getByRole("row").filter({ hasText: "#001" });
    await expect(row1.getByText("Annullato")).toBeVisible();

    const next = await apiSale(page, lido, { spot: "C3" });
    expect(next.ticket.number_label).toBe("#003");
    await page.reload();
    await expect(table.getByText("#003")).toBeVisible();
    await expect(
      table.getByRole("row").filter({ hasText: "#001" }).getByText("Annullato"),
    ).toBeVisible();
    // The spot of the voided ticket is free again.
    await page.goto(`${lido.base}/mappa?date=${lido.today}`);
    await expect(spot(page, "C1")).toHaveAccessibleName(/Libero/);
  });

  test("entry tickets: sale on the shared daily capacity and one entry per ticket at the gate", async ({
    page,
  }) => {
    test.setTimeout(120_000);
    await stubPrint(page);
    await signIn(page);
    const lido = await seedLido(page, "Ingressi");
    await send(page, "POST", `${lido.api}/cash-sessions`, { opening_float: 0 });

    // A quantity zone (equipped free beach) with its daily capacity, and two
    // ticket types each priced by its own pack rule.
    const zone = await send(page, "POST", `${lido.api}/zones`, {
      code: "ingresso",
      name: { it: "Spiaggia libera attrezzata" },
      assignment_mode: "quantity",
      online_visible: true,
      price_category: "ingresso",
    });
    const pool = await send(page, "POST", `${lido.api}/pools`, {
      zone_id: zone.id,
      component_code: "ingresso",
      name: { it: "Ingressi" },
      capacity: 40,
    });
    await send(page, "POST", `${lido.api}/components`, {
      code: "ingresso",
      name: { it: "Ingresso" },
      tracked: true,
      pool_id: pool.id,
    });
    for (const [code, name, amount] of [
      ["adulto", "Adulto", 1000],
      ["bambino", "Bambino", 500],
    ] as const) {
      const pack = await send(page, "POST", `${lido.api}/packs`, {
        code: `ingresso-${code}`,
        name: { it: `Ingresso ${name}` },
        composition: { ingresso: 1 },
        allowed_zone_ids: [zone.id],
        allowed_formulas: ["day", "half_day"],
        channels: ["desk", "online"],
        max_per_booking: 100,
        active: true,
      });
      await send(page, "POST", `${lido.api}/price-rules`, {
        season_id: lido.seasonId,
        name: `Ingresso ${name}`,
        target_type: "pack",
        target_ids: [pack.id],
        conditions: {},
        action: { type: "set", amount },
        stage: "base",
        active: true,
      });
      await send(page, "POST", `${lido.api}/entry-ticket-types`, {
        code,
        name: { it: name },
        pack_id: pack.id,
        zone_id: zone.id,
        active: true,
      });
    }

    await page.goto(`${lido.base}/vendita-rapida?tab=entries`);
    await expect(page.getByRole("tab", { name: "Ingressi" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    await page.getByRole("button", { name: "Aumenta Adulto" }).click();
    await page.getByRole("button", { name: "Aumenta Adulto" }).click();
    await page.getByRole("button", { name: "Aumenta Bambino" }).click();
    const submit = page.getByRole("button", { name: /^Incassa 25,00\s€$/ });
    await expect(submit).toBeEnabled();
    await expect(page.getByText(/posti rimasti/)).toBeVisible();
    await submit.click();
    await expect(page.getByText("Biglietto #001 emesso")).toBeVisible();

    const list = await getJSON(
      page.request,
      `${lido.api}/tickets?date=${lido.today}`,
    );
    const ticket = list.items.find(
      (t: { number_label: string }) => t.number_label === "#001",
    );
    expect(ticket).toBeTruthy();
    const code = ticket.booking_code as string;

    // Gate on the staff phone: manual code entry (the camera is not available here).
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto(`${lido.base}/ticket/varco`);
    await expect(
      page.getByRole("heading", { name: "Varco", level: 1 }),
    ).toBeVisible();
    await page.getByLabel("Codice del biglietto").fill(code);
    await page.getByRole("button", { name: "Verifica" }).click();
    await expect(page.getByText("Ingresso valido")).toBeVisible();
    await expect(page.getByText("3 persone")).toBeVisible();
    await expect(page.getByLabel("Codice del biglietto")).toHaveValue("");
    // Same QR again: refused.
    await page.waitForTimeout(4_100);
    await page.getByLabel("Codice del biglietto").fill(code);
    await page.getByRole("button", { name: "Verifica" }).click();
    await expect(page.getByText("Già entrato")).toBeVisible();
  });
});

test.describe("phone optional (ADR 0020)", () => {
  test("a new customer in the panel needs only the surname", async ({
    page,
  }) => {
    await signIn(page);
    const res = await page.request.get("/api/admin/v1/establishments");
    const body = (await res.json()) as {
      items: { id: string; slug: string }[];
    };
    const demo =
      body.items.find((e) => e.slug === "lido-demo") ?? body.items[0];
    await page.goto(`/admin/e/${demo!.id}/clienti`);
    await page.getByRole("button", { name: "Nuovo cliente" }).first().click();
    const dialog = page.getByRole("dialog", { name: "Nuovo cliente" });
    const surname = `Senzatelefono${Date.now().toString(36)}`;
    await dialog.getByLabel("Cognome").fill(surname);
    await dialog.getByRole("button", { name: /Crea|Salva/ }).click();
    await expect(
      page.getByText(new RegExp(`Cliente .*${surname}.* creato`)),
    ).toBeVisible();
  });
});

test.describe("phone optional on the public page (ADR 0020)", () => {
  test("a public booking goes through step 4 without a phone", async ({
    page,
  }) => {
    test.setTimeout(120_000);
    // Own day: other booking tests take the first free spots of theirs.
    const dayName = /\b26 giugno/;
    await page.goto(`/b/lido-demo`);
    await page.getByRole("button", { name: /Date del soggiorno/ }).click();
    const grid = page.getByRole("grid").first();
    await expect(grid).toBeVisible();
    for (let i = 0; i < 12; i++) {
      if (/giugno/.test((await grid.getAttribute("aria-label")) ?? "")) break;
      await page.getByRole("button", { name: /mese successivo|Next/i }).click();
    }
    await grid.getByRole("button", { name: dayName }).click();
    await page
      .getByRole("grid")
      .first()
      .getByRole("button", { name: dayName })
      .click();
    await page.keyboard.press("Escape");
    await page.getByRole("button", { name: "Avanti" }).click();

    await page.getByRole("button", { name: "Mostra come elenco" }).click();
    const free = page.getByRole("button", {
      name: /^(?!.*(?:Gazebo|GZ)).* – Libero/,
    });
    await expect(free.first()).toBeVisible();
    await free.nth(Math.floor(Math.random() * (await free.count()))).click();
    await page.getByRole("button", { name: "Avanti" }).click();
    await page.getByRole("radio", { name: /^Base/ }).click();
    await page.getByRole("button", { name: "Continua" }).click();

    await page
      .getByRole("textbox", { name: "Nome", exact: true })
      .fill("Lucia");
    await page.getByRole("textbox", { name: "Cognome" }).fill("Senzatelefono");
    await page
      .getByRole("textbox", { name: "Email" })
      .fill("lucia.senzatelefono@mailbox.test");
    const phone = page.getByRole("textbox", { name: "Telefono (facoltativo)" });
    await expect(phone).toHaveValue("");
    await page
      .getByRole("checkbox", { name: /accetto le condizioni di prenotazione/ })
      .check();

    const saved = page.waitForResponse(
      (r) =>
        r.request().method() === "PATCH" &&
        /\/api\/public\/v1\/lido-demo\/bookings\/[^/]+$/.test(
          new URL(r.url()).pathname,
        ),
    );
    await page
      .getByRole("button", { name: /^(Paga|Conferma|Prenota|Invia)/ })
      .click();
    const res = await saved;
    expect(res.status(), await res.text()).toBeLessThan(300);
    await expect(page.getByText("Il telefono è obbligatorio")).toHaveCount(0);
  });
});
