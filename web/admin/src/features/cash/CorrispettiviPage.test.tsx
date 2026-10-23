import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { EST_ID, callsTo, json, renderApp, type Handler } from '../configuration/testing';
import { API, eur, norm, roleRoutes, type CashRole } from './testing';

const PATH = `/admin/e/${EST_ID}/cassa/corrispettivi`;
const CORR = `${API}/fiscal/corrispettivi`;

const report = (from: string, to: string, group_by: 'day' | 'month') => ({
  from,
  to,
  group_by,
  currency: 'EUR',
  rows: [
    {
      period: from.slice(0, group_by === 'day' ? 10 : 7),
      vat_rate_bp: 2200,
      method: 'cash',
      gross: 12200,
      taxable: 10000,
      tax: 2200,
      count: 3,
    },
    {
      period: from.slice(0, group_by === 'day' ? 10 : 7),
      vat_rate_bp: 2200,
      method: 'card',
      gross: 6100,
      taxable: 5000,
      tax: 1100,
      count: 1,
    },
    {
      period: from.slice(0, group_by === 'day' ? 10 : 7),
      vat_rate_bp: 1000,
      method: 'cash',
      gross: 1100,
      taxable: 1000,
      tax: 100,
      count: 2,
    },
    {
      period: from.slice(0, group_by === 'day' ? 10 : 7),
      vat_rate_bp: 1000,
      method: 'cash',
      gross: -550,
      taxable: -500,
      tax: -50,
      count: 1,
    },
  ],
  totals: [
    { vat_rate_bp: 1000, gross: 550, taxable: 500, tax: 50 },
    { vat_rate_bp: 2200, gross: 18300, taxable: 15000, tax: 3300 },
  ],
  grand_total: 18850,
});

function routes(role: CashRole, extra: Record<string, Handler> = {}) {
  return {
    ...roleRoutes(role),
    [`GET ${CORR}`]: (req: Request) => {
      const u = new URL(req.url);
      const p = u.searchParams;
      return json(
        200,
        report(
          p.get('from') ?? '',
          p.get('to') ?? '',
          (p.get('group_by') ?? 'day') as 'day' | 'month',
        ),
      );
    },
    ...extra,
  };
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date'] });
  vi.setSystemTime(new Date('2026-08-15T10:00:00'));
});
afterEach(() => vi.useRealTimers());

describe('CorrispettiviPage', () => {
  it('shows totals by VAT rate and by method for today', async () => {
    const { fetchMock } = renderApp(PATH, routes('viewer'));
    const vat = await screen.findByRole('table', { name: 'Per aliquota IVA' });
    const q = new URL(callsTo(fetchMock, 'GET', CORR)[0]?.url ?? 'http://x').searchParams;
    expect(Object.fromEntries(q)).toEqual({
      from: '2026-08-15',
      to: '2026-08-15',
      group_by: 'day',
      format: 'json',
    });

    const vatRows = within(vat).getAllByRole('row');
    // Header, 22%, 10%, footer.
    expect(norm(vatRows[1]?.textContent)).toBe(norm(`22%${eur(15000)}${eur(3300)}${eur(18300)}`));
    expect(norm(vatRows[2]?.textContent)).toBe(norm(`10%${eur(500)}${eur(50)}${eur(550)}`));
    expect(norm(vatRows[3]?.textContent)).toBe(
      norm(`Totale${eur(15500)}${eur(3350)}${eur(18850)}`),
    );

    const methods = screen.getByRole('table', { name: 'Per metodo di pagamento' });
    const mRows = within(methods).getAllByRole('row');
    expect(norm(mRows[1]?.textContent)).toBe(norm(`Contanti6${eur(12750)}`));
    expect(norm(mRows[2]?.textContent)).toBe(norm(`Carta1${eur(6100)}`));
    expect(norm(screen.getByTestId('grand-total').textContent)).toBe(norm(eur(18850)));
  });

  it('links the CSV and JSON exports of the selected period', async () => {
    renderApp(PATH, routes('manager'));
    await screen.findByRole('table', { name: 'Per aliquota IVA' });
    const csv = screen.getByRole('link', { name: 'Scarica CSV' });
    expect(csv).toHaveAttribute(
      'href',
      `${CORR}?from=2026-08-15&to=2026-08-15&group_by=day&format=csv`,
    );
    expect(csv).toHaveAttribute('download', 'corrispettivi_2026-08-15.csv');
    expect(screen.getByRole('link', { name: 'Scarica JSON' })).toHaveAttribute(
      'href',
      `${CORR}?from=2026-08-15&to=2026-08-15&group_by=day&format=json`,
    );
  });

  it('switches to a month grouped by month', async () => {
    const user = userEvent.setup();
    const { fetchMock } = renderApp(PATH, routes('owner'));
    await screen.findByRole('table', { name: 'Per aliquota IVA' });
    await user.click(screen.getByRole('radio', { name: 'Mese' }));
    await user.selectOptions(screen.getByLabelText('Mese'), '02');
    await user.selectOptions(screen.getByLabelText('Anno'), '2024');
    await waitFor(() => {
      const last = new URL(callsTo(fetchMock, 'GET', CORR).at(-1)?.url ?? 'http://x').searchParams;
      expect(Object.fromEntries(last)).toEqual({
        from: '2024-02-01',
        to: '2024-02-29',
        group_by: 'month',
        format: 'json',
      });
    });
    expect(screen.getByRole('link', { name: 'Scarica CSV' })).toHaveAttribute(
      'href',
      `${CORR}?from=2024-02-01&to=2024-02-29&group_by=month&format=csv`,
    );
  });

  it('shows an empty state without takings', async () => {
    renderApp(
      PATH,
      routes('viewer', {
        [`GET ${CORR}`]: () =>
          json(200, {
            from: '2026-08-15',
            to: '2026-08-15',
            group_by: 'day',
            currency: 'EUR',
            rows: [],
            totals: [],
            grand_total: 0,
          }),
      }),
    );
    expect(await screen.findByText('Nessun incasso nel periodo selezionato.')).toBeInTheDocument();
  });

  it('is not available without reports.read (cashier)', async () => {
    const { fetchMock } = renderApp(PATH, routes('cashier'));
    expect(
      await screen.findByText('Il tuo ruolo non ha accesso ai report e alle esportazioni.'),
    ).toBeInTheDocument();
    expect(callsTo(fetchMock, 'GET', CORR)).toHaveLength(0);
  });
});
