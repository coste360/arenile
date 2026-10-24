import { cleanup, render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { parseBookingLocation } from '../location';
import {
  BASE,
  CARD_ZONE,
  PACK_BASE,
  Providers,
  callsTo,
  establishmentFixture,
  fakeApi,
  json,
  problem,
  summaryFixture,
  type Handler,
} from '../test-utils';
import { ManagePage } from './ManagePage';
import { modifySelection } from './modify';

const TOKEN = 'm'.repeat(43);
const MANAGE = `${BASE}/manage/${TOKEN}`;

function booking(overrides: Record<string, unknown> = {}) {
  return {
    booking_id: 'b1',
    code: 'AR-7Q2K',
    status: 'confirmed',
    is_season_pass: false,
    summary: summaryFixture({ paid: 2500, balance_due: 5000 }),
    customer: { first_name: 'Maria', last_name: 'Rossi', email_masked: 'm***a@e***.it' },
    allowed: {
      pay_balance: true,
      modify: true,
      cancel: true,
      release_days: false,
      modify_until: '2099-07-05',
      cancel_until_free: '2099-07-01',
    },
    qr_payload: 'AR-7Q2K',
    qr_svg: '<svg xmlns="http://www.w3.org/2000/svg"/>',
    link_expires_at: '2099-08-11T22:00:00Z',
    version: 4,
    ...overrides,
  };
}

/** The real window with a spy on location.assign (not framed). */
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

function renderManage(
  routes: Record<string, Handler>,
  token: string | null = TOKEN,
  win?: Window,
  path = token ? `/b/lido-demo/m/${token}` : '/b/lido-demo/m/x',
) {
  const { api, fetchMock } = fakeApi({
    [`GET ${BASE}`]: () => json(200, establishmentFixture()),
    ...routes,
  });
  const location = parseBookingLocation(path, '');
  render(
    <Providers api={api}>
      <ManagePage location={location} win={win} now={() => Date.parse('2099-06-20T10:00:00Z')} />
    </Providers>,
  );
  return { fetchMock };
}

describe('ManagePage', () => {
  afterEach(() => {
    window.sessionStorage.clear();
  });

  it('resumes from the stored token at /m after a balance payment (token never in the return URL)', async () => {
    // Opening the link stores the token for this tab...
    renderManage({ [`GET ${MANAGE}`]: () => json(200, booking()) });
    expect(await screen.findByRole('heading', { name: 'La tua prenotazione' })).toBeInTheDocument();
    expect(window.sessionStorage.getItem('arenile:manage:lido-demo')).toBe(TOKEN);
    cleanup();
    // ...so the provider's return URL, without the token, reopens the booking.
    const { fetchMock } = renderManage(
      { [`GET ${MANAGE}`]: () => json(200, booking()) },
      null,
      undefined,
      '/b/lido-demo/m',
    );
    expect(await screen.findByText('AR-7Q2K')).toBeInTheDocument();
    expect(callsTo(fetchMock, 'GET', MANAGE)).toHaveLength(1);
    cleanup();
    // Without a stored token the page asks for a new link.
    window.sessionStorage.clear();
    renderManage({}, null, undefined, '/b/lido-demo/m');
    expect(await screen.findByRole('alert')).toHaveTextContent(/link/i);
  });

  it('shows details, QR, calendar file and link expiry', async () => {
    renderManage({ [`GET ${MANAGE}`]: () => json(200, booking()) });
    expect(await screen.findByRole('heading', { name: 'La tua prenotazione' })).toBeInTheDocument();
    expect(screen.getByText('Confermata')).toBeInTheDocument();
    expect(screen.getByText('AR-7Q2K')).toBeInTheDocument();
    expect(screen.getByText(/Maria Rossi · m\*\*\*a@e\*\*\*\.it/)).toBeInTheDocument();
    expect(screen.getByRole('img', { name: /QR/ })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Aggiungi al calendario' })).toHaveAttribute(
      'href',
      `${MANAGE}/ics`,
    );
    expect(screen.getByText(/Link valido fino al 12\/08\/2099/)).toBeInTheDocument();
  });

  it('pays the balance with a redirect to the provider', async () => {
    const user = userEvent.setup();
    const { win, assign } = spyWindow();
    const { fetchMock } = renderManage(
      {
        [`GET ${MANAGE}`]: () => json(200, booking()),
        [`POST ${MANAGE}/pay`]: () =>
          json(200, {
            mode: 'redirect',
            status: 'confirmed',
            redirect_url: 'https://pay.example/x',
          }),
      },
      TOKEN,
      win,
    );
    await user.click(await screen.findByRole('button', { name: /Paga il saldo di 50,00/ }));
    await vi.waitFor(() => expect(assign).toHaveBeenCalledWith('https://pay.example/x'));
    expect(
      callsTo(fetchMock, 'POST', `${MANAGE}/pay`)[0]!.headers.get('Idempotency-Key'),
    ).toBeTruthy();
  });

  it('previews the refund, then cancels after confirmation with If-Match', async () => {
    const user = userEvent.setup();
    const bodies: unknown[] = [];
    const { fetchMock } = renderManage({
      [`GET ${MANAGE}`]: () => json(200, booking()),
      [`POST ${MANAGE}/cancel`]: async (req) => {
        const body = (await req.json()) as { dry_run: boolean };
        bodies.push(body);
        const refund = { paid_amount: 2500, refund_percent: 50, amount: 1250, as: 'refund' };
        return json(
          200,
          body.dry_run
            ? { applied: false, refund }
            : { applied: true, refund, refund_status: 'provider_pending' },
        );
      },
    });
    await user.click(await screen.findByRole('button', { name: 'Cancella la prenotazione' }));
    expect(screen.getByText('Cancellazione gratuita fino al 01/07/2099.')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Vedi rimborso' }));
    expect(await screen.findByText(/Rimborso: 12,50\s€ \(50%\)/)).toBeInTheDocument();
    const triggers = screen.getAllByRole('button', { name: 'Cancella la prenotazione' });
    await user.click(triggers[triggers.length - 1]!);
    const dialog = await screen.findByRole('alertdialog');
    expect(dialog).toHaveTextContent('AR-7Q2K');
    await user.click(within(dialog).getByRole('button', { name: 'Conferma la cancellazione' }));
    expect(await screen.findByText(/Rimborso avviato/)).toBeInTheDocument();
    expect(bodies).toEqual([
      { dry_run: true, settlement: 'refund' },
      { dry_run: false, settlement: 'refund' },
    ]);
    for (const req of callsTo(fetchMock, 'POST', `${MANAGE}/cancel`)) {
      expect(req.headers.get('If-Match')).toBe('"4"');
    }
  });

  it('shows the status of the refund of a cancelled booking in the customer language', async () => {
    renderManage({
      [`GET ${MANAGE}`]: () =>
        json(
          200,
          booking({
            status: 'cancelled',
            allowed: { pay_balance: false, modify: false, cancel: false, release_days: false },
            refund: {
              status: 'succeeded',
              amount: 2700,
              requested_at: '2099-06-20T08:00:00Z',
              completed_at: '2099-06-20T08:00:05Z',
            },
          }),
        ),
    });
    const notice = await screen.findByText(/Rimborso di 27,00\s€/);
    expect(notice).toBeInTheDocument();
    expect(screen.getByText('Rimborso eseguito sul tuo metodo di pagamento.')).toBeInTheDocument();
    expect(screen.getByText(/Eseguito il 20\/06\/2099/)).toBeInTheDocument();
    expect(screen.queryByText(/manage\.refund/)).not.toBeInTheDocument();
  });

  it('previews a change of dates before applying it', async () => {
    const user = userEvent.setup();
    renderManage({
      [`GET ${MANAGE}`]: () => json(200, booking()),
      [`POST ${MANAGE}/modify`]: () =>
        json(200, {
          applied: false,
          previous_total: 7500,
          new_total: 10000,
          difference: 2500,
          fee: 500,
          conflicts: [],
          violations: [],
        }),
    });
    await user.click(await screen.findByRole('button', { name: 'Cambia date' }));
    expect(screen.getByText('Puoi modificare fino al 05/07/2099.')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Calcola il nuovo prezzo' }));
    expect(await screen.findByText(/Nuovo totale: 100,00/)).toBeInTheDocument();
    expect(screen.getByText(/Differenza da pagare: 25,00/)).toBeInTheDocument();
    expect(screen.getByText(/Penale di modifica inclusa: 5,00/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Conferma la modifica' })).toBeInTheDocument();
  });

  it('season pass holders preview the credit of released days', async () => {
    const user = userEvent.setup();
    renderManage({
      [`GET ${MANAGE}`]: () =>
        json(
          200,
          booking({
            is_season_pass: true,
            allowed: { pay_balance: false, modify: false, cancel: false, release_days: true },
            summary: summaryFixture({
              starts_on: '2099-06-01',
              ends_on: '2099-09-30',
              formula: 'season',
            }),
          }),
        ),
      [`POST ${MANAGE}/releases`]: () =>
        json(200, { applied: false, released: [], credit_amount: 1800 }),
    });
    await user.click(
      await screen.findByRole('button', { name: "Rilascia giorni dell'abbonamento" }),
    );
    const grid = await screen.findByRole('grid');
    const day = within(grid)
      .getAllByRole('button')
      .find((b) => !b.hasAttribute('disabled'));
    await user.click(day!);
    await user.click(screen.getByRole('button', { name: 'Calcola il credito' }));
    expect(await screen.findByText(/Credito previsto: 18,00/)).toBeInTheDocument();
  });

  it('an invalid link offers to request a new one, always with the same answer', async () => {
    const user = userEvent.setup();
    const { fetchMock } = renderManage({
      [`GET ${MANAGE}`]: () => problem(404, 'not_found'),
      [`POST ${BASE}/manage-links`]: () => json(202, null),
    });
    expect(await screen.findByText('Il link non è valido o è scaduto.')).toBeInTheDocument();
    await user.type(screen.getByLabelText('Email'), 'maria@example.it');
    await user.type(screen.getByLabelText('Codice prenotazione'), 'ar-7q2k');
    await user.click(screen.getByRole('button', { name: 'Invia il link' }));
    expect(await screen.findByText(/riceverai un nuovo link/)).toBeInTheDocument();
    const [req] = callsTo(fetchMock, 'POST', `${BASE}/manage-links`);
    expect(await req!.json()).toEqual({ email: 'maria@example.it', code: 'AR-7Q2K' });
  });

  it('a malformed token never calls the manage API', async () => {
    const { fetchMock } = renderManage({}, null);
    expect(await screen.findByText('Il link non è valido o è scaduto.')).toBeInTheDocument();
    expect(callsTo(fetchMock, 'GET', `${BASE}/manage/x`)).toHaveLength(0);
  });
});

describe('modifySelection', () => {
  it('keeps lines, packs, zones and extras and changes the dates', () => {
    const b = booking({
      summary: summaryFixture({
        lines: [
          {
            pack_id: PACK_BASE,
            pack_name: { it: 'Base' },
            zone_id: CARD_ZONE,
            quantity: 1,
            assignment: 'assigned',
            extras: [{ extra_id: 'e1', name: { it: 'Lettino' }, quantity: 1 }],
          },
        ],
      }),
    });
    expect(
      modifySelection(b as never, { from: '2099-07-11', to: '2099-07-13' }, 'multi_day'),
    ).toEqual({
      starts_on: '2099-07-11',
      ends_on: '2099-07-13',
      slots: 'FULL',
      formula: 'multi_day',
      lines: [
        { pack_id: PACK_BASE, zone_id: CARD_ZONE, extras: [{ extra_id: 'e1', quantity: 1 }] },
      ],
    });
  });
});
