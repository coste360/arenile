import { z } from 'zod';
import {
  apiDecimalToInput,
  centsToEuroText as centsToEuroInput,
  parseEuroToCents as euroToCents,
  toApiDecimal,
} from '@arenile/shared/lib/money';
import {
  CODE_RE,
  V,
  intField,
  isCalendarDate,
  optionalIsoDate,
  optionalInt,
} from '../configuration/lib/validation';
import type {
  Agreement,
  AgreementInput,
  FixedPrice,
  FixedPriceInput,
  PriceAction,
  PriceConditions,
  PriceRule,
  PriceRuleInput,
  PromoCode,
  PromoCodeInput,
} from './api';
import {
  agreementM8Fields,
  agreementM8Values,
  refineAgreementM8,
  toAgreementM8Input,
} from './agreements/agreement.schema';

/**
 * Form schemas of the pricing feature (spec 8): price rules, fixed-duration
 * prices, promo codes and agreements. Amounts are typed in euro and sent as
 * integer cents; percentages and factors are exact decimal strings.
 */

export type Stage = 'base' | 'modifier' | 'discount';
export type ActionType = PriceAction['type'];

/** Action types allowed per stage (same table as the engine's validateAction). */
export const ACTIONS_BY_STAGE: Record<Stage, readonly ActionType[]> = {
  base: ['set'],
  modifier: ['add', 'multiply', 'percent_off', 'amount_off'],
  discount: ['percent_off', 'amount_off', 'multiply', 'free_days'],
};

export const STAGES: readonly Stage[] = ['base', 'modifier', 'discount'];
export const CHANNELS = ['desk', 'online', 'api'] as const;
export const SLOTS = ['AM', 'PM'] as const;
export const FORMULAS = [
  'half_day',
  'day',
  'multi_day',
  'week',
  'fortnight',
  'month',
  'season',
] as const;
export const FIXED_FORMULAS = ['week', 'fortnight', 'month', 'season'] as const;
export const WEEKDAYS = ['1', '2', '3', '4', '5', '6', '7'] as const;
export const PERS = ['booking', 'line', 'day'] as const;

const SIGNED_EURO_RE = /^-?\s*\d+(?:[.,]\d{0,2})?$/;

/** "-5,50" → -550; null when malformed. */
export function signedEuroToCents(v: string): number | null {
  const s = v.trim();
  if (!SIGNED_EURO_RE.test(s)) return null;
  const neg = s.startsWith('-');
  const cents = euroToCents(s.replace('-', '').trim());
  return cents === null ? null : neg ? -cents : cents;
}

// ---------------------------------------------------------------------------
// Action
// ---------------------------------------------------------------------------

export const actionFields = {
  action_type: z.enum(['set', 'add', 'multiply', 'percent_off', 'amount_off', 'free_days']),
  amount: z.string().trim(),
  percent: z.string().trim(),
  factor: z.string().trim(),
  per: z.enum(['', 'booking', 'line', 'day']),
  every: z.string().trim(),
  free: z.string().trim(),
};

type ActionValues = {
  action_type: ActionType;
  amount: string;
  percent: string;
  factor: string;
  per: '' | 'booking' | 'line' | 'day';
  every: string;
  free: string;
};

/** Field-level checks of the action, attached to the right input. */
export function refineAction(v: ActionValues, ctx: z.RefinementCtx) {
  const issue = (path: string, message: string) =>
    ctx.addIssue({ code: 'custom', path: [path], message });
  switch (v.action_type) {
    case 'set':
      if (euroToCents(v.amount) === null) issue('amount', V.amount);
      break;
    case 'add':
      if (signedEuroToCents(v.amount) === null) issue('amount', V.amount);
      break;
    case 'amount_off': {
      const c = euroToCents(v.amount);
      if (c === null || c <= 0) issue('amount', V.amount);
      break;
    }
    case 'percent_off': {
      const p = toApiDecimal(v.percent, 2);
      if (p === null || Number(p) <= 0 || Number(p) > 100) issue('percent', V.percent);
      break;
    }
    case 'multiply':
      if (toApiDecimal(v.factor, 6) === null) issue('factor', V.decimal);
      break;
    case 'free_days': {
      const every = Number(v.every);
      const free = Number(v.free);
      if (!Number.isInteger(every) || every < 2) issue('every', 'pricing:validation.every');
      if (!Number.isInteger(free) || free < 1 || free >= every)
        issue('free', 'pricing:validation.free');
      break;
    }
  }
}

