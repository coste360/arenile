import { screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { baseRoutes, EST_ID, json, list, renderApp, type Handler } from '../configuration/testing';
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
    const table = await screen.findByRole('table', { name: 'Abbonamenti della zona Prima fila' });
    const rows = within(table)
      .getAllByRole('row')
      .slice(1)
      .map((r) => r.textContent ?? '');
    expect(rows[0]).toBe('Fila A');
    expect(rows[1]).toMatch(/^A1Rossi Mario/);
    expect(rows[2]).toMatch(/^A2Bianchi Luca.*4.*24,00\s€/);
    expect(rows[3]).toBe('Fila B');
    expect(rows[4]).toMatch(/^B1Neri Anna/);
    expect(screen.getAllByText('3 abbonamenti')).not.toHaveLength(0);

    await userEvent.setup().click(within(table).getByRole('row', { name: /A2 di Bianchi Luca/ }));
    const sheet = await screen.findByRole('dialog', { name: 'A2 · Bianchi Luca' });
    expect(within(sheet).getByText('Credito maturato: 24,00 €')).toBeInTheDocument();
    expect(within(sheet).getByRole('link', { name: /Apri la prenotazione/ })).toBeInTheDocument();
  });
});
