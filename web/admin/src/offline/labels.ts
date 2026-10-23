import type { Schemas } from '@arenile/shared/api';

/** Queue label of a spot action: spot and booking code, never the customer's contacts. */
export function spotLabel(spot: Pick<Schemas['OperationalSpot'], 'label' | 'booking'>): string {
  return spot.booking ? `${spot.label} · ${spot.booking.code}` : spot.label;
}
