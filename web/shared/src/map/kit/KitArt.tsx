import { memo } from 'react';
import { cn } from '../../lib/utils';
import { STATUS_CLASSES } from '../colors';
import type { Shape, SpotStatus } from '../types';
import { kitSymbolId } from './assets';
import { isTileAsset, spotArtSize, tileLayout } from './geometry';
import { STATUS_BADGES, badgeSymbolId } from './status';

/** Flag colour utilities (globals.css @utility), written out for Tailwind. */
const FLAG_CLASSES: Record<string, string> = {
  green: 'beach-flag-green',
  yellow: 'beach-flag-yellow',
  red: 'beach-flag-red',
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

/** One kit illustration: a <use> of the shared sprite symbol (no inline SVG copy). */
export function KitUse({ asset, x, y, size, className }: KitUseProps) {
  return (
    <use
      href={`#${kitSymbolId(asset)}`}
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
  /** Halo colour: a status (theme token classes) or a literal colour. */
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
 * The art of a spot over its status halo. The halo (first child, so focus
 * styles of the parent can target it) is a tinted disc or rounded square
 * slightly larger than the footprint: booking status lives here, never in
 * the illustration, so the art stays the same and readable in every state.
 */
export const SpotArt = memo(function SpotArt({
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
  const haloW = (isRect ? shape.w : shape.w) * 1.12;
  const haloH = (isRect ? shape.h : shape.w) * 1.12;
  return (
    <>
      {isRect ? (
        <rect
          data-halo=""
          x={x - haloW / 2}
          y={y - haloH / 2}
          width={haloW}
          height={haloH}
          rx={Math.min(haloW, haloH) * 0.22}
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
          r={haloW / 2}
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
        ) : isRect ? (
          <rect
            x={x - shape.w / 2}
            y={y - shape.h / 2}
            width={shape.w}
            height={shape.h}
            rx={Math.min(shape.w, shape.h) * 0.18}
            className="fill-beach-canvas stroke-beach-ink"
            strokeWidth={Math.min(shape.w, shape.h) * 0.05}
          />
        ) : (
          <circle
            cx={x}
            cy={y}
            r={shape.w * 0.42}
            className="fill-beach-canvas stroke-beach-ink"
            strokeWidth={shape.w * 0.05}
          />
        )}
      </g>
    </>
  );
});

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
}

/** A label on a soft pill, readable over any art, in light and dark mode. */
export function MapLabel({ x, y, text, fontSize, className, muted }: MapLabelProps) {
  const w = Math.max(text.length * fontSize * 0.62 + fontSize * 0.7, fontSize * 1.6);
  const h = fontSize * 1.32;
  return (
    <g pointerEvents="none" aria-hidden="true" className={cn(muted && 'opacity-60', className)}>
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
        fontWeight={600}
        className="fill-foreground tabular-nums"
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
  return (
    <svg
      viewBox="0 0 120 120"
      aria-hidden="true"
      focusable="false"
      className={cn('shrink-0', flagClass(variant), className)}
    >
      <use href={`#${kitSymbolId(asset)}`} width={120} height={120} />
    </svg>
  );
}
