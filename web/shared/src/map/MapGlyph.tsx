import type { LucideIcon } from 'lucide-react';

export interface MapGlyphProps {
  icon: LucideIcon;
  /** Centre in logical map units. */
  x: number;
  y: number;
  size: number;
  className?: string;
}

/** A lucide glyph drawn inside the map SVG, centred on (x, y). */
export function MapGlyph({ icon: Icon, x, y, size, className }: MapGlyphProps) {
  return (
    <Icon
      x={x - size / 2}
      y={y - size / 2}
      width={size}
      height={size}
      className={className}
      aria-hidden="true"
      focusable="false"
      pointerEvents="none"
    />
  );
}
