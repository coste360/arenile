import { ApiProblem, useApiClient, useProblemText, type ApiClient } from '@arenile/shared/api';
import { useCallback, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { useRequiredEstablishmentId } from '../../establishment';
import { usePanelMode } from '../../lib/meta';
import { newIdempotencyKey } from '../configuration/lib/validation';
import {
  createPrintJob,
  getPrintJob,
  resolveRoute,
  type PrintDocumentKind,
  type PrintJob,
  type PrintRouteUnreadyReason,
} from './api';

/** Problem types of the printing API with their own message. */
const PRINTING_PROBLEMS = [
  'printing_unavailable',
  'printer_address_not_allowed',
  'print_route_missing',
] as const;

/** Message of a printing error: the printing problem types first, then the shared mapping. */
export function usePrintingProblemText() {
  const { t } = useTranslation('printers');
  const generic = useProblemText();
  return (error: unknown) =>
    error instanceof ApiProblem && (PRINTING_PROBLEMS as readonly string[]).includes(error.type)
      ? t(`problem.${error.type}`)
      : generic(error);
}

const TERMINAL = new Set<PrintJob['status']>(['printed', 'failed', 'cancelled']);

export interface WatchOptions {
  /** Delay between polls (ms). */
  intervalMs?: number;
  /** Give up after this many polls (the job list keeps showing the state). */
  maxPolls?: number;
}

/**
 * Polls a print job until it is printed, failed or cancelled. Resolves with
 * the last state read, or null when the job is still running after
 * `maxPolls` (the retries with backoff can take minutes) or the job cannot
 * be read.
 */
export async function watchPrintJob(
  client: ApiClient,
  establishmentId: string,
  job: PrintJob,
  { intervalMs = 1000, maxPolls = 20 }: WatchOptions = {},
): Promise<PrintJob | null> {
  let current = job;
  for (let i = 0; i < maxPolls && !TERMINAL.has(current.status); i++) {
    await new Promise((r) => setTimeout(r, intervalMs));
    try {
      current = await getPrintJob(client, establishmentId, job.id);
    } catch {
      return null;
    }
  }
  return TERMINAL.has(current.status) ? current : null;
}

export interface PrintRequest {
  kind: Exclude<PrintDocumentKind, 'test_page'>;
  /** Ticket, cash session or payment id. */
  documentId: string;
  /** Register printing the document (selects the routing rule). */
  registerId?: string;
}

/** Why the document went to the browser instead of an ESC/POS printer. */
export type FallbackReason = PrintRouteUnreadyReason | 'error';

/**
 * Print-or-fallback for the cash buttons (ADR 0021). On the desktop edition
 * it resolves the route of the document; when a printer is configured and
 * reachable it queues an ESC/POS job (Idempotency-Key per click, so a double
 * click is one job) and reports the outcome, offering the browser print if
 * the job fails. Otherwise (server edition, no route, printer inactive or
 * unreachable, API error) it runs the browser print `fallback` and says why
 * in a notice (silently on the server edition, where the browser is the only
 * way to print). A document without a browser layout (the payment receipt)
 * passes no fallback: it then reports an error with the reason and no
 * browser action.
 */
export function usePrintDocument(watch?: WatchOptions) {
  const { t } = useTranslation('printers');
  const client = useApiClient();
  const establishmentId = useRequiredEstablishmentId();
  const mode = usePanelMode();
  const desktop = mode === 'local' || mode === 'lan';
  const problemText = usePrintingProblemText();
  const inFlight = useRef(false);

  return useCallback(
    async (req: PrintRequest, fallback?: () => void): Promise<'escpos' | 'browser' | 'none'> => {
      const browser = (reason: FallbackReason) => {
        if (!fallback) {
          toast.error(t('print.notPrinted', { document: t(`kind.${req.kind}`) }), {
            description: t(`fallback.${reason}`),
          });
          return 'none' as const;
        }
        fallback();
        // The server edition always prints from the browser: no notice there.
        if (reason !== 'server_edition') {
          toast.info(t('fallback.title'), { description: t(`fallback.${reason}`) });
        }
        return 'browser' as const;
      };
      const browserAction = fallback ? { label: t('print.browser'), onClick: fallback } : undefined;
      if (!desktop) return browser('server_edition');
      if (inFlight.current) return 'escpos';
      inFlight.current = true;
      try {
        let route;
        try {
          route = await resolveRoute(client, establishmentId, req.kind, req.registerId);
        } catch {
          return browser('error');
        }
        if (!route.ready || !route.printer) return browser(route.reason ?? 'no_route');
        const printerName = route.printer.name;
        let job: PrintJob;
        try {
          job = await createPrintJob(
            client,
            establishmentId,
            {
              document_kind: req.kind,
              document_id: req.documentId,
              ...(req.registerId ? { register_id: req.registerId } : {}),
            },
            newIdempotencyKey(),
          );
        } catch (err) {
          toast.error(t('print.failed', { name: printerName }), {
            description: problemText(err),
            action: browserAction,
          });
          return 'escpos';
        }
        toast.success(t('print.sent', { name: printerName }));
        void watchPrintJob(client, establishmentId, job, watch).then((last) => {
          if (last?.status === 'failed') {
            toast.error(t('print.failed', { name: printerName }), {
              description: last.last_error,
              action: browserAction,
            });
          }
        });
        return 'escpos';
      } finally {
        inFlight.current = false;
      }
    },
    [client, desktop, establishmentId, problemText, t, watch],
  );
}
