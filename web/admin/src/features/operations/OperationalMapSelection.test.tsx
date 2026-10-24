import { fireEvent, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';
import { EST_ID, baseRoutes, callsTo, json, renderApp } from '../configuration/testing';
import { opBooking, opDay, opSpot } from './testing/fixtures';
import { API, operationsRoutes } from './testing/routes';

const PATH = `/admin/e/${EST_ID}/mappa?date=2026-07-14&slot=FULL`;

// A1..A3 free, A4 booked: then A4 blocked by hand (block_id) in another day.
const freeDay = opDay([
  opSpot('s1', 'A1', 'free'),
  opSpot('s2', 'A2', 'free'),
  opSpot('s3', 'A3', 'free'),
  opSpot('s4', 'A4', 'booked', { booking: opBooking() }),
]);
const blockedDay = opDay([
  opSpot('s1', 'A1', 'free'),
  opSpot('s2', 'A2', 'free'),
  opSpot('s3', 'A3', 'free'),
  opSpot('s4', 'A4', 'blocked', { block_id: 'blk1', block_reason: 'Manutenzione' }),
]);

function setup(day = freeDay, extra: Parameters<typeof operationsRoutes>[1] = {}) {
  return renderApp(PATH, { ...baseRoutes('cashier'), ...operationsRoutes(day, extra) });
}

const spot = (label: string) =>
  screen.getByRole('button', { name: new RegExp(`Postazione ${label}`) });

describe('OperationalMapView multi-selection (ADR 0019 C)', () => {
  beforeAll(() => {
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(new Date('2026-07-14T10:00:00'));
  });
  afterAll(() => {
    vi.useRealTimers();
  });

  it('selects with taps and a Shift range, skips occupied spots and announces the count', async () => {
    const user = userEvent.setup();
    setup();
    await screen.findByRole('button', { name: /Postazione A1/ });
    await user.click(screen.getByRole('button', { name: 'Seleziona più postazioni' }));
    const bar = screen.getByTestId('selection-bar');
    expect(within(bar).getByRole('status')).toHaveTextContent('Nessuna postazione selezionata');
    await user.click(spot('A1'));
    fireEvent.click(spot('A3'), { shiftKey: true });
    expect(within(bar).getByRole('status')).toHaveTextContent('3 postazioni selezionate');
    for (const l of ['A1', 'A2', 'A3']) expect(spot(l)).toHaveAttribute('aria-pressed', 'true');
    // A booked spot cannot join the selection and opens no sheet in this mode.
    await user.click(spot('A4'));
    expect(spot('A4')).toHaveAttribute('aria-pressed', 'false');
    expect(screen.queryByRole('dialog')).toBeNull();
    // A second tap deselects.
    await user.click(spot('A2'));
    expect(within(bar).getByRole('status')).toHaveTextContent('2 postazioni selezionate');
    await user.click(within(bar).getByRole('button', { name: 'Esci dalla selezione' }));
    expect(screen.queryByTestId('selection-bar')).toBeNull();
    expect(spot('A1')).toHaveAttribute('aria-pressed', 'false');
  });

  it('Shift-click on a free spot starts the selection without the mode toggle', async () => {
    setup();
    await screen.findByRole('button', { name: /Postazione A1/ });
    fireEvent.click(spot('A2'), { shiftKey: true });
    expect(screen.getByTestId('selection-bar')).toHaveTextContent('1 postazione selezionata');
    expect(screen.getByRole('button', { name: 'Seleziona più postazioni' })).toHaveAttribute(
      'aria-pressed',
      'true',
    );
  });

  it('opens the group booking sheet with every selected spot', async () => {
    const user = userEvent.setup();
    setup();
    await screen.findByRole('button', { name: /Postazione A1/ });
    await user.click(screen.getByRole('button', { name: 'Seleziona più postazioni' }));
    await user.click(spot('A1'));
    await user.click(spot('A2'));
    await user.click(spot('A3'));
    await user.click(screen.getByRole('button', { name: 'Prenotazione di gruppo' }));
    expect(
      await screen.findByRole('dialog', { name: 'Prenotazione di gruppo (3 postazioni)' }),
    ).toBeInTheDocument();
  });

  it('blocks the selected spots with reason, period and slot, with an idempotency key', async () => {
    const user = userEvent.setup();
    let body: unknown;
    let key: string | null = null;
    const { fetchMock } = setup(freeDay, {
      [`POST ${API}/spot-blocks`]: async (req) => {
        body = await req.json();
        key = req.headers.get('Idempotency-Key');
        return json(201, { items: [] });
      },
    });
    await screen.findByRole('button', { name: /Postazione A1/ });
    await user.click(screen.getByRole('button', { name: 'Seleziona più postazioni' }));
    await user.click(spot('A1'));
    await user.click(spot('A2'));
    await user.click(
      within(screen.getByTestId('selection-bar')).getByRole('button', { name: 'Blocca' }),
    );
    const dialog = await screen.findByRole('dialog', { name: 'Blocca 2 postazioni' });
    await user.type(within(dialog).getByLabelText(/Motivo/), 'Manutenzione');
    await user.click(within(dialog).getByRole('radio', { name: 'Pomeriggio' }));
    await user.click(within(dialog).getByRole('button', { name: 'Blocca' }));
    await waitFor(() => expect(callsTo(fetchMock, 'POST', `${API}/spot-blocks`)).toHaveLength(1));
    expect(body).toEqual({
      spot_ids: ['s1', 's2'],
      starts_on: '2026-07-14',
      ends_on: '2026-07-14',
      slots: 'PM',
      reason: 'Manutenzione',
    });
    expect(key).toBeTruthy();
    // Done: dialog closed, selection mode left.
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(screen.queryByTestId('selection-bar')).toBeNull();
  });

  it('shows the conflict when a spot is occupied on one of the days', async () => {
    const user = userEvent.setup();
    setup(freeDay, {
      [`POST ${API}/spot-blocks`]: () =>
        json(409, { type: 'spot_unavailable', title: 'Postazione non disponibile', status: 409 }),
    });
    await user.click(await screen.findByRole('button', { name: /Postazione A1/ }));
    const sheet = await screen.findByRole('dialog');
    await user.click(within(sheet).getByRole('button', { name: 'Blocca postazione' }));
    const dialog = await screen.findByRole('dialog', { name: 'Blocca la postazione A1' });
    await user.click(within(dialog).getByRole('button', { name: 'Blocca' }));
    expect(await within(dialog).findByRole('alert')).toBeInTheDocument();
  });

  it('removes a manual block from the spot sheet', async () => {
    const user = userEvent.setup();
    const { fetchMock } = setup(blockedDay, {
      [`DELETE ${API}/spot-blocks/blk1`]: () => new Response(null, { status: 204 }),
    });
    await user.click(await screen.findByRole('button', { name: /Postazione A4/ }));
    const sheet = await screen.findByRole('dialog');
    expect(within(sheet).getByText('Bloccata: Manutenzione')).toBeInTheDocument();
    await user.click(within(sheet).getByRole('button', { name: 'Sblocca' }));
    const confirm = await screen.findByRole('alertdialog');
    await user.click(within(confirm).getByRole('button', { name: 'Sblocca' }));
    await waitFor(() =>
      expect(callsTo(fetchMock, 'DELETE', `${API}/spot-blocks/blk1`)).toHaveLength(1),
    );
  });
});
