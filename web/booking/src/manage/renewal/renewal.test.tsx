import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { parseBookingLocation } from '../../location';
import {
  BASE,
  Providers,
  callsTo,
  establishmentFixture,
  fakeApi,
  json,
  problem,
  summaryFixture,
  testI18n,
  type Handler,
} from '../../test-utils';
import { ManagePage } from '../ManagePage';
import { timeLeft } from './time';

const TOKEN = 'r'.repeat(43);
const MANAGE = `${BASE}/manage/${TOKEN}`;
const NOW = Date.parse('2099-06-20T10:00:00Z');

function proposal(overrides: Record<string, unknown> = {}) {
  return {
    status: 'proposed',
    season_year: 2100,
    spot_label: 'A12',
    zone_name: 'Prima fila',
    pack_name: 'Abbonamento stagionale',
    starts_on: '2100-05-15',
    ends_on: '2100-09-20',
    slots: 'FULL',
    previous_amount: 140000,
    total_amount: 150000,
    deposit_due: 50000,
    paid_amount: 0,
    credit_available: 2000,
    deadline_at: '2099-06-23T12:30:00Z',
    terms_version: 'v7',
    allowed: { accept: true, decline: true, pay_deposit: true, pay_full: true, use_credit: true },
    ...overrides,
  };
}

function booking(overrides: Record<string, unknown> = {}) {
  return {
    booking_id: 'b-new',
    code: 'AR-R2100',
    status: 'held',
    is_season_pass: true,
    summary: summaryFixture({ total: 150000, paid: 0, balance_due: 150000 }),
    customer: { first_name: 'Maria', last_name: 'Rossi' },
    allowed: { pay_balance: false, modify: false, cancel: false, release_days: false },
    qr_payload: 'AR-R2100',
    link_expires_at: '2099-08-11T22:00:00Z',
    version: 1,
    renewal: proposal(),
    ...overrides,
  };
}

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

function renderManage(routes: Record<string, Handler>, win?: Window, search = '') {
  const locales = search.includes('lang=en') ? ['it', 'en'] : ['it'];
  const { api, fetchMock } = fakeApi({
    [`GET ${BASE}`]: () => json(200, establishmentFixture({ enabled_locales: locales })),
    ...routes,
  });
  render(
    <Providers api={api} i18n={testI18n()}>
      <ManagePage
        location={parseBookingLocation(`/b/lido-demo/m/${TOKEN}`, search)}
        win={win}
        now={() => NOW}
      />
    </Providers>,
  );
  return { fetchMock };
}

describe('timeLeft', () => {
  it('counts days, hours and minutes down to the deadline', () => {
    expect(timeLeft('2099-06-23T12:30:00Z', NOW)).toEqual({
      expired: false,
      days: 3,
      hours: 2,
      minutes: 30,
    });
    expect(timeLeft('2099-06-20T09:00:00Z', NOW).expired).toBe(true);
  });
});

