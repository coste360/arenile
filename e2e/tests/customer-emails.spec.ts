import { expect, test, type Page } from "@playwright/test";
import { login } from "./helpers";

/**
 * "Email ai clienti" (ADR 0022) end to end: the reminders card and the
 * template editor load from the real API, the live preview follows the
 * typed texts with the server rendering (sample data, sandboxed iframe), an
 * unknown placeholder is flagged, and the page fits a 390 px and a 320 px
 * phone without horizontal scrolling. Nothing is saved: the demo templates
 * are shared by the parallel tests.
 */

async function openPage(page: Page): Promise<void> {
  await login(page);
  const res = await page.request.get("/api/admin/v1/establishments");
  expect(res.ok()).toBeTruthy();
  const body = (await res.json()) as { items: { id: string; slug: string }[] };
  const demo = body.items.find((e) => e.slug === "lido-demo") ?? body.items[0];
  await page.goto(`/admin/e/${demo!.id}/config/customer-emails`);
  await expect(
    page.getByRole("heading", { name: "Email ai clienti", level: 1 }),
  ).toBeVisible();
}

test("customer emails: live preview of edited texts and placeholder check", async ({
  page,
}) => {
  await openPage(page);
  const reminders = page.getByRole("region", { name: "Promemoria automatici" });
  await expect(
    reminders.getByRole("switch", { name: "Promemoria il giorno prima" }),
  ).toBeVisible();

  const editor = page.getByRole("region", { name: "Testi delle email" });
  await editor
    .getByLabel("Email", { exact: true })
    .selectOption("day_before_reminder");
  const subject = editor.getByLabel("Oggetto");
  await expect(subject).toHaveValue(/\{stabilimento\}/);
  await subject.fill("A domani, {nome}! Codice {codice}");
  await expect(editor.getByTestId("preview-subject")).toHaveText(
    /^A domani, \S+! Codice PROVA-\d{4}-000123$/,
    { timeout: 10_000 },
  );
  const frame = editor.getByTitle("Anteprima dell'email");
  await expect(frame).toHaveAttribute("sandbox", "");
  await expect(
    page
      .frameLocator('iframe[title="Anteprima dell\'email"]')
      .getByRole("heading", { name: "Ci vediamo domani!" }),
  ).toBeVisible();

  await editor.getByLabel("Testo iniziale").fill("Ciao {telefono}");
  await expect(editor.getByText(/Segnaposto sconosciuto/)).toBeVisible();
});

for (const width of [390, 320]) {
  test(`customer emails: no horizontal scroll at ${width} px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 800 });
    await openPage(page);
    await expect(
      page.getByRole("region", { name: "Testi delle email" }).getByLabel("Oggetto"),
    ).toBeVisible();
    const overflow = await page.evaluate(
      () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
    );
    expect(overflow).toBeLessThanOrEqual(0);
    // Nothing is clipped inside the cards either.
    for (const label of ["Oggetto", "Firma"]) {
      const box = await page
        .getByRole("region", { name: "Testi delle email" })
        .getByLabel(label)
        .boundingBox();
      expect(box!.x + box!.width).toBeLessThanOrEqual(width);
    }
  });
}
