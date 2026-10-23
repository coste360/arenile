import { expect, test } from "@playwright/test";
import { establishmentSwitcher, login } from "./helpers";

/**
 * M5A sea side (spec 11): the classic template draws the sea at the top
 * with row A nearest to it; moving the sea to the bottom from the map
 * settings flips the whole layout in one versioned save, so row A stays
 * the row nearest to the sea.
 */
test("the classic map has the sea at the top and flips with the sea side", async ({
  page,
}) => {
  test.setTimeout(120_000);
  await login(page);

  // A fresh establishment with the classic template (the demo map is shared).
  await establishmentSwitcher(page).click();
  await page.getByRole("menuitem", { name: "Nuovo stabilimento" }).click();
  const name = `Lido Mare ${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`;
  await page.getByLabel("Nome", { exact: true }).fill(name);
  await page.getByRole("button", { name: "Crea stabilimento" }).click();
  await expect(page).toHaveURL(/\/admin\/e\/[0-9a-f-]{36}\/setup/);
  const establishmentId = /\/e\/([0-9a-f-]{36})\//.exec(
    page.url(),
  )?.[1] as string;
  await page.getByRole("button", { name: "Salta questo passo" }).click();
  await page
    .getByRole("radiogroup", { name: "Modelli disponibili" })
    .getByRole("radio", { name: /^Lido classico a file/ })
    .check();
  await page.getByRole("button", { name: "Usa questo modello" }).click();
  await page.getByRole("button", { name: "Crea stagione e mappa" }).click();
  await expect(page.getByText("Configurazione creata.")).toBeVisible();

  await page.goto(`/admin/e/${establishmentId}/maps`);
  await expect(page.getByRole("cell", { name: "In alto" })).toBeVisible();
  await page.getByRole("link", { name: "Apri la mappa Spiaggia" }).click();
  const canvas = page.getByRole("group", {
    name: /^Mappa con \d+ postazioni$/,
  });
  await expect(canvas).toBeVisible();
  const y = async (label: string) =>
    Number(
      await canvas
        .getByRole("button", { name: new RegExp(`^Postazione ${label},`) })
        .getAttribute("data-y"),
    );
  const a1Top = await y("A1");
  expect(a1Top).toBeLessThan(await y("B1"));

  // Map settings: the sea goes to the bottom, the layout flips with it.
  await page.getByRole("button", { name: "Impostazioni mappa" }).click();
  const settings = page.getByRole("dialog");
  await settings.getByRole("combobox", { name: "Lato del mare" }).click();
  await page.getByRole("option", { name: "In basso" }).click();
  await settings.getByRole("button", { name: "Applica" }).click();
  const confirm = page.getByRole("alertdialog", { name: "Spostare il mare?" });
  await expect(confirm).toBeVisible();
  const flip = confirm.getByRole("button", { name: "Ribalta la disposizione" });
  await expect(flip).toBeEnabled();
  await flip.click();
  await expect(confirm).toBeHidden();
  await expect(page.getByText(/postazioni · salvata$/)).toBeVisible();
  expect(await y("A1")).toBeGreaterThan(await y("B1"));

  // Persisted as one new version (the offline copy restored at reload is
  // refetched at once while online, so the saved layout is what stays).
  await page.reload();
  await expect(canvas).toBeVisible();
  await expect.poll(async () => (await y("A1")) > (await y("B1"))).toBe(true);
});
