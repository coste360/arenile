import {
  expect,
  request as playwrightRequest,
  test,
  type APIRequestContext,
  type Page,
} from "@playwright/test";
import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import {
  DEMO_OWNER_EMAIL,
  DEMO_OWNER_PASSWORD,
  DEMO_SLUG,
  stripePort,
} from "./helpers";

/**
 * M5 part B end to end (spec 7.7, 9, 10.4, 10.5, 13.4, 20, 28): season
 * rollover wizard, renewal campaign accepted from the magic link and paid
 * on the fake Stripe page (the signed webhook confirms, INV-19), waitlist
 * offer after a cancellation, customer credit used on a desk booking, and
 * the language switch of the public page and of the panel.
 *
 * The project "m5b" runs after the other projects (playwright.config.ts):
 * it creates the next season and switches the payments to Stripe, which
 * the other specs do not expect. Tests are serial: each builds on the
 * state left by the previous one (the 2028 season, the campaign).
 */

const STRIPE_SK = "sk_test_arenile_fake_0000";
const STRIPE_WHSEC = "whsec_arenile_test_secret";
const MAIL_DIR =
  process.env.ARENILE_E2E_MAIL_DIR ??
  join(process.env.ARENILE_E2E_DATA_DIR ?? "", "mail");

interface Admin {
  api: APIRequestContext;
  est: string;
  call<T = any>(method: string, path: string, body?: unknown, headers?: Record<string, string>): Promise<{ status: number; body: T; etag: string }>;
  ok<T = any>(method: string, path: string, body?: unknown, headers?: Record<string, string>): Promise<T>;
}

async function adminSession(baseURL: string): Promise<Admin> {
  const api = await playwrightRequest.newContext({ baseURL });
  const res = await api.post("/api/admin/v1/auth/login", {
    data: { email: DEMO_OWNER_EMAIL, password: DEMO_OWNER_PASSWORD },
  });
  expect(res.ok(), await res.text()).toBe(true);
  const csrf =
    (await api.storageState()).cookies.find((c) => c.name === "arenile_csrf")
      ?.value ?? "";
  const est = (await (await api.get("/api/admin/v1/establishments")).json())
    .items[0].id as string;
  const base = `/api/admin/v1/establishments/${est}`;
  const call: Admin["call"] = async (method, path, body, headers) => {
    const r = await api.fetch(base + path, {
      method,
      data: body,
      headers: {
        "X-CSRF-Token": csrf,
        "Idempotency-Key": crypto.randomUUID(),
        ...headers,
      },
    });
    const text = await r.text();
    return {
      status: r.status(),
      body: text ? JSON.parse(text) : undefined,
      etag: r.headers()["etag"] ?? "",
    };
  };
  return {
    api,
    est,
    call,
    async ok(method, path, body, headers) {
      const r = await call(method, path, body, headers);
      expect(r.status, `${method} ${path}: ${JSON.stringify(r.body)}`).toBeLessThan(300);
      return r.body;
    },
  };
}

/** PUT with the If-Match of a fresh GET. */
async function put(admin: Admin, path: string, body: unknown) {
  const cur = await admin.call("GET", path);
  return admin.ok("PUT", path, body, { "If-Match": cur.etag });
}

/** Decoded text of the .eml files whose raw text matches every needle. */
function emails(...needles: string[]): string[] {
  let files: string[] = [];
  try {
    files = readdirSync(MAIL_DIR).filter((f) => f.endsWith(".eml"));
  } catch {
    return [];
  }
  return files
    .map((f) =>
      readFileSync(join(MAIL_DIR, f), "utf8")
        .replace(/=\r?\n/g, "")
        .replace(/=([0-9A-F]{2})/g, (_, h: string) => `%${h}`),
    )
    .map((s) => {
      try {
        return decodeURIComponent(s);
      } catch {
        return s;
      }
    })
    .filter((s) => needles.every((n) => s.includes(n)));
}

/** Waits for an email and returns the first path matching re in it. */
async function linkFromEmail(re: RegExp, ...needles: string[]): Promise<string> {
  let link = "";
  await expect
    .poll(
      () => {
        for (const m of emails(...needles)) {
          const found = re.exec(m);
          if (found) link = found[0];
        }
        return link;
      },
      { timeout: 60_000, message: `email with ${needles.join(", ")}` },
    )
    .not.toBe("");
  return link;
}

/** Fake Stripe hosted page: pay, then back to Arenile. */
async function payOnFakeStripe(page: Page) {
  await page.waitForURL(new RegExp(`127\\.0\\.0\\.1:${stripePort}/c/pay/`));
  await page.getByRole("button", { name: "Paga" }).click();
  await page.waitForURL((u) => u.port !== String(stripePort));
}

