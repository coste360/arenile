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
} from '../configuration/testing';

const API = `/api/admin/v1/establishments/${EST_ID}`;
const PATH = `/admin/e/${EST_ID}/config/devices`;

function device(overrides: Record<string, unknown> = {}) {
  return {
    id: 'd1',
    establishment_id: EST_ID,
    user_id: 'u-self',
    user_name: 'Mario Rossi',
    role: 'owner',
    name: 'iPhone di Anna',
    platform: 'ios',
    kind: 'mobile',
    app_version: '1.0.0',
    token_prefix: 'ar_dev_abcdefgh',
    created_at: '2027-06-01T08:00:00Z',
    last_used_at: '2027-06-02T10:30:00Z',
    current: false,
    fiscal_bridge: false,
    ...overrides,
  };
}

const CODE = {
  code: 'ABCD-EFGH',
  expires_at: '2099-01-01T00:05:00Z',
  establishment_id: EST_ID,
  user_id: 'u-self',
  qr_payload: { v: 1, url: 'https://lido.example.it', code: 'ABCD-EFGH' },
  qr_text: 'arenile://pair?d=eyJ2IjoxfQ',
  qr_svg:
    '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"><rect width="1" height="1"/></svg>',
};

describe('DevicesPage', () => {
  it('lists the devices and revokes one after confirmation', async () => {
    const user = userEvent.setup();
    const { fetchMock } = renderApp(PATH, {
      ...baseRoutes('owner'),
      [`GET ${API}/devices`]: () =>
        list([
          device(),
          device({
            id: 'd2',
            name: 'PC cassa',
            platform: 'windows',
            kind: 'desktop',
            user_name: 'Anna Cassa',
            role: 'cashier',
            last_used_at: undefined,
            fiscal_bridge: true,
          }),
        ]),
      [`DELETE ${API}/devices/d2`]: () => json(204, null),
    });
    expect(await screen.findByText('iPhone di Anna')).toBeInTheDocument();
    expect(screen.getByText('PC cassa')).toBeInTheDocument();
    expect(screen.getByText('Mai usato')).toBeInTheDocument();
    expect(screen.getByText('Ponte fiscale')).toBeInTheDocument();
    expect(screen.getByText('Cassiere')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Revoca PC cassa' }));
    const dialog = await screen.findByRole('alertdialog');
    // The fiscal bridge warning is shown for the bridge device.
    expect(within(dialog).getByText(/smette di fare da ponte fiscale/)).toBeInTheDocument();
    await user.click(within(dialog).getByRole('button', { name: 'Revoca' }));
    await waitFor(() => expect(callsTo(fetchMock, 'DELETE', `${API}/devices/d2`)).toHaveLength(1));
  });

  it('pairs a device: QR, code, countdown, then the confirmation', async () => {
    const user = userEvent.setup();
    let paired = false;
    const posts: unknown[] = [];
    const { fetchMock } = renderApp(PATH, {
      ...baseRoutes('cashier'),
      [`GET ${API}/devices`]: () =>
        list(paired ? [device({ id: 'd9', name: 'Telefono di Mario', role: 'cashier' })] : []),
      [`POST ${API}/devices/pairing-codes`]: async (req) => {
        posts.push(await req.json());
        paired = true; // the app pairs while the QR is on screen
        return json(201, CODE);
      },
    });
    expect(await screen.findByText('Nessun dispositivo abbinato')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Abbina un dispositivo' }));
    const dialog = await screen.findByRole('dialog');
    // Without users.manage the code is issued at once, for the caller.
    expect(await within(dialog).findByText('ABCD-EFGH')).toBeInTheDocument();
    expect(posts).toEqual([{ ttl_seconds: 300 }]);
    const qr = within(dialog).getByRole('img', { name: 'QR code di abbinamento' });
    expect(qr.getAttribute('src')).toMatch(/^data:image\/svg\+xml/);
    expect(within(dialog).getByText('Indirizzo: https://lido.example.it')).toBeInTheDocument();
    expect(within(dialog).getByText(/^[45]:\d\d$/)).toBeInTheDocument();
    expect(within(dialog).queryByRole('combobox')).toBeNull();
    // Polling notices the new device.
    expect(
      await within(dialog).findByText('Dispositivo abbinato', {}, { timeout: 6000 }),
    ).toBeInTheDocument();
    expect(callsTo(fetchMock, 'GET', `${API}/devices`).length).toBeGreaterThan(1);
  }, 10000);

  it('lets an owner choose the user before generating the code', async () => {
    const user = userEvent.setup();
    const posts: unknown[] = [];
    renderApp(PATH, {
      ...baseRoutes('owner'),
      [`GET ${API}/devices`]: () => list([]),
      [`GET ${API}/team`]: () =>
        list([
          { user_id: 'u-self', name: 'Mario Rossi', active: true },
          { user_id: 'u2', name: 'Anna Cassa', active: true },
        ]),
      [`POST ${API}/devices/pairing-codes`]: async (req) => {
        posts.push(await req.json());
        return json(201, { ...CODE, user_id: 'u2' });
      },
    });
    await user.click(await screen.findByRole('button', { name: 'Abbina un dispositivo' }));
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('combobox', { name: 'Dispositivo di' }));
    await user.click(await screen.findByRole('option', { name: 'Anna Cassa' }));
    await user.click(within(dialog).getByRole('button', { name: 'Genera il QR code' }));
    expect(await within(dialog).findByText('ABCD-EFGH')).toBeInTheDocument();
    expect(posts).toEqual([{ ttl_seconds: 300, user_id: 'u2' }]);
  });

  it('explains the LAN mode when the server refuses the code', async () => {
    const user = userEvent.setup();
    renderApp(PATH, {
      ...baseRoutes('cashier'),
      [`GET ${API}/devices`]: () => list([]),
      [`POST ${API}/devices/pairing-codes`]: () => problem(409, 'lan_mode_required'),
    });
    await user.click(await screen.findByRole('button', { name: 'Abbina un dispositivo' }));
    const dialog = await screen.findByRole('dialog');
    expect(await within(dialog).findByText('Attiva la modalità rete locale')).toBeInTheDocument();
  });

  it('disables pairing on the desktop edition in local mode', async () => {
    renderApp(PATH, {
      ...baseRoutes('owner'),
      [`GET ${API}/devices`]: () => list([]),
      'GET /api/admin/v1/meta': () =>
        json(200, {
          product: 'arenile',
          version: 'test',
          api_version: '1',
          edition: 'desktop',
          desktop: { mode: 'local' },
          server_time: '2027-06-01T08:00:00Z',
          min_client_versions: { mobile: '1.0.0', desktop: '1.0.0' },
          capabilities: {
            online_booking: false,
            online_payments: false,
            payment_providers: ['manual'],
            fiscal_drivers: [],
            device_pairing: false,
            fiscal_bridge: false,
            realtime: true,
            push_notifications: false,
          },
          features: [],
        }),
    });
    expect(await screen.findByText('Attiva la modalità rete locale')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Abbina un dispositivo' })).toBeDisabled();
  });
});