export function toAction(v: ActionValues): PriceAction {
  switch (v.action_type) {
    case 'set':
      return { type: 'set', amount: euroToCents(v.amount) ?? 0 };
    case 'add':
      return { type: 'add', amount: signedEuroToCents(v.amount) ?? 0 };
    case 'amount_off':
      return {
        type: 'amount_off',
        amount: euroToCents(v.amount) ?? 0,
        ...(v.per ? { per: v.per } : {}),
      };
    case 'percent_off':
      return { type: 'percent_off', percent: toApiDecimal(v.percent, 2) ?? '0' };
    case 'multiply':
      return { type: 'multiply', factor: toApiDecimal(v.factor, 6) ?? '1' };
    case 'free_days':
      return { type: 'free_days', every: Number(v.every), free: Number(v.free) };
  }
}

export function actionValues(a: PriceAction | undefined, fallback: ActionType): ActionValues {
  const amount = a?.amount ?? 0;
  return {
    action_type: a?.type ?? fallback,
    amount: a?.amount == null ? '' : (amount < 0 ? '-' : '') + centsToEuroInput(Math.abs(amount)),
    percent: a?.percent ? apiDecimalToInput(a.percent) : '',
    factor: a?.factor ? apiDecimalToInput(a.factor) : '',
    per: a?.per ?? '',
    every: a?.every ? String(a.every) : '',
    free: a?.free ? String(a.free) : '',
  };
}

// ---------------------------------------------------------------------------
// Conditions
// ---------------------------------------------------------------------------

const dateRangeSchema = z.object({ from: optionalIsoDate, to: optionalIsoDate });
const codeList = z
  .string()
  .trim()
  .refine((v) => splitCodes(v).every((c) => CODE_RE.test(c)), V.code);

export const conditionFields = {
  rate_periods: z.array(z.string()),
  date_ranges: z.array(dateRangeSchema),
  weekdays: z.array(z.string()),
  slots: z.array(z.string()),
  formulas: z.array(z.string()),
  channels: z.array(z.string()),
  zone_ids: z.array(z.string()),
  agreement_ids: z.array(z.string()),
  price_categories: codeList,
  spot_attributes: codeList,
  customer_tags: codeList,
  holidays: z.enum(['any', 'only', 'exclude']),
  min_days: intField({ min: 1, max: 400, optional: true }),
  max_days: intField({ min: 1, max: 400, optional: true }),
  lead_min: intField({ min: 0, max: 1000, optional: true }),
  lead_max: intField({ min: 0, max: 1000, optional: true }),
  occupancy_min: intField({ min: 0, max: 100, optional: true }),
  occupancy_max: intField({ min: 0, max: 100, optional: true }),
};

export type ConditionValues = {
  rate_periods: string[];
  date_ranges: { from: string; to: string }[];
  weekdays: string[];
  slots: string[];
  formulas: string[];
  channels: string[];
  zone_ids: string[];
  agreement_ids: string[];
  price_categories: string;
  spot_attributes: string;
  customer_tags: string;
  holidays: 'any' | 'only' | 'exclude';
  min_days: string;
  max_days: string;
  lead_min: string;
  lead_max: string;
  occupancy_min: string;
  occupancy_max: string;
};

export function refineConditions(v: ConditionValues, ctx: z.RefinementCtx) {
  const issue = (path: (string | number)[], message: string) =>
    ctx.addIssue({ code: 'custom', path, message });
  v.date_ranges.forEach((r, i) => {
    if (r.from === '' && r.to === '') return;
    if (r.from === '') issue(['date_ranges', i, 'from'], V.required);
    else if (r.to === '') issue(['date_ranges', i, 'to'], V.required);
    else if (r.to < r.from) issue(['date_ranges', i, 'to'], V.dateOrder);
  });
  const lessThan = (lo: string, hi: string) => lo !== '' && hi !== '' && Number(hi) < Number(lo);
  if (lessThan(v.min_days, v.max_days)) issue(['max_days'], 'pricing:validation.maxDays');
  if (lessThan(v.lead_min, v.lead_max)) issue(['lead_max'], 'pricing:validation.rangeOrder');
  if (lessThan(v.occupancy_min, v.occupancy_max)) {
    issue(['occupancy_max'], 'pricing:validation.rangeOrder');
  }
}

