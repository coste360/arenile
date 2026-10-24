import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  EST_ID,
  callsTo,
  json,
  list,
  problem,
  renderApp,
  type Handler,
} from '../configuration/testing';
import {
  API,
  EXPECTED_CASH,
  booking,
  closedSession,
  barRegister,
  eur,
  mainRegister,
  norm,
  openDetail,
  openSession,
  roleRoutes,
  type CashRole,
} from './testing';

const PATH = `/admin/e/${EST_ID}/cassa`;

function routes(role: CashRole, extra: Record<string, Handler> = {}) {
  return {
    ...roleRoutes(role),
    [`GET ${API}/cash-sessions`]: () => list([]),
    [`GET ${API}/cash-registers`]: () => list([mainRegister]),
    ...extra,
  };
}

afterEach(() => vi.restoreAllMocks());

describe('CashPage', () => {
  it('opens a session with the float in integer cents and an idempotency key', async () => {
    const user = userEvent.setup();
    let current: unknown = null;
    let body: Record<string, unknown> | undefined;
    const { fetchMock } = renderApp(
      PATH,
      routes('cashier', {
        [`GET ${API}/cash-sessions/current`]: () =>
          current ? json(200, current) : problem(404, 'not_found'),
        [`POST ${API}/cash-sessions`]: async (req) => {
          body = (await req.json()) as Record<string, unknown>;
          current = {
            ...openDetail,
            session: { ...openSession, opening_float: 15050 },
            movements: [],
            payments: [],
            totals_by_method: {},
          };
          return json(201, openSession);
        },
      }),
    );
    const card = await screen.findByRole('form', { name: 'Apri la cassa' });
    await user.click(within(card).getByRole('button', { name: 'Apri sessione' }));
    expect(await within(card).findByText('Inserisci un importo valido.')).toBeInTheDocument();
    await user.type(within(card).getByLabelText('Fondo cassa'), '150,50');
    await user.click(within(card).getByRole('button', { name: 'Apri sessione' }));
    await waitFor(() => expect(body).toEqual({ register_id: 'r-main', opening_float: 15050 }));
    const [post] = callsTo(fetchMock, 'POST', `${API}/cash-sessions`);
    expect(post?.headers.get('Idempotency-Key')).toBeTruthy();
    expect(norm((await screen.findByTestId('stat-expected-cash')).textContent)).toBe(
      norm(eur(15050)),
    );
  });

  it('shows running totals and the expected cash of the open session', async () => {
    renderApp(
      PATH,
      routes('cashier', { [`GET ${API}/cash-sessions/current`]: () => json(200, openDetail) }),
    );
    expect(norm((await screen.findByTestId('stat-expected-cash')).textContent)).toBe(
      norm(eur(EXPECTED_CASH)),
    );
    expect(norm(screen.getByTestId('stat-float').textContent)).toBe(norm(eur(10000)));
    expect(norm(screen.getByTestId('stat-takings').textContent)).toBe(norm(eur(17500)));
    expect(norm(screen.getByTestId('stat-movements').textContent)).toBe(norm(eur(1100)));
    const payments = screen.getByRole('table', { name: 'Incassi' });
    expect(within(payments).getAllByText('AR-7K3Q9')).toHaveLength(2);
    expect(within(payments).getByText('Rimborso')).toBeInTheDocument();
  });

  it('closes with the counted amounts: live difference, If-Match, Z summary', async () => {
    const user = userEvent.setup();
    let current: unknown = openDetail;
    let body: Record<string, unknown> | undefined;
    const { fetchMock } = renderApp(
      PATH,
      routes('cashier', {
        [`GET ${API}/cash-sessions/current`]: () =>
          current ? json(200, current) : problem(404, 'not_found'),
        [`POST ${API}/cash-sessions/s1/close`]: async (req) => {
          body = (await req.json()) as Record<string, unknown>;
          current = null;
          return json(200, closedSession);
        },
        [`GET ${API}/cash-sessions/s1`]: () => json(200, { ...openDetail, session: closedSession }),
      }),
    );
    await user.click(await screen.findByRole('button', { name: 'Chiudi cassa' }));
    const dialog = await screen.findByRole('dialog', { name: 'Chiusura di cassa' });
    // Cash must be counted.
    await user.click(within(dialog).getByRole('button', { name: 'Chiudi sessione' }));
    expect(await within(dialog).findByText('Inserisci i contanti contati.')).toBeInTheDocument();

    await user.type(within(dialog).getByLabelText('Contato Contanti'), '255,90');
    const total = within(dialog).getByTestId('close-total-difference');
    await waitFor(() => expect(norm(total.textContent)).toContain(norm(`-${eur(10)}`)));
    expect(within(total).getByText('Ammanco')).toBeInTheDocument();

    await user.clear(within(dialog).getByLabelText('Contato Contanti'));
    await user.type(within(dialog).getByLabelText('Contato Contanti'), '256');
    await waitFor(() => expect(within(total).getByText('In pari')).toBeInTheDocument());
    await user.type(within(dialog).getByLabelText('Contato Contanti'), ',01');
    await waitFor(() => expect(within(total).getByText('Eccedenza')).toBeInTheDocument());
    expect(norm(total.textContent)).toContain(norm(`+${eur(1)}`));

    await user.click(within(dialog).getByRole('button', { name: 'Chiudi sessione' }));
    await waitFor(() => expect(body).toEqual({ counted_totals: { cash: 25601, card: 3000 } }));
    const [post] = callsTo(fetchMock, 'POST', `${API}/cash-sessions/s1/close`);
    expect(post?.headers.get('If-Match')).toBe('"3"');

    const summary = await screen.findByRole('article', { name: 'Chiusura Z' });
    expect(within(summary).getAllByText('Ammanco').length).toBeGreaterThan(0);
    expect(screen.getByRole('form', { name: 'Apri la cassa' })).toBeInTheDocument();
  });

  it('records a cash movement out with its reason', async () => {
    const user = userEvent.setup();
    let body: Record<string, unknown> | undefined;
    renderApp(
      PATH,
      routes('cashier', {
        [`GET ${API}/cash-sessions/current`]: () => json(200, openDetail),
        [`POST ${API}/cash-sessions/s1/movements`]: async (req) => {
          body = (await req.json()) as Record<string, unknown>;
          return json(201, {
            id: 'm3',
            cash_session_id: 's1',
            created_at: '2026-07-15T11:00:00Z',
            ...body,
          });
        },
      }),
    );
    await user.click(await screen.findByRole('button', { name: 'Movimento' }));
    const dialog = await screen.findByRole('dialog', { name: 'Movimento di cassa' });
    await user.click(within(dialog).getByRole('radio', { name: 'Prelievo' }));
    await user.type(within(dialog).getByLabelText('Importo'), '50');
    await user.click(within(dialog).getByRole('button', { name: 'Registra movimento' }));
    expect(await within(dialog).findByText('Indica la causale.')).toBeInTheDocument();
    await user.type(within(dialog).getByLabelText('Causale'), 'Versamento in banca');
    await user.click(within(dialog).getByRole('button', { name: 'Registra movimento' }));
    await waitFor(() =>
      expect(body).toEqual({ kind: 'out', amount: 5000, reason: 'Versamento in banca' }),
    );
  });

  it('takes a payment for a booking found by code, proposing the balance due', async () => {
    const user = userEvent.setup();
    let body: Record<string, unknown> | undefined;
    const { fetchMock } = renderApp(
      PATH,
      routes('cashier', {
        [`GET ${API}/cash-sessions/current`]: () => json(200, openDetail),
        [`GET ${API}/bookings`]: () => list([booking]),
        [`POST ${API}/bookings/b9/payments`]: async (req) => {
          body = (await req.json()) as Record<string, unknown>;
          return json(201, {
            id: 'p9',
            provider: 'manual',
            direction: 'in',
            status: 'succeeded',
            created_at: '2026-07-15T12:00:00Z',
            ...body,
          });
        },
      }),
    );
    await user.click(await screen.findByRole('button', { name: 'Incassa' }));
    const dialog = await screen.findByRole('dialog', { name: 'Incassa prenotazione' });
    await user.type(within(dialog).getByLabelText('Codice prenotazione o cliente'), 'AR-9Z');
    await user.click(
      await within(dialog).findByRole('button', { name: 'Seleziona la prenotazione AR-9Z8Y7' }),
    );
    const search = callsTo(fetchMock, 'GET', `${API}/bookings`).at(-1);
    expect(new URL(search?.url ?? 'http://x').searchParams.get('q')).toBe('AR-9Z');
    expect(within(dialog).getByLabelText('Importo')).toHaveValue('35,00');
    await user.selectOptions(within(dialog).getByLabelText('Metodo'), 'card');
    await user.click(within(dialog).getByRole('button', { name: 'Registra incasso' }));
    await waitFor(() =>
      expect(body).toEqual({ amount: 3500, method: 'card', cash_session_id: 's1' }),
    );
    const [post] = callsTo(fetchMock, 'POST', `${API}/bookings/b9/payments`);
    expect(post?.headers.get('Idempotency-Key')).toBeTruthy();
  });

  it('shows the history and the detail of a closed session', async () => {
    const user = userEvent.setup();
    renderApp(
      PATH,
      routes('cashier', {
        [`GET ${API}/cash-sessions/current`]: () => problem(404, 'not_found'),
        [`GET ${API}/cash-sessions`]: () => list([closedSession]),
        [`GET ${API}/cash-sessions/s1`]: () => json(200, { ...openDetail, session: closedSession }),
      }),
    );
    const table = await screen.findByRole('table', { name: 'Storico sessioni' });
    // Settle the registers and the current-session queries first (rows re-render).
    await screen.findByRole('form', { name: 'Apri la cassa' });
    const row = (await within(table).findByText('Chiusa')).closest('tr') as HTMLElement;
    expect(norm(within(row).getByTestId('difference-amount').textContent)).toBe(
      norm(`-${eur(10)}`),
    );
    await user.click(row);
    const sheet = await screen.findByRole('dialog');
    const z = await within(sheet).findByRole('article', { name: 'Chiusura Z' });
    expect(within(z).getByText('Cassa principale')).toBeInTheDocument();

    const print = vi.spyOn(window, 'print').mockImplementation(() => {});
    await user.click(within(sheet).getByRole('button', { name: 'Stampa chiusura Z' }));
    expect(print).toHaveBeenCalledOnce();
    // The print-only area is aria-hidden behind the modal sheet; it is still rendered for paper.
    expect(
      within(screen.getByTestId('print-area')).getByRole('article', {
        name: 'Chiusura Z',
        hidden: true,
      }),
    ).toBeInTheDocument();
  });

  it('picks the register of this device and opens a session on it', async () => {
    const user = userEvent.setup();
    window.localStorage.removeItem(`arenile.cash.register.${EST_ID}`);
    let body: Record<string, unknown> | undefined;
    const { fetchMock } = renderApp(
      PATH,
      routes('cashier', {
        [`GET ${API}/cash-registers`]: () =>
          list([{ ...mainRegister, open_session_id: 's1' }, barRegister]),
        [`GET ${API}/cash-sessions/current`]: (req) =>
          new URL(req.url).searchParams.get('register_id') === 'r-main'
            ? json(200, openDetail)
            : problem(404, 'not_found'),
        [`POST ${API}/cash-sessions`]: async (req) => {
          body = (await req.json()) as Record<string, unknown>;
          return json(201, { ...openSession, id: 's2', register: 'bar', register_id: 'r-bar' });
        },
      }),
    );
    // Default register first: its open session is shown.
    await screen.findByTestId('stat-expected-cash');
    const picker = screen.getByRole('combobox', { name: 'Cassa' });
    expect(picker).toHaveTextContent('Cassa principale · aperta');
    await user.click(picker);
    await user.click(await screen.findByRole('option', { name: 'Bar spiaggia · chiusa' }));
    const card = await screen.findByRole('form', { name: 'Apri Bar spiaggia' });
    expect(window.localStorage.getItem(`arenile.cash.register.${EST_ID}`)).toBe('r-bar');
    const currents = callsTo(fetchMock, 'GET', `${API}/cash-sessions/current`);
    expect(currents.map((r) => new URL(r.url).searchParams.get('register_id'))).toContain('r-bar');
    await user.type(within(card).getByLabelText('Fondo cassa'), '20');
    await user.click(within(card).getByRole('button', { name: 'Apri sessione' }));
    await waitFor(() => expect(body).toEqual({ register_id: 'r-bar', opening_float: 2000 }));
    window.localStorage.removeItem(`arenile.cash.register.${EST_ID}`);
  });

  it('prints the server Z closing (PDF) through a hidden iframe', async () => {
    const user = userEvent.setup();
    renderApp(
      PATH,
      routes('cashier', { [`GET ${API}/cash-sessions/current`]: () => json(200, openDetail) }),
    );
    await screen.findByTestId('stat-expected-cash');
    await user.click(screen.getByRole('button', { name: 'PDF chiusura Z' }));
    const frame = document.querySelector('iframe[title="z-report"]') as HTMLIFrameElement;
    expect(frame).not.toBeNull();
    expect(frame.getAttribute('src')).toBe(`${API}/cash-sessions/s1/z-report?paper=a4`);
    frame.remove();
  });

  it('is read-only for the viewer', async () => {
    renderApp(
      PATH,
      routes('viewer', {
        [`GET ${API}/cash-sessions/current`]: () => problem(403, 'forbidden'),
        [`GET ${API}/cash-sessions`]: () => list([closedSession]),
      }),
    );
    expect(await screen.findByText(/sola lettura/)).toBeInTheDocument();
    await screen.findByRole('table', { name: 'Storico sessioni' });
    expect(screen.queryByRole('button', { name: 'Incassa' })).not.toBeInTheDocument();
    expect(screen.queryByRole('form', { name: 'Apri la cassa' })).not.toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Corrispettivi' })).toBeInTheDocument();
  });

  it('denies roles without cash permissions', async () => {
    renderApp(PATH, routes('beach_staff'));
    expect(await screen.findByText('Il tuo ruolo non ha accesso alla cassa.')).toBeInTheDocument();
  });
});
