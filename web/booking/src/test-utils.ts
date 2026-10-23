import { ApiClientProvider, createApiClient, type PublicEstablishment } from '@arenile/shared/api';
import { createTestI18n } from '@arenile/shared/i18n';
import { resolveTheme } from '@arenile/shared/theme';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render } from '@testing-library/react';
import { createElement, type ReactNode } from 'react';
import { I18nextProvider } from 'react-i18next';
import { vi } from 'vitest';
import { BookingEnvProvider, type BookingEnv } from './flow/context';
import { PUBLIC_NS, publicItalian } from './i18n';
import { parseBookingLocation } from './location';

/** Test helpers of the public booking app (imported by tests only, never shipped). */

export type Handler = (req: Request) => Response | Promise<Response>;

export const SEAT_ZONE = '00000000-0000-4000-8000-0000000000a1';
export const CARD_ZONE = '00000000-0000-4000-8000-0000000000a2';
export const QTY_ZONE = '00000000-0000-4000-8000-0000000000a3';
export const MAP_ID = '00000000-0000-4000-8000-0000000000b1';
export const SPOT_A1 = '00000000-0000-4000-8000-0000000000c1';
export const SPOT_A2 = '00000000-0000-4000-8000-0000000000c2';
export const SPOT_B1 = '00000000-0000-4000-8000-0000000000c3';
export const PACK_BASE = '00000000-0000-4000-8000-0000000000d1';
export const PACK_VIP = '00000000-0000-4000-8000-0000000000d2';
export const EXTRA_SUNBED = '00000000-0000-4000-8000-0000000000e1';
export const EXTRA_PARKING = '00000000-0000-4000-8000-0000000000e2';
export const BOOKING_ID = '00000000-0000-4000-8000-0000000000f1';
export const TOKEN = 't'.repeat(43);

export type ZoneSet = 'seat' | 'zone' | 'quantity';

export function zonesFor(modes: readonly ZoneSet[]) {
  const all = [
    {
      id: SEAT_ZONE,
      code: 'prima',
      name: { it: 'Prima fila', en: 'Front row' },
      assignment_mode: 'seat' as const,
      map_id: MAP_ID,
      sort_order: 1,
    },
    {
      id: CARD_ZONE,
      code: 'pineta',
      name: { it: 'Pineta' },
      assignment_mode: 'zone' as const,
      sort_order: 2,
    },
    {
      id: QTY_ZONE,
      code: 'lettini',
      name: { it: 'Lettini in riva' },
      assignment_mode: 'quantity' as const,
      sort_order: 3,
    },
  ];
  return all.filter((z) => modes.includes(z.assignment_mode));
}

export function establishmentFixture(
  overrides: Partial<PublicEstablishment> = {},
  modes: readonly ZoneSet[] = ['seat', 'zone', 'quantity'],
): PublicEstablishment {
  return {
    slug: 'lido-demo',
    name: 'Lido Demo',
    timezone: 'Europe/Rome',
    currency: 'EUR',
    default_locale: 'it',
    enabled_locales: ['it'],
    booking_enabled: true,
    contact_phone: '+39 0771 000000',
    address: { street: 'Lungomare 1', city: 'Gaeta' },
    theme: { primary_color: '#0e6ba8' },
    season: { year: 2099, opens_on: '2099-05-01', closes_on: '2099-09-30' },
    online: {
      zones: zonesFor(modes),
      maps: [{ id: MAP_ID, name: { it: 'Spiaggia' } }],
      formulas: [],
      slots: ['FULL', 'AM', 'PM'],
      hold_minutes: 15,
      payment: { provider: 'stripe', online: true, embedded: false, test_mode: true },
      show_prices_on_map: true,
      show_rate_periods: false,
      promo_codes_enabled: true,
      require_phone: false,
      terms: {
        version: '2099-1',
        texts: {
          it: {
            terms: 'Condizioni di prova.',
            privacy: 'Informativa di prova.',
            marketing: 'Newsletter mensile.',
          },
        },
      },
    },
    ...overrides,
  } as PublicEstablishment;
}

