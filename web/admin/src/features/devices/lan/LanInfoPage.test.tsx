import { screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { EST_ID, baseRoutes, json, list, renderApp } from '../../configuration/testing';
import { certRenewalSoon } from './lib';

const PATH = `/admin/e/${EST_ID}/config/devices/lan`;
const FP = Array.from({ length: 32 }, (_, i) => (i + 160).toString(16).toUpperCase()).join(':');

function meta(desktop: Record<string, unknown> | undefined, edition = 'desktop') {
  return () =>
    json(200, {
      product: 'arenile',
      version: 'test',
      api_version: '1',
      edition,
      ...(desktop ? { desktop } : {}),
      server_time: '2027-06-01T08:00:00Z',
      min_client_versions: { mobile: '1.0.0', desktop: '1.0.0' },
      capabilities: {
        online_booking: false,
        online_payments: false,
        payment_providers: ['manual'],
        fiscal_drivers: [],
        device_pairing: desktop?.mode === 'lan',
        fiscal_bridge: true,
        realtime: true,
        push_notifications: false,
      },
      features: [],
    });
}

describe('LanInfoPage', () => {
  it('shows the address, the fingerprint and how to accept the certificate', async () => {
    const user = userEvent.setup();
    renderApp(PATH, {
      ...baseRoutes('owner'),
      'GET /api/admin/v1/meta': meta({
        mode: 'lan',
        lan_url: 'https://192.168.1.20:8443',
        lan_cert_fingerprint: FP,
        lan_qr_svg: '<svg xmlns="http://www.w3.org/2000/svg"></svg>',
      }),
      [`GET /api/admin/v1/establishments/${EST_ID}/devices`]: () => list([]),
    });
    expect(await screen.findByTestId('lan-host')).toHaveTextContent('192.168.1.20:8443');
    // Own breadcrumb label, not the parent's "Dispositivi".
    const crumbs = screen.getByRole('navigation', { name: 'Percorso' });
    expect(within(crumbs).getByText('Rete locale')).toBeInTheDocument();
    expect(within(crumbs).queryByText('Dispositivi')).not.toBeInTheDocument();
    expect(screen.getByText('https://192.168.1.20:8443/admin/')).toBeInTheDocument();
    expect(
      screen.getByRole('img', { name: "QR code dell'indirizzo https://192.168.1.20:8443/admin/" }),
    ).toBeInTheDocument();
    const fp = screen.getByLabelText('Impronta SHA-256 del certificato');
    expect(within(fp).getByText('A0:A1:A2:A3:A4:A5:A6:A7')).toBeInTheDocument();
    expect(within(fp).getByText('B8:B9:BA:BB:BC:BD:BE:BF')).toBeInTheDocument();
    expect(screen.getByText(/Confronta l'impronta SHA-256/)).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Abbina un dispositivo' }));
    expect(await screen.findByRole('dialog')).toBeInTheDocument();
  });

  it('warns ahead of the certificate renewal, not when it is far away', async () => {
    const day = 86_400_000;
    const soon = new Date(Date.now() + 20 * day).toISOString();
    renderApp(PATH, {
      ...baseRoutes('owner'),
      'GET /api/admin/v1/meta': meta({
        mode: 'lan',
        lan_url: 'https://192.168.1.20:8443',
        lan_cert_fingerprint: FP,
        lan_cert_renews_at: soon,
      }),
      [`GET /api/admin/v1/establishments/${EST_ID}/devices`]: () => list([]),
    });
    const alert = await screen.findByTestId('lan-cert-renewal');
    expect(alert).toHaveTextContent(/Il certificato sarà rinnovato il \d{2}\/\d{2}\/\d{4}/);
    expect(alert).toHaveTextContent(/vanno abbinate di nuovo/);
    expect(certRenewalSoon(soon)).toBe(true);
    expect(certRenewalSoon(new Date(Date.now() + 200 * day).toISOString())).toBe(false);
    expect(certRenewalSoon(undefined)).toBe(false);
    expect(certRenewalSoon('not a date')).toBe(false);
  });

  it('explains how to turn the LAN mode on in local mode', async () => {
    renderApp(PATH, {
      ...baseRoutes('owner'),
      'GET /api/admin/v1/meta': meta({ mode: 'local' }),
    });
    expect(await screen.findByText('La modalità rete locale è spenta')).toBeInTheDocument();
    expect(
      screen.getByRole('link', { name: "Apri le impostazioni dell'app desktop" }),
    ).toHaveAttribute('href', `/admin/e/${EST_ID}/config/desktop`);
    expect(screen.queryByRole('button', { name: 'Abbina un dispositivo' })).not.toBeInTheDocument();
  });

  it('points server instances to the devices page', async () => {
    renderApp(PATH, {
      ...baseRoutes('owner'),
      'GET /api/admin/v1/meta': meta(undefined, 'server'),
    });
    expect(await screen.findByText("Questa pagina riguarda l'app desktop")).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Vai a Dispositivi' })).toHaveAttribute(
      'href',
      `/admin/e/${EST_ID}/config/devices`,
    );
  });
});
