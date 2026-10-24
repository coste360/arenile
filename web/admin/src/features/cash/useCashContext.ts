import { useMe } from '@arenile/shared/api';
import { formatInstant, formatInstantTime } from '@arenile/shared/lib/dates';
import { formatCents } from '@arenile/shared/lib/money';
import { useTranslation } from 'react-i18next';
import { useCurrentEstablishment, useRequiredEstablishmentId } from '../../establishment';
import { cashPermissionsFor, type CashPermissions } from './lib/permissions';

export interface CashContext extends CashPermissions {
  establishmentId: string;
  /** Name of the establishment (print header). */
  establishmentName: string;
  /** True until the principal and the establishment are known. */
  loading: boolean;
  /** Currency formatting of integer cents ("1.234,50 €"). */
  money: (cents: number) => string;
  /** Instant (RFC 3339) shown in the establishment timezone, date + time. */
  dateTime: (iso: string | undefined) => string;
  /** Instant shown as time only. */
  time: (iso: string | undefined) => string;
}

/** Establishment, permissions and formatters shared by the cash pages. */
export function useCashContext(): CashContext {
  const { i18n } = useTranslation();
  const establishmentId = useRequiredEstablishmentId();
  const me = useMe();
  const current = useCurrentEstablishment();
  const est = current.establishment;
  const perms = cashPermissionsFor(me.data, est);
  const fmt = { locale: i18n.language, timeZone: est?.timezone ?? 'Europe/Rome' };
  return {
    ...perms,
    establishmentId,
    establishmentName: est?.name ?? '',
    loading: me.isPending || current.isLoading,
    money: (cents) => formatCents(cents, est?.currency ?? 'EUR'),
    dateTime: (iso) => formatInstant(iso, fmt),
    time: (iso) => formatInstantTime(iso, fmt),
  };
}
