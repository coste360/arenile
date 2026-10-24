import { useProblemText } from '@arenile/shared/api';
import { formatIsoDate } from '@arenile/shared/lib/dates';
import { formatCents } from '@arenile/shared/lib/money';
import { Alert, AlertDescription } from '@arenile/shared/ui/alert';
import { Badge } from '@arenile/shared/ui/badge';
import { Button } from '@arenile/shared/ui/button';
import { ConfirmDialog, StatusBadge } from '@arenile/shared/ui/patterns';
import { Separator } from '@arenile/shared/ui/separator';
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@arenile/shared/ui/sheet';
import { Spinner } from '@arenile/shared/ui/spinner';
import { Link } from '@tanstack/react-router';
import {
  ArrowRightLeftIcon,
  CalendarMinusIcon,
  CalendarPlusIcon,
  CircleAlertIcon,
  ExternalLinkIcon,
  LogInIcon,
  PlusIcon,
  UndoIcon,
  WalletIcon,
  ZapIcon,
} from 'lucide-react';
import { useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { todayIso } from '../../bookings/model';
import { usePackName } from '../../bookings/usePackName';
import { OfflineSpotActions } from '../../../offline/OfflineSpotActions';
import { spotLabel } from '../../../offline/labels';
import { useQueueableCheckIn } from '../../../offline/useQueueableCheckIn';
import { useReleaseDay, useRevokeRelease } from '../api/operations';
import type { OperationsPermissions } from '../lib/permissions';
import { spotActions } from '../lib/actions';
import {
  VIP_TAG,
  slotStatus,
  toSpotStatus,
  type OperationalSpot,
  type SlotChoice,
} from '../lib/status';

export interface SpotSheetProps {
  establishmentId: string;
  spot: OperationalSpot | null;
  date: string;
  slot: SlotChoice;
  permissions: OperationsPermissions;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onNewBooking: (spot: OperationalSpot) => void;
  onStartMove: (spot: OperationalSpot) => void;
  /** Quick walk-in sale on a free spot ("Vendi giornata", daily-sale feature). */
  onDailySale?: (spot: OperationalSpot) => void;
}

function Row({ label, children }: { label: ReactNode; children: ReactNode }) {
  return (
    <div className="flex items-baseline justify-between gap-4 text-sm">
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="text-right font-medium tabular-nums">{children}</dd>
    </div>
  );
}

/**
 * Quick card of a spot (spec 11.3 "tocco su una postazione"): customer,
 * period, packs, extras, balance, and the actions allowed by the status and
 * the user's permissions. Mutations invalidate only the operational areas.
 */
export function SpotSheet({
  establishmentId,
  spot,
  date,
  slot,
  permissions,
  open,
  onOpenChange,
  onNewBooking,
  onStartMove,
  onDailySale,
}: SpotSheetProps) {
  const { t } = useTranslation('operations');
  const problemText = useProblemText();
  const packName = usePackName(establishmentId);
  const checkIn = useQueueableCheckIn(establishmentId);
  const release = useReleaseDay(establishmentId);
  const revoke = useRevokeRelease(establishmentId);
  const [error, setError] = useState<string | null>(null);

  const onOpen = (v: boolean) => {
    if (!v) setError(null);
    onOpenChange(v);
  };

  if (!spot) {
    return (
      <Sheet open={open} onOpenChange={onOpen}>
        <SheetContent className="w-full sm:max-w-md" aria-describedby={undefined}>
          <SheetHeader>
            <SheetTitle>{t('sheet.empty')}</SheetTitle>
          </SheetHeader>
        </SheetContent>
      </Sheet>
    );
  }

  const status = slotStatus(spot, slot);
  const colour = toSpotStatus(status, spot.booking);
  const b = spot.booking;
  const actions = spotActions(spot, slot, permissions, { date, today: todayIso() });
  const vip = b?.customer_tags.some((tag) => tag.toLowerCase() === VIP_TAG);

  const doCheckIn = () => {
    if (!b) return;
    setError(null);
    checkIn.mutate(
      { bookingId: b.booking_id, version: b.version, label: spotLabel(spot) },
      {
        onSuccess: (res) => {
          toast.success(
            res.queued
              ? t('offline:actions.queuedCheckIn', { code: b.code })
              : t('sheet.checkedIn', { code: b.code }),
          );
          onOpen(false);
        },
        onError: (e) => setError(problemText(e)),
      },
    );
  };

  return (
    <Sheet open={open} onOpenChange={onOpen}>
      <SheetContent className="w-full overflow-y-auto sm:max-w-md">
        <SheetHeader>
          <SheetTitle>{t('sheet.title', { label: spot.label })}</SheetTitle>
          <SheetDescription className="flex flex-wrap items-center gap-2">
            <StatusBadge spotStatus={colour}>{t(`status.${status}`)}</StatusBadge>
            <span>
              {formatIsoDate(date)} · {t(`slot.${slot}`)}
            </span>
          </SheetDescription>
        </SheetHeader>

        <div className="space-y-4 px-4">
          {b && (
            <section aria-label={t('sheet.booking')} className="space-y-3">
              <div className="flex flex-wrap items-center gap-2">
                <p className="text-base font-semibold">{b.customer_display_name}</p>
                {vip && <Badge variant="secondary">{t('indicator.vip')}</Badge>}
                {b.is_season_pass && <Badge variant="outline">{t('sheet.seasonPass')}</Badge>}
              </div>
              <dl className="space-y-1.5">
                {b.ticket && <Row label={t('sheet.ticket')}>{b.ticket.label}</Row>}
                <Row label={t('sheet.code')}>{b.code}</Row>
                <Row label={t('sheet.period')}>
                  {b.starts_on === b.ends_on
                    ? formatIsoDate(b.starts_on)
                    : `${formatIsoDate(b.starts_on)} – ${formatIsoDate(b.ends_on)}`}
                </Row>
                <Row label={t('sheet.slots')}>{t(`slot.${b.slots}`)}</Row>
                {b.pack_codes.length > 0 && (
                  <Row label={t('sheet.packs')}>{b.pack_codes.map(packName).join(', ')}</Row>
                )}
                <Row label={t('sheet.bookingStatus')}>{t(`bookingStatus.${b.status}`)}</Row>
              </dl>
              {(b.has_extras || b.has_notes) && (
                <div className="flex flex-wrap gap-1.5">
                  {b.has_extras && <Badge variant="outline">{t('indicator.extra')}</Badge>}
                  {b.has_notes && <Badge variant="outline">{t('indicator.note')}</Badge>}
                </div>
              )}
              <Separator />
              <dl className="space-y-1.5">
                <Row label={t('sheet.total')}>{formatCents(b.total_amount)}</Row>
                <Row label={t('sheet.paid')}>{formatCents(b.paid_amount)}</Row>
                <Row label={t('sheet.balance')}>
                  <span className={b.balance_due > 0 ? 'text-destructive' : undefined}>
                    {formatCents(b.balance_due)}
                  </span>
                </Row>
              </dl>
            </section>
          )}
          {status === 'held' && spot.hold_expires_at && (
            <p className="text-sm text-muted-foreground">
              {t('sheet.holdExpires', {
                time: new Date(spot.hold_expires_at).toLocaleTimeString(undefined, {
                  hour: '2-digit',
                  minute: '2-digit',
                }),
              })}
            </p>
          )}
          {status === 'blocked' && (
            <p className="text-sm text-muted-foreground">
              {spot.block_reason
                ? t('sheet.blockedReason', { reason: spot.block_reason })
                : t('sheet.blocked')}
            </p>
          )}
          {status === 'free' && <p className="text-sm text-muted-foreground">{t('sheet.free')}</p>}
          {error && (
            <Alert variant="destructive">
              <CircleAlertIcon aria-hidden="true" />
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}
        </div>

        <SheetFooter className="gap-2">
          {actions.checkIn && (
            <Button onClick={doCheckIn} disabled={checkIn.isPending}>
              {checkIn.isPending ? <Spinner /> : <LogInIcon aria-hidden="true" />}
              {t('actions.checkIn')}
            </Button>
          )}
          {actions.dailySale && onDailySale && (
            <Button onClick={() => onDailySale(spot)}>
              <ZapIcon aria-hidden="true" />
              {t('actions.dailySale')}
            </Button>
          )}
          {actions.newBooking && (
            <Button
              variant={actions.dailySale && onDailySale ? 'outline' : 'default'}
              onClick={() => onNewBooking(spot)}
            >
              <PlusIcon aria-hidden="true" />
              {t('actions.newBooking')}
            </Button>
          )}
          {actions.releaseDay && b && (
            <ConfirmDialog
              title={t('release.title', { label: spot.label })}
              description={t('release.description', {
                customer: b.customer_display_name,
                date: formatIsoDate(date),
                slot: t(`slot.${slot}`),
              })}
              confirmLabel={t('actions.releaseDay')}
              errorText={problemText}
              onConfirm={() =>
                release.mutateAsync({ passId: b.booking_id, date, slot }).then(() => {
                  toast.success(t('release.done', { label: spot.label }));
                  onOpen(false);
                })
              }
              trigger={
                <Button variant="outline">
                  <CalendarMinusIcon aria-hidden="true" />
                  {t('actions.releaseDay')}
                </Button>
              }
            />
          )}
          {actions.revokeRelease && b && spot.release_id && (
            <ConfirmDialog
              title={t('revoke.title', { label: spot.label })}
              description={t('revoke.description', { customer: b.customer_display_name })}
              confirmLabel={t('actions.revokeRelease')}
              errorText={problemText}
              onConfirm={() =>
                revoke
                  .mutateAsync({ passId: b.booking_id, releaseId: spot.release_id as string })
                  .then(() => {
                    toast.success(t('revoke.done', { label: spot.label }));
                    onOpen(false);
                  })
              }
              trigger={
                <Button variant="outline">
                  <UndoIcon aria-hidden="true" />
                  {t('actions.revokeRelease')}
                </Button>
              }
            />
          )}
          {actions.move && (
            <Button variant="outline" onClick={() => onStartMove(spot)}>
              <ArrowRightLeftIcon aria-hidden="true" />
              {t('actions.move')}
            </Button>
          )}
          {actions.collect && b && (
            <Button variant="outline" asChild>
              <Link
                to="/e/$establishmentId/prenotazioni/$bookingId"
                params={{ establishmentId, bookingId: b.booking_id }}
              >
                <WalletIcon aria-hidden="true" />
                {t('actions.collect')}
              </Link>
            </Button>
          )}
          {actions.openDetail && b && (
            <Button variant="ghost" asChild>
              <Link
                to="/e/$establishmentId/prenotazioni/$bookingId"
                params={{ establishmentId, bookingId: b.booking_id }}
              >
                <ExternalLinkIcon aria-hidden="true" />
                {t('actions.openDetail')}
              </Link>
            </Button>
          )}
          <OfflineSpotActions establishmentId={establishmentId} spot={spot} />
          {!actions.newBooking && status === 'free' && !permissions.canWriteBookings && (
            <p className="text-sm text-muted-foreground">
              <CalendarPlusIcon aria-hidden="true" className="mr-1 inline size-4" />
              {t('sheet.readOnly')}
            </p>
          )}
        </SheetFooter>
      </SheetContent>
    </Sheet>
  );
}
