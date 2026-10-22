/**
 * Exact conversions between decimal strings typed by the user and the integer
 * units used on the wire (cents, basis points). No floating point is involved:
 * the string is split on the decimal separator and both parts are parsed as
 * integers, so "19,99" is always 1999 and never 1998.99999.
 */

const DECIMAL_RE = /^(\d+)(?:[.,](\d*))?$/;
const LEADING_SEP_RE = /^[.,](\d+)$/;

/**
 * Parses a non-negative decimal string ("12", "12,5", "12.50", ",5") into an
 * integer scaled by 10^scale. Returns null when the input is empty, malformed,
 * has more than `scale` decimals or does not fit a safe integer.
 */
export function parseScaled(input: string, scale: number): number | null {
  const s = input.trim().replace(/\s+/g, '');
  if (s === '') return null;
  let whole: string;
  let frac: string;
  const m = DECIMAL_RE.exec(s);
  if (m) {
    whole = m[1] ?? '0';
    frac = m[2] ?? '';
  } else {
    const lead = LEADING_SEP_RE.exec(s);
    if (!lead) return null;
    whole = '0';
    frac = lead[1] ?? '';
  }
  if (frac.length > scale) return null;
  const factor = 10 ** scale;
  const intPart = Number.parseInt(whole, 10);
  const fracPart = frac === '' ? 0 : Number.parseInt(frac.padEnd(scale, '0'), 10);
  const value = intPart * factor + fracPart;
  return Number.isSafeInteger(value) ? value : null;
}

/** Decimal separator of `locale` ("," for it/de/fr, "." for en). */
export function decimalSeparator(locale: string): string {
  const part = new Intl.NumberFormat(locale).formatToParts(1.5).find((p) => p.type === 'decimal');
  return part?.value ?? ',';
}

/** Formats an integer scaled by 10^scale as an editable string, e.g. 1250 → "12,50". */
export function formatScaled(value: number, scale: number, locale = 'it'): string {
  const factor = 10 ** scale;
  const sign = value < 0 ? '-' : '';
  const abs = Math.abs(Math.trunc(value));
  const whole = Math.floor(abs / factor);
  const frac = abs % factor;
  if (scale === 0) return `${sign}${whole}`;
  return `${sign}${whole}${decimalSeparator(locale)}${String(frac).padStart(scale, '0')}`;
}

/** "12,50" → 1250; null when invalid. */
export function euroToCents(input: string): number | null {
  return parseScaled(input, 2);
}

/** 1250 → "12,50" (input value, not a display string). */
export function centsToEuroInput(cents: number, locale = 'it'): string {
  return formatScaled(cents, 2, locale);
}

/** Display-only currency formatting of integer cents. */
export function formatCents(cents: number, locale = 'it', currency = 'EUR'): string {
  return new Intl.NumberFormat(locale, { style: 'currency', currency }).format(cents / 100);
}

/** "22" → 2200 basis points; "5,5" → 550. */
export function percentToBasisPoints(input: string): number | null {
  return parseScaled(input, 2);
}

function trimDecimals(s: string, sep: string): string {
  if (!s.includes(sep)) return s;
  const trimmed = s.replace(/0+$/, '');
  return trimmed.endsWith(sep) ? trimmed.slice(0, -1) : trimmed;
}

/** 2200 → "22"; 550 → "5,5". Trailing zero decimals are dropped. */
export function basisPointsToPercentInput(bp: number, locale = 'it'): string {
  return trimDecimals(formatScaled(bp, 2, locale), decimalSeparator(locale));
}

/** Normalises a user-typed exact decimal ("0,6") to the API `Decimal` ("0.6"); null if malformed. */
export function toApiDecimal(input: string, maxDecimals = 4): string | null {
  const scaled = parseScaled(input, maxDecimals);
  if (scaled === null) return null;
  return trimDecimals(formatScaled(scaled, maxDecimals, 'en'), '.');
}

/** API `Decimal` ("0.6") → localised input value ("0,6"). */
export function apiDecimalToInput(value: string, locale = 'it'): string {
  return value.replace('.', decimalSeparator(locale));
}
