import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import {
  EST_ID,
  baseRoutes,
  callsTo,
  json,
  list,
  problem,
  renderApp,
} from '../configuration/testing';

const API = `/api/admin/v1/establishments/${EST_ID}`;
const ts = { created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' };
const SEASON = {
  id: 's1',
  year: 2027,
  opens_on: '2027-05-01',
  closes_on: '2027-09-30',
  status: 'active',
  version: 1,
  ...ts,
};
const PACK = {
  id: 'p1',
  code: 'base',
  name: { it: 'Ombrellone + 2 lettini' },
  composition: { umbrella: 1, sunbed: 2 },
  allowed_formulas: ['day'],
  channels: ['desk', 'online'],
  max_per_booking: 4,
  requires_approval: false,
  sort_order: 0,
  active: true,
  version: 1,
  ...ts,
};
const ZONE = {
  id: 'z1',
  code: 'row1',
  name: { it: 'Prima fila' },
  assignment_mode: 'seat',
  online_visible: true,
  price_category: 'row1',
  auto_assign: false,
  sort_order: 0,
  spot_count: 15,
  version: 1,
  ...ts,
};
const RESULT = {
  breakdown: {
    engine_version: 1,
    currency: 'EUR',
    lines: [
      {
        pack: 'base',
        zone: 'row1',
        days: [
          {
            date: '2027-08-14',
            slots: 'FULL',
            base: 3000,
            applied: [
              { rule: 'rule-quick', stage: 'base', effect: 'set 30,00' },
              { rule: 'rule-weekend', stage: 'modifier', effect: '+5,00' },
            ],
            total: 3500,
          },
        ],
        extras: [],
        total: 3500,
      },
    ],
    discounts: [],
    rounding: 0,
    total: 3500,
    vat: [{ rate: '22', taxable: 2869, tax: 631 }],
  },
  deposit_due: 3500,
  policy: { rules: {}, violations: [] },
};

function routes(simulate: (req: Request) => Response | Promise<Response>) {
  return {
    ...baseRoutes('manager'),
    [`GET ${API}/seasons`]: () => list([SEASON]),
    [`GET ${API}/rate-periods`]: () => list([]),
    [`GET ${API}/packs`]: () => list([PACK]),
    [`GET ${API}/extras`]: () => list([]),
    [`GET ${API}/zones`]: () => list([ZONE]),
    [`GET ${API}/promo-codes`]: () => list([]),
    [`GET ${API}/agreements`]: () => list([]),
    // The engine reports rules by id; the page shows their names.
    [`GET ${API}/price-rules`]: () =>
      list([
        { id: 'rule-quick', name: 'Listino rapido · row1 · peak' },
        { id: 'rule-weekend', name: 'Weekend' },
      ]),
    [`POST ${API}/pricing/simulate`]: simulate,
  };
}

describe('PriceSimulatorPage', () => {
  it('sends the quote request and shows the step-by-step breakdown', async () => {
    const user = userEvent.setup();
    let body: Record<string, unknown> | undefined;
    const { fetchMock } = renderApp(
      `/admin/e/${EST_ID}/pricing/simulator`,
      routes(async (req) => {
        body = (await req.json()) as Record<string, unknown>;
        return json(200, RESULT);
      }),
    );
    // Date pickers (shadcn Calendar, dd/MM/yyyy) start on the season opening day.
    expect(await screen.findByLabelText('Dal')).toHaveTextContent('01/05/2027');
    expect(screen.getByLabelText('Al')).toHaveTextContent('01/05/2027');
    await user.click(screen.getByRole('button', { name: 'Calcola' }));
    await waitFor(() =>
      expect(callsTo(fetchMock, 'POST', `${API}/pricing/simulate`)).toHaveLength(1),
    );
    expect(body).toMatchObject({
      channel: 'desk',
      starts_on: '2027-05-01',
      ends_on: '2027-05-01',
      slots: 'FULL',
      formula: 'day',
      lines: [{ pack_id: 'p1', zone_id: 'z1' }],
    });
    const result = await screen.findByRole('region', { name: 'Risultato della simulazione' });
    expect(within(result).getByTestId('simulator-total')).toHaveTextContent('35,00');
    expect(within(result).getByText(/Weekend/)).toBeInTheDocument();
    expect(within(result).getByText(/Listino rapido · row1 · peak/)).toBeInTheDocument();
    expect(within(result).getByText(/IVA 22% su 28,69/)).toBeInTheDocument();
    expect(
      within(result).getByText('6,31 €', { normalizer: (s) => s.replace(/\u00a0/g, ' ') }),
    ).toBeInTheDocument();
    expect(within(result).getByText(/Da pagare alla prenotazione/)).toHaveTextContent('35,00');
  });

  it('explains a missing price', async () => {
    const user = userEvent.setup();
    renderApp(
      `/admin/e/${EST_ID}/pricing/simulator`,
      routes(() =>
        problem(422, 'no_price', { detail: 'Nessun prezzo per base/row1 il 2027-05-01' }),
      ),
    );
    await user.click(await screen.findByRole('button', { name: 'Calcola' }));
    expect(await screen.findByText(/Nessun prezzo per base\/row1/)).toBeInTheDocument();
  });
});
