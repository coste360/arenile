import { expect, type APIRequestContext, type Cookie, type Page } from "@playwright/test";

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

/**
 * Session cookies of the last sign-in of this worker. The server allows 30
 * login attempts per IP every 15 minutes (spec 22): reusing the session
 * keeps the whole suite (two projects, parallel workers) under the limit.
 */
let ownerCookies: Cookie[] | undefined;

/** Signs in to the panel as the demo owner (reusing this worker's session when still valid). */
export async function login(page: Page): Promise<void> {
  if (ownerCookies) {
    await page.context().addCookies(ownerCookies);
    await page.goto("/admin/");
    await page.waitForLoadState("domcontentloaded");
    const me = await page.request.get("/api/admin/v1/auth/me");
    if (me.ok()) {
      await expect(page).not.toHaveURL(/\/login/);
      return;
    }
    // Logged out meanwhile (a test signed out): sign in again.
    await page.context().clearCookies();
  }
  await page.goto("/admin/login");
  await page.getByLabel("Email").fill(DEMO_OWNER_EMAIL);
  await page.getByLabel("Password").fill(DEMO_OWNER_PASSWORD);
  await page.getByRole("button", { name: "Accedi" }).click();
  await expect(page).not.toHaveURL(/\/login/);
  ownerCookies = await page.context().cookies();
}

/** Establishment switcher in the sidebar header ("Arenile Stabilimento: <name>"). */
export function establishmentSwitcher(page: Page) {
  return page.getByRole("button", { name: /Stabilimento:\s*/ });
}
