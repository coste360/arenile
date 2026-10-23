import { isVersionConflict, useProblemText } from '@arenile/shared/api';
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
import { CircleAlertIcon, RotateCwIcon } from 'lucide-react';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { OnlineOnlyNotice } from '../../../offline/OnlineOnlyNotice';
import { addDays } from '../../operations/lib/dates';
import { useBookingDetail, usePatchBooking, usePatchPreview, type PatchBookingVars } from '../api';
import { buildMovePatch } from '../lib/move';

export interface PlanningMove {
  bookingId: string;
  code: string;
  label: string;
  dayDelta: number;
  sourceSpotId: string;
  sourceSpotLabel: string;
  targetSpotId: string;
  targetSpotLabel: string;
}

export interface PlanningMoveDialogProps {
  establishmentId: string;
  move: PlanningMove;
  onClose: () => void;
  /** Refetch of the grid after a version conflict. */
  onReload: () => void;
}

/**
 * Confirmation of a drag on the planning: loads the booking (its version is
 * the If-Match), previews the PATCH with dry_run (price difference,
 * conflicts), then applies it. A 412 — someone changed the booking in the
 * meantime — shows a reload prompt that refetches booking and grid and
 * recomputes the preview on the fresh version.
 */
export function PlanningMoveDialog({
  establishmentId,
  move,
  onClose,
  onReload,
}: PlanningMoveDialogProps) {
  const { t } = useTranslation('planning');
  const problemText = useProblemText();
  const detail = useBookingDetail(establishmentId, move.bookingId);
  const apply = usePatchBooking(establishmentId);
  const [stale, setStale] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const vars = useMemo<PatchBookingVars | null>(() => {
    if (!detail.data || stale) return null;
    const patch = buildMovePatch(detail.data, move);
    return patch
      ? { bookingId: move.bookingId, version: detail.data.version, patch, dryRun: true }
      : null;
  }, [detail.data, move, stale]);
  const preview = usePatchPreview(establishmentId, vars);
  const conflicts = preview.data?.conflicts ?? [];
  const versionConflict = stale || isVersionConflict(preview.error);

  const reload = async () => {
    setError(null);
    onReload();
    await detail.refetch();
    setStale(false);
  };

  const confirm = () => {
    if (!vars) return;
    setError(null);
    apply.mutate(
      { ...vars, dryRun: false },
      {
        onSuccess: () => {
          toast.success(t('move.done', { code: move.code }));
          onClose();
        },
        onError: (e) => {
          if (isVersionConflict(e)) setStale(true);
          else setError(problemText(e));
        },
      },
    );
  };

  const d = detail.data;
  const newStart = d ? addDays(d.starts_on, move.dayDelta) : undefined;
  const newEnd = d ? addDays(d.ends_on, move.dayDelta) : undefined;
  const loading = detail.isLoading || (!!vars && preview.isLoading);
  const canConfirm =
    !!vars && !!preview.data && conflicts.length === 0 && !versionConflict && !apply.isPending;

  return (
    <AlertDialog open onOpenChange={(v) => !v && !apply.isPending && onClose()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t('move.title', { code: move.code })}</AlertDialogTitle>
          <AlertDialogDescription>
            {move.label}
            {move.sourceSpotId !== move.targetSpotId &&
              ` · ${t('move.spot', { from: move.sourceSpotLabel, to: move.targetSpotLabel })}`}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <div className="space-y-3 text-sm">
          <OnlineOnlyNotice />
          {d && move.dayDelta !== 0 && newStart && newEnd && (
            <p data-testid="move-dates">
              {t('move.dates', {
                from: `${formatIsoDate(d.starts_on)} – ${formatIsoDate(d.ends_on)}`,
                to: `${formatIsoDate(newStart)} – ${formatIsoDate(newEnd)}`,
              })}
            </p>
          )}
          {loading && (
            <div role="status">
              <span className="sr-only">{t('move.previewing')}</span>
              <Skeleton aria-hidden="true" className="h-12 w-full" />
            </div>
          )}
          {detail.error && (
            <Alert variant="destructive">
              <CircleAlertIcon aria-hidden="true" />
              <AlertDescription>{problemText(detail.error)}</AlertDescription>
            </Alert>
          )}
          {d && !vars && !stale && (
            <Alert variant="destructive">
              <CircleAlertIcon aria-hidden="true" />
              <AlertDescription>{t('move.notApplicable')}</AlertDescription>
            </Alert>
          )}
          {versionConflict ? (
            <Alert variant="destructive">
              <CircleAlertIcon aria-hidden="true" />
              <AlertTitle>{t('move.staleTitle')}</AlertTitle>
              <AlertDescription className="flex flex-wrap items-center gap-3">
                <span>{t('move.staleDescription')}</span>
                <Button size="sm" variant="outline" onClick={() => void reload()}>
                  <RotateCwIcon aria-hidden="true" />
                  {t('move.reload')}
                </Button>
              </AlertDescription>
            </Alert>
          ) : (
            preview.error && (
              <Alert variant="destructive">
                <CircleAlertIcon aria-hidden="true" />
                <AlertDescription>{problemText(preview.error)}</AlertDescription>
              </Alert>
            )
          )}
          {preview.data && conflicts.length > 0 && (
            <Alert variant="destructive">
              <CircleAlertIcon aria-hidden="true" />
              <AlertTitle>{t('move.conflicts', { count: conflicts.length })}</AlertTitle>
              <AlertDescription>
                <ul className="list-disc pl-4">
                  {conflicts.slice(0, 5).map((c) => (
                    <li key={`${c.spot_id ?? c.pool_id ?? ''}-${c.date}-${c.slot}`}>
                      {formatIsoDate(c.date)} {c.slot}: {t(`conflict.${c.reason}`)}
                    </li>
                  ))}
                </ul>
              </AlertDescription>
            </Alert>
          )}
          {preview.data && conflicts.length === 0 && !versionConflict && (
            <p className="tabular-nums" data-testid="move-price">
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
          <AlertDialogCancel disabled={apply.isPending}>{t('move.cancel')}</AlertDialogCancel>
          <Button onClick={confirm} disabled={!canConfirm}>
            {apply.isPending && <Spinner />}
            {t('move.confirm')}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
