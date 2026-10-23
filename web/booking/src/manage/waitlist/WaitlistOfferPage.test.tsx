import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { createCheckoutStore } from '../../checkout/session';
import { parseBookingLocation } from '../../location';
import {
  BASE,
  BOOKING_ID,
  Providers,
  callsTo,
  establishmentFixture,
  fakeApi,
  json,
  problem,
  quoteFixture,
  type Handler,
} from '../../test-utils';
import WaitlistOfferPage from './WaitlistOfferPage';

const TOKEN = 'w'.repeat(43);
const ENTRY_URL = `${BASE}/waitlist/${TOKEN}`;
const NOW = Date.parse('2099-07-01T10:00:00Z');

function entry(overrides: Record<string, unknown> = {}) {
  return {
    status: 'waiting',
    starts_on: '2099-07-10',
    ends_on: '2099-07-12',
    slots: 'FULL',
    pack_name: 'Ombrellone base',
    zone_name: 'Prima fila',
    quantity: 1,
    ...overrides,
  };
}

const OFFERED = entry({
  status: 'offered',
  offer: { status: 'pending', spot_label: 'B7', expires_at: '2099-07-01T11:45:00Z' },
});

function spyWindow() {
  const assign = vi.fn();
  const proxy: Window = new Proxy(window, {
    get(target, prop) {
      if (prop === 'location')
        return { ...window.location, assign, origin: window.location.origin };
      if (prop === 'top' || prop === 'parent' || prop === 'self') return proxy;
      const value = Reflect.get(target, prop);
      return typeof value === 'function' ? value.bind(target) : value;
    },
  });
  return { win: proxy, assign };
}

function renderPage(routes: Record<string, Handler>, win?: Window, path?: string) {
  const { api, fetchMock } = fakeApi({
    [`GET ${BASE}`]: () => json(200, establishmentFixture()),
    ...routes,
  });
  render(
    <Providers api={api}>
      <WaitlistOfferPage
        location={parseBookingLocation(path ?? `/b/lido-demo/m/w/${TOKEN}`, '')}
        win={win}
        now={() => NOW}
      />
    </Providers>,
  );
  return { fetchMock };
}

describe('WaitlistOfferPage', () => {
  afterEach(() => window.sessionStorage.clear());

  it('shows the waiting request and lets the customer leave the list', async () => {
    const user = userEvent.setup();
    const { fetchMock } = renderPage({
      [`GET ${ENTRY_URL}`]: () => json(200, entry()),
      [`DELETE ${ENTRY_URL}`]: () => json(204, null),
    });
    expect(
      await screen.findByRole('heading', { name: "La tua richiesta in lista d'attesa" }),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/10\/07\/2099 – 12\/07\/2099 · Ombrellone base · Prima fila/),
    ).toBeInTheDocument();
    expect(screen.getByText(/Sei in lista/)).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: "Esci dalla lista d'attesa" }));
    await user.click(screen.getByRole('button', { name: 'Esci dalla lista' }));
    expect(await screen.findByText(/Non sei più in lista d'attesa/)).toBeInTheDocument();
    expect(callsTo(fetchMock, 'DELETE', ENTRY_URL)).toHaveLength(1);
  });

  it('accepts a pending offer and continues in the checkout with the stored session', async () => {
    const user = userEvent.setup();
    const { win, assign } = spyWindow();
    const { fetchMock } = renderPage(
      {
        [`GET ${ENTRY_URL}`]: () => json(200, OFFERED),
        [`POST ${ENTRY_URL}/accept`]: () =>
          json(201, {
            booking_id: BOOKING_ID,
            checkout_token: 'c'.repeat(43),
            status: 'held',
            hold_expires_at: '2099-07-01T11:45:00Z',
            quote: quoteFixture(),
          }),
      },
      win,
    );
    expect(await screen.findByText('Si è liberata la postazione B7 per te!')).toBeInTheDocument();
    expect(screen.getByText('mancano 1 h 45 min.')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Prenota ora' }));
    await waitFor(() => expect(assign).toHaveBeenCalledWith('/b/lido-demo'));
    const [req] = callsTo(fetchMock, 'POST', `${ENTRY_URL}/accept`);
    expect(req!.headers.get('Idempotency-Key')).toBeTruthy();
    expect(createCheckoutStore(window.sessionStorage).load('lido-demo')).toEqual({
      bookingId: BOOKING_ID,
      token: 'c'.repeat(43),
      holdExpiresAt: '2099-07-01T11:45:00Z',
    });
  });

  it('declines the offer and leaves the list', async () => {
    const user = userEvent.setup();
    const { fetchMock } = renderPage({
      [`GET ${ENTRY_URL}`]: () => json(200, OFFERED),
      [`POST ${ENTRY_URL}/decline`]: () => json(200, entry({ status: 'removed' })),
    });
    await user.click(await screen.findByRole('button', { name: 'Rinuncia' }));
    await user.click(screen.getByRole('checkbox', { name: "Esci anche dalla lista d'attesa" }));
    await user.click(screen.getByRole('button', { name: "Rinuncio all'offerta" }));
    expect(await screen.findByText(/Non sei più in lista d'attesa/)).toBeInTheDocument();
    const [req] = callsTo(fetchMock, 'POST', `${ENTRY_URL}/decline`);
    expect(JSON.parse(await req!.clone().text())).toEqual({ leave: true });
  });

  it('explains an expired offer', async () => {
    const user = userEvent.setup();
    renderPage({
      [`GET ${ENTRY_URL}`]: () => json(200, OFFERED),
      [`POST ${ENTRY_URL}/accept`]: () => problem(410, 'waitlist_offer_expired'),
    });
    await user.click(await screen.findByRole('button', { name: 'Prenota ora' }));
    expect(await screen.findByRole('alert')).toHaveTextContent(
      "L'offerta è scaduta o non è più disponibile.",
    );
  });

  it('says the link is invalid without telling why', async () => {
    renderPage({ [`GET ${ENTRY_URL}`]: () => problem(404, 'not_found') });
    expect(await screen.findByText(/Il link non è valido/)).toBeInTheDocument();
    renderPage({}, undefined, '/b/lido-demo/m/w/short');
    expect((await screen.findAllByText(/Il link non è valido/)).length).toBeGreaterThan(0);
  });
});
