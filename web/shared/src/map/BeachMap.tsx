import { useGesture } from '@use-gesture/react';
import {
  memo,
  useCallback,
  useId,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent,
  type ReactNode,
} from 'react';
import { useTranslation } from 'react-i18next';
import { cn } from '../lib/utils';
import {
  IDENTITY,
  MAX_SCALE,
  MIN_SCALE,
  clampScale,
  computeViewBox,
  transformAttr,
  zoomAt,
  type Transform,
  type ViewBox,
} from './geometry';
import { decorationKindIcon } from './icons';
import {
  DecorationArt,
  KIT_BY_ID,
  MapBackdrop,
  MapKitDefs,
  MapLabel,
  SpotArt,
  StatusBadge,
  decorationAsset,
  geometryKitIds,
  spotArtSize,
  spotAsset,
} from './kit';
import {
  MIN_LABEL_PX,
  labelFontSize,
  padViewBox,
  quantiseUnitsPerPixel,
  useUnitsPerPixel,
} from './labels';
import { INDICATOR_CLASSES, type SpotIndicator } from './indicators';
import {
  captionsLegible,
  layoutCaption,
  spotNeighbours,
  type CaptionLayout,
  type MapLabelMode,
} from './captions';
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
  /**
   * Search matches (operational map): when non-empty, the other spots are
   * dimmed and these get an emphasised outline. Independent of the selection.
   */
  highlightedIds?: readonly string[];
  /** Overlay indicators per spot id (extra, note, VIP); repeat them in `spotDescription`. */
  spotIndicators?: Readonly<Record<string, readonly SpotIndicator[]>>;
  /** Called on click, tap, Enter or Space on a spot. */
  onSpotActivate?: (spot: MapSpot) => void;
  /**
   * Public seat picker: only spots in these statuses can be activated; the
   * others stay readable (aria-label with their status) but are
   * aria-disabled. Default: every active spot can be activated.
   */
  pickableStatuses?: readonly SpotStatus[];
  /**
   * Minimum on-screen size (CSS px) of a spot for a pointer tap to activate
   * it (WCAG 2.5.8 target size, 44 px on the public page). A tap on a smaller
   * spot zooms the map in, centred on that spot, until it reaches the size;
   * the next tap activates it. Enter and Space always activate.
   */
  minTapTargetPx?: number;
  /** Show the zoom slider next to the zoom buttons (default true). */
  zoomSlider?: boolean;
  /**
   * Caption per spot id, drawn as a second pill under the art (operational
   * map: who occupies the spot). Full text: the map truncates it to the room
   * between neighbours and hides it when it would not be legible. Repeat it
   * in `spotDescription` (accessible name and tooltip).
   */
  spotCaptions?: Readonly<Record<string, string>>;
  /** Which labels are drawn when `spotCaptions` is set (default `both`). */
  labelMode?: MapLabelMode;
  /** Extra controls in the toolbar above the drawing, left of the zoom controls. */
  toolbar?: ReactNode;
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
  /**
   * Maximum CSS height of the drawing area (default 60vh). The drawing keeps
   * the aspect ratio of the map, so a landscape map on a phone takes only
   * the height it needs instead of leaving empty space above and below.
   */
  height?: string;
}

interface Point {
  x: number;
  y: number;
}

/** Transform that shows `point` at the centre of the drawing at `scale` (clamped). */
function centerOn(vb: ViewBox, point: Point, scale: number): Transform {
  const s = clampScale(scale);
  const cx = vb.x + vb.width / 2;
  const cy = vb.y + vb.height / 2;
  return { scale: s, x: cx - point.x * s, y: cy - point.y * s };
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
  /** `true` search match, `false` dimmed (another spot matches), undefined no search. */
  highlighted: boolean | undefined;
  indicators: readonly SpotIndicator[] | undefined;
  /** The one spot reachable with Tab (roving tabindex). */
  focusable: boolean;
  /** Map units per screen pixel, zoom included (label legibility). */
  unitsPerPixel: number;
  ariaLabel: string;
  /** Draw the spot code pill. */
  showCode: boolean;
  /** Native tooltip (the full caption the pill may truncate); undefined = none. */
  tooltip: string | undefined;
  /** False when the status is not pickable (public picker): aria-disabled, no activation. */
  pickable: boolean;
  onActivate?: (spot: MapSpot) => void;
  /** Pointer activation (click / tap); defaults to `onActivate`. */
  onPointerActivate?: (spot: MapSpot) => void;
}

