import { describe, expect, it } from 'vitest';
import { toSaveRequest, validateDraft } from './document';
import {
  addSpots,
  alignElements,
  commitFrom,
  conflictingLabels,
  diffDocuments,
  distributeElements,
  duplicateElements,
  freePosition,
  isEmptyDiff,
  renumberRow,
  selectInRect,
  toggleSelection,
  initHistory,
  moveElements,
  newSpot,
  pushHistory,
  redo,
  removeElements,
  replacePresent,
  rotateElements,
  undo,
  updateSpots,
  normalizeTile,
  spotKindPatch,
  decorationKindPatch,
  tilesSnapToGrid,
  newDecoration,
  updateDecoration,
} from './editor';
import type { DraftDocument, DraftSpot } from './types';

function spot(key: string, x: number, y: number, extra: Partial<DraftSpot> = {}): DraftSpot {
  return {
    key,
    id: key,
    zone_id: 'z1',
    label: key.toUpperCase(),
    row_label: '',
    number: null,
    kind: 'umbrella',
    asset: '',
    x,
    y,
    rotation: 0,
    shape: { type: 'circle', w: 20, h: 20 },
    capacity: {},
    max_extra: {},
    price_category_override: '',
    attributes: [],
    active: true,
    ...extra,
  };
}

function doc(spots: DraftSpot[]): DraftDocument {
  return {
    map: {
      name: { it: 'Spiaggia' },
      width: 500,
      height: 300,
      grid_size: 10,
      sea_side: 'bottom',
      sort_order: 0,
    },
    spots,
    decorations: [],
  };
}

describe('moveElements', () => {
  it('moves only the selection and keeps the input untouched', () => {
    const d = doc([spot('a', 100, 100), spot('b', 200, 100)]);
    const next = moveElements(d, new Set(['a']), { x: 12, y: -3 });
    expect(next.spots[0]).toMatchObject({ x: 112, y: 97 });
    expect(next.spots[1]).toBe(d.spots[1]);
    expect(d.spots[0]?.x).toBe(100);
  });

  it('snaps the anchor to the grid and keeps relative offsets', () => {
    const d = doc([spot('a', 100, 100), spot('b', 133, 100)]);
    const next = moveElements(
      d,
      new Set(['a', 'b']),
      { x: 14, y: 6 },
      { snap: true, anchorKey: 'a' },
    );
    expect(next.spots[0]).toMatchObject({ x: 110, y: 110 });
    expect(next.spots[1]).toMatchObject({ x: 143, y: 110 });
  });

  it('keeps centres inside the map', () => {
    const d = doc([spot('a', 490, 290)]);
    const next = moveElements(d, new Set(['a']), { x: 50, y: 50 });
    expect(next.spots[0]).toMatchObject({ x: 500, y: 300 });
  });

  it('returns the same document for an empty selection or a null move', () => {
    const d = doc([spot('a', 1, 1)]);
    expect(moveElements(d, new Set(), { x: 5, y: 5 })).toBe(d);
    expect(moveElements(d, new Set(['a']), { x: 0, y: 0 })).toBe(d);
  });
});

describe('bulk editing', () => {
  it('aligns, distributes, rotates, updates and removes', () => {
    const d = doc([spot('a', 10, 10), spot('b', 50, 30), spot('c', 70, 50)]);
    const all = new Set(['a', 'b', 'c']);
    expect(alignElements(d, all, 'top').spots.map((s) => s.y)).toEqual([10, 10, 10]);
    expect(distributeElements(d, all, 'x').spots.map((s) => s.x)).toEqual([10, 40, 70]);
    expect(rotateElements(d, new Set(['a']), -15).spots[0]?.rotation).toBe(345);
    const moved = updateSpots(d, new Set(['b']), { zone_id: 'z2', active: false });
    expect(moved.spots[1]).toMatchObject({ zone_id: 'z2', active: false });
    expect(removeElements(d, new Set(['b'])).spots.map((s) => s.key)).toEqual(['a', 'c']);
  });

  it('creates new spots with a free label and no id (created by the server)', () => {
    const d = doc([spot('n2', 10, 10, { label: 'N2' })]);
    const s = newSpot(d, 'z1');
    expect(s.label).toBe('N3');
    expect(s.id).toBeUndefined();
    const body = toSaveRequest(addSpots(d, [s]));
    expect(body.spots[1]).not.toHaveProperty('id');
    expect(body.spots[0]).toHaveProperty('id', 'n2');
  });

  it('reports labels already on the map', () => {
    const d = doc([spot('a', 1, 1, { label: 'A1' })]);
    expect(conflictingLabels(d, [{ label: 'a1' }, { label: 'A2' }])).toEqual(['a1']);
    expect(validateDraft(addSpots(d, [spot('x', 2, 2, { label: 'a1' })]))).toEqual([
      { type: 'duplicate_label', keys: ['a', 'x'], label: 'A1' },
    ]);
  });
});

