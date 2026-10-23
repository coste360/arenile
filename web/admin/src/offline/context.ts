import { useMemo, useSyncExternalStore, createContext, useContext } from 'react';
import { useOnlineStatus } from '../lib/online';
import type { OfflineRuntime } from './runtime';
import type { ConnectionState, OfflineOperation } from './types';

/** How often a non-empty queue is retried while the browser reports online. */
export const RETRY_INTERVAL_MS = 30_000;

export const OfflineContext = createContext<OfflineRuntime | null>(null);

/** The offline runtime, or null outside OfflineProvider (e.g. isolated tests). */
export function useOfflineRuntime(): OfflineRuntime | null {
  return useContext(OfflineContext);
}

const EMPTY: readonly OfflineOperation[] = [];
const noopSubscribe = () => () => {};

export interface OfflineStatus {
  online: boolean;
  state: ConnectionState;
  /** Operations of the queue (every establishment), in send order. */
  operations: readonly OfflineOperation[];
  pending: number;
  /** Conflict or failed operations, waiting for the user. */
  rejected: number;
  /** True after a 401: the queue waits for a new login. */
  paused: boolean;
}

/** Connection and queue state for the shell indicator, the banner and the features. */
export function useOfflineStatus(): OfflineStatus {
  const runtime = useOfflineRuntime();
  const online = useOnlineStatus();
  const operations = useSyncExternalStore(
    runtime?.queue.subscribe ?? noopSubscribe,
    runtime?.queue.getSnapshot ?? (() => EMPTY),
  );
  return useMemo(() => {
    const pending = operations.filter(
      (o) => o.status === 'pending' || o.status === 'sending',
    ).length;
    const rejected = operations.filter(
      (o) => o.status === 'conflict' || o.status === 'failed',
    ).length;
    const sending = operations.some((o) => o.status === 'sending');
    const state: ConnectionState = !online ? 'offline' : sending ? 'syncing' : 'online';
    return {
      online,
      state,
      operations,
      pending,
      rejected,
      paused: runtime?.queue.isPaused() ?? false,
    };
  }, [online, operations, runtime]);
}

export interface QueueSheetControl {
  open: boolean;
  setOpen: (open: boolean) => void;
}

export const QueueSheetContext = createContext<QueueSheetControl>({
  open: false,
  setOpen: () => {},
});

/** Opens the list of offline operations (from the indicator or the banner). */
export function useQueueSheet(): QueueSheetControl {
  return useContext(QueueSheetContext);
}
