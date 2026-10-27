import { describe, expect, it } from 'vitest';
import {
  bookableWindow,
  compatibleFormulas,
  flowReducer,
  formulaChoices,
  initialFlowState,
  isWhatComplete,
  isWhenComplete,
  isWhereComplete,
  toSelection,
  type FlowState,
} from './selection';

const season = { opens_on: '2099-05-01', closes_on: '2099-09-30' };

function ready(): FlowState {
  let s = initialFlowState({ from: '2099-07-10', to: '2099-07-12' });
  s = flowReducer(s, { type: 'formula', formula: 'multi_day' });
  return s;
}

describe('compatibleFormulas', () => {
  it.each([
    ['one full day', { from: '2099-07-10', to: '2099-07-10' }, 'FULL', ['day']],
    ['one morning', { from: '2099-07-10', to: '2099-07-10' }, 'AM', ['half_day']],
    ['three days', { from: '2099-07-10', to: '2099-07-12' }, 'FULL', ['multi_day']],
    ['a week', { from: '2099-07-10', to: '2099-07-16' }, 'FULL', ['week', 'multi_day']],
    ['a fortnight', { from: '2099-07-01', to: '2099-07-14' }, 'FULL', ['fortnight', 'multi_day']],
    ['a month', { from: '2099-07-01', to: '2099-07-31' }, 'FULL', ['month', 'multi_day']],
    ['the season', { from: '2099-05-01', to: '2099-09-30' }, 'FULL', ['season', 'multi_day']],
    ['incomplete range', { from: '2099-07-10' }, 'FULL', []],
  ] as const)('%s', (_name, dates, slots, expected) => {
    expect(compatibleFormulas(dates, slots, [], season)).toEqual(expected);
  });

  it('is restricted to the online formulas', () => {
    expect(
      compatibleFormulas({ from: '2099-07-10', to: '2099-07-16' }, 'FULL', ['multi_day'], season),
    ).toEqual(['multi_day']);
    expect(
      compatibleFormulas({ from: '2099-07-10', to: '2099-07-10' }, 'FULL', ['week'], season),
    ).toEqual([]);
  });

  it('offers the whole season as a choice even for other dates', () => {
    expect(formulaChoices({ from: '2099-07-10', to: '2099-07-12' }, 'FULL', [], season)).toEqual([
      'season',
      'multi_day',
    ]);
    expect(
      formulaChoices({ from: '2099-07-10', to: '2099-07-12' }, 'FULL', ['multi_day'], season),
    ).toEqual(['multi_day']);
  });
});

describe('flowReducer', () => {
  it('toggles spots and zones and sets quantities', () => {
    let s = ready();
    s = flowReducer(s, { type: 'toggleSpot', zoneId: 'z1', spotId: 's1', spotLabel: 'A1' });
    s = flowReducer(s, { type: 'toggleZone', zoneId: 'z2' });
    s = flowReducer(s, { type: 'quantity', zoneId: 'z3', quantity: 2 });
    expect(s.lines.map((l) => [l.key, l.mode, l.quantity])).toEqual([
      ['s1', 'seat', 1],
      ['z2', 'zone', 1],
      ['z3', 'quantity', 2],
    ]);
    s = flowReducer(s, { type: 'quantity', zoneId: 'z3', quantity: 3 });
    expect(s.lines.find((l) => l.key === 'z3')?.quantity).toBe(3);
    s = flowReducer(s, { type: 'quantity', zoneId: 'z3', quantity: 0 });
    s = flowReducer(s, { type: 'toggleSpot', zoneId: 'z1', spotId: 's1', spotLabel: 'A1' });
    expect(s.lines.map((l) => l.key)).toEqual(['z2']);
  });

  it('clears the resources when dates, slot or formula change, keeping the promo code', () => {
    let s = ready();
    s = flowReducer(s, { type: 'promo', code: ' estate ' });
    s = flowReducer(s, { type: 'toggleZone', zoneId: 'z2' });
    s = flowReducer(s, { type: 'bookingExtra', extraId: 'e1', quantity: 1 });
    expect(s.promoCode).toBe('ESTATE');
    const changed = flowReducer(s, {
      type: 'dates',
      dates: { from: '2099-07-11', to: '2099-07-12' },
      formula: 'multi_day',
    });
    expect(changed.lines).toEqual([]);
    expect(changed.bookingExtras).toEqual({});
    expect(changed.promoCode).toBe('ESTATE');
    expect(flowReducer(s, { type: 'slots', slots: 'AM', formula: null }).lines).toEqual([]);
  });

  it('resets line extras when the pack changes and drops quantities of zero', () => {
    let s = flowReducer(ready(), { type: 'toggleZone', zoneId: 'z2' });
    s = flowReducer(s, { type: 'pack', key: 'z2', packId: 'p1' });
    s = flowReducer(s, { type: 'lineExtra', key: 'z2', extraId: 'e1', quantity: 2 });
    expect(s.lines[0]?.extras).toEqual({ e1: 2 });
    s = flowReducer(s, { type: 'lineExtra', key: 'z2', extraId: 'e1', quantity: 0 });
    expect(s.lines[0]?.extras).toEqual({});
    s = flowReducer(s, { type: 'lineExtra', key: 'z2', extraId: 'e1', quantity: 1 });
    s = flowReducer(s, { type: 'pack', key: 'z2', packId: 'p2' });
    expect(s.lines[0]?.extras).toEqual({});
  });

  it('flags dropped lines until the dates change', () => {
    let s = flowReducer(ready(), { type: 'toggleZone', zoneId: 'z2' });
    s = flowReducer(s, { type: 'dropLines', keys: ['z2'] });
    expect(s.dropped).toBe(true);
    expect(s.lines).toEqual([]);
    expect(flowReducer(s, { type: 'dates', dates: undefined, formula: null }).dropped).toBe(false);
  });
});