export function splitCodes(v: string): string[] {
  return [
    ...new Set(
      v
        .split(/[\s,;]+/)
        .map((s) => s.trim().toLowerCase())
        .filter(Boolean),
    ),
  ];
}

function intRange(min: string, max: string): { min?: number; max?: number } | undefined {
  const lo = optionalInt(min);
  const hi = optionalInt(max);
  if (lo === undefined && hi === undefined) return undefined;
  return { ...(lo !== undefined ? { min: lo } : {}), ...(hi !== undefined ? { max: hi } : {}) };
}

export function toConditions(v: ConditionValues): PriceConditions {
  const c: PriceConditions = {};
  if (v.rate_periods.length) c.rate_periods = v.rate_periods;
  const ranges = v.date_ranges
    .filter((r) => isCalendarDate(r.from) && isCalendarDate(r.to))
    .map((r) => ({ from: r.from, to: r.to }));
  if (ranges.length) c.date_ranges = ranges;
  if (v.weekdays.length) c.weekdays = v.weekdays.map(Number).sort((a, b) => a - b);
  if (v.holidays !== 'any') c.holidays = v.holidays === 'only';
  if (v.slots.length) c.slots = v.slots as PriceConditions['slots'];
  if (v.formulas.length) c.formulas = v.formulas as PriceConditions['formulas'];
  const cats = splitCodes(v.price_categories);
  if (cats.length) c.price_categories = cats;
  if (v.zone_ids.length) c.zone_ids = v.zone_ids;
  const attrs = splitCodes(v.spot_attributes);
  if (attrs.length) c.spot_attributes = attrs;
  if (v.channels.length) c.channels = v.channels as PriceConditions['channels'];
  const min = optionalInt(v.min_days);
  const max = optionalInt(v.max_days);
  if (min !== undefined) c.min_days = min;
  if (max !== undefined) c.max_days = max;
  const lead = intRange(v.lead_min, v.lead_max);
  if (lead) c.lead_time_days = lead;
  const occupancy = intRange(v.occupancy_min, v.occupancy_max);
  if (occupancy) c.occupancy_percent = occupancy;
  const tags = splitCodes(v.customer_tags);
  if (tags.length) c.customer_tags = tags;
  if (v.agreement_ids.length) c.agreement_ids = v.agreement_ids;
  return c;
}

const intText = (n: number | undefined) => (n == null ? '' : String(n));

export function conditionValues(c: PriceConditions | undefined): ConditionValues {
  return {
    rate_periods: c?.rate_periods ?? [],
    date_ranges: (c?.date_ranges ?? []).map((r) => ({ from: r.from, to: r.to })),
    weekdays: (c?.weekdays ?? []).map(String),
    slots: c?.slots ?? [],
    formulas: c?.formulas ?? [],
    channels: c?.channels ?? [],
    zone_ids: c?.zone_ids ?? [],
    agreement_ids: c?.agreement_ids ?? [],
    price_categories: (c?.price_categories ?? []).join(', '),
    spot_attributes: (c?.spot_attributes ?? []).join(', '),
    customer_tags: (c?.customer_tags ?? []).join(', '),
    holidays: c?.holidays === undefined ? 'any' : c.holidays ? 'only' : 'exclude',
    min_days: intText(c?.min_days),
    max_days: intText(c?.max_days),
    lead_min: intText(c?.lead_time_days?.min),
    lead_max: intText(c?.lead_time_days?.max),
    occupancy_min: intText(c?.occupancy_percent?.min),
    occupancy_max: intText(c?.occupancy_percent?.max),
  };
}

/**
 * problem+json field paths (API shape) → flat form fields, so server-side
 * validation errors land on the right input. Longer paths come first: the
 * first matching prefix wins.
 */
