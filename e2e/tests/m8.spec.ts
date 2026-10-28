import { expect, test, type Page } from "@playwright/test";
import { addDays, demoToday, displayDate } from "./demo-dates";
import { DEMO_SLUG, login } from "./helpers";

/**
 * M8, advanced cash desk and agreements (spec 30, ADR 0028), on the real
 * binary seeded with `seed demo`. Every test works on its own spots and days
 * so that the suite stays parallel:
 *
 *   - Trova posto: the four gazebos, 51-57 days ahead (no other test sells
 *     gazebos);
 *   - swap: H13 and H14, 55 days ahead; move from tomorrow: two free spots
 *     of rows G-H found through the quote, from today;
 *   - season release by range: a pass of row B, 56-57 days ahead;
 *   - agreements are created with a unique code per run.
 */

const API = "/api/admin/v1/establishments";

interface Ctx {
  page: Page;
  est: string;
  spots: Map<string, string>; // label -> id
  packs: Map<string, string>; // code -> id
}

const uid = () => Math.random().toString(36).slice(2, 7).toUpperCase();

async function csrf(page: Page): Promise<string> {
  return (
    (await page.context().cookies()).find((c) => c.name === "arenile_csrf")
      ?.value ?? ""
  );
}

async function call<T = any>(
  page: Page,
  method: string,
  path: string,
  data?: unknown,
): Promise<{ status: number; body: T }> {
  const res = await page.request.fetch(path, {
    method,
    data,
    headers: {
      "X-CSRF-Token": await csrf(page),
      "Idempotency-Key": `m8-${uid()}-${Date.now()}`,
    },
  });
  const text = await res.text();
  let body: any = text;
  try {
    body = JSON.parse(text);
  } catch {
    /* not JSON */
  }
  return { status: res.status(), body };
}

async function ok<T = any>(
  page: Page,
  method: string,
  path: string,
  data?: unknown,
): Promise<T> {
  const r = await call<T>(page, method, path, data);
  expect(
    r.status,
    `${method} ${path}: ${JSON.stringify(r.body).slice(0, 300)}`,
  ).toBeLessThan(300);
  return r.body;
}

async function setup(page: Page): Promise<Ctx> {
  await login(page);
  const list = await ok<{ items: { id: string; slug: string }[] }>(
    page,
    "GET",
    API,
  );
  const est = (list.items.find((e) => e.slug === DEMO_SLUG) ?? list.items[0])!
    .id;
  const maps = await ok<{ items: { id: string }[] }>(
    page,
    "GET",
    `${API}/${est}/maps`,
  );
  const map = await ok<{ spots: { id: string; label: string }[] }>(
    page,
    "GET",
    `${API}/${est}/maps/${maps.items[0]!.id}`,
  );
  const packs = await ok<{ items: { id: string; code: string }[] }>(
    page,
    "GET",
    `${API}/${est}/packs`,
  );
  return {
    page,
    est,
    spots: new Map(map.spots.map((s) => [s.label, s.id])),
    packs: new Map(
      (packs.items ?? (packs as any)).map((p: { id: string; code: string }) => [
        p.code,
        p.id,
      ]),
    ),
  };
}

function bookingBody(
  c: Ctx,
  label: string,
  from: string,
  to: string,
  lastName: string,
  pack = "base",
) {
  return {
    customer: { first_name: "Playwright", last_name: lastName },
    starts_on: from,
    ends_on: to,
    slots: "FULL",
    formula: from === to ? "day" : "multi_day",
    lines: [{ pack_id: c.packs.get(pack), spot_id: c.spots.get(label) }],
    force: true,
  };
}

async function book(
  c: Ctx,
  label: string,
  from: string,
  to: string,
  lastName: string,
  pack = "base",
) {
  return ok<{
    id: string;
    code: string;
    version: number;
    lines: {
      id: string;
      segments?: { spot_id: string; starts_on: string; ends_on: string }[];
    }[];
  }>(
    c.page,
    "POST",
    `${API}/${c.est}/bookings`,
    bookingBody(c, label, from, to, lastName, pack),
  );
}

