import { de, enGB, fr, it, type Locale as DateFnsLocale } from 'date-fns/locale';

/*
 * date-fns locales of the calendars. Kept apart from lib/dates so the
 * first-paint bundle of the booking page does not carry them: only the
 * lazily loaded calendar imports this module.
 */
const LOCALES: Record<string, DateFnsLocale> = { it, en: enGB, de, fr };

/** date-fns locale for an i18n language (Italian fallback). */
export function dateFnsLocale(language: string | undefined): DateFnsLocale {
  return LOCALES[(language ?? 'it').slice(0, 2)] ?? it;
}
