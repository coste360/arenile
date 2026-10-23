import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { EST_ID, baseRoutes, callsTo, json, renderApp } from '../configuration/testing';
import { opBooking, opDay, opSpot } from './testing/fixtures';
import { API, operationsRoutes } from './testing/routes';

const PATH = `/admin/e/${EST_ID}/oggi?date=2026-07-14`;

const day = opDay(
  [
    opSpot('s1', 'A1', 'free'),
    opSpot('s2', 'A2', 'booked', { booking: opBooking({ balance_due: 1250 }) }),
    opSpot('s3', 'A3', 'pass_holder', {
      booking: opBooking({ booking_id: 'p1', is_season_pass: true, code: 'AB-1' }),
    }),
    opSpot('s4', 'A4', 'released', { release_id: 'r1' }),
  ],
  {
    pending_assignments: [
      {
        booking_id: 'b9',
        line_id: 'l9',
        code: 'AR-0009',
        zone_id: 'z2',
        customer_display_name: 'Neri P.',
        starts_on: '2026-07-14',
        ends_on: '2026-07-16',
        slots: 'FULL',
      },
    ],
    totals: {
      spots: 4,
      free: 1,
      occupied: 3,
      arrivals: 2,
      departures: 1,
      checked_in: 0,
      balance_due_amount: 1250,
      pending_assignments: 1,
    },
  },
);

const payment = (amount: number, direction: 'in' | 'refund') => ({
  id: `${amount}`,
  provider: 'manual',
  method: 'cash',
  direction,
  amount,
  status: 'succeeded',
  created_at: '2026-07-14T09:00:00Z',
});

describe('TodayPage', () => {
  it('shows the KPIs of the day, takings and the worklists', async () => {
    const user = userEvent.setup();
    const { fetchMock } = renderApp(PATH, {
      ...baseRoutes('cashier'),
      ...operationsRoutes(day, {
        [`GET ${API}/payments`]: () =>
          json(200, { items: [payment(4500, 'in'), payment(1000, 'refund')] }),
      }),
    });
    const kpis = await screen.findByRole('region', { name: 'Indicatori del giorno' });
    expect(within(kpis).getByText('75%')).toBeInTheDocument();
    expect(within(kpis).getByText('3 postazioni occupate su 4')).toBeInTheDocument();
    expect(within(kpis).getByText('1 partenza')).toBeInTheDocument();
    expect(await within(kpis).findByText('35,00 €')).toBeInTheDocument();
    expect(within(kpis).getByText('Saldi aperti: 12,50 €')).toBeInTheDocument();
    // Takings are read for the selected day only.
    const req = callsTo(fetchMock, 'GET', `${API}/payments`)[0] as Request;
    expect(new URL(req.url).searchParams.get('from')).toBe('2026-07-14');
    expect(new URL(req.url).searchParams.get('to')).toBe('2026-07-14');

    // Worklists: check-in, balances, lines to assign.
    expect(
      screen.getByRole('button', { name: 'Registra il check-in di Rossi M.' }),
    ).toBeInTheDocument();
    await user.click(screen.getByRole('tab', { name: /Saldi/ }));
    expect(await screen.findByText('12,50 €')).toBeInTheDocument();
    await user.click(screen.getByRole('tab', { name: /Da assegnare/ }));
    expect(await screen.findByText('Neri P.')).toBeInTheDocument();

    // The map of the day is on the page too, sharing the same day request.
    expect(
      await screen.findByRole('button', { name: /Postazione A4: Abbonato assente oggi/ }),
    ).toBeInTheDocument();
    await waitFor(() => expect(callsTo(fetchMock, 'GET', `${API}/operations/day`)).toHaveLength(1));
  });

  it('checks in from the worklist', async () => {
    const user = userEvent.setup();
    const { fetchMock } = renderApp(PATH, {
      ...baseRoutes('cashier'),
      ...operationsRoutes(day, {
        [`POST ${API}/bookings/b1/check-in`]: () => json(200, { id: 'b1' }),
      }),
    });
    await user.click(
      await screen.findByRole('button', { name: 'Registra il check-in di Rossi M.' }),
    );
    await waitFor(() =>
      expect(callsTo(fetchMock, 'POST', `${API}/bookings/b1/check-in`)).toHaveLength(1),
    );
  });
});
