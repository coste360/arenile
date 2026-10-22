import { useGesture } from '@use-gesture/react';
import { memo, useCallback, useId, useMemo, useRef, useState, type KeyboardEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { readableTextColor } from '../theme/contrast';
import { cn } from '../lib/utils';
import { STATUS_CLASSES } from './colors';
import {
  IDENTITY,
  MAX_SCALE,
  MIN_SCALE,
  computeViewBox,
  seaRect,
  transformAttr,
  zoomAt,
  type Transform,
  type ViewBox,
} from './geometry';
import { decorationKindIcon } from './icons';
import { MapGlyph } from './MapGlyph';
import { MapLegend } from './MapLegend';
import { MapZoomControls } from './MapZoomControls';
import { ShapeElement } from './shapes';
import type { MapDecoration, MapGeometry, MapSpot, SpotStatus } from './types';

export interface BeachMapProps {
  geometry: MapGeometry;
  /** Status per spot id. Missing spots default to `defaultStatus`. */
  statuses?: Readonly<Record<string, SpotStatus>>;
  /** Status for spots without an entry in `statuses` (default `free`). */
  defaultStatus?: SpotStatus;
  /** Highlighted spot ids. */
  selectedIds?: readonly string[];
  /** Called on click, tap, Enter or Space on a spot. */
  onSpotActivate?: (spot: MapSpot) => void;
  /** Optional extra text per spot (e.g. a price), appended to the aria-label. */
  spotDescription?: (spot: MapSpot) => string | undefined;
  /** Colour overrides (hex), e.g. from the panel theme; default: the --status-* theme tokens. */
  statusColors?: Partial<Record<SpotStatus, string>>;
  /** Statuses listed in the legend; defaults to those present on the map. */
  legendStatuses?: readonly SpotStatus[];
  showLegend?: boolean;
  /** Accessible name of the map; defaults to the translated "map.title". */
  title?: string;
  className?: string;
  /** CSS height of the drawing area (default 60vh). */
  height?: string;
}

interface Point {
  x: number;
  y: number;
}

function clientToViewBox(
  svg: SVGSVGElement | null,
  vb: ViewBox,
  clientX: number,
  clientY: number,
): Point {
  const center = { x: vb.x + vb.width / 2, y: vb.y + vb.height / 2 };
  if (!svg) return center;
  const ctm = typeof svg.getScreenCTM === 'function' ? svg.getScreenCTM() : null;
  if (!ctm || typeof svg.createSVGPoint !== 'function') return center;
  const pt = svg.createSVGPoint();
  pt.x = clientX;
  pt.y = clientY;
  const p = pt.matrixTransform(ctm.inverse());
  return { x: p.x, y: p.y };
}

/** Screen pixels to viewBox units, assuming preserveAspectRatio "xMidYMid meet". */
function pixelRatio(svg: SVGSVGElement | null, vb: ViewBox): number {
  if (!svg) return 1;
  const w = svg.clientWidth;
  const h = svg.clientHeight;
  if (!w || !h) return 1;
  return Math.max(vb.width / w, vb.height / h);
}

interface SpotProps {
  spot: MapSpot;
  status: SpotStatus;
  /** Hex override; undefined = theme token classes. */
  color: string | undefined;
  selected: boolean;
  ariaLabel: string;
  onActivate?: (spot: MapSpot) => void;
}

const Spot = memo(function Spot({
  spot,
  status,
  color,
  selected,
  ariaLabel,
  onActivate,
}: SpotProps) {
  const disabled = spot.active === false || !onActivate;
  const size = spot.shape.type === 'circle' ? spot.shape.w : Math.min(spot.shape.w, spot.shape.h);
  const activate = () => {
    if (spot.active !== false) onActivate?.(spot);
  };
  const onKeyDown = (e: KeyboardEvent<SVGGElement>) => {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      activate();
    }
  };
  return (
    <g
      role="button"
      tabIndex={0}
      aria-label={ariaLabel}
      aria-pressed={selected}
      aria-disabled={disabled || undefined}
      data-spot-id={spot.id}
      data-status={status}
      className={cn(
        'outline-none focus-visible:[&>:first-child]:stroke-ring focus-visible:[&>:first-child]:[stroke-width:4px]',
        disabled ? 'cursor-default' : 'cursor-pointer',
      )}
      transform={spot.rotation ? `rotate(${spot.rotation} ${spot.x} ${spot.y})` : undefined}
      onClick={activate}
      onKeyDown={onKeyDown}
    >
      <ShapeElement
        shape={spot.shape}
        x={spot.x}
        y={spot.y}
        fill={color}
        className={cn(
          !color && STATUS_CLASSES[status].fill,
          selected ? 'stroke-map-spot-selected' : 'stroke-map-spot-stroke',
        )}
        strokeWidth={selected ? size * 0.12 : size * 0.04}
      />
      <text
        x={spot.x}
        y={spot.y}
        textAnchor="middle"
        dominantBaseline="central"
        fontSize={size * 0.38}
        fontWeight={600}
        fill={color ? readableTextColor(color) : undefined}
        className={color ? undefined : STATUS_CLASSES[status].labelFill}
        pointerEvents="none"
        aria-hidden="true"
      >
        {spot.label}
      </text>
    </g>
  );
});

