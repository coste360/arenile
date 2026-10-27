import {
  KIT_BY_ID,
  KIT_TILE,
  alignPositions,
  clampToMap,
  decorationAsset,
  isTileAsset,
  snapTileCentre,
  snapTileLength,
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

/**
 * Whether walkway tiles snap to the 60-unit tile grid on a map with this
 * grid step: only when the tile grid is a subset of the map grid (the step
 * divides KIT_TILE), so tiles never fight the grid the spots sit on. With
 * grid snapping off (step 0) or an incompatible step, tiles keep their
 * position and only their size is normalised.
 */
export function tilesSnapToGrid(gridSize: number): boolean {
  return gridSize > 0 && KIT_TILE % gridSize === 0;
}

/**
 * Modular walkway tiles (ADR 0012) keep the kit scale so that pieces join:
 * straight runs are one tile thick and a whole number of tiles long, the
 * other pieces are one tile. When `grid` allows it (tilesSnapToGrid), edges
 * are aligned to multiples of KIT_TILE; `from` is the previous centre, so a
 * small move (arrow nudge, duplicate) advances one whole tile instead of
 * snapping back.
 */
export function normalizeTile(
  d: DraftDecoration,
  grid: number,
  from?: { x: number; y: number },
): DraftDecoration {
  const asset = decorationAsset(d);
  if (!isTileAsset(asset)) return d;
  const straight = asset === 'passerella_orizzontale' || asset === 'passerella_verticale';
  const horizontal = d.shape.w >= d.shape.h;
  const w = straight && horizontal ? snapTileLength(d.shape.w) : KIT_TILE;
  const h = straight && !horizontal ? snapTileLength(d.shape.h) : KIT_TILE;
  const snap = tilesSnapToGrid(grid);
  const x = snap ? snapTileCentre(d.x, w, from?.x) : d.x;
  const y = snap ? snapTileCentre(d.y, h, from?.y) : d.y;
  if (x === d.x && y === d.y && w === d.shape.w && h === d.shape.h && d.shape.type === 'rect')
    return d;
  return { ...d, x, y, shape: { type: 'rect', w, h } };
}

/**
 * Patch for a change of spot kind: the kind is semantic (packs'
 * requires_spot_kind, assignment), the illustration only draws it. An
 * explicit illustration of another kind is dropped, so a gazebo is never
 * drawn as a hammock; the default art of the new kind takes over.
 */
export function spotKindPatch(kind: SpotKind, asset: string): Pick<DraftSpot, 'kind' | 'asset'> {
  const a = KIT_BY_ID.get(asset);
  return { kind, asset: a && a.spotKind === kind ? asset : '' };
}

/**
 * Patch for a change of decoration kind: a text has no illustration, and an
 * illustration whose family does not match the new kind (a flag turned into
 * a bar) is dropped with its variant; `custom` keeps any illustration.
 */
export function decorationKindPatch(
  kind: DecorationKind,
  asset: string,
  variant: string,
): Pick<DraftDecoration, 'kind' | 'asset' | 'variant'> {
  const a = KIT_BY_ID.get(asset);
  const keep = kind !== 'text' && !!a && (kind === 'custom' || a.decorationKind === kind);
  return keep ? { kind, asset, variant } : { kind, asset: '', variant: '' };
}

function isDecoration(el: DraftElement): el is DraftDecoration {
  return !('zone_id' in el);
}

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
  // A drag is already where the pointer left it; a nudge moves whole tiles.
  const nudge = !opts.anchorKey;
  return mapElements(doc, keys, (el) => {
    const p = clampToMap(
      { x: round2(el.x + d.x), y: round2(el.y + d.y) },
      doc.map.width,
      doc.map.height,
    );
    const moved = { ...el, x: p.x, y: p.y };
    return isDecoration(moved)
      ? normalizeTile(moved, doc.map.grid_size, nudge ? el : undefined)
      : moved;
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
    decorations: doc.decorations.map((d) =>
      d.key === key ? normalizeTile({ ...d, ...patch }, doc.map.grid_size) : d,
    ),
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
  opts: { at?: Point; kind?: SpotKind; asset?: string } = {},
): DraftSpot {
  const size = Math.max(10, Math.round((doc.map.grid_size || 10) * 4));
  const at = opts.at ?? { x: Math.round(doc.map.width / 2), y: Math.round(doc.map.height / 2) };
  const kind = opts.kind ?? 'umbrella';
  const square = kind === 'gazebo' || kind === 'cabana' || kind === 'daybed';
  return {
    key: newKey(),
    zone_id: zoneId,
    label: nextFreeLabel(doc),
    row_label: '',
    number: null,
    kind,
    asset: opts.asset ?? '',
    x: at.x,
    y: at.y,
    rotation: 0,
    shape: square
      ? { type: 'rect', w: size * 1.5, h: size * 1.5 }
      : { type: 'circle', w: size, h: size },
    capacity: {},
    max_extra: {},
    price_category_override: '',
    attributes: [],
    active: true,
  };
}

/** Default side of a new decoration of a kit category, in grid steps. */
const DECORATION_STEPS: Record<string, number> = { structures: 7, sport: 9, nature: 5 };

export function newDecoration(
  doc: DraftDocument,
  kind: DecorationKind,
  at?: Point,
  asset = '',
): DraftDecoration {
  const grid = doc.map.grid_size || 10;
  const p = at ?? { x: Math.round(doc.map.width / 2), y: Math.round(doc.map.height / 2) };
  const category = KIT_BY_ID.get(asset)?.category ?? '';
  const size = Math.max(10, Math.round(grid * (DECORATION_STEPS[category] ?? 4.5)));
  const d: DraftDecoration = {
    key: newKey(),
    kind,
    asset,
    variant: '',
    x: p.x,
    y: p.y,
    rotation: 0,
    shape:
      kind === 'walkway'
        ? { type: 'rect', w: KIT_TILE * 3, h: KIT_TILE }
        : kind === 'text'
          ? { type: 'rect', w: size * 3, h: size }
          : { type: 'rect', w: size, h: size },
    label: {},
  };
  return normalizeTile(d, doc.map.grid_size);
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
    .map((d) =>
      normalizeTile(
        { ...place(d), shape: { ...d.shape }, label: { ...d.label } },
        doc.map.grid_size,
        d,
      ),
    );
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
    s.online_bookable !== false,
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
