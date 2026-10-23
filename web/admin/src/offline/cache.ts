import type { QueryKey } from '@tanstack/react-query';
import { addDays, todayIn } from '../features/operations/lib/dates';

/**
 * What the panel may keep on the device for offline consultation (spec
 * 21.2, 22, 23). Allowlist on TanStack Query keys, applied when saving and
 * again when restoring:
 *
 * - ['auth','me']: the signed-in user, needed to start the shell offline.
 * - ['establishments', …]: the accessible establishments (name, time zone).
 * - ['config', <current>, area, …] for the read-only areas the operational
 *   pages render (maps and map documents, zones, pools, packs, extras,
 *   seasons, closure days). Never 'team' (staff e-mails) or pricing.
 * - ['est', <current>, 'operations', 'day', {date}] for today and tomorrow
 *   in the establishment time zone: map statuses and the bookings of the
 *   day (customer display name, code, balance).
 * - ['est', <current>, 'bookings', 'detail', id] only for bookings whose
 *   stay overlaps today or tomorrow.
 *
 * Everything else (customer lists and details, payments, reports, cash,
 * audit, integrations) stays in memory only. Other establishments are not
 * kept: switching establishment drops them at the next save.
 */

export const OFFLINE_MAX_AGE_MS = 48 * 3_600_000;

/** Read-only configuration areas the operational pages need offline. */
export const OFFLINE_CONFIG_AREAS: ReadonlySet<string> = new Set([
  'maps',
  'zones',
  'pools',
  'packs',
  'extras',
  'seasons',
  'closure-days',
]);

export interface PersistPolicyContext {
  /**
   * Establishment shown in the URL (only its data is kept). At restore
   * time the URL is not parsed yet: the runtime answers `'*'` so that the
   * data of the one establishment the snapshot holds is accepted (the
   * save path never writes another one).
   */
  currentEstablishmentId: () => string | '*' | undefined;
  /** Time zone of an establishment (from the cached list), if known. */
  timezoneOf: (establishmentId: string) => string | undefined;
  now?: () => Date;
}

/** Today and tomorrow ("YYYY-MM-DD") in the establishment time zone. */
export function offlineWindow(timeZone: string | undefined, now: Date): [string, string] {
  const today = todayIn(timeZone, now);
  return [today, addDays(today, 1)];
}

interface Stay {
  starts_on?: unknown;
  ends_on?: unknown;
}

function overlaps(data: unknown, [today, tomorrow]: [string, string]): boolean {
  const s = data as Stay | null;
  if (!s || typeof s.starts_on !== 'string' || typeof s.ends_on !== 'string') return false;
  return s.starts_on <= tomorrow && s.ends_on >= today;
}

/** Builds the allowlist used by the query persister. */
export function createPersistPolicy(ctx: PersistPolicyContext) {
  const now = ctx.now ?? (() => new Date());
  return function shouldPersist(query: { queryKey: QueryKey; state: { data?: unknown } }): boolean {
    const key = query.queryKey;
    const [root, a, b, c] = key;
    if (root === 'auth') return a === 'me';
    if (root === 'establishments') return true;
    const current = ctx.currentEstablishmentId();
    if (!current || typeof a !== 'string' || (current !== '*' && a !== current)) return false;
    if (root === 'config') return typeof b === 'string' && OFFLINE_CONFIG_AREAS.has(b);
    if (root !== 'est') return false;
    const window = offlineWindow(ctx.timezoneOf(a), now());
    if (b === 'operations' && c === 'day') {
      const params = key[4] as { date?: unknown } | undefined;
      return typeof params?.date === 'string' && window.includes(params.date);
    }
    if (b === 'bookings' && c === 'detail') return overlaps(query.state.data, window);
    return false;
  };
}