export const FORM_ALIASES: Record<string, string> = {
  'action.type': 'action_type',
  'action.amount': 'amount',
  'action.percent': 'percent',
  'action.factor': 'factor',
  'action.per': 'per',
  'action.every': 'every',
  'action.free': 'free',
  'conditions.lead_time_days.max': 'lead_max',
  'conditions.lead_time_days': 'lead_min',
  'conditions.occupancy_percent.max': 'occupancy_max',
  'conditions.occupancy_percent': 'occupancy_min',
  ...Object.fromEntries(
    [
      'rate_periods',
      'date_ranges',
      'weekdays',
      'holidays',
      'slots',
      'formulas',
      'price_categories',
      'zone_ids',
      'spot_attributes',
      'channels',
      'min_days',
      'max_days',
      'customer_tags',
      'agreement_ids',
    ].map((k) => [`conditions.${k}`, k]),
  ),
};

// ---------------------------------------------------------------------------
// Price rule
// ---------------------------------------------------------------------------

export const ruleSchema = z
  .object({
    name: z.string().trim().min(1, V.required).max(200),
    stage: z.enum(['base', 'modifier', 'discount']),
    target_type: z.enum(['pack', 'extra']),
    target_ids: z.array(z.string()),
    priority: z
      .string()
      .trim()
      .regex(/^-?\d{1,6}$/, V.integer),
    stackable: z.boolean(),
    active: z.boolean(),
    ...actionFields,
    ...conditionFields,
  })
  .superRefine((v, ctx) => {
    if (!ACTIONS_BY_STAGE[v.stage].includes(v.action_type)) {
      ctx.addIssue({
        code: 'custom',
        path: ['action_type'],
        message: 'pricing:validation.actionStage',
      });
    }
    refineAction(v, ctx);
    refineConditions(v, ctx);
  });

export type RuleValues = z.infer<typeof ruleSchema>;

export function ruleValues(r: PriceRule | undefined): RuleValues {
  return {
    name: r?.name ?? '',
    stage: r?.stage ?? 'base',
    target_type: r?.target_type ?? 'pack',
    target_ids: r?.target_ids ?? [],
    priority: String(r?.priority ?? 0),
    stackable: r?.stackable ?? false,
    active: r?.active ?? true,
    ...actionValues(r?.action, 'set'),
    ...conditionValues(r?.conditions),
  };
}

export function toRuleInput(seasonId: string, v: RuleValues): PriceRuleInput {
  const input: PriceRuleInput = {
    season_id: seasonId,
    name: v.name.trim(),
    stage: v.stage,
    target_type: v.target_type,
    conditions: toConditions(v),
    action: toAction(v),
    priority: Number(v.priority),
    stackable: v.stackable,
    active: v.active,
  };
  if (v.target_ids.length) input.target_ids = v.target_ids;
  return input;
}

// ---------------------------------------------------------------------------
// Fixed-duration price
// ---------------------------------------------------------------------------

export const fixedSchema = z.object({
  pack_id: z.string().min(1, V.required),
  formula: z.enum(FIXED_FORMULAS),
  price_category: z.string().trim().toLowerCase().regex(CODE_RE, V.code),
  rate_period_code: z.string(),
  amount: z
    .string()
    .trim()
    .refine((v) => euroToCents(v) !== null, V.amount),
});

export type FixedValues = z.infer<typeof fixedSchema>;

export function fixedValues(f: FixedPrice | undefined): FixedValues {
  return {
    pack_id: f?.pack_id ?? '',
    formula: f?.formula ?? 'week',
    price_category: f?.price_category ?? '',
    rate_period_code: f?.rate_period_code ?? '',
    amount: f ? centsToEuroInput(f.amount) : '',
  };
}

export function toFixedInput(seasonId: string, v: FixedValues): FixedPriceInput {
  const input: FixedPriceInput = {
    season_id: seasonId,
    pack_id: v.pack_id,
    formula: v.formula,
    price_category: v.price_category,
    amount: euroToCents(v.amount) ?? 0,
  };
  if (v.rate_period_code) input.rate_period_code = v.rate_period_code;
  return input;
}

// ---------------------------------------------------------------------------
// Promo codes and agreements (discount-stage actions)
// ---------------------------------------------------------------------------

const DISCOUNT_CODE_RE = /^[A-Za-z0-9][A-Za-z0-9_-]{1,39}$/;

const discountBase = {
  valid_from: optionalIsoDate,
  valid_to: optionalIsoDate,
  stackable: z.boolean(),
  active: z.boolean(),
  ...actionFields,
  ...conditionFields,
};

