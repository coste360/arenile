import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import {
  baseRoutes,
  callsTo,
  EST_ID,
  json,
  list,
  renderApp,
  type Handler,
} from '../configuration/testing';
import { bodyOf, bookingDetail, CUSTOMER, estPath } from './testing';

function summary(i: number, overrides: Record<string, unknown> = {}) {
  return {
    id: `b-${i}`,
    code: `AR-${i}`,
    status: 'confirmed',
    channel: 'desk',
    customer_id: CUSTOMER.id,
    customer_display_name: `Cliente ${i}`,
    starts_on: '2026-07-14',
    ends_on: '2026-07-16',
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
    ...overrides,
  };
}

const ZONE_Q = {
  id: 'zq',
  map_id: null,
  code: 'lettini',
  name: { it: 'Lettini liberi' },
  assignment_mode: 'quantity',
  online_visible: true,
  price_category: 'std',
  auto_assign: false,
  sort_order: 1,
  spot_count: 0,
  version: 1,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
};
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

function inventoryRoutes(): Record<string, Handler> {
  return {
    [`GET ${estPath('/maps')}`]: () => list([]),
    [`GET ${estPath('/zones')}`]: () => list([ZONE_Q]),
    [`GET ${estPath('/packs')}`]: () => list([PACK]),
    [`GET ${estPath('/extras')}`]: () => list([]),
  };
}

