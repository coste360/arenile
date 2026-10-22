import type { Shape } from './types';

export interface ShapeElementProps {
  shape: Shape;
  /** Centre in logical map units. */
  x: number;
  y: number;
  fill: string;
  stroke?: string;
  strokeWidth?: number;
  strokeDasharray?: string;
  opacity?: number;
}

/** SVG primitive for a spot or decoration shape, centred on (x, y). */
export function ShapeElement({
  shape,
  x,
  y,
  fill,
  stroke,
  strokeWidth,
  strokeDasharray,
  opacity,
}: ShapeElementProps) {
  if (shape.type === 'circle') {
    return (
      <circle
        cx={x}
        cy={y}
        r={shape.w / 2}
        fill={fill}
        stroke={stroke}
        strokeWidth={strokeWidth}
        strokeDasharray={strokeDasharray}
        opacity={opacity}
      />
    );
  }
  return (
    <rect
      x={x - shape.w / 2}
      y={y - shape.h / 2}
      width={shape.w}
      height={shape.h}
      rx={Math.min(shape.w, shape.h) * 0.15}
      fill={fill}
      stroke={stroke}
      strokeWidth={strokeWidth}
      strokeDasharray={strokeDasharray}
      opacity={opacity}
    />
  );
}

/** Smallest side of a shape, used to scale labels and strokes. */
export function shapeSize(shape: Shape): number {
  return shape.type === 'circle' ? shape.w : Math.min(shape.w, shape.h);
}
