import { memo } from 'react';
import { cn } from '../../lib/utils';
import { STATUS_CLASSES } from '../colors';
import type { Shape, SpotStatus } from '../types';
import { CAPTION_CHAR_EM, graphemeCount, labelPillWidth } from '../captions';
import { kitSymbolId } from './assets';
import { mapKitInfo, useMapKit } from './context';
import type { MapKitKey } from './generated/manifest';
import { isTileAsset, spotArtSize, tileLayout } from './geometry';
import { STATUS_BADGES, badgeSymbolId } from './status';

/**
 * Flag colour of one element: sets the `--beach-flag` variable the kit art
 * reads (Tailwind arbitrary property on a theme token, no custom CSS).
 * Written out literally so Tailwind can see every class.
 */
const FLAG_CLASSES: Record<string, string> = {
  green: '[--beach-flag:var(--beach-flag-green)]',
  yellow: '[--beach-flag:var(--beach-flag-yellow)]',
  red: '[--beach-flag:var(--beach-flag-red)]',
};

export function flagClass(variant: string | null | undefined): string | undefined {
  return variant ? FLAG_CLASSES[variant] : undefined;
}

export interface KitUseProps {
  asset: string;
  /** Top-left corner and size of the drawing box, map units. */
  x: number;
  y: number;
  size: number;
  className?: string;
}

/**
 * One kit illustration: a <use> of the shared sprite symbol (no inline SVG
 * copy), in the kit of the nearest MapKitProvider.
 */
export function KitUse({ asset, x, y, size, className }: KitUseProps) {
  const kit = useMapKit();
  return (
    <use
      href={`#${kitSymbolId(kit, asset)}`}
      x={x}
      y={y}
      width={size}
      height={size}
      data-asset={asset}
      className={className}
      pointerEvents="none"
    />
  );
}

const rotate = (deg: number | null | undefined, x: number, y: number) =>
  deg ? `rotate(${deg} ${x} ${y})` : undefined;

export interface SpotArtProps {
  x: number;
  y: number;
  shape: Shape;
  rotation?: number | null;
  /** Kit asset; null draws the plain footprint (custom spots without asset). */
  asset: string | null;
  /** Status colour: a status (theme token classes) or a literal colour. */
  status?: SpotStatus;
  color?: string;
  /** Strong tint (booked, held...), faint (free) or outline (editor zones: light fill + ring). */
  tint?: 'strong' | 'faint' | 'outline';
  /** Outline: selection / search match. */
  emphasis?: boolean;
  /** Inactive or unavailable: greyed art, dashed halo. */
  muted?: boolean;
  /** Extra classes of the halo (focus ring variants of the parent target it). */
  haloClassName?: string;
}

/**
 * How much larger than the art box the status halo is: the ring and the
 * tint stay visible around the illustration, not only under it.
 */
export const HALO_SCALE = 1.08;

/**
 * Side of the outline ring of the modern kit, as a fraction of the art box:
 * the v3 art stays inside 12..108 of its 120-unit viewBox (0.8), so the
 * ring sits just outside the drawing and never covers the coloured body.
 */
export const RING_SCALE = 0.94;

/**
 * Centre of the corner badge, as a fraction of the art box from its centre:
 * classic draws it over the art (0.34); the modern kit keeps the indicator
 * corner (x 92-120, y 0-28 of 120) empty, centred at 106,14, i.e. 0.383.
 */
export function badgeOffset(kit: MapKitKey): number {
  return mapKitInfo(kit).style === 'fill' ? 0.383 : 0.34;
}

/**
 * The art of a spot with its booking status.
 *
 * Classic kit (`halo` style, ADR 0012): the halo (`data-halo`, so focus
 * styles of the parent can target it) is a tinted disc or rounded square
 * larger than the art (HALO_SCALE x the art box, which is already
 * SPOT_ART_SCALE x the footprint); the status lives in this ring and tint,
 * never in the illustration.
 *
 * Modern kit (`fill` style, ADR 0024): the art paints its status part with
 * `--beach-state`, set here to the status token, so the umbrella itself is
 * the status colour. An outline ring (`data-ring`, transparent fill: the hit area)
 * appears for selection, search matches, hover and keyboard focus (a
 * background-coloured band under a dark ring, visible on any fill). A
 * literal `color` (editor zone colours, panel overrides) cannot reach a CSS
 * variable without inline styles, so it is drawn as a classic halo.
 */
export const SpotArt = memo(function SpotArt(props: SpotArtProps) {
  const kit = useMapKit();
  if (mapKitInfo(kit).style === 'fill' && !props.color) return <FilledSpotArt {...props} />;
  return <HaloSpotArt {...props} />;
});

