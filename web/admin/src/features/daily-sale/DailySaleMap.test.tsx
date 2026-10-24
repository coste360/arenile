import { fireEvent, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';
import {
  EST_ID,
  baseRoutes,
  callsTo,
  json,
  list,
  problem,
  renderApp,
} from '../configuration/testing';
import { opBooking, opDay, opSpot } from '../operations/testing/fixtures';
import { operationsRoutes } from '../operations/testing/routes';
import { API, saleRoutes, ticket } from './testing';

const PATH = `/admin/e/${EST_ID}/mappa?date=2026-07-14&slot=FULL`;

// A2: a walk-in ticket without a name (label "#037"); A3: one with a surname.
const day = opDay([
  opSpot('s1', 'A1', 'free'),
  opSpot('s2', 'A2', 'booked', {
    booking: opBooking({
      booking_id: 'b37',
      code: 'AR-7K3Q9',
      customer_display_name: '',
      occupant_label: '#037',
      ticket: { id: 't37', number: 37, label: '#037', needs_assignment: false, version: 1 },
    }),
  }),
  opSpot('s3', 'A3', 'booked', {
    booking: opBooking({
      booking_id: 'b38',
      code: 'AR-8Z',
      customer_display_name: 'Bianchi',
      occupant_label: 'Bianchi',
      ticket: { id: 't38', number: 38, label: '#038', needs_assignment: false, version: 1 },
    }),
  }),
  opSpot('s4', 'A4', 'free', { zone_id: 'z2' }),
]);

const unassigned = ticket({
  id: 't41',
  ticket_number: 41,
  number_label: '#041',
  booking_id: 'b41',
  spot_id: undefined,
  spot_label: undefined,
  needs_assignment: true,
  state: 'unassigned',
});

function setup(extra: Record<string, (r: Request) => Response> = {}) {
  return renderApp(PATH, {
    ...baseRoutes('cashier'),
    ...saleRoutes(),
    ...operationsRoutes(day, {
      [`GET ${API}/tickets/unassigned`]: () => list([unassigned]),
      ...extra,
    }),
  });
}

const captionOf = (spotId: string) =>
  [
    ...(document
      .querySelector(`[data-caption-for="${spotId}"]`)
      ?.querySelectorAll('[data-map-label="caption"]') ?? []),
  ].map((l) => l.textContent);

describe('daily sale on the day map', () => {
  beforeAll(() => {
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(new Date('2026-07-14T10:00:00'));
  });
  afterAll(() => vi.useRealTimers());

  it('labels sold spots with the ticket number, or the surname when given', async () => {
    setup();
    const a2 = await screen.findByRole('button', {
      name: /Postazione A2, fila A: .*occupato da #037/,
    });
    expect(a2).toBeInTheDocument();
    expect(captionOf('s2')).toEqual(['#037']);
    expect(captionOf('s3')).toEqual(['Bianchi']);
  });

  it('assigns a "da assegnare" ticket by tapping it, then a free spot of its zone', async () => {
    const user = userEvent.setup();
    const { fetchMock } = setup({
      [`POST ${API}/tickets/t41/assign`]: () => json(200, { ...unassigned, spot_id: 's1' }),
    });
    const panel = await screen.findByTestId('assign-panel');
    const chip = within(panel).getByRole('button', {
      name: 'Biglietto #041, Prima fila, Giornata intera',
    });
    await user.click(chip);
    expect(chip).toHaveAttribute('aria-pressed', 'true');
    expect(within(panel).getByRole('status')).toHaveTextContent(
      'Tocca una postazione evidenziata di Prima fila (sulla mappa o qui sotto) per il biglietto #041.',
    );
    // A4 is free but in another zone: refused locally, nothing sent.
    // Keyboard activation: a pointer tap on a small spot zooms in first.
    fireEvent.keyDown(screen.getByRole('button', { name: /Postazione A4/ }), { key: 'Enter' });
    expect(await screen.findByText('Scegli una postazione della zona Prima fila.')).toBeVisible();
    expect(callsTo(fetchMock, 'POST', `${API}/tickets/t41/assign`)).toHaveLength(0);
    // A1 is free in the zone: assigned atomically by the server, no spot sheet.
    // Keyboard activation: a pointer tap on a small spot zooms in first.
    fireEvent.keyDown(screen.getByRole('button', { name: /Postazione A1/ }), { key: 'Enter' });
    await waitFor(() =>
      expect(callsTo(fetchMock, 'POST', `${API}/tickets/t41/assign`)).toHaveLength(1),
    );
    const req = callsTo(fetchMock, 'POST', `${API}/tickets/t41/assign`)[0] as Request;
    expect(await req.clone().json()).toEqual({ spot_id: 's1' });
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(await screen.findByText('Biglietto #041 assegnato alla postazione A1')).toBeVisible();
  });

  it('offers the ticket its own provisional spot when the seat zone is full', async () => {
    const user = userEvent.setup();
    // Zone z1 full: A1 taken by a guest, A2 is the spot the sale parked #041 on.
    const full = opDay([
      opSpot('s1', 'A1', 'booked', { booking: opBooking({ booking_id: 'b1' }) }),
      opSpot('s2', 'A2', 'booked', {
        booking: opBooking({
          booking_id: 'b41',
          customer_display_name: '',
          occupant_label: '#041',
          ticket: { id: 't41', number: 41, label: '#041', needs_assignment: true, version: 1 },
        }),
      }),
      opSpot('s4', 'A4', 'free', { zone_id: 'z2' }),
    ]);
    const { fetchMock } = renderApp(PATH, {
      ...baseRoutes('cashier'),
      ...saleRoutes(),
      ...operationsRoutes(full, {
        [`GET ${API}/tickets/unassigned`]: () => list([unassigned]),
        [`POST ${API}/tickets/t41/assign`]: () =>
          json(200, { ...unassigned, spot_id: 's2', needs_assignment: false }),
      }),
    });
    // The parked ticket is drawn as "da assegnare", not as a guest on A2.
    expect(await screen.findByRole('button', { name: /Postazione A2/ })).toHaveAccessibleName(
      /Da assegnare/,
    );
    const panel = await screen.findByTestId('assign-panel');
    await user.click(within(panel).getByRole('button', { name: /Biglietto #041/ }));
    // Its own spot is the one target, offered as a large button next to the map.
    const targets = within(panel).getByTestId('assign-targets');
    expect(within(targets).getAllByRole('button')).toHaveLength(1);
    await user.click(within(targets).getByRole('button', { name: 'Assegna alla postazione A2' }));
    await waitFor(() =>
      expect(callsTo(fetchMock, 'POST', `${API}/tickets/t41/assign`)).toHaveLength(1),
    );
    const req = callsTo(fetchMock, 'POST', `${API}/tickets/t41/assign`)[0] as Request;
    expect(req.headers.get('If-Match')).toBe('"1"');
    expect(await req.clone().json()).toEqual({ spot_id: 's2' });
  });

  it('reports a spot taken in the meantime (409) and auto-assigns the rest', async () => {
    const user = userEvent.setup();
    const { fetchMock } = setup({
      [`POST ${API}/tickets/t41/assign`]: () => problem(409, 'assignment_conflict'),
      [`POST ${API}/tickets/auto-assign`]: () =>
        json(200, {
          applied: true,
          assigned: [{ ticket_id: 't41', ticket_number: 41, spot_id: 's1', spot_label: 'A1' }],
          unassigned: [],
        }),
    });
    const panel = await screen.findByTestId('assign-panel');
    await user.click(within(panel).getByRole('button', { name: /Biglietto #041/ }));
    // Keyboard activation: a pointer tap on a small spot zooms in first.
    fireEvent.keyDown(screen.getByRole('button', { name: /Postazione A1/ }), { key: 'Enter' });
    expect(
      await screen.findByText("La postazione è stata appena occupata: scegline un'altra."),
    ).toBeVisible();
    await user.click(within(panel).getByRole('button', { name: 'Assegna automaticamente' }));
    await waitFor(() =>
      expect(callsTo(fetchMock, 'POST', `${API}/tickets/auto-assign`)).toHaveLength(1),
    );
    const req = callsTo(fetchMock, 'POST', `${API}/tickets/auto-assign`)[0] as Request;
    expect(await req.clone().json()).toEqual({ date: '2026-07-14' });
    expect(await screen.findByText('1 biglietto assegnato')).toBeVisible();
  });

  it('opens "Vendi giornata" from a free spot with the spot preselected', async () => {
    const user = userEvent.setup();
    const { fetchMock } = setup();
    await user.click(await screen.findByRole('button', { name: /Postazione A1, fila A: Libero/ }));
    const sheet = await screen.findByRole('dialog');
    await user.click(within(sheet).getByRole('button', { name: 'Vendi giornata' }));
    const sale = await screen.findByRole('dialog', { name: 'Vendi giornata' });
    await waitFor(() => expect(within(sale).getByTestId('sale-total')).toHaveTextContent('30,00'));
    const quote = callsTo(fetchMock, 'POST', `${API}/daily-sales/quote`).at(-1) as Request;
    expect(await quote.clone().json()).toMatchObject({ spot_id: 's1', date: '2026-07-14' });
  });

  it('lists seat-mode zone tickets under Oggi > Da assegnare with a link to the map', async () => {
    const user = userEvent.setup();
    renderApp(`/admin/e/${EST_ID}/oggi?date=2026-07-14&slot=FULL`, {
      ...baseRoutes('cashier'),
      ...saleRoutes(),
      ...operationsRoutes(day, {
        [`GET ${API}/tickets/unassigned`]: () => list([{ ...unassigned, holder_name: 'Russo' }]),
      }),
    });
    const tab = await screen.findByRole('tab', { name: /Da assegnare/ });
    await waitFor(() => expect(tab).toHaveTextContent('1'));
    await user.click(tab);
    const panel = screen.getByRole('tabpanel', { name: /Da assegnare/ });
    expect(await within(panel).findByText('#041 · Russo')).toBeInTheDocument();
    expect(within(panel).getByText(/Prima fila/)).toBeInTheDocument();
    expect(
      within(panel).getByRole('link', { name: 'Assegna il biglietto #041 sulla mappa' }),
    ).toHaveAttribute('href', expect.stringContaining('/mappa'));
  });
});
