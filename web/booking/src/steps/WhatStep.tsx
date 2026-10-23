import type { Schemas } from '@arenile/shared/api';
import type { PublicCatalog } from '../api/public';

export interface WhatStepProps {
  catalog: PublicCatalog | undefined;
  /** Pack per chosen resource (index aligned with WhereStep choices). */
  packIds: (string | null)[];
  extras: Schemas['PublicExtraInput'][];
  promoCode: string;
  promoStatus?: Schemas['PromoStatus'];
  onChange: (next: {
    packIds: (string | null)[];
    extras: Schemas['PublicExtraInput'][];
    promoCode: string;
  }) => void;
}

/**
 * "Cosa" step (spec 12.3 passo 3): compatible packs with their composition
 * (icons), extras with quantities, promo code. The price summary
 * (summary/PriceSummary) updates on every change via a debounced `quote`.
 * Owner: booking app builder.
 */
export function WhatStep(_props: WhatStepProps) {
  return null;
}
