import type { SpotStatus } from './types';

/**
 * Default status palette as hex values, for contrast computations and for
 * consumers that need a literal colour (e.g. the map editor palette). The
 * same values are the `--status-*` CSS variables of styles/globals.css, which
 * the renderer uses through STATUS_CLASSES (a test keeps them in sync).
 * Colours are always paired with a text label in the legend, never conveyed
 * by colour alone.
 */
export const DEFAULT_STATUS_COLORS: Record<SpotStatus, string> = {
  free: '#22c55e',
  booked_online: '#2563eb',
  booked_desk: '#7c3aed',
  season: '#0f766e',
  season_released: '#5eead4',
  hold: '#f59e0b',
  to_collect: '#dc2626',
  present: '#1e3a8a',
  blocked: '#6b7280',
  renewal_hold: '#db2777',
  unavailable: '#d1d5db',
};

export interface StatusClasses {
  /** SVG fill of the spot shape. */
  fill: string;
  /** SVG fill of the label drawn on the spot. */
  labelFill: string;
  /** Background (legend swatch, badge dot). */
  bg: string;
  /** SVG stroke (halo ring of the kit art). */
  stroke: string;
  /** Text colour of glyphs drawn on the status colour (currentColor of badge glyphs). */
  fg: string;
  /** Name of the CSS variable, for rare non-class uses. */
  cssVar: string;
}

// Written out literally so Tailwind can see every class name.
export const STATUS_CLASSES: Record<SpotStatus, StatusClasses> = {
  free: {
    fill: 'fill-status-free',
    labelFill: 'fill-status-free-foreground',
    bg: 'bg-status-free',
    stroke: 'stroke-status-free',
    fg: 'text-status-free-foreground',
    cssVar: '--status-free',
  },
  booked_online: {
    fill: 'fill-status-booked-online',
    labelFill: 'fill-status-booked-online-foreground',
    bg: 'bg-status-booked-online',
    stroke: 'stroke-status-booked-online',
    fg: 'text-status-booked-online-foreground',
    cssVar: '--status-booked-online',
  },
  booked_desk: {
    fill: 'fill-status-booked-desk',
    labelFill: 'fill-status-booked-desk-foreground',
    bg: 'bg-status-booked-desk',
    stroke: 'stroke-status-booked-desk',
    fg: 'text-status-booked-desk-foreground',
    cssVar: '--status-booked-desk',
  },
  season: {
    fill: 'fill-status-season',
    labelFill: 'fill-status-season-foreground',
    bg: 'bg-status-season',
    stroke: 'stroke-status-season',
    fg: 'text-status-season-foreground',
    cssVar: '--status-season',
  },
  season_released: {
    fill: 'fill-status-season-released',
    labelFill: 'fill-status-season-released-foreground',
    bg: 'bg-status-season-released',
    stroke: 'stroke-status-season-released',
    fg: 'text-status-season-released-foreground',
    cssVar: '--status-season-released',
  },
  hold: {
    fill: 'fill-status-hold',
    labelFill: 'fill-status-hold-foreground',
    bg: 'bg-status-hold',
    stroke: 'stroke-status-hold',
    fg: 'text-status-hold-foreground',
    cssVar: '--status-hold',
  },
  to_collect: {
    fill: 'fill-status-to-collect',
    labelFill: 'fill-status-to-collect-foreground',
    bg: 'bg-status-to-collect',
    stroke: 'stroke-status-to-collect',
    fg: 'text-status-to-collect-foreground',
    cssVar: '--status-to-collect',
  },
  present: {
    fill: 'fill-status-present',
    labelFill: 'fill-status-present-foreground',
    bg: 'bg-status-present',
    stroke: 'stroke-status-present',
    fg: 'text-status-present-foreground',
    cssVar: '--status-present',
  },
  blocked: {
    fill: 'fill-status-blocked',
    labelFill: 'fill-status-blocked-foreground',
    bg: 'bg-status-blocked',
    stroke: 'stroke-status-blocked',
    fg: 'text-status-blocked-foreground',
    cssVar: '--status-blocked',
  },
  renewal_hold: {
    fill: 'fill-status-renewal-hold',
    labelFill: 'fill-status-renewal-hold-foreground',
    bg: 'bg-status-renewal-hold',
    stroke: 'stroke-status-renewal-hold',
    fg: 'text-status-renewal-hold-foreground',
    cssVar: '--status-renewal-hold',
  },
  unavailable: {
    fill: 'fill-status-unavailable',
    labelFill: 'fill-status-unavailable-foreground',
    bg: 'bg-status-unavailable',
    stroke: 'stroke-status-unavailable',
    fg: 'text-status-unavailable-foreground',
    cssVar: '--status-unavailable',
  },
};

/** Light-mode literals of the map surfaces (--map-* variables). */
export const SAND_COLOR = '#f8edd3';
export const SEA_COLOR = '#7cc4e8';
export const DECORATION_COLOR = '#e7dcc2';
