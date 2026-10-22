import type { Schemas } from '@arenile/shared/api';
import { z } from 'zod';
import { centsToEuroInput, euroToCents } from '../configuration/lib/money';
import { V, intField, optionalInt } from '../configuration/lib/validation';

export type PolicyRules = Schemas['PolicyRules'];
export type BookingPolicy = Schemas['BookingPolicy'];
export type BookingPolicyInput = Schemas['BookingPolicyInput'];
type Formula = Schemas['Formula'];
type Channel = Schemas['Channel'];

/**
 * Form model of PolicyRules (spec 9). Every rule is optional: an empty value
 * or "inherit" leaves the field out, so a season policy inherits it from the
 * establishment default policy (and the default from the engine defaults).
 */

const TIME_RE = /^([01]\d|2[0-3]):[0-5]\d$/;
const tri = z.enum(['inherit', 'yes', 'no']);
type Tri = z.infer<typeof tri>;
const optionalTime = z
  .string()
  .trim()
  .refine((v) => v === '' || TIME_RE.test(v), 'policies:validation.time');
const optionalEuro = z
  .string()
  .trim()
  .refine((v) => v === '' || euroToCents(v) !== null, V.amount);

export const tierSchema = z.object({
  until_days_before: intField({ min: 0, max: 400 }),
  refund_percent: intField({ min: 0, max: 100 }),
  as: z.enum(['refund', 'credit']),
});

export type TierValues = z.infer<typeof tierSchema>;

/** Tiers ordered as the engine reads them: furthest from arrival first. Invalid rows go last. */
export function sortTiers(tiers: TierValues[]): TierValues[] {
  const days = (t: TierValues) =>
    /^\d+$/.test(t.until_days_before) ? Number(t.until_days_before) : -1;
  return [...tiers].sort((a, b) => days(b) - days(a));
}

/** A sensible next row: half the smallest "days before" so far, half the refund. */
export function nextTier(tiers: TierValues[]): TierValues {
  if (tiers.length === 0) return { until_days_before: '7', refund_percent: '100', as: 'refund' };
  const last = sortTiers(tiers)[tiers.length - 1];
  const d = Number(last?.until_days_before);
  const r = Number(last?.refund_percent);
  return {
    until_days_before: String(Number.isInteger(d) && d > 0 ? Math.floor(d / 2) : 0),
    refund_percent: String(Number.isInteger(r) ? Math.floor(r / 2) : 0),
    as: last?.as ?? 'refund',
  };
}

export const rulesFields = {
  min_lead_days: intField({ min: 0, max: 400, optional: true }),
  cutoff_time: optionalTime,
  max_lead_time_days: intField({ min: 0, max: 1000, optional: true }),
  same_day: tri,
  same_day_until: optionalTime,
  allowed_formulas: z.array(z.string()),
  min_days: intField({ min: 1, max: 400, optional: true }),
  max_days: intField({ min: 1, max: 400, optional: true }),
  fixed_start_weekday: z.enum(['', '0', '1', '2', '3', '4', '5', '6', '7']),
  payment_mode: z.enum(['', 'full', 'deposit', 'none']),
  deposit_type: z.enum(['percent', 'amount']),
  deposit_value: z.string().trim(),
  balance_due: z.enum(['', 'on_arrival', 'days_before']),
  balance_days: intField({ min: 0, max: 400, optional: true }),
  cancellation_set: z.boolean(),
  cancellation: z.array(tierSchema),
  modification_until: intField({ min: 0, max: 400, optional: true }),
  modification_fee: optionalEuro,
  weather: z.enum(['inherit', 'off', 'refund', 'credit']),
  approval_required: tri,
  max_spots_per_booking: intField({ min: 1, max: 100, optional: true }),
  allow_spot_change: tri,
  customer_can_release: tri,
  release_credit: z.enum(['inherit', 'off', 'on']),
  release_credit_percent: intField({ min: 0, max: 100, optional: true }),
};

export type RulesValues = {
  [K in keyof typeof rulesFields]: z.infer<(typeof rulesFields)[K]>;
};