describe('BookingsPage', () => {
  it('lists bookings, filters by status and loads the next cursor page', async () => {
    const routes: Record<string, Handler> = {
      ...baseRoutes('cashier'),
      ...inventoryRoutes(),
      [`GET ${estPath('/bookings')}`]: (req) => {
        const url = new URL(req.url);
        if (url.searchParams.get('cursor') === 'next') return json(200, { items: [summary(3)] });
        return json(200, {
          items: [summary(1), summary(2, { status: 'held', balance_due: 0 })],
          next_cursor: 'next',
        });
      },
    };
    const { fetchMock } = renderApp(`/e/${EST_ID}/prenotazioni`, routes);
    expect(
      await screen.findByRole('heading', { name: 'Prenotazioni', level: 1 }),
    ).toBeInTheDocument();
    const table = await screen.findByRole('table', { name: 'Prenotazioni' });
    expect(await within(table).findByText('AR-1')).toBeInTheDocument();
    expect(within(table).getByText('In attesa')).toBeInTheDocument();
    expect(within(table).getAllByText('40,00 €')).toHaveLength(1);

    await userEvent.setup().click(screen.getByRole('button', { name: 'Carica altre' }));
    expect(await within(table).findByText('AR-3')).toBeInTheDocument();

    const user = userEvent.setup();
    await user.click(screen.getByLabelText('Solo con saldo da incassare'));
    await waitFor(() =>
      expect(
        callsTo(fetchMock, 'GET', estPath('/bookings')).some(
          (r) => new URL(r.url).searchParams.get('balance_due') === 'true',
        ),
      ).toBe(true),
    );
  });

  it('creates a quantity booking with an inline customer from the sheet', async () => {
    const routes: Record<string, Handler> = {
      ...baseRoutes('cashier'),
      ...inventoryRoutes(),
      [`GET ${estPath('/bookings')}`]: () => json(200, { items: [] }),
      [`GET ${estPath('/customers')}`]: () => json(200, { items: [] }),
      [`POST ${estPath('/bookings/quote')}`]: () =>
        json(200, {
          breakdown: {
            engine_version: 1,
            currency: 'EUR',
            lines: [{ pack: 'lettino', zone: 'lettini', days: [], extras: [], total: 2400 }],
            discounts: [],
            rounding: 0,
            total: 2400,
            vat: [],
          },
          deposit_due: 0,
          policy: { rules: {}, violations: [] },
          available: true,
          conflicts: [],
        }),
      [`POST ${estPath('/bookings')}`]: () =>
        json(201, bookingDetail({ id: 'new-b', code: 'AR-NEW' })),
      [`POST ${estPath('/bookings/new-b/payments')}`]: () =>
        json(201, {
          id: 'pay',
          provider: 'manual',
          method: 'card',
          direction: 'in',
          amount: 2400,
          status: 'succeeded',
          created_at: '2026-07-14T10:00:00Z',
        }),
    };
    const { fetchMock } = renderApp(`/e/${EST_ID}/prenotazioni`, routes);
    const user = userEvent.setup();
    await user.click(await screen.findByRole('button', { name: 'Nuova prenotazione' }));
    const sheet = await screen.findByRole('dialog', { name: 'Nuova prenotazione' });

    // The only pack is preselected; book 2 units in the quantity zone.
    await user.click(within(sheet).getByRole('tab', { name: 'Zona o quantità' }));
    await user.click(within(sheet).getByRole('combobox', { name: 'Zona' }));
    await user.click(await screen.findByRole('option', { name: 'Lettini liberi' }));
    const qty = within(sheet).getByLabelText('Quantità');
    await user.clear(qty);
    await user.type(qty, '2');

    await user.click(within(sheet).getByRole('radio', { name: 'Nuovo cliente' }));
    await user.type(within(sheet).getByLabelText('Cognome'), 'Verdi');
    await user.type(within(sheet).getByLabelText('Telefono'), '333 000 1111');

    expect(await within(sheet).findByTestId('quote-total')).toHaveTextContent('24,00 €');
    await user.click(within(sheet).getByRole('switch', { name: 'Incassa ora' }));
    await user.click(within(sheet).getByRole('combobox', { name: 'Metodo' }));
    await user.click(await screen.findByRole('option', { name: 'Carta' }));
    await user.click(within(sheet).getByRole('button', { name: 'Crea prenotazione' }));

    await waitFor(() =>
      expect(callsTo(fetchMock, 'POST', estPath('/bookings/new-b/payments'))).toHaveLength(1),
    );
    const [create] = callsTo(fetchMock, 'POST', estPath('/bookings'));
    expect(create!.headers.get('Idempotency-Key')).toBeTruthy();
    expect(await bodyOf(create!)).toMatchObject({
      channel: 'desk',
      status: 'confirmed',
      slots: 'FULL',
      formula: 'day',
      lines: [{ pack_id: 'p1', zone_id: 'zq', quantity: 2 }],
      customer: { last_name: 'Verdi', phone: '+393330001111' },
    });
    const [pay] = callsTo(fetchMock, 'POST', estPath('/bookings/new-b/payments'));
    expect(await bodyOf(pay!)).toEqual({ amount: 2400, method: 'card' });
    expect(await screen.findByText('Prenotazione AR-NEW creata.')).toBeInTheDocument();
  });

  it('shows the conflicting dates when the spot was taken meanwhile', async () => {
    const routes: Record<string, Handler> = {
      ...baseRoutes('cashier'),
      ...inventoryRoutes(),
      [`GET ${estPath('/bookings')}`]: () => json(200, { items: [] }),
      [`GET ${estPath('/customers')}`]: () => json(200, { items: [CUSTOMER] }),
      [`POST ${estPath('/bookings/quote')}`]: () =>
        json(200, {
          breakdown: {
            engine_version: 1,
            currency: 'EUR',
            lines: [],
            discounts: [],
            rounding: 0,
            total: 1000,
            vat: [],
          },
          deposit_due: 0,
          policy: { rules: {}, violations: [] },
          available: true,
          conflicts: [],
        }),
      [`POST ${estPath('/bookings')}`]: () =>
        json(409, {
          type: 'pool_exhausted',
          title: 'x',
          status: 409,
          conflicts: [
            { pool_id: 'pool1', date: '2026-07-14', slot: 'AM', reason: 'pool_exhausted' },
          ],
        }),
    };
    renderApp(`/e/${EST_ID}/prenotazioni`, routes);
    const user = userEvent.setup();
    await user.click(await screen.findByRole('button', { name: 'Nuova prenotazione' }));
    const sheet = await screen.findByRole('dialog', { name: 'Nuova prenotazione' });
    await user.click(within(sheet).getByRole('tab', { name: 'Zona o quantità' }));
    await user.click(within(sheet).getByRole('combobox', { name: 'Zona' }));
    await user.click(await screen.findByRole('option', { name: 'Lettini liberi' }));
    await user.click(within(sheet).getByRole('combobox', { name: 'Cliente' }));
    await user.type(await screen.findByPlaceholderText('Cerca cliente'), 'Ros');
    await user.click(await screen.findByRole('option', { name: /Rossi Mario/ }));
    await user.click(within(sheet).getByRole('button', { name: 'Crea prenotazione' }));
    expect(
      await within(sheet).findByText('Quantità non disponibile per le date scelte.'),
    ).toBeInTheDocument();
    expect(within(sheet).getByText(/^Quantità: 14\/07\/2026 \(Mattina\)$/)).toBeInTheDocument();
  });
});

