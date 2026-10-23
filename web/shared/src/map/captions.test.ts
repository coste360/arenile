import { describe, expect, it } from 'vitest';
import {
  MAX_CAPTION_CHARS,
  captionsLegible,
  labelPillWidth,
  layoutCaption,
  spotNeighbours,
  truncateLabel,
  type SpotNeighbours,
} from './captions';
import type { MapSpot } from './types';

const spot = (id: string, x: number, y: number, w = 40): MapSpot => ({
  id,
  label: id.toUpperCase(),
  kind: 'umbrella',
  x,
  y,
  shape: { type: 'circle', w, h: w },
});

describe('truncateLabel', () => {
  it.each([
    ['Rossi', 12, 'Rossi'],
    ['  De   Santis ', 12, 'De Santis'],
    ['Mastrogiacomo', 12, 'Mastrogiaco…'],
    ['Mastrogiacomo', 6, 'Mastr…'],
    ['De Luca', 4, 'De…'],
    ['Rossi', 1, '…'],
    ['', 12, ''],
  ])('%j at %i → %j', (text, max, want) => {
    expect(truncateLabel(text, max)).toBe(want);
  });

  it('never splits a grapheme (accents, emoji)', () => {
    const e = 'é'; // e + combining acute
    expect(truncateLabel(`Andr${e}ëàòù`, 6)).toBe(`Andr${e}…`);
    expect(truncateLabel('👨‍👩‍👧Rossi', 3)).toBe('👨‍👩‍👧R…');
  });

  it('defaults to MAX_CAPTION_CHARS', () => {
    expect(Array.from(truncateLabel('x'.repeat(40)))).toHaveLength(MAX_CAPTION_CHARS);
  });
});

describe('spotNeighbours', () => {
  it('finds row neighbours and the next row, ignoring other columns', () => {
    const spots = [
      spot('a1', 0, 0),
      spot('a2', 60, 2),
      spot('a3', 120, 0),
      spot('b1', 0, 70),
      spot('b9', 400, 70),
    ];
    const n = spotNeighbours(spots, (s) => s.shape.w);
    expect(n.get('a2')).toMatchObject({ left: 'a1', right: 'a3', leftGap: 60, rightGap: 60 });
    expect(n.get('a1')).toMatchObject({ right: 'a2', leftGap: Infinity, belowGap: 70 });
    expect(n.get('a1')?.left).toBeUndefined();
    expect(n.get('a3')?.belowGap).toBe(Infinity);
    expect(n.get('b1')?.belowGap).toBe(Infinity);
  });
});

describe('layoutCaption', () => {
  const S = 40;
  const row: SpotNeighbours = { left: 'l', right: 'r', leftGap: 60, rightGap: 60, belowGap: 80 };
  const all = () => true;

  it('stays between the neighbours and truncates to the room', () => {
    const c = layoutCaption(spot('x', 100, 100), 'Mastrogiacomo', S, row, all, 1)!;
    // Half of each gap minus a margin: never into a captioned neighbour's half.
    expect(labelPillWidth(Array.from(c.text).length, c.fontSize, 0.56) / 2).toBeLessThanOrEqual(30);
    expect(c.text.endsWith('…')).toBe(true);
    expect(c.x).toBe(100);
    // In the band between this art and the next row.
    expect(c.y).toBeGreaterThan(100 + S * 0.4);
    expect(c.y).toBeLessThan(180 - S * 0.4);
  });

  it('uses the room of a free neighbour, shifted towards it', () => {
    const tight = layoutCaption(spot('x', 100, 100), 'Mastrogiacomo', S, row, all, 1)!;
    // Only the left neighbour has a caption: the right one is free.
    const c = layoutCaption(spot('x', 100, 100), 'Mastrogiacomo', S, row, (id) => id === 'l', 1)!;
    expect(c.text.length).toBeGreaterThan(tight.text.length);
    expect(c.x).toBeGreaterThan(100);
  });

  it('keeps short names whole', () => {
    expect(layoutCaption(spot('x', 100, 100), 'Rossi', S, row, all, 1)?.text).toBe('Rossi');
  });

  it('hides the caption when it would be under 9 px or the band is too low', () => {
    // Zoomed out: 9 px on screen is more than 42% of the art.
    expect(layoutCaption(spot('x', 100, 100), 'Rossi', S, row, all, 3)).toBeUndefined();
    // Rows touching: no band for the pill.
    expect(
      layoutCaption(spot('x', 100, 100), 'Rossi', S, { ...row, belowGap: S }, all, 1),
    ).toBeUndefined();
  });

  it('hides the caption when fewer than 4 characters fit', () => {
    const tight: SpotNeighbours = { ...row, leftGap: 30, rightGap: 30 };
    expect(layoutCaption(spot('x', 100, 100), 'Mastrogiacomo', S, tight, all, 1)).toBeUndefined();
    // ...but a name that fits whole is still drawn.
    expect(layoutCaption(spot('x', 100, 100), 'Ri', S, tight, all, 1)?.text).toBe('Ri');
  });

  it('a lone spot (no neighbours, last row) gets the full caption under the art', () => {
    const c = layoutCaption(spot('x', 0, 0), 'Mastrogiacomo', S, undefined, all, 1)!;
    expect(c.text).toBe('Mastrogiaco…');
    expect(c.y).toBeGreaterThan(S * 0.4);
  });
});

describe('captionsLegible', () => {
  const grid = (pitch: number) =>
    Array.from({ length: 30 }, (_, i) => spot(`s${i}`, (i % 10) * pitch, Math.floor(i / 10) * 80));
  const size = (s: MapSpot) => s.shape.w;

  it('is decided for the whole map from the typical spacing', () => {
    const roomy = grid(70);
    expect(captionsLegible(roomy, size, spotNeighbours(roomy, size), 1)).toBe(true);
    const dense = grid(42);
    // Same map zoomed out: names do not fit between neighbours.
    expect(captionsLegible(dense, size, spotNeighbours(dense, size), 1.6)).toBe(false);
    // Zoom in: they do again.
    expect(captionsLegible(dense, size, spotNeighbours(dense, size), 0.7)).toBe(true);
  });

  it('a lone spot is legible when the font is', () => {
    const one = [spot('a', 0, 0)];
    expect(captionsLegible(one, size, spotNeighbours(one, size), 1)).toBe(true);
    expect(captionsLegible([], size, new Map(), 1)).toBe(false);
  });
});

describe('layoutCaption under the code pill', () => {
  it('starts below minTop and gives up when the band is gone', () => {
    const n: SpotNeighbours = { leftGap: Infinity, rightGap: Infinity, belowGap: 80 };
    const c = layoutCaption(spot('x', 0, 0), 'Rossi', 40, n, () => true, 1, 25)!;
    expect(c.y - (c.fontSize * 1.32) / 2).toBeGreaterThanOrEqual(25);
    expect(layoutCaption(spot('x', 0, 0), 'Rossi', 40, n, () => true, 1, 60)).toBeUndefined();
  });
});
