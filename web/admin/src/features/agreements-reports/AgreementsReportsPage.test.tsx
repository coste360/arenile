import { screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';
import { estPath } from '../bookings/testing';
import {
  baseRoutes,
  callsTo,
  EST_ID,
  json,
  list,
  renderApp,
  type Handler,
} from '../configuration/testing';
import { reportFileName, reportFileUrl } from './api';

const SEASON = {
  id: 'se1',
  year: 2026,
  opens_on: '2026-05-01',
  closes_on: '2026-09-30',
  status: 'active',
  version: 1,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
};

const totals = (o: Record<string, unknown> = {}) => ({
  agreements: 2,
  bookings: 30,
  spot_days: 120,
  people: 75,
  revenue: 360000,
  discount: 54000,
  ...o,
});

const REGISTER = {
  establishment_name: 'Lido Demo',
  establishment_legal_name: 'Lido Demo SRL',
  season_id: 'se1',
  season_year: 2026,
  period: { from: '2026-05-01', to: '2026-09-30' },
  generated_at: '2026-10-01T08:00:00Z',
  entries: [
    {
      agreement_id: 'ag1',
      name: 'Residenti',
      code: 'RES',
      customer_category_name: 'Residenti del Comune',
      target: 'customer_category',
      register_category: 'residents',
      conditions_text: 'Sconto del 20% sui giornalieri',
      valid_from: '2026-05-01',
      valid_to: '2026-09-30',
      bookings: 20,
      spot_days: 80,
      people: 50,
      revenue: 240000,
      discount: 48000,
    },
    {
      agreement_id: 'ag2',
      name: 'Ospiti Miramare',
      partner_name: 'Hotel Miramare',
      target: 'partner',
      register_category: 'accommodation',
      conditions_text: 'Sconto del 5%',
      bookings: 10,
      spot_days: 40,
      people: 25,
      revenue: 120000,
      discount: 6000,
    },
  ],
  by_category: [
    totals({ register_category: 'residents', agreements: 1, discount: 48000 }),
    totals({ register_category: 'accommodation', agreements: 1, discount: 6000 }),
  ],
  totals: totals(),
};

describe('agreement report files', () => {
  it('builds download URLs and file names with the period and the season', () => {
    const period = { from: '2026-05-01', to: '2026-09-30' };
    expect(reportFileUrl('e 1', 'agreements-register', period, 'html', 'se1')).toBe(
      '/api/admin/v1/establishments/e%201/reports/agreements-register?from=2026-05-01&to=2026-09-30&format=html&season_id=se1',
    );
    expect(reportFileName('partners', period, 'csv')).toBe(
      'arenile_partners_2026-05-01_2026-09-30.csv',
    );
  });
});

describe('AgreementsReportsPage', () => {
  beforeAll(() => {
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(new Date('2026-08-10T10:00:00Z'));
  });
  afterAll(() => {
    vi.useRealTimers();
  });

  it('shows the register of the current season with totals and the per-partner report', async () => {
    const routes: Record<string, Handler> = {
      ...baseRoutes('manager'),
      [`GET ${estPath('/seasons')}`]: () => list([SEASON]),
      [`GET ${estPath('/reports/agreements-register')}`]: () => json(200, REGISTER),
      [`GET ${estPath('/reports/partners')}`]: () =>
        json(200, {
          period: { from: '2026-05-01', to: '2026-09-30' },
          rows: [
            {
              partner_id: 'pa1',
              partner_name: 'Hotel Miramare',
              kind: 'struttura_ricettiva',
              bookings: 10,
              people: 25,
              spot_days: 40,
              revenue: 120000,
              discount: 6000,
              charged: 50000,
              paid: 20000,
            },
          ],
          totals: {
            bookings: 10,
            people: 25,
            spot_days: 40,
            revenue: 120000,
            discount: 6000,
            charged: 50000,
            paid: 20000,
          },
        }),
    };
    const { fetchMock } = renderApp(`/e/${EST_ID}/convenzioni/rendiconti`, routes);
    expect(
      await screen.findByRole('heading', { name: 'Rendiconti delle convenzioni', level: 1 }),
    ).toBeInTheDocument();
    await screen.findByText('Ospiti Miramare');
    const register = screen.getByRole('table', { name: 'Registro delle convenzioni' });
    expect(register).toHaveTextContent('Sconto del 20% sui giornalieri');
    expect(register).toHaveTextContent('Residenti del Comune');
    expect(register).toHaveTextContent('540,00 €');
    expect(screen.getByRole('table', { name: 'Totali per categoria' })).toHaveTextContent(
      'Strutture ricettive',
    );
    const [req] = callsTo(fetchMock, 'GET', estPath('/reports/agreements-register'));
    expect(new URL(req!.url).search).toBe('?from=2026-05-01&to=2026-09-30&season_id=se1');

    const user = userEvent.setup();
    await user.click(screen.getByRole('button', { name: 'Esporta' }));
    expect(await screen.findByRole('menuitem', { name: 'Excel' })).toHaveAttribute(
      'href',
      expect.stringContaining(
        '/reports/agreements-register?from=2026-05-01&to=2026-09-30&format=xlsx',
      ),
    );
    expect(screen.getByRole('menuitem', { name: 'Stampa o salva in PDF' })).toHaveAttribute(
      'target',
      '_blank',
    );
    await user.keyboard('{Escape}');

    await user.click(screen.getByRole('tab', { name: 'Rendiconto per partner' }));
    const partners = await screen.findByRole('table', { name: 'Rendiconto per partner' });
    expect(await within(partners).findByRole('link', { name: 'Hotel Miramare' })).toBeVisible();
    expect(partners).toHaveTextContent('500,00 €');
  });

  it('hides the reports from a user without reports.read', async () => {
    renderApp(`/e/${EST_ID}/convenzioni/rendiconti`, {
      ...baseRoutes('cashier'),
      [`GET ${estPath('/seasons')}`]: () => list([SEASON]),
    });
    expect(
      await screen.findByText('Per i rendiconti serve il permesso di consultare i report.'),
    ).toBeInTheDocument();
    expect(screen.queryByRole('table')).toBeNull();
  });
});
