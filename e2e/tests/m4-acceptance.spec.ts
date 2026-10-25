import {
  expect,
  request as playwrightRequest,
  test,
  type APIRequestContext,
  type FrameLocator,
  type Page,
} from "@playwright/test";
import { bookableDay } from "./demo-dates";
import {
  DEMO_OWNER_EMAIL,
  DEMO_OWNER_PASSWORD,
  DEMO_SLUG,
  hostPort,
  stripePort,
} from "./helpers";

/**
 * M4 acceptance (spec 28): a complete booking from a smartphone, inside an
 * iframe on the lido's own website (another origin), in under 2 minutes,
 * paid with Stripe Checkout (live-mode fake key, INV-88). The fake Stripe of
 * internal/testtools/stripefake plays the hosted payment page and posts a
 * correctly SIGNED checkout.session.completed webhook to Arenile, which is
 * the only thing that confirms the booking (INV-19): the page only polls.
 */

const HOST = `http://127.0.0.1:${hostPort}`;
/** Same server, other origin: NOT in allowed_embed_origins. */
const FOREIGN_HOST = `http://localhost:${hostPort}`;
// Live-mode fake key: real bookings never pay with test keys while the
// test mode is off (INV-88); the fake Stripe accepts it.
const STRIPE_SK = "sk_live_arenile_fake_0000";
const STRIPE_WHSEC = "whsec_arenile_test_secret";

interface Admin {
  api: APIRequestContext;
  establishmentId: string;
  put(path: string, body: unknown): Promise<void>;
}

async function adminSession(baseURL: string): Promise<Admin> {
  const api = await playwrightRequest.newContext({ baseURL });
  const login = await api.post("/api/admin/v1/auth/login", {
    data: { email: DEMO_OWNER_EMAIL, password: DEMO_OWNER_PASSWORD },
  });
  expect(login.ok(), await login.text()).toBe(true);
  const csrf = (await api.storageState()).cookies.find(
    (c) => c.name === "arenile_csrf",
  )?.value;
  const list = await api.get("/api/admin/v1/establishments");
  const establishmentId = (await list.json()).items[0].id as string;
  const base = `/api/admin/v1/establishments/${establishmentId}`;
  return {
    api,
    establishmentId,
    async put(path, body) {
      const current = await api.get(base + path);
      expect(current.ok(), await current.text()).toBe(true);
      const res = await api.put(base + path, {
        data: typeof body === "function" ? body(await current.json()) : body,
        headers: {
          "If-Match": current.headers()["etag"] ?? "",
          "X-CSRF-Token": csrf ?? "",
        },
      });
      expect(res.ok(), await res.text()).toBe(true);
    },
  };
}

test.describe.configure({ mode: "serial" });

/**
 * Steps 1-4 of the public flow inside the widget, up to the "Paga" button:
 * a single day in June of the demo season, the first free spot, the Base
 * pack, the customer's details and the accepted terms.
 */
async function bookUntilPay(
  page: Page,
  widget: FrameLocator,
  day = 16,
): Promise<void> {
  // The customer scrolls the lido's page to the widget (the calendar is a
  // popover inside the iframe: it scrolls with the host page, not alone).
  await widget.getByRole("button", { name: /Date del soggiorno/ }).waitFor();
  await page
    .locator("[data-arenile]")
    .evaluate((el) => el.scrollIntoView({ block: "start" }));

  // 1. Quando: a single free day of the demo season.
  const target = bookableDay(day);
  await widget.getByRole("button", { name: /Date del soggiorno/ }).click();
  const grid = widget.getByRole("grid").first();
  await expect(grid).toBeVisible();
  for (let i = 0; i < 6; i++) {
    if (target.month.test((await grid.getAttribute("aria-label")) ?? "")) break;
    await widget.getByRole("button", { name: /mese successivo|Next/i }).click();
  }
  await expect(grid).toHaveAttribute("aria-label", target.month);
  const dayName = target.name;
  await grid.getByRole("button", { name: dayName }).click();
  await widget
    .getByRole("grid")
    .first()
    .getByRole("button", { name: dayName })
    .click();
  await page.keyboard.press("Escape");
  await widget.getByRole("button", { name: "Avanti" }).click();

  // 2. Dove: the first free spot (list view, big tap targets).
  await widget.getByRole("button", { name: "Mostra come elenco" }).click();
  await widget
    .getByRole("button", { name: /^\S+ Libero/ })
    .first()
    .click();
  await widget.getByRole("button", { name: "Avanti" }).click();

  // 3. Cosa: the Base pack.
  await widget.getByRole("radio", { name: /^Base/ }).click();
  await widget.getByRole("button", { name: "Continua" }).click();

  // 4. Chi e pagamento: details and terms; the caller presses "Paga".
  await widget
    .getByRole("textbox", { name: "Nome", exact: true })
    .fill("Giulia");
  await widget.getByRole("textbox", { name: "Cognome" }).fill("Esposito");
  await widget
    .getByRole("textbox", { name: "Email" })
    .fill("giulia.esposito@mailbox.test");
  await widget
    .getByRole("checkbox", { name: /accetto le condizioni di prenotazione/ })
    .check();
}

