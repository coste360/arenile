import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { EST_ID, baseRoutes, list, renderApp } from '../configuration/testing';

const API = `/api/admin/v1/establishments/${EST_ID}`;
const ts = { created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' };

describe('FixedPricesTab', () => {
  it('offers "Nuovo prezzo fisso" when packs exist but no price category is assigned yet', async () => {
    const user = userEvent.setup();
    renderApp(`/admin/e/${EST_ID}/pricing`, {
      ...baseRoutes('owner'),
      [`GET ${API}/seasons`]: () =>
        list([
          {
            id: 's1',
            year: 2027,
            opens_on: '2027-06-01',
            closes_on: '2027-06-10',
            status: 'active',
            version: 1,
            ...ts,
          },
        ]),
      [`GET ${API}/rate-periods`]: () => list([]),
      [`GET ${API}/packs`]: () =>
        list([
          {
            id: 'p1',
            code: 'base',
            name: { it: 'Ombrellone + 2 lettini' },
            composition: { umbrella: 1, sunbed: 2 },
            allowed_formulas: ['day', 'week'],
            channels: ['desk'],
            max_per_booking: 4,
            requires_approval: false,
            sort_order: 0,
            active: true,
            version: 1,
            ...ts,
          },
        ]),
      [`GET ${API}/extras`]: () => list([]),
      [`GET ${API}/zones`]: () => list([]),
      [`GET ${API}/agreements`]: () => list([]),
      [`GET ${API}/promo-codes`]: () => list([]),
      [`GET ${API}/price-rules`]: () => list([]),
      [`GET ${API}/fixed-prices`]: () => list([]),
    });
    await user.click(await screen.findByRole('tab', { name: 'Prezzi a durata fissa' }));
    expect(await screen.findByText(/Nessuna categoria di prezzo/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Nuovo prezzo fisso' })).toBeInTheDocument();
  });
});
