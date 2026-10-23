import { MIN_LABEL_PX } from './labels';
import type { MapSpot } from './types';

/**
 * What the spot labels of the map show (operational map, spec 11.3): the
 * spot code, a caption per spot (who occupies it), or both.
 */
export type MapLabelMode = 'code' | 'customer' | 'both';

export const MAP_LABEL_MODES: readonly MapLabelMode[] = ['code', 'customer', 'both'];

/** Longest caption drawn on the map, ellipsis included (the full text is in the tooltip and accessible name). */
export const MAX_CAPTION_CHARS = 12;

/** Shortest useful truncated caption ("Ros…"): below it the caption is dropped. */
export const MIN_CAPTION_CHARS = 4;

/** Average glyph width of a code label (semibold, tabular digits), in em; MapLabel uses the same. */
export const LABEL_CHAR_EM = 0.62;

/** Average glyph width of a caption (medium weight, mostly lower case), in em. */
export const CAPTION_CHAR_EM = 0.56;

/** Horizontal padding of a label pill, in em. */
export const LABEL_PAD_EM = 0.7;

/** Height of a label pill, in em. */
export const LABEL_HEIGHT_EM = 1.32;

let segmenter: Intl.Segmenter | null | undefined;

function graphemes(text: string): string[] {
  // One segmenter for the module: layouts run for every spot at each zoom step.
  if (segmenter === undefined) {
    segmenter =
      typeof Intl !== 'undefined' && 'Segmenter' in Intl
        ? new Intl.Segmenter(undefined, { granularity: 'grapheme' })
        : null;
  }
  return segmenter ? Array.from(segmenter.segment(text), (s) => s.segment) : Array.from(text);
}

/** Number of user-perceived characters (grapheme clusters) of `text`. */
export function graphemeCount(text: string): number {
  return graphemes(text).length;
}

/**
 * `text` trimmed and cut to `max` characters (grapheme clusters, so accents
 * and emoji are never split), the last one being an ellipsis when cut.
 */
export function truncateLabel(text: string, max = MAX_CAPTION_CHARS): string {
  const t = text.trim().replace(/\s+/g, ' ');
  const g = graphemes(t);
  if (g.length <= max) return t;
  if (max <= 1) return '…';
  return `${g
    .slice(0, max - 1)
    .join('')
    .trimEnd()}…`;
}

/** Width (map units) of a label pill with `chars` characters at `fontSize`. */
export function labelPillWidth(chars: number, fontSize: number, charEm = LABEL_CHAR_EM): number {
  return Math.max(chars * fontSize * charEm + fontSize * LABEL_PAD_EM, fontSize * 1.6);
}

/** Neighbourhood of a spot, computed once per geometry. */
export interface SpotNeighbours {
  /** Nearest spot of the same row on the left / right, if any. */
  left?: string;
  right?: string;
  /** Centre distance to them (map units); Infinity without a neighbour. */
  leftGap: number;
  rightGap: number;
  /** Centre distance to the nearest spot below (map units); Infinity on the last row. */
  belowGap: number;
}

/**
 * Same-row and next-row neighbours of every spot. Two spots share a row
 * when their centres are less than 60% of the smaller art apart vertically,
 * a column likewise horizontally. Quadratic, but run once per geometry
 * (about 10 ms for 1,000 spots), never on pan or zoom.
 */
export function spotNeighbours(
  spots: readonly MapSpot[],
  artSize: (spot: MapSpot) => number,
): Map<string, SpotNeighbours> {
  const out = new Map<string, SpotNeighbours>();
  const sizes = spots.map(artSize);
  for (let i = 0; i < spots.length; i++) {
    const a = spots[i]!;
    const n: SpotNeighbours = { leftGap: Infinity, rightGap: Infinity, belowGap: Infinity };
    for (let j = 0; j < spots.length; j++) {
      if (i === j) continue;
      const b = spots[j]!;
      const tol = Math.min(sizes[i]!, sizes[j]!) * 0.6;
      const dx = b.x - a.x;
      const dy = b.y - a.y;
      if (Math.abs(dy) < tol) {
        if (dx < 0 && -dx < n.leftGap) {
          n.leftGap = -dx;
          n.left = b.id;
        } else if (dx >= 0 && dx < n.rightGap) {
          n.rightGap = dx;
          n.right = b.id;
        }
      } else if (Math.abs(dx) < tol && dy > 0 && dy < n.belowGap) {
        n.belowGap = dy;
      }
    }
    out.set(a.id, n);
  }
  return out;
}

/** Font size (map units) of a caption under an art of `size`: proportional, at least MIN_LABEL_PX on screen. */
export function captionFontSize(size: number, unitsPerPixel: number): number {
  return Math.max(size * 0.18, MIN_LABEL_PX * unitsPerPixel);
}

function median(values: number[]): number {
  if (values.length === 0) return Infinity;
  const v = [...values].sort((a, b) => a - b);
  return v[Math.floor(v.length / 2)]!;
}

/**
 * Whether captions are legible on this map at this zoom, decided once for
 * the whole map so that a row never shows names on some spots only: on the
 * typical spot (median art size and distance to the nearest row neighbour)
 * with captioned neighbours on both sides, at least MIN_CAPTION_CHARS must
 * fit. False → no captions, the zoom hint shows.
 */
