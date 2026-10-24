import { describe, expect, it } from 'vitest';
import type { EntryTicketType } from './api';
import {
  EMPTY_ENTRY_SALE,
  entrySaleSchema,
  toEntrySaleCreate,
  toEntrySaleQuote,
} from './entrySale.schema';
import {
  entryTypeSchema,
  toEntryTypeInput,
  toEntryTypeUpdate,
  toEntryTypeValues,
} from './entryType.schema';

const TYPE: EntryTicketType = {
  id: 't-adult',
  code: 'adulto',
  name: { it: 'Adulto', en: 'Adult' },
  description: {},
  pack_id: 'p1',
  zone_id: 'z1',
  sort_order: 1,
  active: true,
  version: 4,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
};
const CHILD: EntryTicketType = { ...TYPE, id: 't-child', code: 'bambino', sort_order: 2 };

function issues(schema: { safeParse: (v: unknown) => unknown }, v: unknown): string[] {
  const r = schema.safeParse(v) as {
    success: boolean;
    error?: { issues: { path: PropertyKey[]; message: string }[] };
  };
  return r.success ? [] : (r.error?.issues ?? []).map((i) => `${i.path.join('.')}:${i.message}`);
}

describe('entry ticket type form', () => {
  const schema = entryTypeSchema('it');

  it('requires code, default-locale name, pack and zone', () => {
    const empty = toEntryTypeValues(undefined, ['it', 'en']);
    expect(issues(schema, empty)).toEqual([
      'code:entryTickets:validation.code',
      'name:entryTickets:validation.name',
      'pack_id:entryTickets:validation.pack',
      'zone_id:entryTickets:validation.zone',
    ]);
    expect(issues(schema, { ...toEntryTypeValues(TYPE, ['it']), code: 'Not Valid' })).toEqual([
      'code:entryTickets:validation.code',
    ]);
    expect(issues(schema, { ...toEntryTypeValues(TYPE, ['it']), sort_order: '1.5' })).toEqual([
      'sort_order:entryTickets:validation.sortOrder',
    ]);
  });

  it('maps the form to the create and PATCH bodies', () => {
    const v = schema.parse({
      ...toEntryTypeValues(undefined, ['it', 'en']),
      code: ' Ridotto ',
      name: { it: ' Ridotto ', en: '' },
      pack_id: 'p2',
      zone_id: 'z2',
    });
    expect(toEntryTypeInput(v)).toEqual({
      code: 'ridotto',
      name: { it: 'Ridotto' },
      pack_id: 'p2',
      zone_id: 'z2',
      sort_order: 0,
      active: true,
    });
    const update = toEntryTypeUpdate(schema.parse(toEntryTypeValues(TYPE, ['it', 'en'])));
    expect(update).toEqual({
      name: { it: 'Adulto', en: 'Adult' },
      description: {},
      pack_id: 'p1',
      zone_id: 'z1',
      sort_order: 1,
      active: true,
    });
    expect(update).not.toHaveProperty('code');
  });
});

describe('entry sale form', () => {
  it('needs at least one ticket; the name is optional', () => {
    expect(issues(entrySaleSchema, EMPTY_ENTRY_SALE)).toEqual([
      'quantities:entryTickets:validation.noItems',
    ]);
    expect(issues(entrySaleSchema, { ...EMPTY_ENTRY_SALE, quantities: { [TYPE.id]: 2 } })).toEqual(
      [],
    );
  });

  it('builds the quote only when something is selected', () => {
    expect(toEntrySaleQuote(EMPTY_ENTRY_SALE, [TYPE], '2026-07-14')).toBeNull();
    expect(
      toEntrySaleQuote(
        { slots: 'AM', quantities: { [CHILD.id]: 1, [TYPE.id]: 2 } },
        [TYPE, CHILD],
        '2026-07-14',
      ),
    ).toEqual({
      date: '2026-07-14',
      slots: 'AM',
      items: [
        { entry_ticket_type_id: 't-adult', quantity: 2 },
        { entry_ticket_type_id: 't-child', quantity: 1 },
      ],
    });
  });

  it('sends cash with the tendered amount, the optional holder and the quoted total', () => {
    const body = toEntrySaleCreate(
      {
        ...EMPTY_ENTRY_SALE,
        quantities: { [TYPE.id]: 2 },
        tendered: 5000,
        last_name: ' Rossi ',
      },
      [TYPE],
      '2026-07-14',
      2400,
    );
    expect(body).toEqual({
      date: '2026-07-14',
      slots: 'FULL',
      items: [{ entry_ticket_type_id: 't-adult', quantity: 2 }],
      payment: { method: 'cash', tendered_amount: 5000 },
      holder: { last_name: 'Rossi' },
      expected_total: 2400,
    });
  });

  it('sends card without tendered amount and without holder when no name is given', () => {
    const body = toEntrySaleCreate(
      { ...EMPTY_ENTRY_SALE, method: 'card', tendered: 5000, quantities: { [TYPE.id]: 1 } },
      [TYPE],
      '2026-07-14',
    );
    expect(body.payment).toEqual({ method: 'card' });
    expect(body).not.toHaveProperty('holder');
    expect(body).not.toHaveProperty('expected_total');
  });
});
