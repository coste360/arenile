import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';
import { baseRoutes, callsTo, json, list, renderApp, EST_ID } from '../configuration/testing';
import { opBooking, opDay, opSpot } from './testing/fixtures';
import { API, operationsRoutes } from './testing/routes';

/** M8 cash-desk additions of the operational map (spec 30.4-30.10). */

const PATH = `/admin/e/${EST_ID}/mappa?date=2026-07-14&slot=FULL`;

const day = opDay([
  opSpot('s1', 'A1', 'free'),
  opSpot('s2', 'A2', 'booked', {
    booking: opBooking({
      starts_on: '2026-07-12',
      ends_on: '2026-07-16',
      version: 3,
      group_name: 'Gita CRAL',
      agreement: { id: 'ag1', name: 'Residenti' },
    }),
  }),
  opSpot('s3', 'A3', 'booked', {
    booking: opBooking({
      booking_id: 'b2',
      line_id: 'l2',
      code: 'AR-0002',
      customer_display_name: 'Bianchi A.',
      occupant_label: 'Bianchi',
      starts_on: '2026-07-14',
      ends_on: '2026-07-18',
      is_first_day: true,
      version: 5,
      group_name: 'Gita CRAL',
      setup_label: '2L',
    }),
  }),
  opSpot('s4', 'A4', 'blocked'),
]);

const outcome = (id: string, diff: number) => ({
  booking: { id },
  from_spot_id: 's2',
  to_spot_id: 's3',
  previous_total: 3000,
  new_total: 3000 + diff,
  price_difference: diff,
});

const undoInfo = () => ({
  move_id: 'mv1',
  kind: 'move',
  booking_ids: ['b1'],
  expires_at: new Date(Date.now() + 5 * 60_000).toISOString(),
});

function setup(extra: Parameters<typeof operationsRoutes>[1] = {}) {
  return renderApp(PATH, {
    ...baseRoutes('cashier'),
    ...operationsRoutes(day, { [`GET ${API}/moves/last`]: () => json(200, {}), ...extra }),
  });
}

const spot = (label: string) =>
  screen.findByRole('button', { name: new RegExp(`^Postazione ${label}, fila A`) });

