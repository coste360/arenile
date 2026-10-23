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
  renderApp,
  type Handler,
} from '../configuration/testing';

const ts = { version: 1, created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' };
const PACK = {
  id: 'pk1',
  code: 'base',
  name: { it: 'Ombrellone base' },
  composition: {},
  allowed_formulas: ['day'],
  channels: ['desk'],
  sort_order: 1,
  active: true,
  ...ts,
};
const EXPIRES = new Date(Date.now() + 90 * 60_000 + 30_000).toISOString();

function entry(id: string, customer: string, overrides: Record<string, unknown> = {}) {
  return {
    id,
    status: 'waiting',
    channel: 'desk',
    customer_id: `c-${id}`,
    customer_display_name: customer,
    starts_on: '2027-07-10',
    ends_on: '2027-07-12',
    slots: 'FULL',
    pack_id: 'pk1',
    quantity: 1,
    priority: 10,
    position: 1,
    offer_count: 0,
    version: 4,
    created_at: '2027-06-01T08:00:00Z',
    updated_at: '2027-06-01T08:00:00Z',
    ...overrides,
  };
}

const OFFER = {
  id: 'o1',
  entry_id: 'w3',
  status: 'pending',
  hold_id: 'h1',
  spot_label: 'B7',
  cause: 'cancellation',
  expires_at: EXPIRES,
  created_at: '2027-06-02T08:00:00Z',
};

const ENTRIES = [
  entry('w1', 'Rossi Mario'),
  entry('w2', 'Bianchi Luca', { position: 2, priority: 20, quantity: 2, slots: 'AM' }),
  entry('w3', 'Neri Anna', {
    status: 'offered',
    position: 0,
    offer_count: 1,
    active_offer: OFFER,
  }),
];

const SETTINGS = {
  enabled: true,
  public_join: false,
  auto_offer: true,
  offer_hold_minutes: 120,
  version: 2,
};

function routes(extra: Record<string, Handler> = {}): Record<string, Handler> {
  return {
    ...baseRoutes('manager'),
    [`GET ${estPath('/packs')}`]: () => list([PACK]),
    [`GET ${estPath('/waitlist')}`]: () => list(ENTRIES),
    [`GET ${estPath('/waitlist-settings')}`]: () => json(200, SETTINGS),
    ...extra,
  };
}

const PAGE = `/e/${EST_ID}/lista-attesa`;

async function loadedTable() {
  const table = await screen.findByRole('table', { name: "Lista d'attesa" });
  await within(table).findByText('Rossi Mario');
  return table;
}

describe('WaitlistPage', () => {
  it('lists the queue with positions, requests and the running offer', async () => {
    renderApp(PAGE, routes());
    const table = await loadedTable();
    const rows = within(table).getAllByRole('row').slice(1);
    expect(rows).toHaveLength(3);
    expect(rows[0]).toHaveTextContent(/^1Rossi Mario10\/07\/2027 → 12\/07\/2027Ombrellone base/);
    expect(rows[1]).toHaveTextContent(/2Bianchi Luca.*Mattina.*Ombrellone base · ×2.*In attesa/);
    expect(rows[2]).toHaveTextContent(/Neri Anna.*Offerta inviata.*B7.*1 h 30 min/);
    expect(
      screen.getByText('Offerte automatiche attive: ogni offerta resta valida 120 minuti.'),
    ).toBeInTheDocument();
  });

  it('moves an entry down the queue (priorities 10, 20… server side)', async () => {
    const { fetchMock } = renderApp(
      PAGE,
      routes({ [`POST ${estPath('/waitlist/reorder')}`]: () => json(204, null) }),
    );
    const user = userEvent.setup();
    await loadedTable();
    expect(
      screen.getByRole('button', { name: 'Sposta Rossi Mario più avanti in coda' }),
    ).toBeDisabled();
    await user.click(
      screen.getByRole('button', { name: 'Sposta Rossi Mario più indietro in coda' }),
    );
    await waitFor(() =>
      expect(callsTo(fetchMock, 'POST', estPath('/waitlist/reorder'))).toHaveLength(1),
    );
    const [req] = callsTo(fetchMock, 'POST', estPath('/waitlist/reorder'));
    expect(await bodyOf(req!)).toEqual({ entry_ids: ['w2', 'w1'] });
  });

  it('offers a spot by hand with the default hold time', async () => {
    const { fetchMock } = renderApp(
      PAGE,
      routes({
        [`POST ${estPath('/waitlist/w1/offer')}`]: () =>
          json(201, {
            id: 'o2',
            entry_id: 'w1',
            status: 'pending',
            hold_id: 'h2',
            expires_at: EXPIRES,
            created_at: '2027-06-02T08:00:00Z',
          }),
      }),
    );
    const user = userEvent.setup();
    await loadedTable();
    await user.click(screen.getByRole('button', { name: 'Azioni per Rossi Mario' }));
    await user.click(await screen.findByRole('menuitem', { name: 'Offri un posto' }));
    const dialog = await screen.findByRole('dialog', { name: 'Offri un posto' });
    expect(within(dialog).getByLabelText("Durata dell'offerta (minuti)")).toHaveValue('120');
    await user.click(within(dialog).getByRole('button', { name: "Invia l'offerta" }));
    await waitFor(() =>
      expect(callsTo(fetchMock, 'POST', estPath('/waitlist/w1/offer'))).toHaveLength(1),
    );
    const [req] = callsTo(fetchMock, 'POST', estPath('/waitlist/w1/offer'));
    expect(req!.headers.get('Idempotency-Key')).toBeTruthy();
    expect(await bodyOf(req!)).toEqual({ hold_minutes: 120 });
  });

  it('revokes a pending offer', async () => {
    const { fetchMock } = renderApp(
      PAGE,
      routes({
        [`POST ${estPath('/waitlist/w3/offers/o1/revoke')}`]: () =>
          json(200, { ...OFFER, status: 'revoked' }),
      }),
    );
    const user = userEvent.setup();
    await loadedTable();
    await user.click(screen.getByRole('button', { name: 'Azioni per Neri Anna' }));
    await user.click(await screen.findByRole('menuitem', { name: "Revoca l'offerta" }));
    await waitFor(() =>
      expect(callsTo(fetchMock, 'POST', estPath('/waitlist/w3/offers/o1/revoke'))).toHaveLength(1),
    );
  });

  it('warns when the waitlist is off and saves the settings with If-Match', async () => {
    const { fetchMock } = renderApp(
      PAGE,
      routes({
        [`GET ${estPath('/waitlist-settings')}`]: () => json(200, { ...SETTINGS, enabled: false }),
        [`PUT ${estPath('/waitlist-settings')}`]: async (req) =>
          json(200, { ...((await bodyOf(req)) as object), version: 3 }),
      }),
    );
    const user = userEvent.setup();
    expect(await screen.findByText("Lista d'attesa disattivata")).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Nuova iscrizione' })).toBeDisabled();
    await user.click(screen.getByRole('button', { name: 'Impostazioni' }));
    const dialog = await screen.findByRole('dialog', { name: "Impostazioni della lista d'attesa" });
    await user.click(within(dialog).getByRole('switch', { name: "Lista d'attesa attiva" }));
    await user.click(within(dialog).getByRole('button', { name: 'Salva' }));
    await waitFor(() =>
      expect(callsTo(fetchMock, 'PUT', estPath('/waitlist-settings'))).toHaveLength(1),
    );
    const [req] = callsTo(fetchMock, 'PUT', estPath('/waitlist-settings'));
    expect(req!.headers.get('If-Match')).toBe('"2"');
    expect(await bodyOf(req!)).toEqual({
      enabled: true,
      public_join: false,
      auto_offer: true,
      offer_hold_minutes: 120,
    });
    await waitFor(() => expect(screen.queryByText("Lista d'attesa disattivata")).toBeNull());
  });
});
