import {
  alignPositions,
  clampToMap,
  distributePositions,
  itemsInRect,
  normalizeRotation,
  rowAxis,
  snapDelta,
  type AlignMode,
  type Point,
  type Rect,
} from '@arenile/shared/map';
import { newKey } from './document';
import type {
  DecorationKind,
  DraftDecoration,
  DraftDocument,
  DraftElement,
  DraftMapSettings,
  DraftSpot,
  Selection,
  SpotKind,
} from './types';

/**
 * Pure editing operations on the map draft (spec 11.2). Every function
 * returns a new document and never mutates its input, so the page can keep
 * an undo history of plain snapshots.
 */

/** All elements (spots first) in document order. */
export function elements(doc: DraftDocument): DraftElement[] {
  return [...doc.spots, ...doc.decorations];
}

export function findElement(doc: DraftDocument, key: string): DraftElement | undefined {
  return doc.spots.find((s) => s.key === key) ?? doc.decorations.find((d) => d.key === key);
}

function mapElements(
  doc: DraftDocument,
  keys: Selection,
  fn: (el: DraftElement) => DraftElement,
): DraftDocument {
  if (keys.size === 0) return doc;
  return {
    ...doc,
    spots: doc.spots.map((s) => (keys.has(s.key) ? (fn(s) as DraftSpot) : s)),
    decorations: doc.decorations.map((d) => (keys.has(d.key) ? (fn(d) as DraftDecoration) : d)),
  };
}

/**
 * Moves the selected elements by `delta`. With a grid the anchor element
 * lands on the grid and the others keep their relative offsets; centres are
 * kept inside the map.
 */
export function moveElements(
  doc: DraftDocument,
  keys: Selection,
  delta: Point,
  opts: { snap?: boolean; anchorKey?: string } = {},
): DraftDocument {
  if (keys.size === 0 || (delta.x === 0 && delta.y === 0)) return doc;
  const anchor = findElement(doc, opts.anchorKey ?? [...keys][0] ?? '');
  const d = anchor && opts.snap ? snapDelta(anchor, delta, doc.map.grid_size) : delta;
  if (d.x === 0 && d.y === 0) return doc;
  return mapElements(doc, keys, (el) => {
    const p = clampToMap(
      { x: round2(el.x + d.x), y: round2(el.y + d.y) },
      doc.map.width,
      doc.map.height,
    );
    return { ...el, x: p.x, y: p.y };
  });
}

/** Rotates the selected elements by `deg` degrees around their centres. */
export function rotateElements(doc: DraftDocument, keys: Selection, deg: number): DraftDocument {
  return mapElements(doc, keys, (el) => ({
    ...el,
    rotation: normalizeRotation(el.rotation + deg),
  }));
}

export function alignElements(doc: DraftDocument, keys: Selection, mode: AlignMode): DraftDocument {
  const items = elements(doc).filter((el) => keys.has(el.key));
  if (items.length < 2) return doc;
  return applyPositions(doc, items, alignPositions(items, mode));
}

export function distributeElements(
  doc: DraftDocument,
  keys: Selection,
  axis: 'x' | 'y',
): DraftDocument {
  const items = elements(doc).filter((el) => keys.has(el.key));
  if (items.length < 3) return doc;
  return applyPositions(doc, items, distributePositions(items, axis));
}

function applyPositions(doc: DraftDocument, items: DraftElement[], pos: Point[]): DraftDocument {
  const byKey = new Map(items.map((el, i) => [el.key, pos[i] as Point]));
  return mapElements(doc, new Set(byKey.keys()), (el) => {
    const p = byKey.get(el.key) as Point;
    return { ...el, x: round2(p.x), y: round2(p.y) };
  });
}

export function removeElements(doc: DraftDocument, keys: Selection): DraftDocument {
  if (keys.size === 0) return doc;
  return {
    ...doc,
    spots: doc.spots.filter((s) => !keys.has(s.key)),
    decorations: doc.decorations.filter((d) => !keys.has(d.key)),
  };
}

/** Applies `patch` to every selected spot (bulk zone, kind, active...). */
export function updateSpots(
  doc: DraftDocument,
  keys: Selection,
  patch: Partial<Omit<DraftSpot, 'key' | 'id'>>,
): DraftDocument {
  return {
    ...doc,
    spots: doc.spots.map((s) => (keys.has(s.key) ? { ...s, ...patch } : s)),
  };
}

