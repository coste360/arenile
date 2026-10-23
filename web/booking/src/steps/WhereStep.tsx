import type { Schemas } from '@arenile/shared/api';
import { useTranslation } from 'react-i18next';
import { PUBLIC_NS } from '../i18n';
import type { PublicAvailability } from '../api/public';

/** One chosen resource of the "Dove" step (becomes a PublicLineInput). */
export interface WhereChoice {
  zoneId: string;
  /** Spot of a `seat` zone. */
  spotId?: string;
  /** Units of a `quantity` zone (default 1). */
  quantity?: number;
}

export interface WhereStepProps {
  slug: string;
  zones: Schemas['PublicZone'][];
  availability: PublicAvailability | undefined;
  showPrices: boolean;
  value: WhereChoice[];
  onChange: (value: WhereChoice[]) => void;
}

/**
 * "Dove" step (spec 12.3 passo 2): mixed modes render together, map first.
 * - `seat` zones: SeatMapPicker (BeachMap from @arenile/shared/map with the
 *   kit art, free spots highlighted, price legend per row; lazy chunk);
 * - `zone` zones: ZoneCards ("da" price, indicative availability);
 * - `quantity` zones: QuantityPicker.
 * Owner: booking app builder (placeholder until M4 lands).
 */
export function WhereStep(_props: WhereStepProps) {
  const { t } = useTranslation(PUBLIC_NS);
  return <p className="text-sm text-muted-foreground">{t('where.pickSpot')}</p>;
}
