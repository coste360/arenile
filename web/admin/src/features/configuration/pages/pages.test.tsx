import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import {
  EST_ID,
  ORG_ID,
  baseRoutes,
  callsTo,
  establishment,
  json,
  list,
  problem,
  renderApp,
} from '../testing';

const API = `/api/admin/v1/establishments/${EST_ID}`;
const ts = { created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' };

const SEASON = {
  id: 's1',
  establishment_id: EST_ID,
  year: 2027,
  opens_on: '2027-05-01',
  closes_on: '2027-09-30',
  status: 'active',
  version: 2,
  ...ts,
};
const PERIOD_HIGH = {
  id: 'rp-high',
  season_id: 's1',
  code: 'high',
  name: { it: 'Alta stagione' },
  color: '#fb923c',
  ranges: [{ from: '2027-07-01', to: '2027-08-31' }],
  sort_order: 2,
  version: 1,
  ...ts,
};
const ZONE = {
  id: 'z1',
  map_id: 'm1',
  code: 'row1',
  name: { it: 'Prima fila', en: 'First row' },
  assignment_mode: 'seat',
  online_visible: true,
  online_quota: { type: 'percent', value: 70 },
  price_category: 'row1',
  auto_assign: false,
  sort_order: 0,
  spot_count: 15,
  version: 4,
  ...ts,
};

describe('SeasonsPage', () => {
  const routes = (role: 'manager' | 'cashier') => ({
    ...baseRoutes(role),
    [`GET ${API}/seasons`]: () => list([SEASON]),
    [`GET ${API}/rate-periods`]: () => list([PERIOD_HIGH]),
  });

  it('lists seasons and the rate periods on a timeline', async () => {
    renderApp(`/admin/e/${EST_ID}/config/seasons`, routes('manager'));
    expect(await screen.findByRole('button', { name: '2027' })).toHaveAttribute(
      'aria-pressed',
      'true',
    );
    expect(screen.getByText('Attiva')).toBeInTheDocument();
    const timeline = await screen.findByRole('list', {
      name: 'Calendario dei periodi della stagione',
    });
    expect(
      within(timeline).getByRole('listitem', { name: /Alta stagione dal/ }),
    ).toBeInTheDocument();
    expect(screen.getByText(/giorni della stagione non sono coperti/)).toBeInTheDocument();
  });

  it('blocks overlapping rate periods client-side, then shows the server message', async () => {
    const user = userEvent.setup();
    const { fetchMock } = renderApp(`/admin/e/${EST_ID}/config/seasons`, {
      ...routes('manager'),
      [`POST ${API}/rate-periods`]: () =>
        problem(409, 'conflict', { detail: 'Il periodo si sovrappone a "peak".' }),
    });
    await user.click(await screen.findByRole('button', { name: 'Nuovo periodo' }));
    const dialog = await screen.findByRole('dialog');
    await user.type(within(dialog).getByLabelText('Nome'), 'Bassa');
    // Default range = whole season: overlaps "high".
    await user.click(within(dialog).getByRole('button', { name: 'Salva' }));
    expect(
      await within(dialog).findByText(
        "L'intervallo si sovrappone a un altro periodo della stagione",
      ),
    ).toBeInTheDocument();
    expect(callsTo(fetchMock, 'POST', `${API}/rate-periods`)).toHaveLength(0);

    // Shorten the range in the shared DateRangePicker: 1 May – 30 June.
    await user.click(
      within(dialog).getByRole('button', { name: 'Intervallo 1: 01/05/2027 – 30/09/2027' }),
    );
    // The first click starts a new range, the second one ends it.
    await user.click(await screen.findByRole('button', { name: /\b1 maggio 2027/i }));
    await user.click(await screen.findByRole('button', { name: /30 giugno 2027/i }));
    await user.keyboard('{Escape}');
    await user.click(within(dialog).getByRole('button', { name: 'Salva' }));
    expect(
      await within(dialog).findByText('Il periodo si sovrappone a "peak".'),
    ).toBeInTheDocument();
    const [post] = callsTo(fetchMock, 'POST', `${API}/rate-periods`);
    expect(post?.headers.get('Idempotency-Key')).toBeTruthy();
  });

  it('moves the dates of a new season with its year, not those of an existing one', async () => {
    const user = userEvent.setup();
    let posted: unknown;
    const { fetchMock } = renderApp(`/admin/e/${EST_ID}/config/seasons`, {
      ...routes('manager'),
      [`POST ${API}/seasons`]: async (req) => {
        posted = await req.json();
        return json(201, { ...SEASON, ...(posted as object), id: 's2', status: 'draft' });
      },
    });
    await user.click(await screen.findByRole('button', { name: 'Nuova stagione' }));
    let dialog = await screen.findByRole('dialog');
    await user.clear(within(dialog).getByLabelText('Anno'));
    await user.type(within(dialog).getByLabelText('Anno'), '2031');
    expect(
      within(dialog).getByRole('button', { name: /01\/05\/2031 – 30\/09\/2031/ }),
    ).toBeVisible();
    await user.click(within(dialog).getByRole('button', { name: 'Salva' }));
    await waitFor(() => expect(callsTo(fetchMock, 'POST', `${API}/seasons`)).toHaveLength(1));
    expect(posted).toMatchObject({
      year: 2031,
      opens_on: '2031-05-01',
      closes_on: '2031-09-30',
    });
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());

    await user.click(screen.getByRole('button', { name: 'Modifica la stagione 2027' }));
    dialog = await screen.findByRole('dialog');
    await user.clear(within(dialog).getByLabelText('Anno'));
    await user.type(within(dialog).getByLabelText('Anno'), '2028');
    // Kept, and flagged: the season must open in its year.
    expect(
      within(dialog).getByRole('button', { name: /01\/05\/2027 – 30\/09\/2027/ }),
    ).toBeVisible();
    await user.click(within(dialog).getByRole('button', { name: 'Salva' }));
    expect(
      await within(dialog).findByText("La stagione deve aprire nell'anno indicato"),
    ).toBeInTheDocument();
  });

  it('hides editing for a cashier', async () => {
    renderApp(`/admin/e/${EST_ID}/config/seasons`, routes('cashier'));
    await screen.findByRole('button', { name: '2027' });
    expect(screen.queryByRole('button', { name: 'Nuova stagione' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Nuovo periodo' })).toBeNull();
    expect(screen.queryByRole('button', { name: /Elimina/ })).toBeNull();
  });
});

describe('ZonesPage', () => {
  const routes = {
    ...baseRoutes('manager'),
    [`GET ${API}/zones`]: () => list([ZONE]),
    [`GET ${API}/maps`]: () =>
      list([
        {
          id: 'm1',
          name: { it: 'Spiaggia' },
          width: 1000,
          height: 600,
          grid_size: 10,
          sea_side: 'top',
          sort_order: 0,
          spot_count: 15,
          version: 1,
          ...ts,
        },
      ]),
  };

  it('replaces a zone with PUT and If-Match of the version it was opened with', async () => {
    const user = userEvent.setup();
    const bodies: unknown[] = [];
    const { fetchMock } = renderApp(`/admin/e/${EST_ID}/config/zones`, {
      ...routes,
      [`PUT ${API}/zones/z1`]: async (req) => {
        bodies.push(await req.json());
        return json(200, { ...ZONE, version: 5 });
      },
    });
    expect(await screen.findByText('Online 70%')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Modifica la zona Prima fila' }));
    const dialog = await screen.findByRole('dialog');
    const value = within(dialog).getByLabelText('Valore della quota');
    await user.clear(value);
    await user.type(value, '50');
    await user.click(within(dialog).getByRole('button', { name: 'Salva' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    const [put] = callsTo(fetchMock, 'PUT', `${API}/zones/z1`);
    expect(put?.headers.get('If-Match')).toBe('"4"');
    expect(bodies[0]).toEqual({
      map_id: 'm1',
      code: 'row1',
      name: { it: 'Prima fila', en: 'First row' },
      assignment_mode: 'seat',
      online_visible: true,
      online_quota: { type: 'percent', value: 50 },
      price_category: 'row1',
      auto_assign: false,
      sort_order: 0,
    });
  });

  it('shows the reload prompt on 412', async () => {
    const user = userEvent.setup();
    renderApp(`/admin/e/${EST_ID}/config/zones`, {
      ...routes,
      [`PUT ${API}/zones/z1`]: () => problem(412, 'version_conflict', { current_version: 9 }),
    });
    await user.click(await screen.findByRole('button', { name: 'Modifica la zona Prima fila' }));
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: 'Salva' }));
    const alert = await within(dialog).findByRole('alert');
    expect(alert).toHaveTextContent('Un altro utente ha modificato questi dati');
    await user.click(within(alert).getByRole('button', { name: 'Ricarica i dati aggiornati' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
  });
});

describe('PoolsPage', () => {
  it('system-managed pools are read-only', async () => {
    renderApp(`/admin/e/${EST_ID}/config/pools`, {
      ...baseRoutes('owner'),
      [`GET ${API}/pools`]: () =>
        list([
          {
            id: 'p1',
            zone_id: 'z2',
            component_code: 'umbrella',
            name: { it: 'Zona B' },
            capacity: 30,
            capacity_overrides: [],
            system_managed: true,
            version: 1,
            ...ts,
          },
          {
            id: 'p2',
            component_code: 'sunbed',
            name: { it: 'Lettini' },
            capacity: 40,
            capacity_overrides: [],
            system_managed: false,
            version: 1,
            ...ts,
          },
        ]),
      [`GET ${API}/zones`]: () => list([]),
      [`GET ${API}/components`]: () => list([]),
    });
    expect(await screen.findByText('Gestito dal sistema')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Modifica il pool Zona B' })).toBeNull();
    expect(screen.getByRole('button', { name: 'Modifica il pool Lettini' })).toBeInTheDocument();
  });
});

describe('CatalogPage', () => {
  it('builds a pack composition from the component buttons', async () => {
    const user = userEvent.setup();
    const bodies: unknown[] = [];
    renderApp(`/admin/e/${EST_ID}/config/catalog`, {
      ...baseRoutes('owner'),
      [`GET ${API}/packs`]: () => list([]),
      [`GET ${API}/zones`]: () => list([]),
      [`GET ${API}/components`]: () =>
        list([
          {
            id: 'c1',
            code: 'umbrella',
            name: { it: 'Ombrellone' },
            tracked: false,
            version: 1,
            ...ts,
          },
          { id: 'c2', code: 'sunbed', name: { it: 'Lettino' }, tracked: false, version: 1, ...ts },
        ]),
      [`POST ${API}/packs`]: async (req) => {
        bodies.push(await req.json());
        return json(201, { id: 'p1', version: 1 });
      },
    });
    expect(await screen.findByRole('tab', { name: 'Pack' })).toHaveAttribute(
      'aria-selected',
      'true',
    );
    await user.click(await screen.findByRole('button', { name: 'Nuovo pack' }));
    const dialog = await screen.findByRole('dialog');
    await user.type(within(dialog).getByLabelText('Codice'), 'base');
    await user.type(within(dialog).getByLabelText('Nome'), 'Base');
    await user.click(within(dialog).getByRole('button', { name: 'Ombrellone' }));
    await user.click(within(dialog).getByRole('button', { name: 'Lettino' }));
    const qty = within(dialog).getByLabelText('Quantità di Lettino');
    await user.clear(qty);
    await user.type(qty, '2');
    await user.click(within(dialog).getByRole('button', { name: 'Salva' }));
    await waitFor(() => expect(bodies).toHaveLength(1));
    expect(bodies[0]).toMatchObject({
      code: 'base',
      name: { it: 'Base' },
      composition: { umbrella: 1, sunbed: 2 },
      allowed_formulas: ['half_day', 'day', 'multi_day'],
      channels: ['desk', 'online'],
      active: true,
    });
  });
});

describe('TeamPage (owner only)', () => {
  const MEMBER = {
    user_id: 'u2',
    email: 'cassa@lido.it',
    name: 'Anna Cassa',
    locale: 'it',
    active: true,
    totp_enabled: false,
    role: 'cashier',
    scope: 'establishment',
    memberships: [],
  };

  it('is hidden for a manager', async () => {
    const { fetchMock } = renderApp(`/admin/e/${EST_ID}/config/team`, baseRoutes('manager'));
    expect(
      await screen.findByText('La gestione di utenti e ruoli è riservata ai proprietari.'),
    ).toBeInTheDocument();
    expect(callsTo(fetchMock, 'GET', `${API}/team`)).toHaveLength(0);
    expect(screen.queryByRole('link', { name: 'Utenti' })).toBeNull();
  });

  it('lets the owner invite, change role and deactivate', async () => {
    const user = userEvent.setup();
    const bodies: Record<string, unknown[]> = { post: [], patch: [] };
    renderApp(`/admin/e/${EST_ID}/config/team`, {
      ...baseRoutes('owner'),
      [`GET ${API}/team`]: () => list([MEMBER]),
      [`POST ${API}/team`]: async (req) => {
        bodies.post?.push(await req.json());
        return json(201, {
          member: { ...MEMBER, user_id: 'u3', name: 'Luca' },
          user_created: true,
          initial_password: 'Xy7-secret-pass',
        });
      },
      [`PATCH ${API}/team/u2`]: async (req) => {
        bodies.patch?.push(await req.json());
        return json(200, MEMBER);
      },
    });
    await user.click(await screen.findByRole('combobox', { name: 'Ruolo di Anna Cassa' }));
    await user.click(await screen.findByRole('option', { name: 'Gestore' }));
    await waitFor(() => expect(bodies.patch).toHaveLength(1));
    await user.click(screen.getByRole('button', { name: 'Disattiva Anna Cassa' }));
    await waitFor(() => expect(bodies.patch).toHaveLength(2));
    expect(bodies.patch).toEqual([{ role: 'manager', scope: 'establishment' }, { active: false }]);

    await user.click(screen.getByRole('button', { name: 'Invita utente' }));
    const dialog = await screen.findByRole('dialog');
    await user.type(within(dialog).getByLabelText('Email'), 'luca@lido.it');
    await user.type(within(dialog).getByLabelText('Nome'), 'Luca');
    await user.click(within(dialog).getByRole('button', { name: 'Aggiungi utente' }));
    expect(await screen.findByText('Xy7-secret-pass')).toBeInTheDocument();
    expect(bodies.post?.[0]).toEqual({
      email: 'luca@lido.it',
      name: 'Luca',
      role: 'cashier',
      scope: 'establishment',
      locale: 'it',
    });
  });
});

describe('NewEstablishmentPage', () => {
  it('creates the establishment with an auto slug and opens the wizard', async () => {
    const user = userEvent.setup();
    const bodies: unknown[] = [];
    const created = establishment({ id: 'e-new', slug: 'lido-sole', name: 'Lido Sole' });
    const { history, fetchMock } = renderApp('/admin/establishments/new', {
      ...baseRoutes('owner'),
      'POST /api/admin/v1/establishments': async (req) => {
        bodies.push(await req.json());
        return json(201, created);
      },
      'GET /api/admin/v1/establishments/e-new': () => json(200, created),
      'GET /api/admin/v1/establishments/e-new/setup/status': () =>
        json(200, {
          has_season: false,
          has_rate_periods: false,
          has_map: false,
          spot_count: 0,
          has_catalog: false,
          has_prices: false,
          booking_enabled: false,
        }),
    });
    await user.type(await screen.findByLabelText('Nome'), 'Lido Sole');
    expect(screen.getByLabelText('Indirizzo pubblico')).toHaveValue('lido-sole');
    await user.click(screen.getByRole('button', { name: 'Crea stabilimento' }));
    await waitFor(() => expect(history.location.pathname).toBe('/admin/e/e-new/setup'));
    expect(bodies[0]).toMatchObject({
      organization_id: ORG_ID,
      name: 'Lido Sole',
      slug: 'lido-sole',
      currency: 'EUR',
    });
    expect(
      callsTo(fetchMock, 'POST', '/api/admin/v1/establishments')[0]?.headers.get('Idempotency-Key'),
    ).toBeTruthy();
  });

  it('is refused to non-owners', async () => {
    renderApp('/admin/establishments/new', baseRoutes('manager'));
    expect(
      await screen.findByText('Solo i proprietari possono creare nuovi stabilimenti.'),
    ).toBeInTheDocument();
  });
});

describe('ClosuresPage', () => {
  it('creates one closure per day picked in the multi-select calendar', async () => {
    const user = userEvent.setup();
    const bodies: unknown[] = [];
    const { fetchMock } = renderApp(`/admin/e/${EST_ID}/config/closures`, {
      ...baseRoutes('manager'),
      [`GET ${API}/closure-days`]: () => list([]),
      [`POST ${API}/closure-days`]: async (req) => {
        const body = (await req.json()) as Record<string, unknown>;
        bodies.push(body);
        return json(201, { id: `c${bodies.length}`, version: 1, ...body, ...ts });
      },
    });
    await user.click(await screen.findByRole('button', { name: 'Nuova chiusura' }));
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: / 10 \p{L}+ \d{4}$/iu }));
    await user.click(within(dialog).getByRole('button', { name: / 12 \p{L}+ \d{4}$/iu }));
    await user.click(within(dialog).getByLabelText('Mattina'));
    await user.click(within(dialog).getByRole('button', { name: 'Crea 2 chiusure' }));
    await waitFor(() => expect(bodies).toHaveLength(2));
    expect(bodies[0]).toMatchObject({ slots: ['PM'] });
    expect((bodies[0] as { date: string }).date < (bodies[1] as { date: string }).date).toBe(true);
    expect(
      callsTo(fetchMock, 'POST', `${API}/closure-days`)[0]?.headers.get('Idempotency-Key'),
    ).toBeTruthy();
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
  });
});