test.describe("M4 acceptance: embedded booking on another origin", () => {
  let admin: Admin;

  test.beforeAll(async ({ baseURL }) => {
    admin = await adminSession(baseURL!);
    // The lido's website may frame the page; Stripe in live mode with the
    // fake's credentials (ARENILE_STRIPE_API_URL points at the fake).
    await admin.put(
      "/online-settings",
      (s: {
        booking_enabled: boolean;
        allowed_embed_origins: string[];
        theme: unknown;
        flow: unknown;
      }) => ({
        booking_enabled: true,
        allowed_embed_origins: [...new Set([...s.allowed_embed_origins, HOST])],
        theme: s.theme,
        flow: s.flow,
      }),
    );
    await admin.put("/payment-settings", {
      provider: "stripe",
      mode: "live",
      stripe: { secret_key: STRIPE_SK, webhook_secret: STRIPE_WHSEC },
    });
  });

  test.afterAll(async () => {
    await admin.put("/payment-settings", { provider: "manual", mode: "test" });
    await admin.api.dispose();
  });

  test("frame-ancestors lets the allowed site frame the page and blocks the others", async ({
    page,
    request,
  }) => {
    const res = await request.get(`/b/${DEMO_SLUG}?embed=1`);
    const csp = res.headers()["content-security-policy"] ?? "";
    expect(csp).toContain("frame-ancestors");
    expect(csp).toContain(HOST);
    expect(csp).not.toContain(FOREIGN_HOST);

    // The allowed origin loads the widget (so a block below is not a timing
    // artefact)...
    await page.goto(HOST);
    await page
      .frameLocator("[data-arenile] iframe")
      .getByRole("button", { name: /Date del soggiorno/ })
      .waitFor();

    // ...and the browser refuses the frame on the foreign origin: the CSP
    // violation is reported, the frame never renders the page.
    const violations: string[] = [];
    page.on("console", (msg) => {
      if (/frame-ancestors|Refused to (frame|load|display)/i.test(msg.text()))
        violations.push(msg.text());
    });
    await page.goto(FOREIGN_HOST);
    const iframe = page.locator("[data-arenile] iframe");
    await expect(iframe).toHaveCount(1);
    await expect
      .poll(() => violations.length, { timeout: 10_000 })
      .toBeGreaterThan(0);
    const child = page.frames().find((f) => f !== page.mainFrame());
    const text = child
      ? await child
          .locator("body")
          .innerText({ timeout: 2000 })
          .catch(() => "")
      : "";
    expect(text).not.toContain("Prenota il tuo posto in spiaggia");
    expect(text).not.toContain("Date del soggiorno");
  });

  test("the host page can intercept arenile:navigate (preventDefault) before the payment", async ({
    page,
  }) => {
    test.setTimeout(150_000);
    await page.goto(`${HOST}/?intercept=1`);
    const widget = page.frameLocator("[data-arenile] iframe");
    // Another day than the booking above (serial): same free spots.
    await bookUntilPay(page, widget, 17);
    await widget.getByRole("button", { name: /^Paga / }).click();
    // The site handled the event: no navigation, the payment URL is in its hands.
    await expect
      .poll(() =>
        page.evaluate(
          () => (window as unknown as { __navigate: string | null }).__navigate,
        ),
      )
      .toMatch(new RegExp(`127\\.0\\.0\\.1:${stripePort}/c/pay/`));
    await page.waitForTimeout(500);
    expect(new URL(page.url()).origin).toBe(HOST);
  });

  test("complete booking from a smartphone inside the iframe in under 2 minutes", async ({
    page,
  }) => {
    test.setTimeout(150_000);
    const started = Date.now();
    await page.goto(HOST);
    const widget = page.frameLocator("[data-arenile] iframe");
    await bookUntilPay(page, widget);
    await widget.getByRole("button", { name: /^Paga / }).click();

    // The fake Stripe hosted page (top window), then back to the lido's site.
    await page.waitForURL(new RegExp(`127\\.0\\.0\\.1:${stripePort}/c/pay/`));
    await page.getByRole("button", { name: "Paga" }).click();
    await page.waitForURL((u) => u.origin === HOST);
    expect(page.url()).not.toContain("arenile_checkout"); // embed.js cleans the address

    // Confirmation inside the iframe: code and QR, from the signed webhook.
    await expect(
      widget.getByRole("heading", { name: "Prenotazione confermata" }),
    ).toBeVisible({ timeout: 30_000 });
    const code = widget.getByText(/^LD-\d{4}-\d{6}$/);
    await expect(code).toBeVisible();
    await expect(
      widget.getByRole("img", { name: /QR da mostrare/ }),
    ).toBeVisible();
    const elapsed = Date.now() - started;
    test.info().annotations.push({
      type: "booking-time",
      description: `${(elapsed / 1000).toFixed(1)} s`,
    });
    console.log(
      `M4 acceptance: embedded booking confirmed in ${(elapsed / 1000).toFixed(1)} s`,
    );
    expect(elapsed).toBeLessThan(120_000);

    // The host page received arenile:booked with the code only (no PII).
    await expect
      .poll(() =>
        page.evaluate(
          () => (window as unknown as { __booked: unknown }).__booked,
        ),
      )
      .toMatchObject({ type: "arenile:booked", code: await code.innerText() });
  });
});
