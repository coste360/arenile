import type { ApiProblem } from '@arenile/shared/api';
import { call, shouldRetry, useApiClient, type Schemas } from '@arenile/shared/api';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { newIdempotencyKey } from '../lib/validation';
import { configKey } from './resource';

export type TeamMember = Schemas['TeamMember'];
export type TeamMemberCreate = Schemas['TeamMemberCreate'];
export type TeamMemberCreated = Schemas['TeamMemberCreated'];
export type TeamMemberUpdate = Schemas['TeamMemberUpdate'];
export type Role = Schemas['Role'];

const teamKey = (establishmentId: string) => [...configKey(establishmentId), 'team'] as const;

export function useTeam(establishmentId: string, enabled = true) {
  const client = useApiClient();
  return useQuery<TeamMember[], ApiProblem>({
    queryKey: teamKey(establishmentId),
    enabled,
    retry: shouldRetry,
    queryFn: async () => {
      const items: TeamMember[] = [];
      let cursor: string | undefined;
      for (let i = 0; i < 50; i++) {
        const page = await call(() =>
          client.GET('/api/admin/v1/establishments/{establishmentId}/team', {
            params: { path: { establishmentId }, query: { limit: 200, cursor } },
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

export function useInviteMember(establishmentId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<TeamMemberCreated, ApiProblem, TeamMemberCreate>({
    mutationFn: (body) =>
      call(() =>
        client.POST('/api/admin/v1/establishments/{establishmentId}/team', {
          params: {
            path: { establishmentId },
            header: { 'Idempotency-Key': newIdempotencyKey() },
          },
          body,
        }),
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: teamKey(establishmentId) }),
  });
}

export function useUpdateMember(establishmentId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<TeamMember, ApiProblem, { userId: string; patch: TeamMemberUpdate }>({
    mutationFn: ({ userId, patch }) =>
      call(() =>
        client.PATCH('/api/admin/v1/establishments/{establishmentId}/team/{userId}', {
          params: { path: { establishmentId, userId } },
          body: patch,
        }),
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: teamKey(establishmentId) }),
  });
}

export function useRemoveMember(establishmentId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<void, ApiProblem, string>({
    mutationFn: async (userId) => {
      await call(() =>
        client.DELETE('/api/admin/v1/establishments/{establishmentId}/team/{userId}', {
          params: { path: { establishmentId, userId } },
        }),
      );
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: teamKey(establishmentId) }),
  });
}
