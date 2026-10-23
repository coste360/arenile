import { describe, expect, it } from 'vitest';
import {
  difference,
  differenceTone,
  expectedTotals,
  methodsShown,
  netMovements,
  normalizeTotals,
  sumTotals,
  toCountedTotals,
  type SessionDetailLike,
} from './totals';

const open = (over: Partial<SessionDetailLike> = {}): SessionDetailLike => ({
  session: { status: 'open', opening_float: 10000, expected_totals: {} },
  movements: [
    { kind: 'in', amount: 2000 },
    { kind: 'out', amount: 900 },
  ],
  totals_by_method: { cash: 14500, card: 3000 },
  ...over,
});

describe('expectedTotals', () => {
  it('TestExpectedTotals_OpenSession adds float and net movements to cash only', () => {
    expect(expectedTotals(open())).toEqual({ cash: 25600, card: 3000 });
  });

  it('TestExpectedTotals_NoTakings is the float plus movements', () => {
    expect(expectedTotals(open({ totals_by_method: {}, movements: [] }))).toEqual({ cash: 10000 });
  });

  it('TestExpectedTotals_ClosedSession uses the server figure', () => {
    const closed = open({
      session: {
        status: 'closed',
        opening_float: 10000,
        expected_totals: { cash: 99999, bogus: 5 } as Record<string, number>,
      },
    });
    expect(expectedTotals(closed)).toEqual({ cash: 99999 });
  });
});

describe('difference (INV-25: counted - expected, integer cents)', () => {
  it.each([
    { counted: { cash: 25590, card: 3000 }, total: -10, cash: -10, tone: 'shortfall' },
    { counted: { cash: 25600, card: 3000 }, total: 0, cash: 0, tone: 'balanced' },
    { counted: { cash: 25601, card: 3000 }, total: 1, cash: 1, tone: 'surplus' },
    { counted: { cash: 25600 }, total: -3000, cash: 0, tone: 'shortfall' },
  ])('counted $counted → $total', ({ counted, total, cash, tone }) => {
    const d = difference(counted, { cash: 25600, card: 3000 });
    expect(d.total).toBe(total);
    expect(d.byMethod.cash).toBe(cash);
    expect(Number.isInteger(d.total)).toBe(true);
    expect(differenceTone(d.total)).toBe(tone);
  });

  it('is exact where floats are not (0.1 + 0.2)', () => {
    const d = difference({ cash: 10, card: 20 }, { cash: 30 });
    expect(d.total).toBe(0);
    expect(d.byMethod).toEqual({ cash: -20, card: 20 });
  });

  it('stays exact on large amounts', () => {
    const big = 9_007_199_254_740; // ~90 billion euro
    expect(difference({ cash: big + 1 }, { cash: big }).total).toBe(1);
  });
});

describe('helpers', () => {
  it('netMovements splits in and out', () => {
    expect(netMovements(open().movements)).toEqual({ in: 2000, out: 900, net: 1100 });
  });

  it('normalizeTotals drops unknown methods and non-integers', () => {
    expect(normalizeTotals({ cash: 100, foo: 1, card: 1.5 })).toEqual({ cash: 100 });
    expect(sumTotals({ cash: 100, card: 250, other: -50 })).toBe(300);
  });

  it('methodsShown keeps cash first and only methods with amounts', () => {
    expect(methodsShown({ card: 0 }, { wallet: 5 })).toEqual(['cash', 'wallet']);
  });

  it('toCountedTotals omits methods left empty', () => {
    expect(toCountedTotals({ cash: 0, card: null, wallet: 10 }, ['cash', 'card'])).toEqual({
      cash: 0,
    });
  });
});