function Decoration({ decoration, label }: { decoration: MapDecoration; label: string }) {
  const { shape, x, y } = decoration;
  const size = shape.type === 'circle' ? shape.w : Math.min(shape.w, shape.h);
  // Room for a glyph above the label on square-ish shapes.
  const glyph = decoration.kind !== 'text' && size >= 28 ? size * 0.42 : 0;
  return (
    <g
      transform={decoration.rotation ? `rotate(${decoration.rotation} ${x} ${y})` : undefined}
      aria-hidden="true"
      data-decoration={decoration.kind}
    >
      {decoration.kind !== 'text' && (
        <ShapeElement
          shape={shape}
          x={x}
          y={y}
          className="fill-map-decoration stroke-map-decoration-stroke"
          strokeWidth={size * 0.03}
        />
      )}
      {glyph > 0 && (
        <MapGlyph
          icon={decorationKindIcon(decoration.kind)}
          x={x}
          y={label ? y - size * 0.14 : y}
          size={glyph}
          className="text-map-decoration-label opacity-80"
        />
      )}
      <text
        x={x}
        y={glyph > 0 ? y + size * 0.26 : y}
        textAnchor="middle"
        dominantBaseline="central"
        fontSize={Math.max(6, size * (glyph > 0 ? 0.18 : 0.28))}
        className="fill-map-decoration-label"
        pointerEvents="none"
      >
        {label}
      </text>
    </g>
  );
}

/**
 * Read-only SVG beach map (spec 11.1): logical coordinates in a viewBox,
 * pan and zoom with touch, mouse drag and wheel, accessible spots, sea side,
 * decorations and a status legend that is always visible.
 */
