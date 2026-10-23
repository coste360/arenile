import { useEffect, useState, type RefObject } from 'react';
import type { ViewBox } from './geometry';

/** Smallest readable label on screen (CSS px), spots and decorations. */
export const MIN_LABEL_PX = 9;

/**
 * Map units per CSS pixel of an <svg> with `viewBox` and "xMidYMid meet",
 * tracked with a ResizeObserver. 1 where the element cannot be measured
 * (tests, before mount), so sizes fall back to the map-unit defaults.
 */
export function useUnitsPerPixel(svgRef: RefObject<SVGSVGElement | null>, vb: ViewBox): number {
  const [upp, setUpp] = useState(1);
  useEffect(() => {
    const svg = svgRef.current;
    if (!svg || typeof ResizeObserver === 'undefined') return;
    const measure = () => {
      const w = svg.clientWidth;
      const h = svg.clientHeight;
      if (!w || !h) return;
      setUpp(Math.max(vb.width / w, vb.height / h));
    };
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(svg);
    return () => ro.disconnect();
  }, [svgRef, vb]);
  return upp;
}

/**
 * Rounds map units per pixel to a step of about 19% (quarter powers of two):
 * label and badge sizes change in visible steps only, so a wheel or pinch
 * zoom step does not re-render every memoised spot of a 1,000-spot map.
 */
export function quantiseUnitsPerPixel(upp: number): number {
  if (!(upp > 0) || !Number.isFinite(upp)) return 1;
  return 2 ** (Math.round(Math.log2(upp) * 4) / 4);
}

/**
 * Font size (map units) of a label inside a shape of `size` units: the
 * proportional size, raised to MIN_LABEL_PX on screen; 0 when even that
 * would not fit the shape (the label stays in the accessible name and tooltip).
 * `unitsPerPixel` already accounts for the zoom of the drawing.
 */
export function labelFontSize(
  size: number,
  ratio: number,
  unitsPerPixel: number,
  maxRatio = 0.8,
): number {
  const wanted = Math.max(size * ratio, MIN_LABEL_PX * unitsPerPixel);
  return wanted > size * maxRatio ? 0 : wanted;
}

/** Enlarges a viewBox by `margin` units on every side (room for the sea label and strokes). */
export function padViewBox(vb: ViewBox, margin: number): ViewBox {
  return {
    x: vb.x - margin,
    y: vb.y - margin,
    width: vb.width + margin * 2,
    height: vb.height + margin * 2,
  };
}
