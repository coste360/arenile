import { describe, expect, it } from 'vitest';
import {
  actionValues,
  conditionValues,
  promoSchema,
  promoValues,
  ruleSchema,
  ruleValues,
  signedEuroToCents,
  toAction,
  toConditions,
  toPromoInput,
  toRuleInput,
} from './rules.schema';
import { rulesValues, toRules } from '../policies/policy.schema';

describe('signedEuroToCents', () => {
  it.each([
    ['5', 500],
    ['-2,50', -250],
    ['0.99', 99],
    ['abc', null],
    ['1,234', null],
  ])('%s → %s', (input, want) => {
    expect(signedEuroToCents(input)).toBe(want);
  });
});

describe('rule form', () => {
  it('round-trips a weekend modifier on two periods', () => {
    const rule = {
      season_id: 's1',
      name: 'Weekend',
      target_type: 'pack' as const,
      target_ids: ['p1'],
      stage: 'modifier' as const,
      action: { type: 'add' as const, amount: -250 },
      conditions: { weekdays: [6, 7], rate_periods: ['high', 'peak'], price_categories: ['row1'] },
      priority: 10,
      stackable: true,
      active: true,
    };
    const values = ruleValues({
      ...rule,
      id: 'r1',
      seq: 1,
      version: 1,
      created_at: '',
      updated_at: '',
    });
    expect(values.amount).toBe('-2,50');
    expect(ruleSchema.safeParse(values).success).toBe(true);
    expect(toRuleInput('s1', values)).toEqual(rule);
  });

  it('rejects an action not allowed in the stage and incoherent dates', () => {
    const values = {
      ...ruleValues(undefined),
      action_type: 'percent_off' as const,
      percent: '10',
    };
    const bad = ruleSchema.safeParse({
      ...values,
      date_ranges: [
        { from: '2027-08-10', to: '2027-08-01' },
        { from: '2027-07-01', to: '' },
      ],
      lead_min: '5',
      lead_max: '2',
    });
    expect(bad.success).toBe(false);
    const paths = bad.error?.issues.map((i) => i.path.join('.'));
    expect(paths).toEqual(
      expect.arrayContaining([
        'action_type',
        'date_ranges.0.to',
        'date_ranges.1.to',
        'lead_max',
        'name',
      ]),
    );
  });

  it('converts euro amounts without floating point', () => {
    const values = { ...ruleValues(undefined), name: 'Base', amount: '19,99' };
    expect(toRuleInput('s1', values).action).toEqual({ type: 'set', amount: 1999 });
  });
});

describe('conditions', () => {
  it('round-trips every condition of spec 8.2', () => {
    const conditions = {
      rate_periods: ['high', 'peak'],
      date_ranges: [
        { from: '2027-08-10', to: '2027-08-20' },
        { from: '2027-12-24', to: '2027-12-26' },
      ],
      weekdays: [6, 7],
      holidays: true,
      slots: ['AM' as const],
      formulas: ['day' as const, 'multi_day' as const],
      price_categories: ['row1', 'row2'],
      zone_ids: ['z1'],
      spot_attributes: ['accessible'],
      channels: ['online' as const],
      min_days: 3,
      max_days: 6,
      lead_time_days: { min: 0, max: 1 },
      occupancy_percent: { min: 80 },
      customer_tags: ['storico'],
      agreement_ids: ['a1'],
    };
    const values = conditionValues(conditions);
    expect(values.occupancy_min).toBe('80');
    expect(values.occupancy_max).toBe('');
    expect(toConditions(values)).toEqual(conditions);
  });

  it('normalises code lists and drops empty date rows', () => {
    const values = {
      ...conditionValues(undefined),
      price_categories: ' Row1, row2;row1 ',
      date_ranges: [{ from: '', to: '' }],
      weekdays: ['7', '6'],
    };
    expect(toConditions(values)).toEqual({ price_categories: ['row1', 'row2'], weekdays: [6, 7] });
  });
});

