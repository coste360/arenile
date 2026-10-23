import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import {
  EST_ID,
  baseRoutes,
  callsTo,
  json,
  list,
  problem,
  renderApp,
  type Handler,
  type TestRole,
} from '../configuration/testing';

const PATH = `/admin/e/${EST_ID}/config/api-keys`;
const KEYS = `/api/admin/v1/establishments/${EST_ID}/api-keys`;

const apiKey = (extra: Record<string, unknown> = {}) => ({
  id: 'k1',
  name: 'Gestionale',
  prefix: 'ar_live_abcd2345',
  establishment_id: EST_ID,
  scopes: ['bookings.read', 'reports.read'],
  status: 'active',
  created_by_user_id: 'u-self',
  created_by_name: 'Mario Rossi',
  created_at: '2026-06-01T08:00:00Z',
  ...extra,
});

const revokedKey = apiKey({
  id: 'k2',
  name: 'Vecchio sito',
  prefix: 'ar_live_zzzz9999',
  status: 'revoked',
  revoked_at: '2026-06-10T08:00:00Z',
});

function routes(role: TestRole, extra: Record<string, Handler> = {}) {
  return {
    ...baseRoutes(role),
    [`GET ${KEYS}`]: (req: Request) =>
      new URL(req.url).searchParams.get('include_revoked') === 'true'
        ? list([apiKey(), revokedKey])
        : list([apiKey()]),
    ...extra,
  };
}

const bodyOf = async (req: Request | undefined) => (await req?.clone().json()) as unknown;

