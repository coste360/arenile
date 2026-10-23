import { expect, type APIRequestContext, type Page } from "@playwright/test";

export const DEMO_SLUG = "lido-demo";

/** The lido's own website (another origin) embedding the widget: M4 acceptance. */
export const hostPort = Number(process.env.E2E_HOST_PORT ?? 8092);
/** Fake Stripe API + hosted payment page (internal/testtools/stripefake). */
export const stripePort = Number(process.env.E2E_STRIPE_PORT ?? 12111);

/** Demo owner created by `seed demo` (password from ARENILE_ADMIN_PASSWORD, set by the config). */
export const DEMO_OWNER_EMAIL = "demo@lido-demo.example";
export const DEMO_OWNER_PASSWORD =
  process.env.E2E_OWNER_PASSWORD ?? "E2e-Demo-Owner-2026!";

/** Reads the demo establishment from the public API (name is owned by `seed demo`). */
export async function demoEstablishment(
  request: APIRequestContext,
): Promise<{ name: string; slug: string }> {
  const res = await request.get(`/api/public/v1/${DEMO_SLUG}`);
  if (!res.ok())
    throw new Error(`GET /api/public/v1/${DEMO_SLUG} -> ${res.status()}`);
  return (await res.json()) as { name: string; slug: string };
}

/** Signs in to the panel as the demo owner. */
export async function login(page: Page): Promise<void> {
  await page.goto("/admin/login");
  await page.getByLabel("Email").fill(DEMO_OWNER_EMAIL);
  await page.getByLabel("Password").fill(DEMO_OWNER_PASSWORD);
  await page.getByRole("button", { name: "Accedi" }).click();
  await expect(page).not.toHaveURL(/\/login/);
}

/** Establishment switcher in the sidebar header ("Arenile Stabilimento: <name>"). */
export function establishmentSwitcher(page: Page) {
  return page.getByRole("button", { name: /Stabilimento:\s*/ });
}
