import { useApiClient } from '@arenile/shared/api';
import { memoryDatabase } from '@arenile/shared/offline';
import { useQueryClient } from '@tanstack/react-query';
import { useEffect, useMemo, useState, useSyncExternalStore, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { useOnlineStatus } from '../lib/online';
import { UpdatePrompt } from '../pwa/UpdatePrompt';
import { OfflineContext, QueueSheetContext, RETRY_INTERVAL_MS } from './context';
import { OfflineQueueSheet } from './OfflineQueueSheet';
import { createOfflineRuntime, type OfflineRuntime } from './runtime';

export interface OfflineProviderProps {
  /** Runtime created (and restored) in main.tsx; tests may omit it. */
  runtime?: OfflineRuntime;
  children: ReactNode;
}

/**
 * Provides the offline runtime and keeps the queue moving: it sends the
 * queue when the browser comes back online and retries every 30 seconds
 * while operations are pending (the browser may say "online" on a network
 * that does not reach the server). Hosts the list of queued operations and
 * the toasts of each replay. Without a `runtime` prop it creates a
 * memory-only one (tests).
 */
export function OfflineProvider({ runtime, children }: OfflineProviderProps) {
  const { t } = useTranslation('offline');
  const queryClient = useQueryClient();
  const api = useApiClient();
  const [own] = useState(() =>
    runtime
      ? null
      : createOfflineRuntime({ queryClient, api, db: memoryDatabase('arenile-admin-offline') }),
  );
  const value = runtime ?? (own as OfflineRuntime);
  const [sheetOpen, setSheetOpen] = useState(false);
  const sheet = useMemo(() => ({ open: sheetOpen, setOpen: setSheetOpen }), [sheetOpen]);

  useEffect(() => {
    if (!own) return;
    void own.start();
    return () => own.stop();
  }, [own]);

  useEffect(
    () =>
      value.onFlushed((r) => {
        if (r.sent > 0) toast.success(t('sync.sent', { count: r.sent }));
        const rejected = r.conflicts + r.failed;
        if (rejected > 0) {
          toast.error(t('banner.rejected', { count: rejected }), {
            action: { label: t('banner.review'), onClick: () => setSheetOpen(true) },
          });
        }
      }),
    [value, t],
  );

  const online = useOnlineStatus();
  const ops = useSyncExternalStore(value.queue.subscribe, value.queue.getSnapshot);
  const hasPending = ops.some((o) => o.status === 'pending');

  useEffect(() => {
    if (online) void value.flush();
  }, [online, value]);

  useEffect(() => {
    if (!online || !hasPending) return;
    const id = setInterval(() => void value.flush(), RETRY_INTERVAL_MS);
    return () => clearInterval(id);
  }, [online, hasPending, value]);

  return (
    <OfflineContext.Provider value={value}>
      <QueueSheetContext.Provider value={sheet}>
        {children}
        <OfflineQueueSheet open={sheetOpen} onOpenChange={setSheetOpen} />
        <UpdatePrompt />
      </QueueSheetContext.Provider>
    </OfflineContext.Provider>
  );
}
