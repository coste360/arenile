import {
  call,
  establishmentKeys,
  shouldRetry,
  useApiClient,
  type ApiProblem,
  type Schemas,
} from '@arenile/shared/api';
import { useInfiniteQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { ifMatch, newIdempotencyKey } from '../configuration/lib/validation';

export type ImportJob = Schemas['ImportJob'];
export type ImportJobList = Schemas['ImportJobList'];
export type ImportKind = Schemas['ImportKind'];
export type ImportOptions = Schemas['ImportOptions'];
export type ImportReport = Schemas['ImportReport'];
export type ImportIssue = Schemas['ImportIssue'];

const BASE = '/api/admin/v1/establishments/{establishmentId}' as const;

/** Query keys of the imports area; an applied import also invalidates customers and passes. */
export const importKeys = {
  // Not an SSE area: only this page reads it (prefix under the establishment root).
  all: (est: string) => [...establishmentKeys.all(est), 'imports'] as const,
  list: (est: string) => [...importKeys.all(est), 'list'] as const,
};

/** URL of the CSV template (header only) of a kind: a plain download link (cookie session). */
export function templateUrl(establishmentId: string, kind: ImportKind): string {
  return `/api/admin/v1/establishments/${encodeURIComponent(establishmentId)}/imports/templates/${kind}`;
}

/** Import history, newest first ("load more"). */
export function useImportJobs(establishmentId: string) {
  const client = useApiClient();
  return useInfiniteQuery<ImportJobList, ApiProblem>({
    queryKey: importKeys.list(establishmentId),
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam }) =>
      call(() =>
        client.GET(`${BASE}/imports`, {
          params: {
            path: { establishmentId },
            query: { cursor: pageParam as string | undefined, limit: 20 },
          },
        }),
      ),
    getNextPageParam: (last) => last.next_cursor || undefined,
    retry: shouldRetry,
  });
}

function useInvalidateImports(establishmentId: string) {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: importKeys.all(establishmentId) });
}

/** Uploads the raw file (bytes as they are: the server detects UTF-8 or Windows-1252). */
export function useUploadImport(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidateImports(establishmentId);
  return useMutation<ImportJob, ApiProblem, { kind: ImportKind; file: File }>({
    mutationFn: ({ kind, file }) =>
      call(() =>
        client.POST(`${BASE}/imports`, {
          params: {
            path: { establishmentId },
            query: { kind, filename: file.name.slice(0, 200) },
            header: { 'Idempotency-Key': newIdempotencyKey() },
          },
          // The contract types the body as a string; the File is sent untouched.
          body: file as unknown as string,
          bodySerializer: (body: unknown) => body as BodyInit,
          headers: { 'Content-Type': 'text/csv' },
        }),
      ),
    onSuccess: () => invalidate(),
  });
}

export function usePreviewImport(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidateImports(establishmentId);
  return useMutation<ImportJob, ApiProblem, { job: ImportJob; options: ImportOptions }>({
    mutationFn: ({ job, options }) =>
      call(() =>
        client.POST(`${BASE}/imports/{importId}/preview`, {
          params: {
            path: { establishmentId, importId: job.id },
            header: { 'If-Match': ifMatch(job.version) },
          },
          body: options,
        }),
      ),
    onSuccess: () => invalidate(),
  });
}

export function useApplyImport(establishmentId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<ImportJob, ApiProblem, ImportJob>({
    mutationFn: (job) =>
      call(() =>
        client.POST(`${BASE}/imports/{importId}/apply`, {
          params: {
            path: { establishmentId, importId: job.id },
            header: { 'If-Match': ifMatch(job.version) },
          },
        }),
      ),
    // Customers and season passes changed too (or the job became failed).
    onSettled: () => qc.invalidateQueries({ queryKey: establishmentKeys.all(establishmentId) }),
  });
}

/** Reloads one import (its report after a refused apply). */
export function useFetchImport(establishmentId: string) {
  const client = useApiClient();
  return (importId: string) =>
    call(() =>
      client.GET(`${BASE}/imports/{importId}`, {
        params: { path: { establishmentId, importId } },
      }),
    );
}

export function useDiscardImport(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidateImports(establishmentId);
  return useMutation<void, ApiProblem, ImportJob>({
    mutationFn: async (job) => {
      await call(() =>
        client.DELETE(`${BASE}/imports/{importId}`, {
          params: { path: { establishmentId, importId: job.id } },
        }),
      );
    },
    onSuccess: () => invalidate(),
  });
}
