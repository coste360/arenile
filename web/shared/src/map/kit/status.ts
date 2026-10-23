import type { SpotStatus } from '../types';

/**
 * Corner badge of a booking status (drawn over the art, never inside it).
 * Statuses that change what the attendant does carry a glyph, so they stay
 * recognisable without colour (colour-blind users, greyscale tablets):
 * checked in, season pass holder, pass released for the day, on hold,
 * closed/blocked, to collect and renewal option. Plain bookings and free
 * spots rely on the halo and the legend.
 */
export type BadgeGlyph = 'check' | 'star' | 'released' | 'clock' | 'cross' | 'euro' | 'renew';

export const STATUS_BADGES: Readonly<Record<SpotStatus, BadgeGlyph | null>> = {
  free: null,
  booked_online: null,
  booked_desk: null,
  season: 'star',
  season_released: 'released',
  hold: 'clock',
  to_collect: 'euro',
  present: 'check',
  blocked: 'cross',
  renewal_hold: 'renew',
  unavailable: null,
};

export const badgeSymbolId = (g: BadgeGlyph) => `ark-badge-${g}`;
