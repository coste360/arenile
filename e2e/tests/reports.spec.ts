import { expect, test, type Page } from "@playwright/test";
import { DEMO_YEAR } from "./demo-dates";
import { login } from "./helpers";

/**
 * M5A reports (spec 15.3) against the demo season: the KPIs of the
 * season, the year-over-year comparison with the previous year (no data there, so the
 * deltas read "Nuovo") and the CSV export of what is shown.
 */

async function openPanel(page: Page): Promise<string> {
  await login(page);
  const res = await page.request.get("/api/admin/v1/establishments");
  expect(res.ok()).toBeTruthy();
  const body = (await res.json()) as { items: { id: string; slug: string }[] };
  const demo = body.items.find((e) => e.slug === "lido-demo") ?? body.items[0];
  return `/admin/e/${demo!.id}`;
}

test("reports show the season and compare it with the previous year", async ({ page }) => {
  const base = await openPanel(page);
  await page.goto(`${base}/report`);
  await expect(page.getByRole("heading", { name: "Report", level: 1 })).toBeVisible();

  // The demo season (the only one).
  const season = page.getByLabel("Stagione", { exact: true });
  const value = await season.locator("option", { hasText: String(DEMO_YEAR) }).first().getAttribute("value");
  await season.selectOption(value as string);

  const kpis = page.getByRole("region", { name: "Indicatori del periodo" });
  await expect(kpis).toBeVisible();
  await expect(kpis.getByText(/fasce occupate su \d+/)).toBeVisible();

  // Year over year: the previous year has no data, so every change reads "Nuovo".
  await page.getByRole("switch", { name: `Confronta con il ${DEMO_YEAR - 1}` }).click();
  const metrics = page.getByRole("table", {
    name: "Indicatori a confronto con l'anno precedente",
  });
  await expect(metrics).toBeVisible();
  await expect(page.getByText("Nuovo").first()).toBeVisible();
  await expect(
    page.getByRole("table", { name: "Andamento a confronto con l'anno precedente" }),
  ).toBeVisible();

  // The comparison CSV is the spreadsheet format of spec 15.3 (BOM, ";").
  const csvLink = page.getByRole("link", { name: `Esporta CSV: Confronto con il ${DEMO_YEAR - 1}` });
  const href = await csvLink.getAttribute("href");
  expect(href).toContain(`compare_year=${DEMO_YEAR - 1}`);
  const csv = await page.request.get(href as string);
  expect(csv.ok()).toBeTruthy();
  expect(csv.headers()["content-type"]).toContain("text/csv");
  const text = (await csv.body()).toString("utf8");
  expect(text.charCodeAt(0)).toBe(0xfeff);
  expect(text.split("\r\n")[0]).toContain(";");

  // Weekday alignment is a second request with align=weekday.
  const aligned = page.waitForResponse(
    (r) => r.url().includes("/reports/comparison") && r.url().includes("align=weekday"),
  );
  await page.getByLabel("Allineamento").selectOption("weekday");
  expect((await aligned).ok()).toBeTruthy();

  // A breakdown tab with its own table.
  await page.getByRole("tab", { name: "Ricavi" }).click();
  await expect(page.getByRole("table", { name: "Ricavi" })).toBeVisible();
});