describe('euro and percent inputs (exact, integer based)', () => {
  const base = { ...ruleValues(undefined), name: 'X' };
  it.each([
    ['0,10', 10],
    ['0,29', 29],
    ['1,005', null],
    ['19.99', 1999],
    ['1.234,56', 123456],
    ['abc', null],
    ['4,35', 435],
    ['1000000', 100000000],
  ])('set %s → %s cents', (amount, cents) => {
    const values = { ...base, amount };
    const parsed = ruleSchema.safeParse(values);
    expect(parsed.success).toBe(cents !== null);
    if (cents !== null)
      expect(toRuleInput('s1', values).action).toEqual({ type: 'set', amount: cents });
  });

  it.each([
    ['10', '10'],
    ['12,5', '12.5'],
    ['33,33', '33.33'],
    ['0,07', '0.07'],
    ['100', '100'],
  ])('percent_off %s → Decimal %s', (percent, decimal) => {
    const values = {
      ...base,
      stage: 'discount' as const,
      action_type: 'percent_off' as const,
      percent,
    };
    expect(ruleSchema.safeParse(values).success).toBe(true);
    expect(toRuleInput('s1', values).action).toEqual({ type: 'percent_off', percent: decimal });
  });

  it.each(['0', '100,01', '-5', '12,345', 'dieci'])('rejects percent %s', (percent) => {
    const values = {
      ...base,
      stage: 'discount' as const,
      action_type: 'percent_off' as const,
      percent,
    };
    expect(ruleSchema.safeParse(values).success).toBe(false);
  });

  it('keeps factors exact and renders them back with the locale separator', () => {
    const values = {
      ...base,
      stage: 'modifier' as const,
      action_type: 'multiply' as const,
      factor: '1,2',
    };
    const action = toRuleInput('s1', values).action;
    expect(action).toEqual({ type: 'multiply', factor: '1.2' });
    expect(actionValues(action, 'set').factor).toBe('1,2');
  });

  it('round-trips cents through the input string for every cent value up to 10 €', () => {
    for (let c = 0; c <= 1000; c++) {
      const v = actionValues({ type: 'set', amount: c }, 'set');
      expect(toAction(v)).toEqual({ type: 'set', amount: c });
    }
  });
});

describe('promo form', () => {
  it('builds a 10% code with validity and limits', () => {
    const values = {
      ...promoValues(undefined),
      code: 'BENVENUTO10',
      percent: '10',
      valid_from: '2027-05-01',
      valid_to: '2027-06-30',
      max_uses: '100',
    };
    expect(promoSchema.safeParse(values).success).toBe(true);
    expect(toPromoInput(values)).toMatchObject({
      code: 'BENVENUTO10',
      action: { type: 'percent_off', percent: '10' },
      valid_from: '2027-05-01',
      valid_to: '2027-06-30',
      max_uses: 100,
      conditions: {},
    });
  });
});

describe('policy rules form', () => {
  it('round-trips deposit, balance, cancellation tiers and tri-states', () => {
    const rules = {
      min_lead_time: { days: 1, cutoff_time: '18:00' },
      duration: { allowed_formulas: ['week' as const], fixed_start_weekday: 6 },
      payment: {
        mode: 'deposit' as const,
        deposit: { type: 'percent' as const, value: 30 },
        balance_due: 'days_before:7',
      },
      cancellation: [
        { until_days_before: 14, refund_percent: 100, as: 'refund' as const },
        { until_days_before: 3, refund_percent: 50, as: 'credit' as const },
      ],
      approval_required: true,
      weather_policy: { enabled: true, as: 'credit' as const },
    };
    expect(toRules(rulesValues(rules))).toEqual(rules);
  });

  it('leaves out everything that inherits', () => {
    expect(toRules(rulesValues({}))).toEqual({});
  });
});
