import { useMe, type Establishment } from '@arenile/shared/api';
import { useTranslation } from 'react-i18next';
import { useRequiredEstablishmentId } from '../../../establishment';
import { orderedLocales } from '../lib/i18nText';
import { permissionsFor, type EstablishmentPermissions } from '../lib/permissions';
import { useEstablishment } from './establishment';

export interface ConfigContext extends EstablishmentPermissions {
  establishmentId: string;
  establishment: Establishment | undefined;
  /** Default locale of the establishment (fallback for I18nText). */
  defaultLocale: string;
  /** Enabled locales, default first. */
  locales: string[];
  /** UI language, for localised display. */
  uiLocale: string;
  isLoading: boolean;
  error: unknown;
  refetch: () => void;
}

/** Establishment, permissions and locales shared by every configuration page. */
export function useConfigContext(): ConfigContext {
  const establishmentId = useRequiredEstablishmentId();
  const { i18n } = useTranslation();
  const me = useMe();
  const est = useEstablishment(establishmentId);
  const establishment = est.data;
  const defaultLocale = establishment?.default_locale ?? 'it';
  const perms = permissionsFor(me.data, establishment);
  return {
    establishmentId,
    establishment,
    defaultLocale,
    locales: orderedLocales(defaultLocale, establishment?.enabled_locales ?? [defaultLocale]),
    uiLocale: i18n.language || 'it',
    isLoading: est.isPending || me.isPending,
    error: est.error ?? me.error,
    refetch: () => void est.refetch(),
    ...perms,
  };
}
