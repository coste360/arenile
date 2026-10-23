import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import {
  BASE,
  BOOKING_ID,
  TOKEN,
  callsTo,
  checkoutStateFixture,
  json,
  problem,
  renderWithEnv,
  summaryFixture,
} from '../test-utils';
import ConfirmationPage from './ConfirmationPage';
import { confirmationPhase } from './phase';

const session = { bookingId: BOOKING_ID, token: TOKEN };
const PATH = `${BASE}/bookings/${BOOKING_ID}`;

const done = checkoutStateFixture({
  status: 'confirmed',
  checkout_state: 'completed',
  next_action: 'done',
  summary: summaryFixture(),
  confirmation: {
    code: 'AR-7Q2K',
    qr_payload: 'AR-7Q2K',
    qr_svg: '<svg xmlns="http://www.w3.org/2000/svg"><rect width="1" height="1"/></svg>',
    manage_link_sent: true,
    email_masked: 'm***a@e***.it',
  },
});
const waiting = checkoutStateFixture({
  status: 'pending_payment',
  checkout_state: 'paying',
  next_action: 'wait',
  payment: { status: 'pending', amount: 7500 },
});

describe('ConfirmationPage', () => {
  it('polls while the payment is pending and shows the confirmation from the server', async () => {
    const responses = [waiting, waiting, done];
    const post = vi.fn();
    const { fetchMock } = renderWithEnv(
      <ConfirmationPage session={session} onRestart={() => {}} pollMs={10} />,
      { [`GET ${PATH}`]: () => json(200, responses.shift() ?? done) },
      { channel: { origin: 'https://lido.it', post } },
    );
    expect(
      await screen.findByRole('heading', { name: /aspettando la conferma/ }),
    ).toBeInTheDocument();
    expect(
      await screen.findByRole('heading', { name: 'Prenotazione confermata' }),
    ).toBeInTheDocument();
    expect(screen.getByTestId('booking-code')).toHaveTextContent('AR-7Q2K');
    expect(screen.getByRole('img', { name: "QR da mostrare all'ingresso" })).toHaveAttribute(
      'src',
      expect.stringMatching(/^data:image\/svg\+xml/),
    );
    expect(screen.getByText(/m\*\*\*a@e\*\*\*\.it/)).toBeInTheDocument();
    expect(screen.getByText(/Pineta · In assegnazione/)).toBeInTheDocument();
    expect(callsTo(fetchMock, 'GET', PATH).length).toBeGreaterThanOrEqual(3);
    // Only the code goes to the host page, once.
    expect(post).toHaveBeenCalledTimes(1);
    expect(post).toHaveBeenCalledWith({ type: 'arenile:booked', code: 'AR-7Q2K' });
    for (const req of callsTo(fetchMock, 'GET', PATH)) {
      expect(req.headers.get('X-Arenile-Checkout')).toBe(TOKEN);
    }
  });

  it('downloads the .ics with the session header', async () => {
    const user = userEvent.setup();
    const createObjectURL = vi.fn(() => 'blob:ics');
    const revoke = vi.fn();
    Object.assign(URL, { createObjectURL, revokeObjectURL: revoke });
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {});
    const { fetchMock } = renderWithEnv(
      <ConfirmationPage session={session} onRestart={() => {}} />,
      {
        [`GET ${PATH}`]: () => json(200, done),
        [`GET ${PATH}/ics`]: () =>
          new Response('BEGIN:VCALENDAR', {
            status: 200,
            headers: { 'Content-Type': 'text/calendar' },
          }),
      },
    );
    await user.click(await screen.findByRole('button', { name: 'Aggiungi al calendario' }));
    await vi.waitFor(() => expect(click).toHaveBeenCalled());
    expect(callsTo(fetchMock, 'GET', `${PATH}/ics`)[0]!.headers.get('X-Arenile-Checkout')).toBe(
      TOKEN,
    );
    click.mockRestore();
  });

  it('offers to retry a failed payment while the hold is valid', async () => {
    const user = userEvent.setup();
    const failed = checkoutStateFixture({
      status: 'pending_payment',
      next_action: 'pay',
      payment: { status: 'failed', amount: 7500, failure_reason: 'card_declined' },
    });
    const { fetchMock } = renderWithEnv(
      <ConfirmationPage session={session} onRestart={() => {}} />,
      {
        [`GET ${PATH}`]: () => json(200, failed),
        [`POST ${PATH}/pay`]: () => json(200, { mode: 'none', status: 'pending_payment' }),
      },
    );
    expect(
      await screen.findByRole('heading', { name: 'Pagamento non riuscito' }),
    ).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Riprova il pagamento' }));
    await vi.waitFor(() => expect(callsTo(fetchMock, 'POST', `${PATH}/pay`)).toHaveLength(1));
  });

  it('without the session of this tab says the confirmation arrives by email', async () => {
    const onRestart = vi.fn();
    const user = userEvent.setup();
    const { fetchMock } = renderWithEnv(
      <ConfirmationPage session={null} onRestart={onRestart} />,
      {},
    );
    expect(screen.getByRole('heading', { name: 'Pagamento in elaborazione' })).toBeInTheDocument();
    expect(fetchMock).not.toHaveBeenCalled();
    await user.click(screen.getByRole('button', { name: 'Nuova prenotazione' }));
    expect(onRestart).toHaveBeenCalled();
  });

  it('treats an unknown session as the generic outcome', async () => {
    renderWithEnv(<ConfirmationPage session={session} onRestart={() => {}} />, {
      [`GET ${PATH}`]: () => problem(404, 'checkout_not_found'),
    });
    expect(
      await screen.findByRole('heading', { name: 'Pagamento in elaborazione' }),
    ).toBeInTheDocument();
  });

  it('says when the wait is long', async () => {
    renderWithEnv(
      <ConfirmationPage session={session} onRestart={() => {}} pollMs={5} slowAfterMs={30} />,
      {
        [`GET ${PATH}`]: () => json(200, waiting),
      },
    );
    expect(await screen.findByText(/più del previsto/)).toBeInTheDocument();
  });
});

describe('confirmationPhase', () => {
  it.each([
    [undefined, 'loading'],
    [{ next_action: 'done', status: 'confirmed' }, 'done'],
    [{ next_action: 'wait', status: 'pending_payment' }, 'waiting'],
    [{ next_action: 'wait', status: 'awaiting_approval' }, 'approval'],
    [{ next_action: 'pay', status: 'pending_payment', payment: { status: 'failed' } }, 'failed'],
    [{ next_action: 'pay', status: 'pending_payment' }, 'cancelled'],
    [{ next_action: 'expired', status: 'expired' }, 'expired'],
    [{ next_action: 'rejected', status: 'rejected' }, 'rejected'],
  ])('%j → %s', (state, phase) => {
    expect(confirmationPhase(state as never)).toBe(phase);
  });
});
