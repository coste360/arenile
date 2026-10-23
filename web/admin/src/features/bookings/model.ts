import { useMe, type Me, type Schemas } from '@arenile/shared/api';
import { dateToIso, isoToDate } from '@arenile/shared/lib/dates';
import type { StatusTone } from '@arenile/shared/ui/patterns';
import { useCurrentEstablishment } from '../../establishment';

export type BookingStatus = Schemas['BookingStatus'];
export type Channel = Schemas['Channel'];
export type Slots = Schemas['Slots'];
export type Slot = Schemas['Slot'];
export type Formula = Schemas['Formula'];
export type PaymentMethod = Schemas['PaymentMethod'];

export const BOOKING_STATUSES: readonly BookingStatus[] = [
  'draft',
  'held',
  'pending_payment',
  'awaiting_approval',
  'confirmed',
  'checked_in',
  'completed',
  'cancelled',
  'no_show',
  'expired',
  'rejected',
];

export const CHANNELS: readonly Channel[] = ['desk', 'online', 'api'];
export const SLOTS: readonly Slots[] = ['FULL', 'AM', 'PM'];
export const FORMULAS: readonly Formula[] = [
  'half_day',
  'day',
  'multi_day',
  'week',
  'fortnight',
  'month',
  'season',
];
/** Methods an operator records by hand (wallet is set by providers, credit by the ledger). */
export const MANUAL_METHODS: readonly PaymentMethod[] = [
  'cash',
  'card',
  'bank_transfer',
  'credit',
  'other',
];

/** Badge tone of each booking status (spec 10.2). */
export const STATUS_TONE: Record<BookingStatus, StatusTone> = {
  draft: 'neutral',
  held: 'warning',
  pending_payment: 'warning',
  awaiting_approval: 'highlight',
  confirmed: 'success',
  checked_in: 'info',
  completed: 'neutral',
  cancelled: 'danger',
  no_show: 'danger',
  expired: 'neutral',
  rejected: 'danger',
};

/** Statuses that occupy resources: a booking in these can still change. */
export const ACTIVE_STATUSES: readonly BookingStatus[] = [
  'draft',
  'held',
  'pending_payment',
  'awaiting_approval',
  'confirmed',
  'checked_in',
];

/** Lifecycle action of the detail page, enabled by `next_statuses` (server state machine). */
export type BookingAction = 'confirm' | 'approve' | 'reject' | 'check-in' | 'no-show' | 'cancel';

/**
 * Actions offered for a booking. The server is authoritative (it answers
 * 409 invalid_transition otherwise): this only reads its `next_statuses`.
 */
export function availableActions(
  status: BookingStatus,
  nextStatuses: readonly BookingStatus[],
): BookingAction[] {
  const next = new Set(nextStatuses);
  const out: BookingAction[] = [];
  if (status === 'awaiting_approval') {
    if (next.has('confirmed')) out.push('approve');
    if (next.has('rejected')) out.push('reject');
  } else if (next.has('confirmed')) {
    out.push('confirm');
  }
  if (next.has('checked_in')) out.push('check-in');
  if (next.has('no_show')) out.push('no-show');
  if (next.has('cancelled')) out.push('cancel');
  return out;
}

/** Default formula for a stay (the operator may pick a fixed one: week, month...). */
export function defaultFormula(startsOn: string, endsOn: string, slots: Slots): Formula {
  if (startsOn === endsOn) return slots === 'FULL' ? 'day' : 'half_day';
  return 'multi_day';
}

/** Today in the browser's calendar (the server validates in the establishment timezone). */
export function todayIso(now: Date = new Date()): string {
  return dateToIso(now);
}

/** Every date from `from` to `to` inclusive (ISO strings). */
export function eachDate(from: string, to: string): string[] {
  const start = isoToDate(from);
  const end = isoToDate(to);
  if (!start || !end || start > end) return [];
  const out: string[] = [];
  const d = new Date(start);
  while (d <= end && out.length < 400) {
    out.push(dateToIso(d));
    d.setDate(d.getDate() + 1);
  }
  return out;
}

/** Effective operational permissions on the current establishment (spec 16.1). */
export interface OperationalPermissions {
  /** Create and change bookings, customers, passes (owner, manager, cashier). */
  canWrite: boolean;
  /** Record manual payments (cash.operate). */
  canTakePayments: boolean;
  /** Refunds (refunds.issue). */
  canRefund: boolean;
  /** Check-in, no-show, spot moves (operations.run). */
  canOperate: boolean;
}

/**
 * Permissions of `me` on an establishment. The domain services check them
 * again: this only hides actions the user cannot perform.
 */
export function operationalPermissions(
  me: Me | undefined,
  establishment: { id: string; organization_id: string } | undefined,
): OperationalPermissions {
  const granted = new Set<string>();
  if (me && establishment) {
    if (me.principal_type === 'api_key' && me.api_key?.establishment_id === establishment.id) {
      for (const p of me.api_key.scopes) granted.add(p);
    }
    for (const m of me.memberships) {
      const applies = m.establishment_id
        ? m.establishment_id === establishment.id
        : m.organization_id === establishment.organization_id;
      if (applies) for (const p of m.permissions) granted.add(p);
    }
  }
  return {
    canWrite: granted.has('bookings.write'),
    canTakePayments: granted.has('cash.operate'),
    canRefund: granted.has('refunds.issue'),
    canOperate: granted.has('operations.run'),
  };
}

export function useOperationalPermissions(): OperationalPermissions {
  const me = useMe();
  const { establishment } = useCurrentEstablishment();
  return operationalPermissions(me.data, establishment);
}
