import { expect, test } from "@playwright/test";

/**
 * M6 acceptance (spec 28): from the first start of the desktop app to the
 * first desk booking, in the browser opened by the app. Prints the elapsed
 * time; the spec target is "within 15 minutes" for a person, the script
 * gives the floor of the flow.
 */
const OWNER = {
  organization: "Lido Collaudo srl",
  name: "Titolare Collaudo",
  email: "titolare@collaudo.example",
  // Test-only password, local throwaway data folder.
  password: "Collaudo-Desktop-2026!",
};

test("first run, guided setup and first desk booking", async ({ page }) => {
  test.setTimeout(180_000);
  const openURL = process.env.DESKTOP_OPEN_URL;
  test.skip(!openURL, "DESKTOP_OPEN_URL not set");
  const t0 = Date.now();

  // First run: owner from the one-time link opened by the tray.
  await page.goto(openURL as string);
  await expect(
    page.getByRole("heading", { name: "Benvenuto in Arenile" }),
  ).toBeVisible();
  await page.getByLabel("Azienda o stabilimento").fill(OWNER.organization);
  await page.getByLabel("Nome e cognome").fill(OWNER.name);
  await page.getByLabel("Email").fill(OWNER.email);
  await page.getByLabel("Password", { exact: true }).fill(OWNER.password);
  await page.getByLabel("Ripeti la password").fill(OWNER.password);
  await page.getByRole("button", { name: "Crea l'utente e continua" }).click();
  const tOwner = Date.now();

  // New establishment, then the guided setup (minimal path).
  await expect(
    page.getByRole("heading", { name: "Nuovo stabilimento" }),
  ).toBeVisible();
  await page.getByLabel("Nome", { exact: true }).fill("Lido Collaudo");
  await page.getByRole("button", { name: "Crea stabilimento" }).click();
  await expect(page).toHaveURL(/\/admin\/e\/[0-9a-f-]{36}\/setup/);
  const id = /\/e\/([0-9a-f-]{36})\//.exec(page.url())?.[1] as string;
  await page.getByRole("button", { name: "Salta questo passo" }).click();
  await page
    .getByRole("radiogroup", { name: "Modelli disponibili" })
    .getByRole("radio", { name: /^Lido classico a file/ })
    .check();
  await page.getByRole("button", { name: "Usa questo modello" }).click();
  await page.getByRole("button", { name: "Crea stagione e mappa" }).click();
  await expect(page.getByText("Configurazione creata.")).toBeVisible();
  await page.getByRole("button", { name: "Continua" }).click();
  const cells = page.getByRole("textbox", { name: /^Prezzo giornaliero / });
  await expect(cells.first()).toBeVisible();
  for (let i = 0, n = await cells.count(); i < n; i++)
    await cells.nth(i).fill("25");
  await page
    .getByRole("button", { name: "Genera le regole di prezzo" })
    .click();
  await expect(
    page.getByText(/regole? di prezzo \(sostituite: 0\)/),
  ).toBeVisible();
  const tSetup = Date.now();

  // First desk booking on the opening day of the season.
  const seasons = await page.request.get(
    `/api/admin/v1/establishments/${id}/seasons`,
  );
  expect(seasons.ok()).toBeTruthy();
  const day = ((await seasons.json()) as { items: { opens_on: string }[] })
    .items[0]?.opens_on;
  expect(day).toBeTruthy();
  await page.goto(`/admin/e/${id}/mappa?date=${day}`);
  const a1 = page.getByRole("button", { name: /^Postazione A1:/ });
  await expect(a1).toHaveAccessibleName(/Libero/);
  await a1.click();
  await page
    .getByRole("dialog", { name: /Postazione A1/ })
    .getByRole("button", { name: "Nuova prenotazione" })
    .click();
  const form = page.getByRole("dialog", { name: "Nuova prenotazione" });
  await form.getByRole("combobox", { name: "Pack" }).click();
  await page.getByRole("option").first().click();
  await form.getByRole("radio", { name: "Nuovo cliente" }).click();
  await form.getByLabel("Cognome").fill("Rossi");
  await form.getByRole("textbox", { name: /^Nome/ }).fill("Mario");
  await form.getByLabel("Telefono").fill("+39 333 0000002");
  await expect(form.getByText(/Totale/).first()).toBeVisible();
  const force = form.getByLabel(/Conferma comunque/);
  if (await force.isVisible()) await force.check();
  await form.getByRole("button", { name: "Crea prenotazione" }).click();
  await expect(form).toBeHidden();
  await page.goto(`/admin/e/${id}/mappa?date=${day}`);
  await expect(
    page.getByRole("button", { name: /^Postazione A1:/ }),
  ).not.toHaveAccessibleName(/Libero/);
  const tBooking = Date.now();

  const s = (ms: number) => `${(ms / 1000).toFixed(1)}s`;
  console.log(
    `M6 acceptance: owner ${s(tOwner - t0)}, setup ${s(tSetup - tOwner)}, ` +
      `first booking ${s(tBooking - tSetup)}, total ${s(tBooking - t0)}`,
  );
});
