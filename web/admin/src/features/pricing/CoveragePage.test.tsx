import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { EST_ID, baseRoutes, callsTo, json, list, renderApp } from '../configuration/testing';

const API = `/api/admin/v1/establishments/${EST_ID}`;
const ts = { created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' };

describe('CoveragePage', () => {
  it('shows gaps on the calendar and creates the missing rule prefilled', async () => {
    const user = userEvent.setup();
    let body: Record<string, unknown> | undefined;
    const { fetchMock } = renderApp(`/admin/e/${EST_ID}/pricing/coverage`, {
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
            allowed_formulas: ['day'],
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
      [`GET ${API}/pricing/coverage`]: () =>
        json(200, {
          season_id: 's1',
          from: '2027-06-01',
          to: '2027-06-10',
          complete: false,
          gaps: [
            {
              pack_id: 'p1',
              price_category: 'row1',
              channel: 'desk',
              from: '2027-06-05',
              to: '2027-06-06',
            },
          ],
        }),
      [`POST ${API}/price-rules`]: async (req) => {
        body = (await req.json()) as Record<string, unknown>;
        return json(201, { ...body, id: 'r1', seq: 1, version: 1, ...ts }, { ETag: '"1"' });
      },
    });

    expect(await screen.findByText('1 intervallo senza prezzo')).toBeInTheDocument();
    const calendar = screen.getAllByRole('listitem');
    const gapDays = calendar.filter((d) => Number(d.getAttribute('data-gaps') ?? 0) > 0);
    expect(gapDays.map((d) => d.textContent)).toEqual(['5', '6']);

    await user.click(screen.getByRole('button', { name: /Crea la regola mancante/ }));
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByLabelText('Nome')).toHaveValue('Prezzo Ombrellone + 2 lettini row1');
    await user.type(within(dialog).getByLabelText('Importo (€)'), '25');
    expect(within(dialog).getByTestId('rule-preview')).toHaveTextContent(
      /Base per Ombrellone \+ 2 lettini: 25,00\s€ al giorno, quando dal 5 giu 2027 al 6 giu 2027, categorie: row1 e canale: Cassa\./,
    );
    await user.click(within(dialog).getByRole('button', { name: 'Salva' }));
    await waitFor(() => expect(callsTo(fetchMock, 'POST', `${API}/price-rules`)).toHaveLength(1));
    expect(body).toMatchObject({
      season_id: 's1',
      stage: 'base',
      target_type: 'pack',
      target_ids: ['p1'],
      action: { type: 'set', amount: 2500 },
      conditions: {
        date_ranges: [{ from: '2027-06-05', to: '2027-06-06' }],
        price_categories: ['row1'],
        channels: ['desk'],
      },
    });
  });
});
