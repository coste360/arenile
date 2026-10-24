import { describe, expect, it } from 'vitest';
import type { EntryTicketType } from './api';
import {
  changeDue,
  createScanDeduper,
  extractTicketCode,
  sellableTypes,
  toSaleItems,
  totalQuantity,
} from './lib';

function type(id: string, overrides: Partial<EntryTicketType> = {}): EntryTicketType {
  return {
    id,
    code: id,
    name: { it: id },
    description: {},
    pack_id: 'p1',
    zone_id: 'z1',
    sort_order: 0,
    active: true,
    version: 1,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

describe('entry ticket helpers', () => {
  it('sells only active types, by sort order then code', () => {
    const list = [
      type('ridotto', { sort_order: 2 }),
      type('bambino', { sort_order: 2 }),
      type('adulto', { sort_order: 1 }),
      type('vecchio', { active: false }),
    ];
    expect(sellableTypes(list).map((t) => t.code)).toEqual(['adulto', 'bambino', 'ridotto']);
  });

  it('builds sale lines only for quantities above zero, capped at 100', () => {
    const types = [type('a'), type('b'), type('c')];
    expect(toSaleItems(types, { b: 2, a: 0, c: 250 })).toEqual([
      { entry_ticket_type_id: 'b', quantity: 2 },
      { entry_ticket_type_id: 'c', quantity: 100 },
    ]);
    expect(toSaleItems(types, {})).toEqual([]);
    expect(totalQuantity({ a: 2, b: 0, c: 3 })).toBe(5);
  });

  it('computes the change only when enough cash was tendered', () => {
    expect(changeDue(1250, 2000)).toBe(750);
    expect(changeDue(1250, 1250)).toBe(0);
    expect(changeDue(1250, 1000)).toBeNull();
    expect(changeDue(1250, null)).toBeNull();
  });

  it('extracts the booking code from a scan or a typed text', () => {
    expect(extractTicketCode('  AR-7KQ2M9 ')).toBe('AR-7KQ2M9');
    expect(extractTicketCode('')).toBe('');
    expect(extractTicketCode('https://lido.example/p/lido/manage?code=AR-1')).toBe('AR-1');
    expect(extractTicketCode('https://lido.example/t/AR-2')).toBe('AR-2');
    expect(extractTicketCode('x'.repeat(300))).toHaveLength(200);
  });

  it('ignores the same QR read again within the window', () => {
    const accept = createScanDeduper(4000);
    expect(accept('A', 0)).toBe(true);
    expect(accept('A', 1000)).toBe(false);
    expect(accept('B', 1500)).toBe(true);
    expect(accept('A', 2000)).toBe(true);
    expect(accept('A', 7000)).toBe(true);
  });
});
