import { extras, packs, zones } from '../configuration/api/resources';
import type { ConfigContext } from '../configuration/api/useConfigContext';
import { localize } from '../configuration/lib/i18nText';

/** Catalog and zones used by the pricing and policy forms. */
export function usePricingData(ctx: ConfigContext) {
  const packList = packs.useList(ctx.establishmentId);
  const extraList = extras.useList(ctx.establishmentId);
  const zoneList = zones.useList(ctx.establishmentId);
  const name = (text: Record<string, string>) => localize(text, ctx.uiLocale, ctx.defaultLocale);
  const zoneItems = [...(zoneList.data ?? [])].sort((a, b) => a.sort_order - b.sort_order);
  return {
    packs: [...(packList.data ?? [])].sort((a, b) => a.sort_order - b.sort_order),
    extras: [...(extraList.data ?? [])].sort((a, b) => a.sort_order - b.sort_order),
    zones: zoneItems.map((z) => ({ id: z.id, code: z.code, name: name(z.name) || z.code })),
    rawZones: zoneItems,
    /** Price categories in use by zones (spot overrides are not listed here). */
    categories: [...new Set(zoneItems.map((z) => z.price_category))].sort(),
    name,
    isLoading: packList.isPending || extraList.isPending || zoneList.isPending,
    error: packList.error ?? extraList.error ?? zoneList.error,
  };
}