describe('BookingDetailPage', () => {
  it('shows status, amounts, lines and history; checks in with If-Match', async () => {
    const detail = bookingDetail();
    let current: Record<string, unknown> = detail;
    const routes: Record<string, Handler> = {
      ...baseRoutes('cashier'),
      ...inventoryRoutes(),
      [`GET ${estPath(`/bookings/${detail.id}`)}`]: () => json(200, current),
      [`POST ${estPath(`/bookings/${detail.id}/check-in`)}`]: () => {
        current = { ...detail, status: 'checked_in', next_statuses: ['completed'], version: 5 };
        return json(200, current);
      },
    };
    const { fetchMock } = renderApp(`/e/${EST_ID}/prenotazioni/${detail.id}`, routes);
    expect(await screen.findByRole('heading', { name: /AR-7K3Q9/, level: 1 })).toBeInTheDocument();
    expect(screen.getByText('Confermata')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Rossi Mario' })).toBeInTheDocument();
    expect(screen.getByText('Saldo da incassare').nextElementSibling).toHaveTextContent('40,00 €');
    expect(screen.getByRole('table', { name: 'Righe' })).toHaveTextContent('A12');
    expect(screen.getByRole('list', { name: 'Storico' })).toHaveTextContent('Prenotazione creata');
    // Cashier: no refunds.issue, so no refund button; payments allowed.
    expect(screen.queryByRole('button', { name: 'Rimborso' })).toBeNull();
    expect(screen.getByRole('button', { name: 'Registra incasso' })).toBeInTheDocument();

    const user = userEvent.setup();
    await user.click(screen.getByRole('button', { name: 'Check-in' }));
    const dialog = await screen.findByRole('alertdialog');
    await user.click(within(dialog).getByRole('button', { name: 'Check-in' }));
    expect(await screen.findByText('Presente')).toBeInTheDocument();
    const [req] = callsTo(fetchMock, 'POST', estPath(`/bookings/${detail.id}/check-in`));
    expect(req!.headers.get('If-Match')).toBe('"4"');
    expect(screen.queryByRole('button', { name: 'Check-in' })).toBeNull();
  });

  it('records a payment for the balance due', async () => {
    const detail = bookingDetail();
    const routes: Record<string, Handler> = {
      ...baseRoutes('cashier'),
      ...inventoryRoutes(),
      [`GET ${estPath(`/bookings/${detail.id}`)}`]: () => json(200, detail),
      [`POST ${estPath(`/bookings/${detail.id}/payments`)}`]: () =>
        json(409, { type: 'cash_session_closed', title: 'x', status: 409 }),
    };
    const { fetchMock } = renderApp(`/e/${EST_ID}/prenotazioni/${detail.id}`, routes);
    const user = userEvent.setup();
    await user.click(await screen.findByRole('button', { name: 'Registra incasso' }));
    const dialog = await screen.findByRole('dialog', { name: 'Registra incasso' });
    expect(within(dialog).getByLabelText('Importo')).toHaveValue('40,00');
    await user.click(within(dialog).getByRole('button', { name: 'Registra incasso' }));
    expect(await within(dialog).findByText(/La cassa è chiusa/)).toBeInTheDocument();
    const [req] = callsTo(fetchMock, 'POST', estPath(`/bookings/${detail.id}/payments`));
    expect(await bodyOf(req!)).toEqual({ amount: 4000, method: 'cash' });
  });

  it('hides write actions from a viewer', async () => {
    const detail = bookingDetail();
    const routes: Record<string, Handler> = {
      ...baseRoutes('cashier'),
      'GET /api/admin/v1/auth/me': () =>
        json(200, {
          principal_type: 'user',
          user: {
            id: 'u',
            email: 'v@example.it',
            name: 'Viewer',
            locale: 'it',
            totp_enabled: false,
          },
          memberships: [
            {
              organization_id: '22222222-2222-4222-8222-222222222222',
              role: 'viewer',
              permissions: ['bookings.read'],
            },
          ],
          totp_enrolment_required: false,
        }),
      ...inventoryRoutes(),
      [`GET ${estPath(`/bookings/${detail.id}`)}`]: () => json(200, detail),
    };
    renderApp(`/e/${EST_ID}/prenotazioni/${detail.id}`, routes);
    expect(await screen.findByRole('heading', { name: /AR-7K3Q9/, level: 1 })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Check-in' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Cancella prenotazione' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Registra incasso' })).toBeNull();
    expect(screen.queryByLabelText('Aggiungi una nota')).toBeNull();
  });
});