function HaloSpotArt({
  x,
  y,
  shape,
  rotation,
  asset,
  status,
  color,
  tint = 'strong',
  emphasis,
  muted,
  haloClassName,
}: SpotArtProps) {
  const S = spotArtSize(shape);
  const classes = status ? STATUS_CLASSES[status] : undefined;
  const haloClass = cn(
    !color && classes?.fill,
    emphasis ? 'stroke-map-spot-selected' : !color && classes?.stroke,
    haloClassName,
  );
  // Free spots (faint) get a whisper of tint and no ring, so that the
  // states that need attention stand out on a busy day.
  const fillOpacity = tint === 'strong' ? 0.42 : tint === 'outline' ? 0.18 : 0.16;
  const strokeWidth = emphasis ? S * 0.07 : tint === 'faint' ? 0 : S * 0.035;
  const dash = muted ? `${S * 0.06} ${S * 0.05}` : undefined;
  const isRect = shape.type === 'rect';
  // The art is drawn in an S x S box whatever the footprint, so the halo is too.
  const halo = S * HALO_SCALE;
  return (
    <>
      {isRect ? (
        <rect
          data-halo=""
          x={x - halo / 2}
          y={y - halo / 2}
          width={halo}
          height={halo}
          rx={halo * 0.22}
          transform={rotate(rotation, x, y)}
          className={haloClass}
          fill={color}
          stroke={emphasis ? undefined : color}
          fillOpacity={fillOpacity}
          strokeWidth={strokeWidth}
          strokeDasharray={dash}
        />
      ) : (
        <circle
          data-halo=""
          cx={x}
          cy={y}
          r={halo / 2}
          className={haloClass}
          fill={color}
          stroke={emphasis ? undefined : color}
          fillOpacity={fillOpacity}
          strokeWidth={strokeWidth}
          strokeDasharray={dash}
        />
      )}
      <g transform={rotate(rotation, x, y)} className={cn(muted && 'opacity-40 grayscale')}>
        {asset ? (
          <KitUse asset={asset} x={x - S / 2} y={y - S / 2} size={S} />
        ) : (
          <PlainFootprint
            x={x}
            y={y}
            shape={shape}
            className="fill-beach-canvas stroke-beach-ink"
          />
        )}
      </g>
      {status === 'blocked' && <BlockedBar x={x} y={y} size={S} />}
    </>
  );
}

function FilledSpotArt({
  x,
  y,
  shape,
  rotation,
  asset,
  status = 'free',
  emphasis,
  muted,
  haloClassName,
}: SpotArtProps) {
  const S = spotArtSize(shape);
  const classes = STATUS_CLASSES[status];
  const isRect = shape.type === 'rect';
  const ring = S * RING_SCALE;
  const dash = muted && !emphasis ? `${S * 0.05} ${S * 0.04}` : undefined;
  // The first ring is also the hit area (transparent fill, like the classic
  // halo): the art and the badges let pointer events through.
  const ringShape = (className: string, strokeWidth: number, target: boolean) =>
    isRect ? (
      <rect
        data-ring={target ? '' : undefined}
        fill={target ? 'transparent' : 'none'}
        pointerEvents={target ? undefined : 'none'}
        strokeDasharray={dash}
        x={x - ring / 2}
        y={y - ring / 2}
        width={ring}
        height={ring}
        rx={ring * 0.22}
        transform={rotate(rotation, x, y)}
        className={className}
        strokeWidth={strokeWidth}
      />
    ) : (
      <circle
        data-ring={target ? '' : undefined}
        fill={target ? 'transparent' : 'none'}
        pointerEvents={target ? undefined : 'none'}
        strokeDasharray={dash}
        cx={x}
        cy={y}
        r={ring / 2}
        className={className}
        strokeWidth={strokeWidth}
      />
    );
  return (
    <>
      {/* The outline band the parent's hover and focus styles target ([data-ring]). */}
      {ringShape(
        cn(emphasis ? 'stroke-background' : 'stroke-transparent', haloClassName),
        emphasis ? S * 0.12 : S * 0.08,
        true,
      )}
      {emphasis && ringShape('stroke-map-spot-selected', S * 0.055, false)}
      <g
        transform={rotate(rotation, x, y)}
        data-state={status}
        className={cn(classes.state, muted && 'opacity-40 grayscale')}
      >
        {asset ? (
          <KitUse asset={asset} x={x - S / 2} y={y - S / 2} size={S} />
        ) : (
          <PlainFootprint
            x={x}
            y={y}
            shape={shape}
            className={cn(classes.fill, 'stroke-beach-ink')}
          />
        )}
      </g>
      {status === 'blocked' && <BlockedBar x={x} y={y} size={S} />}
    </>
  );
}

/** The footprint itself, for custom spots without an illustration. */
function PlainFootprint({
  x,
  y,
  shape,
  className,
}: {
  x: number;
  y: number;
  shape: Shape;
  className: string;
}) {
  return shape.type === 'rect' ? (
    <rect
      x={x - shape.w / 2}
      y={y - shape.h / 2}
      width={shape.w}
      height={shape.h}
      rx={Math.min(shape.w, shape.h) * 0.18}
      className={className}
      strokeWidth={Math.min(shape.w, shape.h) * 0.05}
    />
  ) : (
    <circle cx={x} cy={y} r={shape.w * 0.42} className={className} strokeWidth={shape.w * 0.05} />
  );
}

