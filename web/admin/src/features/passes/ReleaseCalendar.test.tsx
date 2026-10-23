import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { EST_ID, json } from '../configuration/testing';
import { bodyOf, estPath, renderWithApi } from '../bookings/testing';
import type { SeasonPass, SeasonRelease } from './api';
import { ReleaseCalendar } from './ReleaseCalendar';

const pass: SeasonPass = {
  id: 'pass-1',
  code: 'AB-0001',
  status: 'confirmed',
  customer_id: 'c1',
  customer_display_name: 'Rossi Mario',
  season_id: 's1',
  spot_id: 'sp1',
  spot_label: 'A12',
  pack_id: 'p1',
  starts_on: '2026-07-01',
  ends_on: '2026-08-31',
  slots: 'FULL',
  total_amount: 120000,
  paid_amount: 120000,
  balance_due: 0,
  released_days: 3,
  credit_amount: 1500,
  version: 1,
  created_at: '2026-05-01T00:00:00Z',
};

const releases: SeasonRelease[] = [
  {
    id: 'r1',
    pass_id: 'pass-1',
    date: '2026-07-20',
    slot: 'AM',
    status: 'released',
    credit_amount: 0,
    created_at: '2026-07-01T00:00:00Z',
  },
  {
    id: 'r2',
    pass_id: 'pass-1',
    date: '2026-07-21',
    slot: 'AM',
    status: 'resold',
    credit_amount: 1500,
    created_at: '2026-07-01T00:00:00Z',
  },
  {
    id: 'r3',
    pass_id: 'pass-1',
    date: '2026-07-21',
    slot: 'PM',
    status: 'released',
    credit_amount: 0,
    created_at: '2026-07-01T00:00:00Z',
  },
];

const RELEASES = estPath('/season-passes/pass-1/releases');

function setup(canWrite = true) {
  const utils = renderWithApi(
    <ReleaseCalendar
      establishmentId={EST_ID}
      pass={pass}
      releases={releases}
      canWrite={canWrite}
      today="2026-07-15"
    />,
    {
      [`POST ${RELEASES}`]: () => json(201, { items: [] }),
      [`DELETE ${RELEASES}/r1`]: () => json(204, null),
    },
  );
  return { ...utils, user: userEvent.setup() };
}

const day = (name: string) => screen.getByRole('button', { name: new RegExp(name) });

describe('ReleaseCalendar', () => {
  it('selects days, switches the half and releases them', async () => {
    const { user, fetchMock } = setup();
    expect(screen.getByText(/3 mezze giornate rilasciate/)).toBeInTheDocument();
    expect(screen.getByText('Credito maturato: 15,00 €')).toBeInTheDocument();

    // Past days and days already (partly) released are not selectable for the full day.
    expect(day('10 luglio 2026')).toBeDisabled();
    expect(day('20 luglio 2026')).toBeDisabled();
    expect(day('21 luglio 2026')).toBeDisabled();

    await user.click(day('24 luglio 2026'));
    await user.click(day('25 luglio 2026'));
    await user.click(day('25 luglio 2026')); // toggled off
    await user.click(day('26 luglio 2026'));
    expect(screen.getByRole('button', { name: 'Rilascia 2 giorni' })).toBeEnabled();

    // Afternoon only: 20 July becomes releasable (only its morning is released).
    await user.click(screen.getByRole('radio', { name: 'Pomeriggio' }));
    expect(day('20 luglio 2026')).toBeEnabled();
    await user.click(day('20 luglio 2026'));

    await user.type(screen.getByLabelText('Nota (facoltativa)'), 'ferie');
    await user.click(screen.getByRole('button', { name: 'Rilascia 3 giorni' }));
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Rilascia 0 giorni' })).toBeDisabled(),
    );
    const req = fetchMock.mock.calls.map((c) => c[0] as Request).find((r) => r.method === 'POST')!;
    expect(await bodyOf(req)).toEqual({
      dates: ['2026-07-20', '2026-07-24', '2026-07-26'],
      slots: 'PM',
      note: 'ferie',
    });
    expect(req.headers.get('Idempotency-Key')).toBeTruthy();
  });

  it('lists active releases and revokes only those not resold', async () => {
    const { user, fetchMock } = setup();
    const list = screen.getByRole('region', { name: 'Giorni rilasciati' });
    const items = within(list).getAllByRole('listitem');
    expect(items).toHaveLength(3);
    expect(within(items[1]!).getByText('Rivenduto')).toBeInTheDocument();
    expect(within(items[1]!).queryByRole('button', { name: /Revoca/ })).toBeNull();

    await user.click(
      within(items[0]!).getByRole('button', {
        name: 'Revoca il rilascio del 20/07/2026 (Mattina)',
      }),
    );
    await user.click(await screen.findByRole('button', { name: 'Revoca' }));
    await waitFor(() =>
      expect(fetchMock.mock.calls.some((c) => (c[0] as Request).method === 'DELETE')).toBe(true),
    );
  });

  it('is read-only without write permission', () => {
    setup(false);
    expect(screen.queryByRole('button', { name: /Rilascia/ })).toBeNull();
    expect(screen.queryByRole('button', { name: /Revoca/ })).toBeNull();
    expect(day('24 luglio 2026')).toBeDisabled();
  });
});
