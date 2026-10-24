/**
 * Server state of the printers feature (ESC/POS network printers, ADR 0021):
 * TanStack Query hooks on the generated client. Every query lives under
 * [...establishmentKeys.all(id), 'printing'] (printing is not an SSE area, as
 * for the devices feature) so one invalidation refreshes the printers, the
 * routes and the job list together.
 *
 * Creations carry an Idempotency-Key, edits of a printer and of the route
 * set carry If-Match (412 version_conflict when stale).
 */
import {
  call,
  establishmentKeys,
  shouldRetry,
  useApiClient,
  type ApiClient,
  type ApiProblem,
  type Schemas,
} from '@arenile/shared/api';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ifMatch, newIdempotencyKey } from '../configuration/lib/validation';

export type Printer = Schemas['Printer'];
export type PrinterInput = Schemas['PrinterInput'];
export type PrinterList = Schemas['PrinterList'];
export type PrinterCheckResult = Schemas['PrinterCheckResult'];
export type PrinterStatus = Schemas['PrinterStatus'];
export type PrinterCodepage = Schemas['PrinterCodepage'];
export type PrinterCutMode = Schemas['PrinterCutMode'];
export type PrintRoute = Schemas['PrintRoute'];
export type PrintRouteSet = Schemas['PrintRouteSet'];
export type PrintRouteResolution = Schemas['PrintRouteResolution'];
export type PrintRouteUnreadyReason = Schemas['PrintRouteUnreadyReason'];
export type PrintDocumentKind = Schemas['PrintDocumentKind'];
export type PrintJob = Schemas['PrintJob'];
export type PrintJobInput = Schemas['PrintJobInput'];
export type PrintJobStatus = Schemas['PrintJobStatus'];
export type CashRegister = Schemas['CashRegister'];

const base = '/api/admin/v1/establishments/{establishmentId}' as const;

export const printingKeys = {
  all: (est: string) => [...establishmentKeys.all(est), 'printing'] as const,
  printers: (est: string, includeInactive: boolean) =>
    [...establishmentKeys.all(est), 'printing', 'printers', { includeInactive }] as const,
  routes: (est: string) => [...establishmentKeys.all(est), 'printing', 'routes'] as const,
  resolve: (est: string, kind: PrintDocumentKind, registerId: string | undefined) =>
    [...establishmentKeys.all(est), 'printing', 'resolve', kind, registerId ?? ''] as const,
  jobs: (est: string) => [...establishmentKeys.all(est), 'printing', 'jobs'] as const,
  job: (est: string, id: string) => [...establishmentKeys.all(est), 'printing', 'job', id] as const,
  registers: (est: string) => [...establishmentKeys.all(est), 'printing', 'registers'] as const,
};

function useInvalidatePrinting(establishmentId: string) {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: printingKeys.all(establishmentId) });
}

/** Printers of the establishment plus whether ESC/POS is available on this edition. */
export function usePrinters(establishmentId: string, includeInactive = true) {
  const client = useApiClient();
  return useQuery<PrinterList, ApiProblem>({
    queryKey: printingKeys.printers(establishmentId, includeInactive),
    retry: shouldRetry,
    queryFn: () =>
      call(() =>
        client.GET(`${base}/printers`, {
          params: { path: { establishmentId }, query: { include_inactive: includeInactive } },
        }),
      ),
  });
}

export function useCreatePrinter(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidatePrinting(establishmentId);
  return useMutation<Printer, ApiProblem, PrinterInput>({
    mutationFn: (body) =>
      call(() =>
        client.POST(`${base}/printers`, {
          params: {
            path: { establishmentId },
            header: { 'Idempotency-Key': newIdempotencyKey() },
          },
          body,
        }),
      ),
    onSuccess: () => invalidate(),
  });
}

export interface UpdatePrinterVars {
  id: string;
  version: number;
  input: PrinterInput;
}

export function useUpdatePrinter(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidatePrinting(establishmentId);
  return useMutation<Printer, ApiProblem, UpdatePrinterVars>({
    mutationFn: ({ id, version, input }) =>
      call(() =>
        client.PUT(`${base}/printers/{printerId}`, {
          params: {
            path: { establishmentId, printerId: id },
            header: { 'If-Match': ifMatch(version) },
          },
          body: input,
        }),
      ),
    onSuccess: () => invalidate(),
  });
}

/** Deletes a printer: its routes go with it, its queued jobs become cancelled. */
export function useDeletePrinter(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidatePrinting(establishmentId);
  return useMutation<void, ApiProblem, { id: string; version: number }>({
    mutationFn: async ({ id, version }) => {
      await call(() =>
        client.DELETE(`${base}/printers/{printerId}`, {
          params: {
            path: { establishmentId, printerId: id },
            header: { 'If-Match': ifMatch(version) },
          },
        }),
      );
    },
    onSuccess: () => invalidate(),
  });
}

/** Opens a TCP connection to the printer and reads its status (DLE EOT when supported). */
export function useCheckPrinter(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidatePrinting(establishmentId);
  return useMutation<PrinterCheckResult, ApiProblem, string>({
    mutationFn: (printerId) =>
      call(() =>
        client.POST(`${base}/printers/{printerId}/check`, {
          params: { path: { establishmentId, printerId } },
        }),
      ),
    onSettled: () => invalidate(),
  });
}

