import { createApiClient } from '@arenile/shared/api';
import { createMemoryHistory } from '@tanstack/react-router';
import { render } from '@testing-library/react';
import { vi } from 'vitest';
import { App } from '../../App';
import { createAdminTestI18n } from '../../i18n';
import type { QueryClient } from '@tanstack/react-query';
import { createQueryClient } from '../../lib/queryClient';

/** Test helpers for the configuration and setup-wizard features (not shipped: tests only). */

export type Handler = (req: Request) => Response | Promise<Response>;

export const EST_ID = '11111111-1111-4111-8111-111111111111';
export const ORG_ID = '22222222-2222-4222-8222-222222222222';

const OWNER_PERMS = [
  'establishment.configure',
  'users.manage',
  'payments.configure',
  'bookings.write',
  'cash.operate',
  'refunds.issue',
  'operations.run',
  'bookings.read',
  'reports.read',
];
const MANAGER_PERMS = OWNER_PERMS.filter((p) => p !== 'users.manage' && p !== 'payments.configure');
const CASHIER_PERMS = ['bookings.write', 'cash.operate', 'operations.run', 'bookings.read'];

export type TestRole = 'owner' | 'manager' | 'cashier';

export function meFor(role: TestRole) {
  const permissions =
    role === 'owner' ? OWNER_PERMS : role === 'manager' ? MANAGER_PERMS : CASHIER_PERMS;
  return {
    principal_type: 'user',
    user: {
      id: 'u-self',
      email: 'gestore@example.it',
      name: 'Mario Rossi',
      locale: 'it',
      totp_enabled: false,
    },
    memberships: [{ organization_id: ORG_ID, role, permissions }],
    totp_enrolment_required: false,
  };
}

export function establishment(overrides: Record<string, unknown> = {}) {
  return {
    id: EST_ID,
    organization_id: ORG_ID,
    slug: 'lido-demo',
    name: 'Lido Demo',
    timezone: 'Europe/Rome',
    currency: 'EUR',
    default_locale: 'it',
    enabled_locales: ['it', 'en'],
    contact_email: 'info@lido.it',
    contact_phone: '',
    address: { city: 'Gaeta', country: 'IT' },
    booking_enabled: false,
    allowed_embed_origins: [],
    theme: { primary_color: '#0e6ba8', accent_color: '#f2b134' },
    settings: {
      pricing: { half_day_factor: '0.6', rounding: 'none', default_vat_rate_bp: 2200 },
      default_policy: {},
    },
    version: 3,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

export function json(status: number, body: unknown, headers: Record<string, string> = {}) {
  return new Response(status === 204 ? null : JSON.stringify(body), {
    status,
    headers: {
      'Content-Type': status >= 400 ? 'application/problem+json' : 'application/json',
      ...headers,
    },
  });
}

export function problem(status: number, type: string, extra: Record<string, unknown> = {}) {
  return json(status, { type, title: type, status, ...extra });
}

export const list = (items: unknown[]) => json(200, { items });

/** Base routes every establishment-scoped page needs. */
export function baseRoutes(role: TestRole, est = establishment()): Record<string, Handler> {
  return {
    'GET /api/admin/v1/auth/me': () => json(200, meFor(role)),
    'GET /api/admin/v1/establishments': () => list([est]),
    [`GET /api/admin/v1/establishments/${EST_ID}`]: () =>
      json(200, est, { ETag: `"${String(est.version)}"` }),
  };
}

/** Renders the whole admin app at `path` with a fetch double routing "METHOD /path". */
export function renderApp(
  path: string,
  routes: Record<string, Handler>,
  queryClient: QueryClient = createQueryClient(),
) {
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
  const history = createMemoryHistory({ initialEntries: [path] });
  const utils = render(
    <App api={api} i18n={createAdminTestI18n()} queryClient={queryClient} history={history} />,
  );
  return { fetchMock, history, ...utils };
}

/** Requests the double received for "METHOD /path". */
export function callsTo(fetchMock: ReturnType<typeof vi.fn>, method: string, pathname: string) {
  return fetchMock.mock.calls
    .map((c) => c[0] as Request)
    .filter((r) => r.method === method && new URL(r.url).pathname === pathname);
}