export const MAP_DOC = {
  map: {
    id: MAP_ID,
    name: { it: 'Spiaggia' },
    width: 300,
    height: 200,
    grid_size: 10,
    sea_side: 'bottom',
    version: 1,
  },
  spots: [
    {
      id: SPOT_A1,
      zone_id: SEAT_ZONE,
      label: 'A1',
      row_label: 'A',
      kind: 'umbrella',
      x: 50,
      y: 50,
      rotation: 0,
      shape: { type: 'circle', w: 60, h: 60 },
      price_category: 'prima',
    },
    {
      id: SPOT_A2,
      zone_id: SEAT_ZONE,
      label: 'A2',
      row_label: 'A',
      kind: 'umbrella',
      x: 130,
      y: 50,
      rotation: 0,
      shape: { type: 'circle', w: 60, h: 60 },
      price_category: 'prima',
    },
    {
      id: SPOT_B1,
      zone_id: SEAT_ZONE,
      label: 'B1',
      row_label: 'B',
      kind: 'umbrella',
      x: 50,
      y: 130,
      rotation: 0,
      shape: { type: 'circle', w: 60, h: 60 },
      price_category: 'seconda',
    },
  ],
  decorations: [],
};

export function availabilityFixture(opts: { a1Free?: boolean; cardIndicator?: string } = {}) {
  return {
    from: '2099-07-10',
    to: '2099-07-12',
    slots: 'FULL',
    formula: 'multi_day',
    zones: [
      {
        zone_id: SEAT_ZONE,
        assignment_mode: 'seat',
        available: 2,
        indicator: 'few',
        spots: [
          { spot_id: SPOT_A1, available: opts.a1Free ?? true, price_category: 'prima' },
          { spot_id: SPOT_A2, available: false, price_category: 'prima' },
          { spot_id: SPOT_B1, available: true, price_category: 'seconda' },
        ],
      },
      {
        zone_id: CARD_ZONE,
        assignment_mode: 'zone',
        available: 3,
        indicator: opts.cardIndicator ?? 'last',
        from_price: 2500,
      },
      {
        zone_id: QTY_ZONE,
        assignment_mode: 'quantity',
        available: 4,
        indicator: 'plenty',
        from_price: 800,
      },
    ],
    price_legend: [
      { price_category: 'prima', from_price: 3500, tone: 1 },
      { price_category: 'seconda', from_price: 2500, tone: 0 },
    ],
  };
}

export const CATALOG = {
  packs: [
    {
      id: PACK_BASE,
      code: 'base',
      name: { it: 'Base' },
      description: { it: 'Ombrellone e due lettini' },
      components: [
        { code: 'ombrellone', name: { it: 'Ombrellone' }, quantity: 1, icon: 'umbrella' },
        { code: 'lettino', name: { it: 'Lettino' }, quantity: 2, icon: 'sunbed' },
      ],
      allowed_formulas: ['day', 'multi_day'],
      max_per_booking: 4,
      requires_approval: false,
      from_price: 2500,
    },
    {
      id: PACK_VIP,
      code: 'vip',
      name: { it: 'VIP' },
      description: { it: '' },
      components: [{ code: 'daybed', name: { it: 'Daybed' }, quantity: 1 }],
      allowed_formulas: ['day', 'multi_day'],
      max_per_booking: 1,
      requires_approval: true,
      from_price: 6000,
    },
  ],
  extras: [
    {
      id: EXTRA_SUNBED,
      code: 'lettino-extra',
      name: { it: 'Lettino aggiuntivo' },
      description: { it: '' },
      pricing_unit: 'per_day',
      max_quantity: 2,
      attach_to: 'line',
      unit_price: 500,
    },
    {
      id: EXTRA_PARKING,
      code: 'parcheggio',
      name: { it: 'Parcheggio' },
      description: { it: '' },
      pricing_unit: 'per_booking',
      max_quantity: 1,
      attach_to: 'booking',
      unit_price: 1000,
    },
  ],
};

export function quoteFixture(overrides: Record<string, unknown> = {}) {
  return {
    available: true,
    conflicts: [],
    currency: 'EUR',
    total: 7500,
    deposit_due: 0,
    due_now: 7500,
    balance_later: 0,
    items: [{ kind: 'line', label: { it: 'Base · 3 giorni' }, quantity: 1, amount: 7500 }],
    violations: [],
    payment_plan: 'online_full',
    ...overrides,
  };
}

