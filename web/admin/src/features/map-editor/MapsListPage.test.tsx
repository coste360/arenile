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
  type Handler,
} from '../configuration/testing';

const API = `/api/admin/v1/establishments/${EST_ID}`;
const ts = { created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' };
const map = (id: string, name: string, sort_order: number, spot_count = 0) => ({
  id,
  name: { it: name },
  width: 1200,
  height: 800,
  grid_size: 10,
  sea_side: 'bottom',
  sort_order,
  spot_count,
  version: 2,
  ...ts,
});

function routes(role: 'manager' | 'cashier', extra: Record<string, Handler> = {}) {
  return {
    ...baseRoutes(role),
    [`GET ${API}/maps`]: () => list([map('m2', 'Nord', 1, 3), map('m1', 'Spiaggia', 0, 120)]),
    [`GET ${API}/zones`]: () => list([]),
    [`GET ${API}/components`]: () => list([]),
    ...extra,
  };
}

describe('MapsListPage', () => {
  it('lists maps in order with size and spot count', async () => {
    renderApp(`/admin/e/${EST_ID}/maps`, routes('manager'));
    const table = await screen.findByRole('table', { name: 'Mappe' });
    await within(table).findByRole('link', { name: 'Spiaggia' });
    const rows = within(table).getAllByRole('row').slice(1);
    expect(rows.map((r) => within(r).getAllByRole('cell')[0]?.textContent)).toEqual([
      'Spiaggia',
      'Nord',
    ]);
    expect(within(rows[0] as HTMLElement).getByText('1200 × 800')).toBeInTheDocument();
    expect(within(rows[0] as HTMLElement).getByText('120')).toBeInTheDocument();
  });

  it('creates a map with an idempotency key and opens the editor', async () => {
    const user = userEvent.setup();
    let body: Record<string, unknown> | undefined;
    const created = map('m3', 'Sud', 2);
    const { fetchMock, history } = renderApp(
      `/admin/e/${EST_ID}/maps`,
      routes('manager', {
        [`POST ${API}/maps`]: async (req) => {
          body = (await req.json()) as Record<string, unknown>;
          return json(201, created, { ETag: '"1"' });
        },
        [`GET ${API}/maps/m3`]: () => json(200, { map: created, spots: [], decorations: [] }),
      }),
    );
    await screen.findByRole('table', { name: 'Mappe' });
    await user.click(screen.getByRole('button', { name: 'Nuova mappa' }));
    const dialog = await screen.findByRole('dialog');
    await user.type(within(dialog).getByLabelText('Nome'), 'Sud');
    await user.click(within(dialog).getByRole('button', { name: "Crea e apri l'editor" }));
    await waitFor(() => expect(history.location.pathname).toBe(`/admin/e/${EST_ID}/maps/m3`));
    expect(body).toMatchObject({
      name: { it: 'Sud' },
      width: 1200,
      height: 800,
      grid_size: 10,
      sea_side: 'bottom',
      sort_order: 2,
    });
    const req = callsTo(fetchMock, 'POST', `${API}/maps`)[0] as Request;
    expect(req.headers.get('Idempotency-Key')).toBeTruthy();
  });

  it('validates the map settings before creating', async () => {
    const user = userEvent.setup();
    const { fetchMock } = renderApp(`/admin/e/${EST_ID}/maps`, routes('manager'));
    await screen.findByRole('table', { name: 'Mappe' });
    await user.click(screen.getByRole('button', { name: 'Nuova mappa' }));
    const dialog = await screen.findByRole('dialog');
    const grid = within(dialog).getByLabelText('Passo griglia');
    await user.clear(grid);
    await user.type(grid, '0');
    await user.click(within(dialog).getByRole('button', { name: "Crea e apri l'editor" }));
    await waitFor(() => expect(grid).toHaveAttribute('aria-invalid', 'true'));
    expect(callsTo(fetchMock, 'POST', `${API}/maps`)).toHaveLength(0);
  });

  it('deletes a map with If-Match and shows the server refusal', async () => {
    const user = userEvent.setup();
    let calls = 0;
    const { fetchMock } = renderApp(
      `/admin/e/${EST_ID}/maps`,
      routes('manager', {
        [`DELETE ${API}/maps/m2`]: () => {
          calls++;
          return calls === 1
            ? problem(409, 'spot_has_future_allocations', {
                detail: 'La mappa ha postazioni con prenotazioni future.',
                spot_ids: ['s1'],
              })
            : json(204, null);
        },
      }),
    );
    await user.click(await screen.findByRole('button', { name: 'Elimina la mappa Nord' }));
    const dialog = await screen.findByRole('alertdialog');
    expect(within(dialog).getByText(/e le sue 3 postazioni verranno eliminate/)).toBeVisible();
    await user.click(within(dialog).getByRole('button', { name: 'Elimina' }));
    expect(await within(dialog).findByRole('alert')).toHaveTextContent(
      'La mappa ha postazioni con prenotazioni future.',
    );
    await user.click(within(dialog).getByRole('button', { name: 'Elimina' }));
    await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument());
    const req = callsTo(fetchMock, 'DELETE', `${API}/maps/m2`)[1] as Request;
    expect(req.headers.get('If-Match')).toBe('"2"');
  });

  it('is view-only for a cashier', async () => {
    renderApp(`/admin/e/${EST_ID}/maps`, routes('cashier'));
    expect(await screen.findByRole('link', { name: 'Apri la mappa Spiaggia' })).toHaveTextContent(
      'Visualizza',
    );
    expect(screen.queryByRole('button', { name: 'Nuova mappa' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Elimina la mappa/ })).not.toBeInTheDocument();
  });
});