/** Check mark on a selected spot: the choice reads at a glance, not only as a ring. */
function SelectedBadge({ x, y, r }: { x: number; y: number; r: number }) {
  return (
    <g pointerEvents="none" aria-hidden="true" data-selected-badge="">
      <circle
        cx={x}
        cy={y}
        r={r}
        className="fill-primary stroke-background"
        strokeWidth={r * 0.25}
      />
      <path
        d={`M ${x - r * 0.45} ${y + r * 0.02} L ${x - r * 0.1} ${y + r * 0.38} L ${x + r * 0.48} ${y - r * 0.32}`}
        className="fill-none stroke-primary-foreground"
        strokeWidth={r * 0.28}
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </g>
  );
}

const Spot = memo(function Spot({
  spot,
  status,
  color,
  selected,
  highlighted,
  indicators,
  focusable,
  unitsPerPixel,
  ariaLabel,
  showCode,
  tooltip,
  pickable,
  onActivate,
  onPointerActivate,
}: SpotProps) {
  const disabled = spot.active === false || !onActivate || !pickable;
  const S = spotArtSize(spot.shape);
  const fontSize = labelFontSize(S, 0.2, unitsPerPixel, 0.42);
  const badgeR = Math.min(Math.max(S * 0.12, 6.5 * unitsPerPixel), S * 0.2);
  const dotR = Math.min(Math.max(S * 0.085, 4.5 * unitsPerPixel), S * 0.14);
  const activate = () => {
    if (!disabled) onActivate?.(spot);
  };
  const pointerActivate = () => {
    if (!disabled) (onPointerActivate ?? onActivate)?.(spot);
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
      tabIndex={focusable ? 0 : -1}
      aria-label={ariaLabel}
      aria-pressed={selected}
      aria-disabled={disabled || undefined}
      data-spot-id={spot.id}
      data-status={status}
      data-highlighted={highlighted === undefined ? undefined : String(highlighted)}
      className={cn(
        'outline-none focus-visible:[&>:first-child]:stroke-ring focus-visible:[&>:first-child]:stroke-4',
        disabled ? 'cursor-default' : 'cursor-pointer',
        highlighted === false && 'opacity-30',
      )}
      onClick={pointerActivate}
      onKeyDown={onKeyDown}
    >
      {tooltip && <title>{tooltip}</title>}
      <SpotArt
        x={spot.x}
        y={spot.y}
        shape={spot.shape}
        rotation={spot.rotation}
        asset={spotAsset(spot)}
        status={status}
        color={color}
        tint={status === 'free' && !selected ? 'faint' : 'strong'}
        emphasis={selected || !!highlighted}
        muted={status === 'unavailable'}
      />
      {selected ? (
        <SelectedBadge x={spot.x + S * 0.34} y={spot.y - S * 0.34} r={badgeR * 1.25} />
      ) : (
        <StatusBadge
          status={status}
          x={spot.x + S * 0.34}
          y={spot.y - S * 0.34}
          r={badgeR}
          color={color}
        />
      )}
      {indicators?.map((kind, i) => (
        <circle
          key={kind}
          cx={spot.x - S * 0.36 + i * dotR * 2.3}
          cy={spot.y - S * 0.36}
          r={dotR}
          data-indicator={kind}
          className={cn(INDICATOR_CLASSES[kind].fill, 'stroke-background')}
          strokeWidth={dotR * 0.35}
          pointerEvents="none"
          aria-hidden="true"
        />
      ))}
      {showCode && fontSize > 0 && (
        <MapLabel
          x={spot.x}
          y={spot.y + S * 0.36}
          text={spot.label}
          fontSize={fontSize}
          muted={status === 'unavailable'}
        />
      )}
    </g>
  );
});

/**
 * Caption pills of the spots, one layer above every spot: a caption may
 * reach under a neighbour's art, and it is not part of the spot button, so
 * the button's visible text stays its code (WCAG 2.5.3, label in name) while
 * the full caption is in its accessible name and tooltip. Pointer events
 * pass through to the spots. Re-renders only when the layout changes.
 */
const CaptionLayer = memo(function CaptionLayer({
  layouts,
  highlighted,
}: {
  layouts: ReadonlyMap<string, (CaptionLayout & { spotLabel: string }) | undefined>;
  highlighted: ReadonlySet<string> | null;
}) {
  return (
    <g pointerEvents="none" aria-hidden="true" data-testid="beach-map-captions">
      {[...layouts].map(([id, c]) =>
        c ? (
          <g
            key={id}
            data-caption-for={id}
            data-caption-spot={c.spotLabel}
            className={cn(highlighted && !highlighted.has(id) && 'opacity-30')}
          >
            <MapLabel x={c.x} y={c.y} text={c.text} fontSize={c.fontSize} variant="caption" />
          </g>
        ) : null,
      )}
    </g>
  );
});