export function summaryFixture(overrides: Record<string, unknown> = {}) {
  return {
    code: 'AR-7Q2K',
    starts_on: '2099-07-10',
    ends_on: '2099-07-12',
    slots: 'FULL',
    formula: 'multi_day',
    lines: [
      {
        pack_id: PACK_BASE,
        pack_name: { it: 'Base' },
        zone_id: CARD_ZONE,
        zone_name: { it: 'Pineta' },
        quantity: 1,
        assignment: 'pending',
        extras: [],
      },
    ],
    extras: [],
    currency: 'EUR',
    total: 7500,
    paid: 7500,
    balance_due: 0,
    deposit_due: 0,
    ...overrides,
  };
}

export function checkoutStateFixture(overrides: Record<string, unknown> = {}) {
  return {
    booking_id: BOOKING_ID,
    status: 'held',
    checkout_state: 'open',
    hold_expires_at: '2099-07-01T10:15:00Z',
    next_action: 'details',
    summary: summaryFixture({ code: undefined, paid: 0, balance_due: 7500 }),
    quote: quoteFixture(),
    ...overrides,
  };
}

export function json(status: number, body: unknown, headers: Record<string, string> = {}) {
  return new Response(status === 204 || status === 202 ? null : JSON.stringify(body), {
    status,
    headers: {
      'Content-Type': status >= 400 ? 'application/problem+json' : 'application/json',
      ...headers,
    },
  });
}

export function problem(status: number, type: string) {
  return json(status, { type, title: type, status });
}

/** Fake public API: "METHOD /path" → handler (query strings ignored). */
export function fakeApi(routes: Record<string, Handler>) {
  const fetchMock = vi.fn(async (req: Request) => {
    const url = new URL(req.url);
    const handler = routes[`${req.method} ${url.pathname}`];
    return handler ? handler(req) : problem(404, 'not_found');
  });
  const api = createApiClient({ baseUrl: 'http://localhost', fetch: fetchMock });
  return { api, fetchMock };
}

/** Requests the double received for "METHOD /path". */
export function callsTo(fetchMock: ReturnType<typeof vi.fn>, method: string, pathname: string) {
  return fetchMock.mock.calls
    .map((c) => c[0] as Request)
    .filter((r) => r.method === method && new URL(r.url).pathname === pathname);
}

export const BASE = '/api/public/v1/lido-demo';

/** Italian i18n with the shell and the `public` namespace. */
export function testI18n() {
  const i18n = createTestI18n();
  i18n.addResourceBundle('it', PUBLIC_NS, publicItalian, true, true);
  return i18n;
}

export function newQueryClient() {
  return new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
}

export function Providers({
  api,
  children,
  i18n = testI18n(),
  queryClient = newQueryClient(),
}: {
  api: ReturnType<typeof createApiClient>;
  children: ReactNode;
  i18n?: ReturnType<typeof testI18n>;
  queryClient?: QueryClient;
}) {
  return createElement(
    I18nextProvider,
    { i18n },
    createElement(ApiClientProvider, {
      client: api,
      children: createElement(QueryClientProvider, { client: queryClient }, children),
    }),
  );
}

/** BookingEnv for rendering a single step. */
export function envFixture(overrides: Partial<BookingEnv> = {}): BookingEnv {
  const establishment = establishmentFixture();
  return {
    slug: 'lido-demo',
    establishment,
    online: establishment.online!,
    theme: resolveTheme(establishment.theme),
    location: parseBookingLocation('/b/lido-demo', ''),
    channel: null,
    win: window,
    preview: false,
    today: '2099-06-01',
    ...overrides,
  };
}

/** Renders `ui` with providers and a BookingEnv (kept across `rerender`). */
export function renderWithEnv(
  ui: ReactNode,
  routes: Record<string, Handler>,
  env: Partial<BookingEnv> = {},
) {
  const { api, fetchMock } = fakeApi(routes);
  const i18n = testI18n();
  const queryClient = newQueryClient();
  const value = envFixture(env);
  const wrapper = ({ children }: { children: ReactNode }) =>
    createElement(Providers, {
      api,
      i18n,
      queryClient,
      children: createElement(BookingEnvProvider, { value }, children),
    });
  const utils = render(ui as React.ReactElement, { wrapper });
  return { ...utils, fetchMock, queryClient };
}
