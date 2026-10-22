import {
  DECORATION_COLOR,
  SAND_COLOR,
  SEA_COLOR,
  ShapeElement,
  computeViewBox,
  rectFromPoints,
  seaRect,
  shapeSize,
  snapDelta,
  type Point,
} from '@arenile/shared/map';
import { memo, useId, useLayoutEffect, useRef, type KeyboardEvent, type PointerEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { findElement, selectInRect } from './model/editor';
import type { DraftDecoration, DraftDocument, DraftSpot, Selection } from './model/types';

export interface MapCanvasProps {
  doc: DraftDocument;
  selection: Selection;
  /** Colour per zone id. */
  zoneColor: (zoneId: string) => string;
  /** Accessible description of a spot (label, zone, state). */
  spotLabel: (spot: DraftSpot) => string;
  /** Visible name of a decoration (its label or kind). */
  decorationLabel: (d: DraftDecoration) => string;
  readOnly?: boolean;
  /** Click / Enter / Space on an element; `additive` with Shift, Ctrl or Cmd. */
  onSelect: (key: string, additive: boolean) => void;
  /** Rubber-band selection result (already merged with the selection when additive). */
  onSelectKeys: (keys: Set<string>) => void;
  /** Click on the empty sand. */
  onClearSelection: () => void;
  /**
   * End of a drag of `keys` by `delta` (already snapped so that `anchorKey`
   * lands on the grid): one undo step.
   */
  onMove: (keys: Selection, delta: Point, anchorKey: string) => void;
  /** Keyboard nudge (arrows) of the selection, or of `key` when it is not selected. */
  onNudge: (delta: Point, key: string) => void;
  /** Delete key: removes the selection, or `key` when it is not selected. */
  onDelete: (key: string) => void;
}

interface DragState {
  kind: 'drag';
  pointerId: number;
  anchorKey: string;
  anchor: Point;
  keys: Selection;
  nodes: SVGGElement[];
  start: Point;
  delta: Point;
  moved: boolean;
  frame: number;
}

interface BandState {
  kind: 'band';
  pointerId: number;
  start: Point;
  end: Point;
  additive: boolean;
}

/** Distance in map units the pointer must travel before a press becomes a drag. */
const DRAG_THRESHOLD_UNITS = 2;

function toSvgPoint(svg: SVGSVGElement | null, clientX: number, clientY: number): Point | null {
  if (!svg || typeof svg.getScreenCTM !== 'function' || typeof svg.createSVGPoint !== 'function') {
    return null;
  }
  const ctm = svg.getScreenCTM();
  if (!ctm) return null;
  const pt = svg.createSVGPoint();
  pt.x = clientX;
  pt.y = clientY;
  const p = pt.matrixTransform(ctm.inverse());
  return { x: p.x, y: p.y };
}

function keyOf(target: EventTarget | null): string | null {
  const el = (target as Element | null)?.closest?.('[data-key]');
  return el?.getAttribute('data-key') ?? null;
}

/**
 * Editable SVG drawing of the map: sea band, sand, grid, decorations and
 * spots. Elements are buttons with a roving tabindex: the canvas is one tab
 * stop (the first selected element, else the first spot), Home/End and
 * PageUp/PageDown move the focus and the selection between elements (arrows
 * too when read-only), Tab leaves the canvas. When editable, arrows move the
 * selection by one grid step (Shift: five steps, Alt: one unit) and Delete
 * removes it.
 *
 * Performance with ~1000 elements: events are delegated to the <svg> (so the
 * memoised elements get stable props), and a drag only sets a `translate`
 * on the dragged nodes, without React state; the document is updated once,
 * on release.
 */
export function MapCanvas(props: MapCanvasProps) {
  const { doc, selection, readOnly } = props;
  const { t } = useTranslation('mapEditor');
  const svgRef = useRef<SVGSVGElement>(null);
  const bandRef = useRef<SVGRectElement>(null);
  const gesture = useRef<DragState | BandState | null>(null);
  // Nodes still translated after a drop: cleared once React has drawn the new positions.
  const translated = useRef<SVGGElement[]>([]);
  const gridId = useId();
  // Roving tabindex: one focusable element in the canvas.
  const order = [...doc.spots.map((s) => s.key), ...doc.decorations.map((d) => d.key)];
  const activeKey = order.find((k) => selection.has(k)) ?? order[0];
  const vb = computeViewBox(doc.map);
  const sea = seaRect(doc.map);
  const grid = doc.map.grid_size > 0 ? doc.map.grid_size : 0;

  useLayoutEffect(() => {
    for (const n of translated.current) n.removeAttribute('transform');
    translated.current = [];
  }, [doc]);

  const nodesFor = (keys: Selection): SVGGElement[] => {
    const svg = svgRef.current;
    if (!svg) return [];
    const out: SVGGElement[] = [];
    svg.querySelectorAll<SVGGElement>('g[data-key]').forEach((n) => {
      if (keys.has(n.dataset.key ?? '')) out.push(n);
    });
    return out;
  };

  const drawBand = (b: BandState | null) => {
    const r = bandRef.current;
    if (!r) return;
    if (!b) {
      r.setAttribute('visibility', 'hidden');
      return;
    }
    const rect = rectFromPoints(b.start, b.end);
    r.setAttribute('x', String(rect.x));
    r.setAttribute('y', String(rect.y));
    r.setAttribute('width', String(rect.width));
    r.setAttribute('height', String(rect.height));
    r.setAttribute('visibility', 'visible');
  };

  const onPointerDown = (e: PointerEvent<SVGSVGElement>) => {
    if (e.button !== 0) return;
    const additive = e.shiftKey || e.ctrlKey || e.metaKey;
    const key = keyOf(e.target);
    const p = toSvgPoint(svgRef.current, e.clientX, e.clientY);
    if (!key) {
      // Empty sand or sea: rubber band (or clear on a simple click).
      if (!p) {
        if (!additive) props.onClearSelection();
        return;
      }
      gesture.current = { kind: 'band', pointerId: e.pointerId, start: p, end: p, additive };
      e.currentTarget.setPointerCapture?.(e.pointerId);
      return;
    }
    const keys: Selection = additive ? selection : selection.has(key) ? selection : new Set([key]);
    if (!selection.has(key) || additive) props.onSelect(key, additive);
    if (readOnly || additive || !p) return;
    const anchor = findElement(doc, key);
    if (!anchor) return;
    gesture.current = {
      kind: 'drag',
      pointerId: e.pointerId,
      anchorKey: key,
      anchor: { x: anchor.x, y: anchor.y },
      keys,
      nodes: [],
      start: p,
      delta: { x: 0, y: 0 },
      moved: false,
      frame: 0,
    };
    e.currentTarget.setPointerCapture?.(e.pointerId);
  };

  const onPointerMove = (e: PointerEvent<SVGSVGElement>) => {
    const g = gesture.current;
    if (!g || g.pointerId !== e.pointerId) return;
    const p = toSvgPoint(svgRef.current, e.clientX, e.clientY);
    if (!p) return;
    if (g.kind === 'band') {
      g.end = p;
      drawBand(g);
      return;
    }
    const raw = { x: p.x - g.start.x, y: p.y - g.start.y };
    if (!g.moved && Math.hypot(raw.x, raw.y) < DRAG_THRESHOLD_UNITS) return;
    if (!g.moved) {
      g.moved = true;
      g.nodes = nodesFor(g.keys);
    }
    g.delta = snapDelta(g.anchor, raw, grid);
    if (g.frame) return;
    const apply = () => {
      g.frame = 0;
      const tr = `translate(${g.delta.x} ${g.delta.y})`;
      for (const n of g.nodes) n.setAttribute('transform', tr);
    };
    g.frame = typeof requestAnimationFrame === 'function' ? requestAnimationFrame(apply) : 0;
    if (!g.frame) apply();
  };

  const finish = (e: PointerEvent<SVGSVGElement>, cancelled: boolean) => {
    const g = gesture.current;
    if (!g || g.pointerId !== e.pointerId) return;
    gesture.current = null;
    e.currentTarget.releasePointerCapture?.(e.pointerId);
    if (g.kind === 'band') {
      drawBand(null);
      const rect = rectFromPoints(g.start, g.end);
      if (cancelled) return;
      if (rect.width < DRAG_THRESHOLD_UNITS && rect.height < DRAG_THRESHOLD_UNITS) {
        if (!g.additive) props.onClearSelection();
        return;
      }
      props.onSelectKeys(selectInRect(doc, rect, g.additive ? selection : undefined));
      return;
    }
    if (g.frame && typeof cancelAnimationFrame === 'function') cancelAnimationFrame(g.frame);
    if (!g.moved) return;
    if (cancelled || (g.delta.x === 0 && g.delta.y === 0)) {
      for (const n of g.nodes) n.removeAttribute('transform');
      return;
    }
    const tr = `translate(${g.delta.x} ${g.delta.y})`;
    for (const n of g.nodes) n.setAttribute('transform', tr);
    translated.current = g.nodes;
    props.onMove(g.keys, g.delta, g.anchorKey);
  };

  const focusElement = (key: string) => {
    svgRef.current?.querySelector<SVGGElement>(`g[data-key="${key}"]`)?.focus();
    props.onSelect(key, false);
  };

  const onKeyDown = (e: KeyboardEvent<SVGSVGElement>) => {
    const key = keyOf(e.target);
    if (!key) return;
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      props.onSelect(key, e.shiftKey || e.ctrlKey || e.metaKey);
      return;
    }
    const at = order.indexOf(key);
    const navigation: Record<string, number> = {
      Home: 0,
      End: order.length - 1,
      PageUp: Math.max(0, at - 1),
      PageDown: Math.min(order.length - 1, at + 1),
      ...(readOnly
        ? {
            ArrowLeft: Math.max(0, at - 1),
            ArrowUp: Math.max(0, at - 1),
            ArrowRight: Math.min(order.length - 1, at + 1),
            ArrowDown: Math.min(order.length - 1, at + 1),
          }
        : {}),
    };
    const to = navigation[e.key];
    if (to !== undefined) {
      e.preventDefault();
      const next = order[to];
      if (next && next !== key) focusElement(next);
      return;
    }
    if (readOnly) return;
    const step = e.altKey ? 1 : (grid || 10) * (e.shiftKey ? 5 : 1);
    const deltas: Record<string, Point> = {
      ArrowLeft: { x: -step, y: 0 },
      ArrowRight: { x: step, y: 0 },
      ArrowUp: { x: 0, y: -step },
      ArrowDown: { x: 0, y: step },
    };
    const delta = deltas[e.key];
    if (delta) {
      e.preventDefault();
      if (!selection.has(key)) props.onSelect(key, false);
      props.onNudge(delta, key);
      return;
    }
    if (e.key === 'Delete' || e.key === 'Backspace') {
      e.preventDefault();
      if (!selection.has(key)) props.onSelect(key, false);
      props.onDelete(key);
    }
  };

  return (
    <svg
      ref={svgRef}
      role="group"
      aria-label={t('canvas.label', { count: doc.spots.length })}
      aria-roledescription={t('canvas.roleDescription')}
      viewBox={`${vb.x} ${vb.y} ${vb.width} ${vb.height}`}
      preserveAspectRatio="xMidYMid meet"
      className="h-[65vh] w-full touch-none select-none rounded-md border border-slate-200 bg-white"
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={(e) => finish(e, false)}
      onPointerCancel={(e) => finish(e, true)}
      onKeyDown={onKeyDown}
    >
      {grid > 0 && (
        <defs>
          <pattern id={gridId} width={grid} height={grid} patternUnits="userSpaceOnUse">
            <path
              d={`M ${grid} 0 L 0 0 0 ${grid}`}
              fill="none"
              stroke="#d6c9a8"
              strokeWidth={0.5}
            />
          </pattern>
        </defs>
      )}
      <rect
        data-background="sea"
        x={sea.x}
        y={sea.y}
        width={sea.width}
        height={sea.height}
        fill={SEA_COLOR}
      />
      <rect
        data-background="sand"
        x={0}
        y={0}
        width={doc.map.width}
        height={doc.map.height}
        fill={SAND_COLOR}
      />
      {grid > 0 && (
        <rect
          data-background="grid"
          x={0}
          y={0}
          width={doc.map.width}
          height={doc.map.height}
          fill={`url(#${gridId})`}
          pointerEvents="none"
        />
      )}
      {doc.decorations.map((d) => (
        <DecorationEl
          key={d.key}
          d={d}
          focusable={d.key === activeKey}
          selected={selection.has(d.key)}
          label={props.decorationLabel(d)}
        />
      ))}
      {doc.spots.map((s) => (
        <SpotEl
          key={s.key}
          spot={s}
          focusable={s.key === activeKey}
          selected={selection.has(s.key)}
          color={props.zoneColor(s.zone_id)}
          label={props.spotLabel(s)}
        />
      ))}
      <rect
        ref={bandRef}
        data-testid="rubber-band"
        visibility="hidden"
        fill="rgba(14, 107, 168, 0.12)"
        stroke="#0e6ba8"
        strokeWidth={1}
        strokeDasharray="4 3"
        vectorEffect="non-scaling-stroke"
        pointerEvents="none"
      />
    </svg>
  );
}