/** First label of `labels` free from `from` to `to` (quote without writes). */
async function freeSpot(
  c: Ctx,
  labels: string[],
  from: string,
  to: string,
  except: string[] = [],
): Promise<string> {
  for (const l of labels) {
    if (except.includes(l) || !c.spots.has(l)) continue;
    const q = await call<{ available: boolean; conflicts: unknown[] }>(
      c.page,
      "POST",
      `${API}/${c.est}/bookings/quote`,
      bookingBody(c, l, from, to, "Prova"),
    );
    if (q.status < 300 && q.body.conflicts.length === 0) return l;
  }
  throw new Error(
    `no free spot among ${labels.join(", ")} from ${from} to ${to}`,
  );
}

function spot(page: Page, label: string) {
  return page.getByRole("button", {
    name: new RegExp(`^Postazione ${label}[,:]`),
  });
}

async function pickRange(
  page: Page,
  trigger: RegExp,
  from: string,
  to: string,
) {
  await page.getByRole("button", { name: trigger }).click();
  await page
    .getByRole("textbox", { name: "Dal", exact: true })
    .fill(displayDate(from));
  await page
    .getByRole("textbox", { name: "Al", exact: true })
    .fill(displayDate(to));
  await page.keyboard.press("Escape");
}

test("Trova posto sells a week on two gazebos when one day is full", async ({
  page,
}) => {
  const c = await setup(page);
  const W = (i: number) => addDays(demoToday(), 50 + i);
  // GZ3 and GZ4 taken all week, GZ1 on the last two days ("Ferragosto"), GZ2 on the first five.
  // (409 on a retry: the week is already set up.)
  for (const [label, from, to] of [
    ["GZ3", W(1), W(7)],
    ["GZ4", W(1), W(7)],
    ["GZ1", W(6), W(7)],
    ["GZ2", W(1), W(5)],
  ] as const) {
    const r = await call(
      page,
      "POST",
      `${API}/${c.est}/bookings`,
      bookingBody(c, label, from, to, "Gazebo", "gazebo"),
    );
    expect([201, 409], JSON.stringify(r.body).slice(0, 200)).toContain(
      r.status,
    );
  }
  const surname = `Trovaposto${uid()}`;
  await ok(page, "POST", `${API}/${c.est}/customers`, {
    first_name: "Giulia",
    last_name: surname,
  });

  await page.goto(`/admin/e/${c.est}/trova-posto`);
  await pickRange(page, /^Date:/, W(1), W(7));
  await page
    .getByRole("radiogroup", { name: "Cambi di posto ammessi" })
    .getByRole("radio", { name: "1" })
    .click();
  await page.getByRole("combobox", { name: "Zone" }).click();
  await page.getByRole("option", { name: "Gazebo" }).click();
  await page.keyboard.press("Escape");
  await page.getByRole("combobox", { name: "Pack o allestimento" }).click();
  await page.getByRole("option", { name: /Gazebo/ }).click();
  await page.getByRole("button", { name: "Cerca", exact: true }).click();

  const proposal = page
    .getByTestId("findspot-proposal")
    .filter({
      hasText: `GZ1 dal ${displayDate(W(1))} al ${displayDate(W(5))}`,
    });
  await expect(proposal).toContainText(
    `GZ2 dal ${displayDate(W(6))} al ${displayDate(W(7))}`,
  );
  await expect(page.getByText("1 cambio di posto").first()).toBeVisible();
  await page.getByRole("button", { name: "Prenota" }).first().click();

  const dialog = page.getByRole("dialog", { name: /Prenota la proposta/ });
  await dialog.getByRole("combobox", { name: "Cliente" }).click();
  await page.keyboard.type(surname);
  await page.getByRole("option", { name: new RegExp(surname) }).click();
  await dialog.getByRole("button", { name: "Prenota" }).click();
  await expect(dialog).toBeHidden();

  // One booking, one line, two segments: the whole week on two spots.
  const found = await ok<{ items: { id: string }[] }>(
    page,
    "GET",
    `${API}/${c.est}/bookings?q=${surname}`,
  );
  expect(found.items).toHaveLength(1);
  const detail = await ok<{
    lines: {
      segments?: { spot_id: string; starts_on: string; ends_on: string }[];
    }[];
  }>(page, "GET", `${API}/${c.est}/bookings/${found.items[0]!.id}`);
  expect(detail.lines).toHaveLength(1);
  expect(detail.lines[0]!.segments).toEqual([
    expect.objectContaining({
      spot_id: c.spots.get("GZ1"),
      starts_on: W(1),
      ends_on: W(5),
    }),
    expect.objectContaining({
      spot_id: c.spots.get("GZ2"),
      starts_on: W(6),
      ends_on: W(7),
    }),
  ]);
  // The full day stays full: no gazebo left.
  await page.goto(`/admin/e/${c.est}/mappa?date=${W(6)}`);
  for (const l of ["GZ1", "GZ2", "GZ3", "GZ4"])
    await expect(spot(page, l)).not.toHaveAccessibleName(/Libero/);
});

