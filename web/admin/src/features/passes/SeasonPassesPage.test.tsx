import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import {
  baseRoutes,
  callsTo,
  EST_ID,
  json,
  list,
  renderApp,
  type Handler,
} from '../configuration/testing';
import { estPath } from '../bookings/testing';

const ts = { version: 1, created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' };
const MAP = {
  id: 'm1',
  name: { it: 'Spiaggia' },
  width: 100,
  height: 60,
  grid_size: 5,
  sea_side: 'top',
  sort_order: 1,
  spot_count: 3,
  ...ts,
};
const spot = (id: string, label: string, row: string, n: number) => ({
  id,
  map_id: 'm1',
  zone_id: 'z1',
  label,
  row_label: row,
  number: n,
  kind: 'umbrella',
  x: n * 10,
  y: row === 'A' ? 10 : 20,
  rotation: 0,
  shape: { type: 'circle', w: 6, h: 6 },
  capacity: {},
  max_extra: {},
  attributes: [],
  active: true,
});
const ZONE = {
  id: 'z1',
  map_id: 'm1',
  code: 'prima',
  name: { it: 'Prima fila' },
  assignment_mode: 'seat',
  online_visible: true,
  price_category: 'std',
  auto_assign: false,
  sort_order: 1,
  spot_count: 3,
  ...ts,
};
const pass = (
  id: string,
  spotId: string,
  label: string,
  customer: string,
  extra: Record<string, unknown> = {},
) => ({
  id,
  code: `AB-${id}`,
  status: 'confirmed',
  customer_id: 'c',
  customer_display_name: customer,
  season_id: 's2026',
  spot_id: spotId,
  spot_label: label,
  pack_id: 'p1',
  starts_on: '2026-05-15',
  ends_on: '2026-09-20',
  slots: 'FULL',
  total_amount: 150000,
  paid_amount: 100000,
  balance_due: 50000,
  released_days: 0,
  credit_amount: 0,
  version: 1,
  created_at: '2026-04-01T00:00:00Z',
  ...extra,
});

describe('SeasonPassesPage', () => {
  it('groups passes by zone and row and opens the releases sheet', async () => {
    const routes: Record<string, Handler> = {
      ...baseRoutes('manager'),
      [`GET ${estPath('/seasons')}`]: () =>
        list([
          {
            id: 's2026',
            establishment_id: EST_ID,
            year: 2026,
            opens_on: '2026-05-15',
            closes_on: '2026-09-20',
            status: 'active',
            ...ts,
          },
        ]),
      [`GET ${estPath('/maps')}`]: () => list([MAP]),
      [`GET ${estPath('/maps/m1')}`]: () =>
        json(200, {
          map: MAP,
          spots: [spot('a1', 'A1', 'A', 1), spot('a2', 'A2', 'A', 2), spot('b1', 'B1', 'B', 1)],
          decorations: [],
        }),
      [`GET ${estPath('/zones')}`]: () => list([ZONE]),
      [`GET ${estPath('/season-passes')}`]: (req) => {
        expect(new URL(req.url).searchParams.get('season_id')).toBe('s2026');
        return json(200, {
          items: [
            pass('3', 'b1', 'B1', 'Neri Anna'),
            pass('2', 'a2', 'A2', 'Bianchi Luca', { released_days: 4, credit_amount: 2400 }),
            pass('1', 'a1', 'A1', 'Rossi Mario'),
          ],
        });
      },
      [`GET ${estPath('/season-passes/2')}`]: () =>
        json(200, {
          pass: pass('2', 'a2', 'A2', 'Bianchi Luca', { released_days: 4, credit_amount: 2400 }),
          releases: [],
        }),
    };
    renderApp(`/e/${EST_ID}/abbonamenti`, routes);
    const table = await screen.findByRole('table', { name: 'Abbonamenti' });
    const rows = within(table)
      .getAllByRole('row')
      .slice(1)
      .map((r) => r.textContent ?? '');
    expect(rows[0]).toBe('Prima fila · Fila A2 abbonamenti');
    expect(rows[1]).toMatch(/^A1Rossi Mario/);
    expect(rows[2]).toMatch(/^A2Bianchi Luca.*4.*24,00\s€/);
    expect(rows[3]).toBe('Prima fila · Fila B1 abbonamento');
    expect(rows[4]).toMatch(/^B1Neri Anna/);
    expect(screen.getAllByText('3 abbonamenti')).not.toHaveLength(0);

    await userEvent.setup().click(within(table).getByRole('row', { name: /A2 di Bianchi Luca/ }));
    const sheet = await screen.findByRole('dialog', { name: 'A2 · Bianchi Luca' });
    expect(within(sheet).getByText('Credito maturato: 24,00 €')).toBeInTheDocument();
    expect(within(sheet).getByRole('link', { name: /Apri la prenotazione/ })).toBeInTheDocument();
  });

  it('sets a manual price with reason and If-Match (manager) and shows it in the detail', async () => {
    const user = userEvent.setup();
    let current = pass('1', 'a1', 'A1', 'Rossi Mario');
    const bodies: unknown[] = [];
    const routes: Record<string, Handler> = {
      ...baseRoutes('manager'),
      [`GET ${estPath('/seasons')}`]: () =>
        list([
          {
            id: 's2026',
            establishment_id: EST_ID,
            year: 2026,
            opens_on: '2026-05-15',
            closes_on: '2026-09-20',
            status: 'active',
            ...ts,
          },
        ]),
      [`GET ${estPath('/maps')}`]: () => list([MAP]),
      [`GET ${estPath('/maps/m1')}`]: () =>
        json(200, { map: MAP, spots: [spot('a1', 'A1', 'A', 1)], decorations: [] }),
      [`GET ${estPath('/zones')}`]: () => list([ZONE]),
      [`GET ${estPath('/season-passes')}`]: () => json(200, { items: [current] }),
      [`GET ${estPath('/season-passes/1')}`]: () => json(200, { pass: current, releases: [] }),
      [`PUT ${estPath('/season-passes/1/price-override')}`]: async (req) => {
        const body = (await req.json()) as { amount: number; reason: string };
        bodies.push(body);
        current = pass('1', 'a1', 'A1', 'Rossi Mario', {
          total_amount: body.amount,
          version: 2,
          price_override: {
            amount: body.amount,
            engine_amount: 150000,
            reason: body.reason,
            set_at: '2026-06-01T10:00:00Z',
          },
        });
        return json(200, { pass: current, releases: [] }, { ETag: '"2"' });
      },
    };
    const { fetchMock } = renderApp(`/e/${EST_ID}/abbonamenti`, routes);
    const table = await screen.findByRole('table', { name: 'Abbonamenti' });
    await user.click(within(table).getByRole('row', { name: /A1 di Rossi Mario/ }));
    const sheet = await screen.findByRole('dialog', { name: 'A1 · Rossi Mario' });
    await user.click(within(sheet).getByRole('button', { name: 'Imposta prezzo manuale' }));
    const dialog = await screen.findByRole('dialog', { name: 'Prezzo manuale' });
    const amount = within(dialog).getByRole('textbox', { name: /Nuovo totale/ });
    await user.clear(amount);
    await user.type(amount, '1200');
    await user.type(within(dialog).getByRole('textbox', { name: 'Motivo' }), 'Cliente storico');
    await user.click(within(dialog).getByRole('button', { name: 'Salva prezzo' }));
    await waitFor(() =>
      expect(callsTo(fetchMock, 'PUT', estPath('/season-passes/1/price-override'))).toHaveLength(1),
    );
    expect(
      callsTo(fetchMock, 'PUT', estPath('/season-passes/1/price-override'))[0]!.headers.get(
        'If-Match',
      ),
    ).toBe('"1"');
    expect(bodies).toEqual([{ amount: 120000, reason: 'Cliente storico' }]);
    expect(await within(sheet).findByText('Prezzo manuale')).toBeInTheDocument();
    expect(within(sheet).getByText('Cliente storico')).toBeInTheDocument();
  });

  it('hides the manual price actions from a cashier', async () => {
    const current = pass('1', 'a1', 'A1', 'Rossi Mario');
    renderApp(`/e/${EST_ID}/abbonamenti`, {
      ...baseRoutes('cashier'),
      [`GET ${estPath('/seasons')}`]: () =>
        list([
          {
            id: 's2026',
            establishment_id: EST_ID,
            year: 2026,
            opens_on: '2026-05-15',
            closes_on: '2026-09-20',
            status: 'active',
            ...ts,
          },
        ]),
      [`GET ${estPath('/maps')}`]: () => list([MAP]),
      [`GET ${estPath('/maps/m1')}`]: () =>
        json(200, { map: MAP, spots: [spot('a1', 'A1', 'A', 1)], decorations: [] }),
      [`GET ${estPath('/zones')}`]: () => list([ZONE]),
      [`GET ${estPath('/season-passes')}`]: () => json(200, { items: [current] }),
      [`GET ${estPath('/season-passes/1')}`]: () => json(200, { pass: current, releases: [] }),
    });
    const table = await screen.findByRole('table', { name: 'Abbonamenti' });
    await userEvent.click(within(table).getByRole('row', { name: /A1 di Rossi Mario/ }));
    const sheet = await screen.findByRole('dialog', { name: 'A1 · Rossi Mario' });
    expect(
      within(sheet).getByText('Solo il titolare e il gestore possono cambiare il prezzo.'),
    ).toBeInTheDocument();
    expect(within(sheet).queryByRole('button', { name: 'Imposta prezzo manuale' })).toBeNull();
  });
});
