import { expect, test } from "@playwright/test";
import { DEMO_SLUG, login } from "./helpers";

// Spec 29.2: "Configurazione → Dispositivi" at phone width (390px), the
// pairing QR, the exchange of the code by an app (API, no cookies), the
// device in the list and its revocation.
test.use({ viewport: { width: 390, height: 844 } });

test("pairs a device with the QR code and revokes it, at 390px", async ({
  page,
  playwright,
  baseURL,
}) => {
  await login(page);
  const est = await page.request.get("/api/admin/v1/establishments");
  const items = ((await est.json()) as { items: { id: string; slug: string }[] }).items;
  const id = items.find((e) => e.slug === DEMO_SLUG)?.id;
  expect(id).toBeTruthy();

  await page.goto(`/admin/e/${id}/config/devices`);
  await expect(page.getByRole("heading", { name: "Dispositivi" })).toBeVisible();
  await page.getByRole("button", { name: "Abbina un dispositivo" }).click();
  const dialog = page.getByRole("dialog");
  // The owner may pair for someone else: the default is the owner.
  await dialog.getByRole("button", { name: "Genera il QR code" }).click();
  await expect(dialog.getByRole("img", { name: "QR code di abbinamento" })).toBeVisible();
  const code = (await dialog.getByLabel("Codice di abbinamento").textContent())?.trim() ?? "";
  expect(code).toMatch(/^[0-9A-Z]{4}-[0-9A-Z]{4}$/);
  await expect(dialog.getByText(/^[45]:\d\d$/)).toBeVisible();
  const overflow = await page.evaluate(
    () => document.documentElement.scrollWidth - window.innerWidth,
  );
  expect(overflow).toBeLessThanOrEqual(0);

  // The app: a separate context without the panel cookies.
  const app = await playwright.request.newContext({ baseURL });
  const pair = await app.post("/api/device/v1/pair", {
    data: { code, name: "Telefono e2e", platform: "android", kind: "mobile" },
  });
  expect(pair.status()).toBe(201);
  const { token } = (await pair.json()) as { token: string };
  const self = await app.get("/api/device/v1/self", {
    headers: { Authorization: `Bearer ${token}` },
  });
  expect(self.status()).toBe(200);

  await expect(dialog.getByText("Dispositivo abbinato")).toBeVisible({ timeout: 10_000 });
  await dialog.getByRole("button", { name: "Fatto" }).click();
  await expect(page.getByText("Telefono e2e", { exact: true })).toBeVisible();

  await page.getByRole("button", { name: "Revoca Telefono e2e" }).click();
  await page.getByRole("alertdialog").getByRole("button", { name: "Revoca" }).click();
  await expect(page.getByText("Telefono e2e", { exact: true })).toHaveCount(0);
  const after = await app.get("/api/device/v1/self", {
    headers: { Authorization: `Bearer ${token}` },
  });
  expect(after.status()).toBe(401);
  await app.dispose();
});