export function updateDecoration(
  doc: DraftDocument,
  key: string,
  patch: Partial<Omit<DraftDecoration, 'key' | 'id'>>,
): DraftDocument {
  return {
    ...doc,
    decorations: doc.decorations.map((d) => (d.key === key ? { ...d, ...patch } : d)),
  };
}

export function updateMapSettings(
  doc: DraftDocument,
  patch: Partial<DraftMapSettings>,
): DraftDocument {
  return { ...doc, map: { ...doc.map, ...patch } };
}

/** Next free label ("N1", "N2", ...) not used by any spot nor in `taken`. */
export function nextFreeLabel(doc: DraftDocument, taken?: ReadonlySet<string>): string {
  const used = new Set(doc.spots.map((s) => s.label.trim().toLowerCase()));
  for (let n = doc.spots.length + 1; ; n++) {
    const label = `N${n}`;
    const k = label.toLowerCase();
    if (!used.has(k) && !taken?.has(k)) return label;
  }
}

/** A new spot at the centre of the map (or at `at`), in `zoneId`. */
export function newSpot(
  doc: DraftDocument,
  zoneId: string,
  opts: { at?: Point; kind?: SpotKind } = {},
): DraftSpot {
  const size = Math.max(10, Math.round((doc.map.grid_size || 10) * 4));
  const at = opts.at ?? { x: Math.round(doc.map.width / 2), y: Math.round(doc.map.height / 2) };
  const kind = opts.kind ?? 'umbrella';
  return {
    key: newKey(),
    zone_id: zoneId,
    label: nextFreeLabel(doc),
    row_label: '',
    number: null,
    kind,
    x: at.x,
    y: at.y,
    rotation: 0,
    shape:
      kind === 'gazebo'
        ? { type: 'rect', w: size * 1.5, h: size * 1.5 }
        : { type: 'circle', w: size, h: size },
    capacity: {},
    max_extra: {},
    price_category_override: '',
    attributes: [],
    active: true,
  };
}

export function newDecoration(
  doc: DraftDocument,
  kind: DecorationKind,
  at?: Point,
): DraftDecoration {
  const size = Math.max(10, Math.round((doc.map.grid_size || 10) * 4));
  const p = at ?? { x: Math.round(doc.map.width / 2), y: Math.round(doc.map.height / 2) };
  return {
    key: newKey(),
    kind,
    x: p.x,
    y: p.y,
    rotation: 0,
    shape:
      kind === 'walkway'
        ? { type: 'rect', w: size * 6, h: size / 2 }
        : { type: 'rect', w: size, h: size },
    label: {},
  };
}

export function addSpots(doc: DraftDocument, spots: DraftSpot[]): DraftDocument {
  return spots.length ? { ...doc, spots: [...doc.spots, ...spots] } : doc;
}

export function addDecoration(doc: DraftDocument, d: DraftDecoration): DraftDocument {
  return { ...doc, decorations: [...doc.decorations, d] };
}

/** Labels of `spots` already used on the map (case-insensitive). */
export function conflictingLabels(
  doc: DraftDocument,
  spots: readonly { label: string }[],
): string[] {
  const used = new Set(doc.spots.map((s) => s.label.trim().toLowerCase()));
  return spots.filter((s) => used.has(s.label.trim().toLowerCase())).map((s) => s.label);
}

/** Keys of every element (spots and decorations). */
export function allKeys(doc: DraftDocument): Set<string> {
  return new Set(elements(doc).map((el) => el.key));
}

/**
 * Keys of the elements hit by a rubber-band rectangle (map units). With
 * `base` (Shift/Ctrl held) the hit elements are added to it.
 */
export function selectInRect(doc: DraftDocument, rect: Rect, base?: Selection): Set<string> {
  const els = elements(doc);
  const hit = itemsInRect(els, rect).map((i) => (els[i] as DraftElement).key);
  return new Set([...(base ?? []), ...hit]);
}

/** Toggles `key` in an additive selection, or selects it alone. */
export function toggleSelection(prev: Selection, key: string, additive: boolean): Set<string> {
  if (!additive) return new Set([key]);
  const next = new Set(prev);
  if (next.has(key)) next.delete(key);
  else next.add(key);
  return next;
}