const Decoration = memo(function Decoration({
  decoration,
  label,
  name,
  unitsPerPixel,
}: {
  decoration: MapDecoration;
  /** Explicit label of the decoration (drawn), if any. */
  label: string;
  /** Accessible / tooltip name: the label or the kind. */
  name: string;
  unitsPerPixel: number;
}) {
  const { shape, x, y } = decoration;
  const w = shape.w;
  const h = shape.type === 'circle' ? shape.w : shape.h;
  const asset = decorationAsset(decoration);
  const fontSize = Math.max(Math.min(w, h) * 0.2, MIN_LABEL_PX * unitsPerPixel);
  // Named decorations are exposed as images; unnamed ones stay decorative.
  const a11y = name ? { role: 'img', 'aria-label': name } : { 'aria-hidden': true as const };
  if (asset) {
    return (
      <g {...a11y} data-decoration={decoration.kind}>
        <title>{name}</title>
        <DecorationArt
          asset={asset}
          x={x}
          y={y}
          shape={shape}
          rotation={decoration.rotation}
          variant={decoration.variant}
        />
        {label && (
          <MapLabel x={x} y={y + h / 2 + fontSize * 0.5} text={label} fontSize={fontSize} />
        )}
      </g>
    );
  }
  // Text labels and custom elements without an illustration.
  const size = Math.min(w, h);
  const glyph = decoration.kind !== 'text' && size >= 28 ? size * 0.42 : 0;
  return (
    <g
      transform={decoration.rotation ? `rotate(${decoration.rotation} ${x} ${y})` : undefined}
      {...a11y}
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
        fontSize={Math.max(size * (glyph > 0 ? 0.18 : 0.28), MIN_LABEL_PX * unitsPerPixel)}
        className="fill-map-decoration-label"
        pointerEvents="none"
      >
        <title>{name}</title>
        {label}
      </text>
    </g>
  );
});

const NAV_KEYS: Record<string, 'prev' | 'next' | 'first' | 'last'> = {
  ArrowLeft: 'prev',
  ArrowUp: 'prev',
  PageUp: 'prev',
  ArrowRight: 'next',
  ArrowDown: 'next',
  PageDown: 'next',
  Home: 'first',
  End: 'last',
};

/**
 * Read-only SVG beach map (spec 11.1): logical coordinates in a viewBox,
 * pan and zoom with touch, mouse drag and wheel, accessible spots, sea side,
 * decorations and a status legend that is always visible.
 *
 * Keyboard: the map is one tab stop (the first selected spot, else the first
 * spot); arrows, PageUp/PageDown and Home/End move between spots, Enter or
 * Space activate one, Tab leaves the map.
 */
