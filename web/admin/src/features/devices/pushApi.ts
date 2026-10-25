/**
 * Server state of the push notification settings of the establishment
 * (ADR 0027): the manager's opt-in, off by default. The tokens of the phones
 * are registered by the mobile app itself and never reach the panel.
 */
import {
  call,
  establishmentKeys,
  shouldRetry,
  useApiClient,
  type ApiProblem,
  type Schemas,
} from '@arenile/shared/api';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ifMatch } from '../configuration/lib/validation';

export type PushSettings = Schemas['PushSettings'];
export type PushSettingsInput = Schemas['PushSettingsInput'];
export type PushEvents = Schemas['PushEvents'];

const PATH = '/api/admin/v1/establishments/{establishmentId}/push-settings' as const;

export const pushSettingsKey = (establishmentId: string) =>
  [...establishmentKeys.all(establishmentId), 'push-settings'] as const;

export function usePushSettings(establishmentId: string, enabled: boolean) {
  const client = useApiClient();
  return useQuery<PushSettings, ApiProblem>({
    queryKey: pushSettingsKey(establishmentId),
    enabled,
    retry: shouldRetry,
    queryFn: () => call(() => client.GET(PATH, { params: { path: { establishmentId } } })),
  });
}

export function useUpdatePushSettings(establishmentId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<PushSettings, ApiProblem, { version: number; body: PushSettingsInput }>({
    mutationFn: ({ version, body }) =>
      call(() =>
        client.PUT(PATH, {
          params: { path: { establishmentId }, header: { 'If-Match': ifMatch(version) } },
          body,
        }),
      ),
    onSuccess: (s) => qc.setQueryData(pushSettingsKey(establishmentId), s),
  });
}
