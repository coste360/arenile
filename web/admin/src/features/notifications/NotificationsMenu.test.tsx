import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { EST_ID, baseRoutes, callsTo, json, list, renderApp } from '../configuration/testing';

const API = `/api/admin/v1/establishments/${EST_ID}`;

describe('NotificationsMenu', () => {
  it('shows the unread count, the composed text and marks one as read', async () => {
    const user = userEvent.setup();
    let readAt: string | undefined;
    const { fetchMock } = renderApp(`/e/${EST_ID}/config/test-mode`, {
      ...baseRoutes('owner'),
      [`GET ${API}/test-mode`]: () =>
        json(200, { enabled: false, public_enabled: false, test_bookings_count: 0, version: 1 }),
      [`GET ${API}/notifications`]: () =>
        json(200, {
          unread_count: readAt ? 0 : 1,
          items: [
            {
              id: 'n1',
              kind: 'webhook_endpoint_disabled',
              severity: 'error',
              params: { endpoint_id: 'e1', host: 'hooks.example.org', failures: '10' },
              resource_type: 'webhook_endpoint',
              resource_id: 'e1',
              read_at: readAt,
              created_at: '2027-07-10T08:00:00Z',
            },
          ],
        }),
      [`POST ${API}/notifications/n1/read`]: () => {
        readAt = '2027-07-10T09:00:00Z';
        return new Response(null, { status: 204 });
      },
      [`GET ${API}/webhook-endpoints`]: () => list([]),
    });
    const bell = await screen.findByRole('button', { name: 'Notifiche: 1 da leggere' });
    await user.click(bell);
    expect(
      await screen.findByText(/L'endpoint hooks.example.org è stato disattivato dopo 10 consegne/),
    ).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /Webhook disattivato/ }));
    await waitFor(() =>
      expect(callsTo(fetchMock, 'POST', `${API}/notifications/n1/read`)).toHaveLength(1),
    );
    expect(await screen.findByRole('button', { name: 'Notifiche' })).toBeInTheDocument();
  });
});
