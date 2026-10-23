import { memo, useId } from 'react';
import { seaRect } from '../geometry';
import type { MapGeometry, SeaSide } from '../types';

export interface MapBackdropProps {
  geometry: Pick<MapGeometry, 'width' | 'height' | 'sea_side'>;
  /** Visible name of the sea band ("Mare"); empty hides it. */
  seaLabel?: string;
}

/** Wavelength and amplitude of the shoreline, in map units. */
const WAVE = 96;
const AMPLITUDE = 4;

/**
 * Wavy shoreline along an edge of `length`, in a local frame where the edge
 * runs along +x from 0 and the sea lies towards +y.
 */
function shorePath(length: number, depth: number, offset = 0): { fill: string; line: string } {
  const n = Math.max(1, Math.round(length / WAVE));
  const step = length / n;
  let line = `M0 ${offset}`;
  for (let i = 0; i < n; i++) {
    const x0 = i * step;
    line += ` Q${x0 + step / 4} ${offset - AMPLITUDE} ${x0 + step / 2} ${offset} T${x0 + step} ${offset}`;
  }
  return { line, fill: `${line} L${length} ${depth} L0 ${depth}Z` };
}

const FRAME: Record<SeaSide, (w: number, h: number) => { transform: string; length: number }> = {
  bottom: (w, h) => ({ transform: `translate(0 ${h})`, length: w }),
  top: (w) => ({ transform: `translate(${w} 0) rotate(180)`, length: w }),
  left: (_w, h) => ({ transform: 'rotate(90)', length: h }),
  right: (w, h) => ({ transform: `translate(${w} ${h}) rotate(-90)`, length: h }),
};

/**
 * Sand, sea band and shoreline of a map, in the soft look of the kit
 * (sabbia / mare / linea_costa): flat tokens with a faint grain and a few
 * waves, so the illustrations stay the protagonists. Colours are the
 * --beach-* theme tokens, light and dark.
 */
export const MapBackdrop = memo(function MapBackdrop({ geometry, seaLabel }: MapBackdropProps) {
  const id = useId().replace(/:/g, '');
  const sea = seaRect(geometry);
  const { width, height, sea_side } = geometry;
  const depth = Math.min(sea.width, sea.height);
  const frame = FRAME[sea_side](width, height);
  const shore = shorePath(frame.length, depth);
  const foam = shorePath(frame.length, depth, depth * 0.34);
  const vertical = sea_side === 'left' || sea_side === 'right';
  const cx = sea.x + sea.width / 2;
  const cy = sea.y + sea.height / 2;
  return (
    <g aria-hidden="true" data-sea-side={sea_side} pointerEvents="none">
      <defs>
        <pattern
          id={`${id}-grain`}
          width={120}
          height={120}
          patternUnits="userSpaceOnUse"
          patternTransform="scale(1.4)"
        >
          {[
            [25, 38, 1.6],
            [52, 20, 0.9],
            [80, 45, 1.3],
            [36, 84, 1.1],
            [82, 86, 1.6],
            [104, 14, 0.9],
            [10, 108, 1.2],
          ].map(([x, y, r]) => (
            <circle key={`${x}-${y}`} cx={x} cy={y} r={r} className="fill-beach-wood" />
          ))}
        </pattern>
        <pattern
          id={`${id}-waves`}
          width={120}
          height={120}
          patternUnits="userSpaceOnUse"
          patternTransform={`${vertical ? 'rotate(90) ' : ''}scale(0.7)`}
        >
          <path
            d="M12 37Q26 26 40 37T68 37T96 37M72 92Q86 81 100 92T128 92"
            fill="none"
            className="stroke-beach-water"
            strokeWidth={2.4}
            strokeLinecap="round"
          />
        </pattern>
      </defs>
      <rect
        data-background="sand"
        pointerEvents="all"
        x={0}
        y={0}
        width={width}
        height={height}
        className="fill-beach-sand-soft"
      />
      <rect x={0} y={0} width={width} height={height} fill={`url(#${id}-grain)`} opacity={0.35} />
      <rect
        data-background="sea"
        pointerEvents="all"
        x={sea.x}
        y={sea.y}
        width={sea.width}
        height={sea.height}
        className="fill-beach-water-soft"
      />
      <g transform={frame.transform}>
        <path d={shore.fill} className="fill-beach-water-soft" />
        <path
          d={foam.line}
          fill="none"
          className="stroke-beach-water"
          strokeWidth={1.2}
          opacity={0.45}
        />
        <path d={shore.line} fill="none" className="stroke-beach-water" strokeWidth={2.4} />
      </g>
      <rect
        x={sea.x}
        y={sea.y}
        width={sea.width}
        height={sea.height}
        fill={`url(#${id}-waves)`}
        opacity={0.55}
      />
      {seaLabel && (
        <text
          x={cx}
          y={cy}
          textAnchor="middle"
          dominantBaseline="central"
          fontSize={depth * 0.32}
          fontWeight={600}
          letterSpacing="0.12em"
          className="fill-map-sea-label"
          opacity={0.55}
          transform={vertical ? `rotate(-90 ${cx} ${cy})` : undefined}
        >
          {seaLabel}
        </text>
      )}
    </g>
  );
});