/**
 * Signs the page in with the session of the API context: the suite runs
 * last, after many logins from the same address, and the login form has a
 * per-address attempt limit (spec 17.2).
 */
async function signIn(page: Page) {
  await page.context().addCookies((await admin.api.storageState()).cookies);
}

async function noHorizontalScroll(page: Page) {
  const overflow = await page.evaluate(
    () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
  );
  expect(overflow).toBeLessThanOrEqual(0);
}

test.describe.configure({ mode: "serial" });

let admin: Admin;
let sourceYear = 0;

test.beforeAll(async ({ baseURL }) => {
  admin = await adminSession(baseURL!);
  await put(admin, "/payment-settings", {
    provider: "stripe",
    mode: "test",
    stripe: { secret_key: STRIPE_SK, webhook_secret: STRIPE_WHSEC },
  });
  const seasons = (await admin.ok("GET", "/seasons")).items as { year: number }[];
  sourceYear = Math.max(...seasons.map((s) => s.year));
});

test.afterAll(async () => {
  await put(admin, "/payment-settings", { provider: "manual", mode: "test" });
  await admin.api.dispose();
});

test("season rollover wizard: preview, apply, then the renewal campaign", async ({ page }) => {
  const target = sourceYear + 1;
  await signIn(page);
  await page.goto(`/admin/e/${admin.est}/config/seasons/rollover`);
  await expect(page.getByRole("heading", { name: "Nuova stagione", level: 1 })).toBeVisible();

  // Dry run: counts, nothing created yet.
  await page.getByRole("button", { name: "Calcola anteprima" }).click();
  await expect(page.getByText(`3. Anteprima della stagione ${target}`)).toBeVisible();
  await expect(page.getByText("Nulla è stato ancora creato.", { exact: false })).toBeVisible();
  const before = (await admin.ok("GET", "/seasons")).items as { year: number }[];
  expect(before.some((s) => s.year === target)).toBe(false);

  // Apply (confirmation dialog), idempotent on the server.
  await page.getByRole("button", { name: `Crea la stagione ${target}` }).click();
  await page.getByRole("alertdialog").getByRole("button", { name: "Crea la stagione" }).click();
  await expect(page.getByText(`Stagione ${target} creata`)).toBeVisible();
  const after = (await admin.ok("GET", "/seasons")).items as { year: number; status: string }[];
  expect(after.find((s) => s.year === target)?.status).toBe("draft");

  // "Proponi i rinnovi" opens the campaign sheet on the renewals page.
  await page.getByRole("link", { name: /^Proponi i rinnovi a \d+ abbonati$/ }).click();
  await expect(page).toHaveURL(/\/rinnovi/);
  const sheet = page.getByRole("dialog", { name: "Nuova campagna" });
  await expect(sheet).toBeVisible();
  await sheet.getByRole("button", { name: "Anteprima" }).click();
  await expect(sheet.getByText(/^Verranno create \d+ proposte$/)).toBeVisible();
  await sheet.getByRole("button", { name: "Crea le proposte" }).click();
  await expect(sheet).toBeHidden();
  await expect(page.getByRole("table", { name: "Proposte di rinnovo" })).toBeVisible();
  await expect(page.getByText("In attesa").first()).toBeVisible();
});

test("renewal accepted from the magic link, paid on Stripe, on the new season's map", async ({ page }) => {
  test.setTimeout(150_000);
  const campaign = (await admin.ok("GET", "/renewal-campaigns")).items[0];
  const proposals = (
    await admin.ok("GET", `/renewal-campaigns/${campaign.id}/proposals?limit=200`)
  ).items as { id: string; spot_label: string; customer_display_name: string; booking_code: string; customer_has_email: boolean; starts_on: string }[];
  const p = proposals.find((x) => x.customer_has_email)!;

  // The desk sends this one proposal from its row menu.
  await signIn(page);
  await page.goto(`/admin/e/${admin.est}/rinnovi`);
  await page.getByPlaceholder("Cerca cliente o postazione").fill(p.spot_label);
  await page
    .getByRole("button", { name: `Azioni per ${p.spot_label}, ${p.customer_display_name}` })
    .click();
  await page.getByRole("menuitem", { name: "Invia la proposta" }).click();
  await expect(page.getByText(`Proposta inviata a ${p.customer_display_name}.`)).toBeVisible();

  // The customer opens the magic link of the email (file sink).
  const link = await linkFromEmail(
    new RegExp(`/b/${DEMO_SLUG}/m/[A-Za-z0-9_-]+`),
    "X-Arenile-Template: renewal_proposal",
    p.booking_code,
  );
  await page.goto(link);
  await expect(page.getByRole("heading", { name: `Rinnovo dell'abbonamento ${sourceYear + 1}` })).toBeVisible();
  const full = page.getByRole("radio", { name: /^Intero importo/ });
  if (await full.count()) await full.check();
  await page.getByRole("checkbox", { name: /Ho letto e accetto le condizioni/ }).check();
  await page.getByRole("button", { name: /^Rinnova e paga/ }).click();

  // Not confirmed until the signed webhook of the fake Stripe; the
  // booking.confirmed event then converts the proposal.
  await payOnFakeStripe(page);
  await expect
    .poll(async () => (await admin.ok("GET", `/renewal-proposals/${p.id}`)).status, {
      timeout: 30_000,
    })
    .toBe("converted");
  await page.goto(link);
  await expect(page.getByText("Rinnovo confermato: ci vediamo in spiaggia!")).toBeVisible();

  // The renewed pass is on the map of the new season.
  const day = p.starts_on.slice(0, 4) + "-07-01";
  await page.goto(`/admin/e/${admin.est}/mappa?date=${day}`);
  await expect(
    page.getByRole("button", { name: new RegExp(`^Postazione ${p.spot_label}: Abbonato`) }),
  ).toBeVisible();
});

