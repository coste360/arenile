/**
 * Server clock for the hold countdown (spec 12.3 passo 4). Hold expiries
 * come from the server; a phone whose clock is a few minutes off would
 * otherwise see the countdown end early (or never). Every API response's
 * `Date` header updates the offset; jumps below the header's one-second
 * resolution are ignored so the countdown does not flicker.
 */
let offset = 0;

/** Current time in ms on the server's clock (best effort). */
export function serverNow(): number {
  return Date.now() + offset;
}

/** Records the `Date` header of a response received at `localNow`. */
export function observeServerDate(dateHeader: string | null, localNow: number = Date.now()): void {
  if (!dateHeader) return;
  const t = Date.parse(dateHeader);
  if (Number.isNaN(t)) return;
  const next = t - localNow;
  if (Math.abs(next - offset) > 1500) offset = next;
}

/** Test helper. */
export function resetServerClock(): void {
  offset = 0;
}
