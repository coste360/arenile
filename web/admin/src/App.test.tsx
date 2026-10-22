import { createApiClient } from '@arenile/shared/api';
import { createTestI18n } from '@arenile/shared/i18n';
import { createMemoryHistory } from '@tanstack/react-router';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { App } from './App';
import { createQueryClient } from './lib/queryClient';
import { safeRedirect } from './router';

type Handler = (req: Request) => Response | Promise<Response>;

const ME = {
  principal_type: 'user',
  user: {
    id: 'u1',
    email: 'gestore@example.it',
    name: 'Mario Rossi',
    locale: 'it',
    totp_enabled: false,
  },
  memberships: [],
  totp_enrolment_required: false,
};

const ESTABLISHMENT = {
  id: 'e1',
  organization_id: 'o1',
  slug: 'lido-demo',
  name: 'Lido Demo',
  timezone: 'Europe/Rome',
  currency: 'EUR',
  default_locale: 'it',
  enabled_locales: ['it', 'en'],
  contact_email: '',
  contact_phone: '',
  address: {},
  booking_enabled: true,
  allowed_embed_origins: [],
  theme: {},
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
};

function json(status: number, body: unknown) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': status >= 400 ? 'application/problem+json' : 'application/json' },
  });
}

function problem(status: number, type: string) {
  return json(status, { type, title: type, status });
}

function setup(path: string, routes: Record<string, Handler>) {
  const fetchMock = vi.fn(async (req: Request) => {
    const url = new URL(req.url);
    const handler = routes[`${req.method} ${url.pathname}`];
    return handler ? handler(req) : problem(404, 'not_found');
  });
  const api = createApiClient({
    baseUrl: 'http://localhost',
    fetch: fetchMock,
    getCsrfToken: () => 'csrf',
  });
  const history = createMemoryHistory({ initialEntries: [path] });
  render(
    <App api={api} i18n={createTestI18n()} queryClient={createQueryClient()} history={history} />,
  );
  return { fetchMock, history };
}

