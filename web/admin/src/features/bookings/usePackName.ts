import { useTranslation } from 'react-i18next';
import { extras as extrasResource, packs as packsResource } from '../configuration/api/resources';
import { localize } from '../configuration/lib/i18nText';

type NameOf = (code: string | null | undefined) => string;

/**
 * Localized catalog names from codes: bookings carry `pack_code` and
 * `extra_code`, operators read names. Falls back to the code while the
 * catalog loads or for an item removed since.
 */
export function useCatalogNames(establishmentId: string): { pack: NameOf; extra: NameOf } {
  const { i18n } = useTranslation();
  const packs = packsResource.useList(establishmentId);
  const extras = extrasResource.useList(establishmentId);
  const locale = i18n.language || 'it';
  const lookup =
    (list: { code: string; name: Record<string, string> }[] | undefined): NameOf =>
    (code) => {
      if (!code) return '';
      const item = list?.find((x) => x.code === code);
      return (item && localize(item.name, locale, 'it')) || code;
    };
  return { pack: lookup(packs.data), extra: lookup(extras.data) };
}

/** Localized pack name from its code (see useCatalogNames). */
export function usePackName(establishmentId: string): NameOf {
  return useCatalogNames(establishmentId).pack;
}
