/**
 * Pure arithmetic of the cash desk (spec 14.1, INV-25). Every amount is an
 * integer number of cents: values are only added and subtracted, never
 * divided or multiplied by fractions, so no floating point error can appear.
 * Formatting to euro happens at display time (formatCents).
 */
import type { Schemas } from '@arenile/shared/api';

export type PaymentMethod = Schemas['PaymentMethod'];
export type MethodTotals = Partial<Record<PaymentMethod, number>>;

/** Display order of the methods (spec 6.7). */
export const PAYMENT_METHODS: readonly PaymentMethod[] = [
  'cash',
  'card',
  'bank_transfer',
  'wallet',
  'credit',
  'other',
];

/** Methods a cashier can take at the desk (credit is the customer balance, not money). */
export const DESK_METHODS: readonly PaymentMethod[] = [
  'cash',
  'card',
  'bank_transfer',
  'wallet',
  'other',
];

export interface MovementLike {
  kind: 'in' | 'out';
  amount: number;
}

export interface SessionLike {
  status: 'open' | 'closed';
  opening_float: number;
  expected_totals: Record<string, number>;
  counted_totals?: Record<string, number>;
  difference?: number;
}

export interface SessionDetailLike {
  session: SessionLike;
  movements: MovementLike[];
  totals_by_method: Record<string, number>;
}

function isMethod(m: string): m is PaymentMethod {
  return (PAYMENT_METHODS as readonly string[]).includes(m);
}

/** Copies a wire MethodTotals keeping only known methods with safe integer amounts. */
export function normalizeTotals(totals: Record<string, number> | undefined): MethodTotals {
  const out: MethodTotals = {};
  for (const [m, v] of Object.entries(totals ?? {})) {
    if (isMethod(m) && Number.isSafeInteger(v)) out[m] = v;
  }
  return out;
}

/** Sum of every method. */
export function sumTotals(totals: MethodTotals): number {
  let sum = 0;
  for (const m of PAYMENT_METHODS) sum += totals[m] ?? 0;
  return sum;
}

/** Cash moved in minus cash moved out (versamenti - prelievi). */
export function netMovements(movements: readonly MovementLike[]): {
  in: number;
  out: number;
  net: number;
} {
  let inSum = 0;
  let outSum = 0;
  for (const mv of movements) {
    if (mv.kind === 'in') inSum += mv.amount;
    else outSum += mv.amount;
  }
  return { in: inSum, out: outSum, net: inSum - outSum };
}

/**
 * Expected totals per method (INV-25): takings of the session per method,
 * plus, on cash, the opening float and the net cash movements. A closed
 * session carries the server-computed figure, which is authoritative.
 */
export function expectedTotals(detail: SessionDetailLike): MethodTotals {
  if (detail.session.status === 'closed') return normalizeTotals(detail.session.expected_totals);
  const out = normalizeTotals(detail.totals_by_method);
  out.cash = (out.cash ?? 0) + detail.session.opening_float + netMovements(detail.movements).net;
  return out;
}

/** Methods worth showing: cash always, then every method with an amount. */
export function methodsShown(...totals: MethodTotals[]): PaymentMethod[] {
  return PAYMENT_METHODS.filter(
    (m) => m === 'cash' || totals.some((t) => t[m] !== undefined && t[m] !== 0),
  );
}

export interface Difference {
  byMethod: MethodTotals;
  total: number;
}

/** counted - expected, per method and in total (positive = surplus, negative = shortfall). */
export function difference(counted: MethodTotals, expected: MethodTotals): Difference {
  const byMethod: MethodTotals = {};
  for (const m of PAYMENT_METHODS) {
    if (counted[m] === undefined && expected[m] === undefined) continue;
    byMethod[m] = (counted[m] ?? 0) - (expected[m] ?? 0);
  }
  return { byMethod, total: sumTotals(counted) - sumTotals(expected) };
}

export type DifferenceTone = 'balanced' | 'surplus' | 'shortfall';

export function differenceTone(cents: number): DifferenceTone {
  if (cents === 0) return 'balanced';
  return cents > 0 ? 'surplus' : 'shortfall';
}

/** Counted amounts as sent to the API: methods left empty are not reported. */
export function toCountedTotals(
  counted: Record<string, number | null | undefined>,
  methods: readonly PaymentMethod[],
): MethodTotals {
  const out: MethodTotals = {};
  for (const m of methods) {
    const v = counted[m];
    if (v != null) out[m] = v;
  }
  return out;
}
