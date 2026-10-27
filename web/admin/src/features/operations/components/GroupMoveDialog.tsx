import {
  shouldRetry,
  useApiClient,
  useProblemText,
  type ApiProblem,
  type Schemas,
} from '@arenile/shared/api';
import { formatIsoDate } from '@arenile/shared/lib/dates';
import { formatCents } from '@arenile/shared/lib/money';
import type { MapSpot } from '@arenile/shared/map';
import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { DialogFooter } from '@arenile/shared/ui/dialog';
import { FormDialog } from '@arenile/shared/ui/patterns';
import { Skeleton } from '@arenile/shared/ui/skeleton';
import { Spinner } from '@arenile/shared/ui/spinner';
import { useQuery } from '@tanstack/react-query';
import { ArrowRightIcon, CircleAlertIcon } from 'lucide-react';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { OnlineOnlyNotice } from '../../../offline/OnlineOnlyNotice';
import { useCurrentEstablishment } from '../../../establishment';
import { moveGroup, useMoveGroup } from '../api/m8';
import { todayIn } from '../lib/dates';
import { groupMoveTargets } from '../lib/groupMove';
import { defaultMoveFrom, moveFromDate } from '../lib/moveFrom';
import type { OperationalSpot } from '../lib/status';
import { MoveFromField, type MoveFromValue } from './MoveFromField';
import { useUndoToast } from './useUndoToast';

type GroupMoveResult = Schemas['GroupMoveResult'];

export interface GroupMoveDialogProps {
  establishmentId: string;
  geometry: { spots: readonly MapSpot[] };
  /** Spots of the group being moved (each with its booking line). */
  sources: readonly OperationalSpot[];
  /** The spot whose new place was picked, and that place. */
  anchorId: string;
  anchorTarget: string;
  date: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onDone: () => void;
}

/**
 * Group move (spec 30.7): the lines of a group move together keeping
 * their arrangement on the drawing (the anchor goes where it was dropped,
 * the others follow with the same offset). A dry run checks every new
 * spot; the move is all or nothing on the server, with an audit entry and
 * an event per booking, undoable for 5 minutes.
 */
