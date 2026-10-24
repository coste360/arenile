import { expect, test, type Page } from "@playwright/test";
import { login } from "./helpers";

/**
 * Map actions (ADR 0019 C) on the real binary seeded with `seed demo`: group
 * booking of several spots selected on the operational map (one booking,
 * a line per spot, atomic) and a manual block of a spot, then its removal.
 * Works on its own day and spots so the suite stays parallel.
 */

/** A season day no other test touches. */
const DAY = "2027-07-21";

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

test("three spots selected on the map become one group booking", async ({ page }, info) => {
  const base = await openPanel(page);
  await page.goto(`${base}/mappa?date=${DAY}`);
  for (const l of ["E3", "E4", "E5"]) await expect(spot(page, l)).toHaveAccessibleName(/Libero/);

  // Selection mode: tap the first spot, Shift-click the last one (range).
  await page.getByRole("button", { name: "Selezione multipla delle postazioni" }).click();
  await spot(page, "E3").click();
  await spot(page, "E5").click({ modifiers: ["Shift"] });
  const bar = page.getByTestId("selection-bar");
  await expect(bar.getByRole("status")).toHaveText("3 postazioni selezionate");
  for (const l of ["E3", "E4", "E5"]) await expect(spot(page, l)).toHaveAttribute("aria-pressed", "true");
  await page.screenshot({ path: info.outputPath("map-selection.png"), fullPage: true });

  await bar.getByRole("button", { name: "Prenotazione di gruppo" }).click();
  const form = page.getByRole("dialog", { name: "Prenotazione di gruppo (3 postazioni)" });
  await expect(form).toBeVisible();
  await form.getByRole("combobox", { name: "Pack" }).click();
  await page.getByRole("option", { name: "Base", exact: true }).click();
  await form.getByRole("radio", { name: "Nuovo cliente" }).click();
  await form.getByLabel("Cognome").fill("Gruppo");
  await form.getByRole("textbox", { name: /^Nome/ }).fill("Playwright");
  await form.getByLabel("Telefono").fill("+39 333 0000003");
  await expect(form.getByText(/Totale/).first()).toBeVisible();
  const force = form.getByLabel(/Conferma comunque/);
  if (await force.isVisible()) await force.check();
  await page.screenshot({ path: info.outputPath("group-booking-sheet.png"), fullPage: true });
  await form.getByRole("button", { name: "Crea prenotazione" }).click();
  await expect(form).toBeHidden();

  // The map shows the three spots booked by the same booking.
  for (const l of ["E3", "E4", "E5"]) await expect(spot(page, l)).not.toHaveAccessibleName(/Libero/);
  await page.getByRole("button", { name: "Esci dalla selezione" }).click();
  await expect(page.getByTestId("selection-bar")).toBeHidden();
  const codes = new Set<string>();
  for (const l of ["E3", "E4", "E5"]) {
    await spot(page, l).click();
    const sheet = page.getByRole("dialog", { name: new RegExp(`Postazione ${l}`) });
    await expect(sheet.getByText("Gruppo P.")).toBeVisible();
    codes.add((await sheet.getByText(/^LD-2027-\d+$/).textContent()) ?? "");
    await page.keyboard.press("Escape");
    await expect(sheet).toBeHidden();
  }
  expect(codes.size).toBe(1);
});

test("a spot blocked from the map shows as blocked until unblocked", async ({ page }, info) => {
  const base = await openPanel(page);
  await page.goto(`${base}/mappa?date=${DAY}`);
  await expect(spot(page, "E9")).toHaveAccessibleName(/Libero/);

  await spot(page, "E9").click();
  await page
    .getByRole("dialog", { name: /Postazione E9/ })
    .getByRole("button", { name: "Blocca postazione" })
    .click();
  const dialog = page.getByRole("dialog", { name: "Blocca la postazione E9" });
  await dialog.getByLabel(/Motivo/).fill("Ombrellone rotto");
  await page.screenshot({ path: info.outputPath("block-dialog.png"), fullPage: true });
  await dialog.getByRole("button", { name: "Blocca" }).click();
  await expect(dialog).toBeHidden();

  await expect(spot(page, "E9")).toHaveAccessibleName(/Bloccato/);
  await expect(spot(page, "E9")).toHaveAttribute("data-status", "blocked");
  await page.screenshot({ path: info.outputPath("map-blocked.png"), fullPage: true });

  await spot(page, "E9").click();
  const sheet = page.getByRole("dialog", { name: /Postazione E9/ });
  await expect(sheet.getByText("Bloccata: Ombrellone rotto")).toBeVisible();
  await sheet.getByRole("button", { name: "Sblocca" }).click();
  await page.getByRole("alertdialog").getByRole("button", { name: "Sblocca" }).click();
  await expect(spot(page, "E9")).toHaveAccessibleName(/Libero/);
});

test("map write actions without If-Match are refused with 428", async ({ page }) => {
  await openPanel(page);
  const res = await page.request.get("/api/admin/v1/establishments");
  const est = ((await res.json()) as { items: { id: string; slug: string }[] }).items.find(
    (e) => e.slug === "lido-demo",
  )!;
  const list = await page.request.get(
    `/api/admin/v1/establishments/${est.id}/bookings?status=confirmed&limit=1`,
  );
  const booking = ((await list.json()) as { items: { id: string }[] }).items[0]!;
  const cookies = await page.context().cookies();
  const csrf = cookies.find((c) => c.name === "arenile_csrf")?.value ?? "";
  for (const action of ["check-in", "no-show", "cancel"]) {
    const r = await page.request.post(
      `/api/admin/v1/establishments/${est.id}/bookings/${booking.id}/${action}`,
      { headers: { "X-CSRF-Token": csrf }, data: {} },
    );
    expect(r.status(), action).toBe(428);
    expect(((await r.json()) as { type: string }).type).toBe("precondition_required");
  }
});
