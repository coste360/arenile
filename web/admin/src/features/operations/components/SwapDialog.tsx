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
import { Skeleton } from '@arenile/shared/ui/skeleton';
import { Spinner } from '@arenile/shared/ui/spinner';
import { useQuery } from '@tanstack/react-query';
import { ArrowLeftRightIcon, CircleAlertIcon } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { OnlineOnlyNotice } from '../../../offline/OnlineOnlyNotice';
import { useCurrentEstablishment } from '../../../establishment';
import { swapBookings, useSwapBookings } from '../api/m8';
import { todayIn } from '../lib/dates';
import { commonWindow, defaultMoveFrom, moveFromDate } from '../lib/moveFrom';
import type { OperationalSpot } from '../lib/status';
import { MoveFromField, type MoveFromValue } from './MoveFromField';
import { useUndoToast } from './useUndoToast';

type BookingSwapResult = Schemas['BookingSwapResult'];
type BookingSwapOutcome = Schemas['BookingSwapOutcome'];

export interface SwapDialogProps {
  establishmentId: string;
  /** The booking being moved (picked first). */
  source: OperationalSpot;
  /** The occupied spot it was dropped on. */
  target: OperationalSpot;
  /** Day shown on the map. */
  date: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onDone: () => void;
}

/**
 * Swap of two bookings (spec 30.7): dropping a booking on an occupied spot
 * proposes to exchange the two spots from a chosen day on their common
 * stay. A dry run shows the price difference of both bookings; the swap is
 * one transaction on the server (no instant of double occupation) with an
 * audit entry and an event for each booking, and can be undone for 5
 * minutes. Outside the common stay each booking keeps its spot.
 */
export function SwapDialog({
  establishmentId,
  source,
  target,
  date,
  open,
  onOpenChange,
  onDone,
}: SwapDialogProps) {
  const { t } = useTranslation('operations');
  const client = useApiClient();
  const problemText = useProblemText();
  const swap = useSwapBookings(establishmentId);
  const undoToast = useUndoToast(establishmentId);
  const { establishment } = useCurrentEstablishment();
  const today = todayIn(establishment?.timezone);
  const stay = commonWindow(source, target, date);
  const [from, setFrom] = useState<MoveFromValue>(() => defaultMoveFrom(stay, today, date));
  const [error, setError] = useState<string | null>(null);
  const a = source.booking;
  const b = target.booking;
  const valid = !!a?.line_id && !!b?.line_id && stay.starts_on <= stay.ends_on;
  const body = {
    a: { booking_id: a?.booking_id ?? '', line_id: a?.line_id ?? '', expected_version: a?.version },
    b: { booking_id: b?.booking_id ?? '', line_id: b?.line_id ?? '', expected_version: b?.version },
    from: moveFromDate(from.choice, stay, today, from.date) ?? stay.starts_on,
  };
  const preview = useQuery<BookingSwapResult, ApiProblem>({
    queryKey: ['swap-preview', establishmentId, body],
    queryFn: () => swapBookings(client, establishmentId, { ...body, dry_run: true }),
    enabled: open && valid,
    retry: shouldRetry,
    gcTime: 0,
  });
  const conflicts = preview.data?.conflicts ?? [];
  const blocked = !valid || conflicts.length > 0 || !!preview.error || !preview.data;

  const confirm = () => {
    setError(null);
    swap.mutate(
      { ...body, dry_run: false },
      {
        onSuccess: (res) => {
          undoToast(t('swap.done', { a: source.label, b: target.label }), res.undo);
          onDone();
        },
        onError: (e) => setError(problemText(e)),
      },
    );
  };

  return (
    <AlertDialog open={open} onOpenChange={(v) => !swap.isPending && onOpenChange(v)}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle className="flex items-center gap-2">
            <ArrowLeftRightIcon aria-hidden="true" className="size-5" />
            {t('swap.title', { a: source.label, b: target.label })}
          </AlertDialogTitle>
          <AlertDialogDescription>
            {t('swap.description', {
              a: a?.code ?? '',
              b: b?.code ?? '',
              from: source.label,
              to: target.label,
            })}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <div className="space-y-3">
          <OnlineOnlyNotice />
          {!valid ? (
            <Alert variant="destructive">
              <CircleAlertIcon aria-hidden="true" />
              <AlertDescription>{t('swap.noOverlap')}</AlertDescription>
            </Alert>
          ) : (
            <MoveFromField
              stay={stay}
              today={today}
              value={from}
              onChange={setFrom}
              disabled={swap.isPending}
            />
          )}
          {preview.isLoading && (
            <div role="status">
              <span className="sr-only">{t('move.previewing')}</span>
              <Skeleton aria-hidden="true" className="h-16 w-full" />
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
          {preview.data && conflicts.length === 0 && (
            <div className="space-y-2 text-sm" data-testid="swap-preview">
              <p>
                {t('swap.period', {
                  from: formatIsoDate(preview.data.from),
                  to: formatIsoDate(preview.data.to),
                })}
              </p>
              <ul className="space-y-1">
                <OutcomeRow code={a?.code ?? ''} outcome={preview.data.a} />
                <OutcomeRow code={b?.code ?? ''} outcome={preview.data.b} />
              </ul>
            </div>
          )}
          {error && (
            <Alert variant="destructive">
              <CircleAlertIcon aria-hidden="true" />
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}
        </div>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={swap.isPending}>{t('actions.cancel')}</AlertDialogCancel>
          <Button disabled={blocked || swap.isPending} onClick={confirm}>
            {swap.isPending && <Spinner />}
            {t('swap.confirm')}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

function OutcomeRow({ code, outcome }: { code: string; outcome: BookingSwapOutcome }) {
  const { t } = useTranslation('operations');
  const diff = outcome.price_difference;
  return (
    <li className="flex flex-wrap items-baseline justify-between gap-2 tabular-nums">
      <span className="font-medium">{code}</span>
      <span>
        {diff === 0
          ? t('move.samePrice')
          : diff > 0
            ? t('move.toCollect', { amount: formatCents(diff) })
            : t('move.toRefund', { amount: formatCents(-diff) })}
      </span>
    </li>
  );
}
