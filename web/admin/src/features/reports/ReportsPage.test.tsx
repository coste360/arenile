import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  EST_ID,
  callsTo,
  json,
  list,
  problem,
  renderApp,
  type Handler,
} from '../configuration/testing';
import { API, eur, norm, roleRoutes, type CashRole } from '../cash/testing';

const PATH = `/admin/e/${EST_ID}/report`;
const R = `${API}/reports`;

const period = (p: URLSearchParams) => {
  const from = p.get('from') ?? '';
  const to = p.get('to') ?? '';
  return { from, to, days: 30 };
};

const season = (year: number, extra: Record<string, unknown> = {}) => ({
  id: `s${year}`,
  establishment_id: EST_ID,
  year,
  opens_on: `${year}-05-15`,
  closes_on: `${year}-09-30`,
  status: 'active',
  version: 1,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  ...extra,
});

const summary = (p: URLSearchParams) => ({
  period: period(p),
  occupancy_bp: 6234,
  occupied_units: 1234,
  capacity_units: 1980,
  revenue_collected: 1250000,
  revenue_booked: 1400000,
  bookings: 321,
  average_booking_value: 4361,
  cancellations: 12,
  no_shows: 3,
  season_passes_sold: 40,
  open_balance_total: 98000,
  open_balance_bookings: 7,
  generated_at: '2026-07-15T10:00:00Z',
});

const comparison = (p: URLSearchParams) => ({
  current: period(p),
  previous: { from: '2025-05-15', to: '2025-09-30', days: 139 },
  granularity: p.get('granularity'),
  metrics: [
    {
      metric: 'revenue_collected',
      unit: 'cents',
      current: 1250000,
      previous: 1000000,
      delta: 250000,
      delta_bp: 2500,
    },
    { metric: 'occupancy_bp', unit: 'basis_points', current: 6234, previous: 5784, delta: 450 },
    { metric: 'cancellations', unit: 'count', current: 12, previous: 10, delta: 2, delta_bp: 2000 },
    { metric: 'no_shows', unit: 'count', current: 3, previous: 0, delta: 3 },
  ],
  series: [
    {
      key: '2026-W21',
      label: 'Sett. 21',
      current_from: '2026-05-18',
      previous_from: '2025-05-19',
      current_revenue: 100000,
      previous_revenue: 80000,
      current_occupancy_bp: 3000,
      previous_occupancy_bp: 2500,
      current_bookings: 20,
      previous_bookings: 15,
    },
  ],
});

const revenueRow = (key: string, label: string, gross: number) => ({
  key,
  label,
  count: 3,
  gross,
  taxable: Math.round(gross / 1.22),
  tax: gross - Math.round(gross / 1.22),
  refunds: 0,
});

function routes(role: CashRole, extra: Record<string, Handler> = {}) {
  const q = (req: Request) => new URL(req.url).searchParams;
  return {
    ...roleRoutes(role),
    [`GET ${API}/seasons`]: () => list([season(2025), season(2026)]),
    [`GET ${R}/summary`]: (req: Request) => json(200, summary(q(req))),
    [`GET ${R}/comparison`]: (req: Request) => json(200, comparison(q(req))),
    [`GET ${R}/revenue`]: (req: Request) => {
      const p = q(req);
      return json(200, {
        period: period(p),
        group_by: p.get('group_by'),
        basis: p.get('basis'),
        rows: [revenueRow('desk', 'Cassa', 61000), revenueRow('online', 'Online', 122000)],
        totals: revenueRow('total', 'Totale', 183000),
      });
    },
    [`GET ${R}/top-customers`]: (req: Request) =>
      json(200, {
        period: period(q(req)),
        sort: 'revenue',
        rows: [
          {
            rank: 1,
            customer_id: 'c1',
            display_name: 'Giulia Bianchi',
            bookings: 4,
            spot_days: 30,
            revenue: 150000,
            last_booking_on: '2026-07-01',
          },
        ],
      }),
    [`GET ${R}/open-balances`]: (req: Request) =>
      json(200, {
        period: period(q(req)),
        total_balance: 50000,
        rows: [
          {
            booking_id: 'b1',
            code: 'AR-7K3Q9',
            customer_id: 'c1',
            display_name: 'Giulia Bianchi',
            status: 'confirmed',
            starts_on: '2026-07-20',
            ends_on: '2026-07-27',
            total_amount: 80000,
            paid_amount: 30000,
            balance_due: 50000,
          },
        ],
      }),
    ...extra,
  };
}

