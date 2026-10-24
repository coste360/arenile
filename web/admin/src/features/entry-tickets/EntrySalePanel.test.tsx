import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { bodyOf, estPath, renderWithApi } from '../bookings/testing';
import { EST_ID, callsTo, json, problem } from '../configuration/testing';
import { EntrySalePanel } from './EntrySalePanel';

vi.mock('../configuration/api/useConfigContext', () => ({
  useConfigContext: () => ({
    establishmentId: '11111111-1111-4111-8111-111111111111',
    establishment: { timezone: 'Europe/Rome' },
    defaultLocale: 'it',
    locales: ['it'],
    uiLocale: 'it',
    canConfigure: false,
  }),
}));

const ts = { created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' };
const TYPES = [
  {
    id: 't-adult',
    code: 'adulto',
    name: { it: 'Adulto' },
    description: {},
    pack_id: 'p1',
    zone_id: 'z1',
    sort_order: 1,
    active: true,
    version: 1,
    ...ts,
  },
  {
    id: 't-child',
    code: 'bambino',
    name: { it: 'Bambino' },
    description: {},
    pack_id: 'p2',
    zone_id: 'z1',
    sort_order: 2,
    active: true,
    version: 1,
    ...ts,
  },
  {
    id: 't-old',
    code: 'vecchio',
    name: { it: 'Vecchio' },
    description: {},
    pack_id: 'p3',
    zone_id: 'z1',
    sort_order: 0,
    active: false,
    version: 1,
    ...ts,
  },
];

function quote(total: number, available = true, remaining = 40) {
  return json(200, {
    breakdown: {
      engine_version: 1,
      currency: 'EUR',
      lines: [],
      discounts: [],
      rounding: 0,
      total,
      vat: [],
    },
    total,
    available,
    remaining,
    conflicts: [],
  });
}

const TICKET = {
  id: 'tk1',
  kind: 'entry',
  status: 'valid',
  state: 'issued',
  ticket_date: '2026-07-14',
  ticket_number: 12,
  number_label: '#012',
  booking_id: 'b1',
  booking_code: 'AR-1',
  zone_id: 'z1',
  zone_name: { it: 'Libera' },
  needs_assignment: false,
  slots: 'FULL',
  formula: 'daily',
  extras: [],
  entry_items: [],
  total_amount: 2500,
  vat: [],
  payment_method: 'cash',
  print_count: 0,
  version: 1,
  created_at: '2026-07-14T08:00:00Z',
};

describe('EntrySalePanel', () => {
  it('quotes the selected entries and sells them in cash with the change', async () => {
    const user = userEvent.setup();
    const onSold = vi.fn();
    const { fetchMock } = renderWithApi(<EntrySalePanel date="2026-07-14" onSold={onSold} />, {
      [`GET ${estPath('/entry-ticket-types')}`]: () => json(200, { items: TYPES }),
      [`POST ${estPath('/entry-sales/quote')}`]: async (req) => {
        const body = (await bodyOf(req)) as { items: { quantity: number }[] };
        const n = body.items.reduce((s, i) => s + i.quantity, 0);
        return quote(n === 3 ? 2500 : 1000 * n);
      },
      [`POST ${estPath('/entry-sales')}`]: () =>
        json(201, { ticket: TICKET, booking_id: 'b1', payment_id: 'pay1', change_amount: 2500 }),
    });

    expect(await screen.findByText('Adulto')).toBeInTheDocument();
    expect(screen.queryByText('Vecchio')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Incassa' })).toBeDisabled();

    await user.click(screen.getByRole('button', { name: 'Aumenta Adulto' }));
    await user.click(screen.getByRole('button', { name: 'Aumenta Adulto' }));
    await user.click(screen.getByRole('button', { name: 'Aumenta Bambino' }));
    const submit = await screen.findByRole('button', { name: /^Incassa 25,00\s€$/ });
    await waitFor(() => expect(submit).toBeEnabled());
    expect(screen.getByText(/40 posti rimasti/)).toBeInTheDocument();

    await user.type(screen.getByLabelText(/Contanti ricevuti/), '50');
    expect(await screen.findByText(/^Resto:/)).toHaveTextContent(/Resto:\s25,00\s€/);
    await user.type(screen.getByLabelText(/Cognome/), 'Rossi');
    await user.click(submit);

    expect(await screen.findByText('Biglietto #012 emesso')).toBeInTheDocument();
    expect(onSold).toHaveBeenCalledOnce();
    const [req] = callsTo(fetchMock, 'POST', `/api/admin/v1/establishments/${EST_ID}/entry-sales`);
    expect(req?.headers.get('Idempotency-Key')).toBeTruthy();
    expect(await bodyOf(req as Request)).toEqual({
      date: '2026-07-14',
      slots: 'FULL',
      items: [
        { entry_ticket_type_id: 't-adult', quantity: 2 },
        { entry_ticket_type_id: 't-child', quantity: 1 },
      ],
      payment: { method: 'cash', tendered_amount: 5000 },
      holder: { last_name: 'Rossi' },
      expected_total: 2500,
    });
  });

  it('blocks the sale when the daily capacity is sold out', async () => {
    const user = userEvent.setup();
    renderWithApi(<EntrySalePanel date="2026-07-14" />, {
      [`GET ${estPath('/entry-ticket-types')}`]: () => json(200, { items: TYPES }),
      [`POST ${estPath('/entry-sales/quote')}`]: () => quote(1000, false, 0),
    });
    await user.click(await screen.findByRole('button', { name: 'Aumenta Adulto' }));
    expect(await screen.findByText(/Capienza esaurita/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Incassa/ })).toBeDisabled();
  });

  it('shows the closed cash session problem from the server', async () => {
    const user = userEvent.setup();
    renderWithApi(<EntrySalePanel date="2026-07-14" />, {
      [`GET ${estPath('/entry-ticket-types')}`]: () => json(200, { items: TYPES }),
      [`POST ${estPath('/entry-sales/quote')}`]: () => quote(1000),
      [`POST ${estPath('/entry-sales')}`]: () => problem(409, 'cash_session_closed'),
    });
    await user.click(await screen.findByRole('button', { name: 'Aumenta Adulto' }));
    const submit = await screen.findByRole('button', { name: /^Incassa 10,00\s€$/ });
    await waitFor(() => expect(submit).toBeEnabled());
    await user.click(submit);
    expect(await screen.findByRole('alert')).toBeInTheDocument();
  });

  it('explains how to configure the types when none is active', async () => {
    renderWithApi(<EntrySalePanel date="2026-07-14" />, {
      [`GET ${estPath('/entry-ticket-types')}`]: () => json(200, { items: [TYPES[2]] }),
    });
    expect(await screen.findByText('Nessun tipo di ingresso attivo')).toBeInTheDocument();
  });
});
