import { formatCents } from '@arenile/shared/lib/money';

/** Display helpers of the public page: Intl formats in the page language (spec 20). */

const INTL_LOCALES: Record<string, string> = {
  it: 'it-IT',
  en: 'en-GB',
  de: 'de-DE',
  fr: 'fr-FR',
};

export function intlLocale(language: string | undefined): string {
  return INTL_LOCALES[(language ?? 'it').slice(0, 2)] ?? 'it-IT';
}

/** Integer cents → "25,00 €" in the page language. Display only (cents never become floats in logic). */
export function formatMoney(cents: number, currency: string, language: string | undefined): string {
  return formatCents(cents, currency || 'EUR', intlLocale(language));
}

/** Manager-provided multilingual text with fallback: language, establishment default, Italian, any. */
export function localize(
  text: Record<string, string> | null | undefined,
  language: string | undefined,
  fallback = 'it',
): string {
  if (!text) return '';
  const lang = (language ?? '').slice(0, 2);
  return text[lang] || text[fallback] || text.it || Object.values(text).find(Boolean) || '';
}

/** "2027-07-10" → "sab 10 lug" (short, in the page language, calendar date without time zone). */
export function formatDayShort(iso: string, language: string | undefined): string {
  const [y, m, d] = iso.split('-').map(Number);
  if (!y || !m || !d) return iso;
  return new Intl.DateTimeFormat(intlLocale(language), {
    weekday: 'short',
    day: 'numeric',
    month: 'short',
    timeZone: 'UTC',
  }).format(new Date(Date.UTC(y, m - 1, d)));
}

/** Remaining time as "m:ss" (never negative). */
export function formatCountdown(ms: number): string {
  const total = Math.max(0, Math.ceil(ms / 1000));
  const m = Math.floor(total / 60);
  const s = total % 60;
  return `${m}:${String(s).padStart(2, '0')}`;
}
