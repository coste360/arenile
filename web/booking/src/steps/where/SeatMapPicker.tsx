import type { PublicAvailability, PublicMapDocument } from '../../api/public';

export interface SeatMapPickerProps {
  map: PublicMapDocument;
  availability: PublicAvailability;
  showPrices: boolean;
  /** Selected spot ids. */
  selected: string[];
  onToggle: (spotId: string) => void;
}

/**
 * Map of a `seat` zone with free spots selectable (spec 12.3): renders the
 * shared BeachMap with the kit sprite (MapKitDefs), status overlays and the
 * price legend (`availability.price_legend`). Loaded lazily with the step:
 * it must stay out of the first-screen budget. Owner: booking app builder.
 */
export function SeatMapPicker(_props: SeatMapPickerProps) {
  return null;
}
