import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { EST_ID, baseRoutes, callsTo, json, renderApp } from '../configuration/testing';
import { formatBytes } from './lib';

const META_DESKTOP = {
  product: 'arenile',
  version: '1.0.0',
  api_version: '0.1.0',
  edition: 'desktop',
  desktop: { mode: 'local' },
  server_time: '2027-07-01T08:00:00Z',
  min_client_versions: { mobile: '0.1.0', desktop: '0.1.0' },
  capabilities: {
    online_booking: false,
    online_payments: false,
    payment_providers: ['manual'],
    fiscal_drivers: ['none'],
    device_pairing: false,
    fiscal_bridge: false,
    realtime: true,
    push_notifications: false,
  },
  features: [],
};

const SETTINGS = {
  backup_dir: '/Users/anna/Library/Application Support/Arenile/backups',
  backup_retention: 14,
  backup_daily: true,
  backup_on_exit: true,
  update_check: false,
  autostart: false,
  keep_running_in_tray: false,
  lan_enabled: false,
  lan_port: 8443,
};

const BACKUPS = {
  directory: SETTINGS.backup_dir,
  items: [
    {
      name: 'arenile-20270701T080000Z-daily.db',
      size_bytes: 1_250_000,
      created_at: '2027-07-01T08:00:00Z',
      reason: 'daily',
    },
  ],
};

function desktopRoutes() {
  return {
    ...baseRoutes('owner'),
    'GET /api/admin/v1/meta': () => json(200, META_DESKTOP),
    'GET /api/desktop/v1/settings': () => json(200, SETTINGS),
    'GET /api/desktop/v1/backups': () => json(200, BACKUPS),
  };
}

describe('DesktopSettingsPage', () => {
  it('saves the settings (backup folder chosen in the panel, opt-in update check)', async () => {
    const user = userEvent.setup();
    let body: unknown;
    renderApp(`/admin/e/${EST_ID}/config/desktop`, {
      ...desktopRoutes(),
      'PUT /api/desktop/v1/settings': async (req) => {
        body = await req.json();
        return json(200, body);
      },
    });
    const dir = await screen.findByLabelText('Cartella dei backup');
    await user.clear(dir);
    await user.type(dir, '/Volumes/USB/Arenile');
    await user.click(
      screen.getByRole('switch', { name: "Avvia Arenile all'accensione del computer" }),
    );
    await user.click(
      screen.getByRole('switch', {
        name: 'Alla chiusura della finestra resta attivo nella barra di sistema',
      }),
    );
    await user.click(screen.getByRole('button', { name: 'Salva impostazioni' }));
    await waitFor(() => expect(body).toBeDefined());
    expect(body).toEqual({
      ...SETTINGS,
      backup_dir: '/Volumes/USB/Arenile',
      autostart: true,
      keep_running_in_tray: true,
    });
  });

  it('lists the backups, makes one now and restores one after confirmation', async () => {
    const user = userEvent.setup();
    const { fetchMock } = renderApp(`/admin/e/${EST_ID}/config/desktop`, {
      ...desktopRoutes(),
      'POST /api/desktop/v1/backups': () =>
        json(201, {
          ...BACKUPS.items[0],
          name: 'arenile-20270701T090000Z-manual.db',
          reason: 'manual',
        }),
      [`POST /api/desktop/v1/backups/${BACKUPS.items[0]!.name}/restore`]: () =>
        json(202, { backup: BACKUPS.items[0], restart_required: true }),
    });
    await waitFor(() => expect(screen.getByText(BACKUPS.items[0]!.name)).toBeInTheDocument());
    expect(screen.getByText('Giornaliero')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Backup ora' }));
    await waitFor(() =>
      expect(callsTo(fetchMock, 'POST', '/api/desktop/v1/backups')).toHaveLength(1),
    );

    await user.click(
      screen.getByRole('button', { name: `Ripristina il backup ${BACKUPS.items[0]!.name}` }),
    );
    const dialog = await screen.findByRole('alertdialog');
    expect(within(dialog).getByText(/Prima del ripristino/)).toBeInTheDocument();
    await user.click(within(dialog).getByRole('button', { name: 'Ripristina e riavvia' }));
    expect(await screen.findByText('Ripristino in corso')).toBeInTheDocument();
    expect(
      callsTo(fetchMock, 'POST', `/api/desktop/v1/backups/${BACKUPS.items[0]!.name}/restore`),
    ).toHaveLength(1);
  });
});

describe('desktop online sections', () => {
  it('explain how to move online instead of the public page on the desktop edition', async () => {
    renderApp(`/admin/e/${EST_ID}/config/online`, desktopRoutes());
    expect(await screen.findByText('Pagina pubblica non disponibile')).toBeInTheDocument();
    expect(screen.getByText('arenile restore --in arenile-backup.db')).toBeInTheDocument();
  });

  it('explain the manual-only payments on the desktop edition', async () => {
    renderApp(`/admin/e/${EST_ID}/config/payments`, desktopRoutes());
    expect(await screen.findByText('Incassi solo dalla cassa')).toBeInTheDocument();
  });

  it('leave the server edition untouched', async () => {
    renderApp(`/admin/e/${EST_ID}/config/online`, {
      ...baseRoutes('owner'),
      'GET /api/admin/v1/meta': () =>
        json(200, { ...META_DESKTOP, edition: 'server', desktop: undefined }),
    });
    expect(await screen.findByRole('heading', { level: 1 })).toBeInTheDocument();
    expect(screen.queryByText(/Pagina pubblica non disponibile/)).not.toBeInTheDocument();
  });
});

describe('formatBytes', () => {
  it('formats in the UI language', () => {
    expect(formatBytes(512, 'en')).toBe('512 byte');
    expect(formatBytes(1_250_000, 'en')).toBe('1.3 MB');
    expect(formatBytes(1_250_000, 'it')).toBe('1,3 MB');
  });
});
