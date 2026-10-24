/** Fixtures of the daily sale tests (imported by tests only, never shipped). */
import { EST_ID, json, list, type Handler } from '../configuration/testing';
import { opDay, opSpot } from '../operations/testing/fixtures';
import { mapDocument, mapSummary } from '../operations/testing/routes';
import type { Ticket, TicketPrintData } from './api';

export const API = `/api/admin/v1/establishments/${EST_ID}`;
const ts = { version: 1, created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' };

export const ZONE_FIRST_ROW = {
  id: 'z1',
  map_id: 'm1',
  code: 'prima',
  name: { it: 'Prima fila' },
  assignment_mode: 'zone',
  online_visible: true,
  price_category: 'row1',
  auto_assign: true,
  sort_order: 1,
  spot_count: 4,
  ...ts,
};

export const PACK_STD = {
  id: 'p1',
  code: 'std',
  name: { it: 'Ombrellone + 2 lettini' },
  composition: {},
  allowed_formulas: ['day', 'half_day'],
  channels: ['desk'],
  sort_order: 1,
  active: true,
  ...ts,
};

export const PACK_VIP = { ...PACK_STD, id: 'p2', code: 'vip', name: { it: 'VIP' }, sort_order: 2 };

export const EXTRA_SUNBED = {
  id: 'x1',
  code: 'lettino',
  name: { it: 'Lettino aggiuntivo' },
  description: {},
  pricing_unit: 'per_day',
  max_quantity: 2,
  attach_to: 'line',
  channels: ['desk'],
  sort_order: 1,
  active: true,
  ...ts,
};

export const SALE_DAY = opDay([
  opSpot('s1', 'A1', 'free'),
  opSpot('s2', 'A2', 'free'),
  opSpot('s3', 'A3', 'booked'),
  opSpot('s4', 'A4', 'free'),
]);

export const QUOTE = {
  breakdown: {
    engine_version: 1,
    currency: 'EUR',
    lines: [],
    discounts: [],
    rounding: 0,
    total: 3000,
    vat: [{ rate: '22.00', taxable: 2459, tax: 541 }],
  },
  total: 3000,
  available: true,
  zone_free: 3,
  conflicts: [],
};

export function ticket(overrides: Partial<Ticket> = {}): Ticket {
  return {
    id: 't37',
    kind: 'sale',
    status: 'valid',
    state: 'assigned',
    ticket_date: '2026-07-14',
    ticket_number: 37,
    number_label: '#037',
    booking_id: 'b37',
    booking_code: 'AR-7K3Q9',
    zone_id: 'z1',
    zone_name: { it: 'Prima fila' },
    spot_id: 's1',
    spot_label: 'A1',
    needs_assignment: false,
    slots: 'FULL',
    formula: 'day',
    pack_id: 'p1',
    pack_name: { it: 'Ombrellone + 2 lettini' },
    extras: [{ extra_id: 'x1', name: { it: 'Lettino aggiuntivo' }, quantity: 1, total: 500 }],
    entry_items: [],
    total_amount: 3000,
    vat: [{ rate: '22.00', taxable: 2459, tax: 541 }],
    payment_method: 'cash',
    print_count: 0,
    version: 1,
    created_at: '2026-07-14T08:05:00Z',
    ...overrides,
  };
}

export function printData(overrides: Partial<TicketPrintData> = {}): TicketPrintData {
  return {
    format: 'thermal_80',
    ticket: ticket(),
    establishment: {
      name: 'Lido Demo',
      legal_name: 'Lido Demo SRL',
      vat_number: 'IT01234567890',
      address: 'Lungomare 1, Gaeta',
    },
    qr_payload: 'AR-7K3Q9',
    reprint: false,
    footer_text: 'Grazie e buona giornata!',
    issued_at: '2026-07-14T08:05:00Z',
    ...overrides,
  };
}

export const SETTINGS = {
  print_format: 'thermal_80',
  auto_print: true,
  zone_first_enabled: true,
  footer_text: { it: 'Grazie' },
  version: 1,
};

export const OPEN_SESSION = {
  session: {
    id: 'cs1',
    register: 'main',
    status: 'open',
    opened_at: '2026-07-14T07:00:00Z',
    opened_by: 'u-self',
    opening_float: 10000,
    expected_totals: {},
    notes: '',
    version: 1,
  },
  movements: [],
  payments: [],
};

/** Routes of the sale screen: inventory, catalogue, day, settings, cash session and quote. */
export function saleRoutes(
  opts: { session?: boolean; extra?: Record<string, Handler> } = {},
): Record<string, Handler> {
  return {
    [`GET ${API}/maps`]: () => list([mapSummary]),
    [`GET ${API}/maps/m1`]: () => json(200, mapDocument, { ETag: '"1"' }),
    [`GET ${API}/zones`]: () => list([ZONE_FIRST_ROW]),
    [`GET ${API}/packs`]: () => list([PACK_STD, PACK_VIP]),
    [`GET ${API}/extras`]: () => list([EXTRA_SUNBED]),
    [`GET ${API}/operations/day`]: () => json(200, SALE_DAY),
    [`GET ${API}/daily-sale-settings`]: () => json(200, SETTINGS),
    [`GET ${API}/cash-sessions/current`]: () =>
      opts.session === false
        ? json(404, { type: 'not_found', title: 'not_found', status: 404 })
        : json(200, OPEN_SESSION),
    [`POST ${API}/daily-sales/quote`]: () => json(200, QUOTE),
    ...opts.extra,
  };
}
