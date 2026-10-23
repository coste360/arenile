import { createApiClient } from '@arenile/shared/api';
import { memoryDatabase } from '@arenile/shared/offline';
import { createMemoryHistory } from '@tanstack/react-router';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from 'vitest';
import { App } from '../App';
import {
  EST_ID,
  baseRoutes,
  callsTo,
  json,
  meFor,
  problem,
  renderApp,
} from '../features/configuration/testing';
import { opBooking, opDay, opSpot } from '../features/operations/testing/fixtures';
import { API, operationsRoutes } from '../features/operations/testing/routes';
import { createAdminTestI18n } from '../i18n';
import { createQueryClient } from '../lib/queryClient';
import { createOfflineRuntime } from './runtime';
import { setOnline } from './testing';

const TODAY = `/admin/e/${EST_ID}/oggi?date=2026-07-14`;
const MAP = `/admin/e/${EST_ID}/mappa?date=2026-07-14&slot=FULL`;

const day = opDay([
  opSpot('s1', 'A1', 'free'),
  opSpot('s2', 'A2', 'booked', {
    booking: opBooking({ total_amount: 3000, paid_amount: 1000, balance_due: 2000 }),
  }),
]);

// The pages show 2026-07-14: check-in is offered only when that day is today.
beforeAll(() => {
  vi.useFakeTimers({ toFake: ['Date'] });
  vi.setSystemTime(new Date('2026-07-14T10:00:00'));
});
afterAll(() => {
  vi.useRealTimers();
});
afterEach(() => {
  setOnline(true);
});

const status = () => screen.getAllByRole('status').find((s) => s.closest('header'));

describe('Offline check-in (spec 21.2)', () => {
  it('queues the check-in offline and replays it with its idempotency key on reconnection', async () => {
    const user = userEvent.setup();
    const { fetchMock } = renderApp(TODAY, {
      ...baseRoutes('cashier'),
      ...operationsRoutes(day, {
        [`POST ${API}/bookings/b1/check-in`]: () => json(200, { id: 'b1' }),
      }),
    });
    const button = await screen.findByRole('button', { name: 'Registra il check-in di Rossi M.' });
    expect(status()).toHaveTextContent('Online');

    setOnline(false);
    expect(status()).toHaveTextContent('Offline');
    expect(screen.getByText('Sei offline')).toBeInTheDocument();
    expect(
      screen.getByText(/Nuove prenotazioni e spostamenti non sono disponibili/),
    ).toBeInTheDocument();

    await user.click(button);
    expect(
      await screen.findByText('Check-in di AR-0001 in coda: sarà inviato alla riconnessione.'),
    ).toBeInTheDocument();
    await waitFor(() => expect(status()).toHaveTextContent('Offline · 1 in coda'));
    expect(callsTo(fetchMock, 'POST', `${API}/bookings/b1/check-in`)).toHaveLength(0);

    setOnline(true);
    await waitFor(() =>
      expect(callsTo(fetchMock, 'POST', `${API}/bookings/b1/check-in`)).toHaveLength(1),
    );
    const req = callsTo(fetchMock, 'POST', `${API}/bookings/b1/check-in`)[0] as Request;
    expect(req.headers.get('Idempotency-Key')).toMatch(/.+/);
    expect(req.headers.get('If-Match')).toBe('"1"');
    expect(await screen.findByText('1 operazione offline inviata.')).toBeInTheDocument();
    await waitFor(() => expect(status()).toHaveTextContent(/^Online$/));
    expect(screen.queryByText('Sei offline')).toBeNull();
  });

  it('shows a refused operation with the server detail; it can be discarded', async () => {
    const user = userEvent.setup();
    renderApp(TODAY, {
      ...baseRoutes('cashier'),
      ...operationsRoutes(day, {
        [`POST ${API}/bookings/b1/check-in`]: () =>
          problem(409, 'invalid_transition', { detail: 'La prenotazione è già stata annullata' }),
      }),
    });
    const button = await screen.findByRole('button', { name: 'Registra il check-in di Rossi M.' });
    setOnline(false);
    await user.click(button);
    await waitFor(() => expect(status()).toHaveTextContent('1 in coda'));
    setOnline(true);

    const banner = await screen.findByText('Operazioni offline da rivedere');
    await waitFor(() => expect(status()).toHaveTextContent('1 da rivedere'));
    await user.click(
      within(banner.closest('[role="alert"]') as HTMLElement).getByRole('button', {
        name: 'Rivedi',
      }),
    );
    const sheet = await screen.findByRole('dialog', { name: 'Operazioni offline' });
    expect(within(sheet).getByText('La prenotazione è già stata annullata')).toBeInTheDocument();
    expect(within(sheet).getByText('Conflitto')).toBeInTheDocument();
    await user.click(within(sheet).getByRole('button', { name: 'Scarta' }));
    const confirm = await screen.findByRole('alertdialog');
    await user.click(within(confirm).getByRole('button', { name: 'Scarta' }));
    await waitFor(() => expect(screen.queryByText('Operazioni offline da rivedere')).toBeNull());
  });
});

