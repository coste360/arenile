import type { Schemas } from '@arenile/shared/api';

/*
 * Layout math of the planning grid (spec 15.1 "Calendario"): pure functions
 * from the compact GET /planning payload to rows, bars and pixel rectangles.
 * The component only renders what these return, so the geometry (grouping,
 * bar placement, hit testing, drag targets, windowing) is unit-tested here.
 */

export type Planning = Schemas['Planning'];
export type PlanningEntry = Schemas['PlanningEntry'];
export type PlanningSpot = Schemas['PlanningSpot'];
export type SlotChoice = Schemas['Slots'];

export interface GridMetrics {
  /** Width of one day column, px. */
  cellWidth: number;
  /** Height of one row (spot or group header), px. */
  rowHeight: number;
  /** Width of the sticky label column, px. */
  labelWidth: number;
  /** Height of the sticky day header, px. */
  headerHeight: number;
}

export const DEFAULT_METRICS: GridMetrics = {
  cellWidth: 40,
  rowHeight: 32,
  labelWidth: 132,
  headerHeight: 44,
};

/** Widest day column when stretching the grid to its container, px. */
export const MAX_CELL_WIDTH = 96;

/**
 * Day column width that fills a container of `containerWidth` px with `days`
 * columns, never below `minCell` (horizontal scroll takes over) nor above
 * MAX_CELL_WIDTH. Unknown width (0, e.g. before layout) keeps `minCell`.
 */
export function fitCellWidth(
  containerWidth: number,
  days: number,
  labelWidth: number,
  minCell: number,
): number {
  if (containerWidth <= 0 || days <= 0) return minCell;
  // 2px: the scroller border.
  const fit = Math.floor((containerWidth - 2 - labelWidth) / days);
  return Math.max(minCell, Math.min(MAX_CELL_WIDTH, fit));
}

export type GridRow =
  | { kind: 'zone'; key: string; zoneId: string; label: string }
  | { kind: 'row'; key: string; zoneId: string; label: string }
  | { kind: 'spot'; key: string; spot: PlanningSpot; spotIndex: number };

export interface GridBar {
  key: string;
  /** Index in rows (a spot row). */
  row: number;
  /** Day indexes, inclusive. */
  from: number;
  to: number;
  slots: SlotChoice;
  entry: PlanningEntry;
  entryIndex: number;
}

export interface GridLayout {
  rows: GridRow[];
  bars: GridBar[];
  /** Row index of each spot id. */
  rowOfSpot: Map<string, number>;
  /** Bars per row index, for hit testing. */
  barsByRow: Map<number, GridBar[]>;
  days: string[];
  closed: Set<number>;
}

const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: 'base' });

/**
 * Rows grouped by zone (in `zoneOrder`, unknown zones last) and, inside a
 * zone, by row label; spots in natural label order. A row sub-header is
 * added only when the zone has more than one row label.
 */