const lastQuery = (fetchMock: ReturnType<typeof vi.fn>, path: string) =>
  Object.fromEntries(
    new URL(callsTo(fetchMock, 'GET', path).at(-1)?.url ?? 'http://x').searchParams,
  );

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date'] });
  vi.setSystemTime(new Date('2026-07-15T10:00:00Z'));
});
afterEach(() => vi.useRealTimers());

describe('ReportsPage', () => {
  it('shows the KPIs of the current season by default', async () => {
    const { fetchMock } = renderApp(PATH, routes('viewer'));
    const kpis = await screen.findByRole('region', { name: 'Indicatori del periodo' });
    expect(lastQuery(fetchMock, `${R}/summary`)).toEqual({ from: '2026-05-15', to: '2026-09-30' });
    expect(screen.getByLabelText('Stagione', { selector: 'select' })).toHaveValue('s2026');
    expect(within(kpis).getByText('62,3%')).toBeInTheDocument();
    expect(within(kpis).getByText('1234 fasce occupate su 1980')).toBeInTheDocument();
    expect(norm(within(kpis).getByText(norm(eur(1250000))).textContent)).toBe(norm(eur(1250000)));
    // Without the comparison no comparison request and no deltas.
    expect(callsTo(fetchMock, 'GET', `${R}/comparison`)).toHaveLength(0);
    expect(screen.queryByText('+25,0%')).not.toBeInTheDocument();
  });

  it('compares with the previous year: deltas, chart caption and tables', async () => {
    const user = userEvent.setup();
    const { fetchMock } = renderApp(PATH, routes('manager'));
    await screen.findByRole('region', { name: 'Indicatori del periodo' });
    await user.click(screen.getByRole('switch', { name: 'Confronta con il 2025' }));
    expect((await screen.findAllByText('+25,0%')).length).toBeGreaterThan(0);
    expect(lastQuery(fetchMock, `${R}/comparison`)).toEqual({
      from: '2026-05-15',
      to: '2026-09-30',
      compare_year: '2025',
      granularity: 'week',
      align: 'calendar',
    });
    // Occupancy in percentage points; previous value as text next to the delta.
    expect(screen.getAllByText('+4,5 p.p.').length).toBeGreaterThan(0);
    expect(screen.getAllByText(/^2025:/).map((e) => norm(e.textContent))).toContain(
      norm(`2025: ${eur(1000000)}`),
    );
    // No previous value: "Nuovo" instead of a relative change.
    expect(screen.getAllByText('Nuovo').length).toBeGreaterThan(0);
    const metrics = screen.getByRole('table', {
      name: "Indicatori a confronto con l'anno precedente",
    });
    expect(norm(within(metrics).getAllByRole('row')[1]?.textContent)).toBe(
      norm(`Incassato${eur(1250000)}${eur(1000000)}+25,0%`),
    );
    expect(
      screen.getByRole('table', { name: "Andamento a confronto con l'anno precedente" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole('link', { name: 'Esporta CSV: Confronto con il 2025' }),
    ).toHaveAttribute(
      'href',
      `${R}/comparison?format=csv&from=2026-05-15&to=2026-09-30&compare_year=2025&granularity=week&align=calendar`,
    );
    await user.selectOptions(screen.getByLabelText('Allineamento'), 'weekday');
    await waitFor(() => expect(lastQuery(fetchMock, `${R}/comparison`).align).toBe('weekday'));
  });

  it('switches to a month and queries its exact range', async () => {
    const user = userEvent.setup();
    const { fetchMock } = renderApp(PATH, routes('owner'));
    await screen.findByRole('region', { name: 'Indicatori del periodo' });
    await user.click(screen.getByRole('radio', { name: 'Mese' }));
    await user.selectOptions(screen.getByLabelText('Mese', { selector: 'select' }), '02');
    await user.selectOptions(screen.getByLabelText('Anno'), '2024');
    await waitFor(() =>
      expect(lastQuery(fetchMock, `${R}/summary`)).toEqual({
        from: '2024-02-01',
        to: '2024-02-29',
      }),
    );
    expect(
      screen.getByText('Dati dal 01/02/2024 al 29/02/2024 (estremi inclusi).'),
    ).toBeInTheDocument();
  });

  it('groups revenue, enforces the basis rules and links the CSV of what is shown', async () => {
    const user = userEvent.setup();
    const { fetchMock } = renderApp(PATH, routes('viewer'));
    await screen.findByRole('region', { name: 'Indicatori del periodo' });
    await user.click(screen.getByRole('tab', { name: 'Ricavi' }));
    const table = await screen.findByRole('table', { name: 'Ricavi' });
    expect(lastQuery(fetchMock, `${R}/revenue`)).toEqual({
      from: '2026-05-15',
      to: '2026-09-30',
      group_by: 'month',
      basis: 'collected',
    });
    expect(norm(within(table).getAllByRole('row')[2]?.textContent)).toContain(norm(eur(122000)));

    const panel = screen.getByRole('tabpanel');
    const groupBy = within(panel).getByLabelText('Raggruppa per');
    expect(within(groupBy).getByRole('option', { name: 'Pack' })).toBeDisabled();
    await user.selectOptions(groupBy, 'payment_method');
    await waitFor(() =>
      expect(lastQuery(fetchMock, `${R}/revenue`).group_by).toBe('payment_method'),
    );

    // Booked basis: payment method is not allowed, the grouping falls back to month.
    await user.selectOptions(within(panel).getByLabelText('Base di calcolo'), 'booked');
    await waitFor(() =>
      expect(lastQuery(fetchMock, `${R}/revenue`)).toMatchObject({
        basis: 'booked',
        group_by: 'month',
      }),
    );
    expect(within(groupBy).getByRole('option', { name: 'Metodo di pagamento' })).toBeDisabled();
    const csv = screen.getByRole('link', { name: 'Esporta CSV: Ricavi' });
    expect(csv).toHaveAttribute(
      'href',
      `${R}/revenue?format=csv&from=2026-05-15&to=2026-09-30&group_by=month&basis=booked`,
    );
    expect(csv).toHaveAttribute('download', 'arenile_revenue_2026-05-15_2026-09-30.csv');
  });

  it('lists top customers by name only and open balances with the booking link', async () => {
    const user = userEvent.setup();
    renderApp(PATH, routes('viewer'));
    await screen.findByRole('region', { name: 'Indicatori del periodo' });
    await user.click(screen.getByRole('tab', { name: 'Clienti migliori' }));
    const customers = await screen.findByRole('table', { name: 'Clienti migliori' });
    expect(within(customers).getByRole('link', { name: 'Giulia Bianchi' })).toHaveAttribute(
      'href',
      `/admin/e/${EST_ID}/clienti/c1`,
    );
    await user.click(screen.getByRole('tab', { name: 'Saldi aperti' }));
    const balances = await screen.findByRole('table', { name: 'Saldi aperti' });
    expect(within(balances).getByRole('link', { name: 'AR-7K3Q9' })).toHaveAttribute(
      'href',
      `/admin/e/${EST_ID}/prenotazioni/b1`,
    );
    expect(within(balances).getByText('Confermata')).toBeInTheDocument();
    expect(norm(screen.getByText(/^Da incassare:/).textContent)).toBe(
      norm(`Da incassare: ${eur(50000)} su 1 prenotazioni.`),
    );
  });

  it('shows the server error of a report with a retry', async () => {
    renderApp(
      PATH,
      routes('viewer', { [`GET ${R}/summary`]: () => problem(501, 'not_implemented') }),
    );
    expect(await screen.findByText('Funzione non ancora disponibile.')).toBeInTheDocument();
  });

  it('is not available without reports.read (cashier)', async () => {
    const { fetchMock } = renderApp(PATH, routes('cashier'));
    expect(
      await screen.findByText('Il tuo ruolo non ha accesso ai report e alle esportazioni.'),
    ).toBeInTheDocument();
    expect(callsTo(fetchMock, 'GET', `${R}/summary`)).toHaveLength(0);
  });
});