describe('Check-in on an unreachable network', () => {
  it('queues the request that did not reach the server with the same idempotency key', async () => {
    const user = userEvent.setup();
    let reachable = false;
    const { fetchMock } = renderApp(TODAY, {
      ...baseRoutes('cashier'),
      ...operationsRoutes(day, {
        [`POST ${API}/bookings/b1/check-in`]: () => {
          if (!reachable) throw new TypeError('Failed to fetch');
          return json(200, { id: 'b1' });
        },
      }),
    });
    await user.click(
      await screen.findByRole('button', { name: 'Registra il check-in di Rossi M.' }),
    );
    // Toasts of earlier tests may still be on screen (sonner's store is global).
    expect(
      await screen.findAllByText('Check-in di AR-0001 in coda: sarà inviato alla riconnessione.'),
    ).not.toHaveLength(0);
    await waitFor(() => expect(status()).toHaveTextContent('1 in coda'));

    reachable = true;
    await user.click(screen.getByRole('button', { name: /Apri le operazioni offline/ }));
    const sheet = await screen.findByRole('dialog', { name: 'Operazioni offline' });
    await user.click(within(sheet).getByRole('button', { name: 'Invia ora' }));
    await waitFor(() =>
      expect(within(sheet).getByText('Nessuna operazione in coda.')).toBeVisible(),
    );
    const keys = callsTo(fetchMock, 'POST', `${API}/bookings/b1/check-in`).map((r) =>
      (r as Request).headers.get('Idempotency-Key'),
    );
    // Online attempt, immediate retry after enqueue, manual send: one key.
    expect(keys.length).toBeGreaterThanOrEqual(2);
    expect(new Set(keys).size).toBe(1);
  });
});

