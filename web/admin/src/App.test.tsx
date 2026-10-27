import { createApiClient } from '@arenile/shared/api';
import { createMemoryHistory } from '@tanstack/react-router';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { App } from './App';
import { createAdminTestI18n } from './i18n';
import { createQueryClient } from './lib/queryClient';
import { safeRedirect } from './lib/redirect';

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
    <App
      api={api}
      i18n={createAdminTestI18n()}
      queryClient={createQueryClient()}
      history={history}
    />,
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
      'GET /api/admin/v1/establishments': () => json(200, { items: [ESTABLISHMENT] }),
      'POST /api/admin/v1/auth/login': async (req) => {
        expect(await req.json()).toEqual({ email: 'gestore@example.it', password: 'segreta' });
        loggedIn = true;
        return json(200, ME);
      },
    });
    await user.type(await screen.findByLabelText('Email'), 'gestore@example.it');
    await user.type(screen.getByLabelText('Password'), 'segreta');
    await user.click(screen.getByRole('button', { name: 'Accedi' }));
    // The legacy section path lands on the same section of the current establishment.
    expect(await screen.findByRole('heading', { name: 'Cassa', level: 1 })).toBeInTheDocument();
    await waitFor(() => expect(history.location.pathname).toBe('/admin/e/e1/cassa'));
    const loginCall = fetchMock.mock.calls.find(([r]) => r.method === 'POST');
    expect(loginCall?.[0].headers.get('X-CSRF-Token')).toBe('csrf');
  });

  it('renders the navigation of spec 15.1 and the establishments list', async () => {
    setup('/admin/configurazione', {
      'GET /api/admin/v1/auth/me': () => json(200, ME),
      'GET /api/admin/v1/establishments': () => json(200, { items: [ESTABLISHMENT] }),
    });
    const nav = await screen.findByRole('navigation', { name: 'Navigazione principale' });
    const labels = () =>
      within(nav)
        .getAllByRole('link')
        .map((a) => a.textContent);
    // Every section is delivered (Report since M5A) and links to the current
    // establishment; the "In arrivo" group is hidden when empty.
    const operations = [
      'Oggi',
      'Mappa',
      'Prenotazioni',
      'Calendario',
      'Clienti',
      'Partner e convenzioni',
      'Abbonamenti',
      'Rinnovi',
      "Lista d'attesa",
      'Cassa',
      'Vendita rapida',
      'Ticket del giorno',
      'Report',
    ];
    await waitFor(() =>
      expect(within(nav).getByRole('link', { name: 'Cassa' })).toHaveAttribute(
        'href',
        '/admin/e/e1/cassa',
      ),
    );
    expect(labels()).toEqual(operations);
    expect(within(nav).getByRole('link', { name: 'Cassa' })).not.toHaveAttribute('aria-disabled');
    expect(within(nav).queryByRole('button', { name: /In arrivo/ })).not.toBeInTheDocument();
    expect(within(nav).getByRole('link', { name: 'Report' })).toHaveAttribute(
      'href',
      '/admin/e/e1/report',
    );
    expect(await screen.findByRole('cell', { name: 'Lido Demo' })).toBeInTheDocument();
    expect(
      screen.getByRole('link', { name: /^\/b\/lido-demo\s*Apri la pagina pubblica di Lido Demo$/ }),
    ).toHaveAttribute('href', '/b/lido-demo');
    const config = screen.getByRole('navigation', { name: 'Configurazione dello stabilimento' });
    expect(within(config).getByRole('link', { name: 'Tutti gli stabilimenti' })).toHaveAttribute(
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
    await user.click(await screen.findByRole('button', { name: /Menu utente/ }));
    await user.click(await screen.findByRole('menuitem', { name: 'Esci' }));
    await waitFor(() => expect(history.location.pathname).toBe('/admin/login'));
  });

  it('goes back to login when the session expires on a page, then returns to it', async () => {
    const user = userEvent.setup();
    let sessionValid = true;
    const { history } = setup('/admin/configurazione', {
      // The guard sees a valid session: it expires before the page data loads.
      'GET /api/admin/v1/auth/me': () => {
        const res = sessionValid ? json(200, ME) : problem(401, 'unauthorized');
        sessionValid = false;
        return res;
      },
      'GET /api/admin/v1/establishments': () =>
        sessionValid ? json(200, { items: [ESTABLISHMENT] }) : problem(401, 'unauthorized'),
      'POST /api/admin/v1/auth/login': () => {
        sessionValid = true;
        return json(200, ME);
      },
    });
    expect(await screen.findByRole('heading', { name: 'Accedi al pannello' })).toBeInTheDocument();
    expect(history.location.pathname).toBe('/admin/login');
    expect(history.location.search).toBe('?redirect=%2Fconfigurazione');
    await user.type(screen.getByLabelText('Email'), 'gestore@example.it');
    await user.type(screen.getByLabelText('Password'), 'segreta');
    await user.click(screen.getByRole('button', { name: 'Accedi' }));
    expect(await screen.findByRole('cell', { name: 'Lido Demo' })).toBeInTheDocument();
    expect(history.location.pathname).toBe('/admin/configurazione');
  });

  it('redirects a legacy section to the current establishment', async () => {
    const { history } = setup('/admin/mappa', {
      'GET /api/admin/v1/auth/me': () => json(200, ME),
      'GET /api/admin/v1/establishments': () => json(200, { items: [ESTABLISHMENT] }),
    });
    await waitFor(() => expect(history.location.pathname).toBe('/admin/e/e1/mappa'));
    expect(await screen.findByRole('heading', { name: 'Mappa', level: 1 })).toBeInTheDocument();
  });

  it('sends a legacy section to the establishments list when there is none', async () => {
    const { history } = setup('/admin/oggi', {
      'GET /api/admin/v1/auth/me': () => json(200, ME),
      'GET /api/admin/v1/establishments': () => json(200, { items: [] }),
    });
    await waitFor(() => expect(history.location.pathname).toBe('/admin/configurazione'));
  });
});