test("a move onto an occupied spot becomes a swap", async ({ page }) => {
  const c = await setup(page);
  const day = addDays(demoToday(), 55);
  const a = `Scambioa${uid()}`;
  const b = `Scambiob${uid()}`;
  await book(c, "H13", day, day, a);
  await book(c, "H14", day, day, b);

  await page.goto(`/admin/e/${c.est}/mappa?date=${day}`);
  await expect(spot(page, "H13")).toHaveAccessibleName(new RegExp(a));
  await spot(page, "H13").click();
  await page
    .getByRole("dialog", { name: /Postazione H13/ })
    .getByRole("button", { name: "Sposta" })
    .click();
  await expect(
    page.getByRole("alert").filter({ hasText: "Sposta da H13" }),
  ).toBeVisible();
  await spot(page, "H14").click();

  const swap = page.getByRole("alertdialog", { name: "Scambiare H13 e H14?" });
  await expect(swap).toContainText(
    `Scambio dal ${displayDate(day)} al ${displayDate(day)}`,
  );
  await swap.getByRole("button", { name: "Scambia" }).click();
  await expect(swap).toBeHidden();
  await expect(spot(page, "H13")).toHaveAccessibleName(new RegExp(b));
  await expect(spot(page, "H14")).toHaveAccessibleName(new RegExp(a));
});

test("a move from tomorrow keeps today on the first spot", async ({ page }) => {
  const c = await setup(page);
  const today = demoToday();
  const end = addDays(today, 3);
  const rows = ["H", "G"].flatMap((r) =>
    Array.from({ length: 15 }, (_, i) => `${r}${15 - i}`),
  );
  const from = await freeSpot(c, rows, today, end);
  const to = await freeSpot(c, rows, today, end, [from]);
  const who = `Domani${uid()}`;
  const b = await book(c, from, today, end, who);

  await page.goto(`/admin/e/${c.est}/mappa?date=${today}`);
  await spot(page, from).click();
  await page
    .getByRole("dialog", { name: new RegExp(`Postazione ${from}`) })
    .getByRole("button", { name: "Sposta" })
    .click();
  await spot(page, to).click();
  const move = page.getByRole("alertdialog", {
    name: `Spostare da ${from} a ${to}?`,
  });
  await move.getByRole("radio", { name: "Da domani" }).click();
  await move.getByRole("button", { name: "Sposta" }).click();
  await expect(move).toBeHidden();

  const detail = await ok<{
    lines: {
      segments?: { spot_id: string; starts_on: string; ends_on: string }[];
    }[];
  }>(page, "GET", `${API}/${c.est}/bookings/${b.id}`);
  expect(detail.lines[0]!.segments).toEqual([
    expect.objectContaining({
      spot_id: c.spots.get(from),
      starts_on: today,
      ends_on: today,
    }),
    expect.objectContaining({
      spot_id: c.spots.get(to),
      starts_on: addDays(today, 1),
      ends_on: end,
    }),
  ]);
  await expect(spot(page, from)).toHaveAccessibleName(
    new RegExp(who.slice(0, 6)),
  );
  await page.goto(`/admin/e/${c.est}/mappa?date=${addDays(today, 1)}`);
  await expect(spot(page, to)).toHaveAccessibleName(
    new RegExp(who.slice(0, 6)),
  );
});

