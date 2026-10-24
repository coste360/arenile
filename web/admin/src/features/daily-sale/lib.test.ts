import { describe, expect, it } from 'vitest';
import {
  packsForSale,
  StableKey,
  changeDue,
  isTypingTarget,
  saleDefaults,
  shortcutFor,
  ticketMapLabel,
  ticketNumberLabel,
  toCreateRequest,
  toQuoteRequest,
  vatRateLabel,
} from './lib';

describe('ticketNumberLabel', () => {
  it('pads to three digits', () => {
    expect(ticketNumberLabel(1)).toBe('#001');
    expect(ticketNumberLabel(37)).toBe('#037');
    expect(ticketNumberLabel(1204)).toBe('#1204');
  });
});

describe('ticketMapLabel', () => {
  it('prefers the holder name, else the number', () => {
    expect(ticketMapLabel({ ticket_number: 37 })).toBe('#037');
    expect(ticketMapLabel({ ticket_number: 37, holder_name: ' Rossi ' })).toBe('Rossi');
  });
});

describe('saleDefaults', () => {
  it.each([
    ['from the map (spot)', { spotId: 's1' }, 'spot'],
    ['from a zone', { zoneId: 'z1' }, 'zone'],
    ['from the cash desk', {}, 'spot'],
  ] as const)('%s', (_n, opts, where) => {
    const v = saleDefaults('2026-07-14', opts);
    expect(v).toMatchObject({ date: '2026-07-14', slots: 'FULL', where, extras: {} });
  });
});

describe('toQuoteRequest', () => {
  const base = { ...saleDefaults('2026-07-14'), packId: 'p1' };

  it('needs a pack and the place of the chosen mode', () => {
    expect(toQuoteRequest({ ...base, packId: '', spotId: 's1' })).toBeNull();
    expect(toQuoteRequest(base)).toBeNull();
    expect(toQuoteRequest({ ...base, where: 'zone', spotId: 's1' })).toBeNull();
  });

  it('sends exactly one of spot_id and zone_id', () => {
    const both = { ...base, spotId: 's1', zoneId: 'z1' };
    expect(toQuoteRequest(both)).toEqual({
      date: '2026-07-14',
      slots: 'FULL',
      pack_id: 'p1',
      spot_id: 's1',
    });
    expect(toQuoteRequest({ ...both, where: 'zone' })).toEqual({
      date: '2026-07-14',
      slots: 'FULL',
      pack_id: 'p1',
      zone_id: 'z1',
    });
  });

  it('keeps only extras taken, in a stable order (same cache key)', () => {
    const v = { ...base, spotId: 's1', extras: { x2: 1, x0: 0, x1: 2 } };
    expect(toQuoteRequest(v)?.extras).toEqual([
      { extra_id: 'x1', quantity: 2 },
      { extra_id: 'x2', quantity: 1 },
    ]);
  });
});

describe('toCreateRequest', () => {
  const v = { ...saleDefaults('2026-07-14', { spotId: 's1' }), packId: 'p1' };

  it('card: no session, no holder when the name is blank', () => {
    expect(
      toCreateRequest({ ...v, lastName: '  ', tendered: 5000 }, 'card', { cashSessionId: 'cs' }),
    ).toEqual({
      date: '2026-07-14',
      slots: 'FULL',
      pack_id: 'p1',
      spot_id: 's1',
      payment: { method: 'card' },
    });
  });

  it('cash: session, tendered, optional name and the expected total', () => {
    expect(
      toCreateRequest({ ...v, lastName: ' Rossi ', firstName: '', tendered: 5000 }, 'cash', {
        cashSessionId: 'cs1',
        expectedTotal: 3000,
      }),
    ).toMatchObject({
      payment: { method: 'cash', cash_session_id: 'cs1', tendered_amount: 5000 },
      holder: { last_name: 'Rossi' },
      expected_total: 3000,
    });
  });
});

describe('changeDue', () => {
  it.each([
    [3000, undefined, undefined],
    [3000, 2000, undefined],
    [3000, 3000, 0],
    [3000, 5000, 2000],
  ])('total %i, tendered %s → %s', (total, tendered, expected) => {
    expect(changeDue(total, tendered)).toBe(expected);
  });
});

describe('vatRateLabel', () => {
  it('formats decimal strings without floats', () => {
    expect(vatRateLabel('22.00')).toBe('22%');
    expect(vatRateLabel('10')).toBe('10%');
    expect(vatRateLabel('5.50')).toBe('5,5%');
    expect(vatRateLabel('5.5', 'en')).toBe('5.5%');
  });
});

describe('shortcutFor', () => {
  const input = document.createElement('input');
  const button = document.createElement('button');
  it.each([
    [{ key: '1' }, 'FULL'],
    [{ key: '3' }, 'PM'],
    [{ key: 'C' }, 'cash'],
    [{ key: 'k' }, 'card'],
    [{ key: 'z' }, 'zone'],
    [{ key: 'c', ctrlKey: true }, null],
    [{ key: 'c', repeat: true }, null],
    [{ key: 'c', target: input }, null],
    [{ key: 'c', target: button }, 'cash'],
    [{ key: 'x' }, null],
  ])('%o → %s', (e, expected) => {
    expect(shortcutFor(e)).toBe(expected);
  });

  it('treats text fields and listboxes as typing targets, not buttons or checkboxes', () => {
    const checkbox = Object.assign(document.createElement('input'), { type: 'checkbox' });
    const combo = document.createElement('button');
    combo.setAttribute('role', 'combobox');
    expect(isTypingTarget(document.createElement('textarea'))).toBe(true);
    expect(isTypingTarget(combo)).toBe(true);
    expect(isTypingTarget(checkbox)).toBe(false);
    expect(isTypingTarget(null)).toBe(false);
  });
});

describe('StableKey', () => {
  it('keeps the key while the body is the same (double tap, retry)', () => {
    const k = new StableKey();
    const a = k.for({ a: 1 });
    expect(k.for({ a: 1 })).toBe(a);
    const b = k.for({ a: 2 });
    expect(b).not.toBe(a);
    k.reset();
    expect(k.for({ a: 2 })).not.toBe(b);
  });
});

describe('packsForSale', () => {
  const quantity = new Set(['beach']);
  const packs = [
    { id: 'base', active: true, channels: ['desk', 'online'] },
    { id: 'online-only', active: true, channels: ['online'] },
    { id: 'off', active: false },
    { id: 'row1-only', active: true, allowed_zone_ids: ['row1'] },
    { id: 'entry', active: true, channels: ['desk'], allowed_zone_ids: ['beach'] },
  ];
  const ids = (zone: string) => packsForSale(packs, zone, quantity).map((p) => p.id);

  it('keeps desk packs allowed in the zone and never the entry packs', () => {
    expect(ids('row1')).toEqual(['base', 'row1-only']);
    expect(ids('row2')).toEqual(['base']);
  });

  it('before a zone is chosen, hides packs limited to quantity zones', () => {
    expect(ids('')).toEqual(['base', 'row1-only']);
  });
});
