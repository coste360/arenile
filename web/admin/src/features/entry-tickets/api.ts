/**
 * Server state of the entry tickets feature (spec 14.3, ADR 0020): ticket
 * types, entry sales and the gate check-in, under the `tickets` area.
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

export type EntryTicketType = Schemas['EntryTicketType'];
export type EntryTicketTypeList = Schemas['EntryTicketTypeList'];
export type EntryTicketTypeInput = Schemas['EntryTicketTypeInput'];
export type EntrySaleCreate = Schemas['EntrySaleCreate'];
export type GateCheckInResult = Schemas['GateCheckInResult'];

const BASE = '/api/admin/v1/establishments/{establishmentId}' as const;

export const entryTicketKeys = {
  types: (est: string) => [...establishmentKeys.area(est, 'tickets'), 'entry-types'] as const,
};

export function useEntryTicketTypes(establishmentId: string) {
  const client = useApiClient();
  return useQuery<EntryTicketTypeList, ApiProblem>({
    queryKey: entryTicketKeys.types(establishmentId),
    queryFn: () =>
      call(() =>
        client.GET(`${BASE}/entry-ticket-types`, { params: { path: { establishmentId } } }),
      ),
    retry: shouldRetry,
  });
}

/** One scan at the gate: admitted, already_entered, voided or wrong_date. */
export function useGateCheckIn(establishmentId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<GateCheckInResult, ApiProblem, string>({
    mutationFn: (code) =>
      call(() =>
        client.POST(`${BASE}/entry-gate/check-in`, {
          params: { path: { establishmentId } },
          body: { code },
        }),
      ),
    onSettled: () =>
      qc.invalidateQueries({ queryKey: establishmentKeys.area(establishmentId, 'tickets') }),
  });
}
