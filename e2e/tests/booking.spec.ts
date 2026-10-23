import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { DEMO_SLUG, demoEstablishment } from "./helpers";

test("/b/lido-demo shows the establishment name", async ({ page, request }) => {
  const { name } = await demoEstablishment(request);
  await page.goto(`/b/${DEMO_SLUG}`);
  await expect(page.getByText(name).first()).toBeVisible();
  await expect(page).toHaveTitle(new RegExp(name));
  await expect(page.getByRole("button", { name: /Quando/ })).toBeVisible();

  // The active season (seed demo: May to September) is shown and bounds the calendar.
  await expect(
    page.getByText(/Stagione \d{4}: dal \d{2}\/05\/\d{4} al \d{2}\/09\/\d{4}/),
  ).toBeVisible();

  // "Quando": shadcn date range picker (calendar loaded on open), dd/MM/yyyy, never a native date input.
  const dates = page.getByRole("button", { name: /Date del soggiorno/ });
  await expect(dates).toBeVisible();
  await expect(page.locator('input[type="date"]')).toHaveCount(0);
  await dates.click();
  const grid = page.getByRole("grid").first();
  await expect(grid).toBeVisible();
  // Opens on the first bookable month: the 15th of that month is inside the season.
  await grid
    .getByRole("button", { name: /^15 / })
    .or(grid.getByRole("button", { name: /\b15 \p{L}+ \d{4}/u }))
    .first()
    .click();
  await expect(dates).toHaveText(/^15\/0[5-9]\/\d{4} – /);

  // The later steps are honest: disabled until their backend exists.
  await expect(page.getByRole("button", { name: "Avanti" })).toBeDisabled();
  await expect(page.getByRole("button", { name: /Dove/ })).toHaveAttribute(
    "aria-disabled",
    "true",
  );
});

test("/b/lido-demo has no serious accessibility violations", async ({
  page,
  request,
}) => {
  const { name } = await demoEstablishment(request);
  await page.goto(`/b/${DEMO_SLUG}`);
  await expect(page.getByText(name).first()).toBeVisible();
  const results = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"])
    .analyze();
  const serious = results.violations.filter(
    (v) => v.impact === "serious" || v.impact === "critical",
  );
  expect(serious.map((v) => `${v.id}: ${v.help}`)).toEqual([]);
});

test("/b/lido-demo works at 320px width", async ({ page, request }) => {
  const { name } = await demoEstablishment(request);
  await page.setViewportSize({ width: 320, height: 640 });
  await page.goto(`/b/${DEMO_SLUG}`);
  await expect(page.getByText(name).first()).toBeVisible();
  const overflow = await page.evaluate(
    () => document.documentElement.scrollWidth > window.innerWidth,
  );
  expect(overflow).toBe(false);
});

test("unknown slug shows not found", async ({ page }) => {
  await page.goto("/b/questo-lido-non-esiste");
  await expect(page.getByRole("alert")).toHaveText("Stabilimento non trovato.");
});
