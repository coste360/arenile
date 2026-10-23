import type { Schemas } from '@arenile/shared/api';

/** Indicative availability text (spec 12.3: "ultimi 3 posti"). */
export function indicatorText(
  t: (key: string, opts?: Record<string, unknown>) => string,
  z: Pick<Schemas['PublicZoneAvailability'], 'indicator' | 'available'>,
): string {
  switch (z.indicator) {
    case 'none':
      return t('where.soldOut');
    case 'last':
      return t('where.lastUnits', { count: z.available });
    case 'few':
      return t('where.fewUnits');
    default:
      return t('where.plenty');
  }
}
