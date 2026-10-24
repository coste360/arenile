import { expect, test, type Page } from "@playwright/test";
import { spawn, spawnSync, type ChildProcess } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { createServer, type AddressInfo, type Server } from "node:net";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { DEMO_OWNER_EMAIL, DEMO_OWNER_PASSWORD, DEMO_SLUG } from "./helpers";

// ESC/POS network printing (ADR 0021) runs only on the desktop edition, so
// this spec starts its own `arenile serve` with ARENILE_EDITION=desktop
// (fresh SQLite, demo seed) next to a fake network printer: a TCP server on
// 127.0.0.1 that records the bytes it receives, as a real printer on port
// 9100 would. "Stampa di prova" from the Printers page must reach it.

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(here, "..", "..");
const desktopPort = Number(process.env.E2E_DESKTOP_PORT ?? 18991);
const desktopURL = `http://127.0.0.1:${desktopPort}`;
/** Optional folder for the page screenshots (light/dark, 1440/390). */
const shotsDir = process.env.PRINTERS_SHOTS_DIR;

const GS_V = Buffer.from([0x1d, 0x56]); // cut
const GS_K = Buffer.from([0x1d, 0x28, 0x6b]); // QR code (GS ( k)

let server: ChildProcess | undefined;
let printer: Server | undefined;
let printerPort = 0;
let received: Buffer[] = [];
let dataDir = "";

function startFakePrinter(): Promise<number> {
  return new Promise((ok) => {
    printer = createServer((socket) => {
      const chunks: Buffer[] = [];
      socket.on("data", (c: Buffer) => chunks.push(c));
      socket.on("end", () => {
        if (chunks.length) received.push(Buffer.concat(chunks));
      });
      socket.on("error", () => undefined);
    });
    printer.listen(0, "127.0.0.1", () =>
      ok((printer!.address() as AddressInfo).port),
    );
  });
}

async function waitHealthy(url: string, timeoutMs: number) {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    try {
      const res = await fetch(`${url}/healthz`);
      if (res.ok) return;
    } catch {
      // not up yet
    }
    if (Date.now() > deadline) throw new Error(`${url} did not start`);
    await new Promise((r) => setTimeout(r, 500));
  }
}

test.describe.configure({ mode: "serial" });

test.beforeAll(async () => {
  test.setTimeout(300_000);
  printerPort = await startFakePrinter();
  dataDir = mkdtempSync(join(tmpdir(), "arenile-e2e-desktop-"));
  const bin = join(dataDir, "arenile");
  const build = spawnSync("go", ["build", "-o", bin, "./cmd/arenile"], {
    cwd: repoRoot,
    stdio: "inherit",
  });
  if (build.status !== 0) throw new Error("go build failed");
  const env = {
    ...process.env,
    ARENILE_EDITION: "desktop",
    // The fake printer listens on loopback, refused in production.
    ARENILE_PRINTERS_ALLOW_LOOPBACK: "true",
    ARENILE_ADDR: `127.0.0.1:${desktopPort}`,
    ARENILE_BASE_URL: desktopURL,
    ARENILE_DATA_DIR: dataDir,
    ARENILE_AUTO_MIGRATE: "true",
    // Fixed test-only key (32 zero bytes, base64): never use in production.
    ARENILE_SECRET_KEY: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
    ARENILE_ADMIN_PASSWORD: DEMO_OWNER_PASSWORD,
    ARENILE_LOG_LEVEL: "warn",
    ARENILE_TELEMETRY: "false",
  };
  for (const args of [
    ["migrate", "up"],
    ["seed", "demo"],
  ]) {
    const r = spawnSync(bin, args, { env, stdio: "inherit" });
    if (r.status !== 0) throw new Error(`arenile ${args.join(" ")} failed`);
  }
  server = spawn(bin, ["serve"], { env, stdio: "inherit" });
  await waitHealthy(desktopURL, 60_000);
});

test.afterAll(async () => {
  server?.kill("SIGTERM");
  await new Promise<void>((ok) => (printer ? printer.close(() => ok()) : ok()));
  if (dataDir) rmSync(dataDir, { recursive: true, force: true });
});

test.use({ baseURL: desktopURL });

async function login(page: Page) {
  await page.goto("/admin/login");
  await page.getByLabel("Email").fill(DEMO_OWNER_EMAIL);
  await page.getByLabel("Password").fill(DEMO_OWNER_PASSWORD);
  await page.getByRole("button", { name: "Accedi" }).click();
  await expect(page).not.toHaveURL(/\/login/);
}

