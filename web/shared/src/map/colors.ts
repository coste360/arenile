import type { SpotStatus } from './types';

/**
 * Default status palette; the panel theme may override individual entries.
 * Colours are chosen to be distinguishable and are always paired with a
 * text label in the legend, never conveyed by colour alone.
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

export const SAND_COLOR = '#f8edd3';
export const SEA_COLOR = '#7cc4e8';
export const DECORATION_COLOR = '#e7dcc2';
