import {
  call,
  establishmentKeys,
  shouldRetry,
  useApiClient,
  type ApiProblem,
  type Schemas,
} from '@arenile/shared/api';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

export type Device = Schemas['Device'];
export type DevicePairingCode = Schemas['DevicePairingCode'];
export type DevicePairingCodeRequest = Schemas['DevicePairingCodeRequest'];
export type TeamMember = Schemas['TeamMember'];

/** Prefix of every devices query of one establishment. */
export const devicesKey = (establishmentId: string) =>
  [...establishmentKeys.all(establishmentId), 'devices'] as const;

export interface DevicesQueryOptions {
  includeRevoked: boolean;
  /** Poll while a pairing code is on screen, to notice the new device. */
  refetchInterval?: number | false;
}

/** Every paired device visible to the caller (GET devices, all pages). */
export function useDevices(establishmentId: string, opts: DevicesQueryOptions) {
  const client = useApiClient();
  return useQuery<Device[], ApiProblem>({
    queryKey: [...devicesKey(establishmentId), { includeRevoked: opts.includeRevoked }],
    retry: shouldRetry,
    refetchInterval: opts.refetchInterval ?? false,
    queryFn: async () => {
      const items: Device[] = [];
      let cursor: string | undefined;
      for (let i = 0; i < 20; i++) {
        const page = await call(() =>
          client.GET('/api/admin/v1/establishments/{establishmentId}/devices', {
            params: {
              path: { establishmentId },
              query: { limit: 200, cursor, include_revoked: opts.includeRevoked },
            },
          }),
        );
        items.push(...page.items);
        if (!page.next_cursor) break;
        cursor = page.next_cursor;
      }
      return items;
    },
  });
}

/**
 * Creates a single-use pairing code. No Idempotency-Key on purpose: the
 * code is a credential returned once and never stored server side.
 */
export function useCreatePairingCode(establishmentId: string) {
  const client = useApiClient();
  return useMutation<DevicePairingCode, ApiProblem, DevicePairingCodeRequest>({
    mutationFn: (body) =>
      call(() =>
        client.POST('/api/admin/v1/establishments/{establishmentId}/devices/pairing-codes', {
          params: { path: { establishmentId } },
          body,
        }),
      ),
  });
}

/** Revokes a device: its token stops working immediately. */
export function useRevokeDevice(establishmentId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<void, ApiProblem, string>({
    mutationFn: async (deviceId) => {
      await call(() =>
        client.DELETE('/api/admin/v1/establishments/{establishmentId}/devices/{deviceId}', {
          params: { path: { establishmentId, deviceId } },
        }),
      );
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: devicesKey(establishmentId) }),
  });
}

/** Team members (owners pairing a device for someone else). */
export function useTeamMembers(establishmentId: string, enabled: boolean) {
  const client = useApiClient();
  return useQuery<TeamMember[], ApiProblem>({
    queryKey: [...devicesKey(establishmentId), 'team'],
    enabled,
    retry: shouldRetry,
    queryFn: async () => {
      const page = await call(() =>
        client.GET('/api/admin/v1/establishments/{establishmentId}/team', {
          params: { path: { establishmentId }, query: { limit: 200 } },
        }),
      );
      return page.items.filter((m) => m.active);
    },
  });
}
