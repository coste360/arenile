import { expect, test, type Page } from "@playwright/test";
import { createHmac, timingSafeEqual } from "node:crypto";
import { createServer, type IncomingHttpHeaders, type Server } from "node:http";
import type { AddressInfo } from "node:net";
import { login } from "./helpers";

/**
 * M5A integrations (spec 17, 18.2, 22) from the panel against the real
 * binary: an API key created in "Chiavi API" authenticates a call to the
 * admin API within its scopes; a webhook endpoint created in "Webhook"
 * receives a signed test delivery on a local receiver (allowed by
 * ARENILE_WEBHOOKS_ALLOW_PRIVATE in the e2e config) and shows it in the log.
 */

async function openPanel(page: Page): Promise<{ base: string; id: string }> {
  await login(page);
  const res = await page.request.get("/api/admin/v1/establishments");
  expect(res.ok()).toBeTruthy();
  const body = (await res.json()) as { items: { id: string; slug: string }[] };
  const demo = body.items.find((e) => e.slug === "lido-demo") ?? body.items[0];
  return { base: `/admin/e/${demo!.id}`, id: demo!.id };
}

test("an API key from the panel calls the admin API within its scopes", async ({
  page,
  playwright,
  baseURL,
}) => {
  const { base, id } = await openPanel(page);
  await page.goto(`${base}/config/api-keys`);
  await expect(page.getByRole("heading", { name: "Chiavi API", level: 1 })).toBeVisible();

  const name = `E2E ${Date.now().toString(36)}`;
  await page.getByRole("button", { name: "Nuova chiave" }).click();
  const form = page.getByRole("dialog", { name: "Nuova chiave" });
  await form.getByLabel("Nome").fill(name);
  await form.getByRole("checkbox", { name: "Prenotazioni (lettura)" }).check();
  await form.getByRole("button", { name: "Crea chiave" }).click();

  const created = page.getByRole("dialog", { name: "Chiave creata" });
  const secret = await created.getByLabel("Chiave API").inputValue();
  expect(secret).toMatch(/^ar_/);
  await created.getByRole("button", { name: "Ho salvato, chiudi" }).click();
  await expect(page.getByRole("table", { name: "Chiavi API" }).getByText(name)).toBeVisible();

  // A client without cookies: only the bearer key.
  const api = await playwright.request.newContext({
    baseURL,
    extraHTTPHeaders: { Authorization: `Bearer ${secret}` },
  });
  try {
    const bookings = await api.get(`/api/admin/v1/establishments/${id}/bookings`);
    expect(bookings.status()).toBe(200);
    // Out of scope (bookings.read only) and never for keys: managing keys.
    const keys = await api.get(`/api/admin/v1/establishments/${id}/api-keys`);
    expect(keys.status()).toBe(403);

    // Revoked from the panel: refused on the next request.
    await page.getByRole("button", { name: `Revoca la chiave ${name}` }).click();
    await page.getByRole("alertdialog").getByRole("button", { name: "Revoca" }).click();
    await expect(page.getByRole("alertdialog")).toBeHidden();
    const after = await api.get(`/api/admin/v1/establishments/${id}/bookings`);
    expect(after.status()).toBe(401);
  } finally {
    await api.dispose();
  }
});

interface Received {
  headers: IncomingHttpHeaders;
  body: string;
}

/** A local webhook receiver that records every request and answers 204. */
async function startReceiver(): Promise<{ server: Server; port: number; received: Received[] }> {
  const received: Received[] = [];
  const server = createServer((req, res) => {
    const chunks: Buffer[] = [];
    req.on("data", (c: Buffer) => chunks.push(c));
    req.on("end", () => {
      received.push({ headers: req.headers, body: Buffer.concat(chunks).toString("utf8") });
      res.statusCode = 204;
      res.end();
    });
  });
  // Dual stack: "localhost" may resolve to ::1 or 127.0.0.1.
  await new Promise<void>((resolve) => server.listen(0, "::", resolve));
  return { server, port: (server.address() as AddressInfo).port, received };
}

/** Verifies "t=<ts>,v1=<hex>[,v1=<hex>]" against HMAC-SHA256(secret, "<t>.<body>"). */
function verifySignature(header: string, secret: string, body: string): boolean {
  const parts = header.split(",").map((p) => p.split("=") as [string, string]);
  const t = parts.find(([k]) => k === "t")?.[1];
  if (!t) return false;
  const want = createHmac("sha256", secret).update(`${t}.${body}`).digest();
  return parts
    .filter(([k]) => k === "v1")
    .some(([, v]) => {
      const got = Buffer.from(v, "hex");
      return got.length === want.length && timingSafeEqual(got, want);
    });
}

test("a webhook endpoint from the panel receives a signed delivery", async ({ page }) => {
  const { server, port, received } = await startReceiver();
  try {
    const { base } = await openPanel(page);
    await page.goto(`${base}/config/webhooks`);
    await expect(page.getByRole("heading", { name: "Webhook", level: 1 })).toBeVisible();

    const url = `http://localhost:${port}/arenile`;
    await page.getByRole("button", { name: "Nuovo endpoint" }).click();
    const form = page.getByRole("dialog", { name: "Nuovo endpoint" });
    await form.getByLabel("Indirizzo (URL)").fill(url);
    await form.getByLabel("Descrizione").fill("Ricevitore e2e");
    await form.getByRole("combobox", { name: "Eventi" }).click();
    await page.getByRole("option", { name: /Tutti gli eventi/ }).click();
    await page.keyboard.press("Escape");
    await form.getByRole("button", { name: "Crea endpoint" }).click();

    const created = page.getByRole("dialog", { name: "Endpoint creato" });
    const secret = await created.getByLabel("Segreto di firma").inputValue();
    expect(secret.length).toBeGreaterThan(16);
    await created.getByRole("button", { name: "Ho salvato, chiudi" }).click();

    // Synchronous test event: delivered, signed with the endpoint secret.
    await page.getByRole("button", { name: `Altre azioni per ${url}` }).click();
    await page.getByRole("menuitem", { name: "Invia evento di prova" }).click();
    await expect(page.getByText(`Evento di prova consegnato a ${url}`)).toBeVisible();
    await expect(page.getByText("Risposta HTTP 204")).toBeVisible();

    expect(received.length).toBeGreaterThanOrEqual(1);
    const test = received.find((r) => r.body.includes("webhook.test"));
    expect(test, "test event received").toBeTruthy();
    const signature = String(test!.headers["arenile-signature"] ?? "");
    expect(signature).toMatch(/^t=\d+,v1=[0-9a-f]{64}/);
    expect(verifySignature(signature, secret, test!.body)).toBe(true);
    expect(verifySignature(signature, "wrong-secret", test!.body)).toBe(false);

    // The delivery log shows it as delivered.
    await page.getByRole("button", { name: `Registro consegne di ${url}` }).click();
    const sheet = page.getByRole("dialog", { name: "Registro consegne" });
    await expect(
      sheet.getByRole("table", { name: "Registro consegne" }).getByText("Consegnato").first(),
    ).toBeVisible();
  } finally {
    await new Promise<void>((resolve) => server.close(() => resolve()));
  }
});