export function refineRules(v: RulesValues, ctx: z.RefinementCtx) {
  const issue = (path: string, message: string) =>
    ctx.addIssue({ code: 'custom', path: [path], message });
  if (v.min_days && v.max_days && Number(v.max_days) < Number(v.min_days)) {
    issue('max_days', 'policies:validation.maxDays');
  }
  if (v.payment_mode === 'deposit') {
    const ok =
      v.deposit_type === 'percent'
        ? /^\d+$/.test(v.deposit_value) && Number(v.deposit_value) <= 100
        : euroToCents(v.deposit_value) !== null;
    if (!ok) issue('deposit_value', v.deposit_type === 'percent' ? V.percent : V.amount);
  }
  if (v.balance_due === 'days_before' && v.balance_days === '') issue('balance_days', V.required);
  if (v.cancellation_set) {
    const seen = new Set<string>();
    v.cancellation.forEach((tier, i) => {
      const key = tier.until_days_before.trim();
      if (key !== '' && seen.has(key)) {
        ctx.addIssue({
          code: 'custom',
          path: ['cancellation', i, 'until_days_before'],
          message: 'policies:validation.duplicateTier',
        });
      }
      seen.add(key);
    });
  }
  if (v.release_credit === 'on' && v.release_credit_percent === '') {
    issue('release_credit_percent', V.required);
  }
}

const triValue = (v: Tri) => (v === 'inherit' ? undefined : v === 'yes');
const toTri = (b: boolean | undefined): Tri => (b === undefined ? 'inherit' : b ? 'yes' : 'no');

export function toRules(v: RulesValues): PolicyRules {
  const r: PolicyRules = {};
  const minLead = optionalInt(v.min_lead_days);
  if (minLead !== undefined || v.cutoff_time) {
    r.min_lead_time = {};
    if (minLead !== undefined) r.min_lead_time.days = minLead;
    if (v.cutoff_time) r.min_lead_time.cutoff_time = v.cutoff_time;
  }
  const maxLead = optionalInt(v.max_lead_time_days);
  if (maxLead !== undefined) r.max_lead_time_days = maxLead;
  if (v.same_day !== 'inherit') {
    r.same_day_booking = { enabled: v.same_day === 'yes' };
    if (v.same_day === 'yes' && v.same_day_until) r.same_day_booking.until = v.same_day_until;
  }
  const minDays = optionalInt(v.min_days);
  const maxDays = optionalInt(v.max_days);
  if (
    v.allowed_formulas.length ||
    minDays !== undefined ||
    maxDays !== undefined ||
    v.fixed_start_weekday
  ) {
    r.duration = {};
    if (v.allowed_formulas.length) r.duration.allowed_formulas = v.allowed_formulas as Formula[];
    if (minDays !== undefined) r.duration.min_days = minDays;
    if (maxDays !== undefined) r.duration.max_days = maxDays;
    if (v.fixed_start_weekday) r.duration.fixed_start_weekday = Number(v.fixed_start_weekday);
  }
  if (v.payment_mode) {
    r.payment = { mode: v.payment_mode };
    if (v.payment_mode === 'deposit') {
      r.payment.deposit = {
        type: v.deposit_type,
        value:
          v.deposit_type === 'percent'
            ? Number(v.deposit_value)
            : (euroToCents(v.deposit_value) ?? 0),
      };
    }
    if (v.balance_due === 'on_arrival') r.payment.balance_due = 'on_arrival';
    if (v.balance_due === 'days_before')
      r.payment.balance_due = `days_before:${Number(v.balance_days)}`;
  }
  if (v.cancellation_set) {
    r.cancellation = v.cancellation
      .map((t) => ({
        until_days_before: Number(t.until_days_before),
        refund_percent: Number(t.refund_percent),
        as: t.as,
      }))
      .sort((a, b) => b.until_days_before - a.until_days_before);
  }
  const modUntil = optionalInt(v.modification_until);
  if (modUntil !== undefined || v.modification_fee) {
    r.modification = {};
    if (modUntil !== undefined) r.modification.allowed_until_days_before = modUntil;
    if (v.modification_fee) r.modification.fee = euroToCents(v.modification_fee) ?? 0;
  }
  if (v.weather !== 'inherit') {
    r.weather_policy = v.weather === 'off' ? { enabled: false } : { enabled: true, as: v.weather };
  }
  const approval = triValue(v.approval_required);
  if (approval !== undefined) r.approval_required = approval;
  const maxSpots = optionalInt(v.max_spots_per_booking);
  if (maxSpots !== undefined) r.max_spots_per_booking = maxSpots;
  const change = triValue(v.allow_spot_change);
  if (change !== undefined) r.allow_spot_change_during_stay = change;
  const release = triValue(v.customer_can_release);
  if (release !== undefined) r.customer_can_release_season_days = release;
  if (v.release_credit !== 'inherit') {
    r.season_release_credit =
      v.release_credit === 'off'
        ? { enabled: false }
        : { enabled: true, percent: Number(v.release_credit_percent) };
  }
  return r;
}

