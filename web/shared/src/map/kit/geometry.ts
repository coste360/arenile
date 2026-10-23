import type { Shape } from '../types';
import { KIT_BY_ID } from './assets';

/**
 * How much larger than its footprint (shape) a spot is drawn: the kit art
 * leaves a transparent margin inside its 120-unit viewBox, so the drawing
 * extends slightly beyond the clickable footprint. Keep it equal to
 * `kitArtScale` in internal/domain/setup/mapkit.go.
 */
export const SPOT_ART_SCALE = 1.3;

/**
 * Side of a modular walkway tile in map units: the kit's 120-unit viewBox
 * at half scale. The planks the kit draws are narrower than the viewBox, so
 * a run reads as a strip inside its tiles; the side is the module the pieces
 * join on, not the width of the planks. Every tile is drawn at this scale.
 */
export const KIT_TILE = 60;

/** Side of the square the art of a spot is drawn in. */
export function spotArtSize(shape: Shape): number {
  const w = shape.w;
  const h = shape.type === 'circle' ? shape.w : shape.h;
  return Math.max(w, h) * SPOT_ART_SCALE;
}

export function isTileAsset(id: string | null | undefined): boolean {
  return !!id && KIT_BY_ID.get(id)?.tile === true;
}

export interface TilePlacement {
  /** Asset of each tile (straight pieces follow the long side). */
  asset: string;
  /** Top-left corners, before the element rotation. */
  origins: { x: number; y: number }[];
  side: number;
}

/**
 * Tiles of a modular piece drawn in a w x h rectangle centred on (x, y): the
 * tile side is the short side, repeated along the long one. A straight piece
 * whose orientation does not match the long side is swapped.
 */
export function tileLayout(asset: string, x: number, y: number, shape: Shape): TilePlacement {
  const w = shape.w;
  const h = shape.type === 'circle' ? shape.w : shape.h;
  const side = Math.min(w, h);
  const n = Math.max(1, Math.round(Math.max(w, h) / side));
  const horizontal = w >= h;
  let id = asset;
  if (n > 1 && asset === 'passerella_verticale' && horizontal) id = 'passerella_orizzontale';
  if (n > 1 && asset === 'passerella_orizzontale' && !horizontal) id = 'passerella_verticale';
  const len = n * side;
  const x0 = horizontal ? x - len / 2 : x - side / 2;
  const y0 = horizontal ? y - side / 2 : y - len / 2;
  const origins = Array.from({ length: n }, (_, i) =>
    horizontal ? { x: x0 + i * side, y: y0 } : { x: x0, y: y0 + i * side },
  );
  return { asset: id, origins, side };
}

/** Rounds a length to a whole number of tiles (at least one). */
export function snapTileLength(v: number): number {
  return Math.max(1, Math.round(v / KIT_TILE)) * KIT_TILE;
}

/**
 * Centre coordinate of a tile run of `length` so that its edge lies on the
 * tile grid. With `from`, a move that would round back to the same place
 * (an arrow-key nudge) moves one whole tile in its direction instead.
 */
export function snapTileCentre(v: number, length: number, from?: number): number {
  const half = length / 2;
  let edge = Math.round((v - half) / KIT_TILE) * KIT_TILE;
  if (from !== undefined && v !== from) {
    const fromEdge = Math.round((from - half) / KIT_TILE) * KIT_TILE;
    if (edge === fromEdge) edge = fromEdge + Math.sign(v - from) * KIT_TILE;
  }
  return edge + half;
}
