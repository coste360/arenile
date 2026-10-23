import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { login } from "./helpers";

/**
 * M3 "Prenotazioni dal pannello e cassa" against the real binary seeded with
 * `seed demo` (season 2027 from 1 May; demo bookings in May, a closed and an
 * open cash session). Each test works on its own spots and dates so the
 * suite runs in parallel on the shared database.
 */

/** A demo day with passes, daily bookings and free spots. */
const DEMO_DAY = "2027-05-10";
/** A season day far from the demo bookings, for creating new ones. */
const FREE_DAY = "2027-06-16";

/** Signs in and returns the panel base path of the demo establishment. */
async function openPanel(page: Page): Promise<string> {
  await login(page);
  const res = await page.request.get("/api/admin/v1/establishments");
  expect(res.ok()).toBeTruthy();
  const body = (await res.json()) as { items: { id: string; slug: string }[] };
  const demo = body.items.find((e) => e.slug === "lido-demo") ?? body.items[0];
  return `/admin/e/${demo!.id}`;
}

function spot(page: Page, label: string) {
  return page.getByRole("button", { name: new RegExp(`^Postazione ${label}:`) });
}

test("the operational map shows statuses and opens the spot sheet", async ({ page }) => {
  const base = await openPanel(page);
  await page.goto(`${base}/mappa?date=${DEMO_DAY}`);

  await expect(page.getByRole("heading", { name: "Mappa", level: 1 })).toBeVisible();
  // Season passes on the first row (the demo pass on A1 has an open balance,
  // shown as "Da incassare" before the season colour, spec 11.3), free spots further back.
  await expect(spot(page, "A1")).toHaveAccessibleName(/Da incassare/);
  await expect(spot(page, "F8")).toHaveAccessibleName(/Libero/);
  // Map kit (ADR 0012): the art is a <use> of the shared sprite, the status
  // an overlay badge with a glyph (here the euro of "Da incassare").
  await expect(spot(page, "A1").locator('use[href="#ark-ombrellone_2_lettini"]')).toHaveCount(1);
  await expect(
    spot(page, "A1").locator('[data-badge="to_collect"] use[href="#ark-badge-euro"]'),
  ).toHaveCount(1);
  await expect(spot(page, "F8").locator("[data-badge]")).toHaveCount(0);
  // One sprite, with the symbol actually drawn.
  await expect(page.getByTestId("map-kit-defs")).toHaveCount(1);
  await expect(page.locator("#ark-ombrellone_2_lettini")).toHaveCount(1);
  // The legend always lists every status.
  const legend = page.getByText("Legenda").locator("..");
  for (const status of ["Libero", "Da incassare", "Abbonato", "Bloccato"]) {
    await expect(legend.getByText(status, { exact: true })).toBeVisible();
  }

  await spot(page, "A1").click();
  const sheet = page.getByRole("dialog", { name: /Postazione A1/ });
  await expect(sheet).toBeVisible();
  await expect(sheet.getByText("LD-2027-", { exact: false })).toBeVisible();
  await expect(sheet.getByText("Abbonamento", { exact: true })).toBeVisible();
  // Pack names, never codes.
  await expect(sheet.getByText("Base", { exact: true })).toBeVisible();
  await expect(sheet.getByRole("button", { name: "Rilascia giorno" })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(sheet).toBeHidden();
});

test("a booking created from the new booking sheet appears on the map", async ({ page }) => {
  const base = await openPanel(page);
  await page.goto(`${base}/mappa?date=${FREE_DAY}`);
  await expect(spot(page, "F7")).toHaveAccessibleName(/Libero/);

  await spot(page, "F7").click();
  await page
    .getByRole("dialog", { name: /Postazione F7/ })
    .getByRole("button", { name: "Nuova prenotazione" })
    .click();

  const form = page.getByRole("dialog", { name: "Nuova prenotazione" });
  await expect(form).toBeVisible();
  // Prefilled with the day and the spot from the map.
  await expect(form.getByText("16/06/2027 – 16/06/2027")).toBeVisible();

  await form.getByRole("combobox", { name: "Pack" }).click();
  await page.getByRole("option", { name: "Base", exact: true }).click();

  // Extras use the shared stepper: the value is always visible and it never
  // goes past the catalog maximum ("Lettino aggiuntivo": max 2 per spot).
  const sunbed = form.getByRole("spinbutton", { name: "Lettino aggiuntivo" });
  const moreBeds = form.getByRole("button", { name: "Aumenta Lettino aggiuntivo" });
  await expect(sunbed).toHaveText("0");
  await expect(form.getByText("max 2")).toBeVisible();
  for (let i = 0; i < 3; i++) if (await moreBeds.isEnabled()) await moreBeds.click();
  await expect(sunbed).toHaveAttribute("aria-valuenow", "2");
  await expect(moreBeds).toBeDisabled();
  await form.getByRole("button", { name: "Diminuisci Lettino aggiuntivo" }).click();
  await expect(sunbed).toHaveAttribute("aria-valuenow", "1");
  // Let the buttons' enable/disable transitions finish: axe samples colours
  // mid-transition otherwise (seen on WebKit in CI).
  await page.waitForFunction(() =>
    document.getAnimations().every((a) => a.playState !== "running"),
  );
  const axe = await new AxeBuilder({ page })
    .include('[role="dialog"]')
    .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"])
    .analyze();
  expect(
    axe.violations
      .filter((v) => v.impact === "serious" || v.impact === "critical")
      .map((v) => `${v.id}: ${v.help}`),
  ).toEqual([]);

  await form.getByRole("radio", { name: "Nuovo cliente" }).click();
  await form.getByLabel("Cognome").fill("Playwright");
  await form.getByRole("textbox", { name: /^Nome/ }).fill("Test");
  await form.getByLabel("Telefono").fill("+39 333 0000001");

  // The live quote shows the price; far-ahead bookings break the lead-time
  // rule, which the desk may override (audited).
  const force = form.getByLabel(/Conferma comunque/);
  await expect(form.getByText(/Totale/).first()).toBeVisible();
  if (await force.isVisible()) await force.check();

  await form.getByRole("button", { name: "Crea prenotazione" }).click();
  await expect(form).toBeHidden();

  await page.goto(`${base}/mappa?date=${FREE_DAY}`);
  await expect(spot(page, "F7")).not.toHaveAccessibleName(/Libero/);
  await spot(page, "F7").click();
  await expect(
    page.getByRole("dialog", { name: /Postazione F7/ }).getByText("Playwright T."),
  ).toBeVisible();
});

test("the cashier records a cash payment and closes the session", async ({ page }) => {
  const base = await openPanel(page);
  await page.goto(`${base}/cassa`);
  await expect(page.getByText(/Cassa principale aperta dal/)).toBeVisible();

  // Pick a demo booking with a balance due (pass of spot A15).
  await page.getByRole("button", { name: "Incassa" }).click();
  const pay = page.getByRole("dialog", { name: "Incassa prenotazione" });
  await pay.getByLabel("Codice prenotazione o cliente").fill("LD-2027-000015");
  await pay.getByRole("button", { name: /Seleziona la prenotazione LD-2027-000015/ }).click();
  await pay.getByLabel("Importo").fill("50,00");
  await pay.getByLabel("Metodo").selectOption("cash");
  await pay.getByRole("button", { name: "Registra incasso" }).click();
  await expect(page.getByText(/Incasso di 50,00\s€ registrato su LD-2027-000015/)).toBeVisible();
  await expect(pay).toBeHidden();
  await expect(
    page.getByRole("table", { name: "Incassi" }).getByText("Prenotazione LD-2027-000015"),
  ).toBeVisible();

  await page.getByRole("button", { name: "Chiudi cassa" }).click();
  const close = page.getByRole("dialog", { name: "Chiusura di cassa" });
  await close.getByLabel("Contato Contanti").fill("100,00");
  await expect(close.getByText(/Ammanco|Eccedenza|In pari/).first()).toBeVisible();
  await close.getByRole("button", { name: "Chiudi sessione" }).click();

  // Z summary, then the desk is ready for a new session.
  await expect(page.getByRole("article", { name: /Chius/ }).first()).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("form", { name: "Apri la cassa" })).toBeVisible();
});
