import { STATUS_CLASSES } from '@arenile/shared/map';
import { cn } from '@arenile/shared/lib/utils';
import {
  memo,
  useCallback,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent,
  type PointerEvent,
} from 'react';
import { useTranslation } from 'react-i18next';
import { entryStatus } from '../lib/colors';
import {
  barAt,
  barRect,
  bodySize,
  isMovable,
  proposeMove,
  selectionIsFree,
  selectionToDefaults,
  shiftSpotRow,
  visibleRows,
  type CellSelection,
  type GridBar,
  type GridLayout,
  type GridMetrics,
  type MoveProposal,
  type SelectionDefaults,
} from '../lib/grid';

export interface PlanningGridProps {
  layout: GridLayout;
  metrics: GridMetrics;
  today: string;
  canMove: boolean;
  canCreate: boolean;
  onMove: (proposal: MoveProposal) => void;
  onSelect: (defaults: SelectionDefaults) => void;
  onOpenBooking: (bookingId: string) => void;
  /** Viewport height used before the scroller can be measured (tests, first paint). */
  fallbackViewport?: number;
  className?: string;
}

type Drag =
  | { kind: 'bar'; bar: GridBar; startX: number; startY: number; moved: boolean }
  | { kind: 'select'; sel: CellSelection };

const DRAG_THRESHOLD = 4;

function weekdayFormatter(locale: string) {
  return new Intl.DateTimeFormat(locale, { weekday: 'narrow', timeZone: 'UTC' });
}
function monthFormatter(locale: string) {
  return new Intl.DateTimeFormat(locale, { month: 'short', timeZone: 'UTC' });
}
function longFormatter(locale: string) {
  return new Intl.DateTimeFormat(locale, {
    weekday: 'long',
    day: 'numeric',
    month: 'long',
    timeZone: 'UTC',
  });
}
const toDate = (iso: string) => new Date(`${iso}T00:00:00Z`);

interface BarProps {
  bar: GridBar;
  metrics: GridMetrics;
  movable: boolean;
  dragging: boolean;
}

/** One occupation bar (SVG geometry from barRect; colours from the theme tokens). */
const Bar = memo(function Bar({ bar, metrics, movable, dragging }: BarProps) {
  const r = barRect(bar, metrics);
  const status = entryStatus(bar.entry);
  const cls = STATUS_CLASSES[status];
  const text = bar.entry.code ? `${bar.entry.code} ${bar.entry.label}` : bar.entry.label;
  const fontSize = Math.min(12, r.height * 0.6);
  return (
    <g
      data-bar-key={bar.key}
      data-status={status}
      data-entry-kind={bar.entry.kind}
      className={cn(movable ? 'cursor-grab' : 'cursor-pointer', dragging && 'opacity-40')}
    >
      <title>{text}</title>
      <rect
        x={r.x}
        y={r.y}
        width={r.width}
        height={r.height}
        rx={4}
        className={cn(cls.fill, 'stroke-map-spot-stroke')}
        strokeWidth={1}
      />
      {r.width >= 28 && fontSize >= 8 && (
        <svg x={r.x} y={r.y} width={r.width} height={r.height} overflow="hidden">
          <text
            x={6}
            y={r.height / 2}
            dominantBaseline="central"
            fontSize={fontSize}
            fontWeight={600}
            className={cls.labelFill}
            pointerEvents="none"
          >
            {text}
          </text>
        </svg>
      )}
    </g>
  );
});

/**
 * Spot × day grid of the planning (spec 15.1), drawn in SVG so every
 * position is geometry from the layout math (no inline styles). Rows are
 * windowed to the viewport (hundreds of spots stay fluid); the header and
 * the label column stay pinned by following the scroll offset.
 *
 * Pointer: drag a booking bar to shift its dates (horizontal) and/or move
 * it to another spot (vertical) → `onMove`; click a bar → `onOpenBooking`;
 * drag over empty cells → `onSelect` (prefill of the new booking).
 *
 * Keyboard (the grid is one tab stop): arrows move the cell cursor,
 * Shift+arrows extend a selection, Enter opens the bar under the cursor or
 * creates a booking from the selection, M starts moving the bar under the
 * cursor (arrows, then Enter to confirm), Escape cancels. The cursor cell is
 * announced in a live region.
 */
