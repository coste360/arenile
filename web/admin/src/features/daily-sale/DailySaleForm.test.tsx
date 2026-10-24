import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from 'vitest';
import { EST_ID, baseRoutes, callsTo, json, problem, renderApp } from '../configuration/testing';
import { API, printData, saleRoutes, ticket } from './testing';

const PATH = `/admin/e/${EST_ID}/vendita-rapida`;

async function lastBody(fetchMock: Parameters<typeof callsTo>[0], method: string, path: string) {
  const calls = callsTo(fetchMock, method, path);
  return (await (calls[calls.length - 1] as Request).clone().json()) as Record<string, unknown>;
}

function setup(opts: Parameters<typeof saleRoutes>[0] = {}) {
  return renderApp(PATH, { ...baseRoutes('cashier'), ...saleRoutes(opts) });
}

describe('DailySaleForm (Vendita rapida)', () => {
  const print = vi.fn();
  beforeAll(() => {
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(new Date('2026-07-14T10:00:00'));
    vi.stubGlobal('print', print);
  });
  afterEach(() => print.mockReset());
  afterAll(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it('starts from the defaults: today, full day, first pack, spot-first', async () => {
    setup();
    expect(await screen.findByRole('heading', { level: 1, name: 'Vendita rapida' })).toBeVisible();
    expect(screen.getByRole('radio', { name: /Giornata intera/ })).toHaveAttribute(
      'aria-checked',
      'true',
    );
    expect(await screen.findByRole('radio', { name: 'Ombrellone + 2 lettini' })).toHaveAttribute(
      'aria-checked',
      'true',
    );
    expect(screen.getByRole('radio', { name: 'VIP' })).toHaveAttribute('aria-checked', 'false');
    expect(screen.getByRole('radio', { name: /Postazione/ })).toHaveAttribute(
      'aria-checked',
      'true',
    );
    // No place yet: nothing to quote, payment disabled, with accessible names.
    expect(screen.getByText('Scegli una postazione libera.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Contanti/ })).toBeDisabled();
    expect(screen.getByRole('button', { name: /Carta/ })).toBeDisabled();
  });

  it('quotes the picked spot and shows the price preview with VAT', async () => {
    const user = userEvent.setup();
    const { fetchMock } = setup();
    await user.click(await screen.findByRole('combobox', { name: 'Postazione' }));
    await user.click(await screen.findByRole('option', { name: 'A2 · Prima fila' }));
    await waitFor(() => expect(screen.getByTestId('sale-total')).toHaveTextContent('30,00'));
    expect(screen.getByText(/di cui IVA 22%/)).toBeInTheDocument();
    const body = await lastBody(fetchMock, 'POST', `${API}/daily-sales/quote`);
    expect(body).toEqual({ date: '2026-07-14', slots: 'FULL', pack_id: 'p1', spot_id: 's2' });
    // Booked spots are not offered.
    expect(screen.queryByRole('option', { name: /A3/ })).toBeNull();
  });

  it('switches to zone-first and quotes the zone with its free places', async () => {
    const user = userEvent.setup();
    const { fetchMock } = setup();
    await user.click(await screen.findByRole('radio', { name: /Solo zona/ }));
    const zones = await screen.findByRole('radiogroup', { name: 'Zona' });
    await user.click(within(zones).getByRole('radio', { name: /Prima fila/ }));
    expect(await screen.findByText('3 posti liberi nella zona')).toBeInTheDocument();
    const body = await lastBody(fetchMock, 'POST', `${API}/daily-sales/quote`);
    expect(body).toMatchObject({ zone_id: 'z1', slots: 'FULL', pack_id: 'p1' });
    expect(body).not.toHaveProperty('spot_id');
  });

  it('changes slot and extras with the keyboard shortcuts and steppers', async () => {
    const user = userEvent.setup();
    const { fetchMock } = setup();
    await user.click(await screen.findByRole('combobox', { name: 'Postazione' }));
    await user.click(await screen.findByRole('option', { name: 'A1 · Prima fila' }));
    await user.click(screen.getByRole('button', { name: 'Aumenta Lettino aggiuntivo' }));
    await user.keyboard('2');
    await waitFor(async () =>
      expect(await lastBody(fetchMock, 'POST', `${API}/daily-sales/quote`)).toEqual({
        date: '2026-07-14',
        slots: 'AM',
        pack_id: 'p1',
        spot_id: 's1',
        extras: [{ extra_id: 'x1', quantity: 1 }],
      }),
    );
  });

  it('sells by card in one tap, shows the number and prints the ticket', async () => {
    const user = userEvent.setup();
    const { fetchMock } = setup({
      extra: {
        [`POST ${API}/daily-sales`]: () =>
          json(201, {
            ticket: ticket({ payment_method: 'card' }),
            booking_id: 'b37',
            payment_id: 'pay1',
          }),
        [`GET ${API}/tickets/t37/print`]: () => json(200, printData()),
      },
    });
    await user.click(await screen.findByRole('combobox', { name: 'Postazione' }));
    await user.click(await screen.findByRole('option', { name: 'A1 · Prima fila' }));
    await user.type(screen.getByLabelText('Cognome'), 'Rossi');
    const card = screen.getByRole('button', { name: /Carta/ });
    await waitFor(() => expect(card).toBeEnabled());
    await user.click(card);
    expect(await screen.findByTestId('ticket-number')).toHaveTextContent('#037');
    const req = callsTo(fetchMock, 'POST', `${API}/daily-sales`)[0] as Request;
    expect(req.headers.get('Idempotency-Key')).toBeTruthy();
    expect(await req.clone().json()).toEqual({
      date: '2026-07-14',
      slots: 'FULL',
      pack_id: 'p1',
      spot_id: 's1',
      payment: { method: 'card' },
      holder: { last_name: 'Rossi' },
      expected_total: 3000,
    });
    // auto_print: the receipt is rendered in the print-only area and printed.
    await waitFor(() => expect(print).toHaveBeenCalledTimes(1));
    const area = document.querySelector('[data-ticket-print-area]') as HTMLElement;
    expect(within(area).getByRole('article', { name: 'Biglietto #037' })).toBeInTheDocument();
    // "Nuova vendita" starts again (focused for Enter).
    expect(screen.getByRole('button', { name: 'Nuova vendita' })).toHaveFocus();
  });

  it('sends the open cash session and shows the change for cash', async () => {
    const user = userEvent.setup();
    const { fetchMock } = setup({
      extra: {
        [`POST ${API}/daily-sales`]: () =>
          json(201, { ticket: ticket(), booking_id: 'b37', payment_id: 'p', change_amount: 2000 }),
        [`GET ${API}/tickets/t37/print`]: () => json(200, printData()),
      },
    });
    await user.click(await screen.findByRole('combobox', { name: 'Postazione' }));
    await user.click(await screen.findByRole('option', { name: 'A1 · Prima fila' }));
    await user.type(screen.getByLabelText('Contanti ricevuti'), '50');
    expect(await screen.findByText('20,00 €', { exact: false })).toBeInTheDocument();
    const cash = screen.getByRole('button', { name: /Contanti/ });
    await waitFor(() => expect(cash).toBeEnabled());
    await user.click(cash);
    await screen.findByTestId('ticket-number');
    const body = await lastBody(fetchMock, 'POST', `${API}/daily-sales`);
    expect(body.payment).toEqual({ method: 'cash', cash_session_id: 'cs1', tendered_amount: 5000 });
  });

  it('blocks cash without an open session but still sells by card', async () => {
    const user = userEvent.setup();
    setup({ session: false });
    expect(await screen.findByText('Cassa chiusa')).toBeInTheDocument();
    await user.click(await screen.findByRole('combobox', { name: 'Postazione' }));
    await user.click(await screen.findByRole('option', { name: 'A1 · Prima fila' }));
    await waitFor(() => expect(screen.getByRole('button', { name: /Carta/ })).toBeEnabled());
    expect(screen.getByRole('button', { name: /Contanti/ })).toBeDisabled();
  });

  it('explains a zone that filled up in the meantime', async () => {
    const user = userEvent.setup();
    setup({
      extra: {
        [`POST ${API}/daily-sales`]: () => problem(409, 'zone_full'),
      },
    });
    await user.click(await screen.findByRole('radio', { name: /Solo zona/ }));
    await user.click(await screen.findByRole('radio', { name: /Prima fila/ }));
    const card = screen.getByRole('button', { name: /Carta/ });
    await waitFor(() => expect(card).toBeEnabled());
    await user.click(card);
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Nessun posto libero nella zona per la fascia scelta.',
    );
  });

  it('says to pick the afternoon after the same-day cutoff instead of a generic rule error', async () => {
    const user = userEvent.setup();
    setup({
      extra: {
        [`POST ${API}/daily-sales/quote`]: () =>
          problem(422, 'policy_violation', {
            violations: [{ code: 'same_day_closed', params: { reason: 'time' } }],
          }),
      },
    });
    await user.click(await screen.findByRole('combobox', { name: 'Postazione' }));
    await user.click(await screen.findByRole('option', { name: 'A1 · Prima fila' }));
    expect(await screen.findByText(/scegli Pomeriggio/)).toBeInTheDocument();
    expect(screen.queryByText(/non rispetta le regole/)).not.toBeInTheDocument();
  });

  it('charges with C or K only from inside the form and with no other dialog open', async () => {
    const user = userEvent.setup();
    const { fetchMock } = setup();
    await user.click(await screen.findByRole('combobox', { name: 'Postazione' }));
    await user.click(await screen.findByRole('option', { name: 'A1 · Prima fila' }));
    const card = screen.getByRole('button', { name: /Carta/ });
    await waitFor(() => expect(card).toBeEnabled());
    // Focus on the page body (outside the form): a stray key sells nothing.
    (document.activeElement as HTMLElement | null)?.blur();
    await user.keyboard('k');
    expect(callsTo(fetchMock, 'POST', `${API}/daily-sales`)).toHaveLength(0);
    // Inside the form, but another dialog is open on top: still nothing.
    card.focus();
    const layer = document.createElement('div');
    layer.setAttribute('role', 'dialog');
    layer.setAttribute('data-state', 'open');
    document.body.appendChild(layer);
    await user.keyboard('k');
    expect(callsTo(fetchMock, 'POST', `${API}/daily-sales`)).toHaveLength(0);
    layer.remove();
    // Inside the form and nothing on top: K sells by card.
    card.focus();
    await user.keyboard('k');
    await waitFor(() => expect(callsTo(fetchMock, 'POST', `${API}/daily-sales`)).toHaveLength(1));
  });
});