export function buildLayout(
  planning: Planning,
  zoneNames: ReadonlyMap<string, string>,
  zoneOrder: readonly string[] = [...zoneNames.keys()],
): GridLayout {
  const byZone = new Map<string, { spot: PlanningSpot; index: number }[]>();
  planning.spots.forEach((spot, index) => {
    const list = byZone.get(spot.zone_id) ?? [];
    list.push({ spot, index });
    byZone.set(spot.zone_id, list);
  });
  const orderOf = (z: string) => {
    const i = zoneOrder.indexOf(z);
    return i < 0 ? Number.MAX_SAFE_INTEGER : i;
  };
  const zones = [...byZone.keys()].sort(
    (a, b) =>
      orderOf(a) - orderOf(b) || collator.compare(zoneNames.get(a) ?? a, zoneNames.get(b) ?? b),
  );

  const rows: GridRow[] = [];
  const rowOfSpot = new Map<string, number>();
  const rowOfIndex = new Map<number, number>();
  for (const zoneId of zones) {
    const list = byZone.get(zoneId) ?? [];
    rows.push({ kind: 'zone', key: `z:${zoneId}`, zoneId, label: zoneNames.get(zoneId) ?? '' });
    list.sort(
      (a, b) =>
        collator.compare(a.spot.row_label ?? '', b.spot.row_label ?? '') ||
        collator.compare(a.spot.label, b.spot.label),
    );
    const rowLabels = new Set(list.map((s) => s.spot.row_label ?? ''));
    let currentRow: string | undefined;
    for (const { spot, index } of list) {
      const rl = spot.row_label ?? '';
      if (rowLabels.size > 1 && rl !== currentRow) {
        currentRow = rl;
        rows.push({ kind: 'row', key: `r:${zoneId}:${rl}`, zoneId, label: rl });
      }
      rowOfSpot.set(spot.spot_id, rows.length);
      rowOfIndex.set(index, rows.length);
      rows.push({ kind: 'spot', key: `s:${spot.spot_id}`, spot, spotIndex: index });
    }
  }

  const bars: GridBar[] = [];
  const barsByRow = new Map<number, GridBar[]>();
  for (const pr of planning.rows) {
    const row = rowOfIndex.get(pr.spot);
    if (row === undefined) continue;
    for (const seg of pr.segments) {
      const entry = planning.entries[seg.entry];
      if (!entry) continue;
      const bar: GridBar = {
        key: `${row}:${seg.entry}:${seg.from}:${seg.slots}`,
        row,
        from: seg.from,
        to: seg.to,
        slots: seg.slots,
        entry,
        entryIndex: seg.entry,
      };
      bars.push(bar);
      const list = barsByRow.get(row) ?? [];
      list.push(bar);
      barsByRow.set(row, list);
    }
  }
  return {
    rows,
    bars,
    rowOfSpot,
    barsByRow,
    days: planning.days,
    closed: new Set(planning.closed_days),
  };
}

export interface Rect {
  x: number;
  y: number;
  width: number;
  height: number;
}

/** Rectangle of a bar inside the body (origin = first day, first row); AM top half, PM bottom. */
export function barRect(
  bar: Pick<GridBar, 'row' | 'from' | 'to' | 'slots'>,
  m: GridMetrics,
  inset = 3,
): Rect {
  const top = bar.row * m.rowHeight;
  const full = m.rowHeight - inset * 2;
  const half = full / 2;
  const y = bar.slots === 'PM' ? top + inset + half : top + inset;
  return {
    x: bar.from * m.cellWidth + 1,
    y,
    width: (bar.to - bar.from + 1) * m.cellWidth - 2,
    height: bar.slots === 'FULL' ? full : half,
  };
}

/** Size of the scrollable body. */
export function bodySize(layout: Pick<GridLayout, 'rows' | 'days'>, m: GridMetrics) {
  return { width: layout.days.length * m.cellWidth, height: layout.rows.length * m.rowHeight };
}

/** Rows to render for a scroll position (windowing), with overscan. */
export function visibleRows(
  scrollTop: number,
  viewportHeight: number,
  rowHeight: number,
  count: number,
  overscan = 8,
): { start: number; end: number } {
  if (count === 0) return { start: 0, end: -1 };
  const start = Math.max(0, Math.floor(scrollTop / rowHeight) - overscan);
  const end = Math.min(count - 1, Math.ceil((scrollTop + viewportHeight) / rowHeight) + overscan);
  return { start, end };
}

/** Cell under a body point, clamped to the grid; null outside. */
export function cellAt(
  x: number,
  y: number,
  m: GridMetrics,
  layout: Pick<GridLayout, 'rows' | 'days'>,
): { row: number; day: number } | null {
  if (x < 0 || y < 0) return null;
  const day = Math.floor(x / m.cellWidth);
  const row = Math.floor(y / m.rowHeight);
  if (day >= layout.days.length || row >= layout.rows.length) return null;
  return { row, day };
}

/** Bar covering a cell (FULL bars first; AM/PM by the half of the row when given). */
export function barAt(
  layout: Pick<GridLayout, 'barsByRow'>,
  row: number,
  day: number,
  half?: 'AM' | 'PM',
): GridBar | undefined {
  const list = layout.barsByRow.get(row) ?? [];
  const hits = list.filter((b) => b.from <= day && day <= b.to);
  if (hits.length <= 1) return hits[0];
  return hits.find((b) => b.slots === 'FULL') ?? hits.find((b) => b.slots === half) ?? hits[0];
}