describe('history', () => {
  it('undoes and redoes steps; a drag is a single step', () => {
    const d0 = doc([spot('a', 10, 10)]);
    const d1 = moveElements(d0, new Set(['a']), { x: 10, y: 0 });
    const d2 = moveElements(d1, new Set(['a']), { x: 10, y: 0 });
    let h = pushHistory(initHistory(d0), d1);
    h = replacePresent(h, d2);
    expect(h.past).toHaveLength(1);
    h = undo(h);
    expect(h.present).toBe(d0);
    h = redo(h);
    expect(h.present).toBe(d2);
    expect(undo(initHistory(d0)).present).toBe(d0);
    expect(pushHistory(h, h.present)).toBe(h);
  });
});

describe('selection', () => {
  it('toggles with shift and replaces otherwise', () => {
    const sel = new Set(['a']);
    expect([...toggleSelection(sel, 'b', true)]).toEqual(['a', 'b']);
    expect([...toggleSelection(sel, 'a', true)]).toEqual([]);
    expect([...toggleSelection(sel, 'b', false)]).toEqual(['b']);
  });

  it('selects the elements hit by the rubber band, adding to a base selection', () => {
    const d = doc([spot('a', 10, 10), spot('b', 50, 50), spot('c', 200, 200)]);
    const band = { x: 0, y: 0, width: 45, height: 45 };
    expect([...selectInRect(d, band)]).toEqual(['a', 'b']);
    expect([...selectInRect(d, { x: 300, y: 0, width: 5, height: 5 })]).toEqual([]);
    expect([...selectInRect(d, band, new Set(['c']))].sort()).toEqual(['a', 'b', 'c']);
  });
});

describe('align and distribute', () => {
  it('aligns edges using the bounding boxes, not the centres', () => {
    const d = doc([
      spot('a', 100, 100, { shape: { type: 'circle', w: 20, h: 20 } }),
      spot('b', 200, 150, { shape: { type: 'circle', w: 40, h: 40 } }),
    ]);
    const both = new Set(['a', 'b']);
    // left edges: 90 and 180 -> both at 90
    expect(alignElements(d, both, 'left').spots.map((s) => s.x)).toEqual([100, 110]);
    // right edges: 110 and 220 -> both at 220
    expect(alignElements(d, both, 'right').spots.map((s) => s.x)).toEqual([210, 200]);
    // bottom edges: 110 and 170 -> both at 170
    expect(alignElements(d, both, 'bottom').spots.map((s) => s.y)).toEqual([160, 150]);
    expect(alignElements(d, both, 'center-x').spots.map((s) => s.x)).toEqual([155, 155]);
    expect(alignElements(d, both, 'center-y').spots.map((s) => s.y)).toEqual([130, 130]);
  });

  it('needs two elements to align and three to distribute', () => {
    const d = doc([spot('a', 10, 10), spot('b', 50, 70)]);
    expect(alignElements(d, new Set(['a']), 'left')).toBe(d);
    expect(distributeElements(d, new Set(['a', 'b']), 'y')).toBe(d);
  });

  it('distributes vertically regardless of the document order', () => {
    const d = doc([spot('a', 0, 100), spot('b', 0, 10), spot('c', 0, 40)]);
    expect(distributeElements(d, new Set(['a', 'b', 'c']), 'y').spots.map((s) => s.y)).toEqual([
      100, 10, 55,
    ]);
  });
});

