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

  // Only the first day is chosen: the next steps stay locked until the range is complete.
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

test("/b/lido-demo has no serious accessibility violations in dark mode", async ({
  page,
  request,
}) => {
  const { name } = await demoEstablishment(request);
  await page.goto(`/b/${DEMO_SLUG}?theme=dark`);
  await expect(page.getByText(name).first()).toBeVisible();
  await expect(page.locator("html")).toHaveClass(/dark/);
  const results = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"])
    .analyze();
  const serious = results.violations.filter(
    (v) => v.impact === "serious" || v.impact === "critical",
  );
  expect(serious.map((v) => `${v.id}: ${v.help}`)).toEqual([]);
});

test("/b/lido-demo works at 320px width through the four steps", async ({
  page,
  request,
}) => {
  const { name } = await demoEstablishment(request);
  // One day per project: the projects run in parallel against the same
  // instance and would otherwise race for the same first free spot.
  const day =
    { chromium: 21, mobile: 22, "mobile-safari": 23 }[
      test.info().project.name
    ] ?? 24;
  const dayName = new RegExp(`\\b${day} giugno`);
  await page.setViewportSize({ width: 320, height: 640 });
  await page.goto(`/b/${DEMO_SLUG}`);
  await expect(page.getByText(name).first()).toBeVisible();
  const noSidewaysScroll = async (step: string) => {
    const widths = await page.evaluate(() => {
      const inner = window.innerWidth;
      // Elements reaching past the viewport: named in the failure message.
      const offenders = [...document.querySelectorAll<HTMLElement>("body *")]
        .map((el) => ({ el, right: el.getBoundingClientRect().right }))
        .filter(({ right }) => right > inner + 1)
        .slice(0, 8)
        .map(
          ({ el, right }) =>
            `${el.tagName.toLowerCase()}[${(el.getAttribute("class") ?? "").slice(0, 70)}] right=${Math.round(right)}`,
        );
      return { scroll: document.documentElement.scrollWidth, inner, offenders };
    });
    expect(
      widths.scroll,
      `${step}: scrollWidth ${widths.scroll} > ${widths.inner}\n${widths.offenders.join("\n")}`,
    ).toBeLessThanOrEqual(widths.inner);
  };
  await noSidewaysScroll("1 Quando");

  // 1. Quando: a single day in June.
  await page.getByRole("button", { name: /Date del soggiorno/ }).click();
  const grid = page.getByRole("grid").first();
  await expect(grid).toBeVisible();
  for (let i = 0; i < 6; i++) {
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
  await noSidewaysScroll("1 Quando (date chosen)");
  await page.getByRole("button", { name: "Avanti" }).click();

  // 2. Dove: list view, first free spot.
  // Parallel runs of this test share the instance: a free spot at random,
  // so two of them do not race for the same one.
  await page.getByRole("button", { name: "Mostra come elenco" }).click();
  await noSidewaysScroll("2 Dove");
  const free = page.getByRole("button", { name: / – Libero/ });
  await expect(free.first()).toBeVisible();
  await free.nth(Math.floor(Math.random() * (await free.count()))).click();
  await page.getByRole("button", { name: "Avanti" }).click();

  // 3. Cosa.
  await page.getByRole("radio", { name: /^Base/ }).click();
  await noSidewaysScroll("3 Cosa");
  await page.getByRole("button", { name: "Continua" }).click();

  // 4. Chi e pagamento, with the terms opened (the version label and the
  // long texts must wrap).
  await expect(
    page.getByRole("textbox", { name: "Nome", exact: true }),
  ).toBeVisible();
  await noSidewaysScroll("4 Chi");
  await page.getByRole("button", { name: "Leggi le condizioni" }).click();
  await noSidewaysScroll("4 Chi (terms open)");
  // The summary shows one date for a single day.
  await expect(
    page.getByText(new RegExp(`^${day}/06/\\d{4}$`)).first(),
  ).toBeVisible();
});

test("unknown slug shows not found", async ({ page }) => {
  await page.goto("/b/questo-lido-non-esiste");
  await expect(page.getByRole("alert")).toHaveText("Stabilimento non trovato.");
});
