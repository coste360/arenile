import {
  shouldRetry,
  useApiClient,
  useProblemText,
  type ApiProblem,
  type Schemas,
} from '@arenile/shared/api';
import { formatIsoDate } from '@arenile/shared/lib/dates';
import { formatCents } from '@arenile/shared/lib/money';
import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@arenile/shared/ui/alert-dialog';
import { Button } from '@arenile/shared/ui/button';
import { Spinner } from '@arenile/shared/ui/spinner';
import { Skeleton } from '@arenile/shared/ui/skeleton';
import { useQuery } from '@tanstack/react-query';
import { CircleAlertIcon } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { OnlineOnlyNotice } from '../../../offline/OnlineOnlyNotice';
import { useMoveTicket } from '../../daily-sale';
import { moveLine, useMoveLine } from '../api/operations';
import type { OperationalSpot } from '../lib/status';

type BookingChangeResult = Schemas['BookingChangeResult'];

export interface MoveDialogProps {
  establishmentId: string;
  source: OperationalSpot;
  target: { spotId: string; label: string };
  /** Day from which the line moves (the day shown on the map). */
  date: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onDone: () => void;
}

/**
 * Confirmation of a move on the map (spec 11.3 "trascinamento"): a dry run
 * previews conflicts and the price difference (category change), then the
 * move is applied atomically by the server (allocations swapped in one
 * transaction). Conflicts block the confirmation.
 */
export function MoveDialog({
  establishmentId,
  source,
  target,
  date,
  open,
  onOpenChange,
  onDone,
}: MoveDialogProps) {
  const { t } = useTranslation('operations');
  const client = useApiClient();
  const problemText = useProblemText();
  const move = useMoveLine(establishmentId);
  const moveTicket = useMoveTicket(establishmentId);
  const booking = source.booking;
  // A daily ticket (ADR 0020) moves through the ticket actions: the generic
  // booking move is refused for it (ticket_owned) and its price never
  // changes, so there is no preview to run.
  const ticket = booking?.ticket;
  const vars = {
    bookingId: booking?.booking_id ?? '',
    lineId: booking?.line_id ?? '',
    spotId: target.spotId,
    from: date > (booking?.starts_on ?? date) ? date : undefined,
    version: booking?.version ?? 0,
  };
  const preview = useQuery<BookingChangeResult, ApiProblem>({
    // Outside the SSE-invalidated areas: applying the move must not re-run the preview.
    queryKey: ['move-preview', establishmentId, vars],
    queryFn: () => moveLine(client, establishmentId, { ...vars, dryRun: true }),
    enabled: open && !ticket && !!vars.bookingId && !!vars.lineId && vars.version > 0,
    retry: shouldRetry,
    gcTime: 0,
  });
  const conflicts = preview.data?.conflicts ?? [];
  const blocked = conflicts.length > 0 || !!preview.error;
  const pending = move.isPending || moveTicket.isPending;

  const [error, setError] = useState<string | null>(null);
  const confirm = () => {
    setError(null);
    const callbacks = {
      onSuccess: () => {
        toast.success(t('move.done', { code: booking?.code ?? '', to: target.label }));
        onDone();
      },
      onError: (e: ApiProblem) => setError(problemText(e)),
    };
    if (ticket) {
      moveTicket.mutate({ ticketId: ticket.id, spotId: target.spotId, version: ticket.version }, callbacks);
      return;
    }
    move.mutate({ ...vars, dryRun: false }, callbacks);
  };

  return (
    <AlertDialog open={open} onOpenChange={(v) => !pending && onOpenChange(v)}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>
            {t('move.title', { from: source.label, to: target.label })}
          </AlertDialogTitle>
          <AlertDialogDescription>
            {t('move.description', {
              code: booking?.code ?? '',
              customer: booking?.customer_display_name ?? '',
              from: source.label,
              to: target.label,
              date: formatIsoDate(vars.from ?? booking?.starts_on ?? date),
            })}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <div className="space-y-3">
          <OnlineOnlyNotice />
          {preview.isLoading && (
            <div role="status">
              <span className="sr-only">{t('move.previewing')}</span>
              <Skeleton aria-hidden="true" className="h-12 w-full" />
            </div>
          )}
          {preview.error && (
            <Alert variant="destructive">
              <CircleAlertIcon aria-hidden="true" />
              <AlertDescription>{problemText(preview.error)}</AlertDescription>
            </Alert>
          )}
          {preview.data && conflicts.length > 0 && (
            <Alert variant="destructive">
              <CircleAlertIcon aria-hidden="true" />
              <AlertTitle>{t('move.conflicts', { count: conflicts.length })}</AlertTitle>
              <AlertDescription>
                <ul className="list-disc pl-4">
                  {conflicts.slice(0, 5).map((c) => (
                    <li key={`${c.date}-${c.slot}-${c.reason}`}>
                      {formatIsoDate(c.date)} {t(`slot.${c.slot}`)}: {t(`conflict.${c.reason}`)}
                    </li>
                  ))}
                </ul>
              </AlertDescription>
            </Alert>
          )}
          {ticket && (
            <p className="text-sm" data-testid="move-price">
              {t('move.ticketPriceKept', { number: ticket.label })}
            </p>
          )}
          {preview.data && conflicts.length === 0 && (
            <p className="text-sm tabular-nums" data-testid="move-price">
              {preview.data.price_difference === 0
                ? t('move.samePrice')
                : preview.data.price_difference > 0
                  ? t('move.toCollect', { amount: formatCents(preview.data.price_difference) })
                  : t('move.toRefund', { amount: formatCents(-preview.data.price_difference) })}
            </p>
          )}
          {error && (
            <Alert variant="destructive">
              <CircleAlertIcon aria-hidden="true" />
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}
        </div>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={pending}>{t('actions.cancel')}</AlertDialogCancel>
          <Button disabled={blocked || (!ticket && !preview.data) || pending} onClick={confirm}>
            {pending && <Spinner />}
            {t('move.confirm')}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
