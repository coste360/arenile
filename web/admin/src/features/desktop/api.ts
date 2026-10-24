import {
  call,
  shouldRetry,
  useApiClient,
  type ApiProblem,
  type components,
} from '@arenile/shared/api';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useEffect } from 'react';

export type DesktopSettings = components['schemas']['DesktopSettings'];
export type DesktopBackup = components['schemas']['DesktopBackup'];
export type DesktopBackupList = components['schemas']['DesktopBackupList'];
export type DesktopRestoreScheduled = components['schemas']['DesktopRestoreScheduled'];

export const desktopKeys = {
  settings: ['desktop', 'settings'] as const,
  backups: ['desktop', 'backups'] as const,
};

/** Desktop app settings (GET /api/desktop/v1/settings, owner, loopback). */
export function useDesktopSettings() {
  const client = useApiClient();
  return useQuery<DesktopSettings, ApiProblem>({
    queryKey: desktopKeys.settings,
    queryFn: () => call(() => client.GET('/api/desktop/v1/settings')),
    retry: shouldRetry,
  });
}

export function useUpdateDesktopSettings() {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<DesktopSettings, ApiProblem, DesktopSettings>({
    mutationFn: (body) => call(() => client.PUT('/api/desktop/v1/settings', { body })),
    onSuccess: (s) => {
      qc.setQueryData(desktopKeys.settings, s);
      // The backup folder or the LAN mode may have changed.
      void qc.invalidateQueries({ queryKey: desktopKeys.backups });
      void qc.invalidateQueries({ queryKey: ['instance', 'meta'] });
      // The update check may have been switched (ADR 0025).
      void qc.invalidateQueries({ queryKey: ['instance', 'update'] });
    },
  });
}

/** Backups in the backup folder, newest first. */
export function useDesktopBackups() {
  const client = useApiClient();
  return useQuery<DesktopBackupList, ApiProblem>({
    queryKey: desktopKeys.backups,
    queryFn: () => call(() => client.GET('/api/desktop/v1/backups')),
    retry: shouldRetry,
  });
}

export function useCreateDesktopBackup() {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<DesktopBackup, ApiProblem, void>({
    mutationFn: () => call(() => client.POST('/api/desktop/v1/backups')),
    onSuccess: () => void qc.invalidateQueries({ queryKey: desktopKeys.backups }),
  });
}

export function useRestoreDesktopBackup() {
  const client = useApiClient();
  return useMutation<DesktopRestoreScheduled, ApiProblem, string>({
    mutationFn: (backupName) =>
      call(() =>
        client.POST('/api/desktop/v1/backups/{backupName}/restore', {
          params: { path: { backupName } },
        }),
      ),
  });
}

/**
 * After a restore the launcher restarts the internal server: wait for it to
 * answer again (GET /meta) and reload the panel on the restored data.
 */
export function useReloadWhenServerIsBack(
  active: boolean,
  reload: () => void = () => window.location.reload(),
) {
  const client = useApiClient();
  useEffect(() => {
    if (!active) return;
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout>;
    const poll = async (attempt: number) => {
      if (cancelled) return;
      try {
        await call(() => client.GET('/api/admin/v1/meta'));
        if (attempt > 0) {
          reload();
          return;
        }
      } catch {
        // Server restarting: try again.
      }
      timer = setTimeout(() => void poll(attempt + 1), 1000);
    };
    // The first answer may still come from the old server: start after a pause.
    timer = setTimeout(() => void poll(0), 1500);
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [active, client, reload]);
}