export function BeachMap({
  geometry,
  statuses,
  defaultStatus = 'free',
  selectedIds,
  onSpotActivate,
  spotDescription,
  statusColors,
  legendStatuses,
  showLegend = true,
  title,
  className,
  height = '60vh',
}: BeachMapProps) {
  const { t } = useTranslation();
  const svgRef = useRef<SVGSVGElement>(null);
  const titleId = useId();
  const legendId = useId();
  const [view, setView] = useState<Transform>(IDENTITY);

  const vb = useMemo(() => computeViewBox(geometry), [geometry]);
  const sea = useMemo(() => seaRect(geometry), [geometry]);
  const selected = useMemo(() => new Set(selectedIds ?? []), [selectedIds]);

  const statusOf = useCallback(
    (spot: MapSpot): SpotStatus =>
      spot.active === false ? 'unavailable' : (statuses?.[spot.id] ?? defaultStatus),
    [statuses, defaultStatus],
  );

  const legend = useMemo(() => {
    if (legendStatuses) return legendStatuses;
    const present = new Set(geometry.spots.map(statusOf));
    return [...present];
  }, [legendStatuses, geometry.spots, statusOf]);

  useGesture(
    {
      onDrag: ({ delta: [dx, dy], pinching, cancel, tap }) => {
        if (pinching) {
          cancel();
          return;
        }
        if (tap) return;
        const k = pixelRatio(svgRef.current, vb);
        setView((v) => ({ ...v, x: v.x + dx * k, y: v.y + dy * k }));
      },
      onPinch: ({ origin: [ox, oy], delta: [ds] }) => {
        if (!ds) return;
        const p = clientToViewBox(svgRef.current, vb, ox, oy);
        setView((v) => zoomAt(v, p, 1 + ds));
      },
      onWheel: ({ event, delta: [, dy] }) => {
        event.preventDefault();
        const e = event as WheelEvent;
        const p = clientToViewBox(svgRef.current, vb, e.clientX, e.clientY);
        setView((v) => zoomAt(v, p, Math.exp(-dy * 0.0015)));
      },
    },
    {
      target: svgRef,
      eventOptions: { passive: false },
      drag: { filterTaps: true, pointer: { buttons: [1] } },
      pinch: { scaleBounds: { min: 0.5, max: 8 } },
    },
  );

  const center = { x: vb.x + vb.width / 2, y: vb.y + vb.height / 2 };
  const seaLabelVertical = geometry.sea_side === 'left' || geometry.sea_side === 'right';
  const accessibleTitle = title ?? geometry.name ?? t('map.title');

  return (
    <div className={cn('relative', className)} data-testid="beach-map">
      <svg
        ref={svgRef}
        viewBox={`${vb.x} ${vb.y} ${vb.width} ${vb.height}`}
        preserveAspectRatio="xMidYMid meet"
        role="group"
        aria-labelledby={titleId}
        aria-describedby={showLegend ? legendId : undefined}
        className="block w-full touch-none rounded-lg bg-map-sand select-none"
        // Height is a caller-provided CSS length (dynamic geometry).
        style={{ height }}
      >
        <title id={titleId}>{accessibleTitle}</title>
        <g transform={transformAttr(view)} data-testid="beach-map-viewport">
          <rect
            x={0}
            y={0}
            width={geometry.width}
            height={geometry.height}
            className="fill-map-sand"
          />
          <g aria-hidden="true" data-sea-side={geometry.sea_side}>
            <rect
              x={sea.x}
              y={sea.y}
              width={sea.width}
              height={sea.height}
              className="fill-map-sea"
            />
            <text
              x={sea.x + sea.width / 2}
              y={sea.y + sea.height / 2}
              textAnchor="middle"
              dominantBaseline="central"
              fontSize={Math.min(sea.width, sea.height) * 0.4}
              className="fill-map-sea-label"
              letterSpacing="0.2em"
              transform={
                seaLabelVertical
                  ? `rotate(-90 ${sea.x + sea.width / 2} ${sea.y + sea.height / 2})`
                  : undefined
              }
            >
              {t('map.sea')}
            </text>
          </g>
          {(geometry.decorations ?? []).map((d) => (
            <Decoration
              key={d.id}
              decoration={d}
              label={
                d.label ??
                (d.kind === 'text' ? '' : t(`map.decoration.${d.kind}`, { defaultValue: '' }))
              }
            />
          ))}
          {geometry.spots.map((spot) => {
            const status = statusOf(spot);
            const extra = spotDescription?.(spot);
            const aria = t('map.spotAria', {
              label: spot.label,
              status: t(`map.status.${status}`),
            });
            return (
              <Spot
                key={spot.id}
                spot={spot}
                status={status}
                color={statusColors?.[status]}
                selected={selected.has(spot.id)}
                ariaLabel={extra ? `${aria}, ${extra}` : aria}
                onActivate={onSpotActivate}
              />
            );
          })}
        </g>
      </svg>
      <MapZoomControls
        className="absolute top-2 right-2"
        scale={view.scale}
        min={MIN_SCALE}
        max={MAX_SCALE}
        onZoomIn={() => setView((v) => zoomAt(v, center, 1.25))}
        onZoomOut={() => setView((v) => zoomAt(v, center, 0.8))}
        onFit={() => setView(IDENTITY)}
        onScaleChange={(scale) => setView((v) => zoomAt(v, center, scale / v.scale))}
      />
      {showLegend && (
        <MapLegend id={legendId} statuses={legend} statusColors={statusColors} className="mt-2" />
      )}
    </div>
  );
}
