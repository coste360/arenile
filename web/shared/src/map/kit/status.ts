import type { SpotStatus } from '../types';

/**
 * Corner badge of a booking status (drawn over the art, never inside it).
 * Every status except free and unavailable carries a glyph, so the map stays
 * readable without colour (colour-blind users, greyscale tablets): a globe
 * for online bookings, a counter for desk bookings, checked in, season pass
 * holder, pass released for the day, on hold, closed/blocked, to collect and
 * renewal option. Free spots have no badge and a faint halo.
 */
export type BadgeGlyph =
  'globe' | 'desk' | 'check' | 'star' | 'released' | 'clock' | 'cross' | 'euro' | 'renew';

export const STATUS_BADGES: Readonly<Record<SpotStatus, BadgeGlyph | null>> = {
  free: null,
  booked_online: 'globe',
  booked_desk: 'desk',
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
