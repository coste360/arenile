import { describe, expect, it } from 'vitest';
import type { MapSpot } from '@arenile/shared/map';
import { rowPrices, type SeatMapModel } from './seatMap';

function model(rows: Record<string, string>): SeatMapModel {
  const spots = Object.keys(rows).map(
    (row, i) =>
      ({
        id: `s${i}`,
        label: `${row}1`,
        row_label: row,
        x: i,
        y: 0,
        shape: 'umbrella',
      }) as unknown as MapSpot,
  );
  return {
    geometry: {
      spots,
      decorations: [],
      width: 100,
      height: 100,
    } as unknown as SeatMapModel['geometry'],
    statuses: {},
    categories: Object.fromEntries(spots.map((s, i) => [s.id, Object.values(rows)[i]!])),
    zoneOf: {},
  };
}

describe('rowPrices', () => {
  it('merges consecutive rows with the same price into one entry', () => {
    const m = model({ A: 'row1', B: 'row2', C: 'std', D: 'std', GZ: 'gazebo', H: 'std' });
    const legend = rowPrices(m, [
      { price_category: 'row1', from_price: 3000, tone: 3 },
      { price_category: 'row2', from_price: 2700, tone: 2 },
      { price_category: 'std', from_price: 2200, tone: 1 },
      { price_category: 'gazebo', from_price: 7500, tone: 4 },
    ]);
    expect(legend.map((r) => [r.row, r.toRow, r.fromPrice])).toEqual([
      ['A', undefined, 3000],
      ['B', undefined, 2700],
      ['C', 'H', 2200],
      ['GZ', undefined, 7500],
    ]);
    expect(legend[2]!.category).toBe('std');
  });
});