/**
 * Diagonal bar across a blocked spot: a cue that does not rely on colour
 * (grey reads like "free" or "unavailable" for many colour-blind users and on
 * greyscale tablets), drawn by the app over the art, never inside it. A
 * background-coloured band under a foreground line keeps it visible on any
 * art and in both themes.
 */
export function BlockedBar({ x, y, size }: { x: number; y: number; size: number }) {
  const h = size * 0.3;
  const d = `M ${x - h} ${y - h} L ${x + h} ${y + h}`;
  return (
    <g data-blocked-bar="" pointerEvents="none" aria-hidden="true" strokeLinecap="round">
      <path d={d} className="fill-none stroke-background" strokeWidth={size * 0.13} />
      <path d={d} className="fill-none stroke-foreground" strokeWidth={size * 0.06} />
    </g>
  );
}

export interface StatusBadgeProps {
  status: SpotStatus;
  /** Centre of the badge. */
  x: number;
  y: number;
  r: number;
  /** Literal colour override (panel theme). */
  color?: string;
}

/** Corner badge with a glyph for statuses that must not rely on colour alone. */
export function StatusBadge({ status, x, y, r, color }: StatusBadgeProps) {
  const glyph = STATUS_BADGES[status];
  if (!glyph) return null;
  const c = STATUS_CLASSES[status];
  return (
    <g data-badge={status} pointerEvents="none" aria-hidden="true">
      <circle
        cx={x}
        cy={y}
        r={r}
        fill={color}
        className={cn(!color && c.fill, 'stroke-background')}
        strokeWidth={r * 0.28}
      />
      <use
        href={`#${badgeSymbolId(glyph)}`}
        x={x - r * 0.72}
        y={y - r * 0.72}
        width={r * 1.44}
        height={r * 1.44}
        className={color ? 'text-white' : c.fg}
      />
    </g>
  );
}

export interface MapLabelProps {
  x: number;
  y: number;
  text: string;
  fontSize: number;
  className?: string;
  /** Muted label (inactive spot). */
  muted?: boolean;
  /** `code` (default): bold tabular spot code; `caption`: a name, regular weight. */
  variant?: 'code' | 'caption';
}

/** A label on a soft pill, readable over any art, in light and dark mode. */
export function MapLabel({
  x,
  y,
  text,
  fontSize,
  className,
  muted,
  variant = 'code',
}: MapLabelProps) {
  const w = labelPillWidth(
    graphemeCount(text),
    fontSize,
    variant === 'caption' ? CAPTION_CHAR_EM : undefined,
  );
  const h = fontSize * 1.32;
  return (
    <g
      pointerEvents="none"
      aria-hidden="true"
      data-map-label={variant}
      className={cn(muted && 'opacity-60', className)}
    >
      <rect
        x={x - w / 2}
        y={y - h / 2}
        width={w}
        height={h}
        rx={h / 2}
        className="fill-background/90 stroke-foreground/15"
        strokeWidth={fontSize * 0.08}
      />
      <text
        x={x}
        y={y}
        textAnchor="middle"
        dominantBaseline="central"
        fontSize={fontSize}
        fontWeight={variant === 'caption' ? 500 : 600}
        className={cn('fill-foreground', variant === 'code' && 'tabular-nums')}
      >
        {text}
      </text>
    </g>
  );
}

export interface DecorationArtProps {
  asset: string;
  x: number;
  y: number;
  shape: Shape;
  rotation?: number | null;
  variant?: string | null;
}

/**
 * The art of a decoration in its w x h box: modular tiles (walkways) are
 * repeated along the long side at a constant scale, every other asset is
 * drawn once, centred, in the largest square that fits.
 */
export const DecorationArt = memo(function DecorationArt({
  asset,
  x,
  y,
  shape,
  rotation,
  variant,
}: DecorationArtProps) {
  const w = shape.w;
  const h = shape.type === 'circle' ? shape.w : shape.h;
  let art;
  if (isTileAsset(asset)) {
    const t = tileLayout(asset, x, y, shape);
    art = t.origins.map((o, i) => <KitUse key={i} asset={t.asset} x={o.x} y={o.y} size={t.side} />);
  } else {
    const side = Math.min(w, h);
    art = <KitUse asset={asset} x={x - side / 2} y={y - side / 2} size={side} />;
  }
  return (
    <g transform={rotate(rotation, x, y)} className={flagClass(variant)}>
      {art}
    </g>
  );
});

export interface KitThumbnailProps {
  asset: string;
  /** Sizing classes (e.g. size-12). */
  className?: string;
  variant?: string | null;
}

/**
 * A kit illustration as a standalone icon (pickers, lists): an <svg> with a
 * <use> of the shared sprite, so a grid of 100 thumbnails adds no drawing
 * copies. Render a <MapKitDefs ids> with the same ids next to it.
 */
export function KitThumbnail({ asset, className, variant }: KitThumbnailProps) {
  const kit = useMapKit();
  return (
    <svg
      viewBox="0 0 120 120"
      aria-hidden="true"
      focusable="false"
      className={cn('shrink-0', flagClass(variant), className)}
    >
      <use href={`#${kitSymbolId(kit, asset)}`} width={120} height={120} />
    </svg>
  );
}