describe('duplicate', () => {
  it('copies the selection with an offset, free labels and no ids', () => {
    const d = doc([spot('a', 10, 10, { label: 'A1', number: 1 }), spot('b', 50, 10)]);
    const r = duplicateElements(d, new Set(['a']), { x: 20, y: 20 });
    expect(r.doc.spots).toHaveLength(3);
    const copy = r.doc.spots[2] as DraftSpot;
    expect(r.keys).toEqual(new Set([copy.key]));
    expect(copy).toMatchObject({ x: 30, y: 30, number: null, zone_id: 'z1' });
    expect(copy.id).toBeUndefined();
    expect(copy.label).not.toBe('A1');
    expect(validateDraft(r.doc)).toEqual([]);
  });

  it('gives distinct labels to several copies', () => {
    const d = doc([spot('a', 10, 10), spot('b', 50, 10)]);
    const r = duplicateElements(d, new Set(['a', 'b']), { x: 0, y: 20 });
    expect(validateDraft(r.doc)).toEqual([]);
    expect(duplicateElements(d, new Set(), { x: 1, y: 1 }).doc).toBe(d);
  });
});

describe('renumberRow', () => {
  it('numbers the selection along the shoreline (x when the sea is at the bottom)', () => {
    const d = doc([spot('c', 90, 10), spot('a', 10, 10), spot('b', 50, 12), spot('z', 1, 1)]);
    const next = renumberRow(d, new Set(['a', 'b', 'c']), { rowLabel: 'B', start: 1 });
    expect(next.spots.map((s) => s.label)).toEqual(['B3', 'B1', 'B2', 'Z']);
    expect(next.spots[0]).toMatchObject({ row_label: 'B', number: 3 });
    expect(next.spots[3]).toBe(d.spots[3]);
  });

  it('supports reverse order, a custom format and vertical rows', () => {
    const base = doc([spot('a', 10, 10), spot('b', 10, 50)]);
    const vertical = { ...base, map: { ...base.map, sea_side: 'left' as const } };
    const next = renumberRow(vertical, new Set(['a', 'b']), {
      rowLabel: '2',
      start: 10,
      reverse: true,
      format: '{row}-{number}',
    });
    expect(next.spots.map((s) => s.label)).toEqual(['2-11', '2-10']);
  });
});

describe('diffDocuments', () => {
  it('reports added, removed, moved and changed spots by id', () => {
    const from = doc([spot('a', 10, 10), spot('b', 20, 20), spot('c', 30, 30)]);
    const to = doc([
      spot('a', 15, 10),
      spot('b', 20, 20, { zone_id: 'z2' }),
      spot('new:1', 40, 40, { id: undefined, label: 'N1' }),
    ]);
    const diff = diffDocuments(from, to);
    expect(diff).toMatchObject({
      added: ['N1'],
      removed: ['C'],
      moved: ['A'],
      changed: ['B'],
      decorations: 0,
      settings: false,
    });
    expect(isEmptyDiff(diffDocuments(from, from))).toBe(true);
  });
});

describe('placement', () => {
  it('finds a free position next to occupied ones', () => {
    const d = doc([spot('a', 100, 100)]);
    expect(freePosition(d, { x: 100, y: 100 })).toEqual({ x: 120, y: 120 });
    expect(freePosition(d, { x: 10, y: 10 })).toEqual({ x: 10, y: 10 });
  });
});

describe('commitFrom', () => {
  it('records an interaction as a single undo step', () => {
    const d0 = doc([spot('a', 10, 10)]);
    const d1 = moveElements(d0, new Set(['a']), { x: 30, y: 0 });
    const h = commitFrom(replacePresent(initHistory(d0), d1), d0);
    expect(h.past).toEqual([d0]);
    expect(undo(h).present).toBe(d0);
    expect(commitFrom(h, h.present)).toBe(h);
  });
});

