import { ConnectionIndicator } from '../offline/ConnectionIndicator';

/**
 * Connection status, always visible in the top bar (spec 21.2). Kept as the
 * shell's entry point; the implementation (offline, syncing, queued and
 * refused operations) lives in ../offline.
 */
export function OfflineIndicator() {
  return <ConnectionIndicator />;
}