test("waitlist: a cancellation frees the spot, the offer email leads to checkout", async ({ page, request }) => {
  test.setTimeout(150_000);
  await put(admin, "/waitlist-settings", {
    enabled: true,
    public_join: true,
    auto_offer: true,
    offer_hold_minutes: 60,
  });
  const packs = (await admin.ok("GET", "/packs")).items as { id: string; code: string }[];
  const pack = packs.find((x) => x.code === "base") ?? packs[0];
  const maps = (await admin.ok("GET", "/maps")).items as { id: string }[];
  const spots = (await admin.ok("GET", `/maps/${maps[0].id}`)).spots as { id: string; label: string }[];
  const spot = spots.find((s) => s.label === "F7") ?? spots[60];
  const day = `${sourceYear}-07-14`;
  const occupant = await admin.ok("POST", "/customers", { first_name: "Piero", last_name: "Occupato" });
  const booked = await admin.ok("POST", "/bookings", {
    customer_id: occupant.id,
    starts_on: day,
    ends_on: day,
    slots: "FULL",
    formula: "day",
    lines: [{ pack_id: pack.id, spot_id: spot.id, quantity: 1 }],
    status: "confirmed",
    force: true,
  });
  const bookingId = (booked.booking ?? booked).id as string;

  // The customer joins the list from the public page (202, no enumeration):
  // an online entry, so the offer follows the online payment policy.
  const email = `lara.attesa.${Date.now()}@mailbox.test`;
  const pub = await (await request.get(`/api/public/v1/${DEMO_SLUG}`)).json();
  const joined = await request.post(`/api/public/v1/${DEMO_SLUG}/waitlist`, {
    data: {
      starts_on: day,
      ends_on: day,
      slots: "FULL",
      pack_id: pack.id,
      customer: {
        first_name: "Lara",
        last_name: "Attesa",
        email,
        accept_terms_version: pub.online.terms?.version ?? "",
      },
    },
  });
  expect(joined.status(), await joined.text()).toBe(202);
  await linkFromEmail(new RegExp(`/b/${DEMO_SLUG}/m/w/[A-Za-z0-9_-]+`), "X-Arenile-Template: waitlist_joined", email);

  // The desk sees the request in the queue.
  await signIn(page);
  await page.goto(`/admin/e/${admin.est}/lista-attesa`);
  await expect(page.getByText("Attesa Lara").first()).toBeVisible();

  // The occupant cancels: the freed spot is offered to the first entry.
  const cur = await admin.call("GET", `/bookings/${bookingId}`);
  await admin.ok("POST", `/bookings/${bookingId}/cancel`, { reason: "Imprevisto", settlement: "none" }, { "If-Match": cur.etag });
  const link = await linkFromEmail(
    new RegExp(`/b/${DEMO_SLUG}/m/w/[A-Za-z0-9_-]+`),
    "X-Arenile-Template: waitlist_offer",
    email,
  );

  await page.goto(link);
  await expect(page.getByText(/^Si è liberat[ao] /)).toBeVisible();
  await page.getByRole("button", { name: "Prenota ora" }).click();

  // The normal checkout resumes at the customer step, then Stripe.
  const terms = page.getByRole("checkbox", { name: /accetto le condizioni di prenotazione/ });
  await expect(terms).toBeVisible({ timeout: 15_000 });
  const first = page.getByRole("textbox", { name: "Nome", exact: true });
  if (!(await first.inputValue())) await first.fill("Lara");
  const last = page.getByRole("textbox", { name: "Cognome" });
  if (!(await last.inputValue())) await last.fill("Attesa");
  const mail = page.getByRole("textbox", { name: "Email" });
  if (!(await mail.inputValue())) await mail.fill(email);
  await terms.check();
  await page.getByRole("button", { name: /^Paga / }).click();
  await payOnFakeStripe(page);
  await expect(page.getByRole("heading", { name: "Prenotazione confermata" })).toBeVisible({ timeout: 30_000 });

  const entries = (await admin.ok("GET", "/waitlist?limit=200")).items as { customer_display_name: string; status: string }[];
  expect(entries.find((e) => e.customer_display_name === "Attesa Lara")?.status).toBe("converted");
});

