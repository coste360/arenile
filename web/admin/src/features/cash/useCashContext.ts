import { useMe } from '@arenile/shared/api';
import { formatCents } from '@arenile/shared/lib/money';
import { useTranslation } from 'react-i18next';
import { useCurrentEstablishment, useRequiredEstablishmentId } from '../../establishment';
import { cashPermissionsFor, type CashPermissions } from './lib/permissions';

export interface CashContext extends CashPermissions {
  establishmentId: string;
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
  const locale = i18n.language === 'it' ? 'it-IT' : i18n.language;
  const tz = est?.timezone ?? 'Europe/Rome';
  const fmt = (iso: string | undefined, opts: Intl.DateTimeFormatOptions) =>
    iso ? new Intl.DateTimeFormat(locale, { timeZone: tz, ...opts }).format(new Date(iso)) : '';
  return {
    ...perms,
    establishmentId,
    loading: me.isPending || current.isLoading,
    money: (cents) => formatCents(cents, est?.currency ?? 'EUR'),
    dateTime: (iso) => fmt(iso, { dateStyle: 'short', timeStyle: 'short' }),
    time: (iso) => fmt(iso, { timeStyle: 'short' }),
  };
}
