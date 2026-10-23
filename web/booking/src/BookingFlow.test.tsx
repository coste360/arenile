import { act, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { BookingPage } from './BookingPage';
import { checkoutStore } from './checkout/session';
import { parseBookingLocation } from './location';
import {
  BASE,
  BOOKING_ID,
  CARD_ZONE,
  CATALOG,
  PACK_BASE,
  Providers,
  TOKEN,
  availabilityFixture,
  callsTo,
  checkoutStateFixture,
  establishmentFixture,
  fakeApi,
  json,
  problem,
  quoteFixture,
  summaryFixture,
  type Handler,
} from './test-utils';

const NOW = Date.parse('2099-06-01T08:00:00Z');

function calendar(req: Request) {
  const month = new URL(req.url).searchParams.get('month') ?? '2099-06';
  const days = Array.from({ length: 30 }, (_, i) => ({
    date: `${month}-${String(i + 1).padStart(2, '0')}`,
    status: i === 19 ? 'full' : i === 14 ? 'limited' : 'available',
  }));
  return json(200, { month, days });
}

const confirmedState = checkoutStateFixture({
  status: 'confirmed',
  checkout_state: 'completed',
  next_action: 'done',
  summary: summaryFixture(),
  confirmation: { code: 'AR-7Q2K', qr_payload: 'AR-7Q2K', manage_link_sent: true },
});

function routes(overrides: Record<string, Handler> = {}): Record<string, Handler> {
  let paid = false;
  return {
    [`GET ${BASE}`]: () => json(200, establishmentFixture({}, ['zone'])),
    [`GET ${BASE}/calendar`]: calendar,
    [`GET ${BASE}/availability`]: () => json(200, availabilityFixture()),
    [`GET ${BASE}/catalog`]: () => json(200, { ...CATALOG, packs: [CATALOG.packs[0]] }),
    [`POST ${BASE}/quote`]: () => json(200, quoteFixture()),
    [`POST ${BASE}/bookings`]: () =>
      json(201, {
        booking_id: BOOKING_ID,
        checkout_token: TOKEN,
        status: 'held',
        hold_expires_at: new Date(Date.now() + 15 * 60_000).toISOString(),
        quote: quoteFixture({ payment_plan: 'at_desk', due_now: 0 }),
      }),
    [`GET ${BASE}/bookings/${BOOKING_ID}`]: () =>
      json(
        200,
        paid
          ? confirmedState
          : checkoutStateFixture({
              hold_expires_at: new Date(Date.now() + 600_000).toISOString(),
              quote: quoteFixture({ payment_plan: 'at_desk', due_now: 0 }),
            }),
      ),
    [`PATCH ${BASE}/bookings/${BOOKING_ID}`]: () =>
      json(200, checkoutStateFixture({ next_action: 'pay' })),
    [`POST ${BASE}/bookings/${BOOKING_ID}/pay`]: () => {
      paid = true;
      return json(200, { mode: 'none', status: 'confirmed' });
    },
    [`DELETE ${BASE}/bookings/${BOOKING_ID}`]: () => json(204, null),
    ...overrides,
  };
}

function renderFlow(path: string, r: Record<string, Handler> = routes()) {
  const { api, fetchMock } = fakeApi(r);
  const [pathname, search = ''] = path.split('?');
  render(
    <Providers api={api}>
      <BookingPage
        location={parseBookingLocation(pathname!, search ? `?${search}` : '')}
        now={() => NOW}
      />
    </Providers>,
  );
  return { fetchMock };
}

async function pickDates(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByRole('button', { name: /Date del soggiorno/ }));
  // The grid re-renders when the month availability arrives: always query it fresh.
  const day = (name: RegExp) => screen.findByRole('button', { name });
  // Full days are disabled and say so; limited days say "ultimi posti".
  expect(await day(/20 giugno 2099, pieno/)).toBeDisabled();
  expect(await day(/15 giugno 2099, ultimi posti/)).toBeEnabled();
  await user.click(await day(/\b10 giugno 2099/));
  await user.click(await day(/\b12 giugno 2099/));
}

afterEach(() => {
  checkoutStore.clear('lido-demo');
  vi.useRealTimers();
});

