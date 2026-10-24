import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { bodyOf } from '../bookings/testing';
import { EST_ID, baseRoutes, callsTo, json, problem, renderApp } from '../configuration/testing';

const API = `/api/admin/v1/establishments/${EST_ID}`;

const TICKET = {
  id: 'tk1',
  kind: 'entry',
  status: 'valid',
  state: 'entered',
  ticket_date: '2026-07-14',
  ticket_number: 37,
  number_label: '#037',
  booking_id: 'b1',
  booking_code: 'AR-7KQ2M9',
  zone_id: 'z1',
  zone_name: { it: 'Spiaggia libera' },
  needs_assignment: false,
  slots: 'FULL',
  formula: 'daily',
  extras: [],
  entry_items: [
    { entry_ticket_type_id: 't1', name: { it: 'Adulto' }, quantity: 2, total: 2000 },
    { entry_ticket_type_id: 't2', name: { it: 'Bambino' }, quantity: 1, total: 500 },
  ],
  total_amount: 2500,
  vat: [],
  payment_method: 'cash',
  print_count: 1,
  version: 2,
  created_at: '2026-07-14T08:00:00Z',
  entered_at: '2026-07-14T09:15:00Z',
};

describe('EntryGatePage', () => {
  it('without BarcodeDetector shows the manual fallback and admits a typed code', async () => {
    const user = userEvent.setup();
    const { fetchMock } = renderApp(`/admin/e/${EST_ID}/ticket/varco`, {
      ...baseRoutes('cashier'),
      [`POST ${API}/entry-gate/check-in`]: () => json(200, { outcome: 'admitted', ticket: TICKET }),
    });
    expect(await screen.findByRole('heading', { level: 1, name: 'Varco' })).toBeInTheDocument();
    expect(await screen.findByText(/non legge i QR con la fotocamera/)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Avvia fotocamera' })).not.toBeInTheDocument();

    await user.type(screen.getByLabelText('Codice del biglietto'), ' AR-7KQ2M9 ');
    await user.click(screen.getByRole('button', { name: 'Conferma ingresso' }));

    expect(await screen.findByText('Ingresso valido')).toBeInTheDocument();
    expect(screen.getByText('#037')).toBeInTheDocument();
    expect(screen.getByText('3 persone')).toBeInTheDocument();
    const [req] = callsTo(fetchMock, 'POST', `${API}/entry-gate/check-in`);
    expect(await bodyOf(req as Request)).toEqual({ code: 'AR-7KQ2M9', manual: true });
    await waitFor(() => expect(screen.getByLabelText('Codice del biglietto')).toHaveValue(''));
  });

  it('reports a second pass and an unknown code', async () => {
    const user = userEvent.setup();
    let calls = 0;
    renderApp(`/admin/e/${EST_ID}/ticket/varco`, {
      ...baseRoutes('cashier'),
      [`POST ${API}/entry-gate/check-in`]: () =>
        ++calls === 1
          ? json(200, { outcome: 'already_entered', ticket: TICKET })
          : problem(404, 'not_found'),
    });
    const input = await screen.findByLabelText('Codice del biglietto');
    await user.type(input, 'AR-7KQ2M9{Enter}');
    expect(await screen.findByText('Già entrato')).toBeInTheDocument();
    expect(screen.getByText(/^Entrato alle/)).toBeInTheDocument();

    await user.type(input, 'NOPE{Enter}');
    expect(await screen.findByText('Biglietto non trovato')).toBeInTheDocument();
    expect(screen.getByText('NOPE')).toBeInTheDocument();
  });
});