export function captionsLegible(
  spots: readonly MapSpot[],
  artSize: (spot: MapSpot) => number,
  neighbours: ReadonlyMap<string, SpotNeighbours>,
  unitsPerPixel: number,
  /** Font size of the code pill drawn above the caption ("Entrambi"), 0 = none. */
  codeFontSize: (size: number) => number = () => 0,
): boolean {
  if (spots.length === 0) return false;
  const size = median(spots.map(artSize));
  const fontSize = captionFontSize(size, unitsPerPixel);
  if (fontSize > size * 0.42) return false;
  const pitches: number[] = [];
  const rows: number[] = [];
  for (const s of spots) {
    const n = neighbours.get(s.id);
    const p = Math.min(n?.leftGap ?? Infinity, n?.rightGap ?? Infinity);
    if (Number.isFinite(p)) pitches.push(p);
    if (n && Number.isFinite(n.belowGap)) rows.push(n.belowGap);
  }
  // Vertical: the pill must fit between the art (or its code pill) and the next row.
  const below = median(rows);
  if (Number.isFinite(below)) {
    const margin = fontSize * 0.25;
    const code = codeFontSize(size);
    const top = Math.max(size * 0.4, code > 0 ? size * 0.36 + code * 0.66 + margin * 0.5 : 0);
    if (below - size * 0.4 - top < fontSize * LABEL_HEIGHT_EM + margin) return false;
  }
  const pitch = median(pitches);
  if (!Number.isFinite(pitch)) return true;
  const room = pitch - fontSize * 0.5;
  const chars = Math.floor((room - fontSize * LABEL_PAD_EM) / (fontSize * CAPTION_CHAR_EM));
  return chars >= MIN_CAPTION_CHARS;
}

/** Where a caption pill is drawn (map units). */
export interface CaptionLayout {
  text: string;
  x: number;
  y: number;
  fontSize: number;
}

/**
 * Lays out the caption of `spot` under its art, in the free band before the
 * next row, so that captions never cover a neighbour or each other:
 *
 * - the pill may use half the distance to a captioned neighbour on each
 *   side, the whole distance to a neighbour without caption (a free spot),
 *   and is shifted towards the free side when it needs the room;
 * - the text is truncated to what fits (at most MAX_CAPTION_CHARS);
 * - undefined when the label would be smaller than MIN_LABEL_PX on screen,
 *   when the band is too low for the pill, or when fewer than
 *   MIN_CAPTION_CHARS fit: the caption stays in the tooltip and the
 *   accessible name, and the map shows the zoom hint.
 *
 * Pure: depends on the quantised `unitsPerPixel` only, so pan and zoom
 * steps inside one quantum reuse the memoised layout.
 */
export function layoutCaption(
  spot: MapSpot,
  text: string,
  size: number,
  neighbours: SpotNeighbours | undefined,
  hasCaption: (id: string) => boolean,
  unitsPerPixel: number,
  /** Lowest point already used under the spot (bottom of its code pill), if any. */
  minTop?: number,
): CaptionLayout | undefined {
  const fontSize = captionFontSize(size, unitsPerPixel);
  if (fontSize > size * 0.42) return undefined;
  const height = fontSize * LABEL_HEIGHT_EM;
  const margin = fontSize * 0.25;
  // Vertical band: from the lower edge of the art (its last 10% is the
  // transparent margin of the kit illustrations) or the code pill, to the
  // top of the next row.
  const top = Math.max(spot.y + size * 0.4, (minTop ?? -Infinity) + margin * 0.5);
  const below = neighbours?.belowGap ?? Infinity;
  const bottom = Number.isFinite(below) ? spot.y + below - size * 0.4 : Infinity;
  if (bottom - top < height + margin) return undefined;
  const y = Number.isFinite(bottom) ? (top + bottom) / 2 : top + height / 2 + margin;

  const side = (id: string | undefined, gap: number) =>
    !id || !Number.isFinite(gap)
      ? size * 1.2
      : hasCaption(id)
        ? gap / 2 - margin
        : gap - size * 0.5 - margin;
  const left = Math.max(side(neighbours?.left, neighbours?.leftGap ?? Infinity), 0);
  const right = Math.max(side(neighbours?.right, neighbours?.rightGap ?? Infinity), 0);
  const room = left + right;
  const chars = Math.min(
    MAX_CAPTION_CHARS,
    Math.floor((room - fontSize * LABEL_PAD_EM) / (fontSize * CAPTION_CHAR_EM)),
  );
  if (chars < MIN_CAPTION_CHARS && graphemes(text.trim()).length > Math.max(chars, 0))
    return undefined;
  const shown = truncateLabel(text, Math.max(chars, 1));
  const w = labelPillWidth(graphemes(shown).length, fontSize, CAPTION_CHAR_EM);
  // Centred on the spot when possible, shifted towards the free side otherwise.
  let x = spot.x;
  if (w / 2 > right) x -= w / 2 - right;
  else if (w / 2 > left) x += w / 2 - left;
  return { text: shown, x, y, fontSize };
}
