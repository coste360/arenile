import { act, fireEvent, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
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
import { DEFAULT_METRICS as m } from './lib/grid';
import { planningFixture } from './testing/fixtures';

// The booking sheet belongs to the bookings feature: capture the prefill it receives.
vi.mock('../bookings', () => ({
  BookingDetailPage: () => <h1>detail</h1>,
  BookingsPage: () => <h1>list</h1>,
  NewBookingSheet: (props: { open: boolean; defaults?: unknown }) =>
    props.open ? <div data-testid="new-booking-sheet">{JSON.stringify(props.defaults)}</div> : null,
}));

const API = `/api/admin/v1/establishments/${EST_ID}`;
const PATH = `/admin/e/${EST_ID}/calendario`;
const ts = { created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' };
const zone = (id: string, name: string, sort_order: number) => ({
  id,
  code: id,
  name: { it: name },
  assignment_mode: 'seat',
  online_visible: true,
  price_category: 'std',
  auto_assign: false,
  sort_order,
  spot_count: 2,
  version: 1,
  ...ts,
});

const detail = (version: number) => ({
  id: 'b1',
  code: 'AR-0001',
  status: 'confirmed',
  starts_on: '2026-07-15',
  ends_on: '2026-07-16',
  version,
  lines: [
    {
      id: 'l1',
      pack_id: 'pk1',
      zone_id: 'z1',
      spot_id: 's-a1',
      assignment_status: 'assigned',
      quantity: 1,
      line_total: 3000,
      extras: [],
    },
  ],
});

const change = (applied: boolean, price_difference = 0) => ({
  applied,
  booking: detail(applied ? 9 : 1),
  previous_total: 3000,
  new_total: 3000 + price_difference,
  price_difference,
  conflicts: [],
});

function setup(extra: Record<string, Handler> = {}) {
  return renderApp(PATH, {
    ...baseRoutes('cashier'),
    [`GET ${API}/planning`]: () => json(200, planningFixture()),
    [`GET ${API}/zones`]: () => list([zone('z2', 'Gazebo', 1), zone('z1', 'Prima fila', 0)]),
    ...extra,
  });
}

/** Client point of the centre of a body cell (jsdom: the svg sits at 0,0, no scroll). */
const cell = (row: number, day: number) => ({
  clientX: m.labelWidth + day * m.cellWidth + m.cellWidth / 2,
  clientY: m.headerHeight + row * m.rowHeight + m.rowHeight / 2,
});

async function bookingBar() {
  const grid = await screen.findByTestId('planning-grid');
  await waitFor(() => expect(grid.querySelector('[data-entry-kind="booking"]')).not.toBeNull());
  return { grid, bar: grid.querySelector('[data-entry-kind="booking"] rect') as Element };
}

describe('PlanningPage', () => {
  it('draws grouped rows, bars in status colours and closed days', async () => {
    setup();
    const { grid } = await bookingBar();
    const rowLabels = [...grid.querySelectorAll('[data-row-kind] text')].map((t) => t.textContent);
    expect(rowLabels).toEqual(['Prima fila', 'Fila A', 'A1', 'A2', 'Fila B', 'B1', 'Gazebo', 'G1']);
    expect(grid.querySelector('[data-entry-kind="booking"]')).toHaveAttribute(
      'data-status',
      'booked_desk',
    );
    expect(grid.querySelector('[data-entry-kind="season"]')).toHaveAttribute(
      'data-status',
      'season',
    );
    expect(grid.querySelector('[data-closed-day="2026-07-18"]')).not.toBeNull();
    expect(grid).toHaveAccessibleName('Calendario di 4 postazioni');
  });

  it('drags a booking one day later: dry run, 412 → reload prompt, then saves with the fresh If-Match', async () => {
    const user = userEvent.setup();
    let version = 3;
    const ifMatches: (string | null)[] = [];
    const bodies: unknown[] = [];
    const { fetchMock } = setup({
      [`GET ${API}/bookings/b1`]: () => json(200, detail(version), { ETag: `"${version}"` }),
      [`PATCH ${API}/bookings/b1`]: async (req) => {
        const dry = new URL(req.url).searchParams.get('dry_run') === 'true';
        const ifMatch = req.headers.get('If-Match');
        bodies.push(await req.json());
        if (dry) return json(200, change(false, 0));
        ifMatches.push(ifMatch);
        if (ifMatch !== '"4"') {
          // Someone else saved the booking in the meantime.
          version = 4;
          return problem(412, 'version_conflict', { current_version: 4 });
        }
        return json(200, change(true));
      },
    });
    const { grid, bar } = await bookingBar();
    fireEvent.pointerDown(bar, { ...cell(2, 1), button: 0, pointerId: 1 });
    fireEvent.pointerMove(grid, { ...cell(2, 2), pointerId: 1 });
    expect(screen.getByTestId('planning-move-preview')).toBeInTheDocument();
    fireEvent.pointerUp(grid, { ...cell(2, 2), pointerId: 1 });

    const dialog = await screen.findByRole('alertdialog');
    expect(within(dialog).getByText('Modificare la prenotazione AR-0001?')).toBeInTheDocument();
    expect(await within(dialog).findByTestId('move-dates')).toHaveTextContent(
      'Date: 15/07/2026 – 16/07/2026 → 16/07/2026 – 17/07/2026',
    );
    expect(await within(dialog).findByText('Il prezzo non cambia.')).toBeInTheDocument();
    expect(bodies[0]).toEqual({ starts_on: '2026-07-16', ends_on: '2026-07-17' });

    await user.click(within(dialog).getByRole('button', { name: 'Conferma spostamento' }));
    expect(
      await within(dialog).findByText('Prenotazione modificata da un altro utente'),
    ).toBeInTheDocument();
    expect(within(dialog).getByRole('button', { name: 'Conferma spostamento' })).toBeDisabled();
    const gridLoads = callsTo(fetchMock, 'GET', `${API}/planning`).length;

    await user.click(within(dialog).getByRole('button', { name: 'Ricarica' }));
    await waitFor(() =>
      expect(within(dialog).getByRole('button', { name: 'Conferma spostamento' })).toBeEnabled(),
    );
    expect(callsTo(fetchMock, 'GET', `${API}/planning`).length).toBeGreaterThan(gridLoads);
    await user.click(within(dialog).getByRole('button', { name: 'Conferma spostamento' }));
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull());
    expect(ifMatches).toEqual(['"3"', '"4"']);
  });

  it('drags a booking onto another spot: all lines re-sent with the new spot', async () => {
    const bodies: Record<string, unknown>[] = [];
    setup({
      [`GET ${API}/bookings/b1`]: () => json(200, detail(1), { ETag: '"1"' }),
      [`PATCH ${API}/bookings/b1`]: async (req) => {
        bodies.push((await req.json()) as Record<string, unknown>);
        return json(200, change(false, 700));
      },
    });
    const { grid, bar } = await bookingBar();
    fireEvent.pointerDown(bar, { ...cell(2, 1), button: 0, pointerId: 1 });
    fireEvent.pointerMove(grid, { ...cell(3, 1), pointerId: 1 });
    fireEvent.pointerUp(grid, { ...cell(3, 1), pointerId: 1 });
    const dialog = await screen.findByRole('alertdialog');
    expect(within(dialog).getByText(/da A1 a A2/)).toBeInTheDocument();
    expect(await within(dialog).findByTestId('move-price')).toHaveTextContent(
      'Differenza da incassare: 7,00 €',
    );
    expect(bodies[0]).toEqual({
      lines: [{ pack_id: 'pk1', spot_id: 's-a2', quantity: 1, extras: [] }],
    });
  });

  it('clicking a bar without dragging opens the booking', async () => {
    const { history } = setup({
      [`GET ${API}/bookings/b1`]: () => json(200, detail(1)),
    });
    const { grid, bar } = await bookingBar();
    fireEvent.pointerDown(bar, { ...cell(2, 1), button: 0, pointerId: 1 });
    fireEvent.pointerUp(grid, { ...cell(2, 1), pointerId: 1 });
    await waitFor(() =>
      expect(history.location.pathname).toBe(`/admin/e/${EST_ID}/prenotazioni/b1`),
    );
  });

  it('selecting free cells opens the new booking sheet prefilled', async () => {
    setup();
    const { grid } = await bookingBar();
    fireEvent.pointerDown(grid, { ...cell(3, 0), button: 0, pointerId: 1 });
    fireEvent.pointerMove(grid, { ...cell(3, 2), pointerId: 1 });
    expect(screen.getByTestId('planning-selection')).toBeInTheDocument();
    fireEvent.pointerUp(grid, { ...cell(3, 2), pointerId: 1 });
    expect(JSON.parse((await screen.findByTestId('new-booking-sheet')).textContent ?? '')).toEqual({
      spotIds: ['s-a2'],
      startsOn: '2026-07-14',
      endsOn: '2026-07-16',
      slots: 'FULL',
    });
  });

  it('a selection over an occupied cell does not open the sheet', async () => {
    setup();
    const { grid } = await bookingBar();
    fireEvent.pointerDown(grid, { ...cell(3, 1), button: 0, pointerId: 1 });
    fireEvent.pointerMove(grid, { ...cell(2, 1), pointerId: 1 });
    fireEvent.pointerUp(grid, { ...cell(2, 1), pointerId: 1 });
    expect(screen.queryByTestId('new-booking-sheet')).toBeNull();
  });

  it('moves a booking with the keyboard (M, arrows, Enter)', async () => {
    const user = userEvent.setup();
    setup({
      [`GET ${API}/bookings/b1`]: () => json(200, detail(1)),
      [`PATCH ${API}/bookings/b1`]: () => json(200, change(false)),
    });
    const { grid } = await bookingBar();
    act(() => grid.focus());
    // Cursor starts on the first spot (A1, first day); one step right is the booking.
    await user.keyboard('{ArrowRight}');
    expect(screen.getByTestId('planning-announcer')).toHaveTextContent(/A1, .*: AR-0001 Rossi/);
    await user.keyboard('m{ArrowRight}');
    expect(screen.getByTestId('planning-move-preview')).toBeInTheDocument();
    await user.keyboard('{Enter}');
    const dialog = await screen.findByRole('alertdialog');
    expect(await within(dialog).findByTestId('move-dates')).toHaveTextContent(
      '16/07/2026 – 17/07/2026',
    );
  });

  it('selects free cells with Shift+arrows and Enter', async () => {
    const user = userEvent.setup();
    setup();
    const { grid } = await bookingBar();
    act(() => grid.focus());
    await user.keyboard('{ArrowDown}{Shift>}{ArrowRight}{/Shift}{Enter}');
    expect(JSON.parse((await screen.findByTestId('new-booking-sheet')).textContent ?? '')).toEqual({
      spotIds: ['s-a2'],
      startsOn: '2026-07-14',
      endsOn: '2026-07-15',
      slots: 'FULL',
    });
  });
});
