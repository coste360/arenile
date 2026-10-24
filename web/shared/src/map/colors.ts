import type { SpotStatus } from './types';

/**
 * Default status palette (light scheme) as hex values, for contrast
 * computations and for consumers that need a literal colour (e.g. the map
 * editor palette). The same values are the `--status-*` CSS variables of
 * styles/globals.css, which the renderer uses through STATUS_CLASSES (a test
 * keeps them in sync). Colours are always paired with a text label in the
 * legend and a badge glyph on the map, never conveyed by colour alone.
 */
export const DEFAULT_STATUS_COLORS: Record<SpotStatus, string> = {
  free: '#71717a',
  booked_online: '#0284c7',
  booked_desk: '#7c3aed',
  season: '#0f766e',
  season_released: '#5eead4',
  hold: '#b45309',
  to_collect: '#dc2626',
  present: '#1e3a8a',
  blocked: '#374151',
  renewal_hold: '#db2777',
  unavailable: '#d1d5db',
};

/**
 * Dark-scheme palette: the light values lifted where they would not reach
 * 3:1 on the dark sand (the modern kit fills the spot body with them); free
 * stays a neutral grey, unavailable a faint one.
 */
export const DARK_STATUS_COLORS: Record<SpotStatus, string> = {
  ...DEFAULT_STATUS_COLORS,
  free: '#d4d4d8',
  booked_online: '#bae6fd',
  booked_desk: '#a78bfa',
  season: '#14b8a6',
  hold: '#f59e0b',
  to_collect: '#f87171',
  present: '#818cf8',
  blocked: '#8e8e96',
  renewal_hold: '#f472b6',
  unavailable: '#52525b',
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
  /**
   * Sets `--beach-state` to the status colour (Tailwind arbitrary property on
   * the theme token): the modern kit paints the status part of its art with it.
   */
  state: string;
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
    state: '[--beach-state:var(--status-free)]',
    cssVar: '--status-free',
  },
  booked_online: {
    fill: 'fill-status-booked-online',
    labelFill: 'fill-status-booked-online-foreground',
    bg: 'bg-status-booked-online',
    stroke: 'stroke-status-booked-online',
    fg: 'text-status-booked-online-foreground',
    state: '[--beach-state:var(--status-booked-online)]',
    cssVar: '--status-booked-online',
  },
  booked_desk: {
    fill: 'fill-status-booked-desk',
    labelFill: 'fill-status-booked-desk-foreground',
    bg: 'bg-status-booked-desk',
    stroke: 'stroke-status-booked-desk',
    fg: 'text-status-booked-desk-foreground',
    state: '[--beach-state:var(--status-booked-desk)]',
    cssVar: '--status-booked-desk',
  },
  season: {
    fill: 'fill-status-season',
    labelFill: 'fill-status-season-foreground',
    bg: 'bg-status-season',
    stroke: 'stroke-status-season',
    fg: 'text-status-season-foreground',
    state: '[--beach-state:var(--status-season)]',
    cssVar: '--status-season',
  },
  season_released: {
    fill: 'fill-status-season-released',
    labelFill: 'fill-status-season-released-foreground',
    bg: 'bg-status-season-released',
    stroke: 'stroke-status-season-released',
    fg: 'text-status-season-released-foreground',
    state: '[--beach-state:var(--status-season-released)]',
    cssVar: '--status-season-released',
  },
  hold: {
    fill: 'fill-status-hold',
    labelFill: 'fill-status-hold-foreground',
    bg: 'bg-status-hold',
    stroke: 'stroke-status-hold',
    fg: 'text-status-hold-foreground',
    state: '[--beach-state:var(--status-hold)]',
    cssVar: '--status-hold',
  },
  to_collect: {
    fill: 'fill-status-to-collect',
    labelFill: 'fill-status-to-collect-foreground',
    bg: 'bg-status-to-collect',
    stroke: 'stroke-status-to-collect',
    fg: 'text-status-to-collect-foreground',
    state: '[--beach-state:var(--status-to-collect)]',
    cssVar: '--status-to-collect',
  },
  present: {
    fill: 'fill-status-present',
    labelFill: 'fill-status-present-foreground',
    bg: 'bg-status-present',
    stroke: 'stroke-status-present',
    fg: 'text-status-present-foreground',
    state: '[--beach-state:var(--status-present)]',
    cssVar: '--status-present',
  },
  blocked: {
    fill: 'fill-status-blocked',
    labelFill: 'fill-status-blocked-foreground',
    bg: 'bg-status-blocked',
    stroke: 'stroke-status-blocked',
    fg: 'text-status-blocked-foreground',
    state: '[--beach-state:var(--status-blocked)]',
    cssVar: '--status-blocked',
  },
  renewal_hold: {
    fill: 'fill-status-renewal-hold',
    labelFill: 'fill-status-renewal-hold-foreground',
    bg: 'bg-status-renewal-hold',
    stroke: 'stroke-status-renewal-hold',
    fg: 'text-status-renewal-hold-foreground',
    state: '[--beach-state:var(--status-renewal-hold)]',
    cssVar: '--status-renewal-hold',
  },
  unavailable: {
    fill: 'fill-status-unavailable',
    labelFill: 'fill-status-unavailable-foreground',
    bg: 'bg-status-unavailable',
    stroke: 'stroke-status-unavailable',
    fg: 'text-status-unavailable-foreground',
    state: '[--beach-state:var(--status-unavailable)]',
    cssVar: '--status-unavailable',
  },
};

/** Light-mode literals of the map surfaces (--map-* variables). */
export const SAND_COLOR = '#f8edd3';
export const SEA_COLOR = '#7cc4e8';
export const DECORATION_COLOR = '#e7dcc2';
