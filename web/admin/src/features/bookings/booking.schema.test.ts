import { describe, expect, it } from 'vitest';
import {
  cancelSchema,
  changeDatesSchema,
  newBookingSchema,
  newBookingValues,
  normalizePhone,
  paymentSchema,
  refundSchema,
  toBookingCreate,
  toCancelRequest,
  toInitialPayment,
  toRefundInput,
  toSelection,
  type NewBookingValues,
} from './booking.schema';
import { availableActions, defaultFormula, eachDate } from './model';

const ctx = {
  zoneOfSpot: (id: string) => (id === 's1' || id === 's2' ? 'z1' : undefined),
  lineExtraIds: new Set(['x-line']),
};

function values(overrides: Partial<NewBookingValues> = {}): NewBookingValues {
  return {
    ...newBookingValues('2026-07-14', { spotIds: ['s1'] }),
    pack_id: 'p1',
    customer_id: 'c1',
    ...overrides,
  };
}

function issues(v: unknown) {
  const r = newBookingSchema.safeParse(v);
  return r.success ? [] : r.error.issues.map((i) => `${i.path.join('.')}:${i.message}`);
}

describe('newBookingSchema', () => {
  it('accepts a complete desk booking', () => {
    expect(issues(values())).toEqual([]);
  });

  it('requires at least one spot in spot mode and a zone in zone mode', () => {
    expect(issues(values({ spot_ids: [] }))).toContain('spot_ids:bookings:validation.spots');
    expect(issues(values({ target: 'zone', zone_id: '' }))).toContain(
      'zone_id:bookings:validation.zone',
    );
  });

  it('rejects an end date before the start', () => {
    expect(issues(values({ ends_on: '2026-07-10' }))).toContain(
      'ends_on:bookings:validation.dateOrder',
    );
  });

  it('requires an existing customer or a valid inline one with a contact', () => {
    expect(issues(values({ customer_id: '' }))).toContain(
      'customer_id:bookings:validation.customer',
    );
    const inline = values({
      customer_mode: 'new',
      customer_id: '',
      customer: { first_name: '', last_name: 'Bianchi', email: '', phone: '' },
    });
    expect(issues(inline)).toContain('customer.phone:bookings:validation.contact');
    expect(
      issues({
        ...inline,
        customer: { first_name: '', last_name: 'Bianchi', email: '', phone: '333 1234567' },
      }),
    ).toEqual([]);
    expect(
      issues({
        ...inline,
        customer: { first_name: '', last_name: '', email: 'x@y.it', phone: '' },
      }),
    ).toContain('customer.last_name:bookings:validation.lastName');
  });

  it('ignores a half-typed inline customer once an existing one is chosen', () => {
    const v = values({ customer: { first_name: '', last_name: '', email: 'bad', phone: '' } });
    expect(issues(v)).toEqual([]);
  });

  it('requires a positive amount when paying now', () => {
    expect(issues(values({ pay_now: true, payment_amount: null }))).toContain(
      'payment_amount:bookings:validation.amount',
    );
    expect(issues(values({ pay_now: true, payment_amount: 2500 }))).toEqual([]);
  });
});

describe('request builders', () => {
  it('builds one line per selected spot with its zone, splitting line and booking extras', () => {
    const sel = toSelection(
      values({ spot_ids: ['s1', 's2'], extras: { 'x-line': 1, 'x-booking': 2, 'x-zero': 0 } }),
      ctx,
    );
    expect(sel).toEqual({
      channel: 'desk',
      starts_on: '2026-07-14',
      ends_on: '2026-07-14',
      slots: 'FULL',
      formula: 'day',
      lines: [
        {
          pack_id: 'p1',
          spot_id: 's1',
          zone_id: 'z1',
          extras: [{ extra_id: 'x-line', quantity: 1 }],
        },
        {
          pack_id: 'p1',
          spot_id: 's2',
          zone_id: 'z1',
          extras: [{ extra_id: 'x-line', quantity: 1 }],
        },
      ],
      extras: [{ extra_id: 'x-booking', quantity: 2 }],
    });
  });

  it('builds a zone line with quantity', () => {
    const sel = toSelection(values({ target: 'zone', zone_id: 'zq', quantity: 3 }), ctx);
    expect(sel?.lines).toEqual([{ pack_id: 'p1', zone_id: 'zq', quantity: 3 }]);
  });

  it('returns null for incomplete selections (no quote request)', () => {
    expect(toSelection(values({ pack_id: '' }), ctx)).toBeNull();
    expect(toSelection(values({ spot_ids: [] }), ctx)).toBeNull();
    expect(toSelection(values({ ends_on: '2026-07-01' }), ctx)).toBeNull();
  });

  it('creates a confirmed booking with an inline customer (normalised contacts) and the hold to convert', () => {
    const body = toBookingCreate(
      values({
        customer_mode: 'new',
        customer: {
          first_name: ' Anna ',
          last_name: 'Verdi',
          email: ' Anna@Example.IT ',
          phone: '333 123 4567',
        },
        notes_internal: ' lettino extra ',
        force: true,
      }),
      { ...ctx, holdId: 'h1' },
    );
    expect(body).toMatchObject({
      status: 'confirmed',
      hold_id: 'h1',
      customer: {
        first_name: 'Anna',
        last_name: 'Verdi',
        email: 'anna@example.it',
        phone: '+393331234567',
      },
      notes_internal: 'lettino extra',
      force: true,
    });
    expect(body?.customer_id).toBeUndefined();
  });

  it('pays now only with a positive amount', () => {
    expect(toInitialPayment(values({ pay_now: false, payment_amount: 100 }))).toBeNull();
    expect(
      toInitialPayment(values({ pay_now: true, payment_amount: 2500, payment_method: 'card' })),
    ).toEqual({
      amount: 2500,
      method: 'card',
    });
  });

  it('normalises phone numbers to E.164', () => {
    expect(normalizePhone('333 123 4567')).toBe('+393331234567');
    expect(normalizePhone('0049 170 1234567')).toBe('+491701234567');
    expect(normalizePhone('+39 (333) 123-4567')).toBe('+393331234567');
    expect(normalizePhone('  ')).toBe('');
  });
});