async function establishmentId(page: Page): Promise<string> {
  const res = await page.request.get("/api/admin/v1/establishments");
  const items = (
    (await res.json()) as { items: { id: string; slug: string }[] }
  ).items;
  const id = items.find((e) => e.slug === DEMO_SLUG)?.id;
  expect(id).toBeTruthy();
  return id as string;
}

/** JSON POST with the CSRF token of the cookie session. */
async function apiPost(
  page: Page,
  url: string,
  data: unknown,
  headers: Record<string, string> = {},
) {
  const csrf = (await page.context().cookies()).find(
    (c) => c.name === "arenile_csrf",
  )?.value;
  const res = await page.request.post(url, {
    data,
    headers: { "X-CSRF-Token": csrf ?? "", ...headers },
  });
  expect(res.ok(), `POST ${url} -> ${res.status()} ${await res.text()}`).toBe(
    true,
  );
  return res.json();
}

async function printerIdByName(
  page: Page,
  est: string,
  name: string,
): Promise<string> {
  const res = await page.request.get(
    `/api/admin/v1/establishments/${est}/printers`,
  );
  const items = ((await res.json()) as { items: { id: string; name: string }[] })
    .items;
  const id = items.find((p) => p.name === name)?.id;
  expect(id).toBeTruthy();
  return id as string;
}

/** Sells a card ticket on a free spot of the demo season (10th day). */
async function demoTicket(
  page: Page,
  est: string,
): Promise<{ id: string; label: string }> {
  const api = `/api/admin/v1/establishments/${est}`;
  const seasons = (
    (await (await page.request.get(`${api}/seasons`)).json()) as {
      items: { opens_on: string; status: string }[];
    }
  ).items;
  const season = seasons.find((x) => x.status === "active") ?? seasons[0]!;
  const d = new Date(`${season.opens_on}T12:00:00Z`);
  d.setUTCDate(d.getUTCDate() + 9);
  const date = d.toISOString().slice(0, 10);
  const day = (await (
    await page.request.get(`${api}/operations/day?date=${date}&slot=FULL`)
  ).json()) as {
    spots: { spot_id: string; status: string; booking?: unknown }[];
  };
  const free = day.spots.filter((x) => x.status === "free" && !x.booking);
  const packs = (
    (await (await page.request.get(`${api}/packs`)).json()) as {
      items: { id: string; code: string }[];
    }
  ).items;
  const pack = packs.find((p) => p.code === "base") ?? packs[0]!;
  const sold = (await apiPost(
    page,
    `${api}/daily-sales`,
    {
      date,
      slots: "FULL",
      pack_id: pack.id,
      spot_id: free[free.length - 1]!.spot_id,
      payment: { method: "card" },
    },
    { "Idempotency-Key": crypto.randomUUID() },
  )) as { ticket: { id: string; number_label: string } };
  return { id: sold.ticket.id, label: sold.ticket.number_label };
}

async function shoot(page: Page, name: string) {
  if (!shotsDir) return;
  for (const scheme of ["light", "dark"] as const) {
    await page.emulateMedia({ colorScheme: scheme });
    // Let the colour transitions settle before the capture.
    await page.waitForTimeout(600);
    await page.screenshot({
      path: join(shotsDir, `${name}-${scheme}.png`),
      fullPage: true,
    });
  }
  await page.emulateMedia({ colorScheme: "light" });
}

