import type { MapSpot } from './types';

/**
 * Multi-selection of spots on a map (operational map: group booking and
 * blocks, ADR 0019 C). Pure functions: the map reports gestures, the caller
 * keeps the selection and decides which spots can be selected.
 *
 * - plain activation (tap, click, Enter, Space) toggles one spot;
 * - Shift + activation selects every selectable spot between the anchor
 *   (last spot toggled) and this one, in map order (rows as drawn);
 * - Ctrl/Cmd + activation toggles one spot (same as plain in selection mode);
 * - a lasso (drag a rectangle) selects the spots whose centre lies inside
 *   it; with Shift it adds them to the selection, otherwise it replaces it.
 */

/** Keys held while a spot is activated or a lasso is drawn. */
export interface SpotActivateModifiers {
  /** Shift: range (activation) or add (lasso). */
  shiftKey?: boolean;
  /** Ctrl or Cmd: toggle one spot. */
  toggleKey?: boolean;
}

export interface Selection {
  ids: readonly string[];
  /** Last spot toggled: start of the next Shift range. */
  anchor: string | null;
}

export const EMPTY_SELECTION: Selection = { ids: [], anchor: null };

/** Rectangle in map coordinates (any two opposite corners). */
export interface MapRect {
  x0: number;
  y0: number;
  x1: number;
  y1: number;
}

type Selectable = (id: string) => boolean;

const always: Selectable = () => true;

/** Adds `add` to `ids` keeping order and uniqueness. */
function union(ids: readonly string[], add: readonly string[]): string[] {
  const seen = new Set(ids);
  const out = [...ids];
  for (const id of add) {
    if (!seen.has(id)) {
      seen.add(id);
      out.push(id);
    }
  }
  return out;
}

/** Toggles `id` in `ids`. */
export function toggleId(ids: readonly string[], id: string): string[] {
  return ids.includes(id) ? ids.filter((x) => x !== id) : [...ids, id];
}

/**
 * Selectable ids between `anchor` and `id` (both included) in `order`. An
 * anchor missing from `order` makes the range the single `id`.
 */
export function rangeIds(
  order: readonly string[],
  anchor: string | null,
  id: string,
  selectable: Selectable = always,
): string[] {
  const to = order.indexOf(id);
  if (to < 0) return [];
  const from = anchor ? order.indexOf(anchor) : -1;
  if (from < 0) return selectable(id) ? [id] : [];
  const [lo, hi] = from <= to ? [from, to] : [to, from];
  return order.slice(lo, hi + 1).filter(selectable);
}

/** Next selection after activating `id` with `mods` (see the module comment). */
export function selectSpot(
  current: Selection,
  id: string,
  mods: SpotActivateModifiers | undefined,
  order: readonly string[],
  selectable: Selectable = always,
): Selection {
  if (!selectable(id)) return current;
  if (mods?.shiftKey && current.anchor) {
    return { ids: union(current.ids, rangeIds(order, current.anchor, id, selectable)), anchor: id };
  }
  return { ids: toggleId(current.ids, id), anchor: id };
}

/** Active, selectable spots whose centre lies inside `rect`, in map order. */
export function spotsInRect(
  spots: readonly MapSpot[],
  rect: MapRect,
  selectable: Selectable = always,
): string[] {
  const minX = Math.min(rect.x0, rect.x1);
  const maxX = Math.max(rect.x0, rect.x1);
  const minY = Math.min(rect.y0, rect.y1);
  const maxY = Math.max(rect.y0, rect.y1);
  return spots
    .filter(
      (s) =>
        s.active !== false &&
        s.x >= minX &&
        s.x <= maxX &&
        s.y >= minY &&
        s.y <= maxY &&
        selectable(s.id),
    )
    .map((s) => s.id);
}

/** Selection after a lasso over `ids`: replaces it, or adds to it with Shift. */
export function selectLasso(
  current: Selection,
  ids: readonly string[],
  mods: SpotActivateModifiers | undefined,
): Selection {
  const next = mods?.shiftKey ? union(current.ids, ids) : [...ids];
  return { ids: next, anchor: ids.length > 0 ? (ids[ids.length - 1] ?? null) : current.anchor };
}

/** Drops ids that are no longer selectable (the day changed, a spot got booked). */
export function pruneSelection(current: Selection, selectable: Selectable): Selection {
  const ids = current.ids.filter(selectable);
  if (ids.length === current.ids.length) return current;
  return { ids, anchor: current.anchor && selectable(current.anchor) ? current.anchor : null };
}
