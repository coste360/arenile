/**
 * Exact conversions between what users type and the integer units used on the
 * wire: cents (money) and basis points (percentages). No floating point is
 * involved: the text is split on the decimal separator and both parts are
 * parsed as integers, so "19,99" is always 1999 and never 1998.99999.
 *
 * Accepted input (Italian first, tolerant of the English decimal point):
 *   "12" "12,5" "12,50" "1.234,50" "1 234,50" ",5" "12.50" (single dot + 1-2 digits = decimal)
 *   "1.234" "1.234.567" (dot groups of three = thousands)
 */
import { uiIntlLocale } from './intl';

const PLAIN_RE = /^(\d*)(?:,(\d*))?$/;
const THOUSANDS_DOTS_RE = /^\d{1,3}(?:\.\d{3})+$/;

export interface ParseOptions {
  /** Allow a leading minus sign (default false). */
  allowNegative?: boolean;
}

/**
 * Parses a decimal string into an integer scaled by 10^scale. Returns null
 * when the input is empty, malformed, has more than `scale` decimals or does
 * not fit a safe integer.
 */
export function parseScaled(
  input: string,
  scale: number,
  options: ParseOptions = {},
): number | null {
  let s = input.trim().replace(/[\s\u00a0\u202f€%]/g, '');
  if (s === '') return null;
  let negative = false;
  if (s.startsWith('-')) {
    if (!options.allowNegative) return null;
    negative = true;
    s = s.slice(1);
  }
  if (s.includes(',')) {
    // Italian: comma is the decimal separator, dots group thousands.
    const [whole = '', ...rest] = s.split(',');
    if (rest.length !== 1) return null;
    if (whole.includes('.') && !THOUSANDS_DOTS_RE.test(whole)) return null;
    s = `${whole.replace(/\./g, '')},${rest[0]}`;
  } else if (s.includes('.')) {
    if (THOUSANDS_DOTS_RE.test(s) && !/^\d+\.\d{1,2}$/.test(s)) {
      s = s.replace(/\./g, '');
    } else if (/^\d*\.\d*$/.test(s)) {
      s = s.replace('.', ',');
    } else {
      return null;
    }
  }
  const m = PLAIN_RE.exec(s);
  if (!m) return null;
  const whole = m[1] ?? '';
  const frac = m[2] ?? '';
  if (whole === '' && frac === '') return null;
  if (frac.length > scale) return null;
  const factor = 10 ** scale;
  const intPart = whole === '' ? 0 : Number.parseInt(whole, 10);
  const fracPart = frac === '' ? 0 : Number.parseInt(frac.padEnd(scale, '0'), 10);
  const value = intPart * factor + fracPart;
  if (!Number.isSafeInteger(value)) return null;
  return negative && value !== 0 ? -value : value;
}

/**
 * Formats an integer scaled by 10^scale with Italian grouping, e.g.
 * 123450 (scale 2) → "1.234,50". `minDecimals` trims trailing zeros down to it.
 */
export function formatScaled(
  value: number,
  scale: number,
  { minDecimals = scale, grouping = true }: { minDecimals?: number; grouping?: boolean } = {},
): string {
  const factor = 10 ** scale;
  const sign = value < 0 ? '-' : '';
  const abs = Math.abs(Math.trunc(value));
  const whole = Math.floor(abs / factor);
  let frac = scale > 0 ? String(abs % factor).padStart(scale, '0') : '';
  while (frac.length > minDecimals && frac.endsWith('0')) frac = frac.slice(0, -1);
  const wholeText = grouping ? String(whole).replace(/\B(?=(\d{3})+(?!\d))/g, '.') : String(whole);
  return frac ? `${sign}${wholeText},${frac}` : `${sign}${wholeText}`;
}

/** "12,50" → 1250 cents; null when invalid. */
export function parseEuroToCents(input: string, options?: ParseOptions): number | null {
  return parseScaled(input, 2, options);
}

/** 123450 → "1.234,50" (editable text, no currency symbol). */
export function centsToEuroText(cents: number): string {
  return formatScaled(cents, 2);
}

/** Display-only currency formatting of integer cents in the UI language: 123450 → "1.234,50 €" (it). */
export function formatCents(cents: number, currency = 'EUR', locale = uiIntlLocale()): string {
  return new Intl.NumberFormat(locale, {
    style: 'currency',
    currency,
    // Same grouping as the inputs ("1.234,50"); CLDR it-IT omits it below 10.000.
    useGrouping: 'always',
  }).format(cents / 100);
}

/** "22" → 2200 basis points; "5,5" → 550. */
export function parsePercentToBasisPoints(input: string, options?: ParseOptions): number | null {
  return parseScaled(input, 2, options);
}

/** 2200 → "22"; 550 → "5,5" (trailing zero decimals dropped). */
export function basisPointsToPercentText(bp: number): string {
  return formatScaled(bp, 2, { minDecimals: 0, grouping: false });
}

/** Display-only percentage: 550 → "5,5%". */
export function formatBasisPoints(bp: number, locale = uiIntlLocale()): string {
  return new Intl.NumberFormat(locale, { style: 'percent', maximumFractionDigits: 2 }).format(
    bp / 10_000,
  );
}

/** Decimal separator of `locale` ("," for it/de/fr, "." for en), e.g. for CSV exports. */
export function decimalSeparator(locale: string): string {
  const part = new Intl.NumberFormat(locale).formatToParts(1.5).find((p) => p.type === 'decimal');
  return part?.value ?? ',';
}

/**
 * Normalises a user-typed exact decimal ("0,6", "1.2") to the API `Decimal`
 * string ("0.6", "1.2") with at most `maxDecimals` decimals; null if malformed.
 */
export function toApiDecimal(input: string, maxDecimals = 4): string | null {
  const scaled = parseScaled(input, maxDecimals);
  if (scaled === null) return null;
  return formatScaled(scaled, maxDecimals, { minDecimals: 0, grouping: false }).replace(',', '.');
}

/** API `Decimal` ("0.6") → editable Italian text ("0,6"). */
export function apiDecimalToInput(value: string): string {
  return value.replace('.', ',');
}
