import type { SpotStatus } from '../types';

/**
 * Corner badge of a booking status (drawn over the art, never inside it).
 * Every status except free carries a glyph, so the map stays readable
 * without colour (colour-blind users, greyscale tablets): a globe for online
 * bookings, a counter for desk bookings, checked in, season pass holder,
 * pass released for the day, on hold, closed/blocked, to collect, renewal
 * option and a lock for unavailable (public page, inactive spots). Free
 * spots have no badge and a faint halo.
 */
export type BadgeGlyph =
  'globe' | 'desk' | 'check' | 'star' | 'released' | 'clock' | 'cross' | 'euro' | 'renew' | 'lock';

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
  unavailable: 'lock',
};

export const badgeSymbolId = (g: BadgeGlyph) => `ark-badge-${g}`;

/** Paths of the status badge glyphs, on a 24-unit grid, drawn with currentColor. */
export const BADGE_GLYPHS: Readonly<Record<BadgeGlyph, string>> = {
  globe:
    'M12 4.5a7.5 7.5 0 1 0 0 15 7.5 7.5 0 1 0 0-15M4.5 12h15M12 4.5c-3 2.6-3 12.4 0 15M12 4.5c3 2.6 3 12.4 0 15',
  desk: 'M4.5 10.5h15M4.5 10.5l2.5-5h10l2.5 5M6 10.5V19h12v-8.5M9.5 15h5',
  check: 'M5.5 12.5l4.2 4.2L18.5 8',
  star: 'M12 4.2l2.4 4.9 5.4.8-3.9 3.8.9 5.4-4.8-2.6-4.8 2.6.9-5.4-3.9-3.8 5.4-.8z',
  released: 'M7.5 16.5l9-9M9.5 7.5h7v7',
  clock: 'M12 4.5a7.5 7.5 0 1 0 0 15 7.5 7.5 0 1 0 0-15M12 8.5v4l2.8 1.8',
  cross: 'M7.5 7.5l9 9M16.5 7.5l-9 9',
  euro: 'M16.5 7.6a5.6 5.6 0 1 0 0 8.8M5.5 10.6h8M5.5 13.4h7',
  renew: 'M18.5 12a6.5 6.5 0 1 1-1.9-4.6M18.5 5v3.8h-3.8',
  lock: 'M7 11.5h10v7H7zM9.2 11.5V9a2.8 2.8 0 0 1 5.6 0v2.5',
};

/** Stroke of a glyph on its 24-unit grid (the star is filled). */
export const badgeGlyphStroke = (g: BadgeGlyph) => (g === 'star' ? 1 : 2.6);
