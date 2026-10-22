import type { SeaSide, Shape } from './types';

/**
 * Editor-friendly geometry (spec 11.2): grid snapping, bounding boxes of
 * rotated shapes, rubber-band rectangles, alignment and distribution.
 * Everything here is pure and works in logical map units.
 */

export interface Point {
  x: number;
  y: number;
}

export interface Rect {
  x: number;
  y: number;
  width: number;
  height: number;
}

/** Anything placed on the map: centre, optional rotation in degrees, shape. */
export interface Placed {
  x: number;
  y: number;
  rotation?: number | null;
  shape: Shape;
}

/** Rounds `value` to the nearest multiple of `grid` (no-op when grid <= 0). */
export function snap(value: number, grid: number | null | undefined): number {
  if (!grid || grid <= 0 || !Number.isFinite(grid)) return value;
  const snapped = Math.round(value / grid) * grid;
  // Avoid -0 and floating noise such as 30.000000000000004.
  return Number((snapped === 0 ? 0 : snapped).toFixed(6));
}

export function snapPoint(p: Point, grid: number | null | undefined): Point {
  return { x: snap(p.x, grid), y: snap(p.y, grid) };
}

/**
 * Delta for a group drag: the anchor (usually the grabbed element) lands on
 * the grid and every other element moves by the same amount, preserving the
 * relative layout of the group.
 */
export function snapDelta(anchor: Point, delta: Point, grid: number | null | undefined): Point {
  const target = snapPoint({ x: anchor.x + delta.x, y: anchor.y + delta.y }, grid);
  return { x: round6(target.x - anchor.x), y: round6(target.y - anchor.y) };
}

function round6(v: number): number {
  const r = Number(v.toFixed(6));
  return r === 0 ? 0 : r;
}

/** Normalises a rotation to [0, 360). */
export function normalizeRotation(deg: number): number {
  const r = ((deg % 360) + 360) % 360;
  return round6(r);
}

/** Axis-aligned bounding box of a (possibly rotated) placed element. */
export function elementBounds(el: Placed): Rect {
  const w = el.shape.w;
  const h = el.shape.type === 'circle' ? el.shape.w : el.shape.h;
  const rot = el.shape.type === 'circle' ? 0 : (el.rotation ?? 0);
  const rad = (rot * Math.PI) / 180;
  const cos = Math.abs(Math.cos(rad));
  const sin = Math.abs(Math.sin(rad));
  const bw = round6(w * cos + h * sin);
  const bh = round6(w * sin + h * cos);
  return { x: round6(el.x - bw / 2), y: round6(el.y - bh / 2), width: bw, height: bh };
}

/** Rectangle spanned by two corner points, in any order. */
export function rectFromPoints(a: Point, b: Point): Rect {
  return {
    x: Math.min(a.x, b.x),
    y: Math.min(a.y, b.y),
    width: Math.abs(a.x - b.x),
    height: Math.abs(a.y - b.y),
  };
}

export function rectsIntersect(a: Rect, b: Rect): boolean {
  return (
    a.x <= b.x + b.width && b.x <= a.x + a.width && a.y <= b.y + b.height && b.y <= a.y + a.height
  );
}

/** Union of rectangles, or null for an empty list. */
export function unionRect(rects: readonly Rect[]): Rect | null {
  if (rects.length === 0) return null;
  let minX = Infinity;
  let minY = Infinity;
  let maxX = -Infinity;
  let maxY = -Infinity;
  for (const r of rects) {
    minX = Math.min(minX, r.x);
    minY = Math.min(minY, r.y);
    maxX = Math.max(maxX, r.x + r.width);
    maxY = Math.max(maxY, r.y + r.height);
  }
  return { x: minX, y: minY, width: maxX - minX, height: maxY - minY };
}

export type AlignMode = 'left' | 'right' | 'top' | 'bottom' | 'center-x' | 'center-y';

/**
 * New centres that align the elements' bounding boxes on the selection's
 * bounds. Returned in the input order.
 */
export function alignPositions(items: readonly Placed[], mode: AlignMode): Point[] {
  const bounds = items.map(elementBounds);
  const all = unionRect(bounds);
  if (!all) return [];
  return items.map((it, i) => {
    const b = bounds[i] as Rect;
    switch (mode) {
      case 'left':
        return { x: round6(it.x + (all.x - b.x)), y: it.y };
      case 'right':
        return { x: round6(it.x + (all.x + all.width - (b.x + b.width))), y: it.y };
      case 'top':
        return { x: it.x, y: round6(it.y + (all.y - b.y)) };
      case 'bottom':
        return { x: it.x, y: round6(it.y + (all.y + all.height - (b.y + b.height))) };
      case 'center-x':
        return { x: round6(all.x + all.width / 2), y: it.y };
      case 'center-y':
        return { x: it.x, y: round6(all.y + all.height / 2) };
    }
  });
}

/**
 * Distributes centres evenly between the two outermost elements along `axis`
 * (needs at least three elements; otherwise positions are unchanged).
 * Returned in the input order.
 */
export function distributePositions(items: readonly Placed[], axis: 'x' | 'y'): Point[] {
  const out = items.map((it) => ({ x: it.x, y: it.y }));
  if (items.length < 3) return out;
  const order = items
    .map((_, i) => i)
    .sort((a, b) => (items[a] as Placed)[axis] - (items[b] as Placed)[axis]);
  const first = (items[order[0] as number] as Placed)[axis];
  const last = (items[order[order.length - 1] as number] as Placed)[axis];
  const step = (last - first) / (order.length - 1);
  order.forEach((idx, k) => {
    (out[idx] as Point)[axis] = round6(first + step * k);
  });
  return out;
}

/** Keeps a centre inside the map rectangle [0, width] x [0, height]. */
export function clampToMap(p: Point, width: number, height: number): Point {
  return { x: Math.min(width, Math.max(0, p.x)), y: Math.min(height, Math.max(0, p.y)) };
}

/** True when `inner` lies entirely inside `outer`. */
export function rectContains(outer: Rect, inner: Rect): boolean {
  return (
    inner.x >= outer.x &&
    inner.y >= outer.y &&
    inner.x + inner.width <= outer.x + outer.width &&
    inner.y + inner.height <= outer.y + outer.height
  );
}

/**
 * Indices of the items hit by a rubber-band rectangle: those whose bounding
 * box intersects it (`intersect`, the default) or lies fully inside it
 * (`contain`).
 */
export function itemsInRect(
  items: readonly Placed[],
  rect: Rect,
  mode: 'intersect' | 'contain' = 'intersect',
): number[] {
  const out: number[] = [];
  items.forEach((it, i) => {
    const b = elementBounds(it);
    if (mode === 'contain' ? rectContains(rect, b) : rectsIntersect(rect, b)) out.push(i);
  });
  return out;
}

/**
 * Axis along which a row of spots runs. Rows are parallel to the shoreline:
 * horizontal when the sea is at the top or bottom, vertical otherwise.
 */
export function rowAxis(seaSide: SeaSide): 'x' | 'y' {
  return seaSide === 'left' || seaSide === 'right' ? 'y' : 'x';
}
