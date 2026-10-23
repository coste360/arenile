import { EST_ID, json, list, type Handler } from '../../configuration/testing';
import type { OperationsDay } from '../lib/status';

/** API double of the operational pages (tests only). */

export const API = `/api/admin/v1/establishments/${EST_ID}`;
const ts = { created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' };

export const mapSummary = {
  id: 'm1',
  name: { it: 'Spiaggia' },
  width: 400,
  height: 300,
  grid_size: 10,
  sea_side: 'bottom',
  sort_order: 0,
  spot_count: 4,
  version: 1,
  ...ts,
};

const spot = (id: string, label: string, x: number, y: number) => ({
  id,
  map_id: 'm1',
  zone_id: 'z1',
  label,
  row_label: 'A',
  number: Number(label.slice(1)),
  kind: 'umbrella',
  x,
  y,
  rotation: 0,
  shape: { type: 'circle', w: 30, h: 30 },
  capacity: {},
  max_extra: {},
  attributes: [],
  active: true,
});

export const mapDocument = {
  map: mapSummary,
  spots: [
    spot('s1', 'A1', 50, 50),
    spot('s2', 'A2', 100, 50),
    spot('s3', 'A3', 150, 50),
    spot('s4', 'A4', 200, 50),
  ],
  decorations: [],
};

/** Routes for maps, the day and the takings; `day` may be a function to change between calls. */
export function operationsRoutes(
  day: OperationsDay | (() => OperationsDay),
  extra: Record<string, Handler> = {},
): Record<string, Handler> {
  return {
    [`GET ${API}/maps`]: () => list([mapSummary]),
    [`GET ${API}/maps/m1`]: () => json(200, mapDocument, { ETag: '"1"' }),
    [`GET ${API}/operations/day`]: () => json(200, typeof day === 'function' ? day() : day),
    [`GET ${API}/payments`]: () => json(200, { items: [] }),
    ...extra,
  };
}
