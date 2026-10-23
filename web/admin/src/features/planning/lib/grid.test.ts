import { describe, expect, it } from 'vitest';
import { planningFixture, zoneNames } from '../testing/fixtures';
import { entryStatus } from './colors';
import {
  fitCellWidth,
  DEFAULT_METRICS as m,
  barAt,
  barRect,
  bodySize,
  buildLayout,
  cellAt,
  isMovable,
  proposeMove,
  selectionIsFree,
  selectionToDefaults,
  shiftSpotRow,
  visibleRows,
} from './grid';

const layout = buildLayout(planningFixture(), zoneNames, ['z1', 'z2']);
const labels = layout.rows.map((r) => (r.kind === 'spot' ? r.spot.label : `${r.kind}:${r.label}`));

describe('buildLayout', () => {
  it('groups rows by zone order, then row label, spots in natural order', () => {
    expect(labels).toEqual([
      'zone:Prima fila',
      'row:A',
      'A1',
      'A2',
      'row:B',
      'B1',
      'zone:Gazebo',
      'G1',
    ]);
    expect(layout.rowOfSpot.get('s-a1')).toBe(2);
    expect(layout.rowOfSpot.get('s-g1')).toBe(7);
  });

  it('places segments on the spot rows', () => {
    expect(layout.bars.map((b) => [labels[b.row], b.from, b.to, b.slots, b.entry.kind])).toEqual([
      ['A1', 1, 2, 'FULL', 'booking'],
      ['B1', 0, 4, 'AM', 'season'],
      ['B1', 3, 3, 'PM', 'block'],
    ]);
    expect(layout.closed.has(4)).toBe(true);
  });

  it('puts zones missing from the order last, by name', () => {
    const l = buildLayout(planningFixture(), zoneNames, []);
    expect(l.rows.filter((r) => r.kind === 'zone').map((r) => r.label)).toEqual([
      'Gazebo',
      'Prima fila',
    ]);
  });

  it('omits row sub-headers when a zone has a single row label', () => {
    const p = planningFixture();
    p.spots = p.spots.filter((s) => s.row_label !== 'B');
    p.rows = [];
    expect(buildLayout(p, zoneNames, ['z1', 'z2']).rows.map((r) => r.kind)).toEqual([
      'zone',
      'spot',
      'spot',
      'zone',
      'spot',
    ]);
  });
});

describe('geometry', () => {
  it('computes bar rectangles: FULL whole row, AM top half, PM bottom half', () => {
    const [full, am, pm] = layout.bars;
    expect(barRect(full!, m)).toEqual({ x: 41, y: 2 * 32 + 3, width: 78, height: 26 });
    expect(barRect(am!, m)).toEqual({ x: 1, y: 5 * 32 + 3, width: 198, height: 13 });
    expect(barRect(pm!, m)).toEqual({ x: 121, y: 5 * 32 + 3 + 13, width: 38, height: 13 });
  });

  it('sizes the body from days and rows', () => {
    expect(bodySize(layout, m)).toEqual({ width: 5 * 40, height: 8 * 32 });
  });

  it('windows rows with overscan and clamps to the grid', () => {
    expect(visibleRows(0, 320, 32, 1000, 2)).toEqual({ start: 0, end: 12 });
    expect(visibleRows(3200, 320, 32, 1000, 2)).toEqual({ start: 98, end: 112 });
    expect(visibleRows(31_900, 320, 32, 1000, 2)).toEqual({ start: 994, end: 999 });
    expect(visibleRows(0, 320, 32, 0)).toEqual({ start: 0, end: -1 });
  });

  it('hit-tests cells and bars', () => {
    expect(cellAt(45, 70, m, layout)).toEqual({ row: 2, day: 1 });
    expect(cellAt(-1, 70, m, layout)).toBeNull();
    expect(cellAt(10_000, 70, m, layout)).toBeNull();
    expect(barAt(layout, 2, 1)?.entry.code).toBe('AR-0001');
    expect(barAt(layout, 2, 0)).toBeUndefined();
    expect(barAt(layout, 5, 3, 'PM')?.entry.kind).toBe('block');
    expect(barAt(layout, 5, 3, 'AM')?.entry.kind).toBe('season');
  });
});

