import { expect, test, type Page } from "@playwright/test";
import { DEMO_YEAR } from "./demo-dates";
import { login } from "./helpers";

/**
 * Deferred features (ADR 0019 F) end to end: CSV import of customers from
 * the panel (upload, preview with the validation report, apply all or
 * nothing, same file refused the second time) and the Excel exports of the
 * reports. Test mode is covered by internal/acceptance (switching it on
 * here would mark the bookings of the parallel tests as test rows).
 */

async function openPanel(page: Page): Promise<{ base: string; api: string }> {
  await login(page);
  const res = await page.request.get("/api/admin/v1/establishments");
  expect(res.ok()).toBeTruthy();
  const body = (await res.json()) as { items: { id: string; slug: string }[] };
  const demo = body.items.find((e) => e.slug === "lido-demo") ?? body.items[0];
  return {
    base: `/admin/e/${demo!.id}`,
    api: `/api/admin/v1/establishments/${demo!.id}`,
  };
}

test("customers imported from a CSV after the preview, once", async ({
  page,
}, info) => {
  const { base } = await openPanel(page);
  const tag = `${info.project.name}-${Date.now()}`;
  const csv = [
    "cognome;nome;email;consenso_marketing",
    `Importato;Uno;uno-${tag}@example.org;si`,
    `Importato;Due;due-${tag}@example.org;no`,
    `;Senza cognome;tre-${tag}@example.org;no`,
  ].join("\r\n");
  await page.goto(`${base}/config/import`);
  await expect(
    page.getByRole("heading", { name: "Importa dati", level: 1 }),
  ).toBeVisible();
  await page.getByLabel("File CSV").setInputFiles({
    name: `clienti-${tag}.csv`,
    mimeType: "text/csv",
    buffer: Buffer.from(csv, "utf8"),
  });
  await page.getByRole("button", { name: "Carica e verifica" }).click();
  await page.getByRole("button", { name: "Anteprima" }).click();

  // The row without a last name is reported by row and column; apply stays off.
  const errors = page.getByRole("table", { name: "Errori" });
  await expect(errors).toBeVisible();
  await expect(errors.getByRole("row").nth(1)).toContainText("4");
  await expect(errors.getByRole("row").nth(1)).toContainText("cognome");
  await expect(page.getByRole("button", { name: "Applica" })).toBeDisabled();

  // Fix the file (new upload of the two good rows) and apply.
  await page.getByRole("button", { name: "Scarta" }).click();
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: "Scarta" })
    .click();
  const good = csv.split("\r\n").slice(0, 3).join("\r\n");
  await page.getByLabel("File CSV").setInputFiles({
    name: `clienti-${tag}-ok.csv`,
    mimeType: "text/csv",
    buffer: Buffer.from(good, "utf8"),
  });
  await page.getByRole("button", { name: "Carica e verifica" }).click();
  await page.getByRole("button", { name: "Anteprima" }).click();
  await expect(page.getByRole("button", { name: "Applica" })).toBeEnabled();
  await page.getByRole("button", { name: "Applica" }).click();
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: "Applica" })
    .click();
  await expect(
    page.getByText("Creati 2, aggiornati 0, saltati 0."),
  ).toBeVisible();

  // The customers exist.
  await page.goto(`${base}/clienti`);
  await page.getByRole("searchbox").first().fill(`uno-${tag}`);
  await expect(
    page
      .getByText("Importato Uno")
      .or(page.getByText("Importato, Uno"))
      .first(),
  ).toBeVisible();

  // The same file again is refused at apply (import_already_applied).
  await page.goto(`${base}/config/import`);
  await page.getByLabel("File CSV").setInputFiles({
    name: `clienti-${tag}-ok.csv`,
    mimeType: "text/csv",
    buffer: Buffer.from(good, "utf8"),
  });
  await page.getByRole("button", { name: "Carica e verifica" }).click();
  await page.getByRole("button", { name: "Anteprima" }).click();
  const apply = page.getByRole("button", { name: "Applica" });
  await expect(apply).toBeEnabled();
  await apply.click();
  await page.getByRole("alertdialog").getByRole("button", { name: "Applica" }).click();
  await expect(page.getByRole("alertdialog")).toContainText("Questo file è già stato importato");
});

test("reports and lists download as Excel", async ({ page }) => {
  const { base, api } = await openPanel(page);
  await page.goto(`${base}/report`);
  await page.getByRole("button", { name: "Esporta elenchi" }).click();
  const link = page.getByRole("menuitem", { name: "Excel" }).first();
  const href = await link.getAttribute("href");
  expect(href).toContain("/exports/customers?format=xlsx");
  const res = await page.request.get(href as string);
  expect(res.ok()).toBeTruthy();
  expect(res.headers()["content-type"]).toContain("spreadsheetml");
  const body = await res.body();
  expect(body.subarray(0, 2).toString("latin1")).toBe("PK");

  const corr = await page.request.get(
    `${api}/fiscal/corrispettivi?from=${DEMO_YEAR}-07-01&to=${DEMO_YEAR}-07-31&group_by=day&format=xlsx`,
  );
  expect(corr.ok()).toBeTruthy();
  expect((await corr.body()).subarray(0, 2).toString("latin1")).toBe("PK");
});
