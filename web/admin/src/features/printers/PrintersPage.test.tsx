import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import {
  EST_ID,
  baseRoutes,
  callsTo,
  json,
  list,
  problem,
  renderApp,
} from '../configuration/testing';

const API = `/api/admin/v1/establishments/${EST_ID}`;
const PATH = `/admin/e/${EST_ID}/config/printers`;

function meta(edition: 'server' | 'desktop') {
  return json(200, {
    product: 'arenile',
    version: 'test',
    api_version: '1',
    edition,
    ...(edition === 'desktop' ? { desktop: { mode: 'local' } } : {}),
    server_time: '2027-06-01T08:00:00Z',
    min_client_versions: { mobile: '1.0.0', desktop: '1.0.0' },
    capabilities: {
      online_booking: edition === 'server',
      online_payments: edition === 'server',
      payment_providers: ['manual'],
      fiscal_drivers: [],
      device_pairing: edition === 'server',
      fiscal_bridge: false,
      realtime: true,
      push_notifications: false,
    },
    features: [],
  });
}

function printer(overrides: Record<string, unknown> = {}) {
  return {
    id: 'p1',
    name: 'Cassa bar',
    driver: 'network',
    host: '192.168.1.50',
    port: 9100,
    paper_width_mm: 80,
    chars_per_line: 48,
    codepage: 'cp858',
    cut_mode: 'partial',
    feed_lines: 4,
    print_logo: true,
    logo_cached: false,
    drawer_pin: 2,
    active: true,
    status: 'reachable',
    last_checked_at: '2027-06-01T08:00:00Z',
    version: 1,
    created_at: '2027-06-01T07:00:00Z',
    updated_at: '2027-06-01T07:00:00Z',
    ...overrides,
  };
}

function job(overrides: Record<string, unknown> = {}) {
  return {
    id: 'j1',
    printer_id: 'p1',
    printer_name: 'Cassa bar',
    document_kind: 'test_page',
    copies: 1,
    status: 'queued',
    attempts: 0,
    max_attempts: 5,
    created_at: '2027-06-01T08:01:00Z',
    updated_at: '2027-06-01T08:01:00Z',
    ...overrides,
  };
}

function desktopRoutes(printers: unknown[]) {
  return {
    ...baseRoutes('owner'),
    'GET /api/admin/v1/meta': () => meta('desktop'),
    [`GET ${API}/printers`]: () => json(200, { items: printers, escpos_available: true }),
    [`GET ${API}/print-routes`]: () => json(200, { items: [] }, { ETag: '"r1"' }),
    [`GET ${API}/print-jobs`]: () => list([]),
    [`GET ${API}/cash-registers`]: () => list([]),
  };
}

