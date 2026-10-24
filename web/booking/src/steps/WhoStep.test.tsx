import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import {
  BASE,
  BOOKING_ID,
  TOKEN,
  checkoutStateFixture,
  callsTo,
  establishmentFixture,
  json,
  problem,
  quoteFixture,
  renderWithEnv,
  type Handler,
} from '../test-utils';
import WhoStep from './WhoStep';

const session = { bookingId: BOOKING_ID, token: TOKEN };
const PATH = `${BASE}/bookings/${BOOKING_ID}`;

function topWindow() {
  const assign = vi.fn();
  const win = { location: { assign } } as unknown as Window & { top: Window; parent: Window };
  win.top = win;
  win.parent = win;
  return { win, assign };
}

function renderWho(routes: Record<string, Handler>, quote = quoteFixture(), win?: Window) {
  const onCompleted = vi.fn();
  const onExpired = vi.fn();
  const utils = renderWithEnv(
    <WhoStep
      session={session}
      state={checkoutStateFixture() as never}
      holdExpiresAt={new Date(Date.now() + 10 * 60_000).toISOString()}
      quote={quote as never}
      onCompleted={onCompleted}
      onExpired={onExpired}
    />,
    routes,
    win ? { win } : {},
  );
  return { ...utils, onCompleted, onExpired };
}

async function fill(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByLabelText('Nome'), 'Maria');
  await user.type(screen.getByLabelText('Cognome'), 'Rossi');
  await user.type(screen.getByLabelText('Email'), 'maria@example.it');
  await user.click(screen.getByRole('checkbox', { name: /accetto le condizioni/ }));
}

