import type { MapGeometry, SeaSide } from './types';

export interface ViewBox {
  x: number;
  y: number;
  width: number;
  height: number;
}

export interface Transform {
  x: number;
  y: number;
  scale: number;
}

export const IDENTITY: Transform = { x: 0, y: 0, scale: 1 };
export const MIN_SCALE = 0.5;
export const MAX_SCALE = 8;

/** Thickness of the sea band drawn outside the sand area. */
export function seaDepth(geometry: Pick<MapGeometry, 'width' | 'height'>): number {
  return Math.max(24, Math.round(Math.min(geometry.width, geometry.height) * 0.12));
}

/** viewBox covering the map plus the sea band on `sea_side`. */
export function computeViewBox(
  geometry: Pick<MapGeometry, 'width' | 'height' | 'sea_side'>,
): ViewBox {
  const depth = seaDepth(geometry);
  const vb: ViewBox = { x: 0, y: 0, width: geometry.width, height: geometry.height };
  switch (geometry.sea_side) {
    case 'top':
      return { ...vb, y: -depth, height: vb.height + depth };
    case 'bottom':
      return { ...vb, height: vb.height + depth };
    case 'left':
      return { ...vb, x: -depth, width: vb.width + depth };
    case 'right':
      return { ...vb, width: vb.width + depth };
  }
}

/** Rectangle of the sea band in map coordinates. */
export function seaRect(geometry: Pick<MapGeometry, 'width' | 'height' | 'sea_side'>): ViewBox {
  const depth = seaDepth(geometry);
  const { width, height } = geometry;
  const rects: Record<SeaSide, ViewBox> = {
    top: { x: 0, y: -depth, width, height: depth },
    bottom: { x: 0, y: height, width, height: depth },
    left: { x: -depth, y: 0, width: depth, height },
    right: { x: width, y: 0, width: depth, height },
  };
  return rects[geometry.sea_side];
}

export function clampScale(scale: number): number {
  return Math.min(MAX_SCALE, Math.max(MIN_SCALE, scale));
}

/**
 * Zooms by `factor` keeping `point` (in viewBox coordinates) fixed on screen.
 */
export function zoomAt(t: Transform, point: { x: number; y: number }, factor: number): Transform {
  const scale = clampScale(t.scale * factor);
  const k = scale / t.scale;
  return {
    scale,
    x: point.x - (point.x - t.x) * k,
    y: point.y - (point.y - t.y) * k,
  };
}

/** SVG transform attribute for a pan/zoom state. */
export function transformAttr(t: Transform): string {
  return `translate(${t.x} ${t.y}) scale(${t.scale})`;
}