export function GroupMoveDialog({
  establishmentId,
  geometry,
  sources,
  anchorId,
  anchorTarget,
  date,
  open,
  onOpenChange,
  onDone,
}: GroupMoveDialogProps) {
  const { t } = useTranslation('operations');
  const client = useApiClient();
  const problemText = useProblemText();
  const group = useMoveGroup(establishmentId);
  const undoToast = useUndoToast(establishmentId);
  const { establishment } = useCurrentEstablishment();
  const today = todayIn(establishment?.timezone);
  const labelOf = useMemo(
    () => new Map(geometry.spots.map((s) => [s.id, s.label])),
    [geometry.spots],
  );
  const targets = useMemo(
    () =>
      groupMoveTargets(
        geometry.spots,
        sources.map((s) => s.spot_id),
        anchorId,
        anchorTarget,
      ),
    [geometry.spots, sources, anchorId, anchorTarget],
  );
  const unresolved = targets.filter((x) => !x.toSpotId);
  const stay = useMemo(() => {
    let starts = date;
    let ends = date;
    sources.forEach((s, i) => {
      const b = s.booking;
      const st = b?.segment_starts_on ?? b?.starts_on ?? date;
      const en = b?.segment_ends_on ?? b?.ends_on ?? date;
      if (i === 0 || st > starts) starts = st;
      if (i === 0 || en < ends) ends = en;
    });
    return { starts_on: starts, ends_on: ends };
  }, [sources, date]);
  const [from, setFrom] = useState<MoveFromValue>(() => defaultMoveFrom(stay, today, date));
  const [error, setError] = useState<string | null>(null);
  const bySpot = new Map(sources.map((s) => [s.spot_id, s]));
  const moves = targets
    .filter((x) => x.toSpotId && x.toSpotId !== x.fromSpotId)
    .map((x) => {
      const b = bySpot.get(x.fromSpotId)?.booking;
      return {
        booking_id: b?.booking_id ?? '',
        line_id: b?.line_id ?? '',
        spot_id: x.toSpotId as string,
        expected_version: b?.version,
      };
    });
  const fromDate = moveFromDate(from.choice, stay, today, from.date);
  const body = { moves, ...(fromDate ? { from: fromDate } : {}) };
  const ready = unresolved.length === 0 && moves.length > 0 && moves.every((m) => m.line_id);
  const preview = useQuery<GroupMoveResult, ApiProblem>({
    queryKey: ['group-move-preview', establishmentId, body],
    queryFn: () => moveGroup(client, establishmentId, { ...body, dry_run: true }),
    enabled: open && ready,
    retry: shouldRetry,
    gcTime: 0,
  });
  const conflicts = preview.data?.conflicts ?? [];
  const priceDiff = (preview.data?.results ?? []).reduce((n, r) => n + r.price_difference, 0);

  const confirm = () => {
    setError(null);
    group.mutate(
      { ...body, dry_run: false },
      {
        onSuccess: (res) => {
          undoToast(t('groupMove.done', { count: moves.length }), res.undo);
          onDone();
        },
        onError: (e) => setError(problemText(e)),
      },
    );
  };

  return (
    <FormDialog
      open={open}
      onOpenChange={(v) => !group.isPending && onOpenChange(v)}
      title={t('groupMove.title', { count: sources.length })}
      description={t('groupMove.description')}
    >
      <div className="space-y-4">
        <OnlineOnlyNotice />
        <ul className="space-y-1 text-sm" aria-label={t('groupMove.plan')}>
          {targets.map((x) => (
            <li key={x.fromSpotId} className="flex items-center gap-2">
              <span className="font-medium">{labelOf.get(x.fromSpotId)}</span>
              <ArrowRightIcon aria-hidden="true" className="size-4" />
              <span className="sr-only">{t('groupMove.to')}</span>
              {x.toSpotId ? (
                <span className="font-medium">{labelOf.get(x.toSpotId)}</span>
              ) : (
                <span className="text-destructive">{t('groupMove.noPlace')}</span>
              )}
            </li>
          ))}
        </ul>
        {unresolved.length > 0 && (
          <Alert variant="destructive">
            <CircleAlertIcon aria-hidden="true" />
            <AlertDescription>{t('groupMove.unresolved')}</AlertDescription>
          </Alert>
        )}
        <MoveFromField
          stay={stay}
          today={today}
          value={from}
          onChange={setFrom}
          disabled={group.isPending}
        />
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
                  <li key={`${c.date}-${c.slot}-${c.spot_id ?? ''}-${c.reason}`}>
                    {formatIsoDate(c.date)} {t(`slot.${c.slot}`)}: {t(`conflict.${c.reason}`)}
                  </li>
                ))}
              </ul>
            </AlertDescription>
          </Alert>
        )}
        {preview.data && conflicts.length === 0 && (
          <p className="text-sm tabular-nums" data-testid="group-move-price">
            {priceDiff === 0
              ? t('move.samePrice')
              : priceDiff > 0
                ? t('move.toCollect', { amount: formatCents(priceDiff) })
                : t('move.toRefund', { amount: formatCents(-priceDiff) })}
          </p>
        )}
        {error && (
          <Alert variant="destructive">
            <CircleAlertIcon aria-hidden="true" />
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}
        <DialogFooter className="border-t pt-4">
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={group.isPending}>
            {t('actions.cancel')}
          </Button>
          <Button
            disabled={!ready || !preview.data || conflicts.length > 0 || group.isPending}
            onClick={confirm}
          >
            {group.isPending && <Spinner />}
            {t('groupMove.confirm')}
          </Button>
        </DialogFooter>
      </div>
    </FormDialog>
  );
}
