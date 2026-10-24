import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { EST_ID, baseRoutes, callsTo, json, renderApp } from '../configuration/testing';

const API = `/api/admin/v1/establishments/${EST_ID}`;
const PATH = `/e/${EST_ID}/config/test-mode`;

function state(overrides: Record<string, unknown> = {}) {
  return {
    enabled: false,
    public_enabled: false,
    test_bookings_count: 0,
    stripe_test_configured: false,
    version: 1,
    ...overrides,
  };
}

describe('TestModePage', () => {
  it('switches test mode on with If-Match and shows the shell banner', async () => {
    let current = state();
    const sent: unknown[] = [];
    const { fetchMock } = renderApp(PATH, {
      ...baseRoutes('owner'),
      [`GET ${API}/test-mode`]: () => json(200, current, { ETag: `"${current.version}"` }),
      [`PUT ${API}/test-mode`]: async (req) => {
        const body = (await req.json()) as { enabled: boolean };
        sent.push(body);
        current = state({ enabled: body.enabled, enabled_at: '2027-07-10T08:00:00Z', version: 2 });
        return json(200, current, { ETag: '"2"' });
      },
    });
    const sw = await screen.findByRole('switch', { name: 'Modalità prova attiva' });
    expect(screen.queryByTestId('test-mode-banner')).toBeNull();
    await userEvent.click(sw);
    await waitFor(() => expect(callsTo(fetchMock, 'PUT', `${API}/test-mode`)).toHaveLength(1));
    const req = callsTo(fetchMock, 'PUT', `${API}/test-mode`)[0]!;
    expect(req.headers.get('If-Match')).toBe('"1"');
    expect(sent).toEqual([{ enabled: true, public_enabled: false }]);
    const banner = await screen.findByTestId('test-mode-banner');
    expect(within(banner).getByText('Modalità prova attiva')).toBeTruthy();
  });

  it('purges the test bookings after a dry run and a confirmation', async () => {
    let count = 3;
    const bodies: unknown[] = [];
    const { fetchMock } = renderApp(PATH, {
      ...baseRoutes('owner'),
      [`GET ${API}/test-mode`]: () =>
        json(200, state({ enabled: true, test_bookings_count: count, version: 4 }), {
          ETag: '"4"',
        }),
      [`POST ${API}/test-bookings/purge`]: async (req) => {
        const body = (await req.json()) as { dry_run: boolean };
        bodies.push(body);
        if (body.dry_run) return json(200, { bookings: 3, payments: 2, deleted: false });
        count = 0;
        return json(200, { bookings: 3, payments: 2, deleted: true });
      },
    });
    await userEvent.click(
      await screen.findByRole('button', { name: 'Elimina prenotazioni di prova' }),
    );
    const dialog = await screen.findByRole('alertdialog');
    expect(within(dialog).getByText(/3 prenotazioni e 2 pagamenti di prova/)).toBeTruthy();
    await userEvent.click(within(dialog).getByRole('button', { name: 'Elimina' }));
    await waitFor(() =>
      expect(callsTo(fetchMock, 'POST', `${API}/test-bookings/purge`)).toHaveLength(2),
    );
    expect(bodies).toEqual([{ dry_run: true }, { dry_run: false }]);
  });

  it('is read-only for a cashier', async () => {
    renderApp(PATH, {
      ...baseRoutes('cashier'),
      [`GET ${API}/test-mode`]: () => json(200, state({ test_bookings_count: 2 })),
    });
    const sw = await screen.findByRole('switch', { name: 'Modalità prova attiva' });
    expect(sw).toHaveProperty('disabled', true);
    expect(
      (screen.getByRole('button', { name: 'Elimina prenotazioni di prova' }) as HTMLButtonElement)
        .disabled,
    ).toBe(true);
  });
});