const OWNER_ME = {
  ...ME,
  memberships: [{ organization_id: 'o1', role: 'owner', permissions: [] }],
};

/** Opens the establishment switcher once the list is loaded. */
async function openSwitcher(user: ReturnType<typeof userEvent.setup>) {
  const trigger = await screen.findByRole('button', { name: /Stabilimento:\s*Lido/ });
  await user.click(trigger);
}

describe('establishment-scoped configuration (M1/M2 scaffolding)', () => {
  const routes = {
    'GET /api/admin/v1/auth/me': () => json(200, OWNER_ME),
    'GET /api/admin/v1/establishments': () =>
      json(200, { items: [ESTABLISHMENT, { ...ESTABLISHMENT, id: 'e2', name: 'Lido Due' }] }),
  };

  it.each([
    ['/admin/e/e1/config/establishment', 'Stabilimento'],
    ['/admin/e/e1/config/seasons', 'Stagioni e periodi'],
    ['/admin/e/e1/config/closures', 'Chiusure'],
    ['/admin/e/e1/config/zones', 'Zone'],
    ['/admin/e/e1/config/pools', 'Pool'],
    ['/admin/e/e1/config/catalog', 'Catalogo'],
    ['/admin/e/e1/config/team', 'Utenti'],
    ['/admin/e/e1/maps', 'Mappe'],
    ['/admin/e/e1/maps/m1', 'Editor della mappa'],
    ['/admin/e/e1/setup', 'Configurazione guidata'],
    ['/admin/e/e1/pricing', 'Prezzi'],
    ['/admin/e/e1/pricing/simulator', 'Simulatore prezzi'],
    ['/admin/e/e1/pricing/matrix', 'Matrice di listino'],
    ['/admin/e/e1/pricing/coverage', 'Verifica di copertura'],
    ['/admin/e/e1/policies', 'Policy di prenotazione'],
    ['/admin/e/e1/oggi', 'Oggi'],
    ['/admin/e/e1/mappa', 'Mappa'],
    ['/admin/e/e1/prenotazioni', 'Prenotazioni'],
    ['/admin/e/e1/prenotazioni/b1', 'Prenotazione'],
    ['/admin/e/e1/calendario', 'Calendario'],
    ['/admin/e/e1/clienti', 'Clienti'],
    ['/admin/e/e1/clienti/c1', 'Cliente'],
    ['/admin/e/e1/abbonamenti', 'Abbonamenti'],
    ['/admin/e/e1/cassa', 'Cassa'],
    ['/admin/e/e1/cassa/corrispettivi', 'Corrispettivi'],
    ['/admin/e/e1/vendita-rapida', 'Vendita rapida'],
    ['/admin/e/e1/ticket', 'Ticket del giorno'],
    ['/admin/e/e1/ticket/varco', 'Varco'],
    ['/admin/e/e1/config/entry-tickets', "Biglietti d'ingresso"],
    ['/admin/e/e1/report', 'Report'],
    ['/admin/e/e1/config/api-keys', 'Chiavi API'],
    ['/admin/e/e1/config/webhooks', 'Webhook'],
    ['/admin/establishments/new', 'Nuovo stabilimento'],
  ])('%s renders its feature page', async (path, title) => {
    setup(path, routes);
    expect(await screen.findByRole('heading', { name: title, level: 1 })).toBeInTheDocument();
  });

  it('redirects /e/:id to the establishment settings', async () => {
    const { history } = setup('/admin/e/e1', routes);
    await waitFor(() => expect(history.location.pathname).toBe('/admin/e/e1/config/establishment'));
  });

  it('shows the configuration menu of the current establishment', async () => {
    setup('/admin/e/e2/config/seasons', routes);
    const nav = await screen.findByRole('navigation', {
      name: 'Configurazione dello stabilimento',
    });
    expect(await within(nav).findByText('Lido Due')).toBeInTheDocument();
    expect(within(nav).getByRole('link', { name: 'Stagioni e periodi' })).toHaveAttribute(
      'href',
      '/admin/e/e2/config/seasons',
    );
    expect(within(nav).getByRole('link', { name: 'Utenti' })).toBeInTheDocument();
  });

  it('hides owner-only entries from managers', async () => {
    setup('/admin/e/e1/config/seasons', {
      ...routes,
      'GET /api/admin/v1/auth/me': () =>
        json(200, {
          ...ME,
          memberships: [{ organization_id: 'o1', role: 'manager', permissions: [] }],
        }),
    });
    const nav = await screen.findByRole('navigation', {
      name: 'Configurazione dello stabilimento',
    });
    expect(within(nav).queryByRole('link', { name: 'Utenti' })).not.toBeInTheDocument();
    await userEvent.setup().click(screen.getByRole('button', { name: /Stabilimento:/ }));
    await screen.findByRole('menuitemradio', { name: 'Lido Demo' });
    expect(screen.queryByRole('menuitem', { name: 'Nuovo stabilimento' })).not.toBeInTheDocument();
  });

  it('switches establishment keeping the section', async () => {
    const user = userEvent.setup();
    const { history } = setup('/admin/e/e1/pricing', routes);
    await openSwitcher(user);
    await user.click(await screen.findByRole('menuitemradio', { name: 'Lido Due' }));
    await waitFor(() => expect(history.location.pathname).toBe('/admin/e/e2/pricing'));
  });

  it('switches from a map document to the maps list', async () => {
    const user = userEvent.setup();
    const { history } = setup('/admin/e/e1/maps/m1', routes);
    await openSwitcher(user);
    await user.click(await screen.findByRole('menuitemradio', { name: 'Lido Due' }));
    await waitFor(() => expect(history.location.pathname).toBe('/admin/e/e2/maps'));
  });

  it('opens the new establishment page from the switcher (owners)', async () => {
    const user = userEvent.setup();
    const { history } = setup('/admin/oggi', routes);
    await openSwitcher(user);
    await user.click(await screen.findByRole('menuitem', { name: 'Nuovo stabilimento' }));
    await waitFor(() => expect(history.location.pathname).toBe('/admin/establishments/new'));
  });

  it('offers the first establishment from the empty list (owners), with no placeholder left', async () => {
    const user = userEvent.setup();
    const { history } = setup('/admin/configurazione', {
      ...routes,
      'GET /api/admin/v1/establishments': () => json(200, { items: [] }),
    });
    expect(await screen.findByText('Nessuno stabilimento configurato.')).toBeInTheDocument();
    expect(screen.queryByText('Sezione in preparazione')).not.toBeInTheDocument();
    await user.click(screen.getByRole('link', { name: 'Nuovo stabilimento' }));
    await waitFor(() => expect(history.location.pathname).toBe('/admin/establishments/new'));
  });

  it('switches from a booking detail to the bookings list', async () => {
    const user = userEvent.setup();
    const { history } = setup('/admin/e/e1/prenotazioni/b1', routes);
    await openSwitcher(user);
    await user.click(await screen.findByRole('menuitemradio', { name: 'Lido Due' }));
    await waitFor(() => expect(history.location.pathname).toBe('/admin/e/e2/prenotazioni'));
  });

  it('goes to the establishment settings when switching from a global page', async () => {
    const user = userEvent.setup();
    const { history } = setup('/admin/configurazione', routes);
    await openSwitcher(user);
    await user.click(await screen.findByRole('menuitemradio', { name: 'Lido Due' }));
    await waitFor(() => expect(history.location.pathname).toBe('/admin/e/e2/config/establishment'));
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
