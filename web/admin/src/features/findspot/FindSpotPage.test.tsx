import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';
import {
  baseRoutes,
  callsTo,
  EST_ID,
  json,
  list,
  problem,
  renderApp,
  type Handler,
} from '../configuration/testing';
import { bodyOf, bookingDetail, CUSTOMER, estPath } from '../bookings/testing';

/** "Trova posto" (spec 30.6): search, ranked proposals, booking a proposal. */

const PACK = {
  id: 'p1',
  code: 'lettino',
  name: { it: 'Lettino' },
  composition: {},
  allowed_formulas: ['day', 'multi_day'],
  channels: ['desk'],
  sort_order: 1,
  active: true,
  version: 1,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
};

const PROPOSAL = {
  id: 'p-a5a7',
  rank: 1,
  spot_changes: 1,
  spot_ids: ['s5', 's7'],
  total: 15000,
  agreement_applied: true,
  lines: [
    {
      segments: [
        {
          spot_id: 's5',
          spot_label: 'A5',
          zone_id: 'z1',
          starts_on: '2026-08-10',
          ends_on: '2026-08-14',
        },
        {
          spot_id: 's7',
          spot_label: 'A7',
          zone_id: 'z1',
          starts_on: '2026-08-15',
          ends_on: '2026-08-16',
        },
      ],
    },
  ],
};

function routes(extra: Record<string, Handler> = {}): Record<string, Handler> {
  return {
    ...baseRoutes('cashier'),
    [`GET ${estPath('/maps')}`]: () => list([]),
    [`GET ${estPath('/zones')}`]: () => list([]),
    [`GET ${estPath('/packs')}`]: () => list([PACK]),
    [`GET ${estPath('/agreements')}`]: () => list([]),
    [`GET ${estPath('/customers')}`]: () => list([CUSTOMER]),
    [`POST ${estPath('/find-spot')}`]: () => json(200, { proposals: [PROPOSAL] }),
    ...extra,
  };
}

async function search(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByRole('combobox', { name: 'Pack o allestimento' }));
  await user.click(await screen.findByRole('option', { name: 'Lettino' }));
  await user.click(screen.getByRole('button', { name: 'Cerca' }));
  return screen.findByTestId('findspot-proposal');
}

describe('FindSpotPage', () => {
  beforeAll(() => {
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(new Date('2026-08-01T10:00:00'));
  });
  afterAll(() => {
    vi.useRealTimers();
  });

  it('books a proposal with its segments in one request', async () => {
    const user = userEvent.setup();
    const books: Request[] = [];
    const { fetchMock } = renderApp(
      `/e/${EST_ID}/trova-posto?date=2026-08-10&slot=FULL`,
      routes({
        [`POST ${estPath('/find-spot/book')}`]: (req) => {
          books.push(req);
          return json(201, bookingDetail({ id: 'b-new', code: 'AR-NEW' }));
        },
        [`GET ${estPath('/bookings/b-new')}`]: () =>
          json(200, bookingDetail({ id: 'b-new', code: 'AR-NEW' })),
      }),
    );
    const proposal = await search(user);
    expect(proposal).toHaveTextContent('Proposta 1');
    expect(proposal).toHaveTextContent('1 cambio di posto');
    expect(proposal).toHaveTextContent('Convenzione applicata');
    expect(proposal).toHaveTextContent('150,00 €');
    expect(proposal).toHaveTextContent(
      'A5 dal 10/08/2026 al 14/08/2026, A7 dal 15/08/2026 al 16/08/2026',
    );
    const sent = (await bodyOf(callsTo(fetchMock, 'POST', estPath('/find-spot'))[0]!)) as Record<
      string,
      unknown
    >;
    expect(sent).toMatchObject({ starts_on: '2026-08-10', ends_on: '2026-08-10', pack_id: 'p1' });

    await user.click(within(proposal).getByRole('button', { name: 'Prenota' }));
    const dialog = await screen.findByRole('dialog', { name: 'Prenota la proposta 1' });
    await user.click(within(dialog).getByRole('combobox', { name: 'Cliente' }));
    await user.type(await screen.findByPlaceholderText('Cerca cliente'), 'Ros');
    await user.click(await screen.findByRole('option', { name: /Rossi Mario/ }));
    await user.click(within(dialog).getByRole('button', { name: 'Prenota' }));
    await waitFor(() => expect(books).toHaveLength(1));
    expect(books[0]!.headers.get('Idempotency-Key')).toBeTruthy();
    expect(await bodyOf(books[0]!)).toEqual({
      booking: {
        customer_id: CUSTOMER.id,
        starts_on: '2026-08-10',
        ends_on: '2026-08-10',
        slots: 'FULL',
        formula: 'day',
        lines: [
          {
            pack_id: 'p1',
            segments: [
              { spot_id: 's5', starts_on: '2026-08-10', ends_on: '2026-08-14' },
              { spot_id: 's7', starts_on: '2026-08-15', ends_on: '2026-08-16' },
            ],
          },
        ],
        status: 'confirmed',
      },
      proposal_id: 'p-a5a7',
      expected_total: 15000,
    });
    expect(await screen.findByRole('heading', { name: /AR-NEW/, level: 1 })).toBeInTheDocument();
  });

  it('searches again when a day was taken meanwhile (409)', async () => {
    const user = userEvent.setup();
    const { fetchMock } = renderApp(
      `/e/${EST_ID}/trova-posto`,
      routes({
        [`POST ${estPath('/find-spot/book')}`]: () => problem(409, 'spot_unavailable'),
      }),
    );
    const proposal = await search(user);
    await user.click(within(proposal).getByRole('button', { name: 'Prenota' }));
    const dialog = await screen.findByRole('dialog', { name: 'Prenota la proposta 1' });
    await user.click(within(dialog).getByRole('combobox', { name: 'Cliente' }));
    await user.type(await screen.findByPlaceholderText('Cerca cliente'), 'Ros');
    await user.click(await screen.findByRole('option', { name: /Rossi Mario/ }));
    await user.click(within(dialog).getByRole('button', { name: 'Prenota' }));
    expect(
      await screen.findByText('Nel frattempo una data è stata presa: ecco le nuove proposte.'),
    ).toBeInTheDocument();
    await waitFor(() =>
      expect(callsTo(fetchMock, 'POST', estPath('/find-spot')).length).toBeGreaterThan(1),
    );
  });
});