test("an agreement chosen in a desk booking changes the price", async ({
  page,
}) => {
  const c = await setup(page);
  const code = `RES${uid()}`;
  const name = `Residenti ${code}`;
  await ok(page, "POST", `${API}/${c.est}/agreements`, {
    name,
    code,
    target: "individual_customers",
    action: { type: "percent_off", percent: "20" },
    channels: ["desk"],
    register_category: "residents",
  });

  await page.goto(`/admin/e/${c.est}/prenotazioni`);
  await page
    .getByRole("button", { name: /Nuova prenotazione/ })
    .first()
    .click();
  const sheet = page.getByRole("dialog", { name: "Nuova prenotazione" });
  await sheet.getByRole("combobox", { name: "Pack" }).click();
  await page.getByRole("option", { name: "Base", exact: true }).click();
  await sheet
    .getByRole("button", { name: /^Postazione \S+, fila \S+: Libero/ })
    .first()
    .click();
  const preview = sheet.getByRole("region", { name: "Anteprima prezzo" });
  const total = preview.getByText(/€/).first();
  await expect(total).toBeVisible();
  const before = (await total.textContent())!;

  await sheet.getByRole("combobox", { name: "Convenzione" }).click();
  await page.getByRole("option", { name: new RegExp(code) }).click();
  await expect(total).not.toHaveText(before);
  const cents = (s: string) => Number(s.replace(/[^\d]/g, ""));
  expect(cents((await total.textContent())!)).toBe(
    Math.round(cents(before) * 0.8),
  );
});

test("the agreement link of the public page applies the agreement", async ({
  page,
}) => {
  const c = await setup(page);
  const code = `HOT${uid()}`;
  await ok(page, "POST", `${API}/${c.est}/agreements`, {
    name: `Hotel ${code}`,
    code,
    target: "individual_customers",
    action: { type: "percent_off", percent: "15" },
    channels: ["desk", "online_link"],
    register_category: "accommodation",
  });

  await page.context().clearCookies();
  await page.goto(`/b/${DEMO_SLUG}?c=${code}`);
  const banner = page.getByRole("status").filter({ hasText: `Hotel ${code}` });
  await expect(banner).toBeVisible();
  await expect(banner).toContainText("-15%");

  // An unknown or desk-only code is never shown.
  await page.goto(`/b/${DEMO_SLUG}?c=NOPE${uid()}`);
  await expect(page.getByRole("heading", { name: "1. Quando" })).toBeVisible();
  await expect(
    page.getByRole("status").filter({ hasText: "Convenzione" }),
  ).toHaveCount(0);
});

test("a season pass released by range makes its days sellable", async ({
  page,
}) => {
  const c = await setup(page);
  const from = addDays(demoToday(), 56);
  const to = addDays(demoToday(), 57);
  // A pass of row B still holding both days (a retry finds the next one).
  let label = "";
  for (let n = 15; n >= 1 && !label; n--) {
    const q = await call<{ conflicts: unknown[] }>(
      page,
      "POST",
      `${API}/${c.est}/bookings/quote`,
      bookingBody(c, `B${n}`, from, to, "Prova"),
    );
    if (q.body.conflicts.length === 4) label = `B${n}`; // 2 days x 2 half-days
  }
  expect(label, "a pass of row B occupied on both days").not.toBe("");

  await page.goto(`/admin/e/${c.est}/abbonamenti`);
  await page
    .getByRole("row", { name: new RegExp(`abbonamento ${label} di `) })
    .click();
  const sheet = page.getByRole("dialog", { name: new RegExp(`^${label} ·`) });
  const range = sheet.getByRole("region", { name: "Rilascia un periodo" });
  await range.scrollIntoViewIfNeeded();
  await pickRange(page, /^Periodo:/, from, to);
  await range.getByRole("button", { name: "Rilascia il periodo" }).click();
  await expect(
    sheet.getByRole("region", { name: "Giorni rilasciati" }),
  ).toContainText(`${displayDate(to)} · Pomeriggio`);

  const free = await ok<{ conflicts: unknown[] }>(
    page,
    "POST",
    `${API}/${c.est}/bookings/quote`,
    bookingBody(c, label, from, to, "Prova"),
  );
  expect(free.conflicts).toHaveLength(0);
});

test("the agreements register downloads as Excel", async ({ page }) => {
  const c = await setup(page);
  await page.goto(`/admin/e/${c.est}/convenzioni/rendiconti`);
  await expect(
    page.getByRole("heading", { name: "Rendiconti delle convenzioni" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Esporta" }).click();
  const [download] = await Promise.all([
    page.waitForEvent("download"),
    page.getByRole("menuitem", { name: "Excel" }).click(),
  ]);
  expect(download.suggestedFilename()).toMatch(/\.xlsx$/);
  const path = await download.path();
  expect(path).toBeTruthy();
  const res = await page.request.get(
    `${API}/${c.est}/reports/agreements-register?from=${demoToday()}&to=${addDays(demoToday(), 60)}&format=html`,
  );
  expect(res.status(), await res.text()).toBe(200);
  expect(await res.text()).toContain("Registro delle convenzioni");
});
