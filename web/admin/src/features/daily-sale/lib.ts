import type { StatusTone } from '@arenile/shared/ui/patterns';
import { newIdempotencyKey } from '../configuration/lib/validation';
import type {
  DailySaleCreate,
  DailySaleQuoteRequest,
  TicketAutoAssignResult,
  TicketPaymentMethod,
  TicketPrintFormat,
  TicketState,
} from './api';

/** Daily number label shown on the map and on the receipt ("#037"). */
export function ticketNumberLabel(n: number): string {
  return `#${String(n).padStart(3, '0')}`;
}

/** Map label of a sold spot: the holder name when given, else the number. */
export function ticketMapLabel(t: { ticket_number: number; holder_name?: string }): string {
  return t.holder_name?.trim() ? t.holder_name.trim() : ticketNumberLabel(t.ticket_number);
}

export type SaleWhere = 'spot' | 'zone';
export type SaleSlots = DailySaleQuoteRequest['slots'];
export const SALE_SLOTS: readonly SaleSlots[] = ['FULL', 'AM', 'PM'];
export const PAYMENT_METHODS: readonly TicketPaymentMethod[] = ['cash', 'card'];
export const PRINT_FORMATS: readonly TicketPrintFormat[] = ['thermal_80', 'thermal_58', 'a5', 'a4'];

/** State of the quick sale form (one screen; every field has a default except the place). */
export interface SaleValues {
  date: string;
  where: SaleWhere;
  spotId: string;
  zoneId: string;
  slots: SaleSlots;
  packId: string;
  /** Quantity per extra id (0 = not taken). */
  extras: Record<string, number>;
  lastName: string;
  firstName: string;
  /** Cash received in cents, to show the change (optional). */
  tendered?: number;
}

/** Defaults: the day shown, full day, the first pack, the spot or zone the sale starts from. */
export function saleDefaults(
  date: string,
  opts: { spotId?: string; zoneId?: string; packId?: string; slots?: SaleSlots } = {},
): SaleValues {
  return {
    date,
    where: opts.zoneId && !opts.spotId ? 'zone' : 'spot',
    spotId: opts.spotId ?? '',
    zoneId: opts.zoneId ?? '',
    slots: opts.slots ?? 'FULL',
    packId: opts.packId ?? '',
    extras: {},
    lastName: '',
    firstName: '',
  };
}

/** Quote request of the form, or null while the place or the pack is missing. */
export function toQuoteRequest(v: SaleValues): DailySaleQuoteRequest | null {
  if (!v.packId) return null;
  const place = v.where === 'spot' ? v.spotId : v.zoneId;
  if (!place) return null;
  const extras = Object.entries(v.extras)
    .filter(([, q]) => q > 0)
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([extra_id, quantity]) => ({ extra_id, quantity }));
  return {
    date: v.date,
    slots: v.slots,
    pack_id: v.packId,
    ...(v.where === 'spot' ? { spot_id: v.spotId } : { zone_id: v.zoneId }),
    ...(extras.length > 0 ? { extras } : {}),
  };
}

/**
 * Create request: the quote plus payment, the optional name and the total
 * the operator saw (409 price_changed if the server computes another one).
 */
export function toCreateRequest(
  v: SaleValues,
  method: TicketPaymentMethod,
  opts: { cashSessionId?: string; expectedTotal?: number } = {},
): DailySaleCreate | null {
  const quote = toQuoteRequest(v);
  if (!quote) return null;
  const last = v.lastName.trim();
  const first = v.firstName.trim();
  return {
    ...quote,
    payment: {
      method,
      ...(method === 'cash' && opts.cashSessionId ? { cash_session_id: opts.cashSessionId } : {}),
      ...(method === 'cash' && v.tendered ? { tendered_amount: v.tendered } : {}),
    },
    ...(last || first
      ? {
          holder: {
            ...(last ? { last_name: last } : {}),
            ...(first ? { first_name: first } : {}),
          },
        }
      : {}),
    ...(opts.expectedTotal !== undefined ? { expected_total: opts.expectedTotal } : {}),
  };
}

/** Change to give back for cash, or undefined when nothing (or too little) was entered. */
export function changeDue(total: number, tendered: number | undefined): number | undefined {
  if (!tendered || tendered < total) return undefined;
  return tendered - total;
}

