import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
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
import { storageKey } from './progress';

const API = `/api/admin/v1/establishments/${EST_ID}`;
const PATH = `/admin/e/${EST_ID}/setup`;
const ts = { created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' };

const EMPTY_STATUS = {
  has_season: false,
  has_rate_periods: false,
  has_map: false,
  spot_count: 0,
  has_catalog: false,
  has_prices: false,
  booking_enabled: false,
  price_categories: [] as string[],
  has_operations: false,
};

const TEMPLATES = [
  {
    code: 'classic_rows',
    name: { it: 'Lido classico a file' },
    description: { it: 'File numerate.' },
    rows: 8,
    spots_per_row: 15,
    zones: [
      { code: 'row1', name: { it: 'Fila 1' }, assignment_mode: 'seat', price_category: 'row1' },
    ],
    packs: [{ code: 'base', name: { it: 'Base' }, composition: { umbrella: 1, sunbed: 2 } }],
    rate_periods: ['low', 'mid', 'high', 'peak'],
  },
];

function routes(extra: Record<string, Handler> = {}, status = EMPTY_STATUS) {
  return {
    ...baseRoutes('manager'),
    [`GET ${API}/setup/status`]: () => json(200, status),
    [`GET ${API}/setup/templates`]: () => json(200, { items: TEMPLATES }),
    ...extra,
  };
}

function stepHeading() {
  return screen.findByRole('heading', { level: 2, name: /^Passo/ });
}

beforeEach(() => window.localStorage.clear());
afterEach(() => window.localStorage.clear());

describe('SetupWizardPage', () => {
  it('navigates with skip and back; every step is skippable', async () => {
    const user = userEvent.setup();
    renderApp(PATH, routes());
    expect(await stepHeading()).toHaveTextContent('Passo 1 di 6: Dati e lingue');
    await user.click(screen.getByRole('button', { name: 'Salta questo passo' }));
    expect(await stepHeading()).toHaveTextContent('Passo 2 di 6: Modello');
    // The four templates of spec 15.2, server texts when available.
    const templates = screen.getByRole('radiogroup', { name: 'Modelli disponibili' });
    expect(within(templates).getAllByRole('radio')).toHaveLength(4);
    expect(within(templates).getByText('8 file × 15 posti')).toBeInTheDocument();
    expect(within(templates).getByText('Lido con piscina')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Indietro' }));
    expect(await stepHeading()).toHaveTextContent('Passo 1 di 6');
    const nav = screen.getByRole('navigation', { name: 'Passi della configurazione' });
    expect(within(nav).getByRole('button', { name: /Dati e lingue/ })).toHaveAttribute(
      'aria-current',
      'step',
    );
    expect(within(nav).getByRole('button', { name: /Modello/ })).toHaveTextContent('(da fare)');

    for (let i = 0; i < 5; i++)
      await user.click(screen.getByRole('button', { name: 'Salta questo passo' }));
    await user.click(screen.getByRole('button', { name: 'Concludi la configurazione' }));
    expect(
      await screen.findByRole('heading', { name: 'Configurazione completata' }),
    ).toBeInTheDocument();
    expect(screen.getByText(/Passi saltati: /)).toBeInTheDocument();
  });

  it('applies the template with 8 × 15 rows and generates the quick price list in cents', async () => {
    const user = userEvent.setup();
    const applyBodies: unknown[] = [];
    const quickBodies: unknown[] = [];
    let applied = false;
    const { fetchMock } = renderApp(
      PATH,
      routes(
        {
          [`POST ${API}/setup/apply-template`]: async (req) => {
            applyBodies.push(await req.json());
            applied = true;
            return json(201, {
              template: 'classic_rows',
              season_id: 's1',
              map_id: 'm1',
              zone_ids: ['z1', 'z2'],
              pool_ids: [],
              component_ids: ['c1', 'c2'],
              pack_ids: ['p1'],
              extra_ids: [],
              rate_period_ids: ['r1', 'r2'],
              spots_created: 120,
            });
          },
          [`GET ${API}/seasons`]: () =>
            list(
              applied
                ? [
                    {
                      id: 's1',
                      establishment_id: EST_ID,
                      year: 2027,
                      opens_on: '2027-05-01',
                      closes_on: '2027-09-30',
                      status: 'draft',
                      version: 1,
                      ...ts,
                    },
                  ]
                : [],
            ),
          [`GET ${API}/zones`]: () =>
            list([
              {
                id: 'z1',
                code: 'row1',
                name: { it: 'Fila 1' },
                assignment_mode: 'seat',
                online_visible: true,
                price_category: 'row1',
                auto_assign: false,
                sort_order: 0,
                spot_count: 15,
                version: 1,
                ...ts,
              },
              {
                id: 'z2',
                code: 'row2',
                name: { it: 'Fila 2' },
                assignment_mode: 'seat',
                online_visible: true,
                price_category: 'row2',
                auto_assign: false,
                sort_order: 1,
                spot_count: 15,
                version: 1,
                ...ts,
              },
            ]),
          [`GET ${API}/rate-periods`]: () =>
            list([
              {
                id: 'r1',
                season_id: 's1',
                code: 'low',
                name: { it: 'Bassa' },
                color: '#7dd3fc',
                ranges: [],
                sort_order: 0,
                version: 1,
                ...ts,
              },
              {
                id: 'r2',
                season_id: 's1',
                code: 'high',
                name: { it: 'Alta' },
                color: '#fb923c',
                ranges: [],
                sort_order: 1,
                version: 1,
                ...ts,
              },
            ]),
          [`POST ${API}/setup/quick-pricelist`]: async (req) => {
            quickBodies.push(await req.json());
            return json(201, { rules: [{}, {}, {}], replaced: 0 });
          },
        },
        { ...EMPTY_STATUS, price_categories: ['angolo', 'row1', 'row2'] },
      ),
    );
    await stepHeading();
    await user.click(screen.getByRole('button', { name: 'Salta questo passo' }));
    await user.click(await screen.findByRole('radio', { name: /^Lido classico a file/ }));
    await user.click(screen.getByRole('button', { name: /Usa questo modello/ }));
    expect(await stepHeading()).toHaveTextContent('Passo 3 di 6: File sulla mappa');
    expect(screen.getByLabelText('File')).toHaveValue('8');
    expect(screen.getByLabelText('Posti per fila')).toHaveValue('15');
    expect(screen.getByText('120 postazioni')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Crea stagione e mappa' }));
    expect(await screen.findByText('Configurazione creata.')).toBeInTheDocument();
    expect(applyBodies[0]).toMatchObject({ template: 'classic_rows', rows: 8, spots_per_row: 15 });
    expect(
      callsTo(fetchMock, 'POST', `${API}/setup/apply-template`)[0]?.headers.get('Idempotency-Key'),
    ).toBeTruthy();
    await user.click(screen.getByRole('button', { name: 'Continua' }));

    expect(await stepHeading()).toHaveTextContent('Passo 4 di 6: Listino rapido');
    // The grid has a row per zone category and per spot override category
    // reported by the status (an override has no zone to take it from).
    expect(await screen.findByLabelText('Prezzo giornaliero angolo, periodo Bassa')).toBeVisible();
    await user.type(await screen.findByLabelText('Prezzo giornaliero row1, periodo Bassa'), '25');
    await user.type(screen.getByLabelText('Prezzo giornaliero row1, periodo Alta'), '32,50');
    await user.type(screen.getByLabelText('Prezzo giornaliero row2, periodo Alta'), '28.9');
    await user.click(screen.getByRole('button', { name: 'Genera le regole di prezzo' }));
    expect(
      await screen.findByText('Create 3 regole di prezzo (sostituite: 0).'),
    ).toBeInTheDocument();
    expect(quickBodies[0]).toEqual({
      season_id: 's1',
      replace_existing: true,
      prices: [
        { price_category: 'row1', rate_period_code: 'low', amount: 2500 },
        { price_category: 'row1', rate_period_code: 'high', amount: 3250 },
        { price_category: 'row2', rate_period_code: 'high', amount: 2890 },
      ],
    });
  });

  it('rejects invalid euro amounts before calling the API', async () => {
    const user = userEvent.setup();
    window.localStorage.setItem(
      storageKey(EST_ID),
      JSON.stringify({
        v: 1,
        current: 3,
        states: {
          details: 'done',
          template: 'done',
          rows: 'done',
          pricelist: 'pending',
          publicPage: 'pending',
          payments: 'pending',
        },
        seasonId: 's1',
        finished: false,
      }),
    );
    const { fetchMock } = renderApp(
      PATH,
      routes({
        [`GET ${API}/seasons`]: () =>
          list([
            {
              id: 's1',
              establishment_id: EST_ID,
              year: 2027,
              opens_on: '2027-05-01',
              closes_on: '2027-09-30',
              status: 'draft',
              version: 1,
              ...ts,
            },
          ]),
        [`GET ${API}/zones`]: () =>
          list([
            {
              id: 'z1',
              code: 'row1',
              name: { it: 'Fila 1' },
              assignment_mode: 'seat',
              online_visible: true,
              price_category: 'row1',
              auto_assign: false,
              sort_order: 0,
              spot_count: 15,
              version: 1,
              ...ts,
            },
          ]),
        [`GET ${API}/rate-periods`]: () =>
          list([
            {
              id: 'r1',
              season_id: 's1',
              code: 'low',
              name: { it: 'Bassa' },
              color: '#7dd3fc',
              ranges: [],
              sort_order: 0,
              version: 1,
              ...ts,
            },
          ]),
      }),
    );
    const cell = await screen.findByLabelText('Prezzo giornaliero row1, periodo Bassa');
    await user.type(cell, '12,345');
    await user.click(screen.getByRole('button', { name: 'Genera le regole di prezzo' }));
    expect(
      await screen.findByText('Inserisci un importo valido, ad esempio 25 o 25,50'),
    ).toBeInTheDocument();
    expect(cell).toHaveAttribute('aria-invalid', 'true');
    expect(callsTo(fetchMock, 'POST', `${API}/setup/quick-pricelist`)).toHaveLength(0);
  });

  it('resumes from the saved progress when re-entering', async () => {
    const user = userEvent.setup();
    const first = renderApp(PATH, routes());
    await stepHeading();
    await user.click(screen.getByRole('button', { name: 'Salta questo passo' }));
    await user.click(screen.getByRole('button', { name: 'Salta questo passo' }));
    expect(await stepHeading()).toHaveTextContent('Passo 3 di 6');
    first.unmount();

    renderApp(PATH, routes());
    expect(await stepHeading()).toHaveTextContent('Passo 3 di 6');
    const nav = screen.getByRole('navigation', { name: 'Passi della configurazione' });
    expect(within(nav).getByRole('button', { name: /Modello/ })).toHaveTextContent('(saltato)');
  });

  it('without saved progress resumes from the server status', async () => {
    renderApp(
      PATH,
      routes(
        {},
        { ...EMPTY_STATUS, has_season: true, has_map: true, spot_count: 120, has_catalog: true },
      ),
    );
    expect(await stepHeading()).toHaveTextContent('Passo 4 di 6: Listino rapido');
  });

  it('still works when the status endpoint fails', async () => {
    renderApp(PATH, routes({ [`GET ${API}/setup/status`]: () => problem(501, 'not_implemented') }));
    expect(await stepHeading()).toHaveTextContent('Passo 1 di 6');
  });

  it('shows the payments step with the link to set up Stripe', async () => {
    const user = userEvent.setup();
    renderApp(PATH, routes());
    await stepHeading();
    const nav = screen.getByRole('navigation', { name: 'Passi della configurazione' });
    await user.click(within(nav).getByRole('button', { name: /Pagamenti/ }));
    expect(screen.getByRole('heading', { name: 'In cassa' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Configura i pagamenti online' })).toHaveAttribute(
      'href',
      expect.stringMatching(/\/e\/[^/]+\/config\/payments$/),
    );
    expect(screen.queryByText(/In arrivo|Disponibile a breve|M4/)).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Concludi la configurazione' }));
    expect(
      await screen.findByRole('heading', { name: 'Configurazione completata' }),
    ).toBeInTheDocument();
  });

  it('is not available to a cashier', async () => {
    renderApp(PATH, { ...routes(), ...baseRoutes('cashier') });
    expect(
      await screen.findByText('La configurazione guidata è riservata a proprietari e gestori.'),
    ).toBeInTheDocument();
    await waitFor(() =>
      expect(screen.queryByRole('heading', { level: 2, name: /^Passo/ })).toBeNull(),
    );
  });

  describe('start from scratch', () => {
    const ownerRoutes = (extra: Record<string, Handler> = {}, status = EMPTY_STATUS) => ({
      ...routes(extra, status),
      ...baseRoutes('owner'),
    });

    it('is offered to the owner only', async () => {
      const first = renderApp(PATH, routes());
      await stepHeading();
      expect(screen.queryByRole('heading', { name: 'Ricomincia da zero' })).toBeNull();
      first.unmount();
      renderApp(PATH, ownerRoutes());
      expect(
        await screen.findByRole('heading', { name: 'Ricomincia da zero' }),
      ).toBeInTheDocument();
    });

    it('with operational data only the full reset is possible, after the name and the acknowledgement', async () => {
      const user = userEvent.setup();
      let body: Record<string, unknown> | undefined;
      const { fetchMock } = renderApp(
        PATH,
        ownerRoutes(
          {
            [`POST ${API}/setup/reset`]: async (req) => {
              body = (await req.json()) as Record<string, unknown>;
              return json(200, { scope: 'everything', deleted: { bookings: 70 } });
            },
          },
          { ...EMPTY_STATUS, has_season: true, has_map: true, has_operations: true },
        ),
      );
      const restart = await screen.findByRole('button', { name: 'Ricomincia la configurazione' });
      expect(restart).toBeDisabled();
      expect(restart).toHaveAccessibleDescription(/Ci sono prenotazioni/);

      await user.click(screen.getByRole('button', { name: 'Azzera lo stabilimento' }));
      const dialog = await screen.findByRole('alertdialog', { name: 'Azzerare lo stabilimento?' });
      const confirm = within(dialog).getByRole('button', { name: 'Azzera lo stabilimento' });
      expect(confirm).toBeDisabled();
      await user.type(
        within(dialog).getByLabelText(/Scrivi il nome dello stabilimento/),
        ' lido demo ',
      );
      expect(confirm).toBeDisabled();
      await user.click(
        within(dialog).getByRole('checkbox', { name: /vengono cancellati per sempre/ }),
      );
      await user.click(confirm);
      await waitFor(() =>
        expect(body).toEqual({ scope: 'everything', confirm_name: ' lido demo ' }),
      );
      await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull());
      expect(callsTo(fetchMock, 'POST', `${API}/setup/reset`)).toHaveLength(1);
      // The wizard starts over from the first step.
      expect(await stepHeading()).toHaveTextContent('Passo 1 di 6');
    });

    it('shows the server refusal in the dialog', async () => {
      const user = userEvent.setup();
      renderApp(
        PATH,
        ownerRoutes({
          [`POST ${API}/setup/reset`]: () =>
            problem(409, 'conflict', { detail: 'Lo stabilimento ha prenotazioni.' }),
        }),
      );
      await user.click(await screen.findByRole('button', { name: 'Ricomincia la configurazione' }));
      const dialog = await screen.findByRole('alertdialog', {
        name: 'Ricominciare la configurazione?',
      });
      await user.type(
        within(dialog).getByLabelText(/Scrivi il nome dello stabilimento/),
        'Lido Demo',
      );
      await user.click(within(dialog).getByRole('button', { name: 'Cancella la configurazione' }));
      expect(
        await within(dialog).findByText('Lo stabilimento ha prenotazioni.'),
      ).toBeInTheDocument();
    });
  });
});