/** Queues the test page on one printer (ignores routing). */
export function useTestPrinter(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidatePrinting(establishmentId);
  return useMutation<PrintJob, ApiProblem, string>({
    mutationFn: (printerId) =>
      call(() =>
        client.POST(`${base}/printers/{printerId}/test-print`, {
          params: { path: { establishmentId, printerId } },
        }),
      ),
    onSettled: () => invalidate(),
  });
}

/** The route set with the version of its ETag (sent back as If-Match). */
export interface VersionedRouteSet {
  items: PrintRoute[];
  /** Raw ETag of the set, or undefined when the server sent none. */
  etag: string | undefined;
}

async function fetchRoutes(client: ApiClient, establishmentId: string): Promise<VersionedRouteSet> {
  let etag: string | undefined;
  const set = await call(async () => {
    const res = await client.GET(`${base}/print-routes`, {
      params: { path: { establishmentId } },
    });
    etag = res.response.headers.get('ETag') ?? undefined;
    return res;
  });
  return { items: set.items, etag };
}

export function usePrintRoutes(establishmentId: string, enabled = true) {
  const client = useApiClient();
  return useQuery<VersionedRouteSet, ApiProblem>({
    queryKey: printingKeys.routes(establishmentId),
    enabled,
    retry: shouldRetry,
    queryFn: () => fetchRoutes(client, establishmentId),
  });
}

/** Replaces the whole route set (If-Match on the ETag of the set read before). */
export function useUpdatePrintRoutes(establishmentId: string) {
  const client = useApiClient();
  const qc = useQueryClient();
  return useMutation<VersionedRouteSet, ApiProblem, VersionedRouteSet>({
    mutationFn: async ({ items, etag }) => {
      let next: string | undefined;
      const set = await call(async () => {
        const res = await client.PUT(`${base}/print-routes`, {
          params: {
            path: { establishmentId },
            header: etag ? { 'If-Match': etag } : {},
          },
          body: { items },
        });
        next = res.response.headers.get('ETag') ?? undefined;
        return res;
      });
      return { items: set.items, etag: next };
    },
    onSuccess: (saved) => {
      qc.setQueryData(printingKeys.routes(establishmentId), saved);
      void qc.invalidateQueries({ queryKey: printingKeys.all(establishmentId) });
    },
  });
}

/** Recent print jobs (first page); polls while some job is still queued or printing. */
export function usePrintJobs(establishmentId: string, enabled = true) {
  const client = useApiClient();
  return useQuery<PrintJob[], ApiProblem>({
    queryKey: printingKeys.jobs(establishmentId),
    enabled,
    retry: shouldRetry,
    refetchInterval: (q) =>
      q.state.data?.some((j) => j.status === 'queued' || j.status === 'printing') ? 2000 : false,
    queryFn: async () => {
      const page = await call(() =>
        client.GET(`${base}/print-jobs`, {
          params: { path: { establishmentId }, query: { limit: 20 } },
        }),
      );
      return page.items;
    },
  });
}

export function useRetryPrintJob(establishmentId: string) {
  const client = useApiClient();
  const invalidate = useInvalidatePrinting(establishmentId);
  return useMutation<PrintJob, ApiProblem, string>({
    mutationFn: (printJobId) =>
      call(() =>
        client.POST(`${base}/print-jobs/{printJobId}/retry`, {
          params: { path: { establishmentId, printJobId } },
        }),
      ),
    onSettled: () => invalidate(),
  });
}

/**
 * Cash registers for the routing rules. Optional: when the list cannot be
 * read (permissions, older server) the routing offers the default rules only.
 */
export function useCashRegisters(establishmentId: string, enabled = true) {
  const client = useApiClient();
  return useQuery<CashRegister[], ApiProblem>({
    queryKey: printingKeys.registers(establishmentId),
    enabled,
    retry: false,
    queryFn: async () => {
      const page = await call(() =>
        client.GET(`${base}/cash-registers`, { params: { path: { establishmentId } } }),
      );
      return page.items.filter((r) => r.active);
    },
  });
}

// ---- Imperative calls used by the print-or-fallback helper ------------------------------

export function resolveRoute(
  client: ApiClient,
  establishmentId: string,
  kind: PrintDocumentKind,
  registerId: string | undefined,
): Promise<PrintRouteResolution> {
  return call(() =>
    client.GET(`${base}/print-routes/resolve`, {
      params: {
        path: { establishmentId },
        query: { document_kind: kind, register_id: registerId },
      },
    }),
  );
}

export function createPrintJob(
  client: ApiClient,
  establishmentId: string,
  body: PrintJobInput,
  idempotencyKey: string,
): Promise<PrintJob> {
  return call(() =>
    client.POST(`${base}/print-jobs`, {
      params: { path: { establishmentId }, header: { 'Idempotency-Key': idempotencyKey } },
      body,
    }),
  );
}

export function getPrintJob(
  client: ApiClient,
  establishmentId: string,
  printJobId: string,
): Promise<PrintJob> {
  return call(() =>
    client.GET(`${base}/print-jobs/{printJobId}`, {
      params: { path: { establishmentId, printJobId } },
    }),
  );
}
