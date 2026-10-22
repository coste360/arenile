import { describe, expect, it } from 'vitest';
import {
  alignPositions,
  clampToMap,
  distributePositions,
  elementBounds,
  itemsInRect,
  rectContains,
  rowAxis,
  normalizeRotation,
  rectFromPoints,
  rectsIntersect,
  snap,
  snapDelta,
  snapPoint,
  unionRect,
  type Placed,
} from './editing';

const circle = (x: number, y: number, w = 20): Placed => ({
  x,
  y,
  shape: { type: 'circle', w, h: w },
});

describe('snap', () => {
  it.each([
    [14, 10, 10],
    [15, 10, 20],
    [-4, 10, 0],
    [-6, 10, -10],
    [0.1 + 0.2, 0.1, 0.3],
    [33, 0, 33],
    [33, null, 33],
  ])('snap(%s, %s) = %s', (v, g, expected) => {
    expect(snap(v, g)).toBe(expected);
  });

  it('snaps points and never returns -0', () => {
    expect(snapPoint({ x: -1, y: 26 }, 5)).toEqual({ x: 0, y: 25 });
    expect(Object.is(snap(-1, 5), -0)).toBe(false);
  });

  it('moves a group by the delta that puts the anchor on the grid', () => {
    expect(snapDelta({ x: 12, y: 7 }, { x: 9, y: 1 }, 10)).toEqual({ x: 8, y: 3 });
    expect(snapDelta({ x: 12, y: 7 }, { x: 9, y: 1 }, 0)).toEqual({ x: 9, y: 1 });
  });
});

describe('bounds', () => {
  it('ignores rotation for circles', () => {
    expect(elementBounds({ ...circle(50, 50, 30), rotation: 45 })).toEqual({
      x: 35,
      y: 35,
      width: 30,
      height: 30,
    });
  });

  it('accounts for rotated rectangles', () => {
    const b = elementBounds({ x: 0, y: 0, rotation: 90, shape: { type: 'rect', w: 40, h: 20 } });
    expect(b).toEqual({ x: -10, y: -20, width: 20, height: 40 });
  });

  it('builds and intersects rubber-band rectangles', () => {
    const r = rectFromPoints({ x: 10, y: 50 }, { x: 0, y: 0 });
    expect(r).toEqual({ x: 0, y: 0, width: 10, height: 50 });
    expect(rectsIntersect(r, { x: 9, y: 49, width: 5, height: 5 })).toBe(true);
    expect(rectsIntersect(r, { x: 11, y: 0, width: 5, height: 5 })).toBe(false);
    expect(unionRect([])).toBeNull();
  });

  it('normalises rotations and clamps to the map', () => {
    expect(normalizeRotation(-15)).toBe(345);
    expect(normalizeRotation(720)).toBe(0);
    expect(clampToMap({ x: -5, y: 500 }, 100, 200)).toEqual({ x: 0, y: 200 });
  });
});

describe('align and distribute', () => {
  const items = [circle(10, 10), circle(50, 30, 40), circle(100, 20)];

  it('aligns left and right edges of the bounding boxes', () => {
    expect(alignPositions(items, 'left').map((p) => p.x)).toEqual([10, 20, 10]);
    expect(alignPositions(items, 'right').map((p) => p.x)).toEqual([100, 90, 100]);
  });

  it('aligns top, bottom and centres', () => {
    expect(alignPositions(items, 'top').map((p) => p.y)).toEqual([10, 20, 10]);
    expect(alignPositions(items, 'bottom').map((p) => p.y)).toEqual([40, 30, 40]);
    expect(alignPositions(items, 'center-x').map((p) => p.x)).toEqual([55, 55, 55]);
    expect(alignPositions(items, 'center-y').map((p) => p.y)).toEqual([25, 25, 25]);
  });

  it('distributes centres evenly keeping the outermost ones', () => {
    const pts = distributePositions(
      [circle(0, 0), circle(90, 0), circle(30, 5), circle(40, 0)],
      'x',
    );
    expect(pts.map((p) => p.x)).toEqual([0, 90, 30, 60]);
    expect(pts[2]?.y).toBe(5);
  });

  it('leaves fewer than three elements untouched', () => {
    expect(distributePositions([circle(0, 0), circle(7, 0)], 'x')).toEqual([
      { x: 0, y: 0 },
      { x: 7, y: 0 },
    ]);
  });
});

describe('rubber band', () => {
  const items = [circle(10, 10), circle(50, 50), circle(100, 100)];
  it('selects items intersecting the band', () => {
    expect(itemsInRect(items, rectFromPoints({ x: 15, y: 15 }, { x: 45, y: 45 }))).toEqual([0, 1]);
  });
  it('selects only fully contained items in contain mode', () => {
    expect(itemsInRect(items, { x: 0, y: 0, width: 60, height: 60 }, 'contain')).toEqual([0, 1]);
    expect(itemsInRect(items, { x: 0, y: 0, width: 55, height: 55 }, 'contain')).toEqual([0]);
    expect(
      rectContains({ x: 0, y: 0, width: 10, height: 10 }, { x: 2, y: 2, width: 8, height: 8 }),
    ).toBe(true);
  });
  it('finds the row axis from the sea side', () => {
    expect(rowAxis('bottom')).toBe('x');
    expect(rowAxis('top')).toBe('x');
    expect(rowAxis('left')).toBe('y');
    expect(rowAxis('right')).toBe('y');
  });
});