describe('OperationalMapPage M8', () => {
  beforeAll(() => {
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(new Date('2026-07-14T10:00:00'));
  });
  afterAll(() => {
    vi.useRealTimers();
  });

  it('draws arrival, agreement and group indicators and highlights the quick filters', async () => {
    const user = userEvent.setup();
    setup();
    const a3 = await spot('A3');
    expect(a3).toHaveAccessibleName(/Arrivo, Gruppo, fine 18\/7, allestimento 2L/);
    expect(a3.querySelector('[data-indicator="arrival"]')).not.toBeNull();
    expect((await spot('A2')).querySelector('[data-indicator="agreement"]')).not.toBeNull();
    expect(document.querySelector('[data-indicator-legend="departure"]')).not.toBeNull();

    const filters = screen.getByTestId('quick-filters');
    await user.click(within(filters).getByRole('radio', { name: 'Arrivi' }));
    expect(a3).toHaveAttribute('data-highlighted', 'true');
    expect(await spot('A2')).toHaveAttribute('data-highlighted', 'false');
    await user.click(within(filters).getByRole('button', { name: 'Togli il filtro' }));
    expect(a3).not.toHaveAttribute('data-highlighted');
  });

  it('moves from tomorrow and offers the undo for 5 minutes', async () => {
    const user = userEvent.setup();
    const moves: Record<string, unknown>[] = [];
    const { fetchMock } = setup({
      [`POST ${API}/bookings/b1/move`]: async (req) => {
        const b = (await req.json()) as Record<string, unknown>;
        moves.push(b);
        return json(200, {
          applied: !b.dry_run,
          booking: { id: 'b1' },
          previous_total: 3000,
          new_total: 3000,
          price_difference: 0,
          conflicts: [],
          ...(b.dry_run ? {} : { undo: undoInfo() }),
        });
      },
      [`POST ${API}/moves/mv1/undo`]: () => json(200, { undone: true, bookings: [] }),
    });
    await user.click(await spot('A2'));
    await user.click(
      within(await screen.findByRole('dialog')).getByRole('button', { name: 'Sposta' }),
    );
    await user.click(await spot('A1'));
    const dialog = await screen.findByRole('alertdialog');
    // The stay started on the 12th: today, tomorrow or a date (spec 30.7).
    const from = within(dialog).getByRole('radiogroup', { name: 'Da quando' });
    expect(within(from).getByRole('radio', { name: 'Da oggi' })).toHaveAttribute(
      'aria-checked',
      'true',
    );
    await user.click(within(from).getByRole('radio', { name: 'Da domani' }));
    await waitFor(() => expect(moves.some((m) => m.from === '2026-07-15')).toBe(true));
    await user.click(await within(dialog).findByRole('button', { name: 'Sposta' }));
    await waitFor(() => expect(moves.some((m) => m.dry_run === false)).toBe(true));
    expect(moves.find((m) => m.dry_run === false)).toEqual({
      line_id: 'l1',
      spot_id: 's1',
      from: '2026-07-15',
      dry_run: false,
    });
    await user.click(await screen.findByRole('button', { name: 'Annulla' }));
    await waitFor(() =>
      expect(callsTo(fetchMock, 'POST', `${API}/moves/mv1/undo`)).toHaveLength(1),
    );
  });

  it('proposes the swap when a booking is dropped on an occupied spot', async () => {
    const user = userEvent.setup();
    const swaps: Record<string, unknown>[] = [];
    setup({
      [`POST ${API}/bookings/swap`]: async (req) => {
        const b = (await req.json()) as Record<string, unknown>;
        swaps.push(b);
        return json(200, {
          applied: !b.dry_run,
          from: '2026-07-14',
          to: '2026-07-16',
          a: outcome('b1', 500),
          b: outcome('b2', -500),
          conflicts: [],
        });
      },
    });
    await user.click(await spot('A2'));
    await user.click(
      within(await screen.findByRole('dialog')).getByRole('button', { name: 'Sposta' }),
    );
    expect(
      await screen.findByText(
        'Tocca una postazione libera per spostarla, oppure una occupata per scambiarle.',
      ),
    ).toBeInTheDocument();
    await user.click(await spot('A3'));
    const dialog = await screen.findByRole('alertdialog', { name: /Scambiare A2 e A3/ });
    const preview = await within(dialog).findByTestId('swap-preview');
    expect(preview).toHaveTextContent('AR-0001');
    expect(preview).toHaveTextContent('AR-0001Differenza da incassare: 5,00 €');
    expect(preview).toHaveTextContent('AR-0002Differenza da rimborsare: 5,00 €');
    await user.click(within(dialog).getByRole('button', { name: 'Scambia' }));
    await waitFor(() => expect(swaps.some((s) => s.dry_run === false)).toBe(true));
    expect(swaps.find((s) => s.dry_run === false)).toEqual({
      a: { booking_id: 'b1', line_id: 'l1', expected_version: 3 },
      b: { booking_id: 'b2', line_id: 'l2', expected_version: 5 },
      from: '2026-07-14',
      dry_run: false,
    });
  });

  it('moves a group keeping the arrangement', async () => {
    const user = userEvent.setup();
    const bodies: Record<string, unknown>[] = [];
    setup({
      [`POST ${API}/bookings/group-move`]: async (req) => {
        const b = (await req.json()) as Record<string, unknown>;
        bodies.push(b);
        return json(200, { applied: !b.dry_run, results: [], conflicts: [] });
      },
    });
    await user.click(await spot('A2'));
    await user.click(
      within(await screen.findByRole('dialog')).getByRole('button', {
        name: 'Sposta il gruppo (2)',
      }),
    );
    expect(await screen.findByText(/Tocca la nuova posizione di A2/)).toBeInTheDocument();
    await user.click(await spot('A1'));
    const dialog = await screen.findByRole('dialog', { name: /Sposta 2 postazioni del gruppo/ });
    await within(dialog).findByTestId('group-move-price');
    await user.click(within(dialog).getByRole('button', { name: 'Sposta il gruppo' }));
    await waitFor(() => expect(bodies.some((b) => b.dry_run === false)).toBe(true));
    expect(bodies.find((b) => b.dry_run === false)).toEqual({
      moves: [
        { booking_id: 'b1', line_id: 'l1', spot_id: 's1', expected_version: 3 },
        { booking_id: 'b2', line_id: 'l2', spot_id: 's2', expected_version: 5 },
      ],
      // The common stay starts today: the whole stay, no `from`.
      dry_run: false,
    });
  });

  it('opens the season calendar of a spot and starts a booking from a free day', async () => {
    const user = userEvent.setup();
    setup({
      [`GET ${API}/spots/s1/calendar`]: () =>
        json(200, {
          spot_id: 's1',
          label: 'A1',
          season_id: 'se1',
          from: '2026-07-14',
          to: '2026-07-16',
          days: [
            { date: '2026-07-14', am: { status: 'free' }, pm: { status: 'free' } },
            { date: '2026-07-15', am: { status: 'free' }, pm: { status: 'free' } },
            {
              date: '2026-07-16',
              am: { status: 'booked', booking_id: 'b1' },
              pm: { status: 'booked', booking_id: 'b1' },
            },
          ],
          bookings: [
            {
              booking_id: 'b1',
              code: 'AR-0001',
              status: 'confirmed',
              occupant_label: 'Rossi',
              starts_on: '2026-07-16',
              ends_on: '2026-07-16',
              is_season_pass: false,
              balance_due: 1000,
            },
          ],
        }),
    });
    await user.click(await spot('A1'));
    await user.click(
      within(await screen.findByRole('dialog')).getByRole('button', { name: 'Calendario' }),
    );
    const cal = await screen.findByRole('dialog', { name: 'Calendario di A1' });
    const booked = await within(cal).findByRole('button', { name: /^16\/07\/2026/ });
    expect(booked).toHaveAccessibleName(/Mattina Da incassare \(AR-0001 Rossi\)/);
    expect(booked.querySelector('[data-cal-status="to_collect"]')).not.toBeNull();
    await user.click(within(cal).getByRole('button', { name: /^14\/07\/2026/ }));
    expect(await screen.findByRole('dialog', { name: 'Nuova prenotazione' })).toBeInTheDocument();
  });

  it('shows the lifeguard list by row and prints it', async () => {
    const user = userEvent.setup();
    const print = vi.spyOn(window, 'print').mockImplementation(() => {});
    setup({
      [`GET ${API}/operations/lifeguard-list`]: () =>
        json(200, {
          date: '2026-07-14',
          slot: 'FULL',
          rows: [
            {
              row_label: 'A',
              spot_id: 's2',
              spot_label: 'A2',
              occupant_label: 'Rossi',
              setup_label: '2L 1S',
              arriving: true,
              departing: false,
              balance_due: true,
            },
          ],
          totals: day.totals,
        }),
    });
    await spot('A1');
    await user.click(screen.getByRole('button', { name: 'Stampa' }));
    await user.click(await screen.findByRole('menuitem', { name: 'Lista per i bagnini' }));
    const dialog = await screen.findByRole('dialog', { name: 'Lista per i bagnini' });
    expect(await within(dialog).findByText('2L 1S')).toBeInTheDocument();
    expect(within(dialog).getByText('Fila A')).toBeInTheDocument();
    await user.click(within(dialog).getByRole('button', { name: 'Stampa' }));
    expect(print).toHaveBeenCalled();
    expect(document.querySelector('[data-lifeguard-print]')).toHaveTextContent('Rossi');
    print.mockRestore();
  });

  it('shows the range view with spots free in part', async () => {
    const user = userEvent.setup();
    const { fetchMock } = setup({
      [`GET ${API}/availability`]: () =>
        json(200, {
          from: '2026-07-14',
          to: '2026-07-20',
          slots: 'FULL',
          channel: 'desk',
          zones: [
            {
              zone_id: 'z1',
              code: 'Z1',
              assignment_mode: 'seat',
              capacity: 4,
              available: 1,
              spots: [
                { spot_id: 's1', label: 'A1', status: 'available', free_days: 7, total_days: 7 },
                {
                  spot_id: 's2',
                  label: 'A2',
                  status: 'unavailable',
                  free_days: 3,
                  total_days: 7,
                  free_dates: ['2026-07-17', '2026-07-18', '2026-07-19'],
                },
                { spot_id: 's3', label: 'A3', status: 'unavailable', free_days: 0, total_days: 7 },
              ],
            },
          ],
          pools: [],
        }),
    });
    await spot('A1');
    await user.click(screen.getByRole('radio', { name: 'Intervallo' }));
    const a2 = await screen.findByRole('button', {
      name: 'Postazione A2, fila A: Libero in parte, libero 3 giorni su 7',
    });
    const req = callsTo(fetchMock, 'GET', `${API}/availability`)[0] as Request;
    expect(new URL(req.url).searchParams.get('to')).toBe('2026-07-20');
    expect(
      screen.getByRole('button', { name: 'Postazione A3, fila A: Occupato' }),
    ).toBeInTheDocument();
    await user.click(a2);
    const dialog = await screen.findByRole('dialog', { name: 'Postazione A2' });
    expect(within(dialog).getByText('17/07/2026 → 19/07/2026')).toBeInTheDocument();
  });

  it('finds a spot and highlights the proposal on the map', async () => {
    const user = userEvent.setup();
    const { fetchMock } = setup({
      [`POST ${API}/find-spot`]: () =>
        json(200, {
          proposals: [
            {
              id: 'p1',
              rank: 1,
              spot_changes: 1,
              spot_ids: ['s1'],
              lines: [
                {
                  segments: [
                    {
                      spot_id: 's1',
                      spot_label: 'A1',
                      zone_id: 'z1',
                      starts_on: '2026-07-14',
                      ends_on: '2026-07-15',
                    },
                    {
                      spot_id: 's4',
                      spot_label: 'A4',
                      zone_id: 'z1',
                      starts_on: '2026-07-16',
                      ends_on: '2026-07-16',
                    },
                  ],
                },
              ],
            },
          ],
        }),
      [`GET ${API}/packs`]: () => list([]),
      [`GET ${API}/zones`]: () => list([]),
      [`GET ${API}/agreements`]: () => list([]),
    });
    await spot('A1');
    await user.click(screen.getByRole('button', { name: 'Trova posto' }));
    const sheet = await screen.findByRole('dialog', { name: 'Trova posto' });
    await user.click(within(sheet).getByRole('button', { name: 'Cerca' }));
    const proposal = await within(sheet).findByTestId('findspot-proposal');
    expect(proposal).toHaveTextContent('A1 dal 14/07/2026 al 15/07/2026, A4 il 16/07/2026');
    expect(proposal).toHaveTextContent('1 cambio di posto');
    const body = (await (
      callsTo(fetchMock, 'POST', `${API}/find-spot`)[0] as Request
    ).json()) as Record<string, unknown>;
    expect(body).toMatchObject({ starts_on: '2026-07-14', slots: 'FULL', max_spot_changes: 0 });
    await user.click(within(proposal).getByRole('button', { name: 'Mostra sulla mappa' }));
    expect(await screen.findByTestId('shown-proposal')).toHaveTextContent(
      'Proposta 1 evidenziata sulla mappa',
    );
    expect(await spot('A1')).toHaveAttribute('data-highlighted', 'true');
    expect(await spot('A2')).toHaveAttribute('data-highlighted', 'false');
  });
});