describe('admin app', () => {
  it('redirects anonymous users to the login form', async () => {
    const { history } = setup('/admin/prenotazioni', {
      'GET /api/admin/v1/auth/me': () => problem(401, 'unauthorized'),
    });
    expect(await screen.findByRole('heading', { name: 'Accedi al pannello' })).toBeInTheDocument();
    expect(screen.getByLabelText('Email')).toBeInTheDocument();
    expect(screen.getByLabelText('Password')).toBeInTheDocument();
    expect(history.location.pathname).toBe('/admin/login');
  });

  it('validates the login form before calling the API', async () => {
    const user = userEvent.setup();
    const { fetchMock } = setup('/admin/login', {});
    await user.click(await screen.findByRole('button', { name: 'Accedi' }));
    expect(await screen.findByText('Inserisci un indirizzo email valido')).toBeInTheDocument();
    expect(screen.getByText('Inserisci la password')).toBeInTheDocument();
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it.each([
    ['invalid_credentials', 401, 'Email o password non corretti.'],
    ['rate_limited', 429, 'Troppi tentativi di accesso. Riprova tra qualche minuto.'],
    ['internal_error', 500, 'Accesso non riuscito. Riprova.'],
  ])('shows the problem+json error %s', async (type, status, message) => {
    const user = userEvent.setup();
    setup('/admin/login', { 'POST /api/admin/v1/auth/login': () => problem(status, type) });
    await user.type(await screen.findByLabelText('Email'), 'gestore@example.it');
    await user.type(screen.getByLabelText('Password'), 'sbagliata');
    await user.click(screen.getByRole('button', { name: 'Accedi' }));
    expect(await screen.findByRole('alert')).toHaveTextContent(message);
  });

  it('asks for the TOTP code when required', async () => {
    const user = userEvent.setup();
    setup('/admin/login', { 'POST /api/admin/v1/auth/login': () => problem(401, 'totp_required') });
    await user.type(await screen.findByLabelText('Email'), 'gestore@example.it');
    await user.type(screen.getByLabelText('Password'), 'password');
    await user.click(screen.getByRole('button', { name: 'Accedi' }));
    expect(await screen.findByLabelText('Codice di verifica')).toBeInTheDocument();
  });

  it('logs in and returns to the requested page', async () => {
    const user = userEvent.setup();
    let loggedIn = false;
    const { history, fetchMock } = setup('/admin/cassa', {
      'GET /api/admin/v1/auth/me': () => (loggedIn ? json(200, ME) : problem(401, 'unauthorized')),
      'POST /api/admin/v1/auth/login': async (req) => {
        expect(await req.json()).toEqual({ email: 'gestore@example.it', password: 'segreta' });
        loggedIn = true;
        return json(200, ME);
      },
    });
    await user.type(await screen.findByLabelText('Email'), 'gestore@example.it');
    await user.type(screen.getByLabelText('Password'), 'segreta');
    await user.click(screen.getByRole('button', { name: 'Accedi' }));
    expect(await screen.findByRole('heading', { name: 'Cassa', level: 1 })).toBeInTheDocument();
    expect(history.location.pathname).toBe('/admin/cassa');
    expect(screen.getByText('Disponibile dalla milestone M3')).toBeInTheDocument();
    const loginCall = fetchMock.mock.calls.find(([r]) => r.method === 'POST');
    expect(loginCall?.[0].headers.get('X-CSRF-Token')).toBe('csrf');
  });

  it('renders the navigation of spec 15.1 and the establishments list', async () => {
    setup('/admin/configurazione', {
      'GET /api/admin/v1/auth/me': () => json(200, ME),
      'GET /api/admin/v1/establishments': () => json(200, { items: [ESTABLISHMENT] }),
    });
    const nav = await screen.findByRole('navigation', { name: 'Navigazione principale' });
    const labels = within(nav)
      .getAllByRole('link')
      .map((a) => a.textContent);
    expect(labels).toEqual([
      'Oggi',
      'Mappa',
      'Prenotazioni',
      'Calendario',
      'Clienti',
      'Abbonamenti',
      'Cassa',
      'Report',
      'Configurazione',
    ]);
    expect(await screen.findByRole('rowheader', { name: 'Lido Demo' })).toBeInTheDocument();
    expect(
      screen.getByRole('link', { name: 'Apri la pagina pubblica di Lido Demo' }),
    ).toHaveAttribute('href', '/b/lido-demo');
    expect(within(nav).getByRole('link', { name: 'Configurazione' })).toHaveAttribute(
      'aria-current',
      'page',
    );
    expect(screen.getByRole('status')).toHaveTextContent('Online');
  });

  it('logs out and goes back to login', async () => {
    const user = userEvent.setup();
    let loggedIn = true;
    const { history } = setup('/admin/oggi', {
      'GET /api/admin/v1/auth/me': () => (loggedIn ? json(200, ME) : problem(401, 'unauthorized')),
      'POST /api/admin/v1/auth/logout': () => {
        loggedIn = false;
        return new Response(null, { status: 204 });
      },
    });
    await user.click(await screen.findByRole('button', { name: 'Esci' }));
    await waitFor(() => expect(history.location.pathname).toBe('/admin/login'));
  });

  it('shows the demo map with accessible spots', async () => {
    setup('/admin/mappa', { 'GET /api/admin/v1/auth/me': () => json(200, ME) });
    expect(await screen.findByRole('group', { name: 'Mappa dimostrativa' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /^Postazione A1:/ })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Adatta alla vista' })).toBeInTheDocument();
  });
});

describe('safeRedirect', () => {
  it.each([
    ['/cassa', '/cassa'],
    ['//evil.example', undefined],
    ['https://evil.example', undefined],
    ['/login', undefined],
    [42, undefined],
  ])('%j -> %j', (input, expected) => {
    expect(safeRedirect(input)).toBe(expected);
  });
});