describe('public booking flow', () => {
  it('books a zone from dates to confirmation without online payment', async () => {
    const user = userEvent.setup();
    const { fetchMock } = renderFlow('/b/lido-demo');

    await pickDates(user);
    expect(screen.getByRole('button', { name: /Date del soggiorno/ })).toHaveTextContent(
      '10/06/2099 – 12/06/2099',
    );
    expect(screen.getByRole('radio', { name: 'Più giorni' })).toHaveAttribute(
      'aria-checked',
      'true',
    );
    expect(await screen.findByText('Posti disponibili per tutto il periodo.')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Avanti' }));

    // Dove: focus moves to the step title.
    const where = await screen.findByRole('heading', { name: '2. Dove' });
    expect(document.activeElement).toBe(where);
    await user.click(await screen.findByRole('button', { name: /Pineta/ }));
    await user.click(screen.getByRole('button', { name: 'Avanti' }));

    // Cosa: the only pack is chosen, the quote fills the summary.
    expect(await screen.findByRole('heading', { name: '3. Cosa' })).toBeInTheDocument();
    expect(await screen.findByRole('radio', { name: /Base/ })).toHaveAttribute(
      'aria-checked',
      'true',
    );
    const summary = screen
      .getByRole('heading', { name: 'Riepilogo' })
      .closest('[data-slot="card"]')!;
    await vi.waitFor(() => expect(summary).toHaveTextContent('Base · 3 giorni'));
    expect(summary).toHaveTextContent('75,00');
    const [quote] = callsTo(fetchMock, 'POST', `${BASE}/quote`);
    expect(await quote!.json()).toEqual({
      starts_on: '2099-06-10',
      ends_on: '2099-06-12',
      slots: 'FULL',
      formula: 'multi_day',
      lines: [{ pack_id: PACK_BASE, zone_id: CARD_ZONE }],
    });
    await vi.waitFor(() => expect(screen.getByRole('button', { name: /Continua/ })).toBeEnabled());
    await user.click(screen.getByRole('button', { name: /Continua/ }));

    // Chi e pagamento: held booking, token stored for this tab, countdown visible.
    expect(await screen.findByRole('heading', { name: '4. Chi e pagamento' })).toBeInTheDocument();
    const [create] = callsTo(fetchMock, 'POST', `${BASE}/bookings`);
    expect(create!.headers.get('Idempotency-Key')).toBeTruthy();
    expect(checkoutStore.load('lido-demo')?.token).toBe(TOKEN);
    expect(await screen.findByTestId('hold-countdown')).toBeInTheDocument();
    await user.type(screen.getByLabelText('Nome'), 'Maria');
    await user.type(screen.getByLabelText('Cognome'), 'Rossi');
    await user.type(screen.getByLabelText('Email'), 'maria@example.it');
    await user.click(screen.getByRole('checkbox', { name: /accetto le condizioni/ }));
    await user.click(screen.getByRole('button', { name: 'Conferma la prenotazione' }));

    // Outcome from the server state (never from the browser).
    expect(
      await screen.findByRole('heading', { name: 'Prenotazione confermata' }),
    ).toBeInTheDocument();
    expect(screen.getByTestId('booking-code')).toHaveTextContent('AR-7Q2K');
  }, 20_000);

  it('debounces the quote while the selection changes', async () => {
    const user = userEvent.setup();
    const { fetchMock } = renderFlow(
      '/b/lido-demo',
      routes({ [`GET ${BASE}`]: () => json(200, establishmentFixture({}, ['quantity'])) }),
    );
    await pickDates(user);
    await user.click(screen.getByRole('button', { name: 'Avanti' }));
    const plus = await screen.findByRole('button', { name: 'Aumenta Lettini in riva' });
    await user.click(plus);
    await user.click(screen.getByRole('button', { name: 'Avanti' }));
    await screen.findByRole('radio', { name: /Base/ });
    await vi.waitFor(() => expect(callsTo(fetchMock, 'POST', `${BASE}/quote`)).toHaveLength(1));
    // Three quick changes → one more quote.
    const parking = screen.getByRole('button', { name: 'Aumenta Parcheggio' });
    const sunbed = screen.getByRole('button', { name: 'Aumenta Lettino aggiuntivo' });
    await user.click(parking);
    await user.click(sunbed);
    await user.click(sunbed);
    await vi.waitFor(() => expect(callsTo(fetchMock, 'POST', `${BASE}/quote`)).toHaveLength(2));
    await new Promise((r) => setTimeout(r, 500));
    expect(callsTo(fetchMock, 'POST', `${BASE}/quote`)).toHaveLength(2);
    const last = callsTo(fetchMock, 'POST', `${BASE}/quote`)[1]!;
    expect((await last.json()).lines[0].extras).toEqual([
      { extra_id: CATALOG.extras[0]!.id, quantity: 2 },
    ]);
  }, 20_000);

  it('sends the customer back to "Dove" when the spot was taken meanwhile', async () => {
    const user = userEvent.setup();
    renderFlow(
      '/b/lido-demo',
      routes({ [`POST ${BASE}/bookings`]: () => problem(409, 'spot_unavailable') }),
    );
    await pickDates(user);
    await user.click(screen.getByRole('button', { name: 'Avanti' }));
    await user.click(await screen.findByRole('button', { name: /Pineta/ }));
    await user.click(screen.getByRole('button', { name: 'Avanti' }));
    await screen.findByRole('radio', { name: /Base/ });
    await vi.waitFor(() => expect(screen.getByRole('button', { name: /Continua/ })).toBeEnabled());
    await user.click(screen.getByRole('button', { name: /Continua/ }));
    expect(await screen.findByRole('heading', { name: '2. Dove' })).toBeInTheDocument();
    expect(
      screen.getByText('La postazione è appena stata prenotata da qualcun altro.'),
    ).toBeInTheDocument();
  });

  it('resumes after the payment return with the session of this tab and polls the outcome', async () => {
    checkoutStore.save('lido-demo', { bookingId: BOOKING_ID, token: TOKEN });
    const states = [
      checkoutStateFixture({ status: 'pending_payment', next_action: 'wait' }),
      confirmedState,
    ];
    const { fetchMock } = renderFlow(
      `/b/lido-demo?checkout=${BOOKING_ID}`,
      routes({
        [`GET ${BASE}/bookings/${BOOKING_ID}`]: () => json(200, states.shift() ?? confirmedState),
      }),
    );
    expect(
      await screen.findByRole('heading', { name: /aspettando la conferma/ }),
    ).toBeInTheDocument();
    expect(
      await screen.findByRole('heading', { name: 'Prenotazione confermata' }, { timeout: 5000 }),
    ).toBeInTheDocument();
    expect(callsTo(fetchMock, 'POST', `${BASE}/bookings/${BOOKING_ID}/pay`)).toHaveLength(0);
  });

  it('after a return without this tab session says the email will follow', async () => {
    const { fetchMock } = renderFlow(`/b/lido-demo?checkout=${BOOKING_ID}`);
    expect(
      await screen.findByRole('heading', { name: 'Pagamento in elaborazione' }),
    ).toBeInTheDocument();
    expect(callsTo(fetchMock, 'GET', `${BASE}/bookings/${BOOKING_ID}`)).toHaveLength(0);
  });

  it('resumes a held booking after a reload, and releases it when going back', async () => {
    const user = userEvent.setup();
    checkoutStore.save('lido-demo', {
      bookingId: BOOKING_ID,
      token: TOKEN,
      holdExpiresAt: new Date(NOW + 10 * 60_000).toISOString(),
    });
    const { fetchMock } = renderFlow(
      '/b/lido-demo',
      routes({
        [`GET ${BASE}/bookings/${BOOKING_ID}`]: () =>
          json(
            200,
            checkoutStateFixture({ hold_expires_at: new Date(Date.now() + 600_000).toISOString() }),
          ),
      }),
    );
    expect(await screen.findByRole('heading', { name: '4. Chi e pagamento' })).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Indietro' }));
    await vi.waitFor(() =>
      expect(callsTo(fetchMock, 'DELETE', `${BASE}/bookings/${BOOKING_ID}`)).toHaveLength(1),
    );
    expect(checkoutStore.load('lido-demo')).toBeNull();
    expect(await screen.findByRole('heading', { name: '1. Quando' })).toBeInTheDocument();
  });

  it('applies the unsaved theme sent by the same-origin panel in preview mode and never books', async () => {
    const parent = { postMessage: vi.fn() };
    const win = new Proxy(window, {
      get(target, prop) {
        if (prop === 'parent') return parent;
        const value = Reflect.get(target, prop);
        return typeof value === 'function' ? value.bind(target) : value;
      },
    });
    const { api } = fakeApi(routes());
    render(
      <Providers api={api}>
        <BookingPage
          location={parseBookingLocation('/b/lido-demo', '?embed=1&preview=1')}
          win={win}
          now={() => NOW}
        />
      </Providers>,
    );
    expect(await screen.findByText(/Anteprima del tema/)).toBeInTheDocument();
    expect(parent.postMessage).toHaveBeenCalledWith(
      { type: 'arenile:preview-ready' },
      window.location.origin,
    );
    act(() => {
      const event = new MessageEvent('message', {
        data: { type: 'arenile:preview-theme', theme: { primary_color: '#ffe600' } },
        origin: window.location.origin,
      });
      Object.defineProperty(event, 'source', { value: parent });
      window.dispatchEvent(event);
    });
    await vi.waitFor(() =>
      expect(document.documentElement.style.getPropertyValue('--primary')).toBe('#ffe600'),
    );
    expect(document.documentElement.style.getPropertyValue('--primary-foreground')).toBe('#111827');
  });
});
