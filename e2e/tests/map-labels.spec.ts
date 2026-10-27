import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { DEMO_DAY } from "./demo-dates";
import { login } from "./helpers";

/**
 * Occupant labels on the operational map (spec 11.3): who occupies a spot,
 * the "Etichette" toggles (Codice / Cliente / Fine / Allestimento, combinable,
 * spec 30.4) remembered per user,
 * hidden below the legibility threshold with the zoom hint. Demo data: the
 * season pass on A1 belongs to a customer named Rossi.
 */


async function openMap(page: Page): Promise<void> {
  await login(page);
  const res = await page.request.get("/api/admin/v1/establishments");
  expect(res.ok()).toBeTruthy();
  const body = (await res.json()) as { items: { id: string; slug: string }[] };
  const demo = body.items.find((e) => e.slug === "lido-demo") ?? body.items[0];
  await page.goto(`/admin/e/${demo!.id}/mappa?date=${DEMO_DAY}`);
  await expect(
    page.getByRole("heading", { name: "Mappa", level: 1 }),
  ).toBeVisible();
}

function spot(page: Page, label: string) {
  return page.getByRole("button", {
    name: new RegExp(`^Postazione ${label}[,:]`),
  });
}

/** Caption pill of a spot: drawn in the caption layer, outside the spot button (label in name). */
function caption(page: Page, label: string) {
  return page.locator(
    `[data-caption-spot="${label}"] [data-map-label="caption"]`,
  );
}
const code = (page: Page, label: string) =>
  spot(page, label).locator('[data-map-label="code"]');

async function seriousViolations(page: Page): Promise<string[]> {
  const results = await new AxeBuilder({ page })
    .include('[data-testid="beach-map"]')
    .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"])
    .analyze();
  return results.violations
    .filter((v) => v.impact === "serious" || v.impact === "critical")
    .map(
      (v) =>
        `${v.id}: ${v.help} (${v.nodes.map((n) => n.target.join(" ")).join(", ")})`,
    );
}

test.describe("desktop", () => {
  test.use({ viewport: { width: 1440, height: 900 } });

  test("booked spots show the occupant surname and the toggle switches the labels", async ({
    page,
  }) => {
    await openMap(page);
    // Accessible name and tooltip carry the occupant; the pill shows the surname.
    await expect(spot(page, "A1")).toHaveAccessibleName(/occupato da Rossi/);
    await expect(caption(page, "A1")).toHaveText("Rossi");
    await expect(code(page, "A1")).toHaveText("A1");
    await expect(spot(page, "A1").locator("title")).toHaveText(
      /occupato da Rossi/,
    );
    // Free spots: code only.
    await expect(caption(page, "F8")).toHaveCount(0);

    const labels = () => page.getByRole("toolbar", { name: "Etichette" });
    const part = (name: string) => labels().getByRole("button", { name, exact: true });
    // Customer only.
    await part("Codice").click();
    await expect(part("Codice")).toHaveAttribute("aria-pressed", "false");
    await expect(code(page, "A1")).toHaveCount(0);
    await expect(caption(page, "A1")).toHaveText("Rossi");
    await expect(code(page, "F8")).toHaveText("F8");

    // Code only.
    await part("Codice").click();
    await part("Cliente").click();
    await expect(caption(page, "A1")).toHaveCount(0);
    await expect(code(page, "A1")).toHaveText("A1");

    // Remembered per user across reloads.
    await page.reload();
    await expect(part("Codice")).toHaveAttribute("aria-pressed", "true");
    await expect(part("Cliente")).toHaveAttribute("aria-pressed", "false");
    await expect(caption(page, "A1")).toHaveCount(0);
    await part("Cliente").click();
    await expect(caption(page, "A1")).toHaveText("Rossi");

    expect(await seriousViolations(page)).toEqual([]);
  });

  test("the list view names the occupant", async ({ page }) => {
    await openMap(page);
    await page.getByRole("tab", { name: "Elenco" }).click();
    await expect(
      page.getByRole("button", { name: /^A1 .* – occupato da Rossi/ }),
    ).toBeVisible();
  });
});

test.describe("mobile", () => {
  test.use({
    viewport: { width: 390, height: 844 },
    hasTouch: true,
    isMobile: true,
  });

  test("names wait for the zoom, the toggles wrap", async ({ page }) => {
    await openMap(page);
    await expect(spot(page, "A1")).toHaveAccessibleName(/occupato da Rossi/);
    // Too small to read at the fitted zoom: no pill, the hint says how to get them.
    await expect(caption(page, "A1")).toHaveCount(0);
    await expect(page.getByTestId("beach-map-zoom-hint")).toBeVisible();

    const zoomIn = page.getByRole("button", { name: /Ingrandisci/ }).first();
    for (let i = 0; i < 6 && (await caption(page, "A1").count()) === 0; i++)
      await zoomIn.click();
    await expect(caption(page, "A1")).toHaveText(/^Ross/);

    // Customer only (Codice switched off).
    await page
      .getByRole("toolbar", { name: "Etichette" })
      .getByRole("button", { name: "Codice", exact: true })
      .click();
    await expect(code(page, "A1")).toHaveCount(0);
    await expect(caption(page, "A1")).toHaveText(/^Ross/);

    expect(await seriousViolations(page)).toEqual([]);
  });
});
