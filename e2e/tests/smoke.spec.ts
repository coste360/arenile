import { expect, test } from "@playwright/test";

test("GET /healthz reports ok", async ({ request }) => {
  const res = await request.get("/healthz");
  expect(res.ok()).toBe(true);
  expect(await res.json()).toMatchObject({ status: "ok" });
});

test("/admin/ shows the login form", async ({ page }) => {
  await page.goto("/admin/");
  await expect(page).toHaveURL(/\/admin\/login/);
  await expect(
    page.getByRole("heading", { name: "Accedi al pannello" }),
  ).toBeVisible();
  await expect(page.getByLabel("Email")).toBeVisible();
  await expect(page.getByLabel("Password")).toBeVisible();
  await expect(page.getByRole("button", { name: "Accedi" })).toBeVisible();
});

test("/admin/ rejects wrong credentials with a readable message", async ({
  page,
}) => {
  await page.goto("/admin/login");
  await page.getByLabel("Email").fill("nessuno@example.it");
  await page.getByLabel("Password").fill("password-sbagliata");
  await page.getByRole("button", { name: "Accedi" }).click();
  await expect(page.getByRole("alert")).toBeVisible();
});
