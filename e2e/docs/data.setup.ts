import { test as setup } from "@playwright/test";
import { mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";
// The panel's own mapping from server statuses to map colours (pure, types-only imports):
// the docs show exactly what the operational map shows.
import { statusesBySpot } from "../../web/admin/src/features/operations/lib/status";
import { AdminApi, ApiError, type Schemas } from "./support/api";
import { DATA_DIR, DEMO_SLUG, runFile } from "./support/env";
import type { RunContext } from "./support/context";

/**
 * Prepares the capture run on the freshly seeded demo instance:
 *  - signs in as the demo owner and stores the session for the captures;
 *  - resolves the ids the manifest routes need ({est}, {mapId}, ...);
 *  - writes the data snapshots of the live components
 *    (docs-site/src/data/demo-map.json, demo-pricing.json).
 */

type Establishment = Schemas["Establishment"];
type Season = Schemas["Season"];
type MapDocument = Schemas["MapDocument"];
type OperationsDay = Schemas["OperationsDay"];
type Pack = Schemas["Pack"];
type Zone = Schemas["Zone"];
type RatePeriod = Schemas["RatePeriod"];
type Booking = Schemas["BookingSummary"];
type Customer = Schemas["Customer"];
type SimulateResult = Schemas["SimulateResult"];
type I18nText = Record<string, string>;

const isoAddDays = (iso: string, days: number) => {
  const d = new Date(`${iso}T12:00:00Z`);
  d.setUTCDate(d.getUTCDate() + days);
  return d.toISOString().slice(0, 10);
};
const minIso = (a: string, b: string) => (a < b ? a : b);
const text = (t: I18nText | string | null | undefined): I18nText =>
  typeof t === "string" ? { it: t } : { ...(t ?? {}) };

setup("demo context and data snapshots", async ({ playwright, baseURL }) => {
  setup.setTimeout(180_000);
  const request = await playwright.request.newContext({ baseURL });
  const api = await AdminApi.login(request);
  await request.storageState({ path: runFile("owner.json") });

  const establishments = await api.all<Establishment>("/establishments");
  const est = establishments.find((e) => e.slug === DEMO_SLUG);
  if (!est) throw new Error(`demo establishment "${DEMO_SLUG}" not found: did seed demo run?`);
  const E = `/establishments/${est.id}`;

  // The demo season is the one `seed demo` filled with bookings: the first
  // that has not closed yet (by the real date of the seed), else the last.
  const seasons = (await api.all<Season>(`${E}/seasons`)).sort((a, b) => a.opens_on.localeCompare(b.opens_on));
  const today = new Date().toISOString().slice(0, 10);
  const season = seasons.find((s) => s.closes_on >= today) ?? seasons[seasons.length - 1];
  if (!season) throw new Error("demo season not found");
  const date = process.env.DOCS_DATE ?? minIso(isoAddDays(season.opens_on, 9), season.closes_on);

  const maps = await api.all<{ id: string; sort_order?: number }>(`${E}/maps`);
  const mapId = maps[0]?.id;
  if (!mapId) throw new Error("demo map not found");

  const bookings = await api.all<Booking>(`${E}/bookings`);
  const booking =
    bookings.find((b) => b.status === "confirmed" && b.starts_on <= date && b.ends_on >= date) ??
    bookings.find((b) => b.status === "confirmed") ??
    bookings[0];
  const customers = await api.all<Customer>(`${E}/customers`);
  const customer =
    customers.find((c) => c.id === booking?.customer_id) ??
    customers.find((c) => !String(c.last_name ?? "").startsWith("(")) ??
    customers[0];

  // Magic link of the documented booking (manage area screenshots); the
  // endpoint answers 501 until the M4 notify builder implements it.
  let manageUrl = "";
  if (booking) {
    try {
      const link = await api.post<{ manage_url?: string }>(`${E}/bookings/${booking.id}/magic-link`, {
        send_email: false,
      });
      if (link.manage_url) {
        const u = new URL(link.manage_url, baseURL);
        manageUrl = u.pathname + u.search;
      }
    } catch (err) {
      if (!(err instanceof ApiError) || err.status < 400) throw err;
    }
  }

  const ctx: RunContext = {
    est: est.id,
    slug: est.slug,
    establishmentName: est.name,
    date,
    seasonId: season.id,
    mapId,
    bookingId: booking?.id ?? "",
    customerId: customer?.id ?? "",
    manageUrl,
  };
  writeFileSync(runFile("context.json"), JSON.stringify(ctx, null, 2));

  mkdirSync(DATA_DIR, { recursive: true });
  await writeMapSnapshot(api, E, ctx, est);
  await writePricingSnapshot(api, E, ctx, est, season);
  await request.dispose();
});

async function writeMapSnapshot(api: AdminApi, E: string, ctx: RunContext, est: Establishment) {
  const doc = await api.get<MapDocument>(`${E}/maps/${ctx.mapId}`);
  const day = await api.get<OperationsDay>(`${E}/operations/day`, {
    date: ctx.date,
    slot: "FULL",
    map_id: ctx.mapId,
  });

  // Stable keys instead of the random UUIDs of the seed.
  const spotKey = new Map(doc.spots.map((s) => [s.id, `s-${s.label}`]));
  const statusesById = statusesBySpot(day.spots, "FULL");
  const statuses: Record<string, string> = {};
  for (const [id, status] of Object.entries(statusesById)) {
    const key = spotKey.get(id);
    if (key) statuses[key] = status;
  }
  const decorationLabels: Record<string, I18nText> = {};
  const snapshot = {
    $comment:
      "GENERATED by e2e/docs/data.setup.ts (make docs-screenshots) from `arenile seed demo`: do not edit.",
    establishment: { name: est.name, slug: est.slug },
    date: ctx.date,
    slot: "FULL",
    name: text(doc.map.name as I18nText),
    geometry: {
      id: "demo",
      width: doc.map.width,
      height: doc.map.height,
      grid_size: doc.map.grid_size,
      sea_side: doc.map.sea_side,
      spots: [...doc.spots]
        .sort((a, b) => a.label.localeCompare(b.label, "it", { numeric: true }))
        .map((s) => ({
          id: spotKey.get(s.id)!,
          label: s.label,
          row_label: s.row_label || null,
          number: s.number ?? null,
          kind: s.kind,
          asset: s.asset ?? null,
          x: s.x,
          y: s.y,
          rotation: s.rotation ?? null,
          shape: s.shape,
          active: s.active,
        })),
      decorations: doc.decorations.map((d, i) => {
        const id = `d-${i + 1}`;
        if (d.label) decorationLabels[id] = text(d.label as I18nText);
        return {
          id,
          kind: d.kind,
          asset: d.asset ?? null,
          variant: d.variant ?? null,
          x: d.x,
          y: d.y,
          rotation: d.rotation ?? null,
          shape: d.shape,
        };
      }),
    },
    decorationLabels,
    statuses,
  };
  writeFileSync(join(DATA_DIR, "demo-map.json"), JSON.stringify(snapshot, null, 1) + "\n");
}

const DURATIONS = [
  { code: "half_day", days: 1, formula: "half_day", slots: "AM" },
  { code: "day", days: 1, formula: "day", slots: "FULL" },
  { code: "three_days", days: 3, formula: "multi_day", slots: "FULL" },
  { code: "week", days: 7, formula: "week", slots: "FULL" },
] as const;

async function writePricingSnapshot(
  api: AdminApi,
  E: string,
  ctx: RunContext,
  est: Establishment,
  season: Season,
) {
  const packs = (await api.all<Pack>(`${E}/packs`))
    .filter((p) => p.active !== false)
    .sort((a, b) => a.code.localeCompare(b.code))
    .slice(0, 4);
  const zones = (await api.all<Zone>(`${E}/zones`)).sort((a, b) => (a.sort_order ?? 0) - (b.sort_order ?? 0));
  // One representative zone per price category (the engine prices by category).
  const byCategory = new Map<string, Zone>();
  for (const z of zones) {
    const cat = z.price_category ?? z.code;
    if (!byCategory.has(cat)) byCategory.set(cat, z);
  }
  const categories = [...byCategory.entries()].slice(0, 3);
  const periods = (await api.all<RatePeriod>(`${E}/rate-periods`, { season_id: season.id }))
    .sort((a, b) => (a.sort_order ?? 0) - (b.sort_order ?? 0))
    .slice(0, 4);

  // Rule ids are random per seed: the docs show the rule names instead.
  const rules = await api.all<Schemas["PriceRule"]>(`${E}/price-rules`, { season_id: season.id });
  const ruleName = new Map(rules.map((r) => [r.id, text(r.name as unknown as I18nText | string)]));

  const scenarios: unknown[] = [];
  let engineVersion = 0;
  let currency = "EUR";
  for (const pack of packs) {
    for (const [category, zone] of categories) {
      if (pack.allowed_zone_ids?.length && !pack.allowed_zone_ids.includes(zone.id)) continue;
      for (const period of periods) {
        const range = period.ranges[0];
        if (!range) continue;
        // A weekday a few days into the period, inside the season.
        const start = minIso(isoAddDays(range.from, 2), range.to);
        for (const d of DURATIONS) {
          const end = isoAddDays(start, d.days - 1);
          if (end > season.closes_on) continue;
          const req = {
            season_id: season.id,
            channel: "desk",
            starts_on: start,
            ends_on: end,
            slots: d.slots,
            formula: d.formula,
            today: isoAddDays(season.opens_on, -30),
            lines: [{ pack_id: pack.id, zone_id: zone.id }],
          };
          let res: SimulateResult;
          try {
            res = await api.post<SimulateResult>(`${E}/pricing/simulate`, req);
          } catch (err) {
            // Not sellable (no price for that formula/period): the docs skip it.
            if (err instanceof ApiError && (err.status === 422 || err.status === 400)) continue;
            throw err;
          }
          const b = res.breakdown;
          engineVersion = b.engine_version;
          currency = b.currency;
          scenarios.push({
            pack: pack.code,
            category,
            period: period.code,
            duration: d.code,
            starts_on: start,
            ends_on: end,
            formula: d.formula,
            slots: d.slots,
            days: b.lines.flatMap((l) =>
              l.days.map((day) => ({
                date: day.date,
                slots: day.slots,
                base: day.base,
                applied: day.applied.map((a) => ({
                  rule: ruleName.get(a.rule) ?? { it: a.rule },
                  stage: a.stage,
                  effect: a.effect,
                })),
                total: day.total,
              })),
            ),
            extras_total:
              (b.extras ?? []).reduce((s, x) => s + x.total, 0) +
              b.lines.reduce((s, l) => s + l.extras.reduce((t, x) => t + x.total, 0), 0),
            discounts: b.discounts.map((x) => ({ source: x.source, amount: x.amount })),
            rounding: b.rounding,
            total: b.total,
            vat: b.vat.map((v) => ({ rate: String(v.rate), taxable: v.taxable, tax: v.tax })),
            deposit_due: res.deposit_due,
          });
        }
      }
    }
  }

  const usedDurations = new Set(scenarios.map((s) => (s as { duration: string }).duration));
  const snapshot = {
    $comment:
      "GENERATED by e2e/docs/data.setup.ts (make docs-screenshots): real pricing engine output on `arenile seed demo`, do not edit.",
    establishment: { name: est.name, slug: est.slug },
    season: { name: String(season.year), opens_on: season.opens_on, closes_on: season.closes_on },
    currency,
    channel: "desk",
    engine_version: engineVersion,
    packs: packs.map((p) => ({ code: p.code, name: text(p.name as I18nText) })),
    categories: categories.map(([code, z]) => ({ code, zone: text(z.name as unknown as I18nText) })),
    periods: periods.map((p) => ({ code: p.code, name: text(p.name as unknown as I18nText), color: p.color ?? null })),
    durations: DURATIONS.filter((d) => usedDurations.has(d.code)),
    scenarios,
  };
  writeFileSync(join(DATA_DIR, "demo-pricing.json"), JSON.stringify(snapshot) + "\n");
}
