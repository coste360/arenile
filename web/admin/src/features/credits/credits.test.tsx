import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import type { BookingDetail } from '../bookings/api';
import { RecordPaymentDialog } from '../bookings/detail/PaymentDialogs';
import { bodyOf, bookingDetail, CUSTOMER, estPath, renderWithApi } from '../bookings/testing';
import {
  baseRoutes,
  callsTo,
  EST_ID,
  json,
  renderApp,
  type Handler,
} from '../configuration/testing';

const LEDGER = {
  customer_id: CUSTOMER.id,
  balance: 2500,
  available: 2500,
  expiring: [{ expires_on: '2027-03-31', amount: 1000 }],
  items: [
    {
      id: 'cr2',
      amount: -500,
      source: 'redemption',
      booking_id: 'b-1',
      created_at: '2026-08-02T10:00:00Z',
    },
    {
      id: 'cr1',
      amount: 3000,
      source: 'goodwill',
      note: 'Ombrellone rotto',
      expires_on: '2027-03-31',
      created_at: '2026-08-01T10:00:00Z',
    },
  ],
};

const OPEN_BOOKING = {
  id: 'b-1',
  code: 'AR-1',
  status: 'confirmed',
  channel: 'desk',
  customer_id: CUSTOMER.id,
  customer_display_name: CUSTOMER.display_name,
  starts_on: '2026-08-14',
  ends_on: '2026-08-16',
  slots: 'FULL',
  formula: 'multi_day',
  is_season_pass: false,
  total_amount: 9000,
  paid_amount: 5000,
  balance_due: 4000,
  deposit_due: 0,
  spot_labels: ['A12'],
  version: 1,
  created_at: '2026-07-01T00:00:00Z',
};

function routes(extra: Record<string, Handler> = {}): Record<string, Handler> {
  return {
    ...baseRoutes('manager'),
    [`GET ${estPath(`/customers/${CUSTOMER.id}`)}`]: () =>
      json(200, {
        customer: CUSTOMER,
        stats: { bookings_count: 1, total_spent: 5000, balance_due: 4000 },
        credit_balance: 2500,
        credits: [],
        recent_bookings: [OPEN_BOOKING],
        season_passes: [],
      }),
    [`GET ${estPath(`/customers/${CUSTOMER.id}/credits`)}`]: () => json(200, LEDGER),
    ...extra,
  };
}

const PAGE = `/e/${EST_ID}/clienti/${CUSTOMER.id}`;

async function ledgerTable() {
  const table = await screen.findByRole('table', { name: 'Movimenti crediti' });
  await within(table).findByText('Cortesia');
  return table;
}

