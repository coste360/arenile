import type { AdminApi, Schemas } from "./api";
import type { RunContext } from "./context";

/**
 * Daily-sale data of the capture run (spec 14.3, ADR 0020) on the demo day:
 * walk-in tickets on spots (one with a name), zone tickets still to assign,
 * a voided ticket, and entry tickets on an equipped free beach (quantity
 * zone, its pool, a tracked component and one priced pack per type). Runs
 * after the data snapshots, so the live components keep the plain demo.
 */

type OperationsDay = Schemas["OperationsDay"];
type Pack = Schemas["Pack"];
type Zone = Schemas["Zone"];
type DailySaleResult = Schemas["DailySaleResult"];

export async function seedDailySale(api: AdminApi, E: string, ctx: RunContext): Promise<void> {
  const day = await api.get<OperationsDay>(`${E}/operations/day`, { date: ctx.date, slot: "FULL" });
  const zones = await api.all<Zone>(`${E}/zones`);
  const packs = await api.all<Pack>(`${E}/packs`);
  const base = packs.find((p) => p.code === "base") ?? packs[0];
  if (!base) throw new Error("demo pack not found");
  const standard = zones.find((z) => z.code === "standard") ?? zones[zones.length - 1]!;
  // Free spots of the back rows, from the end: the front of the map keeps the demo look.
  const free = day.spots
    .filter((s) => !s.booking && s.status === "free" && s.zone_id === standard.id)
    .sort((a, b) => b.label.localeCompare(a.label, "it", { numeric: true }));
  if (free.length < 6) throw new Error(`only ${free.length} free spots on ${ctx.date}`);

  const sale = (body: Record<string, unknown>) =>
    api.post<DailySaleResult>(`${E}/daily-sales`, {
      date: ctx.date,
      slots: "FULL",
      pack_id: base.id,
      ...body,
    });
  // #001-#003 on spots, #004 voided, #005-#006 zone tickets to assign.
  await sale({ spot_id: free[0]!.spot_id, payment: { method: "cash", tendered_amount: 5000 }, holder: { last_name: "Bianchi" } });
  await sale({ spot_id: free[1]!.spot_id, payment: { method: "card" } });
  await sale({ spot_id: free[2]!.spot_id, slots: "AM", payment: { method: "cash" } });
  const voided = await sale({ spot_id: free[3]!.spot_id, payment: { method: "card" } });
  // The void requires If-Match with the ticket version (INV-80).
  await api.post(
    `${E}/tickets/${voided.ticket.id}/void`,
    { reason: "Cliente andato via", refund: "none" },
    { "If-Match": `"${voided.ticket.version}"` },
  );
  await sale({ zone_id: standard.id, payment: { method: "card" } });
  await sale({ zone_id: standard.id, payment: { method: "cash" }, holder: { last_name: "Russo" } });

  // Entry tickets: equipped free beach with 80 entries a day.
  const beach = await api.post<Zone>(`${E}/zones`, {
    code: "spiaggia-libera",
    name: { it: "Spiaggia libera attrezzata", en: "Equipped free beach" },
    assignment_mode: "quantity",
    online_visible: false,
    price_category: "ingresso",
  });
  const pool = await api.post<{ id: string }>(`${E}/pools`, {
    zone_id: beach.id,
    component_code: "ingresso",
    name: { it: "Ingressi spiaggia libera", en: "Free beach entries" },
    capacity: 80,
  });
  await api.post(`${E}/components`, {
    code: "ingresso",
    name: { it: "Ingresso", en: "Entry" },
    tracked: true,
    pool_id: pool.id,
  });
  const types: { id: string }[] = [];
  for (const [i, [code, it, en, amount]] of (
    [
      ["adulto", "Adulto", "Adult", 800],
      ["ridotto", "Ridotto", "Reduced", 600],
      ["bambino", "Bambino", "Child", 400],
    ] as const
  ).entries()) {
    const pack = await api.post<Pack>(`${E}/packs`, {
      code: `ingresso-${code}`,
      name: { it: `Ingresso ${it.toLowerCase()}`, en: `${en} entry` },
      composition: { ingresso: 1 },
      allowed_zone_ids: [beach.id],
      allowed_formulas: ["day", "half_day"],
      channels: ["desk"],
      max_per_booking: 50,
      active: true,
    });
    await api.post(`${E}/price-rules`, {
      season_id: ctx.seasonId,
      name: `Ingresso ${it.toLowerCase()}`,
      target_type: "pack",
      target_ids: [pack.id],
      conditions: {},
      action: { type: "set", amount },
      stage: "base",
      active: true,
    });
    types.push(
      await api.post<{ id: string }>(`${E}/entry-ticket-types`, {
        code,
        name: { it, en },
        pack_id: pack.id,
        zone_id: beach.id,
        sort_order: i,
        active: true,
      }),
    );
  }
  // Two entry tickets: one per theme of the gate screenshot (each is admitted once).
  for (const quantities of [
    [2, 0, 1],
    [1, 1, 0],
  ]) {
    await api.post(`${E}/entry-sales`, {
      date: ctx.date,
      slots: "FULL",
      items: quantities
        .map((quantity, i) => ({ entry_ticket_type_id: types[i]!.id, quantity }))
        .filter((it) => it.quantity > 0),
      payment: { method: "card" },
    });
  }
}