export function PlanningGrid({
  layout,
  metrics: m,
  today,
  canMove,
  canCreate,
  onMove,
  onSelect,
  onOpenBooking,
  fallbackViewport = 600,
  className,
}: PlanningGridProps) {
  const { t, i18n } = useTranslation('planning');
  const scroller = useRef<HTMLDivElement>(null);
  const svgRef = useRef<SVGSVGElement>(null);
  const hintId = useId();
  const [scroll, setScroll] = useState({ top: 0, left: 0, height: 0 });
  const [drag, setDrag] = useState<Drag | null>(null);
  const [preview, setPreview] = useState<MoveProposal | null>(null);
  const [cursor, setCursor] = useState<{ row: number; day: number } | null>(null);
  const [anchor, setAnchor] = useState<{ row: number; day: number } | null>(null);
  const [kbdMove, setKbdMove] = useState<{ bar: GridBar; dx: number; dy: number } | null>(null);
  const [focused, setFocused] = useState(false);

  const body = bodySize(layout, m);
  const width = m.labelWidth + body.width;
  const height = m.headerHeight + body.height;
  const viewport = scroll.height || fallbackViewport;
  const win = visibleRows(Math.max(0, scroll.top), viewport, m.rowHeight, layout.rows.length);
  const todayIndex = layout.days.indexOf(today);

  const fmt = useMemo(
    () => ({
      weekday: weekdayFormatter(i18n.language),
      month: monthFormatter(i18n.language),
      long: longFormatter(i18n.language),
    }),
    [i18n.language],
  );

  // rAF-throttled scroll tracking (pinned header/labels, row window).
  const frame = useRef<number | null>(null);
  const onScroll = useCallback(() => {
    if (frame.current !== null) return;
    frame.current = requestAnimationFrame(() => {
      frame.current = null;
      const el = scroller.current;
      if (!el) return;
      const next = { top: el.scrollTop, left: el.scrollLeft, height: el.clientHeight };
      setScroll((prev) =>
        prev.top === next.top && prev.left === next.left && prev.height === next.height
          ? prev
          : next,
      );
    });
  }, []);
  useEffect(() => {
    onScroll();
    return () => {
      if (frame.current !== null) cancelAnimationFrame(frame.current);
    };
  }, [onScroll]);

  /** Body coordinates of a pointer event; null over the pinned header or labels. */
  const bodyPoint = (e: PointerEvent<SVGSVGElement>) => {
    const rect = svgRef.current?.getBoundingClientRect();
    if (!rect) return null;
    const sx = e.clientX - rect.left;
    const sy = e.clientY - rect.top;
    if (sx < scroll.left + m.labelWidth || sy < scroll.top + m.headerHeight) return null;
    return { x: sx - m.labelWidth, y: sy - m.headerHeight };
  };
  const cellOf = (p: { x: number; y: number }) => {
    const day = Math.min(layout.days.length - 1, Math.max(0, Math.floor(p.x / m.cellWidth)));
    const row = Math.min(layout.rows.length - 1, Math.max(0, Math.floor(p.y / m.rowHeight)));
    return { row, day };
  };

  const onPointerDown = (e: PointerEvent<SVGSVGElement>) => {
    if (e.button !== 0) return;
    const p = bodyPoint(e);
    if (!p) return;
    const c = cellOf(p);
    const barEl = (e.target as Element).closest?.('[data-bar-key]');
    const bar = barEl
      ? layout.bars.find((b) => b.key === barEl.getAttribute('data-bar-key'))
      : undefined;
    svgRef.current?.setPointerCapture?.(e.pointerId);
    if (bar) {
      setDrag({ kind: 'bar', bar, startX: e.clientX, startY: e.clientY, moved: false });
      return;
    }
    if (layout.rows[c.row]?.kind !== 'spot' || !canCreate) return;
    setDrag({ kind: 'select', sel: { rowA: c.row, rowB: c.row, dayA: c.day, dayB: c.day } });
  };

  const onPointerMove = (e: PointerEvent<SVGSVGElement>) => {
    if (!drag) return;
    if (drag.kind === 'bar') {
      const dx = e.clientX - drag.startX;
      const dy = e.clientY - drag.startY;
      const moved = drag.moved || Math.abs(dx) > DRAG_THRESHOLD || Math.abs(dy) > DRAG_THRESHOLD;
      if (moved !== drag.moved) setDrag({ ...drag, moved });
      if (moved && canMove && isMovable(drag.bar.entry)) {
        setPreview(proposeMove(layout, drag.bar, dx, dy, m));
      }
      return;
    }
    const p = bodyPoint(e);
    if (!p) return;
    const c = cellOf(p);
    if (c.row !== drag.sel.rowB || c.day !== drag.sel.dayB) {
      setDrag({ kind: 'select', sel: { ...drag.sel, rowB: c.row, dayB: c.day } });
    }
  };

  const finishSelection = (sel: CellSelection) => {
    if (!canCreate || !selectionIsFree(layout, sel)) return false;
    const defaults = selectionToDefaults(layout, sel);
    if (defaults) onSelect(defaults);
    return !!defaults;
  };

  const onPointerUp = (e: PointerEvent<SVGSVGElement>) => {
    svgRef.current?.releasePointerCapture?.(e.pointerId);
    const d = drag;
    setDrag(null);
    if (!d) return;
    if (d.kind === 'bar') {
      const p = preview;
      setPreview(null);
      if (d.moved && p) onMove(p);
      else if (!d.moved && d.bar.entry.booking_id) onOpenBooking(d.bar.entry.booking_id);
      return;
    }
    finishSelection(d.sel);
  };

  const onPointerCancel = () => {
    setDrag(null);
    setPreview(null);
  };

  // ---- keyboard ---------------------------------------------------------
  const firstSpotRow = layout.rows.findIndex((r) => r.kind === 'spot');
  const activeCursor = cursor ?? (firstSpotRow >= 0 ? { row: firstSpotRow, day: 0 } : null);

  const ensureVisible = (row: number, day: number) => {
    const el = scroller.current;
    if (!el) return;
    const y = m.headerHeight + row * m.rowHeight;
    const x = m.labelWidth + day * m.cellWidth;
    if (y - m.headerHeight < el.scrollTop) el.scrollTop = y - m.headerHeight;
    else if (y + m.rowHeight > el.scrollTop + el.clientHeight)
      el.scrollTop = y + m.rowHeight - el.clientHeight;
    if (x - m.labelWidth < el.scrollLeft) el.scrollLeft = x - m.labelWidth;
    else if (x + m.cellWidth > el.scrollLeft + el.clientWidth)
      el.scrollLeft = x + m.cellWidth - el.clientWidth;
  };

  const onKeyDown = (e: KeyboardEvent<SVGSVGElement>) => {
    const c = activeCursor;
    if (!c) return;
    const arrows: Record<string, [number, number]> = {
      ArrowLeft: [0, -1],
      ArrowRight: [0, 1],
      ArrowUp: [-1, 0],
      ArrowDown: [1, 0],
    };
    const dir = arrows[e.key];
    if (kbdMove) {
      if (dir) {
        e.preventDefault();
        const next = { ...kbdMove, dx: kbdMove.dx + dir[1], dy: kbdMove.dy + dir[0] };
        setKbdMove(next);
        setPreview(
          proposeMove(layout, kbdMove.bar, next.dx * m.cellWidth, next.dy * m.rowHeight, m),
        );
      } else if (e.key === 'Enter') {
        e.preventDefault();
        if (preview) onMove(preview);
        setKbdMove(null);
        setPreview(null);
      } else if (e.key === 'Escape') {
        e.preventDefault();
        setKbdMove(null);
        setPreview(null);
      }
      return;
    }
    if (dir) {
      e.preventDefault();
      const row = dir[0] === 0 ? c.row : shiftSpotRow(layout.rows, c.row, dir[0]);
      const day = Math.min(layout.days.length - 1, Math.max(0, c.day + dir[1]));
      if (e.shiftKey) setAnchor((a) => a ?? c);
      else setAnchor(null);
      setCursor({ row, day });
      ensureVisible(row, day);
      return;
    }
    if (e.key === 'Escape') {
      setAnchor(null);
      return;
    }
    const bar = barAt(layout, c.row, c.day);
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      if (anchor) {
        if (finishSelection({ rowA: anchor.row, rowB: c.row, dayA: anchor.day, dayB: c.day }))
          setAnchor(null);
      } else if (bar?.entry.booking_id) {
        onOpenBooking(bar.entry.booking_id);
      } else if (!bar) {
        finishSelection({ rowA: c.row, rowB: c.row, dayA: c.day, dayB: c.day });
      }
      return;
    }
    if ((e.key === 'm' || e.key === 'M') && bar && canMove && isMovable(bar.entry)) {
      e.preventDefault();
      setKbdMove({ bar, dx: 0, dy: 0 });
    }
  };

  // ---- derived drawing state ---------------------------------------------
  const selection: CellSelection | null =
    drag?.kind === 'select'
      ? drag.sel
      : anchor && activeCursor
        ? { rowA: anchor.row, rowB: activeCursor.row, dayA: anchor.day, dayB: activeCursor.day }
        : null;
  const draggingKey = drag?.kind === 'bar' && drag.moved ? drag.bar.key : kbdMove?.bar.key;

  const visibleBars = useMemo(() => {
    const out: GridBar[] = [];
    for (let r = win.start; r <= win.end; r++) out.push(...(layout.barsByRow.get(r) ?? []));
    return out;
  }, [layout, win.start, win.end]);

  const announcement = (() => {
    if (!focused || !activeCursor) return '';
    const row = layout.rows[activeCursor.row];
    const day = layout.days[activeCursor.day];
    if (row?.kind !== 'spot' || !day) return '';
    if (kbdMove) {
      return preview
        ? t('grid.movePreview', {
            days: preview.dayDelta,
            spot:
              layout.rows[preview.targetRow]?.kind === 'spot'
                ? (layout.rows[preview.targetRow] as { spot: { label: string } }).spot.label
                : '',
          })
        : t('grid.moveStart');
    }
    const bar = barAt(layout, activeCursor.row, activeCursor.day);
    const what = bar
      ? [bar.entry.code, bar.entry.label].filter(Boolean).join(' ')
      : layout.closed.has(activeCursor.day)
        ? t('grid.closed')
        : t('grid.free');
    return t('grid.cell', {
      spot: row.spot.label,
      day: fmt.long.format(toDate(day)),
      what,
    });
  })();

  const rowsToDraw = layout.rows.slice(win.start, win.end + 1);

  return (
    <div className={cn('relative', className)}>
      <div
        ref={scroller}
        onScroll={onScroll}
        className="h-[70vh] overflow-auto rounded-lg border bg-background"
        data-testid="planning-scroller"
      >
        <svg
          ref={svgRef}
          width={width}
          height={height}
          viewBox={`0 0 ${width} ${height}`}
          role="application"
          aria-roledescription={t('grid.roledescription')}
          aria-label={t('grid.label', {
            count: layout.rows.filter((r) => r.kind === 'spot').length,
          })}
          aria-describedby={hintId}
          tabIndex={0}
          className="block touch-none outline-none select-none focus-visible:ring-2 focus-visible:ring-ring"
          onPointerDown={onPointerDown}
          onPointerMove={onPointerMove}
          onPointerUp={onPointerUp}
          onPointerCancel={onPointerCancel}
          onKeyDown={onKeyDown}
          onFocus={() => setFocused(true)}
          onBlur={() => {
            setFocused(false);
            setKbdMove(null);
          }}
          data-testid="planning-grid"
        >
          {/* Body: columns, rows, bars. */}
          <g transform={`translate(${m.labelWidth} ${m.headerHeight})`}>
            {layout.days.map((d, i) =>
              layout.closed.has(i) || i === todayIndex ? (
                <rect
                  key={d}
                  x={i * m.cellWidth}
                  y={0}
                  width={m.cellWidth}
                  height={body.height}
                  className={layout.closed.has(i) ? 'fill-muted' : 'fill-primary/10'}
                  data-closed-day={layout.closed.has(i) ? d : undefined}
                />
              ) : null,
            )}
            {rowsToDraw.map((row, k) => {
              const r = win.start + k;
              return row.kind === 'spot' ? (
                <line
                  key={row.key}
                  x1={0}
                  x2={body.width}
                  y1={(r + 1) * m.rowHeight}
                  y2={(r + 1) * m.rowHeight}
                  className="stroke-border"
                />
              ) : (
                <rect
                  key={row.key}
                  x={0}
                  y={r * m.rowHeight}
                  width={body.width}
                  height={m.rowHeight}
                  className={row.kind === 'zone' ? 'fill-muted' : 'fill-muted/50'}
                />
              );
            })}
            {layout.days.map((d, i) => (
              <line
                key={`v-${d}`}
                x1={(i + 1) * m.cellWidth}
                x2={(i + 1) * m.cellWidth}
                y1={0}
                y2={body.height}
                className="stroke-border/60"
              />
            ))}
            {selection && (
              <rect
                x={Math.min(selection.dayA, selection.dayB) * m.cellWidth}
                y={Math.min(selection.rowA, selection.rowB) * m.rowHeight}
                width={(Math.abs(selection.dayB - selection.dayA) + 1) * m.cellWidth}
                height={(Math.abs(selection.rowB - selection.rowA) + 1) * m.rowHeight}
                className="fill-primary/20 stroke-primary"
                strokeWidth={1.5}
                data-testid="planning-selection"
              />
            )}
            {visibleBars.map((bar) => (
              <Bar
                key={bar.key}
                bar={bar}
                metrics={m}
                movable={canMove && isMovable(bar.entry)}
                dragging={bar.key === draggingKey}
              />
            ))}
            {preview && (
              <rect
                {...barRect(
                  {
                    row: preview.targetRow,
                    from: preview.bar.from + preview.dayDelta,
                    to: preview.bar.to + preview.dayDelta,
                    slots: preview.bar.slots,
                  },
                  m,
                )}
                rx={4}
                className="fill-primary/25 stroke-primary"
                strokeWidth={2}
                strokeDasharray="4 3"
                pointerEvents="none"
                data-testid="planning-move-preview"
              />
            )}
            {focused && activeCursor && (
              <rect
                x={activeCursor.day * m.cellWidth + 1}
                y={activeCursor.row * m.rowHeight + 1}
                width={m.cellWidth - 2}
                height={m.rowHeight - 2}
                className="fill-none stroke-ring"
                strokeWidth={2}
                pointerEvents="none"
              />
            )}
          </g>

          {/* Pinned day header. */}
          <g transform={`translate(${m.labelWidth} ${scroll.top})`} aria-hidden="true">
            <rect
              x={0}
              y={0}
              width={body.width}
              height={m.headerHeight}
              className="fill-background"
            />
            {layout.days.map((d, i) => {
              const date = toDate(d);
              const first = i === 0 || d.endsWith('-01');
              return (
                <g key={d} data-day={d}>
                  {first && (
                    <text
                      x={i * m.cellWidth + 4}
                      y={11}
                      fontSize={10}
                      className="fill-muted-foreground uppercase"
                    >
                      {fmt.month.format(date)}
                    </text>
                  )}
                  <text
                    x={i * m.cellWidth + m.cellWidth / 2}
                    y={25}
                    textAnchor="middle"
                    fontSize={10}
                    className={layout.closed.has(i) ? 'fill-muted-foreground' : 'fill-foreground'}
                  >
                    {fmt.weekday.format(date)}
                  </text>
                  <text
                    x={i * m.cellWidth + m.cellWidth / 2}
                    y={38}
                    textAnchor="middle"
                    fontSize={12}
                    fontWeight={i === todayIndex ? 700 : 500}
                    className={i === todayIndex ? 'fill-primary' : 'fill-foreground'}
                  >
                    {date.getUTCDate()}
                  </text>
                </g>
              );
            })}
            <line
              x1={0}
              x2={body.width}
              y1={m.headerHeight - 0.5}
              y2={m.headerHeight - 0.5}
              className="stroke-border"
            />
          </g>

          {/* Pinned label column. */}
          <g transform={`translate(${scroll.left} ${m.headerHeight})`} aria-hidden="true">
            <rect
              x={0}
              y={0}
              width={m.labelWidth}
              height={body.height}
              className="fill-background"
            />
            {rowsToDraw.map((row, k) => {
              const y = (win.start + k) * m.rowHeight;
              return (
                <g key={row.key} data-row-kind={row.kind}>
                  {row.kind !== 'spot' && (
                    <rect
                      x={0}
                      y={y}
                      width={m.labelWidth}
                      height={m.rowHeight}
                      className={row.kind === 'zone' ? 'fill-muted' : 'fill-muted/50'}
                    />
                  )}
                  <text
                    x={row.kind === 'spot' ? 20 : 8}
                    y={y + m.rowHeight / 2}
                    dominantBaseline="central"
                    fontSize={row.kind === 'zone' ? 12 : 11}
                    fontWeight={row.kind === 'spot' ? 500 : 600}
                    className={row.kind === 'row' ? 'fill-muted-foreground' : 'fill-foreground'}
                  >
                    {row.kind === 'spot'
                      ? row.spot.label
                      : row.kind === 'row'
                        ? t('grid.row', { row: row.label })
                        : row.label || t('grid.noZone')}
                  </text>
                </g>
              );
            })}
            <line
              x1={m.labelWidth - 0.5}
              x2={m.labelWidth - 0.5}
              y1={0}
              y2={body.height}
              className="stroke-border"
            />
          </g>

          {/* Pinned corner. */}
          <g transform={`translate(${scroll.left} ${scroll.top})`} aria-hidden="true">
            <rect
              x={0}
              y={0}
              width={m.labelWidth}
              height={m.headerHeight}
              className="fill-background"
            />
            <text
              x={8}
              y={m.headerHeight / 2}
              dominantBaseline="central"
              fontSize={11}
              className="fill-muted-foreground"
            >
              {t('grid.corner')}
            </text>
          </g>
        </svg>
      </div>
      <p id={hintId} className="sr-only">
        {t('grid.keyboardHint')}
      </p>
      <p role="status" aria-live="polite" className="sr-only" data-testid="planning-announcer">
        {announcement}
      </p>
    </div>
  );
}