describe('CustomerCreditsSection', () => {
  it('shows the available credit, the next expiry and the movements', async () => {
    renderApp(PAGE, routes());
    const table = await ledgerTable();
    expect(screen.getByText('Credito disponibile').closest('[data-slot="card"]')).toHaveTextContent(
      '25,00 €',
    );
    expect(screen.getByText('entro il 31/03/2027')).toBeInTheDocument();
    const rows = within(table).getAllByRole('row').slice(1);
    expect(rows[0]).toHaveTextContent(/Utilizzo.*Apri.*-5,00\s€/);
    expect(rows[1]).toHaveTextContent(/Cortesia.*Ombrellone rotto.*31\/03\/2027.*\+30,00\s€/);
  });

  it('grants a goodwill credit', async () => {
    const { fetchMock } = renderApp(
      PAGE,
      routes({
        [`POST ${estPath(`/customers/${CUSTOMER.id}/credits`)}`]: () =>
          json(201, { id: 'cr3', amount: 1500, source: 'goodwill', created_at: '2026-08-03' }),
      }),
    );
    const user = userEvent.setup();
    await ledgerTable();
    await user.click(screen.getByRole('button', { name: 'Riconosci credito' }));
    const dialog = await screen.findByRole('dialog', { name: 'Riconosci un credito' });
    await user.type(within(dialog).getByLabelText('Importo'), '15');
    await user.type(within(dialog).getByLabelText(/Nota/), 'Scuse per il disservizio');
    await user.click(within(dialog).getByRole('button', { name: 'Riconosci' }));
    await waitFor(() =>
      expect(callsTo(fetchMock, 'POST', estPath(`/customers/${CUSTOMER.id}/credits`))).toHaveLength(
        1,
      ),
    );
    const [req] = callsTo(fetchMock, 'POST', estPath(`/customers/${CUSTOMER.id}/credits`));
    expect(req!.headers.get('Idempotency-Key')).toBeTruthy();
    expect(await bodyOf(req!)).toEqual({
      source: 'goodwill',
      amount: 1500,
      note: 'Scuse per il disservizio',
    });
  });

  it('refuses an adjustment above the available credit before calling the server', async () => {
    const { fetchMock } = renderApp(PAGE, routes());
    const user = userEvent.setup();
    await ledgerTable();
    await user.click(screen.getByRole('button', { name: 'Riconosci credito' }));
    const dialog = await screen.findByRole('dialog', { name: 'Riconosci un credito' });
    await user.click(within(dialog).getByRole('radio', { name: 'Rettifica' }));
    await user.click(within(dialog).getByRole('radio', { name: 'Togli' }));
    await user.type(within(dialog).getByLabelText('Importo'), '30');
    await user.click(within(dialog).getByRole('button', { name: 'Togli dal saldo' }));
    expect(
      await within(dialog).findByText('Al massimo il credito disponibile (25,00 €).'),
    ).toBeInTheDocument();
    expect(callsTo(fetchMock, 'POST', estPath(`/customers/${CUSTOMER.id}/credits`))).toHaveLength(
      0,
    );
  });

  it('uses the credit on a booking with a balance due (payment method credit)', async () => {
    const { fetchMock } = renderApp(
      PAGE,
      routes({
        [`POST ${estPath('/bookings/b-1/payments')}`]: () =>
          json(201, { id: 'pay1', amount: 2500, method: 'credit' }),
      }),
    );
    const user = userEvent.setup();
    await ledgerTable();
    await user.click(screen.getByRole('button', { name: 'Usa su una prenotazione' }));
    const dialog = await screen.findByRole('dialog', { name: 'Usa il credito' });
    expect(within(dialog).getByLabelText('Importo')).toHaveValue('25,00');
    await user.click(within(dialog).getByRole('button', { name: 'Usa il credito' }));
    await waitFor(() =>
      expect(callsTo(fetchMock, 'POST', estPath('/bookings/b-1/payments'))).toHaveLength(1),
    );
    const [req] = callsTo(fetchMock, 'POST', estPath('/bookings/b-1/payments'));
    expect(await bodyOf(req!)).toEqual({ amount: 2500, method: 'credit', note: 'Credito cliente' });
  });
});

describe('RecordPaymentDialog with the credit method', () => {
  it('shows the available credit and warns when the amount exceeds it', async () => {
    const booking = bookingDetail() as unknown as BookingDetail;
    renderWithApi(
      <RecordPaymentDialog
        establishmentId={EST_ID}
        booking={booking}
        open
        onOpenChange={vi.fn()}
      />,
      { [`GET ${estPath(`/customers/${CUSTOMER.id}/credits`)}`]: () => json(200, LEDGER) },
    );
    const user = userEvent.setup();
    const dialog = await screen.findByRole('dialog', { name: 'Registra incasso' });
    expect(within(dialog).queryByText(/Credito disponibile del cliente/)).toBeNull();
    await user.click(within(dialog).getByRole('combobox', { name: 'Metodo' }));
    await user.click(await screen.findByRole('option', { name: 'Credito cliente' }));
    expect(
      await within(dialog).findByText(
        "L'importo supera il credito disponibile del cliente (25,00 €).",
      ),
    ).toBeInTheDocument();
    const amount = within(dialog).getByLabelText('Importo');
    await user.clear(amount);
    await user.type(amount, '20');
    expect(
      await within(dialog).findByText('Credito disponibile del cliente: 25,00 €.'),
    ).toBeInTheDocument();
  });
});
