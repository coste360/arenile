/**
 * Event filters of a webhook endpoint (spec 18.1, 18.2): a WebhookEventType,
 * a group `<prefix>.*` or `*` (every event). The picker offers all three and
 * keeps the selection minimal: choosing `*` or a group replaces what it
 * covers, choosing a single event drops the broader filter that covered it.
 */
import type { WebhookEventType } from '../api';

/** Subscribable events, in spec order (`webhook.test` is sent only by the test button). */
export const WEBHOOK_EVENTS = [
  'booking.created',
  'booking.held',
  'booking.confirmed',
  'booking.modified',
  'booking.cancelled',
  'booking.checked_in',
  'booking.no_show',
  'booking.completed',
  'booking.approval_requested',
  'allocation.released',
  'season.day_released',
  'season.renewal_offered',
  'payment.succeeded',
  'payment.failed',
  'payment.refunded',
  'customer.created',
  'customer.anonymized',
  'cash_session.closed',
  'config.pricing_changed',
  'config.map_changed',
] as const satisfies readonly Exclude<WebhookEventType, 'webhook.test'>[];

export const ALL_EVENTS = '*';

const prefixOf = (event: string) => event.slice(0, event.indexOf('.'));

/** Groups in spec order: booking, allocation, season, payment, customer, cash_session, config. */
export const EVENT_GROUPS: readonly string[] = [...new Set(WEBHOOK_EVENTS.map(prefixOf))];

/** Every filter value in display order: `*`, then each group followed by its events. */
export const EVENT_FILTER_ORDER: readonly string[] = [
  ALL_EVENTS,
  ...EVENT_GROUPS.flatMap((g) => [`${g}.*`, ...WEBHOOK_EVENTS.filter((e) => prefixOf(e) === g)]),
];

const isGroup = (f: string) => f.endsWith('.*');

/**
 * Next selection after the picker changed from `prev` to `next`: the added
 * filter wins over what overlaps it. Result in EVENT_FILTER_ORDER.
 */
export function nextEventSelection(prev: readonly string[], next: readonly string[]): string[] {
  const added = next.filter((f) => !prev.includes(f));
  let out = [...next];
  for (const f of added) {
    if (f === ALL_EVENTS) {
      out = [ALL_EVENTS];
      break;
    }
    out = out.filter((x) => x !== ALL_EVENTS);
    if (isGroup(f)) {
      const g = prefixOf(f);
      out = out.filter((x) => x === f || isGroup(x) || prefixOf(x) !== g);
    } else {
      out = out.filter((x) => x !== `${prefixOf(f)}.*`);
    }
  }
  return EVENT_FILTER_ORDER.filter((f) => out.includes(f)).concat(
    out.filter((f) => !EVENT_FILTER_ORDER.includes(f)),
  );
}

/** True when `filters` delivers `event` (same matching as the server). */
export function filtersMatch(filters: readonly string[], event: string): boolean {
  return filters.some(
    (f) => f === ALL_EVENTS || f === event || (isGroup(f) && prefixOf(f) === prefixOf(event)),
  );
}