/**
 * Nearest spot row from `row` moving by `delta` rows, skipping group
 * headers; stays on `row` when no spot row exists in that direction.
 */
export function shiftSpotRow(rows: readonly GridRow[], row: number, delta: number): number {
  if (delta === 0) return row;
  const step = delta > 0 ? 1 : -1;
  let remaining = Math.abs(delta);
  let r = row;
  let last = row;
  while (remaining > 0) {
    r += step;
    if (r < 0 || r >= rows.length) break;
    if (rows[r]?.kind === 'spot') {
      last = r;
      remaining -= 1;
    }
  }
  return last;
}

/** Proposed move of a bar: day shift and target spot (null spot = same spot). */
export interface MoveProposal {
  bar: GridBar;
  dayDelta: number;
  targetRow: number;
  /** Spot of the bar's row. */
  sourceSpotId: string;
  /** Spot of the target row. */
  targetSpotId: string;
}

/**
 * Move of `bar` by a pixel drag (dx, dy), snapped to whole days and spot
 * rows; null when it does not change anything or the bar is not a spot row.
 */
export function proposeMove(
  layout: Pick<GridLayout, 'rows'>,
  bar: GridBar,
  dx: number,
  dy: number,
  m: GridMetrics,
): MoveProposal | null {
  const source = layout.rows[bar.row];
  if (source?.kind !== 'spot') return null;
  // `|| 0` normalises -0 (small leftward drags).
  const dayDelta = Math.round(dx / m.cellWidth) || 0;
  const rowDelta = Math.round(dy / m.rowHeight) || 0;
  const targetRow = shiftSpotRow(layout.rows, bar.row, rowDelta);
  const target = layout.rows[targetRow];
  if (target?.kind !== 'spot') return null;
  if (dayDelta === 0 && targetRow === bar.row) return null;
  return {
    bar,
    dayDelta,
    targetRow,
    sourceSpotId: source.spot.spot_id,
    targetSpotId: target.spot.spot_id,
  };
}

/** Whether a bar can be dragged (a booking, not a season pass, hold, block or release). */
export function isMovable(entry: PlanningEntry): boolean {
  if (entry.kind !== 'booking' || !entry.booking_id) return false;
  return !['cancelled', 'expired', 'rejected', 'no_show', 'completed'].includes(entry.status ?? '');
}

/** Rectangular selection of empty cells (inclusive), normalised. */
export interface CellSelection {
  rowA: number;
  rowB: number;
  dayA: number;
  dayB: number;
}

export interface SelectionDefaults {
  spotIds: string[];
  startsOn: string;
  endsOn: string;
}

/** Prefill of the new-booking sheet from a selection: its spot rows and day range. */
export function selectionToDefaults(
  layout: Pick<GridLayout, 'rows' | 'days'>,
  sel: CellSelection,
): SelectionDefaults | null {
  const r0 = Math.min(sel.rowA, sel.rowB);
  const r1 = Math.max(sel.rowA, sel.rowB);
  const d0 = Math.min(sel.dayA, sel.dayB);
  const d1 = Math.max(sel.dayA, sel.dayB);
  const spotIds: string[] = [];
  for (let r = r0; r <= r1; r++) {
    const row = layout.rows[r];
    if (row?.kind === 'spot') spotIds.push(row.spot.spot_id);
  }
  const startsOn = layout.days[d0];
  const endsOn = layout.days[d1];
  if (spotIds.length === 0 || !startsOn || !endsOn) return null;
  return { spotIds, startsOn, endsOn };
}

/** True when no bar covers any cell of the selection. */
export function selectionIsFree(
  layout: Pick<GridLayout, 'barsByRow'>,
  sel: CellSelection,
): boolean {
  const r0 = Math.min(sel.rowA, sel.rowB);
  const r1 = Math.max(sel.rowA, sel.rowB);
  const d0 = Math.min(sel.dayA, sel.dayB);
  const d1 = Math.max(sel.dayA, sel.dayB);
  for (let r = r0; r <= r1; r++) {
    for (const b of layout.barsByRow.get(r) ?? []) {
      if (b.from <= d1 && d0 <= b.to) return false;
    }
  }
  return true;
}
