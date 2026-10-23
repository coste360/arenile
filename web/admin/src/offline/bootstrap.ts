import { createOfflineRuntime, type OfflineRuntime, type OfflineRuntimeOptions } from './runtime';

/** Longest wait for IndexedDB before rendering anyway (a stuck open must not block the panel). */
const RESTORE_TIMEOUT_MS = 2000;

/**
 * Creates the IndexedDB-backed runtime and restores it, waiting at most
 * RESTORE_TIMEOUT_MS: past that the panel renders and the restore finishes
 * in the background (the queries refetch online anyway).
 */
export async function startOfflineRuntime(
  options: Omit<OfflineRuntimeOptions, 'db'>,
): Promise<OfflineRuntime> {
  const runtime = createOfflineRuntime(options);
  await Promise.race([
    runtime.start().catch(() => undefined),
    new Promise((resolve) => setTimeout(resolve, RESTORE_TIMEOUT_MS)),
  ]);
  return runtime;
}
