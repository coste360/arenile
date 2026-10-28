import { cn } from '@arenile/shared/lib/utils';
import {
  BADGE_GLYPHS,
  STATUS_BADGES,
  STATUS_CLASSES,
  badgeGlyphStroke,
  type SpotStatus,
} from '@arenile/shared/map';

export interface StatusGlyphProps {
  status: SpotStatus;
  className?: string;
}

/**
 * The status swatch with its map glyph (same glyphs as the map badges, so a
 * status never depends on colour alone). Decorative: callers name the status
 * in text or in the accessible name.
 */
export function StatusGlyph({ status, className }: StatusGlyphProps) {
  const glyph = STATUS_BADGES[status];
  return (
    <svg
      aria-hidden="true"
      viewBox="0 0 24 24"
      data-status-glyph={status}
      className={cn(
        'size-4 shrink-0 rounded-full ring-1 ring-foreground/20',
        STATUS_CLASSES[status].bg,
        STATUS_CLASSES[status].fg,
        className,
      )}
    >
      {glyph && (
        <path
          d={BADGE_GLYPHS[glyph]}
          transform="translate(3.6 3.6) scale(0.7)"
          fill={glyph === 'star' ? 'currentColor' : 'none'}
          stroke="currentColor"
          strokeWidth={badgeGlyphStroke(glyph)}
          strokeLinecap="round"
          strokeLinejoin="round"
        />
      )}
    </svg>
  );
}
