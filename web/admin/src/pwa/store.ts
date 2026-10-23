/**
 * State of the service worker lifecycle shown to the user (update prompt).
 * A tiny external store so that the registration (register.ts, imported
 * only by main.tsx because it uses the plugin's virtual module) and the
 * React prompt (UpdatePrompt.tsx) stay decoupled and testable.
 */
export interface PwaState {
  /** A new service worker is waiting: the panel can be updated. */
  needRefresh: boolean;
  /** The app shell is precached: the panel can start offline. */
  offlineReady: boolean;
}

export interface PwaController {
  getSnapshot(): PwaState;
  subscribe(listener: () => void): () => void;
  setNeedRefresh(): void;
  setOfflineReady(): void;
  /** Hides the update prompt until the next new version. */
  dismiss(): void;
  /** The "ready offline" notice was shown. */
  ackOfflineReady(): void;
  /** Installed by register.ts: activates the waiting worker and reloads. */
  bindUpdater(update: () => Promise<void>): void;
  /** Activates the new version (no-op before registration). */
  update(): Promise<void>;
}

export function createPwaController(): PwaController {
  let state: PwaState = { needRefresh: false, offlineReady: false };
  let updater: (() => Promise<void>) | null = null;
  const listeners = new Set<() => void>();
  const set = (next: Partial<PwaState>) => {
    state = { ...state, ...next };
    for (const l of listeners) l();
  };
  return {
    getSnapshot: () => state,
    subscribe(listener) {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    setNeedRefresh: () => set({ needRefresh: true }),
    setOfflineReady: () => set({ offlineReady: true }),
    dismiss: () => set({ needRefresh: false }),
    ackOfflineReady: () => set({ offlineReady: false }),
    bindUpdater(update) {
      updater = update;
    },
    async update() {
      await updater?.();
    },
  };
}

/** Controller of the running app (one service worker per page). */
export const pwaController: PwaController = createPwaController();
