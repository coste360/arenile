import { ApiError, type AdminApi, type Schemas } from "./api";
import type { RunContext } from "./context";

/**
 * Cash desk and agreements data of the capture run (spec 30, ADR 0028) on the
 * demo day: a hotel partner with its agreement (link BELVEDERE, reserved
 * quota), a residents agreement with a requirement, bookings with agreement,
 * partner, group and party size on row D, a stay ending on the demo day and a
 * partner payment. Runs after the data snapshots and the daily sale.
 */

type Pack = Schemas["Pack"];
type MapDocument = Schemas["MapDocument"];

const addDays = (iso: string, days: number) => {
  const d = new Date(`${iso}T12:00:00Z`);
  d.setUTCDate(d.getUTCDate() + days);
  return d.toISOString().slice(0, 10);
};

export const DOCS_AGREEMENT_CODE = "BELVEDERE";

export async function seedM8(api: AdminApi, E: string, ctx: RunContext): Promise<{ partnerId: string }> {
  const packs = await api.all<Pack>(`${E}/packs`);
  const base = packs.find((p) => p.code === "base") ?? packs[0];
  if (!base) throw new Error("demo pack not found");
  const doc = await api.get<MapDocument>(`${E}/maps/${ctx.mapId}`);
  const spotId = new Map(doc.spots.map((s) => [s.label, s.id]));

  const partner = await api.post<{ id: string }>(`${E}/partners`, {
    kind: "struttura_ricettiva",
    accommodation_type: "hotel",
    name: "Hotel Belvedere",
    contact_name: "Laura Neri",
    municipality: "Gaeta",
    phone: "+39 0771 000000",
  });
  const hotel = await api.post<{ id: string }>(`${E}/agreements`, {
    name: "Ospiti Hotel Belvedere",
    code: DOCS_AGREEMENT_CODE,
    target: "partner",
    partner_id: partner.id,
    action: { type: "percent_off", percent: "15" },
    channels: ["desk", "online_link", "online_code"],
    register_category: "accommodation",
    quota: { max_per_day: 10, reserved_per_day: 4, release_days_before: 3 },
  });
  const residents = await api.post<{ id: string }>(`${E}/agreements`, {
    name: "Residenti di Gaeta",
    code: "RESIDENTI",
    target: "individual_customers",
    action: { type: "percent_off", percent: "20" },
    channels: ["desk"],
    register_category: "residents",
    requirement: "Residenza nel Comune di Gaeta",
  });

  // Row D, first free labels: a refused spot (taken by the demo) is skipped.
  const labels = Array.from({ length: 15 }, (_, i) => `D${i + 1}`);
  const book = async (from: string, to: string, last: string, extra: Record<string, unknown>) => {
    while (labels.length > 0) {
      const label = labels.shift()!;
      try {
        return await api.post<{ id: string }>(`${E}/bookings`, {
          customer: { first_name: "Anna", last_name: last },
          starts_on: from,
          ends_on: to,
          slots: "FULL",
          formula: from === to ? "day" : "multi_day",
          lines: [{ pack_id: base.id, spot_id: spotId.get(label) }],
          force: true,
          ...extra,
        });
      } catch (err) {
        if (!(err instanceof ApiError) || err.status !== 409) throw err;
      }
    }
    throw new Error("no free spot of row D for the M8 bookings");
  };
  const d = ctx.date;
  const family = {
    agreement_id: hotel.id,
    partner_id: partner.id,
    group_name: "Famiglia Neri",
    partner_share: { type: "percent", percent: "50" },
  };
  await book(addDays(d, -2), addDays(d, 3), "Neri", { ...family, party_adults: 2, party_children: 2 });
  await book(d, addDays(d, 5), "Neri", { ...family, party_adults: 2 });
  await book(d, addDays(d, 4), "Esposito", { agreement_id: residents.id, requirement_verified: true });
  await book(addDays(d, -3), d, "Fontana", {});
  await book(addDays(d, -1), addDays(d, 2), "Galli", {});

  await api.post(`${E}/partners/${partner.id}/payments`, {
    amount: 5000,
    method: "bank_transfer",
    // Received today (real date): the demo day is a week ahead.
    received_on: new Intl.DateTimeFormat("en-CA", { timeZone: "Europe/Rome" }).format(new Date()),
    reference: "Bonifico 0042",
  });
  return { partnerId: partner.id };
}
