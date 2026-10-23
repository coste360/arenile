import type { Planning } from '../lib/grid';

/** Planning payload of the tests: zone z1 with rows A and B, zone z2 with one spot. */
export function planningFixture(days = 5): Planning {
  const all = Array.from({ length: days }, (_, i) => `2026-07-${String(14 + i).padStart(2, '0')}`);
  return {
    from: all[0] as string,
    to: all[all.length - 1] as string,
    days: all,
    closed_days: [4],
    spots: [
      { spot_id: 's-b1', label: 'B1', row_label: 'B', zone_id: 'z1', map_id: 'm1' },
      { spot_id: 's-a2', label: 'A2', row_label: 'A', zone_id: 'z1', map_id: 'm1' },
      { spot_id: 's-a1', label: 'A1', row_label: 'A', zone_id: 'z1', map_id: 'm1' },
      { spot_id: 's-g1', label: 'G1', zone_id: 'z2', map_id: 'm1' },
    ],
    entries: [
      {
        kind: 'booking',
        booking_id: 'b1',
        code: 'AR-0001',
        label: 'Rossi',
        status: 'confirmed',
        channel: 'desk',
        balance_due: false,
      },
      { kind: 'season', booking_id: 'p1', code: 'AB-1', label: 'Verdi', status: 'confirmed' },
      { kind: 'block', block_id: 'k1', label: 'Manutenzione' },
    ],
    rows: [
      // A1 (index 2): booking days 1-2 FULL.
      { spot: 2, segments: [{ entry: 0, from: 1, to: 2, slots: 'FULL' }] },
      // B1 (index 0): season all week AM, block day 3 PM.
      {
        spot: 0,
        segments: [
          { entry: 1, from: 0, to: 4, slots: 'AM' },
          { entry: 2, from: 3, to: 3, slots: 'PM' },
        ],
      },
    ],
  };
}

export const zoneNames = new Map([
  ['z1', 'Prima fila'],
  ['z2', 'Gazebo'],
]);