/**
 * Copies the selected elements, shifted by `offset`. Copies are new (no id)
 * and copied spots get free labels, since labels are unique per map.
 * Returns the document and the keys of the copies (the new selection).
 */
export function duplicateElements(
  doc: DraftDocument,
  keys: Selection,
  offset: Point,
): { doc: DraftDocument; keys: Set<string> } {
  if (keys.size === 0) return { doc, keys: new Set() };
  const newKeys = new Set<string>();
  const taken = new Set<string>();
  const place = <T extends DraftElement>(el: T): T => {
    const key = newKey();
    newKeys.add(key);
    const p = clampToMap(
      { x: round2(el.x + offset.x), y: round2(el.y + offset.y) },
      doc.map.width,
      doc.map.height,
    );
    const copy: T = { ...el, key, x: p.x, y: p.y };
    delete copy.id;
    return copy;
  };
  const spots = doc.spots
    .filter((s) => keys.has(s.key))
    .map((s) => {
      const label = nextFreeLabel(doc, taken);
      taken.add(label.toLowerCase());
      return {
        ...place(s),
        label,
        number: null,
        shape: { ...s.shape },
        capacity: { ...s.capacity },
        max_extra: { ...s.max_extra },
        attributes: [...s.attributes],
      };
    });
  const decorations = doc.decorations
    .filter((d) => keys.has(d.key))
    .map((d) => ({ ...place(d), shape: { ...d.shape }, label: { ...d.label } }));
  return {
    doc: {
      ...doc,
      spots: [...doc.spots, ...spots],
      decorations: [...doc.decorations, ...decorations],
    },
    keys: newKeys,
  };
}

export interface RenumberOptions {
  /** Row label written on every spot (e.g. "B"). */
  rowLabel: string;
  /** Number of the first spot. */
  start: number;
  /** Number from the far end of the row (right, or bottom). */
  reverse?: boolean;
  /** Label format with {row} and {number}; default "{row}{number}". */
  format?: string;
}

export function formatSpotLabel(format: string | undefined, row: string, n: number): string {
  const f = format?.trim() || '{row}{number}';
  return f.replaceAll('{row}', row).replaceAll('{number}', String(n));
}

/**
 * Renumbers the selected spots as one row: they are ordered along the
 * shoreline (x when the sea is at the top or bottom, y otherwise) and get
 * consecutive numbers, the row label and a label built from `format`.
 */
export function renumberRow(
  doc: DraftDocument,
  keys: Selection,
  opts: RenumberOptions,
): DraftDocument {
  const axis = rowAxis(doc.map.sea_side);
  const other = axis === 'x' ? 'y' : 'x';
  const row = opts.rowLabel.trim();
  const ordered = doc.spots
    .filter((s) => keys.has(s.key))
    .sort((a, b) => a[axis] - b[axis] || a[other] - b[other]);
  if (ordered.length === 0) return doc;
  if (opts.reverse) ordered.reverse();
  const patch = new Map(
    ordered.map((s, i) => {
      const n = opts.start + i;
      return [s.key, { row_label: row, number: n, label: formatSpotLabel(opts.format, row, n) }];
    }),
  );
  return {
    ...doc,
    spots: doc.spots.map((s) => {
      const p = patch.get(s.key);
      return p ? { ...s, ...p } : s;
    }),
  };
}

/** Differences between two versions of a map, by element id. */
export interface DocumentDiff {
  /** Labels of spots present only in `to` (new, or kept only there). */
  added: string[];
  /** Labels of spots present only in `from`. */
  removed: string[];
  /** Labels of spots whose position or rotation differ. */
  moved: string[];
  /** Labels of spots with other differences (label, zone, kind, capacity...). */
  changed: string[];
  /** Number of decorations added, removed or modified. */
  decorations: number;
  /** Map name, size, grid or sea side differ. */
  settings: boolean;
}

const spotGeometryKeys = ['x', 'y', 'rotation'] as const;

function sameJson(a: unknown, b: unknown): boolean {
  return JSON.stringify(a) === JSON.stringify(b);
}

function spotProps(s: DraftSpot) {
  // Everything but identity and geometry, with stable key order for comparison.
  return [
    s.zone_id,
    s.label,
    s.row_label,
    s.number,
    s.kind,
    s.shape,
    Object.entries(s.capacity).sort(),
    Object.entries(s.max_extra).sort(),
    s.price_category_override,
    [...s.attributes].sort(),
    s.active,
  ];
}

