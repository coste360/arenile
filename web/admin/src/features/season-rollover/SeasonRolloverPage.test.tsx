import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { bodyOf, estPath } from '../bookings/testing';
import {
  baseRoutes,
  callsTo,
  EST_ID,
  json,
  list,
  problem,
  renderApp,
  type Handler,
} from '../configuration/testing';

const ts = { version: 1, created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' };
const S2026 = {
  id: 's2026',
  establishment_id: EST_ID,
  year: 2026,
  opens_on: '2026-05-15',
  closes_on: '2026-09-20',
  status: 'active',
  ...ts,
};
const COUNTS = { rate_periods: 3, closure_days: 2, price_rules: 14, fixed_prices: 5, policies: 1 };

function result(overrides: Record<string, unknown> = {}) {
  return {
    dry_run: true,
    already_applied: false,
    source_season_id: 's2026',
    target_year: 2027,
    opens_on: '2027-05-15',
    closes_on: '2027-09-20',
    counts: COUNTS,
    season_passes: 42,
    warnings: [
      { code: 'closure_outside_season', date: '2026-09-20', target_date: '2027-09-19' },
      { code: 'season_copy', text: 'Il periodo «Ferragosto» cade di sabato nel 2027.' },
    ],
    ...overrides,
  };
}

function routes(extra: Record<string, Handler> = {}): Record<string, Handler> {
  return {
    ...baseRoutes('manager'),
    [`GET ${estPath('/seasons')}`]: () => list([S2026]),
    [`GET ${estPath('/season-rollovers')}`]: () => list([]),
    ...extra,
  };
}

const PAGE = `/e/${EST_ID}/config/seasons/rollover?seasonId=s2026`;

describe('SeasonRolloverPage', () => {
  it('previews the copy, applies it once and leads to the renewals', async () => {
    const { fetchMock } = renderApp(
      PAGE,
      routes({
        [`POST ${estPath('/seasons/s2026/rollover')}`]: async (req) => {
          const body = (await bodyOf(req)) as { dry_run: boolean };
          return body.dry_run
            ? json(200, result())
            : json(
                201,
                result({
                  dry_run: false,
                  rollover_id: 'r1',
                  season: { ...S2026, id: 's2027', year: 2027, status: 'draft' },
                }),
              );
        },
      }),
    );
    const user = userEvent.setup();
    expect(await screen.findByRole('heading', { name: 'Nuova stagione' })).toBeInTheDocument();
    expect(await screen.findByLabelText('Anno della nuova stagione')).toHaveValue('2027');
    // The dates follow the target year.
    expect(screen.getByRole('button', { name: /Apertura/ })).toHaveTextContent('15/05/2027');
    await user.click(screen.getByRole('switch', { name: 'Giorni di chiusura' }));
    await user.click(screen.getByRole('button', { name: 'Calcola anteprima' }));

    expect(await screen.findByText('3. Anteprima della stagione 2027')).toBeInTheDocument();
    expect(
      screen.getByText('Abbonati da rinnovare').parentElement?.parentElement,
    ).toHaveTextContent('42');
    expect(screen.getByText(/Ferragosto/)).toBeInTheDocument();
    // Structured warnings are translated by the panel (dates localised).
    expect(
      screen.getByText(
        'Chiusura del 20/09/2026: la data spostata (19/09/2027) cade fuori dalla nuova stagione e non è stata copiata.',
      ),
    ).toBeInTheDocument();
    const [preview] = callsTo(fetchMock, 'POST', estPath('/seasons/s2026/rollover'));
    expect(await bodyOf(preview!)).toEqual({
      target_year: 2027,
      opens_on: '2027-05-15',
      closes_on: '2027-09-20',
      include_rate_periods: true,
      include_closure_days: false,
      include_price_rules: true,
      include_fixed_prices: true,
      include_policies: true,
      dry_run: true,
    });

    await user.click(screen.getByRole('button', { name: 'Crea la stagione 2027' }));
    const confirm = await screen.findByRole('alertdialog', { name: 'Creare la stagione 2027?' });
    await user.click(within(confirm).getByRole('button', { name: 'Crea la stagione' }));
    expect(await screen.findByText('Stagione 2027 creata')).toBeInTheDocument();
    const [, applied] = callsTo(fetchMock, 'POST', estPath('/seasons/s2026/rollover'));
    expect(await bodyOf(applied!)).toMatchObject({ dry_run: false, include_closure_days: false });
    expect(applied!.headers.get('Idempotency-Key')).toBeTruthy();
    const renewals = screen.getByRole('link', { name: /Proponi i rinnovi a 42 abbonati/ });
    expect(renewals.getAttribute('href')).toContain(`/e/${EST_ID}/rinnovi?`);
    expect(renewals.getAttribute('href')).toContain('target=s2027');
    expect(screen.getByRole('link', { name: 'Apri la stagione' }).getAttribute('href')).toContain(
      'seasonId=s2027',
    );
  });

  it('explains a conflicting season of the target year', async () => {
    renderApp(
      PAGE,
      routes({
        [`POST ${estPath('/seasons/s2026/rollover')}`]: () =>
          problem(409, 'season_rollover_conflict'),
      }),
    );
    const user = userEvent.setup();
    await user.click(await screen.findByRole('button', { name: 'Calcola anteprima' }));
    expect(await screen.findByRole('alert')).toHaveTextContent(
      "Esiste già una stagione per quell'anno",
    );
  });

  it('lists the rollovers already run', async () => {
    renderApp(
      PAGE,
      routes({
        [`GET ${estPath('/season-rollovers')}`]: () =>
          list([
            {
              id: 'r1',
              source_season_id: 's2026',
              target_season_id: 's2027',
              target_year: 2027,
              counts: COUNTS,
              created_at: '2026-10-20T08:00:00Z',
            },
          ]),
      }),
    );
    const table = await screen.findByRole('table', { name: 'Passaggi già eseguiti' });
    await waitFor(() =>
      expect(within(table).getAllByRole('row')[1]).toHaveTextContent(
        /2026 → 2027.*3 periodi, 14 regole, 5 prezzi fissi/,
      ),
    );
  });
});
