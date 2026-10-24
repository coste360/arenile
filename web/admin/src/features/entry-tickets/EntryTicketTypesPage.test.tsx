import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { bodyOf } from '../bookings/testing';
import {
  EST_ID,
  baseRoutes,
  callsTo,
  json,
  list,
  problem,
  renderApp,
} from '../configuration/testing';

const API = `/api/admin/v1/establishments/${EST_ID}`;
const ts = { created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' };
const ZONES = [
  {
    id: 'z-free',
    map_id: 'm1',
    code: 'libera',
    name: { it: 'Spiaggia libera' },
    assignment_mode: 'quantity',
    online_visible: true,
    auto_assign: false,
    sort_order: 0,
    version: 1,
    ...ts,
  },
  {
    id: 'z-row',
    map_id: 'm1',
    code: 'row1',
    name: { it: 'Prima fila' },
    assignment_mode: 'seat',
    online_visible: true,
    auto_assign: false,
    sort_order: 1,
    version: 1,
    ...ts,
  },
];
const PACKS = [
  {
    id: 'p-adult',
    code: 'ingresso-adulto',
    name: { it: 'Ingresso adulto' },
    components: [],
    active: true,
    sort_order: 0,
    version: 1,
    ...ts,
  },
];
const TYPE = {
  id: 't1',
  code: 'adulto',
  name: { it: 'Adulto' },
  description: {},
  pack_id: 'p-adult',
  zone_id: 'z-free',
  sort_order: 1,
  active: true,
  version: 3,
  ...ts,
};

function routes(items: unknown[] = []) {
  return {
    ...baseRoutes('manager'),
    [`GET ${API}/entry-ticket-types`]: () => json(200, { items }),
    [`GET ${API}/zones`]: () => list(ZONES),
    [`GET ${API}/packs`]: () => list(PACKS),
  };
}

describe('EntryTicketTypesPage', () => {
  it('lists the types with pack and zone names', async () => {
    renderApp(`/admin/e/${EST_ID}/config/entry-tickets`, routes([TYPE]));
    const row = (await screen.findByText('Adulto')).closest('tr') as HTMLElement;
    expect(within(row).getByText('adulto')).toBeInTheDocument();
    expect(await within(row).findByText('Ingresso adulto')).toBeInTheDocument();
    expect(within(row).getByText('Spiaggia libera')).toBeInTheDocument();
    expect(within(row).getByText('Attivo')).toBeInTheDocument();
  });

  it('validates and creates a type on a quantity zone', async () => {
    const user = userEvent.setup();
    const { fetchMock } = renderApp(`/admin/e/${EST_ID}/config/entry-tickets`, {
      ...routes(),
      [`POST ${API}/entry-ticket-types`]: () => json(201, { ...TYPE, id: 't2', code: 'ridotto' }),
    });
    await user.click(await screen.findByRole('button', { name: 'Nuovo tipo' }));
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: 'Salva' }));
    expect(await within(dialog).findByText('Scegli il pack')).toBeInTheDocument();
    expect(
      within(dialog).getByText('Il nome nella lingua predefinita è obbligatorio'),
    ).toBeInTheDocument();

    await user.type(within(dialog).getByLabelText('Codice'), 'ridotto');
    await user.type(
      within(dialog).getAllByRole('textbox', { name: /Nome/ })[0] as HTMLElement,
      'Ridotto',
    );
    const [packSelect, zoneSelect] = within(dialog).getAllByRole('combobox');
    await user.click(packSelect as HTMLElement);
    await user.click(await screen.findByRole('option', { name: 'Ingresso adulto' }));
    await user.click(zoneSelect as HTMLElement);
    // Only quantity zones can hold entry tickets.
    expect(screen.queryByRole('option', { name: 'Prima fila' })).not.toBeInTheDocument();
    await user.click(await screen.findByRole('option', { name: 'Spiaggia libera' }));
    await user.click(within(dialog).getByRole('button', { name: 'Salva' }));

    await waitFor(() =>
      expect(callsTo(fetchMock, 'POST', `${API}/entry-ticket-types`)).toHaveLength(1),
    );
    const [req] = callsTo(fetchMock, 'POST', `${API}/entry-ticket-types`);
    expect(await bodyOf(req as Request)).toEqual({
      code: 'ridotto',
      name: { it: 'Ridotto' },
      pack_id: 'p-adult',
      zone_id: 'z-free',
      sort_order: 0,
      active: true,
    });
  });

  it('explains that a sold type must be deactivated instead of deleted', async () => {
    const user = userEvent.setup();
    renderApp(`/admin/e/${EST_ID}/config/entry-tickets`, {
      ...routes([TYPE]),
      [`DELETE ${API}/entry-ticket-types/t1`]: () => problem(409, 'conflict'),
    });
    await user.click(await screen.findByRole('button', { name: 'Elimina Adulto' }));
    const confirm = await screen.findByRole('alertdialog');
    await user.click(within(confirm).getByRole('button', { name: 'Elimina' }));
    expect(
      await within(confirm).findByText(/disattivalo invece di eliminarlo/),
    ).toBeInTheDocument();
  });
});
