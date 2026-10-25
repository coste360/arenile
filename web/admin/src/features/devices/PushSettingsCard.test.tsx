import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { EST_ID, baseRoutes, callsTo, json, list, renderApp } from '../configuration/testing';

const API = `/api/admin/v1/establishments/${EST_ID}`;
const PATH = `/admin/e/${EST_ID}/config/devices`;

function settings(overrides: Record<string, unknown> = {}) {
  return {
    enabled: false,
    relay_configured: true,
    version: 1,
    events: { booking_new_online: true, booking_approval_requested: true, payment_failed: true },
    ...overrides,
  };
}

describe('PushSettingsCard', () => {
  it('turns push on with If-Match and one event off', async () => {
    const user = userEvent.setup();
    const { fetchMock } = renderApp(PATH, {
      ...baseRoutes('owner'),
      [`GET ${API}/devices`]: () => list([]),
      [`GET ${API}/push-settings`]: () => json(200, settings(), { ETag: '"1"' }),
      [`PUT ${API}/push-settings`]: () =>
        json(200, settings({ enabled: true, version: 2 }), { ETag: '"2"' }),
    });
    const toggle = await screen.findByRole('switch', { name: 'Invia notifiche push' });
    const payment = screen.getByRole('switch', { name: 'Pagamento non riuscito' });
    // Events wait for the main switch.
    expect(payment).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Salva' })).toBeDisabled();
    await user.click(toggle);
    await user.click(payment);
    await user.click(screen.getByRole('button', { name: 'Salva' }));
    await waitFor(() => expect(callsTo(fetchMock, 'PUT', `${API}/push-settings`)).toHaveLength(1));
    const [req] = callsTo(fetchMock, 'PUT', `${API}/push-settings`);
    if (!req) throw new Error('no PUT');
    expect(req.headers.get('If-Match')).toBe('"1"');
    expect(await req.json()).toEqual({
      enabled: true,
      events: { booking_new_online: true, booking_approval_requested: true, payment_failed: false },
    });
  });

  it('explains a server without the relay key and disables the switches', async () => {
    renderApp(PATH, {
      ...baseRoutes('owner'),
      [`GET ${API}/devices`]: () => list([]),
      [`GET ${API}/push-settings`]: () =>
        json(200, settings({ relay_configured: false }), { ETag: '"1"' }),
    });
    expect(
      await screen.findByText('Notifiche push non configurate su questo server'),
    ).toBeInTheDocument();
    expect(screen.getByRole('switch', { name: 'Invia notifiche push' })).toBeDisabled();
  });

  it('is hidden from roles without establishment.configure', async () => {
    const { fetchMock } = renderApp(PATH, {
      ...baseRoutes('cashier'),
      [`GET ${API}/devices`]: () => list([]),
      [`GET ${API}/push-settings`]: () => json(200, settings(), { ETag: '"1"' }),
    });
    expect(await screen.findByText('Nessun dispositivo abbinato')).toBeInTheDocument();
    expect(screen.queryByText("Notifiche push sull'app mobile")).not.toBeInTheDocument();
    expect(callsTo(fetchMock, 'GET', `${API}/push-settings`)).toHaveLength(0);
  });
});
