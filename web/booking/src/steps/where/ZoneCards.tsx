import type { Schemas } from '@arenile/shared/api';
import type { PublicAvailability } from '../../api/public';

export interface ZoneCardsProps {
  zones: Schemas['PublicZone'][];
  availability: PublicAvailability;
  showPrices: boolean;
  selectedZoneId: string | null;
  onSelect: (zoneId: string) => void;
}

/**
 * Cards of the `zone` zones: name, "da" price and indicative availability
 * (`indicator`: plenty / few / last / none, "ultimi 3 posti"). The spot is
 * assigned automatically at confirmation (spec 7.5). Owner: booking app builder.
 */
export function ZoneCards(_props: ZoneCardsProps) {
  return null;
}