test("desktop edition: add a network printer and the test print reaches it", async ({
  page,
}) => {
  test.setTimeout(120_000);
  received = [];
  await page.setViewportSize({ width: 1440, height: 900 });
  await login(page);
  const id = await establishmentId(page);
  await page.goto(`/admin/e/${id}/config/printers`);
  await expect(
    page.getByRole("heading", { name: "Stampanti", exact: true }),
  ).toBeVisible();
  await expect(page.getByText("Nessuna stampante")).toBeVisible();
  await shoot(page, "printers-empty-1440");

  // A public address is refused before it reaches the server.
  await page
    .getByRole("button", { name: "Aggiungi stampante" })
    .first()
    .click();
  const sheet = page.getByRole("dialog");
  await sheet
    .getByRole("textbox", { name: "Nome", exact: true })
    .fill("Banco bar");
  await sheet.getByLabel("Indirizzo IP o nome host").fill("8.8.8.8");
  await sheet.getByRole("button", { name: "Salva" }).click();
  await expect(
    sheet.getByText(/Sono ammessi solo indirizzi della rete locale/),
  ).toBeVisible();

  // The fake printer listens on loopback (ARENILE_PRINTERS_ALLOW_LOOPBACK, test only).
  await sheet.getByLabel("Indirizzo IP o nome host").fill("127.0.0.1");
  await sheet
    .getByRole("textbox", { name: "Porta", exact: true })
    .fill(String(printerPort));
  await shoot(page, "printers-form-1440");
  await sheet.getByRole("button", { name: "Salva" }).click();
  await expect(sheet).toBeHidden();
  await expect(page.getByRole("listitem", { name: "Banco bar" })).toBeVisible();

  await page
    .getByRole("button", { name: "Verifica la connessione con Banco bar" })
    .click();
  await expect(page.getByText("Banco bar è raggiungibile")).toBeVisible();
  // The probe does not print.
  expect(Buffer.concat(received).includes(GS_V)).toBe(false);

  await page
    .getByRole("button", { name: "Stampa di prova su Banco bar" })
    .click();
  await expect(
    page.getByText("Stampa di prova inviata a Banco bar"),
  ).toBeVisible();

  // The delivery job sends the rendered ESC/POS bytes to the printer.
  await expect
    .poll(() => Buffer.concat(received).includes(GS_V), { timeout: 30_000 })
    .toBe(true);
  // The connection that printed (the status probe, DLE EOT, uses its own).
  const bytes = received.find((c) => c.includes(GS_V))!;
  expect(bytes.includes(Buffer.from([0x1b, 0x40]))).toBe(true); // ESC @
  expect(bytes.includes(Buffer.from("STAMPA DI PROVA"))).toBe(true);
  expect(bytes.includes(Buffer.from("Banco bar"))).toBe(true);
  expect(bytes.includes(GS_K)).toBe(true);
  expect(bytes.includes(Buffer.from([0xd5]))).toBe(true); // € in CP858

  // The job list shows it printed.
  await expect(
    page.getByRole("cell", { name: "Pagina di prova" }),
  ).toBeVisible();
  await expect(page.getByText("Stampato", { exact: true })).toBeVisible({
    timeout: 15_000,
  });

  // Routing: a default rule for the first document (cash closing) on this printer.
  await page.getByRole("button", { name: "Aggiungi regola" }).click();
  await page.getByRole("button", { name: "Salva instradamento" }).click();
  await expect(page.getByText("Instradamento salvato")).toBeVisible();
  await shoot(page, "printers-1440");

  // A daily-sale ticket (ADR 0020) printed on this printer: a card sale on a
  // free spot of the demo season, then a daily_ticket job.
  const printerId = await printerIdByName(page, id, "Banco bar");
  const ticket = await demoTicket(page, id);
  received = [];
  await apiPost(
    page,
    `/api/admin/v1/establishments/${id}/print-jobs`,
    { document_kind: "daily_ticket", document_id: ticket.id, printer_id: printerId },
    { "Idempotency-Key": crypto.randomUUID() },
  );
  await expect
    .poll(
      () => Buffer.concat(received).includes(Buffer.from(`BIGLIETTO ${ticket.label}`)),
      { timeout: 30_000 },
    )
    .toBe(true);
  const slip = received.find((c) => c.includes(Buffer.from("BIGLIETTO")))!;
  expect(slip.includes(Buffer.from("Non valido ai fini fiscali"))).toBe(true);
  expect(slip.includes(GS_K)).toBe(true);
  expect(slip.includes(GS_V)).toBe(true);

  await page.setViewportSize({ width: 390, height: 844 });
  await page.reload();
  await expect(page.getByRole("listitem", { name: "Banco bar" })).toBeVisible();
  const overflow = await page.evaluate(
    () => document.documentElement.scrollWidth - window.innerWidth,
  );
  expect(overflow).toBeLessThanOrEqual(0);
  await shoot(page, "printers-390");
  await page.setViewportSize({ width: 320, height: 700 });
  const overflow320 = await page.evaluate(
    () => document.documentElement.scrollWidth - window.innerWidth,
  );
  expect(overflow320).toBeLessThanOrEqual(0);
});

test.describe("server edition", () => {
  // The main e2e server (VPS/Docker profile): ESC/POS is explained, not offered.
  test.use({
    baseURL: `http://localhost:${Number(process.env.E2E_PORT ?? 8090)}`,
  });

  test("the Printers page explains the desktop app and the browser fallback", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
    await login(page);
    const id = await establishmentId(page);
    await page.goto(`/admin/e/${id}/config/printers`);
    await expect(
      page.getByText("Stampa ESC/POS disponibile con l'app desktop"),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Aggiungi stampante" }),
    ).toHaveCount(0);
    await shoot(page, "printers-server-1440");
  });
});
