import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from 'vitest';
import { EST_ID, baseRoutes, callsTo, json, renderApp } from '../configuration/testing';
import { API, printData, saleRoutes, ticket } from './testing';

const PATH = `/admin/e/${EST_ID}/vendita-rapida`;

const desktopMeta = () =>
  json(200, {
    product: 'arenile',
    version: 'test',
    api_version: '1',
    edition: 'desktop',
    desktop: { mode: 'local' },
    server_time: '2026-07-14T08:00:00Z',
    min_client_versions: { mobile: '1.0.0', desktop: '1.0.0' },
    capabilities: {
      online_booking: false,
      online_payments: false,
      payment_providers: ['manual'],
      fiscal_drivers: [],
      device_pairing: false,
      fiscal_bridge: false,
      realtime: true,
      push_notifications: false,
    },
    features: [],
  });

const printer = {
  id: 'p1',
  name: 'Banco',
  driver: 'network',
  host: '192.168.1.50',
  port: 9100,
  paper_width_mm: 80,
  chars_per_line: 48,
  codepage: 'cp858',
  cut_mode: 'partial',
  feed_lines: 3,
  print_logo: false,
  logo_cached: false,
  active: true,
  status: 'reachable',
  version: 1,
  created_at: '2026-07-14T07:00:00Z',
  updated_at: '2026-07-14T07:00:00Z',
};

function setup(ready: boolean) {
  const bodies: unknown[] = [];
  const utils = renderApp(PATH, {
    ...baseRoutes('cashier'),
    ...saleRoutes({
      extra: {
        'GET /api/admin/v1/meta': desktopMeta,
        [`GET ${API}/print-routes/resolve`]: () =>
          json(
            200,
            ready
              ? {
                  document_kind: 'daily_ticket',
                  ready: true,
                  printer,
                  copies: 1,
                  kick_drawer: false,
                }
              : {
                  document_kind: 'daily_ticket',
                  ready: false,
                  reason: 'no_route',
                  copies: 1,
                  kick_drawer: false,
                },
          ),
        [`POST ${API}/daily-sales`]: () =>
          json(201, { ticket: ticket(), booking_id: 'b37', payment_id: 'p' }),
        [`GET ${API}/tickets/t37/print`]: () => json(200, printData()),
        [`POST ${API}/print-jobs`]: async (req) => {
          bodies.push(await req.json());
          return json(201, {
            id: 'j1',
            printer_id: 'p1',
            printer_name: 'Banco',
            document_kind: 'daily_ticket',
            copies: 1,
            status: 'printed',
            attempts: 1,
            max_attempts: 5,
            created_at: '2026-07-14T08:00:00Z',
            updated_at: '2026-07-14T08:00:00Z',
          });
        },
      },
    }),
  });
  return { ...utils, bodies };
}

async function sellByCard(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByRole('combobox', { name: 'Postazione' }));
  await user.click(await screen.findByRole('option', { name: 'A1 · Prima fila' }));
  const card = screen.getByRole('button', { name: /Carta/ });
  await waitFor(() => expect(card).toBeEnabled());
  await user.click(card);
  await screen.findByTestId('ticket-number');
}

describe('Daily ticket on the thermal printer (desktop edition, ADR 0021)', () => {
  const print = vi.fn();
  beforeAll(() => {
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(new Date('2026-07-14T10:00:00'));
    vi.stubGlobal('print', print);
  });
  afterEach(() => print.mockReset());
  afterAll(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it('auto-prints the sold ticket as an ESC/POS job when the route is ready', async () => {
    const user = userEvent.setup();
    const { fetchMock, bodies } = setup(true);
    await waitFor(() =>
      expect(callsTo(fetchMock, 'GET', `${API}/print-routes/resolve`).length).toBeGreaterThan(0),
    );
    await sellByCard(user);
    expect(await screen.findByText('Inviato a Banco')).toBeInTheDocument();
    expect(bodies).toEqual([{ document_kind: 'daily_ticket', document_id: 't37' }]);
    const post = callsTo(fetchMock, 'POST', `${API}/print-jobs`)[0]!;
    expect(post.headers.get('Idempotency-Key')).toMatch(/.{8,}/);
    expect(callsTo(fetchMock, 'GET', `${API}/tickets/t37/print`)).toHaveLength(0);
    expect(print).not.toHaveBeenCalled();
  });

  it('keeps the browser print, without a notice, when no thermal printer is routed', async () => {
    const user = userEvent.setup();
    const { fetchMock } = setup(false);
    await waitFor(() =>
      expect(callsTo(fetchMock, 'GET', `${API}/print-routes/resolve`).length).toBeGreaterThan(0),
    );
    await sellByCard(user);
    await waitFor(() => expect(print).toHaveBeenCalledOnce());
    expect(callsTo(fetchMock, 'GET', `${API}/tickets/t37/print`)).toHaveLength(1);
    expect(callsTo(fetchMock, 'POST', `${API}/print-jobs`)).toHaveLength(0);
    expect(
      screen.queryByText('Nessuna stampante termica configurata per questo documento.'),
    ).toBeNull();
  });
});
