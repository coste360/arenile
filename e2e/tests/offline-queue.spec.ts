import { expect, test, type APIRequestContext, type Page } from "@playwright/test";
import { login } from "./helpers";

/**
 * M5A offline queue (spec 21.2) against the real binary: a check-in made
 * while the browser is offline is queued with its Idempotency-Key and sent
 * exactly once on reconnect. The demo season is in the future, so the test
 * builds its own establishment with a season open today and one booking.
 */

const TZ = "Europe/Rome";

/** YYYY-MM-DD of today (+ days) in the establishment timezone. */
function romeDate(plusDays = 0): string {
  const d = new Date(Date.now() + plusDays * 86_400_000);
  return new Intl.DateTimeFormat("en-CA", { timeZone: TZ }).format(d);
}

/** JSON request with the CSRF token of the cookie session. */
async function send(
  page: Page,
  method: "POST" | "PUT",
  url: string,
  data: unknown,
  headers: Record<string, string> = {},
) {
  const csrf = (await page.context().cookies()).find((c) => c.name === "arenile_csrf")?.value;
  const res = await page.request.fetch(url, {
    method,
    data,
    headers: { "X-CSRF-Token": csrf ?? "", ...headers },
  });
  expect(res.ok(), `${method} ${url} -> ${res.status()} ${await res.text()}`).toBeTruthy();
  return res.json();
}

async function getJSON(request: APIRequestContext, url: string) {
  const res = await request.get(url);
  expect(res.ok(), `GET ${url} -> ${res.status()}`).toBeTruthy();
  return res.json();
}

interface Seeded {
  establishmentId: string;
  bookingId: string;
  code: string;
  version: number;
  customer: string;
}

/** A new lido with a season open today, a quick price list and a booking for today. */
async function seedTodayBooking(page: Page): Promise<Seeded> {
  const suffix = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`;
  const est = await send(page, "POST", "/api/admin/v1/establishments", {
    name: `Lido Offline ${suffix}`,
    slug: `lido-offline-${suffix}`,
  });
  const E = `/api/admin/v1/establishments/${est.id}`;
  const opens = romeDate(-1);
  const closes = romeDate(30);
  const applied = await send(page, "POST", `${E}/setup/apply-template`, {
    template: "classic_rows",
    opens_on: opens,
    closes_on: closes,
  });
  const season = await getJSON(page.request, `${E}/seasons/${applied.season_id}`);
  await send(page, "PUT", `${E}/seasons/${season.id}`, {
    year: season.year,
    opens_on: season.opens_on,
    closes_on: season.closes_on,
    status: "active",
  });
  // A season around today (autumn or spring) only has the low-season period.
  const prices = ["row1", "row2", "standard", "gazebo"].map((cat) => ({
    price_category: cat,
    rate_period_code: "low",
    amount: 2000,
  }));
  await send(page, "POST", `${E}/setup/quick-pricelist`, { season_id: season.id, prices });

  const map = await getJSON(page.request, `${E}/maps/${applied.map_id}`);
  const spots = (map.spots ?? map.document?.spots ?? []) as { id: string; label: string }[];
  const spot = spots.find((s) => s.label === "B3") ?? spots[0];
  expect(spot, "a spot on the template map").toBeTruthy();
  const packs = (await getJSON(page.request, `${E}/packs`)) as {
    items: { id: string; code: string }[];
  };
  const pack = packs.items.find((p) => p.code === "base") ?? packs.items[0];

  const today = romeDate();
  const lastName = `Offline${suffix}`;
  const created = await send(page, "POST", `${E}/bookings`, {
    customer: { first_name: "Mario", last_name: lastName },
    starts_on: today,
    ends_on: today,
    slots: "FULL",
    formula: "day",
    lines: [{ pack_id: pack!.id, spot_id: spot!.id }],
    force: true,
  });
  const booking = created.booking ?? created;
  return {
    establishmentId: est.id,
    bookingId: booking.id,
    code: booking.code,
    version: booking.version,
    customer: `${lastName} M.`,
  };
}

test("an offline check-in is queued and replayed once on reconnect", async ({
  page,
  context,
}) => {
  test.setTimeout(120_000);
  await login(page);
  const seeded = await seedTodayBooking(page);
  const base = `/admin/e/${seeded.establishmentId}`;

  const checkIns: string[] = [];
  page.on("request", (r) => {
    if (r.method() === "POST" && r.url().endsWith(`/bookings/${seeded.bookingId}/check-in`)) {
      checkIns.push(r.headers()["idempotency-key"] ?? "");
    }
  });

  await page.goto(`${base}/oggi`);
  await expect(page.getByRole("heading", { name: "Oggi", level: 1 })).toBeVisible();
  const checkIn = page.getByRole("button", { name: `Registra il check-in di ${seeded.customer}` });
  await expect(checkIn).toBeVisible();

  // Offline: banner, then the check-in goes to the queue (nothing is sent).
  await context.setOffline(true);
  await expect(page.getByText("Sei offline")).toBeVisible();
  await checkIn.click();
  await expect(
    page.getByText(`Check-in di ${seeded.code} in coda: sarà inviato alla riconnessione.`),
  ).toBeVisible();
  await expect(page.getByText("1 in coda").first()).toBeVisible();
  expect(checkIns).toHaveLength(0);

  // Back online: sent once, with an Idempotency-Key.
  await context.setOffline(false);
  await expect(page.getByText("1 operazione offline inviata.")).toBeVisible({ timeout: 45_000 });
  await expect(page.getByText("Sei offline")).toBeHidden();
  expect(checkIns).toHaveLength(1);
  expect(checkIns[0]).not.toBe("");

  const detail = await getJSON(
    page.request,
    `/api/admin/v1/establishments/${seeded.establishmentId}/bookings/${seeded.bookingId}`,
  );
  const booking = detail.booking ?? detail;
  expect(booking.status).toBe("checked_in");
  // One state change only: the version moved by exactly one.
  expect(booking.version).toBe(seeded.version + 1);
});
