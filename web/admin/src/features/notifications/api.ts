import {
  call,
  establishmentKeys,
  shouldRetry,
  useApiClient,
  type ApiProblem,
  type Schemas,
} from '@arenile/shared/api';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

export type AdminNotification = Schemas['Notification'];
export type NotificationList = Schemas['NotificationList'];

const BASE = '/api/admin/v1/establishments/{establishmentId}' as const;

/** Polling interval of the bell (notifications are rare; no SSE topic for them). */
export const NOTIFICATIONS_POLL_MS = 60_000;

export const notificationKeys = {
  all: (est: string) => [...establishmentKeys.all(est), 'notifications'] as const,
};

/** Latest panel notifications (ADR 0019 F) with the unread count. */
export function useNotifications(establishmentId: string | undefined) {
  const client = useApiClient();
  return useQuery<NotificationList, ApiProblem>({
    queryKey: notificationKeys.all(establishmentId ?? ''),
    queryFn: () =>
      call(() =>
        client.GET(`${BASE}/notifications`, {
          params: { path: { establishmentId: establishmentId as string }, query: { limit: 20 } },
        }),
      ),
    enabled: !!establishmentId,
    refetchInterval: NOTIFICATIONS_POLL_MS,
    retry: shouldRetry,
  });
}

export function useMarkNotificationRead(establishmentId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<void, ApiProblem, string | 'all'>({
    mutationFn: async (id) => {
      if (id === 'all') {
        await call(() =>
          client.POST(`${BASE}/notifications/read-all`, {
            params: { path: { establishmentId } },
          }),
        );
        return;
      }
      await call(() =>
        client.POST(`${BASE}/notifications/{notificationId}/read`, {
          params: { path: { establishmentId, notificationId: id } },
        }),
      );
    },
    onSettled: () => qc.invalidateQueries({ queryKey: notificationKeys.all(establishmentId) }),
  });
}