describe('PrintersPage', () => {
  it('explains on the server edition that ESC/POS needs the desktop app', async () => {
    renderApp(PATH, {
      ...baseRoutes('owner'),
      'GET /api/admin/v1/meta': () => meta('server'),
      [`GET ${API}/printers`]: () =>
        json(200, { items: [], escpos_available: false, unavailable_reason: 'server_edition' }),
    });
    expect(
      await screen.findByText("Stampa ESC/POS disponibile con l'app desktop"),
    ).toBeInTheDocument();
    expect(screen.getByText(/la chiusura di cassa si stampa dal browser/)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Aggiungi stampante' })).toBeNull();
    expect(screen.queryByText('Instradamento')).toBeNull();
  });

  it('checks a printer and sends the test print', async () => {
    const user = userEvent.setup();
    const { fetchMock } = renderApp(PATH, {
      ...desktopRoutes([printer()]),
      [`POST ${API}/printers/p1/check`]: () =>
        json(200, { status: 'reachable', latency_ms: 12, checked_at: '2027-06-01T08:02:00Z' }),
      [`POST ${API}/printers/p1/test-print`]: () => json(202, job()),
    });
    expect(await screen.findByText('Cassa bar')).toBeInTheDocument();
    await screen.findByText('Ancora nessuna stampa');
    expect(screen.getByText('Raggiungibile')).toBeInTheDocument();
    expect(screen.getByText('192.168.1.50')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Verifica la connessione con Cassa bar' }));
    expect(await screen.findByText('Cassa bar è raggiungibile')).toBeInTheDocument();
    expect(screen.getByText('Risposta in 12 ms')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Stampa di prova su Cassa bar' }));
    expect(await screen.findByText('Stampa di prova inviata a Cassa bar')).toBeInTheDocument();
    expect(callsTo(fetchMock, 'POST', `${API}/printers/p1/test-print`)).toHaveLength(1);
  });

  it('refuses a public address and saves a LAN printer with the defaults', async () => {
    const user = userEvent.setup();
    const posts: unknown[] = [];
    const { fetchMock } = renderApp(PATH, {
      ...desktopRoutes([]),
      [`POST ${API}/printers`]: async (req) => {
        posts.push(await req.json());
        return json(201, printer({ name: 'Bar', host: '10.0.0.7', status: 'unknown' }));
      },
    });
    expect(await screen.findByText('Nessuna stampante')).toBeInTheDocument();
    await user.click(screen.getAllByRole('button', { name: 'Aggiungi stampante' })[0]!);
    const sheet = await screen.findByRole('dialog');
    await user.type(within(sheet).getByLabelText('Nome'), 'Bar');
    await user.type(within(sheet).getByLabelText('Indirizzo IP o nome host'), '8.8.8.8');
    await user.click(within(sheet).getByRole('button', { name: 'Salva' }));
    expect(
      await within(sheet).findByText(/Sono ammessi solo indirizzi della rete locale/),
    ).toBeInTheDocument();
    expect(callsTo(fetchMock, 'POST', `${API}/printers`)).toHaveLength(0);

    const host = within(sheet).getByLabelText('Indirizzo IP o nome host');
    await user.clear(host);
    await user.type(host, '10.0.0.7');
    await user.click(within(sheet).getByRole('button', { name: 'Salva' }));
    await waitFor(() => expect(posts).toHaveLength(1));
    expect(posts[0]).toEqual({
      name: 'Bar',
      host: '10.0.0.7',
      port: 9100,
      paper_width_mm: 80,
      chars_per_line: 48,
      codepage: 'cp858',
      cut_mode: 'partial',
      feed_lines: 3,
      print_logo: true,
      active: true,
    });
    expect(await screen.findByText('Stampante Bar aggiunta')).toBeInTheDocument();
  });

  it('shows the server refusal of a hostname outside the LAN on the host field', async () => {
    const user = userEvent.setup();
    renderApp(PATH, {
      ...desktopRoutes([]),
      [`POST ${API}/printers`]: () => problem(422, 'printer_address_not_allowed'),
    });
    await user.click((await screen.findAllByRole('button', { name: 'Aggiungi stampante' }))[0]!);
    const sheet = await screen.findByRole('dialog');
    await user.type(within(sheet).getByLabelText('Nome'), 'Bar');
    await user.type(
      within(sheet).getByLabelText('Indirizzo IP o nome host'),
      'printer.example.com',
    );
    await user.click(within(sheet).getByRole('button', { name: 'Salva' }));
    expect(
      await within(sheet).findByText(/Sono ammessi solo indirizzi della rete locale/),
    ).toBeInTheDocument();
  });

  it('lists the latest jobs and retries a failed one', async () => {
    const user = userEvent.setup();
    const { fetchMock } = renderApp(PATH, {
      ...desktopRoutes([printer({ status: 'unreachable', last_error: 'dial tcp: i/o timeout' })]),
      [`GET ${API}/print-jobs`]: () =>
        list([
          job({
            document_kind: 'cash_z',
            status: 'failed',
            attempts: 5,
            last_error: 'connection refused',
          }),
        ]),
      [`POST ${API}/print-jobs/j1/retry`]: () => json(200, job({ document_kind: 'cash_z' })),
    });
    expect(await screen.findByText('Non raggiungibile')).toBeInTheDocument();
    expect(screen.getByText('dial tcp: i/o timeout')).toBeInTheDocument();
    await waitFor(() => expect(screen.getByText('Tentativo 5 di 5')).toBeInTheDocument());
    expect(screen.getByText('Chiusura di cassa (Z)')).toBeInTheDocument();
    expect(screen.getByText('connection refused')).toBeInTheDocument();
    await user.click(
      screen.getByRole('button', { name: 'Riprova la stampa: Chiusura di cassa (Z)' }),
    );
    await waitFor(() =>
      expect(callsTo(fetchMock, 'POST', `${API}/print-jobs/j1/retry`)).toHaveLength(1),
    );
  });
});