test("customer credit granted and used on a desk booking", async ({ page }) => {
  const packs = (await admin.ok("GET", "/packs")).items as { id: string; code: string }[];
  const pack = packs.find((x) => x.code === "base") ?? packs[0];
  const maps = (await admin.ok("GET", "/maps")).items as { id: string }[];
  const spots = (await admin.ok("GET", `/maps/${maps[0].id}`)).spots as { id: string; label: string }[];
  const spot = spots.find((s) => s.label === "F8") ?? spots[61];
  const cust = await admin.ok("POST", "/customers", { first_name: "Carla", last_name: "Credito" });
  const day = `${sourceYear}-07-21`;
  const created = await admin.ok("POST", "/bookings", {
    customer_id: cust.id,
    starts_on: day,
    ends_on: day,
    slots: "FULL",
    formula: "day",
    lines: [{ pack_id: pack.id, spot_id: spot.id, quantity: 1 }],
    status: "confirmed",
    force: true,
  });
  const booking = created.booking ?? created;
  expect(booking.total_amount).toBeGreaterThan(1000);

  await page.setViewportSize({ width: 390, height: 844 });
  await signIn(page);
  await page.goto(`/admin/e/${admin.est}/clienti/${cust.id}`);
  await page.getByRole("button", { name: "Riconosci credito" }).click();
  const grant = page.getByRole("dialog", { name: "Riconosci un credito" });
  await grant.getByRole("textbox", { name: "Importo" }).fill("10,00");
  await grant.getByRole("button", { name: "Riconosci" }).click();
  await expect(page.getByText("Credito di 10,00 € riconosciuto.")).toBeVisible();

  await page.getByRole("button", { name: "Usa su una prenotazione" }).click();
  const apply = page.getByRole("dialog", { name: "Usa il credito" });
  await expect(apply.getByRole("combobox", { name: "Prenotazione" })).toContainText(booking.code);
  await apply.getByRole("button", { name: "Usa il credito" }).click();
  await expect(page.getByText(`Credito di 10,00 € usato su ${booking.code}.`)).toBeVisible();
  await noHorizontalScroll(page);

  const after = await admin.ok("GET", `/bookings/${booking.id}`);
  expect((after.booking ?? after).paid_amount).toBe(1000);
  const ledger = await admin.ok("GET", `/customers/${cust.id}/credits`);
  expect(ledger.available).toBe(0);
});

test("language switch: public page and panel in German and French", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto(`/b/${DEMO_SLUG}`);
  await expect(page.getByRole("heading", { name: "Prenota il tuo posto in spiaggia" })).toBeVisible();
  const lang = page.getByRole("combobox", { name: "Lingua" });
  await lang.selectOption("de");
  await expect(page.getByRole("heading", { name: "Buchen Sie Ihren Platz am Strand" })).toBeVisible();
  await expect(page.getByRole("button", { name: /Aufenthaltsdaten/ })).toBeVisible();
  await expect(page.locator("html")).toHaveAttribute("lang", "de");
  await noHorizontalScroll(page);
  await page.getByRole("combobox", { name: "Sprache" }).selectOption("fr");
  await expect(page.getByRole("heading", { name: "Réservez votre place à la plage" })).toBeVisible();
  await expect(page.locator("html")).toHaveAttribute("lang", "fr");
  await noHorizontalScroll(page);

  await page.setViewportSize({ width: 1280, height: 800 });
  await signIn(page);
  for (const [l, today, renewals, waitlist] of [
    ["de", "Heute", "Verlängerungen", "Warteliste"],
    ["fr", "Aujourd'hui", "Renouvellements", "Liste d'attente"],
  ] as const) {
    await page.goto(`/admin/e/${admin.est}/oggi?lang=${l}`);
    await expect(page.locator("html")).toHaveAttribute("lang", l);
    await expect(page.getByRole("link", { name: today }).first()).toBeVisible();
    await expect(page.getByRole("link", { name: renewals }).first()).toBeVisible();
    await expect(page.getByRole("link", { name: waitlist }).first()).toBeVisible();
    await page.getByRole("link", { name: renewals }).first().click();
    await expect(page.getByRole("heading", { name: renewals, level: 1 })).toBeVisible();
  }
});