const focusClass =
  'cursor-move outline-none focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-slate-900';

const SpotEl = memo(function SpotEl({
  spot,
  focusable,
  selected,
  color,
  label,
}: {
  spot: DraftSpot;
  focusable: boolean;
  selected: boolean;
  color: string;
  label: string;
}) {
  const size = shapeSize(spot.shape);
  return (
    // The outer group carries only the drag translate (set imperatively).
    <g
      role="button"
      tabIndex={focusable ? 0 : -1}
      aria-label={label}
      aria-pressed={selected}
      data-key={spot.key}
      data-spot-key={spot.key}
      data-spot-label={spot.label}
      data-x={spot.x}
      data-y={spot.y}
      className={focusClass}
    >
      <g transform={spot.rotation ? `rotate(${spot.rotation} ${spot.x} ${spot.y})` : undefined}>
        <ShapeElement
          shape={spot.shape}
          x={spot.x}
          y={spot.y}
          fill={color}
          opacity={spot.active ? 1 : 0.35}
          stroke={selected ? '#0f172a' : '#ffffff'}
          strokeWidth={selected ? Math.max(2, size * 0.1) : Math.max(1, size * 0.04)}
          strokeDasharray={spot.active ? undefined : `${size * 0.12} ${size * 0.08}`}
        />
      </g>
      <text
        x={spot.x}
        y={spot.y}
        textAnchor="middle"
        dominantBaseline="central"
        fontSize={Math.max(6, size * 0.34)}
        fontWeight={600}
        fill="#0f172a"
        pointerEvents="none"
      >
        {spot.label}
      </text>
    </g>
  );
});

