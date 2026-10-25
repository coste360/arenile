import type { ApiProblem } from '@arenile/shared/api';
import { call, shouldRetry, useApiClient, type Schemas } from '@arenile/shared/api';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { configKey } from '../configuration/api/resource';
import { newIdempotencyKey } from '../configuration/lib/validation';

export type SetupTemplate = Schemas['SetupTemplate'];
export type ApplyTemplateRequest = Schemas['ApplyTemplateRequest'];
export type ApplyTemplateResult = Schemas['ApplyTemplateResult'];
export type QuickPricelistRequest = Schemas['QuickPricelistRequest'];
export type QuickPricelistResult = Schemas['QuickPricelistResult'];
export type SetupStatus = Schemas['SetupStatus'];

const setupKey = (establishmentId: string) => [...configKey(establishmentId), 'setup'] as const;

export function useSetupTemplates(establishmentId: string) {
  const client = useApiClient();
  return useQuery<SetupTemplate[], ApiProblem>({
    queryKey: [...setupKey(establishmentId), 'templates'],
    queryFn: async () =>
      (
        await call(() =>
          client.GET('/api/admin/v1/establishments/{establishmentId}/setup/templates', {
            params: { path: { establishmentId } },
          }),
        )
      ).items,
    retry: shouldRetry,
    staleTime: 5 * 60_000,
  });
}

export function useSetupStatus(establishmentId: string) {
  const client = useApiClient();
  return useQuery<SetupStatus, ApiProblem>({
    queryKey: [...setupKey(establishmentId), 'status'],
    queryFn: () =>
      call(() =>
        client.GET('/api/admin/v1/establishments/{establishmentId}/setup/status', {
          params: { path: { establishmentId } },
        }),
      ),
    // Only a hint for resuming the wizard: never keep the page waiting on retries.
    retry: false,
  });
}

export function useApplyTemplate(establishmentId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<ApplyTemplateResult, ApiProblem, ApplyTemplateRequest>({
    mutationFn: (body) =>
      call(() =>
        client.POST('/api/admin/v1/establishments/{establishmentId}/setup/apply-template', {
          params: {
            path: { establishmentId },
            header: { 'Idempotency-Key': newIdempotencyKey() },
          },
          body,
        }),
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: configKey(establishmentId) }),
  });
}

export function useQuickPricelist(establishmentId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<QuickPricelistResult, ApiProblem, QuickPricelistRequest>({
    mutationFn: (body) =>
      call(() =>
        client.POST('/api/admin/v1/establishments/{establishmentId}/setup/quick-pricelist', {
          params: {
            path: { establishmentId },
            header: { 'Idempotency-Key': newIdempotencyKey() },
          },
          body,
        }),
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: configKey(establishmentId) }),
  });
}

export type SetupResetScope = Schemas['SetupResetRequest']['scope'];
export type SetupResetResult = Schemas['SetupResetResult'];

/**
 * Reset of the establishment (owner only): its configuration, or everything
 * with the operational data. Every cached query is stale afterwards.
 */
export function useResetSetup(establishmentId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<SetupResetResult, ApiProblem, { scope: SetupResetScope; confirmName: string }>(
    {
      mutationFn: ({ scope, confirmName }) =>
        call(() =>
          client.POST('/api/admin/v1/establishments/{establishmentId}/setup/reset', {
            params: { path: { establishmentId } },
            body: { scope, confirm_name: confirmName },
          }),
        ),
      onSuccess: () => qc.invalidateQueries(),
    },
  );
}