describe('kit walkway tiles', () => {
  const base = doc([]);
  it('a new walkway is three tiles on the 60-unit tile grid', () => {
    const d = newDecoration(base, 'walkway', { x: 247, y: 131 });
    expect(d.shape).toEqual({ type: 'rect', w: 180, h: 60 });
    expect((((d.x - d.shape.w / 2) % 60) + 60) % 60).toBe(0);
    expect((((d.y - d.shape.h / 2) % 60) + 60) % 60).toBe(0);
  });

  it('pieces other than straight runs are one tile; straight runs are whole tiles long', () => {
    const corner = newDecoration(base, 'walkway', { x: 100, y: 100 }, 'passerella_angolo');
    expect(corner.shape).toMatchObject({ w: 60, h: 60 });
    const run = normalizeTile(
      { ...corner, asset: 'passerella_verticale', shape: { type: 'rect', w: 40, h: 170 } },
      10,
    );
    expect(run.shape).toMatchObject({ w: 60, h: 180 });
  });

  it('arrow nudges and duplicates move whole tiles, so pieces keep joining', () => {
    const d = newDecoration(base, 'walkway', { x: 90, y: 90 }, 'passerella_croce');
    let next = { ...base, decorations: [d] };
    next = moveElements(next, new Set([d.key]), { x: 10, y: 0 });
    expect(next.decorations[0]?.x).toBe(d.x + 60);
    const dup = duplicateElements(next, new Set([d.key]), { x: 20, y: 20 });
    const copy = dup.doc.decorations[1];
    expect(copy && [copy.x - (next.decorations[0]?.x ?? 0), copy.y - d.y]).toEqual([60, 60]);
  });

  it('resizing a straight run snaps the length to whole tiles', () => {
    const d = newDecoration(base, 'walkway', { x: 150, y: 90 });
    const next = updateDecoration({ ...base, decorations: [d] }, d.key, {
      shape: { type: 'rect', w: 250, h: 60 },
    });
    expect(next.decorations[0]?.shape.w).toBe(240);
  });

  it('non-tile decorations are untouched', () => {
    const bar = newDecoration(base, 'bar', { x: 123, y: 77 }, 'bar');
    expect(bar).toMatchObject({ x: 123, y: 77, asset: 'bar' });
    expect(normalizeTile(bar, 10)).toBe(bar);
  });

  it('keeps the position when the map grid does not divide the tile, or snapping is off', () => {
    expect(tilesSnapToGrid(10)).toBe(true);
    expect(tilesSnapToGrid(20)).toBe(true);
    expect(tilesSnapToGrid(25)).toBe(false);
    expect(tilesSnapToGrid(0)).toBe(false);
    for (const grid of [25, 0]) {
      const d25 = { ...base, map: { ...base.map, grid_size: grid } };
      const d = newDecoration(d25, 'walkway', { x: 247, y: 131 });
      expect(d).toMatchObject({ x: 247, y: 131, shape: { w: 180, h: 60 } });
      const moved = moveElements({ ...d25, decorations: [d] }, new Set([d.key]), { x: 5, y: 0 });
      expect(moved.decorations[0]?.x).toBe(252);
    }
  });
});

describe('kind and illustration coupling', () => {
  it('a change of spot kind drops an illustration of another kind', () => {
    expect(spotKindPatch('sunbed', 'ombrellone_2_lettini')).toEqual({ kind: 'sunbed', asset: '' });
    expect(spotKindPatch('umbrella', 'ombrellone_3_lettini')).toEqual({
      kind: 'umbrella',
      asset: 'ombrellone_3_lettini',
    });
    expect(spotKindPatch('gazebo', '')).toEqual({ kind: 'gazebo', asset: '' });
  });

  it('a change of decoration kind drops a foreign illustration, a text never has one', () => {
    expect(decorationKindPatch('text', 'bandiera_mare', 'red')).toEqual({
      kind: 'text',
      asset: '',
      variant: '',
    });
    expect(decorationKindPatch('bar', 'bandiera_mare', 'red')).toEqual({
      kind: 'bar',
      asset: '',
      variant: '',
    });
    expect(decorationKindPatch('custom', 'bandiera_mare', 'red')).toEqual({
      kind: 'custom',
      asset: 'bandiera_mare',
      variant: 'red',
    });
    expect(decorationKindPatch('shower', 'doccia_doppia', '')).toEqual({
      kind: 'shower',
      asset: 'doccia_doppia',
      variant: '',
    });
  });
});