describe('WhoStep', () => {
  it('validates the fields and requires the terms; marketing is optional and separate', async () => {
    const user = userEvent.setup();
    const { fetchMock } = renderWho({});
    expect(screen.getByTestId('hold-countdown')).toHaveTextContent(/Posto riservato per \d+:\d\d/);
    await user.click(screen.getByRole('button', { name: /Paga 75,00/ }));
    expect(await screen.findAllByText('Campo obbligatorio')).toHaveLength(3);
    expect(screen.getByText('Per prenotare devi accettare le condizioni')).toBeInTheDocument();
    expect(screen.getByRole('checkbox', { name: /offerte e novità/ })).not.toBeChecked();
    expect(screen.getByText('Newsletter mensile.')).toBeInTheDocument();
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('shows the versioned terms text on demand', async () => {
    const user = userEvent.setup();
    renderWho({});
    await user.click(screen.getByRole('button', { name: /Leggi le condizioni/ }));
    expect(screen.getByText(/Condizioni di prova/)).toBeInTheDocument();
  });

  it('saves the customer with the accepted terms version, then confirms without payment', async () => {
    const user = userEvent.setup();
    const { fetchMock, onCompleted } = renderWho(
      {
        [`PATCH ${PATH}`]: () => json(200, checkoutStateFixture({ next_action: 'pay' })),
        [`POST ${PATH}/pay`]: () => json(200, { mode: 'none', status: 'confirmed' }),
      },
      quoteFixture({ payment_plan: 'at_desk', due_now: 0 }),
    );
    expect(
      screen.getByText('Pagherai direttamente allo stabilimento.', { exact: false }),
    ).toBeInTheDocument();
    await fill(user);
    await user.click(screen.getByRole('button', { name: 'Conferma la prenotazione' }));
    await vi.waitFor(() => expect(onCompleted).toHaveBeenCalled());
    const [patch] = callsTo(fetchMock, 'PATCH', PATH);
    expect(patch!.headers.get('X-Arenile-Checkout')).toBe(TOKEN);
    expect(await patch!.json()).toEqual({
      first_name: 'Maria',
      last_name: 'Rossi',
      email: 'maria@example.it',
      locale: 'it',
      accept_terms_version: '2099-1',
      marketing_consent: false,
    });
    const [pay] = callsTo(fetchMock, 'POST', `${PATH}/pay`);
    expect(pay!.headers.get('Idempotency-Key')).toMatch(/.{8,}/);
    expect(pay!.headers.get('X-Arenile-Checkout')).toBe(TOKEN);
  });

  it('redirects to the payment page at top level (never confirms from the browser)', async () => {
    const user = userEvent.setup();
    const { win, assign } = topWindow();
    const { onCompleted } = renderWho(
      {
        [`PATCH ${PATH}`]: () => json(200, checkoutStateFixture({ next_action: 'pay' })),
        [`POST ${PATH}/pay`]: () =>
          json(200, {
            mode: 'redirect',
            status: 'pending_payment',
            redirect_url: 'https://checkout.stripe.com/c/pay/cs_test_1',
          }),
      },
      quoteFixture(),
      win,
    );
    expect(screen.getByText(/Modalità di prova/)).toBeInTheDocument();
    await fill(user);
    await user.click(screen.getByRole('button', { name: /Paga 75,00/ }));
    await vi.waitFor(() =>
      expect(assign).toHaveBeenCalledWith('https://checkout.stripe.com/c/pay/cs_test_1'),
    );
    expect(onCompleted).not.toHaveBeenCalled();
    expect(
      await screen.findByText('Ti stiamo portando alla pagina di pagamento…'),
    ).toBeInTheDocument();
  });

  it('blocks the checkout when test keys would take a real payment (INV-88)', async () => {
    const online = establishmentFixture().online!;
    const { fetchMock } = renderWithEnv(
      <WhoStep
        session={session}
        state={checkoutStateFixture() as never}
        holdExpiresAt={new Date(Date.now() + 10 * 60_000).toISOString()}
        quote={quoteFixture() as never}
        onCompleted={vi.fn()}
        onExpired={vi.fn()}
      />,
      {},
      {
        online: {
          ...online,
          payment: { ...online.payment, online: false, live_payments_blocked: true },
        },
      },
    );
    expect(screen.getByRole('alert')).toHaveTextContent(
      'Il pagamento online non è disponibile in questo momento',
    );
    const submit = screen.getByRole('button', { name: 'Conferma la prenotazione' });
    expect(submit).toBeDisabled();
    // Never the "pay at the desk" or "no real charge" hints to a real customer.
    expect(screen.queryByText(/Pagherai direttamente/)).not.toBeInTheDocument();
    expect(screen.queryByText(/Modalità di prova/)).not.toBeInTheDocument();
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('a free or desk plan is not blocked by test keys', () => {
    const online = establishmentFixture().online!;
    renderWithEnv(
      <WhoStep
        session={session}
        state={checkoutStateFixture() as never}
        holdExpiresAt={new Date(Date.now() + 10 * 60_000).toISOString()}
        quote={quoteFixture({ payment_plan: 'at_desk', due_now: 0 }) as never}
        onCompleted={vi.fn()}
        onExpired={vi.fn()}
      />,
      {},
      {
        online: {
          ...online,
          payment: { ...online.payment, online: false, live_payments_blocked: true },
        },
      },
    );
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Conferma la prenotazione' })).toBeEnabled();
  });

  it('goes back when the hold expired on the server', async () => {
    const user = userEvent.setup();
    const { onExpired } = renderWho({ [`PATCH ${PATH}`]: () => problem(410, 'hold_expired') });
    await fill(user);
    await user.click(screen.getByRole('button', { name: /Paga/ }));
    await vi.waitFor(() => expect(onExpired).toHaveBeenCalled());
  });

  it('keeps the hold and explains when the payment provider is down', async () => {
    const user = userEvent.setup();
    renderWho({
      [`PATCH ${PATH}`]: () => json(200, checkoutStateFixture({ next_action: 'pay' })),
      [`POST ${PATH}/pay`]: () => problem(503, 'payment_provider_unavailable'),
    });
    await fill(user);
    await user.click(screen.getByRole('button', { name: /Paga/ }));
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Il servizio di pagamento non risponde',
    );
  });
});