describe('Offline spot card (spec 21.2)', () => {
  it('queues a cash payment on an existing booking and sends it on reconnection', async () => {
    const user = userEvent.setup();
    const { fetchMock } = renderApp(MAP, {
      ...baseRoutes('cashier'),
      ...operationsRoutes(day, {
        [`POST ${API}/bookings/b1/payments`]: () => json(201, { id: 'p1' }),
      }),
    });
    await screen.findByRole('button', { name: /Postazione A2/ });
    setOnline(false);
    await user.click(screen.getByRole('button', { name: /Postazione A2/ }));
    const sheet = await screen.findByRole('dialog');
    expect(
      within(sheet).getByText('Offline: nuove prenotazioni e spostamenti non sono disponibili.'),
    ).toBeInTheDocument();
    await user.click(within(sheet).getByRole('button', { name: 'Incasso in contanti' }));
    const dialog = await screen.findByRole('dialog', { name: 'Incasso in contanti offline' });
    expect(within(dialog).getByLabelText(/Importo/)).toHaveValue('20,00');
    await user.click(within(dialog).getByRole('button', { name: 'Metti in coda' }));
    expect(
      await screen.findByText('Incasso di 20,00 € in coda: sarà inviato alla riconnessione.'),
    ).toBeInTheDocument();
    expect(callsTo(fetchMock, 'POST', `${API}/bookings/b1/payments`)).toHaveLength(0);

    setOnline(true);
    await waitFor(() =>
      expect(callsTo(fetchMock, 'POST', `${API}/bookings/b1/payments`)).toHaveLength(1),
    );
    const req = callsTo(fetchMock, 'POST', `${API}/bookings/b1/payments`)[0] as Request;
    expect(req.headers.get('Idempotency-Key')).toMatch(/.+/);
    expect(await req.json()).toEqual({ amount: 2000, method: 'cash' });
  });

  it('refuses a cash payment above the balance and requires the note text', async () => {
    const user = userEvent.setup();
    renderApp(MAP, { ...baseRoutes('cashier'), ...operationsRoutes(day) });
    await screen.findByRole('button', { name: /Postazione A2/ });
    setOnline(false);
    await user.click(screen.getByRole('button', { name: /Postazione A2/ }));
    const sheet = await screen.findByRole('dialog');

    await user.click(within(sheet).getByRole('button', { name: 'Incasso in contanti' }));
    let dialog = await screen.findByRole('dialog', { name: 'Incasso in contanti offline' });
    const amount = within(dialog).getByLabelText(/Importo/);
    await user.clear(amount);
    await user.type(amount, '25');
    await user.click(within(dialog).getByRole('button', { name: 'Metti in coda' }));
    expect(
      await within(dialog).findByText(
        'Inserisci un importo maggiore di zero e non superiore al saldo.',
      ),
    ).toBeInTheDocument();
    await user.click(within(dialog).getByRole('button', { name: 'Annulla' }));

    await user.click(within(sheet).getByRole('button', { name: 'Aggiungi nota' }));
    dialog = await screen.findByRole('dialog', { name: 'Nota offline' });
    await user.click(within(dialog).getByRole('button', { name: 'Metti in coda' }));
    expect(await within(dialog).findByText('Scrivi il testo della nota.')).toBeInTheDocument();
  });

  it('explains that a new booking is not available offline and never sends it', async () => {
    const user = userEvent.setup();
    const { fetchMock } = renderApp(MAP, { ...baseRoutes('cashier'), ...operationsRoutes(day) });
    await screen.findByRole('button', { name: /Postazione A1/ });
    setOnline(false);
    await user.click(screen.getByRole('button', { name: /Postazione A1/ }));
    const sheet = await screen.findByRole('dialog');
    await user.click(within(sheet).getByRole('button', { name: 'Nuova prenotazione' }));
    expect(await screen.findByText('Non disponibile offline')).toBeInTheDocument();
    expect(
      screen.getByText(/solo il server garantisce che la postazione sia libera/),
    ).toBeInTheDocument();
    expect(callsTo(fetchMock, 'POST', `${API}/bookings`)).toHaveLength(0);
  });
});

describe('Logout', () => {
  it('sends the queue first, then clears the offline data of the device', async () => {
    const user = userEvent.setup();
    const order: string[] = [];
    let loggedIn = true;
    const routes: Record<string, (req: Request) => Response> = {
      ...baseRoutes('cashier'),
      'GET /api/admin/v1/auth/me': () =>
        loggedIn ? json(200, meFor('cashier')) : problem(401, 'unauthorized'),
      'POST /api/admin/v1/auth/logout': () => {
        order.push('logout');
        loggedIn = false;
        return new Response(null, { status: 204 });
      },
    };
    const fetchMock = vi.fn(async (req: Request) => {
      const handler = routes[`${req.method} ${new URL(req.url).pathname}`];
      return handler ? handler(req) : problem(404, 'not_found');
    });
    const api = createApiClient({
      baseUrl: 'http://localhost',
      fetch: fetchMock,
      getCsrfToken: () => 'c',
    });
    const queryClient = createQueryClient();
    const db = memoryDatabase('logout');
    const offline = createOfflineRuntime({ queryClient, api, db });
    await offline.start();
    const beforeLogout = offline.beforeLogout.bind(offline);
    const clearAll = offline.clearAll.bind(offline);
    vi.spyOn(offline, 'beforeLogout').mockImplementation(async () => {
      order.push('before');
      await beforeLogout();
    });
    vi.spyOn(offline, 'clearAll').mockImplementation(async () => {
      order.push('clear');
      await clearAll();
    });
    await offline.enqueue({
      kind: 'note',
      establishmentId: EST_ID,
      bookingId: 'b1',
      label: 'A2',
      text: 'x',
    });
    const history = createMemoryHistory({ initialEntries: ['/admin/configurazione'] });
    render(
      <App
        api={api}
        i18n={createAdminTestI18n()}
        queryClient={queryClient}
        history={history}
        offline={offline}
      />,
    );
    await user.click(await screen.findByRole('button', { name: /Menu utente/ }));
    await user.click(await screen.findByRole('menuitem', { name: 'Esci' }));
    await waitFor(() => expect(history.location.pathname).toBe('/admin/login'));
    await waitFor(() => expect(order).toEqual(['before', 'logout', 'clear']));
    expect(await db.store('queue').values()).toEqual([]);
    expect(await db.store('cache').get('queries')).toBeUndefined();
  });
});