/** "22.00" → "22%", "10" → "10%", "5.5" → "5,5%" (decimal string, never parsed as a float). */
export function vatRateLabel(rate: string, locale = 'it'): string {
  const [int, frac = ''] = rate.split('.');
  const trimmed = frac.replace(/0+$/, '');
  const sep = locale.startsWith('en') ? '.' : ',';
  return `${int}${trimmed ? sep + trimmed : ''}%`;
}

/** Badge tone of a ticket state. */
export const STATE_TONE: Record<TicketState, StatusTone> = {
  assigned: 'success',
  unassigned: 'warning',
  issued: 'info',
  entered: 'neutral',
  voided: 'danger',
};

/** Download link of the day's tickets as CSV (cookie session). */
export function ticketsCsvUrl(establishmentId: string, date: string): string {
  const q = new URLSearchParams({ date, format: 'csv' });
  return `/api/admin/v1/establishments/${establishmentId}/tickets?${q.toString()}`;
}

/** Keyboard shortcuts of the sale screen (desktop), active outside text fields. */
export type SaleShortcut = SaleSlots | SaleWhere | TicketPaymentMethod;
export const SHORTCUT_KEYS: Readonly<Record<string, SaleShortcut>> = {
  '1': 'FULL',
  '2': 'AM',
  '3': 'PM',
  p: 'spot',
  z: 'zone',
  c: 'cash',
  k: 'card',
};

/** Key shown on the button of a shortcut. */
export function shortcutKey(s: SaleShortcut): string {
  const entry = Object.entries(SHORTCUT_KEYS).find(([, v]) => v === s);
  return entry ? entry[0].toUpperCase() : '';
}

/** Shortcut of a key press, or null (modifiers, repeats, or typing in a field). */
export function shortcutFor(e: {
  key: string;
  altKey?: boolean;
  ctrlKey?: boolean;
  metaKey?: boolean;
  repeat?: boolean;
  target?: EventTarget | null;
}): SaleShortcut | null {
  if (e.altKey || e.ctrlKey || e.metaKey || e.repeat) return null;
  if (isTypingTarget(e.target ?? null)) return null;
  return SHORTCUT_KEYS[e.key.toLowerCase()] ?? null;
}

/** True when keys typed into `el` belong to a field (inputs, text areas, editable, listboxes). */
export function isTypingTarget(el: EventTarget | null): boolean {
  if (!el || !(el instanceof HTMLElement)) return false;
  if (el.isContentEditable) return true;
  const tag = el.tagName;
  if (tag === 'TEXTAREA' || tag === 'SELECT') return true;
  if (tag === 'INPUT') {
    const type = (el as HTMLInputElement).type;
    return !['button', 'checkbox', 'radio', 'submit', 'reset'].includes(type);
  }
  const role = el.getAttribute('role');
  return role === 'combobox' || role === 'listbox' || role === 'spinbutton' || role === 'option';
}

/** Counts of an auto-assign run for the toast. */
export function autoAssignSummary(r: TicketAutoAssignResult): { assigned: number; left: number } {
  return { assigned: r.assigned.length, left: r.unassigned.length };
}

/** Idempotency key that stays the same while the request body does not change. */
export class StableKey {
  private body = '';
  private key = '';

  for(body: unknown): string {
    const s = JSON.stringify(body);
    if (s !== this.body || !this.key) {
      this.body = s;
      this.key = newIdempotencyKey();
    }
    return this.key;
  }

  reset(): void {
    this.body = '';
    this.key = '';
  }
}

/** Pack fields that decide where a pack can be sold. */
export interface SalePackFields {
  active: boolean;
  channels?: readonly string[];
  allowed_zone_ids?: readonly string[];
}

/**
 * Packs offered by the daily sale for a spot or zone (`zoneId`, '' while
 * none is chosen): active, sold at the desk, allowed in that zone. Packs
 * limited to quantity zones (entry tickets) never show here: they are sold
 * from the "Ingressi" tab.
 */
export function packsForSale<P extends SalePackFields>(
  packs: readonly P[],
  zoneId: string,
  quantityZoneIds: ReadonlySet<string>,
): P[] {
  return packs.filter((p) => {
    if (!p.active) return false;
    if (p.channels?.length && !p.channels.includes('desk')) return false;
    const allowed = p.allowed_zone_ids ?? [];
    if (!allowed.length) return true;
    return zoneId ? allowed.includes(zoneId) : allowed.some((z) => !quantityZoneIds.has(z));
  });
}
