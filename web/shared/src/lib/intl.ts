/**
 * Intl locale of the UI language (spec 20). Numbers, money and dates are always
 * formatted by Intl in the language the user is reading, never with a
 * hard-coded Italian format: each app sets `<html lang>` at start-up and on
 * every language change (`changeLocale`), and the shared formatters default to
 * it. Italian is the fallback (tests, SSR, unknown languages).
 */
const INTL_LOCALES: Record<string, string> = {
  it: 'it-IT',
  en: 'en-GB',
  de: 'de-DE',
  fr: 'fr-FR',
};

/** "de" / "de-AT" → "de-DE"; anything unknown → "it-IT". */
export function intlLocaleOf(language: string | null | undefined): string {
  return INTL_LOCALES[(language ?? '').slice(0, 2).toLowerCase()] ?? 'it-IT';
}

/** Intl locale of the current UI language, read from `<html lang>`. */
export function uiIntlLocale(): string {
  return intlLocaleOf(typeof document === 'undefined' ? '' : document.documentElement.lang);
}
