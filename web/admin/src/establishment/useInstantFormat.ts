import {
  formatInstant,
  formatInstantDate,
  formatInstantTime,
  type InstantFormatOptions,
} from '@arenile/shared/lib/dates';
import { useTranslation } from 'react-i18next';
import { useCurrentEstablishment } from './context';

export interface InstantFormat {
  /** "dd/MM/yyyy, HH:mm" in the establishment time zone. */
  dateTime: (iso: string | null | undefined) => string;
  /** "dd/MM/yyyy" in the establishment time zone. */
  date: (iso: string | null | undefined) => string;
  /** "HH:mm" in the establishment time zone. */
  time: (iso: string | null | undefined) => string;
}

/**
 * Formatters of RFC 3339 instants (payments, history, consents, sessions)
 * in the UI language and the time zone of the current establishment: an
 * instant is shown as the establishment sees it, never in the browser zone.
 */
export function useInstantFormat(): InstantFormat {
  const { i18n } = useTranslation();
  const { establishment } = useCurrentEstablishment();
  const opts: InstantFormatOptions = { locale: i18n.language, timeZone: establishment?.timezone };
  return {
    dateTime: (iso) => formatInstant(iso, opts),
    date: (iso) => formatInstantDate(iso, opts),
    time: (iso) => formatInstantTime(iso, opts),
  };
}