export function BeachMap({
  geometry,
  statuses,
  defaultStatus = 'free',
  selectedIds,
  highlightedIds,
  spotIndicators,
  onSpotActivate,
  pickableStatuses,
  minTapTargetPx,
  zoomSlider = true,
  spotDescription,
  spotCaptions,
  labelMode = 'both',
  toolbar,
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

  // Padded so that labels on the edges (decorations at x = 0) are never clipped.
  const vb = useMemo(() => padViewBox(computeViewBox(geometry), 8), [geometry]);
  const kitIds = useMemo(() => geometryKitIds(geometry), [geometry]);
  const selected = useMemo(() => new Set(selectedIds ?? []), [selectedIds]);
  const highlighted = useMemo(
    () => (highlightedIds && highlightedIds.length > 0 ? new Set(highlightedIds) : null),
    [highlightedIds],
  );
  // Quantised: a wheel step must not re-render every memoised spot.
  const fitUnitsPerPixel = useUnitsPerPixel(svgRef, vb);
  const unitsPerPixel = quantiseUnitsPerPixel(fitUnitsPerPixel / view.scale);
  const pickable = useMemo(
    () => (pickableStatuses ? new Set(pickableStatuses) : null),
    [pickableStatuses],
  );

  // Captions: neighbours once per geometry, layout once per quantised zoom
  // step / data change (never per pan step), so 500+ memoised spots stay put.
  const showCaptions = !!spotCaptions && labelMode !== 'code';
  const neighbours = useMemo(
    () => (showCaptions ? spotNeighbours(geometry.spots, (s) => spotArtSize(s.shape)) : null),
    [showCaptions, geometry.spots],
  );
  const captions = useMemo(() => {
    const out = new Map<string, (CaptionLayout & { spotLabel: string }) | undefined>();
    let hidden = false;
    if (!neighbours || !spotCaptions) return { out, hidden };
    const has = (id: string) => !!spotCaptions[id];
    const legible = captionsLegible(
      geometry.spots,
      (s) => spotArtSize(s.shape),
      neighbours,
      unitsPerPixel,
      (size) => (labelMode === 'both' ? labelFontSize(size, 0.2, unitsPerPixel, 0.42) : 0),
    );
    for (const spot of geometry.spots) {
      const text = spotCaptions[spot.id];
      if (!text || spot.active === false) continue;
      if (!legible) {
        hidden = true;
        continue;
      }
      const S = spotArtSize(spot.shape);
      // "Entrambi": the caption goes under the code pill, never over it.
      const codeFont = labelMode === 'both' ? labelFontSize(S, 0.2, unitsPerPixel, 0.42) : 0;
      const c = layoutCaption(
        spot,
        text,
        S,
        neighbours.get(spot.id),
        has,
        unitsPerPixel,
        codeFont > 0 ? spot.y + S * 0.36 + codeFont * 0.66 : undefined,
      );
      // A single spot without room (an irregular row) stays without caption
      // but does not raise the zoom hint: only the map-wide decision does.
      out.set(spot.id, c && { ...c, spotLabel: spot.label });
    }
    return { out, hidden };
  }, [neighbours, spotCaptions, geometry.spots, unitsPerPixel, labelMode]);

  // Pointer taps on spots smaller than minTapTargetPx zoom in first. The
  // callback is stable (latest values through a ref) so memoised spots do
  // not re-render on every pan or zoom step.
  const tapRef = useRef({ view, fitUnitsPerPixel, vb, minTapTargetPx, onSpotActivate });
  useLayoutEffect(() => {
    tapRef.current = { view, fitUnitsPerPixel, vb, minTapTargetPx, onSpotActivate };
  });
  const onPointerActivate = useCallback((spot: MapSpot) => {
    const c = tapRef.current;
    const size = spotArtSize(spot.shape);
    const onScreen = (size / c.fitUnitsPerPixel) * c.view.scale;
    if (!c.minTapTargetPx || onScreen >= c.minTapTargetPx || c.view.scale >= MAX_SCALE) {
      c.onSpotActivate?.(spot);
      return;
    }
    setView(centerOn(c.vb, spot, (c.view.scale * c.minTapTargetPx * 1.15) / onScreen));
  }, []);

  // Roving tabindex: one focusable spot, moved with the keyboard.
  const [activeId, setActiveId] = useState<string | null>(null);
  const spotIds = geometry.spots.map((s) => s.id);
  const focusableId =
    (activeId && spotIds.includes(activeId) ? activeId : null) ??
    spotIds.find((id) => selected.has(id)) ??
    (highlighted ? spotIds.find((id) => highlighted.has(id)) : undefined) ??
    spotIds[0];
  const onKeyDown = (e: KeyboardEvent<SVGSVGElement>) => {
    const move = NAV_KEYS[e.key];
    if (!move) return;
    const current = (e.target as Element | null)?.closest?.('[data-spot-id]');
    const id = current?.getAttribute('data-spot-id');
    if (!id) return;
    const at = spotIds.indexOf(id);
    const to =
      move === 'first'
        ? 0
        : move === 'last'
          ? spotIds.length - 1
          : Math.min(spotIds.length - 1, Math.max(0, at + (move === 'next' ? 1 : -1)));
    const next = spotIds[to];
    if (!next || next === id) {
      e.preventDefault();
      return;
    }
    e.preventDefault();
    setActiveId(next);
    svgRef.current?.querySelector<SVGGElement>(`[data-spot-id="${next}"]`)?.focus();
  };

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
  const accessibleTitle = title ?? geometry.name ?? t('map.title');

  // Labels are dropped under 9 px on screen: say so, with the way out.
  const first = geometry.spots[0];
  const labelsHidden =
    (!!first && labelFontSize(spotArtSize(first.shape), 0.2, unitsPerPixel, 0.42) === 0) ||
    (showCaptions && captions.hidden);

  /** Name of a decoration: its label, the kit asset or the kind, plus the flag colour. */
  const decorationName = (d: MapDecoration): string => {
    const asset = decorationAsset(d);
    const base =
      d.label ||
      (d.kind === 'custom' && asset ? (KIT_BY_ID.get(asset)?.label ?? '') : '') ||
      (d.kind === 'text' ? '' : t(`map.decoration.${d.kind}`, { defaultValue: '' }));
    const variants = asset ? KIT_BY_ID.get(asset)?.variants : undefined;
    if (!d.variant || !variants?.includes(d.variant)) return base;
    const flag = t(`map.flag.${d.variant}`, { defaultValue: d.variant });
    return base ? `${base}: ${flag}` : flag;
  };

  return (
    <div className={cn('flex flex-col', className)} data-testid="beach-map">
      {/* Legend and zoom controls above the drawing: visible without scrolling past the map. */}
      <div className="mb-2 flex flex-wrap items-end justify-between gap-x-4 gap-y-2">
        {showLegend && <MapLegend id={legendId} statuses={legend} statusColors={statusColors} />}
        {toolbar}
        <MapZoomControls
          className="ml-auto w-fit"
          scale={view.scale}
          min={MIN_SCALE}
          max={MAX_SCALE}
          onZoomIn={() => setView((v) => zoomAt(v, center, 1.25))}
          onZoomOut={() => setView((v) => zoomAt(v, center, 0.8))}
          onFit={() => setView(IDENTITY)}
          onScaleChange={
            zoomSlider ? (scale) => setView((v) => zoomAt(v, center, scale / v.scale)) : undefined
          }
        />
      </div>
      <svg
        ref={svgRef}
        viewBox={`${vb.x} ${vb.y} ${vb.width} ${vb.height}`}
        preserveAspectRatio="xMidYMid meet"
        role="group"
        aria-labelledby={titleId}
        aria-describedby={showLegend ? legendId : undefined}
        className="block h-auto w-full touch-none rounded-lg bg-muted/40 select-none"
        // Dynamic geometry: the drawing keeps the map's aspect ratio up to the
        // caller-provided maximum CSS height.
        style={{ aspectRatio: `${vb.width} / ${vb.height}`, maxHeight: height }}
        onKeyDown={onKeyDown}
      >
        <title id={titleId}>{accessibleTitle}</title>
        <g transform={transformAttr(view)} data-testid="beach-map-viewport">
          <MapBackdrop geometry={geometry} seaLabel={t('map.sea')} />
          {(geometry.decorations ?? []).map((d) => (
            <Decoration
              key={d.id}
              decoration={d}
              unitsPerPixel={unitsPerPixel}
              // A flag without a label is captioned with its colour, so the
              // sea state never relies on the colour alone.
              label={
                d.label ??
                (d.variant && KIT_BY_ID.get(decorationAsset(d) ?? '')?.variants?.includes(d.variant)
                  ? t(`map.flag.${d.variant}`, { defaultValue: d.variant })
                  : '')
              }
              name={decorationName(d)}
            />
          ))}
          {geometry.spots.map((spot) => {
            const status = statusOf(spot);
            const extra = spotDescription?.(spot);
            const aria = t('map.spotAria', {
              label: spot.label,
              status: t(`map.status.${status}`),
            });
            const caption = showCaptions ? captions.out.get(spot.id) : undefined;
            const ariaLabel = extra ? `${aria}, ${extra}` : aria;
            return (
              <Spot
                key={spot.id}
                spot={spot}
                status={status}
                color={statusColors?.[status]}
                selected={selected.has(spot.id)}
                highlighted={highlighted ? highlighted.has(spot.id) : undefined}
                indicators={spotIndicators?.[spot.id]}
                focusable={spot.id === focusableId}
                unitsPerPixel={unitsPerPixel}
                ariaLabel={ariaLabel}
                showCode={labelMode !== 'customer' || !showCaptions || !caption}
                // Only the description (full occupant name): the name itself is the aria-label.
                tooltip={spotCaptions?.[spot.id] ? extra : undefined}
                pickable={!pickable || pickable.has(status)}
                onActivate={onSpotActivate}
                onPointerActivate={minTapTargetPx ? onPointerActivate : undefined}
              />
            );
          })}
          {showCaptions && <CaptionLayer layouts={captions.out} highlighted={highlighted} />}
        </g>
      </svg>
      {labelsHidden && (
        <p className="mt-1 text-xs text-muted-foreground" data-testid="beach-map-zoom-hint">
          {t('map.zoomHint')}
        </p>
      )}
      <MapKitDefs ids={kitIds} />
    </div>
  );
}