describe('ApiKeysPage', () => {
  it('lists the keys with prefix, scopes and status, and shows revoked keys on request', async () => {
    const user = userEvent.setup();
    const { fetchMock } = renderApp(PATH, routes('owner'));
    const table = await screen.findByRole('table', { name: 'Chiavi API' });
    const row = (await within(table).findByText('Gestionale')).closest('tr') as HTMLElement;
    expect(within(row).getByText('ar_live_abcd2345…')).toBeInTheDocument();
    expect(within(row).getByText('Prenotazioni (lettura)')).toBeInTheDocument();
    expect(within(row).getByText('Report')).toBeInTheDocument();
    expect(within(row).getByText('Attiva')).toBeInTheDocument();
    expect(within(row).getByText('Mai')).toBeInTheDocument();
    expect(screen.queryByText('Vecchio sito')).not.toBeInTheDocument();

    await user.click(screen.getByRole('switch', { name: 'Mostra revocate' }));
    const revokedRow = (await screen.findByText('Vecchio sito')).closest('tr') as HTMLElement;
    expect(within(revokedRow).getByText('Revocata')).toBeInTheDocument();
    // A revoked key has no actions.
    expect(within(revokedRow).queryByRole('button', { name: /Revoca/ })).not.toBeInTheDocument();
    expect(
      new URL(callsTo(fetchMock, 'GET', KEYS).at(-1)?.url ?? 'http://x').searchParams.get(
        'include_revoked',
      ),
    ).toBe('true');
  });

  it('creates a key and shows the full key once', async () => {
    const user = userEvent.setup();
    const { fetchMock } = renderApp(
      PATH,
      routes('owner', {
        [`POST ${KEYS}`]: () =>
          json(201, {
            api_key: apiKey({
              id: 'k3',
              name: 'Sito',
              scopes: ['bookings.read', 'bookings.write'],
            }),
            secret: 'ar_live_abcd2345_S3CR3T',
          }),
      }),
    );
    await screen.findByRole('table', { name: 'Chiavi API' });
    await user.click(screen.getByRole('button', { name: 'Nuova chiave' }));
    const dialog = await screen.findByRole('dialog', { name: 'Nuova chiave' });

    // Validation: name required, at least one scope.
    await user.click(within(dialog).getByRole('checkbox', { name: 'Prenotazioni (lettura)' }));
    await user.click(within(dialog).getByRole('button', { name: 'Crea chiave' }));
    expect(await within(dialog).findByText('Campo obbligatorio.')).toBeInTheDocument();
    expect(within(dialog).getByText('Scegli almeno un permesso.')).toBeInTheDocument();
    expect(callsTo(fetchMock, 'POST', KEYS)).toHaveLength(0);

    await user.type(within(dialog).getByLabelText('Nome'), 'Sito');
    await user.click(within(dialog).getByRole('checkbox', { name: 'Prenotazioni (scrittura)' }));
    await user.click(within(dialog).getByRole('checkbox', { name: 'Prenotazioni (lettura)' }));
    // users.manage is never offered.
    expect(within(dialog).queryByRole('checkbox', { name: /utenti/i })).not.toBeInTheDocument();
    await user.click(within(dialog).getByRole('button', { name: 'Crea chiave' }));

    const secret = await screen.findByRole('dialog', { name: 'Chiave creata' });
    expect(within(secret).getByLabelText('Chiave API')).toHaveValue('ar_live_abcd2345_S3CR3T');
    expect(within(secret).getByText(/non verrà mostrato di nuovo/)).toBeInTheDocument();
    const post = callsTo(fetchMock, 'POST', KEYS)[0];
    expect(await bodyOf(post)).toEqual({
      name: 'Sito',
      scopes: ['bookings.write', 'bookings.read'],
    });
    expect(post?.headers.get('Idempotency-Key')).toBeNull();

    await user.click(within(secret).getByRole('button', { name: 'Ho salvato, chiudi' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(screen.queryByDisplayValue('ar_live_abcd2345_S3CR3T')).not.toBeInTheDocument();
    // The list is refetched (not patched with the secret).
    await waitFor(() => expect(callsTo(fetchMock, 'GET', KEYS).length).toBeGreaterThan(1));
  });

  it('edits a key sending only what changed', async () => {
    const user = userEvent.setup();
    const { fetchMock } = renderApp(
      PATH,
      routes('owner', {
        [`PATCH ${KEYS}/k1`]: () => json(200, apiKey({ name: 'Gestionale cassa' })),
      }),
    );
    await screen.findByText('Gestionale');
    await user.click(screen.getByRole('button', { name: 'Modifica la chiave Gestionale' }));
    const dialog = await screen.findByRole('dialog', { name: 'Modifica chiave' });
    const name = within(dialog).getByLabelText('Nome');
    await user.clear(name);
    await user.type(name, 'Gestionale cassa');
    await user.click(within(dialog).getByRole('button', { name: 'Salva' }));
    await waitFor(() => expect(callsTo(fetchMock, 'PATCH', `${KEYS}/k1`)).toHaveLength(1));
    expect(await bodyOf(callsTo(fetchMock, 'PATCH', `${KEYS}/k1`)[0])).toEqual({
      name: 'Gestionale cassa',
    });
  });

  it('revokes a key after confirmation and shows server errors in the dialog', async () => {
    const user = userEvent.setup();
    let fail = true;
    const { fetchMock } = renderApp(
      PATH,
      routes('owner', {
        [`DELETE ${KEYS}/k1`]: () =>
          fail ? problem(409, 'conflict', { detail: 'Chiave già revocata.' }) : json(204, null),
      }),
    );
    await screen.findByText('Gestionale');
    await user.click(screen.getByRole('button', { name: 'Revoca la chiave Gestionale' }));
    const confirm = await screen.findByRole('alertdialog', {
      name: 'Revocare la chiave Gestionale?',
    });
    await user.click(within(confirm).getByRole('button', { name: 'Revoca' }));
    expect(await within(confirm).findByText('Chiave già revocata.')).toBeInTheDocument();
    fail = false;
    await user.click(within(confirm).getByRole('button', { name: 'Revoca' }));
    await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument());
    expect(callsTo(fetchMock, 'DELETE', `${KEYS}/k1`)).toHaveLength(2);
  });

  it('is reserved to the owner', async () => {
    const { fetchMock } = renderApp(PATH, routes('manager'));
    expect(
      await screen.findByText('Solo il titolare può gestire le integrazioni.'),
    ).toBeInTheDocument();
    expect(callsTo(fetchMock, 'GET', KEYS)).toHaveLength(0);
  });
});
