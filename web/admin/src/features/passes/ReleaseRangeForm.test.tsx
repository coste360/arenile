import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { EST_ID, json } from '../configuration/testing';
import { bodyOf, estPath, renderWithApi } from '../bookings/testing';
import { ReleaseRangeForm } from './ReleaseRangeForm';

const pass = {
  id: 'pass-1',
  starts_on: '2026-07-01',
  ends_on: '2026-08-31',
  slots: 'FULL' as const,
};

describe('ReleaseRangeForm (spec 30.8)', () => {
  it('releases every Monday of a range in one request', async () => {
    const user = userEvent.setup();
    const bodies: unknown[] = [];
    renderWithApi(<ReleaseRangeForm establishmentId={EST_ID} pass={pass} today="2026-06-20" />, {
      [`POST ${estPath('/season-passes/pass-1/releases')}`]: async (req) => {
        bodies.push(await bodyOf(req));
        return json(201, { items: [] });
      },
    });
    await user.click(screen.getByRole('button', { name: /^Periodo/ }));
    await user.click(await screen.findByRole('button', { name: /(^|\s)1 luglio 2026/ }));
    await user.click(screen.getByRole('button', { name: /(^|\s)31 luglio 2026/ }));
    await user.keyboard('{Escape}');
    await user.click(screen.getByRole('button', { name: 'lunedì' }));
    expect(screen.getByRole('status')).toHaveTextContent('4 giorni da rilasciare');
    await user.click(screen.getByRole('button', { name: 'Rilascia il periodo' }));
    await waitFor(() => expect(bodies).toHaveLength(1));
    expect(bodies[0]).toEqual({
      dates: [],
      from: '2026-07-01',
      to: '2026-07-31',
      weekdays: [1],
      slots: 'FULL',
    });
  });
});
