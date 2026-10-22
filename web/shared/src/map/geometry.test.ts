import { describe, expect, it } from 'vitest';
import { IDENTITY, MAX_SCALE, computeViewBox, seaRect, zoomAt } from './geometry';

describe('computeViewBox', () => {
  it.each([
    ['top', { x: 0, y: -24, width: 200, height: 124 }],
    ['bottom', { x: 0, y: 0, width: 200, height: 124 }],
    ['left', { x: -24, y: 0, width: 224, height: 100 }],
    ['right', { x: 0, y: 0, width: 224, height: 100 }],
  ] as const)('adds the sea band on the %s side', (side, expected) => {
    expect(computeViewBox({ width: 200, height: 100, sea_side: side })).toEqual(expected);
  });

  it('places the sea rectangle outside the sand', () => {
    expect(seaRect({ width: 1000, height: 600, sea_side: 'bottom' })).toEqual({
      x: 0,
      y: 600,
      width: 1000,
      height: 72,
    });
  });
});

describe('zoomAt', () => {
  it('keeps the focal point fixed', () => {
    const p = { x: 50, y: 40 };
    const t = zoomAt(IDENTITY, p, 2);
    expect(t.scale).toBe(2);
    // the map point under p before zooming is (50, 40); after: t + 2 * (50, 40)
    expect(t.x + 2 * 50).toBeCloseTo(p.x);
    expect(t.y + 2 * 40).toBeCloseTo(p.y);
  });

  it('clamps the scale', () => {
    expect(zoomAt(IDENTITY, { x: 0, y: 0 }, 1000).scale).toBe(MAX_SCALE);
  });
});