export function rulesValues(r: PolicyRules | undefined, locale: string): RulesValues {
  const balance = r?.payment?.balance_due ?? '';
  const deposit = r?.payment?.deposit;
  return {
    min_lead_days: r?.min_lead_time?.days != null ? String(r.min_lead_time.days) : '',
    cutoff_time: r?.min_lead_time?.cutoff_time ?? '',
    max_lead_time_days: r?.max_lead_time_days != null ? String(r.max_lead_time_days) : '',
    same_day: toTri(r?.same_day_booking?.enabled),
    same_day_until: r?.same_day_booking?.until ?? '',
    allowed_formulas: r?.duration?.allowed_formulas ?? [],
    min_days: r?.duration?.min_days ? String(r.duration.min_days) : '',
    max_days: r?.duration?.max_days ? String(r.duration.max_days) : '',
    fixed_start_weekday: (r?.duration?.fixed_start_weekday != null
      ? String(r.duration.fixed_start_weekday)
      : '') as RulesValues['fixed_start_weekday'],
    payment_mode: r?.payment?.mode ?? '',
    deposit_type: deposit?.type ?? 'percent',
    deposit_value:
      deposit?.value == null
        ? ''
        : deposit.type === 'amount'
          ? centsToEuroInput(deposit.value, locale)
          : String(deposit.value),
    balance_due: balance === '' ? '' : balance === 'on_arrival' ? 'on_arrival' : 'days_before',
    balance_days: balance.startsWith('days_before:') ? balance.slice('days_before:'.length) : '',
    cancellation_set: r?.cancellation !== undefined,
    cancellation: (r?.cancellation ?? []).map((t) => ({
      until_days_before: String(t.until_days_before),
      refund_percent: String(t.refund_percent),
      as: t.as ?? 'refund',
    })),
    modification_until:
      r?.modification?.allowed_until_days_before != null
        ? String(r.modification.allowed_until_days_before)
        : '',
    modification_fee:
      r?.modification?.fee != null ? centsToEuroInput(r.modification.fee, locale) : '',
    weather:
      r?.weather_policy === undefined
        ? 'inherit'
        : !r.weather_policy.enabled
          ? 'off'
          : (r.weather_policy.as ?? 'refund'),
    approval_required: toTri(r?.approval_required),
    max_spots_per_booking: r?.max_spots_per_booking ? String(r.max_spots_per_booking) : '',
    allow_spot_change: toTri(r?.allow_spot_change_during_stay),
    customer_can_release: toTri(r?.customer_can_release_season_days),
    release_credit:
      r?.season_release_credit === undefined
        ? 'inherit'
        : r.season_release_credit.enabled
          ? 'on'
          : 'off',
    release_credit_percent:
      r?.season_release_credit?.percent != null ? String(r.season_release_credit.percent) : '',
  };
}

// ---------------------------------------------------------------------------
// Season policy (scope + rules) and default policy
// ---------------------------------------------------------------------------

export const policySchema = z
  .object({
    name: z.string().trim().min(1, V.required).max(200),
    priority: z
      .string()
      .trim()
      .regex(/^-?\d{1,6}$/, V.integer),
    active: z.boolean(),
    scope_channels: z.array(z.string()),
    scope_zones: z.array(z.string()),
    scope_packs: z.array(z.string()),
    scope_periods: z.array(z.string()),
    ...rulesFields,
  })
  .superRefine(refineRules);

export type PolicyValues = z.infer<typeof policySchema>;

export function policyValues(p: BookingPolicy | undefined, locale: string): PolicyValues {
  return {
    name: p?.name ?? '',
    priority: String(p?.priority ?? 0),
    active: p?.active ?? true,
    scope_channels: p?.scope.channels ?? [],
    scope_zones: p?.scope.zones ?? [],
    scope_packs: p?.scope.packs ?? [],
    scope_periods: p?.scope.rate_periods ?? [],
    ...rulesValues(p?.rules, locale),
  };
}

export function toPolicyInput(seasonId: string, v: PolicyValues): BookingPolicyInput {
  const scope: BookingPolicyInput['scope'] = {};
  if (v.scope_channels.length) scope.channels = v.scope_channels as Channel[];
  if (v.scope_zones.length) scope.zones = v.scope_zones;
  if (v.scope_packs.length) scope.packs = v.scope_packs;
  if (v.scope_periods.length) scope.rate_periods = v.scope_periods;
  return {
    season_id: seasonId,
    name: v.name.trim(),
    priority: Number(v.priority),
    active: v.active,
    scope,
    rules: toRules(v),
  };
}

export const defaultPolicySchema = z.object(rulesFields).superRefine(refineRules);
export type DefaultPolicyValues = z.infer<typeof defaultPolicySchema>;
