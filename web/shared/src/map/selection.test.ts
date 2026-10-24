import { describe, expect, it } from 'vitest';
import { sampleGeometry } from './sample';
import {
  EMPTY_SELECTION,
  pruneSelection,
  rangeIds,
  selectLasso,
  selectSpot,
  spotsInRect,
  toggleId,
} from './selection';

const order = ['A1', 'A2', 'A3', 'A4', 'A5'];
const notA3 = (id: string) => id !== 'A3';

describe('toggleId', () => {
  it('adds a missing id and removes a present one', () => {
    expect(toggleId(['A1'], 'A2')).toEqual(['A1', 'A2']);
    expect(toggleId(['A1', 'A2'], 'A1')).toEqual(['A2']);
  });
});

describe('rangeIds', () => {
  it.each([
    { name: 'forward', anchor: 'A1', id: 'A3', want: ['A1', 'A2', 'A3'] },
    { name: 'backward', anchor: 'A4', id: 'A2', want: ['A2', 'A3', 'A4'] },
    { name: 'same spot', anchor: 'A2', id: 'A2', want: ['A2'] },
    { name: 'no anchor', anchor: null, id: 'A2', want: ['A2'] },
    { name: 'unknown anchor', anchor: 'Z9', id: 'A2', want: ['A2'] },
    { name: 'unknown id', anchor: 'A1', id: 'Z9', want: [] },
  ])('$name', ({ anchor, id, want }) => {
    expect(rangeIds(order, anchor, id)).toEqual(want);
  });

  it('skips spots that cannot be selected', () => {
    expect(rangeIds(order, 'A1', 'A5', notA3)).toEqual(['A1', 'A2', 'A4', 'A5']);
  });
});

describe('selectSpot', () => {
  it('toggles on plain and Ctrl/Cmd activation and moves the anchor', () => {
    let s = selectSpot(EMPTY_SELECTION, 'A2', undefined, order);
    expect(s).toEqual({ ids: ['A2'], anchor: 'A2' });
    s = selectSpot(s, 'A4', { toggleKey: true }, order);
    expect(s).toEqual({ ids: ['A2', 'A4'], anchor: 'A4' });
    s = selectSpot(s, 'A2', undefined, order);
    expect(s).toEqual({ ids: ['A4'], anchor: 'A2' });
  });

  it('adds the range from the anchor with Shift, keeping what was selected', () => {
    const s = selectSpot(
      { ids: ['A5', 'A1'], anchor: 'A1' },
      'A4',
      { shiftKey: true },
      order,
      notA3,
    );
    expect(s).toEqual({ ids: ['A5', 'A1', 'A2', 'A4'], anchor: 'A4' });
  });

  it('Shift without an anchor selects just the spot', () => {
    expect(selectSpot(EMPTY_SELECTION, 'A3', { shiftKey: true }, order)).toEqual({
      ids: ['A3'],
      anchor: 'A3',
    });
  });

  it('ignores a spot that cannot be selected', () => {
    const current = { ids: ['A1'], anchor: 'A1' };
    expect(selectSpot(current, 'A3', undefined, order, notA3)).toBe(current);
  });
});

describe('spotsInRect and selectLasso', () => {
  const geometry = sampleGeometry(2, 4);
  const a = geometry.spots.find((s) => s.id === 'A1')!;
  const b = geometry.spots.find((s) => s.id === 'B2')!;

  it('selects active spots whose centre lies inside, corners in any order', () => {
    const rect = { x0: b.x + 1, y0: b.y - 1, x1: a.x - 1, y1: a.y + 1 };
    expect(spotsInRect(geometry.spots, rect)).toEqual(['A1', 'A2', 'B1', 'B2']);
    expect(spotsInRect(geometry.spots, rect, (id) => id !== 'B1')).toEqual(['A1', 'A2', 'B2']);
  });

  it('skips inactive spots', () => {
    // sampleGeometry makes the last spot of the last row inactive.
    const all = { x0: -1000, y0: -1000, x1: 1000, y1: 1000 };
    expect(spotsInRect(geometry.spots, all)).not.toContain('B4');
  });

  it('replaces the selection, or adds to it with Shift', () => {
    const current = { ids: ['A4'], anchor: 'A4' };
    expect(selectLasso(current, ['A1', 'A2'], {})).toEqual({ ids: ['A1', 'A2'], anchor: 'A2' });
    expect(selectLasso(current, ['A1', 'A4'], { shiftKey: true })).toEqual({
      ids: ['A4', 'A1'],
      anchor: 'A4',
    });
    expect(selectLasso(current, [], {})).toEqual({ ids: [], anchor: 'A4' });
  });
});

describe('pruneSelection', () => {
  it('drops ids no longer selectable and the anchor with them', () => {
    expect(pruneSelection({ ids: ['A1', 'A3'], anchor: 'A3' }, notA3)).toEqual({
      ids: ['A1'],
      anchor: null,
    });
  });

  it('returns the same object when nothing changes', () => {
    const s = { ids: ['A1'], anchor: 'A1' };
    expect(pruneSelection(s, notA3)).toBe(s);
  });
});