describe('toSelection', () => {
  it('sends the agreement code of the link and keeps it across date changes (spec 30.2.2)', () => {
    let s = initialFlowState({ from: '2099-07-10', to: '2099-07-12' }, 'MIRAMARE');
    s = flowReducer(s, { type: 'formula', formula: 'multi_day' });
    s = flowReducer(s, { type: 'toggleZone', zoneId: 'z3' });
    s = flowReducer(s, { type: 'pack', key: 'z3', packId: 'p2' });
    expect(toSelection(s)).toMatchObject({ agreement_code: 'MIRAMARE' });
    s = flowReducer(s, {
      type: 'dates',
      dates: { from: '2099-07-11', to: '2099-07-12' },
      formula: null,
    });
    expect(s.agreementCode).toBe('MIRAMARE');
    s = flowReducer(s, { type: 'agreement', code: ' residenti ' });
    expect(s.agreementCode).toBe('RESIDENTI');
    s = flowReducer(s, { type: 'agreement', code: '' });
    expect(s.agreementCode).toBe('');
  });

  it('is null until every line has a pack', () => {
    let s = ready();
    expect(isWhenComplete(s)).toBe(true);
    expect(isWhereComplete(s)).toBe(false);
    s = flowReducer(s, { type: 'toggleSpot', zoneId: 'z1', spotId: 's1', spotLabel: 'A1' });
    s = flowReducer(s, { type: 'quantity', zoneId: 'z3', quantity: 2 });
    expect(isWhereComplete(s)).toBe(true);
    expect(toSelection(s)).toBeNull();
    s = flowReducer(s, { type: 'pack', key: 's1', packId: 'p1' });
    expect(isWhatComplete(s)).toBe(false);
    s = flowReducer(s, { type: 'pack', key: 'z3', packId: 'p2' });
    s = flowReducer(s, { type: 'lineExtra', key: 's1', extraId: 'e1', quantity: 1 });
    s = flowReducer(s, { type: 'bookingExtra', extraId: 'e2', quantity: 1 });
    s = flowReducer(s, { type: 'promo', code: 'estate' });
    expect(toSelection(s)).toEqual({
      starts_on: '2099-07-10',
      ends_on: '2099-07-12',
      slots: 'FULL',
      formula: 'multi_day',
      lines: [
        { pack_id: 'p1', spot_id: 's1', extras: [{ extra_id: 'e1', quantity: 1 }] },
        { pack_id: 'p2', zone_id: 'z3', quantity: 2 },
      ],
      extras: [{ extra_id: 'e2', quantity: 1 }],
      promo_code: 'ESTATE',
    });
  });

  it('sends zone lines by zone id (automatic assignment, spec 7.5)', () => {
    let s = flowReducer(ready(), { type: 'toggleZone', zoneId: 'z2' });
    s = flowReducer(s, { type: 'pack', key: 'z2', packId: 'p1' });
    expect(toSelection(s)?.lines).toEqual([{ pack_id: 'p1', zone_id: 'z2' }]);
  });
});

describe('bookableWindow', () => {
  it('intersects today, the online window and the season', () => {
    expect(bookableWindow('2099-06-01', { min_date: '2099-06-15' }, season)).toEqual({
      min: '2099-06-15',
      max: '2099-09-30',
    });
    expect(bookableWindow('2099-07-01', { max_date: '2099-08-31' }, season)).toEqual({
      min: '2099-07-01',
      max: '2099-08-31',
    });
    expect(bookableWindow('2099-07-01', null, null)).toEqual({ min: '2099-07-01' });
  });
});
