import { ApiClientProvider, createApiClient } from '@arenile/shared/api';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render } from '@testing-library/react';
import type { ReactElement } from 'react';
import { I18nextProvider } from 'react-i18next';
import { vi } from 'vitest';
import { createAdminTestI18n } from '../../i18n';
import { EST_ID, problem, type Handler } from '../configuration/testing';

/** Test helpers of the bookings, customers and passes features (tests only, not shipped). */

/** Renders `ui` with the API client (fetch double routing "METHOD /path"), TanStack Query and i18n. */
export function renderWithApi(ui: ReactElement, routes: Record<string, Handler> = {}) {
  const fetchMock = vi.fn(async (req: Request) => {
    const url = new URL(req.url);
    const handler = routes[`${req.method} ${url.pathname}`];
    return handler ? handler(req) : problem(404, 'not_found');
  });
  const api = createApiClient({
    baseUrl: 'http://localhost',
    fetch: fetchMock,
    getCsrfToken: () => 'csrf',
  });
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const i18n = createAdminTestI18n();
  const utils = render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={qc}>
        <ApiClientProvider client={api}>{ui}</ApiClientProvider>
      </QueryClientProvider>
    </I18nextProvider>,
  );
  return { fetchMock, queryClient: qc, ...utils };
}

/** Parsed JSON body of a recorded request. */
export async function bodyOf(req: Request): Promise<unknown> {
  return JSON.parse(await req.clone().text());
}

const BASE = `/api/admin/v1/establishments/${EST_ID}`;
export const estPath = (p: string) => `${BASE}${p}`;

export const CUSTOMER = {
  id: 'c0000000-0000-4000-8000-000000000001',
  first_name: 'Mario',
  last_name: 'Rossi',
  display_name: 'Rossi Mario',
  phone: '+393331234567',
  email: 'mario@example.it',
  locale: 'it',
  notes: '',
  tags: ['vip'],
  version: 2,
  created_at: '2026-05-01T08:00:00Z',
  updated_at: '2026-05-01T08:00:00Z',
};

/** A confirmed booking detail with one paid payment (amounts in cents). */
export function bookingDetail(overrides: Record<string, unknown> = {}) {
  return {
    id: 'b0000000-0000-4000-8000-000000000001',
    code: 'AR-7K3Q9',
    status: 'confirmed',
    channel: 'desk',
    customer: CUSTOMER,
    starts_on: '2026-07-14',
    ends_on: '2026-07-16',
    slots: 'FULL',
    formula: 'multi_day',
    is_season_pass: false,
    total_amount: 9000,
    paid_amount: 5000,
    balance_due: 4000,
    deposit_due: 0,
    lines: [
      {
        id: 'l0000000-0000-4000-8000-000000000001',
        pack_id: 'p1',
        pack_code: 'standard',
        zone_id: 'z1',
        spot_id: 's1',
        spot_label: 'A12',
        assignment_status: 'assigned',
        quantity: 1,
        line_total: 9000,
        extras: [],
      },
    ],
    extras: [],
    allocations: [{ date: '2026-07-14', slot: 'AM', kind: 'booking', spot_id: 's1' }],
    payments: [
      {
        id: 'pay1',
        provider: 'manual',
        method: 'cash',
        direction: 'in',
        amount: 5000,
        status: 'succeeded',
        created_at: '2026-07-01T09:00:00Z',
      },
    ],
    history: [
      {
        at: '2026-07-01T09:00:00Z',
        action: 'booking.created',
        actor_type: 'user',
        actor_name: 'Mario Rossi',
      },
    ],
    price_snapshot: {
      engine_version: 1,
      currency: 'EUR',
      lines: [],
      discounts: [],
      rounding: 0,
      total: 9000,
      vat: [],
    },
    policy_snapshot: { rules: {} },
    notes_customer: '',
    notes_internal: '',
    next_statuses: ['checked_in', 'cancelled', 'no_show'],
    version: 4,
    created_at: '2026-07-01T09:00:00Z',
    updated_at: '2026-07-01T09:00:00Z',
    ...overrides,
  };
}
