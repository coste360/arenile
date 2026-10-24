import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { EST_ID, baseRoutes, callsTo, json, renderApp, type TestRole } from '../configuration/testing';
import { DISMISSED_KEY, readDismissed, upgradeDocsUrl, writeDismissed } from './lib';

const PATH = `/e/${EST_ID}/config/test-mode`;
const UPDATE = '/api/admin/v1/system/update';

function status(overrides: Record<string, unknown> = {}) {
  return {
    current: '1.1.0',
    status: 'ok',
    latest: '1.2.0',
    update_available: true,
    published_at: '2026-10-20T08:00:00Z',
    release_notes_url: 'https://github.com/coste360/arenile/releases/tag/v1.2.0',
    download_url: 'https://arenile.app/download/1.2.0/Arenile-1.2.0_setup.exe',
    how_to_update: 'desktop',
    check_enabled: true,
    checked_at: '2026-10-24T09:00:00Z',
    ...overrides,
  };
}

function render(body: Record<string, unknown>, role: TestRole = 'owner') {
  return renderApp(PATH, {
    ...baseRoutes(role),
    [`GET /api/admin/v1/establishments/${EST_ID}/test-mode`]: () =>
      json(200, { enabled: false, public_enabled: false, test_bookings_count: 0, version: 1 }),
    [`GET ${UPDATE}`]: () => json(200, body),
  });
}

describe('UpdateBanner', () => {
  beforeEach(() => window.localStorage.clear());
  afterEach(() => window.localStorage.clear());

  it('offers the download of the desktop package and the release notes', async () => {
    render(status());
    const banner = await screen.findByTestId('update-banner');
    expect(within(banner).getByRole('status')).toBeTruthy();
    expect(within(banner).getByText('È disponibile Arenile 1.2.0')).toBeTruthy();
    const download = within(banner).getByRole('link', { name: 'Scarica' });
    expect(download.getAttribute('href')).toBe(
      'https://arenile.app/download/1.2.0/Arenile-1.2.0_setup.exe',
    );
    expect(within(banner).getByRole('link', { name: 'Novità' }).getAttribute('href')).toBe(
      'https://github.com/coste360/arenile/releases/tag/v1.2.0',
    );
  });

  it('links Docker installs to the upgrade guide', async () => {
    render(status({ how_to_update: 'docker', download_url: undefined }));
    const banner = await screen.findByTestId('update-banner');
    expect(within(banner).queryByRole('link', { name: 'Scarica' })).toBeNull();
    expect(
      within(banner).getByRole('link', { name: 'Come aggiornare' }).getAttribute('href'),
    ).toBe('https://docs.arenile.app/installazione/aggiornamenti/');
  });

  it('hides when dismissed and remembers the version', async () => {
    render(status());
    const banner = await screen.findByTestId('update-banner');
    await userEvent.click(
      within(banner).getByRole('button', { name: "Nascondi l'avviso per questa versione" }),
    );
    await waitFor(() => expect(screen.queryByTestId('update-banner')).toBeNull());
    expect(window.localStorage.getItem(DISMISSED_KEY)).toBe('1.2.0');
  });

  it('stays hidden for a dismissed version and comes back for the next one', async () => {
    window.localStorage.setItem(DISMISSED_KEY, '1.2.0');
    const first = render(status());
    await waitFor(() => expect(callsTo(first.fetchMock, 'GET', UPDATE)).toHaveLength(1));
    await screen.findByRole('switch', { name: 'Modalità prova attiva' });
    expect(screen.queryByTestId('update-banner')).toBeNull();
    first.unmount();
    render(status({ latest: '1.3.0' }));
    expect(await screen.findByText('È disponibile Arenile 1.3.0')).toBeTruthy();
  });

  it('shows nothing when there is no update or the check failed', async () => {
    const { fetchMock } = render(
      status({ status: 'unavailable', latest: undefined, update_available: false }),
    );
    await waitFor(() => expect(callsTo(fetchMock, 'GET', UPDATE)).toHaveLength(1));
    await screen.findByRole('switch', { name: 'Modalità prova attiva' });
    expect(screen.queryByTestId('update-banner')).toBeNull();
    expect(screen.queryByRole('alert')).toBeNull();
  });

  it('never asks a cashier', async () => {
    const { fetchMock } = render(status(), 'cashier');
    await screen.findByRole('switch', { name: 'Modalità prova attiva' });
    expect(callsTo(fetchMock, 'GET', UPDATE)).toHaveLength(0);
    expect(screen.queryByTestId('update-banner')).toBeNull();
  });
});

describe('UpdateCheckStatus', () => {
  it('shows a neutral line in the user menu when the check failed', async () => {
    render(
      status({ status: 'unavailable', how_to_update: 'docker', latest: undefined, update_available: false }),
    );
    await screen.findByRole('switch', { name: 'Modalità prova attiva' });
    await userEvent.click(screen.getByRole('button', { name: /Mario Rossi/ }));
    const info = await screen.findByTestId('update-check-status');
    expect(within(info).getByText('Versione 1.1.0')).toBeTruthy();
    expect(within(info).getByText(/Controllo aggiornamenti: attivo/).textContent).toBe(
      'Controllo aggiornamenti: attivo (ARENILE_UPDATE_CHECK)',
    );
    expect(
      within(info).getByText('Impossibile cercare automaticamente gli aggiornamenti.'),
    ).toBeTruthy();
    expect(screen.queryByRole('alert')).toBeNull();
  });
});

describe('update notice helpers', () => {
  it('survives a storage that throws', () => {
    const broken = {
      getItem: () => {
        throw new Error('blocked');
      },
      setItem: () => {
        throw new Error('blocked');
      },
    };
    expect(readDismissed(broken)).toBeNull();
    expect(() => writeDismissed(broken, '1.2.0')).not.toThrow();
    expect(readDismissed(undefined)).toBeNull();
  });

  it('links the upgrade guide in the panel language', () => {
    expect(upgradeDocsUrl('it')).toBe('https://docs.arenile.app/installazione/aggiornamenti/');
    expect(upgradeDocsUrl('de')).toBe('https://docs.arenile.app/en/installazione/aggiornamenti/');
  });
});
