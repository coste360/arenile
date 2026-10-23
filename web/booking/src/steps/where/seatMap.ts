import type { MapGeometry, MapSpot, SpotStatus } from '@arenile/shared/map';
import type { Schemas } from '@arenile/shared/api';
import type { PublicAvailability, PublicMapDocument } from '../../api/public';

/** Pure adapters between the public map/availability documents and the shared BeachMap. */

export interface SeatMapModel {
  geometry: MapGeometry;
  statuses: Record<string, SpotStatus>;
  /** spot id → price category (effective, from availability or the map). */
  categories: Record<string, string>;
  /** spot id → zone id. */
  zoneOf: Record<string, string>;
}

/**
 * Geometry of the seat zones of one map with the online status of each spot:
 * `free` when available online for the whole interval (quota included),
 * `unavailable` otherwise (booked, held, closed, out of quota). No other
 * status ever reaches the public page (no personal data, spec 12.3).
 */
export function seatMapModel(
  doc: PublicMapDocument,
  availability: PublicAvailability | undefined,
  zoneIds: readonly string[],
  localize: (text: Record<string, string> | undefined) => string,
): SeatMapModel {
  const allowed = new Set(zoneIds);
  const free = new Map<string, string>();
  for (const z of availability?.zones ?? []) {
    if (!allowed.has(z.zone_id)) continue;
    for (const s of z.spots ?? []) if (s.available) free.set(s.spot_id, s.price_category);
  }
  const statuses: Record<string, SpotStatus> = {};
  const categories: Record<string, string> = {};
  const zoneOf: Record<string, string> = {};
  const spots: MapSpot[] = [];
  for (const s of doc.spots) {
    if (!allowed.has(s.zone_id)) continue;
    spots.push({
      id: s.id,
      label: s.label,
      row_label: s.row_label || null,
      kind: s.kind,
      asset: s.asset ?? null,
      x: s.x,
      y: s.y,
      rotation: s.rotation,
      shape: s.shape,
      zone_id: s.zone_id,
    });
    statuses[s.id] = free.has(s.id) ? 'free' : 'unavailable';
    categories[s.id] = free.get(s.id) ?? s.price_category;
    zoneOf[s.id] = s.zone_id;
  }
  return {
    geometry: {
      id: doc.map.id,
      name: localize(doc.map.name),
      width: doc.map.width,
      height: doc.map.height,
      grid_size: doc.map.grid_size,
      sea_side: doc.map.sea_side,
      spots,
      decorations: doc.decorations.map((d) => ({
        id: d.id,
        kind: d.kind,
        asset: d.asset ?? null,
        variant: d.variant ?? null,
        x: d.x,
        y: d.y,
        rotation: d.rotation,
        shape: d.shape,
        label: d.label ? localize(d.label) || null : null,
      })),
    },
    statuses,
    categories,
    zoneOf,
  };
}

export interface RowPrice {
  row: string | null;
  /** Last row of a run of consecutive rows with the same price ("File C–H"). */
  toRow?: string;
  fromPrice: number;
  /** Category label when the whole row has one category. */
  category?: string;
  tone: number;
}

/**
 * Price legend per row (spec 12.3 passo 2): the cheapest "da" price of the
 * categories present in each row, rows in natural order; consecutive rows
 * with the same price and tone are merged into one entry (row … toRow), so
 * a classic beach reads "Fila A · Fila B · File C–H" instead of eight chips.
 */
export function rowPrices(
  model: SeatMapModel,
  legend: readonly Schemas['PriceLegendEntry'][],
): RowPrice[] {
  const byCategory = new Map(legend.map((e) => [e.price_category, e]));
  const rows = new Map<string | null, Set<string>>();
  for (const spot of model.geometry.spots) {
    const key = spot.row_label ?? null;
    const set = rows.get(key) ?? new Set<string>();
    const category = model.categories[spot.id];
    if (category) set.add(category);
    rows.set(key, set);
  }
  const collator = new Intl.Collator(undefined, { numeric: true });
  const out: RowPrice[] = [];
  for (const [row, cats] of rows) {
    const entries = [...cats].map((c) => byCategory.get(c)).filter((e) => !!e);
    if (entries.length === 0) continue;
    const cheapest = entries.reduce((a, b) => (b.from_price < a.from_price ? b : a));
    out.push({
      row,
      fromPrice: cheapest.from_price,
      category: entries.length === 1 ? cheapest.price_category : undefined,
      tone: cheapest.tone,
    });
  }
  // Short labels first (A..H, 1..20), then special rows such as "GZ" for
  // gazebos, so runs of ordinary rows stay together.
  out.sort((a, b) =>
    a.row === null
      ? 1
      : b.row === null
        ? -1
        : a.row.length - b.row.length || collator.compare(a.row, b.row),
  );
  const merged: RowPrice[] = [];
  for (const r of out) {
    const last = merged.at(-1);
    if (
      last &&
      last.row !== null &&
      r.row !== null &&
      last.fromPrice === r.fromPrice &&
      last.tone === r.tone
    ) {
      last.toRow = r.row;
      if (last.category !== r.category) last.category = undefined;
      continue;
    }
    merged.push({ ...r });
  }
  return merged;
}
