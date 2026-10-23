import { useApiClient, useEstablishments } from '@arenile/shared/api';
import { useQueryClient } from '@tanstack/react-query';
import { useEffect } from 'react';
import { useEstablishmentId } from '../establishment/context';
import { useOnlineStatus } from '../lib/online';
import { OfflineBanner } from './OfflineBanner';
import { useOfflineRuntime } from './context';

/** Refresh period of the offline copy while the panel stays open online. */
export const PREFETCH_INTERVAL_MS = 10 * 60_000;

/**
 * Offline part of the shell, mounted once in AppShell: tells the runtime
 * which establishment is shown (only its data is kept on the device),
 * keeps the offline copy of today and tomorrow fresh while online (when
 * IndexedDB is available), and
 * shows the offline banners.
 */
export function OfflineShell() {
  const runtime = useOfflineRuntime();
  const establishmentId = useEstablishmentId();
  const online = useOnlineStatus();
  const qc = useQueryClient();
  const client = useApiClient();
  const establishments = useEstablishments({ limit: 200 });
  const timezone = establishments.data?.items.find((e) => e.id === establishmentId)?.timezone;

  useEffect(() => {
    runtime?.setEstablishment(establishmentId);
  }, [runtime, establishmentId]);

  useEffect(() => {
    // Warming up is useful only when the copy survives a reload (IndexedDB).
    if (!runtime?.persistent || !establishmentId || !online) return;
    let cancelled = false;
    const run = () => {
      void import('./prefetch')
        .then((m) =>
          cancelled ? undefined : m.prefetchOfflineData(qc, client, establishmentId, timezone),
        )
        // Best effort: a missing permission or a network error is not shown.
        .catch(() => undefined);
    };
    run();
    const id = setInterval(run, PREFETCH_INTERVAL_MS);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, [runtime, establishmentId, online, timezone, qc, client]);

  return <OfflineBanner />;
}
