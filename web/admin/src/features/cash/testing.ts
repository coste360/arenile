/** Fixtures of the cash feature tests (not shipped: imported by tests only). */
import { formatCents } from '@arenile/shared/lib/money';
import { EST_ID, ORG_ID, baseRoutes, json, meFor, type Handler } from '../configuration/testing';

export const API = `/api/admin/v1/establishments/${EST_ID}`;

export type CashRole = 'owner' | 'manager' | 'cashier' | 'viewer' | 'beach_staff';

const EXTRA_PERMS: Record<'viewer' | 'beach_staff', string[]> = {
  viewer: ['bookings.read', 'reports.read'],
  beach_staff: ['operations.run', 'bookings.read'],
};

/** Base routes with the given role (configuration helpers cover owner/manager/cashier). */
export function roleRoutes(role: CashRole): Record<string, Handler> {
  if (role === 'viewer' || role === 'beach_staff') {
    const me = meFor('cashier');
    return {
      ...baseRoutes('cashier'),
      'GET /api/admin/v1/auth/me': () =>
        json(200, {
          ...me,
          memberships: [{ organization_id: ORG_ID, role, permissions: EXTRA_PERMS[role] }],
        }),
    };
  }
  return baseRoutes(role);
}

/** Display text of cents as the page renders it (Intl it-IT, non-breaking space). */
export const eur = (cents: number) => formatCents(cents);

/** Text content with normalised spaces, for comparing formatted amounts. */
export const norm = (s: string | null | undefined) =>
  (s ?? '').replace(/[\s\u00a0\u202f]+/g, ' ').trim();

export const openSession = {
  id: 's1',
  register: 'main',
  status: 'open',
  opened_at: '2026-07-15T07:00:00Z',
  opened_by: 'u-self',
  opening_float: 10000,
  expected_totals: {},
  notes: '',
  version: 3,
};

export const openDetail = {
  session: openSession,
  movements: [
    {
      id: 'm1',
      cash_session_id: 's1',
      kind: 'in',
      amount: 2000,
      reason: 'Cambio monete',
      created_at: '2026-07-15T08:00:00Z',
    },
    {
      id: 'm2',
      cash_session_id: 's1',
      kind: 'out',
      amount: 900,
      reason: 'Acquisto ghiaccio',
      created_at: '2026-07-15T09:00:00Z',
    },
  ],
  payments: [
    {
      id: 'p1',
      booking_id: 'b1',
      booking_code: 'AR-7K3Q9',
      provider: 'manual',
      method: 'cash',
      direction: 'in',
      amount: 15000,
      status: 'succeeded',
      created_at: '2026-07-15T08:30:00Z',
    },
    {
      id: 'p2',
      booking_id: 'b2',
      booking_code: 'AR-2B4C6',
      provider: 'manual',
      method: 'card',
      direction: 'in',
      amount: 3000,
      status: 'succeeded',
      created_at: '2026-07-15T08:40:00Z',
    },
    {
      id: 'p3',
      booking_id: 'b1',
      booking_code: 'AR-7K3Q9',
      provider: 'manual',
      method: 'cash',
      direction: 'refund',
      amount: 500,
      status: 'succeeded',
      created_at: '2026-07-15T10:00:00Z',
    },
  ],
  totals_by_method: { cash: 14500, card: 3000 },
};

/** Expected cash of openDetail: 100,00 + 145,00 + 20,00 - 9,00 = 256,00. */
export const EXPECTED_CASH = 25600;

export const closedSession = {
  ...openSession,
  status: 'closed',
  closed_at: '2026-07-15T19:00:00Z',
  closed_by: 'u-self',
  expected_totals: { cash: EXPECTED_CASH, card: 3000 },
  counted_totals: { cash: 25590, card: 3000 },
  difference: -10,
  version: 4,
};

export const booking = {
  id: 'b9',
  code: 'AR-9Z8Y7',
  status: 'confirmed',
  channel: 'counter',
  customer_id: 'c1',
  customer_display_name: 'Giulia Bianchi',
  starts_on: '2026-07-15',
  ends_on: '2026-07-15',
  slots: ['AM', 'PM'],
  formula: 'daily',
  is_season_pass: false,
  total_amount: 4500,
  paid_amount: 1000,
  balance_due: 3500,
  deposit_due: 0,
  spot_labels: ['A12'],
  version: 2,
  created_at: '2026-07-01T10:00:00Z',
};

export const mainRegister = {
  id: 'r-main',
  code: 'main',
  name: 'Cassa principale',
  active: true,
  is_default: true,
  sort_order: 0,
  created_at: '2026-06-01T08:00:00Z',
  updated_at: '2026-06-01T08:00:00Z',
  version: 1,
};

export const barRegister = {
  ...mainRegister,
  id: 'r-bar',
  code: 'bar',
  name: 'Bar spiaggia',
  is_default: false,
  sort_order: 10,
};
