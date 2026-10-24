import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { EST_ID, callsTo, json, list, renderApp } from '../configuration/testing';
import { API, openDetail, roleRoutes } from '../cash/testing';

const PATH = `/admin/e/${EST_ID}/cassa`;

const desktopMeta = () =>
  json(200, {
    product: 'arenile',
    version: 'test',
    api_version: '1',
    edition: 'desktop',
    desktop: { mode: 'local' },
    server_time: '2026-07-15T08:00:00Z',
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
  name: 'Cassa bar',
  driver: 'network',
  host: '192.168.1.50',
  port: 9100,
  paper_width_mm: 80,
  chars_per_line: 48,
  codepage: 'cp858',
  cut_mode: 'partial',
  feed_lines: 4,
  print_logo: false,
  logo_cached: false,
  active: true,
  status: 'reachable',
  version: 1,
  created_at: '2026-07-15T07:00:00Z',
  updated_at: '2026-07-15T07:00:00Z',
};

function routes(resolve: () => Response) {
  return {
    ...roleRoutes('cashier'),
    'GET /api/admin/v1/meta': desktopMeta,
    [`GET ${API}/cash-sessions`]: () => list([]),
    [`GET ${API}/cash-sessions/current`]: () => json(200, openDetail),
    [`GET ${API}/print-routes/resolve`]: resolve,
  };
}

afterEach(() => vi.restoreAllMocks());

describe('Cash Z print with ESC/POS (desktop edition)', () => {
  it('queues an ESC/POS job when a printer is ready, without the browser print', async () => {
    const user = userEvent.setup();
    const print = vi.spyOn(window, 'print').mockImplementation(() => {});
    const bodies: unknown[] = [];
    const { fetchMock } = renderApp(PATH, {
      ...routes(() =>
        json(200, { document_kind: 'cash_z', ready: true, printer, copies: 1, kick_drawer: false }),
      ),
      [`POST ${API}/print-jobs`]: async (req) => {
        bodies.push(await req.json());
        return json(201, {
          id: 'j1',
          printer_id: 'p1',
          printer_name: 'Cassa bar',
          document_kind: 'cash_z',
          copies: 1,
          status: 'printed',
          attempts: 1,
          max_attempts: 5,
          created_at: '2026-07-15T08:00:00Z',
          updated_at: '2026-07-15T08:00:00Z',
        });
      },
    });
    // Wait for the panel mode (meta) before printing.
    await waitFor(() => expect(callsTo(fetchMock, 'GET', '/api/admin/v1/meta')).toHaveLength(1));
    // The summary print lives in the header overflow menu ("Altro").
    await user.click(await screen.findByRole('button', { name: 'Altro' }));
    await user.click(await screen.findByRole('menuitem', { name: 'Stampa riepilogo' }));
    expect(await screen.findByText('Inviato a Cassa bar')).toBeInTheDocument();
    expect(bodies).toEqual([{ document_kind: 'cash_z', document_id: 's1' }]);
    const post = callsTo(fetchMock, 'POST', `${API}/print-jobs`)[0]!;
    expect(post.headers.get('Idempotency-Key')).toMatch(/.{8,}/);
    expect(print).not.toHaveBeenCalled();
  });

  it('falls back to the browser print with a notice when no printer is routed', async () => {
    const user = userEvent.setup();
    const print = vi.spyOn(window, 'print').mockImplementation(() => {});
    const { fetchMock } = renderApp(
      PATH,
      routes(() =>
        json(200, {
          document_kind: 'cash_z',
          ready: false,
          reason: 'no_route',
          copies: 1,
          kick_drawer: false,
        }),
      ),
    );
    await waitFor(() => expect(callsTo(fetchMock, 'GET', '/api/admin/v1/meta')).toHaveLength(1));
    // The summary print lives in the header overflow menu ("Altro").
    await user.click(await screen.findByRole('button', { name: 'Altro' }));
    await user.click(await screen.findByRole('menuitem', { name: 'Stampa riepilogo' }));
    await waitFor(() => expect(print).toHaveBeenCalledOnce());
    expect(
      await screen.findByText('Nessuna stampante termica configurata per questo documento.'),
    ).toBeInTheDocument();
    expect(callsTo(fetchMock, 'POST', `${API}/print-jobs`)).toHaveLength(0);
  });
});