const DecorationEl = memo(function DecorationEl({
  d,
  focusable,
  selected,
  label,
}: {
  d: DraftDecoration;
  focusable: boolean;
  selected: boolean;
  label: string;
}) {
  const { t } = useTranslation('mapEditor');
  const size = shapeSize(d.shape);
  return (
    <g
      role="button"
      tabIndex={focusable ? 0 : -1}
      aria-label={t('canvas.decoration', { name: label })}
      aria-pressed={selected}
      data-key={d.key}
      data-decoration-key={d.key}
      data-x={d.x}
      data-y={d.y}
      className={focusClass}
    >
      <g transform={d.rotation ? `rotate(${d.rotation} ${d.x} ${d.y})` : undefined}>
        <ShapeElement
          shape={d.shape}
          x={d.x}
          y={d.y}
          fill={DECORATION_COLOR}
          stroke={selected ? '#0f172a' : '#b8a77f'}
          strokeWidth={selected ? Math.max(2, size * 0.08) : 1}
        />
      </g>
      <text
        x={d.x}
        y={d.y}
        textAnchor="middle"
        dominantBaseline="central"
        fontSize={Math.max(6, Math.min(size * 0.3, 14))}
        fill="#57534e"
        pointerEvents="none"
      >
        {label}
      </text>
    </g>
  );
});
