import { z } from 'zod';
import {
  parseEuroToCents as euroToCents,
  parsePercentToBasisPoints as percentToBasisPoints,
  toApiDecimal,
} from '@arenile/shared/lib/money';

/**
 * Shared zod building blocks. Messages are i18n keys (namespace-qualified so
 * they resolve from any feature) rendered by <FieldError>.
 */
export const V = {
  required: 'configuration:validation.required',
  code: 'configuration:validation.code',
  date: 'configuration:validation.date',
  dateOrder: 'configuration:validation.dateOrder',
  seasonOpensInYear: 'configuration:validation.seasonOpensInYear',
  seasonClosesBy: 'configuration:validation.seasonClosesBy',
  seasonMaxOneYear: 'configuration:validation.seasonMaxOneYear',
  amount: 'configuration:validation.amount',
  percent: 'configuration:validation.percent',
  integer: 'configuration:validation.integer',
  email: 'configuration:validation.email',
  color: 'configuration:validation.color',
  url: 'configuration:validation.url',
  origin: 'configuration:validation.origin',
  slug: 'configuration:validation.slug',
  decimal: 'configuration:validation.decimal',
  halfDayFactor: 'configuration:validation.halfDayFactor',
  defaultLocaleText: 'configuration:validation.defaultLocaleText',
  localeRequired: 'configuration:validation.localeRequired',
  monthDay: 'configuration:validation.monthDay',
  overlap: 'configuration:validation.overlap',
  outsideSeason: 'configuration:validation.outsideSeason',
  slotRequired: 'configuration:validation.slotRequired',
  dateRequired: 'configuration:validation.dateRequired',
} as const;

export const ISO_DATE_RE = /^\d{4}-(0[1-9]|1[0-2])-(0[1-9]|[12]\d|3[01])$/;
export const MONTH_DAY_RE = /^(0[1-9]|1[0-2])-(0[1-9]|[12]\d|3[01])$/;
export const CODE_RE = /^[a-z0-9][a-z0-9_-]{0,63}$/;
export const SLUG_RE = /^[a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?$/;
export const HEX_COLOR_RE = /^#[0-9a-fA-F]{6}$/;
export const INT_RE = /^\d+$/;

/** True for a real calendar date "YYYY-MM-DD" (rejects 2027-02-30). */
export function isCalendarDate(value: string): boolean {
  if (!ISO_DATE_RE.test(value)) return false;
  const [y, m, d] = value.split('-').map(Number) as [number, number, number];
  const dt = new Date(Date.UTC(y, m - 1, d));
  return dt.getUTCFullYear() === y && dt.getUTCMonth() === m - 1 && dt.getUTCDate() === d;
}

export const isoDate = z.string().trim().refine(isCalendarDate, V.date);
export const optionalIsoDate = z
  .string()
  .trim()
  .refine((v) => v === '' || isCalendarDate(v), V.date);

export const codeField = z.string().trim().toLowerCase().regex(CODE_RE, V.code);

export const requiredText = (max = 200) => z.string().trim().min(1, V.required).max(max);

export const euroField = z
  .string()
  .trim()
  .refine((v) => euroToCents(v) !== null, V.amount);
export const optionalEuroField = z
  .string()
  .trim()
  .refine((v) => v === '' || euroToCents(v) !== null, V.amount);

export const percentField = z
  .string()
  .trim()
  .refine((v) => {
    const bp = percentToBasisPoints(v);
    return bp !== null && bp <= 10000;
  }, V.percent);
export const optionalPercentField = z
  .string()
  .trim()
  .refine((v) => {
    if (v === '') return true;
    const bp = percentToBasisPoints(v);
    return bp !== null && bp <= 10000;
  }, V.percent);

/** Non-negative integer typed as text ("" allowed when optional). */
export const intField = (opts: { min?: number; max?: number; optional?: boolean } = {}) =>
  z
    .string()
    .trim()
    .refine((v) => {
      if (v === '') return !!opts.optional;
      if (!INT_RE.test(v)) return false;
      const n = Number(v);
      return n >= (opts.min ?? 0) && n <= (opts.max ?? 1_000_000);
    }, V.integer);

export const decimalField = z
  .string()
  .trim()
  .refine((v) => toApiDecimal(v) !== null, V.decimal);

export const hexColor = z.string().trim().regex(HEX_COLOR_RE, V.color);
export const optionalHexColor = z
  .string()
  .trim()
  .refine((v) => v === '' || HEX_COLOR_RE.test(v), V.color);

/** Localised text whose default-locale entry is mandatory. */
export function i18nTextField(defaultLocale: string) {
  return z
    .record(z.string(), z.string().max(500))
    .refine((t) => (t[defaultLocale] ?? '').trim() !== '', {
      message: V.defaultLocaleText,
      path: [defaultLocale],
    });
}

/** Optional localised text. */
export const optionalI18nText = z.record(z.string(), z.string().max(2000));

/** "" → undefined, else Number(v). */
export function optionalInt(v: string): number | undefined {
  return v.trim() === '' ? undefined : Number(v.trim());
}

/** Idempotency-Key for a creating POST (one per submission). */
export function newIdempotencyKey(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID();
  }
  return `k-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 12)}`;
}

/** Strong entity tag for If-Match from a resource version. */
export function ifMatch(version: number): string {
  return `"${version}"`;
}
