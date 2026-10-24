import { shouldRetry, useApiClient, type ApiProblem } from '@arenile/shared/api';
import { useQuery } from '@tanstack/react-query';
import { useRequiredEstablishmentId } from '../../establishment';
import { usePanelMode } from '../../lib/meta';
import {
  printingKeys,
  resolveRoute,
  type PrintDocumentKind,
  type PrintRouteResolution,
} from './api';

/**
 * Whether a document of `kind` would go to a ready ESC/POS printer now
 * (desktop edition, routing rule, printer active and reachable). Used to
 * offer print actions that have no browser layout, such as the payment
 * receipt; never queried on the server edition.
 */
export function usePrintReady(
  kind: Exclude<PrintDocumentKind, 'test_page'>,
  registerId?: string,
): boolean {
  const client = useApiClient();
  const establishmentId = useRequiredEstablishmentId();
  const mode = usePanelMode();
  const desktop = mode === 'local' || mode === 'lan';
  const q = useQuery<PrintRouteResolution, ApiProblem>({
    queryKey: printingKeys.resolve(establishmentId, kind, registerId),
    enabled: desktop,
    retry: shouldRetry,
    staleTime: 30_000,
    queryFn: () => resolveRoute(client, establishmentId, kind, registerId),
  });
  return desktop && q.data?.ready === true;
}
