import type { Schemas } from '@arenile/shared/api';

export type BookingCreateBody = Schemas['BookingCreate'];

/** Prefill of a search (from the map day, the new booking sheet). */
export interface FindSpotDefaults {
  startsOn?: string;
  endsOn?: string;
  slots?: 'FULL' | 'AM' | 'PM';
  count?: number;
}
