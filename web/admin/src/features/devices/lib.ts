import type { PanelMode } from '../../lib/meta';
import type { Device, DevicePairingCode } from './api';

/** Pure helpers of the devices feature (tested in lib.test.ts). */

/** Lifetime requested for every code (seconds; the API default). */
export const PAIRING_TTL_SECONDS = 300;

/**
 * Local deadline of a code received at `receivedAt` (browser clock): the
 * requested lifetime counted from the response, never compared with the
 * server `expires_at`, so a skewed browser clock cannot show a wrong
 * countdown. It errs on the safe side (the server started the clock a few
 * milliseconds earlier).
 */
export function localDeadline(receivedAt: number, ttlSeconds = PAIRING_TTL_SECONDS): number {
  return receivedAt + ttlSeconds * 1000;
}

/** "m:ss" left until `deadline`; "0:00" once expired. */
export function formatCountdown(deadline: number, now: number): string {
  const left = Math.max(0, Math.ceil((deadline - now) / 1000));
  const m = Math.floor(left / 60);
  const s = left % 60;
  return `${String(m)}:${String(s).padStart(2, '0')}`;
}

/** True once the code can no longer be exchanged. */
export function isExpired(deadline: number, now: number): boolean {
  return deadline <= now;
}

/**
 * Why pairing is unavailable, if it is. The desktop edition in local mode
 * has no address other devices can reach: the LAN mode must be switched on
 * first (the server answers 409 lan_mode_required anyway). In remote mode
 * the panel belongs to a server instance and pairing works as usual.
 */
export type PairingBlock = 'lan_mode_required' | null;

export function pairingBlock(mode: PanelMode): PairingBlock {
  return mode === 'local' ? 'lan_mode_required' : null;
}

/**
 * The device created with `code`, if it already appears in the list: same
 * user, not revoked, not among the devices known when the code was issued
 * (no clock comparison, so a skewed browser clock does not matter). Used to
 * close the QR dialog as soon as the phone or desktop app has paired.
 */
export function pairedWith(
  devices: readonly Device[],
  code: Pick<DevicePairingCode, 'user_id'>,
  knownIds: ReadonlySet<string>,
): Device | undefined {
  return devices.find((d) => d.user_id === code.user_id && !d.revoked_at && !knownIds.has(d.id));
}

/** Most recent activity first (last use, else pairing), like the API order. */
export function byLastActivity(a: Device, b: Device): number {
  const ta = Date.parse(a.last_used_at ?? a.created_at);
  const tb = Date.parse(b.last_used_at ?? b.created_at);
  return tb - ta || b.id.localeCompare(a.id);
}