describe('moves', () => {
  const booking = layout.bars[0]!;

  it('skips group headers when shifting rows', () => {
    expect(shiftSpotRow(layout.rows, 2, 1)).toBe(3);
    expect(shiftSpotRow(layout.rows, 3, 1)).toBe(5);
    expect(shiftSpotRow(layout.rows, 5, 1)).toBe(7);
    expect(shiftSpotRow(layout.rows, 2, -1)).toBe(2);
    expect(shiftSpotRow(layout.rows, 7, 5)).toBe(7);
  });

  it('snaps a pixel drag to days and spot rows', () => {
    expect(proposeMove(layout, booking, 42, 0, m)).toMatchObject({
      dayDelta: 1,
      targetRow: 2,
      sourceSpotId: 's-a1',
      targetSpotId: 's-a1',
    });
    expect(proposeMove(layout, booking, -18, 33, m)).toMatchObject({
      dayDelta: 0,
      targetRow: 3,
      targetSpotId: 's-a2',
    });
    expect(proposeMove(layout, booking, 79, 70, m)).toMatchObject({
      dayDelta: 2,
      targetRow: 5,
      targetSpotId: 's-b1',
    });
  });

  it('returns null for a drag that changes nothing', () => {
    expect(proposeMove(layout, booking, 10, 10, m)).toBeNull();
  });

  it('only bookings in a live state are movable', () => {
    expect(isMovable(layout.bars[0]!.entry)).toBe(true);
    expect(isMovable(layout.bars[1]!.entry)).toBe(false);
    expect(isMovable(layout.bars[2]!.entry)).toBe(false);
    expect(isMovable({ kind: 'booking', booking_id: 'x', label: '', status: 'cancelled' })).toBe(
      false,
    );
  });
});

describe('selection', () => {
  it('converts a cell rectangle into spots and dates, skipping headers', () => {
    expect(selectionToDefaults(layout, { rowA: 3, rowB: 1, dayA: 2, dayB: 0 })).toEqual({
      spotIds: ['s-a1', 's-a2'],
      startsOn: '2026-07-14',
      endsOn: '2026-07-16',
    });
    expect(selectionToDefaults(layout, { rowA: 0, rowB: 1, dayA: 0, dayB: 0 })).toBeNull();
  });

  it('is free only when no bar overlaps', () => {
    expect(selectionIsFree(layout, { rowA: 3, rowB: 3, dayA: 0, dayB: 4 })).toBe(true);
    expect(selectionIsFree(layout, { rowA: 2, rowB: 3, dayA: 0, dayB: 1 })).toBe(false);
  });
});

describe('entryStatus', () => {
  it.each([
    [{ kind: 'booking', label: '', status: 'confirmed', channel: 'desk' }, 'booked_desk'],
    [{ kind: 'booking', label: '', status: 'confirmed', channel: 'online' }, 'booked_online'],
    [{ kind: 'booking', label: '', status: 'confirmed', balance_due: true }, 'to_collect'],
    [{ kind: 'booking', label: '', status: 'checked_in' }, 'present'],
    [{ kind: 'booking', label: '', status: 'pending_payment' }, 'hold'],
    [{ kind: 'hold', label: '' }, 'hold'],
    [{ kind: 'season', label: '' }, 'season'],
    [{ kind: 'release', label: '' }, 'season_released'],
    [{ kind: 'block', label: '' }, 'blocked'],
  ] as const)('%o → %s', (entry, expected) => {
    expect(entryStatus(entry)).toBe(expected);
  });
});

describe('fitCellWidth', () => {
  it.each([
    { name: 'unknown width keeps the minimum', w: 0, days: 14, want: 40 },
    { name: 'stretches to fill the container', w: 1112, days: 14, want: 69 },
    { name: 'never below the minimum (scrolls)', w: 500, days: 31, want: 40 },
    { name: 'never above the maximum', w: 4000, days: 14, want: 96 },
  ])('$name', ({ w, days, want }) => {
    expect(fitCellWidth(w, days, 132, 40)).toBe(want);
  });
});