/**
 * What changes going from `from` to `to`. Elements are matched by server id;
 * elements without id (created in the draft) count as added.
 */
export function diffDocuments(from: DraftDocument, to: DraftDocument): DocumentDiff {
  const fromSpots = new Map(from.spots.filter((s) => s.id).map((s) => [s.id as string, s]));
  const toIds = new Set(to.spots.map((s) => s.id).filter(Boolean));
  const diff: DocumentDiff = {
    added: [],
    removed: [],
    moved: [],
    changed: [],
    decorations: 0,
    settings: false,
  };
  for (const s of to.spots) {
    const old = s.id ? fromSpots.get(s.id) : undefined;
    if (!old) {
      diff.added.push(s.label);
      continue;
    }
    if (spotGeometryKeys.some((k) => old[k] !== s[k])) diff.moved.push(s.label);
    if (!sameJson(spotProps(old), spotProps(s))) diff.changed.push(s.label);
  }
  for (const s of from.spots) if (!s.id || !toIds.has(s.id)) diff.removed.push(s.label);

  const fromDecos = new Map(from.decorations.filter((d) => d.id).map((d) => [d.id as string, d]));
  const toDecoIds = new Set(to.decorations.map((d) => d.id).filter(Boolean));
  for (const d of to.decorations) {
    const old = d.id ? fromDecos.get(d.id) : undefined;
    if (!old || !sameJson({ ...old, key: '' }, { ...d, key: '' })) diff.decorations++;
  }
  for (const d of from.decorations) if (!d.id || !toDecoIds.has(d.id)) diff.decorations++;

  diff.settings = !sameJson(from.map, to.map);
  return diff;
}

export function isEmptyDiff(d: DocumentDiff): boolean {
  return (
    !d.added.length &&
    !d.removed.length &&
    !d.moved.length &&
    !d.changed.length &&
    !d.decorations &&
    !d.settings
  );
}

function round2(v: number): number {
  const r = Math.round(v * 100) / 100;
  return r === 0 ? 0 : r;
}

/** Bounded undo/redo history of document snapshots. */
export interface History {
  past: DraftDocument[];
  present: DraftDocument;
  future: DraftDocument[];
}

export const HISTORY_LIMIT = 100;

export function initHistory(doc: DraftDocument): History {
  return { past: [], present: doc, future: [] };
}

/** Records `next` as a new step (no-op when the document did not change). */
export function pushHistory(h: History, next: DraftDocument): History {
  if (next === h.present) return h;
  const past = [...h.past, h.present];
  if (past.length > HISTORY_LIMIT) past.shift();
  return { past, present: next, future: [] };
}

/** Replaces the present without a new undo step (e.g. while dragging). */
export function replacePresent(h: History, next: DraftDocument): History {
  return next === h.present ? h : { ...h, present: next };
}

export function undo(h: History): History {
  const prev = h.past[h.past.length - 1];
  if (!prev) return h;
  return { past: h.past.slice(0, -1), present: prev, future: [h.present, ...h.future] };
}

export function redo(h: History): History {
  const [next, ...rest] = h.future;
  if (!next) return h;
  return { past: [...h.past, h.present], present: next, future: rest };
}

/**
 * Closes a live interaction (a drag) as a single undo step: `base` is the
 * document before it started, the present is its result.
 */
export function commitFrom(h: History, base: DraftDocument): History {
  if (base === h.present) return h;
  const past = [...h.past, base];
  if (past.length > HISTORY_LIMIT) past.shift();
  return { past, present: h.present, future: [] };
}

/**
 * First position from `at` (stepping diagonally by two grid steps) not
 * already occupied by an element centre, so repeated additions from the
 * palette do not stack on top of each other.
 */
export function freePosition(doc: DraftDocument, at: Point): Point {
  const step = (doc.map.grid_size > 0 ? doc.map.grid_size : 10) * 2;
  const taken = new Set(elements(doc).map((el) => `${el.x},${el.y}`));
  let p = at;
  for (let i = 0; i < 500 && taken.has(`${p.x},${p.y}`); i++) {
    p = clampToMap({ x: p.x + step, y: p.y + step }, doc.map.width, doc.map.height);
    if (p.x >= doc.map.width && p.y >= doc.map.height) break;
  }
  return p;
}
