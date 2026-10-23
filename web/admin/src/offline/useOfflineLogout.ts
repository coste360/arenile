import { useMemo } from 'react';
import { useOfflineRuntime } from './context';

export interface OfflineLogout {
  /** Sends what is pending while the session is still valid (bounded wait). */
  beforeLogout(): Promise<void>;
  /** Deletes the offline copy, the queue and runtime caches of this browser. */
  clearAll(): Promise<void>;
}

/**
 * Logout steps of the offline mode (spec 22/23: no personal data left on a
 * shared device). Call beforeLogout() before the logout request and
 * clearAll() when it settles, whatever the outcome.
 */
export function useOfflineLogout(): OfflineLogout {
  const runtime = useOfflineRuntime();
  return useMemo(
    () => ({
      beforeLogout: () => runtime?.beforeLogout() ?? Promise.resolve(),
      clearAll: () => runtime?.clearAll() ?? Promise.resolve(),
    }),
    [runtime],
  );
}