function refineDiscount(
  v: ActionValues & ConditionValues & { valid_from: string; valid_to: string },
  ctx: z.RefinementCtx,
) {
  if (!ACTIONS_BY_STAGE.discount.includes(v.action_type)) {
    ctx.addIssue({
      code: 'custom',
      path: ['action_type'],
      message: 'pricing:validation.actionStage',
    });
  }
  if (v.valid_from && v.valid_to && v.valid_to < v.valid_from) {
    ctx.addIssue({ code: 'custom', path: ['valid_to'], message: V.dateOrder });
  }
  refineAction(v, ctx);
  refineConditions(v, ctx);
}

export const promoSchema = z
  .object({
    code: z.string().trim().regex(DISCOUNT_CODE_RE, 'pricing:validation.promoCode'),
    description: z.string().trim().max(500),
    max_uses: intField({ min: 1, optional: true }),
    max_uses_per_customer: intField({ min: 1, optional: true }),
    ...discountBase,
  })
  .superRefine(refineDiscount);

export type PromoValues = z.infer<typeof promoSchema>;

export function promoValues(p: PromoCode | undefined): PromoValues {
  return {
    code: p?.code ?? '',
    description: p?.description ?? '',
    max_uses: p?.max_uses ? String(p.max_uses) : '',
    max_uses_per_customer: p?.max_uses_per_customer ? String(p.max_uses_per_customer) : '',
    valid_from: p?.valid_from ?? '',
    valid_to: p?.valid_to ?? '',
    stackable: p?.stackable ?? false,
    active: p?.active ?? true,
    ...actionValues(p?.action, 'percent_off'),
    ...conditionValues(p?.conditions),
  };
}

export function toPromoInput(v: PromoValues): PromoCodeInput {
  const input: PromoCodeInput = {
    code: v.code.trim(),
    description: v.description.trim(),
    action: toAction(v),
    conditions: toConditions(v),
    stackable: v.stackable,
    active: v.active,
  };
  if (v.valid_from) input.valid_from = v.valid_from;
  if (v.valid_to) input.valid_to = v.valid_to;
  const max = optionalInt(v.max_uses);
  if (max !== undefined) input.max_uses = max;
  const perCustomer = optionalInt(v.max_uses_per_customer);
  if (perCustomer !== undefined) input.max_uses_per_customer = perCustomer;
  return input;
}

export const agreementSchema = z
  .object({
    name: z.string().trim().min(1, V.required).max(200),
    code: z
      .string()
      .trim()
      .refine((v) => v === '' || DISCOUNT_CODE_RE.test(v), 'pricing:validation.promoCode'),
    partner_name: z.string().trim().max(200),
    notes: z.string().trim().max(2000),
    ...discountBase,
    ...agreementM8Fields,
  })
  .superRefine((v, ctx) => {
    refineDiscount(v, ctx);
    refineAgreementM8(v, ctx);
  });

export type AgreementValues = z.infer<typeof agreementSchema>;

export function agreementValues(a: Agreement | undefined): AgreementValues {
  return {
    name: a?.name ?? '',
    code: a?.code ?? '',
    partner_name: a?.partner_name ?? '',
    notes: a?.notes ?? '',
    valid_from: a?.valid_from ?? '',
    valid_to: a?.valid_to ?? '',
    stackable: a?.stackable ?? false,
    active: a?.active ?? true,
    ...actionValues(a?.action, 'percent_off'),
    ...conditionValues(a?.conditions),
    ...agreementM8Values(a),
  };
}

export function toAgreementInput(v: AgreementValues): AgreementInput {
  const input: AgreementInput = {
    name: v.name.trim(),
    partner_name: v.partner_name.trim(),
    notes: v.notes.trim(),
    action: toAction(v),
    conditions: toConditions(v),
    stackable: v.stackable,
    active: v.active,
    ...toAgreementM8Input(v),
  };
  if (v.code) input.code = v.code.trim();
  if (v.valid_from) input.valid_from = v.valid_from;
  if (v.valid_to) input.valid_to = v.valid_to;
  return input;
}

// ---------------------------------------------------------------------------
// Season copy
// ---------------------------------------------------------------------------

export const copySchema = z.object({
  target_year: intField({ min: 2000, max: 2100 }),
  include_rate_periods: z.boolean(),
  include_price_rules: z.boolean(),
  include_fixed_prices: z.boolean(),
  include_policies: z.boolean(),
});

export type CopyValues = z.infer<typeof copySchema>;