describe('renewal proposal on the manage page', () => {
  afterEach(() => window.sessionStorage.clear());

  it('shows the proposal with prices and countdown, then accepts with the deposit and credit', async () => {
    const user = userEvent.setup();
    const { win, assign } = spyWindow();
    const { fetchMock } = renderManage(
      {
        [`GET ${MANAGE}`]: () => json(200, booking()),
        [`POST ${MANAGE}/renewal/accept`]: () =>
          json(200, {
            proposal: proposal({ status: 'accepted' }),
            credit_applied: 2000,
            pay: {
              mode: 'redirect',
              status: 'pending_payment',
              redirect_url: 'https://pay.example/r',
            },
          }),
      },
      win,
    );
    const card = (
      await screen.findByRole('heading', { name: "Rinnovo dell'abbonamento 2100" })
    ).closest('[data-slot="card"]') as HTMLElement;
    expect(card).toHaveTextContent('A12 · Prima fila · Abbonamento stagionale');
    expect(card).toHaveTextContent(/1\.500,00\s€/);
    expect(card).toHaveTextContent(/Prezzo della stagione scorsa\s*1\.400,00\s€/);
    expect(within(card).getByText('mancano 3 giorni.')).toBeInTheDocument();
    expect(within(card).getByRole('radio', { name: /Acconto di 500,00/ })).toBeChecked();
    expect(
      within(card).getByRole('checkbox', { name: /Usa il credito disponibile/ }),
    ).toBeChecked();

    // Deposit 500 € minus 20 € credit.
    const accept = within(card).getByRole('button', { name: /Rinnova e paga 480,00/ });
    await user.click(accept);
    expect(await within(card).findByText(/devi accettare le condizioni/)).toBeInTheDocument();
    expect(callsTo(fetchMock, 'POST', `${MANAGE}/renewal/accept`)).toHaveLength(0);

    await user.click(within(card).getByRole('checkbox', { name: /accetto le condizioni/ }));
    await user.click(accept);
    await waitFor(() => expect(assign).toHaveBeenCalledWith('https://pay.example/r'));
    const [req] = callsTo(fetchMock, 'POST', `${MANAGE}/renewal/accept`);
    expect(req!.headers.get('Idempotency-Key')).toBeTruthy();
    expect(JSON.parse(await req!.clone().text())).toEqual({
      payment: 'deposit',
      use_credit: true,
      accept_terms_version: 'v7',
    });
  });

  it('pays the full amount when chosen', async () => {
    const user = userEvent.setup();
    const { win } = spyWindow();
    const { fetchMock } = renderManage(
      {
        [`GET ${MANAGE}`]: () =>
          json(200, booking({ renewal: proposal({ terms_version: undefined }) })),
        [`POST ${MANAGE}/renewal/accept`]: () =>
          json(200, {
            proposal: proposal({ status: 'converted' }),
            pay: { mode: 'none', status: 'confirmed' },
          }),
      },
      win,
    );
    await user.click(await screen.findByRole('radio', { name: /Intero importo di 1\.500,00/ }));
    await user.click(screen.getByRole('checkbox', { name: /Usa il credito disponibile/ }));
    await user.click(screen.getByRole('button', { name: /Rinnova e paga 1\.500,00/ }));
    expect(await screen.findByText(/Rinnovo accettato/)).toBeInTheDocument();
    const [req] = callsTo(fetchMock, 'POST', `${MANAGE}/renewal/accept`);
    expect(JSON.parse(await req!.clone().text())).toEqual({
      payment: 'full',
      accept_terms_version: '',
    });
  });

  it('declines with a reason', async () => {
    const user = userEvent.setup();
    const { fetchMock } = renderManage({
      [`GET ${MANAGE}`]: () => json(200, booking()),
      [`POST ${MANAGE}/renewal/decline`]: () => json(200, proposal({ status: 'declined' })),
    });
    await user.click(await screen.findByRole('button', { name: 'Non rinnovo' }));
    expect(screen.getByText('Vuoi rinunciare alla postazione A12?')).toBeInTheDocument();
    await user.type(screen.getByLabelText(/Vuoi dirci perché/), 'Ci trasferiamo');
    await user.click(screen.getByRole('button', { name: 'Rinuncio al rinnovo' }));
    await waitFor(() =>
      expect(callsTo(fetchMock, 'POST', `${MANAGE}/renewal/decline`)).toHaveLength(1),
    );
    const [req] = callsTo(fetchMock, 'POST', `${MANAGE}/renewal/decline`);
    expect(JSON.parse(await req!.clone().text())).toEqual({ reason: 'Ci trasferiamo' });
  });

  it('explains an expired pre-emption', async () => {
    const user = userEvent.setup();
    renderManage({
      [`GET ${MANAGE}`]: () =>
        json(200, booking({ renewal: proposal({ terms_version: undefined }) })),
      [`POST ${MANAGE}/renewal/accept`]: () => problem(410, 'renewal_expired'),
    });
    await user.click(await screen.findByRole('button', { name: /Rinnova e paga/ }));
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Il periodo di prelazione è scaduto: la postazione è stata liberata.',
    );
  });

  it('shows the outcome of a renewal already confirmed', async () => {
    renderManage({
      [`GET ${MANAGE}`]: () =>
        json(
          200,
          booking({
            status: 'confirmed',
            renewal: proposal({ status: 'converted', paid_amount: 150000 }),
          }),
        ),
    });
    expect(
      await screen.findByText('Rinnovo confermato: ci vediamo in spiaggia!'),
    ).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Rinnova/ })).toBeNull();
  });

  it('loads the English messages lazily', async () => {
    renderManage({ [`GET ${MANAGE}`]: () => json(200, booking()) }, undefined, '?lang=en');
    expect(
      await screen.findByRole('heading', { name: 'Season pass renewal 2100' }),
    ).toBeInTheDocument();
  });
});

describe('credit on the manage page', () => {
  it('uses the credit balance on the balance due', async () => {
    const user = userEvent.setup();
    const base = booking({
      status: 'confirmed',
      is_season_pass: false,
      renewal: undefined,
      allowed: { pay_balance: true, modify: false, cancel: false, release_days: false },
      summary: summaryFixture({ paid: 2500, balance_due: 5000 }),
      credit_available: 2000,
    });
    const { fetchMock } = renderManage({
      [`GET ${MANAGE}`]: () => json(200, base),
      [`POST ${MANAGE}/apply-credit`]: () =>
        json(200, {
          ...base,
          credit_available: 0,
          summary: summaryFixture({ paid: 4500, balance_due: 3000 }),
        }),
    });
    expect(await screen.findByText(/Hai un credito di 20,00/)).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /Usa 20,00\s€ di credito per il saldo/ }));
    expect(await screen.findByText(/Credito di 20,00\s€ applicato/)).toBeInTheDocument();
    expect(
      await screen.findByRole('button', { name: /Paga il saldo di 30,00/ }),
    ).toBeInTheDocument();
    const [req] = callsTo(fetchMock, 'POST', `${MANAGE}/apply-credit`);
    expect(JSON.parse(await req!.clone().text())).toEqual({ amount: 2000 });
  });
});
