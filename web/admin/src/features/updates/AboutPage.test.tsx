import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { baseRoutes, callsTo, json, renderApp, type TestRole } from '../configuration/testing';
import { PENDING_REFETCH_MS } from './lib';

const PATH = '/informazioni';
const UPDATE = '/api/admin/v1/system/update';
const CHECK = '/api/admin/v1/system/update/check';

function status(overrides: Record<string, unknown> = {}) {
  return {
    current: '1.1.0',
    status: 'ok',
    latest: '1.2.0',
    update_available: true,
    published_at: '2026-10-20T08:00:00Z',
    release_notes_url: 'https://github.com/coste360/arenile/releases/tag/v1.2.0',
    download_url: 'https://updates.coste360.it/arenile/1.2.0/Arenile-1.2.0-arm64.dmg',
    how_to_update: 'desktop',
    check_enabled: true,
    checked_at: '2026-10-24T09:00:00Z',
    last_attempt_at: '2026-10-24T09:00:00Z',
    pending: false,
    os: 'darwin',
    arch: 'arm64',
    ...overrides,
  };
}

function routes(role: TestRole, get: () => Response, post?: () => Response) {
  return {
    ...baseRoutes(role),
    'GET /api/admin/v1/meta': () =>
      json(200, { product: 'arenile', version: '1.1.0', edition: 'desktop' }),
    [`GET ${UPDATE}`]: get,
    ...(post ? { [`POST ${CHECK}`]: post } : {}),
  };
}

afterEach(() => {
  vi.useRealTimers();
  delete (window as { arenileDesktop?: unknown }).arenileDesktop;
});

describe('AboutPage', () => {
  it('shows the version, the update check and the download for this system', async () => {
    (window as { arenileDesktop?: unknown }).arenileDesktop = { mode: 'local', shell: 'window' };
    renderApp(
      PATH,
      routes('owner', () => json(200, status())),
    );
    expect(await screen.findByRole('heading', { name: 'Informazioni su Arenile' })).toBeTruthy();
    const details = await screen.findByTestId('about-details');
    await within(details).findByText('Ultima versione pubblicata');
    expect(within(details).getByText('1.1.0')).toBeTruthy();
    expect(within(details).getByText('App desktop')).toBeTruthy();
    expect(within(details).getByText('Attivo')).toBeTruthy();
    expect(within(details).getByText('1.2.0')).toBeTruthy();
    const download = screen.getByRole('link', {
      name: 'Scarica Arenile 1.2.0 per macOS (Apple Silicon)',
    });
    expect(download.getAttribute('href')).toBe(
      'https://updates.coste360.it/arenile/1.2.0/Arenile-1.2.0-arm64.dmg',
    );
    expect(screen.getAllByRole('link', { name: 'Novità' }).length).toBeGreaterThan(0);
    expect(screen.getByText('Il download si apre nel browser predefinito.')).toBeTruthy();
    expect(
      screen.getByRole('link', { name: 'Tutte le versioni e i sistemi' }).getAttribute('href'),
    ).toBe('https://arenile.app/download/');
  });

  it('runs "Verifica aggiornamenti" and shows the fresh status', async () => {
    const { fetchMock } = renderApp(
      PATH,
      routes(
        'manager',
        () => json(200, status({ latest: '1.1.0', update_available: false })),
        () => json(200, status({ latest: '1.3.0', update_available: true })),
      ),
    );
    const button = await screen.findByRole('button', { name: 'Verifica aggiornamenti' });
    expect(await screen.findByText('Arenile è aggiornato.')).toBeTruthy();
    await userEvent.click(button);
    await waitFor(() => expect(callsTo(fetchMock, 'POST', CHECK)).toHaveLength(1));
    expect(await screen.findByText('È disponibile la versione 1.3.0.')).toBeTruthy();
    expect(
      await screen.findByRole('link', { name: 'Scarica Arenile 1.3.0 per macOS (Apple Silicon)' }),
    ).toBeTruthy();
  });

  it('offers the upgrade guide on Docker and no download', async () => {
    renderApp(
      PATH,
      routes('owner', () =>
        json(200, status({ how_to_update: 'docker', download_url: undefined, os: 'linux' })),
      ),
    );
    await screen.findByText('Server (Docker)');
    expect(screen.queryByRole('link', { name: /Scarica/ })).toBeNull();
    expect(screen.queryByRole('link', { name: 'Tutte le versioni e i sistemi' })).toBeNull();
    expect(screen.getAllByRole('link', { name: 'Come aggiornare' }).length).toBeGreaterThan(0);
  });

  it('shows only the version to a cashier', async () => {
    const { fetchMock } = renderApp(
      PATH,
      routes('cashier', () => json(200, status())),
    );
    expect(
      await screen.findByText('Lo stato degli aggiornamenti è visibile solo a titolari e gestori.'),
    ).toBeTruthy();
    expect(await screen.findByText('1.1.0')).toBeTruthy();
    expect(callsTo(fetchMock, 'GET', UPDATE)).toHaveLength(0);
    expect(screen.queryByRole('button', { name: 'Verifica aggiornamenti' })).toBeNull();
  });

  it('reads the status again every 15 s until the first check is done', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    let pending = true;
    const { fetchMock } = renderApp(
      PATH,
      routes('owner', () =>
        json(
          200,
          pending
            ? status({
                pending: true,
                latest: undefined,
                update_available: false,
                last_attempt_at: undefined,
                checked_at: undefined,
              })
            : status(),
        ),
      ),
    );
    expect(await screen.findByText('Primo controllo in corso…')).toBeTruthy();
    expect(screen.queryByTestId('update-banner')).toBeNull();
    const before = callsTo(fetchMock, 'GET', UPDATE).length;
    pending = false;
    await vi.advanceTimersByTimeAsync(PENDING_REFETCH_MS);
    await waitFor(() => expect(callsTo(fetchMock, 'GET', UPDATE).length).toBe(before + 1));
    expect(
      await screen.findByRole('link', { name: 'Scarica Arenile 1.2.0 per macOS (Apple Silicon)' }),
    ).toBeTruthy();
    // The page has the actions in full: no banner on top of it.
    expect(screen.queryByTestId('update-banner')).toBeNull();
    // Done: back to the hourly cadence, no more polling.
    await vi.advanceTimersByTimeAsync(PENDING_REFETCH_MS * 4);
    expect(callsTo(fetchMock, 'GET', UPDATE).length).toBe(before + 1);
  });
});
