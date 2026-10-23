import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { EST_ID, baseRoutes, callsTo, json, renderApp } from '../configuration/testing';
import { opBooking, opDay, opSpot } from './testing/fixtures';
import { API, operationsRoutes } from './testing/routes';

const PATH = `/admin/e/${EST_ID}/mappa?date=2026-07-14&slot=FULL`;

const day = opDay([
  opSpot('s1', 'A1', 'free'),
  opSpot('s2', 'A2', 'booked', {
    booking: opBooking({ has_notes: true, customer_tags: ['vip'] }),
  }),
  opSpot('s3', 'A3', 'pass_holder', {
    booking: opBooking({
      booking_id: 'p1',
      line_id: 'pl1',
      code: 'AB-0007',
      customer_display_name: 'Verdi L.',
      is_season_pass: true,
    }),
  }),
  opSpot('s4', 'A4', 'blocked', { block_reason: 'Manutenzione' }),
]);

function setup(extra: Parameters<typeof operationsRoutes>[1] = {}) {
  return renderApp(PATH, { ...baseRoutes('cashier'), ...operationsRoutes(day, extra) });
}

describe('OperationalMapPage', () => {
  it('colours spots by status with the full legend and accessible names', async () => {
    const { fetchMock } = setup();
    const a2 = await screen.findByRole('button', { name: /Postazione A2: Prenotato in cassa/ });
    expect(a2).toHaveAttribute('data-status', 'booked_desk');
    expect(a2).toHaveAccessibleName(/Rossi M\., Nota, VIP/);
    expect(a2.querySelector('[data-indicator="vip"]')).not.toBeNull();
    expect(screen.getByRole('button', { name: /Postazione A1: Libero/ })).toHaveAttribute(
      'data-status',
      'free',
    );
    expect(screen.getByRole('button', { name: /Postazione A3: Abbonato/ })).toHaveAttribute(
      'data-status',
      'season',
    );
    // Legend: always every status, plus the indicators.
    const legend = document.querySelector('[data-slot="map-legend"]') as HTMLElement;
    expect(legend.querySelectorAll('[data-legend-status]')).toHaveLength(11);
    expect(within(legend).getByText('Da incassare')).toBeInTheDocument();
    expect(document.querySelector('[data-indicator-legend="vip"]')).not.toBeNull();
    // The day is requested with date and slot from the URL.
    const req = callsTo(fetchMock, 'GET', `${API}/operations/day`)[0] as Request;
    expect(new URL(req.url).searchParams.get('date')).toBe('2026-07-14');
    expect(new URL(req.url).searchParams.get('slot')).toBe('FULL');
  });

  it('opens the sheet of a booked spot and checks in', async () => {
    const user = userEvent.setup();
    const { fetchMock } = setup({
      [`POST ${API}/bookings/b1/check-in`]: () => json(200, { id: 'b1' }),
    });
    await user.click(await screen.findByRole('button', { name: /Postazione A2/ }));
    const sheet = await screen.findByRole('dialog');
    expect(within(sheet).getByText('Rossi M.')).toBeInTheDocument();
    expect(within(sheet).getByText('AR-0001')).toBeInTheDocument();
    expect(within(sheet).queryByRole('button', { name: 'Rilascia giorno' })).toBeNull();
    const before = callsTo(fetchMock, 'GET', `${API}/operations/day`).length;
    await user.click(within(sheet).getByRole('button', { name: 'Check-in' }));
    await waitFor(() =>
      expect(callsTo(fetchMock, 'POST', `${API}/bookings/b1/check-in`)).toHaveLength(1),
    );
    // The day query is invalidated and refetched (targeted, not a page reload).
    await waitFor(() =>
      expect(callsTo(fetchMock, 'GET', `${API}/operations/day`).length).toBeGreaterThan(before),
    );
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
  });

  it('shows the server problem when the check-in is refused', async () => {
    const user = userEvent.setup();
    setup({
      [`POST ${API}/bookings/b1/check-in`]: () =>
        json(409, {
          type: 'invalid_transition',
          title: 'invalid',
          status: 409,
          current_status: 'cancelled',
        }),
    });
    await user.click(await screen.findByRole('button', { name: /Postazione A2/ }));
    const sheet = await screen.findByRole('dialog');
    await user.click(within(sheet).getByRole('button', { name: 'Check-in' }));
    expect(
      await within(sheet).findByText(
        'Operazione non ammessa nello stato attuale della prenotazione.',
      ),
    ).toBeInTheDocument();
  });

  it('releases the day of a pass holder with an idempotency key', async () => {
    const user = userEvent.setup();
    let body: unknown;
    let key: string | null = null;
    const { fetchMock } = setup({
      [`POST ${API}/season-passes/p1/releases`]: async (req) => {
        body = await req.json();
        key = req.headers.get('Idempotency-Key');
        return json(201, { items: [] });
      },
    });
    await user.click(await screen.findByRole('button', { name: /Postazione A3/ }));
    const sheet = await screen.findByRole('dialog');
    await user.click(within(sheet).getByRole('button', { name: 'Rilascia giorno' }));
    const confirm = await screen.findByRole('alertdialog');
    expect(within(confirm).getByText(/Verdi L\. non verrà il 14\/07\/2026/)).toBeInTheDocument();
    await user.click(within(confirm).getByRole('button', { name: 'Rilascia giorno' }));
    await waitFor(() =>
      expect(callsTo(fetchMock, 'POST', `${API}/season-passes/p1/releases`)).toHaveLength(1),
    );
    expect(body).toEqual({ dates: ['2026-07-14'], slots: 'FULL' });
    expect(key).toBeTruthy();
  });

  it('moves a booking onto a free spot after a dry-run preview', async () => {
    const user = userEvent.setup();
    const moves: Record<string, unknown>[] = [];
    const result = (applied: boolean) => ({
      applied,
      booking: { id: 'b1' },
      previous_total: 3000,
      new_total: 3500,
      price_difference: 500,
      conflicts: [],
    });
    const { fetchMock } = setup({
      [`POST ${API}/bookings/b1/move`]: async (req) => {
        const b = (await req.json()) as Record<string, unknown>;
        moves.push(b);
        return json(200, result(!b.dry_run));
      },
    });
    await user.click(await screen.findByRole('button', { name: /Postazione A2/ }));
    await user.click(
      within(await screen.findByRole('dialog')).getByRole('button', { name: 'Sposta' }),
    );
    expect(await screen.findByText('Sposta da A2')).toBeInTheDocument();
    // An occupied target is ignored; the free one opens the confirmation.
    await user.click(screen.getByRole('button', { name: /Postazione A4/ }));
    expect(screen.queryByRole('alertdialog')).toBeNull();
    await user.click(screen.getByRole('button', { name: /Postazione A1/ }));
    const dialog = await screen.findByRole('alertdialog');
    expect(await within(dialog).findByTestId('move-price')).toHaveTextContent(
      'Differenza da incassare: 5,00 €',
    );
    await user.click(within(dialog).getByRole('button', { name: 'Sposta' }));
    await waitFor(() => expect(moves).toHaveLength(2));
    expect(moves[0]).toEqual({ line_id: 'l1', spot_id: 's1', dry_run: true });
    expect(moves[1]).toEqual({ line_id: 'l1', spot_id: 's1', dry_run: false });
    await waitFor(() => expect(screen.queryByText('Sposta da A2')).toBeNull());
    expect(callsTo(fetchMock, 'POST', `${API}/bookings/b1/move`)).toHaveLength(2);
  });

  it('highlights search matches and announces the count', async () => {
    const user = userEvent.setup();
    setup();
    await screen.findByRole('button', { name: /Postazione A2/ });
    await user.type(screen.getByRole('searchbox', { name: 'Cerca sulla mappa' }), 'verdi');
    expect(await screen.findByText('1 postazione trovata')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Postazione A3/ })).toHaveAttribute(
      'data-highlighted',
      'true',
    );
    expect(screen.getByRole('button', { name: /Postazione A1/ })).toHaveAttribute(
      'data-highlighted',
      'false',
    );
  });

  it('changes slot through the URL', async () => {
    const user = userEvent.setup();
    const { history, fetchMock } = setup();
    await screen.findByRole('button', { name: /Postazione A2/ });
    await user.click(screen.getByRole('radio', { name: 'Mattina' }));
    await waitFor(() => expect(history.location.search).toContain('slot=AM'));
    await waitFor(() =>
      expect(
        callsTo(fetchMock, 'GET', `${API}/operations/day`).some(
          (r) => new URL(r.url).searchParams.get('slot') === 'AM',
        ),
      ).toBe(true),
    );
  });
});