describe('payment, refund and cancel schemas', () => {
  it('limits a refund to the paid amount (INV-24)', () => {
    const schema = refundSchema(5000);
    expect(
      schema.safeParse({ amount: 5001, method: 'cash', as: 'refund', reason: '' }).success,
    ).toBe(false);
    expect(
      schema.safeParse({ amount: 5000, method: 'cash', as: 'refund', reason: '' }).success,
    ).toBe(true);
    expect(toRefundInput({ amount: 100, method: 'cash', as: 'credit', reason: '' })).toEqual({
      amount: 100,
      method: 'credit',
      as: 'credit',
    });
  });

  it('rejects zero and fractional payments', () => {
    expect(paymentSchema().safeParse({ amount: 0, method: 'cash', note: '' }).success).toBe(false);
    expect(paymentSchema().safeParse({ amount: 10.5, method: 'cash', note: '' }).success).toBe(
      false,
    );
    expect(paymentSchema(1000).safeParse({ amount: 1001, method: 'card', note: '' }).success).toBe(
      false,
    );
  });

  it('validates forced refund amounts and builds the cancel request', () => {
    const schema = cancelSchema(5000);
    const base = {
      settlement: 'refund' as const,
      refund_amount: 6000,
      refund_method: 'cash',
      weather: false,
      reason: '',
    };
    expect(schema.safeParse(base).success).toBe(false);
    expect(schema.safeParse({ ...base, refund_amount: 2000 }).success).toBe(true);
    expect(
      toCancelRequest({ ...base, refund_amount: 2000, reason: 'pioggia', weather: true }, true),
    ).toEqual({
      settlement: 'refund',
      refund_amount: 2000,
      refund_method: 'cash',
      weather: true,
      reason: 'pioggia',
      dry_run: true,
    });
    expect(toCancelRequest({ ...base, settlement: 'none', refund_amount: 2000 }, false)).toEqual({
      settlement: 'none',
    });
    expect(toCancelRequest({ ...base, settlement: 'credit', refund_amount: 1500 }, false)).toEqual({
      settlement: 'credit',
      refund_amount: 1500,
    });
  });

  it('orders the new dates of a change', () => {
    expect(
      changeDatesSchema.safeParse({
        starts_on: '2026-07-10',
        ends_on: '2026-07-09',
        slots: 'FULL',
        force: false,
      }).success,
    ).toBe(false);
  });
});

describe('model helpers', () => {
  it('derives the default formula', () => {
    expect(defaultFormula('2026-07-14', '2026-07-14', 'AM')).toBe('half_day');
    expect(defaultFormula('2026-07-14', '2026-07-14', 'FULL')).toBe('day');
    expect(defaultFormula('2026-07-14', '2026-07-16', 'FULL')).toBe('multi_day');
  });

  it('offers actions from the server state machine', () => {
    expect(availableActions('confirmed', ['checked_in', 'cancelled', 'no_show'])).toEqual([
      'check-in',
      'no-show',
      'cancel',
    ]);
    expect(availableActions('awaiting_approval', ['confirmed', 'rejected', 'cancelled'])).toEqual([
      'approve',
      'reject',
      'cancel',
    ]);
    expect(availableActions('held', ['confirmed', 'expired'])).toEqual(['confirm']);
    expect(availableActions('cancelled', [])).toEqual([]);
  });

  it('enumerates dates inclusively', () => {
    expect(eachDate('2026-07-30', '2026-08-02')).toEqual([
      '2026-07-30',
      '2026-07-31',
      '2026-08-01',
      '2026-08-02',
    ]);
    expect(eachDate('2026-08-02', '2026-07-30')).toEqual([]);
  });
});
